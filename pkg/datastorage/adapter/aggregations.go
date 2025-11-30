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
	"time"

	"github.com/go-logr/logr"
)

// ========================================
// AGGREGATION METHODS (BR-STORAGE-030)
// TDD GREEN Phase: Minimal stub implementations
// TODO: REFACTOR phase will add real PostgreSQL aggregation SQL
// ========================================

// AggregateSuccessRate calculates success rate for a workflow
// BR-STORAGE-031: Success rate aggregation
// TDD REFACTOR Phase: Real PostgreSQL aggregation with exact count calculations
func (d *DBAdapter) AggregateSuccessRate(workflowID string) (map[string]interface{}, error) {
	d.logger.V(1).Info("DBAdapter.AggregateSuccessRate called",
		"workflow_id", workflowID,
	)

	// REFACTOR: Real PostgreSQL aggregation query with CASE statements
	// ✅ Behavior + Correctness: Returns exact counts from database
	// Query by action_id (workflow_id) as per schema design
	// ✅ Edge Case: COALESCE handles NULL from SUM() when no rows match
	sqlQuery := `
		SELECT
			COUNT(*) as total_count,
			COALESCE(SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END), 0) as success_count,
			COALESCE(SUM(CASE WHEN execution_status = 'failed' THEN 1 ELSE 0 END), 0) as failure_count,
			CASE
				WHEN COUNT(*) = 0 THEN 0.0
				ELSE CAST(SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*)
			END as success_rate
		FROM resource_action_traces
		WHERE action_id = $1
	`

	rows, err := d.db.Query(sqlQuery, workflowID)
	if err != nil {
		d.logger.Error("Failed to execute success rate aggregation",
			"error", err,
			"workflow_id", workflowID,
		)
		return nil, fmt.Errorf("database aggregation error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Parse aggregation results
	if !rows.Next() {
		// No rows found - return zero counts
		return map[string]interface{}{
			"workflow_id":   workflowID,
			"total_count":   0,
			"success_count": 0,
			"failure_count": 0,
			"success_rate":  0.0,
		}, nil
	}

	var totalCount, successCount, failureCount int
	var successRate float64

	if err := rows.Scan(&totalCount, &successCount, &failureCount, &successRate); err != nil {
		d.logger.Error("Failed to scan aggregation results",
			"error", err,
			"workflow_id", workflowID,
		)
		return nil, fmt.Errorf("result scan error: %w", err)
	}

	d.logger.Info("Success rate aggregation completed",
		"workflow_id", workflowID,
		"total_count", totalCount,
		"success_count", successCount,
		"success_rate", successRate,
	)

	// ✅ CORRECTNESS: Return exact database counts
	return map[string]interface{}{
		"workflow_id":   workflowID,
		"total_count":   totalCount,
		"success_count": successCount,
		"failure_count": failureCount,
		"success_rate":  successRate,
	}, nil
}

// AggregateByNamespace groups incidents by namespace
// BR-STORAGE-032: Namespace grouping aggregation
// TDD REFACTOR Phase: Real PostgreSQL GROUP BY with ordering
func (d *DBAdapter) AggregateByNamespace() (map[string]interface{}, error) {
	d.logger.V(1).Info("DBAdapter.AggregateByNamespace called")

	// REFACTOR: Real PostgreSQL GROUP BY query with descending order
	// ✅ Behavior + Correctness: Returns exact counts per namespace
	// Note: resource_action_traces uses cluster_name column (schema compatibility)
	// Filter out empty/null namespaces for cleaner aggregation results
	sqlQuery := `
		SELECT
			cluster_name as namespace,
			COUNT(*) as count
		FROM resource_action_traces
		WHERE cluster_name IS NOT NULL AND cluster_name != ''
		GROUP BY cluster_name
		ORDER BY count DESC
	`

	rows, err := d.db.Query(sqlQuery)
	if err != nil {
		d.logger.Error("Failed to execute namespace aggregation",
			"error", err,
		)
		return nil, fmt.Errorf("database aggregation error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Parse aggregation results
	aggregations := []map[string]interface{}{}

	for rows.Next() {
		var namespace sql.NullString
		var count int

		if err := rows.Scan(&namespace, &count); err != nil {
			d.logger.Error("Failed to scan namespace aggregation row",
				"error", err,
			)
			return nil, fmt.Errorf("result scan error: %w", err)
		}

		// Handle NULL namespaces (convert to empty string or skip)
		namespaceValue := ""
		if namespace.Valid {
			namespaceValue = namespace.String
		}

		aggregations = append(aggregations, map[string]interface{}{
			"namespace": namespaceValue,
			"count":     count,
		})
	}

	d.logger.Info("Namespace aggregation completed",
		"namespace_count", len(aggregations),
	)

	// ✅ CORRECTNESS: Return exact database GROUP BY results
	return map[string]interface{}{
		"aggregations": aggregations,
	}, nil
}

// AggregateBySeverity groups incidents by severity
// BR-STORAGE-033: Severity distribution aggregation
// TDD REFACTOR Phase: Real PostgreSQL GROUP BY with custom severity ordering
func (d *DBAdapter) AggregateBySeverity() (map[string]interface{}, error) {
	d.logger.V(1).Info("DBAdapter.AggregateBySeverity called")

	// REFACTOR: Real PostgreSQL GROUP BY with CASE-based severity ordering
	// ✅ Behavior + Correctness: Returns exact counts per severity level
	// Filter out empty/null severities for cleaner aggregation results
	sqlQuery := `
		SELECT
			signal_severity as severity,
			COUNT(*) as count
		FROM resource_action_traces
		WHERE signal_severity IS NOT NULL AND signal_severity != ''
		GROUP BY signal_severity
		ORDER BY
			CASE signal_severity
				WHEN 'critical' THEN 1
				WHEN 'high' THEN 2
				WHEN 'medium' THEN 3
				WHEN 'low' THEN 4
				ELSE 5
			END
	`

	rows, err := d.db.Query(sqlQuery)
	if err != nil {
		d.logger.Error("Failed to execute severity aggregation",
			"error", err,
		)
		return nil, fmt.Errorf("database aggregation error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Parse aggregation results
	aggregations := []map[string]interface{}{}

	for rows.Next() {
		var severity string
		var count int

		if err := rows.Scan(&severity, &count); err != nil {
			d.logger.Error("Failed to scan severity aggregation row",
				"error", err,
			)
			return nil, fmt.Errorf("result scan error: %w", err)
		}

		aggregations = append(aggregations, map[string]interface{}{
			"severity": severity,
			"count":    count,
		})
	}

	d.logger.Info("Severity aggregation completed",
		"severity_levels", len(aggregations),
	)

	// ✅ CORRECTNESS: Return exact database GROUP BY results
	return map[string]interface{}{
		"aggregations": aggregations,
	}, nil
}

// AggregateIncidentTrend returns incident counts over time
// BR-STORAGE-034: Incident trend aggregation
// TDD REFACTOR Phase: Real PostgreSQL time-series aggregation with interval filtering
func (d *DBAdapter) AggregateIncidentTrend(period string) (map[string]interface{}, error) {
	d.logger.V(1).Info("DBAdapter.AggregateIncidentTrend called",
		"period", period,
	)

	// Convert period to PostgreSQL interval
	var intervalStr string
	switch period {
	case "7d":
		intervalStr = "7 days"
	case "30d":
		intervalStr = "30 days"
	case "90d":
		intervalStr = "90 days"
	default:
		intervalStr = "7 days" // Fallback to 7 days
	}

	// REFACTOR: Real PostgreSQL time-series aggregation
	// ✅ Behavior + Correctness: Returns exact daily counts within time period
	sqlQuery := `
		SELECT
			DATE(action_timestamp) as date,
			COUNT(*) as count
		FROM resource_action_traces
		WHERE action_timestamp >= NOW() - INTERVAL '` + intervalStr + `'
		GROUP BY DATE(action_timestamp)
		ORDER BY date ASC
	`

	rows, err := d.db.Query(sqlQuery)
	if err != nil {
		d.logger.Error("Failed to execute incident trend aggregation",
			"error", err,
			"period", period,
		)
		return nil, fmt.Errorf("database aggregation error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Parse aggregation results
	dataPoints := []map[string]interface{}{}

	for rows.Next() {
		var date time.Time
		var count int

		if err := rows.Scan(&date, &count); err != nil {
			d.logger.Error("Failed to scan trend aggregation row",
				"error", err,
			)
			return nil, fmt.Errorf("result scan error: %w", err)
		}

		dataPoints = append(dataPoints, map[string]interface{}{
			"date":  date.Format("2006-01-02"), // ISO 8601 date format
			"count": count,
		})
	}

	d.logger.Info("Incident trend aggregation completed",
		"period", period,
		"data_points", len(dataPoints),
	)

	// ✅ CORRECTNESS: Return exact database time-series aggregation
	return map[string]interface{}{
		"period":      period,
		"data_points": dataPoints,
	}, nil
}
