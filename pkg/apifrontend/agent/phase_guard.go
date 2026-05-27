package agent

import (
	"log"

	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/tool"
)

const stateKeyDriverActive = "af_interactive_driver_active"

// mcpDependentTools are tools that require an active interactive driver session
// (i.e., a successful kubernaut_takeover) before they can be called. Without
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
var driverEntryTools = map[string]bool{
	"kubernaut_takeover":  true,
	"kubernaut_reconnect": true,
}

// newPhaseGuard returns a BeforeToolCallback that blocks MCP-dependent tools
// unless a successful takeover/reconnect has been recorded in session state,
// and an AfterToolCallback that records successful takeover/reconnect.
func newPhaseGuard() (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
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

		log.Printf("[phase-guard] BLOCKED tool=%q reason=no_active_driver", t.Name())
		return map[string]any{
			"error": "interactive session not active — you must call kubernaut_takeover first to establish a driver session before using this tool",
		}, nil
	}

	after := func(ctx tool.Context, t tool.Tool, _ map[string]any, resp map[string]any, callErr error) (map[string]any, error) {
		if !driverEntryTools[t.Name()] {
			return resp, callErr
		}
		if callErr != nil || resp == nil {
			return resp, callErr
		}
		if errVal, ok := resp["error"]; ok && errVal != nil {
			return resp, callErr
		}

		state := ctx.State()
		if state == nil {
			return resp, callErr
		}
		if err := state.Set(stateKeyDriverActive, true); err != nil {
			log.Printf("[phase-guard] failed to set driver state: %v", err)
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
