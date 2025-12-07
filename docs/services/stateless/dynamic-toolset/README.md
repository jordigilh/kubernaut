# Dynamic Toolset Service

**Version**: V2.0 DEFERRED (Code Preserved)
**Last Updated**: December 7, 2025
**Service Type**: Stateless Controller (Discovery Loop)
**Status**: 🚫 **DEFERRED TO V2.0** - See [DD-016](../../../architecture/decisions/DD-016-dynamic-toolset-v2-deferral.md)

---

## ⚠️ **V2.0 DEFERRAL NOTICE**

This service is **DEFERRED to V2.0** per Design Decision [DD-016](../../../architecture/decisions/DD-016-dynamic-toolset-v2-deferral.md).

**Rationale**: V1.x only requires Prometheus integration, which HolmesGPT-API already handles with built-in service discovery logic. Dynamic Toolset Service will **become relevant in V2.0** when expanding HolmesGPT-API to identify multiple observability services (Grafana, Jaeger, Elasticsearch, custom services).

**Status**:
- ✅ Code and tests **PRESERVED** in repository for V2.0
- ❌ **NOT INCLUDED** in V1.x releases
- ❌ CI/CD tests **EXCLUDED** from V1.x pipelines
- ✅ Will **RETURN** in V2.0 for multi-service observability

**What This Means**:
- **DO NOT** deploy this service in V1.x environments
- **DO NOT** include in V1.x release planning
- **DO** preserve code for V2.0 development
- **DO** review when V2.0 multi-service observability is prioritized

---

## 🎯 Executive Summary

The **Dynamic Toolset Service** was designed to automatically discover Kubernetes services (Prometheus, Grafana, Jaeger, Elasticsearch, custom) and generate HolmesGPT-compatible toolset ConfigMaps.

**V2.0 Deferral Summary**:
- 🚫 **DEFERRED TO V2.0** per [DD-016](../../../architecture/decisions/DD-016-dynamic-toolset-v2-deferral.md)
- ✅ **Code PRESERVED** - All implementation, tests, and documentation remain in repository
- ❌ **NOT DEPLOYED** - Excluded from V1.x releases and CI/CD pipelines
- ⏳ **V2.0 TARGET** - Will return when HolmesGPT-API expands to multi-service observability

**Implementation Status** (Preserved for V2.0):
- ✅ **245/245 tests passing** (100%) - 194 unit + 38 integration + 13 E2E tests
- ✅ **E2E tests complete** - 2m37s execution time with parallel execution
- ✅ **Deployment manifests complete** - Kubernetes manifests ready for V2.0
- ✅ **Operations runbook complete** - Comprehensive troubleshooting guide
- ✅ **8/8 business requirements** (100% coverage)

**Why Deferred**:
- **V1.x Scope**: Prometheus-only observability (HolmesGPT-API already handles Prometheus discovery)
- **V2.0 Value**: Multi-service observability (Grafana, Jaeger, Elasticsearch, custom services)
- **Current Redundancy**: HolmesGPT-API's built-in Prometheus discovery makes this service redundant for V1.x

---

## 📋 Quick Navigation

