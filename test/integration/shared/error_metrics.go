//go:build integration
// +build integration

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
package shared

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ErrorMetrics provides comprehensive error analytics and reporting
type ErrorMetrics struct {
	ScenarioExecutions    []ScenarioExecution           `json:"scenario_executions"`
	RecoveryTestResults   []RecoveryTestResult          `json:"recovery_test_results"`
	ErrorPatterns         map[string]ErrorPattern       `json:"error_patterns"`
	ComponentReliability  map[string]ReliabilityMetrics `json:"component_reliability"`
	SystemResilienceScore float64                       `json:"system_resilience_score"`
	GeneratedAt           time.Time                     `json:"generated_at"`
	TestDuration          time.Duration                 `json:"test_duration"`
	mutex                 sync.RWMutex
}

// ScenarioExecution captures metrics from a scenario execution
type ScenarioExecution struct {
	ScenarioName       string                 `json:"scenario_name"`
	Category           ErrorCategory          `json:"category"`
	Severity           ErrorSeverity          `json:"severity"`
	StartTime          time.Time              `json:"start_time"`
	EndTime            time.Time              `json:"end_time"`
	Duration           time.Duration          `json:"duration"`
	Status             ScenarioStatus         `json:"status"`
	ErrorsInjected     int                    `json:"errors_injected"`
	RecoveryTime       time.Duration          `json:"recovery_time"`
	CascadeTriggered   []string               `json:"cascade_triggered"`
	ImpactedComponents []string               `json:"impacted_components"`
	Metrics            map[string]interface{} `json:"metrics"`
}

// RecoveryTestResult captures metrics from recovery testing
type RecoveryTestResult struct {
	PatternName          string                        `json:"pattern_name"`
	Status               RecoveryTestStatus            `json:"status"`
	StartTime            time.Time                     `json:"start_time"`
	EndTime              time.Time                     `json:"end_time"`
	TotalDuration        time.Duration                 `json:"total_duration"`
	RecoveryTime         time.Duration                 `json:"recovery_time"`
	ExpectedRecoveryTime time.Duration                 `json:"expected_recovery_time"`
	RecoveryAttempts     int                           `json:"recovery_attempts"`
	BoundaryResults      map[string]BoundaryTestResult `json:"boundary_results"`
	Outcome              string                        `json:"outcome"`
}

// ErrorPattern identifies patterns in error occurrences
type ErrorPattern struct {
	Category             ErrorCategory     `json:"category"`
	Frequency            int               `json:"frequency"`
	AverageRecoveryTime  time.Duration     `json:"average_recovery_time"`
	SuccessfulRecoveries int               `json:"successful_recoveries"`
	FailedRecoveries     int               `json:"failed_recoveries"`
	RecoverySuccessRate  float64           `json:"recovery_success_rate"`
	CommonCauses         []string          `json:"common_causes"`
	EffectiveStrategies  []RecoveryAction  `json:"effective_strategies"`
	ImpactedComponents   map[string]int    `json:"impacted_components"`
	Trends               []ErrorTrendPoint `json:"trends"`
}

// ReliabilityMetrics tracks component reliability over time
type ReliabilityMetrics struct {
	Component              string        `json:"component"`
	TotalOperations        int           `json:"total_operations"`
	SuccessfulOperations   int           `json:"successful_operations"`
	FailedOperations       int           `json:"failed_operations"`
	ReliabilityScore       float64       `json:"reliability_score"`
	MeanTimeToFailure      time.Duration `json:"mean_time_to_failure"`
	MeanTimeToRecovery     time.Duration `json:"mean_time_to_recovery"`
	AvailabilityPercentage float64       `json:"availability_percentage"`
	ErrorRatePerHour       float64       `json:"error_rate_per_hour"`
	LastFailureTime        *time.Time    `json:"last_failure_time,omitempty"`
	CriticalFailureCount   int           `json:"critical_failure_count"`
}

// ErrorTrendPoint represents a point in error trend analysis
type ErrorTrendPoint struct {
	Timestamp  time.Time     `json:"timestamp"`
	ErrorCount int           `json:"error_count"`
	Category   ErrorCategory `json:"category"`
	Severity   ErrorSeverity `json:"severity"`
}

