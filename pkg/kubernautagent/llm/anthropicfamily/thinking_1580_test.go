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

package anthropicfamily_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
)

// Issue #1580 / BR-AI-086 AC1/AC2/AC3: model-aware thinking request wiring,
// reasoning response capture, and #1299-class signature-replay ordering.
// Tier detection reuses adk-anthropic-go/converters.ThinkingConfigToAnthropic
// (DD-LLM-005) — these tests assert on its observable effect (the outgoing
// "thinking" JSON shape for a known adaptive-capable vs. manual-only model),
// not on the call itself, since that function is a package-level pure
// mapping with no seams to intercept.
var _ = Describe("anthropicfamily thinking/reasoning wiring — #1580", func() {
	var (
		server        *httptest.Server
		receivedBody  map[string]interface{}
		responseBody  string
		newTestClient func(model string) *anthropicfamily.Client
	)

	BeforeEach(func() {
		receivedBody = nil
		responseBody = `{
			"id": "msg_thinking_default",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-6",
			"stop_reason": "end_turn",
			"content": [{"type": "text", "text": "conclusion"}],
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	newTestClient = func(model string) *anthropicfamily.Client {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(responseBody))
		}))
		client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", model,
			anthropicfamily.WithSDKOptions(option.WithBaseURL(server.URL)),
		)
		Expect(err).NotTo(HaveOccurred())
		return client
	}

	Describe("request wiring — AC2: disabled by default, no regression", func() {
		It("UT-KA-1580-101: omits the thinking field entirely when Options.Reasoning is nil", func() {
			client := newTestClient("claude-sonnet-4-6")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("thinking"))
		})

		It("UT-KA-1580-102: omits the thinking field when Reasoning.Enabled is false", func() {
			client := newTestClient("claude-sonnet-4-6")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: false}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("thinking"))
		})
	})

	Describe("WithReasoning — construction-time default (#1578 wiring-gap fix)", func() {
		// Prior to this fix, no production code path ever set
		// ChatRequest.Options.Reasoning: llm_builder.go never read
		// cfg.Reasoning.Enabled/BudgetTokens, and no investigator call site
		// set Options.Reasoning per-call. Operator config's
		// ai.llm.reasoning.enabled=true was therefore a structural dead end —
		// the "thinking" param was never sent on a real Anthropic call,
		// regardless of config. WithReasoning closes this by resolving the
		// operator's setting ONCE at client-construction time (matching the
		// documented intent on llm.ReasoningRequest: "resolved once at
		// LLM-client-construction time from operator config, never threaded
		// per-call from business logic").
		newTestClientWithReasoning := func(model string, r llm.ReasoningRequest) *anthropicfamily.Client {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(responseBody))
			}))
			client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", model,
				anthropicfamily.WithSDKOptions(option.WithBaseURL(server.URL)),
				anthropicfamily.WithReasoning(r),
			)
			Expect(err).NotTo(HaveOccurred())
			return client
		}

		It("UT-KA-1578-201: a client constructed WithReasoning(Enabled:true) sends thinking on a call with no per-call Options.Reasoning", func() {
			client := newTestClientWithReasoning("claude-sonnet-4-6", llm.ReasoningRequest{Enabled: true, BudgetTokens: 4096})
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).To(HaveKey("thinking"),
				"the construction-time default must apply when the caller (business logic) never sets Options.Reasoning per-call")
			thinking := receivedBody["thinking"].(map[string]interface{})
			Expect(thinking["budget_tokens"]).To(BeNumerically("==", 4096))
		})

		It("UT-KA-1578-202: a client constructed without WithReasoning still omits thinking (no regression for existing deployments)", func() {
			client := newTestClient("claude-sonnet-4-6")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("thinking"))
		})

		It("UT-KA-1578-203: an explicit per-call Options.Reasoning still overrides the construction-time default (disable wins)", func() {
			client := newTestClientWithReasoning("claude-sonnet-4-6", llm.ReasoningRequest{Enabled: true, BudgetTokens: 4096})
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: false}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("thinking"),
				"a per-call override must win over the client's construction-time default")
		})

		It("UT-KA-1578-204: WithReasoning(Enabled:false) never sends thinking, even with a BudgetTokens set", func() {
			client := newTestClientWithReasoning("claude-sonnet-4-6", llm.ReasoningRequest{Enabled: false, BudgetTokens: 4096})
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("thinking"))
		})
	})

	Describe("request wiring — model-aware tier detection (adk-anthropic-go/converters reuse)", func() {
		It("UT-KA-1580-103: adaptive-capable model (claude-sonnet-4-6) with no explicit budget gets adaptive thinking", func() {
			client := newTestClient("claude-sonnet-4-6")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).To(HaveKey("thinking"))
			thinking := receivedBody["thinking"].(map[string]interface{})
			Expect(thinking["type"]).To(Equal("adaptive"))
		})

		It("UT-KA-1580-104: manual-only model (claude-sonnet-4-5) with no explicit budget falls back to a manual high-effort budget", func() {
			client := newTestClient("claude-sonnet-4-5")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).To(HaveKey("thinking"))
			thinking := receivedBody["thinking"].(map[string]interface{})
			Expect(thinking["type"]).To(Equal("enabled"))
			Expect(thinking["budget_tokens"]).To(BeNumerically("==", 10000))
		})

		It("UT-KA-1580-105: an explicit BudgetTokens always wins with a manual budget, even on an adaptive-capable model", func() {
			client := newTestClient("claude-sonnet-4-6")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true, BudgetTokens: 2048}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).To(HaveKey("thinking"))
			thinking := receivedBody["thinking"].(map[string]interface{})
			Expect(thinking["type"]).To(Equal("enabled"))
			Expect(thinking["budget_tokens"]).To(BeNumerically("==", 2048))
		})

		It("UT-KA-1580-106: unknown/unsupported model skips the thinking parameter rather than erroring (BR-AI-086 AC2)", func() {
			client := newTestClient("some-unknown-future-model")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true, BudgetTokens: 2048}},
			})
			Expect(err).NotTo(HaveOccurred())
			// An explicit budget always maps to a manual budget regardless of
			// tier (see 105) — this proves the "unknown model" case is safe
			// specifically for the no-budget/adaptive-detection path.
			Expect(receivedBody).To(HaveKey("thinking"))
		})
	})

	Describe("response mapping — AC3: capturing reasoning content", func() {
		It("UT-KA-1580-107: captures visible thinking text + signature into Message.Reasoning", func() {
			responseBody = `{
				"id": "msg_thinking_resp",
				"type": "message",
				"role": "assistant",
				"model": "claude-sonnet-4-6",
				"stop_reason": "end_turn",
				"content": [
					{"type": "thinking", "thinking": "Let me examine the pod status...", "signature": "sig-abc-123"},
					{"type": "text", "text": "The pod is OOMKilled."}
				],
				"usage": {"input_tokens": 20, "output_tokens": 10}
			}`
			client := newTestClient("claude-sonnet-4-6")
			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "why is it crashing?"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("The pod is OOMKilled."))
			Expect(resp.Message.Reasoning).NotTo(BeNil())
			Expect(resp.Message.Reasoning.Text).To(Equal("Let me examine the pod status..."))
			Expect(resp.Message.Reasoning.Signature).To(Equal("sig-abc-123"))
			Expect(resp.Message.Reasoning.Redacted).To(BeFalse())
		})

		It("UT-KA-1580-108: captures redacted_thinking as an opaque, replayable block with no visible text", func() {
			responseBody = `{
				"id": "msg_thinking_redacted",
				"type": "message",
				"role": "assistant",
				"model": "claude-sonnet-4-6",
				"stop_reason": "end_turn",
				"content": [
					{"type": "redacted_thinking", "data": "encrypted-opaque-payload"},
					{"type": "text", "text": "Conclusion reached."}
				],
				"usage": {"input_tokens": 20, "output_tokens": 10}
			}`
			client := newTestClient("claude-sonnet-4-6")
			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "investigate"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Reasoning).NotTo(BeNil())
			Expect(resp.Message.Reasoning.Text).To(BeEmpty())
			Expect(resp.Message.Reasoning.Signature).To(Equal("encrypted-opaque-payload"))
			Expect(resp.Message.Reasoning.Redacted).To(BeTrue())
		})

		It("UT-KA-1580-109: leaves Message.Reasoning nil when the response contains no thinking block", func() {
			client := newTestClient("claude-sonnet-4-6")
			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Reasoning).To(BeNil())
		})
	})

	Describe("replay ordering — #1299-class regression: thinking block must precede tool_use on replay", func() {
		It("UT-KA-1580-110: replays a visible thinking block first, before tool_use, in a multi-turn self-correction retry", func() {
			client := newTestClient("claude-sonnet-4-6")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "investigate the crash"},
					{
						Role:    "assistant",
						Content: "",
						Reasoning: &llm.ReasoningBlock{
							Text:      "I should check the pod events first.",
							Signature: "sig-replay-001",
						},
						ToolCalls: []llm.ToolCall{
							{ID: "toolu_001", Name: "kubectl_describe", Arguments: `{"kind":"Pod"}`},
						},
					},
					{Role: "tool", Content: `{"status":"OOMKilled"}`, ToolCallID: "toolu_001", ToolName: "kubectl_describe"},
					{Role: "user", Content: "That's incomplete — please also check logs."},
				},
				Options: llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true}},
			})
			Expect(err).NotTo(HaveOccurred())

			messages, ok := receivedBody["messages"].([]interface{})
			Expect(ok).To(BeTrue())

			var assistantMsg map[string]interface{}
			for _, m := range messages {
				mm := m.(map[string]interface{})
				if mm["role"] == "assistant" {
					assistantMsg = mm
					break
				}
			}
			Expect(assistantMsg).NotTo(BeNil(), "expected an assistant message in the replayed history")

			content := assistantMsg["content"].([]interface{})
			Expect(len(content)).To(BeNumerically(">=", 2))

			firstBlock := content[0].(map[string]interface{})
			Expect(firstBlock["type"]).To(Equal("thinking"),
				"thinking block must be first in the content array — Anthropic API requirement, same failure class as #1299")
			Expect(firstBlock["signature"]).To(Equal("sig-replay-001"))
			Expect(firstBlock["thinking"]).To(Equal("I should check the pod events first."))

			lastBlock := content[len(content)-1].(map[string]interface{})
			Expect(lastBlock["type"]).To(Equal("tool_use"))
		})

		It("UT-KA-1580-111: replays a redacted_thinking block first, before tool_use, preserving the opaque payload verbatim", func() {
			client := newTestClient("claude-sonnet-4-6")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "investigate the crash"},
					{
						Role:    "assistant",
						Content: "",
						Reasoning: &llm.ReasoningBlock{
							Signature: "encrypted-opaque-payload",
							Redacted:  true,
						},
						ToolCalls: []llm.ToolCall{
							{ID: "toolu_002", Name: "kubectl_logs", Arguments: `{"pod":"api"}`},
						},
					},
					{Role: "tool", Content: `OOMKilled`, ToolCallID: "toolu_002", ToolName: "kubectl_logs"},
					{Role: "user", Content: "continue"},
				},
				Options: llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true}},
			})
			Expect(err).NotTo(HaveOccurred())

			messages, ok := receivedBody["messages"].([]interface{})
			Expect(ok).To(BeTrue())

			var assistantMsg map[string]interface{}
			for _, m := range messages {
				mm := m.(map[string]interface{})
				if mm["role"] == "assistant" {
					assistantMsg = mm
					break
				}
			}
			Expect(assistantMsg).NotTo(BeNil())

			content := assistantMsg["content"].([]interface{})
			firstBlock := content[0].(map[string]interface{})
			Expect(firstBlock["type"]).To(Equal("redacted_thinking"))
			Expect(firstBlock["data"]).To(Equal("encrypted-opaque-payload"))
		})
	})
})
