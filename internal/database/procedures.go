package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/errors"
	"github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

// ProcedureExecutor wraps stored procedure calls with consistent error handling
type ProcedureExecutor struct {
	db     *sql.DB
	logger *logrus.Logger
}

// NewProcedureExecutor creates a new procedure executor
func NewProcedureExecutor(db *sql.DB, logger *logrus.Logger) *ProcedureExecutor {
	return &ProcedureExecutor{
		db:     db,
		logger: logger,
	}
}

// ScaleOscillationResult represents the result from detect_scale_oscillation procedure
type ScaleOscillationResult struct {
	DirectionChanges  int                                 `json:"direction_changes"`
	FirstChange      time.Time                           `json:"first_change"`
	LastChange       time.Time                           `json:"last_change"`
	AvgEffectiveness float64                             `json:"avg_effectiveness"`
	DurationMinutes  float64                             `json:"duration_minutes"`
	Severity         actionhistory.OscillationSeverity   `json:"severity"`
	ActionSequence   json.RawMessage                     `json:"action_sequence"`
}

// ResourceThrashingResult represents the result from detect_resource_thrashing procedure
type ResourceThrashingResult struct {
	ThrashingTransitions int                                 `json:"thrashing_transitions"`
	TotalActions        int                                 `json:"total_actions"`
	FirstAction         time.Time                           `json:"first_action"`
	LastAction          time.Time                           `json:"last_action"`
	AvgEffectiveness    float64                             `json:"avg_effectiveness"`
	AvgTimeGapMinutes   float64                             `json:"avg_time_gap_minutes"`
	Severity           actionhistory.OscillationSeverity    `json:"severity"`
}

// IneffectiveLoopResult represents the result from detect_ineffective_loops procedure
type IneffectiveLoopResult struct {
	ActionType           string                             `json:"action_type"`
	RepetitionCount     int                                `json:"repetition_count"`
	AvgEffectiveness    float64                            `json:"avg_effectiveness"`
	EffectivenessStddev float64                            `json:"effectiveness_stddev"`
	FirstOccurrence     time.Time                          `json:"first_occurrence"`
	LastOccurrence      time.Time                          `json:"last_occurrence"`
	SpanMinutes         float64                            `json:"span_minutes"`
	Severity           actionhistory.OscillationSeverity   `json:"severity"`
	EffectivenessTrend float64                             `json:"effectiveness_trend"`
	EffectivenessScores pq.Float64Array                    `json:"effectiveness_scores"`
	Timestamps         pq.StringArray                      `json:"timestamps"`
}

// CascadingFailureResult represents the result from detect_cascading_failures procedure
type CascadingFailureResult struct {
	ActionType            string                             `json:"action_type"`
	TotalActions         int                                `json:"total_actions"`
	AvgNewAlerts         float64                            `json:"avg_new_alerts"`
	RecurrenceRate       float64                            `json:"recurrence_rate"`
	AvgEffectiveness     float64                            `json:"avg_effectiveness"`
	ActionsCausingCascades int                              `json:"actions_causing_cascades"`
	MaxAlertsTriggered   int                                `json:"max_alerts_triggered"`
	Severity            actionhistory.OscillationSeverity   `json:"severity"`
}

// ActionTrace represents a simplified action trace from procedures
type ActionTrace struct {
	ActionID         string          `json:"action_id"`
	ActionTimestamp  time.Time       `json:"action_timestamp"`
	ActionType       string          `json:"action_type"`
	ModelUsed        string          `json:"model_used"`
	ModelConfidence  float64         `json:"model_confidence"`
	ExecutionStatus  string          `json:"execution_status"`
	EffectivenessScore *float64       `json:"effectiveness_score"`
	ModelReasoning   *string         `json:"model_reasoning"`
	ActionParameters json.RawMessage `json:"action_parameters"`
	AlertName        string          `json:"alert_name"`
	AlertSeverity    string          `json:"alert_severity"`
}

// EffectivenessMetrics represents effectiveness analysis results
type EffectivenessMetrics struct {
	ActionType        string  `json:"action_type"`
	SampleSize        int     `json:"sample_size"`
	AvgEffectiveness  float64 `json:"avg_effectiveness"`
	StddevEffectiveness float64 `json:"stddev_effectiveness"`
	MinEffectiveness  float64 `json:"min_effectiveness"`
	MaxEffectiveness  float64 `json:"max_effectiveness"`
	SuccessRate      float64 `json:"success_rate"`
}

