# BR-AI-086: Model-Aware LLM Reasoning/Thinking Token Support

**Business Requirement ID**: BR-AI-086
**Category**: AI (Kubernaut Agent investigation quality)
**Priority**: P2
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-07-06

**Related Design Decisions**:
- [DD-HAPI-019: Framework Isolation Pattern (Go rewrite design)](../architecture/decisions/DD-HAPI-019-go-rewrite-design)
- [DD-LLM-004: Generalized Anthropic-Family Client and OpenAI-Compatible Client (langchaingo removal)](../architecture/decisions/DD-LLM-004-langchaingo-removal-generalized-clients.md)
- [DD-LLM-005: Model-Aware Reasoning/Thinking Token Support](../architecture/decisions/DD-LLM-005-model-aware-reasoning-support.md)
- [DD-LLM-006: AWS Bedrock Provider Support via Dual-Client Routing](../architecture/decisions/DD-LLM-006-bedrock-dual-client-routing.md)

**Related Issues**: #1578 (parent), #1580, #1581, #1582

**Note**: This requirement supersedes no prior BR. `BR-HAPI-263` (conversation continuity) describes the API surface of the deprecated Python HolmesGPT-API (HAPI) service (`InvestigateRequest.previous_messages`, `investigate_issues`/`prompt_call`) and does not apply to Kubernaut Agent (KA)'s current Go implementation. HAPI is no longer a service in Kubernaut and must not be referenced or extended by current or future work; this BR is written fresh against KA's actual `llm.Client`/`Investigator` implementation.

---

## Business Need

### Problem Statement

Kubernaut Agent (KA)'s LLM clients never request, capture, or round-trip reasoning/thinking tokens for any provider (Anthropic native/Vertex, OpenAI-protocol, self-hosted). Investigation quality currently depends entirely on prompted, single-pass reasoning: the RCA output schema (`internal/kubernautagent/parser/schema.go`) forces the model to commit to `remediation_target` before generating `due_diligence.alternative_hypotheses`, and self-correction retries only see the model's already-committed prior answer plus a correction message — not a genuine pre-conclusion deliberation step. Reasoning/thinking tokens (Anthropic extended thinking, DeepSeek `reasoning_content`, OpenAI reasoning models) give the model an explicit space to deliberate before committing to a structured answer, and — where visible — improve auditability of *why* a conclusion was reached.

This gap was originally identified while triaging OpenShift Lightspeed issue OLS-3442 (reasoning tokens dropped at cache/turn boundaries) for applicability to KA. Investigation confirmed the same two structural gaps exist in KA:

- **Gap 1**: no code path requests reasoning/thinking output from any provider.
- **Gap 2**: even where a provider/model would return reasoning content, no code path captures it or round-trips it across turns/phases (self-correction retries, Phase 1 RCA -> Phase 3 Workflow Discovery per the three-phase architecture).

### Business Objective

Add opt-in, model-aware reasoning/thinking token support across every LLM provider KA supports, without regressing behavior for models/providers that don't support it (see the compatibility-floor principle in DD-LLM-005), and without introducing the same failure class as issue #1299 (orphaned content blocks on replay).

---

## Acceptance Criteria

1. `llm.ChatOptions` and `llm.Message` (`pkg/kubernautagent/llm/types.go`) expose provider-agnostic reasoning fields (opaque signature/redaction support, visible text) — business logic in `internal/kubernautagent/investigator/*` remains completely unaware of provider-specific reasoning wire formats (DD-HAPI-019).
2. Reasoning defaults to **disabled** (`Enabled: false`) for every provider/model until an explicit operator opt-in, and unsupported/unknown models skip the reasoning parameter rather than erroring.
3. Where enabled and supported, reasoning content (including any required opaque signature) is correctly round-tripped across:
   - Self-correction retries within a phase (mutated-in-place message history, `internal/kubernautagent/investigator/investigator_workflow_selection.go`)
   - Validation gate retries (`internal/kubernautagent/investigator/investigator_gates.go`)
4. Reasoning state does not leak across a phase handoff (Phase 1 -> Phase 3) or a hot-reload model swap (`llm.SwappableClient.Swap`) — proven by regression test, not merely assumed from existing architecture.
5. Self-hosted/custom models (served via the OpenAI-compatible client) support an explicit capability override (`auto` / `force_on` / `force_off`) since they cannot be reliably identified by vendor enum alone.
6. Captured reasoning content is surfaced in the investigation audit trail per SOC2 CC7.2 / BR-AUDIT-005.
7. No regression to current investigation behavior for any provider/model when reasoning is left at its default-disabled state.

---

## Non-Goals (Explicitly Out of Scope)

- Native OpenAI Responses API / `encrypted_content` reasoning continuity (o-series, GPT-5.x) — different endpoint and state model than Chat Completions; tracked as a future decision requiring a new dependency (`github.com/openai/openai-go`).
- Enabling reasoning by default anywhere — requires a separate RCA-quality measurement spike (with vs. without thinking) first. Tracked in [kubernaut-demo-scenarios#401](https://github.com/jordigilh/kubernaut-demo-scenarios/issues/401).
- Prompt caching, strict structured outputs, typed-error retry classification, `tool_choice` forcing, interleaved thinking, token-counting API — identified as adjacent provider-SDK capability gaps during preflight, intentionally deferred to separate follow-on issues to keep this requirement scoped to reasoning tokens only. Filed as [#1583](https://github.com/jordigilh/kubernaut/issues/1583), [#1584](https://github.com/jordigilh/kubernaut/issues/1584), [#1585](https://github.com/jordigilh/kubernaut/issues/1585), [#1586](https://github.com/jordigilh/kubernaut/issues/1586), [#1587](https://github.com/jordigilh/kubernaut/issues/1587), [#1588](https://github.com/jordigilh/kubernaut/issues/1588).
- `kubernaut-operator` CRD field for `LLMReasoningSpec` — tracked separately in [kubernaut-operator#211](https://github.com/jordigilh/kubernaut-operator/issues/211), can land independently of this BR's implementation.

---

## Success Criteria

- All acceptance criteria above hold across the Anthropic-family client (native/Vertex/Bedrock-Claude) and the shared OpenAI-compatible client (OpenAI/Azure/self-hosted/Bedrock-other).
- Zero regressions in existing investigation test suites with reasoning left disabled (the default).
- `go build ./...`, `golangci-lint run`, and `make test` pass with `github.com/tmc/langchaingo` fully removed from `go.mod`.

---

## Confidence Assessment

**Confidence Level**: 85%

**Strengths**:
- Existing phase-handoff architecture already satisfies acceptance criterion 4 by construction (verified by reading `investigator.go`/`phase_resolver.go`, not assumed) — reduces implementation risk.
- `ChatWithParams`/`InstrumentedClient`/`SwappableClient` require zero changes — new clients only need to satisfy the existing `llm.Client` interface.
- The Anthropic-family thinking-tier detection logic can be reimplemented directly against `anthropic-sdk-go` types with zero new dependencies (confirmed via spike — see DD-LLM-005).

**Risks**:
- Exact shared-core API shape for the new `pkg/shared/llm/openaicompat` package will only firm up once AF's existing adapter is actually decomposed in the RED phase.
- Bedrock per-model endpoint availability on AWS's OpenAI-compatible surface is unverified against live documentation until the pre-merge verification task (#1582) runs.

---

**Document Control**:
- **Created**: 2026-07-06
- **Version**: 1.0
- **Status**: Approved
