# Test Plan: Tool Output Truncation Safety Net — Context Window Overflow Prevention

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-752-v1
**Feature**: Prevent LLM context window overflow from oversized tool outputs
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/752-kubectl-context-window-overflow`

---

## 1. Introduction

### 1.1 Purpose

Issue #752 reports that `kubectl_get_by_kind_in_cluster` returned 3.4 MB of Secret data, overwhelming both the summarizer's secondary LLM call and the main investigation LLM's context window. This test plan validates three mitigations: (1) a hard truncation safety net in the investigator pipeline, (2) pre-truncation of oversized input before the summarizer's LLM call, and (3) a new targeted tool `kubectl_get_by_name_in_cluster` that lets the LLM fetch a single resource across namespaces without listing everything.

### 1.2 Objectives

1. **Hard truncation safety net**: Tool outputs exceeding `MaxToolOutputSize` (default 100,000 chars) are truncated with a guidance hint before entering the LLM conversation
2. **Summarizer pre-truncation**: Inputs to the summarizer's secondary LLM call are pre-truncated so the summarizer can produce useful summaries instead of failing
3. **Targeted lookup tool**: `kubectl_get_by_name_in_cluster` returns a single resource by name across all namespaces, eliminating the need to list all resources of a kind
4. **Zero regressions**: Existing summarizer, investigator, and K8s tool tests continue to pass

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable files |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-SAFETY-752: Tool output must not exceed LLM context window capacity
- Issue #752: `kubectl_get_by_kind_in_cluster` returned 3.4 MB, overwhelming summarizer and LLM
- DD-HAPI-019-002: llm_summarize transformer design

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Existing summarizer tests: `test/unit/kubernautagent/tools/summarizer/summarizer_test.go`
- Existing K8s tools tests: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`
- Prometheus `TruncateWithHint` pattern: `pkg/kubernautagent/tools/prometheus/tools.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Truncation removes critical diagnostic data | LLM misses root cause | Medium | UT-KA-752-001,002 | Truncation message guides LLM to use targeted tools; hint includes char count |
| R2 | Summarizer pre-truncation produces poor summaries | Degraded investigation quality | Low | UT-KA-752-003,004 | Pre-truncated input still 100K chars — ample for summary extraction |
| R3 | New tool `kubectl_get_by_name_in_cluster` returns empty for nonexistent resource | LLM confusion | Low | UT-KA-752-007,008 | Returns clear "not found" error matching existing tool patterns |
| R4 | MaxToolOutputSize config not loaded from YAML | Default silently used | Low | UT-KA-752-010 | Config test validates YAML parsing and default application |
| R5 | Summarizer failure + truncation double-truncates | Data loss | Low | IT-KA-752-001 | Truncation in executeTool is a final safety net, applied once after summarizer |

### 3.1 Risk-to-Test Traceability

- **R1** (critical data loss): UT-KA-752-001 verifies truncation message includes guidance; UT-KA-752-002 verifies below-limit output passes through unchanged
- **R2** (poor summaries): UT-KA-752-003 verifies summarizer pre-truncates before LLM call; UT-KA-752-004 verifies summarizer still works for moderate-size inputs
- **R3** (empty results): UT-KA-752-007 verifies single-match return; UT-KA-752-008 verifies not-found behavior
- **R5** (double truncation): IT-KA-752-001 verifies end-to-end pipeline with oversized tool output

---

## 4. Scope

### 4.1 Features to be Tested

- **Summarizer pre-truncation** (`pkg/kubernautagent/tools/summarizer/summarizer.go`): `MaybeSummarize` pre-truncates input exceeding `maxInputSize` before secondary LLM call
- **Hard truncation safety net** (`internal/kubernautagent/investigator/investigator.go`): `executeTool` truncates final result exceeding `MaxToolOutputSize`
- **New tool** (`pkg/kubernautagent/tools/k8s/tools.go`): `kubectl_get_by_name_in_cluster` fetches a single resource by name across all namespaces
- **Configuration** (`internal/kubernautagent/config/config.go`): `MaxToolOutputSize` field in `SummarizerConfig`

### 4.2 Features Not to be Tested

- **Blocklist of high-volume kinds** (Mitigation 4): Deferred per user decision
- **E2E tests**: No Kind cluster changes needed; truncation is internal pipeline behavior
- **Audit store changes**: No audit schema changes

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Hard cap at 100,000 chars (default) | ~25K tokens, well within any production LLM context window |
| Truncation hint guides LLM to targeted tools | Prevents repeated oversized fetches; follows Prometheus `TruncateWithHint` pattern |
| Pre-truncation in summarizer uses same limit | Ensures summarizer LLM call stays within context window |
| New tool reuses `ResourceResolver.List` + name filter | Avoids adding new interface methods; consistent with `kubectl_find_resource` pattern |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (summarizer truncation, K8s tool, config parsing)
- **Integration**: >=80% of integration-testable code (investigator pipeline with truncation)
- **E2E**: Not applicable (internal pipeline behavior, no external API changes)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Validate truncation logic, summarizer pre-truncation, new tool behavior, config parsing
- **Integration tests**: Validate end-to-end investigator pipeline with oversized tool outputs

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes:
- "Operator sees investigation complete (not hang/crash) even when a tool returns multi-MB output"
- "LLM receives actionable truncation guidance instead of raw overflow"
- "LLM can fetch a specific resource by name without listing all resources of that kind"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:
1. All P0 tests pass (0 failures)
2. All P1 tests pass
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing test suites
5. `go build ./...` succeeds with zero errors

**FAIL** — any of the following:
1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing tests regress

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build broken: Code does not compile
- Summarizer LLM mock interface changes upstream

**Resume testing when**:
- Build fixed and green on CI

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kubernautagent/tools/summarizer/summarizer.go` | `MaybeSummarize` (pre-truncation) | ~30 |
| `pkg/kubernautagent/tools/k8s/tools.go` | `newGetByNameInCluster`, `Execute` | ~30 |
| `internal/kubernautagent/config/config.go` | `SummarizerConfig.MaxToolOutputSize`, `DefaultConfig` | ~10 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `executeTool` (hard truncation safety net) | ~20 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/752-kubectl-context-window-overflow` HEAD | Branch |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SAFETY-752 | Hard truncation prevents context overflow | P0 | Unit | UT-KA-752-001 | Pending |
| BR-SAFETY-752 | Below-limit output passes through unchanged | P0 | Unit | UT-KA-752-002 | Pending |
| BR-SAFETY-752 | Summarizer pre-truncates before LLM call | P0 | Unit | UT-KA-752-003 | Pending |
| BR-SAFETY-752 | Summarizer works normally for moderate inputs | P0 | Unit | UT-KA-752-004 | Pending |
| BR-SAFETY-752 | Truncation hint includes output size and guidance | P1 | Unit | UT-KA-752-005 | Pending |
| BR-SAFETY-752 | Summarizer pre-truncation note included in prompt | P1 | Unit | UT-KA-752-006 | Pending |
| BR-SAFETY-752 | New tool returns single resource by name across namespaces | P0 | Unit | UT-KA-752-007 | Pending |
| BR-SAFETY-752 | New tool returns error for nonexistent resource | P0 | Unit | UT-KA-752-008 | Pending |
| BR-SAFETY-752 | New tool registered in AllToolNames and phase map | P0 | Unit | UT-KA-752-009 | Pending |
| BR-SAFETY-752 | MaxToolOutputSize parsed from YAML config | P1 | Unit | UT-KA-752-010 | Pending |
| BR-SAFETY-752 | MaxToolOutputSize default applied when absent | P1 | Unit | UT-KA-752-011 | Pending |
| BR-SAFETY-752 | Pipeline truncation with oversized tool output end-to-end | P0 | Integration | IT-KA-752-001 | Pending |
| BR-SAFETY-752 | Pipeline passes through normal-size tool output end-to-end | P0 | Integration | IT-KA-752-002 | Pending |
| BR-SAFETY-752 | Summarizer failure + hard truncation fallback | P0 | Integration | IT-KA-752-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-752-{SEQUENCE}`
- **TIER**: `UT` (Unit), `IT` (Integration)
- **KA**: Kubernaut Agent
- **752**: Issue number

