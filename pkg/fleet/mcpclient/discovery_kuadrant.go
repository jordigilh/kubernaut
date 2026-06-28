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
// metadata including tool names. An optional category filter narrows results.
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
			Tools      []string `json:"tools"`
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
			Tools:      s.Tools,
		})
	}
	return clusters, nil
}

// ToolsForCluster discovers tool names via discover_tools, then calls
// select_tools with {"tools": [...]} to scope the session, and finally
// calls ListTools to retrieve the scoped tool schemas.
//
// The Kuadrant MCP Gateway requires the tools array (not server_name)
// as the select_tools parameter. See Kuadrant API docs.
func (d *KuadrantDiscoverer) ToolsForCluster(ctx context.Context, clusterID string) ([]ToolDefinition, error) {
	clusters, err := d.ListClusters(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("discover tools for cluster %q: %w", clusterID, err)
	}

	var toolNames []string
	for _, c := range clusters {
		if c.Name == clusterID {
			toolNames = c.Tools
			break
		}
	}
	if len(toolNames) == 0 {
		return nil, fmt.Errorf("cluster %q not found in discover_tools response or has no tools", clusterID)
	}

	selectResult, err := d.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      MetaToolSelectTools,
		Arguments: map[string]any{"tools": toolNames},
	})
	if err != nil {
		return nil, fmt.Errorf("call select_tools for cluster %q: %w", clusterID, err)
	}
	if selectResult.IsError {
		return nil, fmt.Errorf("select_tools for cluster %q returned error: %s", clusterID, ExtractText(selectResult))
	}

	toolsResult, err := d.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools after select_tools for cluster %q: %w", clusterID, err)
	}

	selectedSet := make(map[string]bool, len(toolNames))
	for _, name := range toolNames {
		selectedSet[name] = true
	}

	defs := make([]ToolDefinition, 0, len(toolsResult.Tools))
	for _, t := range toolsResult.Tools {
		if t.Name == MetaToolDiscoverTools || t.Name == MetaToolSelectTools {
			continue
		}
		if selectedSet[t.Name] {
			defs = append(defs, toolDefinitionFromMCP(t))
		}
	}
	return defs, nil
}
