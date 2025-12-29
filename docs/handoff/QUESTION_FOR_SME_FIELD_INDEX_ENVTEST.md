# Question for SME: Field Index Not Working in envtest

## ⚠️ STATUS: QUESTION RESOLVED - SETUP BUG FOUND

**This document is DEPRECATED**. The question was based on incorrect setup.

**✅ RESOLUTION**:
- Problem was client retrieval order (client retrieved BEFORE field index registration)
- After fixing setup order, field indexes work correctly in envtest
- See `GW_FIELD_INDEX_SETUP_GUIDE_DEC_23_2025.md` for corrected implementation

**SME Response**: Confirmed field indexes DO work in envtest with proper setup (indexes first, then get client).

**Reference**: [Cluster API Testing Guide](https://release-1-0.cluster-api.sigs.k8s.io/developer/testing)

---

## Original Question (RESOLVED - Kept for Historical Context)

### Problem Summary
Field index query on custom spec field (`spec.signalFingerprint`) fails in envtest despite following documented patterns. All setup appears correct, but `client.MatchingFields` query returns 0 results.

## Environment
- **controller-runtime**: v0.22.4
- **Test Framework**: Ginkgo/Gomega
- **Environment**: envtest (real API server + etcd)

## What We're Trying to Do
Query RemediationRequest CRDs by a custom spec field using field index:
```go
rrList := &remediationv1.RemediationRequestList{}
err := k8sClient.List(ctx, rrList,
    client.InNamespace(testNamespace),
    client.MatchingFields{"spec.signalFingerprint": fingerprint},
)
```

## Our Setup (Following Best Practices)

### 1. Field Index Registration (Production Code)
```go
// internal/controller/remediationorchestrator/reconciler.go lines 1543-1556
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &remediationv1.RemediationRequest{},
        "spec.signalFingerprint", // Index key
        func(obj client.Object) []string {
            rr := obj.(*remediationv1.RemediationRequest)
            if rr.Spec.SignalFingerprint == "" {
                return nil
            }
            return []string{rr.Spec.SignalFingerprint}
        },
    ); err != nil {
        return fmt.Errorf("failed to create field index on spec.signalFingerprint: %w", err)
    }
    // ... rest of setup
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1.RemediationRequest{}).
        Complete(r)
}
```

### 2. Test Setup
```go
// test/integration/remediationorchestrator/suite_test.go (simplified)

// Create envtest environment
testEnv = &envtest.Environment{
    CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
}
cfg, err := testEnv.Start()

// Create temporary client for namespace setup
tempClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})

// Create namespaces using temp client
tempClient.Create(ctx, systemNamespace)

// Create manager
k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
    Scheme: scheme.Scheme,
    Metrics: metricsserver.Options{BindAddress: "0"},
})

// Register field index via reconciler setup
reconciler := controller.NewReconciler(k8sManager.GetClient(), ...)
err = reconciler.SetupWithManager(k8sManager)  // ← Registers field index here

// CRITICAL: Use manager's cached client (not direct client)
k8sClient = k8sManager.GetClient()

// Start manager in background
go func() {
    err = k8sManager.Start(ctx)
}()

// Wait for cache to sync
if !k8sManager.GetCache().WaitForCacheSync(cacheSyncCtx) {
    Fail("Failed to sync cache")
}
time.Sleep(1 * time.Second)  // Additional buffer
```

### 3. Test Execution
```go
// Test creates RR, then queries by field index
testNamespace := createTestNamespace("notifications")

rr := &remediationv1.RemediationRequest{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "rr-notification-labels",
        Namespace: testNamespace,
    },
    Spec: remediationv1.RemediationRequestSpec{
        SignalFingerprint: "d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5", // 64 chars
        SignalName:        "test-signal",
        // ... other required fields
    },
}
Expect(k8sClient.Create(ctx, rr)).To(Succeed())

// Wait briefly for cache population
time.Sleep(100 * time.Millisecond)

// Query by field index
rrList := &remediationv1.RemediationRequestList{}
err := k8sClient.List(ctx, rrList,
    client.InNamespace(testNamespace),
    client.MatchingFields{"spec.signalFingerprint": fingerprint},
)
Expect(err).ToNot(HaveOccurred())
Expect(len(rrList.Items)).To(BeNumerically(">=", 1))  // ← FAILS: Returns 0
```

## What Happens
- Test fails at expectation: `len(rrList.Items) == 0` (expected >= 1)
- Test runs very fast (0.010 seconds), suggesting immediate failure
- No error returned from `List()` call
- Direct query without field selector works fine:
  ```go
  k8sClient.List(ctx, rrList, client.InNamespace(testNamespace))  // Returns 1 RR ✅
  ```

## What We've Verified
✅ Field index registration completes without error
✅ Manager starts successfully
✅ Cache syncs successfully
✅ Using manager's cached client (not direct client)
✅ RR is created successfully
✅ RR has correct fingerprint (64 chars)
✅ Namespace matches
✅ CRD is properly installed (envtest loads from config/crd/bases)

## What We've Tried
1. ❌ Using direct client → switched to manager's cached client
2. ❌ Wrong fingerprint length (63 chars) → fixed to 64 chars
3. ❌ Added sleep after RR creation → still fails
4. ❌ Verified cache sync → already working

## Similar Working Code (Gateway Service)
Gateway service has identical pattern that works in production, but has a fallback in integration tests:
```go
// pkg/gateway/processing/phase_checker.go
err := c.client.List(ctx, rrList,
    client.InNamespace(namespace),
    client.MatchingFields{"spec.signalFingerprint": fingerprint},
)
if err != nil && strings.Contains(err.Error(), "field label not supported") {
    // Fallback: list all and filter in-memory
    err := c.client.List(ctx, rrList, client.InNamespace(namespace))
    // ... filter in memory
}
```

We're not getting the "field label not supported" error - we get **no error, but empty results**.

## Questions for SME

1. **Is there a race condition** between field index registration and first query?
   - Should we wait for something specific after `SetupWithManager()`?

2. **Does field index work on CRD spec fields** or only built-in types?
   - The example shows `spec.nodeName` on Pod (built-in type)
   - We're using `spec.signalFingerprint` on RemediationRequest (custom CRD)

3. **Is there a difference** between how the manager client cache handles field indexes vs. how we're expecting?

4. **Do we need to force a cache refresh** after creating the RR and before querying?

5. **Is the field index key correct?** Should it be:
   - `"spec.signalFingerprint"` (what we're using)
   - `".spec.signalFingerprint"` (with leading dot)
   - Something else?

## Minimal Reproduction
Can provide a minimal reproduction if needed. Current test file:
- `test/integration/remediationorchestrator/notification_creation_integration_test.go` (line 344)
- Test name: `NC-INT-4: Notification Labels and Correlation`

## Expected Behavior
Query should return the RR we just created, since its `spec.signalFingerprint` matches the query value.

## Actual Behavior
Query returns empty list (0 items) with no error.

---

**Environment Details**:
- Go: 1.25.0
- OS: macOS (darwin 24.6.0)
- Test command: `make test-integration-remediationorchestrator`

**Related Files**:
- Field index setup: `internal/controller/remediationorchestrator/reconciler.go:1543-1556`
- Test setup: `test/integration/remediationorchestrator/suite_test.go:206-336`
- Failing test: `test/integration/remediationorchestrator/notification_creation_integration_test.go:337-347`

