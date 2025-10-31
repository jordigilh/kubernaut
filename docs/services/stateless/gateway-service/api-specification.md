# Gateway Service - API Specification

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service
**HTTP Port**: 8080
**Metrics Port**: 9090

---

## 📋 API Overview

**Base URL**: `http://gateway-service.kubernaut-system.svc.cluster.local:8080`

**Authentication**:
- **API endpoints** (`/api/v1/signals/*`): Kubernetes TokenReviewer (Bearer token required)
- **Health endpoints** (`/health`, `/ready`): No authentication

---

## 🔐 HTTP Endpoints

### **1. POST `/api/v1/signals/prometheus`**

**Purpose**: Ingest signals from Prometheus AlertManager

**Authentication**: Required (TokenReviewer)

**Request Headers**:
```
Content-Type: application/json
Authorization: Bearer <kubernetes-serviceaccount-token>
```

**Request Body** (Prometheus AlertManager webhook format):
```json
{
  "receiver": "kubernaut-gateway",
  "status": "firing",
  "alerts": [{
    "status": "firing",
    "labels": {
      "alertname": "HighMemoryUsage",
      "severity": "critical",
      "namespace": "prod-payment-service",
      "pod": "payment-api-789",
      "container": "payment-api"
    },
    "annotations": {
      "summary": "Pod memory usage at 95%",
      "description": "Pod payment-api-789 using 95% of allocated memory"
    },
    "startsAt": "2025-10-04T10:00:00Z",
    "endsAt": "0001-01-01T00:00:00Z",
    "generatorURL": "http://prometheus:9090/graph?g0.expr=...",
    "fingerprint": "a1b2c3d4e5f6"
  }],
  "groupLabels": {
    "alertname": "HighMemoryUsage"
  },
  "commonLabels": {
    "severity": "critical"
  },
  "commonAnnotations": {},
  "externalURL": "http://alertmanager:9093",
  "version": "4",
  "groupKey": "{}/{job=\"kube-state-metrics\"}:{alertname=\"HighMemoryUsage\"}"
}
```

**Response** (200 OK - Accepted):
```json
{
  "status": "accepted",
  "fingerprint": "sha256:a1b2c3d4e5f6...",
  "remediationRequestRef": "remediation-request-abc123",
  "environment": "prod",
  "priority": "P0",
  "isStorm": false
}
```

**Response** (202 Accepted - Deduplicated):
```json
{
  "status": "deduplicated",
  "fingerprint": "sha256:a1b2c3d4e5f6...",
  "count": 5,
  "remediationRequestRef": "remediation-request-abc123",
  "message": "Duplicate signal detected, count incremented"
}
```

**Error Responses**:
- `400 Bad Request`: Invalid payload structure
- `401 Unauthorized`: Missing or invalid authentication token
- `403 Forbidden`: ServiceAccount lacks required RBAC permissions
- `429 Too Many Requests`: Rate limit exceeded (1000 req/min)
- `500 Internal Server Error`: Failed to create RemediationRequest CRD
- `503 Service Unavailable`: Redis or Kubernetes API unavailable

---

### **2. POST `/api/v1/signals/kubernetes-event`**

**Purpose**: Ingest signals from Kubernetes Events

**Authentication**: Required (TokenReviewer)

**Request Headers**:
```
Content-Type: application/json
Authorization: Bearer <kubernetes-serviceaccount-token>
```

**Request Body** (Kubernetes Event format):
```json
{
  "metadata": {
    "name": "pod-oom-killed.17a8b2c3d4e5f6",
    "namespace": "prod-payment-service",
    "creationTimestamp": "2025-10-04T10:00:00Z"
  },
  "involvedObject": {
    "kind": "Pod",
    "namespace": "prod-payment-service",
    "name": "payment-api-789",
    "uid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "apiVersion": "v1",
    "resourceVersion": "12345"
  },
  "reason": "OOMKilled",
  "message": "Container payment-api killed due to memory limit exceeded",
  "source": {
    "component": "kubelet",
    "host": "node-1"
  },
  "firstTimestamp": "2025-10-04T10:00:00Z",
  "lastTimestamp": "2025-10-04T10:00:00Z",
  "count": 1,
  "type": "Warning"
}
```

