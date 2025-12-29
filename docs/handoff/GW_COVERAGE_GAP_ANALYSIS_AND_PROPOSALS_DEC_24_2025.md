# Gateway Coverage Gap Analysis & Test Scenario Proposals (Dec 24, 2025)

## 📊 **Executive Summary**

**Analysis Status**: ✅ COMPLETE - Coverage gaps identified from all 3 tiers
**Proposed New Scenarios**: **9 business outcome-oriented test groups** (47 total test cases)
**Focus**: Business logic NOT covered in any of the 3 defense layers (Unit/Integration/E2E)

**Current Coverage**:
- Unit: 87.5% (314 tests)
- Integration: 58.3% (92 tests)
- E2E: 70.6% (37 tests)

**Gap**: 58.3% of code covered in ALL tiers, but **41.7% has single-layer or no coverage**

---

## 🔍 **Coverage Gap Analysis Methodology**

### Analysis Approach

1. **Analyzed integration test coverage report** (`gateway-integration-coverage.out`)
2. **Identified functions with 0% or <50% coverage**
3. **Mapped uncovered functions to business requirements** (BR-GATEWAY-XXX)
4. **Verified gaps exist across ALL THREE tiers** (not just one)
5. **Prioritized by business impact** (customer-facing outcomes)

### Key Findings

**0% Coverage Functions** (No tier tests these):
- `handleProcessingError()` - K8s API error classification
- `writeInternalError()` - HTTP 500 error responses
- `writeServiceUnavailableError()` - HTTP 503 with retry-after
- `extractTimestamp()` - Timestamp security validation
- `validateTimestampWindow()` - Replay attack prevention
- `respondTimestampError()` - RFC 7807 timestamp errors
- `NewConfigError()` - Configuration error formatting
- `NewDeduplicationError()` - Deduplication failure context
- `GetAdapter()`, `GetAllAdapters()`, `Count()` - Adapter registry operations

**Business Impact**: These gaps represent **critical error paths** and **security features** that are untested

---

## 🎯 **Proposed Test Scenarios (Business Outcome-Oriented)**

### **GROUP 1: Service Resilience & Degradation (BR-GATEWAY-003)**

**Business Outcome**: Gateway remains available and provides actionable feedback during infrastructure failures

**Missing Coverage**: Service unavailability handling (0% coverage)

#### Proposed Tests (7 scenarios):

| Test ID | Business Scenario | Expected Outcome | Tier | Priority |
|---------|-------------------|------------------|------|----------|
| **GW-RES-001** | Kubernetes API unreachable (all pods down) | HTTP 503 with Retry-After header | Integration | P0 |
| **GW-RES-002** | Kubernetes API rate limiting (429 responses) | HTTP 503 with exponential backoff | Integration | P0 |
| **GW-RES-003** | Kubernetes API partial degradation (slow responses) | Alerts processed with increased latency, no failures | Integration | P1 |
| **GW-RES-004** | DataStorage service unavailable | Deduplication bypassed, CRDs created with warning | Integration | P1 |
| **GW-RES-005** | Sustained K8s API errors (>1 minute) | Circuit breaker activates, 503 responses | Integration | P1 |
| **GW-RES-006** | Recovery from K8s API outage | Service auto-recovers, processes queued alerts | Integration | P1 |
| **GW-RES-007** | Kubernetes API connection pool exhausted | HTTP 503 with actionable error message | Integration | P2 |

**Gap Filled**: Tests critical degradation scenarios currently at 0% coverage

**BR Mapping**: BR-GATEWAY-003 (Service availability), BR-GATEWAY-112 (Error classification)

---

### **GROUP 2: Security & Attack Prevention (BR-GATEWAY-074, BR-GATEWAY-075)**

**Business Outcome**: Gateway rejects malicious requests and prevents security exploits

**Missing Coverage**: Timestamp validation edge cases (0% coverage for replay attack paths)

#### Proposed Tests (8 scenarios):

