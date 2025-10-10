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

## 🔄 **Recovery Coordination Notes**

**Status**: ✅ Phase 1 Critical Fix (C6)
**Reference**: [`docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md)
**Business Requirement**: BR-WF-RECOVERY-007

### Overview

**CRITICAL PRINCIPLE**: WorkflowExecution controller does NOT handle its own recovery. Recovery coordination is the responsibility of the Remediation Orchestrator Controller.

**Controller Responsibilities**:
- ✅ **WorkflowExecution**: Detect failures, update status to "failed", record failure details
- ✅ **Remediation Orchestrator**: Watch for "failed" status, evaluate recovery viability, create new AIAnalysis CRD

**Anti-Pattern**: WorkflowExecution directly triggering recovery actions

### Failure Detection & Status Update

When a workflow step fails, WorkflowExecution updates its own status:

```go
// In reconcileExecuting phase
func (r *WorkflowExecutionReconciler) reconcileExecuting(
    ctx context.Context,
    wf *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {

    // Execute current step
    stepResult, err := r.executeStep(ctx, wf, currentStep)

    if err != nil || stepResult.Failed {
        // Record failure details in WorkflowExecution status
        wf.Status.Phase = "failed"
        wf.Status.FailedStep = currentStep.Index
        wf.Status.FailureReason = fmt.Sprintf("Step %d failed: %v", currentStep.Index, err)
        wf.Status.FailedAction = currentStep.Action.Type
        wf.Status.ErrorType = classifyError(err)  // "timeout", "permission_denied", "resource_not_found", etc.
        wf.Status.CompletionTime = &metav1.Time{Time: time.Now()}

        // Record detailed execution state for recovery analysis
        wf.Status.ExecutionSnapshot = &workflowexecutionv1.ExecutionSnapshot{
            CompletedSteps: wf.Status.CompletedSteps,
            CurrentStep:    currentStep.Index,
            ClusterState:   captureClusterState(ctx, wf),
            ResourceSnapshot: captureResourceSnapshot(ctx, wf),
        }

        // Update status - Remediation Orchestrator watches this
        if err := r.Status().Update(ctx, wf); err != nil {
            return ctrl.Result{}, err
        }

        // Emit event for visibility
        r.Recorder.Event(wf, corev1.EventTypeWarning, "WorkflowFailed",
            fmt.Sprintf("Workflow failed at step %d: %v", currentStep.Index, err))

        // Return with no requeue - WorkflowExecution's job is done
        // Remediation Orchestrator will handle recovery coordination
        return ctrl.Result{}, nil
    }

    // Continue with next step...
}
```

### WorkflowExecution Status Fields for Recovery

```go
// api/workflowexecution/v1/workflowexecution_types.go
type WorkflowExecutionStatus struct {
    // Existing fields...
    Phase            string       `json:"phase"`
    CompletedSteps   int          `json:"completedSteps"`
    TotalSteps       int          `json:"totalSteps"`
    CompletionTime   *metav1.Time `json:"completionTime,omitempty"`

    // Enhanced failure tracking for recovery
    FailedStep       *int         `json:"failedStep,omitempty"`       // Which step failed (0-based)
    FailedAction     *string      `json:"failedAction,omitempty"`     // Action type that failed (e.g., "scale-deployment")
    FailureReason    *string      `json:"failureReason,omitempty"`    // Human-readable failure reason
    ErrorType        *string      `json:"errorType,omitempty"`        // Classified error ("timeout", "permission_denied", etc.)
    ExecutionSnapshot *ExecutionSnapshot `json:"executionSnapshot,omitempty"` // State at failure time
}

// NEW: ExecutionSnapshot captures state at failure time for recovery analysis
type ExecutionSnapshot struct {
    CompletedSteps   []StepResult           `json:"completedSteps"`
    CurrentStep      int                    `json:"currentStep"`
    ClusterState     map[string]interface{} `json:"clusterState"`
    ResourceSnapshot map[string]interface{} `json:"resourceSnapshot"`
    Timestamp        metav1.Time            `json:"timestamp"`
}

type StepResult struct {
    StepIndex   int         `json:"stepIndex"`
    Action      string      `json:"action"`
    Status      string      `json:"status"` // "completed", "failed", "skipped"
    Duration    string      `json:"duration"`
    Output      interface{} `json:"output,omitempty"`
    Error       *string     `json:"error,omitempty"`
}
```

### Remediation Orchestrator Watches WorkflowExecution

The Remediation Orchestrator watches WorkflowExecution CRDs and triggers recovery:

```go
// In Remediation Orchestrator Controller
func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1.RemediationRequest{}).
        Owns(&processingv1.RemediationProcessing{}).
        Owns(&aianalysisv1.AIAnalysis{}).
        Owns(&workflowexecutionv1.WorkflowExecution{}).  // WATCH owned WorkflowExecution CRDs
        Complete(r)
}

