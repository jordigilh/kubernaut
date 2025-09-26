//go:build integration
// +build integration

package ai_orchestration

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/ai/orchestration"
	"github.com/jordigilh/kubernaut/pkg/testutil/hybrid"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

// BR-ENSEMBLE-INT-001: Cross-component orchestration with real LLM providers
// BR-ENSEMBLE-INT-002: Performance tracking integration with monitoring systems
// BR-ENSEMBLE-INT-003: Cost optimization integration with budget management
// BR-ENSEMBLE-INT-004: Health monitoring integration with alerting systems
var _ = Describe("Multi-Model Orchestration Cross-Component Integration", Ordered, func() {
	var (
		hooks  *testshared.TestLifecycleHooks
		suite  *testshared.StandardTestSuite
		ctx    context.Context
		cancel context.CancelFunc

		// REAL business components for integration testing
		orchestrator *orchestration.MultiModelOrchestrator
		llmClients   []llm.Client
		logger       *logrus.Logger

		// Mock ONLY external dependencies (when needed for specific tests)
	)

	BeforeAll(func() {
		// Setup integration test environment with real infrastructure where available
		hooks = testshared.SetupAIIntegrationTest("Multi-Model Orchestration Integration",
			testshared.WithRealVectorDB(),
			testshared.WithDatabaseIsolation(testshared.TransactionIsolation),
		)
		hooks.Setup()
		suite = hooks.GetSuite()
		logger = suite.Logger
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only when needed for specific tests

		// Create REAL LLM clients with different configurations for ensemble testing
		llmClients = createRealLLMClientsForIntegration(logger)

		// Create REAL orchestrator with real business logic
		orchestrator = orchestration.NewMultiModelOrchestrator(llmClients, logger)

		// Configure real cost tracking
		orchestrator.SetModelCost("model-1", 0.10) // High-performance, high-cost
		orchestrator.SetModelCost("model-2", 0.05) // Balanced cost-performance
		orchestrator.SetModelCost("model-3", 0.02) // Low-cost, basic performance
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
	})

	Context("BR-ENSEMBLE-INT-001: Cross-Component LLM Provider Integration", func() {
		It("should coordinate multiple real LLM providers for ensemble decisions", func() {
			// Business Requirement: Real multi-provider coordination
			prompt := "Analyze production alert: Memory usage at 95% in payment service cluster"

			// Test real cross-component interaction
			decision, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.CriticalPriority)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-INT-001: Multi-provider consensus must succeed")
			Expect(decision).ToNot(BeNil(),
				"BR-ENSEMBLE-INT-001: Must return valid consensus decision")
			Expect(decision.ParticipatingModels).To(BeNumerically(">=", 2),
				"BR-ENSEMBLE-INT-001: Must coordinate multiple real providers")
			Expect(decision.Confidence).To(BeNumerically(">=", 0.8),
				"BR-ENSEMBLE-INT-001: Real provider consensus must achieve high confidence")

			// Validate cross-component data flow
			Expect(decision.Action).To(BeElementOf([]string{"restart_pod", "scale_deployment", "investigate_logs"}),
				"BR-ENSEMBLE-INT-001: Must produce valid actionable recommendations")
		})

		It("should handle real provider failures with automatic failover", func() {
			// Simulate real provider failure scenario
			orchestrator.SimulateModelFailure("model-2")

			prompt := "Critical alert: Database connection pool exhausted"
			decision, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.CriticalPriority)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-INT-001: Must handle provider failures gracefully")
			Expect(decision.FailoverApplied).To(BeTrue(),
				"BR-ENSEMBLE-INT-001: Must apply automatic failover")
			Expect(decision.FailedModels).To(ContainElement("model-2"),
				"BR-ENSEMBLE-INT-001: Must track failed providers")
			Expect(decision.ParticipatingModels).To(BeNumerically(">=", 1),
				"BR-ENSEMBLE-INT-001: Must continue with available providers")
		})
	})

	Context("BR-ENSEMBLE-INT-002: Performance Tracking System Integration", func() {
		It("should integrate performance metrics with monitoring systems", func() {
			// Record performance data across multiple requests
			prompts := []string{
				"Analyze CPU spike in web tier",
				"Investigate memory leak in background workers",
				"Review network latency in microservices",
			}

			for _, prompt := range prompts {
				_, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.StandardPriority)
				Expect(err).ToNot(HaveOccurred())
			}

			// Validate cross-component performance tracking
			performance := orchestrator.GetModelPerformance()
			Expect(performance).To(HaveLen(len(llmClients)),
				"BR-ENSEMBLE-INT-002: Must track all provider performance")

			for modelID, metrics := range performance {
				Expect(metrics.RequestCount).To(BeNumerically(">", 0),
					"BR-ENSEMBLE-INT-002: Must record request metrics for %s", modelID)
				Expect(metrics.ResponseTime).To(BeNumerically(">", 0),
					"BR-ENSEMBLE-INT-002: Must track response times for %s", modelID)
				Expect(metrics.AccuracyRate).To(BeNumerically(">=", 0),
					"BR-ENSEMBLE-INT-002: Must calculate accuracy rates for %s", modelID)
			}
		})

		It("should optimize model weights based on real performance data", func() {
			// Record different accuracy levels for models
			orchestrator.RecordModelAccuracy("model-1", 0.95) // High performer
			orchestrator.RecordModelAccuracy("model-2", 0.80) // Medium performer
			orchestrator.RecordModelAccuracy("model-3", 0.65) // Lower performer

			// Trigger cross-component optimization
			err := orchestrator.OptimizeModelWeights()
			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-INT-002: Performance optimization must succeed")

			// Validate weight optimization integration
			weights := orchestrator.GetModelWeights()
			Expect(weights["model-1"]).To(BeNumerically(">", weights["model-2"]),
				"BR-ENSEMBLE-INT-002: High performers must receive higher weights")
			Expect(weights["model-2"]).To(BeNumerically(">", weights["model-3"]),
				"BR-ENSEMBLE-INT-002: Weight optimization must reflect performance hierarchy")
		})
	})

	Context("BR-ENSEMBLE-INT-003: Cost Management System Integration", func() {
		It("should integrate with budget management for cost-aware decisions", func() {
			// Test integration with cost management systems
			budget := orchestration.CostBudget{
				MaxCostPerRequest: 0.08, // Medium budget
				AccuracyThreshold: 0.80,
			}

			prompt := "Routine alert: Disk usage at 75% in logging cluster"
			decision, err := orchestrator.GetCostOptimizedDecision(ctx, prompt, budget)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-INT-003: Cost-optimized decisions must succeed")
			Expect(decision.TotalCost).To(BeNumerically("<=", budget.MaxCostPerRequest),
				"BR-ENSEMBLE-INT-003: Must respect budget constraints")
			Expect(decision.PredictedAccuracy).To(BeNumerically(">=", budget.AccuracyThreshold),
				"BR-ENSEMBLE-INT-003: Must maintain accuracy requirements")
			Expect(decision.CostSavings).To(BeNumerically(">=", 0),
				"BR-ENSEMBLE-INT-003: Must demonstrate cost optimization")
		})

		It("should provide cost optimization recommendations to management systems", func() {
			// Test cross-component cost recommendation integration
			recommendations := orchestrator.GetCostOptimizationRecommendations()

			Expect(recommendations).ToNot(BeEmpty(),
				"BR-ENSEMBLE-INT-003: Must provide cost optimization guidance")

			for _, rec := range recommendations {
				Expect(rec.PotentialSavings).To(BeNumerically(">", 0),
					"BR-ENSEMBLE-INT-003: Recommendations must show measurable savings")
				Expect(rec.AccuracyImpact).To(BeNumerically(">=", -0.10),
					"BR-ENSEMBLE-INT-003: Accuracy impact must be acceptable")
				Expect(rec.Implementation).ToNot(BeEmpty(),
					"BR-ENSEMBLE-INT-003: Must provide actionable implementation guidance")
			}
		})
	})

	Context("BR-ENSEMBLE-INT-004: Health Monitoring and Alerting Integration", func() {
		It("should integrate with monitoring systems for real-time health tracking", func() {
			// Test cross-component health monitoring integration
			health := orchestrator.CheckModelHealth()

			Expect(health).To(HaveLen(len(llmClients)),
				"BR-ENSEMBLE-INT-004: Must monitor all provider health")

			for modelID, status := range health {
				Expect(status.LastChecked).ToNot(BeZero(),
					"BR-ENSEMBLE-INT-004: Must record health check timestamps for %s", modelID)
				Expect(status.ResponseTime).To(BeNumerically(">=", 0),
					"BR-ENSEMBLE-INT-004: Must track response times for %s", modelID)
				Expect(status.ErrorRate).To(BeNumerically(">=", 0),
					"BR-ENSEMBLE-INT-004: Must calculate error rates for %s", modelID)
			}
		})

		It("should validate model recovery through cross-component health checks", func() {
			// Simulate model failure and recovery cycle
			orchestrator.SimulateModelFailure("model-3")
			orchestrator.SimulateModelRecovery("model-3")

			// Test cross-component recovery validation
			recoveryStatus := orchestrator.ValidateModelRecovery("model-3")

			Expect(recoveryStatus.IsRecovered).To(BeTrue(),
				"BR-ENSEMBLE-INT-004: Must validate successful recovery")
			Expect(recoveryStatus.ValidationTests).To(BeNumerically(">", 0),
				"BR-ENSEMBLE-INT-004: Must perform validation tests")
			Expect(recoveryStatus.PerformanceBaseline).To(BeNumerically(">", 0),
				"BR-ENSEMBLE-INT-004: Must establish performance baseline")
		})

		It("should coordinate maintenance mode across monitoring systems", func() {
			// Test cross-component maintenance coordination
			err := orchestrator.SetModelMaintenance("model-1", true)
			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-INT-004: Maintenance mode setting must succeed")

			// Validate maintenance mode affects decision making
			prompt := "Standard alert: Log rotation needed"
			decision, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.StandardPriority)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-INT-004: Must handle maintenance mode gracefully")
			Expect(decision.MaintenanceMode).To(BeTrue(),
				"BR-ENSEMBLE-INT-004: Must indicate maintenance mode operation")
			Expect(decision.ParticipatingModels).To(Equal(len(llmClients)-1),
				"BR-ENSEMBLE-INT-004: Must exclude models in maintenance")
		})
	})

	Context("Cross-Component Performance Integration", func() {
		It("should maintain performance under concurrent cross-component load", func() {
			// Test concurrent cross-component operations
			const concurrentRequests = 5
			results := make(chan error, concurrentRequests)

			for i := 0; i < concurrentRequests; i++ {
				go func(requestID int) {
					prompt := fmt.Sprintf("Concurrent alert %d: Service degradation detected", requestID)
					_, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.StandardPriority)
					results <- err
				}(i)
			}

			// Validate all concurrent operations succeed
			for i := 0; i < concurrentRequests; i++ {
				err := <-results
				Expect(err).ToNot(HaveOccurred(),
					"BR-ENSEMBLE-INT: Concurrent cross-component operations must succeed")
			}
		})

		It("should complete cross-component decisions within performance thresholds", func() {
			start := time.Now()

			prompt := "Performance test: Database connection timeout in user service"
			decision, err := orchestrator.GetConsensusDecision(ctx, prompt, orchestration.StandardPriority)

			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred(),
				"BR-ENSEMBLE-INT: Performance test must succeed")
			Expect(decision).ToNot(BeNil(),
				"BR-ENSEMBLE-INT: Must return valid decision")
			Expect(duration).To(BeNumerically("<", 15*time.Second),
				"BR-ENSEMBLE-INT: Cross-component decisions must complete within 15 seconds")
		})
	})
})

