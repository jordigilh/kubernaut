# IEEE 829 Test Plan — Issue #38: Circuit Breaker and Resilience

**Test Plan Identifier:** TP-AF-038
**Version:** 1.0
**Date:** 2026-05-06
**Status:** Draft
**Issue:** [#38](https://github.com/jordigilh/kubernaut-apifrontend/issues/38)

## 1. Introduction

This test plan covers the implementation of circuit breaker and resilience patterns
for the three downstream dependencies of the API Frontend: KA (Kubernaut Agent),
DS (Data Storage), and K8s API.

## 2. Test Items

| Component | Package | Description |
|---|---|---|
| ResilienceConfig | `internal/config` | Configuration extension with per-dependency timeouts, CB params, retry params |
| RetryTransport | `internal/resilience` | HTTP RoundTripper with exponential backoff retry |
| CircuitBreakerTransport | `internal/resilience` | HTTP RoundTripper with gobreaker/v2 circuit breaker |
| Resilience Metrics | `internal/resilience` | Prometheus metrics wiring for CB state, request duration, retry count |
| DS Ogen Adapter | `internal/ds` | Adapter wrapping ogen-generated DS client satisfying `ds.Client` interface |
| KA Client Refactor | `internal/ka` | KA REST client refactored to use shared resilience transport |
| K8s CB Wrapper | `internal/resilience` | Application-level CB wrapper for K8s dynamic client operations |
| Readiness Integration | `internal/handler` | CB states composed into `/readyz` probe |

## 3. Features to Be Tested

### 3.1 Circuit Breaker Behavior
- CB transitions: closed -> open after N failures
- CB transitions: open -> half-open after timeout
- CB transitions: half-open -> closed after success
- CB fail-fast: requests rejected immediately when open
- CB metrics: `af_circuit_breaker_state` gauge updates on each transition

### 3.2 Retry Behavior
- Retry on transient errors: 502, 503, 504, connection reset, EOF
- No retry on non-transient errors: 400, 401, 404, 500
- No retry on non-replayable bodies (POST without GetBody)
- Exponential backoff with jitter
- Context cancellation during backoff
- Retry exhaustion propagates last error
- Retry counter metric increments

### 3.3 Configuration
- Valid resilience config loads and validates
- Invalid config (negative timeout, zero threshold) rejected
- Default values applied for omitted fields
- Hot-reload of resilience config updates transport parameters

### 3.4 DS Client Adapter
- Maps `ds.Client` interface methods to ogen-generated client
- Injects resilience transport via `WithClient`
- Handles ogen union response types (success vs error variants)
- `CreateAuditEvent` for audit trace storage

### 3.5 KA Client Refactor
- Existing behavior preserved (Analyze, Status, Result, Cancel)
- Body replay enabled for POST requests (bytes.Buffer)
- CB state changes reported to Prometheus metric
- Retry logic applied to GET operations only

### 3.6 K8s CB Wrapper
- CB wraps non-watch CRD operations (Get, List, Create, Update, Delete)
- Watch operations bypass CB
- CB state reported to metric

### 3.7 Readiness
- `/readyz` returns 503 when any CB is open
- `/readyz` returns 200 when all CBs are closed/half-open

## 4. Features Not to Be Tested

- K8s client-go internal retry/backoff (tested upstream)
- ogen client HTTP serialization (tested in kubernaut)
- gobreaker/v2 internal state machine (tested upstream)

## 5. Approach

TDD methodology: Red (failing tests) -> Green (minimal implementation) -> Refactor (quality).
Each phase has a 9-category checkpoint audit before advancing.

## 6. Test Cases

### 6.1 Config Extension (Phase 1)

| ID | Description | Type | AC |
|---|---|---|---|
| UT-AF-038-001 | Load valid resilience config from YAML | Unit | Configurable timeouts |
| UT-AF-038-002 | Validate rejects negative connectTimeout | Unit | Input validation |
| UT-AF-038-003 | Validate rejects negative requestTimeout | Unit | Input validation |
| UT-AF-038-004 | Validate rejects cbFailureThreshold > 100 | Unit | Bounds check |
| UT-AF-038-005 | Validate rejects retryMax > 10 | Unit | Bounds check |
| UT-AF-038-006 | Validate rejects retryableStatuses outside 400-599 | Unit | Bounds check |
| UT-AF-038-007 | Default values applied when resilience section omitted | Unit | Graceful defaults |
| UT-AF-038-008 | Validate rejects requestTimeout < connectTimeout | Unit | Semantic validation |

### 6.2 Resilience Transport (Phase 2)

| ID | Description | Type | AC |
|---|---|---|---|
| UT-AF-038-010 | RetryTransport retries on 503 and succeeds on 2nd attempt | Unit | Retry with backoff |
| UT-AF-038-011 | RetryTransport gives up after max attempts | Unit | Retry exhaustion |
| UT-AF-038-012 | RetryTransport does not retry 400 | Unit | Non-retryable |
| UT-AF-038-013 | RetryTransport does not retry non-replayable body | Unit | Safety |
| UT-AF-038-014 | RetryTransport respects context cancellation during backoff | Unit | Graceful cancel |
| UT-AF-038-015 | RetryTransport increments `af_downstream_retry_total` | Unit | Metrics |
| UT-AF-038-016 | CBTransport opens after N consecutive failures | Unit | CB opens |
| UT-AF-038-017 | CBTransport rejects immediately when open | Unit | Fail-fast |
| UT-AF-038-018 | CBTransport transitions half-open -> closed on success | Unit | Recovery |
| UT-AF-038-019 | CBTransport updates `af_circuit_breaker_state` gauge | Unit | Metrics |
| UT-AF-038-020 | CBTransport records `af_downstream_request_duration_seconds` | Unit | Metrics |
| UT-AF-038-021 | Full chain: CB -> Retry -> base under concurrent load (10 goroutines) | Stress | Concurrency |
| UT-AF-038-022 | RetryTransport handles connection reset (ECONNRESET) | Unit | Error classification |
| UT-AF-038-023 | RetryTransport handles io.EOF | Unit | Error classification |
| UT-AF-038-024 | CBTransport emits audit event on state change | Unit | FedRAMP AU-2 |

### 6.3 DS Ogen Client (Phase 3)

| ID | Description | Type | AC |
|---|---|---|---|
| UT-AF-038-030 | NewOgenDSClient constructs with resilience transport | Unit | Wiring |
| UT-AF-038-031 | ListWorkflows maps opts to ogen params and returns results | Unit | Interface mapping |
| UT-AF-038-032 | GetRemediationHistory maps opts and returns results | Unit | Interface mapping |
| UT-AF-038-033 | GetEffectiveness maps opts and returns results | Unit | Interface mapping |
| UT-AF-038-034 | GetAuditTrail maps opts and returns results | Unit | Interface mapping |
| UT-AF-038-035 | CreateAuditEvent sends event and handles success | Unit | Audit write |
| UT-AF-038-036 | DS client returns error on ogen error response | Unit | Error handling |
| UT-AF-038-037 | DS client returns error on network failure | Unit | Error handling |
| UT-AF-038-038 | DS client with nil config fields handles gracefully | Unit | Nil edge cases |

### 6.4 KA Client Refactor (Phase 4)

| ID | Description | Type | AC |
|---|---|---|---|
| UT-AF-038-040 | KA Analyze still returns session ID on success | Unit | Behavior preserved |
| UT-AF-038-041 | KA Status still returns session status | Unit | Behavior preserved |
| UT-AF-038-042 | KA Result still returns incident response | Unit | Behavior preserved |
| UT-AF-038-043 | KA Cancel still succeeds on 200 | Unit | Behavior preserved |
| UT-AF-038-044 | KA POST body is replayable (retry works on POST) | Unit | RISK-2 fix |
| UT-AF-038-045 | KA CB state reported to Prometheus gauge | Unit | Metrics wiring |
| UT-AF-038-046 | KA retry on GET /status with 503 succeeds on 2nd attempt | Unit | Retry |
| UT-AF-038-047 | KA does NOT retry POST /analyze (non-idempotent) | Unit | Safety |

### 6.5 K8s CB Wrapper (Phase 5)

| ID | Description | Type | AC |
|---|---|---|---|
| UT-AF-038-050 | K8s CB opens after N API server failures | Unit | CB opens |
| UT-AF-038-051 | K8s CB passes through when closed | Unit | Normal operation |
| UT-AF-038-052 | K8s Watch operations bypass CB | Unit | Watch exemption |
| UT-AF-038-053 | K8s CB state reported to Prometheus gauge | Unit | Metrics |

### 6.6 Readiness (Phase 6)

| ID | Description | Type | AC |
|---|---|---|---|
| UT-AF-038-060 | /readyz returns 503 when KA CB is open | Integration | Readiness |
| UT-AF-038-061 | /readyz returns 503 when DS CB is open | Integration | Readiness |
| UT-AF-038-062 | /readyz returns 503 when K8s CB is open | Integration | Readiness |
| UT-AF-038-063 | /readyz returns 200 when all CBs closed | Integration | Readiness |
| UT-AF-038-064 | /readyz returns 200 when CB is half-open | Integration | Readiness |

### 6.7 Integration (Phase 7)

| ID | Description | Type | AC |
|---|---|---|---|
| IT-AF-038-070 | KA 5xx burst opens CB, next call fails fast | Integration | End-to-end |
| IT-AF-038-071 | KA CB recovers after timeout period | Integration | End-to-end |
| IT-AF-038-072 | DS timeout opens CB, subsequent calls fail fast | Integration | End-to-end |
| IT-AF-038-073 | Retry succeeds on transient 503 then 200 | Integration | End-to-end |
| IT-AF-038-074 | Retry exhaustion returns last error to caller | Integration | End-to-end |

## 7. Pass/Fail Criteria

- All unit tests pass with `-race` flag
- Code coverage >= 80% for `internal/resilience/`, `internal/ds/`, `internal/ka/`
- `golangci-lint` clean
- `govulncheck` clean after adding kubernaut module dependency
- All 9-category checkpoint audits pass at each phase boundary

## 8. Environmental Needs

- Go 1.25.6+
- `httptest.Server` for simulating downstream failures
- `prometheus/testutil` for metric assertions
- Ginkgo/Gomega test framework (per ADR-004)

## 9. Responsibilities

| Role | Responsibility |
|---|---|
| Developer | Write tests, implement code, run checkpoint audits |
| CI | Automated lint, race detection, coverage gating |
| Reviewer | Verify 9-category audit compliance |

## 10. Schedule

| Phase | Estimated Tests | Checkpoint |
|---|---|---|
| Phase 1: Config | 8 | After Green |
| Phase 2: Resilience | 15 | After Green |
| Phase 3: DS Client | 9 | After Green |
| Phase 4: KA Refactor | 8 | After Green |
| Phase 5: K8s CB | 4 | After Green |
| Phase 6: Readiness | 5 | After Green |
| Phase 7: Integration | 5 | Final |
