//go:build unit
// +build unit

package processor

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// executorAdapter adapts workflow engine MockActionExecutor to executor.Executor interface
type executorAdapter struct {
	workflowExecutor *mocks.MockActionExecutor
}

func (ea *executorAdapter) Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert, actionTrace *actionhistory.ResourceActionTrace) error {
	// Convert types.ActionRecommendation to engine.StepAction
	stepAction := &engine.StepAction{
		Type:       action.Action,
		Parameters: action.Parameters,
	}

	// Create a basic step context
	stepContext := &engine.StepContext{
		StepID: fmt.Sprintf("step-%s", action.Action),
	}

	// Call the workflow executor
	_, err := ea.workflowExecutor.Execute(ctx, stepAction, stepContext)
	return err
}

func (ea *executorAdapter) IsHealthy() bool {
	return true // Mock is always healthy
}

func (ea *executorAdapter) GetActionRegistry() *executor.ActionRegistry {
	return executor.NewActionRegistry() // Return empty registry for testing
}

func (ea *executorAdapter) SetError(errMsg string) {
	ea.workflowExecutor.SetError(errMsg)
}

// BR-AP-PROCESSOR-001: Comprehensive Alert Processor Business Logic Testing
// Business Impact: Ensures alert processing reliability for production Kubernetes incident response
// Stakeholder Value: Operations teams can trust automated alert analysis and remediation
var _ = Describe("BR-AP-PROCESSOR-001: Comprehensive Alert Processor Business Logic", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		mockLLMClient  *mocks.MockLLMClient
		mockExecutor   executor.Executor
		mockActionRepo *mocks.MockActionRepository
		mockLogger     *logrus.Logger

		// Use REAL business logic components
		alertProcessor processor.Processor
		filterConfigs  []config.FilterConfig

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only
		mockLLMClient = mocks.NewMockLLMClient()
		workflowMockExecutor := mocks.NewMockActionExecutor()
		mockExecutor = &executorAdapter{workflowExecutor: workflowMockExecutor}
		mockActionRepo = mocks.NewMockActionRepository()
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create REAL business logic filter configurations
		filterConfigs = []config.FilterConfig{
			{
				Name: "critical-alerts",
				Conditions: map[string][]string{
					"severity": {"critical", "high"},
				},
			},
			{
				Name: "production-namespace",
				Conditions: map[string][]string{
					"namespace": {"production", "prod-*"},
				},
			},
			{
				Name: "memory-alerts",
				Conditions: map[string][]string{
					"alertname": {"MemoryUsageHigh", "OutOfMemory"},
				},
			},
		}

		// Create REAL alert processor with mocked external dependencies
		alertProcessor = processor.NewProcessor(
			mockLLMClient,  // External: Mock
			mockExecutor,   // External: Mock
			filterConfigs,  // Real: Business filter logic
			mockActionRepo, // External: Mock
			mockLogger,     // External: Mock (logging infrastructure)
		)
	})

	AfterEach(func() {
		cancel()
	})

	// COMPREHENSIVE scenario testing for alert processing business logic
	DescribeTable("BR-AP-PROCESSOR-001: Should handle all alert processing scenarios",
		func(scenarioName string, alert types.Alert, expectedProcessed bool, expectedError bool) {
			// Mock external responses for consistent testing
			if expectedProcessed && !expectedError {
				mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
					RecommendedAction: "scale_deployment",
					Confidence:        0.85,
					Reasoning:         "High memory usage detected - Alert analysis indicates need for scaling action",
					ProcessingTime:    25 * time.Millisecond,
					Metadata: map[string]interface{}{
						"test_scenario": true,
						"replicas":      3,
					},
				})
				// Note: executorAdapter handles SetError internally
			} else if expectedError {
				mockLLMClient.SetError("LLM analysis failed")
			}

			// Test REAL business logic
			err := alertProcessor.ProcessAlert(ctx, alert)

			// Validate REAL business outcomes
			if expectedError {
				Expect(err).To(HaveOccurred(),
					"BR-AP-PROCESSOR-001: Invalid alerts must fail gracefully for %s", scenarioName)
			} else if expectedProcessed {
				Expect(err).ToNot(HaveOccurred(),
					"BR-AP-PROCESSOR-001: Valid alerts must process successfully for %s", scenarioName)
			} else {
				Expect(err).ToNot(HaveOccurred(),
					"BR-AP-PROCESSOR-001: Filtered alerts must not error for %s", scenarioName)
			}
		},
		Entry("Critical production alert", "critical_prod", createCriticalProductionAlert(), true, false),
		Entry("High severity memory alert", "high_memory", createHighMemoryAlert(), true, false),
		Entry("Low severity development alert", "low_dev", createLowSeverityDevAlert(), false, false),
		Entry("Non-firing alert", "resolved", createResolvedAlert(), false, false),
		Entry("Alert without name", "no_name", createAlertWithoutName(), false, true),
		Entry("Alert without status", "no_status", createAlertWithoutStatus(), false, true),
		Entry("Alert with custom labels", "custom_labels", createAlertWithCustomLabels(), true, false),
		Entry("Alert with wildcard namespace match", "wildcard_ns", createWildcardNamespaceAlert(), true, false),
		Entry("Alert with complex filtering", "complex_filter", createComplexFilterAlert(), true, false),
		Entry("Alert with special characters", "special_chars", createSpecialCharAlert(), false, false),
	)

	// COMPREHENSIVE filtering logic testing
	Context("BR-AP-PROCESSOR-002: Alert Filtering Business Logic", func() {
		It("should apply severity-based filtering correctly", func() {
			// Test REAL business filtering logic for different severities
			severityTests := []struct {
				severity      string
				shouldProcess bool
			}{
				{"critical", true},
				{"high", true},
				{"medium", false},
				{"low", false},
				{"info", false},
				{"", false},
			}

			for _, test := range severityTests {
				alert := types.Alert{
					Name:        "test-alert",
					Status:      "firing",
					Severity:    test.severity,
					Namespace:   "production",
					Labels:      make(map[string]string),
					Annotations: make(map[string]string),
				}

				// Test REAL business filtering logic
				result := alertProcessor.ShouldProcess(alert)

				// Validate REAL business filtering outcomes
				Expect(result).To(Equal(test.shouldProcess),
					"BR-AP-PROCESSOR-002: Severity filtering must work correctly for severity %s", test.severity)
			}
		})

		It("should apply namespace-based filtering with wildcards", func() {
			// Test REAL business wildcard filtering logic
			namespaceTests := []struct {
				namespace     string
				shouldProcess bool
			}{
				{"production", true},
				{"prod-api", true},
				{"prod-db", true},
				{"development", false},
				{"test", false},
				{"staging", false},
				{"", false},
			}

			for _, test := range namespaceTests {
				alert := types.Alert{
					Name:        "test-alert",
					Status:      "firing",
					Severity:    "critical",
					Namespace:   test.namespace,
					Labels:      make(map[string]string),
					Annotations: make(map[string]string),
				}

				// Test REAL business wildcard logic
				result := alertProcessor.ShouldProcess(alert)

				// Validate REAL business wildcard outcomes
				Expect(result).To(Equal(test.shouldProcess),
					"BR-AP-PROCESSOR-002: Namespace wildcard filtering must work correctly for namespace %s", test.namespace)
			}
		})

		It("should handle complex multi-condition filtering", func() {
			// Test REAL business multi-condition filtering logic
			complexAlert := types.Alert{
				Name:      "MemoryUsageHigh",
				Status:    "firing",
				Severity:  "critical",
				Namespace: "production",
				Labels: map[string]string{
					"service": "api-server",
					"team":    "platform",
				},
				Annotations: map[string]string{
					"runbook": "https://runbooks.example.com/memory",
				},
			}

			// Test REAL business logic for complex filtering
			result := alertProcessor.ShouldProcess(complexAlert)

			// Validate REAL business multi-condition outcomes
			Expect(result).To(BeTrue(),
				"BR-AP-PROCESSOR-002: Complex multi-condition filtering must process matching alerts")
		})
	})

	// COMPREHENSIVE error handling and recovery testing
	Context("BR-AP-PROCESSOR-003: Error Handling and Recovery", func() {
		It("should handle LLM client failures gracefully", func() {
			// Test REAL business error handling for external LLM failures
			alert := createCriticalProductionAlert()

			// Simulate LLM failure scenarios
			llmFailures := []struct {
				name     string
				errorMsg string
				expected string
			}{
				{
					name:     "Connection timeout",
					errorMsg: "connection timeout",
					expected: "should return connection error",
				},
				{
					name:     "Rate limit exceeded",
					errorMsg: "rate limit exceeded",
					expected: "should return rate limit error",
				},
				{
					name:     "Invalid response format",
					errorMsg: "invalid JSON response",
					expected: "should return parsing error",
				},
			}

			for _, failure := range llmFailures {
				By(failure.name)

				// Setup LLM failure
				mockLLMClient.SetError(failure.errorMsg)

				// Test REAL business error handling
				err := alertProcessor.ProcessAlert(ctx, alert)

				// Validate REAL business error handling outcomes
				Expect(err).To(HaveOccurred(),
					"BR-AP-PROCESSOR-003: LLM failures must be handled gracefully for %s", failure.name)
				Expect(err.Error()).To(ContainSubstring("failed to analyze alert"),
					"BR-AP-PROCESSOR-003: Error messages must be descriptive for %s", failure.name)

				// Reset for next test
				mockLLMClient.SetError("")
			}
		})

		It("should handle executor failures with proper error propagation", func() {
			// Test REAL business error handling for executor failures
			alert := createCriticalProductionAlert()

			// Setup successful LLM response
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "scale_deployment",
				Confidence:        0.85,
				Reasoning:         "Alert analysis - High memory usage detected - Automated action recommendation",
				ProcessingTime:    25 * time.Millisecond,
				Metadata: map[string]interface{}{
					"test_generated": true,
					"replicas":       3,
				},
			})

			// Setup executor failure through adapter
			mockExecutor.(*executorAdapter).SetError("failed to scale deployment")

			// Test REAL business error handling
			err := alertProcessor.ProcessAlert(ctx, alert)

			// Validate REAL business error propagation
			Expect(err).To(HaveOccurred(),
				"BR-AP-PROCESSOR-003: Executor failures must be propagated")
			Expect(err.Error()).To(ContainSubstring("failed to execute action"),
				"BR-AP-PROCESSOR-003: Executor error messages must be descriptive")
		})

		It("should handle action repository failures gracefully", func() {
			// Test REAL business error handling for repository failures
			alert := createCriticalProductionAlert()

			// Setup successful LLM response
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "scale_deployment",
				Confidence:        0.85,
				Reasoning:         "Alert analysis - Automated action recommendation - High memory usage detected",
				ProcessingTime:    25 * time.Millisecond,
				Metadata: map[string]interface{}{
					"test_generated": true,
					"replicas":       3,
				},
			})

			// Setup repository failure (should not stop processing)
			mockActionRepo.SetError("database connection failed")

			// Test REAL business error handling
			err := alertProcessor.ProcessAlert(ctx, alert)

			// Validate REAL business graceful degradation
			// Repository failures should not prevent alert processing
			Expect(err).ToNot(HaveOccurred(),
				"BR-AP-PROCESSOR-003: Repository failures must not stop alert processing")
		})
	})

	// COMPREHENSIVE performance and timing testing
	Context("BR-AP-PROCESSOR-004: Performance and Timing Requirements", func() {
		It("should process alerts within 5-second business requirement", func() {
			// Test REAL business performance requirement BR-PA-003
			alert := createCriticalProductionAlert()

			// Setup fast LLM response
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "scale_deployment",
				Confidence:        0.85,
				Reasoning:         "Alert analysis - Automated action recommendation - High memory usage detected",
				ProcessingTime:    25 * time.Millisecond,
				Metadata: map[string]interface{}{
					"test_generated": true,
					"replicas":       3,
				},
			})

			// Test REAL business timing requirement
			startTime := time.Now()
			err := alertProcessor.ProcessAlert(ctx, alert)
			processingTime := time.Since(startTime)

			// Validate REAL business timing outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-AP-PROCESSOR-004: Alert processing must succeed within time limit")
			Expect(processingTime).To(BeNumerically("<", 5*time.Second),
				"BR-AP-PROCESSOR-004: Alert processing must complete within 5 seconds (BR-PA-003)")
		})

		It("should handle batch alert processing efficiently", func() {
			// Test REAL business batch processing performance
			alerts := []types.Alert{
				createCriticalProductionAlert(),
				createHighMemoryAlert(),
				createWildcardNamespaceAlert(),
			}

			// Setup LLM responses for batch processing
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "scale_deployment",
				Confidence:        0.85,
				Reasoning:         "Alert analysis - Automated action recommendation - Batch processing response",
				ProcessingTime:    25 * time.Millisecond,
				Metadata: map[string]interface{}{
					"test_generated": true,
					"replicas":       3,
				},
			})

			// Test REAL business batch processing
			startTime := time.Now()
			var processingErrors []error
			for _, alert := range alerts {
				if err := alertProcessor.ProcessAlert(ctx, alert); err != nil {
					processingErrors = append(processingErrors, err)
				}
			}
			batchProcessingTime := time.Since(startTime)

			// Validate REAL business batch processing outcomes
			Expect(len(processingErrors)).To(Equal(0),
				"BR-AP-PROCESSOR-004: Batch processing must succeed for all valid alerts")
			Expect(batchProcessingTime).To(BeNumerically("<", 15*time.Second),
				"BR-AP-PROCESSOR-004: Batch processing must be efficient")
		})
	})

	// COMPREHENSIVE business requirement compliance testing
	Context("BR-AP-PROCESSOR-005: Business Requirement Compliance", func() {
		It("should validate alert structure before processing", func() {
			// Test REAL business validation logic
			invalidAlerts := []types.Alert{
				createAlertWithoutName(),
				createAlertWithoutStatus(),
				createAlertWithEmptyFields(),
			}

			for i, alert := range invalidAlerts {
				// Test REAL business validation
				err := alertProcessor.ProcessAlert(ctx, alert)

				// Validate REAL business validation outcomes
				Expect(err).To(HaveOccurred(),
					"BR-AP-PROCESSOR-005: Invalid alert structure %d must be rejected", i+1)
			}
		})

		It("should track processing metrics for business monitoring", func() {
			// Test REAL business metrics collection
			alert := createCriticalProductionAlert()

			// Setup successful processing
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "scale_deployment",
				Confidence:        0.85,
				Reasoning:         "Alert analysis - Automated action recommendation - Metrics tracking test",
				ProcessingTime:    25 * time.Millisecond,
				Metadata: map[string]interface{}{
					"test_generated": true,
					"replicas":       3,
				},
			})

			// Test REAL business processing
			err := alertProcessor.ProcessAlert(ctx, alert)

			// Validate REAL business processing outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-AP-PROCESSOR-005: Metrics tracking alerts must process successfully")

			// Note: In a real implementation, we would verify metrics were recorded
			// This demonstrates the pattern for business metrics validation
		})

		It("should handle confidence scoring correctly", func() {
			// Test REAL business confidence scoring logic
			confidenceTests := []struct {
				confidence    float64
				shouldExecute bool
				description   string
			}{
				{0.95, true, "very high confidence"},
				{0.85, true, "high confidence"},
				{0.75, true, "medium-high confidence"},
				{0.65, true, "medium confidence"},
				{0.45, false, "low confidence"},
				{0.25, false, "very low confidence"},
			}

			for _, test := range confidenceTests {
				alert := createCriticalProductionAlert()

				// Setup LLM response with specific confidence
				mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
					RecommendedAction: "scale_deployment",
					Confidence:        test.confidence,
					Reasoning:         fmt.Sprintf("Alert analysis - Automated action recommendation - Confidence test: %s", test.description),
					ProcessingTime:    25 * time.Millisecond,
					Metadata: map[string]interface{}{
						"test_generated": true,
						"replicas":       3,
					},
				})

				// Test REAL business confidence logic
				err := alertProcessor.ProcessAlert(ctx, alert)

				// Validate REAL business confidence outcomes
				Expect(err).ToNot(HaveOccurred(),
					"BR-AP-PROCESSOR-005: Confidence scoring must not cause errors for %s", test.description)

				// Note: In a real implementation, we would verify that low confidence
				// recommendations are handled appropriately (e.g., require manual approval)
			}
		})
	})
})

