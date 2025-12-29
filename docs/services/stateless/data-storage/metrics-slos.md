# Data Storage Service - Metrics & SLOs

**Version**: 1.0
**Last Updated**: December 4, 2025
**Status**: ✅ CURRENT
**BR Coverage**: BR-STORAGE-019 (Logging and metrics)

---

## Table of Contents

1. [Service Level Indicators (SLIs)](#service-level-indicators-slis)
2. [Service Level Objectives (SLOs)](#service-level-objectives-slos)
3. [Prometheus Metrics](#prometheus-metrics)
4. [Grafana Dashboard](#grafana-dashboard)
5. [Alert Rules](#alert-rules)
6. [Error Budget](#error-budget)

---

## Service Level Indicators (SLIs)

### Write Operations

| SLI | Description | Measurement |
|-----|-------------|-------------|
| **Write Availability** | Percentage of successful writes | `success / total * 100` |
| **Write Latency (p95)** | 95th percentile write duration | `histogram_quantile(0.95, rate(write_duration[5m]))` |
| **Write Latency (p99)** | 99th percentile write duration | `histogram_quantile(0.99, rate(write_duration[5m]))` |

### Query Operations

| SLI | Description | Measurement |
|-----|-------------|-------------|
| **Query Availability** | Percentage of successful queries | `success / total * 100` |
| **Query Latency (p95)** | 95th percentile query duration | `histogram_quantile(0.95, rate(query_duration[5m]))` |
| **Semantic Search Latency** | Vector similarity search duration | `histogram_quantile(0.95, rate(semantic_search[5m]))` |

### Dual-Write Coordination

| SLI | Description | Measurement |
|-----|-------------|-------------|
| **Dual-Write Success** | Both PostgreSQL and Vector DB succeed | `dualwrite_success / dualwrite_total * 100` |
| **Fallback Rate** | Rate of PostgreSQL-only fallback writes | `fallback_total / dualwrite_total * 100` |

---

## Service Level Objectives (SLOs)

### Production SLOs

| Metric | Target | Window | BR Coverage |
|--------|--------|--------|-------------|
| **Write Availability** | 99.9% | 30 days | BR-STORAGE-001, BR-STORAGE-002 |
| **Query Availability** | 99.9% | 30 days | BR-STORAGE-007, BR-STORAGE-012 |
| **Write Latency (p95)** | < 50ms | Rolling 5m | BR-STORAGE-001 |
| **Write Latency (p99)** | < 100ms | Rolling 5m | BR-STORAGE-001 |
| **Query Latency (p95)** | < 250ms | Rolling 5m | BR-STORAGE-007 |
| **Semantic Search (p95)** | < 50ms | Rolling 5m | BR-STORAGE-012 |
| **Dual-Write Success** | 99.9% | 30 days | BR-STORAGE-014 |
| **Fallback Rate** | < 0.1% | 30 days | BR-STORAGE-015 |
| **Cache Hit Rate** | > 80% | Rolling 15m | BR-STORAGE-009 |

### SLO Justification

**Why 99.9% availability?**
- Data Storage Service is a critical path for audit trail persistence
- 99.9% allows ~43 minutes of downtime per month
- Sufficient error budget for deployments and maintenance

**Why < 50ms write latency?**
- Audit writes should not block remediation workflow execution
- p95 target ensures 95% of writes complete quickly
- Allows for occasional slow writes due to GC or network jitter

---

## Prometheus Metrics

### Counter Metrics

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Write operations
    WriteTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "datastorage_write_total",
        Help: "Total write operations by table and status",
    }, []string{"table", "status"})  // status: success, failure

    // Dual-write coordination
    DualWriteSuccessTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "datastorage_dualwrite_success_total",
        Help: "Total successful dual-write operations (PostgreSQL + Vector DB)",
    })

    DualWriteFailureTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "datastorage_dualwrite_failure_total",
        Help: "Total dual-write failures by reason",
    }, []string{"reason"})  // reason: postgresql_failure, vectordb_failure, validation_failure

    FallbackModeTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "datastorage_fallback_mode_total",
        Help: "Total writes using PostgreSQL-only fallback mode",
    })

    // Cache metrics
    CacheHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "datastorage_cache_hits_total",
        Help: "Total embedding cache hits",
    })

    CacheMissesTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "datastorage_cache_misses_total",
        Help: "Total embedding cache misses",
    })

    // Validation metrics
    ValidationFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "datastorage_validation_failures_total",
        Help: "Total validation failures by field and reason",
    }, []string{"field", "reason"})  // reason: required, invalid, length_exceeded, xss_detected

    // Query metrics
    QueryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "datastorage_query_total",
        Help: "Total query operations by operation type and status",
    }, []string{"operation", "status"})  // operation: list, get, semantic_search, filter
)
```

### Histogram Metrics

```go
var (
    // Write duration
    WriteDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "datastorage_write_duration_seconds",
        Help:    "Write operation duration in seconds",
        Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
    }, []string{"table"})

    // Query duration
    QueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "datastorage_query_duration_seconds",
        Help:    "Query operation duration in seconds by operation type",
        Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
    }, []string{"operation"})

    // Embedding generation
    EmbeddingGenerationDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "datastorage_embedding_generation_duration_seconds",
        Help:    "Embedding generation duration in seconds",
        Buckets: []float64{0.05, 0.1, 0.2, 0.5, 1.0, 2.0},
    })
)
```

### Usage Examples

```go
// Record successful write
WriteTotal.WithLabelValues("notification_audit", "success").Inc()
WriteDuration.WithLabelValues("notification_audit").Observe(duration.Seconds())

