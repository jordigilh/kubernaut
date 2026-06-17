# DD-AF-002: Migration to A2A-Go v2 Native Executor with Dual Wire-Format Support

**Status**: APPROVED
**Decision Date**: 2026-06-17
**Version**: 2.0
**Confidence**: 92%
**Applies To**: API Frontend (AF) -- A2A Streaming Stack
**Gate Condition**: ~~Kagenti v1.0 with A2A protocol v2 wire format support~~ Removed -- dual wire-format support via `a2acompat/a2av0` eliminates this dependency

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-06-05 | Architecture Team | Initial decision: defer v2 migration, document scope and design. |
| 2.0 | 2026-06-17 | Architecture Team | Status changed from DEFERRED to APPROVED. Confidence spikes validated channel-merge pattern, compat layer roundtrip, SSE disconnect portability, and test double migration. Dual wire-format support via `a2acompat/a2av0` removes Kagenti v1.0 gate condition. |

---

## Context & Problem

### Current State

AF uses the **deprecated** `google.golang.org/adk/server/adka2a` package (the v0 shim)
to expose ADK agents via the A2A protocol. This shim:

1. Accepts v0 types (`github.com/a2aproject/a2a-go/a2a`) from our callbacks
2. Internally delegates to the v2 executor (`google.golang.org/adk/server/adka2a/v2`)
3. Converts every event and part between v0 and v2 via `a2acompat/a2av0`

The v2 native package (`google.golang.org/adk/server/adka2a/v2`) uses
`github.com/a2aproject/a2a-go/v2/a2a` types directly, eliminating the conversion layer.

### Root Cause of the `{"value": {...}}` Bug

The shim's `GenAIPartConverter` wrapper calls `a2av0.ToV1Part(legacyPart)` to convert
our returned `a2a.Part` to the v2 `*a2a.Part` struct. `ToV1Part` uses a Go type switch:

```go
switch c := p.(type) {
case a2alegacy.TextPart:   // matches VALUE type only
    return &a2a.Part{Content: a2a.Text(c.Text), ...}
default:
    return &a2a.Part{Content: a2a.Data{Value: c}, ...}  // wraps unknown types
}
```

Our code returned `*a2a.TextPart` (pointer). The type switch matched the value type
`a2alegacy.TextPart`, not the pointer, so it fell through to `default` and wrapped the
entire struct as `a2a.Data{Value: c}`. When serialized, this produced
`{"value": {"kind": "text", "text": "..."}}`.

**Fix applied**: Changed all `&a2a.TextPart{...}` to `a2a.TextPart{...}` (value type)
and all `*a2a.TextPart` type assertions to `a2a.TextPart`. See commit `2e5f803e6`.

---

## Decision

**Proceed with migration to v2 native** using dual wire-format support via
`a2acompat/a2av0`. The original gate condition (Kagenti v1.0 with v2 wire format)
is no longer blocking.

### Why Now (Updated 2026-06-17)

The `a2acompat/a2av0` package in `a2a-go/v2` provides a complete v0.3-compatible
JSON-RPC server handler (`NewJSONRPCHandler`) that wraps a v2 `RequestHandler`
and performs bidirectional event conversion. This allows kubernaut to:

1. Migrate internal code to v2 types and the iterator-based executor model
2. Serve the v0.3 wire format to Kagenti v0.6.0 via the compat handler
3. Optionally serve the v2 wire format to v2-capable clients on a separate endpoint

