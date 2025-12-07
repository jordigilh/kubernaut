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
	"crypto/sha256"
	"encoding/hex"
	encJSON "encoding/json"
	"fmt"
	"strings"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

// ========================================
// WorkflowExecution Controller
// ADR-044: Tekton PipelineRun Delegation
// DD-WE-001: Resource Locking Safety
// DD-WE-002: Dedicated Execution Namespace
// DD-WE-003: Lock Persistence (Deterministic Name)
// ========================================

const (
	// FinalizerName is the finalizer for WorkflowExecution cleanup
	// Per finalizers-lifecycle.md: domain/resource-cleanup pattern
	FinalizerName = "workflowexecution.kubernaut.ai/workflowexecution-cleanup"

	// DefaultCooldownPeriod is the default time between workflow executions on same target
	DefaultCooldownPeriod = 5 * time.Minute

	// DefaultServiceAccountName is the default SA for PipelineRuns
	DefaultServiceAccountName = "kubernaut-workflow-runner"
)

// WorkflowExecutionReconciler reconciles a WorkflowExecution object
type WorkflowExecutionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// ExecutionNamespace is where PipelineRuns are created (DD-WE-002)
	// Default: "kubernaut-workflows"
	ExecutionNamespace string

	// CooldownPeriod prevents redundant sequential workflows (DD-WE-001)
	// Default: 5 minutes
	CooldownPeriod time.Duration

	// ServiceAccountName for PipelineRuns
	// Default: "kubernaut-workflow-runner"
	ServiceAccountName string

	// AuditStore for writing audit events (BR-WE-005, ADR-032)
	// Uses pkg/audit buffered store via Data Storage Service
	// Optional: nil disables audit (graceful degradation)
	AuditStore audit.AuditStore

	// ========================================
	// EXPONENTIAL BACKOFF CONFIGURATION (BR-WE-012, DD-WE-004)
	// ========================================

	// BaseCooldownPeriod is the initial cooldown for exponential backoff
	// Formula: Cooldown = BaseCooldownPeriod * 2^(min(failures-1, MaxBackoffExponent))
	// Default: 1 minute
	BaseCooldownPeriod time.Duration

	// MaxCooldownPeriod caps the exponential backoff
	// Default: 10 minutes (prevents RR timeout)
	MaxCooldownPeriod time.Duration

	// MaxBackoffExponent limits exponential growth
	// e.g., 4 means max multiplier is 2^4 = 16x
	// Default: 4
	MaxBackoffExponent int

	// MaxConsecutiveFailures before auto-failing with ExhaustedRetries
	// After this many consecutive pre-execution failures, skip with ExhaustedRetries
	// Default: 5
	MaxConsecutiveFailures int
}

// ========================================
// RBAC Markers
// ========================================

//+kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=tekton.dev,resources=taskruns,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles WorkflowExecution reconciliation
// Phase-based reconciliation per implementation plan
func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the WorkflowExecution instance
	var wfe workflowexecutionv1alpha1.WorkflowExecution
	if err := r.Get(ctx, req.NamespacedName, &wfe); err != nil {
		// Ignore not-found errors (deleted before reconcile)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling WorkflowExecution",
		"name", wfe.Name,
		"namespace", wfe.Namespace,
		"phase", wfe.Status.Phase,
	)

	// ========================================
	// Handle Deletion
	// ========================================
	if !wfe.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(ctx, &wfe)
	}

	// ========================================
	// Add Finalizer (if not present)
	// ========================================
	if !controllerutil.ContainsFinalizer(&wfe, FinalizerName) {
		logger.Info("Adding finalizer", "finalizer", FinalizerName)
		controllerutil.AddFinalizer(&wfe, FinalizerName)
		if err := r.Update(ctx, &wfe); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		// Requeue after adding finalizer
		return ctrl.Result{Requeue: true}, nil
	}

	// ========================================
	// Phase-Based Reconciliation
	// ========================================
	switch wfe.Status.Phase {
	case "", workflowexecutionv1alpha1.PhasePending:
		return r.reconcilePending(ctx, &wfe)
	case workflowexecutionv1alpha1.PhaseRunning:
		return r.reconcileRunning(ctx, &wfe)
	case workflowexecutionv1alpha1.PhaseCompleted, workflowexecutionv1alpha1.PhaseFailed:
		return r.ReconcileTerminal(ctx, &wfe)
	case workflowexecutionv1alpha1.PhaseSkipped:
		// Skipped is terminal - no action needed
		return ctrl.Result{}, nil
	default:
		logger.Error(nil, "Unknown phase", "phase", wfe.Status.Phase)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
}

