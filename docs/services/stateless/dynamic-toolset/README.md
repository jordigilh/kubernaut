# Dynamic Toolset Service - Documentation Hub

**Version**: 1.0
**Last Updated**: 2025-10-10
**Service Type**: Stateless HTTP API
**Status**: ✅ Documentation Complete, Ready for Implementation

---

## 📋 Quick Navigation

1. **[overview.md](./overview.md)** - Service architecture, automatic discovery, design decisions
2. **[api-specification.md](./api-specification.md)** - ConfigMap management API

---

## 🎯 Purpose

**Automatically discover and configure HolmesGPT toolsets.**

**Dynamic configuration** that provides:
- Automatic Kubernetes resource discovery
- HolmesGPT toolset generation
- ConfigMap-based hot-reload
- Toolset validation and compatibility checks

---

## 🔌 Service Configuration

| Aspect | Value |
|--------|-------|
| **HTTP Port** | 8080 (REST API, `/health`, `/ready`) |
| **Metrics Port** | 9090 (Prometheus `/metrics` with auth) |
| **Namespace** | `prometheus-alerts-slm` |
| **ServiceAccount** | `dynamic-toolset-sa` |

---

## 📊 API Endpoints

| Endpoint | Method | Purpose | Latency Target |
|----------|--------|---------|----------------|
| `/api/v1/toolsets/discover` | POST | Discover available K8s resources | < 300ms |
| `/api/v1/toolsets/generate` | POST | Generate toolset configuration | < 200ms |
| `/api/v1/toolsets/validate` | POST | Validate toolset compatibility | < 100ms |

---

## 🔍 Discovery Capabilities

**Automatically Discovers**:
- Available namespaces
- Deployments, StatefulSets, DaemonSets
- Services and Ingresses
- ConfigMaps and Secrets (metadata only)
- Prometheus instances
- Grafana instances

---

## 🎯 Key Features

- ✅ Automatic resource discovery
- ✅ ConfigMap generation for HolmesGPT
- ✅ Hot-reload support (HolmesGPT watches ConfigMaps)
- ✅ Toolset validation
- ✅ RBAC-aware discovery (only shows accessible resources)

---

## 🔗 Integration Points

**Clients**:
1. **HolmesGPT API** - Reads generated toolset ConfigMaps

**Generates**:
- ConfigMaps in `prometheus-alerts-slm` namespace
- Format: HolmesGPT toolset configuration

---

## 📊 Performance

- **Latency**: < 300ms (p95)
- **Throughput**: 5 requests/second
- **Scaling**: 1-2 replicas
- **Discovery Interval**: Every 5 minutes (configurable)

---

## 🚀 Getting Started

### Prerequisites

Before using Dynamic Toolset Service, ensure you have:
- Kubernetes cluster (v1.24+)
- kubectl configured with cluster access
- ServiceAccount with appropriate RBAC permissions
- HolmesGPT API deployed (optional for validation)

### Installation

**Step 1: Deploy the service**
```bash
# Apply RBAC and deployment manifests
kubectl apply -f deploy/dynamic-toolset-deployment.yaml
kubectl apply -f deploy/dynamic-toolset-rbac.yaml

# Verify deployment
kubectl get pods -n kubernaut-system -l app=dynamic-toolset
```

**Step 2: Verify service is running**
```bash
# Check health endpoint
kubectl port-forward -n kubernaut-system svc/dynamic-toolset 8080:8080
curl http://localhost:8080/health
# Expected: {"status":"healthy","timestamp":"..."}
```

**Step 3: Verify discovery is working**
```bash
# Check discovered services (requires authentication)
export TOKEN=$(kubectl create token dynamic-toolset-sa -n kubernaut-system)
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/services/discovered
```

### First Discovery Example

**Scenario**: Discover Prometheus instance in your cluster

```bash
# The service automatically discovers Prometheus every 5 minutes
# To trigger immediate discovery, check the logs:
kubectl logs -n kubernaut-system -l app=dynamic-toolset --tail=50

# Expected log output:
# {"level":"info","msg":"Service discovery complete","discovered_count":1,"prometheus_count":1}
```

**Verify ConfigMap was created**:
```bash
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml
```

Expected output:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-toolset-config
data:
  prometheus-toolset.yaml: |
    toolset: prometheus
    enabled: true
    config:
      url: "http://prometheus.monitoring:9090"
```

### Verification Steps

**1. Check service discovery metrics**:
```bash
kubectl port-forward -n kubernaut-system svc/dynamic-toolset 9090:9090
curl http://localhost:9090/metrics | grep toolset_services_discovered_total
```

**2. Verify HolmesGPT can read the toolsets**:
```bash
# HolmesGPT automatically reads the ConfigMap
# Check HolmesGPT logs for toolset loading confirmation
kubectl logs -n kubernaut-system -l app=holmesgpt | grep "toolset"
```

---

## 📖 Common Workflows

### Workflow 1: Discover Services

**Use Case**: Automatically discover all supported services in your cluster

**Steps**:
1. Deploy service (see Getting Started)
2. Service discovers automatically every 5 minutes
3. Check discovered services via API or logs

**API Method** (on-demand discovery):
```bash
export TOKEN=$(kubectl create token dynamic-toolset-sa -n kubernaut-system)

