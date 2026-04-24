# Test Plan: OAS Endpoint Definitions — Cancel, Snapshot, Stream (#823)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-823-OAS-v1.0
**Feature**: Add cancel, snapshot, and stream endpoint contracts to KA OAS spec; implement cancel + snapshot handlers; fix `mapSessionStatusToAPI` gap
**Version**: 1.0
**Created**: 2026-04-24
**Author**: AI Assistant
**Status**: Active
**Branch**: `feature/pr2-oas-endpoints`

---

## 1. Introduction

### 1.1 Purpose

This test plan verifies that PR 2 of the v1.5 streaming plan correctly exposes three new HTTP endpoints in the Kubernaut Agent (KA) OpenAPI specification, implements the cancel and snapshot handlers, and fixes the `mapSessionStatusToAPI` gap where `StatusCancelled` was reported as `"unknown"`. The stream endpoint is defined in the OAS spec for contract visibility but implemented as a 501 stub (deferred to PR 4).

### 1.2 Objectives

1. **Cancel handler correctness**: `POST /api/v1/incident/session/{session_id}/cancel` returns 200 for running sessions, 404 for unknown sessions, and 409 for already-terminal sessions — with RFC 7807 error bodies.
2. **Snapshot handler correctness**: `GET /api/v1/incident/session/{session_id}/snapshot` returns session state for terminal sessions, 409 for in-progress sessions, and 404 for unknown sessions.
3. **Status accuracy**: `GET /api/v1/incident/session/{session_id}` correctly reports `"cancelled"` status instead of `"unknown"` for cancelled sessions.
4. **Stream stub**: `GET /api/v1/incident/session/{session_id}/stream` returns 501 Not Implemented (PR 4 implements full SSE).
5. **Ogen regen safety**: Regenerated code compiles, no existing handler interface methods change signature.
6. **Zero regression**: All 59 baseline tests pass after changes.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/server/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile -coverpkg=.../server` on handler.go |
| Backward compatibility | 0 regressions | All 59 existing tests pass without modification |
| Build | clean | `go build ./...` exits 0 |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-SESSION-002**: Session lifecycle visibility — operators can query session state at any point
- **BR-SESSION-003**: Session cancellability — operators can cancel investigations via HTTP API
- **BR-SESSION-007**: Investigation event observability — SSE stream contract defined
- **BR-HAPI-200**: RFC 7807 Error Response Standard for all error cases
- **DD-004**: RFC 7807 Problem Details
- Issue #823: Session streaming and cancellation

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [v1.5 Streaming Plan](~/.cursor/plans/ka_session_streaming_42173e7a.plan.md) — PR 2 section
- [PR1 Test Plan](TEST_PLAN.md) — Session cancellation infrastructure
- [PR1.5 Audit Test Plan](TP-823-AUDIT.md) — Session lifecycle audit trail

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Ogen regen breaks existing handler interface | Build failure, all handler tests fail | Medium | All UT-KA-823-OAS-* | Run `go generate`, verify existing 6 Handler methods unchanged |
| R2 | `x-ogen-raw-response` is not a supported ogen extension | Stream endpoint cannot use raw handler pattern | Confirmed | UT-KA-823-OAS-008 | Use ogen's `io.Reader` pattern for `text/event-stream`; stream handler deferred to UnimplementedHandler (501) |
| R3 | Cancel handler does not emit audit event | SOC2 CC8.1 audit gap for cancel via HTTP | Low | None (PR1.5 covers audit in Manager) | Verify `CancelInvestigation` already emits `aiagent.session.cancelled` audit event from PR1.5 |
| R4 | Snapshot returns stale data due to race between cancel and goroutine completion | Operator sees inconsistent session state | Low | UT-KA-823-OAS-005 | `Store.Get` returns a clone under lock; snapshot is point-in-time consistent |
| R5 | mapSessionStatusToAPI fix causes consumer breakage | Downstream consumers expecting "unknown" for cancelled sessions | Very Low | UT-KA-823-OAS-007 | "unknown" was never a documented status value; "cancelled" is the correct contract |
| R6 | ogen CLI version mismatch (CLI v1.18.0 vs go.mod v1.20.1) | Generated code incompatible with runtime library | Medium | All tests | Upgrade ogen CLI to match go.mod before regen |
| R7 | New OAS schemas create tight coupling | Schema changes propagate to downstream consumers | Low | N/A | Use minimal schemas; `jx.Raw` fallback available if schema overhead becomes a concern |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by CHECKPOINT 0 (post-regen build) and all existing tests passing
- **R2**: Confirmed during due diligence; test UT-KA-823-OAS-008 validates 501 stub
- **R3**: Covered by PR1.5 integration tests (IT-KA-823-A04); no handler-level audit needed
- **R4**: Mitigated by UT-KA-823-OAS-005 (snapshot after cancel returns consistent state)
- **R5**: Mitigated by UT-KA-823-OAS-007 (verifies "cancelled" string in status response)
- **R6**: Mitigated by ogen CLI upgrade step in TDD GREEN phase

---

## 4. Scope

### 4.1 Features to be Tested

- **Cancel handler** (`internal/kubernautagent/server/handler.go`): New method implementing `POST .../cancel` — validates that operators can stop a running investigation via HTTP and receive proper error responses for edge cases.
- **Snapshot handler** (`internal/kubernautagent/server/handler.go`): New method implementing `GET .../snapshot` — validates that operators can retrieve session state for terminal sessions.
- **mapSessionStatusToAPI fix** (`internal/kubernautagent/server/handler.go`): Adding `StatusCancelled → "cancelled"` mapping — validates that all API responses correctly report cancelled status.
- **Stream stub** (`internal/kubernautagent/server/handler.go`): Validates that the stream endpoint exists in the ogen interface and returns 501 until PR 4.
- **OAS spec** (`internal/kubernautagent/api/openapi.json`): Three new path entries with proper error response schemas.

### 4.2 Features Not to be Tested

- **SSE streaming implementation**: Deferred to PR 4. PR 2 only defines the contract.
- **CancelledResult accumulation**: Deferred to PR 3. PR 2 snapshot returns basic session state, not accumulated messages/turn/phase data.
- **Session Manager cancellation logic**: Already tested in PR1 (IT-KA-823-001 through IT-KA-823-009).
- **Audit emission**: Already tested in PR1.5 (IT-KA-823-A01 through IT-KA-823-A08).

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Typed OAS schemas for cancel/snapshot responses | Enables type-safe assertions in tests and generates proper Go types via ogen |
| Snapshot works for any terminal state (cancelled, completed, failed) | More useful than cancelled-only; PR3 enriches with CancelledResult |
| Stream uses `text/event-stream` response with `io.Reader` pattern | Ogen generates handler method with standard interface; SSE implementation in PR4 |
| No `x-ogen-raw-response` | Extension does not exist in ogen; io.Reader + flush middleware is the supported path |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of cancel handler, snapshot handler, and mapSessionStatusToAPI code paths
- **Integration**: Not applicable — handlers use in-memory session store, no I/O boundaries beyond what PR1 tests cover
- **E2E**: Deferred — requires running ogen HTTP server with full routing

### 5.2 Two-Tier Minimum

The handlers are tested at the **unit tier** via direct method invocation with real `session.Manager` and `session.Store` instances (no mocks). This follows the established pattern in `test/unit/kubernautagent/server/adversarial_http_test.go`.

**Tier skip rationale (Integration)**: The handler methods perform no I/O beyond in-memory session store access. The session Manager/Store integration boundary is thoroughly tested by PR1 integration tests (21 tests). Adding a second tier here would duplicate PR1 coverage without additional confidence. The ogen HTTP routing layer is tested by ogen's own test suite.

### 5.3 Business Outcome Quality Bar

Each test validates an observable operator outcome: what HTTP response (status code + body) does the operator receive when interacting with the cancel/snapshot/stream APIs?

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All 8 new unit tests pass
2. All 59 existing tests pass (zero regressions)
3. Handler code coverage >=80% on new handler methods
4. `go build ./...` succeeds
5. `go vet ./...` clean

**FAIL** — any of the following:

1. Any P0 test fails
2. Coverage below 80% on handler.go new methods
3. Existing tests regress
4. Build or vet errors

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Ogen regeneration fails (OAS spec invalid)
- Build broken after regen
- Ogen CLI upgrade introduces breaking changes

**Resume testing when**:

- OAS spec corrected and regen succeeds
- Build restored to green
- Ogen CLI version aligned

---

## 6. Test Items

### 6.1 Unit-Testable Code (handler methods)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/server/handler.go` | Cancel handler, Snapshot handler, `mapSessionStatusToAPI` fix | ~60 new |

