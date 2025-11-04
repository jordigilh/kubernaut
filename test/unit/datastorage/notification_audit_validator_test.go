package datastorage

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// func TestValidation(t *testing.T) {
// 	RegisterFailHandler(Fail)
// 	RunSpecs(t, "...")
// }

// ========================================
// NOTIFICATION AUDIT VALIDATOR (BR-STORAGE-006)
// TESTING PRINCIPLE: Behavior + Correctness (Implementation Plan V4.9)
// ========================================
var _ = Describe("NotificationAuditValidator", func() {
	var (
		validator *validation.NotificationAuditValidator
		audit     *models.NotificationAudit
	)

	BeforeEach(func() {
		validator = validation.NewNotificationAuditValidator()
		now := time.Now()
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

	Context("Valid Audit Records", func() {
		// BEHAVIOR: Validator accepts complete, well-formed notification audit records
		// CORRECTNESS: Validation returns no error for valid inputs
		It("should pass validation for a complete valid record", func() {
			// ACT: Validate complete audit
			err := validator.Validate(audit)

			// CORRECTNESS: No error returned
			Expect(err).ToNot(HaveOccurred(), "Complete valid audit should pass validation")
		})

		// BEHAVIOR: Validator accepts minimal required fields (optional fields omitted)
		// CORRECTNESS: DeliveryStatus and ErrorMessage can be empty strings
		It("should pass validation with minimal required fields", func() {
			// ARRANGE: Remove optional fields
			audit.DeliveryStatus = ""
			audit.ErrorMessage = ""

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error returned for minimal valid audit
			Expect(err).ToNot(HaveOccurred(), "Minimal valid audit should pass validation")
		})

		// BEHAVIOR: Validator accepts all defined status values
		// CORRECTNESS: All status enum values are recognized as valid
		DescribeTable("should accept all valid status values",
			func(status string) {
				// ARRANGE: Set status
				audit.Status = status

				// ACT: Validate
				err := validator.Validate(audit)

				// CORRECTNESS: Status is valid
				Expect(err).ToNot(HaveOccurred(), "status %q should be valid", status)
			},
			Entry("status: sent", "sent"),
			Entry("status: failed", "failed"),
			Entry("status: acknowledged", "acknowledged"),
			Entry("status: escalated", "escalated"),
		)

		// BEHAVIOR: Validator accepts all defined channel values
		// CORRECTNESS: All channel enum values are recognized as valid
		DescribeTable("should accept all valid channel values",
			func(channel string) {
				// ARRANGE: Set channel
				audit.Channel = channel

				// ACT: Validate
				err := validator.Validate(audit)

				// CORRECTNESS: Channel is valid
				Expect(err).ToNot(HaveOccurred(), "channel %q should be valid", channel)
			},
			Entry("channel: email", "email"),
			Entry("channel: slack", "slack"),
			Entry("channel: pagerduty", "pagerduty"),
			Entry("channel: sms", "sms"),
		)

		// BEHAVIOR: Validator accepts escalation levels from 0 to 100
		// CORRECTNESS: Boundary value 100 is valid
		It("should accept escalation level at upper boundary (100)", func() {
			// ARRANGE: Set escalation level to maximum
			audit.EscalationLevel = 100

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Maximum escalation level is valid
			Expect(err).ToNot(HaveOccurred(), "escalation_level 100 should be valid")
		})
	})

	Context("Nil Audit Record", func() {
		// BEHAVIOR: Validator rejects nil input
		// CORRECTNESS: Error message explicitly mentions "cannot be nil"
		It("should reject nil audit with explicit error message", func() {
			// ACT: Validate nil audit
			err := validator.Validate(nil)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Nil audit should be rejected")

			// CORRECTNESS: Error message is specific
			Expect(err.Error()).To(ContainSubstring("cannot be nil"),
				"Error should explicitly mention nil audit")
		})
	})

	Context("RemediationID Validation", func() {
		// BEHAVIOR: Validator rejects empty remediation_id (required field)
		// CORRECTNESS: ValidationError contains specific field error for remediation_id
		It("should reject empty remediation_id with field-specific error", func() {
			// ARRANGE: Empty remediation_id
			audit.RemediationID = ""

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Validation failed
			Expect(err).To(HaveOccurred(), "Empty remediation_id should be rejected")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error mentions "required"
			Expect(valErr.FieldErrors["remediation_id"]).To(ContainSubstring("required"),
				"Error should specify remediation_id is required")
		})

		// BEHAVIOR: Validator rejects whitespace-only remediation_id (treated as empty)
		// CORRECTNESS: Whitespace is normalized/trimmed, then validated as empty
		It("should reject whitespace-only remediation_id (treated as empty)", func() {
			// ARRANGE: Whitespace-only remediation_id
			audit.RemediationID = "   "

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Validation failed (whitespace treated as empty)
			Expect(err).To(HaveOccurred(), "Whitespace-only remediation_id should be rejected")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error mentions "required"
			Expect(valErr.FieldErrors["remediation_id"]).To(ContainSubstring("required"),
				"Error should specify remediation_id is required")
		})

		// BEHAVIOR: Validator enforces 255 character limit on remediation_id
		// CORRECTNESS: 256 characters exceeds limit and triggers validation error
		It("should reject remediation_id exceeding 255 character limit", func() {
			// ARRANGE: 256 character remediation_id (exceeds limit)
			audit.RemediationID = strings.Repeat("a", 256)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Validation failed
			Expect(err).To(HaveOccurred(), "256 character remediation_id should be rejected")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error mentions "255 characters" limit
			Expect(valErr.FieldErrors["remediation_id"]).To(ContainSubstring("255 characters"),
				"Error should specify 255 character limit")
		})

		// BEHAVIOR: Validator accepts remediation_id at boundary (255 characters)
		// CORRECTNESS: Exactly 255 characters is valid (boundary test)
		It("should accept remediation_id at 255 character boundary", func() {
			// ARRANGE: Exactly 255 character remediation_id
			audit.RemediationID = strings.Repeat("a", 255)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (255 is valid boundary)
			Expect(err).ToNot(HaveOccurred(), "255 character remediation_id should be valid")
		})
	})

	Context("NotificationID Validation", func() {
		// BEHAVIOR: Validator rejects empty notification_id (required field)
		// CORRECTNESS: ValidationError contains specific field error
		It("should reject empty notification_id with field-specific error", func() {
			// ARRANGE: Empty notification_id
			audit.NotificationID = ""

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Validation failed
			Expect(err).To(HaveOccurred(), "Empty notification_id should be rejected")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error mentions "required"
			Expect(valErr.FieldErrors["notification_id"]).To(ContainSubstring("required"),
				"Error should specify notification_id is required")
		})

		// BEHAVIOR: Validator rejects whitespace-only notification_id
		// CORRECTNESS: Whitespace is treated as empty
		It("should reject whitespace-only notification_id (treated as empty)", func() {
			// ARRANGE: Whitespace-only notification_id
			audit.NotificationID = "   "

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Validation failed
			Expect(err).To(HaveOccurred(), "Whitespace-only notification_id should be rejected")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error mentions "required"
			Expect(valErr.FieldErrors["notification_id"]).To(ContainSubstring("required"),
				"Error should specify notification_id is required")
		})

		// BEHAVIOR: Validator enforces 255 character limit
		// CORRECTNESS: 256 characters exceeds limit
		It("should reject notification_id exceeding 255 character limit", func() {
			// ARRANGE: 256 character notification_id
			audit.NotificationID = strings.Repeat("a", 256)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Validation failed
			Expect(err).To(HaveOccurred(), "256 character notification_id should be rejected")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error mentions limit
			Expect(valErr.FieldErrors["notification_id"]).To(ContainSubstring("255 characters"),
				"Error should specify 255 character limit")
		})

		// BEHAVIOR: Validator accepts notification_id at boundary (255 characters)
		// CORRECTNESS: Exactly 255 characters is valid
		It("should accept notification_id at 255 character boundary", func() {
			// ARRANGE: Exactly 255 character notification_id
			audit.NotificationID = strings.Repeat("a", 255)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (255 is valid)
			Expect(err).ToNot(HaveOccurred(), "255 character notification_id should be valid")
		})
	})

	Context("Recipient Validation", func() {
		// BEHAVIOR: Validator rejects empty recipient (required field)
		// CORRECTNESS: ValidationError contains specific field error
		It("should reject empty recipient with field-specific error", func() {
			// ARRANGE: Empty recipient
			audit.Recipient = ""

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Validation failed
			Expect(err).To(HaveOccurred(), "Empty recipient should be rejected")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error mentions "required"
			Expect(valErr.FieldErrors["recipient"]).To(ContainSubstring("required"),
				"Error should specify recipient is required")
		})

		// BEHAVIOR: Validator rejects whitespace-only recipient
		// CORRECTNESS: Whitespace is treated as empty
		It("should reject whitespace-only recipient (treated as empty)", func() {
			// ARRANGE: Whitespace-only recipient
			audit.Recipient = "   "

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Validation failed
			Expect(err).To(HaveOccurred(), "Whitespace-only recipient should be rejected")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error mentions "required"
			Expect(valErr.FieldErrors["recipient"]).To(ContainSubstring("required"),
				"Error should specify recipient is required")
		})

		// BEHAVIOR: Validator enforces 255 character limit
		// CORRECTNESS: 256 characters exceeds limit
		It("should reject recipient exceeding 255 character limit", func() {
			// ARRANGE: 256 character recipient
			audit.Recipient = strings.Repeat("a", 256)
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["recipient"]).To(ContainSubstring("255 characters"))
		})

		It("should pass validation for recipient at 255 characters", func() {
			audit.Recipient = strings.Repeat("a", 255)
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})
	})

	Context("Channel Validation", func() {
		It("should fail validation for empty channel", func() {
			audit.Channel = ""
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["channel"]).To(ContainSubstring("required"))
		})

		It("should fail validation for invalid channel", func() {
			audit.Channel = "invalid"
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["channel"]).To(ContainSubstring("must be one of"))
		})

		It("should fail validation for channel exceeding 50 characters", func() {
			audit.Channel = strings.Repeat("a", 51)
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["channel"]).To(ContainSubstring("50 characters"))
		})

		It("should accept case-insensitive channel values", func() {
			channels := []string{"EMAIL", "Email", "SLACK", "Slack", "PAGERDUTY", "PagerDuty", "SMS", "Sms"}
			for _, channel := range channels {
				audit.Channel = channel
				err := validator.Validate(audit)
				Expect(err).To(BeNil(), "channel '%s' should be valid (case-insensitive)", channel)
			}
		})
	})

	Context("MessageSummary Validation", func() {
		It("should fail validation for empty message_summary", func() {
			audit.MessageSummary = ""
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["message_summary"]).To(ContainSubstring("required"))
		})

		It("should fail validation for whitespace-only message_summary", func() {
			audit.MessageSummary = "   "
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["message_summary"]).To(ContainSubstring("required"))
		})

		It("should pass validation for long message_summary (TEXT type)", func() {
			audit.MessageSummary = strings.Repeat("a", 10000)
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})
	})

	Context("Status Validation", func() {
		It("should fail validation for empty status", func() {
			audit.Status = ""
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["status"]).To(ContainSubstring("required"))
		})

		It("should fail validation for invalid status", func() {
			audit.Status = "invalid"
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["status"]).To(ContainSubstring("must be one of"))
		})

		It("should fail validation for status exceeding 50 characters", func() {
			audit.Status = strings.Repeat("a", 51)
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["status"]).To(ContainSubstring("50 characters"))
		})

		It("should accept case-insensitive status values", func() {
			statuses := []string{"SENT", "Sent", "FAILED", "Failed", "ACKNOWLEDGED", "Acknowledged", "ESCALATED", "Escalated"}
			for _, status := range statuses {
				audit.Status = status
				err := validator.Validate(audit)
				Expect(err).To(BeNil(), "status '%s' should be valid (case-insensitive)", status)
			}
		})
	})

	Context("SentAt Validation", func() {
		It("should fail validation for zero sent_at", func() {
			audit.SentAt = time.Time{}
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["sent_at"]).To(ContainSubstring("required"))
		})

		It("should fail validation for future sent_at (beyond clock skew)", func() {
			audit.SentAt = time.Now().Add(10 * time.Minute)
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["sent_at"]).To(ContainSubstring("cannot be in the future"))
		})

		It("should pass validation for sent_at within clock skew (5 minutes)", func() {
			audit.SentAt = time.Now().Add(4 * time.Minute)
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})

		It("should pass validation for past sent_at", func() {
			audit.SentAt = time.Now().Add(-1 * time.Hour)
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})
	})

	Context("EscalationLevel Validation", func() {
		It("should fail validation for negative escalation_level", func() {
			audit.EscalationLevel = -1
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["escalation_level"]).To(ContainSubstring("non-negative"))
		})

		It("should fail validation for escalation_level exceeding 100", func() {
			audit.EscalationLevel = 101
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(valErr.FieldErrors["escalation_level"]).To(ContainSubstring("at most 100"))
		})

		It("should pass validation for escalation_level at 0", func() {
			audit.EscalationLevel = 0
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})

		It("should pass validation for escalation_level at 100", func() {
			audit.EscalationLevel = 100
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})
	})

	Context("Multiple Field Errors", func() {
		It("should report all field errors at once", func() {
			audit.RemediationID = ""
			audit.NotificationID = ""
			audit.Recipient = ""
			audit.Channel = "invalid"
			audit.MessageSummary = ""
			audit.Status = "invalid"
			audit.SentAt = time.Time{}
			audit.EscalationLevel = -1

			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Expected ValidationError type")
			Expect(len(valErr.FieldErrors)).To(Equal(8))
			Expect(valErr.FieldErrors).To(HaveKey("remediation_id"))
			Expect(valErr.FieldErrors).To(HaveKey("notification_id"))
			Expect(valErr.FieldErrors).To(HaveKey("recipient"))
			Expect(valErr.FieldErrors).To(HaveKey("channel"))
			Expect(valErr.FieldErrors).To(HaveKey("message_summary"))
			Expect(valErr.FieldErrors).To(HaveKey("status"))
			Expect(valErr.FieldErrors).To(HaveKey("sent_at"))
			Expect(valErr.FieldErrors).To(HaveKey("escalation_level"))
		})
	})

	Context("Optional Fields", func() {
		It("should pass validation with empty delivery_status", func() {
			audit.DeliveryStatus = ""
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})

		It("should pass validation with empty error_message", func() {
			audit.ErrorMessage = ""
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})

		It("should pass validation with long delivery_status (TEXT type)", func() {
			audit.DeliveryStatus = strings.Repeat("a", 10000)
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})

		It("should pass validation with long error_message (TEXT type)", func() {
			audit.ErrorMessage = strings.Repeat("a", 10000)
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})
	})
})

