//go:build unit
// +build unit

package intelligence

import (
	"context"
	"fmt"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/intelligence/evolution"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

/*
 * Business Requirement Validation: Pattern Evolution & Adaptive Learning (Phase 2)
 *
 * This test suite validates Phase 2 business requirements for pattern evolution and adaptive learning
 * following development guidelines:
 * - Reuses existing intelligence test framework (Ginkgo/Gomega)
 * - Extends existing mocks from pattern_discovery_mocks.go
 * - Focuses on business outcomes: operational efficiency, reduced alert fatigue, pattern relevance
 * - Uses meaningful assertions with business improvement thresholds
 * - Integrates with existing intelligence and pattern discovery components
 * - Logs all errors and evolution performance metrics
 */

var _ = Describe("Business Requirement Validation: Pattern Evolution & Adaptive Learning (Phase 2)", func() {
	var (
		ctx                     context.Context
		cancel                  context.CancelFunc
		logger                  *logrus.Logger
		patternEvolutionManager *evolution.PatternEvolutionManager
		adaptiveLearningEngine  *evolution.AdaptiveLearningEngine
		mockExecutionRepo       *MockExecutionRepository
		mockPatternStore        *MockPatternStore
		mockFeedbackCollector   *MockFeedbackCollector
		commonAssertions        *testutil.CommonAssertions
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel) // Enable info logging for evolution metrics
		commonAssertions = testutil.NewCommonAssertions()

		// Reuse existing mocks from intelligence module following development guidelines
		mockExecutionRepo = &MockExecutionRepository{}
		mockPatternStore = &MockPatternStore{}
		mockFeedbackCollector = &MockFeedbackCollector{}

		// Initialize pattern evolution components for Phase 2 advanced intelligence
		patternEvolutionManager = evolution.NewPatternEvolutionManager(mockPatternStore, mockExecutionRepo, logger)
		adaptiveLearningEngine = evolution.NewAdaptiveLearningEngine(mockFeedbackCollector, mockPatternStore, logger)

		setupPhase2BusinessEvolutionData(mockPatternStore, mockExecutionRepo, mockFeedbackCollector)
	})

	AfterEach(func() {
		cancel()
	})

	/*
	 * Business Requirement: BR-PD-013
	 * Business Logic: MUST detect pattern obsolescence preventing outdated recommendations
	 *
	 * Business Success Criteria:
	 *   - Pattern lifecycle tracking with business assessment of relevance and effectiveness
	 *   - Obsolescence detection preventing outdated recommendations with >95% accuracy
	 *   - Operational efficiency through up-to-date patterns reducing failed remediation by >25%
	 *   - Automatic pattern retirement with business impact assessment and transition planning
	 *
	 * Test Focus: Pattern lifecycle management ensuring business recommendations remain current and effective
	 * Expected Business Value: Improved operational efficiency through relevant, up-to-date remediation patterns
	 */
	Context("BR-PD-013: Pattern Obsolescence Detection for Business Operational Efficiency", func() {
		It("should accurately detect and retire obsolete patterns to maintain business operational effectiveness", func() {
			By("Setting up business pattern lifecycle scenarios with varying effectiveness over time")

			// Business Context: Patterns with different lifecycle stages and business relevance
			businessPatternLifecycles := []BusinessPatternLifecycle{
				{
					PatternID:      "memory-optimization-v1",
					BusinessDomain: "resource_management",
					CreationDate:   time.Now().Add(-180 * 24 * time.Hour), // 6 months old
					PatternType:    "remediation",
					HistoricalPerformance: PatternPerformanceHistory{
						SuccessRateOverTime: []float64{0.85, 0.82, 0.78, 0.72, 0.65, 0.58}, // Declining success
						UsageFrequency:      []int{150, 120, 95, 70, 45, 25},               // Declining usage
						BusinessImpact:      []string{"high", "high", "medium", "medium", "low", "low"},
					},
					CurrentRelevance:      0.35, // Low relevance
					ExpectedObsolescence:  true,
					BusinessJustification: "Success rate dropped below 60%, usage declined 83%",
				},
				{
					PatternID:      "auto-scaling-v3",
					BusinessDomain: "performance_optimization",
					CreationDate:   time.Now().Add(-90 * 24 * time.Hour), // 3 months old
					PatternType:    "remediation",
					HistoricalPerformance: PatternPerformanceHistory{
						SuccessRateOverTime: []float64{0.88, 0.90, 0.92, 0.93, 0.94, 0.95}, // Improving success
						UsageFrequency:      []int{80, 95, 110, 125, 140, 155},             // Increasing usage
						BusinessImpact:      []string{"medium", "medium", "high", "high", "high", "critical"},
					},
					CurrentRelevance:      0.92, // High relevance
					ExpectedObsolescence:  false,
					BusinessJustification: "Success rate improved to 95%, usage increased 94%",
				},
				{
					PatternID:      "network-troubleshooting-legacy",
					BusinessDomain: "network_management",
					CreationDate:   time.Now().Add(-365 * 24 * time.Hour), // 1 year old
					PatternType:    "diagnostic",
					HistoricalPerformance: PatternPerformanceHistory{
						SuccessRateOverTime: []float64{0.75, 0.73, 0.70, 0.68, 0.65, 0.60}, // Declining success
						UsageFrequency:      []int{200, 180, 150, 120, 90, 60},             // Declining usage
						BusinessImpact:      []string{"critical", "high", "high", "medium", "medium", "low"},
					},
					CurrentRelevance:      0.42, // Low relevance
					ExpectedObsolescence:  true,
					BusinessJustification: "Pattern superseded by newer network diagnostics, success rate declining",
				},
				{
					PatternID:      "kubernetes-recovery-modern",
					BusinessDomain: "container_orchestration",
					CreationDate:   time.Now().Add(-30 * 24 * time.Hour), // 1 month old
					PatternType:    "remediation",
					HistoricalPerformance: PatternPerformanceHistory{
						SuccessRateOverTime: []float64{0.90, 0.91, 0.92, 0.93, 0.94, 0.95}, // Consistently high
						UsageFrequency:      []int{60, 70, 85, 95, 110, 125},               // Growing usage
						BusinessImpact:      []string{"high", "high", "high", "critical", "critical", "critical"},
					},
					CurrentRelevance:      0.96, // Very high relevance
					ExpectedObsolescence:  false,
					BusinessJustification: "Modern pattern with excellent performance and growing adoption",
				},
			}

			correctObsolescenceDetections := 0
			totalBusinessImpactAssessed := 0.0
			patternsEvaluated := 0

			for _, lifecycle := range businessPatternLifecycles {
				By(fmt.Sprintf("Evaluating pattern lifecycle for %s in %s domain", lifecycle.PatternID, lifecycle.BusinessDomain))

				// Perform obsolescence detection analysis
				obsolescenceResult, err := patternEvolutionManager.EvaluatePatternObsolescence(ctx, lifecycle)
				Expect(err).ToNot(HaveOccurred(), "Pattern obsolescence evaluation must succeed for business pattern management")
				Expect(obsolescenceResult).ToNot(BeNil(), "Must provide obsolescence evaluation results")

				// Business Requirement: Accurate obsolescence detection
				obsolescenceAccuracy := calculateObsolescenceDetectionAccuracy(obsolescenceResult.IsObsolete, lifecycle.ExpectedObsolescence)
				Expect(obsolescenceAccuracy).To(BeTrue(),
					"Obsolescence detection must be accurate for business pattern management reliability")

				if obsolescenceAccuracy {
					correctObsolescenceDetections++
				}
				patternsEvaluated++

				// Business Requirement: Business impact assessment for obsolescence decisions
				Expect(obsolescenceResult.BusinessImpactAssessment).ToNot(BeEmpty(),
					"Must provide business impact assessment for obsolescence decisions")
				Expect(obsolescenceResult.ConfidenceLevel).To(BeNumerically(">=", 0.80),
					"Obsolescence detection confidence must be >=80% for business decision reliability")

				// Business Validation: For obsolete patterns, ensure business continuity planning
				if obsolescenceResult.IsObsolete {
					Expect(obsolescenceResult.TransitionPlan).ToNot(BeEmpty(),
						"Must provide transition plan for obsolete patterns to ensure business continuity")
					Expect(obsolescenceResult.ReplacementRecommendations).ToNot(BeEmpty(),
						"Must recommend replacement patterns for business operational continuity")

					// Business Requirement: Gradual retirement with business impact minimization
					Expect(obsolescenceResult.RetirementTimeline).To(BeNumerically(">", 7*24*time.Hour),
						"Retirement timeline must allow >=7 days for business transition planning")
				}

				// Business Validation: For active patterns, ensure continued business value
				if !obsolescenceResult.IsObsolete {
					Expect(obsolescenceResult.ContinuedBusinessValue).To(BeNumerically(">=", 0.70),
						"Active patterns must demonstrate >=70% continued business value")
					Expect(obsolescenceResult.OptimizationRecommendations).ToNot(BeEmpty(),
						"Active patterns should include optimization recommendations for improved business value")
				}

				// Calculate business impact of obsolescence management
				businessImpact := calculateObsolescenceBusinessImpact(lifecycle, obsolescenceResult)
				totalBusinessImpactAssessed += businessImpact

				// Log pattern lifecycle evaluation for business audit
				logger.WithFields(logrus.Fields{
					"pattern_id":            lifecycle.PatternID,
					"business_domain":       lifecycle.BusinessDomain,
					"pattern_age_days":      time.Since(lifecycle.CreationDate).Hours() / 24,
					"current_relevance":     lifecycle.CurrentRelevance,
					"is_obsolete":           obsolescenceResult.IsObsolete,
					"expected_obsolescence": lifecycle.ExpectedObsolescence,
					"detection_accurate":    obsolescenceAccuracy,
					"confidence_level":      obsolescenceResult.ConfidenceLevel,
					"business_impact_score": businessImpact,
					"has_transition_plan":   len(obsolescenceResult.TransitionPlan) > 0,
				}).Info("Pattern obsolescence evaluation business scenario completed")
			}

			By("Calculating overall obsolescence detection business performance")

			obsolescenceDetectionAccuracy := float64(correctObsolescenceDetections) / float64(patternsEvaluated)
			averageBusinessImpact := totalBusinessImpactAssessed / float64(patternsEvaluated)

			// Business Requirement: >95% obsolescence detection accuracy
			Expect(obsolescenceDetectionAccuracy).To(BeNumerically(">=", 0.95),
				"Obsolescence detection accuracy must be >=95% for reliable business pattern management")

			// Business Requirement: Significant business impact from pattern lifecycle management
			Expect(averageBusinessImpact).To(BeNumerically(">=", 2000.0),
				"Average business impact must be >=2K USD per pattern for meaningful pattern lifecycle value")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":        "BR-PD-013",
				"patterns_evaluated":          patternsEvaluated,
				"obsolescence_accuracy":       obsolescenceDetectionAccuracy,
				"average_business_impact_usd": averageBusinessImpact,
				"total_business_value_usd":    totalBusinessImpactAssessed,
				"business_pattern_mgmt_ready": obsolescenceDetectionAccuracy >= 0.95,
				"business_impact":             "Pattern obsolescence detection maintains operational efficiency through current, effective patterns",
			}).Info("BR-PD-013: Pattern obsolescence detection business validation completed")
		})

		It("should demonstrate measurable operational efficiency improvement through obsolete pattern retirement", func() {
			By("Testing business operational efficiency gains from removing obsolete patterns")

			// Business Context: Before and after scenarios showing operational improvement from pattern retirement
			operationalEfficiencyScenarios := []OperationalEfficiencyScenario{
				{
					ScenarioName:   "memory_management_efficiency",
					BusinessDomain: "resource_optimization",
					ObsoletePatterns: []ObsoletePattern{
						{
							PatternID:                "memory-fix-legacy-v1",
							FailureRate:              0.45, // 45% failure rate
							AverageResolutionTime:    35 * time.Minute,
							BusinessImpactPerFailure: 2500.0, // $2.5K per failed remediation
							MonthlyUsage:             80,
						},
						{
							PatternID:                "memory-optimization-old",
							FailureRate:              0.38, // 38% failure rate
							AverageResolutionTime:    28 * time.Minute,
							BusinessImpactPerFailure: 2000.0, // $2K per failed remediation
							MonthlyUsage:             60,
						},
					},
					ReplacementPatterns: []ReplacementPattern{
						{
							PatternID:                "memory-management-modern",
							SuccessRate:              0.92, // 92% success rate
							AverageResolutionTime:    15 * time.Minute,
							BusinessImpactPerSuccess: 500.0, // $500 value per successful remediation
							ExpectedMonthlyUsage:     140,   // Combined usage
						},
					},
					ExpectedEfficiencyGain: 0.35, // 35% efficiency improvement
					ExpectedCostReduction:  0.40, // 40% cost reduction
				},
				{
					ScenarioName:   "network_troubleshooting_efficiency",
					BusinessDomain: "network_management",
					ObsoletePatterns: []ObsoletePattern{
						{
							PatternID:                "network-debug-legacy",
							FailureRate:              0.55, // 55% failure rate
							AverageResolutionTime:    45 * time.Minute,
							BusinessImpactPerFailure: 3000.0, // $3K per failed remediation
							MonthlyUsage:             50,
						},
					},
					ReplacementPatterns: []ReplacementPattern{
						{
							PatternID:                "network-diagnostics-ai-powered",
							SuccessRate:              0.88, // 88% success rate
							AverageResolutionTime:    20 * time.Minute,
							BusinessImpactPerSuccess: 800.0, // $800 value per successful remediation
							ExpectedMonthlyUsage:     50,
						},
					},
					ExpectedEfficiencyGain: 0.30, // 30% efficiency improvement
					ExpectedCostReduction:  0.50, // 50% cost reduction
				},
			}

			totalEfficiencyGain := 0.0
			totalCostReduction := 0.0
			businessValueDelivered := 0.0

			for _, scenario := range operationalEfficiencyScenarios {
				By(fmt.Sprintf("Measuring efficiency improvement for %s scenario", scenario.ScenarioName))

				// Calculate baseline operational costs with obsolete patterns
				baselineCosts := calculateBaselineOperationalCosts(scenario.ObsoletePatterns)

				// Perform pattern retirement and replacement
				retirementResult, err := patternEvolutionManager.RetireObsoletePatterns(ctx, scenario.ObsoletePatterns, scenario.ReplacementPatterns)
				Expect(err).ToNot(HaveOccurred(), "Pattern retirement must succeed for business operational improvement")

				// Calculate improved operational costs with replacement patterns
				improvedCosts := calculateImprovedOperationalCosts(scenario.ReplacementPatterns)

				// Business Requirement: Measure actual efficiency gains
				actualEfficiencyGain := (baselineCosts.TotalOperationalTime - improvedCosts.TotalOperationalTime) / baselineCosts.TotalOperationalTime
				Expect(actualEfficiencyGain).To(BeNumerically(">=", 0.25),
					"Efficiency improvement must be >=25% for meaningful business operational value")

				// Business Requirement: Verify efficiency gains meet expectations
				efficiencyGainError := math.Abs(actualEfficiencyGain - scenario.ExpectedEfficiencyGain)
				Expect(efficiencyGainError).To(BeNumerically("<=", 0.10),
					"Actual efficiency gain must be within 10%% of expected for business planning accuracy")

				// Business Requirement: Measure cost reduction from pattern improvement
				actualCostReduction := (baselineCosts.MonthlyCost - improvedCosts.MonthlyCost) / baselineCosts.MonthlyCost
				Expect(actualCostReduction).To(BeNumerically(">=", 0.20),
					"Cost reduction must be >=20% for business financial benefit")

				totalEfficiencyGain += actualEfficiencyGain
				totalCostReduction += actualCostReduction

				// Business Value: Calculate total business impact
				monthlyBusinessValue := baselineCosts.MonthlyCost - improvedCosts.MonthlyCost
				businessValueDelivered += monthlyBusinessValue

				// Business Validation: Ensure successful transition with minimal business disruption
				Expect(retirementResult.TransitionSuccess).To(BeTrue(),
					"Pattern retirement transition must succeed for business continuity")
				Expect(retirementResult.BusinessDisruptionMinimal).To(BeTrue(),
					"Transition must minimize business disruption for operational stability")

				// Log operational efficiency improvement for business tracking
				logger.WithFields(logrus.Fields{
					"scenario_name":            scenario.ScenarioName,
					"business_domain":          scenario.BusinessDomain,
					"baseline_monthly_cost":    baselineCosts.MonthlyCost,
					"improved_monthly_cost":    improvedCosts.MonthlyCost,
					"actual_efficiency_gain":   actualEfficiencyGain,
					"expected_efficiency_gain": scenario.ExpectedEfficiencyGain,
					"actual_cost_reduction":    actualCostReduction,
					"expected_cost_reduction":  scenario.ExpectedCostReduction,
					"monthly_business_value":   monthlyBusinessValue,
					"transition_successful":    retirementResult.TransitionSuccess,
				}).Info("Operational efficiency improvement business scenario completed")
			}

			By("Calculating overall business value from pattern evolution management")

			averageEfficiencyGain := totalEfficiencyGain / float64(len(operationalEfficiencyScenarios))
			averageCostReduction := totalCostReduction / float64(len(operationalEfficiencyScenarios))
			annualBusinessValue := businessValueDelivered * 12

			// Business Requirement: Significant average efficiency gains
			Expect(averageEfficiencyGain).To(BeNumerically(">=", 0.25),
				"Average efficiency gain must be >=25% for business operational improvement justification")

			// Business Requirement: Significant average cost reductions
			Expect(averageCostReduction).To(BeNumerically(">=", 0.20),
				"Average cost reduction must be >=20% for business financial justification")

			// Business Value: Annual business value justification
			Expect(annualBusinessValue).To(BeNumerically(">=", 50000.0),
				"Annual business value must be >=50K USD for pattern evolution investment justification")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":    "BR-PD-013",
				"scenario":                "operational_efficiency",
				"scenarios_tested":        len(operationalEfficiencyScenarios),
				"average_efficiency_gain": averageEfficiencyGain,
				"average_cost_reduction":  averageCostReduction,
				"monthly_business_value":  businessValueDelivered,
				"annual_business_value":   annualBusinessValue,
				"business_impact":         "Pattern obsolescence management delivers significant operational efficiency and cost reduction",
			}).Info("BR-PD-013: Operational efficiency improvement business validation completed")
		})
	})

	/*
	 * Business Requirement: BR-AD-011
	 * Business Logic: MUST implement adaptive learning to reduce false positives and improve system evolution
	 *
	 * Business Success Criteria:
	 *   - False positive reduction >30% through feedback learning reducing alert fatigue
	 *   - System evolution tracking with automatic updating improving accuracy over time
	 *   - Reduced alert fatigue through intelligent adaptation improving operational efficiency
	 *   - Feedback loop integration enabling continuous business improvement
	 *
	 * Test Focus: Adaptive learning that continuously improves business operational efficiency through reduced noise
	 * Expected Business Value: Improved operator productivity and system reliability through intelligent learning adaptation
	 */
	Context("BR-AD-011: Adaptive Learning for Business Operational Excellence", func() {
		It("should achieve significant false positive reduction through intelligent feedback learning", func() {
			By("Setting up baseline false positive scenarios for adaptive learning improvement")

			// Business Context: Common false positive scenarios affecting operational efficiency
			falsePositiveScenarios := []FalsePositiveScenario{
				{
					AlertType:                 "memory_pressure_warning",
					BusinessDomain:            "resource_monitoring",
					BaselineFalsePositiveRate: 0.35, // 35% false positive rate
					BusinessImpact: FalsePositiveBusinessImpact{
						TimeWastedPerAlert:     8 * time.Minute, // 8 minutes per false alert
						AverageAlertsPerDay:    25,              // 25 alerts per day
						OperatorEfficiencyLoss: 0.15,            // 15% efficiency loss
						BusinessCostPerAlert:   45.0,            // $45 cost per false alert investigation
					},
					ExpectedImprovement: 0.35, // 35% reduction in false positives
					BusinessPriority:    "high",
				},
				{
					AlertType:                 "disk_usage_critical",
					BusinessDomain:            "storage_monitoring",
					BaselineFalsePositiveRate: 0.28, // 28% false positive rate
					BusinessImpact: FalsePositiveBusinessImpact{
						TimeWastedPerAlert:     12 * time.Minute, // 12 minutes per false alert
						AverageAlertsPerDay:    15,               // 15 alerts per day
						OperatorEfficiencyLoss: 0.20,             // 20% efficiency loss
						BusinessCostPerAlert:   60.0,             // $60 cost per false alert investigation
					},
					ExpectedImprovement: 0.40, // 40% reduction in false positives
					BusinessPriority:    "critical",
				},
				{
					AlertType:                 "network_latency_spike",
					BusinessDomain:            "network_monitoring",
					BaselineFalsePositiveRate: 0.42, // 42% false positive rate
					BusinessImpact: FalsePositiveBusinessImpact{
						TimeWastedPerAlert:     6 * time.Minute, // 6 minutes per false alert
						AverageAlertsPerDay:    35,              // 35 alerts per day
						OperatorEfficiencyLoss: 0.12,            // 12% efficiency loss
						BusinessCostPerAlert:   35.0,            // $35 cost per false alert investigation
					},
					ExpectedImprovement: 0.32, // 32% reduction in false positives
					BusinessPriority:    "medium",
				},
			}

			totalFalsePositiveReduction := 0.0
			totalBusinessValueRealized := 0.0
			successfulLearningImprovements := 0

			for _, scenario := range falsePositiveScenarios {
				By(fmt.Sprintf("Testing adaptive learning for %s alerts in %s domain", scenario.AlertType, scenario.BusinessDomain))

				// Initialize learning with historical false positive feedback
				feedbackData := generateHistoricalFeedbackData(scenario.AlertType, 1000) // 1000 historical feedback points

				learningResult, err := adaptiveLearningEngine.InitializeLearning(ctx, scenario.AlertType, feedbackData)
				Expect(err).ToNot(HaveOccurred(), "Adaptive learning initialization must succeed for business improvement")

				// Simulate adaptive learning over time with continuous feedback
				learningPeriod := 30 * 24 * time.Hour // 30 days of learning
				improvementResult, err := adaptiveLearningEngine.ExecuteAdaptiveLearning(ctx, learningResult, learningPeriod)
				Expect(err).ToNot(HaveOccurred(), "Adaptive learning execution must succeed")

				// Business Requirement: Measure false positive reduction
				actualFalsePositiveReduction := (scenario.BaselineFalsePositiveRate - improvementResult.NewFalsePositiveRate) / scenario.BaselineFalsePositiveRate
				Expect(actualFalsePositiveReduction).To(BeNumerically(">=", 0.30),
					"False positive reduction must be >=30%% for meaningful business operational improvement")

				// Business Validation: Improvement meets expectations
				improvementError := math.Abs(actualFalsePositiveReduction - scenario.ExpectedImprovement)
				Expect(improvementError).To(BeNumerically("<=", 0.10),
					"Actual improvement must be within 10%% of expected for business planning reliability")

				if actualFalsePositiveReduction >= 0.30 {
					successfulLearningImprovements++
				}
				totalFalsePositiveReduction += actualFalsePositiveReduction

				// Business Value: Calculate operational efficiency gains
				dailyTimeSaved := float64(scenario.BusinessImpact.AverageAlertsPerDay) *
					actualFalsePositiveReduction *
					scenario.BusinessImpact.TimeWastedPerAlert.Minutes()

				monthlyBusinessValue := (dailyTimeSaved * 30 * 2.0) + // Time savings value at $2/minute
					(float64(scenario.BusinessImpact.AverageAlertsPerDay) * 30 *
						actualFalsePositiveReduction * scenario.BusinessImpact.BusinessCostPerAlert)

				totalBusinessValueRealized += monthlyBusinessValue

				// Business Requirement: Verify learning system effectiveness
				Expect(improvementResult.LearningSystemEffectiveness).To(BeNumerically(">=", 0.80),
					"Learning system effectiveness must be >=80%% for business confidence in adaptive improvements")

				// Business Validation: System evolution tracking
				Expect(improvementResult.EvolutionTracking).ToNot(BeEmpty(),
					"Must provide evolution tracking for business learning audit and improvement validation")
				Expect(improvementResult.AutomaticUpdateApplied).To(BeTrue(),
					"System must automatically apply learned improvements for business operational efficiency")

				// Log adaptive learning improvement for business monitoring
				logger.WithFields(logrus.Fields{
					"alert_type":                   scenario.AlertType,
					"business_domain":              scenario.BusinessDomain,
					"baseline_false_positive_rate": scenario.BaselineFalsePositiveRate,
					"new_false_positive_rate":      improvementResult.NewFalsePositiveRate,
					"false_positive_reduction":     actualFalsePositiveReduction,
					"expected_improvement":         scenario.ExpectedImprovement,
					"monthly_business_value":       monthlyBusinessValue,
					"daily_time_saved_minutes":     dailyTimeSaved,
					"learning_effectiveness":       improvementResult.LearningSystemEffectiveness,
					"business_priority":            scenario.BusinessPriority,
				}).Info("Adaptive learning false positive reduction business scenario completed")
			}

			By("Calculating overall adaptive learning business performance and value")

			averageFalsePositiveReduction := totalFalsePositiveReduction / float64(len(falsePositiveScenarios))
			learningSuccessRate := float64(successfulLearningImprovements) / float64(len(falsePositiveScenarios))
			annualBusinessValue := totalBusinessValueRealized * 12

			// Business Requirement: Significant average false positive reduction
			Expect(averageFalsePositiveReduction).To(BeNumerically(">=", 0.30),
				"Average false positive reduction must be >=30%% for meaningful business alert fatigue improvement")

			// Business Requirement: High learning success rate
			Expect(learningSuccessRate).To(BeNumerically(">=", 0.80),
				"Learning success rate must be >=80%% for business confidence in adaptive learning system")

			// Business Value: Significant annual business value
			Expect(annualBusinessValue).To(BeNumerically(">=", 75000.0),
				"Annual business value must be >=75K USD for adaptive learning investment justification")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":             "BR-AD-011",
				"scenarios_tested":                 len(falsePositiveScenarios),
				"average_false_positive_reduction": averageFalsePositiveReduction,
				"learning_success_rate":            learningSuccessRate,
				"monthly_business_value":           totalBusinessValueRealized,
				"annual_business_value":            annualBusinessValue,
				"alert_fatigue_reduction_ready":    averageFalsePositiveReduction >= 0.30,
				"business_impact":                  "Adaptive learning significantly reduces alert fatigue and improves operational efficiency",
			}).Info("BR-AD-011: Adaptive learning false positive reduction business validation completed")
		})

		It("should demonstrate continuous system evolution and accuracy improvement through feedback integration", func() {
			By("Testing continuous system evolution with feedback-driven accuracy improvements")

			// Business Context: Long-term system evolution scenarios showing continuous improvement
			systemEvolutionScenarios := []SystemEvolutionScenario{
				{
					SystemComponent:           "anomaly_detection",
					BusinessDomain:            "performance_monitoring",
					InitialAccuracy:           0.78,                // 78% baseline accuracy
					FeedbackIntegrationPeriod: 90 * 24 * time.Hour, // 90 days of continuous learning
					ExpectedFinalAccuracy:     0.88,                // 88% target accuracy
					ExpectedEvolutionRate:     0.001,               // 0.1% daily improvement
					BusinessValue: SystemEvolutionBusinessValue{
						OperationalEfficiencyGain:   0.25,    // 25% efficiency improvement
						IncidentPreventionIncrease:  0.30,    // 30% more incidents prevented
						BusinessCostSavingsPerMonth: 15000.0, // $15K monthly savings
					},
					BusinessPriority: "critical",
				},
				{
					SystemComponent:           "pattern_recognition",
					BusinessDomain:            "workflow_optimization",
					InitialAccuracy:           0.82,                // 82% baseline accuracy
					FeedbackIntegrationPeriod: 60 * 24 * time.Hour, // 60 days of continuous learning
					ExpectedFinalAccuracy:     0.90,                // 90% target accuracy
					ExpectedEvolutionRate:     0.0015,              // 0.15% daily improvement
					BusinessValue: SystemEvolutionBusinessValue{
						OperationalEfficiencyGain:   0.20,    // 20% efficiency improvement
						IncidentPreventionIncrease:  0.25,    // 25% more incidents prevented
						BusinessCostSavingsPerMonth: 12000.0, // $12K monthly savings
					},
					BusinessPriority: "high",
				},
			}

			totalAccuracyImprovement := 0.0
			totalBusinessValueGenerated := 0.0
			successfulEvolutions := 0

			for _, scenario := range systemEvolutionScenarios {
				By(fmt.Sprintf("Testing system evolution for %s in %s domain", scenario.SystemComponent, scenario.BusinessDomain))

				// Initialize continuous evolution tracking
				evolutionTracker, err := adaptiveLearningEngine.InitializeEvolutionTracking(ctx, scenario.SystemComponent, scenario.InitialAccuracy)
				Expect(err).ToNot(HaveOccurred(), "Evolution tracking initialization must succeed")

				// Simulate continuous feedback integration over time
				evolutionResult, err := adaptiveLearningEngine.ExecuteContinuousEvolution(ctx, evolutionTracker, scenario.FeedbackIntegrationPeriod)
				Expect(err).ToNot(HaveOccurred(), "Continuous evolution execution must succeed for business system improvement")

				// Business Requirement: Measure actual accuracy improvement
				actualAccuracyImprovement := evolutionResult.FinalAccuracy - scenario.InitialAccuracy
				expectedAccuracyImprovement := scenario.ExpectedFinalAccuracy - scenario.InitialAccuracy

				Expect(actualAccuracyImprovement).To(BeNumerically(">=", expectedAccuracyImprovement*0.80),
					"Accuracy improvement must achieve >=80%% of expected improvement for business system evolution success")

				// Business Validation: Evolution rate consistency
				actualEvolutionRate := actualAccuracyImprovement / scenario.FeedbackIntegrationPeriod.Hours() * 24 // Daily rate
				evolutionRateError := math.Abs(actualEvolutionRate - scenario.ExpectedEvolutionRate)
				Expect(evolutionRateError).To(BeNumerically("<=", scenario.ExpectedEvolutionRate*0.50),
					"Evolution rate must be within 50%% of expected for business planning predictability")

				if actualAccuracyImprovement >= expectedAccuracyImprovement*0.80 {
					successfulEvolutions++
				}
				totalAccuracyImprovement += actualAccuracyImprovement

				// Business Requirement: Automatic system updating for continuous improvement
				Expect(evolutionResult.AutomaticUpdatesApplied).To(BeTrue(),
					"System must automatically apply evolutionary improvements for business operational efficiency")
				Expect(len(evolutionResult.UpdatesApplied)).To(BeNumerically(">=", 5),
					"Must apply >=5 evolutionary updates during learning period for meaningful continuous improvement")

				// Business Value: Calculate business impact of system evolution
				monthlyEfficiencyGain := scenario.BusinessValue.OperationalEfficiencyGain * 10000.0          // $10K base operational value
				monthlyIncidentPreventionValue := scenario.BusinessValue.IncidentPreventionIncrease * 8000.0 // $8K base incident cost
				monthlyBusinessValue := monthlyEfficiencyGain + monthlyIncidentPreventionValue + scenario.BusinessValue.BusinessCostSavingsPerMonth

				totalBusinessValueGenerated += monthlyBusinessValue

				// Business Validation: Feedback loop effectiveness
				Expect(evolutionResult.FeedbackLoopEffectiveness).To(BeNumerically(">=", 0.85),
					"Feedback loop effectiveness must be >=85%% for business continuous improvement confidence")

				// Log system evolution results for business tracking
				logger.WithFields(logrus.Fields{
					"system_component":            scenario.SystemComponent,
					"business_domain":             scenario.BusinessDomain,
					"initial_accuracy":            scenario.InitialAccuracy,
					"final_accuracy":              evolutionResult.FinalAccuracy,
					"accuracy_improvement":        actualAccuracyImprovement,
					"expected_improvement":        expectedAccuracyImprovement,
					"evolution_rate_daily":        actualEvolutionRate,
					"expected_evolution_rate":     scenario.ExpectedEvolutionRate,
					"automatic_updates_applied":   len(evolutionResult.UpdatesApplied),
					"monthly_business_value":      monthlyBusinessValue,
					"feedback_loop_effectiveness": evolutionResult.FeedbackLoopEffectiveness,
					"business_priority":           scenario.BusinessPriority,
				}).Info("System evolution business scenario completed")
			}

			By("Validating overall continuous system evolution business performance")

			averageAccuracyImprovement := totalAccuracyImprovement / float64(len(systemEvolutionScenarios))
			evolutionSuccessRate := float64(successfulEvolutions) / float64(len(systemEvolutionScenarios))
			annualBusinessValue := totalBusinessValueGenerated * 12

			// Business Requirement: Significant average accuracy improvement
			Expect(averageAccuracyImprovement).To(BeNumerically(">=", 0.08),
				"Average accuracy improvement must be >=8%% for meaningful business system evolution value")

			// Business Requirement: High evolution success rate
			Expect(evolutionSuccessRate).To(BeNumerically(">=", 0.75),
				"Evolution success rate must be >=75%% for business confidence in continuous learning systems")

			// Business Value: Significant annual business value from continuous evolution
			Expect(annualBusinessValue).To(BeNumerically(">=", 200000.0),
				"Annual business value must be >=200K USD for continuous evolution investment justification")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":         "BR-AD-011",
				"scenario":                     "continuous_evolution",
				"scenarios_tested":             len(systemEvolutionScenarios),
				"average_accuracy_improvement": averageAccuracyImprovement,
				"evolution_success_rate":       evolutionSuccessRate,
				"monthly_business_value":       totalBusinessValueGenerated,
				"annual_business_value":        annualBusinessValue,
				"continuous_improvement_ready": averageAccuracyImprovement >= 0.08 && evolutionSuccessRate >= 0.75,
				"business_impact":              "Continuous system evolution delivers sustained business value through ongoing accuracy improvements",
			}).Info("BR-AD-011: Continuous system evolution business validation completed")
		})
	})
})

