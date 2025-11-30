/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/go-logr/logr"
)

// ========================================
// AUDIT EVENTS REPOSITORY (TDD GREEN Phase)
// 📋 Tests Define Contract: test/integration/datastorage/audit_events_write_api_test.go
// Authority: DAY21_PHASE1_IMPLEMENTATION_PLAN.md Phase 3
// ========================================
//
// This file implements PostgreSQL persistence for unified audit_events table.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (audit_events_write_api_test.go - 8 scenarios)
// - Production code implements MINIMAL functionality to pass tests
// - Contract defined by test expectations
//
// Business Requirements:
// - BR-STORAGE-033: Generic audit write API
// - BR-STORAGE-032: Unified audit trail
//
// ADR-034 Compliance:
// - Event sourcing pattern (immutable, append-only)
// - Monthly range partitioning
// - JSONB hybrid storage (26 structured columns + flexible event_data)
//
// ========================================

// AuditEvent represents a single audit event for the unified audit_events table
// This is the domain model for audit events across all services
// AUTHORITATIVE SOURCE: Updated from 26 to 27 columns (added parent_event_date for FK constraint)
// See: ADR-034 (updated 2025-11-18), migration 013
type AuditEvent struct {
	// ========================================
	// PRIMARY IDENTIFIERS (4 columns)
	// ========================================
	EventID        uuid.UUID `json:"event_id"`
	EventTimestamp time.Time `json:"event_timestamp"`
	EventDate      time.Time `json:"event_date"` // Generated column for partitioning
	EventType      string    `json:"event_type"` // e.g., gateway.signal.received

	// ========================================
	// EVENT CLASSIFICATION (ADR-034)
	// ========================================
	EventCategory string `json:"event_category"` // 'signal', 'remediation', 'workflow'
	EventAction   string `json:"event_action"`   // 'received', 'processed', 'executed'
	EventOutcome  string `json:"event_outcome"`  // 'success', 'failure', 'pending'

	// ========================================
	// CONTEXT INFORMATION (ADR-034)
	// ========================================
	CorrelationID   string     `json:"correlation_id"`    // e.g., rr-2025-001
	ParentEventID   *uuid.UUID `json:"parent_event_id"`   // For event causality chains
	ParentEventDate *time.Time `json:"parent_event_date"` // Parent event date (required for FK constraint)

	// ========================================
	// RESOURCE TRACKING (4 columns)
	// ========================================
	ResourceType      string `json:"resource_type"`      // e.g., pod, node, deployment
	ResourceID        string `json:"resource_id"`        // Resource identifier
	ResourceNamespace string `json:"resource_namespace"` // Kubernetes namespace
	ClusterID         string `json:"cluster_id"`         // Cluster identifier

	// ========================================
	// AUDIT METADATA (ADR-034)
	// ========================================
	Severity     string `json:"severity"`      // 'info', 'warning', 'error', 'critical'
	DurationMs   int    `json:"duration_ms"`   // Operation duration in milliseconds
	ErrorCode    string `json:"error_code"`    // Specific error code
	ErrorMessage string `json:"error_message"` // Detailed error message

	// ========================================
	// ACTOR INFORMATION (ADR-034)
	// ========================================
	ActorID   string `json:"actor_id"`   // User, service account, or system
	ActorType string `json:"actor_type"` // e.g., user, service_account, system

	// ========================================
	// COMPLIANCE (ADR-034)
	// ========================================
	RetentionDays int  `json:"retention_days"` // Default: 2555 (7 years)
	IsSensitive   bool `json:"is_sensitive"`   // Flag for sensitive data (GDPR, PII)

	// ========================================
	// FLEXIBLE EVENT DATA (ADR-034)
	// ========================================
	EventData map[string]interface{} `json:"event_data"` // Service-specific data
}

// AuditEventsRepository handles PostgreSQL operations for audit_events table
type AuditEventsRepository struct {
	db     *sql.DB
	logger logr.Logger
}

// NewAuditEventsRepository creates a new repository instance
func NewAuditEventsRepository(db *sql.DB, logger logr.Logger) *AuditEventsRepository {
	return &AuditEventsRepository{
		db:     db,
		logger: logger,
	}
}