// Helper function to create real LLM clients for integration testing
func createRealLLMClientsForIntegration(logger *logrus.Logger) []llm.Client {
	clients := make([]llm.Client, 0, 3)

	// Create different LLM client configurations for ensemble testing
	configs := []config.LLMConfig{
		{
			Provider:    "ramalama",
			Model:       "ggml-org/gpt-oss-20b-GGUF",
			Temperature: 0.7,
			MaxTokens:   8192,
			Timeout:     30 * time.Second,
		},
		{
			Provider:    "ramalama",
			Model:       "ggml-org/gpt-oss-20b-GGUF",
			Temperature: 0.5,
			MaxTokens:   4096,
			Timeout:     20 * time.Second,
		},
		{
			Provider:    "ramalama",
			Model:       "ggml-org/gpt-oss-20b-GGUF",
			Temperature: 0.3,
			MaxTokens:   2048,
			Timeout:     10 * time.Second,
		},
	}

	for i, cfg := range configs {
		client, err := llm.NewClient(cfg, logger)
		if err != nil {
			// Fallback to mock if real LLM not available using centralized infrastructure
			logger.WithError(err).Warnf("Real LLM client %d unavailable, using centralized mock", i+1)
			mockClient := hybrid.CreateLLMClient(logger)
			clients = append(clients, mockClient)
		} else {
			clients = append(clients, client)
		}
	}

	return clients
}

// REMOVED: SimpleLLMClientMock - Replaced with centralized hybrid.CreateLLMClient()
// Following project guidelines: REUSE existing mock infrastructure, AVOID duplication
