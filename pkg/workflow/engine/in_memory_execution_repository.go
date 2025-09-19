package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// InMemoryExecutionRepository provides an in-memory implementation of ExecutionRepository
// This is useful for testing and simple deployments where persistence is not required
type InMemoryExecutionRepository struct {
	executions map[string]*RuntimeWorkflowExecution
	mutex      sync.RWMutex
	log        *logrus.Logger
}

// NewInMemoryExecutionRepository creates a new in-memory execution repository
func NewInMemoryExecutionRepository(log *logrus.Logger) ExecutionRepository {
	return &InMemoryExecutionRepository{
		executions: make(map[string]*RuntimeWorkflowExecution),
		log:        log,
	}
}

// StoreExecution stores a workflow execution in memory
func (repo *InMemoryExecutionRepository) StoreExecution(ctx context.Context, execution *RuntimeWorkflowExecution) error {
	repo.mutex.Lock()
	defer repo.mutex.Unlock()

	repo.executions[execution.ID] = execution

	repo.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"status":       execution.Status,
	}).Debug("Stored workflow execution in memory")

	return nil
}

// GetExecution retrieves a workflow execution from memory
func (repo *InMemoryExecutionRepository) GetExecution(ctx context.Context, executionID string) (*RuntimeWorkflowExecution, error) {
	repo.mutex.RLock()
	defer repo.mutex.RUnlock()

	execution, exists := repo.executions[executionID]
	if !exists {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}

	return execution, nil
}

// ListExecutions lists all executions for a workflow
func (repo *InMemoryExecutionRepository) ListExecutions(ctx context.Context, workflowID string) ([]*RuntimeWorkflowExecution, error) {
	// Check for context cancellation before acquiring lock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	repo.mutex.RLock()
	defer repo.mutex.RUnlock()

	var executions []*RuntimeWorkflowExecution
	for _, execution := range repo.executions {
		// Check for context cancellation during iteration for large datasets
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if execution.WorkflowID == workflowID {
			executions = append(executions, execution)
		}
	}

	return executions, nil
}

// GetExecutionsByWorkflowID lists all executions for a workflow (alias for ListExecutions)
func (repo *InMemoryExecutionRepository) GetExecutionsByWorkflowID(ctx context.Context, workflowID string) ([]*RuntimeWorkflowExecution, error) {
	return repo.ListExecutions(ctx, workflowID)
}

// GetExecutionsByPattern retrieves all executions that match a specific pattern ID
func (repo *InMemoryExecutionRepository) GetExecutionsByPattern(ctx context.Context, patternID string) ([]*RuntimeWorkflowExecution, error) {
	repo.mutex.RLock()
	defer repo.mutex.RUnlock()

	var executions []*RuntimeWorkflowExecution
	for _, execution := range repo.executions {
		// Check if execution has pattern metadata matching the patternID
		if execution.Metadata != nil {
			if executionPatternID, exists := execution.Metadata["pattern_id"]; exists {
				if executionPatternID == patternID {
					executions = append(executions, execution)
				}
			}
		}
	}

	return executions, nil
}

// GetExecutionsInTimeWindow returns executions within a specific time window
func (repo *InMemoryExecutionRepository) GetExecutionsInTimeWindow(ctx context.Context, start, end time.Time) ([]*RuntimeWorkflowExecution, error) {
	repo.mutex.RLock()
	defer repo.mutex.RUnlock()

	var executions []*RuntimeWorkflowExecution
	for _, execution := range repo.executions {
		if execution.StartTime.After(start) && execution.StartTime.Before(end) {
			executions = append(executions, execution)
		}
	}

	return executions, nil
}

// UpdateExecution updates an existing execution
func (repo *InMemoryExecutionRepository) UpdateExecution(ctx context.Context, execution *RuntimeWorkflowExecution) error {
	// Check for context cancellation before acquiring lock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	repo.mutex.Lock()
	defer repo.mutex.Unlock()

	if _, exists := repo.executions[execution.ID]; !exists {
		return fmt.Errorf("execution not found for update: %s", execution.ID)
	}

	repo.executions[execution.ID] = execution

	repo.log.WithContext(ctx).WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"status":       execution.Status,
	}).Debug("Updated workflow execution in memory")

	return nil
}

// DeleteExecution deletes an execution from memory
func (repo *InMemoryExecutionRepository) DeleteExecution(ctx context.Context, executionID string) error {
	// Check for context cancellation before acquiring lock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	repo.mutex.Lock()
	defer repo.mutex.Unlock()

	if _, exists := repo.executions[executionID]; !exists {
		return fmt.Errorf("execution not found for deletion: %s", executionID)
	}

	delete(repo.executions, executionID)

	repo.log.WithContext(ctx).WithField("execution_id", executionID).Debug("Deleted workflow execution from memory")

	return nil
}

// GetExecutionCount returns the total number of executions stored
func (repo *InMemoryExecutionRepository) GetExecutionCount(ctx context.Context) (int, error) {
	// Check for context cancellation before acquiring lock
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	repo.mutex.RLock()
	defer repo.mutex.RUnlock()

	return len(repo.executions), nil
}
