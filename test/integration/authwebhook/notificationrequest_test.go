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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TDD RED Phase: NotificationRequest Integration Tests
// BR-AUTH-001: Operator Attribution (SOC2 CC8.1)
// DD-NOT-005: Immutable Spec (cancellation via DELETE operation)
//
// Per TESTING_GUIDELINES.md §1773-1862: Business Logic Testing Pattern
// 1. Create NotificationRequest CRD (business operation)
// 2. Operator deletes CRD to cancel (business operation)
// 3. Verify webhook captured DELETE attribution via annotations (side effect)
//
// Tests written BEFORE webhook handlers exist (TDD RED Phase)

var _ = Describe("BR-AUTH-001: NotificationRequest Cancellation Attribution", func() {
	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
	})

	Context("INT-NR-01: when operator cancels notification via DELETE", func() {
		It("should capture operator identity in annotations via validating webhook", func() {
			By("Creating NotificationRequest CRD (business operation)")
			nr := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nr-cancel",
					Namespace: namespace,
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeEscalation,
					Priority: notificationv1.NotificationPriorityHigh,
					Subject:  "Test escalation notification",
					Body:     "This is a test notification that will be cancelled",
					Recipients: []notificationv1.Recipient{
						{Email: "oncall@example.com"},
					},
					Channels: []notificationv1.Channel{
						notificationv1.ChannelEmail,
					},
				},
			}

			createAndWaitForCRD(ctx, nr)

		By("Operator deletes NotificationRequest to cancel (business operation)")
		// Per DD-NOT-005: Spec is immutable, cancellation is via DELETE
		// Webhook will intercept DELETE and write audit trace for attribution
		// Note: K8s API prevents object mutation during DELETE, so attribution is via audit
		Expect(k8sClient.Delete(ctx, nr)).To(Succeed(),
			"Webhook should allow DELETE and record audit event")

		By("Verifying webhook recorded DELETE attribution in audit trail (side effect)")
		// The webhook writes audit events to the audit manager
		// In tests, we verify the mockAuditMgr received the event
		Eventually(func() int {
			return len(mockAuditMgr.events)
		}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
			"Webhook should have recorded at least 1 audit event for DELETE")

		// Verify the audit event content
		var deletionEvent *audit.AuditEvent
		for i := range mockAuditMgr.events {
			if mockAuditMgr.events[i].EventType == "notification.request.deleted" {
				deletionEvent = &mockAuditMgr.events[i]
				break
			}
		}

		Expect(deletionEvent).ToNot(BeNil(),
			"Audit trail should contain notification.request.deleted event")
		Expect(deletionEvent.ActorID).ToNot(BeEmpty(),
			"ActorID (operator identity) should be captured")
		Expect(deletionEvent.EventOutcome).To(Equal("success"),
			"Event outcome should be success")
		Expect(deletionEvent.ResourceType).To(Equal("NotificationRequest"),
			"ResourceType should be NotificationRequest")

		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		GinkgoWriter.Printf("✅ INT-NR-01 PASSED: DELETE Attribution via Audit Trail\n")
		GinkgoWriter.Printf("   • Cancelled by: %s\n", deletionEvent.ActorID)
		GinkgoWriter.Printf("   • Event type: %s\n", deletionEvent.EventType)
		GinkgoWriter.Printf("   • Resource: %s/%s\n", deletionEvent.ResourceType, deletionEvent.ResourceID)
		GinkgoWriter.Printf("   • K8s Limitation: Attribution via audit (cannot mutate during DELETE)\n")
		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("INT-NR-02: when NotificationRequest completes successfully", func() {
		It("should not modify CRD on normal lifecycle completion", func() {
			By("Creating NotificationRequest CRD")
			nr := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nr-complete",
					Namespace: namespace,
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeSimple,
					Priority: notificationv1.NotificationPriorityMedium,
					Subject:  "Test notification - normal completion",
					Body:     "This notification will complete normally",
					Recipients: []notificationv1.Recipient{
						{Email: "ops@example.com"},
					},
				},
			}

			createAndWaitForCRD(ctx, nr)

			By("Controller marks notification as Sent (business operation)")
			nr.Status.Phase = notificationv1.NotificationPhaseSent
			nr.Status.SuccessfulDeliveries = 1
			Expect(k8sClient.Status().Update(ctx, nr)).To(Succeed(),
				"Status update for completion should succeed")

			By("Verifying webhook did not add cancellation annotations")
			fetchedNR := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nr), fetchedNR)).To(Succeed())

			annotations := fetchedNR.GetAnnotations()
			if annotations != nil {
				Expect(annotations).ToNot(HaveKey("kubernaut.ai/cancelled-by"),
					"Normal completion should not have cancellation annotations")
				Expect(annotations).ToNot(HaveKey("kubernaut.ai/cancelled-at"),
					"Normal completion should not have cancellation annotations")
			}

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-NR-02 PASSED: Normal Completion (no attribution)\n")
			GinkgoWriter.Printf("   • Phase: %s\n", fetchedNR.Status.Phase)
			GinkgoWriter.Printf("   • No cancellation annotations (as expected)\n")
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

			// Clean up
			Expect(k8sClient.Delete(ctx, nr)).To(Succeed())
		})
	})

	Context("INT-NR-03: when NotificationRequest is deleted during processing", func() {
		It("should capture attribution even if CRD is mid-processing", func() {
			By("Creating NotificationRequest CRD")
			nr := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nr-mid-processing",
					Namespace: namespace,
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeStatusUpdate,
					Priority: notificationv1.NotificationPriorityLow,
					Subject:  "Test notification - cancelled mid-processing",
					Body:     "This notification will be cancelled while processing",
					Recipients: []notificationv1.Recipient{
						{Slack: "#dev-alerts"},
					},
				},
			}

			createAndWaitForCRD(ctx, nr)

			By("Controller marks notification as Sending (processing started)")
			nr.Status.Phase = notificationv1.NotificationPhaseSending
			nr.Status.TotalAttempts = 1
			Expect(k8sClient.Status().Update(ctx, nr)).To(Succeed(),
				"Status update to Sending should succeed")

		By("Resetting mock audit manager for this test")
		mockAuditMgr.events = []audit.AuditEvent{} // Clear events from previous tests

		By("Operator cancels notification mid-processing")
		// Per BR-AUTH-001: DELETE captures attribution via audit trail
		Expect(k8sClient.Delete(ctx, nr)).To(Succeed(),
			"DELETE should succeed and record audit event")

		By("Verifying webhook captured attribution during processing via audit trail")
		// Verify audit event was recorded
		Eventually(func() int {
			return len(mockAuditMgr.events)
		}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
			"Webhook should record audit event even during processing")

		// Verify the audit event content
		var deletionEvent *audit.AuditEvent
		for i := range mockAuditMgr.events {
			if mockAuditMgr.events[i].EventType == "notification.request.deleted" {
				deletionEvent = &mockAuditMgr.events[i]
				break
			}
		}

		Expect(deletionEvent).ToNot(BeNil(),
			"Audit trail should contain deletion event")
		Expect(deletionEvent.ActorID).ToNot(BeEmpty(),
			"ActorID should be captured even during processing")

		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		GinkgoWriter.Printf("✅ INT-NR-03 PASSED: Mid-Processing Cancellation via Audit\n")
		GinkgoWriter.Printf("   • Cancelled by: %s\n", deletionEvent.ActorID)
		GinkgoWriter.Printf("   • Audit captured during 'Sending' phase\n")
		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})
})

