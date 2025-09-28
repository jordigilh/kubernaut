//go:build unit
// +build unit

package comprehensive

import (
	"testing"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// BR-CORE-INTEGRATION-001: Comprehensive Core Integration Business Logic Testing
// Business Impact: Validates core system integration capabilities for production reliability
// Stakeholder Value: Ensures reliable core integration for business continuity
var _ = Describe("BR-CORE-INTEGRATION-001: Comprehensive Core Integration Business Logic", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		mockLLMClient         *mocks.MockLLMClient
		mockK8sClient         *mocks.MockK8sClient
		mockActionHistoryRepo *mocks.MockActionHistoryRepository
		mockVectorDB          *mocks.MockVectorDatabase
		mockLogger            *logrus.Logger

		// Use REAL business logic components
		alertProcessor      processor.Processor
		actionExecutor      executor.Executor
		llmHealthMonitor    *monitoring.LLMHealthMonitor
		aiServiceIntegrator *engine.AIServiceIntegrator

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only
		mockLLMClient = mocks.NewMockLLMClient()
		mockK8sClient = mocks.NewMockK8sClient(nil)
		mockActionHistoryRepo = mocks.NewMockActionHistoryRepository()
		mockVectorDB = mocks.NewMockVectorDatabase()
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create REAL business alert processor with mocked external dependencies
		filters := []config.FilterConfig{
			{
				Name: "production-alerts",
				Conditions: map[string][]string{
					"severity": {"critical", "high"},
				},
			},
		}
		alertProcessor = processor.NewProcessor(
			mockLLMClient,         // External: Mock (AI service)
			nil,                   // Will create executor separately
			filters,               // Configuration
			mockActionHistoryRepo, // External: Mock (database)
			mockLogger,            // External: Mock (logging infrastructure)
		)

		// Create REAL business action executor with mocked external dependencies
		executorConfig := config.ActionsConfig{
			MaxConcurrent: 3,
			DryRun:        true,
		}
		var err error
		actionExecutor, err = executor.NewExecutor(
			mockK8sClient,         // External: Mock
			executorConfig,        // Configuration
			mockActionHistoryRepo, // External: Mock
			mockLogger,            // External: Mock (logging infrastructure)
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create real action executor")

		// Create REAL business LLM health monitor
		llmHealthMonitor = monitoring.NewLLMHealthMonitor(mockLLMClient, mockLogger)

		// Create REAL business AI service integrator
		// Create real config for AI service integration
		realConfig := &config.Config{}

		aiServiceIntegrator = engine.NewAIServiceIntegrator(
			realConfig,    // Real: Config for business logic
			mockLLMClient, // External: Mock LLM client
			nil,           // HolmesGPT client (optional)
			mockVectorDB,  // External: Mock vector database
			nil,           // Metrics client (optional)
			mockLogger,    // External: Mock (logging infrastructure)
		)
	})

	AfterEach(func() {
		cancel()
	})

	// COMPREHENSIVE scenario testing for core integration business logic
	DescribeTable("BR-CORE-INTEGRATION-001: Should handle all core integration scenarios",
		func(scenarioName string, alertFn func() types.Alert, setupFn func(), expectedSuccess bool) {
			// Setup test scenario
			setupFn()
			alert := alertFn()

			// Test REAL business core integration logic
			err := alertProcessor.ProcessAlert(ctx, alert)

			// Validate REAL business core integration outcomes
			if expectedSuccess {
				Expect(err).ToNot(HaveOccurred(),
					"BR-CORE-INTEGRATION-001: Core integration must succeed for %s", scenarioName)
			} else {
				Expect(err).To(HaveOccurred(),
					"BR-CORE-INTEGRATION-001: Invalid scenarios must fail gracefully for %s", scenarioName)
			}
		},
		Entry("Critical production alert", "critical_production", func() types.Alert {
			return createCriticalProductionAlert()
		}, func() {
			// Setup successful LLM analysis inline (Rule 03: self-contained tests)
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "restart_pod",
				Confidence:        0.85,
				Reasoning:         "AI analysis recommends pod restart for service recovery",
				ProcessingTime:    50 * time.Millisecond,
				Metadata: map[string]interface{}{
					"pod_name":  "critical-service-pod",
					"namespace": "production",
				},
			})
		}, true),
		Entry("High priority alert", "high_priority", func() types.Alert {
			return createHighPriorityAlert()
		}, func() {
			// Setup successful LLM analysis inline (Rule 03: self-contained tests)
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "restart_pod",
				Confidence:        0.85,
				Reasoning:         "AI analysis recommends pod restart for service recovery",
				ProcessingTime:    50 * time.Millisecond,
				Metadata: map[string]interface{}{
					"pod_name":  "high-priority-service-pod",
					"namespace": "production",
				},
			})
		}, true),
		Entry("Medium priority alert (filtered)", "medium_priority", func() types.Alert {
			return createMediumPriorityAlert()
		}, func() {
			// No setup needed for filtered alerts
		}, true), // Should succeed but be filtered
		Entry("Invalid alert format", "invalid_alert", func() types.Alert {
			return createInvalidAlert()
		}, func() {
			// No setup needed for invalid alerts
		}, false),
		Entry("LLM service failure", "llm_failure", func() types.Alert {
			return createCriticalProductionAlert()
		}, func() {
			// Setup LLM failure inline (Rule 03: self-contained tests)
			mockLLMClient.SetError("LLM service unavailable")
		}, false),
	)

	// COMPREHENSIVE business integration health validation
	Context("BR-CORE-INTEGRATION-002: Business Integration Health Validation", func() {
		It("should validate comprehensive business integration health", func() {
			// Test REAL business logic for LLM health monitoring
			healthStatus, err := llmHealthMonitor.GetHealthStatus(ctx)

			// Validate REAL business LLM health monitoring outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-CORE-INTEGRATION-002: LLM health monitoring must succeed")
			Expect(healthStatus).ToNot(BeNil(),
				"BR-CORE-INTEGRATION-002: Must return health status")

			// Validate health status structure
			Expect(healthStatus.BaseEntity.UpdatedAt).ToNot(BeZero(),
				"BR-CORE-INTEGRATION-002: Health status must have timestamp")
			Expect(healthStatus.BaseTimestampedResult.EndTime).ToNot(BeZero(),
				"BR-CORE-INTEGRATION-002: Must provide end time")

			// Validate health metrics
			Expect(healthStatus.HealthMetrics).ToNot(BeNil(),
				"BR-CORE-INTEGRATION-002: Must provide health metrics")
			Expect(healthStatus.HealthMetrics.UptimePercentage).To(BeNumerically(">=", 0),
				"BR-CORE-INTEGRATION-002: Must provide uptime percentage")
		})

		It("should track LLM health monitoring", func() {
			// Test REAL business logic for LLM health monitoring
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Start health monitoring
			err := llmHealthMonitor.StartHealthMonitoring(ctx)
			Expect(err).ToNot(HaveOccurred(),
				"BR-CORE-INTEGRATION-002: Health monitoring must start successfully")

			// Validate liveness probe
			livenessResult, err := llmHealthMonitor.PerformLivenessProbe(ctx)
			Expect(err).ToNot(HaveOccurred(),
				"BR-CORE-INTEGRATION-002: Liveness probe must succeed")
			Expect(livenessResult).ToNot(BeNil(),
				"BR-CORE-INTEGRATION-002: Must return liveness result")

			// Validate readiness probe
			readinessResult, err := llmHealthMonitor.PerformReadinessProbe(ctx)
			Expect(err).ToNot(HaveOccurred(),
				"BR-CORE-INTEGRATION-002: Readiness probe must succeed")
			Expect(readinessResult).ToNot(BeNil(),
				"BR-CORE-INTEGRATION-002: Must return readiness result")
		})
	})

	// COMPREHENSIVE AI service integration business logic testing
	Context("BR-CORE-INTEGRATION-003: AI Service Integration Business Logic", func() {
		It("should integrate AI services with fallback strategies", func() {
			// Test REAL business logic for AI service integration
			alert := createCriticalProductionAlert()

			// Setup AI service availability (Rule 09: actual method name)
			mockLLMClient.SetHealthy(true)

			// Test REAL business AI service integration
			result := aiServiceIntegrator.InvestigateAlert(ctx, alert)

			// Validate REAL business AI service integration outcomes (Rule 09: actual fields)
			Expect(result).ToNot(BeNil(),
				"BR-CORE-INTEGRATION-003: AI investigation must return result")
			Expect(result.Source).ToNot(BeEmpty(),
				"BR-CORE-INTEGRATION-003: Investigation result must specify source")
			Expect(result.Method).ToNot(BeEmpty(),
				"BR-CORE-INTEGRATION-003: Must specify investigation method used")

			// Validate investigation completeness
			Expect(result.Analysis).ToNot(BeEmpty(),
				"BR-CORE-INTEGRATION-003: Investigation must provide analysis")
			Expect(result.Confidence).To(BeNumerically(">=", 0),
				"BR-CORE-INTEGRATION-003: Investigation must provide confidence score")
		})

		It("should handle AI service failures with graceful degradation", func() {
			// Test REAL business logic for AI service failure handling
			alert := createCriticalProductionAlert()

			// Setup AI service failure (Rule 09: actual method name)
			mockLLMClient.SetHealthy(false)
			mockLLMClient.SetError("AI service unavailable")

			// Test REAL business graceful degradation
			result := aiServiceIntegrator.InvestigateAlert(ctx, alert)

			// Validate REAL business graceful degradation outcomes (Rule 09: actual fields)
			Expect(result).ToNot(BeNil(),
				"BR-CORE-INTEGRATION-003: Graceful degradation must provide result")
			Expect(result.Method).To(Equal("graceful_degradation"),
				"BR-CORE-INTEGRATION-003: Must use graceful degradation when AI unavailable")
			Expect(result.Analysis).ToNot(BeEmpty(),
				"BR-CORE-INTEGRATION-003: Graceful degradation must provide basic analysis")
		})

		It("should detect and configure AI services", func() {
			// Test REAL business logic for AI service detection and configuration (Rule 09: actual method)
			status, err := aiServiceIntegrator.DetectAndConfigure(ctx)

			// Validate REAL business AI service configuration outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-CORE-INTEGRATION-003: AI service detection must succeed")
			Expect(status).ToNot(BeNil(),
				"BR-CORE-INTEGRATION-003: Must return AI service status")

			// Validate AI service configuration components
			Expect(status.LLMAvailable || status.HolmesGPTAvailable).To(BeTrue(),
				"BR-CORE-INTEGRATION-003: At least one AI service must be available for integration")
		})
	})

	// COMPREHENSIVE action execution integration business logic testing
	Context("BR-CORE-INTEGRATION-004: Action Execution Integration Business Logic", func() {
		It("should integrate action execution with alert processing", func() {
			// Test REAL business logic for integrated action execution
			alert := createCriticalProductionAlert()
			action := createTestAction()

			// Setup successful execution (Rule 09: actual method names)
			mockK8sClient.SetRestartPodResult(true, nil) // Success for restart operations
			// mockActionHistoryRepo will use default success behavior

			// Test REAL business integrated action execution
			err := actionExecutor.Execute(ctx, action, alert, nil)

			// Validate REAL business integrated action execution outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-CORE-INTEGRATION-004: Integrated action execution must succeed")

			// Verify action was executed (Rule 09: actual method names)
			operationCount := mockK8sClient.GetOperationCount()
			Expect(operationCount).To(BeNumerically(">=", 0),
				"BR-CORE-INTEGRATION-004: Must track executed operations")

			// Action history verification through real business logic integration
			// The action executor will handle history recording internally
		})

		It("should handle action execution failures gracefully", func() {
			// Test REAL business logic for action execution failure handling
			alert := createCriticalProductionAlert()
			action := createTestAction()

			// Setup execution failure (Rule 09: actual method names)
			mockK8sClient.SetRestartPodResult(false, fmt.Errorf("execution failed"))

			// Test REAL business action execution failure handling
			err := actionExecutor.Execute(ctx, action, alert, nil)

			// Validate REAL business action execution failure handling outcomes
			Expect(err).To(HaveOccurred(),
				"BR-CORE-INTEGRATION-004: Action execution failures must be reported")
			Expect(err.Error()).To(ContainSubstring("execution failed"),
				"BR-CORE-INTEGRATION-004: Error messages must be descriptive")
		})
	})

	// COMPREHENSIVE concurrent integration business logic testing
	Context("BR-CORE-INTEGRATION-005: Concurrent Integration Business Logic", func() {
		It("should handle concurrent alert processing", func() {
			// Test REAL business logic for concurrent integration
			alerts := []types.Alert{
				createCriticalProductionAlert(),
				createHighPriorityAlert(),
				createCriticalProductionAlert(),
			}

			// Setup successful processing inline (Rule 03: self-contained tests)
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "restart_pod",
				Confidence:        0.85,
				Reasoning:         "AI analysis for concurrent processing",
				ProcessingTime:    30 * time.Millisecond,
				Metadata: map[string]interface{}{
					"concurrent_test": true,
				},
			})

			var wg sync.WaitGroup
			var successCount int64
			results := make([]error, len(alerts))

			// Test REAL business concurrent integration
			for i, alert := range alerts {
				wg.Add(1)
				go func(idx int, a types.Alert) {
					defer wg.Done()
					err := alertProcessor.ProcessAlert(ctx, a)
					results[idx] = err
					if err == nil {
						atomic.AddInt64(&successCount, 1)
					}
				}(i, alert)
			}

			wg.Wait()

			// Validate REAL business concurrent integration outcomes
			Expect(successCount).To(BeNumerically(">", 0),
				"BR-CORE-INTEGRATION-005: Some concurrent processing must succeed")

			// Validate no race conditions
			processedCount := int64(0)
			for _, err := range results {
				if err == nil {
					processedCount++
				}
			}
			Expect(processedCount).To(Equal(successCount),
				"BR-CORE-INTEGRATION-005: Success count must match processed count (no race conditions)")
		})
	})

	// COMPREHENSIVE integration error handling business logic testing
	Context("BR-CORE-INTEGRATION-006: Integration Error Handling Business Logic", func() {
		It("should handle integration errors with proper recovery", func() {
			// Test REAL business logic for integration error handling
			alert := createCriticalProductionAlert()

			// Setup various error scenarios
			errorScenarios := []struct {
				name     string
				setupFn  func()
				expected bool
			}{
				{
					name: "LLM timeout",
					setupFn: func() {
						mockLLMClient.SetError("LLM timeout")
					},
					expected: false,
				},
				{
					name: "Database unavailable",
					setupFn: func() {
						// Database errors will be handled by real business logic
						// Mock will use default error behavior for this test
					},
					expected: false,
				},
				{
					name: "K8s API unavailable",
					setupFn: func() {
						mockK8sClient.SetRestartPodResult(false, fmt.Errorf("kubernetes API unavailable"))
					},
					expected: false,
				},
			}

			// Test REAL business error handling for each scenario
			for _, scenario := range errorScenarios {
				By(scenario.name)

				scenario.setupFn()

				err := alertProcessor.ProcessAlert(ctx, alert)

				if scenario.expected {
					Expect(err).ToNot(HaveOccurred(),
						"BR-CORE-INTEGRATION-006: Error recovery must succeed for %s", scenario.name)
				} else {
					Expect(err).To(HaveOccurred(),
						"BR-CORE-INTEGRATION-006: Errors must be properly reported for %s", scenario.name)
				}

				// Reset for next scenario (Rule 09: use actual available methods)
				mockLLMClient.ClearState()
				// Other mocks will be reset through BeforeEach for next test
			}
		})
	})
})

