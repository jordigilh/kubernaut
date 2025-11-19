package datastorage

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// ========================================
// REPOSITORY INTEGRATION TESTS (TDD RED Phase)
// 📋 Tests Define Contract: Real PostgreSQL required
// Authority: IMPLEMENTATION_PLAN_V4.8.md Day 7
// ========================================
//
// This file defines the repository integration test contract.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (this file)
// - Infrastructure implemented SECOND (suite_test.go BeforeSuite)
// - Contract: Behavior + Correctness validation
//
// Business Requirements:
// - BR-STORAGE-001: Persist notification audit
// - BR-STORAGE-010: Input validation
// - BR-STORAGE-017: RFC 7807 errors
//
// Context API Lessons Applied:
// - Test BOTH behavior AND correctness
// - Verify data in database matches input exactly
// - Test RFC 7807 error responses
//
// ========================================

var _ = Describe("NotificationAudit Repository Integration", func() {
	var audit *models.NotificationAudit
	var testID string

	BeforeEach(func() {
		// Generate unique test ID for parallel execution isolation
		testID = generateTestID()

		// Clean up only this test's data (targeted DELETE for parallel safety)
		_, err := db.ExecContext(ctx, "DELETE FROM notification_audit WHERE remediation_id LIKE $1", fmt.Sprintf("%%-%s", testID))
		Expect(err).ToNot(HaveOccurred())

		// Create test audit with unique IDs
		audit = &models.NotificationAudit{
			RemediationID:   fmt.Sprintf("rr-%s", testID),
			NotificationID:  fmt.Sprintf("notif-%s", testID),
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

	Describe("Create", func() {
		Context("with valid audit record", func() {
			It("should persist audit to real PostgreSQL (Behavior + Correctness)", func() {
				// ✅ BEHAVIOR TEST: Create returns audit with ID
				result, err := repo.Create(ctx, audit)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.ID).To(BeNumerically(">", 0))
				Expect(result.CreatedAt).ToNot(BeZero())
				Expect(result.UpdatedAt).ToNot(BeZero())

				// ✅ CORRECTNESS TEST: Data in database matches input exactly
				var dbRemediationID, dbNotificationID, dbRecipient, dbChannel, dbMessageSummary, dbStatus, dbDeliveryStatus string
				var dbSentAt time.Time
				var dbEscalationLevel int

				row := db.QueryRowContext(ctx, `
					SELECT remediation_id, notification_id, recipient, channel, message_summary,
					       status, sent_at, delivery_status, escalation_level
					FROM notification_audit
					WHERE id = $1
				`, result.ID)

				err = row.Scan(&dbRemediationID, &dbNotificationID, &dbRecipient, &dbChannel,
					&dbMessageSummary, &dbStatus, &dbSentAt, &dbDeliveryStatus, &dbEscalationLevel)

				Expect(err).ToNot(HaveOccurred())
				Expect(dbRemediationID).To(Equal(audit.RemediationID))
				Expect(dbNotificationID).To(Equal(audit.NotificationID))
				Expect(dbRecipient).To(Equal(audit.Recipient))
				Expect(dbChannel).To(Equal(audit.Channel))
				Expect(dbMessageSummary).To(Equal(audit.MessageSummary))
				Expect(dbStatus).To(Equal(audit.Status))
				Expect(dbSentAt.Unix()).To(BeNumerically("~", audit.SentAt.Unix(), 1))
				Expect(dbDeliveryStatus).To(Equal(audit.DeliveryStatus))
				Expect(dbEscalationLevel).To(Equal(audit.EscalationLevel))
			})

			It("should handle empty optional fields correctly", func() {
				audit.DeliveryStatus = ""
				audit.ErrorMessage = ""

				result, err := repo.Create(ctx, audit)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())

				// ✅ CORRECTNESS TEST: NULL fields in database
				var dbDeliveryStatus, dbErrorMessage *string
				row := db.QueryRowContext(ctx, `
					SELECT delivery_status, error_message
					FROM notification_audit
					WHERE id = $1
				`, result.ID)

				err = row.Scan(&dbDeliveryStatus, &dbErrorMessage)
				Expect(err).ToNot(HaveOccurred())
				Expect(dbDeliveryStatus).To(BeNil())
				Expect(dbErrorMessage).To(BeNil())
			})
		})

		Context("with validation errors", func() {
			It("should fail validation for empty remediation_id", func() {
				audit.RemediationID = ""

				result, err := repo.Create(ctx, audit)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())

				// ✅ BEHAVIOR TEST: Returns ValidationError
				validationErr, ok := err.(*validation.ValidationError)
				Expect(ok).To(BeTrue())
				Expect(validationErr.FieldErrors).To(HaveKey("remediation_id"))
			})

			It("should fail validation for empty notification_id", func() {
				audit.NotificationID = ""

				result, err := repo.Create(ctx, audit)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())

				validationErr, ok := err.(*validation.ValidationError)
				Expect(ok).To(BeTrue())
				Expect(validationErr.FieldErrors).To(HaveKey("notification_id"))
			})
		})

		Context("with database errors", func() {
			It("should handle unique constraint violation (RFC 7807)", func() {
				// Insert first audit
				_, err := repo.Create(ctx, audit)
				Expect(err).ToNot(HaveOccurred())

				// Try to insert duplicate notification_id
				result, err := repo.Create(ctx, audit)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())

				// ✅ BEHAVIOR TEST: Returns RFC7807Problem with 409 Conflict
				problem, ok := err.(*validation.RFC7807Problem)
				Expect(ok).To(BeTrue())
				Expect(problem.Status).To(Equal(409))
				Expect(problem.Type).To(Equal("https://kubernaut.io/errors/conflict"))
			})
		})
	})

	Describe("GetByNotificationID", func() {
		Context("when record exists", func() {
			It("should retrieve the audit record (Behavior + Correctness)", func() {
				// Insert test audit
				created, err := repo.Create(ctx, audit)
				Expect(err).ToNot(HaveOccurred())

				// ✅ BEHAVIOR TEST: GetByNotificationID returns audit
				result, err := repo.GetByNotificationID(ctx, audit.NotificationID)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())

				// ✅ CORRECTNESS TEST: All fields match created audit
				Expect(result.ID).To(Equal(created.ID))
				Expect(result.RemediationID).To(Equal(created.RemediationID))
				Expect(result.NotificationID).To(Equal(created.NotificationID))
				Expect(result.Recipient).To(Equal(created.Recipient))
				Expect(result.Channel).To(Equal(created.Channel))
				Expect(result.MessageSummary).To(Equal(created.MessageSummary))
				Expect(result.Status).To(Equal(created.Status))
				Expect(result.SentAt.Unix()).To(BeNumerically("~", created.SentAt.Unix(), 1))
				Expect(result.DeliveryStatus).To(Equal(created.DeliveryStatus))
				Expect(result.EscalationLevel).To(Equal(created.EscalationLevel))
			})

			It("should handle nullable fields correctly", func() {
				audit.DeliveryStatus = ""
				audit.ErrorMessage = "Connection timeout"

				_, err := repo.Create(ctx, audit)
				Expect(err).ToNot(HaveOccurred())

				result, err := repo.GetByNotificationID(ctx, audit.NotificationID)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.DeliveryStatus).To(BeEmpty())
				Expect(result.ErrorMessage).To(Equal("Connection timeout"))
			})
		})

		Context("when record does not exist", func() {
			It("should return not found error (RFC 7807)", func() {
				result, err := repo.GetByNotificationID(ctx, "non-existent-id")

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())

				// ✅ BEHAVIOR TEST: Returns RFC7807Problem with 404 Not Found
				problem, ok := err.(*validation.RFC7807Problem)
				Expect(ok).To(BeTrue())
				Expect(problem.Status).To(Equal(404))
				Expect(problem.Type).To(Equal("https://kubernaut.io/errors/not-found"))
			})
		})
	})

	Describe("HealthCheck", func() {
		It("should verify database connectivity", func() {
			err := repo.HealthCheck(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
