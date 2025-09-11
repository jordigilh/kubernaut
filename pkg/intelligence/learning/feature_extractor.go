package learning

import (
	"crypto/md5"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	workflowtypes "github.com/jordigilh/kubernaut/pkg/workflow/types"
	"github.com/sirupsen/logrus"
)

// FeatureExtractor extracts numerical features from workflow execution data
type FeatureExtractor struct {
	config       *PatternDiscoveryConfig
	log          *logrus.Logger
	featureNames []string
	scaleFactors map[string]float64
	encodings    map[string]map[string]int
}

// NewFeatureExtractor creates a new feature extractor
func NewFeatureExtractor(config *PatternDiscoveryConfig, log *logrus.Logger) *FeatureExtractor {
	fe := &FeatureExtractor{
		config:       config,
		log:          log,
		scaleFactors: make(map[string]float64),
		encodings:    make(map[string]map[string]int),
	}

	fe.initializeFeatureNames()
	fe.initializeEncodings()
	fe.initializeScaleFactors()

	return fe
}

// Extract extracts features from workflow execution data
func (fe *FeatureExtractor) Extract(data *sharedtypes.WorkflowExecutionData) (*shared.WorkflowFeatures, error) {
	features := &shared.WorkflowFeatures{
		CustomMetrics: make(map[string]float64),
	}

	// Extract alert-based features
	if err := fe.extractAlertFeatures(data, features); err != nil {
		return nil, fmt.Errorf("failed to extract alert features: %w", err)
	}

	// Extract resource-based features
	if err := fe.extractResourceFeatures(data, features); err != nil {
		return nil, fmt.Errorf("failed to extract resource features: %w", err)
	}

	// Extract temporal features
	if err := fe.extractTemporalFeatures(data, features); err != nil {
		return nil, fmt.Errorf("failed to extract temporal features: %w", err)
	}

	// Extract historical features
	if err := fe.extractHistoricalFeatures(data, features); err != nil {
		fe.log.WithError(err).Warn("Failed to extract historical features")
		// Don't fail completely, just set defaults
		features.RecentFailures = 0
		features.AverageSuccessRate = 0.5
		features.LastExecutionTime = 0
	}

	// Extract complexity features
	if err := fe.extractComplexityFeatures(data, features); err != nil {
		return nil, fmt.Errorf("failed to extract complexity features: %w", err)
	}

	// Extract environment features
	if err := fe.extractEnvironmentFeatures(data, features); err != nil {
		fe.log.WithError(err).Warn("Failed to extract environment features")
		// Set defaults
		features.ClusterSize = 10
		features.ClusterLoad = 0.5
		features.ResourcePressure = 0.3
	}

	return features, nil
}

// ExtractVector converts workflow execution data to a numerical vector
func (fe *FeatureExtractor) ExtractVector(data *sharedtypes.WorkflowExecutionData) ([]float64, error) {
	features, err := fe.Extract(data)
	if err != nil {
		return nil, err
	}

	return fe.ConvertToVector(features)
}

// ExtractFromLearningData extracts features from learning data
func (fe *FeatureExtractor) ExtractFromLearningData(learningData *shared.WorkflowLearningData) (*shared.WorkflowFeatures, error) {
	return learningData.Features, nil
}

// ConvertToVector converts WorkflowFeatures to a numerical vector
func (fe *FeatureExtractor) ConvertToVector(features *shared.WorkflowFeatures) ([]float64, error) {
	vector := make([]float64, len(fe.featureNames))

	for i, featureName := range fe.featureNames {
		value, err := fe.getFeatureValue(features, featureName)
		if err != nil {
			fe.log.WithError(err).WithField("feature", featureName).Warn("Failed to get feature value")
			vector[i] = 0.0
			continue
		}

		// Apply scaling if configured
		if scaleFactor, exists := fe.scaleFactors[featureName]; exists {
			value *= scaleFactor
		}

		vector[i] = value
	}

	return vector, nil
}

// GetFeatureNames returns the list of feature names
func (fe *FeatureExtractor) GetFeatureNames() []string {
	return fe.featureNames
}

