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

	// ========================================
	// CHANNEL VALIDATION CONTEXT
	// ========================================
	Context("Channel Validation", func() {
		// BEHAVIOR: Validator rejects empty channel
		// CORRECTNESS: ValidationError with "required" in field error
		It("should reject empty channel with required field error", func() {
			// ARRANGE: Empty channel
			audit.Channel = ""

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Empty channel should fail validation")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error mentions "required"
			Expect(valErr.FieldErrors["channel"]).To(ContainSubstring("required"),
				"Field error should indicate channel is required")
		})

		// BEHAVIOR: Validator rejects invalid channel values
		// CORRECTNESS: ValidationError with enumeration hint
		It("should reject invalid channel with enumeration hint", func() {
			// ARRANGE: Invalid channel value
			audit.Channel = "invalid"

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Invalid channel should fail validation")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error lists valid channels
			Expect(valErr.FieldErrors["channel"]).To(ContainSubstring("must be one of"),
				"Field error should list valid channel options")
		})

		// BEHAVIOR: Validator enforces 50-character maximum length
		// CORRECTNESS: ValidationError with length constraint
		It("should reject channel exceeding 50 characters with length error", func() {
			// ARRANGE: Channel at 51 characters (boundary + 1)
			audit.Channel = strings.Repeat("a", 51)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Channel >50 characters should fail validation")

			// CORRECTNESS: Error is ValidationError type
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Field error mentions 50-character limit
			Expect(valErr.FieldErrors["channel"]).To(ContainSubstring("50 characters"),
				"Field error should mention 50-character maximum")
		})

		// BEHAVIOR: Validator accepts case-insensitive channel values
		// CORRECTNESS: All valid channel variations accepted (EMAIL, email, Email, etc.)
		It("should accept all case variations of valid channels", func() {
			// ARRANGE: All valid channel variations (case-insensitive)
			channels := []string{"EMAIL", "Email", "SLACK", "Slack", "PAGERDUTY", "PagerDuty", "SMS", "Sms"}

			// ACT & CORRECTNESS: Validate each variation
			for _, channel := range channels {
				audit.Channel = channel
				err := validator.Validate(audit)
				Expect(err).ToNot(HaveOccurred(), "channel '%s' should be valid (case-insensitive)", channel)
			}
		})
	})

	// ========================================
	// MESSAGESUMMARY VALIDATION CONTEXT
	// ========================================
	Context("MessageSummary Validation", func() {
		// BEHAVIOR: Validator rejects empty message summary
		// CORRECTNESS: ValidationError with "required" in field error
		It("should reject empty message_summary with required field error", func() {
			// ARRANGE: Empty message summary
			audit.MessageSummary = ""

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Empty message_summary should fail validation")
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")
			Expect(valErr.FieldErrors["message_summary"]).To(ContainSubstring("required"),
				"Field error should indicate message_summary is required")
		})

		// BEHAVIOR: Validator rejects whitespace-only message summary
		// CORRECTNESS: ValidationError with "required" (whitespace trimmed)
		It("should reject whitespace-only message_summary as empty", func() {
			// ARRANGE: Whitespace-only message summary
			audit.MessageSummary = "   "

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred (whitespace trimmed = empty)
			Expect(err).To(HaveOccurred(), "Whitespace-only message_summary should fail validation")
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")
			Expect(valErr.FieldErrors["message_summary"]).To(ContainSubstring("required"),
				"Whitespace-only message_summary should be treated as empty")
		})

		// BEHAVIOR: Validator accepts large message summaries (TEXT type, no length limit)
		// CORRECTNESS: 10,000-character message summary accepted
		It("should accept large message_summary for TEXT column type", func() {
			// ARRANGE: Large message summary (10,000 characters)
			audit.MessageSummary = strings.Repeat("a", 10000)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (TEXT type supports large content)
			Expect(err).ToNot(HaveOccurred(), "Large message_summary should be valid for TEXT column")
		})
	})

	// ========================================
	// STATUS VALIDATION CONTEXT
	// ========================================
	Context("Status Validation", func() {
		// BEHAVIOR: Validator rejects empty status
		// CORRECTNESS: ValidationError with "required" in field error
		It("should reject empty status with required field error", func() {
			// ARRANGE: Empty status
			audit.Status = ""

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Empty status should fail validation")
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")
			Expect(valErr.FieldErrors["status"]).To(ContainSubstring("required"),
				"Field error should indicate status is required")
		})

		// BEHAVIOR: Validator rejects invalid status values
		// CORRECTNESS: ValidationError with enumeration hint
		It("should reject invalid status with enumeration hint", func() {
			// ARRANGE: Invalid status value
			audit.Status = "invalid"

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Invalid status should fail validation")
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")
			Expect(valErr.FieldErrors["status"]).To(ContainSubstring("must be one of"),
				"Field error should list valid status options")
		})

		// BEHAVIOR: Validator enforces 50-character maximum length
		// CORRECTNESS: ValidationError with length constraint
		It("should reject status exceeding 50 characters with length error", func() {
			// ARRANGE: Status at 51 characters (boundary + 1)
			audit.Status = strings.Repeat("a", 51)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Status >50 characters should fail validation")
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")
			Expect(valErr.FieldErrors["status"]).To(ContainSubstring("50 characters"),
				"Field error should mention 50-character maximum")
		})

		// BEHAVIOR: Validator accepts case-insensitive status values
		// CORRECTNESS: All valid status variations accepted (SENT, sent, Sent, etc.)
		It("should accept all case variations of valid statuses", func() {
			// ARRANGE: All valid status variations (case-insensitive)
			statuses := []string{"SENT", "Sent", "FAILED", "Failed", "ACKNOWLEDGED", "Acknowledged", "ESCALATED", "Escalated"}

			// ACT & CORRECTNESS: Validate each variation
			for _, status := range statuses {
				audit.Status = status
				err := validator.Validate(audit)
				Expect(err).ToNot(HaveOccurred(), "status '%s' should be valid (case-insensitive)", status)
			}
		})
	})

	// ========================================
	// SENTAT VALIDATION CONTEXT
	// ========================================
	Context("SentAt Validation", func() {
		// BEHAVIOR: Validator rejects zero timestamp (uninitialized time.Time)
		// CORRECTNESS: ValidationError with "required" in field error
		It("should reject zero sent_at timestamp with required field error", func() {
			// ARRANGE: Zero timestamp
			audit.SentAt = time.Time{}

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Zero sent_at should fail validation")
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")
			Expect(valErr.FieldErrors["sent_at"]).To(ContainSubstring("required"),
				"Field error should indicate sent_at is required")
		})

		// BEHAVIOR: Validator rejects future timestamps beyond clock skew tolerance
		// CORRECTNESS: ValidationError with "cannot be in the future" message
		It("should reject future sent_at beyond 5-minute clock skew tolerance", func() {
			// ARRANGE: Timestamp 10 minutes in future (beyond 5-minute tolerance)
			audit.SentAt = time.Now().Add(10 * time.Minute)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "sent_at >5 minutes in future should fail validation")
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")
			Expect(valErr.FieldErrors["sent_at"]).To(ContainSubstring("cannot be in the future"),
				"Field error should indicate future timestamp rejection")
		})

		// BEHAVIOR: Validator accepts timestamps within clock skew tolerance
		// CORRECTNESS: 4-minute future timestamp accepted (within 5-minute tolerance)
		It("should accept sent_at within 5-minute clock skew tolerance", func() {
			// ARRANGE: Timestamp 4 minutes in future (within 5-minute tolerance)
			audit.SentAt = time.Now().Add(4 * time.Minute)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (within clock skew tolerance)
			Expect(err).ToNot(HaveOccurred(), "sent_at within 5-minute clock skew should be valid")
		})

		// BEHAVIOR: Validator accepts past timestamps
		// CORRECTNESS: 1-hour past timestamp accepted
		It("should accept past sent_at timestamps", func() {
			// ARRANGE: Timestamp 1 hour in past
			audit.SentAt = time.Now().Add(-1 * time.Hour)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (past timestamps always valid)
			Expect(err).ToNot(HaveOccurred(), "Past sent_at should be valid")
		})
	})

	// ========================================
	// ESCALATIONLEVEL VALIDATION CONTEXT
	// ========================================
	Context("EscalationLevel Validation", func() {
		// BEHAVIOR: Validator rejects negative escalation levels
		// CORRECTNESS: ValidationError with "non-negative" constraint
		It("should reject negative escalation_level with non-negative constraint error", func() {
			// ARRANGE: Negative escalation level
			audit.EscalationLevel = -1

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Negative escalation_level should fail validation")
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")
			Expect(valErr.FieldErrors["escalation_level"]).To(ContainSubstring("non-negative"),
				"Field error should indicate non-negative constraint")
		})

		// BEHAVIOR: Validator enforces maximum escalation level of 100
		// CORRECTNESS: ValidationError with "at most 100" constraint
		It("should reject escalation_level exceeding 100 with maximum constraint error", func() {
			// ARRANGE: Escalation level at 101 (boundary + 1)
			audit.EscalationLevel = 101

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "escalation_level >100 should fail validation")
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")
			Expect(valErr.FieldErrors["escalation_level"]).To(ContainSubstring("at most 100"),
				"Field error should indicate 100 maximum")
		})

		// BEHAVIOR: Validator accepts escalation level at lower boundary (0)
		// CORRECTNESS: Level 0 accepted (minimum valid value)
		It("should accept escalation_level at lower boundary (0)", func() {
			// ARRANGE: Escalation level at minimum (0)
			audit.EscalationLevel = 0

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (0 is valid lower bound)
			Expect(err).ToNot(HaveOccurred(), "escalation_level 0 should be valid (lower boundary)")
		})

		// BEHAVIOR: Validator accepts escalation level at upper boundary (100)
		// CORRECTNESS: Level 100 accepted (maximum valid value)
		It("should accept escalation_level at upper boundary (100)", func() {
			// ARRANGE: Escalation level at maximum (100)
			audit.EscalationLevel = 100

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (100 is valid upper bound)
			Expect(err).ToNot(HaveOccurred(), "escalation_level 100 should be valid (upper boundary)")
		})
	})

	// ========================================
	// MULTIPLE FIELD ERRORS CONTEXT
	// ========================================
	Context("Multiple Field Errors", func() {
		// BEHAVIOR: Validator reports all field errors in a single validation pass
		// CORRECTNESS: ValidationError contains exactly 8 field errors with correct keys
		It("should report all 8 field errors simultaneously (not fail-fast)", func() {
			// ARRANGE: Audit with 8 invalid fields
			audit.RemediationID = ""
			audit.NotificationID = ""
			audit.Recipient = ""
			audit.Channel = "invalid"
			audit.MessageSummary = ""
			audit.Status = "invalid"
			audit.SentAt = time.Time{}
			audit.EscalationLevel = -1

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: Error occurred
			Expect(err).To(HaveOccurred(), "Multiple invalid fields should fail validation")

			// CORRECTNESS: Error is ValidationError with all field errors
			valErr, ok := err.(*validation.ValidationError)
			Expect(ok).To(BeTrue(), "Error should be ValidationError type")

			// CORRECTNESS: Exactly 8 field errors (not fail-fast behavior)
			Expect(len(valErr.FieldErrors)).To(Equal(8),
				"Validator should report all 8 field errors, not fail-fast")

			// CORRECTNESS: All expected field error keys are present
			Expect(valErr.FieldErrors).To(HaveKey("remediation_id"), "Should report remediation_id error")
			Expect(valErr.FieldErrors).To(HaveKey("notification_id"), "Should report notification_id error")
			Expect(valErr.FieldErrors).To(HaveKey("recipient"), "Should report recipient error")
			Expect(valErr.FieldErrors).To(HaveKey("channel"), "Should report channel error")
			Expect(valErr.FieldErrors).To(HaveKey("message_summary"), "Should report message_summary error")
			Expect(valErr.FieldErrors).To(HaveKey("status"), "Should report status error")
			Expect(valErr.FieldErrors).To(HaveKey("sent_at"), "Should report sent_at error")
			Expect(valErr.FieldErrors).To(HaveKey("escalation_level"), "Should report escalation_level error")
		})
	})

	// ========================================
	// OPTIONAL FIELDS CONTEXT
	// ========================================
	Context("Optional Fields", func() {
		// BEHAVIOR: Validator accepts empty delivery_status (optional field)
		// CORRECTNESS: No validation error when delivery_status is empty
		It("should accept empty delivery_status as valid (optional field)", func() {
			// ARRANGE: Empty delivery_status
			audit.DeliveryStatus = ""

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (delivery_status is optional)
			Expect(err).ToNot(HaveOccurred(), "Empty delivery_status should be valid (optional field)")
		})

		// BEHAVIOR: Validator accepts empty error_message (optional field)
		// CORRECTNESS: No validation error when error_message is empty
		It("should accept empty error_message as valid (optional field)", func() {
			// ARRANGE: Empty error_message
			audit.ErrorMessage = ""

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (error_message is optional)
			Expect(err).ToNot(HaveOccurred(), "Empty error_message should be valid (optional field)")
		})

		// BEHAVIOR: Validator accepts large delivery_status (TEXT type, no length limit)
		// CORRECTNESS: 10,000-character delivery_status accepted
		It("should accept large delivery_status for TEXT column type", func() {
			// ARRANGE: Large delivery_status (10,000 characters)
			audit.DeliveryStatus = strings.Repeat("a", 10000)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (TEXT type supports large content)
			Expect(err).ToNot(HaveOccurred(), "Large delivery_status should be valid for TEXT column")
		})

		// BEHAVIOR: Validator accepts large error_message (TEXT type, no length limit)
		// CORRECTNESS: 10,000-character error_message accepted
		It("should accept large error_message for TEXT column type", func() {
			// ARRANGE: Large error_message (10,000 characters)
			audit.ErrorMessage = strings.Repeat("a", 10000)

			// ACT: Validate
			err := validator.Validate(audit)

			// CORRECTNESS: No error (TEXT type supports large content)
			Expect(err).ToNot(HaveOccurred(), "Large error_message should be valid for TEXT column")
		})
	})
})