// ========================================
// reconcilePending - Handle Pending phase
// Day 3-4: Resource lock check + PipelineRun creation
// ========================================
func (r *WorkflowExecutionReconciler) reconcilePending(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Pending phase")

	// ========================================
	// Step 1: Check resource lock (DD-WE-001)
	// ========================================
	blocked, skipDetails, err := r.CheckResourceLock(ctx, wfe)
	if err != nil {
		logger.Error(err, "Failed to check resource lock")
		return ctrl.Result{}, err
	}
	if blocked {
		logger.Info("Resource is locked, skipping execution",
			"reason", skipDetails.Reason,
		)
		return ctrl.Result{}, r.MarkSkipped(ctx, wfe, skipDetails)
	}

	// ========================================
	// Step 2: Check cooldown (DD-WE-001)
	// ========================================
	blocked, skipDetails, err = r.CheckCooldown(ctx, wfe)
	if err != nil {
		logger.Error(err, "Failed to check cooldown")
		return ctrl.Result{}, err
	}
	if blocked {
		logger.Info("Cooldown active, skipping execution",
			"reason", skipDetails.Reason,
		)
		return ctrl.Result{}, r.MarkSkipped(ctx, wfe, skipDetails)
	}

	// ========================================
	// Step 3: Build and create PipelineRun (Day 4)
	// ========================================
	pr := r.BuildPipelineRun(wfe)
	logger.Info("Creating PipelineRun",
		"pipelineRun", pr.Name,
		"namespace", pr.Namespace,
	)

	if err := r.Create(ctx, pr); err != nil {
		// Check if this is an AlreadyExists error (race condition caught by DD-WE-003)
		skipDetails, handleErr := r.HandleAlreadyExists(ctx, wfe, err)
		if handleErr != nil {
			logger.Error(handleErr, "Failed to create PipelineRun")
			return ctrl.Result{}, handleErr
		}
		if skipDetails != nil {
			// Race condition - another WFE created the PipelineRun
			logger.Info("Race condition detected, skipping execution")
			return ctrl.Result{}, r.MarkSkipped(ctx, wfe, skipDetails)
		}
		// skipDetails == nil means the PipelineRun is ours (we won the race somehow)
		// This is rare but safe - continue to update status
	}

	// Day 7: Record PipelineRun creation metric (BR-WE-008)
	RecordPipelineRunCreation()

	// ========================================
	// Step 4: Update WFE status to Running
	// ========================================
	now := metav1.Now()
	wfe.Status.Phase = workflowexecutionv1alpha1.PhaseRunning
	wfe.Status.StartTime = &now
	wfe.Status.PipelineRunRef = &corev1.LocalObjectReference{
		Name: pr.Name,
	}

	if err := r.Status().Update(ctx, wfe); err != nil {
		logger.Error(err, "Failed to update status to Running")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(wfe, "Normal", "PipelineRunCreated",
		fmt.Sprintf("Created PipelineRun %s/%s", pr.Namespace, pr.Name))

	// Requeue to check PipelineRun status
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// ========================================
// reconcileRunning - Handle Running phase
// Day 5: Status synchronization
// ========================================
func (r *WorkflowExecutionReconciler) reconcileRunning(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Running phase")

	// ========================================
	// Step 1: Fetch PipelineRun from execution namespace (DD-WE-002)
	// ========================================
	var pr tektonv1.PipelineRun
	if err := r.Get(ctx, client.ObjectKey{
		Name:      wfe.Status.PipelineRunRef.Name,
		Namespace: r.ExecutionNamespace,
	}, &pr); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Error(err, "PipelineRun not found - deleted externally")
			return r.MarkFailed(ctx, wfe, nil)
		}
		return ctrl.Result{}, err
	}

	// ========================================
	// Step 2: Update PipelineRunStatusSummary for visibility (v3.2)
	// ========================================
	wfe.Status.PipelineRunStatus = r.BuildPipelineRunStatusSummary(&pr)

	// ========================================
	// Step 3: Map Tekton status to WFE phase using knative APIs (v3.2)
	// ========================================
	succeededCond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if succeededCond != nil {
		switch {
		case succeededCond.IsTrue():
			// Success - calculate duration and mark completed
			logger.Info("PipelineRun succeeded")
			return r.MarkCompleted(ctx, wfe, &pr)
		case succeededCond.IsFalse():
			// Failure - extract details and mark failed
			logger.Info("PipelineRun failed", "reason", succeededCond.Reason)
			return r.MarkFailed(ctx, wfe, &pr)
		default:
			// Still running - update status and requeue
			logger.V(1).Info("PipelineRun still running", "reason", succeededCond.Reason)
		}
	}

	// ========================================
	// Step 4: Update status with current progress
	// ========================================
	if err := r.Status().Update(ctx, wfe); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// ========================================
// ReconcileTerminal - Handle Completed/Failed phases
// Day 6: Cooldown enforcement and cleanup
// DD-WE-003: Lock Persistence (Deterministic Name)
// ========================================
func (r *WorkflowExecutionReconciler) ReconcileTerminal(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Terminal phase", "phase", wfe.Status.Phase)

	// Guard clause: no completion time means we can't calculate cooldown
	if wfe.Status.CompletionTime == nil {
		logger.V(1).Info("No completion time set, skipping cooldown check")
		return ctrl.Result{}, nil
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

	// Cooldown expired - delete PipelineRun to release lock
	// DD-WE-003: Use deterministic name for atomic locking
	prName := PipelineRunName(wfe.Spec.TargetResource)
	pr := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prName,
			Namespace: r.ExecutionNamespace,
		},
	}

	if err := r.Delete(ctx, pr); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to delete PipelineRun after cooldown")
			return ctrl.Result{}, err
		}
		// PipelineRun already deleted - continue
		logger.V(1).Info("PipelineRun already deleted", "pipelineRun", prName)
	} else {
		logger.Info("Lock released after cooldown",
			"targetResource", wfe.Spec.TargetResource,
			"pipelineRun", prName,
			"cooldownPeriod", cooldown,
		)
	}

	// Emit LockReleased event
	r.Recorder.Event(wfe, "Normal", "LockReleased",
		fmt.Sprintf("Lock released for %s after cooldown", wfe.Spec.TargetResource))

	return ctrl.Result{}, nil
}

// ========================================
// ReconcileDelete - Handle deletion with finalizer
// DD-WE-003: Use deterministic PipelineRun name
// finalizers-lifecycle.md: Event emission
// ========================================
func (r *WorkflowExecutionReconciler) ReconcileDelete(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Delete")

	// Check if finalizer is present
	if !controllerutil.ContainsFinalizer(wfe, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// ========================================
	// Cleanup: Delete PipelineRun using deterministic name (DD-WE-003)
	// This ensures cleanup even if PipelineRunRef was never set
	// ========================================
	prName := PipelineRunName(wfe.Spec.TargetResource)
	logger.Info("Deleting associated PipelineRun",
		"pipelineRun", prName,
		"namespace", r.ExecutionNamespace,
	)

	pr := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prName,
			Namespace: r.ExecutionNamespace,
		},
	}

	if err := r.Delete(ctx, pr); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to delete PipelineRun during finalization")
			return ctrl.Result{}, err
		}
		// PipelineRun already deleted - continue
		logger.V(1).Info("PipelineRun already deleted", "pipelineRun", prName)
	} else {
		logger.Info("Finalizer: deleted associated PipelineRun", "pipelineRun", prName)
	}

	// ========================================
	// Emit deletion event (finalizers-lifecycle.md)
	// ========================================
	r.Recorder.Event(wfe, "Normal", "WorkflowExecutionDeleted",
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

// ========================================
// SetupWithManager sets up the controller with the Manager
// ========================================
func (r *WorkflowExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create index on targetResource for O(1) lock check (DD-WE-003)
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&workflowexecutionv1alpha1.WorkflowExecution{},
		"spec.targetResource",
		func(obj client.Object) []string {
			wfe := obj.(*workflowexecutionv1alpha1.WorkflowExecution)
			return []string{wfe.Spec.TargetResource}
		},
	); err != nil {
		return fmt.Errorf("failed to create field index on spec.targetResource: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&workflowexecutionv1alpha1.WorkflowExecution{}).
		// Watch PipelineRuns in execution namespace (cross-namespace via label)
		// Only watch PipelineRuns with our label to avoid unnecessary reconciles
		Watches(
			&tektonv1.PipelineRun{},
			handler.EnqueueRequestsFromMapFunc(r.FindWFEForPipelineRun),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				// Only watch PipelineRuns that have our label
				labels := obj.GetLabels()
				if labels == nil {
					return false
				}
				_, hasLabel := labels["kubernaut.ai/workflow-execution"]
				return hasLabel
			})),
		).
		Complete(r)
}

