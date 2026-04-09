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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/logs"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/prometheus"
)

var resourceContextTools = []string{
	"get_namespaced_resource_context",
	"get_cluster_resource_context",
}

// DefaultPhaseToolMap returns the production phase-to-tool mapping.
// TodoWrite is available in all phases (matching KA CoreInvestigationToolset behavior).
// Resource context tools are in PhaseRCA (matching KA Phase 1).
func DefaultPhaseToolMap() katypes.PhaseToolMap {
	todo := investigation.ToolName

	rca := make([]string, 0, len(k8s.AllToolNames)+len(k8s.MetricsToolNames)+1+len(prometheus.AllToolNames)+len(resourceContextTools)+1)
	rca = append(rca, k8s.AllToolNames...)
	rca = append(rca, k8s.MetricsToolNames...)
	rca = append(rca, logs.ToolName)
	rca = append(rca, prometheus.AllToolNames...)
	rca = append(rca, resourceContextTools...)
	rca = append(rca, todo)

	wd := []string{
		"list_available_actions",
		"list_workflows",
		"get_workflow",
		todo,
	}

	return katypes.PhaseToolMap{
		katypes.PhaseRCA:               rca,
		katypes.PhaseWorkflowDiscovery: wd,
		katypes.PhaseValidation:        {todo},
	}
}
