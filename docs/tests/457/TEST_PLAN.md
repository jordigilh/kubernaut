# Test Plan: AIAnalysis CRD Typed Enums and Field Rename (#457)

**Feature**: Type `analysisTypes`, `reason`, `policyEvaluation.decision`, and rename `recommendedActions[].action` to `workflowId`
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- Coding standard: "AVOID using `any` or `interface{}` — ALWAYS use structured field values with specific types"
- AIAnalysis CRD: `status.subReason` already has kubebuilder enum (reason was missed)
- `RecommendedAction.Action` semantic drift: populated with `WorkflowID`, not action taxonomy

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- Depends on #458 (shared BusinessClassification types)

---

## 1. Scope

### In Scope

- `api/aianalysis/v1alpha1/aianalysis_types.go`:
  - Add `AnalysisType` typed alias + kubebuilder enum (`investigation, root-cause, workflow-selection`)
  - Change `AnalysisTypes []string` to `AnalysisTypes []AnalysisType`
  - Add `AIAnalysisReason` typed alias + kubebuilder enum (6 values)
  - Change `Reason string` to `Reason AIAnalysisReason`
  - Add `PolicyDecision` typed alias + kubebuilder enum (`approved, manual_review_required, denied, degraded_mode`)
  - Change `Decision string` to `Decision PolicyDecision`
  - Rename `RecommendedAction.Action` to `RecommendedAction.WorkflowId` (Go field + JSON tag)
- `pkg/aianalysis/handlers/analyzing.go`: Use typed constants for decision, reason, analysisTypes
- `pkg/aianalysis/handlers/response_processor.go`: Use typed `AIAnalysisReason` constants
- `pkg/aianalysis/handlers/investigating.go`: Use typed `AIAnalysisReason` constants
- `pkg/remediationorchestrator/creator/aianalysis.go`: Use `AnalysisType` constants for `analysisTypes` slice
- CRD regeneration

### Out of Scope

- `status.subReason` (already has kubebuilder enum — no changes needed)
- HolmesGPT API response shape (separate system, maps through handlers)
- `RecommendedAction.Rationale` (string field, appropriate as-is)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| `AIAnalysisReason` values: WorkflowResolutionFailed, WorkflowNotNeeded, NoWorkflowSelected, RegoEvaluationError, TransientError, APIError | All 6 values found in production code |
| `PolicyDecision` includes `degraded_mode` | Produced in code but was undocumented in CRD comment; now explicit |
| `action` -> `workflowId` (Go field + JSON tag rename) | v1.2 dev branch, no backward compat needed; fixes semantic drift |
| ~15 files affected by rename | Only 1 production Go file, 3 test files, rest are CRD/docs (regenerated) |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of unit-testable code (handler reason/decision assignments, type constants, creator)
- **Integration**: >=80% of integration-testable code (CRD schema validation, round-trip, enum rejection)

### 2-Tier Minimum

Both unit and integration tiers for defense-in-depth.

### Business Outcome Quality Bar

Tests validate that analysis types are schema-validated, reason codes are type-safe, policy decisions include all actual values, and the workflowId rename is propagated correctly.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| Type definitions (new) | `AnalysisType`, `AIAnalysisReason`, `PolicyDecision` + constants | ~50 (new) |
| `pkg/aianalysis/handlers/analyzing.go` | `populateApprovalContext`, workflow resolution | ~60 |
| `pkg/aianalysis/handlers/response_processor.go` | Reason/SubReason assignments | ~80 |
| `pkg/aianalysis/handlers/investigating.go` | Error reason assignments | ~30 |
| `pkg/remediationorchestrator/creator/aianalysis.go` | `analysisTypes` slice construction | ~10 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| CRD schema | kubebuilder enum validation, JSON field rename | N/A |
| Reconciler + handlers | Full flow writing typed values to CRD status | ~50 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| Coding Std | AnalysisType enum: investigation, root-cause, workflow-selection | P0 | Unit | UT-AA-457-001 | Pending |
| Coding Std | AIAnalysisReason enum: 6 values matching production code | P0 | Unit | UT-AA-457-002 | Pending |
| Coding Std | PolicyDecision enum: approved, manual_review_required, denied, degraded_mode | P0 | Unit | UT-AA-457-003 | Pending |
| Coding Std | RecommendedAction.WorkflowId field replaces Action | P0 | Unit | UT-AA-457-004 | Pending |
| Coding Std | RO creator uses AnalysisType constants for analysisTypes | P0 | Unit | UT-AA-457-005 | Pending |
| Coding Std | Handler assigns typed AIAnalysisReason constants | P0 | Unit | UT-AA-457-006 | Pending |
| Coding Std | Handler assigns typed PolicyDecision constants | P0 | Unit | UT-AA-457-007 | Pending |
| Coding Std | CRD rejects invalid analysisType value | P0 | Integration | IT-AA-457-001 | Pending |
| Coding Std | CRD rejects invalid reason value | P0 | Integration | IT-AA-457-002 | Pending |
| Coding Std | CRD rejects invalid policy decision value | P0 | Integration | IT-AA-457-003 | Pending |
| Coding Std | CRD JSON uses `workflowId` key (not `action`) | P0 | Integration | IT-AA-457-004 | Pending |
| Coding Std | AIAnalysis status round-trips through K8s API with all typed fields | P0 | Integration | IT-AA-457-005 | Pending |
| Coding Std | analysisTypes array round-trips with typed values | P0 | Integration | IT-AA-457-006 | Pending |

