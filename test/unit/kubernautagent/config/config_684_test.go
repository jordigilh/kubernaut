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

	// Bug 1: MergeSDKConfig() drops critical LLM fields
	Describe("Bug 1: SDK config merge completeness", func() {
		var mainYAML []byte

		BeforeEach(func() {
			mainYAML = []byte(`
llm:
  provider: "vertex_ai"
  model: "claude-sonnet-4-6"
`)
		})

		It("UT-KA-684-001: merges endpoint from SDK config when main has none", func() {
			sdkYAML := []byte(`
llm:
  provider: "vertex_ai"
  model: "claude-sonnet-4-6"
  endpoint: "https://europe-west1-aiplatform.googleapis.com" # pre-commit:allow-sensitive (test fixture)
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.Endpoint).To(Equal("https://europe-west1-aiplatform.googleapis.com")) // pre-commit:allow-sensitive (test fixture)
		})

		It("UT-KA-684-002: merges vertex_project from SDK config when main has none", func() {
			sdkYAML := []byte(`
llm:
  provider: "vertex_ai"
  model: "claude-sonnet-4-6"
  vertex_project: "my-gcp-project"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.VertexProject).To(Equal("my-gcp-project"))
		})

		It("UT-KA-684-003: merges vertex_location from SDK config when main has none", func() {
			sdkYAML := []byte(`
llm:
  provider: "vertex_ai"
  model: "claude-sonnet-4-6"
  vertex_location: "europe-west1"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.VertexLocation).To(Equal("europe-west1"))
		})

		It("UT-KA-684-004: merges api_key from SDK config when main has none", func() {
			sdkYAML := []byte(`
llm:
  provider: "vertex_ai"
  model: "claude-sonnet-4-6"
  api_key: "sk-test-from-sdk"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.APIKey).To(Equal("sk-test-from-sdk"))
		})

		It("UT-KA-684-005: merges temperature from SDK config", func() {
			sdkYAML := []byte(`
llm:
  provider: "vertex_ai"
  model: "claude-sonnet-4-6"
  temperature: 0.7
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.Temperature).To(BeNumerically("~", 0.7, 0.001))
		})

		It("UT-KA-684-006: merges max_retries and timeout_seconds from SDK config", func() {
			sdkYAML := []byte(`
llm:
  provider: "vertex_ai"
  model: "claude-sonnet-4-6"
  max_retries: 3
  timeout_seconds: 120
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.MaxRetries).To(Equal(3))
			Expect(cfg.LLM.TimeoutSeconds).To(Equal(120))
		})

		It("UT-KA-684-007: main config endpoint takes precedence over SDK (gap-fill)", func() {
			mainWithEndpoint := []byte(`
llm:
  provider: "vertex_ai"
  model: "claude-sonnet-4-6"
  endpoint: "http://main-endpoint"
`)
			sdkYAML := []byte(`
llm:
  provider: "vertex_ai"
  model: "claude-sonnet-4-6"
  endpoint: "http://sdk-endpoint"
`)
			cfg, err := config.Load(mainWithEndpoint)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.Endpoint).To(Equal("http://main-endpoint"))
		})

		It("UT-KA-684-008: merges bedrock_region from SDK config when main has none", func() {
			sdkYAML := []byte(`
llm:
  provider: "bedrock"
  model: "anthropic.claude-3-sonnet"
  bedrock_region: "eu-west-1"
`)
			mainBedrock := []byte(`
llm:
  provider: "bedrock"
  model: "anthropic.claude-3-sonnet"
`)
			cfg, err := config.Load(mainBedrock)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.BedrockRegion).To(Equal("eu-west-1"))
		})

		It("UT-KA-684-009: merges azure_api_version from SDK config when main has none", func() {
			sdkYAML := []byte(`
llm:
  provider: "azure"
  model: "gpt-4"
  azure_api_version: "2024-02-15-preview"
`)
			mainAzure := []byte(`
llm:
  provider: "azure"
  model: "gpt-4"
  endpoint: "https://my-resource.openai.azure.com" # pre-commit:allow-sensitive (test fixture)
`)
			cfg, err := config.Load(mainAzure)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.AzureAPIVersion).To(Equal("2024-02-15-preview"))
		})

		It("UT-KA-684-010: existing provider/model gap-fill semantics unchanged", func() {
			minimalMain := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
`)
			sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
			cfg, err := config.Load(minimalMain)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.Provider).To(Equal("anthropic"))
			Expect(cfg.LLM.Model).To(Equal("claude-sonnet-4-20250514"))
			Expect(cfg.LLM.Endpoint).To(Equal("http://localhost:11434/v1"),
				"main config endpoint must not be overwritten")
		})
	})

	// Bug 2: Provider name validation
	Describe("Bug 2: Validate() endpoint exemptions", func() {
		It("UT-KA-684-103: accepts vertex_ai provider with no endpoint", func() {
			yaml := []byte(`
llm:
  provider: "vertex_ai"
  model: "claude-sonnet-4-6"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Validate()).NotTo(HaveOccurred())
		})

		It("UT-KA-684-103b: accepts vertex provider with no endpoint", func() {
			yaml := []byte(`
llm:
  provider: "vertex"
  model: "gemini-1.5-pro"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Validate()).NotTo(HaveOccurred())
		})

		It("UT-KA-684-104: still rejects mistral provider with no endpoint", func() {
			yaml := []byte(`
llm:
  provider: "mistral"
  model: "mistral-large"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endpoint"))
		})
	})
})
