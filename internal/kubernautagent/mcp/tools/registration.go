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
	"encoding/json"

	"github.com/go-logr/logr"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// InvestigateRegistration returns a ToolRegistration that registers the
// kubernaut_investigate tool with the MCP SDK server. When eventStore is
// non-nil, successful session starts register the MCP→interactive session
// mapping for disconnect detection (DD-INTERACTIVE-002).
// When notifier is non-nil, successful start/takeover registers
// ServerSession.Log as the session's notification callback (UX-01).
//
// Identity resolution (#1287): when acting_user is present in the tool input
// (trusted intermediary model), it takes precedence over the middleware-extracted
// identity. This supports the AF SA token pattern where AF authenticates as
// itself and passes user identity in the payload.
func InvestigateRegistration(tool *InvestigateTool, eventStore *mcpinternal.DelegatingEventStore, notifier *mcpinternal.SessionNotifier, logger logr.Logger) mcpinternal.ToolRegistration {
	return func(server *mcpsdk.Server, userFromCtx func(context.Context) mcpinternal.UserInfo) {
		mcpsdk.AddTool(server, &mcpsdk.Tool{
			Name:        "kubernaut_investigate",
			Description: "Investigate a remediation request interactively. Actions: start, takeover, message, complete, cancel, status, reconnect, discover_workflows.",
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input InvestigateInput) (*mcpsdk.CallToolResult, InvestigateOutput, error) {
			user := ResolveUser(userFromCtx(ctx), input.ActingUser, input.ActingUserGroups)
			output, err := tool.Handle(ctx, input, user)
			if err != nil {
				return nil, output, ErrorBoundary(logger, "kubernaut_investigate", err)
			}

			registerMCPSessionMapping(req.Session, output, eventStore, notifier, logger) //nolint:contextcheck // registerMCPSessionMapping's notifier callback fires at an arbitrary future time, detached from the registering request
			wireInvestigationEventBridge(ctx, req.Session, tool, output, logger)

			return nil, output, nil
		})
	}
}

// registerMCPSessionMapping registers the MCP→interactive session mapping
// for disconnect detection (when eventStore is wired) and, for actions that
// (re)establish an active driver, registers the session's Log method as the
// interactive notification callback (when notifier is wired). No-op when
// output carries no session ID.
func registerMCPSessionMapping(sess *mcpsdk.ServerSession, output InvestigateOutput, eventStore *mcpinternal.DelegatingEventStore, notifier *mcpinternal.SessionNotifier, logger logr.Logger) {
	if output.SessionID == "" {
		return
	}
	if eventStore != nil {
		eventStore.RegisterMCPSession(sess.ID(), output.SessionID)
	}
	if notifier == nil {
		return
	}
	if output.Status != "started" && output.Status != "takeover_started" && output.Status != "reconnected" {
		return
	}
	notifier.Register(output.SessionID, func(msg string) {
		if logErr := sess.Log(context.Background(), &mcpsdk.LoggingMessageParams{
			Level:  "warning",
			Logger: "kubernaut-interactive",
			Data:   msg,
		}); logErr != nil {
			logger.Error(logErr, "notifier sess.Log failed",
				"session_id", output.SessionID)
		}
	})
}

// wireInvestigationEventBridge subscribes to the investigation's event
// stream and bridges it to MCP LoggingMessage notifications (BR-MCP-003),
// for action=start with a pending investigation. No-op for any other action
// or status.
func wireInvestigationEventBridge(ctx context.Context, sess *mcpsdk.ServerSession, tool *InvestigateTool, output InvestigateOutput, logger logr.Logger) {
	if output.InvestigationSessionID == "" || output.Status != "started" {
		return
	}
	logger.Info("subscribing to investigation events",
		"investigation_session_id", output.InvestigationSessionID,
		"mcp_session_id", output.SessionID)
	eventCh, subErr := tool.SubscribeEvents(ctx, output.InvestigationSessionID)
	if subErr != nil {
		logger.Error(subErr, "failed to subscribe to investigation events",
			"investigation_session_id", output.InvestigationSessionID)
		return
	}
	if eventCh == nil {
		logger.Info("subscribe returned nil channel, no events to bridge",
			"investigation_session_id", output.InvestigationSessionID)
		return
	}
	bridge := NewEventLogBridge(eventCh, func(level, loggerName string, data json.RawMessage) error { //nolint:contextcheck // wireInvestigationEventBridge's log-bridge callback fires for the life of the investigation, detached from the registering request
		return sess.Log(context.Background(), &mcpsdk.LoggingMessageParams{
			Level:  mcpsdk.LoggingLevel(level),
			Logger: loggerName,
			Data:   data,
		})
	}, logger, output.InvestigationSessionID)
	logger.Info("EventLogBridge wired",
		"investigation_session_id", output.InvestigationSessionID,
		"mcp_session_id", sess.ID())
	go bridge.Run(context.Background()) //nolint:contextcheck // event bridge runs detached for the life of the investigation session, independent of the registering request
}

