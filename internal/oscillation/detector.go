package oscillation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

// ScaleOscillationResult represents the result of scale oscillation detection
type ScaleOscillationResult struct {
	DirectionChanges int                               `json:"direction_changes"`
	FirstChange      time.Time                         `json:"first_change"`
	LastChange       time.Time                         `json:"last_change"`
	AvgEffectiveness float64                           `json:"avg_effectiveness"`
	DurationMinutes  float64                           `json:"duration_minutes"`
	Severity         actionhistory.OscillationSeverity `json:"severity"`
	ActionSequence   []ScaleActionDetail               `json:"action_sequence"`
}

type ScaleActionDetail struct {
	Timestamp     time.Time `json:"timestamp"`
	ReplicaCount  int       `json:"replica_count"`
	Direction     string    `json:"direction"` // up, down, none
	Effectiveness float64   `json:"effectiveness"`
}

// ResourceThrashingResult represents the result of resource thrashing detection
type ResourceThrashingResult struct {
	ThrashingTransitions int                               `json:"thrashing_transitions"`
	TotalActions         int                               `json:"total_actions"`
	FirstAction          time.Time                         `json:"first_action"`
	LastAction           time.Time                         `json:"last_action"`
	AvgEffectiveness     float64                           `json:"avg_effectiveness"`
	AvgTimeGapMinutes    float64                           `json:"avg_time_gap_minutes"`
	Severity             actionhistory.OscillationSeverity `json:"severity"`
	ActionPattern        []ResourceActionDetail            `json:"action_pattern"`
}

type ResourceActionDetail struct {
	Timestamp      time.Time              `json:"timestamp"`
	ActionType     string                 `json:"action_type"`
	Parameters     map[string]interface{} `json:"parameters"`
	Effectiveness  float64                `json:"effectiveness"`
	TimeGapMinutes float64                `json:"time_gap_minutes"`
}

// IneffectiveLoopResult represents the result of ineffective loop detection
type IneffectiveLoopResult struct {
	ActionType          string                            `json:"action_type"`
	RepetitionCount     int                               `json:"repetition_count"`
	AvgEffectiveness    float64                           `json:"avg_effectiveness"`
	EffectivenessStddev float64                           `json:"effectiveness_stddev"`
	FirstOccurrence     time.Time                         `json:"first_occurrence"`
	LastOccurrence      time.Time                         `json:"last_occurrence"`
	SpanMinutes         float64                           `json:"span_minutes"`
	Severity            actionhistory.OscillationSeverity `json:"severity"`
	EffectivenessTrend  float64                           `json:"effectiveness_trend"`
	EffectivenessScores []float64                         `json:"effectiveness_scores"`
	Timestamps          []time.Time                       `json:"timestamps"`
}

// CascadingFailureResult represents the result of cascading failure detection
type CascadingFailureResult struct {
	ActionType             string                            `json:"action_type"`
	TotalActions           int                               `json:"total_actions"`
	AvgNewAlerts           float64                           `json:"avg_new_alerts"`
	RecurrenceRate         float64                           `json:"recurrence_rate"`
	AvgEffectiveness       float64                           `json:"avg_effectiveness"`
	ActionsCausingCascades int                               `json:"actions_causing_cascades"`
	MaxAlertsTriggered     int                               `json:"max_alerts_triggered"`
	Severity               actionhistory.OscillationSeverity `json:"severity"`
}

// OscillationAnalysisResult represents the complete analysis result
type OscillationAnalysisResult struct {
	ResourceReference actionhistory.ResourceReference `json:"resource_reference"`
	AnalysisTimestamp time.Time                       `json:"analysis_timestamp"`
	WindowMinutes     int                             `json:"window_minutes"`

	// Detection results
	ScaleOscillation  *ScaleOscillationResult  `json:"scale_oscillation,omitempty"`
	ResourceThrashing *ResourceThrashingResult `json:"resource_thrashing,omitempty"`
	IneffectiveLoops  []IneffectiveLoopResult  `json:"ineffective_loops,omitempty"`
	CascadingFailures []CascadingFailureResult `json:"cascading_failures,omitempty"`

	// Overall assessment
	OverallSeverity   actionhistory.OscillationSeverity `json:"overall_severity"`
	RecommendedAction actionhistory.PreventionAction    `json:"recommended_action"`
	Confidence        float64                           `json:"confidence"`

	// Prevention history
	PreviousPrevention *PreventionRecord `json:"previous_prevention,omitempty"`
}

