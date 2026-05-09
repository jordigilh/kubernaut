package audit

import (
	"context"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
	"github.com/jordigilh/kubernaut-apifrontend/internal/security"
)

// EventType classifies audit events for L3 forensic analysis.
type EventType string

// EventType values for SOC2-compatible audit classification.
const (
	EventAuthSuccess        EventType = "auth.success"
	EventAuthFailure        EventType = "auth.failure"
	EventRateLimitDenied    EventType = "ratelimit.denied"
	EventCircuitBreakerTrip EventType = "circuitbreaker.trip"

	// EventImpersonation is emitted when an impersonated K8s client is created
	// for a triage tool call. Currently captured indirectly via EventToolInvoked
	// with user identity context. Direct emission deferred until the auditor is
	// threaded through DynamicClientFactory (tracked as follow-up to SEC-05).
	EventImpersonation EventType = "impersonation.created"
	// EventJWTDelegation is emitted when a user's JWT is forwarded to KA.
	// Currently captured indirectly via EventA2ATaskStarted. Direct emission
	// deferred until the auditor is injected into JWTDelegationTransport.
	EventJWTDelegation EventType = "jwt.delegation"

	EventSessionCreated          EventType = "session.created"
	EventSessionDeleted          EventType = "session.deleted"
	EventSessionPhaseChanged     EventType = "session.phase_changed"
	EventSessionAutoCancelled    EventType = "session.auto_cancelled"
	EventSessionRetentionDeleted EventType = "session.retention_deleted"

	EventA2ATaskStarted   EventType = "a2a.task_started"
	EventA2ATaskCompleted EventType = "a2a.task_completed"
	EventA2ATaskFailed    EventType = "a2a.task_failed"
	EventMCPToolInvoked   EventType = "mcp.tool_invoked"

	EventConfigReloaded EventType = "config.reloaded"
	EventConfigRejected EventType = "config.rejected"

	EventRBACDenied  EventType = "rbac.denied"
	EventToolInvoked EventType = "tool.invoked"
)

// Event represents a SOC2-compatible audit event.
type Event struct {
	Timestamp time.Time         `json:"timestamp"`
	Type      EventType         `json:"type"`
	RequestID string            `json:"request_id,omitempty"`
	UserID    string            `json:"user_id,omitempty"`
	SourceIP  string            `json:"source_ip,omitempty"`
	Detail    map[string]string `json:"detail,omitempty"`
}

// Emitter is the interface for writing audit events.
// All callers should treat Emit as non-blocking; implementations must not
// propagate errors to the caller or block the request path.
type Emitter interface {
	Emit(ctx context.Context, event *Event)
}

// ClosableEmitter extends Emitter with lifecycle management for implementations
// that buffer events (e.g. BufferedEmitter). Callers that only need fire-and-forget
// should depend on Emitter; shutdown orchestration depends on ClosableEmitter.
type ClosableEmitter interface {
	Emitter
	Close(ctx context.Context) error
}

// Writer is the backend for durable audit event storage.
// Implemented by ds.OgenClient via WriteAuditEvents.
type Writer interface {
	WriteAuditEvents(ctx context.Context, events []*Event) error
}

// LogEmitter emits audit events as structured log entries.
type LogEmitter struct {
	Logger logr.Logger
}

// NewLogEmitter creates an Emitter that writes audit events via logr.
func NewLogEmitter(logger logr.Logger) *LogEmitter {
	return &LogEmitter{Logger: logger.WithName("audit")}
}

// Emit writes the audit event as a structured log entry.
func (e *LogEmitter) Emit(ctx context.Context, event *Event) {
	event.Timestamp = time.Now()
	if event.RequestID == "" {
		event.RequestID = requestid.FromContext(ctx)
	}

	kv := []interface{}{
		"event_type", string(event.Type),
		"timestamp", event.Timestamp.Format(time.RFC3339Nano),
		"request_id", event.RequestID,
	}
	if event.UserID != "" {
		kv = append(kv, "user_id", event.UserID)
	}
	if event.SourceIP != "" {
		kv = append(kv, "source_ip", event.SourceIP)
	}
	for k, v := range security.RedactMap(event.Detail) {
		kv = append(kv, k, v)
	}

	e.Logger.Info("audit", kv...)
}

// EmitFromContext emits an audit event using the logger from context.
func EmitFromContext(ctx context.Context, emitter Emitter, eventType EventType, userID, sourceIP string, detail map[string]string) {
	if emitter == nil {
		return
	}
	emitter.Emit(ctx, &Event{
		Type:      eventType,
		RequestID: requestid.FromContext(ctx),
		UserID:    userID,
		SourceIP:  sourceIP,
		Detail:    detail,
	})
}
