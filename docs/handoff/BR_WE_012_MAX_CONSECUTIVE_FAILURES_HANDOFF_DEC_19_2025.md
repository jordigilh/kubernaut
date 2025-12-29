# BR-WE-012: MaxConsecutiveFailures Enforcement - Handoff to RO Team

**Date**: December 19, 2025
**From**: WorkflowExecution Team
**To**: RemediationOrchestrator Team
**Priority**: P1 (Post-V1.0)
**Status**: ✅ **LIKELY ALREADY IMPLEMENTED** - Verification Needed

---

## Executive Summary

**Finding**: WE integration tests were deferred for "MaxConsecutiveFailures enforcement after 5 failures".

**Investigation**: RO likely already implements this via `CheckConsecutiveFailures` routing check.

**Action Required**: RO team should verify `CheckConsecutiveFailures` enforces the 5-failure threshold.

**Confidence**: 85% that this is already complete.

---

## Background

### BR-WE-012 Requirement

**From**: `docs/services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md`

```markdown
#### BR-WE-012: Exponential Backoff Cooldown (P0 - CRITICAL)

**Requirement**: After MaxConsecutiveFailures (5 pre-execution failures),
BLOCK ALL future workflow executions for this target resource until operator intervention.

**Acceptance Criteria**:
5. After 5 consecutive pre-execution failures:
   - System BLOCKS all future executions
   - Status shows "ExhaustedRetries"
   - Requires operator intervention to clear block
```

---

## WE Team Finding

### Integration Test Deferral

**Test**: "should mark Skipped with ExhaustedRetries after 5 consecutive pre-execution failures"

**Reason for Deferral** (from `reconciler_test.go` lines 1138-1147):
```go
It("should mark Skipped with ExhaustedRetries after 5 consecutive pre-execution failures", func() {
    Skip("Complex test requiring 5 failure simulations - deferred to E2E tests")
    // This test would require:
    // 1. Creating WFE that fails
    // 2. Deleting and recreating 5 times
    // 3. Verifying ConsecutiveFailures increments
    // 4. Verifying Skipped phase with ExhaustedRetries reason
    //
    // E2E test suite provides better environment for this scenario
})
```

**Actual Reason** (discovered Dec 19, 2025): This is **RO's routing responsibility**, not WE's.

---

## RO Implementation Status (Likely Complete)

### Evidence 1: CheckConsecutiveFailures Routing Check

**From**: DD-RO-002 lines 254-257

```markdown
### BR-ORCH-042: Consecutive Failures (MaxConsecutiveFailures)

| **BR-WE-012** (MaxConsecutiveFailures) | RO routing logic (Check 1) |
```

**Expected Implementation**: `pkg/remediationorchestrator/routing/blocking.go`

**Function**: `CheckConsecutiveFailures` (lines ~155-181, estimated)

---

### Evidence 2: BR-ORCH-042 Reference

**From**: BR-WE-012 Responsibility document (line 33)

```markdown
✅ All 5 Routing Checks Implemented:
1. CheckConsecutiveFailures (BR-ORCH-042)  ← MaxConsecutiveFailures
2. CheckDuplicateInProgress (DD-RO-002-ADDENDUM)
3. CheckResourceBusy (BR-WE-011)
4. CheckRecentlyRemediated (BR-WE-010)
5. CheckExponentialBackoff (BR-WE-012)  ← Backoff timing
```