type PreventionRecord struct {
	Timestamp     time.Time                      `json:"timestamp"`
	Action        actionhistory.PreventionAction `json:"action"`
	Reason        string                         `json:"reason"`
	EffectiveTime time.Duration                  `json:"effective_time"`
	Successful    bool                           `json:"successful"`
}

// ScaleOscillationDetector detects scale oscillation patterns
type ScaleOscillationDetector struct {
	db     *sql.DB
	logger *logrus.Logger
}

// DetectScaleOscillation detects scale oscillation patterns for a resource using stored procedure
func (d *ScaleOscillationDetector) DetectScaleOscillation(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) (*ScaleOscillationResult, error) {
	query := `SELECT * FROM detect_scale_oscillation($1, $2, $3, $4)`

	var result ScaleOscillationResult
	var actionSequenceJSON []byte
	var severityStr string

	err := d.db.QueryRowContext(ctx, query,
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, windowMinutes).Scan(
		&result.DirectionChanges,
		&result.FirstChange,
		&result.LastChange,
		&result.AvgEffectiveness,
		&result.DurationMinutes,
		&severityStr,
		&actionSequenceJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No oscillation detected
		}
		return nil, fmt.Errorf("failed to detect scale oscillation: %w", err)
	}

	// Parse action sequence JSON
	if err := json.Unmarshal(actionSequenceJSON, &result.ActionSequence); err != nil {
		return nil, fmt.Errorf("failed to parse action sequence: %w", err)
	}

	result.Severity = actionhistory.OscillationSeverity(severityStr)

	d.logger.WithFields(logrus.Fields{
		"resource":          resourceRef,
		"direction_changes": result.DirectionChanges,
		"severity":          result.Severity,
		"duration_minutes":  result.DurationMinutes,
	}).Info("Scale oscillation detected")

	return &result, nil
}

// ResourceThrashingDetector detects resource thrashing patterns
type ResourceThrashingDetector struct {
	db     *sql.DB
	logger *logrus.Logger
}

// DetectResourceThrashing detects resource thrashing patterns using stored procedure
func (d *ResourceThrashingDetector) DetectResourceThrashing(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) (*ResourceThrashingResult, error) {
	query := `SELECT * FROM detect_resource_thrashing($1, $2, $3, $4)`

	var result ResourceThrashingResult
	var severityStr string

	err := d.db.QueryRowContext(ctx, query,
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, windowMinutes).Scan(
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
		return nil, fmt.Errorf("failed to detect resource thrashing: %w", err)
	}

	result.Severity = actionhistory.OscillationSeverity(severityStr)

	// Get detailed action pattern if thrashing detected
	if result.ThrashingTransitions > 0 {
		patterns, err := d.getDetailedActionPattern(ctx, resourceRef, windowMinutes)
		if err != nil {
			d.logger.WithError(err).Warn("Failed to get detailed action pattern")
		} else {
			result.ActionPattern = patterns
		}
	}

	d.logger.WithFields(logrus.Fields{
		"resource":              resourceRef,
		"thrashing_transitions": result.ThrashingTransitions,
		"severity":              result.Severity,
		"avg_effectiveness":     result.AvgEffectiveness,
	}).Info("Resource thrashing detected")

	return &result, nil
}

