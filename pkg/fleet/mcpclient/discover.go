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
// The matching itself is gateway-agnostic and delegates to PrefixFromToolNames;
// see that function's doc comment for the extraction algorithm.
//
// Authority: ADR-068 decision #10 (gateway-agnostic business logic), Issue #54
func DiscoverToolPrefix(ctx context.Context, session *mcp.ClientSession, clusterID string) (string, error) {
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("list tools from MCP Gateway: %w", err)
	}

	names := make([]string, len(result.Tools))
	for i, tool := range result.Tools {
		names[i] = tool.Name
	}

	prefix, err := PrefixFromToolNames(clusterID, names)
	if err != nil {
		return "", fmt.Errorf("in MCP Gateway tools/list response: %w", err)
	}
	return prefix, nil
}

// PrefixFromToolNames finds the gateway-specific wire prefix for a given
// cluster by matching the cluster ID against an already-discovered list of
// tool names.
//
// The matching is gateway-agnostic: it normalizes both the cluster ID and each
// tool name (replacing hyphens with underscores) and checks whether the name
// starts with the normalized cluster ID and ends with a known base tool name.
// The prefix returned is the original (un-normalized) substring before the
// base tool name, preserving whatever separator the gateway uses -- this
// covers both EAIGW's "{clusterID}__" convention and Kuadrant's admin-set
// MCPServerRegistration.spec.prefix, which is not required to follow the
// "{clusterID}__" shape at all (Issue #1756).
//
// Operating on a plain name slice (rather than a live *mcp.ClientSession, as
// DiscoverToolPrefix does) lets callers that already hold discovered tool
// definitions -- e.g. cmd/kubernautagent's gatewayOverlayResolver.Overlay(),
// which gets them from GatewayDiscoverer.ToolsForCluster() -- reuse this exact
// extraction logic without an extra ListTools() round trip and without
// duplicating (and silently drifting from) it, which is exactly how #1756
// went undetected.
//
// Authority: ADR-068 decision #10 (gateway-agnostic business logic),
// DD-FLEET-005, BR-INTEGRATION-054, BR-INTEGRATION-1489, Issue #1756.
func PrefixFromToolNames(clusterID string, names []string) (string, error) {
	normalized := strings.ReplaceAll(clusterID, "-", "_")

	for _, name := range names {
		normalizedName := strings.ReplaceAll(name, "-", "_")
		if !strings.HasPrefix(normalizedName, normalized) {
			continue
		}
		for _, suffix := range knownToolSuffixes {
			if strings.HasSuffix(normalizedName, suffix) {
				prefix := name[:len(name)-len(suffix)]
				return prefix, nil
			}
		}
	}

	return "", fmt.Errorf("no tools found for cluster %q among %d discovered tool names", clusterID, len(names))
}

// knownToolSuffixes are the base tool names from kubernetes-mcp-server that
// PrefixFromToolNames (and, transitively, DiscoverToolPrefix) uses as anchors
// to extract the cluster prefix from gateway-prefixed tool names.
var knownToolSuffixes = []string{
	ToolGet,
	ToolList,
	ToolCreateOrUpdate,
	ToolDelete,
}
