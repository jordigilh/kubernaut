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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
)

var _ = Describe("Phase Separation: Prompt Contracts — #700", func() {

	var builder *prompt.Builder

	BeforeEach(func() {
		var err error
		builder, err = prompt.NewBuilder(prompt.WithStructuredOutput(true))
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("UT-KA-700-003: RCA prompt excludes workflow discovery", func() {
		It("should not contain workflow tool names or discovery phases", func() {
			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name: "api-server-abc", Namespace: "production", Severity: "critical",
				Message: "OOMKilled: container api exceeded memory limit",
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			By("excluding workflow discovery tool references")
			Expect(rendered).NotTo(ContainSubstring("list_available_actions"),
				"RCA prompt must not reference workflow discovery tool list_available_actions")
			Expect(rendered).NotTo(ContainSubstring("list_workflows"),
				"RCA prompt must not reference workflow discovery tool list_workflows")
			Expect(rendered).NotTo(ContainSubstring("get_workflow"),
				"RCA prompt must not reference workflow discovery tool get_workflow")

			By("excluding workflow selection phases")
			Expect(rendered).NotTo(ContainSubstring("Phase 4"),
				"RCA prompt must not describe Phase 4 (workflow discovery)")
			Expect(rendered).NotTo(ContainSubstring("Phase 5"),
				"RCA prompt must not describe Phase 5 (workflow selection)")
			Expect(rendered).NotTo(ContainSubstring("Three-Step Protocol"),
				"RCA prompt must not reference workflow Three-Step Protocol")
		})
	})

	Describe("UT-KA-700-004: RCA prompt excludes escalation fields", func() {
		It("should not contain workflow/escalation field names in submit_result instructions", func() {
			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name: "api-server-abc", Namespace: "production", Severity: "critical",
				Message: "OOMKilled",
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			By("excluding escalation and workflow fields from submit_result example")
			Expect(rendered).NotTo(ContainSubstring("selected_workflow"),
				"RCA prompt must not reference selected_workflow")
			Expect(rendered).NotTo(ContainSubstring("needs_human_review"),
				"RCA prompt must not reference needs_human_review (parser-driven)")
			Expect(rendered).NotTo(ContainSubstring("human_review_reason"),
				"RCA prompt must not reference human_review_reason")
			Expect(rendered).NotTo(ContainSubstring("alternative_workflows"),
				"RCA prompt must not reference alternative_workflows")
		})
	})

	Describe("UT-KA-700-005: RCA prompt excludes remediation history", func() {
		It("should not contain remediation history even when enrichment provides it", func() {
			enrichData := &prompt.EnrichmentData{
				OwnerChain:     []string{"Deployment/api-server", "ReplicaSet/api-server-abc123"},
				DetectedLabels: map[string]string{"app": "api-server"},
				HistoryResult: &enrichment.RemediationHistoryResult{
					TargetResource: "production/Pod/api-server-abc",
					Tier1: []enrichment.Tier1Entry{
						{RemediationUID: "oom-increase-memory", ActionType: "increase_memory", Outcome: "success", CompletedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)},
						{RemediationUID: "oom-increase-memory", ActionType: "increase_memory", Outcome: "success", CompletedAt: time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)},
						{RemediationUID: "oom-increase-memory", ActionType: "increase_memory", Outcome: "success", CompletedAt: time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC)},
					},
					Tier1Window: "72h",
				},
			}
			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name: "api-server-abc", Namespace: "production", Severity: "critical",
				Message: "OOMKilled",
			}, enrichData)
			Expect(err).NotTo(HaveOccurred())

			By("excluding remediation history section")
			Expect(rendered).NotTo(ContainSubstring("REMEDIATION HISTORY"),
				"RCA prompt must NOT contain remediation history (belongs in Phase 3)")
			Expect(rendered).NotTo(ContainSubstring("CONFIGURATION REGRESSION DETECTED"),
				"RCA prompt must NOT contain regression warning (biases RCA toward escalation)")
			Expect(rendered).NotTo(ContainSubstring("oom-increase-memory"),
				"RCA prompt must NOT name specific past remediations")

			By("still including non-history enrichment data")
			Expect(rendered).To(ContainSubstring("Deployment/api-server"),
				"RCA prompt should still include owner chain from enrichment")
		})
	})

	Describe("UT-KA-700-006: Workflow selection prompt retains full content", func() {
		It("should contain all workflow discovery tools and submit_result with full schema", func() {
			rendered, err := builder.RenderWorkflowSelection(prompt.SignalData{
				Name: "api-server-abc", Namespace: "production", Severity: "critical",
				Message: "OOMKilled",
			}, "OOMKilled due to memory limit exceeded on api-server", nil)
			Expect(err).NotTo(HaveOccurred())

			By("including workflow discovery references")
			Expect(rendered).To(ContainSubstring("list_available_actions"),
				"workflow prompt must reference list_available_actions")
			Expect(rendered).To(ContainSubstring("list_workflows"),
				"workflow prompt must reference list_workflows")
			Expect(rendered).To(ContainSubstring("get_workflow"),
				"workflow prompt must reference get_workflow")
			Expect(rendered).To(ContainSubstring("submit_result"),
				"workflow prompt must reference submit_result")

			By("including workflow selection fields")
			Expect(rendered).To(ContainSubstring("selected_workflow"),
				"workflow prompt must reference selected_workflow in submit_result example")
		})
	})
})
