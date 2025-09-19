package learning

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	sharedmath "github.com/jordigilh/kubernaut/pkg/shared/math"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	workflowtypes "github.com/jordigilh/kubernaut/pkg/workflow/types"
	"github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"
)

// MLConfig configures the machine learning analyzer
type MLConfig struct {
	MinExecutionsForPattern int     `yaml:"min_executions_for_pattern" default:"10"`
	MaxHistoryDays          int     `yaml:"max_history_days" default:"90"`
	SimilarityThreshold     float64 `yaml:"similarity_threshold" default:"0.85"`
	ClusteringEpsilon       float64 `yaml:"clustering_epsilon" default:"0.3"`
	MinClusterSize          int     `yaml:"min_cluster_size" default:"5"`
	ModelUpdateInterval     time.Duration
	FeatureWindowSize       int     `yaml:"feature_window_size" default:"50"`
	PredictionConfidence    float64 `yaml:"prediction_confidence" default:"0.7"`
}

// MachineLearningAnalyzer provides ML capabilities for pattern analysis
type MachineLearningAnalyzer struct {
	config           *MLConfig
	log              *logrus.Logger
	models           map[string]*MLModel
	featureExtractor *MLAnalyzerFeatureExtractor
	predictor        *MLAnalyzerOutcomePredictor
	clusterer        *MLAnalyzerClusterer
}

// MLModel represents a trained machine learning model
type MLModel struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"` // "regression", "classification", "clustering"
	Version         int                    `json:"version"`
	TrainedAt       time.Time              `json:"trained_at"`
	Accuracy        float64                `json:"accuracy"`
	Features        []string               `json:"features"`
	Parameters      map[string]interface{} `json:"parameters"`
	Weights         []float64              `json:"weights,omitempty"`
	Bias            float64                `json:"bias,omitempty"`
	TrainingMetrics *TrainingMetrics       `json:"training_metrics"`
}

// MLAnalyzer specific types
type MLAnalyzerFeatureExtractor struct {
	config *MLConfig
	log    *logrus.Logger
}

// MLAnalyzerOutcomePredictor predicts workflow outcomes
type MLAnalyzerOutcomePredictor struct {
	log *logrus.Logger
}

// MLAnalyzerClusterer performs clustering analysis
type MLAnalyzerClusterer struct {
	config *MLConfig
	log    *logrus.Logger
}

// TrainingMetrics contains model training statistics
type TrainingMetrics struct {
	TrainingSize    int                     `json:"training_size"`
	ValidationSize  int                     `json:"validation_size"`
	TestSize        int                     `json:"test_size"`
	Accuracy        float64                 `json:"accuracy"`
	Precision       float64                 `json:"precision"`
	Recall          float64                 `json:"recall"`
	F1Score         float64                 `json:"f1_score"`
	MAE             float64                 `json:"mae,omitempty"`      // Mean Absolute Error
	RMSE            float64                 `json:"rmse,omitempty"`     // Root Mean Square Error
	R2Score         float64                 `json:"r2_score,omitempty"` // R-squared
	CrossValidation *CrossValidationMetrics `json:"cross_validation,omitempty"`
}

// CrossValidationMetrics contains cross-validation results
type CrossValidationMetrics struct {
	Folds        int     `json:"folds"`
	MeanAccuracy float64 `json:"mean_accuracy"`
	StdAccuracy  float64 `json:"std_accuracy"`
	MeanF1       float64 `json:"mean_f1"`
	StdF1        float64 `json:"std_f1"`
}

// NewMachineLearningAnalyzer creates a new ML analyzer
func NewMachineLearningAnalyzer(config *MLConfig, log *logrus.Logger) *MachineLearningAnalyzer {
	mla := &MachineLearningAnalyzer{
		config:           config,
		log:              log,
		models:           make(map[string]*MLModel),
		featureExtractor: NewMLAnalyzerFeatureExtractor(config, log),
		predictor:        NewMLAnalyzerOutcomePredictor(log),
		clusterer:        NewMLAnalyzerClusterer(config, log),
	}

	// Initialize default models
	mla.initializeDefaultModels()

	return mla
}

// ExtractFeatures extracts features from workflow execution data
func (mla *MachineLearningAnalyzer) ExtractFeatures(data *sharedtypes.WorkflowExecutionData) (*shared.WorkflowFeatures, error) {
	return mla.featureExtractor.Extract(data)
}

// PredictOutcome predicts workflow outcome based on features and patterns
func (mla *MachineLearningAnalyzer) PredictOutcome(features *shared.WorkflowFeatures, patterns []*shared.DiscoveredPattern) (*shared.WorkflowPrediction, error) {
	return mla.predictor.Predict(features, patterns, mla.models)
}

// GetModelCount returns the number of loaded models
func (mla *MachineLearningAnalyzer) GetModelCount() int {
	return len(mla.models)
}

// GetModels returns a copy of the models map
func (mla *MachineLearningAnalyzer) GetModels() map[string]*MLModel {
	modelsCopy := make(map[string]*MLModel)
	for k, v := range mla.models {
		modelsCopy[k] = v
	}
	return modelsCopy
}

// TrainModel trains or updates an ML model
func (mla *MachineLearningAnalyzer) TrainModel(modelType string, trainingData []*sharedtypes.WorkflowExecutionData) (*MLModel, error) {
	mla.log.WithFields(logrus.Fields{
		"model_type":    modelType,
		"training_size": len(trainingData),
	}).Info("Training ML model")

	switch modelType {
	case "success_prediction":
		return mla.trainSuccessPredictionModel(trainingData)
	case "duration_prediction":
		return mla.trainDurationPredictionModel(trainingData)
	case "resource_prediction":
		return mla.trainResourcePredictionModel(trainingData)
	case "clustering":
		return mla.trainClusteringModel(trainingData)
	default:
		return nil, fmt.Errorf("unknown model type: %s", modelType)
	}
}

