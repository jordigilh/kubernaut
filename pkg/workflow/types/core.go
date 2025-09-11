package types

import "time"

// CoreWorkflowExecutionResult represents the result of a workflow execution
type CoreWorkflowExecutionResult struct {
	Success           bool          `json:"success"`
	Duration          time.Duration `json:"duration"`
	StepsCompleted    int           `json:"steps_completed"`
	ErrorMessage      string        `json:"error_message,omitempty"`
	ResourcesAffected []string      `json:"resources_affected"`
}

// RiskFactor identifies potential risks in analysis context
type RiskFactor struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Probability float64 `json:"probability"`
	Impact      string  `json:"impact"` // "low", "medium", "high", "critical"
	Mitigation  string  `json:"mitigation"`
}
