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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

func writeProfileKey2515(value string) string {
	path := filepath.Join(GinkgoT().TempDir(), "api-key")
	Expect(os.WriteFile(path, []byte(value), 0o600)).To(Succeed())
	return path
}

func completeOpenAIProfile2515(model, endpoint, keyFile string) *types.LLMConfig {
	return &types.LLMConfig{
		Provider:   types.LLMProviderOpenAICompatible,
		Model:      model,
		Endpoint:   endpoint,
		APIKeyFile: keyFile,
	}
}

func setupPhaseResolver(t testing.TB) (*llm.SwappableClient, *investigator.DefaultPhaseResolver) {
	t.Helper()
	base, err := llm.NewSwappableClient(&stubLLMClient{}, "gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	return base, investigator.NewDefaultPhaseResolver(base, nil)
}

var _ = Describe("complete sibling LLM client wiring — #2515", func() {
	It("IT-KA-2515-001 routes workflow discovery to its complete profile", func() {
		var receivedAuth string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		}))
		DeferCleanup(server.Close)

		baseKey := writeProfileKey2515("investigation-key")
		workflowKey := writeProfileKey2515("workflow-key")
		cfg := kaconfig.DefaultConfig()
		cfg.AI.LLM = *completeOpenAIProfile2515("investigation-model", server.URL, baseKey)
		runtime := &kaconfig.LLMRuntimeConfig{
			Model:      "investigation-model",
			Endpoint:   server.URL,
			APIKeyFile: baseKey,
			APIKey:     "investigation-key",
			PhaseModels: map[string]*types.LLMConfig{
				string(katypes.PhaseWorkflowDiscovery): completeOpenAIProfile2515("workflow-model", server.URL, workflowKey),
			},
		}

		_, phaseClients := buildLLMClients(cfg, runtime, testReloadLogger())
		workflowClient := phaseClients[katypes.PhaseWorkflowDiscovery]
		Expect(workflowClient).NotTo(BeNil())

		_, err := workflowClient.Chat(context.Background(), helloChatRequest())
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedAuth).To(Equal("Bearer workflow-key"))
	})

	It("IT-KA-2515-002 routes alignment to its complete profile", func() {
		var receivedAuth string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"{\"suspicious\":false}"},"finish_reason":"stop"}]}`))
		}))
		DeferCleanup(server.Close)

		baseKey := writeProfileKey2515("investigation-key")
		alignmentKey := writeProfileKey2515("alignment-key")
		cfg := kaconfig.DefaultConfig()
		cfg.AI.LLM = *completeOpenAIProfile2515("investigation-model", server.URL, baseKey)
		cfg.AI.AlignmentCheck = kaconfig.AlignmentCheckConfig{
			Enabled:        true,
			Mode:           kaconfig.AlignmentModeEnforce,
			Timeout:        5 * time.Second,
			MaxStepTokens:  500,
			MaxRetries:     1,
			VerdictTimeout: 5 * time.Second,
			LLM:            completeOpenAIProfile2515("alignment-model", server.URL, alignmentKey),
		}
		runtime := &kaconfig.LLMRuntimeConfig{
			Model:      "investigation-model",
			Endpoint:   server.URL,
			APIKeyFile: baseKey,
			APIKey:     "investigation-key",
		}

		_, _, evaluator, alignCfg := buildAlignmentStack(
			cfg, runtime, &stubLLMClient{}, registry.New(), audit.NopAuditStore{}, testReloadLogger())
		Expect(alignCfg.Enabled).To(BeTrue())
		Expect(evaluator).NotTo(BeNil())

		_ = evaluator.EvaluateStep(context.Background(), alignment.Step{
			Index:   0,
			Kind:    alignment.StepKindToolResult,
			Content: "hello",
		})
		Expect(receivedAuth).To(Equal("Bearer alignment-key"))
	})

	It("IT-KA-2515-004 reloads a complete workflow profile and preserves identity locking", func() {
		var receivedAuth string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		}))
		DeferCleanup(server.Close)

		phaseKey := writeProfileKey2515("reloaded-workflow-key")
		swappable, resolver := setupPhaseResolver(GinkgoTB())
		callback := llmRuntimeReloadCallback(staticCfg(), swappable, testReloadLogger(), resolver, bootRuntimeFor())

		err := callback(fmt.Sprintf(`model: gpt-4
endpoint: %s
phaseModels:
  workflow_discovery:
    provider: openai
    model: gpt-4
    endpoint: %s
    apiKeyFile: %s
`, server.URL, server.URL, phaseKey))
		Expect(err).NotTo(HaveOccurred())

		phaseClient, _, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
		Expect(phaseClient).NotTo(BeNil())
		_, err = phaseClient.Chat(context.Background(), helloChatRequest())
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedAuth).To(Equal("Bearer reloaded-workflow-key"))

		err = callback(fmt.Sprintf(`model: gpt-4
endpoint: %s
phaseModels:
  workflow_discovery:
    provider: anthropic
    model: gpt-4
    apiKeyFile: %s
`, server.URL, phaseKey))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("phase identity change"))
	})

	It("IT-KA-2515-005 rejects an unreadable complete phase credential", func() {
		swappable, resolver := setupPhaseResolver(GinkgoTB())
		missingKey := filepath.Join(GinkgoT().TempDir(), "missing-key")
		callback := llmRuntimeReloadCallback(staticCfg(), swappable, testReloadLogger(), resolver, bootRuntimeFor())

		err := callback(fmt.Sprintf(`model: gpt-4
endpoint: http://workflow.example/v1
phaseModels:
  workflow_discovery:
    provider: openai
    model: gpt-4
    endpoint: http://workflow.example/v1
    apiKeyFile: %s
`, missingKey))

		Expect(err).NotTo(HaveOccurred())
		Expect(resolver.PhaseSwappable(katypes.PhaseWorkflowDiscovery)).To(BeNil())
	})
})