### Getting Started
- [Deployment](#-deployment) - In-cluster Kubernetes deployment
- [Prerequisites](#prerequisites) - Requirements for deployment
- [Configuration](#️-configuration) - ConfigMap and environment settings
- [Operations Runbook](./OPERATIONS_RUNBOOK.md) - Troubleshooting and operations

### Core Documentation
1. **[Operations Runbook](./OPERATIONS_RUNBOOK.md)** - Production operations and troubleshooting guide
2. **[BR Coverage Matrix](./BR_COVERAGE_MATRIX.md)** - Business requirement traceability (8 BRs, 100% coverage)
3. **[Production Readiness Assessment](./PRODUCTION_READINESS_ASSESSMENT.md)** - Deployment readiness evaluation
4. **[Implementation Plan](./implementation/IMPLEMENTATION_PLAN_ENHANCED.md)** - 12-day implementation timeline
5. **[Testing Strategy](./implementation/testing/TESTING_STRATEGY.md)** - Comprehensive test approach

### Design Decisions
- **[DD-TOOLSET-001](./implementation/design/01-detector-interface-design.md)** - Detector interface design
- **[DD-TOOLSET-002](./implementation/design/DD-TOOLSET-002-discovery-loop-architecture.md)** - Discovery loop (periodic vs. watch)
- **[DD-TOOLSET-003](./implementation/design/DD-TOOLSET-003-reconciliation-strategy.md)** - Reconciliation strategy
- **[ConfigMap Schema](./implementation/design/02-configmap-schema-validation.md)** - Schema validation

---

## 🎯 What It Does

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
| **HTTP Port** | 8080 (`/health`, `/ready` only) |
| **Metrics Port** | 9090 (Prometheus `/metrics`) |
| **Namespace** | `kubernaut-system` |
| **ServiceAccount** | `dynamic-toolset-sa` |

---

## 📊 Health & Metrics Endpoints

| Endpoint | Method | Purpose | Port |
|----------|--------|---------|------|
| `/health` | GET | Liveness probe | 8080 |
| `/ready` | GET | Readiness probe (DD-007 graceful shutdown) | 8080 |
| `/metrics` | GET | Prometheus metrics | 9090 |

**Note**: REST API endpoints are **disabled in V1.0**. The service operates as a discovery loop controller that automatically updates ConfigMaps. For toolset introspection, use:
```bash
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml
```

---

## 🔍 Discovery Capabilities

**Automatically Discovers Kubernetes Services**:
- **Prometheus** - Label-based detection (`app=prometheus`)
- **Grafana** - Label-based detection (`app=grafana`)
- **Jaeger** - Annotation-based detection (`jaegertracing`)
- **Elasticsearch** - Label-based detection (`app=elasticsearch`)
- **Custom Services** - Annotation-based detection (`kubernaut.io/toolset=enabled`)

**Discovery Method**: Scans Kubernetes Services (not Deployments/Pods) in configured namespaces
**Output**: HolmesGPT-compatible toolset JSON in ConfigMap `kubernaut-toolset-config`

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
1. **HolmesGPT API** - Reads generated toolset ConfigMaps for dynamic tool discovery

**Generates**:
- **ConfigMap Name**: `kubernaut-toolset-config`
- **Namespace**: `kubernaut-system` (configurable)
- **Format**: JSON toolset configuration compatible with HolmesGPT
- **Update Frequency**: Every 5 minutes (production) or 10 seconds (E2E tests)

---

## 📊 Performance Characteristics

**Controller Behavior**:
- **Discovery Interval**: 5 minutes (production), 10 seconds (E2E tests) - configurable
- **Discovery Loop Execution**: Typically completes in < 5 seconds for 100 services
- **ConfigMap Reconciliation**: < 1 second for updates
- **Scaling**: Single replica (stateless controller, can run multiple for HA)

**Resource Usage** (typical):
- **Memory**: ~50-100Mi
- **CPU**: ~0.1 cores (spikes during discovery)

**Note**: No REST API latency metrics - service operates as a background discovery loop controller

---

## 🚀 Deployment

### Deployment

**V1.0 Deployment Mode**: **In-Cluster Only** ✅

- **Status**: Production Ready (245/245 tests passing, manifests complete)
- **Container Image**: Built from `docker/dynamic-toolset-ubi9.Dockerfile`
- **Access**: Uses ServiceAccount with RBAC
- **Deployment**: `deploy/dynamic-toolset-deployment.yaml`
- **Includes**: Namespace, ServiceAccount, ClusterRole, ClusterRoleBinding, ConfigMap, Deployment, Service, ServiceMonitor, NetworkPolicy

```bash
# Build container image
docker build -t kubernaut/dynamic-toolset:v1.0 -f docker/dynamic-toolset-ubi9.Dockerfile .

# Push to registry (if needed)
docker push kubernaut/dynamic-toolset:v1.0

# Deploy to Kubernetes
kubectl apply -f deploy/dynamic-toolset-deployment.yaml

# Verify deployment
kubectl get pods -n kubernaut-system -l app=dynamic-toolset
kubectl logs -n kubernaut-system -l app=dynamic-toolset
```

---

### Prerequisites

- **Kubernetes cluster** (v1.24+)
- **kubectl** configured with cluster-admin access
- **Docker** or **Podman** for building container images
- **Container registry** access (or use local registry)

#### Verification Steps

**1. Check health endpoints**:
```bash
curl http://localhost:8080/health
# Expected: {"status":"ok"}

curl http://localhost:8080/ready
# Expected: {"kubernetes":true}
```

**2. Verify Kubernetes connectivity**:
```bash
# Service logs should show successful K8s connection
# Look for: "Kubernetes client initialized successfully"
```

**3. Verify service discovery**:
```bash
# Check the generated ConfigMap
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml

# View discovered services
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o jsonpath='{.data.toolset\.json}' | jq .
```

**Note**: REST API endpoints are disabled in v1.0. The service operates as a discovery loop controller that automatically updates ConfigMaps. Use `kubectl` to inspect the generated toolset configuration.

**4. Check metrics**:
```bash
curl http://localhost:9090/metrics | grep dynamictoolset
# Should see discovery, API, and ConfigMap metrics
```

**5. Verify ConfigMap generation**:
```bash
# Check if ConfigMap was created
kubectl get configmap kubernaut-toolset-config -n kubernaut-system

# View ConfigMap contents
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml
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
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: dynamic-toolset
data:
  toolset.json: |
    [
      {
        "name": "prometheus",
        "namespace": "monitoring",
        "type": "prometheus",
        "endpoint": "http://prometheus.monitoring.svc.cluster.local:9090",
        "labels": {"app": "prometheus"},
        "healthy": true,
        "last_check": "2025-11-11T10:00:00Z"
      }
    ]
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

**Automated Method** (controller loop):
```bash
# Discovery happens automatically every 5 minutes (configurable)
# Monitor via logs:
kubectl logs -f -n kubernaut-system -l app=dynamic-toolset

# Check discovered services in ConfigMap:
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o jsonpath='{.data.toolset\.json}' | jq .
```

**Note**: REST API endpoints are disabled in V1.0 per DD-TOOLSET-001. Discovery is fully automatic.

---

### Workflow 2: View Generated Toolsets

**Use Case**: Inspect HolmesGPT toolset configuration generated from discovered services

```bash
# View full ConfigMap
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml

# View toolset JSON
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o jsonpath='{.data.toolset\.json}' | jq .

# Watch for changes
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -w
```

**Note**: Toolset generation is fully automatic. The controller generates toolsets immediately after discovery.

---

### Workflow 3: Add Custom ConfigMap Data

**Use Case**: Add custom data to ConfigMap (preserved during reconciliation)

**Step 1**: Add custom key to ConfigMap
```bash
kubectl patch configmap kubernaut-toolset-config \
  -n kubernaut-system \
  --type merge \
  -p '{"data":{"custom-config":"your custom data here"}}'
```

**Step 2**: Verify custom key is preserved
```bash
# The controller reconciles every 30 seconds
# Your custom keys (anything except "toolset.json") are preserved
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml
```

**Note**: The controller only manages the `toolset.json` key. All other keys in the ConfigMap are preserved during reconciliation (BR-TOOLSET-030).

---

## 🔧 Quick Reference

### Health Endpoints

```bash
# Health check (liveness probe)
curl http://localhost:8080/health
# Response: {"status":"ok"}

# Ready check (readiness probe)
curl http://localhost:8080/ready
# Response: {"status":"ready","checks":{"kubernetes":true,"configmap":true}}
```

### Metrics Endpoint

```bash
# Prometheus metrics (port 9090)
curl http://localhost:9090/metrics | grep dynamictoolset_
```

### ConfigMap Introspection

```bash
# View full ConfigMap
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml

# View discovered services (toolset JSON)
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o jsonpath='{.data.toolset\.json}' | jq .

# Watch for changes
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -w
```

### Service Logs

```bash
# View controller logs
kubectl logs -f -n kubernaut-system -l app=dynamic-toolset

# Filter for discovery events
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep "discovery"

# Filter for errors
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep "error"
```

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

#### Issue: ConfigMap toolset.json keeps getting reset

**Symptoms**: Manual edits to `toolset.json` key are overwritten

**Solution**: Don't edit `toolset.json` directly - add custom keys instead
```bash
# The controller manages the "toolset.json" key
# Add your custom data to separate keys (preserved during reconciliation)
kubectl patch configmap kubernaut-toolset-config -n kubernaut-system \
  --type merge \
  -p '{"data":{"custom-config":"your data here"}}'
```

**Note**: The controller only updates the `toolset.json` key. All other ConfigMap keys are preserved (BR-TOOLSET-030).

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

---

## 📖 API Reference

### V1.0 Endpoints

The Dynamic Toolset Service operates as a **discovery loop controller** in V1.0. Only health and metrics endpoints are exposed.

#### 1. GET /health

**Purpose**: Liveness probe for Kubernetes
**Authentication**: None
**Response Time**: < 10ms

**Request**:
```bash
curl http://localhost:8080/health
```

**Success Response** (200 OK):
```json
{
  "status": "ok"
}
```

**Use Cases**:
- Kubernetes liveness probe
- Load balancer health check
- Basic connectivity test

---

#### 2. GET /ready

**Purpose**: Readiness probe for Kubernetes
**Authentication**: None
**Response Time**: < 50ms

**Request**:
```bash
curl http://localhost:8080/ready
```

**Success Response** (200 OK):
```json
{
  "status": "ready",
  "checks": {
    "kubernetes": true,
    "configmap": true
  }
}
```

**Not Ready Response** (503 Service Unavailable):
```json
{
  "status": "not_ready",
  "checks": {
    "kubernetes": true,
    "configmap": false
  },
  "message": "ConfigMap not accessible"
}
```

**Use Cases**:
- Kubernetes readiness probe
- Check if service can accept traffic
- Verify Kubernetes API connectivity

---

#### 3. GET /metrics

**Purpose**: Prometheus metrics
**Authentication**: None (V1.0)
**Response Time**: < 50ms
**Port**: 9090 (separate from API port 8080)

**Request**:
```bash
curl http://localhost:9090/metrics
```

**Success Response** (200 OK):
```text
# HELP dynamictoolset_discovery_cycles_total Total number of discovery cycles
# TYPE dynamictoolset_discovery_cycles_total counter
dynamictoolset_discovery_cycles_total 42

# HELP dynamictoolset_services_discovered_total Number of services discovered
# TYPE dynamictoolset_services_discovered_total gauge
dynamictoolset_services_discovered_total{type="prometheus"} 1
dynamictoolset_services_discovered_total{type="grafana"} 1

# HELP dynamictoolset_discovery_duration_seconds Discovery cycle duration
# TYPE dynamictoolset_discovery_duration_seconds histogram
dynamictoolset_discovery_duration_seconds_bucket{le="0.1"} 5
dynamictoolset_discovery_duration_seconds_bucket{le="0.5"} 30
dynamictoolset_discovery_duration_seconds_bucket{le="1.0"} 40
dynamictoolset_discovery_duration_seconds_bucket{le="5.0"} 42
dynamictoolset_discovery_duration_seconds_sum 45.2
dynamictoolset_discovery_duration_seconds_count 42

# HELP dynamictoolset_configmap_updates_total Number of ConfigMap updates
# TYPE dynamictoolset_configmap_updates_total counter
dynamictoolset_configmap_updates_total 38

# HELP dynamictoolset_discovery_errors_total Discovery errors
# TYPE dynamictoolset_discovery_errors_total counter
dynamictoolset_discovery_errors_total 0
```

**Use Cases**:
- Prometheus scraping
- Monitoring service health
- Alerting on discovery failures
- Performance analysis

---

### ConfigMap Introspection (V1.0)

Use `kubectl` to inspect discovered services and generated toolsets:

```bash
# View full ConfigMap
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml

# View toolset JSON
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o jsonpath='{.data.toolset\.json}' | jq .

# Watch for changes
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -w
```

### V1.1 REST API (Planned)

REST API endpoints will be introduced in V1.1 with ToolsetConfig CRD (BR-TOOLSET-044):
- `GET /api/v1/toolsets` - List toolsets
- `GET /api/v1/toolsets/{name}` - Get specific toolset
- `GET /api/v1/services` - List discovered services
- `POST /api/v1/discover` - Trigger discovery
- `POST /api/v1/toolsets/generate` - Generate toolset
- `POST /api/v1/toolsets/validate` - Validate toolset

---

## ⚙️ Configuration

### Environment Variables

Complete list of configuration options for the Dynamic Toolset Service.

#### Core Configuration

| Variable | Default | Description | Example |
|----------|---------|-------------|---------|
| **KUBECONFIG** | `~/.kube/config` | Path to kubeconfig file (V1 only) | `/root/.kube/config` |
| **PORT** | `8080` | HTTP API server port | `8080` |
| **METRICS_PORT** | `9090` | Prometheus metrics port | `9090` |
| **LOG_LEVEL** | `info` | Logging level (debug, info, warn, error) | `debug` |
| **LOG_FORMAT** | `json` | Log format (json, text) | `json` |

#### Discovery Configuration

| Variable | Default | Description | Example |
|----------|---------|-------------|---------|
| **DISCOVERY_INTERVAL** | `5m` | Discovery loop interval | `1m`, `30s`, `10m` |
| **NAMESPACES** | `""` (all) | Comma-separated namespaces to monitor | `monitoring,observability` |
| **ENABLE_PROMETHEUS** | `true` | Enable Prometheus detector | `true` |
| **ENABLE_GRAFANA** | `true` | Enable Grafana detector | `true` |
| **ENABLE_JAEGER** | `true` | Enable Jaeger detector | `true` |
| **ENABLE_ELASTICSEARCH** | `true` | Enable Elasticsearch detector | `true` |
| **ENABLE_CUSTOM** | `true` | Enable custom annotation detector | `true` |
| **HEALTH_CHECK_TIMEOUT** | `5s` | Timeout for service health checks | `10s` |

#### ConfigMap Configuration

| Variable | Default | Description | Example |
|----------|---------|-------------|---------|
| **CONFIGMAP_NAME** | `kubernaut-toolset-config` | ConfigMap name | `my-toolset-config` |
| **CONFIGMAP_NAMESPACE** | `kubernaut-system` | ConfigMap namespace | `default` |
| **RECONCILIATION_INTERVAL** | `30s` | ConfigMap reconciliation interval | `1m` |

#### Authentication Configuration

**Note**: Authentication is disabled in V1.0. All endpoints are public. Authentication will be added in V1.1 with REST API endpoints.

| Variable | Default (V1.0) | Description | V1.1 Default |
|----------|----------------|-------------|--------------|
| **AUTH_ENABLED** | `false` | Enable API authentication (V1.1+) | `true` |
| **TOKEN_REVIEW_ENABLED** | `false` | Enable Kubernetes TokenReview (V1.1+) | `true` |
| **PUBLIC_ENDPOINTS** | `/health,/ready,/metrics` | All endpoints public in V1.0 | `/health,/ready` |

#### Performance Configuration

| Variable | Default | Description | Example |
|----------|---------|-------------|---------|
| **MAX_CONCURRENT_DETECTORS** | `5` | Max parallel detector executions | `10` |
| **DISCOVERY_TIMEOUT** | `30s` | Max discovery cycle duration | `60s` |
| **API_TIMEOUT** | `30s` | API request timeout | `60s` |
| **CACHE_SIZE** | `100` | Discovery cache size (services) | `200` |

### Configuration Files

#### V1 Configuration Example

```bash
# .env file for V1 deployment
PORT=8080
METRICS_PORT=9090
LOG_LEVEL=info
LOG_FORMAT=json

DISCOVERY_INTERVAL=5m
NAMESPACES=monitoring,observability,default

CONFIGMAP_NAME=kubernaut-toolset-config
CONFIGMAP_NAMESPACE=kubernaut-system

AUTH_ENABLED=true
TOKEN_REVIEW_ENABLED=true
```

#### V2 Configuration Example (ConfigMap)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: dynamic-toolset-config
  namespace: kubernaut-system
data:
  config.yaml: |
    server:
      port: 8080
      metrics_port: 9090
      log_level: info
      log_format: json

    discovery:
      interval: 5m
      namespaces:
        - monitoring
        - observability
        - default
      detectors:
        prometheus: true
        grafana: true
        jaeger: true
        elasticsearch: true
        custom: true

    configmap:
      name: kubernaut-toolset-config
      namespace: kubernaut-system
      reconciliation_interval: 30s

    authentication:
      enabled: true
      token_review_enabled: true
      public_endpoints:
        - /health
        - /ready
```

### Discovery Tuning

#### Interval Guidelines

| Interval | Use Case | API Load | Discovery Latency |
|----------|----------|----------|-------------------|
| **30s** | Development, rapid testing | High (120 calls/hour) | 0-30s |
| **1m** | Staging, frequent changes | Medium (60 calls/hour) | 0-60s |
| **5m** | Production (recommended) | Low (12 calls/hour) | 0-5min |
| **10m** | Large clusters, stable workloads | Very Low (6 calls/hour) | 0-10min |

**Recommendation**: Use 5-minute interval for production. Discovery latency of 0-5 minutes is acceptable for toolset updates.

#### Namespace Filtering

**Best Practice**: Limit discovery to specific namespaces to reduce API load and improve performance.

```bash
# Monitor only specific namespaces
export NAMESPACES="monitoring,observability,default"
```

**Benefits**:
- Reduced Kubernetes API load
- Faster discovery cycles
- Lower memory usage
- More predictable behavior

---

## 🛠️ Advanced Configuration

### Custom Detector Configuration

Add custom service detection via annotations:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-custom-service
  namespace: default
  annotations:
    kubernaut.io/discoverable: "true"
    kubernaut.io/tool-type: "custom_api"
    kubernaut.io/health-check: "/health"
spec:
  ports:
  - port: 8080
    name: http
  selector:
    app: my-custom-service
```

The service will be discovered as `custom_api` tool type with health check at `/health`.

### Override Configuration

**Use Case**: Override auto-discovered services or add manual tools

**Method**: Edit the `overrides.yaml` section in the ConfigMap

```bash
kubectl edit configmap kubernaut-toolset-config -n kubernaut-system
```

Add to `data.overrides.yaml`:
```yaml
tools:
  - name: custom_prometheus
    type: http
    description: Production Prometheus instance
    endpoint: http://prometheus.prod.example.com:9090
    parameters:
      timeout: 60s
      max_retries: 3

  - name: prometheus_query
    # This overrides the auto-discovered prometheus_query
    type: http
    endpoint: http://prometheus.prod.svc:9090
```

**Result**: Overrides take precedence over auto-discovered services. Both will be included in the toolset.

---

## 🔍 Troubleshooting

### Comprehensive Troubleshooting Guide

#### Issue 1: Service Not Discovering Any Services

**Symptoms**:
```bash
kubectl logs -n kubernaut-system -l app=dynamic-toolset
# Output: "discovered_count":0
```

**Diagnostic Steps**:

1. **Check if services exist**:
```bash
kubectl get svc --all-namespaces | grep -E "prometheus|grafana|jaeger|elasticsearch"
```

2. **Verify service labels/annotations**:
```bash
# For Prometheus
kubectl get svc prometheus-server -n monitoring -o yaml | grep -A10 "labels:"

# Expected label:
# labels:
#   app: prometheus
```

3. **Check RBAC permissions**:
```bash
kubectl auth can-i list services \
  --as=system:serviceaccount:kubernaut-system:dynamic-toolset \
  --all-namespaces

# Expected: yes
```

4. **Verify namespace filtering**:
```bash
# Check if NAMESPACES env var is limiting discovery
kubectl get deployment dynamic-toolset -n kubernaut-system -o yaml | grep NAMESPACES
```

5. **Test service health check**:
```bash
# Forward port and test health endpoint
kubectl port-forward -n monitoring svc/prometheus-server 9090:9090
curl http://localhost:9090/api/v1/query?query=up

# Should return valid Prometheus response
```

**Solutions**:
- Add required labels to services (`app: prometheus`, `app: grafana`, etc.)
- Grant RBAC permissions (see RBAC section)
- Add namespace to NAMESPACES filter
- Fix service health check endpoint

---

#### Issue 2: ConfigMap Not Being Created

**Symptoms**: ConfigMap `kubernaut-toolset-config` doesn't exist

**Diagnostic Steps**:

1. **Check ConfigMap existence**:
```bash
kubectl get configmap -n kubernaut-system
```

2. **Check service logs for errors**:
```bash
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep -i error
```

3. **Verify RBAC for ConfigMap operations**:
```bash
kubectl auth can-i create configmaps \
  --as=system:serviceaccount:kubernaut-system:dynamic-toolset \
  -n kubernaut-system

# Expected: yes
```

**Solutions**:
- Grant ConfigMap create/update permissions
- Check for namespace typos in configuration
- Verify service has discovered at least one service (required for ConfigMap creation)

---

#### Issue 3: ConfigMap Keeps Getting Reset (Manual Edits Lost)

**Symptoms**: Manual ConfigMap edits are overwritten after ~30 seconds

**Root Cause**: Editing auto-generated sections (`toolset.yaml`) instead of overrides section

**Solution**: Use `overrides.yaml` section for manual configurations

```bash
# Get current ConfigMap
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml > cm.yaml

# Edit cm.yaml - add your tools to data.overrides.yaml (NOT data.toolset.yaml)

# Apply
kubectl apply -f cm.yaml
```

**Correct Structure**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-toolset-config
data:
  toolset.yaml: |
    # ⚠️ DO NOT EDIT - Auto-generated, will be overwritten
    tools:
      - name: prometheus_query
        # ... auto-discovered

  overrides.yaml: |
    # ✅ EDIT HERE - Manual tools preserved
    tools:
      - name: custom_datadog
        type: http
        endpoint: http://datadog.monitoring:8080
```

---

#### Issue 4: API Returns 401 Unauthorized

**Symptoms**:
```bash
curl http://localhost:8080/api/v1/services
# Response: {"error":"unauthorized"}
```

**Diagnostic Steps**:

1. **Check if token is provided**:
```bash
# Token must be in Authorization header
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/services
```

2. **Verify token is valid**:
```bash
# Create fresh token
kubectl create token dynamic-toolset -n kubernaut-system --duration=1h

# Test with new token
TOKEN=$(kubectl create token dynamic-toolset -n kubernaut-system)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/services
```

3. **Check ServiceAccount exists**:
```bash
kubectl get sa dynamic-toolset -n kubernaut-system
```

4. **Verify TokenReview is working**:
```bash
# Check service logs for TokenReview errors
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep TokenReview
```

**Solutions**:
- Always include `Authorization: Bearer $TOKEN` header
- Use fresh tokens (tokens expire after specified duration)
- Verify ServiceAccount exists
- Check RBAC for TokenReview permissions

---

#### Issue 5: High Memory Usage (> 256MB)

**Symptoms**:
```bash
kubectl top pod -n kubernaut-system -l app=dynamic-toolset
# Output: NAME CPU(cores) MEMORY(bytes)
#         dynamic-toolset-xxx 100m 300Mi
```

**Diagnostic Steps**:

1. **Check discovery cache size**:
```bash
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep cache_size
```

2. **Check number of discovered services**:
```bash
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep discovered_count
```

3. **Check discovery interval**:
```bash
kubectl get deployment dynamic-toolset -n kubernaut-system -o yaml | grep DISCOVERY_INTERVAL
```

**Solutions**:

1. **Reduce discovery scope**:
```bash
# Limit namespaces
kubectl set env deployment/dynamic-toolset \
  -n kubernaut-system \
  NAMESPACES=monitoring,observability
```

2. **Increase discovery interval**:
```bash
# Reduce discovery frequency
kubectl set env deployment/dynamic-toolset \
  -n kubernaut-system \
  DISCOVERY_INTERVAL=10m
```

3. **Adjust memory limits** (if justified):
```bash
kubectl set resources deployment/dynamic-toolset \
  -n kubernaut-system \
  --limits=memory=512Mi \
  --requests=memory=256Mi
```

---

#### Issue 6: Discovery Taking Too Long (> 5 seconds)

**Symptoms**: Discovery cycle duration > 5 seconds for < 100 services

**Diagnostic Steps**:

1. **Check discovery duration metric**:
```bash
curl http://localhost:9090/metrics | grep discovery_duration
```

2. **Check number of services**:
```bash
kubectl get svc --all-namespaces | wc -l
```

3. **Check health check timeout**:
```bash
kubectl get deployment dynamic-toolset -n kubernaut-system -o yaml | grep HEALTH_CHECK_TIMEOUT
```

**Solutions**:

1. **Reduce health check timeout**:
```bash
kubectl set env deployment/dynamic-toolset \
  -n kubernaut-system \
  HEALTH_CHECK_TIMEOUT=2s
```

2. **Limit namespaces**:
```bash
kubectl set env deployment/dynamic-toolset \
  -n kubernaut-system \
  NAMESPACES=monitoring,observability
```

3. **Increase max concurrent detectors**:
```bash
kubectl set env deployment/dynamic-toolset \
  -n kubernaut-system \
  MAX_CONCURRENT_DETECTORS=10
```

---

### Debugging Techniques

#### Enable Debug Logging

```bash
kubectl set env deployment/dynamic-toolset \
  -n kubernaut-system \
  LOG_LEVEL=debug

# Watch logs
kubectl logs -f -n kubernaut-system -l app=dynamic-toolset
```

**Debug Log Output**:
```json
{"level":"debug","msg":"Starting discovery cycle","cycle":42}
{"level":"debug","msg":"Running detector","detector":"prometheus","namespace":"monitoring"}
{"level":"debug","msg":"Service detected","service":"prometheus-server","endpoint":"http://prometheus-server.monitoring.svc.cluster.local:9090"}
{"level":"debug","msg":"Health check passed","service":"prometheus-server","duration_ms":23}
{"level":"debug","msg":"Discovery cycle complete","discovered":2,"duration_ms":1234}
```

#### Check Prometheus Metrics

```bash
# Port-forward metrics port
kubectl port-forward -n kubernaut-system svc/dynamic-toolset 9090:9090

# Check discovery metrics
curl http://localhost:9090/metrics | grep dynamictoolset_discovery

# Check error rates
curl http://localhost:9090/metrics | grep dynamictoolset_discovery_errors_total
```

#### Trace API Requests

```bash
# Enable request tracing
kubectl set env deployment/dynamic-toolset \
  -n kubernaut-system \
  LOG_LEVEL=debug \
  TRACE_REQUESTS=true

# Watch API request logs
kubectl logs -f -n kubernaut-system -l app=dynamic-toolset | grep "method="
```

---

## 📚 Documentation Index

### Getting Started (You Are Here)
- ✅ Installation and prerequisites
- ✅ First discovery example
- ✅ Common workflows
- ✅ Complete API reference (6 endpoints)
- ✅ Configuration (30+ environment variables)
- ✅ Troubleshooting (6 detailed scenarios)

### Core Documentation
1. **[Implementation Plan](./implementation/IMPLEMENTATION_PLAN_ENHANCED.md)** - 12-day implementation timeline
2. **[BR Coverage Matrix](./BR_COVERAGE_MATRIX.md)** - Business requirement traceability (8/8 BRs, 232 tests)
3. **[Testing Strategy](./implementation/testing/TESTING_STRATEGY.md)** - Comprehensive test approach (100% pass rate)
4. **[Production Readiness](./implementation/PRODUCTION_READINESS_REPORT.md)** - 101/109 points (92.7%)
5. **[Handoff Summary](./implementation/00-HANDOFF-SUMMARY.md)** - Complete implementation summary

### Design Decisions
- **[DD-TOOLSET-001](./implementation/design/01-detector-interface-design.md)** - Detector interface design
- **[DD-TOOLSET-002](./implementation/design/DD-TOOLSET-002-discovery-loop-architecture.md)** - Discovery loop (periodic vs. watch)
- **[DD-TOOLSET-003](./implementation/design/DD-TOOLSET-003-reconciliation-strategy.md)** - Reconciliation strategy
- **[ConfigMap Schema](./implementation/design/02-configmap-schema-validation.md)** - Schema validation

### Testing Documentation
- **[Integration-First Rationale](./implementation/testing/01-integration-first-rationale.md)** - Why integration tests first
- **[E2E Test Plan (V2)](./implementation/testing/03-e2e-test-plan.md)** - E2E test plan for in-cluster deployment
- **[Testing Strategy](./implementation/testing/TESTING_STRATEGY.md)** - Comprehensive testing approach

### Implementation Tracking
- **[Day 7 Complete](./implementation/phase0/07-day7-complete.md)** - Schema validation & testing infrastructure
- **[Implementation Plan](./implementation/IMPLEMENTATION_PLAN_ENHANCED.md)** - Detailed 12-day timeline
- **[Deployment Manifests](../../deploy/dynamic-toolset/)** - Kubernetes YAML files (V1 ready)

---

## 📞 Quick Links

- **Parent**: [../README.md](../README.md) - All stateless services
- **Consumer**: [../holmesgpt-api/](../holmesgpt-api/) - Uses generated toolsets
- **Architecture**: [../../architecture/](../../architecture/)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete

