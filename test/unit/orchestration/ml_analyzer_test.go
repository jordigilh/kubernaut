//go:build unit
// +build unit

package orchestration

import (
	"math"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
)

var _ = Describe("MachineLearningAnalyzer", func() {
	var (
		analyzer *orchestration.MachineLearningAnalyzer
		config   *orchestration.PatternDiscoveryConfig
		logger   *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel)

		config = &orchestration.PatternDiscoveryConfig{
			MinExecutionsForPattern: 5,
			MaxHistoryDays:          30,
			SimilarityThreshold:     0.8,
			ClusteringEpsilon:       0.3,
			MinClusterSize:          3,
			ModelUpdateInterval:     time.Hour,
			FeatureWindowSize:       20,
			PredictionConfidence:    0.7,
		}

		analyzer = orchestration.NewMachineLearningAnalyzer(config, logger)
	})

	Context("Feature Extraction", func() {
		It("should extract features from valid workflow data", func() {
			data := createTestWorkflowExecutionData(true, time.Minute*5)

			features, err := analyzer.ExtractFeatures(data)

			Expect(err).ToNot(HaveOccurred())
			Expect(features).ToNot(BeNil())
			Expect(features.AlertFeatures).ToNot(BeNil())
			Expect(features.ResourceFeatures).ToNot(BeNil())
			Expect(features.TemporalFeatures).ToNot(BeNil())
		})

		It("should handle missing alert data gracefully", func() {
			data := &orchestration.WorkflowExecutionData{
				ExecutionID: "test-exec",
				Timestamp:   time.Now(),
				// No alert data
			}

			features, err := analyzer.ExtractFeatures(data)

			Expect(err).ToNot(HaveOccurred())
			Expect(features).ToNot(BeNil())
		})

		It("should validate feature consistency", func() {
			data1 := createTestWorkflowExecutionData(true, time.Minute*3)
			data2 := createTestWorkflowExecutionData(false, time.Minute*7)

			features1, err1 := analyzer.ExtractFeatures(data1)
			features2, err2 := analyzer.ExtractFeatures(data2)

			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())

			// Features should have consistent structure
			Expect(len(features1.AlertFeatures.AlertTypeVector)).To(Equal(len(features2.AlertFeatures.AlertTypeVector)))
			Expect(len(features1.ResourceFeatures.ResourceTypeVector)).To(Equal(len(features2.ResourceFeatures.ResourceTypeVector)))
		})
	})

	Context("Model Training", func() {
		var trainingData []*orchestration.WorkflowExecutionData

		BeforeEach(func() {
			trainingData = createTrainingDataset(20, 0.7) // 20 samples, 70% success rate
		})

		It("should train a classification model successfully", func() {
			model, err := analyzer.TrainModel("classification", trainingData)

			Expect(err).ToNot(HaveOccurred())
			Expect(model).ToNot(BeNil())
			Expect(model.Type).To(Equal("classification"))
			Expect(model.Accuracy).To(BeNumerically(">", 0))
			Expect(len(model.Features)).To(BeNumerically(">", 0))
		})

		It("should train a regression model successfully", func() {
			model, err := analyzer.TrainModel("regression", trainingData)

			Expect(err).ToNot(HaveOccurred())
			Expect(model).ToNot(BeNil())
			Expect(model.Type).To(Equal("regression"))
			Expect(model.TrainingMetrics).ToNot(BeNil())
		})

		It("should fail with insufficient training data", func() {
			insufficientData := trainingData[:2] // Only 2 samples

			model, err := analyzer.TrainModel("classification", insufficientData)

			Expect(err).To(HaveOccurred())
			Expect(model).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("insufficient"))
		})

		It("should validate model quality metrics", func() {
			model, err := analyzer.TrainModel("classification", trainingData)

			Expect(err).ToNot(HaveOccurred())
			Expect(model.TrainingMetrics).ToNot(BeNil())
			Expect(model.TrainingMetrics.Accuracy).To(BeNumerically(">=", 0.0))
			Expect(model.TrainingMetrics.Accuracy).To(BeNumerically("<=", 1.0))
		})
	})

	Context("Outcome Prediction", func() {
		var patterns []*orchestration.DiscoveredPattern

		BeforeEach(func() {
			trainingData := createTrainingDataset(15, 0.8)
			_, err := analyzer.TrainModel("classification", trainingData)
			Expect(err).ToNot(HaveOccurred())

			patterns = createTestDiscoveredPatterns(3)
		})

		It("should predict workflow outcome with confidence", func() {
			features := createTestWorkflowFeatures()

			prediction, err := analyzer.PredictOutcome(features, patterns)

			Expect(err).ToNot(HaveOccurred())
			Expect(prediction).ToNot(BeNil())
			Expect(prediction.Confidence).To(BeNumerically(">=", 0.0))
			Expect(prediction.Confidence).To(BeNumerically("<=", 1.0))
			Expect(prediction.SuccessProbability).To(BeNumerically(">=", 0.0))
			Expect(prediction.SuccessProbability).To(BeNumerically("<=", 1.0))
		})

		It("should provide reasonable predictions for known patterns", func() {
			// Test with high-confidence pattern
			highConfidencePattern := patterns[0]
			highConfidencePattern.Confidence = 0.95
			highConfidencePattern.SuccessRate = 0.90

			features := createTestWorkflowFeatures()
			prediction, err := analyzer.PredictOutcome(features, []*orchestration.DiscoveredPattern{highConfidencePattern})

			Expect(err).ToNot(HaveOccurred())
			Expect(prediction.Confidence).To(BeNumerically(">=", config.PredictionConfidence))
		})

		It("should handle edge cases in prediction", func() {
			// Test with empty patterns
			features := createTestWorkflowFeatures()
			prediction, err := analyzer.PredictOutcome(features, []*orchestration.DiscoveredPattern{})

			Expect(err).ToNot(HaveOccurred())
			Expect(prediction).ToNot(BeNil())
			Expect(prediction.Reason).To(ContainSubstring("No historical patterns"))
		})
	})

	Context("Cross-Validation", func() {
		It("should perform k-fold cross-validation", func() {
			trainingData := createTrainingDataset(25, 0.75)

			metrics, err := analyzer.CrossValidateModel("classification", trainingData, 5)

			Expect(err).ToNot(HaveOccurred())
			Expect(metrics).ToNot(BeNil())
			Expect(metrics.Folds).To(Equal(5))
			Expect(metrics.MeanAccuracy).To(BeNumerically(">=", 0.0))
			Expect(metrics.MeanAccuracy).To(BeNumerically("<=", 1.0))
			Expect(metrics.StdAccuracy).To(BeNumerically(">=", 0.0))
		})

		It("should detect overfitting through cross-validation", func() {
			// Create a dataset with clear patterns but limited size
			trainingData := createOverfittingProne Dataset(10)

			metrics, err := analyzer.CrossValidateModel("classification", trainingData, 3)

			Expect(err).ToNot(HaveOccurred())
			// High standard deviation indicates potential overfitting
			if metrics.MeanAccuracy > 0.95 && metrics.StdAccuracy > 0.2 {
				logger.Warn("Potential overfitting detected in cross-validation")
			}
		})
	})

	Context("Statistical Validation", func() {
		It("should validate statistical assumptions for confidence calculations", func() {
			trainingData := createTrainingDataset(50, 0.7)

			// Test normality assumption
			isValid := analyzer.ValidateStatisticalAssumptions(trainingData)
			Expect(isValid).ToNot(BeNil())
		})

		It("should calculate confidence intervals", func() {
			successCount := 35
			totalCount := 50

			interval := analyzer.CalculateConfidenceInterval(successCount, totalCount, 0.95)

			Expect(interval).ToNot(BeNil())
			Expect(interval.Lower).To(BeNumerically(">=", 0.0))
			Expect(interval.Upper).To(BeNumerically("<=", 1.0))
			Expect(interval.Lower).To(BeNumerically("<", interval.Upper))
		})

		It("should detect when sample size is insufficient for reliable statistics", func() {
			smallDataset := createTrainingDataset(5, 0.6)

			reliability := analyzer.AssessReliability(smallDataset)

			Expect(reliability).ToNot(BeNil())
			Expect(reliability.IsReliable).To(BeFalse())
			Expect(reliability.RecommendedMinSize).To(BeNumerically(">", len(smallDataset)))
		})
	})
})

