# DD-LLM-006: AWS Bedrock Provider Support via Dual-Client Routing

**Status**: 📋 Proposed (2026-07-06)
**Priority**: P3
**Owner**: KubernautAgent Team
**Scope**: `cmd/kubernautagent/llm_builder.go`, `pkg/kubernautagent/llm/anthropicfamily`, `pkg/kubernautagent/llm/openai`
**Related**: [BR-AI-086](../../requirements/BR-AI-086-llm-reasoning-token-support.md), [DD-LLM-004](./DD-LLM-004-langchaingo-removal-generalized-clients.md), [DD-LLM-005](./DD-LLM-005-model-aware-reasoning-support.md), Issue #1582

---

## Context & Problem

The existing `langchaingo`-based `bedrock` provider (being removed per DD-LLM-004) covered AWS Bedrock generically via `langchaingo`'s Bedrock backend. Its replacement must not regress existing Bedrock configurations.

**Documentation spike (2026-07-06)**: no live AWS Bedrock access is currently available to this team; the endpoint/model mapping below is sourced from AWS's own documentation (`docs.aws.amazon.com/bedrock/latest/userguide/{apis,inference-chat-completions-mantle,models-endpoint-availability}.html`) plus two third-party sources cross-checking model-name conventions (promptfoo's Bedrock provider docs, a February 2026 community write-up of Bedrock Mantle model IDs). **Caveat**: AWS's per-model endpoint-availability matrix renders support as checkmark icons that do not survive text extraction — the qualitative routing rule below is corroborated by the promptfoo source's explicit statement of which model families are Mantle-only, but the *exhaustive* per-model matrix must be re-verified against live Bedrock access (or a screenshot/rendered fetch of the AWS page) before merge, per the pre-merge verification task below. This DD captures the requirement cleanly now; implementation is tracked separately in issue #1582 and may land in its own PR once Bedrock test access is available, decoupled from #1580/#1581's landing.

AWS Bedrock exposes **two distinct endpoints**, both of which can serve an OpenAI-compatible Chat Completions surface, plus one Anthropic-native surface:

| Endpoint | APIs | Auth | Model coverage |
|---|---|---|---|
| `bedrock-runtime.{region}.amazonaws.com` (legacy/general) | `InvokeModel`, `Converse`, Chat Completions, Messages API | AWS SigV4 or Bedrock API key (bearer) | Models Bedrock has natively integrated: Claude (all variants), Nova, Llama, Titan, Cohere, Mistral's core lineup, Qwen — reachable via `Converse`/`InvokeModel` **and** via this endpoint's own OpenAI-compatible Chat Completions path (e.g. `model: "anthropic.claude-sonnet-4-6"` against `bedrock-runtime`'s `/v1/chat/completions`, confirmed in AWS's own Chat Completions doc example). |
| `bedrock-mantle.{region}.api.aws` (newer, AWS-recommended for OpenAI-compatible access) | Responses API, Chat Completions API, Anthropic Messages API | Bedrock API key or AWS credentials (bearer token, no SigV4 required) | Third-party/open models with **no native `Converse`/`InvokeModel` integration** — DeepSeek, Z.AI GLM, Google Gemma (Bedrock-hosted variants), MiniMax, Moonshot Kimi, Nvidia Nemotron, several Mistral variants, Qwen's Mantle-namespaced IDs — plus OpenAI's own GPT-5.x/GPT-OSS models and xAI Grok, where the **Responses API specifically (not just Chat Completions) is required to surface reasoning tokens**. |

This is a **three-way** routing decision, not the two-way split originally assumed: (1) Claude-on-Bedrock via `anthropic-sdk-go`'s native Bedrock support (unchanged from the original plan), (2) natively-integrated non-Claude models (Nova, Llama, etc.) via `bedrock-runtime`'s OpenAI-compatible Chat Completions surface, (3) Mantle-only third-party/open models via `bedrock-mantle`'s OpenAI-compatible surface. (2) and (3) both route through the same `openaicompat`-based client since both speak the same wire protocol — only the base URL and, for reasoning-capable Mantle models, the API path (Responses vs. Chat Completions) differ.

