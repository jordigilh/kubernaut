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

	"github.com/go-logr/logr"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// Event type constants (per DD-AUDIT-003 and ADR-034 Table 3)
const (
	// EventTypeSignalProcessed is emitted when signal processing completes
	EventTypeSignalProcessed = "signalprocessing.signal.processed"
	// EventTypePhaseTransition is emitted on phase changes
	EventTypePhaseTransition = "signalprocessing.phase.transition"
	// EventTypeClassificationDecision is emitted when classification is made (ADR-034 Table 3)
	EventTypeClassificationDecision = "signalprocessing.classification.decision"
	// EventTypeBusinessClassified is emitted when business classification is applied
	// AUDIT-06: Separate event for business unit/criticality/SLA (per integration-test-plan.md v1.1.0)
	EventTypeBusinessClassified = "signalprocessing.business.classified"
	// EventTypeEnrichmentComplete is emitted when K8s enrichment completes (ADR-034 Service Requirements)
	EventTypeEnrichmentComplete = "signalprocessing.enrichment.completed"
	// EventTypeError is emitted on errors (ADR-034 Service Requirements)
	EventTypeError = "signalprocessing.error.occurred"
	// CategorySignalProcessing is the service-level category per ADR-034 v1.2
	CategorySignalProcessing = "signalprocessing"
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
	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.SignalProcessingAuditPayload{
		EventType: EventTypeSignalProcessed, // Required for discriminator
		Phase:     toSignalProcessingAuditPayloadPhase(string(sp.Status.Phase)),
		Signal:    sp.Spec.Signal.Name,
	}
	if sev := toSignalProcessingAuditPayloadSeverity(sp.Spec.Signal.Severity); sev != "" {
		payload.Severity.SetTo(sev)
	}

	// Add environment classification if present
	// Note: Confidence field removed per DD-SP-001 V1.1
	if sp.Status.EnvironmentClassification != nil {
		if env := toSignalProcessingAuditPayloadEnvironment(string(sp.Status.EnvironmentClassification.Environment)); env != "" {
			payload.Environment.SetTo(env)
		}
		if envSrc := toSignalProcessingAuditPayloadEnvironmentSource(sp.Status.EnvironmentClassification.Source); envSrc != "" {
			payload.EnvironmentSource.SetTo(envSrc)
		}
	}

	// Add priority assignment if present
	// Note: Confidence field removed per DD-SP-001 V1.1
	if sp.Status.PriorityAssignment != nil {
		if priority := toSignalProcessingAuditPayloadPriority(string(sp.Status.PriorityAssignment.Priority)); priority != "" {
			payload.Priority.SetTo(priority)
		}
		if prioritySrc := toSignalProcessingAuditPayloadPrioritySource(sp.Status.PriorityAssignment.Source); prioritySrc != "" {
			payload.PrioritySource.SetTo(prioritySrc)
		}
	}

	// Add business classification if present
	// Note: OverallConfidence field removed per DD-SP-001 V1.1
	if sp.Status.BusinessClassification != nil {
		if crit := toSignalProcessingAuditPayloadCriticality(string(sp.Status.BusinessClassification.Criticality)); crit != "" {
			payload.Criticality.SetTo(crit)
		}
		if sp.Status.BusinessClassification.SLARequirement != "" {
			payload.SLARequirement.SetTo(sp.Status.BusinessClassification.SLARequirement)
		}
	}

	// Add K8s context indicators
	if sp.Status.KubernetesContext != nil {
		payload.HasOwnerChain.SetTo(len(sp.Status.KubernetesContext.OwnerChain) > 0)
		payload.OwnerChainLength.SetTo(len(sp.Status.KubernetesContext.OwnerChain))
		payload.DegradedMode.SetTo(sp.Status.KubernetesContext.DegradedMode)

		if sp.Status.KubernetesContext.DetectedLabels != nil {
			payload.HasPdb.SetTo(sp.Status.KubernetesContext.DetectedLabels.HasPDB)
			payload.HasHpa.SetTo(sp.Status.KubernetesContext.DetectedLabels.HasHPA)
		}
	}

	// Add error if present
	if sp.Status.Error != "" {
		payload.Error.SetTo(sp.Status.Error)
	}

	// Determine outcome
	var apiOutcome ogenclient.AuditEventRequestEventOutcome
	if sp.Status.Phase == signalprocessingv1alpha1.PhaseFailed {
		apiOutcome = audit.OutcomeFailure
	} else {
		apiOutcome = audit.OutcomeSuccess
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeSignalProcessed)
	audit.SetEventCategory(event, CategorySignalProcessing)
	audit.SetEventAction(event, "processed")
	audit.SetEventOutcome(event, apiOutcome)
	audit.SetActor(event, "service", "signalprocessing-controller")
	audit.SetResource(event, "SignalProcessing", sp.Name)

	// Authority: RO always creates SP with RemediationRequestRef (pkg/remediationorchestrator/creator/signalprocessing.go:91-97)
	// Production architecture: SignalProcessing CRs MUST have parent RemediationRequest
	// Graceful degradation: skip audit if no RemediationRequestRef (test edge cases)
	if sp.Spec.RemediationRequestRef.Name == "" {
		c.log.V(1).Info("Skipping signal processed audit - no RemediationRequestRef")
		return
	}
	audit.SetCorrelationID(event, sp.Spec.RemediationRequestRef.Name)
	audit.SetNamespace(event, sp.Namespace)

	// Set structured payload using union constructor (OGEN-MIGRATION)
	event.EventData = api.NewAuditEventRequestEventDataSignalprocessingSignalProcessedAuditEventRequestEventData(payload)

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
	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.SignalProcessingAuditPayload{
		EventType: EventTypePhaseTransition, // Required for discriminator
		Signal:    sp.Spec.Signal.Name,
		Phase:     toSignalProcessingAuditPayloadPhase(string(sp.Status.Phase)),
	}
	payload.FromPhase.SetTo(from)
	payload.ToPhase.SetTo(to)

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypePhaseTransition)
	audit.SetEventCategory(event, CategorySignalProcessing)
	audit.SetEventAction(event, "phase_transition")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", "signalprocessing-controller")
	audit.SetResource(event, "SignalProcessing", sp.Name)

	// Graceful degradation: skip audit if no RemediationRequestRef (test edge cases)
	if sp.Spec.RemediationRequestRef.Name == "" {
		c.log.V(1).Info("Skipping phase transition audit - no RemediationRequestRef")
		return
	}
	audit.SetCorrelationID(event, sp.Spec.RemediationRequestRef.Name)
	audit.SetNamespace(event, sp.Namespace)

	// Set structured payload using union constructor (OGEN-MIGRATION)
	event.EventData = api.NewAuditEventRequestEventDataSignalprocessingPhaseTransitionAuditEventRequestEventData(payload)

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write phase transition audit")
	}
}

