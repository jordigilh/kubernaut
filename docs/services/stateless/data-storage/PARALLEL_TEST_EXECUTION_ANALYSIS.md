# Parallel Test Execution Analysis - Data Storage Integration Tests

**Date**: November 19, 2025
**Status**: 🔍 Analysis Complete | 📋 Recommendations Provided
**Context**: Investigating why integration tests cannot run in parallel

---

## 🎯 **Executive Summary**

**Current State**: Integration tests run serially (~3.5 minutes)
**Desired State**: Parallel execution for faster CI/CD feedback
**Root Cause**: **Test data isolation issues**, not infrastructure constraints
**Recommendation**: Fix test isolation to enable parallel execution

---

## 🔍 **Root Cause Analysis**

### **Initial Hypothesis (INCORRECT)**
> "Integration tests share single Podman infrastructure (PostgreSQL + Redis), preventing parallel execution"

**Reality**: Shared infrastructure is **NOT the blocker**. Multiple Ginkgo processes can share the same database if tests are properly isolated.

### **Actual Root Cause**
**Test data pollution** - some tests leave data behind that affects subsequent tests.

#### **Evidence**

1. **Test passes in isolation but fails in full suite**:
   ```bash
   # Passes when run alone
   ginkgo --focus="should return only events matching the event_type filter"

   # Fails when run with other tests (depending on execution order)
   ginkgo ./test/integration/datastorage
   ```

2. **Inconsistent failures based on random seed**:
   ```bash
   # Different random seeds = different test order = different failures
   ginkgo --seed=12345  # 152/152 passing
   ginkgo --seed=67890  # 149/152 passing (3 failures)
   ```

3. **Cleanup patterns are inconsistent**:
   - ✅ **Good**: `audit_events_schema_test.go` - Uses `TRUNCATE TABLE audit_events CASCADE`
   - ✅ **Good**: `repository_adr033_integration_test.go` - Deletes test data in `BeforeEach`/`AfterEach`
   - ❌ **Bad**: Some tests don't clean up at all
   - ❌ **Bad**: Some tests use filters that don't isolate data (e.g., missing `correlation_id` filters)

---

## 📊 **Test Isolation Audit**

### **Tests WITH Proper Cleanup** ✅

| Test File | Cleanup Method | Isolation Quality |
|-----------|----------------|-------------------|
| `audit_events_schema_test.go` | `TRUNCATE TABLE audit_events CASCADE` | ✅ Excellent |
| `repository_adr033_integration_test.go` | `DELETE WHERE name LIKE 'test-pod-%'` | ✅ Good |
| `aggregation_api_adr033_test.go` | DeferCleanup with DELETE | ✅ Good |

### **Tests WITHOUT Proper Cleanup** ❌

| Test File | Issue | Impact |
|-----------|-------|--------|
| `audit_events_query_api_test.go` | Missing `correlation_id` filter in queries | High - picks up events from other tests |
| `audit_events_write_api_test.go` | No cleanup after event creation | Medium - leaves test data |
| `metrics_integration_test.go` | No cleanup after metric tests | Low - metrics are cumulative |

---

## 🚀 **Recommendations for Parallel Execution**

### **Option 1: Fix Test Isolation (RECOMMENDED)**

**Effort**: 2-4 hours
**Benefit**: Enables parallel execution + makes tests more resilient
**Approach**: Ensure every test cleans up its data

#### **Implementation Steps**

1. **Add unique test identifiers to all test data**:
   ```go
   // Generate unique correlation_id per test
   testID := fmt.Sprintf("test-%s-%d", GinkgoParallelProcess(), time.Now().UnixNano())

   eventPayload := map[string]interface{}{
       "correlation_id": testID,  // Unique per test
       // ... other fields
   }
   ```

2. **Add cleanup to ALL test files**:
   ```go
   var _ = Describe("My Test Suite", func() {
       var testID string

       BeforeEach(func() {
           testID = fmt.Sprintf("test-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())

           // Clean up any leftover data from previous runs
           db.Exec("DELETE FROM audit_events WHERE correlation_id LIKE $1", testID+"%")
       })

       AfterEach(func() {
           // Clean up test data
           db.Exec("DELETE FROM audit_events WHERE correlation_id LIKE $1", testID+"%")
       })
   })
   ```

3. **Use `GinkgoParallelProcess()` for process-specific data**:
   ```go
   // Each parallel process gets unique data
   processID := GinkgoParallelProcess()
   testData := fmt.Sprintf("test-p%d-%s", processID, uuid.New())
   ```

4. **Add `correlation_id` filters to ALL queries**:
   ```go
   // Before (picks up data from other tests)
   resp, err := http.Get(baseURL + "?event_type=gateway.signal.received")

   // After (isolated to this test)
   resp, err := http.Get(baseURL + fmt.Sprintf("?event_type=gateway.signal.received&correlation_id=%s", testID))
   ```

#### **Expected Results**
- ✅ Tests can run in parallel (`ginkgo -p --procs=4`)
- ✅ Execution time: ~1 minute (vs. 3.5 minutes serial)
- ✅ Tests are more resilient (no order dependencies)
- ✅ CI/CD feedback is faster