// Reconcile detects workflow failures via watch
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var remediation remediationv1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Check if current workflow has failed
    if remediation.Status.CurrentWorkflowExecutionRef != nil {
        var workflow workflowexecutionv1.WorkflowExecution
        if err := r.Get(ctx, client.ObjectKey{
            Name:      *remediation.Status.CurrentWorkflowExecutionRef,
            Namespace: remediation.Namespace,
        }, &workflow); err == nil {

            // Workflow failure detected
            if workflow.Status.Phase == "failed" && remediation.Status.OverallPhase == "executing" {
                log.Info("Workflow failure detected, evaluating recovery viability",
                    "workflow", workflow.Name,
                    "failedStep", *workflow.Status.FailedStep,
                    "failureReason", *workflow.Status.FailureReason)

                // Evaluate recovery viability (BR-WF-RECOVERY-010)
                canRecover, reason := r.evaluateRecoveryViability(ctx, &remediation, &workflow)

                if canRecover {
                    // Transition to recovering phase and create new AIAnalysis
                    return r.initiateRecovery(ctx, &remediation, &workflow)
                } else {
                    // Escalate to manual review
                    return r.escalateToManualReview(ctx, &remediation, reason)
                }
            }
        }
    }

    // Continue with normal reconciliation...
}
```

### Recovery Initiation Flow

**Step 1**: WorkflowExecution updates status to "failed"

```yaml
apiVersion: workflowexecution.kubernaut.io/v1
kind: WorkflowExecution
metadata:
  name: workflow-001
  namespace: default
  ownerReferences:
    - apiVersion: remediation.kubernaut.io/v1
      kind: RemediationRequest
      name: rr-2025-001
status:
  phase: failed  # ← WorkflowExecution sets this
  failedStep: 3
  failedAction: scale-deployment
  failureReason: "Operation timed out after 5m"
  errorType: timeout
  completedSteps: 2
  totalSteps: 5
  executionSnapshot:
    currentStep: 3
    clusterState:
      deployment_replicas: 2
      pod_status: "CrashLoopBackOff"
    resourceSnapshot:
      deployment_name: "api-server"
      namespace: "production"
