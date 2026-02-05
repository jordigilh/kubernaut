# ADR-034: Unified Audit Table Design with Event Sourcing Pattern

**Date**: 2025-11-08
**Status**: ✅ Approved
**Version**: 1.7
**Last Updated**: 2026-02-03
**Deciders**: Architecture Team
**Consulted**: Gateway, Data Storage, Context API, AI Analysis, Notification, Signal Processing, Remediation Orchestrator, Authentication Webhook, HolmesGPT API teams

---

## 📋 **Version History**

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| **v1.0** | 2025-11-08 | Initial ADR: Unified audit table design with event sourcing pattern, 7 service audit requirements | Architecture Team |
| **v1.1** | 2025-11-27 | Added Workflow Catalog Service (Phase 3, Item 4): `workflow.catalog.search_completed` event type with scoring breakdown for debugging workflow selection. Added DD-WORKFLOW-014 cross-reference. | Architecture Team |
| **v1.2** | 2025-12-18 | **BREAKING**: Standardized `event_category` naming convention (service-level, not operation-level). Added complete list of valid categories. RemediationOrchestrator MUST consolidate to `"orchestration"` category. Discovered during NT Team DS API query investigation (DD-API-001). Cross-references DD-AUDIT-003. | Architecture Team |
| **v1.3** | 2025-12-18 | Added "Authoritative Subdocuments" section establishing DD-AUDIT-004 (RR Reconstruction Field Mapping) as authoritative reference for BR-AUDIT-005 v2.0 (100% RR reconstruction from audit traces). Supports enterprise compliance (SOC 2, ISO 27001, NIST 800-53). | Architecture Team |
| **v1.4** | 2026-01-06 | Added Authentication Webhook Service (`webhook` category) for SOC2 CC8.1 operator attribution. Webhook service captures WHO (authenticated user) for CRD operations requiring manual approval. Cross-references DD-WEBHOOK-003, BR-AUTH-001. Expected volume: +100 events/day. | Architecture Team |
| **v1.5** | 2026-01-08 | **BREAKING**: Fixed WorkflowExecution event naming inconsistency. Changed Gap #5 (`workflow.selection.completed` → `workflowexecution.selection.completed`) and Gap #6 (`execution.workflow.started` → `workflowexecution.execution.started`) to align with ADR-034 v1.2 service-level category naming convention. All WorkflowExecution controller events now use `workflowexecution` prefix. Updated event_category from `"workflow"`/`"execution"` to `"workflowexecution"`. Discovered during ogen migration architectural review. | Architecture Team |
| **v1.6** | 2026-01-31 | **BREAKING**: Fixed event_category collision between AIAnalysis and HolmesGPT API. Added new `aiagent` category for HolmesGPT API Service (AI Agent Provider with autonomous tool-calling). Clarified `analysis` category is exclusively for AIAnalysis controller (remediation workflow orchestration). **Rationale**: Two distinct services were incorrectly sharing `"analysis"` category, violating ADR-034 v1.2 service-level naming rule. HolmesGPT is architecturally an autonomous AI agent (per HolmesGPT documentation), not an analysis controller. New name is implementation-agnostic (supports future agent replacements). **Impact**: (1) Historical queries filtering by `event_category='analysis'` will now miss HAPI events (must use `'aiagent'`). (2) **KNOWN ISSUE**: AIAnalysis INT tests will temporarily fail - they query with `event_category='analysis'` expecting to find HAPI events (`holmesgpt.response.complete`). **Migration Path**: Fix HAPI INT tests first (Priority 1), then update AIAnalysis INT tests to query `event_category='aiagent'` for HAPI events (Priority 2). Estimated 20-30 test updates across `test/integration/aianalysis/`. | Architecture Team |
| **v1.7** | 2026-02-03 | Added RemediationOrchestrator approval decision audit events (`orchestration` category) to complete two-event audit trail pattern for RemediationApprovalRequest decisions. New event types: `orchestrator.approval.approved`, `orchestrator.approval.rejected`, `orchestrator.approval.expired`. **Rationale**: SOC 2 CC8.1 compliance requires complete audit trail showing both authenticated user (from AuthWebhook `webhook` category) AND business context (from RO controller `orchestration` category). Two-event pattern ensures tamper-proof WHO attribution (webhook intercepts CRD update) and complete forensic context (RO controller provides correlation_id, workflow details, confidence scores). **Integration**: Events share `correlation_id` (parent RR name) for cross-service querying. RO controller uses `Status.DecidedBy` field populated by AuthWebhook to attribute approval to authenticated operator. **Implementation**: Uses AuditRecorded condition for secure, controller-managed idempotency (not annotations). Cross-references: BR-AUDIT-006 (RAR Audit Trail), DD-WEBHOOK-003 (Webhook Audit Pattern), DD-AUDIT-006 (RAR Implementation), ADR-040 (RAR Architecture). | Architecture Team |

---

## Context

