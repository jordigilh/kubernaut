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

// Package mcpclient provides a shared MCP resource client for querying
// Kubernetes resources on remote clusters via the MCP Gateway.
// Used by GW, SP, RO, EM, KA, and WE for federated resource access.
//
// Authority: Issue #54 (Multi-cluster federation), WS7 (GW/SP/RO/EM Federation)
package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// MCPResourceClient provides access to Kubernetes resources on remote clusters
// via the MCP Gateway. It translates Get/List operations into MCP tool calls
// (resources_get, resources_list) with cluster prefix routing.
type MCPResourceClient interface {
	Get(ctx context.Context, clusterPrefix, kind, namespace, name string) (*unstructured.Unstructured, error)
	List(ctx context.Context, clusterPrefix, kind, namespace string, labels map[string]string) ([]unstructured.Unstructured, error)
	GetLabels(ctx context.Context, clusterPrefix, kind, namespace, name string) (map[string]string, error)
}

// Client implements MCPResourceClient using the MCP Go SDK's StreamableClientTransport.
type Client struct {
	endpoint   string
	httpClient *http.Client
	mcpClient  *mcp.Client
	logger     logr.Logger
}

// NewClient creates an MCPResourceClient that connects to the MCP Gateway.
func NewClient(endpoint string, httpClient *http.Client, logger logr.Logger) *Client {
	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "kubernaut-fleet-client",
		Version: "1.5.0",
	}, nil)

	return &Client{
		endpoint:   endpoint,
		httpClient: httpClient,
		mcpClient:  mcpClient,
		logger:     logger.WithName("mcp-resource-client"),
	}
}

// Get fetches a single Kubernetes resource from a remote cluster via MCP.
func (c *Client) Get(ctx context.Context, clusterPrefix, kind, namespace, name string) (*unstructured.Unstructured, error) {
	toolName := clusterPrefix + "resources_get"
	args := map[string]any{
		"kind": kind,
		"name": name,
	}
	if namespace != "" {
		args["namespace"] = namespace
	}

	result, err := c.callTool(ctx, toolName, args)
	if err != nil {
		return nil, fmt.Errorf("MCP Get %s/%s in %s (cluster %s): %w", kind, name, namespace, clusterPrefix, err)
	}

	obj, err := parseUnstructured(result)
	if err != nil {
		return nil, fmt.Errorf("parsing MCP response for %s/%s: %w", kind, name, err)
	}
	return obj, nil
}

// List fetches multiple Kubernetes resources from a remote cluster via MCP.
func (c *Client) List(ctx context.Context, clusterPrefix, kind, namespace string, labels map[string]string) ([]unstructured.Unstructured, error) {
	toolName := clusterPrefix + "resources_list"
	args := map[string]any{
		"kind": kind,
	}
	if namespace != "" {
		args["namespace"] = namespace
	}
	if len(labels) > 0 {
		labelSelector := ""
		for k, v := range labels {
			if labelSelector != "" {
				labelSelector += ","
			}
			labelSelector += k + "=" + v
		}
		args["labelSelector"] = labelSelector
	}

	result, err := c.callTool(ctx, toolName, args)
	if err != nil {
		return nil, fmt.Errorf("MCP List %s in %s (cluster %s): %w", kind, namespace, clusterPrefix, err)
	}

	items, err := parseUnstructuredList(result)
	if err != nil {
		return nil, fmt.Errorf("parsing MCP list response for %s: %w", kind, err)
	}
	return items, nil
}

// GetLabels fetches only the labels of a resource from a remote cluster.
func (c *Client) GetLabels(ctx context.Context, clusterPrefix, kind, namespace, name string) (map[string]string, error) {
	obj, err := c.Get(ctx, clusterPrefix, kind, namespace, name)
	if err != nil {
		return nil, err
	}
	return obj.GetLabels(), nil
}

func (c *Client) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint:   c.endpoint,
		HTTPClient: c.httpClient,
	}

	session, err := c.mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		return "", fmt.Errorf("connecting to MCP gateway: %w", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", fmt.Errorf("calling tool %q: %w", name, err)
	}

	if result.IsError {
		return "", fmt.Errorf("tool %q returned error: %s", name, extractText(result))
	}

	return extractText(result), nil
}

func extractText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	data, _ := json.Marshal(result.Content)
	return string(data)
}

func parseUnstructured(text string) (*unstructured.Unstructured, error) {
	if text == "" {
		return nil, fmt.Errorf("empty response")
	}

	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal([]byte(text), &obj.Object); err != nil {
		return nil, fmt.Errorf("unmarshaling resource: %w", err)
	}
	return obj, nil
}

func parseUnstructuredList(text string) ([]unstructured.Unstructured, error) {
	if text == "" {
		return nil, nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		var items []map[string]any
		if err2 := json.Unmarshal([]byte(text), &items); err2 != nil {
			return nil, fmt.Errorf("unmarshaling list response: %w", err)
		}
		result := make([]unstructured.Unstructured, len(items))
		for i, item := range items {
			result[i] = unstructured.Unstructured{Object: item}
		}
		return result, nil
	}

	if items, ok := raw["items"].([]any); ok {
		result := make([]unstructured.Unstructured, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				result = append(result, unstructured.Unstructured{Object: m})
			}
		}
		return result, nil
	}

	return []unstructured.Unstructured{{Object: raw}}, nil
}
