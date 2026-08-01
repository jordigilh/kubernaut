# DD-LLM-010: Adopt eino-ext's `agenticgemini` for KA's Native Gemini Client

**Status**: ✅ Approved
**Priority**: P2
**Owner**: KubernautAgent Team
**Scope**: `pkg/kubernautagent/llm/geminifamily` (new), `cmd/kubernautagent/llm_builder.go`
**Related**: [DD-KA-019-001](./DD-KA-019-go-rewrite-design/DD-KA-019-001-framework-selection.md) (Framework Isolation Pattern), [DD-LLM-004](./DD-LLM-004-langchaingo-removal-generalized-clients.md) (langchaingo removal, shared OpenAI-compatible core), [DD-LLM-005](./DD-LLM-005-model-aware-reasoning-support.md) (reasoning/thinking support), [DD-LLM-007](./DD-LLM-007-af-ka-anthropic-client-divergence.md) (AF/KA Anthropic client divergence), [BR-AI-086](../../requirements/BR-AI-086-llm-reasoning-token-support.md), Issue #1778, #1792, #1793

---

## Context & Problem

Kubernaut Agent (KA) has no native Gemini/Google-GenAI client. `cmd/kubernautagent/llm_builder.go`'s `buildLLMClientFromConfig` hardcodes the `vertex_ai` provider to `anthropicfamily.New` regardless of the configured model (issue #1778), and has no case at all for the standalone `gemini` provider (`types.LLMProviderGemini`), which is a validated config enum value with no implementation — a KA process configured with `provider: gemini` fails at client construction with "unsupported LLM provider". API Frontend (AF) has the identical `vertex_ai` dispatch bug (issue #1792) but already has a correct, working standalone Gemini path (`google.golang.org/adk/model/gemini`).

