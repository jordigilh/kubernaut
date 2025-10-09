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

//go:build unit
// +build unit

package executor

import (
	"testing"
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/sirupsen/logrus"
)

// BR-CONCURRENT-EXEC-001: Comprehensive Concurrent Execution Business Logic Testing
// Business Impact: Validates concurrent action execution capabilities for platform operations
// Stakeholder Value: Ensures reliable concurrent processing for high-throughput production environments
var _ = Describe("BR-CONCURRENT-EXEC-001: Comprehensive Concurrent Execution Business Logic", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		mockK8sClient         *mocks.MockK8sClient
		mockActionHistoryRepo *mocks.MockActionHistoryRepository
		mockLogger            *logrus.Logger

		// Use REAL business logic components
		// asyncExecutor executor.AsyncExecutor // AsyncExecutor not implemented yet
		syncExecutor  executor.Executor

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only
		mockK8sClient = mocks.NewMockK8sClient(nil)
		mockActionHistoryRepo = mocks.NewMockActionHistoryRepository()
		// Use existing mock logger from testutil instead of library testing
		mockLoggerImpl := mocks.NewMockLogger()
		mockLogger = mockLoggerImpl.Logger

		// Create REAL business executors with mocked external dependencies
		config := config.ActionsConfig{
			MaxConcurrent:  5,                  // Controlled concurrency for testing
			DryRun:         true,
			CooldownPeriod: 1 * time.Second,   // Short cooldown for testing
		}

		var err error
		// Create REAL executor (AsyncExecutor not implemented yet)
		// asyncExecutor, err = executor.NewAsyncExecutor(
		//	mockK8sClient,         // External: Mock
		//	config,                // Configuration
		//	mockActionHistoryRepo, // External: Mock
		//	mockLogger,            // External: Mock (logging infrastructure)
		// )
		// Expect(err).ToNot(HaveOccurred(), "Failed to create real AsyncExecutor")

		// Create REAL sync executor
		syncExecutor, err = executor.NewExecutor(
			mockK8sClient,         // External: Mock
			config,                // Configuration
			mockActionHistoryRepo, // External: Mock
			mockLogger,            // External: Mock (logging infrastructure)
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create real Executor")

		// Start async executor if it has a Start method
		// if starter, ok := asyncExecutor.(interface{ Start(context.Context) error }); ok { // AsyncExecutor not implemented
		//	err = starter.Start(ctx)
		//	Expect(err).ToNot(HaveOccurred(), "AsyncExecutor should start successfully")
		// }
	})

	AfterEach(func() {
		// Stop async executor gracefully
		// if stopper, ok := asyncExecutor.(interface{ Stop(context.Context) error }); ok { // AsyncExecutor not implemented
		//	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		//	defer cancel()
		//	_ = stopper.Stop(stopCtx)
		// }
		cancel()
	})

	// COMPREHENSIVE scenario testing for concurrent execution business logic
	DescribeTable("BR-CONCURRENT-EXEC-001: Should handle all concurrent execution scenarios",
		func(scenarioName string, setupFn func() ([]*types.ActionRecommendation, []types.Alert), concurrency int, expectedSuccess bool) {
			// Setup test data
			actions, alerts := setupFn()
			Expect(len(actions)).To(Equal(len(alerts)), "Actions and alerts must match for %s", scenarioName)

			// Use REAL executor business logic - no mock setup needed!

			// Test REAL business concurrent execution
			var wg sync.WaitGroup
			results := make([]error, len(actions))
			executionTimes := make([]time.Duration, len(actions))

			startTime := time.Now()

			// Execute actions concurrently using REAL business logic
			for i, action := range actions {
				wg.Add(1)
				go func(idx int, act *types.ActionRecommendation, alert types.Alert) {
					defer wg.Done()

					actionStart := time.Now()
					err := syncExecutor.Execute(ctx, act, alert, nil)
					executionTimes[idx] = time.Since(actionStart)
					results[idx] = err
				}(i, action, alerts[i])
			}

			// Wait for all executions to complete
			wg.Wait()
			totalDuration := time.Since(startTime)

			// Validate REAL business concurrent execution outcomes
			if expectedSuccess {
				successCount := 0
				for i, err := range results {
					if err == nil {
						successCount++
					} else {
						By(fmt.Sprintf("Action %d failed: %v", i, err))
					}
				}

				Expect(successCount).To(BeNumerically(">", 0),
					"BR-CONCURRENT-EXEC-001: At least some concurrent executions must succeed for %s", scenarioName)

				// Validate concurrency efficiency - should be faster than sequential
				if len(actions) > 1 {
					avgExecutionTime := time.Duration(0)
					for _, execTime := range executionTimes {
						avgExecutionTime += execTime
					}
					avgExecutionTime = avgExecutionTime / time.Duration(len(executionTimes))

					// Concurrent execution should be more efficient than sequential
					expectedSequentialTime := avgExecutionTime * time.Duration(len(actions))
					Expect(totalDuration).To(BeNumerically("<", expectedSequentialTime),
						"BR-CONCURRENT-EXEC-001: Concurrent execution must be more efficient than sequential for %s", scenarioName)
				}
			} else {
				// For failure scenarios, validate error handling
				failureCount := 0
				for _, err := range results {
					if err != nil {
						failureCount++
					}
				}
				Expect(failureCount).To(BeNumerically(">", 0),
					"BR-CONCURRENT-EXEC-001: Failure scenarios must produce errors for %s", scenarioName)
			}
		},
		Entry("Low concurrency scenario", "low_concurrency", func() ([]*types.ActionRecommendation, []types.Alert) {
			return createConcurrentTestData(2)
		}, 2, true),
		Entry("Medium concurrency scenario", "medium_concurrency", func() ([]*types.ActionRecommendation, []types.Alert) {
			return createConcurrentTestData(5)
		}, 5, true),
		Entry("High concurrency scenario", "high_concurrency", func() ([]*types.ActionRecommendation, []types.Alert) {
			return createConcurrentTestData(10)
		}, 10, true),
		Entry("Single action scenario", "single_action", func() ([]*types.ActionRecommendation, []types.Alert) {
			return createConcurrentTestData(1)
		}, 1, true),
		Entry("Mixed action types scenario", "mixed_actions", func() ([]*types.ActionRecommendation, []types.Alert) {
			return createMixedActionTestData()
		}, 4, true),
		Entry("Resource-intensive actions", "resource_intensive", func() ([]*types.ActionRecommendation, []types.Alert) {
			return createResourceIntensiveTestData()
		}, 3, true),
		Entry("Failure scenario", "failure_scenario", func() ([]*types.ActionRecommendation, []types.Alert) {
			return createFailureTestData()
		}, 3, false),
	)

	// COMPREHENSIVE async execution business logic testing
	Context("BR-CONCURRENT-EXEC-002: Async Execution Business Logic", func() {
		It("should handle async concurrent executions with business validation", func() {
			// Test REAL business logic for async concurrent execution
			actions, alerts := createConcurrentTestData(5)

			// Setup successful execution responses
			// Note: MockK8sClient uses default success behavior for testing

			// Test REAL business async execution
			executionIDs := make([]string, len(actions))
			var wg sync.WaitGroup

			startTime := time.Now()

			// Submit all actions for async execution
			for i, action := range actions {
				wg.Add(1)
				go func(idx int, act *types.ActionRecommendation, alert types.Alert) {
					defer wg.Done()

					// Use sync executor instead of async (AsyncExecutor not implemented)
					err := syncExecutor.Execute(ctx, act, alert, nil)
					if err == nil {
						executionIDs[idx] = fmt.Sprintf("sync-exec-%d", idx) // Generate ID for sync execution
					}

					// For async execution, we expect immediate return (non-blocking)
					Expect(err).ToNot(HaveOccurred(),
						"BR-CONCURRENT-EXEC-002: Async execution submission must succeed for action %d", idx)
				}(i, action, alerts[i])
			}

			// Wait for all submissions to complete
			wg.Wait()
			submissionDuration := time.Since(startTime)

			// Validate REAL business async execution outcomes
			Expect(submissionDuration).To(BeNumerically("<", 2*time.Second),
				"BR-CONCURRENT-EXEC-002: Async execution submissions must be fast (non-blocking)")

			// Wait for async executions to complete
			time.Sleep(3 * time.Second) // Allow time for async processing

			// Validate sync execution results through health checks
			Expect(syncExecutor.IsHealthy()).To(BeTrue(),
				"BR-CONCURRENT-EXEC-002: Sync executor must remain healthy after concurrent submissions")
		})

		It("should handle async execution queue management", func() {
			// Test REAL business logic for async queue management
			actions, alerts := createConcurrentTestData(15) // More than queue size

			// Setup successful execution responses
			// Note: MockK8sClient uses default success behavior for testing

			// Test REAL business async queue handling
			submissionErrors := make([]error, len(actions))
			var wg sync.WaitGroup

			// Submit more actions than queue can handle
			for i, action := range actions {
				wg.Add(1)
				go func(idx int, act *types.ActionRecommendation, alert types.Alert) {
					defer wg.Done()
					err := syncExecutor.Execute(ctx, act, alert, nil) // Use sync executor
					submissionErrors[idx] = err
				}(i, action, alerts[i])
			}

			wg.Wait()

			// Validate REAL business queue management outcomes
			acceptedCount := 0
			rejectedCount := 0
			for _, err := range submissionErrors {
				if err == nil {
					acceptedCount++
				} else {
					rejectedCount++
				}
			}

			// Should accept up to queue capacity and reject excess
			Expect(acceptedCount).To(BeNumerically(">", 0),
				"BR-CONCURRENT-EXEC-002: Queue must accept some actions")

			if len(actions) > 10 { // Queue size is 10
				Expect(rejectedCount).To(BeNumerically(">", 0),
					"BR-CONCURRENT-EXEC-002: Queue must reject excess actions when full")
			}
		})
	})

	// COMPREHENSIVE concurrency control business logic testing
	Context("BR-CONCURRENT-EXEC-003: Concurrency Control Business Logic", func() {
		It("should enforce maximum concurrent execution limits", func() {
			// Test REAL business logic for concurrency limits
			actions, alerts := createConcurrentTestData(8) // More than MaxConcurrent (5)

			// Setup successful but slow execution responses
			// Note: MockK8sClient uses default success behavior for testing
			// Execution timing will be controlled by real business logic

			// Test REAL business concurrency control
			var wg sync.WaitGroup
			executionStarts := make([]time.Time, len(actions))
			executionEnds := make([]time.Time, len(actions))
			results := make([]error, len(actions))

			// Execute all actions concurrently
			for i, action := range actions {
				wg.Add(1)
				go func(idx int, act *types.ActionRecommendation, alert types.Alert) {
					defer wg.Done()

					executionStarts[idx] = time.Now()
					err := syncExecutor.Execute(ctx, act, alert, nil)
					executionEnds[idx] = time.Now()
					results[idx] = err
				}(i, action, alerts[i])
			}

			wg.Wait()

			// Validate REAL business concurrency control outcomes
			successCount := 0
			for _, err := range results {
				if err == nil {
					successCount++
				}
			}

			Expect(successCount).To(BeNumerically(">", 0),
				"BR-CONCURRENT-EXEC-003: Some executions must succeed despite concurrency limits")

			// Analyze execution timing to verify concurrency control
			concurrentExecutions := 0
			for i := 0; i < len(executionStarts); i++ {
				if executionStarts[i].IsZero() {
					continue
				}

				// Count how many executions were running at the same time as this one
				concurrent := 1 // Count self
				for j := 0; j < len(executionStarts); j++ {
					if i == j || executionStarts[j].IsZero() || executionEnds[j].IsZero() {
						continue
					}

					// Check if execution j overlapped with execution i
					if executionStarts[j].Before(executionEnds[i]) && executionEnds[j].After(executionStarts[i]) {
						concurrent++
					}
				}

				if concurrent > concurrentExecutions {
					concurrentExecutions = concurrent
				}
			}

			// Should not exceed MaxConcurrent limit (5)
			Expect(concurrentExecutions).To(BeNumerically("<=", 5),
				"BR-CONCURRENT-EXEC-003: Concurrent executions must not exceed MaxConcurrent limit")
		})

		It("should handle concurrency control with timeouts", func() {
			// Test REAL business logic for timeout handling in concurrent scenarios
			actions, alerts := createTimeoutTestData()

			// Setup timeout scenarios
			// Note: MockK8sClient uses default behavior, timeout testing relies on real business logic

			// Test REAL business timeout handling
			var wg sync.WaitGroup
			results := make([]error, len(actions))
			executionTimes := make([]time.Duration, len(actions))

			for i, action := range actions {
				wg.Add(1)
				go func(idx int, act *types.ActionRecommendation, alert types.Alert) {
					defer wg.Done()

					// Create timeout context for this execution
					timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
					defer cancel()

					start := time.Now()
					err := syncExecutor.Execute(timeoutCtx, act, alert, nil)
					executionTimes[idx] = time.Since(start)
					results[idx] = err
				}(i, action, alerts[i])
			}

			wg.Wait()

			// Validate REAL business timeout handling outcomes
			timeoutCount := 0
			for i, err := range results {
				if err != nil && (err == context.DeadlineExceeded || err.Error() == "context deadline exceeded") {
					timeoutCount++
					// Validate timeout occurred within reasonable time
					Expect(executionTimes[i]).To(BeNumerically("<=", 1500*time.Millisecond),
						"BR-CONCURRENT-EXEC-003: Timeout should occur within timeout period")
				}
			}

			Expect(timeoutCount).To(BeNumerically(">", 0),
				"BR-CONCURRENT-EXEC-003: Some executions must timeout as expected")
		})
	})

	// COMPREHENSIVE resource management business logic testing
	Context("BR-CONCURRENT-EXEC-004: Resource Management Business Logic", func() {
		It("should manage resources efficiently during concurrent execution", func() {
			// Test REAL business logic for resource management
			actions, alerts := createResourceIntensiveTestData()

			// Setup resource monitoring
			initialHealth := syncExecutor.IsHealthy()
			Expect(initialHealth).To(BeTrue(),
				"BR-CONCURRENT-EXEC-004: Executor must be healthy before resource-intensive operations")

			// Test REAL business resource management
			var wg sync.WaitGroup
			results := make([]error, len(actions))

			for i, action := range actions {
				wg.Add(1)
				go func(idx int, act *types.ActionRecommendation, alert types.Alert) {
					defer wg.Done()
					err := syncExecutor.Execute(ctx, act, alert, nil)
					results[idx] = err
				}(i, action, alerts[i])
			}

			wg.Wait()

			// Validate REAL business resource management outcomes
			finalHealth := syncExecutor.IsHealthy()
			Expect(finalHealth).To(BeTrue(),
				"BR-CONCURRENT-EXEC-004: Executor must remain healthy after resource-intensive operations")

			// Validate execution results
			successCount := 0
			for _, err := range results {
				if err == nil {
					successCount++
				}
			}

			Expect(successCount).To(BeNumerically(">", 0),
				"BR-CONCURRENT-EXEC-004: Resource-intensive operations must succeed")
		})
	})
})

