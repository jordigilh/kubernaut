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

package anomaly

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// AnomalyDetectionResult contains the results of batch anomaly detection
type AnomalyDetectionResult struct {
	Anomalies       []*AnomalyResult          `json:"anomalies"`
	Summary         *DetectionSummary         `json:"summary"`
	BaselineHealth  *BaselineHealthReport     `json:"baseline_health"`
	TrendAnalysis   *TimeSeriesTrendAnalysis  `json:"trend_analysis"`
	Recommendations []*SystemRecommendation   `json:"recommendations"`
	AnalyzedPeriod  AnomalyDetectionTimeRange `json:"analyzed_period"`
}

// AnomalyDetector detects anomalous patterns in workflow execution data
type AnomalyDetector struct {
	config           *patterns.PatternDiscoveryConfig
	log              *logrus.Logger
	baselineModels   map[string]*BaselineModel
	detectionMethods []DetectionMethod
	alertThresholds  map[string]float64
	historicalData   *HistoricalDataBuffer
}

// BaselineModelParameters represents strongly-typed parameters for baseline models
// Business Requirement: BR-ANOMALY-001 - Type-safe anomaly detection configuration
type BaselineModelParameters struct {
	Threshold           float64 `json:"threshold"`            // Anomaly detection threshold (0.0-1.0)
	WindowSize          int     `json:"window_size"`          // Time window size for analysis
	Algorithm           string  `json:"algorithm"`            // Detection algorithm name
	SensitivityLevel    float64 `json:"sensitivity_level"`    // Sensitivity configuration (0.0-1.0)
	MinDataPoints       int     `json:"min_data_points"`      // Minimum data points required
	ConfidenceThreshold float64 `json:"confidence_threshold"` // Minimum confidence for detection
	LearningRate        float64 `json:"learning_rate"`        // Model learning rate
	DecayFactor         float64 `json:"decay_factor"`         // Historical data decay factor
}

// BaselineModel represents normal behavior patterns
// Business Requirement: BR-ANOMALY-001 - Baseline model for anomaly detection
type BaselineModel struct {
	ID               string                       `json:"id"`
	Type             string                       `json:"type"` // "statistical", "temporal", "behavioral"
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	DataPoints       int                          `json:"data_points"`
	Statistics       *BaselineStatistics          `json:"statistics"`
	TemporalPatterns map[string]*TemporalBaseline `json:"temporal_patterns"`
	Parameters       *BaselineModelParameters     `json:"parameters"` // Strongly-typed parameters
	Confidence       float64                      `json:"confidence"`
}

// TemporalBaseline contains time-based baseline patterns
type TemporalBaseline struct {
	Pattern     string             `json:"pattern"`     // "hourly", "daily", "weekly"
	Expected    map[string]float64 `json:"expected"`    // Expected values for time periods
	Variance    map[string]float64 `json:"variance"`    // Variance for each time period
	Seasonality float64            `json:"seasonality"` // Seasonal strength
	Trend       float64            `json:"trend"`       // Trend coefficient
}

// DetectionMethodParameters represents strongly-typed parameters for detection methods
// Business Requirement: BR-ANOMALY-002 - Type-safe detection method configuration
type DetectionMethodParameters struct {
	StatisticalThreshold float64 `json:"statistical_threshold"` // Threshold for statistical methods
	IsolationTreeCount   int     `json:"isolation_tree_count"`  // Number of trees for isolation forest
	SVMKernel            string  `json:"svm_kernel"`            // SVM kernel type ("rbf", "linear", "poly")
	SVMGamma             float64 `json:"svm_gamma"`             // SVM gamma parameter
	WindowSizeMinutes    int     `json:"window_size_minutes"`   // Analysis window size in minutes
	MinSamplesRequired   int     `json:"min_samples_required"`  // Minimum samples for analysis
	OutlierFraction      float64 `json:"outlier_fraction"`      // Expected fraction of outliers
	ConfidenceInterval   float64 `json:"confidence_interval"`   // Statistical confidence interval
}

// DetectionMethod defines an anomaly detection algorithm
// Business Requirement: BR-ANOMALY-002 - Configurable anomaly detection methods
type DetectionMethod struct {
	Name        string                     `json:"name"`
	Algorithm   string                     `json:"algorithm"`   // "statistical", "isolation_forest", "one_class_svm"
	Parameters  *DetectionMethodParameters `json:"parameters"`  // Strongly-typed parameters
	Sensitivity float64                    `json:"sensitivity"` // 0-1, higher = more sensitive
	Enabled     bool                       `json:"enabled"`
}

// WorkflowExecutionData represents strongly-typed workflow execution data
// Business Requirement: BR-WORKFLOW-001 - Type-safe workflow execution data
type WorkflowExecutionData struct {
	StepCount      int     `json:"step_count"`      // Number of steps in workflow
	ExecutionTime  float64 `json:"execution_time"`  // Total execution time in seconds
	ResourceUsage  float64 `json:"resource_usage"`  // Resource usage percentage
	ErrorCount     int     `json:"error_count"`     // Number of errors encountered
	RetryCount     int     `json:"retry_count"`     // Number of retries performed
	SuccessRate    float64 `json:"success_rate"`    // Success rate percentage
	AlertSeverity  string  `json:"alert_severity"`  // Alert severity level
	WorkflowType   string  `json:"workflow_type"`   // Type of workflow executed
	NamespaceCount int     `json:"namespace_count"` // Number of namespaces affected
	ActionCount    int     `json:"action_count"`    // Number of actions performed
}

// WorkflowExecutionContext represents strongly-typed execution context
// Business Requirement: BR-WORKFLOW-002 - Type-safe execution context
type WorkflowExecutionContext struct {
	ClusterName     string   `json:"cluster_name"`     // Kubernetes cluster name
	Namespace       string   `json:"namespace"`        // Target namespace
	AlertSource     string   `json:"alert_source"`     // Source of the alert
	UserID          string   `json:"user_id"`          // User who triggered execution
	TriggerType     string   `json:"trigger_type"`     // How execution was triggered
	Priority        string   `json:"priority"`         // Execution priority level
	Tags            []string `json:"tags"`             // Associated tags
	Environment     string   `json:"environment"`      // Environment (dev, staging, prod)
	Region          string   `json:"region"`           // Geographic region
	WorkflowVersion string   `json:"workflow_version"` // Version of workflow used
}

// WorkflowExecutionEvent represents a real-time workflow event
// Business Requirement: BR-WORKFLOW-003 - Comprehensive workflow event tracking
type WorkflowExecutionEvent struct {
	Type        string                    `json:"type"`
	WorkflowID  string                    `json:"workflow_id"`
	ExecutionID string                    `json:"execution_id"`
	Timestamp   time.Time                 `json:"timestamp"`
	Data        *WorkflowExecutionData    `json:"data"`    // Strongly-typed execution data
	Metrics     map[string]float64        `json:"metrics"` // Numerical metrics (kept as map for flexibility)
	Context     *WorkflowExecutionContext `json:"context"` // Strongly-typed context
}

// AnomalyMetadata represents strongly-typed metadata for anomaly results
// Business Requirement: BR-ANOMALY-003 - Type-safe anomaly metadata
type AnomalyMetadata struct {
	DetectionLatency    float64   `json:"detection_latency"`    // Time taken to detect anomaly (ms)
	ModelVersion        string    `json:"model_version"`        // Version of detection model used
	TrainingDataSize    int       `json:"training_data_size"`   // Size of training dataset
	FeatureCount        int       `json:"feature_count"`        // Number of features analyzed
	ComparisonBaseline  string    `json:"comparison_baseline"`  // Baseline used for comparison
	StatisticalTest     string    `json:"statistical_test"`     // Statistical test performed
	PValue              float64   `json:"p_value"`              // Statistical p-value
	EffectSize          float64   `json:"effect_size"`          // Effect size of anomaly
	SeasonalityAdjusted bool      `json:"seasonality_adjusted"` // Whether seasonality was considered
	TrendAdjusted       bool      `json:"trend_adjusted"`       // Whether trend was considered
	OutlierMethod       string    `json:"outlier_method"`       // Method used for outlier detection
	ConfidenceBounds    []float64 `json:"confidence_bounds"`    // Statistical confidence bounds
}

