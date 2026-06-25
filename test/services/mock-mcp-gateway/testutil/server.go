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

// Package testutil provides a mock MCP gateway for integration tests.
// It simulates an Envoy AI Gateway routing tool calls to per-cluster
// K8s MCP servers via the {backendName}__{toolName} prefix convention,
// enabling testing of multi-cluster federation without requiring a real
// gateway deployment.
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

type gatewayConfig struct {
	tools    []toolDef
	clusters []string
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
// naming convention "{clusterID}__{toolName}" as the MCP Gateway does.
func WithMultiCluster(clusters ...string) Option {
	return func(cfg *gatewayConfig) {
		cfg.clusters = append(cfg.clusters, clusters...)
	}
}

// MockGateway is a test MCP server that simulates the Envoy AI Gateway MCP Gateway.
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

	for _, cluster := range cfg.clusters {
		gw.registerClusterTools(cluster)
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

func (gw *MockGateway) registerClusterTools(cluster string) {
	schema := json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"namespace":{"type":"string"},"name":{"type":"string"}}}`)

	getResourceName := cluster + "__resources_get"
	gw.server.AddTool(&mcp.Tool{
		Name:        getResourceName,
		Description: fmt.Sprintf("Get a Kubernetes resource from cluster %s", cluster),
		InputSchema: schema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gw.recordCall(req.Params.Name, req.Params.Arguments)
		var args struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		}
		_ = json.Unmarshal(req.Params.Arguments, &args)

		response := fmt.Sprintf(`{"apiVersion":"v1","kind":%q,"metadata":{"name":%q,"namespace":%q,"labels":{"kubernaut.ai/managed":"true","app":"nginx"}},"status":{"phase":"Running"}}`,
			args.Kind, args.Name, args.Namespace)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: response}},
		}, nil
	})

	createResourceName := cluster + "__resources_create_or_update"
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

	deleteResourceName := cluster + "__resources_delete"
	gw.server.AddTool(&mcp.Tool{
		Name:        deleteResourceName,
		Description: fmt.Sprintf("Delete a Kubernetes resource from cluster %s", cluster),
		InputSchema: schema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gw.recordCall(req.Params.Name, req.Params.Arguments)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"deleted"}`}},
		}, nil
	})

	listResourcesName := cluster + "__resources_list"
	gw.server.AddTool(&mcp.Tool{
		Name:        listResourcesName,
		Description: fmt.Sprintf("List Kubernetes resources from cluster %s", cluster),
		InputSchema: json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"namespace":{"type":"string"},"labelSelector":{"type":"string"}}}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		gw.recordCall(req.Params.Name, req.Params.Arguments)
		var args struct {
			Kind          string `json:"kind"`
			Namespace     string `json:"namespace"`
			LabelSelector string `json:"labelSelector"`
		}
		_ = json.Unmarshal(req.Params.Arguments, &args)

		response := fmt.Sprintf(`{"apiVersion":"v1","kind":"List","items":[{"apiVersion":"v1","kind":%q,"metadata":{"name":"item-1","namespace":%q,"labels":{"app":"nginx"}}},{"apiVersion":"v1","kind":%q,"metadata":{"name":"item-2","namespace":%q,"labels":{"app":"nginx"}}}]}`,
			args.Kind, args.Namespace, args.Kind, args.Namespace)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: response}},
		}, nil
	})
}
