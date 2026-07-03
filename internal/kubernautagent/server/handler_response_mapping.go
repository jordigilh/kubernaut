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

package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-faster/jx"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/agentclient"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

func mapInvestigationResultToResponse(log logr.Logger, r *katypes.InvestigationResult, incidentID string) *agentclient.IncidentResponse {
	resp := &agentclient.IncidentResponse{
		IncidentID:        incidentID,
		Analysis:          r.RCASummary,
		Confidence:        r.Confidence,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		RootCauseAnalysis: buildRootCauseAnalysis(r),
	}

	applyHumanReviewFields(resp, log, r)

	if r.WorkflowID != "" {
		resp.SelectedWorkflow.SetTo(buildSelectedWorkflow(r))
	}

	if r.IsActionable != nil {
		resp.IsActionable.SetTo(*r.IsActionable)
	}
	resp.Warnings = buildResponseWarnings(r)

	if len(r.DetectedLabels) > 0 {
		resp.DetectedLabels.SetTo(buildDetectedLabelsResponse(r))
	}

	if len(r.AlternativeWorkflows) > 0 {
		resp.AlternativeWorkflows = buildAlternativeWorkflowsResponse(r)
	}

	if len(r.ValidationAttemptsHistory) > 0 {
		resp.ValidationAttemptsHistory = buildValidationAttemptsResponse(r)
	}

	if r.AlignmentVerdict != nil {
		resp.AlignmentVerdict.SetTo(buildAlignmentVerdictResponse(r))
	}

	return resp
}

// buildRootCauseAnalysis maps the RCA-related InvestigationResult fields into
// the wire-format RootCauseAnalysis map, omitting any field that was never
// populated by the investigator.
func buildRootCauseAnalysis(r *katypes.InvestigationResult) agentclient.IncidentResponseRootCauseAnalysis {
	rca := agentclient.IncidentResponseRootCauseAnalysis{}
	if r.RCASummary != "" {
		summaryRaw, _ := json.Marshal(r.RCASummary)
		rca["summary"] = jx.Raw(summaryRaw)
	}
	if r.Severity != "" {
		sevRaw, _ := json.Marshal(r.Severity)
		rca["severity"] = jx.Raw(sevRaw)
	}
	if r.RemediationTarget.Kind != "" {
		targetRaw, _ := json.Marshal(r.RemediationTarget)
		rca["remediationTarget"] = jx.Raw(targetRaw)
	}
	if r.SignalName != "" {
		snRaw, _ := json.Marshal(r.SignalName)
		rca["signal_name"] = jx.Raw(snRaw)
	}
	if len(r.ContributingFactors) > 0 {
		cfRaw, _ := json.Marshal(r.ContributingFactors)
		rca["contributing_factors"] = jx.Raw(cfRaw)
	}
	if len(r.CausalChain) > 0 {
		ccRaw, _ := json.Marshal(r.CausalChain)
		rca["causal_chain"] = jx.Raw(ccRaw)
	}
	if r.DueDiligence != nil {
		ddRaw, _ := json.Marshal(r.DueDiligence)
		rca["due_diligence"] = jx.Raw(ddRaw)
	}
	return rca
}

// applyHumanReviewFields sets resp.NeedsHumanReview and, when review is
// needed, maps the (possibly LLM-provided, possibly unrecognized) reason
// string to the response's HumanReviewReason enum.
func applyHumanReviewFields(resp *agentclient.IncidentResponse, log logr.Logger, r *katypes.InvestigationResult) {
	if !r.HumanReviewNeeded {
		resp.NeedsHumanReview.SetTo(false)
		return
	}
	resp.NeedsHumanReview.SetTo(true)
	reason := r.HumanReviewReason
	if reason == "" {
		reason = r.Reason
	}
	mapped, isDefault := mapHumanReviewReason(reason)
	if isDefault && reason != "" {
		log.Info("unrecognized human review reason, falling back to investigation_inconclusive",
			"original_reason", reason)
	}
	resp.HumanReviewReason.SetTo(mapped)
}

