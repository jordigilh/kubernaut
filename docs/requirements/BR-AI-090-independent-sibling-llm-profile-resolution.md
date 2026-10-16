# BR-AI-090: Independent Complete LLM Profile Resolution for Sibling Agents

**Business Requirement ID**: BR-AI-090  
**Category**: AI (Kubernaut Agent LLM configuration)  
**Priority**: P1  
**Target Version**: V1.6  
**Status**: Implemented (scoped validation complete)  
**Related Issue**: [kubernaut#2515](https://github.com/jordigilh/kubernaut/issues/2515)  
**Related Operator Issue**: [kubernaut-operator#419](https://github.com/jordigilh/kubernaut-operator/issues/419)

## Business Need

Investigation, workflow discovery, and alignment are independent LLM agents. A
sibling agent selected with a distinct profile MUST use that profile's complete
effective `types.LLMConfig`; it MUST NOT inherit unset provider, endpoint,
credential, OAuth2, TLS/mTLS, Vertex, retry, timeout, or other transport values
from investigation.

The previous `LLMOverrideConfig` partial-merge contract is retired by this BR.
Sibling configuration is authored as a complete profile. Sharing a profile is
allowed, but sharing MUST be explicit through the same rendered profile values
and mounted credential paths, not implicit zero-value inheritance.

## Acceptance Criteria

1. Investigation, workflow discovery, and alignment resolve through one shared
   effective-LLM resolution and validation path.
2. Each resolved sibling has an independent complete `types.LLMConfig`,
   including provider/model, endpoint, API-key source, Vertex project/location,
   OAuth2 token URL/scopes/credentials directory, TLS CA and mTLS files,
   reasoning, retries, and timeouts.
3. A distinct sibling profile cannot inherit investigation credentials,
   OAuth2, TLS/mTLS, endpoint, or transport settings.
4. Full profile validation is applied consistently to all three agents and
   failures are observable without logging secret contents.
5. Existing phase/runtime reload behavior remains covered: provider/model
   identity changes still require restart; safe profile tuning continues to
   reload through the existing phase client path.
6. Tests prove distinct provider, credential, OAuth2, TLS/mTLS, Vertex,
   endpoint, retry, and timeout configurations for every sibling path.
7. Omitting or disabling one sibling does not mutate or alter another sibling's
   effective configuration.

## Control Objectives

This engineering requirement provides evidence aligned with FedRAMP AC-6,
IA-5, SC-8, SC-12, and SI-10. It does not by itself make a formal compliance
claim.

## Out of Scope

- Supporting the retired partial `LLMOverrideConfig` contract.
- Changing LLM provider implementations or transport-chain behavior beyond
  consuming the complete effective configuration.
- Live-provider E2E tests requiring external credentials; provider behavior is
  proven with isolated integration doubles and generated TLS material.
- Operator repository changes; operator rendering/mount parity remains tracked
  by kubernaut-operator#419.
