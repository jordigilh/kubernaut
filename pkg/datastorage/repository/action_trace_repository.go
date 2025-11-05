package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// ACTION TRACE REPOSITORY (TDD GREEN Phase)
// 📋 Authority: IMPLEMENTATION_PLAN_V5.0.md Day 13.1
// 📋 Tests: test/unit/datastorage/repository_adr033_test.go
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
// - BR-STORAGE-031-02: Playbook success rate aggregation
// - BR-STORAGE-031-04: AI execution mode tracking
// - BR-STORAGE-031-05: Multi-dimensional success rate aggregation
//
// ========================================

// ActionTraceRepository handles PostgreSQL operations for resource_action_traces table (ADR-033)
type ActionTraceRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewActionTraceRepository creates a new repository instance for action traces
func NewActionTraceRepository(db *sql.DB, logger *zap.Logger) *ActionTraceRepository {
	return &ActionTraceRepository{
		db:     db,
		logger: logger,
	}
}

// ========================================
// BR-STORAGE-031-01: Incident-Type Success Rate
// ========================================

// GetSuccessRateByIncidentType calculates success rate for a specific incident type
// This is the PRIMARY dimension for AI learning - tracks which playbooks work for specific problems
func (r *ActionTraceRepository) GetSuccessRateByIncidentType(
	ctx context.Context,
	incidentType string,
	duration time.Duration,
	minSamples int,
) (*models.IncidentTypeSuccessRateResponse, error) {
	r.logger.Debug("GetSuccessRateByIncidentType called",
		zap.String("incident_type", incidentType),
		zap.Duration("duration", duration),
		zap.Int("min_samples", minSamples))

	// Calculate time threshold
	sinceTime := time.Now().Add(-duration)

	// Main aggregation query
	query := `
		SELECT
			incident_type,
			COUNT(*) as total_executions,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as successful_executions,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed_executions
		FROM resource_action_traces
		WHERE incident_type = $1
			AND action_timestamp >= $2
		GROUP BY incident_type
	`

	var (
		returnedIncidentType  string
		totalExecutions       int
		successfulExecutions  int
		failedExecutions      int
	)

	err := r.db.QueryRowContext(ctx, query, incidentType, sinceTime).Scan(
		&returnedIncidentType,
		&totalExecutions,
		&successfulExecutions,
		&failedExecutions,
	)

	if err == sql.ErrNoRows {
		// No data found - return response with zero values (TDD REFACTOR: use constant)
		return &models.IncidentTypeSuccessRateResponse{
			IncidentType:         incidentType,
			TimeRange:            formatDuration(duration),
			TotalExecutions:      0,
			SuccessfulExecutions: 0,
			FailedExecutions:     0,
			SuccessRate:          0.0,
			Confidence:           confidenceInsufficientData,
			MinSamplesMet:        false,
			BreakdownByPlaybook:  []models.PlaybookBreakdownItem{},
		}, nil
	}

	if err != nil {
		r.logger.Error("failed to query incident-type success rate",
			zap.String("incident_type", incidentType),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query incident-type success rate: %w", err)
	}

	// Calculate success rate using helper (TDD REFACTOR: extracted duplicate logic)
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

	// Query playbook breakdown (only if we have data)
	if totalExecutions > 0 {
		playbookBreakdown, err := r.getPlaybookBreakdownForIncidentType(ctx, incidentType, sinceTime)
		if err != nil {
			r.logger.Warn("failed to get playbook breakdown",
				zap.String("incident_type", incidentType),
				zap.Error(err))
			// Don't fail the entire request for breakdown query failure
			playbookBreakdown = []models.PlaybookBreakdownItem{}
		}
		response.BreakdownByPlaybook = playbookBreakdown

		// Query AI execution mode stats
		aiStats, err := r.getAIExecutionModeForIncidentType(ctx, incidentType, sinceTime)
		if err != nil {
			r.logger.Warn("failed to get AI execution mode stats",
				zap.String("incident_type", incidentType),
				zap.Error(err))
			// Don't fail for AI stats query failure
		} else {
			response.AIExecutionMode = aiStats
		}
	}

	r.logger.Info("incident-type success rate calculated",
		zap.String("incident_type", incidentType),
		zap.Int("total_executions", totalExecutions),
		zap.Float64("success_rate", successRate),
		zap.String("confidence", confidence))

	return response, nil
}

// getPlaybookBreakdownForIncidentType retrieves playbook breakdown for an incident type
func (r *ActionTraceRepository) getPlaybookBreakdownForIncidentType(
	ctx context.Context,
	incidentType string,
	sinceTime time.Time,
) ([]models.PlaybookBreakdownItem, error) {
	query := `
		SELECT
			playbook_id,
			playbook_version,
			COUNT(*) as executions,
			CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate
		FROM resource_action_traces
		WHERE incident_type = $1
			AND action_timestamp >= $2
			AND playbook_id IS NOT NULL
			AND playbook_id != ''
		GROUP BY playbook_id, playbook_version
		ORDER BY executions DESC
	`

	rows, err := r.db.QueryContext(ctx, query, incidentType, sinceTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query playbook breakdown: %w", err)
	}
	defer rows.Close()

	var breakdown []models.PlaybookBreakdownItem
	for rows.Next() {
		var item models.PlaybookBreakdownItem
		if err := rows.Scan(&item.PlaybookID, &item.PlaybookVersion, &item.Executions, &item.SuccessRate); err != nil {
			return nil, fmt.Errorf("failed to scan playbook breakdown row: %w", err)
		}
		breakdown = append(breakdown, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("playbook breakdown rows error: %w", err)
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
			COUNT(CASE WHEN ai_selected_playbook = true THEN 1 END) as catalog_selected,
			COUNT(CASE WHEN ai_chained_playbooks = true THEN 1 END) as chained,
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
// BR-STORAGE-031-02: Playbook Success Rate
// ========================================

// GetSuccessRateByPlaybook calculates success rate for a specific playbook
// This is the SECONDARY dimension - tracks which playbooks are most effective overall
func (r *ActionTraceRepository) GetSuccessRateByPlaybook(
	ctx context.Context,
	playbookID string,
	playbookVersion string,
	duration time.Duration,
	minSamples int,
) (*models.PlaybookSuccessRateResponse, error) {
	r.logger.Debug("GetSuccessRateByPlaybook called",
		zap.String("playbook_id", playbookID),
		zap.String("playbook_version", playbookVersion),
		zap.Duration("duration", duration),
		zap.Int("min_samples", minSamples))

	sinceTime := time.Now().Add(-duration)

	// Main aggregation query
	query := `
		SELECT
			playbook_id,
			playbook_version,
			COUNT(*) as total_executions,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as successful_executions,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed_executions
		FROM resource_action_traces
		WHERE playbook_id = $1
			AND playbook_version = $2
			AND action_timestamp >= $3
		GROUP BY playbook_id, playbook_version
	`

	var (
		returnedPlaybookID      string
		returnedPlaybookVersion string
		totalExecutions         int
		successfulExecutions    int
		failedExecutions        int
	)

	err := r.db.QueryRowContext(ctx, query, playbookID, playbookVersion, sinceTime).Scan(
		&returnedPlaybookID,
		&returnedPlaybookVersion,
		&totalExecutions,
		&successfulExecutions,
		&failedExecutions,
	)

	if err == sql.ErrNoRows {
		// No data found (TDD REFACTOR: use constant)
		return &models.PlaybookSuccessRateResponse{
			PlaybookID:              playbookID,
			PlaybookVersion:         playbookVersion,
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
		r.logger.Error("failed to query playbook success rate",
			zap.String("playbook_id", playbookID),
			zap.String("playbook_version", playbookVersion),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query playbook success rate: %w", err)
	}

	// Calculate success rate using helper (TDD REFACTOR: extracted duplicate logic)
	successRate := calculateSuccessRatePercentage(successfulExecutions, totalExecutions)

	// Determine confidence level
	confidence := calculateConfidence(totalExecutions)
	minSamplesMet := totalExecutions >= minSamples

	// Build response
	response := &models.PlaybookSuccessRateResponse{
		PlaybookID:           playbookID,
		PlaybookVersion:      playbookVersion,
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
		incidentBreakdown, err := r.getIncidentTypeBreakdownForPlaybook(ctx, playbookID, playbookVersion, sinceTime)
		if err != nil {
			r.logger.Warn("failed to get incident type breakdown",
				zap.String("playbook_id", playbookID),
				zap.Error(err))
			incidentBreakdown = []models.IncidentTypeBreakdownItem{}
		}
		response.BreakdownByIncidentType = incidentBreakdown

		// Query AI execution mode stats
		aiStats, err := r.getAIExecutionModeForPlaybook(ctx, playbookID, playbookVersion, sinceTime)
		if err != nil {
			r.logger.Warn("failed to get AI execution mode stats",
				zap.String("playbook_id", playbookID),
				zap.Error(err))
		} else {
			response.AIExecutionMode = aiStats
		}
	}

	r.logger.Info("playbook success rate calculated",
		zap.String("playbook_id", playbookID),
		zap.String("playbook_version", playbookVersion),
		zap.Int("total_executions", totalExecutions),
		zap.Float64("success_rate", successRate),
		zap.String("confidence", confidence))

	return response, nil
}

// getIncidentTypeBreakdownForPlaybook retrieves incident type breakdown for a playbook
func (r *ActionTraceRepository) getIncidentTypeBreakdownForPlaybook(
	ctx context.Context,
	playbookID string,
	playbookVersion string,
	sinceTime time.Time,
) ([]models.IncidentTypeBreakdownItem, error) {
	query := `
		SELECT
			incident_type,
			COUNT(*) as executions,
			CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate
		FROM resource_action_traces
		WHERE playbook_id = $1
			AND playbook_version = $2
			AND action_timestamp >= $3
			AND incident_type IS NOT NULL
			AND incident_type != ''
		GROUP BY incident_type
		ORDER BY executions DESC
	`

	rows, err := r.db.QueryContext(ctx, query, playbookID, playbookVersion, sinceTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query incident type breakdown: %w", err)
	}
	defer rows.Close()

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

// getAIExecutionModeForPlaybook retrieves AI execution mode statistics for a playbook
func (r *ActionTraceRepository) getAIExecutionModeForPlaybook(
	ctx context.Context,
	playbookID string,
	playbookVersion string,
	sinceTime time.Time,
) (*models.AIExecutionModeStats, error) {
	query := `
		SELECT
			COUNT(CASE WHEN ai_selected_playbook = true THEN 1 END) as catalog_selected,
			COUNT(CASE WHEN ai_chained_playbooks = true THEN 1 END) as chained,
			COUNT(CASE WHEN ai_manual_escalation = true THEN 1 END) as manual_escalation
		FROM resource_action_traces
		WHERE playbook_id = $1
			AND playbook_version = $2
			AND action_timestamp >= $3
	`

	var stats models.AIExecutionModeStats
	err := r.db.QueryRowContext(ctx, query, playbookID, playbookVersion, sinceTime).Scan(
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
	confidenceHigh           = "high"
	confidenceMedium         = "medium"
	confidenceLow            = "low"
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

