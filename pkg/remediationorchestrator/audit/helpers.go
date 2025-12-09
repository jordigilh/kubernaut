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

// Package audit provides audit event helpers for Remediation Orchestrator.
//
// Business Requirements:
// - BR-STORAGE-001: Complete audit trail with no data loss
// - DD-AUDIT-003: All services must emit audit events
//
// Authority: ADR-034 (Unified Audit Table), DD-AUDIT-002 (Audit Shared Library)
package audit

import (
	"encoding/json"
	"time"

	"github.com/jordigilh/kubernaut/pkg/audit"
)

// ServiceName is the canonical service identifier for audit events.
const ServiceName = "remediation-orchestrator"

// Event categories for RO audit events
const (
	CategoryRemediation = "remediation"
	CategoryApproval    = "approval"
	CategoryPhase       = "phase"
)

// Event actions for RO audit events
const (
	ActionPhaseTransition   = "phase_transition"
	ActionCreated           = "created"
	ActionCompleted         = "completed"
	ActionFailed            = "failed"
	ActionApprovalRequested = "approval_requested"
	ActionApproved          = "approved"
	ActionRejected          = "rejected"
	ActionExpired           = "expired"
	ActionManualReview      = "manual_review_required"
)

// Helpers provides audit event building helpers for RO.
type Helpers struct {
	serviceName string
}

// NewHelpers creates a new Helpers instance.
func NewHelpers(serviceName string) *Helpers {
	return &Helpers{
		serviceName: serviceName,
	}
}

// PhaseTransitionData is the event_data for phase transition events.
type PhaseTransitionData struct {
	FromPhase string `json:"from_phase"`
	ToPhase   string `json:"to_phase"`
	Namespace string `json:"namespace"`
	RRName    string `json:"rr_name"`
}

// BuildPhaseTransitionEvent builds an audit event for phase transitions.
func (h *Helpers) BuildPhaseTransitionEvent(
	correlationID string,
	namespace string,
	rrName string,
	fromPhase string,
	toPhase string,
) (*audit.AuditEvent, error) {
	event := audit.NewAuditEvent()

	// Event classification
	event.EventType = "ro.phase.transition"
	event.EventCategory = CategoryPhase
	event.EventAction = ActionPhaseTransition
	event.EventOutcome = "success"

	// Actor (service)
	event.ActorType = "service"
	event.ActorID = h.serviceName

	// Resource (RemediationRequest)
	event.ResourceType = "RemediationRequest"
	event.ResourceID = rrName
	event.CorrelationID = correlationID

	// Namespace
	event.Namespace = &namespace

	// Event data
	data := PhaseTransitionData{
		FromPhase: fromPhase,
		ToPhase:   toPhase,
		Namespace: namespace,
		RRName:    rrName,
	}
	eventData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	event.EventData = eventData

	return event, nil
}

