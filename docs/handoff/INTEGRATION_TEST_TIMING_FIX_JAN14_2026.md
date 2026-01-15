# Integration Test Timing Fix - Proper Eventually() Usage

**Date**: 2026-01-14
**Issue**: Tests doing non-polling queries after Eventually() blocks
**Root Cause**: Anti-pattern of checking generic events, then querying specific types
**Solution**: Poll for the specific events being asserted

---

## 🐛 **The Anti-Pattern**

### Current Code (WRONG)

```go
flushAuditStoreAndWait()

// Step 1: Poll for ANY events (too generic)
Eventually(func() int {
    return countAuditEventsByCategory("signalprocessing", correlationID)
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0))

// Step 2: Immediate query for SPECIFIC event types (❌ NO POLLING!)
errorCount := countAuditEvents(spaudit.EventTypeError, correlationID)
completionCount := countAuditEvents(spaudit.EventTypeSignalProcessed, correlationID)

// Step 3: Assert on counts that may not be stable yet
Expect(errorCount > 0 || completionCount > 0).To(BeTrue())  // ❌ Race condition!
```

### Why This Fails

1. ✅ Eventually passes: "At least 1 signalprocessing event exists"
2. ❌ But that event might be a phase.transition, not error/completion
3. ❌ Direct queries for error/completion happen immediately
4. ❌ Those specific events arrive 50-100ms later
5. ❌ Test fails even though events eventually appear

---

## ✅ **The Correct Pattern**

### Fixed Code

```go
flushAuditStoreAndWait()

// ✅ Poll for the SPECIFIC events we're going to assert on
var errorCount, completionCount int
Eventually(func(g Gomega) {
    errorCount = countAuditEvents(spaudit.EventTypeError, correlationID)
    completionCount = countAuditEvents(spaudit.EventTypeSignalProcessed, correlationID)

    // Poll until at least ONE of the required events exists
    g.Expect(errorCount + completionCount).To(BeNumerically(">", 0),
        "Must have either error event OR completion event")
}, 120*time.Second, 500*time.Millisecond).Should(Succeed())

// ✅ Now we can safely assert - counts are stable
Expect(errorCount > 0 || completionCount > 0).To(BeTrue(),
    "BR-SP-090: MUST emit either error event OR degraded mode completion event")

if errorCount > 0 {
    Expect(errorCount).To(Equal(1))
    // Additional error event validation...
}
```

### Why This Works

1. ✅ Eventually polls for the **specific** events we need
2. ✅ Retries every 500ms for up to 120s
3. ✅ Once Eventually succeeds, counts are stable
4. ✅ Assertions use stable data
5. ✅ No race conditions

---

## 🔧 **Fixes Needed**

### File: `test/integration/signalprocessing/audit_integration_test.go`

#### Fix 1: Error Event Test (Line ~680)

**Before**:
```go
flushAuditStoreAndWait()

Eventually(func() int {
    return countAuditEventsByCategory("signalprocessing", correlationID)
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0))

errorCount := countAuditEvents(spaudit.EventTypeError, correlationID)
completionCount := countAuditEvents(spaudit.EventTypeSignalProcessed, correlationID)
```

**After**:
```go
flushAuditStoreAndWait()

// Poll for specific event types, not generic category
var errorCount, completionCount int
Eventually(func(g Gomega) {
    errorCount = countAuditEvents(spaudit.EventTypeError, correlationID)
    completionCount = countAuditEvents(spaudit.EventTypeSignalProcessed, correlationID)

    g.Expect(errorCount + completionCount).To(BeNumerically(">", 0),
        "Must have error event OR completion event")
}, 120*time.Second, 500*time.Millisecond).Should(Succeed())
```

---

#### Fix 2: Phase Transition Test (Line ~600)

**Before**:
```go
flushAuditStoreAndWait()

Eventually(func() int {
    return countAuditEventsByCategory("signalprocessing", correlationID)
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0))

phaseTransitionCount := countAuditEvents(spaudit.EventTypePhaseTransition, correlationID)
Expect(phaseTransitionCount).To(Equal(4))
```

**After**:
```go
flushAuditStoreAndWait()

// Poll until we have the expected number of phase transitions
var phaseTransitionCount int
Eventually(func(g Gomega) {
    phaseTransitionCount = countAuditEvents(spaudit.EventTypePhaseTransition, correlationID)

    g.Expect(phaseTransitionCount).To(BeNumerically(">=", 4),
        "Must have at least 4 phase transitions")
}, 120*time.Second, 500*time.Millisecond).Should(Succeed())

// Now assert on exact count
Expect(phaseTransitionCount).To(Equal(4),
    "BR-SP-090: MUST emit exactly 4 phase transitions")
```

---

## 📊 **Expected Impact**

| Metric | Before Fix | After Fix |
|---|---|---|
| Test Pass Rate | 86% (49/57) | 98%+ (56-57/57) |
| Flaky Tests | 8 (timing-dependent) | 0 (deterministic polling) |
| Test Duration | Same | Same (no sleep added) |
| Code Quality | Anti-pattern | Idiomatic Gomega |

---

## 🎓 **Testing Best Practices**

### ✅ **DO: Poll for What You Assert**

```go
var count int
Eventually(func(g Gomega) {
    count = queryDatabase()
    g.Expect(count).To(BeNumerically(">", 0))
}, timeout, interval).Should(Succeed())

// Now assert on stable data
Expect(count).To(Equal(5))
```

### ❌ **DON'T: Poll Generic, Assert Specific**

```go
// ❌ Bad: Poll for "any data"
Eventually(func() bool {
    return databaseHasAnyData()
}, timeout, interval).Should(BeTrue())

// ❌ Then query specific data (race condition!)
count := querySpecificData()
Expect(count).To(Equal(5))  // May fail due to timing
```

### ❌ **DON'T: Use time.Sleep()**

```go
// ❌ Bad: Non-deterministic timing
flushData()
time.Sleep(500 * time.Millisecond)  // How long is enough?
count := queryData()

// ✅ Good: Polling with timeout
flushData()
Eventually(func() int {
    return queryData()
}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0))
```

---

## 🔗 **Related Issues**

- **SP-AUDIT-001**: Flush bug fix (validated ✅)
- **DD-TESTING-002**: Must-gather diagnostics pattern (production-ready ✅)
- **This Fix**: Proper Eventually() usage for async operations

---

## ✅ **Validation Plan**

1. **Apply Fixes**: Update both failing tests with proper Eventually() usage
2. **Run Tests**: `make test-integration-signalprocessing`
3. **Expected Result**: 56-57/57 passing (98%+ pass rate)
4. **Verify**: No more "INTERRUPTED" failures

---

## 📚 **References**

- **Gomega Eventually**: https://onsi.github.io/gomega/#eventually
- **Testing Anti-Patterns**: time.Sleep(), generic polling
- **Best Practice**: Poll for exactly what you're going to assert

---

**Status**: ✅ **READY TO IMPLEMENT**
**Effort**: 15 minutes (2 test methods to update)
**Impact**: Fixes 6-8 failing tests
**Risk**: Low (improving existing Eventually usage)
