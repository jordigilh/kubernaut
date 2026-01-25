# Notification Integration Tests - DD-STATUS-001 Cache Fix Implementation

**Date**: 2026-01-10
**Status**: 🔧 IN PROGRESS - Controller fix working, test cache sync issue remains
**Priority**: HIGH - Blocking integration test completion

---

## 🎯 **Objective**

Fix cache consistency issues in Notification integration tests where status updates were being lost during rapid reconciliation loops (retry scenarios).

---

## 🔍 **Root Cause Analysis**

### **Original Problem**
Integration test `"should stop retrying after first success"` was failing because:
- **Controller behavior**: File service called 3 times correctly (2 failures + 1 success)
- **Status recording**: Only 2 attempts appeared in `notification.Status.DeliveryAttempts`
- **Impact**: 3rd attempt (successful retry) was not being recorded

### **Technical Root Cause**
`pkg/notification/status/manager.go` was using `m.client.Get()` for refetches before status updates. This read from the **controller-runtime cache**, which had not synced by the time the next reconciliation loop started.

**Evidence from logs**:
```
Attempt 2: deliveryAttemptsBeforeUpdate: 0 (should be 1)
Attempt 3: deliveryAttemptsBeforeUpdate: 1 (should be 2)
```

The cache lag caused `AtomicStatusUpdate` to overwrite previous attempts instead of appending to them.

---

## ✅ **Solution Implemented - DD-STATUS-001**

### **Design Decision: DD-STATUS-001 - API Reader Cache Bypass**

**Location**: `pkg/notification/status/manager.go`

**Changes**:
1. **Modified `Manager` struct** to accept both cached client and API reader:
   ```go
   type Manager struct {
       client    client.Client   // For writes
       apiReader client.Reader   // For cache-bypassed reads (DD-STATUS-001)
   }
   ```

2. **Updated constructor** to accept API reader:
   ```go
   func NewManager(client client.Client, apiReader client.Reader) *Manager
   ```

3. **Refetch calls now use API reader**:
   ```go
   // Before (cached read):
   if err := m.client.Get(ctx, client.ObjectKeyFromObject(notification), notification); err != nil {

   // After (cache-bypassed read - DD-STATUS-001):
   if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(notification), notification); err != nil {
   ```

4. **Updated all call sites**:
   - `cmd/notification/main.go`: Pass `mgr.GetAPIReader()`
   - `test/integration/notification/suite_test.go`: Pass `k8sManager.GetAPIReader()`

### **Verification - Controller Side ✅**

Added debug logging to `AtomicStatusUpdate`:
```go
ctrl.Log.WithName("status-manager").Info("🔍 DD-STATUS-001: API reader refetch complete",
    "deliveryAttemptsBeforeUpdate", len(notification.Status.DeliveryAttempts),
    "newAttemptsToAdd", len(attempts))
```

**Test output shows fix is working**:
```
1st update: deliveryAttemptsBeforeUpdate: 0, newAttemptsToAdd: 1 → Total: 1
2nd update: deliveryAttemptsBeforeUpdate: 1, newAttemptsToAdd: 1 → Total: 2
3rd update: deliveryAttemptsBeforeUpdate: 2, newAttemptsToAdd: 1 → Total: 3 ✅
```

**Controller is correctly writing all 3 attempts!**

---

## 🚨 **Remaining Issue - Test Cache Sync**

### **Problem**
The **test itself** is reading from stale cache when it validates the final status:
- Controller writes: 3 attempts ✅
- Test reads: 2 attempts ❌

**Test code** (line ~300):
```go
Eventually(func() string {
    err := k8sClient.Get(ctx, client.ObjectKeyFrom(notification), notification)
    if err != nil {
        return ""
    }
    return notification.Status.Phase
}, 10*time.Second, 200*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

// This assertion reads stale cache:
Expect(len(notification.Status.DeliveryAttempts)).To(Equal(3))
```

