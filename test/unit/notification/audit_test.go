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
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationctrl "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

func TestAuditHelpers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notification Audit Helpers Suite")
}

var _ = Describe("Audit Helpers", func() {
	var (
		helpers      *notificationctrl.AuditHelpers
		notification *notificationv1alpha1.NotificationRequest
	)

	BeforeEach(func() {
		helpers = notificationctrl.NewAuditHelpers("notification-controller")

		// Create test notification fixture
		notification = createTestNotification()
	})

	// ===== TDD CYCLE 1: CreateMessageSentEvent =====
	// TDD Cycle 1: Write ONE test, verify it FAILS, then STOP
	Context("CreateMessageSentEvent", func() {
		It("should create notification.message.sent audit event with accurate fields", func() {
			// BR-NOT-062: Unified audit table integration
			// BR-NOT-064: Audit event correlation

			// ===== BEHAVIOR TESTING ===== (Gap 2 Fix)
			// Question: Does CreateMessageSentEvent() work without errors?
			event, err := helpers.CreateMessageSentEvent(notification, "slack")
			Expect(err).ToNot(HaveOccurred(), "Event creation should not error")
			Expect(event).ToNot(BeNil(), "Event should be created")

			// ===== CORRECTNESS TESTING ===== (Gap 2 Fix)
			// Question: Are the event fields ACCURATE per ADR-034?

			// Correctness: Event type follows ADR-034 format <service>.<category>.<action>
			Expect(event.EventType).To(Equal("notification.message.sent"),
				"Event type MUST be 'notification.message.sent' (ADR-034 format)")

			// Correctness: Event category identifies service domain
			Expect(event.EventCategory).To(Equal("notification"),
				"Event category MUST be 'notification' for all notification events")

			// Correctness: Event action describes operation
			Expect(event.EventAction).To(Equal("sent"),
				"Event action MUST be 'sent' for message delivery")

			// Correctness: Event outcome indicates success
			Expect(event.EventOutcome).To(Equal("success"),
				"Event outcome MUST be 'success' for successful delivery")

			// Correctness: Actor correctly identifies source service
			Expect(event.ActorType).To(Equal("service"),
				"Actor type MUST be 'service' (not 'user' or 'external')")
			Expect(event.ActorID).To(Equal("notification-controller"),
				"Actor ID MUST match service name for traceability")

			// Correctness: Resource fields identify the notification CRD
			Expect(event.ResourceType).To(Equal("NotificationRequest"),
				"Resource type MUST be 'NotificationRequest' (CRD kind)")
			Expect(event.ResourceID).To(Equal("test-notification"),
				"Resource ID MUST match notification CRD name for correlation")

			// Correctness: Correlation ID enables end-to-end tracing (BR-NOT-064)
			Expect(event.CorrelationID).To(Equal("remediation-123"),
				"Correlation ID MUST match remediation_id for workflow tracing")

			// Correctness: Namespace populated for Kubernetes context
			Expect(event.Namespace).ToNot(BeNil(),
				"Namespace MUST be populated for Kubernetes context")
			Expect(*event.Namespace).To(Equal("default"),
				"Namespace MUST match notification CRD namespace")

			// Correctness: Retention period meets compliance requirements
			Expect(event.RetentionDays).To(Equal(2555),
				"Retention MUST be 2555 days (7 years) for SOC 2 / ISO 27001 compliance")

			// Correctness: Event data is valid JSON with required notification fields
			var eventData map[string]interface{}
			err = json.Unmarshal(event.EventData, &eventData)
			Expect(err).ToNot(HaveOccurred(), "Event data must be valid JSON (JSONB compatible)")

			Expect(eventData).To(HaveKey("notification_id"),
				"Event data MUST contain notification_id for debugging")
			Expect(eventData).To(HaveKey("channel"),
				"Event data MUST contain channel for analytics")
			Expect(eventData).To(HaveKey("subject"),
				"Event data MUST contain subject for audit trail context")

			Expect(eventData["channel"]).To(Equal("slack"),
				"Channel in event_data MUST match actual delivery channel")
			Expect(eventData["notification_id"]).To(Equal("test-notification"),
				"Notification ID in event_data MUST match resource_id")

			// ===== BUSINESS OUTCOME VALIDATION ===== (Gap 2 Fix)
			// This audit event enables (BR-NOT-062):
			// ✅ Compliance audit queries (7-year retention enforced)
			// ✅ End-to-end workflow tracing (correlation_id = remediation_id)
			// ✅ V2.0 RAR timeline reconstruction (event_data contains all context)
			// ✅ Cross-service correlation (follows ADR-034 unified format)
		})
	})

	// ===== TDD CYCLE 2: CreateMessageFailedEvent =====
	// TDD Cycle 2: Write ONE test, verify it FAILS, then STOP
	Context("CreateMessageFailedEvent", func() {
		It("should create notification.message.failed audit event with error details", func() {
			// BR-NOT-062: Unified audit table integration
			deliveryError := fmt.Errorf("Slack API rate limit exceeded")

			// ===== BEHAVIOR TESTING =====
			event, err := helpers.CreateMessageFailedEvent(notification, "slack", deliveryError)
			Expect(err).ToNot(HaveOccurred(), "Event creation should not error")
			Expect(event).ToNot(BeNil(), "Event should be created")

			// ===== CORRECTNESS TESTING =====
			Expect(event.EventType).To(Equal("notification.message.failed"),
				"Event type MUST be 'notification.message.failed' for delivery failures")
			Expect(event.EventAction).To(Equal("sent"),
				"Event action MUST be 'sent' (attempted delivery)")
			Expect(event.EventOutcome).To(Equal("failure"),
				"Event outcome MUST be 'failure' for failed delivery")

			// Correctness: Error details captured for troubleshooting
			Expect(event.ErrorMessage).ToNot(BeNil(),
				"Error message MUST be captured for failed deliveries")
			Expect(*event.ErrorMessage).To(ContainSubstring("rate limit"),
				"Error message MUST contain actual failure reason")

			// Correctness: Event data includes channel and error context
			var eventData map[string]interface{}
			json.Unmarshal(event.EventData, &eventData)
			Expect(eventData["channel"]).To(Equal("slack"))
			Expect(eventData["error"]).To(ContainSubstring("rate limit"))

			// Correctness: Correlation ID for workflow tracing
			Expect(event.CorrelationID).To(Equal("remediation-123"),
				"Correlation ID MUST match for tracking failed attempts")

			// Business outcome: Failed delivery audited for retry analysis and SLA tracking
		})
	})

	// ===== TDD CYCLE 3: CreateMessageAcknowledgedEvent =====
	// TDD Cycle 3: Write ONE test, verify it FAILS, then STOP
	Context("CreateMessageAcknowledgedEvent", func() {
		It("should create notification.message.acknowledged audit event", func() {
			// BR-NOT-062: Unified audit table integration

			// ===== BEHAVIOR TESTING =====
			event, err := helpers.CreateMessageAcknowledgedEvent(notification)
			Expect(err).ToNot(HaveOccurred())
			Expect(event).ToNot(BeNil())

			// ===== CORRECTNESS TESTING =====
			Expect(event.EventType).To(Equal("notification.message.acknowledged"),
				"Event type MUST be 'notification.message.acknowledged'")
			Expect(event.EventAction).To(Equal("acknowledged"),
				"Event action MUST be 'acknowledged'")
			Expect(event.EventOutcome).To(Equal("success"),
				"Event outcome MUST be 'success'")
			Expect(event.CorrelationID).To(Equal("remediation-123"),
				"Correlation ID MUST match for workflow tracing")

			// Correctness: Event category and actor
			Expect(event.EventCategory).To(Equal("notification"))
			Expect(event.ActorType).To(Equal("service"))
			Expect(event.ActorID).To(Equal("notification-controller"))

			// Correctness: Resource identification
			Expect(event.ResourceType).To(Equal("NotificationRequest"))
			Expect(event.ResourceID).To(Equal("test-notification"))

			// Correctness: Retention for compliance
			Expect(event.RetentionDays).To(Equal(2555))

			// Business outcome: User acknowledgment tracked for compliance and effectiveness analysis
		})
	})

	// ===== TDD CYCLE 4: CreateMessageEscalatedEvent =====
	// TDD Cycle 4: Write ONE test, verify it FAILS (final cycle)
	Context("CreateMessageEscalatedEvent", func() {
		It("should create notification.message.escalated audit event", func() {
			// BR-NOT-062: Unified audit table integration

			// ===== BEHAVIOR TESTING =====
			event, err := helpers.CreateMessageEscalatedEvent(notification)
			Expect(err).ToNot(HaveOccurred())
			Expect(event).ToNot(BeNil())

			// ===== CORRECTNESS TESTING =====
			Expect(event.EventType).To(Equal("notification.message.escalated"),
				"Event type MUST be 'notification.message.escalated'")
			Expect(event.EventAction).To(Equal("escalated"),
				"Event action MUST be 'escalated'")
			Expect(event.EventOutcome).To(Equal("success"),
				"Event outcome MUST be 'success'")
			Expect(event.CorrelationID).To(Equal("remediation-123"),
				"Correlation ID MUST match for workflow tracing")

			// Correctness: Event category and actor
			Expect(event.EventCategory).To(Equal("notification"))
			Expect(event.ActorType).To(Equal("service"))
			Expect(event.ActorID).To(Equal("notification-controller"))

			// Correctness: Resource identification
			Expect(event.ResourceType).To(Equal("NotificationRequest"))
			Expect(event.ResourceID).To(Equal("test-notification"))

			// Correctness: Retention for compliance
			Expect(event.RetentionDays).To(Equal(2555))

			// Business outcome: Escalation tracked for incident timeline and response effectiveness
		})
	})

	// ========================================
	// GAP 3 FIX: DescribeTable Pattern for Event Types
	// Best Practice: Reduces test code by 75% for similar scenarios
	// See: DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md Day 3
	// ========================================
	Describe("Event Creation Matrix (DescribeTable Pattern)", func() {
		DescribeTable("Audit event creation for all notification states",
			func(eventType string, eventAction string, eventOutcome string, createFunc func() (*audit.AuditEvent, error), shouldSucceed bool, expectedErrorMsg string) {
				// BR-NOT-062: Unified audit table integration

				// BEHAVIOR: Create event
				event, err := createFunc()

				if shouldSucceed {
					// SUCCESS PATH
					Expect(err).ToNot(HaveOccurred())
					Expect(event).ToNot(BeNil())

					// CORRECTNESS: Validate ADR-034 format
					Expect(event.EventType).To(Equal(eventType),
						"Event type must match expected format")
					Expect(event.EventCategory).To(Equal("notification"),
						"Event category must be 'notification'")
					Expect(event.EventAction).To(Equal(eventAction),
						"Event action must match operation")
					Expect(event.EventOutcome).To(Equal(eventOutcome),
						"Event outcome must match result")
					Expect(event.CorrelationID).To(Equal("remediation-123"),
						"Correlation ID must match for workflow tracing")
					Expect(event.RetentionDays).To(Equal(2555),
						"Retention must be 7 years for compliance")

					// CORRECTNESS: Validate event_data structure
					var eventData map[string]interface{}
					err = json.Unmarshal(event.EventData, &eventData)
					Expect(err).ToNot(HaveOccurred())
					Expect(eventData).To(HaveKey("notification_id"))
				} else {
					// ERROR PATH
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErrorMsg))
				}
			},
			// SUCCESS CASES (4 event types)
			Entry("message sent successfully (BR-NOT-062)",
				"notification.message.sent", "sent", "success",
				func() (*audit.AuditEvent, error) {
					return helpers.CreateMessageSentEvent(notification, "slack")
				}, true, ""),
			Entry("message delivery failed (BR-NOT-062)",
				"notification.message.failed", "sent", "failure",
				func() (*audit.AuditEvent, error) {
					return helpers.CreateMessageFailedEvent(notification, "slack", fmt.Errorf("rate limited"))
				}, true, ""),
			Entry("message acknowledged (BR-NOT-062)",
				"notification.message.acknowledged", "acknowledged", "success",
				func() (*audit.AuditEvent, error) {
					return helpers.CreateMessageAcknowledgedEvent(notification)
				}, true, ""),
			Entry("message escalated (BR-NOT-062)",
				"notification.message.escalated", "escalated", "success",
				func() (*audit.AuditEvent, error) {
					return helpers.CreateMessageEscalatedEvent(notification)
				}, true, ""),

			// ERROR CASES (Edge Cases)
			Entry("nil notification returns error",
				"", "", "",
				func() (*audit.AuditEvent, error) {
					return helpers.CreateMessageSentEvent(nil, "slack")
				}, false, "notification cannot be nil"),
			Entry("empty channel string returns error",
				"", "", "",
				func() (*audit.AuditEvent, error) {
					return helpers.CreateMessageSentEvent(notification, "")
				}, false, "channel cannot be empty"),
		)
	})

	// ========================================
	// GAP 7 FIX: Enhanced Edge Cases (10 tests)
	// Authority: testing-strategy.md Critical Edge Case Categories
	// See: DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md Day 3
	// ========================================
	Describe("Edge Cases (ENHANCED)", func() {
		// ===== CATEGORY 1: Missing/Invalid Input (4 tests) =====

		Context("when RemediationID is missing", func() {
			It("should use notification name as correlation_id fallback", func() {
				// Edge Case: Missing correlation ID in metadata
				notification.Spec.Metadata = map[string]string{} // No remediationRequestName

				event, err := helpers.CreateMessageSentEvent(notification, "slack")

				Expect(err).ToNot(HaveOccurred())
				Expect(event.CorrelationID).To(Equal(notification.Name),
					"Correlation ID MUST fallback to notification.Name when remediationRequestName is empty")
			})
		})

		Context("when namespace is missing", func() {
			It("should handle gracefully with empty namespace", func() {
				// Edge Case: Empty namespace
				notification.Namespace = "" // Empty namespace

				_, err := helpers.CreateMessageSentEvent(notification, "slack")

				Expect(err).ToNot(HaveOccurred())
				// Namespace will be empty string, which is valid for non-namespaced scenarios
			})
		})

		Context("when notification is nil", func() {
			It("should return error 'notification cannot be nil'", func() {
				// Edge Case: Nil input validation
				event, err := helpers.CreateMessageSentEvent(nil, "slack")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("notification cannot be nil"))
				Expect(event).To(BeNil())
			})
		})

		Context("when channel is empty string", func() {
			It("should return error 'channel cannot be empty'", func() {
				// Edge Case: Empty channel validation
				event, err := helpers.CreateMessageSentEvent(notification, "")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("channel cannot be empty"))
				Expect(event).To(BeNil())
			})
		})

		// ===== CATEGORY 2: Boundary Conditions (3 tests) =====

		Context("when subject is very long", func() {
			It("should handle subject >10KB", func() {
				// Edge Case: Large string handling
				longSubject := string(make([]byte, 15000)) // 15KB subject
				for i := range longSubject {
					longSubject = longSubject[:i] + "A" + longSubject[i+1:]
				}
				notification.Spec.Subject = longSubject

				event, err := helpers.CreateMessageSentEvent(notification, "slack")

				Expect(err).ToNot(HaveOccurred())
				// Validate event_data can handle large strings (PostgreSQL JSONB supports up to 1GB)
				var eventData map[string]interface{}
				json.Unmarshal(event.EventData, &eventData)
				Expect(len(eventData["subject"].(string))).To(Equal(15000))
			})
		})

		Context("when subject is empty", func() {
			It("should handle gracefully with empty string in event_data", func() {
				// Edge Case: Empty boundary value
				notification.Spec.Subject = ""

				event, err := helpers.CreateMessageSentEvent(notification, "slack")

				Expect(err).ToNot(HaveOccurred())
				var eventData map[string]interface{}
				json.Unmarshal(event.EventData, &eventData)
				Expect(eventData["subject"]).To(Equal(""))
			})
		})

		Context("when event_data approaches PostgreSQL JSONB limit", func() {
			It("should handle maximum payload size (~1MB test)", func() {
				// Edge Case: PostgreSQL JSONB practical limit test (reduced for test performance)
				// Real limit is ~10MB, but we test with 1MB for faster execution
				largeBody := string(make([]byte, 1*1024*1024)) // 1MB body
				for i := range largeBody {
					largeBody = largeBody[:i] + "X" + largeBody[i+1:]
				}
				notification.Spec.Body = largeBody

				event, err := helpers.CreateMessageSentEvent(notification, "slack")

				Expect(err).ToNot(HaveOccurred())
				Expect(len(event.EventData)).To(BeNumerically(">", 1*1024*1024),
					"JSONB payload should handle large messages")

				// Note: If payload exceeds 10MB, consider truncation or separate storage
			})
		})

		// ===== CATEGORY 3: Error Conditions (1 test) =====

		Context("when channel name contains special characters", func() {
			It("should handle SQL injection patterns safely", func() {
				// Edge Case: SQL injection attempt (should be safe in JSONB)
				maliciousChannel := "slack'; DROP TABLE audit_events; --"

				event, err := helpers.CreateMessageSentEvent(notification, maliciousChannel)

				Expect(err).ToNot(HaveOccurred())
				var eventData map[string]interface{}
				json.Unmarshal(event.EventData, &eventData)
				// Channel stored in JSONB is safe (no SQL injection risk)
				Expect(eventData["channel"]).To(Equal(maliciousChannel))
			})
		})

		// ===== CATEGORY 4: Concurrency (1 test) 🔴 CRITICAL =====

		Context("when multiple notifications write audit events concurrently", func() {
			It("should handle concurrent audit writes without race conditions", func() {
				// BR-NOT-060: Concurrent delivery safety
				// BR-NOT-063: Graceful audit degradation

				const concurrentNotifications = 10
				var wg sync.WaitGroup
				wg.Add(concurrentNotifications)

				// Mock audit store for concurrency testing
				mockStore := NewMockAuditStore()

				// Create 10 notifications writing audit events simultaneously
				for i := 0; i < concurrentNotifications; i++ {
					go func(id int) {
						defer wg.Done()
						notif := createTestNotificationWithID(id)
						event, err := helpers.CreateMessageSentEvent(notif, "slack")
						Expect(err).ToNot(HaveOccurred())

						// Non-blocking audit write
						mockStore.StoreAudit(context.Background(), event)
					}(i)
				}

				wg.Wait()

				// Validate: All 10 events buffered (no race conditions)
				Expect(mockStore.GetEventCount()).To(Equal(10))

				// Note: This test MUST pass with race detector enabled
				// Run: go test -race ./internal/controller/notification/audit_test.go
			})
		})

		// ===== CATEGORY 5: Resource Limits (1 test) =====

		Context("when multiple events are created rapidly", func() {
			It("should handle burst event creation without errors", func() {
				// Edge Case: Rapid event creation (simulates high-volume notifications)
				// BR-NOT-063: Graceful audit degradation

				const burstCount = 100
				events := make([]*audit.AuditEvent, 0, burstCount)

				// Create 100 events rapidly
				for i := 0; i < burstCount; i++ {
					notif := createTestNotificationWithID(i)
					event, err := helpers.CreateMessageSentEvent(notif, "slack")
					Expect(err).ToNot(HaveOccurred())
					events = append(events, event)
				}

				// Validate: All events created successfully
				Expect(events).To(HaveLen(burstCount))

				// Validate: All events have unique notification IDs
				notifIDs := make(map[string]bool)
				for _, event := range events {
					var eventData map[string]interface{}
					json.Unmarshal(event.EventData, &eventData)
					notifID := eventData["notification_id"].(string)
					Expect(notifIDs[notifID]).To(BeFalse(), "Notification IDs should be unique")
					notifIDs[notifID] = true
				}
			})
		})
	})

	// ========================================
	// ADR-034 COMPLIANCE TESTS
	// Authority: ADR-034 Unified Audit Table Design
	// See: docs/architecture/decisions/ADR-034-unified-audit-table-design.md
	// ========================================
	Describe("ADR-034 Compliance Validation", func() {
		Context("Event Type Format", func() {
			It("should use event_type format: <service>.<category>.<action>", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				Expect(event.EventType).To(MatchRegexp(`^notification\.(message|status)\.(sent|failed|acknowledged|escalated)$`),
					"Event type must follow ADR-034 format: <service>.<category>.<action>")
			})
		})

		Context("Retention Period", func() {
			It("should set retention_days to 2555 (7 years) for compliance", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				Expect(event.RetentionDays).To(Equal(2555),
					"Retention must be 2555 days (7 years) for SOC 2 / ISO 27001 compliance")
			})
		})

		Context("Event Data Structure", func() {
			It("should populate event_data as valid JSONB", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				var eventData map[string]interface{}
				err := json.Unmarshal(event.EventData, &eventData)
				Expect(err).ToNot(HaveOccurred(),
					"Event data must be valid JSON for PostgreSQL JSONB compatibility")
				Expect(eventData).ToNot(BeEmpty(),
					"Event data must be populated with notification context")
			})
		})

		Context("Actor Type", func() {
			It("should set actor_type to 'service' for controller actions", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				Expect(event.ActorType).To(Equal("service"),
					"Actor type must be 'service' for Kubernetes controller actions (not 'user' or 'external')")
			})
		})

		Context("Correlation ID", func() {
			It("should use metadata['remediationRequestName'] as correlation_id for workflow tracing", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				Expect(event.CorrelationID).To(Equal("remediation-123"),
					"Correlation ID must match remediationRequestName for end-to-end workflow tracing (BR-NOT-064)")
			})
		})

		Context("Event Version", func() {
			It("should set event_version to '1.0'", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				Expect(event.EventVersion).To(Equal("1.0"),
					"Event version must be '1.0' for initial ADR-034 implementation")
			})
		})

		Context("Resource Identification", func() {
			It("should populate resource_type and resource_id for CRD identification", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				Expect(event.ResourceType).To(Equal("NotificationRequest"),
					"Resource type must match CRD kind")
				Expect(event.ResourceID).To(Equal("test-notification"),
					"Resource ID must match CRD name for correlation")
			})
		})
	})
})

// ===== TEST HELPERS =====

// createTestNotification creates a standard test notification fixture
func createTestNotification() *notificationv1alpha1.NotificationRequest {
	return &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-notification",
			Namespace: "default",
		},
		Spec: notificationv1alpha1.NotificationRequestSpec{
			Type:     notificationv1alpha1.NotificationTypeSimple,
			Priority: notificationv1alpha1.NotificationPriorityCritical,
			Subject:  "Test Alert: Database Connection Failed",
			Body:     "Critical alert detected in production database cluster",
			Channels: []notificationv1alpha1.Channel{
				notificationv1alpha1.ChannelSlack,
				notificationv1alpha1.ChannelEmail,
			},
			Recipients: []notificationv1alpha1.Recipient{
				{Slack: "#alerts"},
			},
			Metadata: map[string]string{
				"remediationRequestName": "remediation-123",
				"cluster":                "production-cluster",
				"namespace":              "database",
				"severity":               "critical",
			},
		},
	}
}

// createTestNotificationWithID creates a test notification with a specific ID
// Used for concurrency and multi-notification tests
func createTestNotificationWithID(id int) *notificationv1alpha1.NotificationRequest {
	notification := createTestNotification()
	notification.Name = fmt.Sprintf("test-notification-%d", id)
	notification.Spec.Metadata["remediationRequestName"] = fmt.Sprintf("remediation-%d", id)
	return notification
}

