package patterns

import (
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// Test Data Helpers

func createTestWorkflowExecutionData(count int, successRate float64) []*engine.WorkflowExecutionData {
	data := make([]*engine.WorkflowExecutionData, count)
	successCount := int(float64(count) * successRate)

	for i := 0; i < count; i++ {
		data[i] = &engine.WorkflowExecutionData{
			ExecutionID: generateExecutionID(i),
			WorkflowID:  generateWorkflowID(i),
			Timestamp:   time.Now().Add(-time.Duration(i) * time.Hour),
			Duration:    time.Duration(100+i*10) * time.Millisecond,
			Success:     i < successCount,
			Metrics: map[string]float64{
				"cpu_usage":    0.1 + float64(i)*0.01,
				"memory_usage": 0.2 + float64(i)*0.02,
			},
			Metadata: map[string]interface{}{
				"node": "test-node-" + string(rune(i%3)),
			},
		}
	}
	return data
}

func createTestMLModel() *MLModel {
	return &MLModel{
		ID:        "test-model-1",
		Type:      "classification",
		Version:   1,
		TrainedAt: time.Now(),
		Accuracy:  0.85,
		Features:  []string{"cpu_usage", "memory_usage", "alert_count", "duration"},
		Parameters: map[string]interface{}{
			"learning_rate": 0.01,
			"epochs":        100,
		},
		Weights: []float64{0.1, 0.2, 0.3, 0.4},
		Bias:    0.05,
		TrainingMetrics: &TrainingMetrics{
			TrainingSize:   800,
			ValidationSize: 200,
			TestSize:       100,
			Accuracy:       0.85,
			Precision:      0.82,
			Recall:         0.88,
			F1Score:        0.85,
		},
	}
}

func createTestCrossValidationMetrics(meanAcc, stdAcc float64) *CrossValidationMetrics {
	return &CrossValidationMetrics{
		Folds:        5,
		MeanAccuracy: meanAcc,
		StdAccuracy:  stdAcc,
		MeanF1:       meanAcc - 0.02, // Typically slightly lower than accuracy
		StdF1:        stdAcc + 0.01,
	}
}

func generateExecutionID(i int) string {
	if i%10 == 0 {
		return "" // Create some missing IDs for data quality testing
	}
	return "exec-" + string(rune(48+i%10))
}

func generateWorkflowID(i int) string {
	if i%15 == 0 {
		return "" // Create some missing workflow IDs
	}
	return "workflow-" + string(rune(65+i%5))
}

// StatisticalValidator Tests

func TestNewStatisticalValidator(t *testing.T) {
	config := &PatternDiscoveryConfig{
		MinExecutionsForPattern: 10,
	}
	logger := logrus.New()

	validator := NewStatisticalValidator(config, logger)

	if validator == nil {
		t.Fatal("Expected non-nil StatisticalValidator")
	}
	if validator.config != config {
		t.Error("Expected config to be set correctly")
	}
	if validator.log != logger {
		t.Error("Expected logger to be set correctly")
	}
}

func TestStatisticalValidator_ValidateStatisticalAssumptions(t *testing.T) {
	config := &PatternDiscoveryConfig{
		MinExecutionsForPattern: 10,
	}
	validator := NewStatisticalValidator(config, logrus.New())

	tests := []struct {
		name           string
		dataSize       int
		successRate    float64
		expectedValid  bool
		expectedSample bool
	}{
		{
			name:           "Adequate sample size with good success rate",
			dataSize:       25,
			successRate:    0.8,
			expectedValid:  true,
			expectedSample: true,
		},
		{
			name:           "Inadequate sample size",
			dataSize:       15,
			successRate:    0.8,
			expectedValid:  false,
			expectedSample: false,
		},
		{
			name:           "Very small sample",
			dataSize:       5,
			successRate:    0.8,
			expectedValid:  false,
			expectedSample: false,
		},
		{
			name:           "Large sample with variable success",
			dataSize:       100,
			successRate:    0.6,
			expectedValid:  true,
			expectedSample: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := createTestWorkflowExecutionData(tt.dataSize, tt.successRate)
			result := validator.ValidateStatisticalAssumptions(data)

			if result.IsValid != tt.expectedValid {
				t.Errorf("Expected IsValid=%v, got=%v", tt.expectedValid, result.IsValid)
			}
			if result.SampleSizeAdequate != tt.expectedSample {
				t.Errorf("Expected SampleSizeAdequate=%v, got=%v", tt.expectedSample, result.SampleSizeAdequate)
			}
			if len(result.Assumptions) == 0 {
				t.Error("Expected at least one assumption check")
			}
			if result.DataQualityScore < 0 || result.DataQualityScore > 1 {
				t.Errorf("DataQualityScore should be between 0 and 1, got=%v", result.DataQualityScore)
			}
		})
	}
}

