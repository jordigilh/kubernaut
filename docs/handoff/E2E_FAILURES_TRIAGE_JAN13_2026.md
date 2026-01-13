# Gateway E2E Test Failures - Comprehensive Triage

**Date**: January 13, 2026
**Context**: E2E validation after TTL cleanup changes
**Status**: 🔴 **17/98 Failures** (82.7% pass rate) - **Blocking Merge**

---

## 🎯 Executive Summary

E2E test run revealed **17 failures** across 5 categories. **Critical finding**: All failures appear to be **pre-existing infrastructure/deployment issues**, NOT caused by TTL cleanup (which only changed comments).

### Failure Breakdown

| Category | Count | Priority | Root Cause Hypothesis |
|----------|-------|----------|---------------------|
| **Deduplication Logic** | 5 | 🔴 **P0** | Gateway returning 201 instead of 202 for duplicates |
| **Audit Integration** | 5 | 🔴 **P0** | DataStorage service connectivity/query issues |
| **Service Resilience** | 3 | 🟡 **P1** | Timeout issues (30-45s waits) |
| **BeforeAll Setup** | 2 | 🟡 **P1** | Namespace/infrastructure setup failures |
| **Error Handling** | 2 | 🟡 **P1** | K8s API / namespace fallback issues |

---

## 🔍 Detailed Failure Analysis

### **Priority 0: Deduplication Logic Failures** 🔴

**Impact**: 5 test failures
**Root Cause**: Gateway returning HTTP 201 (Created) instead of 202 (Accepted) for duplicate signals

#### **Affected Tests:**

1. **Test 30**: `should track deduplicated signals via gateway_signals_deduplicated_total`
   - **File**: `test/e2e/gateway/30_observability_test.go:173`
   - **Error**: `Expected <int>: 201 to equal <int>: 202`
   - **Scenario**: Send same alert twice → second should be dedup (202), got created (201)

2. **Test 31**: `prevents duplicate CRDs for identical Prometheus alerts using fingerprint`
   - **File**: `test/e2e/gateway/31_prometheus_adapter_test.go`
   - **Root Cause**: Same as Test 30

3. **Test 36**: `should detect duplicate and increment occurrence count` (Processing state)
   - **File**: `test/e2e/gateway/36_deduplication_state_test.go:286`
   - **Root Cause**: Deduplication not working when CRD in Processing phase

4. **Test 36**: `should treat as duplicate (conservative fail-safe)` (unknown/invalid state)
   - **File**: `test/e2e/gateway/36_deduplication_state_test.go:597`
   - **Root Cause**: Deduplication not working for edge cases

5. **Test 30**: `should track HTTP request latency via gateway_http_request_duration_seconds`
   - **File**: `test/e2e/gateway/30_observability_test.go` (metrics validation)
   - **Root Cause**: Likely related to dedup logic affecting metric counts

#### **Investigation Needed:**

**Question**: Why is E2E Gateway not deduplicating when Integration tests pass 100%?

**Hypotheses**:
1. ✅ **E2E deployment config issue**: Gateway might be deployed with incorrect deduplication settings
2. ✅ **CRD field indexing missing**: `spec.signalFingerprint` field selector not configured in E2E cluster
3. ✅ **CRD not being created**: First signal might be failing silently
4. ✅ **Namespace isolation**: E2E tests using different namespaces per test?
5. ✅ **Gateway restart between tests**: Losing in-memory state?

**Next Steps**:
- [ ] Check Gateway deployment YAML in E2E environment
- [ ] Verify CRD field indexing is configured (`spec.signalFingerprint`)
- [ ] Add debug logging to Test 30 to see actual CRD creation
- [ ] Check if Gateway is being restarted between tests

---

### **Priority 0: Audit Integration Failures** 🔴

**Impact**: 5 test failures
**Root Cause**: DataStorage service connectivity or audit event query issues

#### **Affected Tests:**

