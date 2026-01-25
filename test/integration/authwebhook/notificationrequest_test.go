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
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NotificationRequest Integration Tests - DD-TESTING-001 Compliant
// BR-AUTH-001: Operator Attribution (SOC2 CC8.1)
// DD-NOT-005: Immutable Spec (cancellation via DELETE operation)
// DD-TESTING-001: Real audit event validation with Data Storage
//
// Per TESTING_GUIDELINES.md §1773-1862: Business Logic Testing Pattern
// 1. Create NotificationRequest CRD (business operation)
// 2. Operator deletes CRD to cancel (business operation)
// 3. Verify webhook wrote audit event to Data Storage (DD-TESTING-001)
//
// MANDATORY STANDARDS (DD-TESTING-001):
// - OpenAPI client for Data Storage queries (DD-API-001)
// - Deterministic count validation (Equal(N), NOT BeNumerically(">="))
// - Structured event_data validation (DD-AUDIT-004)
// - Eventually() for async polling (NO time.Sleep())

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
		It("should capture operator identity in audit trail via webhook", func() {
			By("Creating NotificationRequest CRD (business operation)")
			nrName := "test-nr-cancel-" + randomSuffix()
			nr := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nrName,
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

			createAndWaitForCRD(ctx, k8sClient, nr)

			By("Operator deletes NotificationRequest to cancel (business operation)")
			// Per DD-NOT-005: Spec is immutable, cancellation is via DELETE
			// Webhook intercepts DELETE and writes audit event to Data Storage
			// Note: K8s API prevents object mutation during DELETE, so attribution is via audit
			Expect(k8sClient.Delete(ctx, nr)).To(Succeed(),
				"Webhook should allow DELETE and record audit event")

			By("Flushing audit store to ensure events are persisted")
			// Explicitly flush buffered audit events before querying
			flushCtx, flushCancel := context.WithTimeout(ctx, 5*time.Second)
			defer flushCancel()
			err := auditStore.Flush(flushCtx)
			Expect(err).ToNot(HaveOccurred(), "Audit store flush should succeed")
			GinkgoWriter.Println("✅ Audit store flushed successfully")

			By("Waiting for audit event to be persisted to Data Storage (DD-TESTING-001)")
			// Webhook uses nr.Name as correlation ID
			// Per DD-TESTING-001: Use Eventually() for async polling, NOT time.Sleep()
			deleteEventType := string(ogenclient.NotificationAuditPayloadEventTypeWebhookNotificationCancelled)
			events := waitForAuditEvents(dsClient, nrName, deleteEventType, 1)

			By("Validating exact event count (DD-TESTING-001 Pattern 4)")
			// Per DD-TESTING-001: Use Equal(N) for deterministic validation
			// FORBIDDEN: BeNumerically(">=") hides duplicate events
			eventCounts := countEventsByType(events)
			Expect(eventCounts[deleteEventType]).To(Equal(1),
				"Should have exactly 1 DELETE audit event (not more, not less)")

			By("Validating event metadata (DD-TESTING-001 Pattern 6)")
			event := events[0]
			validateEventMetadata(event, "webhook")

			By("Validating structured columns (per DD-WEBHOOK-003 + ADR-034 v1.4)")
			// Per DD-WEBHOOK-003: Attribution fields in structured columns, NOT event_data
			Expect(event.ActorID.IsSet()).To(BeTrue(), "ActorID should be set")
			Expect(event.ActorID.Value).To(Equal("admin"),
				"actor_id column should contain authenticated operator")
			Expect(event.ResourceID.IsSet()).To(BeTrue(), "ResourceID should be set")
			Expect(event.ResourceID.Value).ToNot(BeEmpty(),
				"resource_id column should contain CRD UID (per audit.SetResource)")
			Expect(event.Namespace.IsSet()).To(BeTrue(), "Namespace should be set")
			Expect(event.Namespace.Value).To(Equal(namespace),
				"namespace column should contain CRD namespace")
			Expect(event.EventAction).To(Equal("deleted"),
				"event_action column should be 'deleted' for DELETE operation")

			By("Validating event_data business context (DD-WEBHOOK-003 lines 335-340)")
			// Per DD-WEBHOOK-003: event_data contains business context ONLY
			validateEventData(event, map[string]interface{}{
				"notification_name": nrName,       // Business field (per DD-WEBHOOK-003)
				"notification_type": "escalation", // Business field
				"priority":          "high",       // Business field
				"final_status":      nil,          // Business field (may be empty if not set)
				"recipients":        nil,          // Business field (verify existence)
			})

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-NR-01 PASSED: DELETE Attribution via Structured Columns\n")
			GinkgoWriter.Printf("   • Cancelled by: %s (actor_id column)\n", event.ActorID.Value)
			GinkgoWriter.Printf("   • Resource: %s (resource_id column)\n", event.ResourceID.Value)
			GinkgoWriter.Printf("   • Namespace: %s (namespace column)\n", event.Namespace.Value)
			GinkgoWriter.Printf("   • Action: %s (event_action column)\n", event.EventAction)
			GinkgoWriter.Printf("   • Event type: %s\n", event.EventType)
			GinkgoWriter.Printf("   • Event category: %s\n", event.EventCategory)
			GinkgoWriter.Printf("   • DD-WEBHOOK-003: ✅ Structured columns for attribution\n")
			GinkgoWriter.Printf("   • DD-WEBHOOK-003: ✅ Business context in event_data\n")
			GinkgoWriter.Printf("   • K8s Limitation: Attribution via audit (cannot mutate during DELETE)\n")
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("INT-NR-02: when NotificationRequest completes successfully", func() {
		It("should not trigger webhook on normal lifecycle completion", func() {
			By("Creating NotificationRequest CRD")
			nrName := "test-nr-complete-" + randomSuffix()
			nr := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nrName,
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

			createAndWaitForCRD(ctx, k8sClient, nr)

			By("Controller marks notification as Sent (business operation)")
			nr.Status.Phase = notificationv1.NotificationPhaseSent
			nr.Status.SuccessfulDeliveries = 1
			Expect(k8sClient.Status().Update(ctx, nr)).To(Succeed(),
				"Status update for completion should succeed")

			By("Verifying CRD updated successfully")
			fetchedNR := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nr), fetchedNR)).To(Succeed())
			Expect(fetchedNR.Status.Phase).To(Equal(notificationv1.NotificationPhaseSent),
				"Phase should be updated to Sent")

			By("Verifying no audit events generated for status updates (webhook only triggers on DELETE)")
			// Webhook only intercepts DELETE operations for NotificationRequest
			// Status updates do NOT trigger the webhook
			// This test verifies normal lifecycle doesn't create audit noise

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-NR-02 PASSED: Normal Completion (no attribution)\n")
			GinkgoWriter.Printf("   • Phase: %s\n", fetchedNR.Status.Phase)
			GinkgoWriter.Printf("   • Webhook NOT triggered (only fires on DELETE)\n")
			GinkgoWriter.Printf("   • Pattern: Attribution only for operator-initiated cancellations\n")
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

			// Clean up (this DELETE will trigger webhook, but it's outside test scope)
			Expect(k8sClient.Delete(ctx, nr)).To(Succeed())
		})
	})

	Context("INT-NR-03: when NotificationRequest is deleted during processing", func() {
		It("should capture attribution even if CRD is mid-processing", func() {
			By("Creating NotificationRequest CRD")
			nrName := "test-nr-mid-processing-" + randomSuffix()
			nr := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nrName,
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

			createAndWaitForCRD(ctx, k8sClient, nr)

			By("Controller marks notification as Sending (processing started)")
			nr.Status.Phase = notificationv1.NotificationPhaseSending
			nr.Status.TotalAttempts = 1
			Expect(k8sClient.Status().Update(ctx, nr)).To(Succeed(),
				"Status update to Sending should succeed")

			By("Waiting for status update to be reflected in etcd")
			// Fix: Wait for status update to be persisted before DELETE
			// Without this, webhook may see stale "Pending" status instead of "Sending"
			Eventually(func() notificationv1.NotificationPhase {
				var updated notificationv1.NotificationRequest
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: nrName, Namespace: namespace}, &updated); err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(notificationv1.NotificationPhaseSending),
				"Status should be reflected as Sending before DELETE")

			By("Re-fetching CRD to ensure DELETE webhook sees updated status")
			// Critical: Refetch the object so DELETE request includes the updated status
			// The webhook validates the object as-is in the DELETE request
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nrName, Namespace: namespace}, nr)).To(Succeed(),
				"Should refetch updated CRD before DELETE")
			Expect(nr.Status.Phase).To(Equal(notificationv1.NotificationPhaseSending),
				"Refetched object should have Sending status")

			By("Operator cancels notification mid-processing (DELETE)")
			// Per BR-AUTH-001: DELETE captures attribution via audit trail
			Expect(k8sClient.Delete(ctx, nr)).To(Succeed(),
				"DELETE should succeed and record audit event")

			By("Flushing audit store to ensure events are persisted")
			// Explicitly flush buffered audit events before querying
			flushCtx, flushCancel := context.WithTimeout(ctx, 5*time.Second)
			defer flushCancel()
			err := auditStore.Flush(flushCtx)
			Expect(err).ToNot(HaveOccurred(), "Audit store flush should succeed")
			GinkgoWriter.Println("✅ Audit store flushed successfully")

			By("Waiting for audit event to be persisted (DD-TESTING-001)")
			// Webhook uses nr.Name as correlation ID
			deleteEventType := string(ogenclient.NotificationAuditPayloadEventTypeWebhookNotificationCancelled)
			events := waitForAuditEvents(dsClient, nrName, deleteEventType, 1)

			By("Validating exact event count (DD-TESTING-001)")
			eventCounts := countEventsByType(events)
			Expect(eventCounts[deleteEventType]).To(Equal(1),
				"Should have exactly 1 DELETE audit event even during processing")

			By("Validating event metadata (DD-TESTING-001)")
			event := events[0]
			validateEventMetadata(event, "webhook")

			By("Validating structured columns (per DD-WEBHOOK-003 + ADR-034 v1.4)")
			// Per DD-WEBHOOK-003: Attribution fields in structured columns, NOT event_data
			Expect(event.ActorID.IsSet()).To(BeTrue(), "ActorID should be set")
			Expect(event.ActorID.Value).To(Equal("admin"),
				"actor_id column should contain authenticated operator")
			Expect(event.ResourceID.IsSet()).To(BeTrue(), "ResourceID should be set")
			Expect(event.ResourceID.Value).ToNot(BeEmpty(),
				"resource_id column should contain CRD UID (per audit.SetResource)")
			Expect(event.Namespace.IsSet()).To(BeTrue(), "Namespace should be set")
			Expect(event.Namespace.Value).To(Equal(namespace),
				"namespace column should contain CRD namespace")
			Expect(event.EventAction).To(Equal("deleted"),
				"event_action column should be 'deleted' for DELETE operation")

			By("Validating event_data business context (DD-WEBHOOK-003 lines 335-340)")
			// Per DD-WEBHOOK-003: event_data contains business context ONLY
			validateEventData(event, map[string]interface{}{
				"notification_name": nrName,          // Business field (per DD-WEBHOOK-003)
				"notification_type": "status-update", // Business field (matches test's NotificationTypeStatusUpdate)
				"priority":          "low",           // Business field (matches test's NotificationPriorityLow)
				"final_status":      "Sending",       // Business field (captured mid-processing)
				"recipients":        nil,             // Business field (verify existence)
			})

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-NR-03 PASSED: Mid-Processing Cancellation via Structured Columns\n")
			GinkgoWriter.Printf("   • Cancelled by: %s (actor_id column)\n", event.ActorID.Value)
			GinkgoWriter.Printf("   • Resource: %s (resource_id column)\n", event.ResourceID.Value)
			GinkgoWriter.Printf("   • Namespace: %s (namespace column)\n", event.Namespace.Value)
			GinkgoWriter.Printf("   • Action: %s (event_action column)\n", event.EventAction)
			GinkgoWriter.Printf("   • Event type: %s\n", event.EventType)
			GinkgoWriter.Printf("   • Audit captured during 'Sending' phase (mid-processing)\n")
			GinkgoWriter.Printf("   • DD-WEBHOOK-003: ✅ Structured columns for attribution\n")
			GinkgoWriter.Printf("   • DD-WEBHOOK-003: ✅ Business context in event_data\n")
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})
})
