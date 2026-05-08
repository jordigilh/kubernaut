# Test Plan: SignalToPrompt Target Resource Label Override

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1061-v1
**Feature**: `SignalToPrompt` prefers `target_resource_kind` / `target_resource_name` signal labels over enrichment-resolved values
**Version**: 1.0
**Created**: 2026-05-08
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/1061-1062-signal-target-and-ambiguous-kind`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

When an alert includes explicit `target_resource_kind` and `target_resource_name` labels, the LLM investigation prompt must reference the actual remediation target rather than the enrichment-resolved namespace container. Today, `SignalToPrompt()` ignores these labels and always uses `SignalContext.ResourceKind` / `ResourceName`, which the gateway's enrichment layer often sets to `Namespace`.

### 1.2 Objectives

1. **Label override**: When `target_resource_kind` and/or `target_resource_name` are present and valid in `SignalLabels`, `SignalToPrompt` uses them for `ResourceKind` / `ResourceName` in the returned `prompt.SignalData`.
2. **Enrichment fallback**: When labels are absent, empty, or invalid, the enrichment-resolved values are preserved.
3. **Input validation (FedRAMP SI-10)**: Label values containing path separators (`/`, `\`, `..`), control characters, or exceeding 253 characters are rejected and the enrichment fallback is used.
4. **No mutation**: The original `SignalContext` struct is not modified by `SignalToPrompt`.
5. **Audit trail (FedRAMP AU-2)**: When an override occurs, the caller logs the original and overridden values with the correlation ID.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/investigator/... --ginkgo.focus="Issue.*1061"` |
| Unit-testable code coverage | >=80% | Coverage on `SignalToPrompt` + `isValidK8sIdentifier` |
| Backward compatibility | 0 regressions | Full investigator suite passes |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority

- Issue #1061: signalToPrompt should prefer target_resource_kind/name from signal labels
- FedRAMP SI-10: Information Input Validation
- FedRAMP AU-2: Auditable Events

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Anti-Pattern Detection](../../testing/ANTI_PATTERN_DETECTION.md)

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Malicious label values injected into LLM prompt | Prompt pollution | Low | UT-KA-1061-008..010, 013 | `isValidK8sIdentifier` validation |
| R2 | `sameKindValidationGate` uses overridden kind instead of signal kind | Incorrect gate behavior | Low | UT-KA-1061-014 | Gate operates on `signal.ResourceKind` (by-value, not pointer) |
| R3 | Nil `SignalLabels` map causes panic | Runtime crash | Medium | UT-KA-1061-007 | Go map index on nil returns zero value |
| R4 | Label override not auditable | FedRAMP AU-2 violation | Medium | N/A (code review) | Structured logging at call sites |

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- `SignalToPrompt()` in `internal/kubernautagent/investigator/investigator_phases.go`
- `isValidK8sIdentifier()` in same file
- Override logging in `runRCA()` and `runWorkflowSelection()` in `investigator.go`

### 4.2 Features Not to be Tested

