# DD-AF-009: Live Event Relay for Pooled Interactive MCP Calls

**Status**: ✅ Approved
**Priority**: P2
**Owner**: API Frontend Team
**Scope**: `pkg/apifrontend/ka/event_relay.go` (new), `pkg/apifrontend/ka/session_pool.go`, `pkg/apifrontend/ka/pooled_mcp_client.go`, `pkg/apifrontend/tools/ka_investigate_bridge.go`, `pkg/apifrontend/tools/ka_investigate_mcp.go`
**Related**: [DD-LLM-009](./DD-LLM-009-reasoning-content-live-stream-event-type.md) (reasoning content live-stream event type), Issues #1634, #1635, #1637

---

## Context & Problem

AF talks to KA over MCP through two distinct paths:

- **`kubernaut_investigate`** (`SDKMCPClient.StartInvestigation`) opens a dedicated MCP session whose `LoggingMessageHandler` decodes KA's logging notifications onto an event channel, then blocks in `bridgeEventsCollectSummary` relaying every event live to the A2A SSE stream.
- **`kubernaut_message` / `discover_workflows` / `select_workflow` / `complete_no_action`** (`PooledMCPClient`) reuse the *same* underlying `*mcp.ClientSession` — handed off into `KASessionPool` after the initial blocking `kubernaut_investigate` call completes — for subsequent synchronous `CallTool`s.