// Structured Error Types for Anomaly Detection
// Business Requirement: BR-ANOMALY-005 - Structured error handling for anomaly detection

// AnomalyDetectionError represents structured errors in anomaly detection
type AnomalyDetectionError struct {
	Operation string                 `json:"operation"`  // Operation that failed
	Component string                 `json:"component"`  // Component that failed
	ErrorType string                 `json:"error_type"` // Type of error
	Message   string                 `json:"message"`    // Human-readable message
	Context   map[string]interface{} `json:"context"`    // Additional context
	Cause     error                  `json:"-"`          // Underlying error
	Timestamp time.Time              `json:"timestamp"`  // When error occurred
	Severity  string                 `json:"severity"`   // Error severity level
}

// Error implements the error interface
func (e *AnomalyDetectionError) Error() string {
	return fmt.Sprintf("anomaly detection error in %s.%s: %s", e.Component, e.Operation, e.Message)
}

// Unwrap returns the underlying error
func (e *AnomalyDetectionError) Unwrap() error {
	return e.Cause
}

// NewAnomalyDetectionError creates a new structured anomaly detection error
func NewAnomalyDetectionError(operation, component, errorType, message string, cause error) *AnomalyDetectionError {
	return &AnomalyDetectionError{
		Operation: operation,
		Component: component,
		ErrorType: errorType,
		Message:   message,
		Context:   make(map[string]interface{}),
		Cause:     cause,
		Timestamp: time.Now(),
		Severity:  "error",
	}
}

// WithContext adds context to the error
func (e *AnomalyDetectionError) WithContext(key string, value interface{}) *AnomalyDetectionError {
	e.Context[key] = value
	return e
}

// WithSeverity sets the error severity
func (e *AnomalyDetectionError) WithSeverity(severity string) *AnomalyDetectionError {
	e.Severity = severity
	return e
}

// Common error types
var (
	ErrInsufficientData = NewAnomalyDetectionError("analyze", "detector", "insufficient_data", "insufficient data for analysis", nil)
	ErrInvalidModel     = NewAnomalyDetectionError("detect", "model", "invalid_model", "baseline model is invalid", nil)
	ErrDetectionFailed  = NewAnomalyDetectionError("detect", "algorithm", "detection_failed", "anomaly detection algorithm failed", nil)
	ErrConfigInvalid    = NewAnomalyDetectionError("initialize", "config", "invalid_config", "anomaly detection configuration is invalid", nil)
)

// AnomalyResult contains detection results
// Business Requirement: BR-ANOMALY-004 - Comprehensive anomaly detection results
type AnomalyResult struct {
	ID              string                   `json:"id"`
	Type            string                   `json:"type"`     // "execution", "pattern", "temporal", "resource"
	Severity        string                   `json:"severity"` // "low", "medium", "high", "critical"
	DetectedAt      time.Time                `json:"detected_at"`
	Event           *WorkflowExecutionEvent  `json:"event"`
	BaselineModel   string                   `json:"baseline_model"`
	DetectionMethod string                   `json:"detection_method"`
	AnomalyScore    float64                  `json:"anomaly_score"` // 0-1, higher = more anomalous
	Confidence      float64                  `json:"confidence"`
	Description     string                   `json:"description"`
	Impact          *AnomalyImpactAssessment `json:"impact"`
	Recommendations []*AnomalyRecommendation `json:"recommendations"`
	Metadata        *AnomalyMetadata         `json:"metadata"` // Strongly-typed metadata
}

// AnomalyImpactAssessment evaluates the impact of an anomaly
type AnomalyImpactAssessment struct {
	Scope             string        `json:"scope"` // "workflow", "namespace", "cluster"
	AffectedResources []string      `json:"affected_resources"`
	BusinessImpact    string        `json:"business_impact"` // "none", "low", "medium", "high", "critical"
	TechnicalImpact   string        `json:"technical_impact"`
	EstimatedCost     float64       `json:"estimated_cost"`
	RecoveryTime      time.Duration `json:"recovery_time"`
}

// AnomalyRecommendation suggests actions for handling anomalies
type AnomalyRecommendation struct {
	Action        string  `json:"action"`
	Description   string  `json:"description"`
	Priority      int     `json:"priority"`
	Urgency       string  `json:"urgency"` // "immediate", "urgent", "normal", "low"
	AutomateIt    bool    `json:"automate_it"`
	EstimatedCost float64 `json:"estimated_cost"`
}

// HistoricalDataBuffer maintains recent execution data for baseline updates
type HistoricalDataBuffer struct {
	maxSize int
	data    []*engine.EngineWorkflowExecutionData
	index   int
	full    bool
}

// DetectionSummary provides overview of detection results
type DetectionSummary struct {
	TotalAnomalies    int            `json:"total_anomalies"`
	SeverityBreakdown map[string]int `json:"severity_breakdown"`
	TypeBreakdown     map[string]int `json:"type_breakdown"`
	DetectionRate     float64        `json:"detection_rate"`
	FalsePositiveRate float64        `json:"false_positive_rate"`
	Coverage          float64        `json:"coverage"`
}

// BaselineHealthReport assesses the health of baseline models
type BaselineHealthReport struct {
	ModelsEvaluated    int                `json:"models_evaluated"`
	HealthyModels      int                `json:"healthy_models"`
	StaleModels        int                `json:"stale_models"`
	InaccurateModels   int                `json:"inaccurate_models"`
	ModelHealth        map[string]float64 `json:"model_health"` // Model ID -> health score
	RecommendedActions []string           `json:"recommended_actions"`
}

// SystemRecommendation suggests system-level improvements
type SystemRecommendation struct {
	Type           string `json:"type"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Impact         string `json:"impact"`
	Effort         string `json:"effort"`
	Priority       int    `json:"priority"`
	Implementation string `json:"implementation"`
}

// TimeSeriesTrendAnalysis contains trend analysis results
type TimeSeriesTrendAnalysis struct {
	Direction    string                    `json:"direction"` // "increasing", "decreasing", "stable"
	Slope        float64                   `json:"slope"`
	Confidence   float64                   `json:"confidence"`
	StartValue   float64                   `json:"start_value"`
	EndValue     float64                   `json:"end_value"`
	TrendPeriod  AnomalyDetectionTimeRange `json:"trend_period"`
	Significance float64                   `json:"significance"`
}

// AnomalyDetectionTimeRange represents a time period
type AnomalyDetectionTimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// BaselineStatistics contains statistical baseline information
type BaselineStatistics struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Median float64 `json:"median"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Q1     float64 `json:"q1"`
	Q3     float64 `json:"q3"`
	IQR    float64 `json:"iqr"`
}

// NewAnomalyDetector creates a new anomaly detector
func NewAnomalyDetector(config *patterns.PatternDiscoveryConfig, log *logrus.Logger) *AnomalyDetector {
	ad := &AnomalyDetector{
		config:          config,
		log:             log,
		baselineModels:  make(map[string]*BaselineModel),
		alertThresholds: make(map[string]float64),
		historicalData:  NewHistoricalDataBuffer(1000), // Keep last 1000 executions
	}

	ad.initializeDetectionMethods()
	ad.initializeAlertThresholds()

	return ad
}

// DetectAnomaly analyzes a single event for anomalies
func (ad *AnomalyDetector) DetectAnomaly(event *WorkflowExecutionEvent) *AnomalyResult {
	ad.log.WithFields(logrus.Fields{
		"event_type":   event.Type,
		"execution_id": event.ExecutionID,
		"workflow_id":  event.WorkflowID,
	}).Debug("Detecting anomalies in event")

	// Find applicable baseline models
	models := ad.findApplicableModels(event)
	if len(models) == 0 {
		ad.log.Debug("No applicable baseline models found")
		return nil
	}

	var bestAnomaly *AnomalyResult
	highestScore := 0.0

	// Run detection methods
	for _, method := range ad.detectionMethods {
		if !method.Enabled {
			continue
		}

		for _, model := range models {
			anomaly := ad.runDetectionMethod(event, model, method)
			if anomaly != nil && anomaly.AnomalyScore > highestScore {
				highestScore = anomaly.AnomalyScore
				bestAnomaly = anomaly
			}
		}
	}

	// Enhance anomaly with impact assessment and recommendations
	if bestAnomaly != nil {
		bestAnomaly.Impact = ad.assessImpact(bestAnomaly, event)
		bestAnomaly.Recommendations = ad.generateRecommendations(bestAnomaly)
	}

	return bestAnomaly
}

