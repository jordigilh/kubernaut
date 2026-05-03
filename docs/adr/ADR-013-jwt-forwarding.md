# ADR-013: JWT Forwarding for AF-to-KA Identity Delegation

**Status:** Accepted
**Date:** 2026-05-03
**Deciders:** AF team, kubernaut team (kubernaut#1009)
**Source:** kubernaut#1009, AF #55

## Context

AF needs to delegate user identity to KA so that KA can execute K8s API calls on behalf of the authenticated user. The original design (DD-AUTH-MCP-001 v1.0) proposed that AF authenticate with its own SA token and inject `Impersonate-*` headers. The kubernaut team proposed an alternative in #1009.

## Decision

AF forwards the user's **original Keycloak JWT** to KA in the `Authorization: Bearer` header. KA validates the JWT independently via JWKS and extracts user identity from verified claims.

This decision was made by the kubernaut team (kubernaut#1009) and adopted by AF after review (validated 2026-05-03).

## Consequences

- **Simpler AF implementation:** One header pass-through, no token minting, no Impersonate-* header injection for KA calls
- **Cryptographic integrity:** JWT signature proves identity (cannot be forged by network intermediary)
- **Defense-in-depth:** Both AF and KA independently validate the same JWT
- **Multi-issuer forward-compatible:** v1.6 adds SPIRE as second JWT issuer — no AF middleware refactor
- **Keycloak configuration required:** JWT must include `kubernaut-agent` in `aud` claim (audience mapper on Keycloak client)
- **AF still uses impersonation for own K8s API calls:** `af_list_events`, `af_get_pods` etc. still use AF SA + Impersonate-User/Group against K8s API
- **JWT replay window:** Forwarded JWT can be replayed within its validity period. Mitigated by: ClusterIP (no external access to KA), NetworkPolicy, short JWT lifetimes. v1.6 path: AF mints short-lived (30s) internal JWTs.

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| AF SA token + Impersonate-* headers to KA | Two tokens (SA + user), unsigned headers (spoofable if NetworkPolicy fails), more complex AF code |
| mTLS client certificate | Complex PKI setup, cert rotation, identity still needs supplementary headers |
| AF mints short-lived internal JWT | Over-engineered for v1.5 (internal network only); planned for v1.6 |
| Shared secret / HMAC token | No standard, no key rotation, no claim extensibility |