// ========================================
// PipelineRunName generates deterministic name from targetResource
// DD-WE-003: Lock Persistence via Deterministic Name
// Format: wfe-<sha256(targetResource)[:16]>
// ========================================
func PipelineRunName(targetResource string) string {
	h := sha256.Sum256([]byte(targetResource))
	return fmt.Sprintf("wfe-%s", hex.EncodeToString(h[:])[:16])
}

// sanitizeLabelValue makes a string safe for use as a Kubernetes label value
// Label values must consist of alphanumeric characters, '-', '_' or '.'
// and must start and end with an alphanumeric character
func sanitizeLabelValue(s string) string {
	// Replace forward slashes with double underscores
	result := strings.ReplaceAll(s, "/", "__")
	// Truncate to max 63 characters (K8s label value limit)
	if len(result) > 63 {
		result = result[:63]
	}
	return result
}

// ========================================
// CheckResourceLock checks if another WFE is Running for same targetResource
// DD-WE-001: Resource Locking Safety (Layer 1 - Active Lock Check)
// Returns: blocked, skipDetails, error
// ========================================
func (r *WorkflowExecutionReconciler) CheckResourceLock(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (bool, *workflowexecutionv1alpha1.SkipDetails, error) {
	logger := log.FromContext(ctx)

	// List all WFEs targeting the same resource
	var wfeList workflowexecutionv1alpha1.WorkflowExecutionList
	if err := r.List(ctx, &wfeList, client.MatchingFields{
		"spec.targetResource": wfe.Spec.TargetResource,
	}); err != nil {
		// If index not found, fall back to filter in memory
		if err := r.List(ctx, &wfeList); err != nil {
			logger.Error(err, "Failed to list WorkflowExecutions")
			return false, nil, err
		}
	}

	// Check if any Running WFE exists for this targetResource (excluding self)
	for _, existing := range wfeList.Items {
		// Skip self
		if existing.UID == wfe.UID {
			continue
		}

		// Skip different targetResource (in case index wasn't available)
		if existing.Spec.TargetResource != wfe.Spec.TargetResource {
			continue
		}

		// Check if Running (active lock)
		if existing.Status.Phase == workflowexecutionv1alpha1.PhaseRunning {
			logger.Info("Resource lock detected",
				"blockedBy", existing.Name,
				"targetResource", wfe.Spec.TargetResource,
			)

			startedAt := metav1.Now()
			if existing.Status.StartTime != nil {
				startedAt = *existing.Status.StartTime
			}

			return true, &workflowexecutionv1alpha1.SkipDetails{
				Reason:    workflowexecutionv1alpha1.SkipReasonResourceBusy,
				Message:   fmt.Sprintf("Another workflow execution '%s' is already running for resource '%s'", existing.Name, wfe.Spec.TargetResource),
				SkippedAt: metav1.Now(),
				ConflictingWorkflow: &workflowexecutionv1alpha1.ConflictingWorkflowRef{
					Name:           existing.Name,
					WorkflowID:     existing.Spec.WorkflowRef.WorkflowID,
					StartedAt:      startedAt,
					TargetResource: existing.Spec.TargetResource,
				},
			}, nil
		}
	}

	return false, nil, nil
}

// ========================================
// CheckCooldown checks cooldown with exponential backoff (DD-WE-004)
// DD-WE-001: Resource Locking Safety (Cooldown Check)
// DD-WE-004: Exponential Backoff for pre-execution failures
//
// Priority (checked in order):
//  1. wasExecutionFailure: true → PreviousExecutionFailed (blocks ALL retries)
//  2. ConsecutiveFailures >= Max → ExhaustedRetries
//  3. time.Now() < NextAllowedExecution → RecentlyRemediated (backoff active)
//  4. Regular cooldown check (for successful completions)
//
// Returns: blocked, skipDetails, error
// ========================================
func (r *WorkflowExecutionReconciler) CheckCooldown(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (bool, *workflowexecutionv1alpha1.SkipDetails, error) {
	logger := log.FromContext(ctx)

	// Find most recent terminal WFE for same target
	recentWFE := r.FindMostRecentTerminalWFE(ctx, wfe)
	if recentWFE == nil {
		return false, nil, nil // No history, allow execution
	}

	now := time.Now()

	// ========================================
	// DD-WE-004-1: Check execution failure FIRST (blocks ALL retries)
	// If previous WFE ran and failed, block with PreviousExecutionFailed
	// ========================================
	if recentWFE.Status.Phase == workflowexecutionv1alpha1.PhaseFailed &&
		recentWFE.Status.FailureDetails != nil &&
		recentWFE.Status.FailureDetails.WasExecutionFailure {

		logger.Info("Blocking due to previous execution failure",
			"blockedBy", recentWFE.Name,
			"targetResource", wfe.Spec.TargetResource,
			"wasExecutionFailure", true,
		)

		return true, &workflowexecutionv1alpha1.SkipDetails{
			Reason:    workflowexecutionv1alpha1.SkipReasonPreviousExecutionFailed,
			Message:   fmt.Sprintf("Previous execution '%s' failed during workflow run on target '%s'. Manual intervention required. Non-idempotent actions may have occurred.", recentWFE.Name, wfe.Spec.TargetResource),
			SkippedAt: metav1.Now(),
			RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
				Name:           recentWFE.Name,
				WorkflowID:     recentWFE.Spec.WorkflowRef.WorkflowID,
				CompletedAt:    *recentWFE.Status.CompletionTime,
				Outcome:        string(recentWFE.Status.Phase),
				TargetResource: recentWFE.Spec.TargetResource,
			},
		}, nil
	}

	// ========================================
	// DD-WE-004-3: Check exhausted retries
	// After MaxConsecutiveFailures pre-execution failures, block permanently
	// ========================================
	if r.MaxConsecutiveFailures > 0 &&
		recentWFE.Status.ConsecutiveFailures >= int32(r.MaxConsecutiveFailures) {

		logger.Info("Blocking due to exhausted retries",
			"blockedBy", recentWFE.Name,
			"targetResource", wfe.Spec.TargetResource,
			"consecutiveFailures", recentWFE.Status.ConsecutiveFailures,
			"maxConsecutiveFailures", r.MaxConsecutiveFailures,
		)

		return true, &workflowexecutionv1alpha1.SkipDetails{
			Reason:    workflowexecutionv1alpha1.SkipReasonExhaustedRetries,
			Message:   fmt.Sprintf("Max consecutive failures (%d) reached for target '%s'. Manual intervention required.", r.MaxConsecutiveFailures, wfe.Spec.TargetResource),
			SkippedAt: metav1.Now(),
			RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
				Name:           recentWFE.Name,
				WorkflowID:     recentWFE.Spec.WorkflowRef.WorkflowID,
				CompletedAt:    *recentWFE.Status.CompletionTime,
				Outcome:        string(recentWFE.Status.Phase),
				TargetResource: recentWFE.Spec.TargetResource,
			},
		}, nil
	}

	// ========================================
	// DD-WE-004-2: Check exponential backoff (NextAllowedExecution)
	// For pre-execution failures with backoff still active
	// ========================================
	if recentWFE.Status.NextAllowedExecution != nil &&
		now.Before(recentWFE.Status.NextAllowedExecution.Time) {

		remainingBackoff := recentWFE.Status.NextAllowedExecution.Time.Sub(now)
		logger.Info("Backoff active",
			"blockedBy", recentWFE.Name,
			"targetResource", wfe.Spec.TargetResource,
			"nextAllowedExecution", recentWFE.Status.NextAllowedExecution.Time,
			"remainingBackoff", remainingBackoff,
		)

		return true, &workflowexecutionv1alpha1.SkipDetails{
			Reason:    workflowexecutionv1alpha1.SkipReasonRecentlyRemediated,
			Message:   fmt.Sprintf("Backoff active for target '%s'. Next allowed: %v (remaining: %v)", wfe.Spec.TargetResource, recentWFE.Status.NextAllowedExecution.Time.Format(time.RFC3339), remainingBackoff.Round(time.Second)),
			SkippedAt: metav1.Now(),
			RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
				Name:              recentWFE.Name,
				WorkflowID:        recentWFE.Spec.WorkflowRef.WorkflowID,
				CompletedAt:       *recentWFE.Status.CompletionTime,
				Outcome:           string(recentWFE.Status.Phase),
				TargetResource:    recentWFE.Spec.TargetResource,
				CooldownRemaining: remainingBackoff.Round(time.Second).String(),
			},
		}, nil
	}

	// ========================================
	// Regular cooldown check (for successful completions)
	// ========================================
	if r.CooldownPeriod > 0 && recentWFE.Status.CompletionTime != nil {
		cooldownThreshold := now.Add(-r.CooldownPeriod)
		if recentWFE.Status.CompletionTime.After(cooldownThreshold) {
			remainingCooldown := recentWFE.Status.CompletionTime.Add(r.CooldownPeriod).Sub(now)
			logger.Info("Cooldown active",
				"blockedBy", recentWFE.Name,
				"targetResource", wfe.Spec.TargetResource,
				"remainingCooldown", remainingCooldown,
			)

			return true, &workflowexecutionv1alpha1.SkipDetails{
				Reason:    workflowexecutionv1alpha1.SkipReasonRecentlyRemediated,
				Message:   fmt.Sprintf("Cooldown active: workflow '%s' completed recently for resource '%s'. Remaining: %v", recentWFE.Name, wfe.Spec.TargetResource, remainingCooldown.Round(time.Second)),
				SkippedAt: metav1.Now(),
				RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
					Name:              recentWFE.Name,
					WorkflowID:        recentWFE.Spec.WorkflowRef.WorkflowID,
					CompletedAt:       *recentWFE.Status.CompletionTime,
					Outcome:           string(recentWFE.Status.Phase),
					TargetResource:    recentWFE.Spec.TargetResource,
					CooldownRemaining: remainingCooldown.Round(time.Second).String(),
				},
			}, nil
		}
	}

	return false, nil, nil
}

