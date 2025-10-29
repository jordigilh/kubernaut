# Tests to Move to Other Testing Tiers

**Date**: October 29, 2025  
**Purpose**: Document tests that belong in E2E, Chaos, or Load tiers (not Integration)  
**Current Status**: 19 pending tests in integration tier  
**Action Required**: Move 14 tests to appropriate tiers

---

## 🎯 **Testing Tier Strategy**

### **Integration Tests** (<20% coverage)
- **Purpose**: Component interactions with real dependencies
- **Scope**: 2-3 components working together
- **Concurrency**: Realistic (5-10 requests)
- **Duration**: <5 minutes total suite
- **Infrastructure**: Local (Kind cluster, local Redis)

### **E2E Tests** (<10% coverage)
- **Purpose**: Complete user workflows, production-like scenarios
- **Scope**: End-to-end scenarios
- **Duration**: 10-30 minutes
- **Infrastructure**: Production-like environment

### **Chaos Tests** (<5% coverage)
- **Purpose**: Failure scenarios, resilience testing
- **Scope**: System resilience under failures
- **Infrastructure**: Chaos engineering tools (toxiproxy, pumba, etc.)

### **Load Tests** (<5% coverage)
- **Purpose**: System under heavy load
- **Scope**: System-wide performance
- **Concurrency**: High (100+ requests)
- **Duration**: 10-30 minutes

---

## 📋 **Tests to Move (14 Total)**

### **🔥 CHAOS TIER (8 tests) - Move to `test/chaos/gateway/`**

#### **1. Redis Connection Failure Gracefully**
**Current Location**: `test/integration/gateway/redis_resilience_test.go:73`  
**Target Location**: `test/chaos/gateway/redis_failure_test.go`  
**Tier**: **CHAOS**

**Rationale**:
- Requires stopping Redis mid-test (chaos infrastructure)
- Tests resilience under infrastructure failure
- Requires chaos tools (podman stop/start, toxiproxy)
- Not a component interaction test

**Use Case**:
```
Scenario: Redis becomes unavailable during production
Given: Gateway is processing alerts
When: Redis connection fails
Then: Gateway returns 503 Service Unavailable
And: Gateway logs error for monitoring
And: Gateway recovers when Redis returns
```

**Implementation Requirements**:
- Chaos infrastructure to stop/start Redis
- Health monitoring to detect Redis state
- Automatic recovery detection

**Business Value**: Validates graceful degradation (DD-GATEWAY-003)  
**Priority**: MEDIUM - Important for production resilience  
**Estimated Effort**: 4-6 hours (chaos infrastructure)

---

#### **2. Redis Recovery After Outage**
**Current Location**: `test/integration/gateway/redis_resilience_test.go:88`  
**Target Location**: `test/chaos/gateway/redis_recovery_test.go`  
**Tier**: **CHAOS**

**Rationale**:
- Tests automatic recovery after chaos event
- Requires stopping and restarting Redis
- Tests resilience behavior, not component interaction

**Use Case**:
```
Scenario: Gateway automatically recovers after Redis outage
Given: Gateway is working normally (201 responses)
When: Redis is stopped (Gateway returns 503)
And: Redis is restarted
Then: Gateway automatically detects recovery
And: Gateway resumes normal operation (201 responses)
```

**Implementation Requirements**:
- Same chaos infrastructure as #1
- Health monitoring with automatic recovery
- State verification after recovery

**Business Value**: Validates automatic recovery without manual intervention  
**Priority**: MEDIUM  
**Estimated Effort**: Covered in #1 (same infrastructure)

---

#### **3. Redis Cluster Failover (redis_integration_test.go)**
**Current Location**: `test/integration/gateway/redis_integration_test.go:305`  
**Target Location**: `test/chaos/gateway/redis_ha_test.go`  
**Tier**: **CHAOS**

**Rationale**:
- Requires Redis HA setup (master/replica/sentinel)
- Tests failover scenario (chaos event)
- Requires infrastructure to trigger failover
- Not a component interaction test

