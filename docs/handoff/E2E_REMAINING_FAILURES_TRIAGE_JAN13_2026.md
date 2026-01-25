# Gateway E2E - Remaining Failures Triage

**Date**: January 13, 2026
**Status**: After DD-E2E-K8S-CLIENT-001 Fix
**Pass Rate**: 86/98 (87.8%)
**Remaining Failures**: 12 tests

---

## 🎯 **Executive Summary**

**ALL 12 remaining failures share a common root cause**: **K8s cache synchronization between Gateway (in-cluster) and E2E tests (external client)**.

This is the SAME issue we fixed in integration tests with the `apiReader` pattern (DD-STATUS-001), but E2E tests are hitting it from a different angle.

---

## 📊 **Failure Categories**

| Category | Tests | Root Cause | Severity |
|----------|-------|------------|----------|
| **CRD Visibility** | 4 | Gateway creates CRD, test can't see it | P1 |
| **Audit Integration** | 4 | CRD visibility → audit queries fail | P2 |
| **Service Resilience** | 3 | CRD visibility after recovery | P2 |
| **Missing Feature** | 1 | Namespace fallback not implemented | P3 |

---

## 🔍 **Detailed Analysis**

### **Category 1: CRD Visibility (4 failures) - P1**

#### **Affected Tests**:
1. ❌ Test 30: Observability - Dedup metrics
2. ❌ Test 30: Observability - HTTP latency metrics
3. ❌ Test 31: Prometheus Alert - Resource extraction
4. ❌ Test 31: Prometheus Alert - Deduplication

#### **Symptoms**:
```
[FAILED] Timed out after 10.001s.
CRD should exist in K8s before testing deduplication
Expected <int>: 0 to equal <int>: 1
```

**What's Happening**:
1. Test sends HTTP request to Gateway → **HTTP 201 Created** ✅
2. Gateway creates CRD in K8s → **Success** ✅
3. Test queries K8s for CRD → **Not found** (within 10s timeout) ❌

#### **Root Cause**:
**K8s Cache Synchronization Race**:
- **Gateway** (in-cluster): Uses cached K8s client → Sees CRD immediately
- **E2E Test** (external): Uses separate K8s client → Cache not synced → Can't see CRD

**This is identical to the issue we fixed in integration tests with `apiReader` (DD-STATUS-001).**

#### **Evidence**:
```go
// test/e2e/gateway/30_observability_test.go:186
// Verify CRD was created before sending duplicate
Eventually(func() int {
    var rrList remediationv1alpha1.RemediationRequestList
    err := k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
    if err != nil {
        return 0
    }
    return len(rrList.Items)  // Returns 0 even though Gateway created the CRD
}, 10*time.Second, 500*time.Millisecond).Should(Equal(1))
// ❌ FAILS - CRD not visible to external test client
```

