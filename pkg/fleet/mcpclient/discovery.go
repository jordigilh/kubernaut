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

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// GatewayDiscoverer abstracts two-phase tool discovery for different MCP Gateway
// implementations. KA uses this to pre-scope the LLM's tool context to the
// alert's target cluster, then optionally expand to other clusters on demand.
//
// Authority: ADR-068 decision #11, BR-FLEET-054
type GatewayDiscoverer interface {
	// ListClusters returns metadata for all clusters visible through the gateway.
	// The result does not include tool schemas to minimize LLM context usage.
	// An optional category filter narrows results (gateway-dependent semantics).
	ListClusters(ctx context.Context, category string) ([]ClusterInfo, error)

	// ToolsForCluster returns the full tool schemas for a specific cluster.
	// For Kuadrant, this calls select_tools then ListTools to scope the session.
	// For EAIGW, this filters the full tools/list by the cluster's prefix.
	ToolsForCluster(ctx context.Context, clusterID string) ([]ToolDefinition, error)
}

// ClusterInfo holds metadata about a cluster discovered through the gateway.
// Tool names are included for select_tools but full schemas are omitted
// to keep the LLM context lean.
type ClusterInfo struct {
	Name       string   `json:"name"`
	Categories []string `json:"categories,omitempty"`
	Hint       string   `json:"hint,omitempty"`
	Prefix     string   `json:"prefix,omitempty"`
	Tools      []string `json:"tools,omitempty"`
}

// NewDiscoverer creates a GatewayDiscoverer for the given gateway type and session.
// Returns an error for unsupported or empty gateway types, or if session is nil.
//
// Authority: ADR-068 decision #11 (CM-6: Configuration Settings)
func NewDiscoverer(gatewayType registry.MCPGatewayType, session *mcp.ClientSession) (GatewayDiscoverer, error) {
	if session == nil {
		return nil, fmt.Errorf("MCP session must not be nil for gateway discovery (gatewayType=%q)", gatewayType)
	}
	switch gatewayType {
	case registry.GatewayKuadrant:
		return &KuadrantDiscoverer{session: session}, nil
	case registry.GatewayEAIGW:
		return &EAIGWDiscoverer{session: session}, nil
	default:
		return nil, fmt.Errorf("unsupported gateway type %q for tool discovery; must be one of: eaigw, kuadrant", gatewayType)
	}
}
