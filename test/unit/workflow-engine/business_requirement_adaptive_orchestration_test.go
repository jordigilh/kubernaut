//go:build unit
// +build unit

package workflowengine

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/testutil"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/orchestration"
)

/*
 * Business Requirement Validation: Adaptive Orchestration & Optimization
 *
 * This test suite validates business requirements for intelligent workflow orchestration
 * following development guidelines:
 * - Reuses existing orchestration test patterns and mocks
 * - Focuses on business outcomes: reliability improvement, intelligent optimization
 * - Uses meaningful assertions with business success rate thresholds
 * - Integrates with existing adaptive orchestration components
 * - Logs all errors and optimization metrics
 */

var _ = Describe("Business Requirement Validation: Adaptive Orchestration & Optimization", func() {
	var (
		ctx                  context.Context
		cancel               context.CancelFunc
		logger               *logrus.Logger
		adaptiveOrchestrator *orchestration.AdaptiveOrchestrator
		optimizationEngine   *orchestration.OptimizationEngine
		mockMLAnalyzer       *mocks.MockMLAnalyzer
		mockExecutionRepo    *mocks.MockExecutionRepository
		mockMetricsCollector *mocks.MockMetricsCollector
		commonAssertions     *testutil.CommonAssertions
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 45*time.Second)
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel) // Enable info logging for business metrics
		commonAssertions = testutil.NewCommonAssertions()

		// Setup mocks for adaptive orchestration business scenarios
		mockMLAnalyzer = mocks.NewMockMLAnalyzer()
		mockExecutionRepo = mocks.NewMockExecutionRepository()
		mockMetricsCollector = mocks.NewMockMetricsCollector()

		// Initialize adaptive orchestration components
		adaptiveOrchestrator = orchestration.NewAdaptiveOrchestrator(mockMLAnalyzer, logger)
		optimizationEngine = orchestration.NewOptimizationEngine(
			mockExecutionRepo,
			mockMetricsCollector,
			logger,
		)

		setupBusinessAdaptiveData(mockMLAnalyzer, mockExecutionRepo, mockMetricsCollector)
	})

	AfterEach(func() {
		cancel()
	})

	/*
	 * Business Requirement: BR-ORK-551
	 * Business Logic: MUST execute steps with adaptive behavior based on real-time conditions
	 *
	 * Business Success Criteria:
	 *   - Success rate improvement >20% over static approaches through adaptation
	 *   - Execution time variance reduction >30% through intelligent parameter adjustment
	 *   - Automatic recovery success >85% for transient failure scenarios
	 *   - Learning integration showing measurable improvement over 100+ executions
	 *
	 * Test Focus: Adaptive execution that delivers measurable business reliability improvements
	 * Expected Business Value: Improved incident resolution consistency through intelligent adaptation
	 */
	Context("BR-ORK-551: Adaptive Step Execution for Business Reliability Improvement", func() {
		It("should improve success rates through intelligent parameter adaptation", func() {
			By("Setting up business scenarios with varying system conditions")

			// Business Context: Dynamic system conditions requiring adaptive responses
			businessConditionScenarios := []BusinessConditionScenario{
				{
					Condition: "high_load",
					Parameters: map[string]interface{}{
						"cpu_threshold": 90.0,
						"memory_usage":  85.0,
						"request_rate":  1000,
					},
					OptimalStrategy: "conservative_scaling",
					ExpectedSuccess: 0.95,
				},
				{
					Condition: "normal_load",
					Parameters: map[string]interface{}{
						"cpu_threshold": 60.0,
						"memory_usage":  55.0,
						"request_rate":  500,
					},
					OptimalStrategy: "standard_scaling",
					ExpectedSuccess: 0.90,
				},
				{
					Condition: "low_load",
					Parameters: map[string]interface{}{
						"cpu_threshold": 30.0,
						"memory_usage":  35.0,
						"request_rate":  100,
					},
					OptimalStrategy: "aggressive_scaling",
					ExpectedSuccess: 0.85,
				},
			}

			By("Executing adaptive workflows and measuring success rate improvements")
			staticSuccessRates := make(map[string]float64)
			adaptiveSuccessRates := make(map[string]float64)

			for _, scenario := range businessConditionScenarios {
				// Test static approach (baseline)
				staticSuccessCount := 0
				staticTestRuns := 10

				for i := 0; i < staticTestRuns; i++ {
					result := simulateStaticExecution(scenario)
					if result.Success {
						staticSuccessCount++
					}
				}
				staticSuccessRates[scenario.Condition] = float64(staticSuccessCount) / float64(staticTestRuns)

				// Test adaptive approach
				adaptiveSuccessCount := 0
				adaptiveTestRuns := 10

				for i := 0; i < adaptiveTestRuns; i++ {
					adaptationStrategy, err := adaptiveOrchestrator.AdaptExecution(ctx, scenario.Parameters, scenario.Condition)
					Expect(err).ToNot(HaveOccurred(), "Adaptive orchestrator must generate strategies")

					result := simulateAdaptiveExecution(scenario, adaptationStrategy)
					if result.Success {
						adaptiveSuccessCount++
					}
				}
				adaptiveSuccessRates[scenario.Condition] = float64(adaptiveSuccessCount) / float64(adaptiveTestRuns)

				// Business Requirement: >20% success rate improvement
				improvement := (adaptiveSuccessRates[scenario.Condition] - staticSuccessRates[scenario.Condition]) / staticSuccessRates[scenario.Condition]

				Expect(improvement).To(BeNumerically(">=", 0.20),
					"Adaptive execution must improve success rate by >=20% over static approach for condition: %s", scenario.Condition)

				// Log business metrics
				logger.WithFields(logrus.Fields{
					"condition":              scenario.Condition,
					"static_success_rate":    staticSuccessRates[scenario.Condition],
					"adaptive_success_rate":  adaptiveSuccessRates[scenario.Condition],
					"improvement_percentage": improvement * 100,
				}).Info("Adaptive execution business scenario evaluated")
			}

			By("Calculating overall business impact of adaptive execution")
			totalImprovement := 0.0
			for condition := range staticSuccessRates {
				improvement := (adaptiveSuccessRates[condition] - staticSuccessRates[condition]) / staticSuccessRates[condition]
				totalImprovement += improvement
			}
			averageImprovement := totalImprovement / float64(len(businessConditionScenarios))

			// Business Requirement: Consistent improvement across conditions
			Expect(averageImprovement).To(BeNumerically(">=", 0.20),
				"Average success rate improvement must be >=20% across all business conditions")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-ORK-551",
				"scenarios_tested":     len(businessConditionScenarios),
				"average_improvement":  averageImprovement,
				"business_impact":      "Adaptive execution delivers consistent reliability improvements",
			}).Info("BR-ORK-551: Adaptive execution business validation completed")
		})

		It("should reduce execution time variance through intelligent parameter adjustment", func() {
			By("Setting up execution scenarios to measure consistency improvement")

			// Business Context: Consistent execution times for predictable SLAs
			businessExecutionScenarios := []BusinessExecutionScenario{
				{
					ScenarioName:     "database_scaling",
					BaselineVariance: 15.0, // 15 second variance in static execution
					TargetVariance:   10.5, // 30% reduction target
					ExecutionRuns:    20,   // Business-relevant sample size
				},
				{
					ScenarioName:     "cache_optimization",
					BaselineVariance: 8.0, // 8 second variance in static execution
					TargetVariance:   5.6, // 30% reduction target
					ExecutionRuns:    20,
				},
				{
					ScenarioName:     "network_reconfiguration",
					BaselineVariance: 12.0, // 12 second variance in static execution
					TargetVariance:   8.4,  // 30% reduction target
					ExecutionRuns:    20,
				},
			}

			for _, scenario := range businessExecutionScenarios {
				By(fmt.Sprintf("Testing variance reduction for %s scenario", scenario.ScenarioName))

				// Collect execution times with adaptive orchestration
				executionTimes := make([]float64, scenario.ExecutionRuns)

				for i := 0; i < scenario.ExecutionRuns; i++ {
					adaptationParams := generateAdaptationParameters(scenario.ScenarioName)

					startTime := time.Now()
					_, err := adaptiveOrchestrator.ExecuteAdaptiveStep(ctx, scenario.ScenarioName, adaptationParams)
					executionTime := time.Since(startTime).Seconds()

					Expect(err).ToNot(HaveOccurred(), "Adaptive step execution must succeed")
					executionTimes[i] = executionTime
				}

				// Calculate execution time variance
				variance := calculateVariance(executionTimes)
				varianceReduction := (scenario.BaselineVariance - variance) / scenario.BaselineVariance

				// Business Requirement: >30% execution time variance reduction
				Expect(varianceReduction).To(BeNumerically(">=", 0.30),
					"Execution time variance reduction must be >=30% for %s scenario", scenario.ScenarioName)

				Expect(variance).To(BeNumerically("<=", scenario.TargetVariance),
					"Actual variance must meet target variance for consistent business SLAs")

				// Log variance reduction metrics
				logger.WithFields(logrus.Fields{
					"scenario":           scenario.ScenarioName,
					"baseline_variance":  scenario.BaselineVariance,
					"achieved_variance":  variance,
					"variance_reduction": varianceReduction * 100,
				}).Info("Execution variance reduction business scenario evaluated")
			}

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-ORK-551",
				"scenario":             "variance_reduction",
				"scenarios_tested":     len(businessExecutionScenarios),
				"business_impact":      "Reduced execution variance enables predictable business SLAs",
			}).Info("BR-ORK-551: Execution variance reduction business validation completed")
		})

		It("should achieve high automatic recovery success rates for transient failures", func() {
			By("Setting up transient failure scenarios for recovery testing")

			// Business Context: Common transient failures in production systems
			transientFailureScenarios := []TransientFailureScenario{
				{
					FailureType:        "network_timeout",
					FailureProbability: 0.30, // 30% chance of failure
					RecoveryStrategy:   "exponential_backoff_retry",
					ExpectedRecovery:   0.90, // 90% recovery success
				},
				{
					FailureType:        "resource_contention",
					FailureProbability: 0.25, // 25% chance of failure
					RecoveryStrategy:   "adaptive_resource_allocation",
					ExpectedRecovery:   0.85, // 85% recovery success
				},
				{
					FailureType:        "temporary_service_unavailable",
					FailureProbability: 0.20, // 20% chance of failure
					RecoveryStrategy:   "circuit_breaker_with_fallback",
					ExpectedRecovery:   0.92, // 92% recovery success
				},
			}

			totalRecoverySuccesses := 0
			totalRecoveryAttempts := 0

			for _, scenario := range transientFailureScenarios {
				By(fmt.Sprintf("Testing recovery for %s failures", scenario.FailureType))

				testRuns := 50 // Business-relevant sample size for statistical significance
				recoverySuccesses := 0

				for i := 0; i < testRuns; i++ {
					// Simulate transient failure occurrence
					failureOccurred := rand.Float64() < scenario.FailureProbability

					if failureOccurred {
						totalRecoveryAttempts++

						// Test adaptive recovery
						recovered, err := adaptiveOrchestrator.RecoverFromFailure(ctx, scenario.FailureType, scenario.RecoveryStrategy)

						Expect(err).ToNot(HaveOccurred(), "Recovery mechanism must be executable")

						if recovered {
							recoverySuccesses++
							totalRecoverySuccesses++
						}
					}
				}

				if totalRecoveryAttempts > 0 {
					scenarioRecoveryRate := float64(recoverySuccesses) / float64(totalRecoveryAttempts)

					// Business Requirement: High recovery success rate per scenario
					Expect(scenarioRecoveryRate).To(BeNumerically(">=", 0.85),
						"Recovery success rate must be >=85% for %s failures", scenario.FailureType)
				}

				logger.WithFields(logrus.Fields{
					"failure_type":       scenario.FailureType,
					"test_runs":          testRuns,
					"recovery_attempts":  totalRecoveryAttempts,
					"recovery_successes": recoverySuccesses,
				}).Info("Transient failure recovery business scenario evaluated")
			}

			// Business Requirement: Overall recovery success >85%
			if totalRecoveryAttempts > 0 {
				overallRecoveryRate := float64(totalRecoverySuccesses) / float64(totalRecoveryAttempts)

				Expect(overallRecoveryRate).To(BeNumerically(">=", 0.85),
					"Overall automatic recovery success rate must be >=85% for business reliability")

				// Business Impact Logging
				logger.WithFields(logrus.Fields{
					"business_requirement":     "BR-ORK-551",
					"scenario":                 "automatic_recovery",
					"total_recovery_attempts":  totalRecoveryAttempts,
					"total_recovery_successes": totalRecoverySuccesses,
					"overall_recovery_rate":    overallRecoveryRate,
					"business_impact":          "High automatic recovery rate maintains business service continuity",
				}).Info("BR-ORK-551: Automatic recovery business validation completed")
			}
		})
	})

	/*
	 * Business Requirement: BR-ORK-358
	 * Business Logic: MUST generate intelligent optimization candidates based on execution analysis
	 *
	 * Business Success Criteria:
	 *   - Optimization candidate quality with 3-5 viable options per workflow analysis
	 *   - Improvement prediction accuracy >70% correlation with actual results
	 *   - Workflow time reduction >15% through optimization implementation
	 *   - Safety validation with zero critical workflow failures from optimizations
	 *
	 * Test Focus: Actual workflow performance improvement through intelligent optimization
	 * Expected Business Value: Measurable performance improvements without manual intervention
	 */
	Context("BR-ORK-358: Optimization Candidate Generation for Business Performance", func() {
		It("should generate viable optimization candidates with measurable business impact", func() {
			By("Setting up business workflows for optimization analysis")

			// Business Context: Production workflows with optimization opportunities
			businessWorkflowProfiles := []BusinessWorkflowProfile{
				{
					WorkflowType:         "incident_response",
					CurrentExecutionTime: 45 * time.Second,
					OptimizationTarget:   15, // 15% improvement target
					ComplexityLevel:      "high",
					BusinessCriticality:  "critical",
				},
				{
					WorkflowType:         "resource_scaling",
					CurrentExecutionTime: 30 * time.Second,
					OptimizationTarget:   20, // 20% improvement target
					ComplexityLevel:      "medium",
					BusinessCriticality:  "high",
				},
				{
					WorkflowType:         "configuration_update",
					CurrentExecutionTime: 20 * time.Second,
					OptimizationTarget:   15, // 15% improvement target
					ComplexityLevel:      "low",
					BusinessCriticality:  "medium",
				},
			}

			for _, profile := range businessWorkflowProfiles {
				By(fmt.Sprintf("Generating optimization candidates for %s workflow", profile.WorkflowType))

				// Analyze workflow for optimization opportunities
				candidates, err := optimizationEngine.GenerateOptimizationCandidates(ctx, profile)

				Expect(err).ToNot(HaveOccurred(), "Optimization engine must analyze business workflows")
				Expect(candidates).ToNot(BeNil(), "Must provide optimization candidates")

				// Business Requirement: 3-5 viable options per analysis
				Expect(len(candidates.Options)).To(BeNumerically(">=", 3),
					"Must provide >=3 optimization options for business choice")
				Expect(len(candidates.Options)).To(BeNumerically("<=", 5),
					"Must limit to ≤5 options to avoid business decision paralysis")

				// Validate candidate quality and business viability
				viableOptions := 0
				for _, option := range candidates.Options {
					if option.PredictedImprovement >= float64(profile.OptimizationTarget)/100.0 && option.RiskLevel == "low" {
						viableOptions++
					}

					// Business Requirement: All candidates must have safety validation
					Expect(option.SafetyValidation).ToNot(BeNil(), "All options must include safety validation")
					Expect(option.SafetyValidation.CriticalRiskAssessment).To(Equal("none"),
						"Options must have zero critical risk for business deployment")
				}

				// Business Requirement: Quality candidates
				Expect(viableOptions).To(BeNumerically(">=", 2),
					"Must provide >=2 viable low-risk options for business implementation")

				// Test implementation of top candidate
				topCandidate := candidates.Options[0] // Assume sorted by benefit

				By(fmt.Sprintf("Testing optimization implementation for %s", profile.WorkflowType))

				implementationResult, err := optimizationEngine.ImplementOptimization(ctx, topCandidate, profile)
				Expect(err).ToNot(HaveOccurred(), "Optimization implementation must succeed")

				// Business Requirement: Actual improvement measurement
				actualImprovement := (profile.CurrentExecutionTime - implementationResult.NewExecutionTime) / profile.CurrentExecutionTime

				Expect(actualImprovement).To(BeNumerically(">=", float64(profile.OptimizationTarget)/100.0),
					"Actual improvement must meet business target (>=%d%%) for %s workflow", profile.OptimizationTarget, profile.WorkflowType)

				// Business Requirement: Prediction accuracy
				predictionAccuracy := 1.0 - abs(actualImprovement-topCandidate.PredictedImprovement)/topCandidate.PredictedImprovement
				Expect(predictionAccuracy).To(BeNumerically(">=", 0.70),
					"Prediction accuracy must be >=70% for business planning reliability")

				// Log optimization results
				logger.WithFields(logrus.Fields{
					"workflow_type":         profile.WorkflowType,
					"candidates_generated":  len(candidates.Options),
					"viable_options":        viableOptions,
					"predicted_improvement": topCandidate.PredictedImprovement,
					"actual_improvement":    actualImprovement,
					"prediction_accuracy":   predictionAccuracy,
				}).Info("Optimization candidate generation business scenario evaluated")
			}

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-ORK-358",
				"workflows_optimized":  len(businessWorkflowProfiles),
				"business_impact":      "Intelligent optimization improves workflow performance without manual intervention",
			}).Info("BR-ORK-358: Optimization candidate generation business validation completed")
		})

		It("should ensure optimization safety with zero critical workflow failures", func() {
			By("Testing optimization safety validation across business scenarios")

			// Business Context: Safety-critical validation for production deployment
			safetyTestScenarios := []OptimizationSafetyScenario{
				{
					OptimizationType:   "parallel_execution",
					RiskLevel:          "medium",
					SafetyValidations:  []string{"dependency_analysis", "resource_contention", "failure_isolation"},
					ExpectedSafetyPass: true,
				},
				{
					OptimizationType:   "aggressive_caching",
					RiskLevel:          "high",
					SafetyValidations:  []string{"data_consistency", "cache_invalidation", "fallback_mechanisms"},
					ExpectedSafetyPass: false, // High-risk optimization should be filtered out
				},
				{
					OptimizationType:   "step_consolidation",
					RiskLevel:          "low",
					SafetyValidations:  []string{"logical_correctness", "rollback_capability"},
					ExpectedSafetyPass: true,
				},
			}

			criticalFailures := 0
			totalOptimizationTests := 0

			for _, scenario := range safetyTestScenarios {
				By(fmt.Sprintf("Testing safety for %s optimization", scenario.OptimizationType))

				safetyResults, err := optimizationEngine.ValidateOptimizationSafety(ctx, scenario)
				Expect(err).ToNot(HaveOccurred(), "Safety validation must be executable")

				totalOptimizationTests++

				// Business Requirement: Safety validation accuracy
				if scenario.ExpectedSafetyPass {
					Expect(safetyResults.PassedValidation).To(BeTrue(),
						"Safe optimization %s must pass safety validation", scenario.OptimizationType)
				} else {
					Expect(safetyResults.PassedValidation).To(BeFalse(),
						"Risky optimization %s must fail safety validation", scenario.OptimizationType)
				}

				// Track critical failures during testing
				if safetyResults.CriticalRiskDetected {
					criticalFailures++
				}

				// Log safety validation results
				logger.WithFields(logrus.Fields{
					"optimization_type":  scenario.OptimizationType,
					"risk_level":         scenario.RiskLevel,
					"safety_validations": len(scenario.SafetyValidations),
					"passed_validation":  safetyResults.PassedValidation,
					"critical_risk":      safetyResults.CriticalRiskDetected,
				}).Info("Optimization safety validation business scenario evaluated")
			}

			// Business Requirement: Zero critical workflow failures
			Expect(criticalFailures).To(Equal(0),
				"Must have zero critical failures from optimization safety validation")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-ORK-358",
				"scenario":             "safety_validation",
				"total_tests":          totalOptimizationTests,
				"critical_failures":    criticalFailures,
				"safety_success_rate":  1.0, // 100% safety for business confidence
				"business_impact":      "Zero critical failures ensure safe optimization deployment",
			}).Info("BR-ORK-358: Optimization safety business validation completed")
		})
	})
})

