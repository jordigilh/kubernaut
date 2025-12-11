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
		timeout  = 3 * time.Minute
		interval = 2 * time.Second
	)

	// ========================================
	// BR-AI-080: Support recovery attempts
	// ========================================
	Context("BR-AI-080: Recovery attempt support", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-basic-" + randomSuffix(),
					Namespace: "kubernaut-system",
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-basic",
						Namespace: "kubernaut-system",
					},
					RemediationID:         "e2e-recovery-rem-001",
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 1,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-recovery-fp-001",
							Severity:         "critical",
							SignalType:       "OOMKilled",
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "oom-service-abc123",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: true,
									GitOpsTool:    "argocd",
								},
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: "staging", Kind: "Deployment", Name: "oom-service"},
								},
							},
						},
						AnalysisTypes: []string{"recovery-analysis", "workflow-selection"},
					},
				},
			}
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, analysis)
		})

		It("should complete recovery analysis using recovery endpoint", func() {
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

			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-with-history-" + randomSuffix(),
					Namespace: "kubernaut-system",
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-history",
						Namespace: "kubernaut-system",
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
								ContainerImage: "quay.io/kubernaut/workflow-oomkill:v1.0.0",
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
							SignalType:       "OOMKilled",
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "memory-hungry-app",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: true,
								},
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: "staging", Kind: "Deployment", Name: "memory-hungry-app"},
								},
							},
						},
						AnalysisTypes: []string{"recovery-analysis", "workflow-selection"},
					},
				},
			}
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, analysis)
		})

		It("should consider previous execution failures in analysis", func() {
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
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-routing-" + randomSuffix(),
					Namespace: "kubernaut-system",
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-routing",
						Namespace: "kubernaut-system",
					},
					RemediationID:         "e2e-recovery-rem-003",
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 1,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-recovery-fp-003",
							Severity:         "warning",
							SignalType:       "CrashLoopBackOff",
							Environment:      "development",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "dev-app",
								Namespace: "development",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: false,
								},
							},
						},
						AnalysisTypes: []string{"recovery-analysis"},
					},
				},
			}
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, analysis)
		})

		It("should use recovery endpoint for IsRecoveryAttempt=true", func() {
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

			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-multi-" + randomSuffix(),
					Namespace: "kubernaut-system",
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-multi",
						Namespace: "kubernaut-system",
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
								ContainerImage: "quay.io/kubernaut/workflow-oomkill:v1.0.0",
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
								ContainerImage: "quay.io/kubernaut/workflow-restart:v1.0.0",
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
							SignalType:       "OOMKilled",
							Environment:      "staging", // Even staging requires approval for 3+ attempts
							BusinessPriority: "P0",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "critical-service",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: true,
									PDBProtected:  true,
								},
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: "staging", Kind: "Deployment", Name: "critical-service"},
								},
							},
						},
						AnalysisTypes: []string{"recovery-analysis", "workflow-selection"},
					},
				},
			}
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, analysis)
		})

		It("should require approval for third recovery attempt", func() {
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
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-conditions-" + randomSuffix(),
					Namespace: "kubernaut-system",
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-rr-recovery-conditions",
						Namespace: "kubernaut-system",
					},
					RemediationID:         "e2e-recovery-rem-005",
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 1,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-recovery-fp-005",
							Severity:         "warning",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
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

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, analysis)
		})

		It("should populate all conditions during recovery flow", func() {
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
})


