# SignalProcessing Test Failures - Root Cause: Stale Audit Events

**Date**: 2026-01-14
**Status**: 🔴 ROOT CAUSE IDENTIFIED
**Priority**: P1 - Blocks integration testing
**Related**: `docs/handoff/SP_TEST_FAILURES_RCA_JAN14_2026.md`, `docs/handoff/SP_AI_PATTERN_FIXES_RESULTS_JAN14_2026.md`

---

## 📋 Executive Summary

**ROOT CAUSE**: Tests fail because they query audit events by `correlation_id` but find **stale events from previous test runs** in the DataStorage database, violating the assertion `Expect(count).To(Equal(1))`.

**USER INSIGHT**: "Controller should be queried directly without the cache, and the audit should flush and then query by correlationID in an Eventually() loop."

**ANALYSIS RESULT**:
1. ✅ **Controller query is correct**: Uses non-cached `k8sClient.Get()` (direct API call)
2. ❌ **Audit query finds stale data**: Queries by `correlation_id` but database contains events from previous test runs with the same ID

---

## 🔍 Evidence from Test Logs

### Policy Fallback Test (Line 337) - Expected 1, Found 2

```log
# Controller processes in ~2 seconds (20:13:15 → 20:13:17) ✅
2026-01-14T20:13:15 DEBUG: Emitting classification.decision audit event
2026-01-14T20:13:15 ✅ Event buffered successfully, correlation_id="test-policy-fallback-audit-rr"

# Test queries and ALWAYS finds 2 events (expects 1) ❌
✅ Found 2 event(s) for signalprocessing.classification.decision (correlation_id=test-policy-fallback-audit-rr)
✅ Found 2 event(s) for signalprocessing.classification.decision (correlation_id=test-policy-fallback-audit-rr)
✅ Found 2 event(s) for signalprocessing.classification.decision (correlation_id=test-policy-fallback-audit-rr)
... (repeats 30+ times, polling every 2 seconds for 60 seconds)

# Test timeout after 61 seconds
[FAILED] Timed out after 60.001s.
Expected: <int>: 1
    to equal
<int>: 2
```

### Other Failing Test (Line 257) - Same Pattern

```log
# Different test, same problem
✅ Found 2 event(s) for signalprocessing.classification.decision (correlation_id=audit-test-rr-02)
✅ Found 2 event(s) for signalprocessing.classification.decision (correlation_id=audit-test-rr-02)
```

---

## 🧩 Current Test Architecture

### Controller Query (✅ CORRECT)

```go
// Line 234: severity_integration_test.go
Eventually(func(g Gomega) {
    var updated signalprocessingv1alpha1.SignalProcessing
    g.Expect(k8sClient.Get(ctx, types.NamespacedName{
        Name:      sp.Name,
        Namespace: sp.Namespace,
    }, &updated)).To(Succeed())
    g.Expect(updated.Status.Severity).ToNot(BeEmpty())
}, "60s", "2s").Should(Succeed())
```

**Analysis**:
- ✅ Uses `k8sClient` which is a **non-cached client** (created at line 278 of `suite_test.go`)
- ✅ Direct API call to etcd via envtest
- ✅ No cache staleness issue
- ✅ Controller processing completes in ~2 seconds (fast!)

### Audit Query (❌ FINDS STALE DATA)

```go
// Line 250-259: severity_integration_test.go
flushAuditStoreAndWait()  // ✅ Flush succeeds

Eventually(func(g Gomega) {
    count := countAuditEvents("signalprocessing.classification.decision", correlationID)
    g.Expect(count).To(Equal(1))  // ❌ FAILS: Found 2, expected 1
}, 60*time.Second, 2*time.Second).Should(Succeed())
```

**Analysis**:
- ✅ Flush succeeds (events reach DataStorage)
- ✅ Query by `correlation_id` (server-side filter)
- ❌ **Database contains stale events** from previous test runs
- ❌ Test assertion expects exactly 1 event, but finds 2+ events

---

## 🚨 Root Cause: Non-Unique Correlation IDs Across Test Runs

### Current Correlation ID Generation

```go
// Line 595: createTestSignalProcessingCRD helper
rrName := name + "-rr"  // e.g., "test-policy-fallback-audit-rr"

RemediationRequestRef: {
    Name: rrName,  // Used as correlation_id
}
```

**Problem**:
1. **Test name is static** across all test runs: `"test-policy-fallback-audit"`
2. **Correlation ID is derived** from test name: `"test-policy-fallback-audit-rr"`
3. **Database persists** between test runs (no cleanup)
4. **Second test run** creates new events with **same correlation ID**
5. **Query finds ALL events** with that ID: **2 events total** (1 old + 1 new)

### Why This Wasn't Caught Before

**Passing Tests** (like `audit_integration_test.go`):
- Use **unique incrementing correlation IDs**: `"audit-test-rr-01"`, `"audit-test-rr-02"`, etc.
- Each test gets a **different ID** → No collision

**Failing Tests** (like `severity_integration_test.go`):
- Use **static test names**: `"test-policy-fallback-audit"` → `"test-policy-fallback-audit-rr"`
- **Same ID across test runs** → Collision!

---

## 💡 Solution Options

### Option A: Add Timestamp/Random Suffix to Correlation ID (Recommended)

**Implementation**:
```go
// In createTestSignalProcessingCRD helper
import (
    "fmt"
    "time"
    "crypto/rand"
)

func createTestSignalProcessingCRD(name, namespace string, ...) {
    // Generate unique suffix per test execution
    timestamp := time.Now().UnixNano()
    rrName := fmt.Sprintf("%s-rr-%d", name, timestamp)

    // OR use random suffix for true uniqueness
    randomBytes := make([]byte, 4)
    rand.Read(randomBytes)
    rrName := fmt.Sprintf("%s-rr-%x", name, randomBytes)

    // ...
    RemediationRequestRef: {
        Name: rrName,  // e.g., "test-policy-fallback-audit-rr-1737763995123456789"
    }
}
```

