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
	"log/slog"
	"strconv"
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
		return nil, &ErrEmptyContent{}
	}

	jsonStr := extractJSON(content)
	if jsonStr != "" {
		coerced := coerceKnownFields(jsonStr)
		var result katypes.InvestigationResult
		if err := json.Unmarshal([]byte(coerced), &result); err == nil && (result.RCASummary != "" || result.WorkflowID != "") {
			// BR-HAPI-200: Clear any HR fields populated by json.Unmarshal via
			// the "human_review_reason" tag match. HR is parser-derived only.
			result.HumanReviewNeeded = false
			result.HumanReviewReason = ""
			var flat flatLLMFields
			_ = json.Unmarshal([]byte(coerced), &flat)
			applyFlatFields(&result, flat)
			mergeNestedRemediationTarget(&result, coerced)
			mergeNestedInvestigationAnalysis(&result, coerced)
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
		return nil, &ErrNoJSON{Content: content}
	}
	return nil, &ErrNoRecognizedFields{Raw: jsonStr}
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
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return unwrapSingleElementArray(trimmed)
	}
	return extractBalancedJSON(content)
}

// unwrapSingleElementArray handles LLMs that wrap their response in a JSON
// array (e.g. `[{"rca_summary":...}]`). Single-element arrays are unwrapped
// to the inner object; multi-element arrays are rejected as ambiguous.
func unwrapSingleElementArray(s string) string {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return ""
	}
	if len(arr) != 1 {
		return ""
	}
	elem := strings.TrimSpace(string(arr[0]))
	if len(elem) > 0 && elem[0] == '{' {
		return elem
	}
	return ""
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
	RCA                  *llmRCA                `json:"root_cause_analysis"`
	RCAAlt               *llmRCA                `json:"rootCauseAnalysis,omitempty"`
	Workflow             *llmWorkflow            `json:"selected_workflow"`
	AlternativeWorkflows []llmAlternative        `json:"alternative_workflows,omitempty"`
	Severity             string                  `json:"severity,omitempty"`
	Confidence           float64                 `json:"confidence,omitempty"`
	Actionable           *bool                   `json:"actionable,omitempty"`
	InvestigationOutcome string                  `json:"investigation_outcome,omitempty"`
	DetectedLabels       map[string]interface{}  `json:"detected_labels,omitempty"`
}

// resolvedRCA returns the RCA from either snake_case or camelCase key.
// #746: LLMs sometimes use camelCase rootCauseAnalysis instead of snake_case.
func (r *llmResponse) resolvedRCA() *llmRCA {
	if r.RCA != nil {
		return r.RCA
	}
	return r.RCAAlt
}

