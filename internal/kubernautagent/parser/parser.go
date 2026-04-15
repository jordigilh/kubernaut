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
// Handles three formats in priority order:
//  1. Single JSON object (flat or nested) — structured output or well-behaved LLM
//  2. Section-header format ("# root_cause_analysis\n{...}\n# confidence\n0.95\n...")
//     produced when structured output is unavailable (e.g. Vertex AI)
//  3. JSON embedded in markdown code blocks or prose
func (p *ResultParser) Parse(content string) (*katypes.InvestigationResult, error) {
	if content == "" {
		return nil, fmt.Errorf("empty JSON content")
	}

	jsonStr := extractJSON(content)
	if jsonStr != "" {
		var result katypes.InvestigationResult
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil && (result.RCASummary != "" || result.WorkflowID != "") {
			var flat flatLLMFields
			_ = json.Unmarshal([]byte(jsonStr), &flat)
			applyFlatFields(&result, flat)
			mergeNestedRemediationTarget(&result, jsonStr)
			applyOutcomeRouting(&result)
			return &result, nil
		}

		if parsed, err := parseLLMFormat(jsonStr); err == nil {
			applyOutcomeRouting(parsed)
			return parsed, nil
		}
	}

	if parsed, err := parseSectionHeaders(content); err == nil {
		applyOutcomeRouting(parsed)
		return parsed, nil
	}

	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}
	return nil, fmt.Errorf("no recognized fields in LLM JSON response")
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
// mirroring KA's json_utils.py balanced extraction.
//
// M3-fix: skip `{` chars that are likely prose (not followed by `"`, `\n`, or
// another `{`). Real JSON objects start with `{"` or `{\n`.
func extractBalancedJSON(content string) string {
	pos := 0
	for pos < len(content) {
		start := strings.IndexByte(content[pos:], '{')
		if start == -1 {
			return ""
		}
		start += pos

		if start+1 < len(content) {
			next := content[start+1]
			if next != '"' && next != '\n' && next != '\r' && next != ' ' && next != '\t' && next != '{' && next != '}' {
				pos = start + 1
				continue
			}
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
		pos = start + 1
	}
	return ""
}

const notActionableWarning = "Alert not actionable — no remediation warranted"
const problemResolvedWarning = "Problem self-resolved"
const confidenceFloor = 0.8

// llmResponse is the nested JSON structure that LLMs typically produce.
// Fields here must stay in sync with the JSON schema in schema.go and the
// structured output prompt template in incident_investigation.tmpl.
type llmResponse struct {
	RCA                  *llmRCA           `json:"root_cause_analysis"`
	Workflow             *llmWorkflow      `json:"selected_workflow"`
	AlternativeWorkflows []llmAlternative  `json:"alternative_workflows,omitempty"`
	Severity             string            `json:"severity,omitempty"`
	Confidence           float64           `json:"confidence,omitempty"`
	Actionable           *bool             `json:"actionable,omitempty"`
	InvestigationOutcome string            `json:"investigation_outcome,omitempty"`
	NeedsHumanReview     *bool             `json:"needs_human_review,omitempty"`
	HumanReviewReason    string            `json:"human_review_reason,omitempty"`
	DetectedLabels       map[string]interface{} `json:"detected_labels,omitempty"`
}

type llmAlternative struct {
	WorkflowID string  `json:"workflow_id"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
}

type llmRCA struct {
	Summary              string        `json:"summary"`
	Severity             string        `json:"severity,omitempty"`
	SignalName           string        `json:"signal_name,omitempty"`
	ContributingFactors  []string      `json:"contributing_factors,omitempty"`
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
	WorkflowID      string                 `json:"workflow_id"`
	ExecutionBundle string                 `json:"execution_bundle,omitempty"`
	Confidence      float64                `json:"confidence"`
	Rationale       string                 `json:"rationale,omitempty"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
	ExecutionEngine string                 `json:"execution_engine,omitempty"`
}

// flatLLMFields captures top-level fields that may appear alongside the flat
// InvestigationResult format (rca_summary, workflow_id, confidence, etc.).
type flatLLMFields struct {
	Severity             string `json:"severity,omitempty"`
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
		result.Severity = resp.RCA.Severity
		result.SignalName = resp.RCA.SignalName
		result.ContributingFactors = resp.RCA.ContributingFactors
		if t := resp.RCA.resolvedTarget(); t != nil {
			result.RemediationTarget = katypes.RemediationTarget{
				Kind:      t.Kind,
				Name:      t.Name,
				Namespace: t.Namespace,
			}
		}
	}
	// Top-level severity takes precedence over nested (allows Mock LLM to set both)
	if resp.Severity != "" {
		result.Severity = resp.Severity
	}
	// Top-level confidence serves as fallback when selected_workflow is absent
	// (e.g., not-actionable outcomes where the LLM still provides a confidence score).
	if resp.Confidence > 0 {
		result.Confidence = resp.Confidence
	}
	if resp.Workflow != nil {
		result.WorkflowID = resp.Workflow.WorkflowID
		result.ExecutionBundle = resp.Workflow.ExecutionBundle
		result.Confidence = resp.Workflow.Confidence
		result.Reason = resp.Workflow.Rationale
		result.WorkflowRationale = resp.Workflow.Rationale
		result.ExecutionEngine = resp.Workflow.ExecutionEngine
		if resp.Workflow.Parameters != nil {
			if result.Parameters == nil {
				result.Parameters = make(map[string]interface{})
			}
			for k, v := range resp.Workflow.Parameters {
				result.Parameters[k] = v
			}
		}
	}

	for _, alt := range resp.AlternativeWorkflows {
		result.AlternativeWorkflows = append(result.AlternativeWorkflows, katypes.AlternativeWorkflow{
			WorkflowID: alt.WorkflowID,
			Confidence: alt.Confidence,
			Rationale:  alt.Rationale,
		})
	}

	applyFlatFields(result, flatLLMFields{
		Severity:             resp.Severity,
		Actionable:           resp.Actionable,
		InvestigationOutcome: resp.InvestigationOutcome,
		NeedsHumanReview:     resp.NeedsHumanReview,
		HumanReviewReason:    resp.HumanReviewReason,
	})

	if len(resp.DetectedLabels) > 0 {
		result.DetectedLabels = resp.DetectedLabels
	}

	if result.RCASummary == "" && result.WorkflowID == "" {
		return nil, fmt.Errorf("no recognized fields in LLM JSON response")
	}

	return result, nil
}

