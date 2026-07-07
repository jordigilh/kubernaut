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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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
	applyWorkflowIdentityFields(m, r)
	applyReviewReasonFields(m, r)
	applyCollectionFields(m, r)

	if r.RemediationTarget.Kind != "" {
		m["remediation_target"] = map[string]interface{}{
			"kind":      r.RemediationTarget.Kind,
			"name":      r.RemediationTarget.Name,
			"namespace": r.RemediationTarget.Namespace,
		}
	}
	if len(r.AlternativeWorkflows) > 0 {
		m["alternative_workflows"] = alternativeWorkflowsToAuditJSON(r.AlternativeWorkflows)
	}
	if len(r.CausalChain) > 0 {
		m["causal_chain"] = r.CausalChain
	}
	if r.DueDiligence != nil {
		m["due_diligence"] = dueDiligenceToAuditJSON(r.DueDiligence)
	}
	if r.Reasoning != nil {
		m["reasoning"] = reasoningAuditFields(r.Reasoning.Text, r.Reasoning.Redacted)
	}
	return m
}

// maxReasoningAuditChars bounds reasoning text stored per audit record
// (BR-AI-086 AC6). Adaptive thinking budgets default to ~10K tokens
// (DD-LLM-005); this cap comfortably covers the default budget in full
// while guarding audit storage against a runaway/misconfigured manual
// budget on models that support much larger extended-thinking windows.
const maxReasoningAuditChars = 40000

const reasoningTruncatedMarker = "…[reasoning truncated for audit storage]…"

// reasoningAuditFields renders reasoning text/redacted into the map shape
// shared by ResultToAuditJSON and messagesToAuditFormat, applying the audit
// size guard.
func reasoningAuditFields(text string, redacted bool) map[string]interface{} {
	return map[string]interface{}{
		"text":     truncateReasoning(text),
		"redacted": redacted,
	}
}

// truncateReasoning rune-safely caps s at maxReasoningAuditChars, appending
// a marker when truncation occurs.
func truncateReasoning(s string) string {
	runes := []rune(s)
	if len(runes) <= maxReasoningAuditChars {
		return s
	}
	return string(runes[:maxReasoningAuditChars]) + reasoningTruncatedMarker
}

// applyWorkflowIdentityFields sets the workflow-identity fields on m,
// omitting each one that is empty on r.
func applyWorkflowIdentityFields(m map[string]interface{}, r *katypes.InvestigationResult) {
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
}

// applyReviewReasonFields sets the human-review/reason/actionability fields
// on m. human_review_reason falls back to r.Reason when HumanReviewReason is
// unset, so a caller-facing reason is always surfaced when either is present.
func applyReviewReasonFields(m map[string]interface{}, r *katypes.InvestigationResult) {
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
}

// applyCollectionFields sets the collection-valued (slice/map) fields on m,
// omitting each one that is nil/empty on r.
func applyCollectionFields(m map[string]interface{}, r *katypes.InvestigationResult) {
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
}

// alternativeWorkflowsToAuditJSON converts alternative workflows to the
// audit-log map representation, omitting empty optional fields.
func alternativeWorkflowsToAuditJSON(alts []katypes.AlternativeWorkflow) []map[string]interface{} {
	out := make([]map[string]interface{}, len(alts))
	for i, alt := range alts {
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
		out[i] = a
	}
	return out
}

// dueDiligenceToAuditJSON converts a DueDiligenceReview to the audit-log map
// representation.
func dueDiligenceToAuditJSON(dd *katypes.DueDiligenceReview) map[string]interface{} {
	return map[string]interface{}{
		"causal_completeness":    dd.CausalCompleteness,
		"target_accuracy":        dd.TargetAccuracy,
		"evidence_sufficiency":   dd.EvidenceSufficiency,
		"alternative_hypotheses": dd.AlternativeHypotheses,
		"scope_completeness":     dd.ScopeCompleteness,
		"proportionality":        dd.Proportionality,
		"regression_awareness":   dd.RegressionAwareness,
		"confidence_calibration": dd.ConfidenceCalibration,
	}
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

// renderCorrectionMessage builds a structured correction message for LLM self-correction.
// BR-HAPI-191: Uses validation_error.tmpl with schema hints for parameter errors,
// falls back to format error template for other validation failures.
func (inv *Investigator) renderCorrectionMessage(validationErr error, attempt, maxAttempts int) (string, error) {
	data := prompt.ValidationErrorData{
		AttemptDisplay: attempt,
		MaxAttempts:    maxAttempts,
		Errors:         []string{validationErr.Error()},
	}

	if paramErr, ok := validationErr.(*parser.ParameterValidationError); ok {
		data.Errors = paramErr.Result.Errors
		data.SchemaHint = paramErr.Result.SchemaHint
	}

	return inv.builder.RenderValidationError(data)
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
		if m.Reasoning != nil {
			entry["reasoning"] = reasoningAuditFields(m.Reasoning.Text, m.Reasoning.Redacted)
		}
		out[i] = entry
	}
	return out
}
