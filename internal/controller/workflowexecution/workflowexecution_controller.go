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
	"fmt"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

// ============================================================================
// WorkflowExecution Controller
// Version: 1.0 - Tekton Delegation Architecture (ADR-044)
// ============================================================================
//
// Architecture Overview:
// - Creates Tekton PipelineRun from OCI bundle references
// - Watches PipelineRun status and updates WorkflowExecution status
// - Implements resource locking to prevent parallel workflows (DD-WE-001)
// - Uses deterministic PipelineRun naming for race condition prevention (DD-WE-003)
//
// Key Design Decisions:
// - ADR-044: Tekton handles step orchestration
// - DD-WE-001: Resource Locking - prevents parallel/redundant workflows
// - DD-WE-002: All PipelineRuns run in dedicated kubernaut-workflows namespace
// - DD-WE-003: Deterministic naming using SHA256 hash of targetResource
// ============================================================================

const (
	// workflowExecutionFinalizer is the finalizer used for cleanup
	workflowExecutionFinalizer = "workflowexecution.kubernaut.ai/finalizer"

	// DefaultCooldownPeriod is the default time to prevent redundant executions (5 minutes)
	// Used when cooldownPeriod flag is not specified
	DefaultCooldownPeriod = 5 * time.Minute

	// DefaultServiceAccountName is used if not specified in spec
	// Used when serviceAccountName flag is not specified
	DefaultServiceAccountName = "kubernaut-workflow-runner"

	// Labels for PipelineRun cross-namespace tracking
	labelWorkflowExecution = "kubernaut.ai/workflow-execution"
	labelSourceNamespace   = "kubernaut.ai/source-namespace"
	labelWorkflowID        = "kubernaut.ai/workflow-id"
	labelTargetResource    = "kubernaut.ai/target-resource"
)

// WorkflowExecutionReconciler reconciles a WorkflowExecution object
type WorkflowExecutionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// ExecutionNamespace is where PipelineRuns are created (DD-WE-002)
	ExecutionNamespace string

	// ServiceAccountName for PipelineRun execution
	ServiceAccountName string

	// CooldownPeriod prevents redundant sequential executions (DD-WE-001)
	CooldownPeriod time.Duration

	// AuditStore for unified audit trail (ADR-034)
	AuditStore audit.AuditStore
}

