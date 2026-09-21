# Test Plan: Console OAuth2 Proxy Provider Logout Persistence

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2448-v1
**Feature**: Persist the Console OAuth2 Proxy provider logout endpoint in the Helm chart
**Version**: 1.0
**Created**: 2026-09-20
**Author**: Kubernaut Platform
**Status**: Draft
**Branch**: `fix/2448-console-provider-logout`

---

## 1. Purpose and Success Criteria

Issue #2448 addresses a logout gap where OAuth2 Proxy clears its local cookie but
the browser can immediately authenticate against the existing OIDC provider session.
The chart must persist an operator-configured provider `end_session_endpoint` without
adding client-side token handling or a user-controlled post-logout redirect.

Success requires:

1. The configured provider logout URL renders as OAuth2 Proxy's
   `--backend-logout-url` argument with the literal `{id_token}` placeholder preserved.
2. The argument is absent when the value is unset, preserving existing releases.
3. Malformed logout endpoint values fail closed during Helm rendering.
4. A real `helm upgrade` produces the configured Deployment argument and a subsequent
   upgrade/reconciliation preserves it.
5. The existing Console live browser journey reaches the IdP login state after logout,
   requires a fresh login, and does not restore the previous transcript.

---

## 2. Authority and Control Objectives

### Business requirements

- `BR-PLATFORM-006`: Helm chart Console feature and security parity.
- Issue #2448: persistent provider logout configuration.
- `kubernaut-console#139`: user-visible logout and fresh-login behavior.

### FedRAMP controls

| Control | Business behavior evidenced |
|---|---|
| AC-2 | The configured provider session termination endpoint is owned by the operator configuration. |
| AC-6 | No default or user-controlled provider/logout target is introduced. |
| AC-12 | OAuth2 Proxy and the OIDC provider session terminate before a new login is allowed. |

### SOC 2 controls

| Control | Business behavior evidenced |
|---|---|
| CC6.1 | Authentication session termination remains at the trusted proxy/IdP boundary. |
| CC6.3 | The authenticated Console session cannot be silently restored after logout. |

### OWASP ASVS objectives

| Objective | Business behavior evidenced |
|---|---|
| V3.3 | Logout invalidates the proxy and provider sessions. |
| V5 | Endpoint configuration is validated and malformed values fail closed. |
| V7.2.1 | The security-relevant logout journey is observable through the live authentication result. |
| V8 | The ID token remains server-side; only the literal placeholder is rendered into the proxy configuration. |

These tests are business-level evidence and are not a standalone compliance claim.

---

## 3. Risks and Mitigations

| ID | Risk | Impact | Affected tests | Mitigation |
|---|---|---|---|---|
| R1 | The value is accepted but never reaches the OAuth2 Proxy container. | Logout remains broken after upgrade. | `IT-HELM-2448-002`, `ST-CHART-CONSOLE-LIVE-002` | Assert rendered and live Deployment arguments. |
| R2 | Existing installations gain an unexpected provider logout default. | Provider-specific logout requests could be sent to the wrong IdP. | `IT-HELM-2448-001` | Keep the default empty and omit the flag. |
| R3 | A malformed endpoint causes unsafe or non-deterministic logout behavior. | Failed logout or SSRF-like provider misconfiguration. | `IT-HELM-2448-003` | Fail closed during Helm rendering; do not discover endpoints during templating. |
| R4 | The ID token or redirect target is moved into the SPA. | Credential exposure or open redirect risk. | `IT-HELM-2448-002`, source review, Console E2E | Keep the value and substitution in OAuth2 Proxy only; add no client-side token code or user redirect value. |
| R5 | Helm single-render tests pass while upgrade persistence regresses. | Reconciliation removes the working logout argument. | `ST-CHART-CONSOLE-LIVE-002` | Exercise real Helm release history and inspect the live Deployment after upgrade. |

---

## 4. Scope

### In scope

