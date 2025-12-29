# Day 5: Critical Integration - START

**Date**: December 15, 2025
**Phase**: Integration
**Status**: 🚧 **IN PROGRESS**
**Priority**: 🚨 **CRITICAL - BLOCKING ALL DOWNSTREAM WORK**

---

## 🎯 **Objective**

**Integrate routing logic into the reconciler** - this is the missing link that makes V1.0 centralized routing actually functional.

**Current State**: Routing code exists but is **never called** from the reconciler
**Target State**: Reconciler calls routing logic before creating WorkflowExecution

---

## 🚨 **Why This is Critical**

From the audit (`docs/handoff/TRIAGE_V1.0_RO_CENTRALIZED_ROUTING_COMPLETE_AUDIT.md`):

> **Critical Finding**: 🚨 **Routing code exists but is NOT integrated into the reconciler** - the logic is never called!
> **Impact**: V1.0 centralized routing is NOT functional

**This is a show-stopper**: Without integration, all the routing work from Days 1-4 is unused.

---

## 📋 **Day 5 Tasks**

### **Task 5.1: Instantiate RoutingEngine in Reconciler** (30 min)

**File**: `pkg/remediationorchestrator/controller/reconciler.go`

**Changes**:
```go
type Reconciler struct {
    client.Client
    Scheme *runtime.Scheme
    // ... existing fields ...

    // NEW: Routing engine for centralized routing decisions (DD-RO-002)
    routingEngine *routing.RoutingEngine
}

// NewReconciler creates a new Reconciler with routing engine
func NewReconciler(client client.Client, scheme *runtime.Scheme, config Config) *Reconciler {
    routingConfig := routing.Config{
        ConsecutiveFailureThreshold:    config.ConsecutiveFailureThreshold,
        ConsecutiveFailureCooldown:     config.ConsecutiveFailureCooldown,
        RecentlyRemediatedCooldown:     config.RecentlyRemediatedCooldown,
        DuplicateInProgressRequeueTime: config.DuplicateInProgressRequeueTime,
        ResourceBusyRequeueTime:        config.ResourceBusyRequeueTime,
    }

    return &Reconciler{
        Client:        client,
        Scheme:        scheme,
        routingEngine: routing.NewRoutingEngine(client, config.Namespace, routingConfig),
    }
}
```

---

### **Task 5.2: Integrate Routing into `reconcileAnalyzing()`** (2 hours)

**File**: `pkg/remediationorchestrator/controller/reconciler.go`

**Current State** (line ~600-700):
```go
func (r *Reconciler) reconcileAnalyzing(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) (ctrl.Result, error) {
    // ... creates SignalProcessing, AIAnalysis ...

    // 🚨 MISSING: Routing logic is NOT called here!

    // Currently goes straight to WFE creation
    return r.createWorkflowExecution(ctx, rr, ai)
}
```

**Target State**:
```go
func (r *Reconciler) reconcileAnalyzing(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // ... creates SignalProcessing, AIAnalysis ...

    // NEW: Check routing conditions (DD-RO-002)
    logger.Info("Checking routing conditions before workflow execution")
    blocked, err := r.routingEngine.CheckBlockingConditions(ctx, rr)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to check routing conditions: %w", err)
    }

    // If blocked, update status and requeue
    if blocked != nil {
        logger.Info("Remediation blocked",
            "reason", blocked.Reason,
            "message", blocked.Message,
            "requeueAfter", blocked.RequeueAfter)
        return r.handleBlocked(ctx, rr, blocked)
    }

    // Not blocked - create WorkflowExecution
    logger.Info("Routing checks passed - creating WorkflowExecution")
    return r.createWorkflowExecution(ctx, rr, ai)
}
```

---

### **Task 5.3: Implement `handleBlocked()` Helper** (1 hour)

**File**: `pkg/remediationorchestrator/controller/reconciler.go`

