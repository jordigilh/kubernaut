package patterns

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// Statistical validation types and implementations

// StatisticalAssumptionResult contains the result of statistical assumption validation
type StatisticalAssumptionResult struct {
	IsValid               bool               `json:"is_valid"`
	Assumptions           []*AssumptionCheck `json:"assumptions"`
	Recommendations       []string           `json:"recommendations"`
	SampleSizeAdequate    bool               `json:"sample_size_adequate"`
	MinRecommendedSamples int                `json:"min_recommended_samples"`
	DataQualityScore      float64            `json:"data_quality_score"`
	TemporalConsistency   float64            `json:"temporal_consistency"`
	OverallReliability    float64            `json:"overall_reliability"`
}

// AssumptionCheck represents a single statistical assumption check
type AssumptionCheck struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Passed      bool    `json:"passed"`
	Score       float64 `json:"score"`
	PValue      float64 `json:"p_value,omitempty"`
	Statistic   float64 `json:"statistic,omitempty"`
	Threshold   float64 `json:"threshold"`
	Details     string  `json:"details"`
}

// StatisticalConfidenceInterval represents a statistical confidence interval
type StatisticalConfidenceInterval struct {
	Lower           float64 `json:"lower"`
	Upper           float64 `json:"upper"`
	ConfidenceLevel float64 `json:"confidence_level"`
	IsReliable      bool    `json:"is_reliable"`
	SampleSize      int     `json:"sample_size"`
	Method          string  `json:"method"`
}

// ReliabilityAssessment assesses the reliability of ML predictions
type ReliabilityAssessment struct {
	IsReliable          bool     `json:"is_reliable"`
	ReliabilityScore    float64  `json:"reliability_score"`
	RecommendedMinSize  int      `json:"recommended_min_size"`
	ActualSize          int      `json:"actual_size"`
	DataQuality         float64  `json:"data_quality"`
	TemporalStability   float64  `json:"temporal_stability"`
	StatisticalValidity float64  `json:"statistical_validity"`
	Recommendations     []string `json:"recommendations"`
}

// StatisticalValidator provides validation for statistical assumptions in ML models
type StatisticalValidator struct {
	log    *logrus.Logger
	config *PatternDiscoveryConfig
}

// Overfitting prevention types and implementations

// OverfittingRisk levels
type OverfittingRisk string

const (
	OverfittingRiskLow      OverfittingRisk = "low"
	OverfittingRiskModerate OverfittingRisk = "moderate"
	OverfittingRiskHigh     OverfittingRisk = "high"
	OverfittingRiskCritical OverfittingRisk = "critical"
)

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

// TrainingMetrics contains training-specific metrics
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

// RegularizationConfig defines regularization techniques
type RegularizationConfig struct {
	Techniques map[string]interface{} `json:"techniques"`
}

// OverfittingPrevention provides mechanisms to detect and prevent model overfitting
type OverfittingPrevention struct {
	log    *logrus.Logger
	config *PatternDiscoveryConfig
}

// StatisticalValidator implementation

// NewStatisticalValidator creates a new statistical validator
func NewStatisticalValidator(config *PatternDiscoveryConfig, log *logrus.Logger) *StatisticalValidator {
	return &StatisticalValidator{
		config: config,
		log:    log,
	}
}

// ValidateStatisticalAssumptions validates key statistical assumptions for ML models
func (sv *StatisticalValidator) ValidateStatisticalAssumptions(data []*sharedtypes.WorkflowExecutionData) *StatisticalAssumptionResult {
	result := &StatisticalAssumptionResult{
		IsValid:            true,
		Assumptions:        make([]*AssumptionCheck, 0),
		Recommendations:    make([]string, 0),
		SampleSizeAdequate: len(data) >= sv.config.MinExecutionsForPattern*2,
	}

	// 1. Check sample size adequacy
	sampleSizeCheck := sv.checkSampleSizeAdequacy(len(data))
	result.Assumptions = append(result.Assumptions, sampleSizeCheck)
	if !sampleSizeCheck.Passed {
		result.IsValid = false
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Increase sample size to at least %d for reliable analysis", int(sampleSizeCheck.Threshold)))
	}

	// Calculate overall scores
	result.DataQualityScore = sv.calculateDataQuality(data)
	result.MinRecommendedSamples = int(sampleSizeCheck.Threshold)

	// Calculate overall reliability
	totalScore := 0.0
	for _, assumption := range result.Assumptions {
		if assumption.Passed {
			totalScore += assumption.Score
		} else {
			totalScore += assumption.Score * 0.5 // Penalty for failed assumptions
		}
	}
	if len(result.Assumptions) > 0 {
		result.OverallReliability = totalScore / float64(len(result.Assumptions))
	}

	return result
}

