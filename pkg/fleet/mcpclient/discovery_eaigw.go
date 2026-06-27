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
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Compile-time interface compliance.
var _ GatewayDiscoverer = (*EAIGWDiscoverer)(nil)

// EAIGWDiscoverer implements GatewayDiscoverer for Envoy AI Gateway (EAIGW).
// It scans the full tools/list response and extracts cluster IDs from the
// "{clusterID}__{toolName}" naming convention.
//
// Authority: ADR-068 decision #11
type EAIGWDiscoverer struct {
	session *mcp.ClientSession
}

// ListClusters scans tools/list and extracts unique cluster IDs from tool names
// using the EAIGW "__" separator convention. Tools without the separator
// (e.g., meta-tools) are ignored. The category parameter is unused for EAIGW.
func (d *EAIGWDiscoverer) ListClusters(ctx context.Context, _ string) ([]ClusterInfo, error) {
	result, err := d.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools from EAIGW gateway: %w", err)
	}

	seen := make(map[string]bool)
	var clusters []ClusterInfo
	for _, tool := range result.Tools {
		clusterID := parseClusterIDFromToolName(tool.Name)
		if clusterID == "" || seen[clusterID] {
			continue
		}
		seen[clusterID] = true
		clusters = append(clusters, ClusterInfo{
			Name:   clusterID,
			Prefix: clusterID + "__",
		})
	}
	return clusters, nil
}

// ToolsForCluster filters tools/list to return only tools matching the given
// cluster's "{clusterID}__" prefix, with full schemas.
func (d *EAIGWDiscoverer) ToolsForCluster(ctx context.Context, clusterID string) ([]ToolDefinition, error) {
	result, err := d.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools from EAIGW gateway for cluster %q: %w", clusterID, err)
	}

	prefix := clusterID + "__"
	var defs []ToolDefinition
	for _, tool := range result.Tools {
		if !strings.HasPrefix(tool.Name, prefix) {
			continue
		}
		defs = append(defs, toolDefinitionFromMCP(tool))
	}

	if len(defs) == 0 {
		return nil, fmt.Errorf("no tools found for cluster %q in EAIGW gateway", clusterID)
	}
	return defs, nil
}
