# Gateway Integration Test Gaps - High-Value Scenarios Analysis

**Date**: January 14, 2026
**Context**: Integration coverage at 30.1% (target: ≥50%)
**Goal**: Identify high-value integration test scenarios that complement E2E tests

---

## 🎯 **Analysis Methodology**

### **Criteria for High-Value Integration Tests**:
1. **Business Logic Validation** - Tests core business rules without full infrastructure
2. **Component Interaction** - Tests integration between 2-3 components
3. **Non-Duplicate** - Scenarios NOT already covered in E2E tests
4. **Fast Execution** - Can run in <5 seconds per test
5. **High BR Coverage** - Maps to P0/P1 Business Requirements

### **E2E vs Integration Separation**:
- **E2E Tests**: Full Gateway server + Kind cluster + real HTTP calls
- **Integration Tests**: Direct business logic calls + envtest + mocked external deps

---

## 📊 **Current Coverage Analysis**

### **What E2E Tests Cover** (96 tests):
✅ Prometheus webhook end-to-end flow
✅ Kubernetes Event webhook end-to-end flow
✅ CORS enforcement
✅ Error response codes (400, 404, 405)
✅ Health/Readiness endpoints
✅ Replay attack prevention
✅ Graceful shutdown foundation
✅ K8s API rate limiting
✅ Metrics endpoint exposure
✅ Audit error scenarios
✅ Deduplication edge cases
✅ Error classification & retry
✅ Security headers

### **What Integration Tests Cover** (22 tests):
✅ State-based deduplication (database-level)
✅ Multi-namespace isolation
✅ Concurrent alerts handling
✅ CRD creation lifecycle
✅ Fingerprint stability
✅ K8s API failure handling
✅ Status deduplication tracking
✅ Field selector precision

### **What's MISSING in Integration Tests** ❌:
❌ Adapter business logic (Prometheus, K8s Event parsing)
❌ Metrics emission validation
❌ Audit event emission validation
❌ Middleware chain integration
❌ Configuration validation
❌ Circuit breaker state transitions
❌ Error classification logic
❌ Retry backoff calculations

---

## 🎯 **HIGH-VALUE INTEGRATION TEST SCENARIOS**

### **Category 1: Audit Event Emission** (BR-GATEWAY-054, BR-GATEWAY-055-065)

#### **Scenario 1.1: Signal Received Audit Event**
**BR**: BR-GATEWAY-055
**Business Value**: SOC2 compliance - every signal must be auditable
**Test Focus**: Verify `gateway.signal.received` audit event emission
**Components**: Adapter → Audit Helper → DataStorage Client
**Current Coverage**: 0% (no integration test)
**E2E Coverage**: Partial (Test 22, 23 cover some audit scenarios)
**Why Integration**: Can validate audit payload structure without full HTTP stack

**Test Scenarios**:
1. Prometheus signal → `gateway.signal.received` with correct `original_payload`
2. K8s Event signal → `gateway.signal.received` with event metadata
3. Audit event includes `correlation_id` for tracing
4. Audit event includes `signal_labels` and `signal_annotations`
5. Audit event emission failure doesn't block signal processing

**Estimated Coverage Gain**: +3%

---

#### **Scenario 1.2: CRD Created Audit Event**
**BR**: BR-GATEWAY-056
**Business Value**: Track every CRD creation for compliance and debugging
**Test Focus**: Verify `gateway.crd.created` audit event emission
**Components**: CRD Creator → Audit Helper → DataStorage Client
**Current Coverage**: 0% (no integration test)
**E2E Coverage**: Partial (Test 22 covers fallback scenario)
**Why Integration**: Validate audit payload without K8s cluster overhead

**Test Scenarios**:
1. CRD creation → `gateway.crd.created` with CRD name/namespace
2. Audit event includes `target_resource` for RR reconstruction
3. Audit event includes `fingerprint` for deduplication tracking
4. Audit event includes `occurrence_count` for storm detection
5. Multiple CRDs → multiple audit events with unique `correlation_id`