// AssessReliability provides a comprehensive reliability assessment
func (sv *StatisticalValidator) AssessReliability(data []*sharedtypes.WorkflowExecutionData) *ReliabilityAssessment {
	assessment := &ReliabilityAssessment{
		ActualSize:      len(data),
		Recommendations: make([]string, 0),
	}

	// Minimum recommended size based on statistical power analysis
	assessment.RecommendedMinSize = sv.calculateMinSampleSize(0.8, 0.05) // 80% power, 5% alpha

	// Check if we have sufficient data
	assessment.IsReliable = len(data) >= assessment.RecommendedMinSize

	// Assess data quality
	assessment.DataQuality = sv.calculateDataQuality(data)

	// Calculate overall reliability score
	weights := map[string]float64{
		"sample_size":  0.4,
		"data_quality": 0.6,
	}

	sampleSizeScore := math.Min(1.0, float64(len(data))/float64(assessment.RecommendedMinSize))
	assessment.ReliabilityScore = weights["sample_size"]*sampleSizeScore + weights["data_quality"]*assessment.DataQuality

	// Generate recommendations
	if !assessment.IsReliable {
		assessment.Recommendations = append(assessment.Recommendations,
			fmt.Sprintf("Increase sample size from %d to at least %d", len(data), assessment.RecommendedMinSize))
	}

	if assessment.DataQuality < 0.7 {
		assessment.Recommendations = append(assessment.Recommendations,
			"Improve data quality by reducing missing values and inconsistencies")
	}

	return assessment
}

func (sv *StatisticalValidator) checkSampleSizeAdequacy(sampleSize int) *AssumptionCheck {
	threshold := float64(sv.config.MinExecutionsForPattern * 2)
	passed := sampleSize >= int(threshold)

	return &AssumptionCheck{
		Name:        "sample_size_adequacy",
		Description: "Adequate sample size for statistical analysis",
		Passed:      passed,
		Score:       math.Min(1.0, float64(sampleSize)/threshold),
		Threshold:   threshold,
		Details:     fmt.Sprintf("Sample size: %d, Required: %.0f", sampleSize, threshold),
	}
}

func (sv *StatisticalValidator) calculateDataQuality(data []*sharedtypes.WorkflowExecutionData) float64 {
	if len(data) == 0 {
		return 0.0
	}

	qualityScore := 0.0
	validCount := 0

	for _, d := range data {
		score := 0.0

		// Check for required fields
		if d.ExecutionID != "" {
			score += 0.25
		}
		if !d.Timestamp.IsZero() {
			score += 0.25
		}
		if d.WorkflowID != "" {
			score += 0.25
		}
		if d.Duration > 0 {
			score += 0.25
		}

		qualityScore += score
		validCount++
	}

	if validCount == 0 {
		return 0.0
	}

	return qualityScore / float64(validCount)
}

func (sv *StatisticalValidator) calculateMinSampleSize(power, alpha float64) int {
	// Simplified sample size calculation for proportion test
	effectSize := 0.2 // Small effect size (Cohen's h)
	zAlpha := sv.getZScore(1.0 - alpha/2.0)
	zBeta := sv.getZScore(power)

	// Guideline #14: Use idiomatic patterns - expand math.Pow for simple squaring
	numerator := zAlpha + zBeta
	n := (numerator * numerator) / (effectSize * effectSize)

	// Minimum practical size
	return int(math.Max(30, math.Ceil(n)))
}

func (sv *StatisticalValidator) getZScore(probability float64) float64 {
	// Simplified Z-score calculation (would use proper implementation in production)
	if probability >= 0.975 {
		return 1.96
	} else if probability >= 0.95 {
		return 1.65
	} else if probability >= 0.9 {
		return 1.28
	} else if probability >= 0.8 {
		return 0.84
	}
	return 0.0
}

// CalculateConfidenceInterval calculates a simple confidence interval for pattern reliability
func (sv *StatisticalValidator) CalculateConfidenceInterval(successes, total int, confidenceLevel float64) *StatisticalConfidenceInterval {
	if total == 0 {
		return &StatisticalConfidenceInterval{
			Lower:           0.0,
			Upper:           1.0,
			ConfidenceLevel: confidenceLevel,
		}
	}

	successRate := float64(successes) / float64(total)
	z := sv.getZScore((1.0 + confidenceLevel) / 2.0)
	margin := z * (successRate * (1.0 - successRate) / float64(total))

	lower := successRate - margin
	upper := successRate + margin

	// Ensure bounds
	if lower < 0.0 {
		lower = 0.0
	}
	if upper > 1.0 {
		upper = 1.0
	}

	return &StatisticalConfidenceInterval{
		Lower:           lower,
		Upper:           upper,
		ConfidenceLevel: confidenceLevel,
	}
}

// OverfittingPrevention implementation

// NewOverfittingPrevention creates a new overfitting prevention system
func NewOverfittingPrevention(config *PatternDiscoveryConfig, log *logrus.Logger) *OverfittingPrevention {
	return &OverfittingPrevention{
		config: config,
		log:    log,
	}
}

