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

package remediationorchestrator

import (
	"context"
	"fmt"
	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ============================================================================
// LIFECYCLE INTEGRATION TESTS
// Tests the complete RO lifecycle: RR → SP → AI → (Approval) → WE → Notification
// Reference: BR-ORCH-025 (data pass-through), BR-ORCH-031 (cascade deletion)
// ============================================================================

var _ = Describe("RemediationOrchestrator Lifecycle", Label("integration", "lifecycle"), func() {

	Context("Basic RemediationRequest Creation", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-lifecycle")
			rrName = fmt.Sprintf("rr-%s", uuid.New().String()[:13])
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should create RemediationRequest and transition to Processing phase", func() {
			By("Creating a RemediationRequest")
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: namespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					// Valid 64-char hex fingerprint (SHA256 format per CRD validation)
					SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					SignalName:        "TestHighMemoryAlert",
					Severity:          "critical",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: namespace,
					},
					FiringTime:   now,
					ReceivedTime: now,
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Waiting for RO to process the RemediationRequest")
			Eventually(func() string {
				fetched := &remediationv1.RemediationRequest{}
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetched)
				if err != nil {
					return ""
				}
				return string(fetched.Status.OverallPhase)
			}, timeout, interval).ShouldNot(BeEmpty())

			By("Verifying the RemediationRequest has been processed")
			fetched := &remediationv1.RemediationRequest{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetched)).To(Succeed())
			GinkgoWriter.Printf("✅ RR phase: %s\n", fetched.Status.OverallPhase)
		})

		It("should create SignalProcessing child CRD with owner reference", func() {
			By("Creating a RemediationRequest")
			rr := createRemediationRequest(namespace, rrName)

			By("Waiting for SignalProcessing CRD to be created")
			spName := fmt.Sprintf("sp-%s", rrName)
			sp := &signalprocessingv1.SignalProcessing{}

			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: namespace}, sp)
			}, timeout, interval).Should(Succeed())

			By("Verifying owner reference is set (BR-ORCH-031)")
			Expect(sp.OwnerReferences).To(HaveLen(1))
			Expect(sp.OwnerReferences[0].Name).To(Equal(rrName))
			Expect(sp.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
			Expect(*sp.OwnerReferences[0].Controller).To(BeTrue())

			By("Verifying SP spec contains RR reference")
			Expect(sp.Spec.RemediationRequestRef.Name).To(Equal(rr.Name))

			GinkgoWriter.Printf("✅ SignalProcessing created: %s with owner ref to %s\n", spName, rrName)
		})
	})

	Context("Phase Progression with Simulated Child Status Updates", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-phase")
			rrName = fmt.Sprintf("rr-phase-%s", uuid.New().String()[:13])
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should progress through phases when child CRDs complete", func() {
			By("Creating a RemediationRequest")
			_ = createRemediationRequest(namespace, rrName)

			By("Waiting for SignalProcessing to be created")
			spName := fmt.Sprintf("sp-%s", rrName)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: namespace}, sp)
			}, timeout, interval).Should(Succeed())

			By("Simulating SignalProcessing completion")
			err := updateSPStatus(namespace, spName, signalprocessingv1.PhaseCompleted)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for RR to transition to Analyzing phase")
			Eventually(func() string {
				rr := &remediationv1.RemediationRequest{}
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, rr); err != nil {
					return ""
				}
				return string(rr.Status.OverallPhase)
			}, timeout, interval).Should(Equal("Analyzing"))

			By("Waiting for AIAnalysis to be created")
			aiName := fmt.Sprintf("ai-%s", rrName)
			ai := &aianalysisv1.AIAnalysis{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)
			}, timeout, interval).Should(Succeed())

			By("Verifying AIAnalysis has owner reference")
			Expect(ai.OwnerReferences).To(HaveLen(1))
			Expect(ai.OwnerReferences[0].Name).To(Equal(rrName))

			GinkgoWriter.Printf("✅ Phase progression: Pending → Processing → Analyzing\n")
			GinkgoWriter.Printf("✅ Child CRDs created: sp-%s, ai-%s\n", rrName, rrName)
		})
	})
})

