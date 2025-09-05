package common

import (
	"context"
	"time"
)

// Analytics and Assessment related interfaces and types
type AssessmentProcessor interface {
	ProcessPendingAssessments(ctx context.Context) error
	GetAnalyticsInsights(ctx context.Context) (*AnalyticsInsights, error)
	GetPatternAnalytics(ctx context.Context) (interface{}, error)
	TrainModels(ctx context.Context) error
}

type EnhancedAssessorInterface interface {
	AssessmentProcessor
	AssessActionEffectiveness(ctx context.Context, trace interface{}) (*EnhancedEffectivenessResult, error)
}

// Analytics related types
type AnalyticsInsights struct {
	OverallEffectiveness      *OverallEffectivenessAnalysis  `json:"overall_effectiveness"`
	ActionTypeAnalysis        map[string]*ActionTypeAnalysis `json:"action_type_analysis"`
	SeasonalPatterns          *SeasonalAnalysis              `json:"seasonal_patterns"`
	CorrelationAnalysis       *CorrelationAnalysis           `json:"correlation_analysis"`
	AnomalyDetection          *AnomalyAnalysis               `json:"anomaly_detection"`
	TrendAnalysis             *TrendAnalysis                 `json:"trend_analysis"`
	CostEffectivenessAnalysis *CostAnalysis                  `json:"cost_effectiveness_analysis"`
	Recommendations           []string                       `json:"recommendations"`
	GeneratedAt               time.Time                      `json:"generated_at"`
	DataWindow                *TimeRange                     `json:"data_window"`
}

type OverallEffectivenessAnalysis struct {
	Mean                     float64  `json:"mean"`
	Median                   float64  `json:"median"`
	StandardDeviation        float64  `json:"standard_deviation"`
	SuccessRate              float64  `json:"success_rate"`
	TotalActions             int      `json:"total_actions"`
	TopPerformingActionTypes []string `json:"top_performing_action_types"`
	BottomPerformingTypes    []string `json:"bottom_performing_types"`
	TrendDirection           string   `json:"trend_direction"`
	ConfidenceLevel          float64  `json:"confidence_level"`
}

type ActionTypeAnalysis struct {
	ActionType           string                 `json:"action_type"`
	TotalExecutions      int                    `json:"total_executions"`
	SuccessRate          float64                `json:"success_rate"`
	AverageEffectiveness float64                `json:"average_effectiveness"`
	EffectivenessRange   *StatisticalRange      `json:"effectiveness_range"`
	CommonParameters     map[string]interface{} `json:"common_parameters"`
	TopPerformers        []string               `json:"top_performers"`
	FailurePatterns      []string               `json:"failure_patterns"`
	Recommendations      []string               `json:"recommendations"`
}

type SeasonalAnalysis struct {
	HourlyPatterns    map[int]float64    `json:"hourly_patterns"`
	DailyPatterns     map[string]float64 `json:"daily_patterns"`
	WeeklyPatterns    map[int]float64    `json:"weekly_patterns"`
	MonthlyPatterns   map[int]float64    `json:"monthly_patterns"`
	SeasonalTrends    []SeasonalTrend    `json:"seasonal_trends"`
	PeakPerformance   TimeWindow         `json:"peak_performance"`
	LowestPerformance TimeWindow         `json:"lowest_performance"`
}

type SeasonalTrend struct {
	Period     string  `json:"period"`
	Trend      string  `json:"trend"`
	Strength   float64 `json:"strength"`
	Confidence float64 `json:"confidence"`
}

type CorrelationAnalysis struct {
	FeatureCorrelations map[string]float64            `json:"feature_correlations"`
	StrongestPositive   []CorrelationPair             `json:"strongest_positive"`
	StrongestNegative   []CorrelationPair             `json:"strongest_negative"`
	SignificantPairs    []CorrelationPair             `json:"significant_pairs"`
	CorrelationMatrix   map[string]map[string]float64 `json:"correlation_matrix"`
}

type CorrelationPair struct {
	Feature1     string  `json:"feature1"`
	Feature2     string  `json:"feature2"`
	Correlation  float64 `json:"correlation"`
	Significance float64 `json:"significance"`
}

type AnomalyAnalysis struct {
	AnomalyThreshold     float64                 `json:"anomaly_threshold"`
	DetectedAnomalies    []*EffectivenessAnomaly `json:"detected_anomalies"`
	AnomalyScore         float64                 `json:"anomaly_score"`
	AnomalyCount         int                     `json:"anomaly_count"`
	SeverityDistribution map[string]int          `json:"severity_distribution"`
}

type EffectivenessAnomaly struct {
	PatternID    string                 `json:"pattern_id"`
	ActionType   string                 `json:"action_type"`
	AlertName    string                 `json:"alert_name"`
	Timestamp    time.Time              `json:"timestamp"`
	Severity     string                 `json:"severity"`
	AnomalyScore float64                `json:"anomaly_score"`
	Description  string                 `json:"description"`
	Context      map[string]interface{} `json:"context"`
}

type TrendAnalysis struct {
	OverallTrend        string                  `json:"overall_trend"` // "improving", "declining", "stable"
	TrendStrength       float64                 `json:"trend_strength"`
	ActionTypeTrends    map[string]*ActionTrend `json:"action_type_trends"`
	ForecastedDirection string                  `json:"forecasted_direction"`
	TrendConfidence     float64                 `json:"trend_confidence"`
	ChangePoints        []ChangePoint           `json:"change_points"`
}

type ActionTrend struct {
	ActionType     string    `json:"action_type"`
	TrendDirection string    `json:"trend_direction"`
	TrendStrength  float64   `json:"trend_strength"`
	Confidence     float64   `json:"confidence"`
	StartDate      time.Time `json:"start_date"`
	EndDate        time.Time `json:"end_date"`
}

