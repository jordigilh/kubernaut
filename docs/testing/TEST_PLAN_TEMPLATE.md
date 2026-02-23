# Test Plan: [FEATURE_NAME]

**Feature**: [One-line description]
**Version**: 1.0
**Created**: [YYYY-MM-DD]
**Author**: [Name]
**Status**: Draft | Ready for Execution | Active | Complete
**Branch**: `feat/[branch-name]`

**Authority**:
- [BR-XXX-NNN]: [Business requirement title]
- [ADR-NNN / DD-XXX-NNN]: [Architecture/design decision title]

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- [Component/module 1]: [what is being tested and why]
- [Component/module 2]: [what is being tested and why]

### Out of Scope

- [What is explicitly excluded and why]

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| [Key design choice] | [Why this approach was chosen] |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (pure logic: validators, parsers, builders, types)
- **Integration**: >=80% of **integration-testable** code (I/O: HTTP handlers, DB adapters, filesystem, controller wiring)
- **E2E**: >=80% of full service code (when applicable)

### 2-Tier Minimum

Every business requirement gap must be covered by at least 2 test tiers (typically UT + IT) to provide defense-in-depth:
- **Unit tests** catch logic and correctness errors (fast feedback, isolated)
- **Integration tests** catch wiring, data fidelity, and behavior errors across component boundaries

If a tier is skipped, the rationale must be documented in Section 5.

### Business Outcome Quality Bar

Tests validate **business outcomes** -- behavior, correctness, and data accuracy -- not just code path coverage. Each test scenario answers: "what does the user/operator/system get?" not "what function is called?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/[path]/[file].go` | `FunctionA`, `FunctionB` | ~NNN |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/[path]/[file].go` | `HandlerA`, `HandlerB` | ~NNN |
| `cmd/[service]/main.go` | Startup wiring | ~NNN |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-XXX-NNN | [Requirement description] | P0 | Unit | UT-SVC-NNN-001 | Pending |
| BR-XXX-NNN | [Requirement description] | P0 | Integration | IT-SVC-NNN-001 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: 2-4 char abbreviation (AA, DS, GW, NOT, RO, SP, WE)
- **BR_NUMBER**: Business requirement or ADR number
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: [List the files and what % coverage is targeted]

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-SVC-NNN-001` | [What the user/operator/system gets when this works correctly] | RED |

### Tier 2: Integration Tests

**Testable code scope**: [List the files and what % coverage is targeted]

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-SVC-NNN-001` | [What the user/operator/system gets when this works correctly] | RED |

### Tier Skip Rationale (if any tier is omitted)

- **[Tier]**: [Why this tier is not applicable or deferred, and what alternative coverage exists]

---

## 6. Test Cases (Detail)

### UT-SVC-NNN-001: [Short name]

**BR**: BR-XXX-NNN
**Type**: Unit
**File**: `test/unit/[service]/[file]_test.go`

**Given**: [Precondition / system state]
**When**: [Action or event]
**Then**: [Observable business outcome with measurable criteria]

**Acceptance Criteria**:
- [Specific, measurable criterion]
- [Data accuracy assertion]
- [Error message quality assertion, if applicable]

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: External dependencies only (DB, APIs, K8s, filesystem)
- **Location**: `test/unit/[service]/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see [No-Mocks Policy](INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Infrastructure**: [What real services are needed: PostgreSQL, filesystem, mock HTTP, etc.]
- **Location**: `test/integration/[service]/`

---

## 8. Execution

```bash
# Unit tests
make test

# Integration tests
make test-integration-[service]

# Specific test by ID
go test ./test/unit/[service]/... -ginkgo.focus="UT-SVC-NNN-001"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | [YYYY-MM-DD] | Initial test plan |
