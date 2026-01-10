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

// Package audit provides audit trail management for WorkflowExecution.
//
// This package implements BR-WE-005 (Audit Trail) by recording all workflow lifecycle
// events to the Data Storage service via the pkg/audit shared library.
//
// Audit Events:
// - workflow.started: PipelineRun initiated
// - workflow.completed: PipelineRun succeeded
// - workflow.failed: PipelineRun failed or timed out
//
// Per ADR-032: Audit is MANDATORY for WorkflowExecution (P0 service).
// Per DD-AUDIT-004: Uses type-safe WorkflowExecutionAuditPayload structures.
//
// Per Controller Refactoring Pattern Library (P3: Audit Manager):
// - Extracted from internal/controller/workflowexecution/audit.go
// - Testable audit logic in isolation
// - Consistent package structure with other controllers
//
// Reference: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
package audit

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit" // BR-AUDIT-005 Gap #7: Standardized error details
)

// ServiceName is the canonical service identifier for audit events.
const ServiceName = "workflowexecution-controller"

// Event category for WorkflowExecution audit events (ADR-034 v1.5: Service-level category)
// Per ADR-034 v1.5: ALL events from WorkflowExecution controller use "workflowexecution" category
const (
	CategoryWorkflowExecution = "workflowexecution" // Per ADR-034 v1.5 (2026-01-08)
)

// Event actions for WorkflowExecution audit events (per DD-AUDIT-003)
const (
	ActionStarted   = "started"
	ActionCompleted = "completed"
	ActionFailed    = "failed"
)

// Event types for WorkflowExecution audit events (per ADR-034 v1.5 + OpenAPI spec)
// Per ADR-034 v1.5: ALL event types from WorkflowExecution controller use "workflowexecution" prefix
// These match the event_type enum values in data-storage-v1.yaml
const (
	EventTypeStarted            = "workflowexecution.workflow.started"    // Per OpenAPI spec discriminator
	EventTypeCompleted          = "workflowexecution.workflow.completed"  // Per OpenAPI spec discriminator
	EventTypeFailed             = "workflowexecution.workflow.failed"     // Per OpenAPI spec discriminator
	EventTypeSelectionCompleted = "workflowexecution.selection.completed" // Gap #5 (BR-AUDIT-005) - Per ADR-034 v1.5
	EventTypeExecutionStarted   = "workflowexecution.execution.started"   // Gap #6 (BR-AUDIT-005) - Per ADR-034 v1.5
)

// Manager handles audit trail recording for WorkflowExecution lifecycle events.
//
// The Manager provides typed methods for each audit event type, ensuring
// consistent audit event structure across all workflow execution events.
//
// Usage:
//
//	auditMgr := audit.NewManager(auditStore, logger)
//	err := auditMgr.RecordWorkflowStarted(ctx, wfe)
type Manager struct {
	store  audit.AuditStore
	logger logr.Logger
}

// NewManager creates a new audit manager.
//
// Parameters:
// - store: AuditStore for writing audit events (from pkg/audit)
// - logger: Logger for audit operations
//
// The store may be nil to disable audit (graceful degradation), though
// per ADR-032 audit is MANDATORY for WorkflowExecution (P0 service).
func NewManager(store audit.AuditStore, logger logr.Logger) *Manager {
	return &Manager{
		store:  store,
		logger: logger,
	}
}

// RecordWorkflowStarted records a workflow.started audit event.
//
// This event is emitted when a PipelineRun is successfully created.
func (m *Manager) RecordWorkflowStarted(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	return m.recordAuditEvent(ctx, wfe, EventTypeStarted, "success")
}

// RecordWorkflowCompleted records a workflow.completed audit event.
//
// This event is emitted when a PipelineRun completes successfully.
func (m *Manager) RecordWorkflowCompleted(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	return m.recordAuditEvent(ctx, wfe, EventTypeCompleted, "success")
}

// RecordWorkflowFailed records a workflow.failed audit event.
//
// This event is emitted when a PipelineRun fails or times out.
// BR-AUDIT-005 Gap #7: Now includes standardized error_details for SOC2 compliance.
func (m *Manager) RecordWorkflowFailed(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	// Use custom audit event with error_details (Gap #7)
	return m.recordFailureAuditWithDetails(ctx, wfe)
}

