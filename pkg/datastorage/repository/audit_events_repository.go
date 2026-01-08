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
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/sqlutil"
)

// DateOnly is a time.Time that serializes to JSON as date-only format (YYYY-MM-DD)
// This is required because the OpenAPI spec defines event_date as format: date
// and oapi-codegen generates openapi_types.Date which expects "2025-12-16" not "2025-12-16T00:00:00Z"
type DateOnly time.Time

// MarshalJSON serializes DateOnly to date-only format "YYYY-MM-DD"
func (d DateOnly) MarshalJSON() ([]byte, error) {
	t := time.Time(d)
	if t.IsZero() {
		return []byte("null"), nil
	}
	return []byte(fmt.Sprintf(`"%s"`, t.Format("2006-01-02"))), nil
}

// UnmarshalJSON deserializes date-only format "YYYY-MM-DD" to DateOnly
func (d *DateOnly) UnmarshalJSON(data []byte) error {
	// Handle null
	if string(data) == "null" {
		*d = DateOnly{}
		return nil
	}
	// Parse date-only format
	t, err := time.Parse(`"2006-01-02"`, string(data))
	if err != nil {
		// Try full datetime format as fallback
		t, err = time.Parse(`"2006-01-02T15:04:05Z"`, string(data))
		if err != nil {
			return err
		}
	}
	*d = DateOnly(t)
	return nil
}

// Time returns the underlying time.Time
func (d DateOnly) Time() time.Time {
	return time.Time(d)
}

// Scan implements sql.Scanner interface for database scanning
func (d *DateOnly) Scan(value interface{}) error {
	if value == nil {
		*d = DateOnly{}
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		*d = DateOnly(v)
		return nil
	default:
		return fmt.Errorf("cannot scan type %T into DateOnly", value)
	}
}

// Value implements driver.Valuer interface for database insertion
func (d DateOnly) Value() (interface{}, error) {
	return time.Time(d), nil
}

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
	EventDate      DateOnly  `json:"event_date"` // Generated column for partitioning (serializes as "YYYY-MM-DD")
	EventType      string    `json:"event_type"` // e.g., gateway.signal.received
	Version        string    `json:"version"`    // Schema version (e.g., "1.0") - maps to event_version in DB

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
	ResourceType      string `json:"resource_type,omitempty"` // e.g., pod, node, deployment
	ResourceID        string `json:"resource_id,omitempty"`   // Resource identifier
	ResourceNamespace string `json:"namespace,omitempty"`     // Kubernetes namespace (DB column: namespace)
	ClusterID         string `json:"cluster_name,omitempty"`  // Cluster identifier (DB column: cluster_name)

	// ========================================
	// AUDIT METADATA (ADR-034)
	// ========================================
	Severity     string `json:"severity,omitempty"`      // 'info', 'warning', 'error', 'critical'
	DurationMs   int    `json:"duration_ms,omitempty"`   // Operation duration in milliseconds
	ErrorCode    string `json:"error_code,omitempty"`    // Specific error code
	ErrorMessage string `json:"error_message,omitempty"` // Detailed error message

	// ========================================
	// ACTOR INFORMATION (ADR-034)
	// ========================================
	ActorID   string `json:"actor_id,omitempty"`   // User, service account, or system
	ActorType string `json:"actor_type,omitempty"` // e.g., user, service_account, system

	// ========================================
	// COMPLIANCE (ADR-034)
	// ========================================
	RetentionDays int  `json:"retention_days"` // Default: 2555 (7 years)
	IsSensitive   bool `json:"is_sensitive"`   // Flag for sensitive data (GDPR, PII)

	// ========================================
	// SOC2 Gap #9: Tamper-Evidence (Hash Chain)
	// ========================================
	EventHash         string `json:"event_hash"`          // SHA256 hash of (previous_event_hash + event_json)
	PreviousEventHash string `json:"previous_event_hash"` // Hash of the previous event in the chain

	// ========================================
	// SOC2 Gap #8: Legal Hold & Retention
	// ========================================
	LegalHold         bool       `json:"legal_hold"`           // Legal hold flag prevents deletion
	LegalHoldReason   string     `json:"legal_hold_reason"`    // Reason for legal hold
	LegalHoldPlacedBy string     `json:"legal_hold_placed_by"` // User who placed legal hold
	LegalHoldPlacedAt *time.Time `json:"legal_hold_placed_at"` // Timestamp when hold was placed

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

