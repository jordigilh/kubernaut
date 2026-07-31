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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/geminifamily"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// IT-KA-1778: buildLLMClientFromConfig's Gemini dispatch (BR-AI-087).
//
// Prior to this fix, KA had zero Gemini capability:
//   - provider: vertex_ai unconditionally called anthropicfamily.New
//     regardless of the configured model, silently mis-constructing a
//     Claude-typed client for a gemini-* model (#1778's literal report).
//   - provider: gemini had no case at all and fell through to the
//     default "unsupported provider" error, despite being a validated
//     types.LLMProviderGemini enum value (discovered during preflight).
//
// Both root causes are the same underlying gap and are proven fixed here
// through the real production dispatch switch in buildLLMClientFromConfig
// — not a direct call to the geminifamily constructors (CHECKPOINT W).
var _ = Describe("buildLLMClientFromConfig — Gemini dispatch (#1778 #1792 BR-AI-087)", func() {

	Describe("provider: vertex_ai with a Gemini model (IT-KA-1778-001)", func() {
		It("dispatches to geminifamily.Client, not anthropicfamily.Client", func() {
			cfg := types.LLMConfig{
				Provider:       types.LLMProviderVertexAI,
				Model:          "gemini-2.5-pro",
				VertexProject:  "my-project",
				VertexLocation: "us-central1",
			}

			client, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).To(BeAssignableToTypeOf(&geminifamily.Client{}),
				"vertex_ai + a gemini-* model must construct a geminifamily.Client (#1778 root-cause fix)")
		})

		It("still requires vertexProject (mirrors the anthropicfamily branch's validation)", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderVertexAI,
				Model:    "gemini-2.5-pro",
			}

			_, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("project"))
		})
	})

	Describe("provider: vertex_ai with a Claude model — no regression (IT-KA-1778-002)", func() {
		It("still dispatches to anthropicfamily.Client, unchanged from before this fix", func() {
			cfg := types.LLMConfig{
				Provider:       types.LLMProviderVertexAI,
				Model:          "claude-sonnet-4-6",
				VertexProject:  "my-project",
				VertexLocation: "us-central1",
			}

			client, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).To(BeAssignableToTypeOf(&anthropicfamily.Client{}),
				"vertex_ai + a claude-* model must remain routed to anthropicfamily.Client (scope: narrow, DD-LLM-010)")
		})
	})

	Describe("provider: gemini — native Gemini API dispatch (IT-KA-1778-003)", func() {
		It("dispatches to geminifamily.Client via the API-key constructor", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderGemini,
				Model:    "gemini-2.5-flash",
				APIKey:   "fake-gemini-api-key",
			}

			client, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).To(BeAssignableToTypeOf(&geminifamily.Client{}),
				"provider: gemini must no longer fall through to the \"unsupported provider\" default case")
		})

		It("fails fast when no apiKey is configured, matching NewWithAPIKey's own validation", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderGemini,
				Model:    "gemini-2.5-flash",
			}

			_, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apiKey"))
		})
	})

	Describe("geminiReasoningOptions — cfg.Reasoning -> geminifamily.Option mapping (IT-KA-1778-004)", func() {
		// Mirrors anthropicReasoningOptions' own wiring-gap-fix test above:
		// proves the config->option mapping in isolation, independently of
		// the (untestable without a live network seam) SDK HTTP call —
		// geminifamily's own httptest-based suite proves WithReasoning's
		// wire effect once applied.
		It("produces exactly one WithReasoning option when reasoning is enabled", func() {
			cfg := types.LLMConfig{
				Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "high"},
			}
			Expect(geminiReasoningOptions(cfg)).To(HaveLen(1))
		})

		It("produces no options when Reasoning is nil", func() {
			Expect(geminiReasoningOptions(types.LLMConfig{})).To(BeEmpty())
		})

		It("produces no options when reasoning.enabled is false", func() {
			cfg := types.LLMConfig{Reasoning: &types.LLMReasoningConfig{Enabled: false, Effort: "high"}}
			Expect(geminiReasoningOptions(cfg)).To(BeEmpty())
		})
	})
})
