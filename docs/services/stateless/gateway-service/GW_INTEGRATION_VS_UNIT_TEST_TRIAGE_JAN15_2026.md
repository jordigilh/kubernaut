# Gateway Integration vs Unit Test Triage - Jan 15, 2026

## 🎯 **Objective**

Triage all 77 test scenarios from `GW_INTEGRATION_TEST_PLAN_V1.0.md` to determine:
1. Which tests **MUST stay in integration tier** (require real DataStorage/K8s)
2. Which tests **SHOULD move to unit tier** (pure business logic, no infrastructure)
3. Which tests **already exist in unit tier** (avoid duplication)

---

## 📋 **Triage Criteria**

### **Integration Test Criteria** (Stay in Integration Tier)

A test **MUST** be in integration tier if it requires:
- ✅ **Real DataStorage**: Query audit events, verify JSONB structure
- ✅ **Real K8s API**: Create CRDs, observe actual K8s behavior
- ✅ **Real Metrics Infrastructure**: Validate metrics emission with real Prometheus registry
- ✅ **Infrastructure Interaction**: Test how components behave with real dependencies
- ✅ **End-to-End Flows**: Multi-component interactions (adapter → processor → K8s → audit)

### **Unit Test Criteria** (Move to Unit Tier)

A test **SHOULD** be in unit tier if it:
- ✅ **Pure Business Logic**: No infrastructure dependencies
- ✅ **Calculation/Transformation**: Math, string manipulation, data conversion
- ✅ **Validation Logic**: Input validation, format checks
- ✅ **Mocks Sufficient**: Can be fully tested with mocks
- ✅ **Already Exists**: Duplicates existing unit test coverage

---

## 📊 **Triage Results Summary**

| Category | Total | Keep Integration | Move to Unit | Already in Unit | New Unit Tests Needed |
|----------|-------|------------------|--------------|-----------------|----------------------|
| **AUD** (Audit) | 20 | **20** ✅ | 0 | 0 | 0 |
| **MET** (Metrics) | 15 | **15** ✅ | 0 | 0 | 0 |
| **ADP** (Adapters) | 15 | **5** | 10 | 8 | 2 |
| **ERR** (Error Handling) | 13 | **3** | 10 | 5 | 5 |
| **CFG** (Configuration) | 7 | **2** | 5 | 3 | 2 |
| **MID** (Middleware) | 7 | **2** | 5 | 5 | 0 |
| **TOTAL** | **77** | **47** | **30** | **21** | **9** |

### **Final Counts**

- **Integration Tests**: 47 (down from 77)
- **Unit Tests**: 30 (21 exist, 9 new needed)
- **Coverage**: Integration tests focus on high-value infrastructure validation

---

## 🔍 **Detailed Triage by Category**

### **Category 1: Audit Event Emission (AUD)** - 20 Tests

**Decision**: **ALL 20 STAY IN INTEGRATION** ✅

**Rationale**: Audit tests are the **primary reason** for adding DataStorage infrastructure. They require:
- Real DataStorage to query `audit_events` table
- JSONB structure validation (`event_data.GatewayAuditPayload`)
- Correlation ID filtering for parallel execution
- OpenAPI structure verification

