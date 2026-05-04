# Test Plan: User Identity/Groups in Rego Policy Context

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-774-v1
**Feature**: Propagate user identity (username + groups) from MCP interactive sessions through KA poll response to AIAnalysis Rego policy input
**Version**: 1.0
**Created**: 2026-05-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feature/774-rego-identity`

---

## 1. Introduction

### 1.1 Purpose

Validates the end-to-end identity propagation path from MCP session takeover through KA poll response, AA CRD status, and into Rego policy evaluation — enabling identity-aware approval policies (e.g., "SRE group auto-approves production remediations") while maintaining clean decoupling between KA and AA.

### 1.2 Objectives

1. **Session bridge**: `TransitionToUserDriving` sets `StatusUserDriving` and writes identity (user + groups) to incident session metadata
2. **Poll response**: KA session status endpoint returns `"user_driving"` with `acting_user` and `acting_user_groups` when session is user-driven
3. **CRD propagation**: AA's `handleSessionPollUserDriving` writes identity from poll response to its own `AIAnalysis.Status.InteractiveSession`
4. **Rego input**: `buildPolicyInput` populates `PolicyInput.Identity` from CR status, making `input.identity.user` and `input.identity.groups` available to Rego
5. **Graceful absence**: Non-interactive flows produce nil identity; Rego policies handle this with `default require_approval := true`

### 1.3 Success Metrics

- Unit test pass rate: 100%
- Unit-testable code coverage: >=80% of new code in session store, evaluator, buildPolicyInput
- Integration-testable code coverage: >=80% of handler, server handler, client mapping
- Backward compatibility: 0 regressions in existing session, handler, or evaluator tests

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-INTERACTIVE-001**: Interactive session observability via CRD status
- **BR-INTERACTIVE-003**: Audit attribution for interactive sessions
- **BR-AI-085**: Rego PolicyInput schema (extending with identity)
- Issue #774: Expose user identity/groups to Rego policy context
- Issue #703: Agentic Integration (parent)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [BR-INTERACTIVE.md](../../requirements/BR-INTERACTIVE.md)
- [DD-INTERACTIVE-002](../../architecture/decisions/DD-INTERACTIVE-002-dynamic-takeover-model.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | StatusUserDriving incorrectly treated as terminal | Session garbage-collected, poll returns 404 | High | UT-KA-774-002 | Explicit test that `IsTerminal(StatusUserDriving) == false` |
| R2 | Groups serialized as JSON array in metadata (string-keyed map) | Deserialization failure | Medium | UT-KA-774-003 | JSON marshal/unmarshal with explicit test |
| R3 | CRD regeneration breaks existing fields | Build failure | Low | Build checkpoint | Regenerate and full build verification |
| R4 | Rego policy evaluation fails with nil identity | Runtime panic in policy engine | Medium | UT-KA-774-010 | Conditional nil check before adding to inputMap |
| R5 | Concurrent session metadata writes (takeover + poll) | Data race | Medium | UT-KA-774-004 | Session store already uses mutex; verify in test |

### 3.1 Risk-to-Test Traceability

- **R1 (High)**: UT-KA-774-002 (StatusUserDriving is non-terminal)
- **R2 (Medium)**: UT-KA-774-003 (groups serialization round-trip)
- **R4 (Medium)**: UT-KA-774-010 (nil identity in Rego input)
- **R5 (Medium)**: UT-KA-774-004 (concurrent access safety)

---

## 4. Scope

### 4.1 Features to be Tested

- **Session store** (`internal/kubernautagent/session/store.go`): `StatusUserDriving` constant, non-terminal semantics
- **Session manager** (`internal/kubernautagent/session/manager.go`): `TransitionToUserDriving` method
- **MCP takeover wiring** (`internal/kubernautagent/mcp/tools/investigate.go`): Call `TransitionToUserDriving` instead of `SuspendInvestigation`
- **KA OpenAPI spec** (`internal/kubernautagent/api/openapi.json`): `acting_user` + `acting_user_groups` on `SessionStatus` schema
- **KA handler** (`internal/kubernautagent/server/handler.go`): `mapSessionStatusToAPI` mapping + identity fields in response
- **AA client** (`pkg/agentclient/client.go`): `SessionStatusResult` fields + `PollSession` mapping
- **CRD types** (`api/aianalysis/v1alpha1/aianalysis_types.go`): `ActingUserGroups` on `InteractiveSessionInfo`
- **AA poll handler** (`pkg/aianalysis/handlers/investigating.go`): Identity copy in `handleSessionPollUserDriving`
- **Rego evaluator** (`pkg/aianalysis/rego/evaluator.go`): `IdentityInput` struct, `PolicyInput.Identity`, `inputMap` wiring
- **Policy input builder** (`pkg/aianalysis/handlers/analyzing.go`): `buildPolicyInput` reads from `status.interactiveSession`

### 4.2 Features Not to be Tested

- **JWT validation**: Covered by issue #1009 tests
- **MCP session lease management**: Covered by existing #703 tests
- **DataStorage audit persistence**: Covered by existing DS tests
- **RO controller hierarchy**: RO reads from children status — no changes to RO code in this issue

### 4.3 Design Decisions

- KA never patches the AIAnalysis CR (decoupling)
- Identity flows through KA's poll response JSON, AA reads and writes to its own status
- RO only reads status from RR's children
- `StatusUserDriving` is non-terminal (session remains pollable)
- Groups stored in session metadata as JSON-encoded string (map values are strings)

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (session status logic, evaluator, URL parser, buildPolicyInput)
- **Integration**: >=80% of integration-testable code (handlers, server handler, client mapping, CRD writes)

### 5.2 Two-Tier Minimum

Every BR is covered by at least Unit + Integration tests.

### 5.3 Pass/Fail Criteria

**PASS**: All P0 tests pass, >=80% per-tier coverage, no regressions.
**FAIL**: Any P0 test fails, coverage below 80%, or existing tests regress.

### 5.5 Suspension & Resumption Criteria

**Suspend**: CRD regeneration or ogen regeneration breaks build.
**Resume**: Build fixed and all generated code validated.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/session/store.go` | `StatusUserDriving`, `IsTerminal` | ~5 new |
| `internal/kubernautagent/session/manager.go` | `TransitionToUserDriving` | ~25 new |
| `pkg/aianalysis/rego/evaluator.go` | `IdentityInput`, `PolicyInput.Identity`, `inputMap` | ~15 new |
| `pkg/aianalysis/handlers/analyzing.go` | `buildPolicyInput` identity population | ~10 new |
| `internal/kubernautagent/server/handler.go` | `mapSessionStatusToAPI` new case | ~3 new |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/server/handler.go` | Session status endpoint with identity fields | ~10 modified |
| `pkg/agentclient/client.go` | `PollSession` mapping | ~10 modified |
| `pkg/aianalysis/handlers/investigating.go` | `handleSessionPollUserDriving` | ~10 modified |
| `internal/kubernautagent/mcp/tools/investigate.go` | `handleTakeover` wiring | ~5 modified |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-INTERACTIVE-001 | StatusUserDriving is non-terminal (session stays pollable) | P0 | Unit | UT-KA-774-001, UT-KA-774-002 | Pending |
| BR-INTERACTIVE-001 | TransitionToUserDriving cancels goroutine + sets status + writes identity | P0 | Unit | UT-KA-774-003 | Pending |
| BR-INTERACTIVE-001 | Groups serialization round-trip in metadata | P0 | Unit | UT-KA-774-004 | Pending |
| BR-INTERACTIVE-003 | mapSessionStatusToAPI maps StatusUserDriving -> "user_driving" | P0 | Unit | UT-KA-774-005 | Pending |
| BR-INTERACTIVE-003 | Session status handler returns identity when user_driving | P0 | Unit | UT-KA-774-006 | Pending |
| BR-INTERACTIVE-001 | handleSessionPollUserDriving copies identity to CR status | P0 | Unit | UT-KA-774-007 | Pending |
| BR-AI-085 | IdentityInput added to PolicyInput and wired into inputMap | P0 | Unit | UT-KA-774-008 | Pending |
| BR-AI-085 | buildPolicyInput reads identity from status.interactiveSession | P0 | Unit | UT-KA-774-009 | Pending |
| BR-AI-085 | Nil identity -> input.identity absent in Rego (graceful) | P0 | Unit | UT-KA-774-010 | Pending |
| BR-AI-085 | Identity-aware Rego policy evaluates correctly | P1 | Unit | UT-KA-774-011 | Pending |
| BR-INTERACTIVE-001 | handleTakeover calls TransitionToUserDriving with identity | P0 | Integration | IT-KA-774-001 | Pending |
| BR-INTERACTIVE-003 | PollSession maps ogen SessionStatus identity fields to SessionStatusResult | P0 | Integration | IT-KA-774-002 | Pending |
| BR-AI-085 | Full chain: poll user_driving -> CR status update -> buildPolicyInput -> Rego sees identity | P0 | Integration | IT-KA-774-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-774-{SEQUENCE}`

