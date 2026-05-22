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

package query

import (
	"fmt"
	"regexp"
	"time"

	"github.com/go-logr/logr"
)

var validJSONBKey = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// AuditEventsQueryBuilder constructs SQL queries for the audit_events table
// DD-STORAGE-010: Query API with offset-based pagination
// BR-STORAGE-021: REST API Read Endpoints
// BR-STORAGE-022: Query Filtering
// BR-STORAGE-023: Pagination Validation
type AuditEventsQueryBuilder struct {
	correlationID  *string
	eventType      *string
	eventCategory  *string // ADR-034: renamed from 'service'
	eventOutcome   *string // ADR-034: renamed from 'outcome'
	severity       *string
	eventDataKey   *string // Issue #1199: JSONB field key for ->> filter
	eventDataValue *string // Issue #1199: JSONB field value for ->> filter
	since          *time.Time
	until          *time.Time
	limit          int
	offset         int
	logger         logr.Logger
}

// AuditEventsBuilderOption is a functional option for configuring AuditEventsQueryBuilder
type AuditEventsBuilderOption func(*AuditEventsQueryBuilder)

// WithAuditEventsLogger sets a custom logger for the audit events query builder
func WithAuditEventsLogger(logger logr.Logger) AuditEventsBuilderOption {
	return func(b *AuditEventsQueryBuilder) {
		b.logger = logger
	}
}

// NewAuditEventsQueryBuilder creates a new query builder for audit_events table
func NewAuditEventsQueryBuilder(opts ...AuditEventsBuilderOption) *AuditEventsQueryBuilder {
	b := &AuditEventsQueryBuilder{
		limit:  100,            // Default limit
		offset: 0,              // Default offset
		logger: logr.Discard(), // Discard logger by default
	}

	// Apply options
	for _, opt := range opts {
		opt(b)
	}

	return b
}

// WithCorrelationID sets correlation_id filter
func (b *AuditEventsQueryBuilder) WithCorrelationID(correlationID string) *AuditEventsQueryBuilder {
	if correlationID != "" {
		b.correlationID = &correlationID
	}
	return b
}

// WithEventType sets event_type filter
func (b *AuditEventsQueryBuilder) WithEventType(eventType string) *AuditEventsQueryBuilder {
	if eventType != "" {
		b.eventType = &eventType
	}
	return b
}

// WithService sets event_category filter (ADR-034: renamed from 'service')
// Kept method name for backward compatibility with existing code
func (b *AuditEventsQueryBuilder) WithService(eventCategory string) *AuditEventsQueryBuilder {
	if eventCategory != "" {
		b.eventCategory = &eventCategory
	}
	return b
}

// WithOutcome sets event_outcome filter (ADR-034: renamed from 'outcome')
// Kept method name for backward compatibility with existing code
func (b *AuditEventsQueryBuilder) WithOutcome(eventOutcome string) *AuditEventsQueryBuilder {
	if eventOutcome != "" {
		b.eventOutcome = &eventOutcome
	}
	return b
}

// WithSeverity sets severity filter
func (b *AuditEventsQueryBuilder) WithSeverity(severity string) *AuditEventsQueryBuilder {
	if severity != "" {
		b.severity = &severity
	}
	return b
}

// WithSince sets start time filter
func (b *AuditEventsQueryBuilder) WithSince(since time.Time) *AuditEventsQueryBuilder {
	if !since.IsZero() {
		b.since = &since
	}
	return b
}

// WithUntil sets end time filter
func (b *AuditEventsQueryBuilder) WithUntil(until time.Time) *AuditEventsQueryBuilder {
	if !until.IsZero() {
		b.until = &until
	}
	return b
}

// WithEventDataFilter sets a JSONB field filter on event_data using the ->> operator.
// Issue #1199: Enables bidirectional correlation queries (task_id <-> rr_name).
// The key must match [a-zA-Z0-9_]+ to prevent SQL injection.
// An empty value is treated as a no-op (filter not applied).
func (b *AuditEventsQueryBuilder) WithEventDataFilter(key, value string) *AuditEventsQueryBuilder {
	if value != "" {
		b.eventDataKey = &key
		b.eventDataValue = &value
	}
	return b
}

// WithLimit sets pagination limit
// BR-STORAGE-023: Limit must be 1-1000
func (b *AuditEventsQueryBuilder) WithLimit(limit int) *AuditEventsQueryBuilder {
	b.limit = limit
	return b
}

// WithOffset sets pagination offset
// BR-STORAGE-023: Offset must be >= 0
func (b *AuditEventsQueryBuilder) WithOffset(offset int) *AuditEventsQueryBuilder {
	b.offset = offset
	return b
}

