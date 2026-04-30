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

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

// WorkflowCatalog abstracts the workflow catalog lookup for testability.
type WorkflowCatalog interface {
	GetWorkflowByID(ctx context.Context, workflowID string) (*CatalogWorkflow, error)
}

// CatalogWorkflow represents the essential fields from a DataStorage workflow
// entry needed for the interactive selection response.
type CatalogWorkflow struct {
	WorkflowID         string `json:"workflow_id"`
	WorkflowName       string `json:"workflow_name"`
	ActionType         string `json:"action_type"`
	Version            string `json:"version"`
	ExecutionEngine    string `json:"execution_engine,omitempty"`
	ExecutionBundle    string `json:"execution_bundle,omitempty"`
	ServiceAccountName string `json:"service_account_name,omitempty"`
}

// SelectWorkflowInput defines the input schema for the kubernaut_select_workflow MCP tool.
type SelectWorkflowInput struct {
	RRID       string `json:"rr_id"`
	WorkflowID string `json:"workflow_id"`
}

// SelectWorkflowOutput defines the output schema for the kubernaut_select_workflow MCP tool.
type SelectWorkflowOutput struct {
	Status     string           `json:"status"`
	Workflow   *CatalogWorkflow `json:"workflow,omitempty"`
	Confidence float64          `json:"confidence"`
	Rationale  string           `json:"rationale"`
}

// SelectWorkflowTool handles the kubernaut_select_workflow MCP tool.
// BR-INTERACTIVE-005: enables interactive workflow selection.
type SelectWorkflowTool struct {
	catalog  WorkflowCatalog
	sessions mcpinternal.SessionManager
}

// NewSelectWorkflowTool creates the tool handler with its dependencies.
func NewSelectWorkflowTool(catalog WorkflowCatalog, sessions mcpinternal.SessionManager) *SelectWorkflowTool {
	return &SelectWorkflowTool{catalog: catalog, sessions: sessions}
}

// Handle executes the workflow selection after validating input and session.
func (t *SelectWorkflowTool) Handle(ctx context.Context, input SelectWorkflowInput, user mcpinternal.UserInfo) (SelectWorkflowOutput, error) {
	if err := validateSelectWorkflowInput(input); err != nil {
		return SelectWorkflowOutput{}, err
	}

	if !t.sessions.IsDriverActive(input.RRID) {
		return SelectWorkflowOutput{}, fmt.Errorf("no active interactive session for rr_id")
	}

	driver, err := t.sessions.GetDriver(input.RRID)
	if err != nil || driver == nil {
		return SelectWorkflowOutput{}, fmt.Errorf("no active interactive session for rr_id")
	}

	if driver.ActingUser.Username != user.Username {
		return SelectWorkflowOutput{}, fmt.Errorf("caller is not the active driver for this session")
	}

	workflow, err := t.catalog.GetWorkflowByID(ctx, input.WorkflowID)
	if err != nil {
		return SelectWorkflowOutput{}, fmt.Errorf("workflow catalog lookup failed: %w", err)
	}

	return SelectWorkflowOutput{
		Status:     "workflow_selected",
		Workflow:   workflow,
		Confidence: 1.0,
		Rationale:  "User-selected via interactive mode",
	}, nil
}

func validateSelectWorkflowInput(input SelectWorkflowInput) error {
	if input.RRID == "" {
		return fmt.Errorf("rr_id is required")
	}
	if input.WorkflowID == "" {
		return fmt.Errorf("workflow_id is required")
	}
	return nil
}
