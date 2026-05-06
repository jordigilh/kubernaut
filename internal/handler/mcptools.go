package handler

// mcpToolRegistry maps tool names to their descriptions for the MCP server.
// This centralizes tool metadata and makes it easy to extend.
var mcpToolRegistry = []MCPToolDef{
	{Name: "kubernaut_list_remediations", Description: "List active and recent remediations"},
	{Name: "kubernaut_get_remediation", Description: "Get details of a specific remediation"},
	{Name: "kubernaut_submit_signal", Description: "Submit a signal to an active remediation"},
	{Name: "kubernaut_approve", Description: "Approve a remediation action"},
	{Name: "kubernaut_cancel_remediation", Description: "Cancel an active remediation"},
	{Name: "kubernaut_watch", Description: "Watch for remediation state changes"},
	{Name: "kubernaut_start_investigation", Description: "Start a new investigation session"},
	{Name: "kubernaut_poll_investigation", Description: "Poll an investigation session for updates"},
	{Name: "kubernaut_select_workflow", Description: "Select a workflow for an investigation"},
	{Name: "kubernaut_present_decision", Description: "Present a decision point requiring user input"},
	{Name: "kubernaut_list_workflows", Description: "List available workflows"},
	{Name: "kubernaut_get_remediation_history", Description: "Get remediation execution history"},
	{Name: "kubernaut_get_effectiveness", Description: "Get remediation effectiveness metrics"},
	{Name: "kubernaut_get_audit_trail", Description: "Get audit trail for remediations"},
}
