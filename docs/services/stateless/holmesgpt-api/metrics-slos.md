# HolmesGPT API - Metrics & SLOs

**Version**: v1.0
**Last Updated**: December 3, 2025
**Status**: ✅ Complete
**Metrics Endpoint**: `http://holmesgpt-api.kubernaut-system:8080/metrics`

---

## 📋 Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| v1.1 | 2025-12-03 | Aligned metric names with actual implementation in `src/middleware/metrics.py` | Code audit |
| v1.0 | 2025-12-03 | Initial metrics-slos.md following SERVICE_DOCUMENTATION_GUIDE.md standard | Migrated from `observability/PROMETHEUS_QUERIES.md` |

---

## 📝 Implementation Status

| Metric | Status | Notes |
|--------|--------|-------|
| `holmesgpt_investigations_total` | ✅ Implemented | Labels: method, endpoint, status |
| `holmesgpt_investigations_duration_seconds` | ✅ Implemented | Histogram with standard buckets |
| `holmesgpt_llm_calls_total` | ✅ Implemented | Labels: provider, model, status |
| `holmesgpt_llm_call_duration_seconds` | ✅ Implemented | Histogram |
| `holmesgpt_llm_token_usage_total` | ✅ Implemented | Labels: provider, model, type (prompt/completion) |
| `holmesgpt_auth_failures_total` | ✅ Implemented | Labels: reason, endpoint |
| `holmesgpt_auth_success_total` | ✅ Implemented | Labels: username, role |
| `holmesgpt_context_api_calls_total` | ⚠️ Deprecated | Context API deprecated (DD-CONTEXT-006) |
| `holmesgpt_active_requests` | ✅ Implemented | Gauge |
| `holmesgpt_http_requests_total` | ✅ Implemented | General HTTP metrics |
| `holmesgpt_rfc7807_errors_total` | ✅ Implemented | Labels: status_code, error_type |
| `holmesgpt_investigation_cost_dollars_total` | ❌ Not implemented | Deferred to v2.0 |

---

## 📊 Service Level Indicators (SLIs)

### Availability SLI

**Definition**: Percentage of successful investigation requests

```promql
# Availability SLI (success rate)
sum(rate(holmesgpt_investigations_total{status="success"}[5m]))
/
sum(rate(holmesgpt_investigations_total[5m]))
* 100
```

### Latency SLI

**Definition**: Investigation response time at various percentiles

```promql
# p50 latency
histogram_quantile(0.50, rate(holmesgpt_investigations_duration_seconds_bucket[5m]))

# p95 latency
histogram_quantile(0.95, rate(holmesgpt_investigations_duration_seconds_bucket[5m]))

# p99 latency
histogram_quantile(0.99, rate(holmesgpt_investigations_duration_seconds_bucket[5m]))
```

### Error Rate SLI

**Definition**: Rate of failed investigations

```promql
# Error rate percentage
sum(rate(holmesgpt_investigations_total{status="error"}[5m]))
/
sum(rate(holmesgpt_investigations_total[5m]))
* 100
```

### LLM Success Rate SLI

**Definition**: Percentage of successful LLM API calls

```promql
# LLM call success rate
sum(rate(holmesgpt_llm_calls_total{status="success"}[5m]))
/
sum(rate(holmesgpt_llm_calls_total[5m]))
* 100
```

---

## 🎯 Service Level Objectives (SLOs)

| SLO | Target | Measurement | Window | Business Impact |
|-----|--------|-------------|--------|-----------------|
| **Availability** | ≥99.0% | Success rate | 30d | AIAnalysis receives valid RCA |
| **Latency (p95)** | <5s | Investigation duration | 30d | Fast remediation decisions |
| **Latency (p99)** | <10s | Investigation duration | 30d | Acceptable worst-case |
| **Error Rate** | <1% | Failed investigations | 30d | Reliable AI analysis |
| **LLM Success Rate** | ≥99.5% | LLM API calls | 30d | Stable AI provider |

### SLO Configuration (YAML)

