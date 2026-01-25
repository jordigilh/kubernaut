# Gateway E2E Remaining Failures Analysis

**Date**: January 11, 2026
**Current Status**: **80/120 tests passing** (66.7%)
**Remaining**: **40 failures** to investigate
**Priority**: Analysis for next fixes

---

## 📊 **Failure Categorization**

### **Category 1: Observability/Metrics Tests** - 5 failures (Priority P1)

**Failing Tests**:
1. "should track deduplicated signals via gateway_signals_deduplicated_total"
2. "should track successful CRD creation via gateway_crds_created_total"
3. "should include namespace and priority labels in CRD metrics"
4. "should track HTTP request latency via gateway_http_request_duration_seconds"
5. "should include endpoint and status code labels in duration metrics"

**Pattern**: All metrics-related tests failing
**Hypothesis**: Metrics endpoint not accessible or metrics not being recorded
**Priority**: **P1** - Clear category, testable fix

---

### **Category 2: Audit/DataStorage Integration** - 6 failures (Priority P1)

**Failing Tests**:
1. "should create 'signal.received' audit event in Data Storage"
2. "should create 'signal.deduplicated' audit event in Data Storage"
3. "should create 'crd.created' audit event in Data Storage"
4. "should capture all 3 fields in gateway.signal.deduplicated events"
5. Test 15: "Audit Trace Validation" (BeforeAll failure)
6. Various service resilience with DataStorage

**Pattern**: Audit events not being emitted or not queryable
**Hypothesis**: DataStorage integration issue or timing
**Priority**: **P1** - Clear category, affects compliance

---

### **Category 3: BeforeAll Failures** - 5 failures (Priority P0)

**Failing Tests**:
1. Test 10: "CRD Creation Lifecycle" (BeforeAll)
2. Test 12: "Gateway Restart Recovery" (BeforeAll) [Serial]
3. Test 13: "Redis Failure Graceful Degradation" (BeforeAll) [Serial]
4. Test 15: "Audit Trace Validation" (BeforeAll)
5. Test 16: "Structured Logging Verification" (BeforeAll)
6. Test 20: "Security Headers & Observability" (BeforeAll)

**Pattern**: Setup failures preventing entire test suites
**Hypothesis**: Test infrastructure or namespace issues
**Priority**: **P0** - Blocks multiple tests each

---

### **Category 4: Deduplication Tests** - 7 failures (Priority P2)

**Failing Tests**:
1. Test 02: "State-Based Deduplication" (should deduplicate identical alerts)
2. "DD-GATEWAY-009: when CRD doesn't exist" (should create new CRD)
3. "DD-GATEWAY-009: when CRD is in Processing state" (should detect duplicate)
4. "DD-GATEWAY-009: when CRD has unknown/invalid state" (conservative fail-safe)
5. "should prevent duplicate CRDs for identical Prometheus alerts using fingerprint"
6. "should return 202 Accepted for duplicate alerts within TTL window"
7. "tracks duplicate count and timestamps in Redis metadata"

**Pattern**: Deduplication logic issues
**Hypothesis**: State checking or fingerprint matching issues
**Priority**: **P2** - Core business logic

---

### **Category 5: Service Resilience/Error Handling** - 6 failures (Priority P3)

**Failing Tests**:
1. "should log DataStorage failures without blocking alert processing"
2. "should maintain normal processing when DataStorage recovers"
3. "BR-GATEWAY-187: should process alerts with degraded functionality when DataStorage unavailable"
4. "returns clear error for missing required fields"
5. "handles namespace not found by using kubernaut-system namespace fallback"
6. "BR-GATEWAY-189: should NOT retry permanent errors (HTTP 400, validation failures)"

**Pattern**: Error handling and resilience logic
**Hypothesis**: Test logic or failure simulation issues
**Priority**: **P3** - Edge cases

---

### **Category 6: Webhook Integration** - 5 failures (Priority P2)

**Failing Tests**:
1. "creates RemediationRequest CRD from Prometheus AlertManager webhook"
2. "creates CRD from Kubernetes Warning events"
3. "populates spec.targetResource from Prometheus alert for downstream services"
4. "populates spec.targetResource from Kubernetes event for downstream services"
5. "creates RemediationRequest CRD with correct business metadata for AI analysis"

