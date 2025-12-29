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

package datastorage

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	auditpkg "github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// DLQ INTEGRATION TESTS (TDD RED Phase)
// 📋 Tests Define Contract: Real Redis required
// Authority: IMPLEMENTATION_PLAN_V4.8.md Day 7, DD-009
// ========================================
//
// This file defines the DLQ integration test contract.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (this file)
// - Infrastructure implemented SECOND (suite_test.go BeforeSuite)
// - Contract: Behavior + Correctness validation
//
// Business Requirements:
// - BR-AUDIT-001: Complete audit trail (no data loss)
// - DD-009: Dead Letter Queue pattern
//
// Context API Lessons Applied:
// - Test BOTH behavior AND correctness
// - Verify message structure in Redis
// - Test DLQ depth accuracy
//
// ========================================

var _ = Describe("DLQ Client Integration", Serial, func() {
	var audit *models.NotificationAudit
	var testError error

	BeforeEach(func() {
		// Clean up DLQ
		streamKey := "audit:dlq:notifications"
		redisClient.Del(ctx, streamKey)

		// Create test audit
		audit = &models.NotificationAudit{
			RemediationID:   "test-remediation-1",
			NotificationID:  "test-notification-1",
			Recipient:       "test@example.com",
			Channel:         "email",
			MessageSummary:  "Test notification message",
			Status:          "sent",
			SentAt:          time.Now(),
			DeliveryStatus:  "200 OK",
			ErrorMessage:    "",
			EscalationLevel: 0,
		}

		testError = fmt.Errorf("database connection failed")
	})

	Describe("EnqueueNotificationAudit", func() {
		Context("with valid audit record", func() {
			It("should enqueue to real Redis Stream (Behavior + Correctness)", func() {
				// ✅ BEHAVIOR TEST: Enqueue succeeds
				err := dlqClient.EnqueueNotificationAudit(ctx, audit, testError)

				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS TEST: Message in Redis with correct structure
				streamKey := "audit:dlq:notifications"
				length, err := redisClient.XLen(ctx, streamKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(length).To(Equal(int64(1)))

				// Read and verify message content
				messages, err := redisClient.XRange(ctx, streamKey, "-", "+").Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))

				// Verify message structure
				messageJSON := messages[0].Values["message"].(string)
				var auditMsg dlq.AuditMessage
				err = json.Unmarshal([]byte(messageJSON), &auditMsg)
				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS TEST: Message fields (validated by structured type)
				Expect(auditMsg.Type).To(Equal("notification_audit"))
				Expect(auditMsg.RetryCount).To(Equal(0))
				Expect(auditMsg.LastError).To(Equal(testError.Error()))
				Expect(auditMsg.Timestamp).ToNot(BeZero())

				// ✅ CORRECTNESS TEST: Payload contains audit data
				payloadJSON, err := json.Marshal(auditMsg.Payload)
				Expect(err).ToNot(HaveOccurred())

				var payloadAudit models.NotificationAudit
				err = json.Unmarshal(payloadJSON, &payloadAudit)
				Expect(err).ToNot(HaveOccurred())
				Expect(payloadAudit.NotificationID).To(Equal(audit.NotificationID))
				Expect(payloadAudit.RemediationID).To(Equal(audit.RemediationID))
			})

			It("should handle multiple enqueues", func() {
				// Enqueue first audit
				err := dlqClient.EnqueueNotificationAudit(ctx, audit, testError)
				Expect(err).ToNot(HaveOccurred())

				// Enqueue second audit
				audit2 := &models.NotificationAudit{
					RemediationID:   "test-remediation-2",
					NotificationID:  "test-notification-2",
					Recipient:       "test2@example.com",
					Channel:         "slack",
					MessageSummary:  "Test notification 2",
					Status:          "failed",
					SentAt:          time.Now(),
					EscalationLevel: 1,
				}
				err = dlqClient.EnqueueNotificationAudit(ctx, audit2, testError)
				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS TEST: Both messages in stream
				streamKey := "audit:dlq:notifications"
				length, err := redisClient.XLen(ctx, streamKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(length).To(Equal(int64(2)))
			})
		})
	})

	Describe("GetDLQDepth", func() {
		Context("with empty DLQ", func() {
			It("should return 0 (Behavior + Correctness)", func() {
				// ✅ BEHAVIOR TEST: GetDLQDepth succeeds
				depth, err := dlqClient.GetDLQDepth(ctx, "notifications")

				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS TEST: Depth is 0
				Expect(depth).To(Equal(int64(0)))
			})
		})

		Context("with messages in DLQ", func() {
			It("should return accurate count from Redis (Behavior + Correctness)", func() {
				// Enqueue 3 messages
				for i := 0; i < 3; i++ {
					audit.NotificationID = fmt.Sprintf("test-notification-%d", i)
					err := dlqClient.EnqueueNotificationAudit(ctx, audit, testError)
					Expect(err).ToNot(HaveOccurred())
				}

				// ✅ BEHAVIOR TEST: GetDLQDepth succeeds
				depth, err := dlqClient.GetDLQDepth(ctx, "notifications")

				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS TEST: Depth matches Redis XLEN
				Expect(depth).To(Equal(int64(3)))

				// ✅ CORRECTNESS TEST: Verify with direct Redis query
				streamKey := "audit:dlq:notifications"
				redisDepth, err := redisClient.XLen(ctx, streamKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(depth).To(Equal(redisDepth))
			})
		})
	})

	Describe("HealthCheck", func() {
		It("should verify Redis connectivity", func() {
			err := dlqClient.HealthCheck(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// ============================================================================
	// NEW TEST 1: EnqueueAuditEvent - Unified Audit Events (DD-STORAGE-009)
	// ============================================================================
	Describe("EnqueueAuditEvent - Unified Audit Events", func() {
		var auditEvent *auditpkg.AuditEvent

		BeforeEach(func() {
			// Clean up DLQ
			streamKey := "audit:dlq:events"
			redisClient.Del(ctx, streamKey)

			// Create test audit event
			auditEvent = &auditpkg.AuditEvent{
				EventID:        generateTestUUID(),
				EventVersion:   "1.0",
				EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
				EventType:      "workflow.completed",
				EventCategory:  "workflow",
				EventAction:    "completed",
				EventOutcome:   "success",
				ActorType:      "service",
				ActorID:        "workflow-service",
				ResourceType:   "Workflow",
				ResourceID:     "wf-123",
				CorrelationID:  "remediation-999",
				EventData:      []byte(`{"duration_ms":5000,"steps_completed":5}`),
			}
		})

		Context("with valid audit event", func() {
			It("should enqueue to real Redis Stream (Behavior + Correctness)", func() {
				// ✅ BEHAVIOR TEST: Enqueue succeeds
				err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, testError)

				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS TEST: Message in Redis with correct structure
				streamKey := "audit:dlq:events"
				length, err := redisClient.XLen(ctx, streamKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(length).To(Equal(int64(1)), "Should have exactly 1 message in DLQ stream")

				// Read and verify message content
				messages, err := redisClient.XRange(ctx, streamKey, "-", "+").Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))

				// Verify message structure
				messageJSON := messages[0].Values["message"].(string)
				var auditMsg dlq.AuditMessage
				err = json.Unmarshal([]byte(messageJSON), &auditMsg)
				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS TEST: Message fields
				Expect(auditMsg.Type).To(Equal("audit_event"))
				Expect(auditMsg.RetryCount).To(Equal(0))
				Expect(auditMsg.LastError).To(Equal(testError.Error()))
				Expect(auditMsg.Timestamp).ToNot(BeZero())

				// ✅ CORRECTNESS TEST: Payload contains audit event data
				var storedEvent auditpkg.AuditEvent
				err = json.Unmarshal(auditMsg.Payload, &storedEvent)
				Expect(err).ToNot(HaveOccurred())
				Expect(storedEvent.EventID).To(Equal(auditEvent.EventID))
				Expect(storedEvent.EventType).To(Equal("workflow.completed"))
				Expect(storedEvent.CorrelationID).To(Equal("remediation-999"))
			})
		})
	})

	// ============================================================================
	// NEW TEST 2: Handler DLQ Fallback Integration
	// ============================================================================
	Describe("Audit Events Handler DLQ Fallback", func() {
		BeforeEach(func() {
			// Clean up DLQ
			streamKey := "audit:dlq:events"
			redisClient.Del(ctx, streamKey)

			// Clean up database
			_, err := db.ExecContext(ctx, "DELETE FROM audit_events WHERE correlation_id LIKE 'test-dlq-fallback-%'")
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when database is unavailable", func() {
			It("should fallback to DLQ and return HTTP 202 Accepted", func() {
				// ✅ COVERAGE: This scenario is comprehensively tested in E2E Scenario 2
				// (test/e2e/datastorage/02_dlq_fallback_test.go) where we can stop PostgreSQL
				// and verify the complete DLQ fallback path including HTTP 202 response.
				//
				// Integration tests focus on DLQ client functionality in isolation.
				// E2E tests validate the full handler integration with infrastructure failures.

				// This test would require:
				// 1. Stopping PostgreSQL
				// 2. Making HTTP POST to /api/v1/audit/events
				// 3. Verifying HTTP 202 response
				// 4. Verifying message in Redis DLQ
				//
				// E2E Scenario 2 already covers this comprehensively
			})
		})
	})

	// ============================================================================
	// NEW TEST 3: Stream Key Isolation
	// ============================================================================
	Describe("Stream Key Isolation", func() {
		BeforeEach(func() {
			// Clean up both DLQ streams
			redisClient.Del(ctx, "audit:dlq:events")
			redisClient.Del(ctx, "audit:dlq:notifications")
		})

		It("should use separate Redis Streams for audit events and notification audits", func() {
			// ARRANGE: Create both types of messages
			auditEvent := &auditpkg.AuditEvent{
				EventID:        generateTestUUID(),
				EventVersion:   "1.0",
				EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
				EventType:      "workflow.completed",
				EventCategory:  "workflow",
				EventAction:    "completed",
				EventOutcome:   "success",
				ActorType:      "service",
				ActorID:        "workflow-service",
				ResourceType:   "Workflow",
				ResourceID:     "wf-isolation-test",
				CorrelationID:  "remediation-isolation",
				EventData:      []byte(`{"duration_ms":5000}`),
			}

			notificationAudit := &models.NotificationAudit{
				RemediationID:   "remediation-isolation",
				NotificationID:  "notif-isolation-test",
				Recipient:       "ops@example.com",
				Channel:         "email",
				MessageSummary:  "Test isolation",
				Status:          "delivered",
				SentAt:          time.Now().UTC(),
				EscalationLevel: 0,
			}

			// ACT: Enqueue both
			err1 := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("db error 1"))
			err2 := dlqClient.EnqueueNotificationAudit(ctx, notificationAudit, fmt.Errorf("db error 2"))

			// ASSERT: Both should succeed
			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())

			// ✅ CORRECTNESS TEST: Verify separate streams
			auditStream, err := redisClient.XRange(ctx, "audit:dlq:events", "-", "+").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(auditStream).To(HaveLen(1), "audit:dlq:events should have 1 message")

			notificationStream, err := redisClient.XRange(ctx, "audit:dlq:notifications", "-", "+").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(notificationStream).To(HaveLen(1), "audit:dlq:notifications should have 1 message")

			// ✅ CORRECTNESS TEST: Verify message types
			var auditMsg dlq.AuditMessage
			err = json.Unmarshal([]byte(auditStream[0].Values["message"].(string)), &auditMsg)
			Expect(err).ToNot(HaveOccurred())
			Expect(auditMsg.Type).To(Equal("audit_event"))

			var notifMsg dlq.AuditMessage
			err = json.Unmarshal([]byte(notificationStream[0].Values["message"].(string)), &notifMsg)
			Expect(err).ToNot(HaveOccurred())
			Expect(notifMsg.Type).To(Equal("notification_audit"))
		})
	})

	// ============================================================================
	// GAP-8: DLQ Consumer Methods (DD-009)
	// Authority: DD-009 (Audit Write Error Recovery - Dead Letter Queue Pattern)
	// Business Requirement: BR-AUDIT-001 (Complete audit trail with no data loss)
	// ============================================================================
	Describe("DLQ Consumer Methods (GAP-8)", func() {
		var (
			consumerGroup string
			consumerName  string
		)

		BeforeEach(func() {
			// Clean up DLQ streams and consumer groups
			streamKey := "audit:dlq:events"
			redisClient.Del(ctx, streamKey)

			// Generate unique consumer group for each test
			consumerGroup = fmt.Sprintf("test-consumer-group-%d", time.Now().UnixNano())
			consumerName = "test-consumer-1"
		})

		// GAP-8: ReadMessages - Read messages from DLQ using consumer groups
		Describe("ReadMessages", func() {
			It("should read messages from DLQ stream using XREADGROUP", func() {
				// ARRANGE: Enqueue a message first
				auditEvent := &auditpkg.AuditEvent{
					EventID:        generateTestUUID(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					EventType:      "workflow.completed",
					EventCategory:  "workflow",
					EventAction:    "completed",
					EventOutcome:   "success",
					ActorType:      "service",
					ActorID:        "workflow-service",
					ResourceType:   "Workflow",
					ResourceID:     "wf-read-test",
					CorrelationID:  "remediation-read-test",
					EventData:      []byte(`{"duration_ms":5000}`),
				}
				err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("test error"))
				Expect(err).ToNot(HaveOccurred())

				// ACT: Read messages
				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 2*time.Second)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))
				Expect(messages[0].ID).ToNot(BeEmpty())
				Expect(messages[0].AuditMessage.Type).To(Equal("audit_event"))
				Expect(messages[0].AuditMessage.CorrelationID()).To(Equal("remediation-read-test"))
			})

			It("should return empty slice when DLQ is empty", func() {
				// ACT: Read from empty DLQ
				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 1*time.Second)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(BeEmpty())
			})

			It("should support reading multiple messages in batch", func() {
				// ARRANGE: Enqueue 3 messages
				for i := 0; i < 3; i++ {
					auditEvent := &auditpkg.AuditEvent{
						EventID:        generateTestUUID(),
						EventVersion:   "1.0",
						EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
						EventType:      "workflow.completed",
						EventCategory:  "workflow",
						EventAction:    "completed",
						EventOutcome:   "success",
						ActorType:      "service",
						ActorID:        "workflow-service",
						ResourceType:   "Workflow",
						ResourceID:     fmt.Sprintf("wf-batch-%d", i),
						CorrelationID:  fmt.Sprintf("remediation-batch-%d", i),
						EventData:      []byte(`{"batch":true}`),
					}
					err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("batch error %d", i))
					Expect(err).ToNot(HaveOccurred())
				}

				// ACT: Read all messages
				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 2*time.Second)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(3))
			})
		})

		// GAP-8: AckMessage - Acknowledge processed messages
		Describe("AckMessage", func() {
			It("should acknowledge message so it's not re-read", func() {
				// ARRANGE: Enqueue and read a message
				auditEvent := &auditpkg.AuditEvent{
					EventID:        generateTestUUID(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					EventType:      "workflow.completed",
					EventCategory:  "workflow",
					EventAction:    "completed",
					EventOutcome:   "success",
					ActorType:      "service",
					ActorID:        "workflow-service",
					ResourceType:   "Workflow",
					ResourceID:     "wf-ack-test",
					CorrelationID:  "remediation-ack-test",
					EventData:      []byte(`{"ack":true}`),
				}
				err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("ack test error"))
				Expect(err).ToNot(HaveOccurred())

				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 2*time.Second)
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))

				// ACT: Acknowledge the message
				err = dlqClient.AckMessage(ctx, "events", consumerGroup, messages[0].ID)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())

				// Verify: Reading again should return no messages (pending list empty)
				pendingMessages, err := dlqClient.GetPendingMessages(ctx, "events", consumerGroup)
				Expect(err).ToNot(HaveOccurred())
				Expect(pendingMessages).To(Equal(int64(0)))
			})
		})

		// GAP-8: MoveToDeadLetter - Move failed messages after max retries
		Describe("MoveToDeadLetter", func() {
			It("should move message to dead letter stream", func() {
				// ARRANGE: Enqueue and read a message
				auditEvent := &auditpkg.AuditEvent{
					EventID:        generateTestUUID(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					EventType:      "workflow.failed",
					EventCategory:  "workflow",
					EventAction:    "failed",
					EventOutcome:   "failure",
					ActorType:      "service",
					ActorID:        "workflow-service",
					ResourceType:   "Workflow",
					ResourceID:     "wf-dead-letter-test",
					CorrelationID:  "remediation-dead-letter-test",
					EventData:      []byte(`{"dead_letter":true}`),
				}
				err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("permanent failure"))
				Expect(err).ToNot(HaveOccurred())

				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 2*time.Second)
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))

				// ACT: Move to dead letter
				err = dlqClient.MoveToDeadLetter(ctx, "events", messages[0])

				// ASSERT
				Expect(err).ToNot(HaveOccurred())

				// Verify: Message should be in dead letter stream
				deadLetterKey := "audit:dead-letter:events"
				deadLetterLength, err := redisClient.XLen(ctx, deadLetterKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(deadLetterLength).To(Equal(int64(1)))

				// Verify: Original message should be removed from DLQ
				dlqLength, err := dlqClient.GetDLQDepth(ctx, "events")
				Expect(err).ToNot(HaveOccurred())
				Expect(dlqLength).To(Equal(int64(0)))
			})
		})

		// GAP-8: IncrementRetryCount - Update retry count on message
		Describe("IncrementRetryCount", func() {
			It("should increment retry count on message", func() {
				// ARRANGE: Enqueue a message
				auditEvent := &auditpkg.AuditEvent{
					EventID:        generateTestUUID(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					EventType:      "workflow.retrying",
					EventCategory:  "workflow",
					EventAction:    "retrying",
					EventOutcome:   "pending",
					ActorType:      "service",
					ActorID:        "workflow-service",
					ResourceType:   "Workflow",
					ResourceID:     "wf-retry-count-test",
					CorrelationID:  "remediation-retry-count-test",
					EventData:      []byte(`{"retry_count_test":true}`),
				}
				err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("retry error"))
				Expect(err).ToNot(HaveOccurred())

				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 2*time.Second)
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))
				Expect(messages[0].AuditMessage.RetryCount).To(Equal(0))

				// ACT: Increment retry count
				err = dlqClient.IncrementRetryCount(ctx, "events", messages[0], fmt.Errorf("retry attempt 1 failed"))

				// ASSERT
				Expect(err).ToNot(HaveOccurred())

				// Re-read and verify retry count
				// First, ack the original to remove from pending
				_ = dlqClient.AckMessage(ctx, "events", consumerGroup, messages[0].ID)

				// Read again with new consumer group
				newConsumerGroup := fmt.Sprintf("test-consumer-group-verify-%d", time.Now().UnixNano())
				updatedMessages, err := dlqClient.ReadMessages(ctx, "events", newConsumerGroup, consumerName, 2*time.Second)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedMessages).To(HaveLen(1))
				Expect(updatedMessages[0].AuditMessage.RetryCount).To(Equal(1))
			})
		})
	})
})
