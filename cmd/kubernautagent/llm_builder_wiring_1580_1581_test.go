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

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// helloChatRequest is a minimal single-turn request shared by the
// request-side wiring specs below; its content is irrelevant — only the
// resulting outgoing "thinking" param (or its absence) is asserted.
func helloChatRequest() llm.ChatRequest {
	return llm.ChatRequest{Messages: []llm.Message{{Role: "user", Content: "hello"}}}
}

var _ = Describe("buildLLMClientFromConfig — provider dispatch wiring (#1578 #1580 #1581)", func() {

	Describe("anthropicReasoningOptions — cfg.Reasoning -> anthropicfamily.Option mapping (IT-KA-1578-001)", func() {
		// The exact mapping used by both buildAnthropicNativeClient and the
		// vertex_ai case in buildLLMClientFromConfig, proven independently of
		// the (untestable without a live network seam) SDK HTTP call.
		// anthropicfamily's own httptest-based suite (thinking_1580_test.go,
		// UT-KA-1578-201..204) proves WithReasoning's effect once applied;
		// this proves the config->option mapping that used to be entirely
		// missing (#1578 wiring-gap fix).

		It("produces exactly one WithReasoning option when reasoning is enabled", func() {
			cfg := types.LLMConfig{
				Reasoning: &types.LLMReasoningConfig{Enabled: true, BudgetTokens: 8192},
			}
			opts := anthropicReasoningOptions(cfg)
			Expect(opts).To(HaveLen(1))
		})

		It("produces no options when Reasoning is nil (no regression for existing deployments)", func() {
			cfg := types.LLMConfig{}
			opts := anthropicReasoningOptions(cfg)
			Expect(opts).To(BeEmpty())
		})

		It("produces no options when reasoning.enabled is false", func() {
			cfg := types.LLMConfig{
				Reasoning: &types.LLMReasoningConfig{Enabled: false, BudgetTokens: 8192},
			}
			opts := anthropicReasoningOptions(cfg)
			Expect(opts).To(BeEmpty())
		})
	})

	Describe("buildAnthropicNativeClient — reasoning-enabled config dispatch (IT-KA-1578-002)", func() {
		// Proves buildAnthropicNativeClient dispatches a reasoning-enabled
		// config through the real production construction path without error
		// (CHECKPOINT W row: config -> client construction).
		It("constructs a client without error and applies the reasoning option", func() {
			cfg := types.LLMConfig{
				Provider:  types.LLMProviderAnthropic,
				Model:     "claude-sonnet-4-6",
				APIKey:    "sk-ant-fake-test-key",
				Reasoning: &types.LLMReasoningConfig{Enabled: true, BudgetTokens: 2048},
			}

			client, err := buildAnthropicNativeClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).To(BeAssignableToTypeOf(&anthropicfamily.Client{}))
		})
	})

	Describe("request-side wiring end-to-end: cfg.Reasoning -> outgoing \"thinking\" param (IT-KA-1578-003)", func() {
		// Closes the full chain that the two Describe blocks above prove in
		// isolation: this test replicates buildAnthropicNativeClient's exact
		// option composition (anthropicReasoningOptions(cfg), the same
		// unexported helper the production dispatch path calls) against a
		// live httptest server, and asserts the real outgoing HTTP body
		// contains "thinking". buildAnthropicNativeClient itself cannot be
		// redirected to a test server in production code (Anthropic's native
		// API has no operator-facing "endpoint override" concept — unlike
		// openai_compatible, see buildOpenAICompatClient — so adding one
		// purely for testability would be speculative code with no real
		// deployment use case). anthropicfamily.WithSDKOptions is already
		// documented as test-only for exactly this purpose.
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

		It("sends the thinking param on the wire when the operator's config has reasoning.enabled=true", func() {
			cfg := types.LLMConfig{
				Provider:  types.LLMProviderAnthropic,
				Model:     "claude-sonnet-4-6",
				APIKey:    "sk-ant-fake-test-key",
				Reasoning: &types.LLMReasoningConfig{Enabled: true, BudgetTokens: 4096},
			}

			opts := append(anthropicReasoningOptions(cfg), anthropicfamily.WithSDKOptions(option.WithBaseURL(server.URL)))
			client, err := anthropicfamily.NewWithAPIKey(cfg.APIKey, cfg.Model, opts...)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.Chat(context.Background(), helloChatRequest())
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedBody).To(HaveKey("thinking"),
				"operator config ai.llm.reasoning.enabled=true must reach the outgoing Anthropic request")
			thinking := receivedBody["thinking"].(map[string]interface{})
			Expect(thinking["budget_tokens"]).To(BeNumerically("==", 4096))
		})

		It("omits the thinking param when the operator's config has no reasoning block (no regression)", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderAnthropic,
				Model:    "claude-sonnet-4-6",
				APIKey:   "sk-ant-fake-test-key",
			}

			opts := append(anthropicReasoningOptions(cfg), anthropicfamily.WithSDKOptions(option.WithBaseURL(server.URL)))
			client, err := anthropicfamily.NewWithAPIKey(cfg.APIKey, cfg.Model, opts...)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.Chat(context.Background(), helloChatRequest())
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedBody).NotTo(HaveKey("thinking"))
		})
	})

	Describe("buildLLMClientFromConfig — provider anthropic dispatch (IT-KA-1580-001)", func() {
		// buildLLMClientFromConfig dispatches provider "anthropic"
		// (LLMProviderAnthropic, native API-key auth) to
		// anthropicfamily.NewWithAPIKey through the actual production switch
		// statement — not just a direct call to the constructor in a
		// package-local test (CHECKPOINT W, Wiring Manifest row 1).
		It("dispatches to the native anthropicfamily.Client constructor", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderAnthropic,
				Model:    "claude-sonnet-4-6",
				APIKey:   "sk-ant-fake-test-key",
			}

			client, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).To(BeAssignableToTypeOf(&anthropicfamily.Client{}))
		})

		// UT-KA-1580-001b: the anthropic provider case fails fast with no API
		// key, matching NewWithAPIKey's own validation (production dispatch
		// must not silently construct a client that will fail on first use).
		It("fails fast when no apiKey is configured", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderAnthropic,
				Model:    "claude-sonnet-4-6",
			}

			_, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("buildLLMClientFromConfig — provider openai/openai_compatible dispatch (IT-KA-1581-002)", func() {
		// buildLLMClientFromConfig dispatches providers "openai" and
		// "openai_compatible" to the shared-core-backed kaopenai.Client
		// through the actual production switch statement (CHECKPOINT W,
		// Wiring Manifest row 3).
		var server *httptest.Server

		BeforeEach(func() {
			server = httptest.NewServer(nil)
		})

		AfterEach(func() {
			server.Close()
		})

		DescribeTable("dispatches to the shared-core kaopenai.Client wrapper",
			func(provider string) {
				cfg := types.LLMConfig{
					Provider: provider,
					Model:    "gpt-4o",
					Endpoint: server.URL,
					APIKey:   "sk-fake-test-key",
				}

				client, err := buildLLMClientFromConfig(context.Background(), cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(client).To(BeAssignableToTypeOf(&kaopenai.Client{}))
			},
			Entry("provider=openai", types.LLMProviderOpenAI),
			Entry("provider=openai_compatible", types.LLMProviderOpenAICompatible),
		)
	})

	Describe("buildLLMClientFromConfig — unsupported provider (UT-KA-1581-002b)", func() {
		// An unrecognized provider (Gemini native, never had a langchaingo
		// case either) still falls through to the default branch and gets an
		// explicit error, proving the anthropic/openai cases above are
		// additive and the removed-langchaingo default path fails loudly
		// rather than silently.
		It("returns an explicit error for gemini: no langchaingo fallback exists anymore", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderGemini,
				Model:    "gemini-not-yet-migrated",
			}

			_, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).To(HaveOccurred())
		})
	})
})