// ========================================
// GAP #9: HASH CHAIN IMPLEMENTATION (Tamper-Evidence)
// Authority: AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md - Day 7
// SOC2 Requirement: Tamper-evident audit logs (SOC 2 Type II, NIST 800-53, Sarbanes-Oxley)
// ========================================

// calculateEventHash computes SHA256 hash for blockchain-style chain
// Hash = SHA256(previous_event_hash + event_json)
// This creates an immutable chain where tampering with ANY event breaks the chain
func calculateEventHash(previousHash string, event *AuditEvent) (string, error) {
	// Serialize event to JSON (canonical form for consistent hashing)
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event for hashing: %w", err)
	}

	// Compute hash: SHA256(previous_hash + event_json)
	hasher := sha256.New()
	hasher.Write([]byte(previousHash))
	hasher.Write(eventJSON)
	hashBytes := hasher.Sum(nil)

	return hex.EncodeToString(hashBytes), nil
}

// getPreviousEventHash retrieves the hash of the most recent event for a given correlation_id
// Returns empty string if no previous event exists (first event in chain)
// Uses advisory lock to prevent race conditions during concurrent inserts
func (r *AuditEventsRepository) getPreviousEventHash(ctx context.Context, tx *sql.Tx, correlationID string) (string, error) {
	// Step 1: Acquire advisory lock for this correlation_id (prevents race conditions)
	// Uses PostgreSQL function audit_event_lock_id() from migration 023
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(audit_event_lock_id($1))", correlationID)
	if err != nil {
		return "", fmt.Errorf("failed to acquire advisory lock: %w", err)
	}

	// Step 2: Query last event hash for this correlation_id
	var previousHash sql.NullString
	query := `
		SELECT event_hash
		FROM audit_events
		WHERE correlation_id = $1
		  AND event_hash IS NOT NULL
		ORDER BY event_timestamp DESC, event_id DESC
		LIMIT 1
	`

	err = tx.QueryRowContext(ctx, query, correlationID).Scan(&previousHash)
	if err == sql.ErrNoRows {
		// First event in chain - no previous hash (return empty string)
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query previous event hash: %w", err)
	}

	return previousHash.String, nil
}

// ========================================
// END GAP #9 HASH CHAIN FUNCTIONS
// ========================================

