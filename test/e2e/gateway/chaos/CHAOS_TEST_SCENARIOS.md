# Gateway Chaos Testing Scenarios

**Test Tier**: Chaos/E2E Testing
**Purpose**: Validate Gateway resilience under infrastructure failures
**Status**: ⏳ **PENDING IMPLEMENTATION** (Infrastructure Not Yet Built)
**Created**: 2025-10-27

---

## 🎯 **Purpose**

Chaos tests validate that the Gateway service remains resilient and maintains data consistency when infrastructure components fail unexpectedly. These tests simulate real-world failure scenarios that cannot be easily replicated in integration tests.

---

## 🚨 **Why Chaos Testing?**

### **Production Risks Mitigated**

1. **Network Failures**: Transient network issues during Redis/K8s operations
2. **Partial Failures**: Mid-operation failures that could corrupt state
3. **Cascading Failures**: One component failure triggering others
4. **Resource Exhaustion**: Infrastructure running out of resources under load
5. **Timing Issues**: Race conditions exposed only under failure scenarios

### **Business Value**

- ✅ **Prevent Data Loss**: Ensure no CRDs are lost during infrastructure failures
- ✅ **Maintain Consistency**: Redis and K8s state remain consistent despite failures
- ✅ **Graceful Degradation**: System fails safely without corrupting data
- ✅ **Fast Recovery**: System recovers quickly after infrastructure is restored

---

## 📋 **Chaos Test Scenarios**

### **Scenario 1: Redis Pipeline Command Failures** ⏳ **PENDING**

**Status**: Moved from integration tier (2025-10-27)
**Original Location**: `test/integration/gateway/redis_integration_test.go:307`
**New Location**: `test/e2e/gateway/chaos/redis_failure_test.go`

#### **Business Requirement**

~~BR-GATEWAY-008: Redis operations~~ ❌ **OBSOLETE** (Redis removed from Gateway December 13, 2025)
- **Original**: BR-GATEWAY-103 (Redis Retry Logic)
- **Status**: Redis deprecated per DD-GATEWAY-011, storm detection removed per DD-GATEWAY-015

#### **Failure Scenario**

~~Redis pipeline commands fail mid-batch~~ **SCENARIO OBSOLETE** - Gateway no longer uses Redis.

#### **Test Steps**

1. **Setup**: Start Gateway with Redis connection
2. **Act 1**: Send 20 alerts (batch 1) → All succeed
3. **Inject Failure**: Simulate Redis pipeline failure (network partition, connection drop)
4. **Act 2**: Send 20 more alerts (batch 2) → Some fail
5. **Verify**: State remains consistent (no partial writes, no corruption)

#### **Expected Outcomes**

- ✅ **Batch 1**: All 20 alerts processed successfully
- ✅ **Batch 2**: Requests fail gracefully (503 Service Unavailable)
- ✅ **State Consistency**: Redis fingerprint count matches K8s CRD count
- ✅ **No Corruption**: No partial writes or orphaned data

#### **Production Risk**

**High** - Network issues during batch operations are common in distributed systems

#### **Implementation Requirements**

1. **Failure Injection**: Mechanism to simulate Redis pipeline failures
2. **Network Chaos**: Tools to inject network partitions (e.g., Toxiproxy, Chaos Mesh)
3. **State Verification**: Comprehensive checks for data consistency
4. **Recovery Testing**: Verify system recovers after failure is resolved

#### **Estimated Effort**

2-3 hours (includes chaos infrastructure setup)

---

### **Scenario 2: Redis Connection Failure During Processing** ❌ **OBSOLETE**

**Status**: ❌ **OBSOLETE** (Redis removed December 13, 2025)
**Original Location**: `test/e2e/gateway/chaos/redis_failure_test.go`

#### **Business Requirement**

~~BR-GATEWAY-008: Gateway must fail fast when Redis is unavailable~~ **REMOVED**
- **Replacement**: BR-GATEWAY-093 (Circuit Breaker for K8s API)
- **Status**: Redis deprecated per DD-GATEWAY-011

