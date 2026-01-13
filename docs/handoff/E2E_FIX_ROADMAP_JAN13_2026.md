# Gateway E2E Fix Roadmap - Complete Work Tracker

**Date**: January 13, 2026
**Context**: Fixing 17/98 E2E failures to achieve 100% pass rate for merge
**Baseline**: 81 Passed | 17 Failed (82.7% pass rate)
**Goal**: 98 Passed | 0 Failed (100% pass rate)

---

## 🎯 Executive Summary

**Current Status**: TTL cleanup complete (Integration + Unit 100% passing), but E2E environment has 17 pre-existing failures.

**Critical Finding**: E2E failures are **NOT caused by TTL cleanup** (which only changed comments). These are pre-existing infrastructure/deployment issues.

**Estimated Total Time**: 12-24 hours across multiple sessions

**Approach**: Systematic investigation and fix, starting with highest-impact failures.

---

## 📊 Failure Categories & Priority

| Category | Count | Priority | Est. Time | Status |
|----------|-------|----------|-----------|--------|
| **Deduplication Logic** | 5 | 🔴 P0 | 2-4h | ⏸️ Not Started |
| **Audit Integration** | 5 | 🔴 P0 | 2-4h | ⏸️ Not Started |
| **BeforeAll Setup** | 2 | 🟡 P1 | 1-2h | ⏸️ Not Started |
| **Service Resilience** | 3 | 🟡 P1 | 1-2h | ⏸️ Not Started |
| **Error Handling** | 2 | 🟡 P1 | 1h | ⏸️ Not Started |

---

## 🔴 Phase 1: Deduplication Logic Fixes (HIGHEST IMPACT)

### **Status**: ⏸️ Not Started
### **Impact**: Blocks 5 tests
### **Estimated Time**: 2-4 hours

### **Root Cause Hypothesis**

Gateway returning **HTTP 201 (Created)** instead of **202 (Accepted)** for duplicate signals in E2E environment, but Integration tests pass 100%.

**Key Finding**: Gateway DOES configure field indexing at startup (lines 230-241 in `pkg/gateway/server.go`), so field indexing is NOT the issue.

### **Affected Tests**

1. ✗ **Test 30** (Line 173): `should track deduplicated signals via gateway_signals_deduplicated_total`
   - **File**: `test/e2e/gateway/30_observability_test.go`
   - **Error**: `Expected <int>: 201 to equal <int>: 202`
   - **Scenario**: Send same alert twice → second should return 202 (dedup), got 201 (created)

2. ✗ **Test 30** (metrics): `should track HTTP request latency via gateway_http_request_duration_seconds`
   - **File**: `test/e2e/gateway/30_observability_test.go`
   - **Root Cause**: Related to dedup logic affecting metric counts

3. ✗ **Test 31**: `prevents duplicate CRDs for identical Prometheus alerts using fingerprint`
   - **File**: `test/e2e/gateway/31_prometheus_adapter_test.go`
   - **Root Cause**: Same dedup failure

4. ✗ **Test 36** (Processing): `should detect duplicate and increment occurrence count`
   - **File**: `test/e2e/gateway/36_deduplication_state_test.go:286`
   - **Scenario**: CRD in Processing phase should trigger dedup

5. ✗ **Test 36** (invalid): `should treat as duplicate (conservative fail-safe)`
   - **File**: `test/e2e/gateway/36_deduplication_state_test.go:597`
   - **Scenario**: Unknown/invalid CRD state should trigger dedup

### **Investigation Steps**

#### **Step 1: Verify Gateway Field Indexing Works** ✓ (Already Done)

**Finding**: Gateway configures field indexing at startup (confirmed in `pkg/gateway/server.go:230-241`)

```go
// BR-GATEWAY-185 v1.1: Create cached client with field index
if err := k8sCache.IndexField(ctx, &remediationv1alpha1.RemediationRequest{},
    "spec.signalFingerprint",
    func(obj client.Object) []string {
        rr := obj.(*remediationv1alpha1.RemediationRequest)
        return []string{rr.Spec.SignalFingerprint}
    }); err != nil {
    cancel()
    return nil, fmt.Errorf("failed to create fingerprint field index: %w", err)
}
```

**Conclusion**: Field indexing IS configured correctly.

#### **Step 2: Check if First Signal Creates CRD** ⏸️ (TODO)

**Hypothesis**: First signal might be failing silently, so second signal appears as "first" → returns 201.