func TestStatisticalValidator_AssessReliability(t *testing.T) {
	config := &PatternDiscoveryConfig{
		MinExecutionsForPattern: 10,
	}
	validator := NewStatisticalValidator(config, logrus.New())

	tests := []struct {
		name             string
		dataSize         int
		successRate      float64
		expectedReliable bool
		minReliableScore float64
	}{
		{
			name:             "Large reliable dataset",
			dataSize:         100,
			successRate:      0.8,
			expectedReliable: true,
			minReliableScore: 0.7,
		},
		{
			name:             "Small unreliable dataset",
			dataSize:         15,
			successRate:      0.8,
			expectedReliable: false,
			minReliableScore: 0.0,
		},
		{
			name:             "Medium dataset",
			dataSize:         50,
			successRate:      0.75,
			expectedReliable: true,
			minReliableScore: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := createTestWorkflowExecutionData(tt.dataSize, tt.successRate)
			assessment := validator.AssessReliability(data)

			if assessment.IsReliable != tt.expectedReliable {
				t.Errorf("Expected IsReliable=%v, got=%v", tt.expectedReliable, assessment.IsReliable)
			}
			if assessment.ActualSize != tt.dataSize {
				t.Errorf("Expected ActualSize=%v, got=%v", tt.dataSize, assessment.ActualSize)
			}
			if assessment.ReliabilityScore < tt.minReliableScore {
				t.Errorf("Expected ReliabilityScore>=%v, got=%v", tt.minReliableScore, assessment.ReliabilityScore)
			}
			if assessment.RecommendedMinSize <= 0 {
				t.Error("Expected positive RecommendedMinSize")
			}
			if assessment.DataQuality < 0 || assessment.DataQuality > 1 {
				t.Errorf("DataQuality should be between 0 and 1, got=%v", assessment.DataQuality)
			}
		})
	}
}

func TestStatisticalValidator_CalculateDataQuality(t *testing.T) {
	validator := NewStatisticalValidator(&PatternDiscoveryConfig{}, logrus.New())

	tests := []struct {
		name     string
		data     []*engine.WorkflowExecutionData
		minScore float64
		maxScore float64
	}{
		{
			name:     "Empty data",
			data:     []*engine.WorkflowExecutionData{},
			minScore: 0.0,
			maxScore: 0.0,
		},
		{
			name: "Complete data",
			data: []*engine.WorkflowExecutionData{
				{
					ExecutionID: "exec-1",
					WorkflowID:  "workflow-1",
					Timestamp:   time.Now(),
					Duration:    100 * time.Millisecond,
				},
			},
			minScore: 1.0,
			maxScore: 1.0,
		},
		{
			name: "Partial data",
			data: []*engine.WorkflowExecutionData{
				{
					ExecutionID: "exec-1",
					WorkflowID:  "", // Missing workflow ID
					Timestamp:   time.Now(),
					Duration:    100 * time.Millisecond,
				},
			},
			minScore: 0.7,
			maxScore: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quality := validator.calculateDataQuality(tt.data)
			if quality < tt.minScore || quality > tt.maxScore {
				t.Errorf("Expected quality between %v and %v, got %v", tt.minScore, tt.maxScore, quality)
			}
		})
	}
}

func TestStatisticalValidator_CalculateMinSampleSize(t *testing.T) {
	validator := NewStatisticalValidator(&PatternDiscoveryConfig{}, logrus.New())

	tests := []struct {
		name    string
		power   float64
		alpha   float64
		minSize int
	}{
		{
			name:    "Standard power analysis",
			power:   0.8,
			alpha:   0.05,
			minSize: 30,
		},
		{
			name:    "High power analysis",
			power:   0.9,
			alpha:   0.01,
			minSize: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := validator.calculateMinSampleSize(tt.power, tt.alpha)
			if size < tt.minSize {
				t.Errorf("Expected minimum size %v, got %v", tt.minSize, size)
			}
		})
	}
}

