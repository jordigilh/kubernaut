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

// Package agentsession implements KA's side of the AgentSession CRD
// dispatch/result channel (DD-AA-KA-001, BR-AA-KA-065): a raw watch on
// AgentSession Create/Update events, exactly-once dispatch via a
// per-AgentSession coordination/v1 Lease, and curated Status writes.
package agentsession

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// MapSpecToSignal converts an AgentSession's Spec (AA's 1:1 translation of
// the retired agentclient.IncidentRequest, BR-AA-KA-065.2) into KA's
// internal SignalContext. Mirrors
// internal/kubernautagent/server/handler.go's MapIncidentRequestToSignal.
func MapSpecToSignal(spec agentsessionv1.AgentSessionSpec) katypes.SignalContext {
	sc := katypes.SignalContext{
		Name:                   spec.SignalName,
		Namespace:              spec.ResourceNamespace,
		Severity:               spec.Severity,
		Message:                spec.ErrorMessage,
		IncidentID:             spec.IncidentID,
		RemediationID:          spec.RemediationID,
		ResourceKind:           spec.ResourceKind,
		ResourceName:           spec.ResourceName,
		ResourceAPIVersion:     spec.ResourceAPIVersion,
		ClusterName:            spec.ClusterName,
		Environment:            spec.Environment,
		Priority:               spec.Priority,
		RiskTolerance:          spec.RiskTolerance,
		SignalSource:           spec.SignalSource,
		BusinessCategory:       spec.BusinessCategory,
		Description:            spec.Description,
		SignalMode:             strings.ToLower(spec.SignalMode),
		ClusterClassification:  spec.Cluster,
		FiringTime:             spec.FiringTime,
		ReceivedTime:           spec.ReceivedTime,
		FirstSeen:              spec.FirstSeen,
		LastSeen:               spec.LastSeen,
		SignalAnnotations:      spec.SignalAnnotations,
		SignalLabels:           spec.SignalLabels,
		// Interactive is deliberately NOT mapped here (DD-AA-KA-001
		// Amendment Gap 1): Spec has no Interactive field. The dispatcher
		// sets sc.Interactive itself, from its own dispatch-time
		// InvestigationSession-existence check -- the freshest-possible
		// data, since Spec is an immutable Create-time snapshot.
	}
	if spec.IsDuplicate != nil {
		sc.IsDuplicate = spec.IsDuplicate
	}
	if spec.OccurrenceCount != nil {
		sc.OccurrenceCount = spec.OccurrenceCount
	}
	if spec.DeduplicationWindowMinutes != nil {
		sc.DeduplicationWindowMinutes = spec.DeduplicationWindowMinutes
	}
	return sc
}

// MapInvestigationResultToAgentSessionResult converts KA's internal
// InvestigationResult into the CRD-native AgentSessionResult written to
// AgentSession.Status.Result on the Completed transition (SI-10: curated
// subset only). Mirrors
// internal/kubernautagent/server/handler_response_mapping.go's
// mapInvestigationResultToResponse, targeting the CRD-native type instead
// of the retired ogen agentclient.IncidentResponse.
func MapInvestigationResultToAgentSessionResult(log logr.Logger, r *katypes.InvestigationResult, incidentID string) *agentsessionv1.AgentSessionResult {
	res := &agentsessionv1.AgentSessionResult{
		IncidentID:        incidentID,
		Analysis:           r.RCASummary,
		Confidence:         r.Confidence,
		Timestamp:          time.Now().UTC().Format(time.RFC3339),
		RootCauseAnalysis:  marshalJSON(buildRootCauseAnalysisMap(r)),
		NeedsHumanReview:   r.HumanReviewNeeded,
		Warnings:           buildWarnings(r),
	}

	applyHumanReviewReason(res, log, r)

	if r.WorkflowID != "" {
		res.SelectedWorkflow = marshalJSON(buildSelectedWorkflowMap(r))
	}
	if r.IsActionable != nil {
		res.IsActionable = r.IsActionable
	}
	if len(r.DetectedLabels) > 0 {
		res.DetectedLabels = marshalJSON(r.DetectedLabels)
	}
	if len(r.AlternativeWorkflows) > 0 {
		res.AlternativeWorkflows = buildAlternativeWorkflows(r)
	}
	if len(r.ValidationAttemptsHistory) > 0 {
		res.ValidationAttemptsHistory = buildValidationAttempts(r)
	}
	if r.AlignmentVerdict != nil {
		res.AlignmentVerdict = buildAlignmentVerdict(r)
	}
	return res
}

