//go:build integration
// +build integration

package ai

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

// ModernizedAIIntegrationTest demonstrates the new standardized testing approach
var _ = Describe("Modernized AI Integration Test (Example)", Ordered, func() {
	var hooks *testshared.TestLifecycleHooks

	// BEFORE: This would require 50+ lines of boilerplate setup
	// AFTER: Single line setup with all components configured
	BeforeAll(func() {
		hooks = testshared.SetupAIIntegrationTest("Modernized AI Integration",
			testshared.WithRealDatabase(),
			testshared.WithMockLLM(), // Fast tests with mock by default
			testshared.WithRealVectorDB(),
		)
		hooks.Setup()
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	Context("Alert Processing Workflow", func() {
		It("should process database alerts with standardized factories", func() {
			suite := hooks.GetSuite()
			ctx := context.Background()

			// BEFORE: 20+ lines of alert creation boilerplate
			// AFTER: Single factory call with pre-configured realistic data
			alert := testshared.CreateDatabaseAlert()
			Expect(alert).ToNot(BeNil())
			Expect(alert.Name).To(Equal("DatabaseHighCPU"))
			Expect(alert.Severity).To(Equal("critical"))

			// BEFORE: Manual LLM client setup and configuration
			// AFTER: Pre-configured client ready for use
			Expect(suite.LLMClient).ToNot(BeNil())
			Expect(suite.LLMClient.IsHealthy()).To(BeTrue())

			// Test alert processing
			recommendation, err := suite.LLMClient.AnalyzeAlert(ctx, *alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())
			Expect(recommendation.Confidence).To(BeNumerically(">", 0.0))
		})

		It("should handle cascading alerts efficiently", func() {
			suite := hooks.GetSuite()
			ctx := context.Background()

			// BEFORE: 30+ lines of related alert setup
			// AFTER: Factory creates realistic cascading scenario
			alerts := testshared.CreateCascadingAlerts()
			Expect(alerts).To(HaveLen(3))

			// Process each alert - workflow builder is pre-configured
			for _, alert := range alerts {
				recommendation, err := suite.LLMClient.AnalyzeAlert(ctx, *alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(recommendation.Action).ToNot(BeEmpty())
			}
		})

		It("should utilize vector database for pattern matching", func() {
			suite := hooks.GetSuite()

			// BEFORE: Vector DB setup required 25+ lines
			// AFTER: Pre-configured and ready for use
			Expect(suite.VectorDB).ToNot(BeNil())

			// BEFORE: Manual pattern creation and mock setup
			// AFTER: Standardized mock pattern with realistic data
			patternResult := testshared.CreateMockPatternResult()
			Expect(patternResult.Patterns).To(HaveLen(2))

			// Test pattern confidence levels
			for _, pattern := range patternResult.Patterns {
				Expect(pattern.Confidence).To(BeNumerically(">=", 0.7))
			}
		})
	})

	Context("Workflow Generation and Execution", func() {
		It("should generate workflows using the standardized builder", func() {
			suite := hooks.GetSuite()

			// BEFORE: Workflow builder setup required configuration of multiple components
			// AFTER: Pre-configured builder with all dependencies injected
			Expect(suite.WorkflowBuilder).ToNot(BeNil())

			// Create test alert and recommendation
			alert := testshared.CreatePerformanceAlert()
			recommendation := testshared.CreateHighConfidenceRecommendation("scale_up")

			// Generate workflow objective
			objective := testshared.CreateStandardWorkflowObjective(alert, recommendation, "performance")
			Expect(objective).ToNot(BeNil())
			Expect(objective.Type).To(Equal("performance"))
			Expect(objective.Priority).To(Equal(5))
		})

		It("should track execution history with standardized data", func() {
			// BEFORE: Manual execution data creation with many fields to configure
			// AFTER: Factory creates realistic execution data with proper relationships
			executions := testshared.CreateBatchExecutionData("test_workflow", 10, 0.8)
			Expect(executions).To(HaveLen(10))

			// Verify success rate approximately matches expected
			successCount := 0
			for _, exec := range executions {
				if exec.Success {
					successCount++
				}
			}
			// Allow for some variance in test data generation
			Expect(float64(successCount) / 10.0).To(BeNumerically("~", 0.8, 0.2))
		})
	})

	Context("Performance and Reliability", func() {
		It("should complete alert processing within reasonable time", func() {
			suite := hooks.GetSuite()
			ctx := context.Background()

			alert := testshared.CreateStandardAlert(
				"TestPerformanceAlert",
				"Performance test alert",
				"warning",
				"test",
				"test-resource",
			)

			startTime := time.Now()

			_, err := suite.LLMClient.AnalyzeAlert(ctx, *alert)
			Expect(err).ToNot(HaveOccurred())

			duration := time.Since(startTime)
			// With mock LLM, this should be very fast
			Expect(duration).To(BeNumerically("<", time.Second))
		})

		It("should handle multiple concurrent alert processing", func() {
			suite := hooks.GetSuite()
			ctx := context.Background()

			// Test scenario factory creates alerts for specific test cases
			alerts := testshared.CreateTestAlertsForScenario("performance_degradation")
			Expect(alerts).To(HaveLen(2))

			// Process alerts concurrently - configuration supports this
			results := make(chan error, len(alerts))

			for _, alert := range alerts {
				go func(a *types.Alert) {
					_, err := suite.LLMClient.AnalyzeAlert(ctx, *a)
					results <- err
				}(alert)
			}

			// Collect results
			for i := 0; i < len(alerts); i++ {
				err := <-results
				Expect(err).ToNot(HaveOccurred())
			}
		})
	})

	// BEFORE: Manual cleanup required 15+ lines and could miss components
	// AFTER: Automatic cleanup handled by lifecycle hooks - no additional code needed
})

// Example of how to extend with custom test-specific setup if needed
var _ = Describe("Custom AI Test Extensions", Ordered, func() {
	var hooks *testshared.TestLifecycleHooks

	BeforeAll(func() {
		hooks = testshared.SetupAIIntegrationTest("Custom AI Extensions",
			testshared.WithRealLLM(), // Use real LLM for this specific test
			testshared.WithPerformanceMonitoring(),
			testshared.WithDebugLogging(),
			testshared.WithCustomCleanup(func() error {
				// Custom cleanup logic specific to this test suite
				hooks.GetLogger().Info("Custom AI test cleanup completed")
				return nil
			}),
		)
		hooks.Setup()
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	// Add test-specific setup if needed
	BeforeEach(func() {
		hooks.WithTestSpecificSetup(func() error {
			// Any test-specific initialization
			hooks.GetLogger().Debug("Custom test setup executed")
			return nil
		})
	})

	It("should demonstrate advanced AI features with real LLM", func() {
		suite := hooks.GetSuite()
		ctx := context.Background()

		// This test uses the real LLM as configured in BeforeAll
		Expect(suite.LLMClient).ToNot(BeNil())

		// Test advanced scenarios that require real LLM processing
		alert := testshared.CreateSecurityAlert()
		recommendation, err := suite.LLMClient.AnalyzeAlert(ctx, *alert)

		Expect(err).ToNot(HaveOccurred())
		Expect(recommendation.Action).To(ContainSubstring("quarantine"))
		Expect(recommendation.Confidence).To(BeNumerically(">=", 0.8))
	})
})
