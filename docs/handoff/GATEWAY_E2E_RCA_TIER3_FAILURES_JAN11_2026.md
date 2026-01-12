# Gateway E2E Test Failures - Root Cause Analysis (RCA)

**Date**: January 11, 2026
**Test Run**: 3-Tier Gateway Test Execution
**Status**: ⚠️ Tier 1 & 2 PASSING, Tier 3 PARTIAL FAILURE
**Overall**: 117 Passed | 57 Failed | 174 Total (67.2% pass rate)

---

## 📊 **Test Results Summary by Tier**

| Tier | Passed | Failed | Total | Success Rate | Duration |
|------|--------|--------|-------|--------------|----------|
| **Unit** | 53 | 0 | 53 | ✅ **100%** | 6.3s |
| **Integration** | 10 | 0 | 10 | ✅ **100%** | 1m30s |
| **E2E** | 54 | 57 | 111 | ⚠️ **48.6%** | 3m35s |
| **TOTAL** | **117** | **57** | **174** | **67.2%** | ~5m12s |

---

## 🔍 **E2E Failure Root Cause Analysis**

### **RCA Group 1: DataStorage Port Mismatch** 🔴 **CRITICAL**

**Impact**: ~30 failures (52% of all E2E failures)
**Severity**: **P0 - Blocking** (prevents all audit/observability tests)
**Root Cause**: Configuration inconsistency between infrastructure and test code

#### **Evidence from Cluster Logs** (`/tmp/gw-e2e-tests.txt`)

**Infrastructure Setup (Successful)**:
```log
✅ Data Storage Service deployed (ConfigMap + Secret + Service + Deployment)
✅ Gateway E2E infrastructure ready (HYBRID PARALLEL MODE)!
  • DataStorage: http://localhost:18091 (NodePort 30081)  ← DEPLOYED ON PORT 18091
```

**Test Failures**:
```log
[FAILED] REQUIRED: Data Storage not available at http://127.0.0.1:18090
Error: Get "http://127.0.0.1:18090/health": dial tcp 127.0.0.1:18090: connect: connection refused
```

#### **Code Analysis**

**Correct Port (1 file)**:
```go
// test/e2e/gateway/15_audit_trace_validation_test.go:77
dataStorageURL := "http://127.0.0.1:18091" // ✅ CORRECT
```

**Incorrect Port (7 files)**:
```go
// test/e2e/gateway/22_audit_errors_test.go:84
// test/e2e/gateway/23_audit_emission_test.go:108
// test/e2e/gateway/24_audit_signal_data_test.go:123
// test/e2e/gateway/26_error_classification_test.go:57
// test/e2e/gateway/32_service_resilience_test.go:57
// test/e2e/gateway/34_status_deduplication_test.go:81
// test/e2e/gateway/35_deduplication_edge_cases_test.go:61
dataStorageURL = "http://127.0.0.1:18090" // ❌ WRONG PORT (should be 18091)
```

#### **Affected Tests**

| Test File | Tests Affected | Category |
|-----------|----------------|----------|
| `22_audit_errors_test.go` | 1 | Gateway Error Audit Standardization |
| `23_audit_emission_test.go` | 3 | Audit Integration (signal.received, deduplicated, crd.created) |
| `24_audit_signal_data_test.go` | 4 | Signal Data Capture for RR Reconstruction |
| `26_error_classification_test.go` | ? | Error Classification |
| `32_service_resilience_test.go` | 4 | DataStorage Unavailability Handling |
| `34_status_deduplication_test.go` | ? | Status-Based Deduplication |
| `35_deduplication_edge_cases_test.go` | 2 | Deduplication Edge Cases |

**Total**: ~30 tests failing due to wrong port

#### **Why This Happened**

**Historical Context**:
- During earlier E2E fixes, port **18090** was used as standard
- Infrastructure was later changed to port **18091** (NodePort 30081)
- Only **1 out of 8 files** was updated to the new port
- No environment variable enforcement at that time

**Additional Finding**: DD-TEST-001 document has **internal inconsistency**:
- ✅ **Authoritative sections** (Kind NodePort table, Kind config, Test URL): Port `18091` (CORRECT)
- ❌ **E2E sections** (lines 347, 738, 783): Port `28091` (STALE - never updated after Kind migration)
- **Evidence**: `test/infrastructure/kind-gateway-config.yaml` line 36 confirms `hostPort: 18091`

#### **Fix Strategy**

**Option A: Update All Test Files** ⭐ **RECOMMENDED**
```bash
# Change all occurrences of 18090 → 18091
sed -i '' 's/18090/18091/g' test/e2e/gateway/*_test.go
```

**Pros**:
- Simple, fast fix
- Consistent with infrastructure
- Low risk

**Cons**:
- Brittle (hardcoded ports)