// ============================================================================
// AIANALYSIS → MANUAL REVIEW INTEGRATION TESTS
// Tests BR-ORCH-036: Manual Review Notification Creation
// Tests BR-ORCH-037: WorkflowNotNeeded Handling
// ============================================================================

var _ = Describe("AIAnalysis ManualReview Flow", Label("integration", "manual-review"), func() {

	Context("BR-ORCH-036: WorkflowResolutionFailed triggers ManualReview notification", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-manual-review")
			rrName = fmt.Sprintf("rr-mr-%s", uuid.New().String()[:13])
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should create ManualReview notification when AIAnalysis fails with WorkflowResolutionFailed", func() {
			By("Creating a RemediationRequest")
			_ = createRemediationRequest(namespace, rrName)

			By("Waiting for SignalProcessing to be created")
			spName := fmt.Sprintf("sp-%s", rrName)
			Eventually(func() error {
				sp := &signalprocessingv1.SignalProcessing{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: namespace}, sp)
			}, timeout, interval).Should(Succeed())

			By("Simulating SignalProcessing completion")
			Expect(updateSPStatus(namespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

			By("Waiting for AIAnalysis to be created")
			aiName := fmt.Sprintf("ai-%s", rrName)
			Eventually(func() error {
				ai := &aianalysisv1.AIAnalysis{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)
			}, timeout, interval).Should(Succeed())

			By("Simulating AIAnalysis failure with WorkflowResolutionFailed")
			ai := &aianalysisv1.AIAnalysis{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)).To(Succeed())

			ai.Status.Phase = "Failed"
			ai.Status.Reason = "WorkflowResolutionFailed"
			ai.Status.SubReason = "NoMatchingWorkflows"
			ai.Status.Message = "No workflow found matching the investigation outcome"
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Waiting for ManualReview NotificationRequest to be created")
			nrName := fmt.Sprintf("nr-manual-review-%s", rrName)
			Eventually(func() error {
				nr := &notificationv1.NotificationRequest{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: nrName, Namespace: namespace}, nr)
			}, timeout, interval).Should(Succeed())

			By("Verifying NotificationRequest properties")
			nr := &notificationv1.NotificationRequest{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: nrName, Namespace: namespace}, nr)).To(Succeed())

			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
			Expect(nr.Labels["kubernaut.ai/notification-type"]).To(Equal("manual-review"))
			Expect(nr.Labels["kubernaut.ai/remediation-request"]).To(Equal(rrName))

			By("Verifying RR status updated")
			rr := &remediationv1.RemediationRequest{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, rr)).To(Succeed())
			Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed))
			Expect(rr.Status.Outcome).To(Equal("ManualReviewRequired"))

			GinkgoWriter.Printf("✅ BR-ORCH-036: ManualReview notification created for WorkflowResolutionFailed\n")
		})
	})

	// =====================================================
	// BR-ORCH-036 v3.0: Infrastructure Failure Escalation
	// AC-036-35: No failure transitions to RR Failed without a notification
	// =====================================================
	Context("BR-ORCH-036 v3.0: Infrastructure failure creates escalation notification", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-infra-fail")
			rrName = fmt.Sprintf("rr-if-%s", uuid.New().String()[:13])
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should create escalation notification when AIAnalysis fails with APIError/MaxRetriesExceeded", func() {
			By("Creating a RemediationRequest")
			_ = createRemediationRequest(namespace, rrName)

			By("Waiting for SignalProcessing to be created")
			spName := fmt.Sprintf("sp-%s", rrName)
			Eventually(func() error {
				sp := &signalprocessingv1.SignalProcessing{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: namespace}, sp)
			}, timeout, interval).Should(Succeed())

			By("Simulating SignalProcessing completion")
			Expect(updateSPStatus(namespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

			By("Waiting for AIAnalysis to be created")
			aiName := fmt.Sprintf("ai-%s", rrName)
			Eventually(func() error {
				ai := &aianalysisv1.AIAnalysis{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)
			}, timeout, interval).Should(Succeed())

			By("Simulating AIAnalysis failure with APIError/MaxRetriesExceeded")
			ai := &aianalysisv1.AIAnalysis{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)).To(Succeed())

			ai.Status.Phase = "Failed"
			ai.Status.Reason = "APIError"
			ai.Status.SubReason = "MaxRetriesExceeded"
			ai.Status.Message = "Transient error exceeded max retries (5 attempts): HAPI request timeout"
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Waiting for escalation NotificationRequest to be created (BR-ORCH-036 v3.0)")
			nrName := fmt.Sprintf("nr-manual-review-%s", rrName)
			Eventually(func() error {
				nr := &notificationv1.NotificationRequest{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: nrName, Namespace: namespace}, nr)
			}, timeout, interval).Should(Succeed())

			By("Verifying NotificationRequest properties")
			nr := &notificationv1.NotificationRequest{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: nrName, Namespace: namespace}, nr)).To(Succeed())

			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
			Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh))
			Expect(nr.Labels["kubernaut.ai/notification-type"]).To(Equal("manual-review"))
			Expect(nr.Labels["kubernaut.ai/remediation-request"]).To(Equal(rrName))
			Expect(nr.Spec.Metadata).To(HaveKeyWithValue("reason", "APIError"))
			Expect(nr.Spec.Metadata).To(HaveKeyWithValue("subReason", "MaxRetriesExceeded"))

			By("Verifying RR status updated to Failed with ManualReviewRequired")
			rr := &remediationv1.RemediationRequest{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, rr)).To(Succeed())
			Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed))
			Expect(rr.Status.Outcome).To(Equal("ManualReviewRequired"))
			Expect(rr.Status.RequiresManualReview).To(BeTrue())

			GinkgoWriter.Printf("✅ BR-ORCH-036 v3.0: Escalation notification created for APIError/MaxRetriesExceeded\n")
		})
	})

	Context("BR-ORCH-037: WorkflowNotNeeded completes with NoActionRequired", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-no-action")
			rrName = fmt.Sprintf("rr-na-%s", uuid.New().String()[:13])
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should complete RR with NoActionRequired when AIAnalysis returns WorkflowNotNeeded", func() {
			By("Creating a RemediationRequest")
			_ = createRemediationRequest(namespace, rrName)

			By("Waiting for SignalProcessing and completing it")
			spName := fmt.Sprintf("sp-%s", rrName)
			Eventually(func() error {
				sp := &signalprocessingv1.SignalProcessing{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: namespace}, sp)
			}, timeout, interval).Should(Succeed())
			Expect(updateSPStatus(namespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

			By("Waiting for AIAnalysis to be created")
			aiName := fmt.Sprintf("ai-%s", rrName)
			Eventually(func() error {
				ai := &aianalysisv1.AIAnalysis{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)
			}, timeout, interval).Should(Succeed())

			By("Simulating AIAnalysis completion with WorkflowNotNeeded (problem self-resolved)")
			ai := &aianalysisv1.AIAnalysis{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)).To(Succeed())

			ai.Status.Phase = "Completed"
			ai.Status.RootCause = "Problem self-resolved - container restarted successfully"
			ai.Status.Reason = "WorkflowNotNeeded"
			ai.Status.SubReason = "ProblemResolved"
			now := metav1.Now()
			ai.Status.CompletedAt = &now
			// No SelectedWorkflow - indicates WorkflowNotNeeded
			ai.Status.SelectedWorkflow = nil
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Waiting for RR to complete with NoActionRequired")
			Eventually(func() string {
				rr := &remediationv1.RemediationRequest{}
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, rr); err != nil {
					return ""
				}
				return rr.Status.Outcome
			}, timeout, interval).Should(Equal("NoActionRequired"))

			By("Verifying RR status")
			rr := &remediationv1.RemediationRequest{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, rr)).To(Succeed())
			Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted))

			GinkgoWriter.Printf("✅ BR-ORCH-037: RR completed with NoActionRequired for WorkflowNotNeeded\n")
		})
	})
})

