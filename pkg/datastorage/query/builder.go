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
	"strings"

	"github.com/go-logr/logr"
)

// Builder constructs SQL queries with parameterized filters
// BR-STORAGE-021: Query construction for read endpoints
// BR-STORAGE-022: Query filtering support
// BR-STORAGE-023: Pagination support
// BR-STORAGE-025: SQL injection prevention via parameterized queries
//
// REFACTOR: Enhanced with structured logging and performance optimizations
type Builder struct {
	namespace   string
	signalName  string
	severity    string
	cluster     string
	environment string
	actionType  string
	limit       int
	offset      int
	logger      logr.Logger
}

// QueryParams represents query filter parameters
type QueryParams struct {
	Namespace   string
	SignalName  string
	Severity    string
	Cluster     string
	Environment string
	ActionType  string
}

// NewBuilder creates a new SQL query builder
// REFACTOR: Supports optional logger for production observability
func NewBuilder(opts ...BuilderOption) *Builder {
	b := &Builder{
		limit:  100,            // Default limit
		offset: 0,              // Default offset
		logger: logr.Discard(), // Discard logger by default (tests don't need logs)
	}

	// Apply options
	for _, opt := range opts {
		opt(b)
	}

	return b
}

// BuilderOption is a functional option for configuring the Builder
type BuilderOption func(*Builder)

// WithLogger sets a custom logger for the query builder
// REFACTOR: Production deployments should provide a real logger
func WithLogger(logger logr.Logger) BuilderOption {
	return func(b *Builder) {
		b.logger = logger
	}
}

// WithParams sets query parameters from QueryParams struct
func (b *Builder) WithParams(params QueryParams) *Builder {
	b.namespace = params.Namespace
	b.signalName = params.SignalName
	b.severity = params.Severity
	b.cluster = params.Cluster
	b.environment = params.Environment
	b.actionType = params.ActionType
	return b
}

// WithNamespace sets namespace filter
// BR-STORAGE-026: Unicode support - accepts any valid string including unicode
func (b *Builder) WithNamespace(namespace string) *Builder {
	b.namespace = namespace
	return b
}

// WithSignalName sets signal_name filter
func (b *Builder) WithSignalName(signalName string) *Builder {
	b.signalName = signalName
	return b
}

// WithSeverity sets severity filter
func (b *Builder) WithSeverity(severity string) *Builder {
	b.severity = severity
	return b
}

// WithCluster sets cluster filter
func (b *Builder) WithCluster(cluster string) *Builder {
	b.cluster = cluster
	return b
}

// WithEnvironment sets environment filter
func (b *Builder) WithEnvironment(environment string) *Builder {
	b.environment = environment
	return b
}

// WithActionType sets action type filter
func (b *Builder) WithActionType(actionType string) *Builder {
	b.actionType = actionType
	return b
}

// WithLimit sets pagination limit
// BR-STORAGE-023: Limit must be 1-1000
func (b *Builder) WithLimit(limit int) *Builder {
	b.limit = limit
	return b
}

// WithOffset sets pagination offset
// BR-STORAGE-023: Offset must be >= 0
func (b *Builder) WithOffset(offset int) *Builder {
	b.offset = offset
	return b
}

// Build constructs the final SQL query with parameterized values
// Returns: (sql string, args []interface{}, error)
// BR-STORAGE-025: Uses parameterized queries to prevent SQL injection
//
// REFACTOR: Enhanced with structured logging and detailed error messages
func (b *Builder) Build() (string, []interface{}, error) {
	// BR-STORAGE-023: Validate pagination parameters with detailed error messages
	if b.limit < 1 || b.limit > 1000 {
		err := fmt.Errorf("pagination validation failed: limit must be between 1 and 1000, got %d (BR-STORAGE-023)", b.limit)
		b.logger.Info("Query build failed",
			"limit", b.limit,
			"error", "invalid_limit",
		)
		return "", nil, err
	}
	if b.offset < 0 {
		err := fmt.Errorf("pagination validation failed: offset must be non-negative, got %d (BR-STORAGE-023)", b.offset)
		b.logger.Info("Query build failed",
			"offset", b.offset,
			"error", "invalid_offset",
		)
		return "", nil, err
	}

	// REFACTOR: Log query construction for observability
	b.logger.V(1).Info("Building SQL query",
		"namespace", b.namespace,
		"signal_name", b.signalName,
		"severity", b.severity,
		"cluster", b.cluster,
		"environment", b.environment,
		"action_type", b.actionType,
		"limit", b.limit,
		"offset", b.offset,
	)

	// Base query
	sql := "SELECT * FROM resource_action_traces WHERE 1=1"

	// REFACTOR: Performance optimization - preallocate args slice
	// Count active filters to size the slice properly
	filterCount := 0
	if b.namespace != "" {
		filterCount++
	}
	if b.signalName != "" {
		filterCount++
	}
	if b.severity != "" {
		filterCount++
	}
	if b.cluster != "" {
		filterCount++
	}
	if b.environment != "" {
		filterCount++
	}
	if b.actionType != "" {
		filterCount++
	}

	// Preallocate: filters + limit + offset
	args := make([]interface{}, 0, filterCount+2)
	argIndex := 1

	// BR-STORAGE-022: Apply filters dynamically
	if b.namespace != "" {
		sql += fmt.Sprintf(" AND namespace = $%d", argIndex)
		args = append(args, b.namespace)
		argIndex++
	}
	if b.signalName != "" {
		sql += fmt.Sprintf(" AND signal_name = $%d", argIndex)
		args = append(args, b.signalName)
		argIndex++
	}
	if b.severity != "" {
		sql += fmt.Sprintf(" AND signal_severity = $%d", argIndex)
		args = append(args, b.severity)
		argIndex++
	}
	if b.cluster != "" {
		sql += fmt.Sprintf(" AND cluster_name = $%d", argIndex)
		args = append(args, b.cluster)
		argIndex++
	}
	if b.environment != "" {
		sql += fmt.Sprintf(" AND environment = $%d", argIndex)
		args = append(args, b.environment)
		argIndex++
	}
	if b.actionType != "" {
		sql += fmt.Sprintf(" AND action_type = $%d", argIndex)
		args = append(args, b.actionType)
		argIndex++
	}

	// BR-STORAGE-021: Add ORDER BY for consistent ordering
	// #213: id DESC tiebreaker ensures deterministic pagination when timestamps collide
	sql += " ORDER BY action_timestamp DESC, id DESC"

	// BR-STORAGE-023: Add pagination
	sql += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, b.limit, b.offset)

	// Convert PostgreSQL placeholders ($1, $2) to standard placeholders (?)
	// This makes tests portable across different SQL drivers
	standardSQL := convertToStandardPlaceholders(sql)

	// REFACTOR: Log successful query construction
	b.logger.V(1).Info("SQL query built successfully",
		"filter_count", filterCount,
		"arg_count", len(args),
		"limit", b.limit,
		"offset", b.offset,
	)

	return standardSQL, args, nil
}

