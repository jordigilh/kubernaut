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
// BR-KA-016: Remediation history context for LLM prompt enrichment.
// DD-KA-016 v1.7: Recursive dual-hash traversal for complete causal history (#2490).
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/lib/pq"
)

var (
	// ErrRemediationHistoryResourceLimit indicates that a complete history could
	// not be returned within the repository's safety budget. Callers must not
	// treat this as an empty or partial history response.
	ErrRemediationHistoryResourceLimit = errors.New("remediation history resource limit exceeded")
)

const (
	// MaxROEventsBySpecHashResults caps the number of rows returned by
	// QueryROEventsBySpecHash to prevent unbounded result sets (PERF-H2).
	MaxROEventsBySpecHashResults = 10000
	// MaxRemediationHistoryTraversalDepth bounds recursive CTE expansion. A
	// query that reaches this depth fails closed rather than returning a prefix.
	MaxRemediationHistoryTraversalDepth = 256
	// remediationHistoryQueryTimeout bounds database work independently of the
	// connection-wide PostgreSQL statement_timeout.
	remediationHistoryQueryTimeout = 10 * time.Second
)

// queryROEventsBySpecHash traverses the linked RO chain from a dual-hash
// anchor. Recursive candidates must have a correlated EM post-hash equal to
// the current RO pre-hash and an earlier RO timestamp. Recursive UNION removes
// duplicate recursive states; the final DISTINCT ON removes duplicate event
// identities reached through convergent paths.
const queryROEventsBySpecHash = `WITH RECURSIVE remediation_chain (
			event_id, event_type, event_data, event_timestamp, correlation_id, pre_hash, chain_depth
		) AS (
			SELECT ro.event_id,
				ro.event_type,
				ro.event_data,
				ro.event_timestamp,
				ro.correlation_id,
				ro.event_data->>'pre_remediation_spec_hash',
				1
			FROM audit_events ro
			WHERE ro.event_type = 'remediation.workflow_created'
				AND ro.event_data->>'target_resource' = $1
				AND ($2 = '' OR ro.cluster_id = $2)
				AND ro.event_timestamp >= $4
				AND ro.event_timestamp < $5
				AND (
					ro.event_data->>'pre_remediation_spec_hash' = $3
					OR EXISTS (
						SELECT 1
						FROM audit_events em
						WHERE em.event_category = 'effectiveness'
							AND em.correlation_id = ro.correlation_id
							AND ($2 = '' OR em.cluster_id = $2)
							AND em.event_data->>'post_remediation_spec_hash' = $3
					)
				)

			UNION

			SELECT ro.event_id,
				ro.event_type,
				ro.event_data,
				ro.event_timestamp,
				ro.correlation_id,
				ro.event_data->>'pre_remediation_spec_hash',
				previous.chain_depth + 1
			FROM audit_events ro
			JOIN remediation_chain previous
				ON EXISTS (
					SELECT 1
					FROM audit_events em
					WHERE em.event_category = 'effectiveness'
						AND em.correlation_id = ro.correlation_id
						AND ($2 = '' OR em.cluster_id = $2)
						AND em.event_data->>'post_remediation_spec_hash' = previous.pre_hash
				)
			WHERE ro.event_type = 'remediation.workflow_created'
				AND ro.event_data->>'target_resource' = $1
				AND ($2 = '' OR ro.cluster_id = $2)
				AND ro.event_timestamp >= $4
				AND ro.event_timestamp < $5
				AND ro.event_timestamp < previous.event_timestamp
				AND previous.chain_depth < $7
		),
		deduplicated_chain AS (
			SELECT DISTINCT ON (event_id)
				event_id, event_type, event_data, event_timestamp, correlation_id, chain_depth
			FROM remediation_chain
			ORDER BY event_id, chain_depth DESC
		)
		SELECT event_type, event_data, event_timestamp, correlation_id, chain_depth
		FROM deduplicated_chain
		ORDER BY event_timestamp ASC, event_id ASC
		LIMIT $6`

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
// DD-KA-016 v1.7, Issue #616/#2490: Both tiers query RO events by spec hash, matching
// BOTH pre_remediation_spec_hash (direct) and post_remediation_spec_hash (via EM correlation).
// The spec-hash query recursively follows each matched remediation's
// pre_remediation_spec_hash so callers receive the complete causal chain, not
// only the latest event whose post hash matches the current resource state.
//  1. Query the linked RO chain by spec hash (Tier 1: 24h window, Tier 2: 90d window)
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
// Each row must have columns: event_type, event_data (JSONB), event_timestamp,
// correlation_id, chain_depth.
func scanRawRows(rows *sql.Rows) ([]RawAuditRow, error) {
	var results []RawAuditRow
	for rows.Next() {
		var row RawAuditRow
		var eventDataJSON []byte
		var chainDepth int
		if err := rows.Scan(&row.EventType, &eventDataJSON, &row.EventTimestamp, &row.CorrelationID, &chainDepth); err != nil {
			return nil, err
		}
		if chainDepth >= MaxRemediationHistoryTraversalDepth {
			return nil, fmt.Errorf("%w: traversal depth reached %d", ErrRemediationHistoryResourceLimit, MaxRemediationHistoryTraversalDepth)
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
// DD-KA-016 v1.1 Step 2: Query Tier 1 — EM component events.
// Same query pattern as queryEffectivenessEvents in effectiveness_handler.go
// but batched across multiple correlation IDs.
//
// PERF-H1 (per-correlation skew risk): A global LIMIT on a batch query can
// starve later correlations when early ones have disproportionately many events.
// The limit is scaled as 100 * len(correlationIDs) with a 50,000 hard cap.
// If a production deployment observes truncated results (logged at V(1)),
// consider increasing the per-correlation factor or switching to a
// per-correlation subquery with LATERAL JOIN.
func (r *RemediationHistoryRepository) QueryEffectivenessEventsBatch(
	ctx context.Context,
	correlationIDs []string,
) (map[string][]*EffectivenessEventRow, error) {
	// Include event_type column so BuildEffectivenessResponse can route events correctly.
	// The event_data JSONB may not contain event_type (E2E tests insert it only as a column),
	// so we merge the column value into EventData to ensure downstream consumers always see it.
	// PERF-H1: LIMIT scaled per correlation to prevent global skew
	// where early correlations consume all rows and later ones get none.
	// Each correlation typically has ~10 EM events; 100x is generous headroom.
	maxEMBatchResults := 100 * len(correlationIDs)
	if maxEMBatchResults > 50000 {
		maxEMBatchResults = 50000
	}
	query := `SELECT correlation_id, event_type, event_data
		FROM audit_events
		WHERE correlation_id = ANY($1)
		AND event_category = 'effectiveness'
		ORDER BY event_timestamp ASC, event_id ASC
		LIMIT $2`

	rows, err := r.db.QueryContext(ctx, query, pq.Array(correlationIDs), maxEMBatchResults)
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
			return nil, fmt.Errorf("corrupt JSONB in remediation history (correlation=%s, type=%s): %w", correlationID, eventType, err)
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

// QueryROEventsBySpecHash queries the complete remediation.workflow_created
// causal chain for a target resource and current spec hash within a time
// window. The anchor hash is matched against BOTH pre_remediation_spec_hash
// (direct) and post_remediation_spec_hash (via EM correlation). Each matched
// remediation then contributes its pre-remediation hash as the next link to
// follow. This exposes every prior remediation connected by the chained
// pre/post hashes, rather than only the latest current-hash match.
//
// The EM subquery is intentionally time-unbounded: effectiveness assessments
// may arrive after the RO event's tier boundary (e.g., RO in tier 2, EM in tier 1).
// Constraining the subquery to the same window causes false negatives at tier
// boundaries (F1 due diligence finding). The idx_audit_events_post_remediation_spec_hash
// partial index limits scan scope despite the lack of time constraint. The EM
// subquery is scoped by cluster when the request is fleet-scoped. EM events do
// not carry the RO target-resource string, so correlation_id remains the
// attempt-level join key while cluster_id prevents same-ID cross-cluster edges.
//
// Issue #616: Original query only matched pre_remediation_spec_hash, missing
// cases where the current resource state matches a previous remediation's
// post-remediation state (the normal successful-remediation cycle).
// The recursive query also preserves that post-hash behavior while walking
// backward through earlier forward changes.
//
// Issue #1802: Matching purely on spec_hash (with no target scoping) let two
// unrelated resources sharing an identical Pod spec (e.g., templated
// Deployments from the same Helm chart) collide and be treated as the same
// remediation chain by RO's ineffective-chain blocking (BR-ORCH-042.5). Both
// targetResource and clusterID now scope the match. clusterID is optional
// (empty string means unscoped, preserving release/v1.5 semantics which has
// no cluster_id concept).
//
// Uses expression indexes:
//   - idx_audit_events_target_resource (existing, migration 001)
//   - idx_audit_events_pre_remediation_spec_hash (existing)
//   - idx_audit_events_post_remediation_spec_hash (migration 004)
//   - idx_audit_events_cluster_id (migration 017, main only)
//
// DD-KA-016 v1.7: Both tiers recursively traverse causal post-hash links,
// scope every hop by target_resource (+ cluster_id on main), and bound the
// result by the requested time window and result cap (#2490).
//
// PERF-H2 Monitoring: Run EXPLAIN ANALYZE periodically in production to verify
// idx_audit_events_pre_remediation_spec_hash and idx_audit_events_post_remediation_spec_hash
// are used. If the planner falls back to a sequential scan, consider adding a composite
// index on (event_timestamp, pre_remediation_spec_hash) to cover the ORDER BY.
func (r *RemediationHistoryRepository) QueryROEventsBySpecHash(
	ctx context.Context,
	targetResource string,
	clusterID string,
	specHash string,
	since time.Time,
	until time.Time,
) ([]RawAuditRow, error) {
	// PERF-H2: the query requests one extra row so a complete-chain overflow
	// can be detected and rejected instead of silently returning a prefix.
	const maxROResults = MaxROEventsBySpecHashResults
	// EM hash lookups intentionally remain time-unbounded: an assessment may be
	// recorded after the RO event's tier boundary (F1 due-diligence finding).

	queryCtx, cancel := context.WithTimeout(ctx, remediationHistoryQueryTimeout)
	defer cancel()

	rows, err := r.db.QueryContext(queryCtx, queryROEventsBySpecHash, targetResource, clusterID, specHash, since, until, maxROResults+1, MaxRemediationHistoryTraversalDepth)
	if err != nil {
		err = wrapRemediationHistoryQueryTimeout(ctx, queryCtx, err)
		r.logger.Error(err, "Failed to query RO events by spec hash",
			"target_resource", targetResource, "cluster_id", clusterID,
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
		err = wrapRemediationHistoryQueryTimeout(ctx, queryCtx, err)
		r.logger.Error(err, "Failed to scan RO events by spec hash",
			"target_resource", targetResource, "cluster_id", clusterID,
			"spec_hash", specHash, "since", since, "until", until)
		return nil, err
	}
	if err := queryCtx.Err(); err != nil {
		err = wrapRemediationHistoryQueryTimeout(ctx, queryCtx, err)
		r.logger.Error(err, "RO events by spec hash query exceeded its context deadline",
			"target_resource", targetResource, "cluster_id", clusterID,
			"spec_hash", specHash, "since", since, "until", until)
		return nil, err
	}
	if len(results) > maxROResults {
		err := fmt.Errorf("%w: result count exceeded %d", ErrRemediationHistoryResourceLimit, maxROResults)
		r.logger.Error(err, "RO events by spec hash result budget exceeded",
			"target_resource", targetResource, "cluster_id", clusterID,
			"spec_hash", specHash, "result_count", len(results),
			"max_results", maxROResults)
		return nil, err
	}

	r.logger.V(1).Info("QueryROEventsBySpecHash completed",
		"target_resource", targetResource,
		"cluster_id", clusterID,
		"spec_hash", specHash,
		"result_count", len(results),
		"window", until.Sub(since).String())

	return results, nil
}

// wrapRemediationHistoryQueryTimeout distinguishes the repository's own
// traversal deadline from cancellation/deadlines inherited from the request.
// Only the repository deadline is classified as a resource-limit failure;
// callers that cancel their request should receive the original context error.
func wrapRemediationHistoryQueryTimeout(parentCtx, queryCtx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) &&
		errors.Is(queryCtx.Err(), context.DeadlineExceeded) &&
		parentCtx.Err() == nil {
		return fmt.Errorf("%w: query exceeded %s: %w", ErrRemediationHistoryResourceLimit, remediationHistoryQueryTimeout, err)
	}
	return err
}
