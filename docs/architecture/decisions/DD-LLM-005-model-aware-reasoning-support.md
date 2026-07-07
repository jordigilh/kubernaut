# DD-LLM-005: Model-Aware Reasoning/Thinking Token Support

**Status**: 📋 Proposed (2026-07-06)
**Priority**: P2
**Owner**: KubernautAgent Team
**Scope**: `pkg/kubernautagent/llm/types.go`, `pkg/kubernautagent/llm/anthropicfamily`, `pkg/kubernautagent/llm/openai`, `pkg/shared/llm/openaicompat`, `pkg/shared/types/llm.go`
**Related**: [BR-AI-086](../../requirements/BR-AI-086-llm-reasoning-token-support.md), [DD-LLM-004](./DD-LLM-004-langchaingo-removal-generalized-clients.md), [DD-HAPI-019](./DD-HAPI-019-go-rewrite-design), Issue #1578, #1580, #1581, #1601, #1604

---

## Context & Problem

KA never requests or preserves reasoning/thinking tokens from any LLM provider (see BR-AI-086 for the full business rationale). Two design questions must be resolved before implementation:

1. **How is model-aware thinking-tier detection/configuration implemented for the Anthropic-family client, and should it reuse `adk-anthropic-go/converters` (AF's proven implementation of exactly this logic) or reimplement it?**
2. **How does the system guarantee it never regresses behavior for models/providers that don't support reasoning — especially self-hosted/local models, which cannot be identified by vendor enum?**

### Spike: does reusing `adk-anthropic-go/converters` couple KA to the ADK agent framework?

`google.golang.org/adk` is already a dependency of this module (used extensively by `pkg/apifrontend/*`), but KA (`cmd/kubernautagent`) is a **separately built and deployed binary** from AF (`cmd/apifrontend`) — confirmed via `go list -deps ./cmd/kubernautagent/...`, which today returns zero ADK-related packages. So the real question is not "is ADK a new dependency for this module" (it isn't) but "what would reusing `converters.ThinkingConfigToAnthropic` cost KA's own binary specifically."

An initial read of `converters/response.go` (which imports `google.golang.org/adk/model`, in the same package as the `request.go` file containing `ThinkingConfigToAnthropic`) suggested importing the package would transitively adopt the full ADK agent-orchestration framework into KA — since Go compiles whole packages, not individual files. This was measured directly rather than left as an assumption:

```
$ go build -o /tmp/ka-before ./cmd/kubernautagent/          # 138,547,170 bytes
$ echo 'import _ "github.com/Alcova-AI/adk-anthropic-go/converters"' >> <anthropicfamily file>
$ go build -o /tmp/ka-after  ./cmd/kubernautagent/           # 138,638,386 bytes
# delta: 91,216 bytes (~89 KB, 0.07%)

$ comm -23 <(go list -deps github.com/Alcova-AI/adk-anthropic-go/converters | sort) \
           <(go list -deps ./cmd/kubernautagent/... | sort)
github.com/Alcova-AI/adk-anthropic-go/converters
github.com/google/go-cmp/cmp{,/internal/...}
github.com/gorilla/websocket
golang.org/x/net/internal/socks
golang.org/x/net/proxy
google.golang.org/adk/model                        # only this one ADK package — not agent/runner/session/memory/server/adka2a/plugin
google.golang.org/genai
```

**Result**: the actual incremental cost is a ~89 KB binary size increase and one thin ADK *types* package (`adk/model`, defining `model.LLM`/`model.LLMRequest`/`model.LLMResponse` — not the agent, runner, session, memory, plugin, or A2A-server code AF depends on ADK for). This is a materially different, much smaller finding than "adopts the ADK framework," and does not implicate DD-HAPI-019's business-logic isolation rule either way — that rule concerns `investigator.go`/`tools/`/`result/`, not the adapter/client layer, which is exactly where framework-specific code is expected to live (see DD-HAPI-019-001's own langchaingo-adapter example).

## Alternatives Considered

### Alternative A — Reimplement the tier-detection logic directly against `anthropic-sdk-go` types (rejected)

- **Pros**: Zero new packages in KA's binary at all (not even the ~89 KB `adk/model` addition). The logic to reimplement is small and self-contained: a `supportsAdaptiveThinking(model anthropic.Model) bool` switch over 3-4 known model constants, plus a budget/effort mapping table.
- **Cons**: Creates a second, independently-maintained copy of the same tier-detection table (AF's and KA's), with a real risk of drift as Anthropic ships new adaptive-capable models and only one of the two copies gets updated — this was the deciding factor against this alternative.

### Alternative B — Reuse `adk-anthropic-go/converters.ThinkingConfigToAnthropic` directly (chosen)

- **Pros**: Single source of truth for tier-detection, shared with AF — no drift risk. Measured incremental cost is negligible (~89 KB, one thin types package, no agent-orchestration code). `ThinkingConfigToAnthropic(cfg *genai.ThinkingConfig, model anthropic.Model) ThinkingMapping` returns plain `anthropic-sdk-go` types (`ThinkingConfigParamUnion`, `OutputConfigEffort`) — the only glue code needed at the call site is constructing a `*genai.ThinkingConfig` from KA's own `ReasoningRequest{Enabled, BudgetTokens}`, a few lines, not a full `genai.Content`/`Part` translation layer (KA does not need `PartToContentBlock` — it already builds `anthropic.ContentBlockParamUnion` thinking blocks directly from its own `ReasoningBlock` struct on the replay side).
- **Cons**: `anthropicfamily` gains a second external dependency edge (`adk-anthropic-go` + `google.golang.org/genai`, both already vendored in this module for AF) purely for one function call — a mild internal coupling, but confined entirely to the adapter package, never surfacing in `llm.ChatRequest`/`ChatResponse`/`Message`.

### Decision

**Alternative B**, reversing the initial (evidence-free) Alternative-A-leaning conclusion once the actual measured cost was known. Chosen by explicit user decision (2026-07-06) after being presented with concrete binary-size and package-diff evidence — prioritizing avoiding tier-detection drift with AF over an unmeasured "fewer dependencies" instinct that, when actually measured, turned out to cost ~89 KB and zero orchestration code.

## Design: compatibility floor for non-frontier/local models

A second, independent design question raised during planning: this effort must not raise the bar for smaller/self-hosted models while adding frontier-model capabilities. Concretely:

- **Fail-closed by construction.** Neither new client (`anthropicfamily`, `openai`) may speculatively include a capability-gated field (the `reasoning`/`thinking` request field) in an outbound request unless that capability was explicitly confirmed — vendor-enum lookup for Anthropic-family, auto-detection or explicit override for the OpenAI-compatible client. This generalizes a concrete finding from preflight: Mistral's API returns HTTP 422 on unrecognized OpenAI fields (e.g. `frequency_penalty`), which is representative of how many self-hosted OpenAI-compatible servers behave, not an isolated case.
- **`pkg/shared/llm/openaicompat` gets its own minimal-capability test fixture**, distinct from AF's existing `adapter_test.go` (written against real OpenAI's fully-compliant behavior). This fixture simulates a bare-bones OpenAI-compatible server — no reasoning field echoed, no strict-mode support, basic tool calls only — and is the actual local-model compatibility regression gate.
- **Bedrock's non-Claude models** (DD-LLM-006), routed through the same shared client, get identical fail-safe treatment.
- **Unsupported/unknown models skip the reasoning parameter, never error** — asserted by test on both clients, not left as an implicit side effect of auto-detection defaulting to "none".