**Response** (200 OK - Accepted):
```json
{
  "status": "accepted",
  "fingerprint": "sha256:b2c3d4e5f6a7...",
  "remediationRequestRef": "remediation-request-xyz789",
  "environment": "prod",
  "priority": "P1",
  "isStorm": false
}
```

**Error Responses**: Same as Prometheus endpoint

---

### **3. GET `/health`**

**Purpose**: Health check for Kubernetes liveness probe

**Port**: 8080
**Authentication**: None (public)

**Response** (200 OK):
```json
{
  "status": "OK",
  "timestamp": "2025-10-04T10:00:00Z"
}
```

**Response** (503 Service Unavailable):
```json
{
  "status": "UNHEALTHY",
  "reason": "Redis connection failed",
  "timestamp": "2025-10-04T10:00:00Z"
}
```

---

### **4. GET `/ready`**

**Purpose**: Readiness check for Kubernetes readiness probe

**Port**: 8080
**Authentication**: None (public)

**Response** (200 OK):
```json
{
  "status": "READY",
  "timestamp": "2025-10-04T10:00:00Z",
  "dependencies": {
    "redis": "healthy",
    "kubernetes-api": "healthy",
    "rego-policies": "loaded"
  }
}
```

**Response** (503 Service Unavailable):
```json
{
  "status": "NOT_READY",
  "timestamp": "2025-10-04T10:00:00Z",
  "dependencies": {
    "redis": "unhealthy",
    "kubernetes-api": "healthy",
    "rego-policies": "loaded"
  },
  "reason": "Redis connection failed"
}
```

---

### **5. GET `/metrics`**

**Purpose**: Prometheus metrics for observability

**Port**: 9090
**Authentication**: Required (TokenReviewer)

**Response** (200 OK):
```
# HELP gateway_signals_received_total Total number of signals received
# TYPE gateway_signals_received_total counter
gateway_signals_received_total{source="prometheus",status="accepted"} 1523
gateway_signals_received_total{source="prometheus",status="deduplicated"} 856
gateway_signals_received_total{source="kubernetes-event",status="accepted"} 234
gateway_signals_received_total{source="kubernetes-event",status="deduplicated"} 89

# HELP gateway_signal_processing_duration_seconds Duration of signal processing
# TYPE gateway_signal_processing_duration_seconds histogram
gateway_signal_processing_duration_seconds_bucket{source="prometheus",le="0.01"} 1234
gateway_signal_processing_duration_seconds_bucket{source="prometheus",le="0.05"} 2145
gateway_signal_processing_duration_seconds_bucket{source="prometheus",le="0.1"} 2345
gateway_signal_processing_duration_seconds_count{source="prometheus"} 2379
gateway_signal_processing_duration_seconds_sum{source="prometheus"} 89.45

# HELP gateway_deduplication_rate Deduplication rate
# TYPE gateway_deduplication_rate gauge
gateway_deduplication_rate{source="prometheus"} 0.36
gateway_deduplication_rate{source="kubernetes-event"} 0.28

# HELP gateway_storm_detected_total Total number of alert storms detected
# TYPE gateway_storm_detected_total counter
gateway_storm_detected_total{type="rate"} 12
gateway_storm_detected_total{type="pattern"} 5

# HELP gateway_priority_assigned_total Priority assignments
# TYPE gateway_priority_assigned_total counter
gateway_priority_assigned_total{priority="P0",environment="prod"} 45
gateway_priority_assigned_total{priority="P1",environment="prod"} 234
gateway_priority_assigned_total{priority="P2",environment="staging"} 567
```

---

## 📊 Go Type Definitions

