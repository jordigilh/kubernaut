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
	"net/http"
	"sync/atomic"

	sharedauth "github.com/jordigilh/kubernaut/pkg/shared/auth"
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
// via RegisterTools before the handler is created.
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

// ToolCount returns the number of tools registered with the MCP SDK server.
func (s *MCPServer) ToolCount() int {
	return int(s.toolCount.Load())
}

// Server returns the underlying go-sdk MCP server for handler construction.
func (s *MCPServer) Server() *mcpsdk.Server {
	return s.server
}

// ToolRegistration is a function that registers a tool with the MCP SDK server.
// Each tool package provides its own registration function that calls mcp.AddTool
// with the correct generic type parameters, avoiding import cycles.
type ToolRegistration func(server *mcpsdk.Server, userFromCtx func(context.Context) UserInfo)

// ToolDeps holds optional tool registration functions.
// nil fields are skipped during registration.
type ToolDeps struct {
	Investigate    ToolRegistration
	Enrich         ToolRegistration
	SelectWorkflow ToolRegistration
}

// MCPDeps holds the dependencies needed to bootstrap the MCP server.
type MCPDeps struct {
	AuthMiddleware func(http.Handler) http.Handler
	Tools          ToolDeps
	EventStore     *DelegatingEventStore // nil = no stream resumption or disconnect detection
}

// userFromContext extracts the authenticated user identity from the request
// context (set by auth middleware). SEC-CRIT-02: uses GetUserInfoFromContext to
// propagate group memberships for impersonation in interactive sessions.
func userFromContext(ctx context.Context) UserInfo {
	info := sharedauth.GetUserInfoFromContext(ctx)
	if info.Username != "" {
		return UserInfo{Username: info.Username, Groups: info.Groups}
	}
	username := sharedauth.GetUserFromContext(ctx)
	return UserInfo{Username: username}
}

// registerTools invokes each non-nil ToolRegistration, which calls mcp.AddTool
// on the SDK server with the correct generic type parameters.
func (s *MCPServer) registerTools(deps ToolDeps) {
	userFn := userFromContext

	if deps.Investigate != nil {
		deps.Investigate(s.server, userFn)
		s.toolCount.Add(1)
	}
	if deps.Enrich != nil {
		deps.Enrich(s.server, userFn)
		s.toolCount.Add(1)
	}
	if deps.SelectWorkflow != nil {
		deps.SelectWorkflow(s.server, userFn)
		s.toolCount.Add(1)
	}
}

// BootstrapMCP creates a fully configured MCP handler with tools registered
// via the MCP SDK's AddTool. Returns the handler and the MCPServer for
// lifecycle management.
//
// Panics if AuthMiddleware is nil — the MCP endpoint must never be exposed
// without authentication (defense-in-depth, DD-AUTH-MCP-001).
func BootstrapMCP(deps MCPDeps) (http.Handler, *MCPServer) {
	if deps.AuthMiddleware == nil {
		panic("MCP interactive mode enabled but auth middleware is nil — refusing to start without authentication")
	}

	srv := NewMCPServer()
	srv.registerTools(deps.Tools)

	var opts *mcpsdk.StreamableHTTPOptions
	if deps.EventStore != nil {
		opts = &mcpsdk.StreamableHTTPOptions{
			EventStore: deps.EventStore,
		}
	}

	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server {
		return srv.Server()
	}, opts)

	handler := deps.AuthMiddleware(mcpHandler)
	return handler, srv
}

// BootstrapMCPNoAuth creates a configured MCP handler without auth middleware.
// Used ONLY by integration tests that provide their own auth context.
func BootstrapMCPNoAuth(toolDeps ToolDeps) (http.Handler, *MCPServer) {
	srv := NewMCPServer()
	srv.registerTools(toolDeps)

	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server {
		return srv.Server()
	}, nil)

	return mcpHandler, srv
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
