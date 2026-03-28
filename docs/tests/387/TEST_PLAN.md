# Test Plan: RemediationRequest Wide Printer Columns

**Feature**: Add wide printer columns to RemediationRequest CRD for operational triage
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `development/v1.2`

**Authority**:
- [BR-HAPI-191]: LLM may identify a higher-level resource than the signal source
- [DD-HAPI-006 v1.2]: AffectedResource defense-in-depth chain
- [DD-CRD-003]: Printer columns for operational triage

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Issue #387](https://github.com/jordigilh/kubernaut/issues/387)

---

## 1. Scope

### In Scope

- **CRD manifest columns**: 4 new `priority=1` printer columns (Source, Target, RCA Target, Workflow)
- **New status field**: `RemediationTarget *ResourceIdentifier` on `RemediationRequestStatus`
- **Reconciler population**: Set `RemediationTarget` from `AIAnalysis.Status.RootCauseAnalysis.AffectedResource` on both direct and post-approval WFE creation paths
- **Type conversion**: `aianalysisv1.AffectedResource` -> `remediationv1.ResourceIdentifier` (3-field copy)

### Out of Scope

- E2E tests (no cluster-level validation in this ticket)
- Existing printer columns (Phase, Outcome, Reason, Age are unchanged)
- `resolveDualTargets` behavior (existing logic, not modified)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use `ResourceIdentifier` (not new type) for `RemediationTarget` | Same schema as `Spec.TargetResource`, reuses existing `String()` method |
| Populate only from AI `AffectedResource`, not spec fallback | "Target" column already shows `spec.targetResource.name`; "RCA Target" should only show LLM-identified resource |
| Set in same status update as `SelectedWorkflowRef` | Avoids extra API write; atomic update with existing pattern |

---

## 2. Coverage Policy

### Tier Skip Rationale

- **Integration**: SKIPPED -- No new I/O paths. The reconciler's status update path is unchanged; only the payload is extended.
- **E2E**: SKIPPED -- Printer columns are a display concern verified by CRD manifest inspection.

### Unit Tier

| ID | Scenario | Validates |
|----|----------|-----------|
| UT-RR-387-001 | CRD manifest contains 4 new wide columns with correct names, JSONPaths, and `priority: 1` | DD-CRD-003: Column annotations generate correct YAML |
| UT-RR-387-002 | `RemediationTarget` populated from AIAnalysis AffectedResource on direct execution path (Analyzing -> Executing) | BR-HAPI-191: LLM target flows to RR status |
| UT-RR-387-003 | `RemediationTarget` populated from AIAnalysis AffectedResource on post-approval path (AwaitingApproval -> Executing) | BR-HAPI-191: LLM target flows to RR status after approval |

---

## 3. Test Scenarios

### UT-RR-387-001: CRD Manifest Printer Columns

**File**: `test/unit/remediationorchestrator/crd_manifest_test.go`
**Pattern**: Follows `test/unit/actiontype/types_test.go` (UT-AT-005)

**Setup**: Read `config/crd/bases/kubernaut.ai_remediationrequests.yaml`

**Assertions**:
- YAML contains `name: Source` with `jsonPath: .spec.signalSource` and `priority: 1`
- YAML contains `name: Target` with `jsonPath: .spec.targetResource.name` and `priority: 1`
- YAML contains `name: RCA Target` with `jsonPath: .status.remediationTarget.name` and `priority: 1`
- YAML contains `name: Workflow` with `jsonPath: .status.selectedWorkflowRef.workflowId` and `priority: 1`

### UT-RR-387-002: RemediationTarget on Direct Execution Path

**File**: `test/unit/remediationorchestrator/controller/reconcile_phases_test.go` (extend scenario 3.1)

**Setup**: Existing Analyzing -> Executing scenario with `newAIAnalysisCompleted` (sets `AffectedResource{Kind: "Deployment", Name: "test-deployment", Namespace: "default"}`)

**Assertions** (via `additionalAsserts`):
- `rr.Status.RemediationTarget` is non-nil
- `rr.Status.RemediationTarget.Kind` == `"Deployment"`
- `rr.Status.RemediationTarget.Name` == `"test-deployment"`
- `rr.Status.RemediationTarget.Namespace` == `"default"`

### UT-RR-387-003: RemediationTarget on Post-Approval Path

**File**: `test/unit/remediationorchestrator/controller/reconcile_phases_test.go` (extend scenario 5.1)

**Setup**: Existing AwaitingApproval -> Executing scenario with `newAIAnalysisCompleted`

**Assertions** (via `additionalAsserts`):
- `rr.Status.RemediationTarget` is non-nil
- `rr.Status.RemediationTarget.Kind` == `"Deployment"`
- `rr.Status.RemediationTarget.Name` == `"test-deployment"`
- `rr.Status.RemediationTarget.Namespace` == `"default"`
