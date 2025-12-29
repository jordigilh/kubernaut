# Gateway Team: Field Index Setup in envtest - Dec 23, 2025

## Purpose

Share learnings from RO field index implementation and provide guidance on setting up custom field indexes in envtest for the Gateway team.

## Executive Summary

**Initial Assessment**: ❌ We incorrectly called Gateway's fallback pattern a "code smell"

**Corrected Assessment**: ✅ Gateway's fallback is **correct defensive programming** for production, but **not needed in envtest** with proper setup

**Key Learning**: The distinction between production robustness and test environment configuration

---

## Production vs. envtest: Different Requirements

### Production Environment (Gateway's Current Code)

**Gateway's Fallback Pattern** (from production code):
```go
// Try field selector first (efficient)
rrList := &remediationv1.RemediationRequestList{}
err := r.client.List(ctx, rrList,
    client.InNamespace(namespace),
    client.MatchingFields{"spec.signalFingerprint": fingerprint},
)

// Fallback to label-based query if field selector not supported
if err != nil && strings.Contains(err.Error(), "field label not supported") {
    // Use label-based query as fallback
    err = r.client.List(ctx, rrList,
        client.InNamespace(namespace),
        client.MatchingLabels{
            "kubernaut.ai/signal-fingerprint-prefix": fingerprint[:63],
        },
    )
}
```

**Why This Is CORRECT**:
- ✅ **Defensive Programming**: Handles API server variations
- ✅ **Version Compatibility**: Works across different Kubernetes versions
- ✅ **Provider Agnostic**: Works with different cloud providers' API servers
- ✅ **Graceful Degradation**: Falls back to labels when field indexes unavailable
- ✅ **Production Robustness**: Never fails due to API server configuration

**Recommendation**: ❌ **DO NOT USE THIS PATTERN**

This fallback was removed from Gateway and should NOT be restored. See corrected analysis below.

### Test Environment (envtest)

