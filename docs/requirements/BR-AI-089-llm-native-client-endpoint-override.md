# BR-AI-089: Native-Auth-Mode LLM Client Endpoint Override

**Business Requirement ID**: BR-AI-089
**Category**: AI (Kubernaut Agent LLM provider configuration)
**Priority**: P2
**Target Version**: V1.6
**Status**: Approved

**Related Design Decisions**:
- [DD-LLM-004: Generalized Anthropic-Family Client and OpenAI-Compatible Client (langchaingo removal)](../architecture/decisions/DD-LLM-004-langchaingo-removal-generalized-clients.md)
- [DD-LLM-007: AF and KA Intentionally Do Not Share an Anthropic Client](../architecture/decisions/DD-LLM-007-af-ka-anthropic-client-divergence.md)
- [DD-LLM-010: Adopt eino-ext's agenticgemini for KA's Native Gemini Client](../architecture/decisions/DD-LLM-010-eino-agenticgemini-for-ka-gemini-client.md)

**Related Issues**: #2255 (parent, Anthropic native), #1819 (KubernautAgent NetworkPolicy egress allowlist, keyed on `ai.llm.endpoint` — direct consumer of this BR), #1342 (LLM transport chain parity — headers/OAuth2/mTLS already solved for these providers)

---

## Business Need

### Problem Statement

Kubernaut Agent (KA)'s native-auth-mode LLM client constructors — `buildAnthropicNativeClient` and `buildGeminiNativeClient` (`cmd/kubernautagent/llm_builder.go`) — never read `cfg.Endpoint` when building their respective clients. Both are left to their SDK's hardcoded default (`https://api.anthropic.com` for Anthropic; `generativelanguage.googleapis.com` for Gemini), regardless of what an operator configures under `ai.llm.endpoint`.

This is a silent configuration footgun, not a hard failure: `types.LLMConfig.Validate` and `internal/kubernautagent/config`'s `LLMRuntimeConfig.Validate` both already treat `endpoint` as *optional* (not forbidden) for `provider: anthropic`/`provider: gemini`, and validate it as a well-formed URL when set. An operator can configure `ai.llm.endpoint` for these providers today, have it pass all validation at startup, and have zero effect on where requests actually go — worse than an outright rejection, because the misconfiguration is invisible.

This is asymmetric with KA's other provider paths:
- `provider: openai`/`openai_compatible` **require** an explicit endpoint (no universal default makes sense for self-hosted/Azure/vLLM/LiteLLM).
- `provider: vertex_ai` derives its endpoint from `vertexProject`/`vertexLocation` via the Anthropic/Gemini SDKs' own Vertex middleware.
- `provider: anthropic`/`gemini` (native) silently ignore any endpoint override entirely.

The rest of KA's transport chain (custom headers, OAuth2 client-credentials, mTLS via `tlsCaFile`/`tlsCertFile`) is already wired for native `anthropic`/`gemini` via `buildTransportChain(cfg)` (issue #1342) — the **only** missing piece for these two provider paths is the target URL itself.

### Business Objective

Allow operators to route native-auth-mode Anthropic and Gemini traffic through an enterprise AI gateway (e.g. Envoy AI Gateway) or private network proxy instead of the provider's public SaaS default, closing the last provider-path gap in KA's ability to enforce a controlled LLM egress boundary.

### Compliance Driver

**FedRAMP AC-4 (Information Flow Enforcement)**: `openai`/`openai_compatible` already satisfy this control today (endpoint is required); `vertex_ai` satisfies it implicitly via GCP project/location scoping. Native `anthropic`/`gemini` are the last two provider paths where KA cannot enforce a controlled egress path.

