package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPToolDef defines a tool to be registered with the MCP server.
type MCPToolDef struct {
	Name        string
	Description string
	InputSchema any
}

// MCPConfig holds configuration for the MCP Streamable HTTP handler.
type MCPConfig struct {
	ServerName    string
	ServerVersion string
	Tools         []MCPToolDef
	// ToolCallback is invoked on each tool call for observability/testing hooks.
	ToolCallback func(ctx context.Context, toolName string)
}

func (c MCPConfig) validate() error {
	if c.ServerName == "" {
		return fmt.Errorf("server name is required")
	}
	if c.ServerVersion == "" {
		return fmt.Errorf("server version is required")
	}
	return nil
}

// NewMCPHandler creates an http.Handler serving the MCP Streamable HTTP protocol.
// Tools are registered as pass-through stubs that return "not implemented" until
// the real tool bridge is wired in Phase 9.
func NewMCPHandler(cfg MCPConfig) (http.Handler, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid MCP config: %w", err)
	}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    cfg.ServerName,
		Version: cfg.ServerVersion,
	}, nil)

	tools := cfg.Tools
	if tools == nil {
		tools = DefaultMCPTools()
	}

	for _, t := range tools {
		toolDef := &mcp.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
		cb := cfg.ToolCallback
		toolName := t.Name
		srv.AddTool(toolDef, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if cb != nil {
				cb(ctx, toolName)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("tool %q: not yet wired to backend", toolName)},
				},
			}, nil
		})
	}

	handler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return srv },
		nil,
	)

	return handler, nil
}

// DefaultMCPTools returns the 14 standard MCP tool definitions for the kubernaut agent.
// Each tool has a minimal "object" input schema; real schemas will be generated
// from the ADK tool structs in the full bridge (PR6+).
func DefaultMCPTools() []MCPToolDef {
	objectSchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}

	result := make([]MCPToolDef, len(mcpToolRegistry))
	for i, t := range mcpToolRegistry {
		result[i] = MCPToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: objectSchema,
		}
	}
	return result
}
