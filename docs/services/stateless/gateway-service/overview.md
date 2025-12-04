# Gateway Service - Overview

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.4 | 2025-12-03 | Added TargetResource, TargetType fields; resource validation rejects signals without K8s info | [DD-GATEWAY-NON-K8S-SIGNALS](../../../architecture/decisions/DD-GATEWAY-NON-K8S-SIGNALS.md) |
> | v1.3 | 2025-12-01 | DeduplicationInfo aligned with shared types (firstOccurrence/lastOccurrence) | [RemediationRequest CRD](../../../../api/remediation/v1alpha1/remediationrequest_types.go) |
> | v1.2 | 2025-11-27 | Categorization delegated to SignalProcessing; Gateway sets placeholder values | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
> | v1.1 | 2025-11-19 | Storm aggregation window, Redis state cleanup on CRD deletion | [DD-GATEWAY-008](../../../architecture/decisions/DD-GATEWAY-008-storm-aggregation-first-alert-handling.md) |
> | v1.0 | 2025-10-04 | Initial design specification | - |

> **📋 Design Decision: DD-CATEGORIZATION-001 - Categorization Delegation**
> **Status**: ✅ Approved Design | **Confidence**: 95%
> **Impact on Gateway**: Gateway sets placeholder priority values; SignalProcessing performs final categorization with richer K8s context
> **See**: [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)

> **📋 Design Decision: DD-GATEWAY-NON-K8S-SIGNALS - Non-Kubernetes Signal Support**
> **Status**: ✅ V1.0 Decision: Kubernetes-only | **Future**: V1.x/V2.0 expansion
> **Impact**: V1.0 validates and rejects signals without Kubernetes resource information
> **See**: [DD-GATEWAY-NON-K8S-SIGNALS](../../../architecture/decisions/DD-GATEWAY-NON-K8S-SIGNALS.md)

---

**Purpose**: Single entry point for all external signals (Prometheus alerts, Kubernetes events) into the Kubernaut intelligent remediation system.

**Core Responsibilities**:
1. **Ingest** alerts from multiple sources (Prometheus AlertManager, K8s Events)
2. **Validate** signal completeness and resource information
3. **Deduplicate** repetitive alerts to prevent redundant processing
4. **Detect** alert storms to aggregate related incidents
5. **Classify** environment context for risk assessment (placeholder for SignalProcessing)
6. **Create** RemediationRequest CRDs for downstream orchestration

**V1 Scope - Signal Ingestion, Deduplication, Storm Detection**:
- **Signal Ingestion**: Prometheus AlertManager webhooks, Kubernetes Event API
- **Resource Validation**: Reject signals without Kubernetes resource information ([DD-GATEWAY-NON-K8S-SIGNALS](../../../architecture/decisions/DD-GATEWAY-NON-K8S-SIGNALS.md))
- **Deduplication**: Redis-based fingerprinting with 5-minute TTL window
- **Storm Detection**: Rate-based (>10 alerts/min) + Pattern-based (>5 similar alerts)
- **Environment Classification**: Placeholder values; final classification by SignalProcessing
- **CRD Creation**: RemediationRequest with TargetResource, TargetType fields

**Key Architectural Decisions**:
- Stateless HTTP server (horizontal scaling with 2-5 replicas)
- Redis for shared deduplication state across replicas
- CRD-based integration (no direct HTTP calls to downstream services)
- **Categorization delegated** to SignalProcessing ([DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md))
- **Audit writes via Data Storage Service REST API** ([ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md))
- **Graceful shutdown** following [DD-007](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md)

---

## Business Requirements Coverage

**Gateway Service** implements signal ingestion and preprocessing for Kubernetes remediation:

### V1 Scope: Signal Ingestion & Preprocessing (BR-GATEWAY-001 to BR-GATEWAY-092)

**Range**: BR-GATEWAY-001 to BR-GATEWAY-180
**V1 Active**: 25+ BRs (see [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md))
**V2 Reserved**: BR-GATEWAY-093 to BR-GATEWAY-180 (additional alert sources, ML-based detection)

**V1 Business Requirements Breakdown**:

#### Core Signal Ingestion (BR-GATEWAY-001 to BR-GATEWAY-023)
**Count**: 8 BRs
**Focus**: Alert ingestion, normalization, validation, and CRD creation

**Primary Functions**:
- Alert source adapters (Prometheus, K8s Events)
- Signal normalization to internal format
- Fingerprint-based deduplication
- RemediationRequest CRD creation

#### Environment & Priority (BR-GATEWAY-051 to BR-GATEWAY-053)
**Count**: 3 BRs
**Focus**: Environment classification with fallback heuristics