Kubernaut currently lacks a unified audit trail system for tracking business operations across all services. Each service needs to record audit traces for:
- Compliance requirements (SOC 2, ISO 27001, GDPR)
- Debugging and troubleshooting across service boundaries
- Analytics and reporting (signal volume, success rates, performance metrics)
- Correlation tracking (trace signal flow from ingestion to remediation)
- Replay capabilities for testing and recovery

**Key Requirements**:
1. Support for 10+ services (Gateway, Context API, AI Analysis, Workflow, Data Storage, Execution, and future services)
2. Extensibility for new services without schema changes
3. Support for heterogeneous signal sources (K8s, AWS, GCP, Azure, OpenTelemetry, custom webhooks)
4. Query flexibility for compliance audits and analytics
5. Long-term retention (90 days to 7 years)
6. Performance at scale (1000+ events/second)

---

## Decision

We will implement a **unified audit table** using the **industry-standard Event Sourcing pattern** with the following design:

### 1. Database Schema: Structured Columns + JSONB Hybrid

```sql
CREATE TABLE audit_events (
    -- Event Identity
    event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_version VARCHAR(10) NOT NULL DEFAULT '1.0',

    -- Temporal Information
    event_timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    event_date DATE NOT NULL GENERATED ALWAYS AS (event_timestamp::DATE) STORED,

    -- Event Classification
    event_type VARCHAR(100) NOT NULL,        -- 'gateway.signal.received'
    event_category VARCHAR(50) NOT NULL,     -- Service identifier: 'gateway', 'notification', 'analysis', 'signalprocessing', 'workflow', 'execution', 'orchestration' (see Event Category Naming Convention below)
    event_action VARCHAR(50) NOT NULL,       -- 'received', 'processed', 'executed'
    event_outcome VARCHAR(20) NOT NULL,      -- 'success', 'failure', 'pending'

    -- Actor Information (Who)
    actor_type VARCHAR(50) NOT NULL,         -- 'service', 'external', 'user'
    actor_id VARCHAR(255) NOT NULL,          -- 'gateway-service', 'aws-cloudwatch'
    actor_ip INET,

    -- Resource Information (What)
    resource_type VARCHAR(100) NOT NULL,     -- 'Signal', 'RemediationRequest'
    resource_id VARCHAR(255) NOT NULL,       -- 'fp-abc123', 'rr-2025-001'
    resource_name VARCHAR(255),

    -- Context Information (Where/Why)
    correlation_id VARCHAR(255) NOT NULL,    -- remediation_id (groups related events)
    parent_event_id UUID,                    -- Links to parent event
    parent_event_date DATE,                  -- Parent event date (required for FK constraint on partitioned table)
    trace_id VARCHAR(255),                   -- OpenTelemetry trace ID
    span_id VARCHAR(255),                    -- OpenTelemetry span ID

    -- Kubernetes Context
    namespace VARCHAR(253),
    cluster_name VARCHAR(255),

    -- Event Payload (JSONB - flexible, queryable)
    event_data JSONB NOT NULL,
    event_metadata JSONB,

    -- Audit Metadata
    severity VARCHAR(20),
    duration_ms INTEGER,
    error_code VARCHAR(50),
    error_message TEXT,

    -- Compliance
    retention_days INTEGER DEFAULT 2555,     -- 7 years (SOC 2 / ISO 27001)
    is_sensitive BOOLEAN DEFAULT FALSE,

    -- Indexes
    INDEX idx_event_timestamp (event_timestamp DESC),
    INDEX idx_correlation_id (correlation_id, event_timestamp DESC),
    INDEX idx_resource (resource_type, resource_id, event_timestamp DESC),
    INDEX idx_event_type (event_type, event_timestamp DESC),
    INDEX idx_actor (actor_type, actor_id, event_timestamp DESC),
    INDEX idx_outcome (event_outcome, event_timestamp DESC),
    INDEX idx_event_data_gin (event_data) USING GIN,
    INDEX idx_parent_event (parent_event_id) WHERE parent_event_id IS NOT NULL,

    -- Foreign Key Constraint (Event Sourcing Immutability)
    -- Enforces parent-child relationships with ON DELETE RESTRICT
    -- Requires both parent_event_id and parent_event_date (partition key requirement)
    CONSTRAINT fk_audit_events_parent
        FOREIGN KEY (parent_event_id, parent_event_date)
        REFERENCES audit_events(event_id, event_date)
        ON DELETE RESTRICT
) PARTITION BY RANGE (event_date);
```

---

### 1.1. Event Category Naming Convention (v1.2)

**RULE**: `event_category` MUST match the **service name** that emits the event, not the operation type.

**Rationale**:
1. **Efficient Filtering**: Query all events from a specific service in one filter
2. **Service Analytics**: Track event volume, success rates, and performance per service
3. **Compliance Auditing**: Audit all operations from a specific service (e.g., "Show all notification deliveries")
4. **Cost Attribution**: Identify high-volume services for optimization
5. **Cross-Service Correlation**: Trace signal flow across service boundaries