// GetFeatureImportance analyzes feature importance for a given model
func (fe *FeatureExtractor) GetFeatureImportance(model *MLModel) map[string]float64 {
	importance := make(map[string]float64)

	if len(model.Weights) != len(fe.featureNames) {
		fe.log.Warn("Model weights length doesn't match feature names length")
		return importance
	}

	// For linear models, absolute weight magnitude indicates importance
	for i, featureName := range fe.featureNames {
		importance[featureName] = math.Abs(model.Weights[i])
	}

	// Normalize importance scores
	maxImportance := 0.0
	for _, imp := range importance {
		if imp > maxImportance {
			maxImportance = imp
		}
	}

	if maxImportance > 0 {
		for featureName := range importance {
			importance[featureName] /= maxImportance
		}
	}

	return importance
}

// AnalyzeFeatureCorrelations analyzes correlations between features
func (fe *FeatureExtractor) AnalyzeFeatureCorrelations(data []*sharedtypes.WorkflowExecutionData) (*FeatureCorrelationAnalysis, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("insufficient data for correlation analysis: %d samples", len(data))
	}

	// Extract feature vectors for all data points
	featureVectors := make([][]float64, 0)
	for _, execData := range data {
		vector, err := fe.ExtractVector(execData)
		if err != nil {
			continue
		}
		featureVectors = append(featureVectors, vector)
	}

	if len(featureVectors) < 10 {
		return nil, fmt.Errorf("insufficient valid feature vectors: %d", len(featureVectors))
	}

	numFeatures := len(fe.featureNames)
	correlationMatrix := make([][]float64, numFeatures)
	for i := range correlationMatrix {
		correlationMatrix[i] = make([]float64, numFeatures)
	}

	// Calculate Pearson correlation coefficients
	for i := 0; i < numFeatures; i++ {
		for j := 0; j < numFeatures; j++ {
			if i == j {
				correlationMatrix[i][j] = 1.0
				continue
			}

			correlation := fe.calculatePearsonCorrelation(featureVectors, i, j)
			correlationMatrix[i][j] = correlation
		}
	}

	// Identify highly correlated feature pairs
	highCorrelations := make([]*FeatureCorrelation, 0)
	for i := 0; i < numFeatures; i++ {
		for j := i + 1; j < numFeatures; j++ {
			correlation := math.Abs(correlationMatrix[i][j])
			if correlation > 0.8 { // High correlation threshold
				highCorrelations = append(highCorrelations, &FeatureCorrelation{
					Feature1:    fe.featureNames[i],
					Feature2:    fe.featureNames[j],
					Correlation: correlationMatrix[i][j],
				})
			}
		}
	}

	analysis := &FeatureCorrelationAnalysis{
		CorrelationMatrix: correlationMatrix,
		FeatureNames:      fe.featureNames,
		HighCorrelations:  highCorrelations,
		AnalyzedSamples:   len(featureVectors),
	}

	return analysis, nil
}

// Private methods for feature extraction

func (fe *FeatureExtractor) initializeFeatureNames() {
	fe.featureNames = []string{
		// Alert features
		"alert_count",
		"severity_score",
		"alert_duration_seconds",
		"alert_type_encoded",

		// Resource features
		"resource_count",
		"namespace_count",
		"deployment_count",
		"pod_count",
		"service_count",

		// Temporal features
		"hour_of_day",
		"day_of_week",
		"is_weekend",
		"is_business_hour",
		"month_of_year",

		// Historical features
		"recent_failures",
		"average_success_rate",
		"last_execution_hours",
		"execution_frequency",

		// Complexity features
		"step_count",
		"dependency_depth",
		"parallel_steps",
		"condition_steps",
		"action_steps",

		// Environment features
		"cluster_size",
		"cluster_load",
		"resource_pressure",
		"cpu_utilization",
		"memory_utilization",
		"network_utilization",
		"storage_utilization",
	}
}

