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
	"log/slog"

	"github.com/google/uuid"
)

// EventCategory is the audit event_category for all Kubernaut Agent events.
const EventCategory = "aiagent"

const (
	EventTypeLLMRequest          = "aiagent.llm.request"
	EventTypeLLMResponse         = "aiagent.llm.response"
	EventTypeLLMToolCall         = "aiagent.llm.tool_call"
	EventTypeValidationAttempt   = "aiagent.workflow.validation_attempt"
	EventTypeResponseComplete    = "aiagent.response.complete"
	EventTypeResponseFailed      = "aiagent.response.failed"
	EventTypeRCAComplete         = "aiagent.rca.complete"
	EventTypeEnrichmentCompleted = "aiagent.enrichment.completed"
	EventTypeEnrichmentFailed    = "aiagent.enrichment.failed"
	EventTypeAlignmentStep       = "aiagent.alignment.step"
	EventTypeAlignmentVerdict    = "aiagent.alignment.verdict"

	EventTypeSessionStarted   = "aiagent.session.started"
	EventTypeSessionCancelled = "aiagent.session.cancelled"
	EventTypeSessionCompleted = "aiagent.session.completed"
	EventTypeSessionFailed    = "aiagent.session.failed"

	// EventTypeInvestigationCancelled is emitted by the investigator when
	// it detects context cancellation mid-investigation (BR-SESSION-001).
	// Unlike EventTypeSessionCancelled (emitted by session.Manager at the
	// session lifecycle level), this event carries investigation-internal
	// state: the phase, turn number, and accumulated token usage at the
	// point of cancellation. This enables SOC2 CC8.1 audit reconstruction
	// of partial investigation progress.
	EventTypeInvestigationCancelled = "aiagent.investigation.cancelled"

	// EventTypeSessionObserved is emitted when an operator subscribes to an
	// active investigation's SSE stream (BR-SESSION-005). Records who is
	// observing which investigation for SOC2 CC8.1 audit trail.
	EventTypeSessionObserved = "aiagent.session.observed"

	// EventTypeSessionAccessDenied is emitted when an authenticated user
	// attempts to access a session they do not own. Records the requesting
	// user, target session, and endpoint for SOC2 CC8.1 failed-access audit.
	EventTypeSessionAccessDenied = "aiagent.session.access_denied"

	// EventTypeSessionSuspended is emitted when an autonomous investigation is
	// suspended due to dynamic takeover (BR-INTERACTIVE-004). The session remains
	// in a terminal state; reconstruction spawns a new session after interactive
	// mode ends. DD-INTERACTIVE-002 identity transition: KA SA → human operator.
	EventTypeSessionSuspended = "aiagent.session.suspended"

	// EventTypeInteractiveStarted is emitted when a user acquires the interactive
	// Lease and begins driving the investigation (BR-INTERACTIVE-004).
	EventTypeInteractiveStarted = "aiagent.interactive.started"

	// EventTypeInteractiveCompleted is emitted when an interactive session ends,
	// either by explicit complete, cancel, disconnect, or timeout. Carries the
	// reason in event data for SOC2 attribution.
	EventTypeInteractiveCompleted = "aiagent.interactive.completed"

	// EventTypeSessionResumed is emitted when autonomous investigation resumes
	// after interactive session ends (cancel+reconstruct). The new session ID
	// and reconstructed context are included in event data.
	EventTypeSessionResumed = "aiagent.session.resumed"
)

const (
	ActionLLMRequest     = "llm_request"
	ActionLLMResponse    = "llm_response"
	ActionToolExecution  = "tool_execution"
	ActionValidation     = "validation"
	ActionResponseSent   = "response_sent"
	ActionResponseFailed    = "response_failed"
	ActionAlignmentEvaluate = "alignment_evaluate"
	ActionAlignmentVerdict  = "alignment_verdict"

	ActionSessionStarted   = "session_started"
	ActionSessionCancelled = "session_cancelled"
	ActionSessionCompleted = "session_completed"
	ActionSessionFailed    = "session_failed"

	ActionInvestigationCancelled = "investigation_cancelled"
	ActionSessionObserved       = "session_observed"
	ActionSessionAccessDenied   = "session_access_denied"

	ActionSessionSuspended     = "session_suspended"
	ActionInteractiveStarted   = "interactive_started"
	ActionInteractiveCompleted = "interactive_completed"
	ActionSessionResumed       = "session_resumed"
)

