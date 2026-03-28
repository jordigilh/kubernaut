# Test Plan: SignalProcessing CRD Typed Enums (#456)

**Feature**: Add typed enums for `environment`, `source` (environment), `priority`, `source` (priority) fields on `EnvironmentClassification` and `PriorityAssignment` status structs
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- BR-SP-080 V2.0: Environment Classification (source values: `namespace-labels`, `rego-inference`, `default`)
- BR-SP-071: Priority Assignment (severity-fallback when Rego fails)
- Coding standard: "AVOID using `any` or `interface{}` — ALWAYS use structured field values with specific types"

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- Depends on #458 (shared BusinessClassification types)

---

## 1. Scope

### In Scope

- `api/signalprocessing/v1alpha1/signalprocessing_types.go`:
  - Add `Environment` typed alias + kubebuilder enum (`production, staging, development, test`)
  - Add `EnvironmentSource` typed alias + kubebuilder enum (`namespace-labels, rego-inference, default`) per BR-SP-080
  - Add `Priority` typed alias + kubebuilder enum (`P0, P1, P2, P3`)
  - Add `PrioritySource` typed alias + kubebuilder enum (`rego-policy, severity-fallback, default`)
- `pkg/signalprocessing/evaluator/evaluator.go`: Update `EvaluateEnvironment` and `EvaluatePriority` to use typed constants
- `internal/controller/signalprocessing/signalprocessing_controller.go`: Update `classifyBusiness` environment switch to use typed constants
- `pkg/signalprocessing/audit/helpers.go`: Fix audit mapper to accept BR-SP-080 source values (`namespace-labels` -> `Labels`, `rego-inference` -> `Rego`)
- `deploy/signalprocessing/policies/environment.rego`: Fix default source from `"unclassified"` to `"default"` (per BR-SP-080)
- CRD regeneration

### Out of Scope

- #458 (BusinessClassification criticality/SLA — prerequisite, done separately)
- Priority validation logic in evaluator (business logic unchanged)
- Rego policy matching rules (only the default source string changes)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Environment source enum: `namespace-labels, rego-inference, default` | Per BR-SP-080 V2.0 (authoritative) |
| Fix Rego default from `unclassified` to `default` | Aligns runtime with BR-SP-080 |
| Fix audit mapper to accept CRD values | Fixes pre-existing bug where `namespace-labels` fell through to empty |
| Audit OpenAPI enum unchanged (`rego, labels, default`) | Different vocabulary for storage layer; mapper is the translation layer |
| `test` added to environment enum | `classifyBusiness` doesn't handle it but Rego can produce it; CRD comment documents it |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of unit-testable code (evaluator, type constants, audit helpers)
- **Integration**: >=80% of integration-testable code (CRD schema validation, reconciler, Rego integration)

### 2-Tier Minimum

Both unit and integration tiers for defense-in-depth.

### Business Outcome Quality Bar

