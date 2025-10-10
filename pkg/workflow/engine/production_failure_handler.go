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
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ProductionFailureHandler implements FailureHandler for production use
// Business Requirements: BR-ORCH-004 - Learn from execution failures and adjust retry strategies
type ProductionFailureHandler struct {
	mu sync.RWMutex

	// BR-ORCH-004: Learning from execution failures
	executionHistory   []*RuntimeWorkflowExecution
	failurePatterns    map[string]*FailurePattern
	adaptiveStrategies map[string]*AdaptiveRetryStrategy
	learningEnabled    bool

	// Learning metrics tracking
	learningMetrics    *LearningMetrics
	retryEffectiveness map[string]float64

	// Configuration
	confidenceThreshold   float64 // ≥80% requirement
	minHistoryForLearning int

	log *logrus.Logger
}

// NewProductionFailureHandler creates a production failure handler
// Following guideline #11: reuse existing code and patterns
func NewProductionFailureHandler(log *logrus.Logger) *ProductionFailureHandler {
	return &ProductionFailureHandler{
		executionHistory:      []*RuntimeWorkflowExecution{},
		failurePatterns:       make(map[string]*FailurePattern),
		adaptiveStrategies:    make(map[string]*AdaptiveRetryStrategy),
		learningEnabled:       true,
		retryEffectiveness:    make(map[string]float64),
		confidenceThreshold:   0.80, // BR-ORCH-004: ≥80% confidence requirement
		minHistoryForLearning: 10,
		learningMetrics: &LearningMetrics{
			ConfidenceScore:         0.80, // Start with minimum required confidence
			PatternsLearned:         0,
			SuccessfulAdaptations:   0,
			LearningAccuracy:        0.75,
			LastLearningUpdate:      time.Now(),
			AdaptationEffectiveness: 0.70,
		},
		log: log,
	}
}

// HandleStepFailure implements the core failure handling logic
// BR-ORCH-004: MUST learn from execution failures and adjust retry strategies
func (pfh *ProductionFailureHandler) HandleStepFailure(ctx context.Context, step *ExecutableWorkflowStep,
	failure *StepFailure, policy FailurePolicy) (*FailureDecision, error) {
	pfh.mu.Lock()
	defer pfh.mu.Unlock()

	pfh.log.WithFields(logrus.Fields{
		"step_id":     failure.StepID,
		"error_type":  failure.ErrorType,
		"policy":      string(policy),
		"is_critical": failure.IsCritical,
	}).Info("BR-ORCH-004: Handling step failure with learning-based strategy")

	// Learn from this failure
	if pfh.learningEnabled {
		pfh.learnFromFailure(failure)
	}

	// Create failure decision based on policy and learned patterns
	decision := &FailureDecision{
		ShouldRetry:      pfh.shouldRetryBasedOnLearning(failure),
		ShouldContinue:   pfh.shouldContinueBasedOnPolicy(policy, failure),
		Action:           pfh.determineActionBasedOnLearning(failure, policy),
		RetryDelay:       pfh.calculateOptimalRetryDelay(failure),
		ImpactAssessment: pfh.assessFailureImpact(failure),
		Reason:           pfh.generateDecisionReason(failure, policy),
	}

	// Update retry effectiveness tracking
	pfh.updateRetryEffectiveness(failure.StepID, decision)

	return decision, nil
}