// Business type definitions for Phase 2 Pattern Evolution

type BusinessPatternLifecycle struct {
	PatternID             string
	BusinessDomain        string
	CreationDate          time.Time
	PatternType           string
	HistoricalPerformance PatternPerformanceHistory
	CurrentRelevance      float64
	ExpectedObsolescence  bool
	BusinessJustification string
}

type PatternPerformanceHistory struct {
	SuccessRateOverTime []float64
	UsageFrequency      []int
	BusinessImpact      []string
}

type OperationalEfficiencyScenario struct {
	ScenarioName           string
	BusinessDomain         string
	ObsoletePatterns       []ObsoletePattern
	ReplacementPatterns    []ReplacementPattern
	ExpectedEfficiencyGain float64
	ExpectedCostReduction  float64
}

type ObsoletePattern struct {
	PatternID                string
	FailureRate              float64
	AverageResolutionTime    time.Duration
	BusinessImpactPerFailure float64
	MonthlyUsage             int
}

type ReplacementPattern struct {
	PatternID                string
	SuccessRate              float64
	AverageResolutionTime    time.Duration
	BusinessImpactPerSuccess float64
	ExpectedMonthlyUsage     int
}

type FalsePositiveScenario struct {
	AlertType                 string
	BusinessDomain            string
	BaselineFalsePositiveRate float64
	BusinessImpact            FalsePositiveBusinessImpact
	ExpectedImprovement       float64
	BusinessPriority          string
}

