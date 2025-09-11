package types

// ConditionType defines the type of workflow condition
type ConditionType string

const (
	ConditionTypeMetric     ConditionType = "metric"
	ConditionTypeResource   ConditionType = "resource"
	ConditionTypeTime       ConditionType = "time"
	ConditionTypeExpression ConditionType = "expression"
	ConditionTypeCustom     ConditionType = "custom"
)

// ConditionExpression represents a condition expression that must be evaluated
type ConditionExpression struct {
	Type       ConditionType          `json:"type"`
	Expression string                 `json:"expression"`
	Variables  map[string]interface{} `json:"variables"`
}

// StepContext provides context information for step execution
type StepContext struct {
	StepID      string                 `json:"step_id"`
	WorkflowID  string                 `json:"workflow_id"`
	ExecutionID string                 `json:"execution_id"`
	Variables   map[string]interface{} `json:"variables"`
}