### Status Legend

- Pending / RED / GREEN / REFACTORED / Pass

---

## 5. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: Type constants (~50 lines), handler assignments (~170 lines), creator (~10 lines) — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AA-457-001` | AnalysisType type has exactly 3 constants | Pending |
| `UT-AA-457-002` | AIAnalysisReason type has exactly 6 constants | Pending |
| `UT-AA-457-003` | PolicyDecision type has exactly 4 constants (including degraded_mode) | Pending |
| `UT-AA-457-004` | RecommendedAction struct has `WorkflowId` field (not `Action`) | Pending |
| `UT-AA-457-005` | RO aianalysis creator builds analysisTypes with typed constants | Pending |
| `UT-AA-457-006` | Response processor assigns typed AIAnalysisReason for all 6 code paths | Pending |
| `UT-AA-457-007` | Analyzing handler assigns typed PolicyDecision (manual_review_required, degraded_mode) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: CRD schema validation, JSON rename, round-trip — target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-AA-457-001` | CRD rejects analysisTypes with invalid value (e.g., `"bogus"`) | Pending |
| `IT-AA-457-002` | CRD rejects invalid reason value | Pending |
| `IT-AA-457-003` | CRD rejects invalid policy decision value | Pending |
| `IT-AA-457-004` | CRD JSON serialization uses `workflowId` key (not `action`) on RecommendedAction | Pending |
| `IT-AA-457-005` | AIAnalysis with typed reason, decision, and workflowId round-trips through K8s API | Pending |
| `IT-AA-457-006` | analysisTypes array with all 3 typed values round-trips correctly | Pending |

### Tier Skip Rationale

- **E2E**: Existing AA E2E tests will validate end-to-end with updated types.

---

## 6. Test Cases (Detail)

### UT-AA-457-004: RecommendedAction field rename

**BR**: Coding Standard
**Type**: Unit
**File**: `test/unit/aianalysis/types_test.go`

**Given**: A `RecommendedAction` struct
**When**: The `WorkflowId` field is set
**Then**: The struct serializes with JSON key `"workflowId"` (not `"action"`)

**Acceptance Criteria**:
- `json.Marshal(RecommendedAction{WorkflowId: "wf-123"})` contains `"workflowId":"wf-123"`
- No `"action"` key in JSON output

### UT-AA-457-006: Response processor assigns typed reasons

**BR**: Coding Standard
**Type**: Unit
**File**: `test/unit/aianalysis/analyzing_handler_test.go`

**Given**: Various response processing outcomes
**When**: The response processor sets `status.reason`
**Then**: The value is a typed `AIAnalysisReason` constant

**Acceptance Criteria**:
- Workflow resolution failure -> `ReasonWorkflowResolutionFailed`
- Workflow not needed (resolved) -> `ReasonWorkflowNotNeeded`
- No workflow selected -> `ReasonNoWorkflowSelected`
- Rego evaluation error -> `ReasonRegoEvaluationError`
- Transient error -> `ReasonTransientError`
- API error -> `ReasonAPIError`

### IT-AA-457-004: JSON field rename verification

**BR**: Coding Standard
**Type**: Integration
**File**: `test/integration/aianalysis/typed_fields_integration_test.go`

**Given**: An AIAnalysis CR with `approvalContext.recommendedActions[].workflowId` set
**When**: The CR is created and read back via K8s API
**Then**: The `workflowId` field is present (not `action`)

**Acceptance Criteria**:
- Read-back `RecommendedActions[0].WorkflowId` equals the original value
- Raw JSON from K8s API contains `"workflowId"` key

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: External dependencies only (HolmesGPT client mocks for handler tests)
- **Location**: `test/unit/aianalysis/`, `test/unit/remediationorchestrator/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (envtest with real K8s API)
- **Infrastructure**: envtest (CRD registration for AIAnalysis)
- **Location**: `test/integration/aianalysis/`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/aianalysis/... -ginkgo.focus="UT-AA-457"
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-AA-457"

# Integration tests
go test ./test/integration/aianalysis/... -ginkgo.focus="IT-AA-457"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
