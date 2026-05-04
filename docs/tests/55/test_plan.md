# Test Plan: End-to-End AuthZ Enforcement — User Impersonation and JWT Delegation

**Test Plan Identifier:** TP-AF-055
**Issue:** [#55](https://github.com/jordigilh/kubernaut-apifrontend/issues/55)
**Version:** 1.0
**Date:** 2026-05-03
**Status:** Draft

---

## 1. Introduction

This test plan validates the authorization enforcement chain in the kubernaut API Frontend (AF): user impersonation for K8s API calls during triage, and JWT delegation to the Kubernaut Agent (KA) for interactive MCP calls. This implements the two-tier access model where user-scoped queries use impersonation while AF-owned resource creation uses the AF ServiceAccount.

### 1.1 Scope

- K8s user impersonation via `rest.ImpersonationConfig`
- Two-tier access model (user identity vs. AF ServiceAccount)
- JWT delegation (forwarding original JWT to KA)
- 403 Forbidden handling (user-friendly error translation)
- Client factory (resource-type-based client selection)
- Config deep-copy safety (original `rest.Config` never mutated)

### 1.2 References

- Issue #55: End-to-end authz enforcement — user impersonation and JWT delegation
- Issue #2: OIDC authentication (dependency — identity resolution)
- kubernaut#1009: Pattern B trust-boundary mechanism (JWT Bearer Token, Option A)
- ARCHITECTURE.md Section 6: Identity Chain, RBAC (ClusterRole)
- KA reference: `internal/kubernautagent/mcp/impersonate.go`

---

## 2. Test Items

| Item | Package | Version |
|------|---------|---------|
| `NewImpersonatedClient` | `internal/auth` | PR2 |
| `JWTDelegationTransport` | `internal/auth` | PR2 |
| `ClientFactory` | `internal/auth` | PR2 |
| `BuildImpersonationConfig` | `internal/auth` | PR2 |

---

## 3. Features to Be Tested

### 3.1 Impersonation (K8s API Calls)

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-55.1 | Impersonate-User header | K8s API requests include `Impersonate-User` header matching authenticated username | Issue #55 |
| F-55.2 | Impersonate-Group header | K8s API requests include `Impersonate-Group` headers for all user groups | Issue #55 |
| F-55.3 | Triage verbs allowed | `get`, `list` on events, pods, deployments succeed with user identity | Issue #55 |
| F-55.4 | Owner resolution | ownerReference chain traversal uses user identity (impersonated) | Issue #55 |
| F-55.5 | Scope check | Reading target resource labels at RCA time uses user identity | Issue #55 |
| F-55.6 | Config deep-copy | Original `rest.Config` is never mutated; new config returned | KA ref impl |
| F-55.7 | Empty username error | Empty username in context returns error (not silently skipped) | KA ref impl |

### 3.2 Two-Tier Access Model

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-55.8 | RR creation uses SA | RemediationRequest creation uses AF's own ServiceAccount (ADR-057) | Issue #55 |
| F-55.9 | IS creation uses SA | InvestigationSession CRD creation uses AF's own SA | Issue #55 |
| F-55.10 | Lease creation uses SA | Coordination Lease operations use AF's own SA | Issue #55 |
| F-55.11 | User queries use impersonation | Triage queries (events, pods, deployments) use impersonation | Issue #55 |

### 3.3 JWT Delegation to KA

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-55.12 | JWT forwarded | Original JWT forwarded as `Authorization: Bearer <jwt>` to KA | Issue #55, kubernaut#1009 |
| F-55.13 | No token modification | Forwarded JWT is byte-identical to received JWT (no re-signing) | Issue #55 |

### 3.4 403 Handling

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-55.14 | User-friendly 403 | K8s 403 Forbidden translated to user-friendly error message | Issue #55 |
| F-55.15 | Stop on 403 | AF stops current operation chain on 403 (natural RBAC gate) | Issue #55 |

---

## 4. Features Not Tested

- AF ServiceAccount ClusterRole/Binding creation (Helm chart scope, PR7)
- KA-side JWT validation (kubernaut#1009, KA repo)
- Actual K8s RBAC policy evaluation (tested via integration tests in CI with real cluster)
- Namespace-scoped impersonation (AF impersonates globally; RBAC on target resources is the gate)

---

## 5. Approach

### 5.1 Test Methodology

Test-Driven Development (TDD):
1. **Red phase:** Write all test cases as failing tests
2. **Green phase:** Implement minimal production code
3. **Refactor phase:** Clean up against 100-go-mistakes (`errors.As`/`errors.Is`, resource cleanup)

### 5.2 Test Levels

| Level | Scope | Tools |
|-------|-------|-------|
| Unit | `NewImpersonatedClient`, `BuildImpersonationConfig`, config deep-copy | `testing`, `rest.CopyConfig` |
| Integration | `ClientFactory` routing decisions, `JWTDelegationTransport` | `httptest`, `net/http` |
| Component | Full impersonation chain with fake K8s client | `k8s.io/client-go/kubernetes/fake`, reactors |

### 5.3 Test Infrastructure

- **Fake K8s client:** `fake.NewSimpleClientset` with custom reactors for 403 simulation
- **HTTP capture server:** `httptest.NewServer` to verify JWT forwarding headers
- **Context setup:** Pre-populated `UserIdentity` in context (simulating JWT middleware output)
- **Config verification:** Compare original `rest.Config` pointer before/after to confirm no mutation

---

## 6. Pass/Fail Criteria

### 6.1 Pass Criteria

- All 15 test cases pass
- Code coverage >= 80% of `internal/auth/*.go` (combined with JWT tests)
- Zero `golangci-lint` errors
- Zero race conditions (`go test -race`)
- 403 error path produces user-friendly message (not raw K8s Status JSON)
- Original JWT forwarded byte-identical (verified by test)

### 6.2 Fail Criteria

- Any test case fails
- Coverage drops below 80%
- Original `rest.Config` mutated by impersonation (pointer equality check)
- JWT modified during delegation (hash mismatch)
- 403 exposes internal K8s error details to user

---

## 7. Suspension and Resumption Criteria

### 7.1 Suspension

- `k8s.io/client-go` fake client cannot simulate 403 responses
- Incompatibility between `rest.ImpersonationConfig` and target K8s version

### 7.2 Resumption

- Alternative 403 simulation approach identified (custom reactor)
- K8s client-go version aligned

---

## 8. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| Test source code | `internal/auth/impersonation_test.go` |
| Coverage report | Generated by `go test -coverprofile=coverage.out` |
| This test plan | `docs/tests/55/test_plan.md` |

---

## 9. Environmental Needs

| Requirement | Details |
|-------------|---------|
| Go version | 1.25.x |
| Dependencies | `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apimachinery` |
| External services | None (all mocked via fake clientset and httptest) |
| CI environment | GitHub Actions (linux/amd64) |

---

## 10. Responsibilities

| Role | Responsibility |
|------|---------------|
| Developer | Write tests, implement code, verify security properties |
| CI | Automated test execution, race detection, coverage |
| Reviewer | Verify two-tier model correctness, 403 handling, JWT integrity |

---

## 11. Schedule

| Phase | Duration | Gate |
|-------|----------|------|
| Red (Phase 7) | Tests written, all fail | Compilation succeeds, all tests fail |
| Green (Phase 8) | All tests pass | Checkpoint E passes |
| Refactor (Phase 9) | Code cleaned up | Checkpoint F passes (final integration) |

---

## 12. Test Case Matrix

| Test Case | Features Covered | Priority |
|-----------|-----------------|----------|
| `TestImpersonatedClient_SetsHeaders` | F-55.1, F-55.2 | P0 |
| `TestImpersonatedClient_TriageVerbs_Allowed` | F-55.3 | P0 |
| `TestImpersonatedClient_OwnerResolution_UsesImpersonation` | F-55.4 | P0 |
| `TestImpersonatedClient_ScopeCheck_UsesImpersonation` | F-55.5 | P1 |
| `TestImpersonatedClient_403_StopsAndInformsUser` | F-55.14, F-55.15 | P0 |
| `TestImpersonatedClient_RRCreation_UsesServiceAccount` | F-55.8 | P0 |
| `TestImpersonatedClient_InvestigationSessionCreation_UsesServiceAccount` | F-55.9 | P0 |
| `TestImpersonatedClient_LeaseCreation_UsesServiceAccount` | F-55.10 | P0 |
| `TestImpersonatedClient_EmptyUsername_ReturnsError` | F-55.7 | P0 |
| `TestImpersonatedClient_DeepCopiesConfig` | F-55.6 | P0 |
| `TestJWTDelegation_ForwardsOriginalJWT` | F-55.12 | P0 |
| `TestJWTDelegation_NoTokenModification` | F-55.13 | P0 |
| `TestTwoTierModel_UserScopedQueries_UseImpersonation` | F-55.11 | P0 |
| `TestTwoTierModel_AFOwnedResources_UseServiceAccount` | F-55.8, F-55.9, F-55.10 | P0 |
| `TestBuildImpersonationConfig_ExtractsFromContext` | F-55.1, F-55.2 | P1 |
