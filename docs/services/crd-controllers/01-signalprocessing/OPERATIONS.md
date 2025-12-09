# SignalProcessing Service - Operations Guide

**Version**: 1.0
**Last Updated**: December 9, 2025
**Related**: [IMPLEMENTATION_PLAN_V1.31](IMPLEMENTATION_PLAN_V1.31.md)

---

## Overview

This document describes how to operate, monitor, and troubleshoot the SignalProcessing CRD controller in production environments.

---

## Service Overview

| Property | Value |
|----------|-------|
| **Service Name** | signalprocessing-controller |
| **Health Port** | 8081 (`/health`, `/ready`) |
| **Metrics Port** | 9090 (`/metrics`) |
| **CRD** | `signalprocessings.signalprocessing.kubernaut.ai` |
| **Namespace** | `kubernaut-system` |

---

## Health Checks

### Endpoints

```bash
# Liveness probe
curl http://localhost:8081/health
# Expected: {"status": "ok"}

# Readiness probe
curl http://localhost:8081/ready
# Expected: {"status": "ready"} when controller is ready to process

# Metrics
curl http://localhost:9090/metrics
```

### Kubernetes Probes

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8081
  initialDelaySeconds: 15
  periodSeconds: 20
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /ready
    port: 8081
  initialDelaySeconds: 5
  periodSeconds: 10
  failureThreshold: 3
```

---

## Monitoring

### Key Metrics (DD-005 Compliant)

| Metric | Type | Description |
|--------|------|-------------|
| `signalprocessing_reconciliations_total` | Counter | Total reconciliation attempts |
| `signalprocessing_reconciliation_duration_seconds` | Histogram | Time per reconciliation |
| `signalprocessing_enrichment_duration_seconds` | Histogram | K8s context enrichment time |
| `signalprocessing_classification_duration_seconds` | Histogram | Classification time |
| `signalprocessing_errors_total` | Counter | Error count by type |

### Prometheus Queries

```promql
# Reconciliation rate (per minute)
rate(signalprocessing_reconciliations_total[5m]) * 60

# Success rate
sum(rate(signalprocessing_reconciliations_total{status="success"}[5m])) /
sum(rate(signalprocessing_reconciliations_total[5m])) * 100

# P95 reconciliation latency
histogram_quantile(0.95,
  rate(signalprocessing_reconciliation_duration_seconds_bucket[5m]))

# Error rate by type
sum by (error_type) (rate(signalprocessing_errors_total[5m])) * 60

# Enrichment timeout rate
rate(signalprocessing_errors_total{error_type="enrichment_timeout"}[5m]) * 60
```

### Alert Rules

```yaml
groups:
  - name: signalprocessing
    rules:
      - alert: SignalProcessingHighErrorRate
        expr: |
          sum(rate(signalprocessing_errors_total[5m])) /
          sum(rate(signalprocessing_reconciliations_total[5m])) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "SignalProcessing error rate > 5%"

      - alert: SignalProcessingHighLatency
        expr: |
          histogram_quantile(0.95,
            rate(signalprocessing_reconciliation_duration_seconds_bucket[5m])) > 5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "SignalProcessing P95 latency > 5s"

      - alert: SignalProcessingDown
        expr: up{job="signalprocessing"} == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "SignalProcessing controller is down"
```

---

## Logging

### Log Levels

| Level | Use Case |
|-------|----------|
| `error` | Failures that require attention |
| `warn` | Degraded operation (fallback modes) |
| `info` | Significant events (phase transitions) |
| `debug` | Detailed debugging (API calls, decisions) |

### Log Configuration

```yaml
# Via environment variable
LOG_LEVEL: info

# Via ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-config
data:
  log-level: info
  log-format: json  # or "text"
```

### Log Analysis

```bash
# View recent errors
kubectl logs -n kubernaut-system deploy/signalprocessing-controller \
  | jq 'select(.level=="error")'

# Track specific signal
kubectl logs -n kubernaut-system deploy/signalprocessing-controller \
  | jq 'select(.signal=="my-signal-123")'

# Monitor reconciliation loop
kubectl logs -n kubernaut-system deploy/signalprocessing-controller -f \
  | jq 'select(.msg | contains("Reconciling"))'
```

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_LEVEL` | `info` | Log verbosity |
| `METRICS_PORT` | `9090` | Metrics endpoint port |
| `HEALTH_PORT` | `8081` | Health/ready endpoint port |
| `ENRICHMENT_TIMEOUT` | `2s` | K8s API timeout |
| `CLASSIFICATION_TIMEOUT` | `1s` | Rego evaluation timeout |
| `LEADER_ELECTION` | `true` | Enable leader election |

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-config
  namespace: kubernaut-system