// UpdateModel updates an existing model with new data
func (mla *MachineLearningAnalyzer) UpdateModel(learningData *shared.WorkflowLearningData) error {
	mla.log.WithField("execution_id", learningData.ExecutionID).Debug("Updating ML models with new data")

	// Update success prediction model
	if successModel, exists := mla.models["success_prediction"]; exists {
		if err := mla.updateSuccessModel(successModel, learningData); err != nil {
			mla.log.WithError(err).Error("Failed to update success prediction model")
		}
	}

	// Update duration prediction model
	if durationModel, exists := mla.models["duration_prediction"]; exists {
		if err := mla.updateDurationModel(durationModel, learningData); err != nil {
			mla.log.WithError(err).Error("Failed to update duration prediction model")
		}
	}

	// Update resource prediction model
	if resourceModel, exists := mla.models["resource_prediction"]; exists {
		if err := mla.updateResourceModel(resourceModel, learningData); err != nil {
			mla.log.WithError(err).Error("Failed to update resource prediction model")
		}
	}

	return nil
}

// AnalyzeModelPerformance evaluates model performance
func (mla *MachineLearningAnalyzer) AnalyzeModelPerformance(modelID string, testData []*sharedtypes.WorkflowExecutionData) (*ModelPerformanceReport, error) {
	model, exists := mla.models[modelID]
	if !exists {
		return nil, fmt.Errorf("model %s not found", modelID)
	}

	mla.log.WithFields(logrus.Fields{
		"model_id":  modelID,
		"test_size": len(testData),
	}).Info("Analyzing model performance")

	report := &ModelPerformanceReport{
		ModelID:             modelID,
		ModelType:           model.Type,
		TestDataSize:        len(testData),
		EvaluatedAt:         time.Now(),
		Metrics:             make(map[string]float64),
		Predictions:         make([]*PredictionResult, 0),
		PerformanceAnalysis: &MLAnalysisPerformance{},
	}

	// Evaluate predictions
	correct := 0
	totalError := 0.0
	predictions := make([]*PredictionResult, 0)

	for _, data := range testData {
		features, err := mla.ExtractFeatures(data)
		if err != nil {
			continue
		}

		prediction, err := mla.makePrediction(model, features)
		if err != nil {
			continue
		}

		predResult := &PredictionResult{
			Features:  features,
			Predicted: prediction,
			Actual:    &workflowtypes.CoreWorkflowExecutionResult{Success: data.Success, Duration: data.Duration},
			Error:     mla.calculatePredictionError(prediction, &workflowtypes.CoreWorkflowExecutionResult{Success: data.Success, Duration: data.Duration}),
		}

		predictions = append(predictions, predResult)

		if mla.isPredictionCorrect(prediction, &workflowtypes.CoreWorkflowExecutionResult{Success: data.Success, Duration: data.Duration}) {
			correct++
		}

		totalError += predResult.Error
	}

	// Calculate metrics
	if len(predictions) > 0 {
		report.Metrics["accuracy"] = float64(correct) / float64(len(predictions))
		report.Metrics["mean_error"] = totalError / float64(len(predictions))
		report.Metrics["confidence"] = mla.calculateModelConfidence(predictions)
	}

	report.Predictions = predictions

	// Generate performance analysis
	report.PerformanceAnalysis = mla.generatePerformanceAnalysis(predictions)

	return report, nil
}

// CrossValidateModel performs k-fold cross-validation on a model
func (mla *MachineLearningAnalyzer) CrossValidateModel(modelType string, data []*sharedtypes.WorkflowExecutionData, folds int) (*CrossValidationMetrics, error) {
	if len(data) < folds {
		return nil, fmt.Errorf("insufficient data for %d-fold cross-validation: have %d samples", folds, len(data))
	}

	mla.log.WithFields(logrus.Fields{
		"model_type": modelType,
		"data_size":  len(data),
		"folds":      folds,
	}).Info("Performing cross-validation")

	// Shuffle data
	shuffledData := make([]*sharedtypes.WorkflowExecutionData, len(data))
	copy(shuffledData, data)

	// Simple shuffle
	for i := range shuffledData {
		j := i + int(time.Now().UnixNano())%(len(shuffledData)-i)
		shuffledData[i], shuffledData[j] = shuffledData[j], shuffledData[i]
	}

	foldSize := len(shuffledData) / folds
	accuracies := make([]float64, folds)
	f1Scores := make([]float64, folds)

	for fold := 0; fold < folds; fold++ {
		// Create train/validation split
		start := fold * foldSize
		end := start + foldSize
		if fold == folds-1 {
			end = len(shuffledData) // Include remaining samples in last fold
		}

		validationData := shuffledData[start:end]
		trainData := append(shuffledData[:start], shuffledData[end:]...)

		// Train model on training data
		model, err := mla.TrainModel(modelType, trainData)
		if err != nil {
			mla.log.WithError(err).WithField("fold", fold).Error("Failed to train model for cross-validation")
			continue
		}

		// Evaluate on validation data
		correct := 0
		truePositives := 0
		falsePositives := 0
		falseNegatives := 0

		for _, validationSample := range validationData {
			features, err := mla.ExtractFeatures(validationSample)
			if err != nil {
				continue
			}

			prediction, err := mla.makePrediction(model, features)
			if err != nil {
				continue
			}

			// Determine ground truth
			actualSuccess := validationSample.Success
			predictedSuccess := mla.interpretPrediction(prediction)

			if predictedSuccess == actualSuccess {
				correct++
			}

			// Calculate precision/recall metrics
			if actualSuccess && predictedSuccess {
				truePositives++
			} else if !actualSuccess && predictedSuccess {
				falsePositives++
			} else if actualSuccess && !predictedSuccess {
				falseNegatives++
			}
		}

		// Calculate fold metrics
		if len(validationData) > 0 {
			accuracies[fold] = float64(correct) / float64(len(validationData))
		}

		// Calculate F1 score
		precision := 0.0
		recall := 0.0
		if truePositives+falsePositives > 0 {
			precision = float64(truePositives) / float64(truePositives+falsePositives)
		}
		if truePositives+falseNegatives > 0 {
			recall = float64(truePositives) / float64(truePositives+falseNegatives)
		}
		if precision+recall > 0 {
			f1Scores[fold] = 2 * (precision * recall) / (precision + recall)
		}
	}

	// Calculate statistics
	meanAccuracy := sharedmath.Mean(accuracies)
	stdAccuracy := sharedmath.StandardDeviation(accuracies)
	meanF1 := sharedmath.Mean(f1Scores)
	stdF1 := sharedmath.StandardDeviation(f1Scores)

	metrics := &CrossValidationMetrics{
		Folds:        folds,
		MeanAccuracy: meanAccuracy,
		StdAccuracy:  stdAccuracy,
		MeanF1:       meanF1,
		StdF1:        stdF1,
	}

	mla.log.WithFields(logrus.Fields{
		"mean_accuracy": meanAccuracy,
		"std_accuracy":  stdAccuracy,
		"mean_f1":       meanF1,
	}).Info("Cross-validation completed")

	return metrics, nil
}

