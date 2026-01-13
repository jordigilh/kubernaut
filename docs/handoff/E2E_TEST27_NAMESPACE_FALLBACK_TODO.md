# Test 27: Namespace Fallback Feature - Implementation Required

**Date**: January 13, 2026
**Status**: 🚧 Feature Not Implemented
**Priority**: P2 - Medium (1 E2E test failure)

---

## 📋 Business Requirement

**Test**: `test/e2e/gateway/27_error_handling_test.go:224`
**Expectation**: Gateway should fallback to `kubernaut-system` namespace when target namespace doesn't exist

### **Business Scenario**:
- Alert references non-existent namespace
- Expected: CRD created in `kubernaut-system` namespace (graceful fallback)
- Why: Invalid namespace shouldn't block remediation

**Examples**:
- Namespace deleted after alert fired
- Cluster-scoped signals (NodeNotReady)
- Configuration errors

---

## 🔍 Current Behavior

**Gateway returns**: `500 Internal Server Error`

**Gateway logs**:
```json
{"msg":"CRD creation failed with non-retryable error",
 "namespace":"does-not-exist-123",
 "error":"namespaces \"does-not-exist-123\" not found"}
```

**Test expectation**:
```go
// test/e2e/gateway/27_error_handling_test.go:258
Expect(resp.StatusCode).To(Equal(http.StatusCreated))

// Line 289
Expect(createdCRD.Namespace).To(Equal("kubernaut-system"))
Expect(createdCRD.Labels["kubernaut.ai/cluster-scoped"]).To(Equal("true"))
Expect(createdCRD.Labels["kubernaut.ai/origin-namespace"]).To(Equal(nonExistentNamespace))
```

---

## 💡 Proposed Implementation

### **Location**: `pkg/gateway/processing/crd_creator.go`

**Option A**: Fallback on namespace not found error

```go
func (c *CRDCreator) CreateRemediationRequest(ctx context.Context, signal *types.NormalizedSignal) (*remediationv1alpha1.RemediationRequest, error) {
    namespace := signal.Resource.Namespace
    if namespace == "" {
        namespace = "kubernaut-system" // Default fallback
    }

    // Try to create in specified namespace
    rr, err := c.createCRD(ctx, namespace, signal)
    if err != nil && isNamespaceNotFound(err) {
        c.logger.Info("Namespace not found, falling back to kubernaut-system",
            "original_namespace", namespace,
            "fallback_namespace", "kubernaut-system")

        // Add labels for tracking
        signal.Labels["kubernaut.ai/cluster-scoped"] = "true"
        signal.Labels["kubernaut.ai/origin-namespace"] = namespace

        // Retry in kubernaut-system
        rr, err = c.createCRD(ctx, "kubernaut-system", signal)
    }
    return rr, err
}

func isNamespaceNotFound(err error) bool {
    if err == nil {
        return false
    }
    return strings.Contains(err.Error(), "namespaces") &&
           strings.Contains(err.Error(), "not found")
}
```

**Option B**: Pre-validate namespace existence

```go
func (c *CRDCreator) CreateRemediationRequest(ctx context.Context, signal *types.NormalizedSignal) (*remediationv1alpha1.RemediationRequest, error) {
    namespace := signal.Resource.Namespace
    if namespace == "" {
        namespace = "kubernaut-system"
    }

    // Check if namespace exists
    ns := &corev1.Namespace{}
    err := c.client.Get(ctx, client.ObjectKey{Name: namespace}, ns)
    if err != nil && apierrors.IsNotFound(err) {
        // Namespace doesn't exist, use fallback
        c.logger.Warn("Namespace not found, using kubernaut-system fallback",
            "requested_namespace", namespace)

        signal.Labels["kubernaut.ai/cluster-scoped"] = "true"
        signal.Labels["kubernaut.ai/origin-namespace"] = namespace
        namespace = "kubernaut-system"
    }

    return c.createCRD(ctx, namespace, signal)
}
```

**Recommended**: **Option A** - Fallback on error (no extra API call)

---

## ✅ Acceptance Criteria

1. **HTTP Response**: Returns `201 Created` for invalid namespace (not 500)
2. **CRD Location**: Created in `kubernaut-system` namespace
3. **Labels**:
   - `kubernaut.ai/cluster-scoped: "true"`
   - `kubernaut.ai/origin-namespace: "<original_namespace>"`
4. **Logging**: Warning logged with original namespace and fallback
5. **Test**: `test/e2e/gateway/27_error_handling_test.go` passes

---

## 🧪 Testing Strategy

### **Unit Tests** (new file: `crd_creator_namespace_fallback_test.go`):
```go
It("falls back to kubernaut-system for non-existent namespace", func() {
    // Mock namespace not found error
    // Verify fallback logic
    // Verify labels set correctly
})

It("uses specified namespace when it exists", func() {
    // Mock namespace exists
    // Verify no fallback
    // Verify no fallback labels
})
```

### **Integration Tests**:
- Not needed (E2E Test 27 covers this)

### **E2E Tests**:
- ✅ Already exists: `test/e2e/gateway/27_error_handling_test.go:224`

---

## 📝 Related Business Requirements

**Potential BRs** (need to be documented):
- **BR-GATEWAY-XXX**: Graceful namespace fallback for invalid namespaces
- **BR-GATEWAY-XXX**: Cluster-scoped signal handling (NodeNotReady, etc.)
- **BR-GATEWAY-XXX**: Namespace label preservation for audit

---

## 🚀 Implementation Checklist

- [ ] Add namespace fallback logic to `crd_creator.go`
- [ ] Add `isNamespaceNotFound()` helper function
- [ ] Add labels: `cluster-scoped`, `origin-namespace`
- [ ] Add warning logging for fallback
- [ ] Write unit tests for fallback logic
- [ ] Run E2E Test 27 to validate
- [ ] Document BR in `docs/requirements/`
- [ ] Update `docs/architecture/` if needed

---

## 📊 Impact Assessment

**Effort**: Medium (2-3 hours)
- Code changes: ~50 lines
- Unit tests: ~100 lines
- Documentation: ~1 hour

**Risk**: Low
- Isolated to CRD creator
- Fallback is graceful degradation
- No breaking changes to existing behavior

**Value**: Medium
- Improves resilience for edge cases
- Unblocks 1 E2E test
- Better user experience (no 500 errors)

---

## 🔗 References

- **Test File**: `test/e2e/gateway/27_error_handling_test.go:224-299`
- **Gateway Logs**: `/tmp/gateway-e2e-logs-20260113-142947/`
- **RCA Document**: `docs/handoff/E2E_FAILURES_RCA_JAN13_2026.md` (Category 5)

---

**Next Action**: Implement namespace fallback logic after validating other E2E fixes
**Estimated Time**: 2-3 hours (including tests)
**Priority**: P2 - Can be deferred if other critical issues exist
