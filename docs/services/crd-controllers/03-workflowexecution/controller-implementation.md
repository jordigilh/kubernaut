## Controller Implementation

**Location**: `internal/controller/workflowexecution_controller.go`

### Controller Configuration

**Critical Patterns from [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)**:
1. **Owner References**: WorkflowExecution CRD owned by RemediationRequest for cascade deletion
2. **Finalizers**: Cleanup coordination before deletion
3. **Watch Optimization**: Watches KubernetesExecution CRDs for step completion
4. **Timeout Handling**: Phase-level timeout detection and escalation
5. **Event Emission**: Operational visibility through Kubernetes events

```go
package controller

import (
    "context"
    "fmt"
    "time"

    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    apimeta "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    executorv1 "github.com/jordigilh/kubernaut/api/executor/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/v1"
)

const (
    // Finalizer for cleanup coordination
    workflowExecutionFinalizer = "workflowexecution.kubernaut.io/finalizer"

    // Timeout configuration
    defaultPlanningTimeout   = 30 * time.Second
    defaultValidationTimeout = 5 * time.Minute
    defaultStepTimeout       = 5 * time.Minute
    defaultMonitoringTimeout = 10 * time.Minute
)

// WorkflowExecutionReconciler reconciles a WorkflowExecution object
type WorkflowExecutionReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    Recorder       record.EventRecorder  // Event emission for visibility
    WorkflowEngine WorkflowEngine        // Workflow planning and orchestration
    Validator      WorkflowValidator     // Safety validation
    Monitor        WorkflowMonitor       // Execution monitoring
}

// ========================================
// ARCHITECTURAL NOTE: Remediation Orchestrator Pattern
// ========================================
//
// This controller (WorkflowExecution) updates ONLY its own status.
// The RemediationRequest Controller (Remediation Orchestrator) watches this CRD and aggregates status.
//
// DO NOT update RemediationRequest.status from this controller.
// The watch-based coordination pattern handles all status aggregation automatically.
//
// See: docs/services/crd-controllers/05-remediationorchestrator/overview.md
// See: docs/services/crd-controllers/CENTRAL_CONTROLLER_VIOLATION_ANALYSIS.md

//+kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubernetesexecution.kubernaut.io,resources=kubernetesexecutions,verbs=create;get;list;watch
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertremediations,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Fetch WorkflowExecution CRD
    var wf workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &wf); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion with finalizer
    if !wf.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, &wf)
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&wf, workflowExecutionFinalizer) {
        controllerutil.AddFinalizer(&wf, workflowExecutionFinalizer)
        if err := r.Update(ctx, &wf); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Set owner reference to RemediationRequest (for cascade deletion)
    if err := r.ensureOwnerReference(ctx, &wf); err != nil {
        log.Error(err, "Failed to set owner reference")
        r.Recorder.Event(&wf, "Warning", "OwnerReferenceFailed",
            fmt.Sprintf("Failed to set owner reference: %v", err))
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Check for phase timeout
    if r.isPhaseTimedOut(&wf) {
        return r.handlePhaseTimeout(ctx, &wf)
    }

    // Initialize phase if empty
    if wf.Status.Phase == "" {
        wf.Status.Phase = "planning"
        wf.Status.PlanningStartTime = &metav1.Time{Time: time.Now()}
        if err := r.Status().Update(ctx, &wf); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Reconcile based on current phase
    var result ctrl.Result
    var err error

    switch wf.Status.Phase {
    case "planning":
        result, err = r.reconcilePlanning(ctx, &wf)
    case "validating":
        result, err = r.reconcileValidating(ctx, &wf)
    case "executing":
        result, err = r.reconcileExecuting(ctx, &wf)
    case "monitoring":
        result, err = r.reconcileMonitoring(ctx, &wf)
    case "rolling_back":
        result, err = r.reconcileRollback(ctx, &wf)
    case "completed", "failed":
        // Terminal states - use optimized requeue strategy
        return r.determineRequeueStrategy(&wf), nil
    default:
        log.Error(nil, "Unknown phase", "phase", wf.Status.Phase)
        r.Recorder.Event(&wf, "Warning", "UnknownPhase",
            fmt.Sprintf("Unknown phase: %s", wf.Status.Phase))
        return ctrl.Result{RequeueAfter: time.Second * 30}, nil
    }

    return result, err
}

// Additional controller methods would be implemented here...
// reconcilePlanning, reconcileValidating, reconcileExecuting, reconcileMonitoring, etc.

func (r *WorkflowExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&workflowexecutionv1.WorkflowExecution{}).
        Owns(&executorv1.KubernetesExecution{}).  // Watch KubernetesExecution CRDs
        Complete(r)
}
```

---

