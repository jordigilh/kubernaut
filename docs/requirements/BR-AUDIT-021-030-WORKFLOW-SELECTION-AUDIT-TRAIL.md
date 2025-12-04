# BR-AUDIT-021-030: Workflow Selection Audit Trail

**Document Version**: 1.0
**Date**: November 2025
**Status**: ✅ APPROVED
**Category**: Audit & Compliance
**Related DDs**: DD-WORKFLOW-014, DD-WORKFLOW-002 v2.3, ADR-034, ADR-038

---

## 1. Purpose & Scope

### 1.1 Business Purpose

The Workflow Selection Audit Trail provides comprehensive tracking of all workflow catalog search operations, enabling operators to debug workflow selection decisions, tune workflow definitions, ensure compliance, and analyze workflow effectiveness patterns.

### 1.2 Scope

- **Workflow Search Audit**: Capture every workflow catalog search with full context
- **Scoring Transparency**: Include hybrid weighted scoring breakdown for debugging
- **Cross-Service Correlation**: Enable tracing from remediation request to workflow selection
- **Compliance Support**: Meet regulatory requirements for decision audit trails

### 1.3 Services Involved

| Service | Role | BR Coverage |
|---------|------|-------------|
| **HolmesGPT API** | Search initiator, passes remediation_id | BR-AUDIT-021, BR-AUDIT-022 |
| **Data Storage Service** | Audit event generator, search executor | BR-AUDIT-023 through BR-AUDIT-030 |

---

## 2. HolmesGPT API Audit Requirements

### 2.1 Remediation ID Propagation

#### BR-AUDIT-021: Mandatory Remediation ID Propagation

**Requirement**: HolmesGPT API MUST propagate `remediation_id` to Data Storage Service for every workflow catalog search request.

**Business Value**:
- Enables correlation of workflow selection with remediation lifecycle
- Supports end-to-end audit trail from signal detection to workflow execution
- Required for compliance reporting and debugging

**Acceptance Criteria**:
- [ ] `remediation_id` is included in JSON body of every `/api/v1/workflows/search` request
- [ ] `remediation_id` is passed through from AIAnalysis controller context
- [ ] Empty `remediation_id` is handled gracefully (search proceeds, audit has empty correlation)
- [ ] `remediation_id` is NOT used for search logic or workflow matching (correlation only)

**Implementation Reference**: DD-WORKFLOW-002 v2.3, DD-WORKFLOW-014 v2.1

#### BR-AUDIT-022: No Audit Generation in HolmesGPT API

**Requirement**: HolmesGPT API MUST NOT generate audit events for workflow searches. Audit generation is the responsibility of Data Storage Service.

**Business Value**:
- Single source of truth for audit events (Data Storage has richer context)
- Avoids duplicate audit events
- Simplifies HolmesGPT API responsibilities

**Acceptance Criteria**:
- [ ] HolmesGPT API does not call `/api/v1/audit/events` endpoint
- [ ] No audit-related code in workflow catalog toolset
- [ ] Only one HTTP call per search (to `/api/v1/workflows/search`)

**Implementation Reference**: DD-WORKFLOW-014 v2.0

---

## 3. Data Storage Service Audit Requirements

### 3.1 Audit Event Generation

#### BR-AUDIT-023: Workflow Search Audit Event Generation

**Requirement**: Data Storage Service MUST generate an audit event for every workflow catalog search operation.

**Business Value**:
- Complete audit trail for compliance and debugging
- Enables workflow effectiveness analysis
- Supports operator troubleshooting

**Acceptance Criteria**:
- [ ] Audit event generated after every successful search
- [ ] Audit event includes `remediation_id` as `correlation_id`
- [ ] Audit event type is `workflow.catalog.search_completed`
- [ ] Audit follows ADR-034 unified audit table schema

**Implementation Reference**: DD-WORKFLOW-014 v2.0, ADR-034

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

#### BR-AUDIT-025: Query Metadata Capture

**Requirement**: Audit events MUST capture complete query metadata for debugging and analysis.

**Business Value**:
- Enables debugging of "why wasn't my workflow selected?"
- Supports query pattern analysis
- Required for workflow tuning recommendations

**Acceptance Criteria**:
- [ ] Query text captured
- [ ] All filters captured (signal-type, severity, optional labels)
- [ ] `top_k` and `min_similarity` parameters captured
- [ ] Query timestamp captured

**Audit Event Schema (query section)**:
```json
{
  "query": {
    "text": "OOMKilled critical",
    "filters": {
      "signal-type": "OOMKilled",
      "severity": "critical",
      "resource-management": "gitops"
    },
    "top_k": 3,
    "min_similarity": 0.7
  }
}
```

#### BR-AUDIT-026: Scoring Breakdown Capture

**Requirement**: Audit events MUST capture complete scoring breakdown for each returned workflow.

**Business Value**:
- Enables debugging of workflow ranking decisions
- Supports workflow definition tuning
- Provides transparency into hybrid weighted scoring

**Acceptance Criteria**:
- [ ] `confidence` (final_score) captured for each workflow
- [ ] `base_similarity` captured (semantic match quality)
- [ ] `label_boost` captured (positive label matches)
- [ ] `label_penalty` captured (conflicting labels)
- [ ] `boost_breakdown` captured (which labels contributed boost)
- [ ] `penalty_breakdown` captured (which labels caused penalty)