// ========================================
// FindMostRecentTerminalWFE finds the most recent Completed/Failed WFE for same target
// DD-WE-004: Used for exponential backoff state lookup
// Returns nil if no terminal WFE found for the target
// ========================================
func (r *WorkflowExecutionReconciler) FindMostRecentTerminalWFE(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
) *workflowexecutionv1alpha1.WorkflowExecution {
	logger := log.FromContext(ctx)

	// List all WFEs targeting the same resource
	var wfeList workflowexecutionv1alpha1.WorkflowExecutionList
	if err := r.List(ctx, &wfeList, client.MatchingFields{
		"spec.targetResource": wfe.Spec.TargetResource,
	}); err != nil {
		// If index not found, fall back to full list and filter
		if err := r.List(ctx, &wfeList); err != nil {
			logger.Error(err, "Failed to list WorkflowExecutions")
			return nil
		}
	}

	var mostRecent *workflowexecutionv1alpha1.WorkflowExecution
	for i := range wfeList.Items {
		existing := &wfeList.Items[i]

		// Skip self
		if existing.UID == wfe.UID {
			continue
		}

		// Skip different targetResource (in case index wasn't available)
		if existing.Spec.TargetResource != wfe.Spec.TargetResource {
			continue
		}

		// Only consider terminal phases
		if existing.Status.Phase != workflowexecutionv1alpha1.PhaseCompleted &&
			existing.Status.Phase != workflowexecutionv1alpha1.PhaseFailed {
			continue
		}

		// Must have completion time
		if existing.Status.CompletionTime == nil {
			continue
		}

		// Find most recent
		if mostRecent == nil ||
			existing.Status.CompletionTime.After(mostRecent.Status.CompletionTime.Time) {
			mostRecent = existing
		}
	}

	return mostRecent
}

// ========================================
// HandleAlreadyExists handles the race condition where PipelineRun already exists
// DD-WE-003: Layer 2 - Deterministic naming catches race conditions
// Returns: skipDetails if should be skipped, nil if PipelineRun is ours
// ========================================
func (r *WorkflowExecutionReconciler) HandleAlreadyExists(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, err error) (*workflowexecutionv1alpha1.SkipDetails, error) {
	logger := log.FromContext(ctx)

	if !apierrors.IsAlreadyExists(err) {
		return nil, err
	}

	// PipelineRun already exists - check if it's ours
	prName := PipelineRunName(wfe.Spec.TargetResource)
	existingPR := &tektonv1.PipelineRun{}
	if getErr := r.Get(ctx, client.ObjectKey{
		Name:      prName,
		Namespace: r.ExecutionNamespace,
	}, existingPR); getErr != nil {
		logger.Error(getErr, "Failed to get existing PipelineRun", "name", prName)
		return nil, getErr
	}

	// Check if the existing PipelineRun was created by this WFE
	if existingPR.Labels != nil &&
		existingPR.Labels["kubernaut.ai/workflow-execution"] == wfe.Name &&
		existingPR.Labels["kubernaut.ai/source-namespace"] == wfe.Namespace {
		// It's ours - we must have lost a race with ourselves (unlikely but safe)
		logger.Info("PipelineRun already exists and is ours", "name", prName)
		return nil, nil
	}

	// Another WFE created this PipelineRun - we lost the race
	logger.Info("Race condition caught: PipelineRun created by another WFE",
		"prName", prName,
		"existingWFE", existingPR.Labels["kubernaut.ai/workflow-execution"],
	)

	return &workflowexecutionv1alpha1.SkipDetails{
		Reason:    workflowexecutionv1alpha1.SkipReasonResourceBusy,
		Message:   fmt.Sprintf("Race condition: PipelineRun '%s' already exists for target resource", prName),
		SkippedAt: metav1.Now(),
		ConflictingWorkflow: &workflowexecutionv1alpha1.ConflictingWorkflowRef{
			Name:           existingPR.Labels["kubernaut.ai/workflow-execution"],
			WorkflowID:     "", // Not available from PipelineRun
			StartedAt:      existingPR.CreationTimestamp,
			TargetResource: wfe.Spec.TargetResource,
		},
	}, nil
}

