# Test Plan: Pagination Exemption from Per-Tool Call Budget (#860)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-860-v1.0
**Feature**: Exempt cursor-based pagination calls from the per-tool anomaly budget; raise `MaxToolCallsPerTool` default from 5 to 10
**Version**: 1.0
**Created**: 2026-04-26
**Author**: AI Agent (Cursor)
**Status**: Draft
**Branch**: `release/v1.3.2`

---

## 1. Introduction

### 1.1 Purpose

Validates that the anomaly detector correctly exempts pagination calls (identified by tool name + non-empty `cursor` argument) from the per-tool call counter while still enforcing the `MaxTotalToolCalls` safety net. Also validates the raised `MaxToolCallsPerTool` default (5 -> 10) and updated prompt guidance.

### 1.2 Objectives

1. **Pagination exemption correctness**: Pagination calls to `list_workflows` and `list_available_actions` do not increment per-tool counters
2. **Safety net preservation**: Pagination calls still count toward `MaxTotalToolCalls=30`
3. **Fail-closed security**: Malformed, empty, or nil args are treated as non-pagination (counted normally)
4. **Default alignment**: Both `DefaultAnomalyConfig()` and `config.DefaultConfig().Anomaly` reflect `MaxToolCallsPerTool=10`
5. **Integration correctness**: Full `executeTool` path allows pagination-heavy workflows without false rejection

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/investigator/... --ginkgo.focus="860"` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/investigator/... --ginkgo.focus="860"` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `anomaly.go` |
| Backward compatibility | 0 regressions | Existing anomaly tests pass after limit update |

---

## 2. References

### 2.1 Authority (governing documents)

- [DD-HAPI-019-003](../../architecture/decisions/DD-HAPI-019-go-rewrite-design/DD-HAPI-019-003-security-architecture.md): I7 Behavioral Anomaly Detection — canonical spec for per-tool/total limits
- [DD-WORKFLOW-016](../../architecture/decisions/DD-WORKFLOW-016-action-type-workflow-indexing.md): Workflow discovery pagination security considerations
- [BR-HAPI-433-004](../../requirements/BR-HAPI-433-go-language-migration/BR-HAPI-433-004-security-requirements.md): I7 configurable per-tool invocation cap
- Issue #860: `list_workflows` per-tool call limit too restrictive for paginated catalogs

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [TP-433-WIR](../433/TP-433-WIR-v1.0.md): Original anomaly detector test plan (I7)
- [TP-688](../688/TEST_PLAN.md): Workflow pagination test plan (scope note update needed)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Malformed JSON in `isPaginationCall` causes panic | Investigation crash | Low | UT-KA-860-004 | Fail-closed: `json.Unmarshal` error returns false |
| R2 | Pagination exemption bypasses `MaxTotalToolCalls` | Unbounded LLM tool usage | Medium | UT-KA-860-003 | Pagination increments `totalCallCount`; only per-tool count exempt |
| R3 | LLM crafts fake cursor to exploit exemption | Per-tool limit evasion for non-paginated tools | Low | UT-KA-860-004 | Tool-name-aware: only `list_workflows` and `list_available_actions` qualify |
| R4 | Raising limit to 10 doubles DoS surface | More tool calls per investigation | Low | UT-KA-860-002 | `MaxTotalToolCalls=30` bounds total; catalog data is small (DD-WORKFLOW-016) |
| R5 | Default drift between `anomaly.go` and `config.go` | Config mismatch in production | Medium | UT-KA-860-005, UT-KA-860-006 | Both defaults tested independently |

### 3.1 Risk-to-Test Traceability

- **R1 (panic)**: UT-KA-860-004 tests malformed JSON, nil args, empty args
- **R2 (total bypass)**: UT-KA-860-003 verifies pagination calls hit `MaxTotalToolCalls`
- **R3 (fake cursor)**: UT-KA-860-004 tests non-paginated tool with cursor (should still count)
- **R4 (DoS surface)**: UT-KA-860-002 verifies enforcement at new limit=10
- **R5 (config drift)**: UT-KA-860-005 + UT-KA-860-006 independently assert both defaults