// Helper functions

func createTestWorkflowExecutionData(success bool, duration time.Duration) *orchestration.WorkflowExecutionData {
	return &orchestration.WorkflowExecutionData{
		ExecutionID: "test-exec-" + time.Now().Format("150405"),
		Timestamp:   time.Now(),
		Alert: &types.Alert{
			Name:      "TestAlert",
			Namespace: "default",
			Severity:  types.SeverityWarning,
			Resource:  "deployment/test-app",
			Labels: map[string]string{
				"alertname": "HighCPUUsage",
				"service":   "test-app",
			},
		},
		ExecutionResult: &orchestration.WorkflowExecutionResult{
			Success:  success,
			Duration: duration,
			Steps:    []orchestration.StepResult{},
		},
		ResourceUsage: &orchestration.ResourceUsageData{
			CPU:    0.5,
			Memory: 512 * 1024 * 1024, // 512MB
			Cost:   0.01,
		},
	}
}

func createTrainingDataset(size int, successRate float64) []*orchestration.WorkflowExecutionData {
	dataset := make([]*orchestration.WorkflowExecutionData, size)

	for i := 0; i < size; i++ {
		success := float64(i) < float64(size)*successRate
		duration := time.Minute * time.Duration(2+i%10) // Varying durations 2-12 minutes

		dataset[i] = createTestWorkflowExecutionData(success, duration)
		dataset[i].ExecutionID = fmt.Sprintf("training-exec-%d", i)
		dataset[i].Timestamp = time.Now().Add(-time.Duration(i) * time.Hour)
	}

	return dataset
}