func (fe *FeatureExtractor) initializeEncodings() {
	// Initialize categorical encodings
	fe.encodings["alert_types"] = map[string]int{
		"HighMemoryUsage":   1,
		"PodCrashLoop":      2,
		"NodeNotReady":      3,
		"DiskSpaceCritical": 4,
		"NetworkIssue":      5,
		"ServiceDown":       6,
		"DeploymentFailed":  7,
		"Unknown":           0,
	}

	fe.encodings["resource_types"] = map[string]int{
		"deployment": 1,
		"pod":        2,
		"service":    3,
		"node":       4,
		"pvc":        5,
		"configmap":  6,
		"secret":     7,
		"unknown":    0,
	}

	fe.encodings["severity"] = map[string]int{
		"critical": 4,
		"warning":  3,
		"info":     2,
		"debug":    1,
		"unknown":  0,
	}
}

func (fe *FeatureExtractor) initializeScaleFactors() {
	// Initialize scaling factors to normalize features
	fe.scaleFactors = map[string]float64{
		"alert_duration_seconds": 1.0 / 3600.0, // Scale to hours
		"last_execution_hours":   1.0 / 24.0,   // Scale to days
		"step_count":             1.0 / 20.0,   // Normalize to 0-1 range assuming max 20 steps
		"dependency_depth":       1.0 / 10.0,   // Normalize assuming max depth 10
		"cluster_size":           1.0 / 1000.0, // Scale assuming max 1000 nodes
		"resource_count":         1.0 / 100.0,  // Scale assuming max 100 resources
	}
}

func (fe *FeatureExtractor) extractAlertFeatures(data *sharedtypes.WorkflowExecutionData, features *shared.WorkflowFeatures) error {
	// Extract alert info from metadata
	alertData, hasAlert := data.Metadata["alert"]
	if !hasAlert {
		// Set default values for missing alert
		features.AlertCount = 0
		features.SeverityScore = 0.0
		features.AlertDuration = 0
		features.AlertTypes = make(map[string]int)
		return nil
	}

	alertMap, ok := alertData.(map[string]interface{})
	if !ok {
		// Set defaults if alert data is malformed
		features.AlertCount = 0
		features.SeverityScore = 0.0
		features.AlertDuration = 0
		features.AlertTypes = make(map[string]int)
		return nil
	}

	// Basic alert features
	features.AlertCount = 1 // Single alert for now

	if alertName, ok := alertMap["name"].(string); ok {
		features.AlertTypes = map[string]int{alertName: 1}
	} else {
		features.AlertTypes = make(map[string]int)
	}

	// Severity score
	if severity, ok := alertMap["severity"].(string); ok {
		if encoding, exists := fe.encodings["severity"][strings.ToLower(severity)]; exists {
			features.SeverityScore = float64(encoding) / 4.0 // Normalize to 0-1
		} else {
			features.SeverityScore = 0.5 // Default for unknown severity
		}
	} else {
		features.SeverityScore = 0.5
	}

	// Alert duration (if available in metadata)
	if startTime, exists := data.Metadata["alert_start_time"]; exists {
		if startTimeVal, ok := startTime.(time.Time); ok {
			features.AlertDuration = data.Timestamp.Sub(startTimeVal)
		}
	}

	// Add custom metrics based on alert labels
	if labels, ok := alertMap["labels"].(map[string]interface{}); ok {
		features.CustomMetrics["alert_label_count"] = float64(len(labels))
	}

	// Hash alert description for uniqueness
	if description, ok := alertMap["description"].(string); ok {
		descHash := fe.hashString(description)
		features.CustomMetrics["alert_description_hash"] = float64(descHash)
	}

	return nil
}

func (fe *FeatureExtractor) extractResourceFeatures(data *sharedtypes.WorkflowExecutionData, features *shared.WorkflowFeatures) error {
	// Extract resource info from metadata
	resourceData, hasResource := data.Metadata["resource"]
	if !hasResource {
		features.ResourceCount = 0
		features.ResourceTypes = make(map[string]int)
		features.NamespaceCount = 0
		return nil
	}

	resourceMap, ok := resourceData.(map[string]interface{})
	if !ok {
		features.ResourceCount = 0
		features.ResourceTypes = make(map[string]int)
		features.NamespaceCount = 0
		return nil
	}

	// Basic resource features
	features.ResourceCount = 1 // Single resource for now
	features.NamespaceCount = 1

	// Resource type encoding
	if resourceType, ok := resourceMap["type"].(string); ok {
		features.ResourceTypes = map[string]int{strings.ToLower(resourceType): 1}
	} else {
		features.ResourceTypes = make(map[string]int)
	}

	// Extract from metadata if available
	if contextData, exists := data.Metadata["resource_info"]; exists {
		if resourceInfo, ok := contextData.(map[string]interface{}); ok {
			// Extract deployment count
			if deplCount, exists := resourceInfo["deployment_count"]; exists {
				if count, ok := deplCount.(int); ok {
					features.CustomMetrics["deployment_count"] = float64(count)
				}
			}

			// Extract pod count
			if podCount, exists := resourceInfo["pod_count"]; exists {
				if count, ok := podCount.(int); ok {
					features.CustomMetrics["pod_count"] = float64(count)
				}
			}

			// Extract service count
			if svcCount, exists := resourceInfo["service_count"]; exists {
				if count, ok := svcCount.(int); ok {
					features.CustomMetrics["service_count"] = float64(count)
				}
			}
		}
	}

	return nil
}

