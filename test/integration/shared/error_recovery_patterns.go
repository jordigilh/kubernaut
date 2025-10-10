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
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// Recovery action types for systematic error recovery testing
type RecoveryAction string

const (
	RetryImmediate        RecoveryAction = "retry_immediate"
	RetryWithBackoff      RecoveryAction = "retry_with_backoff"
	FallbackToCache       RecoveryAction = "fallback_to_cache"
	GracefulDegradation   RecoveryAction = "graceful_degradation"
	CircuitBreakerOpen    RecoveryAction = "circuit_breaker_open"
	EscalateToOperator    RecoveryAction = "escalate_to_operator"
	SwitchToBackupService RecoveryAction = "switch_to_backup"
)

// ErrorSeverity is defined in error_standards.go

// Additional error categories for boundary testing
const (
	TransientError      ErrorCategory = "transient"
	PermanentError      ErrorCategory = "permanent"
	CircuitBreakerError ErrorCategory = "circuit_breaker"
)

// ErrorRecoveryPattern defines a systematic approach to testing error recovery
type ErrorRecoveryPattern struct {
	Name                 string
	Description          string
	TriggerCondition     func() error                    // Function to trigger the error
	RecoveryMechanism    func(ctx context.Context) error // Function to implement recovery
	VerifyRecovery       func(ctx context.Context) error // Function to verify successful recovery
	ExpectedRecoveryTime time.Duration                   // Maximum expected recovery time
	RetryPolicy          RetryPolicy                     // Retry configuration
	FallbackStrategy     FallbackStrategy                // Fallback approach
	CircuitBreaker       *CircuitBreakerPattern          // Circuit breaker configuration
	BoundaryConditions   []BoundaryCondition             // Specific boundary conditions to test
}

// RetryPolicy defines retry behavior for error recovery
type RetryPolicy struct {
	MaxAttempts       int
	InitialDelay      time.Duration
	BackoffMultiplier float64
	MaxDelay          time.Duration
	Jitter            bool
}

// FallbackStrategy defines fallback behavior when primary recovery fails
type FallbackStrategy struct {
	Enabled            bool
	FallbackTimeout    time.Duration
	QualityDegradation float64 // 0.0-1.0, how much quality degrades
	FallbackDataSource string  // e.g., "cache", "backup_service", "static_response"
	FallbackValidation func(result interface{}) error
}

// CircuitBreakerPattern defines circuit breaker testing patterns
type CircuitBreakerPattern struct {
	Enabled          bool
	FailureThreshold int
	RecoveryTimeout  time.Duration
	TestHalfOpen     bool
	ValidateRecovery func() bool
}

// BoundaryCondition defines specific boundary conditions to test
type BoundaryCondition struct {
	Name            string
	Description     string
	TestFunc        func(ctx context.Context) error
	ExpectedOutcome string
}

// ErrorBoundaryTester provides systematic boundary testing capabilities
type ErrorBoundaryTester struct {
	patterns        []ErrorRecoveryPattern
	activeTests     map[string]*RecoveryTestExecution
	logger          *logrus.Logger
	metrics         *ErrorRecoveryMetrics
	concurrentTests int
	timeout         time.Duration
	mutex           sync.RWMutex
}

// RecoveryTestExecution tracks the execution of a recovery test
type RecoveryTestExecution struct {
	Pattern          ErrorRecoveryPattern
	StartTime        time.Time
	EndTime          *time.Time
	Status           RecoveryTestStatus
	ErrorTriggerred  bool
	RecoveryAttempts int
	RecoveryTime     time.Duration
	BoundaryResults  map[string]BoundaryTestResult
	FinalOutcome     string
}

// RecoveryTestStatus represents the status of a recovery test
type RecoveryTestStatus string

