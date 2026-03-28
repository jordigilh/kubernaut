# Test Plan: RemediationWorkflow and ActionType CatalogStatus Typed Enum (#459)

**Feature**: Add shared `CatalogStatus` typed enum for `status.catalogStatus` on RemediationWorkflow and ActionType CRDs
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- Coding standard: "AVOID using `any` or `interface{}` — ALWAYS use structured field values with specific types"
- RemediationWorkflow CRD schema: `status.catalogStatus` documents `active, disabled, deprecated, archived`
- ActionType CRD schema: `status.catalogStatus` documents `active, disabled`

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- `pkg/shared/types/`: New `CatalogStatus` typed string alias with kubebuilder enum + Go constants
- `api/remediationworkflow/v1alpha1/remediationworkflow_types.go`: Change `CatalogStatus string` to use shared type
- `api/actiontype/v1alpha1/actiontype_types.go`: Change `CatalogStatus string` to use shared type
- `pkg/authwebhook/remediationworkflow_handler.go`: Use typed constants for catalog status assignments
- `pkg/authwebhook/actiontype_handler.go`: Use typed constants for catalog status assignments
- CRD regeneration for both RemediationWorkflow and ActionType CRDs

### Out of Scope

- Gateway catalog integration (does not read `catalogStatus` from these CRDs)
- DataStorage catalog status (separate audit concern)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Shared `CatalogStatus` type in `pkg/shared/types/` | Both CRDs share the same lifecycle semantics |
| Enum values: `active, invalid, pending, deprecated, archived` | Superset from RemediationWorkflow (most complete lifecycle) |
| Issue body correction: producers are in `pkg/authwebhook/` | Not `internal/controller/` as the issue states |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of unit-testable code (type constants, handler assignment logic)
- **Integration**: >=80% of integration-testable code (CRD round-trip, schema validation)

### 2-Tier Minimum

Both unit and integration tiers for defense-in-depth.

### Business Outcome Quality Bar

Tests validate that catalog status values are schema-validated and correctly assigned by authwebhook handlers.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/types/` (new) | `CatalogStatus` type, constants | ~15 (new) |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/authwebhook/remediationworkflow_handler.go` | Status update with `CatalogStatus` | ~10 |
| `pkg/authwebhook/actiontype_handler.go` | Status update with `CatalogStatus` | ~10 |
| CRD schemas | kubebuilder enum validation on `catalogStatus` | N/A |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| Coding Std | CatalogStatus enum values: active, invalid, pending, deprecated, archived | P1 | Unit | UT-AW-459-001 | Pending |
| Coding Std | CRD rejects invalid catalogStatus on RemediationWorkflow | P1 | Integration | IT-AW-459-001 | Pending |
| Coding Std | CRD rejects invalid catalogStatus on ActionType | P1 | Integration | IT-AW-459-002 | Pending |
| Coding Std | RW catalogStatus round-trips through K8s API | P1 | Integration | IT-AW-459-003 | Pending |
| Coding Std | AT catalogStatus round-trips through K8s API | P1 | Integration | IT-AW-459-004 | Pending |
| Coding Std | Authwebhook RW handler assigns typed CatalogStatus | P1 | Unit | UT-AW-459-002 | Pending |
| Coding Std | Authwebhook AT handler assigns typed CatalogStatus | P1 | Unit | UT-AW-459-003 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: Type constants (~15 lines), handler assignment logic (~20 lines) — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AW-459-001` | CatalogStatus type has exactly 5 valid constants (active, invalid, pending, deprecated, archived) | Pending |
| `UT-AW-459-002` | RW authwebhook handler assigns CatalogStatus from DS registration result using typed constant | Pending |
| `UT-AW-459-003` | AT authwebhook handler assigns `CatalogStatusActive` typed constant | Pending |

### Tier 2: Integration Tests

**Testable code scope**: CRD schema validation, round-trip — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-AW-459-001` | CRD API server rejects a RemediationWorkflow with `catalogStatus: "bogus"` | Pending |
| `IT-AW-459-002` | CRD API server rejects an ActionType with `catalogStatus: "bogus"` | Pending |
| `IT-AW-459-003` | RemediationWorkflow catalogStatus round-trips through K8s API with typed value | Pending |
| `IT-AW-459-004` | ActionType catalogStatus round-trips through K8s API with typed value | Pending |

