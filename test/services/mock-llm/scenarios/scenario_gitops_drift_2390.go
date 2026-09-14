/*
Copyright 2026 Jordi Gil.

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

package scenarios

import "github.com/jordigilh/kubernaut/pkg/shared/uuid"

// gitopsSelectWorkflow2390Config drives the explicit workflow-selection turn
// in the Issue #2390 A2A journey. It is registered before broad AF keyword
// scenarios so prior-turn remediation text cannot select the generic workflow.
func gitopsSelectWorkflow2390Config() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName:   "af_select_gitops_workflow_2390",
		ToolCallName:   "kubernaut_select_workflow",
		ToolCallArgs:   map[string]interface{}{"rr_id": "$from_tool:kubernaut_remediate:rr_id", "workflow_id": uuid.DeterministicUUID("gitops-drift-2390-v1")},
		ForceText:      BoolPtr(false),
		RepeatToolCall: true,
	}
}

// gitopsDrift2390Config backs the interactive GitOps snapshot journey. The
// catalog, not the mock response, supplies dependencies and execution policy.
func gitopsDrift2390Config() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName:         "gitops_drift_2390",
		SignalName:           "GitOpsDrift2390",
		Severity:             "critical",
		WorkflowName:         "gitops-drift-2390-v1",
		WorkflowID:           uuid.DeterministicUUID("gitops-drift-2390-v1"),
		WorkflowTitle:        "GitOps Drift Remediation",
		Confidence:           0.95,
		Rationale:            "The GitOps remediation workflow is the catalog-authoritative fix for this drift",
		RootCause:            "The GitOps-managed Deployment has drifted from its repository state",
		ResourceKind:         "Deployment",
		ResourceNS:           "production",
		ResourceName:         "memory-eater",
		APIVersion:           "apps/v1",
		Parameters:           map[string]string{"MEMORY_LIMIT_NEW": "512Mi"},
		ExecutionEngine:      "job",
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}
