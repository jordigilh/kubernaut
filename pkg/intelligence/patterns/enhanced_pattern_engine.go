package patterns

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// Minimal interfaces required by this package to avoid import cycles
// These are intentionally simple and focused on what the pattern engine actually needs
type PatternAnalyzer interface {
	AnalyzeData(ctx context.Context, data interface{}) (interface{}, error)
}

// EnhancedPatternDiscoveryEngine wraps the original pattern discovery engine with additional validation and monitoring
type EnhancedPatternDiscoveryEngine struct {
	*PatternDiscoveryEngine

	// Enhanced components
	statisticalValidator  *StatisticalValidator
	overfittingPrevention *OverfittingPrevention
	monitor               *PatternEngineMonitor

	// Configuration
	config *EnhancedPatternConfig
	log    *logrus.Logger
}

// EnhancedPatternConfig extends the original configuration with validation and monitoring settings
type EnhancedPatternConfig struct {
	*PatternDiscoveryConfig

	// Validation settings
	EnableStatisticalValidation bool    `yaml:"enable_statistical_validation" default:"true"`
	RequireValidationPassing    bool    `yaml:"require_validation_passing" default:"false"`
	MinReliabilityScore         float64 `yaml:"min_reliability_score" default:"0.6"`

	// Overfitting prevention
	EnableOverfittingPrevention bool    `yaml:"enable_overfitting_prevention" default:"true"`
	MaxOverfittingRisk          float64 `yaml:"max_overfitting_risk" default:"0.7"`
	RequireCrossValidation      bool    `yaml:"require_cross_validation" default:"true"`

	// Monitoring
	EnableMonitoring bool              `yaml:"enable_monitoring" default:"true"`
	MonitoringConfig *MonitoringConfig `yaml:"monitoring_config"`
	AutoRecovery     bool              `yaml:"auto_recovery" default:"true"`
}

// EnhancedPatternAnalysisResult extends the original result with validation and quality metrics
type EnhancedPatternAnalysisResult struct {
	*PatternAnalysisResult

	// Enhanced metrics
	ValidationResults     *StatisticalAssumptionResult `json:"validation_results"`
	OverfittingAssessment *OverfittingAssessment       `json:"overfitting_assessment"`
	ReliabilityAssessment *ReliabilityAssessment       `json:"reliability_assessment"`
	QualityScore          float64                      `json:"quality_score"`

	// Warnings and recommendations
	Warnings               []string `json:"warnings"`
	QualityRecommendations []string `json:"quality_recommendations"`
	IsProductionReady      bool     `json:"is_production_ready"`
}

// NewEnhancedPatternDiscoveryEngine creates an enhanced pattern discovery engine with validation and monitoring
func NewEnhancedPatternDiscoveryEngine(
	patternStore PatternStore,
	vectorDB PatternVectorDatabase,
	executionRepo ExecutionRepository,
	mlAnalyzer MachineLearningAnalyzer,
	patternAnalyzer PatternAnalyzer,
	timeSeriesAnalyzer sharedtypes.TimeSeriesAnalyzer,
	clusteringEngine sharedtypes.ClusteringEngine,
	anomalyDetector sharedtypes.AnomalyDetector,
	config *EnhancedPatternConfig,
	log *logrus.Logger,
) (*EnhancedPatternDiscoveryEngine, error) {

	if config == nil {
		config = &EnhancedPatternConfig{
			PatternDiscoveryConfig: &PatternDiscoveryConfig{
				MinExecutionsForPattern: 10,
				MaxHistoryDays:          90,
				SamplingInterval:        time.Hour,
				SimilarityThreshold:     0.85,
				ClusteringEpsilon:       0.3,
				MinClusterSize:          5,
				ModelUpdateInterval:     24 * time.Hour,
				FeatureWindowSize:       50,
				PredictionConfidence:    0.7,
				MaxConcurrentAnalysis:   10,
				PatternCacheSize:        1000,
				EnableRealTimeDetection: true,
			},
			EnableStatisticalValidation: true,
			RequireValidationPassing:    false,
			MinReliabilityScore:         0.6,
			EnableOverfittingPrevention: true,
			MaxOverfittingRisk:          0.7,
			RequireCrossValidation:      true,
			EnableMonitoring:            true,
			AutoRecovery:                true,
		}
	}

	// Create the base pattern discovery engine
	baseEngine := NewPatternDiscoveryEngine(
		patternStore,
		vectorDB,
		executionRepo,
		mlAnalyzer,
		timeSeriesAnalyzer,
		clusteringEngine,
		anomalyDetector,
		config.PatternDiscoveryConfig,
		log,
	)

	enhanced := &EnhancedPatternDiscoveryEngine{
		PatternDiscoveryEngine: baseEngine,
		config:                 config,
		log:                    log,
	}

	// Initialize enhanced components
	if config.EnableStatisticalValidation {
		enhanced.statisticalValidator = NewStatisticalValidator(config.PatternDiscoveryConfig, log)
	}

	if config.EnableOverfittingPrevention {
		enhanced.overfittingPrevention = NewOverfittingPrevention(config.PatternDiscoveryConfig, log)
	}

	if config.EnableMonitoring {
		enhanced.monitor = NewPatternEngineMonitor(baseEngine, config.MonitoringConfig, log)
	}

	log.WithFields(logrus.Fields{
		"statistical_validation": config.EnableStatisticalValidation,
		"overfitting_prevention": config.EnableOverfittingPrevention,
		"monitoring_enabled":     config.EnableMonitoring,
	}).Info("Enhanced pattern discovery engine created")

	return enhanced, nil
}