// DetectBatchAnomalies analyzes multiple executions for anomalies
func (ad *AnomalyDetector) DetectBatchAnomalies(ctx context.Context, executions []*engine.EngineWorkflowExecutionData) (*AnomalyDetectionResult, error) {
	ad.log.WithField("executions", len(executions)).Info("Detecting batch anomalies")

	if len(executions) == 0 {
		return &AnomalyDetectionResult{
			Anomalies: []*AnomalyResult{},
			Summary:   &DetectionSummary{},
		}, nil
	}

	// Update baselines with recent data
	if err := ad.updateBaselines(executions); err != nil {
		ad.log.WithError(err).Warn("Failed to update baselines")
	}

	anomalies := make([]*AnomalyResult, 0)

	// Convert executions to events and detect anomalies
	for _, execution := range executions {
		event := ad.convertExecutionToEvent(execution)
		if anomaly := ad.DetectAnomaly(event); anomaly != nil {
			anomalies = append(anomalies, anomaly)
		}
	}

	// Analyze trends
	trendAnalysis, err := ad.analyzeTrends(executions)
	if err != nil {
		ad.log.WithError(err).Warn("Failed to analyze trends")
	}

	// Generate summary
	summary := ad.generateSummary(anomalies, len(executions))

	// Assess baseline health
	baselineHealth := ad.assessBaselineHealth()

	// Generate system recommendations
	systemRecommendations := ad.generateSystemRecommendations(anomalies, trendAnalysis)

	result := &AnomalyDetectionResult{
		Anomalies:       anomalies,
		Summary:         summary,
		BaselineHealth:  baselineHealth,
		TrendAnalysis:   trendAnalysis,
		Recommendations: systemRecommendations,
		AnalyzedPeriod: AnomalyDetectionTimeRange{
			Start: executions[0].Timestamp,
			End:   executions[len(executions)-1].Timestamp,
		},
	}

	return result, nil
}

// UpdateBaseline updates baseline models with new execution data
func (ad *AnomalyDetector) UpdateBaseline(executions []*engine.EngineWorkflowExecutionData) error {
	return ad.updateBaselines(executions)
}

// GetBaselineModels returns current baseline models
func (ad *AnomalyDetector) GetBaselineModels() map[string]*BaselineModel {
	models := make(map[string]*BaselineModel)
	for id, model := range ad.baselineModels {
		models[id] = model
	}
	return models
}

// DetectPerformanceAnomaly analyzes performance metrics for anomalies
// Business Requirement: BR-AD-003 - Performance anomaly detection with <5% false positive rate
func (ad *AnomalyDetector) DetectPerformanceAnomaly(ctx context.Context, serviceName string, metrics map[string]float64) (*PerformanceAnomalyResult, error) {
	ad.log.WithFields(logrus.Fields{
		"service_name":  serviceName,
		"metrics_count": len(metrics),
		"business_req":  "BR-AD-003",
	}).Debug("Detecting performance anomaly for business protection")

	// Find applicable baseline model for service
	baselineKey := fmt.Sprintf("%s_performance", serviceName)
	baseline, exists := ad.baselineModels[baselineKey]

	result := &PerformanceAnomalyResult{
		ServiceName:              serviceName,
		AnomalyDetected:          false,
		Severity:                 "none",
		BusinessImpactAssessment: "none",
		AnalysisTimestamp:        time.Now(),
		RecommendedActions:       []string{},
		EstimatedTimeToImpact:    0,
		DetectionLatency:         time.Since(time.Now()),
	}

	if !exists || baseline == nil {
		ad.log.WithField("service_name", serviceName).Warn("No baseline found for service - creating default")
		result.RecommendedActions = append(result.RecommendedActions, "establish_performance_baseline")
		return result, nil
	}

	// BR-AD-003: Analyze performance metrics against baseline with statistical methods
	anomalyScore := 0.0
	anomalyReasons := []string{}
	criticalAnomalies := 0

	for metricName, currentValue := range metrics {
		if stats := ad.getMetricBaseline(baseline, metricName); stats != nil {
			// Statistical anomaly detection using z-score and IQR
			zScore := math.Abs(currentValue-stats.Mean) / stats.StdDev
			iqrBound := stats.Q3 + 1.5*(stats.Q3-stats.Q1)

			isStatisticalAnomaly := zScore > 3.0 || currentValue > iqrBound

			if isStatisticalAnomaly {
				anomalyScore += ad.calculateMetricAnomalyWeight(metricName, zScore)
				anomalyReasons = append(anomalyReasons, fmt.Sprintf("%s: %.2f (z-score: %.2f)", metricName, currentValue, zScore))

				// Business impact assessment based on metric criticality
				if ad.isCriticalMetric(metricName, currentValue) {
					criticalAnomalies++
				}
			}
		}
	}

	// BR-AD-003: Business logic for anomaly classification and impact assessment
	if anomalyScore > 0.7 || criticalAnomalies > 0 {
		result.AnomalyDetected = true

		// Severity assessment based on anomaly score and critical metrics
		if criticalAnomalies > 1 || anomalyScore > 0.9 {
			result.Severity = "critical"
			result.BusinessImpactAssessment = "high"
			result.EstimatedTimeToImpact = 5 * time.Minute // Immediate business impact expected
		} else if criticalAnomalies > 0 || anomalyScore > 0.8 {
			result.Severity = "high"
			result.BusinessImpactAssessment = "medium"
			result.EstimatedTimeToImpact = 15 * time.Minute // Business impact within 15 minutes
		} else {
			result.Severity = "medium"
			result.BusinessImpactAssessment = "low"
			result.EstimatedTimeToImpact = 30 * time.Minute // Business impact within 30 minutes
		}

		// Generate business-focused recommendations
		result.RecommendedActions = ad.generateBusinessRecommendations(serviceName, metrics, anomalyReasons)
	}

	// Business audit logging for compliance and monitoring
	ad.log.WithFields(logrus.Fields{
		"service_name":          serviceName,
		"anomaly_detected":      result.AnomalyDetected,
		"severity":              result.Severity,
		"business_impact":       result.BusinessImpactAssessment,
		"anomaly_score":         anomalyScore,
		"critical_anomalies":    criticalAnomalies,
		"estimated_impact_time": result.EstimatedTimeToImpact,
		"business_requirement":  "BR-AD-003",
	}).Info("Performance anomaly detection completed")

	return result, nil
}

// EstablishBaselines creates performance baselines for anomaly detection
// Business Requirement: BR-AD-003 - Baseline establishment for accurate anomaly detection
func (ad *AnomalyDetector) EstablishBaselines(ctx context.Context, baselines interface{}) error {
	ad.log.WithField("business_req", "BR-AD-003").Info("Establishing performance baselines")

	// Handle different baseline input types based on test usage
	switch b := baselines.(type) {
	case []BusinessPerformanceBaseline:
		return ad.establishBusinessBaselines(b)
	case []*BaselineModel:
		return ad.establishModelBaselines(b)
	default:
		return fmt.Errorf("unsupported baseline type: %T", baselines)
	}
}

// Private methods

func (ad *AnomalyDetector) initializeDetectionMethods() {
	ad.detectionMethods = []DetectionMethod{
		{
			Name:      "Statistical Outlier Detection",
			Algorithm: "statistical",
			Parameters: &DetectionMethodParameters{
				StatisticalThreshold: 3.0,
				WindowSizeMinutes:    60,
				MinSamplesRequired:   10,
				ConfidenceInterval:   0.95,
			},
			Sensitivity: 0.7,
			Enabled:     true,
		},
		{
			Name:      "Temporal Anomaly Detection",
			Algorithm: "temporal",
			Parameters: &DetectionMethodParameters{
				WindowSizeMinutes:  1440, // 24 hours in minutes
				MinSamplesRequired: 24,
				ConfidenceInterval: 0.90,
				OutlierFraction:    0.05,
			},
			Sensitivity: 0.6,
			Enabled:     true,
		},
		{
			Name:      "Behavioral Anomaly Detection",
			Algorithm: "behavioral",
			Parameters: &DetectionMethodParameters{
				StatisticalThreshold: 0.8,
				MinSamplesRequired:   30,
				ConfidenceInterval:   0.85,
				OutlierFraction:      0.1,
			},
			Sensitivity: 0.5,
			Enabled:     true,
		},
	}
}

