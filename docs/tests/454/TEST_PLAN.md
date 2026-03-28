# Test Plan: RemediationRequest CRD Typed Enums (#454)

**Feature**: Add typed enums for `skipReason`, `blockReason`, `failurePhase`, `timeoutPhase` status fields
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- DD-RO-002: Centralized routing responsibility (skip/block semantics)
- DD-RO-002-ADDENDUM: Blocked Phase Semantics (BlockReason values)
- BR-COMMON-001: Phase Value Format Standard (PascalCase for CRD phase values)
- Coding standard: "AVOID using `any` or `interface{}` — ALWAYS use structured field values with specific types"

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- `api/remediation/v1alpha1/remediationrequest_types.go`:
  - Add `SkipReason` type alias + 4 constants + kubebuilder enum
  - Change `BlockReason` field type from `string` to existing `BlockReason` type + add kubebuilder enum
  - Add `FailurePhase` type alias + 6 PascalCase constants + kubebuilder enum
  - Change `TimeoutPhase` from `*string` to `*RemediationPhase` (reuse existing type)
- `internal/controller/remediationorchestrator/reconciler.go`: Update `transitionToFailed` + timeout producers to use typed constants (PascalCase)
- `pkg/remediationorchestrator/handler/skip/*.go`: Update skip reason assignments to typed constants
- `pkg/remediationorchestrator/audit/manager.go`: Simplify `ToOptFailurePhase` to PascalCase only
- `api/openapi/data-storage-v1.yaml`: Add `Configuration` and `Blocked` to `failure_phase` enum
- Regenerate ogen client + CRD manifests

### Out of Scope

- Other RemediationRequest status fields (already properly typed)
- Skip/block routing logic changes (only the assigned string values change)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| `failurePhase` uses PascalCase | Per BR-COMMON-001; consistent with `RemediationPhase`, `timeoutPhase`, and audit API |
| `timeoutPhase` reuses `RemediationPhase` type | Already derived from `string(OverallPhase)` which is `RemediationPhase` |
| `BlockReason` field uses existing type | Type + 7 constants already exist, field was an oversight |
| `ToOptFailurePhase` drops dual-casing | No backward compatibility needed (v1.2 dev branch) |
| Add `Configuration` + `Blocked` to audit `failure_phase` enum | These values are produced but were missing from the DS schema |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of unit-testable code (skip handlers, `transitionToFailed`, audit mapper, type constants)
- **Integration**: >=80% of integration-testable code (CRD schema validation, round-trip, reconciler status updates)

### 2-Tier Minimum

Both unit and integration tiers for defense-in-depth.

### Business Outcome Quality Bar

Tests validate that status field values are schema-validated, PascalCase-consistent, and correctly propagated through audit.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `api/remediation/v1alpha1/remediationrequest_types.go` | `SkipReason`, `FailurePhase` types + constants | ~30 (new) |
| `pkg/remediationorchestrator/handler/skip/*.go` | 4 skip handlers (skipReason assignment) | ~20 (across 4 files) |
| `pkg/remediationorchestrator/audit/manager.go` | `ToOptFailurePhase` | ~20 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `transitionToFailed`, timeout handlers | ~50 |
| `internal/controller/remediationorchestrator/blocking.go` | `handleBlocked` (blockReason, failurePhase) | ~30 |
| CRD schema | kubebuilder enum validation on 4 fields | N/A |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| DD-RO-002 | SkipReason enum: RecentlyRemediated, ResourceBusy, ExhaustedRetries, PreviousExecutionFailed | P0 | Unit | UT-RO-454-001 | Pending |
| DD-RO-002-ADDENDUM | BlockReason field uses existing BlockReason type with 7 constants | P0 | Unit | UT-RO-454-002 | Pending |
| BR-COMMON-001 | FailurePhase enum (PascalCase): Configuration, SignalProcessing, AIAnalysis, Approval, WorkflowExecution, Blocked | P0 | Unit | UT-RO-454-003 | Pending |
| BR-COMMON-001 | TimeoutPhase reuses RemediationPhase type | P0 | Unit | UT-RO-454-004 | Pending |
| DD-RO-002 | Skip handlers assign typed SkipReason constants | P0 | Unit | UT-RO-454-005 | Pending |
| BR-COMMON-001 | ToOptFailurePhase maps PascalCase FailurePhase to audit enum (including Configuration, Blocked) | P0 | Unit | UT-RO-454-006 | Pending |
| DD-RO-002 | CRD rejects invalid skipReason | P0 | Integration | IT-RO-454-001 | Pending |
| DD-RO-002-ADDENDUM | CRD rejects invalid blockReason | P0 | Integration | IT-RO-454-002 | Pending |
| BR-COMMON-001 | CRD rejects invalid failurePhase | P0 | Integration | IT-RO-454-003 | Pending |
| DD-RO-002 | RR status fields round-trip through K8s API with typed values | P0 | Integration | IT-RO-454-004 | Pending |
| BR-COMMON-001 | transitionToFailed assigns PascalCase FailurePhase constants | P0 | Integration | IT-RO-454-005 | Pending |

### Status Legend

- Pending / RED / GREEN / REFACTORED / Pass

---