---

## 4. Scope

### 4.1 Features to be Tested

- **`isPaginationCall`** (`internal/kubernautagent/investigator/anomaly.go`): New function — determines whether a tool call is a pagination continuation based on tool name (`list_workflows`, `list_available_actions`) and non-empty `cursor` argument
- **`CheckToolCall` update** (`internal/kubernautagent/investigator/anomaly.go`): Pagination calls skip `toolCallCounts[name]++` but still increment `totalCallCount`
- **`DefaultAnomalyConfig()`** (`internal/kubernautagent/investigator/anomaly.go`): `MaxToolCallsPerTool` changed from 5 to 10
- **`DefaultConfig().Anomaly`** (`internal/kubernautagent/config/config.go`): `MaxToolCallsPerTool` changed from 5 to 10
- **`executeTool` integration path** (`internal/kubernautagent/investigator/investigator.go`): Validates pagination-heavy tool call sequences pass through without false rejection

### 4.2 Features Not to be Tested

- **Prompt template changes** (`phase3_workflow_selection.tmpl`): Text-only change; validated by manual review, not programmatic assertion
- **DataStorage pagination server-side**: Covered by TP-688
- **Cursor encoding/validation**: Covered by DD-WORKFLOW-016 / DS tests

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Tool-name-aware `isPaginationCall` | Only `list_workflows` and `list_available_actions` have cursor pagination (DD-WORKFLOW-016). Prevents LLM exploiting cursor field on arbitrary tools. |
| Pagination still counts toward `MaxTotalToolCalls` | Defense-in-depth: prevents unbounded pagination loops. 30-call total is the safety net. |
| No separate `MaxPaginationCalls` cap | Catalog data is small (~10 action types, ~10s workflows per type). `MaxTotalToolCalls=30` provides sufficient bound. |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `isPaginationCall` and `CheckToolCall` pagination paths in `anomaly.go`
- **Integration**: >=80% of `executeTool` pagination flow in `investigator.go`
- **E2E**: Deferred — anomaly detector is tested at unit + integration tiers

### 5.2 Two-Tier Minimum

Every behavior is covered by at least unit + integration:
- Unit: Tests `isPaginationCall` logic and `CheckToolCall` branching in isolation
- Integration: Tests full `executeTool` path with mock LLM producing pagination sequences

### 5.3 Pass/Fail Criteria

**PASS** — all of:
1. All 7 unit tests and 1 integration test pass
2. >=80% statement coverage on `isPaginationCall` and `CheckToolCall` pagination branch
3. All existing anomaly tests pass after limit update (0 regressions)
4. `go build ./...` and `go vet ./...` clean