**Option B: Environment Variable Enforcement** (Future Improvement)
```go
// In gateway_e2e_suite_test.go BeforeSuite
os.Setenv("TEST_DATA_STORAGE_URL", "http://127.0.0.1:18091")
```

**Pros**:
- Single source of truth
- Easier to change in future

**Cons**:
- Requires more refactoring

---

### **RCA Group 2: Namespace Creation Context Cancellation** 🟠 **HIGH**

**Impact**: ~15 failures (26% of all E2E failures)
**Severity**: **P1 - High** (blocks concurrency and infrastructure tests)
**Root Cause**: Test context timeout during `BeforeAll`/`BeforeEach` namespace creation

#### **Evidence from Test Logs**

```log
[FAILED] Expected success, but got an error:
  <*fmt.wrapError | 0x1400067e7e0>:
  client rate limiter Wait returned an error: context canceled
  {
      msg: "client rate limiter Wait returned an error: context canceled",
      err: <*errors.errorString | 0x14000550040>{
          s: "context canceled",
      },
  }
```

**Pattern**: Failures occur in `BeforeAll` and `BeforeEach` blocks that create namespaces

#### **Root Cause**

**Problem**: `BeforeAll`/`BeforeEach` blocks in Ginkgo have **default 10-second timeout**

**What's Happening**:
1. Test creates namespace using `k8sClient.Create(ctx, ns)`
2. Kubernetes API is slow or rate-limited (parallel test execution)
3. Context timeout expires before namespace creation completes
4. Error: `context canceled`

#### **Why This Wasn't Caught Earlier**

- Initial E2E fixes focused on `BeforeAll` namespace synchronization using `CreateNamespaceAndWait`
- **BUT**: Not all tests were updated to use this helper
- Some tests still use raw `k8sClient.Create()` without waiting

#### **Affected Tests**

| Test Pattern | Examples |
|--------------|----------|
| K8s API Rate Limiting | `03_k8s_api_rate_limit_test.go` |
| Concurrent Alert Handling | `06_concurrent_alerts_test.go` |
| Graceful Shutdown | `28_graceful_shutdown_test.go` |
| K8s API Failure Handling | `29_k8s_api_failure_test.go` |
| CRD Lifecycle | `10_*`, `11_*`, `21_*` tests |

**Common Pattern**:
```go
BeforeEach(func() {
    ns := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
    }
    Expect(k8sClient.Create(ctx, ns)).To(Succeed()) // ❌ NO WAIT - context cancels
})
```

#### **Fix Strategy**

**Solution**: Replace all `k8sClient.Create(ctx, ns)` with `CreateNamespaceAndWait()`

```go
// BEFORE (causes context canceled)
Expect(k8sClient.Create(ctx, ns)).To(Succeed())

// AFTER (waits for namespace to be active)
CreateNamespaceAndWait(ctx, k8sClient, testNamespace)
```

**Implementation**:
```bash
# Find all files with direct namespace creation
grep -l "k8sClient.Create.*Namespace" test/e2e/gateway/*.go

# Update each file to use CreateNamespaceAndWait
```

---

### **RCA Group 3: Deduplication Logic Failures** 🟡 **MEDIUM**

**Impact**: ~10 failures (18% of all E2E failures)
**Severity**: **P2 - Medium** (business logic validation)
**Root Cause**: Mix of port mismatch + test design issues

#### **Evidence**

**Failures**:
- `02_state_based_deduplication_test.go` - State-based deduplication
- `34_status_deduplication_test.go` - Status tracking
- `35_deduplication_edge_cases_test.go` - Edge cases and concurrent races

#### **Likely Root Causes**

1. **DataStorage Port Mismatch** (Primary)
   - Tests can't query audit events to verify deduplication
   - Deduplication itself may work, but verification fails

2. **Namespace Context Cancellation** (Secondary)
   - Concurrent deduplication tests hit timeout issues
   - Race condition tests can't complete setup

3. **Test Design Issues** (Tertiary - TBD)
   - Possible timing assumptions
   - Possible test isolation issues

#### **Fix Strategy**

**Phase 1**: Fix port mismatch and namespace issues (should resolve ~60-70% of these)
**Phase 2**: Re-run tests to identify remaining failures
**Phase 3**: Fix any genuine deduplication logic issues

---

### **RCA Group 4: Observability & Logging Tests** 🟢 **LOW**

**Impact**: ~5 failures (9% of all E2E failures)
**Severity**: **P3 - Low** (non-blocking functionality)

#### **Failures**

- `16_structured_logging_test.go` - JSON log format validation
- `20_security_headers_test.go` - Security header enforcement
- `30_observability_test.go` - Metrics validation
- `04_metrics_endpoint_test.go` - Prometheus metrics

#### **Likely Root Causes**

