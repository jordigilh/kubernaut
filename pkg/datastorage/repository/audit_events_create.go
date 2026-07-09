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

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/sqlutil"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/txretry"
)

// Single-event Create for the unified audit_events table (ADR-034 hash
// chain). Split from audit_events_repository.go (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3, pure code motion, no behavior change). See audit_events_batch.go
// for the multi-event CreateBatch path and audit_events_hashchain.go for the
// shared hash-chain primitives both paths use.

// normalizeCreateEvent applies Create's field defaults (event_id,
// event_timestamp normalization, event_date, version, retention_days) and
// normalizes event.EventData through a JSON round-trip in place, returning
// the JSON bytes to persist and the resolved version string.
//
// The JSON round-trip is required for hash consistency: PostgreSQL JSONB
// normalizes all numbers to float64, so the hash calculated at INSERT time
// must be computed against the same normalized representation that
// Export/verify-chain will see when reading the row back (see
// audit_export.go and audit_verify_chain_handler.go).
func normalizeCreateEvent(event *AuditEvent) ([]byte, string, error) {
	if event.EventID == uuid.Nil {
		event.EventID = uuid.New()
	}
	if event.EventTimestamp.IsZero() {
		event.EventTimestamp = time.Now().UTC()
	}
	// Force UTC + microsecond-precision truncation before hash calculation to
	// match PostgreSQL timestamptz precision (see Create's original comment
	// for the exact failure mode this prevents).
	event.EventTimestamp = event.EventTimestamp.UTC().Truncate(time.Microsecond)
	event.EventDate = DateOnly(time.Date(
		event.EventTimestamp.Year(),
		event.EventTimestamp.Month(),
		event.EventTimestamp.Day(),
		0, 0, 0, 0, time.UTC,
	))

	eventDataJSON, err := json.Marshal(event.EventData)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal event_data: %w", err)
	}
	if len(eventDataJSON) > 0 && string(eventDataJSON) != "null" {
		var normalizedEventData map[string]interface{}
		if err := json.Unmarshal(eventDataJSON, &normalizedEventData); err != nil {
			return nil, "", fmt.Errorf("failed to normalize event_data: %w", err)
		}
		event.EventData = normalizedEventData // Replace with normalized version for hash calculation
		eventDataJSON, err = json.Marshal(normalizedEventData)
		if err != nil {
			return nil, "", fmt.Errorf("failed to remarshal normalized event_data: %w", err)
		}
	}

	version := event.Version
	if version == "" {
		version = "1.0"
	}

	// CRITICAL: Set default retention days BEFORE hash calculation so the hash
	// includes the correct retention_days value (2555) instead of 0, matching
	// what will be read back from the DB during verification.
	if event.RetentionDays == 0 {
		event.RetentionDays = 2555
	}

	return eventDataJSON, version, nil
}