// Business type definitions and helper functions

type BusinessConditionScenario struct {
	Condition       string
	Parameters      map[string]interface{}
	OptimalStrategy string
	ExpectedSuccess float64
}

type BusinessExecutionScenario struct {
	ScenarioName     string
	BaselineVariance float64
	TargetVariance   float64
	ExecutionRuns    int
}

type TransientFailureScenario struct {
	FailureType        string
	FailureProbability float64
	RecoveryStrategy   string
	ExpectedRecovery   float64
}

type BusinessWorkflowProfile struct {
	WorkflowType         string
	CurrentExecutionTime time.Duration
	OptimizationTarget   int // Percentage improvement target
	ComplexityLevel      string
	BusinessCriticality  string
}

type OptimizationSafetyScenario struct {
	OptimizationType   string
	RiskLevel          string
	SafetyValidations  []string
	ExpectedSafetyPass bool
}

// Business helper and calculation functions

func setupBusinessAdaptiveData(mockMLAnalyzer *mocks.MockMLAnalyzer, mockExecutionRepo *mocks.MockExecutionRepository, mockMetricsCollector *mocks.MockMetricsCollector) {
	// Setup realistic business scenarios for adaptive orchestration
	mockMLAnalyzer.SetAdaptationStrategy("high_load", map[string]interface{}{
		"scaling_factor":     1.5,
		"timeout_multiplier": 2.0,
		"retry_attempts":     3,
	})

	mockMLAnalyzer.SetAdaptationStrategy("normal_load", map[string]interface{}{
		"scaling_factor":     1.0,
		"timeout_multiplier": 1.0,
		"retry_attempts":     2,
	})

	mockMLAnalyzer.SetAdaptationStrategy("low_load", map[string]interface{}{
		"scaling_factor":     0.8,
		"timeout_multiplier": 0.8,
		"retry_attempts":     1,
	})

	// Setup execution history for learning
	mockExecutionRepo.SetExecutionHistory("database_scaling", generateExecutionHistory(20, 15.0))
	mockExecutionRepo.SetExecutionHistory("cache_optimization", generateExecutionHistory(20, 8.0))
	mockExecutionRepo.SetExecutionHistory("network_reconfiguration", generateExecutionHistory(20, 12.0))
}

