# Dynamic Toolset Service - Overview

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ Design Complete
**Service Type**: Stateless HTTP API + Kubernetes Controller
**Port**: 8080 (REST API + Health), 9090 (Metrics)

---

## Table of Contents

1. [Purpose & Scope](#purpose--scope)
2. [Architecture Overview](#architecture-overview)
3. [Service Discovery Pipeline](#service-discovery-pipeline)
4. [ConfigMap Management](#configmap-management)
5. [Key Architectural Decisions](#key-architectural-decisions)
6. [V1 Scope Boundaries](#v1-scope-boundaries)
7. [System Context Diagram](#system-context-diagram)

---

## Purpose & Scope

### Core Purpose

Dynamic Toolset Service is the **intelligent service discovery** engine for HolmesGPT investigations. It provides:

1. **Automatic service discovery** in Kubernetes cluster
2. **Toolset configuration generation** for HolmesGPT SDK
3. **ConfigMap reconciliation** to prevent drift and deletion
4. **Health validation** for discovered services
5. **Manual override support** for admin-configured toolsets

### Why Dynamic Toolset Service Exists

**Problem**: Without Dynamic Toolset Service, operators would need to:
- **Manually configure** every toolset in HolmesGPT
- **Update configuration** every time a service is added/removed
- **Track service endpoints** across namespace changes
- **Validate service health** before investigations

**Solution**: Dynamic Toolset Service provides **automatic discovery** that:
- ✅ Discovers Prometheus, Grafana, Jaeger, Elasticsearch automatically
- ✅ Generates HolmesGPT toolset configurations dynamically
- ✅ Validates service health before including in toolsets
- ✅ Reconciles ConfigMap to prevent accidental deletion or drift
- ✅ Preserves manual overrides in `overrides` section

---

## Architecture Overview

### Service Characteristics

- **Type**: Hybrid (HTTP API + Kubernetes Controller)
- **Deployment**: Kubernetes Deployment with 1-2 replicas (leader election)
- **State Management**: ConfigMap-based (no database)
- **Integration Pattern**: Service Watch → Discovery → ConfigMap Write → HolmesGPT API polls

### Component Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│               Dynamic Toolset Service                           │
│                                                                 │
│  ┌──────────────┐       ┌──────────────┐                      │
│  │   Service    │       │   ConfigMap  │                      │
│  │  Discovery   │       │ Reconciler   │                      │
│  │   Engine     │       │              │                      │
│  └──────┬───────┘       └──────┬───────┘                      │
│         │                      │                               │
│         │  ┌──────────────────────────────┐                   │
│         │  │  Service Detectors           │                   │
│         │  │  - Prometheus Detector       │                   │
│         │  │  - Grafana Detector          │                   │
│         │  │  - Jaeger Detector           │                   │
│         │  │  - Elasticsearch Detector    │                   │
│         │  │  - Custom Service Detector   │                   │
│         │  └───────────┬──────────────────┘                   │
│         │              │                                       │
│         └──────────────┴───────┐                               │
│                                │                               │
│                    ┌───────────▼──────────┐                    │
│                    │  Health Validator    │                    │
│                    │  - HTTP health check │                    │
│                    │  - Endpoint probe    │                    │
│                    └───────────┬──────────┘                    │
│                                │                               │
│                    ┌───────────▼──────────┐                    │
│                    │  Toolset Generator   │                    │
│                    │  - YAML generation   │                    │
│                    │  - Override merge    │                    │
│                    └───────────┬──────────┘                    │
│                                │                               │
│                                ▼                               │
│                      ConfigMap Write + Watch                   │
└─────────────────────────────────────────────────────────────────┘
         │                                    │
         │ Kubernetes API                     │ ConfigMap
         ▼                                    ▼
    ┌──────────┐                         ┌─────────────────┐
    │Services  │                         │kubernaut-toolset│
    │Prometheus│                         │-config          │
    │Grafana   │                         │(owned by        │
    │Jaeger    │                         │Dynamic Toolset) │
    │etc.      │                         └────────┬────────┘
    └──────────┘                                  │
                                                  │ Volume mount
                                                  ▼
                                         ┌─────────────────┐
                                         │HolmesGPT API    │
                                         │(file polling)   │
                                         └─────────────────┘
```

---

### High-Level Flow

```mermaid
sequenceDiagram
    participant K8s as Kubernetes API
    participant DTS as Dynamic Toolset Service
    participant CM as ConfigMap
    participant HG as HolmesGPT API

    loop Every 5 minutes (discovery)
        DTS->>K8s: List Services with annotations
        K8s-->>DTS: Service list
        DTS->>DTS: Detect Prometheus, Grafana, etc.
        DTS->>K8s: Health check discovered services
        K8s-->>DTS: Health status
        DTS->>DTS: Generate toolset YAML
        DTS->>CM: Write/Update ConfigMap
    end

    loop Every 30 seconds (reconciliation)
        DTS->>CM: Watch ConfigMap
        alt ConfigMap modified/deleted
            DTS->>DTS: Detect drift
            DTS->>CM: Reconcile to desired state
        end
    end

    loop Every 60 seconds (polling)
        HG->>CM: Poll mounted file
        CM-->>HG: Toolset configuration
        HG->>HG: Reload toolsets
    end
```

---

## Service Discovery Pipeline

### 1. Service Detection

**Input**: Kubernetes Service objects
**Processing**: Match services by annotations and labels
**Output**: List of discovered services

**Detection Criteria**:

#### Prometheus Detection
```yaml
# Service must have:
labels:
  app: prometheus
ports:
- name: web
  port: 9090
```

#### Grafana Detection
```yaml
labels:
  app: grafana
ports:
- name: service
  port: 3000
```

#### Jaeger Detection
```yaml
labels:
  app: jaeger
ports:
- name: query
  port: 16686
```

#### Custom Service Detection
```yaml
# Admin-defined services:
annotations:
  kubernaut.io/toolset: "true"
  kubernaut.io/toolset-type: "custom"
  kubernaut.io/toolset-name: "my-service"
```

---

### 2. Health Validation

**Input**: Discovered service endpoint
**Processing**: HTTP health check
**Output**: Healthy/Unhealthy status

**Health Check Strategy**:
- **Timeout**: 5 seconds
- **Retry**: 3 attempts with 1-second interval
- **Success Criteria**: HTTP 200 OK or 204 No Content

**Example**:
```go
func (d *Detector) HealthCheck(ctx context.Context, endpoint string) error {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"/health", nil)
    if err != nil {
        return err
    }

    resp, err := d.httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
        return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
    }

    return nil
}
```

---

### 3. Toolset Configuration Generation

**Input**: Validated services
**Processing**: Generate HolmesGPT toolset YAML
**Output**: ConfigMap data

**Example Output**:
```yaml
# ConfigMap: kubernaut-toolset-config
data:
  kubernetes-toolset.yaml: |
    toolset: kubernetes
    enabled: true
    config:
      incluster: true
      namespaces: ["*"]

  prometheus-toolset.yaml: |
    toolset: prometheus
    enabled: true
    config:
      url: "http://prometheus.monitoring:9090"
      timeout: "30s"

  grafana-toolset.yaml: |
    toolset: grafana
    enabled: true
    config:
      url: "http://grafana.monitoring:3000"
      apiKey: "${GRAFANA_API_KEY}"  # From Secret

  # Manual overrides preserved
  overrides.yaml: |
    custom-service:
      enabled: true
      config:
        url: "http://custom-service:8080"
```

---

## ConfigMap Management

### ConfigMap Ownership

**Owner**: Dynamic Toolset Service
**Name**: `kubernaut-toolset-config`
**Namespace**: `kubernaut-system`

**OwnerReference**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-toolset-config
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: apps/v1
    kind: Deployment
    name: dynamic-toolset
    controller: true
    blockOwnerDeletion: true
```

---

### Reconciliation Strategy

**Watch Interval**: 30 seconds
**Reconciliation Trigger**: ConfigMap modification or deletion

**Reconciliation Logic**:
1. **Drift Detection**: Compare current ConfigMap with desired state
2. **Override Preservation**: Merge `overrides.yaml` section
3. **Conflict Resolution**: Admin overrides take precedence
4. **Write-back**: Update ConfigMap to desired state

**Example**:
```go
func (r *Reconciler) Reconcile(ctx context.Context) error {
    // Get current ConfigMap
    currentCM, err := r.client.ConfigMaps("kubernaut-system").
        Get(ctx, "kubernaut-toolset-config", metav1.GetOptions{})

    if errors.IsNotFound(err) {
        // ConfigMap deleted → recreate
        return r.createConfigMap(ctx, r.desiredState)
    }

    // Detect drift
    if !r.isDesiredState(currentCM) {
        // Merge manual overrides
        merged := r.mergeOverrides(currentCM, r.desiredState)

        // Update ConfigMap
        return r.updateConfigMap(ctx, merged)
    }

    return nil
}
```

---

### Manual Override Support

**Admin Override Section**: `overrides.yaml`

**Example**:
```yaml
# Admin manually adds to ConfigMap
data:
  overrides.yaml: |
    custom-elasticsearch:
      enabled: true
      config:
        url: "http://elasticsearch.logging:9200"
        index: "logs-*"

    prometheus:
      enabled: false  # Temporarily disable
```

**Preservation Logic**:
- Dynamic Toolset Service **always preserves** `overrides.yaml` during reconciliation
- Admin changes to other sections are **overwritten** during reconciliation
- Admin must use `overrides.yaml` for permanent configuration changes

---

## Key Architectural Decisions

### Decision 1: Hybrid HTTP API + Controller

**Decision**: Dynamic Toolset Service runs as **both** HTTP API and Kubernetes Controller

**Rationale**:
- **Controller**: Watches services and reconciles ConfigMap (primary function)
- **HTTP API**: Manual toolset queries and health checks (secondary function)
- **Single Service**: Simplifies deployment and reduces resource usage

**Implications**:
- ✅ Single deployment for both functions
- ✅ Shared service discovery logic
- ✅ Simpler RBAC (one ServiceAccount)
- ⚠️ Requires leader election for multi-replica deployments

---

### Decision 2: File-Based ConfigMap Polling (HolmesGPT API)

**Decision**: HolmesGPT API polls **mounted ConfigMap file** instead of Kubernetes API watch

**Rationale**:
- **Simpler**: No Kubernetes client library in HolmesGPT API
- **No RBAC**: No ConfigMap read permissions needed
- **Efficient**: File system notifications (inotify) trigger reload
- **Acceptable Latency**: 60-120 seconds total latency for toolset changes

**Alternatives Considered**:
- ❌ **Kubernetes API Watch**: Complex, requires RBAC, Python Kubernetes client
- ❌ **HTTP Polling**: Dynamic Toolset Service would need HTTP API for toolsets
- ✅ **File-Based**: Simple, efficient, standard Kubernetes pattern

**Implications**:
- ✅ HolmesGPT API requires no Kubernetes API access
- ✅ ConfigMap changes reflect in 60-120 seconds (kubelet sync + file poll)
- ✅ Standard Kubernetes volume mount pattern

---

### Decision 3: ConfigMap Reconciliation (Not CRD)

**Decision**: Use **ConfigMap with reconciliation** instead of **Custom Resource Definition**

**Rationale**:
- **Simplicity**: ConfigMap is built-in, no CRD installation
- **Volume Mount**: Direct volume mount to HolmesGPT API pod
- **Admin Editable**: Admins can manually edit ConfigMap with `kubectl edit`
- **Reconciliation**: Protects against accidental deletion

**Alternatives Considered**:
- ❌ **CRD**: More complex, requires CRD installation, no direct volume mount
- ❌ **No Reconciliation**: ConfigMap could be deleted or corrupted
- ✅ **ConfigMap + Reconciliation**: Simple, protected, volume-mountable

**Implications**:
- ✅ Standard Kubernetes ConfigMap
- ✅ Admin-friendly (kubectl edit)
- ✅ Protected by reconciliation loop
- ⚠️ Manual edits outside `overrides.yaml` are overwritten

---

### Decision 4: 5-Minute Discovery Interval

**Decision**: Service discovery runs **every 5 minutes**

**Rationale**:
- **Infrequent Changes**: Services are added/removed infrequently
- **Resource Efficiency**: Avoid excessive Kubernetes API calls
- **Acceptable Delay**: 5 minutes is acceptable for new service availability

**Tunable**: Can be configured via environment variable if needed

**Implications**:
- ✅ Low Kubernetes API load
- ✅ Efficient resource usage
- ⚠️ New services take up to 5 minutes to be discovered

---

## V1 Scope Boundaries

### ✅ In Scope for V1

1. **Service Discovery**
   - Prometheus detection
   - Grafana detection
   - Jaeger detection (optional)
   - Elasticsearch detection (optional)
   - Custom service detection (annotations)

2. **Toolset Generation**
   - Kubernetes toolset (always enabled)
   - Prometheus toolset (auto-discovered)
   - Grafana toolset (auto-discovered)
   - Health validation

3. **ConfigMap Management**
   - ConfigMap creation
   - ConfigMap reconciliation (30s interval)
   - Manual override preservation
   - Owner reference protection

4. **REST API** (Optional)
   - GET /api/v1/toolsets (list discovered toolsets)
   - GET /api/v1/services (list discovered services)
   - POST /api/v1/discover (trigger manual discovery)

5. **Observability**
   - Prometheus metrics (discovery count, health status)
   - Structured logging
   - Health/readiness probes

---

### ❌ Out of Scope for V1

1. **Advanced Discovery**
   - Multi-cluster service discovery
   - External service discovery (outside Kubernetes)
   - Dynamic detector plugins

2. **Advanced Toolsets**
   - AlertManager toolset
   - Datadog toolset
   - AWS CloudWatch toolset
   - Custom toolset SDK

3. **UI/Dashboard**
   - Web UI for service discovery
   - Toolset configuration dashboard
   - Health monitoring dashboard

4. **Advanced Features**
   - Service dependency graph
   - Automatic toolset prioritization
   - A/B testing for toolsets
   - Toolset usage analytics

---

## System Context Diagram

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        subgraph "Monitoring Services"
            PROM[Prometheus<br/>:9090]
            GRAF[Grafana<br/>:3000]
            JAEGER[Jaeger<br/>:16686]
            ES[Elasticsearch<br/>:9200]
        end

        DTS[Dynamic Toolset Service]
        CM[ConfigMap<br/>kubernaut-toolset-config]
        HG[HolmesGPT API]

        K8S[Kubernetes API]
    end

    %% Service Discovery
    DTS -->|Watch Services| K8S
    DTS -->|Health Check| PROM
    DTS -->|Health Check| GRAF
    DTS -->|Health Check| JAEGER
    DTS -->|Health Check| ES

    %% ConfigMap Management
    DTS -->|Write/Reconcile| CM
    DTS -->|Watch| CM

    %% HolmesGPT Integration
    CM -->|Volume Mount| HG
    HG -->|Poll File| CM

    style DTS fill:#F0E68C
    style CM fill:#DDA0DD
    style HG fill:#90EE90
```

---

## Service Configuration

### Port Configuration
- **Port 8080**: REST API and health probes (follows kube-apiserver pattern)
- **Port 9090**: Metrics endpoint
- **Authentication**: Kubernetes TokenReviewer API (validates ServiceAccount tokens)

### ServiceAccount
- **Name**: `dynamic-toolset`
- **Namespace**: `kubernaut-system`
- **Purpose**: Service discovery, ConfigMap management, TokenReviewer authentication

### RBAC Requirements
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: dynamic-toolset
rules:
# Service discovery
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list", "watch"]

# ConfigMap management
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
  resourceNames: ["kubernaut-toolset-config"]

# Health checks (optional - can use HTTP directly)
- apiGroups: [""]
  resources: ["pods", "endpoints"]
  verbs: ["get", "list"]
```

---

## Performance Characteristics

### Target SLOs

| Metric | Target | Notes |
|--------|--------|-------|
| **Availability** | 99.5% | Per replica |
| **Discovery Latency** | < 10s | From service deployment to detection |
| **Reconciliation Latency** | < 5s | From ConfigMap modification to reconciliation |
| **API Response Time (p95)** | < 200ms | Manual toolset queries |
| **Memory Usage** | < 128MB | Per replica |
| **CPU Usage** | < 0.1 cores | Average |

---

## Failure Scenarios & Recovery

### Service Discovery Failures

#### Scenario 1: Kubernetes API Unavailable

**Symptoms**:
```
{"level":"error","msg":"Failed to list services","error":"connection refused"}
```

**Causes**:
- Network partition between service and Kubernetes API
- Kubernetes API server overloaded or restarting
- RBAC token expired or invalid

**Impact**:
- Discovery loop fails
- No new services discovered
- Existing ConfigMap remains unchanged (safe)

**Recovery**:
```bash
# Check Kubernetes API accessibility
kubectl get nodes
# If fails, investigate cluster connectivity

# Check service logs
kubectl logs -n kubernaut-system -l app=dynamic-toolset --tail=50

# Verify RBAC permissions
kubectl auth can-i list services \
  --as=system:serviceaccount:kubernaut-system:dynamic-toolset-sa
```

**Automatic Recovery**: Service retries every 5 minutes. No manual intervention needed if transient.

---

#### Scenario 2: All Health Checks Fail

**Symptoms**:
```
{"level":"warn","msg":"Service health check failed, skipping","service_type":"prometheus"}
```

**Causes**:
- Network policy blocking health check traffic
- Services actually unhealthy
- Health check timeout too aggressive (< 5s)

**Impact**:
- No services added to ConfigMap
- HolmesGPT has no toolsets available
- Investigations cannot proceed

**Recovery**:
```bash
# Test health checks manually
kubectl port-forward -n monitoring svc/prometheus 9090:9090
curl http://localhost:9090/-/healthy

# Check network policies
kubectl get networkpolicy -n monitoring
kubectl get networkpolicy -n kubernaut-system

# Temporarily disable health checks (not recommended for production)
kubectl set env deployment/dynamic-toolset \
  -n kubernaut-system \
  HEALTH_CHECK_ENABLED=false
```

**Prevention**: Ensure network policies allow egress from `kubernaut-system` to monitored namespaces.

---

#### Scenario 3: Partial Service Discovery

**Symptoms**:
```
{"level":"info","msg":"Service discovery complete","discovered_count":1,"prometheus_count":1,"grafana_count":0}
```

**Causes**:
- Grafana labels don't match detection criteria
- Grafana in namespace without proper RBAC access
- Grafana service type not recognized

**Impact**:
- Some toolsets available, others missing
- HolmesGPT investigations have limited capabilities

**Diagnosis**:
```bash
# Check if Grafana service exists
kubectl get svc --all-namespaces | grep grafana

# Check Grafana service labels
kubectl get svc grafana -n monitoring -o yaml | grep -A10 labels

# Test custom annotation-based discovery
kubectl annotate svc grafana -n monitoring \
  kubernaut.io/toolset=true \
  kubernaut.io/toolset-type=grafana
```

**Resolution**: Add proper labels to services or use annotation-based custom detector.

---

### ConfigMap Reconciliation Failures

#### Scenario 4: ConfigMap Deleted by Administrator

**Symptoms**:
```
{"level":"error","msg":"ConfigMap not found","name":"kubernaut-toolset-config"}
```

**Impact**:
- HolmesGPT loses all toolset configuration
- Investigations stop working

**Automatic Recovery**:
```bash
# Service detects missing ConfigMap within 30 seconds
# Automatically recreates with current discovered services
# Check logs:
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep "ConfigMap recreated"
```

**Manual Recovery** (if automatic fails):
```bash
# Trigger manual discovery
export TOKEN=$(kubectl create token dynamic-toolset-sa -n kubernaut-system)
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/services/discover
```

**Recovery Time**: < 30 seconds automatic, < 5 minutes full reconciliation

---

#### Scenario 5: ConfigMap Drift (Manual Edits)

**Symptoms**:
```
{"level":"info","msg":"Drift detected","keys":["prometheus-toolset.yaml"],"action":"reconciling"}
```

**Causes**:
- Administrator manually edited auto-generated keys
- ConfigMap modified by another tool
- Prometheus service endpoint changed

**Impact**:
- Manual edits overwritten by reconciliation
- Administrator changes lost

**Resolution**:
```bash
# Use overrides.yaml for manual configurations
kubectl patch configmap kubernaut-toolset-config -n kubernaut-system \
  --type json \
  -p '[{
    "op":"add",
    "path":"/data/overrides.yaml",
    "value":"custom-prometheus:\n  enabled: true\n  config:\n    url: \"http://custom:9090\"\n"
  }]'

# Verify override is preserved after reconciliation
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml | grep -A5 overrides
```

**Best Practice**: Never edit auto-generated keys. Always use `overrides.yaml`.

---

### High Availability Failures

#### Scenario 6: Leader Election Conflict (Multi-Replica)

**Symptoms**:
```
{"level":"warn","msg":"Lost leader election","instance":"dynamic-toolset-7d4f9-xyz"}
```

**Causes**:
- Network partition between replicas
- Leader pod crash during election
- Lease timeout too aggressive

**Impact**:
- Brief pause in discovery (< 30s)
- No data loss (ConfigMap persists)
- New leader takes over automatically

**Monitoring**:
```bash
# Check current leader
kubectl get lease dynamic-toolset-leader -n kubernaut-system -o yaml

# Check replica status
kubectl get pods -n kubernaut-system -l app=dynamic-toolset
```

**Recovery**: Automatic via leader election. No manual intervention needed.

---

## Deployment Considerations

### Single Cluster Deployment

**Recommended Configuration**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dynamic-toolset
  namespace: kubernaut-system
spec:
  replicas: 1  # Single replica for simple deployments
  selector:
    matchLabels:
      app: dynamic-toolset
  template:
    spec:
      serviceAccountName: dynamic-toolset-sa
      containers:
      - name: dynamic-toolset
        image: kubernaut/dynamic-toolset:v1.0
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "100m"
        env:
        - name: DISCOVERY_INTERVAL
          value: "5m"
        - name: RECONCILIATION_INTERVAL
          value: "30s"
```

**Characteristics**:
- ✅ Simple configuration
- ✅ Low resource usage
- ✅ No leader election overhead
- ⚠️ No high availability
- ⚠️ Restart causes brief discovery gap

**Use Cases**:
- Development environments
- Small clusters (< 50 services)
- Non-critical toolset discovery

---

### High Availability Deployment

**Recommended Configuration**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dynamic-toolset
  namespace: kubernaut-system
spec:
  replicas: 2  # Multi-replica for HA
  selector:
    matchLabels:
      app: dynamic-toolset
  template:
    spec:
      serviceAccountName: dynamic-toolset-sa
      affinity:
        podAntiAffinity:  # Spread across nodes
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: dynamic-toolset
            topologyKey: kubernetes.io/hostname
      containers:
      - name: dynamic-toolset
        image: kubernaut/dynamic-toolset:v1.0
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "100m"
        env:
        - name: LEADER_ELECTION_ENABLED
          value: "true"
        - name: LEASE_DURATION
          value: "15s"
        - name: RENEW_DEADLINE
          value: "10s"
        - name: RETRY_PERIOD
          value: "2s"
```

**Additional Resources**:
```yaml
# Leader election lease
apiVersion: coordination.k8s.io/v1
kind: Lease
metadata:
  name: dynamic-toolset-leader
  namespace: kubernaut-system
```

**Characteristics**:
- ✅ High availability (automatic failover)
- ✅ Zero downtime during pod restarts
- ✅ Leader election prevents conflicts
- ⚠️ Slightly higher resource usage
- ⚠️ More complex troubleshooting

**Failover Time**: < 15 seconds (lease duration)

**Use Cases**:
- Production environments
- Large clusters (> 50 services)
- Critical toolset discovery

---

### Multi-Cluster Deployment

**Architecture**: One Dynamic Toolset Service per cluster

```
┌──────────────────────────────────┐
│       Cluster A (Prod)           │
│  ┌────────────────────────────┐  │
│  │ Dynamic Toolset Service    │  │
│  │ discovers: Prometheus A    │  │
│  │ generates: ConfigMap A     │  │
│  └────────────────────────────┘  │
│  ┌────────────────────────────┐  │
│  │ HolmesGPT API reads       │  │
│  │ ConfigMap A               │  │
│  └────────────────────────────┘  │
└──────────────────────────────────┘

┌──────────────────────────────────┐
│       Cluster B (Staging)        │
│  ┌────────────────────────────┐  │
│  │ Dynamic Toolset Service    │  │
│  │ discovers: Prometheus B    │  │
│  │ generates: ConfigMap B     │  │
│  └────────────────────────────┘  │
│  ┌────────────────────────────┐  │
│  │ HolmesGPT API reads       │  │
│  │ ConfigMap B               │  │
│  └────────────────────────────┘  │
└──────────────────────────────────┘
```

**Deployment Pattern**:
1. Deploy service in each cluster independently
2. Each service discovers its own cluster's services
3. Each ConfigMap is cluster-specific
4. HolmesGPT API in each cluster reads local ConfigMap

**Benefits**:
- ✅ Cluster isolation (failure in one doesn't affect others)
- ✅ No cross-cluster network dependencies
- ✅ Cluster-specific toolsets
- ✅ Simple RBAC (no cross-cluster permissions needed)

**Limitations**:
- No global service discovery
- Each cluster's HolmesGPT only knows about local services
- Manual aggregation required for cross-cluster investigations

**V2 Consideration**: Federated discovery with centralized ConfigMap aggregation

---

### Rolling Updates Strategy

**Zero-Downtime Update Process**:

```bash
# Step 1: Update deployment image
kubectl set image deployment/dynamic-toolset \
  -n kubernaut-system \
  dynamic-toolset=kubernaut/dynamic-toolset:v1.1

# Step 2: Monitor rollout
kubectl rollout status deployment/dynamic-toolset -n kubernaut-system

# Step 3: Verify new version
kubectl logs -n kubernaut-system -l app=dynamic-toolset --tail=20 | grep version
```

**Update Strategy Configuration**:
```yaml
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0  # Always keep at least 1 replica running
      maxSurge: 1        # Allow 1 extra pod during update
```

**Update Impact**:
- Discovery continues during update (leader election)
- ConfigMap reconciliation continues
- No API downtime
- Brief leader election during pod replacement (< 15s)

**Rollback**:
```bash
# Rollback to previous version
kubectl rollout undo deployment/dynamic-toolset -n kubernaut-system

# Verify rollback
kubectl rollout status deployment/dynamic-toolset -n kubernaut-system
```

---

### Resource Sizing Guidelines

**Small Cluster** (< 20 services):
```yaml
resources:
  requests:
    memory: "32Mi"
    cpu: "25m"
  limits:
    memory: "64Mi"
    cpu: "50m"
```

**Medium Cluster** (20-100 services):
```yaml
resources:
  requests:
    memory: "64Mi"
    cpu: "50m"
  limits:
    memory: "128Mi"
    cpu: "100m"
```

**Large Cluster** (> 100 services):
```yaml
resources:
  requests:
    memory: "128Mi"
    cpu: "100m"
  limits:
    memory: "256Mi"
    cpu: "200m"
```

**Scaling Factors**:
- Memory: ~1MB per discovered service (with caching)
- CPU: Spikes during discovery (every 5 min), otherwise idle
- Disk: Negligible (no persistent storage)

---

## Related Documentation

### Core Specifications
- [API Specification](./api-specification.md) - REST API endpoints
- [ConfigMap Schema](./configmap-schema.md) - Toolset configuration format
- [Service Detectors](./service-detectors.md) - Detection logic for each service type

### Architecture References
- [Dynamic Toolset Configuration Architecture](../../../../architecture/DYNAMIC_TOOLSET_CONFIGURATION_ARCHITECTURE.md) - Complete architecture
- [HolmesGPT API Overview](../holmesgpt-api/overview.md) - Consumer of toolsets
- [Service Dependency Map](../../../../architecture/SERVICE_DEPENDENCY_MAP.md)

---

**Document Status**: ✅ Complete - Enhanced with Operational Details
**Last Updated**: October 10, 2025
**Maintainer**: Kubernaut Architecture Team
**Version**: 1.0