This is not a hypothetical control gap: open issue #1819 (KubernautAgent NetworkPolicy hardening, v1.6 milestone) adds a `kubernaut.np.llmEgress` Helm NetworkPolicy helper that allowlists KA's egress **specifically keyed off `ai.llm.endpoint`**. Without this BR, that allowlist cannot be correctly enforced for `provider: anthropic`/`gemini`: an operator would configure `ai.llm.endpoint` expecting it to both (a) define the NetworkPolicy allowlist target and (b) be where the client actually sends requests, but today only (a) would hold — the client would keep calling the provider's public default underneath, either breaking connectivity once the NetworkPolicy is enforced (fail-closed) or, if the public default is also reachable, silently defeating the intended boundary.

SC-8 (transmission confidentiality) is unaffected/orthogonal — TLS trust already goes through the separate `tlsCaFile`/`tlsCertFile` machinery regardless of endpoint (proven independently per issue #1342).

---

## Acceptance Criteria

1. `buildAnthropicNativeClient` (`cmd/kubernautagent/llm_builder.go`) passes a non-empty `cfg.Endpoint` through to `anthropicfamily.NewWithAPIKey` via a new first-class `anthropicfamily.WithBaseURL` option, so the Anthropic SDK sends requests to the configured endpoint instead of `https://api.anthropic.com`.
2. `buildGeminiNativeClient` (`cmd/kubernautagent/llm_builder.go`) passes a non-empty `cfg.Endpoint` through to `geminifamily.NewWithAPIKey` via the existing `geminifamily.WithHTTPOptions(genai.HTTPOptions{BaseURL: ...})` option, mirroring the pattern already production-wired in API Frontend's Gemini triager (`cmd/apifrontend/backend_deps.go`).
3. When `cfg.Endpoint` is empty (the default, and every existing deployment's current configuration), both clients continue to hit their SDK's default endpoint with zero behavior change — no regression for existing deployments.
4. `anthropicfamily.New` (the Vertex-hosted constructor) does not silently accept a `WithBaseURL` override — Vertex's endpoint is derived from `vertexProject`/`vertexLocation` via the SDK's own Vertex middleware, and accepting-but-ignoring the option would reintroduce the exact silent-footgun class this BR fixes, just scoped to Vertex instead of native auth.
5. `buildAnthropicVertexClient` and `buildGeminiVertexClient` (Vertex-hosted constructors) are explicitly out of scope for this BR — their endpoint resolution is a materially different, SDK-internal mechanism not covered by issue #2255.
6. No change to `types.LLMConfig.Validate` (`pkg/shared/types/llm.go`) — it already correctly treats `endpoint` as optional-but-well-formed for `anthropic`/`gemini`; this BR is a pure wiring fix at that layer.

   **Known pre-existing asymmetry (not fixed by this BR, called out for follow-up)**: at the separate hot-reload runtime layer (`internal/kubernautagent/config.LLMRuntimeConfig.Validate`), `providersWithoutEndpointRequirement` exempts `anthropic` but not `gemini` — so `provider: gemini` in `llm-runtime.yaml` today *requires* a non-empty `endpoint` to pass validation, even though (pre-this-BR) that endpoint was silently ignored at the client-construction layer just like `anthropic`'s. This BR does not change that validation map. A practical consequence worth flagging to operators: any existing `provider: gemini` runtime config that set `endpoint` to a placeholder/arbitrary value (since it was previously inert) will, after this fix, start actually routing there — operators should confirm their configured `endpoint` is the intended target (or the real `https://generativelanguage.googleapis.com` value) before/while upgrading past this fix.

---

## Out of Scope

- Vertex-hosted endpoint override (`buildAnthropicVertexClient`, `buildGeminiVertexClient`) — see AC5.
- Any change to API Frontend's own LLM client construction — AF's Gemini triager already implements this pattern correctly (`cmd/apifrontend/backend_deps.go`); AF's Anthropic/Vertex path has no reasoning/endpoint parity work tracked here (see DD-LLM-007).
- Issue #1819's NetworkPolicy egress allowlist implementation itself — this BR is a prerequisite/enabler for that issue, not a substitute for it.

---

**Document Control**:
- **Created**: 2026-08-25
- **Version**: 1.0
- **Status**: Approved
