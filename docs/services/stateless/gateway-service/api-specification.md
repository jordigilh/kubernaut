# Gateway Service - API Specification

**Version**: v2.25
**Last Updated**: November 7, 2025
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
  "isStorm": false
}
```

> **Note (2025-12-06)**: `environment` and `priority` fields removed from response.
> Classification is now owned by Signal Processing service per DD-CATEGORIZATION-001.

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
    "kubernetes-api": "healthy"
  }
}
```

> **Note (2025-12-06)**: `rego-policies` dependency removed.
> Priority assignment (Rego) moved to Signal Processing service per DD-CATEGORIZATION-001.

**Response** (503 Service Unavailable):
```json
{
  "status": "NOT_READY",
  "timestamp": "2025-10-04T10:00:00Z",
  "dependencies": {
    "redis": "unhealthy",
    "kubernetes-api": "healthy"
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

# Note: gateway_priority_assigned_total metric removed (2025-12-06)
# Priority assignment moved to Signal Processing service per DD-CATEGORIZATION-001
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

	// Note (2025-12-06): Environment and Priority fields removed
	// Classification is now owned by Signal Processing service per DD-CATEGORIZATION-001

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
// Note (2025-12-06): Environment and Priority fields removed
// Classification is now owned by Signal Processing service per DD-CATEGORIZATION-001
type SignalResponse struct {
	Status                string `json:"status"` // "accepted" or "deduplicated"
	Fingerprint           string `json:"fingerprint"`
	Count                 int    `json:"count,omitempty"` // for duplicates
	RemediationRequestRef string `json:"remediationRequestRef"`
	IsStorm               bool   `json:"isStorm,omitempty"`
	Message               string `json:"message,omitempty"`
}
```

### **ErrorResponse** (HTTP error response - RFC 7807)

```go
// RFC7807Error represents an RFC 7807 Problem Details error response
// BR-GATEWAY-101: RFC 7807 compliant error format
type RFC7807Error struct {
	Type      string `json:"type"`                 // URI reference identifying the problem type
	Title     string `json:"title"`                // Short, human-readable summary
	Detail    string `json:"detail"`               // Human-readable explanation
	Status    int    `json:"status"`               // HTTP status code
	Instance  string `json:"instance,omitempty"`   // URI reference identifying specific occurrence
	RequestID string `json:"requestId,omitempty"`  // Request ID for correlation
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
6. **CRD Creation** (RemediationRequest) - ~10-15ms
   - **Namespace Handling**: Creates CRD in signal's origin namespace
   - **Fallback Behavior**: If namespace doesn't exist → creates in `kubernaut-system`
   - **Cluster-Scoped Signals**: NodeNotReady, ClusterMemoryPressure → `kubernaut-system`
   - **Labels Added**: `kubernaut.ai/origin-namespace`, `kubernaut.ai/cluster-scoped`
7. **Response** - ~1ms

> **Note (2025-12-06)**: Steps 6-7 (Environment Classification, Priority Assignment) removed.
> Classification is now owned by Signal Processing service per DD-CATEGORIZATION-001.

**Total**: 15-30ms (p95)

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
  - kubernaut.ai/origin-namespace: ""
  - kubernaut.ai/cluster-scoped: "true"
```

**Scenario 3: Invalid Namespace (Deleted After Alert)**
```
Signal namespace: "deleted-app"
CRD created in: "storm-ttl-1762470354788403000"  # Auto-detected Gateway namespace
Labels:
  - kubernaut.ai/origin-namespace: "deleted-app"
  - kubernaut.ai/cluster-scoped: "true"
```

### CRD Fallback Namespace Strategy (v2.25 - Configurable)

**NEW in v2.25**: Fallback namespace is now **configurable** with **auto-detection**.

#### Auto-Detection (Default Behavior)

The Gateway automatically detects its running namespace from:
```
/var/run/secrets/kubernetes.io/serviceaccount/namespace
```

**Benefits**:
- ✅ **Zero configuration**: Works out-of-box in any namespace
- ✅ **Test-friendly**: E2E tests automatically use test namespace
- ✅ **Production-ready**: Gateway in `kubernaut-system` uses `kubernaut-system`
- ✅ **Namespace isolation**: Each Gateway deployment is self-contained
- ✅ **Kubernetes-native**: Follows standard K8s controller patterns

