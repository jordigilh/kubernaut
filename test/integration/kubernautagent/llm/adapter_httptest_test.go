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

package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
)

// TP-433 Phase 7b: LLM adapter httptest round-trip integration tests.
// Validates that adapters correctly route requests through the configured
// endpoint and parse provider-specific responses via LangChainGo.

var _ = Describe("LLM Adapter httptest round-trips — #433", func() {

	simpleRequest := llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "You are a test assistant."},
			{Role: "user", Content: "Say hello."},
		},
		Options: llm.ChatOptions{
			MaxTokens:   100,
			Temperature: 0.0,
		},
	}

	Describe("IT-KA-433-050: Azure adapter Chat via httptest.Server", func() {
		It("sends request to configured Azure endpoint and parses response", func() {
			var receivedPath string
			var receivedAPIVersion string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				receivedAPIVersion = r.URL.Query().Get("api-version")
				body, _ := io.ReadAll(r.Body)
				Expect(len(body)).To(BeNumerically(">", 0))

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(openAIResponse("azure says hello"))
			}))
			defer server.Close()

			adapter, err := langchaingo.New("azure", server.URL, "test-deployment", "fake-key",
				langchaingo.WithAzureAPIVersion("2024-02-15-preview"))
			Expect(err).NotTo(HaveOccurred())

			resp, err := adapter.Chat(context.Background(), simpleRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("azure says hello"))
			Expect(resp.Message.Role).To(Equal("assistant"))

			Expect(receivedPath).To(ContainSubstring("chat/completions"))
			Expect(receivedAPIVersion).To(Equal("2024-02-15-preview"))
		})
	})

	Describe("IT-KA-433-051: Anthropic adapter Chat via httptest.Server", func() {
		It("sends request to configured Anthropic endpoint and parses response", func() {
			var receivedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				Expect(r.Header.Get("X-Api-Key")).To(Equal("fake-anthropic-key"))

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(anthropicResponse("anthropic says hello"))
			}))
			defer server.Close()

			adapter, err := langchaingo.New("anthropic", server.URL, "claude-sonnet-4-20250514", "fake-anthropic-key")
			Expect(err).NotTo(HaveOccurred())

			resp, err := adapter.Chat(context.Background(), simpleRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("anthropic says hello"))
			Expect(resp.Message.Role).To(Equal("assistant"))
			Expect(receivedPath).To(ContainSubstring("messages"))
		})
	})

	Describe("IT-KA-433-052: Mistral adapter Chat via httptest.Server", func() {
		It("sends request to configured Mistral endpoint and parses response", func() {
			var receivedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(openAIResponse("mistral says hello"))
			}))
			defer server.Close()

			adapter, err := langchaingo.New("mistral", server.URL, "mistral-large-latest", "fake-mistral-key")
			Expect(err).NotTo(HaveOccurred())

			resp, err := adapter.Chat(context.Background(), simpleRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("mistral says hello"))
			Expect(resp.Message.Role).To(Equal("assistant"))
			Expect(receivedPath).To(ContainSubstring("chat/completions"))
		})
	})
})

func openAIResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"id":      "chatcmpl-test",
		"object":  "chat.completion",
		"model":   "test-model",
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"finish_reason": "stop",
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		},
	}
}

func anthropicResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"id":          "msg_test",
		"type":        "message",
		"role":        "assistant",
		"model":       "claude-sonnet-4-20250514",
		"stop_reason": "end_turn",
		"content": []map[string]interface{}{
			{"type": "text", "text": content},
		},
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 5,
		},
	}
}
