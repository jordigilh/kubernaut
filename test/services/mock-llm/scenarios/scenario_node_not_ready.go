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

func nodeNotReadyConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "node_not_ready", SignalName: "NodeNotReady", Severity: "critical",
		WorkflowName: "node-drain-reboot-v1", WorkflowID: uuid.DeterministicUUID("node-drain-reboot-v1"),
		WorkflowTitle: "NodeNotReady - Drain and Reboot", Confidence: 0.90,
		Rationale:    "Node is experiencing persistent disk pressure that hasn't self-resolved; drain and reboot is the standard remediation",
		RootCause:    "Node experiencing disk pressure causing NotReady condition",
		ResourceKind: "Node", ResourceNS: "", ResourceName: "worker-node-1",
		Parameters:   map[string]string{"NODE_NAME": "worker-node-1"},
		Contributing: []string{"disk_pressure", "ephemeral_storage_full", "log_rotation_stalled"},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}
