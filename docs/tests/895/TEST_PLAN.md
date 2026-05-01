# Test Plan: Impersonation Security Hardening

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-895-v1.0
**Feature**: Harden MCP endpoint impersonation security: strip `Impersonate-Uid`, eliminate dead Pattern B code, and remove double auth middleware application
**Version**: 1.0
**Created**: 2026-05-01
**Author**: Kubernaut Development Team
**Status**: Active
**Branch**: `fix/895-896-impersonation-hardening`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

This test plan ensures that the Kubernaut Agent MCP endpoint security is complete and
correctly architectured. Specifically, it validates that: (1) ALL Kubernetes impersonation
headers are stripped before handler processing (closing the KEP-1513 `Impersonate-Uid` gap),
(2) the dead `ExtractEffectiveUser` Pattern B code is safely removed, and (3) auth
middleware is applied exactly once per request (at the router level) rather than redundantly
inside `BootstrapMCP`.

### 1.2 Objectives

1. **Impersonate-Uid Stripping**: The `stripImpersonationHeaders` function removes `Impersonate-Uid` (KEP-1513, K8s 1.22+) in addition to `Impersonate-User`, `Impersonate-Group`, and `Impersonate-Extra-*`.
2. **Single Auth Application**: `BootstrapMCP` returns a raw MCP SDK handler without wrapping auth middleware internally. Auth is applied exactly once at the Chi router level, eliminating redundant TokenReview+SAR calls.
3. **Dead Code Removal**: `ExtractEffectiveUser` (Pattern B delegated auth) and `BootstrapMCPNoAuth` are removed. The associated unit tests (`auth_test.go`) are deleted.
4. **Zero Regressions**: All existing MCP integration tests continue to pass with auth applied at the router level.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/shared/auth/... ./test/unit/kubernautagent/mcp/... -race` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/mcp/... -race` |
| Unit-testable code coverage (middleware.go) | >=80% | `go test -coverprofile` on `pkg/shared/auth/` |
| Integration-testable code coverage (server.go) | >=80% | `go test -coverprofile` on `internal/kubernautagent/mcp/` |
| Backward compatibility | 0 regressions | Existing tests pass without modification |
| Auth calls per MCP request | Exactly 1 | Integration test assertion (single middleware invocation) |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- **BR-SECURITY-895**: Strip all Kubernetes impersonation headers from external MCP requests
- **BR-SECURITY-896**: Ensure auth middleware is applied exactly once (no double-auth)
- **DD-AUTH-MCP-001**: MCP endpoint security design decision
- **KEP-1513**: Kubernetes `Impersonate-Uid` header (K8s 1.22+)
- Issue #895: Impersonation header stripping incomplete (missing `Impersonate-Uid`)
- Issue #896: Double auth application on MCP endpoint + dead Pattern B code

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `Impersonate-Uid` not stripped allows privilege escalation | Critical: attacker can impersonate UID in K8s API calls | Low (requires valid token + KA SA with impersonate RBAC) | UT-KA-895-001 | Explicit test for Uid stripping |
| R2 | Removing internal auth from BootstrapMCP exposes unauthenticated endpoint | Critical: MCP endpoint accessible without auth | Medium (if router auth misconfigured) | IT-KA-895-001, IT-KA-895-003 | Panic guard + IT proving 401 behavior |
| R3 | Dead code removal breaks compilation | Medium: build failure | Low | CHECKPOINT 2 build validation | `go build ./...` after removal |
| R4 | Double-auth removal causes 2x TokenReview calls to persist | Low: performance degradation | Low | IT-KA-895-002 (single auth invocation) | Integration test counts middleware invocations |
| R5 | Integration tests break due to auth wiring changes | Medium: CI red | Medium | All IT-KA-895-* | Systematic update of test helpers |

### 3.1 Risk-to-Test Traceability

