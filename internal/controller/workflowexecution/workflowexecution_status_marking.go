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

// Status-marking helpers (MarkCompleted/MarkFailed and friends), split out of
// workflowexecution_controller.go per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520) to keep the file under the 700-line convention threshold.
// Pure structural move — no behavior change.
package workflowexecution

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
)

// ========================================
// Day 5: Status Synchronization Methods
// ========================================

// BuildPipelineRunStatusSummary creates a lightweight status summary from PipelineRun
// Provides visibility into task progress during execution (v3.2)
func (r *WorkflowExecutionReconciler) BuildPipelineRunStatusSummary(ctx context.Context, pr *tektonv1.PipelineRun) *workflowexecutionv1alpha1.ExecutionStatusSummary {
	summary := &workflowexecutionv1alpha1.ExecutionStatusSummary{
		Status: corev1.ConditionUnknown,
	}

	// Extract task counts from ChildReferences
	summary.TotalTasks = len(pr.Status.ChildReferences)

	// Count TaskRuns with ConditionSucceeded True (completed tasks)
	for _, ref := range pr.Status.ChildReferences {
		if ref.Kind != "TaskRun" {
			continue
		}
		var tr tektonv1.TaskRun
		if err := r.Get(ctx, client.ObjectKey{
			Name:      ref.Name,
			Namespace: pr.Namespace,
		}, &tr); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			continue
		}
		cond := tr.Status.GetCondition(apis.ConditionSucceeded)
		if cond != nil && cond.IsTrue() {
			summary.CompletedTasks++
		}
	}

	// Get Succeeded condition
	succeededCond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if succeededCond != nil {
		summary.Status = succeededCond.Status
		summary.Reason = succeededCond.Reason
		summary.Message = succeededCond.Message
	}

	return summary
}

// MarkCompleted transitions WFE to Completed phase
// Calculates Duration from StartTime to CompletionTime (v3.2)
// Day 6 Extension (BR-WE-012): Resets ConsecutiveFailures counter
// Records metrics per BR-WE-008 (Day 7)
func (r *WorkflowExecutionReconciler) MarkCompleted(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, pr *tektonv1.PipelineRun, summary ...*workflowexecutionv1alpha1.ExecutionStatusSummary) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Marking WorkflowExecution as Completed")

	// ========================================
	// DD-RO-002 Phase 3: Counter Reset Removed (Dec 19, 2025)
	// RO resets RR.Status.ConsecutiveFailureCount on successful remediation
	// WE no longer tracks routing state
	// ========================================

	// Calculate duration for use in atomic update
	completionTime, durationVal, durationSeconds := completionTimeAndDuration(wfe, pr)

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Consolidates phase transition + conditions into single API call
	// BEFORE: 2 API calls (phase update + conditions update)
	// AFTER: 1 atomic API call (50% reduction)
	// ========================================
	if err := r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() error {
		return r.applyCompletedTransition(wfe, completionTime, durationVal, summary)
	}); err != nil {
		logger.Error(err, "Failed to atomically update status to Completed")
		return ctrl.Result{}, err
	}

	// Issue #1597: audit recorded exactly once, AFTER the phase-transition
	// AtomicStatusUpdate above has durably committed. Previously this call
	// lived inside that closure and re-fired on every RetryOnConflict retry,
	// producing duplicate workflow.completed audit events for one logical
	// completion. See DD-WE-009.
	r.recordTerminalAudit(ctx, wfe, "workflow.completed", r.AuditManager.RecordWorkflowCompleted, logger)

	// Day 7: Record metrics (BR-WE-008)
	// DD-METRICS-001: Use injected metrics instead of global function
	if r.Metrics != nil {
		r.Metrics.RecordWorkflowCompletion(durationSeconds)
	}

	// V1.0: Consecutive failures gauge removed - RO handles routing (DD-RO-002)

	// Emit event
	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonWorkflowCompleted,
		fmt.Sprintf("Workflow %s completed successfully in %s", wfe.Spec.WorkflowRef.WorkflowID, wfe.Status.Duration))

	// DD-EVENT-001 v1.1: PhaseTransition breadcrumb for Running → Completed
	r.emitPhaseTransition(wfe, "Running", "Completed")

	logger.Info("WorkflowExecution completed (atomic status update)")

	// Issue #375: Schedule requeue so ReconcileTerminal runs after cooldown to
	// delete the execution resource (Job/PipelineRun) and release the target lock.
	// Without this, GenerationChangedPredicate blocks the status-only update event,
	// and ReconcileTerminal is never called.
	cooldown := r.CooldownPeriod
	if cooldown == 0 {
		cooldown = DefaultCooldownPeriod
	}
	return ctrl.Result{RequeueAfter: cooldown}, nil
}

