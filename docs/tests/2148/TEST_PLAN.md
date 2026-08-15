# Test Plan: Auth-Only Fallback for the Console-Access Gate (AF Backend)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2148
**Feature**: `RBACConfig.ConsoleAccessAuthOnly` — a narrowly-scoped, opt-in
fallback for the coarse-grained `kubernaut.ai/console` SAR gate (#1919) that
makes it authentication-only (any non-empty authenticated user is allowed,
no SAR call) when no console/RBAC bindings have been configured at all.
Per-tool authorization (`Check`) is completely unaffected and remains
unconditionally fail-closed.
**Version**: 1.0
**Created**: 2026-08-14
**Author**: AI Agent
**Status**: Draft
**Branch**: `feat/2148-console-access-auth-only`

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

A `consoleAuthOnlyGate` decorator wraps `*SARChecker`, embedding it so
`Check` (per-tool authorization) is inherited unchanged and provably
untouched by construction, not merely by convention. Only
`CheckConsoleAccess` is overridden: it still fail-closes on an empty user,
but otherwise returns `(true, nil)` without ever calling the SAR API. The
decision of which concrete type to construct is made exactly once, at
process startup in `buildSARClient`, based on
`RBACConfig.ConsoleAccessAuthOnly` (default `false`) — no per-request
branching is introduced anywhere in the hot path.

### 1.3 Non-Goals

- Per-tool SAR authorization (`Check`) is NOT touched — it remains
  unconditionally fail-closed with no bypass, preserving the existing
  invariant in full.
- Deciding *when* to set the flag (kubernaut-operator's
  `spec.apiFrontend.rbac == nil` heuristic) is out of scope for this repo —
  tracked as a companion change in kubernaut-operator.

## 2. Test Scope

| Component | Test Type | Test IDs |
|---|---|---|
| `auth.ConsoleAuthOnlyGate` | Unit | UT-AF-2148-001..005 |
| `buildSARClient` wiring | Manual/code-review (not independently unit-testable — requires in-cluster config) | N/A |
| Helm chart passthrough | helm-unittest | HT-AF-2148-001 |

## 3. Test Cases

| ID | Description | Type |
|---|---|---|
| UT-AF-2148-001 | Auth-only mode allows any non-empty user, no SAR call made | UT |
| UT-AF-2148-002 | Auth-only mode still fail-closes on empty user | UT |
| UT-AF-2148-003 | Auth-only mode does not affect `Check` (per-tool) — SAR still called, allow/deny still honored | UT |
| UT-AF-2148-004 | Default (flag unset/false) behavior is provably unchanged (delegates straight to `SARChecker.CheckConsoleAccess`) | UT |
| UT-AF-2148-005 | `ConsoleAuthOnlyGate` satisfies both `ToolAuthorizer` and `ConsoleAuthorizer` (compile-time) | UT |
| HT-AF-2148-001 | `apifrontend.config.rbac.consoleAccessAuthOnly` renders into `config.yaml` | helm-unittest |

## 4. Out of Scope

- E2E/integration coverage: this is a pure decorator over already-covered
  `SARChecker` behavior (`UT-AF-1919-*`, `UT-AF-1221-*`); no new wiring path
  through the real K8s API needs re-proving.
