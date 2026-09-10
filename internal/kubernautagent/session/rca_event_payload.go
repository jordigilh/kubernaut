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

package session

import (
	"encoding/json"
	"fmt"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// rcaEventPayload is the bounded subset of InvestigationResult that gets
// attached to the MCP complete event. It deliberately excludes internal
// workflow, validation, and alignment state to keep the payload small
// and avoid leaking implementation details to AF/Console.
type rcaEventPayload struct {
	Severity       string   `json:"severity,omitempty"`
	Confidence     float64  `json:"confidence,omitempty"`
	CausalChain    []string `json:"causal_chain,omitempty"`
	Target         string   `json:"target,omitempty"`
	RCASummary     string   `json:"rca_summary,omitempty"`
	// #2387: deliberately WITHOUT omitempty. A tracked-zero (0,0 after a
	// genuinely tool-free investigation) must serialize as explicit zeros so
	// AF can distinguish it from untracked (keys absent, pre-fix payloads).
	// The LLM is never asked to supply these (see #2073/#2074); KA injects
	// them server-side before MarshalRCASubset.
	TotalLLMTurns  int `json:"total_llm_turns"`
	TotalToolCalls int `json:"total_tool_calls"`
	// #2387 tokens: cumulative raw provider counts for console display
	// (never costs), sourced from InvestigationResult.TokenUsage. Omitempty
	// like the surrounding AF-owned wire contract: absent when never
	// recorded, present once any usage exists.
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	// IsActionable/HasWorkflow (#1918) give AF's phase_guard.go a structured,
	// model-independent signal for its harness-enforced Phase 2 gate --
	// mirrors the same condition (actionable=false && workflow_id=="")
	// investigator.go already treats as authoritative internally. A *bool
	// (rather than bool) so "never computed" (nil, omitted from JSON) stays
	// distinguishable from "computed false" -- the gate must only override
	// on a genuine false, never on absence.
	IsActionable *bool `json:"is_actionable,omitempty"`
	// HasWorkflow is derived from WorkflowID != "" rather than carrying the
	// raw ID itself, preserving this payload's existing SI-10 boundary of
	// never leaking internal workflow state (see the "should NOT leak
	// internal workflow or validation state" case below).
	HasWorkflow bool `json:"has_workflow,omitempty"`
}

// MarshalRCASubset extracts the AF-relevant fields from an InvestigationResult
// and marshals them into a compact JSON payload for the MCP complete event.
// Returns nil if result is nil.
func MarshalRCASubset(result *katypes.InvestigationResult) json.RawMessage {
	if result == nil {
		return nil
	}

	payload := rcaEventPayload{
		Severity:       result.Severity,
		Confidence:     result.Confidence,
		CausalChain:    result.CausalChain,
		Target:         formatTarget(result.RemediationTarget),
		RCASummary:     result.RCASummary,
		TotalLLMTurns:  result.TotalLLMTurns,
		TotalToolCalls: result.TotalToolCalls,
		IsActionable:   result.IsActionable,
		HasWorkflow:    result.WorkflowID != "",
	}
	// #2387 tokens: TokenUsage nil (never recorded, e.g. pre-fix results or
	// pure-sentinel flows) leaves all three keys absent.
	if result.TokenUsage != nil {
		payload.PromptTokens = result.TokenUsage.PromptTokens
		payload.CompletionTokens = result.TokenUsage.CompletionTokens
		payload.TotalTokens = result.TokenUsage.TotalTokens
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return data
}

func formatTarget(t katypes.RemediationTarget) string {
	if t.Kind == "" && t.Name == "" {
		return ""
	}
	if t.Namespace == "" {
		return fmt.Sprintf("%s/%s", t.Kind, t.Name)
	}
	return fmt.Sprintf("%s/%s in %s", t.Kind, t.Name, t.Namespace)
}
