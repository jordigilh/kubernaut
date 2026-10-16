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
	"k8s.io/utils/ptr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("complete profile reasoning configuration — #1616", func() {
	It("UT-AI-1616-001 resolves a phase profile's reasoning settings", func() {
		profile := &types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "claude-sonnet-4-6",
			VertexProject:  "workflow-project",
			VertexLocation: "us-central1",
			Reasoning:      &types.LLMReasoningConfig{Enabled: true, Effort: "high"},
		}

		resolved, err := config.ResolveEffectiveLLM(
			types.LLMConfig{Provider: types.LLMProviderVertexAI, VertexProject: "base-project", VertexLocation: "us-central1", Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "low"}},
			config.LLMRuntimeConfig{Model: "base-model", Temperature: ptr.To(0.7)},
			profile,
			"phaseModels[\"rca\"]",
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Reasoning).NotTo(BeNil())
		Expect(resolved.Reasoning.Effort).To(Equal("high"))
	})

	It("UT-AI-1616-002 validates reasoning against the complete profile provider", func() {
		rt := &config.LLMRuntimeConfig{
			Model:    "base-model",
			Endpoint: "http://base",
			PhaseModels: map[string]*types.LLMConfig{
				"rca": {
					Provider:   types.LLMProviderAnthropic,
					Model:      "claude-sonnet-4-6",
					Reasoning:  &types.LLMReasoningConfig{Enabled: true, Effort: "none"},
					APIKeyFile: "/etc/credentials/anthropic",
				},
			},
		}

		err := rt.Validate(types.LLMProviderOpenAICompatible)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("effort: none"))
	})

	It("UT-AI-1616-003 accepts a complete reasoning profile for a non-Anthropic provider", func() {
		rt := &config.LLMRuntimeConfig{
			Model:    "base-model",
			Endpoint: "http://base",
			PhaseModels: map[string]*types.LLMConfig{
				"rca": {
					Provider:   types.LLMProviderOpenAICompatible,
					Model:      "gpt-5",
					Endpoint:   "http://workflow",
					APIKeyFile: "/etc/credentials/openai",
					Reasoning:  &types.LLMReasoningConfig{Enabled: true, Effort: "high"},
				},
			},
		}

		Expect(rt.Validate(types.LLMProviderOpenAICompatible)).To(Succeed())
	})
})