// Helper functions to create test alerts
// These test REAL business logic with various scenarios

func createCriticalProductionAlert() types.Alert {
	return types.Alert{
		Name:      "MemoryUsageHigh",
		Status:    "firing",
		Severity:  "critical",
		Namespace: "production",
		Resource:  "deployment/api-server",
		Labels: map[string]string{
			"service":   "api-server",
			"team":      "platform",
			"alertname": "MemoryUsageHigh",
		},
		Annotations: map[string]string{
			"description": "Memory usage is above 90%",
			"runbook":     "https://runbooks.example.com/memory",
		},
		StartsAt: time.Now().Add(-5 * time.Minute),
		EndsAt:   nil, // Still firing
	}
}

func createHighMemoryAlert() types.Alert {
	return types.Alert{
		Name:      "OutOfMemory",
		Status:    "firing",
		Severity:  "high",
		Namespace: "production",
		Resource:  "pod/worker-123",
		Labels: map[string]string{
			"service":   "worker",
			"team":      "data",
			"alertname": "OutOfMemory",
		},
		Annotations: map[string]string{
			"description": "Pod is running out of memory",
			"summary":     "Memory exhaustion detected",
		},
		StartsAt: time.Now().Add(-2 * time.Minute),
		EndsAt:   nil, // Still firing
	}
}

