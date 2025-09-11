package engine

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// Additional supporting types specific to intelligent workflow builder

// ResourceReference represents a referenced resource
type ResourceReference struct {
	Type      string `json:"type"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// Implementation of helper methods

// identifyActionTypesFromObjective identifies potential action types from objective
func (iwb *DefaultIntelligentWorkflowBuilder) identifyActionTypesFromObjective(objective *WorkflowObjective) []string {
	actionTypes := make([]string, 0)
	description := strings.ToLower(objective.Description)

	// Map keywords to action types
	actionMap := map[string][]string{
		"scale":    {"scale_deployment", "increase_resources"},
		"restart":  {"restart_pod", "rollback_deployment"},
		"memory":   {"increase_resources", "collect_diagnostics"},
		"cpu":      {"increase_resources", "scale_deployment"},
		"disk":     {"cleanup_resources", "expand_pvc"},
		"network":  {"update_network_policy", "restart_network"},
		"backup":   {"create_backup", "migrate_data"},
		"security": {"rotate_secrets", "update_rbac"},
		"monitor":  {"collect_diagnostics", "update_monitoring"},
		"cleanup":  {"cleanup_resources", "garbage_collect"},
	}

	for keyword, actions := range actionMap {
		if strings.Contains(description, keyword) {
			actionTypes = append(actionTypes, actions...)
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	unique := make([]string, 0)
	for _, action := range actionTypes {
		if !seen[action] {
			seen[action] = true
			unique = append(unique, action)
		}
	}

	return unique
}

// assessRiskLevel assesses the risk level of an objective
func (iwb *DefaultIntelligentWorkflowBuilder) assessRiskLevel(objective *WorkflowObjective, complexity float64) string {
	risk := "low"

	// Increase risk based on complexity
	if complexity > 0.7 {
		risk = "high"
	} else if complexity > 0.4 {
		risk = "medium"
	}

	// Increase risk for certain action types
	description := strings.ToLower(objective.Description)
	highRiskKeywords := []string{"delete", "remove", "drain", "restart", "rollback"}
	for _, keyword := range highRiskKeywords {
		if strings.Contains(description, keyword) {
			if risk == "low" {
				risk = "medium"
			} else if risk == "medium" {
				risk = "high"
			}
			break
		}
	}

	return risk
}

// generateObjectiveRecommendation generates a recommendation for the objective
func (iwb *DefaultIntelligentWorkflowBuilder) generateObjectiveRecommendation(objective *WorkflowObjective, complexity float64, riskLevel string) string {
	recommendations := []string{
		"Consider implementing comprehensive monitoring and alerting",
		"Add appropriate rollback mechanisms for critical changes",
		"Ensure proper testing in development environment first",
		"Implement gradual rollout strategies for production changes",
	}

	if complexity > 0.5 {
		recommendations = append(recommendations, "Break down complex objectives into smaller, manageable steps")
	}

	if riskLevel == "high" {
		recommendations = append(recommendations, "Implement additional safety measures and approval processes")
	}

	return strings.Join(recommendations, "; ")
}

// convertActionPatternToWorkflowPattern converts action pattern to workflow pattern
func (iwb *DefaultIntelligentWorkflowBuilder) convertActionPatternToWorkflowPattern(actionPattern *vector.ActionPattern) *WorkflowPattern {
	return &WorkflowPattern{
		ID:             actionPattern.ID,
		Name:           fmt.Sprintf("Pattern-%s", actionPattern.ActionType),
		Type:           "action-based",
		Steps:          iwb.createStepsFromActionPattern(actionPattern),
		Conditions:     iwb.createConditionsFromActionPattern(actionPattern),
		SuccessRate:    iwb.calculateSuccessRateFromPattern(actionPattern),
		ExecutionCount: actionPattern.EffectivenessData.SuccessCount + actionPattern.EffectivenessData.FailureCount,
		AverageTime:    actionPattern.EffectivenessData.AverageExecutionTime,
		Environments:   []string{actionPattern.Namespace},
		ResourceTypes:  []string{actionPattern.ResourceType},
		Confidence:     iwb.calculatePatternConfidenceFromAction(actionPattern),
		LastUsed:       actionPattern.UpdatedAt,
		CreatedAt:      actionPattern.CreatedAt,
		UpdatedAt:      actionPattern.UpdatedAt,
	}
}

// createStepsFromActionPattern creates workflow steps from action pattern
func (iwb *DefaultIntelligentWorkflowBuilder) createStepsFromActionPattern(pattern *vector.ActionPattern) []*ExecutableWorkflowStep {
	steps := make([]*ExecutableWorkflowStep, 1) // Create at least one step from the pattern

	step := &ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   uuid.New().String(),
			Name: fmt.Sprintf("Execute-%s", pattern.ActionType),
		},
		Type: StepTypeAction,
		Action: &StepAction{
			Type:       pattern.ActionType,
			Parameters: pattern.ActionParameters,
			Target: &ActionTarget{
				Type:      "kubernetes",
				Namespace: pattern.Namespace,
				Resource:  pattern.ResourceType,
				Name:      pattern.ResourceName,
			},
		},
		Timeout:   iwb.config.DefaultStepTimeout,
		Variables: make(map[string]interface{}),
		Metadata:  pattern.Metadata,
	}

	steps[0] = step
	return steps
}

// createConditionsFromActionPattern creates conditions from action pattern
func (iwb *DefaultIntelligentWorkflowBuilder) createConditionsFromActionPattern(pattern *vector.ActionPattern) []*ActionCondition {
	conditions := make([]*ActionCondition, 0)

	// Create conditions from pre/post conditions if available
	if pattern.PreConditions != nil {
		for key, value := range pattern.PreConditions {
			condition := &ActionCondition{
				ID:         uuid.New().String(),
				Type:       "expression",
				Expression: fmt.Sprintf("%s == %v", key, value),
				Variables:  map[string]interface{}{key: value},
			}
			conditions = append(conditions, condition)
		}
	}

	return conditions
}

// calculateSuccessRateFromPattern calculates success rate from action pattern
func (iwb *DefaultIntelligentWorkflowBuilder) calculateSuccessRateFromPattern(pattern *vector.ActionPattern) float64 {
	if pattern.EffectivenessData == nil {
		return 0.5 // Default assumption
	}

	totalExecutions := pattern.EffectivenessData.SuccessCount + pattern.EffectivenessData.FailureCount
	if totalExecutions == 0 {
		return 0.5
	}

	return float64(pattern.EffectivenessData.SuccessCount) / float64(totalExecutions)
}

// calculatePatternConfidenceFromAction calculates pattern confidence from action
func (iwb *DefaultIntelligentWorkflowBuilder) calculatePatternConfidenceFromAction(pattern *vector.ActionPattern) float64 {
	if pattern.EffectivenessData == nil {
		return 0.5
	}

	// Base confidence on effectiveness score
	confidence := pattern.EffectivenessData.Score * 0.8

	// Adjust based on execution count
	totalExecutions := pattern.EffectivenessData.SuccessCount + pattern.EffectivenessData.FailureCount
	if totalExecutions >= 10 {
		confidence += 0.15
	} else if totalExecutions >= 5 {
		confidence += 0.1
	}

	// Normalize to 0-1
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// extractJSONFromResponse extracts JSON from AI response
func (iwb *DefaultIntelligentWorkflowBuilder) extractJSONFromResponse(response string) string {
	// Find JSON object in response
	start := strings.Index(response, "{")
	if start == -1 {
		return response
	}

	// Find matching closing brace
	braceCount := 0
	end := start
	for i := start; i < len(response); i++ {
		if response[i] == '{' {
			braceCount++
		} else if response[i] == '}' {
			braceCount--
			if braceCount == 0 {
				end = i + 1
				break
			}
		}
	}

	return response[start:end]
}

// validateAIResponse validates the structure of AI response
func (iwb *DefaultIntelligentWorkflowBuilder) validateAIResponse(response *AIWorkflowResponse) error {
	if response.WorkflowName == "" {
		return fmt.Errorf("workflow name is required")
	}

	if len(response.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}

	if len(response.Steps) > iwb.config.MaxWorkflowSteps {
		return fmt.Errorf("too many steps: %d (max: %d)", len(response.Steps), iwb.config.MaxWorkflowSteps)
	}

	// Validate each step
	for i, step := range response.Steps {
		if step.Name == "" {
			return fmt.Errorf("step %d is missing name", i)
		}
		if step.Type == "" {
			return fmt.Errorf("step %d is missing type", i)
		}
	}

	return nil
}

// convertAIStepToExecutableWorkflowStep converts AI step to workflow step
func (iwb *DefaultIntelligentWorkflowBuilder) convertAIStepToExecutableWorkflowStep(aiStep *AIGeneratedStep, index int) (*ExecutableWorkflowStep, error) {
	step := &ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   uuid.New().String(),
			Name: aiStep.Name,
		},
		Type:         StepType(aiStep.Type),
		Dependencies: aiStep.Dependencies,
		OnSuccess:    aiStep.OnSuccess,
		OnFailure:    aiStep.OnFailure,
		Variables:    aiStep.Variables,
		Metadata:     map[string]interface{}{"ai_generated": true, "step_index": index},
	}

	// Parse timeout
	if aiStep.Timeout != "" {
		if duration, err := time.ParseDuration(aiStep.Timeout); err == nil {
			step.Timeout = duration
		} else {
			step.Timeout = iwb.config.DefaultStepTimeout
		}
	} else {
		step.Timeout = iwb.config.DefaultStepTimeout
	}

	// Convert action if present
	if aiStep.Action != nil {
		step.Action = &StepAction{
			Type:       aiStep.Action.Type,
			Parameters: aiStep.Action.Parameters,
		}

		if aiStep.Action.Target != nil {
			step.Action.Target = &ActionTarget{
				Type:      aiStep.Action.Target.Type,
				Namespace: aiStep.Action.Target.Namespace,
				Resource:  aiStep.Action.Target.Resource,
				Name:      aiStep.Action.Target.Name,
				Selector:  aiStep.Action.Target.Selector,
			}
		}
	}

	// Add default retry policy
	step.RetryPolicy = &RetryPolicy{
		MaxRetries:  iwb.config.MaxRetries,
		Delay:       time.Second * 5,
		Backoff:     BackoffTypeExponential,
		BackoffRate: 2.0,
		Conditions:  []string{"network_error", "timeout", "temporary_failure"},
	}

	return step, nil
}

// createTimeoutsFromEstimation creates timeouts from AI estimation
func (iwb *DefaultIntelligentWorkflowBuilder) createTimeoutsFromEstimation(estimation string) *WorkflowTimeouts {
	// Parse estimation or use defaults
	defaultTimeout := iwb.config.DefaultStepTimeout

	if estimation != "" {
		if duration, err := time.ParseDuration(estimation); err == nil {
			defaultTimeout = duration
		}
	}

	return &WorkflowTimeouts{
		Execution: defaultTimeout * 2,
		Step:      defaultTimeout,
		Condition: defaultTimeout / 2,
		Recovery:  defaultTimeout,
	}
}

// createRecoveryPolicyFromRisk creates recovery policy based on risk assessment
func (iwb *DefaultIntelligentWorkflowBuilder) createRecoveryPolicyFromRisk(riskAssessment string) *RecoveryPolicy {
	policy := &RecoveryPolicy{
		Enabled:         true,
		MaxRecoveryTime: iwb.config.DefaultStepTimeout * 3,
		Strategies:      make([]*RecoveryStrategy, 0),
		Notifications:   make([]*NotificationConfig, 0),
	}

	// Add strategies based on risk level
	if strings.Contains(strings.ToLower(riskAssessment), "high") {
		policy.Strategies = append(policy.Strategies, &RecoveryStrategy{
			Type:     RecoveryTypeRollback,
			Priority: PriorityHigh,
			Actions: []*RecoveryAction{
				{
					ID:         uuid.New().String(),
					Type:       "rollback_step",
					Trigger:    "step_failure",
					Parameters: map[string]interface{}{"automatic": true, "description": "Immediate rollback on high risk failure", "critical": true},
					Timeout:    int(iwb.config.DefaultStepTimeout.Seconds()),
				},
			},
		})
	} else {
		policy.Strategies = append(policy.Strategies, &RecoveryStrategy{
			Type:     RecoveryTypeRetry,
			Priority: PriorityMedium,
			Actions: []*RecoveryAction{
				{
					ID:         uuid.New().String(),
					Type:       "retry_step",
					Trigger:    "step_failure",
					Parameters: map[string]interface{}{"max_retries": 3, "description": "Retry with exponential backoff", "critical": false},
					Timeout:    int(iwb.config.DefaultStepTimeout.Seconds()),
				},
			},
		})
	}

	return policy
}

// createDefaultRecoveryPolicy creates a default recovery policy
func (iwb *DefaultIntelligentWorkflowBuilder) createDefaultRecoveryPolicy() *RecoveryPolicy {
	return &RecoveryPolicy{
		Enabled:         true,
		MaxRecoveryTime: iwb.config.DefaultStepTimeout * 2,
		Strategies: []*RecoveryStrategy{
			{
				Type:     RecoveryTypeRetry,
				Priority: PriorityMedium,
				Actions: []*RecoveryAction{
					{
						ID:         uuid.New().String(),
						Type:       "retry_step",
						Trigger:    "step_failure",
						Parameters: map[string]interface{}{"max_retries": iwb.config.MaxRetries, "description": "Default retry with backoff policy", "critical": false},
						Timeout:    int(iwb.config.DefaultStepTimeout.Seconds()),
					},
				},
			},
		},
		Notifications: []*NotificationConfig{
			{
				Enabled:    true,
				Channels:   []string{"ops-team"},
				Recipients: []string{"ops-team@company.com"},
				Template:   "workflow_failure",
			},
		},
	}
}

// extractVariablesFromContext extracts variables from workflow context
func (iwb *DefaultIntelligentWorkflowBuilder) extractVariablesFromContext(context *WorkflowContext) map[string]interface{} {
	variables := make(map[string]interface{})

	// Copy context variables
	for k, v := range context.Variables {
		variables[k] = v
	}

	// Add context metadata
	variables["environment"] = context.Environment
	variables["cluster"] = context.Cluster
	variables["namespace"] = context.Namespace

	// Add timestamp
	variables["workflow_created_at"] = context.CreatedAt.Format(time.RFC3339)

	return variables
}

// extractKeywords extracts keywords from text
func (iwb *DefaultIntelligentWorkflowBuilder) extractKeywords(text string) []string {
	// Simple keyword extraction - in production, use NLP libraries
	words := strings.Fields(strings.ToLower(text))
	keywords := make([]string, 0)

	// Filter out common words and keep meaningful terms
	stopWords := map[string]bool{
		"the": true, "and": true, "or": true, "but": true, "in": true,
		"on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "a": true, "an": true, "is": true,
	}

	for _, word := range words {
		if len(word) > 3 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// calculateObjectiveComplexity calculates the complexity of an objective
func (iwb *DefaultIntelligentWorkflowBuilder) calculateObjectiveComplexity(objective *WorkflowObjective) float64 {
	complexity := 0.0

	// Base complexity on number of targets
	complexity += float64(len(objective.Targets)) * 0.2

	// Add complexity for constraints
	complexity += float64(len(objective.Constraints)) * 0.1

	// Add complexity based on priority (higher priority = more complex requirements)
	complexity += float64(objective.Priority) * 0.1

	// Normalize to 0-1 range
	return math.Min(complexity, 1.0)
}

// deepCopyTemplate creates a deep copy of a workflow template
func (iwb *DefaultIntelligentWorkflowBuilder) deepCopyTemplate(template *ExecutableTemplate) *ExecutableTemplate {
	// In production, use a proper deep copy library
	// For now, create a new template with copied values
	copied := &ExecutableTemplate{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:          template.ID,
				Name:        template.Name,
				Description: template.Description,
				CreatedAt:   template.CreatedAt,
			},
			Version:   template.Version,
			CreatedBy: template.CreatedBy,
		},
		Steps:      make([]*ExecutableWorkflowStep, len(template.Steps)),
		Conditions: make([]*ExecutableCondition, len(template.Conditions)),
		Variables:  make(map[string]interface{}),
		Timeouts:   template.Timeouts,
		Recovery:   template.Recovery,
		Tags:       make([]string, len(template.Tags)),
	}

	// Copy steps
	copy(copied.Steps, template.Steps)
	copy(copied.Conditions, template.Conditions)
	copy(copied.Tags, template.Tags)

	// Copy variables
	for k, v := range template.Variables {
		copied.Variables[k] = v
	}

	return copied
}

// getAvailableActionTypes returns list of available action types
func (iwb *DefaultIntelligentWorkflowBuilder) getAvailableActionTypes() []string {
	return []string{
		"scale_deployment",
		"restart_pod",
		"collect_diagnostics",
		"increase_resources",
		"decrease_resources",
		"update_config",
		"rollback_deployment",
		"drain_node",
		"cordon_node",
		"notify_team",
		"create_backup",
		"cleanup_resources",
	}
}

// isValidActionType checks if an action type is valid
func (iwb *DefaultIntelligentWorkflowBuilder) isValidActionType(actionType string) bool {
	validTypes := iwb.getAvailableActionTypes()
	for _, validType := range validTypes {
		if validType == actionType {
			return true
		}
	}
	return false
}

// getRequiredParametersForAction returns required parameters for an action type
func (iwb *DefaultIntelligentWorkflowBuilder) getRequiredParametersForAction(actionType string) []string {
	paramMap := map[string][]string{
		"scale_deployment":    {"replicas"},
		"restart_pod":         {"name"},
		"increase_resources":  {"cpu", "memory"},
		"rollback_deployment": {"revision"},
		"drain_node":          {"node_name"},
	}

	if params, exists := paramMap[actionType]; exists {
		return params
	}
	return []string{}
}

// validateParameterValue validates a parameter value for an action
func (iwb *DefaultIntelligentWorkflowBuilder) validateParameterValue(actionType, param string, value interface{}) error {
	switch actionType {
	case "scale_deployment":
		if param == "replicas" {
			if replicas, ok := value.(float64); ok {
				if replicas < 0 || replicas > 100 {
					return fmt.Errorf("replicas must be between 0 and 100")
				}
			} else {
				return fmt.Errorf("replicas must be a number")
			}
		}
	case "increase_resources":
		if param == "cpu" || param == "memory" {
			if str, ok := value.(string); ok {
				if str == "" {
					return fmt.Errorf("%s cannot be empty", param)
				}
			} else {
				return fmt.Errorf("%s must be a string", param)
			}
		}
	}

	return nil
}

// incrementVersion increments a semantic version string
func (iwb *DefaultIntelligentWorkflowBuilder) incrementVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "1.0.1"
	}

	if patch, err := strconv.Atoi(parts[2]); err == nil {
		return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
	}

	return "1.0.1"
}

// calculateSimulatedDuration calculates simulated execution duration for a step
func (iwb *DefaultIntelligentWorkflowBuilder) calculateSimulatedDuration(step *ExecutableWorkflowStep, env *ExtendedSimulatedEnvironment) time.Duration {
	baseDuration := time.Second * 30 // Base duration

	// Adjust based on step type
	if step.Action != nil {
		switch step.Action.Type {
		case "scale_deployment":
			baseDuration = time.Minute * 2
		case "restart_pod":
			baseDuration = time.Second * 45
		case "collect_diagnostics":
			baseDuration = time.Second * 15
		case "increase_resources":
			baseDuration = time.Minute * 1
		}
	}

	// Adjust based on environment conditions
	if resourceLimits, ok := env.ResourceLimit["cpu"]; ok {
		if resourceLimits < 0.5 {
			baseDuration = time.Duration(float64(baseDuration) * 1.5) // 50% slower
		}
	}

	// Add some randomness
	randomFactor := 0.8 + rand.Float64()*0.4 // 80% to 120%
	return time.Duration(float64(baseDuration) * randomFactor)
}

// shouldStepFail determines if a step should fail in simulation
func (iwb *DefaultIntelligentWorkflowBuilder) shouldStepFail(step *ExecutableWorkflowStep, env *ExtendedSimulatedEnvironment) bool {
	failureRate := 0.0

	// Get failure rate from environment
	if rate, ok := env.Metrics["success_rate"]; ok {
		failureRate = 1.0 - rate
	}

	// Increase failure rate for certain action types in stress conditions
	if env.FailureMode == "random" && step.Action != nil {
		switch step.Action.Type {
		case "scale_deployment":
			failureRate += 0.1
		case "increase_resources":
			failureRate += 0.05
		}
	}

	return rand.Float64() < failureRate
}

// simulateActionResult simulates the result of an action
func (iwb *DefaultIntelligentWorkflowBuilder) simulateActionResult(action *StepAction, env *ExtendedSimulatedEnvironment) map[string]interface{} {
	result := map[string]interface{}{
		"action_type": action.Type,
		"simulated":   true,
	}

	switch action.Type {
	case "scale_deployment":
		result["replicas_before"] = 3
		result["replicas_after"] = 5
		result["scaling_time"] = "45s"
	case "restart_pod":
		result["pod_restarted"] = true
		result["restart_time"] = "30s"
	case "increase_resources":
		result["cpu_before"] = "100m"
		result["cpu_after"] = "200m"
		result["memory_before"] = "256Mi"
		result["memory_after"] = "512Mi"
	}

	return result
}

// calculateStepRiskScore calculates risk score for a step
func (iwb *DefaultIntelligentWorkflowBuilder) calculateStepRiskScore(step *ExecutableWorkflowStep) float64 {
	riskScore := 0.0

	// Base risk based on action type
	if step.Action != nil {
		switch step.Action.Type {
		case "scale_deployment":
			riskScore = 0.3
		case "restart_pod":
			riskScore = 0.5
		case "increase_resources":
			riskScore = 0.2
		case "rollback_deployment":
			riskScore = 0.7
		case "drain_node":
			riskScore = 0.9
		default:
			riskScore = 0.1
		}
	}

	// Increase risk if no rollback is configured
	if step.Action != nil && step.Action.Rollback == nil && riskScore > 0.3 {
		riskScore += 0.2
	}

	// Increase risk if no retry policy
	if step.RetryPolicy == nil {
		riskScore += 0.1
	}

	return math.Min(riskScore, 1.0)
}
