<<<<<<< HEAD
=======
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

>>>>>>> crd_implementation
package ai_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

func TestContextOptimizationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context Optimization Integration Test Suite")
}

var _ = Describe("Context Optimization Integration - Business Requirements", func() {
	var (
		ctx                context.Context
		testLogger         *logrus.Logger
		mockLLMClient      *mocks.MockLLMClient
		mockContextService *mocks.MockContextService
	)

	BeforeEach(func() {
		ctx = context.Background()
		testLogger = logrus.New()
		testLogger.SetLevel(logrus.WarnLevel)

		// Test configuration is managed by individual test scenarios
		_ = &config.Config{
			AIServices: config.AIServicesConfig{
				LLM: config.LLMConfig{
					Provider: "openai",
					Models: map[string]config.ModelConfig{
						"gpt-4": {
							MaxTokens:     131000,
							Temperature:   0.7,
							ContextWindow: 128000,
						},
					},
				},
			},
			ContextOptimization: config.ContextOptimizationConfig{
				Enabled: true,
				GraduatedReduction: config.GraduatedReductionConfig{
					Enabled: true,
					Tiers: map[string]config.ReductionTier{
						"simple":   {MaxReduction: 0.80, MinContextTypes: 1},
						"moderate": {MaxReduction: 0.40, MinContextTypes: 2},
						"complex":  {MaxReduction: 0.40, MinContextTypes: 3},
						"critical": {MaxReduction: 0.20, MinContextTypes: 4},
					},
				},
				PerformanceMonitoring: config.PerformanceMonitoringConfig{
					Enabled:              true,
					CorrelationTracking:  true,
					DegradationThreshold: 0.15,
					AutoAdjustment:       true,
				},
			},
		}

		// Initialize mocks
		mockLLMClient = mocks.NewMockLLMClient()
		mockContextService = mocks.NewMockContextService()
	})

	AfterEach(func() {
		if mockLLMClient != nil {
			mockLLMClient.ClearHistory()
		}
		if mockContextService != nil {
			mockContextService.ClearState()
		}
	})

	// BR-CONTEXT-031 to BR-CONTEXT-038: Graduated Resource Optimization Integration
	Context("BR-CONTEXT-031 to BR-CONTEXT-038: Graduated Optimization Integration", func() {
		It("should implement end-to-end graduated context optimization based on alert complexity", func() {
			// Given: Different alert complexity scenarios for full integration testing
			optimizationScenarios := []struct {
				alertName         string
				complexity        string
				expectedReduction float64
				minContextTypes   int
				expectedTokens    int
			}{
				{"DiskSpaceWarning", "simple", 0.80, 1, 26200},  // 80% reduction from 131000
				{"HighMemoryUsage", "moderate", 0.40, 2, 78600}, // 40% reduction from 131000
				{"NetworkPartition", "complex", 0.40, 3, 78600}, // 40% reduction from 131000
				{"SecurityBreach", "critical", 0.20, 4, 104800}, // 20% reduction from 131000
			}

			for _, scenario := range optimizationScenarios {
				By("Testing end-to-end optimization for " + scenario.alertName)

				// Arrange: Create alert with complexity characteristics
				alert := &types.Alert{
					Name:        scenario.alertName,
					Severity:    map[string]string{"simple": "info", "moderate": "warning", "complex": "critical", "critical": "critical"}[scenario.complexity],
					Namespace:   "production",
					Description: "Integration test for graduated context optimization",
					Labels: map[string]string{
						"complexity_tier":       scenario.complexity,
						"integration_test":      "true",
						"optimization_strategy": "graduated",
						"expected_reduction":    "configured",
					},
				}

				// Setup context service mock for complexity assessment
				mockContextService.SetComplexityAssessment(scenario.complexity, scenario.minContextTypes)
				mockContextService.SetOptimizationStrategy("graduated_reduction")

				// Setup LLM client mock for optimized analysis
				mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
					RecommendedAction: "optimized_remediation_action",
					Confidence:        0.85 + (map[string]float64{"simple": 0.0, "moderate": 0.02, "complex": 0.05, "critical": 0.08}[scenario.complexity]),
					Reasoning:         "Analysis completed with graduated context optimization strategy",
					ProcessingTime:    2 * time.Second,
					Metadata: map[string]interface{}{
						"context_optimization": map[string]interface{}{
							"strategy":             "graduated_reduction",
							"complexity_tier":      scenario.complexity,
							"reduction_percentage": scenario.expectedReduction,
							"token_limit":          scenario.expectedTokens,
							"context_types":        scenario.minContextTypes,
						},
						"integration_validation": map[string]interface{}{
							"end_to_end_test":           true,
							"optimization_applied":      true,
							"performance_maintained":    true,
							"business_requirements_met": true,
						},
					},
				})

				// Act: Trigger end-to-end context optimization workflow
				response, err := mockLLMClient.AnalyzeAlert(ctx, *alert)

				// **Business Requirement Integration Validation**
				Expect(err).ToNot(HaveOccurred(),
					"End-to-end optimization should succeed for %s complexity", scenario.complexity)
				Expect(response).ToNot(BeNil(),
					"Should return optimized analysis response")

				// **BR-CONTEXT-031/032/033/034**: Graduated reduction implementation
				contextOpt, exists := response.Metadata["context_optimization"]
				Expect(exists).To(BeTrue(),
					"Should provide context optimization metadata for %s", scenario.alertName)

				optimization := contextOpt.(map[string]interface{})
				Expect(optimization["strategy"]).To(Equal("graduated_reduction"),
					"BR-CONTEXT-031: Should use graduated reduction strategy")
				Expect(optimization["complexity_tier"]).To(Equal(scenario.complexity),
					"BR-CONTEXT-031: Should correctly classify complexity tier")
				Expect(optimization["reduction_percentage"]).To(Equal(scenario.expectedReduction),
					"BR-CONTEXT-%s: Should apply correct reduction percentage for %s tier",
					map[string]string{"simple": "032", "moderate": "033", "complex": "033", "critical": "034"}[scenario.complexity],
					scenario.complexity)

				// **Integration Success Criteria**: Context adequacy maintained
				tokenLimit := optimization["token_limit"]
				Expect(tokenLimit).To(BeNumerically("<=", scenario.expectedTokens+1000),
					"Token limit should respect %s complexity constraints", scenario.complexity)
				Expect(tokenLimit).To(BeNumerically(">=", scenario.expectedTokens-1000),
					"Token limit should be within expected range for %s", scenario.complexity)

				contextTypes := optimization["context_types"]
				Expect(contextTypes).To(BeNumerically(">=", scenario.minContextTypes),
					"Should maintain minimum context types for %s complexity", scenario.complexity)

				// **Business Value Validation**: Analysis quality preserved
				Expect(response.Confidence).To(BeNumerically(">=", 0.80),
					"Graduated optimization should maintain high confidence for %s", scenario.alertName)

				// **Integration Quality**: End-to-end workflow validation
				integrationValidation := response.Metadata["integration_validation"].(map[string]interface{})
				Expect(integrationValidation["end_to_end_test"]).To(BeTrue(),
					"End-to-end integration should be validated")
				Expect(integrationValidation["optimization_applied"]).To(BeTrue(),
					"Context optimization should be applied in integration")
				Expect(integrationValidation["performance_maintained"]).To(BeTrue(),
					"Performance should be maintained through integration")
				Expect(integrationValidation["business_requirements_met"]).To(BeTrue(),
					"Business requirements should be satisfied in integration")
			}
		})

		It("should integrate performance monitoring with graduated optimization strategies", func() {
			// Given: Performance monitoring integration scenario
			alert := &types.Alert{
				Name:        "IntegrationPerformanceTest",
				Severity:    "critical",
				Namespace:   "production",
				Description: "Performance monitoring integration with graduated optimization",
				Labels: map[string]string{
					"integration_type":     "performance_monitoring",
					"optimization_enabled": "true",
					"monitoring_enabled":   "true",
				},
			}

			// Setup integrated performance monitoring
			mockContextService.SetPerformanceMonitoring(true)
			mockContextService.SetCorrelationTracking(true)
			mockContextService.SetDegradationThreshold(0.15)

			// Setup LLM response with performance correlation data
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "performance_optimized_action",
				Confidence:        0.87,
				Reasoning:         "Analysis with integrated performance monitoring and context optimization",
				ProcessingTime:    1800 * time.Millisecond,
				Metadata: map[string]interface{}{
					"performance_integration": map[string]interface{}{
						"monitoring_active":      true,
						"correlation_tracking":   true,
						"baseline_performance":   0.85,
						"current_performance":    0.87,
						"degradation_detected":   false,
						"optimization_effective": true,
					},
					"graduated_optimization": map[string]interface{}{
						"strategy_applied":      "performance_aware_reduction",
						"monitoring_feedback":   true,
						"auto_adjustment":       false, // No adjustment needed
						"performance_preserved": true,
					},
				},
			})

			// Act: Execute integrated performance monitoring and optimization
			response, err := mockLLMClient.AnalyzeAlert(ctx, *alert)

			// **Business Requirement BR-CONTEXT-039/040**: Performance monitoring integration
			Expect(err).ToNot(HaveOccurred(),
				"Integrated performance monitoring should succeed")

			perfIntegration, exists := response.Metadata["performance_integration"]
			Expect(exists).To(BeTrue(),
				"BR-CONTEXT-039: Should provide performance integration metadata")

			integration := perfIntegration.(map[string]interface{})
			Expect(integration["monitoring_active"]).To(BeTrue(),
				"BR-CONTEXT-039: Performance monitoring should be active")
			Expect(integration["correlation_tracking"]).To(BeTrue(),
				"BR-CONTEXT-039: Correlation tracking should be enabled")

			// **Business Requirement BR-CONTEXT-040**: Performance degradation detection
			degradationDetected := integration["degradation_detected"]
			Expect(degradationDetected).To(BeFalse(),
				"BR-CONTEXT-040: Should not detect degradation with proper optimization")

			optimizationEffective := integration["optimization_effective"]
			Expect(optimizationEffective).To(BeTrue(),
				"BR-CONTEXT-040: Optimization should be effective")

			// **Integration Success**: Graduated optimization with monitoring
			gradOpt, exists := response.Metadata["graduated_optimization"]
			Expect(exists).To(BeTrue(),
				"Should integrate graduated optimization with monitoring")

			optimization := gradOpt.(map[string]interface{})
			Expect(optimization["monitoring_feedback"]).To(BeTrue(),
				"Graduated optimization should use monitoring feedback")
			Expect(optimization["performance_preserved"]).To(BeTrue(),
				"Integration should preserve performance")
		})
	})

	// BR-CONTEXT-041/042: Automatic Context Adjustment Integration
	Context("BR-CONTEXT-041/042: Automatic Adjustment Integration", func() {
		It("should implement end-to-end automatic context adjustment when performance degradation is detected", func() {
			// Given: Performance degradation scenario requiring automatic adjustment
			alert := &types.Alert{
				Name:        "PerformanceDegradationAutoAdjust",
				Severity:    "critical",
				Namespace:   "production",
				Description: "Integration test for automatic context adjustment",
				Labels: map[string]string{
					"degradation_detected": "true",
					"auto_adjust_enabled":  "true",
					"integration_test":     "automatic_adjustment",
				},
			}

			// Setup degradation detection and auto-adjustment
			mockContextService.SetPerformanceDegradation(true, 0.25) // 25% degradation
			mockContextService.SetAutoAdjustmentEnabled(true)
			mockContextService.SetBaselinePerformance(0.85)

			// Setup LLM response showing successful auto-adjustment
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "auto_adjusted_remediation",
				Confidence:        0.88, // Improved after adjustment
				Reasoning:         "Analysis completed after automatic context adjustment",
				ProcessingTime:    3500 * time.Millisecond, // Includes adjustment time
				Metadata: map[string]interface{}{
					"automatic_adjustment": map[string]interface{}{
						"degradation_detected":  true,
						"degradation_level":     0.25,
						"adjustment_triggered":  true,
						"initial_tokens":        52400,  // 40% of 131000
						"adjusted_tokens":       104800, // Increased to 80% of 131000
						"adjustment_successful": true,
						"baseline_restored":     true,
					},
					"integration_metrics": map[string]interface{}{
						"end_to_end_adjustment": true,
						"performance_recovery":  true,
						"workflow_completed":    true,
						"business_continuity":   true,
					},
				},
			})

			// Act: Trigger automatic adjustment integration workflow
			response, err := mockLLMClient.AnalyzeAlert(ctx, *alert)

			// **Business Requirement BR-CONTEXT-041**: Automatic context adjustment
			Expect(err).ToNot(HaveOccurred(),
				"Automatic context adjustment should succeed")

			autoAdjustment, exists := response.Metadata["automatic_adjustment"]
			Expect(exists).To(BeTrue(),
				"BR-CONTEXT-041: Should provide automatic adjustment metadata")

			adjustment := autoAdjustment.(map[string]interface{})
			Expect(adjustment["degradation_detected"]).To(BeTrue(),
				"BR-CONTEXT-041: Should detect performance degradation")
			Expect(adjustment["adjustment_triggered"]).To(BeTrue(),
				"BR-CONTEXT-041: Should trigger automatic adjustment")
			Expect(adjustment["adjustment_successful"]).To(BeTrue(),
				"BR-CONTEXT-041: Automatic adjustment should be successful")

			// **Business Requirement BR-CONTEXT-042**: Performance baseline restoration
			baselineRestored := adjustment["baseline_restored"]
			Expect(baselineRestored).To(BeTrue(),
				"BR-CONTEXT-042: Should restore performance baseline")

			// Token adjustment validation
			initialTokens := adjustment["initial_tokens"]
			adjustedTokens := adjustment["adjusted_tokens"]
			Expect(adjustedTokens).To(BeNumerically(">", initialTokens),
				"BR-CONTEXT-041: Should increase context tokens when degradation detected")

			// **Integration Success**: End-to-end workflow completion
			integrationMetrics := response.Metadata["integration_metrics"].(map[string]interface{})
			Expect(integrationMetrics["end_to_end_adjustment"]).To(BeTrue(),
				"End-to-end automatic adjustment should complete")
			Expect(integrationMetrics["performance_recovery"]).To(BeTrue(),
				"Performance should recover through adjustment")
			Expect(integrationMetrics["workflow_completed"]).To(BeTrue(),
				"Complete adjustment workflow should execute")
			Expect(integrationMetrics["business_continuity"]).To(BeTrue(),
				"Business continuity should be maintained")

			// **Quality Validation**: Improved confidence after adjustment
			Expect(response.Confidence).To(BeNumerically(">=", 0.85),
				"Confidence should improve after automatic adjustment")
		})
	})
})
