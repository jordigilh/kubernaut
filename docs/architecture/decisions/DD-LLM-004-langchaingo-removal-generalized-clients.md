# DD-LLM-004: Remove langchaingo, Generalize Anthropic-Family Client, Extract Shared OpenAI-Compatible Core

**Status**: 📋 Proposed (2026-07-06)
**Priority**: P2
**Owner**: KubernautAgent Team
**Scope**: `pkg/kubernautagent/llm/*`, `pkg/shared/llm/*`, `pkg/apifrontend/launcher/openai/*`, `cmd/kubernautagent/llm_builder.go`
**Related**: [BR-AI-086](../../requirements/BR-AI-086-llm-reasoning-token-support.md), [DD-LLM-005](./DD-LLM-005-model-aware-reasoning-support.md), [DD-LLM-006](./DD-LLM-006-bedrock-dual-client-routing.md), [DD-HAPI-019](./DD-HAPI-019-go-rewrite-design) (Framework Isolation Pattern), Issue #1580, #1581

---

## Context & Problem

KA (`kubernautagent`) uses `github.com/tmc/langchaingo` (`pkg/kubernautagent/llm/langchaingo/adapter.go`) to serve every LLM provider except `vertex_ai`, which has its own hand-rolled client (`pkg/kubernautagent/llm/vertexanthropic/client.go`) built directly on `anthropic-sdk-go`.

Two independent problems motivate removing langchaingo:

1. **It cannot support reasoning/thinking token round-tripping.** `langchaingo`'s Anthropic backend reads `ThinkingContent` blocks on responses but `handleAIMessage` (`llms/anthropic/anthropicllm.go`) only reconstructs `llms.ToolCall` parts when replaying an assistant turn — there is no `ThinkingContent`/signature part type in `llms.MessageContent` at all, making correct replay (mandatory before any `tool_use` block per Anthropic's API) impossible without upstream changes.
2. **Maintenance risk independent of reasoning.** `tmc/langchaingo`'s default branch has not been pushed to since 2026-01-11 (~6 months), with 404 open issues and 111 open PRs; contributor concentration is heavily skewed to a single maintainer. Two independent, unmerged PRs ([tmc/langchaingo#1500](https://github.com/tmc/langchaingo/pull/1500), [#1505](https://github.com/tmc/langchaingo/pull/1505)) fix the same `handleAIMessage` content-dropping bug, tested against our exact production models (`claude-sonnet-4-6`, `claude-opus-4-7`), with zero review activity.

Separately, `pkg/apifrontend/launcher/openai/adapter.go` (AF) already proves a production, `langchaingo`-independent pattern for OpenAI-protocol providers: a hand-rolled `net/http` client covering OpenAI, LlamaStack, vLLM, and Ollama.

## Alternatives Considered

### Alternative A — Patch/vendor langchaingo just for the Anthropic replay bug (rejected)

- **Pros**: Smallest diff; keeps existing provider dispatch in `llm_builder.go` unchanged for `openai`/`ollama`/`azure`/etc.
- **Cons**: Only fixes the Anthropic-family reasoning gap; leaves the maintenance-risk problem (finding 2) completely unaddressed for every other provider; forking/vendoring a third-party dependency to hand-patch is itself an ongoing maintenance burden with no upstream path (both fix PRs are unreviewed).

### Alternative B — Remove langchaingo entirely; generalize `vertexanthropic` into an Anthropic-family client, extract AF's OpenAI-compatible pattern into a shared core (chosen)

