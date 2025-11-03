package validation

import (
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// NOTIFICATION AUDIT VALIDATOR (TDD GREEN Phase)
// 📋 Tests Define Contract: notification_audit_validator_test.go
// Authority: migrations/010_audit_write_api_phase1.sql
// ========================================
//
// This file implements validation logic for NotificationAudit records.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (notification_audit_validator_test.go - 103 tests)
// - Production code implements MINIMAL functionality to pass tests
// - Contract defined by test expectations
//
// Business Requirements:
// - BR-NOTIFICATION-001: Track all notification delivery attempts
// - BR-NOTIFICATION-002: Record notification failures
// - BR-NOTIFICATION-003: Capture escalation events
//
// ========================================

const (
	// Clock skew tolerance for sent_at validation (5 minutes)
	clockSkewTolerance = 5 * time.Minute
)

// NotificationAuditValidator validates NotificationAudit models.
type NotificationAuditValidator struct {
	// No state needed for now - stateless validator
}

// NewNotificationAuditValidator creates a new NotificationAuditValidator.
func NewNotificationAuditValidator() *NotificationAuditValidator {
	return &NotificationAuditValidator{}
}

// Validate performs comprehensive validation of NotificationAudit.
// Returns error if validation fails, nil if valid.
func (v *NotificationAuditValidator) Validate(audit *models.NotificationAudit) error {
	// Nil check
	if audit == nil {
		return NewValidationError("notification_audit", "audit record cannot be nil")
	}

	valErr := NewValidationError("notification_audit", "validation failed")

	// Validate each field
	v.validateRemediationID(audit.RemediationID, valErr)
	v.validateNotificationID(audit.NotificationID, valErr)
	v.validateRecipient(audit.Recipient, valErr)
	v.validateChannel(audit.Channel, valErr)
	v.validateMessageSummary(audit.MessageSummary, valErr)
	v.validateStatus(audit.Status, valErr)
	v.validateSentAt(audit.SentAt, valErr)
	v.validateEscalationLevel(audit.EscalationLevel, valErr)

	// Return nil if no errors
	if len(valErr.FieldErrors) == 0 {
		return nil
	}

	return valErr
}

// validateRemediationID validates the remediation_id field.
func (v *NotificationAuditValidator) validateRemediationID(id string, valErr *ValidationError) {
	if strings.TrimSpace(id) == "" {
		valErr.AddFieldError("remediation_id", "remediation_id is required")
		return
	}
	if len(id) > 255 {
		valErr.AddFieldError("remediation_id", fmt.Sprintf("remediation_id must be at most 255 characters (got %d)", len(id)))
	}
}

// validateNotificationID validates the notification_id field.
func (v *NotificationAuditValidator) validateNotificationID(id string, valErr *ValidationError) {
	if strings.TrimSpace(id) == "" {
		valErr.AddFieldError("notification_id", "notification_id is required")
		return
	}
	if len(id) > 255 {
		valErr.AddFieldError("notification_id", fmt.Sprintf("notification_id must be at most 255 characters (got %d)", len(id)))
	}
}

// validateRecipient validates the recipient field.
func (v *NotificationAuditValidator) validateRecipient(recipient string, valErr *ValidationError) {
	if strings.TrimSpace(recipient) == "" {
		valErr.AddFieldError("recipient", "recipient is required")
		return
	}
	if len(recipient) > 255 {
		valErr.AddFieldError("recipient", fmt.Sprintf("recipient must be at most 255 characters (got %d)", len(recipient)))
	}
}

// validateChannel validates the channel field (enum validation).
func (v *NotificationAuditValidator) validateChannel(channel string, valErr *ValidationError) {
	if strings.TrimSpace(channel) == "" {
		valErr.AddFieldError("channel", "channel is required")
		return
	}

	// Check length BEFORE enum validation (test expects length error first)
	if len(channel) > 50 {
		valErr.AddFieldError("channel", fmt.Sprintf("channel must be at most 50 characters (got %d)", len(channel)))
		return
	}

	// Enum validation (case-insensitive)
	validChannels := map[string]bool{
		"email":     true,
		"slack":     true,
		"pagerduty": true,
		"sms":       true,
	}

	if !validChannels[strings.ToLower(channel)] {
		valErr.AddFieldError("channel", fmt.Sprintf("channel must be one of: email, slack, pagerduty, sms (got '%s')", channel))
	}
}

// validateMessageSummary validates the message_summary field.
func (v *NotificationAuditValidator) validateMessageSummary(summary string, valErr *ValidationError) {
	if strings.TrimSpace(summary) == "" {
		valErr.AddFieldError("message_summary", "message_summary is required")
		return
	}
	// TEXT type - no length limit in PostgreSQL, but tests validate long strings work
}

// validateStatus validates the status field (enum validation).
func (v *NotificationAuditValidator) validateStatus(status string, valErr *ValidationError) {
	if strings.TrimSpace(status) == "" {
		valErr.AddFieldError("status", "status is required")
		return
	}

	// Check length BEFORE enum validation (test expects length error first)
	if len(status) > 50 {
		valErr.AddFieldError("status", fmt.Sprintf("status must be at most 50 characters (got %d)", len(status)))
		return
	}

	// Enum validation (case-insensitive)
	validStatuses := map[string]bool{
		"sent":         true,
		"failed":       true,
		"acknowledged": true,
		"escalated":    true,
	}

	if !validStatuses[strings.ToLower(status)] {
		valErr.AddFieldError("status", fmt.Sprintf("status must be one of: sent, failed, acknowledged, escalated (got '%s')", status))
	}
}

// validateSentAt validates the sent_at timestamp.
func (v *NotificationAuditValidator) validateSentAt(sentAt time.Time, valErr *ValidationError) {
	// Check for zero value
	if sentAt.IsZero() {
		valErr.AddFieldError("sent_at", "sent_at is required")
		return
	}

	// Check for future timestamps (with clock skew tolerance)
	now := time.Now()
	if sentAt.After(now.Add(clockSkewTolerance)) {
		valErr.AddFieldError("sent_at", "sent_at cannot be in the future (beyond 5-minute clock skew tolerance)")
	}
}

// validateEscalationLevel validates the escalation_level field.
func (v *NotificationAuditValidator) validateEscalationLevel(level int, valErr *ValidationError) {
	if level < 0 {
		valErr.AddFieldError("escalation_level", fmt.Sprintf("escalation_level must be non-negative (got %d)", level))
		return
	}
	if level > 100 {
		valErr.AddFieldError("escalation_level", fmt.Sprintf("escalation_level must be at most 100 (got %d)", level))
	}
}

