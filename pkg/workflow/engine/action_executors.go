package engine

import (
	"context"
)

// ActionHandler defines the function signature for action handlers
type ActionHandler func(ctx context.Context, action *StepAction, stepContext *StepContext) (*StepResult, error)

// Note: The specific ActionExecutor implementations (KubernetesActionExecutor,
// MonitoringActionExecutor, CustomActionExecutor) have been moved to separate files:
// - kubernetes_action_executor.go
// - monitoring_action_executor.go
// - custom_action_executor.go
// This prevents duplicate declarations and follows better code organization.