**Audit Event Schema (scoring section)**:
```json
{
  "scoring": {
    "confidence": 0.95,
    "base_similarity": 0.88,
    "label_boost": 0.10,
    "label_penalty": 0.03,
    "boost_breakdown": {
      "resource-management": 0.10
    },
    "penalty_breakdown": {
      "environment": 0.03
    }
  }
}
```

**Implementation Reference**: DD-WORKFLOW-004, DD-WORKFLOW-014

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

#### BR-AUDIT-028: Search Metadata Capture

**Requirement**: Audit events MUST capture search execution metadata for performance analysis.

**Business Value**:
- Enables search performance monitoring
- Supports capacity planning
- Identifies slow queries for optimization

**Acceptance Criteria**:
- [ ] Total search duration captured (ms)
- [ ] Database query time captured (ms)
- [ ] Embedding generation time captured (ms)
- [ ] Index usage captured (HNSW, GIN)
- [ ] Cache hit/miss captured

**Audit Event Schema (search_metadata section)**:
```json
{
  "search_metadata": {
    "duration_ms": 45,
    "db_query_time_ms": 12,
    "embedding_generation_time_ms": 33,
    "embedding_dimensions": 768,
    "embedding_model": "all-mpnet-base-v2",
    "index_used": "workflow_embedding_idx",
    "cache_hit": false
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
- [ ] Query by `correlation_id` (remediation_id)
- [ ] Query by `workflow_id` (which remediations selected this workflow)
- [ ] Query by time range
- [ ] Query by event_type (`workflow.catalog.search_completed`)
- [ ] Aggregation support for analytics

**Example Queries**:
```sql
-- Find all workflow selections for a remediation
SELECT * FROM audit_events
WHERE event_type = 'workflow.catalog.search_completed'
  AND correlation_id = 'req-2025-11-27-abc123';

-- Find selection history for a specific workflow
SELECT * FROM audit_events
WHERE event_type = 'workflow.catalog.search_completed'
  AND event_data->'results'->'workflows' @> '[{"workflow_id": "pod-oom-gitops"}]';
```

---

## 4. Cross-Service Correlation Requirements

### 4.1 End-to-End Traceability

**Requirement**: The audit trail MUST enable end-to-end tracing from remediation request to workflow selection.

**Business Value**:
- Complete visibility into remediation lifecycle
- Supports debugging across service boundaries
- Required for compliance and forensics

**Correlation Chain**:
```
AIAnalysis Controller
  → generates remediation_id (kubernaut.ai/correlation-id label)
  → calls HolmesGPT API with remediation_id

HolmesGPT API
  → receives remediation_id in request context
  → passes remediation_id in JSON body to Data Storage

Data Storage Service
  → extracts remediation_id from request body
  → generates audit event with correlation_id = remediation_id
  → stores in audit_events table

Audit Query
  → query by correlation_id to see all events for a remediation
  → includes workflow selection, execution, and outcome
```

---

## 5. Success Metrics

### 5.1 Audit Coverage

| Metric | Target | Measurement |
|--------|--------|-------------|
| Audit event generation rate | 100% | Every search generates an audit event |
| Correlation ID presence | 100% | Every audit event has correlation_id |
| Scoring breakdown completeness | 100% | All scoring fields populated |

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
- [x] Pass `remediation_id` in JSON body (DD-WORKFLOW-014 v2.1)
- [x] Handle empty `remediation_id` gracefully
- [x] Update tests to verify no audit calls

### 6.2 Data Storage Service

- [ ] Add `remediation_id` field to `WorkflowSearchRequest` model
- [ ] Extract `remediation_id` in workflow search handler
- [ ] Build audit event with full workflow metadata
- [ ] Implement async buffered audit write (ADR-038)
- [ ] Add scoring breakdown calculation
- [ ] Add search metadata capture
- [ ] Add audit query API endpoints

### 6.3 Documentation

- [x] Create this BR document
- [x] Update DD-WORKFLOW-014 to v2.1
- [x] Update DD-WORKFLOW-002 to v2.3
- [ ] Create operator runbook for audit queries

---

## 7. Related Documents

| Document | Relationship |
|----------|--------------|
| [DD-WORKFLOW-014](../architecture/decisions/DD-WORKFLOW-014-workflow-selection-audit-trail.md) | Technical design for audit trail |
| [DD-WORKFLOW-002](../architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md) | MCP workflow catalog architecture |
| [DD-WORKFLOW-004](../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-scoring.md) | Hybrid weighted scoring algorithm |
| [ADR-034](../architecture/decisions/ADR-034-unified-audit-table-design.md) | Unified audit table schema |
| [ADR-038](../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) | Async buffered audit pattern |
| [11_SECURITY_ACCESS_CONTROL.md](./11_SECURITY_ACCESS_CONTROL.md) | Parent audit requirements (BR-AUDIT-001-020) |

---

## 8. Approval

| Role | Name | Date | Status |
|------|------|------|--------|
| Product Owner | - | 2025-11-27 | ✅ Approved |
| Technical Lead | - | 2025-11-27 | ✅ Approved |
| Security | - | 2025-11-27 | ✅ Approved |

---

*This document extends the audit requirements from 11_SECURITY_ACCESS_CONTROL.md (BR-AUDIT-001-020) with specific requirements for workflow selection audit trail. All implementations should align with these requirements to ensure comprehensive audit coverage for workflow catalog operations.*

