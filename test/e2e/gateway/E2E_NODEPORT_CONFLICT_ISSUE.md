# Gateway E2E Tests - NodePort Conflict Issue

**Date**: 2025-11-24
**Status**: 🚨 **BLOCKING ISSUE** - All E2E tests failing

---

## 🚨 **Problem Summary**

All Gateway E2E tests (existing + new 4 tests) are failing with:
```
The Service "gateway-service" is invalid: spec.ports[0].nodePort: Invalid value: 30080: provided port is already allocated
```

**Root Cause**: Each E2E test deploys its own Gateway service with NodePort 30080 in a unique namespace. NodePorts are **cluster-wide resources**, so only ONE service can use port 30080 at a time.

---

## 📊 **Failure Statistics**

- **Total E2E Tests**: 23 specs (9 test files)
- **Failed**: 12 specs (all in `BeforeAll` - infrastructure deployment)
- **Skipped**: 11 specs (dependent on failed `BeforeAll`)
- **Passed**: 0 specs

**Affected Tests**:
1. ❌ Test 1: Storm Window TTL Expiration (existing)
2. ❌ Test 2: TTL-Based Deduplication (**NEW**)
3. ❌ Test 3: K8s API Rate Limiting (existing)
4. ❌ Test 4: State-Based Deduplication (existing)
5. ❌ Test 4b: State-Based Deduplication Edge Cases (existing)
6. ❌ Test 5: Storm Buffering (existing)
7. ❌ Test 6: Storm Window TTL (**NEW**)
8. ❌ Test 7: Concurrent Alerts (**NEW**)
9. ❌ Test 8: Metrics Validation (**NEW**)

---

## 🔍 **Technical Analysis**

### **Current E2E Test Architecture**

```
BeforeSuite (ONCE):
  ✅ Create Kind cluster
  ✅ Install CRDs
  ✅ Build & load Gateway Docker image

Each Test BeforeAll:
  ❌ Deploy Redis in unique namespace (e.g., "test-ns-1763952844")
  ❌ Deploy Gateway with NodePort 30080 in unique namespace
  ❌ Wait for Gateway to be ready

Test Execution:
  ❌ Send HTTP requests to http://localhost:30080
  ❌ Verify CRDs created
```

**Problem**: Steps marked ❌ create NodePort conflicts because:
1. Test 1 deploys Gateway → Claims NodePort 30080 ✅
2. Test 2 tries to deploy Gateway → **FAILS** (port already allocated) ❌
3. All subsequent tests fail in `BeforeAll`

### **Why Serial Execution Didn't Fix It**

Even with `--procs=1` (serial execution), the issue persists because:
- Test 1's Gateway service remains deployed after Test 1 completes
- Test 2 tries to create a NEW Gateway service with the same NodePort
- Kubernetes rejects the second service creation

---

## 🎯 **Solution Options**

### **Option A: Shared Gateway Instance (RECOMMENDED)**

**Architecture**:
```
BeforeSuite (ONCE):
  ✅ Create Kind cluster
  ✅ Install CRDs
  ✅ Build & load Gateway Docker image
  ✅ Deploy Gateway ONCE with NodePort 30080 in "gateway-e2e" namespace
  ✅ Deploy Redis ONCE in "gateway-e2e" namespace
  ✅ Wait for Gateway to be ready

Each Test BeforeAll:
  ✅ Create unique test namespace for CRDs only
  ✅ Configure test to send alerts to shared Gateway

Test Execution:
  ✅ Send HTTP requests to http://localhost:30080 (shared Gateway)
  ✅ Verify CRDs created in test-specific namespace

AfterEach:
  ✅ Cleanup test namespace (CRDs only)

AfterSuite:
  ✅ Cleanup Gateway namespace
  ✅ Delete Kind cluster
```

**Pros**:
- ✅ No NodePort conflicts
- ✅ Faster test execution (no repeated Gateway deployments)
- ✅ Matches real-world usage (single Gateway instance)
- ✅ Tests can still use unique namespaces for CRD isolation

**Cons**:
- ⚠️ Tests share Gateway state (Redis cache)
- ⚠️ Requires careful test isolation (unique alert names/fingerprints)
- ⚠️ Requires refactoring all 9 E2E test files

**Implementation Effort**: HIGH (affects all E2E tests)

---

### **Option B: Dynamic NodePort Allocation**

**Architecture**:
```
Each Test BeforeAll:
  ✅ Deploy Redis in unique namespace
  ✅ Deploy Gateway with DYNAMIC NodePort (let K8s assign)
  ✅ Query assigned NodePort
  ✅ Configure test to use dynamic port
```

**Pros**:
- ✅ No NodePort conflicts
- ✅ Each test has isolated Gateway instance
- ✅ No shared state between tests

