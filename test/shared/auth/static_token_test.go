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

package auth_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/shared/auth"
)

func TestAuthStaticToken(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthStaticToken Suite")
}

var _ = Describe("StaticTokenTransport", func() {
	var (
		server *httptest.Server
	)

	BeforeEach(func() {
		// Create test server that echoes Authorization header
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Echo Authorization header for validation
			if auth := r.Header.Get("Authorization"); auth != "" {
				w.Header().Set("X-Echo-Authorization", auth)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}))
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("NewStaticTokenTransport", func() {
		It("should inject Authorization Bearer header with ServiceAccount token", func() {
			// Simulate ServiceAccount token format
			saToken := "eyJhbGciOiJSUzI1NiIsImtpZCI6InRlc3Qta2V5In0.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJkZWZhdWx0Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZWNyZXQubmFtZSI6ImRhdGFzdG9yYWdlLWUyZS1zYS10b2tlbiIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50Lm5hbWUiOiJkYXRhc3RvcmFnZS1lMmUtc2EiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiIxMjM0NTY3OC05MDEyLTM0NTYtNzg5MC0xMjM0NTY3ODkwMTIiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6ZGVmYXVsdDpkYXRhc3RvcmFnZS1lMmUtc2EifQ.signature"

			// Create static token transport
			transport := auth.NewStaticTokenTransport(saToken)
			client := &http.Client{Transport: transport}

			// Make request
			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify Authorization header was injected
			Expect(resp.Header.Get("X-Echo-Authorization")).To(Equal("Bearer " + saToken))
		})

		It("should inject Authorization Bearer header with kubeadmin token", func() {
			// Simulate kubeadmin token (from kubectl whoami -t)
			kubeadminToken := "sha256~AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"

			// Create static token transport
			transport := auth.NewStaticTokenTransport(kubeadminToken)
			client := &http.Client{Transport: transport}

			// Make request
			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify Authorization header was injected
			Expect(resp.Header.Get("X-Echo-Authorization")).To(Equal("Bearer " + kubeadminToken))
		})

		It("should not inject header if token is empty", func() {
			// Create transport with empty token
			transport := auth.NewStaticTokenTransport("")
			client := &http.Client{Transport: transport}

			// Make request
			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify no headers injected
			Expect(resp.Header.Get("X-Echo-Authorization")).To(BeEmpty())
		})
	})

	Describe("ServiceAccount Token Format Validation", func() {
		It("should accept valid ServiceAccount JWT token format", func() {
			// Valid ServiceAccount JWT token (header.payload.signature)
			validSAToken := "eyJhbGciOiJSUzI1NiIsImtpZCI6InRlc3QifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50OmRlZmF1bHQ6ZGF0YXN0b3JhZ2UtZTJlLXNhIn0.signature"

			transport := auth.NewStaticTokenTransport(validSAToken)
			client := &http.Client{Transport: transport}

			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify token was accepted and injected
			Expect(resp.Header.Get("X-Echo-Authorization")).To(ContainSubstring("Bearer "))
			Expect(resp.Header.Get("X-Echo-Authorization")).To(ContainSubstring(validSAToken))
		})

		It("should accept valid kubeconfig token format (sha256~)", func() {
			// Valid kubeconfig token format
			validKubeconfigToken := "sha256~VeryLongBase64EncodedTokenHere1234567890"

			transport := auth.NewStaticTokenTransport(validKubeconfigToken)
			client := &http.Client{Transport: transport}

			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify token was accepted and injected
			Expect(resp.Header.Get("X-Echo-Authorization")).To(Equal("Bearer " + validKubeconfigToken))
		})

		// NOTE: We don't validate token format in the transport - that's oauth-proxy's job
		// This transport simply injects whatever token is provided
		It("should inject any token format (validation is oauth-proxy's responsibility)", func() {
			// Invalid token format (for demonstration - transport doesn't validate)
			invalidToken := "this-is-not-a-valid-token-but-transport-accepts-it"

			transport := auth.NewStaticTokenTransport(invalidToken)
			client := &http.Client{Transport: transport}

			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Transport injects it (oauth-proxy will reject it later)
			Expect(resp.Header.Get("X-Echo-Authorization")).To(Equal("Bearer " + invalidToken))
		})
	})

	Describe("Request cloning", func() {
		It("should not mutate original request", func() {
			// Create transport
			transport := auth.NewStaticTokenTransport("test-token")
			client := &http.Client{Transport: transport}

			// Create request
			req, err := http.NewRequest("GET", server.URL, nil)
			Expect(err).ToNot(HaveOccurred())

			// Store original headers
			originalHeaders := req.Header.Clone()

			// Make request
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify original request headers were NOT mutated
			Expect(req.Header).To(Equal(originalHeaders))
			Expect(req.Header.Get("Authorization")).To(BeEmpty())
		})
	})

	Describe("WithBase transport", func() {
		It("should use custom base transport", func() {
			// Create custom base transport
			customBase := &testRoundTripper{
				base: http.DefaultTransport,
			}

			// Create static token transport with custom base
			transport := auth.NewStaticTokenTransportWithBase("test-token", customBase)
			client := &http.Client{Transport: transport}

			// Make request
			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify both auth header and custom base were used
			Expect(resp.Header.Get("X-Echo-Authorization")).To(Equal("Bearer test-token"))
			Expect(customBase.called).To(BeTrue())
		})

		It("should use http.DefaultTransport if base is nil", func() {
			// Create transport with nil base
			transport := auth.NewStaticTokenTransportWithBase("test-token", nil)
			client := &http.Client{Transport: transport}

			// Make request (should not panic)
			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify request succeeded
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).To(Equal("OK"))
		})
	})
})

// testRoundTripper is a custom transport for testing custom base transport
type testRoundTripper struct {
	base   http.RoundTripper
	called bool
}

func (t *testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.called = true
	return t.base.RoundTrip(req)
}
