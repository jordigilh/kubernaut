package multimodel

import (
	"testing"
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/ai/orchestration"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// BR-ENSEMBLE-001 to BR-ENSEMBLE-004: Multi-Model Orchestration Unit Tests
var _ = Describe("Multi-Model Orchestration Business Logic", func() {
	var (
		ctx            context.Context
		orchestrator   *orchestration.MultiModelOrchestrator
		mockLLMClient1 *mocks.MockLLMClient
		mockLLMClient2 *mocks.MockLLMClient
		mockLLMClient3 *mocks.MockLLMClient
		logger         *logrus.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)

		// Create multiple simple mock LLM clients for ensemble testing
		mockLLMClient1 = mocks.NewMockLLMClient()
		mockLLMClient2 = mocks.NewMockLLMClient()
		mockLLMClient3 = mocks.NewMockLLMClient()

		// Create orchestrator with multiple models (this will fail initially - RED phase)
		models := []llm.Client{mockLLMClient1, mockLLMClient2, mockLLMClient3}
		orchestrator = orchestration.NewMultiModelOrchestrator(models, logger)
	})

	Context("BR-ENSEMBLE-001: Multi-Model Consensus Decision Making", func() {
		It("should provide weighted consensus for critical decisions", func() {
			// Business Requirement: Multi-model consensus improves accuracy by >15%
			prompt := "Analyze critical production alert: High CPU usage in payment service"

			// This will fail initially (RED phase) - orchestrator doesn't exist yet
			decision, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.CriticalPriority)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-001: Consensus decision must succeed for critical alerts")
			Expect(decision).ToNot(BeNil(),
				"BR-ENSEMBLE-001: Consensus decision must return valid result")
			Expect(decision.Confidence).To(BeNumerically(">=", 0.90),
				"BR-ENSEMBLE-001: Consensus confidence must exceed 90% for critical decisions")
			Expect(decision.Action).ToNot(BeEmpty(),
				"BR-ENSEMBLE-001: Consensus must provide actionable recommendation")
			Expect(decision.ParticipatingModels).To(BeNumerically(">=", 2),
				"BR-ENSEMBLE-001: Consensus must involve multiple models")
		})

		It("should handle model disagreement with intelligent resolution", func() {
			// Configure models to disagree for testing disagreement resolution
			mockLLMClient1.SetChatResponse(`{"action": "restart_pod", "confidence": 0.80}`)
			mockLLMClient2.SetChatResponse(`{"action": "scale_deployment", "confidence": 0.85}`)
			mockLLMClient3.SetChatResponse(`{"action": "notify_only", "confidence": 0.70}`)

			prompt := "Analyze alert with ambiguous symptoms"

			decision, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.StandardPriority)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-001: Must handle model disagreement gracefully")
			Expect(decision.DisagreementResolution).ToNot(BeEmpty(),
				"BR-ENSEMBLE-001: Must provide disagreement resolution strategy")
			Expect(decision.ConflictScore).To(BeNumerically(">=", 0),
				"BR-ENSEMBLE-001: Must quantify model disagreement level")
		})

		It("should support consensus bypass for time-critical operations", func() {
			prompt := "Emergency: Production system down, immediate action needed"

			decision, err := orchestrator.GetFastDecision(ctx, prompt, orchestration.EmergencyPriority)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-001: Emergency decisions must bypass consensus for speed")
			Expect(decision.ResponseTime).To(BeNumerically("<", 2*time.Second),
				"BR-ENSEMBLE-001: Emergency decisions must complete within 2 seconds")
			Expect(decision.BypassReason).To(ContainSubstring("emergency"),
				"BR-ENSEMBLE-001: Must document consensus bypass reasoning")
		})
	})

	Context("BR-ENSEMBLE-002: Model Performance Tracking and Optimization", func() {
		It("should track individual model performance metrics", func() {
			// Simulate multiple decisions to build performance history
			prompts := []string{
				"Analyze memory leak in user service",
				"Investigate network latency issues",
				"Review disk space alerts",
			}

			for _, prompt := range prompts {
				_, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.StandardPriority)
				Expect(err).ToNot(HaveOccurred())
			}

			// Verify performance tracking
			performance := orchestrator.GetModelPerformance()

			Expect(performance).To(HaveLen(3),
				"BR-ENSEMBLE-002: Must track performance for all models")

			for modelID, metrics := range performance {
				Expect(metrics.AccuracyRate).To(BeNumerically(">=", 0),
					"BR-ENSEMBLE-002: Must track accuracy rate for model %s", modelID)
				Expect(metrics.ResponseTime).To(BeNumerically(">", 0),
					"BR-ENSEMBLE-002: Must track response time for model %s", modelID)
				Expect(metrics.RequestCount).To(BeNumerically(">", 0),
					"BR-ENSEMBLE-002: Must track request count for model %s", modelID)
			}
		})

		It("should automatically optimize model weights based on performance", func() {
			// Simulate performance data showing model1 is most accurate
			orchestrator.RecordModelAccuracy("model-1", 0.95)
			orchestrator.RecordModelAccuracy("model-2", 0.80)
			orchestrator.RecordModelAccuracy("model-3", 0.75)

			// Trigger automatic optimization
			err := orchestrator.OptimizeModelWeights()

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-002: Model weight optimization must succeed")

			weights := orchestrator.GetModelWeights()
			Expect(weights["model-1"]).To(BeNumerically(">", weights["model-2"]),
				"BR-ENSEMBLE-002: Higher performing models must receive higher weights")
			Expect(weights["model-2"]).To(BeNumerically(">", weights["model-3"]),
				"BR-ENSEMBLE-002: Model weights must correlate with performance")
		})

		It("should exclude underperforming models from ensembles", func() {
			// Simulate model-3 performing poorly
			orchestrator.RecordModelAccuracy("model-3", 0.45) // Below 50% threshold

			prompt := "Analyze production alert requiring high accuracy"
			decision, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.CriticalPriority)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-002: Must handle model exclusion gracefully")
			Expect(decision.ExcludedModels).To(ContainElement("model-3"),
				"BR-ENSEMBLE-002: Must exclude underperforming models")
			Expect(decision.ParticipatingModels).To(Equal(2),
				"BR-ENSEMBLE-002: Must continue with remaining high-performing models")
		})
	})

	Context("BR-ENSEMBLE-003: Cost-Aware Model Selection", func() {
		It("should select optimal models based on cost-accuracy trade-offs", func() {
			// Configure cost profiles for models
			orchestrator.SetModelCost("model-1", 0.10) // High cost, high accuracy
			orchestrator.SetModelCost("model-2", 0.05) // Medium cost, medium accuracy
			orchestrator.SetModelCost("model-3", 0.02) // Low cost, lower accuracy

			// Set cost budget
			budget := orchestration.CostBudget{
				MaxCostPerRequest: 0.12,
				AccuracyThreshold: 0.85,
			}

			prompt := "Analyze routine maintenance alert"
			decision, err := orchestrator.GetCostOptimizedDecision(ctx, prompt, budget)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-003: Cost-optimized decisions must succeed")
			Expect(decision.TotalCost).To(BeNumerically("<=", budget.MaxCostPerRequest),
				"BR-ENSEMBLE-003: Must respect cost budget constraints")
			Expect(decision.PredictedAccuracy).To(BeNumerically(">=", budget.AccuracyThreshold),
				"BR-ENSEMBLE-003: Must maintain accuracy threshold despite cost constraints")
			Expect(decision.CostSavings).To(BeNumerically(">", 0),
				"BR-ENSEMBLE-003: Must demonstrate cost savings vs full ensemble")
		})

		It("should enforce cost ceilings with graceful degradation", func() {
			budget := orchestration.CostBudget{
				MaxCostPerRequest: 0.03, // Very low budget
				AccuracyThreshold: 0.70, // Reduced accuracy acceptable
			}

			prompt := "Analyze non-critical informational alert"
			decision, err := orchestrator.GetCostOptimizedDecision(ctx, prompt, budget)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-003: Must handle low budget scenarios")
			Expect(decision.DegradationApplied).To(BeTrue(),
				"BR-ENSEMBLE-003: Must apply graceful degradation for cost constraints")
			Expect(decision.SelectedModels).To(HaveLen(1),
				"BR-ENSEMBLE-003: Must use minimal models for cost efficiency")
		})

		It("should generate cost optimization recommendations", func() {
			recommendations := orchestrator.GetCostOptimizationRecommendations()

			Expect(recommendations).ToNot(BeEmpty(),
				"BR-ENSEMBLE-003: Must provide cost optimization recommendations")

			for _, rec := range recommendations {
				Expect(rec.PotentialSavings).To(BeNumerically(">", 0),
					"BR-ENSEMBLE-003: Recommendations must show potential savings")
				Expect(rec.AccuracyImpact).To(BeNumerically(">=", -0.05),
					"BR-ENSEMBLE-003: Accuracy impact must be acceptable (<5% degradation)")
				Expect(rec.Implementation).ToNot(BeEmpty(),
					"BR-ENSEMBLE-003: Must provide actionable implementation guidance")
			}
		})
	})

	Context("BR-ENSEMBLE-004: Real-Time Model Health Monitoring", func() {
		It("should monitor model health and availability", func() {
			// Simulate model health check
			health := orchestrator.CheckModelHealth()

			Expect(health).To(HaveLen(3),
				"BR-ENSEMBLE-004: Must monitor health of all registered models")

			for modelID, status := range health {
				Expect(status.IsHealthy).To(BeAssignableToTypeOf(true),
					"BR-ENSEMBLE-004: Must provide health status for model %s", modelID)
				Expect(status.ResponseTime).To(BeNumerically(">=", 0),
					"BR-ENSEMBLE-004: Must track response time for model %s", modelID)
				Expect(status.ErrorRate).To(BeNumerically(">=", 0),
					"BR-ENSEMBLE-004: Must track error rate for model %s", modelID)
				Expect(status.LastChecked).ToNot(BeZero(),
					"BR-ENSEMBLE-004: Must record last health check time for model %s", modelID)
			}
		})

		It("should implement automatic failover for unhealthy models", func() {
			// Simulate model-2 becoming unhealthy
			orchestrator.SimulateModelFailure("model-2")

			prompt := "Analyze alert requiring ensemble decision"
			decision, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.StandardPriority)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-004: Must handle model failures gracefully")
			Expect(decision.FailedModels).To(ContainElement("model-2"),
				"BR-ENSEMBLE-004: Must track failed models")
			Expect(decision.ParticipatingModels).To(Equal(2),
				"BR-ENSEMBLE-004: Must continue with healthy models only")
			Expect(decision.FailoverApplied).To(BeTrue(),
				"BR-ENSEMBLE-004: Must indicate failover was applied")
		})

		It("should support automatic model recovery validation", func() {
			// Simulate model recovery
			orchestrator.SimulateModelFailure("model-3")
			orchestrator.SimulateModelRecovery("model-3")

			// Verify recovery validation
			recoveryStatus := orchestrator.ValidateModelRecovery("model-3")

			Expect(recoveryStatus.IsRecovered).To(BeTrue(),
				"BR-ENSEMBLE-004: Must validate successful model recovery")
			Expect(recoveryStatus.ValidationTests).To(BeNumerically(">", 0),
				"BR-ENSEMBLE-004: Must perform validation tests before reintegration")
			Expect(recoveryStatus.PerformanceBaseline).To(BeNumerically(">", 0),
				"BR-ENSEMBLE-004: Must establish performance baseline for recovered model")
		})

		It("should maintain service continuity during model maintenance", func() {
			// Simulate maintenance mode for model-1
			err := orchestrator.SetModelMaintenance("model-1", true)
			Expect(err).ToNot(HaveOccurred())

			prompt := "Analyze alert during maintenance window"
			decision, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.StandardPriority)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-004: Must maintain service during model maintenance")
			Expect(decision.MaintenanceMode).To(BeTrue(),
				"BR-ENSEMBLE-004: Must indicate maintenance mode operation")
			Expect(decision.ParticipatingModels).To(Equal(2),
				"BR-ENSEMBLE-004: Must use available models during maintenance")
		})
	})

	Context("Performance and Integration Requirements", func() {
		It("should meet ensemble decision latency requirements", func() {
			start := time.Now()

			prompt := "Standard alert analysis for performance testing"
			_, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.StandardPriority)

			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred(),
				"Performance: Ensemble decisions must succeed")
			Expect(duration).To(BeNumerically("<", 10*time.Second),
				"BR-PERF-ENSEMBLE-001: Ensemble decisions must complete within 10 seconds")
		})

		It("should handle concurrent ensemble requests efficiently", func() {
			// Test concurrent request handling
			const concurrentRequests = 5
			results := make(chan error, concurrentRequests)

			for i := 0; i < concurrentRequests; i++ {
				go func(requestID int) {
					prompt := fmt.Sprintf("Concurrent request %d analysis", requestID)
					_, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.StandardPriority)
					results <- err
				}(i)
			}

			// Verify all requests complete successfully
			for i := 0; i < concurrentRequests; i++ {
				err := <-results
				Expect(err).ToNot(HaveOccurred(),
					"BR-PERF-ENSEMBLE-005: Must handle concurrent requests successfully")
			}
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUmultiUmodelUorchestrator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UmultiUmodelUorchestrator Suite")
}
