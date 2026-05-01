# Test Plan: Target-Workflow Alignment Gate (#934)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-934-v1
**Feature**: Structured target-workflow alignment gate that validates the LLM-selected workflow's component scope matches the RCA remediation target kind
**Version**: 1.1
**Created**: 2026-05-01
**Author**: AI Assistant
**Status**: Complete
**Branch**: `fix/934-target-workflow-alignment`

---

## 1. Introduction

### 1.1 Purpose

Issue #934 identified that the LLM can select the correct workflow but target the wrong resource kind. This test plan validates the structured alignment gate that catches target-workflow mismatches in Phase 3 using catalog metadata.

### 1.2 Objectives

1. **`MatchesTargetKind` correctness**: All 8 matching scenarios produce correct boolean results
2. **Alignment gate behavior**: Gate emits audit events and appends warnings for misaligned pairs without blocking
3. **Backward compatibility**: Workflows without `Component` metadata pass unconditionally

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/parser/... ./test/unit/kubernautagent/investigator/...` |
| Backward compatibility | 0 regressions | All existing tests pass |

---

## 2. References

- **BR-AI-934**: LLM selects correct workflow but wrong remediation target
- Issue #934: LLM selects correct workflow but wrong target
- `sameKindValidationGate` in `investigator.go` (existing gate pattern)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Mitigation |
|----|------|--------|------------|
| R1 | Kind casing mismatch | False negatives | `strings.EqualFold` |
| R2 | Wildcard not recognized | All wildcards flagged | Explicit `"*"` check |
| R3 | Empty Component (backward compat) | Legacy workflows blocked | Treat empty as unconstrained |
| R4 | Gate blocks valid reasoning | False HR escalation | WARNING-level only |
| R5 | Missing audit trail | Ops blind spot | Events for both outcomes |

---

## 4. Test Scenarios

### Group A: `WorkflowMeta.MatchesTargetKind` (parser/validator.go)

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `UT-KA-934-001` | Exact kind match returns true | REFACTORED |
| `UT-KA-934-002` | Wildcard `["*"]` matches any kind | REFACTORED |
| `UT-KA-934-003` | Case-insensitive match | REFACTORED |
| `UT-KA-934-004a` | Nil component = unconstrained | REFACTORED |
| `UT-KA-934-004b` | Empty component = unconstrained | REFACTORED |
| `UT-KA-934-005` | Multi-component slice match | REFACTORED |
| `UT-KA-934-006` | No-match returns false | REFACTORED |
| `UT-KA-934-007` | Empty target kind = unconstrained | REFACTORED |

### Group B: `CheckWorkflowTargetAlignment` (investigator/investigator.go)

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `UT-KA-934-008` | Mismatch emits audit event with failure outcome | REFACTORED |
| `UT-KA-934-009` | Match emits audit event with success outcome | REFACTORED |
| `UT-KA-934-010` | Mismatch appends warning, no HR escalation | REFACTORED |
| `UT-KA-934-011` | Match does not append warning | REFACTORED |
| `UT-KA-934-012` | Gate skipped when WorkflowID empty | REFACTORED |
| `UT-KA-934-013` | Gate skipped when metadata not in catalog | REFACTORED |

---

## 5. Execution

```bash
go test ./test/unit/kubernautagent/parser/... -ginkgo.focus="UT-KA-934"
go test ./test/unit/kubernautagent/investigator/... -ginkgo.focus="UT-KA-934"
```

---

## 6. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-01 | Initial test plan |
| 1.1 | 2026-05-01 | All 14 tests pass. TDD RED-GREEN-REFACTOR complete. Validated against 100 Go Mistakes. |