// actingUserInput is satisfied by any MCP tool input struct that carries a
// trusted-intermediary identity override (#1287: acting_user/acting_user_groups
// take precedence over the middleware-extracted identity).
type actingUserInput interface {
	actingUserOverride() (actingUser string, actingUserGroups []string)
}

// simpleToolRegistration builds a ToolRegistration for tools that follow the
// plain resolve-user/handle/error-boundary shape, with no session-mapping or
// event-bridge wiring of their own (unlike InvestigateRegistration above).
// Issue #1530 (dupl): extracted from SelectWorkflowRegistration and
// CompleteNoActionRegistration, which were byte-identical apart from the
// tool name/description and the concrete input/output types.
func simpleToolRegistration[TIn actingUserInput, TOut any](
	name, description string,
	handle func(ctx context.Context, input TIn, user mcpinternal.UserInfo) (TOut, error),
	logger logr.Logger,
) mcpinternal.ToolRegistration {
	return func(server *mcpsdk.Server, userFromCtx func(context.Context) mcpinternal.UserInfo) {
		mcpsdk.AddTool(server, &mcpsdk.Tool{
			Name:        name,
			Description: description,
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input TIn) (*mcpsdk.CallToolResult, TOut, error) {
			actingUser, actingUserGroups := input.actingUserOverride()
			user := ResolveUser(userFromCtx(ctx), actingUser, actingUserGroups)
			output, err := handle(ctx, input, user)
			return nil, output, ErrorBoundary(logger, name, err)
		})
	}
}

// SelectWorkflowRegistration returns a ToolRegistration that registers the
// kubernaut_select_workflow tool with the MCP SDK server.
func SelectWorkflowRegistration(tool *SelectWorkflowTool, logger logr.Logger) mcpinternal.ToolRegistration {
	return simpleToolRegistration(
		"kubernaut_select_workflow",
		"Select a remediation workflow from the catalog during an interactive investigation. Requires a prior discover_workflows call.",
		tool.Handle,
		logger,
	)
}

// CompleteNoActionRegistration returns a ToolRegistration that registers the
// kubernaut_complete_no_action tool with the MCP SDK server.
func CompleteNoActionRegistration(tool *CompleteNoActionTool, logger logr.Logger) mcpinternal.ToolRegistration {
	return simpleToolRegistration(
		"kubernaut_complete_no_action",
		"Complete an interactive investigation without selecting a workflow. Use when no remediation action is needed.",
		tool.Handle,
		logger,
	)
}

// ListWorkflowsRegistration returns a ToolRegistration that registers the
// kubernaut_list_workflows tool with the MCP SDK server. Unlike
// SelectWorkflowRegistration/InvestigateRegistration, this tool is stateless
// (no rr_id/session gating) so it does not resolve caller identity via
// ResolveUser -- #1677 Phase 2f (DD-WORKFLOW-019).
func ListWorkflowsRegistration(tool *ListWorkflowsTool, logger logr.Logger) mcpinternal.ToolRegistration {
	return func(server *mcpsdk.Server, _ func(context.Context) mcpinternal.UserInfo) {
		mcpsdk.AddTool(server, &mcpsdk.Tool{
			Name:        "kubernaut_list_workflows",
			Description: "List available remediation workflows from the catalog, optionally filtered by resource kind",
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest, input ListWorkflowsInput) (*mcpsdk.CallToolResult, ListWorkflowsOutput, error) {
			output, err := tool.Handle(ctx, input)
			return nil, output, ErrorBoundary(logger, "kubernaut_list_workflows", err)
		})
	}
}
