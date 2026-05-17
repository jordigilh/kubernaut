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

package tools

import (
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// BuildFinalResult exposes buildFinalResult for external test packages.
var BuildFinalResult = func(rca *katypes.InvestigationResult, workflow *CatalogWorkflow, discovery *mcpinternal.WorkflowDiscoveryResult) *katypes.InvestigationResult {
	return buildFinalResult(rca, workflow, discovery)
}

// IsWorkflowInDiscoveryResult exposes isWorkflowInDiscoveryResult for external test packages.
var IsWorkflowInDiscoveryResult = func(workflowID string, dr *mcpinternal.WorkflowDiscoveryResult) bool {
	return isWorkflowInDiscoveryResult(workflowID, dr)
}

// ExtractDiscoveryResult exposes extractDiscoveryResult for external test packages.
var ExtractDiscoveryResult = func(result *katypes.InvestigationResult) *mcpinternal.WorkflowDiscoveryResult {
	return extractDiscoveryResult(result)
}