**Commands**:
```bash
# Run Test 30 with verbose logging
make test-e2e-gateway GINKGO_ARGS="-v --focus='should track deduplicated signals'" 2>&1 | tee /tmp/test30-debug.log

# Check if CRDs are being created
grep -A5 -B5 "First alert" /tmp/test30-debug.log
grep -A5 -B5 "Second alert" /tmp/test30-debug.log
grep "RemediationRequest created" /tmp/test30-debug.log
```

**What to look for**:
- Does first signal return 201 (Created)?
- Is CRD actually created in K8s?
- Does second signal query find the first CRD?

#### **Step 3: Check Gateway Deduplication Query Logic** ⏸️ (TODO)

**Hypothesis**: Gateway's `PhaseBasedDeduplicationChecker.ShouldDeduplicate()` might not be querying correctly.

**Files to check**:
- `pkg/gateway/processing/phase_checker.go` (deduplication logic)
- Gateway logs during E2E test run

**Commands**:
```bash
# Get Gateway pod logs during test
kubectl --kubeconfig ~/.kube/gateway-e2e-config logs -n kubernaut-system deployment/gateway --tail=500 > /tmp/gateway-logs.txt

# Search for deduplication attempts
grep -i "dedup\|fingerprint\|ShouldDeduplicate" /tmp/gateway-logs.txt
```

**What to look for**:
- Are deduplication queries being executed?
- Do queries return results?
- Are there any errors in deduplication logic?

#### **Step 4: Check Namespace Isolation** ⏸️ (TODO)

**Hypothesis**: Each test using different namespace might prevent fingerprint matches.

**Files to check**:
- `test/e2e/gateway/30_observability_test.go` (namespace creation)
- Gateway deduplication logic (does it query cross-namespace?)

**Commands**:
```bash
# Check Test 30's namespace usage
grep -A10 "testNamespace.*GenerateUniqueNamespace" test/e2e/gateway/30_observability_test.go

# Check if Gateway deduplication is namespace-scoped
grep -A20 "ShouldDeduplicate.*namespace" pkg/gateway/processing/phase_checker.go
```

**What to look for**:
- Does Test 30 create a unique namespace per test?
- Does deduplication query scope to single namespace?
- Should deduplication be cross-namespace or per-namespace?

#### **Step 5: Check Gateway Cache Synchronization** ⏸️ (TODO)

**Hypothesis**: Gateway's K8s cache might not have synced before second signal arrives.

**Files to check**:
- `pkg/gateway/server.go` (cache sync logic at lines 250-257)

**Current Implementation**:
```go
// Wait for cache sync (timeout after 30s)
syncCtx, syncCancel := context.WithTimeout(ctx, 30*time.Second)
defer syncCancel()
if !k8sCache.WaitForCacheSync(syncCtx) {
    cancel()
    return nil, fmt.Errorf("failed to sync Kubernetes cache (timeout)")
}
```

**What to check**:
- Is cache sync completing successfully?
- Is there lag between CRD creation and cache visibility?
- Should we add delay between first and second signal in tests?

### **Potential Fixes (Priority Order)**

#### **Fix 1: Add Test Delay for Cache Sync** (If cache lag is the issue)

**File**: `test/e2e/gateway/30_observability_test.go`

```go
// First request (creates CRD)
resp1 := SendWebhook(gatewayURL, payload)
Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

// Wait for Gateway cache to sync CRD
time.Sleep(2 * time.Second) // ← Add delay

// Second request (deduplicated)
resp2 := SendWebhook(gatewayURL, payload)
Expect(resp2.StatusCode).To(Equal(http.StatusAccepted))
```

**Risk**: Might not fix if cache sync isn't the issue.

#### **Fix 2: Verify CRD Creation Before Second Signal** (More robust)

**File**: `test/e2e/gateway/30_observability_test.go`

```go
// First request
resp1 := SendWebhook(gatewayURL, payload)
Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

// Verify CRD actually exists in K8s
Eventually(func() int {
    var rrList remediationv1alpha1.RemediationRequestList
    _ = k8sClient.List(ctx, &rrList,
        client.InNamespace(testNamespace),
        client.MatchingFields{"spec.signalFingerprint": expectedFingerprint},
    )
    return len(rrList.Items)
}, 10*time.Second, 500*time.Millisecond).Should(Equal(1), "CRD should exist before sending duplicate")

// Second request (deduplicated)
resp2 := SendWebhook(gatewayURL, payload)
Expect(resp2.StatusCode).To(Equal(http.StatusAccepted))
```

