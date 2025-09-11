package shared

import (
	"time"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/types"
)

// WorkflowFeatures represents extracted features from workflow data
type WorkflowFeatures struct {
	// Alert Features
	AlertCount    int            `json:"alert_count"`
	AlertTypes    map[string]int `json:"alert_types"`
	SeverityScore float64        `json:"severity_score"`
	AlertDuration time.Duration  `json:"alert_duration"`

	// Resource Features
	ResourceCount  int            `json:"resource_count"`
	ResourceTypes  map[string]int `json:"resource_types"`
	NamespaceCount int            `json:"namespace_count"`

	// Temporal Features
	HourOfDay      int  `json:"hour_of_day"`
	DayOfWeek      int  `json:"day_of_week"`
	IsWeekend      bool `json:"is_weekend"`
	IsBusinessHour bool `json:"is_business_hour"`

	// Historical Features
	RecentFailures     int           `json:"recent_failures"`
	AverageSuccessRate float64       `json:"average_success_rate"`
	LastExecutionTime  time.Duration `json:"last_execution_time"`

	// Complexity Features
	StepCount       int `json:"step_count"`
	DependencyDepth int `json:"dependency_depth"`
	ParallelSteps   int `json:"parallel_steps"`

	// Environment Features
	ClusterSize      int     `json:"cluster_size"`
	ClusterLoad      float64 `json:"cluster_load"`
	ResourcePressure float64 `json:"resource_pressure"`

	// Custom Features
	CustomMetrics map[string]float64 `json:"custom_metrics"`
}

// WorkflowPrediction contains prediction results
type WorkflowPrediction struct {
	SuccessProbability      float64                               `json:"success_probability"`
	ExpectedDuration        time.Duration                         `json:"expected_duration"`
	ResourceRequirements    *ResourcePrediction                   `json:"resource_requirements"`
	Confidence              float64                               `json:"confidence"`
	RiskFactors             []*types.RiskFactor                   `json:"risk_factors"`
	OptimizationSuggestions []*sharedtypes.OptimizationSuggestion `json:"optimization_suggestions"`
	SimilarPatterns         int                                   `json:"similar_patterns"`
	Reason                  string                                `json:"reason"`
}

// ResourcePrediction predicts resource usage
type ResourcePrediction struct {
	CPUUsage     float64                 `json:"cpu_usage"`
	MemoryUsage  float64                 `json:"memory_usage"`
	NetworkUsage float64                 `json:"network_usage"`
	StorageUsage float64                 `json:"storage_usage"`
	PeakTimes    []sharedtypes.TimeRange `json:"peak_times"`
	Confidence   float64                 `json:"confidence"`
}

