# Test Plan: Auth-Only Fallback for the Console-Access Gate (AF Backend)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2148
**Feature**: `RBACConfig.ConsoleAccessAuthorizationCheckEnabled` — a
narrowly-scoped, opt-in switch for the coarse-grained `kubernaut.ai/console`
gate (#1919). When `false` (the default), the gate is authentication-only
(any non-empty authenticated user is allowed, no authorization check runs)
for installs with no console/RBAC bindings configured at all; when `true`,
`consoleAccessGroups` takes precedence and the gate's authorization check
(a SAR call) is enforced. Per-tool authorization (`Check`) is completely
unaffected either way and remains unconditionally fail-closed.
**Version**: 1.0
**Created**: 2026-08-14
**Author**: AI Agent
**Status**: Draft
**Branch**: `fix/2148-console-access-auth-only-v1.5`
**Milestone**: v1.5.7 (`release/v1.5`)

---

## 1. Introduction

### 1.1 Purpose

Issue #2148: the console gate is fail-closed with no permissive default. On
a fresh install with no `consoleAccessGroups`/`roleBindings` configured at
all, every authenticated user — including one who has `cluster-admin` via a
*different* identity (kubeconfig/cert) than their OIDC console login — gets
`403 Forbidden` on `GET /a2a/access` and on every `/mcp`/`/a2a/invoke` call's
inline console check, because the SAR decision is 100% delegated to
in-cluster RBAC bindings that don't exist yet. This is a real install
blocker (kubernaut-operator#334/#335), but only for the "haven't wired OIDC
groups yet" case — anyone who *has* configured RBAC is unaffected.

### 1.2 Design

A `ConsoleAccessAuthorizationCheckGate` decorator wraps `*SARChecker`,
embedding it so `Check` (per-tool authorization) is inherited unchanged and
provably untouched by construction, not merely by convention. Only
`CheckConsoleAccess` is overridden: it still fail-closes on an empty user,
but otherwise returns `(true, nil)` without ever running the authorization
check. The decision of which concrete type to construct is made exactly
once, at process startup in `buildSARClient`, based on
`RBACConfig.ConsoleAccessAuthorizationCheckEnabled` — no per-request
branching is introduced anywhere in the hot path.

**Addendum (#2150)**: `RBACConfig.ConsoleAccessAuthorizationCheckEnabled`
defaults to `false` on this branch too, matching `main`/v1.6's sibling issue
(#2150) exactly: a fresh install's console works out of the box with zero
RBAC configuration for dev/eval — the exact "haven't wired OIDC groups yet"
case this feature exists to unblock. Production installs are expected to
configure `apifrontend.config.rbac.personas`/`consoleAccessGroups` and set
this to `true` explicitly to enable the authorization check on the console
gate; a NOTES.txt hint reminds operators of this whenever the flag is left
at its default.

### 1.3 Non-Goals

- Per-tool SAR authorization (`Check`) is NOT touched — it remains
  unconditionally fail-closed with no bypass, preserving the existing
  invariant in full.
- Deciding *when* to set the flag (kubernaut-operator's
  `spec.apiFrontend.rbac == nil` heuristic) is out of scope for this repo —
  tracked as a companion change in kubernaut-operator.

## 2. Test Scope

Per the project's Pyramid Invariant (UT proves logic, IT proves wiring): UT
coverage alone on `ConsoleAccessAuthorizationCheckGate` would prove its
logic in isolation but not that it is actually consumed correctly at any of
the two production console-gate enforcement points on this branch
(`console_access.go`'s `GET /a2a/access` handler, `mcp_bridge.go`'s per-tool
`Check`). IT coverage was added specifically to close that gap.

| Component | Test Type | Test IDs |
|---|---|---|
| `auth.ConsoleAccessAuthorizationCheckGate` logic | Unit | UT-AF-2148-001..004 |
| Wiring through the real router/bridge | Integration | IT-AF-2148-001..002 |
| `buildSARClient` construction | Manual/code-review (not independently unit-testable — requires in-cluster config) | N/A |
| Helm chart passthrough | helm-unittest | HT-AF-2148-001..002 |

## 3. Test Cases

| ID | Description | Type |
|---|---|---|
| UT-AF-2148-001 | Auth-only mode allows any non-empty user, no SAR call made | UT |
| UT-AF-2148-002 | Auth-only mode still fail-closes on empty user | UT |
| UT-AF-2148-003 | Auth-only mode does not affect `Check` (per-tool) — SAR still called, allow/deny still honored | UT |
| UT-AF-2148-004 | `ConsoleAccessAuthorizationCheckGate` satisfies both `ToolAuthorizer` and `ConsoleAuthorizer` (compile-time) | UT |
| IT-AF-2148-001 | `GET /a2a/access`, through the real `handler.NewRouter` + `handler.NewConsoleAccessHandler`, returns 200 when wired with `ConsoleAccessAuthorizationCheckGate`, even though the underlying `*SARChecker` (backed by a fake K8s clientset that unconditionally denies) would have returned 403 | IT |
| IT-AF-2148-002 | A real `POST /mcp` tool call, through `handler.NewRouter` + `handler.NewMCPHandler`, still fail-closes on per-tool `Check` when wired with `ConsoleAccessAuthorizationCheckGate` as `MCPBridgeConfig.Authorizer` — proves `Check` is untouched end-to-end, not just when called directly | IT |
| HT-AF-2148-001 | Default install renders `consoleAccessAuthorizationCheckEnabled: false` into `config.yaml`, matching the Go default (#2150) | helm-unittest |
| HT-AF-2148-002 | Explicit `consoleAccessAuthorizationCheckEnabled: true` override renders through (production hardening) | helm-unittest |
| HT-AF-2150-001 | NOTES.txt shows the auth-only RBAC notice when `consoleAccessAuthorizationCheckEnabled` is left at its default (`false`) | helm-unittest |
| HT-AF-2150-002 | NOTES.txt notice is absent once `consoleAccessAuthorizationCheckEnabled` is explicitly set to `true` | helm-unittest |

## 4. Out of Scope

- E2E coverage: the underlying `SARChecker` SAR-call/cache behavior is
  already proven end-to-end by `UT-AF-1919-*`/`UT-AF-1221-*`; this feature
  only adds a decorator in front of it, fully proven by the UT+IT pairing
  above without needing a real cluster.
