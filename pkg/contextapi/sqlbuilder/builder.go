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

package sqlbuilder

import (
	"fmt"
	"strings"
	"time"
)

// Builder constructs parameterized SQL queries using Data Storage Service schema
// BR-CONTEXT-001: Query historical incident context
// BR-CONTEXT-002: Filter by namespace, severity, time range
// BR-CONTEXT-004: Filter by cluster, environment, action type
// BR-CONTEXT-007: Pagination support
//
// Schema Authority: Data Storage Service (DD-SCHEMA-001)
// Queries join resource_action_traces, action_histories, resource_references
type Builder struct {
	baseQuery    string
	whereClauses []string
	args         []interface{}
	limit        int
	offset       int
}

// NewBuilder creates a query builder using Data Storage Service schema
// BR-CONTEXT-001: Historical context query builder
//
// Default values:
//   - Base query: JOIN query from resource_action_traces, action_histories, resource_references
//   - Limit: DefaultLimit (100) - prevents accidental large queries
//   - Offset: 0
//
// Schema Design Decision: DD-SCHEMA-001
// Context API uses Data Storage Service's authoritative schema
func NewBuilder() *Builder {
	// Data Storage Service schema with proper JOINs
	// Maps IncidentEvent model to resource_action_traces + related tables
	baseQuery := `SELECT 
    rat.id,
    rat.alert_name AS name,
    rat.alert_fingerprint,
    rat.action_id AS remediation_request_id,
    rr.namespace,
    rat.cluster_name,
    rat.environment,
    rr.kind AS target_resource,
    CASE rat.execution_status
        WHEN 'completed' THEN 'completed'
        WHEN 'failed' THEN 'failed'
        WHEN 'rolled-back' THEN 'failed'
        WHEN 'pending' THEN 'pending'
        WHEN 'executing' THEN 'processing'
        ELSE 'pending'
    END AS phase,
    rat.execution_status AS status,
    rat.alert_severity AS severity,
    rat.action_type,
    rat.action_timestamp AS start_time,
    rat.execution_end_time AS end_time,
    rat.execution_duration_ms AS duration,
    rat.execution_error AS error_message,
    rat.action_parameters::TEXT AS metadata,
    rat.created_at,
    rat.updated_at
FROM resource_action_traces rat
JOIN action_histories ah ON rat.action_history_id = ah.id
JOIN resource_references rr ON ah.resource_id = rr.id`

	return &Builder{
		baseQuery: baseQuery,
		limit:     DefaultLimit, // Default limit (BR-CONTEXT-007)
		offset:    0,            // Default offset
	}
}

// WithNamespace adds namespace filter (parameterized for SQL injection protection)
// BR-CONTEXT-002: Filter by namespace
//
// Example:
//
//	builder.WithNamespace("production")
//	// Generates: WHERE rr.namespace = $1
func (b *Builder) WithNamespace(namespace string) *Builder {
	paramNum := len(b.args) + 1
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("rr.namespace = $%d", paramNum))
	b.args = append(b.args, namespace)
	return b
}

// WithSeverity adds severity filter (parameterized)
// BR-CONTEXT-002: Filter by severity
//
// Example:
//
//	builder.WithSeverity("critical")
//	// Generates: WHERE rat.alert_severity = $1
func (b *Builder) WithSeverity(severity string) *Builder {
	paramNum := len(b.args) + 1
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("rat.alert_severity = $%d", paramNum))
	b.args = append(b.args, severity)
	return b
}

// WithClusterName adds cluster filter (parameterized)
// BR-CONTEXT-004: Filter by cluster (multi-cluster support)
//
// Example:
//
//	builder.WithClusterName("prod-us-west")
//	// Generates: WHERE rat.cluster_name = $1
func (b *Builder) WithClusterName(clusterName string) *Builder {
	paramNum := len(b.args) + 1
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("rat.cluster_name = $%d", paramNum))
	b.args = append(b.args, clusterName)
	return b
}

// WithEnvironment adds environment filter (parameterized)
// BR-CONTEXT-004: Filter by environment (production/staging/development)
//
// Example:
//
//	builder.WithEnvironment("production")
//	// Generates: WHERE rat.environment = $1
func (b *Builder) WithEnvironment(environment string) *Builder {
	paramNum := len(b.args) + 1
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("rat.environment = $%d", paramNum))
	b.args = append(b.args, environment)
	return b
}