func (d *ResourceThrashingDetector) getDetailedActionPattern(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) ([]ResourceActionDetail, error) {
	// Use the stored procedure for getting action traces with filtering
	query := `SELECT action_timestamp, action_type, action_parameters FROM get_action_traces($1, $2, $3, NULL, NULL, $4, $5, 100, 0)`
	timeStart := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)
	timeEnd := time.Now()

	rows, err := d.db.QueryContext(ctx, query,
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, timeStart, timeEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to query detailed action pattern: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			d.logger.WithError(err).Error("Failed to close database rows")
		}
	}()

	var patterns []ResourceActionDetail
	var prevTimestamp *time.Time
	for rows.Next() {
		var detail ResourceActionDetail
		var parametersJSON []byte
		var actionID, modelUsed, executionStatus, modelReasoning, alertName, alertSeverity string
		var modelConfidence, effectivenessScore sql.NullFloat64

		err := rows.Scan(
			&actionID,
			&detail.Timestamp,
			&detail.ActionType,
			&modelUsed,
			&modelConfidence,
			&executionStatus,
			&effectivenessScore,
			&modelReasoning,
			&parametersJSON,
			&alertName,
			&alertSeverity,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan action detail: %w", err)
		}

		// Filter to only include resource-related actions
		if detail.ActionType != "increase_resources" && detail.ActionType != "scale_deployment" {
			continue
		}

		if effectivenessScore.Valid {
			detail.Effectiveness = effectivenessScore.Float64
		}

		// Calculate time gap
		if prevTimestamp != nil {
			detail.TimeGapMinutes = detail.Timestamp.Sub(*prevTimestamp).Minutes()
		}
		prevTimestamp = &detail.Timestamp

		if err := json.Unmarshal(parametersJSON, &detail.Parameters); err != nil {
			d.logger.WithError(err).Warn("Failed to unmarshal action parameters")
			detail.Parameters = make(map[string]interface{})
		}

		patterns = append(patterns, detail)
	}

	return patterns, nil
}

// IneffectiveLoopDetector detects ineffective loop patterns
type IneffectiveLoopDetector struct {
	db     *sql.DB
	logger *logrus.Logger
}

// DetectIneffectiveLoops detects ineffective loop patterns using stored procedure
func (d *IneffectiveLoopDetector) DetectIneffectiveLoops(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) ([]IneffectiveLoopResult, error) {
	query := `SELECT * FROM detect_ineffective_loops($1, $2, $3, $4)`

	rows, err := d.db.QueryContext(ctx, query,
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, windowMinutes)
	if err != nil {
		return nil, fmt.Errorf("failed to detect ineffective loops: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			d.logger.WithError(err).Error("Failed to close database rows")
		}
	}()

	var results []IneffectiveLoopResult
	for rows.Next() {
		var result IneffectiveLoopResult
		var severityStr string
		var effectivenessScoresArray pq.Float64Array
		var timestampsArray pq.StringArray

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
			&effectivenessScoresArray,
			&timestampsArray,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ineffective loop result: %w", err)
		}

		result.Severity = actionhistory.OscillationSeverity(severityStr)
		result.EffectivenessScores = []float64(effectivenessScoresArray)

		// Convert pq.StringArray to []time.Time
		result.Timestamps = make([]time.Time, len(timestampsArray))
		for i, ts := range timestampsArray {
			if timestamp, err := time.Parse(time.RFC3339, ts); err == nil {
				result.Timestamps[i] = timestamp
			}
		}

		results = append(results, result)

		d.logger.WithFields(logrus.Fields{
			"resource":          resourceRef,
			"action_type":       result.ActionType,
			"repetition_count":  result.RepetitionCount,
			"avg_effectiveness": result.AvgEffectiveness,
			"severity":          result.Severity,
		}).Warn("Ineffective loop detected")
	}

	return results, nil
}

// CascadingFailureDetector detects cascading failure patterns
type CascadingFailureDetector struct {
	db     *sql.DB
	logger *logrus.Logger
}

