# Notification Status Update Race Condition - Regression Analysis

**Date**: January 22, 2026
**Priority**: P1 - CRITICAL REGRESSION
**Test**: `Controller Retry Logic (BR-NOT-054) → should stop retrying after first success`
**Status**: 116/117 passing (1 regression)

---

## 📊 Failure Summary

**Expected**: 3 delivery attempts recorded in status (fail, fail, success)
**Actual**: 2 delivery attempts recorded in status

**Test Configuration**:
- Mock file service configured to fail twice, succeed on 3rd attempt
- RetryPolicy: MaxAttempts=5, InitialBackoffSeconds=1, BackoffMultiplier=2

---

## 🔍 Root Cause Analysis (Must-Gather Logs)

### Delivery Attempt Timeline

```
Line 33679: Attempt 1
  persisted: 0, inFlight: 1, total: 1
  Result: "simulated transient failure (attempt 1)" ✓
  Audit: notification.message.failed

Line 33719: Attempt 2
  persisted: 0, inFlight: 1, total: 1  ← 🚨 STILL 0 PERSISTED!
  Result: "simulated transient failure (attempt 2)" ✓
  Audit: notification.message.failed (NOT FOUND - never reached status)

Line 33755: Attempt 3
  persisted: 1, inFlight: 1, total: 2  ← Only 1 attempt persisted
  Result: "Delivery successful" ✓
  Final Status: totalAttempts=2 (should be 3)
```

### Status Update Race Window

After attempt 1's atomic status update:
```
Next reconcile shows:
  "totalAttempts": 0, "deliveryAttemptCount": 0
```

**This proves**: The first attempt's status update was **not persisted** before attempt 2 ran.

---

## 🧬 Technical Analysis

### Current Status Update Flow

```mermaid
sequenceDiagram
    Attempt1->>StatusManager: AtomicStatusUpdate(1 attempt)
    StatusManager->>APIReader: Refetch notification
    StatusManager->>APIServer: Update status
    Note over APIServer: Status write in progress...

    Attempt2->>StatusManager: AtomicStatusUpdate(1 attempt)
    StatusManager->>APIReader: Refetch notification
    Note over StatusManager: STALE READ - Attempt1 not persisted yet!
    StatusManager->>APIServer: Update status (overwrites Attempt1)

    Attempt3->>StatusManager: AtomicStatusUpdate(1 attempt)
    StatusManager->>APIReader: Refetch notification
    Note over StatusManager: Only sees Attempt2 (Attempt1 was overwritten)
    StatusManager->>APIServer: Update status
    Result: Only 2 attempts recorded
```

### Why DD-PERF-001 & SP-CACHE-001 Didn't Prevent This

The current implementation uses `apiReader` to bypass the cache, but:
1. **API Server Propagation Lag**: Even with `apiReader`, there's a window where the API server hasn't finished persisting the previous update
2. **Rapid Reconciles**: With backoff timing, multiple reconciles can occur within milliseconds
3. **No Optimistic Locking**: The status update doesn't use `RetryOnConflict` or resource version checking

---

## 🎯 Proposed Solutions

### Option A: Add Optimistic Locking to Status Updates (RECOMMENDED)

**Implementation**:
```go
// pkg/notification/status/manager.go
func (m *Manager) AtomicStatusUpdate(...) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Refetch with apiReader for fresh data
        if err := m.apiReader.Get(ctx, key, notification); err != nil {
            return err
        }

        // Apply status changes
        notification.Status.DeliveryAttempts = append(
            notification.Status.DeliveryAttempts,
            newAttempts...
        )

        // Update will fail if resource version changed
        return m.client.Status().Update(ctx, notification)
    })
}
```

**Benefits**:
- ✅ Kubernetes-native solution
- ✅ Handles concurrent updates automatically
- ✅ Retries on conflict until success
- ✅ No application-level locking needed

**Risks**:
- Could increase API server load under high concurrency
- May require tuning retry backoff

---

### Option B: Controller-Level Reconcile Serialization

**Implementation**:
```go
// internal/controller/notification/notificationrequest_controller.go
func (r *NotificationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Acquire per-notification lock
    r.reconcileLock.Lock(req.NamespacedName.String())
    defer r.reconcileLock.Unlock(req.NamespacedName.String())

    // ... rest of reconcile logic
}
```

**Benefits**:
- ✅ Guarantees serial processing per notification
- ✅ Simpler reasoning about state

**Risks**:
- ❌ Reduces concurrency
- ❌ Could block legitimate parallel notifications
- ❌ Adds complexity to controller

---

### Option C: In-Memory Status Tracking with Periodic Sync

**Implementation**: Similar to in-flight counter pattern, but for status.

**Benefits**:
- ✅ Eliminates API server round-trips for status reads
- ✅ Can batch multiple status updates

**Risks**:
- ❌ Memory state can diverge from API server
- ❌ Requires careful synchronization on startup/restart
- ❌ More complex than Option A

---

## 💡 Recommendation

**Implement Option A: Optimistic Locking with `RetryOnConflict`**

**Justification**:
1. **Kubernetes-Native**: Uses built-in conflict resolution
2. **Minimal Code Changes**: Only affects `AtomicStatusUpdate` method
3. **Proven Pattern**: Used successfully in SignalProcessing service
4. **DD-PERF-001 Compatible**: Enhances existing atomic update pattern

**Implementation Plan**:
1. Modify `pkg/notification/status/manager.go` to wrap `Status().Update()` with `RetryOnConflict`
2. Add conflict resolution metrics to track contention
3. Add integration test to verify rapid concurrent status updates
4. Document in `DD-PERF-001` as enhancement

---

## 📝 Test Case to Validate Fix

```go
It("should handle concurrent status updates without loss", func() {
    // Create notification with multiple channels
    notification := createNotification(5 channels)

    // Simulate rapid concurrent reconciles
    var wg sync.WaitGroup
    for i := 0; i < 5; i++ {
        wg.Add(1)
        go func(attempt int) {
            defer wg.Done()
            // Each goroutine attempts to record a delivery
            deliveryAttempt := createAttempt(channel[attempt])
            err := statusManager.AtomicStatusUpdate(ctx, notification, deliveryAttempt)
            Expect(err).ToNot(HaveOccurred())
        }(i)
    }
    wg.Wait()

    // Verify all 5 attempts were recorded
    Eventually(func() int {
        err := k8sAPIReader.Get(ctx, key, notification)
        if err != nil {
            return -1
        }
        return len(notification.Status.DeliveryAttempts)
    }, 5*time.Second, 200*time.Millisecond).Should(Equal(5),
        "All concurrent status updates should be persisted")
})
```

---

## 🔗 Related Issues

- **Previous Fix**: `DD-PERF-001` + `SP-CACHE-001` (cache-bypassed reads)
- **Similar Pattern**: SignalProcessing uses `RetryOnConflict` successfully
- **Root Issue**: API server propagation lag under rapid reconciles

---

## ✅ Success Criteria

1. All 117 Notification integration tests pass
2. Retry-until-success test records all 3 attempts correctly
3. No performance degradation under normal load
4. Conflict resolution metrics show successful retries

---

**Next Steps**: Proceed with Option A implementation and validation.
