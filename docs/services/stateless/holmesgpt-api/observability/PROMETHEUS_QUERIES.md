# HolmesGPT API - Prometheus Queries

**Service**: HolmesGPT API
**Metrics Endpoint**: `http://holmesgpt-api.kubernaut-system:8080/metrics`
**Last Updated**: October 16, 2025

---

## 📊 Investigation Request Metrics

### Request Rate

```promql
# Total requests per second
rate(holmesgpt_investigations_total[5m])

# Requests per second by status
sum by (status) (rate(holmesgpt_investigations_total[5m]))

# Success rate percentage
sum(rate(holmesgpt_investigations_total{status="success"}[5m]))
/
sum(rate(holmesgpt_investigations_total[5m]))
* 100
```

### Request Volume

```promql
# Total investigations in last hour
increase(holmesgpt_investigations_total[1h])

# Total investigations in last 24 hours
increase(holmesgpt_investigations_total[24h])

# Investigations by priority
sum by (priority) (increase(holmesgpt_investigations_total[1h]))

# Investigations by environment
sum by (environment) (increase(holmesgpt_investigations_total[1h]))
```

---

## ⏱️ Latency Metrics

### Latency Percentiles

```promql
# p50 latency
histogram_quantile(0.50,
  rate(holmesgpt_investigation_duration_seconds_bucket[5m])
)

# p95 latency
histogram_quantile(0.95,
  rate(holmesgpt_investigation_duration_seconds_bucket[5m])
)

# p99 latency
histogram_quantile(0.99,
  rate(holmesgpt_investigation_duration_seconds_bucket[5m])
)

# p95 latency by priority
histogram_quantile(0.95,
  sum by (priority, le) (rate(holmesgpt_investigation_duration_seconds_bucket[5m]))
)
```

### Average Latency

```promql
# Average latency (last 5 minutes)
rate(holmesgpt_investigation_duration_seconds_sum[5m])
/
rate(holmesgpt_investigation_duration_seconds_count[5m])

# Average latency by environment
rate(holmesgpt_investigation_duration_seconds_sum[5m])
/
rate(holmesgpt_investigation_duration_seconds_count[5m])
* on(environment) group_left()
sum by (environment) (rate(holmesgpt_investigations_total[5m]))
```

---

## 💰 Token Usage and Cost Metrics

### Token Usage Trends

```promql
# Total tokens per minute
rate(holmesgpt_investigation_tokens_total[1m]) * 60

# Input tokens per minute
rate(holmesgpt_investigation_tokens_total{type="input"}[1m]) * 60

# Output tokens per minute
rate(holmesgpt_investigation_tokens_total{type="output"}[1m]) * 60

# Tokens by provider and model
sum by (provider, model) (rate(holmesgpt_investigation_tokens_total[5m]))

# Token usage distribution (input vs output)
sum by (type) (rate(holmesgpt_investigation_tokens_total[5m]))
```

### Cost Tracking

```promql
# Cost per hour
rate(holmesgpt_investigation_cost_dollars_total[1h]) * 3600

# Daily cost
sum(increase(holmesgpt_investigation_cost_dollars_total[24h]))

# Weekly cost
sum(increase(holmesgpt_investigation_cost_dollars_total[7d]))

# Monthly cost estimate
sum(increase(holmesgpt_investigation_cost_dollars_total[30d]))

# Cost by provider and model
sum by (provider, model) (rate(holmesgpt_investigation_cost_dollars_total[1h]) * 3600)

# Average cost per investigation
rate(holmesgpt_investigation_cost_dollars_total[5m])
/
rate(holmesgpt_investigations_total{status="success"}[5m])
```

### Cost Anomaly Detection

```promql
# Current hourly cost vs yesterday's average
rate(holmesgpt_investigation_cost_dollars_total[1h]) * 3600
>
rate(holmesgpt_investigation_cost_dollars_total[24h] offset 1d) * 3600 * 1.5

# Spike in cost (>50% increase)
(
  rate(holmesgpt_investigation_cost_dollars_total[1h])
  -
  rate(holmesgpt_investigation_cost_dollars_total[1h] offset 1h)
)
/
rate(holmesgpt_investigation_cost_dollars_total[1h] offset 1h)
> 0.5
```

