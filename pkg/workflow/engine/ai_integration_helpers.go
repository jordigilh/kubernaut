package engine

import (
	"context"
	"time"
)

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

// isDestructiveAction checks if an action type is potentially destructive
func (iwb *DefaultIntelligentWorkflowBuilder) isDestructiveAction(actionType string) bool {
	destructiveActions := map[string]bool{
		"delete":     true,
		"destroy":    true,
		"drain":      true,
		"restart":    true,
		"rollback":   true,
		"scale_down": true,
		"terminate":  true,
		"remove":     true,
		"cleanup":    true,
	}

	return destructiveActions[actionType]
}

// calculateExecutionQuality calculates a quality score from execution results
func (iwb *DefaultIntelligentWorkflowBuilder) calculateExecutionQuality(execution *RuntimeWorkflowExecution) float64 {
	if execution.OperationalStatus != ExecutionStatusCompleted {
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
	// Learning integration not available
	iwb.log.Debug("Learning integration not available - skipping advanced pattern learning")

	return nil
}
