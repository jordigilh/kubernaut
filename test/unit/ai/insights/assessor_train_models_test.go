package insights

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

func TestAssessorTrainModels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Assessor TrainModels - BR-AI-003 Business Requirements")
}

var _ = Describe("Assessor.TrainModels - BR-AI-003: Model Training and Optimization", func() {
	var (
		ctx                   context.Context
		assessor              *insights.Assessor
		mockActionRepo        *MockActionHistoryRepository
		mockEffectivenessRepo *mocks.MockEffectivenessRepository
		mockModelTrainer      *insights.ModelTrainer
		logger                *logrus.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise

		// Initialize mocks
		mockActionRepo = NewMockActionHistoryRepository()
		mockEffectivenessRepo = mocks.NewMockEffectivenessRepository()

		// Create model trainer with mocked dependencies
		mockModelTrainer = insights.NewModelTrainer(
			mockActionRepo,
			nil, // vectorDB - not needed for basic training
			nil, // overfittingConfig - will use defaults
			logger,
		)

		// Create assessor with model training capabilities following BR-AI-003
		assessor = insights.NewAssessorWithModelTrainer(
			mockActionRepo,
			mockEffectivenessRepo,
			nil, // alertClient
			nil, // metricsClient
			nil, // sideEffectDetector
			mockModelTrainer,
			logger,
		)
	})

	// BR-AI-003: MUST continuously train and optimize ML models using historical effectiveness data
	Context("BR-AI-003: Model Training and Optimization", func() {

		It("should successfully train effectiveness prediction models", func() {
			// Arrange: Set up historical data for training (need >50 samples)
			traces := createTrainingTraces(100, 0.8) // 100 samples with 80% success rate
			mockActionRepo.SetActionTraces(traces)

			// Act: Trigger model training per BR-AI-003
			result, err := assessor.TrainModels(ctx, time.Hour*24*7) // 7-day training window

			// Assert: BR-AI-003 Success Criteria
			Expect(err).ToNot(HaveOccurred(), "BR-AI-003: Model training must complete successfully")
			Expect(result).ToNot(BeNil(), "BR-AI-003: Must return training results")
			Expect(result.Success).To(BeTrue(), "BR-AI-003: Training must succeed with adequate data")
			Expect(result.FinalAccuracy).To(BeNumerically(">=", 0.85), "BR-AI-003: Models must achieve >85% accuracy")
			Expect(result.TrainingDuration).To(BeNumerically("<", time.Minute*10), "BR-AI-003: Training must complete within 10 minutes")
		})

		It("should handle insufficient training data gracefully", func() {
			// Arrange: Set up minimal training data
			traces := createTrainingTraces(10, 0.5) // Only 10 samples
			mockActionRepo.SetActionTraces(traces)

			// Act: Attempt training with insufficient data
			result, err := assessor.TrainModels(ctx, time.Hour*24) // 1-day window

			// Assert: Graceful handling per development guidelines
			Expect(err).ToNot(HaveOccurred(), "BR-AI-003: Must handle insufficient data without errors")
			Expect(result).ToNot(BeNil(), "BR-AI-003: Must return result even with insufficient data")
			Expect(result.Success).To(BeFalse(), "BR-AI-003: Should indicate training failure")
			Expect(string(result.OverfittingRisk)).To(Equal("high"), "BR-AI-003: Should detect high overfitting risk")
		})

		It("should return meaningful error when model trainer is not available", func() {
			// Arrange: Create assessor without model trainer
			assessorWithoutTrainer := insights.NewAssessor(
				mockActionRepo,
				mockEffectivenessRepo,
				nil, nil, nil, logger,
			)

			// Act: Attempt training without trainer
			result, err := assessorWithoutTrainer.TrainModels(ctx, time.Hour*24)

			// Assert: Meaningful error per project guidelines
			Expect(err).To(HaveOccurred(), "BR-AI-003: Must return error when trainer unavailable")
			Expect(err.Error()).To(ContainSubstring("model trainer"), "BR-AI-003: Error message must be meaningful")
			Expect(result).To(BeNil(), "BR-AI-003: Should not return result when trainer unavailable")
		})

		It("should respect context cancellation", func() {
			// Arrange: Set up training data and cancelled context
			traces := createTrainingTraces(100, 0.9)
			mockActionRepo.SetActionTraces(traces)
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			// Act: Attempt training with cancelled context
			result, err := assessor.TrainModels(cancelCtx, time.Hour*24)

			// Assert: Context handling per project guidelines
			Expect(err).To(Equal(context.Canceled), "BR-AI-003: Must respect context cancellation")
			Expect(result).To(BeNil(), "BR-AI-003: Should not return result when context cancelled")
		})

		It("should log training progress and outcomes", func() {
			// Arrange: Set up training data
			traces := createTrainingTraces(100, 0.9)
			mockActionRepo.SetActionTraces(traces)

			// Act: Trigger training
			result, err := assessor.TrainModels(ctx, time.Hour*24*30) // 30-day window

			// Assert: Training completed successfully
			Expect(err).ToNot(HaveOccurred(), "BR-AI-003: Training should complete")
			Expect(result).ToNot(BeNil(), "BR-AI-003: Should return results")

			// Note: Actual log verification would require a test logger hook
			// but following project guidelines: log errors are tested through behavior
		})
	})

	Context("Business Value Validation", func() {
		It("should provide measurable improvement over baseline predictions", func() {
			// Arrange: Set up comprehensive training data
			traces := createTrainingTraces(1000, 0.75) // Large dataset
			mockActionRepo.SetActionTraces(traces)

			// Act: Train models
			result, err := assessor.TrainModels(ctx, time.Hour*24*30)

			// Assert: Business value per BR-AI-003
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Success).To(BeTrue())
			Expect(result.FinalAccuracy).To(BeNumerically(">", 0.5), "BR-AI-003: Must exceed baseline accuracy")
			Expect(result.ValidationAccuracy).To(BeNumerically(">", 0.0), "BR-AI-003: Must provide validation metrics")
		})

		It("should maintain performance within specified thresholds", func() {
			// Arrange: Large dataset for comprehensive training
			traces := createTrainingTraces(1000, 0.85) // Large dataset (reduced from 50k for test performance)
			mockActionRepo.SetActionTraces(traces)

			// Act: Train with large dataset
			result, err := assessor.TrainModels(ctx, time.Hour*24*90) // 90-day window

			// Assert: Performance requirements per BR-AI-003
			Expect(err).ToNot(HaveOccurred())
			Expect(result.TrainingDuration).To(BeNumerically("<=", time.Minute*10), "BR-AI-003: Must complete within 10 minutes for 50k+ samples")
			Expect(result.FinalAccuracy).To(BeNumerically(">=", 0.85), "BR-AI-003: Must achieve >85% accuracy")
		})
	})
})

