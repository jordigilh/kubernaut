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
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/sqlutil"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/txretry"
)

// CreateBatch inserts multiple audit events in a single transaction
// DD-AUDIT-002: StoreBatch interface for batch audit event storage
//
// Split from audit_events_repository.go (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3, pure code motion, no behavior change). See audit_events_create.go
// for the single-event Create path and audit_events_hashchain.go for the
// shared hash-chain primitives both paths use.

// SortedCorrelationIDs returns the keys of a map in lexicographic order.
// Issue #667 / BR-STORAGE-040: Deterministic ordering prevents advisory lock
// deadlocks when multiple concurrent transactions lock overlapping correlation IDs.
func SortedCorrelationIDs[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// BR-AUDIT-001: Complete audit trail with no data loss
// Uses a single transaction for atomic batch insert (all succeed or all fail).
// Wraps the transaction in a retry loop so that transient PostgreSQL deadlocks
// (40P01) are retried transparently.
func (r *AuditEventsRepository) CreateBatch(ctx context.Context, events []*AuditEvent) ([]*AuditEvent, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("batch cannot be empty")
	}

	// Normalize fields that are idempotent across retries (UUID, timestamp,
	// event_data JSON round-trip, retention defaults). These only need to run
	// once regardless of how many retry attempts occur.
	if err := normalizeBatchEvents(events); err != nil {
		return nil, err
	}

	// Group events by correlation_id (stable across retries).
	eventsByCorrelation := make(map[string][]indexedAuditEvent)
	for i, event := range events {
		eventsByCorrelation[event.CorrelationID] = append(
			eventsByCorrelation[event.CorrelationID],
			indexedAuditEvent{originalIndex: i, event: event},
		)
	}

	// Issue #667 / BR-STORAGE-040: sorted order prevents advisory-lock deadlocks.
	sortedCorrIDs := SortedCorrelationIDs(eventsByCorrelation)

	var createdEvents []*AuditEvent

	err := txretry.WithSerializableRetry(ctx, 3, func() error {
		// Reset hash fields so they are recalculated from the current chain head.
		resetBatchHashFields(events)

		tx, txErr := r.db.BeginTx(ctx, nil)
		if txErr != nil {
			return fmt.Errorf("failed to begin transaction: %w", txErr)
		}
		defer func() {
			if txErr != nil {
				_ = tx.Rollback()
			}
		}()

		stmt, txErr := tx.PrepareContext(ctx, insertAuditEventBatchSQL)
		if txErr != nil {
			return fmt.Errorf("failed to prepare statement: %w", txErr)
		}
		defer func() { _ = stmt.Close() }()

		result, insertErr := r.insertBatchByCorrelation(ctx, tx, stmt, sortedCorrIDs, eventsByCorrelation, len(events))
		if insertErr != nil {
			txErr = insertErr
			return txErr
		}

		if txErr = tx.Commit(); txErr != nil {
			return fmt.Errorf("failed to commit batch transaction: %w", txErr)
		}

		createdEvents = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	r.logger.Info("Batch audit events created with hash chains",
		"count", len(createdEvents),
		"correlation_ids", len(eventsByCorrelation),
	)

	return createdEvents, nil
}

// indexedAuditEvent pairs a batch event with its position in the caller's
// original input slice, so CreateBatch can process events grouped and sorted
// by correlation_id (Issue #667 / BR-STORAGE-040) while still returning
// results in the caller's original order.
type indexedAuditEvent struct {
	originalIndex int
	event         *AuditEvent
}

// insertAuditEventBatchSQL is the prepared-statement INSERT used by
// insertBatchByCorrelation for every event in a CreateBatch call.
const insertAuditEventBatchSQL = `
	INSERT INTO audit_events (
		event_id, event_version, event_timestamp, event_date, event_type,
		event_category, event_action, event_outcome,
		correlation_id, parent_event_id, parent_event_date,
		resource_type, resource_id, namespace, cluster_name,
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

// normalizeBatchEvents applies CreateBatch's idempotent-across-retries field
// defaults (event_id, event_timestamp, event_data JSON round-trip, version,
// retention_days) to each event in place.
func normalizeBatchEvents(events []*AuditEvent) error {
	for _, event := range events {
		normalizeBatchEventIdentity(event)
		if err := normalizeBatchEventData(event); err != nil {
			return err
		}
	}
	return nil
}

// normalizeBatchEventIdentity defaults event's identity/timing/retention
// fields (event_id, event_timestamp/event_date, version, retention_days).
func normalizeBatchEventIdentity(event *AuditEvent) {
	if event.EventID == uuid.Nil {
		event.EventID = uuid.New()
	}
	if event.EventTimestamp.IsZero() {
		event.EventTimestamp = time.Now().UTC()
	}
	event.EventTimestamp = event.EventTimestamp.UTC().Truncate(time.Microsecond)
	event.EventDate = DateOnly(event.EventTimestamp.Truncate(24 * time.Hour))

	if event.Version == "" {
		event.Version = "1.0"
	}
	if event.RetentionDays == 0 {
		event.RetentionDays = 2555
	}
}

// normalizeBatchEventData round-trips event.EventData through JSON so that
// hashing (PrepareEventForHashing) sees the same normalized representation
// regardless of whether the caller passed a typed struct or a raw map.
func normalizeBatchEventData(event *AuditEvent) error {
	eventDataJSON, marshalErr := json.Marshal(event.EventData)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal event_data for event %s: %w", event.EventID, marshalErr)
	}
	if len(eventDataJSON) == 0 || string(eventDataJSON) == "null" {
		return nil
	}
	var normalizedEventData map[string]interface{}
	if unmarshalErr := json.Unmarshal(eventDataJSON, &normalizedEventData); unmarshalErr != nil {
		return fmt.Errorf("failed to normalize event_data for event %s: %w", event.EventID, unmarshalErr)
	}
	event.EventData = normalizedEventData
	return nil
}

// resetBatchHashFields clears each event's hash-chain fields so they are
// recalculated from the current chain head on every retry attempt.
func resetBatchHashFields(events []*AuditEvent) {
	for _, event := range events {
		event.EventHash = ""
		event.PreviousEventHash = ""
		event.HashAlgorithm = ""
	}
}

// insertBatchByCorrelation inserts every event in eventsByCorrelation, one
// correlation_id at a time in sortedCorrIDs order (Issue #667 / BR-STORAGE-040
// deadlock-avoidance ordering), chaining each event's previous_event_hash to
// the prior event hashed for the same correlation_id. Results are placed at
// their original input-slice index so CreateBatch returns events in the
// caller's original order regardless of correlation-id grouping.
func (r *AuditEventsRepository) insertBatchByCorrelation(
	ctx context.Context,
	tx *sql.Tx,
	stmt *sql.Stmt,
	sortedCorrIDs []string,
	eventsByCorrelation map[string][]indexedAuditEvent,
	totalEvents int,
) ([]*AuditEvent, error) {
	result := make([]*AuditEvent, totalEvents)
	lastHashByCorrelation := make(map[string]string, len(sortedCorrIDs))

	for _, correlationID := range sortedCorrIDs {
		previousHash, hashErr := r.getPreviousEventHash(ctx, tx, correlationID)
		if hashErr != nil {
			return nil, fmt.Errorf("failed to get previous event hash for correlation_id %s: %w", correlationID, hashErr)
		}
		lastHashByCorrelation[correlationID] = previousHash

		for _, ie := range eventsByCorrelation[correlationID] {
			newHash, err := r.insertBatchEvent(ctx, stmt, ie.event, lastHashByCorrelation[correlationID])
			if err != nil {
				return nil, err
			}
			lastHashByCorrelation[correlationID] = newHash
			result[ie.originalIndex] = ie.event
		}
	}

	return result, nil
}

// insertBatchEvent computes event's hash-chain link from previousHash,
// stamps it onto event, and executes the prepared batch INSERT. It returns
// the newly computed event hash (the next event's previousHash) on success.
func (r *AuditEventsRepository) insertBatchEvent(ctx context.Context, stmt *sql.Stmt, event *AuditEvent, previousHash string) (string, error) {
	eventDataJSON, marshalErr := json.Marshal(event.EventData)
	if marshalErr != nil {
		return "", fmt.Errorf("failed to marshal event_data for event %s in batch insert: %w", event.EventID, marshalErr)
	}
	eventDate := event.EventTimestamp.Truncate(24 * time.Hour)

	eventHash, hashErr := r.hashEvent(previousHash, event)
	if hashErr != nil {
		return "", fmt.Errorf("failed to calculate event hash for event %s: %w", event.EventID, hashErr)
	}

	event.EventHash = eventHash
	event.PreviousEventHash = previousHash

	parentEventID := sqlutil.ToNullUUID(event.ParentEventID)
	parentEventDate := sqlutil.ToNullTime(event.ParentEventDate)
	namespace := sqlutil.ToNullStringValue(event.ResourceNamespace)
	clusterName := sqlutil.ToNullStringValue(event.ClusterID)
	errorCode := sqlutil.ToNullStringValue(event.ErrorCode)
	errorMessage := sqlutil.ToNullStringValue(event.ErrorMessage)
	severity := sqlutil.ToNullStringValue(event.Severity)
	actorIP := sqlutil.ToNullStringValue(event.ActorIP)

	var durationMs sql.NullInt32
	if event.DurationMs != 0 {
		durationMs = sql.NullInt32{Int32: int32(event.DurationMs), Valid: true}
	}

	var returnedTimestamp time.Time
	err := stmt.QueryRowContext(ctx,
		event.EventID,
		event.Version,
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
	).Scan(&returnedTimestamp)
	if err != nil {
		return "", fmt.Errorf("failed to insert event %s: %w", event.EventID, err)
	}

	return eventHash, nil
}
