# Dynamic Toolset Service - Metrics & SLOs

**Version**: v1.0
**Last Updated**: October 10, 2025
**Status**: ✅ Design Complete

---

## Prometheus Metrics

### Service Discovery Metrics

```go
var (
    servicesDiscoveredTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "toolset_services_discovered_total",
            Help: "Total services discovered by type",
        },
        []string{"service_type", "namespace"},
    )

    discoveryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "toolset_discovery_duration_seconds",
            Help:    "Service discovery duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 100ms to ~100s
        },
        []string{"status"}, // "success" or "failure"
    )

    detectorsHealthStatus = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "toolset_detectors_health_status",
            Help: "Health status of service detectors (1=healthy, 0=unhealthy)",
        },
        []string{"detector_type"},
    )
)
```

### ConfigMap Reconciliation Metrics

```go
var (
    configmapReconcileTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "toolset_configmap_reconcile_total",
            Help: "Total ConfigMap reconciliation attempts",
        },
        []string{"operation", "status"}, // operation: "create", "update", "drift_detected"
    )

    configmapDriftDetectedTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "toolset_configmap_drift_detected_total",
            Help: "Total ConfigMap drift detections",
        },
        []string{"drift_type"}, // "missing_key", "modified_value", "deleted"
    )

    reconciliationDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "toolset_reconciliation_duration_seconds",
            Help:    "ConfigMap reconciliation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
        },
    )
)
```

### Health Check Metrics

```go
var (
    healthChecksTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "toolset_health_checks_total",
            Help: "Total health checks performed",
        },
        []string{"service_type", "status"}, // status: "healthy", "unhealthy"
    )

    healthCheckDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "toolset_health_check_duration_seconds",
            Help:    "Health check duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
        },
        []string{"service_type"},
    )

    unhealthyServicesGauge = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "toolset_unhealthy_services",
            Help: "Number of currently unhealthy services",
        },
        []string{"service_type"},
    )
)
```

### HTTP API Metrics

```go
var (
    httpRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "toolset_http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
        },
        []string{"endpoint", "method", "status"},
    )

    httpRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "toolset_http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"endpoint", "method", "status"},
    )
)
```

### Toolset Generation Metrics

```go
var (
    toolsetsGeneratedTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "toolset_toolsets_generated_total",
            Help: "Total toolsets generated",
        },
        []string{"toolset_type"}, // "prometheus", "grafana", "kubernetes", etc.
    )

    generationFailuresTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "toolset_generation_failures_total",
            Help: "Total toolset generation failures",
        },
        []string{"toolset_type", "error_type"},
    )
)
```

---

## Grafana Dashboard

### Key Panels

**1. Service Discovery Count**
```promql
sum(toolset_services_discovered_total) by (service_type)
```

**2. Discovery Success Rate**
```promql
rate(toolset_discovery_duration_seconds_count{status="success"}[5m]) /
rate(toolset_discovery_duration_seconds_count[5m])
```

**3. ConfigMap Drift Detection Rate**
```promql
rate(toolset_configmap_drift_detected_total[5m])
```

**4. Reconciliation Operations**
```promql
rate(toolset_configmap_reconcile_total[5m]) by (operation)
```

**5. Health Check Success Rate**
```promql
rate(toolset_health_checks_total{status="healthy"}[5m]) /
rate(toolset_health_checks_total[5m]) by (service_type)
```

**6. API Latency (P50/P95/P99)**
```promql
histogram_quantile(0.50, rate(toolset_http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(toolset_http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(toolset_http_request_duration_seconds_bucket[5m]))
```

**7. Discovery Duration P95**
```promql
histogram_quantile(0.95, rate(toolset_discovery_duration_seconds_bucket[5m]))
```

**8. Unhealthy Services**
```promql
sum(toolset_unhealthy_services) by (service_type)
```

---

## SLO Definitions

### Availability SLO

**Target**: 99.5% availability
**Measurement**: Successful HTTP 200 responses / total requests
**Query**:
```promql
sum(rate(toolset_http_requests_total{status=~"2.."}[5m])) /
sum(rate(toolset_http_requests_total[5m]))
```

**Acceptable Downtime**: ~3.6 hours/month

### Discovery Latency SLO

**Target**: p95 < 10s
**Measurement**: Time to complete service discovery
**Query**:
```promql
histogram_quantile(0.95, rate(toolset_discovery_duration_seconds_bucket[5m])) < 10.0
```

**Rationale**: Service discovery is not time-critical; 10 seconds is acceptable for discovering all services in a cluster.

### Reconciliation Latency SLO

**Target**: p95 < 5s
**Measurement**: Time to reconcile ConfigMap
**Query**:
```promql
histogram_quantile(0.95, rate(toolset_reconciliation_duration_seconds_bucket[5m])) < 5.0
```

**Rationale**: ConfigMap reconciliation should be quick to minimize drift window.

### API Response Time SLO

**Target**: p95 < 200ms, p99 < 500ms
**Query**:
```promql
histogram_quantile(0.95, rate(toolset_http_request_duration_seconds_bucket[5m])) < 0.200
histogram_quantile(0.99, rate(toolset_http_request_duration_seconds_bucket[5m])) < 0.500
```

**Rationale**: Manual API queries should be fast for operator experience.

### Health Check Success Rate SLO

**Target**: >95% healthy services
**Query**:
```promql
sum(rate(toolset_health_checks_total{status="healthy"}[5m])) /
sum(rate(toolset_health_checks_total[5m])) > 0.95
```

**Rationale**: Most services should be healthy; occasional transient failures are acceptable.

---

## Alert Rules