// RecordWorkflowSelectionCompleted records a workflow.selection.completed audit event (Gap #5).
//
// This event is emitted immediately after workflow selection from spec.WorkflowRef,
// before PipelineRun creation. Per BR-AUDIT-005 Gap #5, this provides visibility
// into which workflow was selected for execution.
//
// Event Data Structure:
//   - selected_workflow_ref: {workflow_id, version, container_image}
func (m *Manager) RecordWorkflowSelectionCompleted(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	// Build audit event with custom event_data for Gap #5
	if m.store == nil {
		err := fmt.Errorf("AuditStore is nil - audit is MANDATORY per ADR-032")
		m.logger.Error(err, "CRITICAL: Cannot record audit event - manager misconfigured",
			"action", EventTypeSelectionCompleted,
			"wfe", wfe.Name,
		)
		return err
	}

	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeSelectionCompleted)
	audit.SetEventCategory(event, CategoryWorkflowExecution)
	audit.SetEventAction(event, "completed") // "workflowexecution.selection.completed" → "completed"
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", ServiceName)
	audit.SetResource(event, "WorkflowExecution", wfe.Name)

	// Correlation ID: Use parent RemediationRequest name (BR-AUDIT-005)
	// Per DD-AUDIT-CORRELATION: WFE.Spec.RemediationRequestRef.Name is the authoritative source
	// Labels are NOT set by RemediationOrchestrator (verified in creator implementation)
	correlationID := wfe.Spec.RemediationRequestRef.Name
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, wfe.Namespace)

	// Gap #5: Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	// Per OGEN-MIGRATION: Use ogen-generated type + union constructor
	// Handle empty phase (defaults to "Pending" per OpenAPI schema requirement)
	phase := wfe.Status.Phase
	if phase == "" {
		phase = "Pending" // Default phase when selection completes but WFE phase not yet set
	}
	payload := api.WorkflowExecutionAuditPayload{
		WorkflowID:      wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion: wfe.Spec.WorkflowRef.Version,
		ContainerImage:  wfe.Spec.WorkflowRef.ContainerImage,
		ExecutionName:   wfe.Name,
		Phase:           api.WorkflowExecutionAuditPayloadPhase(phase),
		TargetResource:  wfe.Spec.TargetResource, // Already a string per CRD definition
	}
	// Use proper Gap #5 constructor (added to OpenAPI spec discriminator)
	event.EventData = api.NewAuditEventRequestEventDataWorkflowexecutionSelectionCompletedAuditEventRequestEventData(payload)

	if err := m.store.StoreAudit(ctx, event); err != nil {
		m.logger.Error(err, "CRITICAL: Failed to store mandatory audit event",
			"action", EventTypeSelectionCompleted,
			"wfe", wfe.Name,
		)
		return fmt.Errorf("mandatory audit write failed per ADR-032: %w", err)
	}

	m.logger.V(1).Info("Audit event recorded",
		"action", EventTypeSelectionCompleted,
		"wfe", wfe.Name,
		"outcome", "success",
	)
	return nil
}

// RecordExecutionWorkflowStarted records an execution.workflow.started audit event (Gap #6).
//
// This event is emitted immediately after PipelineRun creation succeeds,
// providing the PipelineRun reference for complete Request-Response reconstruction.
// Per BR-AUDIT-005 Gap #6, this links WorkflowExecution to Tekton PipelineRun.
//
// Event Data Structure:
//   - execution_ref: {api_version: "tekton.dev/v1", kind: "PipelineRun", name, namespace}
//
// Parameters:
//   - pipelineRunName: Name of the created PipelineRun
//   - pipelineRunNamespace: Namespace of the created PipelineRun
func (m *Manager) RecordExecutionWorkflowStarted(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	pipelineRunName string,
	pipelineRunNamespace string,
) error {
	// Build audit event with custom event_data for Gap #6
	if m.store == nil {
		err := fmt.Errorf("AuditStore is nil - audit is MANDATORY per ADR-032")
		m.logger.Error(err, "CRITICAL: Cannot record audit event - manager misconfigured",
			"action", EventTypeExecutionStarted,
			"wfe", wfe.Name,
		)
		return err
	}

	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeExecutionStarted)
	audit.SetEventCategory(event, CategoryWorkflowExecution) // Per ADR-034 v1.5: workflowexecution category
	audit.SetEventAction(event, "started")                   // "workflowexecution.execution.started" → "started"
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", ServiceName)
	audit.SetResource(event, "WorkflowExecution", wfe.Name)

	// Correlation ID: Use parent RemediationRequest name (BR-AUDIT-005)
	// Per DD-AUDIT-CORRELATION: WFE.Spec.RemediationRequestRef.Name is the authoritative source
	// Labels are NOT set by RemediationOrchestrator (verified in creator implementation)
	correlationID := wfe.Spec.RemediationRequestRef.Name
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, wfe.Namespace)

	// Gap #6: Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	// Per OGEN-MIGRATION: Use ogen-generated type + union constructor
	// Handle empty phase (defaults to "Pending" per OpenAPI schema requirement)
	phase := wfe.Status.Phase
	if phase == "" {
		phase = "Pending" // Default phase when execution starts but WFE phase not yet set
	}
	payload := api.WorkflowExecutionAuditPayload{
		WorkflowID:      wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion: wfe.Spec.WorkflowRef.Version,
		ContainerImage:  wfe.Spec.WorkflowRef.ContainerImage,
		ExecutionName:   wfe.Name,
		Phase:           api.WorkflowExecutionAuditPayloadPhase(phase),
		TargetResource:  wfe.Spec.TargetResource, // Already a string per CRD definition
	}
	payload.PipelinerunName.SetTo(pipelineRunName)
	// Use proper Gap #6 constructor (added to OpenAPI spec discriminator)
	event.EventData = api.NewAuditEventRequestEventDataWorkflowexecutionExecutionStartedAuditEventRequestEventData(payload)

	if err := m.store.StoreAudit(ctx, event); err != nil {
		m.logger.Error(err, "CRITICAL: Failed to store mandatory audit event",
			"action", EventTypeExecutionStarted,
			"wfe", wfe.Name,
		)
		return fmt.Errorf("mandatory audit write failed per ADR-032: %w", err)
	}

	m.logger.V(1).Info("Audit event recorded",
		"action", EventTypeExecutionStarted,
		"wfe", wfe.Name,
		"pipelineRun", pipelineRunName,
		"outcome", "success",
	)
	return nil
}

