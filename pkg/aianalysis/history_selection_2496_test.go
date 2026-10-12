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

package aianalysis_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

const (
	aaRejected2496  = "rejected"
	aaCatalogID2496 = "catalog-id-2496"
)

func selectionResult2496(confidence float64, review bool) *katypes.InvestigationResult {
	required, matched, rcaConfidence := true, true, .93
	return &katypes.InvestigationResult{
		RCASummary: "Memory pressure", RCAConfidence: &rcaConfidence,
		WorkflowID: aaCatalogID2496, WorkflowName: "increase-memory", Confidence: confidence,
		WorkflowSelectionConfidence: &confidence, WorkflowVersion: "1.0.0", ActionType: "IncreaseMemory",
		ExecutionBundle:   "ghcr.io/kubernaut/increase-memory@sha256:" + strings.Repeat("a", 64),
		WorkflowRationale: "Selection based on current memory observations",
		HumanReviewNeeded: review, HumanReviewReason: "rca_incomplete",
		RepeatJustification: &sharedtypes.RepeatJustification{EvidenceBasis: "New memory limit observed",
			PriorOutcomeRelevance: "Prior Full assessment was ineffective", ExpectedMechanism: "Increase the current limit"},
		RepeatJustificationRequired: &required, PriorWorkflowMatch: &matched, RepeatValidationOutcome: "accepted",
		PriorRemediationUIDs: []string{"trusted-prior-2496"}, HandoffDigest: "sha256:" + strings.Repeat("b", 64),
	}
}

func selectionObject2496(raw []byte) map[string]json.RawMessage {
	var object map[string]json.RawMessage
	ExpectWithOffset(1, json.Unmarshal(raw, &object)).To(Succeed())
	return object
}

