# Test Plan: Fix 11 Latent Bugs Found During TP-668 Coverage Audit

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-674-v1
**Feature**: Fix 11 latent bugs in DS, GW, WE, AW uncovered during TP-668 coverage audit
**Version**: 1.0
**Created**: 2026-04-11
**Author**: AI Agent (TP-668 audit)
**Status**: Active
**Branch**: `development/v1.3`

---

## 1. Introduction

### 1.1 Purpose

This test plan provides behavioral assurance for 11 bugs discovered during the TP-668
per-tier coverage audit. The bugs span DataStorage (9 bugs), Gateway (1 bug shared with
WE and AW), and cross-service configuration loading (1 pattern repeated in 3 services).
Each bug was confirmed through static analysis and reproduction reasoning during an
8-track parallel due diligence audit.

### 1.2 Objectives

1. **Bug regression prevention**: Every bug has at least one failing test (TDD RED) that
   proves the defect exists before any fix is applied.
2. **Behavioral assurance**: Tests validate business outcomes (correct SQL, valid configs,
   safe metrics) not just code paths.
3. **Per-tier coverage**: Maintain >=80% UT-tier coverage for all affected services after
   fixes are applied.
4. **Zero regressions**: All pre-existing tests continue to pass.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/datastorage/... ./test/unit/gateway/... ./test/unit/workflowexecution/... ./test/unit/authwebhook/...` |
| RED phase test failures | 35/35 | All new tests fail before fixes |
| GREEN phase test passes | 35/35 | All new tests pass after minimal fixes |
| DS UT-tier coverage | >=80% | `scripts/coverage/coverage_report.py` |
| GW UT-tier coverage | >=80% | `scripts/coverage/coverage_report.py` |
| WE UT-tier coverage | >=80% | `scripts/coverage/coverage_report.py` |
| AW UT-tier coverage | >=80% | `scripts/coverage/coverage_report.py` |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-STORAGE-010: Comprehensive input validation for data storage operations
- BR-STORAGE-019: Logging, metrics, and observability for data storage
- BR-STORAGE-020: Data integrity and consistency guarantees
- BR-PLATFORM-003: Service configuration management (ADR-030)
- BR-SECURITY-001: Defense against injection and information disclosure
- Issue #674: 11 latent bugs found during TP-668 coverage audit
- Issue #673: Security hardening (cross-referenced, not in scope)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Bug 1 SQL corruption triggers in production when >10 query params are used | Data loss or query failure against PostgreSQL | Medium | UT-DS-674-001 through 004 | Remove placeholder round-trip; emit native $N |
| R2 | Bug 4 LoadFromFile fix changes behavior for callers expecting silent defaults | Service startup failures if config file missing | Medium | UT-GW-674-001 through 003, UT-WE-674-001, UT-AW-674-001 | Return wrapped error with file path context; update callers |
| R3 | Bug 7 nil panic crashes DataStorage pod in production | Service outage | High | UT-DS-674-016, 017 | Add nil guard before any dereference |
| R4 | Bug 9 unbounded cardinality causes Prometheus OOM | Monitoring infrastructure failure | High | UT-DS-674-023, 024 | Map to bounded enum set |
| R5 | Bug 11 data race detected by Go race detector | Undefined behavior, data corruption | Medium | UT-DS-674-029 through 031 | Deep-copy struct before goroutine capture |
| R6 | Existing tests break due to signature changes | False negatives in CI | Low | All | Run full test suite after each GREEN fix |

### 3.1 Risk-to-Test Traceability

- **R1 (High)**: UT-DS-674-001, UT-DS-674-002, UT-DS-674-003, UT-DS-674-004
- **R2 (Medium)**: UT-GW-674-001, UT-GW-674-002, UT-GW-674-003, UT-WE-674-001, UT-AW-674-001
- **R3 (High)**: UT-DS-674-016, UT-DS-674-017
- **R4 (High)**: UT-DS-674-023, UT-DS-674-024
- **R5 (Medium)**: UT-DS-674-029, UT-DS-674-030, UT-DS-674-031
- **R6 (Low)**: Full regression suite

---

## 4. Scope

### 4.1 Features to be Tested

- **Query Builder** (`pkg/datastorage/query/builder.go`): SQL placeholder handling for >10 parameters
- **Validation Engine** (`pkg/datastorage/validation/rules.go`, `validator.go`): Custom rules wiring, Status field validation
- **ActionTrace Model** (`pkg/datastorage/models/action_trace.go`): Struct validation enforcement
- **Config Loaders** (`pkg/gateway/config/config.go`, `pkg/workflowexecution/config/config.go`, `pkg/authwebhook/config/config.go`): Error propagation from LoadFromFile
- **Time Parser** (`pkg/datastorage/query/time_parser.go`): Negative duration handling
- **Workflow Status** (`pkg/datastorage/models/workflow.go`): Case-insensitive status comparison
- **Audit Event Factory** (`pkg/datastorage/audit/workflow_catalog_event.go`): Nil pointer safety
- **Schema Validator** (`pkg/datastorage/schema/validator.go`): PostgreSQL size parsing for bare integers
- **Metrics Labels** (`pkg/datastorage/server/audit_events_handler.go`): Bounded cardinality for Prometheus labels
- **Async Audit** (`pkg/datastorage/server/workflow_handlers.go`): Race-free struct access in goroutines

### 4.2 Features Not to be Tested

- **DS-SEC-001 through DS-SEC-007**: Security findings cross-referenced to issue #673 (separate scope)
- **Integration-tier tests**: Bug fixes are pure-logic changes testable at UT tier; IT coverage maintained by existing tests
- **E2E tests**: Deferred; UT tier provides sufficient behavioral assurance for these bugs

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| UT-only for all 11 bugs | All bugs are in pure-logic code (parsers, validators, builders, models) — no I/O needed |
| Table-driven tests for multi-case bugs | Bugs 1, 4, 5, 6, 8 have multiple input variants best covered by table-driven Ginkgo entries |
| Single test file per service | `test/unit/{service}/bugs_674_test.go` keeps tests grouped and discoverable |
| No mocks needed for DS bugs | All DS bugs are in pure functions or methods with no external dependencies |
| httptest not needed | Unlike TP-668 coverage tests, these bugs are in internal logic, not HTTP handlers |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code for DS, GW, WE, AW services
- **Integration**: Not directly targeted (existing IT tests unaffected)
- **E2E**: Not applicable for this issue

### 5.2 Two-Tier Minimum

Each bug is covered at the UT tier. IT-tier coverage is maintained by existing integration
tests that exercise the fixed code paths. The two-tier minimum is satisfied because:
- UT tests prove the fix is correct in isolation
- Existing IT tests exercise the same code paths in integration context

### 5.3 Business Outcome Quality Bar

Each test validates a business outcome:
- "SQL with 11+ parameters produces correct PostgreSQL syntax" (not "function returns string")
- "Malformed config file returns an error to the operator" (not "function calls os.ReadFile")
- "Nil workflow input returns error instead of crashing the service" (not "nil check exists")

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All 35 tests pass (0 failures)
2. All 4 affected services maintain >=80% UT-tier coverage
3. No regressions in existing test suites
4. `go build ./...` succeeds
5. `make lint-test-patterns` passes (no anti-patterns)

**FAIL** — any of the following:

1. Any test fails after GREEN phase
2. Per-tier coverage falls below 80% for any affected service
3. Existing tests that were passing before the change now fail
4. Build errors introduced

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Build broken: code does not compile after a fix
- Cascading failures: fix for one bug breaks tests for another bug
- Issue #673 security fixes conflict with changes in this issue

**Resume testing when**:

- Build fixed and green on CI
- Conflict resolved with explicit merge strategy
- #673 changes integrated into branch

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/query/builder.go` | `convertToStandardPlaceholders`, `Build` | ~20 |
| `pkg/datastorage/validation/rules.go` | `NewValidatorWithRules` | ~10 |
| `pkg/datastorage/validation/validator.go` | `ValidateRemediationAudit`, `isValidPhase` | ~40 |
| `pkg/datastorage/models/action_trace.go` | `Validate` | ~5 |
| `pkg/datastorage/models/workflow.go` | `IsActive`, `IsDisabled`, `IsDeprecated`, `IsArchived` | ~20 |
| `pkg/datastorage/query/time_parser.go` | `ParseTimeParam` | ~25 |
| `pkg/datastorage/audit/workflow_catalog_event.go` | `NewWorkflowCreatedAuditEvent` | ~60 |
| `pkg/datastorage/schema/validator.go` | `parsePostgreSQLSize` | ~30 |
| `pkg/datastorage/server/audit_events_handler.go` | Metrics label usage in `handleCreateAuditEvent` | ~10 |
| `pkg/datastorage/server/workflow_handlers.go` | Async audit goroutines | ~25 |
| `pkg/gateway/config/config.go` | `LoadFromFile` | ~30 |
| `pkg/workflowexecution/config/config.go` | `LoadFromFile` | ~20 |
| `pkg/authwebhook/config/config.go` | `LoadFromFile` | ~20 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

