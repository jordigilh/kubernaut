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

// Package audit provides audit event generation for RemediationApprovalRequest controller.
// BR-AUDIT-006: Approval decision audit trail (SOC 2 CC8.1 User Attribution)
// DD-AUDIT-002 V2.2: Uses shared pkg/audit library with zero unstructured data.
// DD-AUDIT-003: Implements service-specific audit event types.
package audit

import (
	"context"

	"github.com/go-logr/logr"

	remediationapprovalrequestv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// Event categories
const (
	EventCategoryApproval = "approval"
)

// Event types
const (
	EventTypeApprovalDecision      = "approval.decision"       // P0 - SOC 2 critical
	EventTypeApprovalRequestCreated = "approval.request.created" // P1 - context
	EventTypeApprovalTimeout       = "approval.timeout"        // P1 - operational
)

// Event actions
const (
	EventActionDecisionMade   = "decision_made"
	EventActionRequestCreated = "request_created"
	EventActionTimeout        = "timeout"
)

// Actor types
const (
	ActorTypeService = "service"
	ActorTypeUser    = "user"
	ActorTypeSystem  = "system"
)

// Actor IDs
const (
	ActorIDController = "remediationapprovalrequest-controller"
)

// AuditClient handles audit event generation for RemediationApprovalRequest
type AuditClient struct {
	store audit.AuditStore
	log   logr.Logger
}

// NewAuditClient creates a new audit client
func NewAuditClient(store audit.AuditStore, log logr.Logger) *AuditClient {
	return &AuditClient{
		store: store,
		log:   log,
	}
}

// RecordApprovalDecision records approval decision event (P0 - SOC 2 critical)
//
// This method captures WHO, WHEN, WHAT, WHY for all approval decisions to satisfy
// SOC 2 CC8.1 (User Attribution) and CC6.8 (Non-Repudiation) requirements.
//
// Idempotency: Only emits event when decision is non-empty (prevents duplicate events).
//
// Fire-and-Forget: Logs error but doesn't fail controller reconciliation on audit failure
// (per DD-AUDIT-002 graceful degradation pattern).
//
// BR-AUDIT-006: Approval decision audit trail
// DD-AUDIT-006: RAR audit implementation
func (c *AuditClient) RecordApprovalDecision(ctx context.Context, rar *remediationapprovalrequestv1alpha1.RemediationApprovalRequest) {
	// Idempotency: Only emit if decision is final
	if rar.Status.Decision == "" {
		return
	}

	// Build structured payload (BR-AUDIT-006: Complete approval context)
	payload := c.buildApprovalDecisionPayload(rar)

	// Determine outcome (approved=success, rejected/expired=failure)
	apiOutcome := c.determineEventOutcome(rar.Status.Decision)

	// Build audit event using shared library (DD-AUDIT-002)
	event := c.buildAuditEvent(rar, apiOutcome, payload)

	// Fire-and-forget: Store audit event (DD-AUDIT-002)
	c.storeAuditEvent(ctx, event, rar.Status.Decision)
}

// buildApprovalDecisionPayload constructs the audit payload from RAR
func (c *AuditClient) buildApprovalDecisionPayload(rar *remediationapprovalrequestv1alpha1.RemediationApprovalRequest) ogenclient.RemediationApprovalDecisionPayload {
	payload := ogenclient.RemediationApprovalDecisionPayload{
		EventType:              ogenclient.RemediationApprovalDecisionPayloadEventTypeApprovalDecision,
		RemediationRequestName: rar.Spec.RemediationRequestRef.Name,
		AiAnalysisName:         rar.Spec.AIAnalysisRef.Name,
		Decision:               mapDecisionToPayloadEnum(rar.Status.Decision),
		DecidedBy:              rar.Status.DecidedBy,
		Confidence:             float32(rar.Spec.Confidence),
		WorkflowID:             rar.Spec.RecommendedWorkflow.WorkflowID,
	}

	// Set optional fields using .SetTo() pattern
	if rar.Status.DecidedAt != nil {
		payload.DecidedAt.SetTo(rar.Status.DecidedAt.Time)
	}
	if rar.Status.DecisionMessage != "" {
		payload.DecisionMessage.SetTo(rar.Status.DecisionMessage)
	}
	if rar.Spec.RecommendedWorkflow.Version != "" {
		payload.WorkflowVersion.SetTo(rar.Spec.RecommendedWorkflow.Version)
	}

	return payload
}

// determineEventOutcome maps approval decision to audit outcome
func (c *AuditClient) determineEventOutcome(decision remediationapprovalrequestv1alpha1.ApprovalDecision) ogenclient.AuditEventRequestEventOutcome {
	if decision == remediationapprovalrequestv1alpha1.ApprovalDecisionApproved {
		return audit.OutcomeSuccess
	}
	return audit.OutcomeFailure
}

// buildAuditEvent constructs the complete audit event structure
func (c *AuditClient) buildAuditEvent(
	rar *remediationapprovalrequestv1alpha1.RemediationApprovalRequest,
	outcome ogenclient.AuditEventRequestEventOutcome,
	payload ogenclient.RemediationApprovalDecisionPayload,
) *ogenclient.AuditEventRequest {
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"

	// Set event metadata
	audit.SetEventType(event, EventTypeApprovalDecision)
	audit.SetEventCategory(event, EventCategoryApproval)
	audit.SetEventAction(event, EventActionDecisionMade)
	audit.SetEventOutcome(event, outcome)

	// Set actor: authenticated user from webhook (SOC 2 CC8.1)
	audit.SetActor(event, ActorTypeUser, rar.Status.DecidedBy)

	// Set resource
	audit.SetResource(event, "RemediationApprovalRequest", rar.Name)

	// Set correlation ID: parent RR name (DD-AUDIT-CORRELATION-002)
	// This enables querying all events for a remediation by single correlation_id
	audit.SetCorrelationID(event, rar.Spec.RemediationRequestRef.Name)

	audit.SetNamespace(event, rar.Namespace)

	// Set structured payload using ogen union constructor
	event.EventData = ogenclient.NewRemediationApprovalDecisionPayloadAuditEventRequestEventData(payload)

	return event
}

// storeAuditEvent stores the audit event with error handling
func (c *AuditClient) storeAuditEvent(ctx context.Context, event *ogenclient.AuditEventRequest, decision remediationapprovalrequestv1alpha1.ApprovalDecision) {
	if err := c.store.StoreAudit(ctx, event); err != nil {
		// Log error but don't fail reconciliation (graceful degradation per DD-AUDIT-002)
		c.log.Error(err, "Failed to write approval decision audit event",
			"event_type", event.EventType,
			"correlation_id", event.CorrelationID,
			"decision", decision,
		)
	}
}

// mapDecisionToPayloadEnum maps CRD decision to ogen enum
func mapDecisionToPayloadEnum(decision remediationapprovalrequestv1alpha1.ApprovalDecision) ogenclient.RemediationApprovalDecisionPayloadDecision {
	switch decision {
	case remediationapprovalrequestv1alpha1.ApprovalDecisionApproved:
		return ogenclient.RemediationApprovalDecisionPayloadDecisionApproved
	case remediationapprovalrequestv1alpha1.ApprovalDecisionRejected:
		return ogenclient.RemediationApprovalDecisionPayloadDecisionRejected
	case remediationapprovalrequestv1alpha1.ApprovalDecisionExpired:
		return ogenclient.RemediationApprovalDecisionPayloadDecisionExpired
	default:
		return ogenclient.RemediationApprovalDecisionPayloadDecisionApproved
	}
}