var _ = Describe("ValidationError", func() {
	var validationErr *validation.ValidationError

	BeforeEach(func() {
		validationErr = validation.NewValidationError("notification_audit", "validation failed")
	})

	Context("Error Creation", func() {
		// BEHAVIOR: ValidationError constructor initializes empty field errors map
		// CORRECTNESS: Resource, message set correctly; FieldErrors is empty but initialized
		It("should create a validation error with resource, message, and empty field errors", func() {
			// CORRECTNESS: Resource and message have expected values
			Expect(validationErr.Resource).To(Equal("notification_audit"), "Resource should be set")
			Expect(validationErr.Message).To(Equal("validation failed"), "Message should be set")

			// CORRECTNESS: FieldErrors map is initialized and empty
			Expect(validationErr.FieldErrors).To(HaveLen(0), "FieldErrors should be empty initially")
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

			// CORRECTNESS: Extensions contain resource and field_errors map
			Expect(result.Extensions["resource"]).To(Equal("notification_audit"), "Extensions should contain resource")

			// CORRECTNESS: field_errors is a map (type assertion proves it's not nil)
			fieldErrors, ok := result.Extensions["field_errors"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "field_errors should be a map")
			Expect(fieldErrors).To(HaveLen(1), "field_errors should have 1 entry")
			Expect(fieldErrors["field1"]).To(Equal("error1"), "field_errors should contain field1 error")
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
