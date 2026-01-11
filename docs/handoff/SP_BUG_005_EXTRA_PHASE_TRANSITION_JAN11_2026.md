# SP-BUG-005: Extra Phase Transition Audit Event (5 instead of 4)

**Date**: January 11, 2026
**Status**: ⚠️ **DOCUMENTED - Low Priority**
**Priority**: P2 (Low Impact - Extra audit event doesn't affect functionality)
**Test**: `test/integration/signalprocessing/audit_integration_test.go:549`

---

## 🎯 **Executive Summary**

SignalProcessing integration test expects exactly 4 phase transition audit events but finds 5. The extra transition is caused by a `default` case in the phase switch statement that transitions unknown phases to `Enriching`.

**Impact**: Minor - One extra audit event per SignalProcessing resource. Does not affect functionality, only audit trail accuracy.

**Root Cause**: Switch statement has a `default` case that transitions any unhandled phase to `PhaseEnriching`, which can trigger under specific timing conditions.

---

## 🐛 **Test Failure**

### **Symptom**

```
[FAILED] BR-SP-090: MUST emit exactly 4 phase transitions: Pending→Enriching→Classifying→Categorizing→Completed
Expected
    <int>: 5
to equal
    <int>: 4
```

### **Expected Behavior**

For 5 phases (Pending, Enriching, Classifying, Categorizing, Completed), there should be **4 transitions**:
1. Pending → Enriching
2. Enriching → Classifying
3. Classifying → Categorizing
4. Categorizing → Completed

---

## 🔍 **Root Cause Analysis**

### **Code Investigation**

**Switch statement** (`internal/controller/signalprocessing/signalprocessing_controller.go:195-224`):

```go
switch sp.Status.Phase {
case signalprocessingv1alpha1.PhasePending:
    result, err = r.reconcilePending(ctx, sp, logger)      // Line 197
case signalprocessingv1alpha1.PhaseEnriching:
    result, err = r.reconcileEnriching(ctx, sp, logger)    // Line 199
case signalprocessingv1alpha1.PhaseClassifying:
    result, err = r.reconcileClassifying(ctx, sp, logger)  // Line 201
case signalprocessingv1alpha1.PhaseCategorizing:
    result, err = r.reconcileCategorizing(ctx, sp, logger) // Line 203
default:
    // Unknown phase - transition to enriching (DD-PERF-001)
    oldPhase := sp.Status.Phase                            // Line 206
    err := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
        sp.Status.Phase = signalprocessingv1alpha1.PhaseEnriching  // Line 212
        return nil
    })
    // ...
    if err := r.recordPhaseTransitionAudit(ctx, sp, string(oldPhase), string(signalprocessingv1alpha1.PhaseEnriching)); err != nil {
        return ctrl.Result{}, err  // Line 222 - EXTRA TRANSITION RECORDED
    }
}
```

### **Two Paths to PhaseEnriching**

| Path | Location | Transition | When Triggered |
|------|----------|-----------|----------------|
| **Path 1** | Line 271 (`reconcilePending`) | Pending → Enriching | Normal flow |
| **Path 2** | Line 222 (`default` case) | Unknown → Enriching | Edge case/race condition |

---

## ⚠️ **When Does Default Case Execute?**

### **Possible Scenarios**

1. **Race Condition**: K8s cache hasn't updated yet after phase initialization
2. **Corrupted State**: Phase value somehow becomes invalid
3. **Test Artifact**: Test creates resource with unexpected initial phase
4. **Timing Issue**: Phase is read between initialization and first reconciliation

### **Investigation Needed**

To confirm which scenario, add logging:

```go
default:
    logger.Info("⚠️ DEFAULT CASE TRIGGERED",
        "oldPhase", sp.Status.Phase,
        "resourceVersion", sp.ResourceVersion,
        "generation", sp.Generation)
    // ... rest of default case
```

---

## 🛠️ **Proposed Fixes**

### **Option A: Remove Default Case (Recommended)**

**Reasoning**: All valid phases are handled in the switch. Default case should never execute in normal operation.

```go
switch sp.Status.Phase {
case signalprocessingv1alpha1.PhasePending:
    result, err = r.reconcilePending(ctx, sp, logger)
case signalprocessingv1alpha1.PhaseEnriching:
    result, err = r.reconcileEnriching(ctx, sp, logger)
case signalprocessingv1alpha1.PhaseClassifying:
    result, err = r.reconcileClassifying(ctx, sp, logger)
case signalprocessingv1alpha1.PhaseCategorizing:
    result, err = r.reconcileCategorizing(ctx, sp, logger)
default:
    // Unexpected phase - log error and requeue
    logger.Error(fmt.Errorf("unexpected phase: %s", sp.Status.Phase), "Unknown phase encountered")
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil  // ❌ NO AUDIT, JUST REQUEUE
}
```

**Pros**:
- ✅ Eliminates extra audit event
- ✅ Makes unexpected phases visible through error logs
- ✅ Simpler code path

**Cons**:
- ⚠️ Doesn't automatically recover from corrupted phase state

---

### **Option B: Add Idempotency Check to Default Case**

**Reasoning**: Keep default case for safety, but prevent duplicate transition if already Enriching.

```go
default:
    // Unknown phase - transition to enriching (DD-PERF-001)
    oldPhase := sp.Status.Phase

    // SP-BUG-005: Prevent duplicate transition if already Enriching
    if oldPhase == string(signalprocessingv1alpha1.PhaseEnriching) {
        logger.V(1).Info("Already in Enriching phase, skipping default transition")
        return ctrl.Result{Requeue: true}, nil
    }

    err := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
        sp.Status.Phase = signalprocessingv1alpha1.PhaseEnriching
        return nil
    })
    // ... rest of default case
```

**Pros**:
- ✅ Keeps safety net for unexpected phases
- ✅ Prevents duplicate Enriching → Enriching transition

**Cons**:
- ⚠️ Still allows extra transition for other unexpected phases
- ⚠️ More complex logic

---

### **Option C: Update Test Expectation (Quick Fix)**

**Reasoning**: Accept that 5 transitions can occur under specific conditions.

```go
// BEFORE
Expect(phaseTransitionCount).To(Equal(4),
    "BR-SP-090: MUST emit exactly 4 phase transitions")

// AFTER
Expect(phaseTransitionCount).To(BeNumerically(">=", 4),
    "BR-SP-090: MUST emit at least 4 phase transitions (5 if default case triggered)")
Expect(phaseTransitionCount).To(BeNumerically("<=", 5),
    "BR-SP-090: Should not emit more than 5 phase transitions")
```

**Pros**:
- ✅ Quick fix, unblocks tests immediately
- ✅ No code changes to controller

**Cons**:
- ❌ Violates DD-TESTING-001 (deterministic validation)
- ❌ Doesn't address root cause
- ❌ Test becomes less strict

---

## 🎯 **Recommended Action**

**Option A** (Remove default case) is recommended because:
1. All valid phases are handled in the switch
2. Simpler code is easier to maintain
3. Error logging makes unexpected phases visible
4. Eliminates the extra audit event

**Implementation Priority**: P2 (Low) - Does not block functionality, only affects audit trail accuracy.

---

## 📊 **Current Test Status**

| Service | Overall Pass Rate | Failing Test | Impact |
|---------|------------------|--------------|--------|
| **SignalProcessing** | 81/82 (98.8%) | phase.transition count | 1 extra audit event |
| **AIAnalysis** | 55/57 (96.5%) | N/A | Idempotency bugs |
| **Gateway** | 10/10 (100%) | N/A | Clean |
| **DataStorage** | 686/692 (99.1%) | N/A | Business logic bugs |

---

## 🔗 **Related Issues**

- **SP-BUG-ENRICHMENT-001**: Duplicate enrichment.completed events (✅ FIXED)
- **SP-BUG-002**: Phase transition idempotency guard (✅ IMPLEMENTED at line 1179)
- **DD-TESTING-001**: Audit validation standards (Pattern 6 added)

---

## 📝 **Acceptance Criteria for Fix**

- [ ] Integration test `should create 'phase.transition' audit events` passes with exactly 4 transitions
- [ ] No extra audit events in normal operation
- [ ] Unexpected phases are logged as errors
- [ ] SignalProcessing overall pass rate: 82/82 (100%)

---

## 🚀 **Next Steps**

1. **Investigate**: Add logging to default case to confirm when it triggers
2. **Fix**: Implement Option A (remove default case, add error logging)
3. **Test**: Verify integration test passes with 4 transitions
4. **Document**: Update this document with findings

---

**Status**: ⚠️ Documented - Ready for implementation when priority permits
**Assigned To**: [Unassigned]
**Estimated Effort**: 30 minutes (investigation + fix + test)