#### **Failure Scenario**

~~Redis connection drops~~ **SCENARIO OBSOLETE** - Gateway no longer uses Redis.

#### **Replacement Scenario Recommended**

**Scenario 2B: Kubernetes API Failure During CRD Creation**
- **Business Requirement**: BR-GATEWAY-093 (Circuit Breaker for K8s API)
- **Failure**: K8s API unavailable during RemediationRequest CRD creation
- **Expected**: Circuit breaker opens, Gateway returns 503, prevents cascade failures

#### **Test Steps**

1. **Setup**: Start Gateway with Redis connection
2. **Act 1**: Send alert → Processing starts
3. **Inject Failure**: Drop Redis connection mid-processing
4. **Act 2**: Complete request processing
5. **Verify**: Request fails gracefully (503), no partial state

#### **Expected Outcomes**

- ✅ **Request Failure**: Returns 503 Service Unavailable
- ✅ **No Partial State**: No CRD created if Redis write fails
- ✅ **Fast Failure**: Request fails within timeout (5 seconds)
- ✅ **Error Logging**: Failure logged with context

#### **Production Risk**

**Medium** - Redis connection drops are less common but high impact

#### **Implementation Requirements**

1. **Connection Chaos**: Mechanism to drop Redis connections mid-request
2. **Timing Control**: Inject failure at specific points in request lifecycle
3. **Atomicity Verification**: Ensure no partial state on failure

#### **Estimated Effort**

1-2 hours

---

### **Scenario 3: K8s API Failure During CRD Creation** ⏳ **PENDING**

**Status**: New scenario (identified 2025-10-27)
**Location**: `test/e2e/gateway/chaos/k8s_failure_test.go`

#### **Business Requirement**

BR-GATEWAY-093: Circuit Breaker for K8s API - Gateway must handle K8s API failures gracefully

#### **Failure Scenario**

K8s API becomes unavailable while Gateway is creating a RemediationRequest CRD.

#### **Test Steps**

1. **Setup**: Start Gateway with K8s API connection
2. **Act 1**: Send alert → CRD creation starts
3. **Inject Failure**: Make K8s API unavailable (network partition, API server down)
4. **Act 2**: Complete CRD creation attempt
5. **Verify**: Request fails gracefully, Redis state cleaned up

#### **Expected Outcomes**

- ✅ **Request Failure**: Returns 503 Service Unavailable
- ✅ **Redis Cleanup**: Fingerprint removed from Redis (rollback)
- ✅ **Retry Logic**: Client can retry request after K8s API recovers
- ✅ **No Orphaned Data**: No fingerprints without corresponding CRDs

#### **Production Risk**

**Medium** - K8s API throttling/unavailability happens during cluster upgrades

#### **Implementation Requirements**

1. **K8s API Chaos**: Mechanism to make K8s API unavailable
2. **Rollback Testing**: Verify Redis state is cleaned up on K8s failure
3. **Retry Testing**: Verify requests can be retried after recovery

#### **Estimated Effort**

2-3 hours

---

### **Scenario 4: Cascading Failures (Redis + K8s API)** ⏳ **PENDING**

**Status**: New scenario (identified 2025-10-27)
**Location**: `test/e2e/gateway/chaos/cascading_failure_test.go`

#### **Business Requirement**

BR-GATEWAY-093: Circuit Breaker for K8s API - Gateway must handle multiple simultaneous failures

#### **Failure Scenario**

Both Redis and K8s API become unavailable simultaneously.

#### **Test Steps**

1. **Setup**: Start Gateway with Redis and K8s API connections
2. **Act 1**: Send alert → Processing starts
3. **Inject Failure**: Make both Redis and K8s API unavailable
4. **Act 2**: Complete request processing
5. **Verify**: Request fails gracefully, no partial state

#### **Expected Outcomes**

- ✅ **Request Failure**: Returns 503 Service Unavailable
- ✅ **Fast Failure**: Fails within timeout (5 seconds)
- ✅ **No Partial State**: No writes to either Redis or K8s
- ✅ **Recovery**: System recovers when dependencies are restored