// StartEnhancedMonitoring starts the enhanced monitoring system
func (epde *EnhancedPatternDiscoveryEngine) StartEnhancedMonitoring(ctx context.Context) error {
	if epde.monitor == nil {
		return fmt.Errorf("monitoring is not enabled")
	}
	return epde.monitor.StartMonitoring(ctx)
}

// StopEnhancedMonitoring stops the enhanced monitoring system
func (epde *EnhancedPatternDiscoveryEngine) StopEnhancedMonitoring() {
	if epde.monitor != nil {
		epde.monitor.StopMonitoring()
	}
}

// EnhancedDiscoverPatterns performs pattern discovery with comprehensive validation and monitoring
func (epde *EnhancedPatternDiscoveryEngine) EnhancedDiscoverPatterns(
	ctx context.Context,
	request *PatternAnalysisRequest,
) (*EnhancedPatternAnalysisResult, error) {

	startTime := time.Now()
	epde.log.WithFields(logrus.Fields{
		"analysis_type": request.AnalysisType,
		"pattern_types": request.PatternTypes,
		"enhanced_mode": true,
	}).Info("Starting enhanced pattern discovery analysis")

	// Pre-analysis data collection for validation
	historicalData, err := epde.collectHistoricalData(ctx, request)
	if err != nil {
		epde.recordAnalysisMetrics(time.Since(startTime), 0, 0.0, false)
		return nil, fmt.Errorf("failed to collect historical data: %w", err)
	}

	// Statistical validation (if enabled)
	var validationResults *StatisticalAssumptionResult
	if epde.config.EnableStatisticalValidation && epde.statisticalValidator != nil {
		validationResults = epde.statisticalValidator.ValidateStatisticalAssumptions(historicalData)

		// Check if validation requirements are met
		if epde.config.RequireValidationPassing && !validationResults.IsValid {
			epde.recordAnalysisMetrics(time.Since(startTime), 0, 0.0, false)
			return nil, fmt.Errorf("statistical validation failed: %s",
				validationResults.Recommendations)
		}

		epde.log.WithFields(logrus.Fields{
			"validation_passed":    validationResults.IsValid,
			"data_quality_score":   validationResults.DataQualityScore,
			"sample_size_adequate": validationResults.SampleSizeAdequate,
		}).Info("Statistical validation completed")
	}

	// Guideline #14: Use idiomatic patterns - call method directly without embedded field selector
	// Perform the core pattern discovery
	result, err := epde.DiscoverPatterns(ctx, request)
	if err != nil {
		epde.recordAnalysisMetrics(time.Since(startTime), 0, 0.0, false)
		return nil, fmt.Errorf("pattern discovery failed: %w", err)
	}

	// Reliability assessment
	var reliabilityAssessment *ReliabilityAssessment
	if epde.statisticalValidator != nil {
		reliabilityAssessment = epde.statisticalValidator.AssessReliability(historicalData)
	}

	// Overfitting assessment (if enabled)
	var overfittingAssessment *OverfittingAssessment
	if epde.config.EnableOverfittingPrevention && epde.overfittingPrevention != nil {
		// For overfitting assessment, we need ML model information
		// This would typically come from the ML analyzer in the base engine
		if epde.mlAnalyzer != nil && epde.mlAnalyzer.GetModelCount() > 0 {
			models := epde.mlAnalyzer.GetModels()
			for _, model := range models {
				// Assess overfitting risk using historical data and model information
				// Note: Cross-validation would be performed here if available in the ML analyzer
				assessment := epde.overfittingPrevention.AssessOverfittingRisk(
					historicalData, model, nil) // Pass nil for cross-validation metrics

				// Use the assessment with the highest risk score
				if overfittingAssessment == nil || assessment.RiskScore > overfittingAssessment.RiskScore {
					overfittingAssessment = assessment
				}
			}
		}

		// Check overfitting risk threshold
		if overfittingAssessment != nil &&
			overfittingAssessment.RiskScore > epde.config.MaxOverfittingRisk {
			epde.log.WithFields(logrus.Fields{
				"overfitting_risk": overfittingAssessment.OverfittingRisk,
				"risk_score":       overfittingAssessment.RiskScore,
				"threshold":        epde.config.MaxOverfittingRisk,
			}).Warn("High overfitting risk detected")
		}
	}

	// Calculate quality score
	qualityScore := epde.calculateQualityScore(
		result, validationResults, reliabilityAssessment, overfittingAssessment)

	// Generate enhanced result
	enhancedResult := &EnhancedPatternAnalysisResult{
		PatternAnalysisResult:  result,
		ValidationResults:      validationResults,
		OverfittingAssessment:  overfittingAssessment,
		ReliabilityAssessment:  reliabilityAssessment,
		QualityScore:           qualityScore,
		Warnings:               make([]string, 0),
		QualityRecommendations: make([]string, 0),
	}

	// Generate warnings and recommendations
	epde.generateWarningsAndRecommendations(enhancedResult)

	// Determine if results are production-ready
	enhancedResult.IsProductionReady = epde.isProductionReady(enhancedResult)

	// Record metrics for monitoring
	avgConfidence := epde.calculateAverageConfidence(result.Patterns)
	epde.recordAnalysisMetrics(time.Since(startTime), len(result.Patterns), avgConfidence, true)

	epde.log.WithFields(logrus.Fields{
		"patterns_found":   len(result.Patterns),
		"quality_score":    qualityScore,
		"production_ready": enhancedResult.IsProductionReady,
		"analysis_time":    time.Since(startTime),
		"warnings":         len(enhancedResult.Warnings),
	}).Info("Enhanced pattern discovery analysis completed")

	return enhancedResult, nil
}

