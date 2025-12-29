## Controller Implementation

**Version**: 4.4
**Last Updated**: 2025-12-06
**CRD API Group**: `kubernaut.ai/v1alpha1`
**Status**: ✅ Updated for Tekton Architecture (ADR-044), Exponential Backoff (DD-WE-004)

**Location**: `internal/controller/workflowexecution/workflowexecution_controller.go`

---

## Changelog

### Version 4.4 (2025-12-06)
- ✅ **Added**: Exponential backoff cooldown per DD-WE-004 v1.1 and BR-WE-012
- ✅ **CRITICAL**: Backoff ONLY applies to pre-execution failures (`wasExecutionFailure: false`)
- ✅ **CRITICAL**: Execution failures (`wasExecutionFailure: true`) block ALL retries with `PreviousExecutionFailed`
- ✅ **Added**: `BaseCooldownPeriod`, `MaxCooldownPeriod`, `MaxBackoffExponent`, `MaxConsecutiveFailures` reconciler config
- ✅ **Updated**: `CheckCooldown()` to first check `wasExecutionFailure` → block if true
- ✅ **Updated**: `MarkFailed()` to only apply backoff when `wasExecutionFailure: false`
- ✅ **Updated**: `MarkCompleted()` to reset `ConsecutiveFailures` and clear `NextAllowedExecution`
- ✅ **Added**: `ExhaustedRetries` and `PreviousExecutionFailed` skip reasons
- ✅ **Added**: `workflowexecution_backoff_skip_total` and `workflowexecution_consecutive_failures` metrics

### Version 4.3 (2025-12-05)
- ✅ **Updated**: Finalizer name to `kubernaut.ai/workflowexecution-cleanup` per finalizers-lifecycle.md
- ✅ **Updated**: `reconcileDelete()` to use deterministic PipelineRun name per DD-WE-003
- ✅ **Updated**: `reconcileTerminal()` implementation aligned with DD-WE-003
- ✅ **Added**: `WorkflowExecutionDeleted` event emission per finalizers-lifecycle.md
- ✅ **Added**: `LockReleased` event emission after cooldown expiry

### Version 4.2 (2025-12-05)
- ✅ **Added**: TaskRun RBAC for extracting failure details from failed tasks
- ✅ **Added**: PipelineRunStatusSummary population during Running phase for task progress visibility
- ✅ **Clarified**: Duration calculation on phase completion (already in crd-schema.md)
- ✅ **Added**: `reconcileRunning()` implementation with Tekton status mapping (Day 5)
- ✅ **Added**: `markCompleted()` and `markFailed()` with FailureDetails extraction

### Version 4.1 (2025-12-05)
- ✅ **Changed**: Cross-namespace watch uses predicate filter instead of namespace-scoped cache
- ✅ **Added**: `kind` parameter to bundle resolver params (required by Tekton)
- ✅ **Fixed**: ServiceAccountName moved to `TaskRunTemplate` (Tekton v1 API)
- ✅ **Added**: Additional labels (`workflow-id`, `target-resource`) for better tracking
- ✅ **Rationale**: Predicate filter is simpler and achieves same filtering goal

### Version 4.0 (2025-12-02)
- ✅ **Rewritten**: Complete rewrite for Tekton PipelineRun delegation
- ✅ **Removed**: All step orchestration logic (Tekton handles this)
- ✅ **Added**: Resource locking implementation (DD-WE-001)
- ✅ **Updated**: Simplified reconciliation phases

---

### Controller Configuration

**Critical Patterns**:
1. **Owner References**: WorkflowExecution CRD owned by RemediationRequest for cascade deletion
2. **Finalizers**: Cleanup coordination before deletion
3. **PipelineRun Watch**: Watch PipelineRuns via predicate filter on `kubernaut.ai/workflow-execution` label (v4.1)
4. **Resource Locking**: Check for active executions on same target (DD-WE-001)
5. **Event Emission**: Operational visibility through Kubernetes events
6. **Cross-Namespace Tracking**: Labels enable mapping PipelineRun events to WFE reconcile requests