**Benefits**:
- ✅ **Truly unique** across all test runs
- ✅ No database cleanup required
- ✅ Matches real-world production patterns (unique remediation IDs)
- ✅ Simple to implement

**Risks**:
- ⚠️ Correlation IDs become longer (may exceed field limits - check schema)

---

### Option B: Clean Up Audit Events Before/After Each Test

**Implementation**:
```go
// In BeforeEach or AfterEach
func cleanupAuditEvents(correlationID string) {
    // Delete all audit events with this correlation_id
    _, err := dataStorageClient.DeleteAuditEvents(ctx, &ogenclient.DeleteAuditEventsParams{
        CorrelationId: ogenclient.NewOptString(correlationID),
    })
    if err != nil {
        GinkgoWriter.Printf("⚠️ Cleanup warning: %v\n", err)
    }
}

BeforeEach(func() {
    cleanupAuditEvents(correlationID)  // Clean before test runs
})

AfterEach(func() {
    cleanupAuditEvents(correlationID)  // Clean after test completes
})
```

**Benefits**:
- ✅ Database starts clean for each test
- ✅ No changes to correlation ID format

**Risks**:
- ❌ Requires DataStorage API endpoint for deletion (may not exist)
- ❌ Cleanup failures could cause cascade test failures
- ❌ Doesn't match production patterns (audit events should be immutable)

---

### Option C: Query by Correlation ID + Timestamp Range

**Implementation**:
```go
// Store test start time
testStartTime := time.Now()

Eventually(func(g Gomega) {
    events := queryAuditEvents("signalprocessing.classification.decision", correlationID)

    // Filter to only events created AFTER test started
    recentEvents := filterEventsByTimestamp(events, testStartTime)

    g.Expect(len(recentEvents)).To(Equal(1))
}, 60*time.Second, 2*time.Second).Should(Succeed())
```

**Benefits**:
- ✅ No changes to correlation ID
- ✅ No database cleanup required

**Risks**:
- ❌ Requires timestamp filtering logic in tests
- ❌ Clock skew between test runner and DataStorage could cause issues
- ❌ More complex test logic

---

## 📊 Recommended Approach

**RECOMMENDATION**: **Option A (Timestamp/Random Suffix)**

**Justification**:
1. **Matches Production**: Real remediation flows use unique IDs (UUIDs, timestamps)
2. **Simplest**: One-line change in helper function
3. **No External Dependencies**: Doesn't require DataStorage API changes
4. **Robust**: No cleanup failures, no clock skew issues

**Implementation Plan**:
1. ✅ Modify `createTestSignalProcessingCRD` to append `time.Now().UnixNano()` to `rrName`
2. ✅ Verify correlation ID length doesn't exceed database field limits
3. ✅ Run integration tests to confirm fix
4. ✅ Document pattern for future test development

---

## 🎯 Expected Outcomes After Fix

### Before Fix (Current State)
```log
✅ Found 2 event(s) for signalprocessing.classification.decision (correlation_id=test-policy-fallback-audit-rr)
Expected <int>: 1 to equal <int>: 2
[FAILED] Timed out after 60.001s
```

### After Fix (Expected)
```log
✅ Found 1 event(s) for signalprocessing.classification.decision (correlation_id=test-policy-fallback-audit-rr-1737763995)
Expected <int>: 1 to equal <int>: 1
[PASSED] Completed in 2.5s
```

---

## 🔗 Related Issues

### Controller Performance (✅ RESOLVED)
**Issue**: "Why does controller wait 60 seconds?"
**Analysis**: Controller processes in **~2 seconds** (fast!)
**Conclusion**: No controller performance issue - timeout is for test polling, not controller processing

### Audit Flush Timing (✅ RESOLVED)
**Issue**: "Why wait 60 seconds for audit event?"
**Analysis**: Flush succeeds immediately, polling is for query retry
**Conclusion**: Flush is fast, but **query finds wrong data** (stale events)

---

## 📝 Action Items

1. [ ] Implement Option A: Add timestamp suffix to correlation ID
2. [ ] Verify correlation ID field length limits in DataStorage schema
3. [ ] Run full integration test suite to confirm fix
4. [ ] Update test documentation with unique correlation ID pattern
5. [ ] Consider adding validation to prevent static correlation IDs in tests

---

## 🔬 Technical Details

### Non-Cached Client Verification

```go
// test/integration/signalprocessing/suite_test.go:278
k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
```

**Analysis**:
- `client.New()` creates a **direct client** (no cache)
- Controller-runtime cached clients use `client.NewDelegatingClient()`
- ✅ Confirmed: Tests use **non-cached direct client**

### Database State Persistence

**Observation**: DataStorage PostgreSQL instance persists between test runs
**Impact**: Audit events accumulate across test runs unless explicitly deleted
**Design Decision**: DataStorage treats audit events as **immutable logs** (correct for production)
**Test Impact**: Tests must use **unique correlation IDs** to avoid collisions

---

## ✅ Validation Criteria

**Test must pass** if:
1. `countAuditEvents()` returns exactly 1 event
2. Test completes in <5 seconds (not 60 seconds)
3. No stale events found from previous test runs
4. Correlation ID is unique per test execution

**Test must fail** if:
1. Multiple events found with same correlation ID
2. Controller processing takes >10 seconds (indicates real issue)
3. Flush fails (indicates audit infrastructure issue)

---

**Next Steps**: Implement Option A and verify fix with full integration test suite