| Test ID | Test Name | Decision | Reason |
|---------|-----------|----------|--------|
| GW-INT-AUD-001 | Prometheus Signal Audit | **KEEP** | Needs DataStorage query |
| GW-INT-AUD-002 | K8s Event Signal Audit | **KEEP** | Needs DataStorage query |
| GW-INT-AUD-003 | Correlation ID Format | **KEEP** | Needs DataStorage validation |
| GW-INT-AUD-004 | Signal Labels Preservation | **KEEP** | Needs JSONB query |
| GW-INT-AUD-005 | Audit Failure Non-Blocking | **KEEP** | Needs DataStorage failure simulation |
| GW-INT-AUD-006 | CRD Created Audit | **KEEP** | Needs DataStorage + K8s |
| GW-INT-AUD-007 | CRD Target Resource | **KEEP** | Needs DataStorage query |
| GW-INT-AUD-008 | CRD Fingerprint | **KEEP** | Needs audit event validation |
| GW-INT-AUD-009 | CRD Occurrence Count | **KEEP** | Needs DataStorage query |
| GW-INT-AUD-010 | CRD Unique Correlation IDs | **KEEP** | Needs parallel execution test |
| GW-INT-AUD-011 | Signal Deduplicated Audit | **KEEP** | Needs DataStorage query |
| GW-INT-AUD-012 | Dedup Existing RR Name | **KEEP** | Needs K8s + DataStorage |
| GW-INT-AUD-013 | Dedup Occurrence Count | **KEEP** | Needs DataStorage query |
| GW-INT-AUD-014 | Dedup Multiple Fingerprints | **KEEP** | Needs DataStorage query |
| GW-INT-AUD-015 | Dedup Phase Rejection | **KEEP** | Needs DataStorage query |
| GW-INT-AUD-016 | CRD Failed K8s API Error | **KEEP** | Needs DataStorage + K8s error |
| GW-INT-AUD-017 | CRD Failed Error Type | **KEEP** | Needs DataStorage query |
| GW-INT-AUD-018 | CRD Failed Retry Events | **KEEP** | Needs multiple audit events |
| GW-INT-AUD-019 | CRD Failed Circuit Breaker | **KEEP** | Needs DataStorage + circuit breaker state |
| GW-INT-AUD-020 | Audit ID Uniqueness | **KEEP** | Needs DataStorage concurrent inserts |

**Business Value**: SOC2 compliance, audit trail integrity (highest priority)

---

### **Category 2: Metrics Emission (MET)** - 15 Tests

**Decision**: **ALL 15 STAY IN INTEGRATION** ✅

**Rationale**: Metrics tests require real Prometheus registry + real infrastructure to validate:
- Counter increments with real operations
- Histogram observations with real duration
- Gauge updates with real state changes
- Correlation with audit events (requires DataStorage)

| Test ID | Test Name | Decision | Reason |
|---------|-----------|----------|--------|
| GW-INT-MET-001 | Signals Received Counter | **KEEP** | Needs real Prometheus registry |
| GW-INT-MET-002 | Signals By Type Counter | **KEEP** | Needs label validation |
| GW-INT-MET-003 | Signals By Severity Counter | **KEEP** | Needs real operations |
| GW-INT-MET-004 | Processing Duration Histogram | **KEEP** | Needs real timing |
| GW-INT-MET-005 | Metric Label Accuracy | **KEEP** | Needs real infrastructure |
| GW-INT-MET-006 | CRDs Created Counter | **KEEP** | Needs K8s + metrics |
| GW-INT-MET-007 | CRDs By Phase Counter | **KEEP** | Needs K8s operations |
| GW-INT-MET-008 | CRDs By Namespace Counter | **KEEP** | Needs real namespaces |
| GW-INT-MET-009 | Creation Duration Histogram | **KEEP** | Needs real K8s timing |
| GW-INT-MET-010 | CRD Metric Cleanup | **KEEP** | Needs lifecycle testing |
| GW-INT-MET-011 | Deduplicated Signals Counter | **KEEP** | Needs deduplication flow |
| GW-INT-MET-012 | Dedup Rate Gauge | **KEEP** | Needs multiple signals |
| GW-INT-MET-013 | Dedup By Reason Counter | **KEEP** | Needs real dedup logic |
| GW-INT-MET-014 | Dedup Savings Counter | **KEEP** | Needs calculation validation |
| GW-INT-MET-015 | Metric Correlation With Audit | **KEEP** | **Requires DataStorage + metrics** |

**Business Value**: Observability, capacity planning, operations (high priority)

---

### **Category 3: Adapters (ADP)** - 15 Tests

**Decision**: **5 KEEP, 10 MOVE TO UNIT**

**Rationale**: Most adapter logic is pure transformation (unit tests). Only tests needing infrastructure interaction stay in integration.