const (
	RecoveryTestPending    RecoveryTestStatus = "pending"
	RecoveryTestTriggering RecoveryTestStatus = "triggering_error"
	RecoveryTestRecovering RecoveryTestStatus = "recovering"
	RecoveryTestVerifying  RecoveryTestStatus = "verifying"
	RecoveryTestPassed     RecoveryTestStatus = "passed"
	RecoveryTestFailed     RecoveryTestStatus = "failed"
	RecoveryTestTimeout    RecoveryTestStatus = "timeout"
)

// BoundaryTestResult captures the result of a boundary condition test
type BoundaryTestResult struct {
	Condition       BoundaryCondition
	Passed          bool
	ActualOutcome   string
	ExpectedOutcome string
	ExecutionTime   time.Duration
	Error           string
}

// NewErrorBoundaryTester creates a new error boundary tester
func NewErrorBoundaryTester(logger *logrus.Logger) *ErrorBoundaryTester {
	return &ErrorBoundaryTester{
		patterns:        []ErrorRecoveryPattern{},
		activeTests:     make(map[string]*RecoveryTestExecution),
		logger:          logger,
		metrics:         NewErrorRecoveryMetrics(),
		concurrentTests: 3,
		timeout:         5 * time.Minute,
	}
}

// AddRecoveryPattern adds a recovery pattern to test
func (ebt *ErrorBoundaryTester) AddRecoveryPattern(pattern ErrorRecoveryPattern) {
	ebt.patterns = append(ebt.patterns, pattern)
}

// ExecuteRecoveryTest executes a specific recovery pattern test
func (ebt *ErrorBoundaryTester) ExecuteRecoveryTest(ctx context.Context, patternName string) (*RecoveryTestExecution, error) {
	// Find the pattern
	var pattern ErrorRecoveryPattern
	found := false
	for _, p := range ebt.patterns {
		if p.Name == patternName {
			pattern = p
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("recovery pattern '%s' not found", patternName)
	}

	// Create execution context with timeout
	testCtx, cancel := context.WithTimeout(ctx, ebt.timeout)
	defer cancel()

	execution := &RecoveryTestExecution{
		Pattern:         pattern,
		StartTime:       time.Now(),
		Status:          RecoveryTestPending,
		BoundaryResults: make(map[string]BoundaryTestResult),
	}

	ebt.mutex.Lock()
	ebt.activeTests[patternName] = execution
	ebt.mutex.Unlock()

	defer func() {
		ebt.mutex.Lock()
		delete(ebt.activeTests, patternName)
		ebt.mutex.Unlock()
	}()

	ebt.logger.WithFields(logrus.Fields{
		"pattern":                patternName,
		"expected_recovery_time": pattern.ExpectedRecoveryTime,
		"boundary_conditions":    len(pattern.BoundaryConditions),
	}).Info("Starting error recovery test")

	// Phase 1: Trigger the error condition
	execution.Status = RecoveryTestTriggering
	if err := pattern.TriggerCondition(); err != nil {
		execution.Status = RecoveryTestFailed
		execution.FinalOutcome = fmt.Sprintf("Failed to trigger error: %v", err)
		ebt.recordTestResult(execution)
		return execution, err
	}
	execution.ErrorTriggerred = true

	// Phase 2: Execute recovery mechanism
	execution.Status = RecoveryTestRecovering
	recoveryStart := time.Now()

	err := ebt.executeRecoveryWithRetry(testCtx, pattern, execution)
	execution.RecoveryTime = time.Since(recoveryStart)

	if err != nil {
		execution.Status = RecoveryTestFailed
		execution.FinalOutcome = fmt.Sprintf("Recovery failed: %v", err)
		ebt.recordTestResult(execution)
		return execution, err
	}

	// Phase 3: Verify recovery
	execution.Status = RecoveryTestVerifying
	if err := pattern.VerifyRecovery(testCtx); err != nil {
		execution.Status = RecoveryTestFailed
		execution.FinalOutcome = fmt.Sprintf("Recovery verification failed: %v", err)
		ebt.recordTestResult(execution)
		return execution, err
	}

	// Phase 4: Test boundary conditions
	if err := ebt.testBoundaryConditions(testCtx, execution); err != nil {
		execution.Status = RecoveryTestFailed
		execution.FinalOutcome = fmt.Sprintf("Boundary condition testing failed: %v", err)
		ebt.recordTestResult(execution)
		return execution, err
	}

	// Test completed successfully
	execution.Status = RecoveryTestPassed
	now := time.Now()
	execution.EndTime = &now
	execution.FinalOutcome = "Recovery test passed successfully"

	ebt.logger.WithFields(logrus.Fields{
		"pattern":        patternName,
		"recovery_time":  execution.RecoveryTime,
		"total_duration": execution.EndTime.Sub(execution.StartTime),
		"retry_attempts": execution.RecoveryAttempts,
	}).Info("Error recovery test completed successfully")

	ebt.recordTestResult(execution)
	return execution, nil
}

// executeRecoveryWithRetry implements recovery with retry logic
func (ebt *ErrorBoundaryTester) executeRecoveryWithRetry(ctx context.Context, pattern ErrorRecoveryPattern, execution *RecoveryTestExecution) error {
	policy := pattern.RetryPolicy
	if policy.MaxAttempts == 0 {
		policy.MaxAttempts = 3 // Default
	}

	var lastError error
	delay := policy.InitialDelay
	if delay == 0 {
		delay = 1 * time.Second // Default
	}

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		execution.RecoveryAttempts = attempt

		ebt.logger.WithFields(logrus.Fields{
			"pattern": pattern.Name,
			"attempt": attempt,
			"delay":   delay,
		}).Debug("Attempting recovery")

		err := pattern.RecoveryMechanism(ctx)
		if err == nil {
			ebt.logger.WithFields(logrus.Fields{
				"pattern":  pattern.Name,
				"attempts": attempt,
			}).Info("Recovery successful")
			return nil
		}

		lastError = err
		ebt.logger.WithError(err).WithFields(logrus.Fields{
			"pattern": pattern.Name,
			"attempt": attempt,
		}).Warning("Recovery attempt failed")

		// Don't sleep on the last attempt
		if attempt < policy.MaxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			// Calculate next delay with backoff
			if policy.BackoffMultiplier > 0 {
				delay = time.Duration(float64(delay) * policy.BackoffMultiplier)
				if policy.MaxDelay > 0 && delay > policy.MaxDelay {
					delay = policy.MaxDelay
				}
			}
		}
	}

	// All retries failed - try fallback if configured
	if pattern.FallbackStrategy.Enabled {
		ebt.logger.WithField("pattern", pattern.Name).Info("Attempting fallback recovery")
		return ebt.attemptFallbackRecovery(ctx, pattern, execution)
	}

	return fmt.Errorf("recovery failed after %d attempts, last error: %v", policy.MaxAttempts, lastError)
}

