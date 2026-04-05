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

package remediationrequest

import "fmt"

// FormatResourceDisplay produces a Kubernetes-idiomatic Kind/Name string
// for use in printer columns. Returns empty string if name is empty.
func FormatResourceDisplay(kind, name string) string {
	if name == "" {
		return ""
	}
	if kind == "" {
		return name
	}
	return kind + "/" + name
}

// FormatWorkflowDisplay produces an ActionType:WorkflowID string
// for human-readable workflow identification. Returns empty string if
// workflowID is empty.
func FormatWorkflowDisplay(actionType, workflowID string) string {
	if workflowID == "" {
		return ""
	}
	if actionType == "" {
		return workflowID
	}
	return actionType + ":" + workflowID
}

// FormatConfidence formats a float64 confidence score as a 2-decimal string.
// Returns empty string for invalid (negative) values.
func FormatConfidence(confidence float64) string {
	if confidence < 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", confidence)
}