// buildSelectedWorkflow maps the selected-workflow fields of r into the
// wire-format SelectedWorkflow map. Callers must check r.WorkflowID != "".
func buildSelectedWorkflow(r *katypes.InvestigationResult) agentclient.IncidentResponseSelectedWorkflow {
	sw := agentclient.IncidentResponseSelectedWorkflow{}
	wfIDRaw, _ := json.Marshal(r.WorkflowID)
	sw["workflow_id"] = jx.Raw(wfIDRaw)
	if len(r.Parameters) > 0 {
		paramsRaw, _ := json.Marshal(r.Parameters)
		sw["parameters"] = jx.Raw(paramsRaw)
	}
	confRaw, _ := json.Marshal(r.Confidence)
	sw["confidence"] = jx.Raw(confRaw)
	if r.ExecutionBundle != "" {
		ebRaw, _ := json.Marshal(r.ExecutionBundle)
		sw["execution_bundle"] = jx.Raw(ebRaw)
	}
	if r.ExecutionBundleDigest != "" {
		ebdRaw, _ := json.Marshal(r.ExecutionBundleDigest)
		sw["execution_bundle_digest"] = jx.Raw(ebdRaw)
	}
	if r.ExecutionEngine != "" {
		eeRaw, _ := json.Marshal(r.ExecutionEngine)
		sw["execution_engine"] = jx.Raw(eeRaw)
	}
	if r.ServiceAccountName != "" {
		saRaw, _ := json.Marshal(r.ServiceAccountName)
		sw["service_account_name"] = jx.Raw(saRaw)
	}
	if r.WorkflowVersion != "" {
		vRaw, _ := json.Marshal(r.WorkflowVersion)
		sw["version"] = jx.Raw(vRaw)
	}
	if r.WorkflowRationale != "" {
		rRaw, _ := json.Marshal(r.WorkflowRationale)
		sw["rationale"] = jx.Raw(rRaw)
	}
	return sw
}

// buildResponseWarnings returns r.Warnings verbatim when set, a synthesized
// human-review warning (BR-HAPI-197) when review is needed but no warning
// was set, or an empty (non-nil) slice otherwise.
func buildResponseWarnings(r *katypes.InvestigationResult) []string {
	if len(r.Warnings) > 0 {
		return r.Warnings
	}
	if r.HumanReviewNeeded {
		return []string{synthesizeHumanReviewWarning(r)}
	}
	return []string{}
}

// buildDetectedLabelsResponse maps r.DetectedLabels into the wire format.
// Callers must check len(r.DetectedLabels) > 0.
func buildDetectedLabelsResponse(r *katypes.InvestigationResult) agentclient.IncidentResponseDetectedLabels {
	dl := make(agentclient.IncidentResponseDetectedLabels, len(r.DetectedLabels))
	for k, v := range r.DetectedLabels {
		raw, _ := json.Marshal(v)
		dl[k] = jx.Raw(raw)
	}
	return dl
}

// buildAlternativeWorkflowsResponse maps r.AlternativeWorkflows into the wire
// format. Callers must check len(r.AlternativeWorkflows) > 0.
func buildAlternativeWorkflowsResponse(r *katypes.InvestigationResult) []agentclient.AlternativeWorkflow {
	alts := make([]agentclient.AlternativeWorkflow, 0, len(r.AlternativeWorkflows))
	for _, aw := range r.AlternativeWorkflows {
		alt := agentclient.AlternativeWorkflow{
			WorkflowID: aw.WorkflowID,
			Confidence: aw.Confidence,
			Rationale:  aw.Rationale,
		}
		if aw.ExecutionBundle != "" {
			alt.ExecutionBundle.SetTo(aw.ExecutionBundle)
		}
		alts = append(alts, alt)
	}
	return alts
}

// buildValidationAttemptsResponse maps r.ValidationAttemptsHistory into the
// wire format. Callers must check len(r.ValidationAttemptsHistory) > 0.
func buildValidationAttemptsResponse(r *katypes.InvestigationResult) []agentclient.ValidationAttempt {
	attempts := make([]agentclient.ValidationAttempt, 0, len(r.ValidationAttemptsHistory))
	for _, va := range r.ValidationAttemptsHistory {
		attempt := agentclient.ValidationAttempt{
			Attempt:   va.Attempt,
			IsValid:   va.IsValid,
			Errors:    va.Errors,
			Timestamp: va.Timestamp,
		}
		if va.WorkflowID != "" {
			attempt.WorkflowID.SetTo(va.WorkflowID)
		}
		attempts = append(attempts, attempt)
	}
	return attempts
}

