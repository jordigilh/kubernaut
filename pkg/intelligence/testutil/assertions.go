package testutil

import (
	"math"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"

	// Guideline #14: Exception for test utilities - dot import acceptable for test assertion APIs
	. "github.com/onsi/gomega" //nolint:staticcheck
)

// Minimal interfaces to avoid import cycles - assertions only need these methods
type MLModelLike interface {
	GetID() string
	GetModelType() string
	GetAccuracy() float64
	GetTrainingTime() time.Duration
	GetVersion() string
	GetFeatures() []string
	GetTrainedAt() time.Time
}

type PatternAnalysisRequestLike interface {
	GetAnalysisType() string
	GetPatternTypes() []shared.PatternType
}

type PatternAnalysisResultLike interface {
	GetPatterns() []*shared.DiscoveredPattern
	GetAnalysisTime() time.Duration
	GetQualityScore() float64
}

type LearningMetricsLike interface {
	GetTotalAnalyses() int
	GetAverageConfidence() float64
	GetLearningRate() float64
}

type PatternDiscoveryConfigLike interface {
	GetMinExecutionsForPattern() int
	GetConfidenceThreshold() float64
}

type MLConfigLike interface {
	GetModelType() string
	GetLearningRate() float64
	GetBatchSize() int
}

// IntelligenceAssertions provides standardized assertion helpers for intelligence tests
type IntelligenceAssertions struct{}

// NewIntelligenceAssertions creates a new intelligence assertions helper
func NewIntelligenceAssertions() *IntelligenceAssertions {
	return &IntelligenceAssertions{}
}

// AssertNoError verifies no error occurred with context
func (a *IntelligenceAssertions) AssertNoError(err error, context string) {
	Expect(err).NotTo(HaveOccurred(), "Expected no error in %s, but got: %v", context, err)
}

// AssertErrorContains verifies error contains expected text
func (a *IntelligenceAssertions) AssertErrorContains(err error, expectedText string) {
	Expect(err).To(HaveOccurred(), "Expected an error containing '%s'", expectedText)
	Expect(err.Error()).To(ContainSubstring(expectedText))
}

// AssertMLModelValid verifies ML model has valid structure and metrics
func (a *IntelligenceAssertions) AssertMLModelValid(model MLModelLike) {
	Expect(model).NotTo(BeNil(), "ML model should not be nil")
	Expect(model.GetID()).NotTo(BeEmpty(), "ML model ID should not be empty")
	Expect(model.GetModelType()).NotTo(BeEmpty(), "ML model type should not be empty")
	Expect(model.GetAccuracy()).To(BeNumerically(">=", 0), "ML model accuracy should be non-negative")
	Expect(model.GetAccuracy()).To(BeNumerically("<=", 1), "ML model accuracy should be <= 1")
	Expect(model.GetFeatures()).NotTo(BeEmpty(), "ML model should have features")
	Expect(model.GetTrainedAt()).NotTo(BeZero(), "ML model should have training timestamp")
}

// AssertMLModelAccuracy verifies ML model accuracy is within expected range
func (a *IntelligenceAssertions) AssertMLModelAccuracy(model MLModelLike, minAccuracy, maxAccuracy float64) {
	a.AssertMLModelValid(model)
	Expect(model.GetAccuracy()).To(BeNumerically(">=", minAccuracy),
		"ML model accuracy should be >= %.3f, got %.3f", minAccuracy, model.GetAccuracy())
	Expect(model.GetAccuracy()).To(BeNumerically("<=", maxAccuracy),
		"ML model accuracy should be <= %.3f, got %.3f", maxAccuracy, model.GetAccuracy())
}

// AssertHighAccuracyModel verifies model has high accuracy (>= 0.8)
func (a *IntelligenceAssertions) AssertHighAccuracyModel(model MLModelLike) {
	a.AssertMLModelAccuracy(model, 0.8, 1.0)
}

// AssertLowAccuracyModel verifies model has low accuracy (< 0.7)
func (a *IntelligenceAssertions) AssertLowAccuracyModel(model MLModelLike) {
	a.AssertMLModelAccuracy(model, 0.0, 0.7)
}

// AssertPatternAnalysisRequestValid verifies pattern analysis request is valid
func (a *IntelligenceAssertions) AssertPatternAnalysisRequestValid(request PatternAnalysisRequestLike) {
	Expect(request).NotTo(BeNil(), "Pattern analysis request should not be nil")
	Expect(request.GetAnalysisType()).NotTo(BeEmpty(), "Analysis type should not be empty")
}

// AssertPatternAnalysisResultValid verifies pattern analysis result is valid
func (a *IntelligenceAssertions) AssertPatternAnalysisResultValid(result PatternAnalysisResultLike) {
	Expect(result).NotTo(BeNil(), "Pattern analysis result should not be nil")

	// Verify patterns if present
	for _, pattern := range result.GetPatterns() {
		a.AssertDiscoveredPatternValid(pattern)
	}
}