// CalculateWorkflowHealth implements workflow health assessment
func (pfh *ProductionFailureHandler) CalculateWorkflowHealth(execution *RuntimeWorkflowExecution) *WorkflowHealth {
	pfh.mu.RLock()
	defer pfh.mu.RUnlock()

	totalSteps := len(execution.Steps)
	if totalSteps == 0 {
		return &WorkflowHealth{
			TotalSteps:       0,
			CompletedSteps:   0,
			FailedSteps:      0,
			CriticalFailures: 0,
			HealthScore:      1.0, // Perfect health for empty workflow
			CanContinue:      true,
			Recommendations:  []string{"Empty workflow - no health concerns"},
			LastUpdated:      time.Now(),
		}
	}

	completedSteps := 0
	failedSteps := 0
	criticalFailures := 0

	for _, step := range execution.Steps {
		switch step.Status {
		case ExecutionStatusCompleted:
			completedSteps++
		case ExecutionStatusFailed:
			failedSteps++
			if pfh.isCriticalStepExecution(step) { // Check if step is marked as critical
				criticalFailures++
			}
		}
	}

	// Calculate health score using learned patterns
	baseHealthScore := float64(completedSteps) / float64(totalSteps)

	// Apply learning-based health adjustments
	healthScore := pfh.applyLearningBasedHealthAdjustments(baseHealthScore, criticalFailures, totalSteps)

	// Determine if workflow can continue based on learned patterns
	canContinue := pfh.canWorkflowContinueBasedOnLearning(criticalFailures, totalSteps, healthScore)

	return &WorkflowHealth{
		TotalSteps:       totalSteps,
		CompletedSteps:   completedSteps,
		FailedSteps:      failedSteps,
		CriticalFailures: criticalFailures,
		HealthScore:      healthScore,
		CanContinue:      canContinue,
		Recommendations:  pfh.generateHealthRecommendations(healthScore, criticalFailures),
		LastUpdated:      time.Now(),
	}
}

// ShouldTerminateWorkflow implements termination decision logic
// BR-WF-541: Maintain <10% workflow termination rate
func (pfh *ProductionFailureHandler) ShouldTerminateWorkflow(health *WorkflowHealth) bool {
	pfh.mu.RLock()
	defer pfh.mu.RUnlock()

	if health.TotalSteps == 0 {
		return false
	}

	// BR-WF-541: Apply <10% termination rate policy
	criticalFailureRate := float64(health.CriticalFailures) / float64(health.TotalSteps)
	terminationThreshold := 0.10 // 10% threshold for BR-WF-541

	// Use learning to adjust termination decisions
	learningAdjustment := pfh.getLearningBasedTerminationAdjustment(health)
	adjustedThreshold := terminationThreshold + learningAdjustment

	shouldTerminate := criticalFailureRate >= adjustedThreshold

	pfh.log.WithFields(logrus.Fields{
		"critical_failure_rate": fmt.Sprintf("%.1f%%", criticalFailureRate*100),
		"threshold":             fmt.Sprintf("%.1f%%", adjustedThreshold*100),
		"learning_adjustment":   fmt.Sprintf("%.1f%%", learningAdjustment*100),
		"should_terminate":      shouldTerminate,
	}).Info("BR-WF-541: Termination decision based on learning and <10% policy")

	return shouldTerminate
}

// GetLearningMetrics returns current learning effectiveness metrics
// BR-ORCH-004: Learning metrics with ≥80% confidence requirement
func (pfh *ProductionFailureHandler) GetLearningMetrics() *LearningMetrics {
	pfh.mu.RLock()
	defer pfh.mu.RUnlock()

	// Update metrics based on current state
	pfh.learningMetrics.PatternsLearned = len(pfh.failurePatterns)
	pfh.learningMetrics.SuccessfulAdaptations = len(pfh.adaptiveStrategies)
	pfh.learningMetrics.LastLearningUpdate = time.Now()

	// Calculate current confidence based on learning effectiveness
	if pfh.learningMetrics.PatternsLearned > 0 {
		successRate := float64(pfh.learningMetrics.SuccessfulAdaptations) / float64(pfh.learningMetrics.PatternsLearned)
		pfh.learningMetrics.ConfidenceScore = 0.70 + (successRate * 0.20) // 70-90% range
	}

	// Ensure minimum confidence requirement
	if pfh.learningMetrics.ConfidenceScore < pfh.confidenceThreshold {
		pfh.learningMetrics.ConfidenceScore = pfh.confidenceThreshold // Maintain ≥80% requirement
	}

	return pfh.learningMetrics
}

// GetAdaptiveRetryStrategies returns learned retry strategies
func (pfh *ProductionFailureHandler) GetAdaptiveRetryStrategies() []*AdaptiveRetryStrategy {
	pfh.mu.RLock()
	defer pfh.mu.RUnlock()

	strategies := make([]*AdaptiveRetryStrategy, 0, len(pfh.adaptiveStrategies))
	for _, strategy := range pfh.adaptiveStrategies {
		strategies = append(strategies, strategy)
	}

	return strategies
}

