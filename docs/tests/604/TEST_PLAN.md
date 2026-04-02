# Test Plan: Case-Insensitive Approval Rego Environment Matching

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-604-v1
**Feature**: Fix case-sensitive environment matching in default Helm approval Rego policy
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.2.0-rc3`

---

## 1. Introduction

### 1.1 Purpose

Issue #604 reports that the Helm chart's default approval Rego policy uses exact
string comparison for environment (`input.environment == "production"`), but the
Signal Processor's LabelDetector produces PascalCase values (e.g. `Production`,
`Staging`). This means the production approval gate **never fires** when SP-classified
environments flow through.

Additionally, the default Helm Rego file (`charts/kubernaut/files/defaults/approval.rego`)
does not exist, meaning fresh installs without user-supplied policy get an empty
ConfigMap — no approval rules at all.

### 1.2 Objectives

1. **Helm default policy exists and is correct**: `charts/kubernaut/files/defaults/approval.rego` uses `lower()` for all environment comparisons
2. **PascalCase environments are matched**: `Production`, `Staging`, `Development` trigger the same rules as their lowercase equivalents
3. **Unit test fixture consistency**: `test/unit/aianalysis/testdata/policies/approval.rego` uses `lower()` for `is_high_severity`
4. **No regressions**: Existing evaluator tests continue to pass

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| New unit test pass rate | 100% | `go test ./test/unit/aianalysis/... -ginkgo.focus="604"` |
| Existing evaluator tests | 0 regressions | `go test ./test/unit/aianalysis/... -ginkgo.v` |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority

- **BR-AI-011**: Policy evaluation
- **BR-AI-013**: Approval scenarios
- **BR-AI-014**: Graceful degradation
- **Issue #604**: Approval Rego uses case-sensitive environment matching
- **Issue #595**: DS case-insensitive matching (same class, different layer — fixed in rc2)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Helm default Rego diverges from integration test fixture | Users get different behavior in production vs. CI | Medium | UT-AIA-604-001–004 | Base Helm default on integration test fixture (gold standard) |
| R2 | `is_high_severity` dead code in unit test Rego | Severity case-sensitivity bug lurks even after fix | Low | UT-AIA-604-005 | Fix `lower()` for defensive consistency |
| R3 | SP emits unexpected case variants | Tests only cover known PascalCase | Low | All | Test UPPER, lower, PascalCase, mixed |

---

## 4. Scope

### 4.1 Features to be Tested

- **Helm default approval Rego** (`charts/kubernaut/files/defaults/approval.rego`): New file must exist and use `lower()` for environment matching
- **Unit test Rego fixture** (`test/unit/aianalysis/testdata/policies/approval.rego`): `is_high_severity` must use `lower(input.severity)`
- **Go Evaluator** (`pkg/aianalysis/rego/evaluator.go`): No code changes — evaluator passes `input.environment` as-is (normalization is the policy's job)

### 4.2 Features Not to be Tested

- **SP LabelDetector output normalization**: Producer is correct per design (PascalCase is intentional)
- **Integration test Rego**: Already uses `lower()` throughout — no changes needed
- **E2E inline Rego**: Already uses `lower()` — no changes needed

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Test Helm default Rego directly via Go Evaluator | Validates the actual file the chart ships, not a copy |
| Base Helm default on integration test Rego | Integration Rego is the most comprehensive and battle-tested |
| Fix `is_high_severity` even though it's dead code | Defensive — prevents future breakage if severity gating is added |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of Rego evaluation paths (environment matching, severity matching)
- **Integration**: Deferred — Helm chart rendering requires Helm test infrastructure
- **E2E**: Deferred — existing E2E inline Rego already uses `lower()`

### 5.2 Tier Skip Rationale

- **Integration**: Helm template rendering tests are out of scope for this issue; the fix is validated by loading the Rego file through the Go evaluator.
- **E2E**: Existing E2E tests already use `lower()` in their inline Rego.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | What is tested | Lines (approx) |
|------|----------------|-----------------|
| `charts/kubernaut/files/defaults/approval.rego` (NEW) | `lower(input.environment)` normalization in all rules | ~200 |
| `test/unit/aianalysis/testdata/policies/approval.rego` | `lower(input.severity)` in `is_high_severity` | ~128 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AI-013 | PascalCase "Production" triggers approval gate | P0 | Unit | UT-AIA-604-001 | Pending |
| BR-AI-013 | UPPER "PRODUCTION" triggers approval gate | P0 | Unit | UT-AIA-604-002 | Pending |
| BR-AI-013 | Mixed "pRoDuCtIoN" triggers approval gate | P1 | Unit | UT-AIA-604-003 | Pending |
| BR-AI-013 | PascalCase "Staging" is non-production (auto-approve) | P0 | Unit | UT-AIA-604-004 | Pending |
| BR-AI-013 | PascalCase severity "Critical" matches `is_high_severity` | P1 | Unit | UT-AIA-604-005 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Test file**: `test/unit/aianalysis/rego_case_insensitive_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AIA-604-001` | PascalCase "Production" + low confidence → approval required | Pending |
| `UT-AIA-604-002` | UPPER "PRODUCTION" + low confidence → approval required | Pending |
| `UT-AIA-604-003` | Mixed case "pRoDuCtIoN" + low confidence → approval required | Pending |
| `UT-AIA-604-004` | PascalCase "Staging" + remediation target → auto-approve | Pending |
| `UT-AIA-604-005` | PascalCase "Critical" severity recognized by `is_high_severity` | Pending |

---

## 9. Test Cases

### UT-AIA-604-001: PascalCase "Production" triggers approval

**BR**: BR-AI-013
**Priority**: P0
**Type**: Unit

**Preconditions**:
- Helm default Rego loaded via `evaluator.StartHotReload()`

**Test Steps**:
1. **Given**: Evaluator loaded with `charts/kubernaut/files/defaults/approval.rego`
2. **When**: Evaluate with `Environment: "Production"`, `Confidence: 0.6`
3. **Then**: `ApprovalRequired == true`, `Degraded == false`

### UT-AIA-604-002: UPPER "PRODUCTION" triggers approval

**BR**: BR-AI-013
**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: Same evaluator
2. **When**: Evaluate with `Environment: "PRODUCTION"`, `Confidence: 0.6`
3. **Then**: `ApprovalRequired == true`

### UT-AIA-604-003: Mixed case "pRoDuCtIoN" triggers approval

**BR**: BR-AI-013
**Priority**: P1
**Type**: Unit

**Test Steps**:
1. **Given**: Same evaluator
2. **When**: Evaluate with `Environment: "pRoDuCtIoN"`, `Confidence: 0.6`
3. **Then**: `ApprovalRequired == true`

### UT-AIA-604-004: PascalCase "Staging" auto-approves

**BR**: BR-AI-013
**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: Same evaluator
2. **When**: Evaluate with `Environment: "Staging"`, `Confidence: 0.9`, valid remediation target
3. **Then**: `ApprovalRequired == false` (non-production)

### UT-AIA-604-005: PascalCase "Critical" severity

**BR**: BR-AI-013
**Priority**: P1
**Type**: Unit
**Note**: Tests the unit test fixture Rego (not Helm default) for `is_high_severity` fix

**Test Steps**:
1. **Given**: Evaluator loaded with `test/unit/aianalysis/testdata/policies/approval.rego`
2. **When**: Evaluate with `Severity: "Critical"`, `Environment: "production"`, `Confidence: 0.9`
3. **Then**: Evaluates without error (severity recognized — `is_high_severity` fires)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Location**: `test/unit/aianalysis/`
- **Dependencies**: OPA Go SDK (`github.com/open-policy-agent/opa/v1/rego`)

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1 (RED)**: Write failing tests for PascalCase environments against non-existent Helm default Rego
2. **Phase 2 (GREEN)**: Create Helm default Rego with `lower()`, fix unit test Rego `is_high_severity`
3. **Phase 3 (REFACTOR)**: Consistency review across all Rego files

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/604/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/aianalysis/rego_case_insensitive_test.go` | Ginkgo BDD tests |
| Helm default Rego | `charts/kubernaut/files/defaults/approval.rego` | Production default policy |

---

## 13. Execution

```bash
# New tests only
go test ./test/unit/aianalysis/... -ginkgo.focus="604" -ginkgo.v

# Full evaluator suite (regression check)
go test ./test/unit/aianalysis/... -ginkgo.v
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | N/A | N/A | Existing tests use lowercase; no changes needed |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
