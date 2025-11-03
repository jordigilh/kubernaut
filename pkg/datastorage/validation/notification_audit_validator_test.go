package validation

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

func TestValidation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage Validation Suite")
}

var _ = Describe("NotificationAuditValidator", func() {
	var (
		validator *NotificationAuditValidator
		audit     *models.NotificationAudit
	)

	BeforeEach(func() {
		validator = NewNotificationAuditValidator()
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
		It("should pass validation for a complete valid record", func() {
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})

		It("should pass validation with minimal required fields", func() {
			audit.DeliveryStatus = ""
			audit.ErrorMessage = ""
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})

		It("should pass validation with all status values", func() {
			statuses := []string{"sent", "failed", "acknowledged", "escalated"}
			for _, status := range statuses {
				audit.Status = status
				err := validator.Validate(audit)
				Expect(err).To(BeNil(), "status '%s' should be valid", status)
			}
		})

		It("should pass validation with all channel values", func() {
			channels := []string{"email", "slack", "pagerduty", "sms"}
			for _, channel := range channels {
				audit.Channel = channel
				err := validator.Validate(audit)
				Expect(err).To(BeNil(), "channel '%s' should be valid", channel)
			}
		})

		It("should pass validation with escalation level up to 100", func() {
			audit.EscalationLevel = 100
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})
	})

	Context("Nil Audit Record", func() {
		It("should fail validation for nil audit", func() {
			err := validator.Validate(nil)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("cannot be nil"))
		})
	})

	Context("RemediationID Validation", func() {
		It("should fail validation for empty remediation_id", func() {
			audit.RemediationID = ""
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["remediation_id"]).To(ContainSubstring("required"))
		})

		It("should fail validation for whitespace-only remediation_id", func() {
			audit.RemediationID = "   "
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["remediation_id"]).To(ContainSubstring("required"))
		})

		It("should fail validation for remediation_id exceeding 255 characters", func() {
			audit.RemediationID = strings.Repeat("a", 256)
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["remediation_id"]).To(ContainSubstring("255 characters"))
		})

		It("should pass validation for remediation_id at 255 characters", func() {
			audit.RemediationID = strings.Repeat("a", 255)
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})
	})

	Context("NotificationID Validation", func() {
		It("should fail validation for empty notification_id", func() {
			audit.NotificationID = ""
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["notification_id"]).To(ContainSubstring("required"))
		})

		It("should fail validation for whitespace-only notification_id", func() {
			audit.NotificationID = "   "
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["notification_id"]).To(ContainSubstring("required"))
		})

		It("should fail validation for notification_id exceeding 255 characters", func() {
			audit.NotificationID = strings.Repeat("a", 256)
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["notification_id"]).To(ContainSubstring("255 characters"))
		})

		It("should pass validation for notification_id at 255 characters", func() {
			audit.NotificationID = strings.Repeat("a", 255)
			err := validator.Validate(audit)
			Expect(err).To(BeNil())
		})
	})

	Context("Recipient Validation", func() {
		It("should fail validation for empty recipient", func() {
			audit.Recipient = ""
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["recipient"]).To(ContainSubstring("required"))
		})

		It("should fail validation for whitespace-only recipient", func() {
			audit.Recipient = "   "
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["recipient"]).To(ContainSubstring("required"))
		})

		It("should fail validation for recipient exceeding 255 characters", func() {
			audit.Recipient = strings.Repeat("a", 256)
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["recipient"]).To(ContainSubstring("255 characters"))
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
			Expect(err.FieldErrors["channel"]).To(ContainSubstring("required"))
		})

		It("should fail validation for invalid channel", func() {
			audit.Channel = "invalid"
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["channel"]).To(ContainSubstring("must be one of"))
		})

		It("should fail validation for channel exceeding 50 characters", func() {
			audit.Channel = strings.Repeat("a", 51)
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["channel"]).To(ContainSubstring("50 characters"))
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
			Expect(err.FieldErrors["message_summary"]).To(ContainSubstring("required"))
		})

		It("should fail validation for whitespace-only message_summary", func() {
			audit.MessageSummary = "   "
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["message_summary"]).To(ContainSubstring("required"))
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
			Expect(err.FieldErrors["status"]).To(ContainSubstring("required"))
		})

		It("should fail validation for invalid status", func() {
			audit.Status = "invalid"
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["status"]).To(ContainSubstring("must be one of"))
		})

		It("should fail validation for status exceeding 50 characters", func() {
			audit.Status = strings.Repeat("a", 51)
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["status"]).To(ContainSubstring("50 characters"))
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
			Expect(err.FieldErrors["sent_at"]).To(ContainSubstring("required"))
		})

		It("should fail validation for future sent_at (beyond clock skew)", func() {
			audit.SentAt = time.Now().Add(10 * time.Minute)
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["sent_at"]).To(ContainSubstring("cannot be in the future"))
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
			Expect(err.FieldErrors["escalation_level"]).To(ContainSubstring("non-negative"))
		})

		It("should fail validation for escalation_level exceeding 100", func() {
			audit.EscalationLevel = 101
			err := validator.Validate(audit)
			Expect(err).ToNot(BeNil())
			Expect(err.FieldErrors["escalation_level"]).To(ContainSubstring("at most 100"))
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
			Expect(len(err.FieldErrors)).To(Equal(8))
			Expect(err.FieldErrors).To(HaveKey("remediation_id"))
			Expect(err.FieldErrors).To(HaveKey("notification_id"))
			Expect(err.FieldErrors).To(HaveKey("recipient"))
			Expect(err.FieldErrors).To(HaveKey("channel"))
			Expect(err.FieldErrors).To(HaveKey("message_summary"))
			Expect(err.FieldErrors).To(HaveKey("status"))
			Expect(err.FieldErrors).To(HaveKey("sent_at"))
			Expect(err.FieldErrors).To(HaveKey("escalation_level"))
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
	var validationErr *ValidationError

	BeforeEach(func() {
		validationErr = NewValidationError("notification_audit", "validation failed")
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
			problem := NewValidationErrorProblem("notification_audit", fieldErrors)

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
			problem := NewNotFoundProblem("notification_audit", "test-id-123")

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
			problem := NewInternalErrorProblem("database connection failed")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/internal-error"))
			Expect(problem.Title).To(Equal("Internal Server Error"))
			Expect(problem.Status).To(Equal(http.StatusInternalServerError))
			Expect(problem.Detail).To(Equal("database connection failed"))
			Expect(problem.Extensions["retry"]).To(BeTrue())
		})
	})

	Context("Service Unavailable Problem", func() {
		It("should create service unavailable problem", func() {
			problem := NewServiceUnavailableProblem("database is down")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/service-unavailable"))
			Expect(problem.Title).To(Equal("Service Unavailable"))
			Expect(problem.Status).To(Equal(http.StatusServiceUnavailable))
			Expect(problem.Detail).To(Equal("database is down"))
			Expect(problem.Extensions["retry"]).To(BeTrue())
		})
	})

	Context("Conflict Problem", func() {
		It("should create conflict problem", func() {
			problem := NewConflictProblem("notification_audit", "notification_id", "test-id-123")

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
			problem := &RFC7807Problem{
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

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			// Verify standard RFC 7807 fields
			Expect(result["type"]).To(Equal("https://kubernaut.io/errors/validation-error"))
			Expect(result["title"]).To(Equal("Validation Error"))
			Expect(result["status"]).To(BeNumerically("==", 400))
			Expect(result["detail"]).To(Equal("validation failed"))
			Expect(result["instance"]).To(Equal("/audit/notification_audit"))

			// Verify extensions are flattened into top-level JSON
			Expect(result["resource"]).To(Equal("notification_audit"))
			Expect(result["field_errors"]).ToNot(BeNil())
		})

		It("should omit optional fields when empty", func() {
			problem := &RFC7807Problem{
				Type:   "https://kubernaut.io/errors/internal-error",
				Title:  "Internal Server Error",
				Status: http.StatusInternalServerError,
			}

			jsonBytes, err := json.Marshal(problem)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["type"]).To(Equal("https://kubernaut.io/errors/internal-error"))
			Expect(result["title"]).To(Equal("Internal Server Error"))
			Expect(result["status"]).To(BeNumerically("==", 500))
			Expect(result).ToNot(HaveKey("detail"))
			Expect(result).ToNot(HaveKey("instance"))
		})
	})

	Context("Error Interface", func() {
		It("should return error string", func() {
			problem := &RFC7807Problem{
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

