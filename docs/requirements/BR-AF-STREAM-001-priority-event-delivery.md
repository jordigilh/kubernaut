# BR-AF-STREAM-001: Priority-Based Event Delivery for MCP Investigation Channel

**Business Requirement ID**: BR-AF-STREAM-001
**Category**: AF (API Frontend) / Streaming
**Priority**: P1
**Target Version**: V1.6
**Status**: Approved
**Date**: 2026-06-15
**Issue**: [#1433](https://github.com/jordigilh/kubernaut/issues/1433)
**Related**: [#1426](https://github.com/jordigilh/kubernaut/issues/1426), [#1427](https://github.com/jordigilh/kubernaut/issues/1427)

---

## Business Need

### Problem Statement

During KA investigation and workflow orchestration, the MCP event channel (buffer=64) fills with high-frequency streaming events (`token_delta`, `reasoning_delta`). When the buffer is full, the non-blocking send drops ALL subsequent events indiscriminately — including structural lifecycle events (`complete`, `error`, `alignment_verdict`) that downstream consumers (EventBridge, Console) depend on for state transitions.

### Evidence

Backend logs from live dev session (RR `rr-15e67191a457-009f7ed6`):

```
{"level":"info","msg":"event channel full, dropping event","event_type":"token_delta"}
{"level":"info","msg":"event channel full, dropping event","event_type":"token_delta"}
{"level":"info","msg":"event channel full, dropping event","event_type":"reasoning_delta"}
{"level":"info","msg":"event channel full, dropping event","event_type":"tool_call_start"}
{"level":"info","msg":"event channel full, dropping event","event_type":"tool_result"}
```

~20+ consecutive drops during the workflow selection + watch phase.

### Business Impact

| Stakeholder | Gap | Impact |
|---|---|---|
| Console / UX | Lifecycle events dropped | Verification timer doesn't render, phase banner stuck |
| SRE / Operator | No error/complete signal | Cannot determine if remediation succeeded or failed |
| Audit / Compliance | Lost structural events | FedRAMP AU-3 audit trail gaps for critical lifecycle transitions |
| System Reliability | No alignment_verdict delivery | SC-7 circuit breaker activation invisible to AF |

### FedRAMP Control Mapping

| Control | Objective | How This BR Serves It |
|---------|-----------|----------------------|
| **AU-3** | Content of Audit Records | Structural events (complete, error) contain session outcomes — never dropped |
| **AU-12** | Audit Generation | Priority delivery guarantees audit-relevant events reach the EventBridge |
| **SI-4** | Information System Monitoring | Lifecycle events enable real-time monitoring of investigation state |
| **SC-7** | Boundary Protection | alignment_verdict events trigger circuit breaker — must not be lost |

---

## Requirements

### BR-AF-STREAM-001.1: Structural events MUST NOT be dropped

Events classified as **structural** MUST use a blocking send (with bounded timeout) to the event channel. Structural events are:
- `complete`
- `error`
- `cancelled`
- `alignment_verdict`

If the channel remains full after the timeout (5 seconds), the event MUST be logged at Error level with the event type, RR ID, and session ID.

### BR-AF-STREAM-001.2: Streaming events MAY be dropped under backpressure

Events classified as **streaming** MAY use non-blocking send and be dropped when the channel is full. Streaming events are:
- `token_delta`
- `reasoning_delta`
- `tool_call_start`
- `tool_result`
- `tool_call`

Dropped streaming events MUST be logged at V(2) verbosity (debug) to avoid log noise. A drop counter MUST be maintained and logged when the channel drains or the session ends.

### BR-AF-STREAM-001.3: Drop metrics

The MCP client MUST expose a counter of dropped events partitioned by event type classification (structural vs streaming). This enables alerting on structural event drops (which should be zero in normal operation).

### BR-AF-STREAM-001.4: Channel buffer size

The event channel buffer size MUST remain configurable but default to 128 (increased from 64) to reduce drop frequency under normal load while the priority mechanism prevents structural event loss.

### BR-AF-STREAM-001.5: Backward compatibility

The change MUST be transparent to consumers of `<-chan InvestigationEvent`. The channel type, event format, and close semantics remain unchanged. Only the send-side behavior changes.

---

## Acceptance Criteria

- [ ] Structural events (`complete`, `error`, `cancelled`, `alignment_verdict`) use blocking send with timeout
- [ ] Streaming events (`token_delta`, `reasoning_delta`, `tool_call_start`, `tool_result`, `tool_call`) use non-blocking send
- [ ] Drop counter logged at session end
- [ ] Structural event drop logged at Error level (should never happen in practice)
- [ ] Channel buffer increased to 128
- [ ] No change to consumer-side interface (`<-chan InvestigationEvent`)
- [ ] UT tests prove priority classification and blocking behavior
- [ ] IT tests prove end-to-end delivery of structural events under backpressure

---

## Test Coverage

| Test ID | Tier | FedRAMP | What It Proves |
|---------|------|---------|----------------|
| UT-AF-1433-001 | UT | AU-3 | Structural event delivered when channel is full (blocking send) |
| UT-AF-1433-002 | UT | SI-4 | Streaming event dropped when channel is full (non-blocking send) |
| UT-AF-1433-003 | UT | AU-12 | Drop counter incremented for streaming events |
| UT-AF-1433-004 | UT | SC-7 | alignment_verdict classified as structural |
| UT-AF-1433-005 | UT | AU-3 | Structural send times out after deadline (doesn't block forever) |
| IT-AF-1433-001 | IT | AU-3, SI-4 | Complete event reaches bridge under sustained token_delta flood |

---

## Implementation

- **Production code**: `pkg/apifrontend/ka/mcp_sdk_client.go` (StartInvestigation LoggingMessageHandler)
- **Event classification**: `pkg/apifrontend/ka/event_priority.go` (new)
- **Metrics**: `pkg/apifrontend/ka/mcp_sdk_client.go` (drop counter)
- **Tests**: `pkg/apifrontend/ka/event_priority_test.go` (new)

---

## Related Requirements

- **BR-KA-OBSERVABILITY-002** (Verification Step Events): Depends on reliable event delivery through the MCP channel
- **BR-MCP-003** (Event Bridge Goroutine): Consumer of the prioritized channel
- **DD-AF-008** (Priority Event Channel): Design decision documenting the approach
