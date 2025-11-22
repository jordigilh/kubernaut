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
	"time"

	"go.uber.org/zap"
)

// AuditEventsQueryBuilder constructs SQL queries for the audit_events table
// DD-STORAGE-010: Query API with offset-based pagination
// BR-STORAGE-021: REST API Read Endpoints
// BR-STORAGE-022: Query Filtering
// BR-STORAGE-023: Pagination Validation
type AuditEventsQueryBuilder struct {
	correlationID *string
	eventType     *string
	eventCategory *string // ADR-034: renamed from 'service'
	eventOutcome  *string // ADR-034: renamed from 'outcome'
	severity      *string
	since         *time.Time
	until         *time.Time
	limit         int
	offset        int
	logger        *zap.Logger
}

// AuditEventsBuilderOption is a functional option for configuring AuditEventsQueryBuilder
type AuditEventsBuilderOption func(*AuditEventsQueryBuilder)

// WithAuditEventsLogger sets a custom logger for the audit events query builder
func WithAuditEventsLogger(logger *zap.Logger) AuditEventsBuilderOption {
	return func(b *AuditEventsQueryBuilder) {
		if logger != nil {
			b.logger = logger
		}
	}
}

// NewAuditEventsQueryBuilder creates a new query builder for audit_events table
func NewAuditEventsQueryBuilder(opts ...AuditEventsBuilderOption) *AuditEventsQueryBuilder {
	b := &AuditEventsQueryBuilder{
		limit:  100,          // Default limit
		offset: 0,            // Default offset
		logger: zap.NewNop(), // Noop logger by default
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
	// BR-STORAGE-023: Validate pagination parameters
	if b.limit < 1 || b.limit > 1000 {
		err := fmt.Errorf("pagination validation failed: limit must be between 1 and 1000, got %d (BR-STORAGE-023)", b.limit)
		b.logger.Warn("Query build failed",
			zap.Int("limit", b.limit),
			zap.String("error", "invalid_limit"),
		)
		return "", nil, err
	}
	if b.offset < 0 {
		err := fmt.Errorf("pagination validation failed: offset must be non-negative, got %d (BR-STORAGE-023)", b.offset)
		b.logger.Warn("Query build failed",
			zap.Int("offset", b.offset),
			zap.String("error", "invalid_offset"),
		)
		return "", nil, err
	}

	// Log query construction
	b.logger.Debug("Building audit_events SQL query",
		zap.Any("correlation_id", b.correlationID),
		zap.Any("event_type", b.eventType),
		zap.Any("event_category", b.eventCategory),
		zap.Any("event_outcome", b.eventOutcome),
		zap.Any("severity", b.severity),
		zap.Any("since", b.since),
		zap.Any("until", b.until),
		zap.Int("limit", b.limit),
		zap.Int("offset", b.offset),
	)

	// Base query (ADR-034 column names)
	sql := "SELECT event_id, event_type, event_category, correlation_id, event_timestamp, event_outcome, severity, " +
		"resource_type, resource_id, actor_type, actor_id, parent_event_id, event_data, event_date " +
		"FROM audit_events WHERE 1=1"

	// Count active filters
	args := make([]interface{}, 0, 8) // Preallocate for filters + limit + offset
	argIndex := 1

	// BR-STORAGE-022: Apply filters dynamically
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

	// BR-STORAGE-021: Add ORDER BY for consistent ordering (DESC for most recent first)
	sql += " ORDER BY event_timestamp DESC, event_id DESC"

	// BR-STORAGE-023: Add pagination
	sql += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, b.limit, b.offset)

	b.logger.Debug("SQL query built successfully",
		zap.Int("filter_count", len(args)-2), // Exclude limit and offset
		zap.Int("arg_count", len(args)),
		zap.Int("limit", b.limit),
		zap.Int("offset", b.offset),
	)

	return sql, args, nil
}

// BuildCount builds a COUNT(*) SQL query with filters (no pagination, ordering)
// Returns the total count of matching records for pagination metadata
func (b *AuditEventsQueryBuilder) BuildCount() (string, []interface{}, error) {
	// Validate filters (but not pagination)
	b.logger.Debug("Building audit_events COUNT query")

	// Base query
	sql := "SELECT COUNT(*) FROM audit_events WHERE 1=1"

	// Count active filters
	args := make([]interface{}, 0, 7) // Preallocate for filters only
	argIndex := 1

	// Apply same filters as Build() (but no ORDER BY, LIMIT, OFFSET)
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
		// argIndex++ // Last filter, no need to increment
	}

	b.logger.Debug("COUNT query built successfully",
		zap.Int("filter_count", len(args)),
	)

	return sql, args, nil
}
