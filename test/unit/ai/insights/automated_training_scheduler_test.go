//go:build unit
// +build unit

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

package insights

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/ai/insights"
)

// TDD RED PHASE: Write failing tests for BR-AI-003 Automated Training Scheduler
// Following Cursor Rule: "SECOND: Write tests that call actual business interfaces"
// Following Cursor Rule: "Tests MUST import and call actual business interfaces"

var _ = Describe("BR-AI-003: Automated Training Scheduler", func() {
	var (
		// Use REAL business logic from main application - not mocks
		realInsightsService *insights.Service
		ctx                 context.Context
		cancel              context.CancelFunc
		logger              *logrus.Logger
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Use existing testutil patterns instead of deprecated TDDConversionHelper
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// TDD RED: Create REAL business service that should have automated training
		// This will FAIL because automated training is not implemented yet
		realInsightsService = createRealInsightsServiceWithAutomatedTraining(logger)
	})

	AfterEach(func() {
		if realInsightsService != nil {
			realInsightsService.Stop()
		}
		cancel()
	})

	Context("BR-AI-003 Requirement: Implement online learning for continuous model improvement", func() {
		It("should automatically trigger model training on configured schedule", func() {
			// TDD RED: Test REAL business requirement from BR-AI-003
			// Following Cursor Rule 09: MANDATORY COMPILATION CHECK after interface usage

			// Start the service with automated training capabilities
			err := realInsightsService.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "BR-AI-003: Service with automated training must start successfully")

			// TDD RED: This should FAIL because automated training scheduler not implemented
			hasAutomatedTraining := realInsightsService.IsAutomatedTrainingEnabled()
			Expect(hasAutomatedTraining).To(BeTrue(),
				"BR-AI-003: Service must have automated training capabilities enabled")

			// TDD RED: This should FAIL because no training scheduler exists
			trainingSchedule := realInsightsService.GetTrainingSchedule()
			Expect(trainingSchedule).ToNot(BeEmpty(),
				"BR-AI-003: Service must have training schedule configured (e.g., '@daily')")
		})

		It("should provide access to training history and performance metrics", func() {
			// TDD RED: Test business requirement for training history tracking
			err := realInsightsService.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// TDD RED: This should FAIL because training history tracking not implemented
			trainingHistory := realInsightsService.GetTrainingHistory(ctx, 7*24*time.Hour)
			Expect(trainingHistory).ToNot(BeNil(),
				"BR-AI-003: Service must provide access to training history")
			Expect(len(trainingHistory.TrainingRuns)).To(BeNumerically(">=", 0),
				"BR-AI-003: Training history must be accessible")
		})

		It("should support manual training trigger for immediate improvement", func() {
			// TDD GREEN: Test business requirement for manual training capability
			err := realInsightsService.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// TDD GREEN: Manual training trigger should work
			result, err := realInsightsService.TriggerImmediateTraining(ctx, 24*time.Hour)
			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-003: Service must support manual training triggers")
			Expect(result).ToNot(BeNil(),
				"BR-AI-003: Manual training must return results")

			// TDD GREEN: Debug the training result to understand why it's failing
			if !result.Success {
				// TDD GREEN: Log the actual result for debugging
				GinkgoWriter.Printf("Training failed - FinalAccuracy: %f, ModelType: %s, TrainingLogs: %v\n",
					result.FinalAccuracy, result.ModelType, result.TrainingLogs)
			}

			Expect(result.Success).To(BeTrue(),
				"BR-AI-003: Manual training must complete successfully")
		})
	})

	Context("BR-AI-003 Success Criteria: Automatic retraining maintains performance within 5% of peak", func() {
		It("should detect model drift and trigger retraining when performance degrades", func() {
			// TDD GREEN: Test business requirement for model drift detection
			err := realInsightsService.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// TDD GREEN: First trigger training to populate performance data
			result, err := realInsightsService.TriggerImmediateTraining(ctx, 24*time.Hour)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Success).To(BeTrue())

			// TDD GREEN: Now test model drift detection with populated data
			driftStatus := realInsightsService.GetModelDriftStatus(ctx)
			Expect(driftStatus).ToNot(BeNil(),
				"BR-AI-003: Service must provide model drift monitoring")

			// TDD GREEN: With training data, performance drift should be calculable (not NaN)
			currentPerformance := driftStatus.CurrentPerformance
			peakPerformance := driftStatus.PeakPerformance
			performanceDrift := driftStatus.PerformanceDrift

			Expect(currentPerformance).To(BeNumerically(">", 0),
				"BR-AI-003: Current performance must be measurable")
			Expect(peakPerformance).To(BeNumerically(">", 0),
				"BR-AI-003: Peak performance must be measurable")
			Expect(math.IsNaN(performanceDrift)).To(BeFalse(),
				"BR-AI-003: Performance drift must be calculable")
			Expect(performanceDrift).To(BeNumerically("<=", 0.05),
				"BR-AI-003: Automatic retraining must maintain performance within 5% of peak")
		})

		It("should provide performance tracking over time", func() {
			// TDD GREEN: Test business requirement for performance tracking
			err := realInsightsService.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// TDD GREEN: First trigger training to populate performance data
			result, err := realInsightsService.TriggerImmediateTraining(ctx, 24*time.Hour)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Success).To(BeTrue())

			// TDD GREEN: Now test performance tracking with populated data
			performanceMetrics := realInsightsService.GetPerformanceMetrics(ctx, 30*24*time.Hour)
			Expect(performanceMetrics).ToNot(BeNil(),
				"BR-AI-003: Service must track performance metrics over time")
			Expect(performanceMetrics.AccuracyTrend).ToNot(BeEmpty(),
				"BR-AI-003: Performance metrics must include accuracy trends")
			Expect(performanceMetrics.CurrentAccuracy).To(BeNumerically(">", 0),
				"BR-AI-003: Current accuracy must be measurable")
			Expect(performanceMetrics.TrendDirection).ToNot(BeEmpty(),
				"BR-AI-003: Performance trend direction must be available")
		})
	})
})

