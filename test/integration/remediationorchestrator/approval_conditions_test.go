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
	"github.com/google/uuid"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	rarconditions "github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
)
var _ = Describe("RemediationApprovalRequest Conditions Integration", Label("integration", "approval", "conditions"), func() {

	Context("DD-CRD-002-RAR: Initial Condition Setting", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-rar-create")
			rrName = fmt.Sprintf("rr-create-%s", uuid.New().String()[:13])
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
					Namespace: ROControllerNamespace,
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      rrName,
						Namespace: ROControllerNamespace,
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
						ExecutionBundle: "kubernaut/workflows:test",
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
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
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
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, fetched); err != nil {
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