- **R1 (Critical)**: Mitigated by UT-KA-895-001 (direct assertion on header absence)
- **R2 (Critical)**: Mitigated by IT-KA-895-001 (401 without token) + BootstrapMCP panic guard (UT-KA-895-002)
- **R4 (Low)**: Mitigated by IT-KA-895-002 (auth counter = 1)
- **R5 (Medium)**: Mitigated by CHECKPOINT 2 full regression run

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **Impersonate-Uid Stripping** (`pkg/shared/auth/middleware.go:stripImpersonationHeaders`): Validates that Impersonate-Uid header is removed alongside User/Group/Extra-*
- **BootstrapMCP Auth Architecture** (`internal/kubernautagent/mcp/server.go:BootstrapMCP`): Returns raw handler, does not wrap with auth internally
- **Dead Code Removal** (`internal/kubernautagent/mcp/auth.go`): ExtractEffectiveUser and Pattern B logic removed
- **Production Auth Path** (`cmd/kubernautagent/main.go`): Auth applied once at `/api/v1` router level

### 4.2 Features Not to be Tested

- **Pattern B reimplementation**: Deferred until apifrontend ships (separate issue)
- **E2E with real K8s TokenReview**: Covered by existing E2E suite; not modified here
- **Other services' auth middleware**: DataStorage, Gateway unaffected

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Remove BootstrapMCPNoAuth entirely | Was only used by tests; tests should apply auth at router level like production |
| Keep panic guard in BootstrapMCP | Defense-in-depth: caller must prove auth is available even though BootstrapMCP doesn't apply it |
| Delete auth_test.go (Pattern B tests) | Tests exercise dead code; removing code requires removing its tests |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of `pkg/shared/auth/middleware.go` (specifically `stripImpersonationHeaders`)
- **Integration**: >=80% of `internal/kubernautagent/mcp/server.go` (specifically `BootstrapMCP` and router wiring)
- **E2E**: Not modified; existing E2E suite provides coverage

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Validate header stripping logic in isolation (fast, deterministic)
- **Integration tests**: Validate full HTTP path through Chi router + MCP SDK (real HTTP)

### 5.3 Business Outcome Quality Bar

Tests validate **security outcomes**:
- "An attacker cannot inject Impersonate-Uid to escalate privileges"
- "An unauthenticated request is rejected before reaching MCP handlers"
- "Auth is executed exactly once, not duplicated"

### 5.4 Pass/Fail Criteria

> **IEEE 829 §9** — When is this test plan considered passed or failed?

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. Per-tier code coverage meets >=80% threshold on modified files
3. No regressions in existing MCP test suites
4. `go build ./...` succeeds (no compilation errors from dead code removal)
5. `-race` flag detects no data races

**FAIL** — any of the following:

1. Any P0 test fails
2. Coverage falls below 80% on modified files
3. Existing tests fail after changes (regression)
4. Build fails after dead code removal

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build broken: code does not compile after refactoring
- Race condition detected that requires architectural investigation

**Resume testing when**:
- Build fixed and `go build ./...` clean
- Race condition root-caused and resolved

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/auth/middleware.go` | `stripImpersonationHeaders` | ~10 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/mcp/server.go` | `BootstrapMCP` | ~25 |
| `cmd/kubernautagent/main.go` | MCP route wiring (`buildMCPHandler` + router setup) | ~50 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.5` HEAD | Branch: `fix/895-896-impersonation-hardening` |
| MCP SDK | `github.com/modelcontextprotocol/go-sdk` | Current go.mod version |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SECURITY-895 | Strip Impersonate-Uid header (KEP-1513) | P0 | Unit | UT-KA-895-001 | Pending |
| BR-SECURITY-895 | Strip Impersonate-Uid header (KEP-1513) | P0 | Integration | IT-KA-895-001 | Pending |
| BR-SECURITY-896 | BootstrapMCP panics when AuthMiddleware nil | P0 | Unit | UT-KA-895-002 | Pending |
| BR-SECURITY-896 | BootstrapMCP returns raw handler (no internal auth) | P0 | Unit | UT-KA-895-003 | Pending |
| BR-SECURITY-896 | Unauthenticated MCP request rejected (401) | P0 | Integration | IT-KA-895-001 | Pending |
| BR-SECURITY-896 | Auth middleware invoked exactly once per request | P0 | Integration | IT-KA-895-002 | Pending |
| BR-SECURITY-896 | Authenticated request reaches MCP SDK | P0 | Integration | IT-KA-895-003 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

> **IEEE 829 Test Design Specification** — How test cases are organized and grouped.

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `KA` (Kubernaut Agent)
- **BR_NUMBER**: `895` (issue number)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/shared/auth/middleware.go` — `stripImpersonationHeaders` function (>=80% coverage)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-895-001` | Middleware strips `Impersonate-Uid` header before reaching any handler (KEP-1513 completeness) | Pending |
| `UT-KA-895-002` | `BootstrapMCP` panics when `AuthMiddleware` is nil — defense-in-depth against misconfiguration | Pending |
| `UT-KA-895-003` | `BootstrapMCP` returns a raw HTTP handler that does NOT enforce auth internally | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `internal/kubernautagent/mcp/server.go` + Chi router wiring (>=80% coverage)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-895-001` | Unauthenticated POST to `/api/v1/mcp` returns 401 — proves auth is enforced at router level | Pending |
| `IT-KA-895-002` | Auth middleware is invoked exactly once per MCP request — no redundant TokenReview/SAR calls | Pending |
| `IT-KA-895-003` | Authenticated POST to `/api/v1/mcp` reaches MCP SDK and returns valid JSON-RPC response | Pending |