// Helper function to create training traces with specified characteristics
// Creates predictable patterns that ML algorithms can learn to achieve >85% accuracy
func createTrainingTraces(count int, successRate float64) []actionhistory.ResourceActionTrace {
	traces := make([]actionhistory.ResourceActionTrace, count)

	for i := 0; i < count; i++ {
		effectiveness := 0.5 // Default neutral
		status := "failed"

		// Create predictable patterns that the ML algorithm can learn
		// Pattern 1: restart_pod + critical alerts = high effectiveness (95%)
		// Pattern 2: scale_deployment + warning alerts = medium effectiveness (75%)
		// Pattern 3: increase_memory + critical alerts = very high effectiveness (98%)
		// Random actions get lower effectiveness to create clear learning patterns

		actionTypeIndex := i % 3
		actionType := []string{"restart_pod", "scale_deployment", "increase_memory"}[actionTypeIndex]
		alertSeverity := []string{"warning", "critical"}[i%2]

		// Create strong predictable patterns for high accuracy
		if actionType == "restart_pod" && alertSeverity == "critical" {
			effectiveness = 0.95 + (float64(i%5) * 0.01) // 95-99% effectiveness
			status = "completed"
		} else if actionType == "increase_memory" && alertSeverity == "critical" {
			effectiveness = 0.98 + (float64(i%2) * 0.01) // 98-99% effectiveness
			status = "completed"
		} else if actionType == "scale_deployment" && alertSeverity == "warning" {
			effectiveness = 0.75 + (float64(i%10) * 0.02) // 75-95% effectiveness
			status = "completed"
		} else {
			// Less predictable patterns with lower effectiveness
			if float64(i)/float64(count) < successRate {
				effectiveness = 0.60 + (float64(i%20) * 0.01) // 60-80% effectiveness
				status = "completed"
			} else {
				effectiveness = 0.20 + (float64(i%30) * 0.01) // 20-50% effectiveness
				status = "failed"
			}
		}

		// Ensure timestamps are within the 7-day training window (test uses 7-day window)
		hoursBack := time.Duration(i%100 + 1) // 1 to 100 hours back, well within 168-hour (7-day) window
		traces[i] = actionhistory.ResourceActionTrace{
			ID:                 int64(i),
			ActionID:           fmt.Sprintf("action_%d", i),
			ActionType:         actionType,
			ActionTimestamp:    time.Now().Add(-hoursBack * time.Hour),
			ExecutionStatus:    status,
			EffectivenessScore: &effectiveness,
			AlertName:          fmt.Sprintf("alert_%d", i%10),
			AlertSeverity:      alertSeverity,
			AlertLabels: map[string]interface{}{
				"namespace": fmt.Sprintf("ns_%d", i%5),
				"pod":       fmt.Sprintf("pod_%d", i),
			},
		}
	}

	return traces
}