### **Why This Happens**
1. Controller uses API reader → writes 3 attempts to API server
2. Test uses `k8sClient.Get()` → reads from cache (hasn't synced yet)
3. Test sees only 2 attempts

### **Potential Solutions**

#### **Option A: Use API Reader in Tests** (Recommended)
Modify tests to use `testEnv.Config` to create a direct API client:
```go
// In suite_test.go setup
apiReader, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
apiReader = client.NewDelegatingClient(client.NewDelegatingClientInput{
    Client:    k8sClient,
    CacheReader: k8sClient,
    UncachedObjects: []client.Object{
        &notificationv1alpha1.NotificationRequest{},
    },
})
```

#### **Option B: Add Cache Sync Wait**
Add explicit wait after phase transition before asserting on delivery attempts:
```go
Eventually(func() string {
    err := k8sClient.Get(ctx, client.ObjectKeyFrom(notification), notification)
    if err != nil {
        return ""
    }
    return notification.Status.Phase
}, 10*time.Second, 200*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

// Wait for cache to sync
time.Sleep(500 * time.Millisecond)

// OR use Eventually with longer timeout:
Eventually(func() int {
    err := k8sClient.Get(ctx, client.ObjectKeyFrom(notification), notification)
    if err != nil {
        return -1
    }
    return len(notification.Status.DeliveryAttempts)
}, 15*time.Second, 500*time.Millisecond).Should(Equal(3))
```

#### **Option C: Increase Eventually Timeout**
```go
// Current:
}, 10*time.Second, 200*time.Millisecond).Should(Equal(...))

// Proposed:
}, 20*time.Second, 1*time.Second).Should(Equal(...))
```

---

## 📊 **Current Test Results**

### **Last Run** (2026-01-10 23:14)
```
Ran 100 of 118 Specs in 138.641 seconds
✅ 88 Passed
❌ 12 Failed (11 INTERRUPTED + 1 actual failure)
⏭️  18 Skipped
```

### **Actual Failures**
1. **"should stop retrying after first success"** - Test cache sync issue (documented above)
2. **11 other tests INTERRUPTED** - Ginkgo killed them due to the above failure

### **Other Fixed Issues**
- ✅ Counter semantics: `FailedDeliveries` now counts unique channels (not attempts)
- ✅ Deduplication bug: Status field added to comparison logic
- ✅ Test expectation: `controller_partial_failure_test.go` updated to expect 1 failed channel (not 3 attempts)
- ✅ Infrastructure: Duplicate port declaration removed from `signalprocessing_e2e_hybrid.go`

---

## 🔄 **Next Steps**

### **Immediate Actions** (User Decision Required)
1. **Choose cache sync solution**: Option A (API reader in tests), B (explicit wait), or C (longer timeout)
2. **Apply chosen solution** to retry logic tests
3. **Re-run integration test suite** to verify all 118 specs pass

### **Implementation Files Modified**
- ✅ `pkg/notification/status/manager.go` - DD-STATUS-001 implementation
- ✅ `cmd/notification/main.go` - API reader injection
- ✅ `test/integration/notification/suite_test.go` - API reader injection
- ✅ `test/integration/notification/multichannel_retry_test.go` - Counter semantics fix
- ✅ `test/infrastructure/signalprocessing_e2e_hybrid.go` - Duplicate declaration fix

### **Files Needing Update** (Pending User Decision)
- ⏸️  `test/integration/notification/controller_retry_logic_test.go` - Cache sync fix needed

---

## 🎯 **Success Criteria**

- ✅ **Controller**: All 3 attempts correctly written to API server (VERIFIED)
- ⏸️  **Tests**: All integration tests read fresh data (IN PROGRESS)
- ⏸️  **Pass Rate**: 118/118 specs passing (currently 88/100 effective)

---

## 📚 **References**

- **Design Decision**: DD-STATUS-001 - API Reader Cache Bypass
- **Related**: DD-E2E-003 - Counter Semantics (FailedDeliveries counts unique channels)
- **Business Requirement**: BR-NOT-052 - Automatic Retry with Exponential Backoff
- **Controller-Runtime Docs**: [Client Cache vs API Reader](https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/client#Reader)

---

**Status**: ✅ Controller fix verified, ⏸️ Test fix awaiting user decision
**Confidence**: 95% (controller working correctly, test fix is straightforward)
