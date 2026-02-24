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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Recovery Flow E2E Tests
// Per reconciliation-phases.md v2.2 and DD-RECOVERY-002:
// - Recovery attempts use /api/v1/recovery/analyze endpoint
// - PreviousExecutions slice contains ALL failed attempts
// - IsRecoveryAttempt=true routes to recovery endpoint
// - RecoveryAttemptNumber increments with each attempt

var _ = Describe("Recovery Flow E2E", Label("e2e", "recovery"), func() {
	const (
		timeout  = 30 * time.Second       // Allow time for HolmesGPT-API recovery endpoint to be ready
		interval = 500 * time.Millisecond // Poll twice per second
	)

	// ========================================
	// BR-AI-080: Support recovery attempts
	// ========================================
	Context("BR-AI-080: Recovery attempt support", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			namespace := createTestNamespace("recovery-basic")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-basic-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-basic",
						Namespace: namespace,
					},
					RemediationID:         "e2e-recovery-rem-001",
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 1,
					PreviousExecutions: []aianalysisv1alpha1.PreviousExecution{
						{
							WorkflowExecutionRef: "workflow-exec-initial",
							OriginalRCA: aianalysisv1alpha1.OriginalRCA{
								Summary:    "Pod OOMKilled due to insufficient memory limits",
								SignalType: "OOMKilled",
								Severity:   "critical",
							},
							SelectedWorkflow: aianalysisv1alpha1.SelectedWorkflowSummary{
								WorkflowID:     "oomkill-increase-memory-v1",
								ExecutionBundle: "quay.io/kubernaut/workflow-oomkill:v1.0.0",
								Rationale:      "Increase memory limits for OOMKilled pod",
							},
							Failure: aianalysisv1alpha1.ExecutionFailure{
								Reason:        "WorkflowFailed",
								Message:       "Memory increase insufficient",
								FailedAt:      metav1.Now(),
								ExecutionTime: "45s",
							},
						},
					},
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
						Fingerprint:      "e2e-recovery-fp-001",
						Severity:         "critical",
						SignalName:       "OOMKilled",
						Environment:      "staging",
						BusinessPriority: "P1", // Must match oomkill-increase-memory-v1 catalog entry
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment", // Must match workflow component label "deployment"
								Name:      "oom-service-abc123",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"recovery-analysis", "workflow-selection"},
					},
				},
			}
		})

		It("should complete recovery analysis using recovery endpoint", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating recovery AIAnalysis")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying recovery was processed")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// Should have selected a workflow
			Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())

			// InvestigationID should be populated
			Expect(analysis.Status.InvestigationID).NotTo(BeEmpty())
		})
	})

	// ========================================
	// BR-AI-081: Previous execution context
	// ========================================
	Context("BR-AI-081: Previous execution context handling", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			// Create analysis with full previous execution context
			failedAt := metav1.Now()

			namespace := createTestNamespace("recovery-history")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-with-history-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-history",
						Namespace: namespace,
					},
					RemediationID:         "e2e-recovery-rem-002",
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 2, // Second attempt
					PreviousExecutions: []aianalysisv1alpha1.PreviousExecution{
						{
							WorkflowExecutionRef: "workflow-exec-001",
							OriginalRCA: aianalysisv1alpha1.OriginalRCA{
								Summary:    "Memory limit exceeded due to traffic spike",
								SignalType: "OOMKilled",
								Severity:   "critical",
							},
							SelectedWorkflow: aianalysisv1alpha1.SelectedWorkflowSummary{
								WorkflowID:     "oomkill-increase-memory-v1",
								ExecutionBundle: "quay.io/kubernaut/workflow-oomkill:v1.0.0",
								Rationale:      "Conservative memory increase for OOMKilled pod",
							},
							Failure: aianalysisv1alpha1.ExecutionFailure{
								FailedStepIndex: 2,
								FailedStepName:  "apply-memory-patch",
								Reason:          "Forbidden",
								Message:         "RBAC denied: cannot patch deployments",
								ExitCode:        func() *int32 { i := int32(1); return &i }(),
								FailedAt:        failedAt,
								ExecutionTime:   "45s",
							},
						},
					},
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
						Fingerprint:      "e2e-recovery-fp-002",
						Severity:         "critical",
						SignalName:       "OOMKilled",
						Environment:      "staging",
						BusinessPriority: "P1", // Must match oomkill-increase-memory-v1 catalog entry
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment", // Must match workflow component label "deployment"
								Name:      "memory-hungry-app",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"recovery-analysis", "workflow-selection"},
					},
				},
			}
		})

		It("should consider previous execution failures in analysis", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating recovery AIAnalysis with previous execution context")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying recovery processed previous execution")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// Should have workflow selected (potentially different from failed one)
			Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())

			// HAPI should consider RBAC failure and avoid similar approach
			// We can't assert the exact workflow, but it should complete successfully
			Expect(analysis.Status.CompletedAt).NotTo(BeNil())
		})
	})

	// ========================================
	// BR-AI-082: Recovery endpoint routing
	// ========================================
	Context("BR-AI-082: Recovery endpoint routing verification", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			namespace := createTestNamespace("recovery-routing")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-routing-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-routing",
						Namespace: namespace,
					},
					RemediationID:         "e2e-recovery-rem-003",
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 1,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
						Fingerprint:      "e2e-recovery-fp-003",
						Severity:         "high",  // Must match crashloop-config-fix-v1 catalog entry
						SignalName:       "CrashLoopBackOff",
						Environment:      "staging", // Must be in [staging,production,test]
						BusinessPriority: "P1",      // Must match crashloop-config-fix-v1 catalog entry
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment", // Must match workflow component label "deployment"
								Name:      "dev-app",
								Namespace: "development",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"recovery-analysis"},
					},
				},
			}
		})

		It("should use recovery endpoint for IsRecoveryAttempt=true", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating recovery AIAnalysis")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for investigation phase to complete")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				phase := string(analysis.Status.Phase)
				// Either still investigating or already completed
				return phase
			}, timeout, interval).Should(SatisfyAny(
				Equal("Analyzing"),
				Equal("Completed"),
			))

			By("Verifying investigation was performed")
			// If we get past Investigating, the recovery endpoint was called successfully
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// InvestigationID proves HAPI was called
			// For recovery, this may take slightly longer
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return analysis.Status.InvestigationID
			}, timeout, interval).ShouldNot(BeEmpty())
		})
	})

	// ========================================
	// BR-AI-083: Multi-attempt recovery escalation
	// ========================================
	Context("BR-AI-083: Multi-attempt recovery escalation", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			failedAt := metav1.Now()

			namespace := createTestNamespace("recovery-multi")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-multi-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-multi",
						Namespace: namespace,
					},
					RemediationID:         "e2e-recovery-rem-004",
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 3, // Third attempt = escalation
					PreviousExecutions: []aianalysisv1alpha1.PreviousExecution{
						{
							WorkflowExecutionRef: "workflow-exec-001",
							OriginalRCA: aianalysisv1alpha1.OriginalRCA{
								Summary:    "Pod OOMKilled",
								SignalType: "OOMKilled",
								Severity:   "critical",
							},
							SelectedWorkflow: aianalysisv1alpha1.SelectedWorkflowSummary{
								WorkflowID:     "oomkill-increase-memory-v1",
								ExecutionBundle: "quay.io/kubernaut/workflow-oomkill:v1.0.0",
								Rationale:      "Increase memory limit",
							},
							Failure: aianalysisv1alpha1.ExecutionFailure{
								FailedStepIndex: 1,
								FailedStepName:  "increase-memory",
								Reason:          "Forbidden",
								Message:         "RBAC denied",
								FailedAt:        failedAt,
								ExecutionTime:   "30s",
							},
						},
						{
							WorkflowExecutionRef: "workflow-exec-002",
							OriginalRCA: aianalysisv1alpha1.OriginalRCA{
								Summary:    "Pod OOMKilled (second attempt)",
								SignalType: "OOMKilled",
								Severity:   "critical",
							},
							SelectedWorkflow: aianalysisv1alpha1.SelectedWorkflowSummary{
								WorkflowID:     "oomkill-restart-pod-v1",
								ExecutionBundle: "quay.io/kubernaut/workflow-restart:v1.0.0",
								Rationale:      "Restart pod as fallback",
							},
							Failure: aianalysisv1alpha1.ExecutionFailure{
								FailedStepIndex: 0,
								FailedStepName:  "delete-pod",
								Reason:          "PodEvictionFailure",
								Message:         "PDB would be violated",
								FailedAt:        failedAt,
								ExecutionTime:   "15s",
							},
						},
					},
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-recovery-fp-004",
							Severity:         "critical",
							SignalName:       "OOMKilled",
							Environment:      "staging", // Even staging requires approval for 3+ attempts
							BusinessPriority: "P1",      // Must match oomkill-increase-memory-v1 catalog entry
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment", // Must match workflow component label "deployment"
								Name:      "critical-service",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"recovery-analysis", "workflow-selection"},
					},
				},
			}
		})

		It("should require approval for third recovery attempt", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating third recovery attempt AIAnalysis")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying approval required for escalated recovery")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// Per Rego policy: 3+ recovery attempts require approval
			Expect(analysis.Status.ApprovalRequired).To(BeTrue())

			// Approval context should mention recovery escalation
			Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
		})
	})

	// ========================================
	// Kubernetes Conditions verification
	// ========================================
	Context("Conditions population during recovery flow", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			namespace := createTestNamespace("recovery-conditions")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-conditions-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-conditions",
						Namespace: namespace,
					},
					RemediationID:         "e2e-recovery-rem-005",
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 1,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
						Fingerprint:      "e2e-recovery-fp-005",
						Severity:         "high",  // Must match crashloop-config-fix-v1 catalog entry
						SignalName:       "CrashLoopBackOff",
						Environment:      "staging",
						BusinessPriority: "P1", // Must match crashloop-config-fix-v1 catalog entry
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment", // Must match workflow component label "deployment"
								Name:      "test-app",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"recovery-analysis"},
					},
				},
			}
		})

		It("should populate all conditions during recovery flow", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating recovery AIAnalysis")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying conditions are populated")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// Should have conditions populated
			Expect(analysis.Status.Conditions).NotTo(BeEmpty())

			// Find InvestigationComplete condition
			var hasInvestigationComplete, hasAnalysisComplete bool
			for _, cond := range analysis.Status.Conditions {
				if cond.Type == "InvestigationComplete" && cond.Status == "True" {
					hasInvestigationComplete = true
				}
				if cond.Type == "AnalysisComplete" && cond.Status == "True" {
					hasAnalysisComplete = true
				}
			}

			Expect(hasInvestigationComplete).To(BeTrue(), "InvestigationComplete condition should be True")
			Expect(hasAnalysisComplete).To(BeTrue(), "AnalysisComplete condition should be True")
		})
	})

	// ========================================
	// BR-HAPI-197: Recovery Human Review Support
	// ========================================
	Context("BR-HAPI-197: Recovery human review when workflow resolution fails", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			namespace := createTestNamespace("recovery-human-review")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-human-review-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-hr",
						Namespace: namespace,
					},
					RemediationID:         "e2e-recovery-hr-rem-001",
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 1,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint: "e2e-recovery-hr-fp-001",
							Severity:    "critical",
							// MOCK_NO_WORKFLOW_FOUND triggers HAPI mock edge case:
							// - needs_human_review=true
							// - human_review_reason="no_matching_workflows"
							SignalName:       "MOCK_NO_WORKFLOW_FOUND",
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "failing-pod-hr",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"recovery-analysis", "workflow-selection"},
					},
					PreviousExecutions: []aianalysisv1alpha1.PreviousExecution{
						{
							WorkflowExecutionRef: "workflow-exec-hr-001",
							OriginalRCA: aianalysisv1alpha1.OriginalRCA{
								Summary:    "Previous workflow failed",
								SignalType: "CrashLoopBackOff",
								Severity:   "critical",
							},
							SelectedWorkflow: aianalysisv1alpha1.SelectedWorkflowSummary{
								WorkflowID:     "failed-workflow-v1",
								ExecutionBundle: "quay.io/kubernaut/workflow-failed:v1.0.0",
							},
							Failure: aianalysisv1alpha1.ExecutionFailure{
								Reason:        "WorkflowFailed",
								Message:       "Previous workflow execution failed",
								FailedAt:      metav1.Now(),
								ExecutionTime: "60s",
							},
						},
					},
				},
			}
		})

		AfterEach(func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			if analysis != nil {
				_ = k8sClient.Delete(ctx, analysis)
			}
		})

		It("should transition to Failed when HAPI returns needs_human_review=true", func() {
			// BR-HAPI-197: When HAPI cannot provide reliable recovery workflow recommendation,
			// it returns needs_human_review=true with structured reason.
			// AIAnalysis should transition to PhaseFailed and NOT create WorkflowExecution.
			//
			// Test Strategy: Use HAPI mock mode edge case signal type to trigger human review
			// Signal: MOCK_NO_WORKFLOW_FOUND → needs_human_review=true, reason=no_matching_workflows

			// Act: Create the AIAnalysis
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			// Assert: Verify transitions to Failed phase (not AwaitingApproval)
			// This is the key E2E validation: full CRD lifecycle shows user-visible Failed state
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				if err != nil {
					return ""
				}
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Failed"),
				"AIAnalysis should transition to Failed when HAPI returns needs_human_review=true")

			// Assert: Verify human review details are populated in status
			Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"),
				"Status.Reason should indicate workflow resolution failed")

			Expect(string(analysis.Status.SubReason)).To(Equal("NoMatchingWorkflows"),
				"Status.SubReason should map human_review_reason enum to SubReason")

			Expect(analysis.Status.CompletedAt).ToNot(BeNil(),
				"Status.CompletedAt should be set when analysis completes with human review required")

			Expect(analysis.Status.Message).To(ContainSubstring("could not provide reliable"),
				"Status.Message should explain human review is required")

			Expect(analysis.Status.Message).To(ContainSubstring("no_matching_workflows"),
				"Status.Message should include human_review_reason for debugging")

			// E2E Success: Full CRD lifecycle validated
			// - Controller reconciled AIAnalysis
			// - Called HAPI /recovery/analyze
			// - Detected needs_human_review=true
			// - Transitioned to PhaseFailed (not AwaitingApproval)
			// - Set CompletedAt timestamp
			// - Populated Reason, SubReason, Message
			// - User observes correct status via kubectl
		})
	})
})
