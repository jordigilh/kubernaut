# Test Plan: Intrinsic Tool-Level Output Caps — Blast Radius Mitigations

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-752-CAPS-v1
**Feature**: Add intrinsic output caps to high-risk KA tools identified in blast radius analysis
**Version**: 1.0
**Created**: 2026-04-20
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/752-tool-output-intrinsic-caps`

---

## 1. Introduction

### 1.1 Purpose

The first PR for Issue #752 added pipeline-level truncation (hard cap at 100K chars in `executeTool`). However, the blast radius analysis identified four tool categories that can produce unbounded output *before* the pipeline cap fires. This test plan validates intrinsic, tool-level caps that prevent wasteful API fetches and memory spikes at the source, rather than relying solely on the pipeline safety net.

### 1.2 Objectives

1. **Default log tail lines**: All 8 `kubectl_logs` variants apply `DefaultLogTailLines=500` when the LLM omits `tailLines` and `limitBytes`, preventing full container log fetches
2. **Empty keyword rejection**: `kubectl_find_resource` rejects empty `keyword` with an error, preventing full cluster-wide list dumps
3. **JQ output character cap**: `kubernetes_jq_query` truncates joined output at `maxJQOutputChars=100000` with a truncation hint
4. **Events limit**: `kubectl_events` applies `DefaultEventLimit=200` via `ListOptions.Limit`, with a truncation hint when capped
5. **Zero regressions**: All existing K8s tool, investigator, and security tests continue to pass

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on modified files |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-SAFETY-752: Tool output must not exceed LLM context window capacity
- Issue #752: LLM context window overflow — blast radius analysis follow-up
- TP-752-v1: First test plan (pipeline-level truncation, already merged)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Existing K8s tools tests: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`
- Existing integration tests: `test/integration/kubernautagent/tools/k8s/k8s_tools_test.go`
- Prometheus `TruncateWithHint` pattern: `pkg/kubernautagent/tools/prometheus/tools.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Default tailLines=500 misses critical log entries before the tail window | LLM misses root cause in early log lines | Medium | UT-KA-752-101,102,103 | LLM can still pass explicit `tailLines` or `limitBytes` to override; grep variants narrow by pattern first |
| R2 | Rejecting empty keyword breaks LLM workflows relying on find_resource as list-all | LLM gets error instead of data | Low | UT-KA-752-104,105 | `kubectl_get_by_kind_in_cluster` exists for list-all; LLM tool descriptions guide correct usage |
| R3 | JQ char cap truncates mid-JSON-object producing invalid output | LLM sees broken JSON | Medium | UT-KA-752-106,107 | Truncation appends hint explaining the cap; pipeline cap would have truncated anyway |
| R4 | Events Limit=200 misses relevant events on high-churn resources | LLM misses recent events | Low | UT-KA-752-108,109 | K8s API returns most recent events first with Limit; 200 is ample for diagnostic purposes |
| R5 | Default tailLines interferes when LLM passes explicit tailLines | LLM's explicit choice overridden | Critical | UT-KA-752-103 | Default only applies when BOTH tailLines AND limitBytes are nil |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-KA-752-101 (default applied), UT-KA-752-102 (explicit override honored), UT-KA-752-103 (allContainers path)
- **R2**: UT-KA-752-104 (empty keyword error), UT-KA-752-105 (non-empty keyword still works)
- **R3**: UT-KA-752-106 (char cap applied), UT-KA-752-107 (below-cap passes through)
- **R4**: UT-KA-752-108 (limit applied), UT-KA-752-109 (hint when capped)
- **R5**: UT-KA-752-102 (explicit tailLines honored)

---

## 4. Scope

### 4.1 Features to be Tested

- **Log default tail lines** (`pkg/kubernautagent/tools/k8s/tools.go`): `logTool.Execute` applies `DefaultLogTailLines` when `tailLines` and `limitBytes` are both nil
- **Empty keyword rejection** (`pkg/kubernautagent/tools/k8s/tools.go`): `findResourceTool.Execute` returns error on empty keyword
- **JQ output char cap** (`pkg/kubernautagent/tools/k8s/jq.go`): `runJQ` truncates joined output exceeding `maxJQOutputChars`
- **Events limit** (`pkg/kubernautagent/tools/k8s/tools.go`): `newEvents` applies `DefaultEventLimit` via `ListOptions.Limit` with truncation hint

### 4.2 Features Not to be Tested

- **Pipeline-level truncation**: Already tested and merged in TP-752-v1
- **Prometheus tools**: Already capped by `SizeLimit` at HTTP client layer
- **fetch_pod_logs**: Already has default `limit=100` lines
- **Resource context tools**: Fixed structure, bounded by DS response
- **Tier 2 (Medium risk) tools**: `kubectl_top_pods`, `kubectl_memory_requests_*` — covered by pipeline cap; too linear and small per-item to hit 100K in practice

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| DefaultLogTailLines=500 | ~500 lines covers typical diagnostic window; LLM can override |
| Default only when both tailLines AND limitBytes nil | Respects explicit LLM choices; limitBytes alone is a valid constraint |
| Empty keyword returns error, not empty list | Prevents silent full-cluster dumps; LLM has `kubectl_get_by_kind_in_cluster` for list-all |
| maxJQOutputChars=100000 matching pipeline cap | Consistent with `MaxToolOutputSize`; prevents double-truncation |
| DefaultEventLimit=200 via ListOptions.Limit | Server-side limit is more efficient than fetching all and truncating |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (log defaults, find_resource validation, jq cap, events limit)
- **Integration**: Existing integration tests validate no regressions; new IT for events limit behavior
- **E2E**: Not applicable — internal tool behavior, no API surface changes

### 5.2 Two-Tier Minimum

- **Unit tests**: Validate each cap in isolation
- **Integration tests**: Validate K8s tool behavior with fake clients

### 5.3 Pass/Fail Criteria

**PASS** — all of the following must be true:
1. All P0 tests pass (0 failures)
2. Per-tier code coverage meets >=80% threshold
3. No regressions in existing test suites
4. `go build ./...` succeeds with zero errors

**FAIL** — any of the following:
1. Any P0 test fails
2. Existing tests regress

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kubernautagent/tools/k8s/tools.go` | `logTool.Execute` (default tailLines), `findResourceTool.Execute` (empty keyword) | ~20 |
| `pkg/kubernautagent/tools/k8s/jq.go` | `runJQ` (char cap), `maxJQOutputChars` constant | ~10 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kubernautagent/tools/k8s/tools.go` | `newEvents` fetchFunc (Limit on ListOptions) | ~15 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SAFETY-752 | Log tools apply default tailLines when omitted | P0 | Unit | UT-KA-752-101 | Pending |
| BR-SAFETY-752 | Log tools honor explicit tailLines | P0 | Unit | UT-KA-752-102 | Pending |
| BR-SAFETY-752 | Log tools apply default in allContainers path | P0 | Unit | UT-KA-752-103 | Pending |
| BR-SAFETY-752 | find_resource rejects empty keyword | P0 | Unit | UT-KA-752-104 | Pending |
| BR-SAFETY-752 | find_resource works with non-empty keyword (no regression) | P0 | Unit | UT-KA-752-105 | Pending |
| BR-SAFETY-752 | JQ query output truncated at char cap | P0 | Unit | UT-KA-752-106 | Pending |
| BR-SAFETY-752 | JQ query output below cap passes through | P0 | Unit | UT-KA-752-107 | Pending |
| BR-SAFETY-752 | Events tool applies Limit on ListOptions | P0 | Unit | UT-KA-752-108 | Pending |
| BR-SAFETY-752 | Events tool includes hint when result is capped | P1 | Unit | UT-KA-752-109 | Pending |
| BR-SAFETY-752 | kubernetes_count is unaffected (already naturally bounded) | P1 | Unit | UT-KA-752-110 | Pending |
| BR-SAFETY-752 | Log default does not apply when limitBytes is set | P1 | Unit | UT-KA-752-111 | Pending |
| BR-SAFETY-752 | Events tool end-to-end with fake K8s client | P0 | Integration | IT-KA-752-101 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-752-{SEQUENCE}` (100-series for intrinsic caps)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/kubernautagent/tools/k8s/`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-752-101` | Log tool applies DefaultLogTailLines=500 when tailLines and limitBytes are nil | Pending |
| `UT-KA-752-102` | Log tool honors explicit tailLines=100 without applying default | Pending |
| `UT-KA-752-103` | Log tool applies default in logsAllContainers path | Pending |
| `UT-KA-752-104` | find_resource returns error when keyword is empty | Pending |
| `UT-KA-752-105` | find_resource returns matching items with non-empty keyword (no regression) | Pending |
| `UT-KA-752-106` | JQ query truncates output exceeding maxJQOutputChars with hint | Pending |
| `UT-KA-752-107` | JQ query passes through output below maxJQOutputChars unchanged | Pending |
| `UT-KA-752-108` | Events tool includes Limit in ListOptions | Pending |
| `UT-KA-752-109` | Events tool appends truncation hint when result count equals limit | Pending |
| `UT-KA-752-110` | kubernetes_count output is unaffected by JQ char cap (naturally bounded) | Pending |
| `UT-KA-752-111` | Log tool does NOT apply default tailLines when limitBytes is set | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-752-101` | Events tool end-to-end returns capped results via fake K8s client | Pending |

---

## 9. Test Cases

### UT-KA-752-101: Default tailLines applied to log tools

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: A `kubectl_logs` tool with a fake K8s client
2. **When**: Execute with `{"name":"pod","namespace":"ns"}` (no tailLines, no limitBytes)
3. **Then**: The `PodLogOptions` sent to the K8s API has `TailLines=500`

### UT-KA-752-102: Explicit tailLines honored

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: A `kubectl_logs` tool with a fake K8s client
2. **When**: Execute with `{"name":"pod","namespace":"ns","tailLines":100}`
3. **Then**: The `PodLogOptions` sent to the K8s API has `TailLines=100` (not overridden to 500)

### UT-KA-752-103: Default tailLines in allContainers path

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: A `kubectl_logs_all_containers` tool with a fake K8s client and a 2-container pod
2. **When**: Execute with `{"name":"pod","namespace":"ns"}` (no tailLines)
3. **Then**: Both container log requests use `TailLines=500`

### UT-KA-752-104: Empty keyword rejected

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: Registry with `kubectl_find_resource` tool
2. **When**: Execute with `{"kind":"Pod","keyword":""}`
3. **Then**: Returns an error containing "keyword" (not a full cluster dump)

### UT-KA-752-105: Non-empty keyword works (no regression)

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: Registry with `kubectl_find_resource` tool and seeded data
2. **When**: Execute with `{"kind":"Job","keyword":"migration"}`
3. **Then**: Returns matching items (same as UT-KA-433-503 behavior)

### UT-KA-752-106: JQ output char cap applied

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: Registry with `kubernetes_jq_query` tool and a resource whose jq output would exceed `maxJQOutputChars`
2. **When**: Execute with a jq expression that produces very large output
3. **Then**: Output is truncated at `maxJQOutputChars` with a truncation hint

### UT-KA-752-107: JQ output below cap passes through

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: Registry with `kubernetes_jq_query` tool and a small dataset
2. **When**: Execute with `{"kind":"Pod","jq_expr":".items[].metadata.name"}`
3. **Then**: Output is unchanged (no truncation hint)

### UT-KA-752-108: Events tool applies Limit

**BR**: BR-SAFETY-752
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: A fake K8s client with >200 events for a resource
2. **When**: Execute `kubectl_events` for that resource
3. **Then**: Result contains at most 200 events; truncation hint appended

### UT-KA-752-109: Events truncation hint content

**BR**: BR-SAFETY-752
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: Events tool returns exactly `DefaultEventLimit` events
2. **When**: Execute completes
3. **Then**: Output contains `[TRUNCATED]` and mentions the limit

### UT-KA-752-110: kubernetes_count unaffected

**BR**: BR-SAFETY-752
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: `kubernetes_count` with small dataset
2. **When**: Execute count query
3. **Then**: Output starts with "Count:" and has no truncation hint (naturally bounded)

### UT-KA-752-111: Default tailLines not applied when limitBytes set

**BR**: BR-SAFETY-752
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go`

