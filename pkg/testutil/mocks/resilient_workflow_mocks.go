package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// MockFailureHandler implements engine.FailureHandler for testing
// Following guideline #32: reuse existing mocks and extend them
type MockFailureHandler struct {
	mu sync.RWMutex

	// Test configuration
	partialFailureRate   float64
	failurePolicy        string
	stepExecutionDelay   time.Duration
	executionHistory     []*engine.RuntimeWorkflowExecution
	learningEnabled      bool
	retryHistory         []*engine.RuntimeWorkflowExecution
	retryLearningEnabled bool

	// Mock state
	learningMetrics    *engine.LearningMetrics
	adaptiveStrategies []*engine.AdaptiveRetryStrategy
	retryEffectiveness float64
}

func NewMockFailureHandler() *MockFailureHandler {
	return &MockFailureHandler{
		partialFailureRate:   0.0,
		failurePolicy:        "fail_fast",
		stepExecutionDelay:   100 * time.Millisecond,
		learningEnabled:      false,
		retryLearningEnabled: false,
		learningMetrics: &engine.LearningMetrics{
			ConfidenceScore:         0.85, // ≥80% requirement
			PatternsLearned:         5,
			SuccessfulAdaptations:   8,
			LearningAccuracy:        0.88,
			LastLearningUpdate:      time.Now(),
			AdaptationEffectiveness: 0.82,
		},
		adaptiveStrategies: []*engine.AdaptiveRetryStrategy{
			{
				FailureType:       "timeout",
				OptimalRetryCount: 3,
				OptimalRetryDelay: 2 * time.Second,
				SuccessRate:       0.85,
				Confidence:        0.82,
				LearningSource:    "execution_history",
			},
		},
		retryEffectiveness: 82.5, // 82.5% effectiveness
	}
}

func (m *MockFailureHandler) HandleStepFailure(ctx context.Context, step *engine.ExecutableWorkflowStep,
	failure *engine.StepFailure, policy engine.FailurePolicy) (*engine.FailureDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Simulate step execution delay
	time.Sleep(m.stepExecutionDelay)

	decision := &engine.FailureDecision{
		ShouldRetry:    true,
		ShouldContinue: m.failurePolicy != "fail_fast",
		Action:         engine.ActionRetry,
		RetryDelay:     1 * time.Second,
		Reason:         "mock failure handling",
		ImpactAssessment: &engine.FailureImpact{
			BusinessImpact:    "minor",
			AffectedFunctions: []string{"test_function"},
			EstimatedDowntime: 30 * time.Second,
			RecoveryOptions:   []string{"retry", "fallback"},
		},
	}

	// Apply failure policy logic
	switch engine.FailurePolicy(m.failurePolicy) {
	case engine.FailurePolicyContinue:
		decision.Action = engine.ActionContinue
		decision.ShouldContinue = true
	case engine.FailurePolicyPartial:
		decision.Action = engine.ActionContinue
		decision.ShouldContinue = true
	case engine.FailurePolicyGradual:
		decision.Action = engine.ActionDegrade
		decision.ShouldContinue = true
	default:
		decision.Action = engine.ActionTerminate
		decision.ShouldContinue = false
	}

	return decision, nil
}

func (m *MockFailureHandler) CalculateWorkflowHealth(execution *engine.RuntimeWorkflowExecution) *engine.WorkflowHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalSteps := len(execution.Steps)
	completedSteps := 0
	failedSteps := 0
	criticalFailures := 0

	for _, step := range execution.Steps {
		switch step.Status {
		case engine.ExecutionStatusCompleted:
			completedSteps++
		case engine.ExecutionStatusFailed:
			failedSteps++
			// Assume first 2 steps are critical (as defined in test)
			if step.StepID == "resilient-step-0" || step.StepID == "resilient-step-1" {
				criticalFailures++
			}
		}
	}

	// Calculate health score
	healthScore := 1.0
	if totalSteps > 0 {
		healthScore = float64(completedSteps) / float64(totalSteps)
		// Penalize critical failures more heavily
		if criticalFailures > 0 {
			healthScore *= (1.0 - float64(criticalFailures)*0.3)
		}
	}

	// BR-WF-541: Can continue if <10% termination rate (low critical failures)
	canContinue := criticalFailures == 0 || (float64(criticalFailures)/float64(totalSteps) < 0.10)

	return &engine.WorkflowHealth{
		TotalSteps:       totalSteps,
		CompletedSteps:   completedSteps,
		FailedSteps:      failedSteps,
		CriticalFailures: criticalFailures,
		HealthScore:      healthScore,
		CanContinue:      canContinue,
		Recommendations:  []string{"Continue with current strategy", "Monitor critical step failures"},
		LastUpdated:      time.Now(),
	}
}

