# Audit Event Catalog — Data Storage Service (DS)

Authoritative reference for all structured audit events emitted by the `datastorage` service.

**Source of truth:** `pkg/datastorage/audit/ratelimit_event.go`. No `AllEventTypes`-style exported slice exists in `pkg/datastorage`.
**Predecessor doc:** [DD-AUDIT-003](../../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) §"Data Storage Service" documents 11 events; **10 of those 11 no longer exist in DS's production code** (see below). This catalog reflects the current, code-verified reality.

---

## ⚠️ Major correction: DS self-emits exactly ONE audit event today

| Event Type | Constant | Category | Action | Outcome | Trigger | Data Fields |
|-----------|----------|----------|--------|---------|---------|--------------|
| `datastorage.ratelimit.denied` | `EventTypeRatelimitDenied` | `security` | `denied` | `failure` | DS's own per-IP HTTP rate limiter (pre-auth) rejects an incoming request | `EventID` (self-correlation UUID — no upstream correlation ID exists pre-auth), optional `SourceIP`, `Path`, `Method`; actor `("external", sourceIP)`; resource `("HTTPRequest", eventID)` |

**Emitted from:** `pkg/datastorage/audit/ratelimit_event.go` (`NewRatelimitDeniedAuditEvent`), called from `pkg/datastorage/server/server_routes.go` (`AuditFunc` callback wired into `dsmiddleware.IPRateLimitMiddleware`). Authority: BR-STORAGE-1505 (GAP-09, Issue #1505), FedRAMP AU-12.

---

## Where the other 10 documented events actually went

DD-AUDIT-003's 11-event Data Storage baseline predates two design decisions that retired the underlying REST surface and database tables entirely:

1. **DD-WORKFLOW-018** (migration `016_drop_workflow_catalog_and_action_type_tables.sql`): the `remediation_workflow_catalog` and `action_type_taxonomy` Postgres tables were dropped. `RemediationWorkflow`/`ActionType` **CRDs** (etcd) are now sole source of truth, admitted directly by Auth Webhook with zero DS round-trips.
2. **DD-WORKFLOW-019** (Issue #1677, Phase 2g): DS's `/api/v1/workflows*` and `/api/v1/action-types/{name}/workflow-count` REST surface and backing handler code were deleted outright (explicit retirement comment: `pkg/datastorage/server/server_routes.go`).

| Retired DS event | Now emitted by | See |
|---|---|---|
| `datastorage.workflow.created` | *(fully removed, no replacement — RemediationWorkflow CRD admission covers this)* | DD-WORKFLOW-018 |
| `datastorage.workflow.updated` | *(fully removed)* | DD-WORKFLOW-018 |
| `datastorage.actiontype.created` | `actiontype.admitted.create` (Auth Webhook) | [Auth Webhook catalog](../../../shared/authentication-webhook/security/AUDIT_EVENT_CATALOG.md) |
| `datastorage.actiontype.updated` | `actiontype.admitted.update` (Auth Webhook) | [Auth Webhook catalog](../../../shared/authentication-webhook/security/AUDIT_EVENT_CATALOG.md) |
| `datastorage.actiontype.disabled` | `actiontype.admitted.delete` (Auth Webhook) — disable/reenable semantics were replaced by direct CRD CREATE/UPDATE/DELETE admission | [Auth Webhook catalog](../../../shared/authentication-webhook/security/AUDIT_EVENT_CATALOG.md) |
| `datastorage.actiontype.disable_denied` | `actiontype.denied.delete` (Auth Webhook) | [Auth Webhook catalog](../../../shared/authentication-webhook/security/AUDIT_EVENT_CATALOG.md) |
| `datastorage.actiontype.reenabled` | *(concept removed — no reenable path exists post-#1661)* | DD-ACTIONTYPE-001 |
| `workflow.catalog.actions_listed` | KubernautAgent, same string value (DD-WORKFLOW-019) | [KA catalog](../../kubernaut-agent/security/AUDIT_EVENT_CATALOG.md) |
| `workflow.catalog.workflows_listed` | KubernautAgent, same string value | [KA catalog](../../kubernaut-agent/security/AUDIT_EVENT_CATALOG.md) |
| `workflow.catalog.workflow_retrieved` | KubernautAgent, same string value | [KA catalog](../../kubernaut-agent/security/AUDIT_EVENT_CATALOG.md) |
| `workflow.catalog.selection_validated` | KubernautAgent, same string value | [KA catalog](../../kubernaut-agent/security/AUDIT_EVENT_CATALOG.md) |

`internal/kubernautagent/audit/emitter.go` documents this relocation explicitly in its own comments ("Relocated from `pkg/datastorage/audit/workflow_discovery_event.go`: KA, not DS, now generates these 4 events").

---

## Known Gaps (tracked, not fixed by this catalog)

1. **Dead/unused builder code in `pkg/datastorage/audit/`**: `event_builder.go` (`BaseEventBuilder`), `gateway_event.go` (`GatewayEventBuilder`), `workflow_event.go` (`WorkflowEventBuilder`), `aianalysis_event.go` (`AIAnalysisEventBuilder`), and `interfaces.go` (`Writer`/`Reader`/`Repository`) have **zero production callers anywhere in the repo** — confirmed via repo-wide import grep. They appear to be either superseded pre-ADR-034 scaffolding or shared builders other services were meant to adopt but never did. Not removed here (out of scope for a doc-only catalog migration); flagged for a future dead-code decision.
2. **DD-AUDIT-003 still cites two files that don't exist**: `pkg/datastorage/audit/actiontype_events.go` and `pkg/datastorage/audit/workflow_catalog_event.go` are referenced in DD-AUDIT-003's Data Storage narrative but do not exist anywhere in the repository (confirmed via glob).

---

## Adding New Events

1. Define the event type constant in `pkg/datastorage/audit/` and construct the event with a dedicated `New*AuditEvent` function (see `ratelimit_event.go` for the pattern)
2. Wire the emit call at the production entry point (`pkg/datastorage/server/`), never only in a test
3. Add/extend the OpenAPI discriminator schema in `api/openapi/data-storage-v1.yaml`
4. Update this catalog with the new event's trigger, fields, and NIST/SOC2 control mapping
5. Ensure a UT proves the emission decision and an IT proves it fires through the production entry point (Pyramid Invariant)

---

*Last updated: 2026-07-31 | QE readiness audit follow-up (DD-AUDIT-003 single-source-of-truth migration) | Covers the 1 event type DS's production code actually emits as of this date*