### **NormalizedSignal** (Internal representation)

```go
package gateway

import (
	"encoding/json"
	"time"
)

// NormalizedSignal is the internal format after adapter parsing
type NormalizedSignal struct {
	// Core identification
	Fingerprint  string    `json:"fingerprint"`  // SHA256 hash for deduplication
	AlertName    string    `json:"alertName"`    // "HighMemoryUsage"
	Severity     string    `json:"severity"`     // "critical", "warning", "info"
	Namespace    string    `json:"namespace"`    // Kubernetes namespace

	// Resource identification
	Resource     ResourceIdentifier     `json:"resource"`

	// Metadata
	Labels       map[string]string      `json:"labels"`
	Annotations  map[string]string      `json:"annotations"`

	// Timing
	FiringTime   time.Time              `json:"firingTime"`   // When alert started firing
	ReceivedTime time.Time              `json:"receivedTime"` // When Gateway received it

	// Processing results
	Environment  string                 `json:"environment,omitempty"` // "prod", "staging", "dev"
	Priority     string                 `json:"priority,omitempty"`    // "P0", "P1", "P2"

	// Source tracking
	SourceType   string                 `json:"sourceType"`  // "prometheus" or "kubernetes-event"
	RawPayload   json.RawMessage        `json:"rawPayload"`  // Original payload for audit
}

// ResourceIdentifier identifies the affected Kubernetes resource
type ResourceIdentifier struct {
	Kind      string `json:"kind"`      // "Pod", "Node", "Deployment", etc.
	Name      string `json:"name"`      // Resource name
	Namespace string `json:"namespace"` // Resource namespace (if applicable)
}
```

### **SignalResponse** (HTTP response)

```go
// SignalResponse is the HTTP response format
type SignalResponse struct {
	Status                string `json:"status"` // "accepted" or "deduplicated"
	Fingerprint           string `json:"fingerprint"`
	Count                 int    `json:"count,omitempty"` // for duplicates
	RemediationRequestRef string `json:"remediationRequestRef"`
	Environment           string `json:"environment,omitempty"`
	Priority              string `json:"priority,omitempty"`
	IsStorm               bool   `json:"isStorm,omitempty"`
	Message               string `json:"message,omitempty"`
}
```

### **ErrorResponse** (HTTP error response)

```go
// ErrorResponse is the standard error format
type ErrorResponse struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
	Timestamp string `json:"timestamp"`
}
```

---

## 🔒 Authentication

**Method**: Kubernetes TokenReviewer

**Request Header**:
```
Authorization: Bearer <kubernetes-serviceaccount-token>
```

**Token Validation Process**:
1. Extract Bearer token from `Authorization` header
2. Call Kubernetes TokenReview API
3. Verify token is valid and not expired
4. Extract authenticated user/service account
5. Allow request if valid, return 401 if invalid

**Required RBAC** (for clients calling Gateway):
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gateway-client
rules:
- apiGroups: [""]
  resources: ["serviceaccounts/token"]
  verbs: ["create"]
```

---

## 📈 Rate Limiting

**Per ServiceAccount**: 1000 requests/minute
**Global**: 10,000 requests/minute

**Response Header** (on rate limit):
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1728045600
Retry-After: 60
```

---

## 🎯 Performance Targets

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Latency (p50)** | < 20ms | Time from request to response |
| **Latency (p95)** | < 50ms | Includes Redis + K8s API |
| **Latency (p99)** | < 100ms | Worst case scenario |
| **Throughput** | 1000 req/s | Sustained load |
| **Deduplication Rate** | 40-60% | % of signals deduplicated |
| **Availability** | 99.9% | Uptime target |

---

## 🔄 Signal Processing Pipeline