| Test ID | Business Scenario | Expected Outcome | Tier | Priority |
|---------|-------------------|------------------|------|----------|
| **GW-SEC-001** | Replay attack (alert with 10-minute-old timestamp) | HTTP 400 with "timestamp too old" error | Unit | P0 |
| **GW-SEC-002** | Clock skew attack (timestamp 2 hours in future) | HTTP 400 with "timestamp in future" error | Unit | P0 |
| **GW-SEC-003** | Negative timestamp (-1 Unix epoch) | HTTP 400 with "invalid timestamp" error | Unit | P0 |
| **GW-SEC-004** | Missing X-Timestamp header | HTTP 400 with "timestamp required" error | Unit | P0 |
| **GW-SEC-005** | Malformed timestamp (non-numeric) | HTTP 400 with "invalid timestamp format" error | Unit | P0 |
| **GW-SEC-006** | Boundary: Timestamp exactly at tolerance limit (5min) | HTTP 202 (accepted) | Unit | P1 |
| **GW-SEC-007** | Boundary: Timestamp 1 second beyond tolerance | HTTP 400 (rejected) | Unit | P1 |
| **GW-SEC-008** | RFC 7807 error format validation for timestamp failures | Error response matches RFC 7807 schema | Unit | P1 |

**Gap Filled**: Security attack scenarios currently at 0% coverage

**BR Mapping**: BR-GATEWAY-074 (Webhook timestamp validation), BR-GATEWAY-075 (Replay attack prevention), BR-GATEWAY-101 (RFC 7807 errors)

---

### **GROUP 3: IP-Based Rate Limiting & Source Tracking (BR-GATEWAY-004)**

**Business Outcome**: Accurately identify alert sources for rate limiting and audit trails

**Missing Coverage**: IP extraction middleware (0% coverage)

#### Proposed Tests (6 scenarios):

| Test ID | Business Scenario | Expected Outcome | Tier | Priority |
|---------|-------------------|------------------|------|----------|
| **GW-IP-001** | Alert from direct client (no proxy headers) | IP extracted from RemoteAddr | Unit | P0 |
| **GW-IP-002** | Alert through single proxy (X-Forwarded-For) | IP extracted from X-Forwarded-For | Unit | P0 |
| **GW-IP-003** | Alert through multiple proxies (XFF chain) | Leftmost (original) IP extracted | Unit | P0 |
| **GW-IP-004** | X-Real-IP header present (alternative to XFF) | IP extracted from X-Real-IP | Unit | P1 |
| **GW-IP-005** | Conflicting headers (both XFF and X-Real-IP) | X-Forwarded-For takes precedence | Unit | P1 |
| **GW-IP-006** | Malformed IP in proxy headers | Falls back to RemoteAddr | Unit | P1 |

**Gap Filled**: IP extraction for rate limiting (0% coverage)

**BR Mapping**: BR-GATEWAY-004 (Per-source rate limiting infrastructure)

---

### **GROUP 4: Configuration Validation & Startup (BR-GATEWAY-111)**

**Business Outcome**: Gateway fails fast with actionable error messages for misconfigurations

**Missing Coverage**: Config error handling (0% coverage)

#### Proposed Tests (5 scenarios):

| Test ID | Business Scenario | Expected Outcome | Tier | Priority |
|---------|-------------------|------------------|------|----------|
| **GW-CFG-001** | Invalid retry configuration (negative max attempts) | Startup fails with suggestion for valid range | Unit | P0 |
| **GW-CFG-002** | Invalid timeout configuration (timeout > maxBackoff) | Startup fails with configuration conflict explanation | Unit | P0 |
| **GW-CFG-003** | Missing required field (kubeconfig path) | Startup fails with field name and documentation link | Unit | P0 |
| **GW-CFG-004** | Invalid adapter registration (duplicate adapter name) | Startup fails with adapter conflict details | Unit | P1 |
| **GW-CFG-005** | ConfigError with full context (suggestion, impact, docs) | Error message includes all context fields | Unit | P1 |

**Gap Filled**: Configuration validation error paths (0% coverage)

**BR Mapping**: BR-GATEWAY-111 (Retry configuration), startup validation

---

### **GROUP 5: Error Classification & Retry Logic (BR-GATEWAY-112)**