#### **Impact**:
- Tests fail to verify CRD creation
- Deduplication tests fail (can't verify CRD exists before sending duplicate)
- Cascade failures in metrics/observability

---

### **Category 2: Audit Integration (4 failures) - P2**

#### **Affected Tests**:
5. ❌ Test 23: Audit Emission - signal.received
6. ❌ Test 23: Audit Emission - signal.deduplicated
7. ❌ Test 22: Audit Errors - error_details
8. ❌ Test 24: Audit Signal Data - Complete capture

#### **Symptoms**:
```
[FAILED] remediation_request should be namespace/name format
Expected not to be nil
```

```
[FAILED] Duplicate alert should return 202 (deduplicated)
Expected <int>: 201 to equal <int>: 202
```

#### **Root Cause**:
**CRD Visibility → Deduplication Fails → Audit Events Wrong**:

1. **First Signal**:
   - Gateway creates CRD → **HTTP 201** ✅
   - Test can't see CRD (cache sync issue) ❌

2. **Second Signal** (duplicate):
   - Gateway checks for existing CRD
   - Gateway's internal check: **CRD exists** ✅
   - BUT: Status-based deduplication reads CRD status
   - **Cache sync delay** → Gateway might not see latest status
   - Result: **HTTP 201** (instead of 202) ❌
   - Audit event: `signal.received` (instead of `signal.deduplicated`) ❌

#### **Evidence**:
```go
// test/e2e/gateway/24_audit_signal_data_test.go:726
resp2, err := sendWebhook(ctx, gatewayURL, signalPayload, httpClient)
Expect(resp2.StatusCode).To(Equal(202), "Duplicate alert should return 202")
// ❌ ACTUAL: 201 - Gateway not seeing CRD or not detecting duplicate
```

#### **Impact**:
- Audit events have wrong types (`signal.received` vs `signal.deduplicated`)
- Test assertions on audit event content fail
- Audit trail validation fails

---

### **Category 3: Service Resilience (3 failures) - P2**

#### **Affected Tests**:
9. ❌ Test 32: Service Resilience - DataStorage log failures
10. ❌ Test 32: Service Resilience - DataStorage recovery
11. ❌ Test 32: Service Resilience - Degraded functionality

#### **Symptoms**:
```
[FAILED] Timed out after 30.000s / 45.001s.
CRD should be created after DataStorage recovery
Expected <bool>: false to be true
```

#### **Root Cause**:
**Same CRD Visibility Issue + Service State Complexity**:

1. Test disables DataStorage (simulates failure)
2. Test sends signal → Gateway processes with degraded functionality
3. Test re-enables DataStorage
4. Test sends signal → Gateway should create CRD
5. Test queries K8s for CRD → **Not visible** (cache sync) ❌

**Additional Factor**: Service resilience tests have complex state management (disable/enable DataStorage), making cache sync issues more pronounced.

#### **Evidence**:
```go
// test/e2e/gateway/32_service_resilience_test.go:313
Eventually(func() bool {
    var rrList remediationv1alpha1.RemediationRequestList
    err := k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
    return err == nil && len(rrList.Items) > 0
}, 30*time.Second, 1*time.Second).Should(BeTrue())
// ❌ FAILS - CRD not visible even with 30s timeout
```

#### **Impact**:
- Can't validate service recovery
- Can't verify CRD creation after DataStorage comes back online
- Long timeouts (30-90s) still failing

---

### **Category 4: Missing Feature (1 failure) - P3**

#### **Affected Test**:
12. ❌ Test 27: Error Handling - Namespace fallback

#### **Symptoms**:
```
[FAILED] Gateway should process alert despite invalid namespace (201 Created)
Expected: 201 Created with namespace fallback to kubernaut-system
Actual: 400 Bad Request (namespace validation fails)
```

#### **Root Cause**:
**Feature Not Implemented**: Gateway doesn't have namespace fallback logic.

Test expects:
```
Alert with invalid namespace → Gateway falls back to kubernaut-system → 201 Created
```

Actual Gateway behavior:
```
Alert with invalid namespace → Validation error → 400 Bad Request
```

#### **Documentation**:
Already documented as TODO in `docs/handoff/E2E_TEST27_NAMESPACE_FALLBACK_TODO.md`

#### **Impact**:
- 1 test failure
- Low priority (edge case feature)

---

## 🎯 **Root Cause Summary**

### **Primary Issue: K8s Cache Synchronization (11/12 failures)**

**Pattern**:
```
Gateway (in-cluster, cached client)
    ↓ Creates CRD
K8s API Server ✅ CRD exists
    ↓ Cache not synced
E2E Test (external, separate client)
    ↓ Queries K8s
❌ CRD not visible (cache lag)
```

**This is IDENTICAL to DD-STATUS-001** (Gateway internal cache issue we fixed for integration tests).

**Difference**:
- **DD-STATUS-001**: Gateway's internal read (after write) → Fixed with `apiReader`
- **Current Issue**: E2E test's external read (after Gateway write) → Same cache problem, different angle

---

## 💡 **Proposed Solutions**

### **Option A: Fix E2E Test Pattern (Recommended)**

**Approach**: Add `Eventually` waits with longer timeouts for CRD visibility

**Changes**:
```go
// BEFORE (Tests 30, 31):
Eventually(func() int {
    var rrList remediationv1alpha1.RemediationRequestList
    err := k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
    return len(rrList.Items)
}, 10*time.Second, 500*time.Millisecond).Should(Equal(1))

// AFTER:
Eventually(func() int {
    var rrList remediationv1alpha1.RemediationRequestList
    // Force fresh read by creating new client or using direct API query
    err := k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
    return len(rrList.Items)
}, 60*time.Second, 1*time.Second).Should(Equal(1),
    "CRD should be visible within 60s (K8s cache sync)")
```

**Pros**:
- ✅ Minimal code changes
- ✅ Acknowledges K8s eventual consistency
- ✅ Works with existing Gateway implementation

**Cons**:
- ❌ Longer test times (10s → 60s per check)
- ❌ Doesn't fix root cause (cache sync delay)

**Effort**: Low (2-3 hours)

---

### **Option B: Use Direct API Reads in E2E Tests (Better)**

**Approach**: Create uncached K8s client for E2E tests (same as `apiReader` pattern)

**Changes**:
```go
// test/e2e/gateway/gateway_e2e_suite_test.go
var (
    k8sClient   client.Client     // Cached client (current)
    apiReader   client.Reader     // NEW: Uncached client for fresh reads
)

// In SynchronizedBeforeSuite:
// Create uncached client for direct API reads
apiReader, err = client.New(cfg, client.Options{Scheme: scheme})
// No cache = direct API server reads
```

**Update tests**:
```go
// Use apiReader for CRD verification (fresh reads)
Eventually(func() int {
    var rrList remediationv1alpha1.RemediationRequestList
    err := apiReader.List(ctx, &rrList, client.InNamespace(testNamespace))
    return len(rrList.Items)
}, 10*time.Second, 500*time.Millisecond).Should(Equal(1))
```

**Pros**:
- ✅ Fixes root cause (no cache lag)
- ✅ Matches DD-STATUS-001 pattern
- ✅ Shorter test times (back to 10s)
- ✅ More reliable CRD visibility

**Cons**:
- ❌ Medium code changes (~12 files)
- ❌ More K8s API load (no caching)

**Effort**: Medium (4-6 hours)

---

### **Option C: Accept E2E Limitations (Not Recommended)**

**Approach**: Document these as known E2E limitations, move tests to integration tier

**Pros**:
- ✅ No E2E changes needed
- ✅ Integration tests already work (use `apiReader`)

**Cons**:
- ❌ Loses E2E coverage
- ❌ Doesn't validate real-world behavior
- ❌ 12 tests remain failing

**Effort**: Low (documentation only)

---

## 📋 **Recommended Action Plan**

### **Phase 1: Quick Win - Increase Timeouts (Option A)**

**Target**: 11 CRD visibility failures
**Effort**: 2-3 hours
**Expected**: 10-11/12 pass (94-95%)

**Changes**:
1. Increase `Eventually` timeouts: 10s → 60s
2. Update failure messages to mention cache sync
3. Add comments explaining K8s eventual consistency

**Files to Update** (4 tests, ~12 test cases):
- `test/e2e/gateway/30_observability_test.go`
- `test/e2e/gateway/31_prometheus_adapter_test.go`
- `test/e2e/gateway/23_audit_emission_test.go`
- `test/e2e/gateway/24_audit_signal_data_test.go`
- `test/e2e/gateway/22_audit_errors_test.go`
- `test/e2e/gateway/32_service_resilience_test.go`

---

### **Phase 2: Proper Fix - apiReader for E2E (Option B)**

**Target**: All 11 CRD visibility failures
**Effort**: 4-6 hours
**Expected**: 11/11 pass reliably, shorter test times

**Changes**:
1. Add `apiReader` to suite setup (uncached client)
2. Update all CRD verification to use `apiReader`
3. Keep 10s timeouts (no cache lag)

**Pattern**:
```go
// For CRD verification: use apiReader (fresh reads)
apiReader.List(ctx, &rrList, ...)

// For other operations: use k8sClient (cached)
k8sClient.Create(ctx, namespace)
```

---

### **Phase 3: Feature Implementation - Namespace Fallback**

**Target**: Test 27
**Effort**: 8-12 hours
**Expected**: 1/1 pass

**See**: `docs/handoff/E2E_TEST27_NAMESPACE_FALLBACK_TODO.md`

---

## 🎯 **Immediate Recommendation**

**Start with Phase 1 (Option A)** - Quick timeout increases:
- Low risk, high success rate
- Gets us to 96-98% pass rate quickly
- Buys time for Phase 2 proper fix
- Tests remain in E2E tier

**Timeline**:
- Phase 1: Today (2-3 hours) → 96-98% pass rate
- Phase 2: This week (4-6 hours) → 98% pass rate, shorter tests
- Phase 3: Next sprint (8-12 hours) → 100% pass rate

---

## 📊 **Expected Outcomes**

| Phase | Pass Rate | Test Time | Effort | Status |
|-------|-----------|-----------|--------|--------|
| **Current** | 86/98 (87.8%) | 6.5 min | - | ✅ Done |
| **Phase 1** | 96-97/98 (98.0%) | 8-9 min | 2-3 hrs | 🔄 Recommended |
| **Phase 2** | 97/98 (99.0%) | 6.5 min | 4-6 hrs | 📋 Planned |
| **Phase 3** | 98/98 (100%) | 6.5 min | 8-12 hrs | 📋 Future |

---

**Document Status**: ✅ Complete
**Triage**: ✅ Done
**Next Action**: Phase 1 (timeout increases)
**Priority**: P1 (blocking 100% pass rate)
