# Test Plan: Kubernaut Agent Prometheus Observability

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-PR10-v1
**Feature**: Business-value-driven Prometheus metrics for Kubernaut Agent (v1.5)
**Version**: 1.0
**Created**: 2026-04-27
**Author**: Kubernaut Development Team
**Status**: Active
**Branch**: `feature/pr10-v15-prometheus-observability`

---

## 1. Introduction

### 1.1 Purpose

Validate that all 13 new Prometheus metrics defined in BR-KA-OBSERVABILITY-001 are correctly registered, instrumented at production code paths, and exposed at the `:9090/metrics` endpoint with correct labels and values.

### 1.2 Objectives

1. Every metric constant is exported and matches the registered metric name
2. Every `Record*()` helper is nil-safe (`if m == nil { return }`)
3. Every metric increments/observes at the correct production code path
4. Integration tests prove HTTP -> metric wiring from entry point to `/metrics` output

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-kubernautagent` |
| Integration test pass rate | 100% | `make test-integration-kubernautagent` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on metrics package |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on server package |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority

- BR-KA-OBSERVABILITY-001: Agent Prometheus Metrics
- DD-005: Observability Standards
- DD-METRICS-001: Controller Metrics Wiring Pattern

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Nil metrics pointer panic | Test crash | Medium | All existing tests | OPS-1: nil-safe `Record*()` methods |
| R2 | Double metric registration | Test panic | Medium | UT-KA-OBS-002 | OPS-2: sync.Once pattern |
| R3 | Gauge not decremented on panic | Metric drift | Low | UT-KA-OBS-004 | COR-1: defer as first in goroutine |
| R4 | SSE stream skews histogram | Useless P99 | High | IT-KA-OBS-005 | DD-3: exclude `/stream` |

---

## 4. Scope

### 4.1 Features to be Tested

- **Metrics struct** (`internal/kubernautagent/metrics/metrics.go`): Constants, registration, helpers
- **Session Manager instrumentation** (`internal/kubernautagent/session/manager.go`): Session lifecycle metrics
- **Rate limiter instrumentation** (`internal/kubernautagent/server/ratelimit.go`): Rate limit counter
- **HTTP metrics middleware** (`internal/kubernautagent/server/http_metrics.go`): Request duration, in-flight
- **Handler authz metrics** (`internal/kubernautagent/server/handler.go`): Denial counter
- **Audit emit counter** (`internal/kubernautagent/audit/emitter.go`): Event emission tracking
- **Investigator metrics** (`internal/kubernautagent/investigator/investigator.go`): Phase, tool, turn, cost

### 4.2 Features Not to be Tested

- Grafana dashboards (post-v1.0)
- AlertManager rules (post-v1.0)
- Existing `InstrumentedClient` metrics (unchanged)

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of metrics package (pure logic: constants, helpers, nil-safety)
- **Integration**: >=80% of HTTP middleware wiring, rate limiter instrumentation

### 5.2 Pass/Fail Criteria

**PASS**: All P0 tests pass, >=80% per-tier coverage, zero regressions.
**FAIL**: Any P0 test fails, coverage below 80%, or existing tests regress.

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-KA-OBS-001.1 | Session lifecycle metrics | P0 | Unit | UT-KA-OBS-001 through UT-KA-OBS-006 | Pending |
| BR-KA-OBS-001.2 | Investigation quality metrics | P0 | Unit | UT-KA-OBS-007 through UT-KA-OBS-009 | Pending |
| BR-KA-OBS-001.3 | LLM cost tracking | P0 | Unit | UT-KA-OBS-010 | Pending |
| BR-KA-OBS-001.4 | Rate limiting metrics | P0 | Unit | UT-KA-OBS-011 | Pending |
| BR-KA-OBS-001.5 | HTTP request metrics | P0 | Integration | IT-KA-OBS-001 through IT-KA-OBS-003 | Pending |
| BR-KA-OBS-001.6 | Authorization denial metrics | P0 | Integration | IT-KA-OBS-004 | Pending |
| BR-KA-OBS-001.7 | Audit pipeline health | P0 | Unit | UT-KA-OBS-012 | Pending |
| OPS-1 | Nil-safety | P0 | Unit | UT-KA-OBS-013 | Pending |
| DD-005 | Name constants | P0 | Unit | UT-KA-OBS-014 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/metrics/metrics.go` (>=80% coverage)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-OBS-001` | `NewMetricsWithRegistry` registers all 13 metrics without panic | Pending |
| `UT-KA-OBS-002` | `NewMetrics()` called twice does not panic (sync.Once) | Pending |
| `UT-KA-OBS-003` | `RecordSessionStarted` increments counter with correct labels | Pending |
| `UT-KA-OBS-004` | `RecordSessionCompleted` increments counter and observes duration | Pending |
| `UT-KA-OBS-005` | `RecordSessionStarted` truncates signal_name > 128 chars (SEC-1) | Pending |
| `UT-KA-OBS-006` | `RecordInvestigationPhase` increments with phase and outcome | Pending |
| `UT-KA-OBS-007` | `RecordToolCall` increments with tool_name | Pending |
| `UT-KA-OBS-008` | `RecordInvestigationTurns` observes turn count per phase | Pending |
| `UT-KA-OBS-009` | `RecordLLMCost` increments cost counter with model label | Pending |
| `UT-KA-OBS-010` | `RecordLLMCost` returns $0 for unknown model | Pending |
| `UT-KA-OBS-011` | `RecordRateLimited` increments counter | Pending |
| `UT-KA-OBS-012` | `RecordAuditEventEmitted` increments with event_type | Pending |
| `UT-KA-OBS-013` | All `Record*()` methods are nil-safe (no panic on nil receiver) | Pending |
| `UT-KA-OBS-014` | All `MetricName*` constants match registered metric names | Pending |
| `UT-KA-OBS-015` | `RecordAuthzDenied` increments with reason label | Pending |

### Tier 2: Integration Tests

**Testable code scope**: HTTP middleware wiring, rate limiter, handler authz (>=80% coverage)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-OBS-001` | HTTP metrics middleware records request duration on API call | Pending |
| `IT-KA-OBS-002` | HTTP metrics middleware excludes `/stream` from histogram | Pending |
| `IT-KA-OBS-003` | HTTP in-flight gauge increments during request and decrements after | Pending |
| `IT-KA-OBS-004` | Authz denial increments `authz_denied_total` on owner mismatch | Pending |
| `IT-KA-OBS-005` | Rate limiter increments `rate_limited_total` when request rejected | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. Metrics endpoint validation in Kind cluster requires ServiceMonitor CRD and Prometheus operator, which are post-v1.0 infrastructure.

