# Test Plan: WorkflowExecution CRD Typed Fields (#455)

**Feature**: Replace duration strings with `metav1.Duration`, type `executionStatus.status` as `corev1.ConditionStatus`, fix Ansible executor Status semantics
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- Coding standard: "AVOID using `any` or `interface{}` — ALWAYS use structured field values with specific types"
- `ExecutionStatusSummary.Status` comment: "Status of the execution resource (Unknown, True, False)"
- `metav1.Duration` precedent: already used for `ExecutionConfig.Timeout` in the same CRD

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- `api/workflowexecution/v1alpha1/workflowexecution_types.go`:
  - Change `Duration string` to `Duration *metav1.Duration`
  - Change `ExecutionTimeBeforeFailure string` to `ExecutionTimeBeforeFailure *metav1.Duration`
  - Change `ExecutionStatusSummary.Status string` to `Status corev1.ConditionStatus`
- `internal/controller/workflowexecution/workflowexecution_controller.go`: Update duration/status assignments
- `internal/controller/workflowexecution/failure_analysis.go`: Update executionTimeBeforeFailure assignment
- `pkg/workflowexecution/executor/ansible.go`: Fix `MapAWXStatusToResult` to use ConditionStatus (Completed->True, Failed->False, Pending/Running->Unknown)
- `pkg/workflowexecution/executor/tekton.go`: Use `corev1.ConditionStatus` type directly
- `pkg/workflowexecution/executor/job.go`: Use `corev1.ConditionStatus` constants
- `pkg/workflowexecution/audit/manager.go`: Read duration from `metav1.Duration`

### Out of Scope

- `ExecutionStatusSummary.Reason` and `.Message` (free-form strings, appropriate as-is)
- `ExecutionConfig.Timeout` (already `*metav1.Duration`)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| `metav1.Duration` for both duration fields | Already used in same CRD; identical JSON serialization (string format) |
| Preserve `Round(time.Second)` before assignment | Maintains current data quality; sub-second precision not needed |
| `corev1.ConditionStatus` for ExecutionStatusSummary.Status | Documented contract is True/False/Unknown; Tekton and Job already comply |
| Fix Ansible: Completed->True, Failed->False, Pending/Running->Unknown | Ansible violated documented contract; phase detail preserved in `.Reason` |
| Pointer `*metav1.Duration` (not value) | Matches `ExecutionConfig.Timeout` pattern; allows omitempty |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of unit-testable code (executor status mapping, duration assignment, audit reader)
- **Integration**: >=80% of integration-testable code (CRD round-trip, reconciler status updates)

### 2-Tier Minimum

Both unit and integration tiers for defense-in-depth.

### Business Outcome Quality Bar

