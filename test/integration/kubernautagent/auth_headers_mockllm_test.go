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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
	llmclient "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// mockLLMServer simulates the Mock LLM verification API (#570).
// POST /v1/chat/completions — records headers, returns chat completion
// GET /api/test/headers — returns recorded headers as JSON
//
// TODO: Replace with Mock LLM #570 verification API when available
type mockLLMServer struct {
	mu              sync.Mutex
	recordedHeaders map[string]string
}

func newMockLLMServer() *mockLLMServer {
	return &mockLLMServer{
		recordedHeaders: make(map[string]string),
	}
}

func (m *mockLLMServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/v1/chat/completions" && r.Method == http.MethodPost:
		m.mu.Lock()
		for name, values := range r.Header {
			m.recordedHeaders[name] = values[0]
		}
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-mock-001",
			"object": "chat.completion",
			"choices": [{"message": {"role": "assistant", "content": "OOM detected"}}]
		}`))

	case r.URL.Path == "/api/test/headers" && r.Method == http.MethodGet:
		m.mu.Lock()
		data, _ := json.Marshal(m.recordedHeaders)
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

var _ = Describe("Auth Headers Mock LLM Verification — #417", func() {

	// IT-KA-417-006: End-to-end with Mock LLM verification API
	Describe("IT-KA-417-006: Mock LLM records and exposes injected headers", func() {
		var (
			mockLLM *mockLLMServer
			server  *httptest.Server
		)

		BeforeEach(func() {
			mockLLM = newMockLLMServer()
			server = httptest.NewServer(mockLLM)
		})

		AfterEach(func() {
			server.Close()
		})

		It("should verify headers via Mock LLM verification API", func() {
			hdefs := []config.HeaderDefinition{
				{Name: "Authorization", Value: "Bearer test-token-e2e"},
			}
			client, err := llmclient.NewLLMClient(server.URL, hdefs)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Post(server.URL+"/v1/chat/completions",
				"application/json",
				nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			verifyResp, err := http.Get(server.URL + "/api/test/headers")
			Expect(err).NotTo(HaveOccurred())
			defer verifyResp.Body.Close()
			Expect(verifyResp.StatusCode).To(Equal(http.StatusOK))

			var recorded map[string]string
			err = json.NewDecoder(verifyResp.Body).Decode(&recorded)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorded["Authorization"]).To(Equal("Bearer test-token-e2e"),
				"Mock LLM verification API must confirm the header was received")
		})

		It("should process chat completion normally despite auth headers", func() {
			hdefs := []config.HeaderDefinition{
				{Name: "Authorization", Value: "Bearer test-token-e2e"},
				{Name: "x-correlation-id", Value: "req-001"},
			}
			client, err := llmclient.NewLLMClient(server.URL, hdefs)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Post(server.URL+"/v1/chat/completions",
				"application/json",
				nil)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result["id"]).To(Equal("chatcmpl-mock-001"),
				"Mock LLM should return a normal chat completion response")
		})
	})
})
