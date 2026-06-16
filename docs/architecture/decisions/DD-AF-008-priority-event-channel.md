# DD-AF-008: Index-Based Alert Prioritization Response

**Status**: Accepted
**Date**: 2026-06-15
**Issue**: [#1434](https://github.com/jordigilh/kubernaut/issues/1434)
**Related**: DD-AF-005 (Alert Prioritization Algorithm), BR-AF-STREAM-001

## Context & Problem

The `list_alerts` tool returns a `ListAlertsResult` containing both an `alerts[]`
array and a `prioritized` field. The `prioritized` field duplicates full alert
objects (selected, tied, also_active), causing the serialized response to exceed
the 4096-byte `MaxToolResultBytes` limit at just 4 alerts.

The tool-level trimmer (`TrimSliceToFit`) checks only the `[]AlertSummary` slice
size, not accounting for the wrapper struct and duplication. The session-level
trimmer (`trimEventFunctionResponses`) then replaces the entire response with a
128-byte stub, causing the LLM to detect truncation and retry in a loop.

## Alternatives Considered

### Alternative 1: Increase maxToolOutputBytes to 8-16KB

**Pros**: Trivial code change, no contract change.
**Cons**: Increases etcd pressure (session state stored in CRDs), doesn't fix the
architectural duplication, only delays the problem.
**Rejected**: Violates etcd safety constraint.

### Alternative 2: Fix TrimSliceToFit to check full result size

**Pros**: Minimal contract change.
**Cons**: Still wastes ~50% of budget on duplicated data. With 4KB budget and 2x
duplication, only fits ~3 alerts — below the 5-alert target.
**Rejected**: Insufficient capacity.

### Alternative 3: Index-based prioritization (SELECTED)

Replace the `prioritized` field's full alert copies with positional indices into
the already-sorted `alerts[]` array. Add `total_count` for truncation awareness.

**Pros**:
- Eliminates 48% of payload (measured: 6146→3168 bytes for 5 alerts)
- Fits 5-6 realistic alerts within 4KB
- `alerts[]` is already sorted by priority, so indices are meaningful
- No information loss — LLM reads `alerts[selected_index]` for full detail

**Cons**:
- Breaking API contract change for `prioritized` field
- LLM prompt must be updated to understand indices

## Decision

**Accepted: Alternative 3** — Index-based prioritization.

### New Response Contract

```json
{
  "alerts": [ ...full AlertSummary objects sorted by priority... ],
  "count": 5,
  "total_count": 12,
  "truncated": true,
  "prioritized": {
    "selected_index": 0,
    "tied_indices": [1],
    "also_active_start": 2
  }
}
```

### Trimming Strategy

Replace `TrimSliceToFit` usage with `trimResultToFit` that:
1. Builds the complete `ListAlertsResult` (with sorted alerts + index-based prioritized)
2. Marshals the full struct to check total size
3. Removes alerts from the tail (lowest priority) until the result fits in 4KB
4. Adjusts `count` and `prioritized` indices accordingly

### Agent Behavior

The LLM uses existing filters (`namespace`, `severity`, `state`) to narrow results.
No pagination is needed. The `total_count` field tells the LLM how many alerts
exist beyond the visible set.

## Consequences

**Positive**:
- 5 realistic alerts fit within 4KB (measured: 3168 bytes)
- Eliminates the 7-retry loop observed in #1434
- Reduces token waste ($0.07-0.35 per incident)
- Preserves etcd safety (no limit increase)

**Negative**:
- Breaking change to `prioritized` field shape (internal API only, no external consumers)
- Prompt update required (minimal — 3 lines)

## Validation

- UT: 5 alerts with 10 labels + 3 annotations each serialize under 4096 bytes
- UT: `trimResultToFit` correctly trims and adjusts indices
- UT: `total_count` reflects pre-truncation count
- IT: Full HandleListAlerts → session trimmer pipeline does not replace result with stub
