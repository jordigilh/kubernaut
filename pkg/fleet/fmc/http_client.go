/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fmc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// Issue #54 fleet E2E RCA (CI run 30464667745): FMC's handler distinguishes
// a failed check (503 -- e.g. Valkey "context canceled" under resource
// pressure) from a completed check that determined "not managed" (200).
// Check retries only the former: a small, bounded number of
// attempts with exponential backoff, wrapped in a hard wall-clock ceiling.
// This sits on Gateway/RO's synchronous alert-ingestion path, so the ceiling
// is deliberately short (seconds, not the 45s a test's own outer retry
// loop might use) -- a transient blip gets a couple of quick retries, but a
// real outage still returns an error promptly rather than stalling the caller.
const (
	scopeCheckMaxAttempts  = 3
	scopeCheckRetryCeiling = 2 * time.Second
)

// scopeCheckBackoff configures the delay between retry attempts. With
// BasePeriod=100ms, Multiplier=2, MaxPeriod=500ms: attempt 1 waits ~100ms,
// attempt 2 waits ~200ms (both ±10% jitter) before the next try; combined
// with scopeCheckMaxAttempts=3, worst-case pure backoff overhead is ~300ms,
// well inside scopeCheckRetryCeiling.
var scopeCheckBackoff = backoff.Config{
	BasePeriod:    100 * time.Millisecond,
	MaxPeriod:     500 * time.Millisecond,
	Multiplier:    2.0,
	JitterPercent: 10,
}

// HTTPClient is a scope.ScopeChecker that calls the FMC REST API over HTTP.
// ADR-068: GW/RO use this client to resolve federated scope instead of connecting
// to Valkey directly. Backend failures are returned to the caller so the caller
// can apply the appropriate fail-closed policy.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

var _ scope.ScopeChecker = (*HTTPClient)(nil)

// ClientOption configures the FMC HTTP client.
type ClientOption func(*HTTPClient)

// WithHTTPClient overrides the default http.Client used for FMC requests.
// Issue #1683: use this to inject a client with CA-verified TLS transport
// (e.g. via sharedtls.NewCAReloaderFromFile) instead of relying on plaintext.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(client *HTTPClient) {
		client.httpClient = c
	}
}

