package conditions

import (
	"fmt"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Prompt building methods for different condition types

func (ace *DefaultAIConditionEvaluator) buildMetricEvaluationPrompt(condition *engine.WorkflowCondition, stepContext *engine.StepContext, evalContext *ConditionEvaluationContext) string {
	var prompt strings.Builder

	prompt.WriteString("Evaluate a metric-based condition for workflow orchestration.\n\n")
	prompt.WriteString("CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- Execution ID: %s\n", stepContext.ExecutionID))
	prompt.WriteString(fmt.Sprintf("- Step ID: %s\n", stepContext.StepID))
	prompt.WriteString(fmt.Sprintf("- Condition ID: %s\n", condition.ID))
	prompt.WriteString(fmt.Sprintf("- Condition Name: %s\n", condition.Name))

	prompt.WriteString("\nCONDITION TO EVALUATE:\n")
	prompt.WriteString(fmt.Sprintf("Expression: %s\n", condition.Expression))

	if len(condition.Variables) > 0 {
		prompt.WriteString("\nCONDITION VARIABLES:\n")
		for key, value := range condition.Variables {
			prompt.WriteString(fmt.Sprintf("- %s: %v\n", key, value))
		}
	}

	if len(evalContext.CurrentMetrics) > 0 {
		prompt.WriteString("\nCURRENT METRICS:\n")
		for source, metrics := range evalContext.CurrentMetrics {
			prompt.WriteString(fmt.Sprintf("- %s: %v\n", source, summarizeMetrics(metrics)))
		}
	}

	if len(evalContext.AlertHistory) > 0 {
		prompt.WriteString(fmt.Sprintf("\nRECENT ALERTS: %d active alerts\n", len(evalContext.AlertHistory)))
	}

	prompt.WriteString("\nEVALUATION REQUEST:\n")
	prompt.WriteString("1. Analyze the metric condition expression\n")
	prompt.WriteString("2. Evaluate current metric values against the condition\n")
	prompt.WriteString("3. Consider system load and alert context\n")
	prompt.WriteString("4. Determine if the condition is satisfied\n")
	prompt.WriteString("5. Provide confidence level and reasoning\n")

	prompt.WriteString("\nRESPONSE FORMAT:\n")
	prompt.WriteString("Provide a JSON object with:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"satisfied\": boolean,\n")
	prompt.WriteString("  \"confidence\": number_between_0_and_1,\n")
	prompt.WriteString("  \"reasoning\": \"detailed_explanation\",\n")
	prompt.WriteString("  \"recommendations\": [\"action1\", \"action2\"],\n")
	prompt.WriteString("  \"warnings\": [\"warning1\", \"warning2\"],\n")
	prompt.WriteString("  \"metadata\": {\"key\": \"value\"}\n")
	prompt.WriteString("}\n")

	return prompt.String()
}

func (ace *DefaultAIConditionEvaluator) buildResourceEvaluationPrompt(condition *engine.WorkflowCondition, stepContext *engine.StepContext, evalContext *ConditionEvaluationContext) string {
	var prompt strings.Builder

	prompt.WriteString("Evaluate a Kubernetes resource condition for workflow orchestration.\n\n")
	prompt.WriteString("CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- Execution ID: %s\n", stepContext.ExecutionID))
	prompt.WriteString(fmt.Sprintf("- Condition Expression: %s\n", condition.Expression))

	prompt.WriteString("\nRESOURCE STATES:\n")
	if len(evalContext.ResourceStates) > 0 {
		for resourceType, resources := range evalContext.ResourceStates {
			prompt.WriteString(fmt.Sprintf("- %s: %v\n", resourceType, summarizeResources(resources)))
		}
	} else {
		prompt.WriteString("- No resource state information available\n")
	}

	if len(evalContext.RecentEvents) > 0 {
		prompt.WriteString(fmt.Sprintf("\nRECENT EVENTS: %d events\n", len(evalContext.RecentEvents)))
	}

	prompt.WriteString("\nEVALUATION REQUEST:\n")
	prompt.WriteString("1. Parse the resource condition expression\n")
	prompt.WriteString("2. Check current Kubernetes resource states\n")
	prompt.WriteString("3. Evaluate if resources meet the specified conditions\n")
	prompt.WriteString("4. Consider resource health and readiness\n")
	prompt.WriteString("5. Assess any potential issues or warnings\n")

	prompt.WriteString("\nRESPONSE FORMAT:\n")
	prompt.WriteString("Provide a JSON object with resource evaluation results:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"satisfied\": boolean,\n")
	prompt.WriteString("  \"confidence\": number_between_0_and_1,\n")
	prompt.WriteString("  \"reasoning\": \"explanation_of_resource_state_evaluation\",\n")
	prompt.WriteString("  \"recommendations\": [\"suggested_actions\"],\n")
	prompt.WriteString("  \"warnings\": [\"potential_issues\"],\n")
	prompt.WriteString("  \"metadata\": {\"resources_checked\": \"details\"}\n")
	prompt.WriteString("}\n")

	return prompt.String()
}