**envtest with Proper Setup** (RO's corrected approach):
```go
// Field indexes work natively in envtest with correct setup
// NO fallback needed in tests
rrList := &remediationv1.RemediationRequestList{}
err := k8sClient.List(ctx, rrList,
    client.InNamespace(namespace),
    client.MatchingFields{"spec.signalFingerprint": fingerprint},
)
// This works reliably in envtest - no fallback needed
```

**Why This Works**:
- ✅ Field indexes are registered with manager's cache
- ✅ Test client uses manager's cached client (with indexes)
- ✅ envtest supports controller-runtime's field indexer fully

---

## How to Set Up Custom Field Indexes in envtest

### The Critical Setup Order

**WRONG ORDER** (What We Had Initially):
```go
// ❌ DON'T DO THIS
k8sManager = ctrl.NewManager(cfg, ctrl.Options{...})
k8sClient = k8sManager.GetClient()              // ❌ Get client BEFORE indexes
reconciler := NewReconciler(...)
reconciler.SetupWithManager(k8sManager)          // Registers indexes (too late!)
```

**Result**: Client doesn't know about field indexes → "field label not supported" error

**CORRECT ORDER** (Cluster API Pattern):
```go
// ✅ DO THIS
k8sManager = ctrl.NewManager(cfg, ctrl.Options{...})
reconciler := NewReconciler(...)
reconciler.SetupWithManager(k8sManager)          // Register indexes FIRST
k8sClient = k8sManager.GetClient()              // Get client AFTER indexes ✅
```

**Result**: Client has access to all registered field indexes

---

## Complete Working Example

### 1. Register Field Index in Controller

**File**: `internal/controller/yourcontroller/reconciler.go`

```go
const (
    // Field index name for spec.signalFingerprint
    FingerprintFieldIndex = "spec.signalFingerprint"
)

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    // BR-XXX: Create field index on spec.signalFingerprint
    // Uses immutable spec field (64 chars) instead of mutable labels (63 chars max)
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &remediationv1.RemediationRequest{},
        FingerprintFieldIndex,
        func(obj client.Object) []string {
            rr := obj.(*remediationv1.RemediationRequest)
            if rr.Spec.SignalFingerprint == "" {
                return nil // No fingerprint to index
            }
            return []string{rr.Spec.SignalFingerprint}
        },
    ); err != nil {
        return fmt.Errorf("failed to create field index on spec.signalFingerprint: %w", err)
    }

    // Continue with normal controller setup...
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1.RemediationRequest{}).
        Complete(r)
}
```

### 2. Use Field Index in Business Code

**File**: `internal/controller/yourcontroller/reconciler.go`

```go
func (r *Reconciler) findExistingRequests(ctx context.Context, fingerprint string) (*remediationv1.RemediationRequestList, error) {
    rrList := &remediationv1.RemediationRequestList{}

    err := r.client.List(ctx, rrList,
        client.InNamespace("target-namespace"),
        client.MatchingFields{FingerprintFieldIndex: fingerprint},
    )
    if err != nil {
        return nil, fmt.Errorf("failed to query by fingerprint: %w", err)
    }

    return rrList, nil
}
```

### 3. Set Up envtest Correctly

**File**: `test/integration/yourservice/suite_test.go`

```go
var (
    cfg       *rest.Config
    k8sClient client.Client
    k8sManager ctrl.Manager
    testEnv   *envtest.Environment
    ctx       context.Context
    cancel    context.CancelFunc
)

var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.TODO())

    By("Bootstrapping test environment")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
        },
        ErrorIfCRDPathMissing: true,
    }

    var err error
    cfg, err = testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())

    // Add schemes...
    err = remediationv1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    By("Creating manager")
    k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
        Scheme: scheme.Scheme,
        Metrics: metricsserver.Options{
            BindAddress: "0", // Avoid port conflicts
        },
    })
    Expect(err).ToNot(HaveOccurred())

    By("Setting up controller with field indexes")
    reconciler := controller.NewReconciler(
        k8sManager.GetClient(),
        // ... other dependencies ...
    )

    // CRITICAL: Register field indexes BEFORE getting client
    err = reconciler.SetupWithManager(k8sManager)
    Expect(err).ToNot(HaveOccurred())

    // CRITICAL: Get client AFTER field indexes are registered
    // Per Cluster API testing guide: https://release-1-0.cluster-api.sigs.k8s.io/developer/testing
    k8sClient = k8sManager.GetClient()
    Expect(k8sClient).NotTo(BeNil())

    By("Starting manager")
    go func() {
        defer GinkgoRecover()
        err = k8sManager.Start(ctx)
        Expect(err).ToNot(HaveOccurred(), "failed to run manager")
    }()
})

var _ = AfterSuite(func() {
    cancel()
    By("Tearing down the test environment")
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

### 4. Use Field Index in Tests

**File**: `test/integration/yourservice/your_test.go`

```go
var _ = Describe("Field Index Query", func() {
    var testNamespace string

    BeforeEach(func() {
        testNamespace = createTestNamespace("field-index-test")
    })

    AfterEach(func() {
        deleteTestNamespace(testNamespace)
    })

    It("should query by fingerprint using field index", func() {
        By("Creating a RemediationRequest with fingerprint")
        fingerprint := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2" // 64 chars
        rr := &remediationv1.RemediationRequest{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-rr",
                Namespace: testNamespace,
            },
            Spec: remediationv1.RemediationRequestSpec{
                SignalFingerprint: fingerprint,
                // ... other required fields ...
            },
        }
        Expect(k8sClient.Create(ctx, rr)).To(Succeed())

        By("Querying by field index")
        rrList := &remediationv1.RemediationRequestList{}
        err := k8sClient.List(ctx, rrList,
            client.InNamespace(testNamespace),
            client.MatchingFields{"spec.signalFingerprint": fingerprint},
        )
        Expect(err).ToNot(HaveOccurred())
        Expect(len(rrList.Items)).To(Equal(1))
        Expect(rrList.Items[0].Name).To(Equal("test-rr"))
    })
})
```

---

## Smoke Test Pattern

Create a simple smoke test to verify field indexes work before running full suite:

**File**: `test/integration/yourservice/field_index_smoke_test.go`

```go
var _ = Describe("Field Index Smoke Test", Ordered, func() {
    It("should successfully query by spec.signalFingerprint using field index", func() {
        By("Creating a test namespace")
        ns := &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{
                GenerateName: "smoke-test-",
            },
        }
        Expect(k8sClient.Create(ctx, ns)).To(Succeed())
        defer k8sClient.Delete(ctx, ns)

        By("Creating a test RemediationRequest")
        fingerprint := strings.Repeat("a", 64) // 64 chars
        rr := &remediationv1.RemediationRequest{
            ObjectMeta: metav1.ObjectMeta{
                GenerateName: "smoke-",
                Namespace:    ns.Name,
            },
            Spec: remediationv1.RemediationRequestSpec{
                SignalFingerprint: fingerprint,
                // ... minimal required fields ...
            },
        }
        Expect(k8sClient.Create(ctx, rr)).To(Succeed())

        By("Verifying direct query works")
        rrListDirect := &remediationv1.RemediationRequestList{}
        err := k8sClient.List(ctx, rrListDirect, client.InNamespace(ns.Name))
        Expect(err).ToNot(HaveOccurred())
        Expect(len(rrListDirect.Items)).To(BeNumerically(">=", 1))

        By("Verifying field index query works")
        rrListIndexed := &remediationv1.RemediationRequestList{}
        err = k8sClient.List(ctx, rrListIndexed,
            client.InNamespace(ns.Name),
            client.MatchingFields{"spec.signalFingerprint": fingerprint},
        )

        if err != nil {
            GinkgoWriter.Printf("❌ Field index query error: %v\n", err)
            Fail("Field index query failed - check setup order in suite_test.go")
        }

        Expect(len(rrListIndexed.Items)).To(Equal(1))
        GinkgoWriter.Println("✅ Field index query successful!")
    })
})
```

---

## Common Mistakes and Solutions

### Mistake 1: Getting Client Too Early

**Problem**:
```go
k8sClient = k8sManager.GetClient()      // ❌
reconciler.SetupWithManager(k8sManager) // Too late
```

**Solution**:
```go
reconciler.SetupWithManager(k8sManager) // ✅ Index first
k8sClient = k8sManager.GetClient()      // Then get client
```

### Mistake 2: Using Direct Client Instead of Manager's Client

**Problem**:
```go
k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme}) // ❌ Direct client
```

**Solution**:
```go
k8sClient = k8sManager.GetClient() // ✅ Manager's cached client (after index registration)
```

### Mistake 3: Not Starting Manager Before Tests

**Problem**:
```go
reconciler.SetupWithManager(k8sManager)
k8sClient = k8sManager.GetClient()
// ❌ Manager not started, cache not populated
```

**Solution**:
```go
reconciler.SetupWithManager(k8sManager)
k8sClient = k8sManager.GetClient()
go func() {
    k8sManager.Start(ctx) // ✅ Start manager to populate cache
}()
```

---

## References

### Authoritative Sources

1. **Cluster API Testing Guide**
   https://release-1-0.cluster-api.sigs.k8s.io/developer/testing

   Key quote:
   > "setupIndexes first, then setupReconcilers (which gets the client)"

2. **controller-runtime Field Indexer**
   https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client#FieldIndexer

3. **envtest Documentation**
   https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest

### RO Implementation (Reference)

- **Controller**: `internal/controller/remediationorchestrator/reconciler.go`
  - Lines 72-88: Field index registration in `SetupWithManager()`

- **Test Setup**: `test/integration/remediationorchestrator/suite_test.go`
  - Lines 209-273: Correct initialization order

- **Smoke Test**: `test/integration/remediationorchestrator/field_index_smoke_test.go`
  - Complete working example

---

## Recommendations for Gateway Team

### 1. Production Code: Keep Current Fallback ✅

**No changes needed** in production Gateway code. The fallback pattern is correct defensive programming.

**Optional Enhancement**: Add a comment explaining why the fallback exists:
```go
// Try field selector first (efficient client-side cache lookup)
// Fallback to label query for API servers without field index support (production robustness)
```

### 2. Test Code: Remove Fallback, Fix Setup ✅

**If Gateway integration tests have the same fallback pattern**, consider:

1. **Check test setup order**: Ensure client is retrieved AFTER `SetupWithManager()`
2. **Remove test fallback**: With correct setup, field indexes work reliably in envtest
3. **Add smoke test**: Verify field indexes work before running full suite

### 3. Document the Pattern ✅

Add to Gateway team docs:
- Why production fallback exists (defensive programming)
- Why test fallback is unnecessary (with proper setup)
- How to set up field indexes correctly in envtest

---

## Questions for Gateway Team

1. **Does Gateway have integration tests** that query by `spec.signalFingerprint`?
   - If yes, do they have a fallback pattern in tests?
   - If yes, is the test client retrieved before or after `SetupWithManager()`?

2. **What other custom field indexes** does Gateway use?
   - Are they working correctly in envtest?
   - Could they benefit from this setup pattern?

3. **Are there other services** using similar patterns that could benefit from this guide?

---

## Appendix: Debugging Field Index Issues

### Symptoms of Incorrect Setup

1. **Error**: `"field label not supported: spec.signalFingerprint"`
   - **Cause**: Client retrieved before field index registration
   - **Fix**: Reorder initialization (index first, then get client)

2. **Error**: Tests pass locally but fail in CI
   - **Cause**: Race condition in manager startup
   - **Fix**: Ensure manager is started and ready before running tests

3. **Error**: Query returns 0 results but direct list finds objects
   - **Cause**: Field index not registered or not working
   - **Fix**: Add smoke test to verify field index setup

### Debug Commands

```go
// In test BeforeSuite, after setup:
GinkgoWriter.Printf("✅ Manager created: %v\n", k8sManager != nil)
GinkgoWriter.Printf("✅ Reconciler setup complete\n")
GinkgoWriter.Printf("✅ Client retrieved: %v\n", k8sClient != nil)
GinkgoWriter.Printf("✅ Manager started\n")
```

---

**Created**: Dec 23, 2025
**For**: Gateway Team
**From**: RO Team
**Purpose**: Knowledge sharing on field index setup in envtest
**Status**: ✅ Complete
**Priority**: Medium (enhances test reliability)

**Related Documents**:
- `docs/handoff/RO_FIELD_INDEX_FIX_SUMMARY_DEC_23_2025.md`
- `docs/handoff/GW_PRODUCTION_FALLBACK_CODE_SMELL_DEC_23_2025.md` (superseded by this document)

