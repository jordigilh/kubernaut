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
//	transport := auth.NewServiceAccountTransport()
//	httpClient := &http.Client{Transport: transport}
//	dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
// Integration Tests (Mock user header, no oauth-proxy):
//
//	// Use mocks.NewMockUserTransport() to avoid test logic in production code
//	transport := mocks.NewMockUserTransport("test-user@example.com")
//	httpClient := &http.Client{Transport: transport}
//	dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
// E2E Tests: Use SAME ServiceAccount transport as production (run tests in pods with mounted tokens)
//
// Authority: DD-AUTH-005 (Authoritative client authentication pattern)
// Related: DD-AUTH-004 (OAuth-proxy sidecar), DD-HAPI-003 (OpenAPI client mandatory)
// ========================================

// AuthTransport is an http.RoundTripper that handles authentication for DataStorage API calls.
//
// Behavior:
// - Reads token from a configurable filesystem path (default: /var/run/secrets/kubernetes.io/serviceaccount/token)
// - Used by: ALL services in production, E2E, and integration (with mounted tokens)
// - Injects: Authorization: Bearer <token>
// - Caching: 5-minute cache to avoid filesystem reads on every request
// - 401 cache invalidation: When downstream returns 401 Unauthorized, the token
//   cache is immediately invalidated so the next request re-reads from disk.
//   This handles kubelet SA token rotation (#1055).
// - Graceful degradation: If token file missing, request proceeds without auth
//
// Thread Safety:
// - RoundTrip() is thread-safe (clones request, cache uses sync.RWMutex)
// - invalidateTokenCache() is safe for concurrent use
//
// DD-AUTH-005: This transport enables all 7 Go services to authenticate with
// DataStorage without modifying the OpenAPI-generated client code.
//
// ZERO TEST LOGIC: This production code contains no test-specific modes.
// For integration tests (mock user headers), use internal/mocks.NewMockUserTransport().
type AuthTransport struct {
	base      http.RoundTripper
	tokenPath string

	// Token caching
	tokenCache      string
	tokenCacheTime  time.Time
	tokenCacheMutex sync.RWMutex

	// Observability: counts how many times the cache was invalidated due to 401.
	// Exposed via TokenInvalidationCount() for metrics/testing (SRE-M1).
	tokenInvalidationCount int64
}

// NewServiceAccountTransport creates a transport that reads tokens from the ServiceAccount filesystem.
// Used by services in E2E/Production environments.
//
// Token Path: /var/run/secrets/kubernetes.io/serviceaccount/token
// Caching: 5-minute cache to reduce filesystem reads
// Injects: Authorization: Bearer <token>
//
// Usage:
//
//	transport := auth.NewServiceAccountTransport()
//	httpClient := &http.Client{Transport: transport}
//	dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
// DD-AUTH-005: This is the PRIMARY transport for production and E2E services.
// All 7 Go services use this via audit.NewOpenAPIClientAdapter().
func NewServiceAccountTransport() *AuthTransport {
	return NewServiceAccountTransportWithBase(http.DefaultTransport)
}

// NewServiceAccountTransportWithBase creates a ServiceAccount transport with custom base transport.
// Useful for testing or custom transport configuration (e.g., custom timeouts, TLS).
func NewServiceAccountTransportWithBase(base http.RoundTripper) *AuthTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &AuthTransport{
		base:      base,
		tokenPath: "/var/run/secrets/kubernetes.io/serviceaccount/token",
	}
}

// NewServiceAccountTransportWithPath creates a ServiceAccount transport that reads tokens
// from a custom filesystem path. Used by kubernaut-agent where the SA token path is
// configurable via config.DataStorageConfig.SATokenPath (#1055).
//
// The transport caches the token for 5 minutes and invalidates the cache on 401 responses,
// enabling automatic recovery when kubelet rotates the projected SA token.
func NewServiceAccountTransportWithPath(tokenPath string, base http.RoundTripper) *AuthTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &AuthTransport{
		base:      base,
		tokenPath: tokenPath,
	}
}

// RoundTrip implements http.RoundTripper.
// Injects authentication headers based on the transport mode before forwarding the request.
//
// Thread Safety: Safe for concurrent use (clones request to avoid mutation).
//
// DD-AUTH-005: This method is called automatically by the http.Client for every request.
// Services using the OpenAPI-generated client don't need to know about authentication.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid mutating original
	// CRITICAL: http.RoundTripper must NOT modify the original request
	reqClone := req.Clone(req.Context())

	// Read token from filesystem (with 5-minute caching)
	token := t.getServiceAccountToken()
	if token != "" {
		reqClone.Header.Set("Authorization", "Bearer "+token)
	}
	// Note: If token file doesn't exist, request proceeds without auth
	// This allows services to start before ServiceAccount token is mounted
	// Also allows local development without Kubernetes

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
		t.invalidateTokenCache()
	}
	return resp, err
}

// getServiceAccountToken retrieves the ServiceAccount token with 5-minute caching.
// Reduces filesystem reads from every request to once per 5 minutes.
//
// Thread Safety: Uses sync.RWMutex for concurrent access.
//
// Cache Strategy:
// 1. Check cache with read lock (fast path for cache hits)
// 2. If cache miss or expired, acquire write lock
// 3. Double-check cache after acquiring write lock (avoid race)
// 4. Read token from filesystem
// 5. Update cache
//
// DD-AUTH-005: 5-minute cache balances performance (reduced filesystem I/O)
// with security (token rotation every 5 minutes).
func (t *AuthTransport) getServiceAccountToken() string {
	// Fast path: Check cache with read lock
	t.tokenCacheMutex.RLock()
	if time.Since(t.tokenCacheTime) < 5*time.Minute && t.tokenCache != "" {
		cached := t.tokenCache
		t.tokenCacheMutex.RUnlock()
		return cached
	}
	t.tokenCacheMutex.RUnlock()

	// Slow path: Cache miss or expired - read from filesystem with write lock
	t.tokenCacheMutex.Lock()
	defer t.tokenCacheMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine may have updated cache)
	if time.Since(t.tokenCacheTime) < 5*time.Minute && t.tokenCache != "" {
		return t.tokenCache
	}

	// Read token from filesystem
	tokenBytes, err := os.ReadFile(t.tokenPath)
	if err != nil {
		// Token file doesn't exist or read error
		// Common during local development or before ServiceAccount is mounted
		// Return empty string - request proceeds without auth header
		return ""
	}

	// Update cache
	t.tokenCache = string(tokenBytes)
	t.tokenCacheTime = time.Now()
	return t.tokenCache
}

// invalidateTokenCache zeroes the cache timestamp so the next call to
// getServiceAccountToken takes the slow path and re-reads from disk.
// Called when a downstream service returns 401 Unauthorized (#1055).
//
// Thread Safety: Uses exclusive Lock (not RLock). Safe to call concurrently
// from multiple goroutines — redundant invalidations are benign.
func (t *AuthTransport) invalidateTokenCache() {
	t.tokenCacheMutex.Lock()
	defer t.tokenCacheMutex.Unlock()
	t.tokenCacheTime = time.Time{}
	atomic.AddInt64(&t.tokenInvalidationCount, 1)
}

// TokenInvalidationCount returns the number of times the token cache was
// invalidated due to a 401 response. Useful for Prometheus metrics and testing.
func (t *AuthTransport) TokenInvalidationCount() int64 {
	return atomic.LoadInt64(&t.tokenInvalidationCount)
}
