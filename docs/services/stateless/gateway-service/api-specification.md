# Gateway Service - API Specification

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service
**HTTP Port**: 8080
**Metrics Port**: 9090

---

## 📋 API Overview

**Base URL**: `http://gateway-service.prometheus-alerts-slm.svc.cluster.local:8080`

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
9. **Response** - ~1ms

**Total**: 20-50ms (p95)

---

## 📚 Related Documentation

- [Overview](./overview.md) - Service architecture and design decisions
- [Implementation](./implementation.md) - Package structure and code patterns
- [Deduplication](./deduplication.md) - Fingerprint-based deduplication strategy
- [Security Configuration](./security-configuration.md) - RBAC and authentication
- [Observability & Logging](./observability-logging.md) - Metrics and logging

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete Specification