**Use Case**:
```
Scenario: Gateway handles Redis master failover without data loss
Given: Gateway is using Redis master
When: Redis master fails
And: Sentinel promotes replica to master
Then: Gateway experiences temporary 503 errors during failover
And: Gateway automatically reconnects to new master
And: No fingerprint data is lost
```

**Implementation Requirements**:
- Redis Sentinel HA setup (master + replica + sentinel)
- Chaos tool to trigger master failure
- Data persistence verification

**Business Value**: Validates HA resilience for production Redis clusters  
**Priority**: MEDIUM - Important for production HA  
**Estimated Effort**: 8-12 hours (Redis HA setup)

---

#### **4. Redis Cluster Failover (redis_resilience_test.go)**
**Current Location**: `test/integration/gateway/redis_resilience_test.go:181`  
**Target Location**: `test/chaos/gateway/redis_ha_test.go` (same as #3)  
**Tier**: **CHAOS**

**Rationale**: Same as #3 (duplicate test)

**Use Case**: Same as #3

**Implementation Requirements**: Same as #3

**Business Value**: Same as #3  
**Priority**: MEDIUM  
**Estimated Effort**: Covered in #3 (same infrastructure)

---

#### **5. Redis Pipeline Failures**
**Current Location**: `test/integration/gateway/redis_resilience_test.go:198`  
**Target Location**: `test/chaos/gateway/redis_pipeline_failure_test.go`  
**Tier**: **CHAOS**

**Rationale**:
- Requires network failure injection mid-pipeline
- Tests partial failure scenarios (chaos)
- Requires chaos tools (toxiproxy, iptables)
- Not a component interaction test

**Use Case**:
```
Scenario: Gateway handles Redis pipeline failures without state corruption
Given: Gateway is processing batch of 20 alerts
When: Redis pipeline fails mid-batch (network issue)
And: Gateway continues processing next 20 alerts
Then: State remains consistent (no duplicate fingerprints)
And: Correct counts maintained
And: No orphaned Redis keys
```

**Implementation Requirements**:
- Network failure injection (toxiproxy, tc, iptables)
- Pipeline failure simulation
- State consistency verification

**Business Value**: Validates state consistency under partial failures  
**Priority**: LOW - Edge case, rare in production  
**Estimated Effort**: 6-8 hours (failure injection framework)

---

#### **6. K8s API Unavailable During Webhook**
**Current Location**: `test/integration/gateway/k8s_api_failure_test.go:236`  
**Target Location**: `test/chaos/gateway/k8s_api_failure_test.go`  
**Tier**: **CHAOS**

**Rationale**:
- Requires K8s API failure simulation (chaos)
- Tests resilience under infrastructure failure
- Requires chaos tools or network partition
- Not a component interaction test

**Use Case**:
```
Scenario: Gateway returns 500 when K8s API is unavailable
Given: Gateway receives Prometheus webhook
When: K8s API is unavailable (network partition, API server down)
Then: Gateway returns 500 Internal Server Error
And: Prometheus retries webhook (exponential backoff)
And: Gateway logs error for monitoring
```

**Implementation Requirements**:
- K8s API failure simulation (network partition, API server stop)
- ErrorInjectableK8sClient with failure modes
- Retry behavior verification

**Business Value**: Validates correct error handling for K8s API failures  
**Priority**: MEDIUM - Important for production resilience  
**Estimated Effort**: 4-6 hours (K8s chaos infrastructure)

---

#### **7. K8s API Recovery**
**Current Location**: `test/integration/gateway/k8s_api_failure_test.go:252`  
**Target Location**: `test/chaos/gateway/k8s_api_recovery_test.go`  
**Tier**: **CHAOS**

**Rationale**:
- Tests automatic recovery after chaos event
- Requires K8s API failure and recovery simulation
- Tests resilience behavior, not component interaction

**Use Case**:
```
Scenario: Gateway automatically recovers when K8s API returns
Given: Gateway is returning 500 (K8s API unavailable)
When: K8s API becomes available again
Then: Gateway automatically detects recovery
And: Gateway resumes normal operation (201 responses)
And: Pending webhooks are processed successfully
```

**Implementation Requirements**:
- Same K8s chaos infrastructure as #6
- Automatic recovery detection
- State verification after recovery

**Business Value**: Validates automatic recovery without manual intervention  
**Priority**: MEDIUM  
**Estimated Effort**: Covered in #6 (same infrastructure)

---

#### **8. K8s API Slow Responses**
**Current Location**: `test/integration/gateway/k8s_api_integration_test.go:378`  
**Target Location**: `test/chaos/gateway/k8s_api_latency_test.go`  
**Tier**: **CHAOS**

**Rationale**:
- Requires latency injection (chaos tool)
- Tests timeout behavior under slow responses
- Requires toxiproxy or similar latency injection
- Not a component interaction test

**Use Case**:
```
Scenario: Gateway handles slow K8s API responses without timeout
Given: Gateway receives Prometheus webhook
When: K8s API responds slowly (5-10 second latency)
Then: Gateway waits for response (no timeout)
And: CRD is created successfully
And: Gateway returns 201 Created (eventually)
```

**Implementation Requirements**:
- Latency injection tool (toxiproxy, tc)
- Timeout configuration verification
- Response time monitoring

**Business Value**: Validates timeout configuration for slow K8s API  
**Priority**: LOW - Edge case  
**Estimated Effort**: 3-4 hours (latency injection)

---

### **🌐 E2E TIER (5 tests) - Move to `test/e2e/gateway/`**

#### **9. Storm Window TTL Expiration**
**Current Location**: `test/integration/gateway/storm_aggregation_test.go:440`  
**Target Location**: `test/e2e/gateway/storm_ttl_expiration_test.go`  
**Tier**: **E2E**

**Rationale**:
- Test takes 2+ minutes (waits for TTL expiration)
- Tests complete workflow over time
- Too slow for integration tier (<5 min total suite)
- Better suited for nightly E2E suite

**Use Case**:
```
Scenario: New storm window created after TTL expiration
Given: Storm window exists for "HighCPUUsage" alerts
When: 2 minutes pass (storm window TTL expires)
And: New "HighCPUUsage" alert arrives
Then: New storm window is created
And: New CRD is created (not aggregated into expired window)
```

**Implementation Requirements**:
- None (test is complete)
- Just needs to be moved to E2E tier

**Business Value**: Validates storm window lifecycle and TTL behavior  
**Priority**: LOW - Long-running test, better for nightly E2E  
**Estimated Effort**: 0 hours (just move test)

---

#### **10. K8s API Rate Limiting**
**Current Location**: `test/integration/gateway/k8s_api_integration_test.go:171`  
**Target Location**: `test/e2e/gateway/k8s_api_rate_limit_test.go`  
**Tier**: **E2E** or **CHAOS**

**Rationale**:
- Requires rate limiting simulation (429 responses)
- Tests production-like scenario (K8s API rate limits)
- Requires infrastructure to simulate rate limiting
- Could be E2E or Chaos depending on implementation

**Use Case**:
```
Scenario: Gateway handles K8s API rate limiting gracefully
Given: Gateway is processing high volume of alerts
When: K8s API returns 429 Too Many Requests
Then: Gateway backs off exponentially
And: Gateway retries after backoff period
And: CRDs are eventually created successfully
```

**Implementation Requirements**:
- Rate limiting simulation (mock K8s API or proxy)
- Exponential backoff verification
- Retry behavior verification

**Business Value**: Validates rate limiting handling for high-volume scenarios  
**Priority**: LOW - Edge case, K8s API rarely rate limits  
**Estimated Effort**: 3-4 hours (rate limiting simulation)

---

#### **11. CRD Name Length Limit (253 chars)**
**Current Location**: `test/integration/gateway/k8s_api_integration_test.go:322`  
**Target Location**: `test/e2e/gateway/crd_edge_cases_test.go`  
**Tier**: **E2E** or **INTEGRATION** (borderline)

**Rationale**:
- Tests edge case (very long CRD names)
- Could stay in integration if test is fast
- Better in E2E for edge case coverage

**Use Case**:
```
Scenario: Gateway handles very long CRD names correctly
Given: Alert has very long namespace + alertname (>253 chars)
When: Gateway generates CRD name
Then: CRD name is truncated to 253 chars
And: CRD is created successfully
And: Fingerprint is preserved in labels/annotations
```

**Implementation Requirements**:
- None (simple test)
- Just needs verification

**Business Value**: Validates K8s name length limit handling  
**Priority**: LOW - Edge case, unlikely in practice  
**Estimated Effort**: 30 minutes (verification)

**Recommendation**: ⚠️ **Could stay in integration** if test is fast (<1s)

---

#### **12. Concurrent CRD Creates**
**Current Location**: `test/integration/gateway/k8s_api_integration_test.go:407`  
**Target Location**: `test/e2e/gateway/concurrent_operations_test.go` or **LOAD**  
**Tier**: **E2E** or **LOAD** (depends on concurrency level)

**Rationale**:
- Tests concurrent operations
- If <10 concurrent: Integration or E2E
- If 50+ concurrent: Load tier
- Need to check actual concurrency level

**Use Case**:
```
Scenario: Gateway handles concurrent CRD creates to same namespace
Given: Multiple alerts arrive simultaneously
When: Gateway creates CRDs concurrently
Then: All CRDs are created successfully
And: No conflicts or race conditions
And: All CRDs have unique names
```

**Implementation Requirements**:
- Goroutines for concurrent requests
- Race condition detection
- Conflict verification

**Business Value**: Validates concurrent operation handling  
**Priority**: MEDIUM - Important for production load  
**Estimated Effort**: 1-2 hours (verification)

**Recommendation**: ⚠️ **Check concurrency level** - if <10: E2E, if 50+: LOAD

---

#### **13. Redis State Cleanup on CRD Deletion**
**Current Location**: `test/integration/gateway/redis_resilience_test.go:165`  
**Target Location**: `test/e2e/gateway/crd_lifecycle_test.go`  
**Tier**: **E2E** (or **DEFERRED**)

**Rationale**:
- Requires CRD controller integration (finalizers)
- Tests complete CRD lifecycle (create → delete → cleanup)
- Out of scope for Gateway v1.0
- Better suited for E2E when CRD controller is implemented

**Use Case**:
```
Scenario: Redis state is cleaned up when CRD is deleted
Given: Alert creates CRD with fingerprint in Redis
When: CRD is deleted (kubectl delete or TTL expiration)
Then: Fingerprint is removed from Redis
And: Storm state is removed from Redis
And: No orphaned Redis keys remain
```

**Implementation Requirements**:
- CRD controller with finalizers
- Redis cleanup logic on CRD deletion
- Lifecycle management

**Business Value**: Prevents Redis memory leaks from orphaned keys  
**Priority**: LOW - Out of scope for Gateway v1.0  
**Estimated Effort**: 8-12 hours (controller integration)

**Recommendation**: ❌ **DEFER** to future version (out of scope for Gateway v1.0)

---

### **⚡ LOAD TIER (1 test) - Move to `test/load/gateway/`**

#### **14. High Concurrency Storm Detection**
**Current Location**: N/A (not yet created, but mentioned in pending tests)  
**Target Location**: `test/load/gateway/storm_high_concurrency_test.go`  
**Tier**: **LOAD**

**Rationale**:
- Tests storm detection under high load (100+ concurrent requests)
- Tests system-wide performance
- Requires dedicated test environment
- Not a component interaction test

**Use Case**:
```
Scenario: Storm detection works correctly under high concurrency
Given: 100 concurrent alerts arrive simultaneously
When: Storm detection threshold is 10 alerts
Then: Storm CRD is created
And: Subsequent alerts are aggregated
And: No race conditions occur
And: Storm counters are accurate
```

**Implementation Requirements**:
- High concurrency test framework
- Performance monitoring
- Resource usage tracking

**Business Value**: Validates storm detection under production load  
**Priority**: MEDIUM - Important for production scale  
**Estimated Effort**: 4-6 hours (load testing framework)

---

## 📊 **Summary by Tier**

### **Tests to Move**:
| Tier | Count | Tests |
|---|---|---|
| **CHAOS** | 8 | Redis failures (5), K8s API failures (3) |
| **E2E** | 5 | Storm TTL, Rate limiting, CRD edge cases, Concurrent ops, CRD lifecycle |
| **LOAD** | 1 | High concurrency storm detection |
| **TOTAL** | **14** | |

### **Tests to Keep in Integration** (5 tests):
1. Storm Detection - Aggregates Alerts (business logic fix needed)
2. Storm Aggregation - Concurrent HTTP Status (business logic fix needed)
3. Storm Aggregation - Mixed Alerts HTTP Status (business logic fix needed)
4. Storm State Persistence in Redis (business logic fix needed)
5. ⚠️ CRD Name Length Limit (borderline - could move to E2E)

---

## 🎯 **Implementation Roadmap**

### **Phase 1: Move Tests (2-4 hours)**
1. Create new test directories:
   - `test/chaos/gateway/`
   - `test/e2e/gateway/`
   - `test/load/gateway/`

2. Move tests to appropriate tiers (14 tests)

3. Update test documentation

4. Remove PIt markers from moved tests

### **Phase 2: Build Infrastructure (25-36 hours)**
1. **Chaos Infrastructure** (20-30 hours):
   - Redis chaos tools (stop/start, failover)
   - K8s API chaos tools (failure injection, latency)
   - Network failure injection (toxiproxy, tc)

2. **E2E Infrastructure** (5-6 hours):
   - Long-running test support
   - Production-like environment setup
   - Nightly test suite configuration

3. **Load Infrastructure** (0 hours):
   - Already exists in `test/load/`

### **Phase 3: Enable Tests (10-15 hours)**
1. Enable chaos tests after infrastructure is built
2. Enable E2E tests in nightly suite
3. Enable load tests in performance suite

---

## 📋 **Action Items**

### **Immediate (Next Session)**:
1. ✅ Create tier directories
2. ✅ Move 14 tests to appropriate tiers
3. ✅ Update documentation
4. ✅ Remove from integration tier
5. ✅ Update PENDING_TESTS_TRIAGE.md

### **Short-Term (This Week)**:
1. Fix 4 business logic issues in integration tier
2. Verify 2 borderline tests (CRD name length, concurrent creates)
3. Achieve 100% pass rate for integration tier (5 tests remaining)

### **Medium-Term (This Sprint)**:
1. Build chaos testing infrastructure
2. Enable chaos tests
3. Build E2E infrastructure
4. Enable E2E tests in nightly suite

---

## 🎊 **Expected Outcomes**

### **After Moving Tests**:
- **Integration Tier**: 50 passing, 5 pending (business logic fixes)
- **Chaos Tier**: 8 tests ready to enable (after infrastructure)
- **E2E Tier**: 5 tests ready to enable (after infrastructure)
- **Load Tier**: 1 test ready to enable

### **After Fixing Business Logic**:
- **Integration Tier**: 55 passing, 0 pending (100% pass rate)
- **Total Active Tests**: 55 integration + 8 chaos + 5 E2E + 1 load = **69 tests**

### **After Building Infrastructure**:
- **All Tiers**: 69 passing tests across all tiers
- **Coverage**: Integration (20%), E2E (10%), Chaos (5%), Load (5%)

---

**Generated**: October 29, 2025  
**Status**: ✅ **READY TO MOVE TESTS**  
**Confidence**: **90%** - Clear tier assignments with detailed justification