**Estimated Coverage Gain**: +2%

---

#### **Scenario 1.3: Signal Deduplicated Audit Event**
**BR**: BR-GATEWAY-057
**Business Value**: Track deduplication decisions for SLA reporting
**Test Focus**: Verify `gateway.signal.deduplicated` audit event emission
**Components**: Phase Checker → Audit Helper → DataStorage Client
**Current Coverage**: 0% (no integration test)
**E2E Coverage**: None
**Why Integration**: Validate deduplication audit logic independently

**Test Scenarios**:
1. Duplicate signal → `gateway.signal.deduplicated` with existing RR reference
2. Audit event includes `deduplication_reason` (status-based)
3. Audit event includes `existing_rr_name` for tracking
4. Audit event includes updated `occurrence_count`
5. Deduplication across different phases → different audit payloads

**Estimated Coverage Gain**: +2%

---

#### **Scenario 1.4: CRD Creation Failed Audit Event**
**BR**: BR-GATEWAY-058
**Business Value**: Track failures for operational debugging
**Test Focus**: Verify `gateway.crd.failed` audit event emission
**Components**: CRD Creator → Audit Helper → DataStorage Client
**Current Coverage**: 0% (no integration test)
**E2E Coverage**: None
**Why Integration**: Simulate K8s API failures without full cluster

**Test Scenarios**:
1. K8s API failure → `gateway.crd.failed` with error details
2. Audit event includes `error_type` (transient vs permanent)
3. Audit event includes `retry_count` for retry tracking
4. Circuit breaker open → `gateway.crd.failed` with circuit breaker state
5. Validation failure → `gateway.crd.failed` with validation errors

**Estimated Coverage Gain**: +2%

---

### **Category 2: Metrics Emission** (BR-GATEWAY-066-069, BR-GATEWAY-079)

#### **Scenario 2.1: HTTP Request Metrics**
**BR**: BR-GATEWAY-067
**Business Value**: Operational visibility into Gateway performance
**Test Focus**: Verify `gateway_http_requests_total` metric emission
**Components**: HTTP Metrics Middleware → Prometheus Registry
**Current Coverage**: 10% (middleware has minimal coverage)
**E2E Coverage**: Test 04 validates metrics endpoint, not emission logic
**Why Integration**: Validate metric labels and values without HTTP overhead

**Test Scenarios**:
1. Signal processing → `gateway_http_requests_total{status="201"}` increments
2. Deduplication → `gateway_http_requests_total{status="202"}` increments
3. Error → `gateway_http_requests_total{status="500"}` increments
4. Metrics include `method`, `path`, `status` labels
5. Request duration histogram (`gateway_http_request_duration_seconds`) populated

**Estimated Coverage Gain**: +2%

---

#### **Scenario 2.2: CRD Creation Metrics**
**BR**: BR-GATEWAY-068
**Business Value**: Track CRD creation success/failure rates
**Test Focus**: Verify `gateway_crd_creations_total` metric emission
**Components**: CRD Creator → Metrics Package
**Current Coverage**: 0% (no integration test)
**E2E Coverage**: None
**Why Integration**: Validate metric emission logic independently

**Test Scenarios**:
1. Successful CRD creation → `gateway_crd_creations_total{status="success"}` increments
2. Failed CRD creation → `gateway_crd_creations_total{status="failure"}` increments
3. Metrics include `namespace`, `adapter` labels
4. Multiple CRDs → metric counter increments correctly
5. Metric values persist across multiple test iterations

**Estimated Coverage Gain**: +2%

---

#### **Scenario 2.3: Deduplication Metrics**
**BR**: BR-GATEWAY-069
**Business Value**: Track deduplication effectiveness for capacity planning
**Test Focus**: Verify `gateway_deduplications_total` metric emission
**Components**: Phase Checker → Metrics Package
**Current Coverage**: 0% (no integration test)
**E2E Coverage**: None
**Why Integration**: Validate deduplication metric logic

