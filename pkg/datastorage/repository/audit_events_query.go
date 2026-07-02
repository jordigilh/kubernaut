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

	"github.com/google/uuid"
)

// Query, row-scanning, and health-check paths for the unified audit_events
// table. Split from audit_events_repository.go (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3, pure code motion, no behavior change).

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
		event, err := scanQueryRow(rows)
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating audit events: %w", err)
	}

	pagination := buildQueryPagination(args, total, len(events))

	r.logger.V(1).Info("Audit events queried",
		"count", len(events),
		"total", total,
		"limit", pagination.Limit,
		"offset", pagination.Offset,
	)

	return events, pagination, nil
}

// scanQueryRow scans one row from Query's result set into an *AuditEvent,
// converting sql.Null* intermediates to the corresponding AuditEvent fields
// and unmarshaling the event_data JSONB payload.
func scanQueryRow(rows *sql.Rows) (*AuditEvent, error) {
	event := &AuditEvent{}
	var eventDataJSON []byte
	var cols queryNullableColumns

	err := rows.Scan(
		&event.EventID,
		&event.Version, // event_version from DB (maps to version in OpenAPI)
		&event.EventType,
		&event.EventCategory, // ADR-034
		&event.EventAction,   // ADR-034
		&event.CorrelationID,
		&event.EventTimestamp,
		&event.EventOutcome, // ADR-034
		&cols.severity,
		&cols.resourceType,
		&cols.resourceID,
		&cols.actorType,
		&cols.actorID,
		&cols.actorIP,
		&cols.parentEventID,
		&eventDataJSON,
		&event.EventDate,
		&cols.namespace,
		&cols.clusterName,
		&cols.durationMs,   // DD-TESTING-001: Added for top-level field validation
		&cols.errorCode,    // DD-TESTING-001: Added for error validation
		&cols.errorMessage, // DD-TESTING-001: Added for error validation
		&cols.eventHash,
		&cols.previousEventHash,
		&cols.retentionDays,
		&cols.isSensitive,
		&cols.parentEventDate,
		&cols.legalHold,
		&cols.legalHoldReason,
		&cols.legalHoldPlacedBy,
		&cols.legalHoldPlacedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan audit event: %w", err)
	}

	applyQueryNullableColumns(event, cols)

	// Unmarshal event_data JSONB
	if len(eventDataJSON) > 0 {
		if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event_data: %w", err)
		}
	}

	return event, nil
}

// queryNullableColumns groups the sql.Null* scan intermediates for Query's
// nullable columns so scanQueryRow and applyQueryNullableColumns can pass
// them as a single argument instead of a long parameter list.
type queryNullableColumns struct {
	parentEventID                      sql.NullString
	actorID, actorType, actorIP        sql.NullString
	resourceType, resourceID           sql.NullString
	severity, namespace, clusterName   sql.NullString
	errorCode, errorMessage            sql.NullString // DD-TESTING-001: Error fields
	durationMs                         sql.NullInt64  // DD-TESTING-001: Performance tracking (BR-SP-090)
	eventHash, previousEventHash       sql.NullString
	retentionDays                      sql.NullInt64
	isSensitive                        sql.NullBool
	parentEventDate                    sql.NullTime
	legalHold                          sql.NullBool
	legalHoldReason, legalHoldPlacedBy sql.NullString
	legalHoldPlacedAt                  sql.NullTime
}

// applyQueryNullableColumns copies validated sql.Null* intermediates onto
// event's plain-typed fields, leaving zero values where the source column
// was NULL. Split into two field-group helpers (identity/resource fields vs.
// audit-metadata/legal-hold fields) to keep each below the project's
// cyclomatic-complexity convention; the set of fields copied is unchanged.
func applyQueryNullableColumns(event *AuditEvent, cols queryNullableColumns) {
	applyQueryIdentityColumns(event, cols)
	applyQueryAuditMetadataColumns(event, cols)
}

// applyQueryIdentityColumns copies the resource/actor/namespace identity
// columns (severity, resource, actor, parent_event_id, namespace, cluster).
func applyQueryIdentityColumns(event *AuditEvent, cols queryNullableColumns) {
	if cols.severity.Valid {
		event.Severity = cols.severity.String
	}
	if cols.resourceType.Valid {
		event.ResourceType = cols.resourceType.String
	}
	if cols.resourceID.Valid {
		event.ResourceID = cols.resourceID.String
	}
	if cols.actorType.Valid {
		event.ActorType = cols.actorType.String
	}
	if cols.actorID.Valid {
		event.ActorID = cols.actorID.String
	}
	if cols.actorIP.Valid {
		event.ActorIP = cols.actorIP.String
	}
	if cols.parentEventID.Valid {
		if parentUUID, err := uuid.Parse(cols.parentEventID.String); err == nil {
			event.ParentEventID = &parentUUID
		}
	}
	if cols.namespace.Valid {
		event.ResourceNamespace = cols.namespace.String
	}
	if cols.clusterName.Valid {
		event.ClusterID = cols.clusterName.String
	}
}

// applyQueryAuditMetadataColumns copies the audit-metadata columns
// (duration, error details, hash chain, retention, legal hold).
func applyQueryAuditMetadataColumns(event *AuditEvent, cols queryNullableColumns) {
	// DD-TESTING-001: Handle optional fields for comprehensive audit validation
	if cols.durationMs.Valid {
		event.DurationMs = int(cols.durationMs.Int64) // BR-SP-090: Performance tracking
	}
	if cols.errorCode.Valid {
		event.ErrorCode = cols.errorCode.String
	}
	if cols.errorMessage.Valid {
		event.ErrorMessage = cols.errorMessage.String
	}
	if cols.eventHash.Valid {
		event.EventHash = cols.eventHash.String
	}
	if cols.previousEventHash.Valid {
		event.PreviousEventHash = cols.previousEventHash.String
	}
	if cols.retentionDays.Valid {
		event.RetentionDays = int(cols.retentionDays.Int64)
	}
	if cols.isSensitive.Valid {
		event.IsSensitive = cols.isSensitive.Bool
	}
	if cols.parentEventDate.Valid {
		ts := cols.parentEventDate.Time
		event.ParentEventDate = &ts
	}
	if cols.legalHold.Valid {
		event.LegalHold = cols.legalHold.Bool
	}
	if cols.legalHoldReason.Valid {
		event.LegalHoldReason = cols.legalHoldReason.String
	}
	if cols.legalHoldPlacedBy.Valid {
		event.LegalHoldPlacedBy = cols.legalHoldPlacedBy.String
	}
	if cols.legalHoldPlacedAt.Valid {
		ts := cols.legalHoldPlacedAt.Time
		event.LegalHoldPlacedAt = &ts
	}
}

// buildQueryPagination derives PaginationMetadata from the query's trailing
// (limit, offset) args and the total row count. Mirrors Query's original
// inline bounds-checked extraction (Issue #667 array-slice-panic fix):
// args may have 0, 1 (limit only), or 2+ (limit, offset) trailing elements.
func buildQueryPagination(args []interface{}, total int, eventCount int) *PaginationMetadata {
	limit := 0
	offset := 0
	if len(args) >= 2 {
		limit = int(args[len(args)-2].(int))
		offset = int(args[len(args)-1].(int))
	} else if len(args) == 1 {
		limit = int(args[0].(int))
	}
	return &PaginationMetadata{
		Limit:   limit,
		Offset:  offset,
		Total:   total,
		HasMore: offset+eventCount < total,
	}
}

// HealthCheck verifies database connectivity
func (r *AuditEventsRepository) HealthCheck(ctx context.Context) error {
	if err := r.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}
	return nil
}
