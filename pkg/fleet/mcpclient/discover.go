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

// DiscoverToolPrefix queries the MCP Gateway's tools/list endpoint and finds
// the gateway-specific prefix for a given cluster by matching the cluster ID
// against discovered tool names.
//
// The matching is gateway-agnostic: it normalizes both the cluster ID and tool
// names (replacing hyphens with underscores) and checks whether any tool starts
// with the normalized cluster ID and ends with a known base tool name. The
// prefix returned is the original (un-normalized) substring before the base
// tool name, preserving whatever separator the gateway uses.
//
// Authority: ADR-068 decision #10 (gateway-agnostic business logic), Issue #54
func DiscoverToolPrefix(ctx context.Context, session *mcp.ClientSession, clusterID string) (string, error) {
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("list tools from MCP Gateway: %w", err)
	}

	normalized := strings.ReplaceAll(clusterID, "-", "_")

	for _, tool := range result.Tools {
		name := strings.ReplaceAll(tool.Name, "-", "_")
		if !strings.HasPrefix(name, normalized) {
			continue
		}
		for _, suffix := range knownToolSuffixes {
			if strings.HasSuffix(name, suffix) {
				prefix := tool.Name[:len(tool.Name)-len(suffix)]
				return prefix, nil
			}
		}
	}

	return "", fmt.Errorf("no tools found for cluster %q in MCP Gateway tools/list response", clusterID)
}

// knownToolSuffixes are the base tool names from kubernetes-mcp-server that
// DiscoverToolPrefix uses as anchors to extract the cluster prefix from
// gateway-prefixed tool names.
var knownToolSuffixes = []string{
	ToolGet,
	ToolList,
	ToolCreateOrUpdate,
	ToolDelete,
}
