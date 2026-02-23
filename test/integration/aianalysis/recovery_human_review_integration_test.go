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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// Recovery Human Review Integration Tests
//
// BR-HAPI-197: Human Review Required Flag for AI Reliability Issues
// BR-AI-082: Recovery Flow Support
//
// Testing Strategy (per TESTING_GUIDELINES.md):
// ✅ CORRECT PATTERN: Test AA business logic (controller reconciliation)
// ✅ Create AIAnalysis CRD → Wait for reconciliation → Verify status
// ✅ HAPI is side effect (AA controller calls HAPI, we verify AA behavior)
//
// ❌ WRONG PATTERN (AVOIDED): Direct HAPI client calls
// ❌ Don't test: hapiClient.InvestigateRecovery() (tests HAPI, not AA)
// ❌ Don't validate: HAPI response format (HAPI's responsibility)
//
// Reference: SignalProcessing, Gateway integration tests (correct patterns)
// See: TESTING_GUIDELINES.md lines 1689-1948 (Anti-Pattern: Direct Infrastructure Testing)
//
// Infrastructure Required:
// - Real HAPI service with MOCK_LLM_ENABLED=true
// - Started via: make test-integration-aianalysis
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation
// - Integration tests (>50%): Infrastructure interaction, microservices coordination
// - E2E tests (10-15%): Complete workflow validation

