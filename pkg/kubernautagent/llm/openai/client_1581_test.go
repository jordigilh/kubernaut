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

package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
)

// Issue #1581 / BR-AI-086: KA's thin wrapper over the shared
// pkg/shared/llm/openaicompat client, translating llm.ChatRequest/Message
// to/from the shared protocol-neutral types (DD-LLM-005). Covers
// IT-KA-1581-002 (production wiring is asserted separately in
// cmd/kubernautagent's llm_builder tests).
var _ = Describe("kubernautagent/llm/openai.Client — #1581", func() {
	var (
		server       *httptest.Server
		receivedBody map[string]interface{}
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	newTestClient := func(model, responseJSON string) llm.Client {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(responseJSON))
		}))
		return kaopenai.New(model, server.URL, "test-key")
	}

	It("UT-KA-1581-201: implements llm.Client", func() {
		var _ llm.Client = kaopenai.New("gpt-4o", "https://example.invalid", "key")
	})

	It("UT-KA-1581-202: Chat round-trips a plain text response", func() {
		client := newTestClient("gpt-4o", `{
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "hi there"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8}
		}`)
		resp, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hello"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Message.Content).To(Equal("hi there"))
		Expect(resp.FinishReason).To(Equal(llm.FinishReasonStop))
		Expect(resp.Usage.TotalTokens).To(Equal(8))
	})

	It("UT-KA-1581-203: normalizes finish_reason 'tool_calls' to llm.FinishReasonToolCalls", func() {
		client := newTestClient("gpt-4o", `{"choices":[{"index":0,"message":{"role":"assistant","content":null,
			"tool_calls":[{"id":"call_1","type":"function","function":{"name":"kubectl_get","arguments":"{}"}}]},
			"finish_reason":"tool_calls"}]}`)
		resp, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "investigate"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.FinishReason).To(Equal(llm.FinishReasonToolCalls))
		Expect(resp.ToolCalls).To(HaveLen(1))
		Expect(resp.ToolCalls[0].Name).To(Equal("kubectl_get"))
	})

	It("UT-KA-1581-204: forwards tool definitions and outbound tool-call/tool-result history", func() {
		client := newTestClient("gpt-4o", `{"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`)
		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{
				{Role: "user", Content: "run"},
				{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "kubectl_get", Arguments: `{"kind":"Pod"}`}}},
				{Role: "tool", Content: "Running", ToolCallID: "call_1", ToolName: "kubectl_get"},
			},
			Tools: []llm.ToolDefinition{{Name: "kubectl_get", Description: "get resources", Parameters: json.RawMessage(`{"type":"object"}`)}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedBody["tools"]).NotTo(BeNil())
		messages := receivedBody["messages"].([]interface{})
		Expect(messages).To(HaveLen(3))
	})

	Describe("reasoning capture and replay — BR-AI-086 AC3, model-aware auto-detection", func() {
		It("UT-KA-1581-205: captures reasoning_content from a deepseek-reasoner response into Message.Reasoning", func() {
			client := newTestClient("deepseek-reasoner", `{
				"choices": [{"index": 0, "message": {"role": "assistant", "content": "42",
					"reasoning_content": "step by step..."}, "finish_reason": "stop"}]
			}`)
			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "6*7?"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Reasoning).NotTo(BeNil())
			Expect(resp.Message.Reasoning.Text).To(Equal("step by step..."))
		})

		It("UT-KA-1581-206: replays reasoning only for a tool-call turn on deepseek-reasoner (conditional mode)", func() {
			client := newTestClient("deepseek-reasoner", `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "investigate"},
					{
						Role:      "assistant",
						Content:   "checking",
						Reasoning: &llm.ReasoningBlock{Text: "I should check pods first"},
						ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "check", Arguments: "{}"}},
					},
					{Role: "tool", Content: "ok", ToolCallID: "call_1"},
					{Role: "user", Content: "continue"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			messages := receivedBody["messages"].([]interface{})
			assistantMsg := messages[1].(map[string]interface{})
			Expect(assistantMsg["reasoning_content"]).To(Equal("I should check pods first"))
		})

		It("UT-KA-1581-207: omits reasoning_content on an unrecognized model — compatibility floor default", func() {
			client := newTestClient("some-bare-bones-local-model", `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "investigate"},
					{
						Role:      "assistant",
						Content:   "checking",
						Reasoning: &llm.ReasoningBlock{Text: "chain of thought"},
						ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "check", Arguments: "{}"}},
					},
					{Role: "tool", Content: "ok", ToolCallID: "call_1"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			messages := receivedBody["messages"].([]interface{})
			assistantMsg := messages[1].(map[string]interface{})
			Expect(assistantMsg).NotTo(HaveKey("reasoning_content"))
		})
	})

	It("UT-KA-1581-208: StreamChat forwards text deltas and produces the final tool-call-inclusive response", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, c := range []string{
				`{"choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			} {
				_, _ = w.Write([]byte("data: " + c + "\n\n"))
				if flusher != nil {
					flusher.Flush()
				}
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}))
		client := kaopenai.New("gpt-4o", server.URL, "test-key")

		var deltas []string
		resp, err := client.StreamChat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hi"}},
		}, func(ev llm.ChatStreamEvent) error {
			if ev.Delta != "" {
				deltas = append(deltas, ev.Delta)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(deltas).To(Equal([]string{"Hi"}))
		Expect(resp.FinishReason).To(Equal(llm.FinishReasonStop))
	})

	It("UT-KA-1581-209: Close is a no-op returning nil", func() {
		client := kaopenai.New("gpt-4o", "https://example.invalid", "key")
		Expect(client.Close()).To(Succeed())
	})
})
