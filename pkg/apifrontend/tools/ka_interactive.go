package tools

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// InteractiveActionArgs defines the shared input for interactive investigation actions.
type InteractiveActionArgs struct {
	RRID    string `json:"rr_id"`
	Message string `json:"message,omitempty"`
}

// InteractiveActionResult is the shared output for interactive investigation actions.
type InteractiveActionResult struct {
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

func invokeInteractiveAction(ctx context.Context, mcpClient ka.MCPClient, action string, args InteractiveActionArgs, auditor audit.Emitter, auditType audit.EventType) (InteractiveActionResult, error) {
	if mcpClient == nil {
		return InteractiveActionResult{}, fmt.Errorf("interactive investigation not available: MCP client not configured")
	}
	if err := validate.ResourceName(args.RRID); err != nil {
		return InteractiveActionResult{}, fmt.Errorf("invalid rr_id: %w", err)
	}
	if err := validate.Action(action); err != nil {
		return InteractiveActionResult{}, err
	}
	if args.Message != "" {
		if err := validate.MessageLength(args.Message); err != nil {
			return InteractiveActionResult{}, err
		}
	}

	result, err := mcpClient.InvokeAction(ctx, ka.InvokeActionArgs{
		RRID:    args.RRID,
		Action:  action,
		Message: args.Message,
	})
	if err != nil {
		return InteractiveActionResult{}, fmt.Errorf("%s: %w", action, err)
	}

	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type:   auditType,
			Detail: interactiveAuditDetail(action, args.RRID, result, auditType),
		})
	}

	return InteractiveActionResult{
		SessionID: result.SessionID,
		Status:    result.Status,
	}, nil
}

// actionResultTypes maps interactive actions to their OpenAPI result_type enum
// value for EventKAResultReceived (enum: rca_complete, rca_failed, timeout, cancelled).
var actionResultTypes = map[string]string{
	"complete": "rca_complete",
	"cancel":   "cancelled",
}

// actionToolNames maps interactive actions to their canonical tool name for
// EventToolExecuted (required by OpenAPI ApifrontendToolExecutedPayload).
var actionToolNames = map[string]string{
	"message": "kubernaut_message",
	"status":  "kubernaut_status",
}

// interactiveAuditDetail builds the Detail map for an interactive action audit
// event. Common fields are always present; type-specific fields (result_type for
// EventKAResultReceived, tool_name for EventToolExecuted) are added only when
// the audit schema requires them.
func interactiveAuditDetail(action, rrID string, result *ka.InvokeActionResult, auditType audit.EventType) map[string]string {
	d := map[string]string{
		"rr_id":             rrID,
		"action":            action,
		"session_id":        result.SessionID,
		"status":            result.Status,
		"ka_correlation_id": result.SessionID,
		"delegation_type":   "interactive",
		"tool_outcome":      "success",
	}
	if rt, ok := actionResultTypes[action]; ok && auditType == audit.EventKAResultReceived {
		d["result_type"] = rt
	}
	if tn, ok := actionToolNames[action]; ok && auditType == audit.EventToolExecuted {
		d["tool_name"] = tn
	}
	return d
}

// HandleMessage implements the kubernaut_message tool.
func HandleMessage(ctx context.Context, mcpClient ka.MCPClient, args InteractiveActionArgs, auditor audit.Emitter) (InteractiveActionResult, error) {
	if args.Message == "" {
		return InteractiveActionResult{}, fmt.Errorf("message is required for kubernaut_message")
	}
	return invokeInteractiveAction(ctx, mcpClient, "message", args, auditor, audit.EventToolExecuted)
}

// HandleComplete implements the kubernaut_complete tool.
func HandleComplete(ctx context.Context, mcpClient ka.MCPClient, args InteractiveActionArgs, auditor audit.Emitter) (InteractiveActionResult, error) {
	return invokeInteractiveAction(ctx, mcpClient, "complete", args, auditor, audit.EventKAResultReceived)
}

// HandleCancel implements the kubernaut_cancel tool.
func HandleCancel(ctx context.Context, mcpClient ka.MCPClient, args InteractiveActionArgs, auditor audit.Emitter) (InteractiveActionResult, error) {
	return invokeInteractiveAction(ctx, mcpClient, "cancel", args, auditor, audit.EventKAResultReceived)
}

