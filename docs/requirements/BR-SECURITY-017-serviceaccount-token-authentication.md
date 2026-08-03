# BR-SECURITY-017: ServiceAccount Token Authentication

**Business Requirement ID**: BR-SECURITY-017
**Category**: Security — Authentication & Access Control
**Priority**: **P0 (CRITICAL)**
**Status**: ✅ Implemented
**Date**: August 3, 2026
**GitHub Issues**: [#1837](https://github.com/jordigilh/kubernaut/issues/1837)

---

## Business Need

### Problem Statement

Before a Kubernaut REST API service (DataStorage, Kubernaut Agent (KA), Gateway, AIAnalysis controller) can make an RBAC authorization decision ([BR-SECURITY-016](./BR-SECURITY-016-kubernetes-rbac-enforcement.md)), it must first establish *who* is making the request. Callers present a Kubernetes ServiceAccount token as a Bearer credential; the service must validate that the token is genuine, unexpired, and issued by the cluster it is running in — without trusting a client-supplied identity claim.

The prior approach delegated this validation to an `ose-oauth-proxy` sidecar, which is OpenShift-specific and untestable in Kind/envtest environments (the same limitation described in [BR-SECURITY-016](./BR-SECURITY-016-kubernetes-rbac-enforcement.md)).

### Impact Without This Requirement

- No verified identity exists at the application layer, so authorization (BR-SECURITY-016) and audit attribution (SOC2 CC8.1) have nothing trustworthy to key off.
- A client could present a forged or expired token, or spoof an `X-Auth-Request-User` header, without detection.
- Authentication logic tied to an OpenShift-only sidecar cannot be exercised in the project's Kind (E2E) or envtest (Integration) test tiers.

---

## Requirement

**Every Kubernaut REST API service MUST authenticate the identity of each incoming request's Bearer token using the Kubernetes `TokenReview` API** (for ServiceAccount tokens issued by the cluster's own API server) before any authorization or business-logic handling occurs, such that:

1. Authentication is enforced in-process, in the same codebase that is unit/integration/E2E tested (no sidecar dependency).
2. Token validation is delegated to the cluster's own token-issuing authority (`TokenReview`) — Kubernaut does not independently verify token signatures for cluster-local ServiceAccount tokens.
3. A missing `Authorization` header, a malformed (non-`Bearer`) header, an empty token, or a token the API server marks `authenticated: false` all result in `401 Unauthorized`.
4. Any client-supplied `X-Auth-Request-User` header (a SOC2 trust-anchor value downstream services rely on) is stripped from the inbound request *before* authentication, so a caller cannot spoof the identity that downstream services will trust.
5. The verified username (and group memberships, where applicable) is injected into the request context for use by authorization (BR-SECURITY-016) and audit logging (SOC2 CC8.1), never trusted from client input.

### Grounding: Design and Implementation

- **Design Decision**: [DD-AUTH-014](../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md) — Middleware-Based SAR Authentication (Interface-Driven).
- **Implementation**:
  - [`pkg/shared/auth/k8s_auth.go`](../../pkg/shared/auth/k8s_auth.go) — `K8sAuthenticator.ValidateTokenFull` issues the `TokenReview` request against the Kubernetes API server and returns the verified `UserInfo` (username + groups) or a typed `ErrTokenInvalid`.
  - [`pkg/shared/auth/middleware.go`](../../pkg/shared/auth/middleware.go) — `Middleware.Handler` strips the `X-Auth-Request-User` header and Kubernetes impersonation headers *before* calling `authenticateRequest`, which extracts the Bearer token and delegates to the injected `Authenticator`, mapping `ErrTokenInvalid` to `401` and any other authenticator error to `500`.
  - `Authenticator` is an interface ([`pkg/shared/auth/interfaces.go`](../../pkg/shared/auth/interfaces.go)); production wiring uses `K8sAuthenticator` (real `TokenReview` calls), tests inject `MockAuthenticator`.
  - **Composite routing**: [`pkg/shared/auth/composite_auth.go`](../../pkg/shared/auth/composite_auth.go)'s `CompositeAuthenticator` routes JWT-shaped tokens to [`jwt_auth.go`](../../pkg/shared/auth/jwt_auth.go)'s `JWTAuthenticator` first (Pattern B: external OIDC providers with a registered JWKS endpoint per [DD-AUTH-MCP-001](../architecture/decisions/DD-AUTH-MCP-001-mcp-endpoint-security.md)); tokens whose issuer is not a registered external provider — including cluster-local ServiceAccount tokens — fall back to `K8sAuthenticator`'s `TokenReview` path (Pattern A). This BR's scope is Pattern A: cluster-issued ServiceAccount token authentication.
- **Production callers** (confirms this is wired, not just unit-tested): [`cmd/datastorage/main.go`](../../cmd/datastorage/main.go), [`cmd/kubernautagent/routes.go`](../../cmd/kubernautagent/routes.go), [`pkg/gateway/server_constructors.go`](../../pkg/gateway/server_constructors.go) all construct `auth.NewK8sAuthenticator(...)` and wire it into `auth.NewMiddleware(...)` for their respective route sets.

---

## Success Criteria

1. A request with no `Authorization` header, a non-`Bearer` scheme, or an empty Bearer token receives `401 Unauthorized`.
2. A request with a Bearer token that the Kubernetes API server's `TokenReview` marks `authenticated: false` (expired, revoked, or malformed) receives `401 Unauthorized`.
3. A request with a valid, currently-bound ServiceAccount token is authenticated, and the resulting username is available to downstream authorization and audit logging via the request context.
4. A client-supplied `X-Auth-Request-User` header is always overwritten by the server-verified identity (or absent, on auth failure) — never passed through unmodified.
5. A `TokenReview` API call failure (API server unreachable) receives `500 Internal Server Error`, distinguishing an authentication-service outage from an invalid credential.
6. The same `Middleware`/`K8sAuthenticator` code path is exercised in Integration tests (via `MockAuthenticator`, envtest) and E2E tests (via `K8sAuthenticator`, real Kind cluster `TokenReview` calls) — proving the authentication logic itself, not just a mock, is validated end-to-end at least once per service.
7. No runtime flag exists to disable authentication enforcement in production code.

---

## Related Documents

- [DD-AUTH-014](../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md) — Middleware-Based SAR Authentication (primary design authority)
- [DD-AUTH-013](../architecture/decisions/DD-AUTH-013/DD-AUTH-013-http-status-codes-oauth-proxy.md) — HTTP status code mapping for auth errors
- [BR-SECURITY-016](./BR-SECURITY-016-kubernetes-rbac-enforcement.md) — Sibling requirement: RBAC authorization, gated by this authentication check
- Test evidence: [`test/e2e/datastorage/23_sar_access_control_test.go`](../../test/e2e/datastorage/23_sar_access_control_test.go)

---

**Document Version**: 1.0
**Last Updated**: August 3, 2026
**Maintained By**: Kubernaut Architecture Team