// AssessOverfittingRisk evaluates the risk of overfitting for a model
func (op *OverfittingPrevention) AssessOverfittingRisk(
	trainingData []*sharedtypes.WorkflowExecutionData,
	model *MLModel,
	crossValMetrics *CrossValidationMetrics,
) *OverfittingAssessment {

	assessment := &OverfittingAssessment{
		Indicators:           make([]*OverfittingIndicator, 0),
		Recommendations:      make([]string, 0),
		PreventionStrategies: make([]string, 0),
	}

	// 1. Check model complexity relative to data size
	complexityIndicator := op.checkModelComplexity(model, len(trainingData))
	assessment.Indicators = append(assessment.Indicators, complexityIndicator)

	// 2. Check variance in cross-validation scores
	if crossValMetrics != nil {
		varianceIndicator := op.checkCrossValidationVariance(crossValMetrics)
		assessment.Indicators = append(assessment.Indicators, varianceIndicator)
	}

	// Calculate overall risk score
	assessment.RiskScore = op.calculateOverallRiskScore(assessment.Indicators)
	assessment.OverfittingRisk = op.determineRiskLevel(assessment.RiskScore)

	// Generate recommendations based on indicators
	assessment.Recommendations = op.generateRecommendations(assessment.Indicators)
	assessment.PreventionStrategies = op.generatePreventionStrategies(assessment.Indicators)

	// Create validation metrics summary
	if crossValMetrics != nil {
		assessment.ValidationMetrics = op.createValidationMetrics(crossValMetrics, assessment.Indicators)
	}

	// Determine if model is reliable for production use
	assessment.IsModelReliable = assessment.OverfittingRisk != OverfittingRiskHigh &&
		assessment.OverfittingRisk != OverfittingRiskCritical &&
		assessment.RiskScore < 0.7

	return assessment
}

func (op *OverfittingPrevention) checkModelComplexity(model *MLModel, sampleSize int) *OverfittingIndicator {
	if model == nil {
		return &OverfittingIndicator{
			Name:        "model_complexity",
			Description: "Model complexity analysis",
			Value:       0,
			Threshold:   1,
			Severity:    "unknown",
			Detected:    false,
			Impact:      "Unable to assess - no model provided",
		}
	}

	// Estimate parameters (simplified)
	paramCount := len(model.Features)
	if len(model.Weights) > 0 {
		paramCount = len(model.Weights)
	}

	ratio := float64(paramCount) / math.Max(1, float64(sampleSize))
	threshold := 0.1 // 10% rule of thumb

	return &OverfittingIndicator{
		Name:        "model_complexity",
		Description: "Parameter to sample size ratio",
		Value:       ratio,
		Threshold:   threshold,
		Severity:    op.determineSeverity(ratio, threshold),
		Detected:    ratio > threshold,
		Impact:      fmt.Sprintf("Ratio: %.3f (params: %d, samples: %d)", ratio, paramCount, sampleSize),
	}
}

func (op *OverfittingPrevention) checkCrossValidationVariance(metrics *CrossValidationMetrics) *OverfittingIndicator {
	threshold := 0.05 // 5% standard deviation threshold

	return &OverfittingIndicator{
		Name:        "cross_validation_variance",
		Description: "Variance in cross-validation scores",
		Value:       metrics.StdAccuracy,
		Threshold:   threshold,
		Severity:    op.determineSeverity(metrics.StdAccuracy, threshold),
		Detected:    metrics.StdAccuracy > threshold,
		Impact:      fmt.Sprintf("CV std: %.3f, mean accuracy: %.3f", metrics.StdAccuracy, metrics.MeanAccuracy),
	}
}

func (op *OverfittingPrevention) calculateOverallRiskScore(indicators []*OverfittingIndicator) float64 {
	if len(indicators) == 0 {
		return 0.0
	}

	score := 0.0
	for _, indicator := range indicators {
		if indicator.Detected {
			score += indicator.Value / indicator.Threshold
		}
	}

	return math.Min(1.0, score/float64(len(indicators)))
}

func (op *OverfittingPrevention) determineRiskLevel(riskScore float64) OverfittingRisk {
	if riskScore >= 0.8 {
		return OverfittingRiskCritical
	} else if riskScore >= 0.6 {
		return OverfittingRiskHigh
	} else if riskScore >= 0.3 {
		return OverfittingRiskModerate
	}
	return OverfittingRiskLow
}

func (op *OverfittingPrevention) generateRecommendations(indicators []*OverfittingIndicator) []string {
	recommendations := []string{}

	for _, indicator := range indicators {
		if indicator.Detected {
			switch indicator.Name {
			case "model_complexity":
				recommendations = append(recommendations, "Consider model regularization or feature selection")
			case "cross_validation_variance":
				recommendations = append(recommendations, "High variance detected - consider ensemble methods or regularization")
			}
		}
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "No overfitting risks detected")
	}

	return recommendations
}

func (op *OverfittingPrevention) generatePreventionStrategies(indicators []*OverfittingIndicator) []string {
	strategies := []string{
		"Implement k-fold cross-validation",
		"Use early stopping during training",
		"Apply appropriate regularization techniques",
		"Monitor validation metrics during training",
	}

	// Add specific strategies based on detected issues
	for _, indicator := range indicators {
		if indicator.Detected {
			switch indicator.Name {
			case "model_complexity":
				strategies = append(strategies, "Implement model pruning techniques")
			}
		}
	}

	return strategies
}

func (op *OverfittingPrevention) createValidationMetrics(crossVal *CrossValidationMetrics, indicators []*OverfittingIndicator) *ValidationMetrics {
	metrics := &ValidationMetrics{
		ValidationAccuracy: crossVal.MeanAccuracy,
		VarianceScore:      crossVal.StdAccuracy,
	}

	// Extract values from indicators
	for _, indicator := range indicators {
		switch indicator.Name {
		case "model_complexity":
			metrics.ComplexityScore = indicator.Value
		}
	}

	// Calculate derived metrics
	metrics.BiasScore = 1.0 - metrics.ValidationAccuracy // Higher bias = lower validation accuracy
	metrics.GeneralizationScore = math.Max(0, 1.0-metrics.VarianceScore)

	return metrics
}