// Create inserts a new audit event into the unified audit_events table
// Returns the created event with event_id and created_at populated
// This implements the minimal functionality to pass TDD tests
func (r *AuditEventsRepository) Create(ctx context.Context, event *AuditEvent) (*AuditEvent, error) {
	// Generate UUID if not provided
	if event.EventID == uuid.Nil {
		event.EventID = uuid.New()
	}

	// Set event_timestamp if not provided
	if event.EventTimestamp.IsZero() {
		event.EventTimestamp = time.Now().UTC()
	}

	// Set event_date from event_timestamp (for partitioning)
	event.EventDate = time.Date(
		event.EventTimestamp.Year(),
		event.EventTimestamp.Month(),
		event.EventTimestamp.Day(),
		0, 0, 0, 0, time.UTC,
	)

	// Marshal event_data to JSONB
	eventDataJSON, err := json.Marshal(event.EventData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event_data: %w", err)
	}

	// Prepare SQL statement (27 columns - added parent_event_date for FK constraint)
	// Note: event_date MUST be explicitly set for partitioned tables (triggers don't work on partitions)
	// Calculate event_date from event_timestamp
	eventDate := event.EventTimestamp.Truncate(24 * time.Hour)

	query := `
		INSERT INTO audit_events (
			event_id, event_timestamp, event_date, event_type,
			event_category, event_action, event_outcome,
			correlation_id, parent_event_id, parent_event_date,
			resource_type, resource_id, namespace, cluster_name,
			actor_id, actor_type,
			severity, duration_ms, error_code, error_message,
			retention_days, is_sensitive, event_data
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			$11, $12, $13, $14,
			$15, $16,
			$17, $18, $19, $20,
			$21, $22, $23
		)
		RETURNING event_timestamp
	`

	// Handle optional fields with sql.Null* types (ADR-034 schema)
	var parentEventID sql.NullString
	var parentEventDate sql.NullTime
	if event.ParentEventID != nil {
		parentEventID = sql.NullString{String: event.ParentEventID.String(), Valid: true}
		// If parent_event_id is set, parent_event_date must also be set (FK constraint requirement)
		if event.ParentEventDate != nil {
			parentEventDate = sql.NullTime{Time: *event.ParentEventDate, Valid: true}
		}
	}

	// ADR-034: actor_id, actor_type, resource_type, resource_id are NOT NULL (required fields)
	// These are passed as regular strings, not sql.NullString

	var namespace, clusterName sql.NullString
	var errorCode, errorMessage, severity sql.NullString
	var durationMs sql.NullInt32
	if event.ResourceNamespace != "" {
		namespace = sql.NullString{String: event.ResourceNamespace, Valid: true}
	}
	if event.ClusterID != "" {
		clusterName = sql.NullString{String: event.ClusterID, Valid: true}
	}
	if event.ErrorCode != "" {
		errorCode = sql.NullString{String: event.ErrorCode, Valid: true}
	}
	if event.ErrorMessage != "" {
		errorMessage = sql.NullString{String: event.ErrorMessage, Valid: true}
	}
	if event.Severity != "" {
		severity = sql.NullString{String: event.Severity, Valid: true}
	}
	if event.DurationMs != 0 {
		durationMs = sql.NullInt32{Int32: int32(event.DurationMs), Valid: true}
	}

	// Set default retention days if not specified (ADR-034: 7 years = 2555 days)
	retentionDays := event.RetentionDays
	if retentionDays == 0 {
		retentionDays = 2555
	}

	// Execute query (ADR-034 schema - 23 parameters)
	var returnedTimestamp time.Time
	err = r.db.QueryRowContext(ctx, query,
		event.EventID,
		event.EventTimestamp,
		eventDate,
		event.EventType,
		event.EventCategory, // ADR-034
		event.EventAction,   // ADR-034
		event.EventOutcome,  // ADR-034
		event.CorrelationID,
		parentEventID,
		parentEventDate,
		event.ResourceType, // ADR-034 NOT NULL - pass directly
		event.ResourceID,   // ADR-034 NOT NULL - pass directly
		namespace,          // ADR-034: namespace column (not resource_namespace)
		clusterName,        // Renamed from clusterID
		event.ActorID,      // ADR-034 NOT NULL - pass directly
		event.ActorType,    // ADR-034 NOT NULL - pass directly
		severity,
		durationMs,
		errorCode,
		errorMessage,
		retentionDays,
		event.IsSensitive,
		eventDataJSON,
	).Scan(&returnedTimestamp)

	if err != nil {
		return nil, fmt.Errorf("failed to insert audit event: %w", err)
	}

	// Populate returned timestamp
	event.EventTimestamp = returnedTimestamp

	r.logger.V(1).Info("Audit event created",
		"event_id", event.EventID.String(),
		"event_type", event.EventType,
		"event_category", event.EventCategory,
		"correlation_id", event.CorrelationID,
	)

	return event, nil
}