// Create inserts a new audit event into the unified audit_events table with hash chain
// Returns the created event with event_id and created_at populated
// SOC2 Gap #9: Implements blockchain-style hash chain for tamper detection
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
	event.EventDate = DateOnly(time.Date(
		event.EventTimestamp.Year(),
		event.EventTimestamp.Month(),
		event.EventTimestamp.Day(),
		0, 0, 0, 0, time.UTC,
	))

	// Marshal event_data to JSONB
	eventDataJSON, err := json.Marshal(event.EventData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event_data: %w", err)
	}

	// Prepare SQL statement (28 columns - added event_version, parent_event_date for FK constraint)
	// Note: event_date MUST be explicitly set for partitioned tables (triggers don't work on partitions)
	// Calculate event_date from event_timestamp
	eventDate := event.EventTimestamp.Truncate(24 * time.Hour)

	// Set default version if not specified (ADR-034: current version is "1.0")
	version := event.Version
	if version == "" {
		version = "1.0"
	}

	// ========================================
	// SOC2 Gap #9: Hash Chain Integration
	// ========================================
	// Start transaction to ensure advisory lock and insert are atomic
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Get previous event hash (with advisory lock)
	previousHash, err := r.getPreviousEventHash(ctx, tx, event.CorrelationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get previous event hash: %w", err)
	}

	// Calculate current event hash
	eventHash, err := calculateEventHash(previousHash, event)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate event hash: %w", err)
	}

	// Store hashes in event struct
	event.EventHash = eventHash
	event.PreviousEventHash = previousHash
	// ========================================

	query := `
		INSERT INTO audit_events (
			event_id, event_version, event_timestamp, event_date, event_type,
			event_category, event_action, event_outcome,
			correlation_id, parent_event_id, parent_event_date,
			resource_type, resource_id, namespace, cluster_name,
			actor_id, actor_type,
			severity, duration_ms, error_code, error_message,
			retention_days, is_sensitive, event_data,
			event_hash, previous_event_hash,
			legal_hold, legal_hold_reason, legal_hold_placed_by, legal_hold_placed_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11,
			$12, $13, $14, $15,
			$16, $17,
			$18, $19, $20, $21,
			$22, $23, $24,
			$25, $26,
			$27, $28, $29, $30
		)
		RETURNING event_timestamp
	`

	// Handle optional fields with sql.Null* types (ADR-034 schema)
	// V1.0 REFACTOR: Use sqlutil helpers to reduce duplication (Opportunity 2.1)
	parentEventID := sqlutil.ToNullUUID(event.ParentEventID)
	parentEventDate := sqlutil.ToNullTime(event.ParentEventDate)

	// ADR-034: actor_id, actor_type, resource_type, resource_id are NOT NULL (required fields)
	// These are passed as regular strings, not sql.NullString

	// V1.0 REFACTOR: Use sqlutil helpers for optional string fields
	namespace := sqlutil.ToNullStringValue(event.ResourceNamespace)
	clusterName := sqlutil.ToNullStringValue(event.ClusterID)
	errorCode := sqlutil.ToNullStringValue(event.ErrorCode)
	errorMessage := sqlutil.ToNullStringValue(event.ErrorMessage)
	severity := sqlutil.ToNullStringValue(event.Severity)

	// Note: DurationMs stays as sql.NullInt32 (not int64) - keep manual conversion
	var durationMs sql.NullInt32
	if event.DurationMs != 0 {
		durationMs = sql.NullInt32{Int32: int32(event.DurationMs), Valid: true}
	}

	// Set default retention days if not specified (ADR-034: 7 years = 2555 days)
	retentionDays := event.RetentionDays
	if retentionDays == 0 {
		retentionDays = 2555
	}

	// Execute query (ADR-034 schema + Gap #9 hash chain + Gap #8 legal hold - 30 parameters)
	var returnedTimestamp time.Time
	err = tx.QueryRowContext(ctx, query,
		event.EventID,
		version, // event_version
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
		event.EventHash,         // Gap #9: SHA256 hash of (previous_hash + event_json)
		event.PreviousEventHash, // Gap #9: Hash of previous event in chain
		event.LegalHold,         // Gap #8: legal hold flag
		sqlutil.ToNullStringValue(event.LegalHoldReason),   // Gap #8: legal hold reason
		sqlutil.ToNullStringValue(event.LegalHoldPlacedBy), // Gap #8: legal hold placed_by
		sqlutil.ToNullTime(event.LegalHoldPlacedAt),        // Gap #8: legal hold placed_at
	).Scan(&returnedTimestamp)

	if err != nil {
		return nil, fmt.Errorf("failed to insert audit event: %w", err)
	}

	// Commit transaction (releases advisory lock)
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Populate returned timestamp
	event.EventTimestamp = returnedTimestamp

	r.logger.V(1).Info("Audit event created with hash chain",
		"event_id", event.EventID.String(),
		"event_type", event.EventType,
		"event_category", event.EventCategory,
		"correlation_id", event.CorrelationID,
		"event_hash", event.EventHash[:16]+"...", // Log first 16 chars for debugging
		"has_previous_hash", event.PreviousEventHash != "",
	)

	return event, nil
}

