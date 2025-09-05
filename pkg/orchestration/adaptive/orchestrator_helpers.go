package orchestration

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/common"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	. "github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Helper methods for DefaultAdaptiveOrchestrator

func (dao *DefaultAdaptiveOrchestrator) applyAdaptationRules(ctx context.Context, workflow *Workflow, rules *AdaptationRules) error {
	for _, trigger := range rules.Triggers {
		triggered, err := dao.evaluateAdaptationTrigger(ctx, workflow, trigger)
		if err != nil {
			dao.log.WithError(err).WithField("workflow_id", workflow.ID).
				Warn("Failed to evaluate adaptation trigger")
			continue
		}

		if triggered {
			for _, action := range rules.Actions {
				if err := dao.executeAdaptationAction(workflow, action); err != nil {
					dao.log.WithError(err).WithFields(logrus.Fields{
						"workflow_id": workflow.ID,
						"action_type": action.Type,
						"target":      action.Target,
					}).Warn("Failed to execute adaptation action")
				}
			}
		}
	}
	return nil
}

func (dao *DefaultAdaptiveOrchestrator) evaluateAdaptationTrigger(ctx context.Context, workflow *Workflow, trigger *AdaptationTrigger) (bool, error) {
	switch trigger.Type {
	case TriggerTypePerformance:
		return dao.evaluatePerformanceTrigger(workflow, trigger)
	case TriggerTypeEffectiveness:
		return dao.evaluateEffectivenessTrigger(workflow, trigger)
	case TriggerTypeError:
		return dao.evaluateErrorTrigger(workflow, trigger)
	case TriggerTypeTime:
		return dao.evaluateTimeTrigger(workflow, trigger)
	case TriggerTypeMetric:
		return dao.evaluateMetricTrigger(ctx, workflow, trigger)
	default:
		return false, fmt.Errorf("unsupported trigger type: %s", trigger.Type)
	}
}

func (dao *DefaultAdaptiveOrchestrator) evaluatePerformanceTrigger(workflow *Workflow, trigger *AdaptationTrigger) (bool, error) {
	executions := dao.getWorkflowExecutions(workflow.ID)
	if len(executions) == 0 {
		return false, nil
	}

	// Calculate average duration
	var totalDuration time.Duration
	for _, exec := range executions {
		totalDuration += exec.Duration
	}
	avgDuration := totalDuration / time.Duration(len(executions))

	// Check if average duration exceeds threshold
	maxDuration := time.Duration(trigger.Threshold * float64(time.Minute))
	return avgDuration > maxDuration, nil
}

func (dao *DefaultAdaptiveOrchestrator) evaluateEffectivenessTrigger(workflow *Workflow, trigger *AdaptationTrigger) (bool, error) {
	executions := dao.getWorkflowExecutions(workflow.ID)
	if len(executions) == 0 {
		return false, nil
	}

	// Calculate success rate
	successCount := 0
	for _, exec := range executions {
		if exec.Status == ExecutionStatusCompleted {
			successCount++
		}
	}
	successRate := float64(successCount) / float64(len(executions))

	// Check if success rate is below threshold
	return successRate < trigger.Threshold, nil
}

func (dao *DefaultAdaptiveOrchestrator) evaluateErrorTrigger(workflow *Workflow, trigger *AdaptationTrigger) (bool, error) {
	executions := dao.getWorkflowExecutions(workflow.ID)
	if len(executions) == 0 {
		return false, nil
	}

	// Calculate failure rate
	failedCount := 0
	for _, exec := range executions {
		if exec.Status == ExecutionStatusFailed {
			failedCount++
		}
	}
	failureRate := float64(failedCount) / float64(len(executions))

	// Check if failure rate exceeds threshold
	return failureRate > trigger.Threshold, nil
}

func (dao *DefaultAdaptiveOrchestrator) evaluateTimeTrigger(workflow *Workflow, trigger *AdaptationTrigger) (bool, error) {
	executions := dao.getWorkflowExecutions(workflow.ID)
	if len(executions) == 0 {
		return false, nil
	}

	// Get most recent execution
	var lastExecution *engine.WorkflowExecution
	for _, exec := range executions {
		if lastExecution == nil || exec.StartTime.After(lastExecution.StartTime) {
			lastExecution = exec
		}
	}

	// Check if enough time has passed since last execution
	timeSinceLastExecution := time.Since(lastExecution.StartTime)
	thresholdDuration := time.Duration(trigger.Threshold * float64(time.Hour))
	return timeSinceLastExecution > thresholdDuration, nil
}

