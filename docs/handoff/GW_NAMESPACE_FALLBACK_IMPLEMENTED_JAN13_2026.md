# Gateway Namespace Fallback - Implementation Complete

**Date**: January 13, 2026
**Change Type**: Feature Implementation
**Business Requirement**: BR-GATEWAY-NAMESPACE-FALLBACK
**Test**: `test/e2e/gateway/27_error_handling_test.go:224`
**Status**: ✅ **COMPLETE** - Ready for Validation

---

## 🎯 **Business Requirement**

**Scenario**: Alert references non-existent namespace
**Expected Behavior**: CRD created in `kubernaut-system` namespace (graceful fallback)
**Business Value**: Invalid namespace doesn't block remediation

### **Use Cases**
1. Namespace deleted after alert fired
2. Cluster-scoped signals (e.g., NodeNotReady)
3. Configuration errors in alert definitions

---

## ✅ **Implementation Summary**

### **Changes Made**

**File**: `pkg/gateway/processing/crd_creator.go`

**1. Added Namespace Fallback Logic** (lines 175-213)
```go
// BR-GATEWAY-NAMESPACE-FALLBACK: Handle namespace not found by falling back to kubernaut-system
// Business Outcome: Invalid namespace doesn't block remediation
// Example scenarios: Namespace deleted after alert fired, cluster-scoped signals (NodeNotReady)
// Test: test/e2e/gateway/27_error_handling_test.go:224
if k8serrors.IsNotFound(err) && isNamespaceNotFoundError(err) {
    originalNamespace := rr.Namespace

    c.logger.Info("Namespace not found, falling back to kubernaut-system",
        "original_namespace", originalNamespace,
        "fallback_namespace", "kubernaut-system",
        "crd_name", rr.Name)

    // Update CRD to use kubernaut-system namespace
    rr.Namespace = "kubernaut-system"

    // Add labels to track the fallback
    if rr.Labels == nil {
        rr.Labels = make(map[string]string)
    }
    rr.Labels["kubernaut.ai/cluster-scoped"] = "true"
    rr.Labels["kubernaut.ai/origin-namespace"] = originalNamespace

    // Retry creation in kubernaut-system namespace
    err = c.k8sClient.CreateRemediationRequest(ctx, rr)
    if err == nil {
        c.logger.Info("CRD created successfully in kubernaut-system namespace after fallback",
            "original_namespace", originalNamespace,
            "crd_name", rr.Name)
        return nil
    }

    // If fallback also failed, log and continue to error handling below
    c.logger.Error(err, "CRD creation failed even after kubernaut-system fallback",
        "original_namespace", originalNamespace,
        "fallback_namespace", "kubernaut-system",
        "crd_name", rr.Name)
    // Fall through to normal error handling
}
```

**2. Added Helper Function** (end of file)
```go
// isNamespaceNotFoundError checks if an error is specifically about a namespace not being found
// (as opposed to a CRD not being found)
//
// Example error message: "namespaces \"does-not-exist-123\" not found"
//
// BR-GATEWAY-NAMESPACE-FALLBACK: Used to detect when to fallback to kubernaut-system namespace
// Test: test/e2e/gateway/27_error_handling_test.go:224
func isNamespaceNotFoundError(err error) bool {
    if err == nil {
        return false
    }
    errMsg := err.Error()
    // Check if error message contains "namespaces" and "not found"
    // This distinguishes namespace errors from RemediationRequest not found errors
    return strings.Contains(errMsg, "namespaces") && strings.Contains(errMsg, "not found")
}
```

---

## 🔍 **How It Works**

### **Before** (Namespace Not Found = Failure)
```
Signal arrives → Gateway tries to create CRD in "does-not-exist-123"
                           ↓
                  Namespace not found error
                           ↓
                  Return HTTP 500 Error ❌
                           ↓
                  Remediation blocked
```

### **After** (Namespace Not Found = Fallback)
```
Signal arrives → Gateway tries to create CRD in "does-not-exist-123"
                           ↓
                  Namespace not found error detected
                           ↓
                  Fallback to "kubernaut-system" namespace
                           ↓
                  Add labels:
                  - kubernaut.ai/cluster-scoped = "true"
                  - kubernaut.ai/origin-namespace = "does-not-exist-123"
                           ↓
                  Retry creation in kubernaut-system
                           ↓
                  Return HTTP 201 Created ✅
                           ↓
                  Remediation proceeds
```

---

## 📋 **Test Expectations**

### **Test 27: Namespace Fallback**

**Test File**: `test/e2e/gateway/27_error_handling_test.go:224`

**Test Steps**:
1. Send alert with non-existent namespace (`does-not-exist-123`)
2. Gateway detects namespace not found
3. Gateway falls back to `kubernaut-system`
4. CRD created in `kubernaut-system` with labels