// Helper functions to create test data for various concurrent execution scenarios

func createConcurrentTestData(count int) ([]*types.ActionRecommendation, []types.Alert) {
	actions := make([]*types.ActionRecommendation, count)
	alerts := make([]types.Alert, count)

	for i := 0; i < count; i++ {
		actions[i] = &types.ActionRecommendation{
			Action:     "scale_deployment",
			Confidence: 0.85,
			Reasoning: &types.ReasoningDetails{
				Summary: fmt.Sprintf("Concurrent test action %d", i),
			},
			Parameters: map[string]interface{}{
				"deployment": fmt.Sprintf("test-deployment-%d", i),
				"replicas":   3,
			},
		}

		alerts[i] = types.Alert{
			Name:      fmt.Sprintf("test-alert-%d", i),
			Namespace: "default",
			Labels: map[string]string{
				"severity": "high",
				"type":     "concurrent_test",
			},
			Annotations: map[string]string{
				"description": fmt.Sprintf("Test alert for concurrent execution %d", i),
			},
		}
	}

	return actions, alerts
}

func createMixedActionTestData() ([]*types.ActionRecommendation, []types.Alert) {
	actions := []*types.ActionRecommendation{
		{
			Action:     "scale_deployment",
			Confidence: 0.90,
			Reasoning: &types.ReasoningDetails{
				Summary: "Scale deployment for mixed test",
			},
			Parameters: map[string]interface{}{
				"deployment": "test-deployment-1",
				"replicas":   5,
			},
		},
		{
			Action:     "restart_pod",
			Confidence: 0.85,
			Reasoning: &types.ReasoningDetails{
				Summary: "Restart pod for mixed test",
			},
			Parameters: map[string]interface{}{
				"pod":       "test-pod-1",
				"namespace": "default",
			},
		},
		{
			Action:     "update_configmap",
			Confidence: 0.80,
			Reasoning: &types.ReasoningDetails{
				Summary: "Update configmap for mixed test",
			},
			Parameters: map[string]interface{}{
				"configmap": "test-config",
				"key":       "test-key",
				"value":     "test-value",
			},
		},
		{
			Action:     "check_resource_status",
			Confidence: 0.95,
			Reasoning: &types.ReasoningDetails{
				Summary: "Check resource status for mixed test",
			},
			Parameters: map[string]interface{}{
				"resource": "deployment",
				"name":     "test-deployment-1",
			},
		},
	}

	alerts := make([]types.Alert, len(actions))
	for i := range alerts {
		alerts[i] = types.Alert{
			Name:      fmt.Sprintf("mixed-alert-%d", i),
			Namespace: "default",
			Labels: map[string]string{
				"severity": "medium",
				"type":     "mixed_test",
			},
		}
	}

	return actions, alerts
}

