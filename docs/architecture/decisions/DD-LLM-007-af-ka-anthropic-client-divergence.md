# DD-LLM-007: AF and KA Intentionally Do Not Share an Anthropic Client

**Status**: ✅ Approved
**Priority**: P3 (documentation of existing architecture, no code change)
**Owner**: KubernautAgent Team
**Scope**: `pkg/kubernautagent/llm/anthropicfamily`, `pkg/apifrontend/launcher/model.go`
**Related**: [DD-HAPI-019-001](./DD-HAPI-019-go-rewrite-design/DD-HAPI-019-001-framework-selection.md) (Framework Isolation Pattern), [DD-LLM-004](./DD-LLM-004-langchaingo-removal-generalized-clients.md) (langchaingo removal, shared OpenAI-compatible core), [DD-LLM-005](./DD-LLM-005-model-aware-reasoning-support.md) (reasoning/thinking support), [BR-AI-086](../../requirements/BR-AI-086-llm-reasoning-token-support.md), Issue #1578, #1601

---

## Context & Problem

While fixing #1578's reasoning-request wiring gap (KA's `ai.llm.reasoning.enabled` was never threaded into any real Anthropic API call — see #1601), the question arose: KA and AF both talk to Claude (native Anthropic API and Vertex AI). AF's LLM path for Claude uses `github.com/Alcova-AI/adk-anthropic-go`'s `NewModel`; KA's uses the hand-rolled `pkg/kubernautagent/llm/anthropicfamily.Client` built directly on `github.com/anthropics/anthropic-sdk-go`. Given DD-LLM-004 already unified AF and KA on a single shared `pkg/shared/llm/openaicompat` core for the OpenAI-Chat-Completions protocol (and reasoning support came "for free" to AF's OpenAI-compatible surface as a result), should the Anthropic/Claude surface be unified the same way — either by making KA adopt AF's `adk-anthropic-go`-based client, or vice versa?

This DD documents why the answer is **no**, and records that this is a deliberate architectural boundary, not an oversight or inconsistency to be fixed.

### Why AF and KA differ here in the first place

- **KA has no agent framework by design.** DD-HAPI-019-001 ("Framework Isolation Pattern") chose to keep KA's business logic (`internal/kubernautagent/investigator/*`) behind a Kubernaut-owned `llm.Client` interface specifically so that framework/library churn is absorbed by a thin adapter (~120 LOC) rather than leaking into business logic. This is the same principle that made removing `langchaingo` (DD-LLM-004) a contained refactor instead of a rewrite.
- **AF's entire agent loop is built on Google's ADK-Go framework** (`google.golang.org/adk`) — session management, event streaming, and tool execution are all ADK constructs. `adk-anthropic-go` is not a general-purpose Anthropic client; it is a bridge that implements ADK's `model.LLM` interface (`genai.Content`/`genai.Part` types) so that AF's ADK-based loop can address Claude. Using it outside of an ADK-based loop provides no benefit over using `anthropic-sdk-go` directly — it only adds a translation hop.

Both `adk-anthropic-go` and KA's `anthropicfamily` are, underneath, thin layers over the same official `anthropic-sdk-go`. `adk-anthropic-go` does not wrap a materially different or more capable Anthropic implementation; it wraps the identical SDK for ADK-interface compatibility.

## Alternatives Considered

### Alternative A — KA adopts `adk-anthropic-go` (and, transitively, Google ADK) for its Anthropic/Vertex path (rejected)

- **Pros**: Single Anthropic client implementation shared by AF and KA; one place to fix Anthropic-specific bugs.
- **Cons**: Requires either (a) KA adopting Google ADK as its full agent framework — reversing DD-HAPI-019's explicit "no framework" decision for an unrelated reason (Anthropic client reuse), a disproportionate architectural cost — or (b) using `adk-anthropic-go`'s model wrapper as a bolted-on dependency outside of ADK, which adds an extra KA-types → `genai.Content`/`Part` → adk-anthropic-go → `anthropic-sdk-go` translation hop in place of KA's current direct KA-types → `anthropic-sdk-go` hop. Either way, dependency footprint and indirection increase with no corresponding capability gain, since both paths bottom out in the same SDK. Directly contradicts the "lean dependency footprint" and "framework isolation" decision drivers from DD-HAPI-019-001.

### Alternative B — AF adopts KA's `anthropicfamily.Client` for its Anthropic/Vertex path (rejected)

- **Pros**: Single Anthropic client implementation; KA's client already has reasoning/thinking support (DD-LLM-005) and structured tool-call handling proven in production.
- **Cons**: `anthropicfamily.Client` implements KA's own `llm.Client` interface (`Chat`/`StreamChat` over KA's `ChatRequest`/`ChatResponse` types), not ADK's `model.LLM` interface. AF's entire launcher, event bridge, and streaming executor are wired against ADK's `model.LLM` contract (`genai.Content`/`Part`, ADK session/event semantics — see `pkg/apifrontend/launcher/part_converter.go`'s `Thought`-part routing to the interactive "ThinkingPanel", which has no equivalent in KA's `llm.Client` interface). Adopting `anthropicfamily.Client` in AF would require either writing and maintaining a `model.LLM`-shaped adapter around it (functionally identical cost/shape to what `adk-anthropic-go` already provides) or restructuring AF's launcher away from ADK — again a disproportionate cost relative to the goal of "one Anthropic client."