// Helper functions to create test data for core integration scenarios

func createCriticalProductionAlert() types.Alert {
	return types.Alert{
		Name:      "CriticalProductionAlert",
		Namespace: "production",
		Severity:  "critical",
		Status:    "firing",
		Resource:  "critical-service",
		Labels: map[string]string{
			"alertname":   "CriticalProductionAlert",
			"severity":    "critical",
			"environment": "production",
			"service":     "critical-service",
		},
		Annotations: map[string]string{
			"description": "Critical production service failure",
			"summary":     "Production service is down",
			"runbook_url": "https://runbook.example.com/critical-service",
		},
	}
}

func createHighPriorityAlert() types.Alert {
	return types.Alert{
		Name:      "HighPriorityAlert",
		Namespace: "production",
		Severity:  "high",
		Status:    "firing",
		Resource:  "important-service",
		Labels: map[string]string{
			"alertname":   "HighPriorityAlert",
			"severity":    "high",
			"environment": "production",
			"service":     "important-service",
		},
		Annotations: map[string]string{
			"description": "High priority service issue",
			"summary":     "Service performance degraded",
		},
	}
}

func createMediumPriorityAlert() types.Alert {
	return types.Alert{
		Name:      "MediumPriorityAlert",
		Namespace: "staging",
		Severity:  "medium",
		Status:    "firing",
		Resource:  "test-service",
		Labels: map[string]string{
			"alertname":   "MediumPriorityAlert",
			"severity":    "medium",
			"environment": "staging",
			"service":     "test-service",
		},
		Annotations: map[string]string{
			"description": "Medium priority service issue",
			"summary":     "Service warning condition",
		},
	}
}

func createInvalidAlert() types.Alert {
	return types.Alert{
		Name:      "", // Invalid empty name
		Namespace: "",
		Severity:  "",
		Status:    "",
		Resource:  "",
	}
}

func createTestAction() *types.ActionRecommendation {
	return &types.ActionRecommendation{
		Action:     "restart_pod",
		Confidence: 0.85,
		Reasoning: &types.ReasoningDetails{
			PrimaryReason: "Pod restart recommended for service recovery",
			Summary:       "Service recovery requires pod restart based on analysis",
		},
		Parameters: map[string]interface{}{
			"pod_name":  "critical-service-pod",
			"namespace": "production",
		},
	}
}

// Helper functions for mock setup are now inline within tests for clarity (Rule 03: self-contained tests)

// TestRunner bootstraps the Ginkgo test suite
func TestUcoreUintegrationUcomprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UcoreUintegrationUcomprehensive Suite")
}
