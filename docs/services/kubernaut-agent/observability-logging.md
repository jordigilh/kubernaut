# Kubernaut Agent - Observability & Logging

**Version**: 1.0
**Last Updated**: 2026-08-02
**Status**: ✅ Current

---

## Purpose

Covers KA's structured logging and correlation-ID propagation. For metrics, see
[metrics-slos.md](./metrics-slos.md). For the full audit event catalog (SOC2/FedRAMP-mapped), see
[security/AUDIT_EVENT_CATALOG.md](./security/AUDIT_EVENT_CATALOG.md) — this document does not
duplicate that catalog.

---

## Structured Logging

KA uses `go.uber.org/zap` for structured logging, consistent with the rest of the stateless
services (see [stateless/README.md](../stateless/README.md)). Logs are JSON-structured in production.

---

## Correlation ID Propagation

Every investigation carries a **correlation ID** — the `RemediationID` (the owning
`RemediationRequest`'s identity) — from submission through to every emitted audit event. This is
the key that enables SOC2 CC8.1 full-lifecycle reconstruction (signal → analysis → workflow
selection → execution → verification → notification) across all services, per
[BR-AUDIT-005](../../../AGENTS.md#soc2-and-fedramp-compliance).

**Propagation path**:

1. AIAnalysis's `IncidentRequest.remediation_id` field carries the correlation ID into KA (see
   [api-specification.md](./api-specification.md)).
2. `internal/kubernautagent/audit/emitter.go`'s `WithCorrelationID(ctx, correlationID)` /
   `CorrelationIDFromContext(ctx)` thread it through the investigation's `context.Context` for the
   lifetime of the session.
3. Every `AuditEvent` built during the investigation carries `CorrelationID` in its struct (see
   [security/AUDIT_EVENT_CATALOG.md](./security/AUDIT_EVENT_CATALOG.md#schema) for the full schema),
   and is forwarded to Data Storage's unified audit table.

**Additional correlation fields** carried alongside `CorrelationID` on audit events:

| Field | Purpose |
|---|---|
| `SessionID` | KA's own investigation session identifier (distinct from `RemediationID`) |
| `ActingUser` | Populated for Interactive MCP Mode events attributable to a specific operator |
| `ClusterID` | Fleet-target investigations only (multi-cluster provenance, `main` branch) |
| `ParentEventID` | Links a sub-event (e.g. a single tool call) to its parent investigation-turn event |

---

## Log Correlation with Audit Events

Structured logs and audit events share the same correlation ID (`RemediationID`) and session ID,
so operators can pivot from a log line to the full audit trail for the same investigation (and vice
versa) without a separate lookup step.

---

## Related Documentation

- [security/AUDIT_EVENT_CATALOG.md](./security/AUDIT_EVENT_CATALOG.md) — full audit event catalog with NIST/SOC2 control mapping
- [metrics-slos.md](./metrics-slos.md) — Prometheus metrics
- [overview.md](./overview.md) — architecture overview