**Test Steps**:
1. **Given**: A `kubectl_logs` tool
2. **When**: Execute with `{"name":"pod","namespace":"ns","limitBytes":1024}` (no tailLines)
3. **Then**: `PodLogOptions` has `LimitBytes=1024` and `TailLines` is nil (default NOT applied)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Fake K8s client (`fake.NewSimpleClientset`), fake dynamic client
- **Location**: `test/unit/kubernautagent/tools/k8s/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks — uses real tool code with fake K8s client
- **Location**: `test/integration/kubernautagent/tools/k8s/`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| PR #755 (pipeline-level truncation) | Code | Open | Branch base; will rebase to main once merged | Branch from PR branch |

### 11.2 Execution Order

1. **Phase 1 (TDD RED)**: Write failing tests for all 12 scenarios
2. **Phase 2 (TDD GREEN)**: Implement minimal code: constants + logic changes in tools.go, jq.go
3. **Phase 3 (Checkpoint 1)**: Adversarial and security audit
4. **Phase 4 (TDD REFACTOR)**: Extract shared patterns, align truncation hint formats
5. **Phase 5 (Final Checkpoint)**: Build, lint, full test pass

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/752/TEST_PLAN_INTRINSIC_CAPS.md` | Strategy and test design |
| K8s tool unit tests | `test/unit/kubernautagent/tools/k8s/kind_resolution_test.go` | Intrinsic cap tests (UT-KA-752-1xx) |
| Integration tests | `test/integration/kubernautagent/tools/k8s/k8s_tools_test.go` | Events limit IT |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/tools/k8s/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/tools/k8s/... -ginkgo.v

# Specific tests by ID
go test ./test/unit/kubernautagent/tools/k8s/... -ginkgo.focus="UT-KA-752-1"

# Coverage
go test ./test/unit/kubernautagent/tools/k8s/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| UT-KA-433-503 | `kubectl_find_resource` with keyword="migration" works | No change | Non-empty keyword still works |
| IT-KA-433-018 | `kubectl_logs` with explicit `tailLines:500` works | No change | Explicit value honored |
| UT-KA-433-509 | `kubernetes_jq_query` small dataset works | No change | Below char cap |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-20 | Initial test plan for intrinsic tool caps |
