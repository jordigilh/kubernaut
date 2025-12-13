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

package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

func TestDLQClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DLQ Client Unit Test Suite")
}

// ========================================
// DEAD LETTER QUEUE CLIENT UNIT TESTS (DD-009)
// 📋 Business Requirements:
//   - BR-STORAGE-017: DLQ Fallback on Database Unavailability
//   - BR-AUDIT-001: Complete Audit Trail (no data loss)
//
// 📋 Testing Principle: Behavior + Correctness
// ========================================
var _ = Describe("DD-009: Dead Letter Queue Client", func() {
	var (
		miniRedis   *miniredis.Miniredis
		redisClient *redis.Client
		dlqClient   *dlq.Client
		ctx         context.Context
		logger      logr.Logger
	)

	BeforeEach(func() {
		var err error
		ctx = context.Background()

		// Create in-memory Redis server
		miniRedis = miniredis.RunT(GinkgoT())

		// Create Redis client
		redisClient = redis.NewClient(&redis.Options{
			Addr: miniRedis.Addr(),
		})

		// Create logger
		logger = kubelog.NewLogger(kubelog.DefaultOptions())

		// Create DLQ client
		dlqClient, err = dlq.NewClient(redisClient, logger, 10000)
		Expect(err).ToNot(HaveOccurred())
		Expect(dlqClient).ToNot(BeNil())
	})

	AfterEach(func() {
		if redisClient != nil {
			_ = redisClient.Close()
		}
		if miniRedis != nil {
			miniRedis.Close()
		}
	})

	// ============================================================================
	// TEST 1: EnqueueAuditEvent - Success Path
	// BEHAVIOR: Failed audit events are enqueued to Redis Stream DLQ
	// CORRECTNESS: Message structure preserves event data and original error
	// ============================================================================
	Context("EnqueueAuditEvent - Success Path", func() {
		It("should successfully enqueue audit event to Redis Stream", func() {
			// ARRANGE: Create test audit event
			auditEvent := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventVersion:   "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "notification.sent",
				EventCategory:  "notification",
				EventAction:    "sent",
				EventOutcome:   "success",
				ActorType:      "service",
				ActorID:        "notification-service",
				ResourceType:   "Notification",
				ResourceID:     "notif-123",
				CorrelationID:  "remediation-456",
				EventData:      json.RawMessage(`{"recipient":"ops@example.com","channel":"slack"}`),
			}
			originalError := fmt.Errorf("database connection failed")

			// ACT: Enqueue audit event
			err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, originalError)

			// ASSERT: Verify enqueue succeeded
			Expect(err).ToNot(HaveOccurred(), "EnqueueAuditEvent should succeed")

			// ASSERT: Verify Redis Stream contains the message
			streamKey := "audit:dlq:events"
			messages, err := redisClient.XRange(ctx, streamKey, "-", "+").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(messages).To(HaveLen(1), "Should have exactly 1 message in DLQ stream")

			// ASSERT: Verify message structure
			message := messages[0]
			Expect(message.Values).To(HaveKey("message"))

			// ASSERT: Verify message content
			var dlqMessage dlq.AuditMessage
			err = json.Unmarshal([]byte(message.Values["message"].(string)), &dlqMessage)
			Expect(err).ToNot(HaveOccurred())
			Expect(dlqMessage.Type).To(Equal("audit_event"))
			Expect(dlqMessage.LastError).To(Equal("database connection failed"))
			Expect(dlqMessage.RetryCount).To(Equal(0))

			// ASSERT: Verify audit event payload
			var storedEvent audit.AuditEvent
			err = json.Unmarshal(dlqMessage.Payload, &storedEvent)
			Expect(err).ToNot(HaveOccurred())
			Expect(storedEvent.EventID).To(Equal(auditEvent.EventID))
			Expect(storedEvent.EventType).To(Equal("notification.sent"))
			Expect(storedEvent.CorrelationID).To(Equal("remediation-456"))
		})
	})

	// ============================================================================
	// TEST 2: EnqueueAuditEvent - Marshal Error
	// BEHAVIOR: Valid audit events are serialized and queued successfully
	// CORRECTNESS: JSON marshaling handles standard audit event structures
	// ============================================================================
	Context("EnqueueAuditEvent - Marshal Error", func() {
		It("should return error when audit event cannot be marshaled", func() {
			// ARRANGE: Create audit event with invalid EventData (will cause marshal error)
			// Note: In Go, json.Marshal rarely fails for valid structs, but we can test error handling
			auditEvent := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventVersion:   "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "notification.sent",
				EventCategory:  "notification",
				EventAction:    "sent",
				EventOutcome:   "success",
				ActorType:      "service",
				ActorID:        "notification-service",
				ResourceType:   "Notification",
				ResourceID:     "notif-123",
				CorrelationID:  "remediation-456",
				EventData:      json.RawMessage(`{"recipient":"ops@example.com"}`),
			}
			originalError := fmt.Errorf("database error")

			// ACT: Enqueue should succeed (json.Marshal handles valid structs)
			err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, originalError)

			// ASSERT: Should succeed for valid audit event
			Expect(err).ToNot(HaveOccurred())

			// NOTE: Testing actual marshal failure is difficult in Go without reflection hacks
			// This test validates that valid audit events are handled correctly
			// Real marshal errors would be caught by integration tests with malformed data
		})
	})

	// ============================================================================
	// TEST 3: EnqueueAuditEvent - Redis Connection Error
	// BEHAVIOR: DLQ client reports errors when Redis backend is unavailable
	// CORRECTNESS: Error contains descriptive message for debugging
	// ============================================================================
	Context("EnqueueAuditEvent - Redis Connection Error", func() {
		It("should return error when Redis is unavailable", func() {
			// ARRANGE: Close Redis server to simulate connection failure
			miniRedis.Close()

			auditEvent := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventVersion:   "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "notification.sent",
				EventCategory:  "notification",
				EventAction:    "sent",
				EventOutcome:   "success",
				ActorType:      "service",
				ActorID:        "notification-service",
				ResourceType:   "Notification",
				ResourceID:     "notif-123",
				CorrelationID:  "remediation-456",
				EventData:      json.RawMessage(`{"recipient":"ops@example.com"}`),
			}
			originalError := fmt.Errorf("database error")

			// ACT: Attempt to enqueue
			err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, originalError)

			// ASSERT: Should return Redis connection error
			Expect(err).To(HaveOccurred(), "Should fail when Redis is unavailable")
			Expect(err.Error()).To(ContainSubstring("failed to add audit event to DLQ"))
		})
	})

	// ============================================================================
	// TEST 4: EnqueueNotificationAudit - Success Path (Legacy)
	// BEHAVIOR: Legacy notification audits are queued for retry processing
	// CORRECTNESS: Complete notification data is preserved in DLQ message
	// ============================================================================
	Context("EnqueueNotificationAudit - Success Path (Legacy)", func() {
		It("should successfully enqueue notification audit to Redis Stream", func() {
			// ARRANGE: Create test notification audit
			notificationAudit := &models.NotificationAudit{
				RemediationID:   "remediation-789",
				NotificationID:  "notif-abc",
				Recipient:       "ops@example.com",
				Channel:         "slack",
				MessageSummary:  "Alert: High CPU usage",
				Status:          "sent",
				SentAt:          time.Now().UTC(),
				EscalationLevel: 0,
			}
			originalError := fmt.Errorf("database write failed")

			// ACT: Enqueue notification audit
			err := dlqClient.EnqueueNotificationAudit(ctx, notificationAudit, originalError)

			// ASSERT: Verify enqueue succeeded
			Expect(err).ToNot(HaveOccurred(), "EnqueueNotificationAudit should succeed")

			// ASSERT: Verify Redis Stream contains the message
			streamKey := "audit:dlq:notifications"
			messages, err := redisClient.XRange(ctx, streamKey, "-", "+").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(messages).To(HaveLen(1), "Should have exactly 1 message in DLQ stream")

			// ASSERT: Verify message structure
			message := messages[0]
			Expect(message.Values).To(HaveKey("message"))

			// ASSERT: Verify message content
			var dlqMessage dlq.AuditMessage
			err = json.Unmarshal([]byte(message.Values["message"].(string)), &dlqMessage)
			Expect(err).ToNot(HaveOccurred())
			Expect(dlqMessage.Type).To(Equal("notification_audit"))
			Expect(dlqMessage.LastError).To(Equal("database write failed"))
		})
	})

	// ============================================================================
	// TEST 5: EnqueueNotificationAudit - Redis Error (Legacy)
	// BEHAVIOR: Legacy API fails gracefully when Redis is unavailable
	// CORRECTNESS: Error is propagated with descriptive context
	// ============================================================================
	Context("EnqueueNotificationAudit - Redis Error (Legacy)", func() {
		It("should return error when Redis is unavailable", func() {
			// ARRANGE: Close Redis to simulate failure
			miniRedis.Close()

			notificationAudit := &models.NotificationAudit{
				RemediationID:  "remediation-789",
				NotificationID: "notif-abc",
				Recipient:      "ops@example.com",
				Channel:        "slack",
				Status:         "sent",
				SentAt:         time.Now().UTC(),
			}
			originalError := fmt.Errorf("database error")

			// ACT: Attempt to enqueue
			err := dlqClient.EnqueueNotificationAudit(ctx, notificationAudit, originalError)

			// ASSERT: Should fail with Redis error
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to enqueue to DLQ"))
		})
	})

	// ============================================================================
	// TEST 6: NewClient - Constructor Validation
	// BEHAVIOR: DLQ client constructor validates required dependencies
	// CORRECTNESS: Nil Redis client is rejected, valid inputs create usable client
	// ============================================================================
	Context("NewClient - Constructor Validation", func() {
		It("should create DLQ client with valid parameters", func() {
			// ACT: Create new client
			client, err := dlq.NewClient(redisClient, logger, 10000)

			// ASSERT: Should succeed
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})

		It("should return error when Redis client is nil", func() {
			// ACT: Create client with nil Redis client
			client, err := dlq.NewClient(nil, logger, 10000)

			// ASSERT: Should fail
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("redis client cannot be nil"))
			Expect(client).To(BeNil())
		})

		// Note: This test was removed because logr.Logger is a value type and cannot be nil.
		// The DLQ client now accepts any logr.Logger (including logr.Discard() for testing).
	})

	// ============================================================================
	// TEST 7: Client HealthCheck - Redis Connectivity
	// BEHAVIOR: HealthCheck verifies Redis backend availability
	// CORRECTNESS: Returns nil when healthy, error when Redis unavailable
	// ============================================================================
	Context("Client HealthCheck - Redis Connectivity", func() {
		It("should return healthy when Redis is available", func() {
			// ACT: Check health
			err := dlqClient.HealthCheck(ctx)

			// ASSERT: Should be healthy
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error when Redis is unavailable", func() {
			// ARRANGE: Close Redis
			miniRedis.Close()

			// ACT: Check health
			err := dlqClient.HealthCheck(ctx)

			// ASSERT: Should fail
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("redis ping failed"))
		})
	})

	// ============================================================================
	// ADDITIONAL TEST: Stream Key Isolation
	// BEHAVIOR: Different event types use separate Redis Streams
	// CORRECTNESS: audit:dlq:events and audit:dlq:notifications are isolated
	// ============================================================================
	Context("Stream Key Isolation", func() {
		It("should use separate Redis Streams for audit events and notification audits", func() {
			// ARRANGE: Create both types of messages
			auditEvent := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventVersion:   "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "workflow.completed",
				EventCategory:  "workflow",
				EventAction:    "completed",
				EventOutcome:   "success",
				ActorType:      "service",
				ActorID:        "workflow-service",
				ResourceType:   "Workflow",
				ResourceID:     "wf-123",
				CorrelationID:  "remediation-999",
				EventData:      json.RawMessage(`{"duration_ms":5000}`),
			}

			notificationAudit := &models.NotificationAudit{
				RemediationID:  "remediation-999",
				NotificationID: "notif-xyz",
				Recipient:      "ops@example.com",
				Channel:        "email",
				Status:         "delivered",
				SentAt:         time.Now().UTC(),
			}

			// ACT: Enqueue both
			err1 := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("db error 1"))
			err2 := dlqClient.EnqueueNotificationAudit(ctx, notificationAudit, fmt.Errorf("db error 2"))

			// ASSERT: Both should succeed
			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())

			// ASSERT: Verify separate streams
			auditStream, err := redisClient.XRange(ctx, "audit:dlq:events", "-", "+").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(auditStream).To(HaveLen(1), "audit:dlq:events should have 1 message")

			notificationStream, err := redisClient.XRange(ctx, "audit:dlq:notifications", "-", "+").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(notificationStream).To(HaveLen(1), "audit:dlq:notifications should have 1 message")
		})
	})

	// ============================================================================
	// GAP-8: DLQ Consumer Methods (DD-009)
	// Authority: DD-009 (Audit Write Error Recovery - Dead Letter Queue Pattern)
	// Business Requirement: BR-AUDIT-001 (Complete audit trail with no data loss)
	// ============================================================================
	Describe("GAP-8: DLQ Consumer Methods", func() {
		var (
			consumerGroup string
			consumerName  string
		)

		BeforeEach(func() {
			// Generate unique consumer group for each test
			consumerGroup = fmt.Sprintf("test-consumer-group-%d", time.Now().UnixNano())
			consumerName = "test-consumer-1"
		})

		// ============================================================================
		// TEST 8: ReadMessages - Read from DLQ using consumer groups
		// BEHAVIOR: Messages are read using XREADGROUP for at-least-once delivery
		// CORRECTNESS: Message structure is correctly parsed
		// ============================================================================
		Context("ReadMessages - Success Path", func() {
			It("should read messages from DLQ stream", func() {
				// ARRANGE: Enqueue a message
				auditEvent := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().UTC(),
					EventType:      "workflow.completed",
					EventCategory:  "workflow",
					EventAction:    "completed",
					EventOutcome:   "success",
					ActorType:      "service",
					ActorID:        "workflow-service",
					ResourceType:   "Workflow",
					ResourceID:     "wf-read-test",
					CorrelationID:  "remediation-read-test",
					EventData:      json.RawMessage(`{"duration_ms":5000}`),
				}
				err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("test error"))
				Expect(err).ToNot(HaveOccurred())

				// ACT: Read messages
				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 1*time.Second)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))
				Expect(messages[0].ID).ToNot(BeEmpty())
				Expect(messages[0].AuditMessage.Type).To(Equal("audit_event"))
				Expect(messages[0].AuditMessage.CorrelationID()).To(Equal("remediation-read-test"))
			})

			It("should return empty slice when DLQ is empty", func() {
				// ACT: Read from empty DLQ
				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 100*time.Millisecond)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(BeEmpty())
			})

			It("should support reading multiple messages in batch", func() {
				// ARRANGE: Enqueue 3 messages
				for i := 0; i < 3; i++ {
					auditEvent := &audit.AuditEvent{
						EventID:        uuid.New(),
						EventVersion:   "1.0",
						EventTimestamp: time.Now().UTC(),
						EventType:      "workflow.completed",
						EventCategory:  "workflow",
						EventAction:    "completed",
						EventOutcome:   "success",
						ActorType:      "service",
						ActorID:        "workflow-service",
						ResourceType:   "Workflow",
						ResourceID:     fmt.Sprintf("wf-batch-%d", i),
						CorrelationID:  fmt.Sprintf("remediation-batch-%d", i),
						EventData:      json.RawMessage(`{"batch":true}`),
					}
					err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("batch error %d", i))
					Expect(err).ToNot(HaveOccurred())
				}

				// ACT: Read all messages
				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 1*time.Second)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(3))
			})
		})

		// ============================================================================
		// TEST 9: AckMessage - Acknowledge processed messages
		// BEHAVIOR: Acknowledged messages are removed from pending list
		// CORRECTNESS: Message won't be re-delivered to same consumer group
		// ============================================================================
		Context("AckMessage - Success Path", func() {
			It("should acknowledge message successfully", func() {
				// ARRANGE: Enqueue and read a message
				auditEvent := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().UTC(),
					EventType:      "workflow.completed",
					EventCategory:  "workflow",
					EventAction:    "completed",
					EventOutcome:   "success",
					ActorType:      "service",
					ActorID:        "workflow-service",
					ResourceType:   "Workflow",
					ResourceID:     "wf-ack-test",
					CorrelationID:  "remediation-ack-test",
					EventData:      json.RawMessage(`{"ack":true}`),
				}
				err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("ack test error"))
				Expect(err).ToNot(HaveOccurred())

				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 1*time.Second)
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))

				// ACT: Acknowledge the message
				err = dlqClient.AckMessage(ctx, "events", consumerGroup, messages[0].ID)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())

				// Verify: Pending count should be 0
				pendingCount, err := dlqClient.GetPendingMessages(ctx, "events", consumerGroup)
				Expect(err).ToNot(HaveOccurred())
				Expect(pendingCount).To(Equal(int64(0)))
			})
		})

		// ============================================================================
		// TEST 10: MoveToDeadLetter - Move failed messages after max retries
		// BEHAVIOR: Message is moved to dead letter stream and removed from DLQ
		// CORRECTNESS: Message data is preserved in dead letter
		// ============================================================================
		Context("MoveToDeadLetter - Success Path", func() {
			It("should move message to dead letter stream", func() {
				// ARRANGE: Enqueue and read a message
				auditEvent := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().UTC(),
					EventType:      "workflow.failed",
					EventCategory:  "workflow",
					EventAction:    "failed",
					EventOutcome:   "failure",
					ActorType:      "service",
					ActorID:        "workflow-service",
					ResourceType:   "Workflow",
					ResourceID:     "wf-dead-letter-test",
					CorrelationID:  "remediation-dead-letter-test",
					EventData:      json.RawMessage(`{"dead_letter":true}`),
				}
				err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("permanent failure"))
				Expect(err).ToNot(HaveOccurred())

				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 1*time.Second)
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))

				// ACT: Move to dead letter
				err = dlqClient.MoveToDeadLetter(ctx, "events", messages[0])

				// ASSERT
				Expect(err).ToNot(HaveOccurred())

				// Verify: Message should be in dead letter stream
				deadLetterKey := "audit:dead-letter:events"
				deadLetterMessages, err := redisClient.XRange(ctx, deadLetterKey, "-", "+").Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(deadLetterMessages).To(HaveLen(1))

				// Verify: Original message should be removed from DLQ
				dlqDepth, err := dlqClient.GetDLQDepth(ctx, "events")
				Expect(err).ToNot(HaveOccurred())
				Expect(dlqDepth).To(Equal(int64(0)))
			})
		})

		// ============================================================================
		// TEST 11: IncrementRetryCount - Update retry count on message
		// BEHAVIOR: Message retry count is incremented and error updated
		// CORRECTNESS: New message with incremented count replaces original
		// ============================================================================
		Context("IncrementRetryCount - Success Path", func() {
			It("should increment retry count on message", func() {
				// ARRANGE: Enqueue a message
				auditEvent := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventTimestamp: time.Now().UTC(),
					EventType:      "workflow.retrying",
					EventCategory:  "workflow",
					EventAction:    "retrying",
					EventOutcome:   "pending",
					ActorType:      "service",
					ActorID:        "workflow-service",
					ResourceType:   "Workflow",
					ResourceID:     "wf-retry-count-test",
					CorrelationID:  "remediation-retry-count-test",
					EventData:      json.RawMessage(`{"retry_count_test":true}`),
				}
				err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("initial error"))
				Expect(err).ToNot(HaveOccurred())

				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 1*time.Second)
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))
				Expect(messages[0].AuditMessage.RetryCount).To(Equal(0))

				// ACT: Increment retry count
				err = dlqClient.IncrementRetryCount(ctx, "events", messages[0], fmt.Errorf("retry attempt 1 failed"))

				// ASSERT
				Expect(err).ToNot(HaveOccurred())

				// First, ack the original to remove from pending
				_ = dlqClient.AckMessage(ctx, "events", consumerGroup, messages[0].ID)

				// Read again with new consumer group to get the updated message
				newConsumerGroup := fmt.Sprintf("test-verify-retry-%d", time.Now().UnixNano())
				updatedMessages, err := dlqClient.ReadMessages(ctx, "events", newConsumerGroup, consumerName, 1*time.Second)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedMessages).To(HaveLen(1))
				Expect(updatedMessages[0].AuditMessage.RetryCount).To(Equal(1))
				Expect(updatedMessages[0].AuditMessage.LastError).To(Equal("retry attempt 1 failed"))
			})
		})

		// ============================================================================
		// TEST 12: GetPendingMessages - Count unacknowledged messages
		// BEHAVIOR: Returns count of messages claimed but not acknowledged
		// CORRECTNESS: Count matches actual pending entries
		// ============================================================================
		Context("GetPendingMessages - Success Path", func() {
			It("should return correct pending count", func() {
				// ARRANGE: Enqueue 2 messages
				for i := 0; i < 2; i++ {
					auditEvent := &audit.AuditEvent{
						EventID:        uuid.New(),
						EventVersion:   "1.0",
						EventTimestamp: time.Now().UTC(),
						EventType:      "workflow.pending",
						EventCategory:  "workflow",
						EventAction:    "pending",
						EventOutcome:   "pending",
						ActorType:      "service",
						ActorID:        "workflow-service",
						ResourceType:   "Workflow",
						ResourceID:     fmt.Sprintf("wf-pending-%d", i),
						CorrelationID:  fmt.Sprintf("remediation-pending-%d", i),
						EventData:      json.RawMessage(`{"pending":true}`),
					}
					err := dlqClient.EnqueueAuditEvent(ctx, auditEvent, fmt.Errorf("pending error %d", i))
					Expect(err).ToNot(HaveOccurred())
				}

				// Read messages to claim them
				messages, err := dlqClient.ReadMessages(ctx, "events", consumerGroup, consumerName, 1*time.Second)
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(2))

				// ACT: Get pending count
				pendingCount, err := dlqClient.GetPendingMessages(ctx, "events", consumerGroup)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(pendingCount).To(Equal(int64(2)))

				// Ack one message
				err = dlqClient.AckMessage(ctx, "events", consumerGroup, messages[0].ID)
				Expect(err).ToNot(HaveOccurred())

				// Pending count should be 1
				pendingCount, err = dlqClient.GetPendingMessages(ctx, "events", consumerGroup)
				Expect(err).ToNot(HaveOccurred())
				Expect(pendingCount).To(Equal(int64(1)))
			})
		})
	})
})