// completionTimeAndDuration resolves the completion timestamp (preferring
// the PipelineRun's own CompletionTime when available, per v3.2) and the
// resulting execution duration relative to wfe.Status.StartTime. Extracted
// from MarkCompleted (Wave 6 6e-ii GREEN: funlen remediation) — pure code
// motion, no behavior change.
func completionTimeAndDuration(wfe *workflowexecutionv1alpha1.WorkflowExecution, pr *tektonv1.PipelineRun) (*metav1.Time, *metav1.Duration, float64) {
	now := metav1.Now()
	completionTime := &now
	if pr != nil && pr.Status.CompletionTime != nil {
		completionTime = pr.Status.CompletionTime
	}

	var durationVal *metav1.Duration
	var durationSeconds float64
	if wfe.Status.StartTime != nil {
		d := completionTime.Sub(wfe.Status.StartTime.Time).Round(time.Second)
		durationVal = &metav1.Duration{Duration: d}
		durationSeconds = d.Seconds()
	}
	return completionTime, durationVal, durationSeconds
}

// applyCompletedTransition is the AtomicStatusUpdate callback body for
// MarkCompleted: transitions the phase to Completed (P0: Phase State
// Machine), persists completion time/duration/ExecutionStatus, and sets the
// TektonPipelineComplete/Ready conditions (BR-WE-006, Issue #79 Phase 7b).
// Extracted from MarkCompleted (Wave 6 6e-ii GREEN: funlen remediation) —
// pure code motion, no behavior change. Issue #1597: the workflow.completed
// audit event is recorded by the caller (MarkCompleted) via
// recordTerminalAudit AFTER this closure commits, not inside it — see
// DD-WE-009 for why bundling audit into the retryable closure caused
// duplicate audit writes on conflict retries.
func (r *WorkflowExecutionReconciler) applyCompletedTransition(wfe *workflowexecutionv1alpha1.WorkflowExecution, completionTime *metav1.Time, durationVal *metav1.Duration, summary []*workflowexecutionv1alpha1.ExecutionStatusSummary) error {
	if err := r.PhaseManager.TransitionTo(wfe, wephase.Completed); err != nil {
		return fmt.Errorf("failed to transition to Completed: %w", err)
	}

	wfe.Status.CompletionTime = completionTime
	wfe.Status.Duration = durationVal

	// Issue #118 Gap 4: persist ExecutionStatus inside callback (survives refetch)
	if len(summary) > 0 && summary[0] != nil {
		wfe.Status.ExecutionStatus = summary[0]
	}

	// BR-WE-006: Set TektonPipelineComplete condition
	weconditions.SetExecutionComplete(wfe, true,
		weconditions.ReasonExecutionSucceeded,
		fmt.Sprintf("All tasks completed successfully in %s", wfe.Status.Duration))

	// Issue #79 Phase 7b: Set Ready condition on terminal transitions
	weconditions.SetReady(wfe, true, weconditions.ReasonReady, "Workflow execution completed")

	return nil
}