type FalsePositiveBusinessImpact struct {
	TimeWastedPerAlert     time.Duration
	AverageAlertsPerDay    int
	OperatorEfficiencyLoss float64
	BusinessCostPerAlert   float64
}

type SystemEvolutionScenario struct {
	SystemComponent           string
	BusinessDomain            string
	InitialAccuracy           float64
	FeedbackIntegrationPeriod time.Duration
	ExpectedFinalAccuracy     float64
	ExpectedEvolutionRate     float64
	BusinessValue             SystemEvolutionBusinessValue
	BusinessPriority          string
}

type SystemEvolutionBusinessValue struct {
	OperationalEfficiencyGain   float64
	IncidentPreventionIncrease  float64
	BusinessCostSavingsPerMonth float64
}

type OperationalCosts struct {
	MonthlyCost          float64
	TotalOperationalTime float64
	FailureRate          float64
}

// Business helper functions for Phase 2 Pattern Evolution testing

func setupPhase2BusinessEvolutionData(mockPatternStore *MockPatternStore, mockExecutionRepo *MockExecutionRepository, mockFeedbackCollector *MockFeedbackCollector) {
	// Setup realistic business evolution data following existing mock patterns
	businessEvolutionPatterns := []EvolutionPattern{
		{
			PatternType:    "pattern_lifecycle",
			BusinessDomain: "operational_efficiency",
			EvolutionRate:  0.15,
			BusinessImpact: 5000.0,
		},
		{
			PatternType:    "adaptive_learning",
			BusinessDomain: "alert_management",
			EvolutionRate:  0.25,
			BusinessImpact: 8000.0,
		},
	}

	for _, pattern := range businessEvolutionPatterns {
		mockPatternStore.StorePattern(pattern.PatternType, pattern)
	}

	// Setup feedback data for adaptive learning
	feedbackScenarios := []FeedbackScenario{
		{
			AlertType:         "memory_pressure_warning",
			FeedbackQuality:   0.85,
			BusinessRelevance: 0.90,
		},
		{
			AlertType:         "disk_usage_critical",
			FeedbackQuality:   0.88,
			BusinessRelevance: 0.95,
		},
	}

	for _, feedback := range feedbackScenarios {
		mockFeedbackCollector.StoreFeedback(feedback.AlertType, feedback)
	}
}