### Alternative C — Extract only the framework-independent, protocol-pure pieces that are genuinely reusable, leave the two model-wrapper layers separate (chosen — already partially in place)

- **Pros**: Captures the real, available reuse without forcing an agent-framework decision. `anthropicfamily.Client` already does this today: it imports `adk-anthropic-go/converters.ThinkingConfigToAnthropic` for model-tier thinking-budget detection (adaptive vs. manual-only), specifically to avoid a second, independently-maintained tier-detection table (DD-LLM-005) — while implementing `buildParams`/`mapResponse`/message conversion itself, with zero ADK coupling. This mirrors exactly what DD-LLM-004 did for the OpenAI-compatible protocol core (`pkg/shared/llm/openaicompat`), which *was* genuinely wire-protocol-identical and worth sharing between AF and KA.
- **Cons**: Two Anthropic call sites still exist (AF's ADK-wrapped one, KA's direct one); a genuine Anthropic-SDK-level bug fix (e.g. in message conversion) must be applied in both places if both are affected. Mitigated by both being thin, well-tested layers over the same upstream `anthropic-sdk-go` — bugs are far more likely to be in that shared upstream dependency (fixed once, upstream) than independently in each ~150-300 LOC adapter.

### Decision

**Alternative C** — no change to the current architecture. AF continues to use `adk-anthropic-go` (because AF is an ADK-based agent); KA continues to use `anthropicfamily.Client` (because KA is deliberately framework-independent per DD-HAPI-019). Reuse is scoped to genuinely framework-independent protocol logic (as already done for `converters.ThinkingConfigToAnthropic`, and as DD-LLM-004 did wholesale for the OpenAI-compatible protocol core, where AF and KA's needs were wire-protocol-identical).

This directly answers the "shouldn't we have parity" question raised while scoping E2E coverage for #1601: parity in the sense of "one shared Anthropic client" is not achievable without collapsing one of the two intentionally-different architectural choices (framework-based vs. framework-isolated) that predate and are orthogonal to the reasoning-token work. Parity in the sense of "both surfaces should support reasoning" is a separate, legitimate question — AF's Anthropic/Gemini surface via ADK does not currently read `cfg.Reasoning` at all (unlike AF's OpenAI-compatible surface, which gained it for free via `openaicompat`); if AF-side Claude/Gemini extended-thinking support is wanted, it should be scoped as its own BR against ADK's own thinking-config surface, not as an adoption of KA's client.

## Consequences

### Positive
- No disproportionate architectural cost (ADK adoption by KA, or ADK removal from AF) is incurred to chase client-implementation parity that would not, in practice, reduce risk (both paths already sit on the same upstream SDK).
- Preserves DD-HAPI-019's framework-isolation guarantee for KA and AF's ADK-native integration (ThinkingPanel routing, session/event semantics) without compromise.
- Leaves the door open for future, narrowly-scoped reuse of specific `adk-anthropic-go`/`anthropic-sdk-go` protocol-logic pieces (as already done for thinking-tier detection), consistent with CHECKPOINT B.

### Negative
- Two independent Anthropic message-conversion/tool-call-mapping implementations remain (AF's via ADK, KA's in `anthropicfamily`). A future correctness fix discovered in one may need to be checked against the other. Mitigated by both being thin (AF's is entirely `adk-anthropic-go`'s responsibility; KA's is ~300 LOC with its own test suite) and both ultimately bounded by `anthropic-sdk-go`'s own behavior.
- AF's Anthropic/Vertex (and Gemini) surface has no reasoning/thinking-token support today, and this DD does not add any. If needed, track as a separate BR scoped to AF + ADK's thinking-config surface.

**Update (#1604)**: the unified `Effort` reasoning-depth knob extended the "gained it for free via `openaicompat`" parity above from reasoning-content capture to the effort/depth-control dial as well — AF's OpenAI-compatible surface (`pkg/apifrontend/launcher/openai`) now carries `WithReasoningEffort`, wired from `cfg.Reasoning.Effort` in `pkg/apifrontend/launcher/model.go`, at parity with KA's equivalent wrapper. This is the same shared `pkg/shared/llm/openaicompat` dialect code on both sides — no new divergence introduced. The Anthropic/Vertex/Gemini gap noted above is unchanged by this update.

## Related Decisions
- **Builds on**: DD-HAPI-019-001 (Framework Isolation Pattern — the reason KA's and AF's LLM layers are structured differently in the first place)
- **Contrasts with**: DD-LLM-004 (where AF/KA sharing *was* the right call, because the OpenAI-Chat-Completions protocol is wire-identical between them — unlike the Anthropic surface, where AF's consumer is ADK's `model.LLM` interface and KA's is KA's own `llm.Client`)
- **Referenced by**: #1601 (KA reasoning-request wiring fix), while scoping E2E coverage for that fix; #1604 (unified Effort knob, extended to AF's OpenAI-compatible surface)

---

**Document Control**:
- **Created**: 2026-07-06
- **Version**: 1.0
- **Status**: Approved