#### Configuration Override

For multi-tenant or special scenarios, explicitly configure fallback namespace:

```yaml
processing:
  crd:
    fallback_namespace: "my-custom-namespace"  # Override auto-detection
```

#### Fallback Behavior

When target namespace doesn't exist (e.g., deleted namespace or cluster-scoped signals):

1. **Attempt**: Create CRD in target namespace
2. **Failure**: Namespace not found
3. **Fallback**: Create CRD in configured fallback namespace (default: auto-detected pod namespace)
4. **Labels**: Add origin tracking labels

**Example (Production)**:
```yaml
Gateway running in: "kubernaut-system"
Signal namespace: "deleted-app"
CRD created in: "kubernaut-system"  # Auto-detected
Labels:
  - kubernaut.ai/origin-namespace: "deleted-app"
  - kubernaut.ai/cluster-scoped: "true"
```

**Example (E2E Test)**:
```yaml
Gateway running in: "storm-ttl-1762470354788403000"
Signal namespace: "production"
CRD created in: "storm-ttl-1762470354788403000"  # Auto-detected test namespace
Labels:
  - kubernaut.ai/origin-namespace: "production"
  - kubernaut.ai/cluster-scoped: "true"
```

### Querying Fallback CRDs

**Find all cluster-scoped CRDs** (in auto-detected namespace):
```bash
# Production (Gateway in kubernaut-system)
kubectl get remediationrequests -n kubernaut-system \
  -l kubernaut.ai/cluster-scoped=true

# E2E Test (Gateway in test namespace)
kubectl get remediationrequests -n storm-ttl-1762470354788403000 \
  -l kubernaut.ai/cluster-scoped=true
```

**Find CRDs by origin namespace**:
```bash
kubectl get remediationrequests -A \
  -l kubernaut.ai/origin-namespace=production
```

### Rationale

- **Flexibility**: Works in any deployment scenario (production, staging, E2E tests)
- **Kubernetes-Native**: Follows standard K8s controller patterns for namespace detection
- **Audit Trail**: Labels preserve origin namespace for troubleshooting
- **RBAC Alignment**: CRDs created in Gateway's namespace (where RBAC is configured)
- **Cluster-Scoped Support**: Handles cluster-level signals (NodeNotReady, etc.) gracefully

**See**: Configuration section below for complete `processing.crd` settings

---

## 🔄 K8s API Retry Strategy (v2.25)

**NEW in v2.25**: Gateway implements retry logic with exponential backoff to handle transient Kubernetes API errors gracefully.

### Overview

The Gateway automatically retries CRD creation when encountering transient Kubernetes API errors. This prevents alert loss during:
- **API Rate Limiting** (HTTP 429): K8s API throttling during alert storms
- **Service Unavailability** (HTTP 503): Temporary API server unavailability
- **Network Timeouts**: Network latency or API server overload

**Business Impact**: Prevents 5-10% alert loss during production incidents (estimated).

---

### Retryable Errors

The Gateway automatically retries these transient error types:

| Error Type | HTTP Status | Retry Behavior | Business Impact |
|------------|-------------|----------------|-----------------|
| **Rate Limiting** | 429 Too Many Requests | Retry with exponential backoff (1s → 2s → 4s) | Prevents alert loss during alert storms (>100 alerts/min) |
| **Service Unavailable** | 503 Service Unavailable | Retry with exponential backoff | Handles temporary API server unavailability (upgrades, restarts) |
| **Timeout** | N/A (timeout error) | Retry with exponential backoff | Handles network latency or API server overload |
| **Gateway Timeout** | 504 Gateway Timeout | Retry with exponential backoff | Handles API server proxy timeouts |
| **Connection Refused** | N/A (network error) | Retry with exponential backoff | Handles temporary network failures |

**Default Configuration**: All retryable errors are enabled by default.

---

### Non-Retryable Errors

The Gateway **does NOT retry** these permanent error types (immediate failure):

