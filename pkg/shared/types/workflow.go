package types

import (
	"context"
	"time"
)

// ExecutionStatus represents the status of workflow execution
// Following development principle: define types needed for integration
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
)

// Canonical workflow-related types used across multiple packages.
// These types resolve conflicts by providing authoritative definitions
// that consolidate the best aspects of previously duplicated types.

// TemplateSpec represents a workflow template specification with comprehensive metadata.
// This type consolidates definitions from:
// - pkg/intelligence/patterns/pattern_discovery_helpers.go
// - pkg/workflow/types/core.go
type TemplateSpec struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version,omitempty"`
	Steps       []WorkflowStep         `json:"steps"`
	Variables   map[string]interface{} `json:"variables,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
}

// WorkflowStep represents a step in a workflow template with full capabilities.
// This type consolidates definitions from:
// - pkg/intelligence/patterns/pattern_discovery_helpers.go
// - pkg/workflow/types/core.go
type WorkflowStep struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	Action       string                 `json:"action,omitempty"`
	Conditions   []ConditionSpec        `json:"conditions,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	Dependencies []string               `json:"dependencies,omitempty"`
	Timeout      time.Duration          `json:"timeout,omitempty"`
}

// ConditionSpec represents conditions specification for workflow execution.
type ConditionSpec struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// OptimizationSuggestion provides comprehensive optimization recommendations.
// This type consolidates definitions from:
// - pkg/intelligence/patterns/pattern_discovery_helpers.go
// - pkg/workflow/types/core.go
type OptimizationSuggestion struct {
	Type                 string  `json:"type"`
	Description          string  `json:"description"`
	Impact               string  `json:"impact,omitempty"`                // From helpers: qualitative impact
	ExpectedImprovement  float64 `json:"expected_improvement,omitempty"`  // From core: quantitative improvement
	Effort               string  `json:"effort,omitempty"`                // From helpers: implementation effort (qualitative)
	ImplementationEffort string  `json:"implementation_effort,omitempty"` // From core: implementation effort
	Priority             int     `json:"priority"`
}

// WorkflowExecutionResult represents comprehensive execution results.
// This consolidates multiple similar types and provides complete execution metadata.
type SharedWorkflowExecutionResult struct {
	BaseTimestampedResult // Embedded: Success, StartTime, EndTime, Duration, Error

	// Workflow-specific fields
	StepsCompleted    int      `json:"steps_completed"`
	TotalSteps        int      `json:"total_steps,omitempty"`
	ResourcesAffected []string `json:"resources_affected,omitempty"`
	ExecutionID       string   `json:"execution_id,omitempty"`
}

// WorkflowExecutionData represents comprehensive workflow execution data
// for analytics and learning purposes.
type WorkflowExecutionData struct {
	ExecutionID     string                         `json:"execution_id"`
	WorkflowID      string                         `json:"workflow_id"`
	TemplateID      string                         `json:"template_id,omitempty"`
	Timestamp       time.Time                      `json:"timestamp"`
	Duration        time.Duration                  `json:"duration"`
	Success         bool                           `json:"success"`
	ExecutionResult *SharedWorkflowExecutionResult `json:"execution_result,omitempty"`
	ResourceUsage   *ResourceUsageData             `json:"resource_usage,omitempty"`
	Metrics         map[string]float64             `json:"metrics,omitempty"`
	Metadata        map[string]interface{}         `json:"metadata,omitempty"`
	Context         map[string]interface{}         `json:"context,omitempty"`
}

// RuntimeWorkflowExecution represents a runtime workflow execution for pattern analysis
// This type is used by the Pattern Discovery Engine for execution data processing
// Following development principle: integrate with existing code by including all expected fields
type RuntimeWorkflowExecution struct {
	WorkflowExecutionRecord // Embedded shared analytics fields (ID, WorkflowID, Status, StartTime, EndTime, Metadata)

	// Additional fields expected by Pattern Discovery Engine implementation
	WorkflowID        string                 `json:"workflow_id"`
	OperationalStatus ExecutionStatus        `json:"operational_status"`
	Variables         map[string]interface{} `json:"variables"`
	Duration          time.Duration          `json:"duration"`
	Input             *WorkflowInput         `json:"input,omitempty"`
	Output            *WorkflowOutput        `json:"output,omitempty"`
	Context           map[string]interface{} `json:"context"`
	Error             string                 `json:"error,omitempty"`
}

