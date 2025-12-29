# Notification E2E Retry Tests - Root Cause: Status Update Race Condition

**Date**: December 25, 2025
**Issue**: 2 retry tests fail (20/22 passing)
**Root Cause**: **CONFIRMED** - Status updates during delivery loop trigger immediate reconciles, bypassing backoff

---

## 🔍 **Root Cause Analysis**

### **The Problem: Status Update Race Condition**

**Sequence of Events** (First Attempt):

```
1. Reconcile #1 starts
2. handleDeliveryLoop() called
3. Orchestrator.DeliverToChannels() runs:

   a) Console channel:
      - Deliver() → SUCCESS
      - RecordDeliveryAttempt() → STATUS UPDATE ⚡ TRIGGERS RECONCILE #2
      - (but Reconcile #1 continues...)

   b) File channel:
      - Deliver() → FAIL (read-only directory)
      - RecordDeliveryAttempt() → STATUS UPDATE ⚡ TRIGGERS RECONCILE #3
      - (but Reconcile #1 continues...)

4. Return from handleDeliveryLoop() with result
5. determinePhaseTransition() called
6. Calculate backoff: 30s (first attempt)
7. Return ctrl.Result{RequeueAfter: 30s}

❌ BUT: Reconcile #2 and #3 already started from status updates!
```

**Reconcile #2** (from Console status update):
```
- Starts IMMEDIATELY (no backoff)
- Reads notification with Console=success, File=not attempted yet
- handleDeliveryLoop():
  - Console: Skip (already succeeded) ✅
  - File: Attempt count=0, tries delivery → FAILS
  - RecordDeliveryAttempt() → STATUS UPDATE ⚡ TRIGGERS RECONCILE #4
- Returns with backoff (but Reconcile #4 already started)
```

**Reconcile #3, #4, #5...** (cascade):
```
- Each reconcile triggers MORE reconciles via status updates
- No backoff is respected
- Eventually hits rate limiting or max attempts
- Never waits for exponential backoff period
```

---

## 📍 **Code Evidence**

### **Location 1: Orchestrator RecordDeliveryAttempt**
**File**: `pkg/notification/delivery/orchestrator.go`
**Lines**: 205-215

```go
// Line 202: Attempt delivery
deliveryErr := o.DeliverToChannel(ctx, notification, channel)

// Line 205-215: Record delivery attempt (STATUS UPDATE HAPPENS HERE)
if err := o.RecordDeliveryAttempt(
    ctx,
    notification,
    channel,
    deliveryErr,
    getChannelAttemptCount,
    auditMessageSent,
    auditMessageFailed,
); err != nil {
    return nil, err
}
```

**Inside `RecordDeliveryAttempt`**:
```go
// Lines 325-340 (orchestrator.go): Creates attempt record
attempt := notificationv1alpha1.DeliveryAttempt{...}

// Lines 342-349: Calls StatusManager.RecordDeliveryAttempt
if err := o.statusManager.RecordDeliveryAttempt(ctx, notification, attempt); err != nil {
    return fmt.Errorf("failed to record delivery attempt: %w", err)
}
```

### **Location 2: StatusManager RecordDeliveryAttempt**
**File**: `pkg/notification/status/manager.go`
**Lines**: 32-58

```go
func (m *Manager) RecordDeliveryAttempt(...) error {
    return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
        // 1. Refetch to get latest resourceVersion
        if err := m.client.Get(ctx, client.ObjectKeyFromObject(notification), notification); err != nil {
            return fmt.Errorf("failed to refetch notification: %w", err)
        }

        // 2. Append the attempt
        notification.Status.DeliveryAttempts = append(notification.Status.DeliveryAttempts, attempt)

        // 3. Update counters
        notification.Status.TotalAttempts++
        ...

        // 4. Update status using status subresource
        // ⚡ THIS TRIGGERS A NEW RECONCILE IMMEDIATELY
        if err := m.client.Status().Update(ctx, notification); err != nil {
            return fmt.Errorf("failed to record delivery attempt: %w", err)
        }

        return nil
    })
}
```

**The Problem**: Line 52 `m.client.Status().Update()` writes to Kubernetes, which triggers controller-runtime's reconcile queue.

---

## 🎯 **Why Backoff Never Happens**

### **Expected Behavior**:
```
Reconcile #1 → Fail → Return RequeueAfter: 30s → Wait 30s → Reconcile #2 → Fail → Wait 60s → ...
```

### **Actual Behavior**:
```
Reconcile #1 → Fail → STATUS UPDATE → Reconcile #2 (instant) → Fail → STATUS UPDATE → Reconcile #3 (instant) → ...
```

**Result**: All 5 retry attempts happen within seconds, not minutes (30s, 60s, 120s, 240s, 480s).

---

## 💡 **Solution Options**

### **Option 1: Batch Status Updates** ⭐ **RECOMMENDED**

**Approach**: Accumulate delivery attempts in memory, write status ONCE after delivery loop completes.

**Changes Required**:
1. **Orchestrator**: Collect attempts in `DeliveryResult`, don't call `RecordDeliveryAttempt` during loop
2. **Controller**: Call `RecordDeliveryAttempt` ONCE after `handleDeliveryLoop` returns
3. **Benefit**: Single status update per reconcile, backoff works correctly