// MarkFailed transitions WFE to Failed phase with FailureDetails
// Extracts failure information from PipelineRun (v3.2)
// Day 6 Extension (BR-WE-012): Handles exponential backoff for pre-execution failures
// Records metrics per BR-WE-008 (Day 7)
func (r *WorkflowExecutionReconciler) MarkFailed(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, pr *tektonv1.PipelineRun, summary ...*workflowexecutionv1alpha1.ExecutionStatusSummary) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Marking WorkflowExecution as Failed")

	now := metav1.Now()
	var durationVal *metav1.Duration
	var durationSeconds float64
	if wfe.Status.StartTime != nil {
		d := now.Sub(wfe.Status.StartTime.Time).Round(time.Second)
		durationVal = &metav1.Duration{Duration: d}
		durationSeconds = d.Seconds()
	}

	failureDetails, failureReason, failureMessage := r.resolveMarkFailedDetails(ctx, wfe, pr, summary)

	// ========================================
	// DD-RO-002 Phase 3: Routing Logic Removed (Dec 19, 2025)
	// WE is now a pure executor - no routing decisions
	// RO tracks ConsecutiveFailureCount and NextAllowedExecution in RR.Status
	// RO makes ALL routing decisions BEFORE creating WFE
	// ========================================
	logger.V(1).Info("Workflow execution failed - routing handled by RO",
		"wasExecutionFailure", failureDetails != nil && failureDetails.WasExecutionFailure)

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Consolidates phase transition + conditions into single API call
	// ========================================
	failedTransition := failedStatusTransition{
		completionTime: &now,
		durationVal:    durationVal,
		failureDetails: failureDetails,
		failureReason:  failureReason,
		failureMessage: failureMessage,
		summary:        summary,
	}
	if err := r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() error {
		return r.applyFailedStatusTransition(wfe, failedTransition)
	}); err != nil {
		logger.Error(err, "Failed to atomically update status to Failed")
		return ctrl.Result{}, err
	}

	// Issue #1597: audit recorded exactly once, AFTER the phase-transition
	// AtomicStatusUpdate above has durably committed (see MarkCompleted /
	// DD-WE-009 for the full rationale).
	r.recordTerminalAudit(ctx, wfe, "workflow.failed", r.AuditManager.RecordWorkflowFailed, logger)

	// Day 7: Record metrics (BR-WE-008)
	// DD-METRICS-001: Use injected metrics instead of global function
	if r.Metrics != nil {
		r.Metrics.RecordWorkflowFailure(durationSeconds)
	}

	// V1.0: Consecutive failures gauge removed - RO handles routing (DD-RO-002)

	// Emit event.
	// BR-WORKFLOW-008: include FailureDetails.Message alongside the reason so
	// a specific missing-dependency detail (e.g. from Pod FailedMount event
	// enrichment) is visible via `kubectl get events` / `describe wfe`, not
	// just the audit trace or WFE status YAML.
	reason := "Unknown"
	message := "no additional details"
	if wfe.Status.FailureDetails != nil {
		reason = wfe.Status.FailureDetails.Reason
		if wfe.Status.FailureDetails.Message != "" {
			message = wfe.Status.FailureDetails.Message
		}
	}
	r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonWorkflowFailed,
		fmt.Sprintf("Workflow %s failed: %s - %s", wfe.Spec.WorkflowRef.WorkflowID, reason, message))

	// DD-EVENT-001 v1.1: PhaseTransition breadcrumb for Running → Failed
	r.emitPhaseTransition(wfe, "Running", "Failed")

	// Issue #375: Schedule requeue so ReconcileTerminal runs after cooldown to
	// delete the execution resource (Job/PipelineRun) and release the target lock.
	cooldown := r.CooldownPeriod
	if cooldown == 0 {
		cooldown = DefaultCooldownPeriod
	}
	return ctrl.Result{RequeueAfter: cooldown}, nil
}

