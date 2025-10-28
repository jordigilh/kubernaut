# Gateway Service Deployment Guide

**Service**: Kubernaut Gateway
**Version**: v0.1.0
**Namespace**: `kubernaut-gateway`
**Purpose**: Signal ingestion, deduplication, storm detection, and CRD creation

---

## 📋 Overview

The Gateway service is the entry point for all signals in the Kubernaut remediation pipeline. It:

1. **Ingests signals** from multiple sources (Prometheus AlertManager, Kubernetes Events)
2. **Deduplicates** signals using Redis-based fingerprinting
3. **Detects storms** using rate-based and pattern-based detection
4. **Classifies environment** (production, staging, dev) from namespace labels
5. **Assigns priority** (P0-P4) based on severity and environment
6. **Creates RemediationRequest CRDs** for the remediation pipeline

---

## 🚀 Quick Start

### Prerequisites

- Kubernetes cluster (v1.24+)
- `kubectl` configured
- RemediationRequest CRD installed

### Deploy

```bash
# Deploy all Gateway components
kubectl apply -k deploy/gateway/

# Verify deployment
kubectl get pods -n kubernaut-gateway
kubectl get svc -n kubernaut-gateway
```

### Test

```bash
# Port-forward to Gateway service
kubectl port-forward -n kubernaut-gateway svc/gateway 8080:8080

# Send test signal
curl -X POST http://localhost:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d '{
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "HighMemoryUsage",
        "severity": "critical",
        "namespace": "prod-payment-service"
      },
      "annotations": {
        "summary": "Memory usage above 90%"
      }
    }]
  }'

# Check created CRD
kubectl get remediationrequest -n prod-payment-service
```

---

## 📦 Components

### Manifests

| File | Purpose |
|------|---------|
| `00-namespace.yaml` | Creates `kubernaut-gateway` namespace |
| `01-rbac.yaml` | ServiceAccount, ClusterRole, ClusterRoleBinding |
| `02-configmap.yaml` | Gateway configuration (YAML) |
| `03-deployment.yaml` | Gateway Deployment (3 replicas) |
| `04-service.yaml` | Gateway Service (HTTP + metrics) |
| `05-redis.yaml` | Redis Deployment + Service (deduplication/storm state) |
| `06-servicemonitor.yaml` | Prometheus ServiceMonitor |
| `kustomization.yaml` | Kustomize configuration |

### RBAC Permissions

The Gateway service requires:

- **RemediationRequest CRDs**: `create`, `get`, `list`, `watch`, `update`, `patch`
- **Namespaces**: `get`, `list`, `watch` (for environment classification)
- **ConfigMaps**: `get`, `list`, `watch` (for environment overrides)

---

## ⚙️ Configuration

### Configuration File

The Gateway service uses a structured YAML configuration file mounted from ConfigMap:

**Location**: `/etc/gateway/config.yaml`
**Source**: `ConfigMap/gateway-config` → `config.yaml` key

### Configuration Structure

```yaml
# Server settings
listen_addr: ":8080"
read_timeout: 30s
write_timeout: 30s
idle_timeout: 120s

# Rate limiting
rate_limit_requests_per_minute: 100
rate_limit_burst: 10

# Deduplication TTL (5 minutes)
deduplication_ttl: 5m

# Storm detection thresholds
storm_rate_threshold: 10      # 10 alerts/minute
storm_pattern_threshold: 5    # 5 similar alerts

# Storm aggregation window (1 minute)
storm_aggregation_window: 1m

# Environment classification cache TTL (30 seconds)
environment_cache_ttl: 30s

# Environment classification ConfigMap
env_configmap_namespace: kubernaut-system
env_configmap_name: kubernaut-environment-overrides

# Redis configuration
redis:
  addr: redis-gateway.kubernaut-gateway.svc.cluster.local:6379
  db: 0
  dial_timeout: 5s
  read_timeout: 3s
  write_timeout: 3s
  pool_size: 10
  min_idle_conns: 2
```

### Modify Configuration

```bash
# Edit ConfigMap
kubectl edit configmap gateway-config -n kubernaut-gateway

# Restart Gateway pods to pick up changes
kubectl rollout restart deployment/gateway -n kubernaut-gateway
```

---

## 🔧 Operational Tasks

### Scaling

```bash
# Scale Gateway replicas
kubectl scale deployment/gateway -n kubernaut-gateway --replicas=5

# Enable HPA (Horizontal Pod Autoscaler)
kubectl autoscale deployment/gateway -n kubernaut-gateway \
  --min=3 --max=10 --cpu-percent=70
```

### Monitoring

```bash
# Check Gateway metrics
kubectl port-forward -n kubernaut-gateway svc/gateway-metrics 9090:9090
curl http://localhost:9090/metrics

# View logs
kubectl logs -n kubernaut-gateway -l app.kubernetes.io/component=gateway --tail=100 -f

# Check health
kubectl port-forward -n kubernaut-gateway svc/gateway 8080:8080
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

### Redis Operations

```bash
# Connect to Redis
kubectl exec -it -n kubernaut-gateway deployment/redis-gateway -- redis-cli

