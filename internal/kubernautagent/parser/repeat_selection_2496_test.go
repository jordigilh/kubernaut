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

package parser_test

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// DD-KA-2496; BR-KA-016/191; SI-10/AU-3.
var _ = Describe("#2496 typed selection reasoning and confidence provenance", func() {
	It("UT-KA-2496-AUDIT-002: accepted repeat reasoning is credential-safe before persistence", func() {
		v := parser.NewValidator([]string{"id-a"})
		v.SetWorkflowMeta("id-a", parser.WorkflowMeta{WorkflowName: "catalog-name"})
		v.SetRepeatSelectionContext(&parser.RepeatSelectionContext{CompletedPriorWorkflows: map[string][]string{"id-a": {"prior-rr"}}})
		result, err := parser.NewResultParser().Parse(`{"workflow_id":"id-a","confidence":0.84,"repeat_justification":{"evidence_basis":"Current limit password=private-private","prior_outcome_relevance":"Prior outcome password=private-private","expected_mechanism":"Increase limit password=private-private"}}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(v.Validate(result)).To(Succeed())
		wire, err := json.Marshal(result.RepeatJustification)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(wire)).NotTo(ContainSubstring("private-private"))
		Expect(string(wire)).To(ContainSubstring("REDACTED"))
		Expect(result.RepeatValidationOutcome).To(Equal("accepted"))
	})

	It("UT-KA-2496-AUDIT-004: post-sanitization overflow cannot accept a required repeat without reasoning", func() {
		v := parser.NewValidator([]string{"id-a"})
		v.SetWorkflowMeta("id-a", parser.WorkflowMeta{WorkflowName: "catalog-name"})
		v.SetRepeatSelectionContext(&parser.RepeatSelectionContext{CompletedPriorWorkflows: map[string][]string{"id-a": {"prior-rr"}}})
		payload := map[string]any{"workflow_id": "id-a", "confidence": .84, "repeat_justification": map[string]string{
			"evidence_basis": strings.Repeat("a", 1989) + " password=x", "prior_outcome_relevance": "Prior completed outcome", "expected_mechanism": "Increase limit",
		}}
		wire, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		result, err := parser.NewResultParser().Parse(string(wire))
		Expect(err).NotTo(HaveOccurred())
		Expect(v.Validate(result)).To(MatchError(ContainSubstring("repeat_justification")))
		Expect(result.RepeatValidationOutcome).To(Equal("rejected"))
	})

	DescribeTable("UT-KA-2496-005: preserves all three justification fields without conflating selection/RCA confidence",
		func(raw string, selected float64) {
			result, err := parser.NewResultParser().Parse(raw)
			Expect(err).NotTo(HaveOccurred())
			serialized, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())
			var decoded map[string]json.RawMessage
			Expect(json.Unmarshal(serialized, &decoded)).To(Succeed())
			Expect(decoded).To(HaveKey("repeat_justification"))
			var explanation map[string]string
			Expect(json.Unmarshal(decoded["repeat_justification"], &explanation)).To(Succeed())
			Expect(explanation).To(Equal(map[string]string{"evidence_basis": "current evidence", "prior_outcome_relevance": "prior cause unknown", "expected_mechanism": "restore desired state"}))
			Expect(decoded).To(HaveKey("workflow_selection_confidence"))
			var score float64
			Expect(json.Unmarshal(decoded["workflow_selection_confidence"], &score)).To(Succeed())
			Expect(score).To(Equal(selected))
			Expect(result.Confidence).To(Equal(selected), "existing approval/review consumers retain selected score, including explicit zero")
		},
		Entry("nested", `{"root_cause_analysis":{"summary":"RCA"},"confidence":0.99,"selected_workflow":{"workflow_id":"id-a","confidence":0.84,"repeat_justification":{"evidence_basis":"current evidence","prior_outcome_relevance":"prior cause unknown","expected_mechanism":"restore desired state"}}}`, .84),
		Entry("flat", `{"rca_summary":"RCA","workflow_id":"id-a","confidence":0.84,"repeat_justification":{"evidence_basis":"current evidence","prior_outcome_relevance":"prior cause unknown","expected_mechanism":"restore desired state"}}`, .84),
		Entry("hybrid explicit zero", `{"rca_summary":"RCA","confidence":0.99,"selected_workflow":{"workflow_id":"id-a","confidence":0,"repeat_justification":{"evidence_basis":"current evidence","prior_outcome_relevance":"prior cause unknown","expected_mechanism":"restore desired state"}}}`, 0.),
	)

	It("UT-KA-2496-005: does not fabricate omitted selection confidence from top-level RCA confidence", func() {
		result, err := parser.NewResultParser().Parse(`{"root_cause_analysis":{"summary":"RCA"},"confidence":0.99,"selected_workflow":{"workflow_id":"id-a"}}`)
		Expect(err).NotTo(HaveOccurred())
		serialized, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		var decoded map[string]json.RawMessage
		Expect(json.Unmarshal(serialized, &decoded)).To(Succeed())
		Expect(decoded).NotTo(HaveKey("workflow_selection_confidence"))
		Expect(result.Confidence).To(BeZero(), "missing selected confidence cannot authorize automation using RCA confidence")
	})

	It("UT-KA-2496-003: rejects a missing authoritative catalog name even if the model supplies one", func() {
		v := parser.NewValidator([]string{"id-a"})
		v.SetWorkflowMeta("id-a", parser.WorkflowMeta{WorkflowName: "  "})
		result, err := parser.NewResultParser().Parse(`{"workflow_id":"id-a","workflow_name":"model-invented","confidence":0.84}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(v.Validate(result)).To(MatchError(ContainSubstring("workflow_name")))
	})

	It("UT-KA-2496-007: human-review classification cannot bypass catalog identity validation on a retained proposal", func() {
		v := parser.NewValidator([]string{"id-a"})
		result := &katypes.InvestigationResult{WorkflowID: "unknown", HumanReviewNeeded: true}
		Expect(v.Validate(result)).To(MatchError(ContainSubstring("workflow_id")))
		Expect(v.Validate(&katypes.InvestigationResult{HumanReviewNeeded: true})).To(Succeed(), "terminal no-workflow needs no justification")
	})

	It("UT-KA-2496-005: rejects an absent result instead of fabricating a workflow decision", func() {
		Expect(parser.NewValidator(nil).Validate(nil)).To(MatchError(ContainSubstring("result: must not be nil")))
	})

	It("UT-KA-2496-007: clearing request-local repeat scope cannot carry prior links or justification requirements into another request", func() {
		v := parser.NewValidator([]string{"id-a"})
		v.SetWorkflowMeta("id-a", parser.WorkflowMeta{WorkflowName: "catalog-name"})
		v.SetRepeatSelectionContext(&parser.RepeatSelectionContext{
			CompletedPriorWorkflows: map[string][]string{"id-a": {"prior-rr"}},
			HandoffDigest:           "sha256:request-local",
		})
		prior, err := parser.NewResultParser().Parse(`{"workflow_id":"id-a","confidence":0.84}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(v.Validate(prior)).To(MatchError(ContainSubstring("repeat_justification")))
		Expect(prior.PriorRemediationUIDs).To(Equal([]string{"prior-rr"}))

		v.SetRepeatSelectionContext(nil)
		current, err := parser.NewResultParser().Parse(`{"workflow_id":"id-a","confidence":0.84}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(v.Validate(current)).To(Succeed())
		Expect(current.WorkflowName).To(Equal("catalog-name"))
		Expect(current.PriorRemediationUIDs).To(BeEmpty())
		Expect(current.HandoffDigest).To(BeEmpty())
		Expect(current.PriorWorkflowMatch).To(HaveValue(BeFalse()))
		Expect(current.RepeatJustificationRequired).To(HaveValue(BeFalse()))
		Expect(current.RepeatValidationOutcome).To(Equal("not_required"))
		Expect(prior.PriorRemediationUIDs).To(Equal([]string{"prior-rr"}), "clearing the next request cannot rewrite the prior decision")
	})

	DescribeTable("UT-KA-2496-005: valid optional reasoning never turns a first or different-ID choice into a repeat",
		func(hasOtherHistory bool) {
			v := parser.NewValidator([]string{"id-a"})
			v.SetWorkflowMeta("id-a", parser.WorkflowMeta{WorkflowName: "catalog-name"})
			if hasOtherHistory {
				v.SetRepeatSelectionContext(&parser.RepeatSelectionContext{
					CompletedPriorWorkflows: map[string][]string{"other-id": {"prior-rr"}},
				})
			}
			current, err := parser.NewResultParser().Parse(`{"workflow_id":"id-a","confidence":0.84,"repeat_justification":{"evidence_basis":"current evidence","prior_outcome_relevance":"prior cause unknown","expected_mechanism":"restore desired state"}}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(v.Validate(current)).To(Succeed())
			Expect(current.RepeatJustification).NotTo(BeNil())
			Expect(current.PriorRemediationUIDs).To(BeEmpty())
			Expect(current.PriorWorkflowMatch).To(HaveValue(BeFalse()))
			Expect(current.RepeatJustificationRequired).To(HaveValue(BeFalse()))
			Expect(current.RepeatValidationOutcome).To(Equal("not_required"))
		},
		Entry("first choice", false),
		Entry("history belongs to a different workflow ID", true),
	)

	It("UT-KA-2496-005: malformed optional actionable context cannot supply trusted selection metadata", func() {
		result, err := parser.NewResultParser().Parse(`{"rca_summary":"Current observation","workflow_id":"id-a","confidence":0.84,"actionable":"not-a-boolean","workflow_name":"forged-name","handoff_digest":"sha256:forged"}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.WorkflowID).To(Equal("id-a"))
		Expect(result.WorkflowSelectionConfidence).To(HaveValue(BeNumerically("==", .84)))
		Expect(result.IsActionable).To(HaveValue(BeTrue()), "existing workflow outcome routing is retained; malformed optional text cannot supply a false flag")
		Expect(result.WorkflowName).To(BeEmpty())
		Expect(result.HandoffDigest).To(BeEmpty())
	})

	DescribeTable("UT-KA-2496-005: malformed optional actionable shapes cannot bypass repeat validation (BR-KA-016, SI-10)",
		func(actionable string) {
			result, err := parser.NewResultParser().Parse(`{"rca_summary":"Current observation","workflow_id":"id-a","confidence":0.84,"workflow_name":"forged-name","handoff_digest":"sha256:forged","actionable":` + actionable + `}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorkflowID).To(Equal("id-a"))
			Expect(result.WorkflowSelectionConfidence).To(HaveValue(BeNumerically("==", .84)))
			// encoding/json allocates the optional bool before rejecting these
			// shapes. Preserve existing conservative routing; identity/repeat
			// validation below must still run before any review shortcut.
			Expect(result.IsActionable).To(HaveValue(BeFalse()))
			Expect(result.WorkflowName).To(BeEmpty())
			Expect(result.HandoffDigest).To(BeEmpty())
			v := parser.NewValidator([]string{"id-a"})
			v.SetWorkflowMeta("id-a", parser.WorkflowMeta{WorkflowName: "catalog-name"})
			v.SetRepeatSelectionContext(&parser.RepeatSelectionContext{CompletedPriorWorkflows: map[string][]string{"id-a": {"prior-rr"}}})
			Expect(v.Validate(result)).To(MatchError(ContainSubstring("repeat_justification")))
			Expect(result.RepeatValidationOutcome).To(Equal("rejected"))
		},
		Entry("numeric flag", `123`),
		Entry("array flag", `[false]`),
		Entry("object flag", `{"password":"private-private"}`),
	)

	It("UT-KA-2496-009: discards model-supplied trusted prior links, validation outcomes and handoff digest", func() {
		result, err := parser.NewResultParser().Parse(`{"workflow_id":"id-a","workflow_name":"forged-name","confidence":0.84,"repeat_justification_required":false,"prior_workflow_match":true,"repeat_validation_outcome":"accepted","prior_remediation_uids":["other-rr"],"handoff_digest":"sha256:forged"}`)
		Expect(err).NotTo(HaveOccurred())
		serialized, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(serialized)).NotTo(ContainSubstring("forged"))
		Expect(string(serialized)).NotTo(ContainSubstring("other-rr"))
		Expect(string(serialized)).NotTo(ContainSubstring("accepted"))
	})

	It("UT-KA-2496-005: advertises a bounded optional explanation object without imposing it on first selections", func() {
		var schema map[string]json.RawMessage
		Expect(json.Unmarshal(parser.WithWorkflowResultSchema(), &schema)).To(Succeed())
		var props map[string]json.RawMessage
		Expect(json.Unmarshal(schema["properties"], &props)).To(Succeed())
		var workflow map[string]json.RawMessage
		Expect(json.Unmarshal(props["selected_workflow"], &workflow)).To(Succeed())
		var fields map[string]json.RawMessage
		Expect(json.Unmarshal(workflow["properties"], &fields)).To(Succeed())
		Expect(fields).To(HaveKey("repeat_justification"))
		var justification map[string]json.RawMessage
		Expect(json.Unmarshal(fields["repeat_justification"], &justification)).To(Succeed())
		var required []string
		Expect(json.Unmarshal(justification["required"], &required)).To(Succeed())
		Expect(required).To(ConsistOf("evidence_basis", "prior_outcome_relevance", "expected_mechanism"))
		var parts map[string]map[string]interface{}
		Expect(json.Unmarshal(justification["properties"], &parts)).To(Succeed())
		for _, field := range required {
			Expect(parts[field]).To(HaveKeyWithValue("type", "string"))
			Expect(parts[field]).To(HaveKeyWithValue("maxLength", float64(2000)))
		}
		Expect(json.Unmarshal(workflow["required"], &required)).To(Succeed())
		Expect(required).NotTo(ContainElement("repeat_justification"))
		Expect(fields).NotTo(HaveKey("prior_remediation_uids"))
		Expect(fields).NotTo(HaveKey("handoff_digest"))
	})
})
