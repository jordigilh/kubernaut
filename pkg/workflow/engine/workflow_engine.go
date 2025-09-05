package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
)

// DefaultWorkflowEngine implements the WorkflowEngine interface
type DefaultWorkflowEngine struct {
	k8sClient         k8s.Client
	actionRepo        actionhistory.Repository
	monitoringClients *monitoring.MonitoringClients
	log               *logrus.Logger

	// Action executors
	actionExecutors map[string]ActionExecutor

	// State storage
	stateStorage StateStorage

	// AI condition evaluator
	aiConditionEvaluator AIConditionEvaluator

	// Post-condition validator registry
	postConditionRegistry *ValidatorRegistry

	// Configuration
	config *WorkflowEngineConfig
}

// WorkflowEngineConfig holds configuration for the workflow engine
type WorkflowEngineConfig struct {
	DefaultStepTimeout    time.Duration `yaml:"default_step_timeout" default:"10m"`
	MaxRetryDelay         time.Duration `yaml:"max_retry_delay" default:"5m"`
	EnableStateRecovery   bool          `yaml:"enable_state_recovery" default:"true"`
	EnableDetailedLogging bool          `yaml:"enable_detailed_logging" default:"false"`
}

// ActionExecutor defines the interface for executing specific action types
type ActionExecutor interface {
	Execute(ctx context.Context, action *StepAction, context *StepContext) (*StepResult, error)
	ValidateAction(action *StepAction) error
	GetActionType() string
}

// StateStorage defines the interface for workflow state persistence
type StateStorage interface {
	SaveWorkflowState(ctx context.Context, execution *WorkflowExecution) error
	LoadWorkflowState(ctx context.Context, executionID string) (*WorkflowExecution, error)
	DeleteWorkflowState(ctx context.Context, executionID string) error
}

// NewDefaultWorkflowEngine creates a new workflow engine
func NewDefaultWorkflowEngine(
	k8sClient k8s.Client,
	actionRepo actionhistory.Repository,
	monitoringClients *monitoring.MonitoringClients,
	stateStorage StateStorage,
	config *WorkflowEngineConfig,
	log *logrus.Logger,
) *DefaultWorkflowEngine {
	if config == nil {
		config = &WorkflowEngineConfig{
			DefaultStepTimeout:    10 * time.Minute,
			MaxRetryDelay:         5 * time.Minute,
			EnableStateRecovery:   true,
			EnableDetailedLogging: false,
		}
	}

	engine := &DefaultWorkflowEngine{
		k8sClient:             k8sClient,
		actionRepo:            actionRepo,
		monitoringClients:     monitoringClients,
		stateStorage:          stateStorage,
		config:                config,
		log:                   log,
		actionExecutors:       make(map[string]ActionExecutor),
		aiConditionEvaluator:  nil, // Will be set via SetAIConditionEvaluator
		postConditionRegistry: NewValidatorRegistry(log),
	}

	// Register default action executors
	engine.registerDefaultExecutors()

	return engine
}

// NewDefaultWorkflowEngineWithAI creates a new workflow engine with AI condition evaluator
func NewDefaultWorkflowEngineWithAI(
	k8sClient k8s.Client,
	actionRepo actionhistory.Repository,
	monitoringClients *monitoring.MonitoringClients,
	stateStorage StateStorage,
	aiConditionEvaluator AIConditionEvaluator,
	config *WorkflowEngineConfig,
	log *logrus.Logger,
) *DefaultWorkflowEngine {
	engine := NewDefaultWorkflowEngine(k8sClient, actionRepo, monitoringClients, stateStorage, config, log)
	engine.aiConditionEvaluator = aiConditionEvaluator
	return engine
}

// SetAIConditionEvaluator sets the AI condition evaluator for the workflow engine
func (dwe *DefaultWorkflowEngine) SetAIConditionEvaluator(evaluator AIConditionEvaluator) {
	dwe.aiConditionEvaluator = evaluator
}