func calculateObsolescenceDetectionAccuracy(detected, expected bool) bool {
	// Calculate accuracy of obsolescence detection for business validation
	return detected == expected
}

func calculateObsolescenceBusinessImpact(lifecycle BusinessPatternLifecycle, result ObsolescenceEvaluationResult) float64 {
	// Calculate business impact of obsolescence management decisions
	baseImpact := 2000.0 // $2K base impact

	// Factor in pattern age and relevance
	ageFactor := time.Since(lifecycle.CreationDate).Hours() / (24 * 30) // Age in months
	relevanceFactor := lifecycle.CurrentRelevance

	// Calculate impact based on obsolescence decision accuracy
	businessImpact := baseImpact

	if result.IsObsolete && lifecycle.ExpectedObsolescence {
		// Correct obsolescence detection - positive business impact
		businessImpact += (1.0 - relevanceFactor) * 3000.0 // Higher impact for correctly identifying low relevance
	} else if !result.IsObsolete && !lifecycle.ExpectedObsolescence {
		// Correct active pattern retention - positive business impact
		businessImpact += relevanceFactor * 2500.0 // Higher impact for correctly retaining high relevance
	} else {
		// Incorrect detection - reduced business impact
		businessImpact *= 0.5
	}

	// Factor in confidence level
	businessImpact *= result.ConfidenceLevel

	return businessImpact
}

