# DD-AUTH-014: Performance Assertions Removed from E2E Tests

**Date**: 2026-01-26
**Status**: Decision Documented
**Authority**: User Decision - "we can't validate performance on a local environment"

---

## 🎯 Decision Summary

**Performance assertions have been removed from E2E tests** because:
1. E2E tests validate **functionality**, not performance SLAs
2. E2E environment (Kind cluster, CI/CD) has variable, unreliable performance
3. Performance testing requires dedicated load/performance test suite

---

## 📋 Changes Made

### Test 1: `04_workflow_search_test.go:372` ✅
**Before**:
```go
Expect(searchDuration).To(BeNumerically("<", 1000*time.Millisecond),
    "Search latency should be <1s for E2E test (Docker/Kind overhead)")
```

**After**:
```go
// Assertion 5: Search latency measurement (no assertion in E2E - performance tests only)
// Note: E2E validates functionality, not performance SLAs
// Performance benchmarks belong in separate load/performance test suite
// DD-AUTH-014: E2E environment has variable latency (Kind, SAR middleware, 12 parallel processes)
testLogger.Info("Search latency measured (no assertion)", "duration", searchDuration)
```

**Rationale**: No BR defines <1s for search operations. E2E environment != production.

---

### Test 2: `06_workflow_search_audit_test.go:439` ✅
**Before**:
```go
// ASSERT: Average search latency should be <200ms
// Per BR-AUDIT-024: Audit writes use buffered async pattern, search latency < 50ms impact
Expect(avgDuration).To(BeNumerically("<", 200*time.Millisecond),
    "Average search latency should be <200ms (async audit should not block)")
```

**After**:
```go
// NOTE: Performance assertions removed from E2E tests (DD-AUTH-014)
// BR-AUDIT-024 validates audit write IMPACT (<50ms overhead), not absolute search latency
// E2E tests validate functionality; performance testing requires dedicated load test suite
// E2E environment has variable latency: Kind cluster, SAR middleware, 12 parallel processes
testLogger.Info("✅ Async audit behavior validated (functionality only, no performance assertion)",
    "avg_latency", avgDuration,
    "num_searches", numSearches)
```

**Rationale**: BR-AUDIT-024 is about audit write **overhead** (<50ms impact), not absolute search latency. Test was misinterpreting the BR.

---

### Test 2b: `06_workflow_search_audit_test.go:365` ✅
**Before**:
```go
Expect(searchMetadata["duration_ms"]).To(And(
    BeNumerically(">=", 0),
    BeNumerically("<", 2000),
), "Search duration should be non-negative and complete within 2s")
```

**After**:
```go
// Performance upper bound removed - E2E tests validate functionality, not performance
Expect(searchMetadata["duration_ms"]).To(BeNumerically(">=", 0),
    "Search duration should be non-negative")
```

**Rationale**: E2E tests should only validate functional correctness (duration_ms exists and is non-negative), not performance bounds.

---

### Test 4: `05_soc2_compliance_test.go:157` ✅
**Before**:
```go
}, 30*time.Second, 2*time.Second).Should(Equal(200),
    fmt.Sprintf("Certificate generation should complete within 30s..."))
```

**After**:
```go
}, 60*time.Second, 2*time.Second).Should(Equal(200),
    fmt.Sprintf("Certificate generation should complete within 60s..."))
```

**Rationale**: Infrastructure setup timeout, not a performance assertion. cert-manager webhook needs more time in loaded Kind cluster (12 parallel processes).

---

## 📊 Other Performance Assertions (Not Changed)

The following performance assertions remain but are **not critical** for DD-AUTH-014:

| File | Line | Assertion | Type | Action |
|------|------|-----------|------|--------|
| `11_connection_pool_exhaustion_test.go` | 203 | `<30s` for 50 requests | Timeout | Can remove later |
| `11_connection_pool_exhaustion_test.go` | 345 | `<1s` for recovery | Timeout | Can remove later |
| `14_audit_batch_write_api_test.go` | 384 | `<5s` for 100 events | Performance | Can remove later |

**Recommendation**: Remove these in a future cleanup task if they cause flaky test failures.

---

## ✅ Business Requirements Validation

### BR-AUDIT-024: Asynchronous Non-Blocking Audit
**Requirement**: "Audit writes use buffered async pattern, **search latency < 50ms impact**"

**Interpretation**:
- ❌ **WRONG**: Search operations should complete in <200ms
- ✅ **CORRECT**: Audit writes should add <50ms **overhead** to search operations

**Test Design**:
- E2E test validates **functionality**: Audit events are generated for workflow searches
- Performance test (future) should validate **overhead**: `latency_with_audit - latency_without_audit < 50ms`

---

## 📚 Authoritative Documentation

### Performance SLAs (production-only)
**Reference**: `docs/services/stateless/data-storage/performance-requirements.md`

**Latency SLAs for WRITE operations**:
- p50: <250ms
- p95: <1s
- p99: <2s

**NOTE**: These SLAs are for **audit write operations** in **production**, not for:
- Search/query operations (no documented SLA)
- E2E test environment (Kind cluster, CI/CD)

---

## 🚀 Testing Strategy

### E2E Tests (CI/CD)
**Purpose**: Validate functionality, business logic, integration
**Environment**: Kind cluster, Podman/Docker, variable performance
**Assertions**: Functional correctness only (no performance)

### Performance Tests (Future)
**Purpose**: Validate performance SLAs, throughput, latency
**Environment**: Production-like (dedicated cluster, controlled load)
**Assertions**: Performance metrics, percentiles, throughput

### Load Tests (Future)
**Purpose**: Stress test, capacity planning, bottleneck identification
**Environment**: Production-like (scaled resources)
**Assertions**: Max throughput, breaking points, resource limits

---

## 📝 Lessons Learned

1. **Performance assertions don't belong in E2E tests** - E2E validates functionality, not performance
2. **E2E environment is variable** - Kind cluster, CI/CD, resource contention, SAR middleware overhead
3. **BRs should be interpreted correctly** - BR-AUDIT-024 is about overhead, not absolute latency
4. **Separate concerns** - E2E tests, performance tests, and load tests have different goals

---

## ✅ Completion Status

**All 6 failures resolved**:
1. ✅ Test 22 (`22_audit_validation_helper_test.go`): Fixed unauthenticated client
2. ✅ Test 18 (`18_workflow_duplicate_api_test.go`): Fixed unauthenticated client
3. ✅ Test 1 (`04_workflow_search_test.go:372`): Removed <1s performance assertion
4. ✅ Test 2 (`06_workflow_search_audit_test.go:439`): Removed <200ms performance assertion
5. ✅ Test 2b (`06_workflow_search_audit_test.go:365`): Removed <2s performance assertion
6. ✅ Test 4 (`05_soc2_compliance_test.go:157`): Increased cert-manager timeout 30s → 60s

**Next**: Run full E2E suite to validate 100% pass rate.
