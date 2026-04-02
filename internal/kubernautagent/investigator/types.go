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

package investigator

import (
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/investigation"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/prometheus"
)

// Resource context tools belong in RCA phase (matching HAPI behavior).
var resourceContextTools = []string{
	"get_namespaced_resource_context",
	"get_cluster_resource_context",
}

// Workflow discovery tools belong in WorkflowDiscovery phase.
var workflowDiscoveryTools = []string{
	"list_available_actions",
	"list_workflows",
	"get_workflow",
}

// DefaultPhaseToolMap returns the production phase-to-tool mapping.
// PhaseRCA: K8s + Prometheus + resource context tools.
// PhaseWorkflowDiscovery: workflow discovery tools.
// TodoWrite is available in all phases (matching HAPI CoreInvestigationToolset behavior).
func DefaultPhaseToolMap() katypes.PhaseToolMap {
	todo := investigation.ToolName

	rca := make([]string, 0, len(k8s.AllToolNames)+len(prometheus.AllToolNames)+len(resourceContextTools)+1)
	rca = append(rca, k8s.AllToolNames...)
	rca = append(rca, prometheus.AllToolNames...)
	rca = append(rca, resourceContextTools...)
	rca = append(rca, todo)

	wd := make([]string, 0, len(workflowDiscoveryTools)+1)
	wd = append(wd, workflowDiscoveryTools...)
	wd = append(wd, todo)

	return katypes.PhaseToolMap{
		katypes.PhaseRCA:               rca,
		katypes.PhaseWorkflowDiscovery: wd,
		katypes.PhaseValidation:        {todo},
	}
}