# Trigger discovery
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8080/api/v1/services/discover

# Response:
{
  "discovered": [
    {
      "name": "prometheus",
      "namespace": "monitoring",
      "type": "prometheus",
      "endpoint": "http://prometheus.monitoring:9090",
      "healthy": true
    }
  ]
}
```

**Automated Method** (controller loop):
```bash
# Discovery happens automatically
# Monitor via logs:
kubectl logs -f -n kubernaut-system -l app=dynamic-toolset
```

---

### Workflow 2: Generate Toolsets

**Use Case**: Generate HolmesGPT toolset configuration from discovered services

**Automatic Generation** (default):
```bash
# Service generates toolsets automatically after discovery
# Check the ConfigMap:
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml
```

**Manual Generation** (via API):
```bash
export TOKEN=$(kubectl create token dynamic-toolset-sa -n kubernaut-system)

# Generate toolsets for specific services
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "services": [
      {
        "name": "prometheus",
        "namespace": "monitoring",
        "type": "prometheus"
      }
    ]
  }' \
  http://localhost:8080/api/v1/toolsets/generate

# Response includes generated YAML configuration
```

---

### Workflow 3: Validate Configurations

**Use Case**: Validate toolset configuration before deploying

```bash
export TOKEN=$(kubectl create token dynamic-toolset-sa -n kubernaut-system)

# Validate toolset configuration
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "toolset": "prometheus",
    "config": {
      "url": "http://prometheus.monitoring:9090"
    }
  }' \
  http://localhost:8080/api/v1/toolsets/validate

# Response:
{
  "valid": true,
  "checks": [
    {"check": "url_format", "passed": true},
    {"check": "endpoint_reachable", "passed": true},
    {"check": "health_check", "passed": true}
  ]
}
```

---

### Workflow 4: Manual Override

**Use Case**: Add custom toolset or override auto-generated configuration

**Step 1**: Create override configuration
```yaml
# override-toolset.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-toolset-config
  namespace: kubernaut-system
data:
  # Add to existing ConfigMap
  overrides.yaml: |
    custom-datadog:
      enabled: true
      toolset: datadog
      config:
        api_key: "${DATADOG_API_KEY}"
        site: "datadoghq.com"
```

**Step 2**: Apply the override
```bash
kubectl patch configmap kubernaut-toolset-config \
  -n kubernaut-system \
  --type merge \
  -p "$(cat override-toolset.yaml)"
```

**Step 3**: Verify override is preserved
```bash
# The service reconciles every 30 seconds
# Your override will be preserved in the "overrides.yaml" key
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml
```

**Result**: Auto-generated toolsets are maintained, your override is preserved.

---

## 🔧 API Quick Reference

### Authentication

All API endpoints (except health checks) require authentication:

```bash
# Get service account token
export TOKEN=$(kubectl create token dynamic-toolset-sa -n kubernaut-system --duration=1h)

# Use in requests
curl -H "Authorization: Bearer $TOKEN" <endpoint>
```

### Health Endpoints (No Auth)

```bash
# Health check (liveness probe)
curl http://localhost:8080/health
# Response: {"status":"healthy"}

# Ready check (readiness probe)
curl http://localhost:8080/ready
# Response: {"status":"ready"}
```

### Discovery Endpoints (Auth Required)

**GET /api/v1/services/discovered** - List discovered services
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/services/discovered
```

**POST /api/v1/services/discover** - Trigger discovery
```bash
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/services/discover
```

### ConfigMap Endpoints (Auth Required)

**GET /api/v1/configmap** - Get current ConfigMap
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/configmap
```

**POST /api/v1/toolsets/generate** - Generate toolset
```bash
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"services":[...]}' \
  http://localhost:8080/api/v1/toolsets/generate
```

**POST /api/v1/toolsets/validate** - Validate toolset
```bash
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"toolset":"prometheus","config":{...}}' \
  http://localhost:8080/api/v1/toolsets/validate
