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

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Helper methods for DefaultAdaptiveOrchestrator

func (dao *DefaultAdaptiveOrchestrator) applyAdaptationRules(ctx context.Context, workflow *engine.Workflow, rules *engine.AdaptationRules) error {
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

func (dao *DefaultAdaptiveOrchestrator) evaluateAdaptationTrigger(ctx context.Context, workflow *engine.Workflow, trigger *engine.AdaptationTrigger) (bool, error) {
	switch trigger.Type {
	case engine.TriggerTypePerformance:
		return dao.evaluatePerformanceTrigger(workflow, trigger)
	case engine.TriggerTypeEffectiveness:
		return dao.evaluateEffectivenessTrigger(workflow, trigger)
	case engine.TriggerTypeError:
		return dao.evaluateErrorTrigger(workflow, trigger)
	case engine.TriggerTypeTime:
		return dao.evaluateTimeTrigger(workflow, trigger)
	case engine.TriggerTypeMetric:
		return dao.evaluateMetricTrigger(ctx, workflow, trigger)
	default:
		return false, fmt.Errorf("unsupported trigger type: %s", trigger.Type)
	}
}

func (dao *DefaultAdaptiveOrchestrator) evaluatePerformanceTrigger(workflow *engine.Workflow, trigger *engine.AdaptationTrigger) (bool, error) {
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

func (dao *DefaultAdaptiveOrchestrator) evaluateEffectivenessTrigger(workflow *engine.Workflow, trigger *engine.AdaptationTrigger) (bool, error) {
	executions := dao.getWorkflowExecutions(workflow.ID)
	if len(executions) == 0 {
		return false, nil
	}

	// Calculate success rate
	successCount := 0
	for _, exec := range executions {
		if exec.Status == string(engine.ExecutionStatusCompleted) {
			successCount++
		}
	}
	successRate := float64(successCount) / float64(len(executions))

	// Check if success rate is below threshold
	return successRate < trigger.Threshold, nil
}

func (dao *DefaultAdaptiveOrchestrator) evaluateErrorTrigger(workflow *engine.Workflow, trigger *engine.AdaptationTrigger) (bool, error) {
	executions := dao.getWorkflowExecutions(workflow.ID)
	if len(executions) == 0 {
		return false, nil
	}

	// Calculate failure rate
	failedCount := 0
	for _, exec := range executions {
		if exec.Status == string(engine.ExecutionStatusFailed) {
			failedCount++
		}
	}
	failureRate := float64(failedCount) / float64(len(executions))

	// Check if failure rate exceeds threshold
	return failureRate > trigger.Threshold, nil
}

func (dao *DefaultAdaptiveOrchestrator) evaluateTimeTrigger(workflow *engine.Workflow, trigger *engine.AdaptationTrigger) (bool, error) {
	executions := dao.getWorkflowExecutions(workflow.ID)
	if len(executions) == 0 {
		return false, nil
	}

	// Get most recent execution
	var lastExecution *engine.RuntimeWorkflowExecution
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

func (dao *DefaultAdaptiveOrchestrator) evaluateMetricTrigger(ctx context.Context, workflow *engine.Workflow, trigger *engine.AdaptationTrigger) (bool, error) {
	// Simple evaluation based on trigger threshold and recent execution status
	dao.executionMu.RLock()
	recentExecutions := make([]*engine.RuntimeWorkflowExecution, 0)
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
		if exec.Status == string(engine.ExecutionStatusCompleted) {
			successCount++
		}
	}

	successRate := float64(successCount) / float64(len(recentExecutions))

	// Simple threshold comparison - if success rate is below threshold, trigger adaptation
	return successRate < trigger.Threshold, nil
}

func (dao *DefaultAdaptiveOrchestrator) executeAdaptationAction(workflow *engine.Workflow, action *engine.AdaptationAction) error {
	switch action.Type {
	case engine.AdaptationActionModifyTimeout:
		return dao.modifyWorkflowTimeout(workflow, action)
	case engine.AdaptationActionModifyRetry:
		return dao.modifyWorkflowRetry(workflow, action)
	case engine.AdaptationActionModifyParameter:
		return dao.modifyWorkflowParameter(workflow, action)
	case engine.AdaptationActionAddStep:
		return dao.addWorkflowStep(workflow, action)
	case engine.AdaptationActionRemoveStep:
		return dao.removeWorkflowStep(workflow, action)
	case engine.AdaptationActionModifyStep:
		return dao.modifyWorkflowStep(workflow, action)
	default:
		return fmt.Errorf("unsupported adaptation action type: %s", action.Type)
	}
}

func (dao *DefaultAdaptiveOrchestrator) modifyWorkflowTimeout(workflow *engine.Workflow, action *engine.AdaptationAction) error {
	if timeout, ok := action.Value.(time.Duration); ok {
		if workflow.Template.Timeouts == nil {
			workflow.Template.Timeouts = &engine.WorkflowTimeouts{}
		}
		workflow.Template.Timeouts.Execution = timeout
		dao.log.WithFields(logrus.Fields{
			"workflow_id": workflow.ID,
			"new_timeout": timeout,
		}).Info("Modified workflow timeout")
	}
	return nil
}

func (dao *DefaultAdaptiveOrchestrator) modifyWorkflowRetry(workflow *engine.Workflow, action *engine.AdaptationAction) error {
	if retries, ok := action.Value.(int); ok {
		// Find the target step and modify its retry policy
		for _, step := range workflow.Template.Steps {
			if step.ID == action.Target {
				if step.RetryPolicy == nil {
					step.RetryPolicy = &engine.RetryPolicy{}
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

func (dao *DefaultAdaptiveOrchestrator) modifyWorkflowParameter(workflow *engine.Workflow, action *engine.AdaptationAction) error {
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

func (dao *DefaultAdaptiveOrchestrator) addWorkflowStep(workflow *engine.Workflow, action *engine.AdaptationAction) error {
	// Parse the step to add from action value
	stepData, ok := action.Value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid step data for add action")
	}

	// Create new workflow step
	newStep := &engine.ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   fmt.Sprintf("adaptive-step-%d", len(workflow.Template.Steps)+1),
			Name: fmt.Sprintf("Adaptive Step: %s", stepData["name"]),
		},
		Type:    engine.StepTypeAction,
		Timeout: 5 * time.Minute,
	}

	// Set step action based on provided data
	// Following Option 2A: Graceful degradation with warning logs
	// Following Option 3B: Rigid type checking with proper validation
	if actionType, exists := stepData["action_type"]; exists {
		if actionTypeStr, ok := actionType.(string); ok {
			newStep.Action = &engine.StepAction{
				Type:       actionTypeStr,
				Parameters: make(map[string]interface{}),
			}
		} else {
			dao.log.WithFields(logrus.Fields{
				"workflow_id":   workflow.ID,
				"action_type":   fmt.Sprintf("%T", actionType),
				"expected_type": "string",
			}).Warn("Invalid action_type in step data, using default")
			newStep.Action = &engine.StepAction{
				Type:       "default",
				Parameters: make(map[string]interface{}),
			}
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
	workflow.Template.Steps = append(steps[:insertPos], append([]*engine.ExecutableWorkflowStep{newStep}, steps[insertPos:]...)...)

	dao.log.WithFields(logrus.Fields{
		"workflow_id": workflow.ID,
		"step_id":     newStep.ID,
		"position":    insertPos,
	}).Info("Added adaptive workflow step")

	return nil
}

func (dao *DefaultAdaptiveOrchestrator) removeWorkflowStep(workflow *engine.Workflow, action *engine.AdaptationAction) error {
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
	if step.Type == engine.StepTypeCondition || (step.Action != nil && step.Action.Type == "validate") {
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

func (dao *DefaultAdaptiveOrchestrator) modifyWorkflowStep(workflow *engine.Workflow, action *engine.AdaptationAction) error {
	modifications, ok := action.Value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid modification data for modify action")
	}

	stepID, exists := modifications["step_id"]
	if !exists {
		return fmt.Errorf("step_id required for modification")
	}

	// Following Option 2A: Graceful degradation with warning logs
	// Following Option 3B: Rigid type checking with proper validation
	stepIDStr, ok := stepID.(string)
	if !ok {
		return fmt.Errorf("step_id must be string, got %T", stepID)
	}

	// Find step to modify
	var targetStep *engine.ExecutableWorkflowStep
	for _, step := range workflow.Template.Steps {
		if step.ID == stepIDStr {
			targetStep = step
			break
		}
	}

	if targetStep == nil {
		return fmt.Errorf("step %s not found in workflow", stepIDStr)
	}

	// Apply modifications
	// Following Option 2A: Graceful degradation with warning logs
	// Following Option 3B: Rigid type checking with proper validation
	if name, exists := modifications["name"]; exists {
		if nameStr, ok := name.(string); ok {
			targetStep.Name = nameStr
		} else {
			dao.log.WithFields(logrus.Fields{
				"workflow_id":   workflow.ID,
				"step_id":       stepIDStr,
				"name_type":     fmt.Sprintf("%T", name),
				"expected_type": "string",
			}).Warn("Invalid name type in modification data, skipping name change")
		}
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

			// Following Option 2A: Graceful degradation with warning logs
			// Following Option 3B: Rigid type checking with proper validation
			if actionType, exists := actionModMap["type"]; exists {
				if actionTypeStr, ok := actionType.(string); ok {
					targetStep.Action.Type = actionTypeStr
				} else {
					dao.log.WithFields(logrus.Fields{
						"workflow_id":   workflow.ID,
						"step_id":       stepIDStr,
						"action_type":   fmt.Sprintf("%T", actionType),
						"expected_type": "string",
					}).Warn("Invalid action type in modification data, skipping action type change")
				}
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
		"step_id":     stepIDStr,
	}).Info("Modified workflow step")

	return nil
}

func (dao *DefaultAdaptiveOrchestrator) analyzeWorkflowPerformance(ctx context.Context, workflowID string) (*engine.PerformanceAnalysis, error) {
	dao.executionMu.RLock()
	var executions []*engine.RuntimeWorkflowExecution
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
		if execution.Status == string(engine.ExecutionStatusCompleted) {
			completedExecutions++
		} else if execution.Status == string(engine.ExecutionStatusFailed) {
			failedExecutions++
		}
	}

	avgDuration := totalDuration / time.Duration(len(executions))
	successRate := float64(completedExecutions) / float64(len(executions))

	analysis := &engine.PerformanceAnalysis{
		WorkflowID:      workflowID,
		ExecutionTime:   avgDuration,
		ResourceUsage:   dao.calculateResourceUsage(executions),
		Bottlenecks:     dao.identifyBottlenecks(executions),
		Optimizations:   []*engine.OptimizationCandidate{},
		Effectiveness:   successRate,
		CostEfficiency:  dao.calculateCostEfficiency(executions),
		Recommendations: []*engine.OptimizationSuggestion{},
		AnalyzedAt:      time.Now(),
	}

	return analysis, nil
}

// calculateResourceUsage calculates average resource usage from executions
func (dao *DefaultAdaptiveOrchestrator) calculateResourceUsage(executions []*engine.RuntimeWorkflowExecution) *engine.ResourceUsageMetrics {
	// Simple implementation - in practice this would aggregate real resource metrics
	return &engine.ResourceUsageMetrics{
		CPUUsage:    50.0, // Example values
		MemoryUsage: 1024,
		NetworkIO:   100,
		DiskUsage:   512,
	}
}

// calculateCostEfficiency calculates cost efficiency from executions
func (dao *DefaultAdaptiveOrchestrator) calculateCostEfficiency(executions []*engine.RuntimeWorkflowExecution) float64 {
	// Simple implementation - in practice this would use real cost data
	successCount := 0
	for _, exec := range executions {
		if exec.Status == string(engine.ExecutionStatusCompleted) {
			successCount++
		}
	}
	return float64(successCount) / float64(len(executions))
}

func (dao *DefaultAdaptiveOrchestrator) identifyBottlenecks(executions []*engine.RuntimeWorkflowExecution) []*engine.Bottleneck {
	var bottlenecks []*engine.Bottleneck

	// Analyze step durations to identify bottlenecks
	stepDurations := make(map[string][]time.Duration)

	for _, execution := range executions {
		for _, step := range execution.Steps {
			if step.Status == engine.ExecutionStatusCompleted {
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
				bottlenecks = append(bottlenecks, &engine.Bottleneck{
					ID:          fmt.Sprintf("bottleneck-%s", stepID),
					StepID:      stepID,
					Type:        engine.BottleneckTypeLogical,
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

func (dao *DefaultAdaptiveOrchestrator) selectBestOptimizationCandidate(candidates []*engine.OptimizationCandidate) *engine.OptimizationCandidate {
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

func (dao *DefaultAdaptiveOrchestrator) calculateOptimizationScore(candidate *engine.OptimizationCandidate) float64 {
	// Weighted scoring: confidence (60%) + impact (40%)

	// Calculate score based on confidence and impact
	score := candidate.Confidence*0.6 + candidate.Impact*0.4
	return score
}

func (dao *DefaultAdaptiveOrchestrator) extractExecutionLearnings(execution *engine.RuntimeWorkflowExecution) []*engine.WorkflowLearning {
	var learnings []*engine.WorkflowLearning

	// Learn from execution duration
	if execution.Duration > 0 {
		learning := &engine.WorkflowLearning{
			ID:         generateLearningID(),
			WorkflowID: execution.WorkflowID,
			Type:       engine.LearningTypePerformance,
			Trigger:    "execution_completed",
			Data:       map[string]interface{}{"duration_ms": execution.Duration.Milliseconds(), "status": execution.Status},
			Actions:    []*engine.LearningAction{},
			Applied:    false,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if execution.Duration > 30*time.Minute {
			learning.Actions = append(learning.Actions, &engine.LearningAction{
				ID:         generateLearningActionID(),
				Type:       engine.LearningActionTypeOptimizeWorkflow,
				Target:     execution.WorkflowID,
				Parameters: map[string]interface{}{"suggestion": "Consider optimizing workflow due to long execution time"},
				Applied:    false,
				CreatedAt:  time.Now(),
			})
		}

		learnings = append(learnings, learning)
	}

	// Learn from failure patterns
	if execution.Status == string(engine.ExecutionStatusFailed) {
		learning := &engine.WorkflowLearning{
			ID:         generateLearningID(),
			WorkflowID: execution.WorkflowID,
			Type:       engine.LearningTypeFailure,
			Trigger:    "execution_failed",
			Data:       map[string]interface{}{"error": execution.Error, "execution_id": execution.ID},
			Actions: []*engine.LearningAction{
				{
					ID:         generateLearningActionID(),
					Type:       engine.LearningActionTypeImproveRecovery,
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
		if step.Status == engine.ExecutionStatusFailed {
			learning := &engine.WorkflowLearning{
				ID:         generateLearningID(),
				WorkflowID: execution.WorkflowID,
				Type:       engine.LearningTypeFailure,
				Trigger:    "step_failed",
				Data:       map[string]interface{}{"step_id": step.StepID, "error": step.Error, "execution_id": execution.ID},
				Actions: []*engine.LearningAction{
					{
						ID:         generateLearningActionID(),
						Type:       engine.LearningActionTypeAddValidation,
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

// generateengine.LearningActionID generates a unique ID for learning actions
func generateLearningActionID() string {
	return "action-" + uuid.New().String()
}

func (dao *DefaultAdaptiveOrchestrator) patternToRecommendation(pattern *vector.ActionPattern, context *engine.ActionContext) *engine.WorkflowRecommendation {
	if pattern.EffectivenessData == nil {
		return nil
	}

	// Create a workflow recommendation based on the pattern
	// Calculate confidence from success rate and execution count
	executionCount := pattern.EffectivenessData.SuccessCount + pattern.EffectivenessData.FailureCount
	successRate := float64(pattern.EffectivenessData.SuccessCount) / math.Max(float64(executionCount), 1.0)
	confidence := successRate * math.Min(float64(executionCount)/10.0, 1.0) // Normalize by execution count

	return &engine.WorkflowRecommendation{
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

// ActionPerformanceData represents expected structure for action performance analysis
// Following Option 3B: Define structs for expected data formats (rigid type safety)
type ActionPerformanceData struct {
	ActionType     string  `json:"action_type"`
	SuccessRate    float64 `json:"success_rate"`
	ExecutionCount int     `json:"execution_count"`
	AvgDuration    float64 `json:"avg_duration_ms"`
}

// PatternInsightData represents expected structure for pattern insights
type PatternInsightData struct {
	PatternType   string  `json:"pattern_type"`
	Effectiveness float64 `json:"effectiveness"`
	Confidence    float64 `json:"confidence"`
	UsageCount    int     `json:"usage_count"`
}

func (dao *DefaultAdaptiveOrchestrator) insightsToRecommendations(insights *types.AnalyticsInsights, context *engine.ActionContext) []*engine.WorkflowRecommendation {
	var recommendations []*engine.WorkflowRecommendation

	// Option 1B: Use both WorkflowInsights and PatternInsights for comprehensive analysis
	// Option 2A: Graceful degradation with warning logs

	// Extract from WorkflowInsights["action_performance"]
	if actionPerfRecs := dao.extractActionPerformanceRecommendations(insights.WorkflowInsights, context); len(actionPerfRecs) > 0 {
		recommendations = append(recommendations, actionPerfRecs...)
		dao.log.WithField("action_performance_recs", len(actionPerfRecs)).Debug("Generated action performance recommendations")
	}

	// Extract from PatternInsights
	if patternRecs := dao.extractPatternInsightRecommendations(insights.PatternInsights, context); len(patternRecs) > 0 {
		recommendations = append(recommendations, patternRecs...)
		dao.log.WithField("pattern_insight_recs", len(patternRecs)).Debug("Generated pattern insight recommendations")
	}

	// Fallback: Use direct recommendations field
	if len(recommendations) == 0 && len(insights.Recommendations) > 0 {
		fallbackRecs := dao.convertStringRecommendations(insights.Recommendations, context)
		recommendations = append(recommendations, fallbackRecs...)
		dao.log.WithField("fallback_recs", len(fallbackRecs)).Debug("Using fallback string recommendations")
	}

	return recommendations
}

// extractActionPerformanceRecommendations extracts recommendations from workflow insights action performance data
// Following Option 2A: Graceful degradation with warning logs
func (dao *DefaultAdaptiveOrchestrator) extractActionPerformanceRecommendations(workflowInsights map[string]interface{}, context *engine.ActionContext) []*engine.WorkflowRecommendation {
	var recommendations []*engine.WorkflowRecommendation

	actionPerfRaw, exists := workflowInsights["action_performance"]
	if !exists {
		dao.log.Warn("No action_performance data found in workflow insights")
		return recommendations
	}

	// Option 3B: Rigid type checking with struct validation
	actionPerfMap, ok := actionPerfRaw.(map[string]interface{})
	if !ok {
		dao.log.WithField("type", fmt.Sprintf("%T", actionPerfRaw)).Warn("action_performance data is not a map, skipping")
		return recommendations
	}

	for actionType, analysisRaw := range actionPerfMap {
		analysis, err := dao.parseActionPerformanceData(analysisRaw)
		if err != nil {
			dao.log.WithFields(logrus.Fields{
				"action_type": actionType,
				"error":       err,
			}).Warn("Failed to parse action performance data, skipping")
			continue
		}

		// Only recommend high-performing actions (>80% success rate)
		if analysis.SuccessRate > 0.8 && analysis.ExecutionCount >= 5 {
			rec := &engine.WorkflowRecommendation{
				WorkflowID:    fmt.Sprintf("analytics-action-%s", actionType),
				Name:          fmt.Sprintf("Optimized %s Workflow", actionType),
				Description:   fmt.Sprintf("Analytics-driven workflow with %.1f%% success rate over %d executions", analysis.SuccessRate*100, analysis.ExecutionCount),
				Confidence:    analysis.SuccessRate,
				Reason:        fmt.Sprintf("Based on %d successful executions with consistent performance", analysis.ExecutionCount),
				Priority:      dao.successRateToPriority(analysis.SuccessRate),
				Effectiveness: analysis.SuccessRate,
				Risk:          dao.successRateToRisk(analysis.SuccessRate),
				Parameters: map[string]interface{}{
					"source_action_type": actionType,
					"execution_count":    analysis.ExecutionCount,
					"avg_duration_ms":    analysis.AvgDuration,
				},
			}
			recommendations = append(recommendations, rec)
		}
	}

	return recommendations
}

// extractPatternInsightRecommendations extracts recommendations from pattern insights
func (dao *DefaultAdaptiveOrchestrator) extractPatternInsightRecommendations(patternInsights map[string]interface{}, context *engine.ActionContext) []*engine.WorkflowRecommendation {
	var recommendations []*engine.WorkflowRecommendation

	if len(patternInsights) == 0 {
		dao.log.Debug("No pattern insights available")
		return recommendations
	}

	for patternID, insightRaw := range patternInsights {
		insight, err := dao.parsePatternInsightData(insightRaw)
		if err != nil {
			dao.log.WithFields(logrus.Fields{
				"pattern_id": patternID,
				"error":      err,
			}).Warn("Failed to parse pattern insight data, skipping")
			continue
		}

		// Only recommend highly effective patterns (>70% effectiveness)
		if insight.Effectiveness > 0.7 && insight.Confidence > 0.6 && insight.UsageCount >= 3 {
			rec := &engine.WorkflowRecommendation{
				WorkflowID:    fmt.Sprintf("analytics-pattern-%s", patternID),
				Name:          fmt.Sprintf("Pattern-Based %s Workflow", insight.PatternType),
				Description:   fmt.Sprintf("Pattern-driven workflow with %.1f%% effectiveness and %.1f%% confidence", insight.Effectiveness*100, insight.Confidence*100),
				Confidence:    insight.Confidence,
				Reason:        fmt.Sprintf("Based on proven pattern with %d successful applications", insight.UsageCount),
				Priority:      dao.effectivenessScoreToPriority(insight.Effectiveness),
				Effectiveness: insight.Effectiveness,
				Risk:          dao.effectivenessScoreToRisk(insight.Effectiveness),
				Parameters: map[string]interface{}{
					"source_pattern_id": patternID,
					"pattern_type":      insight.PatternType,
					"usage_count":       insight.UsageCount,
					"confidence":        insight.Confidence,
				},
			}
			recommendations = append(recommendations, rec)
		}
	}

	return recommendations
}

// convertStringRecommendations converts string recommendations to WorkflowRecommendation objects
func (dao *DefaultAdaptiveOrchestrator) convertStringRecommendations(recommendations []string, context *engine.ActionContext) []*engine.WorkflowRecommendation {
	var workflowRecs []*engine.WorkflowRecommendation

	for i, rec := range recommendations {
		workflowRec := &engine.WorkflowRecommendation{
			WorkflowID:    fmt.Sprintf("analytics-string-%d", i),
			Name:          fmt.Sprintf("Analytics Recommendation %d", i+1),
			Description:   rec,
			Confidence:    0.6, // Moderate confidence for string-based recommendations
			Reason:        "Generated from analytics insights",
			Priority:      engine.PriorityMedium,
			Effectiveness: 0.7, // Assume reasonable effectiveness
			Risk:          engine.RiskLevelMedium,
			Parameters: map[string]interface{}{
				"source":   "string_recommendation",
				"original": rec,
			},
		}
		workflowRecs = append(workflowRecs, workflowRec)
	}

	return workflowRecs
}

// parseActionPerformanceData parses raw interface{} data into ActionPerformanceData struct
// Following Option 3B: Rigid type checking
func (dao *DefaultAdaptiveOrchestrator) parseActionPerformanceData(raw interface{}) (*ActionPerformanceData, error) {
	dataMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{}, got %T", raw)
	}

	data := &ActionPerformanceData{}

	// Parse action_type
	if actionType, exists := dataMap["action_type"]; exists {
		if str, ok := actionType.(string); ok {
			data.ActionType = str
		}
	}

	// Parse success_rate
	if successRate, exists := dataMap["success_rate"]; exists {
		if rate, ok := successRate.(float64); ok {
			data.SuccessRate = rate
		} else if rate, ok := successRate.(int); ok {
			data.SuccessRate = float64(rate)
		} else {
			return nil, fmt.Errorf("success_rate must be numeric, got %T", successRate)
		}
	} else {
		return nil, fmt.Errorf("success_rate is required")
	}

	// Parse execution_count
	if execCount, exists := dataMap["execution_count"]; exists {
		if count, ok := execCount.(int); ok {
			data.ExecutionCount = count
		} else if count, ok := execCount.(float64); ok {
			data.ExecutionCount = int(count)
		}
	}

	// Parse avg_duration_ms
	if avgDuration, exists := dataMap["avg_duration_ms"]; exists {
		if duration, ok := avgDuration.(float64); ok {
			data.AvgDuration = duration
		} else if duration, ok := avgDuration.(int); ok {
			data.AvgDuration = float64(duration)
		}
	}

	return data, nil
}

// parsePatternInsightData parses raw interface{} data into PatternInsightData struct
func (dao *DefaultAdaptiveOrchestrator) parsePatternInsightData(raw interface{}) (*PatternInsightData, error) {
	dataMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{}, got %T", raw)
	}

	data := &PatternInsightData{}

	// Parse pattern_type
	if patternType, exists := dataMap["pattern_type"]; exists {
		if str, ok := patternType.(string); ok {
			data.PatternType = str
		}
	}

	// Parse effectiveness
	if effectiveness, exists := dataMap["effectiveness"]; exists {
		if eff, ok := effectiveness.(float64); ok {
			data.Effectiveness = eff
		} else if eff, ok := effectiveness.(int); ok {
			data.Effectiveness = float64(eff)
		} else {
			return nil, fmt.Errorf("effectiveness must be numeric, got %T", effectiveness)
		}
	} else {
		return nil, fmt.Errorf("effectiveness is required")
	}

	// Parse confidence
	if confidence, exists := dataMap["confidence"]; exists {
		if conf, ok := confidence.(float64); ok {
			data.Confidence = conf
		} else if conf, ok := confidence.(int); ok {
			data.Confidence = float64(conf)
		}
	}

	// Parse usage_count
	if usageCount, exists := dataMap["usage_count"]; exists {
		if count, ok := usageCount.(int); ok {
			data.UsageCount = count
		} else if count, ok := usageCount.(float64); ok {
			data.UsageCount = int(count)
		}
	}

	return data, nil
}

// Helper methods for priority and risk calculation
func (dao *DefaultAdaptiveOrchestrator) successRateToPriority(successRate float64) engine.Priority {
	if successRate >= 0.95 {
		return engine.PriorityHigh
	} else if successRate >= 0.85 {
		return engine.PriorityMedium
	} else {
		return engine.PriorityLow
	}
}

func (dao *DefaultAdaptiveOrchestrator) successRateToRisk(successRate float64) engine.RiskLevel {
	if successRate >= 0.9 {
		return engine.RiskLevelLow
	} else if successRate >= 0.8 {
		return engine.RiskLevelMedium
	} else {
		return engine.RiskLevelHigh
	}
}

func (dao *DefaultAdaptiveOrchestrator) sortRecommendations(recommendations []*engine.WorkflowRecommendation) {
	sort.Slice(recommendations, func(i, j int) bool {
		// Sort by confidence * effectiveness (descending)
		scoreI := recommendations[i].Confidence * recommendations[i].Effectiveness
		scoreJ := recommendations[j].Confidence * recommendations[j].Effectiveness
		return scoreI > scoreJ
	})
}

func (dao *DefaultAdaptiveOrchestrator) effectivenessScoreToPriority(score float64) engine.Priority {
	if score >= 0.9 {
		return engine.PriorityHigh
	} else if score >= 0.7 {
		return engine.PriorityMedium
	} else {
		return engine.PriorityLow
	}
}

func (dao *DefaultAdaptiveOrchestrator) effectivenessScoreToRisk(score float64) engine.RiskLevel {
	if score >= 0.8 {
		return engine.RiskLevelLow
	} else if score >= 0.6 {
		return engine.RiskLevelMedium
	} else {
		return engine.RiskLevelHigh
	}
}

func (dao *DefaultAdaptiveOrchestrator) getPreviousStepResults(execution *engine.RuntimeWorkflowExecution, currentStepIndex int) []*engine.StepResult {
	var results []*engine.StepResult
	for i := 0; i < currentStepIndex; i++ {
		if execution.Steps[i].Result != nil {
			results = append(results, execution.Steps[i].Result)
		}
	}
	return results
}

func (dao *DefaultAdaptiveOrchestrator) handleStepFailure(ctx context.Context, execution *engine.RuntimeWorkflowExecution, step *engine.ExecutableWorkflowStep, stepIndex int, err error) bool {
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
	stepExecution.Status = engine.ExecutionStatusPending

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
func (dao *DefaultAdaptiveOrchestrator) getWorkflowExecutions(workflowID string) []*engine.RuntimeWorkflowExecution {
	dao.executionMu.RLock()
	defer dao.executionMu.RUnlock()

	var executions []*engine.RuntimeWorkflowExecution
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
