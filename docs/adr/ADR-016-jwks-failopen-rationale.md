# ADR-016: JWKS Cache Fail-Open Behavior and Risk Acceptance

**Status:** Accepted
**Date:** 2026-05-04
**Deciders:** AF team
**NIST Controls:** IA-5, SC-23, SI-10

## Context

AF's JWT validator uses a JWKS cache with circuit breakers per issuer. When the JWKS
endpoint is unreachable (circuit breaker open), the system must decide between:

1. **Fail-closed**: reject all tokens immediately, causing a full outage for authenticated users.
2. **Fail-open (with cached keys)**: continue validating tokens against previously cached JWKS keys.

In a FedRAMP environment, the default posture is fail-closed. However, a brief JWKS
endpoint outage should not cascade into a total service denial when cryptographically
valid cached keys are still available.

## Decision

AF implements **conditional fail-open**:

- **If cached JWKS keys exist**: tokens are validated against the cached keyset.
  Signature verification, expiry, audience, and CEL rules are still enforced. Only the
  key freshness guarantee is relaxed.
- **If no cached keys exist** (first-boot or cache eviction): the system **fail-closes**
  and returns 503 via the `/readyz` probe, causing the load balancer to stop routing traffic.

This behavior is implemented in `internal/auth/jwks_cache.go` via the `GetKeys` method
and surfaced through the `JWKSCache.Healthy()` / `JWTValidator.Ready()` chain used by
the `/readyz` probe.

## Risk Acceptance

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Attacker compromises signing key during cache window | Very Low | High | Keys rotate on a schedule; cached keys are valid for at most one TTL cycle. Key compromise requires OIDC provider breach, which is outside AF's threat boundary. |
| Revoked key continues to validate tokens | Low | Medium | JWKS rotation propagation is typically < 5 minutes. Circuit breaker half-open timeout (30s) ensures rapid re-fetch attempts. |
| Stale keys used indefinitely | Low | Medium | Circuit breaker transitions to half-open after 30s, allowing re-fetch. Pods restart on deployment, clearing caches. |

## NIST 800-53 Mapping

- **IA-5(2)**: Cryptographic authentication is maintained even during fail-open; only
  the key fetch is bypassed, not signature verification.
- **SC-23**: Session authenticity is preserved through cached key validation.
- **SI-10**: Input validation (JWT structure, claims) remains enforced regardless of
  cache state.

## Consequences

- Operators MUST monitor `af_circuit_breaker_state{dependency="jwks_*"}` and alert on
  value `2` (open) sustained for > 60 seconds.
- The `/readyz` probe returns 503 when any JWKS circuit breaker is open, preventing
  new traffic from reaching a degraded instance.
- Key rotation events should be coordinated with JWKS cache TTL to minimize stale-key windows.