---

## 9. Test Cases

### UT-KA-OBS-001: Metrics Registration

**BR**: BR-KA-OBS-001.1
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/metrics/metrics_test.go`

**Test Steps**:
1. **Given**: A fresh `prometheus.NewRegistry()`
2. **When**: `NewMetricsWithRegistry(registry)` is called
3. **Then**: All 13 metrics are gatherable from the registry without error

### UT-KA-OBS-013: Nil-Safety

**BR**: OPS-1
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/metrics/metrics_test.go`

**Test Steps**:
1. **Given**: A nil `*Metrics` pointer
2. **When**: Every `Record*()` method is called on the nil pointer
3. **Then**: No panic occurs

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None (Prometheus test registry is real)
- **Location**: `test/unit/kubernautagent/metrics/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: ZERO mocks (real httptest server, real Chi router)
- **Infrastructure**: `httptest.NewServer` with full route stack
- **Location**: `test/integration/kubernautagent/server/`

---

## 11. Execution Order

1. **Phase 1 (RED)**: Unit tests for metrics package (UT-KA-OBS-001 through UT-KA-OBS-015)
2. **Phase 2 (GREEN)**: Implement `metrics.go`, inject into production code
3. **Phase 3 (REFACTOR)**: Polish helpers, nil-safety, 100 Go Mistakes audit
4. **Phase 4 (WIRING)**: Integration tests (IT-KA-OBS-001 through IT-KA-OBS-005)

---

## 12. Execution

```bash
# Unit tests
make test-unit-kubernautagent

# Integration tests
make test-integration-kubernautagent

# Specific test by ID
go test ./test/unit/kubernautagent/metrics/... -ginkgo.focus="UT-KA-OBS"

# Coverage
go test ./test/unit/kubernautagent/metrics/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 13. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-27 | Initial test plan |