**Benefit**: Ensures CRD exists and is queryable before sending duplicate.

#### **Fix 3: Investigate Gateway Deduplication Logic** (If query logic is broken)

**File**: `pkg/gateway/processing/phase_checker.go`

**Check**:
- Is `MatchingFields` query syntax correct?
- Are there any errors being swallowed?
- Is the query returning empty results even when CRDs exist?

**Potential Issue**: Query might be failing silently, returning "not found", causing Gateway to create new CRD.

### **Success Criteria**

- [ ] Test 30 (both cases) passes
- [ ] Test 31 passes
- [ ] Test 36 (both cases) passes
- [ ] All 5 dedup tests return HTTP 202 for duplicate signals
- [ ] CRDs are queryable by fingerprint
- [ ] No regression in Integration tests (still 100% pass)

### **Validation Commands**

```bash
# Run dedup tests specifically
make test-e2e-gateway GINKGO_ARGS="--focus='dedup|duplicate'" 2>&1 | tee /tmp/dedup-tests.log

# Check pass rate
grep -E "Passed|Failed" /tmp/dedup-tests.log | tail -5
```

---

## 🔴 Phase 2: Audit Integration Fixes (EQUAL PRIORITY)

### **Status**: ⏸️ Not Started
### **Impact**: Blocks 5 tests
### **Estimated Time**: 2-4 hours

### **Root Cause Hypothesis**

DataStorage service connectivity issues or audit event query timeouts.

### **Affected Tests**

1. ✗ **Test 15 [BeforeAll]**: `should emit audit event to Data Storage when signal is ingested`
   - **File**: `test/e2e/gateway/15_audit_trace_validation_test.go`
   - **Impact**: BeforeAll failure → all tests in suite skip

2. ✗ **Test 23** (received): `should create 'signal.received' audit event in Data Storage`
   - **File**: `test/e2e/gateway/23_audit_emission_test.go:317`
   - **Root Cause**: Audit query returning no results or timing out

3. ✗ **Test 23** (deduplicated): `should create 'signal.deduplicated' audit event in Data Storage`
   - **File**: `test/e2e/gateway/23_audit_emission_test.go`
   - **Root Cause**: Same as above

4. ✗ **Test 24**: `should capture all 3 fields in gateway.signal.deduplicated events`
   - **File**: `test/e2e/gateway/24_audit_signal_data_test.go:726`
   - **Root Cause**: Audit event structure/field validation failing

5. ✗ **Test 22**: `should emit standardized error_details on CRD creation failure`
   - **File**: `test/e2e/gateway/22_audit_errors_test.go`
   - **Root Cause**: Audit query for error events failing

### **Investigation Steps**

#### **Step 1: Verify DataStorage Service Running** ⏸️ (TODO)

**Commands**:
```bash
# Check DataStorage pod status
kubectl --kubeconfig ~/.kube/gateway-e2e-config get pods -n kubernaut-system | grep datastorage

# Check DataStorage service
kubectl --kubeconfig ~/.kube/gateway-e2e-config get svc -n kubernaut-system datastorage

# Test DataStorage health endpoint
kubectl --kubeconfig ~/.kube/gateway-e2e-config exec -n kubernaut-system deployment/gateway -- curl -s http://datastorage.kubernaut-system.svc.cluster.local:8080/health
```

**What to look for**:
- Is DataStorage pod running?
- Is service endpoint correct?
- Does health check return 200 OK?

#### **Step 2: Check Gateway Audit Emission** ⏸️ (TODO)

**Commands**:
```bash
# Check Gateway logs for audit emissions
kubectl --kubeconfig ~/.kube/gateway-e2e-config logs -n kubernaut-system deployment/gateway --tail=500 | grep -i "audit\|event"

# Check for audit emission errors
kubectl --kubeconfig ~/.kube/gateway-e2e-config logs -n kubernaut-system deployment/gateway --tail=500 | grep -i "error.*audit"
```

**What to look for**:
- Are audit events being emitted by Gateway?
- Are there any errors during emission?
- Is DataStorage URL configured correctly in Gateway?

#### **Step 3: Check Audit Query Timeout** ⏸️ (TODO)

**Current timeout**: 60 seconds (increased from 10s in earlier fixes)

**Files to check**:
- `test/e2e/gateway/23_audit_emission_test.go` (audit query timeout)
- `test/e2e/gateway/deduplication_helpers.go` (audit query helpers)

**Commands**:
```bash
# Check current timeout values
grep -r "Eventually.*60.*time.Second" test/e2e/gateway/*audit*.go
```

