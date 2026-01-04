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

// Package audit provides audit event generation for the AIAnalysis controller.
// DD-AUDIT-002 V2.2: Uses shared pkg/audit library with zero unstructured data.
// DD-AUDIT-003: Implements service-specific audit event types.
// DD-AUDIT-004 V1.3: Direct struct assignment to SetEventData (no map conversion).
package audit

import (
	"context"

	"github.com/go-logr/logr"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// Event type constants (per DD-AUDIT-003)
const (
	EventTypeAnalysisCompleted = "aianalysis.analysis.completed"
	EventTypePhaseTransition   = "aianalysis.phase.transition"
	EventTypeHolmesGPTCall     = "aianalysis.holmesgpt.call"
	EventTypeApprovalDecision  = "aianalysis.approval.decision"
	EventTypeRegoEvaluation    = "aianalysis.rego.evaluation"
	EventTypeError             = "aianalysis.error.occurred"
)

// Event category constant (per DD-AUDIT-003)
// FIXED: Changed from "aianalysis" to "analysis" to match OpenAPI schema enum (api/openapi/data-storage-v1.yaml:832-920)
const (
	EventCategoryAIAnalysis = "analysis"
)

// Event action constants (per DD-AUDIT-003)
const (
	EventActionAnalysisComplete = "analysis_complete"
	EventActionPhaseTransition  = "phase_transition"
	EventActionError            = "error"
	EventActionHolmesGPTCall    = "holmesgpt_call"
	EventActionApprovalDecision = "approval_decision"
	EventActionPolicyEvaluation = "policy_evaluation"
)

// Actor constants (per DD-AUDIT-003)
const (
	ActorTypeService            = "service"
	ActorIDAIAnalysisController = "aianalysis-controller"
)

// AuditClient handles audit event storage using pkg/audit shared library
type AuditClient struct {
	store audit.AuditStore // Uses shared library interface
	log   logr.Logger
}

// NewAuditClient creates a new audit client
func NewAuditClient(store audit.AuditStore, log logr.Logger) *AuditClient {
	return &AuditClient{
		store: store,
		log:   log.WithName("audit"),
	}
}

// RecordAnalysisComplete records analysis completion event
// This is the primary audit event for AIAnalysis (per DD-AUDIT-003)
//
// Uses structured types per DD-AUDIT-004 for compile-time type safety.
func (c *AuditClient) RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
	// Build structured payload (DD-AUDIT-004: Type-safe event data)
	payload := AnalysisCompletePayload{
		Phase:            analysis.Status.Phase,
		ApprovalRequired: analysis.Status.ApprovalRequired,
		ApprovalReason:   analysis.Status.ApprovalReason,
		DegradedMode:     analysis.Status.DegradedMode,
		WarningsCount:    len(analysis.Status.Warnings),
	}

	// Conditional fields (type-safe pointers)
	if analysis.Status.SelectedWorkflow != nil {
		payload.Confidence = &analysis.Status.SelectedWorkflow.Confidence
		payload.WorkflowID = &analysis.Status.SelectedWorkflow.WorkflowID
	}
	if analysis.Status.TargetInOwnerChain != nil {
		payload.TargetInOwnerChain = analysis.Status.TargetInOwnerChain
	}
	if analysis.Status.Reason != "" {
		payload.Reason = analysis.Status.Reason
	}
	if analysis.Status.SubReason != "" {
		payload.SubReason = analysis.Status.SubReason
	}

	// Determine outcome
	var apiOutcome dsgen.AuditEventRequestEventOutcome
	if analysis.Status.Phase == "Failed" {
		apiOutcome = audit.OutcomeFailure
	} else {
		apiOutcome = audit.OutcomeSuccess
	}

	// Build audit event (DD-AUDIT-002 V2.2: Direct struct assignment)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeAnalysisCompleted)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionAnalysisComplete) // Fixed: Must match test contract
	audit.SetEventOutcome(event, apiOutcome)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	// Fire-and-forget (per Risk #4 / DD-AUDIT-002)
	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write audit event",
			"event_type", event.EventType,
			"correlation_id", event.CorrelationId,
		)
		// Don't fail reconciliation on audit failure (graceful degradation)
	}
}

// RecordPhaseTransition records a phase transition event
//
// Uses structured types per DD-AUDIT-004 for compile-time type safety.
func (c *AuditClient) RecordPhaseTransition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, from, to string) {
	// DEBUG AA-BUG-001: Log every call to understand why events aren't being created
	c.log.Info("🔍 [AA-BUG-001 DEBUG] RecordPhaseTransition called",
		"from", from,
		"to", to,
		"name", analysis.Name,
		"correlationID", analysis.Spec.RemediationID)
	
	// Idempotency check: Only record if phase actually changed
	if from == to {
		c.log.V(1).Info("Skipping phase transition audit - phase unchanged",
			"phase", from,
			"name", analysis.Name,
			"namespace", analysis.Namespace)
		return
	}

	// Build structured payload (DD-AUDIT-004: Type-safe event data)
	payload := PhaseTransitionPayload{
		OldPhase: from,
		NewPhase: to,
	}

	// Build audit event (DD-AUDIT-002 V2.2: Direct struct assignment)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypePhaseTransition)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionPhaseTransition)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write phase transition audit")
	}
}

