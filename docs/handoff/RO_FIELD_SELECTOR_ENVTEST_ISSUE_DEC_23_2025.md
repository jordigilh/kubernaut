# RO Field Selector envtest Issue - Dec 23, 2025

## Summary
Field selector on `spec.signalFingerprint` is not working in envtest environment, causing NC-INT-4 test to fail. The field index is properly set up in `reconciler.SetupWithManager()`, but queries fail silently.

## Current Status

### Test Results
- **45 PASSED** / **6 FAILED** (NC-INT-4 back to failing after removing fallback)
- **Test Duration**: 0.006 seconds (fails immediately)
- **Error**: No detailed error message captured (fails silently)

### What We Know

#### Field Index Setup (CORRECT)
```go
// internal/controller/remediationorchestrator/reconciler.go lines 1543-1556
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    // BR-ORCH-042, BR-GATEWAY-185 v1.1: Create field index on spec.signalFingerprint
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &remediationv1.RemediationRequest{},
        FingerprintFieldIndex, // "spec.signalFingerprint"
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
}
```

#### Test Suite Setup (CORRECT)
```go
// test/integration/remediationorchestrator/suite_test.go lines 253-263
reconciler := controller.NewReconciler(
    k8sManager.GetClient(),
    k8sManager.GetScheme(),
    auditStore,
    nil,
    roMetrics,
    controller.TimeoutConfig{},
    routingEngine,
)
err = reconciler.SetupWithManager(k8sManager)  // This sets up field index
Expect(err).ToNot(HaveOccurred())
```

#### Test Query (CORRECT SYNTAX)
```go
// test/integration/remediationorchestrator/notification_creation_integration_test.go lines 341-347
rrList := &remediationv1.RemediationRequestList{}
err = k8sClient.List(ctx, rrList, client.InNamespace(testNamespace), client.MatchingFields{
    "spec.signalFingerprint": fingerprint, // Full 64-char SHA256 fingerprint
})
Expect(err).ToNot(HaveOccurred(), "Field selector should work in envtest (field index set up by reconciler.SetupWithManager)")
Expect(len(rrList.Items)).To(BeNumerically(">=", 1), "Should find RemediationRequest by fingerprint field")
```

### What's Failing
The `k8sClient.List()` call with `client.MatchingFields` is failing, but the error is not being captured in the test output.

## Root Cause Hypothesis

### Hypothesis 1: envtest Client vs. Manager Client Mismatch
**Problem**: The field index is registered with `k8sManager.GetFieldIndexer()`, but the test uses `k8sClient` which might be a different client instance.

**Evidence**:
- Field index setup uses `mgr.GetFieldIndexer()`
- Test queries use `k8sClient` (from suite setup)
- These might not be the same client instance

**Verification Needed**:
```go
// In suite_test.go, check if:
k8sClient == k8sManager.GetClient()  // Are these the same instance?
```

### Hypothesis 2: envtest Doesn't Support Field Selectors on Custom Fields
**Problem**: envtest might only support field selectors on built-in Kubernetes fields (metadata.name, metadata.namespace), not custom spec fields.

**Evidence**:
- Gateway team added fallback specifically for "tests without field index"
- Test fails immediately (0.006s) suggesting infrastructure issue, not logic issue
- No error message captured (silent failure)

**Verification Needed**:
- Check envtest documentation for field selector support
- Test with a simple field selector on metadata.name (known to work)

### Hypothesis 3: Field Index Not Synced Before Test Runs
**Problem**: Field index might be registered but not yet synced/active when test runs.

**Evidence**:
- Test runs very quickly after setup
- No explicit wait for field index to be ready

**Verification Needed**:
- Add explicit wait/retry logic after field index setup
- Check if other integration tests have similar patterns

## Recommended Investigation Steps

### Step 1: Verify Client Instance
```go
// In suite_test.go after line 263
By("Verifying client instances match")
Expect(k8sClient).To(Equal(k8sManager.GetClient()),
    "k8sClient should be same instance as k8sManager.GetClient()")
```

