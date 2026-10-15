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

package types_test

import (
	"encoding/json"
	"math"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

const (
	liveSelectedKey2496   = "selected_workflow"
	liveAssessmentKey2496 = "selection_assessment"
	liveConfidenceKey2496 = "workflow_selection_confidence"
	liveReasonKey2496     = "repeat_justification"
)

var _ = Describe("UT-KA-2496-011: shared live selection boundary [BR-KA-016, AC-4, SI-10]", func() {
	It("keeps absent results and unknown confidence absent instead of borrowing RCA confidence", func() {
		for _, result := range []*katypes.InvestigationResult{nil, {}} {
			data, err := katypes.LiveSelectionFromResult(result).ToMap()
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(BeEmpty())
		}
		result := liveProjectionResult2496()
		result.WorkflowSelectionConfidence = nil
		data, err := katypes.LiveSelectionFromResult(result).ToMap()
		Expect(err).NotTo(HaveOccurred())
		Expect(data[liveSelectedKey2496]).NotTo(HaveKey(liveConfidenceKey2496))
	})

	DescribeTable("omits invalid selection confidence and retains finite boundary values independently",
		func(value float64, valid bool) {
			result := liveProjectionResult2496()
			result.WorkflowSelectionConfidence = &value
			data, err := katypes.LiveSelectionFromResult(result).ToMap()
			Expect(err).NotTo(HaveOccurred())
			selected := data[liveSelectedKey2496]
			if valid {
				Expect(selected).To(HaveKeyWithValue(liveConfidenceKey2496, value))
			} else {
				Expect(selected).NotTo(HaveKey(liveConfidenceKey2496))
			}
		},
		Entry("known zero", 0., true),
		Entry("exact upper boundary", 1., true),
		Entry("negative", -.01, false),
		Entry("above one", 1.01, false),
		Entry("NaN", math.NaN(), false),
		Entry("positive infinity", math.Inf(1), false),
		Entry("negative infinity", math.Inf(-1), false),
	)

	DescribeTable("omits invalid mandatory identity without promoting another proposed workflow",
		func(id, name string) {
			context := katypes.LiveSelectionContext{
				SelectedWorkflow:    &katypes.LiveSelectedWorkflow{WorkflowID: id, WorkflowName: name},
				SelectionAssessment: &katypes.LiveSelectionAssessment{ProposedWorkflowID: "other-candidate"},
			}
			data, err := context.ToMap()
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(BeEmpty())
		},
		Entry("blank ID", " \t", "catalog-name"),
		Entry("oversized ID", strings.Repeat("a", 254), "catalog-name"),
		Entry("invalid UTF-8 ID", string([]byte{0xff}), "catalog-name"),
		Entry("missing name", "catalog-id", ""),
		Entry("oversized name", "catalog-id", strings.Repeat("a", 254)),
	)

	DescribeTable("omits incomplete or oversized accepted reasoning atomically, never truncating it",
		func(evidence string) {
			result := liveProjectionResult2496()
			result.RepeatJustification.EvidenceBasis = evidence
			data, err := katypes.LiveSelectionFromResult(result).ToMap()
			Expect(err).NotTo(HaveOccurred())
			Expect(data[liveSelectedKey2496]).NotTo(HaveKey(liveReasonKey2496))
			Expect(result.RepeatJustification.EvidenceBasis).To(Equal(evidence), "projection must not mutate authoritative reasoning")
		},
		Entry("missing", ""),
		Entry("blank", " \t\n"),
		Entry("invalid UTF-8", string([]byte{0xff})),
		Entry("2001 Unicode characters", strings.Repeat("界", 2001)),
		Entry("sanitization expands a 2000-character value beyond the budget", strings.Repeat("a", 1989)+" password=x"),
	)

	It("retains the exact Unicode limit, copies all optional values and emits only JSON-shaped public data", func() {
		result := liveProjectionResult2496()
		result.RepeatJustification.EvidenceBasis = strings.Repeat("界", 2000)
		projected := katypes.LiveSelectionFromResult(result)
		data, err := projected.ToMap()
		Expect(err).NotTo(HaveOccurred())
		selected, ok := data[liveSelectedKey2496].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(selected).To(HaveKeyWithValue("repeat_justification_required", false))
		Expect(selected).To(HaveKeyWithValue("prior_workflow_match", false))
		reason, ok := selected[liveReasonKey2496].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(reason).To(HaveKeyWithValue("evidence_basis", result.RepeatJustification.EvidenceBasis))
		raw, err := json.Marshal(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(ContainSubstring("private-private"))
		*projected.SelectedWorkflow.WorkflowSelectionConfidence = .1
		*projected.SelectedWorkflow.RepeatJustificationRequired = true
		*projected.SelectedWorkflow.PriorWorkflowMatch = true
		projected.SelectedWorkflow.RepeatJustification.EvidenceBasis = "changed copy"
		reason["evidence_basis"] = "changed map"
		Expect(*result.WorkflowSelectionConfidence).To(Equal(.84))
		Expect(*result.RepeatJustificationRequired).To(BeFalse())
		Expect(*result.PriorWorkflowMatch).To(BeFalse())
		Expect(result.RepeatJustification.EvidenceBasis).To(Equal(strings.Repeat("界", 2000)))
	})

	It("keeps bounded rejected parts non-executable, independent and optionally absent", func() {
		result := liveProjectionResult2496()
		result.ProposedWorkflowID, result.WorkflowID = result.WorkflowID, ""
		result.RepeatValidationOutcome = "rejected"
		result.RepeatJustification.EvidenceBasis = strings.Repeat("界", 2001)
		result.RepeatJustification.PriorOutcomeRelevance = string([]byte{0xff})
		projected := katypes.LiveSelectionFromResult(result)
		data, err := projected.ToMap()
		Expect(err).NotTo(HaveOccurred())
		Expect(data).NotTo(HaveKey(liveSelectedKey2496))
		assessment := data[liveAssessmentKey2496]
		Expect(assessment).NotTo(HaveKey("workflow_id"))
		Expect(assessment).To(HaveKeyWithValue("proposed_workflow_id", result.ProposedWorkflowID))
		Expect(assessment).To(HaveKeyWithValue(liveReasonKey2496, map[string]any{"expected_mechanism": result.RepeatJustification.ExpectedMechanism}))
		projected.SelectionAssessment.RepeatJustification.ExpectedMechanism = "changed assessment"
		*projected.SelectionAssessment.WorkflowSelectionConfidence = .1
		*projected.SelectionAssessment.RepeatJustificationRequired = true
		*projected.SelectionAssessment.PriorWorkflowMatch = true
		Expect(result.RepeatJustification.ExpectedMechanism).To(Equal("Increase memory"))
		Expect(*result.WorkflowSelectionConfidence).To(Equal(.84))
		Expect(*result.RepeatJustificationRequired).To(BeFalse())
		Expect(*result.PriorWorkflowMatch).To(BeFalse())
		result.RepeatJustification = nil
		data, err = katypes.LiveSelectionFromResult(result).ToMap()
		Expect(err).NotTo(HaveOccurred())
		Expect(data[liveAssessmentKey2496]).NotTo(HaveKey(liveReasonKey2496))
		invalid := katypes.LiveSelectionContext{SelectionAssessment: &katypes.LiveSelectionAssessment{ProposedWorkflowID: " \t"}}
		data, err = invalid.ToMap()
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(BeEmpty())
	})

	It("omits an unknown validation outcome rather than inventing acceptance", func() {
		result := liveProjectionResult2496()
		result.RepeatValidationOutcome = "invented-outcome"
		data, err := katypes.LiveSelectionFromResult(result).ToMap()
		Expect(err).NotTo(HaveOccurred())
		Expect(data[liveSelectedKey2496]).NotTo(HaveKey("repeat_validation_outcome"))
	})
})

func liveProjectionResult2496() *katypes.InvestigationResult {
	confidence := .84
	return &katypes.InvestigationResult{
		WorkflowID: "catalog-id", WorkflowName: "catalog-name", Confidence: .99,
		WorkflowSelectionConfidence: &confidence,
		RepeatJustification: &sharedtypes.RepeatJustification{
			EvidenceBasis: "Current observation", PriorOutcomeRelevance: "Prior Full assessment", ExpectedMechanism: "Increase memory",
		},
		RepeatJustificationRequired: boolPtr(false), PriorWorkflowMatch: boolPtr(false), RepeatValidationOutcome: "accepted",
		Parameters: map[string]any{"PASSWORD": "private-private"}, ExecutionBundle: "private-private", PriorRemediationUIDs: []string{"private-private"},
	}
}
