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
)

// ========================================
// SOC2 Day 9.1: Audit Export with Hash Chain Verification
// Authority: BR-AUDIT-007 (Tamper-evident audit exports)
// ========================================
//
// Exports audit events with tamper-evident hash chain verification for compliance audits.
//
// SOC2 Requirements:
// - CC8.1: Audit Export for external compliance reviews
// - AU-9: Protection of Audit Information (tamper-evident exports)
// - Sarbanes-Oxley: Section 404 (Internal Controls - audit trail integrity)
//
// Features:
// - Query filtering (time range, correlation_id, event_category)
// - Pagination support (offset, limit)
// - Hash chain verification per event (hash_chain_valid flag)
// - Export metadata with statistics
//
// ========================================

// ExportFilters contains the query filters for audit export
type ExportFilters struct {
	StartTime     *time.Time
	EndTime       *time.Time
	CorrelationID string
	EventCategory string
	Offset        int
	Limit         int
	RedactPII     bool // SOC2 Day 10.2: Enable PII redaction for privacy compliance
}

// ExportEvent represents an audit event with hash chain validation
type ExportEvent struct {
	*AuditEvent
	HashChainValid bool `json:"hash_chain_valid"` // Whether this event's hash chain is intact
}

// ExportResult contains the exported events and verification statistics
type ExportResult struct {
	Events                []*ExportEvent
	TotalEventsQueried    int
	ValidChainEvents      int
	BrokenChainEvents     int
	ChainIntegrityPercent float32
	TamperedEventIDs      *[]string // Pointer to slice to match OpenAPI client expectation
	VerificationTimestamp time.Time
}

// Export retrieves audit events matching the filters and verifies hash chain integrity
// BR-AUDIT-007: Audit export with tamper-evident hash chain verification
func (r *AuditEventsRepository) Export(ctx context.Context, filters ExportFilters) (*ExportResult, error) {
	query, args := buildExportQuery(filters)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		r.logger.Error(err, "Failed to query audit events for export")
		return nil, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.Error(err, "Failed to close database rows")
		}
	}()

	var events []*AuditEvent
	for rows.Next() {
		event, err := scanExportRow(rows)
		if err != nil {
			r.logger.Error(err, "Failed to scan audit event row for export")
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		r.logger.Error(err, "Error iterating audit event rows")
		return nil, fmt.Errorf("error iterating audit events: %w", err)
	}

	return r.verifyExportChains(events)
}

// buildExportQuery constructs Export's dynamic SELECT and positional args
// from filters (time range, correlation_id, event_category, pagination).
// Limit defaults to 1000 when unset.
func buildExportQuery(filters ExportFilters) (string, []interface{}) {
	query := `
		SELECT
			event_id, event_version, event_type, event_timestamp,
			event_category, event_action, event_outcome, correlation_id,
			parent_event_id, parent_event_date, resource_type, resource_id,
			namespace, cluster_id, actor_id, actor_type, actor_ip,
			severity, duration_ms, error_code, error_message,
			retention_days, is_sensitive, event_data,
			event_hash, previous_event_hash, hash_algorithm, legal_hold
		FROM audit_events
		WHERE 1=1
	`

	args := make([]interface{}, 0)
	argIndex := 1

	if filters.StartTime != nil {
		query += fmt.Sprintf(" AND event_timestamp >= $%d", argIndex)
		args = append(args, *filters.StartTime)
		argIndex++
	}
	if filters.EndTime != nil {
		query += fmt.Sprintf(" AND event_timestamp <= $%d", argIndex)
		args = append(args, *filters.EndTime)
		argIndex++
	}
	if filters.CorrelationID != "" {
		query += fmt.Sprintf(" AND correlation_id = $%d", argIndex)
		args = append(args, filters.CorrelationID)
		argIndex++
	}
	if filters.EventCategory != "" {
		query += fmt.Sprintf(" AND event_category = $%d", argIndex)
		args = append(args, filters.EventCategory)
		argIndex++
	}

	query += " ORDER BY event_timestamp ASC, event_id ASC"

	limit := filters.Limit
	if limit == 0 {
		limit = 1000 // Default page size
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, filters.Offset)

	return query, args
}