// PaginationMetadata contains pagination information for query results
// DD-STORAGE-010: Offset-based pagination metadata
type PaginationMetadata struct {
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	Total   int  `json:"total"`
	HasMore bool `json:"has_more"`
}

// Query retrieves audit events based on filters with pagination
// DD-STORAGE-010: Query API with offset-based pagination
// BR-STORAGE-021: REST API Read Endpoints
// BR-STORAGE-022: Query Filtering
// BR-STORAGE-023: Pagination Support
func (r *AuditEventsRepository) Query(ctx context.Context, querySQL string, countSQL string, args []interface{}) ([]*AuditEvent, *PaginationMetadata, error) {
	// Execute count query for pagination metadata
	var total int
	err := r.db.QueryRowContext(ctx, countSQL, args[:len(args)-2]...).Scan(&total) // Exclude limit and offset
	if err != nil {
		return nil, nil, fmt.Errorf("failed to count audit events: %w", err)
	}

	// Execute main query
	rows, err := r.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Parse results
	events := make([]*AuditEvent, 0)
	for rows.Next() {
		event := &AuditEvent{}
		var eventDataJSON []byte
		var parentEventID sql.NullString
		var actorID, actorType, resourceType, resourceID sql.NullString
		var severity sql.NullString

		err := rows.Scan(
			&event.EventID,
			&event.EventType,
			&event.EventCategory, // ADR-034
			&event.CorrelationID,
			&event.EventTimestamp,
			&event.EventOutcome, // ADR-034
			&severity,
			&resourceType,
			&resourceID,
			&actorType,
			&actorID,
			&parentEventID,
			&eventDataJSON,
			&event.EventDate,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan audit event: %w", err)
		}

		// Handle NULL fields
		if severity.Valid {
			event.Severity = severity.String
		}
		if resourceType.Valid {
			event.ResourceType = resourceType.String
		}
		if resourceID.Valid {
			event.ResourceID = resourceID.String
		}
		if actorType.Valid {
			event.ActorType = actorType.String
		}
		if actorID.Valid {
			event.ActorID = actorID.String
		}
		if parentEventID.Valid {
			parentUUID, err := uuid.Parse(parentEventID.String)
			if err == nil {
				event.ParentEventID = &parentUUID
			}
		}

		// Unmarshal event_data JSONB
		if len(eventDataJSON) > 0 {
			if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
				return nil, nil, fmt.Errorf("failed to unmarshal event_data: %w", err)
			}
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating audit events: %w", err)
	}

	// Build pagination metadata
	limit := int(args[len(args)-2].(int))
	offset := int(args[len(args)-1].(int))
	pagination := &PaginationMetadata{
		Limit:   limit,
		Offset:  offset,
		Total:   total,
		HasMore: offset+len(events) < total,
	}

	r.logger.V(1).Info("Audit events queried",
		"count", len(events),
		"total", total,
		"limit", limit,
		"offset", offset,
	)

	return events, pagination, nil
}

// HealthCheck verifies database connectivity
func (r *AuditEventsRepository) HealthCheck(ctx context.Context) error {
	if err := r.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}
	return nil
}