// marshalJSON marshals v into an apiextensionsv1.JSON, returning nil when v
// is nil/empty or marshaling fails (never panics on a malformed result).
func marshalJSON(v interface{}) *apiextensionsv1.JSON {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil || len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	return &apiextensionsv1.JSON{Raw: raw}
}

func buildRootCauseAnalysisMap(r *katypes.InvestigationResult) map[string]interface{} {
	rca := map[string]interface{}{}
	if r.RCASummary != "" {
		rca["summary"] = r.RCASummary
	}
	if r.Severity != "" {
		rca["severity"] = r.Severity
	}
	if r.RemediationTarget.Kind != "" {
		rca["remediationTarget"] = r.RemediationTarget
	}
	if r.SignalName != "" {
		rca["signal_name"] = r.SignalName
	}
	if len(r.ContributingFactors) > 0 {
		rca["contributing_factors"] = r.ContributingFactors
	}
	if len(r.CausalChain) > 0 {
		rca["causal_chain"] = r.CausalChain
	}
	if r.DueDiligence != nil {
		rca["due_diligence"] = r.DueDiligence
	}
	if len(rca) == 0 {
		return nil
	}
	return rca
}

func buildSelectedWorkflowMap(r *katypes.InvestigationResult) map[string]interface{} {
	sw := map[string]interface{}{
		"workflow_id": r.WorkflowID,
		"confidence":  r.Confidence,
	}
	if len(r.Parameters) > 0 {
		sw["parameters"] = r.Parameters
	}
	if r.ExecutionBundle != "" {
		sw["execution_bundle"] = r.ExecutionBundle
	}
	if r.ExecutionBundleDigest != "" {
		sw["execution_bundle_digest"] = r.ExecutionBundleDigest
	}
	if r.ExecutionEngine != "" {
		sw["execution_engine"] = r.ExecutionEngine
	}
	if r.ServiceAccountName != "" {
		sw["service_account_name"] = r.ServiceAccountName
	}
	if r.WorkflowVersion != "" {
		sw["version"] = r.WorkflowVersion
	}
	if r.WorkflowRationale != "" {
		sw["rationale"] = r.WorkflowRationale
	}
	if r.Dependencies != nil {
		sw["dependencies"] = r.Dependencies
	}
	if r.Resources != nil {
		sw["resources"] = r.Resources
	}
	if r.DeclaredParameterNames != nil {
		sw["declared_parameter_names"] = r.DeclaredParameterNames
	}
	if r.ActionType != "" {
		sw["action_type"] = r.ActionType
	}
	if r.WorkflowName != "" {
		sw["workflow_name"] = r.WorkflowName
	}
	return sw
}

func buildWarnings(r *katypes.InvestigationResult) []string {
	if len(r.Warnings) > 0 {
		return r.Warnings
	}
	if r.HumanReviewNeeded {
		return []string{synthesizeHumanReviewWarning(r)}
	}
	return []string{}
}

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

// applyHumanReviewReason sets res.HumanReviewReason when review is needed,
// mapping the (possibly LLM-provided, possibly unrecognized) reason string
// to a canonical value. Empty string when review is not needed, matching
// AgentSessionResult.HumanReviewReason's "empty means no reason" contract.
func applyHumanReviewReason(res *agentsessionv1.AgentSessionResult, log logr.Logger, r *katypes.InvestigationResult) {
	if !r.HumanReviewNeeded {
		return
	}
	reason := r.HumanReviewReason
	if reason == "" {
		reason = r.Reason
	}
	mapped, isDefault := mapHumanReviewReason(reason)
	if isDefault && reason != "" {
		log.Info("unrecognized human review reason, falling back to investigation_inconclusive",
			"original_reason", reason)
	}
	res.HumanReviewReason = mapped
}

