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

package authwebhook

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TDD RED Phase: RemediationApprovalRequest Integration Tests
// BR-AUTH-001: Operator Attribution (SOC2 CC8.1)
// SOC2 CC7.4: Audit Completeness (decision message validation)
//
// Per TESTING_GUIDELINES.md §1773-1862: Business Logic Testing Pattern
// 1. Create CRD (business operation)
// 2. Update status with decision (business operation)
// 3. Verify webhook populated fields (side effect validation)
//
// Tests written BEFORE webhook handlers exist (TDD RED Phase)

var _ = Describe("BR-AUTH-001: RemediationApprovalRequest Decision Attribution", func() {
	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
	})

	Context("INT-RAR-01: when operator approves remediation request", func() {
		It("should capture operator identity via webhook", func() {
			By("Creating RemediationApprovalRequest CRD (business operation)")
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rar-approval",
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
					Confidence:           0.75,
					ConfidenceLevel:      "medium",
					Reason:               "Confidence below auto-approve threshold",
					InvestigationSummary: "Memory leak detected in payment service",
					WhyApprovalRequired:  "Medium confidence requires human validation",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
						WorkflowID:     "restart-pod-v1",
						Version:        "1.0.0",
						ContainerImage: "kubernaut/restart-pod:v1",
						Rationale:      "Standard pod restart for memory leak",
					},
					RecommendedActions: []remediationv1.ApprovalRecommendedAction{
						{Action: "Restart pod", Rationale: "Clear memory leak"},
					},
					RequiredBy: metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)), // 15 minutes
				},
			}

			createAndWaitForCRD(ctx, k8sClient, rar)

			By("Operator approves remediation (business operation)")
			// Per BR-AUTH-001: Operator updates status.Decision to "Approved"
			// Webhook will populate DecidedBy and DecidedAt fields
			updateStatusAndWaitForWebhook(ctx, k8sClient, rar,
				func() {
					rar.Status.Decision = remediationv1.ApprovalDecisionApproved
					rar.Status.DecisionMessage = "Reviewed investigation summary - memory leak confirmed, restart approved"
					// DecidedBy: "",  // Will be populated by webhook from K8s UserInfo
					// DecidedAt: nil, // Will be populated by webhook with current timestamp
				},
				func() bool {
					return rar.Status.DecidedBy != ""
				},
			)

			By("Verifying webhook populated authentication fields (side effect)")
			Expect(rar.Status.Decision).To(Equal(remediationv1.ApprovalDecisionApproved),
				"Decision should be Approved")
			Expect(rar.Status.DecidedBy).ToNot(BeEmpty(),
				"DecidedBy should be populated by webhook with K8s UserInfo")
			// In production: user@example.com or system:serviceaccount:namespace:name
			// In envtest: "admin" or "system:serviceaccount:default:default"
			Expect(rar.Status.DecidedBy).To(Or(
				Equal("admin"),
				ContainSubstring("system:serviceaccount"),
				ContainSubstring("@")),
				"DecidedBy should be valid K8s user identity")
			Expect(rar.Status.DecidedAt).ToNot(BeNil(),
				"DecidedAt timestamp should be set by webhook")
			Expect(rar.Status.DecisionMessage).To(Equal("Reviewed investigation summary - memory leak confirmed, restart approved"),
				"DecisionMessage should be preserved by webhook")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-RAR-01 PASSED: Approval Attribution\n")
			GinkgoWriter.Printf("   • Decision: %s\n", rar.Status.Decision)
			GinkgoWriter.Printf("   • Decided by: %s\n", rar.Status.DecidedBy)
			GinkgoWriter.Printf("   • Decided at: %s\n", rar.Status.DecidedAt.Time)
			GinkgoWriter.Printf("   • Message: %s\n", rar.Status.DecisionMessage)
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("INT-RAR-02: when operator rejects remediation request", func() {
		It("should capture operator identity for rejection", func() {
			By("Creating RemediationApprovalRequest CRD")
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rar-rejection",
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
					Confidence:           0.65,
					ConfidenceLevel:      "medium",
					Reason:               "Confidence requires validation",
					InvestigationSummary: "Disk space issue detected",
					WhyApprovalRequired:  "Medium confidence requires human validation",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
						WorkflowID:     "expand-pvc-v1",
						Version:        "1.0.0",
						ContainerImage: "kubernaut/expand-pvc:v1",
						Rationale:      "Expand PVC to resolve disk pressure",
					},
					RecommendedActions: []remediationv1.ApprovalRecommendedAction{
						{Action: "Expand PVC", Rationale: "Increase storage"},
					},
					RequiredBy: metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
				},
			}

			createAndWaitForCRD(ctx, k8sClient, rar)

			By("Operator rejects remediation (business operation)")
			// Per BR-AUTH-001: Operator updates status.Decision to "Rejected"
			updateStatusAndWaitForWebhook(ctx, k8sClient, rar,
				func() {
					rar.Status.Decision = remediationv1.ApprovalDecisionRejected
					rar.Status.DecisionMessage = "Disk expansion requires change control approval first"
				},
				func() bool {
					return rar.Status.DecidedBy != ""
				},
			)

			By("Verifying webhook populated authentication fields for rejection")
			Expect(rar.Status.Decision).To(Equal(remediationv1.ApprovalDecisionRejected),
				"Decision should be Rejected")
			Expect(rar.Status.DecidedBy).ToNot(BeEmpty(),
				"DecidedBy should be populated by webhook for rejection")
			// In production: user@example.com or system:serviceaccount:namespace:name
			// In envtest: "admin" or "system:serviceaccount:default:default"
			Expect(rar.Status.DecidedBy).To(Or(
				Equal("admin"),
				ContainSubstring("system:serviceaccount"),
				ContainSubstring("@")),
				"DecidedBy should be valid K8s user identity")
			Expect(rar.Status.DecidedAt).ToNot(BeNil(),
				"DecidedAt timestamp should be set for rejection")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-RAR-02 PASSED: Rejection Attribution\n")
			GinkgoWriter.Printf("   • Decision: %s\n", rar.Status.Decision)
			GinkgoWriter.Printf("   • Decided by: %s\n", rar.Status.DecidedBy)
			GinkgoWriter.Printf("   • Reason: %s\n", rar.Status.DecisionMessage)
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("INT-RAR-03: when decision is invalid", func() {
		It("should reject invalid decision via webhook validation", func() {
			By("Creating RemediationApprovalRequest CRD")
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rar-invalid",
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
					Confidence:           0.70,
					ConfidenceLevel:      "medium",
					Reason:               "Test reason",
					InvestigationSummary: "Test investigation",
					WhyApprovalRequired:  "Test approval reason",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
						WorkflowID:     "test-workflow",
						Version:        "1.0.0",
						ContainerImage: "test:v1",
						Rationale:      "Test rationale",
					},
					RecommendedActions: []remediationv1.ApprovalRecommendedAction{
						{Action: "Test", Rationale: "Test"},
					},
					RequiredBy: metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
				},
			}

			createAndWaitForCRD(ctx, k8sClient, rar)

			By("Operator provides invalid decision (business validation)")
			// The CRD has +kubebuilder:validation:Enum which should prevent this at API level
			// But webhook should also validate for defense-in-depth
			rar.Status.Decision = remediationv1.ApprovalDecision("Maybe") // Invalid decision

			err := k8sClient.Status().Update(ctx, rar)
			Expect(err).To(HaveOccurred(),
				"Webhook should reject invalid decision (not Approved/Rejected/Expired)")
			Expect(err.Error()).To(Or(
				ContainSubstring("decision"),
				ContainSubstring("Maybe"),
				ContainSubstring("enum"),
			), "Error should mention invalid decision")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-RAR-03 PASSED: Invalid Decision Rejection\n")
			GinkgoWriter.Printf("   • Webhook correctly rejected invalid decision\n")
			GinkgoWriter.Printf("   • Error: %s\n", err.Error())
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})
})
