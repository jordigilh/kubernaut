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
//go:build unit
// +build unit

package optimization

import (
	"testing"
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/internal/config"
	aicontext "github.com/jordigilh/kubernaut/pkg/ai/context"
	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// BR-AI-OPTIMIZATION-001-025: Advanced AI Optimization Algorithm Tests
// Business Impact: Ensures mathematical accuracy of AI optimization algorithms for business intelligence
// Stakeholder Value: Provides executive confidence in AI-driven optimization and cost reduction
var _ = Describe("BR-AI-OPTIMIZATION-001-025: Advanced AI Optimization Algorithm Tests", func() {
	var (
		optimizationService *aicontext.OptimizationService
		modelTrainer        *insights.ModelTrainer
		mockActionHistory   *mocks.MockActionRepository
		mockLogger          *mocks.MockLogger
		ctx                 context.Context
		testConfig          *config.ContextOptimizationConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockActionHistory = mocks.NewMockActionRepository()
		mockLogger = mocks.NewMockLogger()

		// Create actual business logic from pkg/ai/context and pkg/ai/insights
		// Following cursor rules: Use actual business interfaces and implementations
		testConfig = &config.ContextOptimizationConfig{
			GraduatedReduction: config.GraduatedReductionConfig{
				Tiers: map[string]config.ReductionTier{
					"simple": {
						MaxReduction:    0.2,
						MinContextTypes: 2,
					},
					"moderate": {
						MaxReduction:    0.4,
						MinContextTypes: 3,
					},
				},
			},
		}

		optimizationService = aicontext.NewOptimizationService(testConfig, mockLogger.Logger)
		Expect(optimizationService).ToNot(BeNil(), "OptimizationService should be created successfully")

		modelTrainer = insights.NewModelTrainer(mockActionHistory, nil, nil, mockLogger.Logger)
		Expect(modelTrainer).ToNot(BeNil(), "ModelTrainer should be created successfully")
	})

	// BR-AI-OPTIMIZATION-001: Context Optimization Algorithm Tests
	Context("BR-AI-OPTIMIZATION-001: Context Optimization Algorithm Tests", func() {
		It("should optimize context size based on complexity assessment with mathematical precision", func() {
			// Business Requirement: Context optimization must reduce token usage while maintaining quality
			complexityAssessment := &aicontext.ComplexityAssessment{
				Tier:                 "moderate",
				ConfidenceScore:      0.85,
				RecommendedReduction: 0.3,
				MinContextTypes:      3,
				Characteristics:      []string{"moderate-complexity", "multi-component"},
				EscalationRequired:   false,
				Metadata:             map[string]interface{}{"original_size": 1000},
			}

			// Create test context data with multiple types
			contextData := &aicontext.ContextData{
				Kubernetes: &aicontext.KubernetesContext{
					Namespace:    "production",
					ResourceType: "deployment",
					ResourceName: "web-server",
					Labels:       map[string]string{"app": "web-server"},
				},
				Metrics: &aicontext.MetricsContext{
					Source:      "prometheus",
					MetricsData: map[string]float64{"cpu_usage": 0.75, "memory_usage": 0.65, "disk_io": 0.45},
				},
				Logs: &aicontext.LogsContext{
					Source:   "fluentd",
					LogLevel: "error",
					LogEntries: []aicontext.LogEntry{
						{Message: "error1", Level: "error"},
						{Message: "error2", Level: "error"},
					},
				},
			}

			// Test context optimization algorithm
			optimizedContext, err := optimizationService.OptimizeContext(ctx, complexityAssessment, contextData)
			Expect(err).ToNot(HaveOccurred(), "BR-AI-OPTIMIZATION-001: Context optimization should not return an error")
			Expect(optimizedContext).ToNot(BeNil(), "BR-AI-OPTIMIZATION-001: Optimized context should not be nil")

			// Verify mathematical optimization results
			originalTypes := countContextTypes(contextData)
			optimizedTypes := countContextTypes(optimizedContext)
			reductionRate := 1.0 - float64(optimizedTypes)/float64(originalTypes)

			Expect(optimizedTypes).To(BeNumerically(">=", complexityAssessment.MinContextTypes),
				"BR-AI-OPTIMIZATION-001: Optimized context should maintain minimum required types")
			Expect(reductionRate).To(BeNumerically("<=", complexityAssessment.RecommendedReduction+0.1),
				"BR-AI-OPTIMIZATION-001: Reduction rate should not exceed recommended threshold")
		})

		It("should select optimal LLM model based on context size and complexity", func() {
			// Business Requirement: Model selection must optimize cost and performance for business value
			testCases := []struct {
				contextSize   int
				complexity    string
				expectedModel string
			}{
				{300, "simple", "gpt-3.5-turbo"},
				{800, "moderate", "gpt-4"},
				{1200, "complex", "gpt-4"},
			}

			for _, testCase := range testCases {
				selectedModel, err := optimizationService.SelectOptimalLLMModel(ctx, testCase.contextSize, testCase.complexity)
				Expect(err).ToNot(HaveOccurred(), "BR-AI-OPTIMIZATION-001: Model selection should not return an error")
				Expect(selectedModel).To(Equal(testCase.expectedModel),
					"BR-AI-OPTIMIZATION-001: Should select %s for context size %d and complexity %s",
					testCase.expectedModel, testCase.contextSize, testCase.complexity)
			}
		})
	})

	// BR-AI-OPTIMIZATION-002: Model Training Optimization Algorithm Tests
	Context("BR-AI-OPTIMIZATION-002: Model Training Optimization Algorithm Tests", func() {
		It("should train effectiveness prediction models with mathematical convergence", func() {
			// Business Requirement: Model training must achieve >85% accuracy for business confidence
			timeWindow := time.Hour * 24 // 24-hour training window

			// Mock sufficient training data by storing actions in the repository
			// Ensure all actions fall within the 24-hour time window
			for i := 0; i < 100; i++ {
				// Space actions over 20 hours to ensure they're all within the 24-hour window
				minutesAgo := time.Duration(i*12) * time.Minute // 12 minutes apart = 100 actions over ~20 hours
				actionRecord := &actionhistory.ActionRecord{
					ActionType: "scale",
					Alert: actionhistory.AlertContext{
						Name:       "high-cpu",
						Severity:   "warning",
						FiringTime: time.Now().Add(-minutesAgo),
					},
					Timestamp: time.Now().Add(-minutesAgo),
				}
				_, err := mockActionHistory.StoreAction(ctx, actionRecord)
				Expect(err).ToNot(HaveOccurred(), "Failed to store mock action")
			}

			// Test model training algorithm
			result, err := modelTrainer.TrainModels(ctx, insights.ModelTypeEffectivenessPrediction, timeWindow)
			Expect(err).ToNot(HaveOccurred(), "BR-AI-OPTIMIZATION-002: Model training should not return an error")
			Expect(result).ToNot(BeNil(), "BR-AI-OPTIMIZATION-002: Training result should not be nil")
			Expect(result.Success).To(BeTrue(), "BR-AI-OPTIMIZATION-002: Model training should succeed")

			// Verify business requirement compliance: >85% accuracy
			Expect(result.FinalAccuracy).To(BeNumerically(">=", 0.85),
				"BR-AI-OPTIMIZATION-002: Model must achieve >85% accuracy for business requirements")
		})

		It("should handle insufficient training data gracefully with business continuity", func() {
			// Business Requirement: System must handle data scarcity without business disruption
			timeWindow := time.Hour * 1 // Short window with limited data

			// Mock insufficient training data (< 50 samples) by storing fewer actions
			for i := 0; i < 25; i++ {
				actionRecord := &actionhistory.ActionRecord{
					ActionType: "restart",
					Alert: actionhistory.AlertContext{
						Name:       "low-memory",
						Severity:   "critical",
						FiringTime: time.Now().Add(-time.Duration(i) * time.Minute),
					},
					Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
				}
				_, err := mockActionHistory.StoreAction(ctx, actionRecord)
				Expect(err).ToNot(HaveOccurred(), "Failed to store mock action")
			}

			result, err := modelTrainer.TrainModels(ctx, insights.ModelTypeEffectivenessPrediction, timeWindow)
			Expect(err).ToNot(HaveOccurred(), "BR-AI-OPTIMIZATION-002: Should handle insufficient data gracefully")
			Expect(result).ToNot(BeNil(), "BR-AI-OPTIMIZATION-002: Should return result even with insufficient data")
			Expect(result.Success).To(BeFalse(), "BR-AI-OPTIMIZATION-002: Should indicate training failure due to insufficient data")
		})
	})

	// BR-AI-OPTIMIZATION-003: Performance Monitoring Algorithm Tests
	Context("BR-AI-OPTIMIZATION-003: Performance Monitoring Algorithm Tests", func() {
		It("should monitor LLM performance with mathematical precision", func() {
			// Business Requirement: Performance monitoring must provide accurate business metrics
			responseQuality := 0.95
			responseTime := time.Millisecond * 200
			tokenUsage := 1000
			contextSize := 500

			assessment, err := optimizationService.MonitorPerformance(ctx, responseQuality, responseTime, tokenUsage, contextSize)
			Expect(err).ToNot(HaveOccurred(), "BR-AI-OPTIMIZATION-003: Performance monitoring should not return an error")
			Expect(assessment).ToNot(BeNil(), "BR-AI-OPTIMIZATION-003: Performance assessment should not be nil")

			// Verify performance metrics are mathematically sound
			Expect(assessment.ResponseQuality).To(BeNumerically("~", responseQuality, 0.01),
				"BR-AI-OPTIMIZATION-003: Response quality should be accurately recorded")
			Expect(assessment.ResponseTime).To(Equal(responseTime),
				"BR-AI-OPTIMIZATION-003: Response time should be accurately recorded")
		})

		It("should calculate performance correlation coefficients with statistical accuracy", func() {
			// Business Requirement: Statistical analysis must be accurate for business insights

			// Use actual business service that's used in production
			performanceMetrics := aicontext.NewPerformanceMetrics()

			// Record sufficient performance data for statistical correlation (minimum 10 samples)
			// BR-AI-OPTIMIZATION-003: Pattern shows decreasing quality with increasing context size
			for i := 0; i < 10; i++ {
				quality := 0.95 - float64(i)*0.02                              // Decreasing quality: 0.95 to 0.77
				responseTime := time.Millisecond * (200 + time.Duration(i)*50) // Increasing time: 200ms to 650ms
				tokenUsage := 1000 + i*100                                     // Increasing tokens: 1000 to 1900
				contextSize := 500 + i*100                                     // Increasing context: 500 to 1400

				performanceMetrics.RecordPerformance(quality, responseTime, tokenUsage, contextSize)
			}

			// Call actual business method used by main application
			correlationAnalysis := performanceMetrics.GetCorrelationAnalysis()

			// Test business outcomes that matter to operations teams
			Expect(correlationAnalysis).ToNot(BeNil(), "BR-AI-OPTIMIZATION-003: Correlation analysis should not be nil")
			Expect(correlationAnalysis["quality_context_correlation"]).To(BeNumerically("<", -0.8),
				"BR-AI-OPTIMIZATION-003: Should detect strong negative correlation for business insights")
			Expect(correlationAnalysis["analysis_confidence"]).To(BeNumerically(">=", 0.5),
				"BR-AI-OPTIMIZATION-003: Should provide sufficient confidence for business decisions")
			Expect(correlationAnalysis["sample_count"]).To(Equal(10),
				"BR-AI-OPTIMIZATION-003: Should track correct number of performance samples")
		})
	})
})

// Helper functions following cursor rules for business logic integration

// countContextTypes counts the number of context types in ContextData
func countContextTypes(contextData *aicontext.ContextData) int {
	count := 0
	if contextData.Kubernetes != nil {
		count++
	}
	if contextData.Metrics != nil {
		count++
	}
	if contextData.Logs != nil {
		count++
	}
	if contextData.ActionHistory != nil {
		count++
	}
	if contextData.Events != nil {
		count++
	}
	if contextData.Traces != nil {
		count++
	}
	if contextData.NetworkFlows != nil {
		count++
	}
	if contextData.AuditLogs != nil {
		count++
	}
	return count
}

// TestRunner bootstraps the Ginkgo test suite
func TestUadvancedUaiUoptimizationUalgorithms(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UadvancedUaiUoptimizationUalgorithms Suite")
}