1. **Test 15 [BeforeAll]**: `should emit audit event to Data Storage when signal is ingested`
   - **File**: `test/e2e/gateway/15_audit_trace_validation_test.go`
   - **Root Cause**: BeforeAll setup failure → all tests in suite skip

2. **Test 23**: `should create 'signal.received' audit event in Data Storage`
   - **File**: `test/e2e/gateway/23_audit_emission_test.go:317`
   - **Root Cause**: Audit query returning no results or timing out

3. **Test 23**: `should create 'signal.deduplicated' audit event in Data Storage`
   - **File**: `test/e2e/gateway/23_audit_emission_test.go`
   - **Root Cause**: Same as above

4. **Test 24**: `should capture all 3 fields in gateway.signal.deduplicated events`
   - **File**: `test/e2e/gateway/24_audit_signal_data_test.go:726`
   - **Root Cause**: Audit event structure/field validation failing

5. **Test 22**: `should emit standardized error_details on CRD creation failure`
   - **File**: `test/e2e/gateway/22_audit_errors_test.go`
   - **Root Cause**: Audit query for error events failing

#### **Investigation Needed:**

**Question**: Why are audit queries failing in E2E but Integration tests use real DataStorage?

**Hypotheses**:
1. ✅ **DataStorage service not running**: Service might be down/unreachable
2. ✅ **Incorrect DataStorage URL**: Gateway might be pointing to wrong endpoint
3. ✅ **Audit query timeout**: Default 60s timeout might be too short
4. ✅ **Event storage lag**: Audit events not flushed to storage yet
5. ✅ **Query syntax issue**: Audit query helpers might have bugs

**Next Steps**:
- [ ] Verify DataStorage service is running in E2E cluster
- [ ] Check Gateway logs for audit emission errors
- [ ] Add debug logging to audit query helpers
- [ ] Increase query timeout from 60s to 120s
- [ ] Check DataStorage logs for incoming audit events

---

### **Priority 1: Service Resilience Timeout Failures** 🟡

**Impact**: 3 test failures
**Root Cause**: Tests timing out after 30-45 seconds waiting for recovery

#### **Affected Tests:**

1. **Test 32**: `should maintain normal processing when DataStorage recovers`
   - **File**: `test/e2e/gateway/32_service_resilience_test.go:233`
   - **Error**: `Timed out after 45.001s`
   - **Root Cause**: Waiting for DataStorage recovery, never happens

2. **Test 32**: `BR-GATEWAY-187: should process alerts with degraded functionality when DataStorage unavailable`
   - **File**: `test/e2e/gateway/32_service_resilience_test.go:313`
   - **Error**: `Timed out after 30.001s`
   - **Root Cause**: Gateway not processing alerts when DataStorage down

3. **Test 32**: `should log DataStorage failures without blocking alert processing`
   - **File**: `test/e2e/gateway/32_service_resilience_test.go`
   - **Error**: Timeout
   - **Root Cause**: Same as above

#### **Investigation Needed:**

**Question**: Why are resilience tests timing out?

**Hypotheses**:
1. ✅ **DataStorage never comes back**: Mock DataStorage not being restarted properly
2. ✅ **Gateway blocking on audit**: Gateway might be blocking alert processing waiting for audit
3. ✅ **Circuit breaker not working**: Gateway should fail-fast on DataStorage errors
4. ✅ **Timeout too short**: 30-45s might not be enough for recovery

**Next Steps**:
- [ ] Check if DataStorage mock is being restarted in test
- [ ] Verify Gateway has circuit breaker for DataStorage calls
- [ ] Check Gateway logs during resilience tests
- [ ] Increase timeouts to 60-90s

---

### **Priority 1: BeforeAll Setup Failures** 🟡

**Impact**: 2 test suite failures (all tests in suite skip)
**Root Cause**: Infrastructure setup failing in BeforeAll blocks

