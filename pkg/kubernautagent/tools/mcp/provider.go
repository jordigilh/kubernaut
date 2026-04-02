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
)

// Tool mirrors the tool interface for MCP-discovered tools.
type Tool struct {
	ToolName        string
	ToolDescription string
	ToolParameters  json.RawMessage
}

func (t Tool) Name() string                { return t.ToolName }
func (t Tool) Description() string          { return t.ToolDescription }
func (t Tool) Parameters() json.RawMessage  { return t.ToolParameters }

// MCPToolProvider discovers tools from an MCP server.
// v1.3: StubProvider returns empty; v1.4: real SSE transport.
type MCPToolProvider interface {
	DiscoverTools(ctx context.Context) ([]Tool, error)
	Close() error
}