// AssertDiscoveredPatternValid verifies discovered pattern is valid
func (a *IntelligenceAssertions) AssertDiscoveredPatternValid(pattern *shared.DiscoveredPattern) {
	Expect(pattern).NotTo(BeNil(), "Discovered pattern should not be nil")
	Expect(pattern.ID).NotTo(BeEmpty(), "Pattern ID should not be empty")
	Expect(string(pattern.PatternType)).NotTo(BeEmpty(), "Pattern type should not be empty")
	a.AssertProbabilityRange(pattern.Confidence, "pattern_confidence")
	Expect(pattern.Frequency).To(BeNumerically(">", 0), "Pattern frequency should be positive")
	Expect(pattern.DiscoveredAt).NotTo(BeZero(), "Discovered at timestamp should be set")
}

// AssertLearningMetricsValid verifies learning metrics are valid
func (a *IntelligenceAssertions) AssertLearningMetricsValid(metrics LearningMetricsLike) {
	Expect(metrics).NotTo(BeNil(), "Learning metrics should not be nil")

	// Verify basic fields
	Expect(metrics.GetTotalAnalyses()).To(BeNumerically(">=", 0), "Total analyses should be non-negative")
	a.AssertProbabilityRange(metrics.GetAverageConfidence(), "average_confidence")
	Expect(metrics.GetLearningRate()).To(BeNumerically(">=", 0), "Learning rate should be non-negative")

}

// AssertPatternDiscoveryConfigValid verifies pattern discovery configuration is valid
func (a *IntelligenceAssertions) AssertPatternDiscoveryConfigValid(config PatternDiscoveryConfigLike) {
	Expect(config).NotTo(BeNil(), "Pattern discovery config should not be nil")
	Expect(config.GetMinExecutionsForPattern()).To(BeNumerically(">", 0), "Min executions should be positive")
	a.AssertProbabilityRange(config.GetConfidenceThreshold(), "confidence_threshold")
}

// AssertWorkflowExecutionDataValid verifies workflow execution data is valid
func (a *IntelligenceAssertions) AssertWorkflowExecutionDataValid(data []*engine.EngineWorkflowExecutionData) {
	Expect(data).NotTo(BeEmpty(), "Workflow execution data should not be empty")

	for i, execution := range data {
		Expect(execution).NotTo(BeNil(), "Execution %d should not be nil", i)
		Expect(execution.ExecutionID).NotTo(BeEmpty(), "Execution %d ID should not be empty", i)
		Expect(execution.WorkflowID).NotTo(BeEmpty(), "Execution %d workflow ID should not be empty", i)
		Expect(execution.Timestamp).NotTo(BeZero(), "Execution %d timestamp should be set", i)
		Expect(execution.Duration).To(BeNumerically(">=", 0), "Execution %d duration should be non-negative", i)
	}
}

// AssertSuccessRate verifies success rate is within expected range
func (a *IntelligenceAssertions) AssertSuccessRate(data []*engine.EngineWorkflowExecutionData, expectedRate, tolerance float64) {
	a.AssertWorkflowExecutionDataValid(data)

	successCount := 0
	for _, execution := range data {
		if execution.Success {
			successCount++
		}
	}

	actualRate := float64(successCount) / float64(len(data))
	Expect(actualRate).To(BeNumerically("~", expectedRate, tolerance),
		"Success rate should be ~%.3f±%.3f, got %.3f (%d/%d)",
		expectedRate, tolerance, actualRate, successCount, len(data))
}

// AssertWorkflowFeaturesValid verifies workflow features are valid
func (a *IntelligenceAssertions) AssertWorkflowFeaturesValid(features *shared.WorkflowFeatures) {
	Expect(features).NotTo(BeNil(), "Workflow features should not be nil")
}

// AssertWorkflowPredictionValid verifies workflow prediction is valid
func (a *IntelligenceAssertions) AssertWorkflowPredictionValid(prediction *shared.WorkflowPrediction) {
	Expect(prediction).NotTo(BeNil(), "Workflow prediction should not be nil")
	a.AssertProbabilityRange(prediction.Confidence, "prediction_confidence")
}

// AssertMLConfigValid verifies ML configuration is valid
func (a *IntelligenceAssertions) AssertMLConfigValid(config MLConfigLike) {
	Expect(config).NotTo(BeNil(), "ML config should not be nil")
	Expect(config.GetModelType()).NotTo(BeEmpty(), "Model type should not be empty")
	Expect(config.GetLearningRate()).To(BeNumerically(">", 0), "Learning rate should be positive")
	Expect(config.GetBatchSize()).To(BeNumerically(">", 0), "Batch size should be positive")
}

