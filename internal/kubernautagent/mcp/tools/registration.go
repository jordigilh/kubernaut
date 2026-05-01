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

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// InvestigateRegistration returns a ToolRegistration that registers the
// kubernaut_investigate tool with the MCP SDK server. When eventStore is
// non-nil, successful session starts register the MCP→interactive session
// mapping for disconnect detection (DD-INTERACTIVE-002).
// When notifier is non-nil, successful start/takeover registers
// ServerSession.Log as the session's notification callback (UX-01).
func InvestigateRegistration(tool *InvestigateTool, eventStore *mcpinternal.DelegatingEventStore, notifier *mcpinternal.SessionNotifier) mcpinternal.ToolRegistration {
	return func(server *mcpsdk.Server, userFromCtx func(context.Context) mcpinternal.UserInfo) {
		mcpsdk.AddTool(server, &mcpsdk.Tool{
			Name:        "kubernaut_investigate",
			Description: "Investigate a remediation request interactively. Actions: start, takeover, message, complete, cancel, status.",
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input InvestigateInput) (*mcpsdk.CallToolResult, InvestigateOutput, error) {
			user := userFromCtx(ctx)
			output, err := tool.Handle(ctx, input, user)
			if err == nil && output.SessionID != "" {
				if eventStore != nil {
					eventStore.RegisterMCPSession(req.Session.ID(), output.SessionID)
				}
				if notifier != nil && (output.Status == "started" || output.Status == "takeover_started") {
					sess := req.Session
					notifier.Register(output.SessionID, func(msg string) {
						_ = sess.Log(context.Background(), &mcpsdk.LoggingMessageParams{
							Level:  "warning",
							Logger: "kubernaut-interactive",
							Data:   msg,
						})
					})
				}
			}
			return nil, output, err
		})
	}
}

// EnrichRegistration returns a ToolRegistration that registers the
// kubernaut_enrich tool with the MCP SDK server.
func EnrichRegistration(tool *EnrichTool) mcpinternal.ToolRegistration {
	return func(server *mcpsdk.Server, userFromCtx func(context.Context) mcpinternal.UserInfo) {
		mcpsdk.AddTool(server, &mcpsdk.Tool{
			Name:        "kubernaut_enrich",
			Description: "Enrich a resource with K8s owner chain, labels, and remediation history during an interactive investigation.",
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input EnrichInput) (*mcpsdk.CallToolResult, EnrichOutput, error) {
			user := userFromCtx(ctx)
			output, err := tool.Handle(ctx, input, user)
			return nil, output, err
		})
	}
}

// SelectWorkflowRegistration returns a ToolRegistration that registers the
// kubernaut_select_workflow tool with the MCP SDK server.
func SelectWorkflowRegistration(tool *SelectWorkflowTool) mcpinternal.ToolRegistration {
	return func(server *mcpsdk.Server, userFromCtx func(context.Context) mcpinternal.UserInfo) {
		mcpsdk.AddTool(server, &mcpsdk.Tool{
			Name:        "kubernaut_select_workflow",
			Description: "Select a remediation workflow from the catalog during an interactive investigation.",
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input SelectWorkflowInput) (*mcpsdk.CallToolResult, SelectWorkflowOutput, error) {
			user := userFromCtx(ctx)
			output, err := tool.Handle(ctx, input, user)
			return nil, output, err
		})
	}
}
