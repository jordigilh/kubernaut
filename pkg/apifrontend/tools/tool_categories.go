package tools

// SessionDependentTools lists MCP tool names that require KA interactive mode.
// When interactive mode is disabled in the AF config, these tools should not be
// registered at the MCP protocol level or in the A2A agent tool list.
//
// Classification rationale:
//   - investigate, discover_workflows, select_workflow, message, complete, cancel,
//     status, reconnect: require an active KA MCP session
//   - present_decision: pure formatter but only meaningful during investigations
//   - await_session: polls for KA session readiness
//
// See also: phase_guard.go (mcpDependentTools, driverEntryTools) for the A2A
// runtime ordering constraint, which is orthogonal to registration filtering.
var SessionDependentTools = map[string]bool{
	"kubernaut_investigate":        true,
	"kubernaut_discover_workflows": true,
	"kubernaut_select_workflow":    true,
	"kubernaut_present_decision":   true,
	"kubernaut_message":            true,
	"kubernaut_complete":           true,
	"kubernaut_cancel":             true,
	"kubernaut_status":             true,
	"kubernaut_reconnect":          true,
	"kubernaut_await_session":      true,
}
