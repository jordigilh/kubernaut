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

func oomkilledConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "oomkilled", SignalName: "OOMKilled", Severity: "critical",
		WorkflowName: "oomkill-increase-memory-v1", WorkflowID: uuid.DeterministicUUID("oomkill-increase-memory-v1"),
		WorkflowTitle: "OOMKill Recovery - Increase Memory Limits", Confidence: 0.95,
		Rationale:    "Container exceeded memory limits under traffic spike; increasing limits is the safest remediation with medium risk tolerance",
		RootCause:    "Container exceeded memory limits due to traffic spike",
		ResourceKind: "Deployment", ResourceNS: "production", ResourceName: "api-server",
		Parameters:   map[string]string{"MEMORY_LIMIT_NEW": "512Mi"}, ExecutionEngine: "job",
		Contributing: []string{"traffic_spike", "insufficient_memory_limits", "no_HPA_configured"},
		Alternatives: []MockAlternativeWorkflow{
			{WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"), Confidence: 0.60, Rationale: "Restart would temporarily resolve the OOM but doesn't address the underlying memory limit issue"},
		},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}
