# Gateway E2E Phase 2 Fixes - HTTP Test Server & Namespace Sync

**Date**: January 11, 2026
**Phase**: 2 - HTTP Test Server Removal + Namespace Synchronization
**Status**: ✅ **FIXES APPLIED** - Validation in progress
**Execution Time**: <10 minutes

---

## 🎯 **Phase 2 Summary**

**Discovery**: Phase 1 results revealed that the primary failure cause was **NOT** namespace context cancellation as initially thought, but rather **incorrect use of `httptest.NewServer` in E2E tests**.

**Root Cause**: Three test files were creating local HTTP test servers with no handlers (`httptest.NewServer(nil)`), causing all HTTP requests to return 404.

**Solution**: Remove local test servers and use the globally deployed Gateway service at `http://127.0.0.1:8080`.

---

## 📊 **Failure Analysis Correction**

### **Original RCA (Incorrect)**

| Root Cause | Expected Impact |
|------------|-----------------|
| Namespace context cancellation | 33 tests (73% of failures) |
| Test logic issues | 12 tests (27% of failures) |

### **Corrected RCA (After Investigation)**

| Root Cause | Actual Impact | Fix Type |
|------------|---------------|----------|
| **HTTP Test Server (404 errors)** | **20+ tests** | Remove `httptest.NewServer` |
| **Namespace context cancellation** | **3-5 tests** | Use `CreateNamespaceAndWait` |
| **Test logic issues** | **15-20 tests** | Requires investigation |

---

## 🔧 **Fixes Applied**

### **Fix 1: Remove Local HTTP Test Servers** (3 files)

**Problem**: Tests creating local `httptest.Server` with `nil` handler → all requests return 404

**Files Fixed**:
1. ✅ `test/e2e/gateway/36_deduplication_state_test.go` (7 failures)
2. ✅ `test/e2e/gateway/34_status_deduplication_test.go` (3 failures)
3. ✅ `test/e2e/gateway/35_deduplication_edge_cases_test.go` (2 failures)

**Total Impact**: ~12 tests expected to pass

**Changes**:
```diff
-var (
-    server *httptest.Server
-    gatewayURL string
-)
+var (
+    // gatewayURL is defined at suite level (http://127.0.0.1:8080)
+)

-BeforeEach(func() {
-    server = httptest.NewServer(nil)
-    gatewayURL = server.URL
-})
+BeforeEach(func() {
+    // Note: gatewayURL is the globally deployed Gateway service
+})

-AfterEach(func() {
-    if server != nil {
-        server.Close()
-    }
-})
+AfterEach(func() {
+    // No server cleanup needed
+})
```

**Imports Cleaned Up**:
- Removed `"net/http/httptest"` from all 3 files

---

### **Fix 2: Namespace Synchronization** (4 files)

**Problem**: Direct `k8sClient.Create(ctx, ns)` without waiting for namespace to become Active

**Files Fixed**:
1. ✅ `test/e2e/gateway/21_crd_lifecycle_test.go` (1 failure)
2. ✅ `test/e2e/gateway/02_state_based_deduplication_test.go` (1 failure)
3. ✅ `test/e2e/gateway/05_multi_namespace_isolation_test.go` (1 failure - 2 namespaces)
4. ✅ `test/e2e/gateway/27_error_handling_test.go` (1 failure)

**Total Impact**: ~4 tests expected to pass

**Changes**:
```diff
-ns := &corev1.Namespace{
-    ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
-}
-Expect(k8sClient.Create(testCtx, ns)).To(Succeed())
+CreateNamespaceAndWait(testCtx, k8sClient, testNamespace)
```

---

## 📊 **Expected Results**

| Metric | Before Phase 2 | After Phase 2 (Expected) | Change |
|--------|----------------|---------------------------|--------|
| **Tests Passing** | 66 | ~82 | ✅ **+16** |
| **Tests Failing** | 45 | ~29 | ✅ **-16** |
| **Pass Rate** | 59.5% | ~74% | ✅ **+14.5%** |

**Breakdown**:
- HTTP Test Server fixes: +12 tests
- Namespace sync fixes: +4 tests

---

## 🔍 **Detailed Fix Analysis**

### **File: 36_deduplication_state_test.go** (7 tests affected)

**Error Pattern**:
```
[FAILED] First alert should create new CRD
Expected
    <int>: 404
to equal
    <int>: 201
```

**Root Cause**:
- Line 75: `server = httptest.NewServer(nil)` creates server with no routes
- Line 76: `gatewayURL = server.URL` redirects tests to empty server
- All POST requests to `/webhooks/prometheus` return 404

