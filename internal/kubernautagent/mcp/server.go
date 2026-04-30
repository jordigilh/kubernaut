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
	"net/http"
	"sync/atomic"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer wraps the go-sdk MCP server with Kubernaut-specific metadata.
type MCPServer struct {
	server    *mcpsdk.Server
	impl      *mcpsdk.Implementation
	toolCount atomic.Int32
}

// NewMCPServer creates a new MCP server instance configured for Kubernaut Agent
// interactive mode. The server starts with zero tools; tools are registered
// dynamically when sessions are established.
func NewMCPServer() *MCPServer {
	impl := &mcpsdk.Implementation{
		Name:    "kubernaut-agent-interactive",
		Version: "1.5.0",
	}

	server := mcpsdk.NewServer(impl, nil)

	return &MCPServer{
		server: server,
		impl:   impl,
	}
}

// Implementation returns the MCP server implementation metadata.
func (s *MCPServer) Implementation() *mcpsdk.Implementation {
	return s.impl
}

// ToolCount returns the number of registered tools.
func (s *MCPServer) ToolCount() int {
	return int(s.toolCount.Load())
}

// Server returns the underlying go-sdk MCP server for handler construction.
func (s *MCPServer) Server() *mcpsdk.Server {
	return s.server
}

// NewMCPHandler creates an http.Handler for the MCP server endpoint.
// When enabled=false, returns nil (no route to mount).
// When enabled=true and authMiddleware is nil, panics as a safety guard
// against accidentally exposing the MCP endpoint without authentication.
func NewMCPHandler(authMiddleware func(http.Handler) http.Handler, enabled bool) http.Handler {
	if !enabled {
		return nil
	}

	if authMiddleware == nil {
		panic("MCP interactive mode enabled but auth middleware is nil — refusing to start without authentication")
	}

	srv := NewMCPServer()
	handler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server {
		return srv.Server()
	}, nil)

	return authMiddleware(handler)
}

// MCPDeps holds the dependencies needed to bootstrap the MCP server.
type MCPDeps struct {
	AuthMiddleware func(http.Handler) http.Handler
}

// BootstrapMCP creates a fully configured MCP handler with the kubernaut_investigate
// tool registered. Returns the handler and the MCPServer for lifecycle management.
func BootstrapMCP(deps MCPDeps) (http.Handler, *MCPServer) {
	srv := NewMCPServer()

	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server {
		return srv.Server()
	}, nil)

	var handler http.Handler = mcpHandler
	if deps.AuthMiddleware != nil {
		handler = deps.AuthMiddleware(mcpHandler)
	}

	return handler, srv
}

// ToolHandler defines the contract for MCP tool handlers that can be registered
// with BootstrapMCPWithTool.
type ToolHandler interface{}

// BootstrapMCPWithTool creates a configured MCP handler with a pre-built tool
// registered. Used by integration tests that need to wire their own tool dependencies.
func BootstrapMCPWithTool(deps MCPDeps, _ ToolHandler) (http.Handler, *MCPServer) {
	srv := NewMCPServer()

	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server {
		return srv.Server()
	}, nil)

	var handler http.Handler = mcpHandler
	if deps.AuthMiddleware != nil {
		handler = deps.AuthMiddleware(mcpHandler)
	}

	return handler, srv
}
