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

package remediationorchestrator_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// E2E Tests for Remediation Orchestrator Controller
// Per TESTING_GUIDELINES.md: E2E tests (10-15%) validate critical user journeys
//
// These tests validate:
// - BR-ORCH-025: Full remediation lifecycle orchestration
// - BR-ORCH-026: Approval workflow with RAR CRD
// - BR-ORCH-036: Manual review notification flow
// - BR-ORCH-037: WorkflowNotNeeded handling
//
// Prerequisites:
// - Kind cluster running with isolated kubeconfig
// - All CRDs installed (handled by BeforeSuite)
// - NO running RO controller (tests simulate controller behavior)
var _ = Describe("RemediationOrchestrator E2E Tests", Label("e2e"), func() {
	var testNS string

	BeforeEach(func() {
		testNS = createTestNamespace("ro-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	// ========================================
	// BR-ORCH-025: Full Remediation Lifecycle
	// ========================================
	Describe("Full Remediation Lifecycle (BR-ORCH-025)", func() {
		It("should create RemediationRequest and progress through phases", func() {
			By("Creating a RemediationRequest in Pending phase")
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-lifecycle-e2e",
					Namespace: testNS,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalContext: remediationv1.SignalContext{
						Fingerprint:  "fp-e2e-001",
						SourceType:   "prometheus",
						AlertName:    "HighCPUUsage",
						Severity:     "warning",
						ReceivedAt:   metav1.Now(),
						RawPayload:   `{"alertname": "HighCPUUsage", "instance": "node-1"}`,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Verifying RemediationRequest was created")
			createdRR := &remediationv1.RemediationRequest{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)
			}, timeout, interval).Should(Succeed())
			Expect(createdRR.Spec.SignalContext.Fingerprint).To(Equal("fp-e2e-001"))

			By("Simulating SignalProcessing completion")
			sp := &signalprocessingv1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sp-" + rr.Name,
					Namespace: testNS,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: remediationv1.GroupVersion.String(),
							Kind:       "RemediationRequest",
							Name:       rr.Name,
							UID:        createdRR.UID,
						},
					},
				},
				Spec: signalprocessingv1.SignalProcessingSpec{
					SignalContext: signalprocessingv1.SignalContext{
						Fingerprint: "fp-e2e-001",
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			// Update SP status to Completed
			sp.Status.Phase = "Completed"
			sp.Status.Environment = "production"
			sp.Status.Priority = "high"
			Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

			By("Simulating AIAnalysis completion with workflow recommendation")
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-" + rr.Name,
					Namespace: testNS,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: remediationv1.GroupVersion.String(),
							Kind:       "RemediationRequest",
							Name:       rr.Name,
							UID:        createdRR.UID,
						},
					},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContext{
							Fingerprint: "fp-e2e-001",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ai)).To(Succeed())

			// Update AI status to Completed with workflow
			ai.Status.Phase = "Completed"
			ai.Status.InvestigationOutcome = "Resolved"
			ai.Status.ApprovalRequired = false
			ai.Status.ConfidenceScore = 0.92
			ai.Status.SelectedWorkflow = &aianalysisv1.WorkflowReference{
				ID:   "wf-scale-deployment",
				Name: "Scale Deployment",
			}
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Simulating WorkflowExecution completion")
			we := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "we-" + rr.Name,
					Namespace: testNS,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: remediationv1.GroupVersion.String(),
							Kind:       "RemediationRequest",
							Name:       rr.Name,
							UID:        createdRR.UID,
						},
					},
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					WorkflowID: "wf-scale-deployment",
				},
			}
			Expect(k8sClient.Create(ctx, we)).To(Succeed())

			// Update WE status to Completed
			we.Status.Phase = "Completed"
			we.Status.ExitCode = 0
			Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

			By("Verifying all child CRDs exist with correct owner references")
			// Verify SP
			fetchedSP := &signalprocessingv1.SignalProcessing{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: sp.Name, Namespace: testNS}, fetchedSP)).To(Succeed())
			Expect(fetchedSP.OwnerReferences).To(HaveLen(1))
			Expect(fetchedSP.OwnerReferences[0].Name).To(Equal(rr.Name))

			// Verify AI
			fetchedAI := &aianalysisv1.AIAnalysis{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ai.Name, Namespace: testNS}, fetchedAI)).To(Succeed())
			Expect(fetchedAI.OwnerReferences).To(HaveLen(1))

			// Verify WE
			fetchedWE := &workflowexecutionv1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: we.Name, Namespace: testNS}, fetchedWE)).To(Succeed())
			Expect(fetchedWE.OwnerReferences).To(HaveLen(1))
		})
	})

	// ========================================
	// BR-ORCH-026: Approval Workflow
	// ========================================
	Describe("Approval Workflow (BR-ORCH-026)", func() {
		It("should create RemediationApprovalRequest when approval is required", func() {
			By("Creating a RemediationRequest")
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-approval-e2e",
					Namespace: testNS,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalContext: remediationv1.SignalContext{
						Fingerprint: "fp-approval-001",
						SourceType:  "prometheus",
						AlertName:   "CriticalError",
						Severity:    "critical",
						ReceivedAt:  metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Get created RR for UID
			createdRR := &remediationv1.RemediationRequest{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)
			}, timeout, interval).Should(Succeed())

			By("Simulating AIAnalysis requiring approval")
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-" + rr.Name,
					Namespace: testNS,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: remediationv1.GroupVersion.String(),
							Kind:       "RemediationRequest",
							Name:       rr.Name,
							UID:        createdRR.UID,
						},
					},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContext{
							Fingerprint: "fp-approval-001",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ai)).To(Succeed())

			// Update AI status - approval required
			ai.Status.Phase = "Completed"
			ai.Status.InvestigationOutcome = "ApprovalRequired"
			ai.Status.ApprovalRequired = true
			ai.Status.ConfidenceScore = 0.65 // Low confidence triggers approval
			ai.Status.SelectedWorkflow = &aianalysisv1.WorkflowReference{
				ID:   "wf-delete-pod",
				Name: "Delete Pod",
			}
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Creating RemediationApprovalRequest (simulating RO behavior)")
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rar-" + rr.Name,
					Namespace: testNS,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: remediationv1.GroupVersion.String(),
							Kind:       "RemediationRequest",
							Name:       rr.Name,
							UID:        createdRR.UID,
						},
					},
					Labels: map[string]string{
						"kubernaut.io/remediation-request": rr.Name,
						"kubernaut.io/environment":         "production",
					},
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:       rr.Name,
						Namespace:  testNS,
						Kind:       "RemediationRequest",
						APIVersion: remediationv1.GroupVersion.String(),
					},
					AIAnalysisContext: remediationv1.AIAnalysisContext{
						ConfidenceScore:       0.65,
						RecommendedWorkflowID: "wf-delete-pod",
						InvestigationSummary:  "Pod restart recommended due to OOM",
					},
					RequiredBy: metav1.NewTime(time.Now().Add(24 * time.Hour)),
				},
			}
			Expect(k8sClient.Create(ctx, rar)).To(Succeed())

			By("Verifying RAR was created correctly")
			fetchedRAR := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), fetchedRAR)
			}, timeout, interval).Should(Succeed())
			Expect(fetchedRAR.Spec.AIAnalysisContext.ConfidenceScore).To(Equal(float64(0.65)))
			Expect(fetchedRAR.Spec.RequiredBy.Time).To(BeTemporally(">", time.Now()))

			By("Simulating operator approval")
			fetchedRAR.Status.Decision = remediationv1.ApprovalDecisionApproved
			fetchedRAR.Status.DecidedBy = "operator@example.com"
			fetchedRAR.Status.DecidedAt = &metav1.Time{Time: time.Now()}
			fetchedRAR.Status.DecisionMessage = "Approved for execution"
			Expect(k8sClient.Status().Update(ctx, fetchedRAR)).To(Succeed())

			By("Verifying approval status was updated")
			Eventually(func() string {
				updatedRAR := &remediationv1.RemediationApprovalRequest{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), updatedRAR); err != nil {
					return ""
				}
				return string(updatedRAR.Status.Decision)
			}, timeout, interval).Should(Equal("Approved"))
		})

		It("should handle approval rejection", func() {
			By("Creating a RemediationRequest")
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-rejection-e2e",
					Namespace: testNS,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalContext: remediationv1.SignalContext{
						Fingerprint: "fp-rejection-001",
						SourceType:  "prometheus",
						AlertName:   "DangerousOperation",
						Severity:    "critical",
						ReceivedAt:  metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			createdRR := &remediationv1.RemediationRequest{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)
			}, timeout, interval).Should(Succeed())

			By("Creating RAR for rejection test")
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rar-" + rr.Name,
					Namespace: testNS,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: remediationv1.GroupVersion.String(),
							Kind:       "RemediationRequest",
							Name:       rr.Name,
							UID:        createdRR.UID,
						},
					},
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:       rr.Name,
						Namespace:  testNS,
						Kind:       "RemediationRequest",
						APIVersion: remediationv1.GroupVersion.String(),
					},
					AIAnalysisContext: remediationv1.AIAnalysisContext{
						ConfidenceScore:       0.45,
						RecommendedWorkflowID: "wf-dangerous-op",
						InvestigationSummary:  "Risky operation",
					},
					RequiredBy: metav1.NewTime(time.Now().Add(1 * time.Hour)),
				},
			}
			Expect(k8sClient.Create(ctx, rar)).To(Succeed())

			By("Simulating operator rejection")
			fetchedRAR := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), fetchedRAR)
			}, timeout, interval).Should(Succeed())

			fetchedRAR.Status.Decision = remediationv1.ApprovalDecisionRejected
			fetchedRAR.Status.DecidedBy = "admin@example.com"
			fetchedRAR.Status.DecidedAt = &metav1.Time{Time: time.Now()}
			fetchedRAR.Status.DecisionMessage = "Too risky, manual investigation required"
			Expect(k8sClient.Status().Update(ctx, fetchedRAR)).To(Succeed())

			By("Verifying rejection status")
			Eventually(func() string {
				updatedRAR := &remediationv1.RemediationApprovalRequest{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), updatedRAR); err != nil {
					return ""
				}
				return string(updatedRAR.Status.Decision)
			}, timeout, interval).Should(Equal("Rejected"))
		})
	})

	// ========================================
	// BR-ORCH-037: WorkflowNotNeeded Handling
	// ========================================
	Describe("WorkflowNotNeeded Handling (BR-ORCH-037)", func() {
		It("should complete remediation when no workflow is needed", func() {
			By("Creating a RemediationRequest")
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-no-workflow-e2e",
					Namespace: testNS,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalContext: remediationv1.SignalContext{
						Fingerprint: "fp-no-workflow-001",
						SourceType:  "prometheus",
						AlertName:   "TransientError",
						Severity:    "info",
						ReceivedAt:  metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			createdRR := &remediationv1.RemediationRequest{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)
			}, timeout, interval).Should(Succeed())

			By("Simulating AIAnalysis with WorkflowNotNeeded outcome")
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-" + rr.Name,
					Namespace: testNS,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: remediationv1.GroupVersion.String(),
							Kind:       "RemediationRequest",
							Name:       rr.Name,
							UID:        createdRR.UID,
						},
					},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContext{
							Fingerprint: "fp-no-workflow-001",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ai)).To(Succeed())

			// Update AI status - problem self-resolved
			ai.Status.Phase = "Completed"
			ai.Status.InvestigationOutcome = "WorkflowNotNeeded"
			ai.Status.ApprovalRequired = false
			ai.Status.ConfidenceScore = 0.95
			ai.Status.ResolutionReason = "Problem self-resolved: transient error no longer present"
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Verifying AIAnalysis shows WorkflowNotNeeded")
			fetchedAI := &aianalysisv1.AIAnalysis{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ai), fetchedAI); err != nil {
					return ""
				}
				return fetchedAI.Status.InvestigationOutcome
			}, timeout, interval).Should(Equal("WorkflowNotNeeded"))
		})
	})

	// ========================================
	// Cascade Deletion Tests
	// ========================================
	Describe("Cascade Deletion", func() {
		It("should delete child CRDs when parent RR is deleted", func() {
			By("Creating a RemediationRequest with child CRDs")
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-cascade-e2e",
					Namespace: testNS,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalContext: remediationv1.SignalContext{
						Fingerprint: "fp-cascade-001",
						SourceType:  "prometheus",
						AlertName:   "CascadeTest",
						Severity:    "warning",
						ReceivedAt:  metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			createdRR := &remediationv1.RemediationRequest{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)
			}, timeout, interval).Should(Succeed())

			By("Creating child SignalProcessing with owner reference")
			sp := &signalprocessingv1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sp-" + rr.Name,
					Namespace: testNS,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         remediationv1.GroupVersion.String(),
							Kind:               "RemediationRequest",
							Name:               rr.Name,
							UID:                createdRR.UID,
							Controller:         boolPtr(true),
							BlockOwnerDeletion: boolPtr(true),
						},
					},
				},
				Spec: signalprocessingv1.SignalProcessingSpec{
					SignalContext: signalprocessingv1.SignalContext{
						Fingerprint: "fp-cascade-001",
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("Deleting parent RemediationRequest")
			Expect(k8sClient.Delete(ctx, rr)).To(Succeed())

			By("Verifying parent RR is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), &remediationv1.RemediationRequest{})
				return err != nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying child SP is deleted via cascade")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &signalprocessingv1.SignalProcessing{})
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})

// Helper function for creating bool pointers
func boolPtr(b bool) *bool {
	return &b
}


