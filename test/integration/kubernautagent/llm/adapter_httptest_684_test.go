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

var _ = Describe("Vertex AI + Claude httptest round-trips — #684", func() {

	simpleRequest := llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "You are a Kubernetes investigator."},
			{Role: "user", Content: "Why is the pod crashing?"},
		},
		Options: llm.ChatOptions{
			MaxTokens:   200,
			Temperature: 0.0,
		},
	}

	Describe("IT-KA-684-201: vertex_ai adapter sends Anthropic Messages API request and parses response", func() {
		It("routes Chat() through Vertex AI endpoint with Anthropic format", func() {
			var receivedPath string
			var receivedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				receivedBody, _ = io.ReadAll(r.Body)
				Expect(len(receivedBody)).To(BeNumerically(">", 0))

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(vertexAnthropicResponse("The pod is OOMKilled due to memory limit of 256Mi."))
			}))
			defer server.Close()

			adapter, err := langchaingo.New("vertex_ai", server.URL, "claude-sonnet-4-6", "",
				langchaingo.WithVertexProject("my-project"),
				langchaingo.WithVertexLocation("us-central1"),
			)
			Expect(err).NotTo(HaveOccurred())

			resp, err := adapter.Chat(context.Background(), simpleRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(ContainSubstring("OOMKilled"))
			Expect(resp.Message.Role).To(Equal("assistant"))

			Expect(receivedPath).To(ContainSubstring("publishers/anthropic/models/claude-sonnet-4-6"))
		})
	})

	Describe("IT-KA-684-202: vertex_ai adapter handles tool call responses", func() {
		It("returns tool calls from Vertex AI Claude response", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(vertexAnthropicToolCallResponse())
			}))
			defer server.Close()

			adapter, err := langchaingo.New("vertex_ai", server.URL, "claude-sonnet-4-6", "",
				langchaingo.WithVertexProject("my-project"),
				langchaingo.WithVertexLocation("us-central1"),
			)
			Expect(err).NotTo(HaveOccurred())

			req := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "Describe the crashing pod"},
				},
				Tools: []llm.ToolDefinition{
					{
						Name:        "kubectl_describe",
						Description: "Describe a Kubernetes resource",
						Parameters:  json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}}}`),
					},
				},
			}

			resp, err := adapter.Chat(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ToolCalls).To(HaveLen(1))
			Expect(resp.ToolCalls[0].Name).To(Equal("kubectl_describe"))
			Expect(resp.ToolCalls[0].Arguments).To(ContainSubstring("Pod"))
		})
	})

	Describe("IT-KA-684-202b: vertex_ai adapter handles tool response round-trip", func() {
		It("sends tool results back and receives follow-up response", func() {
			callCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				body, _ := io.ReadAll(r.Body)

				if callCount == 1 {
					Expect(string(body)).To(ContainSubstring("tool_result"))
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(vertexAnthropicResponse("The pod has 5 restarts."))
			}))
			defer server.Close()

			adapter, err := langchaingo.New("vertex_ai", server.URL, "claude-sonnet-4-6", "",
				langchaingo.WithVertexProject("my-project"),
				langchaingo.WithVertexLocation("us-central1"),
			)
			Expect(err).NotTo(HaveOccurred())

			req := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "Investigate crash"},
					{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{
						{ID: "toolu_001", Name: "kubectl_describe", Arguments: `{"kind":"Pod"}`},
					}},
					{Role: "tool", Content: `{"restartCount":5}`, ToolCallID: "toolu_001", ToolName: "kubectl_describe"},
				},
			}

			resp, err := adapter.Chat(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(ContainSubstring("5 restarts"))
		})
	})

	Describe("IT-KA-684-203: Azure adapter round-trip unaffected by #684 changes", func() {
		It("still works as before", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(openAIResponse("azure still works"))
			}))
			defer server.Close()

			adapter, err := langchaingo.New("azure", server.URL, "gpt-4", "fake-key",
				langchaingo.WithAzureAPIVersion("2024-02-15-preview"))
			Expect(err).NotTo(HaveOccurred())

			resp, err := adapter.Chat(context.Background(), simpleRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("azure still works"))
		})
	})

	Describe("IT-KA-684-204: Anthropic adapter round-trip unaffected by #684 changes", func() {
		It("still works as before", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(anthropicResponse("anthropic still works"))
			}))
			defer server.Close()

			adapter, err := langchaingo.New("anthropic", server.URL, "claude-sonnet-4-20250514", "fake-key")
			Expect(err).NotTo(HaveOccurred())

			resp, err := adapter.Chat(context.Background(), simpleRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("anthropic still works"))
		})
	})
})

func vertexAnthropicResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"id":          "msg_vertex_test",
		"type":        "message",
		"role":        "assistant",
		"model":       "claude-sonnet-4-6",
		"stop_reason": "end_turn",
		"content": []map[string]interface{}{
			{"type": "text", "text": content},
		},
		"usage": map[string]interface{}{
			"input_tokens":  50,
			"output_tokens": 25,
		},
	}
}

func vertexAnthropicToolCallResponse() map[string]interface{} {
	return map[string]interface{}{
		"id":          "msg_vertex_tc_test",
		"type":        "message",
		"role":        "assistant",
		"model":       "claude-sonnet-4-6",
		"stop_reason": "tool_use",
		"content": []map[string]interface{}{
			{
				"type": "tool_use",
				"id":   "toolu_vertex_001",
				"name": "kubectl_describe",
				"input": map[string]interface{}{
					"kind":      "Pod",
					"name":      "api-server",
					"namespace": "prod",
				},
			},
		},
		"usage": map[string]interface{}{
			"input_tokens":  100,
			"output_tokens": 30,
		},
	}
}
