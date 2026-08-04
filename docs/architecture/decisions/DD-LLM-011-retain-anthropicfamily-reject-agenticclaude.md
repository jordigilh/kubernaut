# DD-LLM-011: Retain `anthropicfamily` on Direct `anthropic-sdk-go` — Reject `eino-ext`'s `agenticclaude`

**Status**: ✅ Approved
**Priority**: P3 (evaluation of an existing, working, non-forced component — no code change)
**Owner**: KubernautAgent Team
**Scope**: `pkg/kubernautagent/llm/anthropicfamily` (evaluated, unchanged), `cmd/kubernautagent/llm_builder.go` (evaluated, unchanged)
**Related**: [DD-LLM-007](./DD-LLM-007-af-ka-anthropic-client-divergence.md) (AF/KA Anthropic client divergence boundary), [DD-LLM-010](./DD-LLM-010-eino-agenticgemini-for-ka-gemini-client.md) (eino-ext adoption precedent for KA's Gemini client, which deferred this exact evaluation), [DD-KA-019-001](./DD-KA-019-go-rewrite-design/DD-KA-019-001-framework-selection.md) (Framework Isolation Pattern), [DD-LLM-006](./DD-LLM-006-bedrock-dual-client-routing.md) (Bedrock dual-client routing), [BR-AI-086](../../requirements/BR-AI-086-llm-reasoning-token-support.md), Issue #1796, #1778

---

## Context & Problem

DD-LLM-010 adopted `cloudwego/eino-ext`'s `components/model/agenticgemini` for KA's new native Gemini client (`pkg/kubernautagent/llm/geminifamily`, #1778), bringing `cloudwego/eino` and `cloudwego/eino-ext` into KA's `go.mod` for the first time. That DD explicitly deferred — not decided — a follow-up question it raised itself: `eino-ext` also ships a `components/model/agenticclaude` component (Claude/Anthropic, the same `eino/schema.AgenticMessage` contract as `agenticgemini`), and now that `eino` is already a dependency, should KA's existing `anthropicfamily.Client` (built directly on `anthropic-sdk-go`, ~300 LOC of hand-written protocol logic) be replaced by a thin wrapper over `agenticclaude`, the way `geminifamily` wraps `agenticgemini`?

Unlike DD-LLM-010's Gemini decision, this is **not forced by a capability gap** — `anthropicfamily` is working, tested code on KA's primary/most-used LLM path (native `provider: anthropic` and `provider: vertex_ai` when the configured model is Claude, both dispatched from `cmd/kubernautagent/llm_builder.go`'s `buildLLMClientFromConfig`). The only premise that changed since DD-LLM-007 rejected Anthropic-client unification is dependency cost: `eino` is now "reuse an existing dependency" rather than "add a new one." This DD re-runs the alternatives analysis under that changed premise, as DD-LLM-010 committed to do.

### Alternatives Considered

#### Alternative A — Retain `anthropicfamily.Client` unchanged, built directly on `anthropic-sdk-go` (chosen)

- **Pros**: Zero regression risk to a working, tested capability (see Discovery Spike Findings below — `agenticclaude` currently regresses on three fronts). No new adapter-layer risk introduced to KA's primary LLM path. `anthropicfamily`'s explicit design intent (`client.go:17-44`) and its "zero ADK coupling" characterization — the evidence DD-LLM-007's Alternative C relies on to justify AF/KA divergence — remain completely unaffected, since nothing about `anthropicfamily` changes.
- **Cons**: `anthropicfamily` remains a second hand-maintained Anthropic protocol adapter (alongside AF's `adk-anthropic-go`), each independently exposed to upstream `anthropic-sdk-go` API changes. No native AWS Bedrock-for-Claude path (see below).

#### Alternative B — Replace `anthropicfamily.Client` with a thin wrapper over `eino-ext`'s `agenticclaude` (rejected)

- **Pros**: Reuses an already-present dependency (`eino` core, pinned at the exact version `agenticclaude` needs — zero eino-core version delta) rather than adding one. Would establish a uniform "thin adapter over eino-ext" pattern across both of KA's Google/Anthropic clients (mirroring `geminifamily`). `agenticclaude` supports Function Tools, Deferred Tools, and Server Tools (web search/fetch/code exec) — broader tool-call surface than `anthropicfamily` currently implements. Gains native AWS Bedrock support (`Config.ByBedrock`) that `anthropicfamily` currently lacks and has deferred to #1582 (DD-LLM-006).
- **Cons** (confirmed by direct inspection of `agenticclaude`'s source on `cloudwego/eino-ext@main`, not assumption):
  1. **`agenticclaude` silently drops `RedactedThinkingBlock` on incoming responses** (`convertor.go:601-603`: explicitly discards it rather than mapping it to a field). `anthropicfamily` captures and replays redacted thinking blocks today (`client.go:511-516`, `432-444`), proven by `UT-KA-1580-108`/`UT-KA-1580-111`. Adopting `agenticclaude` as-is would be a **regression** in a tested, working capability, not parity.
  2. **No per-call thinking override.** `agenticclaude`'s `Thinking` config is fixed at `Model` construction (`model.go:469-471`, `184`) with no `model.Option` equivalent in `claudeOptions` (`option.go`). `anthropicfamily` supports both a construction-time default (`WithReasoning`) *and* a per-call override that wins (`client.go:106-118`, `309-319`), proven by `UT-KA-1578-203`. This is a second regression relative to today's tested behavior.
  3. **No `output_config.effort` support for Claude.** A repo-wide `grep -rn -i "outputconfig|output_config|effort"` across all of `agenticclaude`'s source returns exactly two hits, both in `model.go`'s `ExtraFields` doc comment — a generic, provider-agnostic example (`"reasoning_effort": "high"` against a hypothetical model `"o1"`, i.e. an OpenAI reasoning-model example, not a Claude-specific feature). There is no dedicated `Effort`/`OutputConfig` field on `Config`/`claudeOptions` the way `anthropicfamily.Client.buildParams` sets `params.OutputConfig.Effort` directly (`client.go:316-317`, BR-AI-086's #1604 unified `Effort` knob — see DD-LLM-007's Update note). `agenticclaude`'s `ExtraFields`/`WithExtraFields` top-level JSON-merge escape hatch is an untested, unverified bridging path for reproducing this — it would need its own discovery spike before being relied on, not assumed to work identically to a first-class field.
  4. **No built-in error classification.** `agenticclaude.Generate`/`.Stream` only wrap errors with `fmt.Errorf("...: %w", err)` (`model.go:342`, `387`) — no retryable/non-retryable typing. Since `%w` preserves the unwrap chain, a wrapping adapter could still reimplement `anthropicfamily.classifyErr`'s `errors.As(err, &anthropic.Error{})` logic today (`client.go:274-286`) — meaning adopting `agenticclaude` would **not** reduce KA's hand-written surface for this concern at all; it would need to be rewritten, not inherited.
  5. Net LOC/complexity reduction is smaller than DD-LLM-010's Gemini case: `geminifamily`'s own precedent (`conv.go`) shows `eino-ext` adoption does not eliminate the KA-type ↔ eino-type translation layer, only the wire-protocol/HTTP-transport layer `anthropicfamily` already gets for free from `anthropic-sdk-go` directly. The marginal benefit here is materially smaller than for Gemini, where KA had *no* existing client at all.

### Discovery Spike Findings (`agenticclaude`, verified against `cloudwego/eino-ext@main`)

Verified by direct source inspection (`convertor.go`, `model.go`, `option.go`, `event_convertor.go`) and `go list -m -versions`, not documentation claims alone:

- **Regression #1 — dropped `RedactedThinkingBlock`**: `convertor.go:601-603` explicitly discards redacted thinking content (`// Drop redacted thinking block; return nil, nil`) instead of mapping it to `schema.Reasoning`.
- **Regression #2 — no per-call thinking override**: `Thinking` is `Model`-construction-scoped only (`model.go:469-471`); `claudeOptions` (`option.go`) has no per-call equivalent.
- **Regression #3 — no `effort`/`output_config` support**: zero matches for `outputconfig|output_config|effort` across the entire component source.
- **Not a regression — error classification**: `agenticclaude` has none (`model.go:342`, `387`, plain `fmt.Errorf` wraps); a replacement adapter would have to write this logic itself either way, same cost as today.
- **Advantage — native Bedrock support**: `Config.ByBedrock` (`model.go:147-173`, `261-279`), which `anthropicfamily` lacks (tracked separately under #1582/DD-LLM-006).
- **Advantage — no unexported-contract coupling**: unlike `geminifamily`'s `ThoughtSignature` workaround (DD-LLM-010's Discovery Spike Findings), `agenticclaude` uses the **public** `schema.Reasoning.Signature`/`.Text` fields directly (`convertor.go:284`, `589-591`) — so *if* the three regressions above were fixed upstream, adopting it would not require reproducing an unversioned private-key hack the way `geminifamily` did.
- Tool-call round-tripping and streaming (including thinking-delta and signature-delta streaming, `event_convertor.go:119-123`) are at parity or broader than `anthropicfamily`.

### Decision

**Alternative A** — retain `anthropicfamily.Client` unchanged, built directly on `anthropic-sdk-go`. Do **not** adopt `eino-ext`'s `agenticclaude` at this time. `anthropicfamily` has no forcing bug or capability gap; `agenticclaude` currently has three confirmed regressions against `anthropicfamily`'s existing, tested behavior (dropped redacted-thinking blocks, no per-call thinking override, no effort-dial support) that would have to be accepted or separately worked around to adopt it. The dependency-footprint argument that motivated re-opening this question (eino is now "reuse" not "add") does not, by itself, outweigh those regressions.

**DD-LLM-007 is not revised.** Its Alternative C evidence — that `anthropicfamily` has "zero ADK coupling" and is cheap to maintain independently of AF's `adk-anthropic-go` — remains entirely accurate, since this decision leaves `anthropicfamily`'s internals untouched. The AF/KA Anthropic-client divergence boundary DD-LLM-007 establishes is unaffected by this evaluation either way.

**Revisit conditions** (not a blocking commitment, just the trigger that would make re-evaluation worthwhile): if `eino-ext` closes the redacted-thinking-block gap, adds a per-call thinking override, and adds `effort`/`output_config` support upstream, the cost/benefit shifts materially and this DD should be re-opened. Separately, if AWS Bedrock support for Claude becomes an actual forcing requirement, evaluate extending DD-LLM-006's existing Bedrock dual-client-routing pattern first, as a more narrowly-scoped fix than a full client replacement.

## Consequences

### Positive
- Avoids adopting a regression into a working, tested, BR-AI-086-relevant capability (redacted-thinking replay, per-call thinking override, effort dial) for no forcing reason.
- No new dependency surface added to KA's primary/most-used LLM path (`eino`/`eino-ext` remain scoped to `geminifamily` only, per DD-LLM-010's original intent).
- DD-LLM-007's AF/KA divergence boundary and its supporting evidence remain valid with zero required updates.
- Closes issue #1796 with a documented, evidence-based decision instead of leaving the question open indefinitely.

### Negative
- `anthropicfamily` remains a second hand-maintained Anthropic protocol adapter (alongside AF's `adk-anthropic-go`), each ~300 LOC, independently exposed to `anthropic-sdk-go` API/behavior changes.
- No native Bedrock-for-Claude path from this decision; if ever prioritized, tracked separately under #1582/DD-LLM-006.
- This decision is time-bound to `agenticclaude`'s current state (`cloudwego/eino-ext@main`, evaluated 2026-08-03); it should be revisited, not treated as permanent, if the upstream gaps close.

## Related Decisions
- **Builds on**: DD-LLM-010 (which deferred this exact evaluation to #1796), DD-KA-019-001 (Framework Isolation Pattern)
- **Reaffirms**: DD-LLM-007 (AF/KA Anthropic client divergence boundary — unaffected, since `anthropicfamily`'s internals are unchanged by this decision)
- **Contrasts with**: DD-LLM-010, where eino-ext adoption *was* the right call for KA's Gemini client — that decision closed an actual capability gap (KA had zero native Gemini client); here, `anthropicfamily` has no capability gap, and the eino-ext alternative currently regresses on three fronts
- **Related to**: DD-LLM-006 (Bedrock dual-client routing — the narrower path to Bedrock-for-Claude support if that ever becomes a forcing requirement, instead of a full client replacement)
- **Referenced by**: #1796 (closes this issue)

---

**Document Control**:
- **Created**: 2026-08-03
- **Version**: 1.0
- **Status**: Approved
