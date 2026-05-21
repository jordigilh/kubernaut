# Test Plan: OIDC-Direct Authentication Mode for Triage Tools

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1226-v1.0
**Feature**: OIDC-direct authentication mode for triage tools (eliminates impersonation)
**Version**: 1.0
**Created**: 2026-05-21
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feat/1226-oidc-direct-auth`
**Parent Issue**: [#1226](https://github.com/jordigilh/kubernaut/issues/1226)
**Related**: [#1225](https://github.com/jordigilh/kubernaut/issues/1225) (scope impersonation), [#1220](https://github.com/jordigilh/kubernaut/issues/1220) (SAR RBAC)

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the behavioral acceptance criteria for two changes to the
API Frontend's authentication model:

1. **Quick win**: Remove `serviceaccounts` from the impersonation ClusterRole to
   eliminate the highest-risk attack vector (SA impersonation).
2. **OIDC-direct factory**: Add an opt-in `NewOIDCDirectDynamicFactory` that creates
   K8s API clients using the user's raw JWT as a bearer token instead of impersonation
   headers, eliminating impersonation privileges entirely for compatible deployments.

### 1.2 Feature Description

The API Frontend's 4 read-only triage tools (`af_list_events`, `af_get_pods`,
`af_get_workloads`, `af_resolve_owner`) make K8s API calls scoped to the calling
user's identity. Currently this uses K8s impersonation (`Impersonate-User/Group`
headers), requiring a broad `impersonate` privilege on the AF ClusterRole.

When the K8s API server trusts the same OIDC provider as AF (DEX, Keycloak, Okta),
the user's JWT can be forwarded directly as a bearer token. The API server
authenticates the user natively via OIDC — no impersonation needed.

The implementation uses the existing `DynamicClientFactory` function type:

```go
type DynamicClientFactory func(ctx context.Context) (dynamic.Interface, error)
```

A new `NewOIDCDirectDynamicFactory` returns this type, creating a `rest.Config` with
`BearerToken: identity.RawToken` instead of `Impersonate` headers.

### 1.3 Objectives

1. Validate `NewOIDCDirectDynamicFactory` creates a client with the user's JWT as bearer token
2. Validate fail-closed: no identity, empty token, or expired token all return errors
3. Validate `ClientWrapper`s (circuit breaker) are applied same as impersonation factory
4. Validate base `rest.Config` is never mutated (defensive deep copy)
5. Validate stale SA auth fields (`BearerTokenFile`) are cleared after copy
6. Validate `UseOIDCDirect` config flag controls factory selection
7. Validate `serviceaccounts` removed from impersonation ClusterRole in all manifests
8. Validate backward compatibility: default (`UseOIDCDirect: false`) preserves impersonation

### 1.4 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/auth/... ./pkg/apifrontend/config/... -ginkgo.v` |
| Unit test code coverage (new code) | >=80% | `go test -coverprofile` |
| Race detector | 0 races | `go test -race` |
| Build success | 0 errors | `go build ./...` |
| Vet compliance | 0 errors | `go vet ./...` |
| AC coverage | All 9 ACs | Coverage matrix in Section 7 |

---

## 2. References

### 2.1 Authoritative Documents