// scanExportRow scans one Export result row into an *AuditEvent, converting
// nullable SQL columns to plain-typed fields (NULL -> zero value, matching
// the `omitempty` JSON tags used during hash calculation) and unmarshaling
// the event_data JSONB payload.
// exportRowNullableColumns groups the sql.Null* scan intermediates for
// scanExportRow's nullable columns, so assignExportNullableFields can take
// them as a single argument.
type exportRowNullableColumns struct {
	resourceType, resourceID, resourceNamespace, clusterID         sql.NullString
	actorID, actorType, actorIP, severity, errorCode, errorMessage sql.NullString
	durationMs                                                     sql.NullInt64
	legalHold                                                      bool
}

func scanExportRow(rows *sql.Rows) (*AuditEvent, error) {
	event := &AuditEvent{}
	var eventDataJSON []byte
	var cols exportRowNullableColumns

	err := rows.Scan(
		&event.EventID,
		&event.Version,
		&event.EventType,
		&event.EventTimestamp,
		&event.EventCategory,
		&event.EventAction,
		&event.EventOutcome,
		&event.CorrelationID,
		&event.ParentEventID,
		&event.ParentEventDate,
		&cols.resourceType,
		&cols.resourceID,
		&cols.resourceNamespace,
		&cols.clusterID,
		&cols.actorID,
		&cols.actorType,
		&cols.actorIP,
		&cols.severity,
		&cols.durationMs,
		&cols.errorCode,
		&cols.errorMessage,
		&event.RetentionDays,
		&event.IsSensitive,
		&eventDataJSON,
		&event.EventHash,
		&event.PreviousEventHash,
		&event.HashAlgorithm,
		&cols.legalHold,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan audit event: %w", err)
	}

	// CRITICAL: Force timestamp to UTC for hash consistency. PostgreSQL
	// timestamptz stores in UTC but Go reads it with local timezone; without
	// this conversion, JSON marshaling would produce a different string than
	// the UTC form used at INSERT time, breaking hash verification.
	event.EventTimestamp = event.EventTimestamp.UTC()

	assignExportNullableFields(event, cols)

	if len(eventDataJSON) > 0 {
		if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event_data: %w", err)
		}
	}

	return event, nil
}

// assignExportNullableFields copies the scanned sql.Null* intermediates onto
// event's plain Go fields. Extracted from scanExportRow (Wave 6 6f GREEN:
// funlen remediation) — pure code motion, no behavior change.
func assignExportNullableFields(event *AuditEvent, cols exportRowNullableColumns) {
	event.ResourceType = cols.resourceType.String
	event.ResourceID = cols.resourceID.String
	event.ResourceNamespace = cols.resourceNamespace.String
	event.ClusterID = cols.clusterID.String
	event.ActorID = cols.actorID.String
	event.ActorType = cols.actorType.String
	event.ActorIP = cols.actorIP.String
	event.Severity = cols.severity.String
	event.ErrorCode = cols.errorCode.String
	event.ErrorMessage = cols.errorMessage.String
	event.DurationMs = int(cols.durationMs.Int64)
	event.LegalHold = cols.legalHold
}

