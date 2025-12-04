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

package workflowexecution

import (
	"context"
	"encoding/json"

	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

// ============================================================================
// Audit Trail Implementation (BR-WE-007, ADR-034)
// ============================================================================

// Event type constants following pattern: <service>.<category>.<action>
const (
	// Lifecycle events
	eventTypeCompleted = "workflowexecution.lifecycle.completed"
	eventTypeFailed    = "workflowexecution.lifecycle.failed"
	eventTypeSkipped   = "workflowexecution.lifecycle.skipped"
	eventTypeDeleted   = "workflowexecution.lifecycle.deleted"

	// PipelineRun events
	eventTypePipelineRunCreated = "workflowexecution.pipelinerun.created"

	// Lock events
	eventTypeLockReleased = "workflowexecution.lock.released"
)

// Event category constants
const (
	categoryLifecycle   = "lifecycle"
	categoryPipelineRun = "pipelinerun"
	categoryLock        = "lock"
)

// Event action constants
const (
	actionCreated   = "created"
	actionCompleted = "completed"
	actionFailed    = "failed"
	actionSkipped   = "skipped"
	actionReleased  = "released"
	actionDeleted   = "deleted"
)

// WFEAuditData contains workflow-execution-specific data for audit events
type WFEAuditData struct {
	WorkflowID       string                 `json:"workflow_id"`
	WorkflowVersion  string                 `json:"workflow_version"`
	ContainerImage   string                 `json:"container_image,omitempty"`
	TargetResource   string                 `json:"target_resource"`
	Confidence       float64                `json:"confidence,omitempty"`
	Rationale        string                 `json:"rationale,omitempty"`
	PipelineRunName  string                 `json:"pipelinerun_name,omitempty"`
	Duration         string                 `json:"duration,omitempty"`
	FromPhase        string                 `json:"from_phase,omitempty"`
	ToPhase          string                 `json:"to_phase,omitempty"`
	FailureReason    string                 `json:"failure_reason,omitempty"`
	SkipReason       string                 `json:"skip_reason,omitempty"`
	AdditionalFields map[string]interface{} `json:"additional_fields,omitempty"`
}

// buildAuditEvent creates an AuditEvent from WorkflowExecution
func (r *WorkflowExecutionReconciler) buildAuditEvent(wfe *workflowexecutionv1alpha1.WorkflowExecution, eventType, category, action, outcome string, data *WFEAuditData) *audit.AuditEvent {
	event := audit.NewAuditEvent()

	// Required fields
	event.EventType = eventType
	event.EventCategory = category
	event.EventAction = action
	event.EventOutcome = outcome
	event.ActorType = "service"
	event.ActorID = "workflowexecution-controller"
	event.ResourceType = "WorkflowExecution"
	event.ResourceID = wfe.Name
	event.CorrelationID = getCorrelationID(wfe)

	// Optional fields
	resourceName := wfe.Name
	event.ResourceName = &resourceName

	namespace := wfe.Namespace
	event.Namespace = &namespace

	// Set duration if available
	if wfe.Status.StartTime != nil && wfe.Status.CompletionTime != nil {
		duration := int(wfe.Status.CompletionTime.Sub(wfe.Status.StartTime.Time).Milliseconds())
		event.DurationMs = &duration
	}

	// Set event data as JSON
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err == nil {
			event.EventData = jsonData
		}
	}

	return event
}

// recordAuditEvent writes an audit event to the audit store
// Uses fire-and-forget pattern per ADR-034 - audit failures don't block reconciliation
func (r *WorkflowExecutionReconciler) recordAuditEvent(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, eventType, category, action, outcome string, data *WFEAuditData) {
	log := log.FromContext(ctx)

	if r.AuditStore == nil {
		log.V(1).Info("Audit store not configured, skipping audit event", "eventType", eventType)
		return
	}

	event := r.buildAuditEvent(wfe, eventType, category, action, outcome, data)

	// Fire-and-forget audit write (per ADR-034)
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		// Log but don't fail reconciliation
		log.Error(err, "Failed to write audit event (non-blocking)",
			"eventType", eventType,
			"resourceName", wfe.Name)
	}
}

// recordPipelineRunCreated records PipelineRun creation
func (r *WorkflowExecutionReconciler) recordPipelineRunCreated(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, prName string) {
	r.recordAuditEvent(ctx, wfe, eventTypePipelineRunCreated, categoryPipelineRun, actionCreated, "success", &WFEAuditData{
		WorkflowID:      wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion: wfe.Spec.WorkflowRef.Version,
		TargetResource:  wfe.Spec.TargetResource,
		PipelineRunName: prName,
		AdditionalFields: map[string]interface{}{
			"execution_namespace": r.ExecutionNamespace,
			"service_account":     r.ServiceAccountName,
		},
	})
}

