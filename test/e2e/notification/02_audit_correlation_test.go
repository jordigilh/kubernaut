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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// E2E Test 2: Controller Audit Correlation Across Multiple Notifications (CORRECT PATTERN)
// ========================================
//
// Business Requirements:
// - BR-NOT-062: Unified audit table with correlation support
// - BR-NOT-063: Graceful degradation (async, fire-and-forget)
// - BR-NOT-064: Audit event correlation
//
// ✅ CORRECT PATTERN (Per TESTING_GUIDELINES.md lines 1688-1948):
// This test validates controller BUSINESS LOGIC with audit correlation as side effect:
// 1. Create 3 NotificationRequests with same remediation context (trigger business operations)
// 2. Wait for controller to process all 3 notifications (business logic)
// 3. Verify controller emitted correlated audit events (side effect validation)
//
// ❌ ANTI-PATTERN AVOIDED:
// - NOT manually creating audit events for lifecycle (tests infrastructure)
// - NOT directly calling auditStore.StoreAudit() (tests client library)
// - NOT using test-specific actor_id (tests wrong code path)
//
// Test Scenario:
// 1. Create 3 NotificationRequests with same remediation context
// 2. Wait for controller to process all 3 notifications (phase → Sent)
// 3. Verify controller emitted 3 string(ogenclient.NotificationMessageSentPayloadAuditEventEventData) audit events
// 4. Verify all events share same correlation_id (remediation request name)
// 5. Verify all events follow ADR-034 format
//
// Expected Results:
// - 3 NotificationRequest CRDs created successfully
// - Controller processes all 3 notifications and updates phases
// - Controller emits 3 audit events with actor_id="notification-controller"
// - All events have same correlation_id (remediation request name)
// - Audit events persisted to PostgreSQL via Data Storage API
// - All events follow ADR-034 format
//
// Business Value:
// This test validates the critical ability to trace controller audit events
// across multiple notification attempts using correlation_id, which is essential
// for compliance auditing and post-incident analysis.