func (dao *DefaultAdaptiveOrchestrator) evaluateMetricTrigger(ctx context.Context, workflow *Workflow, trigger *AdaptationTrigger) (bool, error) {
	// Simple evaluation based on trigger threshold and recent execution status
	dao.executionMu.RLock()
	recentExecutions := make([]*engine.WorkflowExecution, 0)
	for _, exec := range dao.executions {
		if exec.WorkflowID == workflow.ID {
			recentExecutions = append(recentExecutions, exec)
		}
	}
	dao.executionMu.RUnlock()

	if len(recentExecutions) == 0 {
		return false, nil // No data to evaluate
	}

	// Calculate success rate as a simple metric
	successCount := 0
	for _, exec := range recentExecutions {
		if exec.Status == ExecutionStatusCompleted {
			successCount++
		}
	}

	successRate := float64(successCount) / float64(len(recentExecutions))

	// Simple threshold comparison - if success rate is below threshold, trigger adaptation
	return successRate < trigger.Threshold, nil
}

func (dao *DefaultAdaptiveOrchestrator) executeAdaptationAction(workflow *Workflow, action *AdaptationAction) error {
	switch action.Type {
	case AdaptationActionModifyTimeout:
		return dao.modifyWorkflowTimeout(workflow, action)
	case AdaptationActionModifyRetry:
		return dao.modifyWorkflowRetry(workflow, action)
	case AdaptationActionModifyParameter:
		return dao.modifyWorkflowParameter(workflow, action)
	case AdaptationActionAddStep:
		return dao.addWorkflowStep(workflow, action)
	case AdaptationActionRemoveStep:
		return dao.removeWorkflowStep(workflow, action)
	case AdaptationActionModifyStep:
		return dao.modifyWorkflowStep(workflow, action)
	default:
		return fmt.Errorf("unsupported adaptation action type: %s", action.Type)
	}
}

func (dao *DefaultAdaptiveOrchestrator) modifyWorkflowTimeout(workflow *Workflow, action *AdaptationAction) error {
	if timeout, ok := action.Value.(time.Duration); ok {
		if workflow.Template.Timeouts == nil {
			workflow.Template.Timeouts = &WorkflowTimeouts{}
		}
		workflow.Template.Timeouts.Execution = timeout
		dao.log.WithFields(logrus.Fields{
			"workflow_id": workflow.ID,
			"new_timeout": timeout,
		}).Info("Modified workflow timeout")
	}
	return nil
}

func (dao *DefaultAdaptiveOrchestrator) modifyWorkflowRetry(workflow *Workflow, action *AdaptationAction) error {
	if retries, ok := action.Value.(int); ok {
		// Find the target step and modify its retry policy
		for _, step := range workflow.Template.Steps {
			if step.ID == action.Target {
				if step.RetryPolicy == nil {
					step.RetryPolicy = &RetryPolicy{}
				}
				step.RetryPolicy.MaxRetries = retries
				dao.log.WithFields(logrus.Fields{
					"workflow_id": workflow.ID,
					"step_id":     step.ID,
					"new_retries": retries,
				}).Info("Modified step retry policy")
				break
			}
		}
	}
	return nil
}

func (dao *DefaultAdaptiveOrchestrator) modifyWorkflowParameter(workflow *Workflow, action *AdaptationAction) error {
	if workflow.Template.Variables == nil {
		workflow.Template.Variables = make(map[string]interface{})
	}
	workflow.Template.Variables[action.Target] = action.Value
	dao.log.WithFields(logrus.Fields{
		"workflow_id": workflow.ID,
		"parameter":   action.Target,
		"new_value":   action.Value,
	}).Info("Modified workflow parameter")
	return nil
}