- [Issue #1226](https://github.com/jordigilh/kubernaut/issues/1226) — OIDC-direct auth mode
- [Issue #1225](https://github.com/jordigilh/kubernaut/issues/1225) — Scope impersonation privileges
- [ADR-021](docs/services/apifrontend/adr/ADR-021-sar-based-tool-authorization.md) — SAR-based tool authorization
- [TESTING_GUIDELINES.md](docs/development/business-requirements/TESTING_GUIDELINES.md) — Per-tier coverage >=80%
- [ANTI_PATTERN_DETECTION.md](docs/testing/ANTI_PATTERN_DETECTION.md) — Forbidden test patterns
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — TDD REFACTOR reference

### 2.2 Key Source Files

| File | Role |
|------|------|
| `pkg/apifrontend/auth/dynamic_impersonation.go` | Factory implementations |
| `pkg/apifrontend/auth/types.go` | `UserIdentity` with `RawToken`, `ExpiresAt` |
| `pkg/apifrontend/auth/jwt_delegation.go` | Token expiry pattern to reuse |
| `pkg/apifrontend/config/config.go` | `RBACConfig` struct |
| `cmd/apifrontend/main.go` | `buildDynFactory()` wiring |
| `charts/kubernaut/templates/apifrontend/apifrontend.yaml` | Helm ClusterRole |
| `deploy/apifrontend/base/02-rbac.yaml` | Deploy ClusterRole |

---

## 3. Test Items

### 3.1 Items Under Test

| Component | Type | Changed |
|-----------|------|---------|
| `NewOIDCDirectDynamicFactory` | New function | Yes (new) |
| `RBACConfig.UseOIDCDirect` | New config field | Yes (new) |
| `buildDynFactory()` | Wiring | Yes (branching) |
| ClusterRole impersonation rules | RBAC manifest | Yes (remove `serviceaccounts`) |

### 3.2 Items Not Under Test

| Component | Reason |
|-----------|--------|
| `NewImpersonatingDynamicFactory` | Unchanged, existing tests cover |
| Triage tools (`af_get_pods`, etc.) | Consume `DynamicClientFactory` interface — transparent |
| K8s API server OIDC validation | Deployment-time configuration |

---

## 4. Business Acceptance Criteria

| ID | Criterion | Verification |
|----|-----------|-------------|
| AC-1 | `NewOIDCDirectDynamicFactory` creates `dynamic.Interface` using `BearerToken: identity.RawToken` | UT-AF-1226-005 |
| AC-2 | Factory rejects when no identity in context (fail-closed) | UT-AF-1226-001, 002 |
| AC-3 | Factory rejects when `RawToken` is empty (fail-closed) | UT-AF-1226-003 |
| AC-4 | Factory rejects expired tokens (fail-closed) | UT-AF-1226-004 |
| AC-5 | Factory applies `ClientWrapper`s (circuit breaker) | UT-AF-1226-006 |
| AC-6 | `UseOIDCDirect: true` in config activates OIDC-direct mode | UT-AF-1226-011 |
| AC-7 | `UseOIDCDirect: false` (default) preserves impersonation | UT-AF-1226-010 |
| AC-8 | `serviceaccounts` removed from impersonation ClusterRole | UT-AF-1226-030, 031 |
| AC-9 | Helm, deploy, E2E manifests consistent | UT-AF-1226-030, 031 + manual review |

---

## 5. Test Scenarios

### 5.1 Unit Tests — `pkg/apifrontend/auth` (NewOIDCDirectDynamicFactory)

| ID | Description | AC | Input | Expected |
|----|-------------|-----|-------|----------|
| UT-AF-1226-001 | No identity in context | AC-2 | `context.Background()` | Error containing "OIDC-direct requires authenticated user identity" |
| UT-AF-1226-002 | Empty username | AC-2 | Identity with `Username: ""` | Error containing "OIDC-direct requires authenticated user identity" |
| UT-AF-1226-003 | Empty RawToken | AC-3 | Identity with valid username, `RawToken: ""` | Error containing "raw JWT token required" |
| UT-AF-1226-004 | Expired token | AC-4 | Identity with `ExpiresAt` in the past | Error wrapping `ErrTokenExpiredDelegation` |
| UT-AF-1226-005 | Valid identity creates client | AC-1 | Identity with username, groups, RawToken | Non-nil `dynamic.Interface`, no error |
| UT-AF-1226-006 | ClientWrappers applied | AC-5 | Valid identity + wrapper func | Wrapper called, client returned |
| UT-AF-1226-007 | Base config not mutated | AC-1 | Valid identity, check `baseCfg` after call | `baseCfg.BearerToken` unchanged, `baseCfg.BearerTokenFile` unchanged |

### 5.2 Unit Tests — `pkg/apifrontend/config` (UseOIDCDirect)

| ID | Description | AC | Input | Expected |
|----|-------------|-----|-------|----------|
| UT-AF-1226-010 | Default config has UseOIDCDirect=false | AC-7 | `DefaultConfig()` | `cfg.RBAC.UseOIDCDirect == false` |
| UT-AF-1226-011 | YAML parsing sets UseOIDCDirect=true | AC-6 | YAML with `rbac.useOIDCDirect: true` | `cfg.RBAC.UseOIDCDirect == true` |

### 5.3 Manifest Tests — ClusterRole validation

| ID | Description | AC | Input | Expected |
|----|-------------|-----|-------|----------|
| UT-AF-1226-030 | Helm ClusterRole has no serviceaccounts | AC-8 | Rendered Helm template | `resources` does not contain `serviceaccounts` |
| UT-AF-1226-031 | Deploy ClusterRole has no serviceaccounts | AC-8 | `deploy/apifrontend/base/02-rbac.yaml` | `resources` does not contain `serviceaccounts` |

---

## 6. TDD Implementation Phases

### Phase 1: TDD RED — Write Failing Tests

**Objective**: All test scenarios compile but fail because `NewOIDCDirectDynamicFactory` does not exist.

**Files**:
- `pkg/apifrontend/auth/dynamic_impersonation_test.go` — Add `Describe("NewOIDCDirectDynamicFactory")` with UT-AF-1226-001..007
- `pkg/apifrontend/config/config_test.go` — Add UT-AF-1226-010, 011

**Exit criteria**: Tests compile with a minimal stub, all 9 UT tests fail for the correct behavioral reason.

### CHECKPOINT 1: GA Readiness Audit (RED)

| Dimension | Check |
|-----------|-------|
| Test quality | No `XIt`, no `time.Sleep`, all test IDs map to ACs |
| Test coverage | All 9 ACs have at least one test |
| Build | Compiles with stub |

### Phase 2: TDD GREEN — Minimal Implementation

**Objective**: All tests pass with minimal correct implementation.

**Files**:
- `pkg/apifrontend/auth/dynamic_impersonation.go` — Add `NewOIDCDirectDynamicFactory`
- `pkg/apifrontend/config/config.go` — Add `UseOIDCDirect bool` to `RBACConfig`
- `cmd/apifrontend/main.go` — Branch in `buildDynFactory()` on config flag
- 4 manifest files — Remove `serviceaccounts` from impersonation

**Key implementation detail** (from preflight spike):
```go
cfg := rest.CopyConfig(baseCfg)
cfg.BearerToken = identity.RawToken
cfg.BearerTokenFile = "" // Spike confirmed: CopyConfig retains SA token file path
```

**Exit criteria**: All tests pass, `go build ./...`, `go vet`, `-race` clean.

### CHECKPOINT 2: GA Readiness Audit (GREEN)

| Dimension | Check |
|-----------|-------|
| Tests | 100% pass, 0 pending |
| Coverage | >=80% on new code |
| Build | `go build ./...` clean |
| Race | `go test -race` clean |
| Security | Fail-closed on all error paths |

### Phase 3: TDD REFACTOR — Code Quality + 100 Go Mistakes

**100 Go Mistakes validation**:

| # | Mistake | Check |
|---|---------|-------|
| 10 | Type embedding awareness | `rest.CopyConfig` deep-copies; verify no shared slice/map refs |
| 48 | Forgetting about context | Factory receives `ctx` from caller; `dynamic.NewForConfig` doesn't need it (document why) |
| 56 | Not using time.Duration | Config already uses `time.Duration` (SARCacheTTL pattern) |
| 77 | Not closing resources | `dynamic.Interface` is stateless — no close needed |
| 89 | Not handling errors | Every error path wraps with `fmt.Errorf("...: %w", err)` |

**Exit criteria**: No code quality findings, consistent naming, no redundant comments.

### CHECKPOINT 3: Full 12-Dimensional GA Readiness Audit

Architecture, security, test coverage, test quality, deployment, backward compatibility,
observability, performance, documentation, supply chain, resilience, E2E readiness.

### Phase 4: Documentation

- `docs/services/apifrontend/security/AUTHENTICATION_AND_RBAC.md` — OIDC-direct mode section
- `docs/services/apifrontend/development/DEVELOPER_GUIDE.md` — Configuration instructions

### Phase 5: Commit, Push, PR

| # | Message | Files |
|---|---------|-------|
| 1 | `security(rbac): remove serviceaccounts from impersonation ClusterRole (#1226)` | 4 manifest files |
| 2 | `test(auth): add failing tests for OIDC-direct factory (TDD RED) (#1226)` | test files |
| 3 | `feat(auth): add OIDC-direct DynamicClientFactory (#1226)` | impl + config + wiring |
| 4 | `docs: add OIDC-direct mode to RBAC and developer docs (#1226)` | doc files |

---

## 7. Acceptance Criteria Coverage Matrix

| AC | UT | IT | E2E | Status |
|----|----|----|-----|--------|
| AC-1 | UT-AF-1226-005 | — | — | Planned |
| AC-2 | UT-AF-1226-001, 002 | — | — | Planned |
| AC-3 | UT-AF-1226-003 | — | — | Planned |
| AC-4 | UT-AF-1226-004 | — | — | Planned |
| AC-5 | UT-AF-1226-006 | — | — | Planned |
| AC-6 | UT-AF-1226-011 | — | — | Planned |
| AC-7 | UT-AF-1226-010 | — | — | Planned |
| AC-8 | UT-AF-1226-030, 031 | — | — | Planned |
| AC-9 | UT-AF-1226-030, 031 | — | Manual | Planned |

---

## 8. Anti-Patterns Checklist

| Anti-Pattern | Status |
|-------------|--------|
| No `time.Sleep` in tests | Enforced |
| No `XIt` / pending tests | Enforced |
| No `Skip()` to avoid failures | Enforced |
| Ginkgo/Gomega BDD framework | Required |
| Table-driven where appropriate | Applied |
| No mocking of business logic | Enforced |
