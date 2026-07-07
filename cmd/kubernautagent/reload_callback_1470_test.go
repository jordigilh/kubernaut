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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

func setupPhaseResolver(t testing.TB) (*llm.SwappableClient, *investigator.DefaultPhaseResolver) {
	t.Helper()
	inner := &stubLLMClient{}
	sc, err := llm.NewSwappableClient(inner, "default-model")
	if err != nil {
		t.Fatal(err)
	}
	resolver := investigator.NewDefaultPhaseResolver(sc, nil)
	return sc, resolver
}

var _ = Describe("llmRuntimeReloadCallback — per-phase model wiring (#1470)", func() {
	// IT-AI-1470-004a (CM-3): Hot-reload with phaseModels rebuild
	It("rebuilds phase overrides on reload and leaves unlisted phases on the reloaded default", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())

		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver)

		err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
phaseModels:
  workflow_discovery:
    model: gpt-4-mini
    endpoint: http://fast-endpoint:11434
`)
		Expect(err).NotTo(HaveOccurred())

		_, wfModel, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
		Expect(wfModel).To(Equal("gpt-4-mini"))

		_, rcaModel, _ := resolver.ResolvePhase(katypes.PhaseRCA)
		Expect(rcaModel).To(Equal("gpt-4"), "RCA should use the default (reloaded) model")
	})

	// IT-AI-1470-004b: Hot-reload adding a new phase override
	It("adds a new phase override on reload", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())

		_, rcaModelBefore, _ := resolver.ResolvePhase(katypes.PhaseRCA)
		Expect(rcaModelBefore).To(Equal("default-model"))

		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver)

		err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
phaseModels:
  rca:
    model: claude-sonnet
    endpoint: http://anthropic:443
`)
		Expect(err).NotTo(HaveOccurred())

		_, rcaModel, _ := resolver.ResolvePhase(katypes.PhaseRCA)
		Expect(rcaModel).To(Equal("claude-sonnet"))
	})

	// IT-AI-1470-004c: Hot-reload removing a phase override
	It("removes a phase override on reload, falling back to the reloaded default", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())

		wfInner := &stubLLMClient{}
		wfSw, err := llm.NewSwappableClient(wfInner, "fast-model")
		Expect(err).NotTo(HaveOccurred())
		resolver.SetPhaseSwappable(katypes.PhaseWorkflowDiscovery, wfSw)

		_, wfModelBefore, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
		Expect(wfModelBefore).To(Equal("fast-model"))

		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver)

		err = cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
`)
		Expect(err).NotTo(HaveOccurred())

		_, wfModelAfter, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
		Expect(wfModelAfter).To(Equal("gpt-4"), "removing a phase override should fall back to the reloaded default")
	})

	// Backward compat: nil resolver does not break existing reload
	It("does not break reload when the resolver is nil (backward compat)", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil)

		err := cb(`model: gpt-4-turbo
endpoint: http://localhost:11434
apiKey: test-key
`)
		Expect(err).NotTo(HaveOccurred())
		Expect(sc.ModelName()).To(Equal("gpt-4-turbo"))
	})
})