func (dao *DefaultAdaptiveOrchestrator) addWorkflowStep(workflow *Workflow, action *AdaptationAction) error {
	// Parse the step to add from action value
	stepData, ok := action.Value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid step data for add action")
	}

	// Create new workflow step
	newStep := &engine.WorkflowStep{
		ID:      fmt.Sprintf("adaptive-step-%d", len(workflow.Template.Steps)+1),
		Name:    fmt.Sprintf("Adaptive Step: %s", stepData["name"]),
		Type:    engine.StepTypeAction,
		Timeout: 5 * time.Minute,
	}

	// Set step action based on provided data
	if actionType, exists := stepData["action_type"]; exists {
		newStep.Action = &engine.StepAction{
			Type:       actionType.(string),
			Parameters: make(map[string]interface{}),
		}

		if params, exists := stepData["parameters"]; exists {
			if paramMap, ok := params.(map[string]interface{}); ok {
				newStep.Action.Parameters = paramMap
			}
		}
	}

	// Determine insertion position
	insertPos := len(workflow.Template.Steps)
	if pos, exists := stepData["position"]; exists {
		if position, ok := pos.(int); ok && position >= 0 && position <= len(workflow.Template.Steps) {
			insertPos = position
		}
	}

	// Insert step at specified position
	steps := workflow.Template.Steps
	workflow.Template.Steps = append(steps[:insertPos], append([]*WorkflowStep{newStep}, steps[insertPos:]...)...)

	dao.log.WithFields(logrus.Fields{
		"workflow_id": workflow.ID,
		"step_id":     newStep.ID,
		"position":    insertPos,
	}).Info("Added adaptive workflow step")

	return nil
}

func (dao *DefaultAdaptiveOrchestrator) removeWorkflowStep(workflow *Workflow, action *AdaptationAction) error {
	stepID, ok := action.Value.(string)
	if !ok {
		return fmt.Errorf("invalid step ID for remove action")
	}

	// Find step index
	stepIndex := -1
	for i, step := range workflow.Template.Steps {
		if step.ID == stepID {
			stepIndex = i
			break
		}
	}

	if stepIndex == -1 {
		return fmt.Errorf("step %s not found in workflow", stepID)
	}

	// Check if step is critical (has dependencies)
	step := workflow.Template.Steps[stepIndex]
	if step.Type == StepTypeCondition || (step.Action != nil && step.Action.Type == "validate") {
		dao.log.WithFields(logrus.Fields{
			"workflow_id": workflow.ID,
			"step_id":     stepID,
		}).Warn("Attempting to remove critical step, adding confirmation")

		// Instead of removing, disable the step
		if step.Metadata == nil {
			step.Metadata = make(map[string]interface{})
		}
		step.Metadata["disabled"] = true
		step.Metadata["disabled_reason"] = "adaptive_removal"

		dao.log.WithField("step_id", stepID).Info("Disabled critical step instead of removing")
		return nil
	}

	// Remove step safely
	workflow.Template.Steps = append(workflow.Template.Steps[:stepIndex], workflow.Template.Steps[stepIndex+1:]...)

	dao.log.WithFields(logrus.Fields{
		"workflow_id": workflow.ID,
		"step_id":     stepID,
	}).Info("Removed workflow step")

	return nil
}

func (dao *DefaultAdaptiveOrchestrator) modifyWorkflowStep(workflow *Workflow, action *AdaptationAction) error {
	modifications, ok := action.Value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid modification data for modify action")
	}

	stepID, exists := modifications["step_id"]
	if !exists {
		return fmt.Errorf("step_id required for modification")
	}

	// Find step to modify
	var targetStep *WorkflowStep
	for _, step := range workflow.Template.Steps {
		if step.ID == stepID.(string) {
			targetStep = step
			break
		}
	}

	if targetStep == nil {
		return fmt.Errorf("step %s not found in workflow", stepID)
	}

	// Apply modifications
	if name, exists := modifications["name"]; exists {
		targetStep.Name = name.(string)
	}

	if timeout, exists := modifications["timeout"]; exists {
		if timeoutStr, ok := timeout.(string); ok {
			if duration, err := time.ParseDuration(timeoutStr); err == nil {
				targetStep.Timeout = duration
			}
		}
	}

	if actionMods, exists := modifications["action"]; exists {
		if actionModMap, ok := actionMods.(map[string]interface{}); ok {
			if targetStep.Action == nil {
				targetStep.Action = &engine.StepAction{
					Parameters: make(map[string]interface{}),
				}
			}

			if actionType, exists := actionModMap["type"]; exists {
				targetStep.Action.Type = actionType.(string)
			}

			if params, exists := actionModMap["parameters"]; exists {
				if paramMap, ok := params.(map[string]interface{}); ok {
					// Merge parameters
					if targetStep.Action.Parameters == nil {
						targetStep.Action.Parameters = make(map[string]interface{})
					}
					for k, v := range paramMap {
						targetStep.Action.Parameters[k] = v
					}
				}
			}
		}
	}

	dao.log.WithFields(logrus.Fields{
		"workflow_id": workflow.ID,
		"step_id":     stepID,
	}).Info("Modified workflow step")

	return nil
}

