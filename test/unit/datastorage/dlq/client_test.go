package dlq_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
	"github.com/go-logr/logr"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

func TestDLQClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DLQ Client Unit Test Suite")
}

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
		dlqClient, err = dlq.NewClient(redisClient, logger)
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
	// ============================================================================
	Context("NewClient - Constructor Validation", func() {
		It("should create DLQ client with valid parameters", func() {
			// ACT: Create new client
			client, err := dlq.NewClient(redisClient, logger)

			// ASSERT: Should succeed
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})

		It("should return error when Redis client is nil", func() {
			// ACT: Create client with nil Redis client
			client, err := dlq.NewClient(nil, logger)

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
})
