# Gateway (GW) Service — Triage (Stateless Go Service)

**Date**: March 2, 2026  
**Service**: Gateway  
**Type**: Stateless HTTP API (Go)  
**Status**: Production-ready (v1.6+), P0 critical entry point

---

## 1. What the Gateway Service Is

The **Gateway** is the **single entry point** for all external signals into the Kubernaut remediation pipeline. It is a **stateless** Go HTTP service that:

- **Ingests** alerts/events from Prometheus AlertManager and the Kubernetes Event API
- **Validates** payloads and rejects signals without Kubernetes resource information
- **Deduplicates** signals (status-based, with occurrence tracking) to cut downstream load
- **Creates** RemediationRequest CRDs so Remediation Orchestrator and Signal Processing can consume work via the Kubernetes API (no direct HTTP calls to downstream services)

**Critical note**: Gateway is the **only** service that does duplicate detection. Downstream services (Signal Processing, AI Analysis, etc.) receive only non-duplicate work via CRDs.

---

## 2. Core Responsibilities (Summary)

| Responsibility | Description | BR / DD |
|----------------|-------------|---------|
| **Signal ingestion** | HTTP webhooks: Prometheus, K8s Events | BR-GATEWAY-001, BR-GATEWAY-002 |
| **Validation** | Reject signals without K8s resource (Kind, Name, namespace) | DD-GATEWAY-NON-K8S-SIGNALS |
| **Deduplication** | Fingerprint + CRD status; no Redis (DD-GATEWAY-012) | DD-GATEWAY-011 |
| **CRD creation** | Create/update RemediationRequest with TargetResource, TargetType | DD-GATEWAY-011, ADR-049 |
| **Audit** | Emit audit events to Data Storage REST API | ADR-032, ADR-034 |
| **Security** | TokenReview auth, rate limiting, security headers | BR-GATEWAY-004, BR-109 |
| **Observability** | Health/ready, Prometheus metrics, request IDs | Standard |

**Explicitly out of scope (delegated or removed)**:

- **Classification** (environment/priority): owned by Signal Processing (DD-CATEGORIZATION-001)
- **Storm detection**: removed (DD-GATEWAY-015); deduplication covers the need
- **Redis**: removed; deduplication state is in K8s CRD status (DD-GATEWAY-012)

---

## 3. Architecture (Stateless Go)

### 3.1 Process Model

- **Single process**, HTTP server on configurable host:port (default 8080 for API).
- **No long-lived in-process state** for request routing; deduplication state lives in **Kubernetes** (RemediationRequest `.status.deduplication`).
- **Horizontally scalable**: 2–5 replicas; coordination via K8s Leases (BR-GATEWAY-190) for distributed locking where needed.
- **Graceful shutdown**: 30s timeout, DD-007 compliant.

### 3.2 Ports

| Port | Purpose |
|------|---------|
| **8080** | REST API (signals, health, ready) |
| **8081** | Health probes (`/healthz`, `/readyz`) — doc variance; implementation may bind both on same port |
| **9090** | Prometheus `/metrics` (authenticated) |

### 3.3 Request Flow (High Level)

1. **Ingestion**: Request hits `/api/v1/signals/prometheus` or `/api/v1/signals/kubernetes-event`.
2. **Adapter**: Prometheus or Kubernetes Event adapter parses and normalizes to internal signal type; owner resolution (e.g. Pod → Deployment) via cached K8s metadata (ADR-053).
3. **Validation**: Resource validation (must have K8s resource info); label filtering (e.g. monitoring metadata per Issue #191).
4. **Deduplication**: Fingerprint computed; lookup/update via RemediationRequest CRD status (DD-GATEWAY-011).
5. **CRD creation**: If new signal, create RemediationRequest; if duplicate, update status (e.g. lastOccurrence).
6. **Audit**: Emit gateway audit events (e.g. signal.received, signal.deduplicated, crd.created) to Data Storage API (ADR-032).
7. **Response**: 201 Created, 202 Accepted (duplicate), 400 Bad Request, or 5xx.

---

## 4. Main Code Layout (`pkg/gateway`)

| Area | Path | Purpose |
|------|------|---------|
| **Entry** | `cmd/gateway/main.go` | Config load, server create, adapter registration, signal handling, graceful shutdown |
| **Server** | `pkg/gateway/server.go` | HTTP server, routing, pipeline orchestration, audit, metrics |
| **Config** | `pkg/gateway/config/` | YAML config (ADR-030), env overrides, validation |
| **Adapters** | `pkg/gateway/adapters/` | Prometheus adapter, Kubernetes Event adapter; registry; owner resolution; label filter |
| **Processing** | `pkg/gateway/processing/` | Deduplication, phase checker, CRD creator, status updater, distributed lock (leases) |
| **Types** | `pkg/gateway/types/` | Fingerprint, normalized signal types, owner resolution types |
| **K8s** | `pkg/gateway/k8s/` | Controller-runtime client, circuit breaker (gobreaker) |
| **Middleware** | `pkg/gateway/middleware/` | Request ID, security headers, metrics, content-type, event freshness |
| **Metrics** | `pkg/gateway/metrics/` | Prometheus metrics |
| **Errors** | `pkg/gateway/errors/` | RFC 7807 problem details |
| **Scope** | `pkg/gateway/scope.go` | Resource scope (BR-SCOPE-002) |

---

## 5. Integrations

| System | Direction | Purpose |
|--------|-----------|---------|
| **Prometheus AlertManager** | Inbound | POST webhook → `/api/v1/signals/prometheus` |
| **Kubernetes Event API** | Inbound | POST webhook → `/api/v1/signals/kubernetes-event` |
| **Kubernetes API** | Outbound | CRD create/update (RemediationRequest), Leases, metadata (owner resolution, scope) |
| **Data Storage Service** | Outbound | REST API for audit events (ADR-032) |
| **Remediation Orchestrator** | Downstream | Watches RemediationRequest CRDs (no direct HTTP from GW) |
| **Signal Processing** | Downstream | Consumes CRDs after RO/SP flow (no direct HTTP from GW) |

**Note**: Integration-points doc may still mention Redis; implementation is **Redis-free** per DD-GATEWAY-012; deduplication state is in K8s only.

---

## 6. Testing and Quality

- **Unit**: 314+ tests (adapters, deduplication, fingerprint, validation, CRD creation, metrics, middleware).
- **Integration**: 104+ tests (envtest, CRD lifecycle, deduplication, scope, audit, error handling).
- **E2E**: 24+ tests (ingestion, metrics, security headers, deduplication, audit, graceful shutdown).
- **Total**: 442+ tests (per README); all three tiers present.

---

## 7. Design Decisions (Quick Ref)

| DD | Summary |
|----|--------|
| DD-GATEWAY-011 | Status-based deduplication; CRD status as source of truth |
| DD-GATEWAY-012 | Redis removed; K8s-native only |
| DD-GATEWAY-015 | Storm detection removed |
| DD-GATEWAY-NON-K8S-SIGNALS | V1.0: reject signals without K8s resource info |
| DD-CATEGORIZATION-001 | Classification moved to Signal Processing |
| ADR-030 | Config via YAML file + env for secrets |
| ADR-032 / ADR-034 | Audit via Data Storage API; event types/categories |
| ADR-049 | RemediationRequest schema owned by RO; GW imports from `api/remediation/v1alpha1` |
| ADR-053 | Metadata-only informer for scope/owner; no extra API calls |
| DD-007 | Kubernetes-aware graceful shutdown |

---

## 8. Triage Summary

| Item | Assessment |
|------|------------|
| **Role** | Single entry point: ingest → validate → deduplicate → CRD create → audit |
| **Stateless** | Yes; no Redis; state in K8s CRD status and Leases |
| **Language / runtime** | Go, single binary, HTTP server |
| **Upstream** | Prometheus AlertManager, K8s Events (webhooks) |
| **Downstream** | RemediationRequest CRDs only (RO/SP watch); audit to Data Storage |
| **Maturity** | Production-ready (v1.6), P0, 442+ tests across unit/integration/E2E |
| **Risks / follow-ups** | Keep integration-points doc aligned with DD-GATEWAY-012 (no Redis); ensure health/ready port usage matches deployment (8080 vs 8081). |

---

## 9. References

- [Gateway README](../services/stateless/gateway-service/README.md)
- [Overview](../services/stateless/gateway-service/overview.md)
- [Implementation](../services/stateless/gateway-service/implementation.md)
- [Integration points](../services/stateless/gateway-service/integration-points.md)
- [Deduplication](../services/stateless/gateway-service/deduplication.md)
- [BUSINESS_REQUIREMENTS](../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md)
- `cmd/gateway/main.go`, `pkg/gateway/server.go`