Tests validate that environment/priority values are schema-validated, correctly classified by evaluator and controller, and faithfully propagated through audit (fixing the pre-existing source mapping bug).

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| Type definitions (new) | 4 typed aliases + ~16 constants | ~40 (new) |
| `pkg/signalprocessing/evaluator/evaluator.go` | `EvaluateEnvironment`, `EvaluatePriority` | ~90 |
| `pkg/signalprocessing/audit/helpers.go` | `toSignalProcessingAuditPayloadEnvironmentSource`, `toSignalProcessingAuditPayloadEnvironment` | ~30 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/signalprocessing/signalprocessing_controller.go` | `reconcileCategorizing`, `classifyBusiness` | ~60 |
| `deploy/signalprocessing/policies/environment.rego` | Default source value | ~5 |
| CRD schema | kubebuilder enum validation on 4 fields | N/A |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SP-080 | Environment enum: production, staging, development, test | P0 | Unit | UT-SP-456-001 | Pending |
| BR-SP-080 | EnvironmentSource enum: namespace-labels, rego-inference, default | P0 | Unit | UT-SP-456-002 | Pending |
| BR-SP-080 | Priority enum: P0, P1, P2, P3 | P0 | Unit | UT-SP-456-003 | Pending |
| BR-SP-071 | PrioritySource enum: rego-policy, severity-fallback, default | P0 | Unit | UT-SP-456-004 | Pending |
| BR-SP-080 | EvaluateEnvironment returns typed Environment and EnvironmentSource | P0 | Unit | UT-SP-456-005 | Pending |
| BR-SP-071 | EvaluatePriority returns typed Priority and PrioritySource | P0 | Unit | UT-SP-456-006 | Pending |
| BR-SP-080 | Audit source mapper accepts CRD values (namespace-labels, rego-inference, default) | P0 | Unit | UT-SP-456-007 | Pending |
| BR-SP-080 | CRD rejects invalid environment value | P0 | Integration | IT-SP-456-001 | Pending |
| BR-SP-080 | CRD rejects invalid environment source value | P0 | Integration | IT-SP-456-002 | Pending |
| BR-SP-080 | CRD rejects invalid priority value | P0 | Integration | IT-SP-456-003 | Pending |
| BR-SP-080 | EnvironmentClassification round-trips through K8s API | P0 | Integration | IT-SP-456-004 | Pending |
| BR-SP-080 | PriorityAssignment round-trips through K8s API | P0 | Integration | IT-SP-456-005 | Pending |
| BR-SP-080 | Rego default source is `default` (not `unclassified`) | P0 | Integration | IT-SP-456-006 | Pending |

### Status Legend

- Pending / RED / GREEN / REFACTORED / Pass

---

## 5. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: Type constants (~40 lines), evaluator (~90 lines), audit helpers (~30 lines) — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-SP-456-001` | Environment type has 4 constants (production, staging, development, test) | Pending |
| `UT-SP-456-002` | EnvironmentSource type has 3 constants per BR-SP-080 | Pending |
| `UT-SP-456-003` | Priority type has 4 constants (P0, P1, P2, P3) | Pending |
| `UT-SP-456-004` | PrioritySource type has 3 constants (rego-policy, severity-fallback, default) | Pending |
| `UT-SP-456-005` | EvaluateEnvironment returns typed values from Rego result map | Pending |
| `UT-SP-456-006` | EvaluatePriority returns typed values with source `rego-policy` | Pending |
| `UT-SP-456-007` | Audit source mapper correctly translates all 3 BR-SP-080 source values (fixes pre-existing bug) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: CRD validation, reconciliation, Rego integration — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-SP-456-001` | CRD rejects invalid environment value | Pending |
| `IT-SP-456-002` | CRD rejects invalid environment source value | Pending |
| `IT-SP-456-003` | CRD rejects invalid priority value | Pending |
| `IT-SP-456-004` | EnvironmentClassification round-trips with typed values | Pending |
| `IT-SP-456-005` | PriorityAssignment round-trips with typed values | Pending |
| `IT-SP-456-006` | Rego environment default produces source `default` (not `unclassified`) | Pending |

### Tier Skip Rationale

- **E2E**: Existing SP E2E tests will validate end-to-end with updated types.

---

## 6. Test Cases (Detail)

### UT-SP-456-007: Audit source mapper fix

**BR**: BR-SP-080
**Type**: Unit
**File**: `test/unit/signalprocessing/audit_client_test.go`

**Given**: A CRD environment source value
**When**: `toSignalProcessingAuditPayloadEnvironmentSource` is called
**Then**: The correct audit enum is returned

**Acceptance Criteria**:
- `"namespace-labels"` -> `...SourceLabels` (was broken: fell through to empty)
- `"rego-inference"` -> `...SourceRego` (was broken: fell through to empty)
- `"default"` -> `...SourceDefault` (worked before, still works)

### IT-SP-456-006: Rego default source alignment

**BR**: BR-SP-080
**Type**: Integration
**File**: `test/integration/signalprocessing/rego_integration_test.go`

**Given**: A namespace without `kubernaut.ai/environment` label
**When**: The environment Rego policy evaluates
**Then**: The result source is `"default"` (not `"unclassified"`)

**Acceptance Criteria**:
- Rego output `source` field equals `"default"`
- This value passes CRD kubebuilder enum validation

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Rego evaluator mocks for unit tests
- **Location**: `test/unit/signalprocessing/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (envtest + real Rego evaluation)
- **Infrastructure**: envtest (CRD registration), real Rego policy files
- **Location**: `test/integration/signalprocessing/`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/signalprocessing/... -ginkgo.focus="UT-SP-456"

# Integration tests
go test ./test/integration/signalprocessing/... -ginkgo.focus="IT-SP-456"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
