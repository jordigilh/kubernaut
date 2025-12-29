# SP Controller Maturity Gaps Triage

**Date**: December 19, 2025
**Compared Against**: WorkflowExecution (WE), AIAnalysis (AA), Notification (NOT)
**Status**: 🚨 **CRITICAL GAPS IDENTIFIED**

---

## Executive Summary

The SignalProcessing controller has significant gaps compared to mature controllers (WE, AA, NOT). These gaps affect observability, debugging, and production readiness.

| Gap Category | Severity | SP Status | WE/AA/NOT Status |
|---|---|---|---|
| **Metrics in Controller** | 🔴 CRITICAL | ❌ Not wired | ✅ Uses metrics |
| **EventRecorder** | 🔴 CRITICAL | ❌ Missing | ✅ Present |
| **Predicates** | 🟡 HIGH | ❌ Missing | ✅ Uses predicates |
| **Controller Logger Field** | 🟡 HIGH | ❌ Uses inline | ✅ Struct field |
| **Metrics Registration** | 🔴 CRITICAL | ❌ Not registered | ✅ `init()` registered |

---

## 🔴 CRITICAL Gap 1: Controller Does Not Use Metrics

### Current State (SP)
```go
type SignalProcessingReconciler struct {
    client.Client
    Scheme      *runtime.Scheme
    AuditClient *audit.AuditClient
    // ... NO Metrics field!
}
```

### Mature Pattern (WE)
```go
// internal/controller/workflowexecution/metrics.go
var (
    WorkflowExecutionTotal = prometheus.NewCounterVec(...)
    WorkflowExecutionDuration = prometheus.NewHistogramVec(...)
)

func init() {
    metrics.Registry.MustRegister(
        WorkflowExecutionTotal,
        WorkflowExecutionDuration,
    )
}
```

**Controller uses metrics:**
```go
// On completion
workflowexecution.RecordWorkflowCompletion(durationSeconds)
// On failure
workflowexecution.RecordWorkflowFailure(durationSeconds)
```

### Gap Analysis
- SP has `pkg/signalprocessing/metrics/metrics.go` ✅
- SP instantiates metrics in main.go ✅
- SP passes metrics to K8sEnricher ✅
- **SP controller does NOT call metrics!** ❌
- **SP metrics are NOT registered with controller-runtime!** ❌

### Fix Required
1. Add `Metrics *metrics.Metrics` to `SignalProcessingReconciler` struct
2. Wire metrics from main.go
3. Call metrics in reconciliation phases
4. Register metrics with `metrics.Registry` (controller-runtime)

---

## 🔴 CRITICAL Gap 2: No EventRecorder

### Current State (SP)
```go
type SignalProcessingReconciler struct {
    // NO Recorder field!
}
```

### Mature Pattern (WE, AA)
```go
type WorkflowExecutionReconciler struct {
    Recorder record.EventRecorder  // ✅
}

type AIAnalysisReconciler struct {
    Recorder record.EventRecorder  // ✅
}
```

**Usage:**
```go
r.Recorder.Event(obj, corev1.EventTypeWarning, "ProcessingFailed", err.Error())
```

### Gap Analysis
- SP cannot emit Kubernetes Events for debugging
- `kubectl describe signalprocessing` shows no events
- Operations team cannot diagnose issues without logs

### Fix Required
1. Add `Recorder record.EventRecorder` to struct
2. Wire via `mgr.GetEventRecorderFor("signalprocessing-controller")`
3. Emit events on phase transitions and errors

---

## 🟡 HIGH Gap 3: No Predicates (Event Filtering)

### Current State (SP)
```go
func (r *SignalProcessingReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&signalprocessingv1alpha1.SignalProcessing{}).
        Named(fmt.Sprintf("signalprocessing-%s", "controller")).
        Complete(r)
    // NO predicates!
}
```

### Mature Pattern (AA, NOT)
```go
// AIAnalysis
func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&aianalysisv1.AIAnalysis{}).
        WithEventFilter(predicate.GenerationChangedPredicate{}).  // ✅
        Complete(r)
}
```

### Gap Analysis
- SP reconciles on every status update (inefficient)
- Generates unnecessary reconciliation loops
- Higher CPU/memory usage

### Fix Required
Add `predicate.GenerationChangedPredicate{}` to filter status-only updates.

---

## 🟡 HIGH Gap 4: No Logger Field

### Current State (SP)
```go
func (r *SignalProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := ctrl.Log.WithValues("signalprocessing", req.NamespacedName)
    // Creates new logger every reconcile
}
```

### Mature Pattern (AA)
```go
type AIAnalysisReconciler struct {
    Log logr.Logger  // Reusable logger
}

func (r *AIAnalysisReconciler) Reconcile(...) {
    log := r.Log.WithValues("aianalysis", req.NamespacedName)
}
```

### Fix Required
Add `Log logr.Logger` to struct, wire from main.go.

---

## 🔴 CRITICAL Gap 5: Metrics Not Registered with Controller-Runtime

### Current State (SP)
```go
// pkg/signalprocessing/metrics/metrics.go
func NewMetrics(registry *prometheus.Registry) *Metrics {
    // Uses passed registry
}
```

### Mature Pattern (WE, NOT)
```go
// internal/controller/workflowexecution/metrics.go
func init() {
    metrics.Registry.MustRegister(...)  // Uses controller-runtime registry
}
```

### Gap Analysis
- SP uses custom registry, not controller-runtime's
- Metrics may not appear on `/metrics` endpoint
- Prometheus scraping may fail

### Fix Required
Use `sigs.k8s.io/controller-runtime/pkg/metrics.Registry`.

---

## Comparison Matrix

| Feature | SP | WE | AA | NOT |
|---|---|---|---|---|
| **Metrics in controller** | ❌ | ✅ | 🟡 | ✅ |
| **EventRecorder** | ❌ | ✅ | ✅ | ❌ |
| **Predicates** | ❌ | ✅ | ✅ | ✅ |
| **Logger field** | ❌ | 🟡 | ✅ | 🟡 |
| **Metrics registered** | ❌ | ✅ | 🟡 | ✅ |
| **Healthz probes** | ✅ | ✅ | ✅ | ✅ |
| **Graceful shutdown** | ✅ | ✅ | ✅ | ✅ |
| **Audit integration** | ✅ | ✅ | ✅ | ✅ |

---

## Recommended Fix Priority

### P0 (Blockers for V1.0)
1. **Wire metrics to controller** - Required for SLO monitoring
2. **Register metrics with controller-runtime** - Required for Prometheus

### P1 (High Priority)
3. **Add EventRecorder** - Required for kubectl debugging
4. **Add predicates** - Reduces unnecessary reconciliation

### P2 (Medium Priority)
5. **Add Logger field** - Code cleanup

---

## Implementation Estimate

| Task | Effort | Risk |
|---|---|---|
| Wire metrics to controller | 30 min | Low |
| Add EventRecorder | 20 min | Low |
| Add predicates | 10 min | Low |
| Add Logger field | 15 min | Low |
| Fix metrics registration | 20 min | Medium |
| **Total** | **~1.5 hours** | **Low-Medium** |

---

## Next Steps

1. **Immediate**: Fix P0 blockers (metrics wiring + registration)
2. **Before V1.0**: Fix P1 items (EventRecorder, predicates)
3. **Post-V1.0**: Fix P2 items (Logger field)

---

## References

- WE Controller: `internal/controller/workflowexecution/`
- AA Controller: `internal/controller/aianalysis/`
- NOT Controller: `internal/controller/notification/`
- SP Metrics Spec: `docs/services/crd-controllers/01-signalprocessing/metrics-slos.md`

