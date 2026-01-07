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
//   transport := auth.NewServiceAccountTransport()
//   httpClient := &http.Client{Transport: transport}
//   dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
// E2E Tests (Static token via TokenRequest API):
//   token := getServiceAccountToken("test-sa", "namespace", 3600)
//   transport := auth.NewStaticTokenTransport(token)
//   httpClient := &http.Client{Transport: transport}
//   dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
// Integration Tests (Mock user header, no oauth-proxy):
//   // Use testutil.NewMockUserTransport() to avoid test logic in production code
//   transport := testutil.NewMockUserTransport("test-user@example.com")
//   httpClient := &http.Client{Transport: transport}
//   dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
// Authority: DD-AUTH-005 (Authoritative client authentication pattern)
// Related: DD-AUTH-004 (OAuth-proxy sidecar), DD-HAPI-003 (OpenAPI client mandatory)
// ========================================

// AuthTransportMode defines how the transport handles authentication
type AuthTransportMode int

const (
	// ModeServiceAccount reads token from filesystem (for services and E2E)
	// Token path: /var/run/secrets/kubernetes.io/serviceaccount/token
	// Injects: Authorization: Bearer <token>
	ModeServiceAccount AuthTransportMode = iota

	// ModeStaticToken uses a provided static token (for E2E tests with TokenRequest)
	// Token: Provided via NewStaticTokenTransport(token)
	// Injects: Authorization: Bearer <token>
	ModeStaticToken
)

// AuthTransport is an http.RoundTripper that handles authentication for DataStorage API calls.
//
// It supports two modes:
// 1. ModeServiceAccount: Reads token from /var/run/secrets/kubernetes.io/serviceaccount/token
//    - Used by: Services in E2E/Production
//    - Injects: Authorization: Bearer <token>
//    - Caching: 5-minute cache to avoid filesystem reads on every request
//
// 2. ModeStaticToken: Uses provided token
//    - Used by: E2E tests (via TokenRequest API)
//    - Injects: Authorization: Bearer <token>
//    - Caching: No caching (token provided once)
//
// Thread Safety:
// - RoundTrip() is thread-safe (clones request, no shared state mutation)
// - Token caching uses sync.RWMutex for concurrent access
//
// DD-AUTH-005: This transport enables all 7 Go services to authenticate with
// DataStorage without modifying the OpenAPI-generated client code.
//
// NOTE: For integration tests (mock user headers), use pkg/testutil.NewMockUserTransport()
// to avoid test logic in production code.
type AuthTransport struct {
	base      http.RoundTripper
	mode      AuthTransportMode
	tokenPath string

	// For ModeStaticToken
	staticToken string

	// Token caching (for ModeServiceAccount)
	tokenCache      string
	tokenCacheTime  time.Time
	tokenCacheMutex sync.RWMutex
}

// NewServiceAccountTransport creates a transport that reads tokens from the ServiceAccount filesystem.
// Used by services in E2E/Production environments.
//
// Token Path: /var/run/secrets/kubernetes.io/serviceaccount/token
// Caching: 5-minute cache to reduce filesystem reads
// Injects: Authorization: Bearer <token>
//
// Usage:
//   transport := auth.NewServiceAccountTransport()
//   httpClient := &http.Client{Transport: transport}
//   dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
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
		mode:      ModeServiceAccount,
		tokenPath: "/var/run/secrets/kubernetes.io/serviceaccount/token",
	}
}

// NewStaticTokenTransport creates a transport that uses a provided static token.
// Used by E2E tests with TokenRequest API.
//
// Token: Provided by caller (via Kubernetes TokenRequest API)
// Injects: Authorization: Bearer <token>
//
// Usage:
//   token := getServiceAccountToken("test-sa", "namespace", 3600)
//   transport := auth.NewStaticTokenTransport(token)
//   httpClient := &http.Client{Transport: transport}
//   dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
// DD-AUTH-005: This is the transport for E2E tests that need real oauth-proxy validation.
func NewStaticTokenTransport(token string) *AuthTransport {
	return NewStaticTokenTransportWithBase(token, http.DefaultTransport)
}

// NewStaticTokenTransportWithBase creates a static token transport with custom base transport.
func NewStaticTokenTransportWithBase(token string, base http.RoundTripper) *AuthTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &AuthTransport{
		base:        base,
		mode:        ModeStaticToken,
		staticToken: token,
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

	switch t.mode {
	case ModeServiceAccount:
		// Read token from filesystem (with 5-minute caching)
		token := t.getServiceAccountToken()
		if token != "" {
			reqClone.Header.Set("Authorization", "Bearer "+token)
		}
		// Note: If token file doesn't exist, request proceeds without auth
		// This allows services to start before ServiceAccount token is mounted

	case ModeStaticToken:
		// Use provided static token (E2E tests)
		if t.staticToken != "" {
			reqClone.Header.Set("Authorization", "Bearer "+t.staticToken)
		}
	}

	return t.base.RoundTrip(reqClone)
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

