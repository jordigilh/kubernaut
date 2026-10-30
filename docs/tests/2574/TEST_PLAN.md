# Test Plan: Operator Demo Application Fixture Alignment

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2574-v1  
**Feature**: Consume the PostgreSQL/Valkey-only operator dependency chart, provision application-owned fixtures separately, and qualify real Console login through an authorized APIFrontend read.  
**Version**: 1.1  
**Created**: 2026-10-29  
**Author**: Kubernaut Platform  
**Status**: Active — consumer adoption implementation  
**Branch**: `issue-2574-triage`

---

## 1. Introduction

### 1.1 Purpose

Issue #2574 required adoption of the published PostgreSQL/Valkey-only dependency
chart without restoring chart ownership of application fixtures. The authoritative
`kubernaut-demo` Keycloak realm defines the browser client as `kubernaut-console`
with secret `e2e-console-secret`; the demo/infrastructure owner must therefore
provision `kubernaut-console-oidc` separately, alongside the LLM credential and
policy ConfigMaps. This plan proves the fixture ownership boundary and then proves
the full browser login, callback, token forwarding, and authorized APIFrontend read
without starting an investigation or remediation.

### 1.2 Approved approach and decision boundary

The upstream chart ownership correction is complete. The published immutable
artifact is:

```text
oci://quay.io/kubernaut-ai/kubernaut-development-dependencies:0.1.1-run.4
  digest: sha256:53a3155522cab4d743d304badab45863d2b099edbcc30e7a48861a2f5c9bcb2d
  source: 7b1b6b10e7fb4521120e59609e5f79adef5a108d
  publication workflow: 41049051350 (succeeded)
  appVersion: 7b1b6b10
```

The chart owns only disposable PostgreSQL/Valkey resources and their supporting
connection/TLS material. Kubernaut provisions LLM credentials, policy ConfigMaps,
and the Console OIDC Secret separately through demo/infrastructure helpers, using
stdin-applied manifests and application ownership labels. This preserves the CR
contract, keeps credentials out of Helm arguments, and does not restore the retired
application Helm deployment path. No temporary values-file override is permitted.

### 1.3 Objectives

1. **Fixture correctness**: A fresh operator demo creates application-owned
   `kubernaut-console-oidc` with the realm's `kubernaut-console` client and
   `e2e-console-secret` secret, while retaining the existing cookie secret; it
   also creates the LLM and policy fixtures before CR reconciliation.
2. **CR and security invariants**: The authored CR still owns only deployment
   configuration: issuer, JWKS, audience, HTTPS callback, TLS, and Secret name.
   Client credentials remain in a demo-owned Kubernetes Secret and are consumed
   server-side by oauth2-proxy.
3. **Real browser authentication**: A browser follows the real Keycloak
   authorization-code flow, submits the demo user login form, reaches
   `/oauth2/callback`, and renders the authenticated Console identity.
4. **Authorized AF read**: The authenticated browser session invokes the real
   Console `/mcp` path and receives an allowed `kubectl_get` result from
   APIFrontend using the `sre` group/SAR binding.
5. **Safety and regression**: The journey preserves TLS verification, audience,
   groups, access-token forwarding, and authorization, and creates no
   RemediationRequest, InvestigationSession, or WorkflowExecution.

### 1.4 Success metrics

| Metric | Target | Measurement |
|---|---:|---|
| Critical unit/static scenarios | 100% | Focused Ginkgo infrastructure suite |
| Dependency-chart render contract | 100% | Upstream Helm/chart test |
| Wiring manifest rows | 100% | Live operator qualification evidence |
| Browser login/callback | Pass | Playwright against operator-managed Console |
| Authorized AF read | Pass | Same browser session, same-origin `/mcp`, `kubectl_get` |
| Credential exposure | 0 occurrences | Helm argv/log/evidence inspection; secret values redacted |
| Regression | 0 new failures | `go build ./...`, focused tests, lint, operator E2E |

---

## 2. References

### 2.1 Authority

