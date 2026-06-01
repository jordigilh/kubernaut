package agent

import (
	"github.com/go-logr/logr"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/tool"
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

// newPhaseGuard returns a BeforeToolCallback that blocks MCP-dependent tools
// unless a successful takeover/reconnect has been recorded in session state,
// and an AfterToolCallback that records successful takeover/reconnect.
func newPhaseGuard() (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
	before := func(ctx tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
		if !mcpDependentTools[t.Name()] {
			return nil, nil
		}

		logger := logr.FromContextOrDiscard(ctx)
		state := ctx.State()
		if state == nil {
			logger.Info("phase-guard blocked tool", "tool", t.Name(), "reason", "no_active_driver")
			return map[string]any{"error": errNoActiveDriver}, nil
		}

		active, err := state.Get(stateKeyDriverActive)
		if err != nil || active == nil {
			logger.Info("phase-guard blocked tool", "tool", t.Name(), "reason", "no_active_driver")
			return map[string]any{"error": errNoActiveDriver}, nil
		}
		if b, ok := active.(bool); !ok || !b {
			logger.Info("phase-guard blocked tool", "tool", t.Name(), "reason", "no_active_driver")
			return map[string]any{"error": errNoActiveDriver}, nil
		}

		if args != nil {
			if rrID, _ := args["rr_id"].(string); rrID == "" {
				if storedRRID, sErr := state.Get(stateKeyActiveRRID); sErr == nil {
					if s, ok := storedRRID.(string); ok && s != "" {
						args["rr_id"] = s
						logger.Info("phase-guard injected rr_id from state",
							"tool", t.Name(), "rr_id", s)
					}
				}
			}
		}

		return nil, nil
	}

	after := func(ctx tool.Context, t tool.Tool, inputArgs map[string]any, resp map[string]any, callErr error) (map[string]any, error) {
		if !driverEntryTools[t.Name()] {
			return resp, callErr
		}
		if callErr != nil || resp == nil {
			return resp, callErr
		}
		if errVal, ok := resp["error"]; ok && errVal != nil {
			return resp, callErr
		}

		logger := logr.FromContextOrDiscard(ctx)
		state := ctx.State()
		if state == nil {
			return resp, callErr
		}
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
		return resp, callErr
	}

	return before, after
}

// NewPhaseGuardForTest exports the phase guard for unit testing.
func NewPhaseGuardForTest() (
	func(tool.Context, tool.Tool, map[string]any) (map[string]any, error),
	func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error),
) {
	return newPhaseGuard()
}
