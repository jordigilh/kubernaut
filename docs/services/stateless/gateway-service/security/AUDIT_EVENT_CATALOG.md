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

## Shared Auth Middleware Security Events (Issue #2664)

Gateway signal routes run [`pkg/shared/auth/middleware.go`](../../../../../pkg/shared/auth/middleware.go) (`Server.RegisterAdapter` applies `s.authMiddleware.Handler` as the outermost layer), which emits structured `security_event` **log** records — not audit-table rows.

| Log Event | `event_type` / `event_action` / `event_outcome` | Trigger | Detail Fields |
|-----------|------------------------------------------------|---------|---------------|
| `security_event` (reason `missing_auth_header`, `invalid_auth_format`, `empty_bearer_token`, `invalid_token`, `invalid_token_audience`) | `authentication` / `authenticate` / `failure` | TokenReview rejects the webhook (401) | `event_id` (UUIDv4), `actor_type="external"`, `actor_id=""`, `reason`, `status_code`, `method`, `path`, `remote_addr`, `resource`, `resource_name`, `verb` |
| `security_event` (reason `authorization_denied`) | `authorization` / `deny` / `failure` | SAR denies the authenticated caller (403) | Same fields, with `actor_type="user"`, `actor_id=<username>`, `user=<username>` |
| `security_event` (reason `authentication_service_unavailable`) | `authentication` / `authenticate` / `failure` | TokenReview API call fails; fail-closed 500 (Issue #2664: previously unlogged) | `actor_type="external"`, `status_code=500` |
| `security_event` (reason `authorization_service_unavailable`) | `authorization` / `authorize` / `failure` | SAR API call fails; fail-closed 500 (Issue #2664: previously unlogged) | `actor_type="user"`, `actor_id=<username>`, `status_code=500` |
| `security_event` (reason `authorization_granted`) | `authorization` / `grant` / `success` | SAR allows the webhook (Issue #2664: granted decisions are now observable) | `actor_type="user"`, `actor_id=<username>`, `status_code=200` (the authorization decision, not the downstream response) |

All events carry `correlation_id` (the service request ID: Gateway's `RequestIDMiddleware`, otherwise chi `RequestID`, exposed as `X-Request-ID`; always present in the four production consumers because chi `RequestID` runs before auth; empty only if a future caller registers auth with no request-ID middleware and sends no header). The two outage events additionally carry DD-ERROR-001 `error_details` (`message`, `code=ERR_AUTH_SERVICE_UNAVAILABLE`/`ERR_AUTHZ_SERVICE_UNAVAILABLE`, `component=auth-middleware`, `retry_possible=true`).

**Controls:** FedRAMP AU-2, AU-3, AC-6; SOC2 CC7.2; OWASP ASVS V2/V3/V7.

**Volume:** one `grant` line per authorization decision (SAR check). Gateway signal routes run a single auth middleware, so this is one line per authorized request, in addition to Gateway's existing per-request "Request completed" line. Since #2694 these records are emitted through a dedicated security-event logger (`log.NewSecurityEventLogger`, BR-SECURITY-016, FedRAMP AU-2) derived from the host logger in the server constructor's `newAuthMiddleware`: zap's sampler is disabled and the level is fixed at Info, independent of the service's configured level and hot-reload, so neither a request burst nor a WARN/ERROR service configuration can drop a grant, deny or outage record. Unrelated service logging keeps its normal sampling and level gating.

**Tests:** `pkg/shared/auth/middleware_security_event_test.go` (UT-AUTH-2664-001..013); `pkg/gateway/auth_security_event_wiring_test.go` (IT-AUTH-2664-050..055 plus IT-AUTH-2694-050 burst, production `RegisterAdapter` chain); `pkg/log/logger_security_event_test.go` (UT-LOG-2694-001..003) and `pkg/shared/auth/middleware_security_event_delivery_test.go` (UT-AUTH-2694-001..005) prove unsampled, non-level-gated delivery.

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
