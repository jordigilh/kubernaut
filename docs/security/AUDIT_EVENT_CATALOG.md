# Audit Event Catalog

Authoritative reference for all structured audit events emitted by the kubernaut-apifrontend service.

**Source of truth:** `internal/audit/audit.go` (EventType constants)
**Schema:** All events conform to the `audit.Event` struct:

```go
type Event struct {
    Timestamp time.Time         `json:"timestamp"`
    Type      EventType         `json:"type"`
    RequestID string            `json:"request_id,omitempty"`
    UserID    string            `json:"user_id,omitempty"`
    SourceIP  string            `json:"source_ip,omitempty"`
    Detail    map[string]string `json:"detail,omitempty"`
}
```

---

## Authentication & Authorization

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `auth.success` | `EventAuthSuccess` | AU-2, AC-7 | JWT validated or TokenReview accepted | `issuer`, `groups` |
| `auth.failure` | `EventAuthFailure` | AU-2, AC-7 | JWT rejected or TokenReview denied | `reason` |
| `rbac.denied` | `EventRBACDenied` | AC-3, AC-6 | Tool call blocked by RBAC guard | `tool`, `role`, `reason` |
| `impersonation.created` | `EventImpersonation` | AC-3, AU-12 | K8s impersonated client created for user | `target_user`, `groups` |
| `jwt.delegation` | `EventJWTDelegation` | AC-3, AU-12 | Original JWT forwarded to downstream service (KA) | `target_service` |

**Emitted from:** `internal/auth/middleware.go`, `internal/agent/root.go` (RBAC guard)

---

## Tool Invocation

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `tool.invoked` | `EventToolInvoked` | AU-12, AU-2 | Any ADK tool call completes (success or error) | `tool`, `result` (`success`/`error`), `namespace` (if applicable), `error` (on failure) |
| `mcp.tool_invoked` | `EventMCPToolInvoked` | AU-12 | MCP protocol tool/call request handled | `tool`, `session_id` |

**Emitted from:** `internal/agent/root.go` (afterAudit callback), `internal/handler/mcp.go`

---

## Session Lifecycle

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `session.created` | `EventSessionCreated` | AU-2, SC-4 | InvestigationSession CRD created | `session_id`, `user`, `rr_ref` |
| `session.deleted` | `EventSessionDeleted` | AU-2 | Session CRD deleted (user or TTL) | `session_id`, `reason` |
| `session.phase_changed` | `EventSessionPhaseChanged` | AU-2 | Session transitions state (e.g. active → completed) | `session_id`, `from`, `to` |
| `session.auto_cancelled` | `EventSessionAutoCancelled` | AU-2 | Session cancelled due to inactivity timeout | `session_id`, `idle_duration` |
| `session.retention_deleted` | `EventSessionRetentionDeleted` | AU-2, SC-28 | Session deleted by retention policy (TTL controller) | `session_id`, `age` |

**Emitted from:** `internal/session/service.go`, `internal/controller/ttl.go`

---

## A2A Protocol

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `a2a.task_started` | `EventA2ATaskStarted` | AU-2 | A2A `message/send` begins execution | `task_id`, `user` |
| `a2a.task_completed` | `EventA2ATaskCompleted` | AU-2 | A2A task finishes successfully | `task_id`, `duration_ms` |
| `a2a.task_failed` | `EventA2ATaskFailed` | AU-2 | A2A task fails with error | `task_id`, `error` |

**Emitted from:** `internal/launcher/launcher.go` (BeforeExecute/AfterExecute callbacks)

---

## Infrastructure & Resilience

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `ratelimit.denied` | `EventRateLimitDenied` | SC-5 | Request rejected by rate limiter | `client_ip`, `limit`, `window` |
| `circuitbreaker.trip` | `EventCircuitBreakerTrip` | SI-17 | Circuit breaker opens (dependency unhealthy) | `dependency`, `failures` |

**Emitted from:** `internal/ratelimit/ratelimit.go`, (circuit breaker state change)

---

## Configuration

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `config.reloaded` | `EventConfigReloaded` | CM-3 | Hot-reload applied new configuration successfully | `source`, `keys_changed` |
| `config.rejected` | `EventConfigRejected` | CM-3 | Hot-reload rejected invalid configuration | `source`, `reason` |

**Emitted from:** `internal/config/hotreload.go`

---

## Backend & Delivery

Events are delivered through the `audit.Emitter` interface. Two implementations exist:

| Implementation | Package | Behavior |
|---------------|---------|----------|
| `LogEmitter` | `internal/audit` | Writes structured log entries via `logr` (stdout/stderr) |
| `BufferedEmitter` | `internal/audit` | Batches events and flushes to `audit.Writer` (Data Store API) with overflow protection |

**Buffering contract (ADR-019):** The `BufferedEmitter` holds up to `MaxPending` events in memory. If the buffer overflows, oldest events are dropped and `af_audit_buffer_overflow_total` metric increments. On graceful shutdown, `Close()` flushes remaining events with a context deadline.

---

## Adding New Events

1. Define the `EventType` constant in `internal/audit/audit.go`
2. Add the emit call at the appropriate location with `Detail` fields
3. Update this catalog with the new event's trigger, fields, and NIST mapping
4. Ensure tests verify emission (check `Emit` call count or captured events)

---

*Last updated: 2026-05-08 | Covers v1.5 milestone (issues #52, #56)*