// Private methods for model training and prediction

func (mla *MachineLearningAnalyzer) initializeDefaultModels() {
	// Initialize with simple baseline models
	mla.models["success_prediction"] = &MLModel{
		ID:        "success_baseline",
		Type:      "classification",
		Version:   1,
		TrainedAt: time.Now(),
		Accuracy:  0.7, // Baseline accuracy
		Features:  []string{"alert_count", "severity_score", "step_count"},
	}

	mla.models["duration_prediction"] = &MLModel{
		ID:        "duration_baseline",
		Type:      "regression",
		Version:   1,
		TrainedAt: time.Now(),
		Accuracy:  0.6,
		Features:  []string{"step_count", "dependency_depth", "resource_count"},
	}

	mla.models["resource_prediction"] = &MLModel{
		ID:        "resource_baseline",
		Type:      "regression",
		Version:   1,
		TrainedAt: time.Now(),
		Accuracy:  0.5,
		Features:  []string{"resource_count", "cluster_load", "resource_pressure"},
	}
}

func (mla *MachineLearningAnalyzer) trainSuccessPredictionModel(trainingData []*sharedtypes.WorkflowExecutionData) (*MLModel, error) {
	// Extract features and labels
	features := make([][]float64, 0)
	labels := make([]float64, 0)

	for _, data := range trainingData {
		featureVec, err := mla.featureExtractor.ExtractVector(data)
		if err != nil {
			continue
		}

		features = append(features, featureVec)
		if data.Success {
			labels = append(labels, 1.0)
		} else {
			labels = append(labels, 0.0)
		}
	}

	if len(features) < 10 {
		return nil, fmt.Errorf("insufficient training data: %d samples", len(features))
	}

	// Train logistic regression model
	weights, bias, metrics := mla.trainLogisticRegression(features, labels)

	model := &MLModel{
		ID:              "success_prediction_v2",
		Type:            "classification",
		Version:         2,
		TrainedAt:       time.Now(),
		Accuracy:        metrics.Accuracy,
		Features:        mla.featureExtractor.GetFeatureNames(),
		Weights:         weights,
		Bias:            bias,
		TrainingMetrics: metrics,
	}

	mla.models["success_prediction"] = model
	return model, nil
}

func (mla *MachineLearningAnalyzer) trainDurationPredictionModel(trainingData []*sharedtypes.WorkflowExecutionData) (*MLModel, error) {
	// Extract features and duration labels
	features := make([][]float64, 0)
	durations := make([]float64, 0)

	for _, data := range trainingData {
		featureVec, err := mla.featureExtractor.ExtractVector(data)
		if err != nil {
			continue
		}

		features = append(features, featureVec)
		durations = append(durations, data.Duration.Seconds())
	}

	if len(features) < 10 {
		return nil, fmt.Errorf("insufficient training data: %d samples", len(features))
	}

	// Train linear regression model
	weights, bias, metrics := mla.trainLinearRegression(features, durations)

	model := &MLModel{
		ID:              "duration_prediction_v2",
		Type:            "regression",
		Version:         2,
		TrainedAt:       time.Now(),
		Accuracy:        metrics.R2Score,
		Features:        mla.featureExtractor.GetFeatureNames(),
		Weights:         weights,
		Bias:            bias,
		TrainingMetrics: metrics,
	}

	mla.models["duration_prediction"] = model
	return model, nil
}

func (mla *MachineLearningAnalyzer) trainResourcePredictionModel(trainingData []*sharedtypes.WorkflowExecutionData) (*MLModel, error) {
	// Multi-output regression for CPU, memory, network, storage
	features := make([][]float64, 0)
	resourceUsage := make([][]float64, 0) // [cpu, memory, network, storage]

	for _, data := range trainingData {
		featureVec, err := mla.featureExtractor.ExtractVector(data)
		if err != nil {
			continue
		}

		// Extract resource usage from metrics if available
		hasResourceData := false
		cpuUsage := 0.0
		memoryUsage := 0.0
		networkUsage := 0.0
		storageUsage := 0.0

		if data.Metrics != nil {
			if cpu, exists := data.Metrics["cpu_usage"]; exists {
				cpuUsage = cpu
				hasResourceData = true
			}
			if memory, exists := data.Metrics["memory_usage"]; exists {
				memoryUsage = memory
				hasResourceData = true
			}
			if network, exists := data.Metrics["network_usage"]; exists {
				networkUsage = network
				hasResourceData = true
			}
			if storage, exists := data.Metrics["storage_usage"]; exists {
				storageUsage = storage
				hasResourceData = true
			}
		}

		if hasResourceData {
			features = append(features, featureVec)
			resourceUsage = append(resourceUsage, []float64{
				cpuUsage,
				memoryUsage,
				networkUsage,
				storageUsage,
			})
		}
	}

	if len(features) < 10 {
		return nil, fmt.Errorf("insufficient training data: %d samples", len(features))
	}

	// Train multi-output regression
	weights, bias, metrics := mla.trainMultiOutputRegression(features, resourceUsage)

	// Flatten multi-output weights for storage
	flatWeights := make([]float64, 0)
	for _, weight := range weights {
		flatWeights = append(flatWeights, weight...)
	}

	// Use first output bias as representative
	representativeBias := 0.0
	if len(bias) > 0 {
		representativeBias = bias[0]
	}

	model := &MLModel{
		ID:              "resource_prediction_v2",
		Type:            "regression",
		Version:         2,
		TrainedAt:       time.Now(),
		Accuracy:        metrics.R2Score,
		Features:        []string{"cpu", "memory", "network", "storage"}, // Simplified
		Weights:         flatWeights,
		Bias:            representativeBias,
		TrainingMetrics: metrics,
	}

	mla.models["resource_prediction"] = model
	return model, nil
}

func (mla *MachineLearningAnalyzer) trainClusteringModel(trainingData []*sharedtypes.WorkflowExecutionData) (*MLModel, error) {
	// K-means clustering for pattern discovery
	features := make([][]float64, 0)

	for _, data := range trainingData {
		featureVec, err := mla.featureExtractor.ExtractVector(data)
		if err != nil {
			continue
		}
		features = append(features, featureVec)
	}

	if len(features) < 10 { // Use fixed minimum instead of config field
		return nil, fmt.Errorf("insufficient data for clustering: %d samples", len(features))
	}

	// Perform K-means clustering
	clusterAssignments, centroids, metrics := mla.performKMeansClustering(features)

	model := &MLModel{
		ID:        "clustering_v2",
		Type:      "clustering",
		Version:   2,
		TrainedAt: time.Now(),
		Features:  mla.featureExtractor.GetFeatureNames(),
		Parameters: map[string]interface{}{
			"centroids":           centroids,
			"cluster_assignments": clusterAssignments,
			"num_clusters":        len(centroids),
		},
		TrainingMetrics: metrics,
	}

	mla.models["clustering"] = model
	return model, nil
}

// Simple implementations of ML algorithms

func (mla *MachineLearningAnalyzer) trainLogisticRegression(features [][]float64, labels []float64) ([]float64, float64, *TrainingMetrics) {
	// Simplified logistic regression using gradient descent
	if len(features) == 0 || len(features[0]) == 0 {
		return nil, 0, nil
	}

	numFeatures := len(features[0])
	weights := make([]float64, numFeatures)
	bias := 0.0
	learningRate := 0.01
	epochs := 1000

	// Initialize weights randomly
	for i := range weights {
		weights[i] = (math.Abs(math.Sin(float64(i))) - 0.5) * 0.1
	}

	// Gradient descent
	for epoch := 0; epoch < epochs; epoch++ {
		// Calculate gradients
		weightGrad := make([]float64, numFeatures)
		biasGrad := 0.0

		for i, feature := range features {
			// Forward pass
			z := bias
			for j, x := range feature {
				z += weights[j] * x
			}
			prediction := 1.0 / (1.0 + math.Exp(-z)) // Sigmoid

			// Calculate error
			error := prediction - labels[i]

			// Accumulate gradients
			biasGrad += error
			for j, x := range feature {
				weightGrad[j] += error * x
			}
		}

		// Update weights
		bias -= learningRate * biasGrad / float64(len(features))
		for j := range weights {
			weights[j] -= learningRate * weightGrad[j] / float64(len(features))
		}
	}

	// Calculate accuracy
	correct := 0
	for i, feature := range features {
		z := bias
		for j, x := range feature {
			z += weights[j] * x
		}
		prediction := 1.0 / (1.0 + math.Exp(-z))
		predicted := 0.0
		if prediction > 0.5 {
			predicted = 1.0
		}
		if predicted == labels[i] {
			correct++
		}
	}

	accuracy := float64(correct) / float64(len(features))

	metrics := &TrainingMetrics{
		TrainingSize: len(features),
		Accuracy:     accuracy,
		Precision:    accuracy, // Simplified
		Recall:       accuracy, // Simplified
		F1Score:      accuracy, // Simplified
	}

	return weights, bias, metrics
}

func (mla *MachineLearningAnalyzer) trainLinearRegression(features [][]float64, targets []float64) ([]float64, float64, *TrainingMetrics) {
	// Simplified linear regression using normal equation
	if len(features) == 0 || len(features[0]) == 0 {
		return nil, 0, nil
	}

	numFeatures := len(features[0])
	numSamples := len(features)

	// Create design matrix X (with bias column)
	X := mat.NewDense(numSamples, numFeatures+1, nil)
	y := mat.NewVecDense(numSamples, targets)

	for i, feature := range features {
		X.Set(i, 0, 1.0) // Bias term
		for j, val := range feature {
			X.Set(i, j+1, val)
		}
	}

	// Normal equation: theta = (X^T * X)^(-1) * X^T * y
	var XTX mat.Dense
	XTX.Mul(X.T(), X)

	var XTXInv mat.Dense
	err := XTXInv.Inverse(&XTX)
	if err != nil {
		// Fallback to simple average if matrix is singular
		weights := make([]float64, numFeatures)
		bias := stat.Mean(targets, nil)
		return weights, bias, &TrainingMetrics{TrainingSize: numSamples, R2Score: 0.0}
	}

	var XTy mat.VecDense
	XTy.MulVec(X.T(), y)

	var theta mat.VecDense
	theta.MulVec(&XTXInv, &XTy)

	// Extract weights and bias
	bias := theta.AtVec(0)
	weights := make([]float64, numFeatures)
	for i := 0; i < numFeatures; i++ {
		weights[i] = theta.AtVec(i + 1)
	}

	// Calculate R² score
	var predictions mat.VecDense
	predictions.MulVec(X, &theta)

	yMean := stat.Mean(targets, nil)
	var ss_res, ss_tot float64
	for i := 0; i < numSamples; i++ {
		predicted := predictions.AtVec(i)
		actual := targets[i]
		diff1 := actual - predicted
		ss_res += diff1 * diff1
		diff2 := actual - yMean
		ss_tot += diff2 * diff2
	}

	r2Score := 1.0 - (ss_res / ss_tot)
	if math.IsNaN(r2Score) || math.IsInf(r2Score, 0) {
		r2Score = 0.0
	}

	metrics := &TrainingMetrics{
		TrainingSize: numSamples,
		R2Score:      r2Score,
		RMSE:         math.Sqrt(ss_res / float64(numSamples)),
		MAE:          math.Sqrt(ss_res) / float64(numSamples), // Approximation
	}

	return weights, bias, metrics
}

func (mla *MachineLearningAnalyzer) trainMultiOutputRegression(features [][]float64, targets [][]float64) ([][]float64, []float64, *TrainingMetrics) {
	// Train separate linear regression for each output
	numOutputs := len(targets[0])
	allWeights := make([][]float64, numOutputs)
	allBias := make([]float64, numOutputs)
	totalR2 := 0.0

	for output := 0; output < numOutputs; output++ {
		// Extract targets for this output
		outputTargets := make([]float64, len(targets))
		for i, target := range targets {
			outputTargets[i] = target[output]
		}

		weights, bias, metrics := mla.trainLinearRegression(features, outputTargets)
		allWeights[output] = weights
		allBias[output] = bias
		totalR2 += metrics.R2Score
	}

	avgR2 := totalR2 / float64(numOutputs)

	metrics := &TrainingMetrics{
		TrainingSize: len(features),
		R2Score:      avgR2,
	}

	return allWeights, allBias, metrics
}

func (mla *MachineLearningAnalyzer) performKMeansClustering(features [][]float64) ([]int, [][]float64, *TrainingMetrics) {
	// Simplified K-means implementation
	k := int(math.Min(float64(len(features)/5), 10)) // Max 10 clusters, min cluster size = 5
	if k < 2 {
		k = 2
	}

	numFeatures := len(features[0])
	centroids := make([][]float64, k)

	// Initialize centroids randomly
	for i := 0; i < k; i++ {
		centroids[i] = make([]float64, numFeatures)
		for j := 0; j < numFeatures; j++ {
			centroids[i][j] = features[i%len(features)][j] // Simple initialization
		}
	}

	assignments := make([]int, len(features))
	maxIterations := 100

	for iter := 0; iter < maxIterations; iter++ {
		// Assign points to nearest centroid
		changed := false
		for i, feature := range features {
			minDist := math.Inf(1)
			bestCluster := 0

			for j, centroid := range centroids {
				dist := mla.euclideanDistance(feature, centroid)
				if dist < minDist {
					minDist = dist
					bestCluster = j
				}
			}

			if assignments[i] != bestCluster {
				changed = true
				assignments[i] = bestCluster
			}
		}

		if !changed {
			break
		}

		// Update centroids
		clusterCounts := make([]int, k)
		newCentroids := make([][]float64, k)
		for i := 0; i < k; i++ {
			newCentroids[i] = make([]float64, numFeatures)
		}

		for i, feature := range features {
			cluster := assignments[i]
			clusterCounts[cluster]++
			for j, val := range feature {
				newCentroids[cluster][j] += val
			}
		}

		for i := 0; i < k; i++ {
			if clusterCounts[i] > 0 {
				for j := 0; j < numFeatures; j++ {
					newCentroids[i][j] /= float64(clusterCounts[i])
				}
				centroids[i] = newCentroids[i]
			}
		}
	}

	metrics := &TrainingMetrics{
		TrainingSize: len(features),
		Accuracy:     0.8, // Placeholder clustering quality metric
	}

	return assignments, centroids, metrics
}

// Helper methods

