# DD-AF-009: A2A Task-Completion Gating for Multi-Turn Re-invocation

**Status**: Accepted
**Date**: 2026-07-29
**Issue**: [#1774](https://github.com/jordigilh/kubernaut/issues/1774)
**Related**: BR-SESS-013 (re-invocation loop), #1435 (`NeedsReinvocationCtx` context-cancellation guard)

## Context

Long-running investigation streams intermittently failed mid-stream with `context canceled` errors, 194-352 seconds into the stream, with no consistent trigger (not a fixed timeout — HAProxy timeout fixes did not resolve it). The variability made this look like an infra flake, but it reproduced across environments.

Root cause, traced through `a2a-go` (`github.com/a2aproject/a2a-go@v0.3.15`) and ADK (`google.golang.org/adk@v1.5.1`) internals:

1. `a2a-go`'s `internal/taskexec/execution_handler.go` runs a producer/consumer pair inside an `errgroup`. The consumer treats the first `a2a.TaskStatusUpdateEvent{State: Completed}` it reads off the queue as definitive task end and returns a sentinel `errConsumerStopped`, which cancels the errgroup's shared context — "there's no point for [the producer] to continue."
2. ADK's `adka2a/v2` executor emits exactly that event at the end of **every** agent turn, including a text-only turn with no tool call.
3. AF's `StreamingExecutor.Execute` implements BR-SESS-013: when the last turn was text-only (no tool call), it injects a synthetic "continue" message and calls `s.inner.Execute` again — but on the **same** `ctx`, which `a2a-go` has by then already canceled per (1)-(2).
4. The re-invoked call doesn't fail immediately (the cancellation is asynchronous relative to the next LLM/tool call), which produced the observed 194-352s variance: the delay is however long the next outbound LLM/tool call took before it happened to check `ctx.Err()`.

This is a variant of #1435, which added a `ctx.Err() != nil` guard to `NeedsReinvocationCtx` to stop the reinvocation loop from firing again *after* a cancellation was already visible. That guard does not help here: the context is not yet visibly canceled at the moment the reinvocation decision is made — the cancellation propagates from the `errgroup` asynchronously, and the intermediate Completed event is the direct cause, not a race AF can out-run by checking `ctx.Err()` earlier.

A secondary defect this also causes: if a fix instead tried to avoid this by writing directly to the queue that `a2a-go`'s own consumer no longer drains, the reinvoked turn's events would be silently dropped once the consumer has exited.

## Decision

Intercept `a2a.TaskStatusUpdateEvent{State: Completed}` at the boundary AF controls — the `eventqueue.Queue` handed to `s.inner.Execute` — rather than modifying `a2a-go` or ADK internals.

`StreamingExecutor.Execute` wraps the real queue in a `completionGateQueue` for the lifetime of one A2A request (spanning all re-invocations):

- **Write**: any non-Completed event (Working status, artifacts, reasoning deltas) is forwarded to the real queue immediately — live streaming visibility is unaffected on every turn, including reinvoked ones.
- A `TaskStateCompleted` status update is **held**, not forwarded.
- Before each re-invocation, the held event is **dropped** (`dropHeld`) — it was premature, produced by a turn that turns out not to be the real end of the task, and must never reach `a2a-go`'s consumer.
- After the re-invocation loop exits (no further re-invocation needed, or an error occurred), the held event — if any — is **flushed** (`flushHeld`) to the real queue exactly once. Only at that point does `a2a-go` observe task completion, and by then AF has already confirmed there will be no further `Execute` call on this context.

The gate is applied once, before `WithEventBridge` is called, so both the ADK's own event emission (via the queue parameter to `Execute`) and AF's side-channel `EventBridge` (used by tool-call handlers to emit artifacts/reasoning deltas) go through the same gated queue — a single point of control, avoiding any risk of divergence if a future code path emits a Completed status through the bridge.

## Alternatives Considered

### A: Push re-invocation into the ADK `Runner`/`BaseFlow` itself

Make the ADK's own agent loop (`internal/llminternal/base_flow.go`) treat a text-only response as "not yet done" and continue internally, so AF never needs to call `Execute` a second time at all.

Rejected: This relies on undocumented ADK internals (`BaseFlow.Run`, `handleFunctionCalls` return-value semantics) that AF does not own and Google can change without notice across ADK releases. It would likely require synthesizing a fake function-call/response pair to keep the ADK's own loop from finalizing the turn, which is a much more invasive and fragile hack than gating one event type at a boundary AF already owns. Spiked and found brittle — no clean hook exists in the v1.5.1 ADK for "don't finalize this turn."

### B: Gate the terminal event at the AF `eventqueue.Queue` boundary (chosen)

Wrap the queue passed into `s.inner.Execute` for the duration of the whole A2A request; hold `Completed` events until re-invocation is confirmed unnecessary.

Chosen: Contained entirely within AF's own code (`pkg/apifrontend/launcher`), does not depend on `a2a-go` or ADK internals beyond the one already-observed event/state contract (`a2a.TaskState`, which is a stable public API), and is easy to reason about and test in isolation with a fake queue. Robust against ADK version bumps as long as ADK keeps emitting a `Completed` status to mark turn end — which is the same contract AF's own `StreamingExecutor` already depends on to render the final response.

### C: Have `NeedsReinvocationCtx` refuse to re-invoke once `err == nil` (status quo, #1435 guard only)

Rely solely on the pre-existing `ctx.Err() != nil` check to stop cascading failures once cancellation becomes visible.

Rejected: This only prevents runaway *subsequent* re-invocation attempts after cancellation is already visible — it does nothing for the re-invocation call that is made concurrently with (and races) the cancellation, which is the actual failure mode observed (194-352s variance is exactly this race). #1435 remains a useful defensive guard and is left in place; it is a different, narrower fix than this one.

## Consequences

### Positive

- Long-running investigations that end a turn in a text-only response no longer fail with `context canceled` on the re-invoked continuation.
- No dependency on undocumented ADK/`a2a-go` internals beyond the stable, public `a2a.TaskState` contract.
- Zero regression to live-streaming visibility: every non-terminal event from every turn (including reinvoked ones) still reaches the client immediately.
- Contained to one file (`streaming_executor.go`); no new public types, no wiring changes to `cmd/apifrontend`.

### Negative

- Adds one more responsibility to `StreamingExecutor.Execute` (queue lifecycle management alongside keepalive and disconnect detection). Mitigated by keeping `completionGateQueue` a small, single-purpose type with its own doc comment explaining the a2a-go/ADK contract it exists to work around.
- If `a2a-go` or ADK ever change how they signal terminal-vs-intermediate turns (e.g., a new state or a `Final` field with different semantics), this gate would need to be revisited. This is an accepted risk consistent with Alternative B's rationale — it is coupled to a stable public API, not an internal implementation detail.

## Test Coverage

| Tier | IDs | Validates |
|------|-----|-----------|
| UT (AF) | UT-AF-1774-001 | Only the final turn's Completed status reaches the downstream queue across a multi-turn re-invocation sequence |
| UT (AF) | UT-AF-1774-002 | Working/non-terminal events from every turn (including reinvoked ones) still reach the queue live — no visibility regression |
| UT (AF) | UT-AF-1774-003 | Regression guard: single-turn completion (no re-invocation) still delivers exactly one Completed event |
| UT (AF) | UT-AF-1774-004 | An error from a re-invoked turn propagates without leaving a stale Completed event to be flushed |
| IT (AF) | IT-AF-REINV-W01, W01b (pre-existing, BR-SESS-013) | Re-invocation wiring itself (unchanged by this fix) continues to pass — no regression in the call-count contract |

## FedRAMP Controls

| Control | Application | Evidence |
|---------|-------------|----------|
| SI-4 (System Monitoring) | Investigation stream stays observable end-to-end instead of terminating mid-session with an opaque cancellation error | UT-AF-1774-001/002 |
| AU-2 (Audit Events) / AU-3 (Content of Audit Records) | Session lifecycle events (stream opened/closed) already logged by `StreamingExecutor`; this fix ensures the "closed" log reflects the true final outcome rather than a premature one | Existing `UT-AF-1258-040/041` logging tests, unaffected |
