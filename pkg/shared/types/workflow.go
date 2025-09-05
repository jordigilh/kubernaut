package types

import "time"

// Canonical workflow-related types used across multiple packages.
// These types resolve conflicts by providing authoritative definitions
// that consolidate the best aspects of previously duplicated types.

// WorkflowTemplate represents a workflow template with comprehensive metadata.
// This type consolidates definitions from:
// - pkg/intelligence/patterns/pattern_discovery_helpers.go
// - pkg/workflow/types/core.go
type WorkflowTemplate struct {
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
	Conditions   []WorkflowCondition    `json:"conditions,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	Dependencies []string               `json:"dependencies,omitempty"`
	Timeout      time.Duration          `json:"timeout,omitempty"`
}

// WorkflowCondition represents conditions for workflow execution.
type WorkflowCondition struct {
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
type WorkflowExecutionResult struct {
	Success           bool          `json:"success"`
	Duration          time.Duration `json:"duration"`
	StepsCompleted    int           `json:"steps_completed"`
	TotalSteps        int           `json:"total_steps,omitempty"`
	ErrorMessage      string        `json:"error_message,omitempty"`
	ResourcesAffected []string      `json:"resources_affected,omitempty"`
	StartTime         time.Time     `json:"start_time,omitempty"`
	EndTime           time.Time     `json:"end_time,omitempty"`
	ExecutionID       string        `json:"execution_id,omitempty"`
}

// WorkflowExecutionData represents comprehensive workflow execution data
// for analytics and learning purposes.
type WorkflowExecutionData struct {
	ExecutionID     string                   `json:"execution_id"`
	WorkflowID      string                   `json:"workflow_id"`
	TemplateID      string                   `json:"template_id,omitempty"`
	Timestamp       time.Time                `json:"timestamp"`
	Duration        time.Duration            `json:"duration"`
	Success         bool                     `json:"success"`
	ExecutionResult *WorkflowExecutionResult `json:"execution_result,omitempty"`
	ResourceUsage   *ResourceUsageData       `json:"resource_usage,omitempty"`
	Metrics         map[string]float64       `json:"metrics,omitempty"`
	Metadata        map[string]interface{}   `json:"metadata,omitempty"`
	Context         map[string]interface{}   `json:"context,omitempty"`
}
