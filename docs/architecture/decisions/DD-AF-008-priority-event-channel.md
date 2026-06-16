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

---

## Addendum: Per-Type EventBridge Text Limits (#1435)

**Date**: 2026-06-16
**Issue**: [#1435](https://github.com/jordigilh/kubernaut/issues/1435)

### Problem

The EventBridge `sanitizeBridgeText` applied a 512-rune hard cap to **all** text
flowing through the bridge, regardless of event type. LLM reasoning text
(investigation plans, tool argument listings, RCA analysis) routinely exceeds 512
characters, causing mid-sentence truncation visible in the Console thinking panel.

The truncation also triggered secondary issues: the LLM detects garbled output and
produces text-only turns, which the streaming executor's re-invocation loop
interprets as premature turn ends, causing ghost re-invocation cascades.

### Root Cause

`sanitizeBridgeText` (line 284) applied a single `maxBridgeTextLen = 512` limit to
all text. `EmitReasoning`, `EmitOutput`, and `EmitStatus` all called
`EmitStatusWithMeta` which applied this limit uniformly.

### Decision: Per-Type Limits

| Event Type | Metadata Type | Rune Limit | Rationale |
|---|---|---|---|
| Reasoning | `reasoning` | 4096 | LLM inner thoughts can be multi-paragraph |
| Output | `output` | 4096 | Final LLM answers can be multi-paragraph |
| Status | `status` | 512 | Ephemeral messages ("Connecting to KA...") are short |
| Structured | (various) | No limit | Already handled by `sanitizeStructuredText` |

### Implementation

- New `sanitizeTextWithLimit(ctx, text, maxLen)` function replaces the monolithic
  `sanitizeBridgeText` with a parameterized variant that logs truncation events.
- `EmitReasoning` and `EmitOutput` call `emitWithLimit(ctx, text, 4096, meta)`
  bypassing the 512-rune path.
- `EmitStatus` and `EmitStatusWithMeta` continue using `sanitizeBridgeText` (512).
- `NeedsReinvocationCtx` checks `ctx.Err()` before evaluating re-invocation,
  preventing ghost cascades when the context is cancelled.

### Validation

- UT-AF-1435-001: 4000-rune reasoning passes through intact
- UT-AF-1435-002: 5000-rune reasoning truncated at 4096 with ellipsis
- UT-AF-1435-003: 600-rune status text still truncated at 512
- UT-AF-1435-004: 4000-rune output passes through intact
- UT-AF-1435-005: Real-world 700-char reasoning (issue scenario) passes intact
- UT-AF-1435-010: Re-invocation blocked when context is cancelled
- UT-AF-1435-011: Re-invocation still fires when context is healthy