**What to check**:
- Is 60s timeout sufficient?
- Are audit events taking longer to appear in DataStorage?
- Should we increase to 120s?

#### **Step 4: Test Direct Audit Query** ⏸️ (TODO)

**Commands**:
```bash
# Query DataStorage directly for audit events
kubectl --kubeconfig ~/.kube/gateway-e2e-config exec -n kubernaut-system deployment/gateway -- \
  curl -s "http://datastorage.kubernaut-system.svc.cluster.local:8080/api/v1/audit/query?category=gateway.signal.received&limit=10"
```

**What to look for**:
- Does query return results?
- Is the query syntax correct?
- Are events being stored in DataStorage?

### **Potential Fixes (Priority Order)**

#### **Fix 1: Increase Audit Query Timeout**

**Files**: `test/e2e/gateway/*audit*.go`

```go
// BEFORE:
Eventually(func() []AuditEvent {
    // query logic
}, 60*time.Second, 1*time.Second).Should(...)

// AFTER:
Eventually(func() []AuditEvent {
    // query logic
}, 120*time.Second, 2*time.Second).Should(...) // Doubled timeout
```

#### **Fix 2: Add Audit Emission Verification**

**Files**: Audit test files

```go
// After sending signal, verify Gateway emitted audit event
// Check Gateway logs before querying DataStorage
```

#### **Fix 3: Fix DataStorage Service Configuration**

**File**: E2E infrastructure setup

- Verify DataStorage deployment YAML
- Check service endpoints
- Validate Gateway config for DataStorage URL

### **Success Criteria**

- [ ] All 5 audit tests pass
- [ ] Audit events queryable from DataStorage
- [ ] BeforeAll blocks succeed
- [ ] Query timeout sufficient (no false failures)

### **Validation Commands**

```bash
# Run audit tests specifically
make test-e2e-gateway GINKGO_ARGS="--focus='audit|Audit'" 2>&1 | tee /tmp/audit-tests.log

# Check pass rate
grep -E "Passed|Failed" /tmp/audit-tests.log | tail -5
```

---

## 🟡 Phase 3: BeforeAll Setup Fixes

### **Status**: ⏸️ Not Started
### **Impact**: Blocks 2 test suites (all tests skip if BeforeAll fails)
### **Estimated Time**: 1-2 hours

### **Root Cause Hypothesis**

Namespace creation timeout or Gateway health check failure in BeforeAll blocks.

### **Affected Tests**

1. ✗ **Test 04 [BeforeAll]**: `should expose Prometheus metrics that update after processing alerts`
   - **File**: `test/e2e/gateway/04_metrics_endpoint_test.go`

2. ✗ **Test 08 [BeforeAll]**: `should process K8s Events and create CRDs with correct resource information`
   - **File**: `test/e2e/gateway/08_k8s_event_ingestion_test.go`

### **Investigation Steps**

#### **Step 1: Check BeforeAll Logic** ⏸️ (TODO)

**Commands**:
```bash
# Check Test 04 BeforeAll
grep -A30 "BeforeAll" test/e2e/gateway/04_metrics_endpoint_test.go

# Check Test 08 BeforeAll
grep -A30 "BeforeAll" test/e2e/gateway/08_k8s_event_ingestion_test.go
```

**What to look for**:
- Namespace creation logic
- Timeout values
- Health check logic

#### **Step 2: Apply Integration Test Fixes** ⏸️ (TODO)

**We already fixed similar issues in Integration tests**:
- Added retry logic to `CreateNamespaceAndWait`
- Increased timeouts from 10s to 60s
- Added `Eventually` waits for namespace readiness

**Apply same fixes to E2E helpers**.

### **Potential Fixes**

#### **Fix 1: Add Retry Logic to E2E Namespace Creation**

**File**: `test/e2e/gateway/deduplication_helpers.go`

Apply the same fixes we applied to integration tests:
- Retry namespace creation (5 attempts)
- Handle `AlreadyExists` race conditions
- Increase `Eventually` timeout to 60s

#### **Fix 2: Add Gateway Health Check Before Tests**

**Files**: Test 04 and Test 08 BeforeAll

```go
BeforeAll(func() {
    // Create namespace
    Expect(CreateNamespaceAndWait(ctx, k8sClient, testNamespace)).To(Succeed())

    // Verify Gateway is ready
    Eventually(func() int {
        resp, err := http.Get(gatewayURL + "/health")
        if err != nil {
            return 0
        }
        return resp.StatusCode
    }, 30*time.Second, 2*time.Second).Should(Equal(200), "Gateway should be healthy")
})
```

