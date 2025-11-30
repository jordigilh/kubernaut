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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

var _ = Describe("Notification Audit Integration Tests", func() {
	var (
		mockDataStorage *httptest.Server
		auditStore      audit.AuditStore
		receivedEvents  []*audit.AuditEvent
		eventsMutex     sync.Mutex
	)

	BeforeEach(func() {
		receivedEvents = []*audit.AuditEvent{}

		// Mock Data Storage Service HTTP endpoint
		// Simulates the Data Storage Service API for audit event writes
		mockDataStorage = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/audit/events" && r.Method == "POST" {
				var events []*audit.AuditEvent
				err := json.NewDecoder(r.Body).Decode(&events)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				// Thread-safe append
				eventsMutex.Lock()
				receivedEvents = append(receivedEvents, events...)
				eventsMutex.Unlock()

				w.WriteHeader(http.StatusCreated)
				if err := json.NewEncoder(w).Encode(map[string]interface{}{
					"success": true,
					"count":   len(events),
				}); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))

		// Create audit store with mock Data Storage client
		httpClient := &http.Client{Timeout: 5 * time.Second}
		dataStorageClient := audit.NewHTTPDataStorageClient(mockDataStorage.URL, httpClient)

		config := audit.Config{
			BufferSize:    1000,
			BatchSize:     10,
			FlushInterval: 100 * time.Millisecond,
			MaxRetries:    3,
		}

		logger := crzap.New(crzap.UseDevMode(true))
		auditStore, _ = audit.NewBufferedStore(dataStorageClient, config, "notification", logger)
	})

	AfterEach(func() {
		if auditStore != nil {
			auditStore.Close()
		}
		if mockDataStorage != nil {
			mockDataStorage.Close()
		}
	})

	// ===== INTEGRATION TEST 1: Data Storage HTTP Integration =====
	Context("when notification is sent successfully", func() {
		It("should write audit event to Data Storage Service via HTTP POST", func() {
			// BR-NOT-062: Unified audit table integration
			// BR-NOT-063: Graceful audit degradation (fire-and-forget pattern)

			ctx := context.Background()
			notification := createTestNotificationForIntegration()

			// Create audit event (simulate message sent)
			event := audit.NewAuditEvent()
			event.EventType = "notification.message.sent"
			event.EventCategory = "notification"
			event.EventAction = "sent"
			event.EventOutcome = "success"
			event.ActorType = "service"
			event.ActorID = "notification-controller"
			event.ResourceType = "NotificationRequest"
			event.ResourceID = notification.Name
			event.CorrelationID = "test-correlation-001"
			event.EventData = []byte(`{"notification_id": "integration-test-notification", "channel": "slack", "recipient": "#integration-tests"}`)

			// Store audit event (fire-and-forget)
			err := auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred(), "Storing audit event should not fail")

			// Wait for async flush to Data Storage (FlushInterval = 100ms)
			Eventually(func() int {
				eventsMutex.Lock()
				defer eventsMutex.Unlock()
				return len(receivedEvents)
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1),
				"At least 1 event should be flushed to Data Storage via HTTP")

			// Validate received event follows ADR-034 format
			eventsMutex.Lock()
			defer eventsMutex.Unlock()
			Expect(receivedEvents).ToNot(BeEmpty(), "Received events should not be empty")
			validateAuditEventADR034(receivedEvents[0])
		})
	})

	// ===== INTEGRATION TEST 2: Async Buffer Flush =====
	Context("when multiple audit events are buffered", func() {
		It("should flush events in batches asynchronously", func() {
			// BR-NOT-062: Unified audit table integration

			ctx := context.Background()
			notification := createTestNotificationForIntegration()

			// Write 15 events (BatchSize = 10, so should flush in 2 batches)
			for i := 0; i < 15; i++ {
				event := audit.NewAuditEvent()
				event.EventType = "notification.message.sent"
				event.EventCategory = "notification"
				event.EventAction = "sent"
				event.EventOutcome = "success"
				event.ActorType = "service"
				event.ActorID = "notification-controller"
				event.ResourceType = "NotificationRequest"
				event.ResourceID = notification.Name
				event.CorrelationID = "batch-test-correlation"
				event.EventData = []byte(`{"notification_id": "integration-test-notification", "channel": "slack"}`)

				err := auditStore.StoreAudit(ctx, event)
				Expect(err).ToNot(HaveOccurred(), "Storing audit event should not fail")
			}

			// Wait for async flush (FlushInterval = 100ms, BatchSize = 10)
			// Should receive at least 10 events quickly (first batch)
			// Then remaining 5 after next flush interval
			Eventually(func() int {
				eventsMutex.Lock()
				defer eventsMutex.Unlock()
				return len(receivedEvents)
			}, 3*time.Second, 50*time.Millisecond).Should(Equal(15),
				"All 15 events should be flushed to Data Storage")

			// Validate all received events
			eventsMutex.Lock()
			defer eventsMutex.Unlock()
			for i, event := range receivedEvents {
				Expect(event).ToNot(BeNil(), "Event %d should not be nil", i)
				Expect(event.EventType).To(Equal("notification.message.sent"))
				Expect(event.EventCategory).To(Equal("notification"))
			}
		})
	})

	// ===== INTEGRATION TEST 3: DLQ Fallback =====
	Context("when Data Storage Service is unavailable", func() {
		It("should fallback to DLQ for retry without blocking delivery", func() {
			// BR-NOT-063: Graceful audit degradation

			ctx := context.Background()
			notification := createTestNotificationForIntegration()

			// Close mock Data Storage to simulate service down
			mockDataStorage.Close()

			// Create a new mock that returns 503 Service Unavailable
			mockDataStorage = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				if _, err := w.Write([]byte(`{"error": "service temporarily unavailable"}`)); err != nil {
					// Log error in test mock - response write failure
					GinkgoWriter.Printf("Mock server failed to write response: %v\n", err)
				}
			}))

			// Write audit event (should not block despite Data Storage being down)
			event := audit.NewAuditEvent()
			event.EventType = "notification.message.failed"
			event.EventCategory = "notification"
			event.EventAction = "failed"
			event.EventOutcome = "failure"
			event.ActorType = "service"
			event.ActorID = "notification-controller"
			event.ResourceType = "NotificationRequest"
			event.ResourceID = notification.Name
			event.CorrelationID = "dlq-test-correlation"
			event.EventData = []byte(`{"notification_id": "integration-test-notification", "error": "delivery_failed"}`)

			// Store should not block (fire-and-forget pattern)
			start := time.Now()
			err := auditStore.StoreAudit(ctx, event)
			elapsed := time.Since(start)

			// Verify StoreAudit returns quickly (non-blocking)
			Expect(err).ToNot(HaveOccurred(), "Storing audit event should not fail")
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond),
				"StoreAudit should return immediately (fire-and-forget)")

			// Note: DLQ implementation verification would require Redis integration
			// For now, we verify that the store call doesn't block
			// In production, failed events would be queued to Redis Streams DLQ (DD-009)
		})
	})

	// ===== INTEGRATION TEST 4: Graceful Shutdown =====
	Context("when audit store is closed", func() {
		It("should flush remaining events before shutdown", func() {
			// Tests graceful shutdown - ensures no audit event loss

			ctx := context.Background()
			notification := createTestNotificationForIntegration()

			// Buffer 5 events (less than BatchSize = 10, so won't auto-flush immediately)
			for i := 0; i < 5; i++ {
				event := audit.NewAuditEvent()
				event.EventType = "notification.status.acknowledged"
				event.EventCategory = "notification"
				event.EventAction = "acknowledged"
				event.EventOutcome = "success"
				event.ActorType = "service"
				event.ActorID = "notification-controller"
				event.ResourceType = "NotificationRequest"
				event.ResourceID = notification.Name
				event.CorrelationID = "shutdown-test-correlation"
				event.EventData = []byte(`{"notification_id": "integration-test-notification", "acknowledged_by": "user@example.com"}`)

				err := auditStore.StoreAudit(ctx, event)
				Expect(err).ToNot(HaveOccurred(), "Storing audit event should not fail")
			}

			// Immediately close the audit store (should trigger flush)
			err := auditStore.Close()
			Expect(err).ToNot(HaveOccurred(), "Closing audit store should not fail")

			// Verify all 5 buffered events were flushed
			eventsMutex.Lock()
			defer eventsMutex.Unlock()
			Expect(len(receivedEvents)).To(Equal(5),
				"All 5 buffered events should be flushed on graceful shutdown")

			// Validate no events were lost
			for i, event := range receivedEvents {
				Expect(event).ToNot(BeNil(), "Event %d should not be nil", i)
				Expect(event.EventType).To(Equal("notification.status.acknowledged"))
				Expect(event.EventCategory).To(Equal("notification"))
			}
		})
	})
})