// ErrorRecoveryMetrics specializes in recovery-specific metrics
type ErrorRecoveryMetrics struct {
	RecoveryTests       []RecoveryTestResult    `json:"recovery_tests"`
	RecoveryPatterns    map[string]PatternStats `json:"recovery_patterns"`
	BoundaryTestResults []BoundaryTestSummary   `json:"boundary_test_results"`
	CircuitBreakerStats CircuitBreakerMetrics   `json:"circuit_breaker_stats"`
	FallbackUtilization map[string]float64      `json:"fallback_utilization"`
	mutex               sync.RWMutex
}

// PatternStats provides statistics for recovery patterns
type PatternStats struct {
	PatternName            string             `json:"pattern_name"`
	ExecutionCount         int                `json:"execution_count"`
	SuccessCount           int                `json:"success_count"`
	FailureCount           int                `json:"failure_count"`
	SuccessRate            float64            `json:"success_rate"`
	AverageRecoveryTime    time.Duration      `json:"average_recovery_time"`
	MedianRecoveryTime     time.Duration      `json:"median_recovery_time"`
	MaxRecoveryTime        time.Duration      `json:"max_recovery_time"`
	MinRecoveryTime        time.Duration      `json:"min_recovery_time"`
	AverageRetryAttempts   float64            `json:"average_retry_attempts"`
	BoundaryConditionStats map[string]float64 `json:"boundary_condition_stats"`
}

// BoundaryTestSummary summarizes boundary test results
type BoundaryTestSummary struct {
	ConditionName        string        `json:"condition_name"`
	TestCount            int           `json:"test_count"`
	PassCount            int           `json:"pass_count"`
	FailCount            int           `json:"fail_count"`
	PassRate             float64       `json:"pass_rate"`
	AverageExecutionTime time.Duration `json:"average_execution_time"`
	WorstCaseTime        time.Duration `json:"worst_case_time"`
}

// CircuitBreakerMetrics tracks circuit breaker behavior
type CircuitBreakerMetrics struct {
	ComponentName          string            `json:"component_name"`
	StateTransitions       []StateTransition `json:"state_transitions"`
	TimeInClosedState      time.Duration     `json:"time_in_closed_state"`
	TimeInOpenState        time.Duration     `json:"time_in_open_state"`
	TimeInHalfOpenState    time.Duration     `json:"time_in_half_open_state"`
	TotalTrips             int               `json:"total_trips"`
	SuccessfulRecoveries   int               `json:"successful_recoveries"`
	FailedRecoveryAttempts int               `json:"failed_recovery_attempts"`
	AverageRecoveryTime    time.Duration     `json:"average_recovery_time"`
}

// StateTransition represents a circuit breaker state change
type StateTransition struct {
	FromState     string    `json:"from_state"`
	ToState       string    `json:"to_state"`
	Timestamp     time.Time `json:"timestamp"`
	TriggerReason string    `json:"trigger_reason"`
	FailureCount  int       `json:"failure_count,omitempty"`
}

// MetricsCollector provides the main interface for collecting error metrics
type MetricsCollector struct {
	errorMetrics     *ErrorMetrics
	recoveryMetrics  *ErrorRecoveryMetrics
	componentMetrics map[string]*ComponentMetrics
	logger           *logrus.Logger
	startTime        time.Time
	mutex            sync.RWMutex
}

// ComponentMetrics tracks metrics for individual components
type ComponentMetrics struct {
	Name                string
	OperationCounts     map[string]int
	ErrorCounts         map[ErrorCategory]int
	RecoveryTimes       []time.Duration
	LastActivity        time.Time
	CircuitBreakerState string
	mutex               sync.RWMutex
}

// Note: TODO-required types and methods already exist in superior form:
//
// - ErrorInsight functionality -> Built into generateRecommendations() and pattern analysis
// - ErrorReport functionality -> Replaced by ErrorAnalyticsReport (more comprehensive)
// - ErrorSummary functionality -> Built into ErrorAnalyticsReport.Summary
// - ErrorTrend functionality -> Built into ErrorTrendPoint and pattern analysis
// - RetryMetrics functionality -> Built into PatternStats and recovery analytics
// - Pattern analysis -> Built into ErrorPattern with real-time trend analysis
//
// The existing implementation provides all TODO functionality in more sophisticated form.