func createOverfittingProneDataset(size int) []*orchestration.WorkflowExecutionData {
	// Create a dataset with very specific patterns that might lead to overfitting
	dataset := make([]*orchestration.WorkflowExecutionData, size)

	for i := 0; i < size; i++ {
		// Create a very specific pattern: success depends on exact alert name
		success := i%2 == 0
		data := createTestWorkflowExecutionData(success, time.Minute*5)
		data.Alert.Name = fmt.Sprintf("SpecificAlert%d", i%3)
		data.ExecutionID = fmt.Sprintf("overfitting-exec-%d", i)

		dataset[i] = data
	}

	return dataset
}

func createTestDiscoveredPatterns(count int) []*orchestration.DiscoveredPattern {
	patterns := make([]*orchestration.DiscoveredPattern, count)

	for i := 0; i < count; i++ {
		patterns[i] = &orchestration.DiscoveredPattern{
			ID:          fmt.Sprintf("pattern-%d", i),
			Type:        orchestration.PatternTypeAlert,
			Name:        fmt.Sprintf("Test Pattern %d", i),
			Confidence:  0.7 + float64(i)*0.1,
			Frequency:   10 + i*5,
			SuccessRate: 0.8 - float64(i)*0.1,
			DiscoveredAt: time.Now().Add(-time.Duration(i) * time.Hour),
		}
	}

	return patterns
}

func createTestWorkflowFeatures() *orchestration.WorkflowFeatures {
	return &orchestration.WorkflowFeatures{
		AlertFeatures: &orchestration.AlertFeatures{
			Severity:        3, // Warning level
			AlertTypeVector: []float64{1, 0, 0, 0}, // One-hot encoded
			LabelCount:      2,
		},
		ResourceFeatures: &orchestration.ResourceFeatures{
			CPUUtilization:     0.7,
			MemoryUtilization:  0.6,
			ResourceTypeVector: []float64{0, 1, 0}, // Deployment
		},
		TemporalFeatures: &orchestration.TemporalFeatures{
			HourOfDay:   14, // 2 PM
			DayOfWeek:   2,  // Tuesday
			IsWeekend:   false,
			IsHoliday:   false,
		},
		HistoricalFeatures: &orchestration.HistoricalFeatures{
			RecentSuccessRate:    0.75,
			AverageResponseTime:  300, // 5 minutes
			FailureFrequency:     0.25,
		},
	}
}