Tests validate duration serialization fidelity, ConditionStatus consistency across all 3 engines, and CRD round-trip correctness.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/workflowexecution/executor/tekton.go` | `buildStatusSummary` | ~35 |
| `pkg/workflowexecution/executor/job.go` | `buildStatusSummary` | ~25 |
| `pkg/workflowexecution/executor/ansible.go` | `MapAWXStatusToResult`, `mapAWXStatusToPhase` | ~45 |
| `internal/controller/workflowexecution/failure_analysis.go` | Duration assignment | ~10 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `MarkCompleted`, `MarkFailed`, `BuildPipelineRunStatusSummary` | ~80 |
| `pkg/workflowexecution/audit/manager.go` | Duration reading for audit payload | ~10 |
| CRD schema | Type changes on `duration`, `executionTimeBeforeFailure`, `executionStatus.status` | N/A |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| Coding Std | Duration fields use metav1.Duration with correct serialization | P0 | Unit | UT-WE-455-001 | Pending |
| Coding Std | ExecutionTimeBeforeFailure uses metav1.Duration | P0 | Unit | UT-WE-455-002 | Pending |
| Coding Std | Tekton executor returns corev1.ConditionStatus | P0 | Unit | UT-WE-455-003 | Pending |
| Coding Std | Job executor returns corev1.ConditionStatus | P0 | Unit | UT-WE-455-004 | Pending |
| Coding Std | Ansible executor maps AWX status to ConditionStatus | P0 | Unit | UT-WE-455-005 | Pending |
| Coding Std | Audit reads duration from metav1.Duration field | P0 | Unit | UT-WE-455-006 | Pending |
| Coding Std | Duration round-trips through K8s API as metav1.Duration | P0 | Integration | IT-WE-455-001 | Pending |
| Coding Std | ExecutionStatus.Status round-trips as ConditionStatus | P0 | Integration | IT-WE-455-002 | Pending |
| Coding Std | MarkCompleted stores metav1.Duration and ConditionStatus | P0 | Integration | IT-WE-455-003 | Pending |
| Coding Std | MarkFailed stores metav1.Duration and ConditionStatus | P0 | Integration | IT-WE-455-004 | Pending |

### Status Legend

- Pending / RED / GREEN / REFACTORED / Pass

---

## 5. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: Executor status builders (~105 lines), failure analysis (~10 lines), audit reader (~10 lines) — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-WE-455-001` | Controller duration assignment produces metav1.Duration with rounded seconds | Pending |
| `UT-WE-455-002` | Failure analysis produces metav1.Duration for executionTimeBeforeFailure | Pending |
| `UT-WE-455-003` | Tekton buildStatusSummary returns corev1.ConditionTrue/False/Unknown | Pending |
| `UT-WE-455-004` | Job buildStatusSummary returns corev1.ConditionTrue/False/Unknown | Pending |
| `UT-WE-455-005` | Ansible MapAWXStatusToResult: successful->True, failed/error/canceled->False, pending/running->Unknown | Pending |
| `UT-WE-455-006` | Audit manager reads Duration.Duration.String() from metav1.Duration field | Pending |

### Tier 2: Integration Tests

**Testable code scope**: CRD round-trip, MarkCompleted/MarkFailed — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-WE-455-001` | metav1.Duration round-trips through K8s API (create, read, verify string format) | Pending |
| `IT-WE-455-002` | corev1.ConditionStatus round-trips through K8s API (True, False, Unknown) | Pending |
| `IT-WE-455-003` | MarkCompleted stores both duration and status correctly on WFE CR | Pending |
| `IT-WE-455-004` | MarkFailed stores duration and failure details correctly on WFE CR | Pending |

### Tier Skip Rationale

- **E2E**: Not applicable for type changes. Existing E2E tests will validate end-to-end with updated types.

---

## 6. Test Cases (Detail)

### UT-WE-455-005: Ansible Status mapping to ConditionStatus

**BR**: Coding Standard
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: An AWX job status string
**When**: `MapAWXStatusToResult` is called
**Then**: `Summary.Status` is a `corev1.ConditionStatus` value

**Acceptance Criteria**:
- `"successful"` -> `corev1.ConditionTrue`, Reason: `"AWXJobSuccessful"`
- `"failed"` -> `corev1.ConditionFalse`, Reason: `"AWXJobFailed"`
- `"error"` -> `corev1.ConditionFalse`, Reason: `"AWXJobError"`
- `"canceled"` -> `corev1.ConditionFalse`, Reason: `"AWXJobCanceled"`
- `"pending"` -> `corev1.ConditionUnknown`, Reason: `"AWXJobPending"`
- `"running"` -> `corev1.ConditionUnknown`, Reason: `"AWXJobRunning"`
- Unknown string -> `corev1.ConditionUnknown`, Reason: `"AWXJobUnknown"`

### IT-WE-455-001: Duration round-trip

**BR**: Coding Standard
**Type**: Integration
**File**: `test/integration/workflowexecution/typed_fields_integration_test.go`

**Given**: A WFE CR with `status.duration` set to `metav1.Duration{Duration: 30 * time.Second}`
**When**: The CR is updated and read back via K8s API
**Then**: The duration value is preserved

**Acceptance Criteria**:
- Read-back `Duration.Duration` equals `30 * time.Second`
- JSON serialization is `"30s"`

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Tekton/Job/Ansible test fixtures
- **Location**: `test/unit/workflowexecution/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (envtest with real K8s API)
- **Infrastructure**: envtest (CRD registration for WorkflowExecution)
- **Location**: `test/integration/workflowexecution/`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-455"

# Integration tests
go test ./test/integration/workflowexecution/... -ginkgo.focus="IT-WE-455"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
