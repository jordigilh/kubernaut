# DD-AUTH-016: Signed User Identity Delegation for Downstream Services

**Status**: Proposed
**Decision Date**: 2026-05-29
**Version**: 1.0
**Confidence**: 85%
**Deciders**: Architecture Team
**Applies To**: kubernaut-apifrontend, kubernaut-agent, data-storage

**Related Business Requirements**:
- BR-INTERACTIVE-003: Audit attribution for interactive actions
- BR-SECURITY-016: Kubernetes RBAC enforcement for REST API endpoints

**Related Design Decisions**:
- DD-AUTH-MCP-001 v3.0: Trusted Intermediary Model (current production baseline)
- DD-AUTH-005: DataStorage Client Authentication Pattern
- DD-AUTH-014: Middleware-Based SAR Authentication

**Inspiration**: [Solving the Identity Crisis for AI Agents](https://www.uber.com/us/en/blog/solving-the-agent-identity-crisis/) (Uber Engineering, May 2026)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-29 | AI-assisted | Initial enhancement proposal |

---

## Context & Problem

### Current State (DD-AUTH-MCP-001 v3.0)

Kubernaut uses a Trusted Intermediary model for user identity delegation. When a human user interacts with the system through API Frontend (AF), the identity flow is:

```
Human (OIDC JWT) → AF (validates JWT, extracts UserIdentity)
  → KA (AF's SA token + acting_user/acting_user_groups in MCP JSON payload)
  → DS (KA's SA token; no user identity)
```

- **AF authenticates the human** via OIDC JWT or K8s TokenReview. This is the sole authorization boundary — SAR checks on `kubernaut.ai/tools/<toolName>` determine what the user can do.
- **AF → KA**: AF authenticates with its own ServiceAccount token. User identity is passed as plain string fields (`acting_user`, `acting_user_groups`) in MCP tool arguments. KA trusts these fields because it trusts AF's SA (validated via TokenReview + SAR).
- **KA → DS**: KA authenticates with its own SA token. No user identity is propagated. DS sees `system:serviceaccount:kubernaut-system:kubernaut-agent`.
- **AF → DS**: AF authenticates with its own SA token. No user identity is propagated. DS sees `system:serviceaccount:kubernaut-system:apifrontend`.

### What Works Well

The current model is architecturally sound for Kubernaut's purpose-built, fixed-topology design:

1. **SA + TokenReview + SAR at each hop** provides cryptographic service-level authentication and authorization. Each service verifies its caller's workload identity via K8s-native mechanisms.
2. **Authorization is centralized at AF** by design. Downstream services don't need to re-authorize the human because AF already made that decision. This is deliberate — Kubernaut has a fixed service topology, not a generic agent-to-agent platform.
3. **User identity in MCP payload is for audit attribution only**, not for authorization decisions at KA or DS.

### Problem Statement

The `acting_user` and `acting_user_groups` fields in the MCP payload are **unsigned plain strings**. While the service-level authentication (SA + TokenReview) is cryptographic, the user attribution is not:

1. **No integrity guarantee on user claims**: If AF has a bug that incorrectly sets `acting_user` (not a compromise — just a code defect), KA records the wrong user in its audit trail with no way to detect the error. The field is trusted implicitly rather than verified independently.

2. **User identity stops at KA**: DS has no knowledge of which human triggered the action. Audit events in DS are attributed to the calling service's SA. Answering "what did Alice cause to happen in DS?" requires joining AF audit events → KA session metadata → DS audit events by correlation ID.

3. **No expiry on user claims**: The `acting_user` string has no temporal binding. A stale or replayed MCP payload carries the same user attribution indefinitely, unlike a JWT with `exp`.

### Scope

This proposal enhances **audit integrity**, not authorization. The authorization model (AF as single boundary) is correct and unchanged. The goal is to make the user attribution that flows downstream **verifiable and tamper-evident** for audit purposes.

### Non-Goals

- **Not changing the authorization model**: AF remains the sole authorization boundary. KA and DS do not make per-user authorization decisions. This is by design for Kubernaut's fixed topology.
- **Not building a generic agent-to-agent identity platform**: Kubernaut is not Uber. We have ~11 services with a deterministic call graph, not thousands of agents composing dynamically. We do not need SPIRE, STS, actor chains, or per-hop token exchange.
- **Not adding K8s impersonation**: DD-AUTH-MCP-001 v3.0 deliberately removed runtime K8s impersonation (#1288).

---

## Decision Drivers

1. **Audit integrity**: User attribution should be verifiable, not just trusted
2. **Minimal infrastructure**: No new services (no STS, no SPIRE); leverage what AF already has
3. **Temporal binding**: User claims should have a bounded validity window
4. **Downstream visibility**: DS should know which human triggered an action (for audit, not authz)
5. **Backward compatibility**: KA Pattern A (direct K8s clients) unaffected
6. **Forward compatibility**: Aligns with v1.6 SPIRE roadmap (#31)

---

## Alternatives Considered

### Alternative A: Status Quo — Plain String `acting_user`

**Approach**: Keep current DD-AUTH-MCP-001 v3.0 model unchanged.

**Pros**:
- Zero implementation effort
- Proven and stable
- SA + TokenReview provides strong service-level security

**Cons**:
- User attribution is unsigned — no integrity guarantee
- DS has zero visibility into originating human
- Audit stitching required to answer "what did Alice do?"
- Bug in AF user extraction would silently corrupt audit trail

**Confidence**: 70% (adequate but improvable)

### Alternative B: AF-Minted Short-Lived JWT for User Claims — PROPOSED

**Approach**: AF mints a short-lived, signed JWT containing the user's identity claims and passes it to downstream services alongside the existing SA token. Downstream services validate the JWT signature to verify user attribution for audit purposes. The SA token remains the authentication/authorization mechanism.

**Identity JWT structure**:

```json
{
  "iss": "kubernaut-apifrontend",
  "sub": "alice@example.com",
  "groups": ["sre-team", "kubernaut-operators"],
  "iat": 1748530000,
  "exp": 1748530300,
  "jti": "req-abc123",
  "purpose": "audit-attribution"
}
```

**Transport**: Passed as a secondary header (`X-Kubernaut-Identity-Token`) or as a field in MCP tool arguments (replacing the plain `acting_user` string). The primary `Authorization: Bearer` header remains the service's SA token.

**Signing**: AF signs with an HMAC or RSA key. For HMAC, KA and DS share the symmetric key via K8s Secret. For RSA, AF signs with a private key and KA/DS verify with the public key (preferred — no secret sharing).

**Validation at KA/DS**:
1. Verify signature (proves AF issued it, not forged)
2. Check `exp` (reject stale claims)
3. Extract `sub` and `groups` for audit attribution
4. If validation fails: log a warning, attribute to calling SA (graceful degradation, not a hard failure — this is audit, not authz)

**Pros**:
- User attribution is cryptographically signed — AF bugs that corrupt the `UserIdentity` struct would be caught because the JWT was signed at OIDC validation time
- Temporal binding via `exp` — stale claims are rejected
- DS gains visibility into originating human for audit
- No new services — AF already has crypto capabilities (OIDC validation, JWKS)
- Graceful degradation — JWT validation failure doesn't block the request
- Forward-compatible with v1.6 SPIRE (SPIRE could become the signer)

**Cons**:
- KA and DS need JWT validation logic (lightweight — signature check + exp, not full OIDC)
- Key management: signing key must be provisioned (K8s Secret or projected volume)
- Additional header/field in the wire format

**Confidence**: 85% (proposed)

### Alternative C: Forward User's Original OIDC JWT — REJECTED

**Approach**: AF passes the user's original Keycloak/OIDC JWT to KA and DS (the v2.0 approach from DD-AUTH-MCP-001).

**Pros**:
- No AF-side minting — just forward the existing token
- Standard JWT validation at KA/DS

**Cons**:
- Long-lived tokens (Keycloak tokens are typically 5-30 minutes) — wider replay window than necessary
- Requires KA and DS to have JWKS endpoint access for every OIDC provider
- Couples downstream services to external OIDC infrastructure
- Already rejected in DD-AUTH-MCP-001 v3.0 for good reasons

**Confidence**: 40% (rejected — revisits a previously rejected approach)

---

## Decision

### Proposed: Alternative B — AF-Minted Short-Lived Identity JWT

AF mints a lightweight, short-lived JWT at the point of OIDC validation and includes it in downstream calls. KA and DS validate the signature for audit attribution. The SA token remains the sole authentication/authorization mechanism.

### Architecture

```
Human (OIDC JWT) → AF
  AF validates OIDC JWT → extracts UserIdentity
  AF mints identity JWT (sub=alice, groups=[...], exp=now+5m)
  AF → KA: Authorization: Bearer <AF SA token>
           X-Kubernaut-Identity-Token: <identity JWT>
  AF → DS: Authorization: Bearer <AF SA token>
           X-Kubernaut-Identity-Token: <identity JWT>

KA receives request:
  1. Validate SA token via TokenReview + SAR (existing — unchanged)
  2. Validate identity JWT signature + expiry (new — audit only)
  3. Use JWT claims for audit attribution (replacing plain acting_user)
  4. If JWT validation fails: warn + fall back to SA attribution

DS receives request:
  1. Validate SA token via TokenReview + SAR (existing — unchanged)
  2. Validate identity JWT signature + expiry (new — audit only)
  3. Use JWT claims for audit event attribution
  4. If JWT validation fails: warn + fall back to SA attribution
```

### Key Design Properties

1. **Audit only, not authz**: JWT validation failure is a warning, not a 401/403. The request proceeds; audit attributes to SA instead of human. Authorization is unchanged (SA + SAR).

2. **Short-lived**: Identity JWT `exp` is set to 5 minutes (configurable). Much shorter than the original OIDC token, reducing the replay window.

3. **AF-signed**: AF is the sole issuer. KA and DS verify AF's signature, not the external OIDC provider's. This keeps downstream services decoupled from OIDC infrastructure.

4. **Signed at validation time**: The JWT is minted when AF validates the OIDC token, so the claims reflect what was cryptographically verified, not what application code set afterward. This closes the "AF bug corrupts acting_user" gap.

5. **Graceful degradation**: If the signing key isn't configured or JWT validation fails, the system behaves exactly as it does today — SA-attributed audit events. No service outage.

### Signing Key Management

**Recommended approach**: RSA key pair stored as a K8s Secret in the kubernaut-system namespace. AF reads the private key; KA and DS read the public key. Helm chart provisions the secret.

**Alternative**: K8s `TokenRequest` API with a custom audience (e.g., `kubernaut-audit-identity`). AF requests a projected SA token with the user's identity encoded in annotations. This avoids custom key management but has weaker claim flexibility.

### Wire Format Options

**Option 1 — HTTP header** (recommended for REST paths):

```
X-Kubernaut-Identity-Token: eyJhbGciOiJSUzI1NiIs...
```

**Option 2 — MCP tool argument** (for MCP paths, replacing `acting_user`):

```json
{
  "identity_token": "eyJhbGciOiJSUzI1NiIs...",
  "signal_id": "..."
}
```

Both options can coexist. The HTTP header works for REST calls (AF → DS, KA → DS). The MCP argument works for MCP protocol calls (AF → KA).

### Validation Implementation

Shared utility in `pkg/shared/auth/`:

```go
type IdentityJWTValidator struct {
    publicKey *rsa.PublicKey
}

func (v *IdentityJWTValidator) ValidateIdentityClaims(tokenString string) (*AuditIdentity, error) {
    // Parse and validate signature + exp
    // Return AuditIdentity{Username, Groups} or error
}
```

KA and DS middleware extract the header, call the validator, and inject the result into the request context for audit handlers. Existing audit event emission code reads from context.

---

## Consequences

### Positive

1. **Tamper-evident audit attribution**: User claims are signed by AF; downstream services can verify they weren't corrupted in transit or by a bug
2. **DS gains human visibility**: Audit events in DS can be attributed to the originating human, not just the calling SA
3. **Temporal binding**: Short-lived JWT with `exp` rejects stale identity claims
4. **No authorization change**: AF remains the sole authz boundary; the model that works stays unchanged
5. **Graceful degradation**: System works identically to today if identity JWT is missing or invalid
6. **Forward-compatible**: v1.6 SPIRE (#31) can replace AF as the JWT signer without changing KA/DS validation logic

### Negative

1. **Key management overhead**: A signing key must be provisioned and rotated
   - **Mitigation**: K8s Secret managed by Helm chart; key rotation via standard Secret update
2. **JWT validation in KA and DS**: Both services gain lightweight JWT verification code
   - **Mitigation**: Shared utility in `pkg/shared/auth/`; signature check + exp only, not full OIDC
3. **Wire format change**: New header or MCP field
   - **Mitigation**: Backward-compatible; missing JWT falls back to current behavior

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Signing key compromise | Low | Medium | Key rotation; K8s Secret RBAC; short-lived JWTs limit blast radius |
| Key provisioning adds deployment complexity | Medium | Low | Helm chart handles it; optional feature gate for environments that don't need it |
| JWT validation adds latency | Low | Low | Signature verification is <1ms; no network calls |
| Teams forget to include identity JWT in new service integrations | Medium | Low | Shared transport layer (DD-AUTH-005 pattern) injects it automatically |

---

## Implementation Scope

### Affected Components

| Component | Change | Effort |
|-----------|--------|--------|
| `pkg/shared/auth/` | New: `identity_jwt.go` (minter + validator) | Small |
| `pkg/apifrontend/auth/` | Mint identity JWT after OIDC validation | Small |
| `pkg/apifrontend/ka/` | Include identity JWT in MCP calls (replacing `acting_user` string) | Small |
| `cmd/apifrontend/main.go` | Load signing key from Secret/file | Small |
| `pkg/shared/auth/transport.go` | Propagate `X-Kubernaut-Identity-Token` header in `AuthTransport` | Small |
| `cmd/kubernautagent/main.go` | Load public key; add identity JWT middleware | Small |
| `pkg/datastorage/server/` | Add identity JWT middleware; use claims in audit attribution | Small |
| Helm chart | New Secret template for signing key pair | Small |

### Not Affected

- AF authorization logic (SAR on tools) — unchanged
- KA autonomous mode (Pattern A) — unchanged
- KA per-tool authorization — unchanged (still AF's job)
- DS per-verb SAR — unchanged (still service-level)
- Auth webhook — unchanged (uses K8s admission UserInfo, not AF identity)
- All CRD controllers — unchanged (don't receive external user identity)

---

## Validation Strategy

1. **Unit tests**: Identity JWT minting and validation (signature, expiry, claim extraction, invalid token handling, graceful degradation)
2. **Integration tests**: AF → KA flow with identity JWT; verify audit events attribute correct user
3. **Integration tests**: AF → DS flow with identity JWT; verify DS audit events attribute correct user
4. **Negative tests**: Missing JWT falls back to SA attribution; expired JWT triggers warning + SA fallback; tampered JWT is rejected
5. **E2E tests**: Full flow from OIDC login → AF → KA → DS; verify end-to-end audit trail attributes human identity at every hop

---

## Target Version

**v1.6** — aligns with SPIRE workload identity roadmap (#31). The identity JWT infrastructure built here becomes the foundation that SPIRE-minted tokens can replace in the future.

---

## References

- [Solving the Identity Crisis for AI Agents](https://www.uber.com/us/en/blog/solving-the-agent-identity-crisis/) — Uber Engineering (May 2026). Describes cryptographic actor chains for agent-to-agent identity delegation at scale. Kubernaut's fixed topology doesn't require the full STS + SPIRE + Actor Chain architecture, but the principle of **signed identity claims for audit integrity** applies directly.
- DD-AUTH-MCP-001 v3.0: Current trusted intermediary model (production baseline)
- DD-AUTH-005: DataStorage client authentication pattern (transport layer injection)
- DD-AUTH-014: Middleware-based SAR authentication
- kubernaut#31: SPIRE workload identity binding (v1.6 roadmap)

---

**Document Version**: 1.0
**Last Updated**: 2026-05-29
