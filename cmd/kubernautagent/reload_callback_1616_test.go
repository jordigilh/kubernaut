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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// IT-KA-1616-001 (CM-6): a hot-reload that changes only a phase's
// reasoning/effort (no provider/model change) must be accepted through the
// real production llmRuntimeReloadCallback path — proving AC3 from #1616:
// reasoning tuning is not identity, so it is not subject to the #1599 /
// DD-LLM-008 restart-required identity lock. This is a regression/
// non-interference test, distinct from the wire-level wiring proofs in
// llm_builder_1616_test.go.
var _ = Describe("llmRuntimeReloadCallback — per-phase reasoning tuning is not identity (#1616)", func() {
	It("accepts a hot-reload that only tunes an existing phase override's reasoning/effort", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())

		rcaInner := &stubLLMClient{}
		rcaSw, err := llm.NewSwappableClient(rcaInner, "gpt-4")
		Expect(err).NotTo(HaveOccurred())
		resolver.SetPhaseSwappable(katypes.PhaseRCA, rcaSw)

		bootRuntime := &kaconfig.LLMRuntimeConfig{
			Model: "gpt-4",
			PhaseModels: map[string]*kaconfig.LLMOverrideConfig{
				"rca": {},
			},
		}
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver, bootRuntime)

		err = cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
phaseModels:
  rca:
    reasoning:
      enabled: true
      effort: high
`)
		Expect(err).NotTo(HaveOccurred(), "reasoning-only tuning must not trip the #1599 identity lock")

		_, rcaModel, _ := resolver.ResolvePhase(katypes.PhaseRCA)
		Expect(rcaModel).To(Equal("gpt-4"), "identity is unchanged; only reasoning was tuned")
	})

	It("accepts adding a brand-new phase override that sets only reasoning (identity matches base)", func() {
		sc, resolver := setupPhaseResolver(GinkgoTB())
		bootRuntime := bootRuntimeFor("gpt-4")

		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver, bootRuntime)

		err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
phaseModels:
  validation:
    reasoning:
      enabled: true
      effort: minimal
`)
		Expect(err).NotTo(HaveOccurred())
		Expect(resolver.PhaseSwappable(katypes.PhaseValidation)).NotTo(BeNil())
	})
})