func (op *OverfittingPrevention) determineSeverity(value, threshold float64) string {
	ratio := value / threshold
	if ratio >= 2.0 {
		return "critical"
	} else if ratio >= 1.5 {
		return "high"
	} else if ratio >= 1.0 {
		return "moderate"
	}
	return "low"
}

// PatternEngineMonitor provides comprehensive monitoring for the pattern discovery engine
type PatternEngineMonitor struct {
	engine                *PatternDiscoveryEngine
	statisticalValidator  *StatisticalValidator
	overfittingPrevention *OverfittingPrevention
	log                   *logrus.Logger
	config                *MonitoringConfig

	// Monitoring state
	mu              sync.RWMutex
	healthStatus    EngineHealthStatus
	alerts          []*EngineAlert
	metrics         *EngineMetrics
	lastHealthCheck time.Time

	// Background monitoring
	stopChan         chan struct{}
	monitoringActive bool
}

// MonitoringConfig configures the monitoring system
type MonitoringConfig struct {
	HealthCheckInterval       time.Duration  `yaml:"health_check_interval" default:"5m"`
	MetricsCollectionInterval time.Duration  `yaml:"metrics_collection_interval" default:"1m"`
	AlertThreshold            AlertThreshold `yaml:"alert_threshold"`
	RetentionPeriod           time.Duration  `yaml:"retention_period" default:"24h"`
	MaxAlerts                 int            `yaml:"max_alerts" default:"100"`
	EnableAutomaticRecovery   bool           `yaml:"enable_automatic_recovery" default:"true"`
	NotificationChannels      []string       `yaml:"notification_channels"`
}

// AlertThreshold defines thresholds for various alerts
type AlertThreshold struct {
	ConfidenceDegradation  float64 `yaml:"confidence_degradation" default:"0.1"`
	PerformanceDegradation float64 `yaml:"performance_degradation" default:"0.15"`
	ErrorRate              float64 `yaml:"error_rate" default:"0.05"`
	MemoryUsage            float64 `yaml:"memory_usage" default:"0.8"`
	ResponseTime           float64 `yaml:"response_time" default:"30.0"`
	OverfittingRisk        float64 `yaml:"overfitting_risk" default:"0.7"`
}

// EngineHealthStatus represents the overall health of the pattern engine
type EngineHealthStatus struct {
	Overall          HealthLevel            `json:"overall"`
	Components       map[string]HealthLevel `json:"components"`
	LastUpdated      time.Time              `json:"last_updated"`
	Issues           []*HealthIssue         `json:"issues"`
	Recommendations  []string               `json:"recommendations"`
	UptimePercentage float64                `json:"uptime_percentage"`

	// Component-specific health
	MLAnalyzer       HealthLevel `json:"ml_analyzer"`
	TimeSeriesEngine HealthLevel `json:"time_series_engine"`
	ClusteringEngine HealthLevel `json:"clustering_engine"`
	AnomalyDetector  HealthLevel `json:"anomaly_detector"`
	PatternStore     HealthLevel `json:"pattern_store"`
	VectorDB         HealthLevel `json:"vector_db"`
}

// HealthLevel represents health status levels
type HealthLevel string

const (
	HealthLevelHealthy   HealthLevel = "healthy"
	HealthLevelWarning   HealthLevel = "warning"
	HealthLevelCritical  HealthLevel = "critical"
	HealthLevelUnhealthy HealthLevel = "unhealthy"
	HealthLevelUnknown   HealthLevel = "unknown"
)

// HealthIssue represents a specific health issue
type HealthIssue struct {
	ID          string      `json:"id"`
	Component   string      `json:"component"`
	Severity    HealthLevel `json:"severity"`
	Description string      `json:"description"`
	FirstSeen   time.Time   `json:"first_seen"`
	LastSeen    time.Time   `json:"last_seen"`
	Count       int         `json:"count"`
	Resolved    bool        `json:"resolved"`
}

// EngineAlert represents an alert from the monitoring system
type EngineAlert struct {
	ID           string                 `json:"id"`
	Type         AlertType              `json:"type"`
	Severity     AlertSeverity          `json:"severity"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Component    string                 `json:"component"`
	Timestamp    time.Time              `json:"timestamp"`
	Resolved     bool                   `json:"resolved"`
	ResolvedAt   *time.Time             `json:"resolved_at,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
	ActionsTaken []string               `json:"actions_taken"`
}

// AlertType defines different types of alerts
type AlertType string

const (
	AlertTypePerformance    AlertType = "performance"
	AlertTypeConfidence     AlertType = "confidence"
	AlertTypeOverfitting    AlertType = "overfitting"
	AlertTypeDataQuality    AlertType = "data_quality"
	AlertTypeSystemHealth   AlertType = "system_health"
	AlertTypeResourceUsage  AlertType = "resource_usage"
	AlertTypePatternQuality AlertType = "pattern_quality"
)

// AlertSeverity defines alert severity levels
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityError    AlertSeverity = "error"
	AlertSeverityCritical AlertSeverity = "critical"
)