// buildAlignmentVerdictResponse maps r.AlignmentVerdict into the wire format.
// Callers must check r.AlignmentVerdict != nil.
func buildAlignmentVerdictResponse(r *katypes.InvestigationResult) agentclient.AlignmentVerdict {
	av := agentclient.AlignmentVerdict{
		Result:  agentclient.AlignmentVerdictResult(r.AlignmentVerdict.Result),
		Flagged: r.AlignmentVerdict.Flagged,
		Total:   r.AlignmentVerdict.Total,
	}
	if r.AlignmentVerdict.CircuitBreakerActivated {
		av.CircuitBreakerActivated.SetTo(true)
	}
	if r.AlignmentVerdict.Summary != "" {
		av.Summary.SetTo(r.AlignmentVerdict.Summary)
	}
	if len(r.AlignmentVerdict.Findings) > 0 {
		findings := make([]agentclient.AlignmentFinding, 0, len(r.AlignmentVerdict.Findings))
		for _, f := range r.AlignmentVerdict.Findings {
			finding := agentclient.AlignmentFinding{
				StepIndex:   f.StepIndex,
				StepKind:    agentclient.AlignmentFindingStepKind(f.StepKind),
				Explanation: f.Explanation,
			}
			if f.Tool != "" {
				finding.Tool.SetTo(f.Tool)
			}
			findings = append(findings, finding)
		}
		av.Findings = findings
	}
	return av
}

// synthesizeHumanReviewWarning generates a warning when human review is required
// but no explicit warnings were set by the parser. Per BR-HAPI-197, human review
// responses must include at least one warning explaining why automation is unavailable.
func synthesizeHumanReviewWarning(r *katypes.InvestigationResult) string {
	reason := r.HumanReviewReason
	if reason == "" {
		reason = r.Reason
	}
	if reason != "" {
		return fmt.Sprintf("Human review required: %s", reason)
	}
	return "Human review required: investigation could not determine automated remediation"
}

// mapHumanReviewReason maps free-form investigator reason strings to valid
// HumanReviewReason enum values. Returns the mapped enum and whether the
// default fallback was used (M4: enables caller logging of unrecognized reasons).
// exactHumanReviewReasons maps the canonical (already-normalized)
// HumanReviewReason string values emitted by the investigator to their
// OpenAPI enum. Kept as a package-level map (rather than a switch) so
// mapHumanReviewReason stays a simple two-step lookup.
var exactHumanReviewReasons = map[string]agentclient.HumanReviewReason{
	"rca_incomplete":              agentclient.HumanReviewReasonRcaIncomplete,
	"investigation_inconclusive":  agentclient.HumanReviewReasonInvestigationInconclusive,
	"workflow_not_found":          agentclient.HumanReviewReasonWorkflowNotFound,
	"no_matching_workflows":       agentclient.HumanReviewReasonNoMatchingWorkflows,
	"image_mismatch":              agentclient.HumanReviewReasonImageMismatch,
	"parameter_validation_failed": agentclient.HumanReviewReasonParameterValidationFailed,
	"low_confidence":              agentclient.HumanReviewReasonLowConfidence,
	"llm_parsing_error":           agentclient.HumanReviewReasonLlmParsingError,
	"alignment_check_failed":      agentclient.HumanReviewReasonAlignmentCheckFailed,
	"operator_escalation":         agentclient.HumanReviewReasonOperatorEscalation,
}

func mapHumanReviewReason(reason string) (agentclient.HumanReviewReason, bool) {
	if mapped, ok := exactHumanReviewReasons[reason]; ok {
		return mapped, false
	}
	return mapHumanReviewReasonHeuristic(reason)
}

// mapHumanReviewReasonHeuristic falls back to substring matching for
// free-form reason strings (e.g. raw error messages) that don't match one of
// the canonical exactHumanReviewReasons values.
func mapHumanReviewReasonHeuristic(reason string) (agentclient.HumanReviewReason, bool) {
	switch {
	case strings.Contains(reason, "exhausted during RCA"):
		return agentclient.HumanReviewReasonRcaIncomplete, false
	case strings.Contains(reason, "exhausted during workflow selection"):
		return agentclient.HumanReviewReasonInvestigationInconclusive, false
	case strings.Contains(reason, "not found") && strings.Contains(reason, "catalog"):
		return agentclient.HumanReviewReasonWorkflowNotFound, false
	case strings.Contains(reason, "no matching"):
		return agentclient.HumanReviewReasonNoMatchingWorkflows, false
	case strings.Contains(reason, "mismatch") || strings.Contains(reason, "image"):
		return agentclient.HumanReviewReasonImageMismatch, false
	case strings.Contains(reason, "parameter") || strings.Contains(reason, "validation"):
		return agentclient.HumanReviewReasonParameterValidationFailed, false
	case strings.Contains(reason, "confidence"):
		return agentclient.HumanReviewReasonLowConfidence, false
	case strings.Contains(reason, "parse") || strings.Contains(reason, "parsing"):
		return agentclient.HumanReviewReasonLlmParsingError, false
	default:
		return agentclient.HumanReviewReasonInvestigationInconclusive, true
	}
}