- `console.oauth2Proxy.backendLogoutURL` schema and documentation.
- Conditional `--backend-logout-url` rendering in the Console Deployment.
- Generated Helm defaults and reference documentation.
- Helm render tests and live Helm upgrade persistence coverage.
- Cross-repository execution of the existing Console live logout scenario.

### Out of scope

- Operator CRD/reconciliation changes, tracked separately by `kubernaut-operator#483`.
- Console SPA identity or transcript changes, owned by `kubernaut-console#139`.
- Provider endpoint discovery or provider-specific API behavior inside the chart.
- Adding a universal Keycloak or Dex default.

---

## 5. Pyramid Strategy

This is a Helm-only change, so the pyramid uses the chart's established tiers:

| Tier | Purpose | Evidence |
|---|---|---|
| Render | Prove template logic and fail-closed configuration behavior. | `helm-unittest` scenarios `IT-HELM-2448-001..003`. |
| Live wiring | Prove Helm upgrade/reconciliation updates the production Deployment. | `ST-CHART-CONSOLE-LIVE-002` in `scripts/helm-smoke-test.sh`. |
| Browser journey | Prove provider session termination and fresh-login behavior. | Existing live Console logout test in `kubernaut-console/e2e/live/logout.spec.ts`. |

No Go unit test is required because no Go business logic changes. The render tier
proves chart logic, the live tier proves production wiring, and the browser tier
proves the end-user journey; no tier substitutes for another.

---

## 6. Test Cases

| Test ID | Scenario | Expected result | Controls |
|---|---|---|---|
| `IT-HELM-2448-001` | Value unset | Existing Console Deployment has no backend logout argument. | AC-6, V3.3 |
| `IT-HELM-2448-002` | Keycloak-style endpoint with `{id_token}` | OAuth2 Proxy receives the exact configured URL; Console container receives no token configuration. | AC-2/AC-6, CC6.1, V5/V8 |
| `IT-HELM-2448-003` | Malformed endpoint | Helm render fails before resources are applied. | AC-6, CC6.1, V5 |
| `ST-CHART-CONSOLE-LIVE-002` | Configure URL, upgrade release, upgrade again with `--reuse-values` | Live Deployment contains the argument after both upgrades and rollout. | AC-2/AC-12, CC6.1/CC6.3, V3.3/V8 |
| Existing Console live logout | Authenticated logout against Keycloak | IdP login state is reached, fresh login is required, and prior transcript is absent. | AC-12, CC6.1/CC6.3, V3.3/V7.2.1 |

---

## 7. Wiring Manifest

| Component | Production entry point | Wiring location | Proof |
|---|---|---|---|
| Provider logout URL | Console OAuth2 Proxy sidecar | `charts/kubernaut/templates/console/console.yaml` | `IT-HELM-2448-001/002`, `ST-CHART-CONSOLE-LIVE-002` |

Checkpoint W passes only when the configured value is visible in the live,
Helm-managed `console` Deployment after upgrade.

---

## 8. TDD Execution Phases

| Phase | Work | Required result |
|---|---|---|
| RED | Add this plan and `console_oauth2proxy_logout_test.yaml`; run the focused suite. | Configured and malformed cases fail against the current chart. |
| GREEN | Add schema, template conditional, generated artifacts, docs, and live smoke assertion. | All new render and live wiring cases pass. |
| REFACTOR | Review validation, argument ordering, documentation, generated-file drift, and security boundaries. | Behavior remains green; no new abstraction or client-side token path. |

---

## 9. Verification

```bash
helm unittest charts/kubernaut/
helm lint charts/kubernaut/
make generate-helm-config-docs
make generate-helm-defaults
make check-helm-coverage
make verify-helm-defaults-parity
go build ./...
golangci-lint run --timeout=5m
git diff --check
```

Live validation requires the repository's Kind smoke environment and the
Console repository's Keycloak-backed logout test.