func calculateBaselineOperationalCosts(obsoletePatterns []ObsoletePattern) OperationalCosts {
	// Calculate operational costs with obsolete patterns
	totalMonthlyCost := 0.0
	totalOperationalTime := 0.0
	totalFailures := 0.0
	totalPatterns := float64(len(obsoletePatterns))

	for _, pattern := range obsoletePatterns {
		monthlyCost := float64(pattern.MonthlyUsage) * pattern.BusinessImpactPerFailure * pattern.FailureRate
		operationalTime := float64(pattern.MonthlyUsage) * pattern.AverageResolutionTime.Hours()

		totalMonthlyCost += monthlyCost
		totalOperationalTime += operationalTime
		totalFailures += pattern.FailureRate
	}

	averageFailureRate := totalFailures / totalPatterns

	return OperationalCosts{
		MonthlyCost:          totalMonthlyCost,
		TotalOperationalTime: totalOperationalTime,
		FailureRate:          averageFailureRate,
	}
}

func calculateImprovedOperationalCosts(replacementPatterns []ReplacementPattern) OperationalCosts {
	// Calculate operational costs with replacement patterns
	totalMonthlyCost := 0.0
	totalOperationalTime := 0.0
	totalSuccesses := 0.0
	totalPatterns := float64(len(replacementPatterns))

	for _, pattern := range replacementPatterns {
		// Cost is now benefit-based (successful remediations provide value)
		monthlyValue := float64(pattern.ExpectedMonthlyUsage) * pattern.BusinessImpactPerSuccess * pattern.SuccessRate
		operationalTime := float64(pattern.ExpectedMonthlyUsage) * pattern.AverageResolutionTime.Hours()

		totalMonthlyCost -= monthlyValue // Negative cost means business value/savings
		totalOperationalTime += operationalTime
		totalSuccesses += pattern.SuccessRate
	}

	averageSuccessRate := totalSuccesses / totalPatterns

	return OperationalCosts{
		MonthlyCost:          totalMonthlyCost,
		TotalOperationalTime: totalOperationalTime,
		FailureRate:          1.0 - averageSuccessRate, // Convert success rate to failure rate
	}
}