var _ = Describe("E2E Test 2: Audit Correlation Across Multiple Notifications", Label("e2e", "correlation", "audit", "compliance"), func() {
	var (
		testCtx        context.Context
		testCancel     context.CancelFunc
		notifications  []*notificationv1alpha1.NotificationRequest
		dsClient       *ogenclient.Client
		dataStorageURL string
		correlationID  string
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 3*time.Minute)
		notifications = []*notificationv1alpha1.NotificationRequest{}

		// Common correlation ID for all notifications (remediation request)
		correlationID = "remediation-" + time.Now().Format("20060102-150405")

		// Use real Data Storage URL from Kind cluster
		dataStorageURL = fmt.Sprintf("http://localhost:%d", dataStorageNodePort)

		// ✅ DD-API-001: Create OpenAPI client for audit queries (MANDATORY)
		var err error
		dsClient, err = ogenclient.NewClient(dataStorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")

		// Create 3 NotificationRequests with same remediation context
		for i := 1; i <= 3; i++ {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      correlationID + "-notification-" + string(rune('0'+i)),
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriority([]string{"low", "medium", "high"}[i-1]),
					Subject:  "E2E Correlation Test - Notification " + string(rune('0'+i)),
					Body:     "Testing audit correlation across multiple notifications",
					Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}, // Use Console to avoid delivery failures
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#e2e-tests"}, // Keep for CRD validation, but Console channel doesn't use it
					},
					Metadata: map[string]string{
						"remediationRequestName": correlationID,
						"cluster":                "test-cluster",
						"attemptNumber":          string(rune('0' + i)),
					},
				},
			}
			notifications = append(notifications, notification)
		}
	})

	AfterEach(func() {
		testCancel()

		// Clean up all NotificationRequest CRDs
		for _, notification := range notifications {
			_ = k8sClient.Delete(testCtx, notification)
		}
	})

	It("should generate correlated audit events persisted to PostgreSQL", func() {
		// Per TESTING_GUIDELINES.md: E2E tests MUST use real services, Skip() is FORBIDDEN
		// If Data Storage is unavailable, test MUST fail with clear error
		Expect(dataStorageNodePort).ToNot(Equal(0),
			"REQUIRED: Data Storage not available\n"+
				"  Per TESTING_GUIDELINES.md: E2E tests MUST use real services\n"+
				"  Per DD-AUDIT-003: Audit infrastructure is MANDATORY\n"+
				"  Audit infrastructure should be deployed in SynchronizedBeforeSuite")

		// ===== STEP 1: Create all NotificationRequest CRDs =====
		By("Creating 3 NotificationRequests with same remediation context")

		for _, notification := range notifications {
			err := k8sClient.Create(testCtx, notification)
			Expect(err).ToNot(HaveOccurred(),
				"NotificationRequest CRD creation should succeed: %s", notification.Name)
		}

		// ===== STEP 2: Wait for controller to process all notifications =====
		// ✅ CORRECT PATTERN: Test controller behavior, NOT audit infrastructure
		By("Waiting for controller to process all 3 notifications")

		for _, notification := range notifications {
			Eventually(func() notificationv1alpha1.NotificationPhase {
				var updated notificationv1alpha1.NotificationRequest
				err := k8sClient.Get(testCtx, types.NamespacedName{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, &updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 30*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Controller should process notification %s and update phase to Sent", notification.Name)
		}

		// ===== STEP 3: Wait for controller to emit audit events (side effect) =====
		// ✅ CORRECT PATTERN: Verify audit as SIDE EFFECT of business operation
		By("Waiting for controller to emit audit events for all processed notifications")

		Eventually(func() int {
			resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				EventType:     ogenclient.NewOptString(string(ogenclient.NotificationMessageSentPayloadAuditEventEventData)),
				EventCategory: ogenclient.NewOptString("notification"),
				CorrelationID: ogenclient.NewOptString(correlationID),
			})
			if err != nil || resp.Data == nil {
				return 0
			}
			// Filter by controller actor_id after retrieving events
			events := resp.Data
			controllerEvents := filterEventsByActorId(events, "notification-controller")
			return len(controllerEvents)
		}, 15*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 3),
			"Controller should emit audit events for all 3 processed notifications")

		// ===== STEP 4: Verify all events queryable by correlation_id =====
		By("Verifying all controller-emitted audit events queryable by correlation_id")

		allEvents := queryAuditEvents(dsClient, correlationID)

		// Filter to only controller-emitted events (ActorId "notification-controller")
		events := filterEventsByActorId(allEvents, "notification-controller")
		Expect(events).To(HaveLen(6),
			"Should have exactly 6 controller-emitted audit events with same correlation_id:\n"+
				"  - 3 'sent' events (1 per notification/channel from delivery orchestrator)\n"+
				"  - 3 'acknowledged' events (1 per notification completion from transitionToSent)\n"+
				"  Bug NT-BUG-008 fix: Generation check prevents 12 events (would be 2x reconciles × 6 = 12 without fix)")

		// Verify all events have same correlation_id
		for _, event := range events {
			Expect(event.CorrelationID).To(Equal(correlationID),
				"All events should have same correlation_id: %s (found: %s)",
				correlationID, event.CorrelationID)
		}

		// ===== STEP 5: Verify event types distribution and per-notification uniqueness =====
		By("Verifying event types: 3 'sent' + 3 'acknowledged' (1 of each per notification)")

		// Track events by notification_id in a single pass
		// Note: `events` already filtered by correlation_id + actor_id, safe for parallel execution
		type notificationEventCount struct {
			sentCount         int
			acknowledgedCount int
		}
		notificationEvents := make(map[string]*notificationEventCount)

	for _, event := range events {
		// Extract notification_id from EventData discriminated union
		// ogen: Different event types use different payload fields
		// - notification.message.sent → NotificationMessageSentPayload.NotificationID (string)
		// - notification.message.acknowledged → NotificationMessageAcknowledgedPayload.NotificationID (string)
		notificationID := ""
		switch event.EventType {
		case string(ogenclient.NotificationMessageSentPayloadAuditEventEventData):
			notificationID = event.EventData.NotificationMessageSentPayload.NotificationID
		case "notification.message.acknowledged":
			notificationID = event.EventData.NotificationMessageAcknowledgedPayload.NotificationID
		default:
			Fail(fmt.Sprintf("Unexpected event type: %s (should be 'notification.message.sent' or 'notification.message.acknowledged')", event.EventType))
		}

		Expect(notificationID).ToNot(BeEmpty(),
			"Event should have notification_id in EventData (got EventType: %s)", event.EventType)

		// Initialize struct for this notification if needed
		if notificationEvents[notificationID] == nil {
			notificationEvents[notificationID] = &notificationEventCount{}
		}

		// Increment count for this event type
		switch event.EventType {
		case string(ogenclient.NotificationMessageSentPayloadAuditEventEventData):
			notificationEvents[notificationID].sentCount++
		case "notification.message.acknowledged":
			notificationEvents[notificationID].acknowledgedCount++
		}
	}

		// Verify we have exactly 3 notifications
		Expect(notificationEvents).To(HaveLen(3),
			"Should have events for exactly 3 distinct notifications")

		// Verify each notification has exactly 1 sent + 1 acknowledged event
		totalSent := 0
		totalAcknowledged := 0
		for notificationID, counts := range notificationEvents {
			Expect(counts.sentCount).To(Equal(1),
				"Notification %s should have exactly 1 'sent' event", notificationID)

			Expect(counts.acknowledgedCount).To(Equal(1),
				"Notification %s should have exactly 1 'acknowledged' event", notificationID)

			totalSent += counts.sentCount
			totalAcknowledged += counts.acknowledgedCount

			GinkgoWriter.Printf("✅ Notification %s: 1 sent + 1 acknowledged event (correct)\n", notificationID)
		}

		// Final sanity check: totals should be 3 + 3 = 6
		Expect(totalSent).To(Equal(3), "Total 'sent' events across all notifications should be 3")
		Expect(totalAcknowledged).To(Equal(3), "Total 'acknowledged' events across all notifications should be 3")

		// ===== STEP 6: Verify ADR-034 compliance for all events =====
		By("Verifying all controller-emitted events follow ADR-034 format")

		for _, event := range events {
			// Verify required fields
			Expect(event.EventTimestamp).ToNot(BeZero(), "EventTimestamp should be set")
			Expect(string(event.EventCategory)).To(Equal("notification"), "EventCategory should be 'notification'")
			Expect(event.ActorType).ToNot(BeNil(), "ActorType should be set")
			Expect(event.ActorType.Value).To(Equal("service"), "ActorType should be 'service'")
			Expect(event.ActorID).ToNot(BeNil(), "ActorID should be set")
			Expect(event.ActorID.Value).To(Equal("notification-controller"), "ActorID should be 'notification-controller'")
			Expect(event.ResourceType).ToNot(BeNil(), "ResourceType should be set")
			Expect(event.ResourceType.Value).To(Equal("NotificationRequest"), "ResourceType should be 'NotificationRequest'")
			// Note: RetentionDays is stored in PostgreSQL but not returned by Data Storage Query API

			// Verify event outcome is valid
			Expect(string(event.EventOutcome)).To(BeElementOf("success", "failure", "error"),
				"EventOutcome should be valid: %s", event.EventOutcome)

		// Verify event data is valid JSON (marshal from discriminated union)
		if event.EventData.Type != "" {
			eventDataBytes, err := json.Marshal(event.EventData)
			Expect(err).ToNot(HaveOccurred(), "EventData should be marshallable")
			var jsonData interface{}
			err = json.Unmarshal(eventDataBytes, &jsonData)
			Expect(err).ToNot(HaveOccurred(),
				"EventData should be valid JSON: %s", string(eventDataBytes))
		}
		}

		// ===== STEP 7: Verify controller audit integration =====
		By("Verifying controller audit integration for all notifications")

		// If we got here without timeout, controller is properly emitting audits
		// All controller-emitted audit events were persisted successfully
		GinkgoWriter.Printf("✅ Audit correlation validated: %d controller-emitted events with correlation_id=%s\n", len(events), correlationID)
		GinkgoWriter.Printf("✅ Controller emits audits for all %d processed notifications\n", len(notifications))
	})
})