// BR-KA-016 / BR-AI-088 / DD-KA-2496: these are real mapper/processor tests,
// not a substitute for the admitted AgentSession -> AA controller IT.
var _ = Describe("#2496 selection persistence", func() {
	DescribeTable("UT-AA-2496-003: both complete and failed typed audit preserve selected or non-executable reasoning (BR-KA-016, BR-AUDIT-005, AU-3, CC7.2)",
		func(confidence float64, mode string) {
			rejected := mode == aaRejected2496 || mode == "rejected-complete-parts"
			r := selectionResult2496(confidence, mode == "review" || rejected)
			if mode == "unknown" {
				r.WorkflowSelectionConfidence = nil
			}
			if rejected {
				r.WorkflowID, r.ProposedWorkflowID, r.RepeatValidationOutcome = "", aaCatalogID2496, aaRejected2496
				if mode == aaRejected2496 {
					r.RepeatJustification.ExpectedMechanism = ""
				}
			}
			if mode == "no-workflow" {
				r = &katypes.InvestigationResult{RCASummary: "No suitable workflow", HumanReviewNeeded: true, HumanReviewReason: "no_matching_workflows"}
			}
			analysis := buildLowConfidenceFloorTestAnalysis()
			store := &aiAuditStoreSpy{}
			auditClient := aiaudit.NewAuditClient(store, logr.Discard())
			_, err := handlers.NewResponseProcessor(logr.Discard(), metrics.NewMetrics(), auditClient).
				ProcessAgentSessionResult(context.Background(), analysis, agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), r, "incident-2496"))
			Expect(err).NotTo(HaveOccurred())
			// Prove the actual terminal dispatch, not just re-emission after the
			// processor has finished preserving context. Exercise the other emitter
			// once so both typed payloads retain the same selection evidence.
			if analysis.Status.Phase != aianalysis.PhaseCompleted {
				auditClient.RecordAnalysisComplete(context.Background(), analysis)
			}
			if analysis.Status.Phase != aianalysis.PhaseFailed {
				Expect(auditClient.RecordAnalysisFailed(context.Background(), analysis, errors.New("investigation requires review"))).To(Succeed())
			}
			Expect(store.events).To(HaveLen(2))
			for _, event := range store.events {
				Expect(string(event.EventData.Type)).To(Equal(event.EventType), "typed discriminator must match the emitted event, even before a phase transition")
				raw, err := json.Marshal(event.EventData)
				Expect(err).NotTo(HaveOccurred())
				payload := selectionObject2496(raw)
				if mode == "no-workflow" {
					Expect(payload).NotTo(HaveKey("workflow_id"))
					Expect(payload).NotTo(HaveKey("selection_assessment"))
					continue
				}
				selection := payload
				if rejected {
					Expect(payload).NotTo(HaveKey("workflow_id"))
					Expect(payload).To(HaveKey("selection_assessment"))
					selection = selectionObject2496(payload["selection_assessment"])
					Expect(string(selection["proposed_workflow_id"])).To(Equal(`"catalog-id-2496"`))
					Expect(selection).NotTo(HaveKey("workflow_id"))
				} else {
					Expect(string(selection["workflow_id"])).To(Equal(`"catalog-id-2496"`))
				}
				Expect(string(selection["workflow_name"])).To(Equal(`"increase-memory"`))
				if mode == "unknown" {
					Expect(selection).NotTo(HaveKey("workflow_selection_confidence"))
				} else {
					var actual float64
					Expect(json.Unmarshal(selection["workflow_selection_confidence"], &actual)).To(Succeed())
					Expect(actual).To(BeNumerically("~", confidence, 0.000001))
				}
				Expect(string(selection["repeat_justification_required"])).To(Equal("true"))
				Expect(string(selection["prior_workflow_match"])).To(Equal("true"))
				Expect(string(selection["repeat_validation_outcome"])).To(Equal(`"` + r.RepeatValidationOutcome + `"`))
				Expect(string(selection["handoff_digest"])).To(Equal(`"` + r.HandoffDigest + `"`))
				Expect(string(selection["prior_remediation_uids"])).To(MatchJSON(`["trusted-prior-2496"]`))
				Expect(string(selection["repeat_justification"])).To(ContainSubstring("New memory limit observed"))
				if mode == "rejected-complete-parts" {
					Expect(string(selection["repeat_justification"])).To(ContainSubstring("Increase the current limit"))
				}
			}
		},
		Entry("success", .84, "success"), Entry("explicit zero", 0., "review"), Entry("partial review", .84, "review"),
		Entry("unknown", .84, "unknown"), Entry(aaRejected2496, .84, aaRejected2496), Entry("no workflow", 0., "no-workflow"),
		Entry("complete bounded rejected parts remain non-executable", .84, "rejected-complete-parts"),
	)

	DescribeTable("UT-AA-2496-003: unrepresentable selection identity omits optional audit context without disclosing trusted prior links (BR-KA-016, AU-3, SI-10)",
		func(rejected bool) {
			analysis := buildLowConfidenceFloorTestAnalysis()
			rca := analysis.Status.EnsureRCAResult()
			if rejected {
				rca.SelectionAssessment = &aianalysisv1.SelectionAssessment{
					ProposedWorkflowID: " \t", WorkflowName: "increase-memory",
					RepeatValidationOutcome: aaRejected2496, PriorRemediationUIDs: []string{"trusted-prior-2496"},
				}
			} else {
				rca.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
					WorkflowSnapshot:        sharedtypes.WorkflowSnapshot{WorkflowID: aaCatalogID2496, WorkflowName: " \t"},
					RepeatValidationOutcome: "accepted", PriorRemediationUIDs: []string{"trusted-prior-2496"},
				}
			}
			before := analysis.DeepCopy()
			store := &aiAuditStoreSpy{}
			aiaudit.NewAuditClient(store, logr.Discard()).RecordAnalysisComplete(context.Background(), analysis)
			Expect(store.events).To(HaveLen(1))
			raw, err := json.Marshal(store.events[0].EventData)
			Expect(err).NotTo(HaveOccurred())
			payload := selectionObject2496(raw)
			for _, key := range []string{"workflow_name", "workflow_selection_confidence", "repeat_justification", "repeat_validation_outcome", "prior_remediation_uids", "selection_assessment"} {
				Expect(payload).NotTo(HaveKey(key))
			}
			if rejected {
				Expect(payload).NotTo(HaveKey("workflow_id"))
			} else {
				Expect(string(payload["workflow_id"])).To(Equal(`"catalog-id-2496"`), "existing recorded identity is not replaced or inferred from a missing name")
			}
			Expect(analysis).To(Equal(before), "audit projection cannot repair or mutate the source snapshot")
		},
		Entry("selected metadata has no representable readable name", false),
		Entry("rejected metadata has no representable proposal identity", true),
	)

	It("UT-AA-2496-001: missing proposal identity is observable and cannot become an executable selection (BR-KA-016, SI-10)", func() {
		core, logs := observer.New(zap.InfoLevel)
		logger := zapr.NewLogger(zap.New(core))
		r := selectionResult2496(.84, true)
		r.WorkflowID, r.ProposedWorkflowID, r.RepeatValidationOutcome = "", aaCatalogID2496, aaRejected2496
		res := agentsession.MapInvestigationResultToAgentSessionResult(logger, r, "incident-missing-proposal-2496")
		const assessment = `{"workflow_name":"increase-memory","repeat_validation_outcome":"rejected"}`
		res.SelectionAssessment = &apiextensionsv1.JSON{Raw: []byte(assessment)}
		analysis := buildLowConfidenceFloorTestAnalysis()
		_, err := handlers.NewResponseProcessor(logger, metrics.NewMetrics(), &noopAuditClient{}).
			ProcessAgentSessionResult(context.Background(), analysis, res)
		Expect(err).NotTo(HaveOccurred())
		Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
		Expect(analysis.Status.GetReview().NeedsHumanReview).To(BeTrue())
		Expect(analysis.Status.GetRCAResult().SelectedWorkflow).To(BeNil())
		Expect(analysis.Status.GetRCAResult().SelectionAssessment).To(BeNil())
		Expect(logs.FilterMessage("selection assessment has no proposed workflow identity").Len()).To(Equal(1))
		Expect(string(res.SelectionAssessment.Raw)).To(Equal(assessment))
	})

	It("UT-AA-2496-003: external audit-write failure is observable without discarding partial selection or suppressing human review (BR-KA-016, BR-AUDIT-005, AU-3)", func() {
		core, logs := observer.New(zap.InfoLevel)
		logger := zapr.NewLogger(zap.New(core))
		store := newMockAuditStore()
		store.StoreError = errors.New("external audit store unavailable")
		analysis := buildLowConfidenceFloorTestAnalysis()
		r := selectionResult2496(.84, true)
		_, err := handlers.NewResponseProcessor(logger, metrics.NewMetrics(), aiaudit.NewAuditClient(store, logger)).
			ProcessAgentSessionResult(context.Background(), analysis, agentsession.MapInvestigationResultToAgentSessionResult(logger, r, "incident-audit-failure-2496"))
		Expect(err).NotTo(HaveOccurred(), "existing terminal review handling must not become a retry or execution path")
		Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
		Expect(analysis.Status.GetReview().NeedsHumanReview).To(BeTrue())
		selected := analysis.Status.GetRCAResult().SelectedWorkflow
		Expect(selected).NotTo(BeNil())
		Expect(selected.RepeatJustification).To(Equal(r.RepeatJustification))
		Expect(selected.WorkflowSelectionConfidence).To(HaveValue(Equal(.84)))
		Expect(selected.PriorRemediationUIDs).To(Equal(r.PriorRemediationUIDs))
		Expect(selected.SelectedAt).NotTo(BeNil())
		Expect(store.StoredEvents).To(BeEmpty(), "a failed write is not persisted audit evidence")
		Expect(logs.FilterMessage("Failed to record analysis failure audit").Len()).To(Equal(1))
	})

	DescribeTable("UT-AA-2496-001: selection context survives every selected-workflow preservation path",
		func(confidence float64, review bool, phase string) {
			r := selectionResult2496(confidence, review)
			res := agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), r, "incident-2496")
			Expect(res.SelectedWorkflow).NotTo(BeNil())
			wire := selectionObject2496(res.SelectedWorkflow.Raw)
			Expect(string(wire["repeat_justification"])).To(MatchJSON(`{"evidence_basis":"New memory limit observed","prior_outcome_relevance":"Prior Full assessment was ineffective","expected_mechanism":"Increase the current limit"}`))
			Expect(string(wire["workflow_selection_confidence"])).To(MatchJSON(confidenceJSON2496(confidence)))
			analysis := buildLowConfidenceFloorTestAnalysis()
			processor := handlers.NewResponseProcessor(logr.Discard(), metrics.NewMetrics(), &noopAuditClient{})
			_, err := processor.ProcessAgentSessionResult(context.Background(), analysis, res)
			Expect(err).NotTo(HaveOccurred())
			Expect(analysis.Status.Phase).To(Equal(phase))
			Expect(analysis.Status.GetReview().NeedsHumanReview).To(Equal(phase == aianalysis.PhaseFailed))
			sw := analysis.Status.GetRCAResult().SelectedWorkflow
			Expect(sw).NotTo(BeNil())
			Expect(sw.WorkflowID).To(Equal(r.WorkflowID))
			Expect(sw.WorkflowName).To(Equal(r.WorkflowName))
			Expect(sw.Confidence).To(Equal(confidence))
			Expect(sw.SelectedAt).NotTo(BeNil())
			raw, err := json.Marshal(sw)
			Expect(err).NotTo(HaveOccurred())
			stored := selectionObject2496(raw)
			Expect(string(stored["repeatJustification"])).To(MatchJSON(string(wire["repeat_justification"])))
			Expect(string(stored["workflowSelectionConfidence"])).To(MatchJSON(confidenceJSON2496(confidence)))
			Expect(string(stored["repeatJustificationRequired"])).To(Equal("true"))
			Expect(string(stored["priorWorkflowMatch"])).To(Equal("true"))
			Expect(string(stored["repeatValidationOutcome"])).To(Equal(`"accepted"`))
			Expect(string(stored["handoffDigest"])).To(Equal(`"` + r.HandoffDigest + `"`))
			Expect(string(stored["priorRemediationUIDs"])).To(MatchJSON(`["trusted-prior-2496"]`))
		},
		Entry("success", .84, false, aianalysis.PhaseAnalyzing),
		Entry("explicit zero requires low-confidence review", 0., false, aianalysis.PhaseFailed),
		Entry("explicit human-review partial context", .84, true, aianalysis.PhaseFailed),
	)

	It("UT-AA-2496-002: unknown selection confidence is not fabricated from RCA confidence", func() {
		r := selectionResult2496(0, false)
		r.WorkflowSelectionConfidence = nil
		res := agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), r, "incident-unknown-2496")
		Expect(selectionObject2496(res.SelectedWorkflow.Raw)).NotTo(HaveKey("workflow_selection_confidence"))
		analysis := buildLowConfidenceFloorTestAnalysis()
		_, err := handlers.NewResponseProcessor(logr.Discard(), metrics.NewMetrics(), &noopAuditClient{}).
			ProcessAgentSessionResult(context.Background(), analysis, res)
		Expect(err).NotTo(HaveOccurred())
		Expect(analysis.Status.GetReview().NeedsHumanReview).To(BeTrue())
		raw, err := json.Marshal(analysis.Status.GetRCAResult().SelectedWorkflow)
		Expect(err).NotTo(HaveOccurred())
		Expect(selectionObject2496(raw)).NotTo(HaveKey("workflowSelectionConfidence"))
		Expect(analysis.Status.GetRCAResult().SelectedWorkflow.Confidence).To(BeZero())
	})

	It("UT-AA-2496-001: rejected reasoning is retained only as a bounded non-executable assessment", func() {
		r := selectionResult2496(.84, true)
		r.WorkflowID, r.ProposedWorkflowID = "", aaCatalogID2496
		r.RepeatValidationOutcome = aaRejected2496
		r.RepeatJustification.ExpectedMechanism = "" // The rejected contract is intentionally incomplete.
		r.Parameters = map[string]interface{}{"PRIOR": "prior-parameter-private-private"}
		r.ExecutionBundle = "bundle-private-private"
		res := agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), r, "incident-rejected-2496")
		Expect(res.SelectedWorkflow).To(BeNil())
		raw, err := json.Marshal(res)
		Expect(err).NotTo(HaveOccurred())
		resultJSON := selectionObject2496(raw)
		Expect(resultJSON).To(HaveKey("selectionAssessment"))
		assessment := selectionObject2496(resultJSON["selectionAssessment"])
		Expect(string(assessment["proposed_workflow_id"])).To(Equal(`"catalog-id-2496"`))
		Expect(string(assessment["repeat_validation_outcome"])).To(Equal(`"rejected"`))
		Expect(string(resultJSON["selectionAssessment"])).NotTo(ContainSubstring("private-private"))
		analysis := buildLowConfidenceFloorTestAnalysis()
		_, err = handlers.NewResponseProcessor(logr.Discard(), metrics.NewMetrics(), &noopAuditClient{}).
			ProcessAgentSessionResult(context.Background(), analysis, res)
		Expect(err).NotTo(HaveOccurred())
		Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
		Expect(analysis.Status.GetReview().NeedsHumanReview).To(BeTrue())
		Expect(analysis.Status.GetRCAResult().SelectedWorkflow).To(BeNil())
		raw, err = json.Marshal(analysis.Status.GetRCAResult())
		Expect(err).NotTo(HaveOccurred())
		stored := selectionObject2496(raw)
		Expect(stored).To(HaveKey("selectionAssessment"))
		assessment = selectionObject2496(stored["selectionAssessment"])
		Expect(string(assessment["proposedWorkflowId"])).To(Equal(`"catalog-id-2496"`))
		Expect(string(assessment["repeatValidationOutcome"])).To(Equal(`"rejected"`))
		Expect(string(assessment["workflowSelectionConfidence"])).To(Equal("0.84"))
		Expect(string(assessment["priorRemediationUIDs"])).To(MatchJSON(`["trusted-prior-2496"]`))
	})

	It("UT-AA-2496-001: an oversized rejected explanation remains bounded without becoming a selection", func() {
		r := selectionResult2496(.84, true)
		r.WorkflowID, r.ProposedWorkflowID = "", aaCatalogID2496
		r.RepeatValidationOutcome = aaRejected2496
		r.RepeatJustification.EvidenceBasis = strings.Repeat("x", 2001)
		res := agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), r, "incident-oversized-2496")
		Expect(res.SelectedWorkflow).To(BeNil())
		raw, err := json.Marshal(res)
		Expect(err).NotTo(HaveOccurred())
		assessment := selectionObject2496(selectionObject2496(raw)["selectionAssessment"])
		justification := selectionObject2496(assessment["repeat_justification"])
		Expect(justification).NotTo(HaveKey("evidence_basis"), "unrepresentable rejected parts are omitted, not silently truncated into valid reasoning")
		Expect(string(justification["expected_mechanism"])).To(Equal(`"Increase the current limit"`))
		Expect(string(assessment["repeat_validation_outcome"])).To(Equal(`"rejected"`))
	})

	It("UT-AA-2496-001: no-workflow never fabricates an executable selection or repeat assessment", func() {
		analysis := buildLowConfidenceFloorTestAnalysis()
		processor := handlers.NewResponseProcessor(logr.Discard(), metrics.NewMetrics(), &noopAuditClient{})
		// Each investigation has its own AA. Reprocessing a new result after
		// selectedAt was frozen is not a production lifecycle and must not be
		// used to justify clearing the immutable execution snapshot.
		declined := &katypes.InvestigationResult{RCASummary: "No suitable workflow", HumanReviewNeeded: true, HumanReviewReason: "no_matching_workflows"}
		_, err := processor.ProcessAgentSessionResult(context.Background(), analysis,
			agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), declined, "incident-declined-2496"))
		Expect(err).NotTo(HaveOccurred())
		Expect(analysis.Status.GetRCAResult().SelectedWorkflow).To(BeNil())
		Expect(analysis.Status.GetRCAResult().SelectionAssessment).To(BeNil())
		Expect(analysis.Status.GetReview().NeedsHumanReview).To(BeTrue())
	})

	It("UT-AA-2496-001: generated copies and mapper snapshots do not alias reasoning or trusted links", func() {
		r := selectionResult2496(.84, false)
		res := agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), r, "incident-copy-2496")
		r.RepeatJustification.EvidenceBasis = "mutated source"
		r.PriorRemediationUIDs[0] = "mutated link"
		analysis := buildLowConfidenceFloorTestAnalysis()
		_, err := handlers.NewResponseProcessor(logr.Discard(), metrics.NewMetrics(), &noopAuditClient{}).
			ProcessAgentSessionResult(context.Background(), analysis, res)
		Expect(err).NotTo(HaveOccurred())
		cloned := analysis.DeepCopy()
		cloned.Status.GetRCAResult().SelectedWorkflow.RepeatJustification.EvidenceBasis = "mutated copy"
		cloned.Status.GetRCAResult().SelectedWorkflow.PriorRemediationUIDs[0] = "mutated copy link"
		*cloned.Status.GetRCAResult().SelectedWorkflow.WorkflowSelectionConfidence = 0
		original := analysis.Status.GetRCAResult().SelectedWorkflow
		Expect(original.RepeatJustification.EvidenceBasis).To(Equal("New memory limit observed"))
		Expect(original.PriorRemediationUIDs).To(Equal([]string{"trusted-prior-2496"}))
		Expect(*original.WorkflowSelectionConfidence).To(Equal(.84))
	})
})

func confidenceJSON2496(confidence float64) string {
	raw, err := json.Marshal(confidence)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return string(raw)
}
