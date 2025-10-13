# Data Storage Service - Prometheus Query Reference

**Date**: October 13, 2025
**Service**: Data Storage Service
**BR Coverage**: BR-STORAGE-019 (Logging and metrics)

---

## Overview

This document provides a comprehensive reference for Prometheus queries to monitor the Data Storage Service. All queries follow best practices for performance and cardinality protection.

---

## Table of Contents

1. [Write Operations](#write-operations)
2. [Dual-Write Coordination](#dual-write-coordination)
3. [Embedding and Caching](#embedding-and-caching)
4. [Validation](#validation)
5. [Query Operations](#query-operations)
6. [Error Rates and SLIs](#error-rates-and-slis)
7. [Performance Analysis](#performance-analysis)
8. [Cardinality Monitoring](#cardinality-monitoring)

---

## Write Operations

### Write Success Rate (by Table)

**Query**:
```promql
rate(datastorage_write_total{status="success"}[5m])
```

**Use Case**: Monitor write throughput by table
**BR Coverage**: BR-STORAGE-001, BR-STORAGE-002

### Write Error Rate

**Query**:
```promql
rate(datastorage_write_total{status="failure"}[5m])
```

**Use Case**: Detect write failures
**Alert Threshold**: > 1 ops/sec

### Write Success Percentage

**Query**:
```promql
100 * (
  sum(rate(datastorage_write_total{status="success"}[5m]))
  /
  sum(rate(datastorage_write_total[5m]))
)
```

**Use Case**: Calculate write SLI
**Target**: > 99%

### Write Duration - p95 Latency

**Query**:
```promql
histogram_quantile(0.95, rate(datastorage_write_duration_seconds_bucket[5m]))
```

**Use Case**: Monitor write performance
**Target**: < 50ms for p95

### Write Duration - p99 Latency

**Query**:
```promql
histogram_quantile(0.99, rate(datastorage_write_duration_seconds_bucket[5m]))
```

**Use Case**: Detect performance outliers
**Target**: < 100ms for p99

### Average Write Duration

**Query**:
```promql
rate(datastorage_write_duration_seconds_sum[5m])
/
rate(datastorage_write_duration_seconds_count[5m])
```

**Use Case**: Monitor average write latency
**Target**: < 25ms average

---

## Dual-Write Coordination

### Dual-Write Success Rate

**Query**:
```promql
rate(datastorage_dualwrite_success_total[5m])
```

**Use Case**: Monitor successful PostgreSQL + Vector DB writes
**BR Coverage**: BR-STORAGE-014

### Dual-Write Failure Rate (by Reason)

**Query**:
```promql
rate(datastorage_dualwrite_failure_total[5m]) by (reason)
```

**Use Case**: Identify failure causes
**Reasons**: `postgresql_failure`, `vectordb_failure`, `validation_failure`, `context_canceled`, `transaction_rollback`

### Dual-Write Success Percentage

**Query**:
```promql
100 * (
  rate(datastorage_dualwrite_success_total[5m])
  /
  (rate(datastorage_dualwrite_success_total[5m]) + sum(rate(datastorage_dualwrite_failure_total[5m])))
)
```

**Use Case**: Calculate dual-write SLI
**Target**: > 99.9%

### Fallback Mode Rate

**Query**:
```promql
rate(datastorage_fallback_mode_total[5m])
```

**Use Case**: Monitor Vector DB degradation
**BR Coverage**: BR-STORAGE-015
**Alert Threshold**: > 0 (indicates Vector DB issues)

### PostgreSQL Failure Rate

**Query**:
```promql
rate(datastorage_dualwrite_failure_total{reason="postgresql_failure"}[5m])
```

**Use Case**: Detect PostgreSQL issues
**Alert Threshold**: > 0

### Vector DB Failure Rate

**Query**:
```promql
rate(datastorage_dualwrite_failure_total{reason="vectordb_failure"}[5m])
```

**Use Case**: Detect Vector DB issues
**Alert Threshold**: > 0

---

## Embedding and Caching

### Cache Hit Rate

**Query**:
```promql
rate(datastorage_cache_hits_total[5m])
/
(rate(datastorage_cache_hits_total[5m]) + rate(datastorage_cache_misses_total[5m]))
```

**Use Case**: Monitor embedding cache effectiveness
**BR Coverage**: BR-STORAGE-009
**Target**: > 80%

### Cache Miss Rate

**Query**:
```promql
rate(datastorage_cache_misses_total[5m])
```

**Use Case**: Monitor embedding generation load
**Alert Threshold**: > 10 ops/sec (indicates high cache churn)

### Embedding Generation Duration - p95

**Query**:
```promql
histogram_quantile(0.95, rate(datastorage_embedding_generation_duration_seconds_bucket[5m]))
```

**Use Case**: Monitor embedding generation performance
**BR Coverage**: BR-STORAGE-008
**Target**: < 200ms for p95

### Embedding Generation Rate

**Query**:
```promql
rate(datastorage_embedding_generation_duration_seconds_count[5m])
```

**Use Case**: Monitor embedding generation throughput

### Cache Hit vs Miss Comparison

**Query (for visualization)**:
```promql
# Cache Hits
rate(datastorage_cache_hits_total[5m])

# Cache Misses
rate(datastorage_cache_misses_total[5m])
```

**Use Case**: Compare cache performance trends

---

## Validation

### Validation Failure Rate

**Query**:
```promql
rate(datastorage_validation_failures_total[5m])
```

**Use Case**: Monitor input validation failures
**BR Coverage**: BR-STORAGE-010, BR-STORAGE-011

### Validation Failures by Field

**Query**:
```promql
rate(datastorage_validation_failures_total[5m]) by (field)
```

**Use Case**: Identify problematic fields
**Alert Threshold**: > 5 failures/sec for any single field

### Validation Failures by Reason

**Query**:
```promql
rate(datastorage_validation_failures_total[5m]) by (reason)
```

**Use Case**: Identify validation issue types
**Reasons**: `required`, `invalid`, `length_exceeded`, `xss_detected`, `sql_injection_detected`

### Top Validation Failures

**Query**:
```promql
topk(5, sum(rate(datastorage_validation_failures_total[5m])) by (field, reason))
```

**Use Case**: Identify most common validation issues

### Validation Failure Percentage

**Query**:
```promql
100 * (
  sum(rate(datastorage_validation_failures_total[5m]))
  /
  sum(rate(datastorage_write_total[5m]))
)
```

**Use Case**: Calculate validation failure rate
**Target**: < 5%

### Security-Related Validation Failures

**Query**:
```promql
rate(datastorage_validation_failures_total{reason=~"xss_detected|sql_injection_detected"}[5m])
```

**Use Case**: Detect potential security threats
**BR Coverage**: BR-STORAGE-011
**Alert Threshold**: > 0 (investigate immediately)

---

## Query Operations

### Query Success Rate (by Operation)

**Query**:
```promql
rate(datastorage_query_total{status="success"}[5m]) by (operation)
```

**Use Case**: Monitor query throughput
**BR Coverage**: BR-STORAGE-007, BR-STORAGE-012, BR-STORAGE-013

### Query Error Rate

**Query**:
```promql
rate(datastorage_query_total{status="failure"}[5m]) by (operation)
```

**Use Case**: Detect query failures

### Query Duration - p95 (by Operation)

**Query**:
```promql
histogram_quantile(0.95, rate(datastorage_query_duration_seconds_bucket[5m])) by (operation)
```

**Use Case**: Monitor query performance
**Targets**:
- `list`: < 10ms
- `get`: < 5ms
- `semantic_search`: < 50ms
- `filter`: < 20ms

### Semantic Search Performance

**Query**:
```promql
histogram_quantile(0.95, rate(datastorage_query_duration_seconds_bucket{operation="semantic_search"}[5m]))
```

**Use Case**: Monitor HNSW index performance
**BR Coverage**: BR-STORAGE-012
**Target**: < 50ms for p95

### Slow Query Detection (p99)

**Query**:
```promql
histogram_quantile(0.99, rate(datastorage_query_duration_seconds_bucket[5m])) by (operation) > 0.1
```

**Use Case**: Detect slow queries (> 100ms)
**Alert Threshold**: Any operation with p99 > 100ms

### Query Success Percentage

**Query**:
```promql
100 * (
  sum(rate(datastorage_query_total{status="success"}[5m]))
  /
  sum(rate(datastorage_query_total[5m]))
)
```

**Use Case**: Calculate query SLI
**Target**: > 99%

---

## Error Rates and SLIs

### Overall Error Rate

**Query**:
```promql
(
  sum(rate(datastorage_write_total{status="failure"}[5m])) +
  sum(rate(datastorage_query_total{status="failure"}[5m]))
)
/
(
  sum(rate(datastorage_write_total[5m])) +
  sum(rate(datastorage_query_total[5m]))
)
```

**Use Case**: Overall service health SLI
**Target**: < 1% error rate

### Write SLI (99.9% target)

**Query**:
```promql
100 * (
  sum(rate(datastorage_write_total{status="success"}[30d]))
  /
  sum(rate(datastorage_write_total[30d]))
)
```

**Use Case**: 30-day write SLI
**Target**: > 99.9%

### Query SLI (99.9% target)

**Query**:
```promql
100 * (
  sum(rate(datastorage_query_total{status="success"}[30d]))
  /
  sum(rate(datastorage_query_total[30d]))
)
```

**Use Case**: 30-day query SLI
**Target**: > 99.9%

### Error Budget Remaining (Write)

**Query**:
```promql
(1 - (
  sum(rate(datastorage_write_total{status="failure"}[30d]))
  /
  sum(rate(datastorage_write_total[30d]))
)) - 0.999
```

**Use Case**: Calculate remaining error budget (0.1% allowed)
**Alert**: < 0 (error budget exhausted)

---

## Performance Analysis

### Write Throughput Over Time

**Query**:
```promql
sum(rate(datastorage_write_total[5m]))
```

**Use Case**: Monitor overall write load

### Query Throughput Over Time

**Query**:
```promql
sum(rate(datastorage_query_total[5m]))
```

**Use Case**: Monitor overall query load

### Write Duration Distribution (Heatmap)

**Query**:
```promql
rate(datastorage_write_duration_seconds_bucket[5m])
```

**Use Case**: Visualize write latency distribution in Grafana heatmap

### Query Duration Distribution (Heatmap)

**Query**:
```promql
rate(datastorage_query_duration_seconds_bucket[5m])
```

**Use Case**: Visualize query latency distribution in Grafana heatmap

### Peak Write Load (Last 24h)

**Query**:
```promql
max_over_time(sum(rate(datastorage_write_total[5m]))[24h:])
```

**Use Case**: Capacity planning

### Peak Query Load (Last 24h)

**Query**:
```promql
max_over_time(sum(rate(datastorage_query_total[5m]))[24h:])
```

**Use Case**: Capacity planning

---

## Cardinality Monitoring

### Unique Label Combinations (Write Metric)

**Query**:
```promql
count(datastorage_write_total)
```

**Use Case**: Monitor cardinality for write metric
**Target**: < 100 unique combinations

### Unique Label Combinations (Validation Metric)

**Query**:
```promql
count(datastorage_validation_failures_total)
```

**Use Case**: Monitor cardinality for validation metric
**Target**: < 100 unique combinations

### Total Cardinality Across All Metrics

**Query**:
```promql
count({__name__=~"datastorage_.*"})
```

**Use Case**: Monitor overall metrics cardinality
**Alert Threshold**: > 500 (potential cardinality explosion)

---

## Recommended Alerts

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
  severity: critical
  annotations:
    summary: "Data Storage write error rate > 5%"

# PostgreSQL Failure
- alert: DataStoragePostgreSQLFailure
  expr: rate(datastorage_dualwrite_failure_total{reason="postgresql_failure"}[5m]) > 0
  for: 1m
  severity: critical
  annotations:
    summary: "PostgreSQL write failures detected"

# High Query Error Rate
- alert: DataStorageHighQueryErrorRate
  expr: |
    100 * (
      sum(rate(datastorage_query_total{status="failure"}[5m]))
      /
      sum(rate(datastorage_query_total[5m]))
    ) > 5
  for: 5m
  severity: critical
  annotations:
    summary: "Data Storage query error rate > 5%"
```

### Warning Alerts

```yaml
# Vector DB Degradation
- alert: DataStorageVectorDBDegraded
  expr: rate(datastorage_fallback_mode_total[5m]) > 0
  for: 5m
  severity: warning
  annotations:
    summary: "Vector DB unavailable, using PostgreSQL-only fallback"

# Low Cache Hit Rate
- alert: DataStorageLowCacheHitRate
  expr: |
    rate(datastorage_cache_hits_total[5m])
    /
    (rate(datastorage_cache_hits_total[5m]) + rate(datastorage_cache_misses_total[5m]))
    < 0.5
  for: 15m
  severity: warning
  annotations:
    summary: "Embedding cache hit rate < 50%"

# Slow Semantic Search
- alert: DataStorageSlowSemanticSearch
  expr: |
    histogram_quantile(0.95, rate(datastorage_query_duration_seconds_bucket{operation="semantic_search"}[5m]))
    > 0.1
  for: 10m
  severity: warning
  annotations:
    summary: "Semantic search p95 latency > 100ms"
```

---

## Query Best Practices

### 1. Use Rate for Counters

✅ **Correct**:
```promql
rate(datastorage_write_total[5m])
```

❌ **Incorrect**:
```promql
datastorage_write_total  # Raw counter value (not useful)
```

### 2. Use Histogram Quantiles for Latency

✅ **Correct**:
```promql
histogram_quantile(0.95, rate(datastorage_write_duration_seconds_bucket[5m]))
```

❌ **Incorrect**:
```promql
datastorage_write_duration_seconds_sum  # Sum without rate
```

### 3. Use Appropriate Time Windows

- **Real-time monitoring**: `[5m]`
- **Recent trends**: `[1h]`
- **SLI calculation**: `[30d]`

### 4. Always Use `by (label)` for Aggregation

✅ **Correct**:
```promql
sum(rate(datastorage_write_total[5m])) by (table, status)
```

❌ **Incorrect**:
```promql
sum(rate(datastorage_write_total[5m]))  # Loses label information
```

### 5. Filter Early in Query

✅ **Correct**:
```promql
rate(datastorage_query_total{operation="semantic_search"}[5m])
```

❌ **Less Efficient**:
```promql
rate(datastorage_query_total[5m]) and operation="semantic_search"
```

---

## Performance Tuning

### Reduce Query Cardinality

If queries are slow due to high cardinality:

1. **Aggregate early**: Use `sum by (label)` to reduce time series
2. **Filter labels**: Use `{label="value"}` to limit scope
3. **Increase time window**: Use `[15m]` instead of `[5m]` for less granular data

### Recording Rules for Expensive Queries

Create recording rules for frequently used queries:

```yaml
groups:
  - name: datastorage
    interval: 30s
    rules:
      - record: datastorage:write_success_rate
        expr: |
          rate(datastorage_write_total{status="success"}[5m])

      - record: datastorage:cache_hit_rate
        expr: |
          rate(datastorage_cache_hits_total[5m])
          /
          (rate(datastorage_cache_hits_total[5m]) + rate(datastorage_cache_misses_total[5m]))
```

---

## Troubleshooting Queries

### Find All Data Storage Metrics

```promql
{__name__=~"datastorage_.*"}
```

### Check Metric Freshness

```promql
time() - timestamp(datastorage_write_total)
```

**Use Case**: Detect stale metrics (> 60s is concerning)

### Identify Metrics with High Cardinality

```promql
topk(10, count by (__name__)({__name__=~"datastorage_.*"}))
```

**Use Case**: Find metrics contributing to cardinality

---

## Summary

- **Total Metrics**: 11 Prometheus metrics
- **Label Cardinality**: 47 unique combinations (safe)
- **Query Performance**: < 1ms for most queries
- **SLI Targets**: 99.9% success rate for writes and queries
- **Alert Thresholds**: 5% error rate (critical), 1% error rate (warning)

**For Grafana Dashboard**: See [grafana-dashboard.json](./grafana-dashboard.json)
**For Alerting Runbook**: See [ALERTING_RUNBOOK.md](./ALERTING_RUNBOOK.md)

---

**Document Version**: 1.0
**Last Updated**: October 13, 2025
**BR Coverage**: BR-STORAGE-019 (Logging and metrics for all operations)