// DetectScaleOscillation calls the stored procedure for scale oscillation detection
func (pe *ProcedureExecutor) DetectScaleOscillation(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) (*ScaleOscillationResult, error) {
	pe.logger.WithFields(logrus.Fields{
		"namespace": resourceRef.Namespace,
		"kind":      resourceRef.Kind,
		"name":      resourceRef.Name,
		"window":    windowMinutes,
	}).Debug("Executing scale oscillation detection procedure")

	var result ScaleOscillationResult
	var severityStr string

	err := pe.db.QueryRowContext(ctx,
		"SELECT * FROM detect_scale_oscillation($1, $2, $3, $4)",
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, windowMinutes,
	).Scan(
		&result.DirectionChanges,
		&result.FirstChange,
		&result.LastChange,
		&result.AvgEffectiveness,
		&result.DurationMinutes,
		&severityStr,
		&result.ActionSequence,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No oscillation detected
		}
		return nil, errors.Wrapf(err, errors.ErrorTypeDatabase,
			"scale oscillation detection failed for %s/%s/%s", 
			resourceRef.Namespace, resourceRef.Kind, resourceRef.Name)
	}

	result.Severity = actionhistory.OscillationSeverity(severityStr)
	return &result, nil
}

// DetectResourceThrashing calls the stored procedure for resource thrashing detection
func (pe *ProcedureExecutor) DetectResourceThrashing(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) (*ResourceThrashingResult, error) {
	pe.logger.WithFields(logrus.Fields{
		"namespace": resourceRef.Namespace,
		"kind":      resourceRef.Kind,
		"name":      resourceRef.Name,
		"window":    windowMinutes,
	}).Debug("Executing resource thrashing detection procedure")

	var result ResourceThrashingResult
	var severityStr string

	err := pe.db.QueryRowContext(ctx,
		"SELECT * FROM detect_resource_thrashing($1, $2, $3, $4)",
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, windowMinutes,
	).Scan(
		&result.ThrashingTransitions,
		&result.TotalActions,
		&result.FirstAction,
		&result.LastAction,
		&result.AvgEffectiveness,
		&result.AvgTimeGapMinutes,
		&severityStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No thrashing detected
		}
		return nil, errors.Wrapf(err, errors.ErrorTypeDatabase,
			"resource thrashing detection failed for %s/%s/%s",
			resourceRef.Namespace, resourceRef.Kind, resourceRef.Name)
	}

	result.Severity = actionhistory.OscillationSeverity(severityStr)
	return &result, nil
}

// DetectIneffectiveLoops calls the stored procedure for ineffective loop detection
func (pe *ProcedureExecutor) DetectIneffectiveLoops(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) ([]IneffectiveLoopResult, error) {
	pe.logger.WithFields(logrus.Fields{
		"namespace": resourceRef.Namespace,
		"kind":      resourceRef.Kind,
		"name":      resourceRef.Name,
		"window":    windowMinutes,
	}).Debug("Executing ineffective loop detection procedure")

	rows, err := pe.db.QueryContext(ctx,
		"SELECT * FROM detect_ineffective_loops($1, $2, $3, $4)",
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, windowMinutes,
	)
	if err != nil {
		return nil, errors.Wrapf(err, errors.ErrorTypeDatabase,
			"ineffective loop detection failed for %s/%s/%s",
			resourceRef.Namespace, resourceRef.Kind, resourceRef.Name)
	}
	defer rows.Close()

	var results []IneffectiveLoopResult
	for rows.Next() {
		var result IneffectiveLoopResult
		var severityStr string

		err := rows.Scan(
			&result.ActionType,
			&result.RepetitionCount,
			&result.AvgEffectiveness,
			&result.EffectivenessStddev,
			&result.FirstOccurrence,
			&result.LastOccurrence,
			&result.SpanMinutes,
			&severityStr,
			&result.EffectivenessTrend,
			&result.EffectivenessScores,
			&result.Timestamps,
		)
		if err != nil {
			return nil, errors.Wrapf(err, errors.ErrorTypeDatabase,
				"failed to scan ineffective loop result")
		}

		result.Severity = actionhistory.OscillationSeverity(severityStr)
		results = append(results, result)
	}

	return results, nil
}

// DetectCascadingFailures calls the stored procedure for cascading failure detection
func (pe *ProcedureExecutor) DetectCascadingFailures(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) ([]CascadingFailureResult, error) {
	pe.logger.WithFields(logrus.Fields{
		"namespace": resourceRef.Namespace,
		"kind":      resourceRef.Kind,
		"name":      resourceRef.Name,
		"window":    windowMinutes,
	}).Debug("Executing cascading failure detection procedure")

	rows, err := pe.db.QueryContext(ctx,
		"SELECT * FROM detect_cascading_failures($1, $2, $3, $4)",
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, windowMinutes,
	)
	if err != nil {
		return nil, errors.Wrapf(err, errors.ErrorTypeDatabase,
			"cascading failure detection failed for %s/%s/%s",
			resourceRef.Namespace, resourceRef.Kind, resourceRef.Name)
	}
	defer rows.Close()

	var results []CascadingFailureResult
	for rows.Next() {
		var result CascadingFailureResult
		var severityStr string

		err := rows.Scan(
			&result.ActionType,
			&result.TotalActions,
			&result.AvgNewAlerts,
			&result.RecurrenceRate,
			&result.AvgEffectiveness,
			&result.ActionsCausingCascades,
			&result.MaxAlertsTriggered,
			&severityStr,
		)
		if err != nil {
			return nil, errors.Wrapf(err, errors.ErrorTypeDatabase,
				"failed to scan cascading failure result")
		}

		result.Severity = actionhistory.OscillationSeverity(severityStr)
		results = append(results, result)
	}

	return results, nil
}