// RecordClassificationDecision records classification decision event.
// BR-SP-090: Logs environment, priority, and business classification decisions
// DD-SEVERITY-001: Includes external and normalized severity for audit trail
// durationMs: Classification duration in milliseconds (for performance metrics)
func (c *AuditClient) RecordClassificationDecision(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, durationMs int) {
	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.SignalProcessingAuditPayload{
		EventType: EventTypeClassificationDecision, // Required for discriminator
		Signal:    sp.Spec.Signal.Name,
		Phase:     toSignalProcessingAuditPayloadPhase(string(sp.Status.Phase)),
	}
	payload.DurationMs.SetTo(durationMs)

	// DD-SEVERITY-001: Use normalized severity (sp.Status.Severity) for the main Severity field
	// to ensure it always matches the required enum ["critical", "warning", "info"]
	if sp.Status.Severity != "" {
		payload.Severity.SetTo(toSignalProcessingAuditPayloadSeverity(sp.Status.Severity))
	}

	// DD-SEVERITY-001: Record external and normalized severity for compliance audit trail
	if sp.Spec.Signal.Severity != "" {
		payload.ExternalSeverity.SetTo(sp.Spec.Signal.Severity)
	}
	if sp.Status.Severity != "" {
		if normalizedSev := toSignalProcessingAuditPayloadNormalizedSeverity(sp.Status.Severity); normalizedSev != "" {
			payload.NormalizedSeverity.SetTo(normalizedSev)
		}
		// Always set determination_source to "rego-policy" when normalized severity exists
		payload.DeterminationSource.SetTo(api.SignalProcessingAuditPayloadDeterminationSourceRegoPolicy)
	}

	// Add policy hash for audit trail and policy version tracking
	if sp.Status.PolicyHash != "" {
		payload.PolicyHash.SetTo(sp.Status.PolicyHash)
	}

	// Add all classification results
	// Note: Confidence fields removed per DD-SP-001 V1.1
	if sp.Status.EnvironmentClassification != nil {
		if env := toSignalProcessingAuditPayloadEnvironment(string(sp.Status.EnvironmentClassification.Environment)); env != "" {
			payload.Environment.SetTo(env)
		}
		if envSrc := toSignalProcessingAuditPayloadEnvironmentSource(sp.Status.EnvironmentClassification.Source); envSrc != "" {
			payload.EnvironmentSource.SetTo(envSrc)
		}
	}
	if sp.Status.PriorityAssignment != nil {
		if priority := toSignalProcessingAuditPayloadPriority(string(sp.Status.PriorityAssignment.Priority)); priority != "" {
			payload.Priority.SetTo(priority)
		}
		if prioritySrc := toSignalProcessingAuditPayloadPrioritySource(sp.Status.PriorityAssignment.Source); prioritySrc != "" {
			payload.PrioritySource.SetTo(prioritySrc)
		}
	}
	if sp.Status.BusinessClassification != nil {
		if crit := toSignalProcessingAuditPayloadCriticality(string(sp.Status.BusinessClassification.Criticality)); crit != "" {
			payload.Criticality.SetTo(crit)
		}
		if sp.Status.BusinessClassification.SLARequirement != "" {
			payload.SLARequirement.SetTo(sp.Status.BusinessClassification.SLARequirement)
		}
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeClassificationDecision)
	audit.SetEventCategory(event, CategorySignalProcessing)
	audit.SetEventAction(event, "classification")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", "signalprocessing-controller")
	audit.SetResource(event, "SignalProcessing", sp.Name)
	audit.SetDuration(event, durationMs)

	// Graceful degradation: skip audit if no RemediationRequestRef (test edge cases)
	if sp.Spec.RemediationRequestRef.Name == "" {
		c.log.V(1).Info("Skipping classification audit - no RemediationRequestRef")
		return
	}
	audit.SetCorrelationID(event, sp.Spec.RemediationRequestRef.Name)
	audit.SetNamespace(event, sp.Namespace)

	// Set structured payload using union constructor (OGEN-MIGRATION)
	event.EventData = api.NewAuditEventRequestEventDataSignalprocessingClassificationDecisionAuditEventRequestEventData(payload)

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write classification audit")
	}
}

