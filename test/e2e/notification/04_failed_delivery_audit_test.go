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

package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// ========================================
// E2E Test: Failed Delivery Audit Event
// ========================================
//
// Business Requirements:
// - BR-NOT-062: Unified audit table integration
// - BR-NOT-063: Graceful audit degradation
// - BR-NOT-064: Audit event correlation
//
// Test Objective:
// Validate that failed notification deliveries generate audit events
// that are persisted to PostgreSQL via Data Storage Service.
//
// This test completes the audit event coverage matrix by testing
// the failure path end-to-end with real infrastructure.
//
// Defense-in-Depth: This test validates the FULL audit chain for failures:
// - Controller detects delivery failure → auditMessageFailed() called →
// - AuditHelpers.CreateMessageFailedEvent() → BufferedStore buffers →
// - HTTPClient sends to Data Storage → PostgreSQL persists
//
// Test Scenario:
// 1. Create NotificationRequest CRD with Email channel (not configured in E2E)
// 2. Controller processes and attempts delivery
// 3. Delivery fails (email service not configured)
// 4. Controller calls auditMessageFailed()
// 5. Verify audit event persisted to PostgreSQL via Data Storage API
// 6. Verify event follows ADR-034 format with error details
// 7. Verify correlation_id for workflow tracing
//
// Expected Results:
// - NotificationRequest CRD created successfully
// - Controller marks notification as Failed or PartiallySent
// - notification.message.failed audit event persisted to PostgreSQL
// - Audit event contains error details in event_data
// - Event follows ADR-034 format
// - Correlation ID enables workflow tracing
//
// ========================================