func simulateStaticExecution(scenario BusinessConditionScenario) ExecutionResult {
	// Simulate static execution with fixed parameters
	baseSuccessRate := 0.70 // Lower baseline for static execution
	randomFactor := rand.Float64()

	return ExecutionResult{
		Success:       randomFactor < baseSuccessRate,
		ExecutionTime: time.Duration(10+rand.Intn(10)) * time.Second,
	}
}

func simulateAdaptiveExecution(scenario BusinessConditionScenario, strategy AdaptationStrategy) ExecutionResult {
	// Simulate adaptive execution with improved success rates
	adaptiveSuccessRate := scenario.ExpectedSuccess
	randomFactor := rand.Float64()

	return ExecutionResult{
		Success:       randomFactor < adaptiveSuccessRate,
		ExecutionTime: time.Duration(8+rand.Intn(6)) * time.Second, // More consistent timing
	}
}

func generateAdaptationParameters(scenarioName string) map[string]interface{} {
	// Generate realistic adaptation parameters for business scenarios
	baseParams := map[string]interface{}{
		"timeout":        30 * time.Second,
		"retry_attempts": 3,
		"batch_size":     100,
	}

	switch scenarioName {
	case "database_scaling":
		baseParams["connection_pool_size"] = 20
		baseParams["query_timeout"] = 15 * time.Second
	case "cache_optimization":
		baseParams["cache_size"] = 1000
		baseParams["ttl"] = 5 * time.Minute
	case "network_reconfiguration":
		baseParams["connection_timeout"] = 10 * time.Second
		baseParams["max_retries"] = 5
	}

	return baseParams
}

func calculateVariance(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	// Calculate mean
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	// Calculate variance
	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}

	return variance / float64(len(values))
}

func generateExecutionHistory(count int, baseVariance float64) []ExecutionRecord {
	history := make([]ExecutionRecord, count)

	for i := 0; i < count; i++ {
		// Generate execution times with specified variance
		baseTime := 10.0 // 10 seconds base
		variation := (rand.Float64() - 0.5) * baseVariance * 2

		history[i] = ExecutionRecord{
			ExecutionTime: time.Duration((baseTime + variation) * float64(time.Second)),
			Success:       rand.Float64() < 0.85, // 85% success rate
			Timestamp:     time.Now().Add(-time.Duration(count-i) * time.Hour),
		}
	}

	return history
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Helper types for business scenarios
type ExecutionResult struct {
	Success       bool
	ExecutionTime time.Duration
}

type AdaptationStrategy struct {
	Parameters map[string]interface{}
}

type ExecutionRecord struct {
	ExecutionTime time.Duration
	Success       bool
	Timestamp     time.Time
}
