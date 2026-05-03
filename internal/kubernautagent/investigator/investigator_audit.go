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

package investigator

import (
	"context"
	"encoding/json"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

func (inv *Investigator) emitResponseComplete(ctx context.Context, result *katypes.InvestigationResult, tokens *TokenAccumulator, correlationID string) {
	completeEvent := audit.NewEvent(audit.EventTypeResponseComplete, correlationID)
	completeEvent.EventAction = audit.ActionResponseSent
	completeEvent.EventOutcome = audit.OutcomeSuccess
	for k, v := range tokens.AuditData() {
		completeEvent.Data[k] = v
	}
	inv.marshalAuditResponseData(completeEvent, result)
	audit.StoreBestEffort(ctx, inv.auditStore, completeEvent, inv.auditLog())
}

func (inv *Investigator) emitRCAComplete(ctx context.Context, result *katypes.InvestigationResult, tokens *TokenAccumulator, correlationID string) {
	ev := audit.NewEvent(audit.EventTypeRCAComplete, correlationID)
	ev.EventAction = audit.ActionLLMResponse
	ev.EventOutcome = audit.OutcomeSuccess
	for k, v := range tokens.AuditData() {
		ev.Data[k] = v
	}
	inv.marshalAuditResponseData(ev, result)
	audit.StoreBestEffort(ctx, inv.auditStore, ev, inv.auditLog())
}

func (inv *Investigator) marshalAuditResponseData(ev *audit.AuditEvent, result *katypes.InvestigationResult) {
	b, err := json.Marshal(ResultToAuditJSON(result))
	if err != nil {
		inv.auditLog().Error(err, "failed to marshal response_data for audit event",
			"event_type", ev.EventType)
		ev.Data["marshal_error"] = err.Error()
		return
	}
	ev.Data["response_data"] = string(b)
}

// ResultToAuditJSON converts an InvestigationResult to a map suitable for audit logging.
func ResultToAuditJSON(r *katypes.InvestigationResult) map[string]interface{} {
	m := map[string]interface{}{
		"rca_summary":        r.RCASummary,
		"severity":           r.Severity,
		"confidence":         r.Confidence,
		"needs_human_review": r.HumanReviewNeeded,
	}
	if r.WorkflowID != "" {
		m["workflow_id"] = r.WorkflowID
	}
	if r.ExecutionBundle != "" {
		m["execution_bundle"] = r.ExecutionBundle
	}
	if r.ExecutionBundleDigest != "" {
		m["execution_bundle_digest"] = r.ExecutionBundleDigest
	}
	if r.ExecutionEngine != "" {
		m["execution_engine"] = r.ExecutionEngine
	}
	if len(r.ContributingFactors) > 0 {
		m["contributing_factors"] = r.ContributingFactors
	}
	if r.HumanReviewReason != "" {
		m["human_review_reason"] = r.HumanReviewReason
	} else if r.Reason != "" {
		m["human_review_reason"] = r.Reason
	}
	if r.Reason != "" {
		m["reason"] = r.Reason
	}
	if r.IsActionable != nil {
		m["is_actionable"] = *r.IsActionable
	}
	if r.SignalName != "" {
		m["signal_name"] = r.SignalName
	}
	if r.DetectedLabels != nil {
		m["detected_labels"] = r.DetectedLabels
	}
	if len(r.ValidationAttemptsHistory) > 0 {
		m["validation_attempts_history"] = r.ValidationAttemptsHistory
	}
	if len(r.Warnings) > 0 {
		m["warnings"] = r.Warnings
	}
	if len(r.Parameters) > 0 {
		m["parameters"] = r.Parameters
	}
	if r.RemediationTarget.Kind != "" {
		m["remediation_target"] = map[string]interface{}{
			"kind":      r.RemediationTarget.Kind,
			"name":      r.RemediationTarget.Name,
			"namespace": r.RemediationTarget.Namespace,
		}
	}
	if len(r.AlternativeWorkflows) > 0 {
		alts := make([]map[string]interface{}, len(r.AlternativeWorkflows))
		for i, alt := range r.AlternativeWorkflows {
			a := map[string]interface{}{
				"workflow_id": alt.WorkflowID,
				"confidence":  alt.Confidence,
			}
			if alt.Rationale != "" {
				a["rationale"] = alt.Rationale
			}
			if alt.ExecutionBundle != "" {
				a["execution_bundle"] = alt.ExecutionBundle
			}
			alts[i] = a
		}
		m["alternative_workflows"] = alts
	}
	if len(r.CausalChain) > 0 {
		m["causal_chain"] = r.CausalChain
	}
	if r.DueDiligence != nil {
		m["due_diligence"] = map[string]interface{}{
			"causal_completeness":    r.DueDiligence.CausalCompleteness,
			"target_accuracy":        r.DueDiligence.TargetAccuracy,
			"evidence_sufficiency":   r.DueDiligence.EvidenceSufficiency,
			"alternative_hypotheses": r.DueDiligence.AlternativeHypotheses,
			"scope_completeness":     r.DueDiligence.ScopeCompleteness,
			"proportionality":        r.DueDiligence.Proportionality,
			"regression_awareness":   r.DueDiligence.RegressionAwareness,
			"confidence_calibration": r.DueDiligence.ConfidenceCalibration,
		}
	}
	return m
}

func (inv *Investigator) emitValidationEvent(ctx context.Context, attempt, maxAttempts int, isValid bool, errors []string, workflowID, correlationID string) {
	valEvent := audit.NewEvent(audit.EventTypeValidationAttempt, correlationID)
	valEvent.EventAction = audit.ActionValidation
	if isValid {
		valEvent.EventOutcome = audit.OutcomeSuccess
	} else {
		valEvent.EventOutcome = audit.OutcomeFailure
	}
	valEvent.Data["attempt"] = attempt
	valEvent.Data["max_attempts"] = maxAttempts
	valEvent.Data["is_valid"] = isValid
	valEvent.Data["errors"] = errors
	valEvent.Data["workflow_id"] = workflowID
	valEvent.Data["is_final_attempt"] = attempt == maxAttempts
	audit.StoreBestEffort(ctx, inv.auditStore, valEvent, inv.auditLog())
}

func toolErrorJSON(msg string) string {
	payload := struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}{Status: "error", Error: msg}
	b, err := json.Marshal(payload)
	if err != nil {
		return `{"status":"error","error":"marshal failure"}`
	}
	return string(b)
}

func messagesToAuditFormat(messages []llm.Message) []map[string]interface{} {
	out := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		entry := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
		if m.ToolCallID != "" {
			entry["tool_call_id"] = m.ToolCallID
		}
		if m.ToolName != "" {
			entry["name"] = m.ToolName
		}
		if len(m.ToolCalls) > 0 {
			entry["tool_calls"] = m.ToolCalls
		}
		out[i] = entry
	}
	return out
}
