# DD-LLM-009: Dedicated Event Type for Live-Streaming Captured Reasoning Content

**Status**: ✅ Approved
**Priority**: P2
**Owner**: KubernautAgent Team
**Scope**: `internal/kubernautagent/session/types.go`, `internal/kubernautagent/investigator/*`, `pkg/apifrontend/ka/config.go`, `pkg/apifrontend/tools/ka_investigate_bridge.go`, `pkg/apifrontend/launcher/event_bridge.go`
**Related**: [BR-AI-086](../../requirements/BR-AI-086-llm-reasoning-token-support.md) (AC10), [DD-LLM-005](./DD-LLM-005-model-aware-reasoning-support.md) (reasoning/thinking data model), [DD-LLM-007](./DD-LLM-007-af-ka-anthropic-client-divergence.md), Issues #1634, #1635

---

## Context & Problem

BR-AI-086 added opt-in reasoning/thinking-token capture to KA's LLM clients (`llm.Message.Reasoning *ReasoningBlock{Text, Signature, Redacted}`). Until now this content had exactly one consumer: the SOC2 audit trail (`internal/kubernautagent/audit/ds_response_mapping.go`). It never reached KA's live MCP event stream, so an operator watching an investigation live (via AF's A2A SSE relay) never saw it — only a post-hoc `correlation_id` audit query could reconstruct it.

Separately, while scoping this gap, a pre-existing, unrelated bug was found (#1634): AF's relay of KA's existing `reasoning_delta` event (KA's per-turn orchestration/progress narration, sourced from `Message.Content`, not `Message.Reasoning`) silently drops all text due to a JSON field-name mismatch (`"text"` expected by AF, `"content"`/`"content_preview"` sent by KA). Fixing this bug and adding live-streaming for genuine captured reasoning content raised the question of whether the new content should reuse the existing `reasoning_delta` event/`"reasoning"` SSE metadata type, or use something new.

## Alternatives Considered

### Alternative A — Reuse `reasoning_delta` / `MetaTypeReasoning` for both narration and captured reasoning content (rejected)

**Approach**: Once #1634's bug is fixed, simply also write `Message.Reasoning.Text` into the same `reasoning_delta` event payload (or append it to the existing `content`/`content_preview` field) at the same three call sites, with no new event type.

**Pros**:
- Zero new wire constants; smallest possible diff.
- AF's relay needs no new case — the existing `reasoning_delta` → `MetaTypeReasoning` path already works (once #1634 is fixed).

**Cons**:
- Conflates two semantically different things under one name: KA's own orchestration/progress narration (always present, sourced from `Message.Content`) and genuine provider-native extended-thinking content (opt-in, sourced from `Message.Reasoning`, subject to redaction).
- No consumer — AF's relay today, or a future Console renderer — could ever treat them differently (different verbosity expectations, different redaction handling, different enable/disable semantics) without inspecting event payload shape heuristically.
- Directly contradicts the explicit decision already made for this work: reasoning content must use a event type distinct from orchestration narration precisely so behavior can diverge later.

### Alternative B — New event type at the KA-to-AF MCP layer only, converge back onto the existing `MetaTypeReasoning` SSE metadata type (rejected)

**Approach**: Introduce `session.EventTypeReasoningContentDelta` / `ka.EventTypeReasoningContentDelta` for the KA-to-AF MCP hop (satisfying "needs its own event type" narrowly), but relay it into AF's SSE stream using the same `metadata.type: "reasoning"` as everything else, via the existing `EmitReasoningSafe` path.

**Pros**:
- Slightly smaller AF-side diff (no new `EventBridge` method, no new `MetaType*` constant).
- Still gives KA-side/internal code a way to distinguish the two sources if needed (e.g. for metrics).

**Cons**:
- The fork does not survive to the wire contract that actually matters: any SSE/A2A consumer (today's Console, or any future one) still cannot tell captured reasoning content apart from AF's own ADK `Thought`-part narration or KA's orchestration-preview text, since all three converge on `metadata.type: "reasoning"`.
- Defeats the explicit stated purpose of this decision — "a unique event type so we can differ behavior if we want to" — since the place behavior would actually need to differ (client-side rendering) never sees the distinction.
- A half-measure: solves the problem's letter (a new Go constant exists) but not its substance (consumers still can't differentiate).

### Alternative C — Dedicated event type end-to-end: new KA event, new AF mirror, new SSE metadata type (chosen)

**Approach**:
- New KA-internal event type: `session.EventTypeReasoningContentDelta = "reasoning_content_delta"` (`internal/kubernautagent/session/types.go`), emitted at the three existing `EventTypeReasoningDelta` call sites, guarded by `resp.Message.Reasoning != nil`, carrying `{"text": ..., "redacted": ...}`.
- Mirrored AF-side wire-compatible constant: `ka.EventTypeReasoningContentDelta` (`pkg/apifrontend/ka/config.go`), following the existing mirroring pattern already used for all other event types.
- New AF SSE metadata type: `launcher.MetaTypeReasoningContent = "reasoning_content"` (`pkg/apifrontend/launcher/event_bridge.go`), emitted via new `EventBridge.EmitReasoningContent`/`EmitReasoningContentSafe` methods, following the exact existing `EmitReasoning`/`EmitReasoningSafe` shape (`emitWithLimit`, `maxReasoningTextLen` = 4096 runes).
- `FormatEventForUser` and `emitEventToA2A` (`pkg/apifrontend/tools/ka_investigate_bridge.go`) gain a new case routing `EventTypeReasoningContentDelta` to `EmitReasoningContentSafe` instead of the default `EmitReasoningSafe`.

**Pros**:
- The fork survives all the way to the wire contract a Console (or any other SSE consumer) actually reads — `metadata.type: "reasoning_content"` is unambiguously distinguishable from `metadata.type: "reasoning"` (AF's own `Thought`-part narration + KA's orchestration-preview text).
- Purely additive: zero behavior change to the existing `reasoning_delta`/`MetaTypeReasoning` narration path (#1634's bug fix is independent and orthogonal).
- Gives a not-yet-built Console feature (tracked separately, out of scope for this DD/repo) an unambiguous, already-scoped wire contract to build against later. An SSE consumer that doesn't yet recognize `metadata.type: "reasoning_content"` simply doesn't render it — no visible behavior change until a consumer opts in.
- Matches the exact existing extension pattern (`EmitReasoning`/`EmitOutput` → `EmitReasoningContent` is the third instance of the same `emitWithLimit` shape) — low implementation risk, no new abstraction.

**Cons**:
- Two mirrored const blocks (`session.EventType*`, `ka.EventType*`) grow by one entry each — already an established, repeated pattern (9 existing pairs), not a new maintenance burden class.
- One new `EventBridge` method pair, one new `MetaType*` constant — small, but real, new surface area.

### Decision

**Alternative C.** Approved 2026-07-08. The whole point of requiring a unique event type is to let a downstream consumer differentiate behavior; an internal-only fork (Alternative B) that reconverges before reaching any actual consumer does not achieve that, so it was rejected as a half-measure.

## Sub-decision: redaction handling

BR-AI-086's `ReasoningBlock.Redacted` marks reasoning whose visible text was withheld by the provider (e.g. Anthropic `redacted_thinking`) but which must still be replayed on subsequent turns. Two options for the live-stream path:

- **Chosen**: KA always includes both `text` and `redacted` fields in the new event's payload, gated only by `resp.Message.Reasoning != nil`. AF's relay forwards `text` through the identical `EmitReasoningContent`/`emitWithLimit` no-op-on-empty-text pattern already used by `EmitReasoning`/`EmitOutput`. Since `ReasoningBlock.Text` is always empty when `Redacted` is true (per the type's own contract, confirmed by existing audit tests), a redacted turn simply produces no live event — identical to today's established behavior for any other empty reasoning/output text. The audit trail remains the complete, durable record of `redacted=true` regardless (AC6, unaffected). This avoids the backend inventing placeholder UX copy for a case where the actual rendering/UX treatment belongs to whichever client eventually consumes this event type.
- **Deferred, not chosen now**: threading `redacted` through as SSE metadata on `EmitReasoningContent` (a `redacted bool` parameter, always emitting even on empty text so a client can render a distinct "reasoning hidden" placeholder). This is additive and can be introduced later without breaking the wire contract, once/if a consumer is ready to build redaction-aware rendering. Not implemented now because nothing consumes it yet — avoids speculative complexity ahead of a confirmed need.

### Revisited (#1716)

The deferral's own stated trigger — "once/if a consumer is ready to build redaction-aware
rendering" — occurred: `kubernaut-console#32` confirmed it will render a "reasoning hidden by
provider" placeholder if AF emits a live signal. Implemented exactly the previously-deferred
shape, unchanged: `EmitReasoningContent`/`EmitReasoningContentSafe` gained a `redacted bool`
parameter; a redacted turn now emits `TaskStatusUpdateEvent{metadata: {type: "reasoning_content",
redacted: true}}` with empty message text, instead of the full no-op described above. No new
event type, no new component — additive, boolean-only signal, matching Alternative C's existing
shape. The audit trail (`ReasoningSummary.Redacted`) remains the durable record of truth (AC6);
this is a live-stream-only, best-effort UX signal layered on top of it.

## Consequences

### Positive
- Reasoning content and orchestration narration are unambiguously distinguishable at every layer of the pipeline (KA event type, AF mirror, AF SSE metadata type).
- Zero behavior change to any existing consumer of `reasoning_delta`/`MetaTypeReasoning`.
- `processBridgeEvent`'s summary-accumulation switch deliberately does not include the new event type — genuine model deliberation text is never concatenated into the final chat-answer/RCA summary returned to the operator, preventing an accidental verbosity/leakage regression.
- Sets up a clean, already-decided wire contract for future client-side work without requiring any client-side change today.

### Negative
- Slightly larger diff than Alternative A/B (new `EventBridge` methods, new constant, new relay case) — mitigated by following an existing, well-tested pattern exactly (`EmitReasoning`/`EmitOutput`).
- ~~Redacted reasoning produces no live signal at all (silently absent, not explicitly marked) until/unless the deferred sub-decision is revisited~~ — superseded by #1716 (see "Revisited" above): redacted reasoning now emits a content-free `metadata.redacted=true` signal instead of a silent no-op.

## Related Decisions
- **Builds on**: DD-LLM-005 (reasoning/thinking data model, `ReasoningBlock` shape), BR-AI-086 (AC10)
- **Orthogonal to**: #1634 (independent bug fix to the pre-existing `reasoning_delta` narration relay; both changes touch adjacent code in the same files but do not depend on each other)
- **Contrasts with**: DD-LLM-007 (documents that AF's own Anthropic/Vertex/Gemini ADK model surface has no reasoning capture at all — unaffected by this DD, which only concerns relaying KA's already-captured reasoning through AF, not adding capture to AF's own model)

---

**Document Control**:
- **Created**: 2026-07-08
- **Version**: 1.0
- **Status**: Approved