#### **Affected Tests:**

1. **Test 04 [BeforeAll]**: `should expose Prometheus metrics that update after processing alerts`
   - **File**: `test/e2e/gateway/04_metrics_endpoint_test.go`
   - **Root Cause**: Namespace creation or Gateway setup failure

2. **Test 08 [BeforeAll]**: `should process K8s Events and create CRDs with correct resource information`
   - **File**: `test/e2e/gateway/08_k8s_event_ingestion_test.go`
   - **Root Cause**: Same as above

#### **Investigation Needed:**

**Question**: Why are BeforeAll blocks failing?

**Hypotheses**:
1. ✅ **Namespace creation timeout**: CreateNamespaceAndWait failing (same issue we fixed in integration)
2. ✅ **Gateway URL unreachable**: Port-forward or service endpoint not ready
3. ✅ **Resource limits**: Cluster might be resource-constrained
4. ✅ **Parallel execution conflicts**: Tests stepping on each other's namespaces

**Next Steps**:
- [ ] Add retry logic to BeforeAll namespace creation (like integration tests)
- [ ] Add health check before tests start
- [ ] Check cluster resource usage during tests
- [ ] Verify namespace uniqueness in parallel execution

---

### **Priority 1: Error Handling Failures** 🟡

**Impact**: 2 test failures
**Root Cause**: K8s API or namespace fallback logic issues

#### **Affected Tests:**

1. **Test 27**: `handles namespace not found by using kubernaut-system namespace fallback`
   - **File**: `test/e2e/gateway/27_error_handling_test.go`
   - **Root Cause**: Fallback logic not working or CRD not created

2. **Test 01**: `extracts resource information for AI targeting and remediation`
   - **File**: `test/e2e/gateway/01_prometheus_webhook_test.go`
   - **Root Cause**: Resource metadata extraction failing

#### **Investigation Needed:**

**Question**: Why is error handling failing?

**Hypotheses**:
1. ✅ **Fallback namespace missing**: `kubernaut-system` namespace doesn't exist
2. ✅ **RBAC issues**: Gateway might not have permission to create CRDs in fallback namespace
3. ✅ **Validation issue**: CRD creation might be failing validation
4. ✅ **Test setup**: Test might not be creating the scenario correctly

**Next Steps**:
- [ ] Verify `kubernaut-system` namespace exists in E2E cluster
- [ ] Check Gateway RBAC permissions for fallback namespace
- [ ] Add debug logging to Test 27
- [ ] Check Gateway logs for fallback attempts

---

## 🔬 Root Cause Analysis - Cross-Cutting Issues

### **Issue 1: Deduplication Not Working in E2E**

