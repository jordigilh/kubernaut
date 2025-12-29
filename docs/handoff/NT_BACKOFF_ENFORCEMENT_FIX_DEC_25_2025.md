# Notification Retry Backoff Enforcement Fix

## Date: December 25, 2025

## Summary
Fixed critical bug where Kubernetes status updates triggered immediate reconciles, **completely bypassing the exponential backoff delays** for retries. All 5 retry attempts were happening within 150ms instead of the expected 5s, 10s, 20s, 38s delays.

---

## Problem Analysis

### Root Cause Discovery Process

1. **Initial Symptom**: Retry tests timing out after 10 seconds
2. **Investigation**: Added debug logging to controller
3. **Discovery**: Permission denied errors were treated as **permanent errors**
4. **Fix Applied**: Wrapped file delivery errors in `RetryableError` type ✅
5. **New Discovery**: Retries working, but **all 5 attempts happening instantly!**

### The Actual Bug (NT-BUG-007)

**File**: `internal/controller/notification/notificationrequest_controller.go`

**Problem**: Status updates trigger immediate K8s reconciles, bypassing `RequeueAfter`

#### Timeline from Controller Logs (BEFORE FIX):
```
2025-12-26T00:49:44.035Z - Attempt 1 (initial)
2025-12-26T00:49:44.061Z - Attempt 2 (26ms later!) ❌ Should be 5s later
2025-12-26T00:49:44.111Z - Attempt 3 (50ms later!) ❌ Should be 10s later
2025-12-26T00:49:44.147Z - Attempt 4 (36ms later!) ❌ Should be 20s later
2025-12-26T00:49:44.170Z - Attempt 5 (23ms later!) ❌ Should be 38s later
```

**All 5 retries completed in ~150 milliseconds instead of ~90 seconds!**

#### Why This Happened

1. `transitionToRetrying()` returns `ctrl.Result{RequeueAfter: backoff}` ✅
2. But it first calls `UpdatePhase()`, which writes to K8s API ❌
3. K8s detects status change and triggers new reconcile **immediately** ❌
4. New reconcile happens within milliseconds, bypassing `RequeueAfter` ❌

**This is the same race condition we fixed earlier with batch status updates!**

---

## Solution: Backoff Enforcement at Reconcile Entry

### Strategy

Add a **gate** at the start of reconcile to check if we're in `Retrying` phase and enforce the backoff delay.

```go
// NT-BUG-007: Backoff enforcement for Retrying phase
// Problem: Status updates trigger immediate reconciles, bypassing RequeueAfter backoff
// Solution: Check if enough time has elapsed since last attempt before retrying

if notification.Status.Phase == notificationv1alpha1.NotificationPhaseRetrying &&
	len(notification.Status.DeliveryAttempts) > 0 {

	// Find the most recent failed delivery attempt
	var lastFailedAttempt *notificationv1alpha1.DeliveryAttempt
	for i := len(notification.Status.DeliveryAttempts) - 1; i >= 0; i-- {
		attempt := &notification.Status.DeliveryAttempts[i]
		if attempt.Status == "failed" {
			lastFailedAttempt = attempt
			break
		}
	}

	if lastFailedAttempt != nil {
		// Calculate expected next retry time
		attemptCount := lastFailedAttempt.Attempt
		nextBackoff := r.calculateBackoffWithPolicy(notification, attemptCount)
		nextRetryTime := lastFailedAttempt.Timestamp.Time.Add(nextBackoff)
		now := time.Now()

		if now.Before(nextRetryTime) {
			remainingBackoff := nextRetryTime.Sub(now)
			log.Info("⏸️ BACKOFF ENFORCEMENT: Too early to retry, requeueing",
				"attemptNumber", attemptCount,
				"lastAttemptTime", lastFailedAttempt.Timestamp.Time.Format(time.RFC3339),
				"nextRetryTime", nextRetryTime.Format(time.RFC3339),
				"remainingBackoff", remainingBackoff,
				"channel", lastFailedAttempt.Channel)
			return ctrl.Result{RequeueAfter: remainingBackoff}, nil
		}

		log.Info("✅ BACKOFF ELAPSED: Ready to retry",
			"attemptNumber", attemptCount,
			"lastAttemptTime", lastFailedAttempt.Timestamp.Time.Format(time.RFC3339),
			"elapsedSinceLastAttempt", now.Sub(lastFailedAttempt.Timestamp.Time),
			"expectedBackoff", nextBackoff)
	}
}
```

### How It Works

1. **Check Phase**: If we're in `Retrying` phase, check timing
2. **Find Last Attempt**: Look for most recent failed delivery attempt
3. **Calculate Next Retry Time**: `lastAttemptTime + backoff`
4. **Enforce Backoff**: If current time < next retry time, return `RequeueAfter: remaining_time`
5. **Allow Retry**: If backoff elapsed, proceed with delivery attempt

---

## Expected Behavior After Fix