// DetectCascadingFailures detects cascading failure patterns using stored procedure
func (d *CascadingFailureDetector) DetectCascadingFailures(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) ([]CascadingFailureResult, error) {
	query := `SELECT * FROM detect_cascading_failures($1, $2, $3, $4)`

	rows, err := d.db.QueryContext(ctx, query,
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name, windowMinutes)
	if err != nil {
		return nil, fmt.Errorf("failed to detect cascading failures: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			d.logger.WithError(err).Error("Failed to close database rows")
		}
	}()

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
			return nil, fmt.Errorf("failed to scan cascading failure result: %w", err)
		}

		result.Severity = actionhistory.OscillationSeverity(severityStr)
		results = append(results, result)

		d.logger.WithFields(logrus.Fields{
			"resource":                 resourceRef,
			"action_type":              result.ActionType,
			"avg_new_alerts":           result.AvgNewAlerts,
			"recurrence_rate":          result.RecurrenceRate,
			"actions_causing_cascades": result.ActionsCausingCascades,
			"severity":                 result.Severity,
		}).Warn("Cascading failure pattern detected")
	}

	return results, nil
}

// OscillationDetectionEngine is the main engine that coordinates all detectors
type OscillationDetectionEngine struct {
	scaleDetector     *ScaleOscillationDetector
	thrashingDetector *ResourceThrashingDetector
	loopDetector      *IneffectiveLoopDetector
	cascadeDetector   *CascadingFailureDetector
	db                *sql.DB
	logger            *logrus.Logger
}

// NewOscillationDetectionEngine creates a new oscillation detection engine
func NewOscillationDetectionEngine(db *sql.DB, logger *logrus.Logger) *OscillationDetectionEngine {
	return &OscillationDetectionEngine{
		scaleDetector:     &ScaleOscillationDetector{db: db, logger: logger},
		thrashingDetector: &ResourceThrashingDetector{db: db, logger: logger},
		loopDetector:      &IneffectiveLoopDetector{db: db, logger: logger},
		cascadeDetector:   &CascadingFailureDetector{db: db, logger: logger},
		db:                db,
		logger:            logger,
	}
}

// AnalyzeResource performs comprehensive oscillation analysis for a resource
func (e *OscillationDetectionEngine) AnalyzeResource(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) (*OscillationAnalysisResult, error) {
	result := &OscillationAnalysisResult{
		ResourceReference: resourceRef,
		AnalysisTimestamp: time.Now(),
		WindowMinutes:     windowMinutes,
		OverallSeverity:   actionhistory.SeverityNone,
	}

	// Run all detectors in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	var detectionErrors []error

	// Scale oscillation detection
	wg.Add(1)
	go func() {
		defer wg.Done()
		scaleResult, err := e.scaleDetector.DetectScaleOscillation(ctx, resourceRef, windowMinutes)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			detectionErrors = append(detectionErrors, fmt.Errorf("scale detection failed: %w", err))
		} else if scaleResult != nil {
			result.ScaleOscillation = scaleResult
			result.OverallSeverity = maxSeverity(result.OverallSeverity, scaleResult.Severity)
		}
	}()

	// Resource thrashing detection
	wg.Add(1)
	go func() {
		defer wg.Done()
		thrashingResult, err := e.thrashingDetector.DetectResourceThrashing(ctx, resourceRef, windowMinutes)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			detectionErrors = append(detectionErrors, fmt.Errorf("thrashing detection failed: %w", err))
		} else if thrashingResult != nil {
			result.ResourceThrashing = thrashingResult
			result.OverallSeverity = maxSeverity(result.OverallSeverity, thrashingResult.Severity)
		}
	}()

	// Ineffective loop detection
	wg.Add(1)
	go func() {
		defer wg.Done()
		loopResults, err := e.loopDetector.DetectIneffectiveLoops(ctx, resourceRef, windowMinutes)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			detectionErrors = append(detectionErrors, fmt.Errorf("loop detection failed: %w", err))
		} else if len(loopResults) > 0 {
			result.IneffectiveLoops = loopResults
			for _, loop := range loopResults {
				result.OverallSeverity = maxSeverity(result.OverallSeverity, loop.Severity)
			}
		}
	}()

	// Cascading failure detection
	wg.Add(1)
	go func() {
		defer wg.Done()
		cascadeResults, err := e.cascadeDetector.DetectCascadingFailures(ctx, resourceRef, windowMinutes)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			detectionErrors = append(detectionErrors, fmt.Errorf("cascade detection failed: %w", err))
		} else if len(cascadeResults) > 0 {
			result.CascadingFailures = cascadeResults
			for _, cascade := range cascadeResults {
				result.OverallSeverity = maxSeverity(result.OverallSeverity, cascade.Severity)
			}
		}
	}()

	wg.Wait()

	// Handle detection errors
	if len(detectionErrors) > 0 {
		e.logger.WithField("errors", detectionErrors).Error("Some oscillation detectors failed")
		// Continue with partial results
	}

	// Determine recommended prevention action
	result.RecommendedAction, result.Confidence = e.determinePreventionAction(result)

	// Check for previous prevention attempts
	prevPrevention, err := e.getPreviousPrevention(ctx, resourceRef)
	if err != nil {
		e.logger.WithError(err).Warn("Failed to get previous prevention record")
	} else {
		result.PreviousPrevention = prevPrevention
	}

	// Store detection result in database
	if err := e.storeDetectionResult(ctx, result); err != nil {
		e.logger.WithError(err).Error("Failed to store detection result")
	}

	return result, nil
}