// BuildCount builds a COUNT(*) SQL query with filters (no pagination, ordering)
// 🚨 FIX: Separate COUNT query for accurate pagination metadata
// This fixes the critical bug where pagination.total was set to len(array) instead of database count
// See: docs/services/stateless/data-storage/implementation/DATA-STORAGE-INTEGRATION-TEST-TRIAGE.md
//
// Returns:
//   - SQL query string with COUNT(*) instead of SELECT *
//   - Query arguments (for parameterized queries)
//   - Error if validation fails
func (b *Builder) BuildCount() (string, []interface{}, error) {
	// REFACTOR: Log count query construction for observability
	b.logger.V(1).Info("Building COUNT(*) query",
		"namespace", b.namespace,
		"signal_name", b.signalName,
		"severity", b.severity,
		"cluster", b.cluster,
		"environment", b.environment,
		"action_type", b.actionType,
	)

	// Base COUNT query (no SELECT *, no ORDER BY, no LIMIT/OFFSET)
	sql := "SELECT COUNT(*) FROM resource_action_traces WHERE 1=1"

	// REFACTOR: Performance optimization - preallocate args slice
	// Count active filters to size the slice properly
	filterCount := 0
	if b.namespace != "" {
		filterCount++
	}
	if b.signalName != "" {
		filterCount++
	}
	if b.severity != "" {
		filterCount++
	}
	if b.cluster != "" {
		filterCount++
	}
	if b.environment != "" {
		filterCount++
	}
	if b.actionType != "" {
		filterCount++
	}

	// Preallocate args slice (no limit/offset for COUNT)
	args := make([]interface{}, 0, filterCount)
	argIndex := 1

	// BR-STORAGE-022: Apply filters dynamically (same as Build method)
	if b.namespace != "" {
		sql += fmt.Sprintf(" AND namespace = $%d", argIndex)
		args = append(args, b.namespace)
		argIndex++
	}
	if b.signalName != "" {
		sql += fmt.Sprintf(" AND signal_name = $%d", argIndex)
		args = append(args, b.signalName)
		argIndex++
	}
	if b.severity != "" {
		sql += fmt.Sprintf(" AND signal_severity = $%d", argIndex)
		args = append(args, b.severity)
		argIndex++
	}
	if b.cluster != "" {
		sql += fmt.Sprintf(" AND cluster_name = $%d", argIndex)
		args = append(args, b.cluster)
		argIndex++
	}
	if b.environment != "" {
		sql += fmt.Sprintf(" AND environment = $%d", argIndex)
		args = append(args, b.environment)
		argIndex++
	}
	if b.actionType != "" {
		sql += fmt.Sprintf(" AND action_type = $%d", argIndex)
		args = append(args, b.actionType)
		// argIndex++ // Not used after this point
	}

	// Convert PostgreSQL placeholders ($1, $2) to standard placeholders (?)
	// This makes tests portable across different SQL drivers
	standardSQL := convertToStandardPlaceholders(sql)

	// REFACTOR: Log successful count query construction
	b.logger.V(1).Info("COUNT(*) query built successfully",
		"filter_count", filterCount,
		"arg_count", len(args),
	)

	return standardSQL, args, nil
}

// convertToStandardPlaceholders converts PostgreSQL-style $1, $2 to ? placeholders
// This is for test compatibility - production code uses PostgreSQL directly
func convertToStandardPlaceholders(sql string) string {
	result := sql
	// Simple replacement for testing - production code uses PostgreSQL driver directly
	for i := 10; i >= 1; i-- {
		result = strings.ReplaceAll(result, fmt.Sprintf("$%d", i), "?")
	}
	return result
}
