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

package aianalysis

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// ========================================
// Test Mocks
// ========================================

// noopAnalyzingAuditClient is a no-op implementation of AnalyzingAuditClientInterface for unit tests.
type noopAnalyzingAuditClient struct{}

func (n *noopAnalyzingAuditClient) RecordRegoEvaluation(ctx context.Context, analysis *aianalysisv1.AIAnalysis, outcome string, degraded bool, durationMs int, reason string) {
	// No-op: Unit tests don't need audit recording
}

func (n *noopAnalyzingAuditClient) RecordApprovalDecision(ctx context.Context, analysis *aianalysisv1.AIAnalysis, decision string, reason string) {
	// No-op: Unit tests don't need audit recording
}

// AA-BUG-008: RecordPhaseTransition removed from AnalyzingAuditClientInterface
// Phase transitions are recorded by controller only, not by handlers

func (n *noopAnalyzingAuditClient) RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
	// No-op: Unit tests don't need audit recording
}

func (n *noopAnalyzingAuditClient) RecordAnalysisFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) error {
	// No-op: Unit tests don't need audit recording
	return nil
}

// BR-AI-012: AnalyzingHandler tests
var _ = Describe("AnalyzingHandler", func() {
	var (
		handler       *handlers.AnalyzingHandler
		mockEvaluator *mocks.MockRegoEvaluator
		ctx           context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockEvaluator = mocks.NewMockRegoEvaluator()
		// Create mock audit client (noop for unit tests) and metrics
		mockAuditClient := &noopAnalyzingAuditClient{}
		testMetrics := metrics.NewMetrics()
		handler = handlers.NewAnalyzingHandler(mockEvaluator, ctrl.Log.WithName("test"), testMetrics, mockAuditClient)
	})

	// Helper to create valid AIAnalysis in Analyzing phase
	createTestAnalysis := func() *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-analysis",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Kind:      "RemediationRequest",
					Name:      "test-rr",
					Namespace: "default",
				},
				RemediationID: "test-remediation-001",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "test-fingerprint",
						Severity:         "warning",
						SignalName:       "OOMKilled",
						Environment:      "production",
						BusinessPriority: "P0",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
					AnalysisTypes: []string{"investigation", "analysis"},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysis.PhaseAnalyzing,
				// Simulating data from InvestigatingHandler
				RootCause: "OOM caused by memory leak",
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID:     "wf-restart-pod",
					ExecutionBundle: "kubernaut.io/workflows/restart:v1.0.0",
					Confidence:     0.92,
					Rationale:      "Selected for OOM recovery",
				},
			},
		}
	}

	Describe("Handle", func() {
		// BR-AI-012: Successful Rego evaluation with approval required
		// Per reconciliation-phases.md v2.0: Analyzing → Completed (no Recommending phase)
		Context("when Rego evaluation requires approval", func() {
			BeforeEach(func() {
				mockEvaluator.WithApprovalRequired("Production environment requires approval")
			})

			// BR-AI-012: Business outcome - analysis completes with approval decision
			It("should complete analysis and require approval for production", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Business outcome: Analysis completes successfully
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted), "Analysis should reach terminal Completed phase")
				Expect(analysis.Status.CompletedAt).NotTo(BeNil(), "Completion timestamp should be set")
				// Business outcome: Production requires approval
				Expect(analysis.Status.ApprovalRequired).To(BeTrue(), "Production environment should require approval")
				Expect(analysis.Status.ApprovalReason).NotTo(BeEmpty(), "Approval reason should explain why approval is needed")
				Expect(analysis.Status.ApprovalReason).To(ContainSubstring("Production"), "Reason should mention production")
			})

			// BR-AI-019: ApprovalContext population
			It("should populate ApprovalContext", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
				Expect(analysis.Status.ApprovalContext.Reason).To(Equal("Production environment requires approval"))
				Expect(analysis.Status.ApprovalContext.WhyApprovalRequired).To(Equal("Production environment requires approval"))
			})

			// BR-AI-019: Confidence level classification for operator visibility
			// Business Value: Operators can quickly assess AI confidence without reading raw numbers
			DescribeTable("should populate ApprovalContext with correct confidence level",
				func(confidenceScore float64, expectedLevel string) {
					analysis := createTestAnalysis()
					analysis.Status.SelectedWorkflow.Confidence = confidenceScore

					_, err := handler.Handle(ctx, analysis)

					Expect(err).NotTo(HaveOccurred())
					Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
					Expect(analysis.Status.ApprovalContext.ConfidenceScore).To(BeNumerically("~", confidenceScore, 0.01))
					Expect(analysis.Status.ApprovalContext.ConfidenceLevel).To(Equal(expectedLevel))
				},
				// High confidence (≥0.8): AI is very confident
				Entry("0.92 → high (above threshold)", 0.92, "high"),
				Entry("0.80 → high (at threshold)", 0.80, "high"),
				Entry("0.95 → high (near perfect)", 0.95, "high"),
				// Medium confidence (0.6-0.8): AI has reasonable confidence
				Entry("0.79 → medium (just below high threshold)", 0.79, "medium"),
				Entry("0.70 → medium (typical medium)", 0.70, "medium"),
				Entry("0.60 → medium (at threshold)", 0.60, "medium"),
				// Low confidence (<0.6): AI has low confidence - needs careful review
				Entry("0.59 → low (just below medium threshold)", 0.59, "low"),
				Entry("0.40 → low (typical low)", 0.40, "low"),
				Entry("0.10 → low (very uncertain)", 0.10, "low"),
			)

			It("should populate ApprovalContext with RecommendedActions from SelectedWorkflow", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
				Expect(analysis.Status.ApprovalContext.RecommendedActions).To(HaveLen(1))
				Expect(analysis.Status.ApprovalContext.RecommendedActions[0].Action).To(Equal("wf-restart-pod"))
			})

			// BR-AI-019: Investigation summary from RootCauseAnalysis
			// Business Value: Operators see AI's investigation findings in approval context
			It("should populate ApprovalContext with InvestigationSummary from RCA", func() {
				analysis := createTestAnalysis()
				analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
					Summary:             "Memory leak in application container",
					ContributingFactors: []string{"OOM killed event", "Gradual memory increase over 2 hours"},
				}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
				Expect(analysis.Status.ApprovalContext.InvestigationSummary).To(Equal("Memory leak in application container"))
				Expect(analysis.Status.ApprovalContext.EvidenceCollected).To(HaveLen(2))
				Expect(analysis.Status.ApprovalContext.EvidenceCollected).To(ContainElement("OOM killed event"))
			})

			// BR-AI-019: Edge case - no RCA available
			// Business Value: System handles missing investigation gracefully
			It("should handle missing RootCauseAnalysis gracefully", func() {
				analysis := createTestAnalysis()
				analysis.Status.RootCauseAnalysis = nil

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
				Expect(analysis.Status.ApprovalContext.InvestigationSummary).To(BeEmpty())
				Expect(analysis.Status.ApprovalContext.EvidenceCollected).To(BeNil())
			})

			// BR-AI-019: AlternativesConsidered from AlternativeWorkflows
			// Business Value: Operators can see what other options AI considered
			It("should populate ApprovalContext with AlternativesConsidered", func() {
				analysis := createTestAnalysis()
				analysis.Status.AlternativeWorkflows = []aianalysisv1.AlternativeWorkflow{
					{
						WorkflowID:     "wf-scale-up",
						ExecutionBundle: "kubernaut.io/workflows/scale:v1.0.0",
						Confidence:     0.75,
						Rationale:      "Could scale up instead of restart",
					},
					{
						WorkflowID:     "wf-rollback",
						ExecutionBundle: "kubernaut.io/workflows/rollback:v1.0.0",
						Confidence:     0.60,
						Rationale:      "Could rollback to previous version",
					},
				}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
				Expect(analysis.Status.ApprovalContext.AlternativesConsidered).To(HaveLen(2))
				Expect(analysis.Status.ApprovalContext.AlternativesConsidered[0].Approach).To(Equal("wf-scale-up"))
				Expect(analysis.Status.ApprovalContext.AlternativesConsidered[1].Approach).To(Equal("wf-rollback"))
			})

			// BR-AI-019: Edge case - no alternatives available
			// Business Value: System handles single workflow scenarios
			It("should handle empty AlternativeWorkflows gracefully", func() {
				analysis := createTestAnalysis()
				analysis.Status.AlternativeWorkflows = nil

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
				Expect(analysis.Status.ApprovalContext.AlternativesConsidered).To(BeNil())
			})

			// BR-AI-019: Business outcome - operator can see policy decision in approval context
			It("should include policy evaluation details for operator visibility", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalContext).NotTo(BeNil(), "ApprovalContext should exist for manual review")
				Expect(analysis.Status.ApprovalContext.PolicyEvaluation).NotTo(BeNil(), "Policy evaluation should be visible to operator")
				// Business outcome: Operator sees clear decision status
				Expect(analysis.Status.ApprovalContext.PolicyEvaluation.Decision).To(
					BeElementOf("manual_review_required", "auto_approved", "degraded_mode"),
					"Decision should be one of the valid business outcomes",
				)
			})
		})

		// BR-AI-012: Successful Rego evaluation with auto-approve
		// Per reconciliation-phases.md v2.0: Analyzing → Completed (no Recommending phase)
		Context("when Rego evaluation auto-approves", func() {
			BeforeEach(func() {
				mockEvaluator.WithAutoApprove("Non-production environment - auto-approved")
			})

			// BR-AI-012: Business outcome - non-production auto-approves without operator intervention
			It("should auto-approve and complete without requiring operator action", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.Environment = "development"

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Business outcome: Analysis completes for auto-approved scenarios
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted), "Analysis should complete")
				Expect(analysis.Status.CompletedAt).NotTo(BeNil(), "Completion timestamp should be set")
				// Business outcome: No approval needed - workflow can proceed immediately
				Expect(analysis.Status.ApprovalRequired).To(BeFalse(), "Non-production should auto-approve")
				// Business outcome: No ApprovalContext needed when auto-approved (reduces operator noise)
				Expect(analysis.Status.ApprovalContext).To(BeNil(), "No approval context for auto-approved analysis")
			})
		})

		// Issue #118 Gap 3: TotalAnalysisTime computation
		Context("when analysis completes with StartedAt set", func() {
			BeforeEach(func() {
				mockEvaluator.WithAutoApprove("Non-production environment - auto-approved")
			})

			It("UT-AA-TAT-001: should compute TotalAnalysisTime in seconds from StartedAt to CompletedAt", func() {
				analysis := createTestAnalysis()
				analysis.Status.StartedAt = &metav1.Time{Time: time.Now().Add(-30 * time.Second)}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted))
				Expect(analysis.Status.TotalAnalysisTime).To(BeNumerically(">=", int64(29)),
					"TotalAnalysisTime must be computed as CompletedAt - StartedAt in seconds")
				Expect(analysis.Status.TotalAnalysisTime).To(BeNumerically("<=", int64(35)),
					"TotalAnalysisTime should be approximately 30 seconds")
			})
		})

		// BR-AI-014: Business outcome - graceful degradation ensures safety when policy fails
		Context("when Rego evaluation fails gracefully (degraded mode)", func() {
			BeforeEach(func() {
				mockEvaluator.WithDegradedMode("Policy evaluation failed - defaulting to manual approval")
			})

			// BR-AI-014: Business outcome - system remains safe when policy infrastructure fails
			It("should complete with safe defaults requiring operator approval", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Business outcome: Analysis completes (doesn't block remediation flow)
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted), "Should complete despite policy failure")
				Expect(analysis.Status.CompletedAt).NotTo(BeNil())
				// Business outcome: Safe default - requires operator approval when uncertain
				Expect(analysis.Status.ApprovalRequired).To(BeTrue(), "Degraded mode should require approval (safe default)")
				// Business outcome: Operator sees degraded mode indicator
				Expect(analysis.Status.DegradedMode).To(BeTrue(), "Operator should know system is in degraded mode")
				Expect(analysis.Status.ApprovalReason).To(ContainSubstring("failed"), "Reason should explain degradation")
			})

			// BR-AI-019: Business outcome - operator sees clear degraded status
			It("should inform operator of degraded mode in approval context", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalContext).NotTo(BeNil(), "ApprovalContext should exist for operator")
				Expect(analysis.Status.ApprovalContext.PolicyEvaluation).NotTo(BeNil())
				Expect(analysis.Status.ApprovalContext.PolicyEvaluation.Decision).To(Equal("degraded_mode"),
					"Operator should see clear 'degraded_mode' status")
			})
		})

		// BR-AI-018: Business outcome - analysis fails gracefully when no workflow available
		Context("when SelectedWorkflow is missing", func() {
			It("should fail analysis with clear explanation for operator", func() {
				analysis := createTestAnalysis()
				analysis.Status.SelectedWorkflow = nil // Simulate missing workflow from HolmesGPT-API

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Business outcome: Analysis terminates (doesn't hang)
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "Should fail when no workflow available")
				// Business outcome: Clear reason for operators/debugging
				Expect(analysis.Status.Reason).To(Equal("NoWorkflowSelected"), "Reason should identify the issue")
				Expect(analysis.Status.Message).To(ContainSubstring("workflow"), "Message should explain missing workflow")
			})

			It("should not evaluate policies when no workflow exists", func() {
				analysis := createTestAnalysis()
				analysis.Status.SelectedWorkflow = nil

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.AssertNotCalled()).To(Succeed())
			})
		})

		// BR-AI-012: Policy input construction
		Context("policy input construction", func() {
			It("should pass correct environment to evaluator", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.Environment = "staging"

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.Environment).To(Equal("staging"))
			})

			It("should pass confidence from SelectedWorkflow", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.Confidence).To(BeNumerically("~", 0.92, 0.01))
			})

			// ADR-055: TargetInOwnerChain replaced by AffectedResource
			It("should pass AffectedResource from RCA status", func() {
				analysis := createTestAnalysis()
				analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
					Summary:  "OOM detected",
					Severity: "high",
					AffectedResource: &aianalysisv1.AffectedResource{
						Kind:      "Deployment",
						Name:      "api-server",
						Namespace: "production",
					},
				}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.AffectedResource).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.AffectedResource.Kind).To(Equal("Deployment"))
				Expect(mockEvaluator.LastInput.AffectedResource.Name).To(Equal("api-server"))
				Expect(mockEvaluator.LastInput.AffectedResource.Namespace).To(Equal("production"))
			})

			It("should pass Warnings from status", func() {
				analysis := createTestAnalysis()
				analysis.Status.Warnings = []string{"High memory pressure", "Node scheduling delayed"}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.Warnings).To(HaveLen(2))
				Expect(mockEvaluator.LastInput.Warnings).To(ContainElement("High memory pressure"))
			})

			// BR-AI-012: Extended PolicyInput fields (per IMPLEMENTATION_PLAN_V1.0.md)
			It("should pass signal context fields", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.SignalName = "OOMKilled"
				analysis.Spec.AnalysisRequest.SignalContext.Severity = "critical"
				analysis.Spec.AnalysisRequest.SignalContext.BusinessPriority = "P0"

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.SignalType).To(Equal("OOMKilled"))
				Expect(mockEvaluator.LastInput.Severity).To(Equal("critical"))
				Expect(mockEvaluator.LastInput.BusinessPriority).To(Equal("P0"))
			})

			It("should pass target resource fields", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Kind = "Deployment"
				analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Name = "my-app"
				analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Namespace = "production"

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.TargetResource.Kind).To(Equal("Deployment"))
				Expect(mockEvaluator.LastInput.TargetResource.Name).To(Equal("my-app"))
				Expect(mockEvaluator.LastInput.TargetResource.Namespace).To(Equal("production"))
			})

			It("should pass recovery context fields", func() {
				analysis := createTestAnalysis()
				analysis.Spec.IsRecoveryAttempt = true
				analysis.Spec.RecoveryAttemptNumber = 3

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.IsRecoveryAttempt).To(BeTrue())
				Expect(mockEvaluator.LastInput.RecoveryAttemptNumber).To(Equal(3))
			})

			It("should default recovery fields when not recovery", func() {
				analysis := createTestAnalysis()
				analysis.Spec.IsRecoveryAttempt = false

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.IsRecoveryAttempt).To(BeFalse())
				Expect(mockEvaluator.LastInput.RecoveryAttemptNumber).To(Equal(0))
			})

			// BR-AI-012: CustomLabels population from EnrichmentResults
			It("should pass CustomLabels from EnrichmentResults", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext = &sharedtypes.KubernetesContext{
					CustomLabels: map[string][]string{
						"constraint": {"cost-constrained", "stateful-safe"},
						"team":       {"name=payments"},
						"region":     {"us-east-1"},
					},
				}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.CustomLabels).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.CustomLabels["constraint"]).To(ContainElement("cost-constrained"))
				Expect(mockEvaluator.LastInput.CustomLabels["constraint"]).To(ContainElement("stateful-safe"))
				Expect(mockEvaluator.LastInput.CustomLabels["team"]).To(ContainElement("name=payments"))
				Expect(mockEvaluator.LastInput.CustomLabels["region"]).To(ContainElement("us-east-1"))
			})

			// BR-AI-012: Empty CustomLabels returns empty map
			It("should return empty map when CustomLabels is nil", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.KubernetesContext = &sharedtypes.KubernetesContext{
					CustomLabels: nil,
				}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.CustomLabels).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.CustomLabels).To(BeEmpty())
			})

			// BR-SP-002, BR-SP-080, BR-SP-081: BusinessClassification pass-through to Rego policy
			It("should pass BusinessClassification from EnrichmentResults to policy input", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.BusinessClassification = &sharedtypes.BusinessClassification{
					BusinessUnit:   "payments",
					ServiceOwner:   "team-checkout",
					Criticality:    "critical",
					SLARequirement: "platinum",
				}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.BusinessClassification).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.BusinessClassification).To(HaveKeyWithValue("business_unit", "payments"))
				Expect(mockEvaluator.LastInput.BusinessClassification).To(HaveKeyWithValue("service_owner", "team-checkout"))
				Expect(mockEvaluator.LastInput.BusinessClassification).To(HaveKeyWithValue("criticality", "critical"))
				Expect(mockEvaluator.LastInput.BusinessClassification).To(HaveKeyWithValue("sla_requirement", "platinum"))
			})

			It("should not set BusinessClassification when nil in EnrichmentResults", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.BusinessClassification = nil

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.BusinessClassification).To(BeNil())
			})

			It("should map only populated BusinessClassification fields", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.BusinessClassification = &sharedtypes.BusinessClassification{
					Criticality: "high",
				}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.BusinessClassification).To(HaveLen(1))
				Expect(mockEvaluator.LastInput.BusinessClassification).To(HaveKeyWithValue("criticality", "high"))
			})
		})

		// BR-AI-012: Evaluator is called exactly once
		Context("evaluator invocation", func() {
			It("should call evaluator exactly once", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.AssertCalled(1)).To(Succeed())
			})
		})
	})

	// BR-AI-012: Name method
	Describe("Name", func() {
		It("should return 'analyzing'", func() {
			Expect(handler.Name()).To(Equal("analyzing"))
		})
	})
})
