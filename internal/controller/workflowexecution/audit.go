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

// Package workflowexecution provides audit trail functionality for workflow execution.
//
// This file implements BR-WE-005 (Audit Trail) by recording all workflow lifecycle
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
// See: docs/architecture/decisions/ADR-032-data-access-layer-isolation.md
package workflowexecution

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
)

// ========================================
// Day 8: Audit Trail (BR-WE-005)
// Per ADR-032: All services use Data Storage Service via pkg/audit
// ========================================

// recordAuditEventWithCondition is a helper that records an audit event
// and updates the AuditRecorded condition accordingly
// This reduces duplication of the audit + condition setting pattern
func (r *WorkflowExecutionReconciler) recordAuditEventWithCondition( //nolint:unused
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	eventType, category string,
) {
	logger := log.FromContext(ctx)

	if err := r.RecordAuditEvent(ctx, wfe, eventType, category); err != nil {
		logger.V(1).Info("Failed to record audit event",
			"eventType", eventType,
			"error", err)
		weconditions.SetAuditRecorded(wfe, false,
			weconditions.ReasonAuditFailed,
			fmt.Sprintf("Failed to record audit event: %v", err))
	} else {
		weconditions.SetAuditRecorded(wfe, true,
			weconditions.ReasonAuditSucceeded,
			fmt.Sprintf("Audit event %s recorded to DataStorage", eventType))
	}
}

// RecordAuditEvent writes an audit event to the Data Storage Service
// Uses pkg/audit BufferedAuditStore for non-blocking, batched writes
// Gracefully handles nil AuditStore (audit disabled)
func (r *WorkflowExecutionReconciler) RecordAuditEvent(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	action string,
	outcome string,
) error {
	logger := log.FromContext(ctx)

	// Audit is MANDATORY per ADR-032: No graceful degradation allowed
	// ADR-032 Audit Mandate: "No Audit Loss - audit writes are MANDATORY, not best-effort"
	if r.AuditStore == nil {
		err := fmt.Errorf("AuditStore is nil - audit is MANDATORY per ADR-032")
		logger.Error(err, "CRITICAL: Cannot record audit event - controller misconfigured",
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
	audit.SetEventCategory(event, "workflowexecution") // Per ADR-034 v1.5 (2026-01-08)
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

	audit.SetActor(event, "service", "workflowexecution-controller")
	audit.SetResource(event, "WorkflowExecution", wfe.Name)

	// Correlation ID from labels (set by RemediationOrchestrator)
	correlationID := wfe.Name // Fallback: use WFE name as correlation ID
	if wfe.Labels != nil {
		if corrID, ok := wfe.Labels["kubernaut.ai/correlation-id"]; ok {
			correlationID = corrID
		}
	}
	audit.SetCorrelationID(event, correlationID)

	// Set namespace context
	audit.SetNamespace(event, wfe.Namespace)

	// Build structured event data (type-safe per DD-AUDIT-004)
	// Eliminates map[string]interface{} per 02-go-coding-standards.mdc
	// Per OGEN-MIGRATION: Use ogen-generated type + union constructor
	payload := ogenclient.WorkflowExecutionAuditPayload{
		WorkflowID:      wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion: wfe.Spec.WorkflowRef.Version,
		TargetResource:  wfe.Spec.TargetResource,
		Phase:           ogenclient.WorkflowExecutionAuditPayloadPhase(wfe.Status.Phase),
		ContainerImage:  wfe.Spec.WorkflowRef.ContainerImage,
		ExecutionName:   wfe.Name,
	}

	// Add timing info if available (use SetTo for optional fields)
	if wfe.Status.StartTime != nil {
		payload.StartedAt.SetTo(wfe.Status.StartTime.Time)
	}
	if wfe.Status.CompletionTime != nil {
		payload.CompletedAt.SetTo(wfe.Status.CompletionTime.Time)
	}
	if wfe.Status.Duration != "" {
		payload.Duration.SetTo(wfe.Status.Duration)
	}

	// Add failure details if present (use SetTo for optional fields)
	if wfe.Status.FailureDetails != nil {
		payload.FailureReason.SetTo(ogenclient.WorkflowExecutionAuditPayloadFailureReason(wfe.Status.FailureDetails.Reason))
		payload.FailureMessage.SetTo(wfe.Status.FailureDetails.Message)
		if wfe.Status.FailureDetails.FailedTaskName != "" {
			payload.FailedTaskName.SetTo(wfe.Status.FailureDetails.FailedTaskName)
		}
	}

	// Add PipelineRun reference if present (use SetTo for optional field)
	if wfe.Status.PipelineRunRef != nil {
		payload.PipelinerunName.SetTo(wfe.Status.PipelineRunRef.Name)
	}

	// Set event data using ogen union constructor based on action
	// Per OGEN-MIGRATION: Direct assignment with union constructor for type safety
	switch action {
	case "workflow.started":
		event.EventData = ogenclient.NewAuditEventRequestEventDataWorkflowexecutionWorkflowStartedAuditEventRequestEventData(payload)
	case "workflow.completed":
		event.EventData = ogenclient.NewAuditEventRequestEventDataWorkflowexecutionWorkflowCompletedAuditEventRequestEventData(payload)
	case "workflow.failed":
		event.EventData = ogenclient.NewAuditEventRequestEventDataWorkflowexecutionWorkflowFailedAuditEventRequestEventData(payload)
	default:
		// Fallback for any other event types
		event.EventData = ogenclient.NewAuditEventRequestEventDataWorkflowexecutionWorkflowCompletedAuditEventRequestEventData(payload)
	}

	// Store audit event - MANDATORY per ADR-032
	// ADR-032: "Write Verification - audit write failures must be detected and handled"
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "CRITICAL: Failed to store mandatory audit event",
			"action", action,
			"wfe", wfe.Name,
		)
		// Return error per ADR-032 "No Audit Loss" - audit writes are MANDATORY
		return fmt.Errorf("mandatory audit write failed per ADR-032: %w", err)
	}

	logger.V(1).Info("Audit event recorded",
		"action", action,
		"wfe", wfe.Name,
		"outcome", outcome,
	)
	return nil
}