type llmAlternative struct {
	WorkflowID string  `json:"workflow_id"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
}

type llmRCA struct {
	Summary               string        `json:"summary"`
	Severity              string        `json:"severity,omitempty"`
	SignalName            string        `json:"signal_name,omitempty"`
	ContributingFactors   []string      `json:"contributing_factors,omitempty"`
	RemediationTarget     *llmRemTarget `json:"remediation_target,omitempty"`
	RemediationTargetAlt  *llmRemTarget `json:"remediationTarget,omitempty"`
	InvestigationAnalysis string        `json:"investigation_analysis,omitempty"`
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

// unwrapDoubleSerializedJSON detects when an LLM has double-serialized a
// structured field (e.g. root_cause_analysis: "{\"summary\":...}" instead of
// root_cause_analysis: {"summary":...}) and returns corrected JSON. This
// defends against LLMs that json.Marshal their tool call arguments before
// embedding them (Issue #795).
func unwrapDoubleSerializedJSON(rawJSON string) string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return rawJSON
	}

	changed := false
	for _, key := range []string{"root_cause_analysis", "rootCauseAnalysis", "selected_workflow"} {
		val, ok := raw[key]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(val, &s) != nil {
			continue
		}
		s = strings.TrimSpace(s)
		if len(s) == 0 || s[0] != '{' {
			continue
		}
		if t := extractBalancedJSON(s); t != "" && json.Valid([]byte(t)) {
			if len(t) != len(s) {
				slog.Debug("unwrapDoubleSerializedJSON: stripped trailing content",
					slog.String("key", key),
					slog.Int("original_len", len(s)),
					slog.Int("extracted_len", len(t)))
			}
			raw[key] = json.RawMessage(t)
			changed = true
		}
	}

	if !changed {
		return rawJSON
	}
	fixed, err := json.Marshal(raw)
	if err != nil {
		return rawJSON
	}
	return string(fixed)
}

// coerceKnownFields fixes type drift from LLMs that return numeric or boolean
// fields as quoted strings (e.g. "confidence":"0.92" or "actionable":"false").
// It operates on raw JSON bytes, unquoting known fields before typed unmarshal.
func coerceKnownFields(rawJSON string) string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return rawJSON
	}

	changed := false
	for _, key := range []string{"confidence"} {
		val, ok := raw[key]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(val, &s) != nil {
			continue
		}
		s = strings.TrimSpace(s)
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			raw[key] = json.RawMessage(s)
		} else {
			delete(raw, key)
		}
		changed = true
	}
	for _, key := range []string{"actionable"} {
		val, ok := raw[key]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(val, &s) != nil {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "true" || s == "false" {
			raw[key] = json.RawMessage(s)
		} else {
			delete(raw, key)
		}
		changed = true
	}

	if nested, ok := raw["selected_workflow"]; ok {
		var wf map[string]json.RawMessage
		if json.Unmarshal(nested, &wf) == nil {
			wfChanged := false
			if confVal, ok := wf["confidence"]; ok {
				var s string
				if json.Unmarshal(confVal, &s) == nil {
					s = strings.TrimSpace(s)
					if _, err := strconv.ParseFloat(s, 64); err == nil {
						wf["confidence"] = json.RawMessage(s)
						wfChanged = true
					}
				}
			}
			if wfChanged {
				if fixed, err := json.Marshal(wf); err == nil {
					raw["selected_workflow"] = json.RawMessage(fixed)
					changed = true
				}
			}
		}
	}

	if !changed {
		return rawJSON
	}
	fixed, err := json.Marshal(raw)
	if err != nil {
		return rawJSON
	}
	return string(fixed)
}

// flatLLMFields captures top-level fields that may appear alongside the flat
// InvestigationResult format (rca_summary, workflow_id, confidence, etc.).
type flatLLMFields struct {
	Severity             string `json:"severity,omitempty"`
	Actionable           *bool  `json:"actionable,omitempty"`
	InvestigationOutcome string `json:"investigation_outcome,omitempty"`
}

// parseLLMFormat parses the nested LLM response format and converts
// it to a flat InvestigationResult.
func parseLLMFormat(jsonStr string) (*katypes.InvestigationResult, error) {
	fixed := unwrapDoubleSerializedJSON(jsonStr)
	fixed = coerceKnownFields(fixed)

	var resp llmResponse
	if err := json.Unmarshal([]byte(fixed), &resp); err != nil {
		return nil, fmt.Errorf("parsing LLM JSON response: %w", err)
	}

	result := &katypes.InvestigationResult{}
	rca := resp.resolvedRCA()
	if rca != nil {
		result.RCASummary = rca.Summary
		result.Severity = rca.Severity
		result.SignalName = rca.SignalName
		result.ContributingFactors = rca.ContributingFactors
		result.InvestigationAnalysis = rca.InvestigationAnalysis
		if t := rca.resolvedTarget(); t != nil {
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
	})

	if len(resp.DetectedLabels) > 0 {
		result.DetectedLabels = resp.DetectedLabels
	}

	// #746: Relax guard to accept responses where the LLM provided at least one
	// recognizable signal (confidence, RCA, or workflow). HAPI v1.2.1 has no such
	// guard — all parsed JSON flows through outcome routing. We require confidence > 0
	// as a minimum to reject truly garbage JSON (e.g., {"foo": "bar"}).
	hasContent := result.RCASummary != "" || result.WorkflowID != "" || resp.Confidence > 0
	if !hasContent {
		return nil, &ErrNoRecognizedFields{}
	}

	// #795 Fix D: Confidence alone without summary or workflow is a partial parse
	// caused by json.Unmarshal silently skipping type-mismatched fields or the inner
	// RCA object missing the required "summary" field. Reject so the investigator's
	// retryRCASubmit can request the missing data. See #795 preflight audit.
	if resp.Confidence > 0 && result.RCASummary == "" && result.WorkflowID == "" {
		return nil, &ErrNoRecognizedFields{}
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
		"root_cause_analysis":   true,
		"selected_workflow":     true,
		"alternative_workflows": true,
		"confidence":            true,
		"severity":              true,
		"actionable":            true,
		"investigation_outcome": true,
		"detected_labels":       true,
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
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return
	}
	rca := resp.resolvedRCA()
	if rca == nil {
		return
	}
	if t := rca.resolvedTarget(); t != nil {
		result.RemediationTarget = katypes.RemediationTarget{
			Kind:      t.Kind,
			Name:      t.Name,
			Namespace: t.Namespace,
		}
	}
}

// mergeNestedInvestigationAnalysis extracts investigation_analysis from a nested
// root_cause_analysis object when the flat parse path succeeded but the field
// is empty. Handles hybrid JSON where the LLM returns flat rca_summary alongside
// a nested RCA containing investigation_analysis (#724 F2).
func mergeNestedInvestigationAnalysis(result *katypes.InvestigationResult, jsonStr string) {
	if result.InvestigationAnalysis != "" {
		return
	}
	var resp llmResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return
	}
	rca := resp.resolvedRCA()
	if rca == nil {
		return
	}
	result.InvestigationAnalysis = rca.InvestigationAnalysis
}

// applyFlatFields applies LLM-provided flat fields to the result:
// - actionable: sets IsActionable, synthesizes warning, applies confidence floor
// - investigation_outcome: maps to outcome routing fields (HR is derived, not propagated)
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
		result.InvestigationOutcome = flat.InvestigationOutcome
		ApplyInvestigationOutcome(result, flat.InvestigationOutcome)
	}

	// #301: Contradiction override — when the outcome is problem_resolved but
	// the parser-derived HR was set (e.g., via inconclusive outcome fallback),
	// the resolution takes precedence. Defense-in-depth per HAPI parity.
	if flat.InvestigationOutcome == "problem_resolved" && result.HumanReviewNeeded {
		result.HumanReviewNeeded = false
		result.HumanReviewReason = ""
	}
}

// ApplyInvestigationOutcome maps KA-style investigation_outcome values
// to is_actionable/needs_human_review/human_review_reason fields.
// Exported for use by the investigator when merging Phase 1 fallbacks (#715).
// H5-fix: explicit `actionable` field takes precedence — only set IsActionable
// from outcome when the `actionable` field was absent.
func ApplyInvestigationOutcome(result *katypes.InvestigationResult, outcome string) {
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
			if result.RCASummary != "" && result.WorkflowID == "" {
				result.HumanReviewReason = "no_matching_workflows"
			} else {
				result.HumanReviewReason = "investigation_inconclusive"
			}
		}
	case "actionable":
		if result.IsActionable == nil {
			trueVal := true
			result.IsActionable = &trueVal
		}
	}
}

// applyOutcomeRouting derives is_actionable and human review fields from other
// fields when the LLM did not provide them explicitly. This mirrors HAPI
// v1.2.1's fallback chain (result_parser.py lines 483-510).
func applyOutcomeRouting(result *katypes.InvestigationResult) {
	if result.IsActionable != nil {
		return
	}
	if result.WorkflowID != "" {
		trueVal := true
		result.IsActionable = &trueVal
		return
	}
	// #746 / BR-HAPI-197.2: When no workflow is selected and no specific outcome
	// (inconclusive, problem_resolved, etc.) has already set HumanReviewNeeded,
	// derive no_matching_workflows. Matches HAPI v1.2.1:
	//   elif selected_workflow is None: needs_human_review = True; reason = "no_matching_workflows"
	if !result.HumanReviewNeeded {
		result.HumanReviewNeeded = true
		result.HumanReviewReason = "no_matching_workflows"
	}
}