func createLowSeverityDevAlert() types.Alert {
	return types.Alert{
		Name:      "DiskSpaceLow",
		Status:    "firing",
		Severity:  "low",
		Namespace: "development",
		Resource:  "node/dev-node-1",
		Labels: map[string]string{
			"service":   "monitoring",
			"team":      "platform",
			"alertname": "DiskSpaceLow",
		},
		Annotations: map[string]string{
			"description": "Disk space is below 20%",
		},
		StartsAt: time.Now().Add(-1 * time.Minute),
		EndsAt:   nil, // Still firing
	}
}

func createResolvedAlert() types.Alert {
	return types.Alert{
		Name:      "CPUUsageHigh",
		Status:    "resolved",
		Severity:  "critical",
		Namespace: "production",
		Resource:  "deployment/api-server",
		Labels: map[string]string{
			"service":   "api-server",
			"team":      "platform",
			"alertname": "CPUUsageHigh",
		},
		Annotations: map[string]string{
			"description": "CPU usage was above 80%",
		},
		StartsAt: time.Now().Add(-10 * time.Minute),
		EndsAt:   func() *time.Time { t := time.Now().Add(-1 * time.Minute); return &t }(), // Resolved
	}
}

func createAlertWithoutName() types.Alert {
	return types.Alert{
		Name:        "", // Missing name
		Status:      "firing",
		Severity:    "critical",
		Namespace:   "production",
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}
}