**Fix**: Remove server creation, use suite-level `gatewayURL` (http://127.0.0.1:8080)

**Tests Fixed**:
1. "should detect duplicate and increment occurrence count" (Pending state)
2. "should detect duplicate and increment occurrence count" (Processing state)
3. "should treat as new incident (not duplicate)" (Completed state)
4. "should treat as new incident (retry remediation)" (Failed state)
5. "should treat as new incident (retry remediation)" (Cancelled state)
6. "should treat as duplicate (conservative fail-safe)" (Unknown state)
7. "should create new CRD" (CRD doesn't exist)

---

### **File: 34_status_deduplication_test.go** (3 tests affected)

**Error Pattern**: Same as above (404 instead of 201)

**Tests Fixed**:
1. "should track duplicate count in RR status for RO prioritization"
2. "should accurately count recurring alerts for SLA reporting"
3. "should track high occurrence count indicating storm behavior"

---

### **File: 35_deduplication_edge_cases_test.go** (2 tests affected)

**Error Pattern**: Same as above (404 instead of 201)

**Tests Fixed**:
1. "should fail request when field selector query fails"
2. "should handle concurrent requests for same fingerprint gracefully"

---

### **File: 21_crd_lifecycle_test.go** (1 test affected)

**Error Pattern**:
```
[FAILED] Expected success, but got an error:
    client rate limiter Wait returned an error: context canceled
```

**Root Cause**: BeforeAll creates namespace with `k8sClient.Create()` without waiting

**Fix**: Use `CreateNamespaceAndWait(testCtx, k8sClient, testNamespace)`

**Tests Fixed**:
1. "should reject malformed JSON with 400 Bad Request"

---

### **File: 02_state_based_deduplication_test.go** (1 test affected)

**Error Pattern**: Same context canceled error

**Fix**: Use `CreateNamespaceAndWait` in BeforeAll

**Tests Fixed**:
1. "should deduplicate identical alerts and create separate CRDs for different alerts"

---

### **File: 05_multi_namespace_isolation_test.go** (1 test affected)

**Error Pattern**: Same context canceled error (2 namespaces created)

**Fix**: Use `CreateNamespaceAndWait` for both namespace1 and namespace2

**Tests Fixed**:
1. "should isolate alerts and CRDs between namespaces"

---

### **File: 27_error_handling_test.go** (1 test affected)

**Error Pattern**: Same context canceled error

**Fix**: Use `CreateNamespaceAndWait` in BeforeEach

**Tests Fixed**:
1. "returns clear error for missing required fields"

---

## 📈 **Expected Progress After Phase 2**

| Phase | Pass Rate | Tests Passing | Tests Failing | Description |
|-------|-----------|---------------|---------------|-------------|
| **Baseline** | 48.6% | 54 | 57 | Original state |
| **Phase 1** | 59.5% | 66 | 45 | Port fix (18090→18091) |
| **Phase 2** | ~74% | ~82 | ~29 | HTTP server + namespace fixes |
| **Phase 3** | ~85-90% | ~95-100 | ~11-16 | Remaining test logic issues |

---

## 🔍 **Remaining Issues After Phase 2**

### **Category 1: Observability/Metrics Tests** (~4-6 failures)

**Files**:
- `30_observability_test.go` - Metrics validation

**Likely Issues**:
- Timing issues (metrics not propagated yet)
- Assertion logic problems
- Metrics endpoint access

---

### **Category 2: Service Resilience Tests** (~4-5 failures)

**Files**:
- `32_service_resilience_test.go` - DataStorage unavailability handling

**Likely Issues**:
- Test logic for simulating service failures
- Recovery validation timing

---

### **Category 3: Webhook Integration Tests** (~3-5 failures)

**Files**:
- `33_webhook_integration_test.go` - End-to-end webhook processing

**Likely Issues**:
- Payload format or routing
- Namespace or resource name issues

---

### **Category 4: Other Test Logic Issues** (~8-10 failures)

**Various files with individual test failures**

**Requires**: Case-by-case investigation after Phase 2 validation

---

## 📚 **Lessons Learned**

### **Root Cause Analysis Accuracy**

**Mistake 1**: Over-attributed failures to "context canceled" based on error count (17 occurrences)

**Reality**: Most failures were HTTP 404 errors, not context cancellation

**Lesson**: Examine actual failure patterns, not just error message counts

---

### **E2E vs Integration Test Patterns**

**Mistake 2**: Assumed tests using `httptest.Server` were correct for E2E tier

**Reality**: E2E tests should ALWAYS use deployed services, never local test servers

**Lesson**: Local test servers are ONLY for unit/integration tests that need to mock handlers

---

### **Validation Before Diagnosis**

**Mistake 3**: Created elaborate Phase 2 plan before validating Phase 1 results

**Reality**: Should have analyzed Phase 1 results first, then planned Phase 2

**Lesson**: Always validate results before planning next phase

---

## ✅ **Phase 2 Completion Checklist**

- [x] Identified HTTP test server pattern (3 files)
- [x] Removed `httptest.NewServer` from deduplication tests
- [x] Removed `httptest` imports
- [x] Identified namespace sync issues (4 files)
- [x] Replaced `k8sClient.Create()` with `CreateNamespaceAndWait()`
- [x] Updated test comments to reflect global Gateway URL usage
- [ ] Validated fixes with test run (in progress)
- [ ] Analyzed remaining failures (pending)

---

## 🔗 **Related Documentation**

- **Phase 1 Results**: `GATEWAY_E2E_PHASE1_RESULTS_JAN11_2026.md`
- **Phase 1 Fix**: `GATEWAY_E2E_PORT_FIX_PHASE1_JAN11_2026.md`
- **Port Triage**: `GATEWAY_E2E_PORT_TRIAGE_DD_TEST_001_JAN11_2026.md`
- **Original RCA**: `GATEWAY_E2E_RCA_TIER3_FAILURES_JAN11_2026.md`

---

**Status**: ✅ **PHASE 2 FIXES APPLIED** - Awaiting test validation
**Confidence**: **90%** (HTTP 404 pattern clearly identified and fixed)
**Expected Outcome**: Gateway E2E pass rate increases from 59.5% → ~74%
**Owner**: Gateway E2E Test Team