### **Success Criteria**

- [ ] Test 04 BeforeAll succeeds
- [ ] Test 08 BeforeAll succeeds
- [ ] All tests in both suites run (not skipped)

### **Validation Commands**

```bash
# Run Test 04 and 08
make test-e2e-gateway GINKGO_ARGS="--focus='Test 04|Test 08'" 2>&1 | tee /tmp/beforeall-tests.log
```

---

## 🟡 Phase 4: Service Resilience Timeout Fixes

### **Status**: ⏸️ Not Started
### **Impact**: Blocks 3 tests
### **Estimated Time**: 1-2 hours

### **Root Cause Hypothesis**

Tests timing out waiting for DataStorage recovery or Gateway processing under degraded conditions.

### **Affected Tests**

1. ✗ **Test 32** (recovery): `should maintain normal processing when DataStorage recovers`
   - **File**: `test/e2e/gateway/32_service_resilience_test.go:233`
   - **Error**: `Timed out after 45.001s`

2. ✗ **Test 32** (degraded): `BR-GATEWAY-187: should process alerts with degraded functionality when DataStorage unavailable`
   - **File**: `test/e2e/gateway/32_service_resilience_test.go:313`
   - **Error**: `Timed out after 30.001s`

3. ✗ **Test 32** (logging): `should log DataStorage failures without blocking alert processing`
   - **File**: `test/e2e/gateway/32_service_resilience_test.go`
   - **Error**: Timeout

### **Investigation Steps**

#### **Step 1: Check DataStorage Mock Restart Logic** ⏸️ (TODO)

**Commands**:
```bash
# Check Test 32 implementation
grep -A50 "should maintain normal processing when DataStorage recovers" test/e2e/gateway/32_service_resilience_test.go
```

**What to look for**:
- How is DataStorage being "stopped"?
- How is it being "restarted"?
- Is the restart actually happening?

#### **Step 2: Check Gateway Circuit Breaker** ⏸️ (TODO)

**Hypothesis**: Gateway should fail-fast on DataStorage errors, not block.

**Commands**:
```bash
# Check if Gateway has circuit breaker for DataStorage
grep -r "circuit.*breaker\|fail.*fast" pkg/gateway/
```

### **Potential Fixes**

#### **Fix 1: Increase Timeout**

**File**: `test/e2e/gateway/32_service_resilience_test.go`

```go
// BEFORE:
Eventually(func() bool {
    // recovery check
}, 45*time.Second, 1*time.Second).Should(BeTrue())

// AFTER:
Eventually(func() bool {
    // recovery check
}, 90*time.Second, 2*time.Second).Should(BeTrue())
```

#### **Fix 2: Verify DataStorage Mock Restart**

Add explicit verification that DataStorage is back online before checking Gateway behavior.

### **Success Criteria**

- [ ] All 3 Test 32 variants pass
- [ ] Tests complete within timeout
- [ ] Gateway processes alerts under degraded conditions

### **Validation Commands**

```bash
# Run Test 32
make test-e2e-gateway GINKGO_ARGS="--focus='Test 32|resilience'" 2>&1 | tee /tmp/resilience-tests.log
```

---

## 🟡 Phase 5: Error Handling Fixes

### **Status**: ⏸️ Not Started
### **Impact**: Blocks 2 tests
### **Estimated Time**: 1 hour

### **Root Cause Hypothesis**

Fallback namespace logic not working or resource metadata extraction failing.

### **Affected Tests**

1. ✗ **Test 27**: `handles namespace not found by using kubernaut-system namespace fallback`
   - **File**: `test/e2e/gateway/27_error_handling_test.go`

2. ✗ **Test 01**: `extracts resource information for AI targeting and remediation`
   - **File**: `test/e2e/gateway/01_prometheus_webhook_test.go`

### **Investigation Steps**

#### **Step 1: Verify Fallback Namespace Exists** ⏸️ (TODO)

**Commands**:
```bash
# Check if kubernaut-system namespace exists
kubectl --kubeconfig ~/.kube/gateway-e2e-config get namespace kubernaut-system

# Check Gateway RBAC for fallback namespace
kubectl --kubeconfig ~/.kube/gateway-e2e-config get rolebinding -n kubernaut-system | grep gateway
```

### **Potential Fixes**

#### **Fix 1: Ensure Fallback Namespace Exists**

**File**: E2E infrastructure setup