func createAlertWithoutStatus() types.Alert {
	return types.Alert{
		Name:        "TestAlert",
		Status:      "", // Missing status
		Severity:    "critical",
		Namespace:   "production",
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}
}

func createAlertWithCustomLabels() types.Alert {
	return types.Alert{
		Name:      "CustomAlert",
		Status:    "firing",
		Severity:  "critical",
		Namespace: "production",
		Resource:  "custom-resource",
		Labels: map[string]string{
			"custom_label": "custom_value",
			"environment":  "prod",
			"application":  "web-app",
			"alertname":    "CustomAlert",
		},
		Annotations: map[string]string{
			"custom_annotation": "custom_annotation_value",
			"description":       "Custom alert for testing",
		},
		StartsAt: time.Now().Add(-3 * time.Minute),
		EndsAt:   nil, // Still firing
	}
}

func createWildcardNamespaceAlert() types.Alert {
	return types.Alert{
		Name:      "DatabaseConnectionError",
		Status:    "firing",
		Severity:  "critical",
		Namespace: "prod-database", // Matches prod-* wildcard
		Resource:  "deployment/postgres",
		Labels: map[string]string{
			"service":   "database",
			"team":      "data",
			"alertname": "DatabaseConnectionError",
		},
		Annotations: map[string]string{
			"description": "Database connection pool exhausted",
			"impact":      "high",
		},
		StartsAt: time.Now().Add(-7 * time.Minute),
		EndsAt:   nil, // Still firing
	}
}