---

## 🔒 Authentication & Security Metrics

### Authentication Failures

```promql
# Auth failures per second
rate(holmesgpt_auth_failures_total[5m])

# Auth failures by reason
sum by (reason) (rate(holmesgpt_auth_failures_total[5m]))

# Total auth failures in last hour
increase(holmesgpt_auth_failures_total[1h])

# Auth failure rate percentage
rate(holmesgpt_auth_failures_total[5m])
/
(rate(holmesgpt_investigations_total[5m]) + rate(holmesgpt_auth_failures_total[5m]))
* 100
```

### Rate Limiting

```promql
# Rate limit hits per second
rate(holmesgpt_rate_limit_hits_total[5m])

# Rate limit hits by client IP
sum by (client_ip) (rate(holmesgpt_rate_limit_hits_total[5m]))

# Top 10 clients hitting rate limits
topk(10,
  sum by (client_ip) (increase(holmesgpt_rate_limit_hits_total[1h]))
)

# Total rate limit hits in last hour
increase(holmesgpt_rate_limit_hits_total[1h])
```

---

## 🤖 LLM Provider Metrics

### LLM Call Statistics

```promql
# LLM calls per second
rate(holmesgpt_llm_calls_total[5m])

# LLM calls by provider and model
sum by (provider, model) (rate(holmesgpt_llm_calls_total[5m]))

# LLM call success rate
sum(rate(holmesgpt_llm_calls_total{status="success"}[5m]))
/
sum(rate(holmesgpt_llm_calls_total[5m]))
* 100

# LLM call failures by provider
sum by (provider) (rate(holmesgpt_llm_calls_total{status="error"}[5m]))
```

### LLM Call Latency

```promql
# LLM call p95 latency
histogram_quantile(0.95,
  rate(holmesgpt_llm_call_duration_seconds_bucket[5m])
)

# LLM call p95 latency by provider
histogram_quantile(0.95,
  sum by (provider, le) (rate(holmesgpt_llm_call_duration_seconds_bucket[5m]))
)

# Average LLM call duration
rate(holmesgpt_llm_call_duration_seconds_sum[5m])
/
rate(holmesgpt_llm_call_duration_seconds_count[5m])
```

---

## 🔧 Context API Integration Metrics

### Context API Call Statistics

```promql
# Context API calls per second
rate(holmesgpt_context_api_calls_total[5m])

# Context API calls by tool name
sum by (tool_name) (rate(holmesgpt_context_api_calls_total[5m]))

# Context API success rate
sum(rate(holmesgpt_context_api_calls_total{status="success"}[5m]))
/
sum(rate(holmesgpt_context_api_calls_total[5m]))
* 100

# Context API failures
sum by (tool_name) (rate(holmesgpt_context_api_calls_total{status="error"}[5m]))
```

---

## 🚨 Error Rate Metrics

### Overall Error Rate

```promql
# Error rate per second
rate(holmesgpt_investigations_total{status="error"}[5m])

# Error rate percentage
sum(rate(holmesgpt_investigations_total{status="error"}[5m]))
/
sum(rate(holmesgpt_investigations_total[5m]))
* 100

# Error rate by priority
sum by (priority) (rate(holmesgpt_investigations_total{status="error"}[5m]))
/
sum by (priority) (rate(holmesgpt_investigations_total[5m]))
* 100

# Errors in last hour
increase(holmesgpt_investigations_total{status="error"}[1h])
```

---

## 📈 Business Metrics

### Investigation Volume by Priority

```promql
# P0 investigations per hour
increase(holmesgpt_investigations_total{priority="P0"}[1h])

# P1 investigations per hour
increase(holmesgpt_investigations_total{priority="P1"}[1h])

# Priority distribution
sum by (priority) (increase(holmesgpt_investigations_total[1h]))
```

### Environment-Based Metrics

