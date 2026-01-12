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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	rarconditions "github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
)

// ========================================
// Phase 1 Integration Tests - Approval Conditions
//
// PHASE 1 PATTERN: RO Controller Only
// - RO controller manages RAR lifecycle and Kubernetes Conditions
// - Tests manually simulate human decisions (approve/reject/expire)
// - NO child controllers running (SP, AI, WE)
//
// This isolates RO's core RAR logic:
// - Condition management (ApprovalPending, ApprovalDecided, ApprovalExpired)
// - Decision processing (approved, rejected, expired)
// - RR phase transitions based on RAR outcomes
// ========================================

// ============================================================================
// REMEDIATION APPROVAL REQUEST CONDITIONS INTEGRATION TESTS
// Tests DD-CRD-002-RAR condition integration for RemediationApprovalRequest
// Reference: DD-CRD-002-RemediationApprovalRequest, DD-CRD-002 v1.2
// ============================================================================

// Helper: Update SignalProcessing to Completed phase
func updateSPStatusToCompleted(namespace, name string) { //nolint:unused
	EventuallyWithOffset(1, func() error {
		return updateSPStatus(namespace, name, signalprocessingv1.PhaseCompleted)
	}, timeout, interval).Should(Succeed(), "Failed to update SignalProcessing status to Completed")
}

// Helper: Simulate AIAnalysis completion with low confidence (triggers approval workflow)
func simulateAICompletionLowConfidence(namespace, name string) { //nolint:unused
	lowConfidenceWorkflow := &aianalysisv1.SelectedWorkflow{
		WorkflowID:     "test-workflow-1",
		Version:        "v1.0.0",
		ContainerImage: "kubernaut/workflows:test",
		Confidence:     0.4, // Below 0.7 threshold - requires approval
		Rationale:      "Test workflow for pod restart",
	}
	EventuallyWithOffset(1, func() error {
		return updateAIAnalysisStatus(namespace, name, "Completed", lowConfidenceWorkflow)
	}, timeout, interval).Should(Succeed(), "Failed to update AIAnalysis status to Completed")
}

// Helper: Simulate human approval decision
func approveRemediationApprovalRequest(namespace, name, approver string) { //nolint:unused
	EventuallyWithOffset(1, func() error {
		rar := &remediationv1.RemediationApprovalRequest{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, rar); err != nil {
			return err
		}

		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.DecidedBy = approver
		rar.Status.DecisionMessage = "Approved for testing"
		now := metav1.Now()
		rar.Status.DecidedAt = &now

		return k8sClient.Status().Update(ctx, rar)
	}, timeout, interval).Should(Succeed(), "Failed to approve RemediationApprovalRequest")
}

// Helper: Simulate human rejection decision
func rejectRemediationApprovalRequest(namespace, name, approver, reason string) { //nolint:unused
	EventuallyWithOffset(1, func() error {
		rar := &remediationv1.RemediationApprovalRequest{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, rar); err != nil {
			return err
		}

		rar.Status.Decision = remediationv1.ApprovalDecisionRejected
		rar.Status.DecidedBy = approver
		rar.Status.DecisionMessage = reason
		now := metav1.Now()
		rar.Status.DecidedAt = &now

		return k8sClient.Status().Update(ctx, rar)
	}, timeout, interval).Should(Succeed(), "Failed to reject RemediationApprovalRequest")
}

// Helper: Force RAR expiration by setting RequiredBy in the past
func forceRARExpiration(namespace, name string) { //nolint:unused
	EventuallyWithOffset(1, func() error {
		rar := &remediationv1.RemediationApprovalRequest{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, rar); err != nil {
			return err
		}

		// Set RequiredBy to 1 second ago to force immediate expiration
		past := metav1.NewTime(time.Now().Add(-1 * time.Second))
		rar.Spec.RequiredBy = past

		return k8sClient.Update(ctx, rar)
	}, timeout, interval).Should(Succeed(), "Failed to force RAR expiration")
}

