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
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// MetaToolDiscoverTools is the Kuadrant gateway meta-tool for listing available servers.
	MetaToolDiscoverTools = "discover_tools"
	// MetaToolSelectTools is the Kuadrant gateway meta-tool for scoping a session to a server.
	MetaToolSelectTools = "select_tools"
)

// Compile-time interface compliance.
var _ GatewayDiscoverer = (*KuadrantDiscoverer)(nil)

// KuadrantDiscoverer implements GatewayDiscoverer for Kuadrant MCP Gateway.
// It wraps the discover_tools and select_tools meta-tools to provide
// progressive tool discovery with server-side session scoping.
//
// Authority: ADR-068 decision #11, Spike S15
type KuadrantDiscoverer struct {
	session *mcp.ClientSession
}

// ListClusters calls discover_tools on the Kuadrant gateway and returns cluster
// metadata without tool schemas. An optional category filter narrows results.
func (d *KuadrantDiscoverer) ListClusters(ctx context.Context, category string) ([]ClusterInfo, error) {
	args := map[string]any{}
	if category != "" {
		args["category"] = category
	}

	result, err := d.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      MetaToolDiscoverTools,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("call discover_tools: %w", err)
	}
	if result.IsError {
		return nil, fmt.Errorf("discover_tools returned error: %s", ExtractText(result))
	}

	text := ExtractText(result)
	var resp struct {
		Servers []struct {
			Name       string   `json:"name"`
			Categories []string `json:"categories"`
			Hint       string   `json:"hint"`
		} `json:"servers"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return nil, fmt.Errorf("parse discover_tools response: %w", err)
	}

	clusters := make([]ClusterInfo, 0, len(resp.Servers))
	for _, s := range resp.Servers {
		clusters = append(clusters, ClusterInfo{
			Name:       s.Name,
			Categories: s.Categories,
			Hint:       s.Hint,
		})
	}
	return clusters, nil
}

// ToolsForCluster calls select_tools to scope the session to the given cluster,
// then calls ListTools to retrieve the scoped tool schemas.
func (d *KuadrantDiscoverer) ToolsForCluster(ctx context.Context, clusterID string) ([]ToolDefinition, error) {
	selectResult, err := d.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      MetaToolSelectTools,
		Arguments: map[string]any{"server_name": clusterID},
	})
	if err != nil {
		return nil, fmt.Errorf("call select_tools for cluster %q: %w", clusterID, err)
	}
	if selectResult.IsError {
		return nil, fmt.Errorf("select_tools for cluster %q returned error: %s", clusterID, ExtractText(selectResult))
	}

	var selectResp struct {
		Prefix string `json:"prefix"`
	}
	selectText := ExtractText(selectResult)
	if err := json.Unmarshal([]byte(selectText), &selectResp); err != nil {
		return nil, fmt.Errorf("parse select_tools response for cluster %q: %w", clusterID, err)
	}

	toolsResult, err := d.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools after select_tools for cluster %q: %w", clusterID, err)
	}

	defs := make([]ToolDefinition, 0, len(toolsResult.Tools))
	for _, t := range toolsResult.Tools {
		if t.Name == MetaToolDiscoverTools || t.Name == MetaToolSelectTools {
			continue
		}
		if selectResp.Prefix != "" && !strings.HasPrefix(t.Name, selectResp.Prefix) {
			continue
		}
		defs = append(defs, toolDefinitionFromMCP(t))
	}
	return defs, nil
}