// +kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=create;get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is the main reconciliation loop for WorkflowExecution CRDs
// BR-WE-001: Create PipelineRun from OCI Bundle
// BR-WE-003: Monitor Execution Status
// BR-WE-009: Prevent Parallel Execution
// BR-WE-010: Cooldown Period
func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the WorkflowExecution CRD
	var wfe workflowexecutionv1alpha1.WorkflowExecution
	if err := r.Get(ctx, req.NamespacedName, &wfe); err != nil {
		if errors.IsNotFound(err) {
			// CRD was deleted, nothing to do
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch WorkflowExecution")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !wfe.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &wfe)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&wfe, workflowExecutionFinalizer) {
		controllerutil.AddFinalizer(&wfe, workflowExecutionFinalizer)
		if err := r.Update(ctx, &wfe); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		// Requeue to continue reconciliation
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile based on current phase
	switch wfe.Status.Phase {
	case "", workflowexecutionv1alpha1.PhasePending:
		return r.reconcilePending(ctx, &wfe)

	case workflowexecutionv1alpha1.PhaseRunning:
		return r.reconcileRunning(ctx, &wfe)

	case workflowexecutionv1alpha1.PhaseCompleted, workflowexecutionv1alpha1.PhaseFailed:
		return r.reconcileTerminal(ctx, &wfe)

	case workflowexecutionv1alpha1.PhaseSkipped:
		// Skipped executions don't need further processing
		return ctrl.Result{}, nil

	default:
		log.Error(nil, "Unknown phase", "phase", wfe.Status.Phase)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
}

// reconcilePending handles WorkflowExecutions in Pending phase
// Implements resource locking (DD-WE-001) and creates PipelineRun
func (r *WorkflowExecutionReconciler) reconcilePending(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// ========================================
	// Step 1: Validate spec
	// ========================================
	if err := r.validateSpec(wfe); err != nil {
		return r.markFailed(ctx, wfe, workflowexecutionv1alpha1.FailureReasonConfigurationError,
			fmt.Sprintf("Invalid spec: %v", err), false)
	}

	// ========================================
	// Step 2: Check resource lock (DD-WE-001)
	// Layer 1: Fast path - check for running WFEs on same target
	// ========================================
	blocked, blockingWFE, err := r.checkResourceLock(ctx, wfe)
	if err != nil {
		log.Error(err, "Failed to check resource lock")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	if blocked {
		return r.markSkipped(ctx, wfe, workflowexecutionv1alpha1.SkipReasonResourceBusy, blockingWFE)
	}

	// ========================================
	// Step 3: Check cooldown (DD-WE-001)
	// ========================================
	recentWFE, cooldownRemaining, err := r.checkCooldown(ctx, wfe)
	if err != nil {
		log.Error(err, "Failed to check cooldown")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	if recentWFE != nil {
		return r.markSkippedRecentlyRemediated(ctx, wfe, recentWFE, cooldownRemaining)
	}

	// ========================================
	// Step 4: Create PipelineRun (DD-WE-003)
	// Uses deterministic name for race condition prevention
	// ========================================
	pr := r.buildPipelineRun(wfe)

	if err := r.Create(ctx, pr); err != nil {
		if errors.IsAlreadyExists(err) {
			// Race condition caught! Another controller instance created the PR
			// This is expected behavior per DD-WE-003
			log.Info("PipelineRun already exists (race condition caught), checking if we should skip",
				"pipelineRun", pr.Name)

			// Check if the existing PR is for a different WFE
			existingPR := &tektonv1.PipelineRun{}
			if getErr := r.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: r.ExecutionNamespace}, existingPR); getErr == nil {
				existingWFEName := existingPR.Labels[labelWorkflowExecution]
				if existingWFEName != wfe.Name {
					// Different WFE created this PR, we should skip
					return r.markSkipped(ctx, wfe, workflowexecutionv1alpha1.SkipReasonResourceBusy, &workflowexecutionv1alpha1.ConflictingWorkflowRef{
						Name:           existingWFEName,
						WorkflowID:     existingPR.Labels[labelWorkflowID],
						TargetResource: existingPR.Labels[labelTargetResource],
						StartedAt:      existingPR.CreationTimestamp,
					})
				}
			}
			// Same WFE, PR exists - continue to Running phase
		} else {
			log.Error(err, "Failed to create PipelineRun")
			RecordPipelineRunCreation("failure")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}
	} else {
		RecordPipelineRunCreation("success")
		log.Info("Created PipelineRun", "pipelineRun", pr.Name, "namespace", r.ExecutionNamespace)
	}

	// ========================================
	// Step 5: Transition to Running
	// ========================================
	now := metav1.Now()
	wfe.Status.Phase = workflowexecutionv1alpha1.PhaseRunning
	wfe.Status.StartTime = &now
	wfe.Status.PipelineRunRef = &corev1.LocalObjectReference{Name: pr.Name}

	if err := r.Status().Update(ctx, wfe); err != nil {
		log.Error(err, "Failed to update status to Running")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(wfe, "Normal", "PipelineRunCreated",
		fmt.Sprintf("Created PipelineRun %s in namespace %s", pr.Name, r.ExecutionNamespace))

	// Record audit event (BR-WE-007)
	r.recordPipelineRunCreated(ctx, wfe, pr.Name)

	// Requeue to check PipelineRun status
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// reconcileRunning handles WorkflowExecutions in Running phase
// BR-WE-003: Monitor Execution Status
func (r *WorkflowExecutionReconciler) reconcileRunning(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get PipelineRun from execution namespace
	prName := pipelineRunName(wfe.Spec.TargetResource)
	var pr tektonv1.PipelineRun
	if err := r.Get(ctx, types.NamespacedName{Name: prName, Namespace: r.ExecutionNamespace}, &pr); err != nil {
		if errors.IsNotFound(err) {
			// BR-WE-007: PipelineRun was deleted externally
			log.Info("PipelineRun not found (externally deleted)", "pipelineRun", prName)
			return r.markFailed(ctx, wfe, workflowexecutionv1alpha1.FailureReasonUnknown,
				"PipelineRun was deleted externally", true)
		}
		log.Error(err, "Failed to get PipelineRun")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	// Map PipelineRun status to WFE status
	return r.syncPipelineRunStatus(ctx, wfe, &pr)
}

// reconcileTerminal handles WorkflowExecutions in Completed or Failed phase
// Implements cooldown period before releasing lock (DD-WE-001)
func (r *WorkflowExecutionReconciler) reconcileTerminal(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Check if cooldown period has elapsed
	if wfe.Status.CompletionTime == nil {
		// No completion time - this shouldn't happen, but handle gracefully
		now := metav1.Now()
		wfe.Status.CompletionTime = &now
		if err := r.Status().Update(ctx, wfe); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.CooldownPeriod}, nil
	}

	elapsed := time.Since(wfe.Status.CompletionTime.Time)
	if elapsed < r.CooldownPeriod {
		// Still in cooldown, requeue
		remaining := r.CooldownPeriod - elapsed
		log.V(1).Info("In cooldown period", "remaining", remaining.String())
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// Cooldown complete - delete PipelineRun to release lock
	if !wfe.Status.LockReleased {
		prName := pipelineRunName(wfe.Spec.TargetResource)
		pr := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prName,
				Namespace: r.ExecutionNamespace,
			},
		}

		if err := r.Delete(ctx, pr); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete PipelineRun", "pipelineRun", prName)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}

		log.Info("Deleted PipelineRun after cooldown", "pipelineRun", prName)

		// Mark lock as released
		wfe.Status.LockReleased = true
		if err := r.Status().Update(ctx, wfe); err != nil {
			return ctrl.Result{}, err
		}

		// Record audit event (BR-WE-007)
		r.recordLockReleased(ctx, wfe)
	}

	// No more reconciliation needed
	return ctrl.Result{}, nil
}

// reconcileDelete handles WorkflowExecution deletion
// BR-WE-008: Finalizer Cleanup
func (r *WorkflowExecutionReconciler) reconcileDelete(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(wfe, workflowExecutionFinalizer) {
		// Delete the PipelineRun if it exists
		prName := pipelineRunName(wfe.Spec.TargetResource)
		pr := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prName,
				Namespace: r.ExecutionNamespace,
			},
		}

		if err := r.Delete(ctx, pr); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete PipelineRun during finalization", "pipelineRun", prName)
			return ctrl.Result{}, err
		}

		log.Info("Deleted PipelineRun during finalization", "pipelineRun", prName)

		// Record audit event before removing finalizer (BR-WE-007)
		r.recordDeleted(ctx, wfe)

		// Remove finalizer
		controllerutil.RemoveFinalizer(wfe, workflowExecutionFinalizer)
		if err := r.Update(ctx, wfe); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
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
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&workflowexecutionv1alpha1.WorkflowExecution{}).
		// Note: PipelineRuns are in a different namespace, so we can't use Owns()
		// We rely on the deterministic naming and periodic reconciliation instead
		Complete(r)
}

