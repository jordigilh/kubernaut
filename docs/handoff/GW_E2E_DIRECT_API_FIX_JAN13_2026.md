# Gateway E2E Direct API Query Fix

**Date**: 2026-01-13
**Authority**: DD-E2E-DIRECT-API-001
**Impact**: 15 failing E2E tests → Expected 100% pass rate
**Status**: Implementation in progress

---

## 🎯 **Root Cause Analysis**

### **Problem**: Gateway E2E tests were using `List()` queries that depend on K8s cache/indexing
```go
// SLOW & UNRELIABLE (120s timeout, field index dependency)
Eventually(func() int {
    var rrList remediationv1alpha1.RemediationRequestList
    err := k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
    return len(rrList.Items)
}, 120*time.Second, 1*time.Second).Should(Equal(1))
```

**Issues**:
- ❌ Depends on K8s API server field index sync (60-120s lag)
- ❌ Slow (120s timeout required)
- ❌ Not how RO/SignalProcessing/AIAnalysis E2E tests work
- ❌ Tests never validated the actual production deduplication logic

---

## ✅ **Solution: Query by Exact CRD Name (RO Pattern)**

### **Discovery**: Gateway returns CRD name in response
```go
type ProcessingResponse struct {
    RemediationRequestName      string `json:"remediationRequestName,omitempty"`
    RemediationRequestNamespace string `json:"remediationRequestNamespace,omitempty"`
    // ...
}
```

### **New Pattern**: Direct API Get (like RO does)
```go
// FAST & RELIABLE (30s timeout, direct API Get, no cache/index dependency)
resp1 := SendWebhook(gatewayURL, payload)
Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

// Parse Gateway response to get CRD name
var gwResp GatewayResponse
Expect(json.Unmarshal(resp1.Body, &gwResp)).To(Succeed())
Expect(gwResp.RemediationRequestName).NotTo(BeEmpty())

// Query CRD by exact name (RO E2E pattern)
var createdRR remediationv1alpha1.RemediationRequest
Eventually(func() error {
    return k8sClient.Get(ctx, client.ObjectKey{
        Namespace: testNamespace,
        Name:      gwResp.RemediationRequestName,
    }, &createdRR)
}, 30*time.Second, 1*time.Second).Should(Succeed())
```

**Benefits**:
- ✅ Direct API Get bypasses cache/indexing issues
- ✅ 4x faster (30s vs 120s timeout)
- ✅ Matches RO/SignalProcessing E2E patterns
- ✅ More reliable (no field index dependency)
- ✅ Tests actual Gateway behavior (returns CRD name to clients)

---

## 📋 **Implementation Checklist**

### **Phase 1: Tests Fixed**
- [x] `test/e2e/gateway/30_observability_test.go` - Deduplication metrics test
- [ ] `test/e2e/gateway/31_prometheus_adapter_test.go` - Prometheus adapter test (if needed)
- [ ] `test/e2e/gateway/36_deduplication_state_test.go` - State-based deduplication (if needed)
- [ ] `test/e2e/gateway/24_audit_signal_data_test.go` - Audit signal data (if needed)
- [ ] `test/e2e/gateway/32_service_resilience_test.go` - Service resilience (if needed)
- [ ] Other tests using `Eventually(List())` pattern

### **Phase 2: Validation**
- [ ] Run full E2E suite: `make test-tier-e2e SERVICE=gateway`
- [ ] Verify 100% pass rate (0 failures)
- [ ] Confirm test execution time improved (15-20 tests × 90s savings = ~25 min faster)
- [ ] Check Kind must-gather logs for any remaining issues

---

## 🔍 **Comparison with Other Services**

### **RemediationOrchestrator (RO) E2E Pattern**
```go
// RO tests query by known name
Eventually(func() error {
    return k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)
}, timeout, interval).Should(Succeed())
```
**✅ Gateway now uses the same pattern**

### **Why RO Works and Gateway Failed**
- **RO E2E**: Creates CRD with known name, queries by that name
- **Gateway E2E (old)**: Sent webhook, waited for List() to find it
- **Gateway E2E (new)**: Sends webhook, parses response for CRD name, queries by that name

---

## 📚 **Design Decision Reference**

### **DD-E2E-DIRECT-API-001: Direct API Queries Over List**

**Decision**: E2E tests should query K8s resources by exact name using `Get()` instead of `List()` where possible.

**Rationale**:
1. Direct Get() bypasses K8s client cache and field index dependencies
2. Faster (30s timeout vs 120s timeout)
3. More reliable (no eventual consistency issues)
4. Matches successful patterns in RO/SignalProcessing/AIAnalysis

**When to use List()**:
- Integration tests validating business logic that uses List()
- Unit tests for components that implement List() queries
- NOT for E2E tests validating CRD creation

**When to use Get()**:
- E2E tests validating resource creation/updates
- Tests where the resource name is known or returned by the service
- Queries requiring immediate consistency

---

## 🚀 **Expected Impact**

### **Pass Rate**
- **Before**: 84.4% (15 failures out of ~45 tests)
- **After**: 100% (0 failures)

### **Test Execution Time**
- **Before**: ~120s per test for CRD visibility
- **After**: ~30s per test for CRD visibility
- **Savings**: 90s × 15 tests = **~25 minutes faster**

### **Reliability**
- **Before**: Flaky due to field index sync timing
- **After**: Deterministic (direct API Get)

---

## 🔗 **References**

- **Related**: `docs/handoff/GW_E2E_FINAL_STATUS_JAN13_2026.md` (identified problem)
- **Related**: `pkg/gateway/server.go:1045` (ProcessingResponse struct)
- **Related**: `test/e2e/remediationorchestrator/lifecycle_e2e_test.go` (RO pattern)
- **Authority**: DD-E2E-DIRECT-API-001 (this design decision)

---

## ✅ **Validation Criteria**

Test fix is successful when:
1. ✅ Test 30 passes with 30s timeout (not 120s)
2. ✅ All other E2E tests pass
3. ✅ No "Timed out" failures in Gateway E2E suite
4. ✅ E2E suite completes in < 30 minutes (previously ~45 minutes)
5. ✅ Must-gather logs show no "Failed to initialize deduplication status" errors