// EngineMetrics contains comprehensive metrics about the pattern engine
type EngineMetrics struct {
	Timestamp time.Time `json:"timestamp"`

	// Performance metrics
	AnalysisCount       int64         `json:"analysis_count"`
	AverageAnalysisTime time.Duration `json:"average_analysis_time"`
	LastAnalysisTime    time.Duration `json:"last_analysis_time"`
	SuccessRate         float64       `json:"success_rate"`
	ErrorRate           float64       `json:"error_rate"`

	// Pattern metrics
	PatternsDiscovered  int64   `json:"patterns_discovered"`
	AverageConfidence   float64 `json:"average_confidence"`
	HighConfidenceCount int64   `json:"high_confidence_count"`
	LowConfidenceCount  int64   `json:"low_confidence_count"`

	// Model metrics
	ModelsActive    int       `json:"models_active"`
	ModelAccuracy   float64   `json:"model_accuracy"`
	OverfittingRisk float64   `json:"overfitting_risk"`
	LastModelUpdate time.Time `json:"last_model_update"`

	// System metrics
	MemoryUsage       float64 `json:"memory_usage"`
	CPUUsage          float64 `json:"cpu_usage"`
	DiskUsage         float64 `json:"disk_usage"`
	ActiveConnections int     `json:"active_connections"`

	// Data quality metrics
	DataQualityScore   float64 `json:"data_quality_score"`
	ValidationFailures int64   `json:"validation_failures"`
	OutlierCount       int64   `json:"outlier_count"`
}

// NewPatternEngineMonitor creates a new pattern engine monitor
func NewPatternEngineMonitor(
	engine *PatternDiscoveryEngine,
	config *MonitoringConfig,
	log *logrus.Logger,
) *PatternEngineMonitor {

	if config == nil {
		config = &MonitoringConfig{
			HealthCheckInterval:       5 * time.Minute,
			MetricsCollectionInterval: 1 * time.Minute,
			RetentionPeriod:           24 * time.Hour,
			MaxAlerts:                 100,
			EnableAutomaticRecovery:   true,
			AlertThreshold: AlertThreshold{
				ConfidenceDegradation:  0.1,
				PerformanceDegradation: 0.15,
				ErrorRate:              0.05,
				MemoryUsage:            0.8,
				ResponseTime:           30.0,
				OverfittingRisk:        0.7,
			},
		}
	}

	monitor := &PatternEngineMonitor{
		engine:                engine,
		log:                   log,
		config:                config,
		alerts:                make([]*EngineAlert, 0),
		metrics:               &EngineMetrics{},
		stopChan:              make(chan struct{}),
		statisticalValidator:  NewStatisticalValidator(engine.config, log),
		overfittingPrevention: NewOverfittingPrevention(engine.config, log),
	}

	monitor.initializeHealthStatus()

	return monitor
}

// StartMonitoring begins continuous monitoring of the pattern engine
func (pem *PatternEngineMonitor) StartMonitoring(ctx context.Context) error {
	pem.mu.Lock()
	if pem.monitoringActive {
		pem.mu.Unlock()
		return fmt.Errorf("monitoring is already active")
	}
	pem.monitoringActive = true
	pem.mu.Unlock()

	pem.log.Info("Starting pattern engine monitoring")

	// Start background monitoring goroutines
	go pem.healthCheckLoop(ctx)
	go pem.metricsCollectionLoop(ctx)
	go pem.alertProcessingLoop(ctx)

	return nil
}

// StopMonitoring stops the monitoring system
func (pem *PatternEngineMonitor) StopMonitoring() {
	pem.mu.Lock()
	if !pem.monitoringActive {
		pem.mu.Unlock()
		return
	}
	pem.monitoringActive = false
	pem.mu.Unlock()

	close(pem.stopChan)
	pem.log.Info("Pattern engine monitoring stopped")
}

// GetHealthStatus returns the current health status
func (pem *PatternEngineMonitor) GetHealthStatus() *EngineHealthStatus {
	pem.mu.RLock()
	defer pem.mu.RUnlock()

	// Return a copy to prevent concurrent access issues
	status := pem.healthStatus
	return &status
}

// GetMetrics returns the current metrics
func (pem *PatternEngineMonitor) GetMetrics() *EngineMetrics {
	pem.mu.RLock()
	defer pem.mu.RUnlock()

	// Return a copy
	metrics := *pem.metrics
	return &metrics
}

// GetAlerts returns current active alerts
func (pem *PatternEngineMonitor) GetAlerts(includeResolved bool) []*EngineAlert {
	pem.mu.RLock()
	defer pem.mu.RUnlock()

	alerts := make([]*EngineAlert, 0)
	for _, alert := range pem.alerts {
		if includeResolved || !alert.Resolved {
			alertCopy := *alert
			alerts = append(alerts, &alertCopy)
		}
	}

	return alerts
}

// TriggerHealthCheck manually triggers a health check
func (pem *PatternEngineMonitor) TriggerHealthCheck() *EngineHealthStatus {
	return pem.performHealthCheck()
}