## 5. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: Type constants (~30 lines), skip handlers (~20 lines), audit mapper (~20 lines) — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-454-001` | SkipReason type has exactly 4 valid constants | Pending |
| `UT-RO-454-002` | BlockReason field type matches existing BlockReason type with 7 constants | Pending |
| `UT-RO-454-003` | FailurePhase type has exactly 6 PascalCase constants | Pending |
| `UT-RO-454-004` | TimeoutPhase field type is `*RemediationPhase` | Pending |
| `UT-RO-454-005` | Each skip handler assigns the correct typed SkipReason constant | Pending |
| `UT-RO-454-006` | ToOptFailurePhase maps all 6 PascalCase FailurePhase values to audit enum (including Configuration, Blocked) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: CRD schema validation, reconciler status updates — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-454-001` | CRD rejects invalid skipReason value | Pending |
| `IT-RO-454-002` | CRD rejects invalid blockReason value | Pending |
| `IT-RO-454-003` | CRD rejects invalid failurePhase value | Pending |
| `IT-RO-454-004` | All 4 typed status fields round-trip through K8s API | Pending |
| `IT-RO-454-005` | transitionToFailed stores PascalCase FailurePhase on RR status | Pending |

### Tier Skip Rationale

- **E2E**: Not applicable — enum typing validated by unit + integration.

---

## 6. Test Cases (Detail)

### UT-RO-454-001: SkipReason enum completeness

**BR**: DD-RO-002
**Type**: Unit
**File**: `test/unit/remediationorchestrator/types/skip_reason_test.go`

**Given**: The `SkipReason` type and its constants are defined
**When**: All documented skip reason values are checked
**Then**: Exactly 4 constants exist

**Acceptance Criteria**:
- `SkipReasonRecentlyRemediated` == `"RecentlyRemediated"`
- `SkipReasonResourceBusy` == `"ResourceBusy"`
- `SkipReasonExhaustedRetries` == `"ExhaustedRetries"`
- `SkipReasonPreviousExecutionFailed` == `"PreviousExecutionFailed"`

### UT-RO-454-003: FailurePhase PascalCase enum

**BR**: BR-COMMON-001
**Type**: Unit
**File**: `test/unit/remediationorchestrator/types/failure_phase_test.go`

**Given**: The `FailurePhase` type and its constants are defined
**When**: All failure phase values are checked
**Then**: 6 PascalCase constants exist

**Acceptance Criteria**:
- `FailurePhaseConfiguration` == `"Configuration"`
- `FailurePhaseSignalProcessing` == `"SignalProcessing"`
- `FailurePhaseAIAnalysis` == `"AIAnalysis"`
- `FailurePhaseApproval` == `"Approval"`
- `FailurePhaseWorkflowExecution` == `"WorkflowExecution"`
- `FailurePhaseBlocked` == `"Blocked"`

### UT-RO-454-005: Skip handlers assign typed constants

**BR**: DD-RO-002
**Type**: Unit
**File**: `test/unit/remediationorchestrator/skip_handler_test.go`

**Given**: A RemediationRequest eligible for each skip reason
**When**: Each skip handler processes the RR
**Then**: `rr.Status.SkipReason` is set to the corresponding typed constant

**Acceptance Criteria**:
- `recently_remediated.go` -> `SkipReasonRecentlyRemediated`
- `resource_busy.go` -> `SkipReasonResourceBusy`
- `exhausted_retries.go` -> `SkipReasonExhaustedRetries`
- `previous_execution_failed.go` -> `SkipReasonPreviousExecutionFailed`

### UT-RO-454-006: Audit mapper handles all FailurePhase values

**BR**: BR-COMMON-001
**Type**: Unit
**File**: `test/unit/remediationorchestrator/audit/manager_test.go`

**Given**: A PascalCase FailurePhase value
**When**: `ToOptFailurePhase` is called
**Then**: The correct audit enum is returned for all 6 values

**Acceptance Criteria**:
- `"Configuration"` -> audit `FailurePhaseConfiguration` (new)
- `"SignalProcessing"` -> audit `FailurePhaseSignalProcessing`
- `"AIAnalysis"` -> audit `FailurePhaseAIAnalysis`
- `"Approval"` -> audit `FailurePhaseApproval`
- `"WorkflowExecution"` -> audit `FailurePhaseWorkflowExecution`
- `"Blocked"` -> audit `FailurePhaseBlocked` (new)

### IT-RO-454-004: Status fields round-trip

**BR**: DD-RO-002, BR-COMMON-001
**Type**: Integration
**File**: `test/integration/remediationorchestrator/typed_status_fields_integration_test.go`

**Given**: An RR with all 4 typed status fields set to valid values
**When**: The RR status is updated and read back via K8s API
**Then**: All 4 fields preserve their typed enum values

**Acceptance Criteria**:
- `skipReason` round-trips correctly
- `blockReason` round-trips correctly
- `failurePhase` round-trips correctly
- `timeoutPhase` round-trips correctly

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: External dependencies only (K8s client for skip handler tests)
- **Location**: `test/unit/remediationorchestrator/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (envtest with real K8s API)
- **Infrastructure**: envtest (CRD registration for RemediationRequest)
- **Location**: `test/integration/remediationorchestrator/`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-454"

# Integration tests
go test ./test/integration/remediationorchestrator/... -ginkgo.focus="IT-RO-454"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
