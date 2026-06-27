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

package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/sync/singleflight"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// Compile-time interface compliance.
var _ tools.Tool = (*ListClustersTool)(nil)
var _ tools.Tool = (*ListToolsForClusterTool)(nil)

// ListClustersTool is a KA tool that returns cluster metadata from the gateway
// without tool schemas, keeping the LLM context lean.
//
// Authority: ADR-068 decision #11 (SC-7: Boundary Protection)
type ListClustersTool struct {
	discoverer GatewayDiscoverer
}

// NewListClustersTool creates a ListClustersTool backed by the given discoverer.
func NewListClustersTool(discoverer GatewayDiscoverer) *ListClustersTool {
	return &ListClustersTool{discoverer: discoverer}
}

func (t *ListClustersTool) Name() string        { return "list_clusters" }
func (t *ListClustersTool) Description() string  { return "List available remote clusters and their categories without loading tool schemas" }
func (t *ListClustersTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"category":{"type":"string","description":"Optional category filter to narrow results"}},"additionalProperties":false}`)
}

func (t *ListClustersTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Category string `json:"category"`
	}
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("parse list_clusters args: %w", err)
		}
	}

	clusters, err := t.discoverer.ListClusters(ctx, params.Category)
	if err != nil {
		return "", fmt.Errorf("list_clusters: %w", err)
	}

	type clusterOut struct {
		ID         string   `json:"id"`
		Hint       string   `json:"hint,omitempty"`
		Categories []string `json:"categories,omitempty"`
	}
	out := make([]clusterOut, len(clusters))
	for i, c := range clusters {
		out[i] = clusterOut{
			ID:         c.Name,
			Hint:       c.Hint,
			Categories: c.Categories,
		}
	}
	resp, _ := json.Marshal(map[string]any{"clusters": out})
	return string(resp), nil
}

// ListToolsForClusterTool is a KA tool that discovers and registers tools for
// a specific remote cluster. It uses singleflight to deduplicate concurrent
// discovery calls for the same cluster.
//
// Authority: ADR-068 decision #11 (SC-5: Denial of Service Protection, SC-7: Boundary Protection)
type ListToolsForClusterTool struct {
	discoverer GatewayDiscoverer
	registry   *registry.Registry
	session    Session
	sf         singleflight.Group
}

// NewListToolsForClusterTool creates a ListToolsForClusterTool.
func NewListToolsForClusterTool(discoverer GatewayDiscoverer, reg *registry.Registry, session Session) *ListToolsForClusterTool {
	return &ListToolsForClusterTool{
		discoverer: discoverer,
		registry:   reg,
		session:    session,
	}
}

func (t *ListToolsForClusterTool) Name() string        { return "list_tools_for_cluster" }
func (t *ListToolsForClusterTool) Description() string  { return "Discover and activate tools for a specific remote cluster" }
func (t *ListToolsForClusterTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"cluster_id":{"type":"string","description":"Cluster ID from list_clusters"}},"required":["cluster_id"],"additionalProperties":false}`)
}

func (t *ListToolsForClusterTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ClusterID string `json:"cluster_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse list_tools_for_cluster args: %w", err)
	}
	if params.ClusterID == "" {
		return "", fmt.Errorf("list_tools_for_cluster: cluster_id is required")
	}

	result, err, _ := t.sf.Do(params.ClusterID, func() (any, error) {
		return t.discoverer.ToolsForCluster(ctx, params.ClusterID)
	})
	if err != nil {
		return "", fmt.Errorf("list_tools_for_cluster for %q: %w", params.ClusterID, err)
	}

	defs := result.([]ToolDefinition)

	for _, def := range defs {
		clusterID := parseClusterIDFromToolName(def.Name)
		bt := NewBridgeTool(def, clusterID, t.session)
		t.registry.Register(bt)
	}

	type toolOut struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	out := make([]toolOut, len(defs))
	for i, d := range defs {
		out[i] = toolOut{Name: d.Name, Description: d.Description}
	}
	resp, _ := json.Marshal(map[string]any{"tools": out, "cluster_id": params.ClusterID})
	return string(resp), nil
}
