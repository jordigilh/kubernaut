# RemediationOrchestrator Integration Test Triage - Jan 9, 2026

## 🎯 **Test Results Summary**

**Status**: 41/46 Passed (89%)
**Failures**: 5 audit-related tests
**Root Cause**: Missing audit events or incomplete event payloads

---

## 🔍 **Failure Analysis**

### **Category 1: Event Type Discriminator Bug (FIXED)**

**Test**: `approval_requested` missing `rar_name` field
**Root Cause**: Wrong `EventType` in payload discriminator
**Location**: `pkg/remediationorchestrator/audit/manager.go:326`

**Bug**:
```go
// ❌ BEFORE (line 326)
EventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleTransitioned,
```

**Fix Applied**:
```go
// ✅ AFTER
EventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorApprovalRequested,
```

**Status**: ✅ **FIXED** - Changed in `manager.go`

---

### **Category 2: Missing Lifecycle Completion Events (INVESTIGATION NEEDED)**

#### **Tests**:
1. `lifecycle_completed` - success case
2. `lifecycle_failed` - failure case with `failure_phase` field

#### **Findings**:

**Controller Code** (✅ CORRECT):
- Line 1572: Calls `BuildCompletionEvent` for success
- Line 1612: Calls `BuildFailureEvent` for failure
- Line 1625: Calls `StoreAudit(ctx, event)` for both

**Audit Manager Code** (✅ CORRECT):
- `BuildFailureEvent` (line 220-285) correctly sets `FailurePhase` field (line 278)
- `BuildCompletionEvent` (line 170-214) correctly sets completion fields

**Problem**: Events stored but not retrieved by tests

**Hypotheses**:
1. **Audit batch not flushed** - Similar to WorkflowExecution issue
2. **Controller not reconciled** - SP status change doesn't trigger RR reconciliation
3. **Event query timing** - Tests query too soon after status change

---

### **Category 3: Error Audit Standardization (NEW TESTS)**

#### **Tests**:
1. Gap #7 Scenario 1: Timeout configuration error
2. Gap #7 Scenario 2: Child CRD creation failure

#### **Status**: Not investigated yet (likely same root cause as Category 2)

---

## 🔧 **Proposed Solutions**

### **Solution A: Add Audit Flush Delay (Similar to WorkflowExecution)**

**Rationale**: WorkflowExecution E2E tests needed 3s delay after completion for audit batch flush

**Change**: Add delay after `PhaseFailed`/`PhaseCompleted` wait
```go
// In audit_emission_integration_test.go after line 332
Eventually(func() remediationv1.RemediationPhase {
    _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
    return rr.Status.OverallPhase
}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

// ADD THIS:
time.Sleep(3 * time.Second) // Wait for audit batch flush (1s flush interval + buffer)

// Then query audit events
```

**Pros**:
- ✅ Simple fix
- ✅ Proven pattern from WorkflowExecution
- ✅ Low risk

**Cons**:
- ⚠️ Adds 3s to each test (~15s total for 5 tests)
- ⚠️ Doesn't address root cause if it's a reconciliation issue

---

### **Solution B: Trigger Manual Reconciliation**

**Rationale**: Test manually updates SP status, might not trigger RR watch/reconciliation

**Change**: Explicitly update RR to trigger reconciliation
```go
// After updating SP status (line 326)
Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

// ADD THIS:
// Trigger RR reconciliation by updating an annotation
if rr.Annotations == nil {
    rr.Annotations = make(map[string]string)
}
rr.Annotations["test.trigger"] = fmt.Sprintf("%d", time.Now().UnixNano())
Expect(k8sClient.Update(ctx, rr)).To(Succeed())
```

**Pros**:
- ✅ Ensures controller processes the failure
- ✅ More robust test

**Cons**:
- ⚠️ More invasive change
- ⚠️ Might not be needed if Solution A works

---

### **Solution C: Increase Eventually Timeout**

**Rationale**: 10s might not be enough for full reconciliation + audit flush

**Change**: Increase timeout from 10s to 20s
```go
// Line 339-349
Eventually(func() int {
    allEvents := queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
    // ...
}, "20s", "500ms").Should(Equal(1), "Expected exactly 1 lifecycle_completed audit event")
```

**Pros**:
- ✅ Simple change
- ✅ Non-invasive

**Cons**:
- ⚠️ Masks the real issue
- ⚠️ Slows down test suite

---

## 📋 **Recommended Action Plan**

### **Phase 1: Quick Wins** (Apply immediately)

1. ✅ **DONE**: Fix `approval_requested` EventType discriminator
2. **TODO**: Run integration tests to verify fix #1
3. **TODO**: Apply Solution A (audit flush delay) to remaining failures
4. **TODO**: Run integration tests again

### **Phase 2: Verify** (If Phase 1 works)

1. Document audit flush delay pattern
2. Update test infrastructure to handle this automatically
3. Move to E2E tests

### **Phase 3: Deep Investigation** (If Phase 1 fails)

1. Add debug logging to controller reconciliation
2. Verify SP→RR watch/ownership is configured
3. Check if manual reconciliation trigger is needed (Solution B)
4. Investigate audit store flush mechanism

---

## 🧪 **Test Execution Plan**

### **Immediate Next Steps**:

```bash
# 1. Compile to verify fix #1
go test -c ./test/integration/remediationorchestrator/

# 2. Run just the approval_requested test
go test -v ./test/integration/remediationorchestrator/... -run "AE-INT-5"

# 3. If that passes, apply Solution A and run all failing tests
go test -v ./test/integration/remediationorchestrator/... -run "AE-INT-3|AE-INT-4|AE-INT-5|Gap"
```

---

## 📊 **Expected Outcomes**

### **After Fix #1 Only**:
- ✅ approval_requested test: PASS (1 more passing, 4 failing)
- ❌ lifecycle_completed: Still FAIL (needs Solution A)
- ❌ lifecycle_failed: Still FAIL (needs Solution A)
- ❌ Error audits: Still FAIL (needs Solution A)

**Total**: 42/46 (91%)

### **After Fix #1 + Solution A**:
- ✅ All 5 tests: PASS (if hypothesis is correct)

**Total**: 46/46 (100%) ← **TARGET**

---

## 🔗 **Related Issues**

- **WorkflowExecution**: Similar audit flush delay needed in E2E tests
- **DataStorage**: Audit store uses 1s flush interval (configurable via `AUDIT_FLUSH_INTERVAL`)
- **ADR-032 §1**: Mandatory audit requirement for all phase transitions

---

**Status**: ✅ Fix #1 applied, waiting for test verification
**Next Action**: Apply Solution A to remaining tests
**Confidence**: 85% - Solution A should fix all remaining failures based on WE precedent
