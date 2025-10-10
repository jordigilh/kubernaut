<<<<<<< HEAD
=======
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

>>>>>>> crd_implementation
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
func (iwb *DefaultIntelligentWorkflowBuilder) UpdateWorkflowPatterns(ctx context.Context, learnings []*WorkflowLearning) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		iwb.log.WithContext(ctx).Warn("Context cancelled during pattern update")
		return
	default:
	}

	if len(learnings) == 0 {
		iwb.log.WithContext(ctx).Debug("No learnings provided for pattern update")
		return
	}

	// Process learnings to update internal patterns
	iwb.log.WithContext(ctx).WithField("learning_count", len(learnings)).Debug("Processing workflow pattern learnings")

	for _, learning := range learnings {
		if learning == nil {
			continue
		}

		// Check for context cancellation during processing
		select {
		case <-ctx.Done():
			iwb.log.WithContext(ctx).Warn("Context cancelled while processing learnings")
			return
		default:
		}

		// Update internal pattern knowledge based on learning
		successRate := 0.5 // default success rate
		if rate, ok := learning.Data["success_rate"].(float64); ok {
			successRate = rate
		}

		iwb.log.WithContext(ctx).WithFields(map[string]interface{}{
			"learning_type": learning.Type,
			"success_rate":  successRate,
		}).Debug("Updating pattern from learning")

		// In a full implementation, this would update the pattern database
		// For now, we'll simulate the pattern update process
		if successRate > 0.8 {
			iwb.log.WithContext(ctx).Debug("High success rate learning - promoting pattern")
		} else if successRate < 0.3 {
			iwb.log.WithContext(ctx).Debug("Low success rate learning - demoting pattern")
		}
	}

	iwb.log.WithContext(ctx).Info("Completed workflow pattern updates")
}
