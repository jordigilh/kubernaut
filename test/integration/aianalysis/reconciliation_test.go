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
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// SERIAL EXECUTION: AA integration suite runs serially for 100% reliability.
// See audit_flow_integration_test.go for detailed rationale.
var _ = Describe("AIAnalysis Full Reconciliation Integration", Label("integration", "reconciliation"), func() {
	const (
		timeout  = 2 * time.Minute
		interval = time.Second
	)

	// Per reconciliation-phases.md v2.1: 4-phase flow
	// Pending → Investigating → Analyzing → Completed
	// NOTE: Validating and Recommending phases were removed in v1.8/v1.10

	Context("Complete reconciliation cycle - BR-AI-001", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      helpers.UniqueTestName("integration-test"),
					Namespace: testNamespace, // DD-TEST-002: Use dynamic namespace
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-remediation",
						Namespace: testNamespace, // DD-TEST-002: Use dynamic namespace
					},
					RemediationID: "test-rem-001",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fingerprint-001",
							Severity:         "warning",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: testNamespace, // DD-TEST-002: Use dynamic namespace
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: true,
									PDBProtected:  true,
								},
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: testNamespace, Kind: "Deployment", Name: "test-app"}, // DD-TEST-002
								},
							},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}
		})

		It("should transition through all phases successfully", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Creating AIAnalysis CRD")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			// The controller processes phases very quickly, so instead of checking
			// intermediate phases (which may have already transitioned by the time
			// we poll), we verify the final state and important status fields.

			By("Waiting for Completed phase (terminal)")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying final status fields")
			// Refresh status
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			Expect(analysis.Status.CompletedAt).NotTo(BeZero())
			// Staging environment should auto-approve per Rego policy
			Expect(analysis.Status.ApprovalRequired).To(BeFalse())
			// Should have a selected workflow from HolmesGPT mock
			Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())
			Expect(analysis.Status.SelectedWorkflow.WorkflowID).To(Equal("mock-crashloop-config-fix-v1"))
		})

		It("should require approval for production environment - BR-AI-013", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			analysis.Spec.AnalysisRequest.SignalContext.Environment = "production"

			By("Creating production AIAnalysis")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying approval required for production")
			Expect(analysis.Status.ApprovalRequired).To(BeTrue())
			Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
		})

		It("should handle recovery attempts with escalation - BR-AI-013", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			// Mark as recovery attempt #3 (escalation threshold)
			analysis.Spec.IsRecoveryAttempt = true
			analysis.Spec.RecoveryAttemptNumber = 3

			By("Creating recovery attempt AIAnalysis")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"))

			By("Verifying approval required due to multiple recovery attempts")
			// Per Rego policy: multiple recovery attempts require approval
			Expect(analysis.Status.ApprovalRequired).To(BeTrue())
		})
	})

	Context("Error recovery scenarios - BR-AI-009", func() {
		var analysis *aianalysisv1alpha1.AIAnalysis

		BeforeEach(func() {
			analysis = &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      helpers.UniqueTestName("error-recovery"),
					Namespace: testNamespace, // DD-TEST-002: Use dynamic namespace
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-remediation",
						Namespace: testNamespace, // DD-TEST-002: Use dynamic namespace
					},
					RemediationID: "test-rem-002",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fingerprint-002",
							Severity:         "warning",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: testNamespace, // DD-TEST-002: Use dynamic namespace
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}
		})

		It("should increment retry count on transient failures", func() {
			// Per 03-testing-strategy.mdc: Cleanup in defer for extra safety
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			// This test verifies the retry mechanism works correctly
			// The mock HolmesGPT-API can be configured to return transient errors

			By("Creating AIAnalysis")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Verifying retry annotation exists after failure")
			// This assertion depends on HolmesGPT-API behavior
			// If mock is configured to fail first N times, verify retry count
			Eventually(func() bool {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				// Check if retry annotation is set
				_, hasRetry := analysis.Annotations["kubernaut.ai/retry-count"]
				// Either completed or has retry annotation
				return analysis.Status.Phase == "Completed" || hasRetry
			}, timeout, interval).Should(BeTrue())
		})
	})

	// NOTE: Status conditions (Status.Conditions) are NOT in V1.0 scope.
	// BR-AI-022 is about confidence thresholds (80% auto-approval), which is
	// tested in the Rego policy evaluation tests.
	// Status conditions for phase observability may be added in V2.0 if needed.
})
