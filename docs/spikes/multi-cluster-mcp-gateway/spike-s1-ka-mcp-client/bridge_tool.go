//go:build ignore

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
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BridgeTool wraps an MCP-discovered tool as a KA tools.Tool implementation.
// Each Execute() call creates a new MCP session (session-per-call pattern),
// matching AF's SDKMCPClient.callTool() approach.
//
// Authority: Spike 3 — KA as MCP Client (2026-06-04)
type BridgeTool struct {
	name        string
	description string
	parameters  json.RawMessage
	serverName  string
	factory     SessionFactory
}

// NewBridgeTool creates a bridge tool that executes remote MCP tools.
func NewBridgeTool(tool Tool, serverName string, factory SessionFactory) *BridgeTool {
	return &BridgeTool{
		name:        tool.ToolName,
		description: tool.ToolDescription,
		parameters:  tool.ToolParameters,
		serverName:  serverName,
		factory:     factory,
	}
}

func (b *BridgeTool) Name() string               { return b.name }
func (b *BridgeTool) Description() string         { return b.description }
func (b *BridgeTool) Parameters() json.RawMessage { return b.parameters }

// Execute calls the remote MCP tool via a new session.
// Returns the text content from the first text result, or the full JSON
// result if no text content is found.
func (b *BridgeTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	session, err := b.factory.NewSession(ctx)
	if err != nil {
		return "", fmt.Errorf("creating session for tool %q on %q: %w", b.name, b.serverName, err)
	}
	defer func() { _ = session.Close() }()

	var argsMap map[string]any
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &argsMap); err != nil {
			return "", fmt.Errorf("unmarshaling args for tool %q: %w", b.name, err)
		}
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      b.name,
		Arguments: argsMap,
	})
	if err != nil {
		return "", fmt.Errorf("calling tool %q on %q: %w", b.name, b.serverName, err)
	}

	if result.IsError {
		return "", fmt.Errorf("remote tool %q on %q returned error: %s", b.name, b.serverName, extractTextContent(result))
	}

	return extractTextContent(result), nil
}

// extractTextContent extracts the first text content from an MCP tool result.
// If no text content exists, returns the full JSON-marshaled result.
func extractTextContent(result *mcp.CallToolResult) string {
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
		return fmt.Sprintf("(failed to marshal result: %v)", err)
	}
	return string(data)
}