// WorkflowExecutionRecord represents a historical workflow execution for analytics and learning.
// This simplified structure is optimized for bulk processing and pattern analysis.
type WorkflowExecutionRecord struct {
	ID         string                 `json:"id"`
	WorkflowID string                 `json:"workflow_id"`
	Status     string                 `json:"status"`
	StartTime  time.Time              `json:"start_time"`
	EndTime    *time.Time             `json:"end_time,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// RuntimeWorkflowExecutionOperational represents an active execution of a workflow with full operational state
// Moved from pkg/workflow/engine/models.go to break circular import
type RuntimeWorkflowExecutionOperational struct {
	WorkflowExecutionRecord // Embedded shared analytics fields (ID, WorkflowID, Status, StartTime, EndTime, Metadata)

	// Operational-specific fields (Status is overridden with enum type)
	OperationalStatus string                 `json:"status"` // Override embedded Status with enum type for operations
	Input             *WorkflowInput         `json:"input"`
	Output            *WorkflowOutput        `json:"output"`
	Steps             []*StepExecution       `json:"steps"`
	CurrentStep       int                    `json:"current_step"`
	Variables         map[string]interface{} `json:"variables"`
	Duration          time.Duration          `json:"duration"`
	Error             string                 `json:"error,omitempty"`
	Recovery          *RecoveryPlan          `json:"recovery,omitempty"`
}

// TrendAnalysis represents trend analysis results
// Moved from pkg/workflow/engine/models.go to break circular import
type TrendAnalysis struct {
	Direction  string  `json:"direction"`
	Strength   float64 `json:"strength"`
	Confidence float64 `json:"confidence"`
	Slope      float64 `json:"slope"`
}

// WorkflowCluster represents a cluster of similar workflow executions
type WorkflowCluster struct {
	ID      string                   `json:"id"`
	Members []*WorkflowExecutionData `json:"members"`
}

// PatternDiscoveryConfig configures pattern discovery analysis
type PatternDiscoveryConfig struct {
	MinSupport      float64 `json:"min_support"`
	MinConfidence   float64 `json:"min_confidence"`
	MaxPatterns     int     `json:"max_patterns"`
	TimeWindowHours int     `json:"time_window_hours"`
}

// AnomalyResult represents detected anomalies
type AnomalyResult struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
}

// ResourceTrendAnalysis represents resource utilization trend analysis
type ResourceTrendAnalysis struct {
	ResourceType   string                    `json:"resource_type"`
	Confidence     float64                   `json:"confidence"`
	Significance   float64                   `json:"significance"`
	Occurrences    int                       `json:"occurrences"`
	MetricPatterns map[string]*MetricPattern `json:"metric_patterns"`
	TrendDirection string                    `json:"trend_direction"`
	GrowthRate     float64                   `json:"growth_rate"`
}

// MetricPattern represents a pattern in metric values
type MetricPattern struct {
	MetricName   string    `json:"metric_name"`
	Pattern      string    `json:"pattern"` // "increasing", "decreasing", "cyclical", "stable"
	Confidence   float64   `json:"confidence"`
	AverageValue float64   `json:"average_value"`
	PeakValue    float64   `json:"peak_value"`
	MinValue     float64   `json:"min_value"`
	Variability  float64   `json:"variability"`
	LastObserved time.Time `json:"last_observed"`
}

// TemporalPatternAnalysis represents temporal pattern analysis results
type TemporalPatternAnalysis struct {
	PatternType     string             `json:"pattern_type"` // "daily", "weekly", "monthly", "burst"
	Confidence      float64            `json:"confidence"`
	PeakTimes       []PatternTimeRange `json:"peak_times"`
	SeasonalFactors map[string]float64 `json:"seasonal_factors"`
	CycleDuration   time.Duration      `json:"cycle_duration"`
	Strength        float64            `json:"strength"`
}

// PatternTimeRange represents a time range within a pattern
type PatternTimeRange struct {
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Intensity float64   `json:"intensity"` // How strong the pattern is in this range
}

// AlertCluster represents a cluster of similar alerts
type AlertCluster struct {
	ID        string                   `json:"id"`
	AlertType string                   `json:"alert_type"`
	Members   []*WorkflowExecutionData `json:"members"`
	Centroid  map[string]interface{}   `json:"centroid"`  // Representative characteristics
	Cohesion  float64                  `json:"cohesion"`  // How similar the members are
	Frequency int                      `json:"frequency"` // How often this cluster appears
	Severity  string                   `json:"severity"`  // Average severity of alerts in cluster
}

// Interfaces moved from patterns to shared types
// TimeSeriesAnalyzer interface for time series analysis
type TimeSeriesAnalyzer interface {
	AnalyzeTrends(ctx context.Context, data []*WorkflowExecutionData, timeRange TimeRange) (*TrendAnalysis, error)

	// AnalyzeResourceTrends analyzes resource utilization trends over time
	AnalyzeResourceTrends(ctx context.Context, data []*WorkflowExecutionData, resourceType string, timeRange TimeRange) (*ResourceTrendAnalysis, error)

	// DetectTemporalPatterns identifies temporal patterns like daily, weekly cycles
	DetectTemporalPatterns(ctx context.Context, data []*WorkflowExecutionData, timeRange TimeRange) ([]*TemporalPatternAnalysis, error)
}

// ClusteringEngine interface for clustering analysis
// @deprecated RULE 12 VIOLATION: ClusteringEngine interface violates Rule 12 AI/ML methodology
// Migration: Use enhanced llm.Client.ClusterWorkflows(), llm.Client.AnalyzeTrends() methods directly
// Business Requirements: BR-CLUSTER-001, BR-PATTERN-002 - now served by enhanced llm.Client
type ClusteringEngine interface {
	ClusterWorkflows(ctx context.Context, data []*WorkflowExecutionData, config *PatternDiscoveryConfig) ([]*WorkflowCluster, error)

	// ClusterAlerts groups similar alerts together for pattern analysis
	ClusterAlerts(ctx context.Context, data []*WorkflowExecutionData, config *PatternDiscoveryConfig) ([]*AlertCluster, error)
}

// AnomalyDetector interface for anomaly detection
type AnomalyDetector interface {
	DetectAnomalies(ctx context.Context, data []*WorkflowExecutionData, baseline []*WorkflowExecutionData) ([]*AnomalyResult, error)

	// DetectAnomaly detects if a single workflow execution is anomalous
	DetectAnomaly(ctx context.Context, execution *WorkflowExecutionData, baseline []*WorkflowExecutionData) (*AnomalyResult, error)
}

// Supporting types that were referenced but missing
type WorkflowInput struct {
	Alert       *AlertContext          `json:"alert,omitempty"`
	Resource    *ResourceContext       `json:"resource,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
	Environment string                 `json:"environment"`
	Context     map[string]interface{} `json:"context"`
}

