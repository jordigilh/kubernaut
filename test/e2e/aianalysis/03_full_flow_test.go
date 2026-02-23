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
		// Uses 30s timeout to match SetDefaultEventuallyTimeout (per RCA Jan 31, 2026)
		// Allows controller initialization + reconciliation time
		timeout  = 30 * time.Second       // Matches suite default (was 10s - too short)
		interval = 500 * time.Millisecond // Poll twice per second
	)

	// Per reconciliation-phases.md v2.1: 4-phase flow
	// Pending → Investigating → Analyzing → Completed

	Context("Production incident analysis - BR-AI-001", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			namespace := createTestNamespace("full-flow-prod")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-prod-incident-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation",
						Namespace: namespace,
					},
					RemediationID: "e2e-rem-001",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-001",
							Severity:        "medium",
							SignalName:       "CrashLoopBackOff",
							Environment:      "production",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment",
								Name:      "payment-service",
								Namespace: "payments",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								KubernetesContext: &sharedtypes.KubernetesContext{
									CustomLabels: map[string][]string{
										"team":        {"payments"},
										"cost_center": {"revenue"},
									},
								},
							},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}
		})

		It("should complete full 4-phase reconciliation cycle", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating AIAnalysis for production incident")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for 4-phase reconciliation to complete")
			// NOTE: In E2E tests with mock LLM, reconciliation completes in <1 second
			// (vs 30-60s in production with real LLM latency). We cannot reliably observe
			// intermediate phases (Pending → Investigating → Analyzing) because the
			// controller processes faster than Kubernetes watch latency and test polling.
			// Instead, we verify the final "Completed" state and business outcomes.
			// Per reconciliation-phases.md v2.1: Pending → Investigating → Analyzing → Completed
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying final status")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// Production should require approval per Rego policy
			Expect(analysis.Status.ApprovalRequired).To(BeTrue())

			// Should have workflow selected
			Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())

			// Should have completion timestamp
			Expect(analysis.Status.CompletedAt).NotTo(BeZero())

			// E2E-AA-163-002: TotalAnalysisTime populated by controller
			Expect(analysis.Status.TotalAnalysisTime).To(BeNumerically(">", 0))

			// E2E-AA-163-001: RootCauseAnalysis populated from mock LLM response
			Expect(analysis.Status.RootCauseAnalysis).NotTo(BeNil())
			Expect(analysis.Status.RootCauseAnalysis.Summary).NotTo(BeEmpty())
			Expect(analysis.Status.RootCauseAnalysis.Severity).To(BeElementOf("critical", "high", "medium", "low", "unknown"))
			Expect(analysis.Status.RootCauseAnalysis.SignalType).NotTo(BeEmpty())
			Expect(analysis.Status.RootCauseAnalysis.ContributingFactors).To(ContainElement("identified_by_mock_llm"))

			// E2E-AA-163-001: AffectedResource populated from mock LLM (crashloop scenario returns Deployment)
			Expect(analysis.Status.RootCauseAnalysis.AffectedResource).NotTo(BeNil())
			Expect(analysis.Status.RootCauseAnalysis.AffectedResource.Kind).To(Equal("Deployment"))

			// E2E-AA-163-002: Condition assertions for completed AA
			Expect(analysis.Status.Conditions).To(ContainElements(
				And(HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionTrue)),
				And(HaveField("Type", "InvestigationComplete"), HaveField("Status", metav1.ConditionTrue)),
				And(HaveField("Type", "AnalysisComplete"), HaveField("Status", metav1.ConditionTrue)),
				And(HaveField("Type", "WorkflowResolved"), HaveField("Status", metav1.ConditionTrue)),
			))
		})

		It("should require approval for production environment - BR-AI-013", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

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
			namespace := createTestNamespace("full-flow-staging")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-staging-incident-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation-staging",
						Namespace: namespace,
					},
					RemediationID: "e2e-rem-002",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-002",
							Severity:        "medium",
							SignalName:       "OOMKilled",
							Environment:      "staging", // Non-production = auto-approve
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "web-app",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation", "workflow-selection"},
					},
				},
			}
		})

		It("should auto-approve for staging environment", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

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
			namespace := createTestNamespace("full-flow-recovery")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-recovery-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation-recovery",
						Namespace: namespace,
					},
					RemediationID: "e2e-rem-003",
					// Recovery attempt fields
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 3, // 3+ attempts = escalation
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
						Fingerprint:      "e2e-fingerprint-003",
						Severity:         "high",    // Must match crashloop-config-fix-v1 catalog entry
						SignalName:       "CrashLoopBackOff",
						Environment:      "staging", // Even staging requires approval for 3+ recovery attempts
						BusinessPriority: "P1",      // Matches crashloop-config-fix-v1 catalog entry
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment", // Must match workflow component label "deployment"
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

		It("should require approval for multiple recovery attempts", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

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
			namespace := createTestNamespace("full-flow-data-quality")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-data-quality-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation-dq",
						Namespace: namespace,
					},
					RemediationID: "e2e-rem-004",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-004",
							Severity:        "medium",
							SignalName:       "CrashLoopBackOff",
							Environment:      "production",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-app",
								Namespace: "production",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}
		})

		It("should require approval for data quality issues in production", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

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

	// E2E-AA-163-003: AlternativeWorkflows - Mock LLM low_confidence scenario returns alternative_workflows
	Context("Low confidence scenario - alternative workflows", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			namespace := createTestNamespace("full-flow-low-conf")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-low-conf-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation-low-conf",
						Namespace: namespace,
					},
					RemediationID: "e2e-rem-low-conf",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-low-conf",
							Severity:         "medium",
							SignalName:       "MOCK_LOW_CONFIDENCE", // Triggers mock scenario with alternative_workflows
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "web-app",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}
		})

		It("should populate AlternativeWorkflows when mock returns alternatives - E2E-AA-163-003", func() {
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating AIAnalysis with MOCK_LOW_CONFIDENCE signal type")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for Failed phase (BR-HAPI-197 AC-4: confidence 0.35 < 0.7 threshold -> Failed/LowConfidence)")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Failed"))

			By("Verifying failure reason per BR-HAPI-197 AC-4")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
			Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"))
			Expect(analysis.Status.SubReason).To(Equal("LowConfidence"))
			Expect(analysis.Status.NeedsHumanReview).To(BeTrue(),
				"NeedsHumanReview must be true when confidence < threshold (BR-HAPI-197)")

			By("Verifying AlternativeWorkflows populated (mock low_confidence scenario returns exactly 2 alternatives)")
			Expect(analysis.Status.AlternativeWorkflows).To(HaveLen(2))
			Expect(analysis.Status.AlternativeWorkflows[0].WorkflowID).To(Equal("d3c95ea1-66cb-6bf2-c59e-7dd27f1fec6d"))
			Expect(analysis.Status.AlternativeWorkflows[0].Rationale).To(Equal("Alternative approach for ambiguous root cause"))
			Expect(analysis.Status.AlternativeWorkflows[1].WorkflowID).To(Equal("e4d06fb2-77dc-7cg3-d60f-8ee38g2gfd7e"))
			Expect(analysis.Status.AlternativeWorkflows[1].Rationale).To(Equal("Requires human expertise to determine correct remediation"))
			Expect(analysis.Status.AlternativeWorkflows[0].Confidence).To(BeNumerically(">", analysis.Status.AlternativeWorkflows[1].Confidence))
		})
	})

	// E2E-AA-163-004: ValidationAttemptsHistory - Mock LLM max_retries_exhausted scenario
	Context("Max retries exhausted - validation attempts history", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			namespace := createTestNamespace("full-flow-max-retries")
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-max-retries-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "e2e-remediation-max-retries",
						Namespace: namespace,
					},
					RemediationID: "e2e-rem-max-retries",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-fingerprint-max-retries",
							Severity:         "high",
							SignalName:       "MOCK_MAX_RETRIES_EXHAUSTED", // Triggers mock scenario with 3 failed validation attempts
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "llm-parse-fail-pod",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}
		})

		It("should populate ValidationAttemptsHistory with 3 failed attempts - E2E-AA-163-004", func() {
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating AIAnalysis with MOCK_MAX_RETRIES_EXHAUSTED signal type")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for phase to reach Failed or Completed (max_retries returns no workflow)")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Or(Equal("Failed"), Equal("Completed")))

			By("Verifying ValidationAttemptsHistory populated with 3 attempts")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
			Expect(analysis.Status.ValidationAttemptsHistory).To(HaveLen(3))
			for i, attempt := range analysis.Status.ValidationAttemptsHistory {
				Expect(attempt.Attempt).To(Equal(i + 1))
				Expect(attempt.IsValid).To(BeFalse())
				Expect(attempt.Errors).NotTo(BeEmpty())
			}
		})
	})
})
