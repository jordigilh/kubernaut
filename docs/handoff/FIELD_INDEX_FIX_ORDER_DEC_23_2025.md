# Field Index Fix - Client Retrieval Order - Dec 23, 2025

## ROOT CAUSE FOUND

**Problem**: Getting client from manager BEFORE field indexes are registered.

## Evidence from Cluster API Testing Guide

Per [Cluster API Testing Documentation](https://release-1-0.cluster-api.sigs.k8s.io/developer/testing):

```golang
setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
    if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
        panic(fmt.Sprintf("unable to setup index: %v", err))
    }
}

setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
    if err := (&MyReconciler{
        Client:  mgr.GetClient(),  // ← Get client AFTER indexes are set up
        // ...
    }).SetupWithManager(mgr, ...); err != nil {
        // ...
    }
}

os.Exit(envtest.Run(ctx, envtest.RunInput{
    SetupIndexes:     setupIndexes,     // ← Indexes FIRST
    SetupReconcilers: setupReconcilers,  // ← Reconcilers SECOND (get client here)
}))
```

## Our Incorrect Order

```golang
// suite_test.go current order:
k8sManager = ctrl.NewManager(...)                 // Line 209 ✅
k8sClient = k8sManager.GetClient()               // Line 220 ❌ TOO EARLY!
// ... 50 lines later ...
reconciler.SetupWithManager(k8sManager)          // Line 271 ← Registers field index
```

**Result**: The client object is retrieved before the field index exists, so it doesn't know about our custom `spec.signalFingerprint` index.

## Fix Required

Move client retrieval to AFTER field index registration:

```golang
// Create manager
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{...})
Expect(err).ToNot(HaveOccurred())

// Setup reconciler (registers field indexes via SetupWithManager)
reconciler := controller.NewReconciler(...)
err = reconciler.SetupWithManager(k8sManager)  // ← Registers field index HERE
Expect(err).ToNot(HaveOccurred())

// NOW get client (after indexes are registered)
k8sClient = k8sManager.GetClient()
Expect(k8sClient).NotTo(BeNil())
```

## File to Fix

`test/integration/remediationorchestrator/suite_test.go`

- **Remove line 220**: `k8sClient = k8sManager.GetClient()`
- **Add AFTER line 272** (after `SetupWithManager`): `k8sClient = k8sManager.GetClient()`

## Expected Result

After fix:
- ✅ Field index registered in `SetupWithManager()`
- ✅ Client retrieved AFTER indexes are ready
- ✅ `client.MatchingFields{"spec.signalFingerprint": ...}` will work
- ✅ Smoke test will pass
- ✅ NC-INT-4 test will pass

---

**Status**: Ready to implement
**Priority**: HIGH - Blocks 2 integration tests
**References**: [Cluster API Testing Guide](https://release-1-0.cluster-api.sigs.k8s.io/developer/testing)