// ========================================
// BuildPipelineRun creates a PipelineRun with bundle resolver
// DD-WE-002: PipelineRuns created in dedicated execution namespace
// DD-WE-003: Deterministic name for atomic locking
// ========================================
func (r *WorkflowExecutionReconciler) BuildPipelineRun(wfe *workflowexecutionv1alpha1.WorkflowExecution) *tektonv1.PipelineRun {
	// Convert parameters to Tekton format
	params := r.ConvertParameters(wfe.Spec.Parameters)

	// Add TARGET_RESOURCE parameter (required by all pipelines)
	params = append(params, tektonv1.Param{
		Name:  "TARGET_RESOURCE",
		Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.TargetResource},
	})

	// Get service account name (use default if not set)
	saName := r.ServiceAccountName
	if saName == "" {
		saName = DefaultServiceAccountName
	}

	return &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			// CRITICAL: Deterministic name = atomic lock (DD-WE-003)
			Name:      PipelineRunName(wfe.Spec.TargetResource),
			Namespace: r.ExecutionNamespace, // Always "kubernaut-workflows" (DD-WE-002)
			Labels: map[string]string{
				"kubernaut.ai/workflow-execution": wfe.Name,
				"kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
				// Sanitize for label (slashes not allowed, replace with __)
				"kubernaut.ai/target-resource": sanitizeLabelValue(wfe.Spec.TargetResource),
				// Source tracking for cross-namespace lookup
				"kubernaut.ai/source-namespace": wfe.Namespace,
			},
			Annotations: map[string]string{
				// Store original target resource value (with slashes) in annotation
				"kubernaut.ai/target-resource": wfe.Spec.TargetResource,
			},
			// NOTE: No OwnerReference - cross-namespace not supported
			// Cleanup handled via finalizer in ReconcileDelete()
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineRef: &tektonv1.PipelineRef{
				ResolverRef: tektonv1.ResolverRef{
					Resolver: "bundles",
					Params: []tektonv1.Param{
						{Name: "bundle", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.WorkflowRef.ContainerImage}},
						{Name: "name", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "workflow"}},
						{Name: "kind", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "pipeline"}},
					},
				},
			},
			Params: params,
			TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
				ServiceAccountName: saName,
			},
		},
	}
}

// ========================================
// ConvertParameters converts map[string]string to Tekton params
// ========================================
func (r *WorkflowExecutionReconciler) ConvertParameters(params map[string]string) []tektonv1.Param {
	if len(params) == 0 {
		return []tektonv1.Param{}
	}

	tektonParams := make([]tektonv1.Param, 0, len(params))
	for key, value := range params {
		tektonParams = append(tektonParams, tektonv1.Param{
			Name:  key,
			Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: value},
		})
	}
	return tektonParams
}

// ========================================
// FindWFEForPipelineRun maps PipelineRun events to WorkflowExecution reconcile requests
// Used for cross-namespace watch
// ========================================
func (r *WorkflowExecutionReconciler) FindWFEForPipelineRun(ctx context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()
	if labels == nil {
		return nil
	}

	wfeName := labels["kubernaut.ai/workflow-execution"]
	sourceNS := labels["kubernaut.ai/source-namespace"]

	if wfeName == "" || sourceNS == "" {
		return nil
	}

	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      wfeName,
			Namespace: sourceNS,
		},
	}}
}

// ========================================
// MarkSkipped updates WFE to Skipped phase with details
// Records metrics per BR-WE-008 (Day 7)
// ========================================
func (r *WorkflowExecutionReconciler) MarkSkipped(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, details *workflowexecutionv1alpha1.SkipDetails) error {
	logger := log.FromContext(ctx)
	logger.Info("Marking WorkflowExecution as Skipped",
		"reason", details.Reason,
		"message", details.Message,
	)

	wfe.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
	wfe.Status.SkipDetails = details
	now := metav1.Now()
	wfe.Status.CompletionTime = &now

	if err := r.Status().Update(ctx, wfe); err != nil {
		logger.Error(err, "Failed to update status to Skipped")
		return err
	}

	// Day 7: Record skip metric (BR-WE-008)
	RecordWorkflowSkip(details.Reason)

	// Day 6 Extension: Record backoff-specific metrics (BR-WE-012)
	if details.Reason == workflowexecutionv1alpha1.SkipReasonExhaustedRetries ||
		details.Reason == workflowexecutionv1alpha1.SkipReasonPreviousExecutionFailed {
		RecordBackoffSkip(details.Reason)
	}

	// Emit event
	r.Recorder.Event(wfe, "Normal", "Skipped", details.Message)

	return nil
}

// ========================================
// Day 5: Status Synchronization Methods
// ========================================

// BuildPipelineRunStatusSummary creates a lightweight status summary from PipelineRun
// Provides visibility into task progress during execution (v3.2)
func (r *WorkflowExecutionReconciler) BuildPipelineRunStatusSummary(pr *tektonv1.PipelineRun) *workflowexecutionv1alpha1.PipelineRunStatusSummary {
	summary := &workflowexecutionv1alpha1.PipelineRunStatusSummary{
		Status: "Unknown",
	}

	// Extract task counts from ChildReferences
	summary.TotalTasks = len(pr.Status.ChildReferences)

	// Get Succeeded condition
	succeededCond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if succeededCond != nil {
		summary.Status = string(succeededCond.Status)
		summary.Reason = succeededCond.Reason
		summary.Message = succeededCond.Message
	}

	return summary
}