No new integration tests planned. Existing IT suites exercise the fixed code paths.

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3` HEAD | Post TP-668 coverage work |
| Issue | #674 | 11 latent bugs |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-STORAGE-010 | Input validation | P0 | Unit | UT-DS-674-005,006,007,008,009,010,025,026,027 | Pending |
| BR-STORAGE-019 | Metrics and observability | P0 | Unit | UT-DS-674-023,024 | Pending |
| BR-STORAGE-020 | Data integrity | P0 | Unit | UT-DS-674-001,002,003,004,016,017,029,030,031 | Pending |
| BR-PLATFORM-003 | Configuration management | P1 | Unit | UT-GW-674-001,002,003, UT-WE-674-001, UT-AW-674-001 | Pending |
| BR-SECURITY-001 | Injection defense | P0 | Unit | UT-DS-674-001,002,003,004 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: `DS` (DataStorage), `GW` (Gateway), `WE` (WorkflowExecution), `AW` (AuthWebhook)
- **BR_NUMBER**: 674 (issue number, mapped to BRs in Section 7)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: All files listed in Section 6.1. Target: >=80% UT-tier coverage maintained.

#### Bug 1: SQL Placeholder Corruption (convertToStandardPlaceholders)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-674-001` | Query with exactly 10 parameters produces correct SQL (boundary) | Pending |
| `UT-DS-674-002` | Query with 11 parameters: $11 must NOT become ?1 (corruption case) | Pending |
| `UT-DS-674-003` | Query with 15 parameters: all placeholders correctly preserved | Pending |
| `UT-DS-674-004` | Round-trip through convertToStandard then convertToPostgreSQL is identity for $1-$20 | Pending |