type ChangePoint struct {
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
	Impact      float64   `json:"impact"`
	Confidence  float64   `json:"confidence"`
}

type CostAnalysis struct {
	TotalCostSavings float64               `json:"total_cost_savings"`
	CostPerAction    map[string]float64    `json:"cost_per_action"`
	ROIByActionType  map[string]float64    `json:"roi_by_action_type"`
	CostTrends       []CostTrend           `json:"cost_trends"`
	TopCostSavers    []CostEffectiveAction `json:"top_cost_savers"`
	CostProjection   *CostProjection       `json:"cost_projection"`
}

type CostTrend struct {
	Period      string  `json:"period"`
	Savings     float64 `json:"savings"`
	ActionCount int     `json:"action_count"`
}

type CostEffectiveAction struct {
	ActionType       string  `json:"action_type"`
	TotalSavings     float64 `json:"total_savings"`
	ExecutionCount   int     `json:"execution_count"`
	AverageSavings   float64 `json:"average_savings"`
	EfficiencyRating float64 `json:"efficiency_rating"`
}

type CostProjection struct {
	ProjectedSavings float64       `json:"projected_savings"`
	Confidence       float64       `json:"confidence"`
	TimeHorizon      time.Duration `json:"time_horizon"`
	Assumptions      []string      `json:"assumptions"`
}

// Enhanced Effectiveness types
type EnhancedEffectivenessResult struct {
	TraditionalScore     float64                  `json:"traditional_score"`
	EnhancedScore        float64                  `json:"enhanced_score"`
	ConfidenceLevel      float64                  `json:"confidence_level"`
	LearningContribution float64                  `json:"learning_contribution"`
	PatternAnalysis      *PatternAnalysisResult   `json:"pattern_analysis"`
	PredictiveInsights   *PredictiveInsightResult `json:"predictive_insights"`
	CostAnalysis         *CostAnalysisResult      `json:"cost_analysis"`
	Recommendations      []string                 `json:"recommendations"`
	ProcessingTime       time.Duration            `json:"processing_time"`
	GeneratedAt          time.Time                `json:"generated_at"`
}

type PatternAnalysisResult struct {
	PatternID          string        `json:"pattern_id"`
	MatchingSimilarity float64       `json:"matching_similarity"`
	HistoricalSuccess  float64       `json:"historical_success"`
	SimilarPatterns    []interface{} `json:"similar_patterns"`
	PatternConfidence  float64       `json:"pattern_confidence"`
	PatternType        string        `json:"pattern_type"`
}

type PredictiveInsightResult struct {
	PredictedEffectiveness float64                `json:"predicted_effectiveness"`
	ModelConfidence        float64                `json:"model_confidence"`
	PredictionFactors      map[string]float64     `json:"prediction_factors"`
	RiskFactors            []string               `json:"risk_factors"`
	AlternativeActions     []string               `json:"alternative_actions"`
	AnomalyScore           float64                `json:"anomaly_score"`
	TrendAnalysis          map[string]interface{} `json:"trend_analysis"`
}

type CostAnalysisResult struct {
	EstimatedCost        float64            `json:"estimated_cost"`
	ExpectedSavings      float64            `json:"expected_savings"`
	ROI                  float64            `json:"roi"`
	CostEfficiencyRating float64            `json:"cost_efficiency_rating"`
	AlternativeCosts     map[string]float64 `json:"alternative_costs"`
	ROIProjection        *ROIProjection     `json:"roi_projection"`
}

type ROIProjection struct {
	ProjectedROI  float64       `json:"projected_roi"`
	TimeToPayback time.Duration `json:"time_to_payback"`
	Confidence    float64       `json:"confidence"`
	RiskFactors   []string      `json:"risk_factors"`
}

// Pattern Analytics types
type PatternAnalytics struct {
	TotalPatterns        int                    `json:"total_patterns"`
	AverageEffectiveness float64                `json:"average_effectiveness"`
	PatternsByType       map[string]int         `json:"patterns_by_type"`
	SuccessRateByType    map[string]float64     `json:"success_rate_by_type"`
	RecentPatterns       []interface{}          `json:"recent_patterns"`
	TopPerformers        []interface{}          `json:"top_performers"`
	FailurePatterns      []interface{}          `json:"failure_patterns"`
	TrendAnalysis        map[string]interface{} `json:"trend_analysis"`
}

// Assessment Metrics
type AssessmentMetrics struct {
	TotalAssessments          int            `json:"total_assessments"`
	SuccessfulAssessments     int            `json:"successful_assessments"`
	FailedAssessments         int            `json:"failed_assessments"`
	AverageAssessmentDuration time.Duration  `json:"average_assessment_duration"`
	ErrorsByComponent         map[string]int `json:"errors_by_component"`
	ErrorsBySeverity          map[string]int `json:"errors_by_severity"`
	PatternLearningUsage      int            `json:"pattern_learning_usage"`
	PredictiveAnalysisUsage   int            `json:"predictive_analysis_usage"`
	CostAnalysisUsage         int            `json:"cost_analysis_usage"`
	VectorDBQueryCount        int            `json:"vector_db_query_count"`
	AnalyticsEngineCallCount  int            `json:"analytics_engine_call_count"`
	AverageConfidenceLevel    float64        `json:"average_confidence_level"`
	AverageEnhancedScore      float64        `json:"average_enhanced_score"`
	LastUpdated               time.Time      `json:"last_updated"`
}

// Effectiveness forecast types
type EffectivenessForecast struct {
	PredictedMean   float64           `json:"predicted_mean"`
	ConfidenceRange *StatisticalRange `json:"confidence_range"`
	Horizon         time.Duration     `json:"horizon"`
}
