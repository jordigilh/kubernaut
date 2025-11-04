package datastorage

import (
	"context"
	"database/sql"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn" // DD-010: Migrated from lib/pq
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// func TestNotificationAuditRepository(t *testing.T) {
// 	RegisterFailHandler(Fail)
// 	RunSpecs(t, "...")
// }

var _ = Describe("NotificationAuditRepository", func() {
	var (
		repo   *repository.NotificationAuditRepository
		mockDB *sql.DB
		mock   sqlmock.Sqlmock
		ctx    context.Context
		logger *zap.Logger
		audit  *models.NotificationAudit
		now    time.Time
	)

	BeforeEach(func() {
		var err error
		mockDB, mock, err = sqlmock.New(sqlmock.MonitorPingsOption(true))
		Expect(err).ToNot(HaveOccurred())

		logger = zap.NewNop()
		repo = repository.NewNotificationAuditRepository(mockDB, logger)
		ctx = context.Background()
		now = time.Now()

		audit = &models.NotificationAudit{
			RemediationID:   "test-remediation-1",
			NotificationID:  "test-notification-1",
			Recipient:       "test@example.com",
			Channel:         "email",
			MessageSummary:  "Test notification message",
			Status:          "sent",
			SentAt:          now,
			DeliveryStatus:  "200 OK",
			ErrorMessage:    "",
			EscalationLevel: 0,
		}
	})

	AfterEach(func() {
		mockDB.Close()
	})

	Describe("Create", func() {
		Context("with valid audit record", func() {
			It("should insert successfully and return audit with ID", func() {
				expectedID := int64(123)
				expectedCreatedAt := now
				expectedUpdatedAt := now

				mock.ExpectQuery(`INSERT INTO notification_audit`).
					WithArgs(
						audit.RemediationID,
						audit.NotificationID,
						audit.Recipient,
						audit.Channel,
						audit.MessageSummary,
						audit.Status,
						audit.SentAt,
						sql.NullString{String: audit.DeliveryStatus, Valid: true},
						sql.NullString{String: audit.ErrorMessage, Valid: false},
						audit.EscalationLevel,
					).
					WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
						AddRow(expectedID, expectedCreatedAt, expectedUpdatedAt))

				// ACT: Create notification audit
				result, err := repo.Create(ctx, audit)

				// CORRECTNESS: Create succeeds
				Expect(err).ToNot(HaveOccurred(), "Create should succeed")

				// CORRECTNESS: Database-generated fields have exact expected values
				Expect(result.ID).To(Equal(expectedID), "ID should match database-generated value")
				Expect(result.CreatedAt).To(Equal(expectedCreatedAt), "CreatedAt should match database timestamp")
				Expect(result.UpdatedAt).To(Equal(expectedUpdatedAt), "UpdatedAt should match database timestamp")

				// CORRECTNESS: All SQL expectations met
				Expect(mock.ExpectationsWereMet()).To(Succeed(), "All SQL expectations should be met")
			})

			It("should handle empty optional fields", func() {
				audit.DeliveryStatus = ""
				audit.ErrorMessage = ""

				expectedID := int64(124)

				mock.ExpectQuery(`INSERT INTO notification_audit`).
					WithArgs(
						audit.RemediationID,
						audit.NotificationID,
						audit.Recipient,
						audit.Channel,
						audit.MessageSummary,
						audit.Status,
						audit.SentAt,
						sql.NullString{String: "", Valid: false},
						sql.NullString{String: "", Valid: false},
						audit.EscalationLevel,
					).
					WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
						AddRow(expectedID, now, now))

				// ACT: Create notification audit with empty optional fields
				result, err := repo.Create(ctx, audit)

				// CORRECTNESS: Create succeeds even with empty optional fields
				Expect(err).ToNot(HaveOccurred(), "Create should succeed with empty optional fields")

				// CORRECTNESS: ID is populated by database
				Expect(result.ID).To(Equal(expectedID), "ID should match database-generated value")

				// CORRECTNESS: All SQL expectations met
				Expect(mock.ExpectationsWereMet()).To(Succeed(), "All SQL expectations should be met")
			})
		})

		Context("with validation errors", func() {
			It("should fail validation for empty remediation_id", func() {
				audit.RemediationID = ""

				result, err := repo.Create(ctx, audit)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
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

			It("should fail validation for invalid channel", func() {
				audit.Channel = "invalid"

				result, err := repo.Create(ctx, audit)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				validationErr, ok := err.(*validation.ValidationError)
				Expect(ok).To(BeTrue())
				Expect(validationErr.FieldErrors).To(HaveKey("channel"))
			})
		})

		Context("with database errors", func() {
			It("should handle unique constraint violation", func() {
				mock.ExpectQuery(`INSERT INTO notification_audit`).
					WithArgs(
						audit.RemediationID,
						audit.NotificationID,
						audit.Recipient,
						audit.Channel,
						audit.MessageSummary,
						audit.Status,
						audit.SentAt,
						sql.NullString{String: audit.DeliveryStatus, Valid: true},
						sql.NullString{String: audit.ErrorMessage, Valid: false},
						audit.EscalationLevel,
					).
					WillReturnError(&pgconn.PgError{Code: "23505"}) // unique_violation (DD-010)

				result, err := repo.Create(ctx, audit)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				problem, ok := err.(*validation.RFC7807Problem)
				Expect(ok).To(BeTrue())
				Expect(problem.Status).To(Equal(409)) // Conflict
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})

			It("should handle generic database errors", func() {
				mock.ExpectQuery(`INSERT INTO notification_audit`).
					WithArgs(
						audit.RemediationID,
						audit.NotificationID,
						audit.Recipient,
						audit.Channel,
						audit.MessageSummary,
						audit.Status,
						audit.SentAt,
						sql.NullString{String: audit.DeliveryStatus, Valid: true},
						sql.NullString{String: audit.ErrorMessage, Valid: false},
						audit.EscalationLevel,
					).
					WillReturnError(sql.ErrConnDone)

				result, err := repo.Create(ctx, audit)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to insert"))
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})
	})

	Describe("GetByNotificationID", func() {
		Context("when record exists", func() {
			It("should retrieve the audit record", func() {
				expectedID := int64(123)
				notificationID := "test-notification-1"

				mock.ExpectQuery(`SELECT (.+) FROM notification_audit WHERE notification_id`).
					WithArgs(notificationID).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "remediation_id", "notification_id", "recipient", "channel",
						"message_summary", "status", "sent_at", "delivery_status", "error_message",
						"escalation_level", "created_at", "updated_at",
					}).AddRow(
						expectedID, "test-remediation-1", notificationID, "test@example.com", "email",
						"Test message", "sent", now, "200 OK", nil,
						0, now, now,
					))

				// ACT: Get notification audit by ID
				result, err := repo.GetByNotificationID(ctx, notificationID)

				// CORRECTNESS: Query succeeds
				Expect(err).ToNot(HaveOccurred(), "GetByNotificationID should succeed")

				// CORRECTNESS: All fields have expected values
				Expect(result.ID).To(Equal(expectedID), "ID should match queried record")
				Expect(result.NotificationID).To(Equal(notificationID), "NotificationID should match query parameter")
				Expect(result.DeliveryStatus).To(Equal("200 OK"), "DeliveryStatus should be populated")
				Expect(result.ErrorMessage).To(BeEmpty(), "ErrorMessage should be empty for successful notification")

				// CORRECTNESS: All SQL expectations met
				Expect(mock.ExpectationsWereMet()).To(Succeed(), "All SQL expectations should be met")
			})

			It("should handle nullable fields correctly", func() {
				expectedID := int64(124)
				notificationID := "test-notification-2"

				mock.ExpectQuery(`SELECT (.+) FROM notification_audit WHERE notification_id`).
					WithArgs(notificationID).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "remediation_id", "notification_id", "recipient", "channel",
						"message_summary", "status", "sent_at", "delivery_status", "error_message",
						"escalation_level", "created_at", "updated_at",
					}).AddRow(
						expectedID, "test-remediation-2", notificationID, "test2@example.com", "slack",
						"Test message 2", "failed", now, nil, "Connection timeout",
						1, now, now,
					))

				// ACT: Get notification audit with nullable fields
				result, err := repo.GetByNotificationID(ctx, notificationID)

				// CORRECTNESS: Query succeeds with NULL fields
				Expect(err).ToNot(HaveOccurred(), "GetByNotificationID should succeed with NULL fields")

				// CORRECTNESS: Nullable fields are properly handled (NULL → empty string, non-NULL → value)
				Expect(result.ID).To(Equal(expectedID), "ID should match queried record")
				Expect(result.NotificationID).To(Equal(notificationID), "NotificationID should match query parameter")
				Expect(result.DeliveryStatus).To(BeEmpty(), "DeliveryStatus should be empty (NULL in DB)")
				Expect(result.ErrorMessage).To(Equal("Connection timeout"), "ErrorMessage should be populated")

				// CORRECTNESS: All SQL expectations met
				Expect(mock.ExpectationsWereMet()).To(Succeed(), "All SQL expectations should be met")
			})
		})

		Context("when record does not exist", func() {
			It("should return not found error", func() {
				notificationID := "non-existent-id"

				mock.ExpectQuery(`SELECT (.+) FROM notification_audit WHERE notification_id`).
					WithArgs(notificationID).
					WillReturnError(sql.ErrNoRows)

				result, err := repo.GetByNotificationID(ctx, notificationID)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				problem, ok := err.(*validation.RFC7807Problem)
				Expect(ok).To(BeTrue())
				Expect(problem.Status).To(Equal(404)) // Not Found
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("with database errors", func() {
			It("should handle generic database errors", func() {
				notificationID := "test-notification-1"

				mock.ExpectQuery(`SELECT (.+) FROM notification_audit WHERE notification_id`).
					WithArgs(notificationID).
					WillReturnError(sql.ErrConnDone)

				result, err := repo.GetByNotificationID(ctx, notificationID)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to retrieve"))
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})
	})

	Describe("HealthCheck", func() {
		Context("when database is healthy", func() {
			It("should return no error", func() {
				mock.ExpectPing()

				err := repo.HealthCheck(ctx)

				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("when database is unhealthy", func() {
			It("should return error", func() {
				mock.ExpectPing().WillReturnError(sql.ErrConnDone)

				err := repo.HealthCheck(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("health check failed"))
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})
	})
})
