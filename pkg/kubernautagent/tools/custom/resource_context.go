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
	"log/slog"

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

func computeSpecHash(ctx context.Context, k8s enrichment.K8sClient, kind, name, namespace, toolName string) string {
	computed, err := k8s.GetSpecHash(ctx, kind, name, namespace)
	if err != nil {
		slog.Warn(toolName+": specHash computation failed, proceeding with empty",
			slog.String("kind", kind), slog.String("name", name),
			slog.String("namespace", namespace), slog.String("error", err.Error()))
		return ""
	}
	return computed
}

func fetchRemediationHistory(ctx context.Context, ds enrichment.DataStorageClient, kind, name, namespace, specHash, toolName string) *enrichment.RemediationHistoryResult {
	result, err := ds.GetRemediationHistory(ctx, kind, name, namespace, specHash)
	if err != nil {
		slog.Warn(toolName+": remediation history fetch failed",
			slog.String("kind", kind), slog.String("name", name),
			slog.String("namespace", namespace), slog.String("error", err.Error()))
	}
	if result == nil {
		result = &enrichment.RemediationHistoryResult{}
	}
	return result
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

	chain, chainErr := t.k8s.GetOwnerChain(ctx, params.Kind, params.Name, params.Namespace)
	if chainErr != nil {
		slog.Warn("get_namespaced_resource_context: owner chain resolution failed",
			slog.String("kind", params.Kind), slog.String("name", params.Name),
			slog.String("namespace", params.Namespace), slog.String("error", chainErr.Error()))
	}

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

	specHash := computeSpecHash(ctx, t.k8s, rootOwner.Kind, rootOwner.Name, rootOwner.Namespace, "get_namespaced_resource_context")
	histResult := fetchRemediationHistory(ctx, t.ds, rootOwner.Kind, rootOwner.Name, rootOwner.Namespace, specHash, "get_namespaced_resource_context")

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
	ds  enrichment.DataStorageClient
	k8s enrichment.K8sClient
}

// NewClusterResourceContextTool creates a get_cluster_resource_context tool.
func NewClusterResourceContextTool(ds enrichment.DataStorageClient, k8s enrichment.K8sClient) tools.Tool {
	return &clusterResourceContextTool{ds: ds, k8s: k8s}
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

	specHash := computeSpecHash(ctx, t.k8s, params.Kind, params.Name, "", "get_cluster_resource_context")
	histResult := fetchRemediationHistory(ctx, t.ds, params.Kind, params.Name, "", specHash, "get_cluster_resource_context")

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