### Tier 1: Unit Tests

**Testable code scope**: Session store/manager, evaluator, buildPolicyInput, handler mapping — >=80% coverage target.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-774-001` | StatusUserDriving constant exists and equals "user_driving" | Pending |
| `UT-KA-774-002` | IsTerminal returns false for StatusUserDriving (session stays pollable) | Pending |
| `UT-KA-774-003` | TransitionToUserDriving sets status + writes user/groups to metadata + cancels goroutine | Pending |
| `UT-KA-774-004` | Groups serialization/deserialization round-trip through string metadata | Pending |
| `UT-KA-774-005` | mapSessionStatusToAPI maps StatusUserDriving to "user_driving" | Pending |
| `UT-KA-774-006` | Session status handler populates acting_user + acting_user_groups when user_driving | Pending |
| `UT-KA-774-007` | handleSessionPollUserDriving copies ActingUser + ActingUserGroups from poll result to CR status.interactiveSession | Pending |
| `UT-KA-774-008` | PolicyInput with non-nil Identity produces input.identity.user and input.identity.groups in inputMap | Pending |
| `UT-KA-774-009` | buildPolicyInput populates Identity from status.interactiveSession when present | Pending |
| `UT-KA-774-010` | buildPolicyInput produces nil Identity when interactiveSession is nil (graceful absence) | Pending |
| `UT-KA-774-011` | Rego policy with identity-based rule evaluates correctly (groups match -> auto-approve) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Handler wiring, client mapping, end-to-end identity chain — >=80% coverage target.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-774-001` | handleTakeover calls TransitionToUserDriving (not SuspendInvestigation) with full UserInfo | Pending |
| `IT-KA-774-002` | PollSession correctly maps ogen SessionStatus fields (acting_user, acting_user_groups) to SessionStatusResult | Pending |
| `IT-KA-774-003` | End-to-end: user_driving poll -> AA writes CR status -> buildPolicyInput -> Rego input contains identity | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — requires Kind cluster with full AA controller. Identity propagation validated at unit + integration level. E2E coverage with #874 E2E suite.