// resolveMarkFailedDetails extracts and finalizes the FailureDetails for a
// MarkFailed call (Day 7 TaskRun-specific fields, Day 6 Extension
// WasExecutionFailure), overriding with the executor's summary for
// non-Tekton engines per BR-WE-015 (ExtractFailureDetails defaults to
// "Unknown" when pr is nil), generates the natural-language summary, and
// maps the WE failure reason onto the ExecutionComplete condition's
// reason/message. Extracted from MarkFailed (Wave 6 6e-ii GREEN: funlen
// remediation) — pure code motion, no behavior change.
func (r *WorkflowExecutionReconciler) resolveMarkFailedDetails(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, pr *tektonv1.PipelineRun, summary []*workflowexecutionv1alpha1.ExecutionStatusSummary) (*workflowexecutionv1alpha1.FailureDetails, string, string) {
	// Extract failure details (Day 7: includes TaskRun-specific fields, Day 6 Extension: WasExecutionFailure)
	failureDetails := r.ExtractFailureDetails(ctx, pr, wfe.Status.StartTime)

	// BR-WE-015: For non-Tekton executors (ansible, job), the PipelineRun is nil and
	// ExtractFailureDetails defaults to "Unknown". Override with the executor's summary
	// which carries the correct engine-specific reason and message.
	if pr == nil && len(summary) > 0 && summary[0] != nil && failureDetails != nil {
		if summary[0].Reason != "" {
			failureDetails.Reason = mapExecutorReasonToCRDEnum(summary[0].Reason)
		}
		if summary[0].Message != "" {
			failureDetails.Message = summary[0].Message
		}
		failureDetails.WasExecutionFailure = true
	}

	// Generate natural language summary
	if failureDetails != nil {
		failureDetails.NaturalLanguageSummary = r.GenerateNaturalLanguageSummary(wfe, failureDetails)
	}

	// Determine condition values
	failureReason := weconditions.ReasonExecutionFailed
	failureMessage := "Pipeline execution failed"
	if failureDetails != nil {
		// Map WE failure reasons to condition reasons
		switch failureDetails.Reason {
		case "TaskFailed":
			failureReason = weconditions.ReasonTaskFailed
			failureMessage = failureDetails.Message
		case "DeadlineExceeded":
			failureReason = weconditions.ReasonDeadlineExceeded
			failureMessage = "Pipeline exceeded timeout deadline"
		case "OOMKilled":
			failureReason = weconditions.ReasonOOMKilled
			failureMessage = "Pipeline task killed due to out of memory"
		default:
			failureMessage = failureDetails.Message
		}
	}

	return failureDetails, failureReason, failureMessage
}

// failedStatusTransition groups the fields needed by applyFailedStatusTransition
// to keep the method's argument count within the project's argument-limit
// (revive) rule. Introduced during Wave 6 6e-ii GREEN decomposition of
// MarkFailed.
type failedStatusTransition struct {
	completionTime *metav1.Time
	durationVal    *metav1.Duration
	failureDetails *workflowexecutionv1alpha1.FailureDetails
	failureReason  string
	failureMessage string
	summary        []*workflowexecutionv1alpha1.ExecutionStatusSummary
}

// applyFailedStatusTransition is the AtomicStatusUpdate callback body for
// MarkFailed: transitions the phase to Failed (P0: Phase State Machine),
// persists completion time/duration/FailureDetails/ExecutionStatus, and sets
// the TektonPipelineComplete/Ready conditions (BR-WE-006, Issue #79 Phase
// 7b). Extracted from MarkFailed (Wave 6 6e-ii GREEN: funlen/gocyclo/gocognit
// remediation) — pure code motion, no behavior change. Issue #1597: the
// workflow.failed audit event is recorded by the caller (MarkFailed) via
// recordTerminalAudit AFTER this closure commits, not inside it (see
// DD-WE-009).
func (r *WorkflowExecutionReconciler) applyFailedStatusTransition(wfe *workflowexecutionv1alpha1.WorkflowExecution, t failedStatusTransition) error {
	if err := r.PhaseManager.TransitionTo(wfe, wephase.Failed); err != nil {
		return fmt.Errorf("failed to transition to Failed: %w", err)
	}

	wfe.Status.CompletionTime = t.completionTime
	wfe.Status.Duration = t.durationVal
	wfe.Status.FailureDetails = t.failureDetails
	// Issue #1690 RCA follow-up: mirror onto the flat top-level field too --
	// RO's executing_handler.go and E2E scenario assertions read
	// Status.FailureReason directly rather than Status.FailureDetails.Reason,
	// and it was never populated on this path, silently blanking out
	// user-facing failure messaging for every in-execution (job/tekton)
	// failure.
	if t.failureDetails != nil {
		wfe.Status.FailureReason = t.failureDetails.Reason
	}

	// Issue #118 Gap 4: persist ExecutionStatus inside callback (survives refetch)
	if len(t.summary) > 0 && t.summary[0] != nil {
		wfe.Status.ExecutionStatus = t.summary[0]
	}

	// BR-WE-006: Set TektonPipelineComplete condition to False
	weconditions.SetExecutionComplete(wfe, false, t.failureReason, t.failureMessage)

	// Issue #79 Phase 7b: Set Ready condition on terminal transitions
	weconditions.SetReady(wfe, false, weconditions.ReasonNotReady, "Workflow execution failed")

	return nil
}

