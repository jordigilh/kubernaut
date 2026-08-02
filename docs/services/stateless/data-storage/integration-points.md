# Data Storage Service - Integration Points

**Version**: v2.0 (Unified Audit Event API — ADR-034)
**Last Updated**: August 2026
**Service Type**: Stateless HTTP REST API (Audit Write + Query)
**Ports**: `8080` (REST API), `8081` (Health), `9090` (Metrics)
**Authentication**: In-process middleware — TokenReview + SubjectAccessReview (DD-AUTH-014), no sidecar

> **Rewritten** ([#1806](https://github.com/jordigilh/kubernaut/issues/1806)): the previous version of this
> document (dated November 2025) described a `GET /api/v1/incidents` read API, per-domain audit tables
> (`remediation_audit`, `workflow_audit`), a `pgvector`-based Vector DB, and an `ose-oauth-proxy` sidecar —
> none of which exist in the current implementation. See [What Changed](#what-changed-since-the-november-2025-version)
> at the end of this document for the full list of corrections.

---

## Table of Contents

1. [Service Position in Architecture](#service-position-in-architecture)
2. [Authentication & Authorization](#authentication--authorization-dd-auth-014)
3. [Upstream Services (Writers)](#upstream-services-writers)
4. [Downstream Consumers (Readers)](#downstream-consumers-readers)
5. [API Surface](#api-surface)
6. [Database Integration](#database-integration)
7. [Write Pattern: Async Buffered Store](#write-pattern-async-buffered-store)
8. [Error Handling](#error-handling)
9. [What Changed Since the November 2025 Version](#what-changed-since-the-november-2025-version)
10. [Reference Documentation](#reference-documentation)

---

## Service Position in Architecture

DataStorage is the **centralized audit event store** for Kubernaut, implementing the unified
audit table / event-sourcing pattern (ADR-034, BR-AUDIT-005). Every business-critical operation
across services persists a structured audit event here, queryable by `correlation_id` to
reconstruct a signal's complete lifecycle (signal → analysis → workflow selection → execution →
verification → notification) for SOC2 CC8.1 and FedRAMP AU-2/AU-3 compliance.

```
┌──────────────────────────────────────────────────────────────────┐
│  Upstream Writers (async, non-blocking)                          │
│  Gateway · AIAnalysis · WorkflowExecution · RemediationOrchestrator│
│  Notification · AuthWebhook · SignalProcessing · Kubernaut Agent  │
└─────────────────────────┬──────────────────────────────────────────┘
                          │ POST /api/v1/audit/events{,/batch}
                          │ (Bearer token; TokenReview + SAR in-process)
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│  Data Storage Service (:8080 API / :8081 health / :9090 metrics) │
│  1. Auth middleware: validate Bearer token, SAR-check RBAC       │
│  2. Validate request against AuditEventRequest schema            │
│  3. Write to unified audit_events table (hash-chained, ADR-034)  │
│  4. On DB failure: 202 Accepted, queue to DLQ (DD-009)           │
└─────────────────────────┬──────────────────────────────────────────┘
                          │ SQL INSERT (parameterized)
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│  PostgreSQL 16 — audit_events (single unified table, ADR-034)    │
└──────────────────────────────────────────────────────────────────┘
                          ▲
                          │ GET (query/reconstruct/effectiveness/history)
┌─────────────────────────┴──────────────────────────────────────────┐
│  Downstream Readers                                               │
│  RemediationOrchestrator (ineffective-chain routing)              │
│  Kubernaut Agent (prompt enrichment context)                      │
│  API Frontend (triage/session context)                            │
└──────────────────────────────────────────────────────────────────┘
```

There is no Vector DB / pgvector component and no per-domain audit tables (`remediation_audit`,
`workflow_audit`, `audit_embeddings`) — see [What Changed](#what-changed-since-the-november-2025-version).

---

## Authentication & Authorization (DD-AUTH-014)

DataStorage does **not** use a sidecar proxy. Authentication and authorization happen in an
in-process Go middleware, confirmed in `deploy/data-storage/deployment.yaml`:

> *"DD-AUTH-014: Middleware-Based Authentication/Authorization — No oauth-proxy sidecar - auth
> handled in DataStorage middleware. Flow: Client → DataStorage:8081 (direct)"*

1. Caller sends `Authorization: Bearer <ServiceAccount token>`.
2. Middleware validates the token via the Kubernetes `TokenReview` API.
3. Middleware authorizes the call via `SubjectAccessReview` (SAR) against the `data-storage-client`
   ClusterRole — each calling ServiceAccount has its own RoleBinding (see
   `deploy/data-storage/client-rbac-v2.yaml`).
4. `401 Unauthorized` = missing/invalid/expired token; `403 Forbidden` = SAR denied (ServiceAccount
   lacks the RoleBinding).

This superseded the earlier `ose-oauth-proxy` sidecar approach (DD-AUTH-011, DD-AUTH-012), which has
been removed from the deployment entirely — see those documents' supersession notices for history.

---

## Upstream Services (Writers)

All writers use the shared `pkg/audit.AuditStore` interface (`StoreAudit(ctx, *ogenclient.AuditEventRequest)
error`, DD-AUDIT-002) backed by `BufferedAuditStore` — a non-blocking, in-memory-buffered,
batched-with-retry writer (ADR-038). A `StoreAudit` call never blocks the caller's primary
operation; on buffer-full it drops the event and returns an error rather than crashing.

| Service | RoleBinding (`client-rbac-v2.yaml`) | Audit package | `event_category` |
|---|---|---|---|
| Gateway | `gateway-data-storage-client` | `pkg/gateway/audit_emission.go` | `gateway` |
| AIAnalysis Controller | `aianalysis-data-storage-client` | `pkg/aianalysis/audit/audit.go` | `analysis` |
| WorkflowExecution Controller | `workflowexecution-data-storage-client` | `pkg/workflowexecution/audit/manager.go` | `workflowexecution` |
| RemediationOrchestrator | `remediationorchestrator-data-storage-client` | `pkg/remediationorchestrator/audit/manager.go` | `orchestration`, `approval` |
| Notification Controller | `notification-data-storage-client` | `pkg/notification/audit/manager.go` | `notification` |
| AuthWebhook | `authwebhook-data-storage-client` | `pkg/authwebhook/*.go` (per-CRD audit builders) | varies by admitted CRD |
| SignalProcessing | `signalprocessing-data-storage-client` | `pkg/signalprocessing/audit/*.go` | `signalprocessing` |
| Kubernaut Agent (KA) | `kubernaut-agent-data-storage-client` | `cmd/kubernautagent/datastorage.go`, `internal/kubernautagent/audit/` | `aiagent` |

`event_category` values are enumerated in the `AuditEventRequest` schema
(`api/openapi/data-storage-v1.yaml`); the full set also includes `workflow`, `effectiveness`,
`actiontype`, `apifrontend`, and `security` for events emitted by other write paths.

#### Write call shape (real, from `pkg/audit/store.go`)

```go
// AuditStore is the shared interface every writer uses (DD-AUDIT-002).
type AuditStore interface {
    // StoreAudit buffers an event for async write. Never blocks; drops on buffer-full.
    StoreAudit(ctx context.Context, event *ogenclient.AuditEventRequest) error
    Flush(ctx context.Context) error
    Close() error
}
```

The Kubernetes Executor Controller and its execution-audit write path referenced in the prior
version of this document no longer exist — execution is performed via Tekton `TaskRun`,
orchestrated by WorkflowExecution (ADR-025); there is no separate executor service or CRD.

---

## Downstream Consumers (Readers)

| Consumer | Endpoint | Purpose | Code |
|---|---|---|---|
| RemediationOrchestrator `RoutingEngine` | `GET /api/v1/remediation-history/context` | Tier1 (24h detail) + Tier2 (90d summary) history to block ineffective-remediation chains (BR-ORCH-042.5, DD-KA-016) | `pkg/remediationorchestrator/routing/ds_history_adapter.go` (`DSHistoryAdapter`, Issue #214) |
| Kubernaut Agent (KA) | `GET /api/v1/remediation-history/context` | Same endpoint, consumed for RCA prompt enrichment context | `internal/kubernautagent/enrichment/`, `cmd/kubernautagent/datastorage.go` |
| API Frontend | audit query endpoints | Triage/session context for interactive investigations | `pkg/apifrontend/ds/ogen_client.go` |
| Any authorized caller | `GET /api/v1/audit/remediation-requests/{correlation_id}/reconstruct` | Full lifecycle reconstruction from audit traces (SOC2 CC8.1) | — |
| Any authorized caller | `GET /api/v1/audit/verify-chain` | Hash-chain integrity verification (FedRAMP AU-9) | `pkg/datastorage/server/audit_verify_chain_handler.go` |
| Any authorized caller | `GET /api/v1/effectiveness/{correlation_id}` | Remediation effectiveness data | — |
| Any authorized caller | `GET /api/v1/audit/export`, `GET/POST /api/v1/audit/legal-hold` | Compliance export and legal-hold management (AU-9, AU-11) | — |

**Effectiveness Monitor**: a query client exists (`pkg/effectivenessmonitor/client/ds_querier.go`),
but as of this writing there is no `deploy/effectiveness*` manifest and no corresponding
`RoleBinding` in `client-rbac-v2.yaml` — treat it as in-development, not yet a production caller.

KA's workflow catalog interaction is **read-only discovery** (e.g. listing workflows for its
validator), not the retired CRUD/search API described in the prior version of this document.

---

## API Surface

Authoritative source: `api/openapi/data-storage-v1.yaml`, registered in `pkg/datastorage/server/server.go`.

| Endpoint | Method(s) | Purpose |
|---|---|---|
| `/api/v1/audit/events` | `POST`, `GET` | Create one audit event (ADR-034) / query with filters |
| `/api/v1/audit/events/batch` | `POST` | Batch-create audit events |
| `/api/v1/audit/notifications` | — | Notification-domain audit query |
| `/api/v1/audit/legal-hold` | `POST`, `GET` | Place/list legal holds on audit records (AU-9/AU-11) |
| `/api/v1/audit/legal-hold/{correlation_id}` | `GET` | Legal-hold status for a correlation ID |
| `/api/v1/audit/export` | `GET` | Compliance export |
| `/api/v1/audit/remediation-requests/{correlation_id}/reconstruct` | `GET` | Full remediation lifecycle reconstruction (SOC2 CC8.1) |
| `/api/v1/audit/verify-chain` | `GET` | Hash-chain integrity verification (AU-9) |
| `/api/v1/effectiveness/{correlation_id}` | `GET` | Effectiveness data for a correlation ID |
| `/api/v1/remediation-history/context` | `GET` | Tier1/Tier2 remediation history for routing/enrichment |

`POST /api/v1/audit/events` behavior (per the OpenAPI spec): `201 Created` on success, `400` on
validation failure (RFC 7807), `401`/`403` per the auth model above, and **`202 Accepted`** on a
database write failure — the event is queued to a dead-letter queue for async retry (DD-009)
rather than being dropped or failing the caller's request.

The `GET /api/v1/incidents`, `POST /api/v1/audit/remediation`, and `POST /api/v1/audit/workflow`
endpoints described in the prior version of this document are **not** in the OpenAPI spec and have
no server registration — they never shipped this way.

---

## Database Integration

- **Engine**: PostgreSQL 16, single unified `audit_events` table (ADR-034) — not the per-domain
  `remediation_audit`/`workflow_audit` tables described previously.
- **Integrity**: hash-chained records (FedRAMP AU-9) — `pkg/datastorage/repository/audit_events_hashchain.go`,
  verified on demand via `GET /api/v1/audit/verify-chain`.
- **No Vector DB**: pgvector-based embedding storage and semantic similarity search were retired
  (see `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md`, Category 5/6 retirement —
  `BR-STORAGE-012` through `BR-STORAGE-016`). No embedding generation happens on the write path.
- **No workflow catalog tables**: CRUD/search endpoints for a workflow catalog were retired
  (same document, Category 10 — `BR-STORAGE-038` through `BR-STORAGE-042`).

---

## Write Pattern: Async Buffered Store

Implemented once in `pkg/audit.BufferedAuditStore` and reused by every writer in the table above
(DD-AUDIT-002, ADR-038):

1. `StoreAudit()` appends the event to an in-memory channel and returns immediately — the caller's
   primary operation (e.g. creating a CRD) is never blocked or failed by an audit write.
2. A background goroutine batches buffered events and flushes them on a timer or when the batch
   size threshold is reached.
3. Failed writes are retried with exponential backoff (`s.writeBatchWithRetry`); on shutdown, any
   remaining buffered events are flushed before the process exits.
4. If DataStorage itself fails to persist a batch (DB unavailable), it responds `202 Accepted` and
   queues to a DLQ rather than returning an error to the writer.

This is a materially different (and more resilient) pattern than the "goroutine-per-write,
best-effort, log-and-continue" model described in the prior version of this document.

---

## Error Handling

All error responses use [RFC 7807 Problem Details](https://datatracker.ietf.org/doc/html/rfc7807)
(`application/problem+json`), per `DD-004-RFC7807-ERROR-RESPONSES.md`:

| Status | Meaning |
|---|---|
| `201` | Audit event created |
| `202` | Validated but DB write failed — queued to DLQ (DD-009), not lost |
| `400` | Request failed `AuditEventRequest` schema validation |
| `401` | Bearer token missing, invalid, or expired (TokenReview rejection) |
| `403` | SAR denied — caller's ServiceAccount lacks the `data-storage-client` RoleBinding |
| `500` | Unexpected server error |

There is no bespoke `{"error": {"code": ..., "message": ...}}` format — that shape, and the
`VALIDATION_ERROR`/`DUPLICATE_AUDIT`/`EMBEDDING_ERROR` error codes described previously, do not
appear in the current OpenAPI spec.

---

## What Changed Since the November 2025 Version

The previous version of this document (v2.0, dated November 1, 2025) was written before
implementation and was never reconciled with what actually shipped. Corrections made in this
rewrite:

| Previous claim | Current reality |
|---|---|
| `GET /api/v1/incidents`, `GET /api/v1/incidents/:id` | Never implemented; not in the OpenAPI spec. `api-specification.md` itself flags this API as disabled pending [#238](https://github.com/jordigilh/kubernaut/issues/238) |
| `POST /api/v1/audit/remediation`, `/workflow`, per-domain write endpoints | Superseded by a single unified `POST /api/v1/audit/events` (+ `/batch`), ADR-034 |
| `remediation_audit`, `workflow_audit`, `audit_embeddings` tables | Single unified `audit_events` table; no vector/embedding table |
| pgvector Vector DB, embedding generation, cosine-similarity search | Fully retired (`BR-STORAGE-012`–`016`) |
| Workflow catalog CRUD/search via this API | Fully retired (`BR-STORAGE-038`–`042`); KA's catalog access today is read-only discovery |
| `ose-oauth-proxy` sidecar + Bearer-token-validated-by-in-handler-TokenReviewer | In-process DD-AUTH-014 middleware; no sidecar; ports are `8080`/`8081`/`9090`, not `8080`+`9090` |
| Synchronous, goroutine-per-write, best-effort audit writes | Shared, non-blocking, batched `BufferedAuditStore` (DD-AUDIT-002/ADR-038) with DLQ fallback |
| Effectiveness Monitor and Kubernetes Executor Controller as active/planned integrations | Effectiveness Monitor has query code but no production deployment yet; Kubernetes Executor Controller doesn't exist (ADR-025 — replaced by Tekton `TaskRun` via WorkflowExecution) |

---

## Reference Documentation

- **API Specification**: `docs/services/stateless/data-storage/api-specification.md`
- **Security Configuration**: `docs/services/stateless/data-storage/security-configuration.md`
- **Testing Strategy**: `docs/services/stateless/data-storage/testing-strategy.md`
- **Business Requirements**: `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md`
- **Unified Audit Table Design**: `docs/architecture/decisions/ADR-034-unified-audit-table-design.md`
- **Middleware-Based Auth**: `docs/architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md`
- **Shared Audit Library**: `docs/architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md`
- **Async Buffered Ingestion**: `docs/architecture/decisions/ADR-038-async-buffered-audit-ingestion.md`
- **DLQ / Write Error Recovery**: `docs/architecture/decisions/DD-009-audit-write-error-recovery.md`

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: August 2026
**Integration Status**: Implemented and in production (V1.0)