- Gateway signal mapping (pre-existing, separate pipeline)
- LLM prompt template rendering (tested by prompt package)
- `sameKindValidationGate` full behavior (tested by #847 tests; only interaction verified here)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Unit-only tier | `SignalToPrompt` is a pure function with no I/O |
| Validation rejects rather than sanitizes | Defense-in-depth: reject suspicious input, fall back to safe default |
| Logging at caller, not in `SignalToPrompt` | Function is pure; callers have access to `logger` and `correlationID` |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

- **Unit**: >=80% of `SignalToPrompt` + `isValidK8sIdentifier`
- **Integration**: Skipped — pure function, no I/O
- **E2E**: Skipped — prompt-level logic, not observable at system boundary

### 5.2 Pass/Fail Criteria

**PASS**: All 14 unit tests pass, no regressions in investigator suite, no `Skip()` or `time.Sleep` anti-patterns.

**FAIL**: Any test failure, or label override not logged at call sites.

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested.

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `internal/kubernautagent/investigator/investigator_phases.go` | `SignalToPrompt`, `isValidK8sIdentifier` | ~40 |
| `internal/kubernautagent/investigator/investigator.go` | `runRCA` (logging), `runWorkflowSelection` (logging) | ~10 added |

---

## 7. BR Coverage Matrix

| BR / Issue ID | Description | Priority | Tier | Test ID | Status |
|---------------|-------------|----------|------|---------|--------|
| #1061 | Label override for kind/name | P0 | Unit | UT-KA-1061-001 | Pass |
| #1061 | No labels preserves defaults | P0 | Unit | UT-KA-1061-002 | Pass |
| #1061 | Partial override — kind only | P0 | Unit | UT-KA-1061-003 | Pass |
| #1061 | Partial override — name only | P0 | Unit | UT-KA-1061-004 | Pass |
| #1061 | Empty labels ignored | P0 | Unit | UT-KA-1061-005 | Pass |
| #1061 | Other fields unchanged | P1 | Unit | UT-KA-1061-006 | Pass |
| #1061 | Nil SignalLabels safety | P0 | Unit | UT-KA-1061-007 | Pass |
| FedRAMP SI-10 | Path traversal rejection | P0 | Unit | UT-KA-1061-008 | Pass |
| FedRAMP SI-10 | Control char rejection | P0 | Unit | UT-KA-1061-009 | Pass |
| FedRAMP SI-10 | Overlong value rejection | P0 | Unit | UT-KA-1061-010 | Pass |
| #1061 | Unicode acceptance | P1 | Unit | UT-KA-1061-011 | Pass |
| FedRAMP SI-10 | Boundary 253 acceptance | P1 | Unit | UT-KA-1061-012 | Pass |
| FedRAMP SI-10 | Backslash rejection | P0 | Unit | UT-KA-1061-013 | Pass |
| ARCH-5 | sameKindGate non-interference | P0 | Unit | UT-KA-1061-014 | Pass |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-KA-{ISSUE}-{SEQUENCE}`

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-1061-001 | Labels override enrichment-resolved kind/name | Pass |
| UT-KA-1061-002 | No labels preserves enrichment values | Pass |
| UT-KA-1061-003 | Partial override — kind only | Pass |
| UT-KA-1061-004 | Partial override — name only | Pass |
| UT-KA-1061-005 | Empty label values ignored | Pass |
| UT-KA-1061-006 | All other fields propagate unchanged | Pass |
| UT-KA-1061-007 | Nil SignalLabels map is safe | Pass |
| UT-KA-1061-008 | Path traversal labels rejected | Pass |
| UT-KA-1061-009 | Control character labels rejected | Pass |
| UT-KA-1061-010 | Overlong labels rejected (254 chars) | Pass |
| UT-KA-1061-011 | Valid Unicode labels accepted | Pass |
| UT-KA-1061-012 | Boundary 253-char label accepted | Pass |
| UT-KA-1061-013 | Backslash labels rejected | Pass |
| UT-KA-1061-014 | sameKindGate interaction (ARCH-5) | Pass |

### Tier Skip Rationale

- **Integration**: Skipped. `SignalToPrompt` is unit-testable pure logic with no I/O.
- **E2E**: Skipped. Prompt-level override is not observable at system boundary without full LLM integration.

---

## 9. Environmental Needs

### 9.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None required (pure function)
- **Location**: `test/unit/kubernautagent/investigator/signal_label_override_test.go`

---

## 10. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/tests/1061/TEST_PLAN.md` |
| Unit test suite | `test/unit/kubernautagent/investigator/signal_label_override_test.go` |

---

## 11. Execution

```bash
# Focused run
go test ./test/unit/kubernautagent/investigator/... -count=1 --ginkgo.focus="Issue.*1061" -v

# Full suite regression
go test ./test/unit/kubernautagent/investigator/... -count=1 -race
```

---

## 12. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-08 | Initial test plan for Issue #1061 |
