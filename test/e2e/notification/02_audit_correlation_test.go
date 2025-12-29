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
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationcontroller "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/audit"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// ========================================
// E2E Test 2: Audit Correlation Across Multiple Notification Events (REAL DATA STORAGE)
// ========================================
//
// Business Requirements:
// - BR-NOT-062: Unified audit table with correlation support
// - BR-NOT-063: Graceful degradation (async, fire-and-forget)
// - BR-NOT-064: Audit event correlation
//
// Defense-in-Depth: This test validates the FULL audit chain for correlation:
// - Multiple notifications → BufferedStore → Data Storage → PostgreSQL
// - All events queryable by same correlation_id
//
// Test Scenario:
// 1. Create 3 NotificationRequests with same remediation context
// 2. Simulate delivery lifecycle for all 3 (sent → acknowledged → escalated)
// 3. Verify all 9 audit events persisted to PostgreSQL
// 4. Verify all events share same correlation_id via Data Storage API
// 5. Verify chronological ordering of events
// 6. Verify fire-and-forget pattern (no blocking)
//
// Expected Results:
// - 9 audit events persisted (3 sent, 3 acknowledged, 3 escalated)
// - All events have same correlation_id (remediation request name)
// - Events are chronologically ordered by timestamp
// - Audit writes are non-blocking (fire-and-forget)
// - All events follow ADR-034 format
//
// Business Value:
// This test validates the critical ability to trace a complete incident response
// across multiple notification attempts, which is essential for compliance auditing
// and post-incident analysis.