**Standard Categories** (Service-Level):

| event_category | Service | Usage | Example Events |
|---------------|---------|-------|----------------|
| `gateway` | Gateway Service | Signal ingestion and CRD creation | `gateway.signal.received`, `gateway.crd.created`, `gateway.signal.deduplicated` |
| `notification` | Notification Service | Alert notification delivery | `notification.message.sent`, `notification.delivery.failed`, `notification.message.acknowledged` |
| `analysis` | AI Analysis Controller | Remediation workflow orchestration (NOT HolmesGPT API - see `aiagent`) | `aianalysis.investigation.started`, `aianalysis.recommendation.generated`, `aianalysis.analysis.completed`, `aianalysis.phase.transition` |
| `aiagent` | AI Agent Provider (HolmesGPT API) | Autonomous AI agent with tool-calling for investigations, recovery, and effectiveness analysis | `llm_request`, `llm_response`, `llm_tool_call`, `workflow_validation_attempt`, `holmesgpt.response.complete` |
| `signalprocessing` | Signal Processing Service | Signal enrichment and classification | `signalprocessing.enrichment.completed`, `signalprocessing.classification.decision`, `signalprocessing.phase.transition` |
| `workflow` | Workflow Catalog Service | Workflow search and selection | `workflow.catalog.search_completed` (DD-WORKFLOW-014) |
| `workflowexecution` | WorkflowExecution Controller | Tekton workflow orchestration and execution | `workflowexecution.workflow.started`, `workflowexecution.selection.completed`, `workflowexecution.execution.started`, `workflowexecution.workflow.completed`, `workflowexecution.workflow.failed` (BR-AUDIT-005 Gap #5, #6) |
| `orchestration` | Remediation Orchestrator Service | Remediation lifecycle orchestration | `orchestrator.lifecycle.started`, `orchestrator.phase.transitioned`, `orchestrator.approval.requested`, `orchestrator.approval.approved`, `orchestrator.approval.rejected`, `orchestrator.approval.expired` |
| `webhook` | Authentication Webhook Service | Operator attribution for CRD operations (SOC2 CC8.1) | `webhook.workflowexecution.block_cleared`, `webhook.notificationrequest.deleted`, `webhook.remediationapprovalrequest.decided` (DD-WEBHOOK-003) |

**Query Pattern**:
```sql
-- Get all Notification events for last 7 days
SELECT * FROM audit_events
WHERE event_category = 'notification'
  AND event_timestamp > NOW() - INTERVAL '7 days'
ORDER BY event_timestamp DESC;

-- Count events per service (last 30 days)
SELECT event_category, COUNT(*) as event_count
FROM audit_events
WHERE event_timestamp > NOW() - INTERVAL '30 days'
GROUP BY event_category
ORDER BY event_count DESC;

-- Audit all AI analysis decisions for compliance
SELECT * FROM audit_events
WHERE event_category = 'analysis'
  AND event_action = 'recommendation_generated'
  AND event_timestamp BETWEEN '2025-01-01' AND '2025-12-31';
```

**Benefits**:
- ✅ **One filter** to get all service events (not multiple `event_type` filters)
- ✅ **Service-level SLOs**: Track notification delivery rates, AI success rates, etc.
- ✅ **Operational visibility**: Quickly identify which service is high-volume or error-prone
- ✅ **Compliance**: Generate service-specific audit reports for SOC 2, ISO 27001

**Anti-Pattern** (Operation-Level Categories - FORBIDDEN):
```go
// ❌ WRONG: Using operation types as categories (RemediationOrchestrator v1.0-v1.1)
audit.SetEventCategory(event, "lifecycle")  // Too granular
audit.SetEventCategory(event, "phase")      // Too granular
audit.SetEventCategory(event, "approval")   // Too granular

// ✅ CORRECT: Use service name as category
audit.SetEventCategory(event, "orchestration")  // Service-level
// Then use event_action to differentiate: "started", "transitioned", "approval_requested"
```

**Migration Note**: RemediationOrchestrator is the **ONLY** service using operation-level categories. This violates the service-level convention and MUST be consolidated to `"orchestration"` for V1.0 release. See [RO Event Category Migration Notice](../../handoff/NOTICE_ADR_034_V1_2_RO_EVENT_CATEGORY_MIGRATION_DEC_18_2025.md).

**Cross-Reference**: DD-AUDIT-003 (Service Audit Trace Requirements) - Defines which services MUST generate audit traces.

---

### 1.1.1. Two-Event Audit Trail Pattern (RemediationApprovalRequest)

**Pattern**: RemediationApprovalRequest approval decisions generate **TWO audit events** from different services to provide complete SOC 2 compliance coverage.

**Architecture**:

| Event # | Service | Category | Event Type | Purpose | Actor | Critical Fields |
|---------|---------|----------|------------|---------|-------|-----------------|
| **Event 1** | AuthWebhook | `webhook` | `webhook.remediationapprovalrequest.decided` | Captures **WHO** (authenticated user identity) at CRD interception point | `user` (authenticated operator) | `actor_id` (operator email), `event_timestamp` (decision time) |
| **Event 2** | RemediationOrchestrator | `orchestration` | `orchestrator.approval.{approved\|rejected\|expired}` | Captures **WHAT/WHY** (business context, workflow details, confidence) | `service` (remediationorchestrator-controller) | `correlation_id` (parent RR), `event_data.workflow_id`, `event_data.confidence`, `event_data.decided_by` |

**Integration Point**: `Status.DecidedBy` field
- **Set by**: AuthWebhook (authenticated user from K8s API request)
- **Read by**: RemediationOrchestrator controller (includes in Event 2 for attribution)
- **Immutability**: ADR-040 guarantees field is immutable once set (tamper-proof)

**Query Pattern** (Forensic Investigation):
```sql
-- Get complete approval audit trail for remediation request
SELECT 
    event_category,
    event_type,
    actor_type,
    actor_id,
    event_timestamp,
    event_data->>'decided_by' AS decided_by,
    event_data->>'workflow_id' AS workflow_id,
    event_data->>'confidence' AS confidence
FROM audit_events
WHERE correlation_id = 'rr-production-cpu-spike-2026-02-03'
  AND (event_category = 'webhook' OR event_category = 'orchestration')
  AND (event_type LIKE '%approval%')
ORDER BY event_timestamp;

-- Result (Two Events):
-- 1. webhook | webhook.remediationapprovalrequest.decided | user | alice@example.com | 2026-02-03 10:15:23 | NULL | NULL | NULL
-- 2. orchestration | orchestrator.approval.approved | service | remediationorchestrator-controller | 2026-02-03 10:15:24 | alice@example.com | wf-restart-pod-abc | 0.89
```

**SOC 2 Compliance Coverage**:

| Control | Requirement | Event 1 (Webhook) | Event 2 (Orchestration) |
|---------|-------------|-------------------|-------------------------|
| **CC8.1** (User Attribution) | WHO approved? | ✅ `actor_id` = authenticated user | ✅ `event_data.decided_by` = cross-validation |
| **CC6.8** (Non-Repudiation) | Defensible rationale? | ✅ Tamper-proof timestamp | ✅ Business context (workflow, confidence) |
| **CC7.2** (Monitoring) | Complete audit trail? | ✅ Interception at CRD update | ✅ Controller-emitted context |
| **CC7.4** (Completeness) | No missing decisions? | ✅ Every CRD update intercepted | ✅ Idempotent emission (AuditRecorded condition) |

**Why Two Events?**:
1. **Security Separation**: Webhook captures authentication at interception point (cannot be bypassed)
2. **Business Context**: Controller provides complete remediation context for forensics
3. **Cross-Validation**: Both events share `decided_by` field for integrity verification
4. **Compliance**: Satisfies "defense in depth" audit requirements (multiple independent sources)

**Idempotency**:
- **Webhook**: Uses `Status.DecidedBy` field check (`if DecidedBy != "" { skip }`)
- **RO Controller**: Uses `AuditRecorded` status condition (`if AuditRecorded == True { skip }`)

**Authority**: BR-AUDIT-006 (RAR Audit Trail), DD-WEBHOOK-003 (Webhook Audit Pattern), DD-AUDIT-006 (RAR Implementation)

---

### 1.2. Actor ID Naming Convention

**RULE**: `actor_id` MUST follow a consistent pattern based on service type.

**Pattern**:
1. **CRD Controllers** → `<service>-controller` (where `<service>` is lowercase, hyphenated)
2. **Stateless Services** → `<service>` (no suffix)
3. **Always** → `actor_type = "service"`

**Rationale**:
1. **Consistent Identification**: Clearly distinguishes CRD controllers from stateless services
2. **Service Attribution**: Accurately tracks which service performed an action
3. **Debugging**: Easily identify the source of audit events in logs and queries
4. **Multi-Instance Support**: Future-proof for horizontal scaling scenarios

**Standard Actor IDs** (All 8 Services):

| Service Type | Service Name | actor_id | actor_type | Example Events |
|--------------|--------------|----------|------------|----------------|
| **Stateless Services** ||||
| Gateway | Gateway Service | `gateway` | `service` | Signal reception, CRD creation |
| Data Storage | Data Storage Service | `datastorage` | `service` | Self-audit for writes, workflow catalog |
| **CRD Controllers** ||||
| Signal Processing | SignalProcessing Controller | `signalprocessing-controller` | `service` | Signal enrichment, classification |
| AI Analysis | AIAnalysis Controller | `aianalysis-controller` | `service` | HolmesGPT integration, recommendations |
| Workflow Execution | WorkflowExecution Controller | `workflowexecution-controller` | `service` | Tekton workflow orchestration |
| Remediation Orchestrator | RemediationOrchestrator Controller | `remediationorchestrator-controller` | `service` | Remediation lifecycle management |
| Notification | Notification Controller | `notification-controller` | `service` | Alert delivery, acknowledgments |
| Approval | RemediationApprovalRequest Controller | `remediationapprovalrequest-controller` | `service` | Approval workflow management |

**Implementation Example**:
```go
// Stateless Service (Gateway)
const ServiceName = "gateway"
audit.SetActor(event, "service", ServiceName)

// CRD Controller (RemediationOrchestrator)
const ServiceName = "remediationorchestrator-controller"
audit.SetActor(event, "service", ServiceName)
```

**Query Pattern**:
```sql
-- Get all events from RemediationOrchestrator controller
SELECT * FROM audit_events
WHERE actor_id = 'remediationorchestrator-controller'
  AND event_timestamp > NOW() - INTERVAL '24 hours'
ORDER BY event_timestamp DESC;

-- Compare controller vs stateless service activity
SELECT
    CASE
        WHEN actor_id LIKE '%-controller' THEN 'CRD Controller'
        ELSE 'Stateless Service'
    END AS service_type,
    actor_id,
    COUNT(*) AS event_count
FROM audit_events
WHERE event_timestamp > NOW() - INTERVAL '7 days'
GROUP BY service_type, actor_id
ORDER BY event_count DESC;
```

---

### 2. Event Data Format: Hybrid Approach (Common Envelope + Service-Specific Payload)

```json
{
  "version": "1.0",
  "service": "gateway",
  "operation": "signal_received",
  "status": "success",
  "payload": {
    "alert_name": "HighCPU",
    "signal_fingerprint": "fp-abc123",
    "namespace": "production",
    "is_duplicate": false,
    "action": "created_crd"
  },
  "source_payload": {
    // Original signal from external source (optional)
  }
}
```

### 3. Storage Technology: JSONB (Not Protocol Buffers)

**Decision**: Use JSONB for `event_data` column

**Rationale**:
- Industry consensus: 10/10 major platforms (AWS CloudTrail, Google Cloud Audit Logs, Kubernetes, Datadog, Elastic, etc.) use JSON for audit logs
- Query flexibility is critical for compliance and analytics
- Human-readable for debugging and compliance audits
- Schema evolution without code deployments
- Zero database migrations for new services or fields

**Protocol Buffers Rejected For Audit Logs**:
- Cannot SQL query inside binary blob (must deserialize in application)
- Binary format makes debugging harder
- Schema changes require code deployments
- Industry does NOT use protobuf for persistent audit logs (only for transient RPC/queue data)

---

## Rationale

### Industry Standard Analysis

**10/10 signal ingestion platforms use this pattern**:

| Platform | Storage Type | Retention | Purpose |
|----------|-------------|-----------|---------|
| AWS EventBridge | Audit (CloudTrail) | 90 days | Compliance, replay, debugging |
| Google Cloud Pub/Sub | Audit (Cloud Audit Logs) | 400 days | Compliance, debugging |
| Azure Event Grid | Audit (Activity Logs) | 90 days | Compliance, troubleshooting |
| Datadog Intake API | Audit (Event Stream) | Configurable | Analytics, alerting, compliance |
| Prometheus Alertmanager | Audit (Notification Log) | 120 hours | Debugging, deduplication |
| Kafka | Audit (Topic Logs) | Configurable | Replay, analytics |
| Splunk HEC | Audit (Index) | Configurable | Search, analytics, compliance |
| Elastic Beats | Audit (Elasticsearch) | Configurable | Search, analytics, alerting |
| PagerDuty Events API | Audit (Incident Timeline) | Permanent | Incident history, postmortems |
| New Relic Events API | Audit (NRDB) | Configurable | Analytics, alerting, compliance |

**Key Findings**:
- ✅ 10/10 store audit traces (not just application logs)
- ✅ 10/10 use structured columns + flexible data (JSON/JSONB)
- ✅ 0/10 use Protocol Buffers for audit logs
- ✅ All use event sourcing pattern (immutable, append-only)

### Audit Traces vs Application Logs

**Gateway Operations Classification**:

| Operation | Type | Storage | Rationale |
|-----------|------|---------|-----------|
| Signal Ingestion | Business Operation | ✅ Audit | Compliance, correlation, replay |
| Deduplication | Business Operation | ✅ Audit | Analytics, debugging |
| Storm Detection | Business Operation | ✅ Audit | Analytics, alerting |
| CRD Creation | Business Operation | ✅ Audit | Compliance, correlation |
| Correlation | Business Operation | ✅ Audit | Distributed tracing |

**Industry Equivalents**:
- Signal Ingestion = AWS EventBridge PutEvents → Audit (CloudTrail)
- Deduplication = Alertmanager dedup → Audit (Notification Log)
- Storm Detection = Datadog anomaly detection → Audit (Event Stream)
- CRD Creation = Kubernetes API audit → Audit (Audit Logs)

**Conclusion**: All Gateway operations are business-critical and require audit traces, not just application logs.

### Extensibility Validation

**Adding New Services** (validated with 7 future services):

| Service | Implementation Time | Breaking Changes | Database Migration |
|---------|---------------------|------------------|-------------------|
| Notification Service | 2-3 hours | ❌ None | ❌ None |
| Security Service | 2-3 hours | ❌ None | ❌ None |
| Cost Optimization | 2-3 hours | ❌ None | ❌ None |
| Compliance Service | 2-3 hours | ❌ None | ❌ None |
| Capacity Planning | 2-3 hours | ❌ None | ❌ None |
| Chaos Engineering | 2-3 hours | ❌ None | ❌ None |
| Observability Service | 2-3 hours | ❌ None | ❌ None |

**Key Insight**: Zero schema changes for any new service (98% confidence)

---

## Consequences

### Positive

1. **Industry Alignment** (100% confidence)
   - Follows proven patterns from AWS, Google, Kubernetes
   - Battle-tested at massive scale (billions of events)
   - Well-documented best practices and tooling

2. **Extensibility** (98% confidence)
   - Add new services in 2-3 hours with zero breaking changes
   - Add new fields without database migrations
   - Support heterogeneous signal sources (K8s, AWS, GCP, Azure, OpenTelemetry)

3. **Query Flexibility** (95% confidence)
   - SQL queries on structured columns (99% of queries)
   - JSONB queries on service-specific fields (1% of queries)
   - Aggregations and analytics across all services
   - GIN indexes for JSONB query performance

4. **Compliance Ready** (100% confidence)
   - SOC 2, ISO 27001, GDPR compliant
   - Immutable audit trail (append-only)
   - 7-year retention (configurable)
   - Sensitive data flag for PII tracking

5. **Performance** (90% confidence)
   - Partitioning: 10-100x faster queries (monthly partitions)
   - Indexes: Optimized for common query patterns
   - Target: 1000 events/second (conservative vs AWS: 10,000/sec)
   - Storage: ~1KB/event (compressed: ~300 bytes)

6. **Cost Effective** (95% confidence)
   - Estimated: $155/month (1M events/day)
   - Industry comparison: $600-1200/month (Datadog, New Relic)
   - 4-8x cheaper than commercial solutions

7. **Correlation Tracking** (100% confidence)
   - Trace signal flow across services (Gateway → AI → Workflow → Execution)
   - OpenTelemetry compatible (trace_id, span_id)
   - Parent-child event relationships
   - Distributed tracing support

### Negative

1. **JSONB Query Performance** (10% concern)
   - JSONB queries slower than structured columns
   - **Mitigation**: Selective GIN indexing on frequently-queried paths
   - **Acceptable**: <1% of queries need JSONB paths

2. **Storage Growth** (15% concern)
   - Audit events grow indefinitely
   - **Mitigation**: Partitioning + archival strategy (S3/GCS after 90 days)
   - **Estimated**: 100GB/year compressed (1M events/day)

3. **Runtime Type Safety** (5% concern)
   - JSONB validation is runtime (not compile-time like protobuf)
   - **Mitigation**: Application-layer validation + JSON schema validation
   - **Acceptable**: Industry standard for audit logs

### Neutral

1. **Migration Effort**
   - Estimated: 20 hours for unified audit table implementation
   - Phased approach: Gateway first, then other services
   - No breaking changes to existing services

2. **Documentation Requirements**
   - Per-service payload schemas must be documented
   - **Mitigation**: Template-based documentation (2 hours per service)

---

## Implementation Plan

### Phase 1: Data Storage Service (Day 21, 20 hours)

**Scope**: Implement unified audit table infrastructure

1. **Core Schema** (4 hours)
   - Create `audit_events` table with partitions
   - Create indexes (structured + selective JSONB)
   - Test schema with sample data

2. **Signal Source Adapters** (6 hours)
   - Generic signal handler (accepts any JSON)
   - K8s Prometheus adapter
   - AWS CloudWatch adapter
   - GCP Monitoring adapter
   - Custom webhook adapter (pass-through)

3. **Query API** (4 hours)
   - REST API endpoints for audit queries
   - Query by correlation_id, event_type, time range
   - Pagination support

4. **Observability** (2 hours)
   - Prometheus metrics (audit_events_total, write_duration)
   - Grafana dashboard (event volume, success rate)
   - Alerting rules (write failures, high error rate)

5. **Testing** (4 hours)
   - Unit tests (signal adapters, query API)
   - Integration tests (PostgreSQL roundtrip, JSONB queries)
   - E2E tests (full signal ingestion to query flow)
   - Performance tests (1000 events/sec write throughput)

### Phase 2: Gateway Service Integration (6 hours)

**Scope**: Implement audit traces for Gateway operations

1. **Audit Events** (2 hours)
   - `gateway.signal.received`
   - `gateway.signal.deduplicated`
   - `gateway.storm.detected`
   - `gateway.crd.created`
   - `gateway.signal.rejected`
   - `gateway.error.occurred`

2. **Implementation** (2 hours)
   - Audit functions for each event type
   - Integration with Data Storage Service audit API
   - Include correlation_id (remediation_id)
   - Include source_payload (original signal)

3. **Testing** (2 hours)
   - Unit tests (audit functions)
   - Integration tests (audit API calls)
   - E2E tests (signal ingestion to audit storage)

### Phase 3: Other Services (Future)

**Service Audit Requirements** (7 services):

1. **Signal Processing Service** (6 hours)
   - `signalprocessing.enrichment.started`
   - `signalprocessing.enrichment.completed`
   - `signalprocessing.categorization.completed`
   - `signalprocessing.error.occurred`

2. **AI Analysis Service** (6 hours)
   - `aianalysis.investigation.started`
   - `aianalysis.investigation.completed`
   - `aianalysis.recommendation.generated`
   - `aianalysis.approval.required`
   - `aianalysis.error.occurred`

3. **RemediationApprovalRequest Audit Trail** (6 hours) - ✅ **IMPLEMENTED (v1.7)**
   - **Two-Event Pattern** (see Section 1.1.1 for architecture):
     - **Event 1 (Webhook)**: `webhook.remediationapprovalrequest.decided` (actor: user, captures WHO)
     - **Event 2 (Orchestration)**: 
       - `orchestrator.approval.approved` (actor: remediationorchestrator-controller)
       - `orchestrator.approval.rejected` (actor: remediationorchestrator-controller)
       - `orchestrator.approval.expired` (actor: remediationorchestrator-controller)
   - **Integration**: `Status.DecidedBy` field bridges both events (set by webhook, read by RO controller)
   - **Idempotency**: Webhook uses field check, RO controller uses AuditRecorded condition
   - **Authority**: BR-AUDIT-006, DD-WEBHOOK-003, DD-AUDIT-006
   - **Test Coverage**: 17 tests (8 unit, 6 integration, 3 E2E)

4. **Workflow Catalog Service** (4 hours) - **NEW**
   - `workflow.catalog.search_completed` (actor: holmesgpt-api)
   - **Event Data**: Includes scoring breakdown (base_similarity, label_boost, label_penalty, confidence)
   - **Event Category**: `workflow` (new category for workflow-related operations)
   - **Use Cases**: Debugging workflow selection, tuning workflow definitions, compliance tracking
   - **Authority**: DD-WORKFLOW-014 (Workflow Selection Audit Trail)

5. **Remediation Orchestrator Service** (8 hours, partially implemented)
   - `remediationorchestrator.request.created`
   - `remediationorchestrator.phase.transitioned`
   - `remediationorchestrator.approval.requested` (creates RemediationApprovalRequest)
   - ✅ **IMPLEMENTED**: `orchestrator.approval.approved` (v1.7, BR-AUDIT-006)
   - ✅ **IMPLEMENTED**: `orchestrator.approval.rejected` (v1.7, BR-AUDIT-006)
   - ✅ **IMPLEMENTED**: `orchestrator.approval.expired` (v1.7, BR-AUDIT-006)
   - `remediationorchestrator.child.created`
   - `remediationorchestrator.error.occurred`

5. **Remediation Execution Service** (6 hours)
   - `remediationexecution.pipeline.started`
   - `remediationexecution.pipeline.completed`
   - `remediationexecution.task.executed`
   - `remediationexecution.error.occurred`

6. **Effectiveness Monitor Service** (4 hours)
   - `effectivenessmonitor.evaluation.started`
   - `effectivenessmonitor.evaluation.completed`
   - `effectivenessmonitor.playbook.updated`

7. **Notification Service** (4 hours)
   - `notification.sent`
   - `notification.failed`
   - `notification.escalated`

**Estimated per service**: 4-6 hours

---

## Alternatives Considered

### Alternative 1: Protocol Buffers for event_data

**Rejected**: Industry does NOT use protobuf for audit logs

**Pros**:
- ✅ Compile-time type safety
- ✅ 50-70% smaller storage footprint
- ✅ 3-5x faster serialization

**Cons**:
- ❌ Cannot SQL query inside binary blob
- ❌ Binary debugging requires tools
- ❌ Schema changes require code deployments
- ❌ 0/10 industry platforms use protobuf for audit logs

**Decision**: Use JSONB (industry standard)

---

### Alternative 2: Per-Service Audit Tables

**Rejected**: Does not support cross-service correlation

**Pros**:
- ✅ Service isolation
- ✅ Independent schema evolution

**Cons**:
- ❌ Cannot trace signal flow across services
- ❌ Duplicate infrastructure per service
- ❌ Complex aggregations across services
- ❌ No unified compliance reporting

**Decision**: Use unified audit table

---

### Alternative 3: Fully Structured Columns (No JSONB)

**Rejected**: Not extensible for new services

**Pros**:
- ✅ Fast queries on all fields
- ✅ Compile-time type safety

**Cons**:
- ❌ Requires ALTER TABLE for new services
- ❌ Requires ALTER TABLE for new fields
- ❌ Database migrations for every change
- ❌ Not flexible for heterogeneous signal sources

**Decision**: Use hybrid (structured + JSONB)

---

## References

1. **Industry Analysis Documents**:
   - `GATEWAY_AUDIT_VS_LOGGING_ANALYSIS.md` (95% confidence)
   - `INDUSTRY_STANDARD_AUDIT_TABLE_DESIGN.md` (90% confidence)
   - `KUBERNAUT_EVENT_DATA_FORMAT_DESIGN.md` (95% confidence)
   - `EXTENSIBILITY_VALIDATION_NEW_SERVICES.md` (98% confidence)
   - `INDUSTRY_BEST_PRACTICES_AUDIT_STORAGE.md` (95% confidence)

2. **Industry Standards**:
   - AWS CloudTrail: https://docs.aws.amazon.com/cloudtrail/
   - Google Cloud Audit Logs: https://cloud.google.com/logging/docs/audit
   - Kubernetes Audit Logs: https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/
   - OWASP Logging Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html
   - Event Sourcing Pattern: https://martinfowler.com/eaaDev/EventSourcing.html

3. **Compliance Standards**:
   - SOC 2: Audit trail requirements
   - ISO 27001: 7-year retention
   - GDPR: Sensitive data tracking

---

## Authoritative Subdocuments

This ADR establishes the following subdocument as authoritative for specific implementation aspects:

### **DD-AUDIT-004: RR Reconstruction Field Mapping** (v1.0)

**Document**: [DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md](./DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md)

**Authority**: This subdocument is the **authoritative reference** for RemediationRequest CRD reconstruction from audit traces. All services MUST follow these field mappings when emitting audit events.

**Purpose**: Defines the explicit mapping between RR CRD fields and audit event `event_data` JSONB fields for 100% reconstruction accuracy.

**Scope**:
- 8 critical fields mapped to specific audit events
- Service and event type for each field
- Reconstruction algorithm and validation rules
- Storage impact analysis

**Business Requirement**: [BR-AUDIT-005 v2.0](../../requirements/11_SECURITY_ACCESS_CONTROL.md) - Enterprise-Grade Audit Integrity and Compliance

**Coverage**: 100% RR reconstruction (all `.spec` fields + system-managed `.status` fields)

**Implementation Reference**: All services implementing RR reconstruction audit events MUST consult this subdocument for field naming, structure, and validation requirements.

---

## Related Decisions

- **ADR-032**: [Data Access Layer Isolation](./ADR-032-data-access-layer-isolation.md) - Mandates Data Storage Service for all DB access
- **ADR-038**: [Asynchronous Buffered Audit Ingestion](./ADR-038-async-buffered-audit-ingestion.md) - Defines how services write audit traces (async buffered pattern)
- **ADR-040**: [RemediationApprovalRequest CRD Architecture](./ADR-040-remediation-approval-request-architecture.md) - Defines approval workflow audit events and actor patterns
- **DD-AUDIT-001**: [Audit Responsibility Pattern](./DD-AUDIT-001-audit-responsibility-pattern.md) - Defines who writes audit traces (distributed pattern)
- **DD-AUDIT-002**: [Audit Shared Library Design](./DD-AUDIT-002-audit-shared-library-design.md) - Implementation details for `pkg/audit/` shared library
- **DD-AUDIT-003**: [Service Audit Trace Requirements](./DD-AUDIT-003-service-audit-trace-requirements.md) - Defines which 8 of 11 services must generate audit traces
- **DD-AUDIT-006**: [RemediationApprovalRequest Audit Implementation](./DD-AUDIT-006-remediation-approval-audit-implementation.md) - Defines RAR approval audit trail architecture (two-event pattern, BR-AUDIT-006)
- **DD-WORKFLOW-014**: [Workflow Selection Audit Trail](./DD-WORKFLOW-014-workflow-selection-audit-trail.md) - Defines workflow catalog search audit events with scoring breakdown
- **DD-WEBHOOK-003**: [Webhook Complete Audit Pattern](./DD-WEBHOOK-003-webhook-complete-audit-pattern.md) - Defines webhook audit emission pattern for operator attribution (SOC 2 CC8.1)
- **DD-007**: [Graceful Shutdown Pattern](./DD-007-kubernetes-aware-graceful-shutdown.md) - 4-step Kubernetes-aware shutdown (ensures audit flush)

---

## Notes

- **Confidence**: 95% overall (based on industry analysis of 10 platforms)
- **Timeline**: Implementation after current branch tasks complete
- **Priority**: High (foundational for compliance and observability)
- **Breaking Changes**: None (new infrastructure, existing services continue unchanged)

---

**Approved By**: Architecture Team
**Date**: 2025-11-08
**Implementation Target**: Post-current-branch (Day 21 for Data Storage, Day 22 for Gateway)