// RecordAnalysisMetrics records metrics from a pattern analysis
func (pem *PatternEngineMonitor) RecordAnalysisMetrics(
	analysisTime time.Duration,
	patternsFound int,
	avgConfidence float64,
	success bool,
) {
	pem.mu.Lock()
	defer pem.mu.Unlock()

	pem.metrics.AnalysisCount++
	pem.metrics.LastAnalysisTime = analysisTime

	// Update average analysis time (moving average)
	if pem.metrics.AverageAnalysisTime == 0 {
		pem.metrics.AverageAnalysisTime = analysisTime
	} else {
		pem.metrics.AverageAnalysisTime =
			(pem.metrics.AverageAnalysisTime + analysisTime) / 2
	}

	pem.metrics.PatternsDiscovered += int64(patternsFound)

	// Update confidence metrics
	if avgConfidence > 0 {
		if pem.metrics.AverageConfidence == 0 {
			pem.metrics.AverageConfidence = avgConfidence
		} else {
			pem.metrics.AverageConfidence =
				(pem.metrics.AverageConfidence + avgConfidence) / 2
		}

		if avgConfidence >= 0.8 {
			pem.metrics.HighConfidenceCount++
		} else if avgConfidence < 0.5 {
			pem.metrics.LowConfidenceCount++
		}
	}

	// Update success/error rates
	totalAnalyses := float64(pem.metrics.AnalysisCount)
	if success {
		pem.metrics.SuccessRate = (pem.metrics.SuccessRate*(totalAnalyses-1) + 1.0) / totalAnalyses
	} else {
		pem.metrics.SuccessRate = (pem.metrics.SuccessRate*(totalAnalyses-1) + 0.0) / totalAnalyses
	}
	pem.metrics.ErrorRate = 1.0 - pem.metrics.SuccessRate

	// Check if we need to raise alerts
	pem.checkMetricThresholds()
}

// Private methods

func (pem *PatternEngineMonitor) initializeHealthStatus() {
	pem.healthStatus = EngineHealthStatus{
		Overall:          HealthLevelHealthy,
		Components:       make(map[string]HealthLevel),
		LastUpdated:      time.Now(),
		Issues:           make([]*HealthIssue, 0),
		Recommendations:  make([]string, 0),
		UptimePercentage: 100.0,
		MLAnalyzer:       HealthLevelUnknown,
		TimeSeriesEngine: HealthLevelUnknown,
		ClusteringEngine: HealthLevelUnknown,
		AnomalyDetector:  HealthLevelUnknown,
		PatternStore:     HealthLevelUnknown,
		VectorDB:         HealthLevelUnknown,
	}
}

func (pem *PatternEngineMonitor) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(pem.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pem.stopChan:
			return
		case <-ticker.C:
			pem.performHealthCheck()
		}
	}
}

func (pem *PatternEngineMonitor) metricsCollectionLoop(ctx context.Context) {
	ticker := time.NewTicker(pem.config.MetricsCollectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pem.stopChan:
			return
		case <-ticker.C:
			pem.collectSystemMetrics()
		}
	}
}

func (pem *PatternEngineMonitor) alertProcessingLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute) // Process alerts every minute
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pem.stopChan:
			return
		case <-ticker.C:
			pem.processAlerts()
			pem.cleanupOldAlerts()
		}
	}
}

func (pem *PatternEngineMonitor) performHealthCheck() *EngineHealthStatus {
	pem.mu.Lock()
	defer pem.mu.Unlock()

	pem.log.Debug("Performing pattern engine health check")

	// Check each component
	pem.healthStatus.MLAnalyzer = pem.checkMLAnalyzerHealth()
	pem.healthStatus.TimeSeriesEngine = pem.checkTimeSeriesEngineHealth()
	pem.healthStatus.ClusteringEngine = pem.checkClusteringEngineHealth()
	pem.healthStatus.AnomalyDetector = pem.checkAnomalyDetectorHealth()
	pem.healthStatus.PatternStore = pem.checkPatternStoreHealth()
	pem.healthStatus.VectorDB = pem.checkVectorDBHealth()

	// Update component map
	pem.healthStatus.Components = map[string]HealthLevel{
		"ml_analyzer":        pem.healthStatus.MLAnalyzer,
		"time_series_engine": pem.healthStatus.TimeSeriesEngine,
		"clustering_engine":  pem.healthStatus.ClusteringEngine,
		"anomaly_detector":   pem.healthStatus.AnomalyDetector,
		"pattern_store":      pem.healthStatus.PatternStore,
		"vector_db":          pem.healthStatus.VectorDB,
	}

	// Determine overall health
	pem.healthStatus.Overall = pem.calculateOverallHealth()
	pem.healthStatus.LastUpdated = time.Now()
	pem.lastHealthCheck = time.Now()

	// Generate recommendations if needed
	pem.generateHealthRecommendations()

	pem.log.WithFields(logrus.Fields{
		"overall_health": pem.healthStatus.Overall,
		"issues_count":   len(pem.healthStatus.Issues),
	}).Info("Health check completed")

	return &pem.healthStatus
}

func (pem *PatternEngineMonitor) checkMLAnalyzerHealth() HealthLevel {
	if pem.engine.mlAnalyzer == nil {
		pem.addHealthIssue("ml_analyzer", HealthLevelCritical, "ML Analyzer is not initialized")
		return HealthLevelCritical
	}

	// Check for overfitting
	if pem.metrics.OverfittingRisk > pem.config.AlertThreshold.OverfittingRisk {
		pem.addHealthIssue("ml_analyzer", HealthLevelWarning, "High overfitting risk detected")
		return HealthLevelWarning
	}

	return HealthLevelHealthy
}

