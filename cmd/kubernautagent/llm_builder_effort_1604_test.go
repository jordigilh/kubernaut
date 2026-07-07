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

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/anthropics/anthropic-sdk-go/option"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Issue #1604 / BR-AI-086: unified Effort knob config->option wiring for
// both LLM client families, closing the same class of wiring gap #1601
// closed for cfg.Reasoning.Enabled/BudgetTokens on Anthropic.
var _ = Describe("buildLLMClientFromConfig — Effort knob wiring (#1604)", func() {

	Describe("anthropicReasoningOptions threads Effort through to WithReasoning", func() {
		var (
			server       *httptest.Server
			receivedBody map[string]interface{}
		)

		BeforeEach(func() {
			receivedBody = nil
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"claude-sonnet-4-6","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("sends the configured Effort as an output_config.effort hint on the wire", func() {
			cfg := types.LLMConfig{
				Provider:  types.LLMProviderAnthropic,
				Model:     "claude-sonnet-4-6",
				APIKey:    "sk-ant-fake-test-key",
				Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "low"},
			}

			opts := append(anthropicReasoningOptions(cfg), anthropicfamily.WithSDKOptions(option.WithBaseURL(server.URL)))
			client, err := anthropicfamily.NewWithAPIKey(cfg.APIKey, cfg.Model, opts...)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.Chat(context.Background(), helloChatRequest())
			Expect(err).NotTo(HaveOccurred())

			outputConfig := receivedBody["output_config"].(map[string]interface{})
			Expect(outputConfig["effort"]).To(Equal("low"))
		})
	})

	Describe("openaiReasoningOptions — cfg.Reasoning -> kaopenai.Option mapping", func() {
		It("produces exactly one WithReasoning option when reasoning is enabled", func() {
			cfg := types.LLMConfig{
				Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "high"},
			}
			Expect(openaiReasoningOptions(cfg)).To(HaveLen(1))
		})

		It("produces no options when Reasoning is nil (no regression)", func() {
			Expect(openaiReasoningOptions(types.LLMConfig{})).To(BeEmpty())
		})

		It("produces no options when reasoning.enabled is false", func() {
			cfg := types.LLMConfig{
				Reasoning: &types.LLMReasoningConfig{Enabled: false, Effort: "high"},
			}
			Expect(openaiReasoningOptions(cfg)).To(BeEmpty())
		})
	})

	Describe("buildOpenAICompatClient — request-side wiring end-to-end: cfg.Reasoning.Effort -> outgoing reasoning_effort", func() {
		var (
			server       *httptest.Server
			receivedBody map[string]interface{}
		)

		BeforeEach(func() {
			receivedBody = nil
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("sends reasoning_effort on the wire when the operator's config has reasoning.enabled=true and effort set", func() {
			cfg := types.LLMConfig{
				Provider:  types.LLMProviderOpenAI,
				Model:     "gpt-5",
				Endpoint:  server.URL,
				APIKey:    "sk-fake-test-key",
				Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "medium"},
			}

			client, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).To(BeAssignableToTypeOf(&kaopenai.Client{}))

			_, err = client.Chat(context.Background(), helloChatRequest())
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody["reasoning_effort"]).To(Equal("medium"),
				"operator config ai.llm.reasoning.effort must reach the outgoing OpenAI request")
		})

		It("omits reasoning_effort when the operator's config has no reasoning block (no regression)", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderOpenAI,
				Model:    "gpt-5",
				Endpoint: server.URL,
				APIKey:   "sk-fake-test-key",
			}

			client, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.Chat(context.Background(), helloChatRequest())
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
		})
	})
})
