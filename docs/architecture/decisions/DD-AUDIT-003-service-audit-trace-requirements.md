# DD-AUDIT-003: Service Audit Trace Requirements

**Status**: ✅ **APPROVED** (Production Standard)
**Date**: November 8, 2025
**Last Reviewed**: July 31, 2026
**Version**: 2.6
**Confidence**: 95%
**Authority Level**: SYSTEM-WIDE - Defines audit requirements for all 14 services

**Recent Changes** (v2.6 - July 31, 2026):
- **Two-Tier Documentation Model**: Added an explicit [Document Map](#️-document-map-v26) codifying this DD as the policy tier (which services audit, at what priority, why) and per-service `AUDIT_EVENT_CATALOG.md` files as the detail tier (exact event strings, triggers, payloads). Slimmed the remaining 7 services' inline event tables (Gateway, Data Storage, Auth Webhook, Effectiveness Monitor, Signal Processing, Remediation Orchestrator, Notification) down to short summaries + catalog pointers, and fixed API Frontend's previously-duplicated inline catalog section (same drift risk, never cleaned up after the catalog was created).
- **Drift found during migration**: code-verified audits of all 7 services surfaced material drift from this DD's narrative, now corrected in each catalog — most notably Data Storage (10 of 11 documented events no longer exist; DS emits exactly 1 event today, `datastorage.ratelimit.denied`, after DD-WORKFLOW-018/019 retired the catalog tables and REST surface), Auth Webhook (15 real events vs. 11 documented), Signal Processing (wrong event-name prefix throughout: `signalprocessing.` not `signal-processing.`), and Remediation Orchestrator (`orchestrator.phase.transitioned` renamed to `orchestrator.lifecycle.transitioned`; `orchestrator.approval.expired` doesn't exist as a distinct type; `orchestrator.remediation.manual_review` is orphaned/unwired). See each catalog's "Known Gaps" section for full detail.
- **Authority**: QE readiness audit follow-up (#1799 cycle), user decision to keep the two-tier model and migrate all remaining services now

**Recent Changes** (v2.5 - July 31, 2026):
- **KA Event Catalog Handoff**: The [`v1.3 Update: Kubernaut Agent Audit Traces`](#v13-update-kubernaut-agent-audit-traces) section below is now **historical/frozen** at the 15 events it documented as of Issue #823 (v1.9, April 2026). KA has since grown to 37 `aiagent.*`/`workflow.*` event types (alignment, shadow-mode, fleet-federation, secret-access, interactive-mode incl. `aiagent.session.resumed`/`aiagent.interactive.k8s_call`, and workflow-catalog-discovery events) — see [`AUDIT_EVENT_CATALOG.md`](../../services/stateless/kubernaut-agent/security/AUDIT_EVENT_CATALOG.md) for the current, complete, and actively-maintained list. This DD remains authoritative for the system-wide "which services MUST/SHOULD/NO audit" decision and per-service summary table; it is no longer the source of truth for KA's individual event names.
- **Authority**: QE readiness audit follow-up (#1799 cycle); AGENTS.md ("New services must declare audit requirements before implementation")

**Recent Changes** (v2.4 - July 8, 2026):
- **Column Rename**: `audit_events.cluster_name` renamed to `cluster_id` (migration 014). The column was never populated by any shipped release; its actual intent was always the unique cluster identifier, not a non-unique display name. All per-service wiring references, `AuditEventRequest.ClusterID`, and `ReconstructionResponse.ClusterID` updated accordingly.
- **Authority**: DD-AUDIT-003 v2.2, SOC2 CC8.1, Issue #1651

**Recent Changes** (v2.3 - July 1, 2026):
- **Fleet Metadata Cache (FMC) Declaration**: FMC (Issue #54, ADR-068) was introduced without an explicit audit-scope declaration, leaving it absent from the service inventory below even though the codebase confirms it emits zero audit events (`grep -r "audit\|AuditEvent" pkg/fleet/fmc/ cmd/fleetmetadatacache/` returns no matches). Added as service #13 under "NO Audit Traces Needed": read-only scope-check/cluster-listing cache with no CRD writes, no external notifications, no LLM calls -- same category as Context API / Dynamic Toolset
- **Authority**: AGENTS.md ("New services must declare audit requirements before implementation"), Issue #54

**Recent Changes** (v2.2 - June 29, 2026):
- **Fleet Cluster-Scoped Audit Requirements**: Added new section documenting that `cluster_name` MUST be populated on every audit event originating from or targeting a remote cluster
- **Per-Service Wiring**: Documented ClusterID source and wiring point for all 8 audit-emitting services (GW, SP, RO, WE, EM, NT, AF, KA)
- **Reconstruction Impact**: `mapper.go` must map `cluster_name` to `ReconstructedRRFields.Spec.ClusterID`; `ReconstructionResponse` must include `cluster_name`
- **Authority**: BR-AUDIT-005 v2.0, ADR-065 (Multi-Cluster Federation), Issue #54
- **Controls**: SOC2 CC8.1 (complete reconstruction), FedRAMP AU-2 (audit events), AU-3 (structured payloads)

**Recent Changes** (v2.1 - May 23, 2026):
- **Auth Webhook**: Added RemediationWorkflow audit events per ADR-058 / Issue #1246:
  - `remediationworkflow.admitted.create` — RW CREATE admitted by webhook
  - `remediationworkflow.admitted.update` — RW UPDATE admitted by webhook
  - `remediationworkflow.admitted.delete` — RW DELETE admitted by webhook (sets CatalogStatus=Disabled)
  - `remediationworkflow.admitted.denied` — RW CREATE/UPDATE denied (validation/auth failure)
  - `authwebhook.workflow.registration_failed` — Startup reconciler: RW registration failed (graceful degradation)
- **Category**: `workflow` (per ADR-034 v1.8 event_category = business domain)
- **Implementation**: `pkg/authwebhook/remediationworkflow_audit.go`, `pkg/authwebhook/startup_reconciler.go`
- **Delivery**: Events batched via `pkg/audit.BufferedAuditStore` (non-blocking, best-effort)
- **Expected Volume**: +100 events/day (RW admission lifecycle + startup re-registration)
- **Authority**: ADR-058, Issue #1246 (graceful degradation), BR-WORKFLOW-007

**Recent Changes** (v2.0 - May 19, 2026):
- **API Frontend (AF)**: Added as P0 MUST service (8th P0, 13th overall) per Issue #1156 (SOC2 AU-2)
  - 30 `apifrontend.*` event types covering: auth (success/failure/denied), sessions (created/phase_changed/completed/auto_cancelled/retention_deleted), tools (executed), MCP (session_init/tool_failed), A2A (task_failed), triage (started/completed), severity (completed/failed), resilience (circuitbreaker.trip), config (reloaded/rejected), auth decorators (impersonation.created/jwt.delegation), remediation (rr.created/rr.deduplicated), user (decision), KA delegation (ka.delegated/ka.result_received), rate limiting (ratelimit.denied)
  - Uses shared `pkg/audit.BufferedAuditStore` with OpenAPI-typed `event_data` payloads (DD-AUDIT-004)
  - MCP session dedup guard: `mcp.session_init` fires once per new MCP session (Mcp-Session-Id header tracking)
  - Decorators wired in `cmd/apifrontend/main.go`: `AuditingJWTDelegationTransport`, `CircuitBreakerAuditFunc` (`AuditingDynamicFactory` removed per ADR-022; audit now at tool-callback level)
  - **Expected Volume**: +500 events/day (session lifecycle + tool execution + auth)
  - **Authority**: BR-AUDIT-005 v2.0 (SOC2 CC8.1, AU-2), Issue #1156

**Recent Changes** (v1.9 - April 24, 2026):
- **Kubernaut Agent (KA)**: Added 4 session lifecycle audit events per Issue #823 (PR 1.5):
  - `aiagent.session.started` — Investigation session started (StatusRunning)
  - `aiagent.session.cancelled` — Investigation session cancelled by operator
  - `aiagent.session.completed` — Investigation session completed successfully
  - `aiagent.session.failed` — Investigation session failed with error
  - `aiagent.investigation.cancelled` — Investigator detected context cancellation mid-investigation; carries phase, turn number for audit reconstruction
- **Authority**: BR-AUDIT-005 v2.0 (SOC2 CC8.1), Issue #823
- **Implementation**: Session lifecycle events emitted by `internal/kubernautagent/session/manager.go` (Manager-level via `StoreBestEffort`). Investigation cancellation event emitted by `internal/kubernautagent/investigator/investigator.go` (`emitCancellationAudit` via `StoreBestEffort` with `context.Background()` since caller context is already cancelled).
- **Expected Volume**: +200 events/day (session lifecycle tracking: 1 started + 1 terminal per investigation, plus ~5% investigation cancellation events)
- **Note**: Typed OpenAPI payloads deferred to follow-up DataStorage schema update; events use untyped `event_data` JSONB fallback

**Recent Changes** (v1.8 - March 25, 2026):
- **HolmesGPT API Service**: Added 2 Phase 2 enrichment audit events per Issue #533:
  - `aiagent.enrichment.completed` — Phase 2 enrichment succeeded (root_owner resolved, labels detected, history fetched)
  - `aiagent.enrichment.failed` — Phase 2 enrichment failed after retry exhaustion (captures reason, detail, affected_resource)
- **Authority**: BR-AUDIT-005 v2.0 (SOC2 CC8.1), #533, #529 (three-phase RCA architecture)
- **OpenAPI**: New `AIAgentEnrichmentCompletedPayload` and `AIAgentEnrichmentFailedPayload` schemas added to discriminated union

**Recent Changes** (v1.7 - March 9, 2026):
- **Auth Webhook**: Added as P0 service (12th service, 7th MUST) per BR-WORKFLOW-007 / Issue #300
  - `actiontype.admitted.create` — ActionType CREATE admitted by webhook
  - `actiontype.admitted.update` — ActionType UPDATE admitted by webhook
  - `actiontype.admitted.delete` — ActionType DELETE admitted by webhook (soft-disable)
  - `actiontype.denied.delete` — ActionType DELETE denied (active workflow dependencies)
- **Data Storage**: Added 5 ActionType catalog audit events per BR-WORKFLOW-007.4:
  - `datastorage.actiontype.created` — ActionType registered in catalog
  - `datastorage.actiontype.updated` — ActionType description updated
  - `datastorage.actiontype.disabled` — ActionType soft-disabled
  - `datastorage.actiontype.disable_denied` — Disable denied (active dependencies)
  - `datastorage.actiontype.reenabled` — Previously disabled ActionType re-enabled via CREATE
- **Expected Volume**: +200 events/day (ActionType lifecycle operations)
- **Authority**: BR-WORKFLOW-007.4 (SOC2 Audit Trail), DD-ACTIONTYPE-001, Issue #300

**Recent Changes** (v1.6 - March 2, 2026):
- **Remediation Orchestrator**: Added 3 Verifying phase audit events per Issue #280:
  - `orchestrator.lifecycle.verifying_started` — RR entered Verifying phase (EA assessment in progress)
  - `orchestrator.lifecycle.verification_completed` — EA reached terminal; Verifying → Completed
  - `orchestrator.lifecycle.verification_timed_out` — Verification deadline or safety-net expired
- **Phase Transition**: Updated `orchestrator.phase.transitioned` description to include Verifying phase
- **Expected Volume**: +300 events/day (verification tracking for successful remediations)
- **Authority**: Issue #280 (Verifying Phase for Duplicate RR Prevention)

**Recent Changes** (v1.5 - January 6, 2026):
- **ALL ERROR EVENTS**: Enhanced with standardized `error_details` field per BR-AUDIT-005 Gap #7
- **AI Analysis**: Added `aianalysis.analysis.failed` event for Holmes API failures (broader than `ai-analysis.llm.request_failed`)
- **Workflow Execution**: Enhanced `workflow.failed` event with Tekton pipeline error details
- **Gateway**: Enhanced `gateway.crd.creation_failed` event with K8s error details
- **Remediation Orchestrator**: Enhanced `orchestrator.lifecycle.completed` (failure) with orchestration error details
- **ErrorDetails Structure**: `{message, code, component, retry_possible, stack_trace?}` - see DD-ERROR-001
- **Error Code Taxonomy**: `ERR_INVALID_*`, `ERR_K8S_*`, `ERR_UPSTREAM_*`, `ERR_INTERNAL_*`, `ERR_LIMIT_*`, `ERR_TIMEOUT_*`
- **Rationale**: SOC2 Type II compliance requires standardized error capture for RR reconstruction
- **Expected Volume**: No change in event count, enhanced event data only
- **Authority**: BR-AUDIT-005 v2.0 Gap #7, DD-ERROR-001 (Error Details Standardization)

**Recent Changes** (v1.4 - January 4, 2026):
- **Workflow Execution**: Added `workflowexecution.block.cleared` event for SOC2 CC8.1 (operator attribution)
- **Notification Service**: Added `notification.request.cancelled` event for SOC2 CC8.1 (operator attribution)
- **Rationale**: SOC2 Type II compliance requires operator attribution for all critical manual actions
- **Expected Volume**: +50 events/day (operator action tracking)
- **Authority**: BR-WE-013 (block clearance audit), DD-WEBHOOK-001 (webhook requirements)

**Recent Changes** (v1.3 - January 4, 2026):
- **AI Analysis**: Added `aianalysis.analysis.completed` event for SOC2 compliance (BR-AUDIT-005 v2.0)
- **Workflow Execution**: Added `workflow.selection.completed` event for RR reconstruction (DD-AUDIT-004)
- **Rationale**: 100% RR CRD reconstruction from audit traces (enterprise compliance)
- **Expected Volume**: +300 events/day (workflow selection tracking)

**Recent Changes** (v1.2 - December 17, 2025):
- **Gateway**: Removed deprecated `gateway.signal.storm_detected` event (storm detection feature removed per DD-GATEWAY-015)
- **Remediation Orchestrator**: Added `orchestrator.routing.blocked` event (routing decisions audit coverage)
- **Remediation Orchestrator**: Added approval lifecycle events (requested, approved, rejected, expired)
- **Remediation Orchestrator**: Added manual review event
- **Remediation Orchestrator**: Updated expected volume: 1,000 → 1,200 events/day
- **Data Storage**: Removed meta-auditing events per DD-AUDIT-002 V2.0.1 (audit writes no longer audited)

---

## 🗺️ **Document Map (v2.6)**

This DD and the per-service `AUDIT_EVENT_CATALOG.md` files form a **two-tier model** — each tier has a distinct, non-overlapping responsibility:

| Tier | Document | Answers | Owned by |
|------|----------|---------|----------|
| **Policy** | This DD (DD-AUDIT-003) | *Which* services audit, at what priority (MUST/SHOULD/NO), and why (compliance/business rationale, industry precedent, volume/cost estimates) | Architecture team, reviewed annually |
| **Detail** | `docs/services/**/security/AUDIT_EVENT_CATALOG.md` (one per audit-emitting service) | *What* events exist today — exact event-type strings, trigger conditions, payload fields, emission call sites, known gaps/drift vs. this DD | Service owners, updated with every audit-event code change |

**Rule of thumb**: if you're asking "should this service audit at all, and at what priority" → read this DD. If you're asking "what does event X actually contain, and where is it emitted" → read that service's catalog. Per-service sections below intentionally keep only a short summary and rationale; they link to the catalog for the current, code-verified event list rather than duplicating it inline. When the two disagree, **the catalog wins** — it is the one regenerated against source code, not narrative prose.

**Current catalogs**: [Gateway](../../services/stateless/gateway-service/security/AUDIT_EVENT_CATALOG.md) · [Data Storage](../../services/stateless/data-storage/security/AUDIT_EVENT_CATALOG.md) · [Auth Webhook](../../services/shared/authentication-webhook/security/AUDIT_EVENT_CATALOG.md) · [Effectiveness Monitor](../../services/stateless/effectiveness-monitor/security/AUDIT_EVENT_CATALOG.md) · [Signal Processing](../../services/crd-controllers/01-signalprocessing/security/AUDIT_EVENT_CATALOG.md) · [Remediation Orchestrator](../../services/crd-controllers/05-remediationorchestrator/security/AUDIT_EVENT_CATALOG.md) · [Notification](../../services/crd-controllers/06-notification/security/AUDIT_EVENT_CATALOG.md) · [Kubernaut Agent](../../services/stateless/kubernaut-agent/security/AUDIT_EVENT_CATALOG.md) · [API Frontend](../../services/apifrontend/security/AUDIT_EVENT_CATALOG.md)

---

## 🎯 **Overview**

This design decision establishes **which services MUST generate audit traces** and **which services should NOT**, providing clear guidance for audit implementation across all Kubernaut services.

**Key Principle**: Audit traces are mandatory for business-critical operations, compliance requirements, and state-changing operations. Read-only and configuration services use application logs instead.

**Scope**: All 12 Kubernaut services (4 CRD controllers + 8 stateless services).

**Decision Summary**:
- ✅ **7 services MUST** generate audit traces (P0 - business-critical)
- ✅ **2 services SHOULD** generate audit traces (P1 - operational visibility)
- ⚠️ **3 services NO audit** traces needed (read-only/configuration)

---

## 📋 **Table of Contents**

1. [Context & Problem](#context--problem)
2. [Requirements](#requirements)
3. [Decision](#decision)
4. [Service-by-Service Analysis](#service-by-service-analysis)
5. [Implementation Priority](#implementation-priority)
6. [Audit Event Volume Estimates](#audit-event-volume-estimates)
7. [Industry Precedents](#industry-precedents)
8. [Related Decisions](#related-decisions)

---

## 🎯 **Context & Problem**

### **Challenge**

Kubernaut consists of 12 microservices with different responsibilities. Not all services require audit traces:

1. ⚠️ **Over-Auditing Risk**: Auditing read-only operations wastes storage and degrades performance
2. ⚠️ **Under-Auditing Risk**: Missing audit traces for business-critical operations violates compliance
3. ⚠️ **Inconsistent Standards**: No clear guidance on which services should audit
4. ⚠️ **Cost Impact**: Unnecessary audit traces increase storage costs

### **Business Impact**

- **Compliance**: SOC 2, ISO 27001, GDPR require audit trails for specific operations
- **Debugging**: Audit traces critical for troubleshooting business-critical failures
- **Cost Optimization**: Avoiding unnecessary audit traces reduces storage costs
- **Performance**: Over-auditing can impact service performance

### **Key Question**

**Which of the 12 Kubernaut services should generate audit traces?**

---

## 📋 **Requirements**

### **Audit Trace Decision Criteria**

| Criterion | Description | Weight |
|-----------|-------------|--------|
| **Business-Critical Operations** | Creates/modifies/deletes business resources | ⭐⭐⭐⭐⭐ |
| **Compliance Requirements** | SOC 2, ISO 27001, GDPR audit trail | ⭐⭐⭐⭐⭐ |
| **External Interactions** | Receives external signals, sends external notifications | ⭐⭐⭐⭐ |
| **State Changes** | Modifies CRDs, database records, or external systems | ⭐⭐⭐⭐ |
| **Debugging Value** | Critical for troubleshooting and root cause analysis | ⭐⭐⭐ |
| **Performance Impact** | High-volume operations that need monitoring | ⭐⭐⭐ |

### **Service Inventory**

**Stateless Services** (8):
1. Gateway Service
2. Data Storage Service
3. Auth Webhook Service
4. Context API Service
5. HolmesGPT API Service
6. Dynamic Toolset Service
7. Notification Service
8. Effectiveness Monitor Service

**CRD Controllers** (4):
8. Signal Processing Controller
9. AI Analysis Controller
10. Remediation Execution Controller
11. Remediation Orchestrator Controller

---

## ✅ **Decision**

**APPROVED**: **9 out of 12 services** generate audit traces

**Breakdown**:
- ✅ **7 services MUST** (P0): Gateway, AI Analysis, Remediation Execution, Notification, Data Storage, Auth Webhook, Effectiveness Monitor
- ✅ **2 services SHOULD** (P1): Signal Processing, Remediation Orchestrator
- ⚠️ **3 services NO audit**: Context API, HolmesGPT API, Dynamic Toolset

**Rationale**:
1. **Compliance Alignment**: P0 services meet SOC 2, ISO 27001, GDPR requirements
2. **Industry Standards**: Matches audit patterns from AWS, Google, Kubernetes
3. **Cost Optimization**: Avoids auditing read-only operations (industry standard)
4. **Performance**: Minimizes audit overhead while maintaining compliance

---

## 📊 **Service-by-Service Analysis**

### **P0: MUST Generate Audit Traces (7 Services)**

---

#### 1. Gateway Service ⭐⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Business-Critical**: Signal ingestion is the entry point for all remediations
- ✅ **Compliance**: External signal ingestion requires audit trail (SOC 2, ISO 27001)
- ✅ **External Interactions**: Receives Prometheus alerts, K8s events, custom webhooks
- ✅ **State Changes**: Creates `RemediationRequest` CRDs
- ✅ **Debugging Value**: Critical for tracing signal flow
- ✅ **Performance Impact**: High-volume (1000+ signals/day)

**Audit Events**: 6 event types covering signal ingestion (`gateway.signal.*`), RemediationRequest CRD lifecycle (`gateway.crd.*`), and hot-reload config (`gateway.config.*`, GAP-11/Issue #1505) — see [AUDIT_EVENT_CATALOG.md](../../services/stateless/gateway-service/security/AUDIT_EVENT_CATALOG.md) for the current, code-verified list, triggers, and payload fields. Note: the actual event is `gateway.crd.failed`, not `gateway.crd.creation_failed`.

**Industry Precedent**: AWS EventBridge, Google Cloud Pub/Sub, Azure Event Grid

**Expected Volume**: 1,000 events/day, 30 MB/month

---

#### 2. AI Analysis Controller ⭐⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Business-Critical**: AI recommendations drive remediation actions
- ✅ **Compliance**: AI decision-making requires audit trail (AI Act, SOC 2)
- ✅ **External Interactions**: Calls HolmesGPT API (external LLM)
- ✅ **State Changes**: Updates `AIAnalysis` CRD with recommendations
- ✅ **Debugging Value**: Critical for understanding AI decisions
- ✅ **Cost Tracking**: LLM API costs need monitoring

**Audit Events**:

| Event Type | Description | Priority |
|------------|-------------|----------|
| `ai-analysis.investigation.started` | AI investigation started | P0 |
| `ai-analysis.llm.request_sent` | LLM API request sent | P0 |
| `ai-analysis.llm.response_received` | LLM API response received | P0 |
| `ai-analysis.recommendation.generated` | Remediation recommendation generated | P0 |
| `ai-analysis.crd.updated` | AIAnalysis CRD updated | P0 |
| `ai-analysis.llm.request_failed` | LLM API request failed | P0 |
| `aianalysis.analysis.completed` | AI analysis completed with full Holmes response (SOC2) | **P0** |
| `aianalysis.analysis.failed` | AI analysis failed (Holmes API timeout, invalid response, etc.) | **P0** |

**SOC2 Compliance Event** (v1.3 - January 2026):
- **Event**: `aianalysis.analysis.completed`
- **Purpose**: Single event capturing complete `provider_data` for RR reconstruction (DD-AUDIT-004)
- **Distinction**: Complements existing granular `ai-analysis.*` events for operational visibility
- **Naming**: No hyphen (`aianalysis` not `ai-analysis`) to match SOC2 test plan convention
- **Required By**: BR-AUDIT-005 v2.0 (100% RR CRD reconstruction accuracy)
- **Event Data Fields**:
  ```json
  {
    "provider_data": {
      "provider": "HolmesGPT",
      "analysis_id": "holmes-abc123",
      "recommendations": [...],
      "confidence_score": 0.95
    }
  }
  ```

**Error Event & Standardized Error Details** (v1.5 - January 2026):
- **Event**: `aianalysis.analysis.failed`
- **Purpose**: Captures all AI analysis failures (broader than `ai-analysis.llm.request_failed`)
- **Distinction**: `aianalysis.analysis.failed` covers Holmes API timeouts, invalid responses, and generic upstream failures, while `ai-analysis.llm.request_failed` is specific to LLM API request errors
- **Naming**: Consistent with `aianalysis.analysis.completed` (no hyphen)
- **Required By**: BR-AUDIT-005 v2.0 Gap #7 (standardized error details for RR reconstruction)
- **ErrorDetails Structure** (applies to ALL error events):
  ```json
  {
    "event_data": {
      "analysis_name": "aianalysis-abc123",
      "error_details": {
        "message": "Holmes API timeout after 30s",
        "code": "ERR_UPSTREAM_TIMEOUT",
        "component": "aianalysis",
        "retry_possible": true,
        "stack_trace": ["..."] // Optional, for internal errors
      }
    }
  }
  ```
- **Error Code Examples**:
  - `ERR_UPSTREAM_TIMEOUT`: Holmes API timeout (retry=true)
  - `ERR_UPSTREAM_INVALID_RESPONSE`: Invalid JSON/schema from Holmes (retry=false)
  - `ERR_UPSTREAM_FAILURE`: Generic Holmes API error (retry=true)
- **Compliance**: DD-ERROR-001 (Error Details Standardization), SOC2 Type II RR reconstruction requirements

**Industry Precedent**: OpenAI API logs, Anthropic Claude logs, AWS Bedrock audit logs

**Expected Volume**: 500 events/day (success), 50 events/day (failures), 16.5 MB/month total

---

#### 3. Remediation Execution Controller ⭐⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Business-Critical**: Executes remediation actions (kubectl apply, scale, delete)
- ✅ **Compliance**: Kubernetes operations require audit trail (SOC 2, ISO 27001)
- ✅ **State Changes**: Modifies Kubernetes resources (Deployments, Pods, ConfigMaps)
- ✅ **Safety**: Critical for rollback and incident investigation
- ✅ **Debugging Value**: Essential for understanding what actions were executed

**Audit Events**:

| Event Type | Description | Priority |
|------------|-------------|----------|
| `execution.workflow.started` | Tekton workflow started | P0 |
| `workflow.selection.completed` | Workflow selected for remediation (SOC2) | **P0** |
| `workflowexecution.block.cleared` | Operator clears execution block (SOC2 CC8.1) | **P0** |
| `execution.action.executed` | Kubernetes action executed | P0 |
| `execution.action.succeeded` | Action succeeded | P0 |
| `execution.action.failed` | Action failed | P0 |
| `execution.workflow.completed` | Workflow completed | P0 |
| `execution.rollback.triggered` | Rollback triggered | P0 |

**SOC2 Compliance Events**:

**v1.3 (January 2026)**:
- **Event**: `workflow.selection.completed`
- **Purpose**: Captures which workflow was chosen for execution (DD-AUDIT-004)
- **Data**: `selected_workflow_ref` (workflow catalog reference)

**v1.4 (January 2026)**:
- **Event**: `workflowexecution.block.cleared`
- **Purpose**: Captures operator identity when clearing execution blocks (BR-WE-013)
- **Data**: `cleared_by` (operator identity), `clear_reason`, `block_duration`
- **Compliance**: SOC2 CC8.1 (Attribution requirement)
- **Distinction**: New event type (no existing equivalent)
- **RR Field**: Maps to `.status.selectedWorkflowRef` in RR CRD
- **Required By**: BR-AUDIT-005 v2.0 (100% RR CRD reconstruction accuracy)
- **Event Data Fields**:
  ```json
  {
    "selected_workflow_ref": {
      "name": "restart-pod-workflow",
      "version": "v1.2.3",
      "namespace": "kubernaut-system",
      "action_type": "RestartPod"
    },
    "selection_reason": "Best match for OOMKill incident",
    "alternatives_considered": 3
  }
  ```

**Industry Precedent**: Kubernetes Audit Logs, Argo Workflows audit, Tekton Pipelines logs

**Expected Volume**: 2,300 events/day, 69 MB/month (updated for workflow selection tracking)

---

#### 4. Notification Service ⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Compliance**: Notification delivery requires audit trail (SOC 2)
- ✅ **External Interactions**: Sends Slack messages, emails, PagerDuty alerts
- ✅ **State Changes**: Updates `Notification` CRD status
- ✅ **Debugging Value**: Critical for troubleshooting notification failures
- ✅ **SLA Tracking**: Notification delivery time monitoring

**Audit Events**: 4 event types covering per-channel delivery outcomes and phase-transition markers (`notification.message.*`) — see [AUDIT_EVENT_CATALOG.md](../../services/crd-controllers/06-notification/security/AUDIT_EVENT_CATALOG.md) for the current, code-verified list, triggers, and payload fields. Note: operator cancellation (SOC2 CC8.1 attribution) is emitted as `webhook.notification.cancelled` by **Auth Webhook**, not this service — see the [Auth Webhook catalog](../../services/shared/authentication-webhook/security/AUDIT_EVENT_CATALOG.md).

**Industry Precedent**: PagerDuty audit logs, Slack audit logs, SendGrid event webhooks

**Expected Volume**: 550 events/day, 16.5 MB/month (+50 events/day for operator cancellations)

---

#### 5. Data Storage Service ⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces (internal only)

**Rationale**:
- ✅ **Business-Critical**: Central data access layer (ADR-032)
- ✅ **Compliance**: Database operations require audit trail (SOC 2, ISO 27001)
- ✅ **State Changes**: All PostgreSQL writes go through this service
- ✅ **Debugging Value**: Critical for data integrity troubleshooting
- ✅ **Performance Monitoring**: Database query performance tracking

**Audit Events**: ⚠️ **Historical/frozen narrative below.** DD-WORKFLOW-018 (dropped the `remediation_workflow_catalog`/`action_type_taxonomy` Postgres tables) and DD-WORKFLOW-019 (Issue #1677, removed the `/api/v1/workflows*` REST surface) retired 10 of the 11 events described below; that responsibility moved to Auth Webhook (CRD admission) and Kubernaut Agent (workflow discovery). Data Storage's production code today emits exactly **one** event: `datastorage.ratelimit.denied` (per-IP rate limiting, BR-STORAGE-1505). See [AUDIT_EVENT_CATALOG.md](../../services/stateless/data-storage/security/AUDIT_EVENT_CATALOG.md) for the current, code-verified reference and the full retirement mapping.

**Note**: Data Storage **NO LONGER** audits meta-operations (audit writes, DLQ fallback) per DD-AUDIT-002 V2.0.1 (December 14, 2025). These were redundant because:
- **Successful writes**: Event in DB **IS** proof of success
- **Failed writes**: DLQ already captures failures
- **Operational visibility**: Maintained via Prometheus metrics (`audit_writes_total{status="success|failure|dlq"}`) and structured logs

**Industry Precedent**: AWS RDS audit logs, Google Cloud SQL audit logs (audit business operations, not CRUD operations)

**Expected Volume**: See catalog — the volume estimate below (§ Audit Event Volume Estimates) predates the DD-WORKFLOW-018/019 retirement and is stale for DS specifically.

**Authority**: DD-AUDIT-002 V2.0.1, DD-WORKFLOW-018, DD-WORKFLOW-019

---

#### 6. Auth Webhook Service ⭐⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Business-Critical**: Validates all ActionType CRD operations (CREATE, UPDATE, DELETE) via Kubernetes admission webhooks
- ✅ **Compliance**: CRD admission decisions require audit trail (SOC 2, ISO 27001)
- ✅ **State Changes**: Admits or denies CRD mutations that modify the ActionType taxonomy
- ✅ **Safety**: Critical for understanding why operations were admitted or denied
- ✅ **Cross-Service**: Coordinates with Data Storage for ActionType registration and workflow dependency checks

**Audit Events**: 15 event types across ActionType admission (`actiontype.*`), RemediationWorkflow admission (`remediationworkflow.*`), startup reconciliation, and 4 additional admission-webhook events (WorkflowExecution block-clear, RemediationRequest timeout edits, RemediationApprovalRequest decisions, NotificationRequest cancellation) — see [AUDIT_EVENT_CATALOG.md](../../services/shared/authentication-webhook/security/AUDIT_EVENT_CATALOG.md) for the current, code-verified list, triggers, and payload fields, including two known gaps (`actiontype.denied.create` is unreachable in production; `webhook.notification.cancelled` has a dormant secondary emission path with a divergent payload).

**Industry Precedent**: Kubernetes Admission Audit Logs, OPA/Gatekeeper decision logs

**Expected Volume**: 200 events/day, 6 MB/month (ActionType + RemediationWorkflow admission + startup reconciliation) — predates the 4 additional event types found in the catalog; stale, needs re-estimation

**Authority**: BR-WORKFLOW-007.4, ADR-058, Issue #1246, ADR-059, DD-WEBHOOK-003, ADR-040

---

#### 7. Effectiveness Monitor Service ⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Business-Critical**: Tracks remediation effectiveness (learning loop)
- ✅ **Compliance**: Effectiveness metrics require audit trail (SOC 2)
- ✅ **State Changes**: Emits assessment data as audit events (DD-017 v2.0)
- ✅ **Debugging Value**: Critical for understanding AI learning
- ✅ **ML Observability**: Model performance tracking

**Audit Events**: 7 implemented, component-level events (per ADR-EM-001 v1.3) covering health/alert/metrics assessment, spec-hash comparison (DD-EM-002), alert-decay detection (Issue #369, BR-EM-012), and assessment scheduling/completion — plus 2 "V1.1 Level 2" events (`effectiveness.learning.triggered`, `effectiveness.crd.updated`) that are aspirational and not implemented anywhere in code. See [AUDIT_EVENT_CATALOG.md](../../services/stateless/effectiveness-monitor/security/AUDIT_EVENT_CATALOG.md) for the current, code-verified list, triggers, and payload fields.

**Note**: EM Level 1 (V1.0) emits **component-level** audit events rather than a single monolithic event. The weighted effectiveness score is computed on-demand by DS (`GET /api/v1/effectiveness/{correlation_id}`) using `ComputeWeightedScore()` (DD-017 v2.1 formula). All events share a `correlation_id` (RemediationRequest name) as the join key. DD-KA-016 uses these events for remediation history context enrichment. Data stored as audit traces only — no new database tables.

**Industry Precedent**: MLflow tracking, Weights & Biases audit logs, Kubeflow Pipelines logs

**Expected Volume**: 500 events/day, 15 MB/month

---

### **P1: SHOULD Generate Audit Traces (2 Services)**

---

#### 8. Signal Processing Controller ⭐⭐⭐

**Status**: ✅ **SHOULD** generate audit traces (operational visibility)

**Rationale**:
- ✅ **State Changes**: Enriches signals with Kubernetes context
- ✅ **Debugging Value**: Useful for troubleshooting enrichment failures
- ⚠️ **Not Business-Critical**: Enrichment is supplementary (not core operation)
- ⚠️ **Low Volume**: Only runs once per signal

**Audit Events**: 6 event types covering signal processing outcome, phase transitions, classification decisions, business classification, enrichment completion, and errors — see [AUDIT_EVENT_CATALOG.md](../../services/crd-controllers/01-signalprocessing/security/AUDIT_EVENT_CATALOG.md) for the current, code-verified list, triggers, and payload fields. Note: the actual prefix is `signalprocessing.` (no hyphen), not `signal-processing.`; there is no distinct "enrichment started/failed" or "crd.updated" event — enrichment failures fold into a generic `signalprocessing.error.occurred`.

**Recommendation**: ✅ Generate audit traces for operational visibility, but P1 priority (not P0).

**Expected Volume**: 1,000 events/day, 30 MB/month

---

#### 9. Remediation Orchestrator Controller ⭐⭐⭐

**Status**: ✅ **SHOULD** generate audit traces (coordination visibility)

**Rationale**:
- ✅ **State Changes**: Coordinates lifecycle across 4 CRD controllers
- ✅ **Debugging Value**: Useful for troubleshooting coordination issues
- ✅ **Routing Decisions**: Tracks cooldown, duplicate detection, resource conflicts
- ⚠️ **Not Business-Critical**: Orchestration is coordination (not core operation)
- ⚠️ **Low Volume**: Only runs once per remediation

**Audit Events**: 15 event types covering lifecycle (created/started/transitioned/completed/EA-created), verification (verifying_started/verification_completed/verification_timed_out), routing-blocked, and approval decisions — see [AUDIT_EVENT_CATALOG.md](../../services/crd-controllers/05-remediationorchestrator/security/AUDIT_EVENT_CATALOG.md) for the current, code-verified list, triggers, and payload fields, including several known gaps: `orchestrator.phase.transitioned` was renamed to `orchestrator.lifecycle.transitioned`; `orchestrator.approval.expired` does not exist as a distinct type (expired decisions reuse `orchestrator.approval.rejected` with `event_action=expired`); `orchestrator.remediation.manual_review` is defined but has zero production callers (orphaned).

**Routing Blocked Event Context** (NEW - Dec 17, 2025):
- Captures: block reason, workflow ID, target resource, requeue timing, blocked duration
- Use cases: cooldown enforcement, duplicate detection, resource conflict resolution, consecutive failure tracking
- ADR-032 compliance: All phase transitions must be audited
- ⚠️ Per the catalog, the emitted payload today only carries `rr_name`/`namespace` — the richer fields listed above are built by the caller but not yet attached to the outgoing event (payload-mapping gap, not a doc issue)

**Recommendation**: ✅ Generate audit traces for coordination visibility, but P1 priority (not P0).

**Expected Volume**: 1,500 events/day, 45 MB/month (updated for Verifying phase events, #280)

---

### **NO Audit Traces Needed (3 Services)**

---

#### 10. Context API Service ❌

**Status**: ⚠️ **NO** audit traces needed

**Rationale**:
- ❌ **Read-Only**: Only queries historical data (no state changes)
- ❌ **No External Interactions**: Internal service (called by AI Analysis)
- ❌ **No Compliance Requirements**: Read operations don't require audit trail
- ❌ **Low Debugging Value**: Query failures are transient (retry handles)
- ✅ **Alternative**: Application logs sufficient for debugging

**Why No Audit Traces**:
- Context API is a **data provider** (like a database read replica)
- Industry standard: Read-only APIs don't generate audit traces
- Example: AWS S3 GET requests are NOT audited (only PUT/DELETE)

**Alternative Observability**:
- ✅ Application logs (structured logging)
- ✅ Prometheus metrics (query latency, cache hit rate)
- ✅ OpenTelemetry traces (request tracing)

**Industry Precedent**: AWS S3 GET (not audited), Google Cloud Storage read (not audited)

**⚠️ Future Consideration: PII Access Tracking**:

**Issue**: Context API handles PII data (incident details). GDPR Article 30 *may* require PII access tracking.

**Current Decision**: NO audit traces (v1.0)
- ✅ Not required for SOC 2, ISO 27001
- ✅ Industry standard: Read-only APIs not audited
- ✅ Cost optimization: Avoids high-volume audit traces

**Escape Hatch** (v2.0+):

If compliance requirements change (GDPR Article 30, HIPAA), Context API can enable optional PII access auditing:

```go
// pkg/contextapi/server.go
type Config struct {
    // Optional: Enable PII access audit (default: false)
    EnablePIIAccessAudit bool
}

func (s *Server) handleQuery(ctx context.Context, query *Query) (*QueryResult, error) {
    result, err := s.executeQuery(ctx, query)

    // Optional PII access audit (disabled by default)
    if s.config.EnablePIIAccessAudit && s.auditStore != nil {
        event := audit.NewAuditEvent()
        event.EventType = "context-api.pii.accessed"
        event.EventCategory = "data_access"
        event.EventAction = "read"
        event.EventOutcome = "success"
        event.IsSensitive = true // Mark as PII access
        _ = s.auditStore.StoreAudit(ctx, event)
    }

    return result, err
}
```

**Configuration**:
```yaml
# v1.0: No audit (default)
context_api:
  enable_pii_access_audit: false

# v2.0+: Enable if GDPR/HIPAA required
context_api:
  enable_pii_access_audit: true
```

**Impact if Enabled**:
- ⚠️ High audit volume (1000+ events/day)
- ⚠️ Storage cost increase (~30 MB/month)
- ✅ GDPR Article 30 compliance
- ✅ HIPAA compliance (if applicable)

**Recommendation**: Monitor compliance requirements and enable if needed in v2.0+.

---

#### 11. HolmesGPT API Service ❌

**Status**: ⚠️ **NO** audit traces needed (delegated to AI Analysis Controller)

**Rationale**:
- ❌ **Wrapper Service**: Thin wrapper around HolmesGPT SDK
- ❌ **No State Changes**: Only proxies requests to external LLM
- ❌ **Audit Responsibility**: AI Analysis Controller audits LLM interactions
- ✅ **Alternative**: Application logs + metrics sufficient

**Why No Audit Traces**:
- HolmesGPT API is a **proxy** (like an API gateway)
- Audit responsibility belongs to the **caller** (AI Analysis Controller)
- Industry standard: Proxy services don't duplicate audit traces

**Alternative Observability**:
- ✅ Application logs (request/response logging)
- ✅ Prometheus metrics (LLM latency, error rate)
- ✅ OpenTelemetry traces (distributed tracing)

**Industry Precedent**: AWS API Gateway (not audited), Nginx reverse proxy (not audited)

---

#### 12. Dynamic Toolset Service ❌

**Status**: ⚠️ **NO** audit traces needed

**Rationale**:
- ❌ **Configuration Service**: Only serves HolmesGPT toolset configuration
- ❌ **Read-Only**: No state changes (configuration is static)
- ❌ **No External Interactions**: Internal service (called by HolmesGPT API)
- ❌ **No Compliance Requirements**: Configuration reads don't require audit trail
- ✅ **Alternative**: Application logs sufficient

**Why No Audit Traces**:
- Dynamic Toolset is a **configuration provider** (like a ConfigMap)
- Industry standard: Configuration reads don't generate audit traces
- Example: Kubernetes ConfigMap reads are NOT audited

**Alternative Observability**:
- ✅ Application logs (configuration requests)
- ✅ Prometheus metrics (request count, latency)

**Industry Precedent**: Kubernetes ConfigMap reads (not audited), Consul KV reads (not audited)

---

#### 13. Fleet Metadata Cache (FMC) Service ❌

**Status**: ⚠️ **NO** audit traces needed

**Rationale**:
- ❌ **Read-Only**: Polls remote clusters via MCP Gateway for `kubernaut.ai/managed=true` resources and caches metadata in Valkey; never mutates cluster state
- ❌ **No CRD Writes**: Does not create, update, or delete any Kubernetes resource
- ❌ **Not in the Remediation CRD Chain**: FMC is a scope-lookup dependency consulted by Gateway/RemediationOrchestrator (via `FederatedScopeChecker`), not a participant in the signal -> analysis -> workflow -> execution -> verification -> notification lifecycle that BR-AUDIT-005's CC8.1 reconstruction targets
- ❌ **No External Interactions**: Internal service; its own network egress is to the Kuadrant MCP Gateway (SC-7 chokepoint), which is not a compliance-relevant external system on FMC's behalf
- ✅ **Alternative**: Application logs + Prometheus metrics (`fmc_sync_total`, `fmc_sync_errors_total`, `fmc_keys_written_total`) sufficient for debugging sync health

**Why No Audit Traces**:
- FMC is a **cache/lookup service** (like a CDN or read replica) -- its own sync cycles and scope-check responses are operational, not business-decision events
- Industry standard: cache population and read-only scope lookups are not audited; the *consumer* of the scope decision (Gateway, RO) is responsible for auditing what it did with that decision
- Same category as Context API (#10) and Dynamic Toolset (#12): configuration/cache provider consulted by other services, not itself a decision-maker in a compliance-relevant workflow

**Alternative Observability**:
- ✅ Application logs (structured logging of sync cycles, cache misses)
- ✅ Prometheus metrics (`pkg/fleet/fmc/syncer.go`: sync duration, errors, keys written per cluster)
- ✅ `/readyz` health probe (Valkey connectivity)

**Industry Precedent**: AWS ElastiCache read-through cache population (not audited), Kubernetes informer cache resyncs (not audited)

**Authority**: Issue #54, ADR-068 (Fleet Federation Architecture)

---

## 📊 **Summary Table**

| Service | Audit Traces? | Priority | Rationale |
|---------|--------------|----------|-----------|
| **Gateway Service** | ✅ **MUST** | P0 | Business-critical signal ingestion |
| **AI Analysis Controller** | ✅ **MUST** | P0 | Business-critical AI recommendations |
| **Remediation Execution Controller** | ✅ **MUST** | P0 | Business-critical Kubernetes operations |
| **Notification Service** | ✅ **MUST** | P0 | Compliance-required notification delivery |
| **Data Storage Service** | ✅ **MUST** | P0 | Workflow + ActionType catalog audit |
| **Auth Webhook Service** | ✅ **MUST** | P0 | ActionType CRD admission decisions (SOC2) |
| **Effectiveness Monitor Service** | ✅ **MUST** | P0 | Business-critical learning loop |
| **API Frontend Service** | ✅ **MUST** | P0 | User-facing MCP/A2A sessions, auth, tool execution (SOC2 AU-2, Issue #1156). Emits 30 `apifrontend.*` events covering auth, sessions, tools, triage, resilience, and config. |
| **Signal Processing Controller** | ✅ **SHOULD** | P1 | Operational visibility (enrichment) |
| **Remediation Orchestrator Controller** | ✅ **SHOULD** | P1 | Operational visibility (coordination) |
| **Context API Service** | ❌ **NO** | N/A | Read-only, no state changes |
| **HolmesGPT API Service** | ✅ **MUST** | P0 | LLM interactions and investigation outcomes (DD-AUDIT-005, BR-AUDIT-005). Emits `aiagent.*` events: `aiagent.llm.request`, `aiagent.llm.response`, `aiagent.llm.tool_call`, `aiagent.workflow.validation_attempt`, `aiagent.response.complete`, `aiagent.response.failed`, `aiagent.enrichment.completed`, `aiagent.enrichment.failed` |
| **Dynamic Toolset Service** | ❌ **NO** | N/A | Configuration, read-only |
| **Fleet Metadata Cache Service** | ❌ **NO** | N/A | Read-only scope cache, not a chain participant (Issue #54, ADR-068) |

**Total**: **11 out of 14 services** generate audit traces (79%)

---

## 🎯 **Implementation Priority**

### Phase 1: P0 Services (MUST) - 8 Services

**Timeline**: Sprint 1-2 (2 weeks)

**Services**:
1. Gateway Service (Week 1)
2. Data Storage Service (Week 1)
3. Auth Webhook Service (Week 1) — ActionType CRD admission audit (Issue #300)
4. AI Analysis Controller (Week 2)
5. Remediation Execution Controller (Week 2)
6. Notification Service (Week 2)
7. Effectiveness Monitor Service (Week 2)
8. API Frontend Service — 30 SOC2 AU-2 audit events (Issue #1156)

**Effort**: 7 hours (1 hour per service)

**Implementation**:
- Use `pkg/audit/` shared library (see [DD-AUDIT-002](./DD-AUDIT-002-audit-shared-library-design.md))
- Follow async buffered pattern (see [ADR-035](./ADR-038-async-buffered-audit-ingestion.md))
- Store in unified audit table (see [ADR-034](./ADR-034-unified-audit-table-design.md))

---

### Phase 2: P1 Services (SHOULD) - 2 Services

**Timeline**: Sprint 3 (1 week)

**Services**:
1. Signal Processing Controller (Week 3)
2. Remediation Orchestrator Controller (Week 3)

**Effort**: 2 hours (1 hour per service)

**Implementation**:
- Same as Phase 1 (use shared library)
- Lower priority (can be deferred if needed)

---

### Phase 3: No Audit Traces - 3 Services

**Services**:
1. Context API Service (application logs only)
2. HolmesGPT API Service (application logs only)
3. Dynamic Toolset Service (application logs only)

**Effort**: 0 hours (no audit implementation needed)

**Alternative Observability**:
- Structured logging (see [DD-005](./DD-005-OBSERVABILITY-STANDARDS.md))
- Prometheus metrics
- OpenTelemetry traces

---

## 📋 **Audit Event Volume Estimates**

### Expected Audit Event Volume (Production)

| Service | Events/Day | Events/Month | Storage/Month |
|---------|-----------|--------------|---------------|
| **Gateway Service** | 1,000 | 30,000 | 30 MB |
| **AI Analysis Controller** | 500 | 15,000 | 15 MB |
| **Remediation Execution Controller** | 2,310 | 69,300 | 69.3 MB |
| **Notification Service** | 550 | 16,500 | 16.5 MB |
| **Data Storage Service** | 5,100 | 153,000 | 153 MB |
| **Auth Webhook Service** | 200 | 6,000 | 6 MB |
| **Effectiveness Monitor Service** | 500 | 15,000 | 15 MB |
| **Signal Processing Controller** | 1,000 | 30,000 | 30 MB |
| **Remediation Orchestrator Controller** | 1,200 | 36,000 | 36 MB |
| **TOTAL** | **12,260** | **367,800** | **367.8 MB** |

**Storage Cost**: ~$0.37/month (PostgreSQL storage at $0.10/GB, 367.8 MB ≈ 0.37 GB)

**Retention**: 2555 days (~7 years, ADR-034 SOC 2 / ISO 27001 default)

**Assumptions**:
- Average event size: 1 KB
- Production load: 1000 remediations/day
- Event compression: None (conservative estimate)

---

## 📚 **Industry Precedents**

### Services That Audit

| Kubernaut Service | Industry Equivalent | Audited? | Rationale |
|-------------------|---------------------|----------|-----------|
| Gateway Service | AWS EventBridge | ✅ Yes | Signal ingestion is business-critical |
| AI Analysis Controller | OpenAI API | ✅ Yes | AI decisions require audit trail |
| Remediation Execution Controller | Kubernetes API | ✅ Yes | Infrastructure changes require audit |
| Notification Service | PagerDuty | ✅ Yes | Notification delivery is compliance-required |
| Data Storage Service | AWS RDS | ✅ Yes | Database operations require audit |
| Auth Webhook Service | OPA/Gatekeeper | ✅ Yes | Admission decisions require audit trail |
| Effectiveness Monitor Service | MLflow | ✅ Yes | ML model performance requires tracking |

---

### Services That Don't Audit

| Kubernaut Service | Industry Equivalent | Audited? | Rationale |
|-------------------|---------------------|----------|-----------|
| Context API Service | AWS S3 GET | ❌ No | Read-only operations not audited |
| HolmesGPT API Service | AWS API Gateway | ✅ Yes | Per DD-AUDIT-005: emits `aiagent.*` events for LLM interactions, tool calls, and investigation outcomes (provider perspective). Updated from v1.0 which incorrectly classified HAPI as proxy-only. |
| Dynamic Toolset Service | Kubernetes ConfigMap | ❌ No | Configuration reads not audited |

**Key Insight**: Industry standard is to audit **state-changing operations** and **external interactions**, NOT read-only or configuration operations.

---

## 🎯 **Confidence Assessment**

**Overall Confidence**: 95%

**Breakdown**:
- **P0 Services (MUST)**: 100% confidence (business-critical, compliance-required)
- **P1 Services (SHOULD)**: 90% confidence (operational visibility, not critical)
- **No Audit Services**: 95% confidence (read-only, no state changes)

**Why 95% (not 100%)**:
- 5% uncertainty: Potential future requirements for Context API (e.g., PII access tracking)
  - **Mitigation**: Re-evaluate if Context API adds write operations or PII access

---

## 🔗 **Related Decisions**

**Audit Architecture**:
- **ADR-034**: [Unified Audit Table Design](./ADR-034-unified-audit-table-design.md) - Database schema for audit events
- **ADR-035**: [Asynchronous Buffered Audit Ingestion](./ADR-038-async-buffered-audit-ingestion.md) - How services write audit traces
- **DD-AUDIT-001**: [Audit Responsibility Pattern](./DD-AUDIT-001-audit-responsibility-pattern.md) - Distributed audit pattern (services write their own traces)
- **DD-AUDIT-002**: [Audit Shared Library Design](./DD-AUDIT-002-audit-shared-library-design.md) - Implementation details for `pkg/audit/` shared library

**Supporting Decisions**:
- **ADR-032**: [Data Access Layer Isolation](./ADR-032-data-access-layer-isolation.md) - All DB access via Data Storage Service
- **DD-005**: [Observability Standards](./DD-005-OBSERVABILITY-STANDARDS.md) - Alternative observability for non-audited services
- **DD-007**: [Graceful Shutdown Pattern](./DD-007-kubernetes-aware-graceful-shutdown.md) - Ensures audit flush before shutdown

**Per-Service Event Catalogs** (current source of truth for individual event names/triggers, superseding the per-service narratives above — see [Document Map](#️-document-map-v26)):
- **Gateway**: [AUDIT_EVENT_CATALOG.md](../../services/stateless/gateway-service/security/AUDIT_EVENT_CATALOG.md) - 6 `gateway.*` events
- **Data Storage**: [AUDIT_EVENT_CATALOG.md](../../services/stateless/data-storage/security/AUDIT_EVENT_CATALOG.md) - 1 `datastorage.*` event (10 of 11 previously-documented events retired to Auth Webhook/KA)
- **Auth Webhook**: [AUDIT_EVENT_CATALOG.md](../../services/shared/authentication-webhook/security/AUDIT_EVENT_CATALOG.md) - 15 `actiontype.*`/`remediationworkflow.*`/`webhook.*`/`workflowexecution.*` events
- **Effectiveness Monitor**: [AUDIT_EVENT_CATALOG.md](../../services/stateless/effectiveness-monitor/security/AUDIT_EVENT_CATALOG.md) - 7 `effectiveness.*` events
- **Signal Processing**: [AUDIT_EVENT_CATALOG.md](../../services/crd-controllers/01-signalprocessing/security/AUDIT_EVENT_CATALOG.md) - 6 `signalprocessing.*` events
- **Remediation Orchestrator**: [AUDIT_EVENT_CATALOG.md](../../services/crd-controllers/05-remediationorchestrator/security/AUDIT_EVENT_CATALOG.md) - 15 `orchestrator.*`/`remediation.*` events
- **Notification**: [AUDIT_EVENT_CATALOG.md](../../services/crd-controllers/06-notification/security/AUDIT_EVENT_CATALOG.md) - 4 `notification.*` events
- **KA**: [AUDIT_EVENT_CATALOG.md](../../services/stateless/kubernaut-agent/security/AUDIT_EVENT_CATALOG.md) - all 37 `aiagent.*`/`workflow.*` events
- **API Frontend**: [AUDIT_EVENT_CATALOG.md](../../services/apifrontend/security/AUDIT_EVENT_CATALOG.md) - all `apifrontend.*`/`a2a.*` events

---

## 📝 **Implementation Checklist**

### Per-Service Implementation (8 services)

- [ ] **Add audit store initialization**
  ```go
  auditStore := audit.NewBufferedStore(dsClient, audit.DefaultConfig(), logger)
  ```

- [ ] **Replace custom audit calls**
  ```go
  auditStore.StoreAudit(ctx, event) // Non-blocking
  ```

- [ ] **Add graceful shutdown**
  ```go
  auditStore.Close() // Flushes remaining events
  ```

- [ ] **Define event types** (per service table above)

- [ ] **Add Prometheus metrics**
  - `audit_events_dropped_total`
  - `audit_events_written_total`

- [ ] **Test integration**
  - Unit tests (buffering, batching, retry)
  - Integration tests (PostgreSQL roundtrip)

---

## 📊 **Success Metrics**

**Compliance Metrics**:
- ✅ 100% of P0 services generate audit traces
- ✅ 100% of business-critical operations audited
- ✅ 100% of external interactions audited

**Performance Metrics**:
- ✅ <1% audit event drop rate
- ✅ <5% audit batch failure rate
- ✅ <1ms latency impact on business operations

**Cost Metrics**:
- ✅ Storage cost <$1/month (367.8 MB/month)
- ✅ Zero audit traces for read-only operations

---

---

## Fleet Cluster-Scoped Audit Requirements (v2.2 - June 2026)

**Status**: ✅ **APPROVED** (June 29, 2026)
**Authority**: BR-AUDIT-005 v2.0, ADR-065 (Multi-Cluster Federation), Issue #54
**Controls**: SOC2 CC8.1, FedRAMP AU-2, AU-3

### Requirement

Every audit event originating from or targeting a **remote cluster** MUST populate the `cluster_id` field (ADR-034 schema column, renamed from `cluster_name` in issue #1651) with the cluster identifier from the RemediationRequest or NormalizedSignal.

### Rationale

SOC2 CC8.1 requires complete remediation request reconstruction from audit traces alone. In a fleet (multi-cluster) deployment, an auditor must be able to determine which cluster a remediation targeted. Without `cluster_id` in audit events, fleet remediations are indistinguishable from local hub remediations in the audit trail, violating CC8.1 completeness.

### Per-Service Requirements

| Service | ClusterID Source | Wiring Point | Notes |
|---------|-----------------|--------------|-------|
| Gateway (GW) | `signal.ClusterID` | All 4 `emit*Audit()` functions | ClusterID extracted from alert labels by adapter |
| Signal Processing (SP) | `rr.Spec.ClusterID` via CRD spec | All `Emit*()` functions in audit manager | Propagated from RR by RO SignalProcessing creator |
| Remediation Orchestrator (RO) | `rr.Spec.ClusterID` | All `Emit*()` functions in audit manager | Direct access to RR in reconciler |
| Workflow Execution (WE) | `rr.Spec.ClusterID` via WFE spec | All `Emit*()` functions in audit manager | Propagated from RR by RO WFE creator |
| Effectiveness Monitor (EM) | `rr.Spec.ClusterID` via EA spec | All `Emit*()` functions in audit manager | Propagated from RR by RO EA creator |
| Notification (NT) | `rr.Spec.ClusterID` via NR spec | All `Emit*()` functions in audit manager | Propagated from RR by RO NR creator |
| API Frontend (AF) | `CreateRRArgs.ClusterID` | `audit.Event.ClusterID` → `AuditEventRequest.ClusterID` | Uses `audit.Event` struct; `StoreAdapter.Emit()` copies to `req.ClusterID` |
| Kubernaut Agent (KA) | `SessionContext.Signal.ClusterID` via `audit.WithClusterID(ctx)` | `AuditEvent.ClusterID` → `AuditEventRequest.ClusterID` | Context-injected in `launchInvestigation`; `StoreBestEffort` inherits from context |

### Backward Compatibility

- `cluster_id` remains nullable in the ADR-034 schema for single-cluster backward compatibility
- Empty `cluster_id` indicates local hub cluster (ADR-065 convention)
- Existing single-cluster audit events are unaffected

### Reconstruction Impact

The reconstruction pipeline (`pkg/datastorage/reconstruction/`) MUST:
1. Read `cluster_id` from audit events (already retrieved by `query.go`)
2. Map `cluster_id` to `ReconstructedRRFields.Spec.ClusterID` (`mapper.go`)
3. Include `cluster_id` in the `ReconstructionResponse` struct

### KA Narrative vs Table Inconsistency

DD-AUDIT-003 v2.0 narrative describes KA as P0 MUST (via the `aiagent` category, v1.3 update). The summary table at Section "Summary Table" correctly lists HolmesGPT API Service as P0 MUST with `aiagent.*` events emitted by KA (per v1.3 clarification). No action needed -- the naming reflects the KA Go rewrite (v1.3).

## API Frontend (AF) Event Catalog

> **⚠️ Historical/frozen as of v2.2 (June 2026).** This section previously
> duplicated AF's event catalog inline — the same drift risk this v2.6
> migration fixed for the other 7 per-service sections. **For the current,
> complete list of `apifrontend.*`/`a2a.*` event types**, see the
> authoritative, actively-maintained
> [`AUDIT_EVENT_CATALOG.md`](../../services/apifrontend/security/AUDIT_EVENT_CATALOG.md).
> This DD remains authoritative for the system-wide MUST/SHOULD/NO audit
> decision (see Summary Table above), not for AF's individual event names.

Fleet cluster provenance: events emitted during RR creation (`rr.created`, `rr.deduplicated`) carry `cluster_id` from `CreateRRArgs.ClusterID` via `audit.Event.ClusterID`; `StoreAdapter` copies this to `AuditEventRequest.ClusterID` for persistence in the unified audit table, enabling fleet-scoped CC8.1 reconstruction (v2.2, per-Service Requirements table above).

---

**Maintained By**: Kubernaut Architecture Team
**Last Updated**: June 29, 2026
**Review Cycle**: Annually or when new services are added

---

## v1.3 Update: Kubernaut Agent Audit Traces

> **⚠️ Historical/frozen as of v1.9 (Issue #823, April 2026).** This section
> documents only KA's original 15 events. **For the current, complete list of
> all 37 KA `aiagent.*`/`workflow.*` event types** (including everything
> added since: alignment/grounding, shadow-mode, fleet-federation,
> secret-access, interactive-mode, `aiagent.session.resumed`,
> `aiagent.interactive.k8s_call`, auth/rate-limit, and workflow-catalog-
> discovery events), see the authoritative, actively-maintained
> [`AUDIT_EVENT_CATALOG.md`](../../services/stateless/kubernaut-agent/security/AUDIT_EVENT_CATALOG.md).
> This DD remains authoritative for the system-wide MUST/SHOULD/NO audit
> decision (see Summary Table above), not for KA's individual event names.

In v1.3 (issue [#433](https://github.com/jordigilh/kubernaut/issues/433), Kubernaut Agent Go rewrite), documentation and operational context that referred to **HolmesGPT API (HAPI)** as the runtime for `aiagent.*` events should be read as **Kubernaut Agent (KA)** unless the text explicitly describes the legacy Python HAPI service. KA is the **authoritative emitter** for the `aiagent` category in v1.3.

**Actor fields (KA)**:

| Field | Value |
|-------|--------|
| `ActorType` | `Service` |
| `ActorID` | `kubernaut-agent` |

(Previously documented as `kubernaut-agent` for HAPI-emitted events.)

**`EventAction` / `EventOutcome` by event type** (KA sets both on every event; `event_id` is a **UUID** generated per event):

| Event type | EventAction (representative) | EventOutcome |
|------------|------------------------------|--------------|
| `aiagent.llm.request` | `llm_request` | `pending` |
| `aiagent.llm.response` | `llm_response` | `success` |
| `aiagent.llm.tool_call` | `tool_call` | `success` or `failure` (per call) |
| `aiagent.workflow.validation_attempt` | `validation_attempt` | `success` or `failure` |
| `aiagent.response.complete` | `response_complete` | `success` |
| `aiagent.response.failed` | `response_failed` | `failure` |
| `aiagent.enrichment.completed` | `enrichment` | `success` |
| `aiagent.enrichment.failed` | `enrichment` | `failure` |
| `aiagent.session.started` | `session_started` | `success` |
| `aiagent.session.cancelled` | `session_cancelled` | `success` |
| `aiagent.session.completed` | `session_completed` | `success` |
| `aiagent.session.failed` | `session_failed` | `failure` |
| `aiagent.investigation.cancelled` | `investigation_cancelled` | `failure` |
| `aiagent.session.observed` | `session_observed` | `success` |
| `aiagent.session.access_denied` | `session_access_denied` | `failure` |

**Granularity**: `aiagent.llm.tool_call` is emitted **once per tool call**. `aiagent.workflow.validation_attempt` is emitted per validation attempt and includes `workflow_id` and `is_final_attempt` where applicable. `aiagent.session.observed` includes `observer_user` (the authenticated identity of the subscribing user, extracted from `auth.UserContextKey` in the request context) for SOC2 CC8.1 operator attribution. `aiagent.session.access_denied` is emitted when an authenticated user attempts to access a session they do not own; includes `requesting_user`, `session_id`, and `endpoint` for SOC2 CC8.1 failed-access audit trail.

**Reference**: [TP-433-AUDIT-SOC2](../../tests/433/TP-433-AUDIT-SOC2.md).

