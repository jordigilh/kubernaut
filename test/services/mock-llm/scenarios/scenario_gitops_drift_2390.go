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