// Create inserts a new audit event into the unified audit_events table with hash chain
// Returns the created event with event_id and created_at populated
// SOC2 Gap #9: Implements blockchain-style hash chain for tamper detection
func (r *AuditEventsRepository) Create(ctx context.Context, event *AuditEvent) (*AuditEvent, error) {
	eventDataJSON, version, err := normalizeCreateEvent(event)
	if err != nil {
		return nil, err
	}

	// Calculate event_date from event_timestamp (28-column INSERT below)
	eventDate := event.EventTimestamp.Truncate(24 * time.Hour)

	// ========================================
	// SOC2 Gap #9: Hash Chain Integration
	// ========================================
	// Wrap the transactional portion in a retry loop so that transient
	// PostgreSQL deadlocks (40P01) are retried transparently. The hash
	// fields are recalculated on each attempt because they depend on the
	// current chain head, which may change between retries.
	err = txretry.WithSerializableRetry(ctx, 3, func() error {
		event.EventHash = ""
		event.PreviousEventHash = ""

		tx, txErr := r.db.BeginTx(ctx, nil)
		if txErr != nil {
			return fmt.Errorf("failed to begin transaction: %w", txErr)
		}
		defer func() {
			if txErr != nil {
				_ = tx.Rollback()
			}
		}()

		previousHash, txErr := r.getPreviousEventHash(ctx, tx, event.CorrelationID)
		if txErr != nil {
			return fmt.Errorf("failed to get previous event hash: %w", txErr)
		}

		eventHash, txErr := r.hashEvent(previousHash, event)
		if txErr != nil {
			return fmt.Errorf("failed to calculate event hash: %w", txErr)
		}

		event.EventHash = eventHash
		event.PreviousEventHash = previousHash

		txErr = execCreateInsert(ctx, tx, event, eventDataJSON, version, eventDate)
		if txErr != nil {
			return fmt.Errorf("failed to insert audit event: %w", txErr)
		}

		if txErr = tx.Commit(); txErr != nil {
			return fmt.Errorf("failed to commit transaction: %w", txErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

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

// createAuditEventSQL is the single-row INSERT used by execCreateInsert.
const createAuditEventSQL = `
	INSERT INTO audit_events (
		event_id, event_version, event_timestamp, event_date, event_type,
		event_category, event_action, event_outcome,
		correlation_id, parent_event_id, parent_event_date,
		resource_type, resource_id, namespace, cluster_id,
		actor_id, actor_type, actor_ip,
		severity, duration_ms, error_code, error_message,
		retention_days, is_sensitive, event_data,
		event_hash, previous_event_hash, hash_algorithm,
		legal_hold, legal_hold_reason, legal_hold_placed_by, legal_hold_placed_at
	) VALUES (
		$1, $2, $3, $4, $5,
		$6, $7, $8,
		$9, $10, $11,
		$12, $13, $14, $15,
		$16, $17, $18,
		$19, $20, $21, $22,
		$23, $24, $25,
		$26, $27, $28,
		$29, $30, $31, $32
	)
	RETURNING event_timestamp
`

// execCreateInsert executes Create's single-row INSERT within tx, assuming
// event's hash-chain fields (EventHash, PreviousEventHash, HashAlgorithm)
// have already been stamped by the caller's hashEvent call.
func execCreateInsert(ctx context.Context, tx *sql.Tx, event *AuditEvent, eventDataJSON []byte, version string, eventDate time.Time) error {
	parentEventID := sqlutil.ToNullUUID(event.ParentEventID)
	parentEventDate := sqlutil.ToNullTime(event.ParentEventDate)
	namespace := sqlutil.ToNullStringValue(event.ResourceNamespace)
	clusterID := sqlutil.ToNullStringValue(event.ClusterID)
	errorCode := sqlutil.ToNullStringValue(event.ErrorCode)
	errorMessage := sqlutil.ToNullStringValue(event.ErrorMessage)
	severity := sqlutil.ToNullStringValue(event.Severity)
	actorIP := sqlutil.ToNullStringValue(event.ActorIP)

	var durationMs sql.NullInt32
	if event.DurationMs != 0 {
		durationMs = sql.NullInt32{Int32: int32(event.DurationMs), Valid: true}
	}

	var ignoredTimestamp time.Time
	return tx.QueryRowContext(ctx, createAuditEventSQL,
		event.EventID,
		version,
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
		clusterID,
		event.ActorID,
		event.ActorType,
		actorIP,
		severity,
		durationMs,
		errorCode,
		errorMessage,
		event.RetentionDays,
		event.IsSensitive,
		eventDataJSON,
		event.EventHash,
		event.PreviousEventHash,
		event.HashAlgorithm,
		event.LegalHold,
		sqlutil.ToNullStringValue(event.LegalHoldReason),
		sqlutil.ToNullStringValue(event.LegalHoldPlacedBy),
		sqlutil.ToNullTime(event.LegalHoldPlacedAt),
	).Scan(&ignoredTimestamp)
}