// MarkCompleted transitions WFE to Completed phase
// Calculates Duration from StartTime to CompletionTime (v3.2)
// Day 6 Extension (BR-WE-012): Resets ConsecutiveFailures counter
// Records metrics per BR-WE-008 (Day 7)
func (r *WorkflowExecutionReconciler) MarkCompleted(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, pr *tektonv1.PipelineRun) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Marking WorkflowExecution as Completed")

	// Set phase
	wfe.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted

	// Set completion time from PipelineRun or now
	now := metav1.Now()
	if pr != nil && pr.Status.CompletionTime != nil {
		wfe.Status.CompletionTime = pr.Status.CompletionTime
	} else {
		wfe.Status.CompletionTime = &now
	}

	// Calculate duration (v3.2)
	var durationSeconds float64
	if wfe.Status.StartTime != nil && wfe.Status.CompletionTime != nil {
		duration := wfe.Status.CompletionTime.Sub(wfe.Status.StartTime.Time)
		wfe.Status.Duration = duration.Round(time.Second).String()
		durationSeconds = duration.Seconds()
	}

	// ========================================
	// Day 6 Extension (BR-WE-012): Reset failure counter on success
	// DD-WE-004-5: Success clears all backoff state
	// ========================================
	wfe.Status.ConsecutiveFailures = 0
	wfe.Status.NextAllowedExecution = nil
	logger.V(1).Info("Reset ConsecutiveFailures on successful completion")

	// Update status
	if err := r.Status().Update(ctx, wfe); err != nil {
		logger.Error(err, "Failed to update status to Completed")
		return ctrl.Result{}, err
	}

	// Day 7: Record metrics (BR-WE-008)
	RecordWorkflowCompletion(durationSeconds)

	// Day 6 Extension: Reset consecutive failures gauge (BR-WE-012)
	ResetConsecutiveFailures(wfe.Spec.TargetResource)

	// Emit event
	r.Recorder.Event(wfe, "Normal", "WorkflowCompleted",
		fmt.Sprintf("Workflow %s completed successfully in %s", wfe.Spec.WorkflowRef.WorkflowID, wfe.Status.Duration))

	return ctrl.Result{}, nil
}

// MarkFailed transitions WFE to Failed phase with FailureDetails
// Extracts failure information from PipelineRun (v3.2)
// Day 6 Extension (BR-WE-012): Handles exponential backoff for pre-execution failures
// Records metrics per BR-WE-008 (Day 7)
func (r *WorkflowExecutionReconciler) MarkFailed(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, pr *tektonv1.PipelineRun) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Marking WorkflowExecution as Failed")

	// Set phase
	wfe.Status.Phase = workflowexecutionv1alpha1.PhaseFailed

	// Set completion time
	now := metav1.Now()
	wfe.Status.CompletionTime = &now

	// Calculate duration (v3.2)
	var durationSeconds float64
	if wfe.Status.StartTime != nil {
		duration := now.Sub(wfe.Status.StartTime.Time)
		wfe.Status.Duration = duration.Round(time.Second).String()
		durationSeconds = duration.Seconds()
	}

	// Extract failure details (Day 7: includes TaskRun-specific fields, Day 6 Extension: WasExecutionFailure)
	wfe.Status.FailureDetails = r.ExtractFailureDetails(ctx, pr, wfe.Status.StartTime)

	// Generate natural language summary
	if wfe.Status.FailureDetails != nil {
		wfe.Status.FailureDetails.NaturalLanguageSummary = r.GenerateNaturalLanguageSummary(wfe, wfe.Status.FailureDetails)
	}

	// ========================================
	// Day 6 Extension (BR-WE-012): Exponential Backoff
	// DD-WE-004: Track consecutive failures for pre-execution failures ONLY
	// ========================================
	if wfe.Status.FailureDetails != nil && !wfe.Status.FailureDetails.WasExecutionFailure {
		// Pre-execution failure: increment counter and calculate backoff
		wfe.Status.ConsecutiveFailures++

		// Calculate exponential backoff: Base * 2^(min(failures-1, maxExponent))
		if r.BaseCooldownPeriod > 0 {
			exponent := int(wfe.Status.ConsecutiveFailures) - 1
			if r.MaxBackoffExponent > 0 && exponent > r.MaxBackoffExponent {
				exponent = r.MaxBackoffExponent
			}
			if exponent < 0 {
				exponent = 0
			}

			backoff := r.BaseCooldownPeriod * time.Duration(1<<exponent)
			if r.MaxCooldownPeriod > 0 && backoff > r.MaxCooldownPeriod {
				backoff = r.MaxCooldownPeriod
			}

			nextAllowed := metav1.NewTime(time.Now().Add(backoff))
			wfe.Status.NextAllowedExecution = &nextAllowed

			logger.Info("Calculated exponential backoff",
				"consecutiveFailures", wfe.Status.ConsecutiveFailures,
				"backoff", backoff,
				"nextAllowedExecution", nextAllowed.Time,
			)
		}
	} else {
		// Execution failure: DO NOT increment counter or set backoff
		// The PreviousExecutionFailed check in CheckCooldown will block ALL retries
		logger.Info("Execution failure detected - not incrementing ConsecutiveFailures",
			"wasExecutionFailure", wfe.Status.FailureDetails != nil && wfe.Status.FailureDetails.WasExecutionFailure,
		)
	}

	// Update status
	if err := r.Status().Update(ctx, wfe); err != nil {
		logger.Error(err, "Failed to update status to Failed")
		return ctrl.Result{}, err
	}

	// Day 7: Record metrics (BR-WE-008)
	RecordWorkflowFailure(durationSeconds)

	// Day 6 Extension: Update consecutive failures gauge (BR-WE-012)
	SetConsecutiveFailures(wfe.Spec.TargetResource, wfe.Status.ConsecutiveFailures)

	// Emit event
	reason := "Unknown"
	if wfe.Status.FailureDetails != nil {
		reason = wfe.Status.FailureDetails.Reason
	}
	r.Recorder.Event(wfe, "Warning", "WorkflowFailed",
		fmt.Sprintf("Workflow %s failed: %s", wfe.Spec.WorkflowRef.WorkflowID, reason))

	return ctrl.Result{}, nil
}

// ========================================
// Day 7: TaskRun-Specific Failure Details
// Plan v3.4: Extract FailedTaskName, FailedTaskIndex, ExitCode
// ========================================

// FindFailedTaskRun finds the first failed TaskRun in a PipelineRun's ChildReferences
// Returns the TaskRun, its index in ChildReferences, and any error
// Returns (nil, -1, nil) if no failed TaskRun is found
func (r *WorkflowExecutionReconciler) FindFailedTaskRun(ctx context.Context, pr *tektonv1.PipelineRun) (*tektonv1.TaskRun, int, error) {
	logger := log.FromContext(ctx)

	for i, ref := range pr.Status.ChildReferences {
		// Skip non-TaskRun references (e.g., Run, CustomRun)
		if ref.Kind != "TaskRun" {
			continue
		}

		// Fetch the TaskRun
		var tr tektonv1.TaskRun
		if err := r.Get(ctx, client.ObjectKey{
			Name:      ref.Name,
			Namespace: pr.Namespace,
		}, &tr); err != nil {
			if apierrors.IsNotFound(err) {
				// TaskRun may have been garbage collected - skip
				logger.V(1).Info("TaskRun not found, may be deleted", "taskRun", ref.Name)
				continue
			}
			// Other errors - log and continue
			logger.Error(err, "Failed to get TaskRun", "taskRun", ref.Name)
			continue
		}

		// Check if this TaskRun failed
		cond := tr.Status.GetCondition(apis.ConditionSucceeded)
		if cond != nil && cond.IsFalse() {
			logger.V(1).Info("Found failed TaskRun",
				"taskRun", tr.Name,
				"index", i,
				"reason", cond.Reason,
			)
			return &tr, i, nil
		}
	}

	// No failed TaskRun found
	return nil, -1, nil
}