- **Pros**: Solves both problems at once. `vertexanthropic.Client`'s `buildParams`/`mapResponse`/`convertAssistantMessage` already contain zero langchaingo dependency and only need new auth-mode constructors (native API key, Bedrock) to generalize — low risk, no protocol-logic changes. AF's adapter is production-proven for the OpenAI-compatible surface; extracting its protocol core into `pkg/shared/llm/openaicompat` avoids duplicating ~500 lines (and its test suite) across two codebases, and the reasoning-support work benefits AF too (AF currently has none). Every current langchaingo provider (`openai`, `ollama`, `azure`, `anthropic`, `bedrock`, plus undocumented `huggingface`/`mistral`) maps cleanly to one of the two resulting clients with zero orphaned providers — both `huggingface` and `mistral` now expose OpenAI-compatible `/v1/chat/completions` endpoints, so they fall out for free via config (`endpoint` + bearer token), no special-case code.
- **Cons**: Larger diff than Alternative A; touches AF's existing production adapter file as a refactor target (mitigated by AF's existing 11-test Ginkgo suite acting as a regression oracle, kept green throughout); requires a rename of `vertexanthropic` -> `anthropicfamily` across all callers/tests (mechanical, done via the `go-refactor-with-gopls` skill).

### Alternative C — Parallel port: duplicate AF's OpenAI-compatible pattern as a new KA-only package, leave AF's adapter untouched (rejected)

- **Pros**: Zero risk to AF's existing production file.
- **Cons**: Duplicates ~500 lines of HTTP/SSE/tool-call protocol logic and its test suite; reasoning-support work would need to be built and maintained twice; directly contradicts CHECKPOINT B ("search for existing implementations first, enhance existing patterns instead of creating new ones").

### Decision

**Alternative B** was selected by explicit user decision (2026-07-06), after being presented as the recommended option given the single-module, zero-module-boundary codebase already precedents cross-package sharing between `pkg/apifrontend/*` and `pkg/kubernautagent/*` (AF's `launcher/model.go` already imports `pkg/kubernautagent/llm/transport`).

## Implementation Summary

- `pkg/kubernautagent/llm/vertexanthropic` renamed to `pkg/kubernautagent/llm/anthropicfamily`, gains `WithNativeAuth`/`WithBedrockAuth` constructors alongside the existing Vertex auth path (see DD-LLM-006 for Bedrock specifics).
- New `pkg/shared/llm/openaicompat` package: plain-Go-struct protocol core (HTTP request construction, SSE parsing, tool-call delta accumulation, JSON schema conversion) with no `genai` or KA `llm.*` type dependency, ported from AF's `adapter.go`.
- `pkg/apifrontend/launcher/openai/adapter.go` refactored into a thin `genai`<->shared-type wrapper over `openaicompat.Client`; new `pkg/kubernautagent/llm/openai` package is the analogous thin `llm.Client`<->shared-type wrapper.
- `cmd/kubernautagent/llm_builder.go`'s `buildLLMClientFromConfig` dispatch replaced: `vertex_ai`/`anthropic`/`bedrock`+Claude -> `anthropicfamily`; everything else -> `pkg/kubernautagent/llm/openai`.
- `pkg/kubernautagent/llm/langchaingo/` package and `github.com/tmc/langchaingo` module dependency deleted once both new clients are wired.

## Consequences

### Positive
- Removes a maintenance-risk dependency entirely, not just for the Anthropic path.
- Both new clients gain reasoning support (DD-LLM-005) as a natural consequence of being purpose-built, rather than fighting an upstream library's data model.
- AF's OpenAI-compatible path gains reasoning support as a side benefit, at no extra implementation cost.
- No orphaned providers: every current langchaingo provider, including undocumented `huggingface`/`mistral`, has a clear home.

### Negative
- Larger single PR than a minimal patch would be — mitigated by splitting into three linked issues (#1580, #1581, #1582) landing together, each independently reviewable against its own scope.
- Refactoring AF's production adapter carries regression risk — mitigated by keeping AF's existing 11-test suite green throughout as the primary regression gate, plus a new minimal-capability fixture (DD-LLM-005) as a second, local-model-oriented regression gate.

## Related Decisions
- **Builds on**: DD-HAPI-019 (Framework Isolation Pattern — `llm.Client` must not leak provider-framework types into business logic)
- **Paired with**: DD-LLM-005 (reasoning support is the primary motivator, implemented on top of this generalization)
- **Enables**: DD-LLM-006 (Bedrock support reuses both resulting clients with zero new protocol code)

---

**Document Control**:
- **Created**: 2026-07-06
- **Version**: 1.0
- **Status**: Proposed — pending implementation
