/*
Copyright 2025 Jordi Gil Heredia.

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

package models

// ========================================
// AGGREGATION API RESPONSE TYPES
// Business Requirement: BR-STORAGE-030, BR-STORAGE-032, BR-STORAGE-033, BR-STORAGE-034
// ========================================
//
// These structured types replace map[string]interface{} for aggregation API responses,
// providing compile-time type safety and clear API contracts.
//
// Anti-Pattern Addressed: Using map[string]interface{} eliminates type safety (IMPLEMENTATION_PLAN_V4.9 #21)
// ========================================

// SuccessRateAggregationResponse represents the success rate aggregation for a workflow
// BR-STORAGE-030: Workflow Success Rate Aggregation
type SuccessRateAggregationResponse struct {
	WorkflowID   string  `json:"workflow_id"`
	TotalCount   int     `json:"total_count"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	SuccessRate  float64 `json:"success_rate"`
}

// AggregationItem represents a single item in a grouped aggregation (namespace, severity, etc.)
type AggregationItem struct {
	// Key is the grouping dimension (namespace, severity, etc.)
	// JSON field name varies by endpoint:
	// - "namespace" for /by-namespace
	// - "severity" for /by-severity
	Key   string `json:"-"` // Omit from JSON, populated dynamically
	Count int    `json:"count"`
}

// NamespaceAggregationItem represents a namespace aggregation item
// BR-STORAGE-032: Namespace Grouping Aggregation
type NamespaceAggregationItem struct {
	Namespace string `json:"namespace"`
	Count     int    `json:"count"`
}

// NamespaceAggregationResponse represents the namespace aggregation endpoint response
// BR-STORAGE-032: Namespace Grouping Aggregation
type NamespaceAggregationResponse struct {
	Aggregations []NamespaceAggregationItem `json:"aggregations"`
}

// SeverityAggregationItem represents a severity aggregation item
// BR-STORAGE-033: Severity Distribution Aggregation
type SeverityAggregationItem struct {
	Severity string `json:"severity"`
	Count    int    `json:"count"`
}

// SeverityAggregationResponse represents the severity aggregation endpoint response
// BR-STORAGE-033: Severity Distribution Aggregation
type SeverityAggregationResponse struct {
	Aggregations []SeverityAggregationItem `json:"aggregations"`
}

// TrendDataPoint represents a single point in a time-series trend
type TrendDataPoint struct {
	Date  string `json:"date"` // Format: "YYYY-MM-DD"
	Count int    `json:"count"`
}

// TrendAggregationResponse represents the trend aggregation response
// BR-STORAGE-034: Incident Trend Aggregation
type TrendAggregationResponse struct {
	Period     string           `json:"period"` // e.g., "7d", "30d"
	DataPoints []TrendDataPoint `json:"data_points"`
}

// ========================================
// ADR-033: MULTI-DIMENSIONAL SUCCESS TRACKING RESPONSE TYPES
// Business Requirement: BR-STORAGE-031-01, BR-STORAGE-031-02, BR-STORAGE-031-04, BR-STORAGE-031-05
// Design Decision: DD-012 (Goose Database Migration Management)
// Authority: migrations/012_adr033_multidimensional_tracking.sql
// ========================================
//
// These response types support ADR-033 Multi-Dimensional Success Tracking:
// 1. Incident Type Success Rate (PRIMARY dimension)
// 2. Playbook Success Rate (SECONDARY dimension)
// 3. Multi-Dimensional Aggregation (all 3 dimensions: incident + playbook + action)
//
// ========================================

// IncidentTypeSuccessRateResponse represents success rate aggregation by incident type
// BR-STORAGE-031-01: Incident-Type Success Rate API
// This is the PRIMARY dimension for AI learning - tracks which playbooks work for specific problems
type IncidentTypeSuccessRateResponse struct {
	// IncidentType is the problem being solved (e.g., "pod-oom-killer", "high-cpu-usage")
	IncidentType string `json:"incident_type"`

	// TimeRange is the analysis period (e.g., "7d", "30d", "90d")
	TimeRange string `json:"time_range"`

	// TotalExecutions is the total number of remediation attempts for this incident type
	TotalExecutions int `json:"total_executions"`

	// SuccessfulExecutions is the number of successful remediations
	SuccessfulExecutions int `json:"successful_executions"`

	// FailedExecutions is the number of failed remediations
	FailedExecutions int `json:"failed_executions"`

	// SuccessRate is the percentage of successful remediations (0.0 to 100.0)
	SuccessRate float64 `json:"success_rate"`

	// Confidence indicates statistical confidence in the success rate
	// Values: "high" (>100 samples), "medium" (20-100 samples), "low" (5-20 samples), "insufficient_data" (<5 samples)
	Confidence string `json:"confidence"`

	// MinSamplesMet indicates if minimum sample size threshold was reached
	MinSamplesMet bool `json:"min_samples_met"`

	// BreakdownByPlaybook shows which playbooks were used for this incident type
	// Sorted by execution count (descending) to highlight most popular playbooks
	BreakdownByPlaybook []PlaybookBreakdownItem `json:"breakdown_by_playbook,omitempty"`

	// AIExecutionMode shows distribution of AI execution modes (catalog/chained/manual)
	// NULL if no AI mode data available (backward compatibility with pre-ADR-033 data)
	AIExecutionMode *AIExecutionModeStats `json:"ai_execution_mode,omitempty"`
}

// PlaybookBreakdownItem represents playbook-specific statistics within an incident type
// Used in IncidentTypeSuccessRateResponse to show which playbooks were tried
type PlaybookBreakdownItem struct {
	// PlaybookID is the remediation playbook identifier (e.g., "pod-oom-recovery")
	PlaybookID string `json:"playbook_id"`

	// PlaybookVersion is the semantic version (e.g., "v1.2", "v2.0")
	PlaybookVersion string `json:"playbook_version"`

	// Executions is the number of times this playbook was used
	Executions int `json:"executions"`

	// SuccessRate is the percentage of successful executions for this specific playbook
	SuccessRate float64 `json:"success_rate"`
}

// AIExecutionModeStats tracks AI execution mode distribution (ADR-033 Hybrid Model)
// Shows how AI selected remediation approaches:
// - 90-95% catalog selection (single playbook from catalog)
// - 4-9% playbook chaining (multiple playbooks composed)
// - <1% manual escalation (human intervention required)
type AIExecutionModeStats struct {
	// CatalogSelected is the count of single-playbook catalog selections
	CatalogSelected int `json:"catalog_selected"`

	// Chained is the count of multi-playbook compositions
	Chained int `json:"chained"`

	// ManualEscalation is the count of escalations to human operators
	ManualEscalation int `json:"manual_escalation"`
}

// PlaybookSuccessRateResponse represents success rate aggregation by playbook
// BR-STORAGE-031-02: Playbook Success Rate API
// This is the SECONDARY dimension - tracks which playbooks are most effective overall
type PlaybookSuccessRateResponse struct {
	// PlaybookID is the remediation playbook identifier (e.g., "disk-cleanup", "network-retry")
	PlaybookID string `json:"playbook_id"`

	// PlaybookVersion is the semantic version (e.g., "v1.0", "v2.0")
	PlaybookVersion string `json:"playbook_version"`

	// TimeRange is the analysis period (e.g., "7d", "30d", "90d")
	TimeRange string `json:"time_range"`

	// TotalExecutions is the total number of times this playbook was executed
	TotalExecutions int `json:"total_executions"`

	// SuccessfulExecutions is the number of successful executions
	SuccessfulExecutions int `json:"successful_executions"`

	// FailedExecutions is the number of failed executions
	FailedExecutions int `json:"failed_executions"`

	// SuccessRate is the percentage of successful executions (0.0 to 100.0)
	SuccessRate float64 `json:"success_rate"`

	// Confidence indicates statistical confidence in the success rate
	// Values: "high" (>100 samples), "medium" (20-100 samples), "low" (5-20 samples), "insufficient_data" (<5 samples)
	Confidence string `json:"confidence"`

	// MinSamplesMet indicates if minimum sample size threshold was reached
	MinSamplesMet bool `json:"min_samples_met"`

	// BreakdownByIncidentType shows which incident types this playbook was used for
	// Sorted by execution count (descending) to highlight primary use cases
	BreakdownByIncidentType []IncidentTypeBreakdownItem `json:"breakdown_by_incident_type,omitempty"`

	// AIExecutionMode shows distribution of AI execution modes for this playbook
	// NULL if no AI mode data available (backward compatibility)
	AIExecutionMode *AIExecutionModeStats `json:"ai_execution_mode,omitempty"`

	// TrendAnalysis shows success rate trend over time (optional)
	TrendAnalysis *TrendAnalysisData `json:"trend_analysis,omitempty"`
}

// IncidentTypeBreakdownItem represents incident-type-specific statistics for a playbook
// Used in PlaybookSuccessRateResponse to show which problems the playbook solves
type IncidentTypeBreakdownItem struct {
	// IncidentType is the problem category (e.g., "pod-oom-killer", "disk-pressure")
	IncidentType string `json:"incident_type"`

	// Executions is the number of times this playbook was used for this incident type
	Executions int `json:"executions"`

	// SuccessRate is the percentage of successful executions for this specific incident type
	SuccessRate float64 `json:"success_rate"`
}

// TrendAnalysisData represents success rate trend over time
// Used to identify if playbook effectiveness is improving or degrading
type TrendAnalysisData struct {
	// Direction indicates the trend direction ("improving", "stable", "degrading")
	Direction string `json:"direction"`

	// Slope is the rate of change in success rate per day (positive = improving)
	Slope float64 `json:"slope"`

	// DataPoints are the individual trend measurements
	DataPoints []TrendDataPoint `json:"data_points"`
}

// MultiDimensionalSuccessRateResponse represents cross-dimensional success rate aggregation
// BR-STORAGE-031-05: Multi-Dimensional Success Rate API
// Combines all 3 dimensions: incident type + playbook + action type
type MultiDimensionalSuccessRateResponse struct {
	// Dimensions specifies which dimensions are being aggregated
	Dimensions QueryDimensions `json:"dimensions"`

	// TimeRange is the analysis period (e.g., "7d", "30d", "90d")
	TimeRange string `json:"time_range"`

	// TotalExecutions is the total matching executions across all dimensions
	TotalExecutions int `json:"total_executions"`

	// SuccessfulExecutions is the number of successful executions
	SuccessfulExecutions int `json:"successful_executions"`

	// FailedExecutions is the number of failed executions
	FailedExecutions int `json:"failed_executions"`

	// SuccessRate is the overall success rate (0.0 to 100.0)
	SuccessRate float64 `json:"success_rate"`

	// Confidence indicates statistical confidence in the success rate
	Confidence string `json:"confidence"`

	// MinSamplesMet indicates if minimum sample size threshold was reached
	MinSamplesMet bool `json:"min_samples_met"`

	// BreakdownByIncidentType groups by incident type (if playbook/action specified)
	BreakdownByIncidentType []IncidentTypeBreakdownItem `json:"breakdown_by_incident_type,omitempty"`

	// BreakdownByPlaybook groups by playbook (if incident_type/action specified)
	BreakdownByPlaybook []PlaybookBreakdownItem `json:"breakdown_by_playbook,omitempty"`

	// BreakdownByAction groups by action type (if incident_type/playbook specified)
	BreakdownByAction []ActionBreakdownItem `json:"breakdown_by_action,omitempty"`

	// AIExecutionMode shows AI execution mode distribution
	AIExecutionMode *AIExecutionModeStats `json:"ai_execution_mode,omitempty"`
}

// QueryDimensions represents the dimensions used in a multi-dimensional query
// BR-STORAGE-031-05: Multi-Dimensional Success Rate API
// Used in MultiDimensionalSuccessRateResponse to echo back query parameters
type QueryDimensions struct {
	// IncidentType is the incident type filter (empty if not specified)
	IncidentType string `json:"incident_type"`

	// PlaybookID is the playbook ID filter (empty if not specified)
	PlaybookID string `json:"playbook_id"`

	// PlaybookVersion is the playbook version filter (empty if not specified)
	PlaybookVersion string `json:"playbook_version"`

	// ActionType is the action type filter (empty if not specified)
	ActionType string `json:"action_type"`
}

// MultiDimensionalQuery represents the input parameters for multi-dimensional queries
// BR-STORAGE-031-05: Multi-Dimensional Success Rate API
// Used by repository layer to construct dynamic WHERE clauses
type MultiDimensionalQuery struct {
	// IncidentType filters to specific incident type (empty = all incident types)
	IncidentType string

	// PlaybookID filters to specific playbook (empty = all playbooks)
	PlaybookID string

	// PlaybookVersion filters to specific playbook version (empty = all versions)
	// Requires PlaybookID to be specified
	PlaybookVersion string

	// ActionType filters to specific action type (empty = all action types)
	ActionType string

	// TimeRange is the time window for aggregation (e.g., "7d", "30d")
	TimeRange string

	// MinSamples is the minimum sample size for confidence calculation (default: 5)
	MinSamples int
}

// DimensionsFilter specifies which dimensions are included in the aggregation
// All fields are optional - NULL means "aggregate across all values of this dimension"
// DEPRECATED: Use QueryDimensions instead for new code
type DimensionsFilter struct {
	// IncidentType filters to specific incident type (NULL = all incident types)
	IncidentType *string `json:"incident_type,omitempty"`

	// PlaybookID filters to specific playbook (NULL = all playbooks)
	PlaybookID *string `json:"playbook_id,omitempty"`

	// PlaybookVersion filters to specific playbook version (NULL = all versions)
	PlaybookVersion *string `json:"playbook_version,omitempty"`

	// ActionType filters to specific action type (NULL = all action types)
	ActionType *string `json:"action_type,omitempty"`
}

// ActionBreakdownItem represents action-type-specific statistics
// Used in MultiDimensionalSuccessRateResponse to show action-level performance
type ActionBreakdownItem struct {
	// ActionType is the specific action (e.g., "restart_pod", "scale_deployment")
	ActionType string `json:"action_type"`

	// Executions is the number of times this action was executed
	Executions int `json:"executions"`

	// SuccessRate is the percentage of successful executions for this action
	SuccessRate float64 `json:"success_rate"`
}

// DeprecatedWorkflowSuccessRateResponse represents the deprecated workflow-based success rate
// BR-STORAGE-031-06: Deprecated Endpoint Warning
// This response type is maintained for backward compatibility but will be removed in V2.0
// DEPRECATED: Use IncidentTypeSuccessRateResponse or PlaybookSuccessRateResponse instead
type DeprecatedWorkflowSuccessRateResponse struct {
	// WorkflowID is the deprecated workflow identifier (mapped to playbook_execution_id)
	// DEPRECATED: Use playbook_id + playbook_execution_id instead
	WorkflowID string `json:"workflow_id"`

	// TotalExecutions is the total count (same as other response types)
	TotalExecutions int `json:"total_executions"`

	// SuccessfulExecutions is the success count (same as other response types)
	SuccessfulExecutions int `json:"successful_executions"`

	// FailedExecutions is the failure count (same as other response types)
	FailedExecutions int `json:"failed_executions"`

	// SuccessRate is the percentage (same as other response types)
	SuccessRate float64 `json:"success_rate"`

	// DeprecationWarning is included in every response to alert clients
	DeprecationWarning string `json:"deprecation_warning"`

	// MigrationGuide provides URL to migration documentation
	MigrationGuide string `json:"migration_guide"`
}