// NewHTTPClient creates an FMC HTTP client targeting the given base URL.
// By default, the client uses a plain http.Client with no TLS verification.
// Use WithHTTPClient to provide a client configured with the cluster's CA cert.
func NewHTTPClient(baseURL string, opts ...ClientOption) *HTTPClient {
	c := &HTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Check calls the enum-only /api/v1/scope/check boundary. Valid unavailable
// responses and transient transport failures have bounded retry; verified,
// invalid, unsupported, denied and protocol outcomes are never retried.
func (c *HTTPClient) Check(ctx context.Context, r scope.ResourceIdentity) (scope.ScopeResult, error) {
	if ctx.Err() != nil {
		return failedScope(), ctx.Err()
	}
	invalid := scope.ScopeResult{State: scope.ScopeInvalid, Provenance: scope.ScopeSourceUnavailable}
	if r.ClusterID == "" || r.Name == "" {
		return invalid, nil
	}
	if _, err := scope.GVKForIdentity(r, true); err != nil {
		return invalid, nil //nolint:nilerr // DD-FLEET-015: missing/invalid remote GVK is a completed input-domain outcome
	}
	reqURL := c.buildScopeCheckURL(r)

	ctx, cancel := context.WithTimeout(ctx, scopeCheckRetryCeiling)
	defer cancel()

	for attempt := int32(1); attempt <= scopeCheckMaxAttempts; attempt++ {
		result, retryable, err := c.attemptScopeCheck(ctx, reqURL)
		if ctx.Err() != nil {
			return failedScope(), ctx.Err()
		}
		if !retryable || attempt == scopeCheckMaxAttempts {
			return result, err
		}

		select {
		case <-ctx.Done():
			return failedScope(), fmt.Errorf("fmc scope check canceled: %w", ctx.Err())
		case <-time.After(scopeCheckBackoff.Calculate(attempt)):
		}
	}
	return failedScope(), fmt.Errorf("fmc scope check failed")
}

// buildScopeCheckURL constructs the FMC scope-check request URL for a resource.
func (c *HTTPClient) buildScopeCheckURL(r scope.ResourceIdentity) string {
	params := url.Values{}
	params.Set("cluster", r.ClusterID)
	params.Set("group", r.Group)
	params.Set("version", r.Version)
	params.Set("kind", r.Kind)
	params.Set("namespace", r.Namespace)
	params.Set("name", r.Name)

	return c.baseURL + ScopeCheckPath + "?" + params.Encode()
}

// attemptScopeCheck performs one strict round-trip. Only a completed unavailable
// domain result or a transient transport error is eligible for bounded retry.
func (c *HTTPClient) attemptScopeCheck(ctx context.Context, reqURL string) (scope.ScopeResult, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return failedScope(), false, fmt.Errorf("build scope request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return failedScope(), ctx.Err() == nil && transientScopeTransport(err), fmt.Errorf("execute scope request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return failedScope(), false, fmt.Errorf("scope protocol: invalid media type")
	}
	result, err := DecodeScopeResponse(resp.Body)
	if err != nil {
		return failedScope(), false, err
	}
	status := scopeStatus(result.State)
	if result.State == scope.ScopeInvalid && resp.StatusCode == http.StatusBadRequest {
		status = http.StatusBadRequest
	}
	if resp.StatusCode != status {
		return failedScope(), false, fmt.Errorf("scope protocol: status and state mismatch")
	}
	return result, result.State == scope.ScopeUnavailable, nil
}

func transientScopeTransport(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

func failedScope() scope.ScopeResult {
	return scope.ScopeResult{State: scope.ScopeUnavailable, Provenance: scope.ScopeSourceUnavailable}
}

// DecodeScopeResponse rejects duplicate keys as well as unknown/missing members. A
// struct decoder alone accepts duplicates and null strings, which is unsafe.
// Serving-path qualification helpers use this same bounded production decoder.
func DecodeScopeResponse(body io.Reader) (scope.ScopeResult, error) {
	data, err := io.ReadAll(io.LimitReader(body, 4097))
	if err != nil || len(data) > 4096 {
		return failedScope(), fmt.Errorf("scope protocol: response exceeds read boundary")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return failedScope(), fmt.Errorf("scope protocol: expected object")
	}
	values := make(map[string]string, 2)
	for decoder.More() {
		if err := readScopeMember(decoder, values); err != nil {
			return failedScope(), err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return failedScope(), fmt.Errorf("scope protocol: malformed object")
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return failedScope(), fmt.Errorf("scope protocol: trailing data")
	}
	result := scope.ScopeResult{State: scope.ScopeState(values["state"]), Provenance: scope.ScopeSource(values["provenance"])}
	if err := result.Validate(); err != nil {
		return failedScope(), err
	}
	return result, nil
}

func readScopeMember(decoder *json.Decoder, values map[string]string) error {
	token, err := decoder.Token()
	key, ok := token.(string)
	if err != nil || !ok || (key != "state" && key != "provenance") {
		return fmt.Errorf("scope protocol: unknown member")
	}
	if _, duplicate := values[key]; duplicate {
		return fmt.Errorf("scope protocol: duplicate member")
	}
	value, err := decoder.Token()
	text, ok := value.(string)
	if err != nil || !ok {
		return fmt.Errorf("scope protocol: expected string")
	}
	values[key] = text
	return nil
}

// Ping checks connectivity to FMC's API by calling ReadyzPath on the same
// base URL used for scope checks. It returns transport and status errors so
// the readiness gate (pkg/fleet/readiness.ScopeCheckerProber)
// needs the real transport/status error to correctly flip GW/RO's own
// /readyz to NotReady when FMC is unreachable or unready.
//
// DD-PLATFORM-010 (Issue #2169): Ping targets ReadyzPath, not ClustersPath.
// ClustersPath is a real, authenticated business-data endpoint; #1993/
// DD-AUTH-014 added TokenReview+SAR auth to the whole apiMux, so polling it
// purely for reachability paid a live, uncached authn/authz round-trip to
// kube-apiserver on every poll -- a cost DD-FLEET-004 never accounted for.
// ReadyzPath is registered as a second, unauthenticated top-level route on
// the same apiMux (cmd/fleetmetadatacache/main.go's buildFMCServers),
// outside the auth-enforcing group, reusing fmc.ReadyzHandler verbatim.
//
// Note this is a deliberate DEEP check, not the "shallow liveness" ClustersPath
// used to provide: ReadyzHandler pings FMC's own Valkey backend
// (deps.cacheReader), so a Valkey outage inside FMC now correctly cascades
// to GW/RO's fail-closed readiness gate (#1553/ADR-068) -- FMC cannot
// answer scope checks correctly without Valkey, so signaling NotReady to
// dependents is the right behavior. This mirrors DataStorage's own
// /readyz (pings Postgres+Redis) cascading to all 10 audit-writing
// services by design (BR-AUDIT-005 v2.0, DD-PLATFORM-010) -- see the
// DD-FLEET-004 amendment for the full rationale.
func (c *HTTPClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+ReadyzPath, nil)
	if err != nil {
		return fmt.Errorf("build FMC ping request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("FMC unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("FMC ping returned status %d", resp.StatusCode)
	}
	return nil
}
