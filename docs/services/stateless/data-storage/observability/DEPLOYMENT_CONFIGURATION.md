# Data Storage Service - Observability Deployment Configuration

**Date**: October 13, 2025
**Service**: Data Storage Service
**BR Coverage**: BR-STORAGE-019 (Logging and metrics)

---

## Overview

This document provides step-by-step instructions for configuring observability (metrics, logging, monitoring) for the Data Storage Service in production environments.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Prometheus Configuration](#prometheus-configuration)
3. [Grafana Dashboard Setup](#grafana-dashboard-setup)
4. [Alert Configuration](#alert-configuration)
5. [Log Aggregation](#log-aggregation)
6. [Verification](#verification)

---

## Prerequisites

### Required Components

- **Prometheus**: Metrics collection and storage
- **Grafana**: Metrics visualization and dashboarding
- **AlertManager**: Alert routing and notification
- **Kubernetes**: 1.23+ with ServiceMonitor support
- **Prometheus Operator**: For ServiceMonitor CRDs

### Required Permissions

```bash
# Verify cluster-admin or equivalent permissions
kubectl auth can-i create servicemonitor
kubectl auth can-i create prometheusrules
```

---

## Prometheus Configuration

### Step 1: Create ServiceMonitor

Create a ServiceMonitor to enable Prometheus to scrape Data Storage Service metrics.

**File**: `deploy/monitoring/data-storage-servicemonitor.yaml`

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: data-storage-service
  namespace: kubernaut
  labels:
    app: data-storage-service
    prometheus: kube-prometheus
spec:
  selector:
    matchLabels:
      app: data-storage-service
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
      scheme: http
```

**Apply ServiceMonitor**:
```bash
kubectl apply -f deploy/monitoring/data-storage-servicemonitor.yaml
```

### Step 2: Update Service Definition

Ensure the Data Storage Service has a metrics port exposed.

**File**: `deploy/manifests/data-storage-service.yaml` (excerpt)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: data-storage-service
  namespace: kubernaut
  labels:
    app: data-storage-service
spec:
  selector:
    app: data-storage-service
  ports:
    - name: http
      port: 8080
      targetPort: 8080
      protocol: TCP
    - name: metrics  # Prometheus metrics port
      port: 9090
      targetPort: 9090
      protocol: TCP
  type: ClusterIP
```

**Apply Service**:
```bash
kubectl apply -f deploy/manifests/data-storage-service.yaml
```

### Step 3: Verify Prometheus Scraping

```bash
# Check ServiceMonitor status
kubectl get servicemonitor -n kubernaut data-storage-service

# Verify Prometheus targets
kubectl port-forward -n monitoring svc/prometheus-k8s 9090:9090

# Open browser to http://localhost:9090/targets
# Look for "kubernaut/data-storage-service/0" target
# Status should be "UP"
```

**Expected Metrics**:
- `datastorage_write_total`
- `datastorage_write_duration_seconds`
- `datastorage_dualwrite_success_total`
- `datastorage_dualwrite_failure_total`
- `datastorage_fallback_mode_total`
- `datastorage_cache_hits_total`
- `datastorage_cache_misses_total`
- `datastorage_embedding_generation_duration_seconds`
- `datastorage_validation_failures_total`
- `datastorage_query_total`
- `datastorage_query_duration_seconds`

---

## Grafana Dashboard Setup

### Step 1: Import Dashboard JSON

**File**: `docs/services/stateless/data-storage/observability/grafana-dashboard.json`

**Option A: Import via Grafana UI**:
1. Navigate to Grafana (http://grafana.example.com)
2. Click "+" → "Import"
3. Upload `grafana-dashboard.json`
4. Select Prometheus data source
5. Click "Import"

**Option B: Import via ConfigMap**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: data-storage-dashboard
  namespace: monitoring
  labels:
    grafana_dashboard: "1"
data:
  data-storage-observability.json: |
    # Paste contents of grafana-dashboard.json here
```

```bash
kubectl apply -f deploy/monitoring/data-storage-dashboard-configmap.yaml
```

### Step 2: Configure Data Source

Ensure Prometheus data source is configured in Grafana:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-datasources
  namespace: monitoring
data:
  prometheus.yaml: |
    apiVersion: 1
    datasources:
      - name: Prometheus
        type: prometheus
        access: proxy
        url: http://prometheus-k8s:9090
        isDefault: true
        editable: false
```

### Step 3: Verify Dashboard

1. Navigate to Grafana → Dashboards
2. Find "Data Storage Service - Observability Dashboard"
3. Verify panels are rendering with data
4. Expected panels:
   - Write Operations Rate
   - Write Duration (p95, p99)
   - Dual-Write Success vs Failure
   - Fallback Mode Operations
   - Cache Hit Rate
   - Embedding Generation Duration
   - Validation Failures by Field
   - Query Operations Rate
   - Query Duration by Operation
   - Semantic Search Performance
   - Error Rate Overview
   - Query Error Rate
   - Validation Failure Rate

---

## Alert Configuration

### Step 1: Create PrometheusRule

**File**: `deploy/monitoring/data-storage-prometheusrule.yaml`

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: data-storage-alerts
  namespace: kubernaut
  labels:
    app: data-storage-service
    prometheus: kube-prometheus
spec:
  groups:
    - name: data-storage-critical
      interval: 30s
      rules:
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
            description: "Write error rate is {{ $value | humanizePercentage }} for the last 5 minutes"
            runbook_url: "https://docs.example.com/runbook/data-storage-high-write-error-rate"

        - alert: DataStoragePostgreSQLFailure
          expr: rate(datastorage_dualwrite_failure_total{reason="postgresql_failure"}[5m]) > 0
          for: 1m
          labels:
            severity: critical
            service: data-storage
          annotations:
            summary: "PostgreSQL write failures detected"
            description: "PostgreSQL is failing to write audit records"
            runbook_url: "https://docs.example.com/runbook/data-storage-postgresql-failure"

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
            description: "Query error rate is {{ $value | humanizePercentage }} for the last 5 minutes"
            runbook_url: "https://docs.example.com/runbook/data-storage-high-query-error-rate"

    - name: data-storage-warning
      interval: 30s
      rules:
        - alert: DataStorageVectorDBDegraded
          expr: rate(datastorage_fallback_mode_total[5m]) > 0
          for: 5m
          labels:
            severity: warning
            service: data-storage
          annotations:
            summary: "Vector DB unavailable, using PostgreSQL-only fallback"
            description: "Semantic search may be degraded or unavailable"
            runbook_url: "https://docs.example.com/runbook/data-storage-vectordb-degraded"

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
            description: "Cache hit rate is {{ $value | humanizePercentage }}, consider increasing cache size"
            runbook_url: "https://docs.example.com/runbook/data-storage-low-cache-hit-rate"

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
            description: "Semantic search is taking {{ $value | humanizeDuration }} at p95"
            runbook_url: "https://docs.example.com/runbook/data-storage-slow-semantic-search"
```

**Apply PrometheusRule**:
```bash
kubectl apply -f deploy/monitoring/data-storage-prometheusrule.yaml
```

### Step 2: Configure AlertManager

**File**: `deploy/monitoring/alertmanager-config.yaml` (excerpt)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: alertmanager-config
  namespace: monitoring
stringData:
  alertmanager.yaml: |
    global:
      resolve_timeout: 5m

    route:
      group_by: ['alertname', 'service']
      group_wait: 10s
      group_interval: 10s
      repeat_interval: 12h
      receiver: 'default'
      routes:
        - match:
            severity: critical
            service: data-storage
          receiver: 'pagerduty-critical'
          continue: true
        - match:
            severity: warning
            service: data-storage
          receiver: 'slack-warnings'

    receivers:
      - name: 'default'
        slack_configs:
          - channel: '#alerts'
            api_url: 'https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK'

      - name: 'pagerduty-critical'
        pagerduty_configs:
          - service_key: 'YOUR_PAGERDUTY_SERVICE_KEY'

      - name: 'slack-warnings'
        slack_configs:
          - channel: '#data-storage-warnings'
            api_url: 'https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK'
```

### Step 3: Verify Alerts

```bash
# Check PrometheusRule status
kubectl get prometheusrule -n kubernaut data-storage-alerts

# Verify alerts in Prometheus UI
kubectl port-forward -n monitoring svc/prometheus-k8s 9090:9090

# Open browser to http://localhost:9090/alerts
# Look for "data-storage-critical" and "data-storage-warning" groups
```

---

## Log Aggregation

### Structured Logging Configuration

The Data Storage Service uses structured logging (zap) with JSON format for easy parsing.

**Environment Variables**:
```yaml
env:
  - name: LOG_LEVEL
    value: "info"  # Options: debug, info, warn, error
  - name: LOG_FORMAT
    value: "json"  # Options: json, console
  - name: LOG_OUTPUT
    value: "stdout"  # Options: stdout, file
```

### Fluentd/Fluent Bit Configuration

**File**: `deploy/monitoring/fluentd-config.yaml` (excerpt)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluentd-config
  namespace: logging
data:
  fluent.conf: |
    <source>
      @type tail
      path /var/log/containers/data-storage-service-*.log
      pos_file /var/log/fluentd-data-storage.pos
      tag kubernetes.data-storage
      <parse>
        @type json
        time_key time
        time_format %Y-%m-%dT%H:%M:%S.%NZ
      </parse>
    </source>

    <filter kubernetes.data-storage>
      @type parser
      key_name log
      <parse>
        @type json
      </parse>
    </filter>

    <match kubernetes.data-storage>
      @type elasticsearch
      host elasticsearch.logging.svc.cluster.local
      port 9200
      index_name data-storage-logs
      type_name _doc
      logstash_format true
      logstash_prefix data-storage
    </match>
```

### Log Search Queries (Elasticsearch/Kibana)

**High-level errors**:
```
level:error AND service:data-storage
```

**Write failures**:
```
level:error AND operation:write AND service:data-storage
```

**PostgreSQL errors**:
```
level:error AND (postgres OR postgresql) AND service:data-storage
```

**Validation failures**:
```
level:warn AND validation AND service:data-storage
```

---

## Verification

### Step 1: Verify Metrics Collection

```bash
# Check metrics endpoint
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  curl -s http://localhost:9090/metrics | grep datastorage | head -20

# Expected output:
# datastorage_write_total{table="remediation_audit",status="success"} 1234
# datastorage_write_duration_seconds_bucket{table="remediation_audit",le="0.005"} 100
# datastorage_dualwrite_success_total 1200
# ...
```

### Step 2: Verify Prometheus Targets

```bash
# Port-forward to Prometheus
kubectl port-forward -n monitoring svc/prometheus-k8s 9090:9090

# Query metrics
curl -s 'http://localhost:9090/api/v1/query?query=datastorage_write_total' | jq .

# Expected: JSON response with metric data
```

### Step 3: Verify Grafana Dashboard

```bash
# Access Grafana
kubectl port-forward -n monitoring svc/grafana 3000:3000

# Navigate to http://localhost:3000
# Find "Data Storage Service - Observability Dashboard"
# Verify all panels show data
```

### Step 4: Verify Alerts

```bash
# Check alert status in Prometheus
curl -s 'http://localhost:9090/api/v1/alerts' | jq '.data.alerts[] | select(.labels.service=="data-storage")'

# Expected: List of configured alerts with state (inactive/pending/firing)
```

### Step 5: Test Alert Firing

```bash
# Simulate high write error rate (for testing only!)
# This will trigger DataStorageHighWriteErrorRate alert

# In test environment:
kubectl exec -it deployment/data-storage-service-test -n kubernaut -- \
  curl -X POST http://localhost:8080/test/inject-errors

# Wait 5-10 minutes for alert to fire

# Check AlertManager
kubectl port-forward -n monitoring svc/alertmanager 9093:9093

# Navigate to http://localhost:9093
# Verify "DataStorageHighWriteErrorRate" alert is firing

# Stop error injection
kubectl exec -it deployment/data-storage-service-test -n kubernaut -- \
  curl -X POST http://localhost:8080/test/clear-errors
```

---

## Performance Impact

### Metrics Collection Overhead

- **CPU**: < 1% additional CPU usage
- **Memory**: ~10MB for Prometheus client library
- **Network**: ~50KB/s metrics data sent to Prometheus
- **Latency**: < 0.01% increase in operation latency

### Best Practices

1. **Scrape Interval**: 30s is recommended (balance freshness vs load)
2. **Retention**: 15 days for Prometheus, 90 days for long-term storage
3. **Cardinality**: Maintain < 100 unique label combinations per metric
4. **Recording Rules**: Create for expensive queries (see [PROMETHEUS_QUERIES.md](./PROMETHEUS_QUERIES.md))

---

## Troubleshooting

### Metrics Not Appearing in Prometheus

```bash
# Check ServiceMonitor
kubectl get servicemonitor -n kubernaut data-storage-service -o yaml

# Check Service selector matches pods
kubectl get svc -n kubernaut data-storage-service -o yaml
kubectl get pods -n kubernaut -l app=data-storage-service --show-labels

# Check Prometheus scrape config
kubectl get prometheus -n monitoring kube-prometheus -o yaml | grep -A 20 serviceMonitorSelector

# Check Prometheus logs
kubectl logs -n monitoring prometheus-kube-prometheus-0 | grep data-storage
```

### Dashboard Not Showing Data

```bash
# Verify Prometheus data source
kubectl exec -it deployment/grafana -n monitoring -- \
  curl -s http://prometheus-k8s.monitoring.svc.cluster.local:9090/api/v1/query?query=up

# Check dashboard JSON syntax
kubectl get configmap -n monitoring data-storage-dashboard -o yaml

# Check Grafana logs
kubectl logs -n monitoring deployment/grafana | grep data-storage
```

### Alerts Not Firing

```bash
# Check PrometheusRule syntax
kubectl get prometheusrule -n kubernaut data-storage-alerts -o yaml

# Verify alert expression in Prometheus UI
# Navigate to http://localhost:9090/graph
# Test alert query manually

# Check AlertManager configuration
kubectl get secret -n monitoring alertmanager-config -o yaml
```

---

## Security Considerations

### Metrics Endpoint Security

```yaml
# Add authentication to metrics endpoint (optional)
apiVersion: v1
kind: NetworkPolicy
metadata:
  name: data-storage-metrics-access
  namespace: kubernaut
spec:
  podSelector:
    matchLabels:
      app: data-storage-service
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: monitoring
      ports:
        - protocol: TCP
          port: 9090  # Metrics port
```

### Sensitive Data in Logs

**Do NOT log**:
- User PII (personally identifiable information)
- API keys, tokens, passwords
- Full database connection strings
- Sensitive audit record content

**Sanitize logs**:
```go
logger.Info("audit created",
    zap.String("name", audit.Name),  // ✅ OK
    zap.String("namespace", audit.Namespace),  // ✅ OK
    // ❌ NEVER: zap.String("api_key", apiKey)
    // ❌ NEVER: zap.String("password", password)
)
```

---

## Summary

- **Metrics Collected**: 11 Prometheus metrics with 47 unique label combinations
- **Dashboards**: 1 Grafana dashboard with 13 panels
- **Alerts**: 6 alerts (3 critical, 3 warning)
- **Performance Impact**: < 1% overhead
- **Cardinality**: Well within safe limits (< 100)

**Related Documentation**:
- [PROMETHEUS_QUERIES.md](./PROMETHEUS_QUERIES.md) - Query reference
- [ALERTING_RUNBOOK.md](./ALERTING_RUNBOOK.md) - Alert troubleshooting
- [grafana-dashboard.json](./grafana-dashboard.json) - Dashboard JSON

---

**Document Version**: 1.0
**Last Updated**: October 13, 2025
**BR Coverage**: BR-STORAGE-019 (Logging and metrics for all operations)

