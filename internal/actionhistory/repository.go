package actionhistory

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

// Repository interface defines the contract for action history storage
type Repository interface {
	// Resource management
	EnsureResourceReference(ctx context.Context, ref ResourceReference) (int64, error)
	GetResourceReference(ctx context.Context, namespace, kind, name string) (*ResourceReference, error)

	// Action history management
	EnsureActionHistory(ctx context.Context, resourceID int64) (*ActionHistory, error)
	GetActionHistory(ctx context.Context, resourceID int64) (*ActionHistory, error)
	UpdateActionHistory(ctx context.Context, history *ActionHistory) error

	// Action traces
	StoreAction(ctx context.Context, action *ActionRecord) (*ResourceActionTrace, error)
	GetActionTraces(ctx context.Context, query ActionQuery) ([]ResourceActionTrace, error)
	GetActionTrace(ctx context.Context, actionID string) (*ResourceActionTrace, error)
	UpdateActionTrace(ctx context.Context, trace *ResourceActionTrace) error
	GetPendingEffectivenessAssessments(ctx context.Context) ([]*ResourceActionTrace, error)

	// Oscillation patterns
	GetOscillationPatterns(ctx context.Context, patternType string) ([]OscillationPattern, error)
	StoreOscillationDetection(ctx context.Context, detection *OscillationDetection) error
	GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]OscillationDetection, error)

	// Cleanup and maintenance
	ApplyRetention(ctx context.Context, actionHistoryID int64) error
	GetActionHistorySummaries(ctx context.Context, since time.Duration) ([]ActionHistorySummary, error)
}

// PostgreSQLRepository implements Repository using PostgreSQL
type PostgreSQLRepository struct {
	db     *sql.DB
	logger *logrus.Logger
}

// NewPostgreSQLRepository creates a new PostgreSQL repository
func NewPostgreSQLRepository(db *sql.DB, logger *logrus.Logger) *PostgreSQLRepository {
	return &PostgreSQLRepository{
		db:     db,
		logger: logger,
	}
}

// EnsureResourceReference ensures a resource reference exists and returns its ID
func (r *PostgreSQLRepository) EnsureResourceReference(ctx context.Context, ref ResourceReference) (int64, error) {
	// Try to get existing resource
	var resourceID int64
	query := `
		SELECT id FROM resource_references
		WHERE namespace = $1 AND kind = $2 AND name = $3
		LIMIT 1`

	err := r.db.QueryRowContext(ctx, query, ref.Namespace, ref.Kind, ref.Name).Scan(&resourceID)
	if err == nil {
		// Update last_seen
		_, err = r.db.ExecContext(ctx, `
			UPDATE resource_references
			SET last_seen = NOW(), deleted_at = NULL
			WHERE id = $1`, resourceID)
		return resourceID, err
	}

	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to query resource reference: %w", err)
	}

	// Create new resource reference
	insertQuery := `
		INSERT INTO resource_references (
			resource_uid, api_version, kind, name, namespace, last_seen
		) VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id`

	err = r.db.QueryRowContext(ctx, insertQuery,
		ref.ResourceUID, ref.APIVersion, ref.Kind, ref.Name, ref.Namespace).Scan(&resourceID)

	if err != nil {
		return 0, fmt.Errorf("failed to create resource reference: %w", err)
	}

	r.logger.WithFields(logrus.Fields{
		"resource_id": resourceID,
		"namespace":   ref.Namespace,
		"kind":        ref.Kind,
		"name":        ref.Name,
	}).Debug("Created resource reference")

	return resourceID, nil
}

// GetResourceReference retrieves a resource reference by namespace, kind, and name
func (r *PostgreSQLRepository) GetResourceReference(ctx context.Context, namespace, kind, name string) (*ResourceReference, error) {
	query := `
		SELECT id, resource_uid, api_version, kind, name, namespace,
		       created_at, deleted_at, last_seen
		FROM resource_references
		WHERE namespace = $1 AND kind = $2 AND name = $3
		LIMIT 1`

	var ref ResourceReference
	err := r.db.QueryRowContext(ctx, query, namespace, kind, name).Scan(
		&ref.ID, &ref.ResourceUID, &ref.APIVersion, &ref.Kind, &ref.Name, &ref.Namespace,
		&ref.CreatedAt, &ref.DeletedAt, &ref.LastSeen,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get resource reference: %w", err)
	}

	return &ref, nil
}