**FAIL** — any of:
1. Any test fails
2. Coverage below 80% on changed code
3. Existing tests regress

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/anomaly.go` | `isPaginationCall`, `CheckToolCall` (pagination branch) | ~15 new |
| `internal/kubernautagent/investigator/anomaly.go` | `DefaultAnomalyConfig()` | 1 line change |
| `internal/kubernautagent/config/config.go` | `DefaultConfig()` | 1 line change |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `executeTool` (anomaly check path) | ~5 (existing, behavior change) |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-433-004 (I7) | Pagination calls exempt from per-tool budget | P0 | Unit | UT-KA-860-001 | Pending |
| BR-HAPI-433-004 (I7) | Non-pagination calls enforced at limit=10 | P0 | Unit | UT-KA-860-002 | Pending |
| BR-HAPI-433-004 (I7) | Pagination calls count toward total budget | P0 | Unit | UT-KA-860-003 | Pending |
| BR-HAPI-433-004 (I7) | Malformed/edge-case args fail-closed | P0 | Unit | UT-KA-860-004 | Pending |
| BR-HAPI-433-004 (I7) | DefaultAnomalyConfig reflects limit=10 | P1 | Unit | UT-KA-860-005 | Pending |
| BR-HAPI-433-004 (I7) | DefaultConfig().Anomaly reflects limit=10 | P1 | Unit | UT-KA-860-006 | Pending |
| BR-HAPI-433-004 (I7) | Full executeTool allows pagination-heavy sequence | P0 | Integration | IT-KA-860-001 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/investigator/anomaly.go` — `isPaginationCall`, `CheckToolCall` pagination path, `DefaultAnomalyConfig()`; `internal/kubernautagent/config/config.go` — `DefaultConfig().Anomaly`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-860-001 | Pagination call (list_workflows with cursor) does NOT increment per-tool count; allows >limit calls | Pending |
| UT-KA-860-002 | Non-pagination call increments per-tool count and rejects at limit=10 (11th call rejected) | Pending |
| UT-KA-860-003 | Pagination calls still count toward MaxTotalToolCalls=30 (safety net enforced) | Pending |
| UT-KA-860-004 | isPaginationCall edge cases: nil args, empty args, malformed JSON, empty cursor, non-paginated tool with cursor — all return false (fail-closed) | Pending |
| UT-KA-860-005 | DefaultAnomalyConfig().MaxToolCallsPerTool == 10 | Pending |
| UT-KA-860-006 | config.DefaultConfig().Anomaly.MaxToolCallsPerTool == 10 | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `internal/kubernautagent/investigator/investigator.go` — `executeTool` with `AnomalyDetector` wired

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-KA-860-001 | Full executeTool path allows 12 list_workflows calls (5 new + 7 pagination) without per-tool rejection | Pending |

### Tier Skip Rationale

- **E2E**: Anomaly detector is fully exercised at unit + integration tiers. E2E would require Kind cluster + mock LLM scenario for pagination, which is disproportionate cost for this change.

---

## 9. Test Cases

### UT-KA-860-001: Pagination call does not increment per-tool count

**BR**: BR-HAPI-433-004 (I7)
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/anomaly_test.go`

**Test Steps**:
1. **Given**: AnomalyDetector with MaxToolCallsPerTool=3, MaxTotalToolCalls=100
2. **When**: Call CheckToolCall("list_workflows", `{"action_type":"cordon","cursor":"abc123"}`) 10 times
3. **Then**: All 10 calls return Allowed=true (cursor calls don't count toward per-tool limit)

### UT-KA-860-002: Non-pagination call enforced at raised limit

**BR**: BR-HAPI-433-004 (I7)
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/anomaly_test.go`

**Test Steps**:
1. **Given**: AnomalyDetector with MaxToolCallsPerTool=10, MaxTotalToolCalls=100
2. **When**: Call CheckToolCall("kubectl_describe", `{}`) 11 times
3. **Then**: First 10 return Allowed=true; 11th returns Allowed=false with "per-tool call limit exceeded"

### UT-KA-860-003: Pagination calls count toward total budget

**BR**: BR-HAPI-433-004 (I7)
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/anomaly_test.go`

**Test Steps**:
1. **Given**: AnomalyDetector with MaxToolCallsPerTool=100, MaxTotalToolCalls=5
2. **When**: Call CheckToolCall("list_workflows", `{"action_type":"cordon","cursor":"abc"}`) 6 times
3. **Then**: First 5 return Allowed=true; 6th returns Allowed=false with "total tool call limit exceeded"

### UT-KA-860-004: isPaginationCall edge cases (fail-closed)

**BR**: BR-HAPI-433-004 (I7)
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/anomaly_test.go`

**Test Steps** (table-driven):

| Sub-case | Tool name | Args | Expected isPagination | Rationale |
|----------|-----------|------|-----------------------|-----------|
| nil args | list_workflows | nil | false | Fail-closed on nil |
| empty args | list_workflows | `{}` | false | No cursor field |
| malformed JSON | list_workflows | `{bad` | false | Fail-closed on parse error |
| empty cursor | list_workflows | `{"cursor":""}` | false | Empty cursor = initial call |
| non-paginated tool with cursor | kubectl_describe | `{"cursor":"abc"}` | false | Tool not in allow-list |
| list_available_actions with cursor | list_available_actions | `{"cursor":"abc"}` | true | Both paginated tools covered |
| list_workflows with cursor | list_workflows | `{"cursor":"abc"}` | true | Primary paginated tool |

