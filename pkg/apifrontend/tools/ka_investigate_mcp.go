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

package tools

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

// InvestigateMCPArgs defines the input for the MCP-based kubernaut_investigate tool.
type InvestigateMCPArgs struct {
	RRID string `json:"rr_id"`
}

// InvestigateMCPResult is the output of the non-blocking MCP investigate.
type InvestigateMCPResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

// HandleInvestigationMCP starts an autonomous MCP investigation and returns
// immediately (non-blocking). The investigation runs in the background and
// streams events via the MCP LoggingMessage channel.
func HandleInvestigationMCP(ctx context.Context, mcpClient ka.MCPClient, args InvestigateMCPArgs, auditor audit.Emitter) (InvestigateMCPResult, error) {
	return HandleInvestigationMCPWithRegistry(ctx, mcpClient, args, auditor, nil)
}

// HandleInvestigationMCPWithRegistry is like HandleInvestigationMCP but also
// registers the session in a MonitorRegistry for lifecycle management.
func HandleInvestigationMCPWithRegistry(ctx context.Context, mcpClient ka.MCPClient, args InvestigateMCPArgs, auditor audit.Emitter, registry *MonitorRegistry) (InvestigateMCPResult, error) {
	if args.RRID == "" {
		return InvestigateMCPResult{}, fmt.Errorf("rr_id is required for MCP investigation")
	}

	result, err := mcpClient.StartAutonomous(ctx, ka.StartAutonomousArgs{
		RRID: args.RRID,
	})
	if err != nil {
		return InvestigateMCPResult{}, fmt.Errorf("start autonomous MCP investigation: %w", err)
	}

	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type: audit.EventKADelegated,
			Detail: map[string]string{
				"rr_id":             args.RRID,
				"session_id":        result.SessionID,
				"ka_correlation_id": result.SessionID,
				"delegation_type":   "autonomous_mcp",
			},
		})
	}

	if registry != nil {
		registry.Register(result.SessionID, result.Closer)
	}

	return InvestigateMCPResult{
		SessionID: result.SessionID,
		Status:    result.Status,
	}, nil
}

// MonitorRegistry tracks active autonomous investigation sessions and their
// cleanup functions. It provides lifecycle management for background goroutines.
type MonitorRegistry struct {
	mu       sync.Mutex
	sessions map[string]func()
}

// NewMonitorRegistry creates a new empty monitor registry.
func NewMonitorRegistry() *MonitorRegistry {
	return &MonitorRegistry{
		sessions: make(map[string]func()),
	}
}

// Register adds a session to the registry with its closer function.
func (r *MonitorRegistry) Register(sessionID string, closer func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sessionID] = closer
}

// Active returns true if the session is tracked in the registry.
func (r *MonitorRegistry) Active(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.sessions[sessionID]
	return ok
}

// Stop calls the closer for the session and removes it from the registry.
func (r *MonitorRegistry) Stop(sessionID string) {
	r.mu.Lock()
	closer, ok := r.sessions[sessionID]
	if ok {
		delete(r.sessions, sessionID)
	}
	r.mu.Unlock()

	if ok && closer != nil {
		closer()
	}
}

// StopAll calls all closers and clears the registry.
func (r *MonitorRegistry) StopAll() {
	r.mu.Lock()
	sessions := r.sessions
	r.sessions = make(map[string]func())
	r.mu.Unlock()

	for _, closer := range sessions {
		if closer != nil {
			closer()
		}
	}
}

// NewInvestigateMCPTool creates the kubernaut_investigate tool backed by MCP.
func NewInvestigateMCPTool(mcpClient ka.MCPClient, auditor audit.Emitter) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "kubernaut_investigate",
		Description: "Investigate an infrastructure incident via MCP. " +
			"Provide rr_id to start an autonomous investigation. " +
			"Returns the session ID when the investigation has started.",
	}, func(ctx tool.Context, args InvestigateMCPArgs) (InvestigateMCPResult, error) {
		return HandleInvestigationMCP(ctx, mcpClient, args, auditor)
	})
}
