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
			Expect(result.Requeue).To(BeFalse(), "Should not requeue")
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
			Expect(result.Requeue).To(BeFalse(), "Should not requeue")

			// AND: Phase should fail
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"Phase must transition to Failed when no workflow available")

			// AND: Reason should indicate recovery not possible
			Expect(analysis.Status.Reason).To(Equal("RecoveryNotPossible"),
				"Reason must indicate recovery impossibility")
			Expect(analysis.Status.SubReason).To(Equal("NoRecoveryStrategy"),
				"SubReason must specify no strategy available")

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
			Expect(result.Requeue).To(BeFalse(), "Should not requeue")
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
})

// ═══════════════════════════════════════════════════════════════════════════
// Helper Functions
// ═══════════════════════════════════════════════════════════════════════════

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