#### **Production Risk**

**Low** - Simultaneous failures are rare but catastrophic

#### **Implementation Requirements**

1. **Multi-Component Chaos**: Inject failures in multiple components simultaneously
2. **Dependency Ordering**: Test different failure orders (Redis first, K8s first, simultaneous)
3. **Recovery Testing**: Verify system recovers when dependencies are restored

#### **Estimated Effort**

2-3 hours

---

### **Scenario 5: Network Latency Injection** ⏳ **PENDING**

**Status**: New scenario (identified 2025-10-27)
**Location**: `test/e2e/gateway/chaos/latency_test.go`

#### **Business Requirement**

BR-GATEWAY-011: Gateway must handle slow dependencies gracefully

#### **Failure Scenario**

Redis or K8s API responses are delayed significantly (e.g., 10+ seconds).

#### **Test Steps**

1. **Setup**: Start Gateway with Redis and K8s API connections
2. **Act 1**: Send alert → Processing starts
3. **Inject Latency**: Add 10-second delay to Redis/K8s API responses
4. **Act 2**: Complete request processing
5. **Verify**: Request times out gracefully, no hanging requests

#### **Expected Outcomes**

- ✅ **Timeout Handling**: Request times out within configured limit (15 seconds)
- ✅ **No Hanging Requests**: Request doesn't hang indefinitely
- ✅ **Error Response**: Returns 504 Gateway Timeout
- ✅ **Resource Cleanup**: No goroutine leaks or connection leaks

#### **Production Risk**

**Medium** - Network latency spikes happen during infrastructure issues

#### **Implementation Requirements**

1. **Latency Injection**: Mechanism to add delays to Redis/K8s API calls
2. **Timeout Testing**: Verify timeouts are enforced correctly
3. **Resource Monitoring**: Verify no leaks during timeout scenarios

#### **Estimated Effort**

1-2 hours

---

### **Scenario 6: Redis Memory Exhaustion (OOM)** ⏳ **PENDING**

**Status**: New scenario (identified 2025-10-27)
**Location**: `test/e2e/gateway/chaos/redis_oom_test.go`

#### **Business Requirement**

BR-GATEWAY-008: Gateway must handle Redis OOM errors gracefully

#### **Failure Scenario**

Redis runs out of memory and starts rejecting commands with OOM errors.

#### **Test Steps**

1. **Setup**: Start Gateway with Redis (limited memory)
2. **Act 1**: Send alerts until Redis memory is full
3. **Inject Failure**: Redis returns "OOM command not allowed"
4. **Act 2**: Send more alerts
5. **Verify**: Requests fail gracefully, no data corruption

#### **Expected Outcomes**

- ✅ **Request Failure**: Returns 503 Service Unavailable
- ✅ **Error Logging**: OOM error logged with context
- ✅ **No Corruption**: Existing data remains consistent
- ✅ **Recovery**: System recovers when Redis memory is freed

#### **Production Risk**

**Medium** - Redis OOM happens during alert storms or memory leaks

#### **Implementation Requirements**

1. **Memory Limit**: Configure Redis with limited memory
2. **OOM Injection**: Fill Redis memory to trigger OOM errors
3. **Recovery Testing**: Verify system recovers after memory is freed

#### **Estimated Effort**

1-2 hours

---

## 🛠️ **Chaos Engineering Infrastructure**

### **Required Tools**

#### **Option 1: Toxiproxy** (Recommended for v1.0)

**Pros**:
- ✅ Simple to set up and use
- ✅ Supports network failures, latency, timeouts
- ✅ HTTP API for programmatic control
- ✅ Works with any TCP service (Redis, K8s API)

**Cons**:
- ⚠️ Requires proxy setup (adds complexity)
- ⚠️ Limited to network-level failures

**Estimated Setup Time**: 2-3 hours

---

#### **Option 2: Chaos Mesh** (Recommended for v2.0+)

**Pros**:
- ✅ Kubernetes-native chaos engineering
- ✅ Supports pod failures, network chaos, stress testing
- ✅ Declarative YAML configuration
- ✅ Built-in observability and monitoring

