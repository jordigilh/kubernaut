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

package openaicompat_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

// Issue #1581 / BR-AI-086: extraction RED for the shared, provider-agnostic
// OpenAI-Chat-Completions-protocol client, ported and generalized from
// pkg/apifrontend/launcher/openai/adapter.go so AF and KA can share one
// implementation instead of two independently-maintained copies (DD-LLM-005).
var _ = Describe("openaicompat.Client — #1581", func() {
	var (
		server       *httptest.Server
		receivedBody map[string]interface{}
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	newTestClient := func(model, responseJSON string) *openaicompat.Client {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(responseJSON))
		}))
		return openaicompat.New(model, server.URL, "test-key")
	}

	buildHistory := func(hadToolCalls bool) []openaicompat.Message {
		msg := openaicompat.Message{Role: "assistant", Content: "conclusion", Reasoning: "my chain of thought"}
		if hadToolCalls {
			msg.ToolCalls = []openaicompat.ToolCall{{ID: "call_1", Name: "check", Arguments: "{}"}}
		}
		return []openaicompat.Message{
			{Role: "user", Content: "investigate"},
			msg,
			{Role: "user", Content: "continue"},
		}
	}

	Describe("basic non-streaming round trip", func() {
		It("UT-KA-1581-001: sends messages and maps a plain text response back", func() {
			client := newTestClient("gpt-4o", `{
				"id": "chatcmpl-1", "object": "chat.completion", "model": "gpt-4o",
				"choices": [{"index": 0, "message": {"role": "assistant", "content": "hello there"}, "finish_reason": "stop"}],
				"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
			}`)

			resp, err := client.Chat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("hello there"))
			Expect(resp.FinishReason).To(Equal("stop"))
			Expect(resp.Usage.TotalTokens).To(Equal(15))
			Expect(receivedBody["model"]).To(Equal("gpt-4o"))
		})

		It("UT-KA-1581-002: sends the Authorization bearer header from apiKey", func() {
			var gotAuth string
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
			}))
			client := openaicompat.New("gpt-4o", server.URL, "sk-test-123")
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(gotAuth).To(Equal("Bearer sk-test-123"))
		})
	})

	Describe("tool calls", func() {
		It("UT-KA-1581-003: maps a tool_calls response into Response.Message.ToolCalls", func() {
			client := newTestClient("gpt-4o", `{
				"choices": [{"index": 0, "message": {"role": "assistant", "content": null,
					"tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"loc\":\"SF\"}"}}]},
					"finish_reason": "tool_calls"}]
			}`)
			resp, err := client.Chat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{{Role: "user", Content: "weather?"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.ToolCalls).To(HaveLen(1))
			Expect(resp.Message.ToolCalls[0].Name).To(Equal("get_weather"))
			Expect(resp.FinishReason).To(Equal("tool_calls"))
		})

		It("UT-KA-1581-004: serializes outbound tool_calls and tool_call_id on replay", func() {
			client := newTestClient("gpt-4o", `{"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`)
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{
					{Role: "user", Content: "run a tool"},
					{Role: "assistant", ToolCalls: []openaicompat.ToolCall{{ID: "call_1", Name: "kubectl_get", Arguments: `{"kind":"Pod"}`}}},
					{Role: "tool", Content: "Running", ToolCallID: "call_1"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			messages := receivedBody["messages"].([]interface{})
			Expect(messages).To(HaveLen(3))
			assistantMsg := messages[1].(map[string]interface{})
			Expect(assistantMsg).To(HaveKey("tool_calls"))
			toolMsg := messages[2].(map[string]interface{})
			Expect(toolMsg["tool_call_id"]).To(Equal("call_1"))
		})
	})

	Describe("streaming", func() {
		It("UT-KA-1581-005: streams text deltas and accumulates tool-call fragments", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				flusher, _ := w.(http.Flusher)
				chunks := []string{
					`{"choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`,
					`{"choices":[{"index":0,"delta":{"content":"lo"}}]}`,
					`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_9","function":{"name":"look","arguments":""}}]}}]}`,
					`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":1}"}}]}}]}`,
					`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
				}
				for _, c := range chunks {
					_, _ = w.Write([]byte("data: " + c + "\n\n"))
					if flusher != nil {
						flusher.Flush()
					}
				}
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
			}))
			client := openaicompat.New("gpt-4o", server.URL, "test-key")

			var deltas []string
			var finalToolCalls []openaicompat.ToolCall
			err := client.StreamChat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
			}, func(ev openaicompat.StreamEvent) bool {
				if ev.Delta != "" {
					deltas = append(deltas, ev.Delta)
				}
				if ev.Done && ev.Final != nil {
					finalToolCalls = ev.Final.Message.ToolCalls
				}
				return true
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(deltas).To(Equal([]string{"Hel", "lo"}))
			Expect(finalToolCalls).To(HaveLen(1))
			Expect(finalToolCalls[0].Name).To(Equal("look"))
			Expect(finalToolCalls[0].Arguments).To(Equal(`{"x":1}`))
		})

		It("UT-KA-1581-017: resolves multiple concurrent tool calls in ascending index order, regardless of chunk arrival order", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				flusher, _ := w.(http.Flusher)
				// Index 1's fragment arrives before index 0's — the final
				// resolved order must still be [0, 1], not arrival order.
				chunks := []string{
					`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","function":{"name":"second","arguments":"{}"}}]}}]}`,
					`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"first","arguments":"{}"}}]}}]}`,
					`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
				}
				for _, c := range chunks {
					_, _ = w.Write([]byte("data: " + c + "\n\n"))
					if flusher != nil {
						flusher.Flush()
					}
				}
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
			}))
			client := openaicompat.New("gpt-4o", server.URL, "test-key")

			var finalToolCalls []openaicompat.ToolCall
			err := client.StreamChat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
			}, func(ev openaicompat.StreamEvent) bool {
				if ev.Done && ev.Final != nil {
					finalToolCalls = ev.Final.Message.ToolCalls
				}
				return true
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(finalToolCalls).To(HaveLen(2))
			Expect(finalToolCalls[0].Name).To(Equal("first"))
			Expect(finalToolCalls[1].Name).To(Equal("second"))
		})
	})

	Describe("reasoning capture — AC3: always capture when the wire response includes it", func() {
		It("UT-KA-1581-006: captures reasoning_content into Response.Message.Reasoning regardless of mode", func() {
			client := newTestClient("deepseek-reasoner", `{
				"choices": [{"index": 0, "message": {"role": "assistant", "content": "42",
					"reasoning_content": "Let me compute step by step..."}, "finish_reason": "stop"}]
			}`)
			resp, err := client.Chat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{{Role: "user", Content: "what is 6*7?"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Reasoning).To(Equal("Let me compute step by step..."))
		})

		It("UT-KA-1581-007: leaves Reasoning empty when the wire response has none", func() {
			client := newTestClient("gpt-4o", `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			resp, err := client.Chat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Reasoning).To(BeEmpty())
		})
	})

	Describe("reasoning round-trip modes — #1578 req 3: replay rules differ per provider", func() {
		It("UT-KA-1581-008: ReasoningModeNone never replays reasoning_content, even for a tool-call turn", func() {
			client := newTestClient("some-model", `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages:      buildHistory(true),
				ReasoningMode: openaicompat.ReasoningModeNone,
			})
			Expect(err).NotTo(HaveOccurred())
			messages := receivedBody["messages"].([]interface{})
			assistantMsg := messages[1].(map[string]interface{})
			Expect(assistantMsg).NotTo(HaveKey("reasoning_content"))
		})

		It("UT-KA-1581-009: ReasoningModePlain always replays reasoning_content, tool call or not", func() {
			client := newTestClient("some-vllm-model", `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages:      buildHistory(false),
				ReasoningMode: openaicompat.ReasoningModePlain,
			})
			Expect(err).NotTo(HaveOccurred())
			messages := receivedBody["messages"].([]interface{})
			assistantMsg := messages[1].(map[string]interface{})
			Expect(assistantMsg["reasoning_content"]).To(Equal("my chain of thought"))
		})

		It("UT-KA-1581-010: ReasoningModeDeepSeekConditional replays reasoning_content ONLY for a tool-call turn", func() {
			client := newTestClient("deepseek-reasoner", `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages:      buildHistory(true),
				ReasoningMode: openaicompat.ReasoningModeDeepSeekConditional,
			})
			Expect(err).NotTo(HaveOccurred())
			messages := receivedBody["messages"].([]interface{})
			assistantMsg := messages[1].(map[string]interface{})
			Expect(assistantMsg["reasoning_content"]).To(Equal("my chain of thought"),
				"DeepSeek requires reasoning_content replay for turns that performed a tool call")
		})

		It("UT-KA-1581-011: ReasoningModeDeepSeekConditional omits reasoning_content for a non-tool-call turn", func() {
			client := newTestClient("deepseek-reasoner", `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages:      buildHistory(false),
				ReasoningMode: openaicompat.ReasoningModeDeepSeekConditional,
			})
			Expect(err).NotTo(HaveOccurred())
			messages := receivedBody["messages"].([]interface{})
			assistantMsg := messages[1].(map[string]interface{})
			Expect(assistantMsg).NotTo(HaveKey("reasoning_content"),
				"DeepSeek returns HTTP 400 if reasoning_content is sent on a non-tool-call turn")
		})
	})

	Describe("DetectReasoningMode — model-aware auto-detection with an explicit override escape hatch", func() {
		It("UT-KA-1581-012: deepseek-reasoner auto-detects to conditional mode", func() {
			Expect(openaicompat.DetectReasoningMode("deepseek-reasoner", "")).To(Equal(openaicompat.ReasoningModeDeepSeekConditional))
		})

		It("UT-KA-1581-013: an unrecognized/unknown model defaults to none — the compatibility floor", func() {
			Expect(openaicompat.DetectReasoningMode("some-future-self-hosted-model", "")).To(Equal(openaicompat.ReasoningModeNone))
		})

		It("UT-KA-1581-014: force_off override always yields none, even for deepseek-reasoner", func() {
			Expect(openaicompat.DetectReasoningMode("deepseek-reasoner", "force_off")).To(Equal(openaicompat.ReasoningModeNone))
		})

		It("UT-KA-1581-015: force_on override yields plain for an otherwise-unrecognized self-hosted model", func() {
			Expect(openaicompat.DetectReasoningMode("my-custom-vllm-deploy", "force_on")).To(Equal(openaicompat.ReasoningModePlain))
		})
	})

	Describe("compatibility floor — minimal OpenAI-compatible server never sees an unexpected field", func() {
		It("UT-KA-1581-016: with ReasoningMode unset (zero value), no reasoning_content field is ever sent", func() {
			client := newTestClient("bare-bones-model", `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages: buildHistory(true),
			})
			Expect(err).NotTo(HaveOccurred())
			messages := receivedBody["messages"].([]interface{})
			assistantMsg := messages[1].(map[string]interface{})
			Expect(assistantMsg).NotTo(HaveKey("reasoning_content"))
		})
	})
})