Confidence spikes (Phase 0.5, 2026-06-17) validated all five critical architectural
patterns. See [Spike Findings](#spike-findings-phase-05) below.

### Original Blocking Concern (Resolved)

Migrating to v2 native changes the SSE wire format in ways that break Kagenti v0.6.0:

| Concept | v0 Wire Value | v2 Wire Value | Kagenti v0.6.0 Expects |
|---------|--------------|---------------|------------------------|
| Task state (completed) | `"completed"` | `"TASK_STATE_COMPLETED"` | `"COMPLETED"` |
| Task state (failed) | `"failed"` | `"TASK_STATE_FAILED"` | `"FAILED"` |
| Task state (working) | `"working"` | `"TASK_STATE_WORKING"` | `"WORKING"` |
| Message role (agent) | `"agent"` | `"ROLE_AGENT"` | N/A |
| Protocol version | `"0.3.0"` | `"1.0"` | N/A |
| `TaskStatusUpdateEvent.Final` | `true`/`false` | **removed** | `result.get("final", False)` |

**Resolution**: The `a2acompat/a2av0` compat handler converts v2 events back to v0.3
wire format before SSE serialization. Spike 4 validated that all kubernaut event types
(reasoning, status, keepalive, artifact with DataPart+TextPart, RR context metadata)
roundtrip correctly. The `Final` field is correctly synthesized from terminal state
detection (`State.Terminal() || State == InputRequired`).

### Former Gate Condition (Removed)

~~Proceed with v2 migration when **all** of these are met:~~

1. ~~Kagenti v1.0 is released with A2A protocol v2 support~~
2. ~~Kagenti v1.0 handles `TASK_STATE_*` prefixed states~~
3. ~~Kagenti v1.0 infers terminality from state (not `final` field)~~
4. ~~All other A2A clients in the ecosystem are v2-compatible~~

These conditions are no longer required. Dual wire-format support allows migration
to proceed independently of downstream client readiness.

---

## Migration Scope (Future Reference)

### Production Files (6)

| File | Key Changes |
|------|-------------|
| `pkg/apifrontend/launcher/launcher.go` | `adka2a` -> `adka2a/v2`, `a2asrv.RequestContext` -> `a2asrv.ExecutorContext`, callback signatures, handler options |
| `pkg/apifrontend/launcher/part_converter.go` | `GenAIPartConverter` returns `*a2a.Part` (struct) instead of `a2a.Part` (interface); `a2a.TextPart{...}` -> `a2a.NewTextPart(...)` |
| `pkg/apifrontend/launcher/event_bridge.go` | `eventqueue.Queue.Write(ctx, event)` -> `eventqueue.Writer.Write(ctx, *Message)`; `a2a.TextPart` -> `a2a.NewTextPart()` |
| `pkg/apifrontend/launcher/streaming_executor.go` | `Execute(ctx, *RequestContext, Queue) error` -> `Execute(ctx, *ExecutorContext) iter.Seq2[Event, error]` |
| `pkg/apifrontend/launcher/session_interceptor.go` | `CallInterceptor.Before` gains early-return (`any`); `MessageSendParams` -> `SendMessageRequest` |
| `pkg/apifrontend/handler/agentcard.go` | `a2a.Version` changes from `"0.3.0"` to `"1.0"` |

### Test Files (9)

`event_bridge_test.go`, `part_converter_test.go`, `streaming_executor_test.go`,
`session_interceptor_test.go`, `audit_emission_test.go`, `export_test.go`,
`test_helpers_test.go`, `ka_investigate_mcp_test.go`, `testhelpers_test.go`

### Key Design Decision: StreamingExecutor + EventBridge

The v2 `AgentExecutor.Execute` returns `iter.Seq2[a2a.Event, error]` instead of
accepting an `eventqueue.Queue`. The server drains the iterator internally via
an `eventpipe.Writer`.

The `EventBridge` currently writes side-channel events (keepalives, reasoning,
status updates) directly to the queue. With v2, the queue is not exposed to the
executor. Options:

1. **Channel-merge pattern**: Run the inner executor's iterator in a goroutine
   sending to a channel; `EventBridge` also sends to the same channel;
   `StreamingExecutor.Execute` yields from the merged channel.
2. **ExecutorContextInterceptor**: Use the v2 `WithExecutorContextInterceptor`
   hook to inject the bridge into context and use a shared writer registry.
3. **AfterEventCallback enrichment**: Emit bridge events as additional yields
   after each ADK event via the callback chain.

**Selected: Option 1 (channel-merge)** -- validated by Spike 1+2. See
[Spike Findings](#spike-findings-phase-05) for details.

#### Validated Channel-Merge Lifecycle

The spike discovered that the shutdown lifecycle must have a clear
primary-signals-auxiliaries ordering. A flat `WaitGroup` across all producers
causes deadlock. The correct pattern:

```
Producer goroutines:
  1. Inner executor (PRIMARY) -- closes `done` channel when finished
  2. Keepalive ticker (AUXILIARY) -- stops on `done` or ctx.Done()
  3. Bridge side-channel (AUXILIARY) -- stops on `done` or ctx.Done()

Lifecycle:
  inner.Execute() finishes -> close(done) -> auxiliaries exit -> wg.Wait() -> close(merged)
```

Re-invocation chaining (BR-SESS-013) is handled by running sequential
`inner.Execute()` calls within the primary producer goroutine. Keepalive and
bridge continue emitting across all re-invocation rounds with no gaps.

### Key Design Decision: SSE Disconnect Detection

v2's `LocalManager.Execute` uses `context.WithoutCancel` to detach the executor
goroutine from the HTTP context -- identical to v0.3's behavior.
`AgentInactivityTimeout` is an orthogonal liveness watchdog, not a disconnect
detector.

**Decision**: Kubernaut's `WithSSEDisconnectCtx` pattern is still needed and
correctly designed under v2. Context values survive `WithoutCancel`, so the
stashed HTTP request context remains a reliable disconnect signal inside the
detached goroutine. No migration changes required for SSE disconnect handling.
Validated by Spike 3.

### Key Design Decision: Dual Wire-Format Support

The `a2acompat/a2av0.NewJSONRPCHandler` wraps a v2 `RequestHandler` and serves
the v0.3 wire format. The handler:

1. Accepts v0.3 JSON-RPC requests (camelCase keys, `kind` discriminator)
2. Converts to v2 types via `ToV1*` functions
3. Delegates to the v2 `RequestHandler`
4. Converts v2 responses back to v0.3 via `FromV1*` functions
5. For streaming: iterates the v2 `iter.Seq2`, converts each event, and writes
   to SSE via a channel-based producer/consumer

The v0.3 wire format uses camelCase keys (`contextId`, `taskId`, `messageId`)
with a `kind` discriminator (`"kind":"status-update"`, `"kind":"text"`). The
`Final` flag is synthesized from `State.Terminal() || State == InputRequired`.
Validated by Spike 4.

### Key Design Decision: Test Double Migration

The `fakeQueue`-based test pattern (`queue.events[]` inspection after execution)
is replaced by an `iter.Seq2`-based pattern:

| v0.3 Pattern | v2 Pattern |
|---|---|
| `queue := &fakeQueue{}` | `exec := &fakeIterExecutor{events: [...]}` |
| `executor.Execute(ctx, reqCtx, queue)` | `seq := exec.Execute(ctx)` |
| `Expect(queue.events).To(HaveLen(N))` | `events, err := collectEvents(seq)` |
| `queue.events[i].(*a2a.TaskStatusUpdateEvent)` | `events[i]` (already typed) |
| `queue.Write(ctx, evt) error` | Error checked during iteration |
| `queue.closed` | Iterator naturally terminates |

Helper functions: `collectEvents()`, `collectAllEvents()`, `filterBySource()`.
The migration is mechanical. Validated by Spike 5.

### Import Path Changes

| v0 | v2 |
|----|-----|
| `github.com/a2aproject/a2a-go/a2a` | `github.com/a2aproject/a2a-go/v2/a2a` |
| `github.com/a2aproject/a2a-go/a2asrv` | `github.com/a2aproject/a2a-go/v2/a2asrv` |
| `github.com/a2aproject/a2a-go/a2asrv/eventqueue` | `github.com/a2aproject/a2a-go/v2/a2asrv/eventqueue` |
| `google.golang.org/adk/server/adka2a` | `google.golang.org/adk/server/adka2a/v2` |

### go.mod Changes

- Promote `github.com/a2aproject/a2a-go/v2 v2.3.1` from indirect to direct
- Remove `github.com/a2aproject/a2a-go v0.3.15` once all imports are migrated

---

## Risks

| Risk | Severity | Mitigation | Spike Validation |
|------|----------|------------|-----------------|
| Wire format breaks Kagenti v0.6 | ~~High~~ **Low** | Dual wire-format via `a2acompat/a2av0`; v0.3 endpoint for legacy clients | Spike 4: all event types roundtrip correctly |
| StreamingExecutor rewrite introduces concurrency bugs | Medium | Channel-merge with primary/auxiliary shutdown ordering; race-detector testing | Spike 1: deadlock discovered and fixed in PoC; validated pattern is safe |
| Re-invocation loop breaks across rounds | Medium | Sequential rounds within primary producer goroutine; auxiliaries span lifecycle | Spike 2: keepalive continues across rounds, bridge interleaves correctly |
| SSE disconnect detection breaks under v2 | ~~Medium~~ **None** | `WithSSEDisconnectCtx` pattern is portable; no changes required | Spike 3: v2 uses identical `context.WithoutCancel` pattern |
| Test rewrite introduces regressions | Low | Mechanical migration with `collectEvents`/`filterBySource` helpers | Spike 5: 1:1 replacement pattern validated for all test scenarios |
| v0 shim introduces new conversion bugs | Low | Value-type TextPart fix prevents the known class; migration eliminates the shim | N/A (pre-existing) |

---

## Spike Findings (Phase 0.5)

Five confidence-raising spikes were executed on 2026-06-17. All spike code lives
in `pkg/apifrontend/launcher/spike/` (3 test files, 24 tests total). These are
throwaway prototypes to be deleted before the actual migration begins.

### Spike 1: Channel-Merge PoC (8 tests, PASS)

**Objective**: Validate merging 3 concurrent event sources (inner executor,
keepalive ticker, EventBridge side-channel) into a single `iter.Seq2[Event, error]`.

**Key finding**: The shutdown lifecycle must use a primary/auxiliary ordering
pattern. The inner executor is the primary producer that closes a `done` channel
when finished, signaling auxiliary producers (keepalive, bridge) to stop. A flat
`WaitGroup` across all producers causes deadlock because `wg.Wait()` blocks on
auxiliaries that are waiting on a stop signal that is only sent after `wg.Wait()`
completes.

**Tests validated**: basic ordering, keepalive emission during slow execution,
context cancellation, no goroutine leaks, error propagation.

### Spike 2: Re-invocation Chaining PoC (3 tests, PASS)

**Objective**: Validate flattening N sequential `inner.Execute()` calls into one
output `iter.Seq2`, with keepalive and bridge continuing across rounds.

**Key finding**: Running sequential re-invocation rounds inside the primary
producer goroutine works correctly. Keepalive ticks fire continuously across
round boundaries (no gap). Bridge side-channel events interleave correctly during
each round.

**Tests validated**: multiple rounds with conditional re-invocation, keepalive
continuity across rounds, bridge writes during execution.

### Spike 3: SSE Disconnect Preflight (analysis, PASS)

**Objective**: Determine whether v2's `LocalManager` uses `context.WithoutCancel`
and whether kubernaut's `WithSSEDisconnectCtx` pattern is still needed.

**Key finding**: v2's `LocalManager.Execute` explicitly detaches execution via
`context.WithoutCancel`, identical to v0.3. `AgentInactivityTimeout` is an
orthogonal liveness watchdog, not a client-disconnect sensor. Context values
survive `WithoutCancel`, so the stashed HTTP context remains a reliable
disconnect indicator.

**Conclusion**: `WithSSEDisconnectCtx` is portable to v2 with zero changes.
This risk item is eliminated entirely.

### Spike 4: a2acompat/a2av0 Roundtrip Validation (8 tests, PASS)

**Objective**: Validate that kubernaut's EventBridge event types survive the
v2 -> v0.3 -> v2 conversion roundtrip through the `a2acompat/a2av0` layer.

**Key findings**:
- All kubernaut event types roundtrip correctly: `TaskStatusUpdateEvent` with
  reasoning/status/keepalive metadata, RR context metadata (7 fields),
  `TaskArtifactUpdateEvent` with `DataPart` + `TextPart`.
- Keepalive events (metadata-only, no message body) convert correctly.
- All 8 v2 task states map bidirectionally.
- Terminal state -> `Final` flag mapping works: `State.Terminal() || State == InputRequired`.
- Wire format uses camelCase keys (`contextId`, `taskId`, `messageId`) with
  `kind` discriminator (`"kind":"status-update"`, `"kind":"text"`).

**Tests validated**: reasoning metadata, RR context metadata, keepalive events,
artifact with DataPart+TextPart, terminal state Final flag, JSON wire format key
names, bidirectional task state mapping, bidirectional message role mapping.

### Spike 5: Test Double Pattern (8 tests, PASS)

**Objective**: Prototype `iter.Seq2`-based test helpers to replace `fakeQueue`.

**Key finding**: The migration is mechanical. `fakeIterExecutor` (returning
`iter.Seq2[Event, error]`) is a 1:1 replacement for `fakeQueue`. Helper functions
`collectEvents()`, `collectAllEvents()`, and `filterBySource()` replace
`queue.events[]` inspection.

**Tests validated**: single event emission, multiple emissions with ordering,
error injection, context cancellation, concurrent bridge writes during execution,
event type assertions, comparison with fakeQueue pattern, collectAllEvents with
errors.

### Confidence Impact

| Phase | Before Spikes | After Spikes | Delta |
|---|---|---|---|
| Executor Model Redesign | ~65% | ~90% | +25% |
| SSE Disconnect Handling | ~75% | ~98% | +23% |
| Compat Layer (Kagenti) | ~70% | ~95% | +25% |
| Test Rewrite | ~60% | ~85% | +25% |
| **Overall** | **~78%** | **~92%** | **+14%** |

---

## References

- ADK v1.4.0 deprecation notice: `google.golang.org/adk/server/adka2a/executor.go` line 17
- a2a-go v2 core types: `github.com/a2aproject/a2a-go/v2@v2.3.1/a2a/core.go`
- a2av0 compat layer: `github.com/a2aproject/a2a-go/v2@v2.3.1/a2acompat/a2av0/conversions.go`
- a2av0 JSON-RPC server: `github.com/a2aproject/a2a-go/v2@v2.3.1/a2acompat/a2av0/jsonrpc_server.go`
- a2av0 JSON key transform: `github.com/a2aproject/a2a-go/v2@v2.3.1/a2acompat/a2av0/json_transform.go`
- v2 local task manager: `github.com/a2aproject/a2a-go/v2@v2.3.1/internal/taskexec/local_manager.go`
- Kagenti upstream: `github.com/kagenti/kagenti/kagenti/backend/app/routers/chat.py`
- TextPart fix commit: `2e5f803e6`
- Spike PoC code: `pkg/apifrontend/launcher/spike/` (throwaway, delete before migration)
- Tracking issue: GitHub #1448