func (fe *FeatureExtractor) extractTemporalFeatures(data *sharedtypes.WorkflowExecutionData, features *shared.WorkflowFeatures) error {
	timestamp := data.Timestamp

	// Time-based features
	features.HourOfDay = timestamp.Hour()
	features.DayOfWeek = int(timestamp.Weekday())
	features.IsWeekend = timestamp.Weekday() == time.Saturday || timestamp.Weekday() == time.Sunday
	features.IsBusinessHour = timestamp.Hour() >= 9 && timestamp.Hour() <= 17 && !features.IsWeekend

	// Additional temporal features
	features.CustomMetrics["month_of_year"] = float64(timestamp.Month())
	features.CustomMetrics["day_of_month"] = float64(timestamp.Day())
	features.CustomMetrics["quarter"] = float64((timestamp.Month()-1)/3 + 1)

	return nil
}

func (fe *FeatureExtractor) extractHistoricalFeatures(data *sharedtypes.WorkflowExecutionData, features *shared.WorkflowFeatures) error {
	// These would typically query historical data from database
	// For now, use context data or defaults

	if histData, exists := data.Metadata["historical_data"]; exists {
		if histMap, ok := histData.(map[string]interface{}); ok {
			// Extract recent failures
			if recentFailures, exists := histMap["recent_failures"]; exists {
				if failures, ok := recentFailures.(int); ok {
					features.RecentFailures = failures
				}
			}

			// Extract average success rate
			if successRate, exists := histMap["average_success_rate"]; exists {
				if rate, ok := successRate.(float64); ok {
					features.AverageSuccessRate = rate
				}
			}

			// Extract last execution time
			if lastExec, exists := histMap["last_execution_time"]; exists {
				if lastTime, ok := lastExec.(time.Time); ok {
					features.LastExecutionTime = data.Timestamp.Sub(lastTime)
				}
			}
		}
	} else {
		// Set defaults when historical data is not available
		features.RecentFailures = 0
		features.AverageSuccessRate = 0.5           // Neutral assumption
		features.LastExecutionTime = 24 * time.Hour // Assume last execution was 24h ago
	}

	return nil
}

func (fe *FeatureExtractor) extractComplexityFeatures(data *sharedtypes.WorkflowExecutionData, features *shared.WorkflowFeatures) error {
	// These would be extracted from the workflow template
	// For now, use context data or make reasonable estimates

	if workflowData, exists := data.Metadata["workflow_template"]; exists {
		if workflowMap, ok := workflowData.(map[string]interface{}); ok {
			// Extract step count
			if stepCount, exists := workflowMap["step_count"]; exists {
				if count, ok := stepCount.(int); ok {
					features.StepCount = count
				}
			}

			// Extract dependency depth
			if depthData, exists := workflowMap["dependency_depth"]; exists {
				if depth, ok := depthData.(int); ok {
					features.DependencyDepth = depth
				}
			}

			// Extract parallel steps
			if parallelData, exists := workflowMap["parallel_steps"]; exists {
				if parallel, ok := parallelData.(int); ok {
					features.ParallelSteps = parallel
				}
			}
		}
	} else {
		// Estimate based on metadata or defaults
		var alertMap map[string]interface{}
		if alertData, exists := data.Metadata["alert"]; exists {
			alertMap, _ = alertData.(map[string]interface{})
		}
		features.StepCount = fe.estimateStepCountFromMap(alertMap)
		features.DependencyDepth = fe.estimateDependencyDepthFromMap(alertMap)
		features.ParallelSteps = fe.estimateParallelStepsFromMap(alertMap)
	}

	return nil
}