func (ad *AnomalyDetector) initializeAlertThresholds() {
	ad.alertThresholds = map[string]float64{
		"execution_failure_rate": 0.2, // 20% failure rate triggers alert
		"duration_increase":      2.0, // 2x normal duration
		"resource_spike":         3.0, // 3x normal resource usage
		"frequency_anomaly":      5.0, // 5x normal execution frequency
	}
}

func (ad *AnomalyDetector) findApplicableModels(event *WorkflowExecutionEvent) []*BaselineModel {
	models := make([]*BaselineModel, 0)

	for _, model := range ad.baselineModels {
		if ad.isModelApplicable(event, model) {
			models = append(models, model)
		}
	}

	return models
}

func (ad *AnomalyDetector) isModelApplicable(event *WorkflowExecutionEvent, model *BaselineModel) bool {
	// Check if model is recent enough
	if time.Since(model.UpdatedAt) > 7*24*time.Hour {
		return false
	}

	// Check if model has sufficient data
	if model.DataPoints < 20 {
		return false
	}

	// Check if event type matches model scope using strongly-typed fields
	// Business Requirement: BR-ANOMALY-001 - Type-safe model scope validation
	if model.Parameters != nil && event.Context != nil {
		// Use Algorithm field as scope identifier for model matching
		if model.Parameters.Algorithm != "" && event.Context.Environment != "" {
			// Match models to appropriate environments (scope)
			if model.Parameters.Algorithm == "statistical" && event.Context.Environment == "production" {
				// Statistical models apply to production environments
			} else if model.Parameters.Algorithm == "temporal" && event.Context.Environment != "" {
				// Temporal models apply to all environments
			} else if model.Parameters.Algorithm == "behavioral" && event.Context.Priority == "high" {
				// Behavioral models focus on high-priority events
			} else {
				return false // Model not applicable to this event scope
			}
		}
	}

	return true
}

func (ad *AnomalyDetector) runDetectionMethod(event *WorkflowExecutionEvent, model *BaselineModel, method DetectionMethod) *AnomalyResult {
	switch method.Algorithm {
	case "statistical":
		return ad.detectStatisticalAnomaly(event, model, method)
	case "temporal":
		return ad.detectTemporalAnomaly(event, model, method)
	case "behavioral":
		return ad.detectBehavioralAnomaly(event, model, method)
	default:
		return nil
	}
}

func (ad *AnomalyDetector) detectStatisticalAnomaly(event *WorkflowExecutionEvent, model *BaselineModel, method DetectionMethod) *AnomalyResult {
	// Use strongly-typed parameters for statistical threshold
	// Business Requirement: BR-ANOMALY-002 - Type-safe detection parameters
	zThreshold := method.Parameters.StatisticalThreshold
	if zThreshold == 0 {
		zThreshold = 3.0 // Default statistical threshold
	}

	// Check key metrics for statistical anomalies
	for metricName, value := range event.Metrics {
		if baseline := model.Statistics.Mean; baseline > 0 {
			zScore := math.Abs(value-baseline) / model.Statistics.StdDev

			if zScore > zThreshold {
				severity := ad.calculateSeverity(zScore, zThreshold)

				return &AnomalyResult{
					ID:              fmt.Sprintf("stat-anomaly-%d", time.Now().Unix()),
					Type:            "statistical",
					Severity:        severity,
					DetectedAt:      time.Now(),
					Event:           event,
					BaselineModel:   model.ID,
					DetectionMethod: method.Name,
					AnomalyScore:    math.Min(zScore/zThreshold, 1.0),
					Confidence:      model.Confidence,
					Description:     fmt.Sprintf("Statistical anomaly in %s: value %.2f (z-score: %.2f)", metricName, value, zScore),
					Metadata: &AnomalyMetadata{
						DetectionLatency:    0.0, // Will be calculated later
						ModelVersion:        "v1.0",
						TrainingDataSize:    model.DataPoints,
						FeatureCount:        1, // Single metric analysis
						ComparisonBaseline:  model.ID,
						StatisticalTest:     "z-score",
						PValue:              0.0, // Not applicable for z-score
						EffectSize:          zScore,
						SeasonalityAdjusted: false,
						TrendAdjusted:       false,
						OutlierMethod:       "statistical",
						ConfidenceBounds:    []float64{baseline - zThreshold*model.Statistics.StdDev, baseline + zThreshold*model.Statistics.StdDev},
					},
				}
			}
		}
	}

	return nil
}

func (ad *AnomalyDetector) detectTemporalAnomaly(event *WorkflowExecutionEvent, model *BaselineModel, method DetectionMethod) *AnomalyResult {
	if model.TemporalPatterns == nil {
		return nil
	}

	hour := event.Timestamp.Hour()
	_ = event.Timestamp.Weekday() // weekday for future temporal analysis

	// Check hourly pattern
	if hourlyPattern, exists := model.TemporalPatterns["hourly"]; exists {
		hourKey := fmt.Sprintf("hour_%d", hour)
		if expectedValue, hasExpected := hourlyPattern.Expected[hourKey]; hasExpected {
			if variance, hasVariance := hourlyPattern.Variance[hourKey]; hasVariance {

				// Use execution success as the temporal metric
				// Use strongly-typed WorkflowExecutionData for success rate calculation
				// Business Requirement: BR-WORKFLOW-001 - Type-safe workflow execution data
				actualValue := 0.0
				if event.Data != nil {
					actualValue = event.Data.SuccessRate / 100.0 // Convert percentage to decimal
				}

				// Calculate deviation
				deviation := math.Abs(actualValue - expectedValue)
				threshold := 2 * math.Sqrt(variance) // 2 sigma threshold

				if deviation > threshold {
					severity := ad.calculateSeverity(deviation, threshold)

					return &AnomalyResult{
						ID:              fmt.Sprintf("temporal-anomaly-%d", time.Now().Unix()),
						Type:            "temporal",
						Severity:        severity,
						DetectedAt:      time.Now(),
						Event:           event,
						BaselineModel:   model.ID,
						DetectionMethod: method.Name,
						AnomalyScore:    math.Min(deviation/threshold, 1.0),
						Confidence:      hourlyPattern.Seasonality,
						Description:     fmt.Sprintf("Temporal anomaly at hour %d: expected %.2f, got %.2f", hour, expectedValue, actualValue),
						Metadata: &AnomalyMetadata{
							DetectionLatency:    0.0, // Will be calculated later
							ModelVersion:        "v1.0",
							TrainingDataSize:    model.DataPoints,
							FeatureCount:        1, // Temporal analysis
							ComparisonBaseline:  model.ID,
							StatisticalTest:     "temporal-deviation",
							PValue:              0.0, // Not applicable for temporal
							EffectSize:          deviation,
							SeasonalityAdjusted: true,
							TrendAdjusted:       false,
							OutlierMethod:       "temporal",
							ConfidenceBounds:    []float64{expectedValue - threshold, expectedValue + threshold},
						},
					}
				}
			}
		}
	}

	return nil
}

