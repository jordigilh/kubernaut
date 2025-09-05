package shared

import (
	"time"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
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
	CPUUsage     float64            `json:"cpu_usage"`
	MemoryUsage  float64            `json:"memory_usage"`
	NetworkUsage float64            `json:"network_usage"`
	StorageUsage float64            `json:"storage_usage"`
	PeakTimes    []engine.TimeRange `json:"peak_times"`
	Confidence   float64            `json:"confidence"`
}

// WorkflowLearningData represents data used for learning from workflow executions
type WorkflowLearningData struct {
	ExecutionID       string                               `json:"execution_id"`
	TemplateID        string                               `json:"template_id"`
	Features          *WorkflowFeatures                    `json:"features"`
	ExecutionResult   *sharedtypes.WorkflowExecutionResult `json:"execution_result"`
	ResourceUsage     *sharedtypes.ResourceUsageData       `json:"resource_usage"`
	Context           map[string]interface{}               `json:"context"`
	LearningObjective string                               `json:"learning_objective"`
	Feedback          *LearningFeedback                    `json:"feedback,omitempty"`
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
	ID                   string        `json:"id"`
	Type                 PatternType   `json:"type"`
	Name                 string        `json:"name"`
	Description          string        `json:"description"`
	Confidence           float64       `json:"confidence"`
	Frequency            int           `json:"frequency"`
	SuccessRate          float64       `json:"success_rate"`
	AverageExecutionTime time.Duration `json:"average_execution_time"`

	// Pattern Characteristics
	AlertPatterns    []*AlertPattern    `json:"alert_patterns"`
	ResourcePatterns []*ResourcePattern `json:"resource_patterns"`
	TemporalPatterns []*TemporalPattern `json:"temporal_patterns"`
	FailurePatterns  []*FailurePattern  `json:"failure_patterns"`

	// Optimization Insights
	OptimizationHints []*OptimizationHint           `json:"optimization_hints"`
	WorkflowTemplate  *sharedtypes.WorkflowTemplate `json:"suggested_template,omitempty"`

	// Metadata
	DiscoveredAt     time.Time          `json:"discovered_at"`
	LastSeen         time.Time          `json:"last_seen"`
	UpdatedAt        time.Time          `json:"updated_at"`
	SourceExecutions []string           `json:"source_executions"`
	Tags             []string           `json:"tags"`
	Metrics          map[string]float64 `json:"metrics"`
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