**Pattern**: Webhook processing issues
**Hypothesis**: Payload parsing or CRD field population
**Priority**: **P2** - Core functionality

---

### **Category 7: Concurrent/Edge Cases** - 4 failures (Priority P3)

**Failing Tests**:
1. "should handle concurrent requests for same fingerprint gracefully"
2. "should update deduplication hit count atomically"
3. "should handle 50 concurrent requests without errors"
4. "K8s API Recovery: successfully creates CRD when K8s API recovers"

**Pattern**: Concurrency and edge cases
**Hypothesis**: Race conditions or test timing
**Priority**: **P3** - Complex scenarios

---

### **Category 8: Other** - 2 failures (Priority P3)

**Failing Tests**:
1. "should enforce request timeouts to prevent hanging"
2. "classifies environment from namespace and assigns correct priority"

**Pattern**: Miscellaneous
**Priority**: **P3** - Individual investigation needed

---

## 🎯 **Recommended Fix Priority**

### **Phase 4: Fix BeforeAll Failures** (Expected +10-15 tests)

**Why**: Each BeforeAll failure blocks entire test suite
**Impact**: High - could unlock 10-15 tests
**Effort**: Medium - investigate setup issues
**Files**: Tests 10, 12, 13, 15, 16, 20

---

### **Phase 5: Fix Observability/Metrics** (Expected +5 tests)

**Why**: Clear category, all metrics tests failing
**Impact**: Medium - 5 tests
**Effort**: Low-Medium - likely metrics endpoint access
**Files**: `30_observability_test.go`

---

### **Phase 6: Fix Audit/DataStorage** (Expected +6 tests)

**Why**: Clear category, compliance-critical
**Impact**: Medium - 6 tests
**Effort**: Medium - DataStorage integration
**Files**: `22_audit_errors_test.go`, `23_audit_emission_test.go`, `24_audit_signal_data_test.go`

---

### **Phase 7: Fix Remaining Deduplication** (Expected +7 tests)

**Why**: Core business logic
**Impact**: Medium - 7 tests
**Effort**: Medium - state checking logic
**Files**: `02_state_based_deduplication_test.go`, `36_deduplication_state_test.go`

---

## 📈 **Expected Progress**

| Phase | Expected Pass Rate | Expected Tests Passing | Cumulative from Baseline |
|-------|-------------------|----------------------|--------------------------|
| **Current** | 66.7% | 80 | +26 |
| **Phase 4 (BeforeAll)** | **78.3%** | **92** | **+38** |
| **Phase 5 (Metrics)** | **80.8%** | **97** | **+43** |
| **Phase 6 (Audit)** | **85.8%** | **103** | **+49** |
| **Phase 7 (Dedup)** | **91.7%** | **110** | **+56** |

**Target**: 90%+ pass rate (108+/120 tests)

---

## 🔍 **Investigation Strategy**

### **For BeforeAll Failures**

1. Check namespace creation in BeforeAll blocks
2. Verify resource setup (Gateway pod, DataStorage, etc.)
3. Add proper synchronization with `Eventually()`

### **For Observability/Metrics**

1. Verify metrics endpoint is accessible (`:8080/metrics`)
2. Check if metrics are being registered
3. Investigate timing (metrics may need time to update)

### **For Audit/DataStorage**

1. Verify DataStorage is accessible at `http://127.0.0.1:18091`
2. Check audit event emission timing
3. Review audit query helpers

---

## ✅ **Quick Wins**

**Lowest Effort, Highest Impact**:
1. **BeforeAll namespace fixes** - Same pattern as we just fixed
2. **Metrics endpoint access** - Likely simple connectivity issue
3. **Audit query timing** - Add `Eventually()` wrappers

**Estimated Time**: 2-3 hours for Phases 4-5 (BeforeAll + Metrics)

---

**Status**: 📊 **ANALYSIS COMPLETE**
**Next Action**: Fix BeforeAll failures (Phase 4)
**Owner**: Gateway E2E Test Team
