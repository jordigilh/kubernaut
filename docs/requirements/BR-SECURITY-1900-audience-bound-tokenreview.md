# BR-SECURITY-1900: Audience-Bound TokenReview Validation

**Business Requirement ID**: BR-SECURITY-1900
**Category**: Security — Authentication & Access Control
**Priority**: **P1 (HIGH)**
**Status**: ❌ Deferred to v1.6 for both API Frontend and Kubernaut Agent — see [§ Fully Deferred to v1.6](#fully-deferred-to-v16-1900). Only the [SOC2 audit-reason enrichment](#soc2-audit-reason-enrichment-delivered) below was delivered.
**Date**: August 3, 2026 (implemented and reverted the same day, see Changelog)
**GitHub Issues**: [#1900](https://github.com/jordigilh/kubernaut/issues/1900) (tracks the deferred work, targeting v1.6)

---

## Business Need

### Problem Statement

[BR-SECURITY-017](./BR-SECURITY-017-serviceaccount-token-authentication.md) established that Kubernaut REST API services authenticate Bearer tokens via the Kubernetes `TokenReview` API. That validation, as originally implemented, requests **no specific audience** (`Spec.Audiences` unset), so the API server returns its own default audience regardless of which service the token was minted for.

This means a valid ServiceAccount token minted (via the `TokenRequest` API) for one Kubernaut service — e.g. a token an operator or CI pipeline requested for API Frontend (AF) — is accepted just as readily by a *different* Kubernaut service that also trusts the cluster's `TokenReview` API. The token itself carries no service-scoping the receiving service currently checks.

### Impact Without This Requirement

- A ServiceAccount token minted for one service can be replayed against a different service that also uses `K8sAuthenticator`/`TokenReviewer`, if an attacker (or a misconfigured client) obtains it — a cross-service token-replay path.
- Kubernaut services cannot express "this token is only valid when presented to me" using the audience mechanism `TokenRequest`/`TokenReview` already provide for exactly this purpose.
- FedRAMP AC-6 (least privilege) is weaker than it could be: a credential's blast radius is the whole `TokenReview`-trusting mesh rather than the single service it was issued for.

---

## Requirement (deferred — not delivered in this iteration)

**API Frontend and Kubernaut Agent were to support requesting and enforcing audience-bound `TokenReview` validation**, such that:

1. Each service can be configured with one or more expected audiences (`apifrontend.auth.tokenReviewAudiences` / `kubernautagent.auth.expectedAFAudience`).
2. When configured, every outbound `TokenReview` request sets `Spec.Audiences` to the configured value(s), so an audience-aware API server enforces the match server-side (the primary defense).
3. Independently of server-side enforcement, the service verifies the returned `Status.Audiences` intersects the configured expected audiences. A `TokenReview` server that is audience-*unaware* (ignores `Spec.Audiences` and always reports `Authenticated: true` with empty `Status.Audiences`) MUST NOT be mistaken for one that enforced the match — this is defense-in-depth against a non-compliant or misconfigured API server.
4. When no audience is configured, behavior is unchanged from [BR-SECURITY-017](./BR-SECURITY-017-serviceaccount-token-authentication.md) (audience-unaware, fully backward compatible) — this is an opt-in hardening, not a breaking change.
5. A token that fails the audience check is rejected with the same error class as any other invalid token (`ErrInvalidAudience`), and the security-event audit log distinguishes an audience mismatch from other authentication failures for SOC2/FedRAMP audit review.

**Neither AF nor KA implements this requirement today** — both were implemented, then fully reverted; see [§ Fully Deferred to v1.6](#fully-deferred-to-v16-1900) below.

### Threat Model

**In scope**: A ServiceAccount token validly minted for one service is presented to a different service that also trusts the cluster's `TokenReview` API. This requirement would ensure that service rejects the token because it was not minted for its audience.

**Out of scope (explicit non-goal)**: A *compromised* token-issuing authority. If an attacker gains code execution inside a service that itself has permission to mint or forward tokens, audience binding does not help — the compromised service can request a token scoped to any audience it likes, including the victim's. Audience binding defends against **credential misuse/replay across trust boundaries**, not against a breach of a trusted minting authority. (This same boundary is documented in [DD-AUTH-016](../architecture/decisions/DD-AUTH-016-signed-user-identity-delegation.md)'s threat model for the analogous AF-minted identity JWT.)

### Grounding: What was built and reverted

- **Design Decision**: [DD-AUTH-014](../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md) v4.0 added the mechanism, v4.1/v4.2 documented its full reversion.
- Both `pkg/apifrontend/auth.TokenReviewer` (AF) and the shared `pkg/shared/auth.K8sAuthenticator` (KA, also used by DataStorage/Gateway) briefly gained a `WithExpectedAudiences(...)` functional option that set `Spec.Audiences` on the outbound request and cross-checked `Status.Audiences` client-side. Both were fully reverted in the same PR ([#1909](https://github.com/jordigilh/kubernaut/pull/1909)) — see below for why.
- DataStorage and Gateway also consume the shared `pkg/shared/auth.K8sAuthenticator` (`cmd/datastorage/main.go`, `pkg/gateway/server_constructors.go`) but never called `NewK8sAuthenticator` with an audience option — zero behavior change for those services throughout.

---

## Fully Deferred to v1.6 (#1900)

Both AF's and KA's audience-bound `TokenReview` were implemented, then fully reverted, in [#1909](https://github.com/jordigilh/kubernaut/pull/1909). [#1900](https://github.com/jordigilh/kubernaut/issues/1900) remains open, tracking this as deferred work for the v1.6 milestone, with the full analysis below recorded there too.

### KA: structurally cannot be bound safely

**Finding 1 — the only path with real AF→KA traffic can never be audience-bound.** `cmd/apifrontend/backend_deps.go` shows AF's investigation/remediation calls to KA go exclusively through `/api/v1/mcp` (`ka.NewSDKMCPClient`, `ka.NewKASessionPool`) — the trusted-intermediary MCP pattern. [DD-AUTH-MCP-001](../architecture/decisions/DD-AUTH-MCP-001-mcp-endpoint-security.md) documents this endpoint as a **permanent** architectural requirement to serve two structurally different populations on the same authenticator:

> "The same MCP endpoint must serve both in-cluster K8s clients (direct) and delegated clients (via apifrontend)"

The direct-client half of that population is an arbitrary K8s principal (a human's own ServiceAccount or user identity) that KA/AF has no ability to mint or coerce into carrying a `kubernaut-agent`-audience token — it isn't an E2E-test artifact or a missing feature, it's a permanent design constraint so direct in-cluster investigation clients keep working. Binding this endpoint's authenticator to an audience breaks that population outright, every time the option is enabled (confirmed by the `E2E (fullpipeline)` CI regression this revert followed).

**Finding 2 — the one path that could safely carry an audience check has zero production traffic.** `deps.KAClient` (`pkg/apifrontend/ka.Client`, exposing `Analyze`/`Status`/`Result`/`Cancel` over KA's plain REST API — a pre-MCP design) is constructed in `backend_deps.go`, but the only production call site (`cmd/apifrontend/mcp_a2a_handlers.go`) only ever calls `.Healthy()`, which is a local circuit-breaker state check with **no network call**. `Analyze`/`Status`/`Result`/`Cancel` have zero callers outside test files (`pkg/apifrontend/ka/integration_test.go`, `test/integration/apifrontend/ka_client_test.go`). Binding this path's authenticator to an audience would protect a dead code path — real security theater, not real security.

**Conclusion**: splitting KA's authenticator into a REST-only audience-checked instance and an MCP audience-unaware instance (a middle-ground option considered to fix the CI regression without a full revert) does not change the outcome — the REST path it would protect carries no real traffic, and the MCP path carrying all the real traffic is the one that permanently cannot be bound. Shipping the option regardless (even Helm-unset, opt-in only) would leave `pkg/shared/auth.K8sAuthenticator`'s `WithExpectedAudiences` with no genuine production caller anywhere (DS/GW don't use it either), which fails CHECKPOINT C (Business Integration Validation — no orphaned `pkg/` code without a real caller) the moment any deployment actually turned it on.

### AF: technically worked, but reverted for unclear incremental value

Unlike KA, AF's `TokenReviewer` has no dual-population constraint, and its audience check was fully implemented and proven against a real `kube-apiserver` via envtest (`IT-AF-1900-001/002`, both green — the only tests in the implementation that exercised a real, live audience check). It was reverted anyway on further review:

- AF already gates all tool/action use via `SubjectAccessReview` (`pkg/apifrontend/auth/sar.go`'s `SARChecker`) — an authenticated-but-audience-mismatched caller is already denied at the authorization layer regardless of whether the audience check exists. The audience check is defense-in-depth *only* at the authentication layer, narrowing which tokens are accepted before SAR ever runs; that has some value, but it's narrower than "closes a replay hole" as originally framed, since SAR already closes the practical exploitation path.
- This issue's original motivation — unblocking [kubernaut-operator#139](https://github.com/jordigilh/kubernaut-operator/issues/139)'s custom-audience projected-token plan for AF's ServiceAccount — is untouched by shipping AF's half in isolation, since KA's half (finding above) is what the operator plan actually depends on for AF→KA traffic. Shipping AF alone doesn't unblock the operator issue.
- Deferring both together to v1.6 keeps the decision coherent: revisit AF's audience check alongside a KA-side design that actually unblocks kubernaut-operator#139, rather than shipping half a feature now.

### What was reverted (both AF and KA)

- **AF**: `pkg/apifrontend/auth.TokenReviewer`'s `WithExpectedAudiences` option, `TokenReviewerOption` type, and the `Spec.Audiences`/`audiencesIntersect` check in `Validate` (`pkg/apifrontend/auth/tokenreview.go`); `pkg/apifrontend/config.AuthConfig.TokenReviewAudiences`; `cmd/apifrontend/auth_wiring.go`'s `buildTokenReviewFallback` wiring; all AF-specific audience tests (`UT-AF-1900-001..006` in `pkg/apifrontend/auth/tokenreview_test.go`, deleted entirely — it was a new file with no other content; `UT-AF-1900-005..007` in `pkg/apifrontend/config/config_test.go`; `IT-AF-1900-001..002` in `test/integration/apifrontend/auth_middleware_test.go`; the wiring test in `cmd/apifrontend/main_wiring_test.go`).
- **KA**: `pkg/shared/auth.K8sAuthenticator`'s `WithExpectedAudiences` option, `K8sAuthenticatorOption` type, and the `Spec.Audiences`/`audiencesIntersect` check in `ValidateTokenFull` (`pkg/shared/auth/k8s_auth.go`) — the shared authenticator used by KA, DataStorage, and Gateway is back to audience-unaware `TokenReview`, matching pre-#1900 behavior for all three; `internal/kubernautagent/config.AuthConfig`/`Config.Auth` (`ExpectedAFAudience` field) and its YAML parsing; `cmd/kubernautagent/routes.go`'s `k8sAuthenticatorOptions` helper and the `authCfg` parameter on `newAuthMiddleware`; all KA-specific audience-binding tests: `UT-KA-1900-001..005` (`pkg/shared/auth/k8s_auth_test.go`), `IT-KA-1900-003..005` (`cmd/kubernautagent/auth_wiring_1900_test.go`, deleted), `IT-KA-1900-001..002` (`test/integration/kubernautagent/auth/`, deleted — envtest suite existed solely for this), `E2E-KA-1900-001` (`test/e2e/kubernautagent/audience_e2e_test.go`, deleted).
- **Shared**: `test/infrastructure.GetServiceAccountTokenWithAudiences` (had no other caller); the Helm chart hardcoded `tokenReviewAudiences`/`expectedAFAudience` defaults for both services and their helm-unittest coverage (these were attempted and reverted earlier in the same PR, before the full revert of both services — see PR history).

### What was kept (unaffected by this revert)

- **SOC2/FedRAMP audit-reason enrichment** — see below. This is the only work item from the original #1900 implementation pass that actually shipped.

---

## SOC2 Audit-Reason Enrichment (Delivered)

Independent of the audience-binding mechanism, this work item enriches KA's persisted audit-table events (`aiagent.auth.failure`/`aiagent.auth.denied`) with a specific `reason` field, closing a gap flagged in the GA readiness audit (dimension 8, SOC2/FedRAMP Compliance): KA's `AuditAuthMiddleware` previously persisted a generic 401/403 event indistinguishable from any other cause.

This is generic to **any** classified auth-failure reason KA's shared middleware already produces — `missing_auth_header`, `invalid_auth_format`, `empty_bearer_token`, `invalid_token`, `authorization_denied` — not specific to audience mismatches, so it retains full value after the audience-binding revert above:

- **Schema**: [`api/openapi/data-storage-v1.yaml`](../../api/openapi/data-storage-v1.yaml) — optional `reason` field added to `AIAgentAuthFailurePayload`/`AIAgentAuthDeniedPayload`, regenerated into the `ogen` client (`pkg/datastorage/ogen-client`).
- **Capture mechanism**: [`pkg/shared/auth/middleware.go`](../../pkg/shared/auth/middleware.go) — `WithFailureReasonCapture` lets an outer HTTP wrapper (which only observes the response status code) retrieve the specific reason the shared `Middleware` classified for the request.
- **Wiring**: [`internal/kubernautagent/server/audit_middleware.go`](../../internal/kubernautagent/server/audit_middleware.go)'s `AuditAuthMiddleware` wraps the request context with `WithFailureReasonCapture`, then includes the captured reason in the persisted `AuditEvent.Data["reason"]`.
- **Payload mapping**: [`internal/kubernautagent/audit/ds_payloads.go`](../../internal/kubernautagent/audit/ds_payloads.go) — `buildAuthFailurePayload`/`buildAuthDeniedPayload` map `Data["reason"]` into the ogen `Reason` field.

The `invalid_token_audience` reason string itself remains defined (`pkg/shared/auth/middleware.go`'s `invalidTokenReason`, matching an "audience mismatch" substring) as forward-compatible, generic classification for any future authenticator that adopts the convention. It has **no live producer anywhere in the codebase today** — neither KA nor AF has an audience-checking authenticator after the full revert above. The tests exercising this reason string (`pkg/shared/auth`) construct a synthetic error by hand for exactly this reason.

---

## Success Criteria

Audience-bound `TokenReview` (AF and KA) is deferred — its success criteria will be re-established when [#1900](https://github.com/jordigilh/kubernaut/issues/1900) is picked up for v1.6. The criteria for the work that *was* delivered:

1. KA's persisted audit-table events for auth failures/denials (`aiagent.auth.failure`/`aiagent.auth.denied`) include a specific `reason` when the shared middleware classified one, and omit it otherwise (backward compatible).

---

## Related Documents

- [DD-AUTH-014](../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md) v4.2 — Middleware-Based SAR Authentication (primary design authority; audience-bound `TokenReview` addendum and its full revert history)
- [BR-SECURITY-017](./BR-SECURITY-017-serviceaccount-token-authentication.md) — Parent requirement: ServiceAccount token authentication via `TokenReview`
- [DD-AUTH-MCP-001](../architecture/decisions/DD-AUTH-MCP-001-mcp-endpoint-security.md) — MCP Endpoint Security (Trusted Intermediary Model); the documented dual-population constraint on KA's `/api/v1/mcp` that drove the KA revert above
- [DD-AUTH-016](../architecture/decisions/DD-AUTH-016-signed-user-identity-delegation.md) — Sibling design exploring signed user-identity delegation; shares this BR's "compromised minting authority is out of scope" threat-model boundary
- [#1900](https://github.com/jordigilh/kubernaut/issues/1900) — tracks the deferred audience-binding work for v1.6, carrying the full investigation findings
- [#1919](https://github.com/jordigilh/kubernaut/issues/1919) / [kubernaut-console#48](https://github.com/jordigilh/kubernaut-console/issues/48) — related but distinct authorization gap found during this review (console UI renders before any authorization check; proposed fix is a dedicated coarse-grained role, not an audience check) — tracked separately for v1.6, out of scope here
- Test evidence (SOC2 audit-reason enrichment — the only surviving delivered scope): `UT-KA-1900-012..015` ([`internal/kubernautagent/audit/auth_reason_payload_test.go`](../../internal/kubernautagent/audit/auth_reason_payload_test.go), [`internal/kubernautagent/server/audit_middleware_reason_test.go`](../../internal/kubernautagent/server/audit_middleware_reason_test.go)), `IT-KA-1900-006..011` ([`pkg/shared/auth/middleware_audience_reason_test.go`](../../pkg/shared/auth/middleware_audience_reason_test.go), [`pkg/shared/auth/failure_reason_capture_test.go`](../../pkg/shared/auth/failure_reason_capture_test.go))

---

**Document Version**: 3.0
**Last Updated**: August 4, 2026
**Maintained By**: Kubernaut Architecture Team

### Changelog

- **v3.0 (August 4, 2026)**: Reverted AF's audience-bound `TokenReview` too (full symmetry with the v2.0 KA revert) after concluding its incremental value was unclear given AF's existing SAR-based authorization already denies unauthorized tool access regardless of the authentication layer, and given the original motivation (kubernaut-operator#139) needs KA's half too, which remains structurally unbuildable. Audience-binding for both services is now fully deferred to [#1900](https://github.com/jordigilh/kubernaut/issues/1900), targeting v1.6. Only the SOC2 audit-reason enrichment work remains delivered. Also filed [#1919](https://github.com/jordigilh/kubernaut/issues/1919)/[kubernaut-console#48](https://github.com/jordigilh/kubernaut-console/issues/48) for a related, separate authorization gap found during this review (console UI has no pre-render authorization gate).
- **v2.0 (August 3, 2026)**: Descoped Kubernaut Agent entirely after tracing AF's real production traffic to KA (100% via the dual-purpose MCP endpoint, which DD-AUTH-MCP-001 requires to stay audience-unaware; the one path that could be bound has no production callers). Retained AF's audience check and the SOC2 audit-reason enrichment work, both unaffected by the KA finding at that time.
- **v1.0 (August 3, 2026)**: Initial version — audience-bound `TokenReview` for both AF and KA.