// CompletionData is the event_data for remediation completion events.
type CompletionData struct {
	Outcome       string `json:"outcome"`
	DurationMs    int64  `json:"duration_ms"`
	Namespace     string `json:"namespace"`
	RRName        string `json:"rr_name"`
	FailurePhase  string `json:"failure_phase,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// BuildCompletionEvent builds an audit event for remediation completion.
func (h *Helpers) BuildCompletionEvent(
	correlationID string,
	namespace string,
	rrName string,
	outcome string,
	durationMs int64,
) (*audit.AuditEvent, error) {
	event := audit.NewAuditEvent()

	// Event classification
	event.EventType = "ro.remediation.completed"
	event.EventCategory = CategoryRemediation
	event.EventAction = ActionCompleted
	event.EventOutcome = "success"

	// Actor (service)
	event.ActorType = "service"
	event.ActorID = h.serviceName

	// Resource (RemediationRequest)
	event.ResourceType = "RemediationRequest"
	event.ResourceID = rrName
	event.CorrelationID = correlationID

	// Namespace
	event.Namespace = &namespace

	// Duration
	durationMsInt := int(durationMs)
	event.DurationMs = &durationMsInt

	// Event data
	data := CompletionData{
		Outcome:    outcome,
		DurationMs: durationMs,
		Namespace:  namespace,
		RRName:     rrName,
	}
	eventData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	event.EventData = eventData

	return event, nil
}

// BuildFailureEvent builds an audit event for remediation failure.
func (h *Helpers) BuildFailureEvent(
	correlationID string,
	namespace string,
	rrName string,
	failurePhase string,
	failureReason string,
	durationMs int64,
) (*audit.AuditEvent, error) {
	event := audit.NewAuditEvent()

	// Event classification
	event.EventType = "ro.remediation.failed"
	event.EventCategory = CategoryRemediation
	event.EventAction = ActionFailed
	event.EventOutcome = "failure"

	// Actor (service)
	event.ActorType = "service"
	event.ActorID = h.serviceName

	// Resource (RemediationRequest)
	event.ResourceType = "RemediationRequest"
	event.ResourceID = rrName
	event.CorrelationID = correlationID

	// Namespace
	event.Namespace = &namespace

	// Error details
	event.ErrorMessage = &failureReason

	// Duration
	durationMsInt := int(durationMs)
	event.DurationMs = &durationMsInt

	// Event data
	data := CompletionData{
		Outcome:       "Failed",
		DurationMs:    durationMs,
		Namespace:     namespace,
		RRName:        rrName,
		FailurePhase:  failurePhase,
		FailureReason: failureReason,
	}
	eventData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	event.EventData = eventData

	return event, nil
}

// ApprovalData is the event_data for approval-related events.
type ApprovalData struct {
	RARName       string `json:"rar_name"`
	RRName        string `json:"rr_name"`
	Namespace     string `json:"namespace"`
	Decision      string `json:"decision,omitempty"`
	DecidedBy     string `json:"decided_by,omitempty"`
	Message       string `json:"message,omitempty"`
	RequiredBy    string `json:"required_by,omitempty"`
	WorkflowID    string `json:"workflow_id,omitempty"`
	ConfidenceStr string `json:"confidence,omitempty"`
}

// BuildApprovalRequestedEvent builds an audit event for approval requested.
func (h *Helpers) BuildApprovalRequestedEvent(
	correlationID string,
	namespace string,
	rrName string,
	rarName string,
	workflowID string,
	confidence string,
	requiredBy time.Time,
) (*audit.AuditEvent, error) {
	event := audit.NewAuditEvent()

	// Event classification
	event.EventType = "ro.approval.requested"
	event.EventCategory = CategoryApproval
	event.EventAction = ActionApprovalRequested
	event.EventOutcome = "success"

	// Actor (service)
	event.ActorType = "service"
	event.ActorID = h.serviceName

	// Resource (RemediationApprovalRequest)
	event.ResourceType = "RemediationApprovalRequest"
	event.ResourceID = rarName
	event.CorrelationID = correlationID

	// Namespace
	event.Namespace = &namespace

	// Event data
	data := ApprovalData{
		RARName:       rarName,
		RRName:        rrName,
		Namespace:     namespace,
		RequiredBy:    requiredBy.Format(time.RFC3339),
		WorkflowID:    workflowID,
		ConfidenceStr: confidence,
	}
	eventData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	event.EventData = eventData

	return event, nil
}

// BuildApprovalDecisionEvent builds an audit event for approval decision.
func (h *Helpers) BuildApprovalDecisionEvent(
	correlationID string,
	namespace string,
	rrName string,
	rarName string,
	decision string,
	decidedBy string,
	message string,
) (*audit.AuditEvent, error) {
	event := audit.NewAuditEvent()

	// Determine action and outcome based on decision
	var action, outcome string
	switch decision {
	case "Approved":
		action = ActionApproved
		outcome = "success"
	case "Rejected":
		action = ActionRejected
		outcome = "failure"
	case "Expired":
		action = ActionExpired
		outcome = "failure"
	default:
		action = decision
		outcome = "unknown"
	}

	// Event classification
	event.EventType = "ro.approval." + action
	event.EventCategory = CategoryApproval
	event.EventAction = action
	event.EventOutcome = outcome

	// Actor (user or system)
	if decidedBy == "system" {
		event.ActorType = "service"
		event.ActorID = h.serviceName
	} else {
		event.ActorType = "user"
		event.ActorID = decidedBy
	}

	// Resource (RemediationApprovalRequest)
	event.ResourceType = "RemediationApprovalRequest"
	event.ResourceID = rarName
	event.CorrelationID = correlationID

	// Namespace
	event.Namespace = &namespace

	// Event data
	data := ApprovalData{
		RARName:   rarName,
		RRName:    rrName,
		Namespace: namespace,
		Decision:  decision,
		DecidedBy: decidedBy,
		Message:   message,
	}
	eventData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	event.EventData = eventData

	return event, nil
}

// ManualReviewData is the event_data for manual review required events.
type ManualReviewData struct {
	RRName        string `json:"rr_name"`
	Namespace     string `json:"namespace"`
	Reason        string `json:"reason"`
	SubReason     string `json:"sub_reason,omitempty"`
	NotificationN string `json:"notification_name,omitempty"`
}

// BuildManualReviewEvent builds an audit event for manual review required.
func (h *Helpers) BuildManualReviewEvent(
	correlationID string,
	namespace string,
	rrName string,
	reason string,
	subReason string,
	notificationName string,
) (*audit.AuditEvent, error) {
	event := audit.NewAuditEvent()

	// Event classification
	event.EventType = "ro.remediation.manual_review"
	event.EventCategory = CategoryRemediation
	event.EventAction = ActionManualReview
	event.EventOutcome = "pending"

	// Actor (service)
	event.ActorType = "service"
	event.ActorID = h.serviceName

	// Resource (RemediationRequest)
	event.ResourceType = "RemediationRequest"
	event.ResourceID = rrName
	event.CorrelationID = correlationID

	// Namespace
	event.Namespace = &namespace

	// Severity (manual review is warning level)
	severity := "warning"
	event.Severity = &severity

	// Event data
	data := ManualReviewData{
		RRName:        rrName,
		Namespace:     namespace,
		Reason:        reason,
		SubReason:     subReason,
		NotificationN: notificationName,
	}
	eventData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	event.EventData = eventData

	return event, nil
}

