package types

import (
	"time"
)

// Alert represents a Prometheus alert
type Alert struct {
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Severity    string            `json:"severity"`
	Description string            `json:"description"`
	Namespace   string            `json:"namespace"`
	Resource    string            `json:"resource"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      *time.Time        `json:"ends_at,omitempty"`
}

// ActionRecommendation represents an action recommendation from the SLM
type ActionRecommendation struct {
	Action     string                 `json:"action"`
	Parameters map[string]interface{} `json:"parameters"`
	Confidence float64                `json:"confidence,omitempty"`
	Reasoning  *ReasoningDetails      `json:"reasoning,omitempty"` // Always structured reasoning
}

// ReasoningDetails provides structured reasoning information
type ReasoningDetails struct {
	PrimaryReason     string            `json:"primary_reason,omitempty"`
	HistoricalContext string            `json:"historical_context,omitempty"`
	OscillationRisk   string            `json:"oscillation_risk,omitempty"`
	AlternativeActions []string         `json:"alternative_actions,omitempty"`
	ConfidenceFactors map[string]float64 `json:"confidence_factors,omitempty"`
	Summary           string            `json:"summary,omitempty"`
}

// ActionResult represents the result of executing an action
type ActionResult struct {
	Action    string `json:"action"`
	Success   bool   `json:"success"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"timestamp"`
}

// ExecutionResult represents the detailed result of action execution
type ExecutionResult struct {
	Action    string        `json:"action"`
	Success   bool          `json:"success"`
	Output    string        `json:"output,omitempty"`
	Error     error         `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// WebhookRequest represents an incoming webhook request
type WebhookRequest struct {
	Version  string  `json:"version"`
	GroupKey string  `json:"groupKey"`
	Status   string  `json:"status"`
	Receiver string  `json:"receiver"`
	Alerts   []Alert `json:"alerts"`
}

// ProcessingResult represents the result of processing an alert
type ProcessingResult struct {
	Alert          Alert                 `json:"alert"`
	Recommendation *ActionRecommendation `json:"recommendation,omitempty"`
	ActionResult   *ActionResult         `json:"action_result,omitempty"`
	ProcessingTime time.Duration         `json:"processing_time"`
	Error          string                `json:"error,omitempty"`
}

// ValidActions contains all supported action types
var ValidActions = map[string]bool{
	// Core actions
	"scale_deployment":    true,
	"restart_pod":         true,
	"increase_resources":  true,
	"notify_only":         true,
	"rollback_deployment": true,
	"expand_pvc":          true,
	"drain_node":          true,
	"quarantine_pod":      true,
	"collect_diagnostics": true,
	
	// Storage & Persistence actions
	"cleanup_storage":     true,
	"backup_data":         true,
	"compact_storage":     true,
	
	// Application Lifecycle actions
	"cordon_node":         true,
	"update_hpa":          true,
	"restart_daemonset":   true,
	
	// Security & Compliance actions
	"rotate_secrets":      true,
	"audit_logs":          true,
	
	// Network & Connectivity actions  
	"update_network_policy": true,
	"restart_network":       true,
	"reset_service_mesh":    true,
	
	// Database & Stateful actions
	"failover_database":   true,
	"repair_database":     true,
	"scale_statefulset":   true,
	
	// Monitoring & Observability actions
	"enable_debug_mode":   true,
	"create_heap_dump":    true,
	
	// Resource Management actions
	"optimize_resources":  true,
	"migrate_workload":    true,
}

// GetValidActionsList returns a slice of valid actions for validation
func GetValidActionsList() []string {
	actions := make([]string, 0, len(ValidActions))
	for action := range ValidActions {
		actions = append(actions, action)
	}
	return actions
}

// IsValidAction checks if the given action is valid
func IsValidAction(action string) bool {
	return ValidActions[action]
}
