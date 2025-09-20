package execution

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// DatabaseExecutionRepository provides database-backed execution storage
type DatabaseExecutionRepository struct {
	db  *sql.DB
	log *logrus.Logger
}

// NewDatabaseExecutionRepository creates a new database execution repository
func NewDatabaseExecutionRepository(db *sql.DB, log *logrus.Logger) *DatabaseExecutionRepository {
	return &DatabaseExecutionRepository{
		db:  db,
		log: log,
	}
}

// GetExecutionsInTimeWindow retrieves executions within a time window
func (der *DatabaseExecutionRepository) GetExecutionsInTimeWindow(ctx context.Context, start, end time.Time) ([]*engine.RuntimeWorkflowExecution, error) {
	query := `
		SELECT id, workflow_id, status, start_time, end_time, duration_ms, error, metadata
		FROM workflow_executions
		WHERE start_time >= $1 AND start_time <= $2
		ORDER BY start_time DESC
	`

	rows, err := der.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query executions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	executions := make([]*engine.RuntimeWorkflowExecution, 0)
	for rows.Next() {
		execution, err := der.scanExecution(rows)
		if err != nil {
			der.log.WithError(err).Warn("Failed to scan execution row")
			continue
		}
		executions = append(executions, execution)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating execution rows: %w", err)
	}

	der.log.WithFields(logrus.Fields{
		"start_time":       start,
		"end_time":         end,
		"executions_found": len(executions),
	}).Debug("Retrieved executions in time window")

	return executions, nil
}

// GetExecutionsByWorkflowID retrieves executions for a specific workflow
func (der *DatabaseExecutionRepository) GetExecutionsByWorkflowID(ctx context.Context, workflowID string) ([]*engine.RuntimeWorkflowExecution, error) {
	query := `
		SELECT id, workflow_id, status, start_time, end_time, duration_ms, error, metadata
		FROM workflow_executions
		WHERE workflow_id = $1
		ORDER BY start_time DESC
	`

	rows, err := der.db.QueryContext(ctx, query, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to query executions for workflow %s: %w", workflowID, err)
	}
	defer func() { _ = rows.Close() }()

	executions := make([]*engine.RuntimeWorkflowExecution, 0)
	for rows.Next() {
		execution, err := der.scanExecution(rows)
		if err != nil {
			der.log.WithError(err).Warn("Failed to scan execution row")
			continue
		}
		executions = append(executions, execution)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating execution rows: %w", err)
	}

	der.log.WithFields(logrus.Fields{
		"workflow_id":      workflowID,
		"executions_found": len(executions),
	}).Debug("Retrieved executions for workflow")

	return executions, nil
}

// GetExecutionsByPattern retrieves executions matching a pattern
func (der *DatabaseExecutionRepository) GetExecutionsByPattern(ctx context.Context, patternID string) ([]*engine.RuntimeWorkflowExecution, error) {
	query := `
		SELECT id, workflow_id, status, start_time, end_time, duration_ms, error, metadata
		FROM workflow_executions
		WHERE metadata->>'pattern_id' = $1
		ORDER BY start_time DESC
	`

	rows, err := der.db.QueryContext(ctx, query, patternID)
	if err != nil {
		return nil, fmt.Errorf("failed to query executions for pattern %s: %w", patternID, err)
	}
	defer func() { _ = rows.Close() }()

	executions := make([]*engine.RuntimeWorkflowExecution, 0)
	for rows.Next() {
		execution, err := der.scanExecution(rows)
		if err != nil {
			der.log.WithError(err).Warn("Failed to scan execution row")
			continue
		}
		executions = append(executions, execution)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating execution rows: %w", err)
	}

	der.log.WithFields(logrus.Fields{
		"pattern_id":       patternID,
		"executions_found": len(executions),
	}).Debug("Retrieved executions for pattern")

	return executions, nil
}

// StoreExecution stores a workflow execution
func (der *DatabaseExecutionRepository) StoreExecution(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	query := `
		INSERT INTO workflow_executions
		(id, workflow_id, status, start_time, end_time, duration_ms, error, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			end_time = EXCLUDED.end_time,
			duration_ms = EXCLUDED.duration_ms,
			error = EXCLUDED.error,
			metadata = EXCLUDED.metadata
	`

	var endTime *time.Time
	var durationMs int64
	var errorStr string

	if execution.EndTime != nil {
		endTime = execution.EndTime
		durationMs = execution.Duration.Milliseconds()
	}

	if execution.Error != "" {
		errorStr = execution.Error
	}

	// Convert metadata to JSON
	metadataJSON := "{}"
	if execution.Metadata != nil {
		// In production, use proper JSON marshaling
		metadataJSON = fmt.Sprintf(`{"workflow_id": "%s"}`, execution.WorkflowID)
	}

	_, err := der.db.ExecContext(ctx, query,
		execution.ID,
		execution.WorkflowID,
		string(execution.Status),
		execution.StartTime,
		endTime,
		durationMs,
		errorStr,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to store execution %s: %w", execution.ID, err)
	}

	der.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"status":       execution.Status,
	}).Debug("Stored workflow execution")

	return nil
}