### Tier Skip Rationale

- **E2E**: Not modified. The existing E2E suite (`test/e2e/kubernautagent/`) exercises the full production path with real K8s auth. The changes here are internal refactoring that don't alter observable E2E behavior.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-KA-895-001: Middleware strips Impersonate-Uid header (KEP-1513)

**BR**: BR-SECURITY-895
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/auth/header_stripping_test.go`

**Preconditions**:
- Auth middleware configured with valid MockAuthenticator and MockAuthorizer
- Request has valid Bearer token

**Test Steps**:
1. **Given**: An HTTP request with `Authorization: Bearer valid-token` and `Impersonate-Uid: fake-uid-12345`
2. **When**: The request passes through `Middleware.Handler`
3. **Then**: The downstream handler receives the request with `Impersonate-Uid` header absent

**Expected Results**:
1. HTTP response status is 200 (request processed normally)
2. Captured headers do NOT contain `Impersonate-Uid`

**Acceptance Criteria**:
- **Behavior**: Header is stripped before any business logic
- **Correctness**: Only the `Impersonate-Uid` header is removed; other headers preserved
- **Security**: No variant spelling (case manipulation) bypasses the strip

**Dependencies**: None

---

### UT-KA-895-002: BootstrapMCP panics when AuthMiddleware is nil

**BR**: BR-SECURITY-896
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/mcp/server_test.go`

**Preconditions**:
- None (tests constructor behavior)

**Test Steps**:
1. **Given**: `MCPDeps{AuthMiddleware: nil}`
2. **When**: `BootstrapMCP(deps)` is called
3. **Then**: The function panics with message containing "auth middleware is nil"

**Expected Results**:
1. Panic is raised (Gomega `Panic()` matcher)
2. Panic message contains "auth middleware is nil"

**Acceptance Criteria**:
- **Behavior**: Defense-in-depth guard prevents starting MCP without proof of auth availability
- **Correctness**: Panic occurs even though BootstrapMCP no longer applies auth internally

**Dependencies**: None

---

### UT-KA-895-003: BootstrapMCP returns raw handler (no internal auth wrapping)

**BR**: BR-SECURITY-896
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/mcp/server_test.go`

**Preconditions**:
- None

**Test Steps**:
1. **Given**: `MCPDeps{AuthMiddleware: countingMiddleware}` where `countingMiddleware` records invocations
2. **When**: `BootstrapMCP(deps)` is called
3. **Then**: The returned handler does NOT invoke the counting middleware when an HTTP request is sent directly to it

**Expected Results**:
1. After sending a request to the returned handler, the counting middleware invocation count is 0
2. The MCP SDK processes the request (returns JSON-RPC response or error)

**Acceptance Criteria**:
- **Behavior**: `BootstrapMCP` returns the raw `mcpsdk.NewStreamableHTTPHandler` without wrapping
- **Correctness**: AuthMiddleware is NOT called by BootstrapMCP's returned handler
- **Security**: Panic guard still present (tested by UT-KA-895-002)

**Dependencies**: UT-KA-895-002

---

### IT-KA-895-001: Unauthenticated MCP request returns 401

**BR**: BR-SECURITY-896
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/mcp/wiring_test.go`