**Test Scenarios**:
1. Duplicate signal → `gateway_deduplications_total` increments
2. Metrics include `reason` label (status-based, fingerprint-based)
3. Metrics include `phase` label (Pending, Processing, Blocked)
4. Deduplication rate calculation (`deduplications / total_signals`)
5. Metric values correlate with `occurrence_count` in CRD status

**Estimated Coverage Gain**: +2%

---

### **Category 3: Adapter Business Logic** (BR-GATEWAY-001, BR-GATEWAY-002, BR-GATEWAY-005)

#### **Scenario 3.1: Prometheus Adapter Signal Parsing**
**BR**: BR-GATEWAY-001, BR-GATEWAY-005
**Business Value**: Validate correct signal extraction from Prometheus payloads
**Test Focus**: Adapter.Parse() business logic
**Components**: Prometheus Adapter → Signal struct
**Current Coverage**: 0% (adapter has 0% integration coverage)
**E2E Coverage**: Test 31, 33 cover end-to-end flow
**Why Integration**: Test parsing logic without HTTP/K8s overhead

**Test Scenarios**:
1. Standard Prometheus alert → Signal with correct labels/annotations
2. Alert with `namespace` label → Signal with correct namespace
3. Alert with `severity` label → Signal with correct severity
4. Alert with `pod` label → Signal with correct target resource
5. Alert with missing optional fields → Signal with safe defaults
6. Alert with custom labels → Signal preserves all labels
7. Alert with long annotation → Signal truncates correctly
8. Multiple alerts in payload → Multiple signals parsed

**Estimated Coverage Gain**: +3%

---

#### **Scenario 3.2: Kubernetes Event Adapter Signal Parsing**
**BR**: BR-GATEWAY-002, BR-GATEWAY-005
**Business Value**: Validate correct signal extraction from K8s Events
**Test Focus**: K8s Event Adapter.Parse() business logic
**Components**: K8s Event Adapter → Signal struct
**Current Coverage**: 0% (adapter has 0% integration coverage)
**E2E Coverage**: Test 08, 33 cover end-to-end flow
**Why Integration**: Test parsing logic without HTTP/K8s overhead

**Test Scenarios**:
1. Warning event → Signal with severity=warning
2. Event with `involvedObject` → Signal with correct target resource
3. Event with `reason` → Signal with correct alert name
4. Event with `message` → Signal with correct description
5. Event with missing namespace → Signal uses event namespace
6. Event with multiple occurrences → Signal with occurrence count
7. Normal event (not Warning) → Filtered out (no signal)

**Estimated Coverage Gain**: +3%

---

### **Category 4: Circuit Breaker State Transitions** (BR-GATEWAY-093)

#### **Scenario 4.1: Circuit Breaker State Machine**
**BR**: BR-GATEWAY-093
**Business Value**: Validate fail-fast behavior for K8s API unavailability
**Test Focus**: Circuit breaker state transitions (Closed → Open → Half-Open → Closed)
**Components**: K8s Client with Circuit Breaker → Metrics
**Current Coverage**: 36.9% (partial integration coverage)
**E2E Coverage**: Test 29 covers some scenarios
**Why Integration**: Validate state machine logic without full cluster

**Test Scenarios**:
1. **Closed → Open**: 5 consecutive failures → circuit opens
2. **Open → Half-Open**: After timeout → circuit allows 1 test request
3. **Half-Open → Closed**: Test request succeeds → circuit closes
4. **Half-Open → Open**: Test request fails → circuit reopens
5. Circuit open → immediate failure (no K8s API call)
6. Circuit state metrics (`gateway_circuit_breaker_state`) update correctly
7. Circuit breaker operations metrics (`gateway_circuit_breaker_operations_total`)

**Estimated Coverage Gain**: +3%

---

### **Category 5: Error Classification & Retry Logic** (BR-GATEWAY-188, BR-GATEWAY-189)

