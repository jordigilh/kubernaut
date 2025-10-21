# Gateway Service - Implementation Plan v1.0

✅ **DESIGN COMPLETE** - Implementation Pending

**Service**: Gateway Service (Entry Point for All Signals)
**Phase**: Phase 2, Service #1
**Plan Version**: v1.0 (Design Complete)
**Template Version**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v2.0
**Plan Date**: October 21, 2025
**Current Status**: ✅ DESIGN COMPLETE / ⏸️ IMPLEMENTATION PENDING
**Business Requirements**: BR-GATEWAY-001 through BR-GATEWAY-040 (estimated 40 BRs)
**Confidence**: 85% ✅ **High - Comprehensive Design**

**Architecture**: Adapter-specific self-registered endpoints (DD-GATEWAY-001)

---

## 📋 Version History

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v0.1** | Sep 2025 | Exploration: Detection-based adapter selection (Design A) | ⚠️ SUPERSEDED |
| **v0.9** | Oct 3, 2025 | Design comparison: Detection vs Specific Endpoints | ⚠️ SUPERSEDED |
| **v1.0** | Oct 4, 2025 | **Adapter-specific endpoints** (Design B, 92% confidence) | ✅ **CURRENT** |
| **v1.0.1** | Oct 21, 2025 | **Enhanced documentation**: Added Configuration Reference, Dependencies, API Examples, Service Integration, Defense-in-Depth, Test Examples, Error Handling | ✅ **CURRENT** |

---

## 🔄 v1.0 Major Architectural Decision

**Date**: October 4, 2025
**Scope**: Signal ingestion architecture
**Design Decision**: DD-GATEWAY-001 - Adapter-Specific Endpoints Architecture
**Impact**: MAJOR - 70% code reduction, improved security and performance

### What Changed

**FROM**: Detection-based adapter selection (Design A)
**TO**: Adapter-specific self-registered endpoints (Design B)

**Rationale**:
1. ✅ **~70% less code** - No detection logic needed
2. ✅ **Better security** - No source spoofing possible
3. ✅ **Better performance** - ~50-100μs faster (no detection overhead)
4. ✅ **Industry standard** - Follows REST/HTTP best practices (Stripe, GitHub, Datadog pattern)
5. ✅ **Better operations** - Clear 404 errors, simple troubleshooting, per-route metrics
6. ✅ **Configuration-driven** - Enable/disable adapters via YAML config

---

## 📊 Implementation Status

### ✅ DESIGN COMPLETE - Implementation Pending

**Current Phase**: Design Complete (Oct 4, 2025)
**Next Phase**: Implementation (Pending)

| Phase | Tests | Status | Effort | Confidence |
|-------|-------|--------|--------|------------|
| **Design Specification** | N/A | ✅ Complete | 16h | 100% |
| **Unit Tests** | 0/75 | ⏸️ Not Started | 20-25h | 85% |
| **Integration Tests** | 0/30 | ⏸️ Not Started | 15-20h | 85% |
| **E2E Tests** | 0/5 | ⏸️ Not Started | 5-10h | 85% |
| **Deployment** | N/A | ⏸️ Not Started | 8h | 90% |

**Total**: 0/110 tests passing (estimated total)
**Estimated Implementation Time**: 46-60 hours (6-8 days)

---

## 📝 Business Requirements

### ✅ ESSENTIAL (Estimated: 40 BRs)

| Category | BR Range | Count | Status | Tests |
|----------|----------|-------|--------|-------|
| **Primary Signal Ingestion** | BR-GATEWAY-001 to 023 | 23 | ⏸️ 0% | 0/45 |
| **Environment Classification** | BR-GATEWAY-051 to 053 | 3 | ⏸️ 0% | 0/10 |
| **GitOps Integration** | BR-GATEWAY-071 to 072 | 2 | ⏸️ 0% | 0/5 |
| **Notification Routing** | BR-GATEWAY-091 to 092 | 2 | ⏸️ 0% | 0/5 |
| **HTTP Server** | BR-GATEWAY-036 to 045 | 10 | ⏸️ 0% | 0/15 |
| **Health & Observability** | BR-GATEWAY-016 to 025 | 10 | ⏸️ 0% | 0/10 |
| **Authentication & Security** | BR-GATEWAY-066 to 075 | 10 | ⏸️ 0% | 0/15 |

**Total**: ~40 BRs (0% implemented)

**Note**: Business requirements need formal enumeration. Current ranges are estimated from documentation review.

---

### Primary Requirements Breakdown

#### BR-GATEWAY-001 to 023: Signal Ingestion & Processing

**Core Functionality**:
- BR-GATEWAY-001: Accept signals from Prometheus AlertManager webhooks
- BR-GATEWAY-002: Accept signals from Kubernetes Event API
- BR-GATEWAY-003: Parse and normalize Prometheus alert format
- BR-GATEWAY-004: Parse and normalize Kubernetes Event format
- BR-GATEWAY-005: Deduplicate signals using Redis fingerprinting
- BR-GATEWAY-006: Generate SHA256 fingerprints for signal identity
- BR-GATEWAY-007: Detect alert storms (rate-based: >10 alerts/min)
- BR-GATEWAY-008: Detect alert storms (pattern-based: similar alerts across resources)
- BR-GATEWAY-009: Aggregate storm alerts into single CRD
- BR-GATEWAY-010: Store deduplication metadata in Redis (5-minute TTL)
- BR-GATEWAY-011: Classify environment from namespace labels
- BR-GATEWAY-012: Classify environment from ConfigMap overrides
- BR-GATEWAY-013: Assign priority using Rego policies
- BR-GATEWAY-014: Assign priority using severity+environment fallback table
- BR-GATEWAY-015: Create RemediationRequest CRD for new signals
- BR-GATEWAY-016: Storm aggregation (1-minute window)
- BR-GATEWAY-017: Return HTTP 201 for new CRD creation
- BR-GATEWAY-018: Return HTTP 202 for duplicate signals
- BR-GATEWAY-019: Return HTTP 400 for invalid signal payloads
- BR-GATEWAY-020: Return HTTP 500 for processing errors
- BR-GATEWAY-021: Record signal metadata in CRD
- BR-GATEWAY-022: Support adapter-specific routes
- BR-GATEWAY-023: Dynamic adapter registration

**Status**: ⏸️ Not Implemented (0/23 BRs)
**Tests**: 0/45 unit tests, 0/15 integration tests

---

#### BR-GATEWAY-051 to 053: Environment Classification

**Core Functionality**:
- BR-GATEWAY-051: Support dynamic environment taxonomy (any label value)
- BR-GATEWAY-052: Cache namespace labels (5-minute TTL)
- BR-GATEWAY-053: ConfigMap override for environment classification

**Status**: ⏸️ Not Implemented (0/3 BRs)
**Tests**: 0/10 unit tests

---

#### BR-GATEWAY-071 to 072: GitOps Integration

**Core Functionality**:
- BR-GATEWAY-071: Environment determines remediation behavior
- BR-GATEWAY-072: Priority-based workflow selection

**Status**: ⏸️ Not Implemented (0/2 BRs)
**Tests**: 0/5 integration tests

---

## 🎯 v1.0 Architecture

### Core Functionality

**API Endpoints** (Adapter-specific):
```
Signal Ingestion:
  POST /api/v1/signals/prometheus         # Prometheus AlertManager webhooks
  POST /api/v1/signals/kubernetes-event   # Kubernetes Event API signals
  POST /api/v1/signals/grafana            # Grafana alerts (future)

Health & Monitoring:
  GET  /health                            # Liveness probe
  GET  /ready                             # Readiness probe
  GET  /metrics                           # Prometheus metrics
```

**Architecture**:
- **Adapter-specific endpoints**: Each adapter registers its own HTTP route
- **Configuration-driven**: Enable/disable adapters via YAML config
- **No detection logic**: HTTP routing handles adapter selection
- **Security**: No source spoofing, explicit routing, clear audit trail

**Authentication**:
- Kubernetes ServiceAccount token validation (TokenReviewer API)
- Bearer token required for all signal endpoints
- No authentication for health endpoints

**Configuration** (minimal for production):
```yaml
server:
  listen_addr: ":8080"
  read_timeout: 30s
  write_timeout: 30s

redis:
  addr: "redis:6379"
  password: "${REDIS_PASSWORD}"

rate_limit:
  requests_per_minute: 100
  burst: 10

deduplication:
  ttl: 5m

storm_detection:
  rate_threshold: 10      # alerts/minute
  pattern_threshold: 5    # similar alerts
  aggregation_window: 1m

environment:
  cache_ttl: 30s
  configmap_namespace: kubernaut-system
  configmap_name: kubernaut-environment-overrides
```

**See**: [Complete Configuration Reference](#️-configuration-reference) for all options and environment variables

---

## ⚙️ Configuration Reference

### Complete Configuration Schema

```yaml
# Complete configuration with all options
server:
  listen_addr: ":8080"              # HTTP server address
  read_timeout: 30s                 # Request read timeout
  write_timeout: 30s                # Response write timeout
  idle_timeout: 120s                # Keep-alive idle timeout
  max_header_bytes: 1048576         # 1MB max header size
  graceful_shutdown_timeout: 30s    # Shutdown grace period

redis:
  addr: "redis:6379"                # Redis server address
  password: ""                      # Redis password (use env var)
  db: 0                             # Redis database number
  max_retries: 3                    # Connection retry attempts
  min_idle_conns: 10                # Min idle connections
  pool_size: 100                    # Max connections
  pool_timeout: 4s                  # Pool wait timeout
  dial_timeout: 5s                  # Connection dial timeout
  read_timeout: 3s                  # Read timeout
  write_timeout: 3s                 # Write timeout

rate_limit:
  requests_per_minute: 100          # Global rate limit
  burst: 10                         # Burst capacity
  per_namespace: false              # Per-namespace limits (future)

deduplication:
  ttl: 5m                           # Fingerprint TTL
  cleanup_interval: 1m              # Cleanup goroutine interval

storm_detection:
  rate_threshold: 10                # Alerts/minute for rate-based
  pattern_threshold: 5              # Similar alerts for pattern-based
  aggregation_window: 1m            # Storm aggregation window
  similarity_threshold: 0.8         # Pattern similarity (0.0-1.0)

environment:
  cache_ttl: 30s                    # Namespace label cache TTL
  configmap_namespace: "kubernaut-system"
  configmap_name: "kubernaut-environment-overrides"
  default_environment: "unknown"    # Fallback environment

priority:
  rego_policy_path: "/etc/kubernaut/policies/priority.rego"
  fallback_table:
    critical_production: "P1"
    critical_staging: "P2"
    warning_production: "P2"
    warning_staging: "P3"
    default: "P4"

logging:
  level: "info"                     # trace, debug, info, warn, error
  format: "json"                    # json, text
  output: "stdout"                  # stdout, stderr, file
  add_caller: true                  # Include file:line in logs

metrics:
  enabled: true
  listen_addr: ":9090"
  path: "/metrics"

health:
  enabled: true
  path: "/health"
  readiness_path: "/ready"

adapters:
  prometheus:
    enabled: true
    path: "/api/v1/signals/prometheus"
  kubernetes_event:
    enabled: true
    path: "/api/v1/signals/kubernetes-event"
  grafana:
    enabled: false                  # Future adapter
    path: "/api/v1/signals/grafana"
```

---

### Environment Variables

All configuration can be overridden via environment variables:

| Environment Variable | Config Path | Example | Required |
|---------------------|-------------|---------|----------|
| `GATEWAY_LISTEN_ADDR` | `server.listen_addr` | `:8080` | No |
| `REDIS_ADDR` | `redis.addr` | `redis:6379` | Yes |
| `REDIS_PASSWORD` | `redis.password` | `<secret>` | Yes (prod) |
| `REDIS_DB` | `redis.db` | `0` | No |
| `RATE_LIMIT_RPM` | `rate_limit.requests_per_minute` | `100` | No |
| `DEDUPLICATION_TTL` | `deduplication.ttl` | `5m` | No |
| `STORM_RATE_THRESHOLD` | `storm_detection.rate_threshold` | `10` | No |
| `STORM_PATTERN_THRESHOLD` | `storm_detection.pattern_threshold` | `5` | No |
| `ENVIRONMENT_CACHE_TTL` | `environment.cache_ttl` | `30s` | No |
| `LOG_LEVEL` | `logging.level` | `info` | No |
| `LOG_FORMAT` | `logging.format` | `json` | No |
| `METRICS_ENABLED` | `metrics.enabled` | `true` | No |

**Example Deployment**:
```yaml
env:
- name: REDIS_ADDR
  value: "redis:6379"
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: redis-credentials
      key: password
- name: LOG_LEVEL
  value: "info"
- name: STORM_RATE_THRESHOLD
  value: "15"  # Tuned for production
```

---

## 📦 Dependencies

### External Dependencies

| Dependency | Version | Purpose | License | Notes |
|------------|---------|---------|---------|-------|
| **go-redis/redis/v9** | v9.3.0+ | Redis client for deduplication | BSD-2-Clause | Production-grade, connection pooling |
| **go-chi/chi/v5** | v5.0.10+ | HTTP router for adapters | MIT | Lightweight, idiomatic Go |
| **sirupsen/logrus** | v1.9.3+ | Structured logging | MIT | Standard for kubernaut |
| **kubernetes/client-go** | v0.28.x | Kubernetes API client | Apache-2.0 | CRD creation, K8s API |
| **sigs.k8s.io/controller-runtime** | v0.16.x | CRD management | Apache-2.0 | Controller-runtime client |
| **open-policy-agent/opa** | v0.57.x | Rego policy engine (priority) | Apache-2.0 | Optional, fallback table if not used |
| **prometheus/client_golang** | v1.17.x | Prometheus metrics | Apache-2.0 | Standard metrics library |
| **gorilla/mux** | v1.8.1+ | HTTP middleware (fallback) | BSD-3-Clause | Alternative to chi if needed |

**Total External**: 7-8 dependencies

---

### Internal Dependencies

| Dependency | Purpose | Location | Status |
|------------|---------|----------|--------|
| **pkg/testutil** | Test helpers, mocks, Kind cluster | `/pkg/testutil/` | ✅ Existing |
| **pkg/shared/types** | Shared type definitions | `/pkg/shared/types/` | ⏸️ May need expansion |
| **api/remediation/v1** | RemediationRequest CRD | `/api/remediation/` | ✅ Existing |

**Total Internal**: 3 dependencies

---

### Dependency Security

**Vulnerability Scanning**:
```bash
# Check for known vulnerabilities
go list -json -m all | nancy sleuth

# Alternative: Use govulncheck
govulncheck ./pkg/gateway/...
```

**License Compliance**:
```bash
# Verify license compatibility
go-licenses check ./pkg/gateway/...
```

**Update Policy**:
- **Security patches**: Immediate (within 24h)
- **Minor version updates**: Monthly maintenance window
- **Major version updates**: Quarterly review with testing
- **Dependency audit**: Every 6 months

---

### Processing Pipeline

**Signal Processing Stages**:

1. **Ingestion** (via adapters):
   - Receive webhook from signal source
   - Parse and normalize signal data (adapter-specific)
   - Extract metadata (labels, annotations, timestamps)
   - Validate signal format

2. **Processing pipeline**:
   - **Deduplication**: Check if signal was seen before (Redis lookup, ~3ms)
   - **Storm detection**: Identify alert storms (rate + pattern-based, ~3ms)
   - **Classification**: Determine environment (namespace labels + ConfigMap, ~15ms)
   - **Priority assignment**: Calculate priority (Rego or fallback table, ~1ms)

3. **CRD creation**:
   - Build RemediationRequest CRD from normalized signal
   - Create CRD in Kubernetes (~30ms)
   - Record deduplication metadata in Redis (~3ms)

4. **HTTP response**:
   - 201 Created: New RemediationRequest CRD created
   - 202 Accepted: Duplicate signal (deduplication successful)
   - 400 Bad Request: Invalid signal payload
   - 500 Internal Server Error: Processing/API errors

**Performance Targets**:
- Webhook Response Time: p95 < 50ms, p99 < 100ms
- Redis Deduplication: p95 < 5ms, p99 < 10ms
- CRD Creation: p95 < 30ms, p99 < 50ms
- Throughput: >100 alerts/second
- Deduplication Rate: 40-60% (typical for production)

---

## 📡 API Examples

### Example 1: Prometheus Webhook (Success - New CRD)

**Request**:
```bash
curl -X POST http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <K8s-ServiceAccount-Token>" \
  -d '{
    "version": "4",
    "groupKey": "{}:{alertname=\"HighMemoryUsage\"}",
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "HighMemoryUsage",
        "severity": "critical",
        "namespace": "prod-payment-service",
        "pod": "payment-api-789"
      },
      "annotations": {
        "description": "Pod using 95% memory",
        "summary": "Memory usage at 95% for payment-api-789"
      },
      "startsAt": "2025-10-04T10:00:00Z"
    }]
  }'
```

**Response** (201 Created):
```json
{
  "status": "created",
  "fingerprint": "a3f8b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
  "remediation_request_name": "remediation-highmemoryusage-a3f8b2",
  "namespace": "prod-payment-service",
  "environment": "production",
  "priority": "P1",
  "duplicate": false,
  "storm_aggregation": false,
  "processing_time_ms": 42
}
```

**CRD Created** (in Kubernetes):
```yaml
apiVersion: remediation.kubernaut.io/v1
kind: RemediationRequest
metadata:
  name: remediation-highmemoryusage-a3f8b2
  namespace: prod-payment-service
  labels:
    kubernaut.io/environment: production
    kubernaut.io/priority: P1
    kubernaut.io/source: prometheus
spec:
  alertName: HighMemoryUsage
  severity: critical
  priority: P1
  environment: production
  resource:
    kind: Pod
    name: payment-api-789
    namespace: prod-payment-service
  fingerprint: a3f8b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1
  metadata:
    source: prometheus
    sourceLabels:
      alertname: HighMemoryUsage
      severity: critical
      namespace: prod-payment-service
      pod: payment-api-789
    annotations:
      description: "Pod using 95% memory"
      summary: "Memory usage at 95% for payment-api-789"
  createdAt: "2025-10-04T10:00:00Z"
```

**Verification**:
```bash
# Check CRD was created
kubectl get remediationrequest -n prod-payment-service

# Get CRD details
kubectl get remediationrequest remediation-highmemoryusage-a3f8b2 -n prod-payment-service -o yaml
```

---

### Example 2: Duplicate Signal (Deduplication)

**Request**:
```bash
# Same alert sent again within 5-minute TTL window
curl -X POST http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <K8s-ServiceAccount-Token>" \
  -d '{
    "version": "4",
    "groupKey": "{}:{alertname=\"HighMemoryUsage\"}",
    "alerts": [{
      "status": "firing",
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
  }'
```

**Response** (202 Accepted):
```json
{
  "status": "duplicate",
  "fingerprint": "a3f8b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
  "duplicate": true,
  "metadata": {
    "count": 2,
    "first_seen": "2025-10-04T10:00:00Z",
    "last_seen": "2025-10-04T10:01:30Z",
    "remediation_request_ref": "prod-payment-service/remediation-highmemoryusage-a3f8b2"
  },
  "processing_time_ms": 5
}
```

**Result**: No new CRD created, deduplication metadata updated in Redis

**Verification**:
```bash
# Check Redis deduplication entry
kubectl exec -n kubernaut-system <gateway-pod> -- \
  redis-cli -h redis -p 6379 GET "dedup:a3f8b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"

# Output:
# {"count":2,"first_seen":"2025-10-04T10:00:00Z","last_seen":"2025-10-04T10:01:30Z","remediation_request_ref":"prod-payment-service/remediation-highmemoryusage-a3f8b2"}
```

---

### Example 3: Storm Aggregation

**Request**:
```bash
# 15 similar alerts within 1 minute (storm detected)
for i in {1..15}; do
  curl -X POST http://gateway-service:8080/api/v1/signals/prometheus \
    -H "Content-Type: application/json" \
    -d "{
      \"alerts\": [{
        \"status\": \"firing\",
        \"labels\": {
          \"alertname\": \"HighCPUUsage\",
          \"namespace\": \"prod-api\",
          \"pod\": \"api-server-$i\"
        }
      }]
    }"
done
```

**Response** (202 Accepted - Storm Aggregation):
```json
{
  "status": "storm_aggregated",
  "fingerprint": "storm-highcpuusage-prod-api-abc123",
  "storm_aggregation": true,
  "storm_metadata": {
    "pattern": "HighCPUUsage in prod-api namespace",
    "alert_count": 15,
    "affected_resources": [
      "Pod/api-server-1",
      "Pod/api-server-2",
      "... (13 more)"
    ],
    "aggregation_window": "1m",
    "remediation_request_ref": "prod-api/remediation-storm-highcpuusage-abc123"
  },
  "processing_time_ms": 8
}
```

**CRD Created** (single aggregated CRD):
```yaml
apiVersion: remediation.kubernaut.io/v1
kind: RemediationRequest
metadata:
  name: remediation-storm-highcpuusage-abc123
  namespace: prod-api
  labels:
    kubernaut.io/storm: "true"
    kubernaut.io/storm-pattern: highcpuusage
spec:
  alertName: HighCPUUsage
  severity: critical
  priority: P1
  environment: production
  stormAggregation:
    pattern: "HighCPUUsage in prod-api namespace"
    alertCount: 15
    affectedResources:
      - kind: Pod
        name: api-server-1
      - kind: Pod
        name: api-server-2
      # ... (13 more)
```

---

### Example 4: Invalid Webhook (Validation Error)

**Request**:
```bash
curl -X POST http://gateway-service:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d '{
    "version": "4",
    "alerts": [{
      "status": "firing",
      "labels": {
        "namespace": "test"
      }
    }]
  }'
```

**Response** (400 Bad Request):
```json
{
  "error": "Signal validation failed: missing required field 'alertname'",
  "details": {
    "validation_errors": [
      "alertname is required",
      "severity is missing or empty"
    ]
  },
  "processing_time_ms": 1
}
```

---

### Example 5: Kubernetes Event Signal

**Request**:
```bash
curl -X POST http://gateway-service:8080/api/v1/signals/kubernetes-event \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <K8s-ServiceAccount-Token>" \
  -d '{
    "type": "Warning",
    "reason": "FailedScheduling",
    "message": "0/3 nodes are available: insufficient cpu",
    "involvedObject": {
      "kind": "Pod",
      "namespace": "prod-database",
      "name": "postgres-primary-0"
    },
    "firstTimestamp": "2025-10-04T10:00:00Z",
    "count": 5
  }'
```

**Response** (201 Created):
```json
{
  "status": "created",
  "fingerprint": "b4c3d2e1f0a9b8c7d6e5f4a3b2c1d0e9f8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3",
  "remediation_request_name": "remediation-failedscheduling-b4c3d2",
  "namespace": "prod-database",
  "environment": "production",
  "priority": "P2",
  "duplicate": false,
  "processing_time_ms": 38
}
```

---

### Example 6: Processing Error (Redis Unavailable)

**Request**:
```bash
# Redis is down
curl -X POST http://gateway-service:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -d '{ ... valid payload ... }'
```

**Response** (500 Internal Server Error):
```json
{
  "error": "Internal server error",
  "details": {
    "message": "Failed to check deduplication status",
    "retry": true,
    "retry_after": "30s"
  },
  "processing_time_ms": 5
}
```

**Gateway Logs**:
```json
{
  "level": "error",
  "msg": "Deduplication check failed",
  "fingerprint": "a3f8b2c1...",
  "error": "dial tcp 10.96.0.5:6379: connect: connection refused",
  "component": "deduplication",
  "timestamp": "2025-10-04T10:00:00Z"
}
```

---

## 🔗 Service Integration Examples

### Integration 1: Prometheus AlertManager → Gateway

**Setup AlertManager Configuration**:

```yaml
# prometheus-alertmanager-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: alertmanager-config
  namespace: prometheus
data:
  alertmanager.yml: |
    global:
      resolve_timeout: 5m

    receivers:
    - name: 'kubernaut-gateway'
      webhook_configs:
      - url: 'http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus'
        send_resolved: true
        http_config:
          bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
        max_alerts: 50  # Prevent overwhelming Gateway

    route:
      receiver: 'kubernaut-gateway'
      group_by: ['alertname', 'namespace', 'severity']
      group_wait: 10s
      group_interval: 10s
      repeat_interval: 12h
      routes:
      - match:
          severity: critical
        receiver: 'kubernaut-gateway'
        repeat_interval: 5m
      - match:
          severity: warning
        receiver: 'kubernaut-gateway'
        repeat_interval: 30m
```

**Apply Configuration**:
```bash
kubectl apply -f prometheus-alertmanager-config.yaml

# Restart AlertManager to pick up config
kubectl rollout restart deployment/alertmanager -n prometheus
```

**Flow Diagram**:
```
Prometheus → [Alert Fires] → AlertManager → [Webhook] → Gateway Service
                                                            ↓
                                                    [Process Signal]
                                                            ↓
                                                    [Create RemediationRequest CRD]
                                                            ↓
                                                    RemediationOrchestrator
```

**Testing**:
```bash
# Test AlertManager connectivity to Gateway
kubectl exec -n prometheus <alertmanager-pod> -- \
  curl -v http://gateway-service.kubernaut-system.svc.cluster.local:8080/health

# Trigger test alert
kubectl exec -n prometheus <prometheus-pod> -- \
  promtool alert test alertmanager.yml
```

---

### Integration 2: Gateway → RemediationOrchestrator

**Gateway Side (CRD Creation)**:

```go
// pkg/gateway/processing/crd_creator.go
package processing

import (
	"context"
	"fmt"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CRDCreator struct {
	client client.Client
	logger *logrus.Logger
}

func (c *CRDCreator) CreateRemediationRequest(
	ctx context.Context,
	signal *types.NormalizedSignal,
) (*remediationv1.RemediationRequest, error) {
	// BR-GATEWAY-015: Create RemediationRequest CRD
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateCRDName(signal),
			Namespace: signal.Namespace,
			Labels: map[string]string{
				"kubernaut.io/environment": signal.Environment,
				"kubernaut.io/priority":    signal.Priority,
				"kubernaut.io/source":      signal.SourceType,
			},
		},
		Spec: remediationv1.RemediationRequestSpec{
			AlertName:   signal.AlertName,
			Severity:    signal.Severity,
			Priority:    signal.Priority,
			Environment: signal.Environment,
			Resource: remediationv1.ResourceReference{
				Kind:      signal.Resource.Kind,
				Name:      signal.Resource.Name,
				Namespace: signal.Namespace,
			},
			Fingerprint: signal.Fingerprint,
			Metadata:    signal.Metadata,
		},
	}

	// BR-GATEWAY-021: Record signal metadata in CRD
	if err := c.client.Create(ctx, rr); err != nil {
		return nil, fmt.Errorf("failed to create RemediationRequest for signal %s (fingerprint=%s, namespace=%s): %w",
			signal.AlertName, signal.Fingerprint, signal.Namespace, err)
	}

	c.logger.WithFields(logrus.Fields{
		"crd_name":    rr.Name,
		"namespace":   rr.Namespace,
		"fingerprint": signal.Fingerprint,
		"priority":    signal.Priority,
	}).Info("RemediationRequest CRD created")

	return rr, nil
}
```

**RemediationOrchestrator Side (Watch CRDs)**:

```go
// pkg/remediation/orchestrator.go
package remediation

import (
	"context"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type RemediationOrchestrator struct {
	client client.Client
	logger *logrus.Logger
}

func (r *RemediationOrchestrator) Watch(ctx context.Context) error {
	// Watch for new RemediationRequest CRDs
	return r.client.Watch(
		ctx,
		&remediationv1.RemediationRequestList{},
		// Only process new CRDs (not updates)
		predicate.NewPredicateFuncs(func(obj client.Object) bool {
			rr := obj.(*remediationv1.RemediationRequest)
			return rr.Status.Phase == "" // New CRD (no status yet)
		}),
	)
}

func (r *RemediationOrchestrator) ProcessRemediation(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) error {
	r.logger.WithFields(logrus.Fields{
		"crd_name":    rr.Name,
		"namespace":   rr.Namespace,
		"priority":    rr.Spec.Priority,
		"environment": rr.Spec.Environment,
	}).Info("Processing new RemediationRequest")

	// Select workflow based on priority + environment
	workflow := r.selectWorkflow(rr.Spec.Priority, rr.Spec.Environment)

	// Execute workflow
	return r.executeWorkflow(ctx, workflow, rr)
}
```

**Flow Diagram**:
```
Gateway Service
    ↓
[Create RemediationRequest CRD]
    ↓
Kubernetes API Server
    ↓
[CRD Event: ADDED]
    ↓
RemediationOrchestrator (Watch)
    ↓
[Process Remediation]
    ↓
[Select Workflow based on Priority/Environment]
    ↓
[Execute Workflow]
```

**Testing Integration**:
```bash
# 1. Create test RemediationRequest CRD
kubectl apply -f - <<EOF
apiVersion: remediation.kubernaut.io/v1
kind: RemediationRequest
metadata:
  name: test-remediation
  namespace: default
spec:
  alertName: TestAlert
  severity: critical
  priority: P1
  environment: development
EOF

# 2. Check RemediationOrchestrator picked it up
kubectl logs -n kubernaut-system -l app=remediation-orchestrator | \
  grep "Processing new RemediationRequest"

# 3. Verify workflow was executed
kubectl get remediationrequest test-remediation -n default -o yaml
# Check status.phase is updated
```

---

### Integration 3: Network Policy Enforcement

**Network Policy**:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-ingress-policy
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway-service
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow from Prometheus AlertManager
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus
      podSelector:
        matchLabels:
          app: alertmanager
    ports:
    - protocol: TCP
      port: 8080
  # Allow Prometheus metrics scraping
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus
      podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 9090
  egress:
  # Allow DNS
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Redis
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
  # Allow Kubernetes API
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: kube-apiserver
    ports:
    - protocol: TCP
      port: 443
```

**Testing Network Policy**:
```bash
# 1. Verify Gateway can reach Redis
kubectl exec -n kubernaut-system <gateway-pod> -- \
  redis-cli -h redis -p 6379 ping
# Expected: PONG

# 2. Verify Gateway can reach K8s API
kubectl exec -n kubernaut-system <gateway-pod> -- \
  curl -k https://kubernetes.default.svc.cluster.local/api
# Expected: {"kind":"APIVersions",...}

# 3. Verify unauthorized pod CANNOT reach Gateway
kubectl run -n default test-pod --image=curlimages/curl --rm -it -- \
  curl http://gateway-service.kubernaut-system.svc.cluster.local:8080/health
# Expected: Timeout (blocked by network policy)
```

---

## 🧪 Test Strategy

### Test Pyramid Distribution

Following Kubernaut's defense-in-depth testing strategy (`.cursor/rules/03-testing-strategy.mdc`):

- **Unit Tests (70%+)**: HTTP handlers, adapters, deduplication logic, storm detection (estimated: 75 tests)
  - **Coverage**: AT LEAST 70% of total business requirements
  - **Confidence**: 85-90%
  - **Mock Strategy**: Mock ONLY external dependencies (Redis, K8s API). Use REAL business logic.

- **Integration Tests (>50%)**: Redis integration, CRD creation, end-to-end webhook flow (estimated: 30 tests)
  - **Coverage**: >50% of total business requirements (microservices architecture)
  - **Confidence**: 80-85%
  - **Mock Strategy**: Use REAL services (Redis in Kind, K8s API in Kind cluster). No mocking.

- **E2E Tests (10-15%)**: Prometheus → Gateway → RemediationRequest → Completion (estimated: 5 tests)
  - **Coverage**: 10-15% of total business requirements for critical user journeys
  - **Confidence**: 90-95%
  - **Mock Strategy**: Minimal mocking. Real components and workflows.

**Total Estimated**: 110 tests covering ~135-140% of BRs (defense-in-depth overlapping coverage)

---

### Unit Test Breakdown (Estimated: 75 tests)

| Module | Tests | BR Coverage | Status |
|--------|-------|-------------|--------|
| **prometheus_adapter_test.go** | 12 | BR-GATEWAY-001, 003 | ⏸️ 0/12 |
| **kubernetes_adapter_test.go** | 10 | BR-GATEWAY-002, 004 | ⏸️ 0/10 |
| **deduplication_test.go** | 15 | BR-GATEWAY-005, 006, 010 | ⏸️ 0/15 |
| **storm_detection_test.go** | 8 | BR-GATEWAY-007, 008 | ⏸️ 0/8 |
| **classification_test.go** | 10 | BR-GATEWAY-051, 052, 053 | ⏸️ 0/10 |
| **priority_test.go** | 8 | BR-GATEWAY-013, 014 | ⏸️ 0/8 |
| **handlers_test.go** | 12 | BR-GATEWAY-017 to 020 | ⏸️ 0/12 |

**Status**: 0/75 unit tests (0%)

---

### Integration Test Breakdown (Estimated: 30 tests)

| Module | Tests | BR Coverage | Status |
|--------|-------|-------------|--------|
| **redis_integration_test.go** | 10 | BR-GATEWAY-005, 010 | ⏸️ 0/10 |
| **crd_creation_test.go** | 8 | BR-GATEWAY-015, 021 | ⏸️ 0/8 |
| **webhook_flow_test.go** | 12 | BR-GATEWAY-001, 002, 015 | ⏸️ 0/12 |

**Status**: 0/30 integration tests (0%)

---

### E2E Test Breakdown (Estimated: 5 tests)

| Module | Tests | BR Coverage | Status |
|--------|-------|-------------|--------|
| **prometheus_to_remediation_test.go** | 5 | BR-GATEWAY-001, 015, 071 | ⏸️ 0/5 |

**Status**: 0/5 E2E tests (0%)

---

### Defense-in-Depth Testing Strategy

**Principle**: Test with **REAL business logic**, mock **ONLY external dependencies**

Following Kubernaut's defense-in-depth approach (`.cursor/rules/03-testing-strategy.mdc`):

| Test Tier | Coverage | What to Test | Mock Strategy |
|-----------|----------|--------------|---------------|
| **Unit Tests** | **70%+** (AT LEAST 70% of ALL BRs) | Business logic, algorithms, HTTP handlers | Mock: Redis, K8s API<br>Real: Adapters, Processing, Handlers |
| **Integration Tests** | **>50%** (due to microservices) | Component interactions, Redis + K8s, CRD coordination | Mock: NONE<br>Real: Redis (in Kind), K8s API (Kind cluster) |
| **E2E Tests** | **10-15%** (critical user journeys) | Complete workflows, multi-service | Mock: NONE<br>Real: All components |

**Key Principle**: **NEVER mock business logic**
- ✅ **REAL**: Adapters, deduplication logic, storm detection, classification, priority engine
- ❌ **MOCK**: Redis (unit tests), Kubernetes API (unit tests), external services only

**Why Defense-in-Depth?**
- **Unit tests** (70%+) validate individual components work correctly with mocked external dependencies
- **Integration tests** (>50%) validate components work together with REAL services (Redis + K8s in Kind)
- **E2E tests** (10-15%) validate complete business workflows across all services
- Each layer catches different types of bugs (unit: business logic, integration: coordination, e2e: workflows)

**Why Percentages Add Up to >100%** (135-140% total):
- **Defense-in-Depth** = Overlapping coverage by design
- Same business requirement tested at multiple levels for different validation purposes:
  - **Unit level**: Business logic correctness (fast, isolated)
  - **Integration level**: Service coordination (real dependencies)
  - **E2E level**: Complete workflow (production-like)
- Example: BR-GATEWAY-001 (Prometheus webhook) tested in:
  - Unit tests: Adapter parsing logic (12 tests)
  - Integration tests: Webhook → CRD flow (5 tests)
  - E2E tests: AlertManager → Gateway → Orchestrator (2 tests)

---

### Mock Strategy

**Unit Tests (70%+)**:
- **MOCK**: Redis (miniredis), Kubernetes API (fake K8s client), Rego engine
- **REAL**: All business logic (adapters, processing pipeline, handlers)

**Integration Tests (<20%)**:
- **MOCK**: NONE - Use real Redis in Kind cluster
- **REAL**: Redis, Kubernetes API (Kind cluster), CRD creation, RBAC

**E2E Tests (<10%)**:
- **MOCK**: NONE
- **REAL**: All components, actual Prometheus AlertManager webhooks

---

## 🧪 Example Tests

### Example Unit Test: Prometheus Adapter (BR-GATEWAY-001)

**File**: `test/unit/gateway/prometheus_adapter_test.go`

```go
package gateway

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

func TestPrometheusAdapter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Prometheus Adapter Suite - BR-GATEWAY-001")
}

var _ = Describe("BR-GATEWAY-001: Prometheus AlertManager Webhook Parsing", func() {
	var (
		adapter *adapters.PrometheusAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
		ctx = context.Background()
	})

	Context("when receiving valid Prometheus webhook", func() {
		It("should parse AlertManager webhook format correctly", func() {
			// BR-GATEWAY-001: Accept signals from Prometheus AlertManager
			payload := []byte(`{
				"version": "4",
				"groupKey": "{}:{alertname=\"HighMemoryUsage\"}",
				"alerts": [{
					"status": "firing",
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
			}`)

			signal, err := adapter.Parse(ctx, payload)

			// Assertions - validate business outcome
			Expect(err).ToNot(HaveOccurred())
			Expect(signal).ToNot(BeNil())

			// BR-GATEWAY-003: Normalize Prometheus format
			Expect(signal.AlertName).To(Equal("HighMemoryUsage"))
			Expect(signal.Severity).To(Equal("critical"))
			Expect(signal.Namespace).To(Equal("prod-payment-service"))
			Expect(signal.Resource.Kind).To(Equal("Pod"))
			Expect(signal.Resource.Name).To(Equal("payment-api-789"))
			Expect(signal.SourceType).To(Equal("prometheus"))

			// BR-GATEWAY-006: Generate fingerprint
			Expect(signal.Fingerprint).ToNot(BeEmpty())
			Expect(signal.Fingerprint).To(HaveLen(64)) // SHA256 hex
		})

		It("should extract resource identifiers correctly", func() {
			// Test different resource types (Deployment, StatefulSet, Node)
			testCases := []struct {
				labels       map[string]string
				expectedKind string
				expectedName string
			}{
				{
					labels:       map[string]string{"deployment": "api-server"},
					expectedKind: "Deployment",
					expectedName: "api-server",
				},
				{
					labels:       map[string]string{"statefulset": "database"},
					expectedKind: "StatefulSet",
					expectedName: "database",
				},
				{
					labels:       map[string]string{"node": "worker-01"},
					expectedKind: "Node",
					expectedName: "worker-01",
				},
			}

			for _, tc := range testCases {
				payload := createPrometheusPayload("TestAlert", tc.labels)
				signal, err := adapter.Parse(ctx, payload)

				Expect(err).ToNot(HaveOccurred())
				Expect(signal.Resource.Kind).To(Equal(tc.expectedKind))
				Expect(signal.Resource.Name).To(Equal(tc.expectedName))
			}
		})
	})

	Context("BR-GATEWAY-002: when receiving invalid webhook", func() {
		It("should reject malformed JSON with clear error", func() {
			// Error handling: Invalid JSON format
			invalidPayload := []byte(`{invalid json}`)

			signal, err := adapter.Parse(ctx, invalidPayload)

			// BR-GATEWAY-019: Return clear error for invalid format
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse"))
			Expect(signal).To(BeNil())
		})

		It("should reject webhook missing required fields", func() {
			// Error handling: Missing required fields
			payloadMissingAlertname := []byte(`{
				"version": "4",
				"alerts": [{
					"status": "firing",
					"labels": {
						"namespace": "test"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payloadMissingAlertname)
			Expect(err).ToNot(HaveOccurred()) // Parse succeeds

			// But validation should fail
			err = adapter.Validate(signal)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing alertname"))
		})
	})

	Context("BR-GATEWAY-006: fingerprint generation", func() {
		It("should generate consistent fingerprints for same alert", func() {
			payload := createPrometheusPayload("TestAlert", map[string]string{
				"namespace": "prod",
				"pod":       "api-123",
			})

			signal1, _ := adapter.Parse(ctx, payload)
			signal2, _ := adapter.Parse(ctx, payload)

			// Fingerprints must be identical for deduplication
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint))
		})

		It("should generate different fingerprints for different alerts", func() {
			payload1 := createPrometheusPayload("Alert1", map[string]string{"pod": "api-123"})
			payload2 := createPrometheusPayload("Alert2", map[string]string{"pod": "api-456"})

			signal1, _ := adapter.Parse(ctx, payload1)
			signal2, _ := adapter.Parse(ctx, payload2)

			Expect(signal1.Fingerprint).ToNot(Equal(signal2.Fingerprint))
		})
	})
})

// Helper function to create test payloads
func createPrometheusPayload(alertName string, labels map[string]string) []byte {
	labels["alertname"] = alertName
	// ... JSON marshaling logic
	return []byte(`{...}`)
}
```

**Test Count**: 12 tests (BR-GATEWAY-001, 002, 003, 006)
**Coverage**: Prometheus adapter parsing, validation, error handling

---

### Example Unit Test: Deduplication Service (BR-GATEWAY-005)

**File**: `test/unit/gateway/deduplication_test.go`

```go
package gateway

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/alicebob/miniredis/v2"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

var _ = Describe("BR-GATEWAY-005: Signal Deduplication", func() {
	var (
		deduplicator *processing.DeduplicationService
		miniRedis    *miniredis.Miniredis
		ctx          context.Context
		testSignal   *types.NormalizedSignal
	)

	BeforeEach(func() {
		var err error
		// Use miniredis for fast, predictable unit tests
		miniRedis, err = miniredis.Run()
		Expect(err).ToNot(HaveOccurred())

		// Create deduplication service with short TTL for testing
		redisClient := createRedisClient(miniRedis.Addr())
		deduplicator = processing.NewDeduplicationServiceWithTTL(
			redisClient,
			5*time.Second, // Short TTL for tests
			testLogger,
		)

		ctx = context.Background()
		testSignal = &types.NormalizedSignal{
			Fingerprint: "test-fingerprint-123",
			AlertName:   "HighMemoryUsage",
			Namespace:   "prod",
		}
	})

	AfterEach(func() {
		miniRedis.Close()
	})

	Context("BR-GATEWAY-005: first occurrence of signal", func() {
		It("should NOT be a duplicate", func() {
			// First time seeing this signal
			isDuplicate, metadata, err := deduplicator.Check(ctx, testSignal)

			Expect(err).ToNot(HaveOccurred())
			Expect(isDuplicate).To(BeFalse())
			Expect(metadata).To(BeNil())
		})
	})

	Context("BR-GATEWAY-010: duplicate signal within TTL window", func() {
		It("should detect duplicate and return metadata", func() {
			// Store signal first time
			err := deduplicator.Store(ctx, testSignal, "remediation-req-123")
			Expect(err).ToNot(HaveOccurred())

			// Check again - should be duplicate
			isDuplicate, metadata, err := deduplicator.Check(ctx, testSignal)

			Expect(err).ToNot(HaveOccurred())
			Expect(isDuplicate).To(BeTrue())
			Expect(metadata).ToNot(BeNil())
			Expect(metadata.RemediationRequestRef).To(Equal("remediation-req-123"))
			Expect(metadata.Count).To(Equal(2)) // Second occurrence
		})

		It("should increment count on repeated duplicates", func() {
			// Store initial signal
			deduplicator.Store(ctx, testSignal, "remediation-req-123")

			// Check 3 more times
			for i := 2; i <= 4; i++ {
				isDuplicate, metadata, err := deduplicator.Check(ctx, testSignal)
				Expect(err).ToNot(HaveOccurred())
				Expect(isDuplicate).To(BeTrue())
				Expect(metadata.Count).To(Equal(i))
			}
		})
	})

	Context("when TTL expires", func() {
		It("should treat expired signal as new (not duplicate)", func() {
			// Store signal
			err := deduplicator.Store(ctx, testSignal, "remediation-req-123")
			Expect(err).ToNot(HaveOccurred())

			// Fast-forward Redis time past TTL (5 seconds)
			miniRedis.FastForward(6 * time.Second)

			// Check again - should NOT be duplicate (TTL expired)
			isDuplicate, _, err := deduplicator.Check(ctx, testSignal)
			Expect(err).ToNot(HaveOccurred())
			Expect(isDuplicate).To(BeFalse())
		})
	})

	Context("BR-GATEWAY-020: error handling when Redis unavailable", func() {
		It("should return error with context when Redis is down", func() {
			// Close Redis to simulate failure
			miniRedis.Close()

			isDuplicate, metadata, err := deduplicator.Check(ctx, testSignal)

			// Error handling: Return clear error, don't panic
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Redis"))
			Expect(isDuplicate).To(BeFalse())
			Expect(metadata).To(BeNil())
		})
	})
})
```

**Test Count**: 15 tests (BR-GATEWAY-005, 010, 020)
**Coverage**: Deduplication logic, TTL expiry, error handling

---

### Example Integration Test: End-to-End Webhook Flow (BR-GATEWAY-001, BR-GATEWAY-015)

**File**: `test/integration/gateway/webhook_flow_test.go`

```go
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
)

var _ = Describe("BR-GATEWAY-001 + BR-GATEWAY-015: Prometheus Webhook → CRD Creation", func() {
	var (
		gatewayServer *gateway.Server
		k8sClient     client.Client
		kindCluster   *kind.TestCluster
		ctx           context.Context
	)

	BeforeEach(func() {
		var err error
		ctx = context.Background()

		// Setup Kind cluster with CRDs + Redis
		kindCluster, err = kind.NewTestCluster(&kind.Config{
			Name: "gateway-integration-test",
			CRDs: []string{
				"config/crd/bases/remediation.kubernaut.io_remediationrequests.yaml",
			},
		})
		Expect(err).ToNot(HaveOccurred())

		k8sClient = kindCluster.GetClient()

		// Start Gateway server with real Redis in Kind
		gatewayConfig := &gateway.ServerConfig{
			ListenAddr:             ":8080",
			Redis:                  kindCluster.GetRedisConfig(),
			DeduplicationTTL:       5 * time.Second,
			StormRateThreshold:     10,
			StormPatternThreshold:  5,
		}

		gatewayServer, err = gateway.NewServer(gatewayConfig, testLogger)
		Expect(err).ToNot(HaveOccurred())

		// Register Prometheus adapter
		prometheusAdapter := adapters.NewPrometheusAdapter()
		err = gatewayServer.RegisterAdapter(prometheusAdapter)
		Expect(err).ToNot(HaveOccurred())

		// Start server in background
		go gatewayServer.Start(ctx)

		// Wait for server to be ready
		Eventually(func() error {
			resp, err := http.Get("http://localhost:8080/ready")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server not ready: %d", resp.StatusCode)
			}
			return nil
		}, "10s", "100ms").Should(Succeed())
	})

	AfterEach(func() {
		gatewayServer.Stop(ctx)
		kindCluster.Cleanup()
	})

	Context("BR-GATEWAY-001: receiving Prometheus webhook", func() {
		It("should create RemediationRequest CRD successfully", func() {
			// BR-GATEWAY-001: Accept Prometheus AlertManager webhook
			webhookPayload := map[string]interface{}{
				"version":  "4",
				"groupKey": "{}:{alertname=\"HighMemoryUsage\"}",
				"alerts": []map[string]interface{}{
					{
						"status": "firing",
						"labels": map[string]string{
							"alertname": "HighMemoryUsage",
							"severity":  "critical",
							"namespace": "prod-payment-service",
							"pod":       "payment-api-789",
						},
						"annotations": map[string]string{
							"description": "Pod using 95% memory",
						},
						"startsAt": "2025-10-04T10:00:00Z",
					},
				},
			}

			payloadBytes, _ := json.Marshal(webhookPayload)

			// Send webhook to Gateway
			resp, err := http.Post(
				"http://localhost:8080/api/v1/signals/prometheus",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BR-GATEWAY-017: Should return HTTP 201 Created
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			// Parse response
			var response gateway.ProcessingResponse
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			Expect(response.Status).To(Equal("created"))
			Expect(response.Fingerprint).ToNot(BeEmpty())
			Expect(response.RemediationRequestName).ToNot(BeEmpty())
			Expect(response.Environment).ToNot(BeEmpty())
			Expect(response.Priority).ToNot(BeEmpty())

			// BR-GATEWAY-015: Verify CRD was created in Kubernetes
			Eventually(func() error {
				rr := &remediationv1.RemediationRequest{}
				key := client.ObjectKey{
					Name:      response.RemediationRequestName,
					Namespace: "prod-payment-service",
				}
				return k8sClient.Get(ctx, key, rr)
			}, "5s", "100ms").Should(Succeed())

			// Verify CRD contents
			rr := &remediationv1.RemediationRequest{}
			key := client.ObjectKey{
				Name:      response.RemediationRequestName,
				Namespace: "prod-payment-service",
			}
			err = k8sClient.Get(ctx, key, rr)
			Expect(err).ToNot(HaveOccurred())

			// BR-GATEWAY-021: Verify signal metadata in CRD
			Expect(rr.Spec.AlertName).To(Equal("HighMemoryUsage"))
			Expect(rr.Spec.Severity).To(Equal("critical"))
			Expect(rr.Spec.Priority).To(Equal(response.Priority))
			Expect(rr.Spec.Environment).To(Equal(response.Environment))
			Expect(rr.Spec.Resource.Kind).To(Equal("Pod"))
			Expect(rr.Spec.Resource.Name).To(Equal("payment-api-789"))
		})
	})

	Context("BR-GATEWAY-010: duplicate signal handling", func() {
		It("should return HTTP 202 for duplicate without creating new CRD", func() {
			payload := createTestPayload("DuplicateAlert")

			// Send first time - should create CRD
			resp1, _ := sendWebhook(payload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

			var response1 gateway.ProcessingResponse
			json.NewDecoder(resp1.Body).Decode(&response1)
			firstCRDName := response1.RemediationRequestName

			// Send again immediately - should be deduplicated
			resp2, _ := sendWebhook(payload)
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted)) // 202

			var response2 gateway.ProcessingResponse
			json.NewDecoder(resp2.Body).Decode(&response2)

			// BR-GATEWAY-018: Should return duplicate status
			Expect(response2.Status).To(Equal("duplicate"))
			Expect(response2.Duplicate).To(BeTrue())
			Expect(response2.Metadata.Count).To(Equal(2))
			Expect(response2.Metadata.RemediationRequestRef).To(ContainSubstring(firstCRDName))

			// Verify NO new CRD was created
			rrList := &remediationv1.RemediationRequestList{}
			err := k8sClient.List(ctx, rrList, client.InNamespace("test"))
			Expect(err).ToNot(HaveOccurred())
			Expect(rrList.Items).To(HaveLen(1)) // Still only 1 CRD
		})
	})
})
```

**Test Count**: 12 tests (BR-GATEWAY-001, 010, 015, 017, 018, 021)
**Coverage**: Complete webhook flow, CRD creation, deduplication, error responses

---

## ⚠️ Error Handling Patterns

### Consistent Error Handling Strategy

Following Notification service pattern for rich error context:

```go
// ✅ CORRECT: Error with context (resource name, namespace, operation)
if err := s.deduplicator.Check(ctx, signal); err != nil {
	return nil, fmt.Errorf("deduplication check failed for signal %s (fingerprint=%s, source=%s, namespace=%s): %w",
		signal.AlertName, signal.Fingerprint, signal.SourceType, signal.Namespace, err)
}

// ❌ WRONG: Generic error without context
if err := s.deduplicator.Check(ctx, signal); err != nil {
	return nil, fmt.Errorf("deduplication check failed: %w", err)
}
```

---

### Error Types by HTTP Status Code

| HTTP Status | Condition | Error Type | Retry? |
|-------------|-----------|------------|--------|
| **201 Created** | CRD created successfully | N/A | N/A |
| **202 Accepted** | Duplicate signal or storm aggregation | N/A | No |
| **400 Bad Request** | Invalid signal format, missing fields | Validation error | No (permanent error) |
| **413 Payload Too Large** | Signal payload > 1MB | Size error | No (reduce payload) |
| **429 Too Many Requests** | Rate limit exceeded | Rate limit error | Yes (with backoff) |
| **500 Internal Server Error** | Redis failure, K8s API failure | Transient error | Yes (Alertmanager retry) |
| **503 Service Unavailable** | Gateway not ready (dependencies down) | Unavailability error | Yes (wait for ready) |

---

### Error Handling Examples

#### 1. Validation Errors (400 Bad Request)

```go
// Validate signal format
if err := adapter.Validate(signal); err != nil {
	s.logger.WithFields(logrus.Fields{
		"adapter":     adapter.Name(),
		"fingerprint": signal.Fingerprint,
		"error":       err,
	}).Warn("Signal validation failed")

	http.Error(w, fmt.Sprintf("Signal validation failed: %v", err), http.StatusBadRequest)
	return
}
```

#### 2. Transient Errors (500 Internal Server Error)

```go
// Handle Redis failures gracefully
isDuplicate, metadata, err := s.deduplicator.Check(ctx, signal)
if err != nil {
	s.logger.WithFields(logrus.Fields{
		"fingerprint": signal.Fingerprint,
		"error":       err,
	}).Error("Deduplication check failed")

	// Return 500 so Alertmanager retries
	http.Error(w, "Internal server error", http.StatusInternalServerError)
	return
}
```

#### 3. Non-Critical Errors (Log and Continue)

```go
// Storm detection failure is non-critical
isStorm, stormMetadata, err := s.stormDetector.Check(ctx, signal)
if err != nil {
	// Log warning but continue processing
	s.logger.WithFields(logrus.Fields{
		"fingerprint": signal.Fingerprint,
		"error":       err,
	}).Warn("Storm detection failed - continuing without storm metadata")
	// Continue to next step...
}
```

#### 4. Defensive Programming (Nil Checks)

```go
// Following Notification service pattern
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	// Defensive: Check for nil signal
	if signal == nil {
		return nil, fmt.Errorf("signal cannot be nil")
	}

	// Defensive: Check for empty fingerprint
	if signal.Fingerprint == "" {
		return nil, fmt.Errorf("signal fingerprint cannot be empty (alertName=%s, namespace=%s)",
			signal.AlertName, signal.Namespace)
	}

	// ... process signal
}
```

---

### Error Metrics

Record errors for monitoring:

```go
// Record error metrics by type
metrics.HTTPRequestErrors.WithLabelValues(
	route,            // "/api/v1/signals/prometheus"
	"parse_error",    // error_type
	"400",            // status_code
).Inc()

metrics.ProcessingErrors.WithLabelValues(
	"deduplication",  // component
	"redis_timeout",  // error_reason
).Inc()
```

**Prometheus Queries**:
```promql
# Error rate by endpoint
rate(gateway_http_errors_total{route="/api/v1/signals/prometheus"}[5m])

# Error rate by type
sum(rate(gateway_processing_errors_total[5m])) by (component, error_reason)
```

---

## 🚀 Deployment Guide

### Production Deployment Checklist

- [ ] All core tests passing (0/110) ⏸️
- [ ] Zero critical lint errors ⏸️
- [ ] Network policies documented ✅
- [ ] K8s ServiceAccount configured ⏸️
- [ ] Health/readiness probes working ⏸️
- [ ] Prometheus metrics exposed ⏸️
- [ ] Configuration externalized ✅
- [ ] Design decisions documented ✅ (DD-GATEWAY-001)
- [ ] Architecture aligned with design ✅

**Status**: ⏸️ **NOT PRODUCTION READY** (Implementation Pending)

---

### Kubernetes Deployment

**Deployment Manifest**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-service
  namespace: kubernaut-system
  labels:
    app: gateway-service
    version: v1.0.0
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-service
  template:
    metadata:
      labels:
        app: gateway-service
    spec:
      serviceAccountName: gateway-sa
      containers:
      - name: gateway
        image: gateway-service:1.0.0
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        env:
        - name: REDIS_ENDPOINT
          value: "redis:6379"
        - name: REDIS_PASSWORD
          valueFrom:
            secretKeyRef:
              name: redis-credentials
              key: password
        - name: REDIS_DB
          value: "0"
        - name: RATE_LIMIT_RPM
          value: "100"
        - name: DEDUPLICATION_TTL
          value: "5m"
        - name: STORM_RATE_THRESHOLD
          value: "10"
        - name: STORM_PATTERN_THRESHOLD
          value: "5"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: gateway-service
  namespace: kubernaut-system
spec:
  selector:
    app: gateway-service
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: gateway-sa
  namespace: kubernaut-system
```

---

### Network Policy

**Restrict access to authorized sources only**:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-ingress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway-service
  policyTypes:
  - Ingress
  ingress:
  # Allow from Prometheus AlertManager
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus
    ports:
    - protocol: TCP
      port: 8080
  # Allow from Kubernetes API (for Event watching)
  - from:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 8080
  # Allow Prometheus scraping
  - from:
    - namespaceSelector:
        matchLabels:
          name: prometheus
    ports:
    - protocol: TCP
      port: 9090
```

---

## 📈 Success Metrics

### Technical Metrics

| Metric | Target | How to Measure | Status |
|--------|--------|---------------|--------|
| **Test Coverage** | 70%+ | `go test -cover ./pkg/gateway/...` | ⏸️ 0% |
| **Unit Tests Passing** | 100% | `go test ./test/unit/gateway/...` | ⏸️ 0/75 |
| **Integration Tests Passing** | 100% | `go test ./test/integration/gateway/...` | ⏸️ 0/30 |
| **E2E Tests Passing** | 100% | `go test ./test/e2e/gateway/...` | ⏸️ 0/5 |
| **Build Success** | 100% | CI/CD pipeline | ⏸️ N/A |
| **Lint Compliance** | 100% | `golangci-lint run ./pkg/gateway/...` | ⏸️ N/A |
| **Technical Debt** | Zero | Code review + automated checks | ⏸️ N/A |

---

### Business Metrics (Production)

| Metric | Target | Prometheus Query | Status |
|--------|--------|------------------|--------|
| **Webhook Response Time (p95)** | < 50ms | `histogram_quantile(0.95, gateway_http_duration_seconds_bucket{endpoint="/api/v1/signals/prometheus"})` | ⏸️ N/A |
| **Webhook Response Time (p99)** | < 100ms | `histogram_quantile(0.99, gateway_http_duration_seconds_bucket{endpoint="/api/v1/signals/prometheus"})` | ⏸️ N/A |
| **Redis Deduplication (p95)** | < 5ms | `histogram_quantile(0.95, gateway_deduplication_duration_seconds_bucket)` | ⏸️ N/A |
| **CRD Creation (p95)** | < 30ms | `histogram_quantile(0.95, gateway_crd_creation_duration_seconds_bucket)` | ⏸️ N/A |
| **Throughput** | >100/sec | `rate(gateway_signals_received_total[5m])` | ⏸️ N/A |
| **Deduplication Rate** | 40-60% | `rate(gateway_signals_deduplicated_total[5m]) / rate(gateway_signals_received_total[5m])` | ⏸️ N/A |
| **Success Rate** | > 95% | `rate(gateway_signals_accepted_total[5m]) / rate(gateway_signals_received_total[5m])` | ⏸️ N/A |
| **Service Availability** | > 99% | `up{job="gateway-service"}` | ⏸️ N/A |

---

## 🔮 Future Evolution Path

### v1.0 (Current): Adapter-Specific Endpoints ✅

**Features**:
- Adapter-specific routes (`/api/v1/signals/prometheus`, etc.)
- Redis-based deduplication (5-minute TTL)
- Hybrid storm detection (rate + pattern-based)
- ConfigMap-based environment classification
- Rego-based priority assignment
- Configuration-driven adapter registration

**Status**: ✅ DESIGN COMPLETE / ⏸️ IMPLEMENTATION PENDING
**Confidence**: 92% (Very High)

---

### v1.5: Optimization (If Needed)

**Add only if metrics show need**:
- Redis Sentinel for HA (if single-point-of-failure detected)
- Prometheus metrics refinement (if monitoring gaps found)
- Enhanced storm aggregation (if >50% storm rate detected)
- Rate limit per-namespace (if per-IP insufficient)

**Trigger**: Performance metrics below SLA
**Estimated**: 2-3 weeks if needed

---

### v2.0: Additional Signal Sources (If Needed)

**Add only if business requirements expand**:
- Grafana alert ingestion adapter
- Cloud-specific alerts (CloudWatch, Azure Monitor)
- Datadog integration
- PagerDuty webhook support

**Trigger**: Business requirement for additional signal sources
**Estimated**: 4-6 weeks if needed
**Note**: Requires DD-GATEWAY-002 design decision

---

## 📚 Related Documentation

**Design Decisions**:
- [DD-GATEWAY-001](../../architecture/decisions/DD-GATEWAY-001-Adapter-Specific-Endpoints.md) - **Current architecture** (adapter-specific endpoints)
- [DESIGN_B_IMPLEMENTATION_SUMMARY.md](DESIGN_B_IMPLEMENTATION_SUMMARY.md) - Architecture rationale

**Technical Documentation**:
- [README.md](README.md) - Service overview and navigation
- [overview.md](overview.md) - High-level architecture
- [implementation.md](implementation.md) - Implementation details (1,300+ lines)
- [deduplication.md](deduplication.md) - Redis fingerprinting and storm detection
- [crd-integration.md](crd-integration.md) - RemediationRequest CRD creation

**Security & Observability**:
- [security-configuration.md](security-configuration.md) - JWT authentication and RBAC
- [observability-logging.md](observability-logging.md) - Structured logging and tracing
- [metrics-slos.md](metrics-slos.md) - Prometheus metrics and Grafana dashboards

**Testing**:
- [testing-strategy.md](testing-strategy.md) - APDC-TDD patterns and mock strategies
- [implementation-checklist.md](implementation-checklist.md) - APDC phases and tasks

**Triage Reports**:
- [GATEWAY_IMPLEMENTATION_TRIAGE.md](GATEWAY_IMPLEMENTATION_TRIAGE.md) - Documentation triage (vs HolmesGPT v3.0)
- [GATEWAY_CODE_IMPLEMENTATION_TRIAGE.md](GATEWAY_CODE_IMPLEMENTATION_TRIAGE.md) - Code pattern comparison (vs Context API, Notification)
- [GATEWAY_TRIAGE_SUMMARY.md](GATEWAY_TRIAGE_SUMMARY.md) - Executive summary

**Superseded Designs** (historical reference):
- [ADAPTER_REGISTRY_DESIGN.md](ADAPTER_REGISTRY_DESIGN.md) - ⚠️ Detection-based architecture (Design A, superseded)
- [ADAPTER_DETECTION_FLOW.md](ADAPTER_DETECTION_FLOW.md) - ⚠️ Detection flow logic (superseded)

---

## ✅ Approval & Next Steps

**Design Approved**: October 4, 2025
**Design Decision**: DD-GATEWAY-001
**Implementation Status**: ⏸️ NOT STARTED
**Production Readiness**: ⏸️ NOT READY (implementation pending)
**Confidence**: 85%

**Critical Next Steps**:
1. ⏸️ Enumerate all business requirements (BR-GATEWAY-001 to 040)
2. ⏸️ Create DD-GATEWAY-001 design decision document
3. ⏸️ Implement unit tests (75 tests, 20-25h)
4. ⏸️ Implement integration tests (30 tests, 15-20h)
5. ⏸️ Implement E2E tests (5 tests, 5-10h)
6. ⏸️ Deploy to development environment
7. ⏸️ Integrate with RemediationOrchestrator
8. ⏸️ Deploy to production with network policies

**Estimated Time to Production**: 48-63 hours (6-8 days) + 8h deployment = 56-71 hours total

---

## 🎯 Implementation Priorities

### Phase 1: Foundation (Week 1)

**Priority**: 🔴 P0 - Critical

**Tasks**:
1. Enumerate all BRs in `GATEWAY_BUSINESS_REQUIREMENTS.md` (6-8h)
2. Create DD-GATEWAY-001 design decision document (3-4h)
3. Setup test structure (suite_test.go) with test count tracking (2h)
4. Implement Prometheus adapter (8-10h)
5. Implement Kubernetes Events adapter (8-10h)

**Deliverable**: 2 adapters implemented with unit tests
**Total Effort**: 27-34 hours

---

### Phase 2: Core Processing (Week 2)

**Priority**: 🔴 P0 - Critical

**Tasks**:
6. Implement deduplication service (10-12h)
7. Implement storm detection (8-10h)
8. Implement environment classification (6-8h)
9. Implement priority engine (6-8h)
10. Implement CRD creator (8-10h)

**Deliverable**: Complete processing pipeline with unit tests
**Total Effort**: 38-48 hours

---

### Phase 3: Integration & Testing (Week 3)

**Priority**: 🟡 P1 - Important

**Tasks**:
11. Integration tests (Redis + K8s) (15-20h)
12. E2E tests (Prometheus → CRD) (5-10h)
13. Performance testing and optimization (8-10h)
14. Security hardening and audit (4-6h)

**Deliverable**: Production-ready service with complete test coverage
**Total Effort**: 32-46 hours

---

### Phase 4: Deployment (Week 4)

**Priority**: 🟡 P1 - Important

**Tasks**:
15. Create deployment manifests (4h)
16. Setup monitoring and alerts (4h)
17. Deploy to development (2h)
18. Integration testing with other services (4h)
19. Deploy to production (2h)
20. Validation and monitoring (2h)

**Deliverable**: Production deployment with monitoring
**Total Effort**: 18 hours

---

**Grand Total**: 115-146 hours (14.5-18 days)

---

## 📊 Risk Assessment

### Technical Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| **Redis connection failures** | Medium | High | Implement circuit breaker, retry logic |
| **Storm detection false positives** | Medium | Medium | Tunable thresholds via ConfigMap |
| **High latency on CRD creation** | Low | Medium | Performance testing, optimize K8s API calls |
| **Adapter complexity growth** | Low | Low | Configuration-driven registration, clean interfaces |

### Business Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| **Missed signals during downtime** | Medium | High | HA deployment (2+ replicas), health monitoring |
| **Deduplication accuracy issues** | Low | Medium | Comprehensive unit tests, integration tests with real Redis |
| **False storm aggregation** | Low | High | Tunable thresholds, admin override capability |

---

**Document Status**: ✅ Complete (Enhanced with comprehensive examples and references)
**Plan Version**: v1.0.1
**Last Updated**: October 21, 2025
**Supersedes**: Design documents (consolidated into single plan)
**Next Review**: After Phase 1 completion (enumerate BRs)

**v1.0.1 Enhancements** (Oct 21, 2025):
- ✅ Complete configuration reference with all options + environment variables
- ✅ Dependencies list (external: 8 packages, internal: 3 packages)
- ✅ API Examples (6 scenarios: success, duplicate, storm, error, K8s events, Redis failure)
- ✅ Service Integration examples (Prometheus, RemediationOrchestrator, Network Policy)
- ✅ Defense-in-depth testing strategy (70%+/>50%/10-15% per `.cursor/rules/03-testing-strategy.mdc`)
- ✅ Unit/integration test examples (3 complete examples with 39 tests)
- ✅ Error handling patterns (HTTP status codes + 4 examples)

