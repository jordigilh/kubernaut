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
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-faster/jx"
	"github.com/go-logr/logr"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// AuditCreator is the subset of the ogen client needed by DSAuditStore.
type AuditCreator interface {
	CreateAuditEvent(ctx context.Context, req *ogenclient.AuditEventRequest) (ogenclient.CreateAuditEventRes, error)
}

// DSAuditStore implements AuditStore by sending events to the DataStorage API.
type DSAuditStore struct {
	client AuditCreator
}

// NewDSAuditStore creates a DSAuditStore backed by the given ogen client.
func NewDSAuditStore(client AuditCreator) *DSAuditStore {
	return &DSAuditStore{client: client}
}

func (s *DSAuditStore) StoreAudit(ctx context.Context, event *AuditEvent) error {
	req := &ogenclient.AuditEventRequest{
		Version:        "1.0",
		EventType:      event.EventType,
		EventTimestamp: time.Now().UTC(),
		EventCategory:  ogenclient.AuditEventRequestEventCategory(event.EventCategory),
		EventAction:    event.EventAction,
		EventOutcome:   ogenclient.AuditEventRequestEventOutcome(event.EventOutcome),
		CorrelationID:  event.CorrelationID,
	}
	req.ActorType.SetTo("Service")
	req.ActorID.SetTo("kubernaut-agent")
	if event.ParentEventID != nil {
		req.ParentEventID.SetTo(*event.ParentEventID)
	}

	if ed, ok := buildEventData(event); ok {
		req.EventData = ed
	}

	if _, err := s.client.CreateAuditEvent(ctx, req); err != nil {
		return fmt.Errorf("audit store: %w", err)
	}
	return nil
}

