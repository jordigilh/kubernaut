# DD-TOOLSET-001: REST API Deprecation and Configuration CRD Migration

**Date**: 2025-11-10
**Status**: ✅ Approved
**Confidence**: 98%
**Decision Type**: Architecture & API Design

---

## Context

The Dynamic Toolset Service currently exposes 9 REST API endpoints:
- 6 endpoints for toolset operations (`/api/v1/discover`, `/api/v1/toolsets/*`, `/api/v1/services`)
- 3 standard endpoints (`/health`, `/ready`, `/metrics`)

### Current State

- **Service Name**: Dynamic Toolset Service
- **API Endpoints**: 9 REST endpoints (6 toolset operations + 3 standard)
- **Business Requirements**: BR-TOOLSET-033 (HTTP Server), BR-TOOLSET-039 (RFC 7807), BR-TOOLSET-043 (Content-Type)
- **Directory**: `docs/services/stateless/dynamic-toolset/`
- **Status**: Production-Ready (V1)

---

## Problem

### Low Business Value (6 out of 9 endpoints)

| Endpoint | Business Value | Issue |
|----------|----------------|-------|
| `POST /api/v1/discover` | 10% | Automatic discovery is primary use case |
| `POST /api/v1/toolsets/generate` | 5% | Controller auto-generates on every discovery |
| `POST /api/v1/toolsets/validate` | 5% | Controller validates automatically |
| `GET /api/v1/toolsets` | 0% | ConfigMap introspection is sufficient |
| `GET /api/v1/toolsets/{name}` | 0% | ConfigMap introspection is sufficient |
| `GET /api/v1/services` | 0% | ConfigMap introspection is sufficient |

**Total Business Value**: **20%** (6 out of 9 endpoints)

### Architectural Inconsistency

**Current Kubernaut Services**:
| Service | Pattern | CRDs |
|---------|---------|------|
| Gateway | Webhook receiver | No CRDs (webhook only) |
| Signal Processing | Controller | SignalProcessing CRD |
| AI Analysis | Controller | AIAnalysis CRD |
| Remediation Execution | Controller | RemediationExecution CRD |
| Notification | Controller | NotificationRequest CRD |
| Remediation Orchestrator | Controller | RemediationRequest CRD |
| **Dynamic Toolset** | **Controller + REST API** | **❌ No CRDs** |

**Inconsistency**: Dynamic Toolset is the only controller-based service with REST API endpoints.

### Maintenance Burden

**Code to Maintain**:
- REST API endpoint handlers (~200 LOC)
- Content-Type validation middleware (~130 LOC)
- RFC 7807 error responses (~100 LOC)
- OpenAPI specification (~600 lines YAML)
- Authentication logic (ADR-036 complexity)

**Total Maintenance**: ~1030 LOC + OpenAPI spec for 20% business value

---

## Decision

### V1 (Immediate)
**Disable 6 REST API endpoints** with low business value (0-10%):
- ❌ `POST /api/v1/discover` - **10% value** (automatic discovery is primary)
- ❌ `POST /api/v1/toolsets/generate` - **5% value** (controller auto-generates)
- ❌ `POST /api/v1/toolsets/validate` - **5% value** (controller validates)
- ❌ `GET /api/v1/toolsets` - **0% value** (redundant with ConfigMap)
- ❌ `GET /api/v1/toolsets/{name}` - **0% value** (redundant with ConfigMap)
- ❌ `GET /api/v1/services` - **0% value** (redundant with ConfigMap)

**Keep 3 standard endpoints** with high business value (100%):
- ✅ `GET /health` - Kubernetes liveness probe
- ✅ `GET /ready` - Kubernetes readiness probe
- ✅ `GET /metrics` - Prometheus metrics

### V1.1 (Next Release)
**Implement single `ToolsetConfig` configuration CRD** for:
- Configurable discovery interval
- Namespace filtering
- Service type filters
- Per-service health status

---

## Rationale

### 1. ConfigMap Introspection is Sufficient (100% Confidence)

**Current State**: Discovered services are already in ConfigMap
```bash
# View all discovered services (no REST API needed)
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml

# View specific service
kubectl get configmap kubernaut-toolset-config -n kubernaut-system \
  -o jsonpath='{.data.services}' | jq '.[] | select(.name=="prometheus-server")'
```

**Benefits**:
- ✅ Standard Kubernetes API (no custom REST endpoints)
- ✅ Works with kubectl, k9s, lens out-of-box
- ✅ RBAC-controlled (Kubernetes RBAC)
- ✅ Audit trail (Kubernetes audit logs)

---

### 2. REST API Business Value Assessment (98% Confidence)