// EnsureActionHistory ensures an action history exists for a resource
func (r *PostgreSQLRepository) EnsureActionHistory(ctx context.Context, resourceID int64) (*ActionHistory, error) {
	// Try to get existing action history
	existing, err := r.GetActionHistory(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing action history: %w", err)
	}

	if existing != nil {
		return existing, nil
	}

	// Create new action history with defaults
	insertQuery := `
		INSERT INTO action_histories (
			resource_id, max_actions, max_age_days, compaction_strategy,
			oscillation_window_minutes, effectiveness_threshold, pattern_min_occurrences
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	var history ActionHistory
	err = r.db.QueryRowContext(ctx, insertQuery,
		resourceID, 1000, 30, "pattern-aware", 120, 0.70, 3).Scan(
		&history.ID, &history.CreatedAt, &history.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create action history: %w", err)
	}

	history.ResourceID = resourceID
	history.MaxActions = 1000
	history.MaxAgeDays = 30
	history.CompactionStrategy = "pattern-aware"
	history.OscillationWindowMins = 120
	history.EffectivenessThreshold = 0.70
	history.PatternMinOccurrences = 3

	r.logger.WithFields(logrus.Fields{
		"action_history_id": history.ID,
		"resource_id":       resourceID,
	}).Debug("Created action history")

	return &history, nil
}

// GetActionHistory retrieves action history for a resource
func (r *PostgreSQLRepository) GetActionHistory(ctx context.Context, resourceID int64) (*ActionHistory, error) {
	query := `
		SELECT id, resource_id, max_actions, max_age_days, compaction_strategy,
		       oscillation_window_minutes, effectiveness_threshold, pattern_min_occurrences,
		       total_actions, last_action_at, last_analysis_at, next_analysis_at,
		       created_at, updated_at
		FROM action_histories
		WHERE resource_id = $1
		LIMIT 1`

	var history ActionHistory
	err := r.db.QueryRowContext(ctx, query, resourceID).Scan(
		&history.ID, &history.ResourceID, &history.MaxActions, &history.MaxAgeDays,
		&history.CompactionStrategy, &history.OscillationWindowMins, &history.EffectivenessThreshold,
		&history.PatternMinOccurrences, &history.TotalActions, &history.LastActionAt,
		&history.LastAnalysisAt, &history.NextAnalysisAt, &history.CreatedAt, &history.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get action history: %w", err)
	}

	return &history, nil
}

// UpdateActionHistory updates an action history record
func (r *PostgreSQLRepository) UpdateActionHistory(ctx context.Context, history *ActionHistory) error {
	query := `
		UPDATE action_histories
		SET total_actions = $2, last_action_at = $3, last_analysis_at = $4,
		    next_analysis_at = $5, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		history.ID, history.TotalActions, history.LastActionAt,
		history.LastAnalysisAt, history.NextAnalysisAt)

	if err != nil {
		return fmt.Errorf("failed to update action history: %w", err)
	}

	return nil
}

// StoreAction stores a new action record
func (r *PostgreSQLRepository) StoreAction(ctx context.Context, action *ActionRecord) (*ResourceActionTrace, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Ensure resource reference exists
	resourceID, err := r.ensureResourceReferenceInTx(ctx, tx, action.ResourceReference)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure resource reference: %w", err)
	}

	// Ensure action history exists
	actionHistory, err := r.ensureActionHistoryInTx(ctx, tx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure action history: %w", err)
	}

	// Generate action ID if not provided
	actionID := action.ActionID
	if actionID == "" {
		actionID = uuid.New().String()
	}

	// Insert action trace
	insertQuery := `
		INSERT INTO resource_action_traces (
			action_history_id, action_id, correlation_id, action_timestamp,
			alert_name, alert_severity, alert_labels, alert_annotations, alert_firing_time,
			model_used, routing_tier, model_confidence, model_reasoning, alternative_actions,
			action_type, action_parameters, resource_state_before
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		) RETURNING id, created_at, updated_at`

	var trace ResourceActionTrace
	err = tx.QueryRowContext(ctx, insertQuery,
		actionHistory.ID, actionID, action.CorrelationID, action.Timestamp,
		action.Alert.Name, action.Alert.Severity, StringMapToJSONMap(action.Alert.Labels),
		StringMapToJSONMap(action.Alert.Annotations), action.Alert.FiringTime,
		action.ModelUsed, action.RoutingTier, action.Confidence, action.Reasoning,
		action.AlternativeActions, action.ActionType, JSONMap(action.Parameters),
		JSONMap(action.ResourceStateBefore),
	).Scan(&trace.ID, &trace.CreatedAt, &trace.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to insert action trace: %w", err)
	}

	// Update action history counters
	_, err = tx.ExecContext(ctx, `
		UPDATE action_histories
		SET total_actions = total_actions + 1,
		    last_action_at = $2,
		    updated_at = NOW()
		WHERE id = $1`,
		actionHistory.ID, action.Timestamp)

	if err != nil {
		return nil, fmt.Errorf("failed to update action history counters: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Populate the returned trace with the data we inserted
	trace.ActionHistoryID = actionHistory.ID
	trace.ActionID = actionID
	trace.CorrelationID = action.CorrelationID
	trace.ActionTimestamp = action.Timestamp
	trace.AlertName = action.Alert.Name
	trace.AlertSeverity = action.Alert.Severity
	trace.AlertLabels = StringMapToJSONMap(action.Alert.Labels)
	trace.AlertAnnotations = StringMapToJSONMap(action.Alert.Annotations)
	trace.AlertFiringTime = &action.Alert.FiringTime
	trace.ModelUsed = action.ModelUsed
	trace.RoutingTier = action.RoutingTier
	trace.ModelConfidence = action.Confidence
	trace.ModelReasoning = action.Reasoning
	trace.AlternativeActions = action.AlternativeActions
	trace.ActionType = action.ActionType
	trace.ActionParameters = JSONMap(action.Parameters)
	trace.ResourceStateBefore = JSONMap(action.ResourceStateBefore)
	trace.ExecutionStatus = "pending"

	r.logger.WithFields(logrus.Fields{
		"action_id":   actionID,
		"resource_id": resourceID,
		"action_type": action.ActionType,
		"model_used":  action.ModelUsed,
	}).Info("Stored action trace")

	return &trace, nil
}

// Helper method to ensure resource reference within transaction
func (r *PostgreSQLRepository) ensureResourceReferenceInTx(ctx context.Context, tx *sql.Tx, ref ResourceReference) (int64, error) {
	var resourceID int64
	query := `
		SELECT id FROM resource_references
		WHERE namespace = $1 AND kind = $2 AND name = $3
		LIMIT 1`

	err := tx.QueryRowContext(ctx, query, ref.Namespace, ref.Kind, ref.Name).Scan(&resourceID)
	if err == nil {
		return resourceID, nil
	}

	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to query resource reference: %w", err)
	}

	// Create new resource reference
	insertQuery := `
		INSERT INTO resource_references (
			resource_uid, api_version, kind, name, namespace, last_seen
		) VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id`

	err = tx.QueryRowContext(ctx, insertQuery,
		ref.ResourceUID, ref.APIVersion, ref.Kind, ref.Name, ref.Namespace).Scan(&resourceID)

	return resourceID, err
}