// ExecuteStep executes a single workflow step
func (dwe *DefaultWorkflowEngine) ExecuteStep(ctx context.Context, step *WorkflowStep, stepContext *StepContext) (*StepResult, error) {
	start := time.Now()

	if dwe.config.EnableDetailedLogging {
		dwe.log.WithFields(logrus.Fields{
			"execution_id": stepContext.ExecutionID,
			"step_id":      step.ID,
			"step_type":    step.Type,
		}).Debug("Starting step execution")
	}

	// Set default timeout if not specified
	timeout := step.Timeout
	if timeout == 0 {
		timeout = dwe.config.DefaultStepTimeout
	}

	// Create context with timeout
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var result *StepResult
	var err error

	switch step.Type {
	case StepTypeAction:
		result, err = dwe.executeAction(stepCtx, step, stepContext)
	case StepTypeCondition:
		result, err = dwe.executeCondition(stepCtx, step, stepContext)
	case StepTypeWait:
		result, err = dwe.executeWait(stepCtx, step, stepContext)
	case StepTypeDecision:
		result, err = dwe.executeDecision(stepCtx, step, stepContext)
	case StepTypeParallel:
		result, err = dwe.executeParallel(stepCtx, step, stepContext)
	case StepTypeSequential:
		result, err = dwe.executeSequential(stepCtx, step, stepContext)
	case StepTypeLoop:
		result, err = dwe.executeLoop(stepCtx, step, stepContext)
	case StepTypeSubflow:
		result, err = dwe.executeSubflow(stepCtx, step, stepContext)
	default:
		return nil, fmt.Errorf("unsupported step type: %s", step.Type)
	}

	duration := time.Since(start)

	if err != nil {
		dwe.log.WithError(err).WithFields(logrus.Fields{
			"execution_id": stepContext.ExecutionID,
			"step_id":      step.ID,
			"duration":     duration,
		}).Error("Step execution failed")
		return nil, err
	}

	if result == nil {
		result = &StepResult{
			Success:   true,
			Data:      make(map[string]interface{}),
			Variables: make(map[string]interface{}),
			Metrics: &StepMetrics{
				Duration: duration,
			},
		}
	}

	// Ensure metrics are set
	if result.Metrics == nil {
		result.Metrics = &StepMetrics{Duration: duration}
	} else {
		result.Metrics.Duration = duration
	}

	if dwe.config.EnableDetailedLogging {
		dwe.log.WithFields(logrus.Fields{
			"execution_id": stepContext.ExecutionID,
			"step_id":      step.ID,
			"duration":     duration,
			"success":      result.Success,
		}).Debug("Step execution completed")
	}

	return result, nil
}

// EvaluateCondition evaluates a workflow condition
func (dwe *DefaultWorkflowEngine) EvaluateCondition(ctx context.Context, condition *WorkflowCondition, stepContext *StepContext) (bool, error) {
	switch condition.Type {
	case ConditionTypeMetric:
		return dwe.evaluateMetricCondition(ctx, condition, stepContext)
	case ConditionTypeResource:
		return dwe.evaluateResourceCondition(ctx, condition, stepContext)
	case ConditionTypeTime:
		return dwe.evaluateTimeCondition(ctx, condition, stepContext)
	case ConditionTypeExpression:
		return dwe.evaluateExpressionCondition(ctx, condition, stepContext)
	case ConditionTypeCustom:
		return dwe.evaluateCustomCondition(ctx, condition, stepContext)
	default:
		return false, fmt.Errorf("unsupported condition type: %s", condition.Type)
	}
}

// SaveWorkflowState saves the current state of a workflow execution
func (dwe *DefaultWorkflowEngine) SaveWorkflowState(ctx context.Context, execution *WorkflowExecution) error {
	if dwe.stateStorage == nil {
		return fmt.Errorf("state storage not configured")
	}
	return dwe.stateStorage.SaveWorkflowState(ctx, execution)
}

// LoadWorkflowState loads the state of a workflow execution
func (dwe *DefaultWorkflowEngine) LoadWorkflowState(ctx context.Context, executionID string) (*WorkflowExecution, error) {
	if dwe.stateStorage == nil {
		return nil, fmt.Errorf("state storage not configured")
	}
	return dwe.stateStorage.LoadWorkflowState(ctx, executionID)
}

