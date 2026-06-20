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

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SessionFactory creates MCP sessions for tool execution.
// Extracted as an interface for testability — tests inject a mock,
// production uses StreamableProvider.
type SessionFactory interface {
	NewSession(ctx context.Context) (Session, error)
}

// Session wraps the MCP SDK session for tool calls.
type Session interface {
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	ListTools(ctx context.Context, params *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	Close() error
}

// sdkSession wraps *mcp.ClientSession to implement Session.
type sdkSession struct {
	session *mcp.ClientSession
}

func (s *sdkSession) CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return s.session.CallTool(ctx, params)
}

func (s *sdkSession) ListTools(ctx context.Context, params *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	return s.session.ListTools(ctx, params)
}

func (s *sdkSession) Close() error {
	return s.session.Close()
}

// StreamableProvider connects to an MCP server via StreamableClientTransport
// and discovers tools. Implements both MCPToolProvider and SessionFactory.
type StreamableProvider struct {
	config     ServerConfig
	httpClient *http.Client
	mcpClient  *mcp.Client
	logger     logr.Logger
}

// NewStreamableProvider creates a provider that connects to an MCP gateway.
func NewStreamableProvider(config ServerConfig, httpClient *http.Client, logger logr.Logger) *StreamableProvider {
	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "kubernaut-agent",
		Version: "1.5.0",
	}, nil)

	return &StreamableProvider{
		config:     config,
		httpClient: httpClient,
		mcpClient:  mcpClient,
		logger:     logger.WithName("mcp-provider").WithValues("server", config.Name),
	}
}

// DiscoverTools connects to the MCP server and lists available tools.
func (p *StreamableProvider) DiscoverTools(ctx context.Context) ([]Tool, error) {
	session, err := p.NewSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("connecting to MCP server %q: %w", p.config.Name, err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("listing tools from %q: %w", p.config.Name, err)
	}

	tools := make([]Tool, 0, len(result.Tools))
	for _, t := range result.Tools {
		params, err := marshalInputSchema(t.InputSchema)
		if err != nil {
			p.logger.Info("skipping tool with unmarshalable schema", "tool", t.Name, "error", err)
			continue
		}
		tools = append(tools, Tool{
			ToolName:        t.Name,
			ToolDescription: t.Description,
			ToolParameters:  params,
		})
	}

	p.logger.Info("discovered MCP tools", "count", len(tools))
	return tools, nil
}

// NewSession creates a new MCP session for tool execution.
func (p *StreamableProvider) NewSession(ctx context.Context) (Session, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint:   p.config.URL,
		HTTPClient: p.httpClient,
	}

	session, err := p.mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("MCP connect to %q at %s: %w", p.config.Name, p.config.URL, err)
	}
	return &sdkSession{session: session}, nil
}

// Close is a no-op for StreamableProvider (sessions are closed individually).
func (p *StreamableProvider) Close() error {
	return nil
}

// marshalInputSchema converts the MCP tool's InputSchema (any) to json.RawMessage.
// Handles nil schemas (tools with no parameters) and complex JSON Schema types.
func marshalInputSchema(schema any) (json.RawMessage, error) {
	if schema == nil {
		return json.RawMessage(`{}`), nil
	}
	if raw, ok := schema.(json.RawMessage); ok {
		return raw, nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshaling input schema: %w", err)
	}
	if len(data) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return data, nil
}

// Compile-time interface compliance.
var (
	_ MCPToolProvider = (*StreamableProvider)(nil)
	_ SessionFactory  = (*StreamableProvider)(nil)
)