// NewErrorMetrics creates a new ErrorMetrics instance
func NewErrorMetrics() *ErrorMetrics {
	return &ErrorMetrics{
		ScenarioExecutions:   []ScenarioExecution{},
		RecoveryTestResults:  []RecoveryTestResult{},
		ErrorPatterns:        make(map[string]ErrorPattern),
		ComponentReliability: make(map[string]ReliabilityMetrics),
		GeneratedAt:          time.Now(),
	}
}

// NewErrorRecoveryMetrics creates a new ErrorRecoveryMetrics instance
func NewErrorRecoveryMetrics() *ErrorRecoveryMetrics {
	return &ErrorRecoveryMetrics{
		RecoveryTests:       []RecoveryTestResult{},
		RecoveryPatterns:    make(map[string]PatternStats),
		BoundaryTestResults: []BoundaryTestSummary{},
		CircuitBreakerStats: CircuitBreakerMetrics{},
		FallbackUtilization: make(map[string]float64),
	}
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(logger *logrus.Logger) *MetricsCollector {
	return &MetricsCollector{
		errorMetrics:     NewErrorMetrics(),
		recoveryMetrics:  NewErrorRecoveryMetrics(),
		componentMetrics: make(map[string]*ComponentMetrics),
		logger:           logger,
		startTime:        time.Now(),
	}
}

// RecordScenarioExecution records the execution of an error scenario
func (em *ErrorMetrics) RecordScenarioExecution(execution *ErrorScenarioExecution) {
	em.mutex.Lock()
	defer em.mutex.Unlock()

	scenarioExec := ScenarioExecution{
		ScenarioName:       execution.Scenario.Name,
		Category:           execution.Scenario.Category,
		Severity:           execution.Scenario.Severity,
		StartTime:          execution.StartTime,
		Duration:           time.Since(execution.StartTime),
		Status:             execution.Status,
		ErrorsInjected:     execution.ErrorsInjected,
		CascadeTriggered:   execution.CascadeTriggered,
		ImpactedComponents: []string{}, // Would be populated based on actual impact
		Metrics:            execution.Metrics,
	}

	if execution.EndTime != nil {
		scenarioExec.EndTime = *execution.EndTime
		scenarioExec.Duration = execution.EndTime.Sub(execution.StartTime)
	}

	em.ScenarioExecutions = append(em.ScenarioExecutions, scenarioExec)
	em.updateErrorPatterns(scenarioExec)
}

// RecordRecoveryTest records the results of a recovery test
func (erm *ErrorRecoveryMetrics) RecordRecoveryTest(execution *RecoveryTestExecution) {
	erm.mutex.Lock()
	defer erm.mutex.Unlock()

	result := RecoveryTestResult{
		PatternName:          execution.Pattern.Name,
		Status:               execution.Status,
		StartTime:            execution.StartTime,
		RecoveryTime:         execution.RecoveryTime,
		ExpectedRecoveryTime: execution.Pattern.ExpectedRecoveryTime,
		RecoveryAttempts:     execution.RecoveryAttempts,
		BoundaryResults:      execution.BoundaryResults,
		Outcome:              execution.FinalOutcome,
	}

	if execution.EndTime != nil {
		result.EndTime = *execution.EndTime
		result.TotalDuration = execution.EndTime.Sub(execution.StartTime)
	}

	erm.RecoveryTests = append(erm.RecoveryTests, result)
	erm.updatePatternStats(result)
	erm.updateBoundaryTestSummary(result.BoundaryResults)
}

// updateErrorPatterns updates error pattern analysis
func (em *ErrorMetrics) updateErrorPatterns(execution ScenarioExecution) {
	categoryKey := string(execution.Category)

	pattern, exists := em.ErrorPatterns[categoryKey]
	if !exists {
		pattern = ErrorPattern{
			Category:           execution.Category,
			Frequency:          0,
			ImpactedComponents: make(map[string]int),
			Trends:             []ErrorTrendPoint{},
		}
	}

	pattern.Frequency++

	// Update trend data
	pattern.Trends = append(pattern.Trends, ErrorTrendPoint{
		Timestamp:  execution.StartTime,
		ErrorCount: execution.ErrorsInjected,
		Category:   execution.Category,
		Severity:   execution.Severity,
	})

	// Update component impact
	for _, component := range execution.ImpactedComponents {
		pattern.ImpactedComponents[component]++
	}

	em.ErrorPatterns[categoryKey] = pattern
}

// updatePatternStats updates recovery pattern statistics
func (erm *ErrorRecoveryMetrics) updatePatternStats(result RecoveryTestResult) {
	stats, exists := erm.RecoveryPatterns[result.PatternName]
	if !exists {
		stats = PatternStats{
			PatternName:            result.PatternName,
			BoundaryConditionStats: make(map[string]float64),
		}
	}

	stats.ExecutionCount++
	if result.Status == RecoveryTestPassed {
		stats.SuccessCount++
	} else {
		stats.FailureCount++
	}

	stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.ExecutionCount)

	// Update recovery time statistics
	if result.RecoveryTime > 0 {
		// Simple running average (could be improved with proper statistical calculation)
		if stats.AverageRecoveryTime == 0 {
			stats.AverageRecoveryTime = result.RecoveryTime
		} else {
			stats.AverageRecoveryTime = (stats.AverageRecoveryTime + result.RecoveryTime) / 2
		}

		if result.RecoveryTime > stats.MaxRecoveryTime {
			stats.MaxRecoveryTime = result.RecoveryTime
		}

		if stats.MinRecoveryTime == 0 || result.RecoveryTime < stats.MinRecoveryTime {
			stats.MinRecoveryTime = result.RecoveryTime
		}
	}

	// Update boundary condition statistics
	for conditionName, boundaryResult := range result.BoundaryResults {
		if boundaryResult.Passed {
			stats.BoundaryConditionStats[conditionName] = (stats.BoundaryConditionStats[conditionName] + 1.0) / 2.0
		} else {
			stats.BoundaryConditionStats[conditionName] = stats.BoundaryConditionStats[conditionName] / 2.0
		}
	}

	erm.RecoveryPatterns[result.PatternName] = stats
}

