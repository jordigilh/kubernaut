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
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAuth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Auth Transport Suite")
}

var _ = Describe("AuthTransport", func() {
	var (
		server *httptest.Server
	)

	BeforeEach(func() {
		// Create test HTTP server that echoes request headers
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Echo Authorization header
			if auth := r.Header.Get("Authorization"); auth != "" {
				w.Header().Set("X-Echo-Authorization", auth)
			}
			// Echo X-Auth-Request-User header
			if user := r.Header.Get("X-Auth-Request-User"); user != "" {
				w.Header().Set("X-Echo-User", user)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}))
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("testauth.NewMockUserTransport", func() {
		It("should inject X-Auth-Request-User header", func() {
			// Create mock user transport (from testutil, not production code)
			transport := testauth.NewMockUserTransport("test-user@example.com")
			client := &http.Client{Transport: transport}

			// Make request
			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify X-Auth-Request-User header was injected
			Expect(resp.Header.Get("X-Echo-User")).To(Equal("test-user@example.com"))
			Expect(resp.Header.Get("X-Echo-Authorization")).To(BeEmpty())
		})

		It("should not inject header if userID is empty", func() {
			// Create transport with empty userID
			transport := testauth.NewMockUserTransport("")
			client := &http.Client{Transport: transport}

			// Make request
			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify no headers injected
			Expect(resp.Header.Get("X-Echo-User")).To(BeEmpty())
			Expect(resp.Header.Get("X-Echo-Authorization")).To(BeEmpty())
		})
	})

	Describe("NewServiceAccountTransport", func() {
		// NOTE: "should read token from filesystem" and "should cache token for
		// 5 minutes" are tested in integration tests with a real filesystem/token.
		// They do not belong at the unit test tier.

		It("should not inject header if token file doesn't exist", func() {
			// Create ServiceAccount transport pointing to non-existent file
			transport := auth.NewServiceAccountTransportWithBase(http.DefaultTransport)
			client := &http.Client{Transport: transport}

			// Make request (token file doesn't exist)
			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify no headers injected (graceful degradation)
			Expect(resp.Header.Get("X-Echo-Authorization")).To(BeEmpty())
			Expect(resp.Header.Get("X-Echo-User")).To(BeEmpty())
		})
	})

	Describe("RoundTrip thread safety", func() {
		It("should be safe for concurrent use", func() {
			// Create transport (using testutil mock)
			transport := testauth.NewMockUserTransport("concurrent-user@example.com")
			client := &http.Client{Transport: transport}

			// Make 100 concurrent requests
			const numRequests = 100
			errors := make(chan error, numRequests)
			responses := make(chan *http.Response, numRequests)

			for i := 0; i < numRequests; i++ {
				go func() {
					resp, err := client.Get(server.URL)
					if err != nil {
						errors <- err
						return
					}
					responses <- resp
				}()
			}

			// Collect results
			for i := 0; i < numRequests; i++ {
				select {
				case err := <-errors:
					Fail("Unexpected error: " + err.Error())
				case resp := <-responses:
					// Verify header was injected correctly
					Expect(resp.Header.Get("X-Echo-User")).To(Equal("concurrent-user@example.com"))
					_ = resp.Body.Close()
				case <-time.After(5 * time.Second):
					Fail("Timeout waiting for response")
				}
			}
		})
	})

	Describe("Request cloning", func() {
		It("should not mutate original request", func() {
			// Create transport (using testutil mock)
			transport := testauth.NewMockUserTransport("test-user@example.com")
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
			Expect(req.Header.Get("X-Auth-Request-User")).To(BeEmpty())
		})
	})

	Describe("WithBase transport", func() {
		It("should use custom base transport", func() {
			// Create custom base transport that adds a header
			customBase := &testRoundTripper{
				base: http.DefaultTransport,
			}

			// Create mock user transport with custom base (using testutil)
			transport := testauth.NewMockUserTransportWithBase("test-user@example.com", customBase)
			client := &http.Client{Transport: transport}

			// Make request
			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify both auth header and custom base were used
			Expect(resp.Header.Get("X-Echo-User")).To(Equal("test-user@example.com"))
			Expect(customBase.called).To(BeTrue())
		})

		It("should use http.DefaultTransport if base is nil", func() {
			// Create transport with nil base (using testutil)
			transport := testauth.NewMockUserTransportWithBase("test-user@example.com", nil)
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

// statusRoundTripper returns a configurable status code and tracks requests.
type statusRoundTripper struct {
	mu          sync.Mutex
	statusCode  int32
	requestLog  []string
	callCount   int32
}

func (s *statusRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(&s.callCount, 1)
	s.mu.Lock()
	s.requestLog = append(s.requestLog, req.Header.Get("Authorization"))
	s.mu.Unlock()
	code := atomic.LoadInt32(&s.statusCode)
	return &http.Response{
		StatusCode: int(code),
		Header:     http.Header{},
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

func (s *statusRoundTripper) setStatus(code int) {
	atomic.StoreInt32(&s.statusCode, int32(code))
}

func (s *statusRoundTripper) getAuthHeaders() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]string, len(s.requestLog))
	copy(cp, s.requestLog)
	return cp
}

var _ = Describe("NewServiceAccountTransportWithPath (#1055)", func() {
	var (
		tokenDir  string
		tokenFile string
	)

	BeforeEach(func() {
		var err error
		tokenDir, err = os.MkdirTemp("", "auth-transport-test-*")
		Expect(err).ToNot(HaveOccurred())
		tokenFile = filepath.Join(tokenDir, "token")
	})

	AfterEach(func() {
		_ = os.RemoveAll(tokenDir)
	})

	// UT-AT-1055-001: Custom path constructor reads from specified path
	It("UT-AT-1055-001: should read token from the specified custom path", func() {
		Expect(os.WriteFile(tokenFile, []byte("custom-token-v1"), 0600)).To(Succeed())

		base := &statusRoundTripper{statusCode: 200}
		transport := auth.NewServiceAccountTransportWithPath(tokenFile, base)
		client := &http.Client{Transport: transport}

		req, err := http.NewRequest("GET", "http://localhost/test", nil)
		Expect(err).ToNot(HaveOccurred())

		resp, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		_ = resp.Body.Close()

		headers := base.getAuthHeaders()
		Expect(headers).To(HaveLen(1))
		Expect(headers[0]).To(Equal("Bearer custom-token-v1"))
	})

	// UT-AT-1055-002: 401 response invalidates token cache
	It("UT-AT-1055-002: should invalidate token cache when downstream returns 401", func() {
		Expect(os.WriteFile(tokenFile, []byte("stale-token"), 0600)).To(Succeed())

		base := &statusRoundTripper{statusCode: 200}
		transport := auth.NewServiceAccountTransportWithPath(tokenFile, base)
		client := &http.Client{Transport: transport}

		req1, _ := http.NewRequest("GET", "http://localhost/test", nil)
		resp1, err := client.Do(req1)
		Expect(err).ToNot(HaveOccurred())
		_ = resp1.Body.Close()
		Expect(resp1.StatusCode).To(Equal(200))

		// Now downstream returns 401 — cache should be invalidated
		base.setStatus(401)
		req2, _ := http.NewRequest("GET", "http://localhost/test", nil)
		resp2, err := client.Do(req2)
		Expect(err).ToNot(HaveOccurred())
		_ = resp2.Body.Close()
		Expect(resp2.StatusCode).To(Equal(401))

		// Write new token to file (simulating kubelet rotation)
		Expect(os.WriteFile(tokenFile, []byte("fresh-token"), 0600)).To(Succeed())

		// Next request should re-read from file (cache was invalidated)
		base.setStatus(200)
		req3, _ := http.NewRequest("GET", "http://localhost/test", nil)
		resp3, err := client.Do(req3)
		Expect(err).ToNot(HaveOccurred())
		_ = resp3.Body.Close()

		headers := base.getAuthHeaders()
		Expect(headers).To(HaveLen(3))
		Expect(headers[0]).To(Equal("Bearer stale-token"))
		Expect(headers[1]).To(Equal("Bearer stale-token"))
		Expect(headers[2]).To(Equal("Bearer fresh-token"))
	})

	// UT-AT-1055-003: Token re-read after invalidation picks up new file content
	It("UT-AT-1055-003: should pick up rotated token content after 401 invalidation", func() {
		Expect(os.WriteFile(tokenFile, []byte("token-v1"), 0600)).To(Succeed())

		base := &statusRoundTripper{statusCode: 401}
		transport := auth.NewServiceAccountTransportWithPath(tokenFile, base)
		client := &http.Client{Transport: transport}

		req1, _ := http.NewRequest("GET", "http://localhost/test", nil)
		_, err := client.Do(req1)
		Expect(err).ToNot(HaveOccurred())

		// Kubelet writes new token
		Expect(os.WriteFile(tokenFile, []byte("token-v2"), 0600)).To(Succeed())

		base.setStatus(200)
		req2, _ := http.NewRequest("GET", "http://localhost/test", nil)
		resp2, err := client.Do(req2)
		Expect(err).ToNot(HaveOccurred())
		_ = resp2.Body.Close()

		headers := base.getAuthHeaders()
		Expect(headers).To(HaveLen(2))
		Expect(headers[0]).To(Equal("Bearer token-v1"))
		Expect(headers[1]).To(Equal("Bearer token-v2"))
	})

	// UT-AT-1055-004: Non-401 responses do NOT invalidate cache
	It("UT-AT-1055-004: should NOT invalidate cache for 200 or 500 responses", func() {
		Expect(os.WriteFile(tokenFile, []byte("cached-token"), 0600)).To(Succeed())

		base := &statusRoundTripper{statusCode: 200}
		transport := auth.NewServiceAccountTransportWithPath(tokenFile, base)
		client := &http.Client{Transport: transport}

		// First request populates cache
		req1, _ := http.NewRequest("GET", "http://localhost/test", nil)
		_, _ = client.Do(req1)

		// Write different token (should NOT be picked up — cache still valid)
		Expect(os.WriteFile(tokenFile, []byte("different-token"), 0600)).To(Succeed())

		// 200 response — cache should remain
		req2, _ := http.NewRequest("GET", "http://localhost/test", nil)
		_, _ = client.Do(req2)

		// 500 response — cache should still remain
		base.setStatus(500)
		req3, _ := http.NewRequest("GET", "http://localhost/test", nil)
		_, _ = client.Do(req3)

		headers := base.getAuthHeaders()
		Expect(headers).To(HaveLen(3))
		for _, h := range headers {
			Expect(h).To(Equal("Bearer cached-token"),
				"Non-401 responses must not invalidate cache")
		}
	})

	// UT-AT-1055-005: Concurrent RoundTrip under 401 storm
	It("UT-AT-1055-005: should handle concurrent 401 storm without races", func() {
		Expect(os.WriteFile(tokenFile, []byte("concurrent-token"), 0600)).To(Succeed())

		base := &statusRoundTripper{statusCode: 401}
		transport := auth.NewServiceAccountTransportWithPath(tokenFile, base)
		client := &http.Client{Transport: transport}

		const numGoroutines = 20
		var wg sync.WaitGroup
		var errCount int32

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, _ := http.NewRequest("GET", "http://localhost/test", nil)
				resp, err := client.Do(req)
				if err != nil {
					atomic.AddInt32(&errCount, 1)
					return
				}
				_ = resp.Body.Close()
			}()
		}

		wg.Wait()
		Expect(atomic.LoadInt32(&errCount)).To(Equal(int32(0)),
			"All concurrent requests should complete without error")
		Expect(int(atomic.LoadInt32(&base.callCount))).To(Equal(numGoroutines))
	})

	Context("nil/zero edge cases", func() {
		It("should use http.DefaultTransport when base is nil", func() {
			Expect(os.WriteFile(tokenFile, []byte("nil-base-token"), 0600)).To(Succeed())
			transport := auth.NewServiceAccountTransportWithPath(tokenFile, nil)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Echo-Auth", r.Header.Get("Authorization"))
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := &http.Client{Transport: transport}
			resp, err := client.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.Header.Get("X-Echo-Auth")).To(Equal("Bearer nil-base-token"))
		})

		It("should handle empty token path gracefully", func() {
			base := &statusRoundTripper{statusCode: 200}
			transport := auth.NewServiceAccountTransportWithPath("", base)
			client := &http.Client{Transport: transport}

			req, _ := http.NewRequest("GET", "http://localhost/test", nil)
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			_ = resp.Body.Close()

			headers := base.getAuthHeaders()
			Expect(headers).To(HaveLen(1))
			Expect(headers[0]).To(BeEmpty(), "Empty path should result in no auth header")
		})
	})
})

// testRoundTripper is a custom RoundTripper for testing
type testRoundTripper struct {
	base   http.RoundTripper
	called bool
}

func (t *testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.called = true
	return t.base.RoundTrip(req)
}
