# Test Plan: PDB Signal Guidance Dead Code (#742)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-742-v1
**Feature**: Wire PDBSignalGuidance so the LLM receives PDB-specific investigation instructions
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/742-743-pdb-guidance-dedup-fields`

---

## 1. Introduction

### 1.1 Purpose

Validate that the prompt builder correctly activates PDB-specific investigation guidance
when the signal targets a PodDisruptionBudget resource. The `PDBSignalGuidance` field on
`investigationTemplateData` was defined but never populated, causing the template conditional
block at `incident_investigation.tmpl:60-73` to be dead code.

### 1.2 Objectives

1. **isPDBSignal detection**: `isPDBSignal` correctly identifies PDB signals by ResourceKind
2. **Template activation**: `RenderInvestigation` includes PDB guidance section when ResourceKind is PodDisruptionBudget
3. **Negative case**: Non-PDB signals do NOT trigger PDB guidance
4. **Template content**: PDB guidance section contains actionable remediation_target instructions

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/prompt/... -ginkgo.focus="742"` |
| Unit-testable code coverage | >=80% | Coverage of `isPDBSignal` + `RenderInvestigation` PDB path |
| Backward compatibility | 0 regressions | Existing prompt builder tests pass without modification |

---

## 2. References

### 2.1 Authority

- Issue #742: PDB workflow fails because TARGET_RESOURCE_NAME receives Deployment name
- Issue #198: Original PDB signal guidance design (template text authored but never wired)
- BR-HAPI-212: RCA target resource identification

### 2.2 Cross-References

- Golden transcript: `test/services/mock-llm/golden-transcripts/pdb-kubepoddisruptionbudgetatlimit.json`
- Template: `internal/kubernautagent/prompt/templates/incident_investigation.tmpl`
- Builder: `internal/kubernautagent/prompt/builder.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | PDB guidance renders for non-PDB signals | False-positive LLM instructions | Low | UT-KA-742-003, UT-KA-742-006 | Negative tests with Deployment/Pod signals |
| R2 | Template text insufficient for LLM | LLM still targets Deployment | Medium | UT-KA-742-007 | Validate text contains key terms; defer LLM validation to Kind cluster |
| R3 | sanitizeSignal corrupts ResourceKind | isPDBSignal never matches | Low | UT-KA-742-001 | Already validated: sanitizeField preserves PascalCase values |

---

## 4. Scope

### 4.1 Features to be Tested

- **`isPDBSignal`** (`internal/kubernautagent/prompt/builder.go`): New helper that detects PDB signals
- **`RenderInvestigation`** (`internal/kubernautagent/prompt/builder.go`): Wiring of PDBSignalGuidance field

### 4.2 Features Not to be Tested

- **`InjectRemediationTarget`**: Covered by existing tests; no changes needed
- **Template text changes**: Using existing text from #198 as-is; LLM validation deferred to Kind cluster
- **Workflow catalog resource type scoping**: Separate enhancement

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `isPDBSignal` + PDB path in `RenderInvestigation`

### 5.2 Pass/Fail Criteria

**PASS**: All 7 tests pass, no regressions in existing builder tests.
**FAIL**: Any test fails or existing tests regress.

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-212 | RCA target resource for PDB signals | P0 | Unit | UT-KA-742-001 | Pending |
| BR-HAPI-212 | RCA target resource for PDB signals | P0 | Unit | UT-KA-742-002 | Pending |
| BR-HAPI-212 | RCA target resource for PDB signals | P0 | Unit | UT-KA-742-003 | Pending |
| BR-HAPI-212 | RCA target resource for PDB signals | P0 | Unit | UT-KA-742-004 | Pending |
| BR-HAPI-212 | RCA target resource for PDB signals | P0 | Unit | UT-KA-742-005 | Pending |
| BR-HAPI-212 | RCA target resource for PDB signals | P0 | Unit | UT-KA-742-006 | Pending |
| BR-HAPI-212 | RCA target resource for PDB signals | P1 | Unit | UT-KA-742-007 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/prompt/builder.go` — `isPDBSignal`, `RenderInvestigation`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-742-001` | isPDBSignal returns true for ResourceKind=PodDisruptionBudget | Pending |
| `UT-KA-742-002` | isPDBSignal returns true for ResourceKind=PodDisruptionBudget (case preserved) | Pending |
| `UT-KA-742-003` | isPDBSignal returns false for non-PDB signals (Deployment, Pod) | Pending |
| `UT-KA-742-004` | RenderInvestigation includes PDB guidance section when ResourceKind=PodDisruptionBudget | Pending |
| `UT-KA-742-005` | RenderInvestigation includes PDB guidance when signal name contains PDB keyword but ResourceKind is empty | Pending |
| `UT-KA-742-006` | RenderInvestigation does NOT include PDB guidance for non-PDB signals | Pending |
| `UT-KA-742-007` | PDB guidance section contains remediation_target and kind=PodDisruptionBudget instruction | Pending |

### Tier Skip Rationale

- **Integration**: No I/O involved — pure template rendering logic. Unit tests provide full coverage.
- **E2E**: LLM behavioral validation deferred to Kind cluster deployment.

---

## 9. Test Cases

### UT-KA-742-001: isPDBSignal detects PodDisruptionBudget kind

**BR**: BR-HAPI-212
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/builder_test.go`

**Test Steps**:
1. **Given**: ResourceKind = "PodDisruptionBudget"
2. **When**: `isPDBSignal` is called
3. **Then**: Returns true

### UT-KA-742-004: RenderInvestigation activates PDB guidance

**BR**: BR-HAPI-212
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/builder_test.go`

**Test Steps**:
1. **Given**: SignalData with ResourceKind = "PodDisruptionBudget", Name = "KubePodDisruptionBudgetAtLimit"
2. **When**: `RenderInvestigation` is called
3. **Then**: Output contains "PDB-Specific Investigation Guidance"

### UT-KA-742-006: Non-PDB signal does NOT activate guidance

**BR**: BR-HAPI-212
**Priority**: P0
**File**: `test/unit/kubernautagent/prompt/builder_test.go`

**Test Steps**:
1. **Given**: SignalData with ResourceKind = "Deployment", Name = "OOMKilled"
2. **When**: `RenderInvestigation` is called
3. **Then**: Output does NOT contain "PDB-Specific Investigation Guidance"

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None required (pure template rendering)
- **Location**: `test/unit/kubernautagent/prompt/`

---

## 13. Execution

```bash
go test ./test/unit/kubernautagent/prompt/... -ginkgo.focus="742" -ginkgo.v
```

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