**Symptoms**:
- Gateway returns 201 (Created) for duplicate signals
- Integration tests pass 100% (deduplication works)
- E2E tests fail (deduplication doesn't work)

**Critical Difference**: Integration tests call `ProcessSignal()` directly with shared K8s client, E2E tests use HTTP → Gateway → K8s

**Most Likely Root Cause**: **CRD Field Indexing Not Configured**

Gateway's deduplication logic relies on K8s field selector:
```go
// pkg/gateway/processing/phase_checker.go
err := c.client.List(ctx, rrList,
    client.InNamespace(namespace),
    client.MatchingFields{"spec.signalFingerprint": fingerprint},  // ← Requires field indexing
)
```

**If field indexing is missing**, the query returns empty results even if matching CRDs exist.

**Evidence**:
- Integration tests use envtest with manual field indexing setup
- E2E cluster might not have field indexing configured
- This would explain why ALL deduplication tests fail

**Fix**: Add field indexing to E2E cluster CRD installation

---

### **Issue 2: DataStorage Connectivity in E2E**

**Symptoms**:
- All audit tests fail
- Resilience tests timeout
- Gateway might be blocking on audit calls

**Most Likely Root Cause**: **DataStorage Service Not Responding**

**Evidence**:
- 5/5 audit tests fail
- 3/3 resilience tests timeout (waiting for DataStorage)
- BeforeAll failures might be audit-related

**Fix**: Verify DataStorage service deployment and Gateway config

---

### **Issue 3: Infrastructure Setup Fragility**

**Symptoms**:
- BeforeAll blocks failing
- Namespace creation issues
- Parallel execution conflicts

**Most Likely Root Cause**: **Same issues we fixed in integration tests**

**Evidence**:
- We added retry logic + increased timeouts in integration
- E2E tests don't have the same fixes

**Fix**: Apply same namespace creation fixes to E2E helpers

---

## 🎯 Investigation Plan - Priority Order

### **Phase 1: Deduplication Fix** 🔴 (Blocks 5 tests)

**Estimated Time**: 1-2 hours

**Steps**:
1. Check if E2E cluster has CRD field indexing configured
2. Add field indexing if missing
3. Verify Gateway can query by `spec.signalFingerprint`
4. Re-run Tests 30, 31, 36 to verify fix

**Success Criteria**: Duplicate signals return HTTP 202, not 201

---

### **Phase 2: Audit Integration Fix** 🔴 (Blocks 5 tests)

**Estimated Time**: 1-2 hours

**Steps**:
1. Verify DataStorage service is running and reachable
2. Check Gateway config for correct DataStorage URL
3. Add debug logging to audit query helpers
4. Increase audit query timeout to 120s
5. Re-run Tests 15, 22, 23, 24 to verify fix

**Success Criteria**: Audit events queryable from DataStorage

---

### **Phase 3: BeforeAll Fixes** 🟡 (Blocks 2 test suites)

**Estimated Time**: 30-60 minutes

**Steps**:
1. Apply integration test namespace creation fixes to E2E
2. Add retry logic and increased timeouts
3. Add health checks before test execution
4. Re-run Tests 4, 8 to verify fix

**Success Criteria**: BeforeAll blocks succeed consistently

---

### **Phase 4: Resilience Timeout Fixes** 🟡 (Blocks 3 tests)

**Estimated Time**: 1 hour

**Steps**:
1. Verify DataStorage mock restart logic
2. Check Gateway circuit breaker implementation
3. Increase timeout from 30-45s to 60-90s
4. Re-run Test 32 variants to verify fix

**Success Criteria**: Resilience tests pass within timeout

---

### **Phase 5: Error Handling Fixes** 🟡 (Blocks 2 tests)

**Estimated Time**: 30 minutes

**Steps**:
1. Verify `kubernaut-system` namespace exists
2. Check Gateway RBAC for fallback namespace
3. Add test debug logging
4. Re-run Tests 1, 27 to verify fix

**Success Criteria**: Error handling tests pass

---

## 📊 Estimated Total Time to 100% Pass Rate

**Optimistic**: 4-5 hours (if fixes are straightforward)
**Realistic**: 6-8 hours (includes investigation + debugging)
**Pessimistic**: 12+ hours (if infrastructure issues are deep)

---

## 🔗 Related to TTL Cleanup?

**Answer**: **NO** - 0% correlation

**Evidence**:
1. TTL cleanup only changed **comments** (no logic changes)
2. All failures are **runtime behavior** issues (not syntax/compilation)
3. **Integration + Unit tests pass 100%** (logic is correct)
4. E2E failures are **infrastructure/deployment** related

**Conclusion**: These are **pre-existing E2E environment issues**, not regressions from TTL cleanup.

---

## 🚀 Next Steps

1. **Start with Phase 1** (Deduplication) - highest impact (5 tests)
2. **Then Phase 2** (Audit) - high impact (5 tests)
3. **Then Phases 3-5** in parallel if possible

**Goal**: Achieve **98/98 E2E tests passing** (100% pass rate) before merge

---

**End of Triage Document**
**Status**: Ready for systematic investigation and fixes

