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

// standaloneExecClusterIDConfig backs E2E-FP-2378-001. The response selects
// the workflow only; execution.clusterId is catalog-authoritative and must not
// be supplied by the mock LLM.
func standaloneExecClusterIDConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName:         "standalone_exec_cluster_id_2378",
		SignalName:           "StandaloneExecutionCluster2378",
		Severity:             "critical",
		WorkflowName:         "standalone-exec-cluster-id-v1",
		WorkflowID:           uuid.DeterministicUUID("standalone-exec-cluster-id-v1"),
		WorkflowTitle:        "Standalone Execution Cluster ID - Increase Memory Limits",
		Confidence:           0.9,
		Rationale:            "E2E-FP-2378-001 fixture: catalog execution.clusterId must be preserved while standalone execution remains local",
		RootCause:            "E2E-FP-2378-001 synthetic standalone routing fixture",
		ResourceKind:         "Deployment",
		ResourceNS:           "kubernaut-system",
		ResourceName:         "memory-eater",
		APIVersion:           "apps/v1",
		Parameters:           map[string]string{"MEMORY_LIMIT_NEW": "512Mi"},
		ExecutionEngine:      "job",
		Contributing:         []string{"e2e_fp_2378_fixture"},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}