### Controller Logs (AFTER FIX):
```
2025-12-26T00:49:44.035Z - Attempt 1 (initial)
2025-12-26T00:49:44.061Z - ⏸️ BACKOFF ENFORCEMENT: Too early, requeueing (4.9s remaining)
   [4.9 seconds pass - no reconcile]
2025-12-26T00:49:49.061Z - ✅ BACKOFF ELAPSED: Ready to retry (Attempt 2)
   [10 seconds pass - no reconcile]
2025-12-26T00:49:59.061Z - ✅ BACKOFF ELAPSED: Ready to retry (Attempt 3)
   [20 seconds pass - no reconcile]
2025-12-26T00:50:19.061Z - ✅ BACKOFF ELAPSED: Ready to retry (Attempt 4)
   [38 seconds pass - no reconcile]
2025-12-26T00:50:57.061Z - ✅ BACKOFF ELAPSED: Ready to retry (Attempt 5)
```

**Total retry time: ~90 seconds (as designed)**

---

## Test Impact

### Before Fix: Tests Failing

**Problem**: All 5 retries happened in 150ms, so by the time test polled K8s API (every 500ms), the notification was already in `PartiallySent` terminal phase.

**Test Assertion**:
```go
Eventually(..., 10*time.Second).Should(Equal(NotificationPhaseRetrying))
```

**Result**: ❌ **FAIL** - Phase went from `Sending` → `Retrying` → `PartiallySent` in < 200ms

### After Fix: Tests Should Pass

**Expected**: Backoff enforcement will slow down retries to match expected timing.

**Test Timeline**:
1. t=0s: Test creates notification
2. t=0s: Initial delivery (Console✅, File❌) → Phase: `Retrying`
3. t=0s-10s: Test polls and sees `Retrying` phase ✅
4. t=5s: Attempt 2 (after backoff elapsed)
5. t=15s: Attempt 3 (after backoff elapsed)
6. ... (retries continue with proper delays)

---

## Files Modified

1. **internal/controller/notification/notificationrequest_controller.go** (Lines ~218-260)
   - Added NT-BUG-007 backoff enforcement check
   - Check executes immediately after terminal phase check
   - Enforces backoff by returning `RequeueAfter: remaining_time`

---

## Related Bugs Fixed

| Bug ID | Description | Status |
|---|---|---|
| **NT-BUG-006** | File errors treated as permanent (no retries) | ✅ FIXED (RetryableError) |
| **NT-BUG-007** | Status updates bypass RequeueAfter backoff | ✅ FIXED (Backoff enforcement) |
| **NT-BUG-005** | PartiallySent instead of Retrying phase | ✅ FIXED (Retrying phase) |

---

## Verification Steps

### 1. Check Controller Logs for Backoff Enforcement

```bash
kubectl logs -n notification-e2e -l app=notification-controller --tail=1000 | \
grep "BACKOFF ENFORCEMENT\|BACKOFF ELAPSED"
```

**Expected**: See `⏸️ BACKOFF ENFORCEMENT` messages when immediate reconciles are rejected.

### 2. Verify Retry Timing

```bash
kubectl logs -n notification-e2e -l app=notification-controller --tail=2000 | \
grep "e2e-retry-backoff-test.*Attempt" | \
awk '{print $1 " " $2 " " $(NF-1) " " $NF}'
```

**Expected**: See ~5s, ~10s, ~20s, ~38s delays between attempts.

### 3. E2E Test Results

```bash
make test-e2e-notification 2>&1 | grep "Ran.*Specs"
```

**Expected**: `Ran 22 of 22 Specs` with `22 Passed | 0 Failed`

---

## Confidence Assessment

**Implementation**: 90% confidence
- Backoff enforcement logic is straightforward
- Reuses existing `calculateBackoffWithPolicy` function
- Similar pattern to batch status update fix (proven approach)

**Potential Issues**: 10% risk
- Clock skew in distributed systems (unlikely in K8s)
- Race condition if delivery attempts aren't properly ordered (mitigated by reverse loop)
- Edge case: What if `DeliveryAttempts` is empty? (handled: check `len > 0`)

**Testing Strategy**: Verify with live controller logs
- Check timing between attempts matches expected backoff
- Ensure `⏸️ BACKOFF ENFORCEMENT` logs appear
- Confirm E2E tests pass

---

## Next Steps

1. ✅ Rebuild controller image with backoff enforcement
2. ⏳ Run E2E tests with KEEP_CLUSTER to inspect logs
3. ⏳ Verify backoff timing in controller logs
4. ⏳ Confirm all 22 E2E tests pass
5. ⏳ Document any remaining issues

---

## Technical Debt

### Potential Improvements

1. **Atomic Phase + Backoff Update**
   - Current: Phase update triggers immediate reconcile
   - Better: Update phase + set "nextRetryTime" field in status atomically
   - Benefit: Eliminates need for backoff enforcement check

2. **Kubernetes Timer Controller**
   - Current: Use `RequeueAfter` for backoff delays
   - Alternative: Use Kubernetes `Job` with `startingDeadlineSeconds`
   - Benefit: More explicit retry scheduling

3. **Retry Queue Service**
   - Current: Each controller handles own retries
   - Alternative: Dedicated retry service with priority queue
   - Benefit: Centralized retry management, better observability

**Recommendation**: Keep current approach for MVP. The backoff enforcement fix is simple, effective, and doesn't require major architectural changes.

---

## Bug Tracking

**Bug ID**: NT-BUG-007
**Title**: Kubernetes status updates bypass RequeueAfter backoff
**Status**: FIXED (implementation complete, testing in progress)
**Related**: NT-BUG-006 (RetryableError), NT-BUG-005 (Retrying phase)


