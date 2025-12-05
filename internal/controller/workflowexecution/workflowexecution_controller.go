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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
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
	FinalizerName = "workflowexecution.kubernaut.ai/finalizer"

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
}

// ========================================
// RBAC Markers
// ========================================

//+kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;list;watch;create;delete
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
		return r.reconcileDelete(ctx, &wfe)
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
		return r.reconcileTerminal(ctx, &wfe)
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

	// TODO (Day 5): Fetch PipelineRun and sync status

	// For now, just requeue
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// ========================================
// reconcileTerminal - Handle Completed/Failed phases
// Day 6: Cooldown enforcement and cleanup
// ========================================
func (r *WorkflowExecutionReconciler) reconcileTerminal(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Terminal phase", "phase", wfe.Status.Phase)

	// TODO (Day 6): Enforce cooldown before releasing lock

	// For now, terminal phases are complete
	return ctrl.Result{}, nil
}

// ========================================
// reconcileDelete - Handle deletion with finalizer
// ========================================
func (r *WorkflowExecutionReconciler) reconcileDelete(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Delete")

	// Check if finalizer is present
	if !controllerutil.ContainsFinalizer(wfe, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// ========================================
	// Cleanup: Delete PipelineRun if exists
	// ========================================
	if wfe.Status.PipelineRunRef != nil {
		logger.Info("Deleting associated PipelineRun",
			"pipelineRun", wfe.Status.PipelineRunRef.Name,
			"namespace", r.ExecutionNamespace,
		)

		pr := &tektonv1.PipelineRun{}
		pr.Name = wfe.Status.PipelineRunRef.Name
		pr.Namespace = r.ExecutionNamespace

		if err := r.Delete(ctx, pr); err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to delete PipelineRun")
				return ctrl.Result{}, err
			}
			// PipelineRun already deleted - continue
		}
	}

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
// CheckCooldown checks if a recent WFE completed within cooldown period
// DD-WE-001: Resource Locking Safety (Cooldown Check)
// Returns: blocked, skipDetails, error
// ========================================
func (r *WorkflowExecutionReconciler) CheckCooldown(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (bool, *workflowexecutionv1alpha1.SkipDetails, error) {
	logger := log.FromContext(ctx)

	// Cooldown disabled
	if r.CooldownPeriod <= 0 {
		return false, nil, nil
	}

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

	now := time.Now()
	cooldownThreshold := now.Add(-r.CooldownPeriod)

	// Check if any terminal WFE completed within cooldown period (excluding self)
	for _, existing := range wfeList.Items {
		// Skip self
		if existing.UID == wfe.UID {
			continue
		}

		// Skip different targetResource (in case index wasn't available)
		if existing.Spec.TargetResource != wfe.Spec.TargetResource {
			continue
		}

		// Check if terminal phase with recent completion
		if existing.Status.Phase == workflowexecutionv1alpha1.PhaseCompleted ||
			existing.Status.Phase == workflowexecutionv1alpha1.PhaseFailed {

			if existing.Status.CompletionTime != nil &&
				existing.Status.CompletionTime.Time.After(cooldownThreshold) {

				remainingCooldown := existing.Status.CompletionTime.Time.Add(r.CooldownPeriod).Sub(now)
				logger.Info("Cooldown active",
					"blockedBy", existing.Name,
					"targetResource", wfe.Spec.TargetResource,
					"remainingCooldown", remainingCooldown,
				)

				return true, &workflowexecutionv1alpha1.SkipDetails{
					Reason:    workflowexecutionv1alpha1.SkipReasonRecentlyRemediated,
					Message:   fmt.Sprintf("Cooldown active: workflow '%s' completed recently for resource '%s'. Remaining: %v", existing.Name, wfe.Spec.TargetResource, remainingCooldown.Round(time.Second)),
					SkippedAt: metav1.Now(),
					RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
						Name:              existing.Name,
						WorkflowID:        existing.Spec.WorkflowRef.WorkflowID,
						CompletedAt:       *existing.Status.CompletionTime,
						Outcome:           string(existing.Status.Phase),
						TargetResource:    existing.Spec.TargetResource,
						CooldownRemaining: remainingCooldown.Round(time.Second).String(),
					},
				}, nil
			}
		}
	}

	return false, nil, nil
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
				"kubernaut.ai/target-resource":    wfe.Spec.TargetResource,
				// Source tracking for cross-namespace lookup
				"kubernaut.ai/source-namespace": wfe.Namespace,
			},
			// NOTE: No OwnerReference - cross-namespace not supported
			// Cleanup handled via finalizer in reconcileDelete()
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

	// Emit event
	r.Recorder.Event(wfe, "Normal", "Skipped", details.Message)

	return nil
}

