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

// Running/Terminal/Delete lifecycle reconciliation logic, split out of
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// ========================================
// reconcileRunning - Handle Running phase
// Day 5: Status synchronization
// ========================================
func (r *WorkflowExecutionReconciler) reconcileRunning(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Issue #518: Lazy resolution for pre-migration WFEs that lack status.executionEngine.
	if _, engineErr := r.resolveExecutionEngine(ctx, wfe); engineErr != nil {
		logger.Error(engineErr, "Failed to resolve execution engine during Running phase")
		return r.MarkFailed(ctx, wfe, nil)
	}
	logger.Info("Reconciling Running phase", "engine", wfe.Status.ExecutionEngine)

	// ========================================
	// Step 1: Get execution status via executor dispatch (BR-WE-014)
	// ========================================
	exec, err := r.ExecutorRegistry.Get(wfe.Status.ExecutionEngine)
	if err != nil {
		logger.Error(err, "Unsupported execution engine during Running phase")
		return r.MarkFailed(ctx, wfe, nil)
	}

	result, getErr := exec.GetStatus(ctx, wfe, r.ExecutionNamespace)
	if getErr != nil {
		if apierrors.IsNotFound(getErr) {
			logger.Error(getErr, "Execution resource not found - deleted externally",
				"engine", wfe.Status.ExecutionEngine)
			return r.MarkFailed(ctx, wfe, nil)
		}
		return ctrl.Result{}, getErr
	}

	// ========================================
	// Step 2: Map execution result to WFE phase
	// ExecutionStatus is passed into Mark* methods to survive AtomicStatusUpdate refetch
	// ========================================
	if res, err, handled := r.handleRunningExecutionResult(ctx, wfe, result, logger); handled {
		return res, err
	}

	// ========================================
	// Step 4: Update status with current progress
	// ========================================
	if err := r.updateStatus(ctx, wfe, "current progress"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// handleRunningExecutionResult maps a Running-phase executor result to the
// WFE's terminal Mark* transition (BR-WE-014): Completed/Failed results are
// dispatched to MarkCompleted/MarkFailed (with a best-effort Tekton
// PipelineRun fetch for annotation purposes), while any other phase is left
// unhandled (handled=false) so the caller continues the still-running path.
// Extracted from reconcileRunning (Wave 6 6e-ii GREEN: funlen remediation) —
// pure code motion, no behavior change.
func (r *WorkflowExecutionReconciler) handleRunningExecutionResult(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, result *weexecutor.ExecutionResult, logger logr.Logger) (ctrl.Result, error, bool) {
	switch result.Phase {
	case workflowexecutionv1alpha1.PhaseCompleted:
		logger.Info("Execution succeeded", "engine", wfe.Status.ExecutionEngine)
		pr := r.fetchTektonPipelineRunForMark(ctx, wfe)
		res, err := r.MarkCompleted(ctx, wfe, pr, result.Summary)
		return res, err, true

	case workflowexecutionv1alpha1.PhaseFailed:
		logger.Info("Execution failed", "engine", wfe.Status.ExecutionEngine, "reason", result.Reason)
		pr := r.fetchTektonPipelineRunForMark(ctx, wfe)
		res, err := r.MarkFailed(ctx, wfe, pr, result.Summary)
		return res, err, true

	default:
		// Still running - update conditions and requeue
		logger.V(1).Info("Execution still running", "reason", result.Reason, "engine", wfe.Status.ExecutionEngine)
		weconditions.SetExecutionRunning(wfe, true,
			weconditions.ReasonExecutionStarted,
			fmt.Sprintf("Execution running (%s: %s)", wfe.Status.ExecutionEngine, result.Reason))
		return ctrl.Result{}, nil, false
	}
}

// fetchTektonPipelineRunForMark best-effort fetches the Tekton PipelineRun
// backing wfe (when the execution engine is tekton) for inclusion in the
// terminal Mark* audit/status payload; returns nil when the engine isn't
// tekton or the fetch fails (Mark* methods gracefully handle a nil
// PipelineRun). Extracted from reconcileRunning (Wave 6 6e-ii GREEN: funlen
// remediation) — pure code motion, no behavior change.
func (r *WorkflowExecutionReconciler) fetchTektonPipelineRunForMark(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) *tektonv1.PipelineRun {
	if wfe.Status.ExecutionEngine != workflowexecutionv1alpha1.ExecutionEngineTekton {
		return nil
	}
	var pr tektonv1.PipelineRun
	if err := r.Get(ctx, client.ObjectKey{
		Name:      wfe.Status.ExecutionRef.Name,
		Namespace: r.ExecutionNamespace,
	}, &pr); err != nil {
		return nil
	}
	return &pr
}

// ========================================
// ReconcileTerminal - Handle Completed/Failed phases
// Day 6: Cooldown enforcement and cleanup
// DD-WE-003: Lock Persistence (Deterministic Name)
// ========================================
func (r *WorkflowExecutionReconciler) ReconcileTerminal(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Issue #518: Lazy resolution for pre-migration WFEs (non-fatal for terminal phase).
	if _, engineErr := r.resolveExecutionEngine(ctx, wfe); engineErr != nil {
		logger.V(1).Info("Could not resolve execution engine in terminal phase (non-fatal)", "error", engineErr)
	}
	logger.Info("Reconciling Terminal phase", "phase", wfe.Status.Phase)

	// WE-H3: No completion time in terminal phase means we can't calculate cooldown,
	// but we must still release execution resources. Set completionTime now so cleanup proceeds.
	if wfe.Status.CompletionTime == nil {
		logger.Info("No completion time set in terminal phase — setting now to unblock cleanup")
		now := metav1.Now()
		wfe.Status.CompletionTime = &now
	}

	// Get cooldown period (use default if not set)
	cooldown := r.CooldownPeriod
	if cooldown == 0 {
		cooldown = DefaultCooldownPeriod
	}

	// Calculate elapsed time since completion
	elapsed := time.Since(wfe.Status.CompletionTime.Time)

	// Wait for cooldown before releasing lock
	if elapsed < cooldown {
		remaining := cooldown - elapsed
		logger.V(1).Info("Waiting for cooldown",
			"remaining", remaining,
			"targetResource", wfe.Spec.TargetResource,
		)
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// Cooldown expired - delete execution resource to release lock
	// DD-WE-003: Use deterministic name for atomic locking
	if err := r.releaseExecutionLock(ctx, wfe, cooldown, logger); err != nil {
		return ctrl.Result{}, err
	}

	// Emit LockReleased event
	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonLockReleased,
		fmt.Sprintf("Lock released for %s after cooldown", wfe.Spec.TargetResource))

	return ctrl.Result{}, nil
}

// releaseExecutionLock deletes the execution resource to release the
// DD-WE-003 deterministic-name lock once cooldown has expired, dispatching
// to the ExecutorRegistry when configured, or falling back to inline Tekton
// PipelineRun cleanup otherwise. Extracted from ReconcileTerminal (Wave 6
// 6e-ii GREEN: funlen/nestif remediation) — pure code motion, no behavior
// change.
func (r *WorkflowExecutionReconciler) releaseExecutionLock(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, cooldown time.Duration, logger logr.Logger) error {
	if r.ExecutorRegistry == nil {
		return r.releaseExecutionLockFallback(ctx, wfe, cooldown, logger)
	}
	return r.releaseExecutionLockViaExecutor(ctx, wfe, cooldown, logger)
}

// releaseExecutionLockViaExecutor dispatches lock release through the
// ExecutorRegistry (BR-WE-014). Extracted from ReconcileTerminal (Wave 6
// 6e-ii GREEN: funlen/nestif remediation) — pure code motion, no behavior
// change.
func (r *WorkflowExecutionReconciler) releaseExecutionLockViaExecutor(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, cooldown time.Duration, logger logr.Logger) error {
	exec, execErr := r.ExecutorRegistry.Get(wfe.Status.ExecutionEngine)
	if execErr != nil {
		logger.Error(execErr, "Unknown engine during cooldown cleanup, skipping",
			"engine", wfe.Status.ExecutionEngine)
		return nil
	}
	if exec == nil {
		return nil
	}

	if err := exec.Cleanup(ctx, wfe, r.ExecutionNamespace); err != nil {
		logger.Error(err, "Failed to cleanup execution resource after cooldown",
			"engine", exec.Engine())
		// DD-EVENT-001 v1.1: Emit CleanupFailed event (P4: Error Path)
		r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonCleanupFailed,
			fmt.Sprintf("Cleanup failed after cooldown: %v", err))
		return err
	}
	logger.Info("Lock released after cooldown",
		"targetResource", wfe.Spec.TargetResource,
		"engine", exec.Engine(),
		"cooldownPeriod", cooldown,
	)
	return nil
}

// releaseExecutionLockFallback performs inline Tekton PipelineRun cleanup
// when the ExecutorRegistry is not configured. Extracted from
// ReconcileTerminal (Wave 6 6e-ii GREEN: funlen/nestif remediation) — pure
// code motion, no behavior change.
func (r *WorkflowExecutionReconciler) releaseExecutionLockFallback(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, cooldown time.Duration, logger logr.Logger) error {
	prName := weexecutor.ExecutionResourceName(wfe.Spec.TargetResource)
	var existing tektonv1.PipelineRun
	err := r.Get(ctx, client.ObjectKey{Name: prName, Namespace: r.ExecutionNamespace}, &existing)
	if err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to get PipelineRun for ownership check")
		return err
	}
	if err == nil && existing.Labels["kubernaut.ai/workflow-execution"] == wfe.Name {
		if delErr := r.Delete(ctx, &existing); delErr != nil && !apierrors.IsNotFound(delErr) {
			logger.Error(delErr, "Failed to delete PipelineRun after cooldown")
			return delErr
		}
	}
	logger.Info("Lock released after cooldown",
		"targetResource", wfe.Spec.TargetResource,
		"cooldownPeriod", cooldown,
	)
	return nil
}