#### Bug 2: Validator Rules Silently Discarded (NewValidatorWithRules)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-674-005` | Custom MaxNameLength rule is enforced by ValidateRemediationAudit | Pending |
| `UT-DS-674-006` | Custom ValidPhases rule rejects phases not in custom set | Pending |
| `UT-DS-674-007` | Default rules still work when NewValidator (no custom rules) is used | Pending |

#### Bug 3: ActionTrace.Validate() No-Op

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-674-008` | ActionTrace with empty ActionID fails validation | Pending |
| `UT-DS-674-009` | ActionTrace with empty WorkflowID fails validation | Pending |
| `UT-DS-674-010` | Valid ActionTrace passes validation | Pending |

#### Bug 4: LoadFromFile Swallows Errors

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-674-001` | Gateway LoadFromFile with nonexistent path returns error | Pending |
| `UT-GW-674-002` | Gateway LoadFromFile with malformed YAML returns error | Pending |
| `UT-GW-674-003` | Gateway LoadFromFile with valid YAML returns parsed config | Pending |
| `UT-WE-674-001` | WorkflowExecution LoadFromFile with nonexistent path returns error | Pending |
| `UT-AW-674-001` | AuthWebhook LoadFromFile with nonexistent path returns error | Pending |

#### Bug 5: ParseTimeParam Future Timestamps

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-674-011` | ParseTimeParam("-1d") returns error (negative days rejected) | Pending |
| `UT-DS-674-012` | ParseTimeParam("-7d") returns error (negative days rejected) | Pending |
| `UT-DS-674-013` | ParseTimeParam("7d") returns timestamp ~7 days in the past | Pending |
| `UT-DS-674-014` | ParseTimeParam("1d") returns timestamp ~24 hours in the past | Pending |

#### Bug 6: Case-Sensitive Workflow Status

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-674-015` | IsActive returns true for "active" (lowercase) | Pending |
| `UT-DS-674-016` | IsActive returns true for "Active" (canonical) | Pending |
| `UT-DS-674-017` | IsActive returns true for "ACTIVE" (uppercase) | Pending |

