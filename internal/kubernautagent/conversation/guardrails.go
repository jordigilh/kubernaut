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

package conversation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// readOnlyTools is the canonical set of tool names allowed in conversation mode.
// todo_write is NOT listed here because it is injected per-session (DD-CONV-001 §4).
var readOnlyTools = map[string]bool{
	// k8s resource tools (exact names from pkg/kubernautagent/tools/k8s)
	"kubectl_get_by_name":                     true,
	"kubectl_get_by_kind_in_namespace":        true,
	"kubectl_get_by_kind_in_cluster":          true,
	"kubectl_describe":                        true,
	"kubectl_get_yaml":                        true,
	"kubectl_find_resource":                   true,
	"kubectl_events":                          true,
	// k8s memory/metrics tools
	"kubectl_memory_requests_all_namespaces":  true,
	"kubectl_memory_requests_namespace":       true,
	"kubectl_top_pods":                        true,
	"kubectl_top_nodes":                       true,
	// k8s log tools
	"kubectl_logs":                            true,
	"kubectl_previous_logs":                   true,
	"kubectl_logs_all_containers":             true,
	"kubectl_container_logs":                  true,
	"kubectl_container_previous_logs":         true,
	"kubectl_previous_logs_all_containers":    true,
	"kubectl_logs_grep":                       true,
	"kubectl_logs_all_containers_grep":        true,
	"fetch_pod_logs":                          true,
	// k8s JQ tools
	"kubernetes_jq_query":                     true,
	"kubernetes_count":                        true,
	// context tools
	"get_namespaced_resource_context":         true,
	"get_cluster_resource_context":            true,
	// prometheus tools
	"execute_prometheus_instant_query":        true,
	"execute_prometheus_range_query":          true,
	// workflow catalog tools
	"list_available_actions":                  true,
	"list_workflows":                          true,
	"get_workflow":                            true,
}

// Guardrails enforces RR-scoped access control during conversations:
// namespace restriction and read-only tool filtering.
type Guardrails struct {
	namespace string
	rrName    string
}

// NewGuardrails creates guardrails scoped to the given RR namespace and name.
func NewGuardrails(namespace, rrName string) *Guardrails {
	return &Guardrails{namespace: namespace, rrName: rrName}
}

// ValidateToolCall checks that a tool invocation is permitted:
// - Only read-only tools are allowed
// - Namespace must match the RR namespace for namespaced operations
func (g *Guardrails) ValidateToolCall(toolName string, args map[string]interface{}) error {
	if !g.IsReadOnlyTool(toolName) {
		return fmt.Errorf("tool %q rejected: only read-only operations are allowed in conversation mode", toolName)
	}
	if ns, ok := args["namespace"].(string); ok && ns != "" {
		if ns != g.namespace {
			return fmt.Errorf("tool %q rejected: namespace %q does not match session namespace %q", toolName, ns, g.namespace)
		}
	}
	return nil
}

// IsReadOnlyTool returns true if the tool name represents a read-only operation.
func (g *Guardrails) IsReadOnlyTool(toolName string) bool {
	if readOnlyTools[toolName] {
		return true
	}
	lower := strings.ToLower(toolName)
	return strings.HasPrefix(lower, "kubectl_get") ||
		strings.HasPrefix(lower, "kubectl_list") ||
		strings.HasPrefix(lower, "kubectl_describe")
}

// ReadOnlyToolNames returns a sorted list of all read-only tool names.
// Used by the prompt builder to list available tools in the system prompt.
func (g *Guardrails) ReadOnlyToolNames() []string {
	names := make([]string, 0, len(readOnlyTools))
	for name := range readOnlyTools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// FilterTools returns only tools that pass IsReadOnlyTool (map + prefix match).
// Used by the tool-call loop to build the tool set sent to the LLM (DD-CONV-001).
func (g *Guardrails) FilterTools(allTools []tools.Tool) []tools.Tool {
	var filtered []tools.Tool
	for _, t := range allTools {
		if g.IsReadOnlyTool(t.Name()) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