```yaml
groups:
  - name: dynamic-toolset-service
    rules:
    # Service discovery failures
    - alert: DynamicToolsetDiscoveryFailure
      expr: |
        rate(toolset_discovery_duration_seconds_count{status="failure"}[5m]) > 0.5
      for: 10m
      labels:
        severity: critical
      annotations:
        summary: "Dynamic Toolset service discovery failing"
        description: "Service discovery failure rate >50% for 10 minutes"

    # High discovery latency
    - alert: DynamicToolsetHighDiscoveryLatency
      expr: |
        histogram_quantile(0.95, rate(toolset_discovery_duration_seconds_bucket[5m])) > 30
      for: 15m
      labels:
        severity: warning
      annotations:
        summary: "Dynamic Toolset discovery latency >30s"
        description: "Service discovery taking longer than expected"

    # ConfigMap reconciliation failures
    - alert: DynamicToolsetReconciliationFailure
      expr: |
        rate(toolset_configmap_reconcile_total{status="failure"}[5m]) > 0.1
      for: 5m
      labels:
        severity: critical
      annotations:
        summary: "Dynamic Toolset ConfigMap reconciliation failing"
        description: "Reconciliation failure rate >10% for 5 minutes"

    # Frequent ConfigMap drift
    - alert: DynamicToolsetFrequentDrift
      expr: |
        rate(toolset_configmap_drift_detected_total[5m]) > 1
      for: 15m
      labels:
        severity: warning
      annotations:
        summary: "Dynamic Toolset ConfigMap drifting frequently"
        description: "ConfigMap drift detected >1/min for 15 minutes (possible external modification)"

    # High health check failure rate
    - alert: DynamicToolsetHealthCheckFailures
      expr: |
        rate(toolset_health_checks_total{status="unhealthy"}[5m]) /
        rate(toolset_health_checks_total[5m]) > 0.20
      for: 10m
      labels:
        severity: warning
      annotations:
        summary: "Dynamic Toolset health check failure rate >20%"
        description: "Many discovered services failing health checks"

    # No services discovered
    - alert: DynamicToolsetNoServicesDiscovered
      expr: |
        sum(toolset_services_discovered_total) == 0
      for: 30m
      labels:
        severity: warning
      annotations:
        summary: "Dynamic Toolset discovering zero services"
        description: "No services discovered for 30 minutes (possible RBAC or cluster issue)"

    # High API latency
    - alert: DynamicToolsetHighAPILatency
      expr: |
        histogram_quantile(0.95, rate(toolset_http_request_duration_seconds_bucket[5m])) > 1.0
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "Dynamic Toolset API p95 latency >1s"
        description: "Manual API queries responding slowly"

    # Service down
    - alert: DynamicToolsetServiceDown
      expr: |
        up{job="dynamic-toolset"} == 0
      for: 2m
      labels:
        severity: critical
      annotations:
        summary: "Dynamic Toolset service is down"
        description: "Service not responding to health checks"
```

---

## Grafana Dashboard JSON

```json
{
  "dashboard": {
    "title": "Dynamic Toolset Service",
    "panels": [
      {
        "id": 1,
        "title": "Discovered Services by Type",
        "type": "stat",
        "targets": [{
          "expr": "sum(toolset_services_discovered_total) by (service_type)"
        }]
      },
      {
        "id": 2,
        "title": "Discovery Success Rate",
        "type": "graph",
        "targets": [{
          "expr": "rate(toolset_discovery_duration_seconds_count{status=\"success\"}[5m]) / rate(toolset_discovery_duration_seconds_count[5m])"
        }]
      },
      {
        "id": 3,
        "title": "ConfigMap Reconciliation Events",
        "type": "graph",
        "targets": [{
          "expr": "rate(toolset_configmap_reconcile_total[5m]) by (operation)"
        }]
      },
      {
        "id": 4,
        "title": "Health Check Success Rate",
        "type": "graph",
        "targets": [{
          "expr": "rate(toolset_health_checks_total{status=\"healthy\"}[5m]) / rate(toolset_health_checks_total[5m]) by (service_type)"
        }]
      },
      {
        "id": 5,
        "title": "API Latency (p95)",
        "type": "graph",
        "targets": [{
          "expr": "histogram_quantile(0.95, rate(toolset_http_request_duration_seconds_bucket[5m]))"
        }]
      },
      {
        "id": 6,
        "title": "Discovery Duration (p95)",
        "type": "graph",
        "targets": [{
          "expr": "histogram_quantile(0.95, rate(toolset_discovery_duration_seconds_bucket[5m]))"
        }]
      },
      {
        "id": 7,
        "title": "Unhealthy Services",
        "type": "stat",
        "targets": [{
          "expr": "sum(toolset_unhealthy_services) by (service_type)"
        }]
      },
      {
        "id": 8,
        "title": "ConfigMap Drift Events",
        "type": "graph",
        "targets": [{
          "expr": "rate(toolset_configmap_drift_detected_total[5m]) by (drift_type)"
        }]
      }
    ]
  }
}
```

---

## Performance Targets

| Metric | Target | Rationale |
|--------|--------|-----------|
| **Availability** | 99.5% | 3.6 hours downtime/month acceptable for non-critical service |
| **Discovery Latency (p95)** | < 10s | Service discovery is infrequent, 10s acceptable |
| **Reconciliation Latency (p95)** | < 5s | Quick reconciliation minimizes drift window |
| **API Response Time (p95)** | < 200ms | Fast operator experience for manual queries |
| **Health Check Success Rate** | >95% | Most services healthy, occasional transient failures OK |
| **Memory Usage** | < 128MB | Lightweight service with minimal state |
| **CPU Usage** | < 0.1 cores | Low CPU for periodic discovery |

---

**Document Status**: ✅ Complete Metrics & SLO Specification
**Last Updated**: October 10, 2025
**Confidence**: 95% (Very High)