// MockAuditStore is a mock implementation of audit.AuditStore for unit testing
// Implements the AuditStore interface for testing audit helper methods
type MockAuditStore struct {
	events      []*audit.AuditEvent
	storeErrors []error
	mu          sync.Mutex
	closed      bool
}

// NewMockAuditStore creates a new mock audit store
func NewMockAuditStore() *MockAuditStore {
	return &MockAuditStore{
		events:      []*audit.AuditEvent{},
		storeErrors: []error{},
	}
}

// StoreAudit stores an audit event in memory
func (m *MockAuditStore) StoreAudit(ctx context.Context, event *audit.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.storeErrors) > 0 {
		err := m.storeErrors[0]
		m.storeErrors = m.storeErrors[1:]
		return err
	}

	m.events = append(m.events, event)
	return nil
}

// Close closes the mock audit store
func (m *MockAuditStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// GetEventCount returns the number of stored events (for testing)
func (m *MockAuditStore) GetEventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

// GetEvents returns all stored events (for testing)
func (m *MockAuditStore) GetEvents() []*audit.AuditEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*audit.AuditEvent, len(m.events))
	copy(result, m.events)
	return result
}

// SetStoreError sets an error to be returned on next StoreAudit call
func (m *MockAuditStore) SetStoreError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeErrors = append(m.storeErrors, err)
}

// IsClosed returns whether the store has been closed
func (m *MockAuditStore) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}