// WorkflowLearningData represents data used for learning from workflow executions
type WorkflowLearningData struct {
	ExecutionID       string                                     `json:"execution_id"`
	TemplateID        string                                     `json:"template_id"`
	Features          *WorkflowFeatures                          `json:"features"`
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

// DiscoveredPattern represents a comprehensive pattern found in historical data
type DiscoveredPattern struct {
	sharedtypes.BasePattern // Embedded: ID, Name, Description, Type, Confidence, Frequency, SuccessRate, AverageExecutionTime, LastSeen, Tags, SourceExecutions, Metrics, CreatedAt, UpdatedAt, Metadata

	// Pattern type-specific field
	PatternType PatternType `json:"pattern_type"` // Separate from generic Type field

	// Pattern Characteristics
	AlertPatterns    []*AlertPattern    `json:"alert_patterns"`
	ResourcePatterns []*ResourcePattern `json:"resource_patterns"`
	TemporalPatterns []*TemporalPattern `json:"temporal_patterns"`
	FailurePatterns  []*FailurePattern  `json:"failure_patterns"`

	// Optimization Insights
	OptimizationHints []*OptimizationHint       `json:"optimization_hints"`
	WorkflowTemplate  *sharedtypes.TemplateSpec `json:"suggested_template,omitempty"`

	// Discovery-specific metadata
	DiscoveredAt time.Time `json:"discovered_at"`
}

// PatternType defines different types of patterns
type PatternType string

const (
	PatternTypeAlert        PatternType = "alert_correlation"
	PatternTypeResource     PatternType = "resource_utilization"
	PatternTypeTemporal     PatternType = "temporal_sequence"
	PatternTypeFailure      PatternType = "failure_chain"
	PatternTypeOptimization PatternType = "optimization_opportunity"
	PatternTypeAnomaly      PatternType = "anomaly_detection"
	PatternTypeWorkflow     PatternType = "workflow_effectiveness"
)

// AlertPattern represents patterns in alert characteristics
type AlertPattern struct {
	AlertTypes      []string          `json:"alert_types"`
	Namespaces      []string          `json:"namespaces"`
	Resources       []string          `json:"resources"`
	SeverityPattern string            `json:"severity_pattern"`
	LabelPatterns   map[string]string `json:"label_patterns"`
	Correlation     *AlertCorrelation `json:"correlation,omitempty"`
	TimeWindow      time.Duration     `json:"time_window"`
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

// ResourcePattern represents patterns in resource behavior
type ResourcePattern struct {
	ResourceType   string                    `json:"resource_type"`
	MetricPatterns map[string]*MetricPattern `json:"metric_patterns"`
}

// MetricPattern represents patterns in resource metrics
type MetricPattern struct {
	MetricName      string    `json:"metric_name"`
	Pattern         string    `json:"pattern"` // "increasing", "decreasing", "oscillating", "stable"
	Threshold       float64   `json:"threshold"`
	Anomalies       []float64 `json:"anomalies"`
	TrendConfidence float64   `json:"trend_confidence"`
}

// TemporalPattern represents time-based patterns
type TemporalPattern struct {
	Pattern         string             `json:"pattern"` // "daily", "weekly", "monthly", "burst"
	PeakTimes       []PatternTimeRange `json:"peak_times"`
	SeasonalFactors map[string]float64 `json:"seasonal_factors"`
	CycleDuration   time.Duration      `json:"cycle_duration"`
	PredictedNext   *time.Time         `json:"predicted_next,omitempty"`
}

// PatternTimeRange represents a time range for patterns
type PatternTimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// FailurePattern represents failure correlation patterns
type FailurePattern struct {
	FailureChain       []*FailureNode   `json:"failure_chain"`
	RootCause          string           `json:"root_cause"`
	PropagationTime    time.Duration    `json:"propagation_time"`
	AffectedComponents []string         `json:"affected_components"`
	RecoveryPattern    *RecoveryPattern `json:"recovery_pattern,omitempty"`
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

// OptimizationHint suggests workflow improvements
type OptimizationHint struct {
	Type               string   `json:"type"`
	Description        string   `json:"description"`
	ImpactEstimate     float64  `json:"impact_estimate"`
	ImplementationCost float64  `json:"implementation_cost"`
	Priority           int      `json:"priority"`
	ActionSuggestion   string   `json:"action_suggestion"`
	Evidence           []string `json:"evidence"`
}

// ResourceTrendAnalysis represents analysis of resource usage trends
// Consolidated from pkg/intelligence/learning and pkg/intelligence/patterns
type ResourceTrendAnalysis struct {
	ResourceType   string                    `json:"resource_type"`
	TrendDirection string                    `json:"trend_direction"`
	Slope          float64                   `json:"slope"`
	Correlation    float64                   `json:"correlation"`
	Seasonality    []float64                 `json:"seasonality"`
	Anomalies      []int                     `json:"anomalies"`
	StartTime      time.Time                 `json:"start_time"`
	EndTime        time.Time                 `json:"end_time"`
	Confidence     float64                   `json:"confidence"`
	Significance   float64                   `json:"significance"`
	Occurrences    int                       `json:"occurrences"`
	MetricPatterns map[string]*MetricPattern `json:"metric_patterns"`
}

// TemporalAnalysis represents temporal pattern analysis results
// Consolidated from pkg/intelligence/learning and pkg/intelligence/patterns
type TemporalAnalysis struct {
	PatternType     string                 `json:"pattern_type"`
	Confidence      float64                `json:"confidence"`
	StartTime       time.Time              `json:"start_time"`
	EndTime         time.Time              `json:"end_time"`
	Frequency       time.Duration          `json:"frequency"`
	Peaks           []time.Time            `json:"peaks"`
	Description     string                 `json:"description"`
	Metadata        map[string]interface{} `json:"metadata"`
	PeakTimes       []PatternTimeRange     `json:"peak_times"`
	SeasonalFactors map[string]float64     `json:"seasonal_factors"`
	CycleDuration   time.Duration          `json:"cycle_duration"`
}

// Overfitting-related types consolidated from pkg/intelligence/learning
// to break import cycles

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
