package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// registerInitialPrompts registers initial prompt versions for optimization
func (iwb *DefaultIntelligentWorkflowBuilder) registerInitialPrompts() {
	// Register the base prompt version
	basePrompt := &PromptVersion{
		ID:          "base-prompt-v1",
		Version:     "1.0.0",
		Name:        "Base Workflow Generation Prompt",
		Description: "Initial production prompt for workflow generation",
		PromptTemplate: `<|system|>
You are an expert Kubernetes workflow automation engineer. Your task is to generate comprehensive, safe, and effective workflow templates based on objectives and historical patterns.

<|user|>
Generate a detailed workflow template for the following objective and analysis:

%s

Requirements:
1. Create a step-by-step workflow with clear dependencies
2. Include appropriate conditions and validation steps
3. Ensure safety measures and rollback capabilities
4. Use proven patterns when available
5. Optimize for effectiveness and reliability
6. Include timeout and retry configurations
7. Add proper variable handling and context awareness

Respond with a valid JSON object in the established workflow template format.`,
		Requirements: []string{
			"JSON response format",
			"Step dependencies",
			"Safety measures",
			"Pattern usage",
			"Error handling",
		},
		OutputFormat: "JSON",
		IsActive:     true,
	}

	if err := iwb.promptOptimizer.RegisterPromptVersion(basePrompt); err != nil {
		iwb.log.WithError(err).Warn("Failed to register base prompt version")
	}

	// Register an enhanced prompt version for A/B testing
	enhancedPrompt := &PromptVersion{
		ID:          "enhanced-prompt-v1",
		Version:     "1.1.0",
		Name:        "Enhanced Safety-First Workflow Prompt",
		Description: "Enhanced prompt with stronger safety emphasis",
		PromptTemplate: `<|system|>
You are an expert Kubernetes workflow automation engineer with a focus on safety-first operations. Your task is to generate comprehensive, safe, and highly reliable workflow templates based on objectives and historical patterns.

<|user|>
Generate a detailed workflow template for the following objective and analysis:

%s

CRITICAL REQUIREMENTS:
1. SAFETY FIRST: Always include validation steps before destructive actions
2. Create a step-by-step workflow with clear dependencies and rollback capabilities
3. Include comprehensive error handling and timeout configurations
4. Use proven patterns when available and validate their applicability
5. Optimize for reliability over speed
6. Add monitoring and observability steps
7. Include proper variable handling and context awareness
8. Ensure each step has clear success/failure criteria

RESPONSE FORMAT: Valid JSON object with the established workflow template structure.
VALIDATION: Each step must include safety measures and rollback procedures.`,
		Requirements: []string{
			"Safety-first approach",
			"Comprehensive validation",
			"Rollback procedures",
			"Monitoring integration",
			"Error handling",
		},
		OutputFormat: "JSON",
		IsActive:     false, // Will be used in A/B tests
	}

	if err := iwb.promptOptimizer.RegisterPromptVersion(enhancedPrompt); err != nil {
		iwb.log.WithError(err).Warn("Failed to register enhanced prompt version")
	}

	iwb.log.Info("Registered initial prompt versions for optimization")
}

// getPromptIDFromContext extracts prompt ID from context (placeholder implementation)
func (iwb *DefaultIntelligentWorkflowBuilder) getPromptIDFromContext(ctx context.Context) string {
	// In a real implementation, this would extract the prompt ID from context
	// For now, return the active prompt ID
	return "base-prompt-v1"
}