```yaml
slos:
  - name: "HolmesGPT API Availability"
    sli: "sum(rate(holmesgpt_investigations_total{status=~'2..'}[5m])) / sum(rate(holmesgpt_investigations_total[5m]))"
    target: 0.99  # 99%
    window: "30d"
    burn_rate_fast: 14.4  # 1h window
    burn_rate_slow: 6     # 6h window

  - name: "HolmesGPT API P95 Latency"
    sli: "histogram_quantile(0.95, rate(holmesgpt_investigations_duration_seconds_bucket[5m]))"
    target: 5  # 5 seconds
    window: "30d"

  - name: "HolmesGPT API LLM Success Rate"
    sli: "sum(rate(holmesgpt_llm_calls_total{status='success'}[5m])) / sum(rate(holmesgpt_llm_calls_total[5m]))"
    target: 0.995  # 99.5%
    window: "30d"
```

---

## 📈 Prometheus Metrics

### Request Metrics

```promql
# Total requests per second
rate(holmesgpt_investigations_total[5m])

# Requests per second by status
sum by (status) (rate(holmesgpt_investigations_total[5m]))

# Total investigations in last hour
increase(holmesgpt_investigations_total[1h])

# Investigations by priority
sum by (priority) (increase(holmesgpt_investigations_total[1h]))

# Investigations by environment
sum by (environment) (increase(holmesgpt_investigations_total[1h]))
```

### Latency Metrics

```promql
# Latency percentiles
histogram_quantile(0.50, rate(holmesgpt_investigations_duration_seconds_bucket[5m]))  # p50
histogram_quantile(0.95, rate(holmesgpt_investigations_duration_seconds_bucket[5m]))  # p95
histogram_quantile(0.99, rate(holmesgpt_investigations_duration_seconds_bucket[5m]))  # p99

# Average latency (last 5 minutes)
rate(holmesgpt_investigations_duration_seconds_sum[5m])
/
rate(holmesgpt_investigations_duration_seconds_count[5m])

# P95 latency by endpoint
histogram_quantile(0.95,
  sum by (endpoint, le) (rate(holmesgpt_investigations_duration_seconds_bucket[5m]))
)
```

### Error Metrics

```promql
# Error rate per second
rate(holmesgpt_investigations_total{status="error"}[5m])

# Error rate by priority
sum by (priority) (rate(holmesgpt_investigations_total{status="error"}[5m]))
/
sum by (priority) (rate(holmesgpt_investigations_total[5m]))
* 100

# Auth failures per second
rate(holmesgpt_auth_failures_total[5m])

# Auth failures by reason
sum by (reason) (rate(holmesgpt_auth_failures_total[5m]))
```

### Token Usage Metrics

```promql
# Total tokens per minute
rate(holmesgpt_llm_token_usage_total[1m]) * 60

# Input vs output tokens (type: prompt, completion)
sum by (type) (rate(holmesgpt_llm_token_usage_total[5m]))

# Tokens by provider and model
sum by (provider, model) (rate(holmesgpt_llm_token_usage_total[5m]))
```

### Cost Metrics

> ⚠️ **NOT YET IMPLEMENTED** - Cost tracking is planned for v2.0.
> Cost can be derived from token usage: `tokens * cost_per_token`

```promql
# Estimated cost from token usage (manual calculation)
# OpenAI GPT-4: ~$0.03 per 1K prompt tokens, ~$0.06 per 1K completion tokens
# Formula: (prompt_tokens * 0.00003) + (completion_tokens * 0.00006)

# Token-based cost estimation per hour
(sum(rate(holmesgpt_llm_token_usage_total{type="prompt"}[1h])) * 0.00003
+
sum(rate(holmesgpt_llm_token_usage_total{type="completion"}[1h])) * 0.00006) * 3600
```

**Future Implementation** (`holmesgpt_investigation_cost_dollars_total`):
```promql
# These queries will work once cost metric is implemented
# Daily cost
# sum(increase(holmesgpt_investigation_cost_dollars_total[24h]))
```

### LLM Provider Metrics

```promql
# LLM calls per second
rate(holmesgpt_llm_calls_total[5m])

# LLM calls by provider and model
sum by (provider, model) (rate(holmesgpt_llm_calls_total[5m]))

# LLM call p95 latency
histogram_quantile(0.95, rate(holmesgpt_llm_call_duration_seconds_bucket[5m]))

# LLM call failures by provider
sum by (provider) (rate(holmesgpt_llm_calls_total{status="error"}[5m]))
```

---

## 📊 Grafana Dashboard

**Dashboard JSON**: [observability/grafana-dashboard.json](./observability/grafana-dashboard.json)

### Panel Overview

