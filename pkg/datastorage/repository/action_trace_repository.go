package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// ACTION TRACE REPOSITORY (TDD GREEN Phase)
// 📋 Authority: test/unit/datastorage/repository_adr033_test.go
// 📋 Tests Define Contract: Unit tests drive implementation
// ========================================
//
// This file implements ADR-033 multi-dimensional success tracking repository methods.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (repository_adr033_test.go)
// - This production code implements MINIMAL functionality to pass tests
// - Contract defined by test expectations
//
// Business Requirements:
// - BR-STORAGE-031-01: Incident-type success rate aggregation
// - BR-STORAGE-031-02: Workflow success rate aggregation
// - BR-STORAGE-031-04: AI execution mode tracking
// - BR-STORAGE-031-05: Multi-dimensional success rate aggregation
//
// ========================================

// ActionTraceRepository handles PostgreSQL operations for resource_action_traces table (ADR-033)
type ActionTraceRepository struct {
	db     *sql.DB
	logger logr.Logger
}

// NewActionTraceRepository creates a new repository instance for action traces
func NewActionTraceRepository(db *sql.DB, logger logr.Logger) *ActionTraceRepository {
	return &ActionTraceRepository{
		db:     db,
		logger: logger,
	}
}

// ========================================
// BR-STORAGE-031-01: Incident-Type Success Rate
// ========================================

// GetSuccessRateByIncidentType calculates success rate for a specific incident type
// This is the PRIMARY dimension for AI learning - tracks which workflows work for specific problems
func (r *ActionTraceRepository) GetSuccessRateByIncidentType(
	ctx context.Context,
	incidentType string,
	duration time.Duration,
	minSamples int,
) (*models.IncidentTypeSuccessRateResponse, error) {
	r.logger.V(1).Info("GetSuccessRateByIncidentType called",
		"incident_type", incidentType,
		"duration", duration,
		"min_samples", minSamples)

	// Calculate time threshold
	sinceTime := time.Now().Add(-duration)

	// Main aggregation query
	query := `
		SELECT
			incident_type,
			COUNT(*) as total_executions,
			SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END) as successful_executions,
			SUM(CASE WHEN execution_status = 'failed' THEN 1 ELSE 0 END) as failed_executions
		FROM resource_action_traces
		WHERE incident_type = $1
			AND action_timestamp >= $2
		GROUP BY incident_type
	`

	var (
		returnedIncidentType string
		totalExecutions      int
		successfulExecutions int
		failedExecutions     int
	)

	err := r.db.QueryRowContext(ctx, query, incidentType, sinceTime).Scan(
		&returnedIncidentType,
		&totalExecutions,
		&successfulExecutions,
		&failedExecutions,
	)

	if err == sql.ErrNoRows {
		// No data found - return response with zero values
		return &models.IncidentTypeSuccessRateResponse{
			IncidentType:         incidentType,
			TimeRange:            formatDuration(duration),
			TotalExecutions:      0,
			SuccessfulExecutions: 0,
			FailedExecutions:     0,
			SuccessRate:          0.0,
			Confidence:           confidenceInsufficientData,
			MinSamplesMet:        false,
			BreakdownByWorkflow:  []models.WorkflowBreakdownItem{},
		}, nil
	}

	if err != nil {
		r.logger.Error(err, "failed to query incident-type success rate", "incident_type", incidentType)
		return nil, fmt.Errorf("failed to query incident-type success rate: %w", err)
	}

	// Calculate success rate using helper
	successRate := calculateSuccessRatePercentage(successfulExecutions, totalExecutions)

	// Determine confidence level based on sample size
	confidence := calculateConfidence(totalExecutions)
	minSamplesMet := totalExecutions >= minSamples

	// Build response
	response := &models.IncidentTypeSuccessRateResponse{
		IncidentType:         incidentType,
		TimeRange:            formatDuration(duration),
		TotalExecutions:      totalExecutions,
		SuccessfulExecutions: successfulExecutions,
		FailedExecutions:     failedExecutions,
		SuccessRate:          successRate,
		Confidence:           confidence,
		MinSamplesMet:        minSamplesMet,
	}

	// Query workflow breakdown (only if we have data)
	if totalExecutions > 0 {
		workflowBreakdown, err := r.getWorkflowBreakdownForIncidentType(ctx, incidentType, sinceTime)
		if err != nil {
			r.logger.Info("failed to get workflow breakdown",
				"incident_type", incidentType,
				"error", err)
			// Don't fail the entire request for breakdown query failure
			workflowBreakdown = []models.WorkflowBreakdownItem{}
		}
		response.BreakdownByWorkflow = workflowBreakdown

		// Query AI execution mode stats
		aiStats, err := r.getAIExecutionModeForIncidentType(ctx, incidentType, sinceTime)
		if err != nil {
			r.logger.Info("failed to get AI execution mode stats",
				"incident_type", incidentType,
				"error", err)
			// Don't fail for AI stats query failure
		} else {
			response.AIExecutionMode = aiStats
		}
	}

	r.logger.Info("incident-type success rate calculated",
		"incident_type", incidentType,
		"total_executions", totalExecutions,
		"success_rate", successRate,
		"confidence", confidence)

	return response, nil
}