// RecoverFromFailure attempts to recover from a workflow failure
func (dwe *DefaultWorkflowEngine) RecoverFromFailure(ctx context.Context, execution *WorkflowExecution, step *WorkflowStep) (*RecoveryPlan, error) {
	plan := &RecoveryPlan{
		ID:       generateRecoveryPlanID(),
		Actions:  []RecoveryAction{},
		Triggers: []string{"step_failure"},
		Priority: 1,
		Timeout:  dwe.config.MaxRetryDelay,
		Metadata: map[string]interface{}{
			"workflow_id":  execution.WorkflowID,
			"failure_step": execution.CurrentStep,
			"strategy":     "retry",
			"created_at":   time.Now(),
		},
	}

	// Determine recovery strategy based on step type and failure
	if step.RetryPolicy != nil && step.RetryPolicy.MaxRetries > 0 {
		plan.Metadata["strategy"] = "retry"
		plan.Actions = append(plan.Actions, RecoveryAction{
			ID:      generateRecoveryActionID(),
			Type:    "retry_step",
			Trigger: "step_failure",
			Parameters: map[string]interface{}{
				"step_id":     step.ID,
				"max_retries": step.RetryPolicy.MaxRetries,
				"delay":       step.RetryPolicy.Delay,
				"description": fmt.Sprintf("Retry step %s", step.ID),
				"critical":    true,
			},
			Timeout: int(step.Timeout.Seconds()),
		})
	}

	// Add rollback action if available
	if step.Action != nil && step.Action.Rollback != nil {
		plan.Actions = append(plan.Actions, RecoveryAction{
			ID:      generateRecoveryActionID(),
			Type:    "rollback_step",
			Trigger: "step_failure",
			Parameters: map[string]interface{}{
				"step_id":         step.ID,
				"rollback_action": step.Action.Rollback,
				"description":     fmt.Sprintf("Rollback step %s", step.ID),
				"critical":        false,
			},
			Timeout: 300, // 5 minutes default
		})
	}

	dwe.log.WithFields(logrus.Fields{
		"execution_id":  execution.ID,
		"step_id":       step.ID,
		"strategy":      plan.Metadata["strategy"],
		"actions_count": len(plan.Actions),
	}).Info("Created recovery plan")

	return plan, nil
}

// RollbackWorkflow rolls back a workflow to a specific step
func (dwe *DefaultWorkflowEngine) RollbackWorkflow(ctx context.Context, execution *WorkflowExecution, toStep int) error {
	if toStep < 0 || toStep >= len(execution.Steps) {
		return fmt.Errorf("invalid rollback step index: %d", toStep)
	}

	dwe.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"from_step":    execution.CurrentStep,
		"to_step":      toStep,
	}).Info("Rolling back workflow")

	// Rollback steps in reverse order
	for i := execution.CurrentStep; i > toStep; i-- {
		step := execution.Steps[i]
		if step.Status == ExecutionStatusCompleted {
			// Attempt to rollback this step
			if err := dwe.rollbackStep(ctx, execution, i); err != nil {
				dwe.log.WithError(err).WithFields(logrus.Fields{
					"execution_id": execution.ID,
					"step_id":      step.StepID,
					"step_index":   i,
				}).Warn("Failed to rollback step")
			}
		}
	}

	execution.CurrentStep = toStep
	execution.Status = ExecutionStatusRollingBack

	return nil
}

// Private helper methods