func (e *OscillationDetectionEngine) determinePreventionAction(result *OscillationAnalysisResult) (actionhistory.PreventionAction, float64) {
	// Calculate confidence based on multiple indicators
	confidence := 0.0
	detectionCount := 0

	if result.ScaleOscillation != nil {
		detectionCount++
		confidence += float64(severityToScore(result.ScaleOscillation.Severity))
	}

	if result.ResourceThrashing != nil {
		detectionCount++
		confidence += float64(severityToScore(result.ResourceThrashing.Severity))
	}

	if len(result.IneffectiveLoops) > 0 {
		detectionCount++
		for _, loop := range result.IneffectiveLoops {
			confidence += float64(severityToScore(loop.Severity))
		}
	}

	if len(result.CascadingFailures) > 0 {
		detectionCount++
		for _, cascade := range result.CascadingFailures {
			confidence += float64(severityToScore(cascade.Severity))
		}
	}

	if detectionCount > 0 {
		confidence = confidence / float64(detectionCount) / 4.0 // Normalize to 0-1
	}

	// Determine action based on severity and confidence
	switch result.OverallSeverity {
	case actionhistory.SeverityCritical:
		if confidence > 0.8 {
			return actionhistory.PreventionBlock, confidence
		}
		return actionhistory.PreventionEscalate, confidence

	case actionhistory.SeverityHigh:
		if confidence > 0.7 {
			return actionhistory.PreventionCoolingPeriod, confidence
		}
		return actionhistory.PreventionAlternative, confidence

	case actionhistory.SeverityMedium:
		return actionhistory.PreventionAlternative, confidence

	case actionhistory.SeverityLow:
		return actionhistory.PreventionNone, confidence

	default:
		return actionhistory.PreventionNone, 0.0
	}
}

func severityToScore(severity actionhistory.OscillationSeverity) int {
	switch severity {
	case actionhistory.SeverityCritical:
		return 4
	case actionhistory.SeverityHigh:
		return 3
	case actionhistory.SeverityMedium:
		return 2
	case actionhistory.SeverityLow:
		return 1
	default:
		return 0
	}
}

func maxSeverity(current, new actionhistory.OscillationSeverity) actionhistory.OscillationSeverity {
	currentScore := severityToScore(current)
	newScore := severityToScore(new)

	if newScore > currentScore {
		return new
	}
	return current
}

