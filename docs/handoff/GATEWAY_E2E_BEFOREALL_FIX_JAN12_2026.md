# Gateway E2E BeforeAll Namespace Creation Fix - Phase 6

**Date**: January 12, 2026  
**Status**: 🔧 IN PROGRESS  
**Issue**: 6 tests failing in BeforeAll due to namespace creation errors  
**Expected Impact**: +10-15 tests passing

---

## 🎯 **Problem Statement**

**Observation**: 6 tests failing in BeforeAll blocks with `context canceled` errors

**Root Cause**: Tests using old `k8sClient.Create()` pattern instead of `CreateNamespaceAndWait()` helper

**Pattern Identified**:
```go
// ❌ OLD PATTERN (FAILING)
ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

// ✅ NEW PATTERN (REQUIRED)
CreateNamespaceAndWait(testCtx, k8sClient, testNamespace)
```

---

## 📋 **Tests Requiring Fix**

| Test # | File | Description | BeforeAll Line |
|--------|------|-------------|----------------|
| 10 | `10_crd_creation_lifecycle_test.go` | CRD Creation Lifecycle | ~67 |
| 12 | `12_gateway_restart_recovery_test.go` | Gateway Restart Recovery | ~? |
| 13 | `13_redis_failure_graceful_degradation_test.go` | Redis Failure Degradation | ~? |
| 15 | `15_audit_trace_validation_test.go` | Audit Trace Validation | ~? |
| 16 | `16_structured_logging_test.go` | Structured Logging | ~? |
| 20 | `20_security_headers_test.go` | Security Headers | ~? |

---

## 🔧 **Fix Implementation**

### Strategy
Replace 2-3 lines of namespace creation code with single helper call:

```diff
- ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
- Expect(k8sClient.Create(testCtx, ns)).To(Succeed())
+ CreateNamespaceAndWait(testCtx, k8sClient, testNamespace)
```

---

## ✅ **Expected Results**

**Before**: 80/120 tests passing (66.7%)  
**After**: 90-95/120 tests passing (75-79%)  
**Improvement**: +10-15 tests

**Why This High Impact?**
- BeforeAll failures block entire test suites
- Each fixed BeforeAll enables multiple test cases
- Same proven pattern from Phase 5 (namespace fix)

---

## 📊 **Progress Tracking**

- [ ] Test 10: `10_crd_creation_lifecycle_test.go`
- [ ] Test 12: `12_gateway_restart_recovery_test.go`
- [ ] Test 13: `13_redis_failure_graceful_degradation_test.go`
- [ ] Test 15: `15_audit_trace_validation_test.go`
- [ ] Test 16: `16_structured_logging_test.go`
- [ ] Test 20: `20_security_headers_test.go`

---

**Status**: Ready for implementation  
**Next**: Apply fixes and run E2E tests