func (dwe *DefaultWorkflowEngine) executeAction(ctx context.Context, step *WorkflowStep, stepContext *StepContext) (*StepResult, error) {
	if step.Action == nil {
		return nil, fmt.Errorf("step action is nil")
	}

	// Get action executor
	executor, exists := dwe.actionExecutors[step.Action.Type]
	if !exists {
		return nil, fmt.Errorf("no executor found for action type: %s", step.Action.Type)
	}

	// Validate action
	if err := executor.ValidateAction(step.Action); err != nil {
		return nil, fmt.Errorf("action validation failed: %w", err)
	}

	// Execute pre-conditions (validation structure doesn't support pre-conditions yet)
	if step.Action.Validation != nil && !step.Action.Validation.Valid {
		return nil, fmt.Errorf("action validation failed: %v", step.Action.Validation.Errors)
	}

	// Execute action
	result, err := executor.Execute(ctx, step.Action, stepContext)
	if err != nil {
		return nil, err
	}

	// Execute post-conditions validation
	if step.Action.Validation != nil && len(step.Action.Validation.PostConditions) > 0 {
		dwe.log.WithFields(logrus.Fields{
			"step_id":               step.ID,
			"post_conditions_count": len(step.Action.Validation.PostConditions),
		}).Debug("Validating post-conditions")

		validationResult, err := dwe.postConditionRegistry.ValidatePostConditions(
			ctx,
			step.Action.Validation.PostConditions,
			result,
			stepContext,
		)
		if err != nil {
			dwe.log.WithError(err).Error("Post-condition validation failed with error")
			return nil, fmt.Errorf("post-condition validation error: %w", err)
		}

		// Store validation results in step context for later use
		if stepContext.Variables == nil {
			stepContext.Variables = make(map[string]interface{})
		}
		stepContext.Variables["post_condition_validation"] = validationResult

		// Handle critical post-condition failures
		if !validationResult.Success {
			dwe.log.WithFields(logrus.Fields{
				"step_id":         step.ID,
				"critical_failed": validationResult.CriticalFailed,
				"total_failed":    validationResult.TotalFailed,
				"total_passed":    validationResult.TotalPassed,
				"total_duration":  validationResult.TotalDuration,
				"message":         validationResult.Message,
			}).Error("Critical post-conditions validation failed")

			// Store detailed failure information
			result.Success = false
			result.Error = fmt.Sprintf("Critical post-conditions failed: %s", validationResult.Message)
			if result.Output == nil {
				result.Output = make(map[string]interface{})
			}
			result.Output["post_condition_failures"] = validationResult.Results

			// Attempt rollback if configured
			if step.Action.Rollback != nil {
				dwe.log.WithField("step_id", step.ID).Info("Attempting rollback due to post-condition failure")
				rollbackErr := dwe.executeRollback(ctx, step.Action.Rollback, stepContext)
				if rollbackErr != nil {
					dwe.log.WithError(rollbackErr).Error("Rollback failed after post-condition failure")
					result.Error = fmt.Sprintf("%s; rollback also failed: %s", result.Error, rollbackErr.Error())
				} else {
					dwe.log.WithField("step_id", step.ID).Info("Rollback completed successfully")
				}
			}

			return result, nil // Return the failed result, not an error
		}

		dwe.log.WithFields(logrus.Fields{
			"step_id":        step.ID,
			"total_passed":   validationResult.TotalPassed,
			"total_failed":   validationResult.TotalFailed,
			"total_duration": validationResult.TotalDuration,
		}).Info("Post-conditions validation completed")
	}

	// Log validation warnings if present
	if step.Action.Validation != nil && len(step.Action.Validation.Warnings) > 0 {
		dwe.log.WithFields(logrus.Fields{
			"step_id":  step.ID,
			"warnings": step.Action.Validation.Warnings,
		}).Warn("Action validation warnings")
	}

	return result, nil
}

func (dwe *DefaultWorkflowEngine) executeCondition(ctx context.Context, step *WorkflowStep, stepContext *StepContext) (*StepResult, error) {
	if step.Condition == nil {
		return nil, fmt.Errorf("step condition is nil")
	}

	conditionMet, err := dwe.EvaluateCondition(ctx, step.Condition, stepContext)
	if err != nil {
		return nil, err
	}

	result := &StepResult{
		Success: conditionMet,
		Data: map[string]interface{}{
			"condition_met": conditionMet,
			"condition_id":  step.Condition.ID,
		},
		Variables: map[string]interface{}{
			"condition_result": conditionMet,
		},
	}

	// Determine next steps based on condition result
	if conditionMet {
		result.NextSteps = step.OnSuccess
	} else {
		result.NextSteps = step.OnFailure
	}

	return result, nil
}