**Preconditions**:
- MCP endpoint wired with auth middleware at Chi router level
- `BootstrapMCP` called with non-nil AuthMiddleware

**Test Steps**:
1. **Given**: A running httptest.Server with `/api/v1/mcp` route and auth middleware applied at router level
2. **When**: A POST request is sent WITHOUT `Authorization` header
3. **Then**: Response status is 401 Unauthorized

**Expected Results**:
1. HTTP 401 response
2. No MCP SDK processing occurs

**Acceptance Criteria**:
- **Behavior**: Auth is enforced at the router level (not inside BootstrapMCP)
- **Security**: Even after removing internal auth from BootstrapMCP, unauthenticated requests are blocked

**Dependencies**: None

---

### IT-KA-895-002: Auth middleware invoked exactly once per MCP request

**BR**: BR-SECURITY-896
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/mcp/wiring_test.go`

**Preconditions**:
- MCP endpoint wired with counting auth middleware at Chi router level
- `BootstrapMCP` returns raw handler (no internal auth)

**Test Steps**:
1. **Given**: A counting auth middleware that increments a counter on each invocation, applied at router level
2. **When**: A valid authenticated POST request (JSON-RPC initialize) is sent to `/api/v1/mcp`
3. **Then**: The auth middleware counter equals exactly 1

**Expected Results**:
1. Auth counter == 1 (not 2 as previously with double-auth)
2. HTTP response is 200 with valid MCP response
3. TokenReview + SAR executed exactly once

**Acceptance Criteria**:
- **Behavior**: Auth is not applied redundantly
- **Performance**: Eliminates 1x unnecessary TokenReview + 1x unnecessary SAR per request
- **Correctness**: MCP SDK still receives the request after single auth pass

**Dependencies**: IT-KA-895-001

---

### IT-KA-895-003: Authenticated request reaches MCP SDK (tools/list succeeds)

**BR**: BR-SECURITY-896
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/mcp/wiring_test.go`

**Preconditions**:
- MCP endpoint wired with auth at router level only
- `BootstrapMCP` returns raw handler

**Test Steps**:
1. **Given**: A running httptest.Server with auth at router level and BootstrapMCP handler mounted
2. **When**: An authenticated POST request (JSON-RPC initialize) is sent with valid `Authorization: Bearer` header
3. **Then**: MCP SDK processes the request and returns a valid JSON-RPC response containing `kubernaut-agent-interactive`

**Expected Results**:
1. HTTP 200 response
2. Response body contains `kubernaut-agent-interactive` (server implementation name)
3. Valid JSON-RPC 2.0 response structure

**Acceptance Criteria**:
- **Behavior**: The full production path works: router auth → raw MCP handler → SDK → response
- **Correctness**: Server name and version are present in initialize response
- **Regression**: Existing IT-KA-703-F01 behavior preserved

**Dependencies**: IT-KA-895-001

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `MockAuthenticator`, `MockAuthorizer` from `pkg/shared/auth/mock_auth.go`
- **Location**: `test/unit/shared/auth/`, `test/unit/kubernautagent/mcp/`
- **Resources**: Minimal (in-memory only)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks of business logic. Uses `httptest.Server` with real Chi router + MCP SDK
- **Infrastructure**: `httptest.Server` (no external services needed for auth wiring tests)
- **Location**: `test/integration/kubernautagent/mcp/`
- **Resources**: Minimal (HTTP server in-process)

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| golangci-lint | latest | Lint validation |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| None | — | — | — | — |

No blocking dependencies. All code is in the same repository and branch.

### 11.2 Execution Order