**Note**: Gateway sets placeholder values; SignalProcessing performs final categorization per [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md).

#### Resource Validation (BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
**Count**: 1 BR
**Focus**: Validate signals contain Kubernetes resource information

**Implementation**: V1.0 rejects signals without `Kind` and `Name` in resource info. Future versions may support non-Kubernetes targets ([DD-GATEWAY-NON-K8S-SIGNALS](../../../architecture/decisions/DD-GATEWAY-NON-K8S-SIGNALS.md)).

#### GitOps & Notification Integration (BR-GATEWAY-071 to BR-GATEWAY-092)
**Count**: 4 BRs
**Focus**: CRD-driven workflows and escalation triggers

---

## Service Configuration

### Port Configuration
- **Port 8080**: HTTP API (signal ingestion endpoints)
- **Port 8081**: Health probes (`/healthz`, `/readyz`)
- **Port 9090**: Metrics endpoint (`/metrics`)
- **Format**: Prometheus text format
- **Authentication**: Kubernetes TokenReviewer API (validates ServiceAccount tokens)

### API Endpoints
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/signals/prometheus` | POST | Prometheus AlertManager webhook |
| `/api/v1/signals/kubernetes-event` | POST | Kubernetes Event ingestion |
| `/healthz` | GET | Liveness probe |
| `/readyz` | GET | Readiness probe |
| `/metrics` | GET | Prometheus metrics (authenticated) |

### ServiceAccount
- **Name**: `gateway-sa`
- **Namespace**: `kubernaut-system`
- **Purpose**: CRD creation, Redis access, namespace label reading

---

## 📊 Visual Architecture

### Architecture Diagram

```mermaid
graph TB
    subgraph "Gateway Service"
        WH[Webhook Handler]
        PA[Prometheus Adapter]
        KA[K8s Event Adapter]
        Norm[Signal Normalizer]
        Dedup[Deduplicator]
        Storm[Storm Detector]
        Class[Environment Classifier]
        Creator[CRD Creator]
    end

    subgraph "External Systems"
        PM[Prometheus AlertManager]
        K8S[Kubernetes Event API]
        Redis[Redis Cluster]
    end

    subgraph "Downstream"
        RR[RemediationRequest CRD]
        RO[Remediation Orchestrator]
    end

    PM -->|Webhook| WH
    K8S -->|Watch/Webhook| WH
    WH --> PA
    WH --> KA
    PA --> Norm
    KA --> Norm
    Norm --> Dedup
    Dedup <-->|Check/Store| Redis
    Dedup --> Storm
    Storm <-->|Rate Counters| Redis
    Storm --> Class
    Class --> Creator
    Creator -->|Create| RR
    RR -->|Watch| RO

    style WH fill:#e1f5ff
    style Dedup fill:#fff4e1
    style Storm fill:#fff4e1
    style RR fill:#ffe1e1
```

### Sequence Diagram - Signal Processing Flow

```mermaid
sequenceDiagram
    participant PM as Prometheus<br/>AlertManager
    participant GW as Gateway<br/>Service
    participant Redis as Redis<br/>Cluster
    participant K8S as Kubernetes<br/>API

    PM->>GW: POST /api/v1/signals/prometheus
    activate GW

    Note over GW: 1. Parse & Normalize
    GW->>GW: Validate resource info

    Note over GW: 2. Check Deduplication
    GW->>Redis: GET alert:fingerprint:<hash>
    Redis-->>GW: null (new) or metadata (duplicate)

    alt New Signal
        Note over GW: 3. Check Storm Detection
        GW->>Redis: INCR alert:storm:rate:<alertname>
        Redis-->>GW: count

        Note over GW: 4. Classify Environment
        GW->>GW: namespace labels → placeholder

        Note over GW: 5. Create CRD
        GW->>K8S: Create RemediationRequest
        K8S-->>GW: Created

        Note over GW: 6. Store Dedup Metadata
        GW->>Redis: SET alert:fingerprint:<hash>

        GW-->>PM: 201 Created
    else Duplicate Signal
        GW->>Redis: HINCRBY count
        GW-->>PM: 202 Accepted (deduplicated)
    end

    deactivate GW
```

### State Machine - Storm Detection

```mermaid
stateDiagram-v2
    [*] --> Normal
    Normal --> Monitoring: Alert received
    Monitoring --> Monitoring: Count < threshold
    Monitoring --> StormDetected: Count >= threshold
    StormDetected --> Buffering: Start aggregation window
    Buffering --> Aggregating: Window expires
    Aggregating --> Normal: Storm CRD created
    Monitoring --> Normal: TTL expires

    note right of Monitoring
        Rate-based: INCR per minute
        Pattern-based: Similar alerts
        Threshold: configurable (default 10)
    end note

    note right of Buffering
        Aggregation window: 1 minute
        Buffer alerts for batch creation
    end note
```

---

## Package Structure

**Implemented Structure**: `{cmd,pkg}/gateway/`

Following Go idioms and codebase patterns, the Gateway service uses:

```
cmd/gateway/               → Main application entry point
  └── main.go

pkg/gateway/               → Business logic (PUBLIC API)
  ├── server.go               → HTTP server and routing
  ├── config/                 → Configuration management
  │   └── config.go
  ├── adapters/               → Signal source adapters
  │   ├── prometheus.go
  │   └── kubernetes_event.go
  ├── processing/             → Core processing logic
  │   ├── normalizer.go
  │   ├── deduplication.go
  │   ├── storm_detector.go
  │   ├── storm_aggregator.go
  │   ├── classifier.go
  │   ├── crd_creator.go
  │   └── crd_updater.go
  └── metrics/                → Prometheus metrics
      └── metrics.go

internal/gateway/          → Internal utilities (INTERNAL)
  └── (future internal types)
```

---

## Development Methodology

**Mandatory Process**: Follow APDC-Enhanced TDD workflow per [.cursor/rules/00-core-development-methodology.mdc](../../../.cursor/rules/00-core-development-methodology.mdc)

### APDC-TDD Workflow

```
┌─────────────────────────────────────────────────────────────┐
│ ANALYSIS → PLAN → DO-RED → DO-GREEN → DO-REFACTOR → CHECK  │
└─────────────────────────────────────────────────────────────┘
```

**ANALYSIS** (5-15 min): Comprehensive context understanding
  - Search existing implementations (`codebase_search "Gateway implementations"`)
  - Identify reusable components in `pkg/gateway/`
  - Map business requirements (BR-GATEWAY-XXX)

**PLAN** (10-20 min): Detailed implementation strategy
  - Define TDD phase breakdown (RED → GREEN → REFACTOR)
  - Plan integration points
  - Establish success criteria (< 50ms p95 response time)

**DO-RED** (10-15 min): Write failing tests FIRST
  - Unit tests defining business contract (70%+ coverage target)
  - Use FAKE K8s client (`sigs.k8s.io/controller-runtime/pkg/client/fake`)
  - Mock ONLY external dependencies (Redis, HTTP clients)
  - Map tests to business requirements (BR-GATEWAY-XXX)

**DO-GREEN** (15-20 min): Minimal implementation
  - Minimal code to pass tests
  - **MANDATORY integration in cmd/gateway/**
  - Verify CRD creation works

**DO-REFACTOR** (20-30 min): Enhance with sophisticated logic
  - **NO new types/interfaces/files** (enhance existing)
  - Add storm detection algorithms
  - Optimize Redis operations

**CHECK** (5-10 min): Validation and confidence assessment
  - Business requirement verification
  - Test coverage validation (70%+ unit, 20% integration, 10% E2E)
  - Performance validation (< 50ms p95)

### Quick Decision Matrix

| Starting Point | Required Phase | Reference |
|----------------|---------------|-----------|
| **New adapter** | Full APDC workflow | New alert source pattern |
| **Enhance deduplication** | ANALYSIS → PLAN → DO-REFACTOR | Existing code well-understood |
| **Fix storm detection bugs** | ANALYSIS → DO-RED → DO-REFACTOR | Understand context first |
| **Add new tests** | DO-RED only | Write tests for existing logic |

**Testing Strategy Reference**: [testing-strategy.md](./testing-strategy.md)
  - Unit Tests (70%+): test/unit/gateway/ - Fake K8s client, mock Redis
  - Integration Tests (20%): test/integration/gateway/ - Real K8s (envtest), real Redis
  - E2E Tests (10%): test/e2e/gateway/ - Complete signal-to-CRD workflow

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

#### 2. **Normalization** (1-2ms)

Both sources converted to `NormalizedSignal` format:
```go
type NormalizedSignal struct {
    Fingerprint   string             // SHA256(alertname:namespace:kind:name)
    AlertName     string             // "HighMemoryUsage"
    Severity      string             // "critical"
    Namespace     string             // "prod-payment-service"
    Resource      ResourceIdentifier // Pod/payment-api-789
    Labels        map[string]string  // Source labels
    Annotations   map[string]string  // Source annotations
    FiringTime    time.Time          // 2025-10-04T10:00:00Z
    ReceivedTime  time.Time          // 2025-10-04T10:00:05Z
    SourceType    string             // "prometheus" or "kubernetes-event"
    RawPayload    json.RawMessage    // Original for audit
}
```

#### 3. **Resource Validation** (1ms)

```go
// V1.0: Reject signals without Kubernetes resource info
if signal.Resource.Kind == "" || signal.Resource.Name == "" {
    metrics.SignalsRejectedTotal.WithLabelValues("missing_resource_info").Inc()
    return HTTP 400 Bad Request
}
```

**See**: [DD-GATEWAY-NON-K8S-SIGNALS](../../../architecture/decisions/DD-GATEWAY-NON-K8S-SIGNALS.md) for future non-K8s support.

#### 4. **Deduplication Check** (3-5ms Redis lookup)

```go
fingerprint := sha256("HighMemoryUsage:prod-payment-service:Pod:payment-api-789")
// Query Redis: GET alert:fingerprint:<fingerprint>

if exists {
    // Update metadata, return 202 Accepted (deduplicated)
    redis.HINCRBY("alert:fingerprint:<fingerprint>", "occurrenceCount", 1)
    redis.HSET("alert:fingerprint:<fingerprint>", "lastOccurrence", time.Now())
    return HTTP 202 with deduplication info
}
```

**Result**: 40-60% of alerts deduplicated (typical production)

#### 5. **Storm Detection** (2-3ms)

**Rate-Based Detection**:
```go
alertsPerMin := redis.INCR("alert:storm:rate:HighMemoryUsage")
redis.EXPIRE("alert:storm:rate:HighMemoryUsage", 60) // 1-minute TTL

if alertsPerMin > 10 {
    // Storm detected: aggregate into single RemediationRequest
    createStormAggregatedCRD()
}
```

#### 6. **RemediationRequest CRD Creation** (15-20ms K8s API)

```yaml
apiVersion: remediation.kubernaut.io/v1alpha1
kind: RemediationRequest
metadata:
  name: remediation-abc123
  namespace: kubernaut-system
  labels:
    kubernaut.ai/alert-name: HighMemoryUsage
    kubernaut.ai/environment: prod
    kubernaut.ai/priority: P0
spec:
  alertFingerprint: "a1b2c3d4..."
  alertName: "HighMemoryUsage"
  severity: "critical"
  environment: "prod"  # Placeholder - SignalProcessing finalizes
  priority: "P0"       # Placeholder - SignalProcessing finalizes
  targetResource:
    kind: Pod
    name: payment-api-789
    namespace: prod-payment-service
  targetType: "kubernetes"
  deduplication:
    isDuplicate: false
    firstOccurrence: "2025-10-04T10:00:00Z"
    lastOccurrence: "2025-10-04T10:00:00Z"
    occurrenceCount: 1
  sourceType: "prometheus"
status:
  phase: "Pending"
```

#### 7. **Response** (Total: 30-50ms)

```http
HTTP/1.1 201 Created
Content-Type: application/json

{
  "status": "created",
  "fingerprint": "a1b2c3d4...",
  "remediationRequestRef": "remediation-abc123",
  "environment": "prod",
  "priority": "P0"
}
```

---

## Key Architectural Decisions

### Decision 1: Categorization Delegation to SignalProcessing

**Choice**: Gateway sets placeholder values; SignalProcessing performs final categorization

**Rationale** ([DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)):
- ✅ **Richer Context**: SignalProcessing has access to enriched K8s context
- ✅ **Single Responsibility**: Gateway focuses on ingestion, not business logic
- ✅ **Simpler Gateway**: Reduced complexity in critical path

### Decision 2: Resource Validation (V1.0)

**Choice**: Reject signals without Kubernetes resource information

**Rationale** ([DD-GATEWAY-NON-K8S-SIGNALS](../../../architecture/decisions/DD-GATEWAY-NON-K8S-SIGNALS.md)):
- ✅ **V1.0 Scope**: Kubernetes-only signals for initial release
- ✅ **Clear Contract**: Downstream services can rely on resource info
- ✅ **Explicit Rejection**: Clear error feedback to alert sources
- 📋 **Future**: V1.x/V2.0 may add non-K8s support with `targetType` routing

### Decision 3: Redis Persistent Deduplication

**Choice**: Redis persistent storage (not in-memory)

**Rationale**:
- ✅ **Survives Gateway Restarts**: Deduplication state persists
- ✅ **HA Multi-Instance Deployments**: Shared state across 2-5 replicas
- ✅ **TTL Expiration**: Automatic cleanup (5-minute window)
- ✅ **Fast Lookups**: ~1ms including network latency

### Decision 4: Hybrid Storm Detection

**Choice**: Rate-based (>10 alerts/min) + Pattern-based (similar alerts across resources)

**Rationale** ([DD-GATEWAY-008](../../../architecture/decisions/DD-GATEWAY-008-storm-aggregation-first-alert-handling.md)):
- ✅ **Prevents Repetitive Storms**: Same alert firing rapidly
- ✅ **Detects Distributed Storms**: 10 pods OOMKilled → cluster-wide issue
- ✅ **Reduces Downstream Load**: 100 alerts → 1 aggregated RemediationRequest
- ✅ **Configurable Thresholds**: Tunable via ConfigMap

### Decision 5: Synchronous Error Handling

**Choice**: HTTP status codes (201/202/400/500), no message queue

**Rationale**:
- ✅ **Simpler Implementation**: No queue management
- ✅ **Clear Error Feedback**: HTTP status codes
- ✅ **Alertmanager Retry**: Built-in retry on 5xx
- ✅ **Lower Latency**: No queuing delay

**HTTP Status Codes**:
- `201 Created`: New alert accepted, CRD created
- `202 Accepted`: Duplicate detected, metadata updated
- `400 Bad Request`: Invalid alert format or missing resource info
- `500 Internal Server Error`: Transient error (Alertmanager retries)

---

## V1 Scope Boundaries

### Included in V1

✅ **Signal Sources**:
- Prometheus AlertManager webhook
- Kubernetes Event API (Warning/Error events)

✅ **Resource Validation**:
- Reject signals without Kubernetes resource information
- Populate `targetType: "kubernetes"` for all V1 signals

✅ **Deduplication**:
- Redis-based fingerprinting (5-minute window)
- Persistent across Gateway restarts

✅ **Storm Detection**:
- Rate-based (>10 alerts/min)
- Pattern-based (>5 similar alerts across resources)

✅ **Environment Classification**:
- Placeholder values only
- Final classification by SignalProcessing

### Excluded from V1 (Deferred to V2)

❌ **Signal Sources**:
- Non-Kubernetes signals (AWS, Azure, GCP, Datadog)
- Grafana alerts (Prometheus covers most use cases)

❌ **Advanced Features**:
- ML-based storm detection
- Multi-cluster alert aggregation
- Alert silencing (Alertmanager handles this)

---

## Summary

**Gateway Service - V1 Design Specification**

### Core Purpose
Single entry point for external signals, providing intelligent preprocessing before downstream analysis.

### Key Responsibilities
1. **Signal Ingestion** - Prometheus + Kubernetes Events (90% coverage)
2. **Resource Validation** - Reject signals without K8s resource info
3. **Intelligent Deduplication** - 40-60% reduction in downstream load
4. **Storm Detection** - Aggregates related incidents
5. **CRD Creation** - Clean interface for downstream services

### Integration Model
```
Prometheus/K8s Events → Gateway Service → RemediationRequest CRD
                                              ↓
                       RemediationOrchestrator watches CRD
                                              ↓
                       Creates SignalProcessing CRD (enrichment)
```

### Implementation Status
- **Core Processing**: Complete - deduplication, storm detection, CRD creation
- **Resource Validation**: Complete - rejects signals without K8s resource info
- **Metrics**: Complete - comprehensive Prometheus metrics
- **Tests**: Unit (333), Integration (145), E2E (pending infrastructure)

### Next Steps
1. ✅ **Resource Validation** implemented (v1.4)
2. ✅ **TargetType field** added for future non-K8s support
3. **E2E Testing** - Requires Kind cluster infrastructure
4. **Production Deployment** - Final validation

**Design Specification Status**: Production-Ready (95% Confidence)

---

## Related Documents

- [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) - Complete BR catalog
- [BR_MAPPING.md](./BR_MAPPING.md) - Test-to-BR traceability
- [testing-strategy.md](./testing-strategy.md) - Defense-in-depth testing approach
- [security-configuration.md](./security-configuration.md) - RBAC, NetworkPolicy, Secrets
- [observability-logging.md](./observability-logging.md) - Structured logging, tracing
- [metrics-slos.md](./metrics-slos.md) - Prometheus metrics, SLI/SLO
- [api-specification.md](./api-specification.md) - OpenAPI specification
- [crd-integration.md](./crd-integration.md) - RemediationRequest CRD details
- [deduplication.md](./deduplication.md) - Deduplication algorithm details
