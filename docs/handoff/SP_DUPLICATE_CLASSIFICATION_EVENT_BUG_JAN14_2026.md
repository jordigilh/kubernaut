# SignalProcessing Duplicate Classification Event Bug - Fix Required

**Date**: 2026-01-14
**Status**: 🔴 BUG CONFIRMED
**Priority**: P1 - Violates BR-SP-090 Audit Standards
**Related**: `docs/handoff/SP_FINAL_ROOT_CAUSE_MULTIPLE_EMISSIONS_JAN14_2026.md`

---

## 📋 Executive Summary

**AUTHORITATIVE DOCUMENTATION**: `docs/handoff/SP_AUDIT_TESTS_DD_TESTING_001_TRIAGE_JAN_03_2026.md` (Line 84-87)

```go
}, 90*time.Second, 500*time.Millisecond).Should(Equal(1),
    "BR-SP-090: SignalProcessing MUST emit exactly 1 classification.decision event per classification")
```

**Rationale**: **"One classification decision = one audit event."**

**BUG**: Controller emits `classification.decision` event **TWICE**:
1. ✅ During Classifying phase (line 576) - **CORRECT**
2. ❌ During Completed phase in `recordCompletionAudit()` (line 1255) - **DUPLICATE BUG**

---

## 🔍 Bug Evidence

### Authoritative Documentation (DD-TESTING-001)

```markdown
### **Violation 1.2** (Line 295)

**Test**: "should create 'classification.decision' audit event with all categorization results"

**Required Fix**:
}, 90*time.Second, 500*time.Millisecond).Should(Equal(1),
    "BR-SP-090: SignalProcessing MUST emit exactly 1 classification.decision event per classification")

**Rationale**: One classification decision = one audit event.
```

**Source**: `docs/handoff/SP_AUDIT_TESTS_DD_TESTING_001_TRIAGE_JAN_03_2026.md` (Lines 71-88)

---

### Controller Code (DUPLICATE EMISSION)

#### Emission Point #1 (✅ CORRECT):

```go
// Line 576: internal/controller/signalprocessing/signalprocessing_controller.go
// Record classification decision audit event (BR-SP-105, DD-SEVERITY-001)
// Must be called after atomic status update to include normalized severity
if r.AuditClient != nil && severityResult != nil {
    logger.V(1).Info("DEBUG: Emitting classification.decision audit event",
        "severityResult", severityResult.Severity)
    r.AuditClient.RecordClassificationDecision(ctx, sp)  // ✅ CORRECT: Emit during classification
}
```

**Location**: Classifying phase, after severity determination
**Purpose**: Record the classification decision with normalized severity
**Status**: **CORRECT** - should remain

---

#### Emission Point #2 (❌ DUPLICATE BUG):

```go
// Line 1255: internal/controller/signalprocessing/signalprocessing_controller.go
func (r *SignalProcessingReconciler) recordCompletionAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) error {
    if r.AuditClient == nil {
        return fmt.Errorf("AuditClient is nil - audit is MANDATORY per ADR-032")
    }
    r.AuditClient.RecordSignalProcessed(ctx, sp)
    r.AuditClient.RecordClassificationDecision(ctx, sp)  // ❌ DUPLICATE: Already emitted at line 576!
    r.AuditClient.RecordBusinessClassification(ctx, sp)
    return nil
}
```

**Location**: Completed phase, during completion audit
**Purpose**: Record final audit events
**Status**: **BUG** - `RecordClassificationDecision` should NOT be called here

---

### Test Failure Evidence

```log
# Unique correlation ID (no stale data):
correlation_id="test-policy-fallback-audit-rr-1768440104645983000"

# TWO classification.decision events emitted:
Event #1: total_buffered:15 (Classifying phase - line 576)
Event #2: total_buffered:17 (Completed phase - line 1255) ← DUPLICATE BUG

# Test expectation (per DD-TESTING-001):
Expected <int>: 1
Got <int>: 2 (or 3 if multiple reconciliations)
```

**Test Result**: FAIL after 60s timeout
**Pass Rate**: 96.6% (84/87 specs) - **3 failing tests, all affected by this bug**

---

## 🔧 Required Fix

### Fix: Remove Duplicate Emission from Completion Audit

**File**: `internal/controller/signalprocessing/signalprocessing_controller.go`
**Line**: 1255 (in `recordCompletionAudit()` function)

#### Before (BUGGY):

```go
func (r *SignalProcessingReconciler) recordCompletionAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) error {
    if r.AuditClient == nil {
        return fmt.Errorf("AuditClient is nil - audit is MANDATORY per ADR-032")
    }
    r.AuditClient.RecordSignalProcessed(ctx, sp)
    r.AuditClient.RecordClassificationDecision(ctx, sp)  // ❌ REMOVE: Duplicate emission
    r.AuditClient.RecordBusinessClassification(ctx, sp)
    return nil
}
```

#### After (FIXED):

```go
func (r *SignalProcessingReconciler) recordCompletionAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) error {
    if r.AuditClient == nil {
        return fmt.Errorf("AuditClient is nil - audit is MANDATORY per ADR-032")
    }
    r.AuditClient.RecordSignalProcessed(ctx, sp)
    // ✅ REMOVED: RecordClassificationDecision already emitted during Classifying phase (line 576)
    // Per DD-TESTING-001: "One classification decision = one audit event"
    r.AuditClient.RecordBusinessClassification(ctx, sp)
    return nil
}
```

---

### Update Function Comment

**Before**:
```go
// recordCompletionAudit records the final signal processed and classification decision audit events.
```

