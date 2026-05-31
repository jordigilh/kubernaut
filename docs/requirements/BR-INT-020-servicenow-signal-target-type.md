# BR-INT-020: ServiceNow Signal Target Type

**Business Requirement ID**: BR-INT-020
**Category**: Integration Layer
**Priority**: **P1 (HIGH)** - Customer Evaluation Enablement
**Target Version**: **V1.5**
**Status**: Proposed
**Date**: May 30, 2026
**Related BRs**: BR-INT-004 (ServiceNow Ticket Creation/Tracking), BR-WE-014 (Job Execution Backend), BR-AI-001 (Signal Investigation)
**GitHub Issue**: [#1338](https://github.com/jordigilh/kubernaut/issues/1338)

---

## Business Need

### Problem Statement

Kubernaut currently only supports signals originating from Kubernetes-native monitoring (Prometheus/AlertManager via Gateway). Customers using ServiceNow as their ITSM platform need to investigate ServiceNow tickets that reference cloud objects (cluster status, workload health) through Kubernaut's AI-powered investigation pipeline.

There is no mechanism today to:
1. Ingest a ServiceNow ticket as a signal into the remediation pipeline
2. Distinguish ServiceNow-originated signals from Kubernetes signals in downstream components
3. Correlate ServiceNow maintenance tickets with reported issues during KA investigation
4. Execute ServiceNow-specific remediation actions (close alert, create escalation ticket)
5. Verify remediation effectiveness for ServiceNow ticket outcomes

### Impact Without This BR

- Customers cannot use Kubernaut for ServiceNow ticket investigation
- No path to multi-platform signal support beyond Kubernetes
- Customer evaluation blocked for ServiceNow-centric organizations

---

## Business Objective

**Kubernaut SHALL support `TargetType="servicenow"` as a signal target type, enabling end-to-end investigation of ServiceNow tickets through the full remediation pipeline (AF -> SP -> AA -> KA -> WFE -> EM) with ServiceNow-specific tools, prompt reasoning, workflows, and effectiveness assessment.**

### Success Criteria

1. User can type "investigate ServiceNow ticket INC0012345" in AF chat and trigger a full investigation
2. AF validates the ticket is open, resolves CMDB CI for fingerprinting, and creates an RR with `targetType: "servicenow"`
3. `targetType` and `ProviderData` propagate through the full pipeline (RR -> SP -> AA -> KA)
4. KA uses ServiceNow-specific tools to query related maintenance tickets in real-time
5. KA prompt includes ServiceNow context and produces maintenance-correlation reasoning
6. Workflow selection chooses between "close alert" (maintenance-correlated) and "escalate" (not correlated)
7. WFE executes ServiceNow operations via existing Job executor with ServiceNow CLI container
8. EM verifies ticket state matches expected outcome (closed/resolved or new ticket created and assigned)
9. Feature toggle (`servicenow.enabled`) controls availability; disabled by default
10. E2E test validates both workflow paths with mock ServiceNow server and mock LLM

---

## Scope

### In Scope (Customer Evaluation Readiness)

- ServiceNow Go REST API client with interface, mock server, and graceful degradation
- CRD enum extension (`servicenow` added to `TargetType`)
- Pipeline plumbing: `targetType` and `ProviderData` propagation through AA boundary to KA
- AF intent detection and `HandleCreateRR` extension for ServiceNow tickets
- CMDB-based fingerprinting with parent CI resolution (mirrors K8s owner chain)
- SP Rego policy branching on `targetType`
- RO guardrails (skip K8s-specific operations for non-K8s targets)
- KA investigation tools (`servicenow_get_ticket`, `servicenow_query_maintenance`)
- KA prompt modification for maintenance-correlation reasoning
- Workflow catalog entries using existing Job executor
- EM assessment component for ServiceNow ticket verification
- Basic observability (metrics, structured logging, audit trail)
- Feature toggle and Helm configuration
- Customer prerequisites documentation
- Full TDD pyramid compliance (UT + IT + E2E)

### Out of Scope (Deferred to Production GA)

- SLO definitions and PrometheusRules for ServiceNow API availability
- Circuit breakers and advanced retry policies
- CMDB response caching
- ServiceNow API rate-limit detection and backpressure
- Credential rotation procedures
- Formal operational runbooks
- Performance benchmarks and load testing
- SOC2/FedRAMP control mapping
- Polished Grafana dashboards
- Multi-instance ServiceNow support
- Shared `ResourceResolver` interface refactor (K8s + ServiceNow unification)

---

## Key Design Decisions

### DD-INT-020-01: TargetType vs SignalSource

`TargetType="servicenow"` discriminates the target system for downstream components. `SignalSource` stays `"a2a-agent"` since AF is the Kubernaut ingestion point. This aligns with the existing pattern where `TargetType` indicates what the signal targets and `SignalSource` indicates where the signal came from.

### DD-INT-020-02: TOCTOU Prevention

AF fetches only the originating ServiceNow ticket at RR creation time. KA queries related maintenance tickets in real-time during investigation to avoid stale data. This prevents Time-of-Check-Time-of-Use problems where maintenance ticket status could change between AF pre-enrichment and KA investigation.

### DD-INT-020-03: CMDB-Based Fingerprinting

ServiceNow signal fingerprints are derived from CMDB CI parent resolution, mirroring the K8s owner-chain pattern in `pkg/gateway/types/fingerprint.go`. Formula: `SHA256("servicenow:{resolved_parent_ci_sys_id}")`. Fallback: `SHA256("servicenow:{ticket_number}")` when `cmdb_ci` is absent. Graceful degradation uses original CI sys_id if CMDB API fails.

### DD-INT-020-04: WFE via Existing Job Executor

ServiceNow ticket operations (close, create) are implemented as K8s Jobs using the existing `job` executor (BR-WE-014), not a new executor type. The ServiceNow Go client is packaged as a CLI container image. This avoids extending the WFE executor interface while reusing proven infrastructure.

### DD-INT-020-05: Pipeline Plumbing Gap

`targetType` and `ProviderData` are currently dropped at the AA boundary (AA CRD has no such fields, `buildSignalContext()` does not copy them, KA `IncidentRequest` and `SignalContext` have no such fields). This must be fixed across 9 files before any downstream ServiceNow-aware behavior can function.

---

## Implementation Phases

| Phase | Description | Dependencies |
|-------|-------------|--------------|
| 1 | ServiceNow Go REST API client | None |
| 2a | CRD enum update | None |
| 2b | Pipeline plumbing (targetType + ProviderData to KA) | None |
| 2c | AF investigate + HandleCreateRR extension | Phases 1, 2a, 2b |
| 3 | SP Rego + RO guardrails | Phase 2c |
| 4 | KA tools + prompt + mock LLM | Phases 2b, 2c, 3 |
| 5 | Workflow catalog (Job executor) | Phase 4 |
| 6 | EM assessment component | Phase 5 |
| 7 | Audit, observability, feature toggle | Phase 6 |
| 8 | Helm wiring | Phase 2a |
| 9 | E2E full pipeline test | Phases 7, 8 |
| 10 | Customer prerequisites docs | Phase 9 |

**Critical path**: Phase 1 -> Phase 2b -> Phase 2c -> Phase 4 -> Phase 5 -> Phase 6 -> Phase 9

---

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| ServiceNow API variability (customized instances) | Medium | High | Start with standard `incident` and `change_request` tables only |
| Customer CMDB lacks K8s workload-level CIs | Medium | Medium | Fingerprint falls back to ticket number |
| Mock LLM scenarios insufficient for maintenance reasoning | Low | Medium | Two deterministic scenarios (correlated + non-correlated) |
| CRD enum change requires coordinated rollout | Low | Low | Additive change, backward compatible |

---

## Detailed Plan

See `.cursor/plans/servicenow_signal_support_2093125a.plan.md` for the full implementation plan with file-level details, code references, and wiring manifests.