var _ = Describe("RemediationApprovalRequest Conditions Integration", Label("integration", "approval", "conditions"), func() {

	Context("DD-CRD-002-RAR: Initial Condition Setting", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-rar-create")
			rrName = fmt.Sprintf("rr-create-%d", time.Now().UnixNano())
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should set all three conditions correctly when RAR is created", func() {
			By("Creating a RAR directly")
			rarName := fmt.Sprintf("rar-%s", rrName)
			requiredBy := metav1.NewTime(time.Now().Add(30 * time.Minute))

			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rarName,
					Namespace: namespace,
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      rrName,
						Namespace: namespace,
					},
					AIAnalysisRef: remediationv1.ObjectRef{
						Name: "ai-" + rrName,
					},
					Confidence:      0.4,
					ConfidenceLevel: "low",
					Reason:          "Confidence below 0.8 threshold",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
						WorkflowID:     "test-workflow-1",
						Version:        "v1.0.0",
						ContainerImage: "kubernaut/workflows:test",
						Rationale:      "Test workflow for approval",
					},
					InvestigationSummary: "Test investigation summary",
					RecommendedActions: []remediationv1.ApprovalRecommendedAction{
						{Action: "Restart pod", Rationale: "Test action"},
					},
					WhyApprovalRequired: "Low confidence requires human review",
					RequiredBy:          requiredBy,
				},
			}

			// Create RAR first (only persists Spec)
			Expect(k8sClient.Create(ctx, rar)).To(Succeed())

			// Fetch the created object to get server-set fields (UID, ResourceVersion, etc.)
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: rarName, Namespace: namespace}, rar)
			}, timeout, interval).Should(Succeed(), "Failed to fetch created RAR")

			// Set initial conditions on the fetched object (simulating creator logic)
			rarconditions.SetApprovalPending(rar, true,
				fmt.Sprintf("Awaiting decision, expires %s", requiredBy.Format(time.RFC3339)), nil)
			rarconditions.SetApprovalDecided(rar, false,
				rarconditions.ReasonPendingDecision, "No decision yet", nil)
			rarconditions.SetApprovalExpired(rar, false, "Approval has not expired", nil)

			// Update status to persist conditions (Kubernetes API requirement)
			Expect(k8sClient.Status().Update(ctx, rar)).To(Succeed())

			By("Verifying ApprovalPending=True condition")
			fetched := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rarName, Namespace: namespace}, fetched); err != nil {
					return false
				}
				cond := meta.FindStatusCondition(fetched.Status.Conditions, rarconditions.ConditionApprovalPending)
				return cond != nil && cond.Status == metav1.ConditionTrue && cond.Reason == rarconditions.ReasonAwaitingDecision
			}, timeout, interval).Should(BeTrue(), "ApprovalPending should be True with reason=AwaitingDecision")

			By("Verifying ApprovalDecided=False condition")
			decidedCond := meta.FindStatusCondition(fetched.Status.Conditions, rarconditions.ConditionApprovalDecided)
			Expect(decidedCond).NotTo(BeNil(), "ApprovalDecided condition should exist")
			Expect(decidedCond.Status).To(Equal(metav1.ConditionFalse), "ApprovalDecided should be False initially")
			Expect(decidedCond.Reason).To(Equal(rarconditions.ReasonPendingDecision), "ApprovalDecided reason should be PendingDecision")

			By("Verifying ApprovalExpired=False condition")
			expiredCond := meta.FindStatusCondition(fetched.Status.Conditions, rarconditions.ConditionApprovalExpired)
			Expect(expiredCond).NotTo(BeNil(), "ApprovalExpired condition should exist")
			Expect(expiredCond.Status).To(Equal(metav1.ConditionFalse), "ApprovalExpired should be False initially")
			Expect(expiredCond.Reason).To(Equal(rarconditions.ReasonNotExpired), "ApprovalExpired reason should be NotExpired")

			GinkgoWriter.Printf("✅ DD-CRD-002-RAR: Initial conditions set correctly at RAR creation\n")
		})
	})

	// Phase 2 tests (Approved Path, Rejected Path, Expired Path) moved to E2E suite:
	// - test/e2e/remediationorchestrator/approval_e2e_test.go
})
