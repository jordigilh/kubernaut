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

var _ = Describe("Full User Journey E2E", Label("e2e", "full-flow"), func() {
	const (
		timeout  = 3 * time.Minute
		interval = 2 * time.Second
	)

	// Per reconciliation-phases.md v2.1: 4-phase flow
	// Pending → Investigating → Analyzing → Completed

	Context("Production incident analysis - BR-AI-001", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-prod-incident-" + randomSuffix(),
					Namespace: "kubernaut-system",
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation",
						Namespace: "kubernaut-system",
					},
					RemediationID: "e2e-rem-001",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-001",
							Severity:         "warning",
							SignalType:       "CrashLoopBackOff",
							Environment:      "production",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment",
								Name:      "payment-service",
								Namespace: "payments",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged:   true,
									GitOpsTool:      "argocd",
									PDBProtected:    true,
									HPAEnabled:      true,
									NetworkIsolated: true,
									ServiceMesh:     "istio",
								},
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: "payments", Kind: "Deployment", Name: "payment-service"},
								},
								CustomLabels: map[string][]string{
									"team":        {"payments"},
									"cost_center": {"revenue"},
								},
							},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}
		})

		AfterEach(func() {
			// Cleanup
			_ = k8sClient.Delete(ctx, analysis)
		})

		It("should complete full 4-phase reconciliation cycle", func() {
			By("Creating AIAnalysis for production incident")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Verifying phase transitions through 4-phase flow")
			// Per reconciliation-phases.md v2.1: Pending → Investigating → Analyzing → Completed
			phases := []string{"Pending", "Investigating", "Analyzing", "Completed"}

			for _, expectedPhase := range phases {
				By("Waiting for phase: " + expectedPhase)
				Eventually(func() string {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
					return string(analysis.Status.Phase)
				}, timeout, interval).Should(Equal(expectedPhase))
			}

			By("Verifying final status")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// Production should require approval per Rego policy
			Expect(analysis.Status.ApprovalRequired).To(BeTrue())

			// Should have workflow selected
			Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())

			// Should have completion timestamp
			Expect(analysis.Status.CompletedAt).NotTo(BeZero())

			// Should capture targetInOwnerChain
			Expect(analysis.Status.TargetInOwnerChain).NotTo(BeNil())
		})

		It("should require approval for production environment - BR-AI-013", func() {
			By("Creating production AIAnalysis")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying approval required")
			Expect(analysis.Status.ApprovalRequired).To(BeTrue())
			Expect(analysis.Status.ApprovalReason).NotTo(BeEmpty())
			Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
		})
	})

	Context("Staging incident analysis - auto-approve", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-staging-incident-" + randomSuffix(),
					Namespace: "kubernaut-system",
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation-staging",
						Namespace: "kubernaut-system",
					},
					RemediationID: "e2e-rem-002",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-002",
							Severity:         "warning",
							SignalType:       "OOMKilled",
							Environment:      "staging", // Non-production = auto-approve
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "web-app",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: true,
								},
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: "staging", Kind: "Deployment", Name: "web-app"},
								},
							},
						},
						AnalysisTypes: []string{"investigation", "workflow-selection"},
					},
				},
			}
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, analysis)
		})

		It("should auto-approve for staging environment", func() {
			By("Creating staging AIAnalysis")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying auto-approved (no approval required)")
			Expect(analysis.Status.ApprovalRequired).To(BeFalse())
		})
	})

	Context("Recovery attempt escalation - BR-AI-013", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-" + randomSuffix(),
					Namespace: "kubernaut-system",
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation-recovery",
						Namespace: "kubernaut-system",
					},
					RemediationID: "e2e-rem-003",
					// Recovery attempt fields
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 3, // 3+ attempts = escalation
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-003",
							Severity:         "critical",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging", // Even staging requires approval for 3+ recovery attempts
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "critical-app",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, analysis)
		})

		It("should require approval for multiple recovery attempts", func() {
			By("Creating recovery attempt AIAnalysis")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying approval required due to recovery escalation")
			// Per Rego policy: 3+ recovery attempts require approval
			Expect(analysis.Status.ApprovalRequired).To(BeTrue())
		})
	})

	Context("Data quality warnings - BR-AI-011", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-data-quality-" + randomSuffix(),
					Namespace: "kubernaut-system",
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation-dq",
						Namespace: "kubernaut-system",
					},
					RemediationID: "e2e-rem-004",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-004",
							Severity:         "warning",
							SignalType:       "CrashLoopBackOff",
							Environment:      "production",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-app",
								Namespace: "production",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									// FailedDetections indicates data quality issues
									FailedDetections: []string{"gitOpsManaged"},
								},
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, analysis)
		})

		It("should require approval for data quality issues in production", func() {
			By("Creating AIAnalysis with detection failures")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying approval required due to data quality")
			Expect(analysis.Status.ApprovalRequired).To(BeTrue())
		})
	})
})