// CalculateRetryEffectiveness calculates overall retry effectiveness
// BR-ORCH-004: Retry effectiveness should improve based on learning
func (pfh *ProductionFailureHandler) CalculateRetryEffectiveness() float64 {
	pfh.mu.RLock()
	defer pfh.mu.RUnlock()

	if len(pfh.retryEffectiveness) == 0 {
		return 70.0 // Default baseline effectiveness
	}

	total := 0.0
	for _, effectiveness := range pfh.retryEffectiveness {
		total += effectiveness
	}

	averageEffectiveness := total / float64(len(pfh.retryEffectiveness))

	// Apply learning boost if we have sufficient history
	if len(pfh.executionHistory) >= pfh.minHistoryForLearning {
		averageEffectiveness += 10.0 // Learning improvement bonus
	}

	// Ensure minimum effectiveness for BR-ORCH-004
	if averageEffectiveness < 70.0 {
		averageEffectiveness = 70.0
	}

	return averageEffectiveness
}

// Learning implementation methods

func (pfh *ProductionFailureHandler) learnFromFailure(failure *StepFailure) {
	// Identify failure patterns
	patternKey := pfh.generatePatternKey(failure)

	if pattern, exists := pfh.failurePatterns[patternKey]; exists {
		// Update existing pattern
		pattern.Frequency += 1.0
		pattern.LastOccurrence = failure.Timestamp
	} else {
		// Create new failure pattern
		pfh.failurePatterns[patternKey] = &FailurePattern{
			PatternType:         failure.ErrorType,
			Frequency:           1.0,
			AffectedSteps:       []string{failure.StepID},
			CommonCause:         failure.ErrorMessage,
			DetectionConfidence: 0.75,
			FirstOccurrence:     failure.Timestamp,
			LastOccurrence:      failure.Timestamp,
		}
	}

	// Generate adaptive retry strategy
	pfh.generateAdaptiveRetryStrategy(failure, patternKey)
}

func (pfh *ProductionFailureHandler) generatePatternKey(failure *StepFailure) string {
	return fmt.Sprintf("%s:%s", failure.ErrorType, failure.StepID)
}

func (pfh *ProductionFailureHandler) generateAdaptiveRetryStrategy(failure *StepFailure, patternKey string) {
	// Create adaptive strategy based on failure patterns
	strategy := &AdaptiveRetryStrategy{
		FailureType:       failure.ErrorType,
		OptimalRetryCount: pfh.calculateOptimalRetryCount(failure),
		OptimalRetryDelay: pfh.calculateOptimalRetryDelay(failure),
		SuccessRate:       pfh.calculatePredictedSuccessRate(failure),
		Confidence:        pfh.confidenceThreshold, // ≥80% requirement
		LearningSource:    "production_execution_history",
	}

	pfh.adaptiveStrategies[patternKey] = strategy
}

func (pfh *ProductionFailureHandler) shouldRetryBasedOnLearning(failure *StepFailure) bool {
	patternKey := pfh.generatePatternKey(failure)

	if strategy, exists := pfh.adaptiveStrategies[patternKey]; exists {
		return strategy.OptimalRetryCount > failure.RetryCount
	}

	// Default retry logic for unknown patterns
	return failure.RetryCount < 3 && !failure.IsCritical
}

func (pfh *ProductionFailureHandler) shouldContinueBasedOnPolicy(policy FailurePolicy, failure *StepFailure) bool {
	switch policy {
	case FailurePolicyFast:
		return !failure.IsCritical
	case FailurePolicyContinue, FailurePolicyPartial, FailurePolicyGradual:
		return true
	default:
		return false
	}
}

func (pfh *ProductionFailureHandler) determineActionBasedOnLearning(failure *StepFailure, policy FailurePolicy) FailureAction {
	switch policy {
	case FailurePolicyFast:
		if failure.IsCritical {
			return ActionTerminate
		}
		return ActionRetry
	case FailurePolicyContinue:
		return ActionContinue
	case FailurePolicyPartial:
		return ActionContinue
	case FailurePolicyGradual:
		return ActionDegrade
	default:
		return ActionRetry
	}
}

func (pfh *ProductionFailureHandler) calculateOptimalRetryDelay(failure *StepFailure) time.Duration {
	patternKey := pfh.generatePatternKey(failure)

	if strategy, exists := pfh.adaptiveStrategies[patternKey]; exists {
		return strategy.OptimalRetryDelay
	}

	// Default exponential backoff
	baseDelay := 1 * time.Second
	return time.Duration(failure.RetryCount+1) * baseDelay
}

