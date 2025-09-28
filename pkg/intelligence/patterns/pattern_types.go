package patterns

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Common types shared across pattern discovery components
// Business Requirement: BR-PATTERN-001 - Pattern discovery and analysis for workflow optimization

// LearningMetrics tracks learning performance over time
// Business Requirement: BR-PATTERN-002 - Learning metrics collection and analysis
type LearningMetrics struct {
	TotalAnalyses      int                       `json:"total_analyses"`
	TotalExecutions    int                       `json:"total_executions"`
	PatternsDiscovered int                       `json:"patterns_discovered"`
	AverageConfidence  float64                   `json:"average_confidence"`
	LearningRate       float64                   `json:"learning_rate"`
	LastUpdated        time.Time                 `json:"last_updated"`
	PerformanceMetrics map[string]float64        `json:"performance_metrics"`
	PatternTrackers    []*PatternAccuracyTracker `json:"pattern_trackers"`
}

// NewLearningMetrics creates a new learning metrics instance
func NewLearningMetrics() *LearningMetrics {
	return &LearningMetrics{
		PerformanceMetrics: make(map[string]float64),
		PatternTrackers:    make([]*PatternAccuracyTracker, 0),
		LastUpdated:        time.Now(),
	}
}

// RecordAnalysis records a pattern analysis
func (lm *LearningMetrics) RecordAnalysis(result *PatternAnalysisResult) {
	lm.TotalAnalyses++
	lm.PatternsDiscovered += len(result.Patterns)

	// Calculate average confidence
	if len(result.Patterns) > 0 {
		totalConfidence := 0.0
		for _, pattern := range result.Patterns {
			totalConfidence += pattern.Confidence
		}
		avgConfidence := totalConfidence / float64(len(result.Patterns))

		// Update running average
		if lm.TotalAnalyses == 1 {
			lm.AverageConfidence = avgConfidence
		} else {
			lm.AverageConfidence = (lm.AverageConfidence*float64(lm.TotalAnalyses-1) + avgConfidence) / float64(lm.TotalAnalyses)
		}
	}

	lm.LastUpdated = time.Now()
}

// RecordExecution records a workflow execution
func (lm *LearningMetrics) RecordExecution(execution *sharedtypes.WorkflowExecutionRecord) {
	lm.TotalExecutions++
	lm.LastUpdated = time.Now()
}

// AlertCorrelation represents correlation between alerts
type AlertCorrelation struct {
	PrimaryAlert     string        `json:"primary_alert"`
	CorrelatedAlerts []string      `json:"correlated_alerts"`
	CorrelationScore float64       `json:"correlation_score"`
	TimeWindow       time.Duration `json:"time_window"`
	Direction        string        `json:"direction"` // "causes", "follows", "concurrent"
	Confidence       float64       `json:"confidence"`
}