---

## 9. Test Cases

### UT-KA-774-002: StatusUserDriving is non-terminal

**BR**: BR-INTERACTIVE-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/session/store_test.go`

**Test Steps**:
1. **Given**: `StatusUserDriving` constant
2. **When**: `IsTerminal(StatusUserDriving)` is called
3. **Then**: Returns `false`

**Acceptance Criteria**:
- Session with `StatusUserDriving` is not garbage-collected by TTL cleanup
- `GetSession` succeeds for sessions in `StatusUserDriving` state

### UT-KA-774-003: TransitionToUserDriving writes identity

**BR**: BR-INTERACTIVE-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/session/manager_test.go`

**Preconditions**:
- Session store with one active `StatusRunning` session (`session-1`)
- Session has a cancel function (simulating active investigation goroutine)

**Test Steps**:
1. **Given**: Running session `session-1`
2. **When**: `TransitionToUserDriving("session-1", "jane@example.com", []string{"sre-team", "oncall"})` is called
3. **Then**:
   - `sess.Status == StatusUserDriving`
   - `sess.Metadata["acting_user"] == "jane@example.com"`
   - `sess.Metadata["acting_user_groups"]` deserializes to `["sre-team", "oncall"]`
   - Cancel function was invoked (goroutine stopped)

### UT-KA-774-008: IdentityInput in PolicyInput inputMap