data:
  config.yaml: |
    enrichment:
      timeout: 2s
      cacheEnabled: true
      cacheTTL: 5m
    classification:
      timeout: 1s
      fallbackEnabled: true
    metrics:
      port: 9090
    health:
      port: 8081
```

---

## Rego Policy Management

### Policy Location

Rego policies are loaded from ConfigMaps mounted at `/etc/kubernaut/policies/`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-rego-policies
  namespace: kubernaut-system
data:
  priority.rego: |
    package signalprocessing.priority

    import rego.v1

    default priority := "P2"

    priority := "P0" if {
      input.environment == "production"
      input.signal.severity == "critical"
    }
```

### Hot-Reload

The controller automatically reloads Rego policies when ConfigMaps change (via fsnotify file watch). No restart required.

```bash
# Update policy
kubectl edit configmap kubernaut-rego-policies -n kubernaut-system

# Verify reload (check logs)
kubectl logs -n kubernaut-system deploy/signalprocessing-controller \
  | grep "Policy reloaded"
```

---

## Troubleshooting

### Common Issues

#### 1. Controller Not Processing Signals

**Symptoms**: SignalProcessing CRDs stuck in `Pending` phase

**Check**:
```bash
# Check controller is running
kubectl get pods -n kubernaut-system -l app=signalprocessing-controller

# Check leader election
kubectl get lease -n kubernaut-system signalprocessing-controller

# Check controller logs
kubectl logs -n kubernaut-system deploy/signalprocessing-controller --tail=100
```

**Solutions**:
- Verify RBAC permissions (see [DEPLOYMENT.md](DEPLOYMENT.md))
- Check for OOM kills: `kubectl describe pod -n kubernaut-system <pod>`
- Restart controller: `kubectl rollout restart deploy/signalprocessing-controller`

#### 2. High Latency / Timeouts

**Symptoms**: `enrichment_timeout` errors in logs

**Check**:
```bash
# Check K8s API server health
kubectl get --raw /healthz

# Check enrichment duration
curl -s http://localhost:9090/metrics | grep enrichment_duration
```

**Solutions**:
- Increase `ENRICHMENT_TIMEOUT` environment variable
- Check K8s API server load
- Enable caching (if not already)

#### 3. Classification Failures

**Symptoms**: `rego_evaluation_failed` errors

**Check**:
```bash
# Verify Rego policy syntax
opa check /etc/kubernaut/policies/*.rego

# Test policy manually
opa eval -d /etc/kubernaut/policies/priority.rego \
  -i input.json "data.signalprocessing.priority.priority"
```

**Solutions**:
- Fix Rego syntax errors in ConfigMap
- Check Rego memory limits (128MB default)
- Verify input schema matches expected format

#### 4. RBAC Errors

**Symptoms**: `forbidden: User cannot list poddisruptionbudgets` in logs

**Check**:
```bash
# Verify ClusterRole
kubectl get clusterrole signalprocessing-controller -o yaml

# Check ServiceAccount binding
kubectl get clusterrolebinding signalprocessing-controller -o yaml

# Test permissions
kubectl auth can-i list poddisruptionbudgets --as system:serviceaccount:kubernaut-system:signalprocessing-controller
```

**Solutions**:
- Apply missing RBAC rules (see [DEPLOYMENT.md](DEPLOYMENT.md))
- Verify ServiceAccount exists

---

## Scaling

### Resource Recommendations

| Workload | CPU Request | CPU Limit | Memory Request | Memory Limit |
|----------|-------------|-----------|----------------|--------------|
| Low (<100 signals/min) | 100m | 500m | 128Mi | 256Mi |
| Medium (100-1000) | 250m | 1000m | 256Mi | 512Mi |
| High (>1000) | 500m | 2000m | 512Mi | 1Gi |

### Leader Election

Only one instance is active at a time (leader election). For HA, run multiple replicas:

```yaml
spec:
  replicas: 3  # Only 1 active, 2 standby
```

---

## Disaster Recovery

### Backup Considerations

- **CRD Data**: Backed up via etcd snapshots
- **ConfigMaps**: Include in GitOps repository
- **Rego Policies**: Store in version control

### Recovery Procedure

1. Restore etcd from backup
2. Apply CRDs: `kubectl apply -f config/crd/bases/`
3. Apply RBAC: `kubectl apply -f config/rbac/`
4. Deploy controller: `kubectl apply -f config/manager/`

---

## Support

### Runbook Links

- [Metrics & SLOs](metrics-slos.md)
- [Observability & Logging](observability-logging.md)
- [Security Configuration](security-configuration.md)

### Contact

- **Team**: SignalProcessing Team
- **Slack**: #kubernaut-signalprocessing
- **On-Call**: PagerDuty rotation

---

## References

- [DD-005: Observability Standards](../../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)
- [DD-007: Graceful Shutdown](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md)
- [ADR-038: Async Buffered Audit](../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md)

