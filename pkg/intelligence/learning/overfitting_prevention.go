package learning

import (
	"math"
	"sort"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// OverfittingPrevention provides mechanisms to detect and prevent model overfitting
type OverfittingPrevention struct {
	log    *logrus.Logger
	config *patterns.PatternDiscoveryConfig
}

// OverfittingAssessment contains the result of overfitting analysis
type OverfittingAssessment struct {
	OverfittingRisk      OverfittingRisk         `json:"overfitting_risk"`
	RiskScore            float64                 `json:"risk_score"`
	Indicators           []*OverfittingIndicator `json:"indicators"`
	Recommendations      []string                `json:"recommendations"`
	ValidationMetrics    *ValidationMetrics      `json:"validation_metrics"`
	PreventionStrategies []string                `json:"prevention_strategies"`
	IsModelReliable      bool                    `json:"is_model_reliable"`
}

// OverfittingRisk levels
type OverfittingRisk string

const (
	OverfittingRiskLow      OverfittingRisk = "low"
	OverfittingRiskModerate OverfittingRisk = "moderate"
	OverfittingRiskHigh     OverfittingRisk = "high"
	OverfittingRiskCritical OverfittingRisk = "critical"
)

// OverfittingIndicator represents a specific indicator of overfitting
type OverfittingIndicator struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
	Severity    string  `json:"severity"`
	Detected    bool    `json:"detected"`
	Impact      string  `json:"impact"`
}

// ValidationMetrics contains validation-specific metrics
type ValidationMetrics struct {
	TrainingAccuracy    float64 `json:"training_accuracy"`
	ValidationAccuracy  float64 `json:"validation_accuracy"`
	TestAccuracy        float64 `json:"test_accuracy,omitempty"`
	AccuracyGap         float64 `json:"accuracy_gap"`
	VarianceScore       float64 `json:"variance_score"`
	BiasScore           float64 `json:"bias_score"`
	ComplexityScore     float64 `json:"complexity_score"`
	GeneralizationScore float64 `json:"generalization_score"`
}

// ModelComplexity represents the complexity characteristics of a model
type ModelComplexity struct {
	ParameterCount    int     `json:"parameter_count"`
	EffectiveRatio    float64 `json:"effective_ratio"` // Parameters / Sample Size
	ComplexityScore   float64 `json:"complexity_score"`
	RecommendedAction string  `json:"recommended_action"`
}

// NewOverfittingPrevention creates a new overfitting prevention system
func NewOverfittingPrevention(config *patterns.PatternDiscoveryConfig, log *logrus.Logger) *OverfittingPrevention {
	return &OverfittingPrevention{
		config: config,
		log:    log,
	}
}

// AssessOverfittingRisk evaluates the risk of overfitting for a model
func (op *OverfittingPrevention) AssessOverfittingRisk(
	trainingData []*engine.WorkflowExecutionData,
	model *MLModel,
	crossValMetrics *CrossValidationMetrics,
) *OverfittingAssessment {

	assessment := &OverfittingAssessment{
		Indicators:           make([]*OverfittingIndicator, 0),
		Recommendations:      make([]string, 0),
		PreventionStrategies: make([]string, 0),
	}

	// 1. Check training vs validation accuracy gap
	gapIndicator := op.checkTrainingValidationGap(crossValMetrics)
	assessment.Indicators = append(assessment.Indicators, gapIndicator)

	// 2. Check model complexity relative to data size
	complexityIndicator := op.checkModelComplexity(model, len(trainingData))
	assessment.Indicators = append(assessment.Indicators, complexityIndicator)

	// 3. Check variance in cross-validation scores
	varianceIndicator := op.checkCrossValidationVariance(crossValMetrics)
	assessment.Indicators = append(assessment.Indicators, varianceIndicator)

	// 4. Check learning curve characteristics
	learningCurveIndicator := op.checkLearningCurve(trainingData, model)
	assessment.Indicators = append(assessment.Indicators, learningCurveIndicator)

	// 5. Check feature-to-sample ratio
	featureRatioIndicator := op.checkFeatureToSampleRatio(model, len(trainingData))
	assessment.Indicators = append(assessment.Indicators, featureRatioIndicator)

	// 6. Check temporal stability of performance
	temporalIndicator := op.checkTemporalStability(trainingData)
	assessment.Indicators = append(assessment.Indicators, temporalIndicator)

	// Calculate overall risk score
	assessment.RiskScore = op.calculateOverallRiskScore(assessment.Indicators)
	assessment.OverfittingRisk = op.determineRiskLevel(assessment.RiskScore)

	// Generate recommendations based on indicators
	assessment.Recommendations = op.generateRecommendations(assessment.Indicators)
	assessment.PreventionStrategies = op.generatePreventionStrategies(assessment.Indicators)

	// Create validation metrics summary
	assessment.ValidationMetrics = op.createValidationMetrics(crossValMetrics, assessment.Indicators)

	// Determine if model is reliable for production use
	assessment.IsModelReliable = assessment.OverfittingRisk != OverfittingRiskHigh &&
		assessment.OverfittingRisk != OverfittingRiskCritical &&
		assessment.RiskScore < 0.7

	op.log.WithFields(logrus.Fields{
		"overfitting_risk":  assessment.OverfittingRisk,
		"risk_score":        assessment.RiskScore,
		"is_model_reliable": assessment.IsModelReliable,
		"indicators_count":  len(assessment.Indicators),
	}).Info("Overfitting assessment completed")

	return assessment
}

// ImplementRegularization applies regularization techniques to prevent overfitting
func (op *OverfittingPrevention) ImplementRegularization(
	trainingData []*engine.WorkflowExecutionData,
	modelType string,
) (*RegularizationConfig, error) {

	config := &RegularizationConfig{
		Techniques: make(map[string]interface{}),
	}

	sampleSize := len(trainingData)

	// Determine appropriate regularization based on data characteristics
	if sampleSize < op.config.MinExecutionsForPattern*3 {
		// Small dataset - stronger regularization needed
		config.Techniques["early_stopping"] = map[string]interface{}{
			"patience":         5,
			"min_improvement":  0.001,
			"validation_split": 0.3,
		}

		config.Techniques["dropout"] = map[string]interface{}{
			"rate": 0.3,
		}

		if modelType == "classification" {
			config.Techniques["l2_regularization"] = map[string]interface{}{
				"lambda": 0.1,
			}
		}
	} else if sampleSize < op.config.MinExecutionsForPattern*10 {
		// Medium dataset - moderate regularization
		config.Techniques["early_stopping"] = map[string]interface{}{
			"patience":         10,
			"min_improvement":  0.0001,
			"validation_split": 0.2,
		}

		config.Techniques["dropout"] = map[string]interface{}{
			"rate": 0.2,
		}
	} else {
		// Large dataset - light regularization
		config.Techniques["early_stopping"] = map[string]interface{}{
			"patience":         15,
			"min_improvement":  0.00001,
			"validation_split": 0.15,
		}

		config.Techniques["dropout"] = map[string]interface{}{
			"rate": 0.1,
		}
	}

	// Add cross-validation configuration
	config.Techniques["cross_validation"] = map[string]interface{}{
		"folds":       op.calculateOptimalFolds(sampleSize),
		"stratified":  true,
		"shuffle":     true,
		"random_seed": 42,
	}

	// Add ensemble methods for better generalization
	config.Techniques["ensemble"] = map[string]interface{}{
		"method":         "bagging",
		"n_estimators":   op.calculateOptimalEstimators(sampleSize),
		"bootstrap":      true,
		"feature_subset": 0.8,
	}

	return config, nil
}

// MonitorModelPerformance continuously monitors model performance to detect overfitting
func (op *OverfittingPrevention) MonitorModelPerformance(
	model *MLModel,
	recentData []*engine.WorkflowExecutionData,
) *PerformanceMonitoringResult {

	result := &PerformanceMonitoringResult{
		ModelID:         model.ID,
		MonitoringTime:  time.Now(),
		Alerts:          make([]*PerformanceAlert, 0),
		Recommendations: make([]string, 0),
		ShouldRetrain:   false,
	}

	// Check if performance has degraded significantly
	if op.hasPerformanceDegraded(model, recentData) {
		result.Alerts = append(result.Alerts, &PerformanceAlert{
			Type:        "performance_degradation",
			Severity:    "high",
			Message:     "Model performance has degraded significantly",
			Threshold:   0.1, // 10% degradation threshold
			ActualValue: op.calculateCurrentPerformance(model, recentData),
		})
		result.ShouldRetrain = true
	}

	// Check for distribution drift
	if op.hasDistributionDrift(model, recentData) {
		result.Alerts = append(result.Alerts, &PerformanceAlert{
			Type:        "distribution_drift",
			Severity:    "medium",
			Message:     "Input data distribution has shifted",
			Threshold:   0.2,
			ActualValue: op.calculateDistributionDrift(model, recentData),
		})
		result.Recommendations = append(result.Recommendations,
			"Consider retraining model with recent data")
	}

	// Check model staleness
	daysSinceTraining := time.Since(model.TrainedAt).Hours() / 24
	maxDays := float64(op.config.MaxHistoryDays) / 2 // Retrain at half the max history

	if daysSinceTraining > maxDays {
		result.Alerts = append(result.Alerts, &PerformanceAlert{
			Type:        "model_staleness",
			Severity:    "low",
			Message:     "Model is getting stale",
			Threshold:   maxDays,
			ActualValue: daysSinceTraining,
		})
		result.Recommendations = append(result.Recommendations,
			"Schedule model retraining with recent data")
	}

	return result
}

// Private helper methods

func (op *OverfittingPrevention) checkTrainingValidationGap(metrics *CrossValidationMetrics) *OverfittingIndicator {
	if metrics == nil {
		return &OverfittingIndicator{
			Name:        "training_validation_gap",
			Description: "Gap between training and validation accuracy",
			Value:       0.0,
			Threshold:   0.1,
			Severity:    "unknown",
			Detected:    false,
			Impact:      "Unable to assess - no cross-validation metrics",
		}
	}

	// Estimate training accuracy (usually higher than validation)
	estimatedTrainingAccuracy := metrics.MeanAccuracy + metrics.StdAccuracy
	gap := estimatedTrainingAccuracy - metrics.MeanAccuracy

	threshold := 0.1 // 10% gap threshold
	detected := gap > threshold
	severity := op.determineSeverity(gap, threshold)

	return &OverfittingIndicator{
		Name:        "training_validation_gap",
		Description: "Gap between training and validation accuracy",
		Value:       gap,
		Threshold:   threshold,
		Severity:    severity,
		Detected:    detected,
		Impact:      op.getImpactMessage(detected, "Model may not generalize well"),
	}
}

func (op *OverfittingPrevention) checkModelComplexity(model *MLModel, sampleSize int) *OverfittingIndicator {
	if model == nil {
		return op.createUnknownIndicator("model_complexity", "Model complexity relative to sample size")
	}

	// Estimate model complexity
	parameterCount := len(model.Weights) + 1 // weights + bias
	if parameterCount == 1 {
		// Estimate from feature count
		parameterCount = len(model.Features) * 2 // Rough estimate
	}

	complexity := float64(parameterCount) / float64(sampleSize)
	threshold := 0.1 // 10% ratio threshold (10 samples per parameter)
	detected := complexity > threshold

	return &OverfittingIndicator{
		Name:        "model_complexity",
		Description: "Model complexity relative to sample size",
		Value:       complexity,
		Threshold:   threshold,
		Severity:    op.determineSeverity(complexity, threshold),
		Detected:    detected,
		Impact:      op.getImpactMessage(detected, "Model may overfit due to high complexity"),
	}
}

func (op *OverfittingPrevention) checkCrossValidationVariance(metrics *CrossValidationMetrics) *OverfittingIndicator {
	if metrics == nil {
		return op.createUnknownIndicator("cross_validation_variance", "Variance in cross-validation scores")
	}

	variance := metrics.StdAccuracy
	threshold := 0.15 // 15% standard deviation threshold
	detected := variance > threshold

	return &OverfittingIndicator{
		Name:        "cross_validation_variance",
		Description: "Variance in cross-validation scores",
		Value:       variance,
		Threshold:   threshold,
		Severity:    op.determineSeverity(variance, threshold),
		Detected:    detected,
		Impact:      op.getImpactMessage(detected, "High variance indicates unstable model"),
	}
}

func (op *OverfittingPrevention) checkLearningCurve(data []*engine.WorkflowExecutionData, model *MLModel) *OverfittingIndicator {
	// Simplified learning curve analysis
	// In a real implementation, this would analyze performance vs training set size

	sampleSize := len(data)
	minSize := op.config.MinExecutionsForPattern * 5

	ratio := float64(sampleSize) / float64(minSize)
	threshold := 1.0 // Need at least the minimum recommended size
	detected := ratio < threshold

	return &OverfittingIndicator{
		Name:        "learning_curve",
		Description: "Adequacy of training data for learning curve",
		Value:       ratio,
		Threshold:   threshold,
		Severity:    op.determineSeverity(1.0-ratio, 0.5), // Invert for severity calculation
		Detected:    detected,
		Impact:      op.getImpactMessage(detected, "Insufficient data may lead to overfitting"),
	}
}

func (op *OverfittingPrevention) checkFeatureToSampleRatio(model *MLModel, sampleSize int) *OverfittingIndicator {
	if model == nil {
		return op.createUnknownIndicator("feature_sample_ratio", "Feature to sample ratio")
	}

	featureCount := len(model.Features)
	if featureCount == 0 {
		featureCount = 10 // Default estimate
	}

	ratio := float64(featureCount) / float64(sampleSize)
	threshold := 0.2 // 20% ratio threshold (5 samples per feature)
	detected := ratio > threshold

	return &OverfittingIndicator{
		Name:        "feature_sample_ratio",
		Description: "Feature to sample ratio",
		Value:       ratio,
		Threshold:   threshold,
		Severity:    op.determineSeverity(ratio, threshold),
		Detected:    detected,
		Impact:      op.getImpactMessage(detected, "Too many features relative to samples"),
	}
}

func (op *OverfittingPrevention) checkTemporalStability(data []*engine.WorkflowExecutionData) *OverfittingIndicator {
	if len(data) < 10 {
		return op.createUnknownIndicator("temporal_stability", "Temporal stability of patterns")
	}

	// Check if performance varies significantly over time
	timeWindows := op.splitIntoTimeWindows(data, 3) // Split into 3 time periods
	if len(timeWindows) < 3 {
		return op.createUnknownIndicator("temporal_stability", "Insufficient time periods")
	}

	// Calculate success rates for each window
	successRates := make([]float64, len(timeWindows))
	for i, window := range timeWindows {
		successCount := 0
		for _, d := range window {
			if d.Success {
				successCount++
			}
		}
		successRates[i] = float64(successCount) / float64(len(window))
	}

	// Calculate variance across time windows
	variance := op.calculateVariance(successRates)
	threshold := 0.1 // 10% variance threshold
	detected := variance > threshold

	return &OverfittingIndicator{
		Name:        "temporal_stability",
		Description: "Temporal stability of success patterns",
		Value:       variance,
		Threshold:   threshold,
		Severity:    op.determineSeverity(variance, threshold),
		Detected:    detected,
		Impact:      op.getImpactMessage(detected, "Patterns may not be stable over time"),
	}
}

func (op *OverfittingPrevention) calculateOverallRiskScore(indicators []*OverfittingIndicator) float64 {
	if len(indicators) == 0 {
		return 0.5 // Neutral score
	}

	score := 0.0
	weights := map[string]float64{
		"training_validation_gap":   0.3,
		"model_complexity":          0.2,
		"cross_validation_variance": 0.2,
		"learning_curve":            0.1,
		"feature_sample_ratio":      0.1,
		"temporal_stability":        0.1,
	}

	totalWeight := 0.0
	for _, indicator := range indicators {
		weight := weights[indicator.Name]
		if weight == 0 {
			weight = 0.1 // Default weight
		}

		// Convert indicator to risk contribution
		indicatorRisk := 0.0
		if indicator.Detected {
			indicatorRisk = math.Min(1.0, indicator.Value/indicator.Threshold)
		} else {
			indicatorRisk = indicator.Value / indicator.Threshold * 0.5
		}

		score += indicatorRisk * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return 0.5
	}

	return math.Min(1.0, score/totalWeight)
}