// recordAuditEvent is the internal implementation for recording audit events.
//
// This method builds the audit event structure and writes it to the Data Storage Service.
// It handles all the common audit event fields and WFE-specific payload construction.
func (m *Manager) recordAuditEvent(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	action string,
	outcome string,
) error {
	// Audit is MANDATORY per ADR-032: No graceful degradation allowed
	// ADR-032 Audit Mandate: "No Audit Loss - audit writes are MANDATORY, not best-effort"
	if m.store == nil {
		err := fmt.Errorf("AuditStore is nil - audit is MANDATORY per ADR-032")
		m.logger.Error(err, "CRITICAL: Cannot record audit event - manager misconfigured",
			"action", action,
			"wfe", wfe.Name,
		)
		// Return error to block business operation
		// ADR-032: "No Audit Loss" - audit write failures must be detected
		return err
	}

	// Build audit event per ADR-034 schema (DD-AUDIT-002 V2.0: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	// Event type = action (e.g., "workflowexecution.workflow.started")
	// Service context is provided by event_category and actor fields
	audit.SetEventType(event, action)
	audit.SetEventCategory(event, CategoryWorkflowExecution)
	// Event action = just the action part (e.g., "started" from "workflow.started")
	// Split on "." and take the last part
	parts := strings.Split(action, ".")
	eventAction := parts[len(parts)-1] // Get last part after final dot
	audit.SetEventAction(event, eventAction)

	// Map outcome string to OpenAPI enum
	switch outcome {
	case "success":
		audit.SetEventOutcome(event, audit.OutcomeSuccess)
	case "failure":
		audit.SetEventOutcome(event, audit.OutcomeFailure)
	case "pending":
		audit.SetEventOutcome(event, audit.OutcomePending)
	default:
		audit.SetEventOutcome(event, audit.OutcomeSuccess) // default to success
	}

	audit.SetActor(event, "service", ServiceName)
	audit.SetResource(event, "WorkflowExecution", wfe.Name)

	// Correlation ID: Use parent RemediationRequest name (BR-AUDIT-005)
	// Per DD-AUDIT-CORRELATION: WFE.Spec.RemediationRequestRef.Name is the authoritative source
	// Labels are NOT set by RemediationOrchestrator (verified in creator implementation)
	correlationID := wfe.Spec.RemediationRequestRef.Name
	audit.SetCorrelationID(event, correlationID)

	// Set namespace context
	audit.SetNamespace(event, wfe.Namespace)

	// Build structured event data (type-safe per DD-AUDIT-004)
	// Eliminates map[string]interface{} per 02-go-coding-standards.mdc
	// Per OGEN-MIGRATION: Use ogen-generated type + union constructor
	payload := api.WorkflowExecutionAuditPayload{
		WorkflowID:      wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion: wfe.Spec.WorkflowRef.Version,
		TargetResource:  wfe.Spec.TargetResource,
		Phase:           api.WorkflowExecutionAuditPayloadPhase(wfe.Status.Phase),
		ContainerImage:  wfe.Spec.WorkflowRef.ContainerImage,
		ExecutionName:   wfe.Name,
	}

	// Add timing info if available
	if wfe.Status.StartTime != nil {
		payload.StartedAt.SetTo(wfe.Status.StartTime.Time)
	}
	if wfe.Status.CompletionTime != nil {
		payload.CompletedAt.SetTo(wfe.Status.CompletionTime.Time)
	}
	if wfe.Status.Duration != "" {
		payload.Duration.SetTo(wfe.Status.Duration)
	}

	// Add failure details if present
	if wfe.Status.FailureDetails != nil {
		payload.FailureReason.SetTo(api.WorkflowExecutionAuditPayloadFailureReason(wfe.Status.FailureDetails.Reason))
		payload.FailureMessage.SetTo(wfe.Status.FailureDetails.Message)
		if wfe.Status.FailureDetails.FailedTaskName != "" {
			payload.FailedTaskName.SetTo(wfe.Status.FailureDetails.FailedTaskName)
		}
	}

	// Add PipelineRun reference if present
	if wfe.Status.PipelineRunRef != nil {
		payload.PipelinerunName.SetTo(wfe.Status.PipelineRunRef.Name)
	}

	// Set event data using ogen union constructor based on action
	// Per OGEN-MIGRATION: Direct assignment with union constructor for type safety
	switch action {
	case EventTypeStarted:
		event.EventData = api.NewAuditEventRequestEventDataWorkflowexecutionWorkflowStartedAuditEventRequestEventData(payload)
	case EventTypeCompleted:
		event.EventData = api.NewAuditEventRequestEventDataWorkflowexecutionWorkflowCompletedAuditEventRequestEventData(payload)
	case EventTypeFailed:
		event.EventData = api.NewAuditEventRequestEventDataWorkflowexecutionWorkflowFailedAuditEventRequestEventData(payload)
	default:
		// Fallback for any other event types (e.g., workflow.selection.completed)
		event.EventData = api.NewAuditEventRequestEventDataWorkflowexecutionWorkflowCompletedAuditEventRequestEventData(payload)
	}

	// Store audit event - MANDATORY per ADR-032
	// ADR-032: "Write Verification - audit write failures must be detected and handled"
	if err := m.store.StoreAudit(ctx, event); err != nil {
		m.logger.Error(err, "CRITICAL: Failed to store mandatory audit event",
			"action", action,
			"wfe", wfe.Name,
		)
		// Return error per ADR-032 "No Audit Loss" - audit writes are MANDATORY
		return fmt.Errorf("mandatory audit write failed per ADR-032: %w", err)
	}

	m.logger.V(1).Info("Audit event recorded",
		"action", action,
		"wfe", wfe.Name,
		"outcome", outcome,
	)
	return nil
}