// RecordBusinessClassification records business classification event (AUDIT-06).
// BR-SP-002: Business classification for team ownership, criticality, SLA
// Separate from classification.decision to provide granular audit trail.
func (c *AuditClient) RecordBusinessClassification(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) {
	// Skip if no business classification
	if sp.Status.BusinessClassification == nil {
		c.log.V(1).Info("Skipping business classification audit - no business classification")
		return
	}

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.SignalProcessingAuditPayload{
		EventType: EventTypeBusinessClassified, // Required for discriminator
		Signal:    sp.Spec.Signal.Name,
		Phase:     toSignalProcessingAuditPayloadPhase(string(sp.Status.Phase)),
	}
	if sev := toSignalProcessingAuditPayloadSeverity(sp.Spec.Signal.Severity); sev != "" {
		payload.Severity.SetTo(sev)
	}

	// Add business classification details
	if sp.Status.BusinessClassification.BusinessUnit != "" {
		payload.BusinessUnit.SetTo(sp.Status.BusinessClassification.BusinessUnit)
	}
	if sp.Status.BusinessClassification.Criticality != "" {
		if crit := toSignalProcessingAuditPayloadCriticality(string(sp.Status.BusinessClassification.Criticality)); crit != "" {
			payload.Criticality.SetTo(crit)
		}
	}
	if sp.Status.BusinessClassification.SLARequirement != "" {
		payload.SLARequirement.SetTo(sp.Status.BusinessClassification.SLARequirement)
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeBusinessClassified)
	audit.SetEventCategory(event, CategorySignalProcessing)
	audit.SetEventAction(event, "classification")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", "signalprocessing-controller")
	audit.SetResource(event, "SignalProcessing", sp.Name)

	// Graceful degradation: skip audit if no RemediationRequestRef (test edge cases)
	if sp.Spec.RemediationRequestRef.Name == "" {
		c.log.V(1).Info("Skipping business classification audit - no RemediationRequestRef")
		return
	}
	audit.SetCorrelationID(event, sp.Spec.RemediationRequestRef.Name)
	audit.SetNamespace(event, sp.Namespace)

	// Set structured payload using union constructor (OGEN-MIGRATION)
	event.EventData = api.NewAuditEventRequestEventDataSignalprocessingBusinessClassifiedAuditEventRequestEventData(payload)

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write business classification audit")
	}
}