func (pem *PatternEngineMonitor) checkTimeSeriesEngineHealth() HealthLevel {
	if pem.engine.timeSeriesEngine == nil {
		pem.addHealthIssue("time_series_engine", HealthLevelCritical, "Time Series Engine is not initialized")
		return HealthLevelCritical
	}

	// Additional health checks would go here
	return HealthLevelHealthy
}

// RULE 12 COMPLIANCE: Updated to check enhanced llm.Client instead of deprecated clusteringEngine
func (pem *PatternEngineMonitor) checkClusteringEngineHealth() HealthLevel {
	if pem.engine.llmClient == nil {
		pem.addHealthIssue("llm_client", HealthLevelCritical, "Enhanced LLM Client is not initialized")
		return HealthLevelCritical
	}

	return HealthLevelHealthy
}

func (pem *PatternEngineMonitor) checkAnomalyDetectorHealth() HealthLevel {
	if pem.engine.anomalyDetector == nil {
		pem.addHealthIssue("anomaly_detector", HealthLevelCritical, "Anomaly Detector is not initialized")
		return HealthLevelCritical
	}

	return HealthLevelHealthy
}

func (pem *PatternEngineMonitor) checkPatternStoreHealth() HealthLevel {
	if pem.engine.patternStore == nil {
		pem.addHealthIssue("pattern_store", HealthLevelWarning, "Pattern Store is not configured")
		return HealthLevelWarning
	}

	// Could add connectivity checks here
	return HealthLevelHealthy
}

func (pem *PatternEngineMonitor) checkVectorDBHealth() HealthLevel {
	if pem.engine.vectorDB == nil {
		pem.addHealthIssue("vector_db", HealthLevelWarning, "Vector Database is not configured")
		return HealthLevelWarning
	}

	// Could add connectivity checks here
	return HealthLevelHealthy
}

func (pem *PatternEngineMonitor) calculateOverallHealth() HealthLevel {
	componentLevels := []HealthLevel{
		pem.healthStatus.MLAnalyzer,
		pem.healthStatus.TimeSeriesEngine,
		pem.healthStatus.ClusteringEngine,
		pem.healthStatus.AnomalyDetector,
		pem.healthStatus.PatternStore,
		pem.healthStatus.VectorDB,
	}

	criticalCount := 0
	warningCount := 0

	for _, level := range componentLevels {
		switch level {
		case HealthLevelCritical, HealthLevelUnhealthy:
			criticalCount++
		case HealthLevelWarning:
			warningCount++
		}
	}

	if criticalCount > 0 {
		return HealthLevelCritical
	}
	if warningCount >= 2 {
		return HealthLevelWarning
	}
	if warningCount == 1 {
		return HealthLevelWarning
	}

	return HealthLevelHealthy
}

func (pem *PatternEngineMonitor) addHealthIssue(component string, severity HealthLevel, description string) {
	issueID := fmt.Sprintf("%s_%s_%d", component, severity, time.Now().Unix())

	issue := &HealthIssue{
		ID:          issueID,
		Component:   component,
		Severity:    severity,
		Description: description,
		FirstSeen:   time.Now(),
		LastSeen:    time.Now(),
		Count:       1,
		Resolved:    false,
	}

	// Check if similar issue already exists
	for _, existingIssue := range pem.healthStatus.Issues {
		if existingIssue.Component == component && existingIssue.Description == description && !existingIssue.Resolved {
			existingIssue.LastSeen = time.Now()
			existingIssue.Count++
			return
		}
	}

	pem.healthStatus.Issues = append(pem.healthStatus.Issues, issue)

	// Create corresponding alert
	pem.createAlert(AlertTypeSystemHealth, pem.severityFromHealthLevel(severity),
		fmt.Sprintf("Health Issue: %s", component), description, component, map[string]interface{}{
			"health_issue_id": issueID,
		})
}

func (pem *PatternEngineMonitor) generateHealthRecommendations() {
	recommendations := make([]string, 0)

	for _, issue := range pem.healthStatus.Issues {
		if issue.Resolved {
			continue
		}

		switch issue.Component {
		case "ml_analyzer":
			if issue.Severity == HealthLevelCritical {
				recommendations = append(recommendations, "Initialize ML Analyzer component")
			}
			recommendations = append(recommendations, "Monitor model performance and retrain if necessary")
		case "pattern_store":
			recommendations = append(recommendations, "Configure pattern storage backend")
		case "vector_db":
			recommendations = append(recommendations, "Configure vector database for pattern similarity search")
		}
	}

	if pem.metrics.ErrorRate > pem.config.AlertThreshold.ErrorRate {
		recommendations = append(recommendations, "Investigate high error rate in pattern analysis")
	}

	if pem.metrics.AverageConfidence < 0.6 {
		recommendations = append(recommendations, "Review pattern discovery parameters to improve confidence")
	}

	pem.healthStatus.Recommendations = recommendations
}

func (pem *PatternEngineMonitor) collectSystemMetrics() {
	pem.mu.Lock()
	defer pem.mu.Unlock()

	// Update timestamp
	pem.metrics.Timestamp = time.Now()

	// Collect model metrics if available
	if pem.engine.mlAnalyzer != nil {
		// Set basic model count (simplified since GetModelCount() may not exist)
		pem.metrics.ModelsActive = 1
		pem.metrics.ModelAccuracy = 0.8 // Placeholder value
	}

	// Could add system resource monitoring here (memory, CPU, etc.)
	// For now, using placeholder values
	pem.metrics.MemoryUsage = 0.3 // 30% memory usage
	pem.metrics.CPUUsage = 0.2    // 20% CPU usage
}

