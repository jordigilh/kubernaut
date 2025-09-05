package patterns

import (
	"time"
)

// Common types used across pattern discovery components

// Range represents a numerical range
type Range struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// ResourceUsage represents resource utilization metrics
type ResourceUsage struct {
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  float64 `json:"memory_usage"`
	NetworkUsage float64 `json:"network_usage"`
	StorageUsage float64 `json:"storage_usage"`
}

// PatternTimeRange represents a time range for pattern analysis
type PatternTimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Contains checks if a time is within the range
func (tr PatternTimeRange) Contains(t time.Time) bool {
	return t.After(tr.Start) && t.Before(tr.End)
}

// PatternDiscoveryRepository interface (simplified for now)
type PatternDiscoveryRepository interface {
	// Mock methods for now - these would be implemented based on actual repository
	GetHistoricalData(ctx interface{}, request interface{}) (interface{}, error)
	StorePatternAnalysis(ctx interface{}, analysis interface{}) error
}

// PatternValidationRule represents a validation rule for pattern features
type PatternValidationRule struct {
	Name          string                           `json:"name"`
	Type          string                           `json:"type"` // "range", "categorical", "format"
	MinValue      *float64                         `json:"min_value,omitempty"`
	MaxValue      *float64                         `json:"max_value,omitempty"`
	AllowedValues []interface{}                    `json:"allowed_values,omitempty"`
	ValidateFunc  func(interface{}) (bool, string) `json:"-"`
	Required      bool                             `json:"required"`
}

// PatternValidationResult represents pattern validation test results
type PatternValidationResult struct {
	ValidationID      string             `json:"validation_id"`
	TestType          string             `json:"test_type"`
	Timestamp         time.Time          `json:"timestamp"`
	TestAccuracy      float64            `json:"test_accuracy"`
	TrainAccuracy     float64            `json:"train_accuracy"`
	GeneralizationGap float64            `json:"generalization_gap"`
	OverfittingScore  float64            `json:"overfitting_score"`
	TestMetrics       *AccuracyMetrics   `json:"test_metrics"`
	Recommendations   []string           `json:"recommendations"`
	QualityIndicators map[string]float64 `json:"quality_indicators"`
}

// AccuracyMetrics represents comprehensive accuracy measurements
type AccuracyMetrics struct {
	// Basic Metrics
	Accuracy    float64 `json:"accuracy"`
	Precision   float64 `json:"precision"`
	Recall      float64 `json:"recall"`
	F1Score     float64 `json:"f1_score"`
	Specificity float64 `json:"specificity"`

	// Advanced Metrics
	BalancedAccuracy    float64 `json:"balanced_accuracy"`
	MatthewsCorrelation float64 `json:"matthews_correlation"`
	CohenKappa          float64 `json:"cohen_kappa"`
	AUC                 float64 `json:"auc"`

	// Confusion Matrix
	TruePositives  int `json:"true_positives"`
	TrueNegatives  int `json:"true_negatives"`
	FalsePositives int `json:"false_positives"`
	FalseNegatives int `json:"false_negatives"`

	// Confidence Intervals (95%)
	AccuracyCI  Range `json:"accuracy_ci"`
	PrecisionCI Range `json:"precision_ci"`
	RecallCI    Range `json:"recall_ci"`

	// Temporal Metrics
	CalculatedAt time.Time     `json:"calculated_at"`
	SampleSize   int           `json:"sample_size"`
	WindowSize   time.Duration `json:"window_size"`
}

// PatternPerformanceAnalysis analyzes pattern performance characteristics
type PatternPerformanceAnalysis struct {
	ConsistencyScore      float64               `json:"consistency_score"`
	StabilityScore        float64               `json:"stability_score"`
	RobustnessScore       float64               `json:"robustness_score"`
	PerformanceSegments   map[string]float64    `json:"performance_segments"`
	PerformanceAnomalies  []*PerformanceAnomaly `json:"performance_anomalies"`
	OptimalOperatingRange *OperatingRange       `json:"optimal_operating_range"`
}

// PatternValidationSummary summarizes pattern validation test results
type PatternValidationSummary struct {
	TotalValidations         int                        `json:"total_validations"`
	PassedValidations        int                        `json:"passed_validations"`
	ValidationSuccessRate    float64                    `json:"validation_success_rate"`
	AverageGeneralizationGap float64                    `json:"average_generalization_gap"`
	OverfittingRisk          string                     `json:"overfitting_risk"` // "low", "medium", "high"
	ValidationHistory        []*PatternValidationResult `json:"validation_history"`
}

// PatternQualityAssessment provides overall pattern quality assessment
type PatternQualityAssessment struct {
	OverallGrade       string             `json:"overall_grade"` // A, B, C, D, F
	QualityScore       float64            `json:"quality_score"`
	QualityFactors     map[string]float64 `json:"quality_factors"`
	CriticalIssues     []string           `json:"critical_issues"`
	QualityTrend       string             `json:"quality_trend"`
	RecommendedActions []string           `json:"recommended_actions"`
}

// PerformanceAnomaly represents detected performance anomalies
type PerformanceAnomaly struct {
	DetectedAt         time.Time     `json:"detected_at"`
	AnomalyType        string        `json:"anomaly_type"`
	Severity           string        `json:"severity"`
	Description        string        `json:"description"`
	AffectedMetric     string        `json:"affected_metric"`
	BaselineValue      float64       `json:"baseline_value"`
	ObservedValue      float64       `json:"observed_value"`
	DeviationMagnitude float64       `json:"deviation_magnitude"`
	Duration           time.Duration `json:"duration"`
	ResolutionStatus   string        `json:"resolution_status"`
}

// OperatingRange defines optimal operating conditions for pattern
type OperatingRange struct {
	ConfidenceRange    Range              `json:"confidence_range"`
	DataQualityRange   Range              `json:"data_quality_range"`
	SampleSizeRange    Range              `json:"sample_size_range"`
	OptimalConditions  []string           `json:"optimal_conditions"`
	PerformanceMetrics map[string]float64 `json:"performance_metrics"`
}

// Additional helper types that might be missing

// PatternAccuracyTracker interface to avoid redeclaration
type IPatternAccuracyTracker interface {
	TrackPrediction(ctx interface{}, patternID string, prediction interface{}) error
	GenerateAccuracyReport(ctx interface{}, patternID string, analysisWindow time.Duration) (interface{}, error)
	ValidateAccuracy(ctx interface{}, patternID string, validationType string) (interface{}, error)
}