# Check keys
KEYS *

# Check deduplication fingerprints
KEYS dedup:*

# Check storm detection counters
KEYS storm:rate:*
KEYS storm:pattern:*

# Monitor Redis
MONITOR
```

### Troubleshooting

#### Gateway pods not starting

```bash
# Check pod status
kubectl describe pod -n kubernaut-gateway -l app.kubernetes.io/component=gateway

# Check logs
kubectl logs -n kubernaut-gateway -l app.kubernetes.io/component=gateway

# Common issues:
# 1. Redis not available → Check redis-gateway pod
# 2. CRD not installed → Install RemediationRequest CRD
# 3. RBAC issues → Check ServiceAccount permissions
```

#### Signals not creating CRDs

```bash
# Check Gateway logs for errors
kubectl logs -n kubernaut-gateway -l app.kubernetes.io/component=gateway | grep ERROR

# Verify RemediationRequest CRD exists
kubectl get crd remediationrequests.remediation.kubernaut.ai

# Check RBAC permissions
kubectl auth can-i create remediationrequests.remediation.kubernaut.ai \
  --as=system:serviceaccount:kubernaut-gateway:gateway \
  --all-namespaces
```

#### High deduplication rate

```bash
# Check deduplication metrics
kubectl port-forward -n kubernaut-gateway svc/gateway-metrics 9090:9090
curl http://localhost:9090/metrics | grep gateway_deduplication

# Adjust deduplication TTL in ConfigMap
# Lower TTL = less aggressive deduplication
# Higher TTL = more aggressive deduplication
```

#### Storm detection false positives

```bash
# Check storm detection metrics
curl http://localhost:9090/metrics | grep gateway_storm

# Adjust thresholds in ConfigMap:
# - storm_rate_threshold: Increase to reduce false positives
# - storm_pattern_threshold: Increase to reduce false positives
```

---

## 🔐 Security

### Network Policies

The Gateway service uses Kubernetes Network Policies to restrict traffic:

- **Ingress**: Allow from monitoring sources (Prometheus, Grafana)
- **Egress**: Allow to Redis, Kubernetes API, and RemediationRequest CRDs

### TLS/mTLS

For production deployments, consider:

1. **Ingress TLS**: Terminate TLS at Ingress/Route level
2. **Service Mesh**: Use Istio/Linkerd for mTLS between services
3. **Network Policies**: Restrict traffic to Gateway service

---

## 📊 Metrics

### Prometheus Metrics

The Gateway exposes Prometheus metrics on `:9090/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `gateway_http_requests_total` | Counter | Total HTTP requests by endpoint, method, status |
| `gateway_http_request_duration_seconds` | Histogram | HTTP request duration |
| `gateway_http_requests_in_flight` | Gauge | Current in-flight HTTP requests |
| `gateway_deduplication_checks_total` | Counter | Total deduplication checks |
| `gateway_deduplication_hits_total` | Counter | Duplicate signals detected |
| `gateway_storm_detections_total` | Counter | Storm detections by type |
| `gateway_crds_created_total` | Counter | RemediationRequest CRDs created |
| `gateway_redis_pool_total` | Gauge | Redis connection pool size |
| `gateway_redis_pool_idle` | Gauge | Redis idle connections |

### Grafana Dashboard

Import the Gateway dashboard:

```bash
kubectl apply -f deploy/monitoring/gateway-dashboard.json
```

---

## 🧪 Testing

### Unit Tests

```bash
# Run Gateway unit tests
make test-gateway

# Run specific test
go test ./pkg/gateway/... -v -run TestDeduplication
```

### Integration Tests

```bash
# Setup test cluster
make test-gateway-setup

# Run integration tests
make test-gateway

# Cleanup
make test-gateway-teardown
```

---

## 🔄 Upgrade

### Rolling Update

```bash
# Update image version in kustomization.yaml
kubectl edit kustomization -n kubernaut-gateway

# Apply changes
kubectl apply -k deploy/gateway/

# Monitor rollout
kubectl rollout status deployment/gateway -n kubernaut-gateway
```

### Rollback

```bash
# Rollback to previous version
kubectl rollout undo deployment/gateway -n kubernaut-gateway

# Check rollout history
kubectl rollout history deployment/gateway -n kubernaut-gateway
```

---

## 📚 References

- [Implementation Plan](../../docs/services/stateless/gateway-service/IMPLEMENTATION_PLAN_V2.17.md)
- [ADR-027: Multi-Architecture Builds](../../docs/architecture/decisions/ADR-027-multi-architecture-build-strategy.md)
- [ADR-028: Container Registry Policy](../../docs/architecture/decisions/ADR-028-container-registry-policy.md)
- [Gateway Service Documentation](../../docs/services/stateless/gateway-service/)

---

## 🆘 Support

For issues or questions:

1. Check logs: `kubectl logs -n kubernaut-gateway -l app.kubernetes.io/component=gateway`
2. Check metrics: Port-forward to `:9090/metrics`
3. Review [Implementation Plan](../../docs/services/stateless/gateway-service/IMPLEMENTATION_PLAN_V2.17.md)
4. File an issue in the Kubernaut repository

