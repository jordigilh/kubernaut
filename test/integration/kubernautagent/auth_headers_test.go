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

package kubernautagent_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
	llmclient "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("Auth Headers Integration — #417", func() {

	// IT-KA-417-001: Full round trip with all three source types
	Describe("IT-KA-417-001: Full round trip with all three sources", func() {
		var (
			server  *httptest.Server
			headers map[string]string
		)

		BeforeEach(func() {
			headers = make(map[string]string)
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for name := range r.Header {
					headers[name] = r.Header.Get(name)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("should inject headers from all three sources over real HTTP", func() {
			os.Setenv("KA_IT_TEST_API_KEY", "secret-key-123")
			defer os.Unsetenv("KA_IT_TEST_API_KEY")

			tmpFile, err := os.CreateTemp("", "ka-it-token-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tmpFile.Name())
			err = os.WriteFile(tmpFile.Name(), []byte("jwt-token-xyz"), 0644)
			Expect(err).NotTo(HaveOccurred())

			hdefs := []config.HeaderDefinition{
				{Name: "x-api-key", SecretKeyRef: "KA_IT_TEST_API_KEY"},
				{Name: "Authorization", FilePath: tmpFile.Name()},
				{Name: "x-tenant-id", Value: "kubernaut-prod"},
			}

			client, err := llmclient.NewLLMClient(server.URL, hdefs)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Post(server.URL+"/v1/chat/completions", "application/json", nil)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(headers["X-Api-Key"]).To(Equal("secret-key-123"))
			Expect(headers["Authorization"]).To(Equal("jwt-token-xyz"))
			Expect(headers["X-Tenant-Id"]).To(Equal("kubernaut-prod"))
		})
	})

	// IT-KA-417-003: Backward compatibility — zero custom headers
	Describe("IT-KA-417-003: Backward compatibility with zero headers", func() {
		It("should send requests without extra headers when none configured", func() {
			var receivedHeaders http.Header
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client, err := llmclient.NewLLMClient(server.URL, nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Get(server.URL + "/v1/chat/completions")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(receivedHeaders.Get("Authorization")).To(BeEmpty())
			Expect(receivedHeaders.Get("x-api-key")).To(BeEmpty())
			Expect(receivedHeaders.Get("x-tenant-id")).To(BeEmpty())
		})
	})

	// IT-KA-417-002: Provider-agnostic transport
	Describe("IT-KA-417-002: Headers injected for multiple endpoints", func() {
		It("should inject headers for both OpenAI and Ollama endpoints", func() {
			openaiHeaders := make(map[string]string)
			openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				openaiHeaders["Authorization"] = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			defer openai.Close()

			ollamaHeaders := make(map[string]string)
			ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ollamaHeaders["Authorization"] = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			defer ollama.Close()

			hdefs := []config.HeaderDefinition{
				{Name: "Authorization", Value: "Bearer shared-token"},
			}
			client, err := llmclient.NewLLMClient("", hdefs)
			Expect(err).NotTo(HaveOccurred())

			resp1, err := client.Get(openai.URL + "/v1/chat/completions")
			Expect(err).NotTo(HaveOccurred())
			resp1.Body.Close()

			resp2, err := client.Get(ollama.URL + "/api/generate")
			Expect(err).NotTo(HaveOccurred())
			resp2.Body.Close()

			Expect(openaiHeaders["Authorization"]).To(Equal("Bearer shared-token"))
			Expect(ollamaHeaders["Authorization"]).To(Equal("Bearer shared-token"))
		})
	})

	// IT-KA-417-004: Error log credential scrubbing
	Describe("IT-KA-417-004: Error log does not contain header value", func() {
		It("should redact sensitive header values in log output on error", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			var logBuf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			hdefs := []config.HeaderDefinition{
				{Name: "Authorization", SecretKeyRef: "KA_IT_SCRUB_SECRET"},
			}
			os.Setenv("KA_IT_SCRUB_SECRET", "Bearer super-secret-key-do-not-leak")
			defer os.Unsetenv("KA_IT_SCRUB_SECRET")

			client, err := llmclient.NewLLMClientWithLogger(server.URL, hdefs, logger)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Get(server.URL + "/v1/chat/completions")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			Expect(logBuf.String()).NotTo(ContainSubstring("super-secret-key-do-not-leak"),
				"sensitive header value must not appear in log output")
		})
	})

	// IT-KA-417-005: Token rotation without restart
	Describe("IT-KA-417-005: Token rotation without restart", func() {
		It("should pick up new token after file update", func() {
			capturedTokens := make([]string, 0, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedTokens = append(capturedTokens, r.Header.Get("Authorization"))
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			tmpFile, err := os.CreateTemp("", "ka-it-rotation-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tmpFile.Name())
			err = os.WriteFile(tmpFile.Name(), []byte("token-v1"), 0644)
			Expect(err).NotTo(HaveOccurred())

			hdefs := []config.HeaderDefinition{
				{Name: "Authorization", FilePath: tmpFile.Name()},
			}
			client, err := llmclient.NewLLMClient(server.URL, hdefs)
			Expect(err).NotTo(HaveOccurred())

			resp1, err := client.Get(server.URL + "/v1/chat/completions")
			Expect(err).NotTo(HaveOccurred())
			resp1.Body.Close()

			err = os.WriteFile(tmpFile.Name(), []byte("token-v2"), 0644)
			Expect(err).NotTo(HaveOccurred())

			resp2, err := client.Get(server.URL + "/v1/chat/completions")
			Expect(err).NotTo(HaveOccurred())
			resp2.Body.Close()

			Expect(capturedTokens).To(HaveLen(2))
			Expect(capturedTokens[0]).To(Equal("token-v1"))
			Expect(capturedTokens[1]).To(Equal("token-v2"))
		})
	})
})
