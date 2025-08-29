package slm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
)

// Tool descriptions for the model
const toolAwarePromptTemplate = `<|system|>
You are a Kubernetes operations expert with access to real-time cluster data and action history through MCP tools.

AVAILABLE TOOLS (you can request multiple tools in parallel):

## Kubernetes Cluster Tools:
1. **get_pod_status(namespace, pod_name)** - Get current pod status, resource usage, and health
2. **check_node_capacity()** - Get cluster node capacity, availability, and resource utilization
3. **get_recent_events(namespace)** - Get recent Kubernetes events for troubleshooting
4. **check_resource_quotas(namespace)** - Check namespace resource quotas and limits

## Action History Tools:
5. **get_action_history(namespace, resource)** - Get previous actions taken for this resource
6. **check_oscillation_risk(namespace, resource)** - Analyze oscillation patterns and thrashing risk
7. **get_effectiveness_metrics(namespace, resource)** - Get effectiveness data for previous actions

## RESPONSE FORMATS:

### If you need more information, respond with:
{
  "need_tools": true,
  "tool_requests": [
    {"name": "get_pod_status", "args": {"namespace": "production", "pod_name": "webapp-123"}},
    {"name": "check_node_capacity", "args": {}},
    {"name": "get_action_history", "args": {"namespace": "production", "resource": "webapp-123"}}
  ],
  "reasoning": "I need current pod status, cluster capacity, and action history to make an informed decision about this memory alert"
}

### If you have enough information for a decision, respond with:
{
  "need_tools": false,
  "action": "increase_resources",
  "parameters": {
    "cpu_limit": "500m",
    "memory_limit": "1Gi",
    "replicas": 3
  },
  "confidence": 0.85,
  "reasoning": "Based on the available information, increasing memory limits is the best approach because..."
}

## AVAILABLE ACTIONS:
### Core Actions:
- scale_deployment: Scale deployment replicas up or down
- restart_pod: Restart the affected pod(s)
- increase_resources: Increase CPU/memory limits
- notify_only: No automated action, notify operators only
- rollback_deployment: Rollback deployment to previous working revision
- expand_pvc: Expand persistent volume claim size
- drain_node: Safely drain and cordon a node for maintenance
- quarantine_pod: Isolate pod with network policies for security
- collect_diagnostics: Gather detailed diagnostic information

### Storage & Persistence Actions:
- cleanup_storage: Clean up old data/logs when disk space is critical
- backup_data: Trigger emergency backups before disruptive actions
- compact_storage: Trigger storage compaction operations

### Application Lifecycle Actions:
- cordon_node: Mark nodes as unschedulable (without draining)
- update_hpa: Modify horizontal pod autoscaler settings
- restart_daemonset: Restart DaemonSet pods across nodes

### Security & Compliance Actions:
- rotate_secrets: Rotate compromised credentials/certificates
- audit_logs: Trigger detailed security audit collection

### Network & Connectivity Actions:
- update_network_policy: Modify network policies for connectivity issues
- restart_network: Restart network components (CNI, DNS)
- reset_service_mesh: Reset service mesh configuration

### Database & Stateful Services Actions:
- failover_database: Trigger database failover to replica
- repair_database: Run database repair/consistency checks
- scale_statefulset: Scale StatefulSets with proper ordering

### Monitoring & Observability Actions:
- enable_debug_mode: Enable debug logging temporarily
- create_heap_dump: Trigger memory dumps for analysis

### Resource Management Actions:
- optimize_resources: Intelligently adjust resource requests/limits
- migrate_workload: Move workloads to different nodes/zones

## CRITICAL DECISION RULES:
1. **OOMKilled ALWAYS = increase_resources** (NEVER scale_deployment)
2. **Check action history FIRST** - avoid repeating failed actions
3. **Oscillation patterns = conservative actions** (notify_only, increase_resources)
4. **Node-level issues = drain_node/collect_diagnostics** (not pod actions)
5. **Unknown problems = collect_diagnostics + gather more data**

## CONFIDENCE SCORING:
- **0.9-1.0**: Clear pattern + historical confirmation + sufficient resources
- **0.7-0.9**: Clear root cause + some historical data + feasible action
- **0.5-0.7**: Multiple possible causes + mixed historical evidence
- **0.3-0.5**: Ambiguous symptoms + need more investigation
- **0.0-0.3**: Conflicting signals + high risk + recommend notify_only

<|user|>
ALERT ANALYSIS REQUEST:
Alert: %s
Status: %s
Severity: %s
Description: %s
Namespace: %s
Resource: %s
Labels: %v
Annotations: %v

Analyze this alert. Do you need more information from the available tools, or can you make a decision based on what you know?

Think step by step:
1. What type of problem does this alert indicate?
2. What additional information would help you make the best decision?
3. Which tools would provide that information?

Respond with either a tool request for more information or your final action decision.
<|assistant|>`

