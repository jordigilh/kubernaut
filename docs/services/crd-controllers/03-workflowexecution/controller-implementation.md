## Controller Implementation

**Version**: 4.0
**Last Updated**: 2025-12-02
**CRD API Group**: `workflowexecution.kubernaut.ai/v1alpha1`
**Status**: ✅ Updated for Tekton Architecture (ADR-044)

**Location**: `internal/controller/workflowexecution_controller.go`

---

## Changelog

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
3. **PipelineRun Watch**: Watch owned PipelineRun for status sync
4. **Resource Locking**: Check for active executions on same target (DD-WE-001)
5. **Event Emission**: Operational visibility through Kubernetes events

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
    // Finalizer for cleanup coordination
    workflowExecutionFinalizer = "workflowexecution.kubernaut.ai/finalizer"

    // Cooldown period for same workflow on same target
    defaultCooldownPeriod = 5 * time.Minute

    // Status check interval for running PipelineRuns
    statusCheckInterval = 10 * time.Second
)

// WorkflowExecutionReconciler reconciles a WorkflowExecution object
type WorkflowExecutionReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    Recorder       record.EventRecorder
    CooldownPeriod time.Duration
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

//+kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=create;get;list;watch;delete
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns/status,verbs=get;list;watch
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
    if err := r.Get(ctx, client.ObjectKey{
        Name:      wfe.Status.PipelineRunRef.Name,
        Namespace: wfe.Namespace,
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
func (r *WorkflowExecutionReconciler) buildPipelineRun(
    wfe *workflowexecutionv1.WorkflowExecution,
) *tektonv1.PipelineRun {
    // Convert parameters to Tekton format
    params := make([]tektonv1.Param, 0, len(wfe.Spec.Parameters))
    for key, value := range wfe.Spec.Parameters {
        params = append(params, tektonv1.Param{
            Name:  key,
            Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: value},
        })
    }

    return &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      wfe.Name,
            Namespace: wfe.Namespace,
            Labels: map[string]string{
                "kubernaut.ai/workflow-execution": wfe.Name,
                "kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
                "kubernaut.ai/target-resource":    wfe.Spec.TargetResource,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(wfe, workflowexecutionv1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: tektonv1.PipelineRunSpec{
            PipelineRef: &tektonv1.PipelineRef{
                ResolverRef: tektonv1.ResolverRef{
                    Resolver: "bundles",
                    Params: []tektonv1.Param{
                        {Name: "bundle", Value: tektonv1.ParamValue{StringVal: wfe.Spec.WorkflowRef.ContainerImage}},
                        {Name: "name", Value: tektonv1.ParamValue{StringVal: "workflow"}},
                    },
                },
            },
            Params: params,
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

// checkCooldown checks if same workflow ran recently on same target (DD-WE-001)
func (r *WorkflowExecutionReconciler) checkCooldown(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) *workflowexecutionv1.WorkflowExecution {
    cooldown := r.CooldownPeriod
    if cooldown == 0 {
        cooldown = defaultCooldownPeriod
    }

    var wfeList workflowexecutionv1.WorkflowExecutionList
    if err := r.List(ctx, &wfeList, client.InNamespace(wfe.Namespace)); err != nil {
        return nil
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
        if other.Status.Phase == workflowexecutionv1.PhaseCompleted &&
           other.Status.CompletionTime != nil &&
           time.Since(other.Status.CompletionTime.Time) < cooldown {
            return other
        }
    }

    return nil
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
    return ctrl.NewControllerManagedBy(mgr).
        For(&workflowexecutionv1.WorkflowExecution{}).
        Owns(&tektonv1.PipelineRun{}).  // Watch owned PipelineRuns
        Complete(r)
}
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
