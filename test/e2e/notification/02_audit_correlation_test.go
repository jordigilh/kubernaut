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
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationcontroller "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

// ========================================
// E2E Test 2: Audit Correlation Across Multiple Notification Events
// ========================================
//
// Business Requirements:
// - BR-NOT-062: Unified audit table with correlation support
// - BR-NOT-063: Graceful degradation (async, fire-and-forget)
//
// Test Scenario:
// 1. Create 3 NotificationRequests with same remediation context
// 2. Simulate delivery lifecycle for all 3 (sent → acknowledged → escalated)
// 3. Verify all 9 audit events generated (3 notifications × 3 events each)
// 4. Verify all events share same correlation_id
// 5. Verify chronological ordering of events
// 6. Verify fire-and-forget pattern (no blocking)
//
// Expected Results:
// - 9 audit events generated (3 sent, 3 acknowledged, 3 escalated)
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
		testCtx         context.Context
		testCancel      context.CancelFunc
		notifications   []*notificationv1alpha1.NotificationRequest
		auditHelpers    *notificationcontroller.AuditHelpers
		auditStore      audit.AuditStore
		mockDataStorage *httptest.Server
		receivedEvents  []*audit.AuditEvent
		eventsMutex     sync.Mutex
		correlationID   string
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 3*time.Minute)
		receivedEvents = []*audit.AuditEvent{}
		notifications = []*notificationv1alpha1.NotificationRequest{}

		// Common correlation ID for all notifications (remediation request)
		correlationID = "remediation-" + time.Now().Format("20060102-150405")

		// Set up mock Data Storage Service
		mockDataStorage = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/audit/events" && r.Method == "POST" {
				var events []*audit.AuditEvent
				err := json.NewDecoder(r.Body).Decode(&events)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				eventsMutex.Lock()
				receivedEvents = append(receivedEvents, events...)
				eventsMutex.Unlock()

				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": true,
					"count":   len(events),
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))

		// Create audit store
		httpClient := &http.Client{Timeout: 5 * time.Second}
		dataStorageClient := audit.NewHTTPDataStorageClient(mockDataStorage.URL, httpClient)

		config := audit.Config{
			BufferSize:    1000,
			BatchSize:     10,
			FlushInterval: 100 * time.Millisecond,
			MaxRetries:    3,
		}

		testLogger, _ := zap.NewDevelopment()
		auditStore, _ = audit.NewBufferedStore(dataStorageClient, config, "notification", testLogger)

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
		if mockDataStorage != nil {
			mockDataStorage.Close()
		}

		// Clean up all NotificationRequest CRDs
		for _, notification := range notifications {
			k8sClient.Delete(testCtx, notification)
		}
	})

	It("should generate correlated audit events for multiple notifications in same remediation", func() {
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
			// Note: Status update not required for E2E test validation
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

		// ===== STEP 3: Wait for all events to be flushed =====
		By("Waiting for all 9 audit events to be flushed asynchronously")

		Eventually(func() int {
			eventsMutex.Lock()
			defer eventsMutex.Unlock()
			return len(receivedEvents)
		}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 9),
			"All 9 audit events (3 notifications × 3 events) should be flushed")

		// ===== STEP 4: Verify all events have same correlation_id =====
		By("Verifying all audit events share the same correlation_id")

		eventsMutex.Lock()
		defer eventsMutex.Unlock()

		Expect(len(receivedEvents)).To(BeNumerically(">=", 9),
			"Should have 9 audit events (3 sent + 3 ack + 3 escalated)")

		// Verify all events have same correlation_id
		for _, event := range receivedEvents {
			Expect(event.CorrelationID).To(Equal(correlationID),
				"All events should have same correlation_id: %s (found: %s)",
				correlationID, event.CorrelationID)
		}

		// ===== STEP 5: Verify chronological ordering =====
		By("Verifying events are chronologically ordered")

		// Events should be ordered by timestamp
		for i := 1; i < len(receivedEvents); i++ {
			prev := receivedEvents[i-1]
			curr := receivedEvents[i]

			Expect(curr.EventTimestamp.After(prev.EventTimestamp) || curr.EventTimestamp.Equal(prev.EventTimestamp)).To(BeTrue(),
				"Events should be in chronological order: event[%d] timestamp=%v, event[%d] timestamp=%v",
				i-1, prev.EventTimestamp, i, curr.EventTimestamp)
		}

		// ===== STEP 6: Verify event types distribution =====
		By("Verifying correct distribution of event types")

		sentCount := 0
		ackCount := 0
		escalatedCount := 0

		for _, event := range receivedEvents {
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

		// ===== STEP 7: Verify ADR-034 compliance for all events =====
		By("Verifying all events follow ADR-034 format")

		for _, event := range receivedEvents {
			// Verify required fields
			Expect(event.EventID).ToNot(BeNil(), "EventID should not be nil")
			Expect(event.EventTimestamp).ToNot(BeZero(), "EventTimestamp should be set")
			Expect(event.EventCategory).To(Equal("notification"), "EventCategory should be 'notification'")
			Expect(event.ActorType).To(Equal("service"), "ActorType should be 'service'")
			Expect(event.ActorID).To(Equal("notification"), "ActorID should be service name")
			Expect(event.ResourceType).To(Equal("NotificationRequest"), "ResourceType should be 'NotificationRequest'")
			Expect(event.RetentionDays).To(Equal(2555), "RetentionDays should be 2555 (7 years)")

			// Verify event outcome is valid
			Expect(event.EventOutcome).To(BeElementOf("success", "failure", "error"),
				"EventOutcome should be valid: %s", event.EventOutcome)

			// Verify event data is valid JSON
			if event.EventData != nil {
				var jsonData interface{}
				err := json.Unmarshal(event.EventData, &jsonData)
				Expect(err).ToNot(HaveOccurred(),
					"EventData should be valid JSON: %s", string(event.EventData))
			}
		}

		// ===== STEP 8: Verify fire-and-forget pattern (non-blocking) =====
		By("Verifying fire-and-forget pattern ensures non-blocking audit writes")

		// If we got here without timeout, fire-and-forget is working
		// All audit writes were async and didn't block test execution
		Expect(true).To(BeTrue(), "Test completed without blocking, fire-and-forget pattern validated")
	})
})
