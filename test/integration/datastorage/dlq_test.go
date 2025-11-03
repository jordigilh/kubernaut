package datastorage

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

var _ = Describe("DLQ Client Integration", func() {
	var audit *models.NotificationAudit
	var testError error

	BeforeEach(func() {
		// Clean up DLQ
		streamKey := "audit:dlq:notification"
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
				streamKey := "audit:dlq:notification"
				length, err := redisClient.XLen(ctx, streamKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(length).To(Equal(int64(1)))

				// Read and verify message content
				messages, err := redisClient.XRange(ctx, streamKey, "-", "+").Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))

				// Verify message structure
				messageJSON := messages[0].Values["message"].(string)
				var auditMsg map[string]interface{}
				err = json.Unmarshal([]byte(messageJSON), &auditMsg)
				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS TEST: Message fields
				Expect(auditMsg["type"]).To(Equal("notification_audit"))
				Expect(auditMsg["retry_count"]).To(BeNumerically("==", 0))
				Expect(auditMsg["last_error"]).To(Equal(testError.Error()))
				Expect(auditMsg["timestamp"]).ToNot(BeEmpty())

				// ✅ CORRECTNESS TEST: Payload contains audit data
				payloadJSON, err := json.Marshal(auditMsg["payload"])
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
				streamKey := "audit:dlq:notification"
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
				depth, err := dlqClient.GetDLQDepth(ctx, "notification")

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
				depth, err := dlqClient.GetDLQDepth(ctx, "notification")

				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS TEST: Depth matches Redis XLEN
				Expect(depth).To(Equal(int64(3)))

				// ✅ CORRECTNESS TEST: Verify with direct Redis query
				streamKey := "audit:dlq:notification"
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
})

