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
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

var listAvailableActionsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"page": {"type": "string", "enum": ["next", "previous"], "description": "Navigation direction. Omit on first call."},
		"cursor": {"type": "string", "description": "Opaque cursor from previous response. Required when page is set."}
	},
	"additionalProperties": false
}`)

var listWorkflowsSchemaJSON = json.RawMessage(`{
	"type": "object",
	"properties": {
		"action_type": {"type": "string", "description": "The action type to filter workflows by"},
		"page": {"type": "string", "enum": ["next", "previous"], "description": "Navigation direction. Omit on first call."},
		"cursor": {"type": "string", "description": "Opaque cursor from previous response. Required when page is set."}
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

// WorkflowDiscoveryClient is the subset of the ogen-generated DS client used by the
// three workflow discovery tools. Satisfied by *ogenclient.Client. Defined here
// so unit tests can substitute a lightweight fake without an HTTP server.
type WorkflowDiscoveryClient interface {
	ListAvailableActions(ctx context.Context, params ogenclient.ListAvailableActionsParams) (ogenclient.ListAvailableActionsRes, error)
	ListWorkflowsByActionType(ctx context.Context, params ogenclient.ListWorkflowsByActionTypeParams) (ogenclient.ListWorkflowsByActionTypeRes, error)
	GetWorkflowByID(ctx context.Context, params ogenclient.GetWorkflowByIDParams) (ogenclient.GetWorkflowByIDRes, error)
}

// AllToolNames lists the 5 custom tool names for DataStorage interaction and resource context.
var AllToolNames = []string{
	"list_available_actions",
	"list_workflows",
	"get_workflow",
	"get_namespaced_resource_context",
	"get_cluster_resource_context",
}

// NewAllTools creates the 3 custom tools using any WorkflowDiscoveryClient.
// Pass *ogenclient.Client in production or a fake in tests.
func NewAllTools(ds WorkflowDiscoveryClient) []tools.Tool {
	return []tools.Tool{
		&listActionsTool{ds: ds},
		&listWorkflowsTool{ds: ds},
		&getWorkflowTool{ds: ds},
	}
}

// RegisterAll registers all 5 custom tools (3 DS workflow tools + 2 resource context tools)
// into the given registry. Pass nil for any dependency to create tools that will fail at
// execution time rather than registration time.
func RegisterAll(reg *registry.Registry, dsOgenClient WorkflowDiscoveryClient, dsClient enrichment.DataStorageClient, k8sClient enrichment.K8sClient) {
	for _, t := range NewAllTools(dsOgenClient) {
		reg.Register(t)
	}
	reg.Register(NewNamespacedResourceContextTool(dsClient, k8sClient))
	reg.Register(NewClusterResourceContextTool(dsClient, k8sClient))
}

// --- list_available_actions ---

type listActionsTool struct{ ds WorkflowDiscoveryClient }

func (t *listActionsTool) Name() string               { return "list_available_actions" }
func (t *listActionsTool) Description() string         { return "List available remediation action types from DataStorage" }
func (t *listActionsTool) Parameters() json.RawMessage { return listAvailableActionsSchema }

func (t *listActionsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Page   string `json:"page"`
		Cursor string `json:"cursor"`
	}
	_ = json.Unmarshal(args, &a)

	params := ogenclient.ListAvailableActionsParams{
		Severity:    "critical",
		Component:   "deployment",
		Environment: "production",
		Priority:    "P0",
	}
	if a.Page != "" && a.Cursor != "" {
		offset, limit := DecodeCursor(a.Cursor)
		params.Offset = ogenclient.NewOptInt(offset)
		params.Limit = ogenclient.NewOptInt(limit)
	}

	res, err := t.ds.ListAvailableActions(ctx, params)
	if err != nil {
		return "", fmt.Errorf("listing action types: %w", err)
	}

	data, _ := json.Marshal(res)
	return string(TransformPagination(data)), nil
}

// --- list_workflows ---

type listWorkflowsTool struct{ ds WorkflowDiscoveryClient }

func (t *listWorkflowsTool) Name() string               { return "list_workflows" }
func (t *listWorkflowsTool) Description() string         { return "Search for workflows by action type in DataStorage" }
func (t *listWorkflowsTool) Parameters() json.RawMessage { return listWorkflowsSchemaJSON }

func (t *listWorkflowsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		ActionType string `json:"action_type"`
		Page       string `json:"page"`
		Cursor     string `json:"cursor"`
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
	if a.Page != "" && a.Cursor != "" {
		offset, limit := DecodeCursor(a.Cursor)
		params.Offset = ogenclient.NewOptInt(offset)
		params.Limit = ogenclient.NewOptInt(limit)
	}

	res, err := t.ds.ListWorkflowsByActionType(ctx, params)
	if err != nil {
		return "", fmt.Errorf("listing workflows: %w", err)
	}

	data, _ := json.Marshal(res)
	return string(TransformPagination(data)), nil
}

// --- get_workflow ---

type getWorkflowTool struct{ ds WorkflowDiscoveryClient }

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

// StripPaginationIfComplete removes the "pagination" field from a JSON response
// when hasMore is false, meaning all results fit in one page. This avoids the
// LLM wasting tool calls trying to paginate when there are no more pages.
// When hasMore is true, the pagination metadata is preserved so the LLM knows
// it is seeing a subset.
func StripPaginationIfComplete(data json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return data
	}

	paginationRaw, ok := obj["pagination"]
	if !ok {
		return data
	}

	var pagination struct {
		HasMore bool `json:"hasMore"`
	}
	if err := json.Unmarshal(paginationRaw, &pagination); err != nil {
		return data
	}

	if pagination.HasMore {
		return data
	}

	delete(obj, "pagination")
	result, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return result
}