| Error Type | HTTP Status | Behavior | Rationale |
|------------|-------------|----------|-----------|
| **Validation Error** | 400 Bad Request | Fail immediately | Invalid CRD schema, cannot be fixed by retry |
| **RBAC Error** | 403 Forbidden | Fail immediately | Insufficient permissions, requires RBAC configuration fix |
| **Schema Validation** | 422 Unprocessable Entity | Fail immediately | CRD schema mismatch, requires code fix |
| **Already Exists** | 409 Conflict | Fail immediately | CRD already exists (idempotent operation) |
| **Not Found** | 404 Not Found | Fail immediately (with fallback) | Namespace not found, uses fallback namespace |

**Rationale**: These errors indicate permanent configuration or code issues that cannot be resolved by retrying.

---

### Exponential Backoff Strategy

Gateway uses exponential backoff to prevent retry storms and thundering herd problems:

```
Attempt 1: Fail → Wait 1s  → Retry
Attempt 2: Fail → Wait 2s  → Retry
Attempt 3: Fail → Wait 4s  → Retry
Attempt 4: Fail → Wait 8s  → Retry (capped at max_backoff)
Attempt 5: Fail → Wait 10s → Retry (capped at max_backoff)
```

**Backoff Parameters**:
- **Initial Backoff**: 1s (configurable via `initial_backoff`)
- **Max Backoff**: 10s (configurable via `max_backoff`)
- **Max Attempts**: 3 (configurable via `max_attempts`)

**Example Timeline** (3 retries, default config):
```
T+0s:   Initial attempt fails (HTTP 429)
T+1s:   Retry attempt 1 fails (HTTP 429)
T+3s:   Retry attempt 2 fails (HTTP 429)
T+7s:   Retry attempt 3 succeeds (HTTP 201)
Total:  7 seconds from initial failure to success
```

---

### Configuration

#### Basic Configuration (Phase 1: Synchronous Retry)

```yaml
processing:
  retry:
    # Maximum number of retry attempts for transient K8s API errors
    # Default: 3 (limits webhook timeout risk to ~7s)
    max_attempts: 3

    # Initial backoff duration (doubles with each retry)
    # Example: 1s → 2s → 4s → 8s (exponential backoff)
    # Default: 1s
    initial_backoff: 1s

    # Maximum backoff duration (cap for exponential backoff)
    # Prevents excessive wait times during retry storms
    # Default: 5s
    max_backoff: 5s
```

> **Reliability-First Design**: Retries are **always enabled** for transient errors
> (429 rate limiting, 503 service unavailable, 504 gateway timeout, timeouts, network errors).
> Configuration only controls retry timing, not whether to retry. This ensures maximum
> reliability without configuration complexity.

#### Advanced Configuration (Phase 2: Async Retry Queue - Future)

> **Note**: Phase 2 async retry queue will be **automatically enabled** based on load,
> not manual configuration. No user configuration required for queue management.

**Auto-Scaling Behavior** (Phase 2 - Future):
- **Queue size**: Auto-scales based on memory availability
- **Worker count**: Auto-scales based on CPU cores (`runtime.NumCPU()`)
- **Redis persistence**: Always enabled for reliability

**User Tuning** (Phase 2, if needed):
```yaml
processing:
  retry:
    max_attempts: 10         # Higher limit for async retry (no webhook timeout risk)
    initial_backoff: 100ms   # Fast initial retry
    max_backoff: 60s         # Longer max backoff for production resilience
```

**Phase 2 Benefits** (Auto-Enabled):
- ✅ **Non-blocking**: Returns HTTP 202 immediately, retries in background
- ✅ **Unlimited retries**: Configurable max_attempts (up to 10+)
- ✅ **Survives restarts**: Retry items persisted to Redis
- ✅ **Scales to high load**: Worker pool processes retries concurrently

**Phase 2 Status**: Optional enhancement, implement after Phase 1 validated in production.

---

### HTTP Response Behavior

#### Synchronous Retry (Phase 1)

**Success after retry**:
```http
HTTP/1.1 201 Created
Content-Type: application/json

{
  "status": "accepted",
  "fingerprint": "sha256:a1b2c3d4e5f6...",
  "remediationRequestRef": "remediation-request-abc123",
  "environment": "prod",
  "priority": "P0",
  "retryAttempts": 2  # NEW: Number of retry attempts before success
}
```