| Endpoint | Business Value | Justification |
|----------|----------------|---------------|
| `POST /api/v1/discover` | **10%** | Automatic discovery is primary use case |
| `POST /api/v1/toolsets/generate` | **5%** | Controller auto-generates on every discovery |
| `POST /api/v1/toolsets/validate` | **5%** | Controller validates automatically |
| `GET /api/v1/toolsets` | **0%** | ConfigMap introspection is sufficient |
| `GET /api/v1/toolsets/{name}` | **0%** | ConfigMap introspection is sufficient |
| `GET /api/v1/services` | **0%** | ConfigMap introspection is sufficient |

**Total Business Value**: **20%** (6 out of 9 endpoints)

---

### 3. Avoids Maintenance Burden (99% Confidence)

**Code to Remove/Disable**:
- REST API endpoint handlers (~200 LOC)
- Content-Type validation middleware (~130 LOC)
- RFC 7807 error responses (~100 LOC)
- OpenAPI specification (~600 lines YAML)
- Authentication logic (ADR-036 complexity)

**Total Savings**: ~1030 LOC + OpenAPI spec

---

### 4. Architectural Consistency (100% Confidence)

**Current Kubernaut Services**:
| Service | Pattern | CRDs |
|---------|---------|------|
| Gateway | Webhook receiver | No CRDs (webhook only) |
| Signal Processing | Controller | SignalProcessing CRD |
| AI Analysis | Controller | AIAnalysis CRD |
| Remediation Execution | Controller | RemediationExecution CRD |
| Notification | Controller | NotificationRequest CRD |
| Remediation Orchestrator | Controller | RemediationRequest CRD |
| **Dynamic Toolset** | **Controller + REST API** | **❌ No CRDs** |

**After V1.1**:
| **Dynamic Toolset** | **Controller** | **✅ ToolsetConfig CRD** |

**Consistency**: 100% (all controllers use CRDs)

---

## V1.1 Configuration CRD Design

### CRD Type: Configuration CRD (Singleton)

**Key Distinction**:
- **Remediation Pipeline CRDs** (SignalProcessing, AIAnalysis, RemediationExecution): **Workflow/Data CRDs**
  - Each CRD instance = one remediation task
  - Multiple instances (one per alert)
  - Lifecycle: created → processed → completed/failed

- **ToolsetConfig CRD**: **Configuration CRD**
  - Singleton (one instance per cluster/namespace)
  - Configures controller behavior (like a validated ConfigMap)
  - No lifecycle (always exists, updated in place)

**Similar Pattern**: Kubernetes `KubeProxyConfiguration`, `KubeletConfiguration`

---

### CRD Schema (V1.1)

```yaml
apiVersion: toolset.kubernaut.io/v1alpha1
kind: ToolsetConfig
metadata:
  name: kubernaut-toolset-config  # Singleton name
  namespace: kubernaut-system
spec:
  # CONFIGURATION: How to discover services

  # Discovery interval (how often to scan)
  discoveryInterval: 5m  # Default: 5 minutes

  # Namespaces to scan
  namespaces:
    - monitoring
    - observability
    - "*"  # All namespaces (default)

  # Service type filters
  serviceTypes:
    prometheus:
      enabled: true
      labels:
        - app=prometheus
        - prometheus.io/scrape=true
    grafana:
      enabled: true
      labels:
        - app=grafana
    jaeger:
      enabled: true
      annotations:
        - jaeger.io/enabled=true
    elasticsearch:
      enabled: true
      labels:
        - app=elasticsearch
    custom:
      enabled: true
      annotations:
        - kubernaut.io/toolset=true

  # Health check configuration
  healthCheck:
    enabled: true
    timeout: 5s

  # ConfigMap generation settings
  configMap:
    name: kubernaut-toolset-config
    namespace: kubernaut-system
    preserveOverrides: true

status:
  # STATUS: Current state of discovery

  # Last discovery run metadata
  lastDiscoveryTime: "2025-11-10T10:30:00Z"
  nextDiscoveryTime: "2025-11-10T10:35:00Z"
  discoveryCount: 42

  # Per-service status (one entry per discovered service)
  # BOUNDED: Status size = number of services (not discovery runs)
  # IN-PLACE UPDATES: Existing entries updated, not appended
  discoveredServices:
    - name: prometheus-server
      namespace: monitoring
      type: prometheus
      endpoint: http://prometheus-server.monitoring.svc.cluster.local:9090
      healthy: true
      lastChecked: "2025-11-10T10:30:00Z"
      condition: "Ready"
      reason: "HealthCheckPassed"
      message: "Service is healthy and reachable"

    - name: grafana
      namespace: monitoring
      type: grafana
      endpoint: http://grafana.monitoring.svc.cluster.local:3000
      healthy: false
      lastChecked: "2025-11-10T10:30:00Z"
      condition: "NotReady"
      reason: "HealthCheckFailed"
      message: "HTTP 503: Service Unavailable"

  # Overall discovery status
  phase: "Completed"
  servicesDiscovered: 2
  servicesHealthy: 1
  servicesUnhealthy: 1
```

---

### Status Design Principles

#### 1. Configuration CRD Pattern
- Singleton (one instance per cluster/namespace)
- Spec = desired discovery behavior
- Status = current discovery state

#### 2. Bounded Status Growth
- Status contains **one entry per discovered service**
- Entry count = number of services (not discovery runs)
- Example: 10 services → 10 status entries (even after 1000 discovery runs)

#### 3. In-Place Updates
- Existing service entries **updated in place**
- No new entries for condition/reason/message changes
- Controller finds entry by `name+namespace` and updates fields

#### 4. History via Logs/Metrics
- CRD status = current state only
- Historical data in logs and Prometheus metrics

---

## Alternatives Considered

### Alternative 1: Keep REST API (Rejected)

**Pros**: No migration needed
**Cons**:
- ❌ High maintenance burden (~1000 LOC + OpenAPI spec)
- ❌ Low business value (20%)
- ❌ Architectural inconsistency (only service with REST API)
- ❌ Redundant with ConfigMap introspection

**Confidence**: 30% (not aligned with Kubernaut architecture)

---

### Alternative 2: Multiple CRDs per Operation (Rejected)

**Example**: `ToolsetDiscovery`, `ToolsetGeneration`, `ToolsetValidation` CRDs

**Pros**: Fine-grained control
**Cons**:
- ❌ Over-engineered (3 CRDs for simple configuration)
- ❌ Doesn't match use case (configuration, not workflow)
- ❌ Unnecessary complexity

**Confidence**: 40% (too complex for the use case)

---

### Alternative 3: Single Configuration CRD (Selected) ✅

**Pros**:
- ✅ Simple (one CRD for all configuration)
- ✅ Matches use case (configuration, not workflow)
- ✅ Bounded status (one entry per service)
- ✅ Kubernetes-native (standard CRD pattern)

**Confidence**: 98% ✅

---

## Impact

### V1 Changes (Immediate)

**Files Modified**:
1. `pkg/toolset/server/server.go` - Comment out REST API handlers
2. `docs/services/stateless/dynamic-toolset/BUSINESS_REQUIREMENTS.md` - Mark endpoints as disabled
3. `docs/services/stateless/dynamic-toolset/api-specification.md` - Add deprecation notice

**Code Removed/Disabled**:
- REST API endpoint handlers
- Content-Type validation tests (no longer needed)
- RFC 7807 error tests for disabled endpoints

**Benefits**:
- ✅ Simplifies V1 PR scope
- ✅ Avoids maintenance burden
- ✅ Clear migration path to V1.1

---

### V1.1 Changes (Next Release)

**New Components**:
1. `ToolsetConfig` CRD definition
2. `ToolsetConfigReconciler` controller
3. Per-service status tracking
4. ConfigMap generation from CRD spec

**Timeline**: 3-5 days

---

## Related Documentation

- **ADR-036**: Authentication and Authorization Strategy (REST API auth patterns)
- **DD-007**: Kubernetes-Aware Graceful Shutdown Pattern (kept for `/health`, `/ready`)
- **BR-TOOLSET-033**: HTTP Server (REST API endpoints)
- **BR-TOOLSET-039**: RFC 7807 Error Response Standard (disabled with REST API)
- **BR-TOOLSET-043**: Content-Type Validation Middleware (disabled with REST API)

---

## Future Considerations

### V1.1 Implementation Plan

**Phase 1: CRD Definition** (1 day)
- Define `ToolsetConfig` CRD schema
- Generate CRD manifests with `kubebuilder`
- Add CRD validation (discovery interval > 0, valid namespaces)

**Phase 2: Controller Implementation** (2 days)
- Implement `ToolsetConfigReconciler`
- Discovery loop with configurable interval
- Per-service status updates (in-place)
- ConfigMap generation from discovered services

**Phase 3: Testing** (1 day)
- Unit tests for controller logic
- Integration tests for CRD reconciliation
- E2E tests for discovery workflows

**Phase 4: Documentation** (1 day)
- Update BRs to reference CRD
- Add CRD examples
- Migration guide from REST API to CRD

---

## Approval

**Approved by**: User
**Date**: 2025-11-10
**Confidence**: 98%
**Risk Assessment**: LOW (REST API has 0-10% business value)
**Value Assessment**: HIGH (simplifies architecture, aligns with Kubernaut patterns)

---

**Document Status**: ✅ **APPROVED**
**Last Updated**: 2025-11-10
**Version**: 1.0.0