**BR**: BR-AI-085
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/rego/evaluator_test.go`

**Test Steps**:
1. **Given**: `PolicyInput` with `Identity: &IdentityInput{User: "jane@example.com", Groups: []string{"sre-team"}}`
2. **When**: `inputMap` is constructed
3. **Then**: `inputMap["identity"]` contains `map["user": "jane@example.com", "groups": ["sre-team"]]`

### UT-KA-774-010: Nil identity graceful absence

**BR**: BR-AI-085
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/rego/evaluator_test.go`

**Test Steps**:
1. **Given**: `PolicyInput` with `Identity: nil` (non-interactive flow)
2. **When**: `inputMap` is constructed
3. **Then**: `inputMap` does NOT contain `"identity"` key. Rego policy with `default require_approval := true` evaluates to `true`.

### IT-KA-774-003: End-to-end identity chain

**BR**: BR-AI-085
**Priority**: P0
**Type**: Integration
**File**: `test/integration/aianalysis/handlers/investigating_identity_test.go`

**Preconditions**:
- Mock KA client returns `SessionStatusResult{Status: "user_driving", ActingUser: "jane@example.com", ActingUserGroups: ["sre-team"]}`
- AIAnalysis CR exists with `status.investigationSession`

**Test Steps**:
1. **Given**: AA controller reconciling an AIAnalysis in `Investigating` phase with active session
2. **When**: `handleSessionPoll` is called and receives `user_driving` status
3. **Then**:
   - `analysis.Status.InteractiveSession.ActingUser == "jane@example.com"`
   - `analysis.Status.InteractiveSession.ActingUserGroups == ["sre-team"]`
   - Subsequent `buildPolicyInput` produces `PolicyInput.Identity.User == "jane@example.com"` and `PolicyInput.Identity.Groups == ["sre-team"]`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock KA client (for poll response), mock Rego policy fixtures
- **Location**: `test/unit/kubernautagent/session/`, `test/unit/aianalysis/rego/`, `test/unit/aianalysis/handlers/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for real components; mock KA HTTP endpoint via `httptest.NewServer`
- **Infrastructure**: envtest for K8s API (AIAnalysis CRD)
- **Location**: `test/integration/kubernautagent/session/`, `test/integration/aianalysis/handlers/`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| ogen | latest | OpenAPI client generation |
| controller-gen | latest | CRD regeneration |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #898 | Code | Pending | #774 can proceed independently; no direct dependency | Parallel development |
| ogen binary | Tool | Available | Cannot regenerate KA client | Use pre-existing types |
| controller-gen | Tool | Available | Cannot regenerate CRDs | Use pre-existing CRD manifests |

### 11.2 Execution Order (TDD Phases)

1. **Phase 1 — TDD RED**: Write all failing unit tests (UT-KA-774-001 through 011) + integration test stubs (IT-KA-774-001 through 003)
2. **Checkpoint A — RED Gate**: All tests compile and fail for the right reason. Verify test descriptions map to BRs. Verify no anti-patterns. Verify `InteractiveSessionInfo` type exists with current fields (read before modifying).
3. **Phase 2 — TDD GREEN**: Minimal implementation across all 4 layers:
   - Layer 1: `StatusUserDriving` + `TransitionToUserDriving` + takeover wiring + `mapSessionStatusToAPI`
   - Layer 2: OpenAPI spec + ogen regen + handler identity fields + client mapping
   - Layer 3: CRD types + regen + `handleSessionPollUserDriving` update
   - Layer 4: `IdentityInput` + `PolicyInput.Identity` + `inputMap` + `buildPolicyInput`
4. **Checkpoint B — GREEN Gate**: All tests pass. `go build ./...` succeeds. `go vet ./...` clean. Coverage >= 80% per tier. CRD manifests regenerated and valid.
5. **Phase 3 — TDD REFACTOR**: Code quality pass against 100 Go Mistakes checklist (see Section 16). Naming consistency. Error handling audit. Nil safety review.
6. **Checkpoint C — REFACTOR Gate**: All tests still pass. No new lint errors. Go mistakes checklist complete. Build clean. `golangci-lint run` passes.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/774/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/session/`, `test/unit/aianalysis/rego/`, `test/unit/aianalysis/handlers/` | Ginkgo BDD test files |
| Integration test suite | `test/integration/kubernautagent/session/`, `test/integration/aianalysis/handlers/` | Ginkgo BDD test files |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/session/... ./test/unit/aianalysis/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/session/... ./test/integration/aianalysis/... -ginkgo.v