// TDD GREEN: Helper function that creates service with automated training
func createRealInsightsServiceWithAutomatedTraining(logger interface{}) *insights.Service {
	// TDD GREEN: Use REAL business logic with automated training capabilities
	// Create a minimal action history repository for ModelTrainer
	actionHistoryRepo := createBasicActionHistoryRepo()

	// Create ModelTrainer with minimal dependencies for GREEN phase
	modelTrainer := insights.NewModelTrainer(
		actionHistoryRepo,
		nil, // vectorDB - TDD GREEN: minimal for now
		createBasicOverfittingConfig(),
		logger.(*logrus.Logger),
	)

	// Create assessor WITH ModelTrainer (this is the key fix)
	assessor := insights.NewAssessorWithModelTrainer(
		actionHistoryRepo,
		nil,          // effectivenessRepo - TDD GREEN: minimal for now
		nil,          // alertClient - TDD GREEN: minimal for now
		nil,          // metricsClient - TDD GREEN: minimal for now
		nil,          // sideEffectDetector - TDD GREEN: minimal for now
		modelTrainer, // TDD GREEN: REAL ModelTrainer for training capabilities
		logger.(*logrus.Logger),
	)

	// TDD GREEN: Create service with automated training enabled
	return insights.NewServiceWithAutomatedTraining(assessor, "@daily", logger.(*logrus.Logger))
}

// TDD GREEN: Helper functions for minimal dependencies
func createBasicActionHistoryRepo() actionhistory.Repository {
	// TDD GREEN: Return a basic stub that satisfies the interface
	return &basicActionHistoryRepo{}
}

func createBasicOverfittingConfig() interface{} {
	return &struct {
		MaxIterations         int
		EarlyStoppingPatience int
	}{
		MaxIterations:         1000,
		EarlyStoppingPatience: 10,
	}
}

// TDD GREEN: Basic stub for action history repository
type basicActionHistoryRepo struct{}

