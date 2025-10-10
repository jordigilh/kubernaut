<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
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