**Cons**:
- ⚠️ Requires Kind cluster NodePort mapping for dynamic ports (complex)
- ⚠️ Slower test execution (repeated Gateway deployments)
- ⚠️ More complex test setup
- ⚠️ Requires refactoring all 9 E2E test files

**Implementation Effort**: HIGH (affects all E2E tests + Kind config)

---

### **Option C: Sequential Namespace Cleanup**

**Architecture**:
```
Each Test AfterAll:
  ✅ Delete test namespace (including Gateway service)
  ✅ Wait for namespace deletion to complete
  ✅ Verify NodePort 30080 is released

Next Test BeforeAll:
  ✅ Deploy Gateway with NodePort 30080 (now available)
```

**Pros**:
- ✅ Minimal code changes
- ✅ Each test has isolated Gateway instance

**Cons**:
- ⚠️ Slower test execution (wait for namespace deletion between tests)
- ⚠️ Fragile (depends on K8s namespace deletion timing)
- ⚠️ May still have race conditions
- ⚠️ Requires refactoring all 9 E2E test files

**Implementation Effort**: MEDIUM (affects all E2E tests)

---

## 📋 **Recommendation**

**RECOMMENDED**: **Option A - Shared Gateway Instance**

**Rationale**:
1. **Matches Production**: Single Gateway instance serving multiple namespaces
2. **Faster Execution**: No repeated Gateway deployments (~2-3 minutes saved per test)
3. **More Reliable**: No NodePort conflicts or timing issues
4. **Better Test Coverage**: Tests Gateway's ability to handle multi-tenant scenarios

**Implementation Plan**:

### **Phase 1: Update BeforeSuite** (1 hour)
1. Add Gateway deployment to `gateway_e2e_suite_test.go` `BeforeSuite`
2. Deploy Gateway + Redis in "gateway-e2e" namespace
3. Wait for Gateway HTTP endpoint to be responsive
4. Store Gateway URL in suite-level variable

### **Phase 2: Refactor Existing Tests** (2-3 hours)
1. Remove `infrastructure.DeployTestServices()` from each test's `BeforeAll`
2. Keep unique namespace creation for CRD isolation
3. Update tests to use suite-level Gateway URL
4. Add unique alert names/fingerprints to prevent cross-test interference

### **Phase 3: Update New Tests** (30 minutes)
1. Apply same refactoring to 4 new E2E tests
2. Ensure unique alert names/fingerprints

### **Phase 4: Validation** (30 minutes)
1. Run full E2E suite with `--procs=1`
2. Verify all 23 specs pass
3. Verify no NodePort conflicts

**Total Effort**: ~4-5 hours

---

## 🚧 **Current Workaround**

**None Available** - All E2E tests are blocked until this is fixed.

**Temporary Mitigation**:
- Unit tests: ✅ PASSING (235 specs)
- Integration tests: ✅ PASSING (144 specs)
- E2E tests: ❌ BLOCKED (23 specs)

---

## 📝 **Files Requiring Changes**

### **Suite Setup**:
- `test/e2e/gateway/gateway_e2e_suite_test.go` (BeforeSuite, AfterSuite)

### **Existing E2E Tests** (5 files):
- `test/e2e/gateway/01_storm_window_ttl_test.go`
- `test/e2e/gateway/03_k8s_api_rate_limit_test.go`
- `test/e2e/gateway/04_state_based_deduplication_test.go`
- `test/e2e/gateway/04b_state_based_deduplication_edge_cases_test.go`
- `test/e2e/gateway/05_storm_buffering_test.go`

### **New E2E Tests** (4 files):
- `test/e2e/gateway/02_ttl_expiration_test.go`
- `test/e2e/gateway/06_storm_window_ttl_test.go`
- `test/e2e/gateway/07_concurrent_alerts_test.go`
- `test/e2e/gateway/08_metrics_test.go`

### **Infrastructure**:
- `test/infrastructure/gateway.go` (may need shared deployment helper)

---

## ✅ **Success Criteria**

E2E tests are fixed when:
- ✅ All 23 E2E specs pass
- ✅ No NodePort conflicts
- ✅ Tests run in <15 minutes total
- ✅ Tests are isolated (no cross-test interference)

---

## 🎯 **Next Steps**

1. **User Decision**: Choose Option A, B, or C
2. **Implementation**: Refactor E2E test suite per chosen option
3. **Validation**: Run full E2E suite and verify all tests pass
4. **Documentation**: Update E2E test documentation with new architecture

---

**Priority**: **P0 - CRITICAL** (blocks all E2E testing)
**Impact**: All Gateway E2E tests (23 specs)
**Effort**: 4-5 hours (Option A)