// HandleStatus implements the kubernaut_status tool.
func HandleStatus(ctx context.Context, mcpClient ka.MCPClient, args InteractiveActionArgs, auditor audit.Emitter) (InteractiveActionResult, error) {
	return invokeInteractiveAction(ctx, mcpClient, "status", args, auditor, audit.EventToolExecuted)
}

// HandleReconnect implements the kubernaut_reconnect tool.
// Before delegating to the MCP client, it validates that the referenced RR
// exists and is in a non-terminal phase. If the RR is missing or terminal,
// the session is considered expired and a graceful result is returned,
// preventing misleading "reconnecting" UX (#1472, BR-SESS-025).
func HandleReconnect(ctx context.Context, mcpClient ka.MCPClient, k8sClient crclient.Client, namespace string, args InteractiveActionArgs, auditor audit.Emitter) (InteractiveActionResult, error) {
	if k8sClient != nil && namespace != "" && args.RRID != "" {
		var rr remediationv1.RemediationRequest
		if err := k8sClient.Get(ctx, crclient.ObjectKey{Namespace: namespace, Name: args.RRID}, &rr); err != nil {
			logger := logr.FromContextOrDiscard(ctx)
			logger.Info("kubernaut_reconnect: RR not found — session expired (#1472, SI-10)",
				"rr_id", args.RRID,
				"error", err.Error(),
			)
			return InteractiveActionResult{
				Status:  "session_expired",
				Message: fmt.Sprintf("Investigation session expired: remediation request %q no longer exists. Start a new investigation.", args.RRID),
			}, nil
		}
		if IsTerminalPhase(string(rr.Status.OverallPhase)) {
			logger := logr.FromContextOrDiscard(ctx)
			logger.Info("kubernaut_reconnect: RR in terminal phase — session expired (#1472, SI-10)",
				"rr_id", args.RRID,
				"phase", string(rr.Status.OverallPhase),
			)
			return InteractiveActionResult{
				Status:  "session_expired",
				Message: fmt.Sprintf("Investigation session expired: remediation request %q is in terminal phase %q. Start a new investigation.", args.RRID, rr.Status.OverallPhase),
			}, nil
		}
	}
	return invokeInteractiveAction(ctx, mcpClient, "reconnect", args, auditor, audit.EventKADelegated)
}


// NewMessageTool creates the kubernaut_message tool.
func NewMessageTool(mcpClient ka.MCPClient, auditor audit.Emitter) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_message",
		Description: "Send a message to an active investigation session",
	}, func(ctx tool.Context, args InteractiveActionArgs) (InteractiveActionResult, error) {
		return HandleMessage(ctx, mcpClient, args, auditor)
	})
}

// NewCompleteTool creates the kubernaut_complete tool.
func NewCompleteTool(mcpClient ka.MCPClient, auditor audit.Emitter) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_complete",
		Description: "Complete an investigation session",
	}, func(ctx tool.Context, args InteractiveActionArgs) (InteractiveActionResult, error) {
		return HandleComplete(ctx, mcpClient, args, auditor)
	})
}

// NewCancelInvestigationTool creates the kubernaut_cancel tool.
func NewCancelInvestigationTool(mcpClient ka.MCPClient, auditor audit.Emitter) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_cancel",
		Description: "Cancel an active investigation session",
	}, func(ctx tool.Context, args InteractiveActionArgs) (InteractiveActionResult, error) {
		return HandleCancel(ctx, mcpClient, args, auditor)
	})
}

// NewStatusTool creates the kubernaut_status tool.
func NewStatusTool(mcpClient ka.MCPClient, auditor audit.Emitter) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_status",
		Description: "Get the current status of an investigation session",
	}, func(ctx tool.Context, args InteractiveActionArgs) (InteractiveActionResult, error) {
		return HandleStatus(ctx, mcpClient, args, auditor)
	})
}

// NewReconnectTool creates the kubernaut_reconnect tool.
// The k8sClient and namespace are used to validate RR existence before
// attempting reconnection (#1472, BR-SESS-025).
func NewReconnectTool(mcpClient ka.MCPClient, k8sClient crclient.Client, namespace string, auditor audit.Emitter) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_reconnect",
		Description: "Reconnect to a disconnected investigation session",
	}, func(ctx tool.Context, args InteractiveActionArgs) (InteractiveActionResult, error) {
		return HandleReconnect(ctx, mcpClient, k8sClient, namespace, args, auditor)
	})
}
