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

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
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
	RootOwner          rootOwnerResponse                    `json:"root_owner"`
	RemediationHistory *enrichment.RemediationHistoryResult `json:"remediation_history"`
}

type clusterResponse struct {
	RootOwner          rootOwnerResponse                    `json:"root_owner"`
	RemediationHistory *enrichment.RemediationHistoryResult `json:"remediation_history"`
}

// resolveK8sClient picks the effective enrichment.K8sClient for one Execute
// call (issue #2306): the hub-bound client for a hub-local investigation (no
// fleet overlay in ctx) — zero regression, byte-identical to before this fix
// — a new overlay-backed K8sClient wrapping the fleet overlay's resources_get
// tool for a fleet-target investigation whose overlay publishes it, or a
// clear-error no-op K8sClient for a fleet-target investigation whose overlay
// lacks resources_get (e.g. a toolset misconfiguration). The no-op case
// deliberately does NOT fall back to hub, which would silently resolve
// owner-chain/spec-hash against the wrong cluster — the exact AC-6 defect
// this issue closes for the k8s-tool suppression half.
func resolveK8sClient(ctx context.Context, hub enrichment.K8sClient, logger logr.Logger) enrichment.K8sClient {
	overlay, hasOverlay := investigator.FleetOverlayFromContext(ctx)
	if !hasOverlay {
		return hub
	}
	if getTool, ok := overlay[mcpclient.ToolGet]; ok {
		return NewOverlayK8sClient(getTool, logger)
	}
	clusterID, _ := audit.ClusterIDFromContext(ctx)
	return noopK8sClient{clusterID: clusterID}
}

func computeSpecHash(ctx context.Context, logger logr.Logger, k8s enrichment.K8sClient, kind, name, namespace, toolName string) string {
	computed, err := k8s.GetSpecHash(ctx, kind, name, namespace, "")
	if err != nil {
		logger.Info(toolName+": specHash computation failed, proceeding with empty",
			"kind", kind, "name", name, "namespace", namespace, "error", err)
		return ""
	}
	return computed
}

// remediationHistoryQuery groups the lookup key and logging context for
// fetchRemediationHistory. Extracted per AGENTS.md's 8+-param Options-pattern
// rule.
type remediationHistoryQuery struct {
	Kind      string
	Name      string
	Namespace string
	ClusterID string
	SpecHash  string
	ToolName  string
}

func fetchRemediationHistory(ctx context.Context, logger logr.Logger, ds enrichment.DataStorageClient, q remediationHistoryQuery) *enrichment.RemediationHistoryResult {
	result, err := ds.GetRemediationHistory(ctx, q.Kind, q.Name, q.Namespace, q.ClusterID, q.SpecHash)
	if err != nil {
		logger.Info(q.ToolName+": remediation history fetch failed",
			"kind", q.Kind, "name", q.Name, "namespace", q.Namespace, "error", err)
	}
	if result == nil {
		result = &enrichment.RemediationHistoryResult{}
	}
	return result
}

// --- get_namespaced_resource_context ---

type namespacedResourceContextTool struct {
	ds     enrichment.DataStorageClient
	k8s    enrichment.K8sClient
	logger logr.Logger
}

// NewNamespacedResourceContextTool creates a get_namespaced_resource_context tool.
func NewNamespacedResourceContextTool(ds enrichment.DataStorageClient, k8s enrichment.K8sClient, logger logr.Logger) tools.Tool {
	return &namespacedResourceContextTool{ds: ds, k8s: k8s, logger: logger.WithName("get_namespaced_resource_context")}
}

func (t *namespacedResourceContextTool) Name() string { return "get_namespaced_resource_context" }
func (t *namespacedResourceContextTool) Description() string {
	return "Get resource context including owner chain root, remediation history, and detected infrastructure for a namespaced resource"
}
func (t *namespacedResourceContextTool) Parameters() json.RawMessage {
	return namespacedResourceContextSchema
}

func (t *namespacedResourceContextTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	k8sClient := resolveK8sClient(ctx, t.k8s, t.logger)

	chain, chainErr := k8sClient.GetOwnerChain(ctx, params.Kind, params.Name, params.Namespace, "")
	if chainErr != nil {
		t.logger.Info("get_namespaced_resource_context: owner chain resolution failed",
			"kind", params.Kind, "name", params.Name, "namespace", params.Namespace, "error", chainErr)
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

	specHash := computeSpecHash(ctx, t.logger, k8sClient, rootOwner.Kind, rootOwner.Name, rootOwner.Namespace, "get_namespaced_resource_context")
	clusterID, _ := audit.ClusterIDFromContext(ctx)
	histResult := fetchRemediationHistory(ctx, t.logger, t.ds, remediationHistoryQuery{
		Kind: rootOwner.Kind, Name: rootOwner.Name, Namespace: rootOwner.Namespace,
		ClusterID: clusterID, SpecHash: specHash, ToolName: "get_namespaced_resource_context",
	})

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
	ds     enrichment.DataStorageClient
	k8s    enrichment.K8sClient
	logger logr.Logger
}

// NewClusterResourceContextTool creates a get_cluster_resource_context tool.
func NewClusterResourceContextTool(ds enrichment.DataStorageClient, k8s enrichment.K8sClient, logger logr.Logger) tools.Tool {
	return &clusterResourceContextTool{ds: ds, k8s: k8s, logger: logger.WithName("get_cluster_resource_context")}
}

func (t *clusterResourceContextTool) Name() string { return "get_cluster_resource_context" }
func (t *clusterResourceContextTool) Description() string {
	return "Get resource context including remediation history for a cluster-scoped resource"
}
func (t *clusterResourceContextTool) Parameters() json.RawMessage {
	return clusterResourceContextSchema
}

func (t *clusterResourceContextTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	k8sClient := resolveK8sClient(ctx, t.k8s, t.logger)

	specHash := computeSpecHash(ctx, t.logger, k8sClient, params.Kind, params.Name, "", "get_cluster_resource_context")
	clusterID, _ := audit.ClusterIDFromContext(ctx)
	histResult := fetchRemediationHistory(ctx, t.logger, t.ds, remediationHistoryQuery{
		Kind: params.Kind, Name: params.Name, ClusterID: clusterID, SpecHash: specHash,
		ToolName: "get_cluster_resource_context",
	})

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
