# DD-GATEWAY-013: Async Status Update Pattern for Deduplication and Storm Aggregation

## Status

**✅ APPROVED** (2025-12-10)
**Last Reviewed**: 2025-12-10
**Confidence**: 85%

## Context & Problem

DD-GATEWAY-012 removed Redis from Gateway, moving all deduplication and storm aggregation state to Kubernetes RemediationRequest (RR) status fields. When a duplicate alert is detected, Gateway needs to:

1. **Update `status.deduplication`** - Increment occurrence count, update lastSeenAt
2. **Update `status.stormAggregation`** - Increment aggregated count, set storm flag if threshold reached

**Key Requirements**:
- Minimize HTTP response latency (P95 <50ms target)
- Deduplication count should be accurate in HTTP response
- Storm aggregation is non-critical for HTTP response
- Status updates should not fail the request
- Audit event emission will use similar pattern (future)

**Problem**: Each K8s status update adds ~10-30ms latency. With retry-on-conflict, worst case can be 100ms+. This blocks HTTP response.

## Alternatives Considered

### Alternative 1: Current Approach (Synchronous with Graceful Degradation)

**Approach**: Execute both status updates synchronously, log errors but continue.

```go
// Synchronous - blocks HTTP response
if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
    logger.Info("Failed to update dedup status", "error", err)
}
if err := s.statusUpdater.UpdateStormAggregationStatus(ctx, existingRR, isThresholdReached); err != nil {
    logger.Info("Failed to update storm status", "error", err)
}
return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
```

**Pros**:
- ✅ Simple implementation
- ✅ Status always updated before response
- ✅ No goroutine management complexity

**Cons**:
- ❌ Adds 20-60ms latency to HTTP response
- ❌ 2 K8s API calls per duplicate
- ❌ Retry-on-conflict can compound latency

**Confidence**: 70% (rejected - latency concern)

---

### Alternative 2: Full Goroutine Fire-and-Forget

**Approach**: Execute both status updates in a background goroutine.

```go
// Both async - immediate HTTP response
go func() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
        logger.Info("Failed to update dedup status (async)", "error", err)
    }
    if err := s.statusUpdater.UpdateStormAggregationStatus(ctx, existingRR, isThresholdReached); err != nil {
        logger.Info("Failed to update storm status (async)", "error", err)
    }
}()
return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
```

**Pros**:
- ✅ Minimal HTTP response latency
- ✅ Non-blocking for caller

**Cons**:
- ❌ Dedup count in response may be stale (not yet updated)
- ❌ Updates lost if pod crashes mid-update
- ❌ No ordering guarantee for rapid duplicates
- ❌ Both updates still 2 API calls

**Confidence**: 65% (rejected - response accuracy concern)

---

### Alternative 3: Combined Single Update

**Approach**: Merge both status updates into a single K8s API call.

```go
// Single API call - still synchronous
if err := s.statusUpdater.UpdateCombinedStatus(ctx, existingRR, isThresholdReached); err != nil {
    logger.Info("Failed to update combined status", "error", err)
}
return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
```

**Pros**:
- ✅ 1 API call instead of 2
- ✅ Atomic update of both fields
- ✅ Status consistent before response

**Cons**:
- ❌ Still adds 10-30ms latency to HTTP response
- ❌ Retry-on-conflict still adds latency
- ❌ Changes StatusUpdater interface

**Confidence**: 75% (considered but not optimal)

---

### Alternative 4: Hybrid (Sync Dedup, Async Storm) - **SELECTED**

**Approach**: 
- **Synchronous**: Deduplication status (needed for accurate HTTP response)
- **Asynchronous**: Storm aggregation status (non-critical for response)

```go
// Sync: Dedup status (needed for accurate response)
if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
    logger.Info("Failed to update dedup status", "error", err)
}

// Async: Storm status (non-critical for response)
go func() {
    asyncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := s.statusUpdater.UpdateStormAggregationStatus(asyncCtx, existingRR, isThresholdReached); err != nil {
        logger.Info("Failed to update storm status (async)", "error", err)
    }
}()

return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
```