// OverfittingPrevention Tests

func TestNewOverfittingPrevention(t *testing.T) {
	config := &PatternDiscoveryConfig{
		MinExecutionsForPattern: 10,
	}
	logger := logrus.New()

	op := NewOverfittingPrevention(config, logger)

	if op == nil {
		t.Fatal("Expected non-nil OverfittingPrevention")
	}
	if op.config != config {
		t.Error("Expected config to be set correctly")
	}
	if op.log != logger {
		t.Error("Expected logger to be set correctly")
	}
}

func TestOverfittingPrevention_AssessOverfittingRisk(t *testing.T) {
	config := &PatternDiscoveryConfig{
		MinExecutionsForPattern: 10,
	}
	op := NewOverfittingPrevention(config, logrus.New())

	tests := []struct {
		name              string
		trainingDataSize  int
		modelComplexity   int
		crossValMeanAcc   float64
		crossValStdAcc    float64
		expectedRiskLevel OverfittingRisk
		expectedReliable  bool
	}{
		{
			name:              "Low risk - simple model, large data",
			trainingDataSize:  1000,
			modelComplexity:   10,
			crossValMeanAcc:   0.85,
			crossValStdAcc:    0.02,
			expectedRiskLevel: OverfittingRiskLow,
			expectedReliable:  true,
		},
		{
			name:              "High risk - complex model, small data",
			trainingDataSize:  50,
			modelComplexity:   20,
			crossValMeanAcc:   0.85,
			crossValStdAcc:    0.15,
			expectedRiskLevel: OverfittingRiskHigh,
			expectedReliable:  false,
		},
		{
			name:              "Moderate risk - balanced scenario",
			trainingDataSize:  200,
			modelComplexity:   15,
			crossValMeanAcc:   0.82,
			crossValStdAcc:    0.08,
			expectedRiskLevel: OverfittingRiskModerate,
			expectedReliable:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trainingData := createTestWorkflowExecutionData(tt.trainingDataSize, 0.8)
			model := createTestMLModel()
			model.Weights = make([]float64, tt.modelComplexity) // Adjust model complexity
			crossValMetrics := createTestCrossValidationMetrics(tt.crossValMeanAcc, tt.crossValStdAcc)

			assessment := op.AssessOverfittingRisk(trainingData, model, crossValMetrics)

			if assessment.OverfittingRisk != tt.expectedRiskLevel {
				t.Errorf("Expected risk level %v, got %v", tt.expectedRiskLevel, assessment.OverfittingRisk)
			}
			if assessment.IsModelReliable != tt.expectedReliable {
				t.Errorf("Expected model reliable %v, got %v", tt.expectedReliable, assessment.IsModelReliable)
			}
			if len(assessment.Indicators) == 0 {
				t.Error("Expected at least one overfitting indicator")
			}
			if len(assessment.Recommendations) == 0 {
				t.Error("Expected at least one recommendation")
			}
			if len(assessment.PreventionStrategies) == 0 {
				t.Error("Expected at least one prevention strategy")
			}
			if assessment.RiskScore < 0 || assessment.RiskScore > 1 {
				t.Errorf("RiskScore should be between 0 and 1, got %v", assessment.RiskScore)
			}
		})
	}
}

func TestOverfittingPrevention_CheckModelComplexity(t *testing.T) {
	op := NewOverfittingPrevention(&PatternDiscoveryConfig{}, logrus.New())

	tests := []struct {
		name           string
		model          *MLModel
		sampleSize     int
		expectedDetect bool
		expectedValue  float64
	}{
		{
			name: "Simple model, large sample",
			model: &MLModel{
				Features: []string{"f1", "f2"},
				Weights:  []float64{0.1, 0.2},
			},
			sampleSize:     100,
			expectedDetect: false,
			expectedValue:  0.02, // 2/100
		},
		{
			name: "Complex model, small sample",
			model: &MLModel{
				Features: []string{"f1", "f2", "f3", "f4", "f5"},
				Weights:  make([]float64, 20),
			},
			sampleSize:     50,
			expectedDetect: true,
			expectedValue:  0.4, // 20/50
		},
		{
			name:           "Nil model",
			model:          nil,
			sampleSize:     100,
			expectedDetect: false,
			expectedValue:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indicator := op.checkModelComplexity(tt.model, tt.sampleSize)

			if indicator.Detected != tt.expectedDetect {
				t.Errorf("Expected detected=%v, got=%v", tt.expectedDetect, indicator.Detected)
			}
			if tt.model != nil && abs(indicator.Value-tt.expectedValue) > 0.01 {
				t.Errorf("Expected value≈%v, got=%v", tt.expectedValue, indicator.Value)
			}
			if indicator.Name != "model_complexity" {
				t.Errorf("Expected name='model_complexity', got=%v", indicator.Name)
			}
		})
	}
}

