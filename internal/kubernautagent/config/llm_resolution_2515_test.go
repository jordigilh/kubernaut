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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

func writeResolutionSecret(content string) string {
	path := filepath.Join(GinkgoT().TempDir(), "secret")
	Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
	return path
}

func writeOAuth2Credentials(clientID, clientSecret string) string {
	dir := GinkgoT().TempDir()
	Expect(os.WriteFile(filepath.Join(dir, "client-id"), []byte(clientID), 0o600)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(dir, "client-secret"), []byte(clientSecret), 0o600)).To(Succeed())
	return dir
}

var _ = Describe("complete sibling LLM profile resolution — #2515", func() {
	It("UT-KA-2515-001 resolves the complete investigation profile", func() {
		apiKeyFile := writeResolutionSecret("investigation-key")
		credentialsDir := writeOAuth2Credentials("investigation-client", "investigation-secret")

		static := types.LLMConfig{
			Provider:  types.LLMProviderOpenAICompatible,
			TLSCaFile: "/investigation/ca.pem",
			OAuth2:    types.LLMOAuth2Config{Enabled: true, TokenURL: "https://investigation.example/token", CredentialsDir: credentialsDir},
		}
		runtime := config.LLMRuntimeConfig{
			Model:          "investigation-model",
			Endpoint:       "https://investigation.example/v1",
			APIKeyFile:     apiKeyFile,
			Temperature:    ptr.To(0.2),
			MaxRetries:     3,
			TimeoutSeconds: 45,
		}

		resolved, err := config.ResolveEffectiveLLM(static, runtime, nil, "ai.llm")

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Provider).To(Equal(types.LLMProviderOpenAICompatible))
		Expect(resolved.Model).To(Equal("investigation-model"))
		Expect(resolved.Endpoint).To(Equal("https://investigation.example/v1"))
		Expect(resolved.APIKey).To(Equal("investigation-key"))
		Expect(resolved.OAuth2.ClientID).To(Equal("investigation-client"))
		Expect(resolved.OAuth2.ClientSecret).To(Equal("investigation-secret"))
		Expect(resolved.Temperature).NotTo(BeNil())
		Expect(*resolved.Temperature).To(Equal(0.2))
		Expect(resolved.TimeoutSeconds).To(Equal(45))
	})

	It("UT-KA-2515-002 resolves workflow discovery without inheriting investigation fields", func() {
		workflowKey := writeResolutionSecret("workflow-key")
		workflowOAuthDir := writeOAuth2Credentials("workflow-client", "workflow-secret")
		selected := &types.LLMConfig{
			Provider:       types.LLMProviderOpenAICompatible,
			Model:          "workflow-model",
			Endpoint:       "https://workflow.example/v1",
			APIKeyFile:     workflowKey,
			TLSCaFile:      "/workflow/ca.pem",
			OAuth2:         types.LLMOAuth2Config{Enabled: true, TokenURL: "https://workflow.example/token", CredentialsDir: workflowOAuthDir},
			Temperature:    ptr.To(0.8),
			MaxRetries:     ptr.To(7),
			TimeoutSeconds: 90,
		}
		static := types.LLMConfig{
			Provider:   types.LLMProviderAnthropic,
			Endpoint:   "https://investigation.example/v1",
			APIKeyFile: writeResolutionSecret("investigation-key"),
			TLSCaFile:  "/investigation/ca.pem",
		}
		runtime := config.LLMRuntimeConfig{Model: "investigation-model", Endpoint: static.Endpoint, MaxRetries: 2, TimeoutSeconds: 30}

		resolved, err := config.ResolveEffectiveLLM(static, runtime, selected, "ai.workflow_discovery")

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Provider).To(Equal(types.LLMProviderOpenAICompatible))
		Expect(resolved.Model).To(Equal("workflow-model"))
		Expect(resolved.Endpoint).To(Equal("https://workflow.example/v1"))
		Expect(resolved.APIKey).To(Equal("workflow-key"))
		Expect(resolved.APIKey).NotTo(Equal("investigation-key"))
		Expect(resolved.TLSCaFile).To(Equal("/workflow/ca.pem"))
		Expect(resolved.OAuth2.TokenURL).To(Equal("https://workflow.example/token"))
		Expect(resolved.OAuth2.ClientID).To(Equal("workflow-client"))
		Expect(resolved.TimeoutSeconds).To(Equal(90))
	})

	It("UT-KA-2515-003 resolves alignment without inheriting investigation transport", func() {
		alignmentKey := writeResolutionSecret("alignment-key")
		selected := &types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "claude-sonnet-4-6",
			APIKeyFile:     alignmentKey,
			VertexProject:  "alignment-project",
			VertexLocation: "europe-west1",
			TLSCaFile:      "/alignment/ca.pem",
			TLSCertFile:    "/alignment/client.crt",
			TLSKeyFile:     "/alignment/client.key",
			TimeoutSeconds: 75,
		}
		static := types.LLMConfig{
			Provider:    types.LLMProviderOpenAICompatible,
			Endpoint:    "https://investigation.example/v1",
			APIKeyFile:  writeResolutionSecret("investigation-key"),
			TLSCaFile:   "/investigation/ca.pem",
			TLSCertFile: "/investigation/client.crt",
			TLSKeyFile:  "/investigation/client.key",
		}
		runtime := config.LLMRuntimeConfig{Model: "investigation-model", Endpoint: static.Endpoint, TimeoutSeconds: 30}

		resolved, err := config.ResolveEffectiveLLM(static, runtime, selected, "ai.alignmentCheck.llm")

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Provider).To(Equal(types.LLMProviderVertexAI))
		Expect(resolved.Model).To(Equal("claude-sonnet-4-6"))
		Expect(resolved.Endpoint).To(BeEmpty())
		Expect(resolved.APIKey).To(Equal("alignment-key"))
		Expect(resolved.VertexProject).To(Equal("alignment-project"))
		Expect(resolved.VertexLocation).To(Equal("europe-west1"))
		Expect(resolved.TLSCaFile).To(Equal("/alignment/ca.pem"))
		Expect(resolved.TLSCertFile).To(Equal("/alignment/client.crt"))
		Expect(resolved.TLSKeyFile).To(Equal("/alignment/client.key"))
	})

	It("UT-KA-2515-004 rejects invalid complete profiles consistently", func() {
		invalid := &types.LLMConfig{
			Provider: types.LLMProviderOpenAI,
			Model:    "invalid-profile",
			// Missing endpoint and APIKeyFile are both invalid for OpenAI.
		}

		_, err := config.ResolveEffectiveLLM(types.LLMConfig{}, config.LLMRuntimeConfig{}, invalid, "ai.workflow_discovery")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ai.workflow_discovery"))
	})

	It("UT-KA-2515-005 does not mutate independent sibling profiles", func() {
		selected := &types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "gemini-2.5-pro",
			VertexProject:  "workflow-project",
			VertexLocation: "us-central1",
		}
		static := types.LLMConfig{Provider: types.LLMProviderAnthropic}

		resolved, err := config.ResolveEffectiveLLM(static, config.LLMRuntimeConfig{Model: "base-model"}, selected, "ai.workflow_discovery")

		Expect(err).NotTo(HaveOccurred())
		resolved.VertexProject = "mutated-after-resolution"
		Expect(selected.VertexProject).To(Equal("workflow-project"))
		Expect(static.Provider).To(Equal(types.LLMProviderAnthropic))
	})

	It("UT-KA-2515-007 does not expose credential contents in resolution errors", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "client-id"), []byte("sensitive-client-id"), 0o600)).To(Succeed())
		profile := &types.LLMConfig{
			Provider: types.LLMProviderOpenAICompatible,
			Model:    "workflow-model",
			Endpoint: "https://workflow.example/v1",
			OAuth2: types.LLMOAuth2Config{
				Enabled:        true,
				TokenURL:       "https://workflow.example/token",
				CredentialsDir: dir,
			},
		}

		_, err := config.ResolveEffectiveLLM(types.LLMConfig{}, config.LLMRuntimeConfig{}, profile, "ai.workflow_discovery")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring("sensitive-client-id"))
	})
})