func buildEventData(event *AuditEvent) (ogenclient.AuditEventRequestEventData, bool) {
	switch event.EventType {
	case EventTypeEnrichmentCompleted:
		payload := ogenclient.AIAgentEnrichmentCompletedPayload{
			EventType:  ogenclient.AIAgentEnrichmentCompletedPayloadEventTypeAiagentEnrichmentCompleted,
			EventID:    dataString(event.Data, "event_id"),
			IncidentID: dataString(event.Data, "incident_id"),
			RootOwnerKind:   dataString(event.Data, "root_owner_kind"),
			RootOwnerName:   dataString(event.Data, "root_owner_name"),
			OwnerChainLength: dataInt(event.Data, "owner_chain_length"),
			RemediationHistoryFetched: dataBool(event.Data, "remediation_history_fetched"),
		}
		if ns := dataString(event.Data, "root_owner_namespace"); ns != "" {
			payload.RootOwnerNamespace.SetTo(ns)
		}
		return ogenclient.NewAIAgentEnrichmentCompletedPayloadAuditEventRequestEventData(payload), true

	case EventTypeEnrichmentFailed:
		payload := ogenclient.AIAgentEnrichmentFailedPayload{
			EventType:  ogenclient.AIAgentEnrichmentFailedPayloadEventTypeAiagentEnrichmentFailed,
			EventID:    dataString(event.Data, "event_id"),
			IncidentID: dataString(event.Data, "incident_id"),
			Reason:     dataString(event.Data, "reason"),
			Detail:     dataString(event.Data, "detail"),
			AffectedResourceKind: dataString(event.Data, "affected_resource_kind"),
			AffectedResourceName: dataString(event.Data, "affected_resource_name"),
		}
		if ns := dataString(event.Data, "affected_resource_namespace"); ns != "" {
			payload.AffectedResourceNamespace.SetTo(ns)
		}
		return ogenclient.NewAIAgentEnrichmentFailedPayloadAuditEventRequestEventData(payload), true

	case EventTypeLLMRequest:
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
		return ogenclient.NewLLMRequestPayloadAuditEventRequestEventData(payload), true

	case EventTypeLLMResponse:
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
		return ogenclient.NewLLMResponsePayloadAuditEventRequestEventData(payload), true

	case EventTypeLLMToolCall:
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
		if result := dataString(event.Data, "tool_result"); result != "" {
			payload.ToolResult = toJxRaw(result)
		}
		if preview := dataString(event.Data, "tool_result_preview"); preview != "" {
			payload.ToolResultPreview.SetTo(truncate(preview, previewMaxLen))
		}
		return ogenclient.NewLLMToolCallPayloadAuditEventRequestEventData(payload), true

	case EventTypeConversationTurn:
		payload := ogenclient.ConversationTurnPayload{
			EventType: ogenclient.ConversationTurnPayloadEventTypeAiagentConversationTurn,
			EventID:   dataString(event.Data, "event_id"),
			SessionID: dataString(event.Data, "session_id"),
			UserID:    dataString(event.Data, "user_id"),
			Question:  dataString(event.Data, "question"),
		}
		if answer := dataString(event.Data, "answer"); answer != "" {
			payload.Answer.SetTo(answer)
		}
		if tn := dataInt(event.Data, "turn_number"); tn > 0 {
			payload.TurnNumber.SetTo(tn)
		}
		return ogenclient.NewConversationTurnPayloadAuditEventRequestEventData(payload), true

	case EventTypeValidationAttempt:
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
		return ogenclient.NewWorkflowValidationPayloadAuditEventRequestEventData(payload), true

	case EventTypeResponseComplete:
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
		return ogenclient.NewAIAgentResponsePayloadAuditEventRequestEventData(payload), true

	case EventTypeRCAComplete:
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
		return ogenclient.NewAIAgentRCACompletePayloadAuditEventRequestEventData(payload), true

	case EventTypeResponseFailed:
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
		return ogenclient.NewAIAgentResponseFailedPayloadAuditEventRequestEventData(payload), true

	default:
		return ogenclient.AuditEventRequestEventData{}, false
	}
}

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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func toJxRaw(s string) jx.Raw {
	if json.Valid([]byte(s)) {
		return jx.Raw(s)
	}
	b, _ := json.Marshal(s)
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
	RCASummary           string                       `json:"rca_summary"`
	Severity             string                       `json:"severity"`
	ContributingFactors  []string                     `json:"contributing_factors"`
	WorkflowID           string                       `json:"workflow_id"`
	ExecutionBundle      string                       `json:"execution_bundle"`
	Confidence           float64                      `json:"confidence"`
	NeedsHumanReview     bool                         `json:"needs_human_review"`
	HumanReviewReason    string                       `json:"human_review_reason"`
	Warnings             []string                     `json:"warnings"`
	Parameters           map[string]interface{}        `json:"parameters"`
	AlternativeWorkflows []altWorkflowJSON            `json:"alternative_workflows"`
	RemediationTarget    *remediationTargetJSON       `json:"remediation_target"`
	CausalChain          []string                     `json:"causal_chain"`
	DueDiligence         *dueDiligenceJSON            `json:"due_diligence"`
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
			Summary:            ir.RCASummary,
			Severity:           mapSeverity(ir.Severity),
			ContributingFactors: ir.ContributingFactors,
		},
	}

	if ir.RemediationTarget != nil {
		var rt ogenclient.IncidentResponseDataRootCauseAnalysisRemediationTarget
		if ir.RemediationTarget.Kind != "" {
			rt.Kind.SetTo(ir.RemediationTarget.Kind)
		}
		if ir.RemediationTarget.Name != "" {
			rt.Name.SetTo(ir.RemediationTarget.Name)
		}
		if ir.RemediationTarget.Namespace != "" {
			rt.Namespace.SetTo(ir.RemediationTarget.Namespace)
		}
		data.RootCauseAnalysis.RemediationTarget.SetTo(rt)
	}

	if len(ir.CausalChain) > 0 {
		data.RootCauseAnalysis.CausalChain = ir.CausalChain
	}
	if ir.DueDiligence != nil {
		dd := ogenclient.IncidentResponseDataRootCauseAnalysisDueDiligence{}
		if ir.DueDiligence.CausalCompleteness != "" {
			dd.CausalCompleteness.SetTo(ir.DueDiligence.CausalCompleteness)
		}
		if ir.DueDiligence.TargetAccuracy != "" {
			dd.TargetAccuracy.SetTo(ir.DueDiligence.TargetAccuracy)
		}
		if ir.DueDiligence.EvidenceSufficiency != "" {
			dd.EvidenceSufficiency.SetTo(ir.DueDiligence.EvidenceSufficiency)
		}
		if ir.DueDiligence.AlternativeHypotheses != "" {
			dd.AlternativeHypotheses.SetTo(ir.DueDiligence.AlternativeHypotheses)
		}
		if ir.DueDiligence.ScopeCompleteness != "" {
			dd.ScopeCompleteness.SetTo(ir.DueDiligence.ScopeCompleteness)
		}
		if ir.DueDiligence.Proportionality != "" {
			dd.Proportionality.SetTo(ir.DueDiligence.Proportionality)
		}
		if ir.DueDiligence.RegressionAwareness != "" {
			dd.RegressionAwareness.SetTo(ir.DueDiligence.RegressionAwareness)
		}
		if ir.DueDiligence.ConfidenceCalibration != "" {
			dd.ConfidenceCalibration.SetTo(ir.DueDiligence.ConfidenceCalibration)
		}
		data.RootCauseAnalysis.DueDiligence.SetTo(dd)
	}

	if ir.WorkflowID != "" {
		sw := ogenclient.IncidentResponseDataSelectedWorkflow{}
		sw.WorkflowId.SetTo(ir.WorkflowID)
		if ir.ExecutionBundle != "" {
			sw.ExecutionBundle.SetTo(ir.ExecutionBundle)
		}
		sw.Confidence.SetTo(float32(ir.Confidence))
		if len(ir.Parameters) > 0 {
			params := make(ogenclient.IncidentResponseDataSelectedWorkflowParameters, len(ir.Parameters))
			for k, v := range ir.Parameters {
				b, _ := json.Marshal(v)
				params[k] = jx.Raw(b)
			}
			sw.Parameters.SetTo(params)
		}
		data.SelectedWorkflow.SetTo(sw)
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

	for _, alt := range ir.AlternativeWorkflows {
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
		data.AlternativeWorkflows = append(data.AlternativeWorkflows, item)
	}

	return data
}

