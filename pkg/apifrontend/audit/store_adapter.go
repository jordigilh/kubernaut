package audit

import (
	"context"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

const eventTypePrefix = "apifrontend."

// StoreAdapter bridges the AF-internal audit.Emitter interface to the shared
// pkg/audit.AuditStore, constructing per-event typed OpenAPI payloads.
type StoreAdapter struct {
	store  sharedaudit.AuditStore
	logger logr.Logger
}

// NewStoreAdapter creates a StoreAdapter wrapping the shared audit store.
func NewStoreAdapter(store sharedaudit.AuditStore, logger logr.Logger) *StoreAdapter {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	return &StoreAdapter{store: store, logger: logger.WithName("audit-adapter")}
}

// Emit converts an AF Event to an AuditEventRequest with a typed payload
// and stores it via the shared AuditStore (non-blocking).
func (a *StoreAdapter) Emit(ctx context.Context, e *Event) {
	if e == nil {
		return
	}

	req := &ogenclient.AuditEventRequest{
		Version:        "1.0",
		EventType:      eventTypePrefix + string(e.Type),
		EventTimestamp: time.Now().UTC(),
		EventCategory:  ogenclient.AuditEventRequestEventCategoryApifrontend,
		EventAction:    eventAction(e.Type),
		EventOutcome:   eventOutcome(e.Type),
		CorrelationID:  correlationID(e),
	}

	if e.UserID != "" {
		req.ActorType.SetTo("user")
		req.ActorID.SetTo(e.UserID)
	} else {
		req.ActorType.SetTo("service")
		req.ActorID.SetTo("apifrontend")
	}

	if e.SourceIP != "" {
		req.ActorIP.SetTo(e.SourceIP)
	}

	if e.ClusterID != "" {
		req.ClusterID.SetTo(e.ClusterID)
	}

	if !hasTypedPayload(e.Type) {
		a.logger.V(2).Info("skipping store for event without typed payload schema (logged only)",
			"event_type", req.EventType)
		return
	}
	req.EventData = buildEventData(e)

	if err := a.store.StoreAudit(ctx, req); err != nil {
		a.logger.Error(err, "failed to store audit event",
			"event_type", req.EventType,
			"correlation_id", req.CorrelationID)
	}
}

// Close delegates to the underlying AuditStore.
func (a *StoreAdapter) Close(_ context.Context) error {
	return a.store.Close()
}

func correlationID(e *Event) string {
	if e.CorrelationID != "" {
		return e.CorrelationID
	}
	if e.RequestID != "" {
		return e.RequestID
	}
	return uuid.New().String()
}

func detailStr(d map[string]string, key string) string {
	if d == nil {
		return ""
	}
	return d[key]
}

func detailInt(d map[string]string, key string) int {
	v, _ := strconv.Atoi(detailStr(d, key))
	return v
}

func detailStrSlice(d map[string]string, key string) []string {
	v := detailStr(d, key)
	if v == "" {
		return nil
	}
	return []string{v}
}

var typedPayloadEvents = map[EventType]bool{
	EventAuthSuccess:             true,
	EventAuthFailure:             true,
	EventRateLimitDenied:         true,
	EventCircuitBreakerTrip:      true,
	EventJWTDelegation:           true,
	EventSessionCreated:          true,
	EventSessionPhaseChanged:     true,
	EventSessionDeleted:          true,
	EventSessionAutoCancelled:    true,
	EventSessionRetentionDeleted: true,
	EventSessionCompleted:        true,
	EventA2ATaskStarted:          true,
	EventA2ATaskCompleted:        true,
	EventA2ATaskFailed:           true,
	EventMCPToolFailed:           true,
	EventMCPSessionInit:          true,
	EventConfigReloaded:          true,
	EventConfigRejected:          true,
	EventSeverityTriageCompleted: true,
	EventSeverityTriageFailed:    true,
	EventTriageStarted:           true,
	EventTriageCompleted:         true,
	EventRRCreated:               true,
	EventRRDeduplicated:          true,
	EventKADelegated:             true,
	EventKAResultReceived:        true,
	EventUserDecision:            true,
	EventAuthAccessDenied:        true,
	EventToolExecuted:            true,
}

func hasTypedPayload(t EventType) bool {
	return typedPayloadEvents[t]
}

var failureEvents = map[EventType]bool{
	EventAuthFailure:             true,
	EventA2ATaskFailed:           true,
	EventMCPToolFailed:           true,
	EventSeverityTriageFailed:    true,
	EventConfigRejected:          true,
	EventAuthAccessDenied:        true,
	EventRateLimitDenied:         true,
	EventCircuitBreakerTrip:      true,
	EventSessionAutoCancelled:    true,
	EventSessionRetentionDeleted: true,
}

func eventOutcome(t EventType) ogenclient.AuditEventRequestEventOutcome {
	if failureEvents[t] {
		return ogenclient.AuditEventRequestEventOutcomeFailure
	}
	return ogenclient.AuditEventRequestEventOutcomeSuccess
}

var actionMap = map[EventType]string{
	EventAuthSuccess:             "authenticated",
	EventAuthFailure:             "authentication_failed",
	EventRateLimitDenied:         "denied",
	EventSessionCreated:          "created",
	EventSessionDeleted:          "deleted",
	EventSessionPhaseChanged:     "phase_changed",
	EventSessionCompleted:        "completed",
	EventSessionAutoCancelled:    "auto_cancelled",
	EventSessionRetentionDeleted: "retention_deleted",
	EventA2ATaskStarted:          "started",
	EventA2ATaskCompleted:        "completed",
	EventA2ATaskFailed:           "failed",
	EventToolExecuted:            "executed",
	EventAuthAccessDenied:        "denied",
	EventMCPToolFailed:           "failed",
	EventMCPSessionInit:          "initialized",
	EventConfigReloaded:          "reloaded",
	EventConfigRejected:          "rejected",
	EventCircuitBreakerTrip:      "tripped",
	// EventImpersonation removed (ADR-022: impersonation deprecated)
	EventJWTDelegation:           "delegated",
	EventSeverityTriageCompleted: "completed",
	EventSeverityTriageFailed:    "failed",
	EventWorkflowDiscovery:       "discovered",
	EventTriageStarted:           "started",
	EventTriageCompleted:         "completed",
	EventRRCreated:               "created",
	EventRRDeduplicated:          "deduplicated",
	EventKADelegated:             "delegated",
	EventKAResultReceived:        "received",
	EventUserDecision:            "decided",
	EventAgentCardAccessed:       "accessed",
	EventInvestigationTimeout:    "timed_out",
}

func eventAction(t EventType) string {
	if a, ok := actionMap[t]; ok {
		return a
	}
	return string(t)
}

// eventDataBuilder constructs a typed OpenAPI event-data payload for one
// EventType. See DD-AUDIT-008 (registry/lookup-table pattern) — this mirrors
// internal/kubernautagent/audit/ds_store.go's buildEventData, applying the
// same already-approved pattern to this package's own (distinct) dispatcher.
type eventDataBuilder func(e *Event) ogenclient.AuditEventRequestEventData

var eventDataBuilders = map[EventType]eventDataBuilder{
	EventAuthSuccess:             buildAuthSuccessPayload,
	EventAuthFailure:             buildAuthFailurePayload,
	EventRateLimitDenied:         buildRateLimitDeniedPayload,
	EventCircuitBreakerTrip:      buildCircuitBreakerTripPayload,
	EventJWTDelegation:           buildJWTDelegationPayload,
	EventSessionCreated:          buildSessionCreatedPayload,
	EventSessionPhaseChanged:     buildSessionPhaseChangedPayload,
	EventSessionDeleted:          buildSessionDeletedPayload,
	EventSessionAutoCancelled:    buildSessionAutoCancelledPayload,
	EventSessionRetentionDeleted: buildSessionRetentionDeletedPayload,
	EventSessionCompleted:        buildSessionCompletedPayload,
	EventA2ATaskStarted:          buildA2ATaskStartedPayload,
	EventA2ATaskCompleted:        buildA2ATaskCompletedPayload,
	EventA2ATaskFailed:           buildA2ATaskFailedPayload,
	EventMCPToolFailed:           buildMCPToolFailedPayload,
	EventMCPSessionInit:          buildMCPSessionInitPayload,
	EventConfigReloaded:          buildConfigReloadedPayload,
	EventConfigRejected:          buildConfigRejectedPayload,
	EventSeverityTriageCompleted: buildSeverityTriageCompletedPayload,
	EventSeverityTriageFailed:    buildSeverityTriageFailedPayload,
	EventTriageStarted:           buildTriageStartedPayload,
	EventTriageCompleted:         buildTriageCompletedPayload,
	EventRRCreated:               buildRRCreatedPayload,
	EventRRDeduplicated:          buildRRDeduplicatedPayload,
	EventKADelegated:             buildKADelegatedPayload,
	EventKAResultReceived:        buildKAResultReceivedPayload,
	EventUserDecision:            buildUserDecisionPayload,
	EventAuthAccessDenied:        buildAuthAccessDeniedPayload,
	EventToolExecuted:            buildToolExecutedPayload,
}

func buildEventData(e *Event) ogenclient.AuditEventRequestEventData {
	builder, ok := eventDataBuilders[e.Type]
	if !ok {
		return ogenclient.AuditEventRequestEventData{}
	}
	return builder(e)
}

func buildAuthSuccessPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendAuthSuccessPayloadAuditEventRequestEventData(ogenclient.ApifrontendAuthSuccessPayload{
		EventType:  ogenclient.ApifrontendAuthSuccessPayloadEventTypeApifrontendAuthSuccess,
		AuthMethod: ogenclient.ApifrontendAuthSuccessPayloadAuthMethod(detailStr(d, "auth_method")),
		Issuer:     ogenclient.NewOptString(detailStr(d, "issuer")),
		Groups:     detailStrSlice(d, "groups"),
	})
}

func buildAuthFailurePayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendAuthFailurePayloadAuditEventRequestEventData(ogenclient.ApifrontendAuthFailurePayload{
		EventType:     ogenclient.ApifrontendAuthFailurePayloadEventTypeApifrontendAuthFailure,
		AuthMethod:    ogenclient.ApifrontendAuthFailurePayloadAuthMethod(detailStr(d, "auth_method")),
		FailureReason: detailStr(d, "failure_reason"),
	})
}

func buildRateLimitDeniedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendRatelimitDeniedPayloadAuditEventRequestEventData(ogenclient.ApifrontendRatelimitDeniedPayload{
		EventType:  ogenclient.ApifrontendRatelimitDeniedPayloadEventTypeApifrontendRatelimitDenied,
		LimitType:  detailStr(d, "limit_type"),
		LimitValue: ogenclient.NewOptString(detailStr(d, "limit_value")),
	})
}

func buildCircuitBreakerTripPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendCircuitbreakerTripPayloadAuditEventRequestEventData(ogenclient.ApifrontendCircuitbreakerTripPayload{
		EventType:    ogenclient.ApifrontendCircuitbreakerTripPayloadEventTypeApifrontendCircuitbreakerTrip,
		CircuitName:  detailStr(d, "circuit_name"),
		FailureCount: detailInt(d, "failure_count"),
	})
}

func buildJWTDelegationPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendJWTDelegationPayloadAuditEventRequestEventData(ogenclient.ApifrontendJWTDelegationPayload{
		EventType:     ogenclient.ApifrontendJWTDelegationPayloadEventTypeApifrontendJwtDelegation,
		TargetService: detailStr(d, "target_service"),
	})
}

func buildSessionCreatedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendSessionCreatedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionCreatedPayload{
		EventType:    ogenclient.ApifrontendSessionCreatedPayloadEventTypeApifrontendSessionCreated,
		SessionID:    detailStr(d, "session_id"),
		A2aTaskID:    detailStr(d, "a2a_task_id"),
		JoinMode:     ogenclient.ApifrontendSessionCreatedPayloadJoinMode(detailStr(d, "join_mode")),
		UserIdentity: detailStr(d, "user_identity"),
		RrRef:        ogenclient.NewOptString(detailStr(d, "rr_ref")),
	})
}

func buildSessionPhaseChangedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendSessionPhaseChangedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionPhaseChangedPayload{
		EventType: ogenclient.ApifrontendSessionPhaseChangedPayloadEventTypeApifrontendSessionPhaseChanged,
		SessionID: detailStr(d, "session_id"),
		FromPhase: detailStr(d, "from_phase"),
		ToPhase:   detailStr(d, "to_phase"),
	})
}

func buildSessionDeletedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendSessionDeletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionDeletedPayload{
		EventType: ogenclient.ApifrontendSessionDeletedPayloadEventTypeApifrontendSessionDeleted,
		SessionID: detailStr(d, "session_id"),
		Reason:    ogenclient.NewOptString(detailStr(d, "reason")),
	})
}

func buildSessionAutoCancelledPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendSessionAutoCancelledPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionAutoCancelledPayload{
		EventType: ogenclient.ApifrontendSessionAutoCancelledPayloadEventTypeApifrontendSessionAutoCancelled,
		SessionID: detailStr(d, "session_id"),
	})
}

func buildSessionRetentionDeletedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendSessionRetentionDeletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionRetentionDeletedPayload{
		EventType: ogenclient.ApifrontendSessionRetentionDeletedPayloadEventTypeApifrontendSessionRetentionDeleted,
		SessionID: detailStr(d, "session_id"),
	})
}

func buildSessionCompletedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendSessionCompletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionCompletedPayload{
		EventType:       ogenclient.ApifrontendSessionCompletedPayloadEventTypeApifrontendSessionCompleted,
		SessionID:       detailStr(d, "session_id"),
		TerminalPhase:   ogenclient.ApifrontendSessionCompletedPayloadTerminalPhase(detailStr(d, "terminal_phase")),
		TotalDurationMs: detailInt(d, "total_duration_ms"),
	})
}

func buildA2ATaskStartedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendA2ATaskStartedPayloadAuditEventRequestEventData(ogenclient.ApifrontendA2ATaskStartedPayload{
		EventType: ogenclient.ApifrontendA2ATaskStartedPayloadEventTypeApifrontendA2aTaskStarted,
		SessionID: detailStr(d, "session_id"),
		TaskID:    detailStr(d, "task_id"),
	})
}

func buildA2ATaskCompletedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	payload := ogenclient.ApifrontendA2ATaskCompletedPayload{
		EventType: ogenclient.ApifrontendA2ATaskCompletedPayloadEventTypeApifrontendA2aTaskCompleted,
		SessionID: detailStr(d, "session_id"),
		TaskID:    detailStr(d, "task_id"),
	}
	if v := detailStr(d, "rr_name"); v != "" {
		payload.RrName = ogenclient.NewOptString(v)
	}
	if v := detailStr(d, "rr_namespace"); v != "" {
		payload.RrNamespace = ogenclient.NewOptString(v)
	}
	return ogenclient.NewApifrontendA2ATaskCompletedPayloadAuditEventRequestEventData(payload)
}

func buildA2ATaskFailedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	payload := ogenclient.ApifrontendA2ATaskFailedPayload{
		EventType: ogenclient.ApifrontendA2ATaskFailedPayloadEventTypeApifrontendA2aTaskFailed,
		SessionID: detailStr(d, "session_id"),
		TaskID:    detailStr(d, "task_id"),
		Error:     detailStr(d, "error"),
	}
	if v := detailStr(d, "rr_name"); v != "" {
		payload.RrName = ogenclient.NewOptString(v)
	}
	if v := detailStr(d, "rr_namespace"); v != "" {
		payload.RrNamespace = ogenclient.NewOptString(v)
	}
	return ogenclient.NewApifrontendA2ATaskFailedPayloadAuditEventRequestEventData(payload)
}

func buildMCPToolFailedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendMCPToolFailedPayloadAuditEventRequestEventData(ogenclient.ApifrontendMCPToolFailedPayload{
		EventType: ogenclient.ApifrontendMCPToolFailedPayloadEventTypeApifrontendMcpToolFailed,
		ToolName:  detailStr(d, "tool_name"),
		Error:     detailStr(d, "error"),
		SessionID: ogenclient.NewOptString(detailStr(d, "session_id")),
	})
}

func buildMCPSessionInitPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendMCPSessionInitPayloadAuditEventRequestEventData(ogenclient.ApifrontendMCPSessionInitPayload{
		EventType:       ogenclient.ApifrontendMCPSessionInitPayloadEventTypeApifrontendMcpSessionInit,
		McpSessionID:    detailStr(d, "mcp_session_id"),
		ProtocolVersion: ogenclient.NewOptString(detailStr(d, "protocol_version")),
	})
}

func buildConfigReloadedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendConfigReloadedPayloadAuditEventRequestEventData(ogenclient.ApifrontendConfigReloadedPayload{
		EventType:     ogenclient.ApifrontendConfigReloadedPayloadEventTypeApifrontendConfigReloaded,
		ConfigVersion: detailStr(d, "config_version"),
	})
}

func buildConfigRejectedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendConfigRejectedPayloadAuditEventRequestEventData(ogenclient.ApifrontendConfigRejectedPayload{
		EventType:       ogenclient.ApifrontendConfigRejectedPayloadEventTypeApifrontendConfigRejected,
		RejectionReason: detailStr(d, "rejection_reason"),
	})
}

func buildSeverityTriageCompletedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendSeverityTriageCompletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSeverityTriageCompletedPayload{
		EventType:  ogenclient.ApifrontendSeverityTriageCompletedPayloadEventTypeApifrontendSeverityTriageCompleted,
		Severity:   detailStr(d, "severity"),
		SourceTier: detailStr(d, "source_tier"),
	})
}

func buildSeverityTriageFailedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendSeverityTriageFailedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSeverityTriageFailedPayload{
		EventType:  ogenclient.ApifrontendSeverityTriageFailedPayloadEventTypeApifrontendSeverityTriageFailed,
		Error:      detailStr(d, "error"),
		FailedTier: ogenclient.NewOptString(detailStr(d, "failed_tier")),
	})
}

func buildTriageStartedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendTriageStartedPayloadAuditEventRequestEventData(ogenclient.ApifrontendTriageStartedPayload{
		EventType: ogenclient.ApifrontendTriageStartedPayloadEventTypeApifrontendTriageStarted,
		SessionID: detailStr(d, "session_id"),
		Persona:   ogenclient.ApifrontendTriageStartedPayloadPersona(detailStr(d, "persona")),
	})
}

func buildTriageCompletedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendTriageCompletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendTriageCompletedPayload{
		EventType:        ogenclient.ApifrontendTriageCompletedPayloadEventTypeApifrontendTriageCompleted,
		SessionID:        detailStr(d, "session_id"),
		TriageOutcome:    ogenclient.ApifrontendTriageCompletedPayloadTriageOutcome(detailStr(d, "triage_outcome")),
		TriageDurationMs: detailInt(d, "triage_duration_ms"),
	})
}

func buildRRCreatedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendRRCreatedPayloadAuditEventRequestEventData(ogenclient.ApifrontendRRCreatedPayload{
		EventType:   ogenclient.ApifrontendRRCreatedPayloadEventTypeApifrontendRrCreated,
		SessionID:   detailStr(d, "session_id"),
		RrName:      detailStr(d, "rr_name"),
		RrNamespace: detailStr(d, "rr_namespace"),
		Fingerprint: detailStr(d, "fingerprint"),
	})
}

func buildRRDeduplicatedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendRRDeduplicatedPayloadAuditEventRequestEventData(ogenclient.ApifrontendRRDeduplicatedPayload{
		EventType:      ogenclient.ApifrontendRRDeduplicatedPayloadEventTypeApifrontendRrDeduplicated,
		SessionID:      detailStr(d, "session_id"),
		Fingerprint:    detailStr(d, "fingerprint"),
		ExistingRrName: detailStr(d, "existing_rr_name"),
	})
}

func buildKADelegatedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendKADelegatedPayloadAuditEventRequestEventData(ogenclient.ApifrontendKADelegatedPayload{
		EventType:       ogenclient.ApifrontendKADelegatedPayloadEventTypeApifrontendKaDelegated,
		SessionID:       detailStr(d, "session_id"),
		KaCorrelationID: detailStr(d, "ka_correlation_id"),
		DelegationType:  ogenclient.ApifrontendKADelegatedPayloadDelegationType(detailStr(d, "delegation_type")),
	})
}

func buildKAResultReceivedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendKAResultReceivedPayloadAuditEventRequestEventData(ogenclient.ApifrontendKAResultReceivedPayload{
		EventType:       ogenclient.ApifrontendKAResultReceivedPayloadEventTypeApifrontendKaResultReceived,
		SessionID:       detailStr(d, "session_id"),
		KaCorrelationID: detailStr(d, "ka_correlation_id"),
		ResultType:      ogenclient.ApifrontendKAResultReceivedPayloadResultType(detailStr(d, "result_type")),
	})
}

func buildUserDecisionPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendUserDecisionPayloadAuditEventRequestEventData(ogenclient.ApifrontendUserDecisionPayload{
		EventType:  ogenclient.ApifrontendUserDecisionPayloadEventTypeApifrontendUserDecision,
		SessionID:  detailStr(d, "session_id"),
		Decision:   ogenclient.ApifrontendUserDecisionPayloadDecision(detailStr(d, "decision")),
		WorkflowID: ogenclient.NewOptString(detailStr(d, "workflow_id")),
	})
}

func buildAuthAccessDeniedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	return ogenclient.NewApifrontendAuthAccessDeniedPayloadAuditEventRequestEventData(ogenclient.ApifrontendAuthAccessDeniedPayload{
		EventType: ogenclient.ApifrontendAuthAccessDeniedPayloadEventTypeApifrontendAuthAccessDenied,
		ToolName:  detailStr(d, "tool_name"),
		UserRole:  detailStr(d, "user_role"),
		Endpoint:  ogenclient.ApifrontendAuthAccessDeniedPayloadEndpoint(detailStr(d, "endpoint")),
	})
}

func buildToolExecutedPayload(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail
	payload := ogenclient.ApifrontendToolExecutedPayload{
		EventType:           ogenclient.ApifrontendToolExecutedPayloadEventTypeApifrontendToolExecuted,
		SessionID:           detailStr(d, "session_id"),
		ToolName:            detailStr(d, "tool_name"),
		ExecutionDurationMs: detailInt(d, "execution_duration_ms"),
		ToolOutcome:         ogenclient.ApifrontendToolExecutedPayloadToolOutcome(detailStr(d, "tool_outcome")),
	}
	if v := detailStr(d, "target_resource"); v != "" {
		payload.TargetResource = ogenclient.NewOptString(v)
	}
	if v := detailStr(d, "target_namespace"); v != "" {
		payload.TargetNamespace = ogenclient.NewOptString(v)
	}
	if v := detailStr(d, "target_kind"); v != "" {
		payload.TargetKind = ogenclient.NewOptString(v)
	}
	if v := detailStr(d, "error_code"); v != "" {
		payload.ErrorCode = ogenclient.NewOptString(v)
	}
	return ogenclient.NewApifrontendToolExecutedPayloadAuditEventRequestEventData(payload)
}
