/*
Copyright 2025 Jordi Gil.

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

// Package audit provides audit event generation for the SignalProcessing controller.
// BR-SP-090: Categorization Audit Trail
// DD-AUDIT-002: Uses shared pkg/audit library for consistent audit behavior.
// DD-AUDIT-003: Implements service-specific audit event types.
//
// Per ADR-038: Fire-and-forget pattern with buffered writes (<1ms overhead).
// Business logic NEVER waits for audit writes.
package audit

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

// Event type constants (per DD-AUDIT-003)
const (
	// EventTypeSignalProcessed is emitted when signal processing completes
	EventTypeSignalProcessed = "signalprocessing.signal.processed"
	// EventTypePhaseTransition is emitted on phase changes
	EventTypePhaseTransition = "signalprocessing.phase.transition"
	// EventTypeClassificationDecision is emitted when classification is made
	EventTypeClassificationDecision = "signalprocessing.classification.decision"
	// EventTypeEnrichmentComplete is emitted when K8s enrichment completes
	EventTypeEnrichmentComplete = "signalprocessing.enrichment.completed"
	// EventTypeError is emitted on errors
	EventTypeError = "signalprocessing.error.occurred"
)

// AuditClient handles audit event storage using pkg/audit shared library.
// BR-SP-090: Categorization Audit Trail
// DD-005 v2.0: Uses logr.Logger
type AuditClient struct {
	store audit.AuditStore // Uses shared library interface
	log   logr.Logger
}

// NewAuditClient creates a new audit client.
// DD-005 v2.0: Accept logr.Logger from caller
func NewAuditClient(store audit.AuditStore, log logr.Logger) *AuditClient {
	return &AuditClient{
		store: store,
		log:   log.WithName("audit"),
	}
}

// RecordSignalProcessed records signal processing completion event.
// BR-SP-090: Primary audit event for SignalProcessing
// ADR-038: Fire-and-forget pattern
func (c *AuditClient) RecordSignalProcessed(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) {
	// Build event data payload
	eventData := map[string]interface{}{
		"phase":    sp.Status.Phase,
		"signal":   sp.Spec.Signal.Name,
		"severity": sp.Spec.Signal.Severity,
	}

	// Add environment classification if present
	if sp.Status.EnvironmentClassification != nil {
		eventData["environment"] = sp.Status.EnvironmentClassification.Environment
		eventData["environment_confidence"] = sp.Status.EnvironmentClassification.Confidence
		eventData["environment_source"] = sp.Status.EnvironmentClassification.Source
	}

	// Add priority assignment if present
	if sp.Status.PriorityAssignment != nil {
		eventData["priority"] = sp.Status.PriorityAssignment.Priority
		eventData["priority_confidence"] = sp.Status.PriorityAssignment.Confidence
		eventData["priority_source"] = sp.Status.PriorityAssignment.Source
	}

	// Add business classification if present
	if sp.Status.BusinessClassification != nil {
		eventData["criticality"] = sp.Status.BusinessClassification.Criticality
		eventData["sla_requirement"] = sp.Status.BusinessClassification.SLARequirement
		eventData["business_confidence"] = sp.Status.BusinessClassification.OverallConfidence
	}

	// Add K8s context indicators
	if sp.Status.KubernetesContext != nil {
		eventData["has_owner_chain"] = len(sp.Status.KubernetesContext.OwnerChain) > 0
		eventData["owner_chain_length"] = len(sp.Status.KubernetesContext.OwnerChain)
		eventData["degraded_mode"] = sp.Status.KubernetesContext.DegradedMode

		if sp.Status.KubernetesContext.DetectedLabels != nil {
			eventData["has_pdb"] = sp.Status.KubernetesContext.DetectedLabels.HasPDB
			eventData["has_hpa"] = sp.Status.KubernetesContext.DetectedLabels.HasHPA
		}
	}

	// Add error if present
	if sp.Status.Error != "" {
		eventData["error"] = sp.Status.Error
	}

	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		c.log.Error(err, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := sp.Namespace

	// Determine outcome
	outcome := "success"
	if sp.Status.Phase == signalprocessingv1alpha1.PhaseFailed {
		outcome = "failure"
	}

	// Build audit event using pkg/audit.AuditEvent
	event := audit.NewAuditEvent()
	event.EventType = EventTypeSignalProcessed
	event.EventCategory = "signalprocessing"
	event.EventAction = "processed"
	event.EventOutcome = outcome
	event.ActorType = "service"
	event.ActorID = "signalprocessing-controller"
	event.ResourceType = "SignalProcessing"
	event.ResourceID = sp.Name
	event.CorrelationID = sp.Spec.RemediationRequestRef.Name
	event.Namespace = &namespace
	event.EventData = eventDataBytes

	// Fire-and-forget (per ADR-038)
	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write audit event",
			"event_type", event.EventType,
			"correlation_id", event.CorrelationID,
		)
		// Don't fail reconciliation on audit failure (graceful degradation)
	}
}

// RecordPhaseTransition records a phase transition event.
func (c *AuditClient) RecordPhaseTransition(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, from, to string) {
	eventData := map[string]interface{}{
		"from_phase": from,
		"to_phase":   to,
		"signal":     sp.Spec.Signal.Name,
	}
	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		c.log.Error(err, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := sp.Namespace

	event := audit.NewAuditEvent()
	event.EventType = EventTypePhaseTransition
	event.EventCategory = "signalprocessing"
	event.EventAction = "phase_transition"
	event.EventOutcome = "success"
	event.ActorType = "service"
	event.ActorID = "signalprocessing-controller"
	event.ResourceType = "SignalProcessing"
	event.ResourceID = sp.Name
	event.CorrelationID = sp.Spec.RemediationRequestRef.Name
	event.Namespace = &namespace
	event.EventData = eventDataBytes

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write phase transition audit")
	}
}

// RecordClassificationDecision records classification decision event.
// BR-SP-090: Logs environment, priority, and business classification decisions
func (c *AuditClient) RecordClassificationDecision(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) {
	eventData := map[string]interface{}{
		"signal":   sp.Spec.Signal.Name,
		"severity": sp.Spec.Signal.Severity,
	}

	// Add all classification results
	if sp.Status.EnvironmentClassification != nil {
		eventData["environment"] = sp.Status.EnvironmentClassification.Environment
		eventData["environment_confidence"] = sp.Status.EnvironmentClassification.Confidence
	}
	if sp.Status.PriorityAssignment != nil {
		eventData["priority"] = sp.Status.PriorityAssignment.Priority
		eventData["priority_confidence"] = sp.Status.PriorityAssignment.Confidence
	}
	if sp.Status.BusinessClassification != nil {
		eventData["criticality"] = sp.Status.BusinessClassification.Criticality
		eventData["sla_requirement"] = sp.Status.BusinessClassification.SLARequirement
	}

	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		c.log.Error(err, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := sp.Namespace

	event := audit.NewAuditEvent()
	event.EventType = EventTypeClassificationDecision
	event.EventCategory = "signalprocessing"
	event.EventAction = "classification"
	event.EventOutcome = "success"
	event.ActorType = "service"
	event.ActorID = "signalprocessing-controller"
	event.ResourceType = "SignalProcessing"
	event.ResourceID = sp.Name
	event.CorrelationID = sp.Spec.RemediationRequestRef.Name
	event.Namespace = &namespace
	event.EventData = eventDataBytes

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write classification audit")
	}
}

// RecordEnrichmentComplete records K8s enrichment completion event.
func (c *AuditClient) RecordEnrichmentComplete(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, durationMs int) {
	eventData := map[string]interface{}{
		"duration_ms": durationMs,
	}

	if sp.Status.KubernetesContext != nil {
		eventData["has_namespace"] = sp.Status.KubernetesContext.Namespace != nil
		eventData["has_pod"] = sp.Status.KubernetesContext.Pod != nil
		eventData["has_deployment"] = sp.Status.KubernetesContext.Deployment != nil
		eventData["owner_chain_length"] = len(sp.Status.KubernetesContext.OwnerChain)
		eventData["degraded_mode"] = sp.Status.KubernetesContext.DegradedMode
	}

	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		c.log.Error(err, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := sp.Namespace

	event := audit.NewAuditEvent()
	event.EventType = EventTypeEnrichmentComplete
	event.EventCategory = "signalprocessing"
	event.EventAction = "enrichment"
	event.EventOutcome = "success"
	event.ActorType = "service"
	event.ActorID = "signalprocessing-controller"
	event.ResourceType = "SignalProcessing"
	event.ResourceID = sp.Name
	event.CorrelationID = sp.Spec.RemediationRequestRef.Name
	event.Namespace = &namespace
	event.EventData = eventDataBytes
	event.DurationMs = &durationMs

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write enrichment audit")
	}
}

// RecordError records an error event.
func (c *AuditClient) RecordError(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, phase string, err error) {
	eventData := map[string]interface{}{
		"phase":  phase,
		"error":  err.Error(),
		"signal": sp.Spec.Signal.Name,
	}
	eventDataBytes, marshalErr := json.Marshal(eventData)
	if marshalErr != nil {
		c.log.Error(marshalErr, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := sp.Namespace
	errorMsg := err.Error()

	event := audit.NewAuditEvent()
	event.EventType = EventTypeError
	event.EventCategory = "signalprocessing"
	event.EventAction = "error"
	event.EventOutcome = "failure"
	event.ActorType = "service"
	event.ActorID = "signalprocessing-controller"
	event.ResourceType = "SignalProcessing"
	event.ResourceID = sp.Name
	event.CorrelationID = sp.Spec.RemediationRequestRef.Name
	event.Namespace = &namespace
	event.EventData = eventDataBytes
	event.ErrorMessage = &errorMsg

	if storeErr := c.store.StoreAudit(ctx, event); storeErr != nil {
		c.log.Error(storeErr, "Failed to write error audit")
	}
}


