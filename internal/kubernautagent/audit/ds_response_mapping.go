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

package audit

import (
	"encoding/json"
	"time"

	"github.com/go-faster/jx"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

func dataString(d map[string]interface{}, key string) string {
	if v, ok := d[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func dataInt(d map[string]interface{}, key string) int {
	if v, ok := d[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func dataBool(d map[string]interface{}, key string) bool {
	if v, ok := d[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func dataFloat64(d map[string]interface{}, key string) float64 {
	if v, ok := d[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		}
	}
	return 0
}

func dataStringSlice(d map[string]interface{}, key string) []string {
	if v, ok := d[key]; ok {
		if ss, ok := v.([]string); ok {
			return ss
		}
	}
	return nil
}

const previewMaxLen = 500

// maxReasoningPayloadLen bounds reasoning text stored in Data-Storage audit
// payloads (BR-AI-086 AC6). Mirrors the investigator package's
// maxReasoningAuditChars: a defense-in-depth cap applied again at this
// consuming boundary regardless of whether the producer already truncated.
const maxReasoningPayloadLen = 40000

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

// reasoningPayloadFields renders reasoning text/redacted into the OptString/
// OptBool pair shared by LLMResponsePayload and
// IncidentResponseDataRootCauseAnalysis, applying the audit size guard.
func reasoningPayloadFields(text string, redacted bool) (ogenclient.OptString, ogenclient.OptBool) {
	var t ogenclient.OptString
	t.SetTo(truncate(text, maxReasoningPayloadLen))
	var r ogenclient.OptBool
	r.SetTo(redacted)
	return t, r
}

func toJxRaw(s string) jx.Raw {
	if s != "null" && json.Valid([]byte(s)) {
		return jx.Raw(s)
	}
	b, err := json.Marshal(s)
	if err != nil {
		return jx.Raw(`""`)
	}
	return jx.Raw(b)
}

func toJxRawMap(s string) ogenclient.LLMToolCallPayloadToolArguments {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil
	}
	result := make(ogenclient.LLMToolCallPayloadToolArguments, len(raw))
	for k, v := range raw {
		result[k] = jx.Raw(v)
	}
	return result
}

type investigationResultJSON struct {
	RCASummary           string                 `json:"rca_summary"`
	Severity             string                 `json:"severity"`
	ContributingFactors  []string               `json:"contributing_factors"`
	WorkflowID           string                 `json:"workflow_id"`
	ExecutionBundle      string                 `json:"execution_bundle"`
	Confidence           float64                `json:"confidence"`
	NeedsHumanReview     bool                   `json:"needs_human_review"`
	HumanReviewReason    string                 `json:"human_review_reason"`
	Warnings             []string               `json:"warnings"`
	Parameters           map[string]interface{} `json:"parameters"`
	AlternativeWorkflows []altWorkflowJSON      `json:"alternative_workflows"`
	RemediationTarget    *remediationTargetJSON `json:"remediation_target"`
	CausalChain          []string               `json:"causal_chain"`
	DueDiligence         *dueDiligenceJSON      `json:"due_diligence"`
	Reasoning            *reasoningJSON         `json:"reasoning"`
}

// reasoningJSON mirrors katypes.ReasoningSummary (BR-AI-086 AC6): visible
// text + a redacted flag only, never an opaque replay signature.
type reasoningJSON struct {
	Text     string `json:"text"`
	Redacted bool   `json:"redacted"`
}

type dueDiligenceJSON struct {
	CausalCompleteness    string `json:"causal_completeness"`
	TargetAccuracy        string `json:"target_accuracy"`
	EvidenceSufficiency   string `json:"evidence_sufficiency"`
	AlternativeHypotheses string `json:"alternative_hypotheses"`
	ScopeCompleteness     string `json:"scope_completeness"`
	Proportionality       string `json:"proportionality"`
	RegressionAwareness   string `json:"regression_awareness"`
	ConfidenceCalibration string `json:"confidence_calibration"`
}

type altWorkflowJSON struct {
	WorkflowID      string  `json:"workflow_id"`
	Rationale       string  `json:"rationale"`
	ExecutionBundle string  `json:"execution_bundle"`
	Confidence      float64 `json:"confidence"`
}

type remediationTargetJSON struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func toIncidentResponseData(responseDataJSON string, incidentID string) ogenclient.IncidentResponseData {
	var ir investigationResultJSON
	if err := json.Unmarshal([]byte(responseDataJSON), &ir); err != nil {
		return ogenclient.IncidentResponseData{
			IncidentId: incidentID,
			Timestamp:  time.Now().UTC(),
		}
	}

	data := ogenclient.IncidentResponseData{
		IncidentId: incidentID,
		Analysis:   ir.RCASummary,
		Confidence: float32(ir.Confidence),
		Timestamp:  time.Now().UTC(),
		RootCauseAnalysis: ogenclient.IncidentResponseDataRootCauseAnalysis{
			Summary:             ir.RCASummary,
			Severity:            mapSeverity(ir.Severity),
			ContributingFactors: ir.ContributingFactors,
		},
	}

	if ir.RemediationTarget != nil {
		data.RootCauseAnalysis.RemediationTarget.SetTo(toRemediationTargetResponse(ir.RemediationTarget))
	}

	if len(ir.CausalChain) > 0 {
		data.RootCauseAnalysis.CausalChain = ir.CausalChain
	}
	if ir.DueDiligence != nil {
		data.RootCauseAnalysis.DueDiligence.SetTo(toDueDiligenceResponse(ir.DueDiligence))
	}
	if ir.Reasoning != nil {
		data.RootCauseAnalysis.Reasoning, data.RootCauseAnalysis.ReasoningRedacted = reasoningPayloadFields(
			ir.Reasoning.Text, ir.Reasoning.Redacted)
	}

	if ir.WorkflowID != "" {
		data.SelectedWorkflow.SetTo(toSelectedWorkflowResponse(ir))
	}

	if ir.NeedsHumanReview {
		data.NeedsHumanReview.SetTo(true)
	}
	if ir.HumanReviewReason != "" {
		if reason, ok := validHumanReviewReason(ir.HumanReviewReason); ok {
			data.HumanReviewReason.SetTo(reason)
		}
	}
	if len(ir.Warnings) > 0 {
		data.Warnings = ir.Warnings
	} else {
		data.Warnings = []string{}
	}

	if len(ir.AlternativeWorkflows) > 0 {
		data.AlternativeWorkflows = toAlternativeWorkflowsResponse(ir.AlternativeWorkflows)
	}

	return data
}

// toRemediationTargetResponse maps the audit-record RemediationTarget into
// the ogen wire format, omitting any field that was never populated.
func toRemediationTargetResponse(rt *remediationTargetJSON) ogenclient.IncidentResponseDataRootCauseAnalysisRemediationTarget {
	var out ogenclient.IncidentResponseDataRootCauseAnalysisRemediationTarget
	if rt.Kind != "" {
		out.Kind.SetTo(rt.Kind)
	}
	if rt.Name != "" {
		out.Name.SetTo(rt.Name)
	}
	if rt.Namespace != "" {
		out.Namespace.SetTo(rt.Namespace)
	}
	return out
}

// toDueDiligenceResponse maps the audit-record DueDiligence review into the
// ogen wire format, omitting any field that was never populated.
func toDueDiligenceResponse(dd *dueDiligenceJSON) ogenclient.IncidentResponseDataRootCauseAnalysisDueDiligence {
	out := ogenclient.IncidentResponseDataRootCauseAnalysisDueDiligence{}
	if dd.CausalCompleteness != "" {
		out.CausalCompleteness.SetTo(dd.CausalCompleteness)
	}
	if dd.TargetAccuracy != "" {
		out.TargetAccuracy.SetTo(dd.TargetAccuracy)
	}
	if dd.EvidenceSufficiency != "" {
		out.EvidenceSufficiency.SetTo(dd.EvidenceSufficiency)
	}
	if dd.AlternativeHypotheses != "" {
		out.AlternativeHypotheses.SetTo(dd.AlternativeHypotheses)
	}
	if dd.ScopeCompleteness != "" {
		out.ScopeCompleteness.SetTo(dd.ScopeCompleteness)
	}
	if dd.Proportionality != "" {
		out.Proportionality.SetTo(dd.Proportionality)
	}
	if dd.RegressionAwareness != "" {
		out.RegressionAwareness.SetTo(dd.RegressionAwareness)
	}
	if dd.ConfidenceCalibration != "" {
		out.ConfidenceCalibration.SetTo(dd.ConfidenceCalibration)
	}
	return out
}

// toSelectedWorkflowResponse maps the selected-workflow fields of ir into the
// ogen wire format. Callers must check ir.WorkflowID != "".
func toSelectedWorkflowResponse(ir investigationResultJSON) ogenclient.IncidentResponseDataSelectedWorkflow {
	sw := ogenclient.IncidentResponseDataSelectedWorkflow{}
	sw.WorkflowId.SetTo(ir.WorkflowID)
	if ir.ExecutionBundle != "" {
		sw.ExecutionBundle.SetTo(ir.ExecutionBundle)
	}
	sw.Confidence.SetTo(float32(ir.Confidence))
	if len(ir.Parameters) > 0 {
		params := make(ogenclient.IncidentResponseDataSelectedWorkflowParameters, len(ir.Parameters))
		for k, v := range ir.Parameters {
			b, err := json.Marshal(v)
			if err != nil {
				continue
			}
			params[k] = jx.Raw(b)
		}
		sw.Parameters.SetTo(params)
	}
	return sw
}

// toAlternativeWorkflowsResponse maps the audit-record AlternativeWorkflows
// list into the ogen wire format.
func toAlternativeWorkflowsResponse(alts []altWorkflowJSON) []ogenclient.IncidentResponseDataAlternativeWorkflowsItem {
	out := make([]ogenclient.IncidentResponseDataAlternativeWorkflowsItem, 0, len(alts))
	for _, alt := range alts {
		item := ogenclient.IncidentResponseDataAlternativeWorkflowsItem{}
		if alt.WorkflowID != "" {
			item.WorkflowId.SetTo(alt.WorkflowID)
		}
		if alt.Rationale != "" {
			item.Rationale.SetTo(alt.Rationale)
		}
		if alt.ExecutionBundle != "" {
			item.ExecutionBundle.SetTo(alt.ExecutionBundle)
		}
		if alt.Confidence > 0 {
			item.Confidence.SetTo(float32(alt.Confidence))
		}
		out = append(out, item)
	}
	return out
}

func mapSeverity(s string) ogenclient.IncidentResponseDataRootCauseAnalysisSeverity {
	switch s {
	case "critical":
		return ogenclient.IncidentResponseDataRootCauseAnalysisSeverityCritical
	case "high":
		return ogenclient.IncidentResponseDataRootCauseAnalysisSeverityHigh
	case "warning":
		return ogenclient.IncidentResponseDataRootCauseAnalysisSeverityWarning
	case "info":
		return ogenclient.IncidentResponseDataRootCauseAnalysisSeverityInfo
	default:
		return ogenclient.IncidentResponseDataRootCauseAnalysisSeverityUnknown
	}
}

var validHumanReviewReasons = map[string]ogenclient.IncidentResponseDataHumanReviewReason{
	"workflow_not_found":          ogenclient.IncidentResponseDataHumanReviewReasonWorkflowNotFound,
	"image_mismatch":              ogenclient.IncidentResponseDataHumanReviewReasonImageMismatch,
	"parameter_validation_failed": ogenclient.IncidentResponseDataHumanReviewReasonParameterValidationFailed,
	"no_matching_workflows":       ogenclient.IncidentResponseDataHumanReviewReasonNoMatchingWorkflows,
	"low_confidence":              ogenclient.IncidentResponseDataHumanReviewReasonLowConfidence,
	"llm_parsing_error":           ogenclient.IncidentResponseDataHumanReviewReasonLlmParsingError,
	"investigation_inconclusive":  ogenclient.IncidentResponseDataHumanReviewReasonInvestigationInconclusive,
	"rca_incomplete":              ogenclient.IncidentResponseDataHumanReviewReasonRcaIncomplete,
	"alignment_check_failed":      ogenclient.IncidentResponseDataHumanReviewReasonAlignmentCheckFailed,
}

// validHumanReviewReason returns the ogen enum value if the string is a recognised
// HumanReviewReason. Free-text values from the LLM are rejected so that the
// ogen client-side OpenAPI validation does not drop the entire audit event.
func validHumanReviewReason(s string) (ogenclient.IncidentResponseDataHumanReviewReason, bool) {
	v, ok := validHumanReviewReasons[s]
	return v, ok
}

var _ AuditStore = (*DSAuditStore)(nil)
