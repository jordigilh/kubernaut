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

package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

// ResultParser extracts and validates InvestigationResult from LLM JSON output.
type ResultParser struct{}

// NewResultParser creates a new result parser.
func NewResultParser() *ResultParser {
	return &ResultParser{}
}

// Parse extracts InvestigationResult from raw LLM content.
// Handles both raw JSON and JSON embedded in markdown code blocks.
func (p *ResultParser) Parse(content string) (*katypes.InvestigationResult, error) {
	if content == "" {
		return nil, fmt.Errorf("empty JSON content")
	}

	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var result katypes.InvestigationResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err == nil && (result.RCASummary != "" || result.WorkflowID != "") {
		var flat flatLLMFields
		_ = json.Unmarshal([]byte(jsonStr), &flat)
		applyFlatFields(&result, flat)
		mergeNestedRemediationTarget(&result, jsonStr)
		applyOutcomeRouting(&result)
		return &result, nil
	}

	parsed, err := parseLLMFormat(jsonStr)
	if err != nil {
		return nil, err
	}
	applyOutcomeRouting(parsed)
	return parsed, nil
}

// extractJSON finds JSON content using a priority chain:
// 1. Fenced ```json ... ``` code blocks
// 2. Fenced ``` ... ``` code blocks containing JSON
// 3. Raw string starting with {
// 4. Balanced brace extraction (GAP-003: handles JSON embedded in prose)
func extractJSON(content string) string {
	if idx := strings.Index(content, "```json"); idx != -1 {
		start := idx + len("```json")
		end := strings.Index(content[start:], "```")
		if end != -1 {
			return strings.TrimSpace(content[start : start+end])
		}
	}
	if idx := strings.Index(content, "```"); idx != -1 {
		start := idx + len("```")
		end := strings.Index(content[start:], "```")
		if end != -1 {
			candidate := strings.TrimSpace(content[start : start+end])
			if len(candidate) > 0 && candidate[0] == '{' {
				return candidate
			}
		}
	}
	trimmed := strings.TrimSpace(content)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed
	}
	return extractBalancedJSON(content)
}

// extractBalancedJSON finds the first complete JSON object in content
// by counting balanced braces. Handles JSON embedded in prose text,
// mirroring HAPI's json_utils.py balanced extraction.
func extractBalancedJSON(content string) string {
	start := strings.IndexByte(content, '{')
	if start == -1 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(content); i++ {
		ch := content[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}
	return ""
}

const notActionableWarning = "Alert not actionable — no remediation warranted"
const confidenceFloor = 0.8

// llmResponse is the nested JSON structure that LLMs typically produce.
type llmResponse struct {
	RCA                  *llmRCA      `json:"root_cause_analysis"`
	Workflow             *llmWorkflow `json:"selected_workflow"`
	Actionable           *bool        `json:"actionable,omitempty"`
	InvestigationOutcome string       `json:"investigation_outcome,omitempty"`
	NeedsHumanReview     *bool        `json:"needs_human_review,omitempty"`
	HumanReviewReason    string       `json:"human_review_reason,omitempty"`
}

type llmRCA struct {
	Summary              string        `json:"summary"`
	RemediationTarget    *llmRemTarget `json:"remediation_target,omitempty"`
	RemediationTargetAlt *llmRemTarget `json:"remediationTarget,omitempty"`
}

func (r *llmRCA) resolvedTarget() *llmRemTarget {
	if r.RemediationTarget != nil {
		return r.RemediationTarget
	}
	return r.RemediationTargetAlt
}

type llmRemTarget struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type llmWorkflow struct {
	WorkflowID      string  `json:"workflow_id"`
	ExecutionBundle string  `json:"execution_bundle,omitempty"`
	Confidence      float64 `json:"confidence"`
}

// flatLLMFields captures top-level fields that may appear alongside the flat
// InvestigationResult format (rca_summary, workflow_id, confidence, etc.).
type flatLLMFields struct {
	Actionable           *bool  `json:"actionable,omitempty"`
	InvestigationOutcome string `json:"investigation_outcome,omitempty"`
	NeedsHumanReview     *bool  `json:"needs_human_review,omitempty"`
	HumanReviewReason    string `json:"human_review_reason,omitempty"`
}

// parseLLMFormat parses the nested LLM response format and converts
// it to a flat InvestigationResult.
func parseLLMFormat(jsonStr string) (*katypes.InvestigationResult, error) {
	var resp llmResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing LLM JSON response: %w", err)
	}

	result := &katypes.InvestigationResult{}
	if resp.RCA != nil {
		result.RCASummary = resp.RCA.Summary
		if t := resp.RCA.resolvedTarget(); t != nil {
			result.RemediationTarget = katypes.RemediationTarget{
				Kind:      t.Kind,
				Name:      t.Name,
				Namespace: t.Namespace,
			}
		}
	}
	if resp.Workflow != nil {
		result.WorkflowID = resp.Workflow.WorkflowID
		result.ExecutionBundle = resp.Workflow.ExecutionBundle
		result.Confidence = resp.Workflow.Confidence
	}

	applyFlatFields(result, flatLLMFields{
		Actionable:           resp.Actionable,
		InvestigationOutcome: resp.InvestigationOutcome,
		NeedsHumanReview:     resp.NeedsHumanReview,
		HumanReviewReason:    resp.HumanReviewReason,
	})

	if result.RCASummary == "" && result.WorkflowID == "" {
		return nil, fmt.Errorf("no recognized fields in LLM JSON response")
	}

	return result, nil
}

