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

// Package testutil provides a mock MCP gateway for unit and integration tests.
// It simulates MCP Gateway tool routing using configurable prefix conventions:
// EAIGW ("{clusterID}__{tool}") via WithMultiCluster, or Kuadrant-style
// ("{prefix}{tool}") via WithKuadrantCluster.
package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolCall records a single tool invocation for test assertions.
type ToolCall struct {
	ToolName  string
	Arguments json.RawMessage
}

// Option configures a MockGateway.
type Option func(*gatewayConfig)

type toolDef struct {
	name        string
	description string
	inputSchema json.RawMessage
	handler     mcp.ToolHandler
}

type clusterEntry struct {
	id     string
	prefix string
}

type discoverableCluster struct {
	name       string
	prefix     string
	categories []string
	hint       string
}

type gatewayConfig struct {
	tools                []toolDef
	clusterEntries       []clusterEntry
	discoverableClusters []discoverableCluster
	structuredContent    []map[string]any
}

// WithTool registers a static tool on the mock gateway.
func WithTool(name, description string, inputSchema json.RawMessage, handler mcp.ToolHandler) Option {
	return func(cfg *gatewayConfig) {
		cfg.tools = append(cfg.tools, toolDef{
			name:        name,
			description: description,
			inputSchema: inputSchema,
			handler:     handler,
		})
	}
}

// WithMultiCluster registers a set of cluster-scoped tools using the
// EAIGW naming convention "{clusterID}__{toolName}".
func WithMultiCluster(clusters ...string) Option {
	return func(cfg *gatewayConfig) {
		for _, c := range clusters {
			cfg.clusterEntries = append(cfg.clusterEntries, clusterEntry{id: c, prefix: c + "__"})
		}
	}
}

// WithStructuredContent configures the mock to return custom StructuredContent
// for resources_list calls. When set, this takes priority over the default
// table-format flat maps. Use for tests that need to inject specific shapes
// (e.g., full K8s objects or malformed data).
func WithStructuredContent(data []map[string]any) Option {
	return func(cfg *gatewayConfig) {
		cfg.structuredContent = data
	}
}

// WithKuadrantCluster registers cluster-scoped tools using a Kuadrant-style
// prefix (e.g., "cluster_a_"). The prefix is the full string prepended to
// tool names, matching MCPServerRegistration.spec.prefix behavior.
func WithKuadrantCluster(clusterID, prefix string) Option {
	return func(cfg *gatewayConfig) {
		cfg.clusterEntries = append(cfg.clusterEntries, clusterEntry{id: clusterID, prefix: prefix})
	}
}

// DiscoverableClusterOption configures a cluster entry for WithDiscoverableTools.
type DiscoverableClusterOption struct {
	Name       string
	Prefix     string
	Categories []string
	Hint       string
}

// WithDiscoverableTools registers discover_tools and select_tools meta-tools on
// the mock gateway, simulating Kuadrant MCP Gateway's progressive discovery API.
// Each cluster entry gets its kube-mcp-server tools registered with the given prefix,
// and the meta-tools return cluster metadata and scoped tool lists respectively.
func WithDiscoverableTools(clusters ...DiscoverableClusterOption) Option {
	return func(cfg *gatewayConfig) {
		for _, c := range clusters {
			cfg.discoverableClusters = append(cfg.discoverableClusters, discoverableCluster{
				name:       c.Name,
				prefix:     c.Prefix,
				categories: c.Categories,
				hint:       c.Hint,
			})
			cfg.clusterEntries = append(cfg.clusterEntries, clusterEntry{id: c.Name, prefix: c.Prefix})
		}
	}
}

// MockGateway is a test MCP server that simulates an MCP Gateway.
type MockGateway struct {
	server     *mcp.Server
	httpServer *httptest.Server
	handler    *mcp.StreamableHTTPHandler

	mu      sync.Mutex
	callLog []ToolCall
}

// NewMockGateway creates and starts a mock MCP gateway server.
func NewMockGateway(opts ...Option) *MockGateway {
	cfg := &gatewayConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	gw := &MockGateway{}
	gw.server = mcp.NewServer(
		&mcp.Implementation{Name: "mock-mcp-gateway", Version: "v0.1.0"},
		nil,
	)

	for _, td := range cfg.tools {
		gw.registerTool(td)
	}

	for _, entry := range cfg.clusterEntries {
		gw.registerClusterToolsWithPrefix(entry.id, entry.prefix, cfg)
	}

	if len(cfg.discoverableClusters) > 0 {
		gw.registerDiscoveryMetaTools(cfg.discoverableClusters)
	}

	gw.handler = mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return gw.server },
		nil,
	)
	gw.httpServer = httptest.NewServer(gw.handler)
	return gw
}

