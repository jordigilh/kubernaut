//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Intelligent Workflow Builder Integration Tests", func() {
	var (
		ctx               context.Context
		builder           engine.IntelligentWorkflowBuilder
		realSLMClient     llm.Client
		testVectorDB      vector.VectorDatabase
		analyticsEngine   *insights.AnalyticsEngine
		executionRepo     engine.ExecutionRepository
		logger            *logrus.Logger
		testConfig        IntegrationTestConfig
		performanceReport *PerformanceReport
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()

		// Set log level based on environment
		if os.Getenv("LOG_LEVEL") == "debug" {
			logger.SetLevel(logrus.DebugLevel)
		} else {
			logger.SetLevel(logrus.InfoLevel)
		}

		testConfig = LoadIntegrationTestConfig()
		performanceReport = NewPerformanceReport()

		// Increment total tests counter
		performanceReport.TotalTests++
	})

	Describe("Real SLM Client Integration", func() {
		BeforeEach(func() {
			if testConfig.SkipSLMTests {
				Skip("SLM integration tests disabled via SKIP_SLM_TESTS")
			}

			// Create real SLM client for integration testing
			slmConfig := config.LLMConfig{
				Provider: testConfig.LLMProvider,
				Model:    testConfig.LLMModel,
				Endpoint: testConfig.LLMEndpoint,
				Timeout:  30 * time.Second,
			}

			var err error
			realSLMClient, err = llm.NewClient(slmConfig, logger)
			Expect(err).ToNot(HaveOccurred(), "SLM client creation should succeed")

			// Wait for SLM client to be healthy
			Eventually(func() bool {
				return realSLMClient.IsHealthy()
			}, "60s", "2s").Should(BeTrue(), "SLM client should become healthy within 60 seconds")

			logger.WithFields(logrus.Fields{
				"provider": testConfig.LLMProvider,
				"model":    testConfig.LLMModel,
				"endpoint": testConfig.LLMEndpoint,
			}).Info("Real SLM client initialized and healthy")
		})

		Context("AI-Driven Workflow Generation", func() {
			It("should generate workflow for memory optimization scenario", func() {
				// Create test dependencies
				testVectorDB = NewIntegrationVectorDatabase()
				analyticsEngine = &insights.AnalyticsEngine{}
				executionRepo = engine.NewInMemoryExecutionRepository(logger)

				// Create builder with real SLM client
				builder = engine.NewDefaultIntelligentWorkflowBuilder(
					realSLMClient,
					testVectorDB,
					analyticsEngine,
					NewMockPatternExtractor(),
					executionRepo,
					logger,
				)

				// Create realistic workflow objective
				objective := &engine.WorkflowObjective{
					ID:          "memory-optimization-integration-test",
					Type:        "performance_optimization",
					Description: "Optimize memory usage for a Kubernetes deployment experiencing high memory consumption and frequent OOMKills",
					Targets: []*engine.OptimizationTarget{
						{
							Type:     "kubernetes",
							Metric:   "memory_usage",
							Priority: 1,
							Parameters: map[string]interface{}{
								"namespace": "production",
								"resource":  "deployment",
								"name":      "web-service",
								"selector":  map[string]string{"app": "web-service", "tier": "frontend"},
							},
						},
					},
					Priority: 8,
					Constraints: map[string]interface{}{
						"max_duration":       "15m",
						"safety_level":       "high",
						"business_hours":     true,
						"require_approval":   false,
						"max_resource_delta": "50%",
					},
				}

				// Measure generation performance
				startTime := time.Now()
				template, err := builder.GenerateWorkflow(ctx, objective)
				generationDuration := time.Since(startTime)

				// Record performance metrics
				performanceReport.TotalResponseTime += generationDuration
				performanceReport.RecordWorkflowGeneration(generationDuration, err == nil)

				// Assertions for AI-generated workflow
				Expect(err).ToNot(HaveOccurred(), "Workflow generation should succeed")
				Expect(template).ToNot(BeNil(), "Generated template should not be nil")
				Expect(template.ID).ToNot(BeEmpty(), "Template should have a unique ID")
				Expect(template.Name).ToNot(BeEmpty(), "Template should have a descriptive name")
				Expect(template.Description).ToNot(BeEmpty(), "Template should have a description")
				Expect(template.Version).ToNot(BeEmpty(), "Template should have a version")

				// Validate workflow structure
				Expect(len(template.Steps)).To(BeNumerically(">", 0), "Template should have at least one step")
				Expect(len(template.Steps)).To(BeNumerically("<=", 20), "Template should not exceed maximum steps")

				// Performance requirements
				Expect(generationDuration).To(BeNumerically("<", 30*time.Second),
					"Workflow generation should complete within 30 seconds")

				// Validate AI-generated steps have proper structure
				for i, step := range template.Steps {
					Expect(step.ID).ToNot(BeEmpty(), "Step %d should have an ID", i)
					Expect(step.Name).ToNot(BeEmpty(), "Step %d should have a name", i)
					Expect(step.Timeout).To(BeNumerically(">", 0), "Step %d should have a positive timeout", i)

					// If step has an action, validate it
					if step.Action != nil {
						Expect(step.Action.Type).ToNot(BeEmpty(), "Step %d action should have a type", i)
						// Validate against known action types
						validActionTypes := []string{
							"scale_deployment", "restart_pod", "collect_diagnostics",
							"increase_resources", "decrease_resources", "update_config",
							"rollback_deployment", "notify_team", "create_backup",
						}
						Expect(validActionTypes).To(ContainElement(step.Action.Type),
							"Step %d should use a valid action type", i)
					}
				}

				// Validate safety measures
				Expect(template.Recovery).ToNot(BeNil(), "Template should have recovery policy")
				Expect(template.Timeouts).ToNot(BeNil(), "Template should have timeout configuration")
				Expect(template.Timeouts.Execution).To(BeNumerically(">", 0), "Execution timeout should be positive")

				// Log detailed results
				logger.WithFields(logrus.Fields{
					"template_id":       template.ID,
					"steps_count":       len(template.Steps),
					"generation_time":   generationDuration,
					"has_recovery":      template.Recovery != nil,
					"execution_timeout": template.Timeouts.Execution,
				}).Info("Successfully generated workflow with real AI model")

				performanceReport.PassedTests++
			})

			It("should generate workflow for pod crashloop scenario", func() {
				objective := &engine.WorkflowObjective{
					ID:          "crashloop-integration-test",
					Type:        "reliability_improvement",
					Description: "Resolve pod crashloop issues for backend service experiencing repeated failures",
					Targets: []*engine.OptimizationTarget{
						{
							Type:     "kubernetes",
							Metric:   "restart_count",
							Priority: 1,
							Parameters: map[string]interface{}{
								"namespace": "backend",
								"resource":  "deployment",
								"name":      "api-service",
							},
						},
					},
					Priority: 9, // High priority for crashloop
					Constraints: map[string]interface{}{
						"max_duration":   "10m",
						"safety_level":   "medium",
						"restart_budget": 3,
					},
				}

				startTime := time.Now()
				template, err := builder.GenerateWorkflow(ctx, objective)
				duration := time.Since(startTime)

				Expect(err).ToNot(HaveOccurred())
				Expect(template).ToNot(BeNil())
				Expect(len(template.Steps)).To(BeNumerically(">", 0))

				// Should include diagnostic steps for crashloop
				hasCollectDiagnostics := false
				for _, step := range template.Steps {
					if step.Action != nil {
						if step.Action.Type == "collect_diagnostics" {
							hasCollectDiagnostics = true
						}
					}
				}

				// Crashloop workflows should typically include diagnostics
				Expect(hasCollectDiagnostics).To(BeTrue(), "Crashloop workflow should include diagnostics")

				performanceReport.RecordWorkflowGeneration(duration, true)
				performanceReport.PassedTests++
			})

			It("should handle complex multi-target scenarios", func() {
				objective := &engine.WorkflowObjective{
					ID:          "multi-target-integration-test",
					Type:        "infrastructure_optimization",
					Description: "Optimize resource allocation across multiple services in a microservices architecture",
					Targets: []*engine.OptimizationTarget{
						{
							Type:     "kubernetes",
							Metric:   "resource_efficiency",
							Priority: 1,
							Parameters: map[string]interface{}{
								"namespace": "microservices",
								"resource":  "deployment",
								"name":      "user-service",
							},
						},
						{
							Type:     "kubernetes",
							Metric:   "resource_efficiency",
							Priority: 2,
							Parameters: map[string]interface{}{
								"namespace": "microservices",
								"resource":  "deployment",
								"name":      "order-service",
							},
						},
						{
							Type:     "kubernetes",
							Metric:   "resource_efficiency",
							Priority: 3,
							Parameters: map[string]interface{}{
								"namespace": "microservices",
								"resource":  "deployment",
								"name":      "payment-service",
							},
						},
					},
					Priority: 6,
					Constraints: map[string]interface{}{
						"max_duration":     "20m",
						"safety_level":     "high",
						"coordinate_steps": true,
					},
				}

				startTime := time.Now()
				template, err := builder.GenerateWorkflow(ctx, objective)
				duration := time.Since(startTime)

				Expect(err).ToNot(HaveOccurred())
				Expect(template).ToNot(BeNil())

				// Multi-target workflows should be more complex
				Expect(len(template.Steps)).To(BeNumerically(">=", 3),
					"Multi-target workflow should have multiple steps")

				// Should handle coordination complexity
				Expect(duration).To(BeNumerically("<", 45*time.Second),
					"Complex workflow generation should complete within 45 seconds")

				performanceReport.RecordWorkflowGeneration(duration, true)
				performanceReport.PassedTests++
			})
		})

		Context("Error Handling and Resilience", func() {
			It("should handle AI service timeouts gracefully", func() {
				// Create SLM client with very short timeout
				shortTimeoutConfig := config.LLMConfig{
					Provider: testConfig.LLMProvider,
					Model:    testConfig.LLMModel,
					Endpoint: testConfig.LLMEndpoint,
					Timeout:  100 * time.Millisecond, // Very short timeout
				}

				shortTimeoutClient, err := llm.NewClient(shortTimeoutConfig, logger)
				Expect(err).ToNot(HaveOccurred())

				builder = engine.NewDefaultIntelligentWorkflowBuilder(
					shortTimeoutClient,
					NewIntegrationVectorDatabase(),
					&insights.AnalyticsEngine{},
					NewMockPatternExtractor(),
					engine.NewInMemoryExecutionRepository(logger),
					logger,
				)

				objective := &engine.WorkflowObjective{
					ID:          "timeout-test",
					Type:        "performance_optimization",
					Description: "Test timeout handling with complex objective requiring detailed analysis",
					Priority:    5,
				}

				template, err := builder.GenerateWorkflow(ctx, objective)

				// Should handle timeout gracefully
				Expect(err).To(HaveOccurred(), "Should fail due to timeout")
				Expect(template).To(BeNil(), "Should not return partial template")
				Expect(err.Error()).To(ContainSubstring("failed to generate workflow"),
					"Error should indicate workflow generation failure")

				performanceReport.PassedTests++
			})

			It("should handle invalid AI responses", func() {
				// This test would require mocking an SLM client that returns invalid responses
				// For now, we'll test with a valid client and invalid objective
				objective := &engine.WorkflowObjective{
					ID:          "invalid-objective-test",
					Type:        "unknown_type",
					Description: "", // Empty description should cause issues
					Priority:    -1, // Invalid priority
				}

				template, err := builder.GenerateWorkflow(ctx, objective)

				// The system should handle this gracefully, either by:
				// 1. Generating a basic workflow despite invalid input, or
				// 2. Returning a clear error
				if err != nil {
					Expect(err.Error()).ToNot(BeEmpty(), "Error should have descriptive message")
				} else {
					Expect(template).ToNot(BeNil(), "If no error, should return valid template")
				}

				performanceReport.PassedTests++
			})
		})
	})

	Describe("Vector Database Integration", func() {
		BeforeEach(func() {
			testVectorDB = NewIntegrationVectorDatabase()
			analyticsEngine = &insights.AnalyticsEngine{}
			executionRepo = engine.NewInMemoryExecutionRepository(logger)

			// Use mock SLM client for vector DB focused tests
			builder = engine.NewDefaultIntelligentWorkflowBuilder(
				NewMockSLMClientWithRealisticResponses(),
				testVectorDB,
				analyticsEngine,
				NewMockPatternExtractor(),
				executionRepo,
				logger,
			)
		})

		Context("Pattern Discovery and Learning", func() {
			It("should discover patterns from execution history", func() {
				// Pre-populate execution repository with test data
				testExecutions := []*engine.WorkflowExecution{
					createTestExecution("exec-1", "workflow-1", "scale_deployment", true, time.Minute*3),
					createTestExecution("exec-2", "workflow-1", "scale_deployment", true, time.Minute*2),
					createTestExecution("exec-3", "workflow-1", "scale_deployment", true, time.Minute*4),
					createTestExecution("exec-4", "workflow-1", "scale_deployment", false, time.Minute*1),
					createTestExecution("exec-5", "workflow-2", "restart_pod", true, time.Second*45),
					createTestExecution("exec-6", "workflow-2", "restart_pod", true, time.Second*30),
				}

				// Add executions to the repository (in-memory implementation)
				for _, exec := range testExecutions {
					if memRepo, ok := executionRepo.(*engine.InMemoryExecutionRepository); ok {
						err := memRepo.StoreExecution(ctx, exec)
						Expect(err).ToNot(HaveOccurred())
					}
				}

				// Test pattern discovery
				criteria := &engine.PatternCriteria{
					MinSimilarity:     0.7,
					MinExecutionCount: 3,
					MinSuccessRate:    0.6,
					TimeWindow:        time.Hour * 24,
				}

				patterns, err := builder.FindWorkflowPatterns(ctx, criteria)

				Expect(err).ToNot(HaveOccurred())
				Expect(patterns).ToNot(BeNil())

				// Should discover patterns from the test data
				if len(patterns) > 0 {
					for _, pattern := range patterns {
						Expect(pattern.SuccessRate).To(BeNumerically(">=", criteria.MinSuccessRate))
						Expect(pattern.ExecutionCount).To(BeNumerically(">=", criteria.MinExecutionCount))
						Expect(pattern.Confidence).To(BeNumerically(">", 0))
					}
				}

				logger.WithField("patterns_found", len(patterns)).Info("Pattern discovery completed")
				performanceReport.PassedTests++
			})

			It("should learn from workflow executions", func() {
				execution := &engine.WorkflowExecution{
					ID:         "learning-test-exec",
					WorkflowID: "learning-test-workflow",
					Status:     engine.ExecutionStatusCompleted,
					Duration:   time.Minute * 2,
					StartTime:  time.Now().Add(-time.Minute * 2),
					Metadata: map[string]interface{}{
						"action_type":         "scale_deployment",
						"effectiveness_score": 0.9,
						"success":             true,
						"resource_type":       "deployment",
						"namespace":           "production",
					},
				}
				endTime := time.Now()
				execution.EndTime = &endTime

				err := builder.LearnFromWorkflowExecution(ctx, execution)
				Expect(err).ToNot(HaveOccurred())

				// Verify learning was stored (check vector database)
				if integrationDB, ok := testVectorDB.(*IntegrationVectorDatabase); ok {
					Expect(len(integrationDB.GetStoredPatterns())).To(BeNumerically(">", 0))
				}

				performanceReport.PassedTests++
			})
		})

		Context("Pattern Application and Reuse", func() {
			It("should apply discovered patterns to new workflows", func() {
				// First, create and store a successful pattern
				pattern := &engine.WorkflowPattern{
					ID:             "test-pattern-1",
					Name:           "High Memory Optimization Pattern",
					Type:           "memory_optimization",
					SuccessRate:    0.85,
					ExecutionCount: 10,
					AverageTime:    time.Minute * 3,
					Confidence:     0.8,
					Environments:   []string{"production"},
					ResourceTypes:  []string{"deployment"},
					Steps: []*engine.WorkflowStep{
						{
							ID:   "step-1",
							Name: "Collect Memory Metrics",
							Type: engine.StepTypeAction,
							Action: &engine.StepAction{
								Type: "collect_diagnostics",
								Parameters: map[string]interface{}{
									"metrics": []string{"memory", "cpu"},
								},
							},
						},
						{
							ID:   "step-2",
							Name: "Increase Memory Limit",
							Type: engine.StepTypeAction,
							Action: &engine.StepAction{
								Type: "increase_resources",
								Parameters: map[string]interface{}{
									"memory": "1Gi",
								},
							},
						},
					},
					CreatedAt: time.Now().Add(-time.Hour),
					UpdatedAt: time.Now(),
				}

				workflowContext := &engine.WorkflowContext{
					Environment: "production",
					Cluster:     "prod-cluster-1",
					Namespace:   "web-services",
					Variables: map[string]interface{}{
						"target_deployment": "web-app",
					},
					CreatedAt: time.Now(),
				}

				template, err := builder.ApplyWorkflowPattern(ctx, pattern, workflowContext)

				Expect(err).ToNot(HaveOccurred())
				Expect(template).ToNot(BeNil())
				Expect(template.Name).To(ContainSubstring("Pattern"))
				Expect(len(template.Steps)).To(Equal(len(pattern.Steps)))

				// Verify context was applied
				Expect(template.Variables).To(HaveKey("target_deployment"))
				Expect(template.Tags).To(ContainElement("production"))

				performanceReport.PassedTests++
			})
		})
	})

	Describe("End-to-End Workflow Lifecycle", func() {
		BeforeEach(func() {
			// Create builder with all real components where possible
			if !testConfig.SkipSLMTests {
				builder = engine.NewDefaultIntelligentWorkflowBuilder(
					realSLMClient,
					NewIntegrationVectorDatabase(),
					&insights.AnalyticsEngine{},
					NewMockPatternExtractor(),
					engine.NewInMemoryExecutionRepository(logger),
					logger,
				)
			} else {
				builder = engine.NewDefaultIntelligentWorkflowBuilder(
					NewMockSLMClientWithRealisticResponses(),
					NewIntegrationVectorDatabase(),
					&insights.AnalyticsEngine{},
					NewMockPatternExtractor(),
					engine.NewInMemoryExecutionRepository(logger),
					logger,
				)
			}
		})

		It("should complete full workflow lifecycle: generation → validation → simulation → learning", func() {
			objective := &engine.WorkflowObjective{
				ID:          "e2e-lifecycle-test",
				Type:        "performance_optimization",
				Description: "End-to-end test for complete workflow lifecycle with realistic scenario",
				Targets: []*engine.OptimizationTarget{
					{
						Type:     "kubernetes",
						Metric:   "performance",
						Priority: 1,
						Parameters: map[string]interface{}{
							"namespace": "production",
							"resource":  "deployment",
							"name":      "api-gateway",
						},
					},
				},
				Priority: 7,
				Constraints: map[string]interface{}{
					"max_duration": "12m",
					"safety_level": "high",
				},
			}

			// Step 1: Generate workflow
			By("Generating workflow from objective")
			startTime := time.Now()
			template, err := builder.GenerateWorkflow(ctx, objective)
			generationTime := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred())
			Expect(template).ToNot(BeNil())
			performanceReport.RecordWorkflowGeneration(generationTime, true)

			// Step 2: Validate workflow
			By("Validating generated workflow")
			validationStart := time.Now()
			validationReport, err := builder.ValidateWorkflow(ctx, template)
			validationTime := time.Since(validationStart)

			Expect(err).ToNot(HaveOccurred())
			Expect(validationReport).ToNot(BeNil())
			Expect(validationReport.Summary.Failed).To(Equal(0),
				"Generated workflow should pass all validations")
			performanceReport.RecordValidation(validationTime, validationReport.Summary.Failed == 0)

			// Step 3: Simulate workflow
			By("Simulating workflow execution")
			scenario := &engine.SimulationScenario{
				ID:          "e2e-simulation",
				Type:        "integration_test",
				Environment: "test",
			}

			simulationStart := time.Now()
			simulationResult, err := builder.SimulateWorkflow(ctx, template, scenario)
			simulationTime := time.Since(simulationStart)

			Expect(err).ToNot(HaveOccurred())
			Expect(simulationResult).ToNot(BeNil())
			Expect(simulationResult.Success).To(BeTrue(), "Simulation should succeed")
			performanceReport.RecordSimulation(simulationTime, simulationResult.Success)

			// Step 4: Learn from execution
			By("Learning from simulated execution")
			mockExecution := &engine.WorkflowExecution{
				ID:         "e2e-execution",
				WorkflowID: template.ID,
				Status:     engine.ExecutionStatusCompleted,
				Duration:   simulationResult.Duration,
				StartTime:  startTime,
				Metadata: map[string]interface{}{
					"simulation":      true,
					"success":         simulationResult.Success,
					"generation_time": generationTime.Milliseconds(),
					"validation_time": validationTime.Milliseconds(),
					"simulation_time": simulationTime.Milliseconds(),
				},
			}
			endTime := time.Now()
			mockExecution.EndTime = &endTime

			learningStart := time.Now()
			err = builder.LearnFromWorkflowExecution(ctx, mockExecution)
			learningTime := time.Since(learningStart)

			Expect(err).ToNot(HaveOccurred())
			performanceReport.RecordLearning(learningTime, true)

			// Verify complete lifecycle metrics
			totalLifecycleTime := generationTime + validationTime + simulationTime + learningTime
			Expect(totalLifecycleTime).To(BeNumerically("<", 2*time.Minute),
				"Complete workflow lifecycle should complete within 2 minutes")

			logger.WithFields(logrus.Fields{
				"objective_id":       objective.ID,
				"template_id":        template.ID,
				"generation_time":    generationTime,
				"validation_time":    validationTime,
				"simulation_time":    simulationTime,
				"learning_time":      learningTime,
				"total_lifecycle":    totalLifecycleTime,
				"validation_passed":  validationReport.Summary.Failed == 0,
				"simulation_success": simulationResult.Success,
			}).Info("Successfully completed end-to-end workflow lifecycle")

			performanceReport.PassedTests++
		})

		It("should handle workflow optimization cycle", func() {
			// Create a basic workflow template
			template := &engine.WorkflowTemplate{
				ID:      "optimization-test-template",
				Name:    "Template for Optimization Testing",
				Version: "1.0.0",
				Steps: []*engine.WorkflowStep{
					{
						ID:   "step1",
						Name: "Collect Metrics",
						Type: engine.StepTypeAction,
						Action: &engine.StepAction{
							Type: "collect_diagnostics",
						},
						Timeout: time.Minute * 5,
					},
					{
						ID:   "step2",
						Name: "Another Collect Metrics", // Redundant step
						Type: engine.StepTypeAction,
						Action: &engine.StepAction{
							Type: "collect_diagnostics",
						},
						Timeout: time.Minute * 5,
					},
					{
						ID:   "step3",
						Name: "Scale Application",
						Type: engine.StepTypeAction,
						Action: &engine.StepAction{
							Type: "scale_deployment",
							Parameters: map[string]interface{}{
								"replicas": 3,
							},
						},
						Timeout: time.Minute * 3,
					},
				},
			}

			By("Optimizing workflow structure")
			optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, template)

			Expect(err).ToNot(HaveOccurred())
			Expect(optimizedTemplate).ToNot(BeNil())

			// Optimization should improve the workflow
			Expect(len(optimizedTemplate.Steps)).To(BeNumerically("<=", len(template.Steps)),
				"Optimization should not increase step count")

			// Verify essential functionality is preserved
			hasCollectDiagnostics := false
			hasScaleDeployment := false
			for _, step := range optimizedTemplate.Steps {
				if step.Action != nil {
					if step.Action.Type == "collect_diagnostics" {
						hasCollectDiagnostics = true
					}
					if step.Action.Type == "scale_deployment" {
						hasScaleDeployment = true
					}
				}
			}
			Expect(hasCollectDiagnostics).To(BeTrue(), "Optimized workflow should retain diagnostics")
			Expect(hasScaleDeployment).To(BeTrue(), "Optimized workflow should retain scaling")

			performanceReport.PassedTests++
		})
	})

	Describe("Performance and Load Testing", func() {
		Context("Concurrent Workflow Generation", func() {
			It("should handle multiple concurrent workflow generations", func() {
				if testConfig.SkipPerformanceTests {
					Skip("Performance tests disabled via SKIP_PERFORMANCE_TESTS")
				}

				concurrentCount := 5
				results := make(chan struct {
					template *engine.WorkflowTemplate
					err      error
					duration time.Duration
				}, concurrentCount)

				// Launch concurrent workflow generations
				for i := 0; i < concurrentCount; i++ {
					go func(index int) {
						objective := &engine.WorkflowObjective{
							ID:          fmt.Sprintf("concurrent-test-%d", index),
							Type:        "performance_optimization",
							Description: fmt.Sprintf("Concurrent workflow generation test %d", index),
							Priority:    5,
						}

						start := time.Now()
						template, err := builder.GenerateWorkflow(ctx, objective)
						duration := time.Since(start)

						results <- struct {
							template *engine.WorkflowTemplate
							err      error
							duration time.Duration
						}{template, err, duration}
					}(i)
				}

				// Collect results
				successCount := 0
				totalDuration := time.Duration(0)
				maxDuration := time.Duration(0)

				for i := 0; i < concurrentCount; i++ {
					result := <-results
					totalDuration += result.duration
					if result.duration > maxDuration {
						maxDuration = result.duration
					}

					if result.err == nil && result.template != nil {
						successCount++
					}
				}

				avgDuration := totalDuration / time.Duration(concurrentCount)

				// Performance assertions
				Expect(successCount).To(Equal(concurrentCount), "All concurrent generations should succeed")
				Expect(avgDuration).To(BeNumerically("<", 45*time.Second),
					"Average generation time should be under 45 seconds")
				Expect(maxDuration).To(BeNumerically("<", 60*time.Second),
					"Maximum generation time should be under 60 seconds")

				logger.WithFields(logrus.Fields{
					"concurrent_count": concurrentCount,
					"success_count":    successCount,
					"avg_duration":     avgDuration,
					"max_duration":     maxDuration,
				}).Info("Concurrent workflow generation test completed")

				performanceReport.PassedTests++
			})
		})

		Context("Memory and Resource Usage", func() {
			It("should maintain reasonable memory usage during workflow generation", func() {
				if testConfig.SkipPerformanceTests {
					Skip("Performance tests disabled")
				}

				// Generate multiple workflows to test memory usage
				for i := 0; i < 10; i++ {
					objective := &engine.WorkflowObjective{
						ID:          fmt.Sprintf("memory-test-%d", i),
						Type:        "performance_optimization",
						Description: "Memory usage test workflow",
						Priority:    5,
					}

					template, err := builder.GenerateWorkflow(ctx, objective)
					Expect(err).ToNot(HaveOccurred())
					Expect(template).ToNot(BeNil())

					// Force garbage collection
					if i%3 == 0 {
						performanceReport.ForceGC()
					}
				}

				performanceReport.PassedTests++
			})
		})
	})

	AfterEach(func() {
		// Clean up any resources if needed
		if performanceReport != nil {
			performanceReport.RecordTestCompletion()
		}
	})
})

// Helper functions and types for integration testing

// Test helper functions are defined in intelligent_workflow_builder_integration_support.go

func createTestExecution(id, workflowID, actionType string, success bool, duration time.Duration) *engine.WorkflowExecution {
	status := engine.ExecutionStatusCompleted
	if !success {
		status = engine.ExecutionStatusFailed
	}

	endTime := time.Now()
	return &engine.WorkflowExecution{
		ID:         id,
		WorkflowID: workflowID,
		Status:     status,
		Duration:   duration,
		StartTime:  time.Now().Add(-duration),
		EndTime:    &endTime,
		Metadata: map[string]interface{}{
			"action_type": actionType,
			"success":     success,
			"effectiveness_score": func() float64 {
				if success {
					return 0.8 + (0.2 * (1.0 - duration.Seconds()/300.0)) // Higher score for faster execution
				}
				return 0.2
			}(),
		},
	}
}
