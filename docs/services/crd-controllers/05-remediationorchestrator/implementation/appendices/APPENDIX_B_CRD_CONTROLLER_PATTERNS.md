# Appendix B: CRD Controller Patterns

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](../IMPLEMENTATION_PLAN_V1.1.md)
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE V3.0 §Appendix B: CRD Controller Variant
**Last Updated**: 2025-12-04

---

## 🔷 CRD API Group Standard (DD-CRD-001)

**API Group**: `remediation.kubernaut.ai/v1alpha1`
**Kind**: `RemediationRequest`

```yaml
apiVersion: remediation.kubernaut.ai/v1alpha1
kind: RemediationRequest
metadata:
  name: example-remediation
  namespace: kubernaut-system
spec:
  alertData:
    fingerprint: "abc123"
    alertName: "HighMemoryUsage"
  targetResource:
    kind: Deployment
    name: my-app
    namespace: production
status:
  phase: Processing
  childCRDs:
    signalProcessing: "sp-abc123"
```

---

## 🔄 Reconciliation Loop Pattern

### Standard Controller Structure

```go
// pkg/controller/remediationorchestrator/controller.go
package controller

import (
    "context"
    "time"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// RemediationOrchestratorReconciler reconciles a RemediationRequest object
type RemediationOrchestratorReconciler struct {
    client.Client
    Scheme *runtime.Scheme

    // Business logic components
    PhaseHandlers     *PhaseHandlerRegistry
    StatusAggregator  *StatusAggregator
    TimeoutDetector   *TimeoutDetector
    NotificationCreator *NotificationCreator
    FinalizerHandler  *FinalizerHandler
    Metrics           *MetricsCollector
}

//+kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests/finalizers,verbs=update

// Reconcile implements the reconciliation loop
func (r *RemediationOrchestratorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    startTime := time.Now()

    defer func() {
        r.Metrics.ReconcileDuration.WithLabelValues(req.Name).Observe(time.Since(startTime).Seconds())
    }()

    // 1. FETCH RESOURCE
    rr := &remediationv1alpha1.RemediationRequest{}
    if err := r.Get(ctx, req.NamespacedName, rr); err != nil {
        if apierrors.IsNotFound(err) {
            log.Info("RemediationRequest not found, ignoring")
            return ctrl.Result{}, nil
        }
        log.Error(err, "Failed to get RemediationRequest")
        return ctrl.Result{}, err
    }

    // 2. HANDLE FINALIZER (deletion)
    result, err := r.FinalizerHandler.HandleFinalizer(ctx, rr)
    if err != nil || !result.IsZero() {
        return result, err
    }
    if !rr.DeletionTimestamp.IsZero() {
        return ctrl.Result{}, nil
    }

    // 3. CHECK TERMINAL STATES
    if isTerminalPhase(rr.Status.Phase) {
        log.Info("RemediationRequest in terminal state", "phase", rr.Status.Phase)
        return ctrl.Result{}, nil
    }

    // 4. CHECK TIMEOUTS
    timeoutResult, err := r.TimeoutDetector.CheckTimeout(ctx, rr)
    if err != nil {
        log.Error(err, "Failed to check timeout")
        return ctrl.Result{}, err
    }
    if timeoutResult.TimedOut {
        return r.handleTimeout(ctx, rr, timeoutResult)
    }

    // 5. INITIALIZE STATUS
    if rr.Status.Phase == "" {
        return r.initializeStatus(ctx, rr)
    }

    // 6. AGGREGATE CHILD STATUS
    aggregated, err := r.StatusAggregator.AggregateStatus(ctx, rr)
    if err != nil {
        log.Error(err, "Failed to aggregate status")
        return ctrl.Result{}, err
    }

    // 7. EXECUTE PHASE HANDLER
    handler := r.PhaseHandlers.GetHandler(rr.Status.Phase)
    if handler == nil {
        log.Error(nil, "No handler for phase", "phase", rr.Status.Phase)
        return ctrl.Result{}, fmt.Errorf("no handler for phase: %s", rr.Status.Phase)
    }

    return handler.Handle(ctx, rr, aggregated)
}

// SetupWithManager sets up the controller with the Manager
func (r *RemediationOrchestratorReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1alpha1.RemediationRequest{}).
        Owns(&signalprocessingv1alpha1.SignalProcessing{}).
        Owns(&aianalysisv1alpha1.AIAnalysis{}).
        Owns(&workflowexecutionv1alpha1.WorkflowExecution{}).
        Owns(&notificationv1alpha1.NotificationRequest{}).
        WithOptions(controller.Options{
            MaxConcurrentReconciles: 10,
        }).
        Complete(r)
}

func isTerminalPhase(phase remediationv1alpha1.RemediationPhase) bool {
    switch phase {
    case remediationv1alpha1.PhaseCompleted,
        remediationv1alpha1.PhaseFailed,
        remediationv1alpha1.PhaseSkipped,
        remediationv1alpha1.PhaseTimedOut:
        return true
    default:
        return false
    }
}
```