**Business Outcome**: Transient errors are retried automatically; permanent errors fail fast

**Missing Coverage**: Error classification and retry error contexts (0% coverage)

#### Proposed Tests (6 scenarios):

| Test ID | Business Scenario | Expected Outcome | Tier | Priority |
|---------|-------------------|------------------|------|----------|
| **GW-ERR-001** | Transient K8s API error (connection refused) | Retried with exponential backoff (3 attempts) | Integration | P0 |
| **GW-ERR-002** | Permanent K8s API error (namespace not found) | Fails immediately without retry | Integration | P0 |
| **GW-ERR-003** | Retryable error exhausts max attempts | Returns RetryError with attempt count and error type | Integration | P0 |
| **GW-ERR-004** | Network timeout during CRD creation | Classified as transient, retried | Integration | P1 |
| **GW-ERR-005** | K8s API 409 conflict (CRD already exists) | Classified as non-retryable, succeeds (idempotent) | Integration | P1 |
| **GW-ERR-006** | Deduplication error with full context | DeduplicationError includes fingerprint, namespace, status | Unit | P1 |

**Gap Filled**: Error classification and retry logic (0% coverage for error types)

**BR Mapping**: BR-GATEWAY-112 (Error classification), BR-GATEWAY-113 (Exponential backoff), BR-GATEWAY-114 (Retry metrics)

---

### **GROUP 6: Adapter Registry & Dynamic Configuration (BR-GATEWAY-027)**

**Business Outcome**: Gateway supports multiple alert sources with runtime adapter management

**Missing Coverage**: Adapter registry operations (0% coverage)

#### Proposed Tests (4 scenarios):

| Test ID | Business Scenario | Expected Outcome | Tier | Priority |
|---------|-------------------|------------------|------|----------|
| **GW-ADR-001** | Gateway startup with zero adapters registered | Startup fails with "no adapters registered" error | Unit | P0 |
| **GW-ADR-002** | Dynamic adapter registration (Prometheus, K8s Events) | GetAllAdapters() returns both adapters | Unit | P1 |
| **GW-ADR-003** | Retrieve adapter by name (GetAdapter("prometheus")) | Returns correct adapter instance | Unit | P1 |
| **GW-ADR-004** | Adapter count validation during startup | Count() >= 1 required for server start | Unit | P1 |

**Gap Filled**: Adapter management currently at 0% coverage

**BR Mapping**: BR-GATEWAY-027 (Multi-source alert ingestion)

---

### **GROUP 7: HTTP Metrics & Observability (BR-GATEWAY-071, BR-GATEWAY-079)**

**Business Outcome**: Complete performance observability for troubleshooting and capacity planning

**Missing Coverage**: HTTP metrics edge cases (0% coverage for some paths)

#### Proposed Tests (4 scenarios):

| Test ID | Business Scenario | Expected Outcome | Tier | Priority |
|---------|-------------------|------------------|------|----------|
| **GW-MET-001** | HTTP 500 error recorded in metrics | `gateway_http_requests_total{status="500"}` increments | Integration | P1 |
| **GW-MET-002** | HTTP 503 error recorded in metrics | `gateway_http_requests_total{status="503"}` increments | Integration | P1 |
| **GW-MET-003** | Request duration for slow K8s API calls | `gateway_http_request_duration_seconds` captures P99 latency | Integration | P1 |
| **GW-MET-004** | In-flight requests metric during concurrent load | `gateway_http_requests_in_flight` accurately tracks concurrent requests | Integration | P2 |

**Gap Filled**: Error path metrics (partial coverage)

**BR Mapping**: BR-GATEWAY-071 (HTTP observability), BR-GATEWAY-072 (In-flight tracking), BR-GATEWAY-079 (Performance metrics)

---

### **GROUP 8: Deduplication Edge Cases (BR-GATEWAY-185)**

**Business Outcome**: Accurate deduplication even under K8s API failures

**Missing Coverage**: Deduplication error paths (partial coverage)

#### Proposed Tests (4 scenarios):

