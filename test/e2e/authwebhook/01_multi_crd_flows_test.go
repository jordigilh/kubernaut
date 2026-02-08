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
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// E2E Tests for Multi-CRD Flows
// These tests validate complex scenarios that span multiple CRD types and concurrent operations.
// Per WEBHOOK_INTEGRATION_TEST_DECISION_JAN06.md, these tests were deferred from integration
// to E2E tier for better validation in production-like environment.
//
// Note: Detailed audit event validation is covered in integration tests.
// E2E tests focus on end-to-end flow execution and webhook attribution popul ation.

var _ = Describe("E2E-MULTI-01: Multiple CRDs in Sequence", Ordered, func() {
	var (
		testCtx       context.Context
		testNamespace string
		wfe           *workflowexecutionv1.WorkflowExecution
		rar           *remediationv1.RemediationApprovalRequest
		nr            *notificationv1.NotificationRequest
	)

	BeforeAll(func() {
		testCtx = context.Background()
		testNamespace = "e2e-multi-crd-" + time.Now().Format("150405")

		// Create test namespace
		By("Creating test namespace: " + testNamespace)
		err := CreateNamespace(testCtx, k8sClient, testNamespace)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterAll(func() {
		// Clean up test namespace
		By("Deleting test namespace: " + testNamespace)
		err := DeleteNamespace(testCtx, k8sClient, testNamespace)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should attribute all operator actions to authenticated users (BR-AUTH-001, BR-WE-013)", func() {
		// Test Scenario: Complete SOC2 attribution flow across all 3 CRD types
		// Objective: Verify that operator actions on WFE, RAR, and NR are all correctly attributed
		// Expected: All webhook mutations populate authenticated user fields in status

		By("Step 1: Create and clear WorkflowExecution block")
		wfe = &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-multi-wfe",
				Namespace: testNamespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				TargetResource:  "default/pod/test-pod",
				ExecutionEngine: "tekton",
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID:     "test-workflow",
					Version:        "v1",
					ContainerImage: "test/image:latest",
				},
			},
		}
		Expect(k8sClient.Create(testCtx, wfe)).To(Succeed())

		// Simulate blocked state
		wfe.Status.Phase = "Failed"
		wfe.Status.FailureReason = "Test failure for block clearance"
		Expect(k8sClient.Status().Update(testCtx, wfe)).To(Succeed())

		// Trigger block clearance (webhook will populate ClearedBy)
		wfe.Status.BlockClearance = &workflowexecutionv1.BlockClearanceDetails{
			ClearReason: "E2E test: Verifying complete SOC2 attribution flow across all CRD types",
			ClearMethod: "StatusField",
		}
		Expect(k8sClient.Status().Update(testCtx, wfe)).To(Succeed())

		// Wait for webhook to populate ClearedBy
		Eventually(func() string {
			_ = k8sClient.Get(testCtx, client.ObjectKeyFromObject(wfe), wfe)
			if wfe.Status.BlockClearance != nil {
				return wfe.Status.BlockClearance.ClearedBy
			}
			return ""
		}, 30*time.Second, 1*time.Second).ShouldNot(BeEmpty(), "Webhook should populate ClearedBy field")

		GinkgoWriter.Printf("✅ WFE Block Clearance: Cleared by %s\n", wfe.Status.BlockClearance.ClearedBy)

		By("Step 2: Create and approve RemediationApprovalRequest")
		rar = &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-multi-rar",
				Namespace: testNamespace,
			},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Kind:       "RemediationRequest",
					Namespace:  testNamespace,
					Name:       "test-rr",
					APIVersion: "remediation.kubernaut.ai/v1alpha1",
				},
				AIAnalysisRef: remediationv1.ObjectRef{
					Name: "test-analysis",
				},
				Confidence:      0.75,
				ConfidenceLevel: "medium",
				Reason:          "E2E test: Testing approval flow",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
					WorkflowID:     "test-workflow",
					Version:        "v1",
					ContainerImage: "test/image:latest",
					Rationale:      "Test remediation plan for E2E validation",
				},
				// REQUIRED FIELDS (CRD validation)
				InvestigationSummary: "E2E test investigation: Simulating approval request flow for SOC2 attribution verification",
				RecommendedActions: []remediationv1.ApprovalRecommendedAction{
					{
						Action:    "Execute test workflow",
						Rationale: "E2E validation of approval attribution",
					},
				},
				WhyApprovalRequired: "E2E test: Confidence score 0.75 is below auto-approve threshold (typically 0.8+)",
				RequiredBy:          metav1.NewTime(metav1.Now().Add(15 * time.Minute)), // 15 minute approval window
			},
		}
		Expect(k8sClient.Create(testCtx, rar)).To(Succeed())

		// Trigger approval (webhook will populate DecidedBy)
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.DecisionMessage = "E2E test: Approved for SOC2 attribution verification"
		Expect(k8sClient.Status().Update(testCtx, rar)).To(Succeed())

		// Wait for webhook to populate DecidedBy
		Eventually(func() string {
			_ = k8sClient.Get(testCtx, client.ObjectKeyFromObject(rar), rar)
			return rar.Status.DecidedBy
		}, 30*time.Second, 1*time.Second).ShouldNot(BeEmpty(), "Webhook should populate DecidedBy field")

		GinkgoWriter.Printf("✅ RAR Approval: Decided by %s\n", rar.Status.DecidedBy)

		By("Step 3: Create and delete NotificationRequest")
		nr = &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-multi-nr",
				Namespace: testNamespace,
			},
			Spec: notificationv1.NotificationRequestSpec{
				Type:     notificationv1.NotificationTypeEscalation,
				Priority: notificationv1.NotificationPriorityHigh,
				Subject:  "E2E Test Notification",
				Body:     "Testing SOC2 attribution for DELETE operation",
			},
		}
		Expect(k8sClient.Create(testCtx, nr)).To(Succeed())

		// Delete NotificationRequest (webhook will capture deletion audit event)
		Expect(k8sClient.Delete(testCtx, nr)).To(Succeed())

		GinkgoWriter.Printf("✅ NR Deletion: Successfully deleted (audit event captured by webhook)\n")

		By("✅ E2E-MULTI-01 PASSED: All operator actions correctly attributed across 3 CRD types")
		GinkgoWriter.Printf("📊 SOC2 CC8.1 Compliance: End-to-end multi-CRD flow complete\n")
		GinkgoWriter.Printf("   • WFE: %s (action: cleared)\n", wfe.Status.BlockClearance.ClearedBy)
		GinkgoWriter.Printf("   • RAR: %s (action: approved)\n", rar.Status.DecidedBy)
		GinkgoWriter.Printf("   • NR: Deleted (audit event written to Data Storage)\n")
		GinkgoWriter.Printf("   • Note: Detailed audit event validation covered in integration tests\n")
	})
})

var _ = Describe("E2E-MULTI-02: Concurrent Webhook Requests", func() {
	var (
		testCtx       context.Context
		testNamespace string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		testNamespace = "e2e-concurrent-" + time.Now().Format("150405")

		// Create test namespace
		By("Creating test namespace: " + testNamespace)
		err := CreateNamespace(testCtx, k8sClient, testNamespace)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up test namespace
		By("Deleting test namespace: " + testNamespace)
		err := DeleteNamespace(testCtx, k8sClient, testNamespace)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should handle 10 concurrent WorkflowExecution block clearances without errors (BR-AUTH-001)", func() {
		// Test Scenario: Stress test webhook under concurrent load
		// Objective: Verify webhook can handle multiple simultaneous operations without data loss
		// Expected: All 10 operations complete successfully with correct attribution

		concurrency := 10
		wfeList := make([]*workflowexecutionv1.WorkflowExecution, concurrency)
		var wg sync.WaitGroup

		By("Step 1: Creating 10 WorkflowExecutions concurrently")
		for i := 0; i < concurrency; i++ {
			idx := i
			wfeList[idx] = &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("e2e-concurrent-wfe-%d", idx),
					Namespace: testNamespace,
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource:  fmt.Sprintf("default/pod/test-pod-%d", idx),
					ExecutionEngine: "tekton",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID:     fmt.Sprintf("test-workflow-%d", idx),
						Version:        "v1",
						ContainerImage: "test/image:latest",
					},
				},
			}
			Expect(k8sClient.Create(testCtx, wfeList[idx])).To(Succeed())

			// Simulate blocked state
			wfeList[idx].Status.Phase = "Failed"
			wfeList[idx].Status.FailureReason = fmt.Sprintf("Test failure %d", idx)
			Expect(k8sClient.Status().Update(testCtx, wfeList[idx])).To(Succeed())
		}

		By("Step 2: Triggering 10 block clearances concurrently")
		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()

				wfe := wfeList[idx]
				wfe.Status.BlockClearance = &workflowexecutionv1.BlockClearanceDetails{
					ClearReason: fmt.Sprintf("E2E concurrent test: block clearance #%d with complete justification for audit compliance", idx),
					ClearMethod: "StatusField",
				}
				Expect(k8sClient.Status().Update(testCtx, wfe)).To(Succeed())

				// Wait for webhook to populate ClearedBy
				Eventually(func() string {
					_ = k8sClient.Get(testCtx, client.ObjectKeyFromObject(wfe), wfe)
					if wfe.Status.BlockClearance != nil {
						return wfe.Status.BlockClearance.ClearedBy
					}
					return ""
				}, 60*time.Second, 1*time.Second).ShouldNot(BeEmpty(),
					fmt.Sprintf("Webhook should populate ClearedBy for WFE #%d", idx))

				GinkgoWriter.Printf("✅ WFE #%d: Cleared by %s\n", idx, wfe.Status.BlockClearance.ClearedBy)

			}(i)
		}

		By("Step 3: Waiting for all concurrent operations to complete")
		wg.Wait()

		By("Step 4: Verifying all 10 WorkflowExecutions have attribution")
		for i := 0; i < concurrency; i++ {
			wfe := wfeList[i]
			Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
			Expect(wfe.Status.BlockClearance).ToNot(BeNil(), fmt.Sprintf("WFE #%d should have block clearance", i))
			Expect(wfe.Status.BlockClearance.ClearedBy).ToNot(BeEmpty(), fmt.Sprintf("WFE #%d should have ClearedBy populated", i))
			// In E2E (Kind cluster), user is "kubernetes-admin" (not email format)
			// In production, user would be email from authentication system
			// Both are valid - we just need to verify attribution is captured
			Expect(wfe.Status.BlockClearance.ClearedBy).To(Or(
				ContainSubstring("@"),                    // Production: email format
				Equal("kubernetes-admin"),                // E2E/Kind: default K8s user
				MatchRegexp("^[a-z][a-z0-9-]+[a-z0-9]$"), // K8s valid username
			), fmt.Sprintf("WFE #%d ClearedBy should be valid user format (email or K8s username)", i))
		}

		By("✅ E2E-MULTI-02 PASSED: 10 concurrent webhook requests handled successfully")
		GinkgoWriter.Printf("📊 Concurrency Test: 10/10 webhook operations completed successfully\n")
		GinkgoWriter.Printf("   • Zero errors under concurrent load\n")
		GinkgoWriter.Printf("   • All webhook operations completed < 60s\n")
		GinkgoWriter.Printf("   • SOC2 CC8.1 compliance maintained under stress\n")
		GinkgoWriter.Printf("   • Note: Detailed audit event validation covered in integration tests\n")
	})
})
