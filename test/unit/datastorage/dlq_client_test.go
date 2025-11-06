package datastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// func TestDLQClient(t *testing.T) {
// 	RegisterFailHandler(Fail)
// 	RunSpecs(t, "...")
// }

var _ = Describe("DLQClient", func() {
	var (
		client      *dlq.Client
		redisClient *redis.Client
		miniRedis   *miniredis.Miniredis
		ctx         context.Context
		logger      *zap.Logger
		audit       *models.NotificationAudit
		testError   error
	)

	BeforeEach(func() {
		miniRedis = miniredis.RunT(GinkgoT())

		redisClient = redis.NewClient(&redis.Options{
			Addr: miniRedis.Addr(),
		})

		logger = zap.NewNop()
		client = dlq.NewClient(redisClient, logger)
		ctx = context.Background()
		testError = Errorf("database connection failed")

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
	})

	AfterEach(func() {
		redisClient.Close()
		miniRedis.Close()
	})

	Describe("EnqueueNotificationAudit", func() {
		Context("with valid audit record", func() {
			It("should enqueue successfully to Redis Stream", func() {
				err := client.EnqueueNotificationAudit(ctx, audit, testError)

				Expect(err).ToNot(HaveOccurred())

				// Verify message was added to stream
				streamKey := "audit:dlq:notification"
				length, err := redisClient.XLen(ctx, streamKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(length).To(Equal(int64(1)))

				// Read and verify message content
				messages, err := redisClient.XRange(ctx, streamKey, "-", "+").Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(messages).To(HaveLen(1))

				messageJSON := messages[0].Values["message"].(string)
				var auditMsg dlq.AuditMessage
				err = json.Unmarshal([]byte(messageJSON), &auditMsg)
				Expect(err).ToNot(HaveOccurred())

				Expect(auditMsg.Type).To(Equal("notification_audit"))
				Expect(auditMsg.RetryCount).To(Equal(0))
				Expect(auditMsg.LastError).To(Equal(testError.Error()))
				Expect(auditMsg.Timestamp).ToNot(BeZero())

				// Verify payload
				var payloadAudit models.NotificationAudit
				err = json.Unmarshal(auditMsg.Payload, &payloadAudit)
				Expect(err).ToNot(HaveOccurred())
				Expect(payloadAudit.NotificationID).To(Equal(audit.NotificationID))
				Expect(payloadAudit.RemediationID).To(Equal(audit.RemediationID))
			})

			It("should handle multiple enqueues", func() {
				// Enqueue first audit
				err := client.EnqueueNotificationAudit(ctx, audit, testError)
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
				err = client.EnqueueNotificationAudit(ctx, audit2, testError)
				Expect(err).ToNot(HaveOccurred())

				// Verify both messages in stream
				streamKey := "audit:dlq:notification"
				length, err := redisClient.XLen(ctx, streamKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(length).To(Equal(int64(2)))
			})
		})

		Context("with Redis errors", func() {
			It("should return error when Redis is unavailable", func() {
				// Close Redis to simulate unavailability
				miniRedis.Close()

				err := client.EnqueueNotificationAudit(ctx, audit, testError)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to add to DLQ"))
			})
		})
	})

	Describe("GetDLQDepth", func() {
		Context("with empty DLQ", func() {
			It("should return 0", func() {
				depth, err := client.GetDLQDepth(ctx, "notification")

				Expect(err).ToNot(HaveOccurred())
				Expect(depth).To(Equal(int64(0)))
			})
		})

		Context("with messages in DLQ", func() {
			It("should return correct depth", func() {
				// Enqueue 3 messages
				for i := 0; i < 3; i++ {
					audit.NotificationID = fmt.Sprintf("test-notification-%d", i)
					err := client.EnqueueNotificationAudit(ctx, audit, testError)
					Expect(err).ToNot(HaveOccurred())
				}

				depth, err := client.GetDLQDepth(ctx, "notification")

				Expect(err).ToNot(HaveOccurred())
				Expect(depth).To(Equal(int64(3)))
			})
		})

		Context("with Redis errors", func() {
			It("should return error when Redis is unavailable", func() {
				miniRedis.Close()

				depth, err := client.GetDLQDepth(ctx, "notification")

				Expect(err).To(HaveOccurred())
				Expect(depth).To(Equal(int64(0)))
				Expect(err.Error()).To(ContainSubstring("failed to get DLQ depth"))
			})
		})
	})

	Describe("HealthCheck", func() {
		Context("when Redis is healthy", func() {
			It("should return no error", func() {
				err := client.HealthCheck(ctx)

				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when Redis is unhealthy", func() {
			It("should return error", func() {
				miniRedis.Close()

				err := client.HealthCheck(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("health check failed"))
			})
		})
	})
})

// Helper function to create errors
func Errorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}

