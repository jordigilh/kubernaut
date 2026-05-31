package handler

// mcpToolRegistry maps tool names to their descriptions for the MCP server.
// This centralizes tool metadata and makes it easy to extend.
var mcpToolRegistry = []MCPToolDef{
	{Name: "kubernaut_list_remediations", Description: "List active and recent remediations"},
	{Name: "kubernaut_get_remediation", Description: "Get details of a specific remediation"},
	{Name: "kubernaut_approve", Description: "Approve a remediation action"},
	{Name: "kubernaut_cancel_remediation", Description: "Cancel an active remediation"},
	{Name: "kubernaut_watch", Description: "Watch for remediation state changes"},
	{Name: "kubernaut_investigate", Description: "Investigate an infrastructure incident"},
	{Name: "kubernaut_select_workflow", Description: "Select a workflow for an investigation"},
	{Name: "kubernaut_discover_workflows", Description: "Discover available workflows with parameter schemas"},
	{Name: "kubernaut_present_decision", Description: "Present a decision point requiring user input"},
	{Name: "kubernaut_list_workflows", Description: "List available workflows"},
	{Name: "kubernaut_get_remediation_history", Description: "Get remediation execution history"},
	{Name: "kubernaut_get_effectiveness", Description: "Get remediation effectiveness metrics"},
	{Name: "kubernaut_get_audit_trail", Description: "Get audit trail for remediations"},
	{Name: "kubernaut_message", Description: "Send a message to an active investigation session"},
	{Name: "kubernaut_complete", Description: "Complete an investigation session"},
	{Name: "kubernaut_cancel", Description: "Cancel an active investigation session"},
	{Name: "kubernaut_status", Description: "Get the current status of an investigation session"},
	{Name: "kubernaut_reconnect", Description: "Reconnect to a disconnected investigation session"},
	{Name: "kubernaut_list_approval_requests", Description: "List remediation approval requests"},
	{Name: "kubernaut_get_approval_request", Description: "Get details of a specific approval request"},
	{Name: "kubernaut_await_session", Description: "Wait for KA investigation session to become ready"},
	// kubernaut_check_existing_remediation and kubernaut_remediate are internal to AF's LLM agent (ADK path only).
}