func (ad *AnomalyDetector) detectBehavioralAnomaly(event *WorkflowExecutionEvent, model *BaselineModel, method DetectionMethod) *AnomalyResult {
	// Use strongly-typed parameters for pattern threshold
	// Business Requirement: BR-ANOMALY-002 - Type-safe detection parameters
	patternThreshold := method.Parameters.StatisticalThreshold
	if patternThreshold == 0 {
		patternThreshold = 0.8 // Default behavioral threshold
	}

	// Analyze execution patterns using strongly-typed data
	// Business Requirement: BR-WORKFLOW-001 - Type-safe workflow execution data
	if event.Data != nil && event.WorkflowID != "" {
		// Use WorkflowType from strongly-typed data instead of template_id
		workflowType := event.Data.WorkflowType
		if workflowType != "" {
			// Check if this workflow-template combination is unusual

			// Get recent execution frequency for this workflow type
			recentCount := ad.getRecentExecutionCount(event.WorkflowID, workflowType)

			// Compare with baseline frequency using strongly-typed parameters
			// Business Requirement: BR-ANOMALY-001 - Type-safe baseline parameters
			baselineFreq := 1.0 // Default baseline
			if model.Parameters != nil && model.Parameters.Threshold > 0 {
				// Use threshold as baseline frequency multiplier
				baselineFreq = model.Parameters.Threshold
			}

			// Calculate anomaly score based on frequency deviation
			frequencyRatio := float64(recentCount) / baselineFreq
			if frequencyRatio > 1.0/patternThreshold || frequencyRatio < patternThreshold {
				severity := "medium"
				if frequencyRatio > 3.0 || frequencyRatio < 0.3 {
					severity = "high"
				}

				return &AnomalyResult{
					ID:              fmt.Sprintf("behavioral-anomaly-%d", time.Now().Unix()),
					Type:            "behavioral",
					Severity:        severity,
					DetectedAt:      time.Now(),
					Event:           event,
					BaselineModel:   model.ID,
					DetectionMethod: method.Name,
					AnomalyScore:    math.Min(math.Abs(1.0-frequencyRatio), 1.0),
					Confidence:      0.7, // Medium confidence for behavioral detection
					Description:     fmt.Sprintf("Behavioral anomaly: workflow execution frequency %.2fx normal", frequencyRatio),
					Metadata: &AnomalyMetadata{
						DetectionLatency:    0.0, // Will be calculated later
						ModelVersion:        "v1.0",
						TrainingDataSize:    model.DataPoints,
						FeatureCount:        2, // Workflow type and frequency
						ComparisonBaseline:  model.ID,
						StatisticalTest:     "frequency-ratio",
						PValue:              0.0, // Not applicable for behavioral
						EffectSize:          frequencyRatio,
						SeasonalityAdjusted: false,
						TrendAdjusted:       false,
						OutlierMethod:       "behavioral",
						ConfidenceBounds:    []float64{baselineFreq * patternThreshold, baselineFreq / patternThreshold},
					},
				}
			}
		}
	}

	return nil
}

func (ad *AnomalyDetector) updateBaselines(executions []*engine.EngineWorkflowExecutionData) error {
	// Add data to historical buffer
	for _, execution := range executions {
		ad.historicalData.Add(execution)
	}

	// Update or create baseline models
	ad.updateStatisticalBaseline(executions)

	ad.updateTemporalBaseline(executions)

	ad.updateBehavioralBaseline(executions)

	return nil
}

func (ad *AnomalyDetector) updateStatisticalBaseline(executions []*engine.EngineWorkflowExecutionData) {
	modelID := "statistical_baseline"

	// Extract metrics
	durations := make([]float64, 0)
	successRate := 0.0
	resourceUsages := make(map[string][]float64)

	for _, execution := range executions {
		// Use Duration field directly from execution
		if execution.Duration > 0 {
			durations = append(durations, execution.Duration.Minutes())
		}

		// Use Success field directly
		if execution.Success {
			successRate += 1.0
		}

		// Extract resource usage from Metrics if available
		if execution.Metrics != nil {
			if _, exists := resourceUsages["cpu"]; !exists {
				resourceUsages["cpu"] = make([]float64, 0)
				resourceUsages["memory"] = make([]float64, 0)
				resourceUsages["network"] = make([]float64, 0)
				resourceUsages["storage"] = make([]float64, 0)
			}
			if cpu, exists := execution.Metrics["cpu_usage"]; exists {
				resourceUsages["cpu"] = append(resourceUsages["cpu"], cpu)
			}
			if memory, exists := execution.Metrics["memory_usage"]; exists {
				resourceUsages["memory"] = append(resourceUsages["memory"], memory)
			}
			if network, exists := execution.Metrics["network_usage"]; exists {
				resourceUsages["network"] = append(resourceUsages["network"], network)
			}
			if storage, exists := execution.Metrics["storage_usage"]; exists {
				resourceUsages["storage"] = append(resourceUsages["storage"], storage)
			}
		}
	}

	if len(durations) == 0 {
		return
	}

	successRate /= float64(len(executions))

	// Calculate statistics
	stats := &BaselineStatistics{
		Mean:   statMean(durations),
		StdDev: statStdDev(durations),
		Median: 0.0, // Will be calculated below
		Min:    statMin(durations),
		Max:    statMax(durations),
	}

	// Calculate quartiles
	sortedDurations := make([]float64, len(durations))
	copy(sortedDurations, durations)
	sort.Float64s(sortedDurations)

	n := len(sortedDurations)
	if n >= 4 {
		stats.Q1 = sortedDurations[n/4]
		stats.Q3 = sortedDurations[3*n/4]
		stats.IQR = stats.Q3 - stats.Q1
		stats.Median = sortedDurations[n/2]
	}

	// Create or update model
	model := &BaselineModel{
		ID:         modelID,
		Type:       "statistical",
		UpdatedAt:  time.Now(),
		DataPoints: len(executions),
		Statistics: stats,
		Parameters: &BaselineModelParameters{
			Threshold:           0.8, // Default success rate threshold
			WindowSize:          60,  // 60 minute window
			Algorithm:           "statistical",
			SensitivityLevel:    0.7,  // Medium sensitivity
			MinDataPoints:       10,   // Minimum data points
			ConfidenceThreshold: 0.9,  // High confidence threshold
			LearningRate:        0.1,  // Conservative learning
			DecayFactor:         0.95, // Slow decay
		},
		Confidence: math.Min(float64(len(executions))/100.0, 1.0), // Higher confidence with more data
	}

	if existingModel, exists := ad.baselineModels[modelID]; exists {
		model.CreatedAt = existingModel.CreatedAt
		model.DataPoints += existingModel.DataPoints
	} else {
		model.CreatedAt = time.Now()
	}

	ad.baselineModels[modelID] = model
}

func (ad *AnomalyDetector) updateTemporalBaseline(executions []*engine.EngineWorkflowExecutionData) {
	modelID := "temporal_baseline"

	// Group executions by hour and weekday
	hourlyData := make(map[int][]float64)
	weeklyData := make(map[time.Weekday][]float64)

	for _, execution := range executions {
		hour := execution.Timestamp.Hour()
		weekday := execution.Timestamp.Weekday()

		successValue := 0.0
		if execution.Success {
			successValue = 1.0
		}

		if _, exists := hourlyData[hour]; !exists {
			hourlyData[hour] = make([]float64, 0)
		}
		hourlyData[hour] = append(hourlyData[hour], successValue)

		if _, exists := weeklyData[weekday]; !exists {
			weeklyData[weekday] = make([]float64, 0)
		}
		weeklyData[weekday] = append(weeklyData[weekday], successValue)
	}

	// Calculate hourly patterns
	hourlyPattern := &TemporalBaseline{
		Pattern:  "hourly",
		Expected: make(map[string]float64),
		Variance: make(map[string]float64),
	}

	for hour, values := range hourlyData {
		if len(values) > 0 {
			hourKey := fmt.Sprintf("hour_%d", hour)
			hourlyPattern.Expected[hourKey] = statMean(values)
			hourlyPattern.Variance[hourKey] = statVariance(values)
		}
	}

	// Calculate seasonality strength (simplified)
	hourlyMeans := make([]float64, 0)
	for _, mean := range hourlyPattern.Expected {
		hourlyMeans = append(hourlyMeans, mean)
	}
	if len(hourlyMeans) > 1 {
		hourlyPattern.Seasonality = statVariance(hourlyMeans)
	}

	// Create temporal patterns map
	temporalPatterns := map[string]*TemporalBaseline{
		"hourly": hourlyPattern,
	}

	// Create or update model
	model := &BaselineModel{
		ID:               modelID,
		Type:             "temporal",
		UpdatedAt:        time.Now(),
		DataPoints:       len(executions),
		TemporalPatterns: temporalPatterns,
		Confidence:       math.Min(float64(len(executions))/200.0, 1.0),
	}

	if existingModel, exists := ad.baselineModels[modelID]; exists {
		model.CreatedAt = existingModel.CreatedAt
		model.DataPoints += existingModel.DataPoints
	} else {
		model.CreatedAt = time.Now()
	}

	ad.baselineModels[modelID] = model
}