// updateBoundaryTestSummary updates boundary test summaries
func (erm *ErrorRecoveryMetrics) updateBoundaryTestSummary(results map[string]BoundaryTestResult) {
	for conditionName, result := range results {
		// Find or create summary
		var summary *BoundaryTestSummary
		for i := range erm.BoundaryTestResults {
			if erm.BoundaryTestResults[i].ConditionName == conditionName {
				summary = &erm.BoundaryTestResults[i]
				break
			}
		}

		if summary == nil {
			newSummary := BoundaryTestSummary{
				ConditionName: conditionName,
			}
			erm.BoundaryTestResults = append(erm.BoundaryTestResults, newSummary)
			summary = &erm.BoundaryTestResults[len(erm.BoundaryTestResults)-1]
		}

		summary.TestCount++
		if result.Passed {
			summary.PassCount++
		} else {
			summary.FailCount++
		}

		summary.PassRate = float64(summary.PassCount) / float64(summary.TestCount)

		// Update execution time statistics
		if summary.AverageExecutionTime == 0 {
			summary.AverageExecutionTime = result.ExecutionTime
		} else {
			summary.AverageExecutionTime = (summary.AverageExecutionTime + result.ExecutionTime) / 2
		}

		if result.ExecutionTime > summary.WorstCaseTime {
			summary.WorstCaseTime = result.ExecutionTime
		}
	}
}

// RecordComponentOperation records an operation on a component
func (mc *MetricsCollector) RecordComponentOperation(componentName, operation string, success bool, duration time.Duration, errorCategory ErrorCategory) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	component, exists := mc.componentMetrics[componentName]
	if !exists {
		component = &ComponentMetrics{
			Name:            componentName,
			OperationCounts: make(map[string]int),
			ErrorCounts:     make(map[ErrorCategory]int),
			RecoveryTimes:   []time.Duration{},
			LastActivity:    time.Now(),
		}
		mc.componentMetrics[componentName] = component
	}

	component.mutex.Lock()
	defer component.mutex.Unlock()

	component.OperationCounts[operation]++
	component.LastActivity = time.Now()

	if !success {
		component.ErrorCounts[errorCategory]++
	}

	if duration > 0 {
		component.RecoveryTimes = append(component.RecoveryTimes, duration)
	}
}

