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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

var namespacedResourceContextSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"kind":      {"type": "string", "description": "Kubernetes resource kind"},
		"name":      {"type": "string", "description": "Resource name"},
		"namespace": {"type": "string", "description": "Kubernetes namespace"}
	},
	"required": ["kind", "name", "namespace"]
}`)

var clusterResourceContextSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"kind": {"type": "string", "description": "Kubernetes resource kind"},
		"name": {"type": "string", "description": "Resource name"}
	},
	"required": ["kind", "name"]
}`)

type rootOwnerResponse struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type namespacedResponse struct {
	RootOwner          rootOwnerResponse              `json:"root_owner"`
	RemediationHistory *enrichment.RemediationHistoryResult `json:"remediation_history"`
}

type clusterResponse struct {
	RootOwner          rootOwnerResponse              `json:"root_owner"`
	RemediationHistory *enrichment.RemediationHistoryResult `json:"remediation_history"`
}

// --- get_namespaced_resource_context ---

type namespacedResourceContextTool struct {
	ds  enrichment.DataStorageClient
	k8s enrichment.K8sClient
}

// NewNamespacedResourceContextTool creates a get_namespaced_resource_context tool.
func NewNamespacedResourceContextTool(ds enrichment.DataStorageClient, k8s enrichment.K8sClient) tools.Tool {
	return &namespacedResourceContextTool{ds: ds, k8s: k8s}
}

func (t *namespacedResourceContextTool) Name() string               { return "get_namespaced_resource_context" }
func (t *namespacedResourceContextTool) Description() string         { return "Get resource context including owner chain root, remediation history, and detected infrastructure for a namespaced resource" }
func (t *namespacedResourceContextTool) Parameters() json.RawMessage { return namespacedResourceContextSchema }

func (t *namespacedResourceContextTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	chain, _ := t.k8s.GetOwnerChain(ctx, params.Kind, params.Name, params.Namespace)

	rootOwner := rootOwnerResponse{
		Kind:      params.Kind,
		Name:      params.Name,
		Namespace: params.Namespace,
	}
	if len(chain) > 0 {
		last := chain[len(chain)-1]
		rootOwner = rootOwnerResponse{
			Kind:      last.Kind,
			Name:      last.Name,
			Namespace: last.Namespace,
		}
	}

	histResult, _ := t.ds.GetRemediationHistory(ctx, rootOwner.Kind, rootOwner.Name, rootOwner.Namespace, "")
	if histResult == nil {
		histResult = &enrichment.RemediationHistoryResult{}
	}

	resp := namespacedResponse{
		RootOwner:          rootOwner,
		RemediationHistory: histResult,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return "", fmt.Errorf("marshaling response: %w", err)
	}
	return string(data), nil
}

// --- get_cluster_resource_context ---

type clusterResourceContextTool struct {
	ds enrichment.DataStorageClient
}

// NewClusterResourceContextTool creates a get_cluster_resource_context tool.
func NewClusterResourceContextTool(ds enrichment.DataStorageClient) tools.Tool {
	return &clusterResourceContextTool{ds: ds}
}

func (t *clusterResourceContextTool) Name() string               { return "get_cluster_resource_context" }
func (t *clusterResourceContextTool) Description() string         { return "Get resource context including remediation history for a cluster-scoped resource" }
func (t *clusterResourceContextTool) Parameters() json.RawMessage { return clusterResourceContextSchema }

func (t *clusterResourceContextTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	histResult, _ := t.ds.GetRemediationHistory(ctx, params.Kind, params.Name, "", "")
	if histResult == nil {
		histResult = &enrichment.RemediationHistoryResult{}
	}

	resp := clusterResponse{
		RootOwner: rootOwnerResponse{
			Kind: params.Kind,
			Name: params.Name,
		},
		RemediationHistory: histResult,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return "", fmt.Errorf("marshaling response: %w", err)
	}
	return string(data), nil
}