// ========================================
// MarkFailedWithReason - Handle pre-execution failures
// Used for validation errors, configuration errors before PipelineRun creation
// ========================================
func (r *WorkflowExecutionReconciler) MarkFailedWithReason(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, reason, message string) error {
	logger := log.FromContext(ctx)
	logger.Info("Marking WorkflowExecution as Failed (pre-execution)",
		"reason", reason,
		"message", message,
	)

	now := metav1.Now()
	failureDetails := &workflowexecutionv1alpha1.FailureDetails{
		Reason:              reason,
		Message:             message,
		FailedAt:            now,
		WasExecutionFailure: false,
	}
	failureDetails.NaturalLanguageSummary = r.GenerateNaturalLanguageSummary(wfe, failureDetails)

	conditionReason := weconditions.ReasonExecutionCreationFailed
	switch reason {
	case "QuotaExceeded":
		conditionReason = weconditions.ReasonQuotaExceeded
	case "PermissionDenied", "RBACDenied":
		conditionReason = weconditions.ReasonRBACDenied
	case "ImagePullFailed":
		conditionReason = weconditions.ReasonImagePullFailed
	}

	condMsg := fmt.Sprintf("Failed to create PipelineRun: %s", message)
	if err := r.markFailedInternal(ctx, wfe, failureDetails, conditionReason, condMsg, &now, nil); err != nil {
		return err
	}

	r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonWorkflowFailed,
		fmt.Sprintf("Pre-execution failure: %s - %s", reason, message))
	r.emitPhaseTransition(wfe, "Pending", "Failed")

	logger.Info("WorkflowExecution failed with reason (atomic status update)", "reason", reason)
	return nil
}

// MarkFailedAsDeduplicated marks a WFE as failed due to an execution-time resource
// collision with another WFE. Sets FailureDetails.Reason = Deduplicated and
// DeduplicatedBy = originalWFE inside the AtomicStatusUpdate closure to satisfy
// the M5 constraint (refetch-safe atomic writes). Issue #190.
func (r *WorkflowExecutionReconciler) MarkFailedAsDeduplicated(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, originalWFE string) error {
	logger := log.FromContext(ctx)
	logger.Info("Marking WorkflowExecution as Failed (deduplicated)", "originalWFE", originalWFE)

	now := metav1.Now()
	failureDetails := &workflowexecutionv1alpha1.FailureDetails{
		Reason:              workflowexecutionv1alpha1.FailureReasonDeduplicated,
		Message:             fmt.Sprintf("Execution resource already exists, owned by WorkflowExecution %s", originalWFE),
		FailedAt:            now,
		WasExecutionFailure: false,
	}
	failureDetails.NaturalLanguageSummary = r.GenerateNaturalLanguageSummary(wfe, failureDetails)

	condMsg := fmt.Sprintf("Execution resource collision (deduplicated by %s)", originalWFE)
	extraUpdates := func() {
		wfe.Status.DeduplicatedBy = originalWFE
	}
	if err := r.markFailedInternal(ctx, wfe, failureDetails, weconditions.ReasonExecutionCreationFailed, condMsg, &now, extraUpdates); err != nil {
		return err
	}

	r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonWorkflowFailed,
		fmt.Sprintf("Deduplicated: execution resource collision with %s", originalWFE))
	r.emitPhaseTransition(wfe, "Pending", "Failed")
	return nil
}