**Implementation**:
```go
// handleBlocked updates RR status when routing is blocked and requeues appropriately.
// Reference: DD-RO-002, DD-RO-002-ADDENDUM (Blocked Phase Semantics)
func (r *Reconciler) handleBlocked(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    blocked *routing.BlockingCondition,
) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // Update RR status to Blocked phase
    err := helpers.UpdateRemediationRequestStatus(ctx, r.Client, rr, func(rr *remediationv1.RemediationRequest) error {
        rr.Status.OverallPhase = remediationv1.PhaseBlocked
        rr.Status.BlockReason = blocked.Reason
        rr.Status.BlockMessage = blocked.Message

        // Set time-based block fields
        if blocked.BlockedUntil != nil {
            rr.Status.BlockedUntil = &metav1.Time{Time: *blocked.BlockedUntil}
        }

        // Set WFE-based block fields
        if blocked.BlockingWorkflowExecution != "" {
            rr.Status.BlockingWorkflowExecution = blocked.BlockingWorkflowExecution
        }

        // Set duplicate tracking
        if blocked.DuplicateOf != "" {
            rr.Status.DuplicateOf = blocked.DuplicateOf
        }

        // Add condition
        meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
            Type:    "Blocked",
            Status:  metav1.ConditionTrue,
            Reason:  blocked.Reason,
            Message: blocked.Message,
        })

        return nil
    })

    if err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to update blocked status: %w", err)
    }

    // Emit metrics
    metrics.RemediationRequestBlockedTotal.
        WithLabelValues(rr.Namespace, blocked.Reason).Inc()

    logger.Info("RemediationRequest blocked",
        "reason", blocked.Reason,
        "requeueAfter", blocked.RequeueAfter)

    // Requeue after specified duration
    return ctrl.Result{RequeueAfter: blocked.RequeueAfter}, nil
}
```

---

### **Task 5.4: Add Routing Metrics** (30 min)

**File**: `pkg/remediationorchestrator/metrics/metrics.go`

**New Metrics**:
```go
var (
    // ... existing metrics ...

    // RemediationRequestBlockedTotal counts blocked remediations by reason
    RemediationRequestBlockedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "remediationrequest_blocked_total",
            Help: "Total number of blocked RemediationRequests by block reason",
        },
        []string{"namespace", "reason"},
    )

    // RemediationRequestBlockedDuration tracks how long RRs stay blocked
    RemediationRequestBlockedDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "remediationrequest_blocked_duration_seconds",
            Help:    "Duration RemediationRequests spend in Blocked phase",
            Buckets: []float64{30, 60, 300, 900, 1800, 3600, 7200}, // 30s to 2h
        },
        []string{"namespace", "reason"},
    )
)

func init() {
    metrics.Registry.MustRegister(
        RemediationRequestBlockedTotal,
        RemediationRequestBlockedDuration,
    )
}
```

---

### **Task 5.5: Update Config Struct** (15 min)

**File**: `pkg/remediationorchestrator/controller/config.go` (or similar)

**Add Routing Config**:
```go
type Config struct {
    // ... existing fields ...

    // Routing configuration (DD-RO-002)
    ConsecutiveFailureThreshold    int   // Default: 3 (from BR-ORCH-042)
    ConsecutiveFailureCooldown     int64 // Default: 3600 (1 hour in seconds)
    RecentlyRemediatedCooldown     int64 // Default: 300 (5 minutes in seconds)
    DuplicateInProgressRequeueTime int64 // Default: 30 (30 seconds)
    ResourceBusyRequeueTime        int64 // Default: 30 (30 seconds)
}
```

---

## ✅ **Success Criteria**

Day 5 is complete when:
- ✅ RoutingEngine instantiated in reconciler
- ✅ `reconcileAnalyzing()` calls `CheckBlockingConditions()`
- ✅ `handleBlocked()` function updates RR status correctly
- ✅ Requeue logic works for `BlockedUntil`
- ✅ Routing metrics emit correctly
- ✅ Manual testing shows routing decisions being made

---

## 🧪 **Testing Strategy**

### **Unit Tests** (Task 5.6: 2 hours)
- Test `handleBlocked()` populates all fields correctly
- Test requeue logic calculates correct durations
- Test metrics are emitted

### **Integration Tests** (Days 8-9)
- Test RO-WE integration with routing
- Test Gateway deduplication with `PhaseBlocked`
- Test cooldown expiry and retry

---

## 📊 **Timeline**

| Task | Duration | Status |
|------|----------|--------|
| 5.1: Instantiate RoutingEngine | 30 min | 🚧 NEXT |
| 5.2: Integrate into reconciler | 2 hours | ⏸️ PENDING |
| 5.3: Implement `handleBlocked()` | 1 hour | ⏸️ PENDING |
| 5.4: Add routing metrics | 30 min | ⏸️ PENDING |
| 5.5: Update config | 15 min | ⏸️ PENDING |
| 5.6: Unit tests | 2 hours | ⏸️ PENDING |
| **Total** | **6.5 hours** | **0% Complete** |

---

## 🚨 **Immediate Next Action**

**Start Task 5.1**: Find the reconciler file and add RoutingEngine field

```bash
# Find the reconciler
find pkg/remediationorchestrator -name "*.go" -exec grep -l "type Reconciler struct" {} \;
```

---

**Status**: 🚧 **STARTING DAY 5 INTEGRATION NOW**
**Priority**: 🚨 **CRITICAL**
**Next Step**: Task 5.1 - Instantiate RoutingEngine

---

**Let's close the critical gap and make V1.0 functional!**




