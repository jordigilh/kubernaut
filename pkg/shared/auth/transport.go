/*
Copyright 2025 Jordi Gil.

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

package auth

import (
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
)

// ========================================
// DD-AUTH-005: DataStorage Client Authentication Pattern
// 📋 Design Decision: DD-AUTH-005 | ✅ Approved Design | Confidence: 95%
// See: docs/architecture/decisions/DD-AUTH-005-datastorage-client-authentication-pattern.md
// ========================================
//
// AuthTransport implements http.RoundTripper to inject authentication headers
// for DataStorage REST API calls across ALL environments (integration/E2E/production).
//
// WHY DD-AUTH-005?
// - ✅ OpenAPI client compliance: Generated clients remain pristine (DD-HAPI-003)
// - ✅ Environment-aware: Different auth modes for integration/E2E/production
// - ✅ Zero service changes: All 7 Go services get auth by updating audit adapter once
// - ✅ Transparent: Services use OpenAPI clients normally, transport handles auth
// - ✅ Scalable: Works for ALL DataStorage endpoints automatically
//
// USAGE PATTERNS:
//
// Production/E2E (ServiceAccount token from filesystem):
//
//	ts := auth.NewDefaultTokenSource()
//	transport := auth.NewAuthTransport(ts, http.DefaultTransport)
//	httpClient := &http.Client{Transport: transport}
//	dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
// Shared cache (kubernaut-agent: DS client + audit store share one cache):
//
//	ts := auth.NewTokenSource(cfg.SATokenPath)
//	dsTransport := auth.NewAuthTransport(ts, dsBase)
//	auditTransport := auth.NewAuthTransport(ts, auditBase)
//	// A 401 on either transport invalidates the cache for both.
//
// Integration Tests (Mock user header, no oauth-proxy):
//
//	// Use mocks.NewMockUserTransport() to avoid test logic in production code
//	transport := mocks.NewMockUserTransport("test-user@example.com")
//	httpClient := &http.Client{Transport: transport}
//	dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
// E2E Tests: Use SAME transport as production (run tests in pods with mounted tokens)
//
// Authority: DD-AUTH-005 (Authoritative client authentication pattern)
// Related: DD-AUTH-004 (OAuth-proxy sidecar), DD-HAPI-003 (OpenAPI client mandatory)
// ========================================

const (
	defaultTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	tokenCacheTTL    = 5 * time.Minute
)

// TokenSource owns the SA token file path and a thread-safe in-memory cache.
// Multiple AuthTransport instances can share a single TokenSource so that a
// 401-triggered cache invalidation on one transport immediately benefits all
// others reading from the same projected volume.
//
// Cache strategy:
//  1. Check cache with read lock (fast path for cache hits)
//  2. If cache miss or expired, acquire write lock
//  3. Double-check after acquiring write lock (avoid thundering herd)
//  4. Read token from filesystem
//  5. Update cache
//
// Thread Safety: All methods are safe for concurrent use.
type TokenSource struct {
	tokenPath       string
	tokenCache      string
	tokenCacheTime  time.Time
	tokenCacheMutex sync.RWMutex

	// Writes use atomic.AddInt64 (under tokenCacheMutex for cache consistency).
	// Reads use atomic.LoadInt64 (no mutex needed — atomics are self-synchronizing).
	tokenInvalidationCount int64

	logger         logr.Logger
	lastReadFailed bool
}

// NewTokenSource creates a TokenSource that reads SA tokens from the given path.
// Use NewDefaultTokenSource for the standard Kubernetes projected volume path.
func NewTokenSource(path string) *TokenSource {
	return &TokenSource{tokenPath: path, logger: logr.Discard()}
}

// SetLogger configures a logger for token lifecycle events (cache invalidation,
// file read failures, recovery). By default, TokenSource uses logr.Discard().
// Callers in cmd/ can wire a real logger for SRE observability.
func (ts *TokenSource) SetLogger(l logr.Logger) {
	ts.logger = l
}

// NewDefaultTokenSource creates a TokenSource for the standard Kubernetes
// projected volume at /var/run/secrets/kubernetes.io/serviceaccount/token.
func NewDefaultTokenSource() *TokenSource {
	return NewTokenSource(defaultTokenPath)
}

// Token returns the cached SA token, re-reading from disk when the cache has
// expired (5-minute TTL) or been invalidated.
func (ts *TokenSource) Token() string {
	ts.tokenCacheMutex.RLock()
	if time.Since(ts.tokenCacheTime) < tokenCacheTTL && ts.tokenCache != "" {
		cached := ts.tokenCache
		ts.tokenCacheMutex.RUnlock()
		return cached
	}
	ts.tokenCacheMutex.RUnlock()

	ts.tokenCacheMutex.Lock()
	defer ts.tokenCacheMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed)
	if time.Since(ts.tokenCacheTime) < tokenCacheTTL && ts.tokenCache != "" {
		return ts.tokenCache
	}

	tokenBytes, err := os.ReadFile(ts.tokenPath)
	if err != nil {
		ts.lastReadFailed = true
		ts.logger.V(1).Info("token file read failed, requests will proceed without auth",
			"path", ts.tokenPath, "error", err)
		return ""
	}

	if ts.lastReadFailed {
		ts.logger.Info("token file recovered, auth resumed",
			"path", ts.tokenPath)
		ts.lastReadFailed = false
	}

	ts.tokenCache = string(tokenBytes)
	ts.tokenCacheTime = time.Now()
	return ts.tokenCache
}

// Invalidate zeroes the cache timestamp so the next Token() call re-reads from
// disk. Called by AuthTransport when a downstream 401 indicates credential expiry.
//
// Safe to call concurrently — redundant invalidations are benign.
func (ts *TokenSource) Invalidate() {
	ts.tokenCacheMutex.Lock()
	defer ts.tokenCacheMutex.Unlock()
	ts.tokenCacheTime = time.Time{}
	count := atomic.AddInt64(&ts.tokenInvalidationCount, 1)
	ts.logger.V(1).Info("token cache invalidated due to 401 response",
		"path", ts.tokenPath, "invalidation_count", count)
}

// InvalidationCount returns the number of times the cache was invalidated
// due to a 401 response. Useful for Prometheus metrics and testing.
func (ts *TokenSource) InvalidationCount() int64 {
	return atomic.LoadInt64(&ts.tokenInvalidationCount)
}

// AuthTransport is an http.RoundTripper that injects a Bearer token from a
// shared TokenSource into every outgoing request.
//
// Behavior:
//   - Reads token via TokenSource (5-minute cache + 401 invalidation)
//   - Injects: Authorization: Bearer <token>
//   - 401 cache invalidation: When downstream returns 401 Unauthorized, the
//     TokenSource cache is immediately invalidated so the next request (from
//     this or any other transport sharing the same TokenSource) re-reads from disk.
//   - Graceful degradation: If token file missing, request proceeds without auth
//
// Thread Safety: RoundTrip() is thread-safe (clones request, TokenSource uses sync.RWMutex).
//
// DD-AUTH-005: This transport enables all 7 Go services to authenticate with
// DataStorage without modifying the OpenAPI-generated client code.
//
// ZERO TEST LOGIC: This production code contains no test-specific modes.
// For integration tests (mock user headers), use internal/mocks.NewMockUserTransport().
type AuthTransport struct {
	base        http.RoundTripper
	tokenSource *TokenSource
}

// NewAuthTransport creates an AuthTransport that reads tokens from the given
// TokenSource and delegates HTTP calls to the given base RoundTripper.
// If base is nil, http.DefaultTransport is used.
func NewAuthTransport(ts *TokenSource, base http.RoundTripper) *AuthTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &AuthTransport{
		base:        base,
		tokenSource: ts,
	}
}

// RoundTrip implements http.RoundTripper.
// Injects the Bearer token from the shared TokenSource before forwarding.
//
// Thread Safety: Safe for concurrent use (clones request to avoid mutation).
//
// DD-AUTH-005: Called automatically by http.Client for every request.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())

	token := t.tokenSource.Token()
	if token != "" {
		reqClone.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := t.base.RoundTrip(reqClone)
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		// #1055: Only 401 triggers cache invalidation, NOT 403. Rationale:
		// - 401 (Unauthorized) = credential expired/invalid → re-reading the
		//   rotated token from disk will fix it.
		// - 403 (Forbidden) = valid credential but insufficient permissions →
		//   re-reading the same token won't help; the issue is RBAC, not expiry.
		// The audit layer (pkg/audit/errors.go) retries both 401 and 403 because
		// RBAC propagation delays can cause transient 403s, but the transport
		// only refreshes credentials on 401.
		t.tokenSource.Invalidate()
	}
	return resp, err
}

// TokenInvalidationCount delegates to the underlying TokenSource for backward
// compatibility with tests that assert on the transport directly.
func (t *AuthTransport) TokenInvalidationCount() int64 {
	return t.tokenSource.InvalidationCount()
}
