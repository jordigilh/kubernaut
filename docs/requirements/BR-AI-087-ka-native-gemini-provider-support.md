# BR-AI-087: Native Gemini/Google-GenAI Provider Support for Kubernaut Agent

**Business Requirement ID**: BR-AI-087
**Category**: AI (Kubernaut Agent LLM provider support)
**Priority**: P2
**Target Version**: V1.5
**Status**: Approved

**Related Design Decisions**:
- [DD-KA-019: Framework Isolation Pattern (Go rewrite design)](../architecture/decisions/DD-KA-019-go-rewrite-design)
- [DD-LLM-004: Generalized Anthropic-Family Client and OpenAI-Compatible Client (langchaingo removal)](../architecture/decisions/DD-LLM-004-langchaingo-removal-generalized-clients.md)
- [DD-LLM-005: Model-Aware Reasoning/Thinking Token Support](../architecture/decisions/DD-LLM-005-model-aware-reasoning-support.md)
- [DD-LLM-007: AF and KA Intentionally Do Not Share an Anthropic Client](../architecture/decisions/DD-LLM-007-af-ka-anthropic-client-divergence.md)
- [DD-LLM-010: Adopt eino-ext's agenticgemini for KA's Native Gemini Client](../architecture/decisions/DD-LLM-010-eino-agenticgemini-for-ka-gemini-client.md)

**Related Issues**: #1778 (parent), #1792 (AF vertex_ai dispatch parity), #1793 (Helm chart docs)

---

## Business Need

### Problem Statement

Kubernaut Agent (KA) has no native Gemini/Google-GenAI client. `cmd/kubernautagent/llm_builder.go`'s provider dispatch:

1. Hardcodes the `vertex_ai` provider to `anthropicfamily.New` (KA's Claude client) regardless of the configured model — a KA process configured with `provider: vertex_ai` and a Gemini model (e.g. `gemini-2.5-pro`) silently sends requests to the Anthropic Messages API with a Gemini model name, which fails or behaves incorrectly rather than erroring clearly at startup.
2. Has no case at all for the standalone `gemini` provider (`types.LLMProviderGemini`), despite it being a validated `LLMConfig.Provider` enum value (`pkg/shared/types/llm.go`) — a KA process configured with `provider: gemini` fails at client construction with "unsupported LLM provider".

API Frontend (AF) already has a correct, working Gemini client (`google.golang.org/adk/model/gemini`) for the standalone `gemini` provider, but shares KA's identical `vertex_ai` dispatch bug (tracked separately as #1792, fixed by reusing AF's existing Gemini path — no new capability needed there).

### Business Objective

Give KA full-parity Gemini support — both the direct Gemini Developer API (`provider: gemini`) and Vertex-AI-hosted Gemini (`provider: vertex_ai` with a Gemini model) — including model-aware reasoning/thinking-token support (BR-AI-086), without regressing KA's existing Anthropic (`anthropicfamily`) or OpenAI-compatible (`openaicompat`) provider surfaces, and without compromising KA's framework-isolation guarantee (DD-KA-019).

---

## Acceptance Criteria

1. `buildLLMClientFromConfig` (`cmd/kubernautagent/llm_builder.go`) constructs a working Gemini-capable `llm.Client` for `provider: gemini` (direct API key auth).
2. `buildLLMClientFromConfig`'s `vertex_ai` case branches on model family: Anthropic-family models (`claude-*`) continue to route to `anthropicfamily.New` (zero regression); all other models route to the new Gemini client configured for the Vertex AI backend.
3. The new Gemini client implements KA's `llm.Client` interface (`Chat`, `StreamChat`, `Close`) and is never exposed to `internal/kubernautagent/investigator/*` business logic in any provider-specific form (DD-KA-019).
4. Tool-call requests/responses round-trip correctly through the new client (`llm.ToolDefinition`/`llm.ToolCall`), matching the existing behavior contract proven for `anthropicfamily` and `openaicompat`.
5. Where enabled (`cfg.Reasoning`), the new client requests and captures Gemini's reasoning/thinking output into `llm.ReasoningBlock`, reusing the shared `Effort` → `genai.ThinkingConfig` mapping (BR-AI-086 AC1, AC8; DD-LLM-005), including correct handling of Gemini's opaque thought-signature replay semantics across turns.
6. No regression to KA's existing Anthropic or OpenAI-compatible provider paths, hot-reload (`llm.SwappableClient`), or per-phase LLM overrides.
7. `charts/kubernaut`'s documented provider enum and examples correctly reflect that `gemini` is a supported provider and that Gemini models are valid under both `gemini` and `vertex_ai` (#1793).

---

## Out of Scope

- Replacing KA's existing `anthropicfamily.Client` with any Gemini-adjacent library component (see DD-LLM-010's "explicitly out of scope" section) — tracked separately as a deferred follow-up issue, not part of this BR.
- Any change to AF's Gemini or Anthropic/Vertex client implementations beyond the `vertex_ai` model-family branching fix tracked under #1792 (AF already has a working, correct Gemini client and needs no new capability here).

---

**Document Control**:
- **Created**: 2026-07-30
- **Version**: 1.0
- **Status**: Approved
