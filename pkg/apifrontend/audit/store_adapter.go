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
	EventImpersonation:           "created",
	EventJWTDelegation:           "delegated",
	EventSeverityTriageCompleted: "completed",
	EventSeverityTriageFailed:    "failed",
	EventTriageStarted:           "started",
	EventTriageCompleted:         "completed",
	EventRRCreated:               "created",
	EventRRDeduplicated:          "deduplicated",
	EventKADelegated:             "delegated",
	EventKAResultReceived:        "received",
	EventUserDecision:            "decided",
}

func eventAction(t EventType) string {
	if a, ok := actionMap[t]; ok {
		return a
	}
	return string(t)
}

//nolint:cyclop
func buildEventData(e *Event) ogenclient.AuditEventRequestEventData {
	d := e.Detail

	switch e.Type {
	case EventAuthSuccess:
		return ogenclient.NewApifrontendAuthSuccessPayloadAuditEventRequestEventData(ogenclient.ApifrontendAuthSuccessPayload{
			EventType:  ogenclient.ApifrontendAuthSuccessPayloadEventTypeApifrontendAuthSuccess,
			AuthMethod: ogenclient.ApifrontendAuthSuccessPayloadAuthMethod(detailStr(d, "auth_method")),
			Issuer:     ogenclient.NewOptString(detailStr(d, "issuer")),
			Groups:     detailStrSlice(d, "groups"),
		})

	case EventAuthFailure:
		return ogenclient.NewApifrontendAuthFailurePayloadAuditEventRequestEventData(ogenclient.ApifrontendAuthFailurePayload{
			EventType:     ogenclient.ApifrontendAuthFailurePayloadEventTypeApifrontendAuthFailure,
			AuthMethod:    ogenclient.ApifrontendAuthFailurePayloadAuthMethod(detailStr(d, "auth_method")),
			FailureReason: detailStr(d, "failure_reason"),
		})

	case EventRateLimitDenied:
		return ogenclient.NewApifrontendRatelimitDeniedPayloadAuditEventRequestEventData(ogenclient.ApifrontendRatelimitDeniedPayload{
			EventType:  ogenclient.ApifrontendRatelimitDeniedPayloadEventTypeApifrontendRatelimitDenied,
			LimitType:  detailStr(d, "limit_type"),
			LimitValue: ogenclient.NewOptString(detailStr(d, "limit_value")),
		})

	case EventCircuitBreakerTrip:
		return ogenclient.NewApifrontendCircuitbreakerTripPayloadAuditEventRequestEventData(ogenclient.ApifrontendCircuitbreakerTripPayload{
			EventType:    ogenclient.ApifrontendCircuitbreakerTripPayloadEventTypeApifrontendCircuitbreakerTrip,
			CircuitName:  detailStr(d, "circuit_name"),
			FailureCount: detailInt(d, "failure_count"),
		})

	case EventImpersonation:
		return ogenclient.NewApifrontendImpersonationCreatedPayloadAuditEventRequestEventData(ogenclient.ApifrontendImpersonationCreatedPayload{
			EventType:  ogenclient.ApifrontendImpersonationCreatedPayloadEventTypeApifrontendImpersonationCreated,
			TargetUser: detailStr(d, "target_user"),
			Groups:     detailStrSlice(d, "groups"),
		})

	case EventJWTDelegation:
		return ogenclient.NewApifrontendJWTDelegationPayloadAuditEventRequestEventData(ogenclient.ApifrontendJWTDelegationPayload{
			EventType:     ogenclient.ApifrontendJWTDelegationPayloadEventTypeApifrontendJwtDelegation,
			TargetService: detailStr(d, "target_service"),
		})

	case EventSessionCreated:
		return ogenclient.NewApifrontendSessionCreatedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionCreatedPayload{
			EventType:    ogenclient.ApifrontendSessionCreatedPayloadEventTypeApifrontendSessionCreated,
			SessionID:    detailStr(d, "session_id"),
			A2aTaskID:    detailStr(d, "a2a_task_id"),
			JoinMode:     ogenclient.ApifrontendSessionCreatedPayloadJoinMode(detailStr(d, "join_mode")),
			UserIdentity: detailStr(d, "user_identity"),
			RrRef:        ogenclient.NewOptString(detailStr(d, "rr_ref")),
		})

	case EventSessionPhaseChanged:
		return ogenclient.NewApifrontendSessionPhaseChangedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionPhaseChangedPayload{
			EventType: ogenclient.ApifrontendSessionPhaseChangedPayloadEventTypeApifrontendSessionPhaseChanged,
			SessionID: detailStr(d, "session_id"),
			FromPhase: detailStr(d, "from_phase"),
			ToPhase:   detailStr(d, "to_phase"),
		})

	case EventSessionDeleted:
		return ogenclient.NewApifrontendSessionDeletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionDeletedPayload{
			EventType: ogenclient.ApifrontendSessionDeletedPayloadEventTypeApifrontendSessionDeleted,
			SessionID: detailStr(d, "session_id"),
			Reason:    ogenclient.NewOptString(detailStr(d, "reason")),
		})

	case EventSessionAutoCancelled:
		return ogenclient.NewApifrontendSessionAutoCancelledPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionAutoCancelledPayload{
			EventType: ogenclient.ApifrontendSessionAutoCancelledPayloadEventTypeApifrontendSessionAutoCancelled,
			SessionID: detailStr(d, "session_id"),
		})

	case EventSessionRetentionDeleted:
		return ogenclient.NewApifrontendSessionRetentionDeletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionRetentionDeletedPayload{
			EventType: ogenclient.ApifrontendSessionRetentionDeletedPayloadEventTypeApifrontendSessionRetentionDeleted,
			SessionID: detailStr(d, "session_id"),
		})

	case EventSessionCompleted:
		return ogenclient.NewApifrontendSessionCompletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSessionCompletedPayload{
			EventType:       ogenclient.ApifrontendSessionCompletedPayloadEventTypeApifrontendSessionCompleted,
			SessionID:       detailStr(d, "session_id"),
			TerminalPhase:   ogenclient.ApifrontendSessionCompletedPayloadTerminalPhase(detailStr(d, "terminal_phase")),
			TotalDurationMs: detailInt(d, "total_duration_ms"),
		})

	case EventA2ATaskStarted:
		return ogenclient.NewApifrontendA2ATaskStartedPayloadAuditEventRequestEventData(ogenclient.ApifrontendA2ATaskStartedPayload{
			EventType: ogenclient.ApifrontendA2ATaskStartedPayloadEventTypeApifrontendA2aTaskStarted,
			SessionID: detailStr(d, "session_id"),
			TaskID:    detailStr(d, "task_id"),
		})

	case EventA2ATaskCompleted:
		return ogenclient.NewApifrontendA2ATaskCompletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendA2ATaskCompletedPayload{
			EventType: ogenclient.ApifrontendA2ATaskCompletedPayloadEventTypeApifrontendA2aTaskCompleted,
			SessionID: detailStr(d, "session_id"),
			TaskID:    detailStr(d, "task_id"),
		})

	case EventA2ATaskFailed:
		return ogenclient.NewApifrontendA2ATaskFailedPayloadAuditEventRequestEventData(ogenclient.ApifrontendA2ATaskFailedPayload{
			EventType: ogenclient.ApifrontendA2ATaskFailedPayloadEventTypeApifrontendA2aTaskFailed,
			SessionID: detailStr(d, "session_id"),
			TaskID:    detailStr(d, "task_id"),
			Error:     detailStr(d, "error"),
		})

	case EventMCPToolFailed:
		return ogenclient.NewApifrontendMCPToolFailedPayloadAuditEventRequestEventData(ogenclient.ApifrontendMCPToolFailedPayload{
			EventType: ogenclient.ApifrontendMCPToolFailedPayloadEventTypeApifrontendMcpToolFailed,
			ToolName:  detailStr(d, "tool_name"),
			Error:     detailStr(d, "error"),
			SessionID: ogenclient.NewOptString(detailStr(d, "session_id")),
		})

	case EventMCPSessionInit:
		return ogenclient.NewApifrontendMCPSessionInitPayloadAuditEventRequestEventData(ogenclient.ApifrontendMCPSessionInitPayload{
			EventType:       ogenclient.ApifrontendMCPSessionInitPayloadEventTypeApifrontendMcpSessionInit,
			McpSessionID:    detailStr(d, "mcp_session_id"),
			ProtocolVersion: ogenclient.NewOptString(detailStr(d, "protocol_version")),
		})

	case EventConfigReloaded:
		return ogenclient.NewApifrontendConfigReloadedPayloadAuditEventRequestEventData(ogenclient.ApifrontendConfigReloadedPayload{
			EventType:     ogenclient.ApifrontendConfigReloadedPayloadEventTypeApifrontendConfigReloaded,
			ConfigVersion: detailStr(d, "config_version"),
		})

	case EventConfigRejected:
		return ogenclient.NewApifrontendConfigRejectedPayloadAuditEventRequestEventData(ogenclient.ApifrontendConfigRejectedPayload{
			EventType:       ogenclient.ApifrontendConfigRejectedPayloadEventTypeApifrontendConfigRejected,
			RejectionReason: detailStr(d, "rejection_reason"),
		})

	case EventSeverityTriageCompleted:
		return ogenclient.NewApifrontendSeverityTriageCompletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSeverityTriageCompletedPayload{
			EventType:  ogenclient.ApifrontendSeverityTriageCompletedPayloadEventTypeApifrontendSeverityTriageCompleted,
			Severity:   detailStr(d, "severity"),
			SourceTier: detailStr(d, "source_tier"),
		})

	case EventSeverityTriageFailed:
		return ogenclient.NewApifrontendSeverityTriageFailedPayloadAuditEventRequestEventData(ogenclient.ApifrontendSeverityTriageFailedPayload{
			EventType:  ogenclient.ApifrontendSeverityTriageFailedPayloadEventTypeApifrontendSeverityTriageFailed,
			Error:      detailStr(d, "error"),
			FailedTier: ogenclient.NewOptString(detailStr(d, "failed_tier")),
		})

	case EventTriageStarted:
		return ogenclient.NewApifrontendTriageStartedPayloadAuditEventRequestEventData(ogenclient.ApifrontendTriageStartedPayload{
			EventType: ogenclient.ApifrontendTriageStartedPayloadEventTypeApifrontendTriageStarted,
			SessionID: detailStr(d, "session_id"),
			Persona:   ogenclient.ApifrontendTriageStartedPayloadPersona(detailStr(d, "persona")),
		})

	case EventTriageCompleted:
		return ogenclient.NewApifrontendTriageCompletedPayloadAuditEventRequestEventData(ogenclient.ApifrontendTriageCompletedPayload{
			EventType:        ogenclient.ApifrontendTriageCompletedPayloadEventTypeApifrontendTriageCompleted,
			SessionID:        detailStr(d, "session_id"),
			TriageOutcome:    ogenclient.ApifrontendTriageCompletedPayloadTriageOutcome(detailStr(d, "triage_outcome")),
			TriageDurationMs: detailInt(d, "triage_duration_ms"),
		})

	case EventRRCreated:
		return ogenclient.NewApifrontendRRCreatedPayloadAuditEventRequestEventData(ogenclient.ApifrontendRRCreatedPayload{
			EventType:   ogenclient.ApifrontendRRCreatedPayloadEventTypeApifrontendRrCreated,
			SessionID:   detailStr(d, "session_id"),
			RrName:      detailStr(d, "rr_name"),
			RrNamespace: detailStr(d, "rr_namespace"),
			Fingerprint: detailStr(d, "fingerprint"),
		})

	case EventRRDeduplicated:
		return ogenclient.NewApifrontendRRDeduplicatedPayloadAuditEventRequestEventData(ogenclient.ApifrontendRRDeduplicatedPayload{
			EventType:      ogenclient.ApifrontendRRDeduplicatedPayloadEventTypeApifrontendRrDeduplicated,
			SessionID:      detailStr(d, "session_id"),
			Fingerprint:    detailStr(d, "fingerprint"),
			ExistingRrName: detailStr(d, "existing_rr_name"),
		})

	case EventKADelegated:
		return ogenclient.NewApifrontendKADelegatedPayloadAuditEventRequestEventData(ogenclient.ApifrontendKADelegatedPayload{
			EventType:       ogenclient.ApifrontendKADelegatedPayloadEventTypeApifrontendKaDelegated,
			SessionID:       detailStr(d, "session_id"),
			KaCorrelationID: detailStr(d, "ka_correlation_id"),
			DelegationType:  ogenclient.ApifrontendKADelegatedPayloadDelegationType(detailStr(d, "delegation_type")),
		})

	case EventKAResultReceived:
		return ogenclient.NewApifrontendKAResultReceivedPayloadAuditEventRequestEventData(ogenclient.ApifrontendKAResultReceivedPayload{
			EventType:       ogenclient.ApifrontendKAResultReceivedPayloadEventTypeApifrontendKaResultReceived,
			SessionID:       detailStr(d, "session_id"),
			KaCorrelationID: detailStr(d, "ka_correlation_id"),
			ResultType:      ogenclient.ApifrontendKAResultReceivedPayloadResultType(detailStr(d, "result_type")),
		})

	case EventUserDecision:
		return ogenclient.NewApifrontendUserDecisionPayloadAuditEventRequestEventData(ogenclient.ApifrontendUserDecisionPayload{
			EventType:  ogenclient.ApifrontendUserDecisionPayloadEventTypeApifrontendUserDecision,
			SessionID:  detailStr(d, "session_id"),
			Decision:   ogenclient.ApifrontendUserDecisionPayloadDecision(detailStr(d, "decision")),
			WorkflowID: ogenclient.NewOptString(detailStr(d, "workflow_id")),
		})

	case EventAuthAccessDenied:
		return ogenclient.NewApifrontendAuthAccessDeniedPayloadAuditEventRequestEventData(ogenclient.ApifrontendAuthAccessDeniedPayload{
			EventType: ogenclient.ApifrontendAuthAccessDeniedPayloadEventTypeApifrontendAuthAccessDenied,
			ToolName:  detailStr(d, "tool_name"),
			UserRole:  detailStr(d, "user_role"),
			Endpoint:  ogenclient.ApifrontendAuthAccessDeniedPayloadEndpoint(detailStr(d, "endpoint")),
		})

	case EventToolExecuted:
		return ogenclient.NewApifrontendToolExecutedPayloadAuditEventRequestEventData(ogenclient.ApifrontendToolExecutedPayload{
			EventType:           ogenclient.ApifrontendToolExecutedPayloadEventTypeApifrontendToolExecuted,
			SessionID:           detailStr(d, "session_id"),
			ToolName:            detailStr(d, "tool_name"),
			ExecutionDurationMs: detailInt(d, "execution_duration_ms"),
			ToolOutcome:         ogenclient.ApifrontendToolExecutedPayloadToolOutcome(detailStr(d, "tool_outcome")),
		})

	default:
		return ogenclient.AuditEventRequestEventData{}
	}
}