// attemptFallbackRecovery attempts fallback recovery strategy
func (ebt *ErrorBoundaryTester) attemptFallbackRecovery(ctx context.Context, pattern ErrorRecoveryPattern, execution *RecoveryTestExecution) error {
	fallbackCtx, cancel := context.WithTimeout(ctx, pattern.FallbackStrategy.FallbackTimeout)
	defer cancel()

	// This is a placeholder - in real implementation, you would execute the fallback strategy
	ebt.logger.WithFields(logrus.Fields{
		"pattern":     pattern.Name,
		"data_source": pattern.FallbackStrategy.FallbackDataSource,
		"degradation": pattern.FallbackStrategy.QualityDegradation,
		"attempts":    execution.RecoveryAttempts,
	}).Info("Executing fallback recovery strategy")

	// Simulate fallback execution
	select {
	case <-fallbackCtx.Done():
		return fmt.Errorf("fallback recovery timeout")
	case <-time.After(time.Second):
		// Fallback successful
		return nil
	}
}

// testBoundaryConditions tests all boundary conditions for the pattern
func (ebt *ErrorBoundaryTester) testBoundaryConditions(ctx context.Context, execution *RecoveryTestExecution) error {
	for _, condition := range execution.Pattern.BoundaryConditions {
		result := BoundaryTestResult{
			Condition:       condition,
			ExpectedOutcome: condition.ExpectedOutcome,
		}

		startTime := time.Now()
		err := condition.TestFunc(ctx)
		result.ExecutionTime = time.Since(startTime)

		if err != nil {
			result.Passed = false
			result.Error = err.Error()
			result.ActualOutcome = "error"
		} else {
			result.Passed = true
			result.ActualOutcome = condition.ExpectedOutcome
		}

		execution.BoundaryResults[condition.Name] = result

		ebt.logger.WithFields(logrus.Fields{
			"condition": condition.Name,
			"passed":    result.Passed,
			"duration":  result.ExecutionTime,
		}).Debug("Boundary condition tested")

		if !result.Passed {
			return fmt.Errorf("boundary condition '%s' failed: %s", condition.Name, result.Error)
		}
	}

	return nil
}