| Test ID | Business Scenario | Expected Outcome | Tier | Priority |
|---------|-------------------|------------------|------|----------|
| **GW-DED-001** | Deduplication check fails (K8s API unavailable) | HTTP 500 with "deduplication check failed" error | Integration | P0 |
| **GW-DED-002** | Field selector query returns corrupted data | Gracefully handles, falls back to "no duplicate found" | Integration | P1 |
| **GW-DED-003** | Deduplication check timeout (K8s API slow) | HTTP 503 after retry timeout | Integration | P1 |
| **GW-DED-004** | Concurrent deduplication checks (race condition) | Both requests succeed, one marked as duplicate | Integration | P1 |

**Gap Filled**: Deduplication failure scenarios (0% coverage for error paths)

**BR Mapping**: BR-GATEWAY-185 (K8s-based deduplication with field selectors)

---

### **GROUP 9: End-to-End Business Outcomes (Cross-BR Validation)**

**Business Outcome**: Complete alert lifecycle validation under real-world conditions

**Missing Coverage**: Multi-service interaction scenarios

#### Proposed Tests (3 scenarios):

| Test ID | Business Scenario | Expected Outcome | Tier | Priority |
|---------|-------------------|------------------|------|----------|
| **GW-E2E-001** | Alert storm (100 alerts/sec for 1 minute) | All alerts processed, deduplication works correctly | E2E | P1 |
| **GW-E2E-002** | Gateway restart during alert processing | In-flight alerts complete, new alerts accepted immediately | E2E | P1 |
| **GW-E2E-003** | Cross-namespace alert deduplication (security test) | Alerts in namespace-A do NOT deduplicate with namespace-B | E2E | P0 |

**Gap Filled**: Real-world production scenarios

**BR Mapping**: Multiple BRs (cross-service validation)

---

## 📊 **Prioritization Matrix**

### P0 (Critical - Production Blockers): 16 tests

**Justification**: These scenarios represent **customer-facing failures** or **security vulnerabilities**

- Service unavailability (2 tests): GW-RES-001, GW-RES-002
- Security attacks (5 tests): GW-SEC-001 through GW-SEC-005
- IP extraction (3 tests): GW-IP-001, GW-IP-002, GW-IP-003
- Config validation (3 tests): GW-CFG-001, GW-CFG-002, GW-CFG-003
- Error classification (3 tests): GW-ERR-001, GW-ERR-002, GW-ERR-003
- Adapter registry (1 test): GW-ADR-001
- Deduplication failures (1 test): GW-DED-001
- Cross-namespace security (1 test): GW-E2E-003

### P1 (High Priority - Operational Excellence): 26 tests

**Justification**: These scenarios represent **operational robustness** and **troubleshooting capabilities**

- Service degradation (4 tests): GW-RES-003 through GW-RES-006
- Security edge cases (3 tests): GW-SEC-006, GW-SEC-007, GW-SEC-008
- IP extraction edge cases (3 tests): GW-IP-004, GW-IP-005, GW-IP-006
- Config validation edge cases (2 tests): GW-CFG-004, GW-CFG-005
- Error classification edge cases (3 tests): GW-ERR-004, GW-ERR-005, GW-ERR-006
- Adapter management (3 tests): GW-ADR-002, GW-ADR-003, GW-ADR-004
- Metrics observability (3 tests): GW-MET-001, GW-MET-002, GW-MET-003
- Deduplication edge cases (3 tests): GW-DED-002, GW-DED-003, GW-DED-004
- E2E real-world scenarios (2 tests): GW-E2E-001, GW-E2E-002

### P2 (Enhancement - Nice-to-Have): 5 tests

**Justification**: These scenarios provide **additional confidence** but are not blocking production

- Connection pool exhaustion: GW-RES-007
- In-flight metrics: GW-MET-004

---

## 📈 **Expected Impact on Defense-in-Depth**

### Current State (Before New Tests)

| Tier | Coverage | Tests | Gap |
|------|----------|-------|-----|
| Unit | 87.5% | 314 | Missing security & error edge cases |
| Integration | 58.3% | 92 | Missing degradation & resilience |
| E2E | 70.6% | 37 | Missing cross-namespace & storm scenarios |

