package workflow

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// WorkflowService defines the interface for workflow orchestration operations
// Business Requirements: BR-WORKFLOW-001 to BR-WORKFLOW-006
// Single Responsibility: Workflow creation, execution coordination, and monitoring
type WorkflowService interface {
	// Core workflow processing
	ProcessAlert(ctx context.Context, alert types.Alert) (*ExecutionResult, error)

	// BR-WORKFLOW-001: Workflow creation and management
	CreateWorkflow(ctx context.Context, alert types.Alert) map[string]interface{}
	StartWorkflow(ctx context.Context, workflowID string) map[string]interface{}
	GetWorkflowStatus(workflowID string) map[string]interface{}

	// BR-WORKFLOW-002: Action execution coordination
	CoordinateActions(ctx context.Context, alert types.Alert) map[string]interface{}

	// BR-WORKFLOW-003: Workflow state management
	PersistWorkflowState(workflowID string, state map[string]interface{}) map[string]interface{}
	RestoreWorkflowState(workflowID string) map[string]interface{}

	// BR-WORKFLOW-004: Execution monitoring
	MonitorExecution(workflowID string) map[string]interface{}
	GetExecutionMetrics() map[string]interface{}

	// BR-WORKFLOW-005: Rollback capabilities
	RollbackWorkflow(ctx context.Context, workflowID string) map[string]interface{}
	GetRollbackHistory(namespace string, duration time.Duration) map[string]interface{}

	// BR-WORKFLOW-006: Service integration
	ProcessAlertFromService(ctx context.Context, alert types.Alert, sourceService string) map[string]interface{}

	// Service health
	Health() map[string]interface{}
}

// ExecutionResult represents the result of workflow execution
type ExecutionResult struct {
	Success           bool                   `json:"success"`
	WorkflowID        string                 `json:"workflow_id"`
	ExecutionID       string                 `json:"execution_id"`
	Status            string                 `json:"status"` // created, running, completed, failed
	ActionsExecuted   int                    `json:"actions_executed"`
	ActionsTotal      int                    `json:"actions_total"`
	ExecutionTime     time.Duration          `json:"execution_time"`
	RollbackRequired  bool                   `json:"rollback_required,omitempty"`
	RollbackInitiated bool                   `json:"rollback_initiated,omitempty"`
	WorkflowResult    map[string]interface{} `json:"workflow_result,omitempty"`
	ExecutionDetails  map[string]interface{} `json:"execution_details,omitempty"`
}

// WorkflowOrchestrator handles the core workflow orchestration logic
type WorkflowOrchestrator interface {
	// Core orchestration operations
	Orchestrate(ctx context.Context, alert types.Alert) (*ExecutionResult, error)

	// Workflow lifecycle
	Create(ctx context.Context, alert types.Alert) (*Workflow, error)
	Execute(ctx context.Context, workflow *Workflow) (*ExecutionResult, error)
	Monitor(workflowID string) (*WorkflowStatus, error)

	// State management
	SaveState(workflowID string, state *WorkflowState) error
	LoadState(workflowID string) (*WorkflowState, error)

	// Rollback operations
	Rollback(ctx context.Context, workflowID string) error
}

// WorkflowExecutor handles action execution within workflows
type WorkflowExecutor interface {
	ExecuteAction(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error
	ExecuteActions(ctx context.Context, actions []*types.ActionRecommendation, alert types.Alert) (*ExecutionResult, error)
	IsHealthy() bool
	GetCapabilities() []string
}

// WorkflowStateManager handles workflow state persistence and recovery
type WorkflowStateManager interface {
	Persist(workflowID string, state *WorkflowState) error
	Restore(workflowID string) (*WorkflowState, error)
	Delete(workflowID string) error
	ListActive() ([]string, error)
}

// WorkflowMonitor handles execution monitoring and metrics
type WorkflowMonitor interface {
	StartMonitoring(workflowID string) error
	StopMonitoring(workflowID string) error
	GetStatus(workflowID string) (*WorkflowStatus, error)
	GetMetrics() map[string]interface{}
}

// WorkflowRollback handles rollback operations
type WorkflowRollback interface {
	CanRollback(workflowID string) bool
	InitiateRollback(ctx context.Context, workflowID string) error
	GetRollbackPlan(workflowID string) (*RollbackPlan, error)
	ExecuteRollback(ctx context.Context, plan *RollbackPlan) error
}

// Workflow represents a workflow instance
type Workflow struct {
	ID        string                        `json:"id"`
	AlertID   string                        `json:"alert_id"`
	Status    string                        `json:"status"`
	Actions   []*types.ActionRecommendation `json:"actions"`
	CreatedAt time.Time                     `json:"created_at"`
	UpdatedAt time.Time                     `json:"updated_at"`
	Metadata  map[string]interface{}        `json:"metadata"`
}

// WorkflowState represents the current state of a workflow
type WorkflowState struct {
	WorkflowID       string                 `json:"workflow_id"`
	CurrentStep      int                    `json:"current_step"`
	CompletedActions []string               `json:"completed_actions"`
	PendingActions   []string               `json:"pending_actions"`
	FailedActions    []string               `json:"failed_actions"`
	LastUpdated      time.Time              `json:"last_updated"`
	ExecutionContext map[string]interface{} `json:"execution_context"`
}

// WorkflowStatus represents the current status of a workflow
type WorkflowStatus struct {
	WorkflowID          string                 `json:"workflow_id"`
	Status              string                 `json:"status"`
	Progress            float64                `json:"progress"`
	CurrentAction       string                 `json:"current_action"`
	EstimatedCompletion *time.Time             `json:"estimated_completion,omitempty"`
	Metrics             map[string]interface{} `json:"metrics"`
}

// RollbackPlan represents a plan for rolling back a workflow
type RollbackPlan struct {
	WorkflowID      string                        `json:"workflow_id"`
	RollbackActions []*types.ActionRecommendation `json:"rollback_actions"`
	Order           []string                      `json:"order"`
	EstimatedTime   time.Duration                 `json:"estimated_time"`
}