func (mla *MachineLearningAnalyzer) euclideanDistance(a, b []float64) float64 {
	sum := 0.0
	for i := 0; i < len(a); i++ {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return math.Sqrt(sum)
}

func (mla *MachineLearningAnalyzer) updateSuccessModel(model *MLModel, learningData *shared.WorkflowLearningData) error {
	// Simplified online learning update
	_, err := mla.featureExtractor.ExtractFromLearningData([]*shared.WorkflowLearningData{learningData})
	if err != nil {
		return err
	}

	// Update model with new observation (simplified)
	// In practice, this would use proper online learning algorithms
	model.TrainingMetrics.TrainingSize++
	return nil
}

func (mla *MachineLearningAnalyzer) updateDurationModel(model *MLModel, learningData *shared.WorkflowLearningData) error {
	return mla.updateSuccessModel(model, learningData) // Simplified
}

func (mla *MachineLearningAnalyzer) updateResourceModel(model *MLModel, learningData *shared.WorkflowLearningData) error {
	return mla.updateSuccessModel(model, learningData) // Simplified
}

func (mla *MachineLearningAnalyzer) makePrediction(model *MLModel, features *shared.WorkflowFeatures) (interface{}, error) {
	featureVec, err := mla.featureExtractor.ConvertToVector(features)
	if err != nil {
		return nil, err
	}

	switch model.Type {
	case "classification":
		return mla.predictClassification(model, featureVec), nil
	case "regression":
		return mla.predictRegression(model, featureVec), nil
	default:
		return nil, fmt.Errorf("unsupported model type: %s", model.Type)
	}
}

func (mla *MachineLearningAnalyzer) predictClassification(model *MLModel, features []float64) bool {
	if len(model.Weights) == 0 {
		return true // Default to success
	}

	z := model.Bias
	for i, weight := range model.Weights {
		if i < len(features) {
			z += weight * features[i]
		}
	}

	probability := 1.0 / (1.0 + math.Exp(-z))
	return probability > 0.5
}

func (mla *MachineLearningAnalyzer) interpretPrediction(prediction interface{}) bool {
	switch p := prediction.(type) {
	case bool:
		return p
	case float64:
		return p > 0.5
	case int:
		return p > 0
	default:
		return false // Default to failure if can't interpret
	}
}

func (mla *MachineLearningAnalyzer) predictRegression(model *MLModel, features []float64) float64 {
	if len(model.Weights) == 0 {
		return 0.0
	}

	result := model.Bias
	for i, weight := range model.Weights {
		if i < len(features) {
			result += weight * features[i]
		}
	}

	return result
}

func (mla *MachineLearningAnalyzer) calculatePredictionError(prediction interface{}, actual *workflowtypes.CoreWorkflowExecutionResult) float64 {
	switch pred := prediction.(type) {
	case bool:
		if (pred && actual.Success) || (!pred && !actual.Success) {
			return 0.0
		}
		return 1.0
	case float64:
		// Assuming this is duration prediction in seconds
		actualSeconds := actual.Duration.Seconds()
		return math.Abs(pred - actualSeconds)
	default:
		return 1.0 // Unknown type, assume error
	}
}

func (mla *MachineLearningAnalyzer) isPredictionCorrect(prediction interface{}, actual *workflowtypes.CoreWorkflowExecutionResult) bool {
	switch pred := prediction.(type) {
	case bool:
		return (pred && actual.Success) || (!pred && !actual.Success)
	case float64:
		// For regression, consider within 10% as correct
		actualSeconds := actual.Duration.Seconds()
		if actualSeconds == 0 {
			return pred == 0
		}
		relativeError := math.Abs(pred-actualSeconds) / actualSeconds
		return relativeError < 0.1
	default:
		return false
	}
}

func (mla *MachineLearningAnalyzer) calculateModelConfidence(predictions []*PredictionResult) float64 {
	if len(predictions) == 0 {
		return 0.0
	}

	totalError := 0.0
	for _, pred := range predictions {
		totalError += pred.Error
	}

	avgError := totalError / float64(len(predictions))
	// Convert error to confidence (lower error = higher confidence)
	confidence := 1.0 / (1.0 + avgError)
	return confidence
}

func (mla *MachineLearningAnalyzer) generatePerformanceAnalysis(predictions []*PredictionResult) *MLAnalysisPerformance {
	if len(predictions) == 0 {
		return &MLAnalysisPerformance{}
	}

	errors := make([]float64, len(predictions))
	for i, pred := range predictions {
		errors[i] = pred.Error
	}

	return &MLAnalysisPerformance{
		ErrorDistribution: &ErrorDistribution{
			Mean:   stat.Mean(errors, nil),
			StdDev: stat.StdDev(errors, nil),
			Min:    minFloat64(errors),
			Max:    maxFloat64(errors),
		},
		OutlierCount:  mla.countOutliers(errors),
		TrendAnalysis: "Stable", // Simplified
	}
}

func (mla *MachineLearningAnalyzer) countOutliers(errors []float64) int {
	if len(errors) < 4 {
		return 0
	}

	// Sort errors to find quartiles
	sortedErrors := make([]float64, len(errors))
	copy(sortedErrors, errors)
	sort.Float64s(sortedErrors)

	q1 := sortedErrors[len(sortedErrors)/4]
	q3 := sortedErrors[3*len(sortedErrors)/4]
	iqr := q3 - q1

	lowerBound := q1 - 1.5*iqr
	upperBound := q3 + 1.5*iqr

	outliers := 0
	for _, err := range errors {
		if err < lowerBound || err > upperBound {
			outliers++
		}
	}

	return outliers
}

// Supporting types

type PredictionResult struct {
	Features  *shared.WorkflowFeatures                   `json:"features"`
	Predicted interface{}                                `json:"predicted"`
	Actual    *workflowtypes.CoreWorkflowExecutionResult `json:"actual"`
	Error     float64                                    `json:"error"`
}

type MLAnalysisPerformance struct {
	ErrorDistribution *ErrorDistribution `json:"error_distribution"`
	OutlierCount      int                `json:"outlier_count"`
	TrendAnalysis     string             `json:"trend_analysis"`
}

type ErrorDistribution struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type ModelPerformanceReport struct {
	ModelID             string                 `json:"model_id"`
	ModelType           string                 `json:"model_type"`
	TestDataSize        int                    `json:"test_data_size"`
	EvaluatedAt         time.Time              `json:"evaluated_at"`
	Metrics             map[string]float64     `json:"metrics"`
	Predictions         []*PredictionResult    `json:"predictions"`
	PerformanceAnalysis *MLAnalysisPerformance `json:"performance_analysis"`
}

// Constructor functions for ML analyzer types

func NewMLAnalyzerFeatureExtractor(config *MLConfig, log *logrus.Logger) *MLAnalyzerFeatureExtractor {
	return &MLAnalyzerFeatureExtractor{
		config: config,
		log:    log,
	}
}

func NewMLAnalyzerOutcomePredictor(log *logrus.Logger) *MLAnalyzerOutcomePredictor {
	return &MLAnalyzerOutcomePredictor{
		log: log,
	}
}

func NewMLAnalyzerClusterer(config *MLConfig, log *logrus.Logger) *MLAnalyzerClusterer {
	return &MLAnalyzerClusterer{
		config: config,
		log:    log,
	}
}

// Methods for ML analyzer components
func (fe *MLAnalyzerFeatureExtractor) Extract(data *sharedtypes.WorkflowExecutionData) (*shared.WorkflowFeatures, error) {
	if data == nil {
		return nil, fmt.Errorf("workflow execution data is nil")
	}

	features := &shared.WorkflowFeatures{
		AlertCount:      int(fe.extractMetricValue(data.Metrics, "alert_count")),
		SeverityScore:   fe.extractMetricValue(data.Metrics, "severity_score"),
		ResourceCount:   int(fe.extractMetricValue(data.Metrics, "resource_count")),
		StepCount:       int(fe.extractMetricValue(data.Metrics, "step_count")),
		DependencyDepth: int(fe.extractMetricValue(data.Metrics, "dependency_depth")),
	}

	// Extract temporal features from timestamp
	features.HourOfDay = data.Timestamp.Hour()
	features.DayOfWeek = int(data.Timestamp.Weekday())
	features.IsWeekend = features.DayOfWeek == 0 || features.DayOfWeek == 6
	features.IsBusinessHour = features.HourOfDay >= 9 && features.HourOfDay <= 17 && !features.IsWeekend

	// Extract resource pressure features from metrics
	if data.Metrics != nil {
		features.ClusterLoad = fe.extractMetricValue(data.Metrics, "cluster_load")
		features.ResourcePressure = fe.extractMetricValue(data.Metrics, "resource_pressure")

		// Initialize custom metrics map
		features.CustomMetrics = make(map[string]float64)
		for key, value := range data.Metrics {
			features.CustomMetrics[key] = value
		}
	}

	// Calculate derived features
	if data.Success {
		features.RecentFailures = 0
		features.AverageSuccessRate = 1.0
	} else {
		features.RecentFailures = 1
		features.AverageSuccessRate = 0.0
	}
	features.LastExecutionTime = data.Duration

	return features, nil
}

func (fe *MLAnalyzerFeatureExtractor) ExtractVector(data *sharedtypes.WorkflowExecutionData) ([]float64, error) {
	features, err := fe.Extract(data)
	if err != nil {
		return nil, err
	}
	return fe.ConvertToVector(features)
}

func (fe *MLAnalyzerFeatureExtractor) GetFeatureNames() []string {
	return []string{
		"alert_count",
		"severity_score",
		"resource_count",
		"step_count",
		"dependency_depth",
		"hour_of_day",
		"day_of_week",
		"is_weekend",
		"is_business_hour",
		"recent_failures",
		"average_success_rate",
		"last_execution_time_seconds",
		"cluster_load",
		"resource_pressure",
	}
}

func (fe *MLAnalyzerFeatureExtractor) ExtractFromLearningData(data []*shared.WorkflowLearningData) ([][]float64, error) {
	vectors := make([][]float64, 0, len(data))

	for _, learningData := range data {
		if learningData == nil || learningData.Features == nil {
			continue
		}

		// Convert features to vector
		vector, err := fe.ConvertToVector(learningData.Features)
		if err != nil {
			continue // Skip invalid data
		}

		vectors = append(vectors, vector)
	}

	return vectors, nil
}

func (fe *MLAnalyzerFeatureExtractor) ConvertToVector(features *shared.WorkflowFeatures) ([]float64, error) {
	if features == nil {
		return nil, fmt.Errorf("features are nil")
	}

	// Convert boolean values to float64
	isWeekend := 0.0
	if features.IsWeekend {
		isWeekend = 1.0
	}

	isBusinessHour := 0.0
	if features.IsBusinessHour {
		isBusinessHour = 1.0
	}

	return []float64{
		float64(features.AlertCount),
		features.SeverityScore,
		float64(features.ResourceCount),
		float64(features.StepCount),
		float64(features.DependencyDepth),
		float64(features.HourOfDay),
		float64(features.DayOfWeek),
		isWeekend,
		isBusinessHour,
		float64(features.RecentFailures),
		features.AverageSuccessRate,
		features.LastExecutionTime.Seconds(),
		features.ClusterLoad,
		features.ResourcePressure,
	}, nil
}

// Helper methods for feature extraction
func (fe *MLAnalyzerFeatureExtractor) extractMetricValue(metrics map[string]float64, key string) float64 {
	if value, exists := metrics[key]; exists {
		return value
	}
	return 0.0
}

func (op *MLAnalyzerOutcomePredictor) Predict(features *shared.WorkflowFeatures, patterns []*shared.DiscoveredPattern, models map[string]*MLModel) (*shared.WorkflowPrediction, error) {
	if features == nil {
		return nil, fmt.Errorf("features cannot be nil")
	}

	prediction := &shared.WorkflowPrediction{
		Confidence: 0.0,
	}

	// Convert features to vector for model prediction
	featureVector, err := op.convertFeaturesToVector(features)
	if err != nil {
		op.log.WithError(err).Error("Failed to convert features to vector")
		return prediction, err
	}

	// Predict success probability
	if successModel, exists := models["success_prediction"]; exists && successModel != nil {
		successPrediction := op.predictWithModel(successModel, featureVector)
		if successProb, ok := successPrediction.(float64); ok {
			prediction.SuccessProbability = successProb
		} else if success, ok := successPrediction.(bool); ok {
			if success {
				prediction.SuccessProbability = 0.8
			} else {
				prediction.SuccessProbability = 0.2
			}
		}
	} else {
		// Fallback prediction based on patterns and features
		prediction.SuccessProbability = op.calculateFallbackSuccessProbability(features, patterns)
	}

	// Predict duration
	if durationModel, exists := models["duration_prediction"]; exists && durationModel != nil {
		durationPrediction := op.predictWithModel(durationModel, featureVector)
		if duration, ok := durationPrediction.(float64); ok {
			prediction.ExpectedDuration = time.Duration(duration * float64(time.Second))
		}
	} else {
		// Fallback duration prediction
		prediction.ExpectedDuration = op.calculateFallbackDuration(features, patterns)
	}

	// Predict resource usage
	if resourceModel, exists := models["resource_prediction"]; exists && resourceModel != nil {
		resourcePrediction := op.predictWithModel(resourceModel, featureVector)
		if resources, ok := resourcePrediction.([]float64); ok && len(resources) >= 4 {
			prediction.ResourceRequirements = &shared.ResourcePrediction{
				CPUUsage:     resources[0],
				MemoryUsage:  resources[1],
				NetworkUsage: resources[2],
				StorageUsage: resources[3],
				Confidence:   0.7, // Base confidence for resource prediction
			}
		}
	}

	// Calculate overall confidence based on available models and pattern matches
	confidence := 0.0
	modelCount := 0

	if _, exists := models["success_prediction"]; exists {
		confidence += prediction.SuccessProbability
		modelCount++
	}

	if _, exists := models["duration_prediction"]; exists {
		confidence += 0.7 // Base confidence for duration model
		modelCount++
	}

	// Add pattern-based confidence boost
	patternConfidence := op.calculatePatternConfidence(features, patterns)
	confidence += patternConfidence
	modelCount++

	if modelCount > 0 {
		prediction.Confidence = confidence / float64(modelCount)
	}

	// Add similar patterns count
	prediction.SimilarPatterns = len(patterns)

	// Add reasoning
	prediction.Reason = op.generatePredictionReason(features, patterns, prediction)

	return prediction, nil
}

// Helper methods for MLAnalyzerOutcomePredictor
func (op *MLAnalyzerOutcomePredictor) convertFeaturesToVector(features *shared.WorkflowFeatures) ([]float64, error) {
	if features == nil {
		return nil, fmt.Errorf("features are nil")
	}

	// Convert boolean values to float64
	isWeekend := 0.0
	if features.IsWeekend {
		isWeekend = 1.0
	}

	isBusinessHour := 0.0
	if features.IsBusinessHour {
		isBusinessHour = 1.0
	}

	return []float64{
		float64(features.AlertCount),
		features.SeverityScore,
		float64(features.ResourceCount),
		float64(features.StepCount),
		float64(features.DependencyDepth),
		float64(features.HourOfDay),
		float64(features.DayOfWeek),
		isWeekend,
		isBusinessHour,
		float64(features.RecentFailures),
		features.AverageSuccessRate,
		features.LastExecutionTime.Seconds(),
		features.ClusterLoad,
		features.ResourcePressure,
	}, nil
}

func (op *MLAnalyzerOutcomePredictor) predictWithModel(model *MLModel, featureVector []float64) interface{} {
	switch model.Type {
	case "classification":
		return op.predictClassificationWithModel(model, featureVector)
	case "regression":
		return op.predictRegressionWithModel(model, featureVector)
	case "clustering":
		return op.predictClusterWithModel(model, featureVector)
	default:
		op.log.WithField("model_type", model.Type).Warn("Unknown model type, using default prediction")
		return 0.5 // Default neutral prediction
	}
}

func (op *MLAnalyzerOutcomePredictor) predictClassificationWithModel(model *MLModel, features []float64) float64 {
	if len(model.Weights) == 0 {
		return 0.5 // Default neutral probability
	}

	z := model.Bias
	for i, weight := range model.Weights {
		if i < len(features) {
			z += weight * features[i]
		}
	}

	// Sigmoid activation
	probability := 1.0 / (1.0 + math.Exp(-z))

	// Ensure probability is within valid range
	if probability < 0.0 {
		probability = 0.0
	} else if probability > 1.0 {
		probability = 1.0
	}

	return probability
}

func (op *MLAnalyzerOutcomePredictor) predictRegressionWithModel(model *MLModel, features []float64) float64 {
	if len(model.Weights) == 0 {
		return 0.0
	}

	result := model.Bias
	for i, weight := range model.Weights {
		if i < len(features) {
			result += weight * features[i]
		}
	}

	return result
}

func (op *MLAnalyzerOutcomePredictor) predictClusterWithModel(model *MLModel, features []float64) int {
	// Find closest centroid
	centroids, ok := model.Parameters["centroids"].([][]float64)
	if !ok {
		return 0
	}

	bestCluster := 0
	minDistance := math.Inf(1)

	for i, centroid := range centroids {
		distance := op.calculateEuclideanDistance(features, centroid)
		if distance < minDistance {
			minDistance = distance
			bestCluster = i
		}
	}

	return bestCluster
}

func (op *MLAnalyzerOutcomePredictor) calculateEuclideanDistance(a, b []float64) float64 {
	sum := 0.0
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	for i := 0; i < minLen; i++ {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return math.Sqrt(sum)
}

func (op *MLAnalyzerOutcomePredictor) calculateFallbackSuccessProbability(features *shared.WorkflowFeatures, patterns []*shared.DiscoveredPattern) float64 {
	// Base probability starts neutral
	probability := 0.5

	// Adjust based on features
	if features.SeverityScore > 3.0 {
		probability -= 0.2 // High severity reduces success probability
	} else if features.SeverityScore < 1.0 {
		probability += 0.1 // Low severity increases success probability
	}

	// Adjust based on resource pressure
	if features.ResourcePressure > 0.8 {
		probability -= 0.1
	} else if features.ResourcePressure < 0.3 {
		probability += 0.05
	}

	// Adjust based on historical success rate
	if features.AverageSuccessRate > 0 {
		probability = (probability + features.AverageSuccessRate) / 2
	}

	// Consider matching patterns
	for _, pattern := range patterns {
		if pattern.SuccessRate > 0.8 {
			probability += 0.1
		} else if pattern.SuccessRate < 0.3 {
			probability -= 0.15
		}
	}

	// Ensure probability stays in valid range
	if probability < 0.0 {
		probability = 0.0
	} else if probability > 1.0 {
		probability = 1.0
	}

	return probability
}

func (op *MLAnalyzerOutcomePredictor) calculateFallbackDuration(features *shared.WorkflowFeatures, patterns []*shared.DiscoveredPattern) time.Duration {
	// Base duration estimate
	baseDuration := time.Duration(features.StepCount) * time.Minute

	// Use historical execution time if available
	if features.LastExecutionTime > 0 {
		baseDuration = (baseDuration + features.LastExecutionTime) / 2
	}

	// Adjust based on resource pressure
	if features.ResourcePressure > 0.7 {
		baseDuration = time.Duration(float64(baseDuration) * 1.5) // 50% longer under pressure
	}

	// Consider pattern-based adjustments
	avgPatternDuration := time.Duration(0)
	patternCount := 0

	for _, pattern := range patterns {
		if pattern.AverageExecutionTime > 0 {
			avgPatternDuration += pattern.AverageExecutionTime
			patternCount++
		}
	}

	if patternCount > 0 {
		avgPatternDuration = avgPatternDuration / time.Duration(patternCount)
		// Blend base estimate with pattern average
		baseDuration = (baseDuration + avgPatternDuration) / 2
	}

	return baseDuration
}

func (op *MLAnalyzerOutcomePredictor) calculatePatternConfidence(features *shared.WorkflowFeatures, patterns []*shared.DiscoveredPattern) float64 {
	if len(patterns) == 0 {
		return 0.3 // Low confidence without patterns
	}

	totalConfidence := 0.0
	weightSum := 0.0

	for _, pattern := range patterns {
		// Calculate pattern relevance based on feature similarity
		relevance := op.calculatePatternRelevance(features, pattern)

		// Use pattern confidence directly
		patternConfidence := pattern.Confidence

		totalConfidence += relevance * patternConfidence
		weightSum += relevance
	}

	if weightSum == 0 {
		return 0.3
	}

	confidence := totalConfidence / weightSum

	// Ensure confidence is in valid range
	if confidence < 0.0 {
		confidence = 0.0
	} else if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

func (op *MLAnalyzerOutcomePredictor) calculatePatternRelevance(features *shared.WorkflowFeatures, pattern *shared.DiscoveredPattern) float64 {
	relevance := 0.0

	// Base relevance from pattern confidence
	relevance += pattern.Confidence * 0.5

	// Consider alert count similarity
	if features.AlertCount > 0 && len(pattern.AlertPatterns) > 0 {
		relevance += 0.3
	}

	// Consider severity similarity (if pattern has severity metrics)
	if severityMetric, exists := pattern.Metrics["average_severity"]; exists {
		if math.Abs(features.SeverityScore-severityMetric) < 1.0 {
			relevance += 0.2
		}
	}

	return math.Min(relevance, 1.0)
}

func (op *MLAnalyzerOutcomePredictor) generatePredictionReason(features *shared.WorkflowFeatures, patterns []*shared.DiscoveredPattern, prediction *shared.WorkflowPrediction) string {
	reasons := []string{}

	if prediction.SuccessProbability > 0.7 {
		reasons = append(reasons, "High success probability based on favorable conditions")
	} else if prediction.SuccessProbability < 0.4 {
		reasons = append(reasons, "Low success probability due to risk factors")
	}

	if features.SeverityScore > 3.0 {
		reasons = append(reasons, "High alert severity may impact success")
	}

	if features.ResourcePressure > 0.8 {
		reasons = append(reasons, "High resource pressure detected")
	}

	if len(patterns) > 0 {
		reasons = append(reasons, fmt.Sprintf("Based on %d similar historical patterns", len(patterns)))
	}

	if len(reasons) == 0 {
		return "Prediction based on available features and models"
	}

	// Join first few reasons to keep it concise
	maxReasons := 3
	if len(reasons) > maxReasons {
		reasons = reasons[:maxReasons]
	}

	result := strings.Join(reasons, "; ")
	return result
}

// Helper functions for statistical operations
func minFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func maxFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}