- [Issue #2574](https://github.com/jordigilh/kubernaut/issues/2574): Operator demo bootstrap wires Console to an OIDC client missing from the demo realm.
- [BR-PLATFORM-006](../../requirements/BR-PLATFORM-006-helm-chart-console-parity.md): Console functional contract; supported deployment ownership is now the operator.
- [BR-PLATFORM-014](../../requirements/BR-PLATFORM-014-demo-environment-cert-lifecycle.md): Demo TLS/state lifecycle and stable demo trust behavior.
- [BR-SECURITY-016](../../requirements/BR-SECURITY-016-kubernetes-rbac-enforcement.md): Authenticated AF tool calls require Kubernetes RBAC/SAR authorization.
- [BR-SECURITY-017](../../requirements/BR-SECURITY-017-serviceaccount-token-authentication.md): Authentication precedes authorization; its composite-auth section covers external OIDC JWT routing.
- [DD-AUTH-010](../../architecture/decisions/DD-AUTH-010-e2e-real-authentication-mandate.md): E2E tests must use production-equivalent authentication and real RBAC.
- [Issue #2492 test plan](../2492/TEST_PLAN.md): Operator-only bootstrap, exact artifact pinning, CR minimality, and wiring evidence.
- [Issue #2566 test plan](../2566/TEST_PLAN.md): Local demo OIDC TLS trust-root alignment.
- [Interactive Mode Setup Guide](../../operations/deployment/INTERACTIVE_MODE_SETUP_GUIDE.md): Console → oauth2-proxy → APIFrontend flow and audience requirement.

### 2.2 Source contracts

- `test/infrastructure/keycloak-realm-demo.json`: authoritative demo client,
  callback, `groups` claim, `sre` group, and `sre-user` fixture.
- `test/infrastructure/operator_deployment.go`: pinned operator/dependency
  artifacts, dependency installation boundary, and application fixture manifests.
- `test/infrastructure/demo_operator.go`: operator CR construction and demo
  Console callback/issuer/audience wiring.
- `test/infrastructure/oidc_helm.go`: demo Console OIDC Secret provisioning.
- `test/infrastructure/fullpipeline_e2e_helm.go`: AF persona ClusterRole
  binding and `sre/kubectl_get` SAR verification.
- `kubernaut-operator` source `7b1b6b10e7fb4521120e59609e5f79adef5a108d`:
  published `charts/kubernaut-development-dependencies` ownership correction.
- `kubernaut-console` live E2E:
  `e2e/live/local-auth.setup.ts` and `e2e/live/helpers.ts` provide the existing
  browser authorization-code login and SSE/MCP response conventions.

### 2.3 Control note

This change adds no production audit event type and does not alter the AF
authorization implementation. Existing authentication/authorization decisions
remain observable through the existing AF security-event path. The qualification
must nevertheless prove the applicable `IA-2`, `AC-3`, `AC-6`, and `SC-8`
behaviors through a real browser token and real SAR-backed tool call.

---

## 3. Risks and mitigations

| ID | Risk | Impact | Probability | Mitigating scenarios | Mitigation |
|---|---|---|---|---|---|
| R1 | Application fixtures are accidentally passed back to the dependency chart or omitted before CR reconciliation. | Credentials/policies are missing or ownership drifts. | High | `UT-INFRA-2574-003`, `UT-INFRA-2574-004`, `IT-INFRA-2574-001` | Keep dependency Helm arguments PostgreSQL/Valkey-only and apply explicit fixture manifests before CR creation. |
| R2 | A mutable or wrong chart artifact is pinned. | Reproducibility and qualification provenance are lost. | Medium | `UT-INFRA-2574-001`, `IT-INFRA-2574-001` | Require an immutable `@sha256` reference, exact version, source commit, and no `latest`. |
| R3 | A Console secret is exposed through Helm argv, logs, or evidence. | Credential disclosure and unsafe fixture handling. | Medium | `UT-INFRA-2574-003`, `UT-INFRA-2574-004`, `IT-INFRA-2574-001` | Apply the Secret through stdin; use no auth `--set` flags; redact all live evidence. |
| R4 | The callback succeeds only because TLS/audience checks were weakened. | The test gives false confidence and masks a production failure. | Medium | `UT-INFRA-2574-002`, `IT-CONSOLE-2574-001`, `E2E-CONSOLE-2574-001` | Assert provider CA/system trust, `pass-access-token`, audience, groups, and absence of insecure OIDC flags; use a real authorization-code browser flow. |
| R5 | The browser test uses a direct token or bypasses Console, so AF authorization is not proven. | Login/forwarding/SAR regressions remain undetected. | Medium | `E2E-CONSOLE-2574-001`, `E2E-CONSOLE-2574-002` | Do not use password grant or injected bearer tokens for the Console journey; call same-origin `/mcp` after browser callback and assert an allowed `kubectl_get`. |

### 3.1 Risk-to-test traceability

All five risks have a P0 or P1 proving scenario. No risk is accepted without a
live or artifact-level assertion. If the pinned chart artifact is unavailable,
testing is suspended rather than switching to a mutable or temporary override.

---

## 4. Scope

### 4.1 Features to be tested

- Immutable PostgreSQL/Valkey-only dependency-chart release and Kubernaut pinning.
- Kubernaut dependency installation arguments and separate application fixtures.
- Demo CR issuer, JWKS, audience, callback, Console Secret name, and minimality.
- Live demo-owned `kubernaut-console-oidc`, LLM Secret, and policy ConfigMap contents
  and ownership.
- Operator-rendered Console oauth2-proxy and nginx security/token-forwarding
  contract.
- Real Keycloak authorization-code login and `/oauth2/callback` completion.
- Real same-origin Console `/mcp` call to AF's authorized `kubectl_get` tool.
- AF `sre` group binding and `kubectl_get` SAR verification.

### 4.2 Features not to be tested

- New Kubernaut CR authentication fields or IdP/client-management APIs.
- Application Helm chart deployment or restoration of the deprecated Helm path.
- Console UI redesign, investigation/remediation behavior, or workflow execution.
- Direct-grant token tests as a substitute for the browser callback journey.
- New AF authorization logic, new RBAC personas, or a new authorization model.
- Production credential rotation; the chart fixture remains disposable and
  synthetic.

### 4.3 Design decisions

| Decision | Rationale |
|---|---|
| Keep the dependency chart narrow | The chart owns only PostgreSQL/Valkey support; application fixtures are applied separately and the CR remains deployment authority only. |
| Treat the realm JSON as the fixture authority | It already defines the browser client, exact redirect URI, `groups` claim, `sre` group, and test user. |
| Keep the dependency chart pinned by immutable digest | The operator demo must be reproducible and must not silently consume a changed fixture. |
| Keep credentials server-side | Console OIDC values are applied as Secret `stringData` through stdin; no client secret is a CR field or Helm argument. |
| Use Playwright for callback completion | Existing Console E2E already exercises the real browser authorization-code flow; an HTTP/direct-grant helper would not prove browser callback wiring. |
| Prove AF access with a read-only tool | `kubectl_get` is already bound and SAR-verified for `sre`; it proves forwarding and authorization without creating an investigation or remediation. |

---

## 5. Approach

### 5.1 TDD sequence

1. **RED — upstream chart**: The upstream chart ownership/render contract was
   corrected and released; its negative assertions prove that application
   fixtures are absent.
2. **RED — Kubernaut contracts**: Add/update focused static assertions for the
   immutable dependency pin, fixture ownership, and unchanged CR/TLS/audience/
   callback contract.
3. **RED — live wiring**: Add the live application Secret/ConfigMap ownership and
   operator Console deployment checks; retain the cross-repository browser
   login/read scenario.
4. **GREEN — provisioning**: Repin Kubernaut and apply LLM, policy, and Console
   fixtures separately before the CR is reconciled. Do not add a CR field or a
   general auth override.
5. **REFACTOR**: Remove obsolete fixture Helm plumbing and keep manifest/error
   handling explicit. Do not create a new credential or authentication abstraction.
6. **WIRING VERIFICATION**: Execute every row in Section 12 through the real
   production/qualification entry point.
7. **VALIDATION**: Run the focused tests, build, lint, and fresh operator-backed
   Kind/Playwright qualification. Record artifact digest and redacted evidence.

### 5.2 Coverage policy

- **Unit/static**: at least 90% of defined in-scope scenarios; every P0 contract
  scenario passes.
- **Integration/wiring**: 100% of Wiring Manifest rows pass.
- **Browser E2E**: 100% of the Console login/callback and authorized-read
  acceptance journey passes.
- Structural line coverage is diagnostic only for this infrastructure-focused
  change; it does not replace the scenario and wiring criteria.

### 5.3 Pass/fail criteria

**PASS** — all of the following are true:

1. The upstream chart render test proves that application fixtures, including
   Console OIDC, are absent; the Kubernaut fixture-manifest test proves the
   separate `kubernaut-console` / `e2e-console-secret` Secret.
2. Kubernaut resolves the new dependency chart by exact immutable digest and
   version, with no `latest` or mutable fallback.
3. The live dependency install creates the expected Secret and the operator CR
   reaches its existing positive readiness/TLS conditions.
4. Console oauth2-proxy starts with normal provider certificate verification,
   access-token forwarding, and the configured audience; no insecure issuer
   verification bypass is present.
5. The browser completes the Keycloak login and `/oauth2/callback`, returns to
   `https://kubernaut-console.local:8843`, and shows the `sre-user` identity.
6. The same browser session's `/mcp` `kubectl_get` call is allowed and returns
   the expected non-secret `apifrontend` Deployment object.
7. No new RemediationRequest, InvestigationSession, or WorkflowExecution is
   created by the read-only journey.
8. Focused tests, `go build ./...`, and `golangci-lint run --timeout=5m` pass
   without new failures.

**FAIL** — any P0 scenario fails, a callback/redirect mismatch occurs, the
fixture Secret is wrong, a TLS/audience/access-token/authorization invariant is
weakened, a credential is exposed, or the read-only journey creates a
remediation-side effect.

### 5.4 Suspension and resumption

**Suspend when**:

- The upstream chart fix is not merged/published with a digest.
- Keycloak, cert-manager, the operator, or the browser-facing Console endpoint
  is unavailable.
- The current #2566 trust-root prerequisite is not healthy.
- More than one failure has the same infrastructure root cause and further
  retries would only generate duplicate clusters/evidence.

**Resume when**:

- The candidate chart artifact and source provenance are available.
- A fresh cluster reaches operator/Console readiness with normal TLS.
- The browser runner has the test user's credentials through its secret
  environment, without adding them to source or command-line arguments.

---

## 6. Test items and version identification

### 6.1 Kubernaut repository

| Item | Location | Contract |
|---|---|---|
| Dependency artifact resolution | `test/infrastructure/operator_deployment.go` | Exact operator/dependency references; no mutable `latest`. |
| Dependency Helm argument builder | `test/infrastructure/operator_deployment.go` | Owns PostgreSQL/Valkey settings only; no Console auth values. |
| Demo CR builder | `test/infrastructure/demo_operator.go` | Secret name, issuer, JWKS, audience, HTTPS callback, APIFrontend/Console enablement. |
| Static infrastructure tests | `test/infrastructure/operator_deployment_test.go`, `demo_helm_test.go` | Render and ownership contracts. |
| Live operator qualification | `test/e2e/fleet/` or the operator-backed demo lane | Real Kubernetes Secret, Deployment, SAR, and Console exposure. |

### 6.2 External coordinated items

| Item | Repository/location | Required change |
|---|---|---|
| Development dependency chart | `kubernaut-operator/charts/kubernaut-development-dependencies` | Ownership correction and immutable OCI publication are complete; retain PostgreSQL/Valkey-only render assertions. |
| Browser callback/read test | `kubernaut-console/e2e/live/` | Add/reuse an external-Console Playwright config and a read-only spec using existing `local-auth.setup.ts` conventions. |

### 6.3 Version identification

| Item | Current value | Required final evidence |
|---|---|---|
| Kubernaut source | `ae4fd255c0` / branch `issue-2574-triage` | Final commit under test |
| Operator source | `9dc9394fd1844be4f4d4adb89db9a881dbc066ed` | Pinned operator qualification revision |
| Dependency chart | `0.1.1-run.4`, digest `sha256:53a3155522cab4d743d304badab45863d2b099edbcc30e7a48861a2f5c9bcb2d` | Published from source `7b1b6b10e7fb4521120e59609e5f79adef5a108d`; PostgreSQL/Valkey-only ownership |
| Keycloak realm | `test/infrastructure/keycloak-realm-demo.json` | Client/redirect/group values unchanged and matched by live Secret/test |

---

## 7. BR coverage matrix

| BR / authority | Description | Priority | Tier | Test IDs | Status |
|---|---|---:|---|---|---|
| BR-PLATFORM-006 | Operator-managed Console remains a functional browser-facing OIDC feature. | P0 | Static/E2E | `UT-INFRA-2574-002`, `E2E-CONSOLE-2574-001` | Pending |
| BR-PLATFORM-014 | Demo TLS and browser-facing infrastructure retain the trusted lifecycle. | P0 | IT/E2E | `IT-CONSOLE-2574-001`, `E2E-CONSOLE-2574-001` | Pending |
| BR-SECURITY-016 | An authenticated Console identity can use an AF tool only when its real group/SAR binding allows it. | P0 | IT/E2E | `IT-INFRA-2574-002`, `E2E-CONSOLE-2574-002` | Pending |
| BR-SECURITY-017 / DD-AUTH-010 | Authentication precedes authorization and E2E uses the real OIDC/browser path rather than an injected identity. | P0 | E2E | `E2E-CONSOLE-2574-001`, `E2E-CONSOLE-2574-002` | Pending |
| Operator-only deployment contract | The dependency chart remains bootstrap-only and the CR remains application deployment authority. | P0 | Unit/IT | `UT-INFRA-2574-001`, `IT-INFRA-2574-001` | Pending |

---

## 8. Test scenarios

### Tier 1: Unit/static and chart tests

| ID | Business outcome under test | Phase |
|---|---|---|
| `HT-OP-2574-001` | The upstream dependency chart renders PostgreSQL/Valkey resources only and does not render application credentials, policies, notification fixtures, or Console OIDC. | Verified in published chart source/release |
| `UT-INFRA-2574-001` | Demo artifact resolution uses the new immutable dependency chart reference/version and never silently falls back to `latest`. | Passing |
| `UT-INFRA-2574-002` | Demo CR keeps the existing issuer, JWKS, `kubernaut-apifrontend` audience, HTTPS callback, `kubernaut-console-oidc` Secret name, and minimal operator contract; it contains no client credential fields. | Passing |
| `UT-INFRA-2574-003` | Dependency Helm arguments continue to own PostgreSQL and Valkey only; no Console client secret or fixture value is placed in `--set`, `--set-string`, or rendered command output. | Passing |
| `UT-INFRA-2574-004` | LLM, policy, and Console fixtures render as explicitly application-owned resources with `setup-demo-infra` and `kubernaut.ai/e2e-owner: upstream-2492` labels. | Passing |

### Tier 2: Integration and wiring tests

| ID | Business outcome under test | Phase |
|---|---|---|
| `IT-INFRA-2574-001` | A fresh operator demo bootstrap creates the application-owned `kubernaut-console-oidc` Secret, LLM Secret, and policy ConfigMaps separately from the dependency chart, with realm values, expected keys, owner labels, and no stale `e2e-client` value. | Pending |
| `IT-CONSOLE-2574-001` | The reconciled Console Deployment retains provider CA/system trust, `pass-access-token`, `skip-jwt-bearer-tokens`, and the expected HTTPS exposure; insecure OIDC verification is absent. | Pending |
| `IT-INFRA-2574-002` | `BindAFPersonaRBAC` plus the operator-managed AF ClusterRoles produce an allowed `sre`/`kubectl_get` SubjectAccessReview before browser qualification. | Pending |

### Tier 3: Browser E2E tests

| ID | Business outcome under test | Phase |
|---|---|---|
| `E2E-CONSOLE-2574-001` | A real browser follows Keycloak authorization-code login, submits the test user's credentials, completes `/oauth2/callback`, returns to the configured Console origin, and renders `sre-user`. | Pending |
| `E2E-CONSOLE-2574-002` | From the authenticated browser session, a same-origin `/mcp` JSON-RPC `tools/call` for `kubectl_get` reads the `kubernaut-system/apifrontend` Deployment and is allowed by AF's real JWT/group/SAR path. | Pending |
| `E2E-CONSOLE-2574-003` | The read-only Console/MCP journey produces no new RemediationRequest, InvestigationSession, or WorkflowExecution and does not invoke an investigation/remediation tool. | Pending |

### Tier skip rationale

No tier is skipped. The upstream chart render test catches the fixture regression
before cluster startup, integration tests prove operator wiring and Secret
ownership, and browser E2E proves the real authorization-code callback and AF
authorization boundary.

---

## 9. Test cases

### HT-OP-2574-001 — Dependency chart ownership boundary

**BR**: Operator-only deployment contract; BR-PLATFORM-006  
**Priority**: P0  
**Type**: Upstream Helm/chart test  
**File**: `kubernaut-operator` chart test/verification location

**Given** the published development dependency chart with default values  
**When** Helm renders the chart  
**Then** it contains only disposable PostgreSQL/Valkey resources and their
supporting connection/TLS material.  
**And** it does not render `kubernaut-console-oidc`, LLM credentials, policy
ConfigMaps, notification fixtures, a Kubernaut CR, or an application release.

### UT-INFRA-2574-002 — Demo CR security contract

**BR**: BR-PLATFORM-006, BR-PLATFORM-014  
**Priority**: P0  
**Type**: Ginkgo/static contract  
**File**: `test/infrastructure/demo_helm_test.go`, `operator_deployment_test.go`

**Given** local demo options with Console enabled  
**When** `BuildKubernautCRManifest` renders the authored CR  
**Then** it contains the existing Keycloak issuer/JWKS, audience
`kubernaut-apifrontend`, Secret name `kubernaut-console-oidc`, and callback
`https://kubernaut-console.local:8843/oauth2/callback`.  
**And** it contains no client ID, client secret, IdP-management field, or
deprecated application Helm configuration.

### UT-INFRA-2574-004 — Application fixture ownership

**BR**: Operator-only deployment contract; BR-PLATFORM-014  
**Priority**: P0  
**Type**: Ginkgo/static manifest contract  
**File**: `test/infrastructure/operator_deployment_test.go`

**Given** LLM credentials, policy files, and the demo realm client values  
**When** the application fixture builders render their manifests  
**Then** the Secret/ConfigMaps use `setup-demo-infra` management and
`kubernaut.ai/e2e-owner: upstream-2492` ownership labels.  
**And** the Console Secret uses `stringData` and is not represented in dependency
Helm arguments.

### IT-INFRA-2574-001 — Bootstrap Secret ownership and values

**BR**: BR-PLATFORM-014; operator-only deployment contract  
**Priority**: P0  
**Type**: Real Kind/Helm/operator integration  
**Entry point**: `InstallDemoOperatorApplication` → `InstallOperatorDependencies`

**Given** a fresh disposable Kind cluster and the corrected immutable dependency
chart  
**When** dependencies are installed before the operator CR is applied  
**Then** `kubernaut-console-oidc` exists in `kubernaut-system`, has application
fixture ownership labels, and has the three expected keys.  
**And** decoded values match the realm client without writing those values to
stdout, Helm arguments, or test artifacts.  
**And** PostgreSQL/Valkey remain dependency-chart resources while the CR remains
the application deployment authority.

### IT-CONSOLE-2574-001 — Operator Console security/render contract

**BR**: BR-PLATFORM-014, BR-SECURITY-017 / DD-AUTH-010  
**Priority**: P0  
**Type**: Real Kubernetes manifest/Pod inspection

**Given** the operator has reconciled the Console  
**When** the live Deployment and nginx ConfigMap are inspected  
**Then** oauth2-proxy reads `client-id`, `client-secret`, and `cookie-secret`
from `kubernaut-console-oidc`; provider CA/system trust and access-token
forwarding are enabled; the configured audience remains `kubernaut-apifrontend`;
and the nginx upstream Authorization header is sourced from the oauth2-proxy
forwarded access token.  
**And** no insecure issuer-verification bypass is present.

### IT-INFRA-2574-002 — AF persona authorization preflight

**BR**: BR-SECURITY-016  
**Priority**: P0  
**Type**: Real Kubernetes SAR integration  
**Entry point**: `BindAFPersonaRBAC`

**Given** the operator-managed AF persona ClusterRoles exist  
**When** the test-owned `sre` group binding is applied and the preflight issues
the existing SAR  
**Then** `use kubernaut.ai/tools/kubectl_get` is allowed for user `sre-user` in
group `sre`.  
**And** the test does not create a new Console-access authorization model.

### E2E-CONSOLE-2574-001 — Real browser login and callback

**BR**: BR-PLATFORM-006, BR-PLATFORM-014, DD-AUTH-010  
**Priority**: P0  
**Type**: Browser E2E with Playwright against the operator-managed Console  
**Entry point**: `https://kubernaut-console.local:8843/`

**Given** the fresh operator demo, Keycloak realm, browser-facing Console TLS,
and credentials supplied only through the test runner's protected environment  
**When** a browser opens the Console, follows the real Keycloak redirect,
submits the login form, and follows the authorization-code callback  
**Then** the final origin is the configured Console origin, the callback does
not return `invalid_client`, `redirect_uri` mismatch, `invalid_scope`, or 5xx,
and the authenticated Console identity is `sre-user`.  
**And** the test does not use a direct-grant token, injected bearer token, or
client secret in browser JavaScript.  
**TLS note**: the server-side assertion requires normal oauth2-proxy provider
certificate verification. If the local browser edge uses a private demo CA, the
browser harness must trust that CA or clearly isolate any edge-only test
certificate accommodation; it must not weaken the provider verification flags.

### E2E-CONSOLE-2574-002 — Authorized AF read through Console

**BR**: BR-SECURITY-016, BR-SECURITY-017 / DD-AUTH-010  
**Priority**: P0  
**Type**: Browser E2E, same authenticated context

**Given** `E2E-CONSOLE-2574-001` completed successfully  
**When** the browser posts MCP `initialize`, `notifications/initialized`, and
`tools/call` to the Console's same-origin `/mcp` endpoint for:

```json
{
  "name": "kubectl_get",
  "arguments": {
    "kind": "Deployment",
    "name": "apifrontend",
    "namespace": "kubernaut-system"
  }
}
```

**Then** the response is HTTP 200, parses the Streamable HTTP/SSE `data:` frame,
has no JSON-RPC/tool error or `permission denied`, and returns the expected
non-secret Deployment identity. This proves the Console cookie → oauth2-proxy
forwarded access token → nginx Authorization header → AF JWT/group mapping →
real SAR → `kubectl_get` chain.

### E2E-CONSOLE-2574-003 — Read-only boundary

**BR**: BR-SECURITY-016; operator qualification safety  
**Priority**: P0  
**Type**: Browser E2E plus Kubernetes readback

**Given** the isolated read-only qualification lane  
**When** the browser performs only the `kubectl_get` MCP call above  
**Then** no new RemediationRequest, InvestigationSession, or WorkflowExecution
appears during the test, and no `kubernaut_investigate`, `af_create_rr`,
`kubernaut_remediate`, workflow-selection, or approval tool call is observed.

---

## 10. Environmental needs

### 10.1 Unit/static tests

- Go and Ginkgo/Gomega.
- No external-service mocks are needed for pure render/argument assertions.
- Tests must not print or compare secret values in failure messages.

### 10.2 Integration tests

- Fresh disposable Kind cluster with Podman/Docker, Helm, cert-manager, and the
  pinned operator/dependency artifacts.
- Real Keycloak deployment using the embedded demo realm.
- Real operator-managed PostgreSQL, Valkey, APIFrontend, and Console.
- Real Kubernetes client/SAR calls; no mock auth or fake RBAC for the proving
  path.

### 10.3 Browser E2E

- Chromium/Playwright from the existing `kubernaut-console` live E2E tooling.
- Browser-facing host mapping for `kubernaut-console.local` and Keycloak.
- Test credentials supplied through protected environment/secret injection.
- Browser trust for the demo Console edge certificate, without changing
  oauth2-proxy's provider CA verification.
- The live browser test runs serially against an isolated cluster and preserves
  screenshots/traces only after redaction review.

---

## 11. Dependencies and execution order

### 11.1 Blocking dependencies

| Dependency | Type | Status | Impact | Workaround |
|---|---|---|---|---|
| `kubernaut-operator` chart fixture correction | External code/release | Complete | `HT-OP-2574-001` is satisfied by the published ownership-corrected chart | Keep the consumer pinned to the released digest. |
| Immutable dependency-chart digest | Artifact | Complete | Consumer can be repinned reproducibly | Reject mutable `latest` references. |
| #2566 trust-root alignment | Existing demo prerequisite | Expected prerequisite | Browser/OIDC discovery can fail for an unrelated CA mismatch | Fix/qualify #2566 first; do not add an insecure OIDC bypass. |
| Browser test runner against operator URL | Cross-repository test | Open | Real callback/read proof is unavailable | Coordinate a small Console live-E2E config/spec change. |

### 11.2 Execution order

1. **Phase 1 (RED)**: Add the Kubernaut static contracts for the immutable pin,
   PostgreSQL/Valkey-only Helm arguments, and application fixture ownership.
2. **Phase 2 (GREEN)**: Consume the published digest and provision LLM, policy,
   and Console fixtures before CR reconciliation.
3. **Phase 3 (REFACTOR)**: Remove obsolete fixture Helm plumbing and keep error
   handling/ownership labels explicit.
4. **Phase 4 (WIRING, ~30 minutes)**: Run bootstrap → CR reconciliation → Secret,
   Deployment, exposure, and SAR checks.
5. **Phase 5 (browser E2E, ~30–60 minutes)**: Run real login/callback and the
   same-session authorized AF read on a fresh isolated cluster.
6. **Phase 6 (validation, ~15–30 minutes)**: Focused tests, build, lint, and
   evidence/provenance review.

---

## 12. Wiring verification manifest

| Component | Production/qualification entry point | Wiring location | Proving test | Status |
|---|---|---|---|---|
| Console OIDC fixture values | `SetupOIDCInfrastructure` | `ensureDemoConsoleOIDCSecret` → stdin-applied Secret | `IT-INFRA-2574-001`, `E2E-CONSOLE-2574-001` | Pending |
| LLM and policy fixtures | `InstallDemoOperatorApplication` | `InstallOperatorDependencies` → `ensureOperatorApplicationFixtures` | `IT-INFRA-2574-001` | Pending |
| Operator CR Console contract | Demo setup | `buildDemoOperatorCROptions` → `BuildKubernautCRManifest` → operator reconcile | `UT-INFRA-2574-002`, `IT-CONSOLE-2574-001` | Pending |
| Console oauth2-proxy callback | Browser Console URL | operator Console Deployment → Keycloak → `/oauth2/callback` | `E2E-CONSOLE-2574-001` | Pending |
| Access-token forwarding | Browser `/mcp` request | oauth2-proxy → Console nginx → APIFrontend | `E2E-CONSOLE-2574-002` | Pending |
| AF persona authorization | Post-reconciliation setup | `BindAFPersonaRBAC` → `sre` GroupBinding → AF SAR | `IT-INFRA-2574-002`, `E2E-CONSOLE-2574-002` | Pending |
| Read-only journey boundary | Authenticated Console session | MCP `kubectl_get` only; no remediation tools | `E2E-CONSOLE-2574-003` | Pending |

**Checkpoint W requirement**: every row must have a production/qualification
caller and a passing integration/E2E test. Unit tests alone do not count as
wiring proof.

---

## 13. Existing tests requiring updates

| Location | Current assertion | Required change | Reason |
|---|---|---|---|
| `test/infrastructure/operator_deployment_test.go` | Dependency pin and fixture ownership contracts. | Keep the released immutable digest and assert no fixture Helm values plus explicit application labels. | Prevent chart ownership regression. |
| `test/infrastructure/demo_helm_test.go` | Local demo asserts Secret name, issuer, JWKS, and callback. | Keep all CR/TLS/audience/callback assertions. | The CR remains the deployment contract, not the credential source. |
| `docs/tests/2492/TEST_PLAN.md` | Version table named the old dependency chart release. | Update the provenance note to `0.1.1-run.4` and document separate fixtures. | Keep the operator qualification record accurate. |
| `kubernaut-operator` `origin/main` E2E consumers | Previous fixture setup/ownership assertions included application fixtures with the dependency witness set. | Verified `ensureKubernautApplicationFixtures` is separate and ownership witnesses cover PostgreSQL/Valkey only. | Preserve upstream consumer adoption without modifying a dirty unrelated checkout. |
| `kubernaut-console/e2e/live` | Existing auth setup targets the Console team's live harness configuration. | Reuse its browser login helper with an operator-URL config and add the read-only MCP assertion. | The Kubernaut repository has no browser automation; the external suite is the real callback authority. |

---

## 14. Test deliverables and evidence

| Deliverable | Location | Evidence |
|---|---|---|
| This plan | `docs/tests/2574/TEST_PLAN.md` | Consumer-adoption plan and release provenance |
| Upstream chart fix | `kubernaut-operator` | Source `7b1b6b10e7fb4521120e59609e5f79adef5a108d`, version `0.1.1-run.4`, digest `sha256:53a3155522cab4d...` |
| Kubernaut pin/test changes | `test/infrastructure/` and `test/e2e/` | Focused Ginkgo output and diff |
| Browser qualification | `kubernaut-console/e2e/live/` or coordinated runner | Redacted Playwright report showing callback and AF read |
| Live infrastructure evidence | CI artifact or approved private evidence path | Secret key names/ownership only; no client-secret value, token, cookie, or credential dump |
| Verification report | Issue/PR comment | Test IDs, artifact provenance, build/lint results, and any unrelated baseline failures |

---

## 15. Execution commands

```bash
# Kubernaut focused infrastructure contracts
go test ./test/infrastructure -run TestInfrastructure -ginkgo.focus='2574|operator-backed demo|dependency'

# Required repository validation
go build ./...
golangci-lint run --timeout=5m

# Operator-backed live lane (fresh isolated cluster)
FLEET_E2E=true PRESERVE_E2E_CLUSTER=true \
  go test ./test/e2e/fleet -ginkgo.focus='2574'

# Coordinated Console browser lane (credentials are environment-injected;
# do not place them in this command line or source files)
LIVE_E2E_CONSOLE_URL=https://kubernaut-console.local:8843 \
  pnpm exec playwright test --config=playwright.operator-live.config.ts \
  e2e/live/operator-oidc-af-read.spec.ts
```

---

## 16. Approval and scope guard

The upstream chart release and consumer-adoption direction are approved for this
issue. No CR schema, IdP API, application Helm path, or hard-coded general operator
client contract may be added. Any browser-runner changes remain coordinated with
the external `kubernaut-console` repository.

---

## 17. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.1 | 2026-10-29 | Updated for the published PostgreSQL/Valkey-only chart; application-owned LLM, policy, and Console OIDC fixture provisioning is now the Kubernaut consumer contract. |
| 1.0 | 2026-10-29 | Initial draft; defined bootstrap, callback, authorized-read, and wiring coverage. |
