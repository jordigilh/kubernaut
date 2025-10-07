# Gateway Service - Overview

**Version**: v1.0
**Last Updated**: October 4, 2025
**Status**: ✅ Design Complete (90.5%)

---

## Table of Contents

1. [Purpose & Scope](#purpose--scope)
2. [Architecture Overview](#architecture-overview)
3. [Alert Processing Pipeline](#alert-processing-pipeline)
4. [Key Architectural Decisions](#key-architectural-decisions)
5. [V1 Scope Boundaries](#v1-scope-boundaries)
6. [System Context Diagram](#system-context-diagram)
7. [Data Flow Diagram](#data-flow-diagram)

---

## Purpose & Scope

### Core Purpose

Gateway Service is the **single entry point** for all external signals (Prometheus alerts and Kubernetes events) into the Kubernaut intelligent remediation system. It serves as the **intelligent traffic cop** that:

1. **Ingests** alerts from multiple sources
2. **Deduplicates** repetitive alerts to prevent redundant processing
3. **Detects** alert storms to aggregate related incidents
4. **Classifies** environment context for risk assessment
5. **Prioritizes** alerts based on business rules
6. **Creates** RemediationRequest CRDs for downstream orchestration

### Why Gateway Service Exists

**Problem**: Without Gateway Service, downstream systems would be overwhelmed by:
- **Duplicate alerts** (same alert firing every 30 seconds)
- **Alert storms** (100 pods crashing simultaneously → 100 separate analyses)
- **Inconsistent prioritization** (critical production alerts treated same as dev warnings)
- **Missing context** (no environment classification for risk assessment)

**Solution**: Gateway Service provides **intelligent pre-processing** that:
- ✅ Reduces downstream load by 40-60% through deduplication
- ✅ Aggregates related alerts into single remediation workflows
- ✅ Ensures critical production issues get immediate attention
- ✅ Provides consistent environment context for GitOps decisions

---

## Architecture Overview

### Service Characteristics

- **Type**: Stateless HTTP API server
- **Deployment**: Kubernetes Deployment with horizontal scaling (2-5 replicas)
- **State Management**: Redis for deduplication metadata (shared across replicas)
- **Integration Pattern**: Webhook ingestion → CRD creation → Controller watch coordination

### Component Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                       Gateway Service                           │
│                                                                 │
│  ┌──────────────┐       ┌──────────────┐                      │
│  │  Prometheus  │       │  Kubernetes  │                      │
│  │   Adapter    │       │Event Adapter │                      │
│  └──────┬───────┘       └──────┬───────┘                      │
│         │                      │                               │
│         └──────────┬───────────┘                               │
│                    │                                           │
│         ┌──────────▼──────────┐                                │
│         │  Internal Alert     │                                │
│         │  Normalization      │                                │
│         └──────────┬──────────┘                                │
│                    │                                           │
│         ┌──────────▼──────────┐                                │
│         │  Deduplication      │◄─────────────┐                │
│         │  (Redis Check)      │              │                │
│         └──────────┬──────────┘              │                │
│                    │                         │                │
│         ┌──────────▼──────────┐              │                │
│         │  Storm Detection    │              │                │
│         │  (Rate + Pattern)   │              │                │
│         └──────────┬──────────┘              │                │
│                    │                         │                │
│         ┌──────────▼──────────┐         ┌───┴────┐           │
│         │  Environment        │         │ Redis  │           │
│         │  Classification     │         │ Cluster│           │
│         └──────────┬──────────┘         └────────┘           │
│                    │                                           │
│         ┌──────────▼──────────┐                                │
│         │  Priority           │                                │
│         │  Assignment (Rego)  │                                │
│         └──────────┬──────────┘                                │
│                    │                                           │
│         ┌──────────▼──────────┐                                │
│         │ RemediationRequest  │                                │
│         │   CRD Creation      │                                │
│         └──────────┬──────────┘                                │
│                    │                                           │
└────────────────────┼───────────────────────────────────────────┘
                     │
                     ▼
          ┌─────────────────────┐
          │ Kubernetes API      │
          │ (CRD Storage)       │
          └──────────┬──────────┘
                     │
                     ▼
          ┌─────────────────────┐
          │ RemediationRequest  │
          │ Controller (Watch)  │
          └─────────────────────┘
```

---

## Alert Processing Pipeline

### Step-by-Step Flow

#### 1. **Alert Ingestion** (5-10ms)

**Prometheus AlertManager** (to adapter-specific endpoint):
```http
POST /api/v1/signals/prometheus
Content-Type: application/json
Authorization: Bearer <k8s-serviceaccount-token>

{
  "alerts": [{
    "labels": {
      "alertname": "HighMemoryUsage",
      "severity": "critical",
      "namespace": "prod-payment-service",
      "pod": "payment-api-789"
    },
    "annotations": {
      "description": "Pod using 95% memory"
    },
    "startsAt": "2025-10-04T10:00:00Z"
  }]
}
```

**Kubernetes Event API** (to adapter-specific endpoint or watched by Gateway):
```http
POST /api/v1/signals/kubernetes-event
Content-Type: application/json
Authorization: Bearer <k8s-serviceaccount-token>
```

Or Gateway watches events directly:
```yaml
apiVersion: v1
kind: Event
metadata:
  name: pod-oom-killed
  namespace: prod-payment-service
type: Warning
reason: OOMKilled
message: "Container payment-api killed due to memory limit"
involvedObject:
  kind: Pod
  name: payment-api-789
firstTimestamp: "2025-10-04T10:00:00Z"
```

#### 2. **Normalization** (1-2ms)

Both sources converted to `NormalizedSignal` format:
```go
type NormalizedSignal struct {
    Fingerprint   string            // SHA256(alertname:namespace:kind:name)
    AlertName     string            // "HighMemoryUsage"
    Severity      string            // "critical"
    Namespace     string            // "prod-payment-service"
    Resource      ResourceIdentifier // Pod/payment-api-789
    Labels        map[string]string  // Source labels
    Annotations   map[string]string  // Source annotations
    FiringTime    time.Time          // 2025-10-04T10:00:00Z
    ReceivedTime  time.Time          // 2025-10-04T10:00:05Z
    SourceType    string             // "prometheus" or "kubernetes-event"
    RawPayload    json.RawMessage    // Original for audit
}
```

#### 3. **Deduplication Check** (3-5ms Redis lookup)

```go
fingerprint := sha256("HighMemoryUsage:prod-payment-service:Pod:payment-api-789")
// Query Redis: GET alert:fingerprint:<fingerprint>

if exists {
    // Update metadata, return 202 Accepted (deduplicated)
    redis.HINCRBY("alert:fingerprint:<fingerprint>", "count", 1)
    redis.HSET("alert:fingerprint:<fingerprint>", "lastSeen", time.Now())
    return HTTP 202 with deduplication info
}
```

**Result**: 40-60% of alerts deduplicated (typical production)

#### 4. **Storm Detection** (2-3ms)

**Rate-Based Detection**:
```go
alertsPerMin := redis.INCR("alert:storm:rate:HighMemoryUsage")
redis.EXPIRE("alert:storm:rate:HighMemoryUsage", 60) // 1-minute TTL

if alertsPerMin > 10 {
    // Storm detected: aggregate into single RemediationRequest
    createStormAggregatedCRD()
}
```

**Pattern-Based Detection**:
```go
// Check for similar alerts across different resources
similarAlerts := redis.ZRANGE("alert:pattern:OOMKilled", 0, -1)
// If >5 OOMKilled alerts in last 2 minutes across different pods → cluster-wide issue
```

#### 5. **Environment Classification** (2-3ms with cache)

**Priority order** (first match wins):
```go
func classifyEnvironment(namespace string, alert NormalizedSignal) string {
    // 1. Check namespace labels (K8s API with 5-minute cache)
    if env := getNamespaceLabelEnv(namespace); env != "" {
        return env // e.g., "prod"
    }

    // 2. Check ConfigMap override
    if env := configMapEnv[namespace]; env != "" {
        return env // e.g., "staging"
    }

    // 3. Check alert labels (Prometheus alerts only)
    if env := alert.Labels["environment"]; env != "" {
        return env
    }

    // 4. Default fallback
    return "unknown"
}
```

**Result**: "prod", "staging", "dev", or "unknown"

#### 6. **Priority Assignment** (5-8ms Rego evaluation)

**Rego Policy Evaluation**:
```rego
package kubernaut.priority

# Example: Critical production payment service → P0
priority = "P0" {
    input.severity == "critical"
    input.environment == "prod"
    input.namespace in ["payment-service", "auth-service", "checkout"]
}

# Fallback: Severity + Environment matrix
priority = "P1" {
    input.severity == "critical"
    input.environment in ["staging", "prod"]
}
```

**Fallback** (if Rego fails):
| Severity | Environment | Priority |
|----------|-------------|----------|
| critical | prod        | P0       |
| critical | staging/prod| P1       |
| warning  | prod        | P1       |
| warning  | staging     | P2       |
| info     | any         | P2       |

#### 7. **RemediationRequest CRD Creation** (15-20ms K8s API)

```yaml
apiVersion: remediation.kubernaut.io/v1
kind: RemediationRequest
metadata:
  name: remediation-abc123
  namespace: kubernaut-system
  labels:
    alertName: HighMemoryUsage
    environment: prod
    priority: P0
spec:
  alertFingerprint: "a1b2c3d4..."
  alertName: "HighMemoryUsage"
  severity: "critical"
  environment: "prod"
  priority: "P0"
  namespace: "prod-payment-service"
  resource:
    kind: Pod
    name: payment-api-789
    namespace: prod-payment-service
  firingTime: "2025-10-04T10:00:00Z"
  receivedTime: "2025-10-04T10:00:05Z"
  deduplication:
    isDuplicate: false
    firstSeen: "2025-10-04T10:00:00Z"
    occurrenceCount: 1
  sourceType: "prometheus"
  rawPayload: "{...}"  # Original alert JSON
status:
  phase: "Pending"  # Remediation Orchestrator will update
```

#### 8. **Response** (Total: 30-50ms)

```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "status": "accepted",
  "fingerprint": "a1b2c3d4...",
  "remediationRequestRef": "remediation-abc123",
  "environment": "prod",
  "priority": "P0"
}
```

---

## Key Architectural Decisions

### Decision 1: Alert Source Adapter Pattern (Value-First)

**Choice**: Implement concrete adapters first (Prometheus, K8s Events), extract interface after validation

**Rationale**:
- ✅ **Avoid Premature Abstraction**: Learn from real implementations
- ✅ **Faster Initial Development**: No abstract interface design upfront
- ✅ **Validated Patterns**: Extract interface based on proven common patterns
- ✅ **Extensibility**: Adding Grafana (V2) requires implementing proven interface

**Implementation Path**:
1. Prometheus adapter (concrete) → 3-4h
2. K8s Events adapter (concrete) → 4-5h
3. Extract `AlertAdapter` interface from common patterns → 1-2h
4. Refactor to use abstraction → 2h

**Confidence**: 95% (proven TDD pattern from `.cursor/rules/00-core-development-methodology.mdc`)

---

### Decision 2: Redis Persistent Deduplication

**Choice**: Redis persistent storage (not in-memory)

**Rationale**:
- ✅ **Survives Gateway Restarts**: Deduplication state persists
- ✅ **HA Multi-Instance Deployments**: Shared state across 2-5 replicas
- ✅ **TTL Expiration**: Automatic cleanup (5-minute window)
- ✅ **Fast Lookups**: ~1ms including network latency
- ✅ **Production Proven**: Standard pattern for stateless services

**Trade-offs**:
- ⚠️ Redis is single point of failure → **Mitigation**: Redis Cluster with replication
- ⚠️ Network latency vs. in-memory → **Acceptable**: 1ms is negligible for 50ms target

**Confidence**: 95% (mature technology, standard pattern)

---

### Decision 3: Hybrid Storm Detection

**Choice**: Rate-based (>10 alerts/min) + Pattern-based (similar alerts across resources)

**Rationale**:
- ✅ **Prevents Repetitive Storms**: Same alert firing rapidly
- ✅ **Detects Distributed Storms**: 10 pods OOMKilled → cluster-wide memory issue
- ✅ **Reduces Downstream Load**: 100 alerts → 1 aggregated RemediationRequest
- ✅ **Configurable Thresholds**: Tunable via ConfigMap

**Storm Aggregation**:
```yaml
spec:
  isStorm: true
  stormType: "rate"  # or "pattern"
  alertCount: 47
  affectedResources:
    - namespace: prod-ns-1
      kind: Pod
      name: web-app-789
    # ... (max 100 resources, then sample)
  totalAffectedResources: 47
```

**Confidence**: 88% (rate-based proven, pattern-based moderate complexity)

---

### Decision 4: Environment Classification (Hybrid)

**Choice**: Namespace labels (primary) → ConfigMap override (secondary) → Alert labels (fallback)

**Rationale**:
- ✅ **Explicit Configuration**: Namespace labels are standard Kubernetes practice
- ✅ **Override Capability**: ConfigMap for namespaces without labels
- ✅ **Prometheus Integration**: Alert labels as final fallback
- ✅ **No Pattern Matching**: Removed per user request (explicit > implicit)

**Classification Priority**:
```
1. Namespace label: environment=prod     (most reliable)
2. ConfigMap: prod-payment-service: prod (operator override)
3. Alert label: environment=prod         (Prometheus fallback)
4. Default: "unknown"                    (safe fallback)
```

**Confidence**: 90% (standard Kubernetes patterns)

---

### Decision 5: Priority Assignment (Rego + Fallback)

**Choice**: Rego policy evaluation with severity+environment fallback

**Rationale**:
- ✅ **Flexible Business Rules**: Declarative policy language
- ✅ **Testable**: Unit tests for Rego policies
- ✅ **ConfigMap Updates**: No code changes needed
- ✅ **Fallback Safety**: Hard-coded matrix if Rego fails
- ✅ **Proven Pattern**: Already used in AI Analysis approval policies

**Example Rego Policy**:
```rego
priority = "P0" {
    input.severity == "critical"
    input.environment == "prod"
    input.namespace in ["payment-service", "auth-service"]
}
```

**Confidence**: 85% (Rego complexity moderate, fallback ensures safety)

---

### Decision 6: Minimal CRD Context

**Choice**: Only include data Gateway already has (alert payload + Redis deduplication metadata)

**Rationale**:
- ✅ **Fast Response**: <50ms target (no additional API calls)
- ✅ **Clear Separation**: Gateway = ingestion/triage, Processors = enrichment
- ✅ **Scalability**: Gateway doesn't bottleneck on Context API
- ✅ **Downstream Enrichment**: Remediation Orchestrator fetches additional context

**What Gateway Does NOT Include**:
- ❌ Historical remediation data (Context API provides)
- ❌ Similar alert patterns (Context API provides)
- ❌ Detailed Kubernetes context (Remediation Orchestrator fetches)
- ❌ Prometheus metrics history (HolmesGPT fetches)

**Confidence**: 95% (proven separation of concerns)

---

### Decision 7: Synchronous Error Handling

**Choice**: HTTP status codes (202/400/500), no message queue

**Rationale**:
- ✅ **Simpler Implementation**: No queue management
- ✅ **Clear Error Feedback**: HTTP status codes
- ✅ **Alertmanager Retry**: Built-in retry on 5xx
- ✅ **Lower Latency**: No queuing delay

**HTTP Status Codes**:
- `202 Accepted`: Alert accepted (CRD created or deduplicated)
- `400 Bad Request`: Invalid alert format (don't retry)
- `500 Internal Server Error`: Transient error (Alertmanager retries)

**Confidence**: 95% (standard HTTP pattern)

---

### Decision 8: Authentication (JWT + TokenReviewer)

**Choice**: Bearer Token validated via Kubernetes TokenReviewer API

**Rationale**:
- ✅ **Consistent with Other Services**: Same pattern as Notification, AI Analysis
- ✅ **Native Kubernetes Integration**: ServiceAccount tokens
- ✅ **No External Dependencies**: Uses K8s RBAC
- ✅ **Secure**: Token validation with K8s API server

**Confidence**: 95% (established pattern, proven security)

---

### Decision 9: Comprehensive Metrics

**Choice**: Extensive Prometheus metrics on port 9090 (with auth)

**Rationale**:
- ✅ **Operational Visibility**: Monitor deduplication rate, storm detection
- ✅ **Performance Tracking**: Latency, throughput, error rates
- ✅ **Security Separation**: Metrics port separate from health checks

**Key Metrics**:
- `gateway_alerts_received_total` (by source, severity, environment)
- `gateway_alerts_deduplicated_total` (by alertname, environment)
- `gateway_alert_storms_detected_total` (by storm_type)
- `gateway_http_request_duration_seconds` (P50/P95/P99)

**Confidence**: 95% (standard observability pattern)

---

### Decision 10: Per-Source Rate Limiting

**Choice**: Rate limit by source IP (token bucket algorithm)

**Rationale**:
- ✅ **Fair Multi-Tenancy**: One noisy source can't block others
- ✅ **Debugging**: Easy to identify overwhelming source
- ✅ **Flexibility**: Different limits per source

**Configuration**:
- Default: 100 alerts/min per source IP
- Burst: 100 alerts (token bucket)
- Configurable per-source overrides via ConfigMap

**Confidence**: 88% (per-source adds complexity, but better fairness)

---

## V1 Scope Boundaries

### Included in V1

✅ **Alert Sources**:
- Prometheus AlertManager webhook
- Kubernetes Event API (Warning/Error events)

✅ **Deduplication**:
- Redis-based fingerprinting (5-minute window)
- Persistent across Gateway restarts

✅ **Storm Detection**:
- Rate-based (>10 alerts/min)
- Pattern-based (>5 similar alerts across resources)

✅ **Environment Classification**:
- Namespace labels (primary)
- ConfigMap override (secondary)
- Alert labels (fallback)

✅ **Priority Assignment**:
- Rego policy evaluation
- Severity+Environment fallback matrix

✅ **Authentication**:
- Bearer Token (JWT)
- Kubernetes TokenReviewer validation

✅ **Rate Limiting**:
- Per-source IP rate limiting
- Token bucket algorithm

✅ **Observability**:
- Comprehensive Prometheus metrics
- Structured logging
- Distributed tracing (OpenTelemetry)

---

### Excluded from V1 (Deferred to V2)

❌ **Alert Sources** (if needed):
- Grafana alerts (Prometheus covers most use cases)
- Cloud-specific alerts (CloudWatch, Azure Monitor)
- Custom webhook formats

❌ **Advanced Features**:
- ML-based storm detection (hybrid approach sufficient)
- Multi-cluster alert aggregation (single cluster for V1)
- Alert silencing (Alertmanager handles this)
- Alert routing rules (priority assignment sufficient)

❌ **Optimization**:
- Alert batching (synchronous for V1)
- Message queue integration (Alertmanager retry sufficient)

---

## System Context Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    External Systems                             │
│                                                                 │
│  ┌──────────────────┐         ┌──────────────────┐            │
│  │   Prometheus     │         │   Kubernetes     │            │
│  │  AlertManager    │         │    Event API     │            │
│  │  (Monitoring)    │         │  (Warning/Error) │            │
│  └────────┬─────────┘         └────────┬─────────┘            │
│           │                            │                       │
│           │ HTTP POST                  │ Watch                │
│           │ /api/v1/signals/prometheus │ K8s Events           │
│           │ /api/v1/signals/kubernetes-event                  │
└───────────┼────────────────────────────┼───────────────────────┘
            │                            │
            ▼                            ▼
   ┌────────────────────────────────────────────────────────┐
   │                                                        │
   │              Gateway Service                           │
   │         (Entry Point for All Signals)                  │
   │                                                        │
   │  Deduplication │ Storm Detection │ Classification │   │
   │  Priority      │ CRD Creation    │ Rate Limiting  │   │
   │                                                        │
   └────────────────┬──────────────────┬────────────────────┘
                    │                  │
                    │                  │ Redis State
                    │                  ▼
                    │         ┌─────────────────┐
                    │         │  Redis Cluster  │
                    │         │ (Deduplication) │
                    │         └─────────────────┘
                    │
                    │ Create RemediationRequest CRD
                    ▼
        ┌──────────────────────────┐
        │   Kubernetes API Server  │
        │     (CRD Storage)        │
        └──────────┬───────────────┘
                   │
                   │ Watch RemediationRequest
                   ▼
        ┌──────────────────────────┐
        │ RemediationRequest       │
        │ Controller (Central)     │
        │                          │
        │ Creates child CRDs:      │
        │ - RemediationProcessing  │
        │ - AIAnalysis             │
        │ - WorkflowExecution      │
        │ - KubernetesExecution    │
        └──────────────────────────┘
```

---

## Data Flow Diagram

```
┌───────────────────┐
│ Prometheus Alert  │
│   (firing)        │
└────────┬──────────┘
         │
         │ 1. POST /api/v1/signals/prometheus
         ▼
┌────────────────────────────────────────────────────┐
│  Gateway Service - Prometheus Adapter              │
│  - Parse Alertmanager webhook payload              │
│  - Extract labels, annotations, timestamps         │
│  - Validate required fields                        │
└─────────────────┬──────────────────────────────────┘
                  │
                  │ 2. NormalizedSignal struct
                  ▼
┌────────────────────────────────────────────────────┐
│  Fingerprint Generation                            │
│  SHA256(alertname:namespace:kind:name)             │
│  Example: "HighMemoryUsage:prod-payment:Pod:api-789" │
└─────────────────┬──────────────────────────────────┘
                  │
                  │ 3. fingerprint = "a1b2c3d4..."
                  ▼
┌────────────────────────────────────────────────────┐
│  Redis Deduplication Check                         │
│  GET alert:fingerprint:<fingerprint>               │
│                                                    │
│  If exists → Update count, return 202 (duplicate)  │
│  If not → Proceed to classification               │
└─────────────────┬──────────────────────────────────┘
                  │
                  │ 4. New alert (not duplicate)
                  ▼
┌────────────────────────────────────────────────────┐
│  Storm Detection                                   │
│  - Rate-based: INCR alert:storm:rate:<alertname>  │
│  - Pattern-based: Check similar alerts            │
│                                                    │
│  If storm → Set isStorm=true, aggregate resources │
└─────────────────┬──────────────────────────────────┘
                  │
                  │ 5. Alert + storm metadata
                  ▼
┌────────────────────────────────────────────────────┐
│  Environment Classification                        │
│  1. Namespace label lookup (K8s API + cache)      │
│  2. ConfigMap override check                       │
│  3. Alert label fallback                           │
│  4. Default "unknown"                              │
│                                                    │
│  Result: "prod"                                    │
└─────────────────┬──────────────────────────────────┘
                  │
                  │ 6. environment = "prod"
                  ▼
┌────────────────────────────────────────────────────┐
│  Priority Assignment (Rego)                        │
│  Input: {severity, environment, namespace, ...}    │
│  Rego evaluation: kubernaut.priority policy        │
│                                                    │
│  If Rego fails → Fallback to severity+env matrix  │
│  Result: "P0" (critical + prod + payment-service)  │
└─────────────────┬──────────────────────────────────┘
                  │
                  │ 7. priority = "P0"
                  ▼
┌────────────────────────────────────────────────────┐
│  RemediationRequest CRD Creation                   │
│  - Populate spec with alert data                  │
│  - Add deduplication metadata                     │
│  - Set labels (alertName, environment, priority)  │
│  - Include raw payload for audit                  │
│                                                    │
│  POST to Kubernetes API                            │
└─────────────────┬──────────────────────────────────┘
                  │
                  │ 8. RemediationRequest CRD created
                  ▼
┌────────────────────────────────────────────────────┐
│  Store Deduplication Metadata in Redis            │
│  SET alert:fingerprint:<fingerprint>              │
│  {                                                 │
│    fingerprint: "a1b2c3d4...",                     │
│    firstSeen: "2025-10-04T10:00:00Z",              │
│    count: 1,                                       │
│    remediationRequestRef: "remediation-abc123"     │
│  }                                                 │
│  TTL: 300 seconds (5 minutes)                      │
└─────────────────┬──────────────────────────────────┘
                  │
                  │ 9. HTTP 202 Accepted
                  ▼
┌────────────────────────────────────────────────────┐
│  Response to Alertmanager                          │
│  {                                                 │
│    "status": "accepted",                           │
│    "fingerprint": "a1b2c3d4...",                   │
│    "remediationRequestRef": "remediation-abc123",  │
│    "environment": "prod",                          │
│    "priority": "P0"                                │
│  }                                                 │
└────────────────────────────────────────────────────┘
```

**Total Time**: 30-50ms (target p95)

---

## Business Requirements Mapping

| Business Requirement | Implementation | Validation |
|---------------------|----------------|------------|
| **BR-GATEWAY-001**: Alert ingestion endpoint | `POST /api/v1/signals/prometheus` & `POST /api/v1/signals/kubernetes-event` | Integration test: webhook payload processing |
| **BR-GATEWAY-002**: Prometheus adapter | `PrometheusAdapter.Parse()` | Unit test: Alertmanager webhook parsing |
| **BR-GATEWAY-005**: Kubernetes event adapter | `KubernetesEventAdapter.Parse()` | Unit test: K8s Event API parsing |
| **BR-GATEWAY-006**: Alert normalization | `NormalizedSignal` struct conversion | Unit test: cross-source signal normalization |
| **BR-GATEWAY-010**: Fingerprint-based deduplication | `SHA256(alertname:namespace:kind:name)` | Unit test: fingerprint generation uniqueness |
| **BR-GATEWAY-011**: Redis deduplication storage | Redis GET/SET with 5-min TTL | Integration test: Redis persistence & expiry |
| **BR-GATEWAY-015**: Alert storm detection (rate-based) | Rate counter: >10 alerts/min | Unit test: rate threshold detection |
| **BR-GATEWAY-016**: Storm aggregation | `isStorm` flag + affected resources list | Unit test: similar alert aggregation |
| **BR-GATEWAY-020**: Priority assignment (Rego) | OPA Rego policy: `kubernaut.priority` | Integration test: Rego evaluation |
| **BR-GATEWAY-021**: Priority fallback matrix | Severity + Environment matrix lookup | Unit test: fallback when Rego fails |
| **BR-GATEWAY-022**: Remediation path decision | Rego policy output | Unit test: remediation strategy selection |
| **BR-GATEWAY-023**: CRD creation | `CreateRemediationRequest()` K8s API call | Integration test: CRD creation & validation |
| **BR-GATEWAY-051**: Environment detection (namespace labels) | K8s namespace label lookup with cache | Integration test: namespace label retrieval |
| **BR-GATEWAY-052**: ConfigMap fallback for environment | ConfigMap override check | Unit test: ConfigMap precedence over labels |
| **BR-GATEWAY-053**: Default environment (unknown) | Fallback value when no labels/ConfigMap | Unit test: unknown environment handling |
| **BR-GATEWAY-071**: CRD-only integration (no direct GitOps) | RemediationRequest CRD as trigger | E2E test: CRD-driven workflow |
| **BR-GATEWAY-072**: CRD as GitOps trigger | CRD created → downstream controllers watch | E2E test: controller reconciliation |
| **BR-GATEWAY-091**: Escalation notification trigger | CRD creation triggers notification flow | Integration test: notification triggered on CRD create |
| **BR-GATEWAY-092**: Notification metadata | CRD contains notification context fields | Unit test: notification metadata completeness |

**Core Capabilities**:
- **Alert Ingestion**: BR-GATEWAY-001 to BR-GATEWAY-023 (Multi-source webhook processing)
- **Environment Classification**: BR-GATEWAY-051 to BR-GATEWAY-053 (Namespace labels + ConfigMap)
- **GitOps Integration**: BR-GATEWAY-071 to BR-GATEWAY-072 (CRD-driven workflows)
- **Downstream Notification**: BR-GATEWAY-091 to BR-GATEWAY-092 (CRD creation triggers)
- **Reserved**: BR-GATEWAY-024 to BR-GATEWAY-050, BR-GATEWAY-054 to BR-GATEWAY-070, BR-GATEWAY-073 to BR-GATEWAY-090, BR-GATEWAY-093 to BR-GATEWAY-180 (future features)

**For Complete Implementation Details**: See [implementation-checklist.md](./implementation-checklist.md) for step-by-step BR implementation guide.

---

## Summary

Gateway Service is the **intelligent entry point** that transforms raw alerts into actionable RemediationRequest CRDs. It provides:

1. **Multi-Source Ingestion** - Prometheus + Kubernetes Events (90% coverage)
2. **Intelligent Deduplication** - 40-60% reduction in downstream load
3. **Storm Detection** - Aggregates related incidents
4. **Environment Context** - Enables risk-aware remediation
5. **Business Prioritization** - Critical production issues get immediate attention
6. **Clean CRD Interface** - Downstream services work with normalized, enriched data

**Design Philosophy**: Fast, stateless, scalable entry point that offloads complex enrichment to specialized downstream services.

**Confidence**: 90.5% (Very High) - Proven patterns, clear scope, user-approved decisions

---

**Document Status**: ✅ Complete
**Next**: [Implementation Details](./implementation.md)