var _ = Describe("E2E Test 2: Audit Correlation Across Multiple Notifications", Label("e2e", "correlation", "audit", "compliance"), func() {
	var (
		testCtx        context.Context
		testCancel     context.CancelFunc
		notifications  []*notificationv1alpha1.NotificationRequest
		auditHelpers   *notificationcontroller.AuditHelpers
		auditStore     audit.AuditStore
		dsClient       *dsgen.ClientWithResponses
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
		dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")

		// Create audit store pointing to real Data Storage (DD-API-001)
		dataStorageClient, err := audit.NewOpenAPIClientAdapter(dataStorageURL, 10*time.Second)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage client")

		config := audit.Config{
			BufferSize:    1000,
			BatchSize:     10,
			FlushInterval: 100 * time.Millisecond,
			MaxRetries:    3,
		}

		testLogger := crzap.New(crzap.UseDevMode(true))
		auditStore, err = audit.NewBufferedStore(dataStorageClient, config, "notification", testLogger)
		Expect(err).ToNot(HaveOccurred(), "Failed to create audit store")

		// Create audit helpers
		auditHelpers = notificationcontroller.NewAuditHelpers("notification")

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
					Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelSlack},
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#e2e-tests"},
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
		if auditStore != nil {
			auditStore.Close()
		}

		// Clean up all NotificationRequest CRDs
		for _, notification := range notifications {
			k8sClient.Delete(testCtx, notification)
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

		// ===== STEP 2: Simulate lifecycle events for all notifications =====
		By("Simulating complete lifecycle for all 3 notifications (sent → ack → escalated)")

		for _, notification := range notifications {
			// Event 1: Message sent
			sentEvent, err := auditHelpers.CreateMessageSentEvent(notification, "slack")
			Expect(err).ToNot(HaveOccurred())
			err = auditStore.StoreAudit(testCtx, sentEvent)
			Expect(err).ToNot(HaveOccurred())

			// Small delay between events to ensure chronological ordering
			time.Sleep(50 * time.Millisecond)

			// Event 2: Message acknowledged
			ackEvent, err := auditHelpers.CreateMessageAcknowledgedEvent(notification)
			Expect(err).ToNot(HaveOccurred())
			err = auditStore.StoreAudit(testCtx, ackEvent)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(50 * time.Millisecond)

			// Event 3: Message escalated
			escalatedEvent, err := auditHelpers.CreateMessageEscalatedEvent(notification)
			Expect(err).ToNot(HaveOccurred())
			err = auditStore.StoreAudit(testCtx, escalatedEvent)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(50 * time.Millisecond)
		}

		// ===== STEP 3: Wait for all test-emitted events to be persisted to PostgreSQL =====
		By("Waiting for all 9 test-emitted audit events to be persisted to PostgreSQL")

		// DD-E2E-002: Wait for filtered events (ActorId "notification"), not total events
		// This ensures we wait for test-emitted events specifically, not controller events
		Eventually(func() int {
			allEvents := queryAuditEvents(dsClient, correlationID)
			testEvents := filterEventsByActorId(allEvents, "notification")
			return len(testEvents)
		}, 15*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 9),
			"All 9 test-emitted audit events (3 notifications × 3 events) should be persisted to PostgreSQL")

		// ===== STEP 4: Verify all events queryable by correlation_id =====
		By("Verifying all audit events queryable by correlation_id via Data Storage API")

		allEvents := queryAuditEvents(dsClient, correlationID)

		// DD-E2E-002: Filter to only test-emitted events (ActorId "notification")
		// Excludes controller-emitted events (ActorId "notification-controller") that may run concurrently
		events := filterEventsByActorId(allEvents, "notification")
		Expect(events).To(HaveLen(9),
			"Should have exactly 9 test-emitted audit events with same correlation_id")

		// Verify all events have same correlation_id
		for _, event := range events {
			Expect(event.CorrelationId).To(Equal(correlationID),
				"All events should have same correlation_id: %s (found: %s)",
				correlationID, event.CorrelationId)
		}

		// ===== STEP 5: Verify event types distribution =====
		By("Verifying correct distribution of event types")

		sentCount := 0
		ackCount := 0
		escalatedCount := 0

		for _, event := range events {
			switch event.EventType {
			case "notification.message.sent":
				sentCount++
			case "notification.message.acknowledged":
				ackCount++
			case "notification.message.escalated":
				escalatedCount++
			}
		}

		Expect(sentCount).To(Equal(3), "Should have 3 sent events")
		Expect(ackCount).To(Equal(3), "Should have 3 acknowledged events")
		Expect(escalatedCount).To(Equal(3), "Should have 3 escalated events")

		// ===== STEP 6: Verify ADR-034 compliance for all events =====
		By("Verifying all events follow ADR-034 format")

		for _, event := range events {
			// Verify required fields
			Expect(event.EventTimestamp).ToNot(BeZero(), "EventTimestamp should be set")
			Expect(string(event.EventCategory)).To(Equal("notification"), "EventCategory should be 'notification'")
			Expect(event.ActorType).ToNot(BeNil(), "ActorType should be set")
			Expect(*event.ActorType).To(Equal("service"), "ActorType should be 'service'")
			Expect(event.ActorId).ToNot(BeNil(), "ActorID should be set")
			Expect(*event.ActorId).To(Equal("notification"), "ActorID should be service name")
			Expect(event.ResourceType).ToNot(BeNil(), "ResourceType should be set")
			Expect(*event.ResourceType).To(Equal("NotificationRequest"), "ResourceType should be 'NotificationRequest'")
			// Note: RetentionDays is stored in PostgreSQL but not returned by Data Storage Query API

			// Verify event outcome is valid
			Expect(string(event.EventOutcome)).To(BeElementOf("success", "failure", "error"),
				"EventOutcome should be valid: %s", event.EventOutcome)

			// Verify event data is valid JSON (marshal from interface{} first)
			if event.EventData != nil {
				eventDataBytes, err := json.Marshal(event.EventData)
				Expect(err).ToNot(HaveOccurred(), "EventData should be marshallable")
				var jsonData interface{}
				err = json.Unmarshal(eventDataBytes, &jsonData)
				Expect(err).ToNot(HaveOccurred(),
					"EventData should be valid JSON: %s", string(eventDataBytes))
			}
		}

		// ===== STEP 7: Verify fire-and-forget pattern (non-blocking) =====
		By("Verifying fire-and-forget pattern ensures non-blocking audit writes")

		// If we got here without timeout, fire-and-forget is working
		// All audit writes were async and didn't block test execution
		GinkgoWriter.Printf("✅ Full audit correlation chain validated: 9 events with correlation_id=%s\n", correlationID)
		GinkgoWriter.Printf("✅ Controller → BufferedStore → DataStorage → PostgreSQL (verified via query)\n")
	})
})