## Data Model

```go
// pkg/kubernautagent/llm/types.go
type Message struct {
    // ...existing fields...
    Reasoning *ReasoningBlock `json:"reasoning,omitempty"`
}
type ReasoningBlock struct {
    Text      string `json:"text,omitempty"`
    Signature string `json:"signature,omitempty"`
    Redacted  bool   `json:"redacted,omitempty"`
}
type ChatOptions struct {
    // ...existing fields...
    Reasoning *ReasoningRequest `json:"reasoning,omitempty"`
}
type ReasoningRequest struct {
    Enabled      bool   `json:"enabled,omitempty"`
    BudgetTokens int    `json:"budget_tokens,omitempty"`
    Effort       string `json:"effort,omitempty"` // "", none/minimal/low/medium/high/xhigh (#1604)
}
```

Reasoning is resolved once at client-construction time (auto-detect from model name + `LLMReasoningConfig.CapabilityOverride`), never threaded per-call from the investigator — `internal/kubernautagent/investigator/*` remains completely unaware of reasoning.

### Addendum (#1604): unified `Effort` knob

`Effort` is a single, provider-agnostic reasoning-depth value using OpenAI's own vocabulary (the vendor with the widest tier granularity), reused as the canonical form for every provider rather than exposing a separate per-provider knob — the same "one config surface, provider-specific mapping under the hood" pattern this DD already established for `ReasoningMode`/`DetectReasoningMode`. Each client maps/clamps it into its own wire dialect:

- **Anthropic (native/Vertex)**: mapped onto `genai.ThinkingLevel` and passed through the same `converters.ThinkingConfigToAnthropic` call this DD already reuses — the effort hint (`ThinkingMapping.Effort`, Anthropic's `output_config.effort`) that call already computed for adaptive-capable models but which was previously discarded is now applied. `xhigh` clamps to `High`: `genai.ThinkingLevel` has no tier above High, even though Anthropic's raw `OutputConfigEffort` enum does (up to `max`) — staying within the shared converter's intermediate representation was chosen over hand-mapping a second, independently-maintained table straight to the SDK's fuller enum, consistent with this DD's Alternative-B rationale.
- **Real OpenAI/Azure o-series and gpt-5-family models**: passed through verbatim as Chat Completions' `reasoning_effort` field (`pkg/shared/llm/openaicompat`'s new `EffortDialectOpenAI`).
- **DeepSeek (openai_compatible)**: downscaled to DeepSeek's own two-tier dialect (`high`/`max`) plus an explicit `thinking.type` enabled/disabled toggle (`EffortDialectDeepSeek`).
- **`effort: none` + `enabled: true` for an Anthropic-family provider is a config-validation error (fail-closed at startup)**, not a runtime clamp — a deliberate exception to this DD's own compatibility-floor/graceful-degrade principle, because this is a known, deterministic operator-facing contradiction on a provider always identified by vendor enum, not an unknown-capability case the floor principle is meant to cover.
- **AF's OpenAI-compatible adapter (`pkg/apifrontend/launcher/openai`) gets the same Effort knob**, via a construction-time-only `WithReasoningEffort` option wired from `cfg.Reasoning.Effort` in `pkg/apifrontend/launcher/model.go`'s `newOpenAICompatibleModel` — the openai/deepseek dialect detection and wire mapping are the exact same `pkg/shared/llm/openaicompat` code KA's wrapper calls, so there is no drift risk between the two call sites. Unlike KA's `Options.Reasoning`, AF has no per-call override: ADK's `model.LLMRequest` carries no reasoning field, so the construction-time default is the only knob for this family, consistent with AF's existing reasoning-capture wiring (`DetectReasoningMode`) having no per-call override either. AF's Anthropic/Vertex path (via `adk-anthropic-go`) still has no reasoning/effort support at all — that gap remains tracked separately in DD-LLM-007, unaffected by this addendum.

## Consequences

### Positive
- Single source of truth for Anthropic thinking-tier detection, shared between AF and KA — no drift risk between two hand-maintained copies.
- Measured, negligible incremental cost to KA's binary (~89 KB, one thin types package).
- Compatibility floor is an enforced, tested guarantee rather than an assumption riding on frontier-provider test coverage.

### Negative
- `anthropicfamily` takes on a dependency-graph edge to `adk-anthropic-go` + `google.golang.org/genai`, confined to the adapter package. If AF ever migrates off `adk-anthropic-go` for its own reasons, KA's Anthropic-family client would need re-evaluation at that time — an acceptable, monitored coupling given the current shared-maintenance benefit outweighs it.

---

**Document Control**:
- **Created**: 2026-07-06
- **Version**: 1.0
- **Status**: Proposed — pending implementation
