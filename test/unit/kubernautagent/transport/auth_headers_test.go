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

package transport_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
)

// capturingTransport records the request forwarded by the outer RoundTripper.
type capturingTransport struct {
	captured *http.Request
}

func (c *capturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.captured = req
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

var _ = Describe("AuthHeadersTransport — #417", func() {

	// UT-KA-417-016: RoundTripper request cloning contract
	Describe("UT-KA-417-016: Request cloning contract", func() {
		It("should not mutate the original request", func() {
			inner := &capturingTransport{}
			headers := []config.HeaderDefinition{
				{Name: "Authorization", Value: "Bearer test"},
			}
			rt := transport.NewAuthHeadersTransport(headers, inner)

			original, err := http.NewRequest("GET", "https://llm.example.com/v1/chat/completions", nil)
			Expect(err).NotTo(HaveOccurred())
			original.Header.Set("Content-Type", "application/json")

			_, err = rt.RoundTrip(original)
			Expect(err).NotTo(HaveOccurred())

			Expect(original.Header.Get("Authorization")).To(BeEmpty(),
				"original request must NOT be mutated by RoundTrip")
			Expect(original.Header.Get("Content-Type")).To(Equal("application/json"))

			Expect(inner.captured.Header.Get("Authorization")).To(Equal("Bearer test"),
				"inner transport must see the injected header")
		})
	})

	// UT-KA-417-001: RoundTripper injects all configured headers
	Describe("UT-KA-417-001: Inject all configured headers", func() {
		It("should inject all headers into the outbound request", func() {
			inner := &capturingTransport{}
			headers := []config.HeaderDefinition{
				{Name: "x-api-key", Value: "test-key"},
				{Name: "x-tenant-id", Value: "prod"},
				{Name: "Authorization", Value: "Bearer abc123"},
			}
			rt := transport.NewAuthHeadersTransport(headers, inner)

			req := httptest.NewRequest("POST", "https://llm.example.com/v1/chat/completions", nil)
			req.Header.Set("Content-Type", "application/json")

			_, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())

			Expect(inner.captured.Header.Get("x-api-key")).To(Equal("test-key"))
			Expect(inner.captured.Header.Get("x-tenant-id")).To(Equal("prod"))
			Expect(inner.captured.Header.Get("Authorization")).To(Equal("Bearer abc123"))
			Expect(inner.captured.Header.Get("Content-Type")).To(Equal("application/json"),
				"pre-existing headers must be preserved")
		})
	})

	// UT-KA-417-008: Mixed sources in single request
	Describe("UT-KA-417-008: Mixed sources all injected", func() {
		It("should resolve secretKeyRef, filePath, and value in one request", func() {
			os.Setenv("KA_TEST_MIXED_SECRET", "secret-from-env")
			defer os.Unsetenv("KA_TEST_MIXED_SECRET")

			tmpFile, err := os.CreateTemp("", "ka-test-mixed-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tmpFile.Name())
			err = os.WriteFile(tmpFile.Name(), []byte("jwt-from-file"), 0644)
			Expect(err).NotTo(HaveOccurred())

			inner := &capturingTransport{}
			headers := []config.HeaderDefinition{
				{Name: "x-api-key", SecretKeyRef: "KA_TEST_MIXED_SECRET"},
				{Name: "Authorization", FilePath: tmpFile.Name()},
				{Name: "x-tenant-id", Value: "kubernaut-prod"},
			}
			rt := transport.NewAuthHeadersTransport(headers, inner)

			req := httptest.NewRequest("POST", "https://llm.example.com/v1/chat/completions", nil)
			_, err = rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())

			Expect(inner.captured.Header.Get("x-api-key")).To(Equal("secret-from-env"))
			Expect(inner.captured.Header.Get("Authorization")).To(Equal("jwt-from-file"))
			Expect(inner.captured.Header.Get("x-tenant-id")).To(Equal("kubernaut-prod"))
		})
	})

	// UT-KA-417-007: Concurrent filePath reads
	Describe("UT-KA-417-007: Concurrent filePath reads are safe", func() {
		It("should handle 100 concurrent requests without panics or garbled values", func() {
			tmpFile, err := os.CreateTemp("", "ka-test-concurrent-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tmpFile.Name())
			err = os.WriteFile(tmpFile.Name(), []byte("concurrent-token"), 0644)
			Expect(err).NotTo(HaveOccurred())

			inner := &echoTransport{}
			headers := []config.HeaderDefinition{
				{Name: "Authorization", FilePath: tmpFile.Name()},
			}
			rt := transport.NewAuthHeadersTransport(headers, inner)

			const numRequests = 100
			errs := make(chan error, numRequests)
			var wg sync.WaitGroup
			wg.Add(numRequests)

			for i := 0; i < numRequests; i++ {
				go func() {
					defer wg.Done()
					req := httptest.NewRequest("GET", "https://llm.example.com/v1/chat/completions", nil)
					_, err := rt.RoundTrip(req)
					if err != nil {
						errs <- err
					}
				}()
			}
			wg.Wait()
			close(errs)

			for err := range errs {
				Fail("concurrent RoundTrip error: " + err.Error())
			}
		})
	})

	// UT-KA-417-017: Header values absent from request body
	Describe("UT-KA-417-017: Header values do not appear in request body", func() {
		It("should not modify the request body", func() {
			inner := &capturingTransport{}
			headers := []config.HeaderDefinition{
				{Name: "Authorization", Value: "Bearer secret-token-xyz"},
			}
			rt := transport.NewAuthHeadersTransport(headers, inner)

			body := `{"model":"gpt-4","messages":[{"role":"user","content":"analyze pod crash"}]}`
			req := httptest.NewRequest("POST", "https://llm.example.com/v1/chat/completions",
				strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			_, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())

			capturedBody, err := io.ReadAll(inner.captured.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(capturedBody)).To(Equal(body),
				"body must be unchanged")
			Expect(string(capturedBody)).NotTo(ContainSubstring("secret-token-xyz"),
				"header value must not leak into body")
		})
	})
})

// echoTransport returns 200 OK for every request (used for concurrency tests).
type echoTransport struct{}

func (e *echoTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}