// FailureNode represents a node in a failure chain
type FailureNode struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Component    string                 `json:"component"`
	FailureTime  time.Time              `json:"failure_time"`
	RecoveryTime *time.Time             `json:"recovery_time,omitempty"`
	Impact       string                 `json:"impact"`
	RootCause    bool                   `json:"root_cause"`
	Dependencies []string               `json:"dependencies"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// RecoveryPattern represents how systems recover from failures
type RecoveryPattern struct {
	ID              string        `json:"id"`
	FailureType     string        `json:"failure_type"`
	RecoverySteps   []string      `json:"recovery_steps"`
	AverageTime     time.Duration `json:"average_time"`
	SuccessRate     float64       `json:"success_rate"`
	AutomationLevel float64       `json:"automation_level"`
	Prerequisites   []string      `json:"prerequisites"`
	Effectiveness   float64       `json:"effectiveness"`
}

// AnalysisMetrics provides metrics about pattern analysis
type AnalysisMetrics struct {
	DataPointsAnalyzed      int            `json:"data_points_analyzed"`
	PatternsFound           int            `json:"patterns_found"`
	AnalysisTime            time.Duration  `json:"analysis_time"`
	ConfidenceDistribution  map[string]int `json:"confidence_distribution"`
	PatternTypeDistribution map[string]int `json:"pattern_type_distribution"`
	CoveragePercentage      float64        `json:"coverage_percentage"`
	QualityScore            float64        `json:"quality_score"`
}

// PatternRecommendation suggests actions based on discovered patterns
type PatternRecommendation struct {
	ID               string               `json:"id"`
	Type             string               `json:"type"`
	Title            string               `json:"title"`
	Description      string               `json:"description"`
	Impact           string               `json:"impact"`
	Effort           string               `json:"effort"`
	Priority         int                  `json:"priority"`
	BasedOnPatterns  []string             `json:"based_on_patterns"`
	Implementation   *ImplementationGuide `json:"implementation"`
	EstimatedBenefit float64              `json:"estimated_benefit"`
}

// ImplementationGuide provides guidance for implementing recommendations
type ImplementationGuide struct {
	Steps           []string      `json:"steps"`
	Prerequisites   []string      `json:"prerequisites"`
	EstimatedTime   time.Duration `json:"estimated_time"`
	RequiredSkills  []string      `json:"required_skills"`
	RiskLevel       string        `json:"risk_level"`
	TestingGuidance string        `json:"testing_guidance"`
}

// OptimizedWorkflowTemplate represents an optimized workflow template
type OptimizedWorkflowTemplate struct {
	OriginalTemplate  *sharedtypes.TemplateSpec `json:"original_template"`
	OptimizedTemplate *sharedtypes.TemplateSpec `json:"optimized_template"`
	Optimizations     []*TemplateOptimization   `json:"optimizations"`
	ImpactEstimate    *OptimizationImpact       `json:"impact_estimate"`
	ConfidenceScore   float64                   `json:"confidence_score"`
	RecommendedFor    []string                  `json:"recommended_for"`
}

// TemplateOptimization represents a specific optimization
type TemplateOptimization struct {
	ID                  string      `json:"id"`
	Type                string      `json:"type"`
	Description         string      `json:"description"`
	OptimizedStep       string      `json:"optimized_step,omitempty"`
	OriginalValue       interface{} `json:"original_value"`
	OptimizedValue      interface{} `json:"optimized_value"`
	Rationale           string      `json:"rationale"`
	ExpectedImprovement float64     `json:"expected_improvement"`
	RiskLevel           string      `json:"risk_level"`
}

// OptimizationImpact estimates the impact of optimizations
type OptimizationImpact struct {
	PerformanceGain      float64       `json:"performance_gain"`
	ResourceSavings      float64       `json:"resource_savings"`
	ReliabilityGain      float64       `json:"reliability_gain"`
	MaintenanceReduction float64       `json:"maintenance_reduction"`
	EstimatedROI         float64       `json:"estimated_roi"`
	PaybackPeriod        time.Duration `json:"payback_period"`
}

// PatternInsights provides comprehensive insights about discovered patterns
type PatternInsights struct {
	TotalPatterns       int                    `json:"total_patterns"`
	PatternDistribution map[string]int         `json:"pattern_distribution"`
	ConfidenceStats     *ConfidenceStatistics  `json:"confidence_stats"`
	TemporalTrends      *TemporalTrendAnalysis `json:"temporal_trends"`
	TopOptimizations    []*OptimizationInsight `json:"top_optimizations"`
	LearningMetrics     *LearningMetrics       `json:"learning_metrics"`
	RecommendedActions  []string               `json:"recommended_actions"`
	QualityAssessment   *QualityAssessment     `json:"quality_assessment"`
}

// ConfidenceStatistics provides statistics about pattern confidence
type ConfidenceStatistics struct {
	Mean                float64 `json:"mean"`
	Median              float64 `json:"median"`
	StandardDeviation   float64 `json:"standard_deviation"`
	Min                 float64 `json:"min"`
	Max                 float64 `json:"max"`
	HighConfidenceCount int     `json:"high_confidence_count"`
	LowConfidenceCount  int     `json:"low_confidence_count"`
}

// TemporalTrendAnalysis analyzes trends over time
type TemporalTrendAnalysis struct {
	OverallTrend     string      `json:"overall_trend"`
	TrendStrength    float64     `json:"trend_strength"`
	SeasonalPatterns []string    `json:"seasonal_patterns"`
	AnomalousPerIODS []time.Time `json:"anomalous_periods"`
	ForecastAccuracy float64     `json:"forecast_accuracy"`
	TrendConfidence  float64     `json:"trend_confidence"`
}

// OptimizationInsight provides insights about optimization opportunities
type OptimizationInsight struct {
	Area                     string  `json:"area"`
	PotentialImprovement     float64 `json:"potential_improvement"`
	ImplementationDifficulty string  `json:"implementation_difficulty"`
	Priority                 int     `json:"priority"`
	AffectedWorkflows        int     `json:"affected_workflows"`
	EstimatedROI             float64 `json:"estimated_roi"`
}

// QualityAssessment assesses the quality of discovered patterns
type QualityAssessment struct {
	OverallQuality     float64            `json:"overall_quality"`
	DataQuality        float64            `json:"data_quality"`
	PatternReliability float64            `json:"pattern_reliability"`
	CoverageScore      float64            `json:"coverage_score"`
	NoveltyScore       float64            `json:"novelty_score"`
	ActionabilityScore float64            `json:"actionability_score"`
	QualityFactors     map[string]float64 `json:"quality_factors"`
}

// VectorSearchResult is now defined as a type alias in pattern_discovery_engine.go
// for integration with the unified vector database interface

// PatternWorkflowExecution represents a historical workflow execution for pattern analysis
type PatternWorkflowExecution struct {
	ID               string                         `json:"id"`
	TemplateID       string                         `json:"template_id"`
	WorkflowID       string                         `json:"workflow_id"`
	Success          bool                           `json:"success"`
	StartTime        time.Time                      `json:"start_time"`
	EndTime          time.Time                      `json:"end_time"`
	Duration         time.Duration                  `json:"duration"`
	StepsExecuted    int                            `json:"steps_executed"`
	StepResults      []*StepExecutionResult         `json:"step_results"`
	FinalState       map[string]interface{}         `json:"final_state"`
	ErrorMessage     string                         `json:"error_message,omitempty"`
	ExecutionContext map[string]interface{}         `json:"execution_context"`
	ResourceUsage    *sharedtypes.ResourceUsageData `json:"resource_usage,omitempty"`
	Metrics          map[string]float64             `json:"metrics"`
}

// StepExecutionResult represents the result of executing a workflow step
type StepExecutionResult struct {
	StepID           string                 `json:"step_id"`
	StepName         string                 `json:"step_name"`
	Success          bool                   `json:"success"`
	StartTime        time.Time              `json:"start_time"`
	EndTime          time.Time              `json:"end_time"`
	Duration         time.Duration          `json:"duration"`
	Output           map[string]interface{} `json:"output"`
	ErrorMessage     string                 `json:"error_message,omitempty"`
	ResourcesChanged []string               `json:"resources_changed"`
	Warnings         []string               `json:"warnings"`
}

// WorkflowLearningData represents data used for learning from workflow executions
type WorkflowLearningData struct {
	ExecutionID       string                                     `json:"execution_id"`
	TemplateID        string                                     `json:"template_id"`
	Features          *shared.WorkflowFeatures                   `json:"features"`
	ExecutionResult   *sharedtypes.SharedWorkflowExecutionResult `json:"execution_result"`
	ResourceUsage     *sharedtypes.ResourceUsageData             `json:"resource_usage"`
	Context           map[string]interface{}                     `json:"context"`
	LearningObjective string                                     `json:"learning_objective"`
	Feedback          *LearningFeedback                          `json:"feedback,omitempty"`
}

// LearningFeedback provides feedback for learning algorithms
type LearningFeedback struct {
	CorrectPrediction bool                   `json:"correct_prediction"`
	ActualOutcome     string                 `json:"actual_outcome"`
	PredictedOutcome  string                 `json:"predicted_outcome"`
	FeedbackType      string                 `json:"feedback_type"`
	ConfidenceScore   float64                `json:"confidence_score"`
	Improvements      []string               `json:"improvements"`
	Labels            map[string]interface{} `json:"labels"`
}

// Pattern analysis result types for insights and effectiveness assessment

// ActionSequencePattern represents patterns in action sequences
type ActionSequencePattern struct {
	Actions       []string      `json:"actions"`
	Frequency     int           `json:"frequency"`
	SuccessRate   float64       `json:"success_rate"`
	AvgDuration   time.Duration `json:"avg_duration"`
	ResourceType  string        `json:"resource_type"`
	Effectiveness float64       `json:"effectiveness"`
}

// ResourcePattern represents patterns by resource type
type ResourcePattern struct {
	ResourceName     string        `json:"resource_name"`
	ActionCount      int           `json:"action_count"`
	SuccessRate      float64       `json:"success_rate"`
	AvgEffectiveness float64       `json:"avg_effectiveness"`
	AvgDuration      time.Duration `json:"avg_duration"`
	CommonActions    []string      `json:"common_actions"`
}

// TemporalPattern represents temporal patterns in actions
type TemporalPattern struct {
	Type             string  `json:"type"`
	Description      string  `json:"description"`
	ActionCount      int     `json:"action_count"`
	SuccessRate      float64 `json:"success_rate"`
	AvgEffectiveness float64 `json:"avg_effectiveness"`
}

// EffectivenessPattern represents effectiveness patterns
type EffectivenessPattern struct {
	Action                string  `json:"action"`
	AvgEffectiveness      float64 `json:"avg_effectiveness"`
	Count                 int     `json:"count"`
	HighEffectivenessRate float64 `json:"high_effectiveness_rate"`
	LowEffectivenessRate  float64 `json:"low_effectiveness_rate"`
}

// OscillationAnalysisPattern represents oscillation patterns for analysis
type OscillationAnalysisPattern struct {
	ResourceName        string        `json:"resource_name"`
	Pattern             string        `json:"pattern"`
	Frequency           int           `json:"frequency"`
	AvgInterval         time.Duration `json:"avg_interval"`
	EffectivenessImpact float64       `json:"effectiveness_impact"`
}

// SuccessPattern represents success and failure patterns
type SuccessPattern struct {
	Action           string  `json:"action"`
	SuccessRate      float64 `json:"success_rate"`
	TotalExecutions  int     `json:"total_executions"`
	AvgEffectiveness float64 `json:"avg_effectiveness"`
}