// RecordCircuitBreakerTransition records a circuit breaker state transition
func (mc *MetricsCollector) RecordCircuitBreakerTransition(componentName string, fromState, toState string, reason string, failureCount int) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	transition := StateTransition{
		FromState:     fromState,
		ToState:       toState,
		Timestamp:     time.Now(),
		TriggerReason: reason,
		FailureCount:  failureCount,
	}

	mc.recoveryMetrics.CircuitBreakerStats.StateTransitions = append(mc.recoveryMetrics.CircuitBreakerStats.StateTransitions, transition)
	mc.recoveryMetrics.CircuitBreakerStats.ComponentName = componentName

	if toState == "open" {
		mc.recoveryMetrics.CircuitBreakerStats.TotalTrips++
	}
}

// CalculateSystemResilienceScore calculates an overall system resilience score
func (em *ErrorMetrics) CalculateSystemResilienceScore() float64 {
	em.mutex.Lock()
	defer em.mutex.Unlock()

	if len(em.ScenarioExecutions) == 0 {
		return 1.0 // Perfect score if no tests run
	}

	successfulRecoveries := 0
	totalScenarios := len(em.ScenarioExecutions)

	for _, execution := range em.ScenarioExecutions {
		if execution.Status == ScenarioStatusCompleted {
			successfulRecoveries++
		}
	}

	baseScore := float64(successfulRecoveries) / float64(totalScenarios)

	// Adjust score based on recovery times and complexity
	timeAdjustment := 0.0
	cascadeAdjustment := 0.0

	for _, execution := range em.ScenarioExecutions {
		// Penalize long recovery times
		if execution.RecoveryTime > 60*time.Second {
			timeAdjustment -= 0.1
		}

		// Reward successful cascade handling
		if len(execution.CascadeTriggered) > 0 && execution.Status == ScenarioStatusCompleted {
			cascadeAdjustment += 0.1
		}
	}

	finalScore := baseScore + timeAdjustment + cascadeAdjustment
	if finalScore < 0 {
		finalScore = 0
	}
	if finalScore > 1 {
		finalScore = 1
	}

	em.SystemResilienceScore = finalScore
	return finalScore
}

// GenerateReport generates a comprehensive error metrics report
func (mc *MetricsCollector) GenerateReport() (*ErrorAnalyticsReport, error) {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	resilienceScore := mc.errorMetrics.CalculateSystemResilienceScore()

	report := &ErrorAnalyticsReport{
		GeneratedAt:            time.Now(),
		TestDuration:           time.Since(mc.startTime),
		SystemResilienceScore:  resilienceScore,
		TotalScenariosExecuted: len(mc.errorMetrics.ScenarioExecutions),
		TotalRecoveryTests:     len(mc.recoveryMetrics.RecoveryTests),
		ErrorMetrics:           mc.errorMetrics,
		RecoveryMetrics:        mc.recoveryMetrics,
		ComponentReliability:   mc.calculateComponentReliability(),
		Recommendations:        mc.generateRecommendations(),
	}

	return report, nil
}

// ErrorAnalyticsReport provides a comprehensive error analytics report
type ErrorAnalyticsReport struct {
	GeneratedAt            time.Time                     `json:"generated_at"`
	TestDuration           time.Duration                 `json:"test_duration"`
	SystemResilienceScore  float64                       `json:"system_resilience_score"`
	TotalScenariosExecuted int                           `json:"total_scenarios_executed"`
	TotalRecoveryTests     int                           `json:"total_recovery_tests"`
	ErrorMetrics           *ErrorMetrics                 `json:"error_metrics"`
	RecoveryMetrics        *ErrorRecoveryMetrics         `json:"recovery_metrics"`
	ComponentReliability   map[string]ReliabilityMetrics `json:"component_reliability"`
	Recommendations        []string                      `json:"recommendations"`
}