func TestOverfittingPrevention_CheckCrossValidationVariance(t *testing.T) {
	op := NewOverfittingPrevention(&PatternDiscoveryConfig{}, logrus.New())

	tests := []struct {
		name           string
		stdAccuracy    float64
		expectedDetect bool
	}{
		{
			name:           "Low variance",
			stdAccuracy:    0.02,
			expectedDetect: false,
		},
		{
			name:           "High variance",
			stdAccuracy:    0.15,
			expectedDetect: true,
		},
		{
			name:           "Threshold variance",
			stdAccuracy:    0.05,
			expectedDetect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &CrossValidationMetrics{
				Folds:        5,
				MeanAccuracy: 0.85,
				StdAccuracy:  tt.stdAccuracy,
				MeanF1:       0.83,
				StdF1:        tt.stdAccuracy + 0.01,
			}

			indicator := op.checkCrossValidationVariance(metrics)

			if indicator.Detected != tt.expectedDetect {
				t.Errorf("Expected detected=%v, got=%v", tt.expectedDetect, indicator.Detected)
			}
			if indicator.Value != tt.stdAccuracy {
				t.Errorf("Expected value=%v, got=%v", tt.stdAccuracy, indicator.Value)
			}
			if indicator.Name != "cross_validation_variance" {
				t.Errorf("Expected name='cross_validation_variance', got=%v", indicator.Name)
			}
		})
	}
}

func TestOverfittingPrevention_DetermineRiskLevel(t *testing.T) {
	op := NewOverfittingPrevention(&PatternDiscoveryConfig{}, logrus.New())

	tests := []struct {
		riskScore    float64
		expectedRisk OverfittingRisk
	}{
		{0.1, OverfittingRiskLow},
		{0.2, OverfittingRiskLow},
		{0.4, OverfittingRiskModerate},
		{0.5, OverfittingRiskModerate},
		{0.7, OverfittingRiskHigh},
		{0.9, OverfittingRiskCritical},
		{1.0, OverfittingRiskCritical},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			risk := op.determineRiskLevel(tt.riskScore)
			if risk != tt.expectedRisk {
				t.Errorf("Score %v: expected %v, got %v", tt.riskScore, tt.expectedRisk, risk)
			}
		})
	}
}

func TestOverfittingPrevention_DetermineSeverity(t *testing.T) {
	op := NewOverfittingPrevention(&PatternDiscoveryConfig{}, logrus.New())

	tests := []struct {
		value            float64
		threshold        float64
		expectedSeverity string
	}{
		{0.05, 0.1, "low"},
		{0.1, 0.1, "moderate"},
		{0.15, 0.1, "high"},
		{0.25, 0.1, "critical"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			severity := op.determineSeverity(tt.value, tt.threshold)
			if severity != tt.expectedSeverity {
				t.Errorf("Value %v, threshold %v: expected %v, got %v",
					tt.value, tt.threshold, tt.expectedSeverity, severity)
			}
		})
	}
}

// Benchmark Tests

func BenchmarkStatisticalValidator_ValidateAssumptions(b *testing.B) {
	config := &PatternDiscoveryConfig{MinExecutionsForPattern: 10}
	validator := NewStatisticalValidator(config, logrus.New())
	data := createTestWorkflowExecutionData(1000, 0.8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateStatisticalAssumptions(data)
	}
}

func BenchmarkOverfittingPrevention_AssessRisk(b *testing.B) {
	config := &PatternDiscoveryConfig{MinExecutionsForPattern: 10}
	op := NewOverfittingPrevention(config, logrus.New())
	trainingData := createTestWorkflowExecutionData(1000, 0.8)
	model := createTestMLModel()
	crossVal := createTestCrossValidationMetrics(0.85, 0.03)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = op.AssessOverfittingRisk(trainingData, model, crossVal)
	}
}

// Helper function
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