// RecordError records an error event
//
// Uses structured types per DD-AUDIT-004 for compile-time type safety.
func (c *AuditClient) RecordError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, phase string, err error) {
	// Build structured payload (DD-AUDIT-004: Type-safe event data)
	payload := ErrorPayload{
		Phase:        phase,
		ErrorMessage: err.Error(),
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeError)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionError)
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	// Note: error_message DB column is populated by Data Storage from event_data JSON
	// Per ADR-034: Error details stored in event_data for query-ability

	if storeErr := c.store.StoreAudit(ctx, event); storeErr != nil {
		c.log.Error(storeErr, "Failed to write error audit")
	}
}

// RecordHolmesGPTCall records a HolmesGPT-API call event
//
// Uses structured types per DD-AUDIT-004 for compile-time type safety.
func (c *AuditClient) RecordHolmesGPTCall(ctx context.Context, analysis *aianalysisv1.AIAnalysis, endpoint string, statusCode int, durationMs int) {
	// Build structured payload (DD-AUDIT-004: Type-safe event data)
	payload := HolmesGPTCallPayload{
		Endpoint:       endpoint,
		HTTPStatusCode: statusCode,
		DurationMs:     durationMs,
	}

	// Determine outcome
	var apiOutcome dsgen.AuditEventRequestEventOutcome
	if statusCode >= 400 {
		apiOutcome = audit.OutcomeFailure
	} else {
		apiOutcome = audit.OutcomeSuccess
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeHolmesGPTCall)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionHolmesGPTCall) // Fixed: Must match test contract
	audit.SetEventOutcome(event, apiOutcome)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetDuration(event, durationMs)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write HolmesGPT call audit")
	}
}

// RecordApprovalDecision records an approval decision event
//
// Uses structured types per DD-AUDIT-004 for compile-time type safety.
func (c *AuditClient) RecordApprovalDecision(ctx context.Context, analysis *aianalysisv1.AIAnalysis, decision string, reason string) {
	// Derive boolean flags from decision string
	approvalRequired := decision == "requires_approval"
	autoApproved := decision == "auto_approved"

	// Build structured payload (DD-AUDIT-004: Type-safe event data)
	payload := ApprovalDecisionPayload{
		ApprovalRequired: approvalRequired,
		ApprovalReason:   reason,
		AutoApproved:     autoApproved,
		Decision:         decision,
		Reason:           reason,
		Environment:      analysis.Spec.AnalysisRequest.SignalContext.Environment,
	}

	// Conditional fields (type-safe pointers)
	if analysis.Status.SelectedWorkflow != nil {
		payload.Confidence = &analysis.Status.SelectedWorkflow.Confidence
		payload.WorkflowID = &analysis.Status.SelectedWorkflow.WorkflowID
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeApprovalDecision)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionApprovalDecision)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write approval decision audit")
	}
}

// RecordRegoEvaluation records a Rego policy evaluation event
//
// Uses structured types per DD-AUDIT-004 for compile-time type safety.
func (c *AuditClient) RecordRegoEvaluation(ctx context.Context, analysis *aianalysisv1.AIAnalysis, outcome string, degraded bool, durationMs int, reason string) {
	// Build structured payload (DD-AUDIT-004: Type-safe event data)
	payload := RegoEvaluationPayload{
		Outcome:    outcome,
		Degraded:   degraded,
		DurationMs: durationMs,
		Reason:     reason,
	}

	// Map outcome to OpenAPI enum
	var apiOutcome dsgen.AuditEventRequestEventOutcome
	switch outcome {
	case "allow", "success", "requires_approval", "auto_approved": // Rego policy decision outcomes
		apiOutcome = audit.OutcomeSuccess
	case "deny", "failure":
		apiOutcome = audit.OutcomeFailure
	default:
		apiOutcome = audit.OutcomePending
	}

	// Build audit event (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeRegoEvaluation)
	audit.SetEventCategory(event, EventCategoryAIAnalysis)
	audit.SetEventAction(event, EventActionPolicyEvaluation)
	audit.SetEventOutcome(event, apiOutcome)
	audit.SetActor(event, ActorTypeService, ActorIDAIAnalysisController)
	audit.SetResource(event, "AIAnalysis", analysis.Name)
	audit.SetCorrelationID(event, analysis.Spec.RemediationID)
	audit.SetNamespace(event, analysis.Namespace)
	audit.SetDuration(event, durationMs)
	audit.SetEventData(event, payload) // V2.2: Direct struct assignment

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write Rego evaluation audit")
	}
}