// ============================================================================
// Helper Functions
// ============================================================================

// pipelineRunName generates a deterministic name for the PipelineRun
// based on the target resource (DD-WE-003)
func pipelineRunName(targetResource string) string {
	h := sha256.Sum256([]byte(targetResource))
	return fmt.Sprintf("wfe-%s", hex.EncodeToString(h[:])[:16])
}

// validateSpec validates the WorkflowExecution spec
func (r *WorkflowExecutionReconciler) validateSpec(wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	if wfe.Spec.TargetResource == "" {
		return fmt.Errorf("targetResource is required")
	}
	if wfe.Spec.WorkflowRef.WorkflowID == "" {
		return fmt.Errorf("workflowRef.workflowId is required")
	}
	if wfe.Spec.WorkflowRef.ContainerImage == "" {
		return fmt.Errorf("workflowRef.containerImage is required")
	}
	return nil
}

// checkResourceLock checks if another workflow is running on the same target
func (r *WorkflowExecutionReconciler) checkResourceLock(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (bool, *workflowexecutionv1alpha1.ConflictingWorkflowRef, error) {
	var wfeList workflowexecutionv1alpha1.WorkflowExecutionList
	if err := r.List(ctx, &wfeList, client.MatchingFields{"spec.targetResource": wfe.Spec.TargetResource}); err != nil {
		return false, nil, err
	}

	for _, existing := range wfeList.Items {
		// Skip self
		if existing.Name == wfe.Name && existing.Namespace == wfe.Namespace {
			continue
		}

		// Check if existing WFE is in Running or Pending phase
		if existing.Status.Phase == workflowexecutionv1alpha1.PhaseRunning ||
			existing.Status.Phase == workflowexecutionv1alpha1.PhasePending {
			return true, &workflowexecutionv1alpha1.ConflictingWorkflowRef{
				Name:           existing.Name,
				WorkflowID:     existing.Spec.WorkflowRef.WorkflowID,
				TargetResource: existing.Spec.TargetResource,
				StartedAt:      existing.CreationTimestamp,
			}, nil
		}
	}

	return false, nil, nil
}

// checkCooldown checks if a recent workflow ran on the same target
func (r *WorkflowExecutionReconciler) checkCooldown(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (*workflowexecutionv1alpha1.WorkflowExecution, time.Duration, error) {
	var wfeList workflowexecutionv1alpha1.WorkflowExecutionList
	if err := r.List(ctx, &wfeList, client.MatchingFields{"spec.targetResource": wfe.Spec.TargetResource}); err != nil {
		return nil, 0, err
	}

	for i := range wfeList.Items {
		existing := &wfeList.Items[i]
		// Skip self
		if existing.Name == wfe.Name && existing.Namespace == wfe.Namespace {
			continue
		}

		// Check if existing WFE is in terminal state with same workflow
		if (existing.Status.Phase == workflowexecutionv1alpha1.PhaseCompleted ||
			existing.Status.Phase == workflowexecutionv1alpha1.PhaseFailed) &&
			existing.Spec.WorkflowRef.WorkflowID == wfe.Spec.WorkflowRef.WorkflowID &&
			existing.Status.CompletionTime != nil {

			elapsed := time.Since(existing.Status.CompletionTime.Time)
			if elapsed < r.CooldownPeriod {
				remaining := r.CooldownPeriod - elapsed
				return existing, remaining, nil
			}
		}
	}

	return nil, 0, nil
}