### UT-KA-860-005: DefaultAnomalyConfig reflects limit=10

**BR**: BR-HAPI-433-004 (I7)
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/anomaly_test.go`

**Test Steps**:
1. **Given**: Call `DefaultAnomalyConfig()`
2. **Then**: `MaxToolCallsPerTool == 10`, `MaxTotalToolCalls == 30`, `MaxRepeatedFailures == 3`

### UT-KA-860-006: DefaultConfig().Anomaly reflects limit=10

**BR**: BR-HAPI-433-004 (I7)
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/config/config_test.go`

**Test Steps**:
1. **Given**: Call `config.DefaultConfig()`
2. **Then**: `cfg.Anomaly.MaxToolCallsPerTool == 10`

### IT-KA-860-001: executeTool allows pagination-heavy sequence

**BR**: BR-HAPI-433-004 (I7)
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_anomaly_test.go`

**Test Steps**:
1. **Given**: Investigator with AnomalyDetector (MaxToolCallsPerTool=10, MaxTotalToolCalls=100), mock LLM that issues 12 list_workflows tool calls (5 initial + 7 with cursor)
2. **When**: Investigate is called
3. **Then**: No tool call is rejected with "per-tool call limit exceeded"; investigation completes normally

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None (AnomalyDetector is pure logic)
- **Location**: `test/unit/kubernautagent/investigator/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: Mock LLM client (existing `mockLLMClient` pattern), fake tool registry
- **Location**: `test/integration/kubernautagent/investigator/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Cherry-picked #852/#853 | Code | Merged to release/v1.3.2 | None — independent code paths | N/A |
| Cherry-picked B+D fix | Code | Merged to release/v1.3.2 | None — independent code paths | N/A |

### 11.2 Execution Order

1. **TDD RED**: Write UT-KA-860-001..006 + IT-KA-860-001 (all fail)
2. **TDD GREEN**: Implement `isPaginationCall`, update `CheckToolCall`, change defaults, update prompt
3. **TDD REFACTOR**: Coverage check, 100-go-mistakes audit, anti-pattern check

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/860/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/investigator/anomaly_test.go` | 6 new Ginkgo specs |
| Config unit test update | `test/unit/kubernautagent/config/config_test.go` | 1 updated spec |
| Integration test suite | `test/integration/kubernautagent/investigator/investigator_anomaly_test.go` | 1 new Ginkgo spec |

---

## 13. Execution

```bash
# Unit tests (focused)
go test ./test/unit/kubernautagent/investigator/... --ginkgo.focus="860"

# Config unit tests
go test ./test/unit/kubernautagent/config/... --ginkgo.focus="860"

# Integration tests (focused)
go test ./test/integration/kubernautagent/investigator/... --ginkgo.focus="860"

# Coverage
go test ./test/unit/kubernautagent/investigator/... -coverprofile=coverage.out -coverpkg=github.com/jordigilh/kubernaut/internal/kubernautagent/investigator
go tool cover -func=coverage.out | grep -E "isPaginationCall|CheckToolCall"
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `anomaly_test.go:323` `DefaultAnomalyConfig()` | `Expect(cfg.MaxToolCallsPerTool).To(Equal(5))` | Change to `Equal(10)` | Default raised to 10 |
| `config_test.go:75` `DefaultConfig()` | `Expect(cfg.Anomaly.MaxToolCallsPerTool).To(Equal(5))` | Change to `Equal(10)` | Default raised to 10 |
| `config_test.go:266-268` `MaxToolCallsPerTool=5` | Title + assertion reference 5 | Update to 10 | Default raised to 10 |
| `investigator_anomaly_test.go:64-71` `IT-KA-433W-012` | `MaxToolCallsPerTool: 5`, "6th call", 6 tool calls | Change to `MaxToolCallsPerTool: 10`, "11th call", 11 tool calls | Production default alignment |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-26 | Initial test plan |