var _ = Describe("ValidationError", func() {
	var validationErr *validation.ValidationError

	BeforeEach(func() {
		validationErr = validation.NewValidationError("notification_audit", "validation failed")
	})

	Context("Error Creation", func() {
		It("should create a validation error with resource and message", func() {
			Expect(validationErr.Resource).To(Equal("notification_audit"))
			Expect(validationErr.Message).To(Equal("validation failed"))
			Expect(validationErr.FieldErrors).ToNot(BeNil())
			Expect(len(validationErr.FieldErrors)).To(Equal(0))
		})
	})

	Context("Field Errors", func() {
		It("should add field errors", func() {
			validationErr.AddFieldError("field1", "error1")
			validationErr.AddFieldError("field2", "error2")

			Expect(len(validationErr.FieldErrors)).To(Equal(2))
			Expect(validationErr.FieldErrors["field1"]).To(Equal("error1"))
			Expect(validationErr.FieldErrors["field2"]).To(Equal("error2"))
		})

		It("should overwrite existing field error", func() {
			validationErr.AddFieldError("field1", "error1")
			validationErr.AddFieldError("field1", "error2")

			Expect(len(validationErr.FieldErrors)).To(Equal(1))
			Expect(validationErr.FieldErrors["field1"]).To(Equal("error2"))
		})
	})

	Context("Error Interface", func() {
		It("should return error string without field errors", func() {
			errStr := validationErr.Error()
			Expect(errStr).To(ContainSubstring("notification_audit"))
			Expect(errStr).To(ContainSubstring("validation failed"))
		})

		It("should return error string with field errors", func() {
			validationErr.AddFieldError("field1", "error1")
			errStr := validationErr.Error()
			Expect(errStr).To(ContainSubstring("notification_audit"))
			Expect(errStr).To(ContainSubstring("validation failed"))
			Expect(errStr).To(ContainSubstring("fields"))
		})
	})

	Context("RFC 7807 Conversion", func() {
		It("should convert to RFC 7807 problem", func() {
			validationErr.AddFieldError("field1", "error1")
			validationErr.AddFieldError("field2", "error2")

			problem := validationErr.ToRFC7807()

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/validation-error"))
			Expect(problem.Title).To(Equal("Validation Error"))
			Expect(problem.Status).To(Equal(http.StatusBadRequest))
			Expect(problem.Detail).To(Equal("validation failed"))
			Expect(problem.Instance).To(Equal("/audit/notification_audit"))
			Expect(problem.Extensions["resource"]).To(Equal("notification_audit"))
			Expect(problem.Extensions["field_errors"]).To(Equal(validationErr.FieldErrors))
		})
	})
})

