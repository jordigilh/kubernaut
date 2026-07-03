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
	"strconv"
	"strings"

	"github.com/go-logr/logr"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// llmResponse is the nested JSON structure that LLMs typically produce.
// Fields here must stay in sync with the JSON schema in schema.go and the
// structured output prompt template in incident_investigation.tmpl.
type llmResponse struct {
	RCA                  *llmRCA                `json:"root_cause_analysis"`
	RCAAlt               *llmRCA                `json:"rootCauseAnalysis,omitempty"`
	Workflow             *llmWorkflow           `json:"selected_workflow"`
	AlternativeWorkflows []llmAlternative       `json:"alternative_workflows,omitempty"`
	Severity             string                 `json:"severity,omitempty"`
	Confidence           float64                `json:"confidence,omitempty"`
	Actionable           *bool                  `json:"actionable,omitempty"`
	InvestigationOutcome string                 `json:"investigation_outcome,omitempty"`
	DetectedLabels       map[string]interface{} `json:"detected_labels,omitempty"`
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
	WorkflowID string                 `json:"workflow_id"`
	Confidence float64                `json:"confidence"`
	Rationale  string                 `json:"rationale"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type llmRCA struct {
	Summary               string                      `json:"summary"`
	Severity              string                      `json:"severity,omitempty"`
	SignalName            string                      `json:"signal_name,omitempty"`
	ContributingFactors   []string                    `json:"contributing_factors,omitempty"`
	RemediationTarget     *llmRemTarget               `json:"remediation_target,omitempty"`
	RemediationTargetAlt  *llmRemTarget               `json:"remediationTarget,omitempty"`
	InvestigationAnalysis string                      `json:"investigation_analysis,omitempty"`
	CausalChain           []string                    `json:"causal_chain,omitempty"`
	DueDiligence          *katypes.DueDiligenceReview `json:"due_diligence,omitempty"`
}

func (r *llmRCA) resolvedTarget() *llmRemTarget {
	if r.RemediationTarget != nil {
		return r.RemediationTarget
	}
	return r.RemediationTargetAlt
}

type llmRemTarget struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	APIVersion string `json:"api_version,omitempty"`
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
func unwrapDoubleSerializedJSON(rawJSON string, logger logr.Logger) string {
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
				logger.V(1).Info("unwrapDoubleSerializedJSON: stripped trailing content",
					"key", key,
					"original_len", len(s),
					"extracted_len", len(t))
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

	changed := coerceStringifiedNumberField(raw, "confidence")
	changed = coerceStringifiedBoolField(raw, "actionable") || changed
	changed = coerceNestedWorkflowConfidence(raw) || changed

	if !changed {
		return rawJSON
	}
	fixed, err := json.Marshal(raw)
	if err != nil {
		return rawJSON
	}
	return string(fixed)
}

// coerceStringifiedNumberField unquotes raw[key] in place when it holds a
// quoted numeric string (e.g. "0.92"), or deletes the key when the quoted
// value doesn't parse as a number. Reports whether raw was touched.
func coerceStringifiedNumberField(raw map[string]json.RawMessage, key string) bool {
	val, ok := raw[key]
	if !ok {
		return false
	}
	var s string
	if json.Unmarshal(val, &s) != nil {
		return false
	}
	s = strings.TrimSpace(s)
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		raw[key] = json.RawMessage(s)
	} else {
		delete(raw, key)
	}
	return true
}

// coerceStringifiedBoolField unquotes raw[key] in place when it holds a
// quoted "true"/"false" string, or deletes the key otherwise. Reports
// whether raw was touched.
func coerceStringifiedBoolField(raw map[string]json.RawMessage, key string) bool {
	val, ok := raw[key]
	if !ok {
		return false
	}
	var s string
	if json.Unmarshal(val, &s) != nil {
		return false
	}
	s = strings.TrimSpace(s)
	if s == "true" || s == "false" {
		raw[key] = json.RawMessage(s)
	} else {
		delete(raw, key)
	}
	return true
}

// coerceNestedWorkflowConfidence applies the same stringified-number fix to
// selected_workflow.confidence, re-marshaling the nested object back into
// raw when changed. Reports whether raw was touched.
func coerceNestedWorkflowConfidence(raw map[string]json.RawMessage) bool {
	nested, ok := raw["selected_workflow"]
	if !ok {
		return false
	}
	var wf map[string]json.RawMessage
	if json.Unmarshal(nested, &wf) != nil {
		return false
	}
	if !coerceStringifiedNumberFieldIfQuoted(wf, "confidence") {
		return false
	}
	fixed, err := json.Marshal(wf)
	if err != nil {
		return false
	}
	raw["selected_workflow"] = json.RawMessage(fixed)
	return true
}

// coerceStringifiedNumberFieldIfQuoted mirrors coerceStringifiedNumberField's
// unquote behavior but only reports a change when the field was present,
// quoted, and successfully parsed as a number — it leaves unparsable values
// in place rather than deleting them (matching the original nested-workflow
// coercion, which only ever touched confidence on a successful parse).
func coerceStringifiedNumberFieldIfQuoted(raw map[string]json.RawMessage, key string) bool {
	val, ok := raw[key]
	if !ok {
		return false
	}
	var s string
	if json.Unmarshal(val, &s) != nil {
		return false
	}
	s = strings.TrimSpace(s)
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return false
	}
	raw[key] = json.RawMessage(s)
	return true
}

// flatLLMFields captures top-level fields that may appear alongside the flat
// InvestigationResult format (rca_summary, workflow_id, confidence, etc.).
type flatLLMFields struct {
	Severity             string `json:"severity,omitempty"`
	Actionable           *bool  `json:"actionable,omitempty"`
	InvestigationOutcome string `json:"investigation_outcome,omitempty"`
}