// recordFailureAuditWithDetails records a workflow.failed audit event with standardized error_details.
//
// This method implements BR-AUDIT-005 Gap #7: Standardized error details
// for SOC2 compliance and RR reconstruction.
//
// Error details are extracted from wfe.Status.FailureDetails which contains
// Tekton pipeline failure information (failed task, failed step, error message).
func (m *Manager) recordFailureAuditWithDetails(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	if m.store == nil {
		err := fmt.Errorf("AuditStore is nil - audit is MANDATORY per ADR-032")
		m.logger.Error(err, "CRITICAL: Cannot record audit event - manager misconfigured",
			"action", EventTypeFailed,
			"wfe", wfe.Name,
		)
		return err
	}

	// Build error_details from FailureDetails (Gap #7)
	var errorDetails *sharedaudit.ErrorDetails
	if wfe.Status.FailureDetails != nil {
		// Construct error message from Tekton failure details
		errorMessage := fmt.Sprintf("Pipeline failed at task '%s'", wfe.Status.FailureDetails.FailedTaskName)
		if wfe.Status.FailureDetails.FailedStepName != "" {
			errorMessage += fmt.Sprintf(" step '%s'", wfe.Status.FailureDetails.FailedStepName)
		}
		if wfe.Status.FailureDetails.Message != "" {
			errorMessage += ": " + wfe.Status.FailureDetails.Message
		}

		// Determine error code based on failure type
		errorCode := "ERR_PIPELINE_FAILED"
		retryPossible := true // Pipeline failures may be transient

		// Check if it's a permanent error (e.g., invalid workflow)
		if strings.Contains(errorMessage, "not found") || strings.Contains(errorMessage, "invalid") {
			errorCode = "ERR_WORKFLOW_NOT_FOUND"
			retryPossible = false
		}

		errorDetails = sharedaudit.NewErrorDetails(
			"workflowexecution",
			errorCode,
			errorMessage,
			retryPossible,
		)
	} else {
		// No FailureDetails (shouldn't happen, but handle gracefully)
		errorDetails = sharedaudit.NewErrorDetails(
			"workflowexecution",
			"ERR_PIPELINE_FAILED",
			"Workflow execution failed with unknown error",
			true,
		)
	}

	// Build audit event
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeFailed)
	audit.SetEventCategory(event, CategoryWorkflowExecution)
	audit.SetEventAction(event, ActionFailed)
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, "service", ServiceName)
	audit.SetResource(event, "WorkflowExecution", wfe.Name)

	// Correlation ID from RemediationRequestRef
	correlationID := wfe.Spec.RemediationRequestRef.Name
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, wfe.Namespace)

	// Build structured event data (type-safe per DD-AUDIT-004)
	// Eliminates map[string]interface{} per 02-go-coding-standards.mdc
	// Per OGEN-MIGRATION: Use ogen-generated type + union constructor
	payload := api.WorkflowExecutionAuditPayload{
		WorkflowID:      wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion: wfe.Spec.WorkflowRef.Version,
		TargetResource:  wfe.Spec.TargetResource,
		Phase:           api.WorkflowExecutionAuditPayloadPhase(wfe.Status.Phase),
		ContainerImage:  wfe.Spec.WorkflowRef.ContainerImage,
		ExecutionName:   wfe.Name,
	}

	// BR-AUDIT-005 Gap #7: Standardized error_details for SOC2 compliance
	if errorDetails != nil {
		payload.ErrorDetails.SetTo(api.ErrorDetails{
			Message:       errorDetails.Message,
			Code:          errorDetails.Code,
			Component:     api.ErrorDetailsComponent(errorDetails.Component),
			RetryPossible: errorDetails.RetryPossible,
			StackTrace:    errorDetails.StackTrace,
		})
	}

	// Add timing info if available
	if wfe.Status.StartTime != nil {
		payload.StartedAt.SetTo(wfe.Status.StartTime.Time)
	}
	if wfe.Status.CompletionTime != nil {
		payload.CompletedAt.SetTo(wfe.Status.CompletionTime.Time)
	}
	if wfe.Status.Duration != "" {
		payload.Duration.SetTo(wfe.Status.Duration)
	}

	// Add failure details if present
	if wfe.Status.FailureDetails != nil {
		payload.FailureReason.SetTo(api.WorkflowExecutionAuditPayloadFailureReason(wfe.Status.FailureDetails.Reason))
		payload.FailureMessage.SetTo(wfe.Status.FailureDetails.Message)
		if wfe.Status.FailureDetails.FailedTaskName != "" {
			payload.FailedTaskName.SetTo(wfe.Status.FailureDetails.FailedTaskName)
		}
	}

	// Add PipelineRun reference if present
	if wfe.Status.PipelineRunRef != nil {
		payload.PipelinerunName.SetTo(wfe.Status.PipelineRunRef.Name)
	}

	// Set event data using ogen union constructor - always use "failed" for recordFailureAuditWithDetails
	// Per OGEN-MIGRATION: Direct assignment with union constructor for type safety
	event.EventData = api.NewAuditEventRequestEventDataWorkflowexecutionWorkflowFailedAuditEventRequestEventData(payload)

	// Store audit event
	if err := m.store.StoreAudit(ctx, event); err != nil {
		m.logger.Error(err, "CRITICAL: Failed to store mandatory audit event",
			"action", EventTypeFailed,
			"wfe", wfe.Name,
		)
		return fmt.Errorf("mandatory audit write failed per ADR-032: %w", err)
	}

	m.logger.V(1).Info("Failure audit event recorded with error_details",
		"wfe", wfe.Name,
		"error_code", errorDetails.Code,
	)
	return nil
}

// toWorkflowExecutionAuditPayloadEventType converts event type string to ogen enum
// Uses constants defined above that match OpenAPI spec event_type enum values
func toWorkflowExecutionAuditPayloadEventType(eventType string) api.WorkflowExecutionAuditPayloadEventType {
	switch eventType {
	case EventTypeStarted:
		return api.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowStarted
	case EventTypeCompleted:
		return api.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowCompleted
	case EventTypeFailed:
		return api.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowFailed
	case EventTypeSelectionCompleted:
		return api.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionSelectionCompleted
	case EventTypeExecutionStarted:
		return api.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionExecutionStarted
	default:
		return api.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowStarted // default fallback
	}
}
