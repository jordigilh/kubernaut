package audit

import (
	"context"
	"time"
)

// EventType classifies audit events for L3 forensic analysis.
type EventType string

// EventType values for SOC2-compatible audit classification.
const (
	EventAuthSuccess        EventType = "auth.success"
	EventAuthFailure        EventType = "auth.failure"
	EventRateLimitDenied    EventType = "ratelimit.denied"
	EventCircuitBreakerTrip EventType = "circuitbreaker.trip"

	EventJWTDelegation EventType = "jwt.delegation"

	EventSessionCreated          EventType = "session.created"
	EventSessionDeleted          EventType = "session.deleted"
	EventSessionPhaseChanged     EventType = "session.phase_changed"
	EventSessionAutoCancelled    EventType = "session.auto_cancelled"
	EventSessionRetentionDeleted EventType = "session.retention_deleted"

	EventA2ATaskStarted   EventType = "a2a.task_started"
	EventA2ATaskCompleted EventType = "a2a.task_completed"
	EventA2ATaskFailed    EventType = "a2a.task_failed"
	EventA2AStreamOpened  EventType = "a2a.stream_opened"
	EventA2AStreamClosed  EventType = "a2a.stream_closed"
	EventMCPToolFailed    EventType = "mcp.tool_failed"
	EventMCPSessionInit   EventType = "mcp.session_init"

	EventSeverityTriageCompleted EventType = "severity_triage.completed"
	EventSeverityTriageFailed    EventType = "severity_triage.failed"

	EventWorkflowDiscovery EventType = "workflow.discovery"

	EventConfigReloaded EventType = "config.reloaded"
	EventConfigRejected EventType = "config.rejected"

	// Consolidated from rbac.denied + mcp.tool_denied (Issue #1156)
	EventAuthAccessDenied EventType = "auth.access_denied"
	// Consolidated from tool.invoked + mcp.tool_invoked (Issue #1156)
	EventToolExecuted EventType = "tool.executed"

	// AU-2/AU-3: Agent card discovery audit (Issue #1259)
	EventAgentCardAccessed EventType = "discovery.agent_card_accessed"

	// New from Issue #1021 catalog (Issue #1156)
	EventSessionCompleted EventType = "session.completed"
	EventTriageStarted    EventType = "triage.started"
	EventTriageCompleted  EventType = "triage.completed"
	EventRRCreated        EventType = "rr.created"
	EventRRDeduplicated   EventType = "rr.deduplicated"
	EventKADelegated      EventType = "ka.delegated"
	EventKAResultReceived EventType = "ka.result_received"
	EventUserDecision     EventType = "user.decision"
)

// Event represents a SOC2-compatible audit event.
type Event struct {
	Timestamp     time.Time         `json:"timestamp"`
	Type          EventType         `json:"type"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	RequestID     string            `json:"request_id,omitempty"`
	UserID        string            `json:"user_id,omitempty"`
	SourceIP      string            `json:"source_ip,omitempty"`
	Detail        map[string]string `json:"detail,omitempty"`
}

// Emitter is the interface for writing audit events.
// All callers should treat Emit as non-blocking; implementations must not
// propagate errors to the caller or block the request path.
type Emitter interface {
	Emit(ctx context.Context, event *Event)
}

// ClosableEmitter extends Emitter with lifecycle management for implementations
// that buffer events (e.g. StoreAdapter). Callers that only need fire-and-forget
// should depend on Emitter; shutdown orchestration depends on ClosableEmitter.
type ClosableEmitter interface {
	Emitter
	Close(ctx context.Context) error
}

