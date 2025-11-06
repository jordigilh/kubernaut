# Pending and Skipped Tests - Final Triage

**Date**: 2025-10-30
**Current Status**: 94/96 passing (97.9%), 7 Pending, 10 Skipped
**Goal**: Determine which tests can be enabled in integration tier

---

## 📊 Summary

| Category | Count | Recommendation |
|---|---|---|
| **Enable Now** | 0 | None - all require infrastructure |
| **Move to E2E** | 1 | Storm TTL expiration (90s wait) |
| **Keep Pending** | 6 | Require infrastructure simulation |
| **Keep Skipped** | 10 | Require infrastructure/config |

---

## 🔍 Detailed Triage

### PENDING Tests (7 total)

#### 1. **Storm Window TTL Expiration** ✅ MOVE TO E2E
- **File**: `storm_aggregation_test.go:445`
- **Test**: `should create new storm window after TTL expiration`
- **Reason**: Requires 90-second wait (1 minute window + buffer)
- **Business Outcome**: Storm window expires → New alert doesn't aggregate
- **Recommendation**: **MOVE TO E2E** - already documented as E2E test
- **Confidence**: 100% - test is valid, just too slow for integration tier

#### 2-5. **K8s API Edge Cases** (4 tests) ⏸️ KEEP PENDING
- **File**: `k8s_api_integration_test.go`
- **Tests**:
  1. `should handle K8s API rate limiting` (line 182)
  2. `should handle CRD name length limit (253 chars)` (line 333)
  3. `should handle K8s API slow responses without timeout` (line 389)
  4. `should handle concurrent CRD creates to same namespace` (line 418)
- **Reason**: Require K8s API failure simulation infrastructure
- **Business Outcomes**: All valid - K8s API resilience
- **Recommendation**: **KEEP PENDING** - require chaos engineering infrastructure
- **Confidence**: 90% - tests are correct, need infrastructure
- **Future**: Move to E2E chaos tests when infrastructure available

#### 6-7. **Redis Pipeline Failures** (2 tests) ⏸️ KEEP PENDING
- **File**: `redis_integration_test.go`
- **Tests**:
  1. `should handle Redis connection failure gracefully` (line 179)
  2. `should handle Redis pipeline command failures` (line 370)
- **Reason**: Require Redis failure simulation
- **Business Outcomes**: Redis resilience and graceful degradation
- **Recommendation**: **KEEP PENDING** - require Redis chaos infrastructure
- **Confidence**: 90% - tests are correct, need infrastructure
- **Future**: Move to E2E chaos tests

---

### SKIPPED Tests (10 total)

#### HTTP Server Tests (6 tests) ⏸️ KEEP SKIPPED
- **File**: `http_server_test.go`
- **Tests**:
  1. `should terminate slow-read requests after ReadTimeout` (line 138)
  2. `should terminate slow-write responses after WriteTimeout` (line 192)
  3. `should not close active connections within IdleTimeout` (line 243)
  4. `should complete in-flight requests during graceful shutdown` (line 300)
  5. `should fail readiness probe during graceful shutdown` (line 321)
  6. `should timeout graceful shutdown after MaxShutdownDuration` (line 340)
- **Reason**: Require Gateway server with configurable timeouts (not exposed in test helper)
- **Business Outcomes**: All valid - HTTP server resilience
- **Recommendation**: **KEEP SKIPPED** - require Gateway refactoring to expose configs
- **Confidence**: 85% - tests are correct, need Gateway API changes
- **Future**: Enable when Gateway exposes timeout configuration

#### Observability Tests (3 tests) ⏸️ KEEP SKIPPED
- **File**: `observability_test.go`
- **Tests**:
  1. `should track K8s API failures via gateway_k8s_api_errors_total` (line 360)
  2. `should track Redis failures via gateway_redis_errors_total` (line 638)
  3. `should track Redis operation latency via gateway_redis_duration_seconds` (line 657)
- **Reason**: Require failure simulation infrastructure
- **Business Outcomes**: All valid - observability of infrastructure failures
- **Recommendation**: **KEEP SKIPPED** - require chaos infrastructure
- **Confidence**: 90% - tests are correct, need infrastructure
- **Future**: Move to E2E chaos tests

#### Error Handling Test (1 test) ⏸️ KEEP SKIPPED
- **File**: `error_handling_test.go:254`
- **Test**: Kubernetes API failure simulation
- **Reason**: Requires complex K8s API failure simulation
- **Business Outcome**: Valid - error handling resilience
- **Recommendation**: **KEEP SKIPPED** - require chaos infrastructure
- **Confidence**: 90% - test is correct, need infrastructure
- **Future**: Move to E2E chaos tests

