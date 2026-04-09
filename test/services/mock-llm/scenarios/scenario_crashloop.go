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

func crashloopConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "crashloop", SignalName: "CrashLoopBackOff", Severity: "high",
		WorkflowName: "crashloop-config-fix-v1", WorkflowID: uuid.DeterministicUUID("crashloop-config-fix-v1"),
		WorkflowTitle: "CrashLoopBackOff - Configuration Fix", Confidence: 0.95,
		Rationale:    "Configuration regression introduced in recent deployment revision; rollback to last known good revision is the safest approach with medium risk tolerance",
		RootCause:    "Container failing due to invalid configuration directive introduced in recent deployment update",
		ResourceKind: "Deployment", ResourceNS: "staging", ResourceName: "worker",
		Parameters:   map[string]string{"NAMESPACE": "staging", "DEPLOYMENT_NAME": "worker"},
		Contributing: []string{"invalid_configuration_directive", "recent_deployment_update", "application_fails_validation_on_startup"},
		Alternatives: []MockAlternativeWorkflow{
			{WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"), Confidence: 0.60, Rationale: "Restart-only approach would be faster but doesn't address the root cause (bad configuration)"},
		},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
		ExecutionEngine:      "job",
	}
}