// generateToolAwarePrompt creates the initial prompt that introduces tools to the model
func (b *MCPBridge) generateToolAwarePrompt(alert types.Alert) string {
	return fmt.Sprintf(toolAwarePromptTemplate,
		alert.Name,
		alert.Status,
		alert.Severity,
		alert.Description,
		alert.Namespace,
		alert.Resource,
		alert.Labels,
		alert.Annotations,
	)
}

// generateToolResultsPrompt creates a prompt with tool execution results
func (b *MCPBridge) generateToolResultsPrompt(alert types.Alert, toolResults []ToolResult) string {
	resultsText := b.formatToolResults(toolResults)

	return fmt.Sprintf(`<|system|>
You are a Kubernetes operations expert. You requested information from MCP tools and received the results below.

ORIGINAL ALERT:
Alert: %s
Status: %s
Severity: %s
Description: %s
Namespace: %s
Resource: %s
Labels: %v
Annotations: %v

TOOL EXECUTION RESULTS:
%s

Based on this information, do you need additional data from other tools, or can you make your final decision?

## If you need more tools, respond with:
{
  "need_tools": true,
  "tool_requests": [
    {"name": "tool_name", "args": {"param": "value"}}
  ],
  "reasoning": "I need additional information because..."
}

## If ready to decide, respond with:
{
  "need_tools": false,
  "action": "chosen_action",
  "parameters": {"param": "value"},
  "confidence": 0.85,
  "reasoning": "Based on the tool results: [specific findings that led to this decision]..."
}

Remember to reference the specific tool results in your reasoning and explain how they influenced your decision.
<|assistant|>`,
		alert.Name,
		alert.Status,
		alert.Severity,
		alert.Description,
		alert.Namespace,
		alert.Resource,
		alert.Labels,
		alert.Annotations,
		resultsText,
	)
}

// formatToolResults formats tool execution results for the model
func (b *MCPBridge) formatToolResults(toolResults []ToolResult) string {
	var resultStrings []string

	for _, result := range toolResults {
		var resultText string

		if result.Error != "" {
			resultText = fmt.Sprintf("**%s**: ERROR - %s", result.Name, result.Error)
		} else {
			// Format the result data
			if resultData, err := json.MarshalIndent(result.Result, "", "  "); err == nil {
				resultText = fmt.Sprintf("**%s**: SUCCESS\n```json\n%s\n```", result.Name, string(resultData))
			} else {
				resultText = fmt.Sprintf("**%s**: SUCCESS - %v", result.Name, result.Result)
			}
		}

		resultStrings = append(resultStrings, resultText)
	}

	if len(resultStrings) == 0 {
		return "No tool results available."
	}

	return strings.Join(resultStrings, "\n\n")
}

// formatAlert formats alert information for prompts
func (b *MCPBridge) formatAlert(alert types.Alert) string {
	return fmt.Sprintf(`Alert: %s
Status: %s
Severity: %s
Description: %s
Namespace: %s
Resource: %s
Labels: %v
Annotations: %v`,
		alert.Name,
		alert.Status,
		alert.Severity,
		alert.Description,
		alert.Namespace,
		alert.Resource,
		alert.Labels,
		alert.Annotations,
	)
}