# Specific test by ID
go test ./test/unit/kubernautagent/session/... -ginkgo.focus="UT-KA-774"

# Coverage
go test ./test/unit/kubernautagent/session/... -coverprofile=coverage-774-session-ut.out
go test ./test/unit/aianalysis/... -coverprofile=coverage-774-aa-ut.out
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| Takeover -> TransitionToUserDriving | `handleTakeover()` | `sess.Status == StatusUserDriving` | IT-KA-774-001 | Pending |
| Poll -> identity in response | `GET /session/{id}` | `SessionStatus.acting_user` | IT-KA-774-002 | Pending |
| Poll -> CR status -> Rego input | `handleSessionPollUserDriving` | `input.identity.user` in Rego | IT-KA-774-003 | Pending |

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/kubernautagent/session/store_test.go` | `IsTerminal` tests cover existing statuses | Add case for `StatusUserDriving` returning `false` | New status value |
| `test/unit/kubernautagent/session/manager_test.go` | `SuspendInvestigation` tests | No change (suspend still exists); add `TransitionToUserDriving` tests | New method |
| `test/unit/kubernautagent/server/handler_test.go` | `mapSessionStatusToAPI` tests | Add case for `StatusUserDriving` -> `"user_driving"` | New mapping |
| `test/unit/aianalysis/rego/evaluator_test.go` | `PolicyInput` tests | Add identity fields to existing test cases | Extended struct |

---

## 16. 100 Go Mistakes Refactoring Checklist

| # | Mistake | Check | Applies To |
|---|---------|-------|------------|
| 1 | Variable shadowing | No `err` shadowing in `TransitionToUserDriving` or `buildPolicyInput` | Session manager, analyzing handler |
| 2 | Unnecessary nesting | Early-return for nil identity in `buildPolicyInput` | Policy input builder |
| 5 | Interface pollution | No new interfaces introduced unnecessarily | All new code |
| 8 | `any` says nothing | `IdentityInput` uses typed fields, not `any` | Evaluator types |
| 21 | Inefficient slice init | `ActingUserGroups` pre-allocated if length known | CRD types, metadata |
| 22 | nil vs empty slice | `ActingUserGroups` is nil when absent, not `[]string{}` | CRD types, evaluator |
| 27 | Inefficient map init | `inputMap` sized with known capacity | Evaluator inputMap |
| 29 | Comparing values incorrectly | Use `errors.Is`/`errors.As` for error checks | Session transitions |
| 42 | Wrong receiver type | Pointer receiver for `IdentityInput` if mutable | Evaluator |
| 47 | Defer argument eval | No defer with mutable session fields | TransitionToUserDriving |
| 49 | Error wrapping | `%w` for wrappable, `%v` for opaque | Error returns |
| 52 | Handling error twice | Don't log AND return in session transition | Manager methods |
| 53 | Not handling error | Explicit `_ =` for JSON marshal in metadata | Groups serialization |
| 58 | Data races | Session mutex held during status + metadata writes | TransitionToUserDriving |
| 69 | append data races | No concurrent append to groups slice | CRD types |
| 70 | Mutex with slices/maps | Metadata map accessed under lock only | Session store |
| 74 | Copying sync type | Session struct with mutex never copied by value | Session store/manager |
| 78 | context.Value abuse | Identity passed via struct fields, not context values | All layers |

---

## 17. Anti-Pattern Compliance

Per TESTING_GUIDELINES.md:

- **No `time.Sleep()`**: All async assertions use `Eventually()` with Gomega
- **No `Skip()`**: All tests either pass or `Fail()` with clear messages
- **No pending tests**: Every test scenario is fully implemented
- **Ginkgo/Gomega BDD**: Mandatory framework, no standard `testing` package
- **Metrics validation**: Initial/final pattern for any counter/gauge assertions
- **Mock only externals**: KA HTTP endpoint mocked via `httptest.NewServer` for integration; real session store, evaluator, and business logic used

---

## 18. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-04 | Initial test plan |