// GetEngineHealth returns the current health status of the pattern engine
func (epde *EnhancedPatternDiscoveryEngine) GetEngineHealth() *EngineHealthStatus {
	if epde.monitor == nil {
		return &EngineHealthStatus{
			Overall:     HealthLevelUnknown,
			LastUpdated: time.Now(),
			Issues:      []*HealthIssue{},
		}
	}
	return epde.monitor.GetHealthStatus()
}

// GetEngineMetrics returns comprehensive metrics about the pattern engine
func (epde *EnhancedPatternDiscoveryEngine) GetEngineMetrics() *EngineMetrics {
	if epde.monitor == nil {
		return &EngineMetrics{Timestamp: time.Now()}
	}
	return epde.monitor.GetMetrics()
}

// GetActiveAlerts returns current alerts from the monitoring system
func (epde *EnhancedPatternDiscoveryEngine) GetActiveAlerts() []*EngineAlert {
	if epde.monitor == nil {
		return []*EngineAlert{}
	}
	return epde.monitor.GetAlerts(false) // Only active alerts
}

// ValidatePatternReliability validates a specific pattern's reliability
func (epde *EnhancedPatternDiscoveryEngine) ValidatePatternReliability(
	pattern *shared.DiscoveredPattern,
	validationData []*sharedtypes.WorkflowExecutionData,
) (*PatternReliabilityResult, error) {

	if epde.statisticalValidator == nil {
		return nil, fmt.Errorf("statistical validator not available")
	}

	result := &PatternReliabilityResult{
		PatternID:  pattern.ID,
		IsReliable: false,
		Confidence: pattern.Confidence,
		Timestamp:  time.Now(),
	}

	// Statistical validation
	assumptions := epde.statisticalValidator.ValidateStatisticalAssumptions(validationData)
	result.StatisticalValidation = assumptions

	// Reliability assessment
	reliability := epde.statisticalValidator.AssessReliability(validationData)
	result.ReliabilityScore = reliability.ReliabilityScore
	result.IsReliable = reliability.IsReliable

	// Generate confidence interval
	successCount := 0
	for _, data := range validationData {
		if data.Success {
			successCount++
		}
	}

	confidenceInterval := epde.statisticalValidator.CalculateConfidenceInterval(
		successCount, len(validationData), 0.95)
	result.ConfidenceInterval = confidenceInterval

	epde.log.WithFields(logrus.Fields{
		"pattern_id":        pattern.ID,
		"is_reliable":       result.IsReliable,
		"reliability_score": result.ReliabilityScore,
		"sample_size":       len(validationData),
	}).Info("Pattern reliability validation completed")

	return result, nil
}

// Private helper methods

