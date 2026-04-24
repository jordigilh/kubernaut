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
	EventTypeConversationTurn    = "aiagent.conversation.turn"
	EventTypeValidationAttempt   = "aiagent.workflow.validation_attempt"
	EventTypeResponseComplete    = "aiagent.response.complete"
	EventTypeResponseFailed      = "aiagent.response.failed"
	EventTypeEnrichmentCompleted = "aiagent.enrichment.completed"
	EventTypeEnrichmentFailed    = "aiagent.enrichment.failed"
	EventTypeAlignmentStep       = "aiagent.alignment.step"
	EventTypeAlignmentVerdict    = "aiagent.alignment.verdict"

	EventTypeSessionStarted   = "aiagent.session.started"
	EventTypeSessionCancelled = "aiagent.session.cancelled"
	EventTypeSessionCompleted = "aiagent.session.completed"
	EventTypeSessionFailed    = "aiagent.session.failed"
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
	EventTypeConversationTurn,
	EventTypeValidationAttempt,
	EventTypeResponseComplete,
	EventTypeResponseFailed,
	EventTypeEnrichmentCompleted,
	EventTypeEnrichmentFailed,
	EventTypeAlignmentStep,
	EventTypeAlignmentVerdict,
	EventTypeSessionStarted,
	EventTypeSessionCancelled,
	EventTypeSessionCompleted,
	EventTypeSessionFailed,
}

// AuditEvent represents an audit event to be stored.
type AuditEvent struct {
	EventType     string
	EventCategory string
	EventAction   string
	EventOutcome  string
	CorrelationID string
	ParentEventID *uuid.UUID
	Data          map[string]interface{}
}

// AuditStore is the interface for storing audit events (matches pkg/audit.AuditStore).
type AuditStore interface {
	StoreAudit(ctx context.Context, event *AuditEvent) error
}

// NewEvent creates an AuditEvent with the correct event_category and a unique event_id.
func NewEvent(eventType string, correlationID string) *AuditEvent {
	data := make(map[string]interface{})
	data["event_id"] = uuid.New().String()
	return &AuditEvent{
		EventType:     eventType,
		EventCategory: EventCategory,
		CorrelationID: correlationID,
		Data:          data,
	}
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