// scanExecution scans a database row into a WorkflowExecution
func (der *DatabaseExecutionRepository) scanExecution(rows *sql.Rows) (*engine.RuntimeWorkflowExecution, error) {
	var execution engine.RuntimeWorkflowExecution
	var endTime sql.NullTime
	var durationMs sql.NullInt64
	var errorStr sql.NullString
	var metadataJSON string

	err := rows.Scan(
		&execution.ID,
		&execution.WorkflowID,
		&execution.Status,
		&execution.StartTime,
		&endTime,
		&durationMs,
		&errorStr,
		&metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan execution row: %w", err)
	}

	// Handle nullable fields
	if endTime.Valid {
		execution.EndTime = &endTime.Time
	}

	if durationMs.Valid {
		execution.Duration = time.Duration(durationMs.Int64) * time.Millisecond
	}

	if errorStr.Valid {
		execution.Error = errorStr.String
	}

	// Basic metadata parsing (in production, use proper JSON unmarshaling)
	execution.Metadata = map[string]interface{}{
		"stored": true,
	}

	return &execution, nil
}

// OrchestrationInMemoryExecutionRepository provides in-memory execution storage for testing
type OrchestrationInMemoryExecutionRepository struct {
	executions map[string]*engine.RuntimeWorkflowExecution
	mutex      sync.RWMutex
	log        *logrus.Logger
}

// NewOrchestrationInMemoryExecutionRepository creates a new in-memory execution repository
func NewOrchestrationInMemoryExecutionRepository(log *logrus.Logger) *OrchestrationInMemoryExecutionRepository {
	return &OrchestrationInMemoryExecutionRepository{
		executions: make(map[string]*engine.RuntimeWorkflowExecution),
		log:        log,
	}
}

// GetExecutionsInTimeWindow retrieves executions within a time window
func (imer *OrchestrationInMemoryExecutionRepository) GetExecutionsInTimeWindow(ctx context.Context, start, end time.Time) ([]*engine.RuntimeWorkflowExecution, error) {
	imer.mutex.RLock()
	defer imer.mutex.RUnlock()

	executions := make([]*engine.RuntimeWorkflowExecution, 0)

	for _, execution := range imer.executions {
		if !execution.StartTime.Before(start) && !execution.StartTime.After(end) {
			executions = append(executions, execution)
		}
	}

	imer.log.WithFields(logrus.Fields{
		"start_time":       start,
		"end_time":         end,
		"executions_found": len(executions),
	}).Debug("Retrieved executions in time window from memory")

	return executions, nil
}

// GetExecutionsByWorkflowID retrieves executions for a specific workflow
func (imer *OrchestrationInMemoryExecutionRepository) GetExecutionsByWorkflowID(ctx context.Context, workflowID string) ([]*engine.RuntimeWorkflowExecution, error) {
	imer.mutex.RLock()
	defer imer.mutex.RUnlock()

	executions := make([]*engine.RuntimeWorkflowExecution, 0)

	for _, execution := range imer.executions {
		if execution.WorkflowID == workflowID {
			executions = append(executions, execution)
		}
	}

	imer.log.WithFields(logrus.Fields{
		"workflow_id":      workflowID,
		"executions_found": len(executions),
	}).Debug("Retrieved executions for workflow from memory")

	return executions, nil
}

// GetExecutionsByPattern retrieves executions matching a pattern
func (imer *OrchestrationInMemoryExecutionRepository) GetExecutionsByPattern(ctx context.Context, patternID string) ([]*engine.RuntimeWorkflowExecution, error) {
	imer.mutex.RLock()
	defer imer.mutex.RUnlock()

	executions := make([]*engine.RuntimeWorkflowExecution, 0)

	for _, execution := range imer.executions {
		if execution.Metadata != nil {
			if pid, ok := execution.Metadata["pattern_id"].(string); ok && pid == patternID {
				executions = append(executions, execution)
			}
		}
	}

	imer.log.WithFields(logrus.Fields{
		"pattern_id":       patternID,
		"executions_found": len(executions),
	}).Debug("Retrieved executions for pattern from memory")

	return executions, nil
}

// StoreExecution stores a workflow execution in memory
func (imer *OrchestrationInMemoryExecutionRepository) StoreExecution(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	imer.mutex.Lock()
	defer imer.mutex.Unlock()

	imer.executions[execution.ID] = execution

	imer.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"status":       execution.Status,
	}).Debug("Stored workflow execution in memory")

	return nil
}