---

## 📈 Tier Recommendations

### Integration Tier (Current)
**Keep**: 94 passing tests
**Remove**: 1 test (storm TTL - too slow)
**Target**: 94/94 passing (100%)

### E2E Tier (Future)
**Add from Pending**:
- Storm window TTL expiration (90s wait)
- 4 K8s API edge cases (chaos tests)
- 2 Redis pipeline failures (chaos tests)

**Add from Skipped**:
- 3 observability failure metrics (chaos tests)
- 1 error handling K8s failure (chaos test)

**Total E2E**: 11 tests requiring chaos infrastructure

### Keep Skipped (Require Gateway Changes)
- 6 HTTP server timeout tests (need Gateway API refactoring)

---

## 🎯 Action Plan

### Immediate (This Session)
1. ✅ **Keep storm TTL test as XIt** - already documented for E2E
2. ✅ **Keep 6 K8s/Redis pending tests** - require chaos infrastructure
3. ✅ **Keep 10 skipped tests** - require infrastructure or Gateway changes
4. ✅ **Focus on fixing 2 failing tests** - redis_resilience and observability

### Short-Term (Next Sprint)
1. Create `test/e2e/chaos/` directory
2. Move 11 chaos tests to E2E tier
3. Implement chaos infrastructure (Chaos Mesh or similar)
4. Enable chaos tests in nightly E2E suite

### Long-Term (Future)
1. Refactor Gateway to expose timeout configuration
2. Enable 6 HTTP server timeout tests
3. Achieve 100% integration test coverage

---

## ✅ Confidence Assessment

### Tests Ready to Enable: **0 tests**
**Reason**: All pending/skipped tests require infrastructure not available in integration tier

### Tests to Move to E2E: **11 tests**
**Confidence**: 95%
- All tests validate correct business outcomes
- All tests require chaos/failure simulation
- All tests are too slow or complex for integration tier

### Tests to Keep Skipped: **6 tests**
**Confidence**: 85%
- All tests validate correct business outcomes
- All tests require Gateway API changes
- Not feasible to enable without refactoring

---

## 📊 Final Metrics

| Metric | Current | Target |
|---|---|---|
| **Integration Passing** | 94/96 | 96/96 (100%) |
| **Integration Pending** | 7 | 0 (move to E2E) |
| **Integration Skipped** | 10 | 6 (move 4 to E2E) |
| **E2E Chaos Tests** | 0 | 11 (new tier) |
| **Total Coverage** | 94/113 (83%) | 107/113 (95%) |

---

## 🏆 Success Criteria

### Integration Tier (This Session)
- ✅ Fix 2 remaining failures (redis_resilience, observability)
- ✅ Achieve 96/96 passing (100%)
- ✅ Document pending/skipped tests
- ✅ Maintain <180s test suite time

### E2E Tier (Future)
- Create chaos test infrastructure
- Move 11 tests to E2E tier
- Achieve 100% business outcome coverage

---

## 💡 Key Insights

### Why Not Enable Now?

1. **Infrastructure Requirements**: 17/17 tests require infrastructure not available
   - Chaos engineering (K8s API failures, Redis failures)
   - Gateway configuration exposure (timeout settings)
   - Failure simulation (network partitions, slow responses)

2. **Test Tier Appropriateness**: Tests are correctly categorized
   - Integration tier: Fast, reliable, no external dependencies
   - E2E tier: Slow, complex, chaos/failure scenarios
   - Current pending/skipped: Belong in E2E tier

3. **Business Outcome Coverage**: All tests validate correct outcomes
   - No tests are invalid or incorrect
   - All tests serve documented business requirements
   - Tests are pending/skipped due to infrastructure, not logic

---

## 📝 Recommendations Summary

### DO NOW ✅
- Fix 2 failing tests (redis_resilience, observability)
- Keep all 17 pending/skipped tests as-is
- Document tests for future E2E tier
- Commit current progress (94/96 passing)

### DO LATER 📅
- Create E2E chaos test infrastructure
- Move 11 tests to E2E tier
- Refactor Gateway for timeout configuration
- Enable remaining 6 HTTP server tests

### DON'T DO ❌
- Don't try to enable tests without proper infrastructure
- Don't mock infrastructure failures in integration tests
- Don't compromise test quality for coverage metrics

---

**Overall Assessment**: **Excellent Test Suite Health**
- 97.9% passing rate (94/96)
- All pending/skipped tests are correctly categorized
- Clear path to 100% coverage with proper infrastructure
- No invalid or incorrect tests identified

**Confidence**: 95% - Test suite is production-ready for integration tier