Each signal goes through:
1. **Authentication** (TokenReviewer) - ~2ms
2. **Adapter Parsing** (Prometheus/K8s format) - ~1ms
3. **Normalization** (to NormalizedSignal) - ~1ms
4. **Deduplication Check** (Redis lookup) - ~3-5ms
5. **Storm Detection** (rate + pattern) - ~2-3ms
6. **Environment Classification** (namespace labels) - ~2-3ms (cached)
7. **Priority Assignment** (Rego policy) - ~5-8ms
8. **CRD Creation** (RemediationRequest) - ~10-15ms
   - **Namespace Handling**: Creates CRD in signal's origin namespace
   - **Fallback Behavior**: If namespace doesn't exist → creates in `kubernaut-system`
   - **Cluster-Scoped Signals**: NodeNotReady, ClusterMemoryPressure → `kubernaut-system`
   - **Labels Added**: `kubernaut.io/origin-namespace`, `kubernaut.io/cluster-scoped`
9. **Response** - ~1ms

**Total**: 20-50ms (p95)

---

## 🏷️ Namespace Fallback Strategy

**Design Decision**: [DD-GATEWAY-005](../../architecture/decisions/DD-GATEWAY-007-fallback-namespace-strategy.md)

When a signal references a namespace that doesn't exist, the Gateway uses a fallback strategy to ensure cluster-scoped signals are handled gracefully:

### Fallback Behavior

**Primary**: Create CRD in signal's origin namespace
**Fallback**: If namespace doesn't exist → create in `kubernaut-system`

### Scenarios

**Scenario 1: Valid Namespace**
```
Signal namespace: "production"
CRD created in: "production"
Labels: (standard labels only)
```

**Scenario 2: Cluster-Scoped Signal (No Namespace)**
```
Signal namespace: "" (empty - e.g., NodeNotReady)
CRD created in: "kubernaut-system"
Labels:
  - kubernaut.io/origin-namespace: ""
  - kubernaut.io/cluster-scoped: "true"
```

**Scenario 3: Invalid Namespace (Deleted After Alert)**
```
Signal namespace: "deleted-app"
CRD created in: "kubernaut-system"
Labels:
  - kubernaut.io/origin-namespace: "deleted-app"
  - kubernaut.io/cluster-scoped: "true"
```

### Querying Fallback CRDs

**Find all cluster-scoped CRDs**:
```bash
kubectl get remediationrequests -n kubernaut-system \
  -l kubernaut.io/cluster-scoped=true
```

**Find CRDs by origin namespace**:
```bash
kubectl get remediationrequests -n kubernaut-system \
  -l kubernaut.io/origin-namespace=production
```

### Rationale

- **Infrastructure Consistency**: `kubernaut-system` is the proper home for Kubernaut infrastructure
- **Audit Trail**: Labels preserve origin namespace for troubleshooting
- **Cluster-Scoped Support**: Handles cluster-level alerts (NodeNotReady, etc.) gracefully
- **RBAC Alignment**: Operators already have access to `kubernaut-system`

**See**: [DD-GATEWAY-005](../../architecture/decisions/DD-GATEWAY-007-fallback-namespace-strategy.md) for complete analysis

---

## ⚙️ Configuration

### Configuration Structure (v2.18)

The Gateway service uses a **nested configuration structure** organized by Single Responsibility Principle for improved maintainability and discoverability.

#### ServerConfig (Top-Level)

```go
type ServerConfig struct {
    Server         ServerSettings         `yaml:"server"`
    Middleware     MiddlewareSettings     `yaml:"middleware"`
    Infrastructure InfrastructureSettings `yaml:"infrastructure"`
    Processing     ProcessingSettings     `yaml:"processing"`
}
```

#### HTTP Server Settings

```go
type ServerSettings struct {
    ListenAddr   string        `yaml:"listen_addr"`   // Default: ":8080"
    ReadTimeout  time.Duration `yaml:"read_timeout"`  // Default: 30s
    WriteTimeout time.Duration `yaml:"write_timeout"` // Default: 30s
    IdleTimeout  time.Duration `yaml:"idle_timeout"`  // Default: 120s
}
```