```

### Common Response Codes

| Code | Meaning | Action |
|------|---------|--------|
| 200 | Success | Request completed |
| 401 | Unauthorized | Check token is valid |
| 403 | Forbidden | Check RBAC permissions |
| 500 | Server Error | Check service logs |
| 503 | Service Unavailable | Service starting up |

---

## 🔍 Troubleshooting

### Common Issues

#### Issue: Service not discovering any services

**Symptoms**:
```bash
kubectl logs -n kubernaut-system -l app=dynamic-toolset
# Output: "discovered_count":0
```

**Causes & Solutions**:

1. **No services match detection criteria**
   ```bash
   # Check if Prometheus/Grafana exist
   kubectl get svc --all-namespaces | grep -E "prometheus|grafana"

   # If exists, check labels
   kubectl get svc prometheus -n monitoring -o yaml | grep -A5 labels
   ```

2. **RBAC permissions insufficient**
   ```bash
   # Verify ServiceAccount can list services
   kubectl auth can-i list services \
     --as=system:serviceaccount:kubernaut-system:dynamic-toolset-sa \
     --all-namespaces
   ```

3. **Health checks failing**
   ```bash
   # Check if services are actually healthy
   kubectl port-forward -n monitoring svc/prometheus 9090:9090
   curl http://localhost:9090/-/healthy
   ```

---

#### Issue: ConfigMap keeps getting reset

**Symptoms**: Manual edits to ConfigMap are overwritten

**Solution**: Use `overrides.yaml` key for manual configurations
```bash
# Don't edit auto-generated keys (prometheus-toolset.yaml, grafana-toolset.yaml)
# Add your customizations to overrides.yaml instead
kubectl patch configmap kubernaut-toolset-config -n kubernaut-system \
  --type json \
  -p '[{"op":"add","path":"/data/overrides.yaml","value":"custom-config: here"}]'
```

---

#### Issue: API returns 401 Unauthorized

**Symptoms**:
```bash
curl http://localhost:8080/api/v1/services/discovered
# Response: {"error":"unauthorized"}
```

**Solutions**:

1. **Token expired**
   ```bash
   # Create new token
   export TOKEN=$(kubectl create token dynamic-toolset-sa -n kubernaut-system)
   ```

2. **Wrong ServiceAccount**
   ```bash
   # Verify ServiceAccount exists
   kubectl get sa dynamic-toolset-sa -n kubernaut-system
   ```

3. **TokenReview not configured**
   ```bash
   # Check service logs for TokenReview errors
   kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep TokenReview
   ```

---

#### Issue: High memory usage

**Symptoms**: Service using > 128MB memory

**Investigation**:
```bash
# Check current usage
kubectl top pod -n kubernaut-system -l app=dynamic-toolset

# Check discovery cache size
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep cache_size
```

**Solutions**:
1. Reduce discovery interval in configuration
2. Limit namespaces to monitor
3. Increase memory limits if justified

---

### Getting Help

**Check service logs**:
```bash
kubectl logs -n kubernaut-system -l app=dynamic-toolset --tail=100
```

**Check service status**:
```bash
kubectl describe pod -n kubernaut-system -l app=dynamic-toolset
```

**Enable debug logging**:
```bash
kubectl set env deployment/dynamic-toolset \
  -n kubernaut-system \
  LOG_LEVEL=debug
```

**Check metrics**:
```bash
kubectl port-forward -n kubernaut-system svc/dynamic-toolset 9090:9090
curl http://localhost:9090/metrics | grep toolset_
```

---

## 📚 Documentation Index

### Getting Started (You Are Here)
- ✅ Installation and prerequisites
- ✅ First discovery example
- ✅ Common workflows
- ✅ API quick reference
- ✅ Troubleshooting

### Core Documentation
1. **[overview.md](./overview.md)** (626 lines) - Architecture, service discovery pipeline, design decisions
2. **[implementation.md](./implementation.md)** (1,338 lines) - Package structure, detailed implementation
3. **[testing-strategy.md](./testing-strategy.md)** (1,430 lines) - Comprehensive testing approach

### API & Integration
4. **[api-specification.md](./api-specification.md)** (594 lines) - Complete API reference with schemas
5. **[integration-points.md](./integration-points.md)** (662 lines) - Integration with HolmesGPT and other services

### Deep Dives
6. **[service-discovery.md](./service-discovery.md)** (469 lines) - Discovery algorithm details
7. **[configmap-reconciliation.md](./configmap-reconciliation.md)** (574 lines) - Reconciliation and drift detection
8. **[toolset-generation.md](./toolset-generation.md)** (648 lines) - Toolset configuration generation

### Operational
9. **[metrics-slos.md](./metrics-slos.md)** (453 lines) - Prometheus metrics and SLOs
10. **[observability-logging.md](./observability-logging.md)** (547 lines) - Structured logging patterns
11. **[security-configuration.md](./security-configuration.md)** (582 lines) - Security and RBAC

### Implementation Tracking
12. **[implementation/](./implementation/)** - Phase 0 plan, testing strategy, design decisions

---

## 📞 Quick Links

- **Parent**: [../README.md](../README.md) - All stateless services
- **Consumer**: [../holmesgpt-api/](../holmesgpt-api/) - Uses generated toolsets
- **Architecture**: [../../architecture/](../../architecture/)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete

