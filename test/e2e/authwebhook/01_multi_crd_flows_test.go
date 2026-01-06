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
	auditclient "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// E2E Tests for Multi-CRD Flows
// These tests validate complex scenarios that span multiple CRD types and concurrent operations.
// Per WEBHOOK_INTEGRATION_TEST_DECISION_JAN06.md, these tests were deferred from integration
// to E2E tier for better validation in production-like environment.

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
		// Expected: All 3 audit events have authenticated actor_id and complete event_data

		By("Step 1: Create and clear WorkflowExecution block")
		wfe = &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-multi-wfe",
				Namespace: testNamespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				TargetResource: "default/pod/test-pod",
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					Name:    "test-workflow",
					Version: "v1",
				},
			},
		}
		Expect(k8sClient.Create(testCtx, wfe)).To(Succeed())

		// Simulate blocked state
		wfe.Status.Phase = "Blocked"
		Expect(k8sClient.Status().Update(testCtx, wfe)).To(Succeed())

		// Trigger block clearance (webhook will populate ClearedBy)
		wfe.Status.BlockClearance = &workflowexecutionv1.BlockClearanceDetails{
			ClearReason: "E2E test: Verifying complete SOC2 attribution flow",
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
		}, 15*time.Second, 1*time.Second).ShouldNot(BeEmpty(), "Webhook should populate ClearedBy field")

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
					Name:        "test-workflow",
					Description: "Test remediation plan",
				},
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
		}, 15*time.Second, 1*time.Second).ShouldNot(BeEmpty(), "Webhook should populate DecidedBy field")

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

		By("Step 4: Verify all 3 audit events have correct actors (DD-TESTING-001)")
		// Query audit events for all 3 operations
		// Per DD-TESTING-001: Use exact event counts and structured content validation

		// WorkflowExecution block clearance audit event
		Eventually(func() int {
			resp, err := auditClient.ListAuditEventsWithResponse(testCtx, &auditclient.ListAuditEventsParams{
				EventType:  ptr("webhook.workflowexecution.block.cleared"),
				ResourceId: ptr(string(wfe.UID)),
			})
			if err != nil || resp.StatusCode() != 200 {
				return 0
			}
			return len(*resp.JSON200.Events)
		}, 30*time.Second, 2*time.Second).Should(Equal(1), "Should have exactly 1 WFE audit event")

		weAuditResp, err := auditClient.ListAuditEventsWithResponse(testCtx, &auditclient.ListAuditEventsParams{
			EventType:  ptr("webhook.workflowexecution.block.cleared"),
			ResourceId: ptr(string(wfe.UID)),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(weAuditResp.StatusCode()).To(Equal(200))
		weEvents := *weAuditResp.JSON200.Events
		Expect(weEvents).To(HaveLen(1))
		weEvent := weEvents[0]

		// RemediationApprovalRequest approval audit event
		Eventually(func() int {
			resp, err := auditClient.ListAuditEventsWithResponse(testCtx, &auditclient.ListAuditEventsParams{
				EventType:  ptr("webhook.remediationapprovalrequest.approved"),
				ResourceId: ptr(string(rar.UID)),
			})
			if err != nil || resp.StatusCode() != 200 {
				return 0
			}
			return len(*resp.JSON200.Events)
		}, 30*time.Second, 2*time.Second).Should(Equal(1), "Should have exactly 1 RAR audit event")

		rarAuditResp, err := auditClient.ListAuditEventsWithResponse(testCtx, &auditclient.ListAuditEventsParams{
			EventType:  ptr("webhook.remediationapprovalrequest.approved"),
			ResourceId: ptr(string(rar.UID)),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(rarAuditResp.StatusCode()).To(Equal(200))
		rarEvents := *rarAuditResp.JSON200.Events
		Expect(rarEvents).To(HaveLen(1))
		rarEvent := rarEvents[0]

		// NotificationRequest deletion audit event
		Eventually(func() int {
			resp, err := auditClient.ListAuditEventsWithResponse(testCtx, &auditclient.ListAuditEventsParams{
				EventType:  ptr("webhook.notificationrequest.deleted"),
				ResourceId: ptr(string(nr.UID)),
			})
			if err != nil || resp.StatusCode() != 200 {
				return 0
			}
			return len(*resp.JSON200.Events)
		}, 30*time.Second, 2*time.Second).Should(Equal(1), "Should have exactly 1 NR audit event")

		nrAuditResp, err := auditClient.ListAuditEventsWithResponse(testCtx, &auditclient.ListAuditEventsParams{
			EventType:  ptr("webhook.notificationrequest.deleted"),
			ResourceId: ptr(string(nr.UID)),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(nrAuditResp.StatusCode()).To(Equal(200))
		nrEvents := *nrAuditResp.JSON200.Events
		Expect(nrEvents).To(HaveLen(1))
		nrEvent := nrEvents[0]

		By("Step 5: Validate SOC2 CC8.1 attribution for all events")
		// Per DD-WEBHOOK-003: actor_id in structured column, business context in event_data

		// WFE audit event validation
		Expect(weEvent.ActorId).ToNot(BeNil(), "WFE audit event should have actor_id")
		Expect(*weEvent.ActorId).ToNot(BeEmpty(), "WFE actor_id should not be empty")
		Expect(*weEvent.ActorId).To(ContainSubstring("@"), "WFE actor_id should be email format")
		Expect(weEvent.EventCategory).To(Equal("webhook"))
		Expect(weEvent.EventAction).To(Equal("cleared"))

		// Validate WFE event_data business context
		eventData := weEvent.EventData.(map[string]interface{})
		Expect(eventData).To(HaveKey("workflow_name"))
		Expect(eventData).To(HaveKey("clear_reason"))
		Expect(eventData["clear_reason"]).To(ContainSubstring("E2E test"))

		// RAR audit event validation
		Expect(rarEvent.ActorId).ToNot(BeNil(), "RAR audit event should have actor_id")
		Expect(*rarEvent.ActorId).ToNot(BeEmpty(), "RAR actor_id should not be empty")
		Expect(*rarEvent.ActorId).To(ContainSubstring("@"), "RAR actor_id should be email format")
		Expect(rarEvent.EventCategory).To(Equal("webhook"))
		Expect(rarEvent.EventAction).To(Equal("approved"))

		// Validate RAR event_data business context
		rarEventData := rarEvent.EventData.(map[string]interface{})
		Expect(rarEventData).To(HaveKey("approval_request_name"))
		Expect(rarEventData).To(HaveKey("decision"))
		Expect(rarEventData["decision"]).To(Equal("approved"))

		// NR audit event validation
		Expect(nrEvent.ActorId).ToNot(BeNil(), "NR audit event should have actor_id")
		Expect(*nrEvent.ActorId).ToNot(BeEmpty(), "NR actor_id should not be empty")
		Expect(*nrEvent.ActorId).To(ContainSubstring("@"), "NR actor_id should be email format")
		Expect(nrEvent.EventCategory).To(Equal("webhook"))
		Expect(nrEvent.EventAction).To(Equal("deleted"))

		// Validate NR event_data business context
		nrEventData := nrEvent.EventData.(map[string]interface{})
		Expect(nrEventData).To(HaveKey("notification_name"))
		Expect(nrEventData).To(HaveKey("notification_type"))
		Expect(nrEventData["notification_type"]).To(Equal("escalation"))

		By("✅ E2E-MULTI-01 PASSED: All operator actions correctly attributed across 3 CRD types")
		GinkgoWriter.Printf("📊 SOC2 CC8.1 Compliance: 3/3 audit events have authenticated actors\n")
		GinkgoWriter.Printf("   • WFE: %s (action: cleared)\n", *weEvent.ActorId)
		GinkgoWriter.Printf("   • RAR: %s (action: approved)\n", *rarEvent.ActorId)
		GinkgoWriter.Printf("   • NR: %s (action: deleted)\n", *nrEvent.ActorId)
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
					TargetResource: fmt.Sprintf("default/pod/test-pod-%d", idx),
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						Name:    fmt.Sprintf("test-workflow-%d", idx),
						Version: "v1",
					},
				},
			}
			Expect(k8sClient.Create(testCtx, wfeList[idx])).To(Succeed())

			// Simulate blocked state
			wfeList[idx].Status.Phase = "Blocked"
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
					ClearReason: fmt.Sprintf("E2E concurrent test: block clearance #%d", idx),
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
				}, 30*time.Second, 1*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Webhook should populate ClearedBy for WFE #%d", idx))

			}(i)
		}

		By("Step 3: Waiting for all concurrent operations to complete")
		wg.Wait()

		By("Step 4: Verifying all 10 audit events were created (DD-TESTING-001)")
		// Query audit events for all 10 WorkflowExecutions
		for i := 0; i < concurrency; i++ {
			wfe := wfeList[i]

			Eventually(func() int {
				resp, err := auditClient.ListAuditEventsWithResponse(testCtx, &auditclient.ListAuditEventsParams{
					EventType:  ptr("webhook.workflowexecution.block.cleared"),
					ResourceId: ptr(string(wfe.UID)),
				})
				if err != nil || resp.StatusCode() != 200 {
					return 0
				}
				return len(*resp.JSON200.Events)
			}, 30*time.Second, 2*time.Second).Should(Equal(1), fmt.Sprintf("Should have exactly 1 audit event for WFE #%d", i))
		}

		By("Step 5: Validating all audit events have correct attribution")
		for i := 0; i < concurrency; i++ {
			wfe := wfeList[i]

			resp, err := auditClient.ListAuditEventsWithResponse(testCtx, &auditclient.ListAuditEventsParams{
				EventType:  ptr("webhook.workflowexecution.block.cleared"),
				ResourceId: ptr(string(wfe.UID)),
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(200))
			events := *resp.JSON200.Events
			Expect(events).To(HaveLen(1))

			event := events[0]
			Expect(event.ActorId).ToNot(BeNil(), fmt.Sprintf("WFE #%d audit event should have actor_id", i))
			Expect(*event.ActorId).ToNot(BeEmpty(), fmt.Sprintf("WFE #%d actor_id should not be empty", i))
			Expect(*event.ActorId).To(ContainSubstring("@"), fmt.Sprintf("WFE #%d actor_id should be email format", i))

			// Validate event_data business context
			eventData := event.EventData.(map[string]interface{})
			Expect(eventData).To(HaveKey("workflow_name"))
			Expect(eventData).To(HaveKey("clear_reason"))
			Expect(eventData["clear_reason"]).To(ContainSubstring(fmt.Sprintf("#%d", i)), fmt.Sprintf("WFE #%d event_data should have correct reason", i))
		}

		By("✅ E2E-MULTI-02 PASSED: 10 concurrent webhook requests handled successfully")
		GinkgoWriter.Printf("📊 Concurrency Test: 10/10 audit events created with correct attribution\n")
		GinkgoWriter.Printf("   • Zero errors under concurrent load\n")
		GinkgoWriter.Printf("   • All webhook operations completed < 30s\n")
		GinkgoWriter.Printf("   • SOC2 CC8.1 compliance maintained under stress\n")
	})
})

// Helper function to create string pointers
func ptr(s string) *string {
	return &s
}

