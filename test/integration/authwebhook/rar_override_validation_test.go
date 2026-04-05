/*
Copyright 2026 Jordi Gil.

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

package authwebhook

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TDD Phase: RED — Issue #594, G3: Authwebhook Override Validation Integration Test
// BR-ORCH-031: Webhook rejects override referencing non-existent RW
//
// Relocated from RO integration suite (IT-RO-594-005 → IT-AW-594-005)
// because the RO suite has no admission webhook wiring.

var _ = Describe("BR-ORCH-031: RAR Override Webhook Validation (#594)", Label("integration", "override"), func() {
	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
	})

	Context("IT-AW-594-005: Override referencing non-existent RW → webhook denies", func() {
		It("should reject the status update when override references a RW that does not exist", func() {
			By("Creating a RAR without override (initial state)")
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rar-override-deny-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr",
						Namespace: namespace,
					},
					AIAnalysisRef: remediationv1.ObjectRef{
						Name: "test-analysis",
					},
					Confidence:           0.72,
					ConfidenceLevel:      "medium",
					Reason:               "Below threshold",
					InvestigationSummary: "Test investigation",
					WhyApprovalRequired:  "Medium confidence",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
						WorkflowID:      "wf-original",
						Version:         "1.0.0",
						ExecutionBundle: "original-bundle:v1",
						Rationale:       "AI selected",
					},
					RecommendedActions: []remediationv1.ApprovalRecommendedAction{
						{Action: "Restart", Rationale: "OOM recovery"},
					},
					RequiredBy: metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
				},
			}
			createAndWaitForCRD(ctx, k8sClient, rar)

			By("Updating status with override referencing non-existent RW")
			rar.Status.Decision = remediationv1.ApprovalDecisionApproved
			rar.Status.DecisionMessage = "Override to non-existent workflow"
			rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{
				WorkflowName: "this-workflow-does-not-exist",
				Rationale:    "testing webhook rejection",
			}

			err := k8sClient.Status().Update(ctx, rar)
			Expect(err).To(HaveOccurred(), "webhook should deny override with non-existent RW")
			Expect(err.Error()).To(ContainSubstring("not found"),
				"error message should indicate RW was not found")
		})
	})
})