Add `kubernaut-system` namespace creation if missing.

#### **Fix 2: Check Gateway Fallback Logic**

**File**: `pkg/gateway/processing/crd_creator.go`

Verify fallback logic is working correctly.

### **Success Criteria**

- [ ] Test 27 passes
- [ ] Test 01 passes
- [ ] Fallback namespace used when target namespace missing

### **Validation Commands**

```bash
# Run error handling tests
make test-e2e-gateway GINKGO_ARGS="--focus='Test 27|Test 01|error'" 2>&1 | tee /tmp/error-tests.log
```

---

## 📁 File References

### **Key Files to Investigate/Modify**

#### **Test Files**:
- `test/e2e/gateway/30_observability_test.go` (dedup failure)
- `test/e2e/gateway/31_prometheus_adapter_test.go` (dedup failure)
- `test/e2e/gateway/36_deduplication_state_test.go` (dedup failure)
- `test/e2e/gateway/23_audit_emission_test.go` (audit failure)
- `test/e2e/gateway/15_audit_trace_validation_test.go` (audit BeforeAll)
- `test/e2e/gateway/04_metrics_endpoint_test.go` (BeforeAll failure)
- `test/e2e/gateway/08_k8s_event_ingestion_test.go` (BeforeAll failure)
- `test/e2e/gateway/32_service_resilience_test.go` (timeout)
- `test/e2e/gateway/27_error_handling_test.go` (fallback)

#### **Helper Files**:
- `test/e2e/gateway/deduplication_helpers.go` (namespace creation, audit queries)

#### **Gateway Code**:
- `pkg/gateway/server.go` (field indexing, cache sync)
- `pkg/gateway/processing/phase_checker.go` (deduplication logic)
- `pkg/gateway/processing/crd_creator.go` (CRD creation, fallback)

#### **Logs** (from this session):
- `/tmp/gateway-e2e-run.log` (complete E2E test output)
- Retrieve with: `cat /tmp/gateway-e2e-run.log | grep -A10 -B10 "FAIL"`

---

## 📊 Progress Tracking

### **Session 1 (Current)** - Jan 13, 2026
- ✅ TTL cleanup completed (Integration + Unit 100%)
- ✅ Comprehensive triage document created (`E2E_FAILURES_TRIAGE_JAN13_2026.md`)
- ✅ Work roadmap created (this document)
- ✅ Gateway field indexing verified (NOT the issue)
- ⏸️ Starting Phase 1 investigation (Deduplication)

### **Next Session TODO**
- [ ] Phase 1: Investigate deduplication failures (Step 2: Check if first signal creates CRD)
- [ ] Run Test 30 with verbose logging
- [ ] Check Gateway deduplication query logs

---

## 🎯 Success Metrics

### **Phase Completion Criteria**

| Phase | Tests Fixed | Pass Rate Target | Status |
|-------|-------------|------------------|--------|
| Baseline | 0 | 82.7% (81/98) | ✅ Current |
| Phase 1 | +5 | 87.8% (86/98) | ⏸️ |
| Phase 2 | +5 | 92.9% (91/98) | ⏸️ |
| Phase 3 | +2 | 94.9% (93/98) | ⏸️ |
| Phase 4 | +3 | 98.0% (96/98) | ⏸️ |
| Phase 5 | +2 | 100% (98/98) | ⏸️ **GOAL** |

### **Merge Readiness Checklist**

- [ ] Phase 1 complete (dedup tests passing)
- [ ] Phase 2 complete (audit tests passing)
- [ ] Phase 3 complete (BeforeAll tests passing)
- [ ] Phase 4 complete (resilience tests passing)
- [ ] Phase 5 complete (error handling tests passing)
- [ ] **100% E2E pass rate** (98/98)
- [ ] No regressions in Integration tests (30/30)
- [ ] No regressions in Unit tests (95/95)
- [ ] Documentation updated

---

## 🔗 Related Documents

- `docs/handoff/E2E_FAILURES_TRIAGE_JAN13_2026.md` - Detailed failure analysis
- `docs/handoff/TTL_CLEANUP_COMPLETE_JAN13_2026.md` - TTL cleanup summary
- `docs/handoff/GATEWAY_FIXES_COMPLETE_JAN13_2026.md` - Integration test fixes
- `/tmp/gateway-e2e-run.log` - E2E test output from this session

---

**End of Roadmap**
**Status**: Ready for Phase 1 investigation
**Next Step**: Run Test 30 with verbose logging to debug deduplication failure