func (fe *FeatureExtractor) extractEnvironmentFeatures(data *sharedtypes.WorkflowExecutionData, features *shared.WorkflowFeatures) error {
	// These would be fetched from cluster metrics
	// For now, use context data or defaults

	if envData, exists := data.Metadata["environment_metrics"]; exists {
		if envMap, ok := envData.(map[string]interface{}); ok {
			// Extract cluster size
			if clusterSize, exists := envMap["cluster_size"]; exists {
				if size, ok := clusterSize.(int); ok {
					features.ClusterSize = size
				}
			}

			// Extract cluster load
			if clusterLoad, exists := envMap["cluster_load"]; exists {
				if load, ok := clusterLoad.(float64); ok {
					features.ClusterLoad = load
				}
			}

			// Extract resource pressure
			if resourcePressure, exists := envMap["resource_pressure"]; exists {
				if pressure, ok := resourcePressure.(float64); ok {
					features.ResourcePressure = pressure
				}
			}
		}
	} else {
		// Set reasonable defaults
		features.ClusterSize = 10
		features.ClusterLoad = 0.5
		features.ResourcePressure = 0.3
	}

	// Add resource utilization if available in metrics
	if cpuUsage, exists := data.Metrics["cpu_utilization"]; exists {
		features.CustomMetrics["cpu_utilization"] = cpuUsage
	}
	if memUsage, exists := data.Metrics["memory_utilization"]; exists {
		features.CustomMetrics["memory_utilization"] = memUsage
	}
	if netUsage, exists := data.Metrics["network_utilization"]; exists {
		features.CustomMetrics["network_utilization"] = netUsage
	}
	if storageUsage, exists := data.Metrics["storage_utilization"]; exists {
		features.CustomMetrics["storage_utilization"] = storageUsage
	}

	return nil
}

func (fe *FeatureExtractor) getFeatureValue(features *shared.WorkflowFeatures, featureName string) (float64, error) {
	switch featureName {
	case "alert_count":
		return float64(features.AlertCount), nil
	case "severity_score":
		return features.SeverityScore, nil
	case "alert_duration_seconds":
		return features.AlertDuration.Seconds(), nil
	case "alert_type_encoded":
		return fe.encodeAlertType(features.AlertTypes), nil
	case "resource_count":
		return float64(features.ResourceCount), nil
	case "namespace_count":
		return float64(features.NamespaceCount), nil
	case "hour_of_day":
		return float64(features.HourOfDay), nil
	case "day_of_week":
		return float64(features.DayOfWeek), nil
	case "is_weekend":
		if features.IsWeekend {
			return 1.0, nil
		}
		return 0.0, nil
	case "is_business_hour":
		if features.IsBusinessHour {
			return 1.0, nil
		}
		return 0.0, nil
	case "recent_failures":
		return float64(features.RecentFailures), nil
	case "average_success_rate":
		return features.AverageSuccessRate, nil
	case "last_execution_hours":
		return features.LastExecutionTime.Hours(), nil
	case "step_count":
		return float64(features.StepCount), nil
	case "dependency_depth":
		return float64(features.DependencyDepth), nil
	case "parallel_steps":
		return float64(features.ParallelSteps), nil
	case "cluster_size":
		return float64(features.ClusterSize), nil
	case "cluster_load":
		return features.ClusterLoad, nil
	case "resource_pressure":
		return features.ResourcePressure, nil
	default:
		// Check custom metrics
		if value, exists := features.CustomMetrics[featureName]; exists {
			return value, nil
		}
		return 0.0, fmt.Errorf("unknown feature: %s", featureName)
	}
}

func (fe *FeatureExtractor) encodeAlertType(alertTypes map[string]int) float64 {
	// Encode primary alert type
	maxCount := 0
	primaryType := "Unknown"

	for alertType, count := range alertTypes {
		if count > maxCount {
			maxCount = count
			primaryType = alertType
		}
	}

	if encoding, exists := fe.encodings["alert_types"][primaryType]; exists {
		return float64(encoding)
	}

	return 0.0 // Unknown type
}