func (epde *EnhancedPatternDiscoveryEngine) calculateQualityScore(
	result *PatternAnalysisResult,
	validation *StatisticalAssumptionResult,
	reliability *ReliabilityAssessment,
	overfitting *OverfittingAssessment,
) float64 {

	score := 0.0
	weights := map[string]float64{
		"pattern_quality": 0.3,
		"validation":      0.25,
		"reliability":     0.25,
		"overfitting":     0.2,
	}

	// Pattern quality component
	if len(result.Patterns) > 0 {
		avgConfidence := epde.calculateAverageConfidence(result.Patterns)
		score += weights["pattern_quality"] * avgConfidence
	}

	// Validation component
	if validation != nil {
		score += weights["validation"] * validation.OverallReliability
	} else {
		score += weights["validation"] * 0.5 // Neutral score if no validation
	}

	// Reliability component
	if reliability != nil {
		score += weights["reliability"] * reliability.ReliabilityScore
	} else {
		score += weights["reliability"] * 0.5
	}

	// Overfitting component (inverted - lower risk = higher score)
	if overfitting != nil {
		overfittingScore := 1.0 - overfitting.RiskScore
		score += weights["overfitting"] * overfittingScore
	} else {
		score += weights["overfitting"] * 0.7 // Assume moderate risk if unknown
	}

	return score
}

func (epde *EnhancedPatternDiscoveryEngine) generateWarningsAndRecommendations(result *EnhancedPatternAnalysisResult) {
	// Statistical validation warnings
	if result.ValidationResults != nil && !result.ValidationResults.IsValid {
		result.Warnings = append(result.Warnings, "Statistical assumptions not met")
		result.QualityRecommendations = append(result.QualityRecommendations,
			result.ValidationResults.Recommendations...)
	}

	// Sample size warnings
	if result.ValidationResults != nil && !result.ValidationResults.SampleSizeAdequate {
		result.Warnings = append(result.Warnings, "Insufficient sample size for reliable analysis")
	}

	// Reliability warnings
	if result.ReliabilityAssessment != nil && !result.ReliabilityAssessment.IsReliable {
		result.Warnings = append(result.Warnings, "Pattern reliability below threshold")
		result.QualityRecommendations = append(result.QualityRecommendations,
			result.ReliabilityAssessment.Recommendations...)
	}

	// Overfitting warnings
	if result.OverfittingAssessment != nil {
		if result.OverfittingAssessment.OverfittingRisk == OverfittingRiskHigh ||
			result.OverfittingAssessment.OverfittingRisk == OverfittingRiskCritical {
			result.Warnings = append(result.Warnings, "High overfitting risk detected")
			result.QualityRecommendations = append(result.QualityRecommendations,
				result.OverfittingAssessment.Recommendations...)
		}
	}

	// Quality score warnings
	if result.QualityScore < epde.config.MinReliabilityScore {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Quality score %.2f below minimum threshold %.2f",
				result.QualityScore, epde.config.MinReliabilityScore))
	}
}

func (epde *EnhancedPatternDiscoveryEngine) isProductionReady(result *EnhancedPatternAnalysisResult) bool {
	// Check minimum quality score
	if result.QualityScore < epde.config.MinReliabilityScore {
		return false
	}

	// Check reliability if available
	if result.ReliabilityAssessment != nil && !result.ReliabilityAssessment.IsReliable {
		return false
	}

	// Check overfitting risk
	if result.OverfittingAssessment != nil &&
		result.OverfittingAssessment.RiskScore > epde.config.MaxOverfittingRisk {
		return false
	}

	// Check if validation is required and passed
	if epde.config.RequireValidationPassing &&
		(result.ValidationResults == nil || !result.ValidationResults.IsValid) {
		return false
	}

	// Check if we have sufficient patterns
	if len(result.Patterns) == 0 {
		return false
	}

	return true
}

func (epde *EnhancedPatternDiscoveryEngine) calculateAverageConfidence(patterns []*shared.DiscoveredPattern) float64 {
	if len(patterns) == 0 {
		return 0.0
	}

	total := 0.0
	for _, pattern := range patterns {
		total += pattern.Confidence
	}

	return total / float64(len(patterns))
}

func (epde *EnhancedPatternDiscoveryEngine) recordAnalysisMetrics(
	duration time.Duration,
	patternsFound int,
	avgConfidence float64,
	success bool,
) {
	if epde.monitor != nil {
		epde.monitor.RecordAnalysisMetrics(duration, patternsFound, avgConfidence, success)
	}
}

// PatternReliabilityResult contains the result of pattern reliability validation
type PatternReliabilityResult struct {
	PatternID             string                         `json:"pattern_id"`
	IsReliable            bool                           `json:"is_reliable"`
	ReliabilityScore      float64                        `json:"reliability_score"`
	Confidence            float64                        `json:"confidence"`
	StatisticalValidation *StatisticalAssumptionResult   `json:"statistical_validation"`
	ConfidenceInterval    *StatisticalConfidenceInterval `json:"confidence_interval"`
	Timestamp             time.Time                      `json:"timestamp"`
}