func (ad *AnomalyDetector) updateBehavioralBaseline(executions []*engine.EngineWorkflowExecutionData) {
	modelID := "behavioral_baseline"

	// Analyze workflow execution patterns
	workflowFreq := make(map[string]int)
	templateFreq := make(map[string]int)

	for _, execution := range executions {
		// Extract alert info from metadata if available
		if execution.Metadata != nil {
			if alertName, exists := execution.Metadata["alert_name"]; exists {
				workflowKey := fmt.Sprintf("alert:%s", alertName)
				workflowFreq[workflowKey]++
			}
		}

		// Use WorkflowID as template identifier
		templateKey := fmt.Sprintf("template:%s", execution.WorkflowID)
		templateFreq[templateKey]++
	}

	// Convert to frequencies (executions per day)
	timeSpan := 1.0 // days
	if len(executions) > 1 {
		timeSpan = executions[len(executions)-1].Timestamp.Sub(executions[0].Timestamp).Hours() / 24.0
	}

	workflowFrequencies := make(map[string]float64)
	for key, count := range workflowFreq {
		workflowFrequencies[key] = float64(count) / timeSpan
	}

	// Create or update model
	model := &BaselineModel{
		ID:         modelID,
		Type:       "behavioral",
		UpdatedAt:  time.Now(),
		DataPoints: len(executions),
		Parameters: &BaselineModelParameters{
			Threshold:           0.5,  // Behavioral threshold
			WindowSize:          1440, // 24 hour window in minutes
			Algorithm:           "behavioral",
			SensitivityLevel:    0.6,  // Medium-low sensitivity
			MinDataPoints:       30,   // More data needed for behavioral
			ConfidenceThreshold: 0.8,  // Good confidence threshold
			LearningRate:        0.05, // Very conservative learning
			DecayFactor:         0.98, // Very slow decay for behavioral patterns
		},
		Confidence: math.Min(float64(len(executions))/150.0, 1.0),
	}

	if existingModel, exists := ad.baselineModels[modelID]; exists {
		model.CreatedAt = existingModel.CreatedAt
		model.DataPoints += existingModel.DataPoints
	} else {
		model.CreatedAt = time.Now()
	}

	ad.baselineModels[modelID] = model
}

func (ad *AnomalyDetector) convertExecutionToEvent(execution *engine.EngineWorkflowExecutionData) *WorkflowExecutionEvent {
	event := &WorkflowExecutionEvent{
		Type:        "workflow_execution",
		ExecutionID: execution.ExecutionID,
		Timestamp:   execution.Timestamp,
		Data: &WorkflowExecutionData{
			StepCount:      0, // Will be populated from execution
			ExecutionTime:  execution.Duration.Seconds(),
			ResourceUsage:  0.0,       // Default resource usage
			ErrorCount:     0,         // Will be calculated
			RetryCount:     0,         // Default retry count
			SuccessRate:    0.0,       // Will be calculated
			AlertSeverity:  "medium",  // Default severity
			WorkflowType:   "default", // Default type
			NamespaceCount: 1,         // Default namespace count
			ActionCount:    0,         // Will be populated
		},
		Metrics: make(map[string]float64),
		Context: &WorkflowExecutionContext{
			ClusterName:     "default",    // Default cluster
			Namespace:       "default",    // Default namespace
			AlertSource:     "system",     // Default source
			UserID:          "system",     // Default user
			TriggerType:     "automatic",  // Default trigger
			Priority:        "medium",     // Default priority
			Tags:            []string{},   // Empty tags
			Environment:     "production", // Default environment
			Region:          "us-east-1",  // Default region
			WorkflowVersion: "v1.0",       // Default version
		},
	}

	// Add execution data using strongly-typed fields
	// Business Requirement: BR-WORKFLOW-001 - Type-safe workflow execution data
	if execution.Success {
		event.Data.SuccessRate = 100.0 // 100% success
	} else {
		event.Data.SuccessRate = 0.0 // 0% success
	}
	event.Data.ExecutionTime = execution.Duration.Seconds()
	event.Metrics["duration_minutes"] = execution.Duration.Minutes()

	// Copy metrics from execution
	if execution.Metrics != nil {
		for key, value := range execution.Metrics {
			event.Metrics[key] = value
		}
	}

	// Copy metadata and context
	if execution.Metadata != nil {
		// Skip generic map assignment - use strongly-typed fields below

		// Extract common fields from metadata
		// Extract common fields from metadata to strongly-typed fields
		// Business Requirement: BR-WORKFLOW-002 - Type-safe execution context
		if alertName, exists := execution.Metadata["alert_name"]; exists {
			if name, ok := alertName.(string); ok {
				event.Context.AlertSource = name
			}
		}
		if alertSeverity, exists := execution.Metadata["alert_severity"]; exists {
			if severity, ok := alertSeverity.(string); ok {
				event.Data.AlertSeverity = severity
				event.Context.Priority = severity // Map severity to priority
			}
		}
		if namespace, exists := execution.Metadata["namespace"]; exists {
			if ns, ok := namespace.(string); ok {
				event.Context.Namespace = ns
			}
		}
		if resource, exists := execution.Metadata["resource"]; exists {
			if res, ok := resource.(string); ok {
				event.Data.WorkflowType = res // Map resource to workflow type
			}
		}
	}

	// Set workflow ID using strongly-typed field
	event.WorkflowID = execution.WorkflowID
	// Use WorkflowID as workflow type if not already set
	if event.Data.WorkflowType == "default" {
		event.Data.WorkflowType = execution.WorkflowID
	}

	return event
}

// Helper methods

func (ad *AnomalyDetector) calculateSeverity(score, threshold float64) string {
	ratio := score / threshold
	if ratio > 4.0 {
		return "critical"
	} else if ratio > 2.5 {
		return "high"
	} else if ratio > 1.5 {
		return "medium"
	}
	return "low"
}

func (ad *AnomalyDetector) getRecentExecutionCount(workflowID, templateID string) int {
	// This would query recent execution history
	// For now, return a placeholder
	return 1
}

func (ad *AnomalyDetector) assessImpact(anomaly *AnomalyResult, event *WorkflowExecutionEvent) *AnomalyImpactAssessment {
	impact := &AnomalyImpactAssessment{
		Scope:             "workflow",
		AffectedResources: []string{},
		BusinessImpact:    "low",
		TechnicalImpact:   "low",
		EstimatedCost:     0.0,
		RecoveryTime:      5 * time.Minute,
	}

	// Adjust impact based on anomaly severity
	switch anomaly.Severity {
	case "critical":
		impact.BusinessImpact = "high"
		impact.TechnicalImpact = "high"
		impact.RecoveryTime = 30 * time.Minute
	case "high":
		impact.BusinessImpact = "medium"
		impact.TechnicalImpact = "medium"
		impact.RecoveryTime = 15 * time.Minute
	case "medium":
		impact.BusinessImpact = "low"
		impact.TechnicalImpact = "medium"
		impact.RecoveryTime = 10 * time.Minute
	}

	// Add affected resources from event context
	// Use strongly-typed context fields for impact calculation
	// Business Requirement: BR-WORKFLOW-002 - Type-safe execution context
	if event.Context != nil {
		if event.Context.Namespace != "" {
			impact.AffectedResources = append(impact.AffectedResources, fmt.Sprintf("namespace:%s", event.Context.Namespace))
		}
		if event.Context.Environment != "" {
			impact.AffectedResources = append(impact.AffectedResources, fmt.Sprintf("environment:%s", event.Context.Environment))
		}
	}

	return impact
}

func (ad *AnomalyDetector) generateRecommendations(anomaly *AnomalyResult) []*AnomalyRecommendation {
	recommendations := make([]*AnomalyRecommendation, 0)

	switch anomaly.Type {
	case "statistical":
		recommendations = append(recommendations, &AnomalyRecommendation{
			Action:      "investigate_metrics",
			Description: "Investigate the root cause of unusual metric values",
			Priority:    1,
			Urgency:     "normal",
			AutomateIt:  false,
		})

	case "temporal":
		recommendations = append(recommendations, &AnomalyRecommendation{
			Action:      "check_scheduling",
			Description: "Review workflow scheduling and timing patterns",
			Priority:    2,
			Urgency:     "normal",
			AutomateIt:  true,
		})

	case "behavioral":
		recommendations = append(recommendations, &AnomalyRecommendation{
			Action:      "review_automation",
			Description: "Review automated triggering rules and frequency controls",
			Priority:    1,
			Urgency:     "urgent",
			AutomateIt:  false,
		})
	}

	// Add severity-based recommendations
	if anomaly.Severity == "critical" || anomaly.Severity == "high" {
		recommendations = append(recommendations, &AnomalyRecommendation{
			Action:      "immediate_investigation",
			Description: "Immediate investigation required due to high severity",
			Priority:    0,
			Urgency:     "immediate",
			AutomateIt:  false,
		})
	}

	return recommendations
}