func (ace *DefaultAIConditionEvaluator) buildTimeEvaluationPrompt(condition *engine.WorkflowCondition, stepContext *engine.StepContext, evalContext *ConditionEvaluationContext) string {
	var prompt strings.Builder

	prompt.WriteString("Evaluate a time-based condition for workflow orchestration.\n\n")
	prompt.WriteString("CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- Condition Expression: %s\n", condition.Expression))

	prompt.WriteString("\nTIME CONTEXT:\n")
	for key, value := range evalContext.EnvironmentContext {
		if strings.Contains(key, "time") || strings.Contains(key, "day") || strings.Contains(key, "hour") {
			prompt.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
		}
	}

	if condition.Timeout > 0 {
		prompt.WriteString(fmt.Sprintf("- Condition Timeout: %s\n", condition.Timeout.String()))
	}

	prompt.WriteString("\nEVALUATION REQUEST:\n")
	prompt.WriteString("1. Parse the time-based condition expression\n")
	prompt.WriteString("2. Evaluate current time against condition requirements\n")
	prompt.WriteString("3. Consider execution timing and timeouts\n")
	prompt.WriteString("4. Assess if timing conditions are met\n")
	prompt.WriteString("5. Predict any timing-related issues\n")

	prompt.WriteString("\nRESPONSE FORMAT:\n")
	prompt.WriteString("Provide a JSON object with time evaluation results:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"satisfied\": boolean,\n")
	prompt.WriteString("  \"confidence\": number_between_0_and_1,\n")
	prompt.WriteString("  \"reasoning\": \"timing_analysis_explanation\",\n")
	prompt.WriteString("  \"recommendations\": [\"timing_suggestions\"],\n")
	prompt.WriteString("  \"warnings\": [\"timeout_or_timing_warnings\"],\n")
	prompt.WriteString("  \"metadata\": {\"timing_details\": \"info\"}\n")
	prompt.WriteString("}\n")

	return prompt.String()
}

func (ace *DefaultAIConditionEvaluator) buildExpressionEvaluationPrompt(condition *engine.WorkflowCondition, stepContext *engine.StepContext, evalContext *ConditionEvaluationContext) string {
	var prompt strings.Builder

	prompt.WriteString("Evaluate a complex expression condition for workflow orchestration.\n\n")
	prompt.WriteString("CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- Expression to evaluate: %s\n", condition.Expression))

	prompt.WriteString("\nAVAILABLE VARIABLES:\n")
	for key, value := range evalContext.EnvironmentContext {
		prompt.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
	}

	if len(condition.Variables) > 0 {
		prompt.WriteString("\nCONDITION VARIABLES:\n")
		for key, value := range condition.Variables {
			prompt.WriteString(fmt.Sprintf("- %s: %v\n", key, value))
		}
	}

	if len(stepContext.Variables) > 0 {
		prompt.WriteString("\nSTEP VARIABLES:\n")
		for key, value := range stepContext.Variables {
			prompt.WriteString(fmt.Sprintf("- %s: %v\n", key, value))
		}
	}

	prompt.WriteString("\nEVALUATION REQUEST:\n")
	prompt.WriteString("1. Parse the expression syntax\n")
	prompt.WriteString("2. Substitute available variables into the expression\n")
	prompt.WriteString("3. Evaluate the logical or mathematical expression\n")
	prompt.WriteString("4. Handle any syntax errors or missing variables gracefully\n")
	prompt.WriteString("5. Provide clear reasoning for the evaluation result\n")

	prompt.WriteString("\nRESPONSE FORMAT:\n")
	prompt.WriteString("Provide a JSON object with expression evaluation results:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"satisfied\": boolean,\n")
	prompt.WriteString("  \"confidence\": number_between_0_and_1,\n")
	prompt.WriteString("  \"reasoning\": \"expression_evaluation_explanation\",\n")
	prompt.WriteString("  \"recommendations\": [\"expression_improvements\"],\n")
	prompt.WriteString("  \"warnings\": [\"syntax_or_variable_issues\"],\n")
	prompt.WriteString("  \"metadata\": {\"parsed_expression\": \"details\", \"variable_substitutions\": \"info\"}\n")
	prompt.WriteString("}\n")

	return prompt.String()
}