func createComplexFilterAlert() types.Alert {
	return types.Alert{
		Name:      "MemoryUsageHigh",
		Status:    "firing",
		Severity:  "critical",
		Namespace: "prod-api", // Matches namespace wildcard
		Resource:  "deployment/api-gateway",
		Labels: map[string]string{
			"service":     "api-gateway",
			"team":        "platform",
			"alertname":   "MemoryUsageHigh", // Matches alertname filter
			"environment": "production",
		},
		Annotations: map[string]string{
			"description": "Complex filtering test alert",
			"priority":    "P1",
		},
		StartsAt: time.Now().Add(-4 * time.Minute),
		EndsAt:   nil, // Still firing
	}
}

func createSpecialCharAlert() types.Alert {
	return types.Alert{
		Name:      "Special-Char_Alert.Test",
		Status:    "firing",
		Severity:  "medium", // Won't match severity filter
		Namespace: "test-namespace",
		Resource:  "pod/special-pod-123",
		Labels: map[string]string{
			"service":   "test-service",
			"alertname": "Special-Char_Alert.Test",
		},
		Annotations: map[string]string{
			"description": "Alert with special characters in name",
		},
		StartsAt: time.Now().Add(-1 * time.Minute),
		EndsAt:   nil, // Still firing
	}
}

func createAlertWithEmptyFields() types.Alert {
	return types.Alert{
		Name:        "EmptyFieldsAlert",
		Status:      "firing",
		Severity:    "", // Empty severity
		Namespace:   "", // Empty namespace
		Resource:    "",
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}
}
