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
	"k8s.io/utils/ptr"
)

var _ = Describe("complete phase LLM profiles — #1470", func() {
	It("UT-AI-1470-001 parses a complete phase profile from YAML", func() {
		rt, err := config.LoadLLMRuntime([]byte(`
model: claude-sonnet-4-6
endpoint: http://localhost:11434
phaseModels:
  workflow_discovery:
    provider: openai_compatible
    model: claude-haiku-3
    endpoint: http://workflow:11434
    apiKeyFile: /etc/credentials/workflow
`))

		Expect(err).NotTo(HaveOccurred())
		Expect(rt.PhaseModels).To(HaveLen(1))
		Expect(rt.PhaseModels["workflow_discovery"].Provider).To(Equal(types.LLMProviderOpenAICompatible))
		Expect(rt.PhaseModels["workflow_discovery"].Model).To(Equal("claude-haiku-3"))
	})

	It("UT-AI-1470-001b rejects an unknown phase name", func() {
		rt := &config.LLMRuntimeConfig{
			Model:    "test-model",
			Endpoint: "http://localhost",
			PhaseModels: map[string]*types.LLMConfig{
				"foo": {Provider: types.LLMProviderOpenAICompatible, Model: "bad-model", Endpoint: "http://bad", APIKeyFile: "/etc/key"},
			},
		}

		err := rt.Validate(types.LLMProviderOpenAICompatible)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown phase"))
	})

	It("UT-AI-1470-001c rejects a nil phase profile", func() {
		rt := &config.LLMRuntimeConfig{
			Model:       "test-model",
			Endpoint:    "http://localhost",
			PhaseModels: map[string]*types.LLMConfig{"workflow_discovery": nil},
		}

		err := rt.Validate(types.LLMProviderOpenAICompatible)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("complete profile"))
	})

	It("UT-AI-1470-001d accepts a complete phase profile", func() {
		rt := &config.LLMRuntimeConfig{
			Model:    "test-model",
			Endpoint: "http://localhost",
			PhaseModels: map[string]*types.LLMConfig{
				"workflow_discovery": {
					Provider:   types.LLMProviderOpenAICompatible,
					Model:      "claude-haiku-3",
					Endpoint:   "http://workflow",
					APIKeyFile: "/etc/credentials/workflow",
				},
			},
		}

		Expect(rt.Validate(types.LLMProviderOpenAICompatible)).To(Succeed())
	})

	It("UT-AI-1470-002 resolves the base profile from static and runtime fields", func() {
		resolved, err := config.ResolveEffectiveLLM(
			types.LLMConfig{Provider: types.LLMProviderVertexAI, VertexProject: "base-project", VertexLocation: "us-central1"},
			config.LLMRuntimeConfig{Model: "base-model", Endpoint: "http://base", Temperature: ptr.To(0.7)},
			nil,
			"ai.llm",
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Provider).To(Equal(types.LLMProviderVertexAI))
		Expect(resolved.Model).To(Equal("base-model"))
		Expect(resolved.Endpoint).To(Equal("http://base"))
		Expect(resolved.VertexProject).To(Equal("base-project"))
		Expect(resolved.Temperature).NotTo(BeNil())
	})

	It("UT-AI-1470-003 uses the complete phase profile instead of inheriting base fields", func() {
		profile := &types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "workflow-model",
			VertexProject:  "workflow-project",
			VertexLocation: "europe-west1",
			Temperature:    ptr.To(0.2),
		}

		resolved, err := config.ResolveEffectiveLLM(
			types.LLMConfig{Provider: types.LLMProviderAnthropic, VertexProject: "base-project", VertexLocation: "base-location"},
			config.LLMRuntimeConfig{Model: "base-model", Endpoint: "http://base", Temperature: ptr.To(0.7)},
			profile,
			"phaseModels[\"workflow_discovery\"]",
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Provider).To(Equal(types.LLMProviderVertexAI))
		Expect(resolved.Model).To(Equal("workflow-model"))
		Expect(resolved.VertexProject).To(Equal("workflow-project"))
		Expect(resolved.VertexLocation).To(Equal("europe-west1"))
		Expect(*resolved.Temperature).To(Equal(0.2))
		Expect(resolved.Endpoint).To(BeEmpty())
	})
})
