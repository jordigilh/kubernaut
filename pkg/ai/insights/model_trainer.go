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
	"fmt"
	"math"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	"github.com/sirupsen/logrus"
)

// ModelType represents the type of AI model
type ModelType string

const (
	ModelTypeEffectivenessPrediction ModelType = "effectiveness_prediction"
	ModelTypeActionClassification    ModelType = "action_classification"
	ModelTypeOscillationDetection    ModelType = "oscillation_detection"
	ModelTypePatternRecognition      ModelType = "pattern_recognition"
)

// ModelTrainingResult represents the result of model training
type ModelTrainingResult struct {
	ModelType          string                 `json:"model_type"`
	Success            bool                   `json:"success"`
	FinalAccuracy      float64                `json:"final_accuracy"`
	ValidationAccuracy float64                `json:"validation_accuracy"`
	TrainingDuration   time.Duration          `json:"training_duration"`
	TrainingLogs       []string               `json:"training_logs"`
	OverfittingRisk    shared.OverfittingRisk `json:"overfitting_risk"`
}

// FeatureVector represents extracted features for ML training per BR-AI-003
type FeatureVector struct {
	ActionType         string
	AlertSeverity      string
	CPUUsage           float64
	MemoryUsage        float64
	DiskIO             float64
	NetworkThroughput  float64
	PodCount           float64
	DeploymentAge      float64
	TimeOfDay          float64
	DayOfWeek          float64
	EffectivenessScore float64
	// Feature importance weights
	FeatureImportance map[string]float64
}

// CrossValidationResult represents k-fold validation results
type CrossValidationResult struct {
	Accuracy    float64
	Precision   float64
	Recall      float64
	F1Score     float64
	FoldResults []float64
}

// ModelTrainer handles training of AI models per BR-AI-003: Model Training and Optimization
type ModelTrainer struct {
	actionHistoryRepo  actionhistory.Repository
	vectorDB           interface{} // AIInsightsVectorDatabase interface
	overfittingConfig  interface{} // Overfitting prevention config
	logger             *logrus.Logger
	modelSaveThreshold float64 // BR-AI-003: Models must achieve >85% accuracy
}

// NewModelTrainer creates a new model trainer instance
func NewModelTrainer(
	actionHistoryRepo actionhistory.Repository,
	vectorDB interface{},
	overfittingConfig interface{},
	logger *logrus.Logger,
) *ModelTrainer {
	return &ModelTrainer{
		actionHistoryRepo:  actionHistoryRepo,
		vectorDB:           vectorDB,
		overfittingConfig:  overfittingConfig,
		logger:             logger,
		modelSaveThreshold: 0.85, // BR-AI-003: Models must achieve >85% accuracy
	}
}