### Projected State (After New Tests)

| Tier | Projected Coverage | New Tests | Gap Addressed |
|------|-------------------|-----------|---------------|
| Unit | **92-95%** | +29 tests | Security, config, IP extraction |
| Integration | **68-72%** | +13 tests | K8s API failures, error classification |
| E2E | **75-78%** | +3 tests | Real-world production scenarios |

**Key Improvement**: Error paths and security features will move from **0% to 90%+ coverage** across all tiers

---

## 🛠️ **Implementation Roadmap**

### Phase 1: P0 Tests (16 tests) - Estimated 3-4 days

**Focus**: Security vulnerabilities and critical error paths

**Deliverables**:
- Unit tests for timestamp validation (5 tests)
- Unit tests for IP extraction (3 tests)
- Unit tests for config validation (3 tests)
- Integration tests for K8s API failures (3 tests)
- Integration tests for error classification (3 tests)
- E2E test for cross-namespace security (1 test)

**Success Criteria**: No security vulnerabilities, K8s API error handling validated

### Phase 2: P1 Tests (26 tests) - Estimated 5-6 days

**Focus**: Operational resilience and observability

**Deliverables**:
- Unit tests for edge cases (11 tests)
- Integration tests for degradation scenarios (10 tests)
- Integration tests for metrics (3 tests)
- E2E tests for real-world scenarios (2 tests)

**Success Criteria**: Service resilience validated, metrics coverage complete

### Phase 3: P2 Tests (5 tests) - Estimated 1-2 days

**Focus**: Performance and capacity planning

**Deliverables**:
- Integration tests for connection pool (1 test)
- Integration tests for in-flight metrics (1 test)
- E2E tests for sustained load (3 tests)

**Success Criteria**: Capacity planning data available

---

## ✅ **Validation Approach**

### For Each Proposed Test

**Before Implementation**:
1. Verify test scenario is NOT covered in existing tests (grep for similar assertions)
2. Map to specific BR-GATEWAY-XXX requirement
3. Define expected business outcome (not just technical behavior)
4. Identify tier (Unit/Integration/E2E) based on dependencies

**During Implementation**:
1. Follow TDD (test fails first, then implement, then refactor)
2. Add BR-GATEWAY-XXX reference in test description
3. Include business outcome in test documentation
4. Verify test covers 0% coverage functions

**After Implementation**:
1. Re-run coverage analysis (expect 10-20% increase)
2. Verify test appears in defense-in-depth overlap (tested in multiple tiers where applicable)
3. Document coverage improvement in test summary

---

## 📚 **Related Documents**

- **Defense-in-Depth Analysis**: `docs/handoff/GW_DEFENSE_IN_DEPTH_ANALYSIS_DEC_24_2025.md`
- **Complete Validation**: `docs/handoff/GW_COMPLETE_DEFENSE_IN_DEPTH_VALIDATED_DEC_24_2025.md`
- **Testing Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md`
- **Coverage Standards**: `.cursor/rules/15-testing-coverage-standards.mdc`

---

## 🎯 **Key Takeaways**

### 1. **Critical Gap: Error Paths**

**47 test scenarios** address error paths and edge cases currently at **0% coverage**

**Business Impact**: These are the paths users experience when things go wrong - untested = production incidents

### 2. **Security Validation Missing**

**8 security tests** for replay attacks, timestamp validation, and cross-namespace isolation are at **0% coverage**

**Business Impact**: Potential security vulnerabilities currently undetected

### 3. **Resilience Untested**

**7 service degradation tests** for K8s API failures are at **0% coverage**

**Business Impact**: Unknown behavior during infrastructure failures

### 4. **Defense-in-Depth Enhancement**

Adding these tests will increase **defense-in-depth overlap from 58.3% to ~75%+**

**Business Impact**: Bugs must pass through more defense layers to reach production

---

**Document Version**: 1.0
**Analysis Date**: Dec 24, 2025
**Total Proposed Tests**: 47 scenarios across 9 business outcome groups
**Expected Timeline**: 9-12 days for full implementation
**Priority**: P0 tests (16) should be implemented immediately for production readiness







