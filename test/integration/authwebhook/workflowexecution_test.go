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

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TDD RED Phase: WorkflowExecution Integration Tests
// BR-WE-013: Audit-Tracked Block Clearing
// BR-AUTH-001: Operator Attribution (SOC2 CC8.1)
// SOC2 CC7.4: Audit Completeness (reason validation)
//
// Per TESTING_GUIDELINES.md §1773-1862: Business Logic Testing Pattern
// 1. Create CRD (business operation)
// 2. Update status (business operation)
// 3. Verify webhook populated fields (side effect validation)
//
// Tests written BEFORE webhook handlers exist (TDD RED Phase)

var _ = Describe("BR-WE-013: WorkflowExecution Block Clearance Attribution", func() {
	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
	})

	Context("INT-WE-01: when operator clears workflow execution block", func() {
		It("should capture operator identity via webhook", func() {
			By("Creating WorkflowExecution CRD (business operation)")
			// ✅ CORRECT: Trigger business operation (create CRD)
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-clearance",
					Namespace: namespace,
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource:  "default/pod/test-pod",
					ExecutionEngine: "tekton",
				},
			}

			createAndWaitForCRD(ctx, k8sClient, wfe)

			By("Operator requests block clearance (business operation)")
			// ✅ CORRECT: Business logic - operator updates status.BlockClearance
			// Per BR-WE-013: Operator clears PreviousExecutionFailed block after investigation
			updateStatusAndWaitForWebhook(ctx, k8sClient, wfe,
				func() {
					// Populate status.BlockClearance with operator's reason
					// Webhook will populate ClearedBy and ClearedAt fields
					wfe.Status.BlockClearance = &workflowexecutionv1.BlockClearanceDetails{
						ClearReason: "Integration test clearance - validated operator decision after complete analysis review",
						ClearMethod: "StatusField", // Operator updated status directly
						// ClearedBy: "",  // Will be populated by webhook from K8s UserInfo
						// ClearedAt: nil, // Will be populated by webhook with current timestamp
					}
				},
				func() bool {
					// Verify webhook populated the authentication fields
					return wfe.Status.BlockClearance != nil &&
						wfe.Status.BlockClearance.ClearedBy != ""
				},
			)

			By("Verifying webhook populated authentication fields (side effect)")
			// ✅ CORRECT: Validate webhook side effects
			// Per SOC2 CC8.1: Operator attribution is captured
			Expect(wfe.Status.BlockClearance).ToNot(BeNil(),
				"BlockClearance should be set")
			Expect(wfe.Status.BlockClearance.ClearedBy).ToNot(BeEmpty(),
				"ClearedBy should be populated by webhook with K8s UserInfo (envtest provides 'admin' or service account)")
			// In production: user@example.com or system:serviceaccount:namespace:name
			// In envtest: "admin" or "system:serviceaccount:default:default"
			Expect(wfe.Status.BlockClearance.ClearedBy).To(Or(
				Equal("admin"),
				ContainSubstring("system:serviceaccount"),
				ContainSubstring("@")),
				"ClearedBy should be valid K8s user identity")
			Expect(wfe.Status.BlockClearance.ClearedAt.IsZero()).To(BeFalse(),
				"ClearedAt timestamp should be set by webhook")
			Expect(wfe.Status.BlockClearance.ClearReason).To(Equal("Integration test clearance - validated operator decision after complete analysis review"),
				"ClearReason should be preserved by webhook")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-WE-01 PASSED: Block Clearance Attribution\n")
			GinkgoWriter.Printf("   • Cleared by: %s\n", wfe.Status.BlockClearance.ClearedBy)
			GinkgoWriter.Printf("   • Cleared at: %s\n", wfe.Status.BlockClearance.ClearedAt.Time)
			GinkgoWriter.Printf("   • Reason: %s\n", wfe.Status.BlockClearance.ClearReason)
			GinkgoWriter.Printf("   • Method: %s\n", wfe.Status.BlockClearance.ClearMethod)
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("INT-WE-02: when clearance reason is missing", func() {
		It("should reject clearance request via webhook validation", func() {
			By("Creating WorkflowExecution CRD")
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-no-reason",
					Namespace: namespace,
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource:  "default/pod/test-pod",
					ExecutionEngine: "tekton",
				},
			}

			createAndWaitForCRD(ctx, k8sClient, wfe)

			By("Operator attempts clearance without reason (invalid business operation)")
			// Per SOC2 CC7.4: Audit completeness requires justification
			wfe.Status.BlockClearance = &workflowexecutionv1.BlockClearanceDetails{
				ClearReason: "", // ❌ Invalid - empty reason
				ClearMethod: "StatusField",
			}

			// ✅ CORRECT: Expect K8s API to reject due to webhook validation
			err := k8sClient.Status().Update(ctx, wfe)
			Expect(err).To(HaveOccurred(),
				"Webhook should reject clearance without reason (SOC2 CC7.4 violation)")
			Expect(err.Error()).To(ContainSubstring("reason"),
				"Error should mention missing reason")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-WE-02 PASSED: Reject Missing Reason\n")
			GinkgoWriter.Printf("   • Webhook correctly rejected empty reason\n")
			GinkgoWriter.Printf("   • Error: %s\n", err.Error())
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("INT-WE-03: when clearance reason is too short", func() {
		It("should reject clearance with weak justification", func() {
			By("Creating WorkflowExecution CRD")
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-weak-reason",
					Namespace: namespace,
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource:  "default/pod/test-pod",
					ExecutionEngine: "tekton",
				},
			}

			createAndWaitForCRD(ctx, k8sClient, wfe)

			By("Operator provides insufficient justification (SOC2 CC7.4 violation)")
			// Per BR-AUTH-001 + SOC2 CC7.4: Minimum 10 words for audit completeness
			wfe.Status.BlockClearance = &workflowexecutionv1.BlockClearanceDetails{
				ClearReason: "Test", // ❌ Invalid - < 10 words minimum
				ClearMethod: "StatusField",
			}

			// ✅ CORRECT: Webhook should enforce minimum reason length
			err := k8sClient.Status().Update(ctx, wfe)
			Expect(err).To(HaveOccurred(),
				"Webhook should reject weak justification (< 10 words, SOC2 CC7.4)")
			Expect(err.Error()).To(Or(
				ContainSubstring("words"),
				ContainSubstring("reason"),
			), "Error should mention reason validation")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-WE-03 PASSED: Reject Weak Justification\n")
			GinkgoWriter.Printf("   • Webhook correctly rejected < 10 word reason\n")
			GinkgoWriter.Printf("   • Error: %s\n", err.Error())
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})
})