// getWorkflowBreakdownForIncidentType retrieves workflow breakdown for an incident type
func (r *ActionTraceRepository) getWorkflowBreakdownForIncidentType(
	ctx context.Context,
	incidentType string,
	sinceTime time.Time,
) ([]models.WorkflowBreakdownItem, error) {
	query := `
		SELECT
			workflow_id,
			workflow_version,
			COUNT(*) as executions,
			CAST(SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate
		FROM resource_action_traces
		WHERE incident_type = $1
			AND action_timestamp >= $2
			AND workflow_id IS NOT NULL
			AND workflow_id != ''
		GROUP BY workflow_id, workflow_version
		ORDER BY executions DESC
	`

	rows, err := r.db.QueryContext(ctx, query, incidentType, sinceTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query workflow breakdown: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var breakdown []models.WorkflowBreakdownItem
	for rows.Next() {
		var item models.WorkflowBreakdownItem
		if err := rows.Scan(&item.WorkflowID, &item.WorkflowVersion, &item.Executions, &item.SuccessRate); err != nil {
			return nil, fmt.Errorf("failed to scan workflow breakdown row: %w", err)
		}
		breakdown = append(breakdown, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workflow breakdown rows error: %w", err)
	}

	return breakdown, nil
}

// getAIExecutionModeForIncidentType retrieves AI execution mode statistics for an incident type
func (r *ActionTraceRepository) getAIExecutionModeForIncidentType(
	ctx context.Context,
	incidentType string,
	sinceTime time.Time,
) (*models.AIExecutionModeStats, error) {
	query := `
		SELECT
			COUNT(CASE WHEN ai_selected_workflow = true THEN 1 END) as catalog_selected,
			COUNT(CASE WHEN ai_chained_workflows = true THEN 1 END) as chained,
			COUNT(CASE WHEN ai_manual_escalation = true THEN 1 END) as manual_escalation
		FROM resource_action_traces
		WHERE incident_type = $1
			AND action_timestamp >= $2
	`

	var stats models.AIExecutionModeStats
	err := r.db.QueryRowContext(ctx, query, incidentType, sinceTime).Scan(
		&stats.CatalogSelected,
		&stats.Chained,
		&stats.ManualEscalation,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to query AI execution mode: %w", err)
	}

	return &stats, nil
}

// ========================================
// BR-STORAGE-031-02: Workflow Success Rate
// ========================================

// GetSuccessRateByWorkflow calculates success rate for a specific workflow
// This is the SECONDARY dimension - tracks which workflows are most effective overall
func (r *ActionTraceRepository) GetSuccessRateByWorkflow(
	ctx context.Context,
	workflowID string,
	workflowVersion string,
	duration time.Duration,
	minSamples int,
) (*models.WorkflowSuccessRateResponse, error) {
	r.logger.V(1).Info("GetSuccessRateByWorkflow called",
		"workflow_id", workflowID,
		"workflow_version", workflowVersion,
		"duration", duration,
		"min_samples", minSamples)

	sinceTime := time.Now().Add(-duration)

	// Main aggregation query
	query := `
		SELECT
			workflow_id,
			workflow_version,
			COUNT(*) as total_executions,
			SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END) as successful_executions,
			SUM(CASE WHEN execution_status = 'failed' THEN 1 ELSE 0 END) as failed_executions
		FROM resource_action_traces
		WHERE workflow_id = $1
			AND workflow_version = $2
			AND action_timestamp >= $3
		GROUP BY workflow_id, workflow_version
	`

	var (
		returnedWorkflowID      string
		returnedWorkflowVersion string
		totalExecutions         int
		successfulExecutions    int
		failedExecutions        int
	)

	err := r.db.QueryRowContext(ctx, query, workflowID, workflowVersion, sinceTime).Scan(
		&returnedWorkflowID,
		&returnedWorkflowVersion,
		&totalExecutions,
		&successfulExecutions,
		&failedExecutions,
	)

	if err == sql.ErrNoRows {
		// No data found
		return &models.WorkflowSuccessRateResponse{
			WorkflowID:              workflowID,
			WorkflowVersion:         workflowVersion,
			TimeRange:               formatDuration(duration),
			TotalExecutions:         0,
			SuccessfulExecutions:    0,
			FailedExecutions:        0,
			SuccessRate:             0.0,
			Confidence:              confidenceInsufficientData,
			MinSamplesMet:           false,
			BreakdownByIncidentType: []models.IncidentTypeBreakdownItem{},
		}, nil
	}

	if err != nil {
		r.logger.Error(err, "failed to query workflow success rate",
			"workflow_id", workflowID,
			"workflow_version", workflowVersion,
			"error", err)
		return nil, fmt.Errorf("failed to query workflow success rate: %w", err)
	}

	// Calculate success rate using helper
	successRate := calculateSuccessRatePercentage(successfulExecutions, totalExecutions)

	// Determine confidence level
	confidence := calculateConfidence(totalExecutions)
	minSamplesMet := totalExecutions >= minSamples

	// Build response
	response := &models.WorkflowSuccessRateResponse{
		WorkflowID:           workflowID,
		WorkflowVersion:      workflowVersion,
		TimeRange:            formatDuration(duration),
		TotalExecutions:      totalExecutions,
		SuccessfulExecutions: successfulExecutions,
		FailedExecutions:     failedExecutions,
		SuccessRate:          successRate,
		Confidence:           confidence,
		MinSamplesMet:        minSamplesMet,
	}

	// Query incident type breakdown (only if we have data)
	if totalExecutions > 0 {
		incidentBreakdown, err := r.getIncidentTypeBreakdownForWorkflow(ctx, workflowID, workflowVersion, sinceTime)
		if err != nil {
			r.logger.Info("failed to get incident type breakdown",
				"workflow_id", workflowID,
				"error", err)
			incidentBreakdown = []models.IncidentTypeBreakdownItem{}
		}
		response.BreakdownByIncidentType = incidentBreakdown

		// Query AI execution mode stats
		aiStats, err := r.getAIExecutionModeForWorkflow(ctx, workflowID, workflowVersion, sinceTime)
		if err != nil {
			r.logger.Info("failed to get AI execution mode stats",
				"workflow_id", workflowID,
				"error", err)
		} else {
			response.AIExecutionMode = aiStats
		}
	}

	r.logger.Info("workflow success rate calculated",
		"workflow_id", workflowID,
		"workflow_version", workflowVersion,
		"total_executions", totalExecutions,
		"success_rate", successRate,
		"confidence", confidence)

	return response, nil
}

// getIncidentTypeBreakdownForWorkflow retrieves incident type breakdown for a workflow
func (r *ActionTraceRepository) getIncidentTypeBreakdownForWorkflow(
	ctx context.Context,
	workflowID string,
	workflowVersion string,
	sinceTime time.Time,
) ([]models.IncidentTypeBreakdownItem, error) {
	query := `
		SELECT
			incident_type,
			COUNT(*) as executions,
			CAST(SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate
		FROM resource_action_traces
		WHERE workflow_id = $1
			AND workflow_version = $2
			AND action_timestamp >= $3
			AND incident_type IS NOT NULL
			AND incident_type != ''
		GROUP BY incident_type
		ORDER BY executions DESC
	`

	rows, err := r.db.QueryContext(ctx, query, workflowID, workflowVersion, sinceTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query incident type breakdown: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var breakdown []models.IncidentTypeBreakdownItem
	for rows.Next() {
		var item models.IncidentTypeBreakdownItem
		if err := rows.Scan(&item.IncidentType, &item.Executions, &item.SuccessRate); err != nil {
			return nil, fmt.Errorf("failed to scan incident type breakdown row: %w", err)
		}
		breakdown = append(breakdown, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("incident type breakdown rows error: %w", err)
	}

	return breakdown, nil
}

// getAIExecutionModeForWorkflow retrieves AI execution mode statistics for a workflow
func (r *ActionTraceRepository) getAIExecutionModeForWorkflow(
	ctx context.Context,
	workflowID string,
	workflowVersion string,
	sinceTime time.Time,
) (*models.AIExecutionModeStats, error) {
	query := `
		SELECT
			COUNT(CASE WHEN ai_selected_workflow = true THEN 1 END) as catalog_selected,
			COUNT(CASE WHEN ai_chained_workflows = true THEN 1 END) as chained,
			COUNT(CASE WHEN ai_manual_escalation = true THEN 1 END) as manual_escalation
		FROM resource_action_traces
		WHERE workflow_id = $1
			AND workflow_version = $2
			AND action_timestamp >= $3
	`

	var stats models.AIExecutionModeStats
	err := r.db.QueryRowContext(ctx, query, workflowID, workflowVersion, sinceTime).Scan(
		&stats.CatalogSelected,
		&stats.Chained,
		&stats.ManualEscalation,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to query AI execution mode: %w", err)
	}

	return &stats, nil
}

// ========================================
// HELPER FUNCTIONS (TDD REFACTOR)
// ========================================

// Confidence level thresholds (BR-STORAGE-031-01)
// These thresholds determine statistical confidence in success rate calculations
const (
	confidenceLevelHigh   = 100 // High confidence: >=100 samples
	confidenceLevelMedium = 20  // Medium confidence: 20-99 samples
	confidenceLevelLow    = 5   // Low confidence: 5-19 samples
)

// Confidence level labels
const (
	confidenceHigh             = "high"
	confidenceMedium           = "medium"
	confidenceLow              = "low"
	confidenceInsufficientData = "insufficient_data"
)

// calculateConfidence determines confidence level based on sample size
// Confidence thresholds (per BR-STORAGE-031-01):
// - high: >=100 samples
// - medium: 20-99 samples
// - low: 5-19 samples
// - insufficient_data: <5 samples
func calculateConfidence(sampleSize int) string {
	switch {
	case sampleSize >= confidenceLevelHigh:
		return confidenceHigh
	case sampleSize >= confidenceLevelMedium:
		return confidenceMedium
	case sampleSize >= confidenceLevelLow:
		return confidenceLow
	default:
		return confidenceInsufficientData
	}
}

// calculateSuccessRatePercentage calculates success rate as a percentage
// Returns 0.0 if totalExecutions is 0 to avoid division by zero
func calculateSuccessRatePercentage(successfulExecutions, totalExecutions int) float64 {
	if totalExecutions == 0 {
		return 0.0
	}
	return (float64(successfulExecutions) / float64(totalExecutions)) * 100.0
}

// formatDuration formats a duration into a human-readable string (e.g., "7d", "30d")
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return "1h"
}

// parseTimeRange parses a time range string (e.g., "7d", "30d", "1h") into a time.Duration
// Returns error for invalid format
func parseTimeRange(timeRange string) (time.Duration, error) {
	// Common time ranges
	switch timeRange {
	case "1h":
		return 1 * time.Hour, nil
	case "24h", "1d":
		return 24 * time.Hour, nil
	case "7d":
		return 7 * 24 * time.Hour, nil
	case "30d":
		return 30 * 24 * time.Hour, nil
	case "90d":
		return 90 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid time_range: %s (expected: 1h, 1d, 7d, 30d, 90d)", timeRange)
	}
}

// ========================================
// BR-STORAGE-031-05: Multi-Dimensional Success Rate
// ========================================

// GetSuccessRateMultiDimensional calculates success rate across multiple dimensions
//
// BR-STORAGE-031-05: Multi-Dimensional Success Rate API
// ADR-033: Remediation Workflow Catalog - Multi-dimensional tracking
//
// Supports any combination of: incident_type, workflow_id + workflow_version, action_type
func (r *ActionTraceRepository) GetSuccessRateMultiDimensional(
	ctx context.Context,
	query *models.MultiDimensionalQuery,
) (*models.MultiDimensionalSuccessRateResponse, error) {
	r.logger.V(1).Info("GetSuccessRateMultiDimensional called",
		"incident_type", query.IncidentType,
		"workflow_id", query.WorkflowID,
		"workflow_version", query.WorkflowVersion,
		"action_type", query.ActionType,
		"time_range", query.TimeRange,
		"min_samples", query.MinSamples)

	// Validation: workflow_version requires workflow_id
	if query.WorkflowVersion != "" && query.WorkflowID == "" {
		return nil, fmt.Errorf("workflow_version requires workflow_id to be specified")
	}

	// Parse time range
	duration, err := parseTimeRange(query.TimeRange)
	if err != nil {
		return nil, err
	}

	// Calculate time threshold
	sinceTime := time.Now().Add(-duration)

	// Build dynamic WHERE clause based on provided dimensions
	var whereClauses []string
	var args []interface{}
	argIndex := 1

	if query.IncidentType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("incident_type = $%d", argIndex))
		args = append(args, query.IncidentType)
		argIndex++
	}

	if query.WorkflowID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("workflow_id = $%d", argIndex))
		args = append(args, query.WorkflowID)
		argIndex++

		if query.WorkflowVersion != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("workflow_version = $%d", argIndex))
			args = append(args, query.WorkflowVersion)
			argIndex++
		}
	}

	if query.ActionType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("action_type = $%d", argIndex))
		args = append(args, query.ActionType)
		argIndex++
	}

	// Add time range filter
	whereClauses = append(whereClauses, fmt.Sprintf("action_timestamp >= $%d", argIndex))
	args = append(args, sinceTime)

	// Build SQL query
	var whereClause string
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + whereClauses[0]
		for i := 1; i < len(whereClauses); i++ {
			whereClause += " AND " + whereClauses[i]
		}
	}

	sqlQuery := fmt.Sprintf(`
		SELECT
			COUNT(*) AS total_executions,
			COALESCE(SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END), 0) AS successful_executions,
			COALESCE(SUM(CASE WHEN execution_status = 'failed' THEN 1 ELSE 0 END), 0) AS failed_executions
		FROM resource_action_traces
		%s
	`, whereClause)

	// Execute query
	var totalExecutions, successfulExecutions, failedExecutions int
	err = r.db.QueryRowContext(ctx, sqlQuery, args...).Scan(
		&totalExecutions,
		&successfulExecutions,
		&failedExecutions,
	)

	// Handle no rows case (empty database or no matching data)
	if err == sql.ErrNoRows || totalExecutions == 0 {
		// Return response with zero values and insufficient_data confidence
		return &models.MultiDimensionalSuccessRateResponse{
			Dimensions: models.QueryDimensions{
				IncidentType:    query.IncidentType,
				WorkflowID:      query.WorkflowID,
				WorkflowVersion: query.WorkflowVersion,
				ActionType:      query.ActionType,
			},
			TimeRange:            query.TimeRange,
			TotalExecutions:      0,
			SuccessfulExecutions: 0,
			FailedExecutions:     0,
			SuccessRate:          0.0,
			Confidence:           confidenceInsufficientData,
			MinSamplesMet:        false,
		}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query multi-dimensional success rate: %w", err)
	}

	// Calculate success rate and confidence
	successRate := calculateSuccessRatePercentage(successfulExecutions, totalExecutions)
	confidence := calculateConfidence(totalExecutions)
	minSamplesMet := totalExecutions >= query.MinSamples

	// Build response
	response := &models.MultiDimensionalSuccessRateResponse{
		Dimensions: models.QueryDimensions{
			IncidentType:    query.IncidentType,
			WorkflowID:      query.WorkflowID,
			WorkflowVersion: query.WorkflowVersion,
			ActionType:      query.ActionType,
		},
		TimeRange:            query.TimeRange,
		TotalExecutions:      totalExecutions,
		SuccessfulExecutions: successfulExecutions,
		FailedExecutions:     failedExecutions,
		SuccessRate:          successRate,
		Confidence:           confidence,
		MinSamplesMet:        minSamplesMet,
	}

	r.logger.V(1).Info("GetSuccessRateMultiDimensional result",
		"total_executions", totalExecutions,
		"successful_executions", successfulExecutions,
		"success_rate", successRate,
		"confidence", confidence)

	return response, nil
}
