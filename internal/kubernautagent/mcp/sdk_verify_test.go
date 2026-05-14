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

package mcp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestOfficialMCPGoSDKCompiles verifies that modelcontextprotocol/go-sdk v1.5.0
// API surface matches our design: server creation, streamable HTTP handler,
// tool registration, progress notifications, and context cancellation.
func TestOfficialMCPGoSDKCompiles(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "kubernaut-agent-interactive",
		Version: "1.5.0",
	}, &mcp.ServerOptions{
		ProgressNotificationHandler: func(_ context.Context, _ *mcp.ProgressNotificationServerRequest) {
		},
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "kubernaut_investigate",
		Description: "Investigate a remediation request interactively",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)

	ts := httptest.NewServer(handler)
	defer ts.Close()

	if ts.URL == "" {
		t.Fatal("httptest server did not start")
	}
}