| Test ID | Test Name | Decision | Reason | Unit Test Exists? |
|---------|-----------|----------|--------|------------------|
| GW-INT-ADP-001 | Prometheus Alert Parsing | **MOVE** → Unit | Pure parsing logic | ✅ Yes (`prometheus_adapter_test.go`) |
| GW-INT-ADP-002 | Prometheus Alertname Extraction | **MOVE** → Unit | Field extraction | ✅ Yes (`prometheus_adapter_test.go`) |
| GW-INT-ADP-003 | Prometheus Namespace Extraction | **MOVE** → Unit | Field extraction | ✅ Yes (`resource_extraction_test.go`) |
| GW-INT-ADP-004 | Prometheus Severity Mapping | **MOVE** → Unit | Enum mapping | ✅ Yes (`prometheus_adapter_test.go`) |
| GW-INT-ADP-005 | Prometheus Fingerprint Generation | **KEEP** | Needs audit event validation | No (integration only) |
| GW-INT-ADP-006 | Prometheus Custom Labels Preservation | **MOVE** → Unit | Label processing | ✅ Yes (`prometheus_adapter_test.go`) |
| GW-INT-ADP-007 | Prometheus Long Annotations Truncation | **MOVE** → Unit | String manipulation | ❌ No (NEW unit test needed) |
| GW-INT-ADP-008 | K8s Event Parsing | **MOVE** → Unit | Pure parsing | ✅ Yes (`k8s_event_adapter_test.go`) |
| GW-INT-ADP-009 | K8s Event Reason Extraction | **MOVE** → Unit | Field extraction | ✅ Yes (`k8s_event_adapter_test.go`) |
| GW-INT-ADP-010 | K8s Event InvolvedObject Mapping | **MOVE** → Unit | Object mapping | ✅ Yes (`resource_extraction_test.go`) |
| GW-INT-ADP-011 | K8s Event Severity Inference | **MOVE** → Unit | Logic/rules | ✅ Yes (`k8s_event_adapter_test.go`) |
| GW-INT-ADP-012 | K8s Event Fingerprint Generation | **KEEP** | Needs audit event validation | No (integration only) |
| GW-INT-ADP-013 | K8s Event Malformed Handling | **KEEP** | Error flow with audit | No (integration only) |
| GW-INT-ADP-014 | K8s Event Empty Fields Handling | **KEEP** | Error flow with audit | No (integration only) |
| GW-INT-ADP-015 | Adapter Error Non-Fatal | **KEEP** | Resilience with infrastructure | No (integration only) |

**Keep in Integration**: 5 tests (fingerprint validation, error flows with audit)
**Move to Unit**: 10 tests (8 exist, 1 new needed)

---

### **Category 4: Error Handling (ERR)** - 13 Tests

**Decision**: **3 KEEP, 10 MOVE TO UNIT**

**Rationale**: Error classification and backoff are pure algorithms (unit tests). Only circuit breaker and infrastructure interaction stay in integration.

| Test ID | Test Name | Decision | Reason | Unit Test Exists? |
|---------|-----------|----------|--------|------------------|
| GW-INT-ERR-001 | Transient Error Classification | **MOVE** → Unit | Pure classification logic | ✅ Yes (`edge_cases_test.go`) |
| GW-INT-ERR-002 | Permanent Error Classification | **MOVE** → Unit | Pure classification logic | ✅ Yes (`edge_cases_test.go`) |
| GW-INT-ERR-003 | HTTP Status Error Classification | **MOVE** → Unit | HTTP code mapping | ✅ Yes (`edge_cases_test.go`) |
| GW-INT-ERR-004 | K8s API Error Classification | **MOVE** → Unit | K8s error types | ✅ Yes (`edge_cases_test.go`) |
| GW-INT-ERR-005 | Error Classification Metrics | **KEEP** | Needs real metrics + infrastructure | No (integration only) |
| GW-INT-ERR-006 | Exponential Backoff Calculation | **MOVE** → Unit | Pure math | ❌ No (NEW unit test needed) |
| GW-INT-ERR-007 | Backoff Max Delay Cap | **MOVE** → Unit | Boundary testing | ❌ No (NEW unit test needed) |
| GW-INT-ERR-008 | Backoff Jitter Addition | **MOVE** → Unit | Math + randomness | ❌ No (NEW unit test needed) |
| GW-INT-ERR-009 | Retry Count Tracking | **MOVE** → Unit | State tracking | ✅ Yes (`crd_creator_retry_test.go`) |
| GW-INT-ERR-010 | Backoff Reset On Success | **MOVE** → Unit | State machine | ❌ No (NEW unit test needed) |
| GW-INT-ERR-011 | Retry Context Deadline | **KEEP** | Needs real timing + K8s | No (integration only) |
| GW-INT-ERR-012 | Circuit Breaker Retry Block | **KEEP** | Needs real circuit breaker state | No (integration only) |
| GW-INT-ERR-013 | Error Recovery Metrics | **MOVE** → Unit | Metric increment logic | ❌ No (NEW unit test needed) |

