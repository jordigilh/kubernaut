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
	"time"

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
func NewMCPServer(keepAlive time.Duration) *MCPServer {
	impl := &mcpsdk.Implementation{
		Name:    "kubernaut-agent-interactive",
		Version: "1.5.0",
	}

	var opts *mcpsdk.ServerOptions
	if keepAlive > 0 {
		opts = &mcpsdk.ServerOptions{KeepAlive: keepAlive}
	}
	server := mcpsdk.NewServer(impl, opts)

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
	Investigate      ToolRegistration
	SelectWorkflow   ToolRegistration
	CompleteNoAction ToolRegistration
	// ListWorkflows registers the stateless kubernaut_list_workflows catalog
	// browse tool (#1677 Phase 2f, DD-WORKFLOW-019).
	ListWorkflows ToolRegistration
}

// MCPDeps holds the dependencies needed to bootstrap the MCP server.
type MCPDeps struct {
	// AuthMiddleware is required (non-nil) as proof that auth is available.
	// BootstrapMCP does NOT apply it; the caller must apply it at router level.
	AuthMiddleware func(http.Handler) http.Handler
	Tools          ToolDeps
	EventStore     *DelegatingEventStore // nil = no stream resumption or disconnect detection

	// KeepAlive is the server-side ping interval for MCP sessions (#1387).
	// Zero means no keepalive pings.
	KeepAlive time.Duration
	// SessionTimeout auto-closes idle MCP HTTP sessions (#1387).
	// Zero means never.
	SessionTimeout time.Duration
}

// userFromContext extracts the authenticated user identity from the request
// context (set by auth middleware). Returns the SA identity for trusted
// intermediary callers; tools.ResolveUser further resolves acting_user.
func userFromContext(ctx context.Context) UserInfo {
	info := sharedauth.GetUserInfoFromContext(ctx)
	if info.Username != "" {
		return UserInfo{Username: info.Username, Groups: info.Groups, ProviderType: info.ProviderType}
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
	if deps.SelectWorkflow != nil {
		deps.SelectWorkflow(s.server, userFn)
		s.toolCount.Add(1)
	}
	if deps.CompleteNoAction != nil {
		deps.CompleteNoAction(s.server, userFn)
		s.toolCount.Add(1)
	}
	if deps.ListWorkflows != nil {
		deps.ListWorkflows(s.server, userFn)
		s.toolCount.Add(1)
	}
}

// BootstrapMCP creates a fully configured MCP handler with tools registered
// via the MCP SDK's AddTool. Returns the raw handler and the MCPServer for
// lifecycle management.
//
// Authentication is NOT applied internally — the caller is responsible for
// wrapping the returned handler with auth middleware (e.g., via chi r.Use).
// This matches the production pattern in cmd/kubernautagent/main.go where
// auth is applied once at the /api/v1 router level.
//
// Panics if AuthMiddleware is nil — defense-in-depth guard ensuring the caller
// has auth available even though BootstrapMCP does not apply it (DD-AUTH-MCP-001).
func BootstrapMCP(deps MCPDeps) (http.Handler, *MCPServer) {
	if deps.AuthMiddleware == nil {
		panic("MCP interactive mode enabled but auth middleware is nil — refusing to start without authentication")
	}

	srv := NewMCPServer(deps.KeepAlive)
	srv.registerTools(deps.Tools)

	var opts *mcpsdk.StreamableHTTPOptions
	if deps.EventStore != nil || deps.SessionTimeout > 0 {
		opts = &mcpsdk.StreamableHTTPOptions{}
		if deps.EventStore != nil {
			opts.EventStore = deps.EventStore
		}
		if deps.SessionTimeout > 0 {
			opts.SessionTimeout = deps.SessionTimeout
		}
	}

	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server {
		return srv.Server()
	}, opts)

	return mcpHandler, srv
}