// parseSectionHeaders handles the "# header\nvalue" format produced by LLMs when
// structured output (output_config) is unavailable (e.g. Vertex AI).
// The prompt instructs:
//
//	# root_cause_analysis
//	{"summary": "...", ...}
//	# confidence
//	0.95
//	# selected_workflow
//	{"workflow_id": "...", ...}
//	# alternative_workflows
//	[{"workflow_id": "...", ...}]
//
// This function extracts each section's content, assembles an llmResponse,
// and maps it to InvestigationResult via parseLLMFormat.
func parseSectionHeaders(content string) (*katypes.InvestigationResult, error) {
	sections := extractSections(content)
	if len(sections) == 0 {
		return nil, fmt.Errorf("no section headers found")
	}

	assembled := make(map[string]json.RawMessage)

	for header, body := range sections {
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}

		// Arrays (e.g. alternative_workflows) must be preserved as-is.
		// extractJSON only finds objects, so it would strip the enclosing [].
		if body[0] == '[' {
			assembled[header] = json.RawMessage(body)
			continue
		}

		jsonBody := extractJSON(body)
		if jsonBody != "" {
			assembled[header] = json.RawMessage(jsonBody)
			continue
		}
		// Scalar values: numbers ("0.95"), booleans ("true"/"false"), or strings ("critical")
		quotedOrRaw := body
		if quotedOrRaw != "true" && quotedOrRaw != "false" && quotedOrRaw != "null" {
			if len(quotedOrRaw) > 0 && quotedOrRaw[0] != '"' {
				if _, err := json.Number(quotedOrRaw).Float64(); err != nil {
					quotedOrRaw = `"` + strings.ReplaceAll(quotedOrRaw, `"`, `\"`) + `"`
				}
			}
		}
		assembled[header] = json.RawMessage(quotedOrRaw)
	}

	if len(assembled) == 0 {
		return nil, fmt.Errorf("no parseable section content found")
	}

	compositeJSON, err := json.Marshal(assembled)
	if err != nil {
		return nil, fmt.Errorf("assembling section headers: %w", err)
	}

	return parseLLMFormat(string(compositeJSON))
}