// Record dual-write failure
DualWriteFailureTotal.WithLabelValues("postgresql_failure").Inc()

// Record cache hit
CacheHitsTotal.Inc()

// Record validation failure
ValidationFailuresTotal.WithLabelValues("remediation_id", "required").Inc()

// Record query
QueryTotal.WithLabelValues("semantic_search", "success").Inc()
QueryDuration.WithLabelValues("semantic_search").Observe(duration.Seconds())
```

---

## Grafana Dashboard

### Dashboard Panels

The Data Storage Service Grafana dashboard includes:

| Panel | Type | Query |
|-------|------|-------|
| **Write Rate** | Graph | `rate(datastorage_write_total[5m])` |
| **Write Latency (p95/p99)** | Graph | `histogram_quantile(0.95/0.99, rate(datastorage_write_duration_seconds_bucket[5m]))` |
| **Dual-Write Success/Failure** | Graph | `rate(datastorage_dualwrite_{success,failure}_total[5m])` |
| **Fallback Mode Rate** | Graph | `rate(datastorage_fallback_mode_total[5m])` |
| **Cache Hit Rate** | Gauge | `cache_hits / (cache_hits + cache_misses)` |
| **Embedding Generation Time** | Graph | `histogram_quantile(0.95, rate(datastorage_embedding_generation_duration_seconds_bucket[5m]))` |
| **Validation Failures** | Table | `rate(datastorage_validation_failures_total[5m]) by (field, reason)` |
| **Query Rate** | Graph | `rate(datastorage_query_total[5m]) by (operation)` |
| **Query Latency** | Graph | `histogram_quantile(0.95, rate(datastorage_query_duration_seconds_bucket[5m])) by (operation)` |
| **Semantic Search Performance** | Graph | `histogram_quantile(0.95, rate(datastorage_query_duration_seconds_bucket{operation="semantic_search"}[5m]))` |
| **Error Rate Overview** | Stat | `sum(rate(datastorage_write_total{status="failure"}[5m])) / sum(rate(datastorage_write_total[5m]))` |
| **Query Error Rate** | Stat | `sum(rate(datastorage_query_total{status="failure"}[5m])) / sum(rate(datastorage_query_total[5m]))` |

### Dashboard Location

**File**: [`observability/grafana-dashboard.json`](./observability/grafana-dashboard.json)

**Import via Grafana UI**:
1. Navigate to Grafana → Dashboards → Import
2. Upload `grafana-dashboard.json`
3. Select Prometheus data source
4. Click "Import"

---

## Alert Rules

### Critical Alerts

```yaml
# High Write Error Rate
- alert: DataStorageHighWriteErrorRate
  expr: |
    100 * (
      sum(rate(datastorage_write_total{status="failure"}[5m]))
      /
      sum(rate(datastorage_write_total[5m]))
    ) > 5
  for: 5m
  labels:
    severity: critical
    service: data-storage
  annotations:
    summary: "Data Storage write error rate > 5%"
    runbook_url: "observability/ALERTING_RUNBOOK.md#datastoragehighwriteerrorrate"

# PostgreSQL Failure
- alert: DataStoragePostgreSQLFailure
  expr: rate(datastorage_dualwrite_failure_total{reason="postgresql_failure"}[5m]) > 0
  for: 1m
  labels:
    severity: critical
    service: data-storage
  annotations:
    summary: "PostgreSQL write failures detected"
    runbook_url: "observability/ALERTING_RUNBOOK.md#datastoragepostgresqlfailure"

