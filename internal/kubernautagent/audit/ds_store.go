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
	"fmt"
	"time"

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
	if event.ActingUser != "" {
		req.ActorType.SetTo("User")
		req.ActorID.SetTo(event.ActingUser)
	} else {
		actorType := "Service"
		actorID := "kubernaut-agent"
		if event.ActorID != "" {
			actorID = event.ActorID
		}
		if event.ActorType != "" {
			actorType = event.ActorType
		}
		req.ActorType.SetTo(actorType)
		req.ActorID.SetTo(actorID)
	}
	if event.ParentEventID != nil {
		req.ParentEventID.SetTo(*event.ParentEventID)
	}
	if event.ClusterID != "" {
		req.ClusterID.SetTo(event.ClusterID)
	}

	if ed, ok := buildEventData(event); ok {
		req.EventData = ed
	}

	if _, err := s.client.CreateAuditEvent(ctx, req); err != nil {
		return fmt.Errorf("audit store: %w", err)
	}
	return nil
}

// eventDataBuilder builds a typed event-data payload for one EventType*
// constant. See DD-AUDIT-008 for why this registry/lookup-table pattern was
// chosen over a type-switch.
type eventDataBuilder func(event *AuditEvent) ogenclient.AuditEventRequestEventData

// eventDataBuilders maps each EventType* constant to its payload builder.
// Adding a new event type means adding one entry here plus the builder
// function — buildEventData itself never needs to change (DD-AUDIT-008).
var eventDataBuilders = map[string]eventDataBuilder{
	EventTypeEnrichmentCompleted:    buildEnrichmentCompletedPayload,
	EventTypeEnrichmentFailed:       buildEnrichmentFailedPayload,
	EventTypeLLMRequest:             buildLLMRequestPayload,
	EventTypeLLMResponse:            buildLLMResponsePayload,
	EventTypeLLMToolCall:            buildLLMToolCallPayload,
	EventTypeValidationAttempt:      buildValidationAttemptPayload,
	EventTypeResponseComplete:       buildResponseCompletePayload,
	EventTypeRCAComplete:            buildRCACompletePayload,
	EventTypeResponseFailed:         buildResponseFailedPayload,
	EventTypeSessionStarted:         buildSessionStartedPayload,
	EventTypeSessionCompleted:       buildSessionCompletedPayload,
	EventTypeSessionFailed:          buildSessionFailedPayload,
	EventTypeSessionCancelled:       buildSessionCancelledPayload,
	EventTypeSessionObserved:        buildSessionObservedPayload,
	EventTypeSessionAccessDenied:    buildSessionAccessDeniedPayload,
	EventTypeInvestigationCancelled: buildInvestigationCancelledPayload,
	EventTypeAlignmentStep:          buildAlignmentStepPayload,
	EventTypeAlignmentVerdict:       buildAlignmentVerdictPayload,
	EventTypeSessionSuspended:       buildSessionSuspendedPayload,
	EventTypeSessionResumed:         buildSessionResumedPayload,
	EventTypeInteractiveStarted:     buildInteractiveStartedPayload,
	EventTypeInteractiveCompleted:   buildInteractiveCompletedPayload,
	EventTypeInteractiveK8sCall:     buildInteractiveK8sCallPayload,
	EventTypeSecretAccessed:         buildSecretAccessedPayload,
	EventTypeShadowLLMRequest:       buildShadowLLMRequestPayload,
	EventTypeShadowLLMResponse:      buildShadowLLMResponsePayload,
	EventTypeRateLimitDenied:        buildRateLimitDeniedPayload,
	EventTypeAuthFailure:            buildAuthFailurePayload,
	EventTypeAuthDenied:             buildAuthDeniedPayload,
}

func buildEventData(event *AuditEvent) (ogenclient.AuditEventRequestEventData, bool) {
	builder, ok := eventDataBuilders[event.EventType]
	if !ok {
		return ogenclient.AuditEventRequestEventData{}, false
	}
	return builder(event), true
}
