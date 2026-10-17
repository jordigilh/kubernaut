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

import ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

const (
	decisionLedgerStringMaxLen                  = 128
	decisionLedgerModelMaxLen                   = 253
	decisionLedgerMaxRelated                    = 32
	decisionPhaseRCA                            = "rca"
	decisionPhaseWorkflowDiscovery              = "workflow_discovery"
	decisionOutcomeEvidenceTransportUnavailable = "evidence_transport_unavailable"
)

// buildInvestigationDecisionPayload maps only the allow-listed decision
// fields. Arbitrary event.Data keys are intentionally ignored so prompts,
// private reasoning, credentials, raw transport errors, and unrestricted tool
// arguments/results cannot enter the persisted decision ledger.
func buildInvestigationDecisionPayload(event *AuditEvent) ogenclient.AuditEventRequestEventData {
	phase := decisionPhase(dataString(event.Data, "phase"))
	turnKind := decisionTurnKind(dataString(event.Data, "turn_kind"))
	historyStatus := decisionHistoryStatus(dataString(event.Data, "history_status"))
	terminalOutcome := decisionTerminalOutcome(dataString(event.Data, "terminal_outcome"))
	discoveryStatus := decisionDiscoveryStatus(dataString(event.Data, "workflow_discovery_status"))

	payload := ogenclient.AIAgentInvestigationDecisionPayload{
		EventType:               ogenclient.AIAgentInvestigationDecisionPayloadEventTypeAiagentInvestigationDecision,
		EventID:                 safeLedgerString(dataString(event.Data, "event_id"), decisionLedgerStringMaxLen),
		IncidentID:              safeLedgerString(event.CorrelationID, decisionLedgerStringMaxLen),
		SessionID:               optionalLedgerString(event.SessionID, decisionLedgerStringMaxLen),
		TurnID:                  optionalLedgerString(dataString(event.Data, "turn_id"), decisionLedgerStringMaxLen),
		Phase:                   phase,
		TurnKind:                turnKind,
		HistoryStatus:           historyStatus,
		TerminalOutcome:         terminalOutcome,
		WorkflowDiscoveryStatus: discoveryStatus,
		RelatedEventIds:         boundedRelatedEventIDs(event.Data["related_event_ids"]),
	}

	setDecisionOptionalStrings(&payload, event)
	setDecisionTokenCounts(&payload, event)
	return ogenclient.NewAIAgentInvestigationDecisionPayloadAuditEventRequestEventData(payload)
}

func setDecisionOptionalStrings(payload *ogenclient.AIAgentInvestigationDecisionPayload, event *AuditEvent) {
	if v := dataString(event.Data, "model"); v != "" {
		payload.Model.SetTo(safeLedgerString(v, decisionLedgerModelMaxLen))
	}
	for key, dst := range map[string]*ogenclient.OptString{
		"evidence_status":         &payload.EvidenceStatus,
		"target_status":           &payload.TargetStatus,
		"causal_mechanism_status": &payload.CausalMechanismStatus,
		"continuation_reason":     &payload.ContinuationReason,
		"human_review_reason":     &payload.HumanReviewReason,
		"failure_class":           &payload.FailureClass,
	} {
		if v, ok := decisionOptionalString(key, dataString(event.Data, key)); ok {
			dst.SetTo(v)
		}
	}
}

// decisionOptionalString keeps producer-controlled values enum-like at the
// persistence boundary. Field names alone are not enough: a model-supplied
// human-review reason or a raw provider error placed under an allowed key
// would still violate the bounded decision-ledger contract.
func decisionOptionalString(key, value string) (string, bool) {
	if value == "" {
		return "", false
	}
	var allowed map[string]struct{}
	switch key {
	case "evidence_status":
		allowed = map[string]struct{}{
			"available": {}, "empty": {}, "transport_unavailable": {},
			"schema_error": {}, "not_requested": {},
		}
	case "target_status":
		allowed = map[string]struct{}{"identified": {}, "not_identified": {}, "not_evaluated": {}}
	case "causal_mechanism_status":
		allowed = map[string]struct{}{"verified": {}, "probable": {}, "unknown": {}, "not_assessed": {}}
	case "continuation_reason":
		allowed = map[string]struct{}{
			"rca_complete": {}, "workflow_discovery_started": {}, "investigation_complete": {},
			"investigation_cancelled": {}, "context_cancelled": {}, "sentinel_submit": {},
			"tool_budget_exhausted": {}, "tool_turn_complete": {}, "response_truncated_retry": {},
			"output_truncated_after_retry": {}, "text_response": {}, "llm_call_failed": {},
			"turn_budget_exhausted": {},
		}
	case "human_review_reason":
		allowed = map[string]struct{}{
			"workflow_not_found": {}, "image_mismatch": {}, "parameter_validation_failed": {},
			"no_matching_workflows": {}, "low_confidence": {}, "llm_parsing_error": {},
			"investigation_inconclusive": {}, "rca_incomplete": {}, "alignment_check_failed": {},
			"operator_escalation": {}, "decision_expired": {}, "catalog_unavailable": {},
		}
	case "failure_class":
		allowed = map[string]struct{}{
			"turn_budget_exhausted": {}, "llm_call_failed": {}, "tool_budget_exhausted": {},
			"output_truncated": {}, "workflow_selected": {}, "problem_resolved": {},
			"evidence_transport_unavailable": {}, "history_enrichment_unavailable": {},
			"rca_incomplete": {}, "no_matching_workflows": {}, "workflow_discovery_failed": {},
			"cancelled": {}, "validation_failed": {}, "alignment_check_failed": {},
		}
	default:
		return "", false
	}
	if _, ok := allowed[value]; !ok {
		return "", false
	}
	return safeLedgerString(value, decisionLedgerStringMaxLen), true
}

