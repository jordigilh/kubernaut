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

// fleetExecClusterOverrideConfig backs E2E-FLEET-2326-001 (DD-FLEET-008,
// BR-FLEET-004, Issue #2326): a dedicated, fully-isolated workflow name so
// this scenario can never be matched by -- or interfere with -- any other
// Fleet E2E test's alert traffic. The workflow itself declares
// execution.clusterId: prod-east in its fixture
// (test/fixtures/workflows/fleet-exec-cluster-override/workflow-schema.yaml).
// ExecutionClusterID is catalog-authoritative (never LLM-suppliable, see
// investigator_gates.go's enrichFromCatalog), so this mock response only
// needs to select the workflow -- the cross-cluster assertion is proven by
// KA's real catalog cache reading the real CRD, not by anything returned
// here.
func fleetExecClusterOverrideConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "fleet_exec_cluster_override", SignalName: "FleetExecClusterOverride2326", Severity: "critical",
		WorkflowName: "fleet-exec-cluster-override-v1", WorkflowID: uuid.DeterministicUUID("fleet-exec-cluster-override-v1"),
		WorkflowTitle: "Fleet Exec Cluster Override - Increase Memory Limits", Confidence: 0.9,
		Rationale:    "E2E-FLEET-2326-001 fixture: selected to prove WorkflowExecution.Spec.ClusterID follows the workflow's declared execution cluster, not the signal's origin cluster",
		RootCause:    "E2E-FLEET-2326-001 synthetic fixture signal (DD-FLEET-008 cross-cluster execution routing coverage)",
		ResourceKind: "Deployment", ResourceNS: "default", ResourceName: "memory-eater",
		APIVersion:           "apps/v1",
		Parameters:           map[string]string{"MEMORY_LIMIT_NEW": "512Mi"},
		ExecutionEngine:      "job",
		Contributing:         []string{"e2e_fleet_2326_fixture"},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}
