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

// helmManagedConfig scripts the preferred-family E2E journey for #2478.
// The target is a real Helm-labeled Pod seeded by the KA E2E infrastructure;
// the workflow UUID is replaced with the seeded catalog UUID at deployment
// time through the existing workflow-name override mechanism.
func helmManagedConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "helm_managed", SignalName: "HelmManagedConfigFailure", Severity: "warning", ActionType: "HelmRollback",
		WorkflowName: "helm-rollback-v1", WorkflowID: uuid.DeterministicUUID("helm-rollback-v1"),
		WorkflowTitle: "HelmManaged Remediation - Rollback Release", Confidence: 0.95,
		Rationale:    "The target is Helm-managed, so rolling back the release is safer than a generic restart",
		RootCause:    "A Helm-managed release introduced an invalid configuration revision",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "helm-managed-pod", APIVersion: "v1",
		ExecutionEngine: "job", Contributing: []string{"helm_release_regression", "invalid_configuration"},
		InvestigationOutcome: "actionable", IsActionable: BoolPtr(true), ForceText: BoolPtr(false),
	}
}

func helmManagedScenario() *configScenario {
	return newSignalScenario("helm_managed", []string{"helmmanagedconfigfailure"}, helmManagedConfig())
}
