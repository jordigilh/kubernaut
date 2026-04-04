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

package prompt_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
)

var _ = Describe("TP-433-ADV P5: Prompt Parity — GAP-010/012/019", func() {

	var builder *prompt.Builder

	BeforeEach(func() {
		var err error
		builder, err = prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("UT-KA-433-PRM-001: Severity-based priority descriptions in Phase 1 prompt", func() {
		It("should render critical severity with P1 priority", func() {
			signal := prompt.SignalData{
				Name: "OOMKilled", Namespace: "prod", Severity: "critical", Message: "OOM",
			}
			rendered, err := builder.RenderInvestigation(signal, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("P1"))
			Expect(rendered).To(ContainSubstring("critical"))
		})
	})

	Describe("UT-KA-433-PRM-002: Priority descriptions in Phase 1 prompt", func() {
		It("should render warning severity with P2 priority", func() {
			signal := prompt.SignalData{
				Name: "HighMemory", Namespace: "staging", Severity: "warning", Message: "Memory high",
			}
			rendered, err := builder.RenderInvestigation(signal, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("P2"))
		})
	})

	Describe("UT-KA-433-PRM-003: Risk guidance in Phase 1 prompt", func() {
		It("should render low risk tolerance for critical severity", func() {
			signal := prompt.SignalData{
				Name: "OOMKilled", Namespace: "prod", Severity: "critical", Message: "OOM",
			}
			rendered, err := builder.RenderInvestigation(signal, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("Low risk tolerance"))
		})
	})

	Describe("UT-KA-433-PRM-004: Guardrails section present in rendered prompt", func() {
		It("should include investigation guardrails", func() {
			signal := prompt.SignalData{
				Name: "CrashLoop", Namespace: "default", Severity: "warning", Message: "crash",
			}
			rendered, err := builder.RenderInvestigation(signal, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("Investigation Guardrails"))
			Expect(rendered).To(ContainSubstring("Exhaustive Verification"))
		})
	})

	Describe("UT-KA-433-PRM-005: Phase 3 prompt includes full remediation history (GAP-012)", func() {
		It("should render full history detail in Phase 3, not just counts", func() {
			signal := prompt.SignalData{
				Name: "OOMKilled", Namespace: "prod", Severity: "critical", Message: "OOM",
			}
			score := 0.8
			enrichData := &prompt.EnrichmentData{
				HistoryResult: &enrichment.RemediationHistoryResult{
					Tier1: []enrichment.Tier1Entry{
						{ActionType: "IncreaseMemory", Outcome: "success", EffectivenessScore: &score},
					},
				},
			}

			rendered, err := builder.RenderWorkflowSelection(signal, "Memory limit exceeded", enrichData)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("IncreaseMemory"),
				"Phase 3 should include action types from history, not just counts")
		})
	})

	Describe("UT-KA-433-PRM-006: Phase 3 prompt includes effectiveness scoring when history exists", func() {
		It("should render regression warning when detected", func() {
			signal := prompt.SignalData{
				Name: "OOMKilled", Namespace: "prod", Severity: "critical", Message: "OOM",
			}
			enrichData := &prompt.EnrichmentData{
				HistoryResult: &enrichment.RemediationHistoryResult{
					RegressionDetected: true,
					Tier1: []enrichment.Tier1Entry{
						{ActionType: "IncreaseMemory", Outcome: "failure"},
					},
				},
			}

			rendered, err := builder.RenderWorkflowSelection(signal, "Memory limit exceeded", enrichData)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("regression"),
				"Phase 3 should propagate regression warning")
		})
	})

	Describe("UT-KA-433-PRM-007: Phase 3 prompt includes investigation_outcome in JSON schema", func() {
		It("should reference investigation_outcome in the expected response format", func() {
			signal := prompt.SignalData{
				Name: "Alert", Namespace: "default", Severity: "warning", Message: "test",
			}
			rendered, err := builder.RenderWorkflowSelection(signal, "RCA summary", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("investigation_outcome"))
		})
	})

	Describe("UT-KA-433-PRM-008: Timestamps and dedup fields rendered in Phase 1", func() {
		It("should render firing and received times when provided", func() {
			signal := prompt.SignalData{
				Name: "OOMKilled", Namespace: "prod", Severity: "critical", Message: "OOM",
			}
			rendered, err := builder.RenderInvestigation(signal, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("Firing Time"))
			Expect(rendered).To(ContainSubstring("Received Time"))
		})
	})
})
