package conditions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// EvaluateMetricCondition uses AI to evaluate metric-based conditions
func (ace *DefaultAIConditionEvaluator) EvaluateMetricCondition(ctx context.Context, condition *engine.WorkflowCondition, stepContext *engine.StepContext) (*ConditionResult, error) {
	// Gather current metric context
	evalContext, err := ace.gatherMetricContext(ctx, condition, stepContext)
	if err != nil {
		return nil, fmt.Errorf("failed to gather metric context: %w", err)
	}

	// Build AI analysis request
	prompt := ace.buildMetricEvaluationPrompt(condition, stepContext, evalContext)

	// Create alert for SLM analysis
	alert := types.Alert{
		Name:        "metric_condition_evaluation",
		Description: "AI-powered metric condition evaluation for workflow orchestration",
		Labels: map[string]string{
			"condition_type": string(condition.Type),
			"condition_id":   condition.ID,
			"execution_id":   stepContext.ExecutionID,
			"step_id":        stepContext.StepID,
		},
		Annotations: map[string]string{
			"prompt":               prompt,
			"condition_expression": condition.Expression,
			"context":              "orchestration_condition_evaluation",
		},
		Severity: "info",
		Status:   "firing",
		StartsAt: time.Now(),
	}

	// Get AI analysis
	recommendation, err := ace.slmClient.AnalyzeAlert(ctx, alert)
	if err != nil {
		return ace.fallbackMetricEvaluation(condition, stepContext), nil
	}

	// Parse AI response
	result, err := ace.parseConditionResponse(recommendation, condition)
	if err != nil {
		return ace.fallbackMetricEvaluation(condition, stepContext), nil
	}

	return result, nil
}

// EvaluateResourceCondition uses AI to evaluate Kubernetes resource conditions
func (ace *DefaultAIConditionEvaluator) EvaluateResourceCondition(ctx context.Context, condition *engine.WorkflowCondition, stepContext *engine.StepContext) (*ConditionResult, error) {
	// Gather current resource context
	evalContext, err := ace.gatherResourceContext(ctx, condition, stepContext)
	if err != nil {
		return nil, fmt.Errorf("failed to gather resource context: %w", err)
	}

	// Build AI analysis request
	prompt := ace.buildResourceEvaluationPrompt(condition, stepContext, evalContext)

	alert := types.Alert{
		Name:        "resource_condition_evaluation",
		Description: "AI-powered Kubernetes resource condition evaluation",
		Labels: map[string]string{
			"condition_type": string(condition.Type),
			"condition_id":   condition.ID,
			"execution_id":   stepContext.ExecutionID,
			"step_id":        stepContext.StepID,
		},
		Annotations: map[string]string{
			"prompt":               prompt,
			"condition_expression": condition.Expression,
			"context":              "orchestration_condition_evaluation",
		},
		Severity: "info",
		Status:   "firing",
		StartsAt: time.Now(),
	}

	recommendation, err := ace.slmClient.AnalyzeAlert(ctx, alert)
	if err != nil {
		return ace.fallbackResourceEvaluation(condition, stepContext), nil
	}

	result, err := ace.parseConditionResponse(recommendation, condition)
	if err != nil {
		return ace.fallbackResourceEvaluation(condition, stepContext), nil
	}

	return result, nil
}

// EvaluateTimeCondition uses AI to evaluate time-based conditions
func (ace *DefaultAIConditionEvaluator) EvaluateTimeCondition(ctx context.Context, condition *engine.WorkflowCondition, stepContext *engine.StepContext) (*ConditionResult, error) {
	// Gather temporal context
	evalContext, err := ace.gatherTimeContext(ctx, condition, stepContext)
	if err != nil {
		return nil, fmt.Errorf("failed to gather time context: %w", err)
	}

	prompt := ace.buildTimeEvaluationPrompt(condition, stepContext, evalContext)

	alert := types.Alert{
		Name:        "time_condition_evaluation",
		Description: "AI-powered time-based condition evaluation",
		Labels: map[string]string{
			"condition_type": string(condition.Type),
			"condition_id":   condition.ID,
			"execution_id":   stepContext.ExecutionID,
		},
		Annotations: map[string]string{
			"prompt":               prompt,
			"condition_expression": condition.Expression,
			"context":              "orchestration_condition_evaluation",
		},
		Severity: "info",
		Status:   "firing",
		StartsAt: time.Now(),
	}

	recommendation, err := ace.slmClient.AnalyzeAlert(ctx, alert)
	if err != nil {
		return ace.fallbackTimeEvaluation(condition, stepContext), nil
	}

	result, err := ace.parseConditionResponse(recommendation, condition)
	if err != nil {
		return ace.fallbackTimeEvaluation(condition, stepContext), nil
	}

	return result, nil
}