**Cons**:
- ⚠️ Requires Kubernetes cluster
- ⚠️ More complex setup
- ⚠️ Heavier weight

**Estimated Setup Time**: 4-6 hours

---

#### **Option 3: Manual Failure Injection** (Quick Start)

**Pros**:
- ✅ No additional tools required
- ✅ Simple to implement
- ✅ Good for initial testing

**Cons**:
- ⚠️ Limited failure scenarios
- ⚠️ Not production-realistic
- ⚠️ Manual test execution

**Estimated Setup Time**: 1-2 hours

---

### **Recommended Approach**

**Phase 1 (v1.0)**: Manual failure injection for basic scenarios
**Phase 2 (v1.5)**: Toxiproxy for network-level failures
**Phase 3 (v2.0)**: Chaos Mesh for comprehensive chaos testing

---

## 📊 **Test Coverage**

| Scenario | Priority | Effort | Status |
|----------|----------|--------|--------|
| **Redis Pipeline Failures** | High | 2-3h | ⏳ Pending |
| **Redis Connection Failure** | High | 1-2h | ⏳ Pending |
| **K8s API Failure** | Medium | 2-3h | ⏳ Pending |
| **Cascading Failures** | Low | 2-3h | ⏳ Pending |
| **Network Latency** | Medium | 1-2h | ⏳ Pending |
| **Redis OOM** | Medium | 1-2h | ⏳ Pending |
| **Total** | - | 10-16h | ⏳ Pending |

---

## 🎯 **Success Criteria**

### **Functional**

- ✅ All chaos scenarios pass (100% pass rate)
- ✅ No data loss during failures
- ✅ State consistency maintained (Redis ↔ K8s)
- ✅ Graceful degradation (503 errors, not panics)

### **Non-Functional**

- ✅ Fast failure (<5 seconds timeout)
- ✅ No resource leaks (goroutines, connections)
- ✅ Comprehensive error logging
- ✅ Recovery within 30 seconds after failure resolved

---

## 📝 **Implementation Plan**

### **Phase 1: Infrastructure Setup** (4-6 hours)

1. Choose chaos engineering tool (Toxiproxy recommended)
2. Set up chaos testing environment
3. Create failure injection mechanisms
4. Implement state verification helpers

### **Phase 2: Scenario Implementation** (10-16 hours)

1. Implement Redis pipeline failures test (2-3h)
2. Implement Redis connection failure test (1-2h)
3. Implement K8s API failure test (2-3h)
4. Implement cascading failures test (2-3h)
5. Implement network latency test (1-2h)
6. Implement Redis OOM test (1-2h)

### **Phase 3: Documentation & CI/CD** (2-3 hours)

1. Document chaos test execution
2. Create chaos test scripts
3. Integrate with CI/CD pipeline (optional)

**Total Estimated Effort**: 16-25 hours

---

## 🔗 **Related Documentation**

- **Integration Tests**: `test/integration/gateway/README.md`
- **Load Tests**: `test/load/gateway/README.md`
- **Test Tier Classification**: `test/integration/gateway/TEST_TIER_CLASSIFICATION_ASSESSMENT.md`
- **Testing Strategy**: `.cursor/rules/03-testing-strategy.mdc`

---

## 📋 **Next Steps**

### **When Ready to Implement**

1. **Review this document** and prioritize scenarios
2. **Choose chaos engineering tool** (Toxiproxy recommended)
3. **Set up chaos testing environment**
4. **Implement high-priority scenarios** (Redis pipeline failures, Redis connection failure)
5. **Integrate with CI/CD pipeline**

### **Prerequisites**

- ✅ Integration tests passing (100% pass rate)
- ✅ Load tests implemented and passing
- ✅ Dedicated chaos testing environment available
- ✅ Monitoring and observability in place

---

**Status**: ⏳ **PENDING IMPLEMENTATION**
**Priority**: **Medium** (after integration and load tests are stable)
**Next Review**: When ready to start chaos testing implementation