---

## 📊 Status Update Patterns

### Complete Status Update with Conditions

```go
// pkg/orchestrator/status/updater.go
func (u *StatusUpdater) UpdatePhase(ctx context.Context, rr *remediationv1alpha1.RemediationRequest, newPhase remediationv1alpha1.RemediationPhase, reason string) error {
    log := u.log.WithValues("remediationRequest", rr.Name)

    oldPhase := rr.Status.Phase
    if oldPhase == newPhase {
        return nil // No change
    }

    // Update phase
    rr.Status.Phase = newPhase
    now := metav1.Now()
    rr.Status.PhaseStartTime = &now
    rr.Status.ObservedGeneration = rr.Generation
    rr.Status.LastUpdated = now

    // Update conditions
    condition := metav1.Condition{
        Type:               "Phase" + string(newPhase),
        Status:             metav1.ConditionTrue,
        ObservedGeneration: rr.Generation,
        LastTransitionTime: now,
        Reason:             reason,
        Message:            fmt.Sprintf("Transitioned from %s to %s", oldPhase, newPhase),
    }
    meta.SetStatusCondition(&rr.Status.Conditions, condition)

    // Set completion time for terminal states
    if isTerminalPhase(newPhase) {
        rr.Status.CompletionTime = &now

        // Calculate duration
        if rr.Status.StartTime != nil {
            duration := now.Sub(rr.Status.StartTime.Time)
            rr.Status.Duration = &metav1.Duration{Duration: duration}
        }
    }

    // Update status subresource
    if err := u.client.Status().Update(ctx, rr); err != nil {
        return fmt.Errorf("failed to update status: %w", err)
    }

    log.Info("Phase transition complete",
        "from", oldPhase,
        "to", newPhase,
        "reason", reason)

    u.metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(newPhase)).Inc()

    return nil
}
```

---

## 🔐 Finalizer Pattern

### Finalizer Implementation for Cleanup

```go
// pkg/orchestrator/lifecycle/finalizer.go
const finalizerName = "remediation.kubernaut.ai/cleanup"

func (h *FinalizerHandler) HandleFinalizer(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) (ctrl.Result, error) {
    log := h.log.WithValues("remediationRequest", rr.Name)

    // Check if resource is being deleted
    if !rr.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(rr, finalizerName) {
            // Perform cleanup
            if err := h.cleanupExternalResources(ctx, rr); err != nil {
                return ctrl.Result{}, err
            }

            // Remove finalizer
            controllerutil.RemoveFinalizer(rr, finalizerName)
            if err := h.client.Update(ctx, rr); err != nil {
                return ctrl.Result{}, err
            }

            log.Info("Finalizer removed after cleanup")
        }
        return ctrl.Result{}, nil
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(rr, finalizerName) {
        controllerutil.AddFinalizer(rr, finalizerName)
        if err := h.client.Update(ctx, rr); err != nil {
            return ctrl.Result{}, err
        }
        log.Info("Finalizer added")
    }

    return ctrl.Result{}, nil
}
```

---

## ⏱️ Exponential Backoff Requeue

### Production-Ready Backoff Implementation

```go
// pkg/orchestrator/retry/backoff.go
import "math"

// calculateBackoff returns exponential backoff duration
// Attempts: 0→30s, 1→60s, 2→120s, 3→240s, 4+→480s (capped)
func calculateBackoff(attemptCount int) time.Duration {
    baseDelay := 30 * time.Second
    maxDelay := 480 * time.Second

    // Calculate exponential backoff: baseDelay * 2^attemptCount
    delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attemptCount)))

    // Cap at maximum delay
    if delay > maxDelay {
        delay = maxDelay
    }

    // Add jitter (±10%) to prevent thundering herd
    jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))

    return jitter
}

// Usage in Reconcile
func (r *Reconciler) handleRetryableError(ctx context.Context, rr *remediationv1alpha1.RemediationRequest, err error) (ctrl.Result, error) {
    rr.Status.AttemptCount++
    backoff := calculateBackoff(rr.Status.AttemptCount)

    r.log.Info("Transient error, requeueing",
        "attempt", rr.Status.AttemptCount,
        "backoff", backoff,
        "error", err)

    r.Metrics.RetriesTotal.WithLabelValues(rr.Name).Inc()

    return ctrl.Result{RequeueAfter: backoff}, nil
}
```