// Helper method to ensure action history within transaction
func (r *PostgreSQLRepository) ensureActionHistoryInTx(ctx context.Context, tx *sql.Tx, resourceID int64) (*ActionHistory, error) {
	var history ActionHistory
	query := `
		SELECT id, resource_id, max_actions, max_age_days, compaction_strategy,
		       oscillation_window_minutes, effectiveness_threshold, pattern_min_occurrences,
		       total_actions, last_action_at, last_analysis_at, next_analysis_at,
		       created_at, updated_at
		FROM action_histories
		WHERE resource_id = $1
		LIMIT 1`

	err := tx.QueryRowContext(ctx, query, resourceID).Scan(
		&history.ID, &history.ResourceID, &history.MaxActions, &history.MaxAgeDays,
		&history.CompactionStrategy, &history.OscillationWindowMins, &history.EffectivenessThreshold,
		&history.PatternMinOccurrences, &history.TotalActions, &history.LastActionAt,
		&history.LastAnalysisAt, &history.NextAnalysisAt, &history.CreatedAt, &history.UpdatedAt,
	)

	if err == nil {
		return &history, nil
	}

	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query action history: %w", err)
	}

	// Create new action history
	insertQuery := `
		INSERT INTO action_histories (
			resource_id, max_actions, max_age_days, compaction_strategy,
			oscillation_window_minutes, effectiveness_threshold, pattern_min_occurrences
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	err = tx.QueryRowContext(ctx, insertQuery,
		resourceID, 1000, 30, "pattern-aware", 120, 0.70, 3).Scan(
		&history.ID, &history.CreatedAt, &history.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create action history: %w", err)
	}

	history.ResourceID = resourceID
	history.MaxActions = 1000
	history.MaxAgeDays = 30
	history.CompactionStrategy = "pattern-aware"
	history.OscillationWindowMins = 120
	history.EffectivenessThreshold = 0.70
	history.PatternMinOccurrences = 3

	return &history, nil
}

// GetActionTraces retrieves action traces based on query parameters
func (r *PostgreSQLRepository) GetActionTraces(ctx context.Context, query ActionQuery) ([]ResourceActionTrace, error) {
	sqlQuery := `
		SELECT rat.id, rat.action_history_id, rat.action_id, rat.correlation_id,
		       rat.action_timestamp, rat.execution_start_time, rat.execution_end_time,
		       rat.execution_duration_ms, rat.alert_name, rat.alert_severity,
		       rat.alert_labels, rat.alert_annotations, rat.alert_firing_time,
		       rat.model_used, rat.routing_tier, rat.model_confidence, rat.model_reasoning,
		       rat.alternative_actions, rat.action_type, rat.action_parameters,
		       rat.resource_state_before, rat.resource_state_after, rat.execution_status,
		       rat.execution_error, rat.kubernetes_operations, rat.effectiveness_score,
		       rat.effectiveness_criteria, rat.effectiveness_assessed_at,
		       rat.effectiveness_assessment_method, rat.effectiveness_notes,
		       rat.follow_up_actions, rat.created_at, rat.updated_at
		FROM resource_action_traces rat
		JOIN action_histories ah ON rat.action_history_id = ah.id
		JOIN resource_references rr ON ah.resource_id = rr.id
		WHERE 1=1`

	var args []interface{}
	argIndex := 1

	// Add query filters
	if query.Namespace != "" {
		sqlQuery += fmt.Sprintf(" AND rr.namespace = $%d", argIndex)
		args = append(args, query.Namespace)
		argIndex++
	}

	if query.ResourceKind != "" {
		sqlQuery += fmt.Sprintf(" AND rr.kind = $%d", argIndex)
		args = append(args, query.ResourceKind)
		argIndex++
	}

	if query.ResourceName != "" {
		sqlQuery += fmt.Sprintf(" AND rr.name = $%d", argIndex)
		args = append(args, query.ResourceName)
		argIndex++
	}

	if query.ActionType != "" {
		sqlQuery += fmt.Sprintf(" AND rat.action_type = $%d", argIndex)
		args = append(args, query.ActionType)
		argIndex++
	}

	if query.ModelUsed != "" {
		sqlQuery += fmt.Sprintf(" AND rat.model_used = $%d", argIndex)
		args = append(args, query.ModelUsed)
		argIndex++
	}

	if !query.TimeRange.Start.IsZero() {
		sqlQuery += fmt.Sprintf(" AND rat.action_timestamp >= $%d", argIndex)
		args = append(args, query.TimeRange.Start)
		argIndex++
	}

	if !query.TimeRange.End.IsZero() {
		sqlQuery += fmt.Sprintf(" AND rat.action_timestamp <= $%d", argIndex)
		args = append(args, query.TimeRange.End)
		argIndex++
	}

	// Add ordering and pagination
	sqlQuery += " ORDER BY rat.action_timestamp DESC"

	if query.Limit > 0 {
		sqlQuery += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, query.Limit)
		argIndex++
	}

	if query.Offset > 0 {
		sqlQuery += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, query.Offset)
	}

	rows, err := r.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query action traces: %w", err)
	}
	defer rows.Close()

	var traces []ResourceActionTrace
	for rows.Next() {
		var trace ResourceActionTrace
		err := rows.Scan(
			&trace.ID, &trace.ActionHistoryID, &trace.ActionID, &trace.CorrelationID,
			&trace.ActionTimestamp, &trace.ExecutionStartTime, &trace.ExecutionEndTime,
			&trace.ExecutionDurationMs, &trace.AlertName, &trace.AlertSeverity,
			&trace.AlertLabels, &trace.AlertAnnotations, &trace.AlertFiringTime,
			&trace.ModelUsed, &trace.RoutingTier, &trace.ModelConfidence, &trace.ModelReasoning,
			&trace.AlternativeActions, &trace.ActionType, &trace.ActionParameters,
			&trace.ResourceStateBefore, &trace.ResourceStateAfter, &trace.ExecutionStatus,
			&trace.ExecutionError, &trace.KubernetesOperations, &trace.EffectivenessScore,
			&trace.EffectivenessCriteria, &trace.EffectivenessAssessedAt,
			&trace.EffectivenessAssessmentMethod, &trace.EffectivenessNotes,
			&trace.FollowUpActions, &trace.CreatedAt, &trace.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan action trace: %w", err)
		}
		traces = append(traces, trace)
	}

	return traces, nil
}

// GetActionTrace retrieves a single action trace by ID
func (r *PostgreSQLRepository) GetActionTrace(ctx context.Context, actionID string) (*ResourceActionTrace, error) {
	query := `
		SELECT id, action_history_id, action_id, correlation_id, action_timestamp,
		       execution_start_time, execution_end_time, execution_duration_ms,
		       alert_name, alert_severity, alert_labels, alert_annotations, alert_firing_time,
		       model_used, routing_tier, model_confidence, model_reasoning, alternative_actions,
		       action_type, action_parameters, resource_state_before, resource_state_after,
		       execution_status, execution_error, kubernetes_operations, effectiveness_score,
		       effectiveness_criteria, effectiveness_assessed_at, effectiveness_assessment_method,
		       effectiveness_notes, follow_up_actions, created_at, updated_at
		FROM resource_action_traces
		WHERE action_id = $1
		LIMIT 1`

	var trace ResourceActionTrace
	err := r.db.QueryRowContext(ctx, query, actionID).Scan(
		&trace.ID, &trace.ActionHistoryID, &trace.ActionID, &trace.CorrelationID,
		&trace.ActionTimestamp, &trace.ExecutionStartTime, &trace.ExecutionEndTime,
		&trace.ExecutionDurationMs, &trace.AlertName, &trace.AlertSeverity,
		&trace.AlertLabels, &trace.AlertAnnotations, &trace.AlertFiringTime,
		&trace.ModelUsed, &trace.RoutingTier, &trace.ModelConfidence, &trace.ModelReasoning,
		&trace.AlternativeActions, &trace.ActionType, &trace.ActionParameters,
		&trace.ResourceStateBefore, &trace.ResourceStateAfter, &trace.ExecutionStatus,
		&trace.ExecutionError, &trace.KubernetesOperations, &trace.EffectivenessScore,
		&trace.EffectivenessCriteria, &trace.EffectivenessAssessedAt,
		&trace.EffectivenessAssessmentMethod, &trace.EffectivenessNotes,
		&trace.FollowUpActions, &trace.CreatedAt, &trace.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get action trace: %w", err)
	}

	return &trace, nil
}

// UpdateActionTrace updates an action trace
func (r *PostgreSQLRepository) UpdateActionTrace(ctx context.Context, trace *ResourceActionTrace) error {
	query := `
		UPDATE resource_action_traces
		SET execution_start_time = $2, execution_end_time = $3, execution_duration_ms = $4,
		    resource_state_after = $5, execution_status = $6, execution_error = $7,
		    kubernetes_operations = $8, effectiveness_score = $9, effectiveness_criteria = $10,
		    effectiveness_assessed_at = $11, effectiveness_assessment_due = $12,
		    effectiveness_assessment_method = $13, effectiveness_notes = $14,
		    follow_up_actions = $15, updated_at = NOW()
		WHERE action_id = $1`

	_, err := r.db.ExecContext(ctx, query,
		trace.ActionID, trace.ExecutionStartTime, trace.ExecutionEndTime,
		trace.ExecutionDurationMs, trace.ResourceStateAfter, trace.ExecutionStatus,
		trace.ExecutionError, trace.KubernetesOperations, trace.EffectivenessScore,
		trace.EffectivenessCriteria, trace.EffectivenessAssessedAt, trace.EffectivenessAssessmentDue,
		trace.EffectivenessAssessmentMethod, trace.EffectivenessNotes,
		trace.FollowUpActions)

	if err != nil {
		return fmt.Errorf("failed to update action trace: %w", err)
	}

	return nil
}

// GetPendingEffectivenessAssessments retrieves action traces that need effectiveness assessment
func (r *PostgreSQLRepository) GetPendingEffectivenessAssessments(ctx context.Context) ([]*ResourceActionTrace, error) {
	query := `
		SELECT id, action_history_id, action_id, correlation_id, action_timestamp,
		       execution_start_time, execution_end_time, execution_duration_ms,
		       alert_name, alert_severity, alert_labels, alert_annotations, alert_firing_time,
		       model_used, routing_tier, model_confidence, model_reasoning, alternative_actions,
		       action_type, action_parameters, resource_state_before, resource_state_after,
		       execution_status, execution_error, kubernetes_operations,
		       effectiveness_score, effectiveness_criteria, effectiveness_assessed_at,
		       effectiveness_assessment_due, effectiveness_assessment_method, effectiveness_notes,
		       follow_up_actions, created_at, updated_at
		FROM resource_action_traces
		WHERE effectiveness_assessment_due IS NOT NULL
		  AND effectiveness_assessment_due <= NOW()
		  AND effectiveness_score IS NULL
		  AND execution_status = 'completed'
		ORDER BY effectiveness_assessment_due ASC
		LIMIT 100`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending assessments: %w", err)
	}
	defer rows.Close()

	var traces []*ResourceActionTrace
	for rows.Next() {
		trace := &ResourceActionTrace{}

		err := rows.Scan(
			&trace.ID, &trace.ActionHistoryID, &trace.ActionID, &trace.CorrelationID,
			&trace.ActionTimestamp, &trace.ExecutionStartTime, &trace.ExecutionEndTime,
			&trace.ExecutionDurationMs, &trace.AlertName, &trace.AlertSeverity,
			&trace.AlertLabels, &trace.AlertAnnotations, &trace.AlertFiringTime,
			&trace.ModelUsed, &trace.RoutingTier, &trace.ModelConfidence,
			&trace.ModelReasoning, &trace.AlternativeActions, &trace.ActionType,
			&trace.ActionParameters, &trace.ResourceStateBefore, &trace.ResourceStateAfter,
			&trace.ExecutionStatus, &trace.ExecutionError, &trace.KubernetesOperations,
			&trace.EffectivenessScore, &trace.EffectivenessCriteria, &trace.EffectivenessAssessedAt,
			&trace.EffectivenessAssessmentDue, &trace.EffectivenessAssessmentMethod,
			&trace.EffectivenessNotes, &trace.FollowUpActions, &trace.CreatedAt, &trace.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan action trace: %w", err)
		}

		traces = append(traces, trace)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	r.logger.WithField("count", len(traces)).Debug("Retrieved pending effectiveness assessments")
	return traces, nil
}

// GetOscillationPatterns retrieves oscillation patterns
func (r *PostgreSQLRepository) GetOscillationPatterns(ctx context.Context, patternType string) ([]OscillationPattern, error) {
	query := `
		SELECT id, pattern_type, pattern_name, description, min_occurrences,
		       time_window_minutes, action_sequence, threshold_config, resource_types,
		       namespaces, label_selectors, prevention_strategy, prevention_parameters,
		       alerting_enabled, alert_severity, alert_channels, total_detections,
		       prevention_success_rate, false_positive_rate, last_detection_at,
		       active, created_at, updated_at
		FROM oscillation_patterns
		WHERE active = true`

	var args []interface{}
	if patternType != "" {
		query += " AND pattern_type = $1"
		args = append(args, patternType)
	}

	query += " ORDER BY pattern_type, pattern_name"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query oscillation patterns: %w", err)
	}
	defer rows.Close()

	var patterns []OscillationPattern
	for rows.Next() {
		var pattern OscillationPattern
		err := rows.Scan(
			&pattern.ID, &pattern.PatternType, &pattern.PatternName, &pattern.Description,
			&pattern.MinOccurrences, &pattern.TimeWindowMinutes, &pattern.ActionSequence,
			&pattern.ThresholdConfig, pq.Array(&pattern.ResourceTypes), pq.Array(&pattern.Namespaces),
			&pattern.LabelSelectors, &pattern.PreventionStrategy, &pattern.PreventionParameters,
			&pattern.AlertingEnabled, &pattern.AlertSeverity, pq.Array(&pattern.AlertChannels),
			&pattern.TotalDetections, &pattern.PreventionSuccessRate, &pattern.FalsePositiveRate,
			&pattern.LastDetectionAt, &pattern.Active, &pattern.CreatedAt, &pattern.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan oscillation pattern: %w", err)
		}
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// StoreOscillationDetection stores an oscillation detection
func (r *PostgreSQLRepository) StoreOscillationDetection(ctx context.Context, detection *OscillationDetection) error {
	query := `
		INSERT INTO oscillation_detections (
			pattern_id, resource_id, detected_at, confidence, action_count,
			time_span_minutes, matching_actions, pattern_evidence, prevention_applied,
			prevention_action, prevention_details
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at`

	err := r.db.QueryRowContext(ctx, query,
		detection.PatternID, detection.ResourceID, detection.DetectedAt,
		detection.Confidence, detection.ActionCount, detection.TimeSpanMinutes,
		pq.Array(detection.MatchingActions), detection.PatternEvidence,
		detection.PreventionApplied, detection.PreventionAction,
		detection.PreventionDetails).Scan(&detection.ID, &detection.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to store oscillation detection: %w", err)
	}

	return nil
}

// GetOscillationDetections retrieves oscillation detections for a resource
func (r *PostgreSQLRepository) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]OscillationDetection, error) {
	query := `
		SELECT id, pattern_id, resource_id, detected_at, confidence, action_count,
		       time_span_minutes, matching_actions, pattern_evidence, prevention_applied,
		       prevention_action, prevention_details, prevention_successful, resolved,
		       resolved_at, resolution_method, resolution_notes, created_at
		FROM oscillation_detections
		WHERE resource_id = $1`

	var args []interface{}
	args = append(args, resourceID)

	if resolved != nil {
		query += " AND resolved = $2"
		args = append(args, *resolved)
	}

	query += " ORDER BY detected_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query oscillation detections: %w", err)
	}
	defer rows.Close()

	var detections []OscillationDetection
	for rows.Next() {
		var detection OscillationDetection
		err := rows.Scan(
			&detection.ID, &detection.PatternID, &detection.ResourceID, &detection.DetectedAt,
			&detection.Confidence, &detection.ActionCount, &detection.TimeSpanMinutes,
			pq.Array(&detection.MatchingActions), &detection.PatternEvidence,
			&detection.PreventionApplied, &detection.PreventionAction, &detection.PreventionDetails,
			&detection.PreventionSuccessful, &detection.Resolved, &detection.ResolvedAt,
			&detection.ResolutionMethod, &detection.ResolutionNotes, &detection.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan oscillation detection: %w", err)
		}
		detections = append(detections, detection)
	}

	return detections, nil
}

// ApplyRetention applies retention policy to action history
func (r *PostgreSQLRepository) ApplyRetention(ctx context.Context, actionHistoryID int64) error {
	// This is a placeholder implementation
	// In a full implementation, this would implement the retention logic
	// from the design document

	r.logger.WithFields(logrus.Fields{
		"action_history_id": actionHistoryID,
	}).Debug("Retention policy applied (placeholder)")

	return nil
}

// ActionHistorySummary represents a summary of action history for sync to CRDs
type ActionHistorySummary struct {
	DatabaseID          int64             `json:"database_id"`
	ResourceRef         ResourceReference `json:"resource_ref"`
	TotalActions        int               `json:"total_actions"`
	LastActionTime      time.Time         `json:"last_action_time"`
	RecentEffectiveness float64           `json:"recent_effectiveness"`
	ActivePatterns      []string          `json:"active_patterns"`
	QuickStats          map[string]int    `json:"quick_stats"`
}

// GetActionHistorySummaries retrieves action history summaries for CRD sync
func (r *PostgreSQLRepository) GetActionHistorySummaries(ctx context.Context, since time.Duration) ([]ActionHistorySummary, error) {
	query := `
		SELECT
		    ah.id as database_id,
		    rr.id, rr.resource_uid, rr.api_version, rr.kind, rr.name, rr.namespace,
		    rr.created_at, rr.deleted_at, rr.last_seen,
		    ah.total_actions,
		    COALESCE(ah.last_action_at, ah.created_at) as last_action_time,
		    COALESCE(recent_stats.avg_effectiveness, 0.0) as recent_effectiveness,
		    COALESCE(recent_stats.action_count_24h, 0) as last_24h,
		    COALESCE(recent_stats.action_count_7d, 0) as last_7d
		FROM action_histories ah
		JOIN resource_references rr ON ah.resource_id = rr.id
		LEFT JOIN (
		    SELECT
		        action_history_id,
		        AVG(effectiveness_score) as avg_effectiveness,
		        COUNT(*) FILTER (WHERE action_timestamp > NOW() - INTERVAL '24 hours') as action_count_24h,
		        COUNT(*) FILTER (WHERE action_timestamp > NOW() - INTERVAL '7 days') as action_count_7d
		    FROM resource_action_traces
		    WHERE action_timestamp > NOW() - INTERVAL '7 days'
		    GROUP BY action_history_id
		) recent_stats ON ah.id = recent_stats.action_history_id
		WHERE ah.updated_at > NOW() - INTERVAL '%d minutes'
		ORDER BY ah.updated_at DESC`

	formattedQuery := fmt.Sprintf(query, int(since.Minutes()))

	rows, err := r.db.QueryContext(ctx, formattedQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query action history summaries: %w", err)
	}
	defer rows.Close()

	var summaries []ActionHistorySummary
	for rows.Next() {
		var summary ActionHistorySummary
		var last24h, last7d int

		err := rows.Scan(
			&summary.DatabaseID,
			&summary.ResourceRef.ID, &summary.ResourceRef.ResourceUID,
			&summary.ResourceRef.APIVersion, &summary.ResourceRef.Kind,
			&summary.ResourceRef.Name, &summary.ResourceRef.Namespace,
			&summary.ResourceRef.CreatedAt, &summary.ResourceRef.DeletedAt,
			&summary.ResourceRef.LastSeen, &summary.TotalActions,
			&summary.LastActionTime, &summary.RecentEffectiveness,
			&last24h, &last7d,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan action history summary: %w", err)
		}

		summary.QuickStats = map[string]int{
			"last_24h": last24h,
			"last_7d":  last7d,
		}

		// TODO: Add active patterns query
		summary.ActivePatterns = []string{}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}