// ExtractFailureDetails extracts structured failure information from PipelineRun
// Day 7: Now includes TaskRun-specific fields (FailedTaskName, FailedTaskIndex, ExitCode)
// Day 6 Extension (BR-WE-012): Includes WasExecutionFailure for backoff decisions
// Maps Tekton failure reasons to our FailureReason enum
func (r *WorkflowExecutionReconciler) ExtractFailureDetails(ctx context.Context, pr *tektonv1.PipelineRun, startTime *metav1.Time) *workflowexecutionv1alpha1.FailureDetails {
	details := &workflowexecutionv1alpha1.FailureDetails{
		FailedAt:            metav1.Now(),
		Reason:              workflowexecutionv1alpha1.FailureReasonUnknown,
		WasExecutionFailure: false, // Default: pre-execution failure
	}

	// Calculate execution time before failure
	if startTime != nil {
		duration := time.Since(startTime.Time)
		details.ExecutionTimeBeforeFailure = duration.Round(time.Second).String()
	}

	// Handle nil PipelineRun (deleted externally)
	if pr == nil {
		details.Message = "PipelineRun was deleted externally"
		return details
	}

	// Get failed condition from PipelineRun
	succeededCond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if succeededCond != nil {
		details.Message = succeededCond.Message

		// Map Tekton reasons to our enum
		details.Reason = r.mapTektonReasonToFailureReason(succeededCond.Reason, succeededCond.Message)
	}

	// ========================================
	// Day 7: Extract TaskRun-specific fields
	// ========================================
	failedTaskRun, index, err := r.FindFailedTaskRun(ctx, pr)
	if err == nil && failedTaskRun != nil {
		details.FailedTaskName = failedTaskRun.Name
		details.FailedTaskIndex = index

		// Extract exit code from container state (if available)
		details.ExitCode = r.extractExitCode(failedTaskRun)
	}

	// ========================================
	// Day 6 Extension (BR-WE-012): Determine WasExecutionFailure
	// DD-WE-004: Critical for backoff decisions
	//
	// WasExecutionFailure = true if:
	//   - PipelineRun has StartTime (execution began)
	//   - OR PipelineRun has any TaskRun (tasks were created)
	//   - OR failure reason indicates execution started (TaskFailed, OOMKilled, etc.)
	//
	// WasExecutionFailure = false if:
	//   - Failure is clearly pre-execution (ImagePullBackOff, ConfigurationError)
	//   - PipelineRun never started
	// ========================================
	details.WasExecutionFailure = r.determineWasExecutionFailure(pr, details.Reason)

	return details
}

// determineWasExecutionFailure checks if failure occurred during execution or before
// DD-WE-004: Critical for determining retry behavior
func (r *WorkflowExecutionReconciler) determineWasExecutionFailure(pr *tektonv1.PipelineRun, failureReason string) bool {
	if pr == nil {
		return false // Can't determine, assume pre-execution
	}

	// If PipelineRun has StartTime, execution began
	if pr.Status.StartTime != nil {
		// But check for pre-execution failure reasons even after start
		switch failureReason {
		case workflowexecutionv1alpha1.FailureReasonImagePullBackOff,
			workflowexecutionv1alpha1.FailureReasonConfigurationError,
			workflowexecutionv1alpha1.FailureReasonResourceExhausted:
			// These are typically pre-execution even if PipelineRun started
			return false
		default:
			// PipelineRun started and failed with a non-pre-execution reason
			return true
		}
	}

	// If there are TaskRuns in ChildReferences, tasks were created
	if len(pr.Status.ChildReferences) > 0 {
		return true
	}

	// Check for specific execution failure reasons
	switch failureReason {
	case workflowexecutionv1alpha1.FailureReasonOOMKilled,
		workflowexecutionv1alpha1.FailureReasonDeadlineExceeded,
		workflowexecutionv1alpha1.FailureReasonForbidden,
		workflowexecutionv1alpha1.FailureReasonTaskFailed:
		// These indicate execution started
		return true
	}

	// Default: assume pre-execution failure
	return false
}

// extractExitCode extracts the exit code from a failed TaskRun's step states
func (r *WorkflowExecutionReconciler) extractExitCode(tr *tektonv1.TaskRun) *int32 {
	if tr == nil {
		return nil
	}

	// Check step states for terminated containers with exit codes
	for _, step := range tr.Status.Steps {
		if step.Terminated != nil && step.Terminated.ExitCode != 0 {
			exitCode := step.Terminated.ExitCode
			return &exitCode
		}
	}

	return nil
}

// mapTektonReasonToFailureReason converts Tekton/K8s reasons to our FailureReason enum
func (r *WorkflowExecutionReconciler) mapTektonReasonToFailureReason(reason, message string) string {
	messageLower := strings.ToLower(message)
	reasonLower := strings.ToLower(reason)

	switch {
	case strings.Contains(messageLower, "oomkilled") || strings.Contains(messageLower, "oom"):
		return workflowexecutionv1alpha1.FailureReasonOOMKilled
	case strings.Contains(reasonLower, "timeout") || strings.Contains(messageLower, "timeout") ||
		strings.Contains(messageLower, "deadline"):
		return workflowexecutionv1alpha1.FailureReasonDeadlineExceeded
	case strings.Contains(messageLower, "forbidden") || strings.Contains(messageLower, "rbac") ||
		strings.Contains(messageLower, "permission denied"):
		return workflowexecutionv1alpha1.FailureReasonForbidden
	case strings.Contains(messageLower, "quota") || strings.Contains(messageLower, "resource exhausted"):
		return workflowexecutionv1alpha1.FailureReasonResourceExhausted
	case strings.Contains(messageLower, "imagepullbackoff") || strings.Contains(messageLower, "image pull"):
		return workflowexecutionv1alpha1.FailureReasonImagePullBackOff
	case strings.Contains(messageLower, "invalid") || strings.Contains(messageLower, "configuration"):
		return workflowexecutionv1alpha1.FailureReasonConfigurationError
	default:
		return workflowexecutionv1alpha1.FailureReasonUnknown
	}
}