### Tier 1: Unit Tests

**Testable code scope**: `pkg/kubernautagent/tools/summarizer/`, `pkg/kubernautagent/tools/k8s/`, `internal/kubernautagent/config/`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-752-001` | Tool output exceeding MaxToolOutputSize is truncated with guidance hint | Pending |
| `UT-KA-752-002` | Tool output at or below MaxToolOutputSize passes through unchanged | Pending |
| `UT-KA-752-003` | Summarizer pre-truncates input exceeding MaxToolOutputSize before LLM call | Pending |
| `UT-KA-752-004` | Summarizer works normally for inputs between threshold and MaxToolOutputSize | Pending |
| `UT-KA-752-005` | Truncation hint includes original output size and tool name | Pending |
| `UT-KA-752-006` | Summarizer pre-truncation note mentions truncation in prompt | Pending |
| `UT-KA-752-007` | `kubectl_get_by_name_in_cluster` returns single matching resource | Pending |
| `UT-KA-752-008` | `kubectl_get_by_name_in_cluster` returns error when resource not found | Pending |
| `UT-KA-752-009` | New tool registered in AllToolNames and available in RCA phase | Pending |
| `UT-KA-752-010` | MaxToolOutputSize parsed correctly from YAML configuration | Pending |
| `UT-KA-752-011` | MaxToolOutputSize defaults to 100000 when not specified in config | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `internal/kubernautagent/investigator/`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-752-001` | Investigator pipeline truncates oversized tool output before LLM receives it | Pending |
| `IT-KA-752-002` | Investigator pipeline passes normal-size tool output unchanged | Pending |
| `IT-KA-752-003` | When summarizer fails, hard truncation safety net still protects context window | Pending |

### Tier Skip Rationale

- **E2E**: Truncation is internal pipeline behavior with no external API surface changes. Integration tests with real pipeline wiring provide sufficient coverage.

---

## 9. Test Cases

