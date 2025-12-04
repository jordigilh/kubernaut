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

// Package workflowexecution provides audit trail for the WorkflowExecution controller
// TDD GREEN: Implementation driven by failing tests in audit_test.go
package workflowexecution

import (
	"time"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// Audit event type constants
const (
	AuditEventTypeCreated            = "Created"
	AuditEventTypePhaseTransition    = "PhaseTransition"
	AuditEventTypeCompleted          = "Completed"
	AuditEventTypeFailed             = "Failed"
	AuditEventTypeSkipped            = "Skipped"
	AuditEventTypePipelineRunCreated = "PipelineRunCreated"
	AuditEventTypeDeleted            = "Deleted"
)

// AuditEvent represents an audit event for WorkflowExecution lifecycle
// TDD: Fields defined by test requirements
type AuditEvent struct {
	// EventType identifies the type of audit event
	EventType string `json:"eventType"`

	// Timestamp when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// ResourceName is the name of the WorkflowExecution
	ResourceName string `json:"resourceName"`

	// ResourceNamespace is the namespace of the WorkflowExecution
	ResourceNamespace string `json:"resourceNamespace"`

	// ResourceUID is the unique identifier
	ResourceUID string `json:"resourceUid"`

	// WorkflowID from the WorkflowRef
	WorkflowID string `json:"workflowId"`

	// TargetResource being remediated
	TargetResource string `json:"targetResource"`

	// Phase transition details
	FromPhase string `json:"fromPhase,omitempty"`
	ToPhase   string `json:"toPhase,omitempty"`

	// Outcome (Success, Failure, Skipped)
	Outcome string `json:"outcome,omitempty"`

	// Duration of execution
	Duration string `json:"duration,omitempty"`

	// Failure details
	FailureReason  string `json:"failureReason,omitempty"`
	FailureMessage string `json:"failureMessage,omitempty"`

	// Skip details
	SkipReason string `json:"skipReason,omitempty"`

	// PipelineRun reference
	PipelineRunName string `json:"pipelineRunName,omitempty"`
}

// AuditHelper provides methods to build audit events
// TDD: Struct defined by test requirements
type AuditHelper struct{}

// NewAuditHelper creates a new audit helper
// TDD GREEN: Constructor defined by test requirements
func NewAuditHelper() *AuditHelper {
	return &AuditHelper{}
}

// BuildCreatedEvent creates an audit event for WFE creation
func (h *AuditHelper) BuildCreatedEvent(wfe *workflowexecutionv1.WorkflowExecution) *AuditEvent {
	return &AuditEvent{
		EventType:         AuditEventTypeCreated,
		Timestamp:         time.Now(),
		ResourceName:      wfe.Name,
		ResourceNamespace: wfe.Namespace,
		ResourceUID:       string(wfe.UID),
		WorkflowID:        wfe.Spec.WorkflowRef.WorkflowID,
		TargetResource:    wfe.Spec.TargetResource,
	}
}

// BuildPhaseTransitionEvent creates an audit event for phase transition
func (h *AuditHelper) BuildPhaseTransitionEvent(wfe *workflowexecutionv1.WorkflowExecution, fromPhase, toPhase string) *AuditEvent {
	return &AuditEvent{
		EventType:         AuditEventTypePhaseTransition,
		Timestamp:         time.Now(),
		ResourceName:      wfe.Name,
		ResourceNamespace: wfe.Namespace,
		ResourceUID:       string(wfe.UID),
		WorkflowID:        wfe.Spec.WorkflowRef.WorkflowID,
		TargetResource:    wfe.Spec.TargetResource,
		FromPhase:         fromPhase,
		ToPhase:           toPhase,
	}
}

// BuildCompletedEvent creates an audit event for successful completion
func (h *AuditHelper) BuildCompletedEvent(wfe *workflowexecutionv1.WorkflowExecution) *AuditEvent {
	return &AuditEvent{
		EventType:         AuditEventTypeCompleted,
		Timestamp:         time.Now(),
		ResourceName:      wfe.Name,
		ResourceNamespace: wfe.Namespace,
		ResourceUID:       string(wfe.UID),
		WorkflowID:        wfe.Spec.WorkflowRef.WorkflowID,
		TargetResource:    wfe.Spec.TargetResource,
		Outcome:           "Success",
		Duration:          wfe.Status.Duration,
	}
}

// BuildFailedEvent creates an audit event for failure
func (h *AuditHelper) BuildFailedEvent(wfe *workflowexecutionv1.WorkflowExecution) *AuditEvent {
	event := &AuditEvent{
		EventType:         AuditEventTypeFailed,
		Timestamp:         time.Now(),
		ResourceName:      wfe.Name,
		ResourceNamespace: wfe.Namespace,
		ResourceUID:       string(wfe.UID),
		WorkflowID:        wfe.Spec.WorkflowRef.WorkflowID,
		TargetResource:    wfe.Spec.TargetResource,
		Outcome:           "Failure",
	}

	if wfe.Status.FailureDetails != nil {
		event.FailureReason = wfe.Status.FailureDetails.Reason
		event.FailureMessage = wfe.Status.FailureDetails.Message
	}

	return event
}

// BuildSkippedEvent creates an audit event for skipped execution
func (h *AuditHelper) BuildSkippedEvent(wfe *workflowexecutionv1.WorkflowExecution) *AuditEvent {
	event := &AuditEvent{
		EventType:         AuditEventTypeSkipped,
		Timestamp:         time.Now(),
		ResourceName:      wfe.Name,
		ResourceNamespace: wfe.Namespace,
		ResourceUID:       string(wfe.UID),
		WorkflowID:        wfe.Spec.WorkflowRef.WorkflowID,
		TargetResource:    wfe.Spec.TargetResource,
		Outcome:           "Skipped",
	}

	if wfe.Status.SkipDetails != nil {
		event.SkipReason = wfe.Status.SkipDetails.Reason
	}

	return event
}

// BuildPipelineRunCreatedEvent creates an audit event for PipelineRun creation
func (h *AuditHelper) BuildPipelineRunCreatedEvent(wfe *workflowexecutionv1.WorkflowExecution, pipelineRunName string) *AuditEvent {
	return &AuditEvent{
		EventType:         AuditEventTypePipelineRunCreated,
		Timestamp:         time.Now(),
		ResourceName:      wfe.Name,
		ResourceNamespace: wfe.Namespace,
		ResourceUID:       string(wfe.UID),
		WorkflowID:        wfe.Spec.WorkflowRef.WorkflowID,
		TargetResource:    wfe.Spec.TargetResource,
		PipelineRunName:   pipelineRunName,
	}
}

// BuildDeletedEvent creates an audit event for WFE deletion
func (h *AuditHelper) BuildDeletedEvent(wfe *workflowexecutionv1.WorkflowExecution) *AuditEvent {
	return &AuditEvent{
		EventType:         AuditEventTypeDeleted,
		Timestamp:         time.Now(),
		ResourceName:      wfe.Name,
		ResourceNamespace: wfe.Namespace,
		ResourceUID:       string(wfe.UID),
		WorkflowID:        wfe.Spec.WorkflowRef.WorkflowID,
		TargetResource:    wfe.Spec.TargetResource,
	}
}

// GetCorrelationID returns the correlation ID for tracing
func (h *AuditHelper) GetCorrelationID(wfe *workflowexecutionv1.WorkflowExecution) string {
	return string(wfe.UID)
}