This DD covers only KA's client (the actual capability gap). AF's fix (#1792) reuses AF's existing, already-tested Gemini path and needs no new library — see the plan for #1778/#1792/#1793.

### Alternatives Considered

#### Alternative A — In-house `pkg/shared/llm/geminicompat` core built directly on `google.golang.org/genai` (rejected for now)

- **Pros**: Zero new third-party dependency; full control; mirrors DD-LLM-004's precedent of building a shared, framework-free protocol core (`openaicompat`) directly on the vendor SDK.
- **Cons**: ~700 LOC of new request/response mapping, tool-call handling, and streaming-aggregation logic to write and maintain ourselves, duplicating logic that a maintained upstream package already provides and tests. Given the team's explicit goal of reducing — not growing — the surface area of hand-maintained LLM protocol code, this was judged higher long-term maintenance cost than a well-vetted dependency.

#### Alternative B — Google ADK's `model/gemini.NewModel` (rejected)

- **Pros**: Zero new dependency (`google.golang.org/adk` is already in `go.mod` for AF); official Google package.
- **Cons**: Investigated directly — `adk/model/gemini.NewModel` is a thin wrapper over `genai.NewClient`, but it imports ADK's internal `internal/llminternal` package for its streaming aggregator. `internal/llminternal` is not a small, isolated leaf package: it imports ADK's `agent`, `session`, and `tool` packages as part of the same internal module. Importing `adk/model/gemini` therefore transitively compiles ADK's agent/session/tool framework into KA's binary — precisely the framework-coupling risk DD-KA-019 (Framework Isolation Pattern) exists to prevent for KA. Confirmed by direct inspection of the ADK v1.5.1 module source (`internal/llminternal/*.go` imports), not assumption.

#### Alternative C — `cloudwego/eino-ext`'s `components/model/agenticgemini` (chosen)

- **Pros**:
  - **Independently-versioned Go module**: `eino-ext/components/model/agenticgemini` ships its own `go.mod`, depending only on `google.golang.org/genai`, `cloudwego/eino` core (`schema`, `components/model`, `callbacks`), and small JSON/mapstructure utilities — no graph engine, no multi-agent runtime, no ADK. Verified directly against the published `go.mod` and against `eino/components/model/interface.go`, which imports only `context` + `eino/schema`.
  - **Native reasoning/thinking support** using the vendor SDK's own type (`Config.ThinkingConfig *genai.ThinkingConfig`), not a reimplementation — the same capability gap that motivated `langchaingo`'s removal (DD-LLM-004).
  - **Actively maintained**: `cloudwego/eino`/`eino-ext` — 12.5k/782 GitHub stars, pushed to `main` the same day this decision was made, GPG-signed commits on a roughly monthly cadence from a named, ByteDance-employed maintainer, Apache-2.0 licensed. Materially more active than `langchaingo`'s state at the time of its removal.
  - **Version-compatible**: kubernaut's `go.mod` already carries `google.golang.org/genai v1.65.0` (newer than `agenticgemini`'s own `v1.36.0` requirement — Go's MVS keeps the higher version, no downgrade) and is on Go 1.26.5, above eino-ext's `go 1.24` floor.
  - **Convergence with existing reasoning infrastructure**: KA's `anthropicfamily` package already resolves the operator's `Effort` config into a `*genai.ThinkingConfig` as its canonical intermediate representation (per DD-LLM-005), purely to feed Anthropic's converter. That same `*genai.ThinkingConfig` is Gemini's *native* wire type — the new Gemini client reuses this mapping directly (promoted to a shared location), rather than deriving a second effort-tier table.
  - Directly serves KA's `llm.Client` interface's own stated purpose: its doc comment (`pkg/kubernautagent/llm/types.go`) already names Eino by example as a framework this interface exists to isolate business logic from — this usage (Eino behind a thin, KA-owned adapter, never leaking `eino/schema` types into `internal/kubernautagent/investigator`) is exactly that pattern.
- **Cons**:
  - A new third-party dependency (`cloudwego/eino` + `cloudwego/eino-ext/components/model/agenticgemini`) enters `go.mod`, increasing (modestly) the supply-chain surface and go.sum size.
  - `eino`'s own prior evaluation for Kubernaut ([#507](https://github.com/jordigilh/kubernaut/issues/507)) deferred adopting Eino's *full* graph/multi-agent framework for KA's agent loop, citing dependency footprint. That decision is not overridden here: this DD adopts a single leaf model-component package, not the framework #507 evaluated.
  - The `eino/schema.AgenticMessage`/`ContentBlock` mapping (tool calls, `ThoughtSignature`-based reasoning replay) requires careful adapter-layer translation to/from KA's `llm.Message`/`ReasoningBlock` — tracked as an implementation-detail risk, addressed by a RED-phase discovery spike, not an architectural one.

### Discovery Spike Findings (RED-phase, `agenticgemini` v0.2.2)

Confirmed by direct source inspection of `agenticgemini`'s `conv.go`/`content_block_extra.go`/`message_extra.go` and `eino/schema/agentic_message.go`, plus Google's Gemini 3 thought-signature documentation:

- **Thought-signature replay is mandatory, not optional, for Gemini 3.** Per Google's docs, the first `functionCall` part in each step of a turn must carry back the exact `thoughtSignature` byte string the model returned, or the API rejects the request with a 400. Gemini 2.5 treats this as optional/best-effort. KA's client must therefore treat this as a correctness requirement, not a nice-to-have.
- **`agenticgemini` stores the signature in an unexported, package-private `ContentBlock.Extra` key** (`_eino_ext_agentic_gemini_thought_signature`), not in `schema.Reasoning.Signature` (the public field that field's own doc comment says exists for exactly this purpose). `getThoughtSignature`/`setThoughtSignature` are unexported with no public equivalent anywhere in the module (verified: `grep -rn ThoughtSignature` across the package only turns up the private accessors and their own tests) and this is unchanged at the latest available version (`v0.2.2`, confirmed via `go list -m -versions`).
- **Reasoning ("thought") content blocks are additionally dropped on replay unless the message is recognized as "self-generated"** (`ResponseMeta.GeminiExtension != nil`) — `ContentBlockTypeReasoning` is absent from `agenticgemini`'s non-self-generated whitelist, while `ContentBlockTypeFunctionToolCall`/`FunctionToolResult` are present and always pass through regardless.
- **Resolution adopted for `geminifamily`** (documented in code, not architectural): reproduce the identical Extra-map string key as an internal constant in `geminifamily` (map keys are structural, not identifier-scoped, so this is legal Go, just coupled to an unversioned internal contract) to attach/read the signature bytes; synthesize a minimal non-nil `ResponseMeta.GeminiExtension` marker when reconstructing an assistant message that carries reasoning text, so it clears the self-generated gate. `llm.ReasoningBlock.Signature` (already designed as an opaque, provider-specific, verbatim-replay string per its doc comment) is reused unchanged as the carrier — base64-encoded raw signature bytes — so no KA-facing contract changes. Risk is mitigated by: unit tests around `geminifamily`'s own encode/decode round-trip, and a tracked follow-up to request a public signature-accessor API upstream in `cloudwego/eino-ext`.

### Decision

**Alternative C** — adopt `eino-ext/components/model/agenticgemini`, wrapped by a new `pkg/kubernautagent/llm/geminifamily` package implementing KA's own `llm.Client` interface. No `eino` type is ever exposed outside this package; `internal/kubernautagent/investigator/*` remains completely unaware of it, consistent with DD-KA-019.

**Explicitly out of scope for this decision**: KA's existing `anthropicfamily.Client` (built on `anthropic-sdk-go` directly) is left untouched. `eino-ext` also ships an `agenticclaude` component that could, in principle, later replace `anthropicfamily` — since `eino` would already be a dependency at that point, the marginal adoption cost would be lower than today. But that is a separate, unforced decision: `anthropicfamily` is working, tested code on KA's primary/most-used LLM path, with no driving bug, and reversing it would revise `DD-LLM-007`'s intentional AF/KA-divergence boundary. If pursued, it needs its own CHECKPOINT DD alternatives-and-approval pass. Tracked as a follow-up issue ([#1796](https://github.com/jordigilh/kubernaut/issues/1796)), not bundled here.

## Consequences

### Positive
- KA gains full-parity Gemini support (direct API and Vertex-AI-hosted) without hand-writing and maintaining a new ~700 LOC protocol layer.
- Reasoning/thinking-token support for Gemini arrives "for free" by reusing the existing `genai.ThinkingConfig`-based effort mapping (DD-LLM-005), satisfying BR-AI-086's model-aware reasoning contract for a new provider with no new mapping logic.
- KA's framework-isolation guarantee (DD-KA-019) is preserved: `eino` is confined to one new leaf package behind `llm.Client`, exactly as `anthropicfamily` and `openaicompat` already are for their respective SDKs.
- Establishes a low-risk precedent: if a future provider surface is needed and eino-ext ships a maintained component for it, the same "thin adapter over an isolated eino-ext leaf module" pattern applies without re-litigating the ADK/framework-coupling question.

### Negative
- One new third-party dependency tree (`cloudwego/eino` + `eino-ext/components/model/agenticgemini`) is added to `go.mod`/`go.sum`. Mitigated by the maintenance/isolation evidence above and by confining its usage to a single new package.
- `anthropicfamily` (KA) and `adk-anthropic-go` (AF) remain independent Anthropic implementations, per DD-LLM-007 — this DD does not change that, and does not attempt Gemini/Anthropic client unification across AF and KA (AF's Gemini path stays ADK-native; KA's Gemini path is eino-native; the two consumer interfaces, `model.LLM` vs `llm.Client`, are irreconcilably different by design).

## Related Decisions
- **Builds on**: DD-KA-019-001 (Framework Isolation Pattern), DD-LLM-005 (reasoning/thinking mapping reused here)
- **Contrasts with**: the rejected Google ADK alternative (Alternative B above) — the opposite dependency-footprint outcome from the same investigation technique used to accept eino-ext here
- **Complements**: DD-LLM-007 (which this DD does not revise — KA's Anthropic path is explicitly out of scope)
- **Referenced by**: #1778 (KA Gemini client), #1792 (AF vertex_ai dispatch fix, independent of this DD), #1793 (Helm chart docs), #1796 (deferred follow-up: evaluate `anthropicfamily` → `agenticclaude` replacement)

---

**Document Control**:
- **Created**: 2026-07-30
- **Version**: 1.0
- **Status**: Approved
