# DD-AF-008: Priority-Based Event Channel for MCP Investigation Stream

**Status**: Approved
**Date**: 2026-06-15
**Last Reviewed**: 2026-06-15
**Confidence**: 90%
**Author**: AI Assistant
**Issue**: [#1433](https://github.com/jordigilh/kubernaut/issues/1433)
**BR**: [BR-AF-STREAM-001](../../requirements/BR-AF-STREAM-001-priority-event-delivery.md)

---

## Context & Problem

The AF MCP client (`pkg/apifrontend/ka/mcp_sdk_client.go`) receives investigation events from KA via the MCP SDK's `LoggingMessageHandler`. Events are written to a buffered channel (`make(chan InvestigationEvent, 64)`) using a non-blocking send:

```go
select {
case eventCh <- evt:
case <-doneCh:
default:
    c.logger.Info("event channel full, dropping event", "event_type", evt.Type)
}
```

During active LLM inference (workflow selection, investigation reasoning), KA produces `token_delta` events at high frequency (~50-100/s). When the bridge goroutine (`BridgeEventsToA2A`) cannot drain fast enough, the buffer fills and ALL events are dropped indiscriminately — including lifecycle events (`complete`, `error`, `alignment_verdict`) that the system depends on for state transitions.

**Key Requirements**:
- Structural/lifecycle events must never be silently dropped
- Streaming delta events are loss-tolerant (Console renders partial text gracefully)
- Solution must not change the consumer-side channel interface
- Solution must not introduce unbounded memory growth
- Solution must not block the LoggingMessageHandler goroutine indefinitely

---

## Alternatives Considered

### Alternative 1: Increase Buffer Size

**Approach**: Increase channel buffer from 64 to 256 or higher.

**Pros**:
- Minimal code change (one constant)
- No behavioral change to test

**Cons**:
- Does not solve the problem — any finite buffer can fill under sustained output
- Masks the issue rather than addressing it
- Higher memory usage per session (256 * sizeof(InvestigationEvent))
- Does not provide guarantees for structural events

**Confidence**: 30% (rejected — treats symptom, not cause)

---

### Alternative 2: Separate Channels (Structural + Streaming)

**Approach**: Two channels — one for structural events (small buffer, blocking) and one for streaming events (large buffer, non-blocking). Consumer selects on both.

**Pros**:
- Clean separation of concerns
- Structural events have dedicated path

**Cons**:
- Changes consumer interface (must select on 2 channels)
- Existing tests and bridge goroutine need significant rework
- More complex lifecycle management (two channels to close)
- `BridgeEventsToA2A` and `bridgeEventsCollectSummary` both need updates

**Confidence**: 60% (viable but high churn)

---

### Alternative 3: Priority Send at Producer (Classification + Blocking for Structural)

**Approach**: Classify events at the send site. Structural events use a blocking send with a bounded timeout. Streaming events retain non-blocking send with drop. Single channel, unchanged consumer interface.

**Pros**:
- Zero consumer-side changes (channel type unchanged)
- Structural events guaranteed delivered (up to timeout)
- Streaming events still drop gracefully under pressure
- Minimal code change — isolated to LoggingMessageHandler
- Drop counter enables observability without log spam

**Cons**:
- Structural send blocks the LoggingMessageHandler callback (up to 5s)
- If channel is permanently stuck, structural events also eventually drop (with Error log)
- Classification logic must be maintained when new event types are added

**Confidence**: 90% (approved)

---

## Decision

**APPROVED: Alternative 3** — Priority Send at Producer

**Rationale**:
1. **Zero consumer change**: The `<-chan InvestigationEvent` interface remains identical. `BridgeEventsToA2A`, `bridgeEventsCollectSummary`, and all tests continue working unchanged.
2. **Guaranteed delivery for lifecycle events**: A 5s blocking timeout is far beyond any realistic channel drain latency (the bridge processes events in <1ms each). The only scenario where structural events drop is if the consumer is completely dead — which means the session is already broken.
3. **Minimal blast radius**: The change is isolated to ~20 lines in `StartInvestigation`'s LoggingMessageHandler closure and a small helper function for classification.

**Key Insight**: The problem is asymmetric — streaming events are loss-tolerant (partial text is acceptable) while structural events are loss-intolerant (missed `complete` = stuck session). The solution should reflect this asymmetry at the send site rather than adding complexity to the receive site.

---

## Implementation

### Event Classification

```go
func IsStructuralEvent(eventType string) bool {
    switch eventType {
    case EventTypeComplete, EventTypeError, EventTypeCancelled, EventTypeAlignmentVerdict:
        return true
    default:
        return false
    }
}
```

### Priority Send Pattern

```go
if IsStructuralEvent(evt.Type) {
    select {
    case eventCh <- evt:
    case <-doneCh:
    case <-time.After(structuralSendTimeout):
        c.logger.Error(nil, "CRITICAL: structural event dropped after timeout",
            "event_type", evt.Type, "rr_id", args.RRID)
        atomic.AddInt64(&structuralDrops, 1)
    }
} else {
    select {
    case eventCh <- evt:
    case <-doneCh:
    default:
        atomic.AddInt64(&streamingDrops, 1)
    }
}
```

### Primary Implementation Files
- `pkg/apifrontend/ka/event_priority.go` — `IsStructuralEvent()` + constants
- `pkg/apifrontend/ka/mcp_sdk_client.go` — Updated LoggingMessageHandler in `StartInvestigation`

### Data Flow
1. KA emits LoggingMessage via MCP SDK
2. `LoggingMessageHandler` callback fires, parses event
3. `IsStructuralEvent(evt.Type)` classifies
4. Structural → blocking send with 5s timeout
5. Streaming → non-blocking send with `default: drop`
6. Consumer (`BridgeEventsToA2A` / `bridgeEventsCollectSummary`) drains as before

### Graceful Degradation
- If channel is stuck for >5s: structural event is logged at Error and dropped (session is likely dead anyway)
- If LoggingMessageHandler blocks for 5s: subsequent events queue in the MCP SDK's internal buffer (SDK handles its own backpressure)

---

## Consequences

**Positive**:
- Lifecycle events (`complete`, `error`, `alignment_verdict`) reliably reach the bridge and Console
- Drop counter provides observability into backpressure frequency
- Buffer increase (64→128) reduces drop frequency for streaming events too
- No test rework required (consumer unchanged)

**Negative**:
- LoggingMessageHandler may block up to 5s per structural event if channel is full — **Mitigation**: This scenario implies the consumer is stuck, so the 5s delay is acceptable (session will timeout anyway)
- New event types must be explicitly classified — **Mitigation**: Default to streaming (safe) with a lint rule to check new constants

**Neutral**:
- Channel buffer increase from 64 to 128 doubles per-session memory footprint (~8KB → ~16KB, negligible)
- Drop counter adds one atomic.Int64 per session (8 bytes)

---

## Validation Results

**Confidence Assessment Progression**:
- Initial assessment: 85% confidence
- After alternatives analysis: 90% confidence
- After implementation review: 90% confidence (pending test validation)

**Key Validation Points**:
- Consumer interface unchanged (verified by existing test compilation)
- `time.After` in select is safe in callback context (no goroutine leak — timer is GC'd after select)
- Atomic counters are lock-free and have no contention with single-writer pattern

---

## Related Decisions

- **Builds On**: DD-AF-002 (A2A v2 Native Migration) — EventBridge is the consumer
- **Supports**: BR-AF-STREAM-001 (Priority Event Delivery)
- **Related**: #1389 (Event channel lifecycle management) — prior work on channel close semantics

---

## Review & Evolution

**When to Revisit**:
- If new structural event types are added (must update `IsStructuralEvent`)
- If KA event frequency increases 10x+ (may need adaptive buffer sizing)
- If multiple concurrent investigations per AF instance are introduced (per-session channels already isolated)

**Success Metrics**:
- Structural event drop count = 0 in production (Prometheus alert if > 0)
- Streaming event drop rate < 5% of total events (acceptable loss)
- Console receives all `complete`/`error` events (end-to-end validation)
