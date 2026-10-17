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

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Investigation terminal outcome classification — #2521", func() {
	It("UT-KA-2521-001: does not infer no-match from an absent workflow", func() {
		outcome := investigator.ClassifyTerminalOutcome(investigator.OutcomeClassificationInput{
			Result: &katypes.InvestigationResult{},
		})

		Expect(outcome).NotTo(Equal(sharedtypes.TerminalOutcomeNoMatchingWorkflows))
		Expect(outcome).To(Equal(sharedtypes.TerminalOutcomeUnknown))
	})

	It("UT-KA-2521-002: classifies #2518 history transport exhaustion before discovery", func() {
		outcome := investigator.ClassifyTerminalOutcome(investigator.OutcomeClassificationInput{
			Result: &katypes.InvestigationResult{
				HumanReviewNeeded: true,
				HumanReviewReason: katypes.HumanReviewReasonRCAIncomplete,
			},
			HistoryStatus:           sharedtypes.HistoryStatusTransportUnavailable,
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusSkippedDueToFailure,
		})

		Expect(outcome).To(Equal(sharedtypes.TerminalOutcomeEvidenceTransportUnavailable))
	})

	It("UT-KA-2521-003: retains RCA-incomplete only when evidence is available", func() {
		outcome := investigator.ClassifyTerminalOutcome(investigator.OutcomeClassificationInput{
			Result: &katypes.InvestigationResult{
				HumanReviewNeeded: true,
				HumanReviewReason: katypes.HumanReviewReasonRCAIncomplete,
			},
			HistoryStatus:           sharedtypes.HistoryStatusAvailable,
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusNotStarted,
		})

		Expect(outcome).To(Equal(sharedtypes.TerminalOutcomeRCAIncomplete))
	})

	It("UT-KA-2521-004: requires completed discovery before producing no-match", func() {
		outcome := investigator.ClassifyTerminalOutcome(investigator.OutcomeClassificationInput{
			Result:                  &katypes.InvestigationResult{},
			HistoryStatus:           sharedtypes.HistoryStatusEmpty,
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch,
		})

		Expect(outcome).To(Equal(sharedtypes.TerminalOutcomeNoMatchingWorkflows))
	})

	It("UT-KA-2521-005: distinguishes workflow-discovery failure from no-match", func() {
		outcome := investigator.ClassifyTerminalOutcome(investigator.OutcomeClassificationInput{
			Result:                  &katypes.InvestigationResult{},
			HistoryStatus:           sharedtypes.HistoryStatusAvailable,
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusFailed,
		})

		Expect(outcome).To(Equal(sharedtypes.TerminalOutcomeWorkflowDiscoveryFailed))
	})

	It("UT-KA-2521-006: preserves a selected workflow over secondary status fields", func() {
		outcome := investigator.ClassifyTerminalOutcome(investigator.OutcomeClassificationInput{
			Result:                  &katypes.InvestigationResult{WorkflowID: "wf-oom-recovery"},
			HistoryStatus:           sharedtypes.HistoryStatusAvailable,
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusCompletedSelected,
		})

		Expect(outcome).To(Equal(sharedtypes.TerminalOutcomeWorkflowSelected))
	})

	It("UT-KA-2521-007: preserves actionable RCA when required history enrichment later fails", func() {
		actionable := true
		result := &katypes.InvestigationResult{
			RCASummary:           "OOMKilled after memory pressure",
			InvestigationOutcome: "actionable",
			IsActionable:         &actionable,
			HumanReviewNeeded:    true,
			HumanReviewReason:    katypes.HumanReviewReasonRCAIncomplete,
		}

		outcome := investigator.ClassifyTerminalOutcome(investigator.OutcomeClassificationInput{
			Result:                       result,
			HistoryStatus:                sharedtypes.HistoryStatusTransportUnavailable,
			WorkflowDiscoveryStatus:      sharedtypes.WorkflowDiscoveryStatusSkippedDueToFailure,
			HistoryEnrichmentUnavailable: true,
		})

		Expect(outcome).To(Equal(sharedtypes.TerminalOutcomeHistoryEnrichmentUnavailable))
		Expect(result.RCASummary).To(Equal("OOMKilled after memory pressure"))
		Expect(result.InvestigationOutcome).To(Equal("actionable"))
		Expect(result.IsActionable).NotTo(BeNil())
		Expect(*result.IsActionable).To(BeTrue())
	})
})