// URL returns the base URL of the mock gateway.
func (gw *MockGateway) URL() string {
	return gw.httpServer.URL
}

// Close shuts down the mock gateway.
func (gw *MockGateway) Close() {
	gw.httpServer.Close()
}

// CallLog returns a copy of all tool invocations recorded.
func (gw *MockGateway) CallLog() []ToolCall {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	result := make([]ToolCall, len(gw.callLog))
	copy(result, gw.callLog)
	return result
}

func (gw *MockGateway) recordCall(name string, args json.RawMessage) {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	gw.callLog = append(gw.callLog, ToolCall{ToolName: name, Arguments: args})
}

func (gw *MockGateway) registerTool(td toolDef) {
	wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gw.recordCall(req.Params.Name, req.Params.Arguments)
		return td.handler(ctx, req)
	}
	gw.server.AddTool(&mcp.Tool{
		Name:        td.name,
		Description: td.description,
		InputSchema: td.inputSchema,
	}, wrappedHandler)
}

func (gw *MockGateway) registerClusterToolsWithPrefix(cluster, prefix string, cfg *gatewayConfig) {
	getListSchema := json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"apiVersion":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"}},"required":["kind","apiVersion"]}`)
	deleteSchema := json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"apiVersion":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"}},"required":["kind","apiVersion","name"]}`)

	getResourceName := prefix + "resources_get"
	gw.server.AddTool(&mcp.Tool{
		Name:        getResourceName,
		Description: fmt.Sprintf("Get a Kubernetes resource from cluster %s", cluster),
		InputSchema: getListSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gw.recordCall(req.Params.Name, req.Params.Arguments)
		var args struct {
			Kind       string `json:"kind"`
			APIVersion string `json:"apiVersion"`
			Namespace  string `json:"namespace"`
			Name       string `json:"name"`
		}
		_ = json.Unmarshal(req.Params.Arguments, &args)

		if args.APIVersion == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: `{"error":"apiVersion is required"}`}},
				IsError: true,
			}, nil
		}

		obj := map[string]any{
			"apiVersion": args.APIVersion,
			"kind":       args.Kind,
			"metadata": map[string]any{
				"name":      args.Name,
				"namespace": args.Namespace,
				"labels": map[string]any{
					"kubernaut.ai/managed": "true",
					"app":                  "nginx",
				},
			},
			"status": map[string]any{"phase": "Running"},
		}
		response, _ := json.Marshal(obj)
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: string(response)}},
			StructuredContent: obj,
		}, nil
	})

	createResourceName := prefix + "resources_create_or_update"
	gw.server.AddTool(&mcp.Tool{
		Name:        createResourceName,
		Description: fmt.Sprintf("Create a Kubernetes resource on cluster %s", cluster),
		InputSchema: json.RawMessage(`{"type":"object","properties":{"manifest":{"type":"string"}},"required":["manifest"]}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gw.recordCall(req.Params.Name, req.Params.Arguments)
		var args struct {
			Manifest string `json:"manifest"`
		}
		_ = json.Unmarshal(req.Params.Arguments, &args)

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: args.Manifest}},
		}, nil
	})

	deleteResourceName := prefix + "resources_delete"
	gw.server.AddTool(&mcp.Tool{
		Name:        deleteResourceName,
		Description: fmt.Sprintf("Delete a Kubernetes resource from cluster %s", cluster),
		InputSchema: deleteSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gw.recordCall(req.Params.Name, req.Params.Arguments)
		var args struct {
			Kind       string `json:"kind"`
			APIVersion string `json:"apiVersion"`
		}
		_ = json.Unmarshal(req.Params.Arguments, &args)

		if args.APIVersion == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: `{"error":"apiVersion is required"}`}},
				IsError: true,
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"deleted"}`}},
		}, nil
	})

	listResourcesName := prefix + "resources_list"
	structuredContent := cfg.structuredContent
	gw.server.AddTool(&mcp.Tool{
		Name:        listResourcesName,
		Description: fmt.Sprintf("List Kubernetes resources from cluster %s", cluster),
		InputSchema: json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"apiVersion":{"type":"string"},"namespace":{"type":"string"},"labelSelector":{"type":"string"}},"required":["kind","apiVersion"]}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gw.recordCall(req.Params.Name, req.Params.Arguments)
		var args struct {
			Kind          string `json:"kind"`
			APIVersion    string `json:"apiVersion"`
			Namespace     string `json:"namespace"`
			LabelSelector string `json:"labelSelector"`
		}
		_ = json.Unmarshal(req.Params.Arguments, &args)

		if args.APIVersion == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: `{"error":"apiVersion is required"}`}},
				IsError: true,
			}, nil
		}

		if len(structuredContent) > 0 {
			items := make([]any, len(structuredContent))
			for i, m := range structuredContent {
				items[i] = m
			}
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: "structured content provided"}},
				StructuredContent: map[string]any{"items": items},
			}, nil
		}

		// Default: table-format structuredContent (flat maps matching
		// kube-mcp-server --list-output=table). Shape validated by Spike S17.
		item1 := map[string]any{
			"Name":   "item-1",
			"Ready":  "1/1",
			"Status": "Running",
			"Age":    "5m",
		}
		item2 := map[string]any{
			"Name":   "item-2",
			"Ready":  "1/1",
			"Status": "Running",
			"Age":    "3m",
		}
		if args.Namespace != "" {
			item1["Namespace"] = args.Namespace
			item2["Namespace"] = args.Namespace
		}

		tableText := fmt.Sprintf("NAMESPACE   NAME     READY   STATUS    AGE\n%-12s%-9s%-8s%-10s%s\n%-12s%-9s%-8s%-10s%s",
			args.Namespace, "item-1", "1/1", "Running", "5m",
			args.Namespace, "item-2", "1/1", "Running", "3m")

		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: tableText}},
			StructuredContent: map[string]any{"items": []any{item1, item2}},
		}, nil
	})
}

