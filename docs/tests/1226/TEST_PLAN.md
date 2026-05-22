# Test Plan: Issue #1226 — OIDC-Direct Authentication Mode

| Field               | Value                                                    |
|---------------------|----------------------------------------------------------|
| **Issue**           | #1226                                                    |
| **Author**          | AI Agent (supervised)                                    |
| **Status**          | **Superseded by ADR-022** (AF SA unified security model) |
| **Standard**        | IEEE 829 (adapted)                                       |
| **BR Mapping**      | BR-SECURITY-1226                                         |
| **Created**         | 2026-05-21                                               |

## 1. Introduction

This test plan covers the implementation of an opt-in OIDC-direct authentication
mode for the API Frontend. In this mode, triage tool K8s API calls use the user's
raw OIDC JWT as a bearer token instead of ServiceAccount impersonation.

### 1.1 Acceptance Criteria

| ID     | Criterion                                                                       |
|--------|---------------------------------------------------------------------------------|
| AC-01  | `NewOIDCDirectDynamicFactory` creates a `dynamic.Interface` using the user's raw JWT as BearerToken |
| AC-02  | Factory fails closed: missing identity, empty username, empty token, expired token all return errors |
| AC-03  | Base `rest.Config` is never mutated (deep copy via `rest.CopyConfig`)           |
| AC-04  | `BearerTokenFile` is explicitly cleared to prevent conflict with `BearerToken`  |
| AC-05  | `Impersonate` config is cleared (no residual impersonation headers)             |
| AC-06  | `ClientWrapper`s (e.g. circuit breaker) are applied in order                    |
| AC-07  | `UseOIDCDirect` config flag defaults to `false` and parses from YAML            |
| AC-08  | `buildDynFactory` in `main.go` routes to OIDC-direct or impersonation based on config |
| AC-09  | `serviceaccounts` removed from impersonation ClusterRole manifests              |

## 2. Test Scenarios

### 2.1 Unit Tests — NewOIDCDirectDynamicFactory

| ID              | Description                                    | AC   | Phase |
|-----------------|------------------------------------------------|------|-------|
| UT-AF-1226-001  | Error when no identity in context              | AC-02 | RED  |
| UT-AF-1226-002  | Error when username is empty                   | AC-02 | RED  |
| UT-AF-1226-003  | Error when RawToken is empty                   | AC-02 | RED  |
| UT-AF-1226-004  | Error when token is expired                    | AC-02 | RED  |
| UT-AF-1226-005  | Success with valid identity and token          | AC-01 | RED  |
| UT-AF-1226-006  | Client wrappers applied in order               | AC-06 | RED  |
| UT-AF-1226-007  | Base rest.Config not mutated                   | AC-03 | RED  |

### 2.2 Unit Tests — Config

| ID              | Description                                    | AC   | Phase |
|-----------------|------------------------------------------------|------|-------|
| UT-AF-1226-010  | `UseOIDCDirect` defaults to `false`            | AC-07 | RED  |
| UT-AF-1226-011  | `UseOIDCDirect` parses `true` from YAML        | AC-07 | RED  |

### 2.3 Unit Tests — Manifest Validation

| ID              | Description                                    | AC   | Phase |
|-----------------|------------------------------------------------|------|-------|
| UT-AF-1226-030  | Helm ClusterRole has no `serviceaccounts`      | AC-09 | RED  |
| UT-AF-1226-031  | Deploy base ClusterRole has no `serviceaccounts`| AC-09 | RED  |

### 2.4 Spike: `buildDynFactory` Testability (REMOVED)

UT-AF-1226-020/021 were removed after a preflight spike determined that
`buildDynFactory` is sufficiently covered by higher-level integration tests,
and direct unit testing would require complex mocking of `ctrl.GetConfig()`.

## 3. TDD Phases

### Phase 1 — RED
Write all failing tests from §2. Verify each fails for the correct behavioral
reason (function not found, field missing, or assertion mismatch).

### Phase 2 — GREEN
Implement minimal code to pass all tests:
- `NewOIDCDirectDynamicFactory` in `dynamic_impersonation.go`
- `UseOIDCDirect` field in `config.go`
- `buildDynFactory(cfg)` routing in `main.go`
- Remove `serviceaccounts` from 4 manifest files

### Phase 3 — REFACTOR
100 Go Mistakes audit. Code quality improvements if warranted.

## 4. GA Readiness Checkpoints

| Checkpoint | After  | Dimensions                                          |
|------------|--------|-----------------------------------------------------|
| CP-1       | RED    | Test quality, AC coverage, build, anti-patterns      |
| CP-2       | GREEN  | Tests pass, coverage, build, vet, race, security     |
| CP-3       | REFACTOR | Full 12-dimensional audit                          |

## 5. Preflight Spikes (Completed)

| Spike | Finding                                                                |
|-------|------------------------------------------------------------------------|
| S-1   | `rest.CopyConfig` does NOT clear `BearerTokenFile` when `BearerToken` is set — must clear explicitly |
| S-2   | `buildDynFactory` UT deemed unnecessary — covered by IT/E2E            |
