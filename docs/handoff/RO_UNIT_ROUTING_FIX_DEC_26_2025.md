# RO Unit Test Routing Fixes - Dec 26, 2025

## 📊 Issue Summary

**Problem**: 6 routing unit tests failing in `test/unit/remediationorchestrator/routing/blocking_test.go`

**Root Cause**: Tests set `ConsecutiveFailureCount` field directly, but routing engine counts **previous Failed/Blocked RRs** by querying the client, not reading the field.

## 🔍 Routing Engine Logic

```go
// CheckConsecutiveFailures counts Previous Failed RRs
func (r *RoutingEngine) CheckConsecutiveFailures(...) {
    // Query for ALL RRs with same fingerprint
    list := &remediationv1.RemediationRequestList{}
    err := r.client.List(ctx, list, client.MatchingFields{
        "spec.signalFingerprint": rr.Spec.SignalFingerprint,
    })

    // Count consecutive failures from most recent RRs
    consecutiveFailures := 0
    for _, item := range list.Items {
        if item.UID == rr.UID {
            continue // Skip incoming RR
        }
        if item.Status.OverallPhase == PhaseFailed || item.Status.OverallPhase == PhaseBlocked {
            consecutiveFailures++
        } else if item.Status.OverallPhase == PhaseCompleted {
            break // Success breaks chain
        }
    }

    if consecutiveFailures >= r.config.ConsecutiveFailureThreshold {
        return &BlockingCondition{...} // Block
    }
    return nil
}
```

## ✅ Required Fix Pattern

### Before (Incorrect):
```go
rr := &remediationv1.RemediationRequest{
    Spec: remediationv1.RemediationRequestSpec{
        SignalFingerprint: "abc123",
    },
    Status: remediationv1.RemediationRequestStatus{
        ConsecutiveFailureCount: 3, // This is IGNORED by routing engine
    },
}
blocked := engine.CheckConsecutiveFailures(ctx, rr)
```

### After (Correct):
```go
// Create 3 previous Failed RRs with same fingerprint
for i := 0; i < 3; i++ {
    failedRR := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name: fmt.Sprintf("failed-rr-%d", i),
            Namespace: "default",
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

// Now check incoming RR
rr := &remediationv1.RemediationRequest{
    ObjectMeta: metav1.ObjectMeta{
        Name: "incoming-rr",
        Namespace: "default",
    },
    Spec: remediationv1.RemediationRequestSpec{
        SignalFingerprint: "abc123",
    },
}
Expect(fakeClient.Create(ctx, rr)).To(Succeed())

blocked := engine.CheckConsecutiveFailures(ctx, rr)
Expect(blocked).ToNot(BeNil()) // Now it blocks!
```

## 📋 Tests Requiring Fix

1. ✅ `should block when consecutive failures >= threshold` (line 88)
2. ✅ `should set cooldown message with expiry time` (line 135)
3. ✅ `should check all conditions in priority order` (line 757)
4. ✅ `should handle ConsecutiveFailureCount at exactly threshold boundary` (line 1068)
5. ✅ `should handle RR with ConsecutiveFailureCount > threshold` (line 1160)
6. ✅ `should handle priority order: ConsecutiveFailures > DuplicateInProgress` (line 1188)

## 🎯 Next Steps

1. Update each test to create N previous Failed RRs
2. Set `OverallPhase: PhaseFailed` for previous RRs
3. Ensure all RRs have same `SignalFingerprint`
4. Create incoming RR in client before calling routing engine
5. Rerun tests to verify all 34 routing tests pass

## 📊 Expected Result

After fix: **34/34 routing tests passing** ✅

Total RO unit tests: **439/439 (100%)**
- controller: 51/51
- creator: 269/269
- handler: 20/20
- helpers: 22/22
- metrics: 16/16
- notification: 27/27
- routing: 34/34