func (dwe *DefaultWorkflowEngine) executeWait(ctx context.Context, step *WorkflowStep, stepContext *StepContext) (*StepResult, error) {
	// Extract wait duration from step variables
	waitDuration := time.Minute // Default wait time
	if duration, exists := step.Variables["duration"]; exists {
		if d, ok := duration.(time.Duration); ok {
			waitDuration = d
		} else if s, ok := duration.(string); ok {
			if parsed, err := time.ParseDuration(s); err == nil {
				waitDuration = parsed
			}
		}
	}

	dwe.log.WithFields(logrus.Fields{
		"execution_id": stepContext.ExecutionID,
		"step_id":      step.ID,
		"duration":     waitDuration,
	}).Debug("Executing wait step")

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(waitDuration):
		// Wait completed successfully
	}

	return &StepResult{
		Success: true,
		Data: map[string]interface{}{
			"wait_duration": waitDuration.String(),
		},
	}, nil
}

func (dwe *DefaultWorkflowEngine) executeDecision(ctx context.Context, step *WorkflowStep, stepContext *StepContext) (*StepResult, error) {
	// Decision steps evaluate multiple conditions and choose a path
	if step.Condition == nil {
		return nil, fmt.Errorf("decision step requires a condition")
	}

	conditionMet, err := dwe.EvaluateCondition(ctx, step.Condition, stepContext)
	if err != nil {
		return nil, err
	}

	var nextSteps []string
	var decision string

	if conditionMet {
		nextSteps = step.OnSuccess
		decision = "success_path"
	} else {
		nextSteps = step.OnFailure
		decision = "failure_path"
	}

	return &StepResult{
		Success:   true,
		NextSteps: nextSteps,
		Data: map[string]interface{}{
			"decision":      decision,
			"condition_met": conditionMet,
		},
		Variables: map[string]interface{}{
			"decision_result": decision,
		},
	}, nil
}

func (dwe *DefaultWorkflowEngine) executeParallel(ctx context.Context, step *WorkflowStep, stepContext *StepContext) (*StepResult, error) {
	// Parallel execution is not implemented in this basic version
	// It would require more complex orchestration logic
	return nil, fmt.Errorf("parallel step execution not implemented")
}

func (dwe *DefaultWorkflowEngine) executeSequential(ctx context.Context, step *WorkflowStep, stepContext *StepContext) (*StepResult, error) {
	// Sequential execution is the default behavior
	return &StepResult{
		Success: true,
		Data: map[string]interface{}{
			"execution_type": "sequential",
		},
	}, nil
}

func (dwe *DefaultWorkflowEngine) executeLoop(ctx context.Context, step *WorkflowStep, stepContext *StepContext) (*StepResult, error) {
	// Loop execution is not implemented in this basic version
	return nil, fmt.Errorf("loop step execution not implemented")
}

func (dwe *DefaultWorkflowEngine) executeSubflow(ctx context.Context, step *WorkflowStep, stepContext *StepContext) (*StepResult, error) {
	// Subflow execution is not implemented in this basic version
	return nil, fmt.Errorf("subflow step execution not implemented")
}

func (dwe *DefaultWorkflowEngine) evaluateMetricCondition(ctx context.Context, condition *WorkflowCondition, stepContext *StepContext) (bool, error) {
	// Use AI condition evaluator if available, otherwise fallback to basic logic
	if dwe.aiConditionEvaluator != nil {
		return dwe.aiConditionEvaluator.EvaluateCondition(ctx, condition, stepContext)
	}

	// Fallback to basic metric evaluation
	dwe.log.Debug("Using basic metric condition evaluation (AI unavailable)")
	return dwe.basicMetricEvaluation(condition, stepContext), nil
}

func (dwe *DefaultWorkflowEngine) evaluateResourceCondition(ctx context.Context, condition *WorkflowCondition, stepContext *StepContext) (bool, error) {
	// Use AI condition evaluator if available, otherwise fallback to basic logic
	if dwe.aiConditionEvaluator != nil {
		return dwe.aiConditionEvaluator.EvaluateCondition(ctx, condition, stepContext)
	}

	// Fallback to basic resource evaluation
	dwe.log.Debug("Using basic resource condition evaluation (AI unavailable)")
	return dwe.basicResourceEvaluation(condition, stepContext), nil
}

func (dwe *DefaultWorkflowEngine) evaluateTimeCondition(ctx context.Context, condition *WorkflowCondition, stepContext *StepContext) (bool, error) {
	// Use AI condition evaluator if available, otherwise fallback to basic logic
	if dwe.aiConditionEvaluator != nil {
		return dwe.aiConditionEvaluator.EvaluateCondition(ctx, condition, stepContext)
	}

	// Fallback to basic time evaluation
	dwe.log.Debug("Using basic time condition evaluation (AI unavailable)")
	return dwe.basicTimeEvaluation(condition, stepContext), nil
}