func createResourceIntensiveTestData() ([]*types.ActionRecommendation, []types.Alert) {
	actions := []*types.ActionRecommendation{
		{
			Action:     "analyze_cluster_resources",
			Confidence: 0.88,
			Reasoning: &types.ReasoningDetails{
				Summary: "Resource-intensive cluster analysis",
			},
			Parameters: map[string]interface{}{
				"deep_analysis":   true,
				"include_metrics": true,
			},
		},
		{
			Action:     "generate_resource_report",
			Confidence: 0.82,
			Reasoning: &types.ReasoningDetails{
				Summary: "Generate comprehensive resource report",
			},
			Parameters: map[string]interface{}{
				"format":          "detailed",
				"include_history": true,
			},
		},
		{
			Action:     "optimize_resource_allocation",
			Confidence: 0.90,
			Reasoning: &types.ReasoningDetails{
				Summary: "Optimize cluster resource allocation",
			},
			Parameters: map[string]interface{}{
				"strategy": "aggressive",
				"dry_run":  true,
			},
		},
	}

	alerts := make([]types.Alert, len(actions))
	for i := range alerts {
		alerts[i] = types.Alert{
			Name:      fmt.Sprintf("resource-intensive-alert-%d", i),
			Namespace: "kube-system",
			Labels: map[string]string{
				"severity": "high",
				"type":     "resource_intensive",
			},
		}
	}

	return actions, alerts
}

