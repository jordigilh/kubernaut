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
	case openai.ToolSubmitResultWithWorkflow:
		return analysisJSON(cfg)
	case openai.ToolSubmitResultNoWorkflow:
		rca := map[string]interface{}{
			"summary":     cfg.RootCause,
			"severity":    cfg.Severity,
			"signal_name": cfg.SignalName,
		}
		return map[string]interface{}{
			"root_cause_analysis": rca,
			"reasoning":           "No matching workflow found for this scenario",
		}
	default:
		return map[string]interface{}{}
	}
}

// analysisJSON builds a structured response that KA's parser can fully extract.
// Top-level fields (investigation_outcome, actionable, severity, confidence) are
// required by KA's outcome routing; needs_human_review / human_review_reason are
// parser-derived (BR-HAPI-200) and must NOT appear in LLM responses.
//
// Golden transcript ref: kubernaut-demo-scenarios#296 — response structure mirrors
// real Claude Sonnet 4 output to ensure KA parser fidelity.
func analysisJSON(cfg scenarios.MockScenarioConfig) map[string]interface{} {
	rca := map[string]interface{}{
		"summary":              cfg.RootCause,
		"severity":             cfg.Severity,
		"signal_name":          cfg.SignalName,
		"contributing_factors": contributingSlice(cfg),
	}
	if cfg.ResourceKind != "" {
		rca["remediation_target"] = map[string]string{
			"kind":      cfg.ResourceKind,
			"name":      cfg.ResourceName,
			"namespace": cfg.ResourceNS,
		}
	}

	obj := map[string]interface{}{
		"root_cause_analysis": rca,
		"severity":            cfg.Severity,
		"confidence":          cfg.Confidence,
	}

	if cfg.WorkflowID != "" {
		sw := map[string]interface{}{
			"workflow_id":      cfg.WorkflowID,
			"confidence":       cfg.Confidence,
			"rationale":        workflowRationale(cfg),
			"execution_engine": executionEngine(cfg),
		}
		if len(cfg.Parameters) > 0 {
			sw["parameters"] = cfg.Parameters
		}
		obj["selected_workflow"] = sw
	}

	if len(cfg.Alternatives) > 0 {
		alts := make([]map[string]interface{}, 0, len(cfg.Alternatives))
		for _, a := range cfg.Alternatives {
			alts = append(alts, map[string]interface{}{
				"workflow_id": a.WorkflowID,
				"confidence":  a.Confidence,
				"rationale":   a.Rationale,
			})
		}
		obj["alternative_workflows"] = alts
	}

	if cfg.InvestigationOutcome != "" {
		obj["investigation_outcome"] = cfg.InvestigationOutcome
	}
	if cfg.IsActionable != nil {
		obj["actionable"] = *cfg.IsActionable
	}
	return obj
}

func workflowRationale(cfg scenarios.MockScenarioConfig) string {
	if cfg.Rationale != "" {
		return cfg.Rationale
	}
	return "Selected based on signal analysis"
}

func buildAnalysisText(cfg scenarios.MockScenarioConfig) string {
	if cfg.ExactAnalysisText != "" {
		return cfg.ExactAnalysisText
	}
	obj := analysisJSON(cfg)
	data, _ := json.MarshalIndent(obj, "", "  ")
	return string(data)
}

func contributingSlice(cfg scenarios.MockScenarioConfig) []string {
	if len(cfg.Contributing) > 0 {
		return cfg.Contributing
	}
	return []string{"traffic_spike", "resource_limits"}
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