func mapSeverity(s string) ogenclient.IncidentResponseDataRootCauseAnalysisSeverity {
	switch s {
	case "critical":
		return ogenclient.IncidentResponseDataRootCauseAnalysisSeverityCritical
	case "high":
		return ogenclient.IncidentResponseDataRootCauseAnalysisSeverityHigh
	case "medium":
		return ogenclient.IncidentResponseDataRootCauseAnalysisSeverityMedium
	case "low":
		return ogenclient.IncidentResponseDataRootCauseAnalysisSeverityLow
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
}

// validHumanReviewReason returns the ogen enum value if the string is a recognised
// HumanReviewReason. Free-text values from the LLM are rejected so that the
// ogen client-side OpenAPI validation does not drop the entire audit event.
func validHumanReviewReason(s string) (ogenclient.IncidentResponseDataHumanReviewReason, bool) {
	v, ok := validHumanReviewReasons[s]
	return v, ok
}

var _ AuditStore = (*DSAuditStore)(nil)

// BufferedDSAuditStore wraps pkg/audit.BufferedAuditStore to implement KA's
// internal AuditStore interface. Events are converted from KA's AuditEvent
// format to the OpenAPI AuditEventRequest format, then enqueued into the
// platform buffered store for batched writes with retry.
//
// Uses the same OpenAPIClientAdapter + BufferedAuditStore stack as every other
// platform service (DD-AUDIT-002 alignment).
type BufferedDSAuditStore struct {
	inner sharedaudit.AuditStore
}

// BufferedDSAuditStoreOption allows callers to override RecommendedConfig fields.
type BufferedDSAuditStoreOption func(*sharedaudit.Config)

// WithFlushInterval overrides the default flush interval from RecommendedConfig.
func WithFlushInterval(d time.Duration) BufferedDSAuditStoreOption {
	return func(c *sharedaudit.Config) {
		if d > 0 {
			c.FlushInterval = d
		}
	}
}

// WithBufferSize overrides the default buffer size from RecommendedConfig.
func WithBufferSize(n int) BufferedDSAuditStoreOption {
	return func(c *sharedaudit.Config) {
		if n > 0 {
			c.BufferSize = n
		}
	}
}

// WithBatchSize overrides the default batch size from RecommendedConfig.
func WithBatchSize(n int) BufferedDSAuditStoreOption {
	return func(c *sharedaudit.Config) {
		if n > 0 {
			c.BatchSize = n
		}
	}
}

// NewBufferedDSAuditStore creates a KA audit store backed by the platform
// BufferedAuditStore. The caller provides a DataStorageClient (typically
// created via audit.NewOpenAPIClientAdapterWithTransport to share auth
// transport with the rest of KA's DS operations).
//
// Optional BufferedDSAuditStoreOption values override the RecommendedConfig
// defaults, allowing integration/E2E tests to control flush behaviour via
// the same YAML fields that KA v1.2 used (flush_interval_seconds, etc.).
func NewBufferedDSAuditStore(dsClient sharedaudit.DataStorageClient, logger logr.Logger, opts ...BufferedDSAuditStoreOption) (*BufferedDSAuditStore, error) {
	cfg := sharedaudit.RecommendedConfig("kubernaut-agent")
	for _, o := range opts {
		o(&cfg)
	}
	inner, err := sharedaudit.NewBufferedStore(dsClient, cfg, "kubernaut-agent", logger)
	if err != nil {
		return nil, fmt.Errorf("create buffered audit store: %w", err)
	}
	return &BufferedDSAuditStore{inner: inner}, nil
}

func (s *BufferedDSAuditStore) StoreAudit(ctx context.Context, event *AuditEvent) error {
	req := &ogenclient.AuditEventRequest{
		Version:        "1.0",
		EventType:      event.EventType,
		EventTimestamp: time.Now().UTC(),
		EventCategory:  ogenclient.AuditEventRequestEventCategory(event.EventCategory),
		EventAction:    event.EventAction,
		EventOutcome:   ogenclient.AuditEventRequestEventOutcome(event.EventOutcome),
		CorrelationID:  event.CorrelationID,
	}
	req.ActorType.SetTo("Service")
	req.ActorID.SetTo("kubernaut-agent")

	if ed, ok := buildEventData(event); ok {
		req.EventData = ed
	}

	return s.inner.StoreAudit(ctx, req)
}

// Flush forces all buffered events to be written to DataStorage.
func (s *BufferedDSAuditStore) Flush(ctx context.Context) error {
	return s.inner.Flush(ctx)
}

// Close flushes remaining events and stops the background worker.
func (s *BufferedDSAuditStore) Close() error {
	return s.inner.Close()
}

var _ AuditStore = (*BufferedDSAuditStore)(nil)