func setDecisionTokenCounts(payload *ogenclient.AIAgentInvestigationDecisionPayload, event *AuditEvent) {
	for key, dst := range map[string]*ogenclient.OptInt{
		"prompt_tokens":     &payload.PromptTokens,
		"completion_tokens": &payload.CompletionTokens,
		"total_tokens":      &payload.TotalTokens,
		"retry_attempt":     &payload.RetryAttempt,
	} {
		if _, present := event.Data[key]; present {
			dst.SetTo(maxInt(0, dataInt(event.Data, key)))
		}
	}
}

func boundedRelatedEventIDs(value interface{}) []string {
	ids, ok := value.([]string)
	if !ok || len(ids) == 0 {
		return nil
	}
	if len(ids) > decisionLedgerMaxRelated {
		ids = ids[:decisionLedgerMaxRelated]
	}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			result = append(result, safeLedgerString(id, decisionLedgerStringMaxLen))
		}
	}
	return result
}

func safeLedgerString(value string, maxLen int) string {
	return truncate(value, maxLen)
}

func decisionPhase(value string) ogenclient.AIAgentInvestigationDecisionPayloadPhase {
	switch value {
	case decisionPhaseRCA:
		return ogenclient.AIAgentInvestigationDecisionPayloadPhaseRca
	case decisionPhaseWorkflowDiscovery:
		return ogenclient.AIAgentInvestigationDecisionPayloadPhaseWorkflowDiscovery
	case "validation":
		return ogenclient.AIAgentInvestigationDecisionPayloadPhaseValidation
	default:
		return ogenclient.AIAgentInvestigationDecisionPayloadPhaseUnknown
	}
}

func decisionTurnKind(value string) ogenclient.AIAgentInvestigationDecisionPayloadTurnKind {
	switch value {
	case "tool_only":
		return ogenclient.AIAgentInvestigationDecisionPayloadTurnKindToolOnly
	case "terminal_decision":
		return ogenclient.AIAgentInvestigationDecisionPayloadTurnKindTerminalDecision
	default:
		return ogenclient.AIAgentInvestigationDecisionPayloadTurnKindStateTransition
	}
}

func decisionHistoryStatus(value string) ogenclient.AIAgentInvestigationDecisionPayloadHistoryStatus {
	switch value {
	case "available":
		return ogenclient.AIAgentInvestigationDecisionPayloadHistoryStatusAvailable
	case "empty":
		return ogenclient.AIAgentInvestigationDecisionPayloadHistoryStatusEmpty
	case "transport_unavailable":
		return ogenclient.AIAgentInvestigationDecisionPayloadHistoryStatusTransportUnavailable
	case "schema_error":
		return ogenclient.AIAgentInvestigationDecisionPayloadHistoryStatusSchemaError
	default:
		return ogenclient.AIAgentInvestigationDecisionPayloadHistoryStatusNotRequested
	}
}

func decisionTerminalOutcome(value string) ogenclient.AIAgentInvestigationDecisionPayloadTerminalOutcome {
	switch value {
	case "workflow_selected":
		return ogenclient.AIAgentInvestigationDecisionPayloadTerminalOutcomeWorkflowSelected
	case "problem_resolved":
		return ogenclient.AIAgentInvestigationDecisionPayloadTerminalOutcomeProblemResolved
	case decisionOutcomeEvidenceTransportUnavailable:
		return ogenclient.AIAgentInvestigationDecisionPayloadTerminalOutcomeEvidenceTransportUnavailable
	case "history_enrichment_unavailable":
		return ogenclient.AIAgentInvestigationDecisionPayloadTerminalOutcomeHistoryEnrichmentUnavailable
	case "rca_incomplete":
		return ogenclient.AIAgentInvestigationDecisionPayloadTerminalOutcomeRcaIncomplete
	case "no_matching_workflows":
		return ogenclient.AIAgentInvestigationDecisionPayloadTerminalOutcomeNoMatchingWorkflows
	case "workflow_discovery_failed":
		return ogenclient.AIAgentInvestigationDecisionPayloadTerminalOutcomeWorkflowDiscoveryFailed
	case "cancelled":
		return ogenclient.AIAgentInvestigationDecisionPayloadTerminalOutcomeCancelled
	default:
		return ogenclient.AIAgentInvestigationDecisionPayloadTerminalOutcomeUnknown
	}
}

func decisionDiscoveryStatus(value string) ogenclient.AIAgentInvestigationDecisionPayloadWorkflowDiscoveryStatus {
	switch value {
	case "started":
		return ogenclient.AIAgentInvestigationDecisionPayloadWorkflowDiscoveryStatusStarted
	case "completed_selected":
		return ogenclient.AIAgentInvestigationDecisionPayloadWorkflowDiscoveryStatusCompletedSelected
	case "completed_no_match":
		return ogenclient.AIAgentInvestigationDecisionPayloadWorkflowDiscoveryStatusCompletedNoMatch
	case "failed":
		return ogenclient.AIAgentInvestigationDecisionPayloadWorkflowDiscoveryStatusFailed
	case "skipped_due_to_failure":
		return ogenclient.AIAgentInvestigationDecisionPayloadWorkflowDiscoveryStatusSkippedDueToFailure
	default:
		return ogenclient.AIAgentInvestigationDecisionPayloadWorkflowDiscoveryStatusNotStarted
	}
}

func optionalLedgerString(value string, maxLen int) ogenclient.OptString {
	var result ogenclient.OptString
	if value != "" {
		result.SetTo(safeLedgerString(value, maxLen))
	}
	return result
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