### 6.2 Integration-Testable Code

Not applicable for this PR — see Section 5.2 Tier Skip Rationale.

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feature/pr2-oas-endpoints` HEAD | Branched from `development/v1.5` |
| Dependency: ogen | v1.20.1 (go.mod) | CLI to be upgraded to match |
| Dependency: PR1 infrastructure | `feature/pr1-session-cancel-infra` merged | `CancelInvestigation`, `StatusCancelled`, `ErrSessionTerminal` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SESSION-003 | Running investigation cancelled via HTTP | P0 | Unit | UT-KA-823-OAS-001 | Pending |
| BR-SESSION-003 | Cancel nonexistent session returns 404 | P0 | Unit | UT-KA-823-OAS-002 | Pending |
| BR-SESSION-003 | Cancel already-terminal session returns 409 | P0 | Unit | UT-KA-823-OAS-003 | Pending |
| BR-SESSION-002 | Cancelled session state retrievable via snapshot | P0 | Unit | UT-KA-823-OAS-004 | Pending |
| BR-SESSION-002 | Running session snapshot returns 409 | P1 | Unit | UT-KA-823-OAS-005 | Pending |
| BR-SESSION-002 | Nonexistent session snapshot returns 404 | P1 | Unit | UT-KA-823-OAS-006 | Pending |
| BR-SESSION-002 | Status endpoint shows "cancelled" instead of "unknown" | P0 | Unit | UT-KA-823-OAS-007 | Pending |
| BR-SESSION-007 | Stream endpoint returns 501 stub | P1 | Unit | UT-KA-823-OAS-008 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-KA-823-OAS-{SEQUENCE}` (unit tests, Kubernaut Agent, issue #823, OAS endpoints)

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/server/handler.go` — cancel handler, snapshot handler, `mapSessionStatusToAPI` fix. Target >=80% coverage on new methods.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-823-OAS-001` | Operator cancels a running investigation and receives 200 with session_id and "cancelled" status | Pending |
| `UT-KA-823-OAS-002` | Operator attempting to cancel a nonexistent session receives 404 RFC 7807 error with session ID in detail | Pending |
| `UT-KA-823-OAS-003` | Operator attempting to cancel an already-completed session receives 409 RFC 7807 error explaining terminal state | Pending |
| `UT-KA-823-OAS-004` | Operator retrieves snapshot of a cancelled session and receives 200 with session state (id, status, metadata, created_at) | Pending |
| `UT-KA-823-OAS-005` | Operator requesting snapshot of a running session receives 409 indicating session still in progress | Pending |
| `UT-KA-823-OAS-006` | Operator requesting snapshot of a nonexistent session receives 404 RFC 7807 error | Pending |
| `UT-KA-823-OAS-007` | Status endpoint reports "cancelled" for cancelled session (previously reported "unknown") | Pending |
| `UT-KA-823-OAS-008` | Stream endpoint returns 501 Not Implemented (contract defined, implementation deferred to PR 4) | Pending |

### Tier Skip Rationale

- **Integration**: Handlers use in-memory session store with no I/O boundaries. PR1 integration tests (21 tests) cover the Manager/Store interaction layer. Adding integration tests for handler → Manager calls would duplicate coverage.
- **E2E**: Requires running ogen HTTP server infrastructure; deferred to full v1.5 integration testing.

---

## 9. Test Cases

### UT-KA-823-OAS-001: Cancel running session returns 200

**BR**: BR-SESSION-003
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/server/cancel_snapshot_test.go`

**Preconditions**:
- Session Manager with a running investigation (blocking on context)

**Test Steps**:
1. **Given**: An investigation is started and reaches `StatusRunning`
2. **When**: Handler's cancel method is called with the session ID
3. **Then**: Response is 200 OK with JSON body containing `session_id` and `status: "cancelled"`

**Expected Results**:
1. Response type assertion succeeds for 200 OK type
2. Response body contains the correct `session_id`
3. Response body `status` field equals `"cancelled"`

### UT-KA-823-OAS-002: Cancel nonexistent session returns 404

**BR**: BR-SESSION-003, BR-HAPI-200
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/server/cancel_snapshot_test.go`

**Preconditions**:
- Empty session store

**Test Steps**:
1. **Given**: No sessions exist
2. **When**: Cancel is called with a nonexistent session ID
3. **Then**: Response is 404 with RFC 7807 fields (type, title, detail, status, instance)

**Expected Results**:
1. Response type assertion succeeds for 404 Not Found type
2. `type` field is `"https://kubernaut.ai/problems/not-found"`
3. `detail` field contains the session ID
4. `instance` field contains the endpoint path

### UT-KA-823-OAS-003: Cancel already-terminal session returns 409

**BR**: BR-SESSION-003, BR-HAPI-200
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/server/cancel_snapshot_test.go`

**Preconditions**:
- Session that has already completed

**Test Steps**:
1. **Given**: An investigation has completed successfully
2. **When**: Cancel is called on the completed session
3. **Then**: Response is 409 Conflict with RFC 7807 fields explaining terminal state

**Expected Results**:
1. Response type assertion succeeds for 409 Conflict type
2. `detail` field indicates the session is already terminal
3. `status` field equals 409

### UT-KA-823-OAS-004: Snapshot of cancelled session returns 200

**BR**: BR-SESSION-002
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/server/cancel_snapshot_test.go`

**Preconditions**:
- Session that has been cancelled (via cancel handler first)

**Test Steps**:
1. **Given**: An investigation was started and then cancelled
2. **When**: Snapshot is requested for the cancelled session
3. **Then**: Response is 200 with session state including id, status, metadata, and created_at

**Expected Results**:
1. Response type assertion succeeds for 200 OK type
2. `session_id` matches the cancelled session
3. `status` equals `"cancelled"`
4. `metadata` includes `incident_id` from the original request
5. `created_at` is a valid RFC3339 timestamp

### UT-KA-823-OAS-005: Snapshot of running session returns 409

**BR**: BR-SESSION-002
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/server/cancel_snapshot_test.go`

**Test Steps**:
1. **Given**: An investigation is currently running
2. **When**: Snapshot is requested for the running session
3. **Then**: Response is 409 Conflict with detail indicating session is in progress

### UT-KA-823-OAS-006: Snapshot of nonexistent session returns 404

**BR**: BR-SESSION-002, BR-HAPI-200
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/server/cancel_snapshot_test.go`

**Test Steps**:
1. **Given**: No sessions exist
2. **When**: Snapshot is requested for a nonexistent session ID
3. **Then**: Response is 404 with RFC 7807 fields

### UT-KA-823-OAS-007: Status reports "cancelled" for cancelled session

**BR**: BR-SESSION-002
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/server/cancel_snapshot_test.go`

**Test Steps**:
1. **Given**: A session has been cancelled
2. **When**: The existing status endpoint is queried
3. **Then**: Response JSON `status` field equals `"cancelled"` (not `"unknown"`)

### UT-KA-823-OAS-008: Stream stub returns 501

**BR**: BR-SESSION-007
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/server/cancel_snapshot_test.go`

**Test Steps**:
1. **Given**: Stream endpoint is defined in OAS but not yet implemented
2. **When**: The stream handler method is called
3. **Then**: Returns `ht.ErrNotImplemented` error (ogen translates to 501)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None — real `session.Manager` and `session.Store` instances
- **Location**: `test/unit/kubernautagent/server/`
- **Dependencies**: `pkg/agentclient/` (ogen-generated types for response assertions)

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.24+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| ogen | v1.20.1 | OpenAPI code generation (CLI must match go.mod) |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| PR1 session infrastructure | Code | Merged to development/v1.5 | Cancel/snapshot handlers depend on `CancelInvestigation`, `GetSession`, `ErrSessionTerminal` | N/A — already available |
| PR1.5 audit trail | Code | Merged to development/v1.5 | Audit emission for cancel; not directly tested in PR2 | N/A — already available |
| ogen CLI v1.20.1 | Tool | Upgrade needed (currently v1.18.0) | Regen may produce incompatible code | `go install github.com/ogen-go/ogen/cmd/ogen@v1.20.1` |

### 11.2 Execution Order

1. **Phase 0**: CHECKPOINT 0 — Baseline verification (build, all 59 tests pass)
2. **Phase 1**: TDD RED — Write 8 failing handler tests with ogen type stubs
3. **Phase 2**: CHECKPOINT 1 — Test plan quality gate
4. **Phase 3**: TDD GREEN — Update OAS spec, regen ogen, implement handlers
5. **Phase 4**: CHECKPOINT 2 — GREEN quality gate
6. **Phase 5**: TDD REFACTOR — GoDoc, error messages, documentation
7. **Phase 6**: CHECKPOINT 3 — Final gate

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/823/TP-823-OAS-ENDPOINTS.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/server/cancel_snapshot_test.go` | Ginkgo BDD test file |
| Coverage report | CI artifact | Coverage percentages for handler.go |

---

## 13. Execution

```bash
# Unit tests (server handlers)
go test ./test/unit/kubernautagent/server/... -ginkgo.v

# Coverage (handler.go)
go test ./test/unit/kubernautagent/server/... -coverprofile=coverage.out \
  -coverpkg=github.com/jordigilh/kubernaut/internal/kubernautagent/server
go tool cover -func=coverage.out

# Full regression
go test ./test/unit/kubernautagent/... ./test/integration/kubernautagent/session/... -count=1
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | N/A | N/A | All new endpoints are additive; existing handler interface methods are unchanged by ogen regen |

---

## 15. Due Diligence Findings

### F1: `mapSessionStatusToAPI` does not handle `StatusCancelled` (SEVERITY: HIGH)

`StatusCancelled` falls through to `default` and returns `"unknown"`. Both the status endpoint and result endpoint misreport cancelled sessions.

**Mitigation**: Add `case session.StatusCancelled: return "cancelled"` in handler.go. Test UT-KA-823-OAS-007 validates this fix.

### F2: `x-ogen-raw-response` is not a supported ogen extension (SEVERITY: HIGH for plan, NONE for PR2)

The v1.5 plan assumed `x-ogen-raw-response: true` would generate a `RawHandler` method with `http.ResponseWriter` access. This extension does not exist in ogen. Ogen handles `text/event-stream` by generating a response struct with `io.Reader` field.

**Impact on PR2**: None — the stream endpoint is a 501 stub via `UnimplementedHandler`.
**Impact on PR4**: SSE implementation will need `io.Pipe()` with flush middleware, or register the stream route directly on the chi mux. Documented as a PR4 design decision.

### F3: Ogen CLI version mismatch (SEVERITY: MEDIUM)

CLI is v1.18.0, go.mod has v1.20.1. Generated code may be incompatible with the runtime library.

**Mitigation**: Run `go install github.com/ogen-go/ogen/cmd/ogen@v1.20.1` before regeneration.

### F4: Error response pattern inconsistency in existing spec (SEVERITY: LOW)

Existing endpoints mix inline HTTPError definitions and `$ref: HTTPError`. Some use `application/problem+json`, others use both `application/problem+json` and `application/json`.

**Mitigation**: New endpoints consistently use `$ref: "#/components/schemas/HTTPError"` with `application/problem+json` only.

### F5: Snapshot scope for PR2 vs PR3 (SEVERITY: LOW)

PR2 snapshot returns basic session state (id, status, metadata, created_at, error). PR3 extends snapshot with `CancelledResult` fields (messages, turn, phase, tokens).

**Mitigation**: PR2 schema is minimal and extensible. PR3 adds fields without breaking changes.

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-24 | Initial test plan with due diligence findings |
