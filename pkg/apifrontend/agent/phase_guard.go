package agent

import (
	"github.com/go-logr/logr"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

const (
	stateKeyDriverActive  = "af_interactive_driver_active"
	stateKeyActiveRRID    = "af_active_rr_id"
	stateKeyActiveSession = "af_active_session_id"
)

const errNoActiveDriver = "interactive session not active — you must call kubernaut_investigate first to establish a driver session before using this tool"

// mcpDependentTools are tools that require an active interactive driver session
// (i.e., a successful kubernaut_investigate) before they can be called. Without
// this prerequisite, KA rejects them with not_driving errors.
var mcpDependentTools = map[string]bool{
	"kubernaut_discover_workflows": true,
	"kubernaut_select_workflow":    true,
	"kubernaut_message":            true,
	"kubernaut_complete":           true,
	"kubernaut_cancel":             true,
	"kubernaut_status":             true,
}

// driverEntryTools are tools that establish the interactive driver session.
// After a successful call to one of these, mcpDependentTools are unblocked.
// kubernaut_investigate is included because it handles both fresh investigations
// and takeover of autonomous sessions (consolidated per #1332).
var driverEntryTools = map[string]bool{
	"kubernaut_investigate": true,
	"kubernaut_reconnect":   true,
}

// sessionTerminalTools end the active investigation session.
// A successful call clears the ActiveContextRegistry entry (BR-SESS-022).
var sessionTerminalTools = map[string]bool{
	"kubernaut_complete":           true,
	"kubernaut_cancel":             true,
	"kubernaut_complete_no_action": true,
}

// newPhaseGuard returns a BeforeToolCallback that blocks MCP-dependent tools
// unless a successful takeover/reconnect has been recorded in session state,
// and an AfterToolCallback that records successful takeover/reconnect.
// When registry is non-nil, the after-callback also manages the
// ActiveContextRegistry for multi-turn session continuity (BR-SESS-020).
func newPhaseGuard(registry *launcher.ActiveContextRegistry) (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
	return phaseGuardBefore, func(ctx tool.Context, t tool.Tool, inputArgs, resp map[string]any, callErr error) (map[string]any, error) {
		return phaseGuardAfter(registry, ctx, t, inputArgs, resp, callErr)
	}
}

// phaseGuardBefore blocks MCP-dependent tool calls unless a driver session is
// active in state, and injects a stashed rr_id when the caller omitted one.
func phaseGuardBefore(ctx tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
	if !mcpDependentTools[t.Name()] {
		return nil, nil
	}

	logger := logr.FromContextOrDiscard(ctx)
	state := ctx.State()
	if !driverIsActive(state) {
		logger.Info("phase-guard blocked tool", "tool", t.Name(), "reason", "no_active_driver")
		return map[string]any{"error": errNoActiveDriver}, nil
	}

	injectStoredRRID(state, args, t.Name(), logger)
	return nil, nil
}

// driverIsActive reports whether the session state records an active
// interactive driver (i.e., a prior successful takeover/reconnect).
func driverIsActive(state session.State) bool {
	if state == nil {
		return false
	}
	active, err := state.Get(stateKeyDriverActive)
	if err != nil || active == nil {
		return false
	}
	b, ok := active.(bool)
	return ok && b
}

// injectStoredRRID fills args["rr_id"] from session state when the caller
// did not supply one, so MCP-dependent tools can omit it after takeover.
func injectStoredRRID(state session.State, args map[string]any, toolName string, logger logr.Logger) {
	if args == nil || state == nil {
		return
	}
	if rrID, _ := args["rr_id"].(string); rrID != "" {
		return
	}
	storedRRID, err := state.Get(stateKeyActiveRRID)
	if err != nil {
		return
	}
	s, ok := storedRRID.(string)
	if !ok || s == "" {
		return
	}
	args["rr_id"] = s
	logger.Info("phase-guard injected rr_id from state", "tool", toolName, "rr_id", s)
}

// phaseGuardAfter records successful driver-entry/session-terminal tool calls
// into session state and, when registry is non-nil, keeps the
// ActiveContextRegistry in sync for multi-turn session continuity (BR-SESS-020).
func phaseGuardAfter(registry *launcher.ActiveContextRegistry, ctx tool.Context, t tool.Tool, inputArgs, resp map[string]any, callErr error) (map[string]any, error) {
	toolName := t.Name()
	isEntry := driverEntryTools[toolName]
	isTerminal := sessionTerminalTools[toolName]
	isSuccess := toolCallSucceeded(callErr, resp)

	// Refresh idle timer for any successful tool call to keep the
	// active session alive during ongoing engagement (#1446, AU-3).
	if registry != nil && isSuccess && !isEntry && !isTerminal {
		refreshActiveContext(registry, ctx)
	}

	if (!isEntry && !isTerminal) || !isSuccess {
		return resp, callErr
	}

	if isEntry {
		recordDriverEntryState(ctx, inputArgs, resp)
	}
	syncActiveContextRegistry(registry, ctx, isEntry, isTerminal)

	return resp, callErr
}

// toolCallSucceeded reports whether a tool call completed without a Go error
// and without an embedded "error" field in its response payload.
func toolCallSucceeded(callErr error, resp map[string]any) bool {
	if callErr != nil || resp == nil {
		return false
	}
	if errVal, ok := resp["error"]; ok && errVal != nil {
		return false
	}
	return true
}

func refreshActiveContext(registry *launcher.ActiveContextRegistry, ctx tool.Context) {
	if identity := auth.UserIdentityFromContext(ctx); identity != nil && identity.Username != "" {
		registry.Refresh(identity.Username)
	}
}

// recordDriverEntryState persists driver-active flag, rr_id, and session_id
// into session state after a successful driver-entry tool call (investigate/reconnect).
func recordDriverEntryState(ctx tool.Context, inputArgs, resp map[string]any) {
	state := ctx.State()
	if state == nil {
		return
	}
	logger := logr.FromContextOrDiscard(ctx)

	if err := state.Set(stateKeyDriverActive, true); err != nil {
		logger.Error(err, "phase-guard failed to set driver state")
	}

	// Prefer rr_id from response (kubernaut_investigate returns it).
	// Fall back to input args (kubernaut_reconnect takes it as input
	// but does not echo it in the response).
	if rrID, ok := resp["rr_id"].(string); ok && rrID != "" {
		if err := state.Set(stateKeyActiveRRID, rrID); err != nil {
			logger.Error(err, "phase-guard failed to store rr_id in state")
		}
	} else if inputArgs != nil {
		if rrID, ok := inputArgs["rr_id"].(string); ok && rrID != "" {
			if err := state.Set(stateKeyActiveRRID, rrID); err != nil {
				logger.Error(err, "phase-guard failed to store rr_id from input args")
			}
		}
	}

	if sessionID, ok := resp["session_id"].(string); ok && sessionID != "" {
		if err := state.Set(stateKeyActiveSession, sessionID); err != nil {
			logger.Error(err, "phase-guard failed to store session_id in state")
		}
	}
}

// syncActiveContextRegistry sets or clears the per-user ActiveContextRegistry
// entry after a successful driver-entry or session-terminal tool call.
func syncActiveContextRegistry(registry *launcher.ActiveContextRegistry, ctx tool.Context, isEntry, isTerminal bool) {
	if registry == nil {
		return
	}
	identity := auth.UserIdentityFromContext(ctx)
	if identity == nil || identity.Username == "" {
		return
	}
	switch {
	case isEntry:
		registry.Set(identity.Username, ctx.SessionID())
	case isTerminal:
		registry.Clear(identity.Username)
	}
}

// NewPhaseGuardForTest exports the phase guard without registry for unit testing.
func NewPhaseGuardForTest() (
	func(tool.Context, tool.Tool, map[string]any) (map[string]any, error),
	func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error),
) {
	return newPhaseGuard(nil)
}

// NewPhaseGuardWithRegistryForTest exports the phase guard with registry for
// session continuity integration testing (BR-SESS-020).
func NewPhaseGuardWithRegistryForTest(registry *launcher.ActiveContextRegistry) (
	func(tool.Context, tool.Tool, map[string]any) (map[string]any, error),
	func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error),
) {
	return newPhaseGuard(registry)
}
