# DD-LLM-012: Independent Complete LLM Profile Resolution for Sibling Agents

**Status**: Implemented  
**Date**: 2026-10-15  
**Scope**: `internal/kubernautagent/config`, `cmd/kubernautagent`, Helm LLM profile rendering  
**Related**: kubernaut#2515, kubernaut-operator#419, BR-AI-090, BR-SECURITY-1726, DD-LLM-008

## Context

Kubernaut Agent currently represents workflow-discovery and alignment settings
with `LLMOverrideConfig`. The type is a partial override and its merge methods
copy the investigation configuration for fields the override does not expose.
That makes a distinct sibling profile inherit investigation-owned credentials,
OAuth2, TLS/mTLS, endpoint, and transport settings.

The shared `types.LLMConfig` already represents the complete client contract.
The operator's named profile model also defines complete profile data, but the
current rendering path does not expose every field to all sibling consumers.

## Decision

Use one shared resolver that produces an independent, deep-copied complete
`types.LLMConfig` for each enabled agent:

- Investigation resolves the static `ai.llm` profile with its hot-reloadable
  runtime envelope.
- Workflow discovery resolves its complete phase profile directly.
- Alignment resolves its complete dedicated profile directly; when no dedicated
  profile is configured, the existing intentional shared-client behavior remains.
- API-key and OAuth2 credentials are resolved per effective profile.
- Full `types.LLMConfig.Validate` validation is applied to every effective
  profile before client construction.
- No secret values are included in logs.

The partial `LLMOverrideConfig` contract is removed rather than expanded. A
profile with omitted fields is invalid or uses the provider's documented zero
value; it does not inherit those fields from investigation. Explicitly shared
profiles may reuse the same rendered values and mounted paths.

## Alternatives Considered

### Alternative A — Complete `types.LLMConfig` profiles and one resolver (chosen)

**Pros**: one source of truth for validation and client construction; zero-value
inheritance cannot silently cross agent boundaries; all transport and credential
fields are represented; future profile fields do not require another partial
override field.  
**Cons**: configuration producers and existing tests/docs must migrate to the
complete contract; operator rendering/mount parity remains a coordinated
follow-up.

### Alternative B — Expand `LLMOverrideConfig` indefinitely (rejected)

**Pros**: smaller initial code movement.  
**Cons**: every new `types.LLMConfig` field must be duplicated; absent versus
explicitly empty values remain ambiguous; partial merge semantics continue to
allow credential and transport bleed; validation remains split across paths.

### Alternative C — Change only Helm/operator rendering (rejected)

**Pros**: avoids KA code changes.  
**Cons**: the KA runtime would still merge and validate sibling profiles
indirectly, so independently selected OAuth2/TLS/mTLS credentials would not be
guaranteed; it cannot satisfy BR-AI-090.

## Consequences

Positive:

- Sibling LLM clients are configuration-isolated by construction.
- Credential and transport resolution is auditable and testable in one place.
- Existing provider clients and `buildTransportChain` remain reusable.
- DD-LLM-008's provider/model restart lock remains intact.

Negative:

- Existing partial `phaseModels` and alignment YAML must be migrated; this is
  intentional and authorized by the issue decision to discard legacy support.
- The operator must render complete sibling profiles and their credential/TLS
  mounts before all deployments can use the new contract; that work is tracked
  separately in kubernaut-operator#419.