// EvaluateExpressionCondition uses AI to parse and evaluate complex expressions
func (ace *DefaultAIConditionEvaluator) EvaluateExpressionCondition(ctx context.Context, condition *engine.WorkflowCondition, stepContext *engine.StepContext) (*ConditionResult, error) {
	// Gather expression evaluation context
	evalContext, err := ace.gatherExpressionContext(ctx, condition, stepContext)
	if err != nil {
		return nil, fmt.Errorf("failed to gather expression context: %w", err)
	}

	prompt := ace.buildExpressionEvaluationPrompt(condition, stepContext, evalContext)

	alert := types.Alert{
		Name:        "expression_condition_evaluation",
		Description: "AI-powered expression condition evaluation",
		Labels: map[string]string{
			"condition_type": string(condition.Type),
			"condition_id":   condition.ID,
			"expression":     condition.Expression,
		},
		Annotations: map[string]string{
			"prompt":               prompt,
			"condition_expression": condition.Expression,
			"context":              "orchestration_condition_evaluation",
		},
		Severity: "info",
		Status:   "firing",
		StartsAt: time.Now(),
	}

	recommendation, err := ace.slmClient.AnalyzeAlert(ctx, alert)
	if err != nil {
		return ace.fallbackExpressionEvaluation(condition, stepContext), nil
	}

	result, err := ace.parseConditionResponse(recommendation, condition)
	if err != nil {
		return ace.fallbackExpressionEvaluation(condition, stepContext), nil
	}

	return result, nil
}

// EvaluateCustomCondition uses AI for custom condition logic
func (ace *DefaultAIConditionEvaluator) EvaluateCustomCondition(ctx context.Context, condition *engine.WorkflowCondition, stepContext *engine.StepContext) (*ConditionResult, error) {
	// Gather comprehensive context for custom evaluation
	evalContext, err := ace.gatherCustomContext(ctx, condition, stepContext)
	if err != nil {
		return nil, fmt.Errorf("failed to gather custom context: %w", err)
	}

	prompt := ace.buildCustomEvaluationPrompt(condition, stepContext, evalContext)

	alert := types.Alert{
		Name:        "custom_condition_evaluation",
		Description: "AI-powered custom condition evaluation",
		Labels: map[string]string{
			"condition_type": string(condition.Type),
			"condition_id":   condition.ID,
			"execution_id":   stepContext.ExecutionID,
		},
		Annotations: map[string]string{
			"prompt":               prompt,
			"condition_expression": condition.Expression,
			"context":              "orchestration_condition_evaluation",
		},
		Severity: "info",
		Status:   "firing",
		StartsAt: time.Now(),
	}

	recommendation, err := ace.slmClient.AnalyzeAlert(ctx, alert)
	if err != nil {
		return ace.fallbackCustomEvaluation(condition, stepContext), nil
	}

	result, err := ace.parseConditionResponse(recommendation, condition)
	if err != nil {
		return ace.fallbackCustomEvaluation(condition, stepContext), nil
	}

	return result, nil
}

// Context gathering methods

func (ace *DefaultAIConditionEvaluator) gatherMetricContext(ctx context.Context, condition *engine.WorkflowCondition, stepContext *engine.StepContext) (*ConditionEvaluationContext, error) {
	evalContext := &ConditionEvaluationContext{
		CurrentMetrics:     make(map[string]interface{}),
		SystemLoad:         make(map[string]float64),
		EnvironmentContext: make(map[string]string),
	}

	// Gather metrics from monitoring clients if available
	if ace.monitoringClients != nil {
		// Get metrics from metrics client
		if metricsClient := ace.monitoringClients.MetricsClient; metricsClient != nil {
			// Try to extract resource info from condition for metrics query
			resourceInfo := ace.extractResourceInfoFromExpression(condition.Expression)
			if resourceInfo.Namespace != "" && resourceInfo.ResourceName != "" {
				metrics, err := metricsClient.GetResourceMetrics(ctx, resourceInfo.Namespace, resourceInfo.ResourceName, []string{"cpu", "memory"})
				if err == nil && metrics != nil {
					evalContext.CurrentMetrics["metrics"] = metrics
				}
			}
		}

		// Note: AlertClient interface doesn't have GetActiveAlerts, so we skip this for now
		// In a real implementation, we'd extend the interface or use a different approach
	}

	// Add step context information
	evalContext.EnvironmentContext["execution_id"] = stepContext.ExecutionID
	evalContext.EnvironmentContext["step_id"] = stepContext.StepID

	return evalContext, nil
}

func (ace *DefaultAIConditionEvaluator) gatherResourceContext(ctx context.Context, condition *engine.WorkflowCondition, stepContext *engine.StepContext) (*ConditionEvaluationContext, error) {
	evalContext := &ConditionEvaluationContext{
		ResourceStates:     make(map[string]interface{}),
		RecentEvents:       make([]interface{}, 0),
		EnvironmentContext: make(map[string]string),
	}

	// Gather Kubernetes resource information
	if ace.k8sClient != nil {
		// Get relevant resource states based on condition expression
		resourceInfo := ace.extractResourceInfoFromExpression(condition.Expression)

		if resourceInfo.Namespace != "" {
			// Get pods in namespace
			pods, err := ace.k8sClient.ListPodsWithLabel(ctx, resourceInfo.Namespace, resourceInfo.LabelSelector)
			if err == nil {
				evalContext.ResourceStates["pods"] = pods
			}

			// Get deployment if resource name is specified
			if resourceInfo.ResourceName != "" && resourceInfo.ResourceType == "Deployment" {
				deployment, err := ace.k8sClient.GetDeployment(ctx, resourceInfo.Namespace, resourceInfo.ResourceName)
				if err == nil {
					evalContext.ResourceStates["deployment"] = deployment
				}
			}
		}

		// Get cluster-level information
		nodes, err := ace.k8sClient.ListNodes(ctx)
		if err == nil {
			evalContext.ResourceStates["nodes"] = nodes
		}
	}

	// Add step context information
	evalContext.EnvironmentContext["execution_id"] = stepContext.ExecutionID
	evalContext.EnvironmentContext["step_id"] = stepContext.StepID

	return evalContext, nil
}