// ========================================
// CheckCooldownActive checks if cooldown is active for a target resource
// BR-WE-009: Cooldown Period is Configurable
// Returns (remaining duration, is active)
// currentWFEName format: "namespace/name" to uniquely identify the current WFE
// ========================================
func (r *WorkflowExecutionReconciler) CheckCooldownActive(ctx context.Context, targetResource, currentWFEKey string) (time.Duration, bool) {
	logger := log.FromContext(ctx)

	// Get cooldown period (use default if not set)
	cooldown := r.CooldownPeriod
	if cooldown == 0 {
		cooldown = DefaultCooldownPeriod
	}

	// Query all WorkflowExecutions with the same targetResource.
	// Retry up to 3 times with short backoff to tolerate transient informer/API
	// pressure in resource-constrained environments (e.g., Kind CI clusters).
	// Only fail-closed if ALL attempts fail (DD-WE-001 compliance).
	wfeList := &workflowexecutionv1alpha1.WorkflowExecutionList{}
	const maxAttempts = 3
	var listErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		listErr = r.List(ctx, wfeList, client.MatchingFields{"spec.targetResource": targetResource})
		if listErr == nil {
			break
		}
		if attempt < maxAttempts {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if listErr != nil {
		logger.Error(listErr, "Failed to list WorkflowExecutions for cooldown check after retries — failing closed",
			"targetResource", targetResource, "attempts", maxAttempts)
		return cooldown, true
	}

	// Find any completed/failed WFE still within cooldown period
	now := time.Now()
	for i := range wfeList.Items {
		otherWFE := &wfeList.Items[i]

		// Skip the current WFE (don't check cooldown against ourselves)
		otherKey := fmt.Sprintf("%s/%s", otherWFE.Namespace, otherWFE.Name)
		if otherKey == currentWFEKey {
			continue
		}

		// Only check terminal phases (Completed or Failed)
		if otherWFE.Status.Phase != workflowexecutionv1alpha1.PhaseCompleted &&
			otherWFE.Status.Phase != workflowexecutionv1alpha1.PhaseFailed {
			continue
		}

		// Check if completion time is set
		if otherWFE.Status.CompletionTime == nil {
			continue
		}

		// Calculate elapsed time since completion
		elapsed := now.Sub(otherWFE.Status.CompletionTime.Time)

		// Check if still within cooldown period
		if elapsed < cooldown {
			remaining := cooldown - elapsed
			logger.V(1).Info("Cooldown active for target resource",
				"targetResource", targetResource,
				"blockingWFE", otherKey,
				"remaining", remaining,
			)
			return remaining, true
		}
	}

	// No active cooldown found
	return 0, false
}

// ========================================
// ReconcileDelete - Handle deletion with finalizer
// DD-WE-003: Use deterministic PipelineRun name
// finalizers-lifecycle.md: Event emission
// ========================================
func (r *WorkflowExecutionReconciler) ReconcileDelete(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Issue #518: Lazy resolution for pre-migration WFEs (non-fatal for delete).
	if _, engineErr := r.resolveExecutionEngine(ctx, wfe); engineErr != nil {
		logger.V(1).Info("Could not resolve execution engine during delete (non-fatal, will try fallback)", "error", engineErr)
	}
	logger.Info("Reconciling Delete")

	// Check if finalizer is present
	if !controllerutil.ContainsFinalizer(wfe, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// ========================================
	// Cleanup: Delete execution resource via executor dispatch (BR-WE-014, DD-WE-003)
	// This ensures cleanup even if ExecutionRef was never set
	// ========================================
	if err := r.cleanupExecutionResourceForDelete(ctx, wfe, logger); err != nil {
		return ctrl.Result{}, err
	}

	// ========================================
	// Emit deletion event (finalizers-lifecycle.md)
	// ========================================
	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonWorkflowExecutionDeleted,
		fmt.Sprintf("WorkflowExecution cleanup completed (phase: %s)", wfe.Status.Phase))

	// ========================================
	// Remove Finalizer
	// ========================================
	logger.Info("Removing finalizer", "finalizer", FinalizerName)
	controllerutil.RemoveFinalizer(wfe, FinalizerName)
	if err := r.Update(ctx, wfe); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("WorkflowExecution cleanup complete")
	return ctrl.Result{}, nil
}

// cleanupExecutionResourceForDelete deletes the execution resource during
// finalization (BR-WE-014, DD-WE-003), ensuring cleanup even if ExecutionRef
// was never set, dispatching to the ExecutorRegistry when configured, or
// falling back to inline Tekton PipelineRun deletion otherwise. Extracted
// from ReconcileDelete (Wave 6 6e-ii GREEN: funlen/nestif remediation) —
// pure code motion, no behavior change.
func (r *WorkflowExecutionReconciler) cleanupExecutionResourceForDelete(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, logger logr.Logger) error {
	if r.ExecutorRegistry == nil {
		return r.cleanupExecutionResourceForDeleteFallback(ctx, wfe, logger)
	}
	return r.cleanupExecutionResourceForDeleteViaExecutor(ctx, wfe, logger)
}

// cleanupExecutionResourceForDeleteViaExecutor dispatches finalization
// cleanup through the ExecutorRegistry. Extracted from ReconcileDelete
// (Wave 6 6e-ii GREEN: funlen/nestif remediation) — pure code motion, no
// behavior change.
func (r *WorkflowExecutionReconciler) cleanupExecutionResourceForDeleteViaExecutor(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, logger logr.Logger) error {
	exec, execErr := r.ExecutorRegistry.Get(wfe.Status.ExecutionEngine)
	if execErr != nil {
		logger.Error(execErr, "Unknown engine during finalization cleanup, skipping executor cleanup",
			"engine", wfe.Status.ExecutionEngine)
		return nil
	}
	if exec == nil {
		return nil
	}

	logger.Info("Cleaning up execution resource",
		"engine", exec.Engine(),
		"namespace", r.ExecutionNamespace,
	)
	if err := exec.Cleanup(ctx, wfe, r.ExecutionNamespace); err != nil {
		logger.Error(err, "Failed to cleanup execution resource during finalization",
			"engine", exec.Engine())
		// DD-EVENT-001 v1.1: Emit CleanupFailed event (P4: Error Path)
		r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonCleanupFailed,
			fmt.Sprintf("Cleanup failed during finalization: %v", err))
		return err
	}
	logger.Info("Finalizer: cleaned up execution resource", "engine", exec.Engine())
	return nil
}

// cleanupExecutionResourceForDeleteFallback performs inline Tekton
// PipelineRun cleanup when the ExecutorRegistry is not configured. Extracted
// from ReconcileDelete (Wave 6 6e-ii GREEN: funlen/nestif remediation) —
// pure code motion, no behavior change.
func (r *WorkflowExecutionReconciler) cleanupExecutionResourceForDeleteFallback(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, logger logr.Logger) error {
	prName := weexecutor.ExecutionResourceName(wfe.Spec.TargetResource)
	pr := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prName,
			Namespace: r.ExecutionNamespace,
		},
	}
	if err := r.Delete(ctx, pr); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to delete PipelineRun during finalization")
			return err
		}
		return nil
	}
	logger.Info("Finalizer: deleted associated PipelineRun", "pipelineRun", prName)
	return nil
}
