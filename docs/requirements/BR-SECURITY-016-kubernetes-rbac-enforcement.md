# BR-SECURITY-016: Kubernetes RBAC Enforcement for REST API Endpoints

**Business Requirement ID**: BR-SECURITY-016
**Category**: Security — Authentication & Access Control
**Priority**: **P0 (CRITICAL)**
**Status**: ✅ Implemented
**Date**: August 3, 2026
**GitHub Issues**: [#1837](https://github.com/jordigilh/kubernaut/issues/1837)

---

## Business Need

### Problem Statement

Kubernaut's REST API services (DataStorage, Kubernaut Agent (KA), Gateway, AIAnalysis controller) expose HTTP endpoints that create, read, or mutate cluster-affecting resources — audit events, workflow catalog entries, AI investigation sessions, and remediation signals. Without authorization enforcement, any caller that reaches the network endpoint (in-cluster or, for Gateway, external webhook senders) could invoke these endpoints regardless of identity, bypassing the principle of least privilege (FedRAMP AC-6) and the SOC2 CC7.2 requirement for internal-control-backed decision auditing.

An earlier approach relied on an `ose-oauth-proxy` sidecar to perform authorization external to the application. This was OpenShift-specific, could not be exercised in Kind (E2E) or envtest (integration) environments, and placed authorization logic outside the auditable application boundary.

### Impact Without This Requirement

- Any workload with network reachability to a Kubernaut REST endpoint could perform privileged operations (e.g., writing audit events, mutating the workflow catalog) with no identity check.
- No SOC2/FedRAMP-compliant record of *who* was authorized to perform *what* action exists at the application layer.
- Authorization logic living in an external sidecar cannot be unit- or integration-tested as part of the Go codebase, and is not portable off OpenShift.

---

## Requirement

**Every Kubernaut REST API service MUST authorize each authenticated request against Kubernetes RBAC before executing the requested operation**, using the Kubernetes `SubjectAccessReview` (SAR) API as the authorization decision point — not a network-perimeter control (NetworkPolicy) or an external proxy — so that:

1. Authorization is enforced in-process, in the same codebase that is unit/integration/E2E tested (no sidecar dependency).
2. The RBAC decision is delegated to the cluster's own `Role`/`ClusterRole`/`RoleBinding` objects — Kubernaut does not maintain a parallel permissions model.
3. The check is portable across OpenShift, vanilla Kubernetes, and Kind — no OpenShift-specific resources required.
4. Denial (`403 Forbidden`) and authorization-service failure (`500 Internal Server Error`) are distinguished from authentication failure (`401 Unauthorized`), per [DD-AUTH-013](../architecture/decisions/DD-AUTH-013/DD-AUTH-013-http-status-codes-oauth-proxy.md).
5. Every authorization decision (grant or deny) is observable as a structured security-event log entry (FedRAMP AU-2).

### Grounding: Design and Implementation

- **Design Decision**: [DD-AUTH-014](../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md) — Middleware-Based SAR Authentication (Interface-Driven). Supersedes [DD-AUTH-011](../architecture/decisions/DD-AUTH-011/DD-AUTH-011-granular-rbac-sar-verb-mapping.md) (RBAC via oauth-proxy) and [DD-AUTH-012](../architecture/decisions/DD-AUTH-012/DD-AUTH-012-ose-oauth-proxy-sar-rest-api-endpoints.md) (`ose-oauth-proxy` sidecar).
- **Implementation**:
  - [`pkg/shared/auth/k8s_auth.go`](../../pkg/shared/auth/k8s_auth.go) — `K8sAuthorizer.CheckAccess`/`CheckAccessWithGroup` issue the `SubjectAccessReview` request against the Kubernetes API server.
  - [`pkg/shared/auth/middleware.go`](../../pkg/shared/auth/middleware.go) — `Middleware.authorizeRequest` invokes the injected `Authorizer` after authentication succeeds, maps the result to `403`/`500`, and emits a `security_event` log entry via `logSecurityEvent` for every denial.
  - `Authorizer` is an interface ([`pkg/shared/auth/interfaces.go`](../../pkg/shared/auth/interfaces.go)); production wiring uses `K8sAuthorizer` (real SAR calls), tests inject `MockAuthorizer`.
- **Production callers** (confirms this is wired, not just unit-tested): [`cmd/datastorage/main.go`](../../cmd/datastorage/main.go), [`cmd/kubernautagent/routes.go`](../../cmd/kubernautagent/routes.go), [`pkg/gateway/server_constructors.go`](../../pkg/gateway/server_constructors.go) all construct `auth.NewK8sAuthorizer(...)` and wire it into `auth.NewMiddleware(...)` for their respective route sets.
- **RBAC contract**: [`deploy/data-storage/service-rbac.yaml`](../../deploy/data-storage/service-rbac.yaml) and equivalent per-service manifests define the `Role`/`RoleBinding` objects that the SAR check evaluates against.

---

## Success Criteria

1. A request with a valid, authenticated identity but no RBAC grant for the target `resource`/`verb` receives `403 Forbidden`.
2. A request where the SAR API call itself fails (API server unreachable) receives `500 Internal Server Error`, not a silent allow (fail-closed for authorization-service errors) or a silent deny masquerading as `403`.
3. A request with both a valid identity and an RBAC grant is allowed through to the handler.
4. Every `403` and `500` authorization outcome produces a structured `security_event` log entry carrying user, resource, verb, path, and status code (FedRAMP AU-2/AU-3).
5. The same `Middleware`/`K8sAuthorizer` code path is exercised in Integration tests (via `MockAuthorizer`, envtest) and E2E tests (via `K8sAuthorizer`, real Kind cluster SAR calls) — proving the authorization logic itself, not just a mock, is validated end-to-end at least once per service.
6. No runtime flag exists to disable authorization enforcement in production code.

---

## Related Documents

- [DD-AUTH-014](../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md) — Middleware-Based SAR Authentication (primary design authority)
- [DD-AUTH-013](../architecture/decisions/DD-AUTH-013/DD-AUTH-013-http-status-codes-oauth-proxy.md) — HTTP status code mapping for auth errors
- [DD-AUTH-011](../architecture/decisions/DD-AUTH-011/DD-AUTH-011-granular-rbac-sar-verb-mapping.md) — Superseded by DD-AUTH-014
- [DD-AUTH-012](../architecture/decisions/DD-AUTH-012/DD-AUTH-012-ose-oauth-proxy-sar-rest-api-endpoints.md) — Superseded by DD-AUTH-014
- [DD-AUTH-016](../architecture/decisions/DD-AUTH-016-signed-user-identity-delegation.md) — Cites this BR for interactive-session identity delegation
- [BR-SECURITY-017](./BR-SECURITY-017-serviceaccount-token-authentication.md) — Sibling requirement: authentication (identity) as the precondition for this authorization check
- Test evidence: [`test/e2e/datastorage/23_sar_access_control_test.go`](../../test/e2e/datastorage/23_sar_access_control_test.go)

---

**Document Version**: 1.0
**Last Updated**: August 3, 2026
**Maintained By**: Kubernaut Architecture Team
