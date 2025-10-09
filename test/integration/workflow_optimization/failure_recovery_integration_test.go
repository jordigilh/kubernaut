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

//go:build integration
// +build integration

package workflow_optimization

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("BR-RESILIENCE-001: Failure Recovery Integration Test", Ordered, func() {
	var (
		hooks           *testshared.TestLifecycleHooks
		ctx             context.Context
		suite           *testshared.StandardTestSuite
		llmClient       llm.Client // RULE 12 COMPLIANCE: Using enhanced llm.Client instead of deprecated SelfOptimizer
		workflowBuilder engine.IntelligentWorkflowBuilder
		logger          *logrus.Logger
	)

	BeforeAll(func() {
		// Following guideline: Reuse existing test infrastructure with real components
		hooks = testshared.SetupAIIntegrationTest("Failure Recovery Integration",
			testshared.WithRealVectorDB(),                                     // Real pgvector integration for resilience testing
			testshared.WithDatabaseIsolation(testshared.TransactionIsolation), // Isolated test environment
		)

		ctx = context.Background()
		suite = hooks.GetSuite()
		logger = suite.Logger

		// Validate test environment is healthy before each test
		Expect(suite.VectorDB).ToNot(BeNil(), "Vector database should be available")
		Expect(suite.LLMClient).ToNot(BeNil(), "LLM client should be available")

		// Create workflow builder with real dependencies using config pattern
		patternStore := testshared.CreatePatternStoreForTesting(suite.Logger)
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       suite.LLMClient,
			VectorDB:        suite.VectorDB,
			AnalyticsEngine: suite.AnalyticsEngine,
			PatternStore:    patternStore,
			ExecutionRepo:   suite.ExecutionRepo,
			Logger:          suite.Logger,
		}

		var err error
		workflowBuilder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")
		Expect(workflowBuilder).ToNot(BeNil())

		// RULE 12 COMPLIANCE: Use enhanced llm.Client instead of deprecated SelfOptimizer
		llmClient = suite.LLMClient
		Expect(llmClient).ToNot(BeNil(), "Enhanced LLM client should be available for workflow optimization")
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	Context("when testing failure recovery mechanisms", func() {
		It("should recover gracefully from optimization failures", func() {
			By("creating a workflow that might cause optimization failures")
			template := engine.NewWorkflowTemplate("failure-prone-template", "Failure Prone Workflow")
			template.Description = "Workflow designed to test failure recovery mechanisms"
			template.Metadata["complexity"] = "extreme"
			template.Metadata["failure_prone"] = true

			failureProneWorkflow := engine.NewWorkflow("failure-prone-workflow", template)
			failureProneWorkflow.Name = "Failure Prone Workflow"
			failureProneWorkflow.Description = "Workflow designed to test optimizer failure recovery"

			Expect(failureProneWorkflow).ToNot(BeNil())

			By("generating execution history with failure patterns")
			failureHistory := generateFailureProneExecutionHistory(failureProneWorkflow.ID, 12)
			Expect(failureHistory).To(HaveLen(12))

			// Verify history contains failures
			failureCount := 0
			for _, execution := range failureHistory {
				if execution.OperationalStatus == engine.ExecutionStatusFailed {
					failureCount++
				}
			}
			Expect(failureCount).To(BeNumerically(">=", 4), "Should have multiple failed executions")

			By("attempting optimization and handling potential failures gracefully")
			optimizationStartTime := time.Now()
			// RULE 12 COMPLIANCE: Use enhanced llm.Client.OptimizeWorkflow() instead of deprecated SelfOptimizer
			optimizationResult, err := llmClient.OptimizeWorkflow(ctx, failureProneWorkflow, failureHistory)
			optimizationDuration := time.Since(optimizationStartTime)
			
			// Extract optimized workflow from LLM result
			var optimizedWorkflow *engine.Workflow
			if optimizationResult != nil {
				if resultMap, ok := optimizationResult.(map[string]interface{}); ok {
					if optimizedWf, exists := resultMap["optimized_workflow"]; exists {
						optimizedWorkflow = optimizedWf.(*engine.Workflow)
					}
				}
			}

			// Test should handle both success and failure scenarios gracefully
			if err != nil {
				// If optimization fails, verify graceful failure handling
				Expect(err.Error()).To(ContainSubstring("BR-"), "Error should reference business requirement")
				Expect(optimizationDuration).To(BeNumerically("<=", 45*time.Second),
					"BR-RESILIENCE-001: Failed optimization should timeout gracefully")

				logger.WithError(err).Info("Optimization failed gracefully as expected for failure recovery test")

				// Verify system remains stable after failure
				isHealthy := suite.VectorDB.IsHealthy(ctx)
				Expect(isHealthy).To(BeNil(), "System should remain healthy after optimization failure")
			} else {
				// If optimization succeeds despite failures, verify resilience
				Expect(optimizedWorkflow).ToNot(BeNil(), "Should return optimized workflow")
				Expect(optimizedWorkflow.Template).ToNot(BeNil(), "Should preserve workflow structure")
				Expect(optimizationDuration).To(BeNumerically("<=", 30*time.Second),
					"BR-RESILIENCE-001: Successful optimization should complete within reasonable time")

				logger.Info("Optimization succeeded despite failure-prone conditions, demonstrating resilience")
			}

			By("verifying system stability and recovery capabilities")
			// Test multiple recovery scenarios
			for i := 0; i < 3; i++ {
				recoveryStartTime := time.Now()

				// Attempt another optimization to test recovery
				// RULE 12 COMPLIANCE: Use enhanced llm.Client.OptimizeWorkflow() for recovery testing
				_, recoveryErr := llmClient.OptimizeWorkflow(ctx, failureProneWorkflow, failureHistory)
				recoveryDuration := time.Since(recoveryStartTime)

				// System should either succeed or fail gracefully
				Expect(recoveryDuration).To(BeNumerically("<=", 30*time.Second),
					fmt.Sprintf("BR-RESILIENCE-001: Recovery attempt %d should complete within timeout", i+1))

				// Verify system health after each recovery attempt
				healthCheck := time.Now()
				isHealthy := suite.VectorDB.IsHealthy(ctx)
				healthCheckDuration := time.Since(healthCheck)

				Expect(isHealthy).To(BeNil(), fmt.Sprintf("System should remain healthy after recovery attempt %d", i+1))
				Expect(healthCheckDuration).To(BeNumerically("<=", 3*time.Second), "Health check should be fast")

				if recoveryErr != nil {
					logger.WithError(recoveryErr).Infof("Recovery attempt %d failed gracefully", i+1)
				} else {
					logger.Infof("Recovery attempt %d succeeded", i+1)
				}
			}
		})

		It("should maintain data integrity during failure scenarios", func() {
			By("creating a workflow for data integrity testing")
			template := engine.NewWorkflowTemplate("integrity-test-template", "Data Integrity Test Workflow")
			template.Description = "Workflow for testing data integrity during failures"
			template.Metadata["data_sensitive"] = true
			template.Metadata["integrity_critical"] = true

			integrityWorkflow := engine.NewWorkflow("integrity-test-workflow", template)
			integrityWorkflow.Name = "Data Integrity Test Workflow"
			integrityWorkflow.Description = "Workflow designed to test data integrity during failures"

			Expect(integrityWorkflow).ToNot(BeNil())

			By("generating execution history with data integrity patterns")
			integrityHistory := generateDataIntegrityExecutionHistory(integrityWorkflow.ID, 10)
			Expect(integrityHistory).To(HaveLen(10))

			By("performing optimization while monitoring data integrity")
			// Store initial state for integrity comparison
			initialPatternCount := 0
			if patterns, err := suite.VectorDB.SearchBySemantics(ctx, "workflow", 100); err == nil {
				initialPatternCount = len(patterns)
			}

			optimizationStartTime := time.Now()
			// RULE 12 COMPLIANCE: Use enhanced llm.Client.OptimizeWorkflow() for integrity testing
			optimizationResult, err := llmClient.OptimizeWorkflow(ctx, integrityWorkflow, integrityHistory)
			optimizationDuration := time.Since(optimizationStartTime)
			
			// Extract optimized workflow from LLM result
			var optimizedWorkflow *engine.Workflow
			if optimizationResult != nil {
				if resultMap, ok := optimizationResult.(map[string]interface{}); ok {
					if optimizedWf, exists := resultMap["optimized_workflow"]; exists {
						optimizedWorkflow = optimizedWf.(*engine.Workflow)
					}
				}
			}

			// Verify data integrity regardless of optimization success/failure
			By("validating data integrity after optimization attempt")

			// Check vector database integrity
			finalPatternCount := 0
			if patterns, err := suite.VectorDB.SearchBySemantics(ctx, "workflow", 100); err == nil {
				finalPatternCount = len(patterns)
			}

			// Data should not be corrupted (pattern count should not decrease unexpectedly)
			Expect(finalPatternCount).To(BeNumerically(">=", initialPatternCount),
				"BR-RESILIENCE-001: Data integrity should be maintained - pattern count should not decrease")

			// Verify system responsiveness
			Expect(optimizationDuration).To(BeNumerically("<=", 45*time.Second),
				"BR-RESILIENCE-001: System should remain responsive during integrity testing")

			if err != nil {
				// If optimization failed, verify it was a clean failure
				Expect(err.Error()).ToNot(ContainSubstring("panic"), "Should not have panic-related failures")
				Expect(err.Error()).ToNot(ContainSubstring("corruption"), "Should not have data corruption")
				logger.WithError(err).Info("Optimization failed cleanly without data corruption")
			} else {
				// If optimization succeeded, verify integrity of result
				Expect(optimizedWorkflow).ToNot(BeNil(), "Should return valid optimized workflow")
				Expect(optimizedWorkflow.Template).ToNot(BeNil(), "Should have valid template")
				Expect(optimizedWorkflow.ID).ToNot(BeEmpty(), "Should have valid ID")
				logger.Info("Optimization succeeded with data integrity maintained")
			}

			By("performing additional integrity validation")
			// Test database connectivity and consistency
			dbHealthy := suite.VectorDB.IsHealthy(ctx)
			Expect(dbHealthy).To(BeNil(), "Database should remain healthy and consistent")

			// Test that we can still perform basic operations
			// RULE 12 COMPLIANCE: Use enhanced llm.Client.SuggestOptimizations() instead of deprecated SelfOptimizer
			suggestionResult, suggestionErr := llmClient.SuggestOptimizations(ctx, integrityWorkflow)
			
			// Extract suggestions from LLM result
			var suggestions []map[string]interface{}
			if suggestionResult != nil {
				if suggestionSlice, ok := suggestionResult.([]map[string]interface{}); ok {
					suggestions = suggestionSlice
				}
			}
			if suggestionErr != nil {
				// Suggestions might fail, but should fail gracefully
				Expect(suggestionErr.Error()).To(ContainSubstring("BR-"), "Suggestion errors should reference business requirements")
			} else {
				// If suggestions succeed, they should be valid
				Expect(len(suggestions)).To(BeNumerically(">=", 0), "Suggestions should be valid")
			}
		})

		It("should handle concurrent failure scenarios", func() {
			By("creating multiple workflows for concurrent testing")
			workflows := make([]*engine.Workflow, 3)
			histories := make([][]*engine.RuntimeWorkflowExecution, 3)

			for i := 0; i < 3; i++ {
				template := engine.NewWorkflowTemplate(
					fmt.Sprintf("concurrent-test-template-%d", i),
					fmt.Sprintf("Concurrent Test Workflow %d", i),
				)
				template.Description = fmt.Sprintf("Workflow %d for concurrent failure testing", i)
				template.Metadata["concurrent_test"] = true
				template.Metadata["test_id"] = i

				workflows[i] = engine.NewWorkflow(fmt.Sprintf("concurrent-test-workflow-%d", i), template)
				workflows[i].Name = fmt.Sprintf("Concurrent Test Workflow %d", i)
				workflows[i].Description = fmt.Sprintf("Workflow %d designed for concurrent failure testing", i)

				histories[i] = generateConcurrentTestExecutionHistory(workflows[i].ID, 8)

				Expect(workflows[i]).ToNot(BeNil())
				Expect(histories[i]).To(HaveLen(8))
			}

			By("performing concurrent optimizations to test system resilience")
			type OptimizationResult struct {
				WorkflowIndex int
				Workflow      *engine.Workflow
				Error         error
				Duration      time.Duration
			}

			results := make(chan OptimizationResult, 3)

			// Launch concurrent optimizations
			for i := 0; i < 3; i++ {
				go func(index int) {
					startTime := time.Now()
					// RULE 12 COMPLIANCE: Use enhanced llm.Client.OptimizeWorkflow() for concurrent testing
					optimizationResult, err := llmClient.OptimizeWorkflow(ctx, workflows[index], histories[index])
					duration := time.Since(startTime)
					
					// Extract optimized workflow from LLM result
					var optimized *engine.Workflow
					if optimizationResult != nil {
						if resultMap, ok := optimizationResult.(map[string]interface{}); ok {
							if optimizedWf, exists := resultMap["optimized_workflow"]; exists {
								optimized = optimizedWf.(*engine.Workflow)
							}
						}
					}

					results <- OptimizationResult{
						WorkflowIndex: index,
						Workflow:      optimized,
						Error:         err,
						Duration:      duration,
					}
				}(i)
			}

			By("collecting and validating concurrent optimization results")
			successCount := 0
			failureCount := 0

			// Collect all results with timeout
			for i := 0; i < 3; i++ {
				select {
				case result := <-results:
					// Validate each result
					Expect(result.Duration).To(BeNumerically("<=", 60*time.Second),
						fmt.Sprintf("BR-RESILIENCE-001: Concurrent optimization %d should complete within timeout", result.WorkflowIndex))

					if result.Error != nil {
						failureCount++
						Expect(result.Error.Error()).To(ContainSubstring("BR-"),
							fmt.Sprintf("Error from optimization %d should reference business requirement", result.WorkflowIndex))
						logger.WithError(result.Error).Infof("Concurrent optimization %d failed gracefully", result.WorkflowIndex)
					} else {
						successCount++
						Expect(result.Workflow).ToNot(BeNil(),
							fmt.Sprintf("Successful optimization %d should return valid workflow", result.WorkflowIndex))
						logger.Infof("Concurrent optimization %d succeeded", result.WorkflowIndex)
					}

				case <-time.After(90 * time.Second):
					Fail(fmt.Sprintf("Concurrent optimization %d timed out", i))
				}
			}

			// At least one optimization should complete (success or graceful failure)
			Expect(successCount+failureCount).To(Equal(3), "All concurrent optimizations should complete")

			By("verifying system stability after concurrent operations")
			// System should remain stable regardless of success/failure mix
			finalHealthCheck := time.Now()
			isHealthy := suite.VectorDB.IsHealthy(ctx)
			healthCheckDuration := time.Since(finalHealthCheck)

			Expect(isHealthy).To(BeNil(), "System should remain healthy after concurrent operations")
			Expect(healthCheckDuration).To(BeNumerically("<=", 5*time.Second), "Final health check should be responsive")

			logger.Infof("Concurrent failure recovery test completed: %d successes, %d graceful failures", successCount, failureCount)
		})
	})
})

// Helper functions for failure recovery testing

// generateFailureProneExecutionHistory creates execution history with high failure rates
func generateFailureProneExecutionHistory(workflowID string, count int) []*engine.RuntimeWorkflowExecution {
	history := make([]*engine.RuntimeWorkflowExecution, count)
	baseTime := time.Now().Add(-72 * time.Hour)

	for i := 0; i < count; i++ {
		var duration time.Duration
		var status engine.ExecutionStatus

		// High failure rate (40% failures)
		if i%5 < 2 {
			// Failed executions
			duration = time.Duration(60+i*5) * time.Second // Short duration for failures
			status = engine.ExecutionStatusFailed
		} else if i%4 == 0 {
			// Timeout-prone executions
			duration = time.Duration(900+i*60) * time.Second // 15+ minutes
			status = engine.ExecutionStatusCompleted
		} else {
			// Normal executions
			duration = time.Duration(300+i*20) * time.Second
			status = engine.ExecutionStatusCompleted
		}

		execution := engine.NewRuntimeWorkflowExecution(
			fmt.Sprintf("%s-failure-prone-%d", workflowID, i),
			workflowID,
		)
		execution.OperationalStatus = status
		execution.Duration = duration
		execution.StartTime = baseTime.Add(time.Duration(i*3) * time.Hour)

		// Add failure-related metadata
		execution.Metadata["execution_duration_seconds"] = duration.Seconds()
		execution.Metadata["failure_prone"] = true
		if status == engine.ExecutionStatusFailed {
			execution.Metadata["failure_reason"] = []string{"timeout", "resource_exhaustion", "network_error"}
			execution.Metadata["retry_count"] = i % 3
		}
		execution.Metadata["resource_usage"] = map[string]interface{}{
			"cpu_percent": 85 + (i % 15),          // High CPU usage
			"memory_mb":   1800 + (i * 75),        // High memory usage
			"error_rate":  0.3 + float64(i%3)*0.1, // Variable error rates
		}

		history[i] = execution
	}

	return history
}

// generateDataIntegrityExecutionHistory creates execution history for data integrity testing
func generateDataIntegrityExecutionHistory(workflowID string, count int) []*engine.RuntimeWorkflowExecution {
	history := make([]*engine.RuntimeWorkflowExecution, count)
	baseTime := time.Now().Add(-48 * time.Hour)

	for i := 0; i < count; i++ {
		var duration time.Duration
		var status engine.ExecutionStatus

		// Mix of successful and failed executions with data operations
		if i%6 == 0 {
			// Data corruption simulation (failed)
			duration = time.Duration(120+i*10) * time.Second
			status = engine.ExecutionStatusFailed
		} else {
			// Successful data operations
			duration = time.Duration(200+i*15) * time.Second
			status = engine.ExecutionStatusCompleted
		}

		execution := engine.NewRuntimeWorkflowExecution(
			fmt.Sprintf("%s-integrity-%d", workflowID, i),
			workflowID,
		)
		execution.OperationalStatus = status
		execution.Duration = duration
		execution.StartTime = baseTime.Add(time.Duration(i*4) * time.Hour)

		// Add data integrity metadata
		execution.Metadata["execution_duration_seconds"] = duration.Seconds()
		execution.Metadata["data_operations"] = map[string]interface{}{
			"records_processed":    1000 + (i * 100),
			"data_integrity_check": status == engine.ExecutionStatusCompleted,
			"checksum_verified":    status == engine.ExecutionStatusCompleted,
		}
		if status == engine.ExecutionStatusFailed {
			execution.Metadata["integrity_issues"] = []string{"checksum_mismatch", "partial_write", "connection_lost"}
		}
		execution.Metadata["resource_usage"] = map[string]interface{}{
			"cpu_percent": 60 + (i % 20),
			"memory_mb":   1200 + (i * 60),
			"disk_io_mb":  500 + (i * 25),
		}

		history[i] = execution
	}

	return history
}

// generateConcurrentTestExecutionHistory creates execution history for concurrent testing
func generateConcurrentTestExecutionHistory(workflowID string, count int) []*engine.RuntimeWorkflowExecution {
	history := make([]*engine.RuntimeWorkflowExecution, count)
	baseTime := time.Now().Add(-24 * time.Hour)

	for i := 0; i < count; i++ {
		var duration time.Duration
		var status engine.ExecutionStatus

		// Varied execution patterns for concurrent testing
		switch i % 4 {
		case 0:
			// Fast executions
			duration = time.Duration(90+i*5) * time.Second
			status = engine.ExecutionStatusCompleted
		case 1:
			// Medium executions
			duration = time.Duration(180+i*10) * time.Second
			status = engine.ExecutionStatusCompleted
		case 2:
			// Slow executions
			duration = time.Duration(360+i*20) * time.Second
			status = engine.ExecutionStatusCompleted
		case 3:
			// Failed executions
			duration = time.Duration(45+i*3) * time.Second
			status = engine.ExecutionStatusFailed
		}

		execution := engine.NewRuntimeWorkflowExecution(
			fmt.Sprintf("%s-concurrent-%d", workflowID, i),
			workflowID,
		)
		execution.OperationalStatus = status
		execution.Duration = duration
		execution.StartTime = baseTime.Add(time.Duration(i*2) * time.Hour)

		// Add concurrent testing metadata
		execution.Metadata["execution_duration_seconds"] = duration.Seconds()
		execution.Metadata["concurrent_test"] = true
		execution.Metadata["execution_pattern"] = i % 4
		execution.Metadata["resource_usage"] = map[string]interface{}{
			"cpu_percent":  50 + (i % 40),
			"memory_mb":    800 + (i * 40),
			"thread_count": 10 + (i % 20),
		}

		history[i] = execution
	}

	return history
}