// mergeNestedRemediationTarget checks for a nested root_cause_analysis.remediation_target
// (or camelCase remediationTarget) when the flat parse path succeeded but
// RemediationTarget is still empty. This handles hybrid JSON where the LLM
// returns flat rca_summary/workflow_id alongside a nested RCA object.
func mergeNestedRemediationTarget(result *katypes.InvestigationResult, jsonStr string) {
	if result.RemediationTarget.Kind != "" {
		return
	}
	var resp llmResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil || resp.RCA == nil {
		return
	}
	if t := resp.RCA.resolvedTarget(); t != nil {
		result.RemediationTarget = katypes.RemediationTarget{
			Kind:      t.Kind,
			Name:      t.Name,
			Namespace: t.Namespace,
		}
	}
}

// applyFlatFields applies LLM-provided flat fields to the result:
// - actionable: sets IsActionable, synthesizes warning, applies confidence floor
// - investigation_outcome: maps to outcome routing fields
// - needs_human_review / human_review_reason: propagates directly
func applyFlatFields(result *katypes.InvestigationResult, flat flatLLMFields) {
	if flat.Actionable != nil && !*flat.Actionable {
		falseVal := false
		result.IsActionable = &falseVal
		result.Warnings = append(result.Warnings, notActionableWarning)
		if result.Confidence < confidenceFloor {
			result.Confidence = confidenceFloor
		}
	}

	if flat.InvestigationOutcome != "" {
		applyInvestigationOutcome(result, flat.InvestigationOutcome)
	}

	if flat.NeedsHumanReview != nil && *flat.NeedsHumanReview {
		result.HumanReviewNeeded = true
		if flat.HumanReviewReason != "" {
			result.HumanReviewReason = flat.HumanReviewReason
		}
	}
}

// applyInvestigationOutcome maps HAPI-style investigation_outcome values
// to is_actionable/needs_human_review/human_review_reason fields.
func applyInvestigationOutcome(result *katypes.InvestigationResult, outcome string) {
	switch outcome {
	case "problem_resolved", "predictive_no_action":
		falseVal := false
		result.IsActionable = &falseVal
	case "inconclusive":
		result.HumanReviewNeeded = true
		if result.HumanReviewReason == "" {
			result.HumanReviewReason = "investigation_inconclusive"
		}
	case "actionable":
		trueVal := true
		result.IsActionable = &trueVal
	}
}

// applyOutcomeRouting derives is_actionable from other fields when the LLM
// did not provide it explicitly. This mirrors HAPI's determine_investigation_outcome().
func applyOutcomeRouting(result *katypes.InvestigationResult) {
	if result.IsActionable != nil {
		return
	}
	if result.WorkflowID != "" {
		trueVal := true
		result.IsActionable = &trueVal
	}
}