var _ = Describe("E2E Test: Failed Delivery Audit Event", Label("e2e", "audit", "failure"), func() {
	var (
		testCtx          context.Context
		testCancel       context.CancelFunc
		notification     *notificationv1alpha1.NotificationRequest
		dsClient         *ogenclient.Client
		dataStorageURL   string
		notificationName string
		notificationNS   string
		correlationID    string
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 2*time.Minute)

		// Generate unique identifiers for this test
		testID := time.Now().Format("20060102-150405")
		notificationName = "e2e-failed-delivery-" + testID
		notificationNS = "default"
		correlationID = "e2e-failed-remediation-" + testID

		// Use real Data Storage URL from Kind cluster
		dataStorageURL = fmt.Sprintf("http://localhost:%d", dataStorageNodePort)

		// ✅ DD-API-001 + DD-AUTH-014: Create authenticated OpenAPI client (MANDATORY)
		// Per DD-AUTH-014: All DataStorage requests require ServiceAccount Bearer tokens
		saTransport := testauth.NewServiceAccountTransport(e2eAuthToken)
		httpClient := &http.Client{
			Timeout:   20 * time.Second,
			Transport: saTransport,
		}
		var err error
		dsClient, err = ogenclient.NewClient(dataStorageURL, ogenclient.WithClient(httpClient))
		Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated DataStorage OpenAPI client")

		// Create NotificationRequest that will FAIL delivery
		// Strategy: Use Email channel which is NOT configured in E2E environment
		// This will cause controller to fail delivery and emit audit event
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationName,
				Namespace: notificationNS,
				Labels: map[string]string{
					"test-type": "failed-delivery-audit",
				},
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				// FIX: Set RemediationRequestRef to enable correlation_id matching in audit queries
				RemediationRequestRef: &corev1.ObjectReference{
					APIVersion: "kubernaut.ai/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       correlationID,
					Namespace:  notificationNS,
				},
				Type:     notificationv1alpha1.NotificationTypeSimple,
				Priority: notificationv1alpha1.NotificationPriorityCritical,
				Subject:  "E2E Failed Delivery Audit Test",
				Body:     "Testing failed delivery audit event persistence",
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelEmail, // Email service NOT configured → will fail
				},
				Recipients: []notificationv1alpha1.Recipient{
					{Email: "test@example.com"},
				},
				Metadata: map[string]string{
					"remediationRequestName": correlationID,
					"test-scenario":          "failed-delivery-audit",
				},
			},
		}
	})

	AfterEach(func() {
		testCancel()

		// Clean up NotificationRequest CRD if it exists
		if notification != nil {
			_ = k8sClient.Delete(testCtx, notification)
		}
	})

	It("should persist notification.message.failed audit event when delivery fails", func() {
		// Per TESTING_GUIDELINES.md: E2E tests MUST use real services, Skip() is FORBIDDEN
		Expect(dataStorageNodePort).ToNot(Equal(0),
			"REQUIRED: Data Storage not available\n"+
				"  Per TESTING_GUIDELINES.md: E2E tests MUST use real services\n"+
				"  Per DD-AUDIT-003: Audit infrastructure is MANDATORY\n"+
				"  Audit infrastructure should be deployed in SynchronizedBeforeSuite")

		// ===== STEP 1: Create NotificationRequest CRD =====
		By("Creating NotificationRequest CRD with Email channel (will fail)")
		err := k8sClient.Create(testCtx, notification)
		Expect(err).ToNot(HaveOccurred(), "NotificationRequest CRD creation should succeed")

		// Verify CRD was created
		createdNotification := &notificationv1alpha1.NotificationRequest{}
		err = k8sClient.Get(testCtx, types.NamespacedName{
			Name:      notificationName,
			Namespace: notificationNS,
		}, createdNotification)
		Expect(err).ToNot(HaveOccurred(), "Should be able to get created NotificationRequest")
		Expect(createdNotification.Name).To(Equal(notificationName))

		// ===== STEP 2: Wait for controller to process and fail delivery =====
		By("Waiting for controller to process notification and fail delivery")

		// Controller will attempt Email delivery, which will fail (service not configured)
		// Expected phases: Pending → Sending → Failed (or PartiallySent)
		// Give controller time to process and emit audit event
		Eventually(func() bool {
			var n notificationv1alpha1.NotificationRequest
			if err := k8sClient.Get(testCtx, types.NamespacedName{
				Name:      notificationName,
				Namespace: notificationNS,
			}, &n); err != nil {
				return false
			}

			// Check if controller has processed and recorded failure
			// Phase might be Failed (all channels failed) or PartiallySent (some succeeded, some failed)
			// or Sending (still attempting)
			// We're looking for delivery attempts recorded in status
			return len(n.Status.DeliveryAttempts) > 0
		}, 30*time.Second, 1*time.Second).Should(BeTrue(),
			"Controller should attempt delivery and record delivery attempt")

		// ===== STEP 3: Verify failed audit event persisted to PostgreSQL =====
		By("Verifying notification.message.failed audit event persisted to PostgreSQL")

		// Query Data Storage API for failed event
		// Controller calls auditMessageFailed() which creates notification.message.failed event
		// BufferedStore flushes to Data Storage → PostgreSQL
		Eventually(func() int {
			count := queryAuditEventCount(dsClient, correlationID, string(ogenclient.NotificationMessageFailedPayloadAuditEventEventData))
			GinkgoWriter.Printf("DEBUG: Found %d failed audit events for correlation_id=%s\n", count, correlationID)
			return count
		}, 30*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
			"Failed audit event should be persisted to PostgreSQL within 30 seconds")

		// ===== STEP 4: Verify ADR-034 compliance and error details =====
		By("Verifying ADR-034 compliance of failed audit event")

		events := queryAuditEvents(dsClient, correlationID)
		Expect(events).ToNot(BeEmpty(), "Should have at least one audit event")

		// Find the failed event
		var failedEvent *ogenclient.AuditEvent
		for i := range events {
			if events[i].EventType == string(ogenclient.NotificationMessageFailedPayloadAuditEventEventData) {
				failedEvent = &events[i]
				break
			}
		}
		Expect(failedEvent).ToNot(BeNil(), "Should have notification.message.failed event")

		// Validate ADR-034 required fields
		Expect(string(failedEvent.EventCategory)).To(Equal("notification"),
			"Event category should be 'notification'")
		Expect(failedEvent.EventAction).To(Equal("sent"),
			"Event action should be 'sent' (attempted delivery)")
		Expect(string(failedEvent.EventOutcome)).To(Equal("failure"),
			"Event outcome should be 'failure'")
		Expect(failedEvent.ActorType.IsSet()).To(BeTrue(), "Actor type should be set")
		Expect(failedEvent.ActorType.Value).To(Equal("service"),
			"Actor type should be 'service'")
		// NT-TEST-001 Fix: Expect actual service name from controller
		Expect(failedEvent.ActorID.IsSet()).To(BeTrue(), "Actor ID should be set")
		Expect(failedEvent.ActorID.Value).To(Equal("notification-controller"),
			"Actor ID should be 'notification-controller' (service name)")
		Expect(failedEvent.ResourceType.IsSet()).To(BeTrue(), "Resource type should be set")
		Expect(failedEvent.ResourceType.Value).To(Equal("NotificationRequest"),
			"Resource type should be 'NotificationRequest'")
		Expect(failedEvent.ResourceID.IsSet()).To(BeTrue(), "Resource ID should be set")
		Expect(failedEvent.ResourceID.Value).To(Equal(notificationName),
			"Resource ID should match notification name")
		Expect(failedEvent.CorrelationID).To(Equal(correlationID),
			"Correlation ID should match for workflow tracing")

		// Validate error details in event_data
		Expect(failedEvent.EventData).ToNot(BeNil(),
			"Event data should be populated with failure context")

		// EventData is interface{} - marshal then unmarshal to validate structure
		var eventData map[string]interface{}
		eventDataBytes, err := json.Marshal(failedEvent.EventData)
		Expect(err).ToNot(HaveOccurred(), "Event data should be marshallable")
		err = json.Unmarshal(eventDataBytes, &eventData)
		Expect(err).ToNot(HaveOccurred(), "Event data should be valid JSON")

		Expect(eventData).To(HaveKey("notification_id"),
			"Event data should contain notification_id")
		Expect(eventData).To(HaveKey("channel"),
			"Event data should contain channel (email)")
		Expect(eventData).To(HaveKey("error"),
			"Event data should contain error details for failed delivery")

		// Validate error message is meaningful
		errorMsg, ok := eventData["error"].(string)
		Expect(ok).To(BeTrue(), "Error should be a string")
		Expect(errorMsg).ToNot(BeEmpty(), "Error message should not be empty")
		GinkgoWriter.Printf("✅ Failed delivery error captured in audit: %s\n", errorMsg)

		// ===== STEP 4.5: FIELD MATCHING VALIDATION =====
		By("Validating stored fields match audit helper output")

		// Verify notification_id in event_data matches resource_id
		Expect(eventData["notification_id"]).To(Equal(notificationName),
			"FIELD MATCH: event_data.notification_id should match resource_id")

		// Verify channel in event_data is correct
		Expect(eventData["channel"]).To(Equal("email"),
			"FIELD MATCH: event_data.channel should be 'email'")

		// Verify subject and body are preserved in event_data
		Expect(eventData).To(HaveKey("subject"),
			"FIELD MATCH: event_data should contain subject")
		Expect(eventData["subject"]).To(Equal("E2E Failed Delivery Audit Test"),
			"FIELD MATCH: event_data.subject should match notification spec")

		Expect(eventData).To(HaveKey("body"),
			"FIELD MATCH: event_data should contain body")
		Expect(eventData["body"]).To(Equal("Testing failed delivery audit event persistence"),
			"FIELD MATCH: event_data.body should match notification spec")

		// Verify priority is preserved
		Expect(eventData).To(HaveKey("priority"),
			"FIELD MATCH: event_data should contain priority")
		Expect(eventData["priority"]).To(Equal("critical"),
			"FIELD MATCH: event_data.priority should match notification spec")

		// Verify metadata is preserved
		Expect(eventData).To(HaveKey("metadata"),
			"FIELD MATCH: event_data should contain metadata")
		metadata, ok := eventData["metadata"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "Metadata should be a map")
		Expect(metadata).To(HaveKey("remediationRequestName"),
			"FIELD MATCH: metadata should contain remediationRequestName")
		Expect(metadata["remediationRequestName"]).To(Equal(correlationID),
			"FIELD MATCH: metadata.remediationRequestName should match correlation_id")

		GinkgoWriter.Printf("✅ Field matching validation complete: All stored fields match audit helper output\n")

		// ===== STEP 5: Verify correlation enables workflow tracing =====
		By("Verifying correlation_id enables workflow tracing")

		// Query all events for this correlation_id (should have at least the failed event)
		allEvents := queryAuditEvents(dsClient, correlationID)
		Expect(allEvents).ToNot(BeEmpty(),
			"Should be able to trace all events by correlation_id")

		for _, event := range allEvents {
			Expect(event.CorrelationID).To(Equal(correlationID),
				"All events should share same correlation_id for workflow tracing")
		}

		GinkgoWriter.Printf("✅ Full failed delivery audit chain validated:\n")
		GinkgoWriter.Printf("   Controller → auditMessageFailed() → BufferedStore → DataStorage → PostgreSQL\n")
		GinkgoWriter.Printf("   Event count: %d, Failed events: 1, Correlation ID: %s\n",
			len(allEvents), correlationID)
	})

	// ========================================
	// Additional Test: Multi-Channel Partial Failure
	// ========================================
	// This test validates audit events when SOME channels succeed and SOME fail
	It("should emit separate audit events for each channel (success + failure)", func() {
		Expect(dataStorageNodePort).ToNot(Equal(0),
			"REQUIRED: Data Storage not available")

		// Create unique test identifiers
		testID := time.Now().Format("20060102-150405")
		notificationName = "e2e-partial-failure-" + testID
		correlationID = "e2e-partial-" + testID

		// Create notification with TWO channels:
		// - Console (will SUCCEED - always available in E2E)
		// - Email (will FAIL - not configured)
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationName,
				Namespace: "default",
				Labels: map[string]string{
					"test-type": "partial-failure-audit",
				},
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				// FIX: Set RemediationRequestRef to enable correlation_id matching in audit queries
				RemediationRequestRef: &corev1.ObjectReference{
					APIVersion: "kubernaut.ai/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       correlationID,
					Namespace:  "default",
				},
				Type:     notificationv1alpha1.NotificationTypeSimple,
				Priority: notificationv1alpha1.NotificationPriorityCritical,
				Subject:  "E2E Partial Failure Audit Test",
				Body:     "Testing audit events for partial delivery failures",
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelConsole, // SUCCEEDS
					notificationv1alpha1.ChannelEmail,   // FAILS
				},
				Recipients: []notificationv1alpha1.Recipient{
					{Email: "test@example.com"},
				},
				Metadata: map[string]string{
					"remediationRequestName": correlationID,
					"test-scenario":          "partial-failure",
				},
			},
		}

		By("Creating NotificationRequest with Console + Email channels")
		err := k8sClient.Create(testCtx, notification)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for controller to process both channels")
		Eventually(func() int {
			var n notificationv1alpha1.NotificationRequest
			if err := k8sClient.Get(testCtx, types.NamespacedName{
				Name:      notificationName,
				Namespace: "default",
			}, &n); err != nil {
				return 0
			}
			// Wait for both channels to be attempted
			return len(n.Status.DeliveryAttempts)
		}, 30*time.Second, 1*time.Second).Should(BeNumerically(">=", 2),
			"Controller should attempt delivery for both channels")

		By("Verifying BOTH success and failure audit events are persisted")

		// Should have 1 sent event (console) + 1 failed event (email)
		Eventually(func() bool {
			sentCount := queryAuditEventCount(dsClient, correlationID, string(ogenclient.NotificationMessageSentPayloadAuditEventEventData))
			failedCount := queryAuditEventCount(dsClient, correlationID, string(ogenclient.NotificationMessageFailedPayloadAuditEventEventData))
			GinkgoWriter.Printf("DEBUG: Partial failure - sent=%d, failed=%d\n", sentCount, failedCount)
			return sentCount >= 1 && failedCount >= 1
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"Should have both success (console) and failure (email) audit events")

		By("Verifying each event has correct channel in event_data")
		allEvents := queryAuditEvents(dsClient, correlationID)
		Expect(allEvents).To(HaveLen(2), "Should have exactly 2 events (1 success, 1 failure)")

		// Validate channel-specific event_data and field matching
		var sentEvent, failedEvent *ogenclient.AuditEvent
		for i := range allEvents {
			if allEvents[i].EventType == string(ogenclient.NotificationMessageSentPayloadAuditEventEventData) {
				sentEvent = &allEvents[i]
			} else if allEvents[i].EventType == string(ogenclient.NotificationMessageFailedPayloadAuditEventEventData) {
				failedEvent = &allEvents[i]
			}
		}

		Expect(sentEvent).ToNot(BeNil(), "Should have sent event")
		Expect(failedEvent).ToNot(BeNil(), "Should have failed event")

		// FIELD MATCHING VALIDATION: Success event
		By("Validating success event fields match audit helper output")
		var sentEventData map[string]interface{}
		sentEventDataBytes, err := json.Marshal(sentEvent.EventData)
		Expect(err).ToNot(HaveOccurred())
		err = json.Unmarshal(sentEventDataBytes, &sentEventData)
		Expect(err).ToNot(HaveOccurred())

		Expect(sentEventData["channel"]).To(Equal("console"),
			"FIELD MATCH: Success event channel should be console")
		Expect(sentEventData["notification_id"]).To(Equal(notificationName),
			"FIELD MATCH: Success event notification_id should match resource_id")
		Expect(sentEventData["subject"]).To(Equal("E2E Partial Failure Audit Test"),
			"FIELD MATCH: Success event subject should match notification spec")
		Expect(sentEventData["priority"]).To(Equal("critical"),
			"FIELD MATCH: Success event priority should match notification spec")
		Expect(string(sentEvent.EventOutcome)).To(Equal("success"),
			"FIELD MATCH: Success event outcome should be success")
		Expect(sentEvent.ResourceID.IsSet()).To(BeTrue(), "Resource ID should be set")
		Expect(sentEvent.ResourceID.Value).To(Equal(notificationName),
			"FIELD MATCH: Success event resource_id should match notification name")

		// FIELD MATCHING VALIDATION: Failed event
		By("Validating failed event fields match audit helper output")
		var failedEventData map[string]interface{}
		failedEventDataBytes, err := json.Marshal(failedEvent.EventData)
		Expect(err).ToNot(HaveOccurred())
		err = json.Unmarshal(failedEventDataBytes, &failedEventData)
		Expect(err).ToNot(HaveOccurred())

		Expect(failedEventData["channel"]).To(Equal("email"),
			"FIELD MATCH: Failed event channel should be email")
		Expect(failedEventData["notification_id"]).To(Equal(notificationName),
			"FIELD MATCH: Failed event notification_id should match resource_id")
		Expect(failedEventData["subject"]).To(Equal("E2E Partial Failure Audit Test"),
			"FIELD MATCH: Failed event subject should match notification spec")
		Expect(failedEventData["priority"]).To(Equal("critical"),
			"FIELD MATCH: Failed event priority should match notification spec")
		Expect(failedEventData).To(HaveKey("error"),
			"FIELD MATCH: Failed event should contain error details")
		Expect(string(failedEvent.EventOutcome)).To(Equal("failure"),
			"FIELD MATCH: Failed event outcome should be failure")
		Expect(failedEvent.ResourceID.IsSet()).To(BeTrue(), "Resource ID should be set")
		Expect(failedEvent.ResourceID.Value).To(Equal(notificationName),
			"FIELD MATCH: Failed event resource_id should match notification name")

		GinkgoWriter.Printf("✅ Partial failure audit validation complete:\n")
		GinkgoWriter.Printf("   Console channel: SUCCESS (audit: notification.message.sent)\n")
		GinkgoWriter.Printf("   Email channel: FAILURE (audit: notification.message.failed)\n")
	})
})
