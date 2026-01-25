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
)

// ========================================
// TEST-ONLY AUTHENTICATION TRANSPORT
// DD-AUTH-005: DataStorage Client Authentication Pattern
// ========================================
//
// MockUserTransport is a test-only http.RoundTripper that directly injects
// X-Auth-Request-User headers to simulate oauth-proxy behavior in integration tests.
//
// WHY SEPARATE FROM PRODUCTION CODE?
// - ❌ Test logic must NEVER be in production/business code
// - ✅ Production binaries should contain ZERO test-specific code
// - ✅ Clear separation: pkg/shared/auth = production, test/shared = test
//
// USAGE (Integration Tests Only):
//
//   transport := mocks.NewMockUserTransport("test-user@example.com")
//   httpClient := &http.Client{Transport: transport}
//   dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
// What This Simulates:
// - In production/E2E: oauth-proxy validates JWT token, performs SAR, injects X-Auth-Request-User
// - In integration tests: No oauth-proxy running, we inject header directly
//
// Authority: DD-AUTH-005 (Authoritative client authentication pattern)
// Related: pkg/shared/auth/transport.go (production transports)
// ========================================

// MockUserTransport implements http.RoundTripper for integration tests.
// Directly injects X-Auth-Request-User header without token validation.
//
// Thread Safety: Safe for concurrent use (clones request to avoid mutation).
type MockUserTransport struct {
	base       http.RoundTripper
	mockUserID string
}

// NewMockUserTransport creates a test-only transport that injects X-Auth-Request-User header.
//
// Used by: Integration tests (no oauth-proxy, no token validation)
// Injects: X-Auth-Request-User: <mockUserID>
// Simulates: What oauth-proxy would inject after JWT validation + SAR
//
// Example:
//
//	transport := mocks.NewMockUserTransport("test-operator@example.com")
//	httpClient := &http.Client{Transport: transport}
//	dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
//
//	// All DataStorage API calls will have X-Auth-Request-User: test-operator@example.com
//	resp, err := dsClient.PlaceLegalHoldWithResponse(ctx, req)
//
// DD-AUTH-005: This is the ONLY way integration tests should authenticate with DataStorage.
func NewMockUserTransport(userID string) *MockUserTransport {
	return NewMockUserTransportWithBase(userID, http.DefaultTransport)
}

// NewMockUserTransportWithBase creates a mock user transport with custom base transport.
// Useful for testing or custom transport configuration.
func NewMockUserTransportWithBase(userID string, base http.RoundTripper) *MockUserTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &MockUserTransport{
		base:       base,
		mockUserID: userID,
	}
}

// RoundTrip implements http.RoundTripper.
// Injects X-Auth-Request-User header to simulate oauth-proxy behavior.
//
// Thread Safety: Safe for concurrent use (clones request to avoid mutation).
func (t *MockUserTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid mutating original
	// CRITICAL: http.RoundTripper must NOT modify the original request
	reqClone := req.Clone(req.Context())

	// Inject X-Auth-Request-User header (simulates oauth-proxy)
	if t.mockUserID != "" {
		reqClone.Header.Set("X-Auth-Request-User", t.mockUserID)
	}

	return t.base.RoundTrip(reqClone)
}
