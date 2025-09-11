package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/security"
	"github.com/sirupsen/logrus"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

// securityContextKey is the key for storing security context in context.Context
const securityContextKey contextKey = "security_context"

// SecuredWorkflowEngine wraps workflow execution with RBAC security
type SecuredWorkflowEngine struct {
	engine       WorkflowEngine
	rbacProvider security.RBACProvider
	logger       *logrus.Logger
}

// NewSecuredWorkflowEngine creates a workflow engine with RBAC protection
func NewSecuredWorkflowEngine(
	baseEngine WorkflowEngine,
	rbacProvider security.RBACProvider,
	logger *logrus.Logger,
) *SecuredWorkflowEngine {
	return &SecuredWorkflowEngine{
		engine:       baseEngine,
		rbacProvider: rbacProvider,
		logger:       logger,
	}
}

// ExecuteWorkflow executes a workflow with security checks
func (s *SecuredWorkflowEngine) ExecuteWorkflow(ctx context.Context, workflow *Workflow) (*RuntimeWorkflowExecution, error) {
	// Extract security context from request
	securityCtx := extractSecurityContext(ctx)
	if securityCtx == nil {
		return nil, fmt.Errorf("security context required for workflow execution")
	}

	// Check basic workflow execution permission
	hasPermission, err := s.rbacProvider.HasPermission(
		ctx,
		securityCtx.Subject,
		security.PermissionExecuteWorkflow,
		fmt.Sprintf("workflow:%s", workflow.Name),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to check workflow execution permission: %w", err)
	}

	if !hasPermission {
		return nil, fmt.Errorf("subject %s does not have permission to execute workflows", securityCtx.Subject.Identifier)
	}

	// Pre-validate all actions in the workflow
	err = s.validateWorkflowActions(ctx, workflow, securityCtx.Subject)
	if err != nil {
		return nil, fmt.Errorf("workflow validation failed: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"workflow":   workflow.Name,
		"subject":    securityCtx.Subject.Identifier,
		"step_count": len(workflow.Template.Steps),
	}).Info("Starting secured workflow execution")

	// Execute the workflow with the base engine
	execution, err := s.engine.Execute(ctx, workflow)
	if err != nil {
		return nil, fmt.Errorf("workflow execution failed: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"workflow":     workflow.Name,
		"execution_id": execution.ID,
		"status":       execution.Status,
	}).Info("Completed secured workflow execution")

	return execution, nil
}

// validateWorkflowActions validates that the subject has permissions for all actions in the workflow
func (s *SecuredWorkflowEngine) validateWorkflowActions(ctx context.Context, workflow *Workflow, subject security.Subject) error {
	for i, step := range workflow.Template.Steps {
		if step.Type == StepTypeAction && step.Action != nil {
			permission, err := s.getRequiredPermissionForAction(step.Action.Type)
			if err != nil {
				return fmt.Errorf("step %d: %w", i, err)
			}

			resource := fmt.Sprintf("%s:%s", step.Action.Target.Namespace, step.Action.Target.Name)
			hasPermission, err := s.rbacProvider.HasPermission(ctx, subject, permission, resource)
			if err != nil {
				return fmt.Errorf("step %d: failed to check permission: %w", i, err)
			}

			if !hasPermission {
				return fmt.Errorf("step %d: subject %s does not have permission %s for resource %s",
					i, subject.Identifier, permission, resource)
			}
		}
	}

	return nil
}

// getRequiredPermissionForAction maps action types to required permissions
func (s *SecuredWorkflowEngine) getRequiredPermissionForAction(actionType string) (security.Permission, error) {
	actionPermissions := map[string]security.Permission{
		"pod_restart":            security.PermissionRestartPod,
		"pod_delete":             security.PermissionRestartPod,
		"scale_deployment":       security.PermissionScaleDeployment,
		"restart_deployment":     security.PermissionScaleDeployment,
		"drain_node":             security.PermissionDrainNode,
		"cordon_node":            security.PermissionDrainNode,
		"uncordon_node":          security.PermissionDrainNode,
		"update_resource_limits": security.PermissionUpdateResourceLimits,
		"create_configmap":       security.PermissionUpdateResourceLimits,
	}

	permission, exists := actionPermissions[actionType]
	if !exists {
		return "", fmt.Errorf("unknown action type: %s", actionType)
	}

	return permission, nil
}

// SecuredActionExecutor wraps action executors with per-action security checks
type SecuredActionExecutor struct {
	executor     ActionExecutor
	rbacProvider security.RBACProvider
	logger       *logrus.Logger
}

// NewSecuredActionExecutor creates a secured action executor
func NewSecuredActionExecutor(
	baseExecutor ActionExecutor,
	rbacProvider security.RBACProvider,
	logger *logrus.Logger,
) *SecuredActionExecutor {
	return &SecuredActionExecutor{
		executor:     baseExecutor,
		rbacProvider: rbacProvider,
		logger:       logger,
	}
}

