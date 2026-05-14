# BR-KA-OBSERVABILITY-001: Kubernaut Agent Prometheus Metrics

**Business Requirement ID**: BR-KA-OBSERVABILITY-001
**Category**: KA (Kubernaut Agent)
**Priority**: P1
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-04-27

---

## Business Need

### Problem Statement

The Kubernaut Agent (KA) exposes LLM request-level metrics via `InstrumentedClient` (`aiagent_api_llm_requests_total`, `aiagent_api_llm_request_duration_seconds`, `aiagent_api_llm_tokens_total`) but has no session-level, investigation-quality, HTTP, or cost observability. Operators cannot answer fundamental questions:

- "How many investigations are running right now?"
- "What is our investigation success/failure ratio?"
- "Which investigation phase fails most?"
- "How much are we spending on LLM calls?"
- "Are clients being rate-limited?"
- "What are our API response times?"
- "Are there unauthorized access attempts?"
- "Is the audit pipeline healthy?"

### Business Impact

| Stakeholder | Gap | Impact |
|---|---|---|
| SRE | No session lifecycle visibility | Cannot set SLOs or capacity plan |
| Product | No investigation quality metrics | Cannot measure AI effectiveness |
| Finance/Ops | No cost tracking | Budget overruns go undetected |
| Security | No authz denial metrics | Unauthorized access attempts invisible |
| Ops | No audit pipeline health | Silent data loss undetectable |

---

## Requirements

### BR-KA-OBSERVABILITY-001.1: Session Lifecycle Metrics

**MUST**: KA SHALL expose Prometheus metrics tracking investigation session lifecycle.

| Metric | Type | Labels | Business Question |
|---|---|---|---|
| `aiagent_sessions_started_total` | Counter | `signal_name`, `severity` | Investigation throughput by signal type |
| `aiagent_sessions_completed_total` | Counter | `outcome` | Success/failure/cancellation ratio |
| `aiagent_sessions_active` | Gauge | (none) | Current capacity utilization |
| `aiagent_session_duration_seconds` | Histogram | `outcome` | P50/P95/P99 investigation duration SLOs |

**Acceptance Criteria**:
- AC-001-1.1: Metrics exposed at `:9090/metrics`
- AC-001-1.2: `sessions_active` gauge increments on investigation start, decrements on ALL exit paths (success, failure, cancellation, panic)
- AC-001-1.3: `session_duration_seconds` measures goroutine wall-clock time, not HTTP handler time
- AC-001-1.4: `outcome` label reflects final session store status

### BR-KA-OBSERVABILITY-001.2: Investigation Quality Metrics

**MUST**: KA SHALL expose metrics tracking investigation phase outcomes, tool usage, and LLM turn efficiency.

| Metric | Type | Labels | Business Question |
|---|---|---|---|
| `aiagent_investigation_phases_total` | Counter | `phase`, `outcome` | Which phase fails most? |
| `aiagent_investigation_tool_calls_total` | Counter | `tool_name` | Tool usage distribution |
| `aiagent_investigation_turns_total` | Histogram | `phase` | LLM turn efficiency per phase |

**Acceptance Criteria**:
- AC-001-2.1: `phases_total` incremented at RCA, workflow selection, and validation phase completion
- AC-001-2.2: `tool_calls_total` excludes sentinel tools (`submit_result`, `submit_result_with_workflow`, `submit_result_no_workflow`)
- AC-001-2.3: `turns_total` observes actual turn count from `runLLMLoop`

### BR-KA-OBSERVABILITY-001.3: LLM Cost Tracking

**MUST**: KA SHALL expose a cost estimation counter metric per BR-HAPI-195 precedent.

| Metric | Type | Labels | Business Question |
|---|---|---|---|
| `aiagent_llm_cost_dollars_total` | Counter | `model` | Real-time LLM cost tracking |

**Acceptance Criteria**:
- AC-001-3.1: Cost recorded for every LLM call via `chatOrStream` (single instrumentation point)
- AC-001-3.2: Unknown models default to $0 (safe fallback)
- AC-001-3.3: Hardcoded pricing map; ConfigMap-driven pricing deferred to BR-HAPI-195 V2.0

### BR-KA-OBSERVABILITY-001.4: Rate Limiting Metrics

**MUST**: KA SHALL expose a counter tracking rate-limited requests.

| Metric | Type | Labels | Business Question |
|---|---|---|---|
| `aiagent_http_rate_limited_total` | Counter | (none) | Are legitimate users being throttled? |

**Acceptance Criteria**:
- AC-001-4.1: Counter increments inside rate limiter rejection path

### BR-KA-OBSERVABILITY-001.5: HTTP Request Metrics

**MUST**: KA SHALL expose HTTP request latency and concurrency metrics.

| Metric | Type | Labels | Business Question |
|---|---|---|---|
| `aiagent_http_request_duration_seconds` | Histogram | `endpoint`, `method`, `status` | P50/P95/P99 per endpoint |
| `aiagent_http_requests_in_flight` | Gauge | (none) | Current concurrency |

**Acceptance Criteria**:
- AC-001-5.1: `/stream` endpoint excluded from duration histogram (DD-3: SSE connections are long-lived)
- AC-001-5.2: Histogram buckets tuned for API latency: `ExponentialBuckets(0.001, 2, 10)`

### BR-KA-OBSERVABILITY-001.6: Authorization Denial Metrics

**MUST**: KA SHALL expose a counter tracking authorization denials for security monitoring.

| Metric | Type | Labels | Business Question |
|---|---|---|---|
| `aiagent_authz_denied_total` | Counter | `reason` | Unauthorized access attempts |

**Acceptance Criteria**:
- AC-001-6.1: `reason` label values: `owner_mismatch`, `session_not_found`
- AC-001-6.2: Incremented in `getAuthorizedSession` error paths

### BR-KA-OBSERVABILITY-001.7: Audit Pipeline Health Metrics

**MUST**: KA SHALL expose a counter tracking audit event emissions by type.

| Metric | Type | Labels | Business Question |
|---|---|---|---|
| `aiagent_audit_events_emitted_total` | Counter | `event_type` | Audit pipeline throughput |

**Acceptance Criteria**:
- AC-001-7.1: All 17 event types from `AllEventTypes` tracked
- AC-001-7.2: Correlates with `audit_events_dropped_total` for pipeline health monitoring

---

## Design Decisions

| Decision | Rationale |
|---|---|
| DD-METRICS-001 injection pattern | Testability; tests use `NewMetricsWithRegistry` for isolation |
| DD-005 naming convention | `aiagent_` prefix; exported `MetricName*` constants |
| `signal_name` truncated to 128 chars | SEC-1: Attacker-influenced input bounded for Prometheus TSDB safety |
| All `Record*()` methods nil-safe | OPS-1: `if m == nil { return }` prevents panics in 100+ tests |
| Cost metric in `chatOrStream` | DES-2: Single instrumentation point covers all 4 LLM call sites |
| `/stream` excluded from HTTP histogram | DD-3: Long-lived SSE connections would skew P99 to minutes |

---

## Cardinality Budget

Total new time series: ~432 base (plus histogram bucket expansion). DD-005 limit: <10,000 per service. See plan DD-1 for per-metric analysis.

---

## References

- DD-005: Observability Standards
- DD-METRICS-001: Controller Metrics Wiring Pattern
- BR-HAPI-195: LLM Cost Tracking Metrics
- BR-ORCH-044: Operational Observability Metrics

---

**Document Version**: 1.0
**Author**: Kubernaut Development Team
