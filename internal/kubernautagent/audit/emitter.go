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

	"github.com/go-logr/logr"
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
	EventTypeShadowLLMRequest    = "aiagent.shadow.llm.request"
	EventTypeShadowLLMResponse   = "aiagent.shadow.llm.response"
)

const (
	ActionLLMRequest        = "llm_request"
	ActionLLMResponse       = "llm_response"
	ActionToolExecution     = "tool_execution"
	ActionValidation        = "validation"
	ActionResponseSent      = "response_sent"
	ActionResponseFailed    = "response_failed"
	ActionAlignmentEvaluate          = "alignment_evaluate"
	ActionAlignmentVerdict           = "alignment_verdict"
	ActionSameKindGate               = "same_kind_validation_gate"
	ActionAPIVersionGate             = "api_version_validation_gate"
	ActionWorkflowAlignmentGate      = "workflow_target_alignment_gate"
	ActionShadowLLMRequest           = "shadow_llm_request"
	ActionShadowLLMResponse          = "shadow_llm_response"
	ActionTruncationDetected         = "truncation_detected"
	ActionEnriched                   = "enriched"
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
	EventTypeShadowLLMRequest,
	EventTypeShadowLLMResponse,
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
	ActorID       string
	ActorType     string
}

// AuditStore is the interface for storing audit events (matches pkg/audit.AuditStore).
type AuditStore interface {
	StoreAudit(ctx context.Context, event *AuditEvent) error
}

type actorContextKey struct{}

type actorValue struct {
	ID   string
	Type string
}

// WithActor returns a context carrying the given actor identity.
// All audit events emitted via StoreBestEffort on this context will
// inherit the actor unless the event already has explicit ActorID/ActorType.
func WithActor(ctx context.Context, actorID, actorType string) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actorValue{ID: actorID, Type: actorType})
}

// ActorFromContext extracts the actor identity from the context.
func ActorFromContext(ctx context.Context) (actorID, actorType string, ok bool) {
	v, ok := ctx.Value(actorContextKey{}).(actorValue)
	if !ok {
		return "", "", false
	}
	return v.ID, v.Type, true
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
// If the event has no ActorID/ActorType set, it inherits from the context (see WithActor).
func StoreBestEffort(ctx context.Context, store AuditStore, event *AuditEvent, logger logr.Logger) {
	if event.ActorID == "" || event.ActorType == "" {
		if id, typ, ok := ActorFromContext(ctx); ok {
			if event.ActorID == "" {
				event.ActorID = id
			}
			if event.ActorType == "" {
				event.ActorType = typ
			}
		}
	}
	if err := store.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "audit store failure (best-effort)", "event_type", event.EventType)
	}
}