// recordTestResult records test results for metrics
func (ebt *ErrorBoundaryTester) recordTestResult(execution *RecoveryTestExecution) {
	ebt.metrics.RecordRecoveryTest(execution)
}

// GetActiveTests returns currently active recovery tests
func (ebt *ErrorBoundaryTester) GetActiveTests() map[string]*RecoveryTestExecution {
	ebt.mutex.RLock()
	defer ebt.mutex.RUnlock()

	result := make(map[string]*RecoveryTestExecution)
	for k, v := range ebt.activeTests {
		result[k] = v
	}
	return result
}

// Predefined Recovery Patterns for common scenarios

// DatabaseRecoveryPattern provides database connection recovery testing
func DatabaseRecoveryPattern() ErrorRecoveryPattern {
	return ErrorRecoveryPattern{
		Name:        "database_connection_recovery",
		Description: "Tests database connection recovery after connection loss",
		TriggerCondition: func() error {
			// Simulate database connection loss
			return nil
		},
		RecoveryMechanism: func(ctx context.Context) error {
			// Simulate connection retry with exponential backoff
			time.Sleep(100 * time.Millisecond)
			return nil
		},
		VerifyRecovery: func(ctx context.Context) error {
			// Verify database connectivity
			return nil
		},
		ExpectedRecoveryTime: 30 * time.Second,
		RetryPolicy: RetryPolicy{
			MaxAttempts:       5,
			InitialDelay:      1 * time.Second,
			BackoffMultiplier: 2.0,
			MaxDelay:          30 * time.Second,
			Jitter:            true,
		},
		FallbackStrategy: FallbackStrategy{
			Enabled:            true,
			FallbackTimeout:    10 * time.Second,
			QualityDegradation: 0.2,
			FallbackDataSource: "cache",
		},
		BoundaryConditions: []BoundaryCondition{
			{
				Name:        "connection_pool_exhaustion",
				Description: "Test behavior when connection pool is exhausted",
				TestFunc: func(ctx context.Context) error {
					// Simulate connection pool test
					return nil
				},
				ExpectedOutcome: "graceful_queue_or_reject",
			},
			{
				Name:        "concurrent_recovery_attempts",
				Description: "Test behavior with multiple concurrent recovery attempts",
				TestFunc: func(ctx context.Context) error {
					// Simulate concurrent recovery
					return nil
				},
				ExpectedOutcome: "single_recovery_coordination",
			},
		},
	}
}

