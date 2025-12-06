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
// DD-AUDIT-002: Uses shared pkg/audit library for consistent audit behavior.
// DD-AUDIT-003: Implements service-specific audit event types.
package audit

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
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
func (c *AuditClient) RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
	// Build event data payload
	eventData := map[string]interface{}{
		"phase":             analysis.Status.Phase,
		"approval_required": analysis.Status.ApprovalRequired,
		"approval_reason":   analysis.Status.ApprovalReason,
		"degraded_mode":     analysis.Status.DegradedMode,
		"warnings_count":    len(analysis.Status.Warnings),
	}
	if analysis.Status.SelectedWorkflow != nil {
		eventData["confidence"] = analysis.Status.SelectedWorkflow.Confidence
		eventData["workflow_id"] = analysis.Status.SelectedWorkflow.WorkflowID
	}
	if analysis.Status.TargetInOwnerChain != nil {
		eventData["target_in_owner_chain"] = *analysis.Status.TargetInOwnerChain
	}
	// Include failure info if present
	if analysis.Status.Reason != "" {
		eventData["reason"] = analysis.Status.Reason
	}
	if analysis.Status.SubReason != "" {
		eventData["sub_reason"] = analysis.Status.SubReason
	}

	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		c.log.Error(err, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := analysis.Namespace

	// Determine outcome
	outcome := "success"
	if analysis.Status.Phase == "Failed" {
		outcome = "failure"
	}

	// Build audit event using pkg/audit.AuditEvent
	event := audit.NewAuditEvent()
	event.EventType = EventTypeAnalysisCompleted
	event.EventCategory = "analysis"
	event.EventAction = "completed"
	event.EventOutcome = outcome
	event.ActorType = "service"
	event.ActorID = "aianalysis-controller"
	event.ResourceType = "AIAnalysis"
	event.ResourceID = analysis.Name
	event.CorrelationID = analysis.Spec.RemediationID
	event.Namespace = &namespace
	event.EventData = eventDataBytes

	// Fire-and-forget (per Risk #4 / DD-AUDIT-002)
	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write audit event",
			"event_type", event.EventType,
			"correlation_id", event.CorrelationID,
		)
		// Don't fail reconciliation on audit failure (graceful degradation)
	}
}

// RecordPhaseTransition records a phase transition event
func (c *AuditClient) RecordPhaseTransition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, from, to string) {
	eventData := map[string]interface{}{
		"from_phase": from,
		"to_phase":   to,
	}
	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		c.log.Error(err, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := analysis.Namespace

	event := audit.NewAuditEvent()
	event.EventType = EventTypePhaseTransition
	event.EventCategory = "analysis"
	event.EventAction = "phase_transition"
	event.EventOutcome = "success"
	event.ActorType = "service"
	event.ActorID = "aianalysis-controller"
	event.ResourceType = "AIAnalysis"
	event.ResourceID = analysis.Name
	event.CorrelationID = analysis.Spec.RemediationID
	event.Namespace = &namespace
	event.EventData = eventDataBytes

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write phase transition audit")
	}
}

// RecordError records an error event
func (c *AuditClient) RecordError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, phase string, err error) {
	eventData := map[string]interface{}{
		"phase": phase,
		"error": err.Error(),
	}
	eventDataBytes, marshalErr := json.Marshal(eventData)
	if marshalErr != nil {
		c.log.Error(marshalErr, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := analysis.Namespace
	errorMsg := err.Error()

	event := audit.NewAuditEvent()
	event.EventType = EventTypeError
	event.EventCategory = "analysis"
	event.EventAction = "error"
	event.EventOutcome = "failure"
	event.ActorType = "service"
	event.ActorID = "aianalysis-controller"
	event.ResourceType = "AIAnalysis"
	event.ResourceID = analysis.Name
	event.CorrelationID = analysis.Spec.RemediationID
	event.Namespace = &namespace
	event.EventData = eventDataBytes
	event.ErrorMessage = &errorMsg

	if storeErr := c.store.StoreAudit(ctx, event); storeErr != nil {
		c.log.Error(storeErr, "Failed to write error audit")
	}
}

// RecordHolmesGPTCall records a HolmesGPT-API call event
func (c *AuditClient) RecordHolmesGPTCall(ctx context.Context, analysis *aianalysisv1.AIAnalysis, endpoint string, statusCode int, durationMs int) {
	eventData := map[string]interface{}{
		"endpoint":    endpoint,
		"status_code": statusCode,
		"duration_ms": durationMs,
	}
	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		c.log.Error(err, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := analysis.Namespace
	outcome := "success"
	if statusCode >= 400 {
		outcome = "failure"
	}

	event := audit.NewAuditEvent()
	event.EventType = EventTypeHolmesGPTCall
	event.EventCategory = "analysis"
	event.EventAction = "api_call"
	event.EventOutcome = outcome
	event.ActorType = "service"
	event.ActorID = "aianalysis-controller"
	event.ResourceType = "AIAnalysis"
	event.ResourceID = analysis.Name
	event.CorrelationID = analysis.Spec.RemediationID
	event.Namespace = &namespace
	event.EventData = eventDataBytes
	event.DurationMs = &durationMs

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write HolmesGPT call audit")
	}
}

// RecordApprovalDecision records an approval decision event
func (c *AuditClient) RecordApprovalDecision(ctx context.Context, analysis *aianalysisv1.AIAnalysis, decision string, reason string) {
	eventData := map[string]interface{}{
		"decision":    decision,
		"reason":      reason,
		"environment": analysis.Spec.AnalysisRequest.SignalContext.Environment,
	}
	if analysis.Status.SelectedWorkflow != nil {
		eventData["confidence"] = analysis.Status.SelectedWorkflow.Confidence
		eventData["workflow_id"] = analysis.Status.SelectedWorkflow.WorkflowID
	}

	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		c.log.Error(err, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := analysis.Namespace

	event := audit.NewAuditEvent()
	event.EventType = EventTypeApprovalDecision
	event.EventCategory = "analysis"
	event.EventAction = "approval_decision"
	event.EventOutcome = "success"
	event.ActorType = "service"
	event.ActorID = "aianalysis-controller"
	event.ResourceType = "AIAnalysis"
	event.ResourceID = analysis.Name
	event.CorrelationID = analysis.Spec.RemediationID
	event.Namespace = &namespace
	event.EventData = eventDataBytes

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write approval decision audit")
	}
}

// RecordRegoEvaluation records a Rego policy evaluation event
func (c *AuditClient) RecordRegoEvaluation(ctx context.Context, analysis *aianalysisv1.AIAnalysis, outcome string, degraded bool, durationMs int) {
	eventData := map[string]interface{}{
		"outcome":     outcome,
		"degraded":    degraded,
		"duration_ms": durationMs,
	}
	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		c.log.Error(err, "Failed to marshal event data")
		eventDataBytes = []byte("{}")
	}

	namespace := analysis.Namespace

	event := audit.NewAuditEvent()
	event.EventType = EventTypeRegoEvaluation
	event.EventCategory = "analysis"
	event.EventAction = "policy_evaluation"
	event.EventOutcome = outcome
	event.ActorType = "service"
	event.ActorID = "aianalysis-controller"
	event.ResourceType = "AIAnalysis"
	event.ResourceID = analysis.Name
	event.CorrelationID = analysis.Spec.RemediationID
	event.Namespace = &namespace
	event.EventData = eventDataBytes
	event.DurationMs = &durationMs

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write Rego evaluation audit")
	}
}

