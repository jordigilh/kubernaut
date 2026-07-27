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

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// setupPhaseResolver returns a base SwappableClient pinned to "gpt-4" (matching
// the base model used by every reload YAML in this file) and an empty
// DefaultPhaseResolver (no boot-time phase overrides).
func setupPhaseResolver(t testing.TB) (*llm.SwappableClient, *investigator.DefaultPhaseResolver) {
	t.Helper()
	inner := &stubLLMClient{}
	sc, err := llm.NewSwappableClient(inner, "gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	resolver := investigator.NewDefaultPhaseResolver(sc, nil)
	return sc, resolver
}

var _ = Describe("llmRuntimeReloadCallback — per-phase model wiring (#1470, restart-required identity lock #1599)", func() {
	// IT-KA-1599-002: tuning an existing-at-boot phase override's non-identity
	// fields (endpoint) succeeds because the phase's effective identity
	// (provider+model) does not change.
	It("accepts tuning an existing phase override's endpoint when identity is unchanged", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())

		wfInner := &stubLLMClient{}
		wfSw, err := llm.NewSwappableClient(wfInner, "gpt-4")
		Expect(err).NotTo(HaveOccurred())
		resolver.SetPhaseSwappable(katypes.PhaseWorkflowDiscovery, wfSw)

		bootRuntime := &kaconfig.LLMRuntimeConfig{
			Model: "gpt-4",
			PhaseModels: map[string]*kaconfig.LLMOverrideConfig{
				"workflow_discovery": {Endpoint: "http://fast-endpoint:11434"},
			},
		}
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver, bootRuntime)

		err = cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
phaseModels:
  workflow_discovery:
    endpoint: http://new-fast-endpoint:11434
`)
		Expect(err).NotTo(HaveOccurred())

		_, wfModel, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
		Expect(wfModel).To(Equal("gpt-4"), "identity is unchanged; only endpoint was tuned")

		_, rcaModel, _ := resolver.ResolvePhase(katypes.PhaseRCA)
		Expect(rcaModel).To(Equal("gpt-4"), "unlisted phase falls back to the base model, unchanged")
	})

	// IT-KA-1599-003: adding a brand-new phase key via hot-reload is rejected
	// when its effective identity (provider+model) differs from base — this
	// is the exact cross-provider signature-replay case #1599 exists to
	// prevent (was previously accepted; see git history of this test).
	It("rejects adding a new phase override whose identity differs from base", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())
		bootRuntime := bootRuntimeFor()

		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver, bootRuntime)

		err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
phaseModels:
  rca:
    model: claude-sonnet
    endpoint: http://anthropic:443
`)
		Expect(err).To(HaveOccurred())

		Expect(resolver.PhaseSwappable(katypes.PhaseRCA)).To(BeNil(), "rejected reload must not register a new phase override")
		_, rcaModel, _ := resolver.ResolvePhase(katypes.PhaseRCA)
		Expect(rcaModel).To(Equal("gpt-4"), "RCA still falls back to the unaffected base model")
	})

	// IT-KA-1599-004: adding a brand-new phase key is allowed when its
	// effective identity matches base (i.e. it introduces no new identity —
	// just additional non-identity tuning scoped to that phase).
	It("accepts adding a new phase override whose identity matches base", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())
		bootRuntime := bootRuntimeFor()

		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver, bootRuntime)

		err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
phaseModels:
  validation:
    endpoint: http://validation-endpoint:11434
`)
		Expect(err).NotTo(HaveOccurred())

		Expect(resolver.PhaseSwappable(katypes.PhaseValidation)).NotTo(BeNil())
		_, validationModel, _ := resolver.ResolvePhase(katypes.PhaseValidation)
		Expect(validationModel).To(Equal("gpt-4"), "identity is unchanged from base; only endpoint tuning was added")
	})

	// IT-KA-1599-005: rejects a hot-reload attempt to change an existing
	// phase override's provider (identity), even when the model string
	// happens to stay the same. Provider is part of identity, not just Model.
	It("rejects a phase override provider-only change", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())
		bootRuntime := bootRuntimeFor()

		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver, bootRuntime)

		err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
phaseModels:
  rca:
    provider: anthropic
    model: gpt-4
`)
		Expect(err).To(HaveOccurred())
		Expect(resolver.PhaseSwappable(katypes.PhaseRCA)).To(BeNil())
	})

	// IT-KA-1599-006: removing a phase override via hot-reload is rejected
	// when doing so would change that phase's effective identity (falling
	// back to a base identity different from the override's).
	It("rejects removing a phase override that would change effective identity", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())

		wfInner := &stubLLMClient{}
		wfSw, err := llm.NewSwappableClient(wfInner, "fast-model")
		Expect(err).NotTo(HaveOccurred())
		resolver.SetPhaseSwappable(katypes.PhaseWorkflowDiscovery, wfSw)

		bootRuntime := &kaconfig.LLMRuntimeConfig{
			Model: "gpt-4",
			PhaseModels: map[string]*kaconfig.LLMOverrideConfig{
				"workflow_discovery": {Model: "fast-model"},
			},
		}
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver, bootRuntime)

		err = cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
`)
		Expect(err).To(HaveOccurred())

		_, wfModel, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
		Expect(wfModel).To(Equal("fast-model"), "rejected removal must leave the existing override serving unaffected")
	})

	// IT-KA-1599-007: removing a phase override is allowed when doing so
	// does not change effective identity (the override never actually
	// differed from base — it only carried non-identity tuning).
	It("accepts removing a phase override when identity is unchanged", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())

		wfInner := &stubLLMClient{}
		wfSw, err := llm.NewSwappableClient(wfInner, "gpt-4")
		Expect(err).NotTo(HaveOccurred())
		resolver.SetPhaseSwappable(katypes.PhaseWorkflowDiscovery, wfSw)

		bootRuntime := &kaconfig.LLMRuntimeConfig{
			Model: "gpt-4",
			PhaseModels: map[string]*kaconfig.LLMOverrideConfig{
				"workflow_discovery": {Endpoint: "http://a:1"},
			},
		}
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver, bootRuntime)

		err = cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
`)
		Expect(err).NotTo(HaveOccurred())

		_, wfModel, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
		Expect(wfModel).To(Equal("gpt-4"), "override removed, falls back to base — identity was unchanged either way")
	})

	// Backward compat: nil resolver does not break existing reload, and the
	// base-model identity lock (#1599) still applies when no resolver is
	// configured.
	It("rejects a base model change and does not break reload when the resolver is nil (backward compat)", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil, bootRuntimeFor())

		err := cb(`model: gpt-4-turbo
endpoint: http://localhost:11434
apiKey: test-key
`)
		Expect(err).To(HaveOccurred())
		Expect(sc.ModelName()).To(Equal("gpt-4"))
	})
})