func (ace *DefaultAIConditionEvaluator) buildCustomEvaluationPrompt(condition *engine.WorkflowCondition, stepContext *engine.StepContext, evalContext *ConditionEvaluationContext) string {
	var prompt strings.Builder

	prompt.WriteString("Evaluate a custom condition for workflow orchestration using comprehensive context.\n\n")
	prompt.WriteString("CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- Execution ID: %s\n", stepContext.ExecutionID))
	prompt.WriteString(fmt.Sprintf("- Step ID: %s\n", stepContext.StepID))
	prompt.WriteString(fmt.Sprintf("- Custom Condition: %s\n", condition.Expression))

	prompt.WriteString("\nSYSTEM CONTEXT:\n")
	if len(evalContext.CurrentMetrics) > 0 {
		prompt.WriteString("- Metrics available: Yes\n")
	}
	if len(evalContext.ResourceStates) > 0 {
		prompt.WriteString("- Resource states available: Yes\n")
	}
	if len(evalContext.AlertHistory) > 0 {
		prompt.WriteString(fmt.Sprintf("- Active alerts: %d\n", len(evalContext.AlertHistory)))
	}

	prompt.WriteString("\nENVIRONMENT VARIABLES:\n")
	for key, value := range evalContext.EnvironmentContext {
		prompt.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
	}

	prompt.WriteString("\nEVALUATION REQUEST:\n")
	prompt.WriteString("1. Analyze the custom condition in the context of all available information\n")
	prompt.WriteString("2. Use creative reasoning to interpret the condition's intent\n")
	prompt.WriteString("3. Consider system health, performance, and operational factors\n")
	prompt.WriteString("4. Evaluate whether the condition should be satisfied\n")
	prompt.WriteString("5. Provide actionable insights and recommendations\n")

	prompt.WriteString("\nRESPONSE FORMAT:\n")
	prompt.WriteString("Provide a JSON object with custom evaluation results:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"satisfied\": boolean,\n")
	prompt.WriteString("  \"confidence\": number_between_0_and_1,\n")
	prompt.WriteString("  \"reasoning\": \"comprehensive_analysis_explanation\",\n")
	prompt.WriteString("  \"recommendations\": [\"actionable_suggestions\"],\n")
	prompt.WriteString("  \"warnings\": [\"potential_concerns\"],\n")
	prompt.WriteString("  \"metadata\": {\"analysis_scope\": \"comprehensive\", \"factors_considered\": [\"list\"]}\n")
	prompt.WriteString("}\n")

	return prompt.String()
}

// Helper functions for summarizing complex data structures

func summarizeMetrics(metrics interface{}) string {
	if metrics == nil {
		return "No metrics available"
	}

	// Simple summary - in a real implementation, this would be more sophisticated
	return fmt.Sprintf("Available (%T)", metrics)
}

func summarizeResources(resources interface{}) string {
	if resources == nil {
		return "No resources found"
	}

	// Simple summary - in a real implementation, this would parse the actual resource data
	return fmt.Sprintf("Available (%T)", resources)
}