#### Bug 7: Nil Workflow Panic

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-674-018` | NewWorkflowCreatedAuditEvent(nil) returns error, does not panic | Pending |
| `UT-DS-674-019` | NewWorkflowCreatedAuditEvent with valid workflow returns audit event | Pending |

#### Bug 8: parsePostgreSQLSize Bare Integer

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-674-020` | parsePostgreSQLSize("16384") interprets as 8kB blocks (128 MB), not 16384 MB | Pending |
| `UT-DS-674-021` | parsePostgreSQLSize("128MB") returns 134217728 bytes (correct) | Pending |
| `UT-DS-674-022` | parsePostgreSQLSize("1GB") returns 1073741824 bytes (correct) | Pending |

#### Bug 9: Unbounded Prometheus Label Cardinality

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-674-023` | Known event categories (e.g., "remediation") are used as label values | Pending |
| `UT-DS-674-024` | Unknown event category is mapped to "other" (bounded set) | Pending |

#### Bug 10: Missing Status Validation

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-674-025` | ValidateRemediationAudit rejects audit with invalid Status | Pending |
| `UT-DS-674-026` | ValidateRemediationAudit accepts audit with valid Status | Pending |
| `UT-DS-674-027` | DefaultRules().ValidStatuses contains all expected statuses | Pending |

#### Bug 11: Data Race in Async Audit

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-674-028` | Workflow struct is not modified by audit goroutine (deep copy verified) | Pending |
| `UT-DS-674-029` | Async audit does not block HTTP response | Pending |

### Tier Skip Rationale

- **Integration**: All 11 bugs are in pure-logic code (parsers, validators, builders, models). Existing IT suites exercise these code paths through handler calls. No new IT tests needed.
- **E2E**: Deferred. UT tier provides sufficient behavioral assurance. E2E would add cost without proportional confidence gain for these specific logic bugs.

---

## 9. Test Cases

### UT-DS-674-001: SQL 10-param boundary

**BR**: BR-STORAGE-020, BR-SECURITY-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/bugs_674_test.go`

**Preconditions**:
- `convertToStandardPlaceholders` function accessible (may need export or test in same package)

**Test Steps**:
1. **Given**: SQL string with placeholders $1 through $10
2. **When**: `convertToStandardPlaceholders` is called
3. **Then**: All 10 placeholders are replaced with `?`

**Expected Results**:
1. Output contains exactly 10 `?` placeholders
2. No `$N` placeholders remain

**Acceptance Criteria**:
- **Behavior**: Boundary case (10 params) works correctly
- **Correctness**: Each `$N` maps to exactly one `?`

---

### UT-DS-674-002: SQL 11-param corruption

**BR**: BR-STORAGE-020, BR-SECURITY-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/bugs_674_test.go`

**Preconditions**:
- SQL string with $1 through $11

**Test Steps**:
1. **Given**: SQL string `SELECT * FROM t WHERE a=$1 AND b=$2 ... AND k=$11`
2. **When**: `convertToStandardPlaceholders` is called
3. **Then**: $11 must NOT become `?1`

**Expected Results**:
1. All 11 placeholders are correctly handled
2. No corruption where `$11` becomes `?1`

**Acceptance Criteria**:
- **Behavior**: Placeholder integrity maintained for >10 params
- **Correctness**: `$11` maps to `?`, not `?1`
- **Accuracy**: Round-trip through both conversion functions preserves parameter identity

---

### UT-DS-674-018: Nil workflow safety

**BR**: BR-STORAGE-020
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/bugs_674_test.go`

**Preconditions**:
- `NewWorkflowCreatedAuditEvent` callable with nil argument