// RecordEnrichmentComplete records K8s enrichment completion event.
func (c *AuditClient) RecordEnrichmentComplete(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, durationMs int) {
	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.SignalProcessingAuditPayload{
		EventType: EventTypeEnrichmentComplete, // Required for discriminator
		Signal:    sp.Spec.Signal.Name,
		Phase:     toSignalProcessingAuditPayloadPhase(string(sp.Status.Phase)),
	}
	payload.DurationMs.SetTo(durationMs)

	if sp.Status.KubernetesContext != nil {
		payload.HasNamespace.SetTo(sp.Status.KubernetesContext.Namespace != nil)
		payload.HasPod.SetTo(sp.Status.KubernetesContext.Pod != nil)
		payload.HasDeployment.SetTo(sp.Status.KubernetesContext.Deployment != nil)
		payload.OwnerChainLength.SetTo(len(sp.Status.KubernetesContext.OwnerChain))
		payload.DegradedMode.SetTo(sp.Status.KubernetesContext.DegradedMode)
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeEnrichmentComplete)
	audit.SetEventCategory(event, CategorySignalProcessing)
	audit.SetEventAction(event, "enrichment")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", "signalprocessing-controller")
	audit.SetResource(event, "SignalProcessing", sp.Name)
	audit.SetDuration(event, durationMs)

	// Graceful degradation: skip audit if no RemediationRequestRef (test edge cases)
	if sp.Spec.RemediationRequestRef.Name == "" {
		c.log.V(1).Info("Skipping enrichment audit - no RemediationRequestRef")
		return
	}
	audit.SetCorrelationID(event, sp.Spec.RemediationRequestRef.Name)
	audit.SetNamespace(event, sp.Namespace)

	// Set structured payload using union constructor (OGEN-MIGRATION)
	event.EventData = api.NewAuditEventRequestEventDataSignalprocessingEnrichmentCompletedAuditEventRequestEventData(payload)

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write enrichment audit")
	}
}

// RecordError records an error event.
func (c *AuditClient) RecordError(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, phase string, err error) {
	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.SignalProcessingAuditPayload{
		EventType: EventTypeError, // Required for discriminator
		Phase:     toSignalProcessingAuditPayloadPhase(phase),
		Signal:    sp.Spec.Signal.Name,
	}
	payload.Error.SetTo(err.Error())

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeError)
	audit.SetEventCategory(event, CategorySignalProcessing)
	audit.SetEventAction(event, "error")
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, "service", "signalprocessing-controller")
	audit.SetResource(event, "SignalProcessing", sp.Name)

	// Graceful degradation: skip audit if no RemediationRequestRef (test edge cases)
	if sp.Spec.RemediationRequestRef.Name == "" {
		c.log.V(1).Info("Skipping error audit - no RemediationRequestRef")
		return
	}
	audit.SetCorrelationID(event, sp.Spec.RemediationRequestRef.Name)
	audit.SetNamespace(event, sp.Namespace)

	// Set structured payload using union constructor (OGEN-MIGRATION)
	event.EventData = api.NewAuditEventRequestEventDataSignalprocessingErrorOccurredAuditEventRequestEventData(payload)

	if storeErr := c.store.StoreAudit(ctx, event); storeErr != nil {
		c.log.Error(storeErr, "Failed to write error audit")
	}
}
