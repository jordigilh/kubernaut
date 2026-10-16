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
	"errors"
	"fmt"
	"strings"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
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
	applySelectionAuditFields(m, r)
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
	if r.CausalMechanismStatus != "" {
		m["causal_mechanism_status"] = r.CausalMechanismStatus
	}
	// #2387 (BR-KA-OBSERVABILITY-001, FedRAMP AU-3): server-computed
	// call-level counts ride the audit trail so a remediation lifecycle is
	// fully reconstructable from audit traces alone (BR-AUDIT-005). Always
	// present — even zero — for a uniform SIEM query schema.
	m["total_llm_turns"] = r.TotalLLMTurns
	m["total_tool_calls"] = r.TotalToolCalls
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
	// A rejected proposal is never execution configuration (DD-KA-2496).
	if r.WorkflowID == "" && r.ProposedWorkflowID != "" {
		return
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

// applySelectionAuditFields shares the sanitized live allow-list, then adds
// trusted prior-attempt links only for the durable audit boundary (AU-3/CC7.2).
func applySelectionAuditFields(m map[string]interface{}, r *katypes.InvestigationResult) {
	selection := katypes.LiveSelectionFromResult(r)
	if selected := selection.SelectedWorkflow; selected != nil {
		applySelectedAuditFields(m, selected)
		if len(r.PriorRemediationUIDs) > 0 {
			m["prior_remediation_uids"] = append([]string(nil), r.PriorRemediationUIDs...)
		}
		return
	}
	if assessed := selection.SelectionAssessment; assessed != nil {
		assessment := selectionAuditAssessment(assessed)
		if len(r.PriorRemediationUIDs) > 0 {
			assessment["prior_remediation_uids"] = append([]string(nil), r.PriorRemediationUIDs...)
		}
		m["selection_assessment"] = assessment
	}
}

func applySelectedAuditFields(m map[string]interface{}, selected *katypes.LiveSelectedWorkflow) {
	m["workflow_name"] = selected.WorkflowName
	if selected.WorkflowSelectionConfidence != nil {
		m["workflow_selection_confidence"] = *selected.WorkflowSelectionConfidence
	}
	if selected.RepeatJustification != nil {
		m["repeat_justification"] = selected.RepeatJustification
	}
	if selected.RepeatJustificationRequired != nil {
		m["repeat_justification_required"] = *selected.RepeatJustificationRequired
	}
	if selected.PriorWorkflowMatch != nil {
		m["prior_workflow_match"] = *selected.PriorWorkflowMatch
	}
	if selected.RepeatValidationOutcome != "" {
		m["repeat_validation_outcome"] = selected.RepeatValidationOutcome
	}
	if selected.HandoffDigest != "" {
		m["handoff_digest"] = selected.HandoffDigest
	}
}

// ResultToAuditJSON uses encoding/json, not ogen's optional-field encoder.
// Keep the sanitized projection JSON-shaped so absent rejected reasoning remains
// absent instead of invoking an unset ogen Opt value's MarshalJSON (AU-3/CC7.2).
func selectionAuditAssessment(assessed *katypes.LiveSelectionAssessment) map[string]interface{} {
	assessment := map[string]interface{}{"proposed_workflow_id": assessed.ProposedWorkflowID}
	if assessed.WorkflowName != "" {
		assessment["workflow_name"] = assessed.WorkflowName
	}
	if assessed.WorkflowSelectionConfidence != nil {
		assessment["workflow_selection_confidence"] = *assessed.WorkflowSelectionConfidence
	}
	if assessed.RepeatJustification != nil {
		assessment["repeat_justification"] = assessed.RepeatJustification
	}
	if assessed.RepeatJustificationRequired != nil {
		assessment["repeat_justification_required"] = *assessed.RepeatJustificationRequired
	}
	if assessed.PriorWorkflowMatch != nil {
		assessment["prior_workflow_match"] = *assessed.PriorWorkflowMatch
	}
	if assessed.RepeatValidationOutcome != "" {
		assessment["repeat_validation_outcome"] = assessed.RepeatValidationOutcome
	}
	if assessed.HandoffDigest != "" {
		assessment["handoff_digest"] = assessed.HandoffDigest
	}
	return assessment
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
	if len(r.Parameters) > 0 && (r.WorkflowID != "" || r.ProposedWorkflowID == "") {
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
// BR-KA-191: Uses validation_error.tmpl with schema hints for parameter errors,
// falls back to format error template for other validation failures.
func (inv *Investigator) renderCorrectionMessage(validationErr error, attempt, maxAttempts int) (string, error) {
	data := prompt.ValidationErrorData{
		AttemptDisplay: attempt,
		MaxAttempts:    maxAttempts,
		Errors:         []string{validationErr.Error()},
	}

	var paramErr *parser.ParameterValidationError
	if errors.As(validationErr, &paramErr) {
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

const maxAuditToolArgumentsBytes = 4096

var sensitiveAuditArgumentKeyFragments = []string{
	"api_key", "apikey", "authorization", "body", "credential", "header", "password",
	"parameters", "payload", "private_key", "secret", "signature", "token",
}

func auditToolErrorDetails(result string) *sharedaudit.ErrorDetails {
	var envelope struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &envelope); err != nil || envelope.Status != "error" {
		return nil
	}

	message := strings.TrimSpace(envelope.Error)
	if message == "" {
		message = "tool execution failed"
	}
	message = sanitization.SanitizeForLog(truncatePreview(message, 500))
	return &sharedaudit.ErrorDetails{
		Message:       message,
		Code:          "ERR_TOOL_EXECUTION_FAILED",
		Component:     "kubernautagent",
		RetryPossible: false,
	}
}

func auditToolArgumentKeyIsSensitive(key string) bool {
	lowerKey := strings.ToLower(key)
	for _, fragment := range sensitiveAuditArgumentKeyFragments {
		if strings.Contains(lowerKey, fragment) {
			return true
		}
	}
	return false
}

// auditSafeToolArguments preserves bounded, non-sensitive tool identity while
// preventing raw model arguments from entering durable audit storage (AU-3/AU-9).
func auditSafeToolArguments(raw string) string {
	var arguments map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil || arguments == nil {
		return `{"arguments_omitted":true,"reason":"invalid_or_non_object"}`
	}

	safe := make(map[string]json.RawMessage, len(arguments))
	for key, value := range arguments {
		if auditToolArgumentKeyIsSensitive(key) {
			safe[key] = json.RawMessage(`"[REDACTED]"`)
			continue
		}

		sanitized := sanitization.SanitizeForLog(string(value))
		if len(sanitized) > 1024 || !json.Valid([]byte(sanitized)) {
			safe[key] = json.RawMessage(`"[TRUNCATED]"`)
			continue
		}
		safe[key] = json.RawMessage(sanitized)
	}

	encoded, err := json.Marshal(safe)
	if err != nil || len(encoded) > maxAuditToolArgumentsBytes {
		return `{"arguments_omitted":true,"reason":"size_limit"}`
	}
	return string(encoded)
}

// emitToolCallAudit preserves the trusted phase and standardized denial details
// through the typed DS boundary (DD-KA-2496, DD-ERROR-001, AU-2/AU-3, CC7.2).
func (inv *Investigator) emitToolCallAudit(ctx context.Context, tc llm.ToolCall, index int, result, phase, correlationID string) {
	event := audit.NewEvent(audit.EventTypeLLMToolCall, correlationID)
	event.EventAction = audit.ActionToolExecution
	event.EventOutcome = audit.OutcomeSuccess
	event.Data["phase"] = phase
	if !inv.toolAllowedInPhase(ctx, katypes.Phase(phase), tc.Name) {
		event.EventOutcome = audit.OutcomeFailure
		event.Data["error_details"] = &sharedaudit.ErrorDetails{
			Message: fmt.Sprintf("tool %q not allowed in phase %q", tc.Name, phase),
			Code:    "ERR_TOOL_PHASE_DENIED", Component: "kubernautagent", RetryPossible: false,
		}
	} else if details := auditToolErrorDetails(result); details != nil {
		event.EventOutcome = audit.OutcomeFailure
		event.Data["error_details"] = details
	}
	event.Data["tool_call_index"] = index
	event.Data["tool_name"] = tc.Name
	event.Data["tool_arguments"] = auditSafeToolArguments(tc.Arguments)
	auditResult := auditSafeToolResult(tc.Name, result)
	event.Data["tool_result"] = auditResult
	event.Data["tool_result_preview"] = truncatePreview(auditResult, 500)
	audit.StoreBestEffort(ctx, inv.auditStore, event, inv.auditLog())
}

func messagesToAuditFormat(messages []llm.Message) []map[string]interface{} {
	out := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		entry := map[string]interface{}{
			"role":    m.Role,
			"content": auditSafeToolResult(m.ToolName, m.Content),
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
