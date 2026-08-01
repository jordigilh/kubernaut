# Audit Event Catalog — Gateway Service (GW)

Authoritative reference for all structured audit events emitted by the `gateway` service.

**Source of truth:** `pkg/gateway/server.go` (`EventType*`/`Action*` const blocks, lines 63-86). No `AllEventTypes`-style exported slice exists — the two const blocks are the definitive enumeration, cross-checked against the OpenAPI discriminator enums in `api/openapi/data-storage-v1.yaml` (`GatewayAuditPayload`, `GatewayConfigReloadedPayload`, `GatewayConfigRejectedPayload`).
**Payload mapping:** `pkg/gateway/audit_emission.go` (emission functions build `api.GatewayAuditPayload`/`GatewayConfigReloadedPayload`/`GatewayConfigRejectedPayload`)
**Predecessor doc:** [DD-AUDIT-003](../../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) §"Gateway Service" — this catalog is the current, code-verified reference; see Known Gaps for one naming correction.

**Schema:** `EventCategory = "gateway"` (`CategoryGateway`) for all events. Fleet provenance: `cluster_id` set conditionally per DD-AUDIT-003 v2.2 (CC8.1) wherever `signal.ClusterID`/similar is non-empty.

---

## Signal Ingestion & CRD Lifecycle

| Event Type | Constant | Action | Trigger | Data Fields |
|-----------|----------|--------|---------|--------------|
| `gateway.signal.received` | `EventTypeSignalReceived` | `received` | A new (non-duplicate) signal's RemediationRequest CRD was just successfully created | `SignalType`, `SignalName`, `Namespace`, `Fingerprint`, `Severity`, `ResourceKind`/`ResourceName`, `RemediationRequest`, `DeduplicationStatus="new"`; conditionally `OriginalPayload`/`SignalLabels`/`SignalAnnotations` (BR-AUDIT-005 Gaps #1-3) |
| `gateway.signal.deduplicated` | `EventTypeSignalDeduplicated` | `deduplicated` | Lock-contention retry discovers another Gateway pod already created the RR for this fingerprint | Base fields + `OccurrenceCount`, `RemediationRequest`, `DeduplicationStatus="duplicate"` |
| `gateway.crd.created` | `EventTypeCRDCreated` | `created` | RemediationRequest CRD successfully created in Kubernetes | Base fields + `Severity`, `ResourceKind`/`ResourceName`, `RemediationRequest`, `OccurrenceCount=1` (BR-GATEWAY-056) |
| `gateway.crd.failed` | `EventTypeCRDFailed` | `failed` | Fires up to N+1 times per failed signal: once per intermediate `CRDCreator` retry attempt (via `RetryObserver.OnRetryAttempt`), plus once for the final non-retryable failure | Base fields + `Severity`, `ResourceKind`/`ResourceName`, `ErrorDetails` (BR-AUDIT-005 Gap #7, standardized); specialized `ERR_CIRCUIT_BREAKER_OPEN` detail when the cause is `circuitbreaker.ErrOpenState` (BR-GATEWAY-093) |
| `gateway.config.reloaded` | `EventTypeConfigReloaded` | `reloaded` | A hot-reloadable setting (log level or CA cert) reloads successfully (GAP-11, Issue #1505) | `Component` (`"log_level"` or `"ca_cert"`) |
| `gateway.config.rejected` | `EventTypeConfigRejected` | `rejected` | A hot-reload attempt fails validation; previous config is kept | `Component`, `RejectionReason` |

**Emitted from:** `pkg/gateway/audit_emission.go` (`emitSignalReceivedAudit`, `emitSignalDeduplicatedAudit`, `emitCRDCreatedAudit`, `emitCRDCreationFailedAudit`, `EmitConfigReloadAudit`), called from `pkg/gateway/signal_ingestion_process.go` and `cmd/gateway/main.go` (`startLogLevelWatcher`/`wireHotReload` callbacks). No fleet/scope-related audit events exist despite Gateway's fleet owner-resolution code (`pkg/gateway/adapters/owner_resolver.go`, `pkg/gateway/scope.go` have zero audit references).

---

## Known Gaps (tracked, not fixed by this catalog)

1. **Event renamed, DD-AUDIT-003 never updated**: the baseline's `gateway.crd.creation_failed` does not exist in code. The actual (and long-standing) constant is `gateway.crd.failed` — confirmed consistent across the Go constant, both emission call sites, the OpenAPI schema, `docs/requirements/BR-GATEWAY-058-audit-event-emission-requirements.md`, and the integration test suite (`GW-INT-AUD-016`/`GW-INT-AUD-019`). Only `DD-AUDIT-003` and the older `DD-ERROR-001` still use the stale `creation_failed` spelling.

---

## Adding New Events

1. Define the `EventType`/`Action` constants in `pkg/gateway/server.go`
2. Add the emission function in `pkg/gateway/audit_emission.go` and wire the call at the production entry point (`signal_ingestion_process.go` or `cmd/gateway/main.go`), never only in a test
3. Add/extend the OpenAPI discriminator schema in `api/openapi/data-storage-v1.yaml`
4. Update this catalog with the new event's trigger, fields, and NIST/SOC2 control mapping
5. Ensure a UT proves the emission decision and an IT proves it fires through the production entry point (Pyramid Invariant)

---

*Last updated: 2026-07-31 | QE readiness audit follow-up (DD-AUDIT-003 single-source-of-truth migration) | Covers all 6 event types in the `pkg/gateway/server.go` const block*