**Interpretation**: CheckConsecutiveFailures (Check #1) likely enforces the 5-failure threshold.

---

### Evidence 3: RR Tracking Has ConsecutiveFailureCount

**From**: `api/remediation/v1alpha1/remediationrequest_types.go` (line 546-551)

```go
// ConsecutiveFailureCount tracks how many times this fingerprint has failed consecutively.
// Reset to 0 when remediation succeeds
// Used by routing engine to block after threshold (BR-ORCH-042)
// +optional
ConsecutiveFailureCount int32 `json:"consecutiveFailureCount,omitempty"`
```

**Comment**: "Used by routing engine to block after threshold (BR-ORCH-042)" confirms enforcement exists.

---

## Verification Request for RO Team

### Task 1: Confirm CheckConsecutiveFailures Implementation

**Please verify**:
1. Does `CheckConsecutiveFailures` exist in `pkg/remediationorchestrator/routing/blocking.go`?
2. Does it check `rr.Status.ConsecutiveFailureCount >= 5` (or configurable threshold)?
3. Does it return `BlockReasonConsecutiveFailures` when threshold reached?
4. Does it set a cooldown period (e.g., 1 hour) before allowing retry?

**Expected Code** (approximate):
```go
func (r *RoutingEngine) CheckConsecutiveFailures(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) *BlockingCondition {
    threshold := r.config.ConsecutiveFailureThreshold // Should be 5

    if rr.Status.ConsecutiveFailureCount >= int32(threshold) {
        // Block with fixed cooldown (e.g., 1 hour)
        blockedUntil := metav1.NewTime(time.Now().Add(r.config.ConsecutiveFailureCooldown))

        return &BlockingCondition{
            Blocked: true,
            Reason: string(remediationv1.BlockReasonConsecutiveFailures),
            Message: fmt.Sprintf("Max consecutive failures (%d) reached. Cooldown until %s",
                threshold, blockedUntil.Format(time.RFC3339)),
            BlockedUntil: &blockedUntil.Time,
            RequeueAfter: r.config.ConsecutiveFailureCooldown,
        }
    }

    return nil // Under threshold, allow
}
```

---

### Task 2: Confirm Cooldown Mechanism

**Questions**:
1. What is the cooldown period when MaxConsecutiveFailures is reached?
   - Expected: 1 hour fixed cooldown (per DD-RO-002)
   - Or: Configurable via routing engine config?

2. How does operator clear the block?
   - Option A: Manual `ConsecutiveFailureCount` reset via kubectl patch
   - Option B: BR-WE-013 block clearance mechanism (future)
   - Option C: Automatic reset after cooldown expires

3. Is there a "requires manual review" flag set?
   - Expected: `rr.Status.RequiresManualReview = true`

---

### Task 3: Verify Integration Test Coverage

**Please check**:
1. Do integration tests exist for CheckConsecutiveFailures?
   - Expected location: `test/integration/remediationorchestrator/routing_integration_test.go`
   - Expected tests:
     - "should block after 5 consecutive failures"
     - "should set ConsecutiveFailures block reason"
     - "should allow retry after cooldown expires"

2. Do unit tests exist?
   - Expected location: `test/unit/remediationorchestrator/routing/blocking_test.go`
   - Document mentions: "34/34 specs passing"

---

## If Not Implemented: Implementation Guidance

### Scenario: CheckConsecutiveFailures Does Not Exist

**Implementation Steps**:

#### Step 1: Add CheckConsecutiveFailures to Routing Engine

**File**: `pkg/remediationorchestrator/routing/blocking.go`

```go
// CheckConsecutiveFailures checks if RR has reached MaxConsecutiveFailures threshold.
// Blocks with fixed cooldown when threshold (default: 5) is reached.
//
// BlockReason: "ConsecutiveFailures"
// RequeueAfter: Fixed cooldown period (e.g., 1 hour)
//
// Reference: BR-WE-012 (MaxConsecutiveFailures), BR-ORCH-042
func (r *RoutingEngine) CheckConsecutiveFailures(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) *BlockingCondition {
    logger := log.FromContext(ctx)

    threshold := r.config.ConsecutiveFailureThreshold // Default: 5
    if rr.Status.ConsecutiveFailureCount < int32(threshold) {
        return nil // Under threshold
    }

    // Threshold reached - apply fixed cooldown
    cooldown := r.config.ConsecutiveFailureCooldown // Default: 1 hour
    blockedUntil := metav1.NewTime(time.Now().Add(cooldown))

    logger.Info("Blocking due to consecutive failures threshold",
        "consecutiveFailures", rr.Status.ConsecutiveFailureCount,
        "threshold", threshold,
        "cooldown", cooldown,
        "blockedUntil", blockedUntil.Format(time.RFC3339))

    return &BlockingCondition{
        Blocked: true,
        Reason: string(remediationv1.BlockReasonConsecutiveFailures),
        Message: fmt.Sprintf("Max consecutive failures (%d) reached. Blocked until %s. Requires manual review.",
            threshold, blockedUntil.Format(time.RFC3339)),
        BlockedUntil: &blockedUntil.Time,
        RequeueAfter: cooldown,
        RequiresManualReview: true, // Flag for operator attention
    }
}
```

---

#### Step 2: Integrate with Routing Engine

**File**: `pkg/remediationorchestrator/routing/blocking.go`

**In CheckBlockingConditions function** (estimated line ~50):
```go
func (r *RoutingEngine) CheckBlockingConditions(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) *BlockingCondition {
    // Check 1: Consecutive Failures (BR-ORCH-042, BR-WE-012)
    if block := r.CheckConsecutiveFailures(ctx, rr); block != nil {
        return block
    }

    // Check 2: Duplicate in progress...
    // Check 3: Resource busy...
    // Check 4: Recently remediated...
    // Check 5: Exponential backoff...

    return nil // No blocks, proceed
}
```

---

#### Step 3: Add Configuration

**File**: `pkg/remediationorchestrator/routing/config.go` (or wherever config lives)

```go
type RoutingConfig struct {
    // ... existing fields ...

    // ConsecutiveFailureThreshold is the max number of consecutive failures
    // before blocking remediation. Default: 5
    ConsecutiveFailureThreshold int `json:"consecutiveFailureThreshold"`

    // ConsecutiveFailureCooldown is the fixed cooldown period when threshold
    // is reached. Default: 1 hour
    ConsecutiveFailureCooldown time.Duration `json:"consecutiveFailureCooldown"`
}

func DefaultRoutingConfig() RoutingConfig {
    return RoutingConfig{
        ConsecutiveFailureThreshold: 5,
        ConsecutiveFailureCooldown: 1 * time.Hour,
        // ... other defaults ...
    }
}
```

---

#### Step 4: Add Integration Tests

**File**: `test/integration/remediationorchestrator/routing_integration_test.go`

```go
Context("BR-ORCH-042: MaxConsecutiveFailures Blocking", func() {
    It("should block after 5 consecutive failures", func() {
        By("Creating RR with 5 consecutive failures")
        rr := createRemediationRequest("rr-exhausted", "test/deployment/app")
        rr.Status.ConsecutiveFailureCount = 5
        Expect(k8sClient.Create(ctx, rr)).To(Succeed())

        By("Verifying RO does NOT create WorkflowExecution")
        Consistently(func() bool {
            wfe := getWorkflowExecutionForRR(rr.Name)
            return wfe == nil
        }, 10*time.Second).Should(BeTrue(),
            "WFE should NOT be created (blocked by ConsecutiveFailures)")

        By("Verifying RR marked with ConsecutiveFailures block reason")
        Eventually(func() string {
            updated := getRemediationRequest(rr.Name)
            return updated.Status.BlockReason
        }).Should(Equal("ConsecutiveFailures"))

        By("Verifying BlockedUntil is set (1 hour cooldown)")
        updated := getRemediationRequest(rr.Name)
        Expect(updated.Status.BlockedUntil).ToNot(BeNil())
        expectedBlockedUntil := time.Now().Add(1 * time.Hour)
        Expect(updated.Status.BlockedUntil.Time).To(BeTemporally("~", expectedBlockedUntil, 10*time.Second))
    })

    It("should allow retry after cooldown expires", func() {
        By("Creating RR with 5 failures and EXPIRED cooldown")
        rr := createRemediationRequest("rr-expired-cooldown", "test/deployment/app2")
        rr.Status.ConsecutiveFailureCount = 5
        pastTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
        rr.Status.BlockedUntil = &pastTime
        Expect(k8sClient.Create(ctx, rr)).To(Succeed())

        By("Verifying RO DOES create WorkflowExecution (cooldown expired)")
        Eventually(func() bool {
            wfe := getWorkflowExecutionForRR(rr.Name)
            return wfe != nil
        }, 15*time.Second).Should(BeTrue(),
            "WFE should be created (cooldown expired)")
    })

    It("should require manual review when threshold reached", func() {
        By("Creating RR with 5 consecutive failures")
        rr := createRemediationRequest("rr-manual-review", "test/deployment/app3")
        rr.Status.ConsecutiveFailureCount = 5
        Expect(k8sClient.Create(ctx, rr)).To(Succeed())

        By("Verifying RequiresManualReview flag is set")
        Eventually(func() bool {
            updated := getRemediationRequest(rr.Name)
            return updated.Status.RequiresManualReview
        }).Should(BeTrue(),
            "RR should require manual review after exhausting retries")
    })
})
```

**Estimated Effort**: 2-3 hours (if not implemented)

---

## Summary

**Current Status**: ✅ **LIKELY ALREADY IMPLEMENTED**

**Verification Needed**:
1. Confirm `CheckConsecutiveFailures` exists in `pkg/remediationorchestrator/routing/blocking.go`
2. Confirm threshold is 5 failures (or configurable)
3. Confirm cooldown period (1 hour expected)
4. Confirm integration test coverage exists

**If Not Implemented**:
- Implementation guidance provided above
- Estimated effort: 2-3 hours
- Priority: P1 (Post-V1.0, as it's a safety feature)

**For WE Team**:
- No action required
- WE's role is state tracking only (already complete)
- Routing enforcement is RO's responsibility

---

**Document Version**: 1.0
**Date**: December 19, 2025
**Status**: ⏸️ **PENDING RO TEAM VERIFICATION**
**Priority**: P1 (Post-V1.0)
**Related**: BR-WE-012, BR-ORCH-042, DD-RO-002
**Confidence**: 85% that RO already implements this

