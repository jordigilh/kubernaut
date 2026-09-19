# DD-2439: Gemini Tool-Call Delta Stream Propagation

**Status**: Accepted
**Date**: 2026-09-18
**Issue**: #2439
**Business requirements**: BR-AI-087, BR-SESSION-003, BR-AUDIT-005

## Context

Gemini function calls arrive as streamed fragments. The provider-neutral KA LLM
interface already has `PartialToolCall`, but the production stream path must carry
those fragments through the investigator session sink and the AF A2A bridge. A
fragment is observer-facing progress only; it is not an executable tool call and
must not create an execution audit event. The completed `ChatResponse.ToolCalls`
remains the sole execution authority.

## Decision

Use a dedicated `tool_call_delta` investigation event. Its JSON payload preserves:

- `index`
- `id` when supplied
- `name` when supplied
- `arguments_delta`

The event also carries the existing session `turn` and `phase` envelope fields.
The investigator forwards the event with the same non-blocking sink semantics as
text deltas. The streaming attempt is marked unsafe to retry once either a text
delta or a tool-call delta has been delivered. AF relays the structured event to
the A2A bridge without converting it into a lossy text-only tool event.

Native Gemini and Vertex use the same `geminifamily.Client` conversion code. The
production Vertex builder honors the configured location; the manual validation
configuration uses location `global`. Live provider calls are not part of
mandatory CI.

## Alternatives Considered

### A. Execute or audit from each partial fragment

Rejected. Arguments may be incomplete or syntactically invalid, and execution
would violate the distinction between observation and action.

### B. Buffer fragments until the final response and emit only the complete call

Rejected. Operators lose real-time visibility, and the final response alone cannot
reconstruct what was observed during a stalled or failed stream.

### C. Dedicated structured `tool_call_delta` event

Accepted. It preserves provider-neutral structured data, keeps execution authority
unchanged, and allows AF/A2A consumers to render or store the event without parsing
human-readable text.

## Consequences

Positive:

- Tool-call progress is visible without changing tool execution semantics.
- Native and Vertex Gemini behavior remains aligned.
- Retry behavior avoids duplicate or interleaved partial output.
- AF consumers retain the exact fragment fields needed for reconstruction.

Negative:

- Session and AF event registries gain one additional event type.
- Consumers that do not understand the event will ignore it, while existing text,
  tool execution, and audit events remain backward-compatible.

## Verification

See `docs/tests/2439/TEST_PLAN.md` and scenarios `UT-GM-2439-001`,
`UT-KA-2439-002`, `UT-KA-2439-003`, `IT-KA-2439-002`, and
`IT-AF-2439-001`.