func (ace *DefaultAIConditionEvaluator) gatherTimeContext(ctx context.Context, condition *engine.WorkflowCondition, stepContext *engine.StepContext) (*ConditionEvaluationContext, error) {
	evalContext := &ConditionEvaluationContext{
		EnvironmentContext: make(map[string]string),
	}

	// Add temporal context
	now := time.Now()
	evalContext.EnvironmentContext["current_time"] = now.Format(time.RFC3339)
	evalContext.EnvironmentContext["current_hour"] = fmt.Sprintf("%d", now.Hour())
	evalContext.EnvironmentContext["current_day"] = now.Weekday().String()
	evalContext.EnvironmentContext["timezone"] = now.Location().String()

	// Add execution timing context
	// Note: StartTime not available in StepContext, would need to be tracked elsewhere
	evalContext.EnvironmentContext["evaluation_time"] = now.Format(time.RFC3339)

	// Add step context information
	evalContext.EnvironmentContext["execution_id"] = stepContext.ExecutionID
	evalContext.EnvironmentContext["step_id"] = stepContext.StepID

	return evalContext, nil
}

func (ace *DefaultAIConditionEvaluator) gatherExpressionContext(ctx context.Context, condition *engine.WorkflowCondition, stepContext *engine.StepContext) (*ConditionEvaluationContext, error) {
	evalContext := &ConditionEvaluationContext{
		EnvironmentContext: make(map[string]string),
	}

	// Add variable context from step and condition
	for key, value := range stepContext.Variables {
		evalContext.EnvironmentContext[fmt.Sprintf("step_var_%s", key)] = fmt.Sprintf("%v", value)
	}

	for key, value := range condition.Variables {
		evalContext.EnvironmentContext[fmt.Sprintf("condition_var_%s", key)] = fmt.Sprintf("%v", value)
	}

	return evalContext, nil
}

func (ace *DefaultAIConditionEvaluator) gatherCustomContext(ctx context.Context, condition *engine.WorkflowCondition, stepContext *engine.StepContext) (*ConditionEvaluationContext, error) {
	// Combine all context types for custom evaluation
	metricCtx, _ := ace.gatherMetricContext(ctx, condition, stepContext)
	resourceCtx, _ := ace.gatherResourceContext(ctx, condition, stepContext)
	timeCtx, _ := ace.gatherTimeContext(ctx, condition, stepContext)
	exprCtx, _ := ace.gatherExpressionContext(ctx, condition, stepContext)

	// Merge contexts
	evalContext := &ConditionEvaluationContext{
		CurrentMetrics:     metricCtx.CurrentMetrics,
		ResourceStates:     resourceCtx.ResourceStates,
		RecentEvents:       resourceCtx.RecentEvents,
		AlertHistory:       metricCtx.AlertHistory,
		EnvironmentContext: make(map[string]string),
	}

	// Merge environment contexts
	for k, v := range timeCtx.EnvironmentContext {
		evalContext.EnvironmentContext[k] = v
	}
	for k, v := range exprCtx.EnvironmentContext {
		evalContext.EnvironmentContext[k] = v
	}

	return evalContext, nil
}

// Helper types and methods

type ResourceInfo struct {
	Namespace     string
	ResourceType  string
	ResourceName  string
	LabelSelector string
}

func (ace *DefaultAIConditionEvaluator) extractResourceInfoFromExpression(expression string) ResourceInfo {
	info := ResourceInfo{}

	// Simple parsing of common patterns in expressions
	expr := strings.ToLower(expression)

	if strings.Contains(expr, "namespace") {
		// Extract namespace from expressions like "namespace=production"
		parts := strings.Split(expr, "namespace")
		if len(parts) > 1 {
			namespacePart := strings.TrimSpace(parts[1])
			if strings.HasPrefix(namespacePart, "=") {
				namespace := strings.TrimSpace(strings.TrimPrefix(namespacePart, "="))
				namespace = strings.Trim(namespace, "\"'")
				info.Namespace = namespace
			}
		}
	}

	if strings.Contains(expr, "pod") {
		info.ResourceType = "Pod"
	} else if strings.Contains(expr, "deployment") {
		info.ResourceType = "Deployment"
	} else if strings.Contains(expr, "service") {
		info.ResourceType = "Service"
	}

	return info
}
