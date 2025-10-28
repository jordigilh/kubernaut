# Gateway Load Tests

**Test Tier**: Load Testing
**Purpose**: Validate Gateway behavior under high concurrent load
**Execution Environment**: Dedicated load testing environment

---

## 🎯 **Purpose**

Load tests validate that the Gateway service can handle high concurrent request volumes without data loss, performance degradation, or resource leaks. These tests focus on system limits and performance characteristics rather than business logic.

---

## 📊 **Test Categories**

### **1. Concurrent Processing Tests** (11 tests)

**File**: `concurrent_load_test.go`

**Coverage**:
- 100+ concurrent unique alerts
- 100+ concurrent duplicate alerts
- 50+ concurrent storm detection alerts
- Mixed concurrent operations
- State consistency under load
- Multi-namespace concurrent requests
- Race condition handling
- Varying payload sizes
- Context cancellation
- Goroutine leak detection
- Burst traffic patterns

**Business Requirements**:
- BR-GATEWAY-003 (Deduplication under load)
- BR-GATEWAY-007 (Storm detection under load)

---

### **2. Redis Operations Tests** (1 test)

**File**: `redis_load_test.go`

**Coverage**:
- 200+ concurrent Redis operations
- Connection pool exhaustion
- Connection pool recovery
- State consistency under pool exhaustion

**Business Requirements**:
- BR-GATEWAY-008 (Redis connection pool management)

---

## 🚀 **Running Load Tests**

### **Prerequisites**

1. **Dedicated Test Environment**: Load tests should run in a dedicated environment to avoid impacting other services.
2. **Resource Monitoring**: Set up monitoring for CPU, memory, network, and Redis metrics.
3. **Baseline Metrics**: Establish baseline performance metrics before running load tests.

### **Execution**

```bash
# Run all load tests
cd test/load/gateway
go test -v . -timeout 30m

# Run specific test
go test -v . -run "TestGatewayLoadTests/should_handle_100_concurrent_unique_alerts"

# Run with Ginkgo
ginkgo -v . --timeout 30m
```

### **Expected Execution Time**

- **Full Suite**: 20-30 minutes
- **Individual Test**: 1-5 minutes

---

## 📈 **Performance Metrics**

### **Success Criteria**

| Metric | Target | Acceptable | Critical |
|--------|--------|------------|----------|
| **Throughput** | >100 req/s | >50 req/s | <50 req/s |
| **Latency (p50)** | <100ms | <200ms | >200ms |
| **Latency (p95)** | <500ms | <1s | >1s |
| **Latency (p99)** | <1s | <2s | >2s |
| **Error Rate** | 0% | <1% | >1% |
| **Memory Growth** | <10% | <20% | >20% |
| **Goroutine Leaks** | 0 | <5 | >5 |

### **Resource Limits**

| Resource | Limit | Rationale |
|----------|-------|-----------|
| **Gateway CPU** | 2 cores | Realistic production allocation |
| **Gateway Memory** | 2GB | Realistic production allocation |
| **Redis CPU** | 1 core | Dedicated Redis instance |
| **Redis Memory** | 2GB | Sufficient for load test data |
| **K8s API QPS** | 50 | Prevent API server throttling |

---

## 🔍 **Test Infrastructure**

### **Components**

1. **Gateway Service**: Target service under load
2. **Redis**: Deduplication and storm detection state
3. **Kubernetes API**: CRD creation and management
4. **Load Generator**: Test harness generating concurrent requests

### **Test Helpers** (Shared with Integration Tests)

Load tests reuse integration test helpers from `test/integration/gateway/helpers.go`:

- `SetupRedisTestClient()` - Redis client setup
- `SetupK8sTestClient()` - Kubernetes client setup
- `StartTestGateway()` - Gateway server startup
- `SendWebhook()` - HTTP request helper
- `GeneratePrometheusAlert()` - Test data generation
- `ListRemediationRequests()` - CRD listing
- `CountGoroutines()` - Goroutine leak detection