**Keep in Integration**: 3 tests (metrics, timing, circuit breaker)
**Move to Unit**: 10 tests (5 exist, 5 new needed)

---

### **Category 5: Configuration (CFG)** - 7 Tests

**Decision**: **2 KEEP, 5 MOVE TO UNIT**

**Rationale**: Config validation is pure business logic (unit tests). Only hot reload with infrastructure stays in integration.

| Test ID | Test Name | Decision | Reason | Unit Test Exists? |
|---------|-----------|----------|--------|------------------|
| GW-INT-CFG-001 | Config Reload Trigger | **KEEP** | Needs real file watching + infrastructure | No (integration only) |
| GW-INT-CFG-002 | Safe Defaults Validation | **MOVE** → Unit | Pure validation | ✅ Yes (`config_test.go`) |
| GW-INT-CFG-003 | Invalid Config Rejection | **MOVE** → Unit | Validation logic | ✅ Yes (`config_test.go`) |
| GW-INT-CFG-004 | Config Change Audit | **KEEP** | Needs DataStorage audit | No (integration only) |
| GW-INT-CFG-005 | Config Validation Metrics | **MOVE** → Unit | Metric increment | ✅ Yes (`config_test.go`) |
| GW-INT-CFG-006 | Config Rollback On Error | **MOVE** → Unit | State management | ❌ No (NEW unit test needed) |
| GW-INT-CFG-007 | Config Hot Reload No Restart | **MOVE** → Unit | State transition | ❌ No (NEW unit test needed) |

**Keep in Integration**: 2 tests (hot reload, audit)
**Move to Unit**: 5 tests (3 exist, 2 new needed)

---

### **Category 6: Middleware (MID)** - 7 Tests

**Decision**: **2 KEEP, 5 MOVE TO UNIT**

**Rationale**: Middleware logic is mostly pure functions (unit tests). Only metrics emission stays in integration.

| Test ID | Test Name | Decision | Reason | Unit Test Exists? |
|---------|-----------|----------|--------|------------------|
| GW-INT-MID-001 | Middleware Execution Order | **MOVE** → Unit | Chain composition | ✅ Yes (`middleware_suite_test.go`) |
| GW-INT-MID-002 | Request ID Injection | **MOVE** → Unit | Context manipulation | ✅ Yes (`request_id_test.go`) |
| GW-INT-MID-003 | Context Propagation | **MOVE** → Unit | Context passing | ✅ Yes (`request_id_test.go`) |
| GW-INT-MID-004 | Error Middleware Short Circuit | **MOVE** → Unit | Control flow | ✅ Yes (`middleware_suite_test.go`) |
| GW-INT-MID-005 | Middleware Panic Recovery | **MOVE** → Unit | Panic handling | ✅ Yes (`middleware_suite_test.go`) |
| GW-INT-MID-006 | Middleware Metrics Emission | **KEEP** | Needs real metrics + HTTP | No (integration only) |
| GW-INT-MID-007 | Middleware Chain Composition | **KEEP** | Needs full stack integration | No (integration only) |

**Keep in Integration**: 2 tests (metrics, full chain)
**Move to Unit**: 5 tests (all exist)

---

## 📋 **Implementation Actions**

### **Action 1: Keep 47 Tests in Integration Tier**

**Categories**: AUD (20), MET (15), ADP (5), ERR (3), CFG (2), MID (2)

**Infrastructure Requirements**:
- ✅ Real PostgreSQL in Podman
- ✅ Real DataStorage client
- ✅ Real K8s client (envtest)
- ✅ Real Prometheus registry
- ✅ SynchronizedBeforeSuite pattern