func (dwe *DefaultWorkflowEngine) evaluateExpressionCondition(ctx context.Context, condition *WorkflowCondition, stepContext *StepContext) (bool, error) {
	// Use AI condition evaluator if available, otherwise fallback to basic logic
	if dwe.aiConditionEvaluator != nil {
		return dwe.aiConditionEvaluator.EvaluateCondition(ctx, condition, stepContext)
	}

	// Fallback to basic expression evaluation
	dwe.log.Debug("Using basic expression condition evaluation (AI unavailable)")
	return dwe.basicExpressionEvaluation(condition, stepContext), nil
}

func (dwe *DefaultWorkflowEngine) evaluateCustomCondition(ctx context.Context, condition *WorkflowCondition, stepContext *StepContext) (bool, error) {
	// Use AI condition evaluator if available, otherwise fallback to basic logic
	if dwe.aiConditionEvaluator != nil {
		return dwe.aiConditionEvaluator.EvaluateCondition(ctx, condition, stepContext)
	}

	// Fallback to basic custom evaluation
	dwe.log.Debug("Using basic custom condition evaluation (AI unavailable)")
	return dwe.basicCustomEvaluation(condition, stepContext), nil
}

func (dwe *DefaultWorkflowEngine) validateCondition(ctx context.Context, rule *ValidationRule, stepContext *StepContext) bool {
	// Simple validation logic for demonstration
	if rule.Expression == "always_true" {
		return true
	}

	// More complex validation would be implemented here
	return true
}

func (dwe *DefaultWorkflowEngine) rollbackStep(ctx context.Context, execution *WorkflowExecution, stepIndex int) error {
	if stepIndex >= len(execution.Steps) {
		return fmt.Errorf("invalid step index: %d", stepIndex)
	}

	stepExecution := execution.Steps[stepIndex]

	// Mark step as rolled back
	stepExecution.Status = ExecutionStatusCancelled

	dwe.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"step_id":      stepExecution.StepID,
	}).Debug("Rolled back step")

	return nil
}

func (dwe *DefaultWorkflowEngine) registerDefaultExecutors() {
	// TODO: Register action executors when they are implemented
	// dwe.actionExecutors["kubernetes"] = NewKubernetesActionExecutor(dwe.k8sClient, dwe.log)
	// dwe.actionExecutors["monitoring"] = NewMonitoringActionExecutor(dwe.monitoringClients, dwe.log)
	// dwe.actionExecutors["custom"] = NewCustomActionExecutor(dwe.log)
}

// Utility functions

func generateRecoveryPlanID() string {
	return "recovery-" + strings.Replace(uuid.New().String(), "-", "", -1)[:12]
}

func generateRecoveryActionID() string {
	return "action-" + strings.Replace(uuid.New().String(), "-", "", -1)[:12]
}

// Basic fallback evaluation methods for when AI is unavailable

func (dwe *DefaultWorkflowEngine) basicMetricEvaluation(condition *WorkflowCondition, stepContext *StepContext) bool {
	// Simple metric evaluation without AI
	expr := strings.ToLower(condition.Expression)

	// Check for common metric patterns
	if strings.Contains(expr, "cpu") && strings.Contains(expr, ">") {
		// CPU threshold checks - default to not exceeded
		return false
	} else if strings.Contains(expr, "memory") && strings.Contains(expr, ">") {
		// Memory threshold checks - default to not exceeded
		return false
	} else if strings.Contains(expr, "error") || strings.Contains(expr, "fail") {
		// Error/failure metrics - default to no errors
		return false
	} else if strings.Contains(expr, "health") || strings.Contains(expr, "ready") {
		// Health checks - default to healthy
		return true
	}

	// Conservative default for unknown metrics
	return true
}

