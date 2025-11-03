package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// ========================================
// NOTIFICATION AUDIT REPOSITORY (TDD GREEN Phase)
// 📋 Tests Define Contract: notification_audit_repository_test.go
// Authority: migrations/010_audit_write_api_phase1.sql
// ========================================
//
// This file implements PostgreSQL persistence for NotificationAudit records.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (notification_audit_repository_test.go - 13 tests)
// - Production code implements MINIMAL functionality to pass tests
// - Contract defined by test expectations
//
// Business Requirements:
// - BR-NOTIFICATION-001: Persist notification delivery attempts
// - BR-NOTIFICATION-004: Enable V2.0 RAR timeline reconstruction
//
// ========================================

// NotificationAuditRepository handles PostgreSQL operations for notification_audit table.
type NotificationAuditRepository struct {
	db        *sql.DB
	logger    *zap.Logger
	validator *validation.NotificationAuditValidator
}

// NewNotificationAuditRepository creates a new repository instance.
func NewNotificationAuditRepository(db *sql.DB, logger *zap.Logger) *NotificationAuditRepository {
	return &NotificationAuditRepository{
		db:        db,
		logger:    logger,
		validator: validation.NewNotificationAuditValidator(),
	}
}

// Create inserts a new NotificationAudit record.
// Returns the created record with ID, created_at, and updated_at populated.
// Returns validation.ValidationError if input is invalid.
// Returns error for database errors (connection, unique constraint, etc).
func (r *NotificationAuditRepository) Create(ctx context.Context, audit *models.NotificationAudit) (*models.NotificationAudit, error) {
	// Validate input
	if err := r.validator.Validate(audit); err != nil {
		return nil, err
	}

	// Prepare SQL statement
	query := `
		INSERT INTO notification_audit (
			remediation_id, notification_id, recipient, channel,
			message_summary, status, sent_at, delivery_status,
			error_message, escalation_level
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at
	`

	// Handle optional fields (delivery_status, error_message)
	var deliveryStatus, errorMessage sql.NullString
	if audit.DeliveryStatus != "" {
		deliveryStatus = sql.NullString{String: audit.DeliveryStatus, Valid: true}
	}
	if audit.ErrorMessage != "" {
		errorMessage = sql.NullString{String: audit.ErrorMessage, Valid: true}
	}

	// Execute query
	var id int64
	var createdAt, updatedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query,
		audit.RemediationID,
		audit.NotificationID,
		audit.Recipient,
		audit.Channel,
		audit.MessageSummary,
		audit.Status,
		audit.SentAt,
		deliveryStatus,
		errorMessage,
		audit.EscalationLevel,
	).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		// Check for unique constraint violation
		if pqErr, ok := err.(*pq.Error); ok {
			if pqErr.Code == "23505" { // unique_violation
				return nil, validation.NewConflictProblem("notification_audit", "notification_id", audit.NotificationID)
			}
		}
		return nil, fmt.Errorf("failed to insert notification_audit: %w", err)
	}

	// Populate returned fields
	audit.ID = id
	if createdAt.Valid {
		audit.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		audit.UpdatedAt = updatedAt.Time
	}

	return audit, nil
}

// GetByNotificationID retrieves a NotificationAudit record by notification_id.
// Returns sql.ErrNoRows if not found.
func (r *NotificationAuditRepository) GetByNotificationID(ctx context.Context, notificationID string) (*models.NotificationAudit, error) {
	query := `
		SELECT id, remediation_id, notification_id, recipient, channel,
			   message_summary, status, sent_at, delivery_status,
			   error_message, escalation_level, created_at, updated_at
		FROM notification_audit
		WHERE notification_id = $1
	`

	audit := &models.NotificationAudit{}
	var deliveryStatus, errorMessage sql.NullString
	var createdAt, updatedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, notificationID).Scan(
		&audit.ID,
		&audit.RemediationID,
		&audit.NotificationID,
		&audit.Recipient,
		&audit.Channel,
		&audit.MessageSummary,
		&audit.Status,
		&audit.SentAt,
		&deliveryStatus,
		&errorMessage,
		&audit.EscalationLevel,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, validation.NewNotFoundProblem("notification_audit", notificationID)
		}
		return nil, fmt.Errorf("failed to retrieve notification_audit: %w", err)
	}

	// Populate optional fields
	if deliveryStatus.Valid {
		audit.DeliveryStatus = deliveryStatus.String
	}
	if errorMessage.Valid {
		audit.ErrorMessage = errorMessage.String
	}
	if createdAt.Valid {
		audit.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		audit.UpdatedAt = updatedAt.Time
	}

	return audit, nil
}

// HealthCheck verifies database connectivity.
func (r *NotificationAuditRepository) HealthCheck(ctx context.Context) error {
	if err := r.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}
	return nil
}

