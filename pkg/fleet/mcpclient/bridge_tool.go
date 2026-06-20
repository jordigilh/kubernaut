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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// Compile-time interface compliance.
var _ tools.Tool = (*BridgeTool)(nil)

// Session abstracts an MCP client session for testing.
type Session interface {
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
}

// BridgeTool wraps an MCP-discovered tool as a KA tools.Tool implementation.
// Only used for remote cluster tools — local cluster tools keep using
// existing direct k8stools.
type BridgeTool struct {
	name        string
	description string
	parameters  json.RawMessage
	clusterID   string
	session     Session
}

// ToolDefinition holds the metadata for an MCP tool discovered via tools/list.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// NewBridgeTool creates a bridge tool that executes a remote MCP tool via the
// given session. The clusterID is used for audit logging context.
func NewBridgeTool(def ToolDefinition, clusterID string, session Session) *BridgeTool {
	return &BridgeTool{
		name:        def.Name,
		description: def.Description,
		parameters:  def.InputSchema,
		clusterID:   clusterID,
		session:     session,
	}
}

// NewBridgeToolFromSession creates a BridgeTool using a direct MCP session,
// auto-parsing the clusterID from the tool name's "{clusterID}__" prefix.
// Falls back to empty clusterID for tools without the prefix convention.
func NewBridgeToolFromSession(def ToolDefinition, session *mcp.ClientSession) *BridgeTool {
	clusterID := parseClusterIDFromToolName(def.Name)
	return &BridgeTool{
		name:        def.Name,
		description: def.Description,
		parameters:  def.InputSchema,
		clusterID:   clusterID,
		session:     session,
	}
}

// ClusterID returns the cluster this tool is bound to.
func (b *BridgeTool) ClusterID() string { return b.clusterID }

// parseClusterIDFromToolName extracts the clusterID from a tool name following
// the "{clusterID}__tool_name" convention. Returns empty string if no prefix found.
func parseClusterIDFromToolName(name string) string {
	if idx := strings.Index(name, "__"); idx > 0 {
		return name[:idx]
	}
	return ""
}

func (b *BridgeTool) Name() string               { return b.name }
func (b *BridgeTool) Description() string         { return b.description }
func (b *BridgeTool) Parameters() json.RawMessage { return b.parameters }

// Execute calls the remote MCP tool via the shared session.
// Returns text content from the result, joining multiple text parts with newlines.
func (b *BridgeTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var argsMap map[string]any
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &argsMap); err != nil {
			return "", fmt.Errorf("unmarshal args for tool %q: %w", b.name, err)
		}
	}

	result, err := b.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      b.name,
		Arguments: argsMap,
	})
	if err != nil {
		return "", fmt.Errorf("call tool %q (cluster %q): %w", b.name, b.clusterID, err)
	}

	if result.IsError {
		return "", fmt.Errorf("remote tool %q (cluster %q) returned error: %s",
			b.name, b.clusterID, ExtractText(result))
	}

	return ExtractText(result), nil
}