// SERIAL EXECUTION: AA integration suite runs serially for 100% reliability.
// See audit_flow_integration_test.go for detailed rationale.
var _ = Describe("BR-HAPI-197: Recovery Human Review Integration", Label("integration", "recovery", "human-review"), func() {
	// DD-TEST-002: testNamespace is set dynamically in suite_test.go BeforeEach
	// No need to set it here - each test gets a unique namespace automatically

	// ========================================
	// BR-HAPI-197: Recovery Human Review - No Matching Workflows
	// ========================================
	Context("Recovery human review when no workflows match", func() {
		It("should transition AIAnalysis to Failed with WorkflowResolutionFailed reason", func() {
			// Given: AIAnalysis with recovery attempt and special signal type
			// The REAL HAPI service (http://localhost:18120) has mock logic that responds
			// to MOCK_NO_WORKFLOW_FOUND with needs_human_review=true
			// (No need to configure mock - we're using the real service!)
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      helpers.UniqueTestName("recovery-hr-no-wf"),
					Namespace: testNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      helpers.UniqueTestName("rr-recovery-hr-no-wf"),
						Namespace: testNamespace,
					},
					RemediationID:         helpers.UniqueTestName("rem-recovery-hr-no-wf"),
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 1,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint: helpers.UniqueTestName("fp-recovery-hr-no-wf"),
							Severity:    "critical",
							// Special signal type triggers HAPI mock edge case:
							// - needs_human_review=true
							// - human_review_reason="no_matching_workflows"
							SignalName:       "MOCK_NO_WORKFLOW_FOUND",
							Environment:      "production",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "failing-pod",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"recovery-analysis", "workflow-selection"},
					},
					PreviousExecutions: []aianalysisv1alpha1.PreviousExecution{
						{
							WorkflowExecutionRef: "workflow-exec-001",
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

			// When: Create AIAnalysis (trigger AA controller reconciliation)
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			// Then: AA controller should reconcile and transition to Failed
			// (AA controller calls HAPI, receives needs_human_review=true,
			//  and sets status accordingly - this is AA business logic)
			By("Waiting for AIAnalysis to transition to Failed phase")
			Eventually(func() string {
				var updated aianalysisv1alpha1.AIAnalysis
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated); err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, "60s", "2s").Should(Equal(aianalysisv1alpha1.PhaseFailed),
				"AA controller should transition to Failed when HAPI returns needs_human_review=true")

			// Verify: AA controller set correct status fields (business outcome)
			var finalAnalysis aianalysisv1alpha1.AIAnalysis
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &finalAnalysis)).To(Succeed())

			By("Verifying Status.Reason is WorkflowResolutionFailed")
			Expect(finalAnalysis.Status.Reason).To(Equal("WorkflowResolutionFailed"),
				"AA controller should set Reason=WorkflowResolutionFailed for human review scenarios")

			By("Verifying Status.SubReason maps to NoMatchingWorkflows")
			Expect(string(finalAnalysis.Status.SubReason)).To(Equal("NoMatchingWorkflows"),
				"AA controller should map HAPI's human_review_reason enum to SubReason")

			By("Verifying Status.CompletedAt is set")
			Expect(finalAnalysis.Status.CompletedAt).ToNot(BeNil(),
				"AA controller should set CompletedAt timestamp when analysis fails")

			By("Verifying Status.Message explains human review requirement")
			Expect(finalAnalysis.Status.Message).To(ContainSubstring("could not provide reliable recovery workflow recommendation"),
				"AA controller should provide clear message for human review scenarios")
			Expect(finalAnalysis.Status.Message).To(ContainSubstring("no_matching_workflows"),
				"AA controller should include HAPI's reason in message")
		})
	})

	// ========================================
	// BR-HAPI-197: Recovery Human Review - Low Confidence
	// ========================================
	Context("Recovery human review when confidence is low", func() {
		It("should transition AIAnalysis to Failed with LowConfidence subreason", func() {
			// Given: AIAnalysis with recovery attempt and special signal type
			// that triggers HAPI mock edge case (needs_human_review=true, reason=low_confidence)
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      helpers.UniqueTestName("recovery-hr-low-conf"),
					Namespace: testNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      helpers.UniqueTestName("rr-recovery-hr-low-conf"),
						Namespace: testNamespace,
					},
					RemediationID:         helpers.UniqueTestName("rem-recovery-hr-low-conf"),
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 1,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint: helpers.UniqueTestName("fp-recovery-hr-low-conf"),
							// DD-HAPI-017: Signal context must match generic-restart-v1 OCI labels
							// for the security gate to pass and the low_confidence path to be exercised.
							// generic-restart-v1 labels: severity=[low,medium], component=pod,
							// environment=[production,staging,test], priority=[P1,P2,P3]
							Severity: "medium",
							// Special signal type triggers HAPI mock edge case:
							// - needs_human_review=true (from low confidence)
							// - human_review_reason="low_confidence"
							SignalName:       "MOCK_LOW_CONFIDENCE",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "unstable-pod",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"recovery-analysis"},
					},
					PreviousExecutions: []aianalysisv1alpha1.PreviousExecution{
						{
							WorkflowExecutionRef: "workflow-exec-002",
							OriginalRCA: aianalysisv1alpha1.OriginalRCA{
								Summary:    "Intermittent failure",
								SignalType: "ImagePullBackOff",
								Severity:   "critical",
							},
							SelectedWorkflow: aianalysisv1alpha1.SelectedWorkflowSummary{
								WorkflowID:     "recovery-workflow-v1",
								ExecutionBundle: "quay.io/kubernaut/recovery:v1.0.0",
							},
							Failure: aianalysisv1alpha1.ExecutionFailure{
								Reason:        "WorkflowFailed",
								Message:       "Recovery attempt failed",
								FailedAt:      metav1.Now(),
								ExecutionTime: "45s",
							},
						},
					},
				},
			}

			// When: Create AIAnalysis (trigger AA controller reconciliation)
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			// Then: AA controller should reconcile and transition to Failed
			By("Waiting for AIAnalysis to transition to Failed phase")
			Eventually(func() string {
				var updated aianalysisv1alpha1.AIAnalysis
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated); err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, "60s", "2s").Should(Equal(aianalysisv1alpha1.PhaseFailed))

			// Verify: AA controller set correct status fields for low confidence scenario
			var finalAnalysis aianalysisv1alpha1.AIAnalysis
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &finalAnalysis)).To(Succeed())

			By("Verifying Status.Reason is WorkflowResolutionFailed")
			Expect(finalAnalysis.Status.Reason).To(Equal("WorkflowResolutionFailed"))

			By("Verifying Status.SubReason maps to LowConfidence")
			Expect(string(finalAnalysis.Status.SubReason)).To(Equal("LowConfidence"),
				"AA controller should map low_confidence to LowConfidence SubReason")

			By("Verifying Status.CompletedAt is set")
			Expect(finalAnalysis.Status.CompletedAt).ToNot(BeNil())

			By("Verifying Status.Message includes low_confidence reason")
			Expect(finalAnalysis.Status.Message).To(ContainSubstring("low_confidence"),
				"AA controller should include specific reason in message")
		})
	})

	// ========================================
	// Baseline: Normal Recovery Flow (No Human Review)
	// ========================================
	Context("Normal recovery flow without human review", func() {
		It("should complete recovery successfully when HAPI provides workflow recommendation", func() {
			// Given: AIAnalysis with recovery attempt using normal signal type
			// (NOT an edge case, HAPI returns needs_human_review=false)
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      helpers.UniqueTestName("recovery-normal"),
					Namespace: testNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      helpers.UniqueTestName("rr-recovery-normal"),
						Namespace: testNamespace,
					},
					RemediationID:         helpers.UniqueTestName("rem-recovery-normal"),
					IsRecoveryAttempt:     true,
					RecoveryAttemptNumber: 1,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
						Fingerprint: helpers.UniqueTestName("fp-recovery-normal"),
						Severity:    "high",
						// Normal signal type (not an edge case)
						// HAPI returns needs_human_review=false and provides workflow
						// Severity=high matches crashloop-config-fix-v1 workflow catalog entry
						SignalName:       "CrashLoopBackOff",
						Environment:      "production",
						BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment", // Must match workflow component label "deployment"
								Name:      "app-pod",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"recovery-analysis"},
					},
					PreviousExecutions: []aianalysisv1alpha1.PreviousExecution{
						{
							WorkflowExecutionRef: "workflow-exec-003",
							OriginalRCA: aianalysisv1alpha1.OriginalRCA{
								Summary:    "Pod crashed",
								SignalType: "OOMKilled",
								Severity:   "critical",
							},
							SelectedWorkflow: aianalysisv1alpha1.SelectedWorkflowSummary{
								WorkflowID:     "memory-increase-v1",
								ExecutionBundle: "quay.io/kubernaut/memory-increase:v1.0.0",
							},
							Failure: aianalysisv1alpha1.ExecutionFailure{
								Reason:        "WorkflowFailed",
								Message:       "Memory increase failed",
								FailedAt:      metav1.Now(),
								ExecutionTime: "30s",
							},
						},
					},
				},
			}

			// When: Create AIAnalysis (trigger AA controller reconciliation)
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			// Then: AA controller should complete recovery successfully
			// (HAPI provides workflow recommendation, needs_human_review=false)
			By("Waiting for AIAnalysis to transition to Completed phase (or remain in Investigating)")
			Eventually(func() bool {
				var updated aianalysisv1alpha1.AIAnalysis
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated); err != nil {
					return false
				}
				// Normal recovery should complete successfully (Completed phase) - NOT Failed
				// Note: May stay in Investigating if workflow creation is pending
				phase := string(updated.Status.Phase)
				return phase == aianalysisv1alpha1.PhaseCompleted ||
					phase == aianalysisv1alpha1.PhaseInvestigating
			}, "60s", "2s").Should(BeTrue(),
				"Normal recovery should reach Completed or Investigating, not Failed")

			// Verify: AA controller did NOT set WorkflowResolutionFailed reason
			var finalAnalysis aianalysisv1alpha1.AIAnalysis
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &finalAnalysis)).To(Succeed())

			By("Verifying Status.Reason is NOT WorkflowResolutionFailed")
			Expect(finalAnalysis.Status.Reason).ToNot(Equal("WorkflowResolutionFailed"),
				"Normal recovery should NOT have WorkflowResolutionFailed reason")

			By("Verifying Status.Phase is NOT Failed")
			Expect(string(finalAnalysis.Status.Phase)).ToNot(Equal(aianalysisv1alpha1.PhaseFailed),
				"Normal recovery should NOT transition to Failed phase")
		})
	})
})