const defaultPaginationLimit = 10
const maxPaginationLimit = 100

type cursorPayload struct {
	Offset int `json:"o"`
	Limit  int `json:"l"`
}

// EncodeCursor encodes offset and limit into an opaque base64-URL cursor token.
// DD-WORKFLOW-016 v1.4: cursors hide pagination implementation from the LLM.
func EncodeCursor(offset, limit int) string {
	b, _ := json.Marshal(cursorPayload{Offset: offset, Limit: limit})
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor decodes an opaque cursor token into offset and limit.
// Returns safe defaults (0, 10) on any failure (invalid base64, non-JSON, tampered values).
// Mirrors DS ParsePagination clamping for defense-in-depth.
func DecodeCursor(token string) (offset int, limit int) {
	if token == "" {
		return 0, defaultPaginationLimit
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, defaultPaginationLimit
	}

	var p cursorPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return 0, defaultPaginationLimit
	}

	if p.Offset < 0 {
		p.Offset = 0
	}
	if p.Limit <= 0 {
		p.Limit = defaultPaginationLimit
	}
	if p.Limit > maxPaginationLimit {
		p.Limit = maxPaginationLimit
	}

	return p.Offset, p.Limit
}

// TransformPagination converts DS PaginationMetadata (totalCount, offset, limit, hasMore)
// into LLM-facing cursor-based pagination (hasNext, nextCursor, hasPrevious, previousCursor).
// Single-page results (offset=0, hasMore=false) have pagination stripped entirely.
// DD-WORKFLOW-016 v1.4: totalCount is never exposed to the LLM.
func TransformPagination(data json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return data
	}

	paginationRaw, ok := obj["pagination"]
	if !ok {
		return data
	}

	var dsPag struct {
		TotalCount int  `json:"totalCount"`
		Offset     int  `json:"offset"`
		Limit      int  `json:"limit"`
		HasMore    bool `json:"hasMore"`
	}
	if err := json.Unmarshal(paginationRaw, &dsPag); err != nil {
		return data
	}

	if dsPag.Offset == 0 && !dsPag.HasMore {
		delete(obj, "pagination")
		result, err := json.Marshal(obj)
		if err != nil {
			return data
		}
		return result
	}

	llmPag := make(map[string]interface{})

	if dsPag.HasMore {
		llmPag["hasNext"] = true
		llmPag["nextCursor"] = EncodeCursor(dsPag.Offset+dsPag.Limit, dsPag.Limit)
	}

	if dsPag.Offset > 0 {
		prevOffset := dsPag.Offset - dsPag.Limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		llmPag["hasPrevious"] = true
		llmPag["previousCursor"] = EncodeCursor(prevOffset, dsPag.Limit)
	}

	transformed, err := json.Marshal(llmPag)
	if err != nil {
		return data
	}
	obj["pagination"] = transformed

	result, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return result
}
