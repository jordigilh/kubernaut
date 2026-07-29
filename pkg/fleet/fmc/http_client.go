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
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// Issue #54 fleet E2E RCA (CI run 30464667745): FMC's handler distinguishes
// a failed check (503 -- e.g. Valkey "context canceled" under resource
// pressure) from a completed check that determined "not managed" (200).
// IsManagedResource retries only the former: a small, bounded number of
// attempts with exponential backoff, wrapped in a hard wall-clock ceiling.
// This sits on Gateway/RO's synchronous alert-ingestion path, so the ceiling
// is deliberately short (seconds, not the 45s a test's own outer retry
// loop might use) -- a transient blip gets a couple of quick retries, but a
// real outage still fails safe (managed=false, SC-7) promptly rather than
// stalling the caller.
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
// to Valkey directly. All failures are fail-safe (return unmanaged).
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

// IsManagedResource checks whether a resource is in-scope by calling FMC's
// /api/v1/scope/check endpoint. A transient failure (5xx / transport error)
// is retried a bounded number of times with backoff (see scopeCheckMaxAttempts,
// scopeCheckRetryCeiling); a definitive response (200, whether managed true
// or false) is never retried. All failures ultimately return false, nil
// (fail-safe per SC-7) -- retries improve resilience to short blips, they
// never change the fail-closed guarantee on exhaustion.
func (c *HTTPClient) IsManagedResource(ctx context.Context, r scope.ResourceIdentity) (bool, error) {
	reqURL := c.buildScopeCheckURL(r)

	ctx, cancel := context.WithTimeout(ctx, scopeCheckRetryCeiling)
	defer cancel()

	for attempt := int32(1); attempt <= scopeCheckMaxAttempts; attempt++ {
		managed, final, retryable := c.attemptScopeCheck(ctx, reqURL)
		if final {
			return managed, nil
		}
		if !retryable || attempt == scopeCheckMaxAttempts {
			return false, nil
		}

		select {
		case <-ctx.Done():
			return false, nil
		case <-time.After(scopeCheckBackoff.Calculate(attempt)):
		}
	}
	return false, nil
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

// attemptScopeCheck performs a single scope-check HTTP round-trip.
//
// final=true means the response is a definitive answer (200 OK, decoded
// successfully) that must not be retried, regardless of the managed value.
// retryable=true means the failure looks transient (transport error, or a
// 5xx indicating FMC's own check failed rather than completed) and is worth
// a bounded retry. A non-retryable failure (e.g. 4xx, or a malformed 200
// body) ends the attempt loop immediately, fail-safe.
func (c *HTTPClient) attemptScopeCheck(ctx context.Context, reqURL string) (managed, final, retryable bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, false, false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, false, true // transport error: transient, worth a retry
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusInternalServerError {
		return false, false, true // 5xx: FMC's check failed (e.g. transient backend error), worth a retry
	}
	if resp.StatusCode != http.StatusOK {
		return false, false, false // e.g. 4xx: caller-side problem, not transient -- fail fast
	}

	var result ScopeCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, false, false // malformed 200 body: not transient -- fail fast
	}

	return result.Managed, true, false
}

// Ping checks connectivity to FMC's API by calling ClustersPath on the same
// base URL used for scope checks. Unlike IsManagedResource, Ping does NOT
// swallow errors: the readiness gate (pkg/fleet/readiness.ScopeCheckerProber)
// needs the real transport/status error to correctly flip /readyz to
// NotReady when FMC is unreachable.
//
// DD-FLEET-004: Ping deliberately targets ClustersPath, not HealthzPath.
// FMC's liveness endpoint (/healthz) is kubelet-only, served exclusively on
// the dedicated health port (Issue #1683 3-port split) -- it is not, and
// must not become, reachable from other pods (no NetworkPolicy ingress rule
// permits it). ClustersPath already is: it's a real, already-registered API
// endpoint that only reads FMC's in-memory cluster registry (no Valkey
// round-trip), giving the same "shallow liveness" signal HealthzPath would
// have, without duplicating a liveness handler onto the API mux.
func (c *HTTPClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+ClustersPath, nil)
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
