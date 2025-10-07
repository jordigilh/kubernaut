# Gateway Service - Metrics & SLOs

**Version**: v1.0
**Last Updated**: October 4, 2025
**Status**: ✅ Design Complete

---

## Prometheus Metrics

### Alert Ingestion Metrics

```go
var (
    alertsReceivedTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_alerts_received_total",
            Help: "Total alerts received by source and severity",
        },
        []string{"source", "severity", "environment"},
    )

    alertsDeduplicatedTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_alerts_deduplicated_total",
            Help: "Total alerts deduplicated",
        },
        []string{"alertname", "environment"},
    )

    alertStormsDetectedTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_alert_storms_detected_total",
            Help: "Total alert storms detected",
        },
        []string{"storm_type", "alertname"},
    )
)
```

### CRD Creation Metrics

```go
var (
    alertRemediationCreatedTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_remediationrequest_created_total",
            Help: "Total RemediationRequest CRDs created",
        },
        []string{"environment", "priority"},
    )

    alertRemediationCreationFailuresTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_remediationrequest_creation_failures_total",
            Help: "Total RemediationRequest CRD creation failures",
        },
        []string{"error_type"},
    )
)
```

### Performance Metrics

```go
var (
    httpRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "gateway_http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
        },
        []string{"endpoint", "method", "status"},
    )

    redisOperationDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "gateway_redis_operation_duration_seconds",
            Help:    "Redis operation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.0001, 2, 10), // 0.1ms to ~100ms
        },
        []string{"operation"},
    )
)
```

### Deduplication Metrics

```go
var (
    deduplicationCacheHitsTotal = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "gateway_deduplication_cache_hits_total",
            Help: "Total deduplication cache hits",
        },
    )

    deduplicationRate = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "gateway_deduplication_rate",
            Help: "Current deduplication rate (percentage)",
        },
    )
)
```

---

## Grafana Dashboard

### Key Panels

**1. Alert Ingestion Rate**
```promql
rate(gateway_alerts_received_total[5m])
```

**2. Deduplication Rate**
```promql
gateway_deduplication_rate
```

**3. Storm Detection Events**
```promql
rate(gateway_alert_storms_detected_total[5m])
```

**4. CRD Creation Success Rate**
```promql
rate(gateway_remediationrequest_created_total[5m]) / 
(rate(gateway_remediationrequest_created_total[5m]) + 
 rate(gateway_remediationrequest_creation_failures_total[5m]))
```

**5. API Latency (P50/P95/P99)**
```promql
histogram_quantile(0.50, rate(gateway_http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(gateway_http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(gateway_http_request_duration_seconds_bucket[5m]))
```

**6. Redis Performance**
```promql
histogram_quantile(0.95, rate(gateway_redis_operation_duration_seconds_bucket[5m]))
```

---

## SLO Definitions

### Availability SLO

**Target**: 99.9% availability
**Measurement**: Successful HTTP 202 responses / total requests
**Query**:
```promql
sum(rate(gateway_http_request_duration_seconds_count{status="202"}[5m])) / 
sum(rate(gateway_http_request_duration_seconds_count[5m]))
```

### Latency SLO

**Target**: p95 < 50ms, p99 < 100ms
**Query**:
```promql
histogram_quantile(0.95, rate(gateway_http_request_duration_seconds_bucket[5m])) < 0.050
histogram_quantile(0.99, rate(gateway_http_request_duration_seconds_bucket[5m])) < 0.100
```

### Error Rate SLO

**Target**: < 0.1% error rate
**Query**:
```promql
sum(rate(gateway_http_request_duration_seconds_count{status=~"5.."}[5m])) / 
sum(rate(gateway_http_request_duration_seconds_count[5m])) < 0.001
```

---

## Alert Rules

```yaml
groups:
  - name: gateway-service
    rules:
    # High CRD creation failure rate
    - alert: GatewayHighCRDCreationFailureRate
      expr: |
        rate(gateway_remediationrequest_creation_failures_total[5m]) > 0.1
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "Gateway CRD creation failure rate >10%"

    # High API latency
    - alert: GatewayHighLatency
      expr: |
        histogram_quantile(0.95, rate(gateway_http_request_duration_seconds_bucket[5m])) > 0.100
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "Gateway API p95 latency >100ms"

    # High Redis latency
    - alert: GatewayHighRedisLatency
      expr: |
        histogram_quantile(0.95, rate(gateway_redis_operation_duration_seconds_bucket[5m])) > 0.050
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "Gateway Redis p95 latency >50ms"

    # High rate limit rejections
    - alert: GatewayHighRateLimitRejections
      expr: |
        rate(gateway_rate_limit_exceeded_total[5m]) > 10
      for: 2m
      labels:
        severity: warning
      annotations:
        summary: "Gateway rate limit rejections >10/s"
```

**Confidence**: 95%