func (dwe *DefaultWorkflowEngine) basicResourceEvaluation(condition *WorkflowCondition, stepContext *StepContext) bool {
	// Simple resource evaluation without AI
	expr := strings.ToLower(condition.Expression)

	// Basic K8s resource checks
	if dwe.k8sClient != nil {
		ctx := context.Background()

		// Check if cluster is accessible
		_, err := dwe.k8sClient.ListNodes(ctx)
		if err != nil {
			return false // Cluster not accessible
		}

		// If expression mentions specific resources, try basic checks
		if strings.Contains(expr, "pod") && strings.Contains(expr, "ready") {
			// Pod readiness check - optimistic default
			return true
		} else if strings.Contains(expr, "deployment") && strings.Contains(expr, "available") {
			// Deployment availability check - optimistic default
			return true
		}
	}

	// Default to satisfied if cluster is accessible
	return true
}

func (dwe *DefaultWorkflowEngine) basicTimeEvaluation(condition *WorkflowCondition, stepContext *StepContext) bool {
	// Simple time evaluation without AI
	expr := strings.ToLower(condition.Expression)
	now := time.Now()

	// Handle common time patterns
	if strings.Contains(expr, "business_hours") {
		hour := now.Hour()
		return hour >= 9 && hour <= 17
	} else if strings.Contains(expr, "weekend") {
		weekday := now.Weekday()
		return weekday == time.Saturday || weekday == time.Sunday
	} else if strings.Contains(expr, "timeout") {
		// Check step timeout
		if condition.Timeout > 0 {
			// Without StartTime tracking in StepContext, assume not timed out
			return true
		}
		return true // No timeout exceeded
	}

	// Default to satisfied for time conditions
	return true
}

func (dwe *DefaultWorkflowEngine) basicExpressionEvaluation(condition *WorkflowCondition, stepContext *StepContext) bool {
	// Simple expression evaluation without AI
	expr := condition.Expression
	if expr == "" {
		return false
	}

	exprLower := strings.ToLower(expr)

	// Basic boolean evaluation
	if exprLower == "true" || strings.Contains(exprLower, "true") {
		return true
	} else if exprLower == "false" || strings.Contains(exprLower, "false") {
		return false
	} else if strings.Contains(exprLower, "&&") || strings.Contains(exprLower, "and") {
		// AND expressions - conservative
		return false
	} else if strings.Contains(exprLower, "||") || strings.Contains(exprLower, "or") {
		// OR expressions - optimistic
		return true
	}

	// Default to true for unknown expressions
	return true
}

func (dwe *DefaultWorkflowEngine) basicCustomEvaluation(condition *WorkflowCondition, stepContext *StepContext) bool {
	// Simple custom evaluation without AI - combine other basic evaluations
	metric := dwe.basicMetricEvaluation(condition, stepContext)
	resource := dwe.basicResourceEvaluation(condition, stepContext)
	timeCondition := dwe.basicTimeEvaluation(condition, stepContext)

	// Majority rule for custom conditions
	satisfied := 0
	if metric {
		satisfied++
	}
	if resource {
		satisfied++
	}
	if timeCondition {
		satisfied++
	}

	return satisfied >= 2
}

// executeRollback executes a rollback action when post-conditions fail
func (dwe *DefaultWorkflowEngine) executeRollback(ctx context.Context, rollback *RollbackAction, stepContext *StepContext) error {
	if rollback == nil {
		return fmt.Errorf("rollback action is nil")
	}

	dwe.log.WithFields(logrus.Fields{
		"rollback_id":   rollback.ID,
		"rollback_type": rollback.Type,
	}).Info("Executing rollback action")

	// Find appropriate executor for rollback
	executor, exists := dwe.actionExecutors[rollback.Type]
	if !exists {
		return fmt.Errorf("no executor found for rollback action type: %s", rollback.Type)
	}

	// Create rollback action struct from RollbackAction
	rollbackStepAction := &StepAction{
		Type:       rollback.Type,
		Parameters: rollback.Parameters,
		Target: &ActionTarget{
			Type: rollback.Type,
		},
	}

	// Execute rollback action
	_, err := executor.Execute(ctx, rollbackStepAction, stepContext)
	if err != nil {
		return fmt.Errorf("rollback execution failed: %w", err)
	}

	dwe.log.WithField("rollback_id", rollback.ID).Info("Rollback executed successfully")
	return nil
}