---

## 🎯 **Test Scenarios**

### **Scenario 1: Sustained Load**

**Objective**: Validate Gateway handles sustained high load without degradation.

**Test**: `should handle 100 concurrent unique alerts`

**Expected Outcome**:
- All 100 alerts processed successfully
- 100 CRDs created
- No errors or timeouts
- Memory usage stable

---

### **Scenario 2: Deduplication Under Load**

**Objective**: Validate deduplication works correctly under concurrent load.

**Test**: `should deduplicate 100 identical concurrent alerts`

**Expected Outcome**:
- Exactly 1 CRD created
- 99 requests deduplicated
- No duplicate CRDs
- Redis state consistent

---

### **Scenario 3: Storm Detection Under Load**

**Objective**: Validate storm detection works with concurrent alerts.

**Test**: `should detect storm with 50 concurrent similar alerts`

**Expected Outcome**:
- Storm detected (>10 alerts in 60s)
- Storm CRD created with aggregation metadata
- Individual alerts aggregated correctly

---

### **Scenario 4: Mixed Operations**

**Objective**: Validate Gateway handles mixed concurrent scenarios.

**Test**: `should handle mixed concurrent operations`

**Expected Outcome**:
- All scenarios processed correctly
- State remains consistent
- No resource leaks

---

### **Scenario 5: Resource Leak Detection**

**Objective**: Validate no goroutine or memory leaks under load.

**Test**: `should prevent goroutine leaks under concurrent load`

**Expected Outcome**:
- Goroutine count returns to baseline
- Memory usage stable
- No resource leaks

---

## 🚨 **Known Issues**

### **Issue 1: Port Exhaustion**

**Symptom**: Tests fail with "cannot assign requested address"

**Root Cause**: Too many concurrent HTTP connections exhaust available ports.

**Mitigation**: Tests use batching (20 requests per batch) with 100ms delays between batches.

---

### **Issue 2: Redis OOM**

**Symptom**: Redis returns "OOM command not allowed"

**Root Cause**: Redis memory limit (2GB) exceeded during load tests.

**Mitigation**: Tests clean Redis state before each test with `FlushDB()`.

---

### **Issue 3: K8s API Throttling**

**Symptom**: Tests fail with "rate limit exceeded"

**Root Cause**: K8s API rate limiting (default: 5 QPS).

**Mitigation**: Configure higher QPS limit for load tests (50 QPS).

---

## 📝 **Test Maintenance**

### **Adding New Load Tests**

1. Follow existing test patterns in `concurrent_load_test.go`
2. Use realistic concurrency levels (100+ requests)
3. Include performance metrics collection
4. Document expected outcomes
5. Add to this README

### **Updating Test Infrastructure**

1. Update shared helpers in `test/integration/gateway/helpers.go`
2. Update test configuration in `suite_test.go`
3. Update documentation in this README

---

## 🔗 **Related Documentation**

- **Integration Tests**: `test/integration/gateway/README.md`
- **E2E Tests**: `test/e2e/gateway/README.md`
- **Test Tier Classification**: `test/integration/gateway/TEST_TIER_CLASSIFICATION_ASSESSMENT.md`
- **Testing Strategy**: `.cursor/rules/03-testing-strategy.mdc`

---

## 📊 **Test Coverage**

| Category | Tests | Status |
|----------|-------|--------|
| **Concurrent Processing** | 11 | ⏳ Pending Implementation |
| **Redis Operations** | 1 | ⏳ Pending Implementation |
| **Total** | 12 | ⏳ Pending Implementation |

**Note**: These tests were moved from the integration tier on 2025-10-27 because they test system limits (100+ concurrent requests) rather than business logic. Integration tests should focus on business scenarios with realistic concurrency (5-10 requests).

---

**Status**: ⏳ **PENDING IMPLEMENTATION**
**Next Step**: Implement load test infrastructure and enable tests
**Estimated Effort**: 4-6 hours