// extractSections splits content on lines matching "# <header_name>" and returns
// a map of header → body text. Recognizes headers with and without markdown fencing.
func extractSections(content string) map[string]string {
	knownHeaders := map[string]bool{
		"root_cause_analysis": true,
		"selected_workflow":   true,
		"alternative_workflows": true,
		"confidence":          true,
		"severity":            true,
		"actionable":          true,
		"investigation_outcome": true,
		"needs_human_review":  true,
		"human_review_reason": true,
		"detected_labels":     true,
	}

	sections := make(map[string]string)
	lines := strings.Split(content, "\n")
	var currentHeader string
	var currentBody strings.Builder

	flush := func() {
		if currentHeader != "" {
			sections[currentHeader] = currentBody.String()
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			candidate := strings.TrimSpace(trimmed[2:])
			if knownHeaders[candidate] {
				flush()
				currentHeader = candidate
				currentBody.Reset()
				continue
			}
		}
		if currentHeader != "" {
			currentBody.WriteString(line)
			currentBody.WriteString("\n")
		}
	}
	flush()

	return sections
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
	if flat.Severity != "" && result.Severity == "" {
		result.Severity = flat.Severity
	}

	if flat.Actionable != nil && !*flat.Actionable {
		falseVal := false
		result.IsActionable = &falseVal
		// When investigation_outcome=problem_resolved, the outcome handler synthesizes
		// its own warning ("Problem self-resolved"). Adding the generic "not actionable"
		// warning would cause AA's response processor to route to NotActionable (Outcome D)
		// instead of ProblemResolved (Outcome A). KA Python had the same precedence:
		// the resolved outcome is authoritative over the actionable flag.
		if flat.InvestigationOutcome != "problem_resolved" {
			result.Warnings = append(result.Warnings, notActionableWarning)
		}
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

	// #301: Contradiction override — when the LLM says the problem is resolved
	// but also sets needs_human_review=true, the resolution takes precedence.
	// Python KA enforced this; KA must match for AA parity.
	if flat.InvestigationOutcome == "problem_resolved" && result.HumanReviewNeeded {
		result.HumanReviewNeeded = false
		result.HumanReviewReason = ""
	}
}

// applyInvestigationOutcome maps KA-style investigation_outcome values
// to is_actionable/needs_human_review/human_review_reason fields.
// H5-fix: explicit `actionable` field takes precedence — only set IsActionable
// from outcome when the `actionable` field was absent.
func applyInvestigationOutcome(result *katypes.InvestigationResult, outcome string) {
	switch outcome {
	case "problem_resolved":
		if result.IsActionable == nil {
			falseVal := false
			result.IsActionable = &falseVal
		}
		warning := problemResolvedWarning
		if result.RCASummary != "" {
			warning += ": " + result.RCASummary
		}
		result.Warnings = append(result.Warnings, warning)
	case "predictive_no_action":
		if result.IsActionable == nil {
			falseVal := false
			result.IsActionable = &falseVal
		}
	case "inconclusive":
		result.HumanReviewNeeded = true
		if result.HumanReviewReason == "" {
			result.HumanReviewReason = "investigation_inconclusive"
		}
	case "actionable":
		if result.IsActionable == nil {
			trueVal := true
			result.IsActionable = &trueVal
		}
	}
}

// applyOutcomeRouting derives is_actionable from other fields when the LLM
// did not provide it explicitly. This mirrors KA's determine_investigation_outcome().
func applyOutcomeRouting(result *katypes.InvestigationResult) {
	if result.IsActionable != nil {
		return
	}
	if result.WorkflowID != "" {
		trueVal := true
		result.IsActionable = &trueVal
	}
}