| Panel | Type | Purpose | Query |
|-------|------|---------|-------|
| **Success Rate** | Gauge | Current availability | `holmesgpt_investigations_success_total / holmesgpt_investigations_total` |
| **RPS** | Single Stat | Current requests/sec | `sum(rate(holmesgpt_investigations_total[1m]))` |
| **P95 Latency** | Single Stat | Current latency | `histogram_quantile(0.95, ...)` |
| **Daily Cost** | Single Stat | Cost today | `sum(increase(holmesgpt_investigation_cost_dollars_total[24h]))` |
| **Request Rate** | Time Series | RPS over time by status | `sum by (status) (rate(holmesgpt_investigations_total[5m]))` |
| **Latency Percentiles** | Time Series | p50/p95/p99 over time | `histogram_quantile(...)` |
| **Cost Rate** | Time Series | $/hour over time | `rate(holmesgpt_investigation_cost_dollars_total[5m]) * 3600` |
| **LLM Provider Health** | Table | Success rate by provider | `sum by (provider) (...)` |

---

## 🚨 Alert Rules

### Critical Alerts

```yaml
groups:
  - name: holmesgpt-api-critical
    rules:
      - alert: HolmesGPTHighErrorRate
        expr: |
          sum(rate(holmesgpt_investigations_total{status="error"}[5m]))
          /
          sum(rate(holmesgpt_investigations_total[5m]))
          > 0.05
        for: 5m
        labels:
          severity: critical
          service: holmesgpt-api
        annotations:
          summary: "HolmesGPT API error rate > 5%"
          description: "Error rate {{ $value | humanizePercentage }} exceeds threshold"

      - alert: HolmesGPTHighLatency
        expr: |
          histogram_quantile(0.95, rate(holmesgpt_investigations_duration_seconds_bucket[5m]))
          > 5.0
        for: 10m
        labels:
          severity: critical
          service: holmesgpt-api
        annotations:
          summary: "HolmesGPT API p95 latency > 5s"
          description: "P95 latency {{ $value | humanizeDuration }} exceeds SLO"
```

### Warning Alerts

```yaml
groups:
  - name: holmesgpt-api-warning
    rules:
      - alert: HolmesGPTTokenUsageSpike
        expr: |
          rate(holmesgpt_llm_token_usage_total[1h])
          >
          1.5 * rate(holmesgpt_llm_token_usage_total[24h] offset 1d)
        for: 30m
        labels:
          severity: warning
          service: holmesgpt-api
        annotations:
          summary: "HolmesGPT API token usage spike detected"
          description: "Hourly token usage 50% above daily average"

      - alert: HolmesGPTLLMProviderErrors
        expr: |
          sum by (provider) (rate(holmesgpt_llm_calls_total{status="error"}[5m]))
          /
          sum by (provider) (rate(holmesgpt_llm_calls_total[5m]))
          > 0.01
        for: 10m
        labels:
          severity: warning
          service: holmesgpt-api
        annotations:
          summary: "LLM provider {{ $labels.provider }} error rate elevated"
          description: "Error rate {{ $value | humanizePercentage }}"
```

---

## 🔍 Debugging Queries

### Slow Investigations

```promql
# Investigations taking > 5 seconds (95th percentile check)
histogram_quantile(0.95, rate(holmesgpt_investigations_duration_seconds_bucket[5m]))
> 5.0
```

### Token Usage Anomalies

```promql
# Unusually high token usage (prompt + completion)
rate(holmesgpt_llm_token_usage_total[5m])
>
avg_over_time(rate(holmesgpt_llm_token_usage_total[5m])[1h:5m]) * 2
```

### LLM Provider Health

```promql
# LLM error rate by provider
sum by (provider) (rate(holmesgpt_llm_calls_total{status="error"}[5m]))
/
sum by (provider) (rate(holmesgpt_llm_calls_total[5m]))
> 0.01
```

---

## 📚 References

- [SERVICE_DOCUMENTATION_GUIDE.md](../../SERVICE_DOCUMENTATION_GUIDE.md) - Documentation standard
- [Prometheus Query Language](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [Histogram Quantiles](https://prometheus.io/docs/practices/histograms/)
- [DD-HOLMESGPT-009](../../../architecture/decisions/DD-HOLMESGPT-009-Ultra-Compact-JSON-Format.md) - Token optimization

---

**Document Status**: ✅ Complete
**Migrated From**: `observability/PROMETHEUS_QUERIES.md`
**Standard Compliance**: SERVICE_DOCUMENTATION_GUIDE.md v3.1