**Timeline**: Implement with DataStorage infrastructure setup (Week 1-3)

---

### **Action 2: Create 9 New Unit Tests**

**New unit tests needed** (tests that don't exist yet):

1. **GW-UNIT-ADP-007**: Prometheus long annotations truncation
   - **Location**: `test/unit/gateway/adapters/prometheus_adapter_test.go`
   - **BR**: BR-GATEWAY-001
   - **Effort**: 30 minutes

2. **GW-UNIT-ERR-006**: Exponential backoff calculation
   - **Location**: `test/unit/gateway/processing/backoff_test.go` (new file)
   - **BR**: BR-GATEWAY-113
   - **Effort**: 1 hour

3. **GW-UNIT-ERR-007**: Backoff max delay cap
   - **Location**: `test/unit/gateway/processing/backoff_test.go`
   - **BR**: BR-GATEWAY-113
   - **Effort**: 30 minutes

4. **GW-UNIT-ERR-008**: Backoff jitter addition
   - **Location**: `test/unit/gateway/processing/backoff_test.go`
   - **BR**: BR-GATEWAY-113
   - **Effort**: 30 minutes

5. **GW-UNIT-ERR-010**: Backoff reset on success
   - **Location**: `test/unit/gateway/processing/backoff_test.go`
   - **BR**: BR-GATEWAY-113
   - **Effort**: 30 minutes

6. **GW-UNIT-ERR-013**: Error recovery metrics (mock metrics)
   - **Location**: `test/unit/gateway/metrics/error_recovery_test.go` (new file)
   - **BR**: BR-GATEWAY-113
   - **Effort**: 45 minutes

7. **GW-UNIT-CFG-006**: Config rollback on error
   - **Location**: `test/unit/gateway/config/config_test.go`
   - **BR**: BR-GATEWAY-082
   - **Effort**: 45 minutes

8. **GW-UNIT-CFG-007**: Config hot reload no restart
   - **Location**: `test/unit/gateway/config/config_test.go`
   - **BR**: BR-GATEWAY-082
   - **Effort**: 1 hour

9. **GW-UNIT-ADP-015**: Adapter error non-fatal (mock audit)
   - **Location**: `test/unit/gateway/adapters/adapter_interface_test.go`
   - **BR**: BR-GATEWAY-005
   - **Effort**: 45 minutes

**Total Effort**: ~6 hours for 9 new unit tests

---

### **Action 3: Update Test Plan Document**

**Changes Required**:
1. Remove 30 tests from integration test plan
2. Note which 21 tests already exist in unit tier
3. Add 9 new unit tests to unit test plan
4. Update coverage estimates

**New Integration Test Count**: 47 (down from 77)
**New Coverage Estimate**: ~55% (instead of 62%)

---

## 🎯 **Coverage Impact**

### **Before Triage**

- Integration Tests: 77 planned
- Unit Tests: 221 existing
- Total: 298 tests
- Integration Coverage: 62% (estimated)

### **After Triage**

- Integration Tests: 47 planned (30 moved out)
- Unit Tests: 230 (221 existing + 9 new)
- Total: 277 tests
- Integration Coverage: ~55% (still meets >50% requirement ✅)

**Key Insight**: By focusing integration tests on infrastructure-dependent scenarios and moving pure logic to unit tests, we:
- ✅ Reduce integration test complexity
- ✅ Speed up integration test execution
- ✅ Improve test isolation
- ✅ Still meet >50% coverage mandate
- ✅ Increase unit test coverage (better for TDD)

---

## 📚 **References**

- **Test Plan**: `GW_INTEGRATION_TEST_PLAN_V1.0.md`
- **Unit Tests**: `test/unit/gateway/**/*_test.go` (221 existing tests)
- **Integration Tests**: `test/integration/gateway/**/*_test.go` (22 existing tests)
- **Testing Strategy**: `03-testing-strategy.mdc` (>50% integration coverage required)

---

**Document Status**: ✅ Active
**Created**: 2026-01-15
**Purpose**: Triage test plan for Option A implementation
**Next Step**: Update test plan with triage results
