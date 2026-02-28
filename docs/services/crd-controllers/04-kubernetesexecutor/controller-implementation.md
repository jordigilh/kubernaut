## Controller Implementation

> **DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service were eliminated by ADR-025 and replaced by Tekton TaskRun via WorkflowExecution. This documentation is retained for historical reference only. API types and CRD manifests have been removed from the codebase.

**Location**: `internal/controller/kubernetesexecution_controller.go`

### Controller Configuration

```go
package controller

import (
    "context"
    "fmt"
    "time"

    batchv1 "k8s.io/api/batch/v1"
    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    "k8s.io/utils/ptr"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"
    "github.com/jordigilh/kubernaut/pkg/kubernetesexecution"
    "github.com/jordigilh/kubernaut/pkg/storage"
)

const (
    kubernetesExecutionFinalizer = "kubernetesexecution.kubernaut.io/finalizer"

    // Timeout configuration
    defaultValidationTimeout = 30 * time.Second
    defaultExecutionTimeout  = 5 * time.Minute
    defaultApprovalTimeout   = 1 * time.Hour
)

// KubernetesExecutionReconciler reconciles a KubernetesExecution object
type KubernetesExecutionReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    Recorder       record.EventRecorder

    // Action handlers for each action type
    ActionHandlers map[string]kubernetesexecution.ActionHandler

    // Rego policy evaluator
    PolicyEvaluator *kubernetesexecution.PolicyEvaluator

    // Audit storage client
    AuditStorage storage.AuditStorageClient
}

//+kubebuilder:rbac:groups=kubernetesexecution.kubernaut.io,resources=kubernetesexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesexecution.kubernaut.io,resources=kubernetesexecutions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernetesexecution.kubernaut.io,resources=kubernetesexecutions/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods/log,verbs=get
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *KubernetesExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Fetch KubernetesExecution CRD
    var ke kubernetesexecutionv1.KubernetesExecution
    if err := r.Get(ctx, req.NamespacedName, &ke); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion
    if !ke.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, &ke)
    }

    // Add finalizer
    if !controllerutil.ContainsFinalizer(&ke, kubernetesExecutionFinalizer) {
        controllerutil.AddFinalizer(&ke, kubernetesExecutionFinalizer)
        if err := r.Update(ctx, &ke); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Set owner reference to WorkflowExecution
    if err := r.ensureOwnerReference(ctx, &ke); err != nil {
        log.Error(err, "Failed to set owner reference")
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // Initialize phase
    if ke.Status.Phase == "" {
        ke.Status.Phase = "validating"
        if err := r.Status().Update(ctx, &ke); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Reconcile based on phase
    var result ctrl.Result
    var err error

    switch ke.Status.Phase {
    case "validating":
        result, err = r.reconcileValidating(ctx, &ke)
    case "validated":
        result, err = r.reconcileValidated(ctx, &ke)
    case "executing":
        result, err = r.reconcileExecuting(ctx, &ke)
    case "rollback_ready", "completed":
        return ctrl.Result{}, nil // Terminal state
    case "failed":
        return ctrl.Result{}, nil // Terminal state
    default:
        log.Error(nil, "Unknown phase", "phase", ke.Status.Phase)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }

    return result, err
}

func (r *KubernetesExecutionReconciler) reconcileValidating(ctx context.Context, ke *kubernetesexecutionv1.KubernetesExecution) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    log.Info("Validating action execution", "action", ke.Spec.Action)

    // Step 1: Parameter validation
    if err := r.validateParameters(ke); err != nil {
        ke.Status.Phase = "failed"
        ke.Status.ValidationResults = &kubernetesexecutionv1.ValidationResults{
            ParameterValidation: false,
        }
        r.Status().Update(ctx, ke)
        return ctrl.Result{}, err
    }

    // Step 2: RBAC validation
    handler := r.ActionHandlers[ke.Spec.Action]
    if handler == nil {
        return ctrl.Result{}, fmt.Errorf("unknown action type: %s", ke.Spec.Action)
    }

    saName := handler.GetServiceAccount()
    if err := r.validateServiceAccount(ctx, saName); err != nil {
        ke.Status.Phase = "failed"
        return ctrl.Result{}, err
    }

    // Step 3: Resource existence check
    if err := r.validateResourceExists(ctx, ke); err != nil {
        ke.Status.Phase = "failed"
        return ctrl.Result{}, err
    }

    // Step 4: Rego policy evaluation
    policyResult, err := r.PolicyEvaluator.Evaluate(ctx, ke)
    if err != nil || !policyResult.Allowed {
        ke.Status.Phase = "failed"
        ke.Status.ValidationResults = &kubernetesexecutionv1.ValidationResults{
            PolicyValidation: policyResult,
        }
        r.Status().Update(ctx, ke)
        return ctrl.Result{}, fmt.Errorf("policy validation failed: %v", policyResult.Violations)
    }

    // Step 5: Dry-run if required
    var dryRunResults *kubernetesexecutionv1.DryRunResults
    if policyResult.RequiresDryRun {
        dryRunResults, err = r.executeDryRun(ctx, ke, handler)
        if err != nil {
            ke.Status.Phase = "failed"
            return ctrl.Result{}, err
        }
    }

    // Validation complete
    ke.Status.Phase = "validated"
    ke.Status.ValidationResults = &kubernetesexecutionv1.ValidationResults{
        ParameterValidation: true,
        RBACValidation:      true,
        ResourceExists:      true,
        PolicyValidation:    policyResult,
        DryRunResults:       dryRunResults,
        ValidationTime:      metav1.Now(),
    }

    if err := r.Status().Update(ctx, ke); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{Requeue: true}, nil
}

func (r *KubernetesExecutionReconciler) reconcileValidated(ctx context.Context, ke *kubernetesexecutionv1.KubernetesExecution) (ctrl.Result, error) {
    // Check if approval required
    if ke.Status.ValidationResults.PolicyValidation.RequiredApproval && !ke.Spec.ApprovalReceived {
        // Wait for approval
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    // Approval received or not required - proceed to execution
    ke.Status.Phase = "executing"
    if err := r.Status().Update(ctx, ke); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{Requeue: true}, nil
}

func (r *KubernetesExecutionReconciler) reconcileExecuting(ctx context.Context, ke *kubernetesexecutionv1.KubernetesExecution) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Check if Job already exists
    if ke.Status.JobName != "" {
        // Monitor existing Job
        return r.monitorJob(ctx, ke)
    }

    // Create execution Job
    handler := r.ActionHandlers[ke.Spec.Action]
    job := r.buildExecutionJob(ke, handler)

    if err := r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
        log.Error(err, "Failed to create execution Job")
        return ctrl.Result{RequeueAfter: 15 * time.Second}, err
    }

    ke.Status.JobName = job.Name
    if err := r.Status().Update(ctx, ke); err != nil {
        return ctrl.Result{}, err
    }

    log.Info("Execution Job created", "jobName", job.Name)

    // Monitor Job for completion
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *KubernetesExecutionReconciler) buildExecutionJob(ke *kubernetesexecutionv1.KubernetesExecution, handler kubernetesexecution.ActionHandler) *batchv1.Job {
    jobName := fmt.Sprintf("exec-%s-%s", ke.Spec.Action, ke.Name[:8])

    return &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      jobName,
            Namespace: "kubernaut-executor",
            Labels: map[string]string{
                "kubernaut.io/execution-id": ke.Name,
                "kubernaut.io/action":       ke.Spec.Action,
            },
        },
        Spec: batchv1.JobSpec{
            TTLSecondsAfterFinished: ptr.To(int32(300)), // 5min cleanup
            BackoffLimit:            ptr.To(int32(ke.Spec.MaxRetries)),
            ActiveDeadlineSeconds:   ptr.To(int64(ke.Spec.Timeout.Seconds())),
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    ServiceAccountName: handler.GetServiceAccount(),
                    RestartPolicy:      corev1.RestartPolicyNever,
                    Containers: []corev1.Container{
                        {
                            Name:    "kubectl-executor",
                            Image:   "bitnami/kubectl:1.28",
                            Command: handler.BuildCommand(ke.Spec.Parameters),
                        },
                    },
                },
            },
        },
    }
}

func (r *KubernetesExecutionReconciler) monitorJob(ctx context.Context, ke *kubernetesexecutionv1.KubernetesExecution) (ctrl.Result, error) {
    var job batchv1.Job
    if err := r.Get(ctx, client.ObjectKey{Name: ke.Status.JobName, Namespace: "kubernaut-executor"}, &job); err != nil {
        return ctrl.Result{}, err
    }

    // Check Job status
    if job.Status.Succeeded > 0 {
        // Job succeeded
        return r.handleJobSuccess(ctx, ke, &job)
    } else if job.Status.Failed > 0 {
        // Job failed
        return r.handleJobFailure(ctx, ke, &job)
    }

    // Job still running - requeue
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *KubernetesExecutionReconciler) handleJobSuccess(ctx context.Context, ke *kubernetesexecutionv1.KubernetesExecution, job *batchv1.Job) (ctrl.Result, error) {
    // Extract rollback information
    handler := r.ActionHandlers[ke.Spec.Action]
    rollbackInfo, err := handler.ExtractRollbackInfo(ctx, ke.Spec.Parameters)
    if err != nil {
        // Log error but don't fail execution
        log.FromContext(ctx).Error(err, "Failed to extract rollback info")
    }

    // Update status
    ke.Status.Phase = "rollback_ready"
    ke.Status.ExecutionResults = &kubernetesexecutionv1.ExecutionResults{
        Success:   true,
        JobName:   job.Name,
        StartTime: job.Status.StartTime,
        EndTime:   job.Status.CompletionTime,
        Duration:  job.Status.CompletionTime.Sub(job.Status.StartTime.Time).String(),
    }
    ke.Status.RollbackInformation = rollbackInfo

    if err := r.Status().Update(ctx, ke); err != nil {
        return ctrl.Result{}, err
    }

    // Store audit
    r.storeAudit(ctx, ke)

    return ctrl.Result{}, nil
}

func (r *KubernetesExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&kubernetesexecutionv1.KubernetesExecution{}).
        Owns(&batchv1.Job{}).
        Complete(r)
}
```

---