// recordCompleted records successful completion
func (r *WorkflowExecutionReconciler) recordCompleted(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) {
	r.recordAuditEvent(ctx, wfe, eventTypeCompleted, categoryLifecycle, actionCompleted, "success", &WFEAuditData{
		WorkflowID:      wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion: wfe.Spec.WorkflowRef.Version,
		TargetResource:  wfe.Spec.TargetResource,
		Duration:        wfe.Status.Duration,
		PipelineRunName: getPipelineRunName(wfe),
	})
}

// recordFailed records execution failure
func (r *WorkflowExecutionReconciler) recordFailed(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) {
	additionalFields := map[string]interface{}{}

	if wfe.Status.FailureDetails != nil {
		additionalFields["failed_task_name"] = wfe.Status.FailureDetails.FailedTaskName
		additionalFields["failed_task_index"] = wfe.Status.FailureDetails.FailedTaskIndex
		additionalFields["exit_code"] = wfe.Status.FailureDetails.ExitCode
		additionalFields["natural_language_summary"] = wfe.Status.FailureDetails.NaturalLanguageSummary
		additionalFields["was_execution_failure"] = wfe.Status.FailureDetails.WasExecutionFailure
	}

	r.recordAuditEvent(ctx, wfe, eventTypeFailed, categoryLifecycle, actionFailed, "failure", &WFEAuditData{
		WorkflowID:       wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion:  wfe.Spec.WorkflowRef.Version,
		TargetResource:   wfe.Spec.TargetResource,
		Duration:         wfe.Status.Duration,
		FailureReason:    wfe.Status.FailureReason,
		AdditionalFields: additionalFields,
	})
}

// recordSkipped records skipped execution
func (r *WorkflowExecutionReconciler) recordSkipped(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) {
	additionalFields := map[string]interface{}{}
	skipReason := ""

	if wfe.Status.SkipDetails != nil {
		skipReason = wfe.Status.SkipDetails.Reason
		additionalFields["skip_message"] = wfe.Status.SkipDetails.Message

		if wfe.Status.SkipDetails.ConflictingWorkflow != nil {
			additionalFields["conflicting_workflow_name"] = wfe.Status.SkipDetails.ConflictingWorkflow.Name
			additionalFields["conflicting_workflow_id"] = wfe.Status.SkipDetails.ConflictingWorkflow.WorkflowID
		}

		if wfe.Status.SkipDetails.RecentRemediation != nil {
			additionalFields["recent_workflow_name"] = wfe.Status.SkipDetails.RecentRemediation.Name
			additionalFields["recent_workflow_outcome"] = wfe.Status.SkipDetails.RecentRemediation.Outcome
			additionalFields["cooldown_remaining"] = wfe.Status.SkipDetails.RecentRemediation.CooldownRemaining
		}
	}

	r.recordAuditEvent(ctx, wfe, eventTypeSkipped, categoryLifecycle, actionSkipped, "skipped", &WFEAuditData{
		WorkflowID:       wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion:  wfe.Spec.WorkflowRef.Version,
		TargetResource:   wfe.Spec.TargetResource,
		SkipReason:       skipReason,
		AdditionalFields: additionalFields,
	})
}

// recordLockReleased records resource lock release after cooldown
func (r *WorkflowExecutionReconciler) recordLockReleased(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) {
	r.recordAuditEvent(ctx, wfe, eventTypeLockReleased, categoryLock, actionReleased, "success", &WFEAuditData{
		WorkflowID:      wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion: wfe.Spec.WorkflowRef.Version,
		TargetResource:  wfe.Spec.TargetResource,
		AdditionalFields: map[string]interface{}{
			"cooldown_period": r.CooldownPeriod.String(),
		},
	})
}

// recordDeleted records WorkflowExecution deletion
func (r *WorkflowExecutionReconciler) recordDeleted(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) {
	r.recordAuditEvent(ctx, wfe, eventTypeDeleted, categoryLifecycle, actionDeleted, "success", &WFEAuditData{
		WorkflowID:      wfe.Spec.WorkflowRef.WorkflowID,
		WorkflowVersion: wfe.Spec.WorkflowRef.Version,
		TargetResource:  wfe.Spec.TargetResource,
		AdditionalFields: map[string]interface{}{
			"final_phase": wfe.Status.Phase,
		},
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

// getCorrelationID extracts or generates a correlation ID for tracing
func getCorrelationID(wfe *workflowexecutionv1alpha1.WorkflowExecution) string {
	// Try to get from annotations first
	if wfe.Annotations != nil {
		if id, ok := wfe.Annotations["kubernaut.ai/correlation-id"]; ok {
			return id
		}
	}
	// Fall back to remediation request reference
	return wfe.Spec.RemediationRequestRef.Name
}

// getPipelineRunName returns the PipelineRun name if available
func getPipelineRunName(wfe *workflowexecutionv1alpha1.WorkflowExecution) string {
	if wfe.Status.PipelineRunRef != nil {
		return wfe.Status.PipelineRunRef.Name
	}
	return ""
}