func (fe *FeatureExtractor) hashString(s string) uint32 {
	hash := md5.Sum([]byte(s))
	return uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])
}

func (fe *FeatureExtractor) estimateStepCountFromMap(alertMap map[string]interface{}) int {
	if alertMap == nil {
		return 5 // Default estimate
	}

	alertName, ok := alertMap["name"].(string)
	if !ok {
		return 5
	}

	// Estimate based on alert type
	switch alertName {
	case "HighMemoryUsage":
		return 6 // Check, evaluate, scale, verify, rollback, cleanup
	case "PodCrashLoop":
		return 8 // Diagnose, analyze, restart, monitor, rollback, etc.
	case "NodeNotReady":
		return 10 // More complex node operations
	case "DiskSpaceCritical":
		return 7 // Cleanup, expand, verify
	case "NetworkIssue":
		return 6 // Test, restart, update
	default:
		return 5
	}
}

func (fe *FeatureExtractor) estimateDependencyDepthFromMap(alertMap map[string]interface{}) int {
	if alertMap == nil {
		return 2
	}

	alertName, ok := alertMap["name"].(string)
	if !ok {
		return 2
	}

	// Estimate based on complexity
	switch alertName {
	case "NodeNotReady":
		return 4 // Deep dependencies for node operations
	case "PodCrashLoop":
		return 3 // Medium depth
	default:
		return 2 // Shallow dependencies
	}
}

func (fe *FeatureExtractor) estimateParallelStepsFromMap(alertMap map[string]interface{}) int {
	if alertMap == nil {
		return 1
	}

	alertName, ok := alertMap["name"].(string)
	if !ok {
		return 1
	}

	// Most workflows have some parallel execution capability
	switch alertName {
	case "HighMemoryUsage":
		return 2 // Can check multiple resources in parallel
	case "DiskSpaceCritical":
		return 3 // Multiple cleanup operations
	default:
		return 1
	}
}

func (fe *FeatureExtractor) calculatePearsonCorrelation(vectors [][]float64, feature1, feature2 int) float64 {
	n := len(vectors)
	if n < 2 {
		return 0.0
	}

	// Extract feature values
	x := make([]float64, n)
	y := make([]float64, n)

	for i, vector := range vectors {
		x[i] = vector[feature1]
		y[i] = vector[feature2]
	}

	// Calculate means
	var sumX, sumY float64
	for i := 0; i < n; i++ {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)

	// Calculate correlation coefficient
	var numerator, denomX, denomY float64
	for i := 0; i < n; i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		numerator += dx * dy
		denomX += dx * dx
		denomY += dy * dy
	}

	if denomX == 0 || denomY == 0 {
		return 0.0
	}

	correlation := numerator / math.Sqrt(denomX*denomY)
	return correlation
}

// Supporting types

type FeatureCorrelationAnalysis struct {
	CorrelationMatrix [][]float64           `json:"correlation_matrix"`
	FeatureNames      []string              `json:"feature_names"`
	HighCorrelations  []*FeatureCorrelation `json:"high_correlations"`
	AnalyzedSamples   int                   `json:"analyzed_samples"`
}

type FeatureCorrelation struct {
	Feature1    string  `json:"feature1"`
	Feature2    string  `json:"feature2"`
	Correlation float64 `json:"correlation"`
}

// OutcomePredictor provides prediction capabilities
type OutcomePredictor struct {
	log *logrus.Logger
}

// NewOutcomePredictor creates a new outcome predictor
func NewOutcomePredictor(log *logrus.Logger) *OutcomePredictor {
	return &OutcomePredictor{
		log: log,
	}
}