func (op *OverfittingPrevention) determineRiskLevel(riskScore float64) OverfittingRisk {
	switch {
	case riskScore < 0.3:
		return OverfittingRiskLow
	case riskScore < 0.6:
		return OverfittingRiskModerate
	case riskScore < 0.8:
		return OverfittingRiskHigh
	default:
		return OverfittingRiskCritical
	}
}

func (op *OverfittingPrevention) generateRecommendations(indicators []*OverfittingIndicator) []string {
	recommendations := make([]string, 0)
	detectedIssues := make(map[string]bool)

	for _, indicator := range indicators {
		if indicator.Detected {
			detectedIssues[indicator.Name] = true
		}
	}

	if detectedIssues["training_validation_gap"] {
		recommendations = append(recommendations, "Implement cross-validation and regularization")
		recommendations = append(recommendations, "Increase training data size")
	}

	if detectedIssues["model_complexity"] {
		recommendations = append(recommendations, "Reduce model complexity (fewer parameters)")
		recommendations = append(recommendations, "Implement L1/L2 regularization")
	}

	if detectedIssues["cross_validation_variance"] {
		recommendations = append(recommendations, "Use ensemble methods to reduce variance")
		recommendations = append(recommendations, "Increase cross-validation folds")
	}

	if detectedIssues["learning_curve"] {
		recommendations = append(recommendations, "Collect more training data")
		recommendations = append(recommendations, "Use data augmentation techniques")
	}

	if detectedIssues["feature_sample_ratio"] {
		recommendations = append(recommendations, "Implement feature selection")
		recommendations = append(recommendations, "Use dimensionality reduction techniques")
	}

	if detectedIssues["temporal_stability"] {
		recommendations = append(recommendations, "Use time-based validation splits")
		recommendations = append(recommendations, "Implement online learning for adaptation")
	}

	return recommendations
}

func (op *OverfittingPrevention) generatePreventionStrategies(indicators []*OverfittingIndicator) []string {
	strategies := []string{
		"Implement k-fold cross-validation",
		"Use early stopping during training",
		"Apply appropriate regularization techniques",
		"Monitor validation metrics during training",
		"Use ensemble methods for better generalization",
	}

	// Add specific strategies based on detected issues
	for _, indicator := range indicators {
		if indicator.Detected {
			switch indicator.Name {
			case "model_complexity":
				strategies = append(strategies, "Implement model pruning techniques")
			case "feature_sample_ratio":
				strategies = append(strategies, "Apply feature selection algorithms")
			case "temporal_stability":
				strategies = append(strategies, "Use time-aware validation strategies")
			}
		}
	}

	return strategies
}

// Additional supporting types and methods

type RegularizationConfig struct {
	Techniques map[string]interface{} `json:"techniques"`
}

type PerformanceMonitoringResult struct {
	ModelID         string              `json:"model_id"`
	MonitoringTime  time.Time           `json:"monitoring_time"`
	Alerts          []*PerformanceAlert `json:"alerts"`
	Recommendations []string            `json:"recommendations"`
	ShouldRetrain   bool                `json:"should_retrain"`
}

type PerformanceAlert struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"`
	Message     string  `json:"message"`
	Threshold   float64 `json:"threshold"`
	ActualValue float64 `json:"actual_value"`
}

// Utility functions

func (op *OverfittingPrevention) createUnknownIndicator(name, description string) *OverfittingIndicator {
	return &OverfittingIndicator{
		Name:        name,
		Description: description,
		Value:       0.0,
		Threshold:   1.0,
		Severity:    "unknown",
		Detected:    false,
		Impact:      "Unable to assess - insufficient data",
	}
}

