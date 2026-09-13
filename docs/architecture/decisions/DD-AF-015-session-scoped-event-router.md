# DD-AF-015: Session-Scoped In-Process Event Router

**Status**: Approved
**Date**: 2026-09-13
**Issue**: #2390
**Related**: [DD-AF-009](DD-AF-009-pooled-session-live-event-relay.md), [DD-AF-014](DD-AF-014-business-outcome-completion-coordinator.md), BR-AI-086

## Context

`EventRelay` was introduced by DD-AF-009 as a mutex-protected pointer to the
context of the currently executing pooled MCP call. That was sufficient for one
subscriber, but it made event delivery last-writer-wins and tied the event
source to a particular call shape.

The residual KA event channel has one consumer by design. After an interactive
investigation is handed to the session pool, events may need to reach either a
pooled KA call, `kubernaut_watch`, or both. The consumer must therefore route
events to explicit subscribers rather than selecting one current context.

## Decision

Replace `EventRelay` with an `EventRouter` owned by each injected pooled session.
The existing `(rr_id, username)` `KASessionPool` key remains the isolation
boundary; callers obtain the router only through that entry.

The router provides:

- `Subscribe(EventSink)`, returning a token-specific unsubscribe function.
- `Publish(InvestigationEvent)`, which snapshots subscribers under a lock and
  invokes them after unlocking.
- Multiple concurrent subscribers without last-writer-wins replacement.
- Explicit lifecycle cleanup through deferred unsubscribe at each call boundary.

`WatchTerminalEvents` remains the sole consumer of the KA event channel. It
publishes non-terminal and terminal events through the router. Pooled calls and
`kubernaut_watch` subscribe only for the duration of their active A2A call.
When a router is present but has no subscribers, terminal events are not sent to
the detached handoff context, preventing writes to a closed A2A queue. Direct
legacy calls without a router retain the DD-AF-009/#1438 fallback behavior.

The A2A adapter is injected into the KA package as an `EventEmitter`, keeping
KA session management independent of A2A presentation details.

## Alternatives

### A. Continue extending `EventRelay`

Rejected as the primary design. Attaching `kubernaut_watch` to the pointer is a
small fix, but concurrent calls still overwrite one another and lifecycle
ownership remains implicit.

### B. Durable external event stream or replay store

Deferred. BR-AI-086 defines live reasoning visibility as best-effort; the audit
trail remains the durable record. Cross-replica delivery, replay, and process
recovery require a separate deployment and storage decision rather than an
in-process AF change.

### C. Session-scoped in-process router

Selected. It preserves one channel consumer, supports the current multi-turn
and concurrent-observer requirement, removes context storage from the router,
and keeps the implementation within the existing session-pool lifecycle.

## Consequences

### Positive

- KA events can reach pooled calls and `kubernaut_watch` without misrouting.
- Subscriber cleanup is explicit and token-specific.
- User isolation remains enforced by the existing composite pool key.
- No KA protocol, MCP transport, or A2A wire-format change is required.
- Slow sinks do not hold the router mutex.

### Limits

- The router is process-local and provides no replay after disconnect or restart.
- Sink delivery remains best-effort and occurs in the watcher goroutine.
- A future cross-replica/replay requirement must introduce an external event
  stream or durable store under a new design decision.

## Verification

BDD coverage proves router fan-out, unsubscribe behavior, concurrent lifecycle
operations, pooled-call subscription cleanup, terminal-event routing, and the
existing direct fallback path.