// checkSafetyFlags checks for safety concerns in AI response
func (iwb *DefaultIntelligentWorkflowBuilder) checkSafetyFlags(response *AIWorkflowResponse) []string {
	var flags []string

	// Check for missing validation steps
	hasValidation := false
	for _, step := range response.Steps {
		if step.Action != nil && step.Action.Type == "validate" {
			hasValidation = true
			break
		}
	}
	if !hasValidation {
		flags = append(flags, "missing_validation")
	}

	// Check for destructive actions without confirmation
	for _, step := range response.Steps {
		if step.Action != nil && iwb.isDestructiveAction(step.Action.Type) {
			// Check if there's a preceding confirmation step
			hasConfirmation := false
			for i, prevStep := range response.Steps {
				if prevStep == step {
					break
				}
				if prevStep.Type == "condition" ||
					(prevStep.Action != nil && prevStep.Action.Type == "confirm") {
					hasConfirmation = true
					break
				}
				// If this is the step just before our destructive action
				if i == len(response.Steps)-2 {
					break
				}
			}
			if !hasConfirmation {
				flags = append(flags, "destructive_without_confirmation")
			}
		}
	}

	// Check for missing rollback mechanisms
	hasRollback := false
	for _, step := range response.Steps {
		if step.Action != nil && (step.Action.Type == "rollback" || step.Action.Type == "rollback_deployment") {
			hasRollback = true
			break
		}
	}
	if !hasRollback {
		flags = append(flags, "missing_rollback")
	}

	// Check for unreasonably short timeouts
	for _, step := range response.Steps {
		if step.Timeout != "" {
			if duration, err := time.ParseDuration(step.Timeout); err == nil {
				if duration < 30*time.Second {
					flags = append(flags, "short_timeout")
					break
				}
			}
		}
	}

	return flags
}

// StartPromptABTest starts an A/B test between different prompt versions
func (iwb *DefaultIntelligentWorkflowBuilder) StartPromptABTest(testName string, controlPrompt string, testPrompts []string) error {
	if iwb.promptOptimizer == nil {
		return fmt.Errorf("prompt optimizer not initialized")
	}

	experiment := &PromptExperiment{
		Name:          testName,
		Description:   fmt.Sprintf("A/B test comparing %s with alternatives", controlPrompt),
		ControlPrompt: controlPrompt,
		TestPrompts:   testPrompts,
		TrafficSplit: map[string]float64{
			controlPrompt: 0.5, // 50% control
		},
	}

	// Split remaining traffic among test prompts
	remainingTraffic := 0.5
	trafficPerTest := remainingTraffic / float64(len(testPrompts))
	for _, testPrompt := range testPrompts {
		experiment.TrafficSplit[testPrompt] = trafficPerTest
	}

	return iwb.promptOptimizer.StartABTest(experiment)
}

// GetAIPerformanceReport returns comprehensive AI performance metrics
func (iwb *DefaultIntelligentWorkflowBuilder) GetAIPerformanceReport() map[string]interface{} {
	if iwb.metricsCollector == nil {
		return map[string]interface{}{"error": "metrics collector not initialized"}
	}

	report := make(map[string]interface{})

	// Add prompt optimization data
	if iwb.promptOptimizer != nil {
		promptStats := iwb.promptOptimizer.GetPromptStatistics()
		report["prompt_optimization"] = map[string]interface{}{
			"versions":            promptStats,
			"running_experiments": iwb.promptOptimizer.GetRunningExperiments(),
		}
	}

	return report
}

// LearnFromWorkflowExecution integrates learning from workflow execution
func (iwb *DefaultIntelligentWorkflowBuilder) LearnFromWorkflowExecution(ctx context.Context, execution *WorkflowExecution) error {
	if iwb.learningBuilder == nil {
		return nil // Learning not enabled
	}

	// Learn from the execution
	if iwb.learningBuilder != nil {
		return iwb.learningBuilder.GetLearnFromExecution(ctx, execution)
	}
	return nil
}

// createMockResponseFromExecution creates a mock AI response for quality evaluation
func (iwb *DefaultIntelligentWorkflowBuilder) createMockResponseFromExecution(execution *WorkflowExecution) *AIWorkflowResponse {
	steps := make([]*AIGeneratedStep, len(execution.Steps))
	for i, stepExec := range execution.Steps {
		steps[i] = &AIGeneratedStep{
			Name: stepExec.StepID, // Simplified
			Type: "action",
			Action: &AIGeneratedAction{
				Type: "execute", // Simplified
			},
		}
	}

	return &AIWorkflowResponse{
		WorkflowName: execution.WorkflowID,
		Description:  "Reconstructed from execution",
		Steps:        steps,
	}
}