func (op *OverfittingPrevention) determineSeverity(value, threshold float64) string {
	ratio := value / threshold
	switch {
	case ratio < 0.5:
		return "low"
	case ratio < 1.0:
		return "medium"
	case ratio < 2.0:
		return "high"
	default:
		return "critical"
	}
}

func (op *OverfittingPrevention) getImpactMessage(detected bool, message string) string {
	if detected {
		return message
	}
	return "No significant impact detected"
}

func (op *OverfittingPrevention) splitIntoTimeWindows(data []*engine.WorkflowExecutionData, numWindows int) [][]*engine.WorkflowExecutionData {
	if len(data) < numWindows {
		return nil
	}

	// Sort by timestamp
	sortedData := make([]*engine.WorkflowExecutionData, len(data))
	copy(sortedData, data)
	sort.Slice(sortedData, func(i, j int) bool {
		return sortedData[i].Timestamp.Before(sortedData[j].Timestamp)
	})

	windowSize := len(sortedData) / numWindows
	windows := make([][]*engine.WorkflowExecutionData, numWindows)

	for i := 0; i < numWindows; i++ {
		start := i * windowSize
		end := start + windowSize
		if i == numWindows-1 {
			end = len(sortedData) // Include remaining items in last window
		}
		windows[i] = sortedData[start:end]
	}

	return windows
}

func (op *OverfittingPrevention) calculateVariance(values []float64) float64 {
	if len(values) < 2 {
		return 0.0
	}

	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	variance /= float64(len(values) - 1)

	return variance
}

func (op *OverfittingPrevention) calculateOptimalFolds(sampleSize int) int {
	// Common rule: use 5-fold for small datasets, 10-fold for larger ones
	if sampleSize < 50 {
		return 5
	} else if sampleSize < 200 {
		return 10
	}
	return 10 // Cap at 10-fold
}

func (op *OverfittingPrevention) calculateOptimalEstimators(sampleSize int) int {
	// Rule of thumb: more estimators for larger datasets, but cap for efficiency
	estimators := int(math.Sqrt(float64(sampleSize)))
	return int(math.Min(math.Max(float64(estimators), 3), 20))
}

func (op *OverfittingPrevention) createValidationMetrics(crossVal *CrossValidationMetrics, indicators []*OverfittingIndicator) *ValidationMetrics {
	metrics := &ValidationMetrics{
		ValidationAccuracy: crossVal.MeanAccuracy,
		VarianceScore:      crossVal.StdAccuracy,
	}

	// Extract values from indicators
	for _, indicator := range indicators {
		switch indicator.Name {
		case "training_validation_gap":
			metrics.TrainingAccuracy = crossVal.MeanAccuracy + indicator.Value
			metrics.AccuracyGap = indicator.Value
		case "model_complexity":
			metrics.ComplexityScore = indicator.Value
		}
	}

	// Calculate derived metrics
	if metrics.AccuracyGap > 0 && metrics.ValidationAccuracy > 0 {
		metrics.GeneralizationScore = 1.0 - (metrics.AccuracyGap / metrics.ValidationAccuracy)
	}

	metrics.BiasScore = 1.0 - metrics.ValidationAccuracy // Higher bias = lower validation accuracy

	return metrics
}

// Stub implementations for monitoring methods (would be implemented based on actual model usage)
func (op *OverfittingPrevention) hasPerformanceDegraded(model *MLModel, recentData []*engine.WorkflowExecutionData) bool {
	// Placeholder implementation
	return false
}

func (op *OverfittingPrevention) hasDistributionDrift(model *MLModel, recentData []*engine.WorkflowExecutionData) bool {
	// Placeholder implementation
	return false
}

func (op *OverfittingPrevention) calculateCurrentPerformance(model *MLModel, recentData []*engine.WorkflowExecutionData) float64 {
	// Placeholder implementation
	return model.Accuracy
}

func (op *OverfittingPrevention) calculateDistributionDrift(model *MLModel, recentData []*engine.WorkflowExecutionData) float64 {
	// Placeholder implementation
	return 0.0
}
