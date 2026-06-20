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

// Package mcpclient provides a shared MCP resource client for accessing
// Kubernetes resources on remote clusters via the MCP Gateway.
// All services that need remote cluster access import this package.
//
// Routing pattern: only used when ClusterID is non-empty (remote cluster).
// Local cluster operations continue using existing direct K8s API paths.
package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPResourceClient provides access to Kubernetes resources on remote clusters
// via the MCP Gateway. It wraps an MCP client session and uses the gateway's
// tool naming convention: {clusterID}__get_resource, {clusterID}__list_resources.
type MCPResourceClient struct {
	session *mcp.ClientSession
	mu      sync.Mutex
	closed  bool
}

// New creates an MCPResourceClient connected to the given MCP Gateway endpoint.
// The connection is established immediately; returns error if unreachable.
func New(ctx context.Context, endpoint string, opts ...Option) (*MCPResourceClient, error) {
	cfg := &clientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "kubernaut-fleet-client", Version: "v0.1.0"},
		nil,
	)

	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: cfg.httpClient,
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to MCP Gateway at %s: %w", endpoint, err)
	}

	return &MCPResourceClient{session: session}, nil
}

// Get retrieves a single Kubernetes resource from a remote cluster via MCP Gateway.
// Calls the tool "{clusterID}__get_resource" with kind, name, and namespace arguments.
func (c *MCPResourceClient) Get(ctx context.Context, clusterID, kind, name, namespace string) (string, error) {
	toolName := clusterID + "__get_resource"
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name: toolName,
		Arguments: map[string]any{
			"kind":      kind,
			"name":      name,
			"namespace": namespace,
		},
	})
	if err != nil {
		return "", fmt.Errorf("call %s: %w", toolName, err)
	}

	return extractTextContent(result), nil
}

// List retrieves Kubernetes resources of a given kind from a remote cluster via MCP Gateway.
// Calls the tool "{clusterID}__list_resources" with kind and namespace arguments.
func (c *MCPResourceClient) List(ctx context.Context, clusterID, kind, namespace string) (string, error) {
	toolName := clusterID + "__list_resources"
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name: toolName,
		Arguments: map[string]any{
			"kind":      kind,
			"namespace": namespace,
		},
	})
	if err != nil {
		return "", fmt.Errorf("call %s: %w", toolName, err)
	}

	return extractTextContent(result), nil
}

// Close terminates the MCP session. Safe to call multiple times.
func (c *MCPResourceClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.session.Close()
}

// Session returns the underlying MCP client session for direct tool calls.
// Used by BridgeTool to call discovered tools without creating new sessions.
func (c *MCPResourceClient) Session() *mcp.ClientSession {
	return c.session
}

func extractTextContent(result *mcp.CallToolResult) string {
	return ExtractText(result)
}

// ExtractText extracts and concatenates all text content from an MCP tool result.
// Returns text parts joined with newlines, or a JSON-serialized fallback if no text parts are found.
func ExtractText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}

	var texts []string
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			texts = append(texts, tc.Text)
		}
	}
	if len(texts) > 0 {
		return strings.Join(texts, "\n")
	}

	data, err := json.Marshal(result.Content)
	if err != nil {
		return ""
	}
	return string(data)
}
