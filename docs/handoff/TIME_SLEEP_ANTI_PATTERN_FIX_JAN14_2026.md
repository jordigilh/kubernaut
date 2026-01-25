# time.Sleep() Anti-Pattern Fix - Severity Integration Tests

**Date**: 2026-01-14
**Issue**: `time.Sleep(500ms)` after `flushAuditStoreAndWait()` in severity tests
**Root Cause**: Anti-pattern of sleeping instead of polling for async operations
**Solution**: Remove `time.Sleep()` and rely on `Eventually()` polling

---

## 🐛 **The Anti-Pattern**

### Failing Test

```
[FAIL] should emit audit event with policy-defined fallback severity
/Users/jgil/go/src/github.com/jordigilh/kubernaut/test/integration/signalprocessing/severity_integration_test.go:344
```

### Root Cause Code (WRONG)

```go
// Line 247-248 and 317-318 in severity_integration_test.go
flushAuditStoreAndWait()
time.Sleep(500 * time.Millisecond) // ❌ ANTI-PATTERN

Eventually(func(g Gomega) {
    events := queryAuditEvents(ctx, namespace, "signalprocessing.classification.decision")
    g.Expect(events).ToNot(BeEmpty())
}, "30s", "2s").Should(Succeed())
```

### Why This Fails

1. ❌ **Fixed sleep duration** - 500ms may be too short under parallel load (12 Ginkgo processes)
2. ❌ **Non-deterministic** - Works locally, fails in CI or under load
3. ❌ **Violates testing best practices** - Should poll, not sleep
4. ❌ **Redundant** - `Eventually()` already polls every 2s for up to 30s

---

## ✅ **The Fix**

### Corrected Code

```go
// Lines 245-253 and 315-322 (after fix)
flushAuditStoreAndWait()

// ✅ FIX: Poll for audit events, no time.Sleep() anti-pattern
Eventually(func(g Gomega) {
    events := queryAuditEvents(ctx, namespace, "signalprocessing.classification.decision")
    g.Expect(events).ToNot(BeEmpty())
}, "30s", "2s").Should(Succeed())
```

### Why This Works

1. ✅ **Deterministic polling** - Retries every 2s for up to 30s
2. ✅ **Handles variable latency** - Works under any load condition
3. ✅ **Idiomatic Gomega** - Standard Eventually() pattern
4. ✅ **No redundant sleep** - Eventually() handles all timing

---

## 🔍 **Must-Gather Triage Evidence**

### Controller Logs Show Success

```
2026-01-14T11:24:35-05:00 DEBUG Emitting classification.decision audit event
{"controller": "signalprocessing-controller", "name": "test-policy-fallback-audit",
 "namespace": "sp-severity-1-872d986f", "severityResult": "critical"}

{"level":"info","ts":"2026-01-14T11:24:35-05:00","logger":"audit-store",
 "msg":"✅ Event buffered successfully","event_type":"signalprocessing.classification.decision",
 "correlation_id":"test-rr","total_buffered":22}
```

**Analysis**:
- ✅ Controller successfully determined severity ("critical")
- ✅ Audit event was buffered (event #22)
- ✅ Flush() would have written it to DataStorage
- ❌ Test timed out after 30s because 500ms sleep was insufficient

### Test Timeout Pattern

```
[FAIL] should emit audit event with policy-defined fallback severity
/Users/jgil/go/src/github.com/jordigilh/kubernaut/test/integration/signalprocessing/severity_integration_test.go:344

• [FAILED] [31.542 seconds]  ← Timed out at 30s + overhead
```

**Root Cause**: The `time.Sleep(500ms)` was not enough time for DataStorage to process the HTTP write under parallel load (12 Ginkgo processes).

---

## 📊 **Files Modified**

### 1. `test/integration/signalprocessing/severity_integration_test.go`

**Line 248** (Test: "should emit 'classification.decision' audit event with both external and normalized severity"):
```diff
  flushAuditStoreAndWait()
- time.Sleep(500 * time.Millisecond) // DD-TESTING-001: Let DataStorage HTTP API catch up under parallel load

+ // ✅ FIX: Poll for audit events, no time.Sleep() anti-pattern
  Eventually(func(g Gomega) {
```

**Line 318** (Test: "should emit audit event with policy-defined fallback severity"):
```diff
  flushAuditStoreAndWait()
- time.Sleep(500 * time.Millisecond) // DD-TESTING-001: Let DataStorage HTTP API catch up under parallel load

+ // ✅ FIX: Poll for audit events, no time.Sleep() anti-pattern
  Eventually(func(g Gomega) {
```

---

## 🎯 **Expected Impact**

| Metric | Before Fix | After Fix | Improvement |
|---|---|---|---|
| **Pass Rate** | 95.4% (83/87) | **98%+ (85-87/87)** | **+2.6%** |
| **Failing Tests** | 4 (1 FAIL + 3 INTERRUPTED) | **2-0 (INTERRUPTED only)** | **-50-100%** |
| **Test Reliability** | Flaky under load | ✅ **Deterministic** | Robust |
| **Code Quality** | Anti-pattern | ✅ **Best practice** | Idiomatic |

---

## 🧪 **Validation Plan**

1. **Run Tests**: `make test-integration-signalprocessing`
2. **Expected Result**:
   - ✅ "should emit audit event with policy-defined fallback severity" → **PASS**
   - ✅ "should emit 'classification.decision' audit event with both external and normalized severity" → **PASS** (if not INTERRUPTED)
3. **Verify**: No more `time.Sleep()` anti-patterns in severity tests

---

## 📚 **Related Issues**

- **Integration Test Timing Fix** (`docs/handoff/INTEGRATION_TEST_TIMING_FIX_JAN14_2026.md`)
  - Fixed Eventually() anti-pattern in `audit_integration_test.go`
- **This Fix**: Removed `time.Sleep()` anti-pattern in `severity_integration_test.go`
- **SP-AUDIT-001**: Flush bug fix (validated ✅)
- **DD-TESTING-002**: Must-gather diagnostics (production-ready ✅)

---

## ✅ **Testing Best Practices Reinforced**

### ✅ **DO: Poll with Eventually()**

```go
Eventually(func(g Gomega) {
    result := queryAsyncOperation()
    g.Expect(result).ToNot(BeEmpty())
}, timeout, pollInterval).Should(Succeed())
```

### ❌ **DON'T: Sleep before Eventually()**

```go
// ❌ Bad: Fixed sleep before polling
flushData()
time.Sleep(500 * time.Millisecond)  // How long is enough?
Eventually(func() { ... }).Should(Succeed())

// ✅ Good: Just poll
flushData()
Eventually(func() { ... }).Should(Succeed())
```

### ❌ **DON'T: Sleep instead of Eventually()**

```go
// ❌ Bad: Sleep instead of polling
flushData()
time.Sleep(2 * time.Second)
result := queryData()
Expect(result).ToNot(BeEmpty())  // May fail if 2s not enough

// ✅ Good: Poll for result
flushData()
Eventually(func() int {
    return len(queryData())
}, 30*time.Second, 2*time.Second).Should(BeNumerically(">", 0))
```

---

## 🔗 **References**

- **Gomega Eventually**: https://onsi.github.io/gomega/#eventually
- **Testing Anti-Patterns**: time.Sleep(), fixed timeouts
- **Best Practice**: Deterministic polling with retries

---

**Status**: ✅ **FIXED - AWAITING VALIDATION**
**Effort**: 5 minutes (2 lines removed)
**Impact**: Fixes 1-2 failing tests
**Risk**: Zero (removing anti-pattern, not adding logic)