### Step 2: Test with Built-in Field Selector
```go
// In NC-INT-4 test, add before fingerprint query
By("Testing built-in field selector (metadata.name)")
testList := &remediationv1.RemediationRequestList{}
err = k8sClient.List(ctx, testList, client.InNamespace(testNamespace), client.MatchingFields{
    "metadata.name": rrName,
})
Expect(err).ToNot(HaveOccurred(), "Built-in field selector should work")
```

### Step 3: Add Detailed Error Logging
```go
// In NC-INT-4 test, change query to:
err = k8sClient.List(ctx, rrList, client.InNamespace(testNamespace), client.MatchingFields{
    "spec.signalFingerprint": fingerprint,
})
if err != nil {
    GinkgoWriter.Printf("Field selector error: %v\n", err)
    GinkgoWriter.Printf("Error type: %T\n", err)
    GinkgoWriter.Printf("Error details: %+v\n", err)
}
Expect(err).ToNot(HaveOccurred())
```

### Step 4: Check Field Index Registration
```go
// In suite_test.go after line 263
By("Verifying field index was registered")
// Try a simple query to see if it fails with "field label not supported"
testRRList := &remediationv1.RemediationRequestList{}
testErr := k8sClient.List(context.Background(), testRRList,
    client.MatchingFields{"spec.signalFingerprint": "test-fingerprint"})
if testErr != nil {
    GinkgoWriter.Printf("Field index test error: %v\n", testErr)
}
```

## Temporary Workaround Options

### Option A: Skip NC-INT-4 Test (NOT RECOMMENDED)
Mark test as `PIt()` (pending) until envtest issue is resolved.

**Pros**: Unblocks other tests
**Cons**: Doesn't validate field selector functionality

### Option B: Use Label-Based Query (COMPROMISED)
Change test to use truncated fingerprint in labels (63 chars).

**Pros**: Test passes
**Cons**: Doesn't test actual production behavior (field selectors)

### Option C: Move to E2E Only (RECOMMENDED)
Remove NC-INT-4 from integration tests, add to E2E tests where field selectors work in real Kind cluster.

**Pros**: Tests actual production behavior
**Cons**: Slower test execution

## Related Issues

### Gateway Team Has Same Problem
Gateway service has production fallback code in `pkg/gateway/processing/phase_checker.go` (lines 107-123) that catches field selector errors.

**GW Team Document**: `docs/handoff/GW_PRODUCTION_FALLBACK_CODE_SMELL_DEC_23_2025.md`

**Question for GW Team**: How do you test field selectors in your integration tests? Do you have the same envtest issue?

## Files Modified
- `test/integration/remediationorchestrator/notification_creation_integration_test.go` (lines 337-347)
  - Removed fallback pattern
  - Added clear error message
  - Fixed fingerprint length (63 → 64 chars)

## Files Created
- `docs/handoff/GW_PRODUCTION_FALLBACK_CODE_SMELL_DEC_23_2025.md` - GW team code smell report
- `docs/handoff/RO_FIELD_SELECTOR_UPDATE_DEC_23_2025.md` - Field selector implementation details

## Next Steps

1. **Investigate**: Run Step 1-4 above to identify root cause
2. **Decide**: Based on findings, choose approach:
   - Fix envtest setup (if possible)
   - Move test to E2E (if envtest limitation)
   - Document limitation (if envtest doesn't support custom field selectors)
3. **Coordinate with GW**: Ask how they handle this in their tests

## Questions for User

1. Should we investigate the root cause now, or defer to later?
2. Is it acceptable to move NC-INT-4 to E2E tests if envtest doesn't support field selectors?
3. Should we coordinate with GW team on this issue since they have the same problem?

---

**Created**: Dec 23, 2025
**Status**: Investigation Needed
**Priority**: Medium (blocks 1 integration test)
**Related**: GW_PRODUCTION_FALLBACK_CODE_SMELL_DEC_23_2025.md, RO_FIELD_SELECTOR_UPDATE_DEC_23_2025.md