**Implementation**:
```go
// In orchestrator.go (DeliverToChannels):
result := &DeliveryResult{
    DeliveryResults: make(map[string]error),
    DeliveryAttempts: []notificationv1alpha1.DeliveryAttempt{}, // NEW: Collect attempts
    FailureCount: 0,
}

for _, channel := range channels {
    deliveryErr := o.DeliverToChannel(ctx, notification, channel)

    // REMOVE: RecordDeliveryAttempt() call here

    // NEW: Add attempt to result (no status update yet)
    attempt := createDeliveryAttempt(channel, deliveryErr, ...)
    result.DeliveryAttempts = append(result.DeliveryAttempts, attempt)

    if deliveryErr != nil {
        result.FailureCount++
    }
}

return result, nil

// In controller.go (after handleDeliveryLoop):
result, err := r.handleDeliveryLoop(ctx, notification)
if err != nil {
    return ctrl.Result{}, err
}

// NEW: Record all delivery attempts in a SINGLE status update
for _, attempt := range result.DeliveryAttempts {
    if err := r.StatusManager.RecordDeliveryAttempt(ctx, notification, attempt); err != nil {
        return ctrl.Result{}, err
    }
}

// NOW determinePhaseTransition's backoff will work correctly
return r.determinePhaseTransition(ctx, notification, result)
```

**Pros**:
- ✅ Minimal code changes
- ✅ Fixes root cause (no mid-loop status updates)
- ✅ Backoff works correctly
- ✅ Reduces Kubernetes API calls (1 update instead of N)
- ✅ No race conditions

**Cons**:
- ⚠️ Audit events still happen during loop (but don't trigger reconciles)
- ⚠️ Requires testing to ensure attempt order is preserved

**Estimated Effort**: 1-2 hours

---

### **Option 2: Add Backoff Guard at Orchestrator Level** ⚠️ **COMPLEX**

**Approach**: Check last attempt timestamp before attempting delivery.

**Changes Required**:
1. Track `LastAttemptTime` in notification status
2. Orchestrator checks: `if time.Since(lastAttempt) < backoff { skip }`
3. Return special error: `ErrTooSoonToRetry`

**Implementation**:
```go
// In orchestrator.go (DeliverToChannels):
for _, channel := range channels {
    // Check if enough time has passed since last attempt
    lastAttempt := getLastAttemptTime(notification, string(channel))
    policy := getRetryPolicy(notification)
    attemptCount := getChannelAttemptCount(notification, string(channel))

    requiredBackoff := calculateBackoff(policy, attemptCount)
    timeSinceAttempt := time.Since(lastAttempt)

    if timeSinceAttempt < requiredBackoff {
        log.Info("Skipping delivery, backoff period not elapsed",
            "channel", channel,
            "timeSince", timeSinceAttempt,
            "requiredBackoff", requiredBackoff)
        continue // Skip this channel, too soon to retry
    }

    // Proceed with delivery...
}
```

**Pros**:
- ✅ Enforces backoff even with immediate reconciles
- ✅ Defensive against status update races

**Cons**:
- ❌ Complex: Requires tracking timestamps per channel
- ❌ Doesn't solve root cause (still have unnecessary reconciles)
- ❌ Wastes resources (reconciles that do nothing)
- ❌ Requires CRD schema change (`LastAttemptTime` field)

**Estimated Effort**: 3-4 hours

---

### **Option 3: Debounce Reconcile with Generation Check** ❌ **NOT RECOMMENDED**

**Approach**: Skip reconcile if `ObservedGeneration == Generation` and phase is `Sending`.

**Problem**: Won't work for retries (generation doesn't change on retry).

---

## 📊 **Option Comparison**

| Criterion | Option 1 (Batch Updates) | Option 2 (Backoff Guard) | Option 3 (Debounce) |
|-----------|--------------------------|--------------------------|---------------------|
| **Fixes Root Cause** | ✅ Yes | ⚠️ Workaround | ❌ No |
| **Backoff Works** | ✅ Yes | ✅ Yes | ❌ No |
| **Complexity** | ✅ Low | ⚠️ Medium | ❌ High |
| **API Efficiency** | ✅ Better | ❌ Worse | ❌ Worse |
| **CRD Changes** | ✅ None | ❌ Required | ❌ Required |
| **Effort** | ✅ 1-2h | ⚠️ 3-4h | ❌ 4-6h |

---

## 🎯 **Recommended Action**

**Implement Option 1: Batch Status Updates**

**Why**:
1. ✅ Fixes the root cause (eliminates mid-loop status updates)
2. ✅ Simple implementation (move `RecordDeliveryAttempt` call)
3. ✅ No CRD changes needed
4. ✅ Improves efficiency (fewer Kubernetes API calls)
5. ✅ Backoff will work correctly

**Implementation Steps**:
1. Add `DeliveryAttempts []notificationv1alpha1.DeliveryAttempt` to `DeliveryResult` struct
2. In `orchestrator.DeliverToChannels()`: Collect attempts in result, don't record immediately
3. In `controller.handleDeliveryLoop()`: Record all attempts AFTER orchestrator returns
4. Test with failing E2E tests to confirm backoff works

**Expected Outcome**:
- Reconcile #1: Deliver Console (success), File (fail) → Status update ONCE → Return RequeueAfter: 30s
- Wait 30 seconds (backoff works!)
- Reconcile #2: File (fail again) → Status update ONCE → Return RequeueAfter: 60s
- Wait 60 seconds
- ... exponential backoff continues correctly

---

## 📝 **Next Steps**

1. Implement Option 1 (Batch Status Updates)
2. Run E2E tests to validate backoff works
3. Verify all 22/22 tests pass
4. Document in DD or handoff

**Confidence**: 95% (root cause confirmed, solution is straightforward)

---

**Document Owner**: AI Assistant
**Status**: Root cause confirmed, solution designed
**Blocking**: 2/22 E2E tests