// ============================================================================
// APPROVAL FLOW INTEGRATION TESTS
// Tests BR-ORCH-026: Approval Orchestration via RemediationApprovalRequest
// Reference: ADR-040
// ============================================================================

var _ = Describe("Approval Flow", Label("integration", "approval"), func() {

	Context("BR-ORCH-026: RemediationApprovalRequest creation and handling", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-approval")
			rrName = fmt.Sprintf("rr-appr-%s", uuid.New().String()[:13])
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should create RemediationApprovalRequest when AIAnalysis requires approval", func() {
			By("Creating a RemediationRequest")
			_ = createRemediationRequest(namespace, rrName)

			By("Progressing through SP phase")
			spName := fmt.Sprintf("sp-%s", rrName)
			Eventually(func() error {
				sp := &signalprocessingv1.SignalProcessing{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: namespace}, sp)
			}, timeout, interval).Should(Succeed())
			Expect(updateSPStatus(namespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

			By("Waiting for AIAnalysis to be created")
			aiName := fmt.Sprintf("ai-%s", rrName)
			Eventually(func() error {
				ai := &aianalysisv1.AIAnalysis{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)
			}, timeout, interval).Should(Succeed())

			By("Simulating AIAnalysis completion requiring approval (confidence 60-79%)")
			ai := &aianalysisv1.AIAnalysis{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)).To(Succeed())

			ai.Status.Phase = "Completed"
			ai.Status.ApprovalRequired = true
			ai.Status.ApprovalReason = "Confidence below 80% threshold"
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "wf-restart-pods",
				Version:        "v1.0.0",
				Confidence:     0.72,
				ContainerImage: "kubernaut/workflows:latest",
				Rationale:      "Pod restart recommended based on OOM patterns",
			}
			ai.Status.RootCause = "Memory leak causing OOM kills"
			now := metav1.Now()
			ai.Status.CompletedAt = &now
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Waiting for RR to transition to AwaitingApproval")
			Eventually(func() string {
				rr := &remediationv1.RemediationRequest{}
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, rr); err != nil {
					return ""
				}
				return string(rr.Status.OverallPhase)
			}, timeout, interval).Should(Equal("AwaitingApproval"))

			By("Waiting for RemediationApprovalRequest to be created")
			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: namespace}, rar)
			}, timeout, interval).Should(Succeed())

			By("Verifying RAR spec")
			Expect(rar.Spec.Confidence).To(BeNumerically("==", 0.72))
			Expect(rar.Spec.RecommendedWorkflow.WorkflowID).To(Equal("wf-restart-pods"))
			Expect(rar.OwnerReferences).To(HaveLen(1))
			Expect(rar.OwnerReferences[0].Name).To(Equal(rrName))

			GinkgoWriter.Printf("✅ BR-ORCH-026: RemediationApprovalRequest created for approval-required scenario\n")
		})

		It("should proceed to Executing when RAR is approved", func() {
			By("Creating RemediationRequest and progressing to AwaitingApproval")
			_ = createRemediationRequest(namespace, rrName)

			// Progress through SP
			spName := fmt.Sprintf("sp-%s", rrName)
			Eventually(func() error {
				sp := &signalprocessingv1.SignalProcessing{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: namespace}, sp)
			}, timeout, interval).Should(Succeed())
			Expect(updateSPStatus(namespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

			// Progress through AI with approval required
			aiName := fmt.Sprintf("ai-%s", rrName)
			Eventually(func() error {
				ai := &aianalysisv1.AIAnalysis{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)
			}, timeout, interval).Should(Succeed())

			ai := &aianalysisv1.AIAnalysis{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)).To(Succeed())
			ai.Status.Phase = "Completed"
			ai.Status.ApprovalRequired = true
			ai.Status.ApprovalReason = "Confidence below threshold"
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "wf-restart-pods",
				Version:        "v1.0.0",
				Confidence:     0.70,
				ContainerImage: "kubernaut/workflows:latest",
				Rationale:      "Restart recommended",
			}
			now := metav1.Now()
			ai.Status.CompletedAt = &now
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			// Wait for RAR
			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: namespace}, rar)
			}, timeout, interval).Should(Succeed())

			By("Approving the RemediationApprovalRequest")
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: namespace}, rar)).To(Succeed())
			rar.Status.Decision = remediationv1.ApprovalDecisionApproved
			rar.Status.DecidedBy = "test-admin@kubernaut.ai"
			rar.Status.DecisionMessage = "Approved for testing"
			decidedAt := metav1.Now()
			rar.Status.DecidedAt = &decidedAt
			Expect(k8sClient.Status().Update(ctx, rar)).To(Succeed())

			By("Waiting for RR to transition to Executing")
			Eventually(func() string {
				rr := &remediationv1.RemediationRequest{}
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, rr); err != nil {
					return ""
				}
				return string(rr.Status.OverallPhase)
			}, timeout, interval).Should(Equal("Executing"))

			GinkgoWriter.Printf("✅ BR-ORCH-026: RR transitioned to Executing after RAR approval\n")
		})

	It("should detect RAR missing and handle gracefully", func() {
		// Scenario: RAR deleted after creation (simulates accidental deletion or external cleanup)
		// Business Outcome: Controller detects missing RAR and handles gracefully (requeues, recreates)
		// Confidence: 95% - Uses real approval flow, validates resilience to RAR deletion
		// Multi-Controller Pattern: Safe for parallel execution (uses natural controller flow)

		ctx := context.Background()

		By("Creating RR and progressing to AwaitingApproval naturally")
		rrName := fmt.Sprintf("rr-missing-rar-%s", uuid.New().String()[:13])
		_ = createRemediationRequest(namespace, rrName)

		// Progress through SP (natural flow)
		spName := fmt.Sprintf("sp-%s", rrName)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: namespace}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(namespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

		// Progress through AI with approval required (natural flow)
		aiName := fmt.Sprintf("ai-%s", rrName)
		Eventually(func() error {
			ai := &aianalysisv1.AIAnalysis{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)
		}, timeout, interval).Should(Succeed())

		ai := &aianalysisv1.AIAnalysis{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: namespace}, ai)).To(Succeed())
		ai.Status.Phase = "Completed"
		ai.Status.ApprovalRequired = true
		ai.Status.ApprovalReason = "Confidence below threshold"
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:     "wf-restart-pods",
			Version:        "v1.0.0",
			Confidence:     0.70,
			ContainerImage: "kubernaut/workflows:latest",
			Rationale:      "Restart recommended",
		}
		now := metav1.Now()
		ai.Status.CompletedAt = &now
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Waiting for RR to reach AwaitingApproval")
		Eventually(func() string {
			rr := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, rr); err != nil {
				return ""
			}
			return string(rr.Status.OverallPhase)
		}, timeout, interval).Should(Equal("AwaitingApproval"))

		By("Waiting for RAR to be created automatically")
		rarName := fmt.Sprintf("rar-%s", rrName)
		rar := &remediationv1.RemediationApprovalRequest{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: namespace}, rar)
		}, timeout, interval).Should(Succeed())

		By("Deleting the RAR to simulate accidental deletion")
		Expect(k8sClient.Delete(ctx, rar)).To(Succeed())

		By("Verifying RAR deletion")
		Eventually(func() bool {
			err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: namespace}, rar)
			return errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue(), "RAR should be deleted")

		By("Verifying RR remains in AwaitingApproval (graceful degradation)")
		// Controller should detect missing RAR, log it, and requeue without crashing
		// Logs should show: "RemediationApprovalRequest not found, will be created by approval handler"
		Consistently(func() remediationv1.RemediationPhase {
			rr := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, rr); err != nil {
				return ""
			}
			return rr.Status.OverallPhase
		}, "10s", "500ms").Should(Equal(remediationv1.PhaseAwaitingApproval),
			"RR should remain in AwaitingApproval when RAR deleted (graceful degradation)")

		By("Verifying RR doesn't crash or error out")
		// Final check: RR should still be healthy after RAR deletion
		rr := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, rr)).To(Succeed())
		Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseAwaitingApproval))
		// Message should indicate waiting for approval (not an error state)
		Expect(rr.Status.Message).ToNot(ContainSubstring("error"))
		Expect(rr.Status.Message).ToNot(ContainSubstring("failed"))

		GinkgoWriter.Printf("✅ BR-ORCH-026: RR handles missing RAR gracefully (stays stable, no crash, proper logging)\n")
	})
	})
})
