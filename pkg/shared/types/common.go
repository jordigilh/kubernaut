package types

import (
	"fmt"
	"strings"
	"time"
)

// Common types used across multiple packages in the kubernaut system.
// These types were consolidated to eliminate duplication and provide
// consistent data structures throughout the codebase.

// TimeRange represents a time range for analysis.
// This type consolidates identical definitions from:
// - pkg/workflow/engine/models.go
// - pkg/intelligence/shared/types.go (PatternTimeRange)
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// UtilizationTrend represents resource utilization trend analysis data.
// This type consolidates identical definitions from:
// - pkg/intelligence/patterns/pattern_discovery_helpers.go
// - pkg/intelligence/learning/time_series_analyzer.go
type UtilizationTrend struct {
	ResourceType       string  `json:"resource_type"`
	TrendDirection     string  `json:"trend_direction"`
	GrowthRate         float64 `json:"growth_rate"`
	SeasonalVariation  float64 `json:"seasonal_variation"`
	PeakUtilization    float64 `json:"peak_utilization"`
	AverageUtilization float64 `json:"average_utilization"`
	EfficiencyScore    float64 `json:"efficiency_score"`
}

// ConfidenceInterval represents statistical confidence interval data.
// This type consolidates identical definitions from:
// - pkg/intelligence/patterns/pattern_discovery_helpers.go
// - pkg/intelligence/learning/time_series_analyzer.go
type ConfidenceInterval struct {
	Level float64   `json:"level"` // e.g., 0.95 for 95% confidence
	Lower []float64 `json:"lower"`
	Upper []float64 `json:"upper"`
}

// ResourceUsageData represents resource usage metrics.
// This type consolidates identical definitions from:
// - pkg/intelligence/patterns/pattern_discovery_helpers.go
// - pkg/workflow/types/core.go
type ResourceUsageData struct {
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  float64 `json:"memory_usage"`
	NetworkUsage float64 `json:"network_usage"`
	StorageUsage float64 `json:"storage_usage"`
}

// ValidationResult represents the result of a validation operation.
// This type provides consistent validation result structure and consolidates:
// - pkg/intelligence/patterns/pattern_discovery_types.PatternValidationResult
// - internal/validation/validators.ValidationError
type ValidationResult struct {
	RuleID    string                 `json:"rule_id"`
	Type      ValidationType         `json:"type"`
	Passed    bool                   `json:"passed"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details"`
	Timestamp time.Time              `json:"timestamp"`

	// Enhanced fields for pattern validation compatibility
	TestType          string             `json:"test_type,omitempty"`
	TestAccuracy      float64            `json:"test_accuracy,omitempty"`
	TrainAccuracy     float64            `json:"train_accuracy,omitempty"`
	GeneralizationGap float64            `json:"generalization_gap,omitempty"`
	OverfittingScore  float64            `json:"overfitting_score,omitempty"`
	Recommendations   []string           `json:"recommendations,omitempty"`
	QualityIndicators map[string]float64 `json:"quality_indicators,omitempty"`

	// Field validation support
	Field string `json:"field,omitempty"` // For field-level validation errors
}

// Error implements the error interface
func (vr ValidationResult) Error() string {
	if vr.Field != "" {
		return fmt.Sprintf("validation error for field '%s': %s", vr.Field, vr.Message)
	}
	return vr.Message
}

// ValidationError creates a ValidationResult representing an error
func NewValidationError(field, message string) ValidationResult {
	return ValidationResult{
		RuleID:    field + "_validation",
		Type:      ValidationTypeStructural,
		Passed:    false,
		Message:   message,
		Field:     field,
		Timestamp: time.Now(),
	}
}

// ValidationErrors represents multiple validation results
type ValidationErrors []ValidationResult

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		if e[0].Field != "" {
			return fmt.Sprintf("validation error for field '%s': %s", e[0].Field, e[0].Message)
		}
		return e[0].Message
	}

	var messages []string
	for _, err := range e {
		if err.Field != "" {
			messages = append(messages, fmt.Sprintf("field '%s': %s", err.Field, err.Message))
		} else {
			messages = append(messages, err.Message)
		}
	}
	return fmt.Sprintf("multiple validation errors: %s", strings.Join(messages, "; "))
}

// HasErrors returns true if any validation failed
func (e ValidationErrors) HasErrors() bool {
	for _, result := range e {
		if !result.Passed {
			return true
		}
	}
	return false
}

// Alert represents a unified alert structure consolidating:
// - pkg/ai/common/types.go Alert struct
// - pkg/infrastructure/types/types.go Alert struct
// This eliminates duplication and provides a comprehensive alert type.
type Alert struct {
	// From AI common types
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Summary     string            `json:"summary"`
	Description string            `json:"description"`
	Severity    string            `json:"severity"`
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      *time.Time        `json:"ends_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at"`

	// From infrastructure types (Kubernetes-specific fields)
	Namespace string `json:"namespace,omitempty"`
	Resource  string `json:"resource,omitempty"`
}