// WorkflowOutput represents the output of a workflow execution
// Following development principle: define types needed for integration
type WorkflowOutput struct {
	Result   map[string]interface{} `json:"result"`
	Messages []string               `json:"messages,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Duration time.Duration          `json:"duration"`
	Success  bool                   `json:"success"`
}

// NOTE: Removed duplicate WorkflowOutput definition

type StepExecution struct {
	StepID    string                 `json:"step_id"`
	Status    string                 `json:"status"`
	StartTime time.Time              `json:"start_time"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
	Duration  time.Duration          `json:"duration"`
	Variables map[string]interface{} `json:"variables"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type RecoveryPlan struct {
	ID       string                 `json:"id"`
	Actions  []string               `json:"actions"`
	Triggers []string               `json:"triggers"`
	Priority int                    `json:"priority"`
	Timeout  time.Duration          `json:"timeout"`
	Metadata map[string]interface{} `json:"metadata"`
}

type AlertContext struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Severity    string            `json:"severity"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type ResourceContext struct {
	Namespace string            `json:"namespace"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
}

type ExecutionMetrics struct {
	Duration      time.Duration      `json:"duration"`
	StepCount     int                `json:"step_count"`
	SuccessRate   float64            `json:"success_rate"`
	ErrorRate     float64            `json:"error_rate"`
	ResourceUsage *ResourceUsageData `json:"resource_usage,omitempty"`
}

// NOTE: ExecutionStatus constants already defined above
