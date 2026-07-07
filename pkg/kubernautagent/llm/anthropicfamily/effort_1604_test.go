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

// Issue #1604 / BR-AI-086: unified Effort knob wiring for the Anthropic
// client family. Effort maps onto genai.ThinkingLevel and is resolved via
// the same adk-anthropic-go/converters.ThinkingConfigToAnthropic call
// resolveThinkingParam already delegates to (DD-LLM-005) — these tests
// additionally cover the OutputConfig.Effort hint that ThinkingMapping
// already computes for adaptive-capable models but which, prior to this
// change, was silently discarded (only .Thinking was read).
var _ = Describe("anthropicfamily Effort knob wiring — #1604", func() {
	var (
		server        *httptest.Server
		receivedBody  map[string]interface{}
		responseBody  string
		newTestClient func(model string) *anthropicfamily.Client
	)

	BeforeEach(func() {
		receivedBody = nil
		responseBody = `{
			"id": "msg_effort_default",
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
			receivedBody = nil
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

	Describe("adaptive-capable model (claude-sonnet-4-6) — Effort surfaces as an output_config.effort hint", func() {
		DescribeTable("maps the canonical Effort value to the matching output_config.effort hint",
			func(effort, wantWireEffort string) {
				client := newTestClient("claude-sonnet-4-6")
				_, err := client.Chat(context.Background(), llm.ChatRequest{
					Messages: []llm.Message{{Role: "user", Content: "hello"}},
					Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true, Effort: effort}},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(receivedBody).To(HaveKey("thinking"))
				thinking := receivedBody["thinking"].(map[string]interface{})
				Expect(thinking["type"]).To(Equal("adaptive"))
				outputConfig, ok := receivedBody["output_config"].(map[string]interface{})
				Expect(ok).To(BeTrue(), "expected an output_config object in the request body")
				Expect(outputConfig["effort"]).To(Equal(wantWireEffort))
			},
			Entry("low", "low", "low"),
			Entry("medium", "medium", "medium"),
			Entry("high", "high", "high"),
			Entry("xhigh clamps to high (genai.ThinkingLevel has no tier above High)", "xhigh", "high"),
		)

		It("UT-KA-1604-101: Effort: minimal turns thinking off entirely (Anthropic has no minimal thinking tier)", func() {
			client := newTestClient("claude-sonnet-4-6")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true, Effort: "minimal"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("thinking"))
			Expect(receivedBody).NotTo(HaveKey("output_config"))
		})

		It("UT-KA-1604-102: no Effort and no BudgetTokens defaults to the zero-regression High tier (adaptive + high effort)", func() {
			client := newTestClient("claude-sonnet-4-6")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true}},
			})
			Expect(err).NotTo(HaveOccurred())
			thinking := receivedBody["thinking"].(map[string]interface{})
			Expect(thinking["type"]).To(Equal("adaptive"))
			outputConfig := receivedBody["output_config"].(map[string]interface{})
			Expect(outputConfig["effort"]).To(Equal("high"))
		})

		It("UT-KA-1604-103: an explicit BudgetTokens always wins over Effort, even on an adaptive-capable model", func() {
			client := newTestClient("claude-sonnet-4-6")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true, Effort: "low", BudgetTokens: 2048}},
			})
			Expect(err).NotTo(HaveOccurred())
			thinking := receivedBody["thinking"].(map[string]interface{})
			Expect(thinking["type"]).To(Equal("enabled"))
			Expect(thinking["budget_tokens"]).To(BeNumerically("==", 2048))
			Expect(receivedBody).NotTo(HaveKey("output_config"),
				"a manual budget takes the non-adaptive path, which has no effort hint")
		})
	})

	Describe("manual-budget-only model (claude-sonnet-4-5) — Effort maps to a manual token budget, no effort hint", func() {
		DescribeTable("maps the canonical Effort value to the matching manual budget",
			func(effort string, wantBudget int) {
				client := newTestClient("claude-sonnet-4-5")
				_, err := client.Chat(context.Background(), llm.ChatRequest{
					Messages: []llm.Message{{Role: "user", Content: "hello"}},
					Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true, Effort: effort}},
				})
				Expect(err).NotTo(HaveOccurred())
				thinking := receivedBody["thinking"].(map[string]interface{})
				Expect(thinking["type"]).To(Equal("enabled"))
				Expect(thinking["budget_tokens"]).To(BeNumerically("==", wantBudget))
				Expect(receivedBody).NotTo(HaveKey("output_config"))
			},
			Entry("low", "low", 1024),
			Entry("medium", "medium", 5000),
			Entry("high", "high", 10000),
			Entry("xhigh clamps to high's budget", "xhigh", 10000),
		)

		It("UT-KA-1604-104: Effort: minimal turns thinking off entirely on a manual-only model too", func() {
			client := newTestClient("claude-sonnet-4-5")
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true, Effort: "minimal"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("thinking"))
		})
	})
})
