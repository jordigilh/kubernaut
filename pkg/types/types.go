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
	Reasoning  string                 `json:"reasoning,omitempty"`
}

// ActionResult represents the result of executing an action
type ActionResult struct {
	Action    string `json:"action"`
	Success   bool   `json:"success"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"timestamp"`
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
	"scale_deployment":    true,
	"restart_pod":         true,
	"increase_resources":  true,
	"notify_only":         true,
	"rollback_deployment": true,
	"expand_pvc":          true,
	"drain_node":          true,
	"quarantine_pod":      true,
	"collect_diagnostics": true,
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
