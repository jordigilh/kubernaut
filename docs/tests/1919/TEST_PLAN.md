# Test Plan: Coarse-Grained Console Access Authorization Gate (AF Backend) — `release/v1.5` Backport

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1919
**Feature**: A new `kubernaut.ai/console` `use` SubjectAccessReview grant, checked
(a) via a new lightweight `GET /a2a/access` pre-flight endpoint for UX purposes,
and (b) — the actual security fix — enforced server-side inside both existing
tool-invocation paths (`checkRBAC` for `/mcp`, `newRBACGuard` for `/a2a/invoke`)
so console access is a real security boundary, not merely a UI-advisory check
a non-conforming client could bypass.
**Version**: 1.0
**Created**: 2026-08-05
**Author**: AI Agent
**Status**: Complete
**Branch**: `fix/1941-console-access-gate-v1.5`

---

## 0. Backport Provenance

This is a `release/v1.5` backport of the fix originally implemented against
`main` for issue [#1919](https://github.com/jordigilh/kubernaut/issues/1919)
(merged via [PR #1940](https://github.com/jordigilh/kubernaut/pull/1940)),
tracked by backport issue
[#1941](https://github.com/jordigilh/kubernaut/issues/1941). `main` and
`release/v1.5` had diverged significantly in the affected files (`mcp_bridge.go`,
`root.go`, `apifrontend.yaml`, `main.go` all carry unrelated `main`-only
refactors from later issues — #1366, #1496, #1677, #1718, #1737, #1790,
#1801, #1827, #1923, DD-PLATFORM-006/008), so this was a manual, surgical
re-application of the #1919 diff onto `release/v1.5`'s actual code shape, not
a cherry-pick. Two notable adaptations from the original `main` implementation:

1. **No `buildRouterConfig`/`routerBuildParams` extraction on `release/v1.5`**:
   `main`'s `TestBuildRouterConfig_ConsoleAccessHandler_*` wiring unit tests
   target a function-extraction refactor that predates #1919 on `main` but
   does not exist on `release/v1.5` (router construction is inline in `run()`
   here). Porting those two tests verbatim was not possible without
   introducing an out-of-scope refactor. Wiring is instead proven by (a) grep
   evidence that `cmd/apifrontend/main.go` constructs
   `handler.NewConsoleAccessHandler(sarChecker, ...)` and passes it to
   `RouterConfig.ConsoleAccessHandler`, and (b) the IT-AF-1919-001..005 tests
   in `console_access_test.go`, which exercise the real
   `handler.NewRouter` dispatch path end-to-end.
2. **Pre-existing chart label duplication bug fixed as a prerequisite**: the
   `kubernaut-tool-{{ $persona }}` ClusterRole template on `release/v1.5` set
   `app.kubernetes.io/part-of: kubernaut` both explicitly and via the
   `kubernaut.labels` helper (which already emits it), producing a duplicate
   YAML mapping key. `helm template`'s plain-text renderer never caught this,
   but the `helm-unittest` plugin's strict YAML decode does, and it blocked
   every test targeting `templates/apifrontend/apifrontend.yaml` — including
   the new `console_access_rbac_test.yaml` added here. Fixed by removing the
   redundant explicit label (no behavior change; `kubernaut.labels` already
   provides it).
3. **#1934 (mcpclient connect timeout) is explicitly out of scope**: `pkg/fleet`
   (and the `mcpclient` resilience code #1934 fixed) does not exist on
   `release/v1.5` — it was introduced on `main` after `release/v1.5` branched.
   See #1941 for the confirmation.

Everything else below mirrors the original `main` test plan's intent,
re-verified against `release/v1.5`'s actual code.

---

## 1. Introduction

### 1.1 Purpose

Issue #1919 proposes gating console UI rendering on a dedicated coarse-grained
authorization role, separate from per-tool RBAC, so operators can grant
"console access" and "tool access" as two explicit, separately-auditable
steps. The issue's own 3-part proposal spans a new AF endpoint, RBAC
convention/manifests, and `kubernaut-console` frontend gating
(`jordigilh/kubernaut-console#48`, a different repo — explicitly out of scope
here).

During the original `main` implementation's planning, a critical architectural
gap was identified and resolved before implementation began: an advisory-only
`GET /a2a/access` endpoint does not close the loop — a client that never calls
it (curl, a script, a non-conforming frontend build) could invoke `/mcp` or
`/a2a/invoke` directly and only ever be checked against `kubernaut.ai/tools`,
never `kubernaut.ai/console`. This defeats the issue's own "two explicit,
separately-auditable steps" premise for any non-browser client. The design was
therefore extended to also enforce `kubernaut.ai/console` `use` at both
pre-existing tool-invocation enforcement points, `checkRBAC` and
`newRBACGuard`, making the new endpoint a UX convenience layered on top of a
real, unbypassable server-side gate. This backport carries that same design
onto `release/v1.5`.

### 1.2 Objectives

1. **New `SARChecker` capability**: `CheckConsoleAccess(ctx, user, groups)`
   checks `kubernaut.ai/console` `use` (a coarse-grained, unnamed resource,
   distinct from the existing per-tool `kubernaut.ai/tools` check), sharing
   the existing SAR-call + cache infrastructure via an extracted `checkSAR`
   helper.
2. **New endpoint**: `GET /a2a/access` returns `200` when the caller's
   identity has console access, `403` (RFC 7807) otherwise. Sits behind the
   same `AuthMiddleware`/`PostAuthMiddleware` tier as the existing
   authenticated routes — no new unauthenticated surface.
3. **Real server-side enforcement (the actual fix)**: `checkRBAC` (`/mcp`)
   and `newRBACGuard` (`/a2a/invoke`'s tool-calling loop) both additionally
   deny a tool call when the configured authorizer implements
   `ConsoleAuthorizer` and denies console access — even when the specific
   tool being called would otherwise be allowed. Proven by real HTTP/runner
   round trips that **never call `/a2a/access`**, demonstrating the bypass is
   closed, not merely that the new endpoint itself works.
4. **Zero-touch backward compatibility**: the enforcement is implemented as
   an optional-interface type-assertion on the existing `Authorizer`
   field/param (no new required field, no signature change), so pre-existing
   test doubles (`allowAllAuthorizer`, `mapAuthorizer`, `mockAuthorizerImpl`,
   etc.) that don't implement `ConsoleAuthorizer` continue to exercise the
   unchanged, per-tool-only code path.
5. **Production correctness guaranteed at compile time**: `var _
   auth.ConsoleAuthorizer = (*SARChecker)(nil)` ensures the single
   production `*SARChecker` instance — wired to both `MCPBridgeConfig.Authorizer`
   and `agent.AgentConfig.Authorizer` from the same `cmd/apifrontend/main.go`
   construction site — always satisfies `ConsoleAuthorizer`, so the "skip"
   branch can only ever be taken by test doubles.
6. **RBAC manifests are CI-gated, not just manually inspected**: a new
   `kubernaut-console-access` `ClusterRole`/`ClusterRoleBinding` pair,
   validated by new cases in the existing CI-gated `helm-unittest` suite
   (`charts/kubernaut/tests/`, per `DD-PLATFORM-005`).

### 1.3 Success Metrics

| Metric | Target | Measurement | Result |
|--------|--------|-------------|--------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/... -run "UT-AF-1919" -v -count=1` | 100% |
| Integration test pass rate | 100% | `go test ./pkg/apifrontend/... -run "IT-AF-1919" -v -count=1` | 100% |
| Helm-unittest pass rate | 100% | `helm unittest charts/kubernaut` | 100% |
| Regression pass rate (pre-existing) | 100% | Full `pkg/apifrontend/...` suite unchanged | 100% |
| Build/Lint | Clean | `go build ./...`, `golangci-lint run` | Clean |

---

## 2. References

### 2.1 Authority

- Issue #1919: original feature request (backend scope only).
- Issue #1941: this `release/v1.5` backport's tracking issue.
- PR #1940: original `main` implementation.
- `jordigilh/kubernaut-console#48`: companion frontend issue (separate repo,
  out of scope).
- ADR-021 / ADR-022: existing SAR-based `kubernaut.ai/tools` authorization
  model this feature extends by one coarse-grained resource.
- `docs/architecture/decisions/DD-PLATFORM-005-helm-unittest-ci-integration.md`:
  authority for using `charts/kubernaut/tests/*.yaml` (not manual `helm
  template`) for RBAC manifest validation.

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| AC-3 (Access Enforcement) | The system enforces approved authorizations for logical access | `kubernaut.ai/console` `use` gates both the new endpoint and, critically, both existing tool-invocation paths | UT-AF-1919-001..002, 006, 008; IT-AF-1919-001, 002, 005, 006 |
| AC-6 (Least Privilege) | Users/processes are granted only the access necessary | Console access is a separate, explicit grant from per-tool access — not derived/implied from any tool grant | UT-AF-1919-006..009; RBAC manifest design (§3.1) |
| AU-12 (Audit Generation) | The system generates audit records for security-relevant events | Console-access denial at the endpoint and at both enforcement sites emits `audit.EventAuthAccessDenied` | IT-AF-1919-003; UT-AF-1919-008 |

---

## 3. Scope

### 3.1 In Scope

| Area | Description |
|------|--------------|
| `pkg/apifrontend/auth/sar.go` | `checkSAR` extraction, `CheckConsoleAccess`, `ConsoleAuthorizer` interface, compile-time assertion |
| `pkg/apifrontend/handler/console_access.go` (new) | `NewConsoleAccessHandler` |
| `pkg/apifrontend/handler/router.go`, `cmd/apifrontend/main.go` | `GET /a2a/access` route + wiring |
| `pkg/apifrontend/handler/mcp_bridge.go` (`checkRBAC`) | Console-access type-assertion enforcement for `/mcp` |
| `pkg/apifrontend/agent/root.go` (`newRBACGuard`) | Console-access type-assertion enforcement for `/a2a/invoke` |
| `charts/kubernaut/templates/apifrontend/apifrontend.yaml`, `charts/kubernaut/values.yaml`, `charts/kubernaut/values.schema.json` | New `kubernaut-console-access` ClusterRole/Binding + `consoleAccessGroups` values key (all 6 personas by default) |
| `deploy/apifrontend/overlays/e2e/e2e-user-rbac.yaml` | Dedicated cluster-scoped `e2e-console-access` ClusterRole/Bindings so E2E users keep passing the new gate |
| `charts/kubernaut/tests/console_access_rbac_test.yaml` (new) | Helm-unittest coverage for the new manifests |

### 3.2 Out of Scope

- `kubernaut-console` frontend gating (`jordigilh/kubernaut-console#48`) — separate repo.
- Issue #1934 (mcpclient connect timeout) — `pkg/fleet` does not exist on
  `release/v1.5`; confirmed not applicable in issue #1941.
- Any `main`-only refactor unrelated to #1919 (function extractions for
  #1366/#1496/#1677/#1718/#1737/#1790/#1801/#1827/#1923,
  DD-PLATFORM-006/008) — deliberately not carried over; this backport is
  scoped to the #1919 security fix only.
- Any change to `ADR-022`'s "AF SA does all K8s I/O" model — this is just
  another `subjectaccessreviews create` call by the existing AF ServiceAccount.
- Any change to the existing `ToolAuthorizer`/`Check` contract or its
  existing call sites — the new capability is strictly additive.

---

## 4. Test Scenarios

### 4.1 Unit — `SARChecker.CheckConsoleAccess` (`pkg/apifrontend/auth/sar_test.go`)

| ID | FedRAMP | Business Behavior Verified | Acceptance Criteria | Status |
|----|---------|------------------------------|----------------------|--------|
| UT-AF-1919-001 | AC-3 | `CheckConsoleAccess` returns `true` when the SAR reactor allows `kubernaut.ai/console` `use` | `(true, nil)` returned | Passed |
| UT-AF-1919-002 | AC-3 | `CheckConsoleAccess` returns `false` when SAR denies | `(false, nil)` returned | Passed |
| UT-AF-1919-003 | AC-3 | Fail-closed on empty user | `(false, err)` returned, no SAR call made | Passed |
| UT-AF-1919-004 | AC-3 | Fail-closed on SAR API error | `(false, err)` returned | Passed |
| UT-AF-1919-005 | AC-3 | Constructed `SubjectAccessReview` has the correct shape | `Verb=use, Group=kubernaut.ai, Resource=console, Name=""` | Passed |

### 4.2 Unit — `checkRBAC` enforcement (`pkg/apifrontend/handler/mcp_bridge_test.go`)

New `dualAuthorizer{toolAllowed, consoleAllowed bool}` double implementing both `ToolAuthorizer` and `ConsoleAuthorizer`.

| ID | FedRAMP | Business Behavior Verified | Acceptance Criteria | Status |
|----|---------|------------------------------|----------------------|--------|
| UT-AF-1919-006 | AC-3, AC-6 | `checkRBAC` denies the tool call when the authorizer implements `ConsoleAuthorizer` and denies console access, even though the per-tool check would allow that tool | Real MCP tool call via `handler.NewMCPHandler` returns an error result containing "console" | Passed |
| UT-AF-1919-007 | AC-3 | `checkRBAC` behavior is unchanged when the configured authorizer does **not** implement `ConsoleAuthorizer` | Tool call succeeds exactly as the pre-existing `allowAllAuthorizer`-based tests already prove — zero-touch confirmed | Passed |

### 4.3 Unit — `newRBACGuard` enforcement (`pkg/apifrontend/agent/root_test.go`)

Same `dualAuthorizer` double (agent-package-local copy, mirroring the existing per-package `mockAuthorizerImpl` duplication convention already in this codebase).

| ID | FedRAMP | Business Behavior Verified | Acceptance Criteria | Status |
|----|---------|------------------------------|----------------------|--------|
| UT-AF-1919-008 | AC-3, AU-12 | `newRBACGuard` denies the tool call when console access is denied even though the per-tool check would allow it; emits audit event with `reason=console_access_denied` | `NewRBACGuardForTest` direct invocation returns `{"error": ...}` containing "console"; spy auditor captures one `EventAuthAccessDenied` event with `Detail["reason"]=="console_access_denied"` | Passed |
| UT-AF-1919-009 | AC-3 | `newRBACGuard` behavior is unchanged when the configured authorizer does not implement `ConsoleAuthorizer` | Existing `mockAuthorizerImpl`-based tests (UT-AF-1221-020/021, UT-AF-1332-080/081) continue to pass unmodified — zero-touch confirmed | Passed |

### 4.4 Integration — `GET /a2a/access` (new `pkg/apifrontend/handler/console_access_test.go`)

Real `handler.NewRouter` + real middleware chain + fake `ConsoleAuthorizer`, mirroring `handler_test.go`'s router-level harness.

| ID | FedRAMP | Business Behavior Verified | Acceptance Criteria | Status |
|----|---------|------------------------------|----------------------|--------|
| IT-AF-1919-001 | AC-3 | `GET /a2a/access` with a valid identity + allowed console access returns `200` | HTTP 200 | Passed |
| IT-AF-1919-002 | AC-3 | Returns `403` (RFC 7807 `application/problem+json`) when console access is denied | HTTP 403, `Content-Type: application/problem+json` | Passed |
| IT-AF-1919-003 | AU-12 | Denial emits `audit.EventAuthAccessDenied` with `Detail["endpoint"]=="console"` | Spy auditor captures the event | Passed |
| IT-AF-1919-004 | IA-2 | Request with no identity in context returns `401` (route sits behind `AuthMiddleware`, not a new unauthenticated hole) | HTTP 401 | Passed |

### 4.5 Integration — bypass-closure proof (the actual security fix)

Real router/agent dispatch, **`/a2a/access` is never called** — proves a
client that skips the pre-flight endpoint entirely still cannot bypass the
console gate.

| ID | FedRAMP | Business Behavior Verified | Acceptance Criteria | Status |
|----|---------|------------------------------|----------------------|--------|
| IT-AF-1919-005 | AC-3 | Real `POST /mcp` tool-call request, console access denied but tool access allowed, `/a2a/access` never called — the tool call is still denied | Error result mentioning console access denial | Passed |
| IT-AF-1919-006 | AC-3 | Real ADK `runner.Run` dispatch (same `newRBACGuard`-as-`BeforeToolCallback` wiring shape `NewRootAgent` uses) with an LLM that decides to call a tool, console denied/tool allowed, `/a2a/access` never called — the tool call is denied | Function response contains the console-denial error, mirroring the existing `IT-AF-1221-020/021` harness convention in this file | Passed |

### 4.6 Helm-unittest — RBAC manifests (new `charts/kubernaut/tests/console_access_rbac_test.yaml`)

| ID | Business Behavior Verified | Acceptance Criteria | Status |
|----|------------------------------|----------------------|--------|
| IT-HELM-1919-001 | `kubernaut-console-access` ClusterRole is rendered with the correct rule | `isKind: ClusterRole`, `rules[0].resources` contains `console`, `rules[0].verbs` contains `use` | Passed |
| IT-HELM-1919-002 / 002b / 002c | A `ClusterRoleBinding` is rendered per configured group; ungranted groups get none; `cicd`/`l3-audit` are bound by default | `isKind: ClusterRoleBinding`, `subjects[0].name` equals the configured group | Passed |
| IT-HELM-1919-003 / 003b | Empty `consoleAccessGroups` renders the ClusterRole but no bindings (loop is empty-safe) | ClusterRole renders; no matching `ClusterRoleBinding` document | Passed |

---

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/UT Test ID |
|---|---|---|---|
| `SARChecker.CheckConsoleAccess` / `checkSAR` | `consoleAccessHandler.ServeHTTP` | `pkg/apifrontend/auth/sar.go` | IT-AF-1919-001/002 |
| `NewConsoleAccessHandler` | `GET /a2a/access` route | `pkg/apifrontend/handler/console_access.go` (new) | IT-AF-1919-001..004 |
| `RouterConfig.ConsoleAccessHandler` | `cmd/apifrontend/main.go` inline `RouterConfig` construction (no `buildRouterConfig` extraction on `release/v1.5`) | `pkg/apifrontend/handler/router.go` | IT-AF-1919-001..004 (via real `handler.NewRouter`) |
| `checkRBAC` console-access enforcement (type-assertion on `cfg.Authorizer`) | MCP tool dispatch (`cmd/apifrontend/main.go` -> `sarChecker` as `Authorizer` in `MCPBridgeConfig`) | `pkg/apifrontend/handler/mcp_bridge.go` | IT-AF-1919-005, UT-AF-1919-006/007 |
| `newRBACGuard` console-access enforcement (type-assertion on `authorizer`) | agent `BeforeToolCallback` chain (`cmd/apifrontend/main.go` -> `agent.AgentConfig` -> `sarChecker` as `Authorizer`) | `pkg/apifrontend/agent/root.go` | IT-AF-1919-006, UT-AF-1919-008/009 |
| `var _ auth.ConsoleAuthorizer = (*SARChecker)(nil)` | compile-time guarantee, no runtime entry point | `pkg/apifrontend/auth/sar.go` | build (compile failure = test failure) |
| `kubernaut-console-access` ClusterRole/Binding | Helm chart render + E2E overlay | `charts/kubernaut/templates/apifrontend/apifrontend.yaml`, `deploy/apifrontend/overlays/e2e/e2e-user-rbac.yaml` | IT-HELM-1919-001..003 |

---

## 6. TDD Execution Phases

| Phase | Type | Scope | Result |
|-------|------|-------|--------|
| 1 | RED (equivalent) | Confirmed `pkg/fleet`/`buildRouterConfig` absence and pre-#1919 code shape on `release/v1.5` via a temporary `git worktree` diff against the original `main` implementation | Confirmed gap exists |
| 2 | GREEN | `checkSAR`/`CheckConsoleAccess`/`ConsoleAuthorizer` + compile-time assertion; `NewConsoleAccessHandler`; router/main.go wiring; `checkRBAC`/`newRBACGuard` type-assertion enforcement; Helm manifests + values; E2E RBAC overlay; fixed a pre-existing chart label-duplication bug blocking helm-unittest | All tests above pass; no regression in existing `pkg/apifrontend/...` suite |
| 3 | REFACTOR | Lint/build/race for `pkg/apifrontend`; `helm lint --strict`/`helm unittest` validation | All tests remain green |

---

## 7. Execution

```bash
go build ./...
go test ./pkg/apifrontend/... -run "UT-AF-1919|IT-AF-1919" -v -count=1
go test ./pkg/apifrontend/... -count=1
go test ./pkg/apifrontend/... -race -count=1
golangci-lint run --timeout=5m ./pkg/apifrontend/...
helm lint charts/kubernaut/ --strict --set global.image.tag=schema-check --set postgresql.auth.existingSecret=lint-placeholder --set datastorage.dbExistingSecret=lint-placeholder --set valkey.existingSecret=lint-placeholder --set kubernautAgent.llm.provider=openai --set kubernautAgent.llm.model=gpt-4o --set kubernautAgent.llm.credentialsSecretName=lint-placeholder
helm unittest charts/kubernaut
```
