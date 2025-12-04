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

package adapter

import (
	"database/sql"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
)

// DBAdapter adapts sql.DB to work with our Handler
// Day 3: Real database implementation using query builder
type DBAdapter struct {
	db     *sql.DB
	logger logr.Logger
}

// NewDBAdapter creates a new database adapter
func NewDBAdapter(db *sql.DB, logger logr.Logger) *DBAdapter {
	return &DBAdapter{
		db:     db,
		logger: logger,
	}
}

// Query executes a filtered query against PostgreSQL
// BR-STORAGE-021: Query database with filters and pagination
// BR-STORAGE-022: Apply dynamic filters
// BR-STORAGE-023: Pagination support
func (d *DBAdapter) Query(filters map[string]string, limit, offset int) ([]map[string]interface{}, error) {
	d.logger.V(1).Info("DBAdapter.Query called",
		"filters", filters,
		"limit", limit,
		"offset", offset,
	)

	// Build query using query builder
	builder := query.NewBuilder(query.WithLogger(d.logger))

	// Apply filters
	if ns, ok := filters["namespace"]; ok && ns != "" {
		builder = builder.WithNamespace(ns)
	}
	if signalName, ok := filters["signal_name"]; ok && signalName != "" {
		builder = builder.WithSignalName(signalName)
	}
	if sev, ok := filters["severity"]; ok && sev != "" {
		builder = builder.WithSeverity(sev)
	}
	if cluster, ok := filters["cluster"]; ok && cluster != "" {
		builder = builder.WithCluster(cluster)
	}
	if env, ok := filters["environment"]; ok && env != "" {
		builder = builder.WithEnvironment(env)
	}
	if actionType, ok := filters["action_type"]; ok && actionType != "" {
		builder = builder.WithActionType(actionType)
	}

	// Apply pagination
	builder = builder.WithLimit(limit).WithOffset(offset)

	// Build SQL query
	sqlQuery, args, err := builder.Build()
	if err != nil {
		d.logger.Error(err, "Failed to build SQL query",
			"filters", filters,
		)
		return nil, fmt.Errorf("query builder error: %w", err)
	}

	// Convert ? placeholders back to PostgreSQL $1, $2, etc.
	// (query builder uses ? for test compatibility, but PostgreSQL needs $N)
	pgQuery := convertPlaceholdersToPostgreSQL(sqlQuery, len(args))

	d.logger.V(1).Info("Executing SQL query",
		"sql", pgQuery,
		"arg_count", len(args),
	)

	// Execute query
	rows, err := d.db.Query(pgQuery, args...)
	if err != nil {
		d.logger.Error(err, "Failed to execute SQL query",
			"sql", pgQuery,
		)
		return nil, fmt.Errorf("database query error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		d.logger.Error(err, "Failed to get column names")
		return nil, fmt.Errorf("column retrieval error: %w", err)
	}

	// Scan results into map slices
	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		// Create slice for scanning
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan row
		if err := rows.Scan(valuePtrs...); err != nil {
			d.logger.Error(err, "Failed to scan row")
			return nil, fmt.Errorf("row scan error: %w", err)
		}

		// Convert to map
		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	// Check for iteration errors
	if err := rows.Err(); err != nil {
		d.logger.Error(err, "Row iteration error")
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	d.logger.Info("Query executed successfully",
		"result_count", len(results),
		"limit", limit,
		"offset", offset,
	)

	return results, nil
}

// CountTotal returns the total number of records matching the filters
// 🚨 FIX: Separate COUNT(*) query for accurate pagination metadata
// This fixes the critical bug where pagination.total was set to len(array) instead of database count
// See: docs/services/stateless/data-storage/implementation/DATA-STORAGE-INTEGRATION-TEST-TRIAGE.md
func (d *DBAdapter) CountTotal(filters map[string]string) (int64, error) {
	d.logger.V(1).Info("DBAdapter.CountTotal called",
		"filters", filters,
	)

	// Build count query using query builder
	builder := query.NewBuilder(query.WithLogger(d.logger))

	// Apply filters (same as Query method)
	if ns, ok := filters["namespace"]; ok && ns != "" {
		builder = builder.WithNamespace(ns)
	}
	if signalName, ok := filters["signal_name"]; ok && signalName != "" {
		builder = builder.WithSignalName(signalName)
	}
	if sev, ok := filters["severity"]; ok && sev != "" {
		builder = builder.WithSeverity(sev)
	}
	if cluster, ok := filters["cluster"]; ok && cluster != "" {
		builder = builder.WithCluster(cluster)
	}
	if env, ok := filters["environment"]; ok && env != "" {
		builder = builder.WithEnvironment(env)
	}
	if actionType, ok := filters["action_type"]; ok && actionType != "" {
		builder = builder.WithActionType(actionType)
	}

	// Build SQL query for count
	sqlQuery, args, err := builder.BuildCount()
	if err != nil {
		d.logger.Error(err, "Failed to build COUNT query",
			"filters", filters,
		)
		return 0, fmt.Errorf("count query builder error: %w", err)
	}

	// Convert ? placeholders to PostgreSQL $1, $2, etc.
	pgQuery := convertPlaceholdersToPostgreSQL(sqlQuery, len(args))

	d.logger.V(1).Info("Executing COUNT query",
		"sql", pgQuery,
		"arg_count", len(args),
	)

	// Execute count query
	var count int64
	err = d.db.QueryRow(pgQuery, args...).Scan(&count)
	if err != nil {
		d.logger.Error(err, "Failed to execute COUNT query",
			"sql", pgQuery,
		)
		return 0, fmt.Errorf("count query error: %w", err)
	}

	d.logger.Info("COUNT query executed successfully",
		"total_count", count,
	)

	return count, nil
}

// Get retrieves a single incident by ID
// BR-STORAGE-021: Get incident by ID
func (d *DBAdapter) Get(id int) (map[string]interface{}, error) {
	d.logger.V(1).Info("DBAdapter.Get called",
		"id", id,
	)

	// Query for specific ID
	// Note: Using direct SQL here since it's a simple ID lookup
	sqlQuery := `
		SELECT *
		FROM resource_action_traces
		WHERE id = $1
		LIMIT 1
	`

	rows, err := d.db.Query(sqlQuery, id)
	if err != nil {
		d.logger.Error(err, "Failed to execute Get query",
			"id", id,
		)
		return nil, fmt.Errorf("database query error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Check if any rows returned
	if !rows.Next() {
		d.logger.V(1).Info("No incident found with ID",
			"id", id,
		)
		return nil, nil // Not found
	}

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		d.logger.Error(err, "Failed to get column names")
		return nil, fmt.Errorf("column retrieval error: %w", err)
	}

	// Create slice for scanning
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	// Scan row
	if err := rows.Scan(valuePtrs...); err != nil {
		d.logger.Error(err, "Failed to scan row",
			"id", id,
		)
		return nil, fmt.Errorf("row scan error: %w", err)
	}

	// Convert to map
	result := make(map[string]interface{})
	for i, col := range columns {
		result[col] = values[i]
	}

	d.logger.Info("Incident retrieved successfully",
		"id", id,
	)

	return result, nil
}
