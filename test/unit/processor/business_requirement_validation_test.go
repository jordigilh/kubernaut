//go:build unit
// +build unit

package processor_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/sirupsen/logrus"
)

// Business Requirement Validation Tests
// Purpose: Validate that business logic changes are backed by correct business requirement tests
// Focus: Ensure tests validate actual business outcomes, not incorrect expectations

var _ = Describe("Business Requirement Validation", func() {
	var (
		// Mock ONLY external dependencies
		mockLLMClient  *mocks.MockLLMClient
		mockExecutor   *mocks.MockActionExecutor
		mockActionRepo *mocks.MockActionRepository
		mockLogger     *logrus.Logger

		// Use REAL business logic
		standardProcessor processor.Processor
		enhancedProcessor processor.EnhancedProcessor
		filterConfigs     []config.FilterConfig

		cancel context.CancelFunc
	)

	BeforeEach(func() {
		_, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only
		mockLLMClient = mocks.NewMockLLMClient()
		mockExecutor = mocks.NewMockActionExecutor()
		mockActionRepo = mocks.NewMockActionRepository()
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel)

		mockLLMClient.SetHealthy(true)
		mockLLMClient.SetEndpoint("mock://ai-service")

		// Create REAL business filter configurations
		// These match the existing test configurations
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

		// Create both standard and enhanced processors
		standardProcessor = processor.NewProcessor(
			mockLLMClient,
			&executorAdapter{workflowExecutor: mockExecutor},
			filterConfigs,
			mockActionRepo,
			mockLogger,
		)

		enhancedProcessor = processor.NewEnhancedProcessor(
			mockLLMClient,
			&executorAdapter{workflowExecutor: mockExecutor},
			filterConfigs,
			mockActionRepo,
			mockLogger,
			&processor.Config{
				AI: processor.AIConfig{
					ConfidenceThreshold: 0.7,
				},
			},
		)
	})

	AfterEach(func() {
		cancel()
	})

	// BR-AP-001: Validate Actual Business Filtering Logic
	Context("BR-AP-001: Actual Business Filtering Logic Validation", func() {
		It("should validate the REAL business filtering logic (OR between filters, AND within filters)", func() {
			// Business Requirement: Filtering uses OR logic between filters
			// Filter 1: severity in [critical, high]
			// Filter 2: namespace in [production, prod-*]
			// Filter 3: alertname in [MemoryUsageHigh, OutOfMemory]

			testCases := []struct {
				name          string
				alert         types.Alert
				shouldProcess bool
				reason        string
			}{
				{
					name: "critical production alert",
					alert: types.Alert{
						Name:      "CriticalAlert",
						Severity:  "critical",
						Status:    "firing",
						Namespace: "production",
					},
					shouldProcess: true,
					reason:        "matches severity filter AND namespace filter",
				},
				{
					name: "medium production alert",
					alert: types.Alert{
						Name:      "MediumAlert",
						Severity:  "medium",
						Status:    "firing",
						Namespace: "production",
					},
					shouldProcess: true,
					reason:        "matches namespace filter (production) - OR logic",
				},
				{
					name: "critical development alert",
					alert: types.Alert{
						Name:      "CriticalDevAlert",
						Severity:  "critical",
						Status:    "firing",
						Namespace: "development",
					},
					shouldProcess: true,
					reason:        "matches severity filter (critical) - OR logic",
				},
				{
					name: "MemoryUsageHigh in development",
					alert: types.Alert{
						Name:      "MemoryUsageHigh",
						Severity:  "medium",
						Status:    "firing",
						Namespace: "development",
					},
					shouldProcess: true,
					reason:        "matches alertname filter (MemoryUsageHigh) - OR logic",
				},
				{
					name: "medium development generic alert",
					alert: types.Alert{
						Name:      "GenericAlert",
						Severity:  "medium",
						Status:    "firing",
						Namespace: "development",
					},
					shouldProcess: false,
					reason:        "matches no filters - filtered out",
				},
			}

			for _, tc := range testCases {
				// Test standard processor
				standardResult := standardProcessor.ShouldProcess(tc.alert)
				Expect(standardResult).To(Equal(tc.shouldProcess),
					"Standard Processor: %s - %s", tc.name, tc.reason)

				// Test enhanced processor (should have same filtering logic)
				enhancedResult := enhancedProcessor.ShouldProcess(tc.alert)
				Expect(enhancedResult).To(Equal(tc.shouldProcess),
					"Enhanced Processor: %s - %s", tc.name, tc.reason)

				// Validate both processors have identical filtering behavior
				Expect(enhancedResult).To(Equal(standardResult),
					"Enhanced processor must preserve standard processor filtering: %s", tc.name)
			}
		})

		It("should demonstrate why existing tests have incorrect expectations", func() {
			// Business Requirement: Document why some existing tests fail

			// This alert should be processed because it matches the production namespace filter
			productionMediumAlert := types.Alert{
				Name:      "MediumProductionAlert",
				Severity:  "medium",
				Status:    "firing",
				Namespace: "production",
			}

			// ACTUAL business logic: Production alerts are processed regardless of severity
			actualResult := standardProcessor.ShouldProcess(productionMediumAlert)
			Expect(actualResult).To(BeTrue(), "Production alerts should be processed (namespace filter)")

			// INCORRECT expectation in existing tests: medium severity should be filtered
			// This expectation is wrong because it ignores the namespace filter
			incorrectExpectation := false // This is what the existing test expects

			// Demonstrate the discrepancy
			Expect(actualResult).ToNot(Equal(incorrectExpectation),
				"Existing test expectation is incorrect - it ignores namespace filter logic")
		})
	})

	// BR-AP-016: Enhanced Processor Backward Compatibility
	Context("BR-AP-016: Enhanced Processor Backward Compatibility", func() {
		It("should maintain identical filtering behavior between standard and enhanced processors", func() {
			// Business Requirement: Enhanced processor must not change existing filtering behavior

			testAlerts := []types.Alert{
				{Name: "Test1", Severity: "critical", Status: "firing", Namespace: "production"},
				{Name: "Test2", Severity: "medium", Status: "firing", Namespace: "production"},
				{Name: "Test3", Severity: "critical", Status: "firing", Namespace: "development"},
				{Name: "Test4", Severity: "low", Status: "firing", Namespace: "development"},
				{Name: "MemoryUsageHigh", Severity: "info", Status: "firing", Namespace: "test"},
			}

			for _, alert := range testAlerts {
				standardResult := standardProcessor.ShouldProcess(alert)
				enhancedResult := enhancedProcessor.ShouldProcess(alert)

				Expect(enhancedResult).To(Equal(standardResult),
					"Enhanced processor filtering must match standard processor for alert: %s", alert.Name)
			}
		})
	})
})

// executorAdapter adapts MockActionExecutor to executor.Executor interface
type executorAdapter struct {
	workflowExecutor *mocks.MockActionExecutor
}

func (ea *executorAdapter) Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert, actionTrace *actionhistory.ResourceActionTrace) error {
	return nil // Mock successful execution
}

func (ea *executorAdapter) IsHealthy() bool {
	return true
}

func (ea *executorAdapter) GetActionRegistry() *executor.ActionRegistry {
	return executor.NewActionRegistry()
}

func TestBusinessRequirementValidation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Business Requirement Validation Suite")
}