// verifyExportChains groups events by correlation_id and verifies each
// correlation's hash chain (GAP-05: algorithm-aware), building the final
// ExportResult with per-event validity flags and aggregate statistics.
func (r *AuditEventsRepository) verifyExportChains(events []*AuditEvent) (*ExportResult, error) {
	// TamperedEventIDs is a pointer to an empty (not nil) slice so JSON
	// serialization produces [] instead of null, matching the OpenAPI client.
	tamperedIDs := make([]string, 0)
	result := &ExportResult{
		Events:                make([]*ExportEvent, 0, len(events)),
		TamperedEventIDs:      &tamperedIDs,
		TotalEventsQueried:    len(events),
		VerificationTimestamp: time.Now().UTC(),
	}

	if len(events) == 0 {
		result.ChainIntegrityPercent = 100.0 // No events = no tampering
		return result, nil
	}

	eventsByCorrelation := make(map[string][]*AuditEvent)
	for _, event := range events {
		eventsByCorrelation[event.CorrelationID] = append(eventsByCorrelation[event.CorrelationID], event)
	}

	// Events within each correlation_id are already ordered by the query
	// (ORDER BY event_timestamp ASC, event_id ASC).
	for correlationID, corrEvents := range eventsByCorrelation {
		previousHash := ""
		for i, event := range corrEvents {
			exportEvent, err := r.verifyExportEvent(correlationID, previousHash, i == 0, event)
			if err != nil {
				return nil, err
			}
			if exportEvent.HashChainValid {
				result.ValidChainEvents++
			} else {
				result.BrokenChainEvents++
				*result.TamperedEventIDs = append(*result.TamperedEventIDs, event.EventID.String())
			}
			result.Events = append(result.Events, exportEvent)
			previousHash = event.EventHash
		}
	}

	if result.TotalEventsQueried > 0 {
		result.ChainIntegrityPercent = float32((float64(result.ValidChainEvents) / float64(result.TotalEventsQueried)) * 100.0)
	} else {
		result.ChainIntegrityPercent = 100.0
	}

	r.logger.Info("Audit export completed",
		"total_events", result.TotalEventsQueried,
		"valid_chain", result.ValidChainEvents,
		"broken_chain", result.BrokenChainEvents,
		"integrity_percent", result.ChainIntegrityPercent)

	return result, nil
}

// verifyExportEvent verifies one event's position in its correlation_id's
// hash chain against previousHash (the prior event's event_hash, or "" for
// the first event / legacy events with no hash data). GAP-05: uses
// CalculateHashForVerification so the check is algorithm-aware (unkeyed
// SHA256 vs. keyed HMAC-SHA256) per event.
func (r *AuditEventsRepository) verifyExportEvent(correlationID, previousHash string, isFirst bool, event *AuditEvent) (*ExportEvent, error) {
	exportEvent := &ExportEvent{AuditEvent: event, HashChainValid: true}

	// Legacy events (pre-hash-chain) carry no hash data and are never
	// considered tampered.
	if event.EventHash == "" && event.PreviousEventHash == "" {
		return exportEvent, nil
	}

	if event.PreviousEventHash != previousHash {
		exportEvent.HashChainValid = false
		r.logger.Info("Hash chain broken: previous_event_hash mismatch",
			"event_id", event.EventID,
			"correlation_id", correlationID,
			"expected_previous_hash", previousHash,
			"actual_previous_hash", event.PreviousEventHash)
		return exportEvent, nil
	}

	expectedHash, err := CalculateHashForVerification(r.hmacKey, previousHash, event)
	if err != nil {
		r.logger.Error(err, "Failed to calculate expected hash", "event_id", event.EventID)
		return nil, fmt.Errorf("failed to calculate expected hash: %w", err)
	}

	if event.EventHash != expectedHash {
		exportEvent.HashChainValid = false
		r.logger.Info("Hash chain broken: event_hash mismatch (tampering detected)",
			"event_id", event.EventID,
			"correlation_id", correlationID,
			"expected_hash", expectedHash,
			"actual_hash", event.EventHash)
		return exportEvent, nil
	}

	// First event in a chain must have an empty previous_hash.
	if isFirst && previousHash != "" {
		exportEvent.HashChainValid = false
		r.logger.Info("Hash chain broken: first event has non-empty previous_hash",
			"event_id", event.EventID,
			"correlation_id", correlationID,
			"previous_hash", previousHash)
	}

	return exportEvent, nil
}