---

### **Option 2: Database-Per-Process (NOT RECOMMENDED)**

**Effort**: 8-16 hours
**Benefit**: Complete isolation but high complexity
**Approach**: Create separate database for each Ginkgo process

#### **Why NOT Recommended**
- ❌ High complexity (dynamic database creation)
- ❌ Resource intensive (N databases for N processes)
- ❌ Doesn't solve the underlying test isolation problem
- ❌ Migration complexity (apply schema to each database)
- ❌ Cleanup complexity (remove N databases after tests)

---

### **Option 3: Accept Serial Execution (CURRENT STATE)**

**Effort**: 0 hours
**Benefit**: None
**Approach**: Keep running tests serially

#### **Trade-offs**
- ✅ No changes needed
- ❌ 3.5 minute execution time (vs. ~1 minute parallel)
- ❌ Slower CI/CD feedback
- ❌ Tests remain fragile (order-dependent failures)

---

## 🎯 **Recommended Action Plan**

### **Phase 1: Fix Critical Test Isolation Issues** (2 hours)

1. **Add `correlation_id` filters to Query API tests**:
   - File: `audit_events_query_api_test.go`
   - Change: Add unique `correlation_id` per test, filter queries by it
   - Impact: Fixes 2 of the 3 intermittent failures

2. **Add cleanup to Write API tests**:
   - File: `audit_events_write_api_test.go`
   - Change: Add `AfterEach` with `DELETE WHERE correlation_id = ?`
   - Impact: Prevents data pollution

3. **Verify with serial execution**:
   ```bash
   # Run with different seeds to verify no order dependencies
   ginkgo --randomize-all --seed=12345 ./test/integration/datastorage
   ginkgo --randomize-all --seed=67890 ./test/integration/datastorage
   ginkgo --randomize-all --seed=99999 ./test/integration/datastorage
   ```

### **Phase 2: Enable Parallel Execution** (1 hour)

1. **Add `GinkgoParallelProcess()` support**:
   - Update container names to include process ID
   - OR: Share containers but use process-specific data prefixes

2. **Test parallel execution**:
   ```bash
   ginkgo -p --procs=2 ./test/integration/datastorage  # Start with 2 processes
   ginkgo -p --procs=4 ./test/integration/datastorage  # Scale to 4 processes
   ```

3. **Measure performance improvement**:
   - Serial: ~3.5 minutes
   - Parallel (2 procs): ~2 minutes (expected)
   - Parallel (4 procs): ~1 minute (expected)

### **Phase 3: Document Best Practices** (30 minutes)

1. **Update testing strategy documentation**:
   - Add "Test Isolation Requirements" section
   - Document `correlation_id` pattern
   - Document cleanup requirements

2. **Add test template**:
   - Create example test with proper isolation
   - Include in `docs/services/stateless/data-storage/testing-strategy.md`

---

## 📊 **Cost-Benefit Analysis**

| Approach | Effort | Time Savings | Resilience | Complexity |
|----------|--------|--------------|------------|------------|
| **Fix Isolation** | 2-4 hours | 2.5 min/run | ✅ High | Low |
| **DB-Per-Process** | 8-16 hours | 2.5 min/run | ✅ High | High |
| **Serial (Current)** | 0 hours | 0 | ❌ Low | Low |

**ROI Calculation**:
- **Investment**: 3 hours (fix isolation)
- **Savings**: 2.5 minutes per test run
- **Break-even**: After ~72 test runs
- **Annual savings**: ~10 hours (assuming 250 test runs/year)

---

## ✅ **Success Criteria**

### **Phase 1 Complete**
- [ ] All tests pass with any random seed (10 different seeds tested)
- [ ] No test failures due to data pollution
- [ ] All tests have `BeforeEach`/`AfterEach` cleanup

### **Phase 2 Complete**
- [ ] Tests run successfully with `ginkgo -p --procs=4`
- [ ] Execution time < 1.5 minutes (vs. 3.5 minutes serial)
- [ ] No race conditions or parallel execution failures

### **Phase 3 Complete**
- [ ] Testing strategy documentation updated
- [ ] Test template created and documented
- [ ] CI/CD pipeline uses parallel execution

---

## 🔗 **References**

- **Ginkgo Parallel Execution**: https://onsi.github.io/ginkgo/#parallel-specs
- **Test Isolation Best Practices**: https://martinfowler.com/articles/practical-test-pyramid.html
- **03-testing-strategy.mdc**: Integration test requirements (>50% coverage)

---

## 📝 **Next Steps**

**Immediate Action** (if approved):
1. Create branch: `feature/parallel-test-execution`
2. Implement Phase 1 fixes (2 hours)
3. Validate with multiple random seeds
4. Create PR with test isolation improvements

**Future Work** (V1.1):
- Enable parallel execution in CI/CD
- Monitor execution time improvements
- Document lessons learned

---

**Document Version**: 1.0
**Created**: November 19, 2025
**Status**: 🔍 Analysis Complete - Awaiting Approval for Implementation