// registerDiscoveryMetaTools registers discover_tools and select_tools meta-tools
// that simulate Kuadrant MCP Gateway's progressive discovery API.
func (gw *MockGateway) registerDiscoveryMetaTools(clusters []discoverableCluster) {
	clusterIndex := make(map[string]discoverableCluster, len(clusters))
	clusterToolNames := make(map[string][]string, len(clusters))
	toolToCluster := make(map[string]string)
	for _, c := range clusters {
		clusterIndex[c.name] = c
		tools := []string{
			c.prefix + "resources_get",
			c.prefix + "resources_create_or_update",
			c.prefix + "resources_delete",
			c.prefix + "resources_list",
		}
		clusterToolNames[c.name] = tools
		for _, t := range tools {
			toolToCluster[t] = c.name
		}
	}

	discoverSchema := json.RawMessage(`{"type":"object","properties":{"category":{"type":"string","description":"Optional category filter"}},"additionalProperties":false}`)
	gw.server.AddTool(&mcp.Tool{
		Name:        "discover_tools",
		Description: "Discover available MCP servers and their tool categories",
		InputSchema: discoverSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gw.recordCall(req.Params.Name, req.Params.Arguments)

		var args struct {
			Category string `json:"category"`
		}
		if req.Params.Arguments != nil {
			_ = json.Unmarshal(req.Params.Arguments, &args)
		}

		type serverInfo struct {
			Name       string   `json:"name"`
			Categories []string `json:"categories"`
			Hint       string   `json:"hint,omitempty"`
			Tools      []string `json:"tools"`
		}
		var servers []serverInfo
		for _, c := range clusters {
			if args.Category != "" {
				matched := false
				for _, cat := range c.categories {
					if cat == args.Category {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			servers = append(servers, serverInfo{
				Name:       c.name,
				Categories: c.categories,
				Hint:       c.hint,
				Tools:      clusterToolNames[c.name],
			})
		}

		resp, _ := json.Marshal(map[string]any{"servers": servers})
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(resp)}},
		}, nil
	})

	selectSchema := json.RawMessage(`{"type":"object","properties":{"tools":{"type":"array","items":{"type":"string"},"description":"Tool names to scope the session to"}},"required":["tools"],"additionalProperties":false}`)
	gw.server.AddTool(&mcp.Tool{
		Name:        "select_tools",
		Description: "Select tools and scope the session to them",
		InputSchema: selectSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gw.recordCall(req.Params.Name, req.Params.Arguments)

		var args struct {
			Tools []string `json:"tools"`
		}
		_ = json.Unmarshal(req.Params.Arguments, &args)

		if len(args.Tools) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: `{"error":"invalid tools parameter"}`}},
				IsError: true,
			}, nil
		}

		clusterName, ok := toolToCluster[args.Tools[0]]
		if !ok {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{
					Text: fmt.Sprintf(`{"error":"unknown tool %q"}`, args.Tools[0]),
				}},
				IsError: true,
			}, nil
		}

		c := clusterIndex[clusterName]
		resp, _ := json.Marshal(map[string]any{
			"selected": c.name,
			"prefix":   c.prefix,
			"message":  fmt.Sprintf("Session scoped to %s tools (prefix: %s)", c.name, c.prefix),
		})
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(resp)}},
		}, nil
	})
}