var _ = Describe("RFC7807Problem", func() {
	Context("Validation Error Problem", func() {
		It("should create validation error problem", func() {
			fieldErrors := map[string]string{
				"field1": "error1",
				"field2": "error2",
			}
			problem := validation.NewValidationErrorProblem("notification_audit", fieldErrors)

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/validation-error"))
			Expect(problem.Title).To(Equal("Validation Error"))
			Expect(problem.Status).To(Equal(http.StatusBadRequest))
			Expect(problem.Detail).To(ContainSubstring("notification_audit"))
			Expect(problem.Instance).To(Equal("/audit/notification_audit"))
			Expect(problem.Extensions["resource"]).To(Equal("notification_audit"))
			Expect(problem.Extensions["field_errors"]).To(Equal(fieldErrors))
		})
	})

	Context("Not Found Problem", func() {
		It("should create not found problem", func() {
			problem := validation.NewNotFoundProblem("notification_audit", "test-id-123")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/not-found"))
			Expect(problem.Title).To(Equal("Resource Not Found"))
			Expect(problem.Status).To(Equal(http.StatusNotFound))
			Expect(problem.Detail).To(ContainSubstring("test-id-123"))
			Expect(problem.Instance).To(Equal("/audit/notification_audit/test-id-123"))
			Expect(problem.Extensions["resource"]).To(Equal("notification_audit"))
			Expect(problem.Extensions["id"]).To(Equal("test-id-123"))
		})
	})

	Context("Internal Error Problem", func() {
		It("should create internal error problem", func() {
			problem := validation.NewInternalErrorProblem("database connection failed")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/internal-error"))
			Expect(problem.Title).To(Equal("Internal Server Error"))
			Expect(problem.Status).To(Equal(http.StatusInternalServerError))
			Expect(problem.Detail).To(Equal("database connection failed"))
			Expect(problem.Extensions["retry"]).To(BeTrue())
		})
	})

	Context("Service Unavailable Problem", func() {
		It("should create service unavailable problem", func() {
			problem := validation.NewServiceUnavailableProblem("database is down")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/service-unavailable"))
			Expect(problem.Title).To(Equal("Service Unavailable"))
			Expect(problem.Status).To(Equal(http.StatusServiceUnavailable))
			Expect(problem.Detail).To(Equal("database is down"))
			Expect(problem.Extensions["retry"]).To(BeTrue())
		})
	})

	Context("Conflict Problem", func() {
		It("should create conflict problem", func() {
			problem := validation.NewConflictProblem("notification_audit", "notification_id", "test-id-123")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/conflict"))
			Expect(problem.Title).To(Equal("Resource Conflict"))
			Expect(problem.Status).To(Equal(http.StatusConflict))
			Expect(problem.Detail).To(ContainSubstring("test-id-123"))
			Expect(problem.Instance).To(Equal("/audit/notification_audit"))
			Expect(problem.Extensions["resource"]).To(Equal("notification_audit"))
			Expect(problem.Extensions["field"]).To(Equal("notification_id"))
			Expect(problem.Extensions["value"]).To(Equal("test-id-123"))
		})
	})

	Context("JSON Marshaling", func() {
		It("should marshal to RFC 7807 compliant JSON", func() {
			problem := &validation.RFC7807Problem{
				Type:     "https://kubernaut.io/errors/validation-error",
				Title:    "Validation Error",
				Status:   http.StatusBadRequest,
				Detail:   "validation failed",
				Instance: "/audit/notification_audit",
				Extensions: map[string]interface{}{
					"resource": "notification_audit",
					"field_errors": map[string]string{
						"field1": "error1",
					},
				},
			}

			jsonBytes, err := json.Marshal(problem)
			Expect(err).ToNot(HaveOccurred())

			var result validation.RFC7807Problem
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			// Verify standard RFC 7807 fields (type-safe access)
			Expect(result.Type).To(Equal("https://kubernaut.io/errors/validation-error"))
			Expect(result.Title).To(Equal("Validation Error"))
			Expect(result.Status).To(Equal(400))
			Expect(result.Detail).To(Equal("validation failed"))
			Expect(result.Instance).To(Equal("/audit/notification_audit"))

			// Verify extensions are captured correctly
			Expect(result.Extensions["resource"]).To(Equal("notification_audit"))
			Expect(result.Extensions["field_errors"]).ToNot(BeNil())
		})

		It("should omit optional fields when empty", func() {
			problem := &validation.RFC7807Problem{
				Type:   "https://kubernaut.io/errors/internal-error",
				Title:  "Internal Server Error",
				Status: http.StatusInternalServerError,
			}

			jsonBytes, err := json.Marshal(problem)
			Expect(err).ToNot(HaveOccurred())

			var result validation.RFC7807Problem
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.Type).To(Equal("https://kubernaut.io/errors/internal-error"))
			Expect(result.Title).To(Equal("Internal Server Error"))
			Expect(result.Status).To(Equal(500))
			Expect(result.Detail).To(BeEmpty(), "Optional detail field should be empty")
			Expect(result.Instance).To(BeEmpty(), "Optional instance field should be empty")
		})
	})

	Context("Error Interface", func() {
		It("should return error string", func() {
			problem := &validation.RFC7807Problem{
				Type:   "https://kubernaut.io/errors/validation-error",
				Title:  "Validation Error",
				Status: http.StatusBadRequest,
				Detail: "validation failed",
			}

			errStr := problem.Error()
			Expect(errStr).To(ContainSubstring("Validation Error"))
			Expect(errStr).To(ContainSubstring("validation failed"))
			Expect(errStr).To(ContainSubstring("400"))
		})
	})
})