func (dao *DefaultAdaptiveOrchestrator) analyzeWorkflowPerformance(ctx context.Context, workflowID string) (*PerformanceAnalysis, error) {
	dao.executionMu.RLock()
	var executions []*engine.WorkflowExecution
	for _, execution := range dao.executions {
		if execution.WorkflowID == workflowID {
			executions = append(executions, execution)
		}
	}
	dao.executionMu.RUnlock()

	if len(executions) == 0 {
		return nil, fmt.Errorf("no executions found for workflow %s", workflowID)
	}

	// Calculate performance metrics
	var totalDuration time.Duration
	var completedExecutions int
	var failedExecutions int

	for _, execution := range executions {
		totalDuration += execution.Duration
		if execution.Status == ExecutionStatusCompleted {
			completedExecutions++
		} else if execution.Status == ExecutionStatusFailed {
			failedExecutions++
		}
	}

	avgDuration := totalDuration / time.Duration(len(executions))
	successRate := float64(completedExecutions) / float64(len(executions))

	analysis := &PerformanceAnalysis{
		WorkflowID:      workflowID,
		ExecutionTime:   avgDuration,
		ResourceUsage:   dao.calculateResourceUsage(executions),
		Bottlenecks:     dao.identifyBottlenecks(executions),
		Optimizations:   []*OptimizationCandidate{},
		Effectiveness:   successRate,
		CostEfficiency:  dao.calculateCostEfficiency(executions),
		Recommendations: []*OptimizationSuggestion{},
		AnalyzedAt:      time.Now(),
	}

	return analysis, nil
}

// calculateResourceUsage calculates average resource usage from executions
func (dao *DefaultAdaptiveOrchestrator) calculateResourceUsage(executions []*engine.WorkflowExecution) *engine.ResourceUsageMetrics {
	// Simple implementation - in practice this would aggregate real resource metrics
	return &engine.ResourceUsageMetrics{
		CPUUsage:    50.0, // Example values
		MemoryUsage: 1024,
		NetworkIO:   100,
		DiskUsage:   512,
	}
}

// calculateCostEfficiency calculates cost efficiency from executions
func (dao *DefaultAdaptiveOrchestrator) calculateCostEfficiency(executions []*engine.WorkflowExecution) float64 {
	// Simple implementation - in practice this would use real cost data
	successCount := 0
	for _, exec := range executions {
		if exec.Status == ExecutionStatusCompleted {
			successCount++
		}
	}
	return float64(successCount) / float64(len(executions))
}

func (dao *DefaultAdaptiveOrchestrator) identifyBottlenecks(executions []*engine.WorkflowExecution) []*Bottleneck {
	var bottlenecks []*Bottleneck

	// Analyze step durations to identify bottlenecks
	stepDurations := make(map[string][]time.Duration)

	for _, execution := range executions {
		for _, step := range execution.Steps {
			if step.Status == ExecutionStatusCompleted {
				stepDurations[step.StepID] = append(stepDurations[step.StepID], step.Duration)
			}
		}
	}

	// Find steps that take significantly longer than others
	for stepID, durations := range stepDurations {
		if len(durations) < 2 {
			continue
		}

		var totalDuration time.Duration
		for _, duration := range durations {
			totalDuration += duration
		}
		avgDuration := totalDuration / time.Duration(len(durations))

		// If step takes more than 30% of total workflow time, consider it a bottleneck
		if len(executions) > 0 {
			avgWorkflowDuration := executions[len(executions)-1].Duration // Use last execution as reference
			if avgDuration > avgWorkflowDuration*30/100 {
				bottlenecks = append(bottlenecks, &Bottleneck{
					ID:          fmt.Sprintf("bottleneck-%s", stepID),
					StepID:      stepID,
					Type:        BottleneckTypeLogical,
					Severity:    "warning",
					Impact:      float64(avgDuration) / float64(avgWorkflowDuration),
					Description: fmt.Sprintf("Step %s consumes %.1f%% of workflow execution time", stepID, float64(avgDuration)/float64(avgWorkflowDuration)*100),
					Suggestion:  "Consider optimizing step logic or evaluating parallel execution options",
				})
			}
		}
	}

	return bottlenecks
}

func (dao *DefaultAdaptiveOrchestrator) selectBestOptimizationCandidate(candidates []*OptimizationCandidate) *OptimizationCandidate {
	if len(candidates) == 0 {
		return nil
	}

	// Sort candidates by a combination of confidence, impact, and ROI
	sort.Slice(candidates, func(i, j int) bool {
		scoreI := dao.calculateOptimizationScore(candidates[i])
		scoreJ := dao.calculateOptimizationScore(candidates[j])
		return scoreI > scoreJ
	})

	return candidates[0]
}