**Retry exhausted** (all attempts failed):
```http
HTTP/1.1 500 Internal Server Error
Content-Type: application/json

{
  "error": "Failed to create RemediationRequest CRD",
  "details": "failed after 3 retries: Too Many Requests (rate limiting)",
  "fingerprint": "sha256:a1b2c3d4e5f6...",
  "retryAttempts": 3
}
```

#### Async Retry (Phase 2 - Optional)

**Enqueued for async retry**:
```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "status": "enqueued",
  "fingerprint": "sha256:a1b2c3d4e5f6...",
  "message": "CRD creation enqueued for async retry",
  "queuePosition": 5,
  "estimatedRetryTime": "2025-11-07T10:00:01Z"
}
```

---

### Metrics

Gateway exposes Prometheus metrics for retry behavior:

#### Phase 1 Metrics (Synchronous Retry)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_crd_retry_attempts_total` | Counter | `attempt` | Total retry attempts by attempt number (attempt_1, attempt_2, attempt_3) |
| `gateway_crd_retry_success_total` | Counter | `attempt` | Successful retries by attempt number |
| `gateway_crd_retry_exhausted_total` | Counter | N/A | Retries exhausted (max attempts reached, alert loss) |

#### Phase 2 Metrics (Async Retry Queue - Optional)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_retry_queue_depth` | Gauge | N/A | Current number of items in retry queue |
| `gateway_retry_queue_dropped_total` | Counter | N/A | Items dropped due to queue overflow |
| `gateway_retry_latency_seconds` | Histogram | `attempt` | Time from enqueue to successful retry |
| `gateway_retry_worker_utilization` | Gauge | `worker_id` | Worker utilization (0.0 = idle, 1.0 = busy) |

---

### Example Prometheus Queries

#### Retry Success Rate
```promql
# Percentage of retries that eventually succeed
rate(gateway_crd_retry_success_total[5m]) / rate(gateway_crd_retry_attempts_total[5m]) * 100
```

#### Alert Loss Rate (Retry Exhaustion)
```promql
# Percentage of alerts lost due to retry exhaustion
rate(gateway_crd_retry_exhausted_total[5m]) / rate(gateway_alerts_received_total[5m]) * 100
```

#### Average Retry Attempts Before Success
```promql
# Average number of retry attempts before success
avg(gateway_crd_retry_success_total) by (attempt)
```

#### Retry Queue Depth (Phase 2)
```promql
# Current retry queue depth (alerts pending retry)
gateway_retry_queue_depth
```

#### Retry Queue Overflow Rate (Phase 2)
```promql
# Rate of retry items dropped due to queue overflow
rate(gateway_retry_queue_dropped_total[5m])
```

---

### Alerting Recommendations

#### Critical Alerts

**High Retry Exhaustion Rate** (Alert Loss):
```yaml
alert: GatewayHighRetryExhaustion
expr: rate(gateway_crd_retry_exhausted_total[5m]) > 0.05
for: 5m
severity: critical
annotations:
  summary: "Gateway losing >5% of alerts due to retry exhaustion"
  description: "K8s API rate limiting or unavailability causing alert loss"
  runbook: "Check K8s API health, increase max_attempts, enable async retry queue"
```

**Retry Queue Overflow** (Phase 2):
```yaml
alert: GatewayRetryQueueOverflow
expr: rate(gateway_retry_queue_dropped_total[5m]) > 0
for: 2m
severity: critical
annotations:
  summary: "Gateway retry queue overflowing, dropping retry items"
  description: "Retry queue size exceeded, increase queue_size or worker_count"
  runbook: "Scale up Gateway replicas, increase queue_size, add more workers"
```

#### Warning Alerts

**High Retry Rate**:
```yaml
alert: GatewayHighRetryRate
expr: rate(gateway_crd_retry_attempts_total[5m]) / rate(gateway_alerts_received_total[5m]) > 0.10
for: 10m
severity: warning
annotations:
  summary: "Gateway retrying >10% of CRD creations"
  description: "K8s API experiencing transient errors (429, 503, timeout)"
  runbook: "Monitor K8s API health, check for rate limiting or overload"
```