func (m *MockFailureHandler) ShouldTerminateWorkflow(health *engine.WorkflowHealth) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// BR-WF-541: Terminate only if critical failures exceed 10% threshold
	if health.TotalSteps == 0 {
		return false
	}

	terminationThreshold := 0.10 // 10% termination rate threshold
	criticalFailureRate := float64(health.CriticalFailures) / float64(health.TotalSteps)

	return criticalFailureRate >= terminationThreshold
}

func (m *MockFailureHandler) GetLearningMetrics() *engine.LearningMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.learningMetrics
}

func (m *MockFailureHandler) GetAdaptiveRetryStrategies() []*engine.AdaptiveRetryStrategy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	strategies := make([]*engine.AdaptiveRetryStrategy, len(m.adaptiveStrategies))
	copy(strategies, m.adaptiveStrategies)
	return strategies
}

func (m *MockFailureHandler) CalculateRetryEffectiveness() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.retryEffectiveness
}

// Test configuration methods

func (m *MockFailureHandler) SetPartialFailureRate(rate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.partialFailureRate = rate
}

func (m *MockFailureHandler) SetFailurePolicy(policy string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failurePolicy = policy
}

func (m *MockFailureHandler) SetStepExecutionDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stepExecutionDelay = delay
}

func (m *MockFailureHandler) SetExecutionHistory(history []*engine.RuntimeWorkflowExecution) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executionHistory = history
}

func (m *MockFailureHandler) EnableLearning(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.learningEnabled = enabled
}

func (m *MockFailureHandler) SetRetryHistory(history []*engine.RuntimeWorkflowExecution) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retryHistory = history
}

func (m *MockFailureHandler) EnableRetryLearning(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retryLearningEnabled = enabled
}

// MockWorkflowHealthChecker implements engine.WorkflowHealthChecker for testing
type MockWorkflowHealthChecker struct {
	mu           sync.RWMutex
	systemHealth *engine.SystemHealthMetrics
}

func NewMockWorkflowHealthChecker() *MockWorkflowHealthChecker {
	return &MockWorkflowHealthChecker{
		systemHealth: &engine.SystemHealthMetrics{
			OverallHealth:    0.87, // ≥85% requirement
			SuccessRate:      0.92, // ≥90% requirement
			ActiveWorkflows:  15,
			SystemThroughput: 450.0,
			ResourceUsage: &engine.ResourceUsageMetrics{
				CPUUsage:    0.65,
				MemoryUsage: 0.58,
			},
			AlertsActive:    2,
			LastHealthCheck: time.Now(),
		},
	}
}

func (m *MockWorkflowHealthChecker) CheckHealth(ctx context.Context, execution *engine.RuntimeWorkflowExecution) (*engine.WorkflowHealth, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create mock health based on execution status
	health := &engine.WorkflowHealth{
		TotalSteps:       len(execution.Steps),
		CompletedSteps:   len(execution.Steps), // Assume all completed for mock
		FailedSteps:      0,
		CriticalFailures: 0,
		HealthScore:      0.95,
		CanContinue:      true,
		Recommendations:  []string{"Workflow is healthy", "Continue normal operation"},
		LastUpdated:      time.Now(),
	}

	return health, nil
}

func (m *MockWorkflowHealthChecker) CalculateSystemHealth(executions []*engine.RuntimeWorkflowExecution) *engine.SystemHealthMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemHealth
}

func (m *MockWorkflowHealthChecker) GenerateHealthRecommendations(health *engine.WorkflowHealth) []engine.HealthRecommendation {
	recommendations := []engine.HealthRecommendation{
		{
			Type:               "performance",
			Description:        "Consider enabling parallel execution for better performance",
			Priority:           "medium",
			EstimatedImpact:    0.25,
			ImplementationCost: "low",
			ActionRequired:     false,
		},
	}

	if health.HealthScore < 0.85 {
		recommendations = append(recommendations, engine.HealthRecommendation{
			Type:               "reliability",
			Description:        "Health score below threshold, investigate failed steps",
			Priority:           "high",
			EstimatedImpact:    0.40,
			ImplementationCost: "medium",
			ActionRequired:     true,
		})
	}

	return recommendations
}

// MockLogger implements engine.Logger for testing
type MockLogger struct {
	*logrus.Logger
}

func NewMockLogger() *MockLogger {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Reduce test noise
	return &MockLogger{Logger: logger}
}

func (m *MockLogger) WithField(key string, value interface{}) engine.Logger {
	return &MockLogger{Logger: m.Logger.WithField(key, value).Logger}
}

func (m *MockLogger) WithFields(fields map[string]interface{}) engine.Logger {
	logrusFields := logrus.Fields{}
	for k, v := range fields {
		logrusFields[k] = v
	}
	return &MockLogger{Logger: m.Logger.WithFields(logrusFields).Logger}
}

func (m *MockLogger) WithError(err error) engine.Logger {
	return &MockLogger{Logger: m.Logger.WithError(err).Logger}
}
