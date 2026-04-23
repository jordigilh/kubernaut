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
package openai

// ToolCall represents a tool invocation in an assistant message.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the function name and arguments for a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool represents a tool definition in a chat completion request.
type Tool struct {
	Type     string         `json:"type"`
	Function ToolDefinition `json:"function"`
}

// ToolDefinition describes a function available as a tool.
type ToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// Tool name constants shared between Mock LLM and Kubernaut Agent.
// These must match the Go Mock LLM string values exactly.
const (
	ToolSearchWorkflowCatalog        = "search_workflow_catalog"
	ToolListAvailableActions         = "list_available_actions"
	ToolListWorkflows                = "list_workflows"
	ToolGetWorkflow                  = "get_workflow"
	ToolGetResourceContext           = "get_resource_context"
	ToolFetchKubernetesResourceYaml  = "fetchKubernetesResourceYaml"
	ToolListNamespacedEvents         = "listNamespacedEvents"
	ToolKubectlGetYAML               = "kubectl_get_yaml"
	ToolKubectlGetByName             = "kubectl_get_by_name"
	ToolSubmitResult                 = "submit_result"
	ToolSubmitResultWithWorkflow     = "submit_result_with_workflow"
	ToolSubmitResultNoWorkflow       = "submit_result_no_workflow"
)
