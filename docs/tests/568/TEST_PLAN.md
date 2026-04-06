# Test Plan: Mock LLM Prometheus Metrics (#568)

**Test Plan Identifier**: TP-568-v1
**Feature**: Prometheus metrics endpoint for Mock LLM observability
**Version**: 1.0
**Created**: 2026-03-28
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.3`

---

## 1. Introduction

### 1.1 Purpose

Validate that the Mock LLM service exposes Prometheus-format metrics at `/metrics`,
covering request counts, response latencies, scenario detection, and DAG phase transitions.
Metrics must be resettable via the existing `/api/test/reset` endpoint.

### 1.2 Objectives

1. **Endpoint availability**: `GET /metrics` returns Prometheus text format
2. **Request metrics**: Counter and histogram correctly track all OpenAI/Ollama requests
3. **Scenario metrics**: Detection counter increments with correct labels
4. **DAG metrics**: Phase transition counter tracks node traversals
5. **Resettability**: All metrics reset to zero on `POST /api/test/reset`

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/mockllm/...` |
| Integration test pass rate | 100% | `go test ./test/integration/mockllm/...` |
| Backward compatibility | 0 regressions | Existing tests pass unmodified |

---

## 2. References

| Document | Relevance |
|----------|-----------|
| BR-MOCK-080: Prometheus Metrics Endpoint | `/metrics` endpoint requirement |
| BR-MOCK-081: Request Metrics | Counter + histogram specs |
| BR-MOCK-082: Scenario Detection Metrics | Detection counter spec |
| BR-MOCK-083: Phase Transition Metrics | DAG traversal counter spec |
| DD-005 V3.0 | Metric naming pattern (service_component_metric) |
| `docs/tests/531/IMPLEMENTATION_PLAN.md` | Master plan Phase 6 |

---

## 3. Scope

### 3.1 In Scope

- `GET /metrics` endpoint returning Prometheus text format
- `mock_llm_requests_total{endpoint,status_code,scenario}` counter
- `mock_llm_response_duration_seconds{endpoint,scenario}` histogram
- `mock_llm_scenario_detection_total{scenario,method}` counter
- `mock_llm_dag_phase_transitions_total{from_node,to_node}` counter
- Metric reset on `POST /api/test/reset`
- Test isolation via `prometheus.NewRegistry()` (no global pollution)

### 3.2 Out of Scope

- controller-runtime integration (Mock LLM is standalone)
- Grafana dashboards
- Alert rules

---

## 4. Design Decisions

1. **Standalone registry**: Mock LLM is not a controller-runtime app, so we use a
   plain `prometheus.NewRegistry()` + `promhttp.HandlerFor()` instead of
   `ctrlmetrics.Registry`.

2. **Resettable metrics**: Since `/api/test/reset` must clear metrics, we use a
   custom `ResettableRegistry` that can unregister and re-register collectors,
   or we recreate the `Metrics` struct on reset.

3. **Test isolation**: Each test creates its own `prometheus.NewRegistry()` to
   avoid cross-test pollution (following `NewMetricsWithRegistry` pattern from
   other services).

---

## 5. Test Scenarios

### 5.1 Integration Tests

| ID | Description | BR |
|----|-------------|-----|
| IT-MOCK-568-001 | GET /metrics returns 200 with Prometheus text format | BR-MOCK-080 |
| IT-MOCK-568-002 | Request counter increments after OpenAI chat request | BR-MOCK-081 |
| IT-MOCK-568-003 | Response duration histogram records latency | BR-MOCK-081 |
| IT-MOCK-568-004 | Scenario detection counter increments with correct labels | BR-MOCK-082 |
| IT-MOCK-568-005 | DAG phase transition counter records traversal | BR-MOCK-083 |
| IT-MOCK-568-006 | POST /api/test/reset clears all metric counters | BR-MOCK-080 |
| IT-MOCK-568-007 | Ollama request increments request counter | BR-MOCK-081 |

### 5.2 Unit Tests

| ID | Description | BR |
|----|-------------|-----|
| UT-MOCK-568-001 | Metrics struct registers all collectors without panic | BR-MOCK-080 |
| UT-MOCK-568-002 | RecordRequest increments counter with correct labels | BR-MOCK-081 |
| UT-MOCK-568-003 | RecordScenarioDetection increments with method label | BR-MOCK-082 |
| UT-MOCK-568-004 | RecordDAGTransition increments with from/to labels | BR-MOCK-083 |