#### **Scenario 5.1: Transient vs Permanent Error Classification**
**BR**: BR-GATEWAY-188, BR-GATEWAY-189
**Business Value**: Validate correct retry behavior for different error types
**Test Focus**: Error classification logic
**Components**: Error Classifier → Retry Logic
**Current Coverage**: 0% (no integration test)
**E2E Coverage**: Test 26 covers end-to-end retry behavior
**Why Integration**: Test classification logic without HTTP overhead

**Test Scenarios**:
1. K8s API 500 error → classified as TRANSIENT → retry
2. K8s API 503 error → classified as TRANSIENT → retry
3. K8s API 400 error → classified as PERMANENT → no retry
4. K8s API 422 error → classified as PERMANENT → no retry
5. Network timeout → classified as TRANSIENT → retry
6. Context canceled → classified as PERMANENT → no retry
7. Validation error → classified as PERMANENT → no retry

**Estimated Coverage Gain**: +2%

---

#### **Scenario 5.2: Exponential Backoff Calculation**
**BR**: BR-GATEWAY-188
**Business Value**: Validate correct backoff timing for retries
**Test Focus**: Backoff calculation logic
**Components**: Retry Logic → Clock
**Current Coverage**: 33.3% (clock has minimal coverage)
**E2E Coverage**: Test 26 validates timing behavior
**Why Integration**: Test backoff math without time delays

**Test Scenarios**:
1. First retry → 100ms backoff
2. Second retry → 200ms backoff (2x)
3. Third retry → 400ms backoff (2x)
4. Max backoff → capped at 5 seconds
5. Jitter applied → backoff varies within range
6. Retry count tracked correctly

**Estimated Coverage Gain**: +1%

---

### **Category 6: Configuration Validation** (BR-GATEWAY-043, BR-GATEWAY-052)

#### **Scenario 6.1: Configuration Loading & Validation**
**BR**: BR-GATEWAY-043
**Business Value**: Validate Gateway starts with correct configuration
**Test Focus**: Config validation logic
**Components**: Config Loader → Validator
**Current Coverage**: 16.7% (config has minimal coverage)
**E2E Coverage**: None (E2E assumes valid config)
**Why Integration**: Test config validation without full server startup

**Test Scenarios**:
1. Valid config → loads successfully
2. Missing required field → validation error
3. Invalid port number → validation error
4. Invalid timeout value → validation error
5. Invalid log level → validation error
6. Config with defaults → defaults applied correctly
7. Environment variable override → config updated

**Estimated Coverage Gain**: +2%

---

### **Category 7: Middleware Chain Integration** (BR-GATEWAY-039, BR-GATEWAY-074-076)

#### **Scenario 7.1: Middleware Chain Execution Order**
**BR**: BR-GATEWAY-039, BR-GATEWAY-074, BR-GATEWAY-075, BR-GATEWAY-076
**Business Value**: Validate middleware executes in correct order
**Test Focus**: Middleware chain integration
**Components**: Request ID → Timestamp → Security Headers → Content Type
**Current Coverage**: ~15% average across middleware
**E2E Coverage**: Test 19, 20 cover some middleware behavior
**Why Integration**: Test middleware chain without full HTTP stack

**Test Scenarios**:
1. Request → Request ID generated first
2. Request → Timestamp validation before processing
3. Request → Security headers added to response
4. Request → Content-Type validated before adapter
5. Middleware chain → all middleware execute in order
6. Middleware failure → request rejected early
7. Middleware metrics → each middleware tracked

**Estimated Coverage Gain**: +3%

---

## 📊 **Coverage Gain Summary**

| Category | Scenarios | Estimated Coverage Gain |
|----------|-----------|-------------------------|
| **1. Audit Event Emission** | 4 scenarios (20 tests) | **+9%** |
| **2. Metrics Emission** | 3 scenarios (15 tests) | **+6%** |
| **3. Adapter Business Logic** | 2 scenarios (15 tests) | **+6%** |
| **4. Circuit Breaker** | 1 scenario (7 tests) | **+3%** |
| **5. Error Classification** | 2 scenarios (13 tests) | **+3%** |
| **6. Configuration** | 1 scenario (7 tests) | **+2%** |
| **7. Middleware Chain** | 1 scenario (7 tests) | **+3%** |
| **TOTAL** | **14 scenarios (84 tests)** | **+32%** |