var exactHumanReviewReasons = map[string]string{
	"rca_incomplete":              "rca_incomplete",
	"investigation_inconclusive":  "investigation_inconclusive",
	"workflow_not_found":          "workflow_not_found",
	"no_matching_workflows":       "no_matching_workflows",
	"image_mismatch":              "image_mismatch",
	"parameter_validation_failed": "parameter_validation_failed",
	"low_confidence":              "low_confidence",
	"llm_parsing_error":           "llm_parsing_error",
	"alignment_check_failed":      "alignment_check_failed",
	"operator_escalation":         "operator_escalation",
	"decision_expired":            "decision_expired",
}

func mapHumanReviewReason(reason string) (string, bool) {
	if mapped, ok := exactHumanReviewReasons[reason]; ok {
		return mapped, false
	}
	return mapHumanReviewReasonHeuristic(reason)
}

func mapHumanReviewReasonHeuristic(reason string) (string, bool) {
	switch {
	case strings.Contains(reason, "exhausted during RCA"):
		return "rca_incomplete", false
	case strings.Contains(reason, "exhausted during workflow selection"):
		return "investigation_inconclusive", false
	case strings.Contains(reason, "not found") && strings.Contains(reason, "catalog"):
		return "workflow_not_found", false
	case strings.Contains(reason, "no matching"):
		return "no_matching_workflows", false
	case strings.Contains(reason, "mismatch") || strings.Contains(reason, "image"):
		return "image_mismatch", false
	case strings.Contains(reason, "parameter") || strings.Contains(reason, "validation"):
		return "parameter_validation_failed", false
	case strings.Contains(reason, "confidence"):
		return "low_confidence", false
	case strings.Contains(reason, "parse") || strings.Contains(reason, "parsing"):
		return "llm_parsing_error", false
	default:
		return "investigation_inconclusive", true
	}
}

func buildAlternativeWorkflows(r *katypes.InvestigationResult) []agentsessionv1.AgentSessionAlternativeWorkflow {
	alts := make([]agentsessionv1.AgentSessionAlternativeWorkflow, 0, len(r.AlternativeWorkflows))
	for _, aw := range r.AlternativeWorkflows {
		alts = append(alts, agentsessionv1.AgentSessionAlternativeWorkflow{
			WorkflowID:      aw.WorkflowID,
			ExecutionBundle: aw.ExecutionBundle,
			Confidence:      aw.Confidence,
			Rationale:       aw.Rationale,
		})
	}
	return alts
}

func buildValidationAttempts(r *katypes.InvestigationResult) []agentsessionv1.AgentSessionValidationAttempt {
	attempts := make([]agentsessionv1.AgentSessionValidationAttempt, 0, len(r.ValidationAttemptsHistory))
	for _, va := range r.ValidationAttemptsHistory {
		attempts = append(attempts, agentsessionv1.AgentSessionValidationAttempt{
			Attempt:    va.Attempt,
			WorkflowID: va.WorkflowID,
			IsValid:    va.IsValid,
			Errors:     va.Errors,
			Timestamp:  va.Timestamp,
		})
	}
	return attempts
}

func buildAlignmentVerdict(r *katypes.InvestigationResult) *agentsessionv1.AgentSessionAlignmentVerdict {
	av := &agentsessionv1.AgentSessionAlignmentVerdict{
		Result:                  r.AlignmentVerdict.Result,
		CircuitBreakerActivated: r.AlignmentVerdict.CircuitBreakerActivated,
		Summary:                 r.AlignmentVerdict.Summary,
		Flagged:                 r.AlignmentVerdict.Flagged,
		Total:                   r.AlignmentVerdict.Total,
	}
	for _, f := range r.AlignmentVerdict.Findings {
		av.Findings = append(av.Findings, agentsessionv1.AgentSessionAlignmentFinding{
			StepIndex:   f.StepIndex,
			StepKind:    f.StepKind,
			Tool:        f.Tool,
			Explanation: f.Explanation,
		})
	}
	return av
}