// Build constructs the final SQL query with parameterized values
// Returns: (sql string, args []interface{}, error)
// BR-STORAGE-025: Uses parameterized queries to prevent SQL injection
func (b *AuditEventsQueryBuilder) Build() (string, []interface{}, error) {
	if b.limit < 1 || b.limit > 1000 {
		err := fmt.Errorf("pagination validation failed: limit must be between 1 and 1000, got %d (BR-STORAGE-023)", b.limit)
		b.logger.Info("Query build failed", "limit", b.limit, "error", "invalid_limit")
		return "", nil, err
	}
	if b.offset < 0 {
		err := fmt.Errorf("pagination validation failed: offset must be non-negative, got %d (BR-STORAGE-023)", b.offset)
		b.logger.Info("Query build failed", "offset", b.offset, "error", "invalid_offset")
		return "", nil, err
	}
	if err := b.validateEventDataFilter(); err != nil {
		return "", nil, err
	}

	b.logger.V(1).Info("Building audit_events SQL query",
		"correlation_id", b.correlationID,
		"event_type", b.eventType,
		"event_category", b.eventCategory,
		"event_outcome", b.eventOutcome,
		"severity", b.severity,
		"since", b.since,
		"until", b.until,
		"limit", b.limit,
		"offset", b.offset,
	)

	// DD-TESTING-001: Include ALL optional fields for comprehensive audit validation
	sql := "SELECT event_id, event_version, event_type, event_category, event_action, correlation_id, event_timestamp, event_outcome, severity, " +
		"resource_type, resource_id, actor_type, actor_id, actor_ip, parent_event_id, event_data, event_date, namespace, cluster_name, " +
		"duration_ms, error_code, error_message, event_hash, previous_event_hash, retention_days, is_sensitive, parent_event_date, " +
		"legal_hold, legal_hold_reason, legal_hold_placed_by, legal_hold_placed_at " +
		"FROM audit_events WHERE 1=1"

	args := make([]interface{}, 0, 10)
	sql, args = b.appendFilters(sql, args)
	argIndex := len(args) + 1

	sql += " ORDER BY event_timestamp DESC, event_id DESC"
	sql += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, b.limit, b.offset)

	b.logger.V(1).Info("SQL query built successfully",
		"filter_count", len(args)-2,
		"arg_count", len(args),
		"limit", b.limit,
		"offset", b.offset,
	)

	return sql, args, nil
}

// BuildCount builds a COUNT(*) SQL query with filters (no pagination, ordering)
// Returns the total count of matching records for pagination metadata
func (b *AuditEventsQueryBuilder) BuildCount() (string, []interface{}, error) {
	b.logger.V(1).Info("Building audit_events COUNT query")

	if err := b.validateEventDataFilter(); err != nil {
		return "", nil, err
	}

	sql := "SELECT COUNT(*) FROM audit_events WHERE 1=1"
	args := make([]interface{}, 0, 8)
	sql, args = b.appendFilters(sql, args)

	b.logger.V(1).Info("COUNT query built successfully", "filter_count", len(args))

	return sql, args, nil
}

// appendFilters appends WHERE-clause conditions for all configured filters.
// Shared by Build() and BuildCount() to guarantee filter parity.
func (b *AuditEventsQueryBuilder) appendFilters(sql string, args []interface{}) (string, []interface{}) {
	argIndex := len(args) + 1

	if b.correlationID != nil {
		sql += fmt.Sprintf(" AND correlation_id = $%d", argIndex)
		args = append(args, *b.correlationID)
		argIndex++
	}
	if b.eventType != nil {
		sql += fmt.Sprintf(" AND event_type = $%d", argIndex)
		args = append(args, *b.eventType)
		argIndex++
	}
	if b.eventCategory != nil {
		sql += fmt.Sprintf(" AND event_category = $%d", argIndex)
		args = append(args, *b.eventCategory)
		argIndex++
	}
	if b.eventOutcome != nil {
		sql += fmt.Sprintf(" AND event_outcome = $%d", argIndex)
		args = append(args, *b.eventOutcome)
		argIndex++
	}
	if b.severity != nil {
		sql += fmt.Sprintf(" AND severity = $%d", argIndex)
		args = append(args, *b.severity)
		argIndex++
	}
	if b.since != nil {
		sql += fmt.Sprintf(" AND event_timestamp >= $%d", argIndex)
		args = append(args, *b.since)
		argIndex++
	}
	if b.until != nil {
		sql += fmt.Sprintf(" AND event_timestamp <= $%d", argIndex)
		args = append(args, *b.until)
		argIndex++
	}
	if b.eventDataKey != nil && b.eventDataValue != nil {
		sql += fmt.Sprintf(" AND event_data->>'%s' = $%d", *b.eventDataKey, argIndex)
		args = append(args, *b.eventDataValue)
	}

	return sql, args
}

// validateEventDataFilter checks that the JSONB key is safe for SQL embedding.
// Returns an error if the key is set but does not match [a-zA-Z0-9_]+.
func (b *AuditEventsQueryBuilder) validateEventDataFilter() error {
	if b.eventDataKey == nil {
		return nil
	}
	if !validJSONBKey.MatchString(*b.eventDataKey) {
		return fmt.Errorf("invalid event_data filter key: must match [a-zA-Z0-9_]+, got %q", *b.eventDataKey)
	}
	return nil
}
