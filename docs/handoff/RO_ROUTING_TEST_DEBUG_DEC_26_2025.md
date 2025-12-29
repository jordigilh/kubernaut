# RO Routing Unit Test Debug Session - Dec 26, 2025

## 🚨 **Status**: BLOCKED - Needs Investigation

**Problem**: 6 routing unit tests failing despite correct test setup
**Time Spent**: ~2 hours debugging
**Blocker**: Routing engine query works, but counting logic fails

---

## 📊 **Failing Tests** (6/34)

1. `should block when consecutive failures >= threshold` (line 89)
2. `should set cooldown message with expiry time` (line 154)
3. `should check all conditions in priority order` (line 792)
4. `should handle ConsecutiveFailureCount at exactly threshold boundary` (line 1119)
5. `should handle RR with ConsecutiveFailureCount > threshold` (line 1237)
6. `should handle priority order: ConsecutiveFailures > DuplicateInProgress` (line 1279)

**Common Error**: `Expected <*routing.BlockingCondition | 0x0>: nil not to be nil`

---

## 🔍 **Key Findings**

### **Finding 1**: Field Index Works ✅
- Debug assertion added at line 110: `Expect(debugList.Items).To(HaveLen(3))`
- Debug assertion **PASSES** (test fails at line 135, not at debug line)
- Conclusion: Fake client's field index correctly returns 3 Failed RRs

### **Finding 2**: Query Pattern Matches Successful Tests ✅
- `CheckDuplicateInProgress` tests PASS (28/34 tests pass overall)
- Both use same field index: `spec.signalFingerprint`
- Same test setup pattern: Create historical RRs, DON'T create incoming RR

### **Finding 3**: Namespace Configuration Correct ✅
- Routing engine configured with namespace "default" (line 81)
- All test RRs created in namespace "default"
- `FindActiveRRForFingerprint` filters by namespace, works fine

---

## 🐛 **Suspected Issue**

**Hypothesis**: Routing engine's consecutive failure counting logic has a bug

**Evidence**:
1. ✅ Field index query returns correct RRs (debug assertion passes)
2. ✅ Test setup follows correct pattern (historical RRs created)
3. ❌ Routing engine returns `nil` (no blocking condition)
4. ❌ All 6 tests fail with same error

**Code to Investigate**:
```go
// pkg/remediationorchestrator/routing/blocking.go lines 214-246
consecutiveFailures := 0
for _, item := range list.Items {
    // Skip the incoming RR itself (it's not failed yet)
    if item.UID == rr.UID {  // ← INVESTIGATE: Does this skip correctly?
        logger.Info("Skipping incoming RR", "name", item.Name)
        continue
    }

    // Count both Failed and Blocked RRs as failures
    if item.Status.OverallPhase == remediationv1.PhaseFailed ||
       item.Status.OverallPhase == remediationv1.PhaseBlocked {
        consecutiveFailures++  // ← INVESTIGATE: Is this incrementing?
        // ...
    } else if item.Status.OverallPhase == remediationv1.PhaseCompleted {
        break
    } else {
        // Ignore non-terminal RRs  ← INVESTIGATE: Are Failed RRs being ignored?
    }
}
```

---

## 🔬 **Next Steps for Investigation**

### **Option A: Add Routing Engine Logging**
Temporarily add print statements to routing engine to see:
1. How many RRs are in `list.Items`
2. What phase each RR has
3. What `consecutiveFailures` counter value is
4. Whether the UID comparison is skipping anything

### **Option B: Inspect RR Phases**
Check if the test's Failed RRs actually have `Status.OverallPhase = PhaseFailed`:
```go
// In test, after creating Failed RRs:
for i := 0; i < 3; i++ {
    var created remediationv1.RemediationRequest
    Expect(fakeClient.Get(ctx, client.ObjectKeyFrom(&failedRR), &created)).To(Succeed())
    GinkgoWriter.Printf("Created RR phase: %s\n", created.Status.OverallPhase)
}
```

### **Option C: Compare with Passing Test**
`CheckDuplicateInProgress` works. Compare its logic to `CheckConsecutiveFailures` to find the difference.

---

## 📝 **Test Setup Pattern (Confirmed Correct)**

```go
// 1. Create 3 historical Failed RRs
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
            OverallPhase: remediationv1.PhaseFailed,  // ← This is set!
        },
    }
    Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
}

// 2. Create incoming RR (NOT created in fake client)
rr := &remediationv1.RemediationRequest{
    // ...
    Spec: remediationv1.RemediationRequestSpec{
        SignalFingerprint: "abc123",  // Same fingerprint
    },
}

// 3. Call routing engine
blocked := engine.CheckConsecutiveFailures(ctx, rr)
Expect(blocked).ToNot(BeNil())  // ❌ FAILS: blocked is nil
```

---

## ⚠️ **Important Context**

### **What We Know Works**
- ✅ Fake client field indexing
- ✅ Query by `spec.signalFingerprint`
- ✅ `CheckDuplicateInProgress` routing logic
- ✅ Test namespace configuration
- ✅ RR creation with `Status.OverallPhase` set

### **What We Know Fails**
- ❌ `CheckConsecutiveFailures` returns `nil` instead of blocking condition
- ❌ All 6 tests that rely on consecutive failure counting
- ❌ Counting logic doesn't reach threshold (3 Failed RRs, threshold=3)

---

## 🎯 **Recommended Next Action**

**Priority**: HIGH - Blocks PR merge (6/34 tests failing = 18% failure rate)

**Quickest Path to Resolution**:
1. Add temporary logging to `CheckConsecutiveFailures` (lines 214-246)
2. Run failing test with logs
3. Identify why `consecutiveFailures` counter doesn't reach 3
4. Fix the bug in routing engine logic
5. Remove temporary logging
6. Run all tests to verify fix

**Estimated Time**: 30-60 minutes once logging reveals the issue

---

## 📚 **References**

- **Test File**: `test/unit/remediationorchestrator/routing/blocking_test.go`
- **Routing Engine**: `pkg/remediationorchestrator/routing/blocking.go`
- **Design Decision**: `DD-RO-002` (Routing Engine Blocking Conditions)
- **Business Requirement**: `BR-ORCH-042` (Consecutive Failure Blocking)

---

**Created**: 2025-12-26 19:03
**Last Updated**: 2025-12-26 19:03
**Status**: NEEDS_INVESTIGATION
**Assignee**: Next Session / User