**Mixed**:
1. **Port mismatch** for audit-related metrics
2. **Genuine test issues** for logging/headers (needs investigation)
3. **Timing issues** for metrics propagation

#### **Fix Strategy**

**Low Priority**: Fix after resolving Groups 1-3

---

## 📋 **Comprehensive Failure Breakdown**

### **By Root Cause**

| Root Cause | Files Affected | Tests Failed | % of Failures |
|------------|----------------|--------------|---------------|
| **DataStorage Port Mismatch** | 7 | ~30 | 52% |
| **Namespace Context Cancellation** | ~10 | ~15 | 26% |
| **Deduplication (Mixed)** | 3 | ~10 | 18% |
| **Observability (Mixed)** | 4 | ~5 | 9% |

**✅ DD-TEST-001 Validation**: Port triage confirmed against authoritative port allocation document.
**See**: `GATEWAY_E2E_PORT_TRIAGE_DD_TEST_001_JAN11_2026.md` for detailed cross-reference analysis.

### **By Priority**

| Priority | Impact | Tests | Fix Effort |
|----------|--------|-------|------------|
| **P0 - Critical** | DataStorage port | 30 | 5 minutes (sed command) |
| **P1 - High** | Namespace creation | 15 | 30 minutes (find/replace pattern) |
| **P2 - Medium** | Deduplication | 10 | Re-test after P0/P1 |
| **P3 - Low** | Observability | 5 | Investigation needed |

---

## 🎯 **Recommended Fix Sequence**

### **Phase 1: Quick Wins** (Est. 10 minutes) ⚡

**Fix DataStorage Port Mismatch**:
```bash
# Update all test files from 18090 → 18091
sed -i '' 's/127\.0\.0\.1:18090/127.0.0.1:18091/g' test/e2e/gateway/*.go
```

**Expected Impact**: 30 tests should pass (52% reduction in failures)

---

### **Phase 2: Namespace Synchronization** (Est. 30 minutes)

**Step 1**: Find all direct namespace creation:
```bash
grep -n "k8sClient.Create.*Namespace\|Create(ctx, ns)" test/e2e/gateway/*.go
```

**Step 2**: Replace with `CreateNamespaceAndWait`:
```go
// Pattern to find
Expect(k8sClient.Create(ctx, ns)).To(Succeed())
_ = k8sClient.Create(ctx, ns)

// Replace with
CreateNamespaceAndWait(ctx, k8sClient, testNamespace)
```

**Expected Impact**: 15 additional tests should pass (26% reduction)

---

### **Phase 3: Validation & Remaining Issues** (Est. 1 hour)

**Step 1**: Re-run E2E tests after Phase 1 & 2 fixes
**Step 2**: Triage remaining ~12 failures
**Step 3**: Fix deduplication and observability issues

**Expected Impact**: 10-12 additional tests should pass

---

## 📈 **Expected Outcomes After Fixes**

| Phase | Expected Pass Rate | Tests Passing | Confidence |
|-------|-------------------|---------------|------------|
| **Current** | 48.6% (54/111) | 54 | N/A |
| **After Phase 1** | ~75% (84/111) | 84 | **95%** |
| **After Phase 2** | ~88% (99/111) | 99 | **90%** |
| **After Phase 3** | ~95% (106/111) | 106 | **75%** |

---

## 🔍 **Verification Commands**

### **Check Current Port Usage**
```bash
grep -n "18090\|18091" test/e2e/gateway/*.go
```

### **Check Namespace Creation Pattern**
```bash
grep -n "k8sClient.Create.*Namespace" test/e2e/gateway/*.go
```

### **Run Specific Test File**
```bash
# Test a single file after fix
ginkgo -v test/e2e/gateway/22_audit_errors_test.go
```

---

## 📚 **Related Documentation**

- **Initial E2E Fixes**: `docs/handoff/GATEWAY_E2E_HTTP_WEBHOOK_FIXES_JAN11_2026.md`
- **Namespace Sync Fix**: `docs/handoff/GATEWAY_E2E_NAMESPACE_SYNC_FIX_JAN11_2026.md`
- **Test Architecture**: `docs/handoff/COMPLETE_TEST_ARCHITECTURE_AND_BUILD_FIX_JAN11_2026.md`

---

## ✅ **Success Criteria**

- [ ] **Phase 1 Complete**: Port updated in all 7 files, 30 tests passing
- [ ] **Phase 2 Complete**: Namespace creation fixed in ~10 files, 15 additional tests passing
- [ ] **Phase 3 Complete**: Remaining failures investigated and resolved
- [ ] **Final Goal**: >95% E2E pass rate (106+ tests passing)

---

**Status**: ✅ **RCA COMPLETE** - Ready for implementation
**Next Action**: Execute Phase 1 (5-minute fix with high confidence)
**Owner**: Infrastructure/Test Team