1. **Phase 1 (RED)**: Write failing tests — UT-KA-895-001, UT-KA-895-002, UT-KA-895-003, IT-KA-895-001, IT-KA-895-002, IT-KA-895-003
2. **CHECKPOINT 1**: RED audit — all tests compile and fail, adversarial coverage confirmed
3. **Phase 2 (GREEN)**: Minimal implementation — Uid strip, BootstrapMCP refactor, dead code removal, IT helper updates
4. **CHECKPOINT 2**: GREEN audit — all tests pass, build clean, no regressions, race-clean
5. **Phase 3 (REFACTOR)**: 100-go-mistakes validation, lint, documentation accuracy
6. **CHECKPOINT 3**: REFACTOR + Security audit — lint clean, adversarial review, performance confirmed
7. **Phase 4 (WIRING VERIFICATION)**: Prove all production code paths are reachable via IT
8. **CHECKPOINT 4**: Final audit — coverage >=80%, race-clean, PR ready

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/895/TEST_PLAN.md` | Strategy and test design |
| Unit test (Uid stripping) | `test/unit/shared/auth/header_stripping_test.go` | UT-KA-895-001 |
| Unit test (BootstrapMCP) | `test/unit/kubernautagent/mcp/server_test.go` | UT-KA-895-002, UT-KA-895-003 |
| Integration tests (wiring) | `test/integration/kubernautagent/mcp/wiring_test.go` | IT-KA-895-001..003 |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests (header stripping)
go test ./test/unit/shared/auth/... -ginkgo.v -race

# Unit tests (MCP server)
go test ./test/unit/kubernautagent/mcp/... -ginkgo.v -race

# Integration tests (MCP wiring)
go test ./test/integration/kubernautagent/mcp/... -ginkgo.v -race -ginkgo.focus="895"

# Specific test by ID
go test ./test/unit/shared/auth/... -ginkgo.focus="UT-KA-895-001"

# Coverage
go test ./pkg/shared/auth/... -coverprofile=coverage-auth.out
go tool cover -func=coverage-auth.out
```

---

## 14. Wiring Verification (TDD Phase 4)

> **Authority**: `.cursor/rules/10-wiring-verification.mdc`

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| `stripImpersonationHeaders` (Uid) | HTTP request with `Impersonate-Uid` | Header absent in handler | UT-KA-895-001 | Pending |
| `BootstrapMCP` (no inner wrap) | POST /api/v1/mcp (authenticated) | MCP SDK processes request | IT-KA-895-003 | Pending |
| Auth rejection (router level) | POST /api/v1/mcp (no token) | 401 response | IT-KA-895-001 | Pending |
| Single auth invocation | POST /api/v1/mcp (authenticated) | Auth counter == 1 | IT-KA-895-002 | Pending |

**Unit tests do NOT count as wiring proof.** Only integration tests that traverse the
real middleware/handler/router stack qualify.

---

## 15. Existing Tests Requiring Updates

> When implementation changes behavior that existing tests assert on, document the
> required updates here to prevent surprises during TDD GREEN phase.

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/kubernautagent/mcp/auth_test.go` | Tests `ExtractEffectiveUser` (Pattern B) | **DELETE entire file** | `ExtractEffectiveUser` is dead code being removed |
| `test/integration/kubernautagent/mcp/helpers_test.go` | Uses `BootstrapMCP` with `AuthMiddleware` then mounts directly | Add `r.Use(fakeAuthMiddlewareWithUserInfo)` at router level | Auth no longer applied inside BootstrapMCP |
| `test/integration/kubernautagent/mcp/wiring_test.go` | Uses `BootstrapMCP` with auth, also applies `r.Use(fakeAuthMiddleware)` | Remove double auth (BootstrapMCP no longer wraps) | Single auth at router level |
| `test/integration/kubernautagent/mcp/takeover_test.go` | Mounts handler from `BootstrapMCPNoAuth` | Use `BootstrapMCP` + router-level auth | `BootstrapMCPNoAuth` removed |
| `test/integration/kubernautagent/mcp/interactive_compat_test.go` | Uses `BootstrapMCP` with auth then mounts directly | Add router-level auth | Consistent with production pattern |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-01 | Initial test plan |
