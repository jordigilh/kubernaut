package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/sirupsen/logrus"
)

// DetectorBase provides common functionality for all oscillation detectors
type DetectorBase struct {
	db     *sql.DB
	logger *logrus.Logger
}

// NewDetectorBase creates a new detector base
func NewDetectorBase(db *sql.DB, logger *logrus.Logger) *DetectorBase {
	return &DetectorBase{
		db:     db,
		logger: logger,
	}
}

// DB returns the database connection
func (d *DetectorBase) DB() *sql.DB {
	return d.db
}

// Logger returns the logger instance
func (d *DetectorBase) Logger() *logrus.Logger {
	return d.logger
}

// Removed QueryBuilder - now using stored procedures for all database operations

// QueryResourceActions executes the base resource actions stored procedure
func (d *DetectorBase) QueryResourceActions(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes *int) (*sql.Rows, error) {
	query := `SELECT * FROM get_resource_actions_base($1, $2, $3, $4)`

	rows, err := d.db.QueryContext(ctx, query,
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, windowMinutes)
	if err != nil {
		d.logger.WithError(err).WithFields(logrus.Fields{
			"namespace": resourceRef.Namespace,
			"kind":      resourceRef.Kind,
			"name":      resourceRef.Name,
		}).Error("Failed to execute resource action query")
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	return rows, nil
}

// GetResourceEffectiveness gets action effectiveness metrics using stored procedure
func (d *DetectorBase) GetResourceEffectiveness(ctx context.Context, resourceRef actionhistory.ResourceReference, actionType *string, timeStart, timeEnd *time.Time) (*sql.Rows, error) {
	query := `SELECT * FROM get_action_effectiveness($1, $2, $3, $4, $5, $6)`

	rows, err := d.db.QueryContext(ctx, query,
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, actionType, timeStart, timeEnd)
	if err != nil {
		d.logger.WithError(err).WithFields(logrus.Fields{
			"namespace": resourceRef.Namespace,
			"kind":      resourceRef.Kind,
			"name":      resourceRef.Name,
		}).Error("Failed to get resource effectiveness")
		return nil, fmt.Errorf("failed to get effectiveness metrics: %w", err)
	}

	return rows, nil
}

// GetResourceID retrieves the database ID for a resource reference using stored procedure
func (d *DetectorBase) GetResourceID(ctx context.Context, resourceRef actionhistory.ResourceReference) (int64, error) {
	var resourceID int64
	query := `SELECT get_resource_id($1, $2, $3)`

	err := d.db.QueryRowContext(ctx, query,
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name).Scan(&resourceID)

	if err != nil {
		d.logger.WithError(err).WithField("resource", resourceRef).Error("Failed to get resource ID")
		return 0, fmt.Errorf("failed to get resource ID: %w", err)
	}

	return resourceID, nil
}

// GetOscillationAnalysis retrieves oscillation analysis using stored procedure
func (d *DetectorBase) GetOscillationAnalysis(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) (*sql.Rows, error) {
	query := `SELECT * FROM analyze_action_oscillation($1, $2, $3, $4)`

	rows, err := d.db.QueryContext(ctx, query,
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, windowMinutes)
	if err != nil {
		d.logger.WithError(err).WithFields(logrus.Fields{
			"namespace": resourceRef.Namespace,
			"kind":      resourceRef.Kind,
			"name":      resourceRef.Name,
		}).Error("Failed to get oscillation analysis")
		return nil, fmt.Errorf("failed to get oscillation analysis: %w", err)
	}

	return rows, nil
}

// ActionEffectivenessResult represents effectiveness metrics for an action type
type ActionEffectivenessResult struct {
	ActionType          string  `json:"action_type"`
	SampleSize          int     `json:"sample_size"`
	AvgEffectiveness    float64 `json:"avg_effectiveness"`
	StddevEffectiveness float64 `json:"stddev_effectiveness"`
	MinEffectiveness    float64 `json:"min_effectiveness"`
	MaxEffectiveness    float64 `json:"max_effectiveness"`
	SuccessRate         float64 `json:"success_rate"`
}

// OscillationAnalysisPoint represents a single point in oscillation analysis
type OscillationAnalysisPoint struct {
	ActionTimestamp        time.Time  `json:"action_timestamp"`
	ActionType             string     `json:"action_type"`
	EffectivenessScore     float64    `json:"effectiveness_score"`
	PrevTimestamp          *time.Time `json:"prev_timestamp,omitempty"`
	PrevActionType         *string    `json:"prev_action_type,omitempty"`
	TimeGapMinutes         float64    `json:"time_gap_minutes"`
	ActionSequencePosition int        `json:"action_sequence_position"`
}