KA's `handleMessage` (and the other pooled actions) run their LLM/business logic **synchronously**, inside the same RPC AF is blocked on, and KA continues emitting logging notifications (`reasoning_delta`, `reasoning_content_delta`, `tool_call_start`, `error`, etc.) on that same session while doing so. But after handoff, the only consumer of that session's event channel is `WatchTerminalEvents`, which exists solely to catch the terminal `session_ended` event for pool cleanup (#1438) — every other event arriving mid-call was silently dropped. This meant BR-AI-086/#1635's live reasoning-content streaming (and, more generally, any KA progress narration) was only reachable during the initial `kubernaut_investigate` call, not during the follow-up conversation turns that make up the bulk of an interactive session.

Discovered while adding E2E coverage for #1635: the real KA→MCP→AF→SSE journey test for a `kubernaut_message`-driven turn could not observe the events KA's own logs confirmed it emitted — filed as #1637.

## Alternatives Considered

### Alternative A — Dedicated MCP session per pooled call, mirroring `StartInvestigation` (rejected)

**Approach**: Open a second, dedicated MCP session with its own `LoggingMessageHandler` for every `kubernaut_message` call, bridging its events the same way the initial investigate call does.

**Cons**:
- Doubles MCP connections per interactive turn, defeating the entire purpose of session pooling (#1306).
- KA's per-RR interactive session/lease model (`getSessionMutex(rrID)`, single-driver enforcement) is not designed for a second concurrent connection observing the same session's notifications — unclear whether KA would even deliver them there.
- Reconnect/teardown coordination between two live sessions for the same logical interaction adds real complexity for no benefit over reusing the connection that is already open and already receiving the notifications.

### Alternative B — Poll-based progress instead of live events (rejected)

**Approach**: Have `kubernaut_message` return a richer progress log in its RPC response instead of relying on live notifications; AF surfaces it after the call completes.

**Cons**:
- Reverts the live-streaming UX that #1635 exists to deliver — progress becomes visible only in bulk, after the fact, not while it happens.
- Requires a new KA response schema; far more invasive than reusing an already-open channel.

### Alternative C — Second goroutine consuming the same channel concurrently (rejected)

**Approach**: Keep `WatchTerminalEvents` as-is (terminal-only) and add a second, separate goroutine that reads live events during a pooled call.

**Cons**:
- Unsafe: a Go channel delivers each message to exactly one reader. Two goroutines racing to read the same channel would non-deterministically steal events from each other (a terminal event could be lost to the "live" reader, or a live event lost to the terminal watcher).
- Would require a broadcast/fan-out wrapper (subscriber list, buffering, backpressure policy) — real complexity for no benefit over a single consumer that already knows how to reach the event bridge.

### Alternative D — `EventRelay` attach/detach pointer on the existing pooled entry, consumed by a generalized `WatchTerminalEvents` (chosen)

**Approach**:
- `ka.EventRelay`: a small mutex-guarded holder of the `context.Context` (and therefore, transitively, the `launcher.EventBridge`) belonging to whichever pooled call is currently in flight for a given `(rr_id, username)` session. `Attach(ctx) (detach func())` records it for the call's duration; `Current()` returns it (or `nil` when idle).
- `KASessionPool.InjectVerified` (the handoff path used after a blocking `kubernaut_investigate`) now constructs one `EventRelay` per entry and returns it alongside the existing `error`. `RelayFor(rrID, username)` looks it up later.
- `PooledMCPClient.callPooledTool` — the single choke point already shared by `InvokeAction`, `DiscoverWorkflows`, `SelectWorkflow`, and `CompleteNoAction` — attaches the current call's `ctx` to the relay before `CallTool` and detaches immediately after, via `defer`.
- `WatchTerminalEvents` (renamed in spirit, not in name — same function, extended) takes the `*EventRelay` and, for every non-terminal event, checks `relay.Current()`: if a call is in flight, the event is relayed live to that call's `EventBridge` via the existing `emitEventToA2A`/`FormatEventForUser` machinery (unchanged); if idle, the event is dropped exactly as before (#1438's original behavior, byte-for-byte).

**Pros**:
- Reuses the MCP session and its already-open notification stream — confirmed at the SDK level (`go-sdk` v1.6.1's streamable HTTP client design: a persistent, connection-lifetime "standalone SSE stream" delivers server-initiated notifications independently of any in-flight POST/`CallTool`, so mid-call delivery is a transport guarantee, not a timing accident).
- Exactly one consumer goroutine per pooled entry — no fan-out, no new concurrency primitives beyond a mutex around a single pointer.
- `emitEventToA2A`'s existing type-based routing (`isStatusEvent`, `EventTypeReasoningContentDelta` → `EmitReasoningContentSafe`, default → `EmitReasoningSafe`) needed no changes — every KA event type that can be relayed from the initial `kubernaut_investigate` call is automatically relayed from pooled calls too, not just `reasoning_content_delta`.
- No KA-side change and no new MCP connection.

**Cons**:
- `InjectVerified`'s signature changes (`error` → `(*EventRelay, error)`) and `WatchTerminalEvents` gains a required parameter — both are internal, monorepo-only functions with no external consumers, so this is a compiler-checked, one-line edit at each of the 11 existing call sites, not a compatibility break.
- A pooled session that never went through the `kubernaut_investigate` handoff (e.g. a directly-`Acquire`d session created fresh by the pool factory) has no events channel and therefore no relay (`RelayFor` returns `nil`) — pre-existing behavior, not a regression introduced here.

### Decision

**Alternative D.** Approved 2026-07-08. Reusing the already-open, already-receiving session avoids doubling connections or inventing a new response schema, and a single attach/detach pointer is the minimal mechanism that lets the one existing consumer goroutine discover which call is "live" without unsafe multi-reader channel access.

## Consequences

### Positive
- Closes #1637: `kubernaut_message` (and `discover_workflows`/`select_workflow`/`complete_no_action`) conversation turns now stream KA's live progress — reasoning narration, reasoning content, tool-call starts, errors — exactly as the initial investigation does, closing a previously silent gap during the majority of an interactive session's lifetime.
- No wire/schema change for any SSE consumer (Console included): the same event types and metadata shapes now simply reach the stream from a code path that previously dropped them.
- `WatchTerminalEvents`'s original terminal-only behavior is preserved exactly when no relay is attached (idle sessions, or sessions without an events channel) — zero regression risk for #1438/#1442's existing test suites.

### Negative
- The stale-session retry path in `callPooledTool` (evict-and-reconnect on a dead pooled session) acquires a fresh session with no relay, so a retried call after a stale-session eviction has no live relay for that one call — an accepted, narrow edge case, not a regression (there was never a relay for freshly-`Acquire`d sessions in the first place).

## Related Decisions
- **Builds on**: DD-LLM-009 (the `reasoning_content_delta` event type this relay carries end-to-end for the first time on the `kubernaut_message` path)
- **Orthogonal to**: #1634 (KA-side `reasoning_delta` payload key fix; independent of AF's relay wiring)

---

**Document Control**:
- **Created**: 2026-07-08
- **Version**: 1.0
- **Status**: Approved