# High Query Error Rate
- alert: DataStorageHighQueryErrorRate
  expr: |
    100 * (
      sum(rate(datastorage_query_total{status="failure"}[5m]))
      /
      sum(rate(datastorage_query_total[5m]))
    ) > 5
  for: 5m
  labels:
    severity: critical
    service: data-storage
  annotations:
    summary: "Data Storage query error rate > 5%"
    runbook_url: "observability/ALERTING_RUNBOOK.md#datastoragehighqueryerrorrate"
```

### Warning Alerts

```yaml
# Vector DB Degradation
- alert: DataStorageVectorDBDegraded
  expr: rate(datastorage_fallback_mode_total[5m]) > 0
  for: 5m
  labels:
    severity: warning
    service: data-storage
  annotations:
    summary: "Vector DB unavailable, using PostgreSQL-only fallback"
    runbook_url: "observability/ALERTING_RUNBOOK.md#datastoragevectordbdegraded"

# Low Cache Hit Rate
- alert: DataStorageLowCacheHitRate
  expr: |
    rate(datastorage_cache_hits_total[5m])
    /
    (rate(datastorage_cache_hits_total[5m]) + rate(datastorage_cache_misses_total[5m]))
    < 0.5
  for: 15m
  labels:
    severity: warning
    service: data-storage
  annotations:
    summary: "Embedding cache hit rate < 50%"
    runbook_url: "observability/ALERTING_RUNBOOK.md#datastoragelowcachehitrate"

# Slow Semantic Search
- alert: DataStorageSlowSemanticSearch
  expr: |
    histogram_quantile(0.95, rate(datastorage_query_duration_seconds_bucket{operation="semantic_search"}[5m]))
    > 0.1
  for: 10m
  labels:
    severity: warning
    service: data-storage
  annotations:
    summary: "Semantic search p95 latency > 100ms"
    runbook_url: "observability/ALERTING_RUNBOOK.md#datastoragesemanticsearchslow"
```

---

## Error Budget

### Error Budget Calculation

**Monthly Error Budget (99.9% SLO)**:
- Total minutes per month: 43,200 (30 days)
- Allowed downtime: 43.2 minutes (0.1%)
- Allowed errors: 0.1% of total requests

### Error Budget Queries

```promql
# Write Error Budget Remaining
(1 - (
  sum(rate(datastorage_write_total{status="failure"}[30d]))
  /
  sum(rate(datastorage_write_total[30d]))
)) - 0.999

# Query Error Budget Remaining
(1 - (
  sum(rate(datastorage_query_total{status="failure"}[30d]))
  /
  sum(rate(datastorage_query_total[30d]))
)) - 0.999
```

### Error Budget Alerts

```yaml
# Error Budget Exhausted (Critical)
- alert: DataStorageErrorBudgetExhausted
  expr: |
    (1 - (
      sum(rate(datastorage_write_total{status="failure"}[30d]))
      /
      sum(rate(datastorage_write_total[30d]))
    )) - 0.999 < 0
  for: 1h
  labels:
    severity: critical
    service: data-storage
  annotations:
    summary: "Data Storage write error budget exhausted"

# Error Budget Low (Warning)
- alert: DataStorageErrorBudgetLow
  expr: |
    (1 - (
      sum(rate(datastorage_write_total{status="failure"}[30d]))
      /
      sum(rate(datastorage_write_total[30d]))
    )) - 0.999 < 0.0001
  for: 1h
  labels:
    severity: warning
    service: data-storage
  annotations:
    summary: "Data Storage write error budget < 10% remaining"
```

---

## Metrics Summary

| Category | Metric Count | Cardinality | Performance Impact |
|----------|--------------|-------------|-------------------|
| Counters | 7 | ~50 unique labels | < 1% CPU |
| Histograms | 3 | ~30 unique labels | < 1% CPU |
| **Total** | 11 | ~80 unique labels | < 1% overhead |

**Cardinality Safety**: All metrics are designed with bounded cardinality (< 100 unique label combinations).

---

## Related Documentation

- [observability-logging.md](./observability-logging.md) - Structured logging patterns
- [observability/ALERTING_RUNBOOK.md](./observability/ALERTING_RUNBOOK.md) - Alert troubleshooting
- [observability/PROMETHEUS_QUERIES.md](./observability/PROMETHEUS_QUERIES.md) - Complete query reference
- [observability/DEPLOYMENT_CONFIGURATION.md](./observability/DEPLOYMENT_CONFIGURATION.md) - Setup guide

---

**Document Version**: 1.0
**Changelog**:
- v1.0 (Dec 4, 2025): Initial document - consolidated SLI/SLO and metrics per SERVICE_DOCUMENTATION_GUIDE.md


