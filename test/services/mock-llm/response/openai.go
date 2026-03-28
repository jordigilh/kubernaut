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
package response

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// BuildToolCallResponse creates a ChatCompletionResponse with a single tool call.
func BuildToolCallResponse(model, toolName string, cfg scenarios.MockScenarioConfig) openai.ChatCompletionResponse {
	args := buildToolArguments(toolName, cfg)
	argsJSON, _ := json.Marshal(args)

	content := (*string)(nil)
	return openai.ChatCompletionResponse{
		ID:      randomChatID(),
		Object:  openai.ObjectChatCompletion,
		Created: openai.FixedCreatedTime,
		Model:   model,
		Choices: []openai.Choice{
			{
				Index: 0,
				Message: openai.Message{
					Role:    "assistant",
					Content: content,
					ToolCalls: []openai.ToolCall{
						{
							ID:   randomCallID(),
							Type: "function",
							Function: openai.FunctionCall{
								Name:      toolName,
								Arguments: string(argsJSON),
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: openai.Usage{PromptTokens: 500, CompletionTokens: 50, TotalTokens: 550},
	}
}

// BuildTextResponse creates a ChatCompletionResponse with the final RCA text.
func BuildTextResponse(model string, cfg scenarios.MockScenarioConfig) openai.ChatCompletionResponse {
	text := buildAnalysisText(cfg)
	return openai.ChatCompletionResponse{
		ID:      randomChatID(),
		Object:  openai.ObjectChatCompletion,
		Created: openai.FixedCreatedTime,
		Model:   model,
		Choices: []openai.Choice{
			{
				Index: 0,
				Message: openai.Message{
					Role:    "assistant",
					Content: &text,
				},
				FinishReason: "stop",
			},
		},
		Usage: openai.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	}
}

// BuildForceTextResponse produces a text-only response regardless of tools provided.
// Used when MOCK_LLM_FORCE_TEXT=true or when tools are nil/empty.
func BuildForceTextResponse(model string, cfg scenarios.MockScenarioConfig, _ []openai.Tool) openai.ChatCompletionResponse {
	return BuildTextResponse(model, cfg)
}

// BuildErrorResponse creates an OpenAI-compatible error response.
func BuildErrorResponse(message string) openai.ErrorResponse {
	return openai.ErrorResponse{
		Error: openai.ErrorDetail{
			Message: message,
			Type:    "server_error",
			Code:    "internal_error",
		},
	}
}

func buildToolArguments(toolName string, cfg scenarios.MockScenarioConfig) map[string]interface{} {
	switch toolName {
	case openai.ToolSearchWorkflowCatalog:
		return map[string]interface{}{
			"query": fmt.Sprintf("%s %s", cfg.SignalName, cfg.Severity),
			"rca_resource": map[string]string{
				"signal_name": cfg.SignalName,
				"kind":        cfg.ResourceKind,
				"namespace":   cfg.ResourceNS,
				"name":        cfg.ResourceName,
			},
		}
	case openai.ToolListAvailableActions:
		return map[string]interface{}{"limit": 100}
	case openai.ToolListWorkflows:
		return map[string]interface{}{"action_type": "remediation"}
	case openai.ToolGetWorkflow:
		return map[string]interface{}{"workflow_id": cfg.WorkflowID}
	case openai.ToolGetResourceContext:
		return map[string]interface{}{
			"kind": cfg.ResourceKind, "namespace": cfg.ResourceNS, "name": cfg.ResourceName,
		}
	default:
		return map[string]interface{}{}
	}
}

func buildAnalysisText(cfg scenarios.MockScenarioConfig) string {
	if cfg.WorkflowID == "" {
		return fmt.Sprintf(
			"Based on my investigation of the incident:\n\n## Root Cause Analysis\n\n%s\n\n"+
				"```json\n{\n  \"root_cause_analysis\": {\n    \"summary\": %q,\n    \"severity\": %q,\n    "+
				"\"contributing_factors\": %s\n  }\n}\n```\n",
			cfg.RootCause, cfg.RootCause, cfg.Severity,
			contributingJSON(cfg),
		)
	}
	return fmt.Sprintf(
		"Based on my investigation of the incident:\n\n## Root Cause Analysis\n\n%s\n\n"+
			"```json\n{\n  \"root_cause_analysis\": {\n    \"summary\": %q,\n    \"severity\": %q,\n    "+
			"\"contributing_factors\": %s\n  },\n  \"selected_workflow\": {\n    \"workflow_id\": %q,\n    "+
			"\"version\": \"1.0.0\",\n    \"confidence\": %.2f,\n    "+
			"\"rationale\": \"Selected based on signal analysis\",\n    \"execution_engine\": %q\n  }\n}\n```\n",
		cfg.RootCause, cfg.RootCause, cfg.Severity,
		contributingJSON(cfg),
		cfg.WorkflowID, cfg.Confidence,
		executionEngine(cfg),
	)
}

func contributingJSON(cfg scenarios.MockScenarioConfig) string {
	if len(cfg.Contributing) > 0 {
		data, _ := json.Marshal(cfg.Contributing)
		return string(data)
	}
	return `["traffic_spike", "resource_limits"]`
}

func executionEngine(cfg scenarios.MockScenarioConfig) string {
	if cfg.ExecutionEngine != "" {
		return cfg.ExecutionEngine
	}
	return "tekton"
}

func randomChatID() string {
	return "chatcmpl-" + randomHex(4)
}

func randomCallID() string {
	return "call_" + randomHex(6)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