```go
package controller

import (
    "context"
    "fmt"
    "time"

    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"

    tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

const (
    // Finalizer for cleanup coordination (v4.3: aligned with finalizers-lifecycle.md)
    FinalizerName = "kubernaut.ai/workflowexecution-cleanup"

    // Cooldown period for same workflow on same target
    DefaultCooldownPeriod = 5 * time.Minute

    // Status check interval for running PipelineRuns
    StatusCheckInterval = 10 * time.Second
)

// WorkflowExecutionReconciler reconciles a WorkflowExecution object
type WorkflowExecutionReconciler struct {
    client.Client
    Scheme             *runtime.Scheme
    Recorder           record.EventRecorder
    CooldownPeriod     time.Duration  // DEPRECATED: Use BaseCooldownPeriod (DD-WE-004)
    ExecutionNamespace string         // "kubernaut-workflows" (DD-WE-002)
    ServiceAccountName string         // "kubernaut-workflow-runner"

    // Exponential Backoff Configuration (DD-WE-004)
    BaseCooldownPeriod     time.Duration  // Default: 1 minute
    MaxCooldownPeriod      time.Duration  // Default: 10 minutes
    MaxBackoffExponent     int            // Default: 4
    MaxConsecutiveFailures int            // Default: 5
}

// ========================================
// ARCHITECTURAL NOTE: Tekton Delegation Pattern
// ========================================
//
// This controller DELEGATES workflow execution to Tekton.
// WorkflowExecution creates exactly one PipelineRun per execution.
// Tekton handles all step orchestration, parallelism, and dependencies.
//
// This controller:
// - Creates PipelineRun with bundle resolver
// - Syncs status from PipelineRun to WorkflowExecution
// - Enforces resource locking (DD-WE-001)
//
// See: ADR-044 (Tekton Delegation Architecture)
// See: DD-WE-001 (Resource Locking Safety)

//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=create;get;list;watch;delete
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=tekton.dev,resources=taskruns,verbs=get;list;watch
//+kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Fetch WorkflowExecution CRD
    var wfe workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &wfe); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion with finalizer
    if !wfe.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, &wfe)
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&wfe, workflowExecutionFinalizer) {
        controllerutil.AddFinalizer(&wfe, workflowExecutionFinalizer)
        if err := r.Update(ctx, &wfe); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Reconcile based on current phase
    switch wfe.Status.Phase {
    case "", workflowexecutionv1.PhasePending:
        return r.reconcilePending(ctx, &wfe)
    case workflowexecutionv1.PhaseRunning:
        return r.reconcileRunning(ctx, &wfe)
    case workflowexecutionv1.PhaseCompleted, workflowexecutionv1.PhaseFailed, workflowexecutionv1.PhaseSkipped:
        // Terminal states - no requeue
        return ctrl.Result{}, nil
    default:
        log.Error(nil, "Unknown phase", "phase", wfe.Status.Phase)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }
}

// reconcilePending handles the Pending phase
// - Validates spec
// - Checks resource locks (DD-WE-001)
// - Creates PipelineRun
func (r *WorkflowExecutionReconciler) reconcilePending(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Step 1: Validate spec
    if err := r.validateSpec(wfe); err != nil {
        log.Error(err, "Spec validation failed")
        return r.markFailed(ctx, wfe, "ValidationError", err.Error(), false)
    }

    // Step 2: Check resource lock (DD-WE-001)
    if blocked, conflicting := r.checkResourceLock(ctx, wfe); blocked {
        log.Info("Resource locked by another execution", "conflicting", conflicting.Name)
        return r.markSkipped(ctx, wfe, "ResourceBusy", conflicting)
    }

    // Step 3: Check cooldown (DD-WE-001)
    if recent := r.checkCooldown(ctx, wfe); recent != nil {
        log.Info("Same workflow ran recently", "previous", recent.Name)
        return r.markSkippedCooldown(ctx, wfe, "RecentlyRemediated", recent)
    }

    // Step 4: Create PipelineRun
    pr := r.buildPipelineRun(wfe)
    if err := r.Create(ctx, pr); err != nil {
        log.Error(err, "Failed to create PipelineRun")
        return r.markFailed(ctx, wfe, "PipelineRunCreationFailed", err.Error(), false)
    }

    log.Info("Created PipelineRun", "pipelineRun", pr.Name)
    r.Recorder.Event(wfe, "Normal", "PipelineRunCreated",
        fmt.Sprintf("Created PipelineRun %s for workflow %s", pr.Name, wfe.Spec.WorkflowRef.WorkflowID))

    // Transition to Running
    wfe.Status.Phase = workflowexecutionv1.PhaseRunning
    wfe.Status.StartTime = &metav1.Time{Time: time.Now()}
    wfe.Status.PipelineRunRef = &corev1.ObjectReference{
        Name:      pr.Name,
        Namespace: pr.Namespace,
    }

    if err := r.Status().Update(ctx, wfe); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: statusCheckInterval}, nil
}

// reconcileRunning handles the Running phase
// - Gets PipelineRun status
// - Maps Tekton status to WFE status
func (r *WorkflowExecutionReconciler) reconcileRunning(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Get PipelineRun
    var pr tektonv1.PipelineRun
    // PipelineRun is in execution namespace, not wfe.Namespace (DD-WE-002)
    if err := r.Get(ctx, client.ObjectKey{
        Name:      wfe.Status.PipelineRunRef.Name,
        Namespace: r.ExecutionNamespace,  // "kubernaut-workflows"
    }, &pr); err != nil {
        if apierrors.IsNotFound(err) {
            log.Error(err, "PipelineRun not found - deleted externally")
            return r.markFailed(ctx, wfe, "PipelineRunDeleted", "PipelineRun was deleted externally", false)
        }
        return ctrl.Result{}, err
    }

    // Check Tekton conditions
    for _, cond := range pr.Status.Conditions {
        if cond.Type != "Succeeded" {
            continue
        }

        switch cond.Status {
        case "True":
            // Success!
            return r.markCompleted(ctx, wfe, &pr)
        case "False":
            // Failure - extract details
            return r.markFailedFromPipelineRun(ctx, wfe, &pr, cond)
        default:
            // Still running - check again soon
            log.V(1).Info("PipelineRun still running", "reason", cond.Reason)
        }
    }

    // Requeue for next status check
    return ctrl.Result{RequeueAfter: statusCheckInterval}, nil
}

// buildPipelineRun creates a PipelineRun with bundle resolver
// ========================================
// RESOURCE LOCK PERSISTENCE (DD-WE-003)
// ========================================
// The PipelineRun IS the lock. Its existence means the resource is locked.
// Deterministic naming ensures Kubernetes atomically prevents race conditions.

// pipelineRunName generates a deterministic name based on target resource.
// Two WFEs targeting the same resource generate the same name.
// Kubernetes rejects duplicate creation, providing atomic locking.
func pipelineRunName(targetResource string) string {
    h := sha256.Sum256([]byte(targetResource))
    return fmt.Sprintf("wfe-%s", hex.EncodeToString(h[:])[:16])
}

// BuildPipelineRun creates a PipelineRun in the dedicated execution namespace (DD-WE-002)
// Uses deterministic name for atomic locking (DD-WE-003)
// Updated in v4.1: Added kind param, fixed ServiceAccountName location
func (r *WorkflowExecutionReconciler) BuildPipelineRun(
    wfe *workflowexecutionv1.WorkflowExecution,
) *tektonv1.PipelineRun {
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
            Namespace: r.ExecutionNamespace,  // Always "kubernaut-workflows" (DD-WE-002)
            Labels: map[string]string{
                "kubernaut.ai/workflow-execution": wfe.Name,
                "kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
                "kubernaut.ai/target-resource":    wfe.Spec.TargetResource,
                // Source tracking for cross-namespace lookup
                "kubernaut.ai/source-namespace":   wfe.Namespace,
            },
            // NOTE: No OwnerReference - cross-namespace not supported
            // Cleanup handled via finalizer in reconcileDelete()
        },
        Spec: tektonv1.PipelineRunSpec{
            PipelineRef: &tektonv1.PipelineRef{
                ResolverRef: tektonv1.ResolverRef{
                    Resolver: "bundles",
                    Params: []tektonv1.Param{
                        {Name: "bundle", Value: tektonv1.ParamValue{StringVal: wfe.Spec.WorkflowRef.ContainerImage}},
                        {Name: "name", Value: tektonv1.ParamValue{StringVal: "workflow"}},
                        {Name: "kind", Value: tektonv1.ParamValue{StringVal: "pipeline"}},  // Required by Tekton (v4.1)
                    },
                },
            },
            Params: params,
            TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
                ServiceAccountName: saName,  // Tekton v1 API location (v4.1)
            },
        },
    }
}

// checkResourceLock checks if another WFE is running on the same target (DD-WE-001)
func (r *WorkflowExecutionReconciler) checkResourceLock(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (blocked bool, conflicting *workflowexecutionv1.WorkflowExecution) {
    var wfeList workflowexecutionv1.WorkflowExecutionList
    if err := r.List(ctx, &wfeList, client.InNamespace(wfe.Namespace)); err != nil {
        return false, nil
    }

    for i := range wfeList.Items {
        other := &wfeList.Items[i]
        if other.Name == wfe.Name {
            continue
        }
        if other.Spec.TargetResource != wfe.Spec.TargetResource {
            continue
        }
        if other.Status.Phase == workflowexecutionv1.PhaseRunning ||
           other.Status.Phase == workflowexecutionv1.PhasePending {
            return true, other
        }
    }

    return false, nil
}

// checkCooldown checks cooldown and execution failure blocking (DD-WE-001, DD-WE-004)
// CRITICAL: Execution failures (wasExecutionFailure: true) block ALL retries
// Pre-execution failures use exponential backoff
func (r *WorkflowExecutionReconciler) checkCooldown(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (blocked bool, skipDetails *workflowexecutionv1.SkipDetails) {
    var wfeList workflowexecutionv1.WorkflowExecutionList
    if err := r.List(ctx, &wfeList, client.InNamespace(wfe.Namespace)); err != nil {
        return false, nil
    }

    for i := range wfeList.Items {
        other := &wfeList.Items[i]
        if other.Name == wfe.Name {
            continue
        }
        if other.Spec.TargetResource != wfe.Spec.TargetResource {
            continue
        }
        if other.Spec.WorkflowRef.WorkflowID != wfe.Spec.WorkflowRef.WorkflowID {
            continue
        }

        // CRITICAL: Check if previous execution FAILED DURING EXECUTION
        // Per cross-team agreement (WE→RO-003): NO retry for execution failures
        if other.Status.Phase == workflowexecutionv1.PhaseFailed &&
           other.Status.FailureDetails != nil &&
           other.Status.FailureDetails.WasExecutionFailure {
            return true, &workflowexecutionv1.SkipDetails{
                Reason:  SkipReasonPreviousExecutionFailed,
                Message: "Previous execution failed during workflow run. Manual intervention required.",
                SkippedAt: metav1.Now(),
            }
        }

        // Check exponential backoff for pre-execution failures (DD-WE-004)
        if other.Status.Phase == workflowexecutionv1.PhaseFailed &&
           other.Status.NextAllowedExecution != nil &&
           time.Now().Before(other.Status.NextAllowedExecution.Time) {
            remaining := time.Until(other.Status.NextAllowedExecution.Time)
            return true, &workflowexecutionv1.SkipDetails{
                Reason:  SkipReasonRecentlyRemediated,
                Message: fmt.Sprintf("Backoff active, next execution allowed in %s", remaining.Round(time.Second)),
                SkippedAt: metav1.Now(),
            }
        }

        // Check max consecutive failures (pre-execution only)
        if other.Status.ConsecutiveFailures >= int32(r.MaxConsecutiveFailures) {
            return true, &workflowexecutionv1.SkipDetails{
                Reason:  SkipReasonExhaustedRetries,
                Message: fmt.Sprintf("Exhausted %d consecutive pre-execution retries", r.MaxConsecutiveFailures),
                SkippedAt: metav1.Now(),
            }
        }

        // Standard cooldown for successful completions
        if other.Status.Phase == workflowexecutionv1.PhaseCompleted &&
           other.Status.CompletionTime != nil &&
           time.Since(other.Status.CompletionTime.Time) < r.BaseCooldownPeriod {
            return true, &workflowexecutionv1.SkipDetails{
                Reason:  SkipReasonRecentlyRemediated,
                Message: "Same workflow completed recently",
                SkippedAt: metav1.Now(),
            }
        }
    }

    return false, nil
}

// markCompleted transitions to Completed phase
func (r *WorkflowExecutionReconciler) markCompleted(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    pr *tektonv1.PipelineRun,
) (ctrl.Result, error) {
    wfe.Status.Phase = workflowexecutionv1.PhaseCompleted
    wfe.Status.Outcome = workflowexecutionv1.OutcomeSuccess
    wfe.Status.CompletionTime = pr.Status.CompletionTime
    wfe.Status.Message = "Workflow completed successfully"

    r.Recorder.Event(wfe, "Normal", "WorkflowCompleted",
        fmt.Sprintf("Workflow %s completed successfully", wfe.Spec.WorkflowRef.WorkflowID))

    return ctrl.Result{}, r.Status().Update(ctx, wfe)
}

// markFailed transitions to Failed phase
func (r *WorkflowExecutionReconciler) markFailed(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    reason string,
    message string,
    wasExecutionFailure bool,
) (ctrl.Result, error) {
    wfe.Status.Phase = workflowexecutionv1.PhaseFailed
    wfe.Status.Outcome = workflowexecutionv1.OutcomeFailed
    wfe.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    wfe.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
        Reason:              reason,
        Message:             message,
        FailedAt:            metav1.Now(),
        WasExecutionFailure: wasExecutionFailure,
        RequiresManualReview: wasExecutionFailure, // Manual review for execution failures
        NaturalLanguageSummary: fmt.Sprintf(
            "Workflow '%s' failed on target '%s': %s - %s",
            wfe.Spec.WorkflowRef.WorkflowID,
            wfe.Spec.TargetResource,
            reason,
            message,
        ),
    }

    r.Recorder.Event(wfe, "Warning", "WorkflowFailed",
        fmt.Sprintf("Workflow %s failed: %s", wfe.Spec.WorkflowRef.WorkflowID, reason))

    return ctrl.Result{}, r.Status().Update(ctx, wfe)
}

// markSkipped transitions to Skipped phase (resource lock)
func (r *WorkflowExecutionReconciler) markSkipped(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    reason string,
    conflicting *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    wfe.Status.Phase = workflowexecutionv1.PhaseSkipped
    wfe.Status.SkipDetails = &workflowexecutionv1.SkipDetails{
        Reason:  reason,
        Message: fmt.Sprintf("Target resource %s is being remediated by %s", wfe.Spec.TargetResource, conflicting.Name),
        ConflictingWorkflow: &workflowexecutionv1.ConflictingWorkflowInfo{
            Name:       conflicting.Name,
            WorkflowID: conflicting.Spec.WorkflowRef.WorkflowID,
            StartTime:  conflicting.Status.StartTime,
        },
        SkippedAt: metav1.Now(),
    }

    r.Recorder.Event(wfe, "Normal", "WorkflowSkipped",
        fmt.Sprintf("Skipped: target %s already being remediated by %s", wfe.Spec.TargetResource, conflicting.Name))

    return ctrl.Result{}, r.Status().Update(ctx, wfe)
}

// markSkippedCooldown transitions to Skipped phase (cooldown)
func (r *WorkflowExecutionReconciler) markSkippedCooldown(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    reason string,
    recent *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    wfe.Status.Phase = workflowexecutionv1.PhaseSkipped
    wfe.Status.SkipDetails = &workflowexecutionv1.SkipDetails{
        Reason:  reason,
        Message: fmt.Sprintf("Same workflow ran at %s (cooldown active)", recent.Status.CompletionTime.Format(time.RFC3339)),
        ConflictingWorkflow: &workflowexecutionv1.ConflictingWorkflowInfo{
            Name:       recent.Name,
            WorkflowID: recent.Spec.WorkflowRef.WorkflowID,
            StartTime:  recent.Status.CompletionTime,
        },
        SkippedAt: metav1.Now(),
    }

    r.Recorder.Event(wfe, "Normal", "WorkflowSkipped",
        fmt.Sprintf("Skipped: same workflow completed recently at %s", recent.Status.CompletionTime.Format(time.RFC3339)))

    return ctrl.Result{}, r.Status().Update(ctx, wfe)
}

// validateSpec validates the WorkflowExecution spec
func (r *WorkflowExecutionReconciler) validateSpec(wfe *workflowexecutionv1.WorkflowExecution) error {
    if wfe.Spec.WorkflowRef.ContainerImage == "" {
        return fmt.Errorf("workflowRef.containerImage is required")
    }
    if wfe.Spec.TargetResource == "" {
        return fmt.Errorf("targetResource is required")
    }
    // Validate targetResource format: {namespace}/{kind}/{name}
    parts := strings.Split(wfe.Spec.TargetResource, "/")
    if len(parts) != 3 {
        return fmt.Errorf("targetResource must be in format {namespace}/{kind}/{name}")
    }
    return nil
}

// reconcileDelete handles cleanup before deletion
func (r *WorkflowExecutionReconciler) reconcileDelete(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    if controllerutil.ContainsFinalizer(wfe, workflowExecutionFinalizer) {
        // Cleanup: Record final audit
        log.Info("Cleaning up WorkflowExecution", "name", wfe.Name)

        // Remove finalizer
        controllerutil.RemoveFinalizer(wfe, workflowExecutionFinalizer)
        if err := r.Update(ctx, wfe); err != nil {
            return ctrl.Result{}, err
        }
    }

    return ctrl.Result{}, nil
}

func (r *WorkflowExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Create index on targetResource for O(1) lock check (DD-WE-003)
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &workflowexecutionv1.WorkflowExecution{},
        "spec.targetResource",
        func(obj client.Object) []string {
            wfe := obj.(*workflowexecutionv1.WorkflowExecution)
            return []string{wfe.Spec.TargetResource}
        },
    ); err != nil {
        return err
    }

    return ctrl.NewControllerManagedBy(mgr).
        For(&workflowexecutionv1.WorkflowExecution{}).
        // Watch PipelineRuns using predicate filter (v4.1)
        // Predicate filter is simpler than namespace-scoped cache and achieves same goal:
        // Only watch PipelineRuns that have our tracking label
        Watches(
            &tektonv1.PipelineRun{},
            handler.EnqueueRequestsFromMapFunc(r.FindWFEForPipelineRun),
            builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
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

// FindWFEForPipelineRun maps PipelineRun events to WorkflowExecution reconcile requests
// Used for cross-namespace watch via labels (v4.1)
func (r *WorkflowExecutionReconciler) FindWFEForPipelineRun(
    ctx context.Context,
    obj client.Object,
) []reconcile.Request {
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
// STARTUP VALIDATION (ADR-030: Crash-if-Missing)
// ========================================

// CheckTektonAvailable validates Tekton Pipelines is installed.
// Called from main.go BEFORE starting the manager.
// MUST crash if Tekton is not available - this is a required dependency.
func CheckTektonAvailable(ctx context.Context, restMapper meta.RESTMapper) error {
    // Check if Pipeline CRD exists
    _, err := restMapper.RESTMapping(
        schema.GroupKind{Group: "tekton.dev", Kind: "Pipeline"},
        "v1",
    )
    if err != nil {
        return fmt.Errorf("Tekton Pipelines not installed or not accessible: %w", err)
    }

    // Check if PipelineRun CRD exists
    _, err = restMapper.RESTMapping(
        schema.GroupKind{Group: "tekton.dev", Kind: "PipelineRun"},
        "v1",
    )
    if err != nil {
        return fmt.Errorf("Tekton Pipelines CRDs incomplete: %w", err)
    }

    return nil
}

// Usage in cmd/workflowexecution/main.go:
//
// func main() {
//     // ... setup manager ...
//
//     // REQUIRED: Validate Tekton is installed (ADR-030)
//     if err := controller.CheckTektonAvailable(ctx, mgr.GetRESTMapper()); err != nil {
//         setupLog.Error(err, "Required dependency check failed")
//         os.Exit(1)  // CRASH - Tekton is required
//     }
//
//     // ... start manager ...
// }
```

---

## Recovery Coordination

**Status**: ✅ Aligned with Tekton Architecture

**CRITICAL PRINCIPLE**: WorkflowExecution controller does NOT handle its own recovery. Recovery coordination is the responsibility of the RemediationOrchestrator.

**Controller Responsibilities**:
- ✅ **WorkflowExecution**: Detect failures, update status to "Failed", extract failure details
- ✅ **RemediationOrchestrator**: Watch for "Failed" status, evaluate recovery viability

**Failure Classification**:
- `wasExecutionFailure: false` → Pre-execution failure (validation, resource lock) - safe to retry
- `wasExecutionFailure: true` → During-execution failure - requires manual review

---