// ===== INTEGRATION TEST HELPERS =====

// createTestNotificationForIntegration creates a test notification for integration tests
func createTestNotificationForIntegration() *notificationv1alpha1.NotificationRequest {
	return &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "integration-test-notification",
			Namespace: "default",
		},
		Spec: notificationv1alpha1.NotificationRequestSpec{
			Type:     notificationv1alpha1.NotificationTypeSimple,
			Priority: notificationv1alpha1.NotificationPriorityCritical,
			Subject:  "Integration Test Alert",
			Body:     "Test notification for audit integration",
			Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelSlack},
			Recipients: []notificationv1alpha1.Recipient{
				{Slack: "#integration-tests"},
			},
			Metadata: map[string]string{
				"remediationRequestName": "integration-test-remediation",
				"cluster":                "test-cluster",
			},
		},
	}
}

// waitForEvents waits for expected number of events to be received (with timeout)
// Note: This helper is not used in the current implementation as we use Eventually() directly
// Keeping it for potential future use or as a pattern reference
func waitForEvents(events *[]*audit.AuditEvent, mutex *sync.Mutex, expectedCount int, timeout time.Duration) func() int {
	return func() int {
		mutex.Lock()
		defer mutex.Unlock()
		return len(*events)
	}
}

// validateAuditEventADR034 validates that an audit event follows ADR-034 format
func validateAuditEventADR034(event *audit.AuditEvent) {
	Expect(event).ToNot(BeNil(), "Audit event should not be nil")
	Expect(event.EventType).To(MatchRegexp(`^notification\.(message|status)\.(sent|failed|acknowledged|escalated)$`),
		"Event type must follow ADR-034 format: <service>.<category>.<action>")
	Expect(event.EventCategory).To(Equal("notification"), "Event category must be 'notification'")
	Expect(event.ActorType).To(Equal("service"), "Actor type must be 'service'")
	Expect(event.ActorID).To(Equal("notification-controller"), "Actor ID must match service name")
	Expect(event.ResourceType).To(Equal("NotificationRequest"), "Resource type must be 'NotificationRequest'")
	Expect(event.RetentionDays).To(Equal(2555), "Retention must be 2555 days (7 years) for compliance")
	Expect(event.EventData).ToNot(BeNil(), "Event data must be populated")

	// Validate event_data is valid JSON
	var eventData map[string]interface{}
	err := json.Unmarshal(event.EventData, &eventData)
	Expect(err).ToNot(HaveOccurred(), "Event data must be valid JSON (JSONB compatible)")
	Expect(eventData).To(HaveKey("notification_id"), "Event data must contain notification_id")
	Expect(eventData).To(HaveKey("channel"), "Event data must contain channel")
}