// Predict predicts workflow outcome based on features and patterns
func (op *OutcomePredictor) Predict(features *shared.WorkflowFeatures, patterns []*shared.DiscoveredPattern, models map[string]*MLModel) (*shared.WorkflowPrediction, error) {
	prediction := &shared.WorkflowPrediction{
		RiskFactors:             make([]*workflowtypes.RiskFactor, 0),
		OptimizationSuggestions: make([]*sharedtypes.OptimizationSuggestion, 0),
	}

	// Use success prediction model if available
	if successModel, exists := models["success_prediction"]; exists {
		featureVec, err := (&FeatureExtractor{}).ConvertToVector(features)
		if err == nil && len(successModel.Weights) > 0 {
			// Simple logistic regression prediction
			z := successModel.Bias
			for i, weight := range successModel.Weights {
				if i < len(featureVec) {
					z += weight * featureVec[i]
				}
			}
			prediction.SuccessProbability = 1.0 / (1.0 + math.Exp(-z))
			prediction.Confidence = successModel.Accuracy
		}
	}

	// Use duration prediction model if available
	if durationModel, exists := models["duration_prediction"]; exists {
		featureVec, err := (&FeatureExtractor{}).ConvertToVector(features)
		if err == nil && len(durationModel.Weights) > 0 {
			// Linear regression prediction
			duration := durationModel.Bias
			for i, weight := range durationModel.Weights {
				if i < len(featureVec) {
					duration += weight * featureVec[i]
				}
			}
			if duration > 0 {
				prediction.ExpectedDuration = time.Duration(duration) * time.Second
			}
		}
	}

	// Analyze risk factors based on features
	prediction.RiskFactors = op.identifyRiskFactors(features)

	// Generate optimization suggestions based on patterns
	if len(patterns) > 0 {
		prediction.OptimizationSuggestions = op.generateOptimizationSuggestions(patterns)
	}

	// Set defaults if models didn't provide predictions
	if prediction.SuccessProbability == 0 {
		prediction.SuccessProbability = 0.5 // Neutral
		prediction.Confidence = 0.3         // Low confidence
		prediction.Reason = "No historical data available"
	}

	if prediction.ExpectedDuration == 0 {
		prediction.ExpectedDuration = 5 * time.Minute // Default estimate
	}

	return prediction, nil
}

func (op *OutcomePredictor) identifyRiskFactors(features *shared.WorkflowFeatures) []*workflowtypes.RiskFactor {
	risks := make([]*workflowtypes.RiskFactor, 0)

	// High cluster load risk
	if features.ClusterLoad > 0.8 {
		risks = append(risks, &workflowtypes.RiskFactor{
			Type:        "resource_contention",
			Description: "High cluster load may cause resource contention",
			Probability: features.ClusterLoad,
			Impact:      "medium",
			Mitigation:  "Consider scheduling during off-peak hours",
		})
	}

	// Weekend execution risk
	if features.IsWeekend {
		risks = append(risks, &workflowtypes.RiskFactor{
			Type:        "support_availability",
			Description: "Executing during weekend when support may be limited",
			Probability: 0.3,
			Impact:      "low",
			Mitigation:  "Ensure monitoring is active and escalation paths are clear",
		})
	}

	// High complexity risk
	if features.StepCount > 10 {
		risks = append(risks, &workflowtypes.RiskFactor{
			Type:        "workflow_complexity",
			Description: "High number of steps increases failure probability",
			Probability: float64(features.StepCount) / 20.0,
			Impact:      "medium",
			Mitigation:  "Consider breaking into smaller workflows",
		})
	}

	// Recent failures risk
	if features.RecentFailures > 2 {
		risks = append(risks, &workflowtypes.RiskFactor{
			Type:        "historical_failures",
			Description: "Recent failures indicate potential systemic issues",
			Probability: math.Min(float64(features.RecentFailures)/5.0, 0.9),
			Impact:      "high",
			Mitigation:  "Investigate root cause of recent failures before proceeding",
		})
	}

	return risks
}

func (op *OutcomePredictor) generateOptimizationSuggestions(patterns []*shared.DiscoveredPattern) []*sharedtypes.OptimizationSuggestion {
	suggestions := make([]*sharedtypes.OptimizationSuggestion, 0)

	for _, pattern := range patterns {
		// Create suggestions based on pattern metadata
		suggestion := &sharedtypes.OptimizationSuggestion{
			Type:                string(pattern.Type),
			Description:         pattern.Description,
			ExpectedImprovement: pattern.Confidence,
		}

		suggestions = append(suggestions, suggestion)
	}

	return suggestions
}