### UT-KA-752-001: Hard truncation applied to oversized output

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/summarizer/truncation_test.go`

**Preconditions**:
- `TruncateToolOutput` function available with configurable limit

**Test Steps**:
1. **Given**: A tool output string of 200,000 characters and a limit of 100,000
2. **When**: `TruncateToolOutput` is called
3. **Then**: Result is <= 100,000 chars + hint suffix length; result ends with truncation guidance

**Expected Results**:
1. Output truncated to limit
2. Truncation hint appended with tool name and original size

### UT-KA-752-002: Below-limit output passes through unchanged

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/summarizer/truncation_test.go`

**Preconditions**:
- `TruncateToolOutput` function available

**Test Steps**:
1. **Given**: A tool output string of 500 characters and a limit of 100,000
2. **When**: `TruncateToolOutput` is called
3. **Then**: Result is identical to input

### UT-KA-752-003: Summarizer pre-truncates before LLM call

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/summarizer/summarizer_test.go`

**Preconditions**:
- Summarizer created with threshold=100 and maxInputSize=500
- Fake LLM client records calls

**Test Steps**:
1. **Given**: Tool output of 1000 characters (exceeds both threshold and maxInputSize)
2. **When**: `MaybeSummarize` is called
3. **Then**: The LLM receives a prompt whose tool output portion is <= 500 characters

### UT-KA-752-004: Summarizer works for moderate inputs

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/summarizer/summarizer_test.go`

**Preconditions**:
- Summarizer with threshold=100 and maxInputSize=5000

**Test Steps**:
1. **Given**: Tool output of 300 characters (exceeds threshold but below maxInputSize)
2. **When**: `MaybeSummarize` is called
3. **Then**: Full output is sent to LLM (no pre-truncation)

### UT-KA-752-007: New tool returns single matching resource

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Preconditions**:
- Fake K8s cluster with 3 Pods in different namespaces; one named "api-server"

**Test Steps**:
1. **Given**: Registry with `kubectl_get_by_name_in_cluster` tool registered
2. **When**: Execute with `{"kind":"Pod","name":"api-server"}`
3. **Then**: Returns JSON for exactly one Pod named "api-server"

### UT-KA-752-008: New tool returns error for nonexistent resource

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Preconditions**:
- Fake K8s cluster with no Pod named "nonexistent"

**Test Steps**:
1. **Given**: Registry with `kubectl_get_by_name_in_cluster` tool registered
2. **When**: Execute with `{"kind":"Pod","name":"nonexistent"}`
3. **Then**: Returns error or empty result indicating resource not found

### IT-KA-752-001: Pipeline truncation with oversized tool output

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_truncation_test.go`

**Preconditions**:
- Investigator with real pipeline (summarizer + truncation), fake tool returning 200K chars

**Test Steps**:
1. **Given**: Investigator configured with MaxToolOutputSize=1000
2. **When**: Tool returns 200,000 character output
3. **Then**: The tool result message in the LLM conversation is <= 1000 chars + hint

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Fake LLM client (existing `fakeLLM` pattern), fake K8s dynamic client
- **Location**: `test/unit/kubernautagent/tools/summarizer/`, `test/unit/kubernautagent/tools/k8s/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks — uses real Investigator with fake K8s client and fake LLM
- **Location**: `test/integration/kubernautagent/investigator/`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| None | — | — | — | — |

### 11.2 Execution Order

1. **Phase 1 (TDD RED)**: Write failing tests for truncation, summarizer pre-truncation, new tool, config
2. **Phase 2 (TDD GREEN)**: Implement minimal code to pass all tests
3. **Phase 3 (Checkpoint 1)**: Comprehensive adversarial and security audit
4. **Phase 4 (TDD REFACTOR)**: Extract constants, improve hints, ensure consistency
5. **Phase 5 (Final Checkpoint)**: Build, lint, full test pass

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/752/TEST_PLAN.md` | Strategy and test design |
| Truncation unit tests | `test/unit/kubernautagent/tools/summarizer/truncation_test.go` | Truncation logic tests |
| Summarizer pre-truncation tests | `test/unit/kubernautagent/tools/summarizer/summarizer_test.go` | Extended summarizer tests |
| New K8s tool unit tests | `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go` | `kubectl_get_by_name_in_cluster` tests |
| Config unit tests | `test/unit/kubernautagent/config/config_test.go` | MaxToolOutputSize config tests |
| Integration tests | `test/integration/kubernautagent/investigator/investigator_truncation_test.go` | Pipeline truncation tests |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/tools/summarizer/... -ginkgo.v
go test ./test/unit/kubernautagent/tools/k8s/... -ginkgo.v
go test ./test/unit/kubernautagent/config/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/investigator/... -ginkgo.v

# Specific test by ID
go test ./test/unit/kubernautagent/tools/summarizer/... -ginkgo.focus="UT-KA-752"
go test ./test/unit/kubernautagent/tools/k8s/... -ginkgo.focus="UT-KA-752"

# Coverage
go test ./test/unit/kubernautagent/tools/summarizer/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | — | — | All changes are additive; existing tests unaffected |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
