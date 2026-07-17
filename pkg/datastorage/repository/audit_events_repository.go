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
	"database/sql"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

// jsonNull is the JSON literal for a null value, used when checking/writing
// raw JSON bytes for optional event_data payloads.
const jsonNull = "null"

// defaultAuditEventVersion is the audit event schema version stamped by the
// repository when a persisted event arrives with no Version set.
const defaultAuditEventVersion = "1.0"

// DateOnly is a time.Time that serializes to JSON as date-only format (YYYY-MM-DD)
// This is required because the OpenAPI spec defines event_date as format: date
// and oapi-codegen generates openapi_types.Date which expects "2025-12-16" not "2025-12-16T00:00:00Z"
type DateOnly time.Time

// MarshalJSON serializes DateOnly to date-only format "YYYY-MM-DD"
func (d DateOnly) MarshalJSON() ([]byte, error) {
	t := time.Time(d)
	if t.IsZero() {
		return []byte(jsonNull), nil
	}
	return []byte(fmt.Sprintf(`"%s"`, t.Format("2006-01-02"))), nil
}

// UnmarshalJSON deserializes date-only format "YYYY-MM-DD" to DateOnly
func (d *DateOnly) UnmarshalJSON(data []byte) error {
	// Handle null
	if string(data) == jsonNull {
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
// File layout (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3 split, pure code
// motion, no behavior change):
//   - audit_events_repository.go (this file): AuditEvent model, DateOnly,
//     AuditEventsRepository struct + constructor
//   - audit_events_hashchain.go: SOC2 Gap #9 hash-chain primitives
//   - audit_events_create.go: single-event Create
//   - audit_events_batch.go: multi-event CreateBatch
//   - audit_events_query.go: Query, row-scanning, HealthCheck
//   - audit_export.go: Export (CSV/JSON audit export)
//
// ========================================

// AuditEvent is the PostgreSQL persistence model for the unified audit_events table (ADR-034).
//
// Triple alignment (authority: migrations/001_v1_schema.sql + incremental 002–010; pkg/shared/assets/migrations/;
// REST contract api/openapi/data-storage-v1.yaml — AuditEventRequest for writes; AuditEvent for query payloads):
//
// Columns on this model match INSERT statements and pkg/datastorage/query Build() SELECT column order.
//
// Persisted audit_events columns not represented on this struct (left NULL unless future writers add them):
// actor_ip INET, resource_name, event_metadata JSONB, trace_id, span_id. Upstream callers may populate
// these via pkg/audit.AuditEvent; repository conversion/INSERT paths omit them intentionally today.
//
// Type mapping: legal_hold_placed_at and event_timestamp map to PostgreSQL TIMESTAMP WITH TIME ZONE (UTC);
// event_date and parent_event_date map to DATE (partition keys); ParentEventDate uses *time.Time in JSON for API ergonomics.
//
// Wide DTO by design: reviewed in GO-ANTIPATTERN-AUDIT-2026-07-01 §4a (God Structs).
// Splitting it would fragment the persistence boundary this struct exists to mirror,
// for no behavioral gain, so it is intentionally not decomposed.
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
	ActorIP   string `json:"actor_ip,omitempty"`   // Source IP (SOC2 CC7.2 / AU-3)

	// ========================================
	// COMPLIANCE (ADR-034)
	// ========================================
	RetentionDays int  `json:"retention_days"` // Default: 2555 (7 years)
	IsSensitive   bool `json:"is_sensitive"`   // Flag for sensitive data (GDPR, PII)

	// ========================================
	// SOC2 Gap #9: Tamper-Evidence (Hash Chain)
	// ========================================
	EventHash         string `json:"event_hash"`          // Hash of (previous_event_hash + event_json); see HashAlgorithm
	PreviousEventHash string `json:"previous_event_hash"` // Hash of the previous event in the chain

	// HashAlgorithm records which algorithm produced EventHash: HashAlgorithmHMACSHA256
	// (keyed, GAP-05/Issue #1505) or HashAlgorithmSHA256Unkeyed (legacy default).
	// Excluded from the hash payload itself (see PrepareEventForHashing) so that
	// events written before GAP-05 continue to verify against their original hash.
	HashAlgorithm string `json:"hash_algorithm,omitempty"`

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

	// hmacKey enables keyed HMAC-SHA256 hash chaining (GAP-05, Issue #1505) for
	// newly written events. When empty, the repository falls back to the legacy
	// unkeyed SHA256 algorithm for backward compatibility with environments that
	// have not yet provisioned the datastorage audit HMAC key secret.
	hmacKey []byte
}

// AuditEventsRepositoryOption configures optional AuditEventsRepository behavior.
type AuditEventsRepositoryOption func(*AuditEventsRepository)

// WithHMACKey enables keyed HMAC-SHA256 hash chaining (GAP-05, Issue #1505).
// A nil/empty key is a no-op, preserving the legacy unkeyed SHA256 algorithm.
func WithHMACKey(key []byte) AuditEventsRepositoryOption {
	return func(r *AuditEventsRepository) {
		if len(key) > 0 {
			r.hmacKey = key
		}
	}
}

// NewAuditEventsRepository creates a new repository instance
func NewAuditEventsRepository(db *sql.DB, logger logr.Logger, opts ...AuditEventsRepositoryOption) *AuditEventsRepository {
	r := &AuditEventsRepository{
		db:     db,
		logger: logger,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// HMACKey returns the configured HMAC key, or nil when keyed hashing is disabled.
// GAP-05 (Issue #1505): exposed so verification paths outside this package (e.g.
// the /api/v1/audit/verify-chain HTTP handler) can recompute hmac-sha256 hashes
// using the same key the repository uses at write time.
func (r *AuditEventsRepository) HMACKey() []byte {
	return r.hmacKey
}
