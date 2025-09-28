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
	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/sirupsen/logrus"
)

// TDD RED PHASE: Processor Service Business Requirements Tests
// Following TDD methodology: Write failing tests FIRST that define business contract
// Business Requirements: BR-PROC-001 to BR-PROC-015 (Processor Service)

var _ = Describe("TDD RED: Processor Service Business Requirements", func() {
	var (
		// Mock ONLY external dependencies (Rule 03 compliance)
		mockLLMClient *mocks.MockLLMClient
		mockExecutor  executor.Executor
		mockLogger    *logrus.Logger

		// Use REAL business logic components
		processorService *processor.EnhancedService
		processorConfig  *processor.Config

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock ONLY external dependencies
		mockLLMClient = mocks.NewMockLLMClient()
		mockExecutor = &mockExecutorAdapter{}
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel)

		// Configure AI client for testing
		mockLLMClient.SetHealthy(true)
		mockLLMClient.SetEndpoint("mock://ai-service")

		// Create REAL business configuration
		processorConfig = &processor.Config{
			ProcessorPort:           8095,
			AIServiceTimeout:        60 * time.Second,
			MaxConcurrentProcessing: 100,
			ProcessingTimeout:       300 * time.Second,
			AI: processor.AIConfig{
				Provider:            "holmesgpt",
				Endpoint:            "mock://ai-service",
				Model:               "hf://ggml-org/gpt-oss-20b-GGUF",
				Timeout:             60 * time.Second,
				MaxRetries:          3,
				ConfidenceThreshold: 0.7,
			},
		}

		// Create enhanced processor service with REAL business logic
		// This MUST fail initially (TDD RED phase requirement)
		processorService = processor.NewEnhancedService(
			mockLLMClient,   // External: AI service
			mockExecutor,    // External: K8s operations (simplified for TDD RED)
			processorConfig, // Real: Business configuration
		)
	})

	AfterEach(func() {
		cancel()
	})

	// BR-PROC-001: NEW Enhanced Service Metrics and Monitoring (NOT YET IMPLEMENTED)
	Context("BR-PROC-001: Enhanced Service Metrics", func() {
		It("should provide detailed service metrics", func() {
			// TDD RED: This test MUST fail - GetMetrics() method doesn't exist yet
			metrics := processorService.GetMetrics()
			Expect(metrics).To(HaveKey("alerts_processed"))
			Expect(metrics).To(HaveKey("ai_analysis_count"))
			Expect(metrics).To(HaveKey("fallback_count"))
			Expect(metrics).To(HaveKey("average_processing_time"))
		})

		It("should track processing statistics", func() {
			// TDD RED: This test MUST fail - GetProcessingStats() method doesn't exist yet
			stats := processorService.GetProcessingStats()
			Expect(stats).To(HaveKey("total_processed"))
			Expect(stats).To(HaveKey("success_rate"))
			Expect(stats).To(HaveKey("ai_success_rate"))
		})
	})

	// BR-PROC-002: Alert Processing with Enhanced AI Integration
	Context("BR-PROC-002: Enhanced Alert Processing", func() {
		It("should process alerts with AI analysis and return detailed results", func() {
			// TDD RED: This test MUST fail initially
			alert := types.Alert{
				Name:      "TestAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
				Labels: map[string]string{
					"component": "database",
				},
			}

			// Configure mock AI response
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "restart-pod",
				Confidence:        0.85,
				Reasoning:         "High memory usage detected",
				ProcessingTime:    50 * time.Millisecond,
			})

			// Test enhanced processing
			result, err := processorService.ProcessAlert(ctx, alert)

			// Validate enhanced processing results
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Success).To(BeTrue())
			Expect(result.AIAnalysisPerformed).To(BeTrue())
			Expect(result.Confidence).To(BeNumerically(">=", 0.7))
			Expect(result.ProcessingMethod).To(Equal("ai-enhanced"))
			Expect(result.RecommendedActions).To(ContainElement("restart-pod"))
		})

		It("should handle AI service failures with graceful fallback", func() {
			// TDD RED: This test MUST fail initially
			alert := types.Alert{
				Name:     "TestAlert",
				Severity: "critical",
				Status:   "firing",
			}

			// Configure AI to fail
			mockLLMClient.SetError("AI service unavailable")

			// Test fallback processing
			result, err := processorService.ProcessAlert(ctx, alert)

			// Validate fallback behavior
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Success).To(BeTrue())
			Expect(result.AIAnalysisPerformed).To(BeFalse())
			Expect(result.FallbackUsed).To(BeTrue())
			Expect(result.ProcessingMethod).To(Equal("rule-based"))
		})
	})

	// BR-PROC-003: NEW Batch Processing Capabilities (NOT YET IMPLEMENTED)
	Context("BR-PROC-003: Batch Processing", func() {
		It("should process multiple alerts in batch", func() {
			// TDD RED: This test MUST fail - ProcessAlertBatch() method doesn't exist yet
			alerts := []types.Alert{
				{Name: "Alert1", Severity: "critical", Status: "firing"},
				{Name: "Alert2", Severity: "high", Status: "firing"},
			}

			batchResult := processorService.ProcessAlertBatch(ctx, alerts)
			Expect(batchResult.TotalProcessed).To(Equal(2))
			Expect(batchResult.SuccessCount).To(Equal(2))
			Expect(batchResult.BatchID).ToNot(BeEmpty())
		})

		It("should provide batch processing statistics", func() {
			// TDD RED: This test MUST fail - GetBatchStats() method doesn't exist yet
			stats := processorService.GetBatchStats()
			Expect(stats).To(HaveKey("active_batches"))
			Expect(stats).To(HaveKey("completed_batches"))
		})
	})

	// BR-PROC-004: Service Health and Monitoring
	Context("BR-PROC-004: Service Health Monitoring", func() {
		It("should provide comprehensive health status", func() {
			// TDD RED: This test MUST fail initially
			health := processorService.Health()

			Expect(health).To(HaveKey("status"))
			Expect(health).To(HaveKey("service"))
			Expect(health).To(HaveKey("ai_integration"))
			Expect(health["status"]).To(Equal("healthy"))
			Expect(health["service"]).To(Equal("processor-service"))
			Expect(health["ai_integration"]).To(Equal("enabled"))
		})
	})
})

// mockExecutorAdapter implements executor.Executor for TDD RED phase
type mockExecutorAdapter struct{}

func (m *mockExecutorAdapter) Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert, trace *actionhistory.ResourceActionTrace) error {
	// TDD RED: Simple mock implementation that will be enhanced in GREEN phase
	return nil
}

func (m *mockExecutorAdapter) IsHealthy() bool {
	return true
}

func (m *mockExecutorAdapter) GetActionRegistry() *executor.ActionRegistry {
	return executor.NewActionRegistry()
}

func TestProcessorServiceTDD(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Processor Service TDD Suite")
}
