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

## Non-Goals

- Pagination with offset/cursor (existing filters suffice for the agent's workflow).
- Increasing the 4096-byte limit (etcd safety constraint remains).