---

### Performance Impact

#### Latency Impact

| Scenario | p50 Latency | p95 Latency | p99 Latency |
|----------|-------------|-------------|-------------|
| **No Retry** (success on first attempt) | 50ms | 100ms | 150ms |
| **1 Retry** (success on 2nd attempt) | 1.05s | 1.15s | 1.25s |
| **2 Retries** (success on 3rd attempt) | 3.05s | 3.15s | 3.25s |
| **3 Retries** (exhausted, failure) | 7.05s | 7.15s | 7.25s |

**Webhook Timeout Risk**: AlertManager webhook timeout is typically 30s. With max 3 retries, worst-case latency is ~7s (well within timeout).

#### Throughput Impact

| Scenario | Throughput (req/s) | Notes |
|----------|-------------------|-------|
| **No Retry** | 1000 req/s | Baseline (no K8s API errors) |
| **10% Retry Rate** | 950 req/s | Slight decrease due to retry overhead |
| **50% Retry Rate** | 700 req/s | Significant decrease during K8s API issues |
| **Async Retry** (Phase 2) | 1000 req/s | No throughput impact (non-blocking) |

**Recommendation**: Enable async retry queue (Phase 2) for production deployments with >500 alerts/min.

---

### Operational Considerations

#### When to Enable Async Retry (Phase 2)

Enable async retry queue if:
- ✅ Alert rate >500 alerts/min
- ✅ K8s API rate limiting occurs frequently (>1% of requests)
- ✅ AlertManager webhook timeouts observed
- ✅ Gateway restart recovery required (retain retry items across restarts)

#### When to Increase max_attempts

Increase `max_attempts` from default 3 to 5-10 if:
- ✅ K8s API rate limiting is persistent (>5 minutes)
- ✅ Retry exhaustion rate >1%
- ✅ Async retry queue enabled (no webhook timeout risk)

#### When to Adjust Backoff Durations

Increase `initial_backoff` or `max_backoff` if:
- ✅ K8s API rate limiting is severe (429 errors persist >10s)
- ✅ Retry storms observed (multiple Gateways retrying simultaneously)

Decrease `initial_backoff` if:
- ✅ K8s API errors are transient (<1s duration)
- ✅ Faster recovery time required

---

### Troubleshooting

#### High Retry Exhaustion Rate

**Symptoms**: `gateway_crd_retry_exhausted_total` increasing, alerts lost

**Diagnosis**:
```bash
# Check K8s API health
kubectl get --raw /healthz

# Check K8s API rate limiting
kubectl get --raw /metrics | grep apiserver_request_total

# Check Gateway retry metrics
curl http://gateway:9090/metrics | grep gateway_crd_retry
```

**Solutions**:
1. **Increase max_attempts**: `max_attempts: 5` (from default 3)
2. **Scale K8s API**: Add more API server replicas
3. **Reduce alert rate**: Tune Prometheus alerting rules
4. **Phase 2**: Async retry queue will automatically handle high retry load (auto-enabled)

#### Webhook Timeouts

**Symptoms**: AlertManager logs show webhook timeout errors

**Diagnosis**:
```bash
# Check AlertManager logs
kubectl logs -n monitoring deployment/alertmanager | grep timeout

# Check Gateway retry latency
curl http://gateway:9090/metrics | grep gateway_crd_retry_attempts
```

**Solutions**:
1. **Reduce max_attempts**: `max_attempts: 2` (reduce worst-case latency to ~3s)
2. **Increase AlertManager timeout**: `timeout: 60s` in AlertManager config
3. **Phase 2**: Async retry queue will be non-blocking (auto-enabled based on load)

#### Retry Queue Overflow (Phase 2 - Future)

**Symptoms**: `gateway_retry_queue_dropped_total` increasing

**Diagnosis**:
```bash
# Check retry queue depth
curl http://gateway:9090/metrics | grep gateway_retry_queue_depth

# Check worker utilization
curl http://gateway:9090/metrics | grep gateway_retry_worker_utilization
```

**Solutions**:
> **Note**: Phase 2 async retry queue will **auto-scale** based on system resources.
> Manual queue tuning is not required.