// calculateExecutionQuality calculates a quality score from execution results
func (iwb *DefaultIntelligentWorkflowBuilder) calculateExecutionQuality(execution *WorkflowExecution) float64 {
	if execution.Status != ExecutionStatusCompleted {
		return 0.3 // Failed executions get low quality score
	}

	// Base score for successful execution
	score := 0.7

	// Bonus for fast execution
	if execution.Duration < 5*time.Minute {
		score += 0.1
	}

	// Bonus for no errors
	if execution.Error == "" {
		score += 0.1
	}

	// Bonus for all steps successful
	allStepsSuccessful := true
	for _, step := range execution.Steps {
		if step.Status != ExecutionStatusCompleted {
			allStepsSuccessful = false
			break
		}
	}
	if allStepsSuccessful {
		score += 0.1
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

// UpdateWorkflowPatterns updates patterns based on learnings (enhanced version)
func (iwb *DefaultIntelligentWorkflowBuilder) UpdateWorkflowPatterns(ctx context.Context, learnings []*WorkflowLearning) error {
	// Call the base implementation first
	if err := iwb.updateWorkflowPatternsBase(ctx, learnings); err != nil {
		return err
	}

	// If learning integration is enabled, also update learned patterns
	if iwb.learningBuilder != nil {
		for _, learning := range learnings {
			// Create a mock execution from learning for the learning system
			execution := iwb.createExecutionFromLearning(learning)

			if err := iwb.learningBuilder.GetLearnFromExecution(ctx, execution); err != nil {
				iwb.log.WithError(err).Warn("Failed to integrate learning into learning system")
			}
		}
	}

	return nil
}

// updateWorkflowPatternsBase is the original implementation (placeholder)
func (iwb *DefaultIntelligentWorkflowBuilder) updateWorkflowPatternsBase(ctx context.Context, learnings []*WorkflowLearning) error {
	// This would contain the original logic from UpdateWorkflowPatterns
	// For now, it's a placeholder that groups learnings and creates patterns
	return nil
}

// createExecutionFromLearning creates a mock execution from learning data
func (iwb *DefaultIntelligentWorkflowBuilder) createExecutionFromLearning(learning *WorkflowLearning) *WorkflowExecution {
	execution := &WorkflowExecution{
		ID:         uuid.New().String(),
		WorkflowID: learning.WorkflowID,
		Status:     ExecutionStatusCompleted,
		StartTime:  learning.CreatedAt,
		Duration:   time.Minute, // Default duration
		Steps:      []*StepExecution{},
		Context:    &ExecutionContext{},
	}

	// Extract execution status from learning data
	if success, ok := learning.Data["success"].(bool); ok && !success {
		execution.Status = ExecutionStatusFailed
	}

	return execution
}

// extractQualityFromLearning extracts quality score from learning data
func (iwb *DefaultIntelligentWorkflowBuilder) extractQualityFromLearning(learning *WorkflowLearning) float64 {
	// Try to extract quality/effectiveness from learning data
	if effectiveness, ok := learning.Data["effectiveness"].(float64); ok {
		return effectiveness
	}

	if improvement, ok := learning.Data["improvement"].(float64); ok {
		return 0.7 + improvement // Base score + improvement
	}

	if success, ok := learning.Data["success"].(bool); ok {
		if success {
			return 0.8
		}
		return 0.3
	}

	return 0.5 // Default neutral score
}

// EvaluateAndPromotePrompts evaluates running prompt experiments and promotes winners
func (iwb *DefaultIntelligentWorkflowBuilder) EvaluateAndPromotePrompts() {
	if iwb.promptOptimizer == nil {
		return
	}

	iwb.promptOptimizer.EvaluateExperiments()
	iwb.log.Debug("Evaluated prompt optimization experiments")
}

// GetPromptOptimizationStatus returns the current status of prompt optimization
func (iwb *DefaultIntelligentWorkflowBuilder) GetPromptOptimizationStatus() map[string]interface{} {
	if iwb.promptOptimizer == nil {
		return map[string]interface{}{"enabled": false}
	}

	return map[string]interface{}{
		"enabled":             true,
		"prompt_versions":     iwb.promptOptimizer.GetPromptStatistics(),
		"running_experiments": iwb.promptOptimizer.GetRunningExperiments(),
	}
}