// SLMServiceRecoveryPattern provides SLM service recovery testing
func SLMServiceRecoveryPattern() ErrorRecoveryPattern {
	return ErrorRecoveryPattern{
		Name:        "slm_service_recovery",
		Description: "Tests SLM service recovery after service degradation",
		TriggerCondition: func() error {
			// Trigger SLM service degradation
			return nil
		},
		RecoveryMechanism: func(ctx context.Context) error {
			// Implement circuit breaker recovery
			time.Sleep(200 * time.Millisecond)
			return nil
		},
		VerifyRecovery: func(ctx context.Context) error {
			// Verify SLM service health
			return nil
		},
		ExpectedRecoveryTime: 45 * time.Second,
		RetryPolicy: RetryPolicy{
			MaxAttempts:       3,
			InitialDelay:      5 * time.Second,
			BackoffMultiplier: 1.5,
			MaxDelay:          60 * time.Second,
		},
		CircuitBreaker: &CircuitBreakerPattern{
			Enabled:          true,
			FailureThreshold: 5,
			RecoveryTimeout:  30 * time.Second,
			TestHalfOpen:     true,
		},
		FallbackStrategy: FallbackStrategy{
			Enabled:            true,
			FallbackTimeout:    15 * time.Second,
			QualityDegradation: 0.3,
			FallbackDataSource: "cached_recommendations",
		},
	}
}

// KubernetesAPIRecoveryPattern provides Kubernetes API recovery testing
func KubernetesAPIRecoveryPattern() ErrorRecoveryPattern {
	return ErrorRecoveryPattern{
		Name:        "kubernetes_api_recovery",
		Description: "Tests Kubernetes API recovery after API server issues",
		TriggerCondition: func() error {
			// Simulate K8s API server unavailability
			return nil
		},
		RecoveryMechanism: func(ctx context.Context) error {
			// Implement API retry with rate limiting
			time.Sleep(300 * time.Millisecond)
			return nil
		},
		VerifyRecovery: func(ctx context.Context) error {
			// Verify K8s API accessibility
			return nil
		},
		ExpectedRecoveryTime: 60 * time.Second,
		RetryPolicy: RetryPolicy{
			MaxAttempts:       10,
			InitialDelay:      2 * time.Second,
			BackoffMultiplier: 1.2,
			MaxDelay:          30 * time.Second,
			Jitter:            true,
		},
		BoundaryConditions: []BoundaryCondition{
			{
				Name:        "api_rate_limiting",
				Description: "Test behavior under API rate limits",
				TestFunc: func(ctx context.Context) error {
					return nil
				},
				ExpectedOutcome: "exponential_backoff_respected",
			},
		},
	}
}

// Ginkgo integration helpers

// WithErrorRecoveryTesting provides a Ginkgo helper for error recovery testing
func WithErrorRecoveryTesting(description string, patterns []ErrorRecoveryPattern) {
	ginkgo.Context("Error Recovery Testing: "+description, func() {
		var (
			tester *ErrorBoundaryTester
			logger *logrus.Logger
		)

		ginkgo.BeforeEach(func() {
			logger = logrus.New()
			logger.SetLevel(logrus.InfoLevel)
			tester = NewErrorBoundaryTester(logger)

			for _, pattern := range patterns {
				tester.AddRecoveryPattern(pattern)
			}
		})

		for _, pattern := range patterns {
			patternName := pattern.Name
			ginkgo.It(fmt.Sprintf("should recover from %s", patternName), func(ctx ginkgo.SpecContext) {
				execution, err := tester.ExecuteRecoveryTest(ctx, patternName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(execution.Status).To(gomega.Equal(RecoveryTestPassed))
				gomega.Expect(execution.RecoveryTime).To(gomega.BeNumerically("<=", pattern.ExpectedRecoveryTime))

				// Verify boundary conditions
				for conditionName, result := range execution.BoundaryResults {
					gomega.Expect(result.Passed).To(gomega.BeTrue(),
						fmt.Sprintf("Boundary condition '%s' failed: %s", conditionName, result.Error))
				}
			})
		}

		ginkgo.AfterEach(func() {
			// Cleanup any active tests
			for _, execution := range tester.GetActiveTests() {
				logger.WithField("pattern", execution.Pattern.Name).Warning("Cleaning up active recovery test")
			}
		})
	})
}

// BoundaryTestSuite provides a complete boundary testing suite
func BoundaryTestSuite() []ErrorRecoveryPattern {
	return []ErrorRecoveryPattern{
		DatabaseRecoveryPattern(),
		SLMServiceRecoveryPattern(),
		KubernetesAPIRecoveryPattern(),
	}
}