// AssertTimeSeriesDataValid verifies time series data
func (a *IntelligenceAssertions) AssertTimeSeriesDataValid(data []map[string]interface{}, expectedPattern string) {
	Expect(data).NotTo(BeEmpty(), "Time series data should not be empty")

	// Verify each point
	var prevTimestamp time.Time
	for i, point := range data {
		timestamp, ok := point["timestamp"].(time.Time)
		Expect(ok).To(BeTrue(), "Point %d should have valid timestamp", i)
		Expect(timestamp).NotTo(BeZero(), "Point %d timestamp should be set", i)

		value, ok := point["value"].(float64)
		Expect(ok).To(BeTrue(), "Point %d should have valid value", i)
		Expect(math.IsNaN(value)).To(BeFalse(), "Point %d value should not be NaN", i)

		if i > 0 {
			Expect(timestamp).To(BeTemporally(">=", prevTimestamp), "Timestamps should be ordered")
		}
		prevTimestamp = timestamp
	}

	// Verify pattern characteristics if specified
	if expectedPattern != "" && len(data) > 2 {
		switch expectedPattern {
		case "increasing":
			a.assertIncreasingTrend(data)
		case "decreasing":
			a.assertDecreasingTrend(data)
		case "stable":
			a.assertStableTrend(data)
		case "oscillating":
			a.assertOscillatingTrend(data)
		}
	}
}

// AssertProbabilityRange verifies value is in probability range [0, 1]
func (a *IntelligenceAssertions) AssertProbabilityRange(value float64, metricName string) {
	Expect(value).To(BeNumerically(">=", 0), "%s should be >= 0, got %.4f", metricName, value)
	Expect(value).To(BeNumerically("<=", 1), "%s should be <= 1, got %.4f", metricName, value)
}

// AssertConfidenceLevel verifies confidence level is appropriate
func (a *IntelligenceAssertions) AssertConfidenceLevel(confidence float64, minConfidence float64) {
	a.AssertProbabilityRange(confidence, "confidence")
	Expect(confidence).To(BeNumerically(">=", minConfidence),
		"Confidence should be >= %.3f, got %.3f", minConfidence, confidence)
}

// AssertModelPerformanceThreshold verifies model meets performance threshold
func (a *IntelligenceAssertions) AssertModelPerformanceThreshold(model MLModelLike, minAccuracy float64) {
	a.AssertMLModelValid(model)
	Expect(model.GetAccuracy()).To(BeNumerically(">=", minAccuracy),
		"Model accuracy should meet threshold %.3f, got %.3f", minAccuracy, model.GetAccuracy())
}

// Helper methods for trend analysis
func (a *IntelligenceAssertions) assertIncreasingTrend(data []map[string]interface{}) {
	increases := 0
	for i := 1; i < len(data); i++ {
		current := data[i]["value"].(float64)
		previous := data[i-1]["value"].(float64)
		if current > previous {
			increases++
		}
	}

	// Allow some noise - expect at least 60% of points to increase
	expectedIncreases := int(float64(len(data)-1) * 0.6)
	Expect(increases).To(BeNumerically(">=", expectedIncreases),
		"Increasing trend: expected >= %d increases, got %d", expectedIncreases, increases)
}

func (a *IntelligenceAssertions) assertDecreasingTrend(data []map[string]interface{}) {
	decreases := 0
	for i := 1; i < len(data); i++ {
		current := data[i]["value"].(float64)
		previous := data[i-1]["value"].(float64)
		if current < previous {
			decreases++
		}
	}

	expectedDecreases := int(float64(len(data)-1) * 0.6)
	Expect(decreases).To(BeNumerically(">=", expectedDecreases),
		"Decreasing trend: expected >= %d decreases, got %d", expectedDecreases, decreases)
}

func (a *IntelligenceAssertions) assertStableTrend(data []map[string]interface{}) {
	// Calculate variance - should be low for stable data
	sum := 0.0
	for _, point := range data {
		sum += point["value"].(float64)
	}
	mean := sum / float64(len(data))

	variance := 0.0
	for _, point := range data {
		value := point["value"].(float64)
		diff := value - mean
		variance += diff * diff
	}
	variance /= float64(len(data))

	stdDev := math.Sqrt(variance)
	Expect(stdDev).To(BeNumerically("<=", 0.15),
		"Stable trend: standard deviation should be <= 0.15, got %.4f", stdDev)
}

func (a *IntelligenceAssertions) assertOscillatingTrend(data []map[string]interface{}) {
	// Look for direction changes - oscillating data should have many
	directionChanges := 0
	for i := 2; i < len(data); i++ {
		current := data[i]["value"].(float64)
		previous := data[i-1]["value"].(float64)
		beforePrevious := data[i-2]["value"].(float64)

		trend1 := previous - beforePrevious
		trend2 := current - previous

		// Direction change if trends have different signs
		if (trend1 > 0 && trend2 < 0) || (trend1 < 0 && trend2 > 0) {
			directionChanges++
		}
	}

	// Expect at least 30% of possible direction changes
	minChanges := int(float64(len(data)-2) * 0.3)
	Expect(directionChanges).To(BeNumerically(">=", minChanges),
		"Oscillating trend: expected >= %d direction changes, got %d", minChanges, directionChanges)
}