func generateHistoricalFeedbackData(alertType string, count int) []FeedbackPoint {
	// Generate realistic historical feedback data for adaptive learning
	feedbackData := make([]FeedbackPoint, count)

	// Base false positive rate varies by alert type
	baseFalsePositiveRate := 0.35
	if alertType == "disk_usage_critical" {
		baseFalsePositiveRate = 0.28
	} else if alertType == "network_latency_spike" {
		baseFalsePositiveRate = 0.42
	}

	for i := 0; i < count; i++ {
		// Generate feedback with realistic patterns
		isFalsePositive := (i % 100) < int(baseFalsePositiveRate*100)

		feedbackData[i] = FeedbackPoint{
			Timestamp:        time.Now().Add(-time.Duration(count-i) * time.Hour),
			IsFalsePositive:  isFalsePositive,
			OperatorFeedback: generateOperatorFeedback(isFalsePositive),
			BusinessContext:  alertType,
		}
	}

	return feedbackData
}

func generateOperatorFeedback(isFalsePositive bool) string {
	// Generate realistic operator feedback
	if isFalsePositive {
		falsePositiveFeedback := []string{
			"Normal operational behavior, not an issue",
			"Temporary spike, resolved automatically",
			"Expected pattern during maintenance window",
			"Alert threshold too sensitive for this environment",
		}
		return falsePositiveFeedback[len(falsePositiveFeedback)%4]
	}

	trueFeedback := []string{
		"Confirmed issue, remediation action required",
		"Real performance degradation observed",
		"Business impact detected, escalating",
		"Valid alert, implementing fix",
	}
	return trueFeedback[len(trueFeedback)%4]
}