---

## 📈 Phase State Machine Pattern

### Phase Definitions

```go
// api/remediation/v1alpha1/phases.go
type RemediationPhase string

const (
    PhasePending          RemediationPhase = "Pending"
    PhaseProcessing       RemediationPhase = "Processing"
    PhaseAnalyzing        RemediationPhase = "Analyzing"
    PhaseAwaitingApproval RemediationPhase = "AwaitingApproval"
    PhaseExecuting        RemediationPhase = "Executing"
    PhaseCompleted        RemediationPhase = "Completed"
    PhaseFailed           RemediationPhase = "Failed"
    PhaseSkipped          RemediationPhase = "Skipped"
    PhaseTimedOut         RemediationPhase = "TimedOut"
)

// ValidTransitions defines allowed phase transitions
var ValidTransitions = map[RemediationPhase][]RemediationPhase{
    PhasePending:          {PhaseProcessing, PhaseFailed, PhaseTimedOut},
    PhaseProcessing:       {PhaseAnalyzing, PhaseFailed, PhaseTimedOut},
    PhaseAnalyzing:        {PhaseAwaitingApproval, PhaseExecuting, PhaseFailed, PhaseTimedOut},
    PhaseAwaitingApproval: {PhaseExecuting, PhaseFailed, PhaseTimedOut},
    PhaseExecuting:        {PhaseCompleted, PhaseFailed, PhaseSkipped, PhaseTimedOut},
    // Terminal states have no valid transitions
    PhaseCompleted: {},
    PhaseFailed:    {},
    PhaseSkipped:   {},
    PhaseTimedOut:  {},
}

// IsValidTransition checks if transition is allowed
func IsValidTransition(from, to RemediationPhase) bool {
    validNext, ok := ValidTransitions[from]
    if !ok {
        return false
    }
    for _, phase := range validNext {
        if phase == to {
            return true
        }
    }
    return false
}
```

---

## 🧪 Unit Test Pattern (Fake K8s Client)

### ADR-004 Compliant Testing

```go
// test/unit/remediationorchestrator/reconciler_test.go
var _ = Describe("RemediationOrchestrator Reconciler", func() {
    var (
        fakeClient client.Client
        reconciler *RemediationOrchestratorReconciler
        scheme     *runtime.Scheme
        ctx        context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        scheme = runtime.NewScheme()

        // Register all schemes
        _ = remediationv1alpha1.AddToScheme(scheme)
        _ = signalprocessingv1alpha1.AddToScheme(scheme)
        _ = aianalysisv1alpha1.AddToScheme(scheme)
        _ = workflowexecutionv1alpha1.AddToScheme(scheme)
        _ = notificationv1alpha1.AddToScheme(scheme)

        fakeClient = fake.NewClientBuilder().
            WithScheme(scheme).
            Build()

        reconciler = &RemediationOrchestratorReconciler{
            Client: fakeClient,
            Scheme: scheme,
            // Initialize other components...
        }
    })

    Context("when RemediationRequest is created", func() {
        It("should transition to Processing phase", func() {
            rr := &remediationv1alpha1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-rr",
                    Namespace: "default",
                },
                Spec: remediationv1alpha1.RemediationRequestSpec{
                    AlertData: remediationv1alpha1.AlertData{
                        Fingerprint: "test-fingerprint",
                    },
                },
            }

            Expect(fakeClient.Create(ctx, rr)).To(Succeed())

            result, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: types.NamespacedName{
                    Name:      rr.Name,
                    Namespace: rr.Namespace,
                },
            })

            Expect(err).ToNot(HaveOccurred())
            Expect(result.Requeue).To(BeTrue())

            // Verify phase transition
            updated := &remediationv1alpha1.RemediationRequest{}
            Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(rr), updated)).To(Succeed())
            Expect(updated.Status.Phase).To(Equal(remediationv1alpha1.PhaseProcessing))
        })
    })
})
```

---

## 📋 RBAC Configuration

### Complete RBAC Markers

```go
// Main CRD permissions
//+kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests/finalizers,verbs=update

// Child CRD permissions
//+kubebuilder:rbac:groups=signalprocessing.kubernaut.ai,resources=signalprocessings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.ai,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=notification.kubernaut.ai,resources=notificationrequests,verbs=get;list;watch;create;update;patch;delete

// Core K8s permissions
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
```

---

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](../IMPLEMENTATION_PLAN_V1.1.md)

