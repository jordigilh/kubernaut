package agent

import (
	"github.com/go-logr/logr"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/tool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

const stateKeyDriverActive = "af_interactive_driver_active"

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
	"kubernaut_complete": true,
	"kubernaut_cancel":   true,
}

// newPhaseGuard returns a BeforeToolCallback that blocks MCP-dependent tools
// unless a successful takeover/reconnect has been recorded in session state,
// and an AfterToolCallback that records successful takeover/reconnect.
// When registry is non-nil, the after-callback also manages the
// ActiveContextRegistry for multi-turn session continuity (BR-SESS-020).
func newPhaseGuard(registry *launcher.ActiveContextRegistry) (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
	before := func(ctx tool.Context, t tool.Tool, _ map[string]any) (map[string]any, error) {
		if !mcpDependentTools[t.Name()] {
			return nil, nil
		}

		state := ctx.State()
		if state != nil {
			if active, err := state.Get(stateKeyDriverActive); err == nil {
				if b, ok := active.(bool); ok && b {
					return nil, nil
				}
			}
		}

		logr.FromContextOrDiscard(ctx).Info("phase-guard blocked tool", "tool", t.Name(), "reason", "no_active_driver")
		return map[string]any{
			"error": "interactive session not active — you must call kubernaut_investigate first to establish a driver session before using this tool",
		}, nil
	}

	after := func(ctx tool.Context, t tool.Tool, _ map[string]any, resp map[string]any, callErr error) (map[string]any, error) {
		toolName := t.Name()
		isEntry := driverEntryTools[toolName]
		isTerminal := sessionTerminalTools[toolName]

		if !isEntry && !isTerminal {
			return resp, callErr
		}
		if callErr != nil || resp == nil {
			return resp, callErr
		}
		if errVal, ok := resp["error"]; ok && errVal != nil {
			return resp, callErr
		}

		logger := logr.FromContextOrDiscard(ctx)

		if isEntry {
			state := ctx.State()
			if state != nil {
				if err := state.Set(stateKeyDriverActive, true); err != nil {
					logger.Error(err, "phase-guard failed to set driver state")
				}
			}
		}

		if registry != nil {
			if identity := auth.UserIdentityFromContext(ctx); identity != nil && identity.Username != "" {
				if isEntry {
					registry.Set(identity.Username, ctx.SessionID())
				} else if isTerminal {
					registry.Clear(identity.Username)
				}
			}
		}

		return resp, callErr
	}

	return before, after
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
