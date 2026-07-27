# BR-AUDIT-021-030: Workflow Selection Audit Trail

**Document Version**: 2.1
**Date**: November 2025
**Updated**: July 2026
**Status**: ✅ APPROVED
**Category**: Audit & Compliance
**Related DDs**: DD-WORKFLOW-014 v4.0, DD-WORKFLOW-016, DD-WORKFLOW-017, DD-WORKFLOW-019, ADR-034, ADR-038

### Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.1 | 2026-07-23 | **Amendment (Issue #1677, DD-WORKFLOW-019)**: Relocated ownership of audit event *generation* for the four `workflow.catalog.*` events from Data Storage Service to KubernautAgent (KA), which now owns the discovery/scoring logic and informer-backed cache directly. Data Storage's role narrows to persistence (audit_events table) and the query API (BR-AUDIT-029/030, unchanged). Event types, schemas, and correlation semantics are unchanged. See §1.3, BR-AUDIT-022/023, §4.1, and the new §9 (v2.1 Implementation Status) below. |
| 2.0 | 2026-02-05 | **BREAKING**: Aligned with DD-WORKFLOW-014 v3.0 three-step discovery protocol. Replaced single `workflow.catalog.search_completed` event (BR-AUDIT-023) with four step-specific events. Updated query metadata (BR-AUDIT-025) for action-type taxonomy. Updated scoring (BR-AUDIT-026) for label-based scoring (no semantic search). Updated search metadata (BR-AUDIT-028) to remove embedding references. Updated query API (BR-AUDIT-030) for V3.0 event types. Supersedes DD-WORKFLOW-002 references with DD-WORKFLOW-016. |
| 1.0 | 2025-11-27 | Initial release with single `workflow.catalog.search_completed` event model |

---

## 1. Purpose & Scope

### 1.1 Business Purpose

The Workflow Selection Audit Trail provides comprehensive tracking of all workflow catalog search operations, enabling operators to debug workflow selection decisions, tune workflow definitions, ensure compliance, and analyze workflow effectiveness patterns.

### 1.2 Scope

- **Three-Step Discovery Audit**: Capture every step of the workflow discovery protocol (DD-WORKFLOW-016) with full context
- **Scoring Transparency**: Include label-based scoring breakdown for debugging
- **Cross-Service Correlation**: Enable tracing from remediation request to workflow selection via `remediationId`
- **Compliance Support**: Meet regulatory requirements for decision audit trails (SOC2 CC7.2, CC8.1)

### 1.3 Services Involved