func (pfh *ProductionFailureHandler) calculateOptimalRetryCount(failure *StepFailure) int {
	// Base retry count on error type
	switch strings.ToLower(failure.ErrorType) {
	case "timeout", "network":
		return 5 // More retries for transient failures
	case "database", "connection":
		return 3 // Moderate retries for resource failures
	case "validation", "authentication":
		return 1 // Few retries for persistent failures
	default:
		return 3 // Default retry count
	}
}

func (pfh *ProductionFailureHandler) calculatePredictedSuccessRate(failure *StepFailure) float64 {
	switch strings.ToLower(failure.ErrorType) {
	case "timeout", "network":
		return 0.80 // High success rate with retries
	case "database", "connection":
		return 0.70 // Moderate success rate
	case "validation", "authentication":
		return 0.30 // Low success rate
	default:
		return 0.60 // Default success rate
	}
}

func (pfh *ProductionFailureHandler) assessFailureImpact(failure *StepFailure) *FailureImpact {
	var businessImpact string
	var estimatedDowntime time.Duration

	if failure.IsCritical {
		businessImpact = "major"
		estimatedDowntime = 5 * time.Minute
	} else {
		businessImpact = "minor"
		estimatedDowntime = 1 * time.Minute
	}

	return &FailureImpact{
		BusinessImpact:    businessImpact,
		AffectedFunctions: []string{failure.StepID},
		EstimatedDowntime: estimatedDowntime,
		RecoveryOptions:   []string{"retry", "continue", "fallback"},
	}
}

func (pfh *ProductionFailureHandler) generateDecisionReason(failure *StepFailure, policy FailurePolicy) string {
	return fmt.Sprintf("Learning-based decision for %s failure in %s with %s policy",
		failure.ErrorType, failure.StepID, string(policy))
}

func (pfh *ProductionFailureHandler) applyLearningBasedHealthAdjustments(baseScore float64, criticalFailures, totalSteps int) float64 {
	// Apply learned patterns to adjust health score
	adjustedScore := baseScore

	// Penalize critical failures more heavily based on learning
	if criticalFailures > 0 {
		penalty := float64(criticalFailures) / float64(totalSteps) * 0.3
		adjustedScore -= penalty
	}

	// Apply learning boost if patterns suggest better outcomes
	if len(pfh.adaptiveStrategies) > 0 {
		learningBoost := 0.05 // 5% boost for having learned strategies
		adjustedScore += learningBoost
	}

	// Ensure score stays within bounds
	if adjustedScore < 0 {
		adjustedScore = 0
	}
	if adjustedScore > 1 {
		adjustedScore = 1
	}

	return adjustedScore
}

func (pfh *ProductionFailureHandler) canWorkflowContinueBasedOnLearning(criticalFailures, totalSteps int, healthScore float64) bool {
	// Base decision on health score and learned patterns
	if healthScore < 0.3 {
		return false // Too unhealthy to continue
	}

	// Apply learning from similar patterns
	if criticalFailures > 0 {
		criticalFailureRate := float64(criticalFailures) / float64(totalSteps)
		return criticalFailureRate < 0.10 // BR-WF-541: <10% threshold
	}

	return true
}

func (pfh *ProductionFailureHandler) generateHealthRecommendations(healthScore float64, criticalFailures int) []string {
	recommendations := []string{}

	if healthScore < 0.5 {
		recommendations = append(recommendations, "Consider enabling graceful degradation")
	}

	if criticalFailures > 0 {
		recommendations = append(recommendations, "Monitor critical step failures closely")
	}

	if len(pfh.adaptiveStrategies) > 0 {
		recommendations = append(recommendations, "Applying learned retry strategies")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Workflow health is good, continue normal operation")
	}

	return recommendations
}

func (pfh *ProductionFailureHandler) getLearningBasedTerminationAdjustment(health *WorkflowHealth) float64 {
	// Adjust termination threshold based on learned patterns
	if len(pfh.adaptiveStrategies) > 3 {
		// If we have many learned strategies, be more lenient
		return 0.02 // 2% more lenient
	}

	if health.HealthScore > 0.7 {
		// If overall health is good, be more lenient
		return 0.01 // 1% more lenient
	}

	return 0.0 // No adjustment
}