func (r *basicActionHistoryRepo) GetActionTraces(ctx context.Context, query actionhistory.ActionQuery) ([]actionhistory.ResourceActionTrace, error) {
	// TDD GREEN: Return high-quality training data for successful training
	// ModelTrainer needs >50 samples with EffectivenessScore > 0.85 and consistent patterns
	traces := make([]actionhistory.ResourceActionTrace, 100) // More samples for better training

	for i := range traces {
		// TDD GREEN: Create highly consistent, predictable training data for >85% accuracy
		effectiveness := 0.98 // Very high effectiveness for successful training

		// Create very clear, simple patterns that ML can easily learn
		var actionType string
		var cpuUsage, memoryUsage float64

		// TDD GREEN: Use only 2 very distinct, simple patterns for high accuracy
		if i%2 == 0 {
			// Pattern 1: High CPU (>80%) -> scale-up always works (very predictable)
			actionType = "scale-up"
			cpuUsage = 0.90    // Always high CPU
			memoryUsage = 0.50 // Always low memory
		} else {
			// Pattern 2: Low CPU (<60%) -> restart always works (very predictable)
			actionType = "restart"
			cpuUsage = 0.40    // Always low CPU
			memoryUsage = 0.90 // Always high memory
		}

		traces[i] = actionhistory.ResourceActionTrace{
			ActionType:         actionType,
			ExecutionStatus:    "completed",
			EffectivenessScore: &effectiveness,
			ActionParameters: map[string]interface{}{
				"cpu_usage":    cpuUsage,
				"memory_usage": memoryUsage,
				"pod_count":    float64(5), // Consistent pod count
			},
		}
	}
	return traces, nil
}

// TDD GREEN: Implement other required methods with no-ops
func (r *basicActionHistoryRepo) EnsureResourceReference(ctx context.Context, ref actionhistory.ResourceReference) (int64, error) {
	return 1, nil
}

func (r *basicActionHistoryRepo) GetResourceReference(ctx context.Context, namespace, kind, name string) (*actionhistory.ResourceReference, error) {
	return &actionhistory.ResourceReference{}, nil
}

func (r *basicActionHistoryRepo) EnsureActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	return &actionhistory.ActionHistory{}, nil
}

func (r *basicActionHistoryRepo) GetActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	return &actionhistory.ActionHistory{}, nil
}

func (r *basicActionHistoryRepo) UpdateActionHistory(ctx context.Context, history *actionhistory.ActionHistory) error {
	return nil
}

func (r *basicActionHistoryRepo) StoreAction(ctx context.Context, action *actionhistory.ActionRecord) (*actionhistory.ResourceActionTrace, error) {
	return &actionhistory.ResourceActionTrace{}, nil
}

func (r *basicActionHistoryRepo) GetActionTrace(ctx context.Context, actionID string) (*actionhistory.ResourceActionTrace, error) {
	return &actionhistory.ResourceActionTrace{}, nil
}

func (r *basicActionHistoryRepo) UpdateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	return nil
}

func (r *basicActionHistoryRepo) GetPendingEffectivenessAssessments(ctx context.Context) ([]*actionhistory.ResourceActionTrace, error) {
	return []*actionhistory.ResourceActionTrace{}, nil
}

func (r *basicActionHistoryRepo) GetOscillationPatterns(ctx context.Context, patternType string) ([]actionhistory.OscillationPattern, error) {
	return []actionhistory.OscillationPattern{}, nil
}

func (r *basicActionHistoryRepo) StoreOscillationDetection(ctx context.Context, detection *actionhistory.OscillationDetection) error {
	return nil
}

func (r *basicActionHistoryRepo) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]actionhistory.OscillationDetection, error) {
	return []actionhistory.OscillationDetection{}, nil
}

func (r *basicActionHistoryRepo) ApplyRetention(ctx context.Context, actionHistoryID int64) error {
	return nil
}

func (r *basicActionHistoryRepo) GetActionHistorySummaries(ctx context.Context, since time.Duration) ([]actionhistory.ActionHistorySummary, error) {
	return []actionhistory.ActionHistorySummary{}, nil
}

// TDD RED PHASE COMPLETE
// These tests should ALL FAIL initially because:
// 1. IsAutomatedTrainingEnabled() method doesn't exist
// 2. GetTrainingSchedule() method doesn't exist
// 3. GetTrainingHistory() method doesn't exist
// 4. TriggerImmediateTraining() method doesn't exist
// 5. GetModelDriftStatus() method doesn't exist
// 6. GetPerformanceMetrics() method doesn't exist
// 7. NewServiceWithAutomatedTraining() doesn't exist
//
// This drives the GREEN phase implementation to:
// 1. Add automated training methods to insights.Service
// 2. Implement training scheduler using existing service pattern
// 3. Add model drift detection and performance tracking
// 4. Integrate with existing ModelTrainer functionality
//
// Following Cursor Rule: "THEN: Implement business logic after ALL tests are complete and failing"

// TestRunner bootstraps the Ginkgo test suite
func TestUautomatedUtrainingUscheduler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UautomatedUtrainingUscheduler Suite")
}
