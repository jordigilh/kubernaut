# Fake Client Migration Plan - crd_creator_retry_test.go

**Date**: 2025-11-23
**File**: `test/unit/gateway/processing/crd_creator_retry_test.go`
**Status**: 🚧 IN PROGRESS
**ADR**: [ADR-004: Fake Kubernetes Client for Unit Testing](../../../../docs/architecture/decisions/ADR-004-fake-kubernetes-client.md)

---

## 🎯 **Migration Objective**

Replace custom `mockK8sClient` (156 lines) with controller-runtime's fake client + interceptors.

**Benefits**:
- ✅ Maintained by controller-runtime (no breakage on `Apply()` method updates)
- ✅ Compile-time type safety
- ✅ Real K8s semantics with in-memory storage
- ✅ Error simulation via `interceptor.Funcs`

---

## 📋 **Migration Status**

### ✅ **Completed**
1. Updated imports to include `fake` and `interceptor` packages
2. Removed custom `mockK8sClient` implementation (156 lines)
3. Updated test variables in `BeforeEach`:
   - Added `fakeClient client.Client`
   - Added `scheme *runtime.Scheme`
   - Added `callCount *atomic.Int32` for thread-safe counting
   - Removed `mockClient *mockK8sClient`
4. Updated `AfterEach` to remove `mockClient.ResetCallCount()`

### 🚧 **In Progress**
5. Refactor individual test cases to use fake client with interceptors

---

## 🔧 **Refactoring Pattern**

### **Before** (Custom Mock):
```go
// Setup: First attempt fails with 429, second succeeds
attemptCount := 0
mockClient.createFunc = func(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
    attemptCount++
    if attemptCount == 1 {
        return apierrors.NewTooManyRequests("rate limited", 1)
    }
    return nil
}

k8sClient := k8s.NewClient(mockClient)
creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)
```

### **After** (Fake Client + Interceptor):
```go
// Setup: Fake client with interceptor (ADR-004)
callCount.Store(0)
fakeClient = fake.NewClientBuilder().
    WithScheme(scheme).
    WithInterceptorFuncs(interceptor.Funcs{
        Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
            count := callCount.Add(1)
            if count == 1 {
                return apierrors.NewTooManyRequests("rate limited", 1)
            }
            // Success on subsequent attempts - let fake client handle it
            return c.Create(ctx, obj, opts...)
        },
    }).
    Build()

k8sClient := k8s.NewClient(fakeClient)
creator = processing.NewCRDCreator(k8sClient, logger, metricsInst, "test-namespace", retryConfig)
```

**Key Changes**:
1. Use `callCount.Store(0)` and `callCount.Add(1)` for thread-safe counting
2. Create fake client with `interceptor.Funcs` for error simulation
3. Call `c.Create(ctx, obj, opts...)` for success cases (real K8s semantics)
4. Use `callCount.Load()` in assertions

---

## 📊 **Test Cases to Refactor**

### **File Structure** (962 lines total)
- Lines 1-53: Imports and ADR-004 comment (✅ Done)
- Lines 54-102: Test variables and setup (✅ Done)
- Lines 103-856: Test cases (🚧 In Progress)
- Lines 857-962: Additional test contexts

### **Test Contexts** (Estimated 30+ test cases)

| Line Range | Context | Test Count | Status |
|---|---|---|---|
| 107-243 | Retryable Errors - HTTP 429 | 3 tests | 🚧 Pending |
| 245-329 | Retryable Errors - HTTP 504 Timeout | 2 tests | 🚧 Pending |
| 331-502 | Non-Retryable Errors | 4 tests | 🚧 Pending |
| 504-620 | Exponential Backoff | 2 tests | 🚧 Pending |
| 622-720 | Retry Metrics | 2 tests | 🚧 Pending |
| 722-856 | Context Cancellation | 2 tests | 🚧 Pending |
| 858-962 | Additional Contexts | ~15 tests | 🚧 Pending |

**Total**: ~30 test cases to refactor

---

## ⚡ **Recommended Approach**

### **Option A: Systematic Refactoring** (Recommended)
1. Refactor test contexts one at a time
2. Run tests after each context to verify
3. Commit after each successful context migration
4. **Estimated Time**: 2-3 hours

### **Option B: Bulk Refactoring**
1. Use search/replace for common patterns
2. Manual fixes for edge cases
3. Run full suite at end
4. **Estimated Time**: 1-2 hours (higher risk)

### **Option C: Incremental with Fallback**
1. Keep both mock and fake client temporarily
2. Migrate tests one by one with feature flag
3. Remove mock after all tests pass
4. **Estimated Time**: 3-4 hours (safest)

---

## 🚀 **Next Steps**

1. **User Decision**: Choose refactoring approach (A/B/C)
2. **Execute Migration**: Refactor all test cases
3. **Verify Tests**: Run `go test ./test/unit/gateway/processing/crd_creator_retry_test.go -v`
4. **Clean Up**: Remove migration plan document
5. **Update Documentation**: Add ADR-004 reference to test file header

---

## 📝 **Migration Checklist**

- [x] Update imports
- [x] Remove custom mock implementation
- [x] Update test variables
- [x] Update BeforeEach setup
- [x] Update AfterEach cleanup
- [ ] Refactor HTTP 429 tests (3 tests)
- [ ] Refactor HTTP 504 tests (2 tests)
- [ ] Refactor Non-Retryable Error tests (4 tests)
- [ ] Refactor Exponential Backoff tests (2 tests)
- [ ] Refactor Retry Metrics tests (2 tests)
- [ ] Refactor Context Cancellation tests (2 tests)
- [ ] Refactor remaining tests (~15 tests)
- [ ] Run full test suite
- [ ] Verify all tests pass
- [ ] Clean up migration artifacts

---

**Status**: ✅ **READY FOR USER DECISION** - Choose refactoring approach
**Confidence**: **95%** - Pattern is clear, execution is straightforward