**Expected Assertions**:
```go
// HTTP response
Expect(resp.StatusCode).To(Equal(http.StatusCreated))  // 201, not 500

// CRD location
Expect(createdCRD.Namespace).To(Equal("kubernaut-system"))

// Tracking labels
Expect(createdCRD.Labels["kubernaut.ai/cluster-scoped"]).To(Equal("true"))
Expect(createdCRD.Labels["kubernaut.ai/origin-namespace"]).To(Equal("does-not-exist-123"))
```

---

## ✅ **Validation Results**

### **Compilation** ✅
```bash
✅ go build ./pkg/gateway/processing/...
```

### **Lint Checks** ✅
```bash
✅ No linter errors in pkg/gateway/processing/crd_creator.go
```

### **Code Quality** ✅
- ✅ Clear comments explaining business requirement
- ✅ Comprehensive logging for fallback scenarios
- ✅ Helper function for namespace error detection
- ✅ Labels added for audit trail

---

## 📊 **Impact Assessment**

### **Behavior Changes**

| Scenario | Before | After |
|---|---|---|
| **Valid Namespace** | CRD created | CRD created (no change) |
| **Non-Existent Namespace** | HTTP 500 Error | HTTP 201 Created (fallback to kubernaut-system) |
| **CRD Not Found** | Retry logic | Retry logic (no change) |
| **RBAC Error** | Return error | Return error (no change) |

### **Edge Cases Handled**

1. **Namespace deleted after alert fired** → Fallback to kubernaut-system ✅
2. **Cluster-scoped signals (NodeNotReady)** → Fallback to kubernaut-system ✅
3. **Configuration errors** → Fallback to kubernaut-system ✅
4. **kubernaut-system also not found** → Return error (fail fast) ✅

### **No Breaking Changes**

- ✅ Existing behavior preserved for valid namespaces
- ✅ Only affects error handling for namespace not found
- ✅ Graceful degradation (fallback, not failure)

---

## 🎯 **Success Criteria**

- [x] Detect namespace not found errors correctly
- [x] Fallback to kubernaut-system namespace
- [x] Add `kubernaut.ai/cluster-scoped=true` label
- [x] Add `kubernaut.ai/origin-namespace` label
- [x] Return HTTP 201 Created (not 500 Error)
- [x] Comprehensive logging for fallback scenarios
- [x] Helper function for error detection
- [x] Code compiles successfully
- [x] No linter errors
- [ ] **Pending**: Test 27 validation

---

## 🔄 **Next Steps**

### **Immediate**
1. Run E2E tests to validate Test 27 passes
2. Confirm HTTP 201 response for non-existent namespace
3. Verify CRD created in kubernaut-system with correct labels

### **Follow-Up** (If Needed)
1. Add unit tests for namespace fallback logic
2. Document BR-GATEWAY-NAMESPACE-FALLBACK in `docs/requirements/`
3. Update architecture docs if needed

---

## 📚 **References**

### **Test**
- **Test File**: `test/e2e/gateway/27_error_handling_test.go:224-299`
- **Test Expectations**: Namespace fallback with labels

### **Documentation**
- **TODO Doc**: `docs/handoff/E2E_TEST27_NAMESPACE_FALLBACK_TODO.md`
- **Phase 3 Triage**: `docs/handoff/E2E_REMAINING_FAILURES_TRIAGE_JAN13_2026.md`

### **Code**
- **Implementation**: `pkg/gateway/processing/crd_creator.go:175-213`
- **Helper Function**: `pkg/gateway/processing/crd_creator.go` (end of file)

---

## ✅ **Summary**

**Status**: ✅ **IMPLEMENTATION COMPLETE** - Ready for E2E Validation

**Changes**:
- **File Modified**: 1 file (`crd_creator.go`)
- **Lines Added**: ~55 lines (fallback logic + helper function)
- **Features Implemented**: Namespace fallback to kubernaut-system
- **Labels Added**: `cluster-scoped`, `origin-namespace`

**Expected Outcome**:
- **Test 27**: ❌ Failing → ✅ Passing
- **HTTP Response**: 500 Error → 201 Created
- **Business Value**: Invalid namespace no longer blocks remediation

**Confidence**: 95%

**Justification**:
- ✅ Implementation matches Test 27 expectations exactly
- ✅ Comprehensive error detection (namespace vs CRD not found)
- ✅ Graceful degradation (fallback, not failure)
- ✅ Code compiles and passes linting
- ✅ Clear logging and audit trail

**Next Action**: Run E2E tests to validate all 98 tests pass (100% pass rate)

---

**Implementation Complete**: January 13, 2026
**Total Development Time**: ~30 minutes
**Files Modified**: 1 file
**Lines Added**: ~55 lines
**Test 27**: Ready for validation
