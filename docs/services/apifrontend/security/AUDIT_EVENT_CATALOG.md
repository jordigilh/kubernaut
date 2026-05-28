# Audit Event Catalog

Authoritative reference for all structured audit events emitted by the kubernaut-apifrontend service.

**Source of truth:** `pkg/apifrontend/audit/audit.go` (EventType constants)
**Schema:** All events conform to the `audit.Event` struct:

```go
type Event struct {
    Timestamp     time.Time         `json:"timestamp"`
    Type          EventType         `json:"type"`
    CorrelationID string            `json:"correlation_id,omitempty"`
    RequestID     string            `json:"request_id,omitempty"`
    UserID        string            `json:"user_id,omitempty"`
    SourceIP      string            `json:"source_ip,omitempty"`
    Detail        map[string]string `json:"detail,omitempty"`
}
```

---

## Authentication & Authorization

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `auth.success` | `EventAuthSuccess` | AU-2, AC-7 | JWT validated or TokenReview accepted | `issuer`, `groups` |
| `auth.failure` | `EventAuthFailure` | AU-2, AC-7 | JWT rejected or TokenReview denied | `reason` |
| `auth.access_denied` | `EventAuthAccessDenied` | AC-3, AC-6 | Tool call blocked by RBAC guard (consolidated from `rbac.denied` + `mcp.tool_denied`, Issue #1156) | `tool_name`, `user_role`, `endpoint`, `reason` |
| `jwt.delegation` | `EventJWTDelegation` | AC-3, AU-12 | Original JWT forwarded to downstream service (KA) | `target_service` |

**Emitted from:** `pkg/apifrontend/auth/middleware.go`, `pkg/apifrontend/agent/root.go` (RBAC guard), `pkg/apifrontend/handler/mcp_bridge.go`

---

## Tool Invocation

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `tool.executed` | `EventToolExecuted` | AU-12, AU-2 | Any tool call completes (consolidated from `tool.invoked` + `mcp.tool_invoked`, Issue #1156) | `tool_name`, `tool_outcome` (`success`/`error`), `session_id`, `execution_duration_ms`, `namespace` (if applicable), `error` (on failure) |
| `mcp.tool_failed` | `EventMCPToolFailed` | AU-12 | MCP tool/call request fails (panic, rate limit, throttle, error, marshal failure) | `tool_name`, `error` |
| `mcp.session_init` | `EventMCPSessionInit` | AU-12 | MCP `initialize` JSON-RPC request handled (first per session) | `mcp_session_id`, `protocol_version` |
| `workflow.discovery` | `EventWorkflowDiscovery` | AU-12 | `kubernaut_discover_workflows` tool returns workflow list | `workflow_count` |

**Emitted from:** `pkg/apifrontend/agent/root.go` (afterAudit callback), `pkg/apifrontend/handler/mcp.go`, `pkg/apifrontend/handler/mcp_bridge.go`, `pkg/apifrontend/tools/ka_interactive.go`

---

## Session Lifecycle

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `session.created` | `EventSessionCreated` | AU-2, SC-4 | InvestigationSession CRD created | `session_id`, `user`, `rr_ref` |
| `session.deleted` | `EventSessionDeleted` | AU-2 | Session CRD deleted (user or TTL) | `session_id`, `reason` |
| `session.phase_changed` | `EventSessionPhaseChanged` | AU-2 | Session transitions state (e.g. active -> completed) | `session_id`, `from`, `to` |
| `session.completed` | `EventSessionCompleted` | AU-2 | Session reaches terminal phase (Completed/Failed/Cancelled) | `session_id`, `terminal_phase`, `total_duration_ms` |
| `session.auto_cancelled` | `EventSessionAutoCancelled` | AU-2 | Session cancelled due to inactivity timeout | `session_id`, `idle_duration` |
| `session.retention_deleted` | `EventSessionRetentionDeleted` | AU-2, SC-28 | Session deleted by retention policy (TTL controller) | `session_id`, `age` |

**Emitted from:** `pkg/apifrontend/session/statemachine.go`, `pkg/apifrontend/session/service.go`, `pkg/apifrontend/controller/ttl.go`

---

## A2A Protocol

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `a2a.task_started` | `EventA2ATaskStarted` | AU-2 | A2A `message/send` begins execution | `task_id`, `user`, `session_id` |
| `a2a.task_completed` | `EventA2ATaskCompleted` | AU-2 | A2A task finishes successfully | `task_id`, `duration_ms` |
| `a2a.task_failed` | `EventA2ATaskFailed` | AU-2 | A2A task fails with error | `task_id`, `error` |
| `a2a.stream_opened` | `EventA2AStreamOpened` | AU-2 | SSE stream opened for `message/stream` | *(defined; not yet emitted — logged only, see `streaming_executor.go`)* |
| `a2a.stream_closed` | `EventA2AStreamClosed` | AU-2 | SSE stream closed | *(defined; not yet emitted — logged only, see `streaming_executor.go`)* |

**Emitted from:** `pkg/apifrontend/launcher/launcher.go` (BeforeExecute/AfterExecute callbacks)

---

## Triage & Remediation

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `triage.started` | `EventTriageStarted` | AU-2, AU-12 | Triage pipeline begins for a session | `session_id`, `persona` |
| `triage.completed` | `EventTriageCompleted` | AU-2, AU-12 | Triage pipeline completes | `session_id`, `triage_outcome`, `triage_duration_ms` |
| `severity_triage.completed` | `EventSeverityTriageCompleted` | AU-2, AU-12 | Severity triage pipeline determines severity | `tier`, `severity`, `source`, `duration_ms`, `alert_name` (if Tier 1), `rule_name` (if Tier 1.5/2/2.5) |
| `severity_triage.failed` | `EventSeverityTriageFailed` | AU-2 | All severity triage tiers fail or LLM error | `error` (redacted), `tier` (last attempted), `namespace`, `kind`, `name` |
| `rr.created` | `EventRRCreated` | AU-2 | RemediationRequest CRD created | `session_id`, `rr_name`, `rr_namespace`, `fingerprint` |
| `rr.deduplicated` | `EventRRDeduplicated` | AU-2 | Duplicate RemediationRequest detected, creation skipped | `session_id`, `fingerprint`, `existing_rr_name` |

**Emitted from:** `pkg/apifrontend/launcher/launcher.go`, `pkg/apifrontend/tools/af_create_rr.go`

---

## KubernautAgent Delegation

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `ka.delegated` | `EventKADelegated` | AU-2, AU-12 | Work delegated to KubernautAgent (takeover, reconnect, REST) | `session_id`, `ka_correlation_id`, `delegation_type` |
| `ka.result_received` | `EventKAResultReceived` | AU-2, AU-12 | Result received from KubernautAgent (complete, cancel, REST) | `session_id`, `ka_correlation_id`, `result_type` |

**Emitted from:** `pkg/apifrontend/tools/ka_investigate.go`, `pkg/apifrontend/tools/ka_interactive.go`

---

## User Interaction

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `user.decision` | `EventUserDecision` | AU-2 | User accepts/rejects a remediation workflow | `session_id`, `decision`, `workflow_id` |

**Emitted from:** `pkg/apifrontend/tools/ka_tools.go`

---

## Discovery

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `discovery.agent_card_accessed` | `EventAgentCardAccessed` | AU-2, AU-3 | Agent card endpoint queried (Issue #1259) | `source_ip`, `user_agent` |

**Emitted from:** `pkg/apifrontend/handler/agentcard.go`

---

## Infrastructure & Resilience

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `ratelimit.denied` | `EventRateLimitDenied` | SC-5 | Request rejected by rate limiter | `client_ip`, `limit`, `window` |
| `circuitbreaker.trip` | `EventCircuitBreakerTrip` | SI-17 | Circuit breaker opens (dependency unhealthy) | `dependency`, `failures` |

**Emitted from:** `pkg/apifrontend/ratelimit/ratelimit.go`, (circuit breaker state change)

---

## Configuration

| Event Type | Constant | NIST Control | Trigger | Detail Fields |
|-----------|----------|-------------|---------|---------------|
| `config.reloaded` | `EventConfigReloaded` | CM-3 | Hot-reload applied new configuration successfully | `source`, `keys_changed`, `config_version` |
| `config.rejected` | `EventConfigRejected` | CM-3 | Hot-reload rejected invalid configuration | `source`, `reason` |

**Emitted from:** `pkg/apifrontend/config/hotreload.go`

---

## Backend & Delivery

Events are delivered through the `audit.Emitter` interface. Two implementations exist:

| Implementation | Package | Behavior |
|---------------|---------|----------|
| `LogEmitter` | `pkg/apifrontend/audit` | Writes structured log entries via `logr` (stdout/stderr) |
| `StoreAdapter` | `pkg/apifrontend/audit` | Normalizes events to `apifrontend.<event_type>` format, classifies severity, and forwards to Data Store API with correlation-ID enrichment |

**Buffering contract (ADR-019):** The shared `pkg/audit.BufferedAuditStore` (default capacity 10,000) buffers events in memory. If the buffer is full, newest events are dropped and the platform-standard `audit_events_dropped_total{service="apifrontend"}` metric increments. On graceful shutdown, `Close()` flushes remaining events with a context deadline.

---

## Adding New Events

1. Define the `EventType` constant in `pkg/apifrontend/audit/audit.go`
2. Add the emit call at the appropriate location with `Detail` fields
3. Update this catalog with the new event's trigger, fields, and NIST mapping
4. Ensure tests verify emission (check `Emit` call count or captured events)

---

*Last updated: 2026-05-19 | Covers v1.5 milestone (issues #52, #56, #92, #1156, #1259, #1268)*