// TrainModels trains the specified model type with given time window per BR-AI-003
// Implements: feature engineering, cross-validation, hyperparameter optimization, early stopping
func (mt *ModelTrainer) TrainModels(ctx context.Context, modelType ModelType, timeWindow time.Duration) (*ModelTrainingResult, error) {
	startTime := time.Now()
	trainingLogs := []string{
		fmt.Sprintf("Starting model training for type: %s", modelType),
		fmt.Sprintf("Training time window: %v", timeWindow),
	}

	mt.logger.WithFields(logrus.Fields{
		"model_type":  modelType,
		"time_window": timeWindow,
	}).Info("BR-AI-003: Starting model training and optimization")

	// Step 1: Get training data with proper error handling per development guidelines
	query := actionhistory.ActionQuery{
		TimeRange: actionhistory.ActionHistoryTimeRange{
			Start: time.Now().Add(-timeWindow),
			End:   time.Now(),
		},
		Limit: 100000, // Support large datasets per BR-AI-003 (50k+ samples requirement)
	}

	traces, err := mt.actionHistoryRepo.GetActionTraces(ctx, query)
	if err != nil {
		mt.logger.WithError(err).Error("BR-AI-003: Failed to retrieve training data")
		return nil, fmt.Errorf("failed to prepare training data: %w", err)
	}

	trainingLogs = append(trainingLogs, fmt.Sprintf("Retrieved %d training samples", len(traces)))
	mt.logger.WithField("sample_count", len(traces)).Info("BR-AI-003: Training data retrieval completed")

	// Step 2: BR-AI-003 Requirement - Handle insufficient data scenarios gracefully
	if len(traces) < 50 {
		trainingLogs = append(trainingLogs, "insufficient training data", "minimum 50 samples required for reliable training")
		mt.logger.Warn("BR-AI-003: Insufficient training data for model training")
		return &ModelTrainingResult{
			ModelType:          string(modelType),
			Success:            false,
			FinalAccuracy:      0.0,
			ValidationAccuracy: 0.0,
			TrainingDuration:   time.Since(startTime),
			TrainingLogs:       trainingLogs,
			OverfittingRisk:    shared.OverfittingRiskHigh, // High risk due to insufficient data
		}, nil
	}

	// Step 3: BR-AI-003 Feature Engineering - Extract relevant features from action context, metrics, and outcomes
	featureVectors := mt.extractFeatures(traces, trainingLogs)
	trainingLogs = append(trainingLogs, fmt.Sprintf("Feature extraction completed for %d samples", len(featureVectors)))

	// Step 4: BR-AI-003 Model Training Pipeline - Support multiple model types
	modelResult := mt.trainModelByType(ctx, modelType, featureVectors, trainingLogs)

	// Step 5: BR-AI-003 Cross-validation for model performance assessment
	cvResult := mt.performCrossValidation(featureVectors, modelType)
	if cvResult != nil {
		modelResult.ValidationAccuracy = cvResult.Accuracy
		trainingLogs = append(trainingLogs,
			fmt.Sprintf("Cross-validation completed: accuracy=%.3f, precision=%.3f, recall=%.3f",
				cvResult.Accuracy, cvResult.Precision, cvResult.Recall))

		// BR-AI-003: Detect overfitting through validation performance gap analysis
		validationGap := math.Abs(modelResult.FinalAccuracy - cvResult.Accuracy)
		if validationGap > 0.15 {
			modelResult.OverfittingRisk = shared.OverfittingRiskHigh
		} else if validationGap > 0.08 {
			modelResult.OverfittingRisk = shared.OverfittingRiskModerate
		} else {
			modelResult.OverfittingRisk = shared.OverfittingRiskLow
		}
	} else {
		// Continue with training but mark overfitting risk as high
		modelResult.OverfittingRisk = shared.OverfittingRiskHigh
		trainingLogs = append(trainingLogs, "cross-validation failed, using training accuracy only")
	}

	// Step 6: BR-AI-003 Performance Requirements Validation
	trainingDuration := time.Since(startTime)
	modelResult.TrainingDuration = trainingDuration

	// BR-AI-003: Training must complete within 10 minutes for 50,000+ samples
	if len(traces) >= 50000 && trainingDuration > 10*time.Minute {
		trainingLogs = append(trainingLogs, "WARNING: Training exceeded 10-minute limit for large dataset")
		mt.logger.WithFields(logrus.Fields{
			"sample_count": len(traces),
			"duration":     trainingDuration,
		}).Warn("BR-AI-003: Training performance requirement not met")
	}

	// BR-AI-003: Models must achieve >85% accuracy in effectiveness prediction
	if modelResult.FinalAccuracy < mt.modelSaveThreshold {
		modelResult.Success = false
		trainingLogs = append(trainingLogs, fmt.Sprintf("Model accuracy %.3f below threshold %.3f",
			modelResult.FinalAccuracy, mt.modelSaveThreshold))
	}

	modelResult.TrainingLogs = trainingLogs

	mt.logger.WithFields(logrus.Fields{
		"model_type":          modelType,
		"final_accuracy":      modelResult.FinalAccuracy,
		"validation_accuracy": modelResult.ValidationAccuracy,
		"training_duration":   trainingDuration,
		"overfitting_risk":    modelResult.OverfittingRisk,
		"sample_count":        len(traces),
	}).Info("BR-AI-003: Model training and optimization completed")

	return modelResult, nil
}
