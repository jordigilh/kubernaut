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
// E2E TEST-ONLY STATIC TOKEN TRANSPORT
// DD-AUTH-005: DataStorage Client Authentication Pattern
// ========================================
//
// StaticTokenTransport is a test-only http.RoundTripper that injects
// static tokens (acquired externally) for E2E tests that run outside Kubernetes.
//
// WHY SEPARATE FROM PRODUCTION CODE?
// - ❌ Test logic must NEVER be in production/business code
// - ✅ Production binaries should contain ZERO test-specific code
// - ✅ Clear separation: pkg/shared/auth = production, test/shared = test
//
// USAGE (E2E Tests Only - External Test Runners):
//
//   // Acquire token externally (ServiceAccount or kubeadmin)
//   token := getServiceAccountToken("datastorage-e2e-sa", "default", 3600)
//   // OR: token := exec.Command("kubectl", "whoami", "-t").Output()
//
//   // Inject token into E2E test client
//   transport := testutil.NewStaticTokenTransport(token)
//   dsClient := audit.NewOpenAPIClientAdapterWithTransport(nodePortURL, timeout, transport)
//
// E2E Test Scenario:
// - E2E tests: Run as Ginkgo tests on host machine (NOT in pods)
// - Target: Kind cluster NodePort → DataStorage pod (with oauth-proxy sidecar)
// - No mounted ServiceAccount token available (tests run externally)
// - Solution: Acquire token via TokenRequest API or kubectl, inject via this transport
//
// Authority: DD-AUTH-005 (Authoritative client authentication pattern)
// Related: test/shared/auth_mock.go (integration test mock transport)
// ========================================

// StaticTokenTransport implements http.RoundTripper for E2E tests.
// Injects static token (acquired externally) without token refresh.
//
// Thread Safety: Safe for concurrent use (clones request to avoid mutation).
type StaticTokenTransport struct {
	base  http.RoundTripper
	token string
}

// NewStaticTokenTransport creates a test-only transport that injects static tokens.
//
// Used by: E2E tests that run externally (outside Kubernetes, no mounted tokens)
// Injects: Authorization: Bearer <token>
// Token source: TokenRequest API or kubectl whoami -t
//
// Example (ServiceAccount token via TokenRequest API):
//
//	token := getServiceAccountToken("datastorage-e2e-sa", "default", 3600)
//	transport := testutil.NewStaticTokenTransport(token)
//	httpClient := &http.Client{Transport: transport}
//	dsClient := datastorage.NewClientWithResponses(nodePortURL, datastorage.WithHTTPClient(httpClient))
//
//	// All DataStorage API calls will have Authorization: Bearer <token>
//	resp, err := dsClient.CreateAuditEvent(ctx, req)
//
// Example (Kubeadmin token via kubectl):
//
//	output, _ := exec.Command("kubectl", "whoami", "-t").Output()
//	token := strings.TrimSpace(string(output))
//	transport := testutil.NewStaticTokenTransport(token)
//	// ... same as above
//
// DD-AUTH-005: This is the ONLY way E2E tests (running externally) should authenticate.
func NewStaticTokenTransport(token string) *StaticTokenTransport {
	return NewStaticTokenTransportWithBase(token, http.DefaultTransport)
}

// NewStaticTokenTransportWithBase creates a static token transport with custom base transport.
// Useful for custom transport configuration (e.g., timeouts, TLS).
func NewStaticTokenTransportWithBase(token string, base http.RoundTripper) *StaticTokenTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &StaticTokenTransport{
		base:  base,
		token: token,
	}
}

// RoundTrip implements http.RoundTripper.
// Injects Authorization: Bearer <token> header for E2E tests.
//
// Thread Safety: Safe for concurrent use (clones request to avoid mutation).
func (t *StaticTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid mutating original
	// CRITICAL: http.RoundTripper must NOT modify the original request
	reqClone := req.Clone(req.Context())

	// Inject Authorization header (static token from E2E test setup)
	if t.token != "" {
		reqClone.Header.Set("Authorization", "Bearer "+t.token)
	}

	return t.base.RoundTrip(reqClone)
}
