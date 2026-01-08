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
)

// ========================================
// SOC2 Day 9.1: Audit Export with Hash Chain Verification
// Authority: SOC2_WEEK2_COMPLETE_PLAN_V1_1_JAN07.md - Day 9
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
	StartTime      *time.Time
	EndTime        *time.Time
	CorrelationID  string
	EventCategory  string
	Offset         int
	Limit          int
	RedactPII      bool // SOC2 Day 10.2: Enable PII redaction for privacy compliance
}

// ExportEvent represents an audit event with hash chain validation
type ExportEvent struct {
	*AuditEvent
	HashChainValid bool `json:"hash_chain_valid"` // Whether this event's hash chain is intact
}

// ExportResult contains the exported events and verification statistics
type ExportResult struct {
	Events                 []*ExportEvent
	TotalEventsQueried     int
	ValidChainEvents       int
	BrokenChainEvents      int
	ChainIntegrityPercent  float32
	TamperedEventIDs       []string
	VerificationTimestamp  time.Time
}

// Export retrieves audit events matching the filters and verifies hash chain integrity
// BR-AUDIT-007: Audit export with tamper-evident hash chain verification
func (r *AuditEventsRepository) Export(ctx context.Context, filters ExportFilters) (*ExportResult, error) {
	// Build dynamic query based on filters
	query := `
		SELECT
			event_id, event_version, event_type, event_timestamp,
			event_category, event_action, event_outcome, correlation_id,
			parent_event_id, parent_event_date, resource_type, resource_id,
			namespace, cluster_name, actor_id, actor_type,
			severity, duration_ms, error_code, error_message,
			retention_days, is_sensitive, event_data,
			event_hash, previous_event_hash, legal_hold
		FROM audit_events
		WHERE 1=1
	`

	args := make([]interface{}, 0)
	argIndex := 1

	// Apply filters
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

	// Order by timestamp for consistent export
	query += " ORDER BY event_timestamp ASC, event_id ASC"

	// Apply pagination (default limit to 1000 if not specified)
	limit := filters.Limit
	if limit == 0 {
		limit = 1000 // Default page size
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, filters.Offset)

	// Execute query
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		r.logger.Error(err, "Failed to query audit events for export")
		return nil, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer rows.Close()

	var events []*AuditEvent
	for rows.Next() {
		event := &AuditEvent{}
		var eventDataJSON []byte
		var legalHold bool

		// Use sql.NullString for nullable string columns and sql.NullInt64 for nullable int columns
		// to handle NULL values from database
		var resourceType, resourceID, resourceNamespace, clusterID sql.NullString
		var actorID, actorType, severity, errorCode, errorMessage sql.NullString
		var durationMs sql.NullInt64

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
			&resourceType,
			&resourceID,
			&resourceNamespace,
			&clusterID,
			&actorID,
			&actorType,
			&severity,
			&durationMs,
			&errorCode,
			&errorMessage,
			&event.RetentionDays,
			&event.IsSensitive,
			&eventDataJSON,
			&event.EventHash,
			&event.PreviousEventHash,
			&legalHold,
		)
		if err != nil {
			r.logger.Error(err, "Failed to scan audit event row")
			return nil, fmt.Errorf("failed to scan audit event: %w", err)
		}

		// Convert sql.NullString to regular strings
		// NULL → empty string, which will be omitted by `omitempty` JSON tags during hash calculation
		// This preserves the original JSON structure for hash verification
		event.ResourceType = resourceType.String
		event.ResourceID = resourceID.String
		event.ResourceNamespace = resourceNamespace.String
		event.ClusterID = clusterID.String
		event.ActorID = actorID.String
		event.ActorType = actorType.String
		event.Severity = severity.String
		event.ErrorCode = errorCode.String
		event.ErrorMessage = errorMessage.String

		// Convert sql.NullInt64 to int
		// NULL → 0, which will be omitted by `omitempty` JSON tag during hash calculation
		event.DurationMs = int(durationMs.Int64)

		// Unmarshal event_data JSON
		if len(eventDataJSON) > 0 {
			if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
				r.logger.Error(err, "Failed to unmarshal event_data", "event_id", event.EventID)
				return nil, fmt.Errorf("failed to unmarshal event_data: %w", err)
			}
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error(err, "Error iterating audit event rows")
		return nil, fmt.Errorf("error iterating audit events: %w", err)
	}

	// Verify hash chains
	result := &ExportResult{
		Events:                 make([]*ExportEvent, 0, len(events)),
		TotalEventsQueried:     len(events),
		ValidChainEvents:       0,
		BrokenChainEvents:      0,
		TamperedEventIDs:       make([]string, 0),
		VerificationTimestamp:  time.Now().UTC(),
	}

	if len(events) == 0 {
		result.ChainIntegrityPercent = 100.0 // No events = no tampering
		return result, nil
	}

	// Group events by correlation_id for chain verification
	eventsByCorrelation := make(map[string][]*AuditEvent)
	for _, event := range events {
		eventsByCorrelation[event.CorrelationID] = append(eventsByCorrelation[event.CorrelationID], event)
	}

	// Verify each correlation_id's hash chain
	for correlationID, corrEvents := range eventsByCorrelation {
		// Sort by timestamp (should already be sorted from query, but ensure)
		// Events are already ordered by query: ORDER BY event_timestamp ASC, event_id ASC

		previousHash := ""
		for i, event := range corrEvents {
			exportEvent := &ExportEvent{
				AuditEvent:     event,
				HashChainValid: true, // Assume valid until proven otherwise
			}

			// Skip verification if no hash data (legacy events)
			if event.EventHash == "" && event.PreviousEventHash == "" {
				exportEvent.HashChainValid = true // Legacy events are not considered tampered
				result.Events = append(result.Events, exportEvent)
				result.ValidChainEvents++
				continue
			}

			// Verify previous_event_hash matches
			if event.PreviousEventHash != previousHash {
				exportEvent.HashChainValid = false
				result.BrokenChainEvents++
				result.TamperedEventIDs = append(result.TamperedEventIDs, event.EventID.String())
				r.logger.Info("Hash chain broken: previous_event_hash mismatch",
					"event_id", event.EventID,
					"correlation_id", correlationID,
					"expected_previous_hash", previousHash,
					"actual_previous_hash", event.PreviousEventHash)
			} else {
				// Calculate expected hash for this event
				expectedHash, err := calculateEventHashForVerification(previousHash, event)
				if err != nil {
					r.logger.Error(err, "Failed to calculate expected hash", "event_id", event.EventID)
					return nil, fmt.Errorf("failed to calculate expected hash: %w", err)
				}

				// Verify event_hash matches calculated hash
				if event.EventHash != expectedHash {
					exportEvent.HashChainValid = false
					result.BrokenChainEvents++
					result.TamperedEventIDs = append(result.TamperedEventIDs, event.EventID.String())
					r.logger.Info("Hash chain broken: event_hash mismatch (tampering detected)",
						"event_id", event.EventID,
						"correlation_id", correlationID,
						"expected_hash", expectedHash,
						"actual_hash", event.EventHash)
				} else {
					result.ValidChainEvents++
				}

				// First event should have empty previous_hash
				if i == 0 && previousHash != "" {
					exportEvent.HashChainValid = false
					result.BrokenChainEvents++
					result.TamperedEventIDs = append(result.TamperedEventIDs, event.EventID.String())
					r.logger.Info("Hash chain broken: first event has non-empty previous_hash",
						"event_id", event.EventID,
						"correlation_id", correlationID,
						"previous_hash", previousHash)
				}
			}

			result.Events = append(result.Events, exportEvent)

			// Update previous hash for next iteration
			previousHash = event.EventHash
		}
	}

	// Calculate chain integrity percentage
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

// calculateEventHashForVerification computes the expected SHA256 hash for verification
// Must match the calculateEventHash logic in audit_events_repository.go
func calculateEventHashForVerification(previousHash string, event *AuditEvent) (string, error) {
	// CRITICAL: Clear fields to match INSERT-time state
	// This MUST match the logic in calculateEventHash() in audit_events_repository.go
	// 1. EventHash/PreviousEventHash: Not yet calculated during INSERT
	// 2. EventDate: Derived from EventTimestamp (not stored separately in hash)
	// Note: EventTimestamp IS included in hash (set before calculation during INSERT)
	eventCopy := *event
	eventCopy.EventHash = ""
	eventCopy.PreviousEventHash = ""
	eventCopy.EventDate = DateOnly{} // Clear derived field only

	// Serialize event to JSON (canonical form for consistent hashing)
	eventJSON, err := json.Marshal(&eventCopy)
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

