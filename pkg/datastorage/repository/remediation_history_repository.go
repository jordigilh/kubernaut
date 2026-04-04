/*
Copyright 2026 Jordi Gil.

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

// Package repository provides data access for the DataStorage service.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// DD-HAPI-016 v1.4: Both tiers query by spec hash for causal chain integrity (#586).
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
)

// RawAuditRow represents a single audit event row from the database.
// Used as an intermediate representation before correlation logic in the handler.
type RawAuditRow struct {
	EventType      string
	EventData      map[string]interface{}
	EventTimestamp time.Time
	CorrelationID  string
}

// EffectivenessEventRow represents a parsed EM component audit event.
// Mirrors the EffectivenessEvent type in effectiveness_handler.go but lives
// in the repository package to avoid circular imports.
type EffectivenessEventRow struct {
	EventData map[string]interface{}
}

// RemediationHistoryRepository provides queries for remediation history context.
// DD-HAPI-016 v1.4, Issue #616: Both tiers query RO events by spec hash, matching
// BOTH pre_remediation_spec_hash (direct) and post_remediation_spec_hash (via EM correlation).
//  1. Query RO events by spec hash (Tier 1: 24h window, Tier 2: 90d window)
//  2. Batch query EM component events by correlation_id
type RemediationHistoryRepository struct {
	db     *sql.DB
	logger logr.Logger
}

// NewRemediationHistoryRepository creates a new RemediationHistoryRepository.
func NewRemediationHistoryRepository(db *sql.DB, logger logr.Logger) *RemediationHistoryRepository {
	return &RemediationHistoryRepository{
		db:     db,
		logger: logger.WithName("remediation-history-repository"),
	}
}

// scanRawRows scans sql.Rows into a slice of RawAuditRow.
// Each row must have columns: event_type, event_data (JSONB), event_timestamp, correlation_id.
func scanRawRows(rows *sql.Rows) ([]RawAuditRow, error) {
	var results []RawAuditRow
	for rows.Next() {
		var row RawAuditRow
		var eventDataJSON []byte
		if err := rows.Scan(&row.EventType, &eventDataJSON, &row.EventTimestamp, &row.CorrelationID); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(eventDataJSON, &row.EventData); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// QueryEffectivenessEventsBatch queries EM component events for a batch of
// correlation IDs. Returns events grouped by correlation_id.
//
// DD-HAPI-016 v1.1 Step 2: Query Tier 1 — EM component events.
// Same query pattern as queryEffectivenessEvents in effectiveness_handler.go
// but batched across multiple correlation IDs.
func (r *RemediationHistoryRepository) QueryEffectivenessEventsBatch(
	ctx context.Context,
	correlationIDs []string,
) (map[string][]*EffectivenessEventRow, error) {
	// Include event_type column so BuildEffectivenessResponse can route events correctly.
	// The event_data JSONB may not contain event_type (E2E tests insert it only as a column),
	// so we merge the column value into EventData to ensure downstream consumers always see it.
	query := `SELECT correlation_id, event_type, event_data
		FROM audit_events
		WHERE correlation_id = ANY($1)
		AND event_category = 'effectiveness'
		ORDER BY event_timestamp ASC, event_id ASC`

	rows, err := r.db.QueryContext(ctx, query, pq.Array(correlationIDs))
	if err != nil {
		r.logger.Error(err, "Failed to query EM events batch",
			"correlation_id_count", len(correlationIDs))
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			r.logger.Error(cerr, "Failed to close EM events batch query rows")
		}
	}()

	results := make(map[string][]*EffectivenessEventRow)
	for rows.Next() {
		var correlationID string
		var eventType string
		var eventDataJSON []byte
		if err := rows.Scan(&correlationID, &eventType, &eventDataJSON); err != nil {
			return nil, err
		}
		var eventData map[string]interface{}
		if err := json.Unmarshal(eventDataJSON, &eventData); err != nil {
			r.logger.Error(err, "Failed to unmarshal EM event data", "correlation_id", correlationID)
			continue
		}
		// Merge event_type column into EventData for BuildEffectivenessResponse routing.
		// Column value takes precedence (authoritative source).
		eventData["event_type"] = eventType
		results[correlationID] = append(results[correlationID], &EffectivenessEventRow{
			EventData: eventData,
		})
	}

	return results, rows.Err()
}

// QueryROEventsBySpecHash queries remediation.workflow_created audit events
// matching a specific spec hash within a time window. The hash is matched
// against BOTH pre_remediation_spec_hash (direct) and post_remediation_spec_hash
// (via EM correlation_id subquery). An OR combines both paths in a single scan;
// no DISTINCT is needed because each event_id appears at most once regardless
// of which OR branch matched.
//
// The EM subquery is intentionally time-unbounded: effectiveness assessments
// may arrive after the RO event's tier boundary (e.g., RO in tier 2, EM in tier 1).
// Constraining the subquery to the same window causes false negatives at tier
// boundaries (F1 due diligence finding). The idx_audit_events_post_remediation_spec_hash
// partial index limits scan scope despite the lack of time constraint.
//
// Issue #616: Original query only matched pre_remediation_spec_hash, missing
// cases where the current resource state matches a previous remediation's
// post-remediation state (the normal successful-remediation cycle).
//
// Uses expression indexes:
//   - idx_audit_events_pre_remediation_spec_hash (existing)
//   - idx_audit_events_post_remediation_spec_hash (migration 004)
//
// DD-HAPI-016 v1.4: Both tiers query by spec hash (#586).
func (r *RemediationHistoryRepository) QueryROEventsBySpecHash(
	ctx context.Context,
	specHash string,
	since time.Time,
	until time.Time,
) ([]RawAuditRow, error) {
	query := `SELECT event_type, event_data, event_timestamp, correlation_id
		FROM audit_events
		WHERE event_type = 'remediation.workflow_created'
		AND event_timestamp >= $2
		AND event_timestamp < $3
		AND (
			event_data->>'pre_remediation_spec_hash' = $1
			OR correlation_id IN (
				SELECT correlation_id FROM audit_events
				WHERE event_category = 'effectiveness'
				AND event_data->>'post_remediation_spec_hash' = $1
			)
		)
		ORDER BY event_timestamp ASC, event_id ASC`

	rows, err := r.db.QueryContext(ctx, query, specHash, since, until)
	if err != nil {
		r.logger.Error(err, "Failed to query RO events by spec hash",
			"spec_hash", specHash, "since", since, "until", until)
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			r.logger.Error(cerr, "Failed to close RO events by spec hash query rows")
		}
	}()

	results, err := scanRawRows(rows)
	if err != nil {
		return nil, err
	}

	r.logger.V(1).Info("QueryROEventsBySpecHash completed",
		"spec_hash", specHash,
		"result_count", len(results),
		"window", until.Sub(since).String())

	return results, nil
}