func (e *OscillationDetectionEngine) storeDetectionResult(ctx context.Context, result *OscillationAnalysisResult) error {
	// Use stored procedure to store oscillation detection result
	query := `SELECT store_oscillation_detection($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	// Convert result to JSON for evidence storage
	evidenceJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal evidence: %w", err)
	}

	actionCount := 0
	if result.ScaleOscillation != nil {
		actionCount += result.ScaleOscillation.DirectionChanges
	}
	if result.ResourceThrashing != nil {
		actionCount += result.ResourceThrashing.ThrashingTransitions
	}

	var preventionAction *string
	if result.RecommendedAction != actionhistory.PreventionNone {
		action := string(result.RecommendedAction)
		preventionAction = &action
	}

	var detectionID int
	err = e.db.QueryRowContext(ctx, query,
		1, // Default pattern ID - in production, this would be dynamic
		result.ResourceReference.Namespace,
		result.ResourceReference.Kind,
		result.ResourceReference.Name,
		result.Confidence,
		actionCount,
		result.WindowMinutes,
		evidenceJSON,
		preventionAction,
	).Scan(&detectionID)

	if err != nil {
		return fmt.Errorf("failed to store detection result: %w", err)
	}

	e.logger.WithFields(logrus.Fields{
		"detection_id": detectionID,
		"resource":     result.ResourceReference,
		"confidence":   result.Confidence,
	}).Debug("Stored oscillation detection result")

	return nil
}

func (e *OscillationDetectionEngine) getPreviousPrevention(ctx context.Context, resourceRef actionhistory.ResourceReference) (*PreventionRecord, error) {
	// Simplified query - stored procedure handles resource lookups internally
	query := `
        SELECT od.detected_at, od.prevention_action, od.prevention_successful
        FROM oscillation_detections od
        JOIN resource_references rr ON od.resource_id = rr.id
        WHERE rr.namespace = $1 AND rr.kind = $2 AND rr.name = $3
        AND od.prevention_applied = true
        ORDER BY od.detected_at DESC
        LIMIT 1`

	var record PreventionRecord
	var actionStr string

	err := e.db.QueryRowContext(ctx, query,
		resourceRef.Namespace, resourceRef.Kind, resourceRef.Name).Scan(
		&record.Timestamp,
		&actionStr,
		&record.Successful,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No previous prevention
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query previous prevention: %w", err)
	}

	record.Action = actionhistory.PreventionAction(actionStr)
	record.EffectiveTime = time.Since(record.Timestamp)

	return &record, nil
}

// GenerateSafetyReasoning generates human-readable reasoning for safety decisions
func GenerateSafetyReasoning(analysis *OscillationAnalysisResult) string {
	if analysis.OverallSeverity == actionhistory.SeverityNone {
		return "No oscillation patterns detected. Action appears safe to proceed."
	}

	var reasons []string

	if analysis.ScaleOscillation != nil {
		reasons = append(reasons, fmt.Sprintf(
			"Scale oscillation detected: %d direction changes with %.1f%% effectiveness",
			analysis.ScaleOscillation.DirectionChanges,
			analysis.ScaleOscillation.AvgEffectiveness*100,
		))
	}

	if analysis.ResourceThrashing != nil {
		reasons = append(reasons, fmt.Sprintf(
			"Resource thrashing detected: %d transitions between resource/scale actions",
			analysis.ResourceThrashing.ThrashingTransitions,
		))
	}

	if len(analysis.IneffectiveLoops) > 0 {
		for _, loop := range analysis.IneffectiveLoops {
			reasons = append(reasons, fmt.Sprintf(
				"Ineffective loop: %s action repeated %d times with %.1f%% effectiveness",
				loop.ActionType, loop.RepetitionCount, loop.AvgEffectiveness*100,
			))
		}
	}

	if len(analysis.CascadingFailures) > 0 {
		for _, cascade := range analysis.CascadingFailures {
			reasons = append(reasons, fmt.Sprintf(
				"Cascading failure risk: %s actions trigger %.1f new alerts on average",
				cascade.ActionType, cascade.AvgNewAlerts,
			))
		}
	}

	return fmt.Sprintf("CAUTION: %s. Recommended action: %s",
		strings.Join(reasons, "; "), analysis.RecommendedAction)
}
