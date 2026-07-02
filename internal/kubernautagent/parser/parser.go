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

	"github.com/go-logr/logr"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// ResultParser extracts and validates InvestigationResult from LLM JSON output.
type ResultParser struct {
	logger logr.Logger
}

// NewResultParser creates a new result parser. An optional logr.Logger enables
// debug-level diagnostics per DD-005 v2.0. If omitted, a discarding logger is
// used so callers are not forced to supply one.
func NewResultParser(logger ...logr.Logger) *ResultParser {
	l := logr.Discard()
	if len(logger) > 0 {
		l = logger[0]
	}
	return &ResultParser{logger: l}
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
			mergeNestedSelectedWorkflow(&result, coerced)
			applyOutcomeRouting(&result)
			return &result, nil
		}

		if parsed, err := parseLLMFormat(jsonStr, p.logger); err == nil {
			applyOutcomeRouting(parsed)
			return parsed, nil
		}
	}

	if parsed, err := parseSectionHeaders(content, p.logger); err == nil {
		applyOutcomeRouting(parsed)
		return parsed, nil
	}

	if jsonStr == "" {
		return nil, &ErrNoJSON{Content: content}
	}
	return nil, &ErrNoRecognizedFields{Raw: jsonStr}
}

const notActionableWarning = "Alert not actionable — no remediation warranted"
const problemResolvedWarning = "Problem self-resolved"
const confidenceFloor = 0.8

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