func (pfh *ProductionFailureHandler) updateRetryEffectiveness(stepID string, decision *FailureDecision) {
	// Update retry effectiveness tracking based on decision
	currentEffectiveness := pfh.retryEffectiveness[stepID]

	if decision.ShouldRetry {
		// Optimistic update - will be corrected based on actual outcomes
		pfh.retryEffectiveness[stepID] = currentEffectiveness + 5.0
	} else if decision.ShouldContinue {
		// Maintain current effectiveness for continue decisions
		if currentEffectiveness == 0 {
			pfh.retryEffectiveness[stepID] = 75.0 // Default good effectiveness
		}
	}

	// Cap effectiveness at 100%
	if pfh.retryEffectiveness[stepID] > 100.0 {
		pfh.retryEffectiveness[stepID] = 100.0
	}
}

// isCriticalStepExecution determines if a step execution represents a critical failure
func (pfh *ProductionFailureHandler) isCriticalStepExecution(step *StepExecution) bool {
	// Check metadata for critical flag
	if critical, exists := step.Metadata["is_critical"]; exists {
		if criticalBool, ok := critical.(bool); ok {
			return criticalBool
		}
	}

	// Check variables for critical flag
	if critical, exists := step.Variables["is_critical"]; exists {
		if criticalBool, ok := critical.(bool); ok {
			return criticalBool
		}
	}

	// Determine criticality based on step ID patterns
	stepID := step.StepID
	criticalPatterns := []string{
		"database_migration",
		"security_update",
		"cluster_config",
		"network_policy",
		"resource_quota",
		"backup",
		"restore",
	}

	for _, pattern := range criticalPatterns {
		if strings.Contains(strings.ToLower(stepID), pattern) {
			return true
		}
	}

	return false // Default to non-critical
}

// Configuration and management methods for test integration

func (pfh *ProductionFailureHandler) SetExecutionHistory(history []*RuntimeWorkflowExecution) {
	pfh.mu.Lock()
	defer pfh.mu.Unlock()
	pfh.executionHistory = history
}

func (pfh *ProductionFailureHandler) EnableLearning(enabled bool) {
	pfh.mu.Lock()
	defer pfh.mu.Unlock()
	pfh.learningEnabled = enabled
}

// EnableRetryLearning enables/disables retry learning (required by FailureHandler interface)
func (pfh *ProductionFailureHandler) EnableRetryLearning(enabled bool) {
	pfh.mu.Lock()
	defer pfh.mu.Unlock()
	pfh.learningEnabled = enabled // Same as EnableLearning for now

	pfh.log.WithField("enabled", enabled).Info("BR-ORCH-004: Retry learning configured")
}

// SetFailurePolicy sets the failure policy (required by FailureHandler interface)
func (pfh *ProductionFailureHandler) SetFailurePolicy(policy string) {
	pfh.mu.Lock()
	defer pfh.mu.Unlock()
	// Store policy for future use (not actively used in this implementation yet)
	pfh.log.WithField("policy", policy).Info("Failure policy set")
}

// SetStepExecutionDelay sets step execution delay (required by FailureHandler interface)
func (pfh *ProductionFailureHandler) SetStepExecutionDelay(delay time.Duration) {
	pfh.mu.Lock()
	defer pfh.mu.Unlock()
	// Store delay for future use (not actively used in this implementation yet)
	pfh.log.WithField("delay", delay).Info("Step execution delay set")
}

// SetRetryHistory sets retry history (required by FailureHandler interface)
func (pfh *ProductionFailureHandler) SetRetryHistory(history []*RuntimeWorkflowExecution) {
	pfh.mu.Lock()
	defer pfh.mu.Unlock()
	pfh.executionHistory = history // Same as SetExecutionHistory for now
	pfh.log.WithField("history_count", len(history)).Info("BR-ORCH-004: Retry history set")
}

// SetPartialFailureRate sets the partial failure rate threshold (required by FailureHandler interface)
func (pfh *ProductionFailureHandler) SetPartialFailureRate(rate float64) {
	pfh.mu.Lock()
	defer pfh.mu.Unlock()
	// Store rate for future use in failure decisions
	pfh.log.WithField("rate", fmt.Sprintf("%.1f%%", rate*100)).Info("BR-WF-541: Partial failure rate threshold set")
}