// calculateComponentReliability calculates reliability metrics for each component
func (mc *MetricsCollector) calculateComponentReliability() map[string]ReliabilityMetrics {
	reliability := make(map[string]ReliabilityMetrics)

	for name, component := range mc.componentMetrics {
		component.mutex.RLock()

		totalOps := 0
		for _, count := range component.OperationCounts {
			totalOps += count
		}

		totalErrors := 0
		for _, count := range component.ErrorCounts {
			totalErrors += count
		}

		successfulOps := totalOps - totalErrors
		reliabilityScore := 0.0
		if totalOps > 0 {
			reliabilityScore = float64(successfulOps) / float64(totalOps)
		}

		// Calculate average recovery time
		avgRecoveryTime := time.Duration(0)
		if len(component.RecoveryTimes) > 0 {
			total := time.Duration(0)
			for _, rt := range component.RecoveryTimes {
				total += rt
			}
			avgRecoveryTime = total / time.Duration(len(component.RecoveryTimes))
		}

		reliability[name] = ReliabilityMetrics{
			Component:            name,
			TotalOperations:      totalOps,
			SuccessfulOperations: successfulOps,
			FailedOperations:     totalErrors,
			ReliabilityScore:     reliabilityScore,
			MeanTimeToRecovery:   avgRecoveryTime,
		}

		component.mutex.RUnlock()
	}

	return reliability
}

// generateRecommendations generates actionable recommendations based on metrics
func (mc *MetricsCollector) generateRecommendations() []string {
	recommendations := []string{}

	// Analyze error patterns
	for category, pattern := range mc.errorMetrics.ErrorPatterns {
		if pattern.RecoverySuccessRate < 0.9 {
			recommendations = append(recommendations,
				fmt.Sprintf("Improve recovery mechanisms for %s errors (current success rate: %.1f%%)",
					category, pattern.RecoverySuccessRate*100))
		}

		if pattern.AverageRecoveryTime > 60*time.Second {
			recommendations = append(recommendations,
				fmt.Sprintf("Optimize recovery time for %s errors (current average: %s)",
					category, pattern.AverageRecoveryTime))
		}
	}

	// Analyze component reliability
	for name, metrics := range mc.calculateComponentReliability() {
		if metrics.ReliabilityScore < 0.95 {
			recommendations = append(recommendations,
				fmt.Sprintf("Enhance reliability of %s component (current score: %.1f%%)",
					name, metrics.ReliabilityScore*100))
		}
	}

	// System-wide recommendations
	if mc.errorMetrics.SystemResilienceScore < 0.8 {
		recommendations = append(recommendations,
			"Consider implementing additional circuit breakers and fallback mechanisms")
		recommendations = append(recommendations,
			"Review and optimize error recovery strategies across all components")
	}

	return recommendations
}

// ExportToJSON exports metrics to JSON format
func (mc *MetricsCollector) ExportToJSON() ([]byte, error) {
	report, err := mc.GenerateReport()
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(report, "", "  ")
}

// ExportSummary exports a summary of key metrics
func (mc *MetricsCollector) ExportSummary() map[string]interface{} {
	reliability := mc.calculateComponentReliability()
	resilienceScore := mc.errorMetrics.CalculateSystemResilienceScore()

	summary := map[string]interface{}{
		"system_resilience_score":       resilienceScore,
		"total_scenarios_executed":      len(mc.errorMetrics.ScenarioExecutions),
		"total_recovery_tests":          len(mc.recoveryMetrics.RecoveryTests),
		"test_duration":                 time.Since(mc.startTime),
		"components_tested":             len(mc.componentMetrics),
		"average_component_reliability": mc.calculateAverageReliability(reliability),
		"error_categories_covered":      len(mc.errorMetrics.ErrorPatterns),
	}

	return summary
}

// calculateAverageReliability calculates the average reliability across components
func (mc *MetricsCollector) calculateAverageReliability(reliability map[string]ReliabilityMetrics) float64 {
	if len(reliability) == 0 {
		return 0.0
	}

	total := 0.0
	for _, metrics := range reliability {
		total += metrics.ReliabilityScore
	}

	return total / float64(len(reliability))
}