// ValidationType represents the type of validation being performed.
type ValidationType string

const (
	ValidationTypeStructural  ValidationType = "structural"
	ValidationTypeSemantic    ValidationType = "semantic"
	ValidationTypePerformance ValidationType = "performance"
	ValidationTypeSecurity    ValidationType = "security"
	ValidationTypePattern     ValidationType = "pattern"     // For ML pattern validation
	ValidationTypeAccuracy    ValidationType = "accuracy"    // For model accuracy validation
	ValidationTypeOverfitting ValidationType = "overfitting" // For overfitting detection
)

// ActionRecommendation represents an action recommendation from the SLM
// Consolidated from pkg/infrastructure/types/types.go
type ActionRecommendation struct {
	Action     string                 `json:"action"`
	Parameters map[string]interface{} `json:"parameters"`
	Confidence float64                `json:"confidence,omitempty"`
	Reasoning  *ReasoningDetails      `json:"reasoning,omitempty"` // Always structured reasoning
}

// FilterConfig represents alert filtering configuration
// Following Go coding standards: use shared types instead of internal package imports
type FilterConfig struct {
	Name       string              `yaml:"name" json:"name"`
	Conditions map[string][]string `yaml:"conditions" json:"conditions"`
}

// ReasoningDetails provides structured reasoning information
// Consolidated from pkg/infrastructure/types/types.go
type ReasoningDetails struct {
	PrimaryReason      string             `json:"primary_reason,omitempty"`
	HistoricalContext  string             `json:"historical_context,omitempty"`
	OscillationRisk    string             `json:"oscillation_risk,omitempty"`
	AlternativeActions []string           `json:"alternative_actions,omitempty"`
	ConfidenceFactors  map[string]float64 `json:"confidence_factors,omitempty"`
	Summary            string             `json:"summary,omitempty"`
}

