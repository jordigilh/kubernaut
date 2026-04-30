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
)

var _ = Describe("Vertex AI + Claude Regression — #684", func() {

	Describe("Static LLM fields parsed from main config", func() {

		It("UT-KA-684-002: parses vertexProject from main config", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "vertex_ai"
    vertexProject: "my-gcp-project"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.LLM.VertexProject).To(Equal("my-gcp-project"))
		})

		It("UT-KA-684-003: parses vertexLocation from main config", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "vertex_ai"
    vertexLocation: "europe-west1"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.LLM.VertexLocation).To(Equal("europe-west1"))
		})

		It("UT-KA-684-008: parses bedrockRegion from main config", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "bedrock"
    bedrockRegion: "eu-west-1"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.LLM.BedrockRegion).To(Equal("eu-west-1"))
		})

		It("UT-KA-684-009: parses azureApiVersion from main config", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "azure"
    azureApiVersion: "2024-02-15-preview"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.LLM.AzureAPIVersion).To(Equal("2024-02-15-preview"))
		})
	})

	Describe("Runtime LLM fields parsed from LLMRuntimeConfig", func() {

		It("UT-KA-684-001: parses endpoint from runtime config", func() {
			rtYAML := []byte(`
model: "claude-sonnet-4-6"
endpoint: "https://europe-west1-aiplatform.googleapis.com" # pre-commit:allow-sensitive (test fixture)
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.Endpoint).To(Equal("https://europe-west1-aiplatform.googleapis.com")) // pre-commit:allow-sensitive (test fixture)
		})

		It("UT-KA-684-004: parses apiKey from runtime config", func() {
			rtYAML := []byte(`
model: "claude-sonnet-4-6"
apiKey: "sk-test-from-config"
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.APIKey).To(Equal("sk-test-from-config"))
		})

		It("UT-KA-684-005: parses temperature from runtime config", func() {
			rtYAML := []byte(`
model: "claude-sonnet-4-6"
temperature: 0.7
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.Temperature).To(BeNumerically("~", 0.7, 0.001))
		})

		It("UT-KA-684-006: parses maxRetries and timeoutSeconds from runtime config", func() {
			rtYAML := []byte(`
model: "claude-sonnet-4-6"
maxRetries: 3
timeoutSeconds: 120
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.MaxRetries).To(Equal(3))
			Expect(rt.TimeoutSeconds).To(Equal(120))
		})

		It("UT-KA-684-010: all runtime LLM fields parsed correctly", func() {
			rtYAML := []byte(`
model: "claude-sonnet-4-20250514"
endpoint: "http://localhost:11434/v1"
temperature: 0.5
maxRetries: 5
timeoutSeconds: 60
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.Model).To(Equal("claude-sonnet-4-20250514"))
			Expect(rt.Endpoint).To(Equal("http://localhost:11434/v1"))
			Expect(rt.Temperature).To(BeNumerically("~", 0.5, 0.001))
			Expect(rt.MaxRetries).To(Equal(5))
			Expect(rt.TimeoutSeconds).To(Equal(60))
		})
	})

	Describe("Validate() runtime config", func() {
		It("UT-KA-684-103: runtime config accepts vertex_ai provider with no endpoint", func() {
			rtYAML := []byte(`
model: "claude-sonnet-4-6"
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.Validate("vertex_ai")).NotTo(HaveOccurred())
		})

		It("UT-KA-684-103b: runtime config accepts vertex provider with no endpoint", func() {
			rtYAML := []byte(`
model: "gemini-1.5-pro"
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.Validate("vertex")).NotTo(HaveOccurred())
		})

		It("UT-KA-684-104: runtime config rejects mistral provider with no endpoint", func() {
			rtYAML := []byte(`
model: "mistral-large"
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			err = rt.Validate("mistral")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endpoint"))
		})
	})
})
