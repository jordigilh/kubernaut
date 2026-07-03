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
	"github.com/go-faster/jx"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

func buildEnrichmentCompletedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentEnrichmentCompletedPayload{
		EventType:                 ogenclient.AIAgentEnrichmentCompletedPayloadEventTypeAiagentEnrichmentCompleted,
		EventID:                   dataString(event.Data, "event_id"),
		IncidentID:                dataString(event.Data, "incident_id"),
		RootOwnerKind:             dataString(event.Data, "root_owner_kind"),
		RootOwnerName:             dataString(event.Data, "root_owner_name"),
		OwnerChainLength:          dataInt(event.Data, "owner_chain_length"),
		RemediationHistoryFetched: dataBool(event.Data, "remediation_history_fetched"),
	}
	if ns := dataString(event.Data, "root_owner_namespace"); ns != "" {
		payload.RootOwnerNamespace.SetTo(ns)
	}
	return ogenclient.NewAIAgentEnrichmentCompletedPayloadAuditEventRequestEventData(payload)
}

func buildEnrichmentFailedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentEnrichmentFailedPayload{
		EventType:            ogenclient.AIAgentEnrichmentFailedPayloadEventTypeAiagentEnrichmentFailed,
		EventID:              dataString(event.Data, "event_id"),
		IncidentID:           dataString(event.Data, "incident_id"),
		Reason:               dataString(event.Data, "reason"),
		Detail:               dataString(event.Data, "detail"),
		AffectedResourceKind: dataString(event.Data, "affected_resource_kind"),
		AffectedResourceName: dataString(event.Data, "affected_resource_name"),
	}
	if ns := dataString(event.Data, "affected_resource_namespace"); ns != "" {
		payload.AffectedResourceNamespace.SetTo(ns)
	}
	return ogenclient.NewAIAgentEnrichmentFailedPayloadAuditEventRequestEventData(payload)
}

func buildLLMRequestPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.LLMRequestPayload{
		EventType:     ogenclient.LLMRequestPayloadEventTypeAiagentLlmRequest,
		EventID:       dataString(event.Data, "event_id"),
		IncidentID:    event.CorrelationID,
		Model:         dataString(event.Data, "model"),
		PromptLength:  dataInt(event.Data, "prompt_length"),
		PromptPreview: truncate(dataString(event.Data, "prompt_preview"), previewMaxLen),
	}
	if tools := dataStringSlice(event.Data, "toolsets_enabled"); len(tools) > 0 {
		payload.ToolsetsEnabled = tools
	}
	return ogenclient.NewLLMRequestPayloadAuditEventRequestEventData(payload)
}

func buildLLMResponsePayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.LLMResponsePayload{
		EventType:       ogenclient.LLMResponsePayloadEventTypeAiagentLlmResponse,
		EventID:         dataString(event.Data, "event_id"),
		IncidentID:      event.CorrelationID,
		HasAnalysis:     dataBool(event.Data, "has_analysis"),
		AnalysisLength:  dataInt(event.Data, "analysis_length"),
		AnalysisPreview: truncate(dataString(event.Data, "analysis_preview"), previewMaxLen),
	}
	if full := dataString(event.Data, "analysis_full"); full != "" {
		payload.AnalysisFull.SetTo(full)
	}
	if tokens := dataInt(event.Data, "total_tokens"); tokens > 0 {
		payload.TokensUsed.SetTo(tokens)
	}
	if tc := dataInt(event.Data, "tool_call_count"); tc > 0 {
		payload.ToolCallCount.SetTo(tc)
	}
	return ogenclient.NewLLMResponsePayloadAuditEventRequestEventData(payload)
}

func buildLLMToolCallPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.LLMToolCallPayload{
		EventType:     ogenclient.LLMToolCallPayloadEventTypeAiagentLlmToolCall,
		EventID:       dataString(event.Data, "event_id"),
		IncidentID:    event.CorrelationID,
		ToolCallIndex: dataInt(event.Data, "tool_call_index"),
		ToolName:      dataString(event.Data, "tool_name"),
	}
	if args := dataString(event.Data, "tool_arguments"); args != "" {
		payload.ToolArguments.SetTo(toJxRawMap(args))
	}
	payload.ToolResult = toJxRaw(dataString(event.Data, "tool_result"))
	if len(payload.ToolResult) == 0 {
		payload.ToolResult = jx.Raw(`""`)
	}
	if preview := dataString(event.Data, "tool_result_preview"); preview != "" {
		payload.ToolResultPreview.SetTo(truncate(preview, previewMaxLen))
	}
	return ogenclient.NewLLMToolCallPayloadAuditEventRequestEventData(payload)
}

func buildValidationAttemptPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.WorkflowValidationPayload{
		EventType:   ogenclient.WorkflowValidationPayloadEventTypeAiagentWorkflowValidationAttempt,
		EventID:     dataString(event.Data, "event_id"),
		IncidentID:  event.CorrelationID,
		Attempt:     dataInt(event.Data, "attempt"),
		MaxAttempts: dataInt(event.Data, "max_attempts"),
		IsValid:     dataBool(event.Data, "is_valid"),
	}
	if errs, ok := event.Data["errors"].([]string); ok {
		payload.Errors = errs
	}
	if wfID := dataString(event.Data, "workflow_id"); wfID != "" {
		payload.WorkflowID.SetTo(wfID)
	}
	if dataBool(event.Data, "is_final_attempt") {
		payload.IsFinalAttempt.SetTo(true)
	}
	return ogenclient.NewWorkflowValidationPayloadAuditEventRequestEventData(payload)
}

func buildResponseCompletePayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentResponsePayload{
		EventType:  ogenclient.AIAgentResponsePayloadEventTypeAiagentResponseComplete,
		EventID:    dataString(event.Data, "event_id"),
		IncidentID: event.CorrelationID,
	}
	if rd := dataString(event.Data, "response_data"); rd != "" {
		payload.ResponseData = toIncidentResponseData(rd, event.CorrelationID)
	}
	if pt := dataInt(event.Data, "total_prompt_tokens"); pt > 0 {
		payload.TotalPromptTokens.SetTo(pt)
	}
	if ct := dataInt(event.Data, "total_completion_tokens"); ct > 0 {
		payload.TotalCompletionTokens.SetTo(ct)
	}
	return ogenclient.NewAIAgentResponsePayloadAuditEventRequestEventData(payload)
}

func buildRCACompletePayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentRCACompletePayload{
		EventType:  ogenclient.AIAgentRCACompletePayloadEventTypeAiagentRcaComplete,
		EventID:    dataString(event.Data, "event_id"),
		IncidentID: event.CorrelationID,
	}
	if rd := dataString(event.Data, "response_data"); rd != "" {
		payload.ResponseData = toIncidentResponseData(rd, event.CorrelationID)
	}
	if pt := dataInt(event.Data, "total_prompt_tokens"); pt > 0 {
		payload.TotalPromptTokens.SetTo(pt)
	}
	if ct := dataInt(event.Data, "total_completion_tokens"); ct > 0 {
		payload.TotalCompletionTokens.SetTo(ct)
	}
	return ogenclient.NewAIAgentRCACompletePayloadAuditEventRequestEventData(payload)
}

func buildResponseFailedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentResponseFailedPayload{
		EventType:    ogenclient.AIAgentResponseFailedPayloadEventTypeAiagentResponseFailed,
		EventID:      dataString(event.Data, "event_id"),
		IncidentID:   event.CorrelationID,
		ErrorMessage: dataString(event.Data, "error_message"),
		Phase:        dataString(event.Data, "phase"),
	}
	if dur := dataFloat64(event.Data, "duration_seconds"); dur > 0 {
		payload.DurationSeconds.SetTo(float32(dur))
	}
	return ogenclient.NewAIAgentResponseFailedPayloadAuditEventRequestEventData(payload)
}

func buildSessionStartedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentSessionStartedPayload{
		EventType: ogenclient.AIAgentSessionStartedPayloadEventTypeAiagentSessionStarted,
		EventID:   dataString(event.Data, "event_id"),
		SessionID: event.SessionID,
	}
	if v := dataString(event.Data, "incident_id"); v != "" {
		payload.IncidentID.SetTo(v)
	}
	if v := dataString(event.Data, "signal_name"); v != "" {
		payload.SignalName.SetTo(v)
	}
	if v := dataString(event.Data, "severity"); v != "" {
		payload.Severity.SetTo(v)
	}
	if v := dataString(event.Data, "created_by"); v != "" {
		payload.CreatedBy.SetTo(v)
	}
	return ogenclient.NewAIAgentSessionStartedPayloadAuditEventRequestEventData(payload)
}

func buildSessionCompletedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentSessionCompletedPayload{
		EventType: ogenclient.AIAgentSessionCompletedPayloadEventTypeAiagentSessionCompleted,
		EventID:   dataString(event.Data, "event_id"),
		SessionID: event.SessionID,
	}
	return ogenclient.NewAIAgentSessionCompletedPayloadAuditEventRequestEventData(payload)
}

func buildSessionFailedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentSessionFailedPayload{
		EventType: ogenclient.AIAgentSessionFailedPayloadEventTypeAiagentSessionFailed,
		EventID:   dataString(event.Data, "event_id"),
		SessionID: event.SessionID,
	}
	if v := dataString(event.Data, "error"); v != "" {
		payload.Error.SetTo(v)
	}
	return ogenclient.NewAIAgentSessionFailedPayloadAuditEventRequestEventData(payload)
}

func buildSessionCancelledPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentSessionCancelledPayload{
		EventType: ogenclient.AIAgentSessionCancelledPayloadEventTypeAiagentSessionCancelled,
		EventID:   dataString(event.Data, "event_id"),
		SessionID: event.SessionID,
	}
	return ogenclient.NewAIAgentSessionCancelledPayloadAuditEventRequestEventData(payload)
}

func buildSessionObservedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentSessionObservedPayload{
		EventType: ogenclient.AIAgentSessionObservedPayloadEventTypeAiagentSessionObserved,
		EventID:   dataString(event.Data, "event_id"),
		SessionID: event.SessionID,
	}
	if v := dataString(event.Data, "observer_user"); v != "" {
		payload.ObserverUser.SetTo(v)
	}
	if v := dataString(event.Data, "session_owner"); v != "" {
		payload.SessionOwner.SetTo(v)
	}
	return ogenclient.NewAIAgentSessionObservedPayloadAuditEventRequestEventData(payload)
}

func buildSessionAccessDeniedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentSessionAccessDeniedPayload{
		EventType:      ogenclient.AIAgentSessionAccessDeniedPayloadEventTypeAiagentSessionAccessDenied,
		EventID:        dataString(event.Data, "event_id"),
		SessionID:      event.SessionID,
		Endpoint:       dataString(event.Data, "endpoint"),
		RequestingUser: dataString(event.Data, "requesting_user"),
	}
	if v := dataString(event.Data, "session_owner"); v != "" {
		payload.SessionOwner.SetTo(v)
	}
	return ogenclient.NewAIAgentSessionAccessDeniedPayloadAuditEventRequestEventData(payload)
}

func buildInvestigationCancelledPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentInvestigationCancelledPayload{
		EventType:       ogenclient.AIAgentInvestigationCancelledPayloadEventTypeAiagentInvestigationCancelled,
		EventID:         dataString(event.Data, "event_id"),
		CancelledPhase:  dataString(event.Data, "cancelled_phase"),
		CancelledAtTurn: dataInt(event.Data, "cancelled_at_turn"),
	}
	if event.SessionID != "" {
		payload.SessionID.SetTo(event.SessionID)
	}
	if pt := dataInt(event.Data, "total_prompt_tokens"); pt > 0 {
		payload.TotalPromptTokens.SetTo(pt)
	}
	if ct := dataInt(event.Data, "total_completion_tokens"); ct > 0 {
		payload.TotalCompletionTokens.SetTo(ct)
	}
	if tt := dataInt(event.Data, "total_tokens"); tt > 0 {
		payload.TotalTokens.SetTo(tt)
	}
	if v := dataString(event.Data, "accumulated_messages"); v != "" {
		payload.AccumulatedMessages.SetTo(v)
	}
	return ogenclient.NewAIAgentInvestigationCancelledPayloadAuditEventRequestEventData(payload)
}

func buildAlignmentStepPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentAlignmentStepPayload{
		EventType:   ogenclient.AIAgentAlignmentStepPayloadEventTypeAiagentAlignmentStep,
		EventID:     dataString(event.Data, "event_id"),
		StepIndex:   dataInt(event.Data, "step_index"),
		StepKind:    dataString(event.Data, "step_kind"),
		Explanation: dataString(event.Data, "explanation"),
	}
	if v := dataString(event.Data, "tool"); v != "" {
		payload.Tool.SetTo(v)
	}
	return ogenclient.NewAIAgentAlignmentStepPayloadAuditEventRequestEventData(payload)
}

func buildAlignmentVerdictPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentAlignmentVerdictPayload{
		EventType: ogenclient.AIAgentAlignmentVerdictPayloadEventTypeAiagentAlignmentVerdict,
		EventID:   dataString(event.Data, "event_id"),
		Result:    dataString(event.Data, "result"),
		Flagged:   dataInt(event.Data, "flagged"),
		Total:     dataInt(event.Data, "total"),
	}
	if v := dataString(event.Data, "summary"); v != "" {
		payload.Summary.SetTo(v)
	}
	if pt := dataInt(event.Data, "shadow_prompt_tokens"); pt > 0 {
		payload.ShadowPromptTokens.SetTo(pt)
	}
	if ct := dataInt(event.Data, "shadow_completion_tokens"); ct > 0 {
		payload.ShadowCompletionTokens.SetTo(ct)
	}
	if tt := dataInt(event.Data, "shadow_total_tokens"); tt > 0 {
		payload.ShadowTotalTokens.SetTo(tt)
	}
	return ogenclient.NewAIAgentAlignmentVerdictPayloadAuditEventRequestEventData(payload)
}

func buildSessionSuspendedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentSessionSuspendedPayload{
		EventType: ogenclient.AIAgentSessionSuspendedPayloadEventTypeAiagentSessionSuspended,
		EventID:   dataString(event.Data, "event_id"),
		SessionID: event.SessionID,
	}
	return ogenclient.NewAIAgentSessionSuspendedPayloadAuditEventRequestEventData(payload)
}

func buildSessionResumedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentSessionResumedPayload{
		EventType: ogenclient.AIAgentSessionResumedPayloadEventTypeAiagentSessionResumed,
		EventID:   dataString(event.Data, "event_id"),
		SessionID: event.SessionID,
	}
	return ogenclient.NewAIAgentSessionResumedPayloadAuditEventRequestEventData(payload)
}

func buildInteractiveStartedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentInteractiveStartedPayload{
		EventType: ogenclient.AIAgentInteractiveStartedPayloadEventTypeAiagentInteractiveStarted,
		EventID:   dataString(event.Data, "event_id"),
		SessionID: event.SessionID,
	}
	return ogenclient.NewAIAgentInteractiveStartedPayloadAuditEventRequestEventData(payload)
}

func buildInteractiveCompletedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentInteractiveCompletedPayload{
		EventType: ogenclient.AIAgentInteractiveCompletedPayloadEventTypeAiagentInteractiveCompleted,
		EventID:   dataString(event.Data, "event_id"),
		SessionID: event.SessionID,
	}
	if reason, ok := event.Data["reason"].(string); ok {
		payload.Reason.SetTo(reason)
	}
	return ogenclient.NewAIAgentInteractiveCompletedPayloadAuditEventRequestEventData(payload)
}

func buildInteractiveK8sCallPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentInteractiveK8sCallPayload{
		EventType:  ogenclient.AIAgentInteractiveK8sCallPayloadEventTypeAiagentInteractiveK8sCall,
		EventID:    dataString(event.Data, "event_id"),
		SessionID:  event.SessionID,
		ActingUser: event.ActingUser,
		Resource:   dataString(event.Data, "resource"),
		Verb:       dataString(event.Data, "verb"),
	}
	if ns := dataString(event.Data, "namespace"); ns != "" {
		payload.Namespace.SetTo(ns)
	}
	if name := dataString(event.Data, "resource_name"); name != "" {
		payload.ResourceName.SetTo(name)
	}
	if code := dataInt(event.Data, "http_status_code"); code > 0 {
		payload.HTTPStatusCode.SetTo(code)
	}
	if cid := event.CorrelationID; cid != "" {
		payload.CorrelationID.SetTo(cid)
	}
	return ogenclient.NewAIAgentInteractiveK8sCallPayloadAuditEventRequestEventData(payload)
}

func buildSecretAccessedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	verb := ogenclient.AIAgentSecretAccessedPayloadVerbGet
	if dataString(event.Data, "verb") == "list" {
		verb = ogenclient.AIAgentSecretAccessedPayloadVerbList
	}
	payload := ogenclient.AIAgentSecretAccessedPayload{
		EventType: ogenclient.AIAgentSecretAccessedPayloadEventTypeAiagentSecretAccessed,
		EventID:   dataString(event.Data, "event_id"),
		Verb:      verb,
	}
	if ns := dataString(event.Data, "namespace"); ns != "" {
		payload.Namespace.SetTo(ns)
	}
	if name := dataString(event.Data, "secret_name"); name != "" {
		payload.SecretName.SetTo(name)
	}
	if toolName := dataString(event.Data, "tool_name"); toolName != "" {
		payload.ToolName.SetTo(toolName)
	}
	if detail := dataString(event.Data, "outcome_detail"); detail != "" {
		payload.OutcomeDetail.SetTo(detail)
	}
	return ogenclient.NewAIAgentSecretAccessedPayloadAuditEventRequestEventData(payload)
}

func buildShadowLLMRequestPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.ShadowLLMRequestPayload{
		EventType:    ogenclient.ShadowLLMRequestPayloadEventTypeAiagentShadowLlmRequest,
		EventID:      dataString(event.Data, "event_id"),
		IncidentID:   event.CorrelationID,
		StepIndex:    dataInt(event.Data, "step_index"),
		StepKind:     dataString(event.Data, "step_kind"),
		PromptLength: dataInt(event.Data, "prompt_length"),
	}
	return ogenclient.NewShadowLLMRequestPayloadAuditEventRequestEventData(payload)
}

func buildShadowLLMResponsePayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.ShadowLLMResponsePayload{
		EventType:        ogenclient.ShadowLLMResponsePayloadEventTypeAiagentShadowLlmResponse,
		EventID:          dataString(event.Data, "event_id"),
		IncidentID:       event.CorrelationID,
		StepIndex:        dataInt(event.Data, "step_index"),
		StepKind:         dataString(event.Data, "step_kind"),
		PromptTokens:     dataInt(event.Data, "prompt_tokens"),
		CompletionTokens: dataInt(event.Data, "completion_tokens"),
		TotalTokens:      dataInt(event.Data, "total_tokens"),
	}
	if attempt := dataInt(event.Data, "attempt"); attempt > 0 {
		payload.Attempt.SetTo(attempt)
	}
	if er := dataString(event.Data, "evaluation_result"); er != "" {
		payload.EvaluationResult.SetTo(ogenclient.ShadowLLMResponsePayloadEvaluationResult(er))
	}
	return ogenclient.NewShadowLLMResponsePayloadAuditEventRequestEventData(payload)
}

func buildRateLimitDeniedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentRatelimitDeniedPayload{
		EventType: ogenclient.AIAgentRatelimitDeniedPayloadEventTypeAiagentRatelimitDenied,
		EventID:   dataString(event.Data, "event_id"),
	}
	if v := dataString(event.Data, "source_ip"); v != "" {
		payload.SourceIP.SetTo(v)
	}
	if v := dataString(event.Data, "path"); v != "" {
		payload.Path.SetTo(v)
	}
	if v := dataString(event.Data, "method"); v != "" {
		payload.Method.SetTo(v)
	}
	return ogenclient.NewAIAgentRatelimitDeniedPayloadAuditEventRequestEventData(payload)
}

func buildAuthFailurePayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentAuthFailurePayload{
		EventType: ogenclient.AIAgentAuthFailurePayloadEventTypeAiagentAuthFailure,
		EventID:   dataString(event.Data, "event_id"),
	}
	if v := dataString(event.Data, "source_ip"); v != "" {
		payload.SourceIP.SetTo(v)
	}
	if v := dataString(event.Data, "path"); v != "" {
		payload.Path.SetTo(v)
	}
	if v := dataString(event.Data, "method"); v != "" {
		payload.Method.SetTo(v)
	}
	return ogenclient.NewAIAgentAuthFailurePayloadAuditEventRequestEventData(payload)
}

func buildAuthDeniedPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	payload := ogenclient.AIAgentAuthDeniedPayload{
		EventType: ogenclient.AIAgentAuthDeniedPayloadEventTypeAiagentAuthDenied,
		EventID:   dataString(event.Data, "event_id"),
	}
	if v := dataString(event.Data, "source_ip"); v != "" {
		payload.SourceIP.SetTo(v)
	}
	if v := dataString(event.Data, "path"); v != "" {
		payload.Path.SetTo(v)
	}
	if v := dataString(event.Data, "method"); v != "" {
		payload.Method.SetTo(v)
	}
	return ogenclient.NewAIAgentAuthDeniedPayloadAuditEventRequestEventData(payload)
}
