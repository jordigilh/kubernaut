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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	notificationaudit "github.com/jordigilh/kubernaut/pkg/notification/audit"
)

// Note: RunSpecs is called in suite_test.go - do not add another TestXxx function here
// to avoid "RunSpecs called more than once" error

var _ = Describe("Audit Helpers", func() {
	var (
		helpers      *notificationaudit.Manager
		notification *notificationv1alpha1.NotificationRequest
	)

	BeforeEach(func() {
		helpers = notificationaudit.NewManager("notification-controller")

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
			Expect(string(event.EventCategory)).To(Equal("notification"),
				"Event category MUST be 'notification' for all notification events")

			// Correctness: Event action describes operation
			Expect(event.EventAction).To(Equal("sent"),
				"Event action MUST be 'sent' for message delivery")

			// Correctness: Event outcome indicates success
			Expect(string(event.EventOutcome)).To(Equal("success"),
				"Event outcome MUST be 'success' for successful delivery")

			// Correctness: Actor correctly identifies source service
			Expect(event.ActorType.IsSet()).To(BeTrue())
			Expect(event.ActorType.Value).To(Equal("service"),
				"Actor type MUST be 'service' (not 'user' or 'external')")
			Expect(event.ActorID.IsSet()).To(BeTrue())
			Expect(event.ActorID.Value).To(Equal("notification-controller"),
				"Actor ID MUST match service name for traceability")

			// Correctness: Resource fields identify the notification CRD
			Expect(event.ResourceType.IsSet()).To(BeTrue())
			Expect(event.ResourceType.Value).To(Equal("NotificationRequest"),
				"Resource type MUST be 'NotificationRequest' (CRD kind)")
			Expect(event.ResourceID.IsSet()).To(BeTrue())
			Expect(event.ResourceID.Value).To(Equal("test-notification"),
				"Resource ID MUST match notification CRD name for correlation")

			// Correctness: Correlation ID enables end-to-end tracing (BR-NOT-064)
			Expect(event.CorrelationID).To(Equal("remediation-123"),
				"Correlation ID MUST match remediation_id for workflow tracing")

			// Correctness: Namespace populated for Kubernetes context
			Expect(event.Namespace).ToNot(BeNil(),
				"Namespace MUST be populated for Kubernetes context")
			Expect(event.Namespace.Value).To(Equal("default"),
				"Namespace MUST match notification CRD namespace")

			// Note: RetentionDays removed - not in OpenAPI spec (DD-AUDIT-002 V2.0.1)

			// Correctness: Event data is valid with required notification fields
			// V2.2: EventData is structured type, convert to map for testing
			eventDataBytes, err := json.Marshal(event.EventData)
			Expect(err).ToNot(HaveOccurred(), "EventData should be JSON-marshalable")
			var eventData map[string]interface{}
			err = json.Unmarshal(eventDataBytes, &eventData)
			Expect(err).ToNot(HaveOccurred(), "EventData should unmarshal to map")
			Expect(eventData).ToNot(BeEmpty(), "Event data must be populated (JSONB compatible)")

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
			Expect(string(event.EventOutcome)).To(Equal("failure"),
				"Event outcome MUST be 'failure' for failed delivery")

			// Note: ErrorMessage field removed - not in OpenAPI spec (DD-AUDIT-002 V2.0.1)
			// Error details are now captured in event_data only

			// Correctness: Event data includes channel and error context
			// V2.2: EventData is structured type, convert to map for testing
			eventDataBytes, err := json.Marshal(event.EventData)
			Expect(err).ToNot(HaveOccurred(), "EventData should be JSON-marshalable")
			var eventData map[string]interface{}
			err = json.Unmarshal(eventDataBytes, &eventData)
			Expect(err).ToNot(HaveOccurred(), "EventData should unmarshal to map")

			Expect(eventData["channel"]).To(Equal("slack"))
			Expect(eventData["error"]).To(ContainSubstring("rate limit"),
				"Error message MUST be in event_data for failed deliveries")

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
			Expect(string(event.EventOutcome)).To(Equal("success"),
				"Event outcome MUST be 'success'")
			Expect(event.CorrelationID).To(Equal("remediation-123"),
				"Correlation ID MUST match for workflow tracing")

			// Correctness: Event category and actor
			Expect(string(event.EventCategory)).To(Equal("notification"))
			Expect(event.ActorType.IsSet()).To(BeTrue())
			Expect(event.ActorType.Value).To(Equal("service"))
			Expect(event.ActorID.IsSet()).To(BeTrue())
			Expect(event.ActorID.Value).To(Equal("notification-controller"))

			// Correctness: Resource identification
			Expect(event.ResourceType.IsSet()).To(BeTrue())
			Expect(event.ResourceType.Value).To(Equal("NotificationRequest"))
			Expect(event.ResourceID.IsSet()).To(BeTrue())
			Expect(event.ResourceID.Value).To(Equal("test-notification"))

			// Note: RetentionDays removed - not in OpenAPI spec (DD-AUDIT-002 V2.0.1)

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
			Expect(string(event.EventOutcome)).To(Equal("success"),
				"Event outcome MUST be 'success'")
			Expect(event.CorrelationID).To(Equal("remediation-123"),
				"Correlation ID MUST match for workflow tracing")

			// Correctness: Event category and actor
			Expect(string(event.EventCategory)).To(Equal("notification"))
			Expect(event.ActorType.IsSet()).To(BeTrue())
			Expect(event.ActorType.Value).To(Equal("service"))
			Expect(event.ActorID.IsSet()).To(BeTrue())
			Expect(event.ActorID.Value).To(Equal("notification-controller"))

			// Correctness: Resource identification
			Expect(event.ResourceType.IsSet()).To(BeTrue())
			Expect(event.ResourceType.Value).To(Equal("NotificationRequest"))
			Expect(event.ResourceID.IsSet()).To(BeTrue())
			Expect(event.ResourceID.Value).To(Equal("test-notification"))

			// Note: RetentionDays removed - not in OpenAPI spec (DD-AUDIT-002 V2.0.1)

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
			func(eventType string, eventAction string, eventOutcome string, createFunc func() (*ogenclient.AuditEventRequest, error), shouldSucceed bool, expectedErrorMsg string) {
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
					Expect(string(event.EventCategory)).To(Equal("notification"),
						"Event category must be 'notification'")
					Expect(event.EventAction).To(Equal(eventAction),
						"Event action must match operation")
					Expect(string(event.EventOutcome)).To(Equal(eventOutcome),
						"Event outcome must match result")
					Expect(event.CorrelationID).To(Equal("remediation-123"),
						"Correlation ID must match for workflow tracing")
					// Note: RetentionDays removed - not in OpenAPI spec (DD-AUDIT-002 V2.0.1)

					// CORRECTNESS: Validate event_data structure
					// V2.2: EventData is structured type, convert to map for testing
					eventDataBytes, marshalErr := json.Marshal(event.EventData)
					Expect(marshalErr).ToNot(HaveOccurred())
					var eventData map[string]interface{}
					marshalErr = json.Unmarshal(eventDataBytes, &eventData)
					Expect(marshalErr).ToNot(HaveOccurred())
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
				func() (*ogenclient.AuditEventRequest, error) {
					return helpers.CreateMessageSentEvent(notification, "slack")
				}, true, ""),
			Entry("message delivery failed (BR-NOT-062)",
				"notification.message.failed", "sent", "failure",
				func() (*ogenclient.AuditEventRequest, error) {
					return helpers.CreateMessageFailedEvent(notification, "slack", fmt.Errorf("rate limited"))
				}, true, ""),
			Entry("message acknowledged (BR-NOT-062)",
				"notification.message.acknowledged", "acknowledged", "success",
				func() (*ogenclient.AuditEventRequest, error) {
					return helpers.CreateMessageAcknowledgedEvent(notification)
				}, true, ""),
			Entry("message escalated (BR-NOT-062)",
				"notification.message.escalated", "escalated", "success",
				func() (*ogenclient.AuditEventRequest, error) {
					return helpers.CreateMessageEscalatedEvent(notification)
				}, true, ""),

			// ERROR CASES (Edge Cases) - CreateMessageSentEvent
			Entry("nil notification for CreateMessageSentEvent returns error",
				"", "", "",
				func() (*ogenclient.AuditEventRequest, error) {
					return helpers.CreateMessageSentEvent(nil, "slack")
				}, false, "notification cannot be nil"),
			Entry("empty channel for CreateMessageSentEvent returns error",
				"", "", "",
				func() (*ogenclient.AuditEventRequest, error) {
					return helpers.CreateMessageSentEvent(notification, "")
				}, false, "channel cannot be empty"),

			// ERROR CASES - CreateMessageFailedEvent (100% coverage fix)
			Entry("nil notification for CreateMessageFailedEvent returns error",
				"", "", "",
				func() (*ogenclient.AuditEventRequest, error) {
					return helpers.CreateMessageFailedEvent(nil, "slack", fmt.Errorf("test error"))
				}, false, "notification cannot be nil"),
			Entry("empty channel for CreateMessageFailedEvent returns error",
				"", "", "",
				func() (*ogenclient.AuditEventRequest, error) {
					return helpers.CreateMessageFailedEvent(notification, "", fmt.Errorf("test error"))
				}, false, "channel cannot be empty"),

			// ERROR CASES - CreateMessageAcknowledgedEvent (100% coverage fix)
			Entry("nil notification for CreateMessageAcknowledgedEvent returns error",
				"", "", "",
				func() (*ogenclient.AuditEventRequest, error) {
					return helpers.CreateMessageAcknowledgedEvent(nil)
				}, false, "notification cannot be nil"),

			// ERROR CASES - CreateMessageEscalatedEvent (100% coverage fix)
			Entry("nil notification for CreateMessageEscalatedEvent returns error",
				"", "", "",
				func() (*ogenclient.AuditEventRequest, error) {
					return helpers.CreateMessageEscalatedEvent(nil)
				}, false, "notification cannot be nil"),
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
			It("should use notification UID as correlation_id fallback", func() {
				// Edge Case: Missing correlation ID in metadata
				notification.Spec.Metadata = map[string]string{} // No remediationRequestName

				event, err := helpers.CreateMessageSentEvent(notification, "slack")

				Expect(err).ToNot(HaveOccurred())
				Expect(event.CorrelationID).To(Equal(string(notification.UID)),
					"Correlation ID MUST fallback to notification.UID when remediationRequestName is empty (per ADR-032)")
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

		// ===== 100% COVERAGE FIX: Additional Input Validation =====

		Context("when Metadata is nil (not empty map)", func() {
			It("should use notification UID as correlation_id fallback", func() {
				// Edge Case: nil Metadata (different from empty map)
				// BEHAVIOR: Event creation succeeds
				// CORRECTNESS: correlation_id falls back to notification.UID (per ADR-032)
				notification.Spec.Metadata = nil // nil, not map[string]string{}

				event, err := helpers.CreateMessageSentEvent(notification, "slack")

				Expect(err).ToNot(HaveOccurred(),
					"BEHAVIOR: Event creation should succeed with nil Metadata")
				Expect(event.CorrelationID).To(Equal(string(notification.UID)),
					"CORRECTNESS: Correlation ID MUST fallback to notification.UID when Metadata is nil (per ADR-032)")
			})
		})

		Context("when error is nil for CreateMessageFailedEvent", func() {
			It("should create event without error details", func() {
				// Edge Case: nil error for failed event
				// BEHAVIOR: Event creation succeeds
				// CORRECTNESS: event_data has no "error" key
				event, err := helpers.CreateMessageFailedEvent(notification, "slack", nil)

				Expect(err).ToNot(HaveOccurred(),
					"BEHAVIOR: Event creation should succeed with nil error")
				// Note: ErrorMessage field removed - not in OpenAPI spec (DD-AUDIT-002 V2.0.1)

				// V2.2: EventData is structured type, convert to map for testing
				eventDataBytes, err := json.Marshal(event.EventData)
				Expect(err).ToNot(HaveOccurred())
				var eventData map[string]interface{}
				err = json.Unmarshal(eventDataBytes, &eventData)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventData).ToNot(HaveKey("error"),
					"CORRECTNESS: event_data should not have 'error' key when error is nil")
			})
		})

		// ===== CATEGORY 2: Boundary Conditions (3 tests) =====

		Context("when subject is very long", func() {
			It("should handle subject >10KB", func() {
				// Edge Case: Large string handling
				longSubjectBytes := make([]byte, 15000) // 15KB subject
				for i := range longSubjectBytes {
					longSubjectBytes[i] = 'A'
				}
				notification.Spec.Subject = string(longSubjectBytes)

				event, err := helpers.CreateMessageSentEvent(notification, "slack")

				Expect(err).ToNot(HaveOccurred())
				// Validate event_data can handle large strings (PostgreSQL JSONB supports up to 1GB)
				// V2.2: EventData is structured type, convert to map for testing
				eventDataBytes, err := json.Marshal(event.EventData)
				Expect(err).ToNot(HaveOccurred())
				var eventData map[string]interface{}
				err = json.Unmarshal(eventDataBytes, &eventData)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(eventData["subject"].(string))).To(Equal(15000))
			})
		})

		Context("when subject is empty", func() {
			It("should handle gracefully with empty string in event_data", func() {
				// Edge Case: Empty boundary value
				notification.Spec.Subject = ""

				event, err := helpers.CreateMessageSentEvent(notification, "slack")

				Expect(err).ToNot(HaveOccurred())
				// V2.2: EventData is structured type, convert to map for testing
				eventDataBytes, err := json.Marshal(event.EventData)
				Expect(err).ToNot(HaveOccurred())
				var eventData map[string]interface{}
				err = json.Unmarshal(eventDataBytes, &eventData)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventData["subject"]).To(Equal(""))
			})
		})

		Context("when event_data approaches PostgreSQL JSONB limit", func() {
			It("should handle maximum payload size (~1MB test)", func() {
				// Edge Case: PostgreSQL JSONB practical limit test (reduced for test performance)
				// Real limit is ~10MB, but we test with 1MB for faster execution
				largeBodyBytes := make([]byte, 1*1024*1024) // 1MB body
				for i := range largeBodyBytes {
					largeBodyBytes[i] = 'X'
				}
				notification.Spec.Body = string(largeBodyBytes)

				event, err := helpers.CreateMessageSentEvent(notification, "slack")

				Expect(err).ToNot(HaveOccurred())
				eventDataBytes, _ := json.Marshal(event.EventData)
				Expect(len(eventDataBytes)).To(BeNumerically(">", 1*1024*1024),
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
				// V2.2: EventData is structured type, convert to map for testing
				eventDataBytes, err := json.Marshal(event.EventData)
				Expect(err).ToNot(HaveOccurred())
				var eventData map[string]interface{}
				err = json.Unmarshal(eventDataBytes, &eventData)
				Expect(err).ToNot(HaveOccurred())
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
						_ = mockStore.StoreAudit(context.Background(), event)
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
				events := make([]*ogenclient.AuditEventRequest, 0, burstCount)

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
					// V2.2: EventData is structured type, convert to map for testing
					eventDataBytes, err := json.Marshal(event.EventData)
					Expect(err).ToNot(HaveOccurred())
					var eventData map[string]interface{}
					err = json.Unmarshal(eventDataBytes, &eventData)
					Expect(err).ToNot(HaveOccurred())
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

		// NOTE: Retention Period test removed (DD-AUDIT-002 V2.0.1)
		// RetentionDays field is not present in OpenAPI spec, managed by Data Storage service

		Context("Event Data Structure", func() {
			It("should populate event_data as valid JSONB", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				// V2.2: EventData is structured type, verify it's marshalable
				Expect(event.EventData).ToNot(BeNil(),
					"Event data must be populated with notification context")
				eventDataBytes, err := json.Marshal(event.EventData)
				Expect(err).ToNot(HaveOccurred(), "EventData should be JSON-marshalable")
				Expect(eventDataBytes).ToNot(BeEmpty(), "Marshaled data should not be empty")
			})
		})

		Context("Actor Type", func() {
			It("should set actor_type to 'service' for controller actions", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				Expect(event.ActorType.IsSet()).To(BeTrue())
				Expect(event.ActorType.Value).To(Equal("service"),
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
			It("should set version to '1.0'", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				Expect(event.Version).To(Equal("1.0"),
					"Version must be '1.0' for initial ADR-034 implementation")
			})
		})

		Context("Resource Identification", func() {
			It("should populate resource_type and resource_id for CRD identification", func() {
				event, _ := helpers.CreateMessageSentEvent(notification, "slack")
				Expect(event.ResourceType.IsSet()).To(BeTrue())
				Expect(event.ResourceType.Value).To(Equal("NotificationRequest"),
					"Resource type must match CRD kind")
				Expect(event.ResourceID.IsSet()).To(BeTrue())
				Expect(event.ResourceID.Value).To(Equal("test-notification"),
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
	events      []*ogenclient.AuditEventRequest
	storeErrors []error
	mu          sync.Mutex
	closed      bool
}

// NewMockAuditStore creates a new mock audit store
func NewMockAuditStore() *MockAuditStore {
	return &MockAuditStore{
		events:      []*ogenclient.AuditEventRequest{},
		storeErrors: []error{},
	}
}

// StoreAudit stores an audit event in memory
func (m *MockAuditStore) StoreAudit(ctx context.Context, event *ogenclient.AuditEventRequest) error {
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

// Flush forces immediate flush of buffered events (no-op in mock)
func (m *MockAuditStore) Flush(ctx context.Context) error {
	// Mock: no-op - events already stored synchronously
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
func (m *MockAuditStore) GetEvents() []*ogenclient.AuditEventRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*ogenclient.AuditEventRequest, len(m.events))
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
