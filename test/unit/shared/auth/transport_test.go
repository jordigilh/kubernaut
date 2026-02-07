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

// testRoundTripper is a custom RoundTripper for testing
type testRoundTripper struct {
	base   http.RoundTripper
	called bool
}

func (t *testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.called = true
	return t.base.RoundTrip(req)
}