// Execute executes an action with security validation
func (s *SecuredActionExecutor) Execute(ctx context.Context, action *StepAction, stepContext *StepContext) (*StepResult, error) {
	// Extract security context
	securityCtx := extractSecurityContext(ctx)
	if securityCtx == nil {
		return &StepResult{
			Success: false,
			Error:   "security context required for action execution",
		}, nil
	}

	// Get required permission for this action
	permission, err := s.getRequiredPermissionForAction(action.Type)
	if err != nil {
		return &StepResult{
			Success: false,
			Error:   fmt.Sprintf("failed to determine required permission: %v", err),
		}, nil
	}

	// Check permission
	resource := fmt.Sprintf("%s:%s", action.Target.Namespace, action.Target.Name)
	hasPermission, err := s.rbacProvider.HasPermission(ctx, securityCtx.Subject, permission, resource)
	if err != nil {
		return &StepResult{
			Success: false,
			Error:   fmt.Sprintf("failed to check permission: %v", err),
		}, nil
	}

	if !hasPermission {
		s.logger.WithFields(logrus.Fields{
			"action":     action.Type,
			"subject":    securityCtx.Subject.Identifier,
			"resource":   resource,
			"permission": permission,
		}).Warn("Action execution denied due to insufficient permissions")

		return &StepResult{
			Success: false,
			Error:   fmt.Sprintf("insufficient permissions for action %s on resource %s", action.Type, resource),
		}, nil
	}

	s.logger.WithFields(logrus.Fields{
		"action":   action.Type,
		"subject":  securityCtx.Subject.Identifier,
		"resource": resource,
	}).Debug("Action execution authorized")

	// Execute the action with the base executor
	return s.executor.Execute(ctx, action, stepContext)
}

// ValidateAction validates an action
func (s *SecuredActionExecutor) ValidateAction(action *StepAction) error {
	return s.executor.ValidateAction(action)
}

// GetActionType returns the action type
func (s *SecuredActionExecutor) GetActionType() string {
	return s.executor.GetActionType()
}

// getRequiredPermissionForAction maps action types to required permissions (duplicate from above)
func (s *SecuredActionExecutor) getRequiredPermissionForAction(actionType string) (security.Permission, error) {
	actionPermissions := map[string]security.Permission{
		"pod_restart":            security.PermissionRestartPod,
		"pod_delete":             security.PermissionRestartPod,
		"scale_deployment":       security.PermissionScaleDeployment,
		"restart_deployment":     security.PermissionScaleDeployment,
		"drain_node":             security.PermissionDrainNode,
		"cordon_node":            security.PermissionDrainNode,
		"uncordon_node":          security.PermissionDrainNode,
		"update_resource_limits": security.PermissionUpdateResourceLimits,
		"create_configmap":       security.PermissionUpdateResourceLimits,
	}

	permission, exists := actionPermissions[actionType]
	if !exists {
		return "", fmt.Errorf("unknown action type: %s", actionType)
	}

	return permission, nil
}

// Helper functions for security context management

// WithSecurityContext adds security context to a context
func WithSecurityContext(ctx context.Context, securityCtx *security.SecurityContext) context.Context {
	return context.WithValue(ctx, securityContextKey, securityCtx)
}

// extractSecurityContext extracts security context from a context
func extractSecurityContext(ctx context.Context) *security.SecurityContext {
	if securityCtx, ok := ctx.Value(securityContextKey).(*security.SecurityContext); ok {
		return securityCtx
	}
	return nil
}

// CreateSecurityContext creates a security context for a user/service
func CreateSecurityContext(userID, userType, namespace, resource, action string) *security.SecurityContext {
	return &security.SecurityContext{
		Subject: security.Subject{
			Type:       userType,
			Identifier: userID,
		},
		Namespace: namespace,
		Resource:  resource,
		Action:    action,
		RequestID: generateRequestID(),
		Timestamp: time.Now(),
		Metadata:  make(map[string]string),
	}
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	// Simple implementation - in production would use proper UUID generation
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// SecurityMetrics tracks security-related metrics
type SecurityMetrics struct {
	TotalRequests        int64 `json:"total_requests"`
	AuthorizedRequests   int64 `json:"authorized_requests"`
	UnauthorizedRequests int64 `json:"unauthorized_requests"`
	PermissionChecks     int64 `json:"permission_checks"`
	FailedChecks         int64 `json:"failed_checks"`
}

// GetSecurityMetrics returns security metrics for monitoring
func (s *SecuredWorkflowEngine) GetSecurityMetrics() *SecurityMetrics {
	// In a real implementation, this would maintain actual counters
	return &SecurityMetrics{
		TotalRequests:        0,
		AuthorizedRequests:   0,
		UnauthorizedRequests: 0,
		PermissionChecks:     0,
		FailedChecks:         0,
	}
}