| Service | Role | BR Coverage |
|---------|------|-------------|
| **HolmesGPT API** (superseded by **KubernautAgent**, see §9) | Search initiator, passes remediation_id | BR-AUDIT-021, BR-AUDIT-022 |
| **KubernautAgent (KA)** | Audit event **generator** for `workflow.catalog.*` (v2.1, Issue #1677, DD-WORKFLOW-019) — owns discovery/scoring and emits events via `BufferedDSAuditStore` | BR-AUDIT-023, BR-AUDIT-024 |
| **Data Storage Service** | Audit event **persistence** (audit_events table) + query API; no longer the generator (v2.1) | BR-AUDIT-025 through BR-AUDIT-030 |

---

## 2. HolmesGPT API Audit Requirements

### 2.1 Remediation ID Propagation

#### BR-AUDIT-021: Mandatory Remediation ID Propagation

**Requirement**: HolmesGPT API MUST propagate `remediationId` to Data Storage Service for every workflow discovery request across all three steps of the discovery protocol (DD-WORKFLOW-016).

**Business Value**:
- Enables correlation of all three discovery steps with the remediation lifecycle
- Supports end-to-end audit trail from signal detection to workflow selection to execution
- Required for compliance reporting and debugging

**Acceptance Criteria**:
- [ ] `remediationId` is included as a query parameter in all three DS discovery endpoints:
  - `GET /api/v1/workflows/actions?remediationId=...` (Step 1)
  - `GET /api/v1/workflows/actions/{action_type}?remediationId=...` (Step 2)
  - `GET /api/v1/workflows/{workflow_id}?remediationId=...` (Step 3)
- [ ] `remediationId` is passed through from AIAnalysis controller context
- [ ] Empty `remediationId` is handled gracefully (discovery proceeds, audit has empty correlation)
- [ ] `remediationId` is NOT used for search logic or workflow matching (correlation only)

**Implementation Reference**: DD-WORKFLOW-016, DD-WORKFLOW-014 v3.0

#### BR-AUDIT-022: No Ad Hoc Audit Generation Outside the Designated Owner

**Requirement (amended v2.1, Issue #1677, DD-WORKFLOW-019)**: No caller other than the designated audit-event owner may generate `workflow.catalog.*` audit events. **KubernautAgent (KA)** is that owner as of v2.1 — it both performs the three-step discovery/validation directly against its own informer-backed cache *and* generates the corresponding audit events via `internal/kubernautagent/audit/BufferedDSAuditStore`. This supersedes the original v1.0-v2.0 design, where HolmesGPT API called Data Storage Service and DS generated the events as a side effect of serving the query.

**Business Value**:
- Single source of truth for audit events (one designated generator, avoiding duplicate or missing events)
- Simplifies the caller's responsibilities — no other component reimplements audit-event construction for this domain

**Acceptance Criteria**:
- [ ] Only KA calls `internal/kubernautagent/audit` to emit `workflow.catalog.*` events; no other service independently constructs these event types
- [ ] No audit-related code duplicating this responsibility in APIFrontend's workflow-catalog MCP tools (they call KA, not DS, and do not emit their own audit events for this domain)
- [ ] KA performs discovery (Steps 1-3) and post-selection validation against its own cache, generating one audit event per step

**Implementation Reference**: DD-WORKFLOW-014 v4.0, DD-WORKFLOW-016, DD-WORKFLOW-019

---

## 3. Workflow Discovery Audit Event Generation Requirements

> **Amended v2.1 (Issue #1677, DD-WORKFLOW-019)**: this section's requirements were originally scoped to "Data Storage Service" (v1.0-v2.0 design, when DS hosted the discovery/scoring logic). As of v2.1, **KubernautAgent (KA)** owns discovery/scoring directly and is the audit event **generator**; Data Storage Service's remaining role is **persistence** (audit_events table) and the **query API** (§3.3 below, unchanged). The heading and BR text are updated in place below rather than left to silently contradict the current implementation — see §9 for the full amendment rationale.

### 3.1 Audit Event Generation

#### BR-AUDIT-023: Workflow Discovery Audit Event Generation (V3.0, amended v2.1)

**Requirement**: **KubernautAgent (KA)** MUST generate a step-specific audit event for every workflow discovery operation across the three-step protocol (DD-WORKFLOW-016), writing them to Data Storage Service via the buffered async audit pipeline (`BufferedDSAuditStore`, BR-AUDIT-024/ADR-038). The V2.0 single `workflow.catalog.search_completed` event is **deprecated** and replaced by four step-specific events. *(Originally scoped to Data Storage Service in v1.0-v2.0; ownership relocated to KA per DD-WORKFLOW-019 once DS's discovery/scoring logic itself moved to KA — see §9.)*

**Business Value**:
- Fine-grained audit trail for each decision point in the discovery flow
- Enables debugging of individual steps (e.g., "which action types were available?", "which workflows were considered?")
- Supports compliance with full traceability per remediation

**Acceptance Criteria**:
- [ ] `workflow.catalog.actions_listed` emitted after Step 1 (list available actions)
- [ ] `workflow.catalog.workflows_listed` emitted after Step 2 (list workflows for action type)
- [ ] `workflow.catalog.workflow_retrieved` emitted after Step 3 (get single workflow)
- [ ] `workflow.catalog.selection_validated` emitted after KA's post-selection validation re-query
- [ ] All four events include `remediationId` as `correlationId`
- [ ] All four events follow ADR-034 unified audit table schema
- [ ] Event schemas as defined in DD-WORKFLOW-014 v4.0

**Implementation Reference**: DD-WORKFLOW-014 v4.0, DD-WORKFLOW-016, DD-WORKFLOW-019, ADR-034

#### BR-AUDIT-024: Asynchronous Non-Blocking Audit

**Requirement**: Audit event generation MUST be asynchronous and non-blocking. Discovery response MUST NOT be delayed by audit operations.

**Business Value**:
- Maintains discovery performance SLA
- Audit failures don't impact business operations
- Graceful degradation under high load

**Acceptance Criteria**:
- [ ] Audit uses buffered async pattern (ADR-038) — KA's existing `BufferedDSAuditStore` (DD-AUDIT-002)
- [ ] Discovery response returned before audit write completes
- [ ] Audit failures logged but don't fail the discovery operation
- [ ] Buffer overflow handled gracefully (oldest events dropped)

**Implementation Reference**: ADR-038

### 3.2 Audit Event Content

#### BR-AUDIT-025: Signal Context Capture

**Requirement**: All discovery audit events MUST capture the complete signal context filters used for the query.

**Business Value**:
- Enables debugging of "why wasn't my workflow/action type available?"
- Supports filter pattern analysis across remediations
- Required for workflow tuning and catalog coverage analysis

**Acceptance Criteria**:
- [ ] `signalContext` captured in all four audit events with: `severity`, `component`, `environment`, `priority`, `customLabels`, `detectedLabels`
- [ ] `actionType` captured in Step 2 and Step 3 events (the selected action type)
- [ ] `workflowId` captured in Step 3 and validation events (the selected workflow)
- [ ] Pagination parameters (`offset`, `limit`) captured in Step 1 and Step 2 events
- [ ] Query timestamp captured

**Audit Event Schema (signal context section)**:
```json
{
  "signalContext": {
    "severity": "critical",
    "component": "deployment",
    "environment": "production",
    "priority": "P0",
    "customLabels": {"team": ["platform"]},
    "detectedLabels": {"hpaEnabled": true}
  }
}
```

#### BR-AUDIT-026: Scoring Breakdown Capture

**Requirement**: Step 2 audit events (`workflow.catalog.workflows_listed`) MUST capture the `finalScore` for each returned workflow. This score is used for debugging and tuning -- it is **not** exposed to the LLM.

**Business Value**:
- Enables debugging of workflow ranking decisions
- Supports workflow definition tuning
- Provides transparency into label-based scoring

**Acceptance Criteria**:
- [ ] `finalScore` captured for each workflow in Step 2 audit event
- [ ] `resultCount` captured (total workflows returned)
- [ ] Score is computed from label matching (detected label boost, custom label boost, label penalty) per DD-WORKFLOW-004
- [ ] `finalScore` is stripped before rendering to LLM (audit-only field)

**Audit Event Schema (scoring section in Step 2)**:
```json
{
  "workflows": [
    {
      "workflowId": "550e8400-e29b-41d4-a716-446655440000",
      "workflowName": "scale-conservative",
      "version": "1.0.0",
      "finalScore": 0.92
    }
  ]
}
```

**Implementation Reference**: DD-WORKFLOW-004, DD-WORKFLOW-014 v3.0

#### BR-AUDIT-027: Workflow Metadata Capture

**Requirement**: Audit events MUST capture full workflow metadata for each returned workflow.

**Business Value**:
- Provides complete context for debugging
- Enables workflow version tracking
- Supports owner/maintainer attribution

**Acceptance Criteria**:
- [ ] `workflow_id` and `version` captured
- [ ] `title` and `description` captured
- [ ] All `labels` captured
- [ ] `owner` and `maintainer` captured (if available)
- [ ] `content_hash` captured for integrity verification

**Audit Event Schema (workflow metadata section)**:
```json
{
  "workflow_id": "pod-oom-gitops",
  "version": "v1.0.0",
  "title": "Pod OOM GitOps Recovery",
  "rank": 1,
  "labels": {
    "signal-type": "OOMKilled",
    "severity": "critical",
    "resource-management": "gitops"
  },
  "owner": "platform-team",
  "maintainer": "oncall@example.com",
  "content_hash": "sha256:abc123..."
}
```

#### BR-AUDIT-028: Query Performance Metadata Capture

**Requirement**: All discovery audit events MUST capture query execution metadata for performance analysis.

**Business Value**:
- Enables discovery performance monitoring per step
- Supports capacity planning for workflow catalog growth
- Identifies slow queries for optimization

**Acceptance Criteria**:
- [ ] `queryDurationMs` captured in all four audit events
- [ ] Pagination metadata captured (`offset`, `limit`, `totalCount`, `hasMore`) in Step 1 and Step 2
- [ ] `securityGateResult` captured in Step 3 (`"pass"` or `"fail"`)
- [ ] `parameterCount` captured in Step 3 (number of parameters in the workflow schema)

**Audit Event Schema (query performance section)**:
```json
{
  "queryDurationMs": 12,
  "pagination": {
    "offset": 0,
    "limit": 10,
    "totalCount": 3,
    "hasMore": false
  }
}
```

### 3.3 Audit Data Retention & Access

#### BR-AUDIT-029: Audit Data Retention

**Requirement**: Workflow selection audit events MUST be retained according to compliance requirements.

**Business Value**:
- Meets regulatory retention requirements
- Enables historical analysis
- Supports long-term trend analysis

**Acceptance Criteria**:
- [ ] Minimum 90-day retention for operational analysis
- [ ] Configurable retention period (default: 365 days)
- [ ] Efficient querying for retained period
- [ ] Automatic partition management for audit_events table

**Implementation Reference**: ADR-034 (partitioned audit_events table)

#### BR-AUDIT-030: Audit Query API

**Requirement**: Data Storage Service MUST provide API endpoints for querying workflow selection audit events.

**Business Value**:
- Enables operator debugging workflows
- Supports compliance reporting
- Enables effectiveness analysis dashboards

**Acceptance Criteria**:
- [ ] Query by `correlationId` (remediationId) to reconstruct the full three-step discovery flow
- [ ] Query by `workflowId` (which remediations selected this workflow)
- [ ] Query by time range
- [ ] Query by event_type (any of the four V3.0 event types)
- [ ] Aggregation support for analytics

**V3.0 Event Types**:
- `workflow.catalog.actions_listed` (Step 1)
- `workflow.catalog.workflows_listed` (Step 2)
- `workflow.catalog.workflow_retrieved` (Step 3)
- `workflow.catalog.selection_validated` (post-selection)

**Example Queries**:
```sql
-- Reconstruct the full discovery flow for a remediation
SELECT * FROM audit_events
WHERE event_type LIKE 'workflow.catalog.%'
  AND correlation_id = 'rr-2026-02-05-abc123'
ORDER BY created_at ASC;

-- Find all remediations that selected a specific workflow
SELECT * FROM audit_events
WHERE event_type = 'workflow.catalog.workflow_retrieved'
  AND event_data->'payload'->>'workflowId' = '550e8400-e29b-41d4-a716-446655440000';

-- Find security gate failures (LLM attempted to use out-of-context workflow)
SELECT * FROM audit_events
WHERE event_type = 'workflow.catalog.workflow_retrieved'
  AND event_data->'payload'->>'securityGateResult' = 'fail';
```

---

## 4. Cross-Service Correlation Requirements

### 4.1 End-to-End Traceability

**Requirement**: The audit trail MUST enable end-to-end tracing from remediation request to workflow selection.

**Business Value**:
- Complete visibility into remediation lifecycle
- Supports debugging across service boundaries
- Required for compliance and forensics

**Correlation Chain (v2.1, KA-owned discovery, Issue #1677/DD-WORKFLOW-019)**:
```
AIAnalysis Controller
  → generates remediationId (kubernaut.ai/correlation-id label)
  → invokes KubernautAgent (KA) with remediationId

KubernautAgent (KA) — three-step discovery against its own informer-backed cache
  → Step 1: list_available_actions(remediationId=...)
  → Step 2: list_workflows(action_type, remediationId=...)
  → Step 3: get_workflow(workflow_id, remediationId=...)
  → Post-selection: validation re-query (remediationId=...)
  → for each step: constructs a step-specific audit event with correlationId = remediationId,
    event_category="workflow" (WithEventCategory), ResourceType/ResourceID set for Step 3 and
    validation (WithResource("Workflow", workflowId))
  → writes via BufferedDSAuditStore (async, non-blocking — BR-AUDIT-024/ADR-038)

Data Storage Service
  → persists each event to the audit_events table (one event per step)
  → serves the audit query API (BR-AUDIT-030, unchanged)

Audit Query
  → query by correlationId to reconstruct full three-step discovery flow
  → includes action type selection, workflow selection, parameter lookup, and validation
```

**Historical chain (v1.0-v2.0, superseded)**: HolmesGPT API called Data Storage Service's REST discovery endpoints directly, and DS generated the audit events as a side effect of serving each query. See DD-WORKFLOW-014 §"V3.0 Three-Step Discovery Audit Events" for the original design this superseded.

---

## 5. Success Metrics

### 5.1 Audit Coverage

| Metric | Target | Measurement |
|--------|--------|-------------|
| Audit event generation rate | 100% | Every discovery step generates its corresponding audit event |
| Correlation ID presence | 100% | Every audit event has correlationId |
| Scoring breakdown completeness | 100% | Step 2 events include finalScore for all workflows |

### 5.2 Performance

| Metric | Target | Measurement |
|--------|--------|-------------|
| Audit latency impact | <5ms | Search response time with/without audit |
| Audit write success rate | >99.9% | Successful audit writes / total searches |
| Audit query response time | <1s | P95 for common queries |

### 5.3 Compliance

| Metric | Target | Measurement |
|--------|--------|-------------|
| Retention compliance | 100% | All events retained for required period |
| Audit integrity | 100% | No audit events lost or corrupted |
| Query availability | 99.9% | Audit query API uptime |

---

## 6. Implementation Checklist

### 6.1 HolmesGPT API

- [x] Remove audit event generation code (DD-WORKFLOW-014 v2.0)
- [ ] Pass `remediationId` as query parameter in all three DS discovery endpoints (DD-WORKFLOW-016)
- [ ] Handle empty `remediationId` gracefully
- [ ] Update tests to verify no audit calls and correct `remediationId` propagation

### 6.2 Data Storage Service (v1.0-v2.0 historical checklist — superseded by §6.2b for generation, retained for persistence/query)

- [ ] ~~Implement `workflow.catalog.actions_listed` audit event in Step 1 handler~~ (superseded — see §6.2b)
- [ ] ~~Implement `workflow.catalog.workflows_listed` audit event in Step 2 handler~~ (superseded — see §6.2b)
- [ ] ~~Implement `workflow.catalog.workflow_retrieved` audit event in Step 3 handler~~ (superseded — see §6.2b)
- [ ] ~~Implement `workflow.catalog.selection_validated` audit event in validation handler~~ (superseded — see §6.2b)
- [ ] ~~Extract `remediationId` from query parameter in all three discovery handlers~~ (superseded — see §6.2b)
- [ ] ~~Include `signalContext`, `resultCount`, `pagination`, `queryDurationMs` in all events~~ (superseded — see §6.2b)
- [ ] ~~Include `finalScore` per workflow in Step 2 events (stripped before LLM rendering)~~ (superseded — see §6.2b)
- [ ] ~~Include `securityGateResult` in Step 3 events~~ (superseded — see §6.2b)
- [ ] ~~Implement async buffered audit write (ADR-038) for all four events~~ (superseded — see §6.2b)
- [ ] Deprecate `workflow.catalog.search_completed` event handler
- [x] Add audit query API endpoints for V3.0 event types (unchanged, DS remains the query API owner — BR-AUDIT-030)

### 6.2b KubernautAgent (v2.1, Issue #1677, DD-WORKFLOW-019) — current audit event generator

- [x] Add `WithEventCategory`/`WithResource` `EventOption`s and `ResourceType`/`ResourceID` fields to `AuditEvent` (`internal/kubernautagent/audit/emitter.go`)
- [x] Implement all 4 `workflow.catalog.*` `eventDataBuilder`s (`internal/kubernautagent/audit/ds_workflow_catalog_payloads.go`), registered in `eventDataBuilders` (`ds_store.go`)
- [x] Translate `ResourceType`/`ResourceID` in both `DSAuditStore.StoreAudit` and `BufferedDSAuditStore.StoreAudit`; fix pre-existing `ParentEventID` translation gap in `BufferedDSAuditStore`
- [ ] Wire audit emission into the 3 custom MCP tools' `Execute` methods (`list_available_actions`/`list_workflows`/`get_workflow`) — Phase 2d
- [ ] Wire audit emission into `select_workflow`/`investigate_discovery`'s post-selection validation path — Phase 2e

### 6.3 Documentation

- [x] Create this BR document
- [x] Update DD-WORKFLOW-014 to v3.0
- [x] Mark DD-WORKFLOW-002 as superseded by DD-WORKFLOW-016
- [ ] Create operator runbook for audit queries with V3.0 event types

---

## 7. Related Documents

| Document | Relationship |
|----------|--------------|
| [DD-WORKFLOW-014 v4.0](../architecture/decisions/DD-WORKFLOW-014-workflow-selection-audit-trail.md) | Technical design for three-step audit trail (V3.0 event schemas; v4.0 amends "who generates" to KA) |
| [DD-WORKFLOW-019](../architecture/decisions/DD-WORKFLOW-019-ka-owned-workflow-discovery.md) | Relocates discovery/scoring ownership (and, per this v2.1 amendment, audit generation) from DS to KA |
| [DD-WORKFLOW-016](../architecture/decisions/DD-WORKFLOW-016-action-type-workflow-indexing.md) | Three-step discovery protocol, KA-owned as of v2.1 (originally DS endpoints, HAPI tools) |
| [DD-WORKFLOW-017](../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) | Workflow lifecycle component interactions |
| [DD-WORKFLOW-004](../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-scoring.md) | Label-based scoring algorithm |
| [ADR-034](../architecture/decisions/ADR-034-unified-audit-table-design.md) | Unified audit table schema |
| [ADR-038](../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) | Async buffered audit pattern |
| [11_SECURITY_ACCESS_CONTROL.md](./11_SECURITY_ACCESS_CONTROL.md) | Parent audit requirements (BR-AUDIT-001-020) |
| ~~DD-WORKFLOW-002~~ | ~~SUPERSEDED by DD-WORKFLOW-016~~ |

---

## 8. Approval

| Role | Name | Date | Status |
|------|------|------|--------|
| Product Owner | - | 2025-11-27 | ✅ Approved |
| Technical Lead | - | 2025-11-27 | ✅ Approved |
| Security | - | 2025-11-27 | ✅ Approved |

---

*This document extends the audit requirements from 11_SECURITY_ACCESS_CONTROL.md (BR-AUDIT-001-020) with specific requirements for the workflow discovery audit trail. V2.0 aligns with the three-step discovery protocol (DD-WORKFLOW-016) and DD-WORKFLOW-014 v3.0 step-specific audit events. All implementations should align with these requirements to ensure comprehensive audit coverage for workflow catalog operations.*

---

## v1.3 Implementation Status: Kubernaut Agent

Issue [#433](https://github.com/jordigilh/kubernaut/issues/433) (Kubernaut Agent Go rewrite): **Kubernaut Agent (KA)** is the component that performs workflow discovery against Data Storage and drives post-selection validation. For **workflow catalog** audit events (`workflow.catalog.*`), behavior and BR mapping are unchanged: **Data Storage** remains the audit generator; **KA** replaces **HolmesGPT API** as the caller that propagates `remediationId` and triggers DS endpoints.

| BR | v1.3 status (KA) |
|----|------------------|
| **BR-AUDIT-021** | **Met by KA**: `remediationId` propagated on all three discovery steps and validation re-query (same contract as HAPI; implementation in KA). |
| **BR-AUDIT-022** | **Unchanged at v1.3, since amended at v2.1 (§9)**: HolmesGPT API / HAPI still MUST NOT emit `workflow.catalog.*` events; at v1.3, KA did not generate those events either (DS did). **Superseded by v2.1** — KA now IS the generator. |
| **BR-AUDIT-023**–**BR-AUDIT-024** | **Unchanged at v1.3, since amended at v2.1 (§9)**: DS owned step events and async non-blocking audit. **Superseded by v2.1** — KA now owns generation; DS retains persistence. |
| **BR-AUDIT-025**–**BR-AUDIT-028** | **Unchanged**: Payload requirements for the four V3.0 event types are unchanged in shape; construction moved from DS to KA at v2.1 (§9). |
| **BR-AUDIT-029**–**BR-AUDIT-030** | **Unchanged**: Retention and audit query API remain Data Storage Service responsibilities. |

**Granularity (related `aiagent.*` trail, #433)**: KA emits `aiagent.llm.tool_call` **per tool call** (not per turn) and `aiagent.workflow.validation_attempt` **per attempt**, including `workflow_id` and `is_final_attempt` where applicable. This complements the `workflow.catalog.*` events (generated by KA as of v2.1, see §9) for end-to-end forensics.

**Verification**: [TP-433-AUDIT-SOC2](../tests/433/TP-433-AUDIT-SOC2.md) — 19 unit tests, 8 integration tests, 3 E2E tests.

---

## 9. v2.1 Implementation Status: KA-Owned Workflow Discovery Audit Generation (Issue #1677)

**Authority**: [DD-WORKFLOW-019](../architecture/decisions/DD-WORKFLOW-019-ka-owned-workflow-discovery.md), [DD-WORKFLOW-014 v4.0](../architecture/decisions/DD-WORKFLOW-014-workflow-selection-audit-trail.md)

Issue [#1677](https://github.com/jordigilh/kubernaut/issues/1677) relocates ownership of the workflow/action-type discovery, scoring, and informer-backed cache from Data Storage Service (DS) into KubernautAgent (KA) — see DD-WORKFLOW-019. Because KA now performs the discovery/validation logic directly (not DS), **audit event generation for the four `workflow.catalog.*` events moves with it.** The v1.3 status above ("DS owns step events... Unchanged") is **superseded** by this section wherever the two conflict.

| BR | v2.1 status (KA-owned generation) |
|----|------------------------------------|
| **BR-AUDIT-021** | **Unchanged from v1.3**: `remediationId` propagation, now internal to KA (no longer an HTTP query parameter to DS — KA holds `remediationId` as the audit event's `correlationId` directly). |
| **BR-AUDIT-022** | **Amended**: KA is now the sole authorized generator of `workflow.catalog.*` events (was DS). No other service — including APIFrontend, which calls KA's MCP tools rather than DS directly — independently generates these events. |
| **BR-AUDIT-023** | **Amended**: KA generates all four step-specific events (`internal/kubernautagent/audit/ds_workflow_catalog_payloads.go`), using `event_category="workflow"` (`audit.WorkflowCatalogEventCategory`, via `WithEventCategory`) instead of KA's default `"aiagent"` category, and `ResourceType`/`ResourceID` = `("Workflow", workflowId)` for the Step 3 and validation events (via `WithResource`). Event type strings, correlation semantics, and payload schema (`WorkflowDiscoveryAuditPayload`) are byte-for-byte unchanged from v3.0/v1.3. |
| **BR-AUDIT-024** | **Unchanged in mechanism, changed in owner**: still async/non-blocking, now via KA's existing `BufferedDSAuditStore` (DD-AUDIT-002) rather than DS's internal buffered writer. |
| **BR-AUDIT-025**–**BR-AUDIT-028** | **Unchanged in shape**: same `signalContext`/scoring/metadata fields, now populated from KA's in-process discovery state rather than DS's request/response cycle. |
| **BR-AUDIT-029**–**BR-AUDIT-030** | **Unchanged**: Data Storage Service remains the persistence layer (audit_events table) and query API — KA writes to it exactly as it does for its other `aiagent.*` events. |

**Implementation** (Phase 2c of the DD-WORKFLOW-019 rollout):
- `internal/kubernautagent/audit/emitter.go`: `WithEventCategory`, `WithResource` `EventOption`s; `ResourceType`/`ResourceID` fields on `AuditEvent`; `EventTypeActionsListed`/`EventTypeWorkflowsListed`/`EventTypeWorkflowRetrieved`/`EventTypeSelectionValidated` and `ActionDiscovery`/`ActionRetrieve`/`ActionValidate` constants (values unchanged from DS's originals).
- `internal/kubernautagent/audit/ds_workflow_catalog_payloads.go`: the four `eventDataBuilder`s, reimplemented independently of DS's `pkg/datastorage/audit/workflow_discovery_event.go` (no KA dependency on DS's internal packages).
- `internal/kubernautagent/audit/ds_store.go` / `ds_buffered_store.go`: `ResourceType`/`ResourceID` translation onto `ogenclient.AuditEventRequest`; the four new `eventDataBuilders` entries; fixed a pre-existing `ParentEventID` translation gap in `BufferedDSAuditStore` (found while touching this code).
- Audit-event *emission* at the call sites (the 3 custom MCP tools and the post-selection validation path) lands in Phase 2d/2e — tracked separately, not yet complete as of this section's authoring.

**Verification**: `internal/kubernautagent/audit/emitter_test.go`, `ds_store_test.go`, `coverage_668_test.go` — unit tests for the `EventOption`s, payload builders, and `ResourceType`/`ParentEventID`/`EventCategory` round-trip through `BufferedDSAuditStore`'s in-process batch worker (`IT-KA-1677-AUDIT-INFRA-001a/b`), plus a 4-event single-`correlationId` reconstruction test (`IT-KA-1677-AUDIT-INFRA-002`) proving the SOC2 CC8.1 acceptance contract this amendment strengthens.