**After**:
```go
// recordCompletionAudit records the final signal processed and business classification audit events.
// NOTE: classification.decision event is emitted during Classifying phase (line 576), NOT here.
// Per DD-TESTING-001: "One classification decision = one audit event"
```

---

## 📊 Impact Assessment

### Affected Tests (All Will Pass After Fix)

1. **Line 337**: `should emit audit event with policy-defined fallback severity`
   - **Before**: Expected 1, Found 2-3 (FAIL after 60s)
   - **After**: Expected 1, Found 1 (PASS in ~2-4s)

2. **Line 213**: `should emit 'classification.decision' audit event with both external and normalized severity`
   - **Before**: Expected 1, Found 2-3 (INTERRUPTED)
   - **After**: Expected 1, Found 1 (PASS in ~2-4s)

3. **audit_integration_test.go:274**: `should create 'classification.decision' audit event with all categorization results`
   - **Before**: Expected 1, Found 2-3 (INTERRUPTED)
   - **After**: Expected 1, Found 1 (PASS in ~2-4s)

### Expected Test Results After Fix

```
Ran 92 of 92 Specs in ~90 seconds
PASS! -- 92 Passed | 0 Failed | 2 Pending | 0 Skipped
Pass Rate: 100% (92/92)
```

**Test Duration Improvement**: **93% faster** per test (60s → 4s)

---

## 🧪 Validation Plan

### Step 1: Apply Fix

```bash
# Edit internal/controller/signalprocessing/signalprocessing_controller.go
# Remove line 1255: r.AuditClient.RecordClassificationDecision(ctx, sp)
# Update function comment at line 1247
```

### Step 2: Run Integration Tests

```bash
make test-integration-signalprocessing
```

**Expected Results**:
- ✅ All 3 previously failing tests now pass
- ✅ All tests find exactly 1 `classification.decision` event per correlation ID
- ✅ Test duration <5s per test (not 60s)
- ✅ 100% pass rate (92/92 specs)

### Step 3: Verify Audit Event Count

```bash
# Run integration tests and check event counts
make test-integration-signalprocessing 2>&1 | tee /tmp/sp-duplicate-fix.log

# Verify exactly 1 classification.decision event per correlation ID
grep "✅ Found.*classification.decision" /tmp/sp-duplicate-fix.log | \
  grep -v "Found 1 event"

# Should return NO results (all should be "Found 1 event")
```

---

## 📝 Root Cause Analysis

### Why Was This Bug Introduced?

**Historical Context**: The `recordCompletionAudit()` function was likely created to emit ALL completion-related audit events in one place for simplicity.

**Problem**: `classification.decision` is NOT a completion event - it's a **classification phase event** that should be emitted immediately after classification occurs (line 576).

**Confusion**: The function name `recordCompletionAudit()` implies "emit all audit events at completion", but classification already happened earlier in the Classifying phase.

### Why Tests Didn't Catch It Initially

**Likely Scenario**: Tests initially used `BeNumerically(">=", 1)` (which passes with 1+ events), but DD-TESTING-001 documentation correctly required `Equal(1)` for precise validation.

**Evidence**: `docs/handoff/SP_AUDIT_TESTS_DD_TESTING_001_TRIAGE_JAN_03_2026.md` shows tests were updated from `BeNumerically(">=", 1)` to `Equal(1)` to detect duplicates.

---

## 🔗 Related Documents

1. **Authoritative Specification**: `docs/handoff/SP_AUDIT_TESTS_DD_TESTING_001_TRIAGE_JAN_03_2026.md`
   - Line 84-87: "One classification decision = one audit event"

2. **Root Cause Triage**: `docs/handoff/SP_FINAL_ROOT_CAUSE_MULTIPLE_EMISSIONS_JAN14_2026.md`
   - Identified duplicate emissions via log analysis

3. **Correlation ID Fix**: `docs/handoff/SP_UNIQUE_CORRELATION_ID_FIX_JAN14_2026.md`
   - Ensured unique correlation IDs to detect duplicates

4. **Business Requirements**: `BR-SP-090` (SignalProcessing audit event emission standards)

5. **Architecture Decision**: `DD-TESTING-001` (Audit event validation standards)

---

## ⏭️ Next Steps

1. [ ] Apply fix: Remove line 1255 in `signalprocessing_controller.go`
2. [ ] Update function comment at line 1247
3. [ ] Run integration tests to verify 100% pass rate
4. [ ] Verify exactly 1 event per classification in test logs
5. [ ] Commit fix with reference to this document

---

## ✅ Success Criteria

**Fix is successful if**:
1. ✅ All 3 failing tests pass (100% pass rate)
2. ✅ Each test finds exactly 1 `classification.decision` event per correlation ID
3. ✅ Test duration <5s per test (not 60s timeout)
4. ✅ No "Expected 1, got N" failures
5. ✅ Audit trail still contains all required events (signal.processed, business.classified)

---

**Confidence**: **99%**

**Justification**:
1. ✅ Authoritative documentation is explicit: "One classification decision = one audit event"
2. ✅ Duplicate emission point clearly identified in controller code
3. ✅ Fix is simple: Delete 1 line of code
4. ✅ No side effects: Other audit events remain unchanged
5. ✅ Test expectations already correct per DD-TESTING-001

**Risk**: **1%** - Minimal risk. Other audit events (signal.processed, business.classified) remain in `recordCompletionAudit()` and continue functioning correctly.

---

**Last Updated**: 2026-01-14T20:45:00
**Status**: 🔄 Awaiting fix implementation and validation