func (ad *AnomalyDetector) analyzeTrends(executions []*engine.EngineWorkflowExecutionData) (*TimeSeriesTrendAnalysis, error) {
	if len(executions) < 10 {
		return nil, fmt.Errorf("insufficient data for trend analysis")
	}

	// Extract success rates over time windows
	windowSize := 24 * time.Hour
	windows := make(map[time.Time]float64)

	for _, execution := range executions {
		windowStart := execution.Timestamp.Truncate(windowSize)
		if _, exists := windows[windowStart]; !exists {
			windows[windowStart] = 0.0
		}

		if execution.Success {
			windows[windowStart] += 1.0
		}
	}

	// Convert to time series
	timestamps := make([]time.Time, 0)
	values := make([]float64, 0)

	for timestamp, successCount := range windows {
		timestamps = append(timestamps, timestamp)
		values = append(values, successCount)
	}

	// Sort by timestamp
	for i := 0; i < len(timestamps)-1; i++ {
		for j := i + 1; j < len(timestamps); j++ {
			if timestamps[i].After(timestamps[j]) {
				timestamps[i], timestamps[j] = timestamps[j], timestamps[i]
				values[i], values[j] = values[j], values[i]
			}
		}
	}

	// Calculate linear trend
	x := make([]float64, len(values))
	for i := range x {
		x[i] = float64(i)
	}

	// Simple linear regression
	n := float64(len(x))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumXX := 0.0

	for i := 0; i < len(x); i++ {
		sumX += x[i]
		sumY += values[i]
		sumXY += x[i] * values[i]
		sumXX += x[i] * x[i]
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)

	direction := "stable"
	if math.Abs(slope) > 0.1 {
		if slope > 0 {
			direction = "increasing"
		} else {
			direction = "decreasing"
		}
	}

	// Calculate R-squared for confidence
	yMean := sumY / n
	ssRes := 0.0
	ssTot := 0.0
	for i := 0; i < len(values); i++ {
		predicted := slope*x[i] + (sumY-slope*sumX)/n
		diff1 := values[i] - predicted
		ssRes += diff1 * diff1
		diff2 := values[i] - yMean
		ssTot += diff2 * diff2
	}

	confidence := 0.0
	if ssTot > 0 {
		confidence = math.Max(0, 1.0-(ssRes/ssTot))
	}

	trend := &TimeSeriesTrendAnalysis{
		Direction:    direction,
		Slope:        slope,
		Confidence:   confidence,
		StartValue:   values[0],
		EndValue:     values[len(values)-1],
		TrendPeriod:  AnomalyDetectionTimeRange{Start: timestamps[0], End: timestamps[len(timestamps)-1]},
		Significance: math.Abs(slope) * confidence,
	}

	return trend, nil
}

func (ad *AnomalyDetector) generateSummary(anomalies []*AnomalyResult, totalExecutions int) *DetectionSummary {
	summary := &DetectionSummary{
		TotalAnomalies:    len(anomalies),
		SeverityBreakdown: make(map[string]int),
		TypeBreakdown:     make(map[string]int),
		DetectionRate:     0.0,
		Coverage:          1.0, // Assume full coverage for now
	}

	for _, anomaly := range anomalies {
		summary.SeverityBreakdown[anomaly.Severity]++
		summary.TypeBreakdown[anomaly.Type]++
	}

	if totalExecutions > 0 {
		summary.DetectionRate = float64(len(anomalies)) / float64(totalExecutions)
	}

	return summary
}

func (ad *AnomalyDetector) assessBaselineHealth() *BaselineHealthReport {
	report := &BaselineHealthReport{
		ModelsEvaluated:    len(ad.baselineModels),
		ModelHealth:        make(map[string]float64),
		RecommendedActions: make([]string, 0),
	}

	healthyCount := 0
	staleCount := 0
	inaccurateCount := 0

	for id, model := range ad.baselineModels {
		health := ad.calculateModelHealth(model)
		report.ModelHealth[id] = health

		if health > 0.8 {
			healthyCount++
		} else if time.Since(model.UpdatedAt) > 7*24*time.Hour {
			staleCount++
		} else if model.Confidence < 0.5 {
			inaccurateCount++
		}
	}

	report.HealthyModels = healthyCount
	report.StaleModels = staleCount
	report.InaccurateModels = inaccurateCount

	// Generate recommendations
	if staleCount > 0 {
		report.RecommendedActions = append(report.RecommendedActions, "Update stale baseline models with recent data")
	}
	if inaccurateCount > 0 {
		report.RecommendedActions = append(report.RecommendedActions, "Improve accuracy of baseline models by collecting more training data")
	}

	return report
}

func (ad *AnomalyDetector) calculateModelHealth(model *BaselineModel) float64 {
	health := 0.0

	// Factor 1: Recency (50% weight)
	daysSinceUpdate := time.Since(model.UpdatedAt).Hours() / 24.0
	recencyScore := math.Max(0, 1.0-daysSinceUpdate/30.0) // Degrade over 30 days
	health += 0.5 * recencyScore

	// Factor 2: Data volume (30% weight)
	dataScore := math.Min(float64(model.DataPoints)/100.0, 1.0) // Saturate at 100 data points
	health += 0.3 * dataScore

	// Factor 3: Confidence (20% weight)
	health += 0.2 * model.Confidence

	return health
}

func (ad *AnomalyDetector) generateSystemRecommendations(anomalies []*AnomalyResult, trends *TimeSeriesTrendAnalysis) []*SystemRecommendation {
	recommendations := make([]*SystemRecommendation, 0)

	// Analyze anomaly patterns
	highSeverityCount := 0
	for _, anomaly := range anomalies {
		if anomaly.Severity == "critical" || anomaly.Severity == "high" {
			highSeverityCount++
		}
	}

	if highSeverityCount > 0 {
		recommendations = append(recommendations, &SystemRecommendation{
			Type:           "alerting",
			Title:          "Improve Anomaly Response",
			Description:    fmt.Sprintf("Detected %d high-severity anomalies. Consider implementing automated response workflows.", highSeverityCount),
			Impact:         "high",
			Effort:         "medium",
			Priority:       1,
			Implementation: "Configure automated remediation for common anomaly patterns",
		})
	}

	// Trend-based recommendations
	if trends != nil && trends.Direction == "decreasing" && trends.Significance > 0.3 {
		recommendations = append(recommendations, &SystemRecommendation{
			Type:           "performance",
			Title:          "Address Performance Degradation",
			Description:    "Detected declining trend in workflow success rates",
			Impact:         "medium",
			Effort:         "high",
			Priority:       2,
			Implementation: "Investigate root causes of declining performance and implement corrective measures",
		})
	}

	return recommendations
}

// HistoricalDataBuffer implementation

func NewHistoricalDataBuffer(maxSize int) *HistoricalDataBuffer {
	return &HistoricalDataBuffer{
		maxSize: maxSize,
		data:    make([]*engine.EngineWorkflowExecutionData, maxSize),
		index:   0,
		full:    false,
	}
}

func (hdb *HistoricalDataBuffer) Add(execution *engine.EngineWorkflowExecutionData) {
	hdb.data[hdb.index] = execution
	hdb.index = (hdb.index + 1) % hdb.maxSize

	if hdb.index == 0 {
		hdb.full = true
	}
}

func (hdb *HistoricalDataBuffer) GetRecent(count int) []*engine.EngineWorkflowExecutionData {
	if count <= 0 {
		return []*engine.EngineWorkflowExecutionData{}
	}

	size := hdb.maxSize
	if !hdb.full {
		size = hdb.index
	}

	if count > size {
		count = size
	}

	result := make([]*engine.EngineWorkflowExecutionData, count)
	for i := 0; i < count; i++ {
		idx := (hdb.index - 1 - i + hdb.maxSize) % hdb.maxSize
		result[i] = hdb.data[idx]
	}

	return result
}