// ActionResult represents the result of executing an action
// Consolidated from pkg/infrastructure/types/types.go
type ActionResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`

	// Action-specific fields
	Action    string `json:"action"`
	Message   string `json:"message,omitempty"`
	Timestamp string `json:"timestamp"` // Note: different format from BaseTimestampedResult
}

// ExecutionResult represents the detailed result of action execution
// Consolidated from pkg/infrastructure/types/types.go
type ExecutionResult struct {
	Success   bool          `json:"success"`
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`

	// Execution-specific fields
	Action  string      `json:"action"`
	Message string      `json:"message,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

// WebhookRequest represents the structure of a webhook request from Prometheus Alertmanager
// Consolidated from pkg/infrastructure/types/types.go
type WebhookRequest struct {
	Version  string  `json:"version"`
	GroupKey string  `json:"groupKey"`
	Status   string  `json:"status"`
	Receiver string  `json:"receiver"`
	Alerts   []Alert `json:"alerts"`
}

// ProcessingResult represents the result of processing an alert
// Consolidated from pkg/infrastructure/types/types.go
type ProcessingResult struct {
	Alert          Alert                 `json:"alert"`
	Recommendation *ActionRecommendation `json:"recommendation,omitempty"`
	ActionResult   *ActionResult         `json:"action_result,omitempty"`
	ProcessingTime time.Duration         `json:"processing_time"`
	Error          string                `json:"error,omitempty"`
}

// ValidActions contains all supported action types
// Consolidated from pkg/infrastructure/types/types.go
// ValidActions defines the canonical set of 29 predefined action types
// Source of Truth: docs/design/CANONICAL_ACTION_TYPES.md
// This list MUST match the actions registered in pkg/platform/executor/executor.go
var ValidActions = map[string]bool{
	// Core Actions (P0) - 5 actions
	"scale_deployment":    true,
	"restart_pod":         true,
	"increase_resources":  true,
	"rollback_deployment": true,
	"expand_pvc":          true,

	// Infrastructure Actions (P1) - 6 actions
	"drain_node":     true,
	"cordon_node":    true,
	"uncordon_node":  true,
	"taint_node":     true,
	"untaint_node":   true,
	"quarantine_pod": true,

	// Storage & Persistence (P2) - 3 actions
	"cleanup_storage": true,
	"backup_data":     true,
	"compact_storage": true,

	// Application Lifecycle (P1) - 3 actions
	"update_hpa":        true,
	"restart_daemonset": true,
	"scale_statefulset": true,

	// Security & Compliance (P2) - 3 actions
	"rotate_secrets":        true,
	"audit_logs":            true,
	"update_network_policy": true,

	// Network & Connectivity (P2) - 2 actions
	"restart_network":    true,
	"reset_service_mesh": true,

	// Database & Stateful (P2) - 2 actions
	"failover_database": true,
	"repair_database":   true,

	// Monitoring & Observability (P2) - 3 actions
	"enable_debug_mode":   true,
	"create_heap_dump":    true,
	"collect_diagnostics": true,

	// Resource Management (P1) - 2 actions
	"optimize_resources": true,
	"migrate_workload":   true,

	// Fallback (P3) - 1 action
	"notify_only": true,
}

// Total: 29 canonical action types

// GetValidActionsList returns a slice of valid actions for validation
// Consolidated from pkg/infrastructure/types/types.go
func GetValidActionsList() []string {
	actions := make([]string, 0, len(ValidActions))
	for action := range ValidActions {
		actions = append(actions, action)
	}
	return actions
}

// IsValidAction checks if the given action is valid
// Consolidated from pkg/infrastructure/types/types.go
func IsValidAction(action string) bool {
	return ValidActions[action]
}

// =============================================================================
// HEALTH MONITORING TYPES - BR-HEALTH-XXX Requirements
// =============================================================================

// HealthStatus represents the comprehensive health status of a system component
// BR-HEALTH-001: MUST implement comprehensive health checks for all components
type HealthStatus struct {
	BaseEntity
	BaseTimestampedResult
	IsHealthy       bool                   `json:"is_healthy"`
	ComponentType   string                 `json:"component_type"`
	ServiceEndpoint string                 `json:"service_endpoint"`
	ResponseTime    time.Duration          `json:"response_time"`
	HealthMetrics   HealthMetrics          `json:"health_metrics"`
	ProbeResults    map[string]ProbeResult `json:"probe_results,omitempty"`
}

// HealthMetrics provides detailed health metrics with structured types
// BR-HEALTH-016: MUST track system availability and uptime metrics
// BR-REL-011: MUST maintain monitoring accuracy >99% for critical metrics
type HealthMetrics struct {
	UptimePercentage   float64       `json:"uptime_percentage"`
	TotalUptime        time.Duration `json:"total_uptime"`
	TotalDowntime      time.Duration `json:"total_downtime"`
	FailureCount       int           `json:"failure_count"`
	DowntimeEvents     int           `json:"downtime_events"`
	AccuracyRate       float64       `json:"accuracy_rate"`
	LastFailureTime    time.Time     `json:"last_failure_time,omitempty"`
	LastRecoveryTime   time.Time     `json:"last_recovery_time,omitempty"`
	ResponseTimestamps []time.Time   `json:"response_timestamps,omitempty"`
}

// ProbeResult represents Kubernetes liveness/readiness probe results
// BR-HEALTH-002: MUST provide liveness and readiness probes for Kubernetes
type ProbeResult struct {
	ProbeType           string        `json:"probe_type"` // "liveness" or "readiness"
	IsHealthy           bool          `json:"is_healthy"`
	ComponentID         string        `json:"component_id"`
	ResponseTime        time.Duration `json:"response_time"`
	LastCheckTime       time.Time     `json:"last_check_time"`
	ConsecutivePasses   int           `json:"consecutive_passes"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
}

// DependencyStatus represents external dependency health status
// BR-HEALTH-003: MUST monitor external dependency health and availability
type DependencyStatus struct {
	BaseEntity
	IsAvailable    bool              `json:"is_available"`
	DependencyType string            `json:"dependency_type"`
	Endpoint       string            `json:"endpoint"`
	Criticality    string            `json:"criticality"`
	LastError      string            `json:"last_error,omitempty"`
	FailureCount   int               `json:"failure_count"`
	LastCheckTime  time.Time         `json:"last_check_time"`
	HealthMetrics  HealthMetrics     `json:"health_metrics"`
	Configuration  map[string]string `json:"configuration,omitempty"`
}

// =============================================================================
// LLM AI TYPES
// =============================================================================

// EnhancedActionRecommendation wraps a standard ActionRecommendation with validation and processing metadata
type EnhancedActionRecommendation struct {
	ActionRecommendation *ActionRecommendation `json:"action_recommendation"`
	ValidationResult     *LLMValidationResult  `json:"validation_result,omitempty"`
	ProcessingMetadata   *ProcessingMetadata   `json:"processing_metadata,omitempty"`
}

// LLMValidationResult contains validation information for an action recommendation
type LLMValidationResult struct {
	ValidationScore float64            `json:"validation_score"`
	RiskAssessment  *LLMRiskAssessment `json:"risk_assessment,omitempty"`
}

// LLMRiskAssessment provides risk analysis for an action (renamed to avoid conflicts with other RiskAssessment types)
type LLMRiskAssessment struct {
	RiskLevel          string  `json:"risk_level"`
	ReversibilityScore float64 `json:"reversibility_score"`
}

// ProcessingMetadata contains information about how the recommendation was processed
type ProcessingMetadata struct {
	ProcessingTime time.Duration `json:"processing_time"`
	AIModelUsed    string        `json:"ai_model_used"`
}
