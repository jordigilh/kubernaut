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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// AllToolNames lists the 5 custom tool names matching HAPI v1.2 tool surface.
var AllToolNames = []string{
	"list_available_actions",
	"list_workflows",
	"get_workflow",
	"get_namespaced_resource_context",
	"get_cluster_resource_context",
}

// NewAllTools creates the 5 custom tools using the ogen-generated DS client directly.
func NewAllTools(ds *ogenclient.Client, k8s enrichment.K8sClient, dsEnrich enrichment.DataStorageClient) []tools.Tool {
	return []tools.Tool{
		&listActionsTool{ds: ds},
		&listWorkflowsTool{ds: ds},
		&getWorkflowTool{ds: ds},
		&namespacedResourceContextTool{dsEnrich: dsEnrich, k8s: k8s},
		&clusterResourceContextTool{dsEnrich: dsEnrich},
	}
}

// --- list_available_actions ---

type listActionsTool struct{ ds *ogenclient.Client }

var listActionsParams = json.RawMessage(`{"type":"object","properties":{"offset":{"type":"integer","description":"Pagination offset"},"limit":{"type":"integer","description":"Pagination limit"}},"required":[]}`)

func (t *listActionsTool) Name() string               { return "list_available_actions" }
func (t *listActionsTool) Description() string         { return "List available remediation action types from DataStorage" }
func (t *listActionsTool) Parameters() json.RawMessage { return listActionsParams }

func (t *listActionsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	signal, _ := katypes.SignalContextFromContext(ctx)

	severity := signal.Severity
	if severity == "" {
		severity = "critical"
	}
	component := signal.ResourceKind
	if component == "" {
		component = "deployment"
	}
	environment := signal.Environment
	if environment == "" {
		environment = "production"
	}
	priority := signal.Priority
	if priority == "" {
		priority = "P0"
	}

	res, err := t.ds.ListAvailableActions(ctx, ogenclient.ListAvailableActionsParams{
		Severity:    ogenclient.ListAvailableActionsSeverity(severity),
		Component:   component,
		Environment: environment,
		Priority:    ogenclient.ListAvailableActionsPriority(priority),
	})
	if err != nil {
		return "", fmt.Errorf("listing action types: %w", err)
	}

	data, _ := json.Marshal(res)
	return string(data), nil
}

// --- list_workflows ---

type listWorkflowsTool struct{ ds *ogenclient.Client }

var listWorkflowsParams = json.RawMessage(`{"type":"object","properties":{"action_type":{"type":"string","description":"Action type from taxonomy (e.g., ScaleReplicas, RestartPod)"},"offset":{"type":"integer","description":"Pagination offset"},"limit":{"type":"integer","description":"Pagination limit"}},"required":["action_type"]}`)

func (t *listWorkflowsTool) Name() string               { return "list_workflows" }
func (t *listWorkflowsTool) Description() string         { return "Search for workflows by action type in DataStorage" }
func (t *listWorkflowsTool) Parameters() json.RawMessage { return listWorkflowsParams }

func (t *listWorkflowsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		ActionType string `json:"action_type"`
		Offset     *int   `json:"offset,omitempty"`
		Limit      *int   `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	signal, _ := katypes.SignalContextFromContext(ctx)

	severity := signal.Severity
	if severity == "" {
		severity = "critical"
	}
	component := signal.ResourceKind
	if component == "" {
		component = "deployment"
	}
	environment := signal.Environment
	if environment == "" {
		environment = "production"
	}
	priority := signal.Priority
	if priority == "" {
		priority = "P0"
	}

	params := ogenclient.ListWorkflowsByActionTypeParams{
		ActionType:  a.ActionType,
		Severity:    ogenclient.ListWorkflowsByActionTypeSeverity(severity),
		Component:   component,
		Environment: environment,
		Priority:    ogenclient.ListWorkflowsByActionTypePriority(priority),
	}
	if a.Offset != nil {
		params.Offset = ogenclient.NewOptInt(*a.Offset)
	}
	if a.Limit != nil {
		params.Limit = ogenclient.NewOptInt(*a.Limit)
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

var getWorkflowParams = json.RawMessage(`{"type":"object","properties":{"workflow_id":{"type":"string","description":"Workflow UUID"}},"required":["workflow_id"]}`)

func (t *getWorkflowTool) Name() string               { return "get_workflow" }
func (t *getWorkflowTool) Description() string         { return "Get a specific workflow definition by ID" }
func (t *getWorkflowTool) Parameters() json.RawMessage { return getWorkflowParams }

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

// --- get_resource_context ---

// --- get_namespaced_resource_context ---

type namespacedResourceContextTool struct {
	dsEnrich enrichment.DataStorageClient
	k8s      enrichment.K8sClient
}

var namespacedResourceContextParams = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","description":"Kubernetes resource kind"},"name":{"type":"string","description":"Resource name"},"namespace":{"type":"string","description":"Kubernetes namespace"}},"required":["kind","name","namespace"]}`)

func (t *namespacedResourceContextTool) Name() string               { return "get_namespaced_resource_context" }
func (t *namespacedResourceContextTool) Description() string         { return "Get namespaced resource context: root owner, remediation history, detected infrastructure, and quota details" }
func (t *namespacedResourceContextTool) Parameters() json.RawMessage { return namespacedResourceContextParams }

func (t *namespacedResourceContextTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	rootOwner := enrichment.OwnerChainEntry{Kind: a.Kind, Name: a.Name, Namespace: a.Namespace}

	chain, _ := t.k8s.GetOwnerChain(ctx, a.Kind, a.Name, a.Namespace)
	if len(chain) > 0 {
		rootOwner = chain[len(chain)-1]
	}

	history, _ := t.dsEnrich.GetRemediationHistory(ctx, rootOwner.Kind, rootOwner.Name, rootOwner.Namespace, "")

	type response struct {
		RootOwner          enrichment.OwnerChainEntry         `json:"root_owner"`
		RemediationHistory []enrichment.RemediationHistoryEntry `json:"remediation_history"`
	}

	result := response{
		RootOwner:          rootOwner,
		RemediationHistory: history,
	}
	if result.RemediationHistory == nil {
		result.RemediationHistory = []enrichment.RemediationHistoryEntry{}
	}

	data, _ := json.Marshal(result)
	return string(data), nil
}

// --- get_cluster_resource_context ---

type clusterResourceContextTool struct {
	dsEnrich enrichment.DataStorageClient
}

var clusterResourceContextParams = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","description":"Kubernetes resource kind"},"name":{"type":"string","description":"Resource name"}},"required":["kind","name"]}`)

func (t *clusterResourceContextTool) Name() string               { return "get_cluster_resource_context" }
func (t *clusterResourceContextTool) Description() string         { return "Get cluster-scoped resource context: root owner and remediation history" }
func (t *clusterResourceContextTool) Parameters() json.RawMessage { return clusterResourceContextParams }

func (t *clusterResourceContextTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	rootOwner := struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}{Kind: a.Kind, Name: a.Name}

	history, _ := t.dsEnrich.GetRemediationHistory(ctx, a.Kind, a.Name, "", "")

	type response struct {
		RootOwner struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"root_owner"`
		RemediationHistory []enrichment.RemediationHistoryEntry `json:"remediation_history"`
	}

	result := response{
		RootOwner:          rootOwner,
		RemediationHistory: history,
	}
	if result.RemediationHistory == nil {
		result.RemediationHistory = []enrichment.RemediationHistoryEntry{}
	}

	data, _ := json.Marshal(result)
	return string(data), nil
}
