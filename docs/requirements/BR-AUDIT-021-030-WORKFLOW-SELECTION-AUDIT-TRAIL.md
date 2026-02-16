# BR-AUDIT-021-030: Workflow Selection Audit Trail

**Document Version**: 2.0
**Date**: November 2025
**Updated**: February 2026
**Status**: ✅ APPROVED
**Category**: Audit & Compliance
**Related DDs**: DD-WORKFLOW-014 v3.0, DD-WORKFLOW-016, DD-WORKFLOW-017, ADR-034, ADR-038

### Changelog

| Version | Date | Changes |
|---------|------|---------|
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
| **HolmesGPT API** | Search initiator, passes remediation_id | BR-AUDIT-021, BR-AUDIT-022 |
| **Data Storage Service** | Audit event generator, search executor | BR-AUDIT-023 through BR-AUDIT-030 |

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

#### BR-AUDIT-022: No Audit Generation in HolmesGPT API

**Requirement**: HolmesGPT API MUST NOT generate audit events for workflow searches. Audit generation is the responsibility of Data Storage Service.

**Business Value**:
- Single source of truth for audit events (Data Storage has richer context)
- Avoids duplicate audit events
- Simplifies HolmesGPT API responsibilities

**Acceptance Criteria**:
- [ ] HolmesGPT API does not call `/api/v1/audit/events` endpoint
- [ ] No audit-related code in workflow catalog toolset
- [ ] HAPI makes only DS discovery calls (Steps 1-3) plus the post-selection validation re-query; DS generates audit events for each

**Implementation Reference**: DD-WORKFLOW-014 v3.0, DD-WORKFLOW-016

---

## 3. Data Storage Service Audit Requirements

### 3.1 Audit Event Generation

#### BR-AUDIT-023: Workflow Discovery Audit Event Generation (V3.0)

**Requirement**: Data Storage Service MUST generate a step-specific audit event for every workflow discovery operation across the three-step protocol (DD-WORKFLOW-016). The V2.0 single `workflow.catalog.search_completed` event is **deprecated** and replaced by four step-specific events.

**Business Value**:
- Fine-grained audit trail for each decision point in the discovery flow
- Enables debugging of individual steps (e.g., "which action types were available?", "which workflows were considered?")
- Supports compliance with full traceability per remediation

**Acceptance Criteria**:
- [ ] `workflow.catalog.actions_listed` emitted after Step 1 (list available actions)
- [ ] `workflow.catalog.workflows_listed` emitted after Step 2 (list workflows for action type)
- [ ] `workflow.catalog.workflow_retrieved` emitted after Step 3 (get single workflow)
- [ ] `workflow.catalog.selection_validated` emitted after HAPI post-selection validation re-query
- [ ] All four events include `remediationId` as `correlationId`
- [ ] All four events follow ADR-034 unified audit table schema
- [ ] Event schemas as defined in DD-WORKFLOW-014 v3.0

**Implementation Reference**: DD-WORKFLOW-014 v3.0, DD-WORKFLOW-016, ADR-034

#### BR-AUDIT-024: Asynchronous Non-Blocking Audit

**Requirement**: Audit event generation MUST be asynchronous and non-blocking. Search response MUST NOT be delayed by audit operations.

**Business Value**:
- Maintains search performance SLA
- Audit failures don't impact business operations
- Graceful degradation under high load

**Acceptance Criteria**:
- [ ] Audit uses buffered async pattern (ADR-038)
- [ ] Search response returned before audit write completes
- [ ] Audit failures logged but don't fail search operation
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

**Correlation Chain (V3.0 Three-Step Discovery)**:
```
AIAnalysis Controller
  → generates remediationId (kubernaut.ai/correlation-id label)
  → calls HolmesGPT API with remediationId

HolmesGPT API (three-step discovery)
  → Step 1: GET /api/v1/workflows/actions?remediationId=...
  → Step 2: GET /api/v1/workflows/actions/{action_type}?remediationId=...
  → Step 3: GET /api/v1/workflows/{workflow_id}?remediationId=...
  → Post-selection: GET /api/v1/workflows/{workflow_id}?remediationId=... (validation)

Data Storage Service
  → extracts remediationId from query parameter on each request
  → generates step-specific audit event with correlationId = remediationId
  → stores in audit_events table (one event per step)

Audit Query
  → query by correlationId to reconstruct full three-step discovery flow
  → includes action type selection, workflow selection, parameter lookup, and validation
```

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

### 6.2 Data Storage Service

- [ ] Implement `workflow.catalog.actions_listed` audit event in Step 1 handler
- [ ] Implement `workflow.catalog.workflows_listed` audit event in Step 2 handler
- [ ] Implement `workflow.catalog.workflow_retrieved` audit event in Step 3 handler
- [ ] Implement `workflow.catalog.selection_validated` audit event in validation handler
- [ ] Extract `remediationId` from query parameter in all three discovery handlers
- [ ] Include `signalContext`, `resultCount`, `pagination`, `queryDurationMs` in all events
- [ ] Include `finalScore` per workflow in Step 2 events (stripped before LLM rendering)
- [ ] Include `securityGateResult` in Step 3 events
- [ ] Implement async buffered audit write (ADR-038) for all four events
- [ ] Deprecate `workflow.catalog.search_completed` event handler
- [ ] Add audit query API endpoints for V3.0 event types

### 6.3 Documentation

- [x] Create this BR document
- [x] Update DD-WORKFLOW-014 to v3.0
- [x] Mark DD-WORKFLOW-002 as superseded by DD-WORKFLOW-016
- [ ] Create operator runbook for audit queries with V3.0 event types

---

## 7. Related Documents

| Document | Relationship |
|----------|--------------|
| [DD-WORKFLOW-014 v3.0](../architecture/decisions/DD-WORKFLOW-014-workflow-selection-audit-trail.md) | Technical design for three-step audit trail (V3.0 event schemas) |
| [DD-WORKFLOW-016](../architecture/decisions/DD-WORKFLOW-016-action-type-workflow-indexing.md) | Three-step discovery protocol, DS endpoints, HAPI tools |
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

