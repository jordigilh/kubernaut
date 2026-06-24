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

var _ = Describe("Per-Phase LLM Model Routing — #1470", func() {

	Describe("UT-AI-1470-001a: LoadLLMRuntime parses phaseModels from YAML", func() {
		It("should populate PhaseModels map with override config", func() {
			yaml := []byte(`
model: claude-sonnet-4-6
endpoint: http://localhost:11434
phaseModels:
  workflow_discovery:
    model: claude-haiku-3
`)
			rt, err := config.LoadLLMRuntime(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.PhaseModels).To(HaveLen(1))
			Expect(rt.PhaseModels).To(HaveKey("workflow_discovery"))
			Expect(rt.PhaseModels["workflow_discovery"].Model).To(Equal("claude-haiku-3"))
		})
	})

	Describe("UT-AI-1470-001b (SI-10): Validate rejects unknown phase name", func() {
		It("should return error for unrecognized phase key", func() {
			rt := &config.LLMRuntimeConfig{
				Model:    "test-model",
				Endpoint: "http://localhost",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"foo": {Model: "bad-model"},
				},
			}
			err := rt.Validate("openai")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown phase"))
		})
	})

	Describe("UT-AI-1470-001c (SI-10): Validate rejects empty override", func() {
		It("should return error when no override fields are set", func() {
			rt := &config.LLMRuntimeConfig{
				Model:    "test-model",
				Endpoint: "http://localhost",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"workflow_discovery": {},
				},
			}
			err := rt.Validate("openai")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one override field"))
		})
	})

	Describe("UT-AI-1470-001d (CM-6): Validate accepts valid phase model", func() {
		It("should not return error for well-formed phase override", func() {
			rt := &config.LLMRuntimeConfig{
				Model:    "test-model",
				Endpoint: "http://localhost",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"workflow_discovery": {Model: "claude-haiku-3"},
				},
			}
			err := rt.Validate("openai")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-AI-1470-002a: EffectivePhaseConfig returns base when no override", func() {
		It("should return base config unchanged for non-overridden phase", func() {
			rt := &config.LLMRuntimeConfig{
				Model:    "default-model",
				Endpoint: "http://default",
			}
			baseLLM := types.LLMConfig{Provider: "openai"}
			baseRT := config.LLMRuntimeConfig{Model: "base-model", Endpoint: "http://base"}

			outLLM, outRT := rt.EffectivePhaseConfig("rca", baseLLM, baseRT)
			Expect(outLLM.Provider).To(Equal("openai"))
			Expect(outRT.Model).To(Equal("base-model"))
			Expect(outRT.Endpoint).To(Equal("http://base"))
		})
	})

	Describe("UT-AI-1470-002b: EffectivePhaseConfig merges model-only override", func() {
		It("should override only the model field, preserving all others", func() {
			rt := &config.LLMRuntimeConfig{
				Model:    "default-model",
				Endpoint: "http://default",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"workflow_discovery": {Model: "fast-model"},
				},
			}
			baseLLM := types.LLMConfig{Provider: "openai"}
			baseRT := config.LLMRuntimeConfig{Model: "base-model", Endpoint: "http://base", Temperature: 0.7}

			outLLM, outRT := rt.EffectivePhaseConfig("workflow_discovery", baseLLM, baseRT)
			Expect(outLLM.Provider).To(Equal("openai"), "provider should be preserved")
			Expect(outRT.Model).To(Equal("fast-model"), "model should be overridden")
			Expect(outRT.Endpoint).To(Equal("http://base"), "endpoint should be preserved")
			Expect(outRT.Temperature).To(Equal(0.7), "temperature should be preserved")
		})
	})

	Describe("UT-AI-1470-002c: EffectivePhaseConfig merges provider + endpoint override", func() {
		It("should override both static and runtime fields", func() {
			rt := &config.LLMRuntimeConfig{
				Model: "default-model",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"rca": {
						Provider: "anthropic",
						Endpoint: "http://anthropic-api",
						Model:    "sonnet",
					},
				},
			}
			baseLLM := types.LLMConfig{Provider: "openai"}
			baseRT := config.LLMRuntimeConfig{Model: "gpt-4", Endpoint: "http://openai-api"}

			outLLM, outRT := rt.EffectivePhaseConfig("rca", baseLLM, baseRT)
			Expect(outLLM.Provider).To(Equal("anthropic"))
			Expect(outRT.Model).To(Equal("sonnet"))
			Expect(outRT.Endpoint).To(Equal("http://anthropic-api"))
		})
	})

	Describe("UT-AI-1470-002d: EffectivePhaseConfig does not mutate arguments", func() {
		It("should leave original base configs unchanged after merge", func() {
			rt := &config.LLMRuntimeConfig{
				Model: "default-model",
				PhaseModels: map[string]*config.LLMOverrideConfig{
					"workflow_discovery": {Model: "fast-model", Provider: "anthropic"},
				},
			}
			baseLLM := types.LLMConfig{Provider: "openai"}
			baseRT := config.LLMRuntimeConfig{Model: "base-model", Endpoint: "http://base"}

			_, _ = rt.EffectivePhaseConfig("workflow_discovery", baseLLM, baseRT)

			Expect(baseLLM.Provider).To(Equal("openai"), "baseLLM must not be mutated")
			Expect(baseRT.Model).To(Equal("base-model"), "baseRT must not be mutated")
		})
	})
})