// markFailedInternal is the shared core for MarkFailedWithReason and
// MarkFailedAsDeduplicated. It performs the AtomicStatusUpdate with phase
// transition, completion time, failure details, conditions, and metrics.
// The optional extraUpdates callback runs inside the atomic closure to set
// method-specific fields (e.g., DeduplicatedBy for the M5 constraint).
// Issue #1597: the workflow.failed audit event is recorded via
// recordTerminalAudit AFTER the AtomicStatusUpdate below commits, not inside
// its closure (see DD-WE-009) — audit calls inside a RetryOnConflict closure
// re-fire on every conflict-triggered retry, producing duplicate audit
// writes for one logical transition.
func (r *WorkflowExecutionReconciler) markFailedInternal(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	failureDetails *workflowexecutionv1alpha1.FailureDetails,
	conditionReason, conditionMessage string,
	now *metav1.Time,
	extraUpdates func(),
) error {
	logger := log.FromContext(ctx)

	if err := r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() error {
		if err := r.PhaseManager.TransitionTo(wfe, wephase.Failed); err != nil {
			return fmt.Errorf("failed to transition to Failed: %w", err)
		}

		wfe.Status.CompletionTime = now
		wfe.Status.FailureDetails = failureDetails
		// Issue #1690 RCA follow-up: see applyFailedStatusTransition's comment --
		// mirror onto the flat top-level field for the pre-execution path too.
		if failureDetails != nil {
			wfe.Status.FailureReason = failureDetails.Reason
		}

		if extraUpdates != nil {
			extraUpdates()
		}

		weconditions.SetExecutionCreated(wfe, false, conditionReason, conditionMessage)
		weconditions.SetReady(wfe, false, weconditions.ReasonNotReady, "Workflow execution failed")

		return nil
	}); err != nil {
		logger.Error(err, "Failed to atomically update status to Failed")
		return err
	}

	r.recordTerminalAudit(ctx, wfe, "workflow.failed", r.AuditManager.RecordWorkflowFailed, logger)

	if r.Metrics != nil {
		r.Metrics.RecordWorkflowFailure(0)
	}
	return nil
}

// recordTerminalAudit (Issue #1597) records a terminal-transition audit
// event exactly once, AFTER the caller's primary AtomicStatusUpdate has
// durably committed the phase transition, then persists the outcome via a
// second, idempotent AtomicStatusUpdate that only touches the AuditRecorded
// condition. This replaces the prior pattern of calling auditFn and setting
// AuditRecorded INSIDE the primary AtomicStatusUpdate's RetryOnConflict
// closure, which re-ran the audit call on every conflict-triggered retry —
// producing duplicate audit writes for a single logical transition. See
// DD-WE-009 and DD-PERF-001 (the atomic-status-update mandate targets status
// *field* consolidation, not audit-call bundling).
func (r *WorkflowExecutionReconciler) recordTerminalAudit(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	eventName string,
	auditFn func(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) error,
	logger logr.Logger,
) {
	succeeded, reason, message := true, weconditions.ReasonAuditSucceeded,
		fmt.Sprintf("Audit event %s recorded to DataStorage", eventName)
	if err := auditFn(ctx, wfe); err != nil {
		logger.V(1).Info(fmt.Sprintf("Failed to record %s audit event", eventName), "error", err)
		succeeded, reason, message = false, weconditions.ReasonAuditFailed,
			fmt.Sprintf("Failed to record audit event: %v", err)
	}

	if err := r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() error {
		weconditions.SetAuditRecorded(wfe, succeeded, reason, message)
		return nil
	}); err != nil {
		logger.Error(err, "Failed to persist AuditRecorded condition", "event", eventName)
	}
}

// ========================================
// Helper Functions for Status Updates
// ========================================

// updateStatus is a helper that updates the WFE status with consistent error handling
func (r *WorkflowExecutionReconciler) updateStatus(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	operation string,
) error {
	logger := log.FromContext(ctx)

	if err := r.Status().Update(ctx, wfe); err != nil {
		logger.Error(err, "Failed to update status", "operation", operation)
		return err
	}
	return nil
}
