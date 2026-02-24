/*
Copyright 2025 Jordi Gil.

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

// Package aianalysis contains unit tests for the AIAnalysis controller ResponseProcessor.
//
// Test Coverage: BR-AI-082 (Recovery flow support)
// Phase 1: Critical unit test gaps for V1.0 readiness (REVISED)
//
// Test Plan: docs/testing/test-plans/AA_COVERAGE_GAP_REMEDIATION_PLAN_V1_0.md
// Gap Analysis: docs/handoff/AA_COVERAGE_ANALYSIS_GAPS_DEC_24_2025.md
//
// IMPORTANT: handleWorkflowResolutionFailureFromRecovery() was removed as dead code
// during Phase 1 implementation (never called, no business value per triage).
// Tests focus on handleRecoveryNotPossible() which is the production code path.
package aianalysis

import (
	"context"

	"github.com/go-faster/jx"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	client "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// Note: noopAuditClient is defined in investigating_handler_test.go
// and is reused across all ResponseProcessor tests in this package

var _ = Describe("ResponseProcessor Recovery Flow", func() {
	var (
		processor *handlers.ResponseProcessor
		analysis  *aianalysisv1.AIAnalysis
		ctx       context.Context
		m         *metrics.Metrics
	)

	BeforeEach(func() {
		// Create metrics with test registry for isolation
		m = metrics.NewMetrics()
		processor = handlers.NewResponseProcessor(logr.Discard(), m, &noopAuditClient{})
		ctx = context.Background()
	})

	// ═══════════════════════════════════════════════════════════════════════════
	// AA-UNIT-RCV-002: Recovery Determined Impossible (can_recover=false)
	// BR-AI-082: Recovery flow support
	// Function: handleRecoveryNotPossible() via ProcessRecoveryResponse()
	// Duration: 1 hour
	// Test Plan: Phase 1, Test 2 (REVISED - tests production code path)
	// ═══════════════════════════════════════════════════════════════════════════

	Context("AA-UNIT-RCV-002: Recovery Determined Impossible", func() {
		BeforeEach(func() {
			analysis = createAnalysisForRecovery()
			analysis.Status.Phase = aianalysis.PhaseAnalyzing
		})

		It("should handle HAPI determination that recovery is impossible (can_recover=false)", func() {
			// GIVEN: An AIAnalysis requesting recovery assessment
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing),
				"Initial phase must be Analyzing")

			// AND: A recovery response indicating recovery is not possible
			recoveryResp := &client.RecoveryResponse{
				IncidentID:         "test-recovery-impossible-001",
				CanRecover:         false,                       // KEY: HAPI says no recovery possible
				Strategies:         []client.RecoveryStrategy{}, // Empty strategies
				AnalysisConfidence: 0.95,                        // High confidence in impossibility
				Warnings: []string{
					"Insufficient context to determine recovery path",
					"Underlying infrastructure issue requires manual fix",
				},
			}

			// WHEN: Processing the recovery response through the production entry point
			result, err := processor.ProcessRecoveryResponse(ctx, analysis, recoveryResp)

			// THEN: Processing should succeed without error
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")
			Expect(result.RequeueAfter).To(BeZero(), "Should not requeue")
			Expect(result.RequeueAfter).To(BeZero(), "No retry for recovery impossibility")

			// AND: Phase should transition to Failed (no recovery possible)
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"Phase must transition to Failed when recovery is impossible")

			// AND: Clear reasoning should be provided
			Expect(analysis.Status.Reason).To(Equal("RecoveryNotPossible"),
				"Reason must indicate recovery impossibility")
			Expect(analysis.Status.SubReason).To(Equal("NoRecoveryStrategy"),
				"SubReason must specify no strategy available")

			// AND: Message should clearly state impossibility
			Expect(analysis.Status.Message).To(ContainSubstring("recovery is not possible"),
				"Message must clearly state impossibility")

			// AND: Investigation ID should be captured
			Expect(analysis.Status.InvestigationID).To(Equal("test-recovery-impossible-001"),
				"Investigation ID must be preserved for traceability")

			// AND: Warnings should be captured for observability
			Expect(analysis.Status.Warnings).To(HaveLen(2),
				"Warnings must be preserved for operational context")
			Expect(analysis.Status.Warnings).To(ContainElement(ContainSubstring("Insufficient context")),
				"Must include HAPI's diagnostic warnings")

			// AND: Completion timestamp should be set
			Expect(analysis.Status.CompletedAt).ToNot(BeNil(),
				"CompletedAt must be set for terminal Failed state")

			// BUSINESS OUTCOME VERIFIED: System prevents futile recovery attempts and provides actionable guidance ✅
		})

		It("should handle no selected workflow scenario (empty strategies)", func() {
			// GIVEN: Recovery response with can_recover=true but no selected workflow
			recoveryResp := &client.RecoveryResponse{
				IncidentID:         "test-no-workflow-001",
				CanRecover:         true,                                            // Says it can recover
				Strategies:         []client.RecoveryStrategy{},                     // But no strategies
				SelectedWorkflow:   client.OptNilRecoveryResponseSelectedWorkflow{}, // No workflow selected
				AnalysisConfidence: 0.60,
				Warnings: []string{
					"No matching recovery workflow available",
				},
			}

			// WHEN: Processing the response
			result, err := processor.ProcessRecoveryResponse(ctx, analysis, recoveryResp)

			// THEN: Should be treated as recovery not possible
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")
			Expect(result.RequeueAfter).To(BeZero(), "Should not requeue")

			// AND: Phase should fail
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"Phase must transition to Failed when no workflow available")

			// AND: Per reconciliation-phases.md v2.1 structured taxonomy
			// Reason is umbrella category, SubReason is specific cause
			Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"),
				"Reason must be umbrella category per structured taxonomy")
			Expect(analysis.Status.SubReason).To(Equal("NoMatchingWorkflows"),
				"SubReason must specify no workflow available (reconciliation-phases.md v2.1:726)")

			// AND: Warning should be captured
			Expect(analysis.Status.Warnings).To(ContainElement(ContainSubstring("No matching")),
				"Must preserve HAPI's explanation")

			// BUSINESS OUTCOME: System handles inconsistent HAPI responses gracefully ✅
		})

		It("should reset consecutive failures counter on successful API call", func() {
			// GIVEN: AIAnalysis with previous failures
			analysis.Status.ConsecutiveFailures = 3

			// AND: Successful recovery response (even if can_recover=false)
			recoveryResp := &client.RecoveryResponse{
				IncidentID:         "test-reset-failures-001",
				CanRecover:         false,
				Strategies:         []client.RecoveryStrategy{},
				AnalysisConfidence: 0.80,
				Warnings:           []string{"Issue self-resolved"},
			}

			// WHEN: Processing the response
			_, err := processor.ProcessRecoveryResponse(ctx, analysis, recoveryResp)

			// THEN: Consecutive failures should be reset
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")
			Expect(analysis.Status.ConsecutiveFailures).To(BeEquivalentTo(0),
				"BR-AI-009: Consecutive failures must be reset on successful API call")

			// BUSINESS OUTCOME: Retry counter properly managed for transient error handling ✅
		})
	})

	// ═══════════════════════════════════════════════════════════════════════════
	// AA-UNIT-RCV-003: Distinguish Scenarios (can_recover=false edge cases)
	// BR-AI-082: Recovery flow support
	// Function: handleRecoveryNotPossible() via ProcessRecoveryResponse()
	// Duration: Included in Test 2
	// Test Plan: Phase 1, Test 3 (REVISED - tests different impossibility scenarios)
	// ═══════════════════════════════════════════════════════════════════════════

	Context("AA-UNIT-RCV-003: Edge Cases for Recovery Impossibility", func() {
		BeforeEach(func() {
			analysis = createAnalysisForRecovery()
			analysis.Status.Phase = aianalysis.PhaseAnalyzing
		})

		It("should handle issue self-resolved scenario (MOCK_NOT_REPRODUCIBLE)", func() {
			// GIVEN: Recovery response indicating issue self-resolved
			recoveryResp := &client.RecoveryResponse{
				IncidentID:         "test-self-resolved-001",
				CanRecover:         false, // No recovery needed
				Strategies:         []client.RecoveryStrategy{},
				AnalysisConfidence: 0.95, // High confidence issue is gone
				Warnings: []string{
					"Original signal no longer present",
					"Resource health checks passing",
				},
			}

			// WHEN: Processing the response
			result, err := processor.ProcessRecoveryResponse(ctx, analysis, recoveryResp)

			// THEN: Should be treated as recovery not possible (not needed)
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")
			Expect(result.RequeueAfter).To(BeZero(), "Should not requeue")
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"Failed because recovery not needed (issue resolved)")

			// AND: Warnings should indicate self-resolution
			Expect(analysis.Status.Warnings).To(Or(
				ContainElement(ContainSubstring("signal no longer present")),
				ContainElement(ContainSubstring("health checks passing")),
			), "Must capture self-resolution indicators")

			// BUSINESS OUTCOME: System recognizes when recovery is unnecessary ✅
		})

		It("should handle high-confidence impossibility (permanent failure)", func() {
			// GIVEN: High confidence that recovery is permanently impossible
			recoveryResp := &client.RecoveryResponse{
				IncidentID:         "test-permanent-fail-001",
				CanRecover:         false,
				Strategies:         []client.RecoveryStrategy{},
				AnalysisConfidence: 0.98, // Very high confidence
				Warnings: []string{
					"Catastrophic failure - manual intervention required",
					"Data corruption detected",
				},
			}

			// WHEN: Processing the response
			_, err := processor.ProcessRecoveryResponse(ctx, analysis, recoveryResp)

			// THEN: Should clearly indicate permanent failure
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"Phase must be Failed for permanent impossibility")
			Expect(analysis.Status.Reason).To(Equal("RecoveryNotPossible"),
				"Reason must indicate impossibility")

			// AND: High-severity warnings preserved
			Expect(analysis.Status.Warnings).To(ContainElement(ContainSubstring("Catastrophic")),
				"Must preserve critical warnings for operator awareness")

			// BUSINESS OUTCOME: System distinguishes permanent vs temporary failures ✅
		})

		It("should handle low-confidence impossibility (uncertain)", func() {
			// GIVEN: Low confidence scenario (HAPI uncertain)
			recoveryResp := &client.RecoveryResponse{
				IncidentID:         "test-uncertain-001",
				CanRecover:         false,
				Strategies:         []client.RecoveryStrategy{},
				AnalysisConfidence: 0.45, // Low confidence
				Warnings: []string{
					"Insufficient diagnostic information",
					"Unable to determine root cause",
				},
			}

			// WHEN: Processing the response
			_, err := processor.ProcessRecoveryResponse(ctx, analysis, recoveryResp)

			// THEN: Should still fail (can't proceed without certainty)
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"Phase must be Failed even with uncertainty")

			// AND: Warnings should indicate diagnostic gap
			Expect(analysis.Status.Warnings).To(Or(
				ContainElement(ContainSubstring("Insufficient")),
				ContainElement(ContainSubstring("Unable to determine")),
			), "Must preserve diagnostic limitations")

			// BUSINESS OUTCOME: System fails safely when uncertain ✅
		})
	})

	// ═══════════════════════════════════════════════════════════════════════════
	// BR-HAPI-197: Human Review Required Flag
	// Test Plan: docs/testing/BR-HAPI-197/aianalysis_test_plan_v1.0.md
	// ═══════════════════════════════════════════════════════════════════════════

	Context("BR-HAPI-197: Human Review Required", func() {
		BeforeEach(func() {
			analysis = &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-human-review",
					Namespace: "default",
					UID:       types.UID("test-uid-hr-001"),
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: "test-rr-001",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: aianalysis.PhaseInvestigating,
				},
			}
		})

		// UT-AA-197-001: Human review required (needs_human_review=true)
		It("should set NeedsHumanReview=true when HAPI requires human review", func() {
			// GIVEN: HAPI response with needs_human_review=true
			needsReview := true
			reason := client.HumanReviewReasonLowConfidence
			hapiResp := &client.IncidentResponse{
				IncidentID:        "test-needs-review-001",
				NeedsHumanReview:  client.NewOptBool(needsReview),
				HumanReviewReason: client.NewOptNilHumanReviewReason(reason),
				Confidence:        0.65, // Below threshold
				Warnings: []string{
					"Confidence below threshold for automatic remediation",
				},
			}

			// WHEN: Processing the incident response
			_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

			// THEN: Processing should succeed
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")

			// AND: NeedsHumanReview should be true
			Expect(analysis.Status.NeedsHumanReview).To(BeTrue(),
				"NeedsHumanReview must be set when HAPI requires human review")

			// AND: HumanReviewReason should be populated
			Expect(analysis.Status.HumanReviewReason).To(Equal("low_confidence"),
				"HumanReviewReason must match HAPI reason")

			// AND: Phase should be Failed (requires human intervention)
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"Phase must be Failed when human review required")
		})

		// UT-AA-197-002: Normal flow (needs_human_review=false)
		It("should set NeedsHumanReview=false when HAPI response is successful", func() {
			// GIVEN: Successful HAPI response (problem resolved, no workflow needed)
			// BR-HAPI-200: High confidence + no workflow = problem resolved
			hapiResp := &client.IncidentResponse{
				IncidentID:       "test-success-001",
				NeedsHumanReview: client.NewOptBool(false),
				Confidence:       0.85, // High confidence
				// Note: SelectedWorkflow not set (problem resolved case)
			}

			// WHEN: Processing the incident response
			_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

			// THEN: Processing should succeed
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")

			// AND: NeedsHumanReview should remain false
			Expect(analysis.Status.NeedsHumanReview).To(BeFalse(),
				"NeedsHumanReview must be false for successful responses")

			// AND: HumanReviewReason should be empty
			Expect(analysis.Status.HumanReviewReason).To(BeEmpty(),
				"HumanReviewReason must be empty when no review needed")

			// AND: Phase should be Completed (problem resolved)
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted),
				"Phase must be Completed for problem resolved case")
		})

		// UT-AA-197-003: Map all 7 human_review_reason values (rca_incomplete added via BR-HAPI-212)
		// Note: rca_incomplete test deferred until HAPI OpenAPI spec updated
		DescribeTable("should map all human_review_reason enum values",
			func(reason client.HumanReviewReason, expectedString string) {
				// GIVEN: HAPI response with specific human_review_reason
				hapiResp := &client.IncidentResponse{
					IncidentID:        "test-reason-mapping",
					NeedsHumanReview:  client.NewOptBool(true),
					HumanReviewReason: client.NewOptNilHumanReviewReason(reason),
					Confidence:        0.50,
					Warnings: []string{
						"Human review required: " + expectedString,
					},
				}

				// WHEN: Processing the incident response
				_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

				// THEN: Processing should succeed
				Expect(err).ToNot(HaveOccurred(), "Processing should succeed")

				// AND: HumanReviewReason should match exactly
				Expect(analysis.Status.HumanReviewReason).To(Equal(expectedString),
					"HumanReviewReason must match HAPI reason exactly (no transformation)")

				// AND: NeedsHumanReview should be true
				Expect(analysis.Status.NeedsHumanReview).To(BeTrue(),
					"NeedsHumanReview must be true when reason is provided")
			},
			Entry("workflow_not_found",
				client.HumanReviewReasonWorkflowNotFound,
				"workflow_not_found"),
			Entry("no_matching_workflows",
				client.HumanReviewReasonNoMatchingWorkflows,
				"no_matching_workflows"),
			Entry("low_confidence",
				client.HumanReviewReasonLowConfidence,
				"low_confidence"),
			Entry("llm_parsing_error",
				client.HumanReviewReasonLlmParsingError,
				"llm_parsing_error"),
			Entry("parameter_validation_failed",
				client.HumanReviewReasonParameterValidationFailed,
				"parameter_validation_failed"),
			Entry("image_mismatch",
				client.HumanReviewReasonImageMismatch,
				"image_mismatch"),
			Entry("investigation_inconclusive",
				client.HumanReviewReasonInvestigationInconclusive,
				"investigation_inconclusive"),
			// TODO: Add rca_incomplete test once HAPI OpenAPI spec is updated with BR-HAPI-212 enum value
		)
	})

	// ═══════════════════════════════════════════════════════════════════════════
	// BR-HAPI-200.6: Incident Response Classification Fix
	// Bug: ProcessIncidentResponse misclassifies "active problem, no workflow"
	//       as "ProblemResolved" when confidence >= 0.7 regardless of
	//       needs_human_review or warning signals.
	// Fix: Align decision tree with BR-HAPI-200.6 detection patterns:
	//   Outcome A (ProblemResolved): needs_human_review=false AND
	//       selected_workflow=null AND confidence >= 0.7 AND no warning signals
	//   Outcome B (WorkflowResolutionFailed): needs_human_review=true
	// ═══════════════════════════════════════════════════════════════════════════

	Context("BR-HAPI-200.6: Incident Response Classification", func() {
		BeforeEach(func() {
			analysis = &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-classification",
					Namespace: "default",
					UID:       types.UID("test-uid-class-001"),
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: "test-rr-class-001",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: aianalysis.PhaseInvestigating,
				},
			}
		})

		// UT-AA-200-BUG-001: Investigation inconclusive with high confidence
		// Reproduces the memory-eater bug: HAPI says needs_human_review=true with
		// investigation_inconclusive, but high confidence causes misclassification
		// as ProblemResolved instead of WorkflowResolutionFailed.
		It("UT-AA-200-BUG-001: should route to WorkflowResolutionFailed when needs_human_review=true even with high confidence", func() {
			// GIVEN: HAPI response with explicit needs_human_review=true
			// AND: investigation_inconclusive reason (BR-HAPI-200 Outcome B)
			// AND: high confidence (0.95) -- LLM is confident in its analysis
			// AND: no workflow selected (problem is real, but no automated fix)
			hapiResp := &client.IncidentResponse{
				IncidentID:        "test-inconclusive-high-conf-001",
				NeedsHumanReview:  client.NewOptBool(true),
				HumanReviewReason: client.NewOptNilHumanReviewReason(client.HumanReviewReasonInvestigationInconclusive),
				Confidence:        0.95, // High confidence -- this is the key trigger for the bug
				Warnings: []string{
					"Investigation inconclusive - human review recommended",
				},
			}

			// WHEN: Processing the incident response
			_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

			// THEN: Processing should succeed
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")

			// AND: Phase MUST be Failed (NOT Completed)
			// BUG: Current code routes to handleProblemResolvedFromIncident → Phase=Completed
			// FIX: needsHumanReview check must take priority → Phase=Failed
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"BR-HAPI-200.6 AC-5: Phase must be Failed for inconclusive investigation")

			// AND: Reason must indicate workflow resolution failure
			Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"),
				"BR-HAPI-200.6 AC-5: Reason must be WorkflowResolutionFailed")

			// AND: SubReason must reflect the inconclusive investigation
			Expect(analysis.Status.SubReason).To(Equal("InvestigationInconclusive"),
				"BR-HAPI-200.6 AC-5: SubReason must be InvestigationInconclusive")

			// AND: NeedsHumanReview MUST be true (HAPI explicitly said so)
			Expect(analysis.Status.NeedsHumanReview).To(BeTrue(),
				"BR-HAPI-197: HAPI's needs_human_review=true must be respected")

			// AND: HumanReviewReason must be preserved
			Expect(analysis.Status.HumanReviewReason).To(Equal("investigation_inconclusive"),
				"BR-HAPI-200: HumanReviewReason must be investigation_inconclusive")
		})

		// UT-AA-200-BUG-002: No matching workflows with high confidence
		// HAPI correctly sets needs_human_review=true when catalog search found
		// no matching workflows, but high confidence causes misclassification.
		It("UT-AA-200-BUG-002: should route to WorkflowResolutionFailed when needs_human_review=true with no_matching_workflows", func() {
			// GIVEN: HAPI response with needs_human_review=true
			// AND: no_matching_workflows reason (catalog had no match)
			// AND: high confidence (LLM is confident about its RCA)
			hapiResp := &client.IncidentResponse{
				IncidentID:        "test-no-match-high-conf-001",
				NeedsHumanReview:  client.NewOptBool(true),
				HumanReviewReason: client.NewOptNilHumanReviewReason(client.HumanReviewReasonNoMatchingWorkflows),
				Confidence:        0.80,
				Warnings: []string{
					"No workflows matched the search criteria",
				},
			}

			// WHEN: Processing the incident response
			_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

			// THEN: Processing should succeed
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")

			// AND: Phase MUST be Failed (NOT Completed)
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"Phase must be Failed when no matching workflows found")

			// AND: Reason must indicate workflow resolution failure
			Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"),
				"Reason must be WorkflowResolutionFailed")

			// AND: NeedsHumanReview must be true
			Expect(analysis.Status.NeedsHumanReview).To(BeTrue(),
				"NeedsHumanReview must be true when HAPI says so")

			// AND: HumanReviewReason must be preserved
			Expect(analysis.Status.HumanReviewReason).To(Equal("no_matching_workflows"),
				"HumanReviewReason must be no_matching_workflows")
		})

		// UT-AA-200-BUG-003: Defense-in-depth -- LLM incorrectly overrides needs_human_review
		// Edge case: LLM sets needs_human_review=false but HAPI still appends
		// "inconclusive" warnings. The warnings-based check must catch this.
		It("UT-AA-200-BUG-003: should NOT classify as ProblemResolved when warnings indicate inconclusive investigation", func() {
			// GIVEN: HAPI response where LLM incorrectly set needs_human_review=false
			// BUT: HAPI's result_parser added "inconclusive" warning
			// AND: high confidence, no workflow
			hapiResp := &client.IncidentResponse{
				IncidentID:       "test-warning-override-001",
				NeedsHumanReview: client.NewOptBool(false), // LLM incorrectly overrode
				Confidence:       0.85,                     // High confidence
				Warnings: []string{
					"Investigation inconclusive - human review recommended", // HAPI safety signal
				},
			}

			// WHEN: Processing the incident response
			_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

			// THEN: Processing should succeed
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")

			// AND: Phase MUST be Failed (defense-in-depth catches via warnings)
			// Even though needs_human_review=false, the "inconclusive" warning
			// prevents misclassification as ProblemResolved
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"Defense-in-depth: inconclusive warnings must prevent ProblemResolved classification")

			// AND: Must require human review (set by NoWorkflowTerminalFailure path)
			Expect(analysis.Status.NeedsHumanReview).To(BeTrue(),
				"NoWorkflowTerminalFailure path sets NeedsHumanReview=true")

			// AND: Reason must indicate failure, not success
			Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"),
				"Reason must be WorkflowResolutionFailed, NOT WorkflowNotNeeded")
		})

		// UT-AA-200-REG-001: Genuine problem resolved (regression test)
		// Ensures the fix doesn't break the legitimate ProblemResolved path.
		It("UT-AA-200-REG-001: should classify as ProblemResolved when genuinely resolved with self-resolved warning", func() {
			// GIVEN: HAPI response confirming problem self-resolved
			// AND: needs_human_review=false (LLM and HAPI agree: no review needed)
			// AND: high confidence, no workflow
			// AND: "Problem self-resolved" warning (HAPI's resolved outcome signal)
			hapiResp := &client.IncidentResponse{
				IncidentID:       "test-genuine-resolved-001",
				NeedsHumanReview: client.NewOptBool(false),
				Confidence:       0.92, // Very high confidence
				Warnings: []string{
					"Problem self-resolved - no remediation required",
					"Pod automatically recovered from OOMKilled event",
				},
			}

			// WHEN: Processing the incident response
			_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

			// THEN: Processing should succeed
			Expect(err).ToNot(HaveOccurred(), "Processing should succeed")

			// AND: Phase MUST be Completed (genuine problem resolution)
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted),
				"BR-HAPI-200.6 AC-4: Phase must be Completed for genuine resolution")

			// AND: Reason must indicate no workflow needed
			Expect(analysis.Status.Reason).To(Equal("WorkflowNotNeeded"),
				"BR-HAPI-200.6 AC-4: Reason must be WorkflowNotNeeded")

			// AND: SubReason must indicate problem resolved
			Expect(analysis.Status.SubReason).To(Equal("ProblemResolved"),
				"BR-HAPI-200.6 AC-4: SubReason must be ProblemResolved")

			// AND: NeedsHumanReview must be false
			Expect(analysis.Status.NeedsHumanReview).To(BeFalse(),
				"No human review needed for genuinely resolved problems")
		})
	})
})

// ═══════════════════════════════════════════════════════════════════════════
// Helper Functions
// ═══════════════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════════════
// Issue #97: ExtractRootCauseAnalysis helper tests
// Validates centralized RCA extraction including affectedResource
// ═══════════════════════════════════════════════════════════════════════════

var _ = Describe("ExtractRootCauseAnalysis", func() {
	Context("with standard RCA fields", func() {
		It("should extract summary, severity, and contributing factors", func() {
			rcaData := client.IncidentResponseRootCauseAnalysis{
				"summary":              jx.Raw(`"OOM caused by memory leak"`),
				"severity":             jx.Raw(`"high"`),
				"contributing_factors": jx.Raw(`["Memory leak in main container","No resource limits set"]`),
			}

			rca := handlers.ExtractRootCauseAnalysis(rcaData)

			Expect(rca).NotTo(BeNil())
			Expect(rca.Summary).To(Equal("OOM caused by memory leak"))
			Expect(rca.Severity).To(Equal("high"))
			Expect(rca.ContributingFactors).To(HaveLen(2))
			Expect(rca.ContributingFactors).To(ContainElement("Memory leak in main container"))
			Expect(rca.AffectedResource).To(BeNil(), "No affectedResource in input")
		})
	})

	Context("with affectedResource present", func() {
		It("should extract affectedResource from RCA", func() {
			rcaData := client.IncidentResponseRootCauseAnalysis{
				"summary":  jx.Raw(`"OOM caused by memory leak"`),
				"severity": jx.Raw(`"critical"`),
				"affectedResource": jx.Raw(`{
					"kind": "Deployment",
					"name": "api-server",
					"namespace": "production"
				}`),
			}

			rca := handlers.ExtractRootCauseAnalysis(rcaData)

			Expect(rca).NotTo(BeNil())
			Expect(rca.AffectedResource).NotTo(BeNil())
			Expect(rca.AffectedResource.Kind).To(Equal("Deployment"))
			Expect(rca.AffectedResource.Name).To(Equal("api-server"))
			Expect(rca.AffectedResource.Namespace).To(Equal("production"))
		})
	})

	Context("with empty or nil RCA data", func() {
		It("should return nil for nil input", func() {
			rca := handlers.ExtractRootCauseAnalysis(nil)
			Expect(rca).To(BeNil())
		})

		It("should return empty RCA for empty map (callers guard with len check)", func() {
			rca := handlers.ExtractRootCauseAnalysis(client.IncidentResponseRootCauseAnalysis{})
			// Note: ExtractRootCauseAnalysis returns non-nil with empty fields
			// because GetMapFromOptNil succeeds with {}. All callers guard with
			// `if len(resp.RootCauseAnalysis) > 0` before calling.
			Expect(rca).NotTo(BeNil())
			Expect(rca.Summary).To(BeEmpty())
			Expect(rca.AffectedResource).To(BeNil())
		})
	})

	Context("with signal_name present (Issue #118 Gap 2)", func() {
		It("UT-AA-RCA-ST-001: should extract SignalType from RCA response", func() {
			rcaData := client.IncidentResponseRootCauseAnalysis{
				"summary":     jx.Raw(`"OOM caused by memory leak"`),
				"severity":    jx.Raw(`"high"`),
				"signal_name": jx.Raw(`"OOMKilled"`),
				"contributing_factors": jx.Raw(`["Memory leak in main container"]`),
			}

			rca := handlers.ExtractRootCauseAnalysis(rcaData)

			Expect(rca).NotTo(BeNil())
			Expect(rca.SignalType).To(Equal("OOMKilled"),
				"SignalType must be extracted from HAPI response root_cause_analysis.signal_name")
		})
	})

	Context("with affectedResource missing required fields", func() {
		It("should not populate affectedResource when kind is empty", func() {
			rcaData := client.IncidentResponseRootCauseAnalysis{
				"summary":  jx.Raw(`"test summary"`),
				"severity": jx.Raw(`"low"`),
				"affectedResource": jx.Raw(`{
					"kind": "",
					"name": "api-server",
					"namespace": "production"
				}`),
			}

			rca := handlers.ExtractRootCauseAnalysis(rcaData)

			Expect(rca).NotTo(BeNil())
			Expect(rca.AffectedResource).To(BeNil(),
				"affectedResource should be nil when kind is empty")
		})
	})
})

// createAnalysisForRecovery creates an AIAnalysis for recovery testing
func createAnalysisForRecovery() *aianalysisv1.AIAnalysis {
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-recovery-analysis",
			Namespace: "default",
			UID:       types.UID("test-uid-recovery-001"),
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			// Per DD-RECOVERY-002, recovery context is in spec
			// (simplified for unit test, full context in integration tests)
		},
		Status: aianalysisv1.AIAnalysisStatus{
			Phase: aianalysis.PhaseAnalyzing,
		},
	}
}