// WithActionType adds action type filter (parameterized)
// BR-CONTEXT-004: Filter by action type
//
// Example:
//
//	builder.WithActionType("scale_deployment")
//	// Generates: WHERE rat.action_type = $1
func (b *Builder) WithActionType(actionType string) *Builder {
	paramNum := len(b.args) + 1
	b.whereClauses = append(b.whereClauses, fmt.Sprintf("rat.action_type = $%d", paramNum))
	b.args = append(b.args, actionType)
	return b
}

// WithTimeRange adds time range filter (parameterized)
// BR-CONTEXT-002: Filter by time range
//
// Example:
//
//	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
//	end := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)
//	builder.WithTimeRange(start, end)
//	// Generates: WHERE rat.action_timestamp BETWEEN $1 AND $2
//
// Updated for Data Storage Service schema (DD-SCHEMA-001)
func (b *Builder) WithTimeRange(start, end time.Time) *Builder {
	paramNum := len(b.args) + 1
	b.whereClauses = append(b.whereClauses,
		fmt.Sprintf("rat.action_timestamp BETWEEN $%d AND $%d", paramNum, paramNum+1))
	b.args = append(b.args, start, end)
	return b
}

// WithLimit sets query limit with boundary validation (1-1000)
// BR-CONTEXT-007: Pagination limit validation
//
// Boundary rules:
//   - Minimum: MinLimit (1) - at least one result
//   - Maximum: MaxLimit (1000) - prevent database overload
//   - Default: DefaultLimit (100) - if not set
//
// Returns ValidationError if limit is outside valid range.
func (b *Builder) WithLimit(limit int) error {
	if err := ValidateLimit(limit); err != nil {
		return err
	}
	b.limit = limit
	return nil
}

// WithOffset sets query offset with boundary validation (>= 0)
// BR-CONTEXT-007: Pagination offset validation
//
// Boundary rules:
//   - Minimum: MinOffset (0) - start of results
//   - Maximum: No upper limit (but consider performance)
//
// Returns ValidationError if offset is negative.
func (b *Builder) WithOffset(offset int) error {
	if err := ValidateOffset(offset); err != nil {
		return err
	}
	b.offset = offset
	return nil
}

// Build constructs final parameterized SQL query
// BR-CONTEXT-001: Generate query with all filters and pagination
//
// Query structure:
//  1. Base SELECT
//  2. WHERE clauses (if any filters)
//  3. ORDER BY created_at DESC (for consistent pagination)
//  4. LIMIT and OFFSET (parameterized)
//
// Returns:
//   - query: SQL query string with $1, $2, ... placeholders
//   - args: Slice of parameter values in order
//   - error: nil (for future extension)
//
// Example output:
//
//	query: "SELECT * FROM remediation_audit WHERE namespace = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3"
//	args: ["production", 100, 0]
//
// Note: Build() is idempotent - calling it multiple times returns the same result
func (b *Builder) Build() (string, []interface{}, error) {
	query := b.baseQuery

	// Add WHERE clauses if any filters were added
	if len(b.whereClauses) > 0 {
		query += " WHERE " + strings.Join(b.whereClauses, " AND ")
	}

	// Add ORDER BY for consistent pagination
	// BR-CONTEXT-007: Consistent ordering required for reliable pagination
	query += " ORDER BY rat.action_timestamp DESC"

	// Create args copy to avoid modifying builder state (for idempotency)
	args := make([]interface{}, len(b.args))
	copy(args, b.args)

	// Add LIMIT and OFFSET (parameterized)
	paramNum := len(args) + 1
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", paramNum, paramNum+1)
	args = append(args, b.limit, b.offset)

	return query, args, nil
}

// BuildCount constructs SQL query to count total matching records
// BR-CONTEXT-007: Total count needed for pagination metadata
//
// Returns:
//   - query: SQL count query with same WHERE filters as Build()
//   - args: Slice of parameter values (excludes LIMIT/OFFSET)
//
// Note: Count query uses same JOIN structure but excludes ORDER BY, LIMIT, OFFSET
func (b *Builder) BuildCount() (string, []interface{}) {
	// Start with SELECT COUNT(*) and same JOIN structure
	query := `SELECT COUNT(*)
FROM resource_action_traces rat
JOIN action_histories ah ON rat.action_history_id = ah.id
JOIN resource_references rr ON ah.resource_id = rr.id`

	// Add WHERE clauses if any filters were added
	if len(b.whereClauses) > 0 {
		query += " WHERE " + strings.Join(b.whereClauses, " AND ")
	}

	// Return args without LIMIT/OFFSET
	args := make([]interface{}, len(b.args))
	copy(args, b.args)

	return query, args
}