```promql
# Production investigations per hour
increase(holmesgpt_investigations_total{environment="production"}[1h])

# Staging investigations per hour
increase(holmesgpt_investigations_total{environment="staging"}[1h])

# Environment distribution
sum by (environment) (increase(holmesgpt_investigations_total[1h]))
```

---

## 🎯 SLI/SLO Monitoring

### Availability SLI

```promql
# Service availability (success rate > 99%)
sum(rate(holmesgpt_investigations_total{status="success"}[5m]))
/
sum(rate(holmesgpt_investigations_total[5m]))
> 0.99
```

### Latency SLI

```promql
# Latency SLI: 95% of requests < 2.5 seconds
histogram_quantile(0.95,
  rate(holmesgpt_investigation_duration_seconds_bucket[5m])
) < 2.5
```

### Error Budget

```promql
# Error budget consumption (monthly)
1 - (
  sum(increase(holmesgpt_investigations_total{status="success"}[30d]))
  /
  sum(increase(holmesgpt_investigations_total[30d]))
)
```

---

## 🔍 Debugging Queries

### Slow Investigations

```promql
# Investigations taking > 5 seconds
rate(holmesgpt_investigation_duration_seconds_count{le="5"}[5m])
/
rate(holmesgpt_investigation_duration_seconds_count{le="+Inf"}[5m])
< 0.95
```

### High-Cost Investigations

```promql
# Cost spike detection
rate(holmesgpt_investigation_cost_dollars_total[5m])
>
avg_over_time(rate(holmesgpt_investigation_cost_dollars_total[5m])[1h:5m]) * 1.5
```

### Token Count Anomalies

```promql
# Unusually high token usage
rate(holmesgpt_investigation_tokens_total[5m])
>
avg_over_time(rate(holmesgpt_investigation_tokens_total[5m])[1h:5m]) * 2
```

---

## 📊 Dashboard Query Examples

### Single Stat Panels

```promql
# Current RPS
sum(rate(holmesgpt_investigations_total[1m]))

# Success rate (last 5min)
sum(rate(holmesgpt_investigations_total{status="success"}[5m]))
/
sum(rate(holmesgpt_investigations_total[5m]))
* 100

# P95 latency
histogram_quantile(0.95, rate(holmesgpt_investigation_duration_seconds_bucket[5m]))

# Daily cost (so far)
sum(increase(holmesgpt_investigation_cost_dollars_total[24h]))
```

### Time Series Panels

```promql
# Request rate over time
sum(rate(holmesgpt_investigations_total[5m])) by (status)

# Latency percentiles over time
histogram_quantile(0.50, rate(holmesgpt_investigation_duration_seconds_bucket[5m])) # p50
histogram_quantile(0.95, rate(holmesgpt_investigation_duration_seconds_bucket[5m])) # p95
histogram_quantile(0.99, rate(holmesgpt_investigation_duration_seconds_bucket[5m])) # p99

# Cost rate over time
rate(holmesgpt_investigation_cost_dollars_total[5m]) * 3600  # $/hour
```

---

## 🚨 Alert Query Examples

### High Error Rate Alert

```promql
# Trigger when error rate > 5% for 5 minutes
sum(rate(holmesgpt_investigations_total{status="error"}[5m]))
/
sum(rate(holmesgpt_investigations_total[5m]))
> 0.05
```

### High Latency Alert

```promql
# Trigger when p95 latency > 5 seconds for 10 minutes
histogram_quantile(0.95,
  rate(holmesgpt_investigation_duration_seconds_bucket[5m])
) > 5.0
```

### Cost Anomaly Alert

```promql
# Trigger when hourly cost 50% above daily average
rate(holmesgpt_investigation_cost_dollars_total[1h])
>
1.5 * rate(holmesgpt_investigation_cost_dollars_total[24h] offset 1d)
```

---

## 📚 References

- Prometheus Query Language: https://prometheus.io/docs/prometheus/latest/querying/basics/
- Histogram Quantiles: https://prometheus.io/docs/practices/histograms/
- Recording Rules: https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/
- Alert Rules: https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/