// GetActionTraces calls the stored procedure to retrieve action traces
func (pe *ProcedureExecutor) GetActionTraces(ctx context.Context, resourceRef actionhistory.ResourceReference, actionType, modelUsed *string, timeStart, timeEnd *time.Time, limit, offset int) ([]ActionTrace, error) {
	pe.logger.WithFields(logrus.Fields{
		"namespace": resourceRef.Namespace,
		"kind":      resourceRef.Kind,
		"name":      resourceRef.Name,
		"limit":     limit,
		"offset":    offset,
	}).Debug("Executing get action traces procedure")

	rows, err := pe.db.QueryContext(ctx,
		"SELECT * FROM get_action_traces($1, $2, $3, $4, $5, $6, $7, $8, $9)",
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name,
		actionType, modelUsed, timeStart, timeEnd, limit, offset,
	)
	if err != nil {
		return nil, errors.Wrapf(err, errors.ErrorTypeDatabase,
			"get action traces failed for %s/%s/%s",
			resourceRef.Namespace, resourceRef.Kind, resourceRef.Name)
	}
	defer rows.Close()

	var traces []ActionTrace
	for rows.Next() {
		var trace ActionTrace

		err := rows.Scan(
			&trace.ActionID,
			&trace.ActionTimestamp,
			&trace.ActionType,
			&trace.ModelUsed,
			&trace.ModelConfidence,
			&trace.ExecutionStatus,
			&trace.EffectivenessScore,
			&trace.ModelReasoning,
			&trace.ActionParameters,
			&trace.AlertName,
			&trace.AlertSeverity,
		)
		if err != nil {
			return nil, errors.Wrapf(err, errors.ErrorTypeDatabase,
				"failed to scan action trace")
		}

		traces = append(traces, trace)
	}

	return traces, nil
}

// GetActionEffectiveness calls the stored procedure to calculate effectiveness metrics
func (pe *ProcedureExecutor) GetActionEffectiveness(ctx context.Context, resourceRef actionhistory.ResourceReference, actionType *string, timeStart, timeEnd time.Time) ([]EffectivenessMetrics, error) {
	pe.logger.WithFields(logrus.Fields{
		"namespace":  resourceRef.Namespace,
		"kind":       resourceRef.Kind,
		"name":       resourceRef.Name,
		"time_start": timeStart,
		"time_end":   timeEnd,
	}).Debug("Executing get action effectiveness procedure")

	rows, err := pe.db.QueryContext(ctx,
		"SELECT * FROM get_action_effectiveness($1, $2, $3, $4, $5, $6)",
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name,
		actionType, timeStart, timeEnd,
	)
	if err != nil {
		return nil, errors.Wrapf(err, errors.ErrorTypeDatabase,
			"get action effectiveness failed for %s/%s/%s",
			resourceRef.Namespace, resourceRef.Kind, resourceRef.Name)
	}
	defer rows.Close()

	var metrics []EffectivenessMetrics
	for rows.Next() {
		var metric EffectivenessMetrics

		err := rows.Scan(
			&metric.ActionType,
			&metric.SampleSize,
			&metric.AvgEffectiveness,
			&metric.StddevEffectiveness,
			&metric.MinEffectiveness,
			&metric.MaxEffectiveness,
			&metric.SuccessRate,
		)
		if err != nil {
			return nil, errors.Wrapf(err, errors.ErrorTypeDatabase,
				"failed to scan effectiveness metrics")
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// StoreOscillationDetection calls the stored procedure to store detection results
func (pe *ProcedureExecutor) StoreOscillationDetection(ctx context.Context, patternID int, resourceRef actionhistory.ResourceReference, confidence float64, actionCount, timeSpanMinutes int, evidence map[string]interface{}, preventionAction *string) (int, error) {
	pe.logger.WithFields(logrus.Fields{
		"namespace":  resourceRef.Namespace,
		"kind":       resourceRef.Kind,
		"name":       resourceRef.Name,
		"pattern_id": patternID,
	}).Debug("Executing store oscillation detection procedure")

	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return 0, errors.Wrapf(err, errors.ErrorTypeInternal,
			"failed to marshal detection evidence")
	}

	var detectionID int
	err = pe.db.QueryRowContext(ctx,
		"SELECT store_oscillation_detection($1, $2, $3, $4, $5, $6, $7, $8, $9)",
		patternID, resourceRef.Namespace, resourceRef.Kind, resourceRef.Name,
		confidence, actionCount, timeSpanMinutes, evidenceJSON, preventionAction,
	).Scan(&detectionID)

	if err != nil {
		return 0, errors.Wrapf(err, errors.ErrorTypeDatabase,
			"store oscillation detection failed for %s/%s/%s",
			resourceRef.Namespace, resourceRef.Kind, resourceRef.Name)
	}

	return detectionID, nil
}