# Effectiveness Monitor Service

**Version**: v2.0
**Status**: ✅ Implemented (V1.0 Level 1: Automated Assessment). Level 2 (AI analysis): planned V1.1, [DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md)
**Health/Ready Port**: 8081 (`/healthz`, `/readyz` — no auth required)
**Metrics Port**: 9090 (`/metrics`)
**CRD**: `EffectivenessAssessment` (short name: `ea`)
**CRD API Group**: `kubernaut.ai/v1alpha1`
**Controller**: `Reconciler` (`internal/controller/effectivenessmonitor/`)
**Type**: Kubernetes CRD controller (controller-runtime `ctrl.Manager`) — **not** a stateless HTTP service
**AI/LLM dependency**: None in V1.0 — 100% deterministic rule-based scoring

---

## 📋 Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| **v2.0** | 2026-08-02 | **#1806 CORRECTION**: Full rewrite of this documentation hub (and `overview.md`, `crd-schema.md`, `integration-points.md`, `observability-logging.md`). Corrected the service type from "Stateless HTTP API Service" (port 8080, Docker image `quay.io/jordigilh/monitor-service`) to the real Kubernetes CRD controller. Removed all references to `pkg/ai/insights/`, PostgreSQL persistence (`migrations/001_v1_schema.sql`), the "Context API" learning sink, `BR-INS-*` requirements, and a live HolmesGPT `POST /api/v1/postexec/analyze` call. Renamed `api-specification.md` → `crd-schema.md`. | [#1806](https://github.com/jordigilh/kubernaut/issues/1806) |
| v1.x | 2025-10-06 → 2025-10-16 | ⚠️ **STALE (superseded by v2.0)** — Described a fictional stateless HTTP architecture with a live selective-AI-analysis endpoint that was never built. `security-configuration.md` and `testing-strategy.md` still carry this framing as of this rewrite — out of scope for this correction pass, flagged for future cleanup. | — |

---

## 🗂️ Documentation Index

| Document | Purpose | Status |
|----------|---------|--------|
| **[Overview](./overview.md)** | Service purpose, V1.0 scope, architecture, component scorers, phases | ✅ Rewritten (v2.0) |
| **[CRD Schema](./crd-schema.md)** | `EffectivenessAssessment` spec/status field reference | ✅ Rewritten (v2.0), renamed from `api-specification.md` |
| **[Integration Points](./integration-points.md)** | Upstream/downstream services, audit event contract | ✅ Rewritten (v2.0) |
| **[Observability & Logging](./observability-logging.md)** | Structured logging, Prometheus metrics, alert rules | ✅ Rewritten (v2.0) |
| **[Audit Event Catalog](./security/AUDIT_EVENT_CATALOG.md)** | Authoritative reference for all 7 audit events EM emits | ✅ Current (as of 2026-07-31) |
| `security-configuration.md` | RBAC, network policies | ⚠️ **STALE** — still describes a stateless HTTP service with TokenReviewer auth on port 8080; not corrected in this pass |
| `testing-strategy.md` | Test patterns | ⚠️ **STALE** — still describes HTTP-mock integration tests for a REST API; not corrected in this pass |

---

## 📁 File Organization

```
07-effectivenessmonitor/
├── 📄 README.md (you are here)         - Service index & navigation
├── 📘 overview.md                      - Architecture, phases, component scorers ✅
├── 🔧 crd-schema.md                    - EffectivenessAssessment CRD fields ✅
├── 🔗 integration-points.md            - Upstream/downstream, audit contract ✅
├── 📊 observability-logging.md         - Logging & Prometheus metrics ✅
├── 🔒 security-configuration.md        - RBAC, network policies ⚠️ STALE
├── 🧪 testing-strategy.md              - Test patterns ⚠️ STALE
└── security/
    └── 📋 AUDIT_EVENT_CATALOG.md       - Audit event schema reference ✅
```

---

## 🏗️ Implementation Structure

| Layer | Location |
|-------|----------|
| **Entry point** | `cmd/effectivenessmonitor/main.go` |
| **Controller** | `internal/controller/effectivenessmonitor/` (`reconciler.go`, `completion.go`, `assess_components.go`, `reconcile_status.go`, `reconcile_spec_drift.go`, `reconcile_validity_phase.go`, `target_resources.go`, `pod_health.go`, `scope.go`) |
| **CRD types** | `api/effectivenessassessment/v1alpha1/effectivenessassessment_types.go` |
| **Business logic** | `pkg/effectivenessmonitor/{health,alert,metrics,hash,validity,types,conditions,audit,client,metrics}/` |
| **Config** | `internal/config/effectivenessmonitor/config.go` (default `/etc/effectivenessmonitor/config.yaml`, ADR-030; Helm overrides the filename via `--config`) |
| **Deployment** | `charts/kubernaut/templates/effectivenessmonitor/{effectivenessmonitor,networkpolicy,servicemonitor}.yaml` |

**Build**: `go build -o bin/effectivenessmonitor ./cmd/effectivenessmonitor`

---

## 🎯 V1.0 Scope

### ✅ In Scope (implemented)

| Feature | Description | Reference |
|---------|-------------|-----------|
| **Health check** | K8s pod status decision tree | BR-EM-001 |
| **Alert resolution check** | AlertManager query + alert decay detection | BR-EM-002, BR-EM-012 |
| **Metric comparison** | 5 PromQL queries, pre/post remediation | BR-EM-003 |
| **Spec hash / drift detection** | SHA-256 canonical fingerprint comparison | BR-EM-004, DD-EM-002 |
| **Dual-target assessment** | Signal target vs. remediation target tracked separately | DD-EM-003 |
| **Async hash deferral** | `WaitingForPropagation` phase for GitOps/operator-managed targets | DD-EM-004, BR-EM-010 |
| **Derived timing** | Validity/stabilization/alert-check deadlines computed and persisted in status | BR-EM-009 |
| **Cluster-scoped metrics** | Node/PersistentVolume kube-state-metrics queries | DD-EM-005 |
| **Fleet-aware reads** | Multi-cluster target reads via `fleet.ReaderFactory` | BR-FLEET-054, ADR-068 |
| **Audit trail** | 7 typed component audit events to DataStorage | BR-AUDIT-006 |

### ❌ Out of Scope (V1.1+)

| Feature | Deferred Reason | Target Version |
|---------|-----------------|-----------------|
| **AI-powered root cause validation** ("problem solved" vs. "problem masked") | Requires historical Level 1 data to be useful | V1.1 (Level 2), DD-017 |
| **Oscillation detection** ("fix A caused problem B") | Same as above | V1.1, DD-017 |
| **`POST /api/v1/postexec/analyze`** | ⚠️ **Planned — NOT YET IMPLEMENTED anywhere in the codebase** (not in EM, not in KA's `openapi.json`) | V1.1, DD-017 |
| **Computing the final weighted effectiveness score** | By design, EM only emits raw component data; DataStorage aggregates | Never EM's job — see ADR-EM-001 §6 |
| **Side-effect / collateral-damage causality analysis** | Cross-resource causality is out of scope for the formula-based V1.0 approach | Post-V1.0 |

---

## 🔗 Related Services

| Service | Relationship | Purpose |
|---------|--------------|---------|
| **Remediation Orchestrator** | Upstream (creates) | Creates the `EffectivenessAssessment` CRD (ownerRef → `RemediationRequest`) when RR reaches `Verifying`/`Completed`/`Failed`/`TimedOut`; later watches EA completion to set the `EffectivenessAssessed` condition on the RR |
| **Kubernetes API** | Downstream | Pod/health status, current spec for hashing (Fleet-aware via `fleet.ReaderFactory`) |
| **Prometheus** | Downstream | Pre/post metric comparison (`/api/v1/query_range`) |
| **AlertManager** | Downstream | Alert resolution check |
| **DataStorage** | Downstream (writes + reads) | Audit event sink; also a `remediation.workflow_created` fallback query and pre-hash lookup |
| **MCP Gateway** (Fleet federation) | Downstream (optional) | Multi-cluster target reads when Fleet federation is enabled |

**Coordination pattern**: CRD watch only — EM has no direct HTTP/gRPC calls to or from any other Kubernaut controller.

---

## 📋 Business Requirements Coverage

| Category | Key BRs |
|----------|---------|
| **Core assessment** | BR-EM-001 (health), BR-EM-002 (alert), BR-EM-003 (metrics), BR-EM-004 (hash) |
| **Lifecycle & timing** | BR-EM-005 (phases), BR-EM-006/007/008 (stabilization/validity), BR-EM-009 (derived timing), BR-EM-010 (async hash deferral) |
| **Alert decay** | BR-EM-012 (Issue #369) |
| **Audit / compliance** | BR-AUDIT-006 (SOC2 CC7.2) |
| **Multi-cluster** | BR-FLEET-054 |

⚠️ **STALE ID note**: Older docs in this directory (`security-configuration.md`, `testing-strategy.md`) reference `BR-INS-001` through `BR-INS-010`. These IDs have no backing requirement document under `docs/requirements/` and do not appear in the current implementation's source comments. Per repo convention, this rewrite does not silently renumber them — they are flagged here as orphaned pending a dedicated requirements-mapping correction.

---

## 🎯 Key Architectural Decisions

| Decision | Choice | Rationale | Document |
|----------|--------|-----------|-----------|
| **Service type** | CRD controller, not HTTP service | Matches the rest of Kubernaut's CRD-orchestrated architecture; no polling or push API needed | ADR-EM-001 |
| **Scoring ownership** | EM emits raw data; DataStorage computes the weighted score | Lets the scoring formula evolve without re-emitting historical audit events | ADR-EM-001 §6, DD-017 |
| **AI integration** | None in V1.0 | Level 1 (deterministic) ships first; Level 2 (AI) requires historical Level 1 data to add value | DD-017 |
| **Dual-target tracking** | `signalTarget` + `remediationTarget` on spec | The alert-triggering resource and the resource the workflow modified can differ | DD-EM-003 |
| **Async hash deferral** | `WaitingForPropagation` phase, `hashComputeDelay` | GitOps/operator-managed targets need time for the spec change to propagate before it's safe to hash | DD-EM-004 |
| **Watch predicate** | `predicate.GenerationChangedPredicate{}` | EA spec is immutable and EM is the sole status writer; relying on generation changes (not status writes) avoids a hot reconciliation loop — progression is driven by explicit `RequeueAfter`, not further watch events | `reconciler.go` `SetupWithManager` |

---

## 📊 Performance Targets

| Metric | Target | Notes |
|--------|--------|-------|
| **Stabilization window** | Default 5m (configurable per-EA via RO) | Time to wait after remediation before assessing |
| **Component checks** | Independent, non-blocking | A slow/unreachable Prometheus or AlertManager does not block the other 3 components |
| **Validity window** | EM config (`internal/config/effectivenessmonitor/config.go`) | Forces `Completed`/`Expired` if components never all report |

---

## 🚀 Quick Start

**For new developers**:
1. Start with [Overview](./overview.md) (5 min read) — architecture, component scorers, phase state machine
2. Read [CRD Schema](./crd-schema.md) (10 min read) — the `EffectivenessAssessment` spec/status contract
3. Read [Integration Points](./integration-points.md) — how EM fits between the Remediation Orchestrator and DataStorage

**For implementers**:
1. Read `pkg/effectivenessmonitor/types/types.go` for the shared `ComponentResult`/`AuditEventType` types
2. Read the [Audit Event Catalog](./security/AUDIT_EVENT_CATALOG.md) before touching any audit-emitting code
3. Reference [ADR-EM-001](../../../architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md) for the full scoring formula and SOC2 chain-of-custody design

---

## 🔍 Common Pitfalls & Best Practices

**Don't**:
- ❌ Assume EM computes the final effectiveness score — it doesn't; DataStorage does, on demand
- ❌ Add an HTTP handler or business API to EM — it is watch-driven only; there is no port 8080
- ❌ Call HolmesGPT/KA from EM — no AI dependency exists in V1.0 (planned V1.1, DD-017)
- ❌ Persist assessment data directly to a database from EM — all writes go through DataStorage's audit pipeline (`pkg/audit.BufferedStore`)
- ❌ Rely on a watch event to re-trigger reconciliation after a status update — the watch predicate is generation-based; use `RequeueAfter` for time-driven progression

**Do**:
- ✅ Treat each of the 4 component scorers (health, alert, metrics, hash) as independent — one failing must not block the others
- ✅ Propagate `ea.Spec.CorrelationID` into every log line and audit event
- ✅ Use `fleet.ReaderFactory`/`ReaderFor()` for target-facing reads when `ea.Spec.ClusterID` is set
- ✅ Check `docs/architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md` before proposing any AI-related enhancement to this service

---

## 📞 Support & Documentation

- **Architecture**: [ADR-EM-001](../../../architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md) — authoritative integration architecture
- **Scoping decision**: [DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md) — V1.0 Level 1 / V1.1 Level 2 boundary
- **CRD design**: [CRD Schema](./crd-schema.md)
- **Fleet federation**: [ADR-068](../../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
- **Documentation structure**: [DD-006](../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md) (COMMON PATTERN vs SERVICE-SPECIFIC files)

---

**Document Maintenance**:
- **Last Updated**: 2026-08-02
- **Source of Truth**: `api/effectivenessassessment/v1alpha1/effectivenessassessment_types.go`, `internal/controller/effectivenessmonitor/`, `pkg/effectivenessmonitor/`
