# DD-AF-008: Status Subscribe SSE Endpoint

**Status**: Accepted
**Issue**: #1460
**Business Requirement**: BR-AF-1460
**Date**: 2026-06-18

## Context

After workflow selection, the MCP chat session terminates by design (`select_workflow.go:293`).
The console has no channel to learn about subsequent RR phase changes
(Executing, Verifying, Completed, etc.). The console team requested a dedicated
SSE stream for RR phase transitions, independent of the A2A `message/stream`.

## Decision

Implement a standalone HTTP handler on `POST /a2a/status` that accepts a
JSON-RPC `status/subscribe` request and returns an SSE stream of phase
transition events.

### Route Design: Standalone Handler (not a2a-go extension)

`status/subscribe` is a kubernaut-specific extension, not an A2A spec method.
The handler is registered directly in `RouterConfig` and `NewRouter`, reusing
the same auth middleware chain and SSE tracker but bypassing `a2a-go`'s method
dispatch (which only knows `message/send` and `message/stream`).

### Contract

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "status-1",
  "method": "status/subscribe",
  "params": { "rr_id": "rr-0fd7ce7b49a7-784e8ea4" }
}
```

Namespace is resolved server-side per ADR-057 (controller namespace injected at
startup). The `rr_id` is a plain resource name.

**Response:** SSE stream (`Content-Type: text/event-stream`)

Phase transition events:
```
event: status/update
data: {"jsonrpc":"2.0","method":"status/update","params":{"rr_id":"...","phase":"Verifying","timestamp":"2026-06-18T15:10:00Z","final":false,"metadata":{...}}}
```

Pre-expiry warning (before token deadline kills stream):
```
event: status/closing
data: {"jsonrpc":"2.0","method":"status/closing","params":{"reason":"token_expiry","reconnect":true}}
```

Heartbeat (every 15s):
```
: heartbeat
```

**Error codes** (non-streaming JSON-RPC error response):

| Code | Message | When |
|------|---------|------|
| -32600 | invalid_request | Malformed JSON-RPC |
| -32601 | method_not_found | Method is not status/subscribe |
| -32602 | invalid_params | Missing rr_id |
| -32001 | rr_not_found | RR does not exist |
| -32002 | access_denied | User lacks RBAC access |

### Per-Phase Metadata

Metadata uses raw CRD field names. The console computes derived values.

| Phase | Metadata fields |
|-------|----------------|
| Pending, Processing, Analyzing, Cancelled | (none) |
| AwaitingApproval | approval_request_name |
| Executing | workflow_id, started_at |
| Verifying | verification_deadline, started_at, ea_phase, stabilization_deadline |
| Blocked | blocked_until, block_reason, block_message |
| Completed | outcome |
| Failed | failure_reason, failure_phase |
| TimedOut | failure_phase |
| Skipped | skip_reason |

### EA Sub-Phase Events During Verifying

While the RR stays in `Verifying`, the EffectivenessAssessment (EA) progresses
through Pending, WaitingForPropagation, Stabilizing, Assessing, Completed, Failed.
The stream emits `status/update` events with `phase: "Verifying"` unchanged but
updated `ea_phase` and `stabilization_deadline` in metadata.

EA discovery uses `RR.status.effectivenessAssessmentRef` (corev1.ObjectReference)
instead of the `EANameForRR()` naming convention.

### Stream Lifecycle

1. On subscribe: GET current RR, send current phase immediately (REQ-2)
2. If RR is already terminal: send event with `final: true`, close stream
3. Start K8s watch on RR; on Verifying, start EA watch via effectivenessAssessmentRef
4. On terminal phase: send event with `final: true`, close stream
5. On token deadline approaching (~5s before): send `status/closing`
6. On watch channel close: re-establish watch transparently (reconnection loop)
7. Heartbeat every 15s

### Token Deadline Handling

The auth middleware sets `context.WithDeadline(ctx, expiresAt - [25-34s] jitter)`.
This kills the SSE stream before the token expires. The handler sends a
`status/closing` pre-warning ~5s before the deadline so the client can
proactively reconnect with a fresh token. REQ-2 (current-state on reconnect) is
mandatory.

## Consequences

- AF RBAC ClusterRole needs `watch` and `list` verbs on effectivenessassessments
  (currently only `get`)
- `HandleWatch` in `crd_tools.go` migrated from `EANameForRR()` to
  `effectivenessAssessmentRef` for consistency
- Per-connection K8s watch (REQ-1 shared fan-out deferred to future iteration)
- Global connection cap (1000) shared with A2A/MCP streams
