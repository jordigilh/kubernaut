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

	"github.com/go-logr/logr"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// applyLLMRCA copies the resolved RCA fields (whichever of snake_case /
// camelCase the LLM used) onto result, logging when the remediation target
// is missing its required api_version.
func applyLLMRCA(result *katypes.InvestigationResult, rca *llmRCA, logger logr.Logger) {
	if rca == nil {
		return
	}
	result.RCASummary = rca.Summary
	result.Severity = rca.Severity
	result.SignalName = rca.SignalName
	result.ContributingFactors = rca.ContributingFactors
	result.InvestigationAnalysis = rca.InvestigationAnalysis
	result.CausalChain = rca.CausalChain
	result.DueDiligence = rca.DueDiligence

	t := rca.resolvedTarget()
	if t == nil {
		return
	}
	if t.APIVersion == "" && t.Kind != "" {
		logger.Info("LLM omitted required api_version for remediation_target, resolution will use heuristic fallback",
			"kind", t.Kind, "name", t.Name, "namespace", t.Namespace)
	}
	result.RemediationTarget = katypes.RemediationTarget{
		Kind:       t.Kind,
		Name:       t.Name,
		Namespace:  t.Namespace,
		APIVersion: t.APIVersion,
	}
}

// applyLLMWorkflow copies the selected-workflow fields onto result, merging
// (rather than replacing) any parameters already present.
func applyLLMWorkflow(result *katypes.InvestigationResult, wf *llmWorkflow) {
	if wf == nil {
		return
	}
	result.WorkflowID = wf.WorkflowID
	result.ExecutionBundle = wf.ExecutionBundle
	result.Confidence = wf.Confidence
	result.Reason = wf.Rationale
	result.WorkflowRationale = wf.Rationale
	result.ExecutionEngine = wf.ExecutionEngine
	if wf.Parameters == nil {
		return
	}
	if result.Parameters == nil {
		result.Parameters = make(map[string]interface{})
	}
	for k, v := range wf.Parameters {
		result.Parameters[k] = v
	}
}

// parseLLMFormat parses the nested LLM response format and converts
// it to a flat InvestigationResult.
func parseLLMFormat(jsonStr string, logger logr.Logger) (*katypes.InvestigationResult, error) {
	fixed := unwrapDoubleSerializedJSON(jsonStr, logger)
	fixed = coerceKnownFields(fixed)

	var resp llmResponse
	if err := json.Unmarshal([]byte(fixed), &resp); err != nil {
		return nil, fmt.Errorf("parsing LLM JSON response: %w", err)
	}

	result := &katypes.InvestigationResult{}
	applyLLMRCA(result, resp.resolvedRCA(), logger)

	// Top-level severity takes precedence over nested (allows Mock LLM to set both)
	if resp.Severity != "" {
		result.Severity = resp.Severity
	}
	// Top-level confidence serves as fallback when selected_workflow is absent
	// (e.g., not-actionable outcomes where the LLM still provides a confidence score).
	if resp.Confidence > 0 {
		result.Confidence = resp.Confidence
	}
	applyLLMWorkflow(result, resp.Workflow)

	for _, alt := range resp.AlternativeWorkflows {
		result.AlternativeWorkflows = append(result.AlternativeWorkflows, katypes.AlternativeWorkflow{
			WorkflowID: alt.WorkflowID,
			Confidence: alt.Confidence,
			Rationale:  alt.Rationale,
			Parameters: alt.Parameters,
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
func parseSectionHeaders(content string, logger logr.Logger) (*katypes.InvestigationResult, error) {
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
		assembled[header] = sectionBodyToJSON(body)
	}

	if len(assembled) == 0 {
		return nil, fmt.Errorf("no parseable section content found")
	}

	compositeJSON, err := json.Marshal(assembled)
	if err != nil {
		return nil, fmt.Errorf("assembling section headers: %w", err)
	}

	return parseLLMFormat(string(compositeJSON), logger)
}

// sectionBodyToJSON converts a single trimmed, non-empty section body into
// its JSON representation: arrays and embedded JSON objects are preserved
// as-is, and bare scalars (numbers, "true"/"false"/"null", or unquoted
// strings like "critical") are coerced into valid JSON.
func sectionBodyToJSON(body string) json.RawMessage {
	// Arrays (e.g. alternative_workflows) must be preserved as-is.
	// extractJSON only finds objects, so it would strip the enclosing [].
	if body[0] == '[' {
		return json.RawMessage(body)
	}

	if jsonBody := extractJSON(body); jsonBody != "" {
		return json.RawMessage(jsonBody)
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
	return json.RawMessage(quotedOrRaw)
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
			Kind:       t.Kind,
			Name:       t.Name,
			Namespace:  t.Namespace,
			APIVersion: t.APIVersion,
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

// mergeNestedSelectedWorkflow extracts selected_workflow fields from a nested
// JSON object when the flat parse path succeeded but WorkflowID is empty.
// Handles hybrid JSON where the LLM returns flat rca_summary alongside a nested
// selected_workflow (#1431). Top-level workflow_id (already populated by
// json.Unmarshal) takes precedence.
func mergeNestedSelectedWorkflow(result *katypes.InvestigationResult, jsonStr string) {
	if result.WorkflowID != "" {
		return
	}
	var resp llmResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return
	}
	if resp.Workflow == nil {
		return
	}
	result.WorkflowID = resp.Workflow.WorkflowID
	result.ExecutionBundle = resp.Workflow.ExecutionBundle
	if resp.Workflow.Confidence > 0 {
		result.Confidence = resp.Workflow.Confidence
	}
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
		if flat.InvestigationOutcome != katypes.InvestigationOutcomeProblemResolved {
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
	if flat.InvestigationOutcome == katypes.InvestigationOutcomeProblemResolved && result.HumanReviewNeeded {
		result.HumanReviewNeeded = false
		result.HumanReviewReason = ""
	}
}