**Pros**:
- ✅ Accurate dedup count in HTTP response
- ✅ Reduced HTTP response latency (only 1 sync API call)
- ✅ Storm status eventually consistent
- ✅ Simple implementation (no shared library needed)
- ✅ Graceful degradation on storm update failure

**Cons**:
- ⚠️ Storm status may lag by 1 update - **Acceptable**: Storm detection is informational
- ⚠️ Async storm update lost on pod crash - **Acceptable**: Next alert will update correctly

**Confidence**: 85% (approved)

---

## Decision

**APPROVED: Alternative 4 (Hybrid)** - Synchronous Deduplication, Asynchronous Storm Aggregation

**Rationale**:
1. **Response Accuracy**: Deduplication count must be accurate for HTTP 202 response (operators rely on this)
2. **Latency Reduction**: Storm aggregation is informational - async saves ~10-30ms per request
3. **Simplicity**: Inline goroutine pattern, no shared library needed
4. **Graceful Degradation**: Storm update failures don't impact core deduplication functionality

**Key Insight**: Storm aggregation status is a "nice to have" metric for observability. Deduplication count is a "must have" for correct API contract.

## Implementation

**Primary Implementation Files**:
- `pkg/gateway/server.go` - ProcessSignal method
- `pkg/gateway/processing/status_updater.go` - StatusUpdater (unchanged)

**Pattern** (inline goroutine):
```go
// Async fire-and-forget pattern - no shared library needed
go func() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := operation(ctx); err != nil {
        logger.Info("Async operation failed", "operation", "name", "error", err)
    }
}()
```

**Why No Shared Library?**:
- Pattern is ~8-10 lines - trivial to inline
- Different contexts (K8s API vs HTTP API for audit)
- Different timeout requirements
- YAGNI - only 2 call sites currently
- Abstraction would add more complexity than it saves

**Future Considerations**:
If we need more sophistication (circuit breaker, metrics, retry queues) for multiple async operations, we can introduce a shared library then.

## Consequences

**Positive**:
- ✅ HTTP response latency reduced by ~10-30ms
- ✅ Accurate deduplication count in responses
- ✅ Simple implementation, easy to understand
- ✅ No new dependencies or abstractions

**Negative**:
- ⚠️ Storm status may lag by 1 update - **Mitigation**: Next alert corrects it
- ⚠️ Async update lost on pod crash - **Mitigation**: Rare, next alert corrects it

**Neutral**:
- 🔄 Pattern can be reused for audit event emission
- 🔄 No change to StatusUpdater interface

## Validation Results

**Confidence Assessment Progression**:
- Initial assessment: 75% confidence
- After latency analysis: 80% confidence
- After simplicity analysis: 85% confidence

**Key Validation Points**:
- ✅ Dedup count accuracy maintained
- ✅ Latency reduction achieved
- ✅ No new abstractions needed
- ✅ Graceful degradation preserved

## Related Decisions

- **Supersedes**: N/A (new pattern)
- **Builds On**: DD-GATEWAY-011 (Status-Based Deduplication), DD-GATEWAY-012 (Redis Deprecation)
- **Supports**: BR-GATEWAY-183 (Optimistic Concurrency), BR-GATEWAY-185 (Redis Deprecation)

## Review & Evolution

**When to Revisit**:
- If more than 3 async fire-and-forget patterns emerge → consider shared library
- If storm status accuracy becomes critical → move to synchronous
- If HTTP latency requirements tighten further → consider full async (Alternative 2)

**Success Metrics**:
- HTTP P95 latency: <50ms (current), target <40ms with this change
- Storm status accuracy: >99% (eventual consistency acceptable)
- Code complexity: No increase in abstraction layers

