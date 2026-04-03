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

package custom

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

var listAvailableActionsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {},
	"additionalProperties": false
}`)

var listWorkflowsSchemaJSON = json.RawMessage(`{
	"type": "object",
	"properties": {
		"action_type": {"type": "string", "description": "The action type to filter workflows by"},
		"offset":      {"type": "integer", "description": "Pagination offset"},
		"limit":       {"type": "integer", "description": "Maximum number of results to return"}
	},
	"required": ["action_type"]
}`)

var getWorkflowSchemaJSON = json.RawMessage(`{
	"type": "object",
	"properties": {
		"workflow_id": {"type": "string", "description": "UUID of the workflow to retrieve"}
	},
	"required": ["workflow_id"]
}`)

// ListAvailableActionsSchema returns the JSON schema for list_available_actions.
func ListAvailableActionsSchema() json.RawMessage { return listAvailableActionsSchema }

// ListWorkflowsSchema returns the JSON schema for list_workflows.
func ListWorkflowsSchema() json.RawMessage { return listWorkflowsSchemaJSON }

// GetWorkflowSchema returns the JSON schema for get_workflow.
func GetWorkflowSchema() json.RawMessage { return getWorkflowSchemaJSON }

// AllToolNames lists the 5 custom tool names for DataStorage interaction and resource context.
var AllToolNames = []string{
	"list_available_actions",
	"list_workflows",
	"get_workflow",
	"get_namespaced_resource_context",
	"get_cluster_resource_context",
}

// NewAllTools creates the 3 custom tools using the ogen-generated DS client.
func NewAllTools(ds *ogenclient.Client) []tools.Tool {
	return []tools.Tool{
		&listActionsTool{ds: ds},
		&listWorkflowsTool{ds: ds},
		&getWorkflowTool{ds: ds},
	}
}

// --- list_available_actions ---

type listActionsTool struct{ ds *ogenclient.Client }

func (t *listActionsTool) Name() string               { return "list_available_actions" }
func (t *listActionsTool) Description() string         { return "List available remediation action types from DataStorage" }
func (t *listActionsTool) Parameters() json.RawMessage { return listAvailableActionsSchema }

func (t *listActionsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	res, err := t.ds.ListAvailableActions(ctx, ogenclient.ListAvailableActionsParams{
		Severity:    "critical",
		Component:   "deployment",
		Environment: "production",
		Priority:    "P0",
	})
	if err != nil {
		return "", fmt.Errorf("listing action types: %w", err)
	}

	data, _ := json.Marshal(res)
	return string(data), nil
}

// --- list_workflows ---

type listWorkflowsTool struct{ ds *ogenclient.Client }

func (t *listWorkflowsTool) Name() string               { return "list_workflows" }
func (t *listWorkflowsTool) Description() string         { return "Search for workflows by action type in DataStorage" }
func (t *listWorkflowsTool) Parameters() json.RawMessage { return listWorkflowsSchemaJSON }

func (t *listWorkflowsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		ActionType string `json:"action_type"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	params := ogenclient.ListWorkflowsByActionTypeParams{
		ActionType:  a.ActionType,
		Severity:    "critical",
		Component:   "deployment",
		Environment: "production",
		Priority:    "P0",
	}

	res, err := t.ds.ListWorkflowsByActionType(ctx, params)
	if err != nil {
		return "", fmt.Errorf("listing workflows: %w", err)
	}

	data, _ := json.Marshal(res)
	return string(data), nil
}

// --- get_workflow ---

type getWorkflowTool struct{ ds *ogenclient.Client }

func (t *getWorkflowTool) Name() string               { return "get_workflow" }
func (t *getWorkflowTool) Description() string         { return "Get a specific workflow definition by ID" }
func (t *getWorkflowTool) Parameters() json.RawMessage { return getWorkflowSchemaJSON }

func (t *getWorkflowTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		WorkflowID string `json:"workflow_id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	uid, err := uuid.Parse(a.WorkflowID)
	if err != nil {
		return "", fmt.Errorf("invalid workflow ID %q: %w", a.WorkflowID, err)
	}

	res, err := t.ds.GetWorkflowByID(ctx, ogenclient.GetWorkflowByIDParams{
		WorkflowID: uid,
	})
	if err != nil {
		return "", fmt.Errorf("getting workflow: %w", err)
	}

	data, _ := json.Marshal(res)
	return string(data), nil
}