**Test Steps**:
1. **Given**: A nil `*models.RemediationWorkflow`
2. **When**: `NewWorkflowCreatedAuditEvent(nil)` is called
3. **Then**: Returns non-nil error, does NOT panic

**Expected Results**:
1. Error returned with descriptive message (e.g., "workflow must not be nil")
2. No panic occurs
3. Return value for audit event is nil

**Acceptance Criteria**:
- **Behavior**: Graceful error handling
- **Correctness**: Pod does not crash
- **Accuracy**: Error message identifies the problem

---

### Remaining P0 Test Cases (UT-DS-674-003 through UT-DS-674-029)

Detailed specifications follow the same Given/When/Then pattern as above. Each test case
maps to a specific row in the Section 8 scenario table. Full specifications are available
in the test source code with Ginkgo `Describe`/`It` blocks that serve as executable
specifications.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None required (all bugs are in pure-logic code)
- **Location**: `test/unit/datastorage/bugs_674_test.go`, `test/unit/gateway/bugs_674_test.go`, `test/unit/workflowexecution/bugs_674_test.go`, `test/unit/authwebhook/bugs_674_test.go`
- **Resources**: Standard (no special CPU/memory)

### 10.2 Integration Tests

Not applicable for this issue. Existing IT suites provide coverage.

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| golangci-lint | latest | Lint validation |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #673 | Code | Open | DS-SEC findings overlap; fixes may conflict | Coordinate merge order; #674 first (logic), #673 second (security) |
| TP-668 coverage tests | Code | Merged | New test files already present | No conflict expected |

### 11.2 Execution Order

1. **Phase 1 (TDD RED)**: Write all 29 DS + 5 GW/WE/AW failing tests
2. **Checkpoint 1**: Adversarial audit — verify all tests fail for correct reasons
3. **Phase 2 (TDD GREEN)**: Minimal fixes for each of the 11 bugs
4. **Checkpoint 2**: All tests pass, zero regressions, build succeeds, security review
5. **Phase 3 (TDD REFACTOR)**: Extract patterns, improve error messages, reduce duplication
6. **Checkpoint 3**: Final adversarial + security audit, >=80% UT-tier confirmed

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/674/TEST_PLAN.md` | Strategy and test design |
| DS unit test suite | `test/unit/datastorage/bugs_674_test.go` | 29 Ginkgo BDD tests for 9 DS bugs |
| GW unit test suite | `test/unit/gateway/bugs_674_test.go` | 3 Ginkgo BDD tests for LoadFromFile |
| WE unit test suite | `test/unit/workflowexecution/bugs_674_test.go` | 1 Ginkgo BDD test for LoadFromFile |
| AW unit test suite | `test/unit/authwebhook/bugs_674_test.go` | 1 Ginkgo BDD test for LoadFromFile |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# All bug-674 unit tests
go test ./test/unit/datastorage/... -ginkgo.v -ginkgo.focus="Bug 674"
go test ./test/unit/gateway/... -ginkgo.v -ginkgo.focus="Bug 674"
go test ./test/unit/workflowexecution/... -ginkgo.v -ginkgo.focus="Bug 674"
go test ./test/unit/authwebhook/... -ginkgo.v -ginkgo.focus="Bug 674"

# Specific bug by ID
go test ./test/unit/datastorage/... -ginkgo.focus="UT-DS-674-002"

# Coverage
go test ./test/unit/datastorage/... -coverprofile=ds_coverage.out
go tool cover -func=ds_coverage.out

# Full regression suite
go test ./test/unit/... -count=1
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/datastorage/coverage_668_test.go` (LoadFromFile tests) | Asserts `err == nil` for nonexistent file | Assert `err != nil` after Bug 4 fix | LoadFromFile will now return errors |
| `test/unit/gateway/coverage_668_test.go` (LoadFromFile tests) | Asserts defaults returned, `err == nil` | Assert `err != nil` after Bug 4 fix | LoadFromFile will now return errors |
| `test/unit/datastorage/coverage_668_test.go` (ActionTrace.Validate) | Asserts `Validate()` returns nil | Assert validation errors after Bug 3 fix | Validate() will enforce struct tags |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-11 | Initial test plan — 11 bugs, 34 test scenarios, 3 TDD phases |