// GenerateNaturalLanguageSummary creates a human/LLM-readable failure description
// For recovery context and user notifications
// Day 9 (v3.5): Handles nil FailureDetails gracefully per Q4 decision
func (r *WorkflowExecutionReconciler) GenerateNaturalLanguageSummary(wfe *workflowexecutionv1alpha1.WorkflowExecution, details *workflowexecutionv1alpha1.FailureDetails) string {
	var sb strings.Builder

	// Workflow identification
	sb.WriteString(fmt.Sprintf("Workflow '%s' failed on target '%s'.\n",
		wfe.Spec.WorkflowRef.WorkflowID,
		wfe.Spec.TargetResource))

	// Handle nil FailureDetails gracefully (Day 9 edge case)
	if details == nil {
		sb.WriteString("Reason: Unknown - No failure details available.\n")
		sb.WriteString("Recommendation: Check PipelineRun logs for detailed failure information.\n")
		return sb.String()
	}

	// Failure reason
	sb.WriteString(fmt.Sprintf("Reason: %s\n", details.Reason))

	// Error message
	if details.Message != "" {
		sb.WriteString(fmt.Sprintf("Error: %s\n", details.Message))
	}

	// Execution time
	if details.ExecutionTimeBeforeFailure != "" {
		sb.WriteString(fmt.Sprintf("Failed after: %s\n", details.ExecutionTimeBeforeFailure))
	}

	// Reason-specific recommendations
	switch details.Reason {
	case workflowexecutionv1alpha1.FailureReasonOOMKilled:
		sb.WriteString("Recommendation: The workflow task ran out of memory. Consider increasing task resource limits.\n")
	case workflowexecutionv1alpha1.FailureReasonForbidden:
		sb.WriteString("Recommendation: The service account lacks required RBAC permissions. Grant appropriate permissions or use an alternative workflow.\n")
	case workflowexecutionv1alpha1.FailureReasonDeadlineExceeded:
		sb.WriteString("Recommendation: The workflow exceeded its timeout. Consider increasing the timeout or using a faster workflow variant.\n")
	case workflowexecutionv1alpha1.FailureReasonImagePullBackOff:
		sb.WriteString("Recommendation: Unable to pull the workflow container image. Verify image exists and credentials are configured.\n")
	}

	return sb.String()
}

// ========================================
// Day 8: Audit Trail (BR-WE-005)
// Per ADR-032: All services use Data Storage Service via pkg/audit
// ========================================

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

	// Graceful degradation: skip audit if store not configured
	if r.AuditStore == nil {
		logger.V(1).Info("AuditStore not configured, skipping audit event",
			"action", action,
			"wfe", wfe.Name,
		)
		return nil
	}

	// Build audit event per ADR-034 schema
	event := audit.NewAuditEvent()
	event.EventType = "workflowexecution." + action
	event.EventCategory = "workflow"
	event.EventAction = action
	event.EventOutcome = outcome
	event.ActorType = "service"
	event.ActorID = "workflowexecution-controller"
	event.ResourceType = "WorkflowExecution"
	event.ResourceID = wfe.Name

	// Correlation ID from labels (set by RemediationOrchestrator)
	if wfe.Labels != nil {
		if corrID, ok := wfe.Labels["kubernaut.ai/correlation-id"]; ok {
			event.CorrelationID = corrID
		}
	}
	if event.CorrelationID == "" {
		// Fallback: use WFE name as correlation ID
		event.CorrelationID = wfe.Name
	}

	// Set namespace context
	ns := wfe.Namespace
	event.Namespace = &ns

	// Build event data per database-integration.md schema
	eventData := map[string]interface{}{
		"workflow_id":     wfe.Spec.WorkflowRef.WorkflowID,
		"target_resource": wfe.Spec.TargetResource,
		"phase":           string(wfe.Status.Phase),
		"container_image": wfe.Spec.WorkflowRef.ContainerImage,
		"execution_name":  wfe.Name,
	}

	// Add timing info if available
	if wfe.Status.StartTime != nil {
		eventData["started_at"] = wfe.Status.StartTime.Time
	}
	if wfe.Status.CompletionTime != nil {
		eventData["completed_at"] = wfe.Status.CompletionTime.Time
	}
	if wfe.Status.Duration != "" {
		eventData["duration"] = wfe.Status.Duration
	}

	// Add failure details if present
	if wfe.Status.FailureDetails != nil {
		eventData["failure_reason"] = wfe.Status.FailureDetails.Reason
		eventData["failure_message"] = wfe.Status.FailureDetails.Message
		if wfe.Status.FailureDetails.FailedTaskName != "" {
			eventData["failed_task_name"] = wfe.Status.FailureDetails.FailedTaskName
		}
	}

	// Add skip details if present
	if wfe.Status.SkipDetails != nil {
		eventData["skip_reason"] = wfe.Status.SkipDetails.Reason
		eventData["skip_message"] = wfe.Status.SkipDetails.Message
	}

	// Add PipelineRun reference if present
	if wfe.Status.PipelineRunRef != nil {
		eventData["pipelinerun_name"] = wfe.Status.PipelineRunRef.Name
	}

	// Marshal event data
	eventDataBytes, err := marshalJSON(eventData)
	if err != nil {
		logger.Error(err, "Failed to marshal audit event data")
		return nil // Don't fail business logic on audit error
	}
	event.EventData = eventDataBytes

	// Store audit event (non-blocking per DD-AUDIT-002)
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store audit event",
			"action", action,
			"wfe", wfe.Name,
		)
		// Don't fail business logic on audit error (graceful degradation)
		return nil
	}

	logger.V(1).Info("Audit event recorded",
		"action", action,
		"wfe", wfe.Name,
		"outcome", outcome,
	)
	return nil
}

// ========================================
// Day 8: Spec Validation
// Per controller-implementation.md
// ========================================

// ValidateSpec validates the WorkflowExecution spec
// Returns error if validation fails (ConfigurationError reason)
func (r *WorkflowExecutionReconciler) ValidateSpec(wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	// Validate container image is required
	if wfe.Spec.WorkflowRef.ContainerImage == "" {
		return fmt.Errorf("workflowRef.containerImage is required")
	}

	// Validate target resource is required
	if wfe.Spec.TargetResource == "" {
		return fmt.Errorf("targetResource is required")
	}

	// Validate targetResource format: {namespace}/{kind}/{name}
	parts := strings.Split(wfe.Spec.TargetResource, "/")
	if len(parts) != 3 {
		return fmt.Errorf("targetResource must be in format {namespace}/{kind}/{name}, got %d parts", len(parts))
	}

	// Validate each part is non-empty
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("targetResource has empty part at position %d", i)
		}
	}

	return nil
}

// marshalJSON marshals data to JSON bytes
func marshalJSON(data interface{}) ([]byte, error) {
	return jsonMarshal(data)
}

// jsonMarshal is a variable to allow mocking in tests
var jsonMarshal = func(v interface{}) ([]byte, error) {
	// Use encoding/json
	return jsonEncode(v)
}

// jsonEncode uses encoding/json
func jsonEncode(v interface{}) ([]byte, error) {
	return encJSON.Marshal(v)
}