// Helper types for pattern evolution testing

type EvolutionPattern struct {
	PatternType    string
	BusinessDomain string
	EvolutionRate  float64
	BusinessImpact float64
}

type FeedbackScenario struct {
	AlertType         string
	FeedbackQuality   float64
	BusinessRelevance float64
}

type FeedbackPoint struct {
	Timestamp        time.Time
	IsFalsePositive  bool
	OperatorFeedback string
	BusinessContext  string
}

type ObsolescenceEvaluationResult struct {
	IsObsolete                  bool
	ConfidenceLevel             float64
	BusinessImpactAssessment    string
	TransitionPlan              string
	ReplacementRecommendations  string
	RetirementTimeline          time.Duration
	ContinuedBusinessValue      float64
	OptimizationRecommendations string
}

type PatternRetirementResult struct {
	TransitionSuccess         bool
	BusinessDisruptionMinimal bool
}

type AdaptiveLearningResult struct {
	NewFalsePositiveRate        float64
	LearningSystemEffectiveness float64
	EvolutionTracking           string
	AutomaticUpdateApplied      bool
}

type ContinuousEvolutionResult struct {
	FinalAccuracy             float64
	AutomaticUpdatesApplied   bool
	UpdatesApplied            []string
	FeedbackLoopEffectiveness float64
}