1. **Scale Gateway replicas**: Horizontal scaling (more Gateway pods)
2. **Reduce alert rate**: Tune Prometheus alerting rules
3. **Phase 2 Auto-Scaling**: Queue size and worker count automatically adjust based on CPU/memory

---

### Design Decision

**See**: [DD-GATEWAY-008](../../architecture/decisions/DD-GATEWAY-008-k8s-api-retry-strategy.md) for comprehensive analysis of retry strategy alternatives, performance implications, and rollback plan.

---

## ⚙️ Configuration

### Configuration Structure (v2.25)

The Gateway service uses a **nested configuration structure** organized by Single Responsibility Principle for improved maintainability and discoverability.

**NEW in v2.25**: Added `CRDSettings` for configurable fallback namespace.

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
// Note (2025-12-06): Environment and Priority settings removed
// Classification is now owned by Signal Processing service per DD-CATEGORIZATION-001
type ProcessingSettings struct {
    Deduplication DeduplicationSettings `yaml:"deduplication"`
    Storm         StormSettings         `yaml:"storm"`
    CRD           CRDSettings           `yaml:"crd"`
    Retry         RetrySettings         `yaml:"retry"`  // NEW in v2.25
}

type DeduplicationSettings struct {
    TTL time.Duration `yaml:"ttl"` // Default: 5m
}

type StormSettings struct {
    RateThreshold     int           `yaml:"rate_threshold"`     // Default: 10 alerts/minute
    PatternThreshold  int           `yaml:"pattern_threshold"`  // Default: 5 similar alerts
    AggregationWindow time.Duration `yaml:"aggregation_window"` // Default: 1m
}

type CRDSettings struct {
    // Fallback namespace for CRD creation when target namespace doesn't exist
    // This handles cluster-scoped signals (e.g., NodeNotReady) that don't have a namespace
    // Default: auto-detect from pod's namespace (/var/run/secrets/kubernetes.io/serviceaccount/namespace)
    // Override: set explicitly for multi-tenant or special scenarios
    // If auto-detect fails (non-K8s environment), falls back to "kubernaut-system"
    FallbackNamespace string `yaml:"fallback_namespace"` // Default: auto-detect pod namespace
}

// NEW in v2.25: K8s API Retry Settings
type RetrySettings struct {
    // Maximum number of retry attempts for transient K8s API errors
    // Default: 3 (Phase 1), 10 (Phase 2 with async queue)
    MaxAttempts int `yaml:"max_attempts"`

    // Initial backoff duration (doubles with each retry)
    // Example: 1s → 2s → 4s → 8s (exponential backoff)
    // Default: 1s
    InitialBackoff time.Duration `yaml:"initial_backoff"`

    // Maximum backoff duration (cap for exponential backoff)
    // Prevents excessive wait times during retry storms
    // Default: 5s
    MaxBackoff time.Duration `yaml:"max_backoff"`
}

// Reliability-First Design: Retries are always enabled for transient errors
// (429, 503, 504, timeouts, network errors). Configuration only controls retry
// timing, not whether to retry. This ensures maximum reliability without
// configuration complexity.
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

  # Note (2025-12-06): environment and priority settings removed
  # Classification is now owned by Signal Processing service per DD-CATEGORIZATION-001

  # NEW in v2.25: Configurable CRD fallback namespace
  crd:
    # Optional: Override auto-detected fallback namespace
    # Default: auto-detect from pod's namespace
    # Uncomment to set explicitly:
    # fallback_namespace: "my-custom-namespace"

  # NEW in v2.25: K8s API Retry Configuration
  retry:
    # Phase 1: Synchronous Retry (default)
    max_attempts: 3          # Maximum retry attempts (default: 3)
    initial_backoff: 100ms   # Initial backoff duration (default: 100ms)
    max_backoff: 5s          # Maximum backoff duration (default: 5s)

    # Note: Retries are ALWAYS enabled for transient errors (429, 503, 504, timeouts, network errors)
    # Configuration only controls retry timing, not whether to retry (reliability-first design)
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
**Last Updated**: 2025-11-06 (v2.25 - Configurable fallback namespace with auto-detection)
**Status**: ✅ Complete Specification

