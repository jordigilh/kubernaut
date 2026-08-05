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

package vertexanthropic_test

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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/vertexanthropic"
)

// Issue #1935 / BR-AI-086 AC1/AC3: backport of main's anthropicfamily
// thinking/reasoning capture-and-replay mechanism (pkg/kubernautagent/llm/
// anthropicfamily/thinking_1580_test.go, UT-KA-1580-107..111) into v1.5's
// separate vertexanthropic client. root cause of #1935: claude-sonnet-5
// emits a signed "thinking" content block by default; this client silently
// dropped it, corrupting gate-retry replay history (#1299-class regression)
// and producing false "no tool access" claims in due-diligence narratives.
//
// Only the capture (mapResponse) and replay (buildParams) mechanism is
// ported here — NOT main's opt-in request-side thinking configuration
// (WithReasoning/ReasoningRequest/effort dials), which is out of scope for
// this targeted bug-fix backport. v1.5's llm.ChatOptions has no Reasoning
// field; these tests assert on whatever the model spontaneously returns.
var _ = Describe("vertexanthropic thinking/reasoning round-trip — #1935", func() {
	var (
		server        *httptest.Server
		tokenSrv      *httptest.Server
		receivedBody  map[string]interface{}
		responseBody  string
		newTestClient func(model string) *vertexanthropic.Client
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
		if tokenSrv != nil {
			tokenSrv.Close()
		}
	})

	newTestClient = func(model string) *vertexanthropic.Client {
		tokenSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fake-token","token_type":"Bearer","expires_in":3600}`))
		}))
		fakeCreds := generateFakeServiceAccountJSONWithTokenURL(tokenSrv.URL)

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(responseBody))
		}))
		client, err := vertexanthropic.New(context.Background(),
			model, fakeCreds, "test-project", "us-central1",
			vertexanthropic.WithSDKOptions(option.WithBaseURL(server.URL)),
		)
		Expect(err).NotTo(HaveOccurred())
		return client
	}

	Describe("response mapping — captures reasoning content (ported from UT-KA-1580-107/108/109)", func() {
		It("UT-KA-1935-001: captures visible thinking text + signature into Message.Reasoning", func() {
			responseBody = `{
				"id": "msg_thinking_resp",
				"type": "message",
				"role": "assistant",
				"model": "claude-sonnet-5",
				"stop_reason": "end_turn",
				"content": [
					{"type": "thinking", "thinking": "Let me examine the pod status...", "signature": "sig-abc-123"},
					{"type": "text", "text": "The pod is OOMKilled."}
				],
				"usage": {"input_tokens": 20, "output_tokens": 10}
			}`
			client := newTestClient("claude-sonnet-5")
			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "why is it crashing?"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("The pod is OOMKilled."))
			Expect(resp.Message.Reasoning).NotTo(BeNil())
			Expect(resp.Message.Reasoning.Text).To(Equal("Let me examine the pod status..."))
			Expect(resp.Message.Reasoning.Signature).To(Equal("sig-abc-123"))
			Expect(resp.Message.Reasoning.Redacted).To(BeFalse())
		})

		It("UT-KA-1935-002: captures redacted_thinking as an opaque, replayable block with no visible text", func() {
			responseBody = `{
				"id": "msg_thinking_redacted",
				"type": "message",
				"role": "assistant",
				"model": "claude-sonnet-5",
				"stop_reason": "end_turn",
				"content": [
					{"type": "redacted_thinking", "data": "encrypted-opaque-payload"},
					{"type": "text", "text": "Conclusion reached."}
				],
				"usage": {"input_tokens": 20, "output_tokens": 10}
			}`
			client := newTestClient("claude-sonnet-5")
			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "investigate"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Reasoning).NotTo(BeNil())
			Expect(resp.Message.Reasoning.Text).To(BeEmpty())
			Expect(resp.Message.Reasoning.Signature).To(Equal("encrypted-opaque-payload"))
			Expect(resp.Message.Reasoning.Redacted).To(BeTrue())
		})

		It("UT-KA-1935-003: leaves Message.Reasoning nil when the response contains no thinking block", func() {
			client := newTestClient("claude-sonnet-4-6")
			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Reasoning).To(BeNil())
		})
	})

	Describe("replay ordering — #1299-class regression: thinking block must precede tool_use on replay (ported from UT-KA-1580-110/111)", func() {
		It("UT-KA-1935-004: replays a visible thinking block first, before tool_use, in a multi-turn self-correction retry", func() {
			client := newTestClient("claude-sonnet-5")
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

		It("UT-KA-1935-005: replays a redacted_thinking block first, before tool_use, preserving the opaque payload verbatim", func() {
			client := newTestClient("claude-sonnet-5")
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

			lastBlock := content[len(content)-1].(map[string]interface{})
			Expect(lastBlock["type"]).To(Equal("tool_use"))
		})
	})
})