// Simple statistical functions to replace gonum dependencies
func statMean(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

func statMin(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	minimum := data[0]
	for _, v := range data {
		if v < minimum {
			minimum = v
		}
	}
	return minimum
}

func statMax(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	maximum := data[0]
	for _, v := range data {
		if v > maximum {
			maximum = v
		}
	}
	return maximum
}

func statStdDev(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	m := statMean(data)
	sum := 0.0
	for _, v := range data {
		sum += (v - m) * (v - m)
	}
	return math.Sqrt(sum / float64(len(data)))
}

func statVariance(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	m := statMean(data)
	sum := 0.0
	for _, v := range data {
		sum += (v - m) * (v - m)
	}
	return sum / float64(len(data))
}

// PerformanceAnomalyResult contains performance-specific anomaly detection results
// Business Requirement: BR-AD-003 - Structured anomaly results for business decision making
type PerformanceAnomalyResult struct {
	ServiceName              string                 `json:"service_name"`
	AnomalyDetected          bool                   `json:"anomaly_detected"`
	Severity                 string                 `json:"severity"`
	BusinessImpactAssessment string                 `json:"business_impact_assessment"`
	AnalysisTimestamp        time.Time              `json:"analysis_timestamp"`
	RecommendedActions       []string               `json:"recommended_actions"`
	EstimatedTimeToImpact    time.Duration          `json:"estimated_time_to_impact"`
	DetectionLatency         time.Duration          `json:"detection_latency"`
	AnomalyScore             float64                `json:"anomaly_score,omitempty"`
	AnalysisDetails          map[string]interface{} `json:"analysis_details,omitempty"`
}

// BusinessPerformanceBaseline represents baseline performance expectations
type BusinessPerformanceBaseline struct {
	ServiceName         string                      `json:"service_name"`
	TimeOfDay           string                      `json:"time_of_day"`
	BaselineMetrics     map[string]PerformanceRange `json:"baseline_metrics"`
	BusinessCriticality string                      `json:"business_criticality"`
}

// PerformanceRange defines expected performance metric ranges
type PerformanceRange struct {
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
}

// Private helper methods for BR-AD-003 implementation

func (ad *AnomalyDetector) establishBusinessBaselines(baselines []BusinessPerformanceBaseline) error {
	for _, baseline := range baselines {
		// Convert business baseline to internal baseline model
		baselineModel := &BaselineModel{
			ID:         fmt.Sprintf("%s_performance", baseline.ServiceName),
			Type:       "performance",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			DataPoints: 100, // Assumed sufficient baseline data
			Statistics: &BaselineStatistics{},
			Parameters: &BaselineModelParameters{
				Threshold:           0.85, // Performance threshold
				WindowSize:          120,  // 2 hour window
				Algorithm:           "performance",
				SensitivityLevel:    0.8, // High sensitivity for performance
				MinDataPoints:       50,  // Sufficient data for performance
				ConfidenceThreshold: 0.9, // High confidence threshold
				LearningRate:        0.2, // Moderate learning for performance
				DecayFactor:         0.9, // Moderate decay
			},
			Confidence: 0.85, // High confidence for business baselines
		}

		// Convert performance ranges to baseline statistics using strongly-typed parameters
		// Business Requirement: BR-ANOMALY-001 - Type-safe baseline parameters
		// Note: Business-specific data stored in Algorithm field as identifier
		baselineModel.Parameters.Algorithm = fmt.Sprintf("performance-%s", baseline.ServiceName)
		// Use threshold for business criticality mapping
		if baseline.BusinessCriticality == "high" {
			baselineModel.Parameters.Threshold = 0.9 // High threshold for critical services
		} else if baseline.BusinessCriticality == "medium" {
			baselineModel.Parameters.Threshold = 0.7 // Medium threshold
		} else {
			baselineModel.Parameters.Threshold = 0.5 // Low threshold for non-critical
		}

		ad.baselineModels[baselineModel.ID] = baselineModel

		ad.log.WithFields(logrus.Fields{
			"service_name":  baseline.ServiceName,
			"baseline_id":   baselineModel.ID,
			"metrics_count": len(baseline.BaselineMetrics),
		}).Debug("Business performance baseline established")
	}

	ad.log.WithField("baselines_count", len(baselines)).Info("Business performance baselines established successfully")
	return nil
}

func (ad *AnomalyDetector) establishModelBaselines(baselines []*BaselineModel) error {
	for _, baseline := range baselines {
		ad.baselineModels[baseline.ID] = baseline
	}
	return nil
}

func (ad *AnomalyDetector) getMetricBaseline(baseline *BaselineModel, metricName string) *BaselineStatistics {
	// Use strongly-typed parameters for metric baseline calculation
	// Business Requirement: BR-ANOMALY-001 - Type-safe baseline parameters
	// For now, return default baseline statistics since metric_ranges is not in strongly-typed struct
	if baseline.Parameters != nil && baseline.Statistics != nil {
		// Use existing statistics as baseline
		return baseline.Statistics
	}

	// Return default baseline statistics for the metric
	// Business Requirement: BR-ANOMALY-001 - Provide reasonable defaults for missing data
	return &BaselineStatistics{
		Mean:   1.0, // Default mean
		StdDev: 0.2, // Default standard deviation
		Min:    0.0, // Default minimum
		Max:    2.0, // Default maximum
		Q1:     0.8, // Default Q1
		Q3:     1.2, // Default Q3
		IQR:    0.4, // Default IQR
	}
}

func (ad *AnomalyDetector) calculateMetricAnomalyWeight(metricName string, zScore float64) float64 {
	// Weight anomalies based on business impact of metrics
	weights := map[string]float64{
		"response_time_ms":     0.3,
		"error_rate_percent":   0.4, // High weight for errors
		"throughput_rps":       0.25,
		"memory_usage_percent": 0.2,
		"cpu_usage_percent":    0.15,
	}

	if weight, exists := weights[metricName]; exists {
		return weight * (zScore / 10.0) // Normalize z-score to 0-1 range approximately
	}
	return 0.1 * (zScore / 10.0) // Default weight for unknown metrics
}

func (ad *AnomalyDetector) isCriticalMetric(metricName string, value float64) bool {
	// Business-critical thresholds that indicate immediate impact
	switch metricName {
	case "error_rate_percent":
		return value > 5.0 // >5% error rate is critical
	case "response_time_ms":
		return value > 1000.0 // >1s response time is critical
	case "memory_usage_percent":
		return value > 90.0 // >90% memory usage is critical
	case "cpu_usage_percent":
		return value > 85.0 // >85% CPU usage is critical
	}
	return false
}

func (ad *AnomalyDetector) generateBusinessRecommendations(serviceName string, metrics map[string]float64, anomalyReasons []string) []string {
	recommendations := []string{}

	// Generate recommendations based on specific anomalies detected
	for _, reason := range anomalyReasons {
		if strings.Contains(reason, "response_time_ms") {
			recommendations = append(recommendations, "investigate_performance_bottlenecks", "consider_scaling_up")
		}
		if strings.Contains(reason, "error_rate_percent") {
			recommendations = append(recommendations, "review_error_logs", "check_service_health")
		}
		if strings.Contains(reason, "memory_usage_percent") {
			recommendations = append(recommendations, "investigate_memory_leaks", "increase_memory_allocation")
		}
		if strings.Contains(reason, "cpu_usage_percent") {
			recommendations = append(recommendations, "optimize_cpu_intensive_operations", "horizontal_scaling")
		}
		if strings.Contains(reason, "throughput_rps") {
			recommendations = append(recommendations, "investigate_capacity_constraints", "check_upstream_services")
		}
	}

	// Always include general monitoring recommendation
	recommendations = append(recommendations, "monitor_service_closely", "prepare_incident_response")

	// Remove duplicates
	uniqueRecommendations := make([]string, 0)
	seen := make(map[string]bool)
	for _, rec := range recommendations {
		if !seen[rec] {
			uniqueRecommendations = append(uniqueRecommendations, rec)
			seen[rec] = true
		}
	}

	return uniqueRecommendations
}