// CreateBatch inserts multiple audit events in a single transaction
// DD-AUDIT-002: StoreBatch interface for batch audit event storage
// BR-AUDIT-001: Complete audit trail with no data loss
// Uses a single transaction for atomic batch insert (all succeed or all fail)
func (r *AuditEventsRepository) CreateBatch(ctx context.Context, events []*AuditEvent) ([]*AuditEvent, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("batch cannot be empty")
	}

	// Start transaction for atomic batch insert
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	createdEvents := make([]*AuditEvent, 0, len(events))

	// ========================================
	// SOC2 Gap #9: Hash Chain Integration
	// Group events by correlation_id to maintain hash chains
	// ========================================
	eventsByCorrelation := make(map[string][]*AuditEvent)
	for _, event := range events {
		eventsByCorrelation[event.CorrelationID] = append(eventsByCorrelation[event.CorrelationID], event)
	}

	// Track last hash for each correlation_id within this batch
	lastHashByCorrelation := make(map[string]string)

	// Prepare batch insert statement (includes hash chain columns)
	query := `
		INSERT INTO audit_events (
			event_id, event_timestamp, event_date, event_type,
			event_category, event_action, event_outcome,
			correlation_id, parent_event_id, parent_event_date,
			resource_type, resource_id, namespace, cluster_name,
			actor_id, actor_type,
			severity, duration_ms, error_code, error_message,
			retention_days, is_sensitive, event_data,
			event_hash, previous_event_hash,
			legal_hold, legal_hold_reason, legal_hold_placed_by, legal_hold_placed_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			$11, $12, $13, $14,
			$15, $16,
			$17, $18, $19, $20,
			$21, $22, $23,
			$24, $25,
			$26, $27, $28, $29
		)
		RETURNING event_timestamp
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	// Process each correlation_id group sequentially to maintain chain
	for correlationID, correlationEvents := range eventsByCorrelation {
		// Get previous hash for this correlation_id (with advisory lock)
		previousHash, hashErr := r.getPreviousEventHash(ctx, tx, correlationID)
		if hashErr != nil {
			err = fmt.Errorf("failed to get previous event hash for correlation_id %s: %w", correlationID, hashErr)
			return nil, err
		}

		lastHashByCorrelation[correlationID] = previousHash

		// Process events in this correlation sequentially
		for _, event := range correlationEvents {
			// Generate UUID if not provided
			if event.EventID == uuid.Nil {
				event.EventID = uuid.New()
			}

			// Set event_timestamp if not provided
			if event.EventTimestamp.IsZero() {
				event.EventTimestamp = time.Now().UTC()
			}

			// Set event_date from event_timestamp (for partitioning)
			eventDate := event.EventTimestamp.Truncate(24 * time.Hour)
			event.EventDate = DateOnly(eventDate)

			// Marshal event_data to JSONB
			eventDataJSON, marshalErr := json.Marshal(event.EventData)
			if marshalErr != nil {
				err = fmt.Errorf("failed to marshal event_data for event %s: %w", event.EventID, marshalErr)
				return nil, err
			}

			// Handle optional fields with sql.Null* types
			// V1.0 REFACTOR: Use sqlutil helpers to reduce duplication (Opportunity 2.1)
			parentEventID := sqlutil.ToNullUUID(event.ParentEventID)
			parentEventDate := sqlutil.ToNullTime(event.ParentEventDate)

			// V1.0 REFACTOR: Use sqlutil helpers for optional string fields
			namespace := sqlutil.ToNullStringValue(event.ResourceNamespace)
			clusterName := sqlutil.ToNullStringValue(event.ClusterID)
			errorCode := sqlutil.ToNullStringValue(event.ErrorCode)
			errorMessage := sqlutil.ToNullStringValue(event.ErrorMessage)
			severity := sqlutil.ToNullStringValue(event.Severity)

			// Note: DurationMs stays as sql.NullInt32 (not int64) - keep manual conversion
			var durationMs sql.NullInt32
			if event.DurationMs != 0 {
				durationMs = sql.NullInt32{Int32: int32(event.DurationMs), Valid: true}
			}

			// Set default retention days
			retentionDays := event.RetentionDays
			if retentionDays == 0 {
				retentionDays = 2555
			}

			// ========================================
			// SOC2 Gap #9: Calculate hash chain for this event
			// ========================================
			previousHash := lastHashByCorrelation[event.CorrelationID]

			eventHash, hashErr := calculateEventHash(previousHash, event)
			if hashErr != nil {
				err = fmt.Errorf("failed to calculate event hash for event %s: %w", event.EventID, hashErr)
				return nil, err
			}

			event.EventHash = eventHash
			event.PreviousEventHash = previousHash

			// Update last hash for this correlation_id
			lastHashByCorrelation[event.CorrelationID] = eventHash
			// ========================================

			// Execute insert (Gap #9: includes hash chain + Gap #8: legal hold - 29 parameters)
			var returnedTimestamp time.Time
			execErr := stmt.QueryRowContext(ctx,
				event.EventID,
				event.EventTimestamp,
				eventDate,
				event.EventType,
				event.EventCategory,
				event.EventAction,
				event.EventOutcome,
				event.CorrelationID,
				parentEventID,
				parentEventDate,
				event.ResourceType,
				event.ResourceID,
				namespace,
				clusterName,
				event.ActorID,
				event.ActorType,
				severity,
				durationMs,
				errorCode,
				errorMessage,
				retentionDays,
				event.IsSensitive,
				eventDataJSON,
				event.EventHash,         // Gap #9: SHA256 hash of (previous_hash + event_json)
				event.PreviousEventHash, // Gap #9: Hash of previous event in chain
				event.LegalHold,         // Gap #8: legal hold flag
				sqlutil.ToNullStringValue(event.LegalHoldReason),   // Gap #8: legal hold reason
				sqlutil.ToNullStringValue(event.LegalHoldPlacedBy), // Gap #8: legal hold placed_by
				sqlutil.ToNullTime(event.LegalHoldPlacedAt),        // Gap #8: legal hold placed_at
			).Scan(&returnedTimestamp)

			if execErr != nil {
				err = fmt.Errorf("failed to insert event %s: %w", event.EventID, execErr)
				return nil, err
			}

			event.EventTimestamp = returnedTimestamp
			createdEvents = append(createdEvents, event)
		}
	}

	// Commit transaction (releases all advisory locks)
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit batch transaction: %w", err)
	}

	r.logger.Info("Batch audit events created with hash chains",
		"count", len(createdEvents),
		"correlation_ids", len(eventsByCorrelation),
	)

	return createdEvents, nil
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
	// Safely exclude limit and offset from count query args
	// Fix: Prevent panic if args has fewer than 2 elements
	countArgs := args
	if len(args) >= 2 {
		countArgs = args[:len(args)-2] // Exclude limit and offset for count query
	}
	err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total)
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
		var severity, namespace, clusterName sql.NullString

		err := rows.Scan(
			&event.EventID,
			&event.Version, // event_version from DB (maps to version in OpenAPI)
			&event.EventType,
			&event.EventCategory, // ADR-034
			&event.EventAction,   // ADR-034
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
			&namespace,
			&clusterName,
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
		if namespace.Valid {
			event.ResourceNamespace = namespace.String
		}
		if clusterName.Valid {
			event.ClusterID = clusterName.String
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
	// Safely extract limit and offset from args (default to 0 if not present)
	limit := 0
	offset := 0
	if len(args) >= 2 {
		limit = int(args[len(args)-2].(int))
		offset = int(args[len(args)-1].(int))
	} else if len(args) == 1 {
		limit = int(args[0].(int))
	}
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