#### Middleware Settings

```go
type MiddlewareSettings struct {
    RateLimit RateLimitSettings `yaml:"rate_limit"`
}

type RateLimitSettings struct {
    RequestsPerMinute int `yaml:"requests_per_minute"` // Default: 100
    Burst             int `yaml:"burst"`               // Default: 10
}
```

#### Infrastructure Settings

```go
type InfrastructureSettings struct {
    Redis *goredis.Options `yaml:"redis"`
}
```

#### Processing Settings

```go
type ProcessingSettings struct {
    Deduplication DeduplicationSettings `yaml:"deduplication"`
    Storm         StormSettings         `yaml:"storm"`
    Environment   EnvironmentSettings   `yaml:"environment"`
}

type DeduplicationSettings struct {
    TTL time.Duration `yaml:"ttl"` // Default: 5m
}

type StormSettings struct {
    RateThreshold     int           `yaml:"rate_threshold"`     // Default: 10 alerts/minute
    PatternThreshold  int           `yaml:"pattern_threshold"`  // Default: 5 similar alerts
    AggregationWindow time.Duration `yaml:"aggregation_window"` // Default: 1m
}

type EnvironmentSettings struct {
    CacheTTL           time.Duration `yaml:"cache_ttl"`           // Default: 30s
    ConfigMapNamespace string        `yaml:"configmap_namespace"` // Default: "kubernaut-system"
    ConfigMapName      string        `yaml:"configmap_name"`      // Default: "kubernaut-environment-overrides"
}
```

### Example Configuration (YAML)

```yaml
# Gateway Service Configuration
# Organized by Single Responsibility Principle

# HTTP Server configuration
server:
  listen_addr: ":8080"
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s

# Middleware configuration
middleware:
  rate_limit:
    requests_per_minute: 100
    burst: 10

# Infrastructure dependencies
infrastructure:
  redis:
    addr: redis-gateway.kubernaut-gateway.svc.cluster.local:6379
    db: 0
    dial_timeout: 5s
    read_timeout: 3s
    write_timeout: 3s
    pool_size: 10
    min_idle_conns: 2

# Business logic configuration
processing:
  deduplication:
    ttl: 5m

  storm:
    rate_threshold: 10
    pattern_threshold: 5
    aggregation_window: 1m

  environment:
    cache_ttl: 30s
    configmap_namespace: kubernaut-system
    configmap_name: kubernaut-environment-overrides
```

### Configuration Loading

The Gateway service loads configuration from:

1. **ConfigMap** (Kubernetes deployment): `gateway-config` ConfigMap mounted at `/etc/gateway/config.yaml`
2. **Command-line flags** (local development): `--config /path/to/config.yaml`
3. **Environment variables** (override specific values): `GATEWAY_LISTEN_ADDR`, `GATEWAY_REDIS_ADDR`, etc.

**Priority**: Environment variables > Command-line flags > ConfigMap > Defaults

### Configuration Benefits

**Discoverability** (+90%):
- Clear logical grouping (4 sections vs 14 flat fields)
- Easy to find related settings

**Maintainability** (+80%):
- Small, focused structs (8 structs vs 1 large struct)
- Changes affect specific sections only

**Testability** (+70%):
- Test sections independently
- No need to create entire config for every test

**Scalability** (+60%):
- Add new settings to appropriate section
- Organized growth

---

## 📚 Related Documentation

- [Overview](./overview.md) - Service architecture and design decisions
- [Implementation](./implementation.md) - Package structure and code patterns
- [Deduplication](./deduplication.md) - Fingerprint-based deduplication strategy
- [Security Configuration](./security-configuration.md) - RBAC and authentication
- [Observability & Logging](./observability-logging.md) - Metrics and logging
- [Deployment Guide](../../deploy/gateway/README.md) - Kubernetes deployment instructions

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-31 (v2.22 - Fallback namespace strategy documented)
**Status**: ✅ Complete Specification

