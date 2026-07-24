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
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// defaultListWorkflowsLimit mirrors DS's HandleListWorkflows default page
// size (pkg/datastorage/server/workflow_query_handlers.go's
// parseListPagination), preserving response-size parity for the now
// KA-backed kubernaut_list_workflows tool (#1677 Phase 2f, DD-WORKFLOW-019).
// This tool exposes no pagination args of its own -- same as AF's prior
// DS-backed ds.Client.ListWorkflows call, which never sent limit/offset.
const defaultListWorkflowsLimit = 50

// ListWorkflowsInput defines the input schema for the kubernaut_list_workflows
// MCP tool. Unlike select_workflow/investigate, this is a stateless catalog
// browse -- no rr_id or active session required.
type ListWorkflowsInput struct {
	// Kind filters by Kubernetes resource kind (e.g. "Deployment", "Pod"),
	// mapped onto WorkflowSearchFilters.Component.
	Kind string `json:"kind,omitempty"`
}

// CatalogWorkflowSummary is a compact view of a cataloged workflow, mirroring
// the shape AF's ds.Workflow previously returned for this tool.
type CatalogWorkflowSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

// ListWorkflowsOutput defines the output schema for the kubernaut_list_workflows
// MCP tool.
type ListWorkflowsOutput struct {
	Workflows []CatalogWorkflowSummary `json:"workflows"`
	Count     int                      `json:"count"`
}

// WorkflowLister abstracts workflowcatalog.Catalog.List for testability,
// avoiding an import-cycle-prone direct dependency on the concrete type.
type WorkflowLister interface {
	List(ctx context.Context, filters *models.WorkflowSearchFilters, limit, offset int) ([]models.RemediationWorkflow, int, error)
}

// ListWorkflowsTool handles the kubernaut_list_workflows MCP tool: a
// stateless, unfiltered-by-default catalog browse distinct from the
// action-type-scoped 3-step discovery protocol (list_available_actions ->
// list_workflows -> get_workflow in internal/kubernautagent/tools/custom).
// #1677 Phase 2f (DD-WORKFLOW-019): backed by workflowcatalog.Catalog.List,
// replacing DS's GET /api/v1/workflows as AF's kubernaut_list_workflows
// backend.
type ListWorkflowsTool struct {
	catalog WorkflowLister
}

// NewListWorkflowsTool creates the tool handler with its catalog dependency.
func NewListWorkflowsTool(catalog WorkflowLister) *ListWorkflowsTool {
	return &ListWorkflowsTool{catalog: catalog}
}

// Handle executes the catalog browse, translating Kind into the
// Component search filter and narrowing each result to the summary fields
// exposed by this tool (id, name, description, kind) -- avoids leaking raw
// workflow Content/Labels to callers, consistent with the DTO-narrowing
// applied by the discovery-protocol tools (Phase 2d).
func (t *ListWorkflowsTool) Handle(ctx context.Context, input ListWorkflowsInput) (ListWorkflowsOutput, error) {
	if t.catalog == nil {
		return ListWorkflowsOutput{}, ErrCodeInternalError.WithDetail("reason", "workflow catalog unavailable")
	}

	filters := &models.WorkflowSearchFilters{Component: input.Kind}
	workflows, _, err := t.catalog.List(ctx, filters, defaultListWorkflowsLimit, 0)
	if err != nil {
		return ListWorkflowsOutput{}, fmt.Errorf("listing workflows: %w", err)
	}

	summaries := make([]CatalogWorkflowSummary, 0, len(workflows))
	for i := range workflows {
		w := &workflows[i]
		summaries = append(summaries, CatalogWorkflowSummary{
			ID:          w.WorkflowID,
			Name:        w.WorkflowName,
			Description: w.Description.What,
			Kind:        w.ActionType,
		})
	}

	return ListWorkflowsOutput{Workflows: summaries, Count: len(summaries)}, nil
}
