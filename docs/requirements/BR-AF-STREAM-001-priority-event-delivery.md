# BR-AF-STREAM-001: Priority Event Delivery — Alert Tool Output Sizing

**Status**: Approved
**Issue**: [#1434](https://github.com/jordigilh/kubernaut/issues/1434)
**Date**: 2026-06-15

## Business Need

The AF agent's `list_alerts` tool must return at least 5 realistic Prometheus alerts
within the 4KB tool result budget so the LLM can present a meaningful prioritized
catalog to the user without entering a retry loop.

## Requirements

- **BR-AF-STREAM-001-R1**: `list_alerts` MUST return at least 5 alerts with full
  detail (labels, annotations, state, active_at) within the 4096-byte tool output
  limit when 5 or more alerts are firing.

- **BR-AF-STREAM-001-R2**: The response MUST include a `total_count` field
  reflecting the total number of matching alerts before truncation, so the LLM
  knows how many alerts exist beyond the returned set.

- **BR-AF-STREAM-001-R3**: The `prioritized` field MUST NOT duplicate alert data.
  It MUST reference alerts by index into the `alerts[]` array.

- **BR-AF-STREAM-001-R4**: The `alerts[]` array MUST be sorted by priority
  (severity descending, then ActiveAt ascending/FIFO) so that indices in
  `prioritized` are positionally meaningful.

- **BR-AF-STREAM-001-R5**: When truncation occurs, the tool MUST set
  `truncated: true` and the response MUST still be valid JSON that the session
  trimmer (`trimEventFunctionResponses`) does not replace with a stub.

## Acceptance Criteria

1. A `list_alerts` call with 5 typical Prometheus alerts (10 labels, 3 annotations
   each) produces a JSON response under 4096 bytes.
2. The LLM does not enter a retry loop when `truncated: true` is returned.
3. The agent prompt instructs the LLM to use filters (namespace, severity, state)
   to narrow results rather than retrying.
4. Existing prioritization behavior (highest severity selected, FIFO tiebreak) is
   preserved.

## Status Event Text Sizing (#1435)

The EventBridge `sanitizeBridgeText` truncates all outbound text at 512 runes,
including LLM reasoning and output text that operators see in the Console thinking
panel. This causes mid-sentence truncation during live investigations.

- **BR-AF-STREAM-001-R6**: Reasoning text (`metadata.type: "reasoning"`) and output
  text (`metadata.type: "output"`) MUST support up to 4096 runes without truncation.
  Ephemeral status messages (`metadata.type: "status"`) MAY remain capped at 512
  runes since they are short by nature.

- **BR-AF-STREAM-001-R7**: When the EventBridge truncates text, it MUST log the
  truncation event with original size, max limit, and truncated rune count so
  operators can monitor payload sizing.

- **BR-AF-STREAM-001-R8**: The streaming executor MUST NOT re-invoke the agent
  when the context is cancelled. Re-invocation after context cancellation produces
  ghost sessions that fail immediately with `context canceled`.

### Acceptance Criteria

5. Reasoning text of 700 characters (typical investigation plan) passes through the
   EventBridge without truncation.
6. Truncation events are observable in AF pod logs at verbosity level 1.
7. The streaming executor does not attempt re-invocation after `context.Canceled`.

## Non-Goals

- Pagination with offset/cursor (existing filters suffice for the agent's workflow).
- Increasing the 4096-byte limit (etcd safety constraint remains).
