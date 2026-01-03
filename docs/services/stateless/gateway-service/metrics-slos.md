# Gateway Service - Metrics & SLOs

**Version**: v1.1
**Last Updated**: January 3, 2026
**Status**: ✅ Production (Redis Removed per DD-GATEWAY-012)

**Changelog**:
- **2026-01-03**: Added performance observability metrics (Option A: conflict resolution, field index)
- **2026-01-03**: Added circuit breaker metrics (Option B: K8s API resilience)
- **2026-01-03**: Circuit breaker implementation using github.com/sony/gobreaker
- **2025-12-21**: Redis metrics removed (Gateway no longer uses Redis)
- **2025-12-21**: Metric names updated to match actual implementation
- **2025-12-21**: DD-005 V3.0 metric constants applied

---

## Prometheus Metrics

### Signal Ingestion Metrics

```go
// BR-GATEWAY-066: Signal ingestion metrics
var (
    signalsReceivedTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_signals_received_total",
            Help: "Total signals received by source type and severity",
        },
        []string{"source_type", "severity"},
    )

    signalsDeduplicatedTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_signals_deduplicated_total",
            Help: "Total signals deduplicated",
        },
        []string{"signal_name"},
    )
)
```

### CRD Creation Metrics

```go
// BR-GATEWAY-068: CRD creation metrics
var (
    crdsCreatedTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_crds_created_total",
            Help: "Total RemediationRequest CRDs created by source type",
        },
        []string{"source_type", "status"},
    )

    crdCreationErrorsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_crd_creation_errors_total",
            Help: "Total CRD creation errors by error type",
        },
        []string{"error_type"},
    )
)
```

### Performance Metrics

```go
// BR-GATEWAY-067, BR-GATEWAY-079: Performance metrics
var (
    httpRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "gateway_http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
        },
        []string{"endpoint", "method", "status"},
    )
)
```

### Deduplication Metrics

```go
// BR-GATEWAY-069: Deduplication metrics
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

### Conflict Resolution Metrics (Option A: Performance Observability)

```go
// Track optimistic concurrency control performance for status updates
var (
    conflictRetriesTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_conflict_retries_total",
            Help: "Total K8s optimistic concurrency retry attempts by operation and error type",
        },
        []string{"operation", "error_type"},
    )

    conflictResolutionLatency = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "gateway_conflict_resolution_duration_seconds",
            Help:    "Latency for K8s conflict resolution including retries (seconds)",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
        },
        []string{"operation"},
    )
)
```

### Field Index Performance Metrics (Option A: Performance Observability)

```go
// Track O(1) field index query performance for deduplication lookups
var (
    fieldIndexQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "gateway_field_index_query_duration_seconds",
            Help:    "Field index query duration for deduplication lookups (seconds)",
            Buckets: prometheus.ExponentialBuckets(0.0001, 2, 10), // 0.1ms to ~100ms
        },
        []string{"index_name"},
    )
)
```

### Circuit Breaker Metrics (Option B: K8s API Resilience)

```go
// Track K8s API circuit breaker state and operation results
var (
    circuitBreakerState = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "gateway_circuit_breaker_state",
            Help: "Circuit breaker state (0=closed, 1=half-open, 2=open)",
        },
        []string{"name"},
    )

    circuitBreakerOperations = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_circuit_breaker_operations_total",
            Help: "Total operations through circuit breaker by name and result",
        },
        []string{"name", "result"},
    )
)
```

---

## Grafana Dashboard

### Key Panels

**1. Signal Ingestion Rate**
```promql
rate(gateway_signals_received_total[5m])
```

**2. Deduplication Rate**
```promql
gateway_deduplication_rate
```

**3. High Occurrence Count Signals (Persistent Issues)**
```promql
# Signals with high occurrence count (persistent issues)
count(kube_customresource_remediation_request_status_deduplication_occurrence_count >= 5)
```

**4. CRD Creation Success Rate**
```promql
rate(gateway_crds_created_total{status="success"}[5m]) /
(rate(gateway_crds_created_total[5m]) +
 rate(gateway_crd_creation_errors_total[5m]))
```

**5. API Latency (P50/P95/P99)**
```promql
histogram_quantile(0.50, rate(gateway_http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(gateway_http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(gateway_http_request_duration_seconds_bucket[5m]))
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
        rate(gateway_crd_creation_errors_total[5m]) > 0.1
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

    # Low deduplication rate
    - alert: GatewayLowDeduplicationRate
      expr: |
        gateway_deduplication_rate < 0.5
      for: 10m
      labels:
        severity: info
      annotations:
        summary: "Gateway deduplication rate <50%"
```

---

## DD-005 V3.0 Compliance

**Status**: ✅ Fully Compliant

All metric names are defined as exported constants in `pkg/gateway/metrics/metrics.go`:

- `MetricNameSignalsReceivedTotal = "gateway_signals_received_total"`
- `MetricNameSignalsDeduplicatedTotal = "gateway_signals_deduplicated_total"`
- `MetricNameCRDsCreatedTotal = "gateway_crds_created_total"`
- `MetricNameCRDCreationErrorsTotal = "gateway_crd_creation_errors_total"`
- `MetricNameHTTPRequestDuration = "gateway_http_request_duration_seconds"`
- `MetricNameDeduplicationCacheHitsTotal = "gateway_deduplication_cache_hits_total"`
- `MetricNameDeduplicationRate = "gateway_deduplication_rate"`

**Confidence**: 100%
