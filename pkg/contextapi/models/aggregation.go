package models

import "time"

// TimeWindow represents a time range for aggregation queries
type TimeWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// SuccessRateResult represents aggregated success rate data for a workflow
type SuccessRateResult struct {
	WorkflowID         string    `json:"workflow_id"`
	TotalAttempts      int64     `json:"total_attempts" db:"total_attempts"`
	SuccessfulAttempts int64     `json:"successful_attempts" db:"successful_attempts"`
	SuccessRate        float64   `json:"success_rate" db:"success_rate"`
	TimeWindow         string    `json:"time_window"` // e.g., "30 days"
	CalculatedAt       time.Time `json:"calculated_at"`
}

// ActionSuccessRate represents success rate data for a specific action type
type ActionSuccessRate struct {
	ActionType           string    `json:"action_type" db:"action_type"`
	TotalAttempts        int64     `json:"total_attempts" db:"total_attempts"`
	SuccessfulAttempts   int64     `json:"successful_attempts" db:"successful_attempts"`
	SuccessRate          float64   `json:"success_rate" db:"success_rate"`
	AverageExecutionTime float64   `json:"average_execution_time_ms" db:"avg_execution_time"` // milliseconds
	TimeWindow           string    `json:"time_window"`
	CalculatedAt         time.Time `json:"calculated_at"`
}

// TrendPoint represents a data point in an incident trend analysis
type TrendPoint struct {
	Date  time.Time `json:"date" db:"date"`
	Count int64     `json:"count" db:"count"`
}

// NamespaceGroup represents incident count grouped by namespace
type NamespaceGroup struct {
	Namespace           string `json:"namespace" db:"namespace"`
	TotalIncidents      int    `json:"total_incidents" db:"total_incidents"`
	SuccessfulIncidents int    `json:"successful_incidents" db:"successful_incidents"`
	FailedIncidents     int    `json:"failed_incidents" db:"failed_incidents"`
	Count               int64  `json:"count" db:"count"` // For backward compatibility
}

// IncidentTrend represents incident count trend over time
type IncidentTrend struct {
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	Count     int       `json:"count" db:"count"`
}

// TopFailingAction represents an action with high failure rate
type TopFailingAction struct {
	ActionType   string  `json:"action_type" db:"action_type"`
	FailureCount int     `json:"failure_count" db:"failure_count"`
	TotalCount   int     `json:"total_count" db:"total_count"`
	FailureRate  float64 `json:"failure_rate"`
}

// ActionComparison compares two actions
type ActionComparison struct {
	Action1            string        `json:"action1"`
	Action2            string        `json:"action2"`
	Action1Total       int           `json:"action1_total"`
	Action1Success     int           `json:"action1_success"`
	Action1Failures    int           `json:"action1_failures"`
	Action1AvgDuration time.Duration `json:"action1_avg_duration"`
	Action2Total       int           `json:"action2_total"`
	Action2Success     int           `json:"action2_success"`
	Action2Failures    int           `json:"action2_failures"`
	Action2AvgDuration time.Duration `json:"action2_avg_duration"`
	Winner             string        `json:"winner"` // Based on success rate
}

// NamespaceHealthScore represents health metrics for a namespace
type NamespaceHealthScore struct {
	Namespace      string  `json:"namespace"`
	HealthScore    float64 `json:"health_score"` // 0.0-1.0
	TotalIncidents int     `json:"total_incidents"`
	SuccessRate    float64 `json:"success_rate"`
	AvgDuration    float64 `json:"avg_duration_ms"`
}

// SeverityDistribution represents incident distribution by severity
type SeverityDistribution struct {
	Severity string `json:"severity" db:"severity"`
	Count    int64  `json:"count" db:"count"`
}

// ClusterDistribution represents incident distribution by cluster
type ClusterDistribution struct {
	ClusterName string `json:"cluster_name" db:"cluster_name"`
	Count       int64  `json:"count" db:"count"`
}

// PhaseDistribution represents incident distribution by remediation phase
type PhaseDistribution struct {
	Phase string `json:"phase" db:"phase"`
	Count int64  `json:"count" db:"count"`
}

// AggregationFilters defines common filters for aggregation queries
type AggregationFilters struct {
	Namespace   *string    `json:"namespace,omitempty"`
	ClusterName *string    `json:"cluster_name,omitempty"`
	Environment *string    `json:"environment,omitempty"`
	Severity    *string    `json:"severity,omitempty"`
	ActionType  *string    `json:"action_type,omitempty"`
	Phase       *string    `json:"phase,omitempty"`
	StartTime   *time.Time `json:"start_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`
}
