# RO Routing Unit Test Blocker - Dec 26, 2025

## 🚨 CRITICAL BLOCKER

**Status**: 6/34 routing tests failing (28 passing)
**Duration**: 2+ hours of debugging
**Impact**: Blocking RO PR (CI/CD requires 100% pass)

## 📊 Problem Summary

The routing engine's `CheckConsecutiveFailures()` function is not detecting consecutive failures in unit tests despite:
1. ✅ Creating 3+ Failed RRs with same fingerprint in fake client
2. ✅ Setting explicit CreationTimestamp for proper sorting
3. ✅ NOT creating incoming RR in client (matching DuplicateInProgress pattern)
4. ✅ Field index registered for `spec.signalFingerprint`

## 🔍 Failing Tests

1. "should block when consecutive failures >= threshold" (line 89)
2. "should set cooldown message with expiry time" (line 164)
3. "should check all conditions in priority order" (line 803)
4. "should handle ConsecutiveFailureCount at exactly threshold boundary" (line 1122)
5. "should handle RR with ConsecutiveFailureCount > threshold" (line 1232)
6. "should handle priority order: ConsecutiveFailures > DuplicateInProgress" (line 1273)

## 💡 Root Cause Hypothesis

**HYPOTHESIS**: The atomic status updates (`DD-PERF-001`) may have changed how consecutive failures are tracked.

### Evidence:
- These tests were likely written BEFORE atomic updates implementation
- Routing engine counts Previous Failed RRs by querying the client
- Fake client returns empty list despite RRs being created with correct fingerprint
- Similar `CheckDuplicateInProgress` tests PASS with same fake client pattern

### Possible Issues:
1. **Field Index Not Working**: Fake client not indexing `spec.signalFingerprint` correctly
2. **Query Timing**: RRs not committed before query executes
3. **Atomic Update Side Effect**: ConsecutiveFailures tracking changed but tests not updated
4. **Test Pattern Mismatch**: Test setup doesn't match actual runtime behavior

## 🎯 Recommended Next Steps

### Option A: Debug Field Index (30 min)
Add debug logging to routing engine to see what's returned from query:
```go
list := &remediationv1.RemediationRequestList{}
err := r.client.List(ctx, list, client.MatchingFields{
    "spec.signalFingerprint": rr.Spec.SignalFingerprint,
})
logger.Info("QUERY RESULT", "count", len(list.Items), "fingerprint", rr.Spec.SignalFingerprint)
for _, item := range list.Items {
    logger.Info("FOUND RR", "name", item.Name, "phase", item.Status.OverallPhase)
}
```

### Option B: Check DD-PERF-001 Implementation (20 min)
Review if atomic updates changed how ConsecutiveFailureCount is managed:
- Was field removed/renamed?
- Is tracking now done differently?
- Do tests need to mock a different API?

### Option C: Compare with Working Tests (15 min)
Analyze why `CheckDuplicateInProgress` tests pass but `CheckConsecutiveFailures` fails:
- Both use same fake client
- Both query by fingerprint
- Both use field index
- What's different?

### Option D: Skip Routing Tests Temporarily (5 min - NOT RECOMMENDED)
Mark tests as `Pending` to unblock PR, create follow-up issue.
- ⚠️ **Risk**: Consecutive failures blocking feature untested
- ⚠️ **Risk**: Production bugs if logic broken

## 📋 Test Pattern Attempted

```go
// Create 3 previous Failed RRs
baseTime := time.Now().Add(-10 * time.Minute)
for i := 0; i < 3; i++ {
    failedRR := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name: fmt.Sprintf("failed-rr-%d", i),
            Namespace: "default",
            CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
        },
        Spec: remediationv1.RemediationRequestSpec{
            SignalFingerprint: "abc123",
        },
        Status: remediationv1.RemediationRequestStatus{
            OverallPhase: remediationv1.PhaseFailed,
        },
    }
    Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
}

// Create incoming RR (NOT in client - passed to function only)
rr := &remediationv1.RemediationRequest{
    ObjectMeta: metav1.ObjectMeta{
        Name: "rr-incoming",
        Namespace: "default",
        CreationTimestamp: metav1.Time{Time: time.Now()},
    },
    Spec: remediationv1.RemediationRequestSpec{
        SignalFingerprint: "abc123",
    },
}

// Routing engine queries for RRs with same fingerprint
blocked := engine.CheckConsecutiveFailures(ctx, rr)
// Expected: blocked != nil (found 3 failures)
// Actual: blocked == nil (found 0 failures!)
```

## 🔗 Related Files

- `pkg/remediationorchestrator/routing/blocking.go` (lines 175-257)
- `test/unit/remediationorchestrator/routing/blocking_test.go`
- `docs/architecture/decisions/DD-PERF-001-atomic-status-updates-mandate.md`
- `docs/handoff/RO_UNIT_ROUTING_FIX_DEC_26_2025.md`

## ⏰ Time Investment

- Initial fix attempt: 30 min
- Debug iterations: 90 min
- Documentation: 20 min
- **Total**: 2h 20min

## 🚦 Decision Required

**User input needed**: Which option should I pursue? (A, B, C, or D)

My recommendation: **Option A** (debug logging) to understand why fake client query returns empty list.