### Tier Skip Rationale

- **E2E**: Not applicable — enum typing is fully validated by unit + integration. No end-to-end behavior changes.

---

## 6. Test Cases (Detail)

### UT-AW-459-001: CatalogStatus enum completeness

**BR**: Coding Standard
**Type**: Unit
**File**: `test/unit/shared/types/catalog_status_test.go`

**Given**: The `CatalogStatus` type and its constants are defined
**When**: All documented catalog status values are checked
**Then**: Exactly 5 constants exist

**Acceptance Criteria**:
- `CatalogStatusActive` == `"active"`
- `CatalogStatusInvalid` == `"invalid"`
- `CatalogStatusPending` == `"pending"`
- `CatalogStatusDeprecated` == `"deprecated"`
- `CatalogStatusArchived` == `"archived"`

### UT-AW-459-002: RW handler assigns typed CatalogStatus

**BR**: Coding Standard
**Type**: Unit
**File**: `test/unit/authwebhook/remediationworkflow_handler_test.go`

**Given**: A DS registration result with status `"active"`
**When**: The RW authwebhook handler processes the result
**Then**: `rw.Status.CatalogStatus` is set to a typed `CatalogStatus` constant

**Acceptance Criteria**:
- Handler uses `CatalogStatusActive` (or equivalent) instead of string literal `"active"`

### UT-AW-459-003: AT handler assigns typed CatalogStatus

**BR**: Coding Standard
**Type**: Unit
**File**: `test/unit/authwebhook/actiontype_handler_test.go`

**Given**: An ActionType registration succeeds
**When**: The AT authwebhook handler processes the result
**Then**: `at.Status.CatalogStatus` is set to `CatalogStatusActive`

**Acceptance Criteria**:
- Handler uses typed constant instead of string literal `"active"`

### IT-AW-459-001: CRD rejects invalid RW catalogStatus

**BR**: Coding Standard
**Type**: Integration
**File**: `test/integration/authwebhook/catalog_status_integration_test.go`

**Given**: A RemediationWorkflow CR with `status.catalogStatus: "bogus"`
**When**: The CR status is updated via the K8s API
**Then**: The API server rejects the update

**Acceptance Criteria**:
- Status update returns a validation error

### IT-AW-459-002: CRD rejects invalid AT catalogStatus

**BR**: Coding Standard
**Type**: Integration
**File**: `test/integration/authwebhook/catalog_status_integration_test.go`

**Given**: An ActionType CR with `status.catalogStatus: "bogus"`
**When**: The CR status is updated via the K8s API
**Then**: The API server rejects the update

**Acceptance Criteria**:
- Status update returns a validation error

### IT-AW-459-003: RW catalogStatus round-trip

**BR**: Coding Standard
**Type**: Integration
**File**: `test/integration/authwebhook/catalog_status_integration_test.go`

**Given**: A RemediationWorkflow CR with valid `catalogStatus: "active"`
**When**: The CR is created and read back
**Then**: `catalogStatus` preserves its value

**Acceptance Criteria**:
- Read-back value equals `CatalogStatusActive`

### IT-AW-459-004: AT catalogStatus round-trip

**BR**: Coding Standard
**Type**: Integration
**File**: `test/integration/authwebhook/catalog_status_integration_test.go`

**Given**: An ActionType CR with valid `catalogStatus: "active"`
**When**: The CR is created and read back
**Then**: `catalogStatus` preserves its value

**Acceptance Criteria**:
- Read-back value equals `CatalogStatusActive`

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None (type constant tests), existing mocks for handler tests
- **Location**: `test/unit/shared/types/`, `test/unit/authwebhook/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (envtest with real K8s API)
- **Infrastructure**: envtest (CRD registration for RW + AT)
- **Location**: `test/integration/authwebhook/`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/shared/types/... -ginkgo.focus="UT-AW-459"
go test ./test/unit/authwebhook/... -ginkgo.focus="UT-AW-459"

# Integration tests
go test ./test/integration/authwebhook/... -ginkgo.focus="IT-AW-459"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