```

**Step 2**: Remediation Orchestrator watches and detects failure

```go
// Remediation Orchestrator reconcile triggered by watch event
func (r *RemediationRequestReconciler) initiateRecovery(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    failedWorkflow *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {

    log := ctrl.LoggerFrom(ctx)
    log.Info("Initiating recovery",
        "recoveryAttempt", remediation.Status.RecoveryAttempts+1,
        "maxAttempts", remediation.Status.MaxRecoveryAttempts)

    // Update RemediationRequest to "recovering" phase
    remediation.Status.OverallPhase = "recovering"
    remediation.Status.RecoveryAttempts++
    remediation.Status.LastFailureTime = &metav1.Time{Time: time.Now()}
    reason := fmt.Sprintf("workflow_%s_step_%d",
        *failedWorkflow.Status.ErrorType,
        *failedWorkflow.Status.FailedStep)
    remediation.Status.RecoveryReason = &reason

    // Create new AIAnalysis CRD with recovery context
    aiAnalysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-recovery-%d", remediation.Name, remediation.Status.RecoveryAttempts),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            // Copy signal context from original
            SignalContext:          remediation.Spec.SignalContext,
            RemediationRequestRef:  corev1.LocalObjectReference{Name: remediation.Name},

            // NEW: Recovery-specific fields
            IsRecoveryAttempt:      true,
            RecoveryAttemptNumber:  remediation.Status.RecoveryAttempts,
            FailedWorkflowRef:      &corev1.LocalObjectReference{Name: failedWorkflow.Name},
            FailedStep:             failedWorkflow.Status.FailedStep,
            FailureReason:          failedWorkflow.Status.FailureReason,
            PreviousAIAnalysisRefs: remediation.Status.AIAnalysisRefs,
        },
    }

    if err := r.Create(ctx, aiAnalysis); err != nil {
        return ctrl.Result{}, err
    }

    // Update refs arrays in RemediationRequest status
    remediation.Status.AIAnalysisRefs = append(
        remediation.Status.AIAnalysisRefs,
        remediationv1.AIAnalysisReference{
            Name:      aiAnalysis.Name,
            Namespace: aiAnalysis.Namespace,
        },
    )

    remediation.Status.WorkflowExecutionRefs = append(
        remediation.Status.WorkflowExecutionRefs,
        remediationv1.WorkflowExecutionReferenceWithOutcome{
            Name:           failedWorkflow.Name,
            Namespace:      failedWorkflow.Namespace,
            Outcome:        "failed",
            FailedStep:     failedWorkflow.Status.FailedStep,
            FailureReason:  failedWorkflow.Status.FailureReason,
            CompletionTime: failedWorkflow.Status.CompletionTime,
            AttemptNumber:  remediation.Status.RecoveryAttempts,
        },
    )

    if err := r.Status().Update(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    r.Recorder.Event(remediation, corev1.EventTypeNormal, "RecoveryInitiated",
        fmt.Sprintf("Recovery attempt %d initiated after workflow failure",
            remediation.Status.RecoveryAttempts))

    return ctrl.Result{}, nil
}
```

### Error Classification

WorkflowExecution classifies errors to help recovery decision-making:

```go
// classifyError categorizes error types for pattern detection
func classifyError(err error) string {
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        return "timeout"
    case strings.Contains(err.Error(), "forbidden"):
        return "permission_denied"
    case strings.Contains(err.Error(), "not found"):
        return "resource_not_found"
    case strings.Contains(err.Error(), "conflict"):
        return "resource_conflict"
    case strings.Contains(err.Error(), "quota exceeded"):
        return "resource_quota_exceeded"
    case strings.Contains(err.Error(), "invalid"):
        return "validation_error"
    default:
        return "unknown_error"
    }
}
```

### Separation of Concerns

| Responsibility | WorkflowExecution | Remediation Orchestrator |
|---|---|---|
| **Execute workflow steps** | ✅ Yes | ❌ No |
| **Detect step failures** | ✅ Yes | ❌ No |
| **Update own status to "failed"** | ✅ Yes | ❌ No |
| **Capture execution snapshot** | ✅ Yes | ❌ No |
| **Evaluate recovery viability** | ❌ No | ✅ Yes |
| **Create recovery AIAnalysis** | ❌ No | ✅ Yes |
| **Track recovery attempts** | ❌ No | ✅ Yes |
| **Escalate to manual review** | ❌ No | ✅ Yes |
| **Update RemediationRequest** | ❌ No | ✅ Yes |

### Testing Recovery Coordination

```go
func TestWorkflowExecution_FailureDetection(t *testing.T) {
    // WorkflowExecution detects step failure
    wf := &workflowexecutionv1.WorkflowExecution{
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            Phase: "executing",
            CompletedSteps: 2,
        },
    }

    // Simulate step failure
    err := fmt.Errorf("operation timed out")
    reconciler := &WorkflowExecutionReconciler{}

    result, err := reconciler.reconcileExecuting(context.Background(), wf)

    // Verify WorkflowExecution updated its own status
    assert.Equal(t, "failed", wf.Status.Phase)
    assert.Equal(t, 2, *wf.Status.FailedStep)
    assert.Equal(t, "timeout", *wf.Status.ErrorType)
    assert.NotNil(t, wf.Status.ExecutionSnapshot)

    // Verify NO recovery triggered (that's Remediation Orchestrator's job)
    assert.NoError(t, err)  // No error returned
    assert.Equal(t, ctrl.Result{}, result)  // No requeue
}

func TestRemediationOrchestrator_WatchesWorkflowFailure(t *testing.T) {
    // Setup: RemediationRequest with executing workflow
    remediation := &remediationv1.RemediationRequest{
        Status: remediationv1.RemediationRequestStatus{
            OverallPhase: "executing",
            CurrentWorkflowExecutionRef: ptr.To("workflow-001"),
            RecoveryAttempts: 0,
            MaxRecoveryAttempts: 3,
        },
    }

    // Workflow fails
    workflow := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{Name: "workflow-001"},
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            Phase: "failed",
            FailedStep: ptr.To(3),
            FailureReason: ptr.To("timeout"),
        },
    }

    reconciler := &RemediationRequestReconciler{}

    // Remediation Orchestrator reconciles
    result, err := reconciler.Reconcile(context.Background(), ctrl.Request{})

    // Verify recovery initiated
    assert.NoError(t, err)
    assert.Equal(t, "recovering", remediation.Status.OverallPhase)
    assert.Equal(t, 1, remediation.Status.RecoveryAttempts)
    assert.Len(t, remediation.Status.AIAnalysisRefs, 2)  // Initial + recovery
}
```

### Key Principles

1. **WorkflowExecution**: "I failed, here's why" (status update only)
2. **Remediation Orchestrator**: "I see you failed, let me coordinate recovery" (watch & coordinate)
3. **No self-recovery**: WorkflowExecution NEVER triggers its own recovery
4. **Clean separation**: Clear responsibility boundaries prevent tight coupling
5. **Watch-based coordination**: Kubernetes-native pattern for controller coordination

### Related Documentation

- **Architecture**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md)
- **Business Requirements**: BR-WF-RECOVERY-007 in `docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`
- **Remediation Orchestrator**: `docs/services/crd-controllers/05-remediationorchestrator/controller-implementation.md` (C7)
- **CRD Schema**: `docs/services/crd-controllers/05-remediationorchestrator/crd-schema.md`

---