## Alternatives Considered

### Alternative A — Single new `pkg/kubernautagent/llm/bedrock` client wrapping the AWS SDK directly (rejected)

- **Pros**: One package, one mental model for "Bedrock".
- **Cons**: Duplicates protocol logic (message conversion, tool-call handling, reasoning mapping) that DD-LLM-004's two clients already implement; a Claude-on-Bedrock request is byte-for-byte the same Anthropic Messages API shape `anthropicfamily` already builds, just routed to a different endpoint/auth — reimplementing it in a third client directly contradicts CHECKPOINT B.

### Alternative B — Route Bedrock to whichever of the two DD-LLM-004 clients matches the configured model's protocol, via a model-ID dispatch heuristic (chosen)

- **Pros**: Zero new protocol code. `anthropicfamily` gains a `WithBedrockAuth` constructor (AWS SigV4 or Bedrock API key auth instead of Vertex OAuth2/native API key — only the auth/transport layer differs, `buildParams`/`mapResponse` are unchanged) for Claude-on-Bedrock model IDs (`anthropic.*`/`*claude*` prefixes). `pkg/kubernautagent/llm/openai` gains Bedrock endpoint support for everything else, pointed at `bedrock-runtime` or `bedrock-mantle` depending on the model. Both clients get reasoning support "for free" as a consequence of DD-LLM-005, so Bedrock does too.
- **Cons**: Requires a model-ID dispatch table in `llm_builder.go`, now three-way rather than two-way (Claude -> `anthropicfamily`; natively-integrated non-Claude -> `openai` client against `bedrock-runtime`; Mantle-only third-party models -> `openai` client against `bedrock-mantle`, with Responses-API path selection for OpenAI/xAI reasoning models specifically). Mitigated by explicit unit tests pinning each known model-ID pattern to its expected endpoint/client, and treating an unrecognized model ID as a hard construction error (fail closed) rather than a silent guess.

### Decision

**Alternative B**, per explicit user direction ("can we support openapi as well? I don't want to have regressions with the current configuration") — this also directly satisfies the compatibility-floor principle in DD-LLM-005 for Bedrock's non-Claude models, since they route through the same fail-closed OpenAI-compatible client.

## Verification Required Before Merge

The documentation spike above establishes the *shape* of the routing requirement (three-way dispatch, two possible base URLs, Responses-vs-Chat-Completions path selection for reasoning-capable Mantle models) with reasonable confidence, but AWS's exhaustive per-model endpoint-availability matrix could not be extracted as text (checkmark icons, not machine-readable) and must be re-verified against live Bedrock access before merge — tracked as its own pre-merge checklist item, not a blocking spike (this is a fast-changing external fact, not an architectural unknown). Given no live Bedrock access is currently available to this team, implementation of this DD is tracked in issue #1582 as a **separate, later PR**, decoupled from #1580/#1581's landing.

## Consequences

### Positive
- No new protocol/client code; Bedrock support is a routing decision on top of DD-LLM-004's two clients.
- Existing Bedrock configurations continue to work (Claude-on-Bedrock via `anthropicfamily`, matching prior `langchaingo` Bedrock+Claude behavior); OpenAI-compatible Bedrock models are a net-new capability, not a regression surface.

### Negative
- Model-ID prefix heuristic is a maintenance point if AWS changes Bedrock model ID conventions — mitigated by fail-closed behavior on unrecognized prefixes (explicit config error, never a silent wrong-protocol guess) and unit tests pinning known prefixes.

---

**Document Control**:
- **Created**: 2026-07-06
- **Version**: 1.0
- **Status**: Proposed — pending implementation and pre-merge endpoint verification
