/*
Copyright 2025 Jordi Gil.

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
