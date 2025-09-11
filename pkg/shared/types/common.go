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
var ValidActions = map[string]bool{
	// Core actions
	"scale_deployment":      true,
	"restart_pod":           true,
	"increase_resources":    true,
	"notify_only":           true,
	"rollback_deployment":   true,
	"expand_pvc":            true,
	"drain_node":            true,
	"quarantine_pod":        true,
	"collect_diagnostics":   true,
	"cleanup_storage":       true,
	"backup_data":           true,
	"compact_storage":       true,
	"update_network_policy": true,
	"restart_network":       true,
	"rotate_secrets":        true,
	"audit_logs":            true,
	"migrate_workload":      true,

	// Extended actions for enhanced functionality
	"optimize_resources":       true,
	"enable_autoscaling":       true,
	"update_deployment":        true,
	"patch_resource":           true,
	"create_resource":          true,
	"delete_resource":          true,
	"scale_node_pool":          true,
	"update_node_labels":       true,
	"cordon_node":              true,
	"uncordon_node":            true,
	"taint_node":               true,
	"untaint_node":             true,
	"restart_service":          true,
	"update_service":           true,
	"create_configmap":         true,
	"update_configmap":         true,
	"create_secret":            true,
	"update_secret":            true,
	"enable_monitoring":        true,
	"disable_monitoring":       true,
	"update_resource_quota":    true,
	"create_network_policy":    true,
	"update_ingress":           true,
	"create_persistent_volume": true,
	"resize_persistent_volume": true,
}

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