const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomePending = "pending"
)

// AllEventTypes lists all Kubernaut Agent audit event types.
var AllEventTypes = []string{
	EventTypeLLMRequest,
	EventTypeLLMResponse,
	EventTypeLLMToolCall,
	EventTypeValidationAttempt,
	EventTypeResponseComplete,
	EventTypeRCAComplete,
	EventTypeResponseFailed,
	EventTypeEnrichmentCompleted,
	EventTypeEnrichmentFailed,
	EventTypeAlignmentStep,
	EventTypeAlignmentVerdict,
	EventTypeSessionStarted,
	EventTypeSessionCancelled,
	EventTypeSessionCompleted,
	EventTypeSessionFailed,
	EventTypeInvestigationCancelled,
	EventTypeSessionObserved,
	EventTypeSessionAccessDenied,
	EventTypeSessionSuspended,
	EventTypeInteractiveStarted,
	EventTypeInteractiveCompleted,
	EventTypeSessionResumed,
}

// AuditEvent represents an audit event to be stored.
type AuditEvent struct {
	EventType     string
	EventCategory string
	EventAction   string
	EventOutcome  string
	CorrelationID string
	SessionID     string
	ActingUser    string
	ParentEventID *uuid.UUID
	Data          map[string]interface{}
}

// AuditStore is the interface for storing audit events (matches pkg/audit.AuditStore).
type AuditStore interface {
	StoreAudit(ctx context.Context, event *AuditEvent) error
}

// EventOption configures optional fields on an AuditEvent.
type EventOption func(*AuditEvent)

// WithSessionID attaches an interactive session identifier to the audit event.
func WithSessionID(sessionID string) EventOption {
	return func(e *AuditEvent) {
		e.SessionID = sessionID
	}
}

// WithActingUser attaches the identity of the user who triggered the event.
// Used in interactive MCP sessions for SOC2 per-event user attribution
// (BR-INTERACTIVE-005).
func WithActingUser(user string) EventOption {
	return func(e *AuditEvent) {
		e.ActingUser = user
	}
}

// NewEvent creates an AuditEvent with the correct event_category and a unique event_id.
func NewEvent(eventType string, correlationID string, opts ...EventOption) *AuditEvent {
	data := make(map[string]interface{})
	data["event_id"] = uuid.New().String()
	event := &AuditEvent{
		EventType:     eventType,
		EventCategory: EventCategory,
		CorrelationID: correlationID,
		Data:          data,
	}
	for _, opt := range opts {
		opt(event)
	}
	return event
}

// StoreBestEffort stores an audit event without propagating errors (fire-and-forget).
func StoreBestEffort(ctx context.Context, store AuditStore, event *AuditEvent, logger *slog.Logger) {
	if err := store.StoreAudit(ctx, event); err != nil {
		logger.Warn("audit store failure (best-effort)",
			slog.String("event_type", event.EventType),
			slog.String("error", err.Error()),
		)
	}
}

// InstrumentedAuditStore wraps an AuditStore to call a recorder after each
// successful store. This enables BR-KA-OBSERVABILITY-001.7 audit pipeline
// throughput metrics without changing StoreBestEffort callers.
type InstrumentedAuditStore struct {
	inner    AuditStore
	recorder func(eventType string)
}

// NewInstrumentedAuditStore wraps an AuditStore. recorder is called on each
// successful StoreAudit with the event type. recorder may be nil.
func NewInstrumentedAuditStore(inner AuditStore, recorder func(eventType string)) AuditStore {
	if recorder == nil {
		return inner
	}
	return &InstrumentedAuditStore{inner: inner, recorder: recorder}
}

func (s *InstrumentedAuditStore) StoreAudit(ctx context.Context, event *AuditEvent) error {
	err := s.inner.StoreAudit(ctx, event)
	if err == nil {
		s.recorder(event.EventType)
	}
	return err
}