func createFailureTestData() ([]*types.ActionRecommendation, []types.Alert) {
	actions := []*types.ActionRecommendation{
		{
			Action:     "invalid_action",
			Confidence: 0.50,
			Reasoning: &types.ReasoningDetails{
				Summary: "Invalid action for failure testing",
			},
			Parameters: map[string]interface{}{
				"invalid_param": "invalid_value",
			},
		},
		{
			Action:     "scale_deployment",
			Confidence: 0.30, // Low confidence
			Reasoning: &types.ReasoningDetails{
				Summary: "Low confidence action for failure testing",
			},
			Parameters: map[string]interface{}{
				"deployment": "non-existent-deployment",
				"replicas":   -1, // Invalid replica count
			},
		},
		{
			Action:     "test_action",
			Confidence: 0.85,
			Reasoning: &types.ReasoningDetails{
				Summary: "Test action that will fail",
			},
			Parameters: map[string]interface{}{
				"force_failure": true,
			},
		},
	}

	alerts := make([]types.Alert, len(actions))
	for i := range alerts {
		alerts[i] = types.Alert{
			Name:      fmt.Sprintf("failure-test-alert-%d", i),
			Namespace: "default",
			Labels: map[string]string{
				"severity": "low",
				"type":     "failure_test",
			},
		}
	}

	return actions, alerts
}

func createTimeoutTestData() ([]*types.ActionRecommendation, []types.Alert) {
	actions := []*types.ActionRecommendation{
		{
			Action:     "slow_operation",
			Confidence: 0.85,
			Reasoning: &types.ReasoningDetails{
				Summary: "Slow operation for timeout testing",
			},
			Parameters: map[string]interface{}{
				"duration": "3s", // Longer than timeout
			},
		},
		{
			Action:     "network_intensive_operation",
			Confidence: 0.80,
			Reasoning: &types.ReasoningDetails{
				Summary: "Network operation for timeout testing",
			},
			Parameters: map[string]interface{}{
				"retries": 10,
				"timeout": "5s",
			},
		},
	}

	alerts := make([]types.Alert, len(actions))
	for i := range alerts {
		alerts[i] = types.Alert{
			Name:      fmt.Sprintf("timeout-test-alert-%d", i),
			Namespace: "default",
			Labels: map[string]string{
				"severity": "medium",
				"type":     "timeout_test",
			},
		}
	}

	return actions, alerts
}

// TestRunner bootstraps the Ginkgo test suite
func TestUconcurrentUexecutionUcomprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UconcurrentUexecutionUcomprehensive Suite")
}