**Projected Final Coverage**: 30.1% + 32% = **~62%** ✅ **COMPLIANT**

---

## 🎯 **Prioritized Implementation Plan**

### **Phase 1: Quick Wins (Week 1)** - Target: +15% → 45%
**Goal**: Achieve near-compliance with fastest scenarios

1. **Audit Event Emission** (Scenarios 1.1-1.4) - +9%
   - High business value (SOC2 compliance)
   - Fast to implement (direct function calls)
   - Clear BR mapping

2. **Metrics Emission** (Scenarios 2.1-2.3) - +6%
   - High operational value
   - Existing metrics package to leverage
   - Clear validation criteria

**Week 1 Deliverable**: 29 tests, +15% coverage → **45% total**

---

### **Phase 2: Adapter & Error Logic (Week 2)** - Target: +12% → 57%
**Goal**: Achieve full compliance with core business logic

3. **Adapter Business Logic** (Scenarios 3.1-3.2) - +6%
   - Critical signal processing path
   - Currently 0% integration coverage
   - High BR value

4. **Error Classification** (Scenarios 5.1-5.2) - +3%
   - Important for reliability
   - Currently 0% integration coverage
   - Clear test scenarios

5. **Circuit Breaker** (Scenario 4.1) - +3%
   - Improves existing 36.9% coverage
   - High operational value
   - State machine validation

**Week 2 Deliverable**: 35 tests, +12% coverage → **57% total** ✅

---

### **Phase 3: Infrastructure & Polish (Week 3)** - Target: +5% → 62%
**Goal**: Exceed compliance with infrastructure validation

6. **Configuration** (Scenario 6.1) - +2%
   - Startup validation
   - Currently 16.7% coverage
   - Important for operations

7. **Middleware Chain** (Scenario 7.1) - +3%
   - HTTP layer validation
   - Currently ~15% average coverage
   - Integration between components

**Week 3 Deliverable**: 14 tests, +5% coverage → **62% total** ✅

---

## 🎯 **Success Criteria**

### **Coverage Targets**:
- ✅ **Minimum**: 50% integration coverage (compliance)
- ✅ **Target**: 55-60% integration coverage (healthy)
- ✅ **Stretch**: 62%+ integration coverage (excellent)

### **Quality Targets**:
- ✅ All tests map to specific BRs
- ✅ All tests use direct business logic calls (no HTTP)
- ✅ All tests run in <5 seconds
- ✅ All tests validate business outcomes (not implementation)
- ✅ Zero NULL-TESTING anti-patterns

### **Business Value Targets**:
- ✅ SOC2 compliance (audit event validation)
- ✅ Operational visibility (metrics validation)
- ✅ Reliability (error handling, circuit breaker)
- ✅ Correctness (adapter parsing, config validation)

---

## 📋 **Next Steps**

1. **DECISION**: Review and approve prioritized implementation plan
2. **PLANNING**: Create detailed test specifications for Phase 1 scenarios
3. **EXECUTION**: Implement Phase 1 (Week 1) - Audit + Metrics tests
4. **VALIDATION**: Run coverage analysis after Phase 1
5. **ITERATION**: Proceed to Phase 2 if Phase 1 achieves +15% gain

---

## 📚 **References**

- **Business Requirements**: `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md`
- **Current Coverage Report**: `/tmp/gw-integration-coverage.out`
- **E2E Test Scenarios**: `/tmp/gw-e2e-scenarios.txt`
- **Integration Test Scenarios**: `/tmp/gw-integration-scenarios.txt`
- **Testing Standards**: `.cursor/rules/15-testing-coverage-standards.mdc`

---

**Status**: 📋 **READY FOR APPROVAL**
**Priority**: **P0 - COMPLIANCE REQUIREMENT**
**Estimated Effort**: 3 weeks (84 tests)
**Expected Outcome**: 62% integration coverage ✅
