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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Per-Phase Reasoning/Effort Overrides — #1616, BR-AI-086.
//
// Root cause (confirmed during due diligence): LLMOverrideConfig had no
// Reasoning field at all, so EffectivePhaseConfig never overrode it and
// every phase silently inherited base ai.llm.reasoning regardless of
// phaseModels. See docs/tests/1616/TEST_PLAN.md and IMPLEMENTATION_PLAN.md.
var _ = Describe("Per-Phase Reasoning/Effort Overrides — #1616", func() {

	Describe("UT-AI-1616-001 (CM-6): EffectivePhaseConfig applies the phase override's Reasoning when set", func() {
		It("should override Reasoning, not inherit the base's", func() {
			rt := &config.LLMRuntimeConfig{
				Model: "default-model",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"rca": {
						Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "high"},
					},
				},
			}
			baseLLM := types.LLMConfig{
				Provider:  "anthropic",
				Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "low"},
			}
			baseRT := config.LLMRuntimeConfig{Model: "base-model"}

			outLLM, _ := rt.EffectivePhaseConfig("rca", baseLLM, baseRT)

			Expect(outLLM.Reasoning).NotTo(BeNil())
			Expect(outLLM.Reasoning.Effort).To(Equal("high"), "the phase override's Reasoning must win over the base's")
			Expect(outLLM.Reasoning.Enabled).To(BeTrue())
		})
	})

	Describe("UT-AI-1616-002 (CM-6): EffectivePhaseConfig falls back to base Reasoning when the override has none", func() {
		It("should leave base Reasoning unchanged when the phase override does not set it (regression, mirrors UT-AI-1470-002a)", func() {
			rt := &config.LLMRuntimeConfig{
				Model: "default-model",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"workflow_discovery": {Model: "fast-model"},
				},
			}
			baseReasoning := &types.LLMReasoningConfig{Enabled: true, Effort: "medium"}
			baseLLM := types.LLMConfig{Provider: "openai", Reasoning: baseReasoning}
			baseRT := config.LLMRuntimeConfig{Model: "base-model"}

			outLLM, _ := rt.EffectivePhaseConfig("workflow_discovery", baseLLM, baseRT)

			Expect(outLLM.Reasoning).To(Equal(baseReasoning), "base Reasoning must be preserved when the override doesn't set it")
		})
	})

	Describe("UT-AI-1616-003 (CM-6): a reasoning-only phase override is not rejected as empty", func() {
		It("should accept a phase override that sets only reasoning", func() {
			rt := &config.LLMRuntimeConfig{
				Model:    "test-model",
				Endpoint: "http://localhost",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"rca": {
						Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "high"},
					},
				},
			}
			Expect(rt.Validate("openai")).To(Succeed(), "a reasoning-only override sets a real field and must not be rejected as an empty no-op override")
		})
	})

	Describe("UT-AI-1616-004 (SI-10): Validate rejects an invalid effort value on a phase override", func() {
		It("should reject an unrecognized effort value", func() {
			rt := &config.LLMRuntimeConfig{
				Model:    "test-model",
				Endpoint: "http://localhost",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"rca": {
						Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "extreme"},
					},
				},
			}
			err := rt.Validate("openai")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("effort"))
		})
	})

	Describe("UT-AI-1616-005 (SI-10): Validate rejects the Anthropic none+enabled contradiction on a phase override's effective provider", func() {
		It("should reject effort: none + enabled: true when the override inherits an Anthropic-family base provider", func() {
			rt := &config.LLMRuntimeConfig{
				Model:    "test-model",
				Endpoint: "http://localhost",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"rca": {
						// No override.Provider set: effective provider must
						// fall back to the base provider passed to Validate.
						Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "none"},
					},
				},
			}
			err := rt.Validate("anthropic")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("effort: none"))
		})

		It("should accept effort: none + enabled: true when the override's own provider is not Anthropic-family, even if base is", func() {
			rt := &config.LLMRuntimeConfig{
				Model:    "test-model",
				Endpoint: "http://localhost",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"rca": {
						Provider:  "openai",
						Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "none"},
					},
				},
			}
			Expect(rt.Validate("anthropic")).To(Succeed(), "the override's own provider (openai) is the effective provider, not the base's (anthropic)")
		})
	})
})
