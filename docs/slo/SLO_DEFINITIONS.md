# SLO Definitions — kubernaut API Frontend

**Version:** 1.0
**Date:** 2026-05-06
**Status:** Aspirational (pre-validation)

## Overview

These SLOs are aspirational targets to be validated once the service is running under production-like load. They drive alerting rules in `deploy/prometheus-rules.yaml` and histogram bucket selection in `internal/metrics/metrics.go`.

## SLO Targets

| SLO ID | Category | Target | Metric | Measurement |
|--------|----------|--------|--------|-------------|
| SLO-1 | HTTP Request Latency (p95) | < 500ms | `af_http_request_duration_seconds` | `histogram_quantile(0.95, rate(...[5m]))` |
| SLO-2 | HTTP Request Latency (p99) | < 1s | `af_http_request_duration_seconds` | `histogram_quantile(0.99, rate(...[5m]))` |
| SLO-3 | Tool Call Latency (native CRD, p99) | < 500ms | `af_tool_call_duration_seconds{type="crd"}` | `histogram_quantile(0.99, rate(...[5m]))` |
| SLO-4 | Tool Call Latency (proxied, p99) | < 2s | `af_tool_call_duration_seconds{type="proxy"}` | `histogram_quantile(0.99, rate(...[5m]))` |
| SLO-5 | Authentication Latency (p99) | < 200ms | `af_auth_duration_seconds` | `histogram_quantile(0.99, rate(...[5m]))` |
| SLO-6 | Availability (5xx rate) | < 0.1% | `af_http_requests_total{status=~"5.."}` | `sum(rate({status=~"5.."}[5m])) / sum(rate(total[5m]))` |
| SLO-7 | Agent Card Response (p99) | < 50ms | `af_http_request_duration_seconds{path="/.well-known/agent-card.json"}` | `histogram_quantile(0.99, rate(...[5m]))` |
| SLO-8 | Severity Triage (Tier 1-2, p95) | < 500ms | `af_severity_triage_duration_seconds{tier=~"1\|1.5\|2"}` | `histogram_quantile(0.95, rate(...[5m]))` |
| SLO-9 | Severity Triage (Tier 3 LLM, p95) | < 15s | `af_severity_triage_duration_seconds{tier="3"}` | `histogram_quantile(0.95, rate(...[5m]))` |

## Histogram Bucket Alignment

The API Frontend uses `prometheus.DefBuckets` for all histogram metrics:

```
DefBuckets = {.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
```

### Alignment Analysis

| SLO | Threshold | Nearest Bucket Boundary | Aligned? |
|-----|-----------|------------------------|----------|
| SLO-1 (p95 < 500ms) | 0.5s | 0.5 (exact) | Yes |
| SLO-2 (p99 < 1s) | 1.0s | 1.0 (exact) | Yes |
| SLO-3 (native CRD p99 < 500ms) | 0.5s | 0.5 (exact) | Yes |
| SLO-4 (proxied p99 < 2s) | 2.0s | 2.5 (within 25%) | Yes |
| SLO-5 (auth p99 < 200ms) | 0.2s | 0.25 (within 25%) | Yes |
| SLO-7 (agent card p99 < 50ms) | 0.05s | 0.05 (exact) | Yes |

**Conclusion:** `prometheus.DefBuckets` provides sufficient resolution at all SLO thresholds. No custom bucket configuration is required.

### Rationale for Keeping DefBuckets

1. All SLO thresholds fall on or near a DefBucket boundary
2. DefBuckets are well-understood by SRE teams and Grafana dashboards
3. Custom buckets increase cardinality and complicate cross-service comparison
4. If future measurement shows DefBuckets are insufficient, custom buckets can be added without breaking changes (additive only)

## Alerting Rules

Alerting rules are defined in `deploy/prometheus-rules.yaml`. Each alert is derived from an SLO:

| Alert | Derived From | Condition | Severity |
|-------|-------------|-----------|----------|
| `ApifrontendDown` | SLO-6 | `up{job="apifrontend"} == 0` for 5m | critical |
| `ApifrontendHighLatencyP95` | SLO-1 | p95 > 500ms for 2m | warning |
| `ApifrontendHighLatencyP99` | SLO-2 | p99 > 1s for 2m | critical |
| `ApifrontendHighErrorRateWarning` | SLO-6 | 5xx > 0.5% for 2m | warning |
| `ApifrontendHighErrorRate` | SLO-6 | 5xx > 1% for 2m | critical |
| `ApifrontendAuthFailureSpike` | SLO-5 | Auth failure rate > 10% for 2m | critical |
| `ApifrontendCircuitBreakerOpen` | Operational | CB open for > 2m | warning |
| `ApifrontendAuditBufferOverflow` | FedRAMP AU-2 | overflow rate > 0 for 1m | critical |
| `ApifrontendToolLatencyHigh` | SLO-4 | Tool P99 > 5s for 5m | warning |
| `ApifrontendDependencyLatencyHigh` | Operational | Dep P95 > 2s for 5m | warning |
| `ApifrontendRateLimitStorm` | Operational | Rejections > 10/s for 2m | warning |
| `ApifrontendSSEConnectionsHigh` | Operational | Active SSE > 100 for 5m | warning |
| `ApifrontendSeverityTriageErrorRate` | SLO-8/9 | Triage error rate > 10% for 5m | warning |

### Triage Latency Impact on SLO-3 (CRD Tool p99)

The `af_create_rr` tool path now includes severity triage when no severity is provided. This adds latency:
- **Tier 1** (Prometheus `/api/v1/alerts`): ~50ms — within SLO-3 budget
- **Tier 1.5/2** (cached rules + instant query): ~100-200ms — within SLO-3 budget
- **Tier 2.5/3** (LLM fallback): 2-15s — **exceeds** SLO-3; measured separately by SLO-8/9

When triage falls to LLM tiers, the tool call will exceed SLO-3's 500ms target. This is expected and tracked independently. The `af_severity_triage_duration_seconds` histogram isolates triage latency from the overall tool latency.

## Validation Plan

1. **Phase 1 (this PR):** Define targets, document alignment, create alerting rules
2. **Phase 2 (post-deploy):** Measure baselines under normal load
3. **Phase 3 (tuning):** Adjust targets based on measured P50/P95/P99
4. **Phase 4 (burn-rate):** Implement multi-window burn-rate alerts once baseline data exists

## Dependencies

- `internal/metrics/metrics.go` — metric definitions (all use `af_` prefix)
  - `af_tool_call_duration_seconds` → `Registry.ToolCallDuration` (line ~74)
  - `af_circuit_breaker_state` → `Registry.CircuitBreakerState` (line ~95)
- Issue #11 — observability implementation
- Issue #43 — performance testing to validate targets