func (dao *DefaultAdaptiveOrchestrator) calculateOptimizationScore(candidate *OptimizationCandidate) float64 {
	// Weighted scoring: confidence (60%) + impact (40%)

	// Calculate score based on confidence and impact
	score := candidate.Confidence*0.6 + candidate.Impact*0.4
	return score
}

func (dao *DefaultAdaptiveOrchestrator) extractExecutionLearnings(execution *engine.WorkflowExecution) []*WorkflowLearning {
	var learnings []*WorkflowLearning

	// Learn from execution duration
	if execution.Duration > 0 {
		learning := &WorkflowLearning{
			ID:         generateLearningID(),
			WorkflowID: execution.WorkflowID,
			Type:       LearningTypePerformance,
			Trigger:    "execution_completed",
			Data:       map[string]interface{}{"duration_ms": execution.Duration.Milliseconds(), "status": execution.Status},
			Actions:    []*LearningAction{},
			Applied:    false,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if execution.Duration > 30*time.Minute {
			learning.Actions = append(learning.Actions, &LearningAction{
				ID:         generateLearningActionID(),
				Type:       LearningActionTypeOptimizeWorkflow,
				Target:     execution.WorkflowID,
				Parameters: map[string]interface{}{"suggestion": "Consider optimizing workflow due to long execution time"},
				Applied:    false,
				CreatedAt:  time.Now(),
			})
		}

		learnings = append(learnings, learning)
	}

	// Learn from failure patterns
	if execution.Status == ExecutionStatusFailed {
		learning := &WorkflowLearning{
			ID:         generateLearningID(),
			WorkflowID: execution.WorkflowID,
			Type:       LearningTypeFailure,
			Trigger:    "execution_failed",
			Data:       map[string]interface{}{"error": execution.Error, "execution_id": execution.ID},
			Actions: []*LearningAction{
				{
					ID:         generateLearningActionID(),
					Type:       LearningActionTypeImproveRecovery,
					Target:     execution.WorkflowID,
					Parameters: map[string]interface{}{"suggestion": "Improve failure recovery mechanisms"},
					Applied:    false,
					CreatedAt:  time.Now(),
				},
			},
			Applied:   false,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		learnings = append(learnings, learning)
	}

	// Learn from step patterns
	for _, step := range execution.Steps {
		if step.Status == ExecutionStatusFailed {
			learning := &WorkflowLearning{
				ID:         generateLearningID(),
				WorkflowID: execution.WorkflowID,
				Type:       LearningTypeFailure,
				Trigger:    "step_failed",
				Data:       map[string]interface{}{"step_id": step.StepID, "error": step.Error, "execution_id": execution.ID},
				Actions: []*LearningAction{
					{
						ID:         generateLearningActionID(),
						Type:       LearningActionTypeAddValidation,
						Target:     step.StepID,
						Parameters: map[string]interface{}{"suggestion": "Add validation for step to prevent similar failures"},
						Applied:    false,
						CreatedAt:  time.Now(),
					},
				},
				Applied:   false,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			learnings = append(learnings, learning)
		}
	}

	return learnings
}

// generateLearningActionID generates a unique ID for learning actions
func generateLearningActionID() string {
	return "action-" + uuid.New().String()
}

func (dao *DefaultAdaptiveOrchestrator) patternToRecommendation(pattern *vector.ActionPattern, context *ActionContext) *WorkflowRecommendation {
	if pattern.EffectivenessData == nil {
		return nil
	}

	// Create a workflow recommendation based on the pattern
	// Calculate confidence from success rate and execution count
	executionCount := pattern.EffectivenessData.SuccessCount + pattern.EffectivenessData.FailureCount
	successRate := float64(pattern.EffectivenessData.SuccessCount) / math.Max(float64(executionCount), 1.0)
	confidence := successRate * math.Min(float64(executionCount)/10.0, 1.0) // Normalize by execution count

	return &WorkflowRecommendation{
		WorkflowID:    fmt.Sprintf("pattern-%s", pattern.ID),
		Name:          fmt.Sprintf("%s for %s", pattern.ActionType, pattern.AlertName),
		Description:   fmt.Sprintf("Workflow based on successful pattern with %.1f%% effectiveness", pattern.EffectivenessData.Score*100),
		Confidence:    confidence,
		Reason:        fmt.Sprintf("Similar pattern found with %d successful executions", executionCount),
		Parameters:    pattern.ActionParameters,
		Priority:      dao.effectivenessScoreToPriority(pattern.EffectivenessData.Score),
		Effectiveness: pattern.EffectivenessData.Score,
		Risk:          dao.effectivenessScoreToRisk(pattern.EffectivenessData.Score),
	}
}

func (dao *DefaultAdaptiveOrchestrator) insightsToRecommendations(insights *common.AnalyticsInsights, context *ActionContext) []*WorkflowRecommendation {
	var recommendations []*WorkflowRecommendation

	// Extract recommendations from action type analysis
	if insights.ActionTypeAnalysis != nil {
		for actionType, analysis := range insights.ActionTypeAnalysis {
			if analysis.SuccessRate > 0.8 {
				rec := &WorkflowRecommendation{
					WorkflowID:    fmt.Sprintf("analytics-%s", actionType),
					Name:          fmt.Sprintf("Optimized %s workflow", actionType),
					Description:   fmt.Sprintf("Analytics-driven workflow with %.1f%% success rate", analysis.SuccessRate*100),
					Confidence:    analysis.SuccessRate,
					Reason:        "Based on analytics of successful actions",
					Priority:      PriorityMedium,
					Effectiveness: analysis.SuccessRate,
					Risk:          RiskLevelLow,
				}
				recommendations = append(recommendations, rec)
			}
		}
	}

	return recommendations
}

func (dao *DefaultAdaptiveOrchestrator) sortRecommendations(recommendations []*WorkflowRecommendation) {
	sort.Slice(recommendations, func(i, j int) bool {
		// Sort by confidence * effectiveness (descending)
		scoreI := recommendations[i].Confidence * recommendations[i].Effectiveness
		scoreJ := recommendations[j].Confidence * recommendations[j].Effectiveness
		return scoreI > scoreJ
	})
}

func (dao *DefaultAdaptiveOrchestrator) effectivenessScoreToPriority(score float64) Priority {
	if score >= 0.9 {
		return PriorityHigh
	} else if score >= 0.7 {
		return PriorityMedium
	} else {
		return PriorityLow
	}
}

func (dao *DefaultAdaptiveOrchestrator) effectivenessScoreToRisk(score float64) RiskLevel {
	if score >= 0.8 {
		return RiskLevelLow
	} else if score >= 0.6 {
		return RiskLevelMedium
	} else {
		return RiskLevelHigh
	}
}

func (dao *DefaultAdaptiveOrchestrator) getPreviousStepResults(execution *engine.WorkflowExecution, currentStepIndex int) []*StepResult {
	var results []*StepResult
	for i := 0; i < currentStepIndex; i++ {
		if execution.Steps[i].Result != nil {
			results = append(results, execution.Steps[i].Result)
		}
	}
	return results
}

func (dao *DefaultAdaptiveOrchestrator) handleStepFailure(ctx context.Context, execution *engine.WorkflowExecution, step *WorkflowStep, stepIndex int, err error) bool {
	if step.RetryPolicy == nil || step.RetryPolicy.MaxRetries == 0 {
		return false
	}

	stepExecution := execution.Steps[stepIndex]
	if stepExecution.RetryCount >= step.RetryPolicy.MaxRetries {
		return false
	}

	// Wait before retrying
	if step.RetryPolicy.Delay > 0 {
		time.Sleep(step.RetryPolicy.Delay)
	}

	stepExecution.RetryCount++
	stepExecution.Status = ExecutionStatusPending

	dao.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"step_id":      step.ID,
		"retry_count":  stepExecution.RetryCount,
		"max_retries":  step.RetryPolicy.MaxRetries,
	}).Info("Retrying failed step")

	// The step will be retried in the main execution loop
	return true
}

// Utility functions

// getWorkflowExecutions gets all executions for a given workflow ID
func (dao *DefaultAdaptiveOrchestrator) getWorkflowExecutions(workflowID string) []*engine.WorkflowExecution {
	dao.executionMu.RLock()
	defer dao.executionMu.RUnlock()

	var executions []*engine.WorkflowExecution
	for _, exec := range dao.executions {
		if exec.WorkflowID == workflowID {
			executions = append(executions, exec)
		}
	}
	return executions
}

func generateLearningID() string {
	return "learning-" + strings.Replace(uuid.New().String(), "-", "", -1)[:16]
}