func (pem *PatternEngineMonitor) checkMetricThresholds() {
	// Check error rate
	if pem.metrics.ErrorRate > pem.config.AlertThreshold.ErrorRate {
		pem.createAlert(AlertTypePerformance, AlertSeverityWarning,
			"High Error Rate",
			fmt.Sprintf("Error rate %.2f%% exceeds threshold %.2f%%",
				pem.metrics.ErrorRate*100, pem.config.AlertThreshold.ErrorRate*100),
			"pattern_engine", map[string]interface{}{
				"error_rate": pem.metrics.ErrorRate,
				"threshold":  pem.config.AlertThreshold.ErrorRate,
			})
	}

	// Check response time
	if pem.metrics.LastAnalysisTime.Seconds() > pem.config.AlertThreshold.ResponseTime {
		pem.createAlert(AlertTypePerformance, AlertSeverityWarning,
			"Slow Response Time",
			fmt.Sprintf("Analysis time %.2fs exceeds threshold %.2fs",
				pem.metrics.LastAnalysisTime.Seconds(), pem.config.AlertThreshold.ResponseTime),
			"pattern_engine", map[string]interface{}{
				"analysis_time": pem.metrics.LastAnalysisTime.Seconds(),
				"threshold":     pem.config.AlertThreshold.ResponseTime,
			})
	}

	// Check confidence degradation
	if pem.metrics.AverageConfidence > 0 &&
		pem.metrics.AverageConfidence < (1.0-pem.config.AlertThreshold.ConfidenceDegradation) {
		pem.createAlert(AlertTypeConfidence, AlertSeverityWarning,
			"Low Pattern Confidence",
			fmt.Sprintf("Average confidence %.2f is below acceptable threshold",
				pem.metrics.AverageConfidence),
			"pattern_discovery", map[string]interface{}{
				"average_confidence": pem.metrics.AverageConfidence,
				"threshold":          1.0 - pem.config.AlertThreshold.ConfidenceDegradation,
			})
	}
}

func (pem *PatternEngineMonitor) createAlert(alertType AlertType, severity AlertSeverity, title, description, component string, metadata map[string]interface{}) {
	alert := &EngineAlert{
		ID:           fmt.Sprintf("alert_%d", time.Now().UnixNano()),
		Type:         alertType,
		Severity:     severity,
		Title:        title,
		Description:  description,
		Component:    component,
		Timestamp:    time.Now(),
		Resolved:     false,
		Metadata:     metadata,
		ActionsTaken: make([]string, 0),
	}

	pem.alerts = append(pem.alerts, alert)

	// Log the alert
	pem.log.WithFields(logrus.Fields{
		"alert_id":   alert.ID,
		"alert_type": alertType,
		"severity":   severity,
		"component":  component,
		"title":      title,
	}).Warn("Pattern engine alert created")

	// Limit the number of alerts
	if len(pem.alerts) > pem.config.MaxAlerts {
		pem.alerts = pem.alerts[1:] // Remove oldest alert
	}
}

func (pem *PatternEngineMonitor) processAlerts() {
	pem.mu.Lock()
	defer pem.mu.Unlock()

	for _, alert := range pem.alerts {
		if alert.Resolved {
			continue
		}

		// Check if alert should be auto-resolved
		if pem.shouldAutoResolveAlert(alert) {
			alert.Resolved = true
			now := time.Now()
			alert.ResolvedAt = &now
			alert.ActionsTaken = append(alert.ActionsTaken, "Auto-resolved by monitoring system")
		}
	}
}

func (pem *PatternEngineMonitor) shouldAutoResolveAlert(alert *EngineAlert) bool {
	switch alert.Type {
	case AlertTypePerformance:
		// Auto-resolve performance alerts if metrics are back to normal
		if alert.Metadata["error_rate"] != nil {
			return pem.metrics.ErrorRate <= pem.config.AlertThreshold.ErrorRate
		}
		if alert.Metadata["analysis_time"] != nil {
			return pem.metrics.LastAnalysisTime.Seconds() <= pem.config.AlertThreshold.ResponseTime
		}
	case AlertTypeConfidence:
		return pem.metrics.AverageConfidence >= (1.0 - pem.config.AlertThreshold.ConfidenceDegradation)
	}

	return false
}

func (pem *PatternEngineMonitor) cleanupOldAlerts() {
	pem.mu.Lock()
	defer pem.mu.Unlock()

	cutoff := time.Now().Add(-pem.config.RetentionPeriod)
	filtered := make([]*EngineAlert, 0)

	for _, alert := range pem.alerts {
		if alert.Timestamp.After(cutoff) {
			filtered = append(filtered, alert)
		}
	}

	pem.alerts = filtered
}

func (pem *PatternEngineMonitor) severityFromHealthLevel(level HealthLevel) AlertSeverity {
	switch level {
	case HealthLevelCritical, HealthLevelUnhealthy:
		return AlertSeverityCritical
	case HealthLevelWarning:
		return AlertSeverityWarning
	default:
		return AlertSeverityInfo
	}
}
