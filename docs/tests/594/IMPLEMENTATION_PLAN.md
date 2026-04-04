# Implementation Plan: Operator Workflow/Parameter Override via RAR Approval

**Issue**: #594
**Test Plan**: [TP-594-v1.0](TEST_PLAN.md)
**Branch**: `development/v1.4`
**Created**: 2026-03-04

---

## Overview

This plan adds the ability for operators to override the AI-recommended workflow and/or parameters when approving a RAR. The override travels through the existing RAR status path (not spec — per ADR-040), is validated by the authwebhook, and is merged by the RO reconciler before creating the WorkflowExecution.

### Design Decisions (from #594)

- **Override location**: RAR **status** (not spec). Spec remains immutable.
- **Workflow reference**: By `RemediationWorkflow` CRD `.metadata.name`, not DS UUID. The RO resolves name → `workflowId`, `bundle`, `bundleDigest`, `engine`, `version` via a GET on the RW CRD.
- **Parameters**: Full replacement semantics. Present (even empty map) replaces AIA params. Nil/absent → AIA params used.
- **Validation**: Authwebhook validates RW exists and is `Ready`. No schema validation on parameters.
- **Audit**: WE gets annotation `kubernaut.ai/override-source: rar/{rar-name}`. Event emitted on RR.

### Key Files

| File | Change |
|------|--------|
| `api/remediation/v1alpha1/remediationapprovalrequest_types.go` | Add `WorkflowOverride` struct + field to RAR status |
| `pkg/authwebhook/remediationapprovalrequest_handler.go` | Validate override RW exists and is Ready |
| `internal/controller/remediationorchestrator/reconciler.go` | Merge logic in `handleAwaitingApprovalPhase` |
| `pkg/remediationorchestrator/creator/workflowexecution.go` | No change (RO pre-merges before calling `Create`) |

---

## Phase 1: TDD RED — CRD Types (Day 1)

### Phase 1.1: Type serialization tests

**File**: `test/unit/remediationorchestrator/controller/override_types_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-OV-594-001 | `WorkflowOverride` JSON round-trip preserves all fields | `WorkflowOverride` struct doesn't exist |
| UT-OV-594-002 | Nil `WorkflowOverride` is omitted from JSON | Same |

### Phase 1.2: Implement types (GREEN)

**File**: `api/remediation/v1alpha1/remediationapprovalrequest_types.go`

```go
// WorkflowOverride allows operators to redirect WorkflowExecution to a
// different workflow and/or parameters than what the AI recommended.
// All fields are optional — only specified fields override AIA defaults.
type WorkflowOverride struct {
    // WorkflowName is the .metadata.name of a RemediationWorkflow CRD.
    // When set, the RO resolves bundle/version/engine from this CRD
    // instead of using AIA's selectedWorkflow.
    // +optional
    WorkflowName string `json:"workflowName,omitempty"`

    // Parameters replaces the AI-recommended parameter map entirely.
    // An empty map means "no parameters"; nil/absent means "use AIA params".
    // +optional
    Parameters map[string]string `json:"parameters,omitempty"`

    // Rationale documents why the operator chose to override.
    // +optional
    Rationale string `json:"rationale,omitempty"`
}
```

Added to `RemediationApprovalRequestStatus`:
```go
    // WorkflowOverride allows the approving operator to redirect execution
    // to a different workflow and/or parameters than the AI recommended.
    // Only applied when Decision is Approved.
    // +optional
    WorkflowOverride *WorkflowOverride `json:"workflowOverride,omitempty"`
```

Then: `make manifests` to regenerate CRDs.

### Phase 1 Checkpoint

- [ ] UT-OV-594-001, -002 pass
- [ ] `make manifests` succeeds
- [ ] CRD YAML includes `workflowOverride` in status schema

---

## Phase 2: TDD RED + GREEN — Authwebhook Validation (Days 2-3)

### Phase 2.1: Webhook validation tests (RED)

**File**: `test/unit/authwebhook/rar_override_validation_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-OV-594-003 | Approved + override + valid Ready RW → allow | No override validation logic |
| UT-OV-594-004 | Approved + override + non-existent RW → deny | Same |
| UT-OV-594-005 | Rejected + override → deny with message | Same |
| UT-OV-594-006 | Approved + override + RW not Ready → deny | Same |
| UT-OV-594-007 | Approved + override (no workflowName, only params) → allow | Same |

### Phase 2.2: Webhook implementation (GREEN)

**File**: `pkg/authwebhook/remediationapprovalrequest_handler.go`

In the `Handle` method, after decision validation and before DecidedBy attribution:

1. Check if `rar.Status.WorkflowOverride != nil`
2. If override present and decision != Approved → deny with "override only valid with Approved decision"
3. If `override.WorkflowName != ""`:
   - GET `RemediationWorkflow` by name in the RR's namespace
   - Not found → deny with "referenced RemediationWorkflow not found"
   - Found but `status.catalogStatus != Ready` → deny with "RemediationWorkflow not in Ready status"
4. If only parameters (no workflowName) → allow (will be merged with AIA workflow)

The handler needs a K8s client reader to perform the RW GET. The existing handler struct accepts dependencies; add a `client.Reader` field.

### Phase 2 Checkpoint

- [ ] UT-OV-594-003 through -007 pass
- [ ] Invalid overrides are rejected with descriptive messages
- [ ] go build succeeds

---

## Phase 3: TDD RED + GREEN — RO Merge Logic (Days 3-5)

### Phase 3.1: Merge logic tests (RED)

**File**: `test/unit/remediationorchestrator/controller/override_merge_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-OV-594-008 | Override workflowName → WE spec has RW's bundle/version/engine | No merge logic |
| UT-OV-594-009 | Override params only → WE has AIA workflow + override params | Same |
| UT-OV-594-010 | Override params `{}` → WE params empty; params nil → AIA params | Same |
| UT-OV-594-011 | No override → WE matches AIA exactly | Existing behavior (should pass) |
| UT-OV-594-012 | Override applied → WE annotation present | No annotation logic |
| UT-OV-594-013 | Override applied → K8s event on RR | No event logic |
| UT-OV-594-014 | Override RW deleted → fail gracefully with event | No error handling |

### Phase 3.2: Merge implementation (GREEN)

**File**: `internal/controller/remediationorchestrator/reconciler.go`

In `handleAwaitingApprovalPhase`, after confirming Approved and loading AIAnalysis:

```go
// Apply operator override if present
override := rar.Status.WorkflowOverride
if override != nil {
    mergedWorkflow, err := r.applyWorkflowOverride(ctx, ai, override)
    if err != nil {
        // Emit event, fail gracefully
        return r.failWithEvent(ctx, rr, "OverrideResolutionFailed", err)
    }
    // Use mergedWorkflow for WE creation
    ai.Status.SelectedWorkflow = mergedWorkflow
    
    // Prepare override annotation for WE
    overrideAnnotation = fmt.Sprintf("rar/%s", rar.Name)
}
```

New helper method `applyWorkflowOverride(ctx, ai, override) (*SelectedWorkflow, error)`:
1. Start with a copy of `ai.Status.SelectedWorkflow`
2. If `override.WorkflowName != ""`:
   - GET `RemediationWorkflow` CRD by name
   - Replace: `WorkflowID = rw.Status.WorkflowID`, `Version = rw.Spec.Version`, `ExecutionBundle = rw.Spec.Execution.Bundle`, `ExecutionBundleDigest = rw.Spec.Execution.BundleDigest`, `EngineConfig = rw.Spec.Execution.EngineConfig`
3. If `override.Parameters != nil`:
   - Replace: `mergedWorkflow.Parameters = override.Parameters`
4. Return merged workflow

The annotation is passed to `weCreator.Create` via the existing metadata path or by setting it on the RR for the creator to read.

### Phase 3 Checkpoint

- [ ] UT-OV-594-008 through -014 pass
- [ ] Merge logic produces correct WE spec for all combinations
- [ ] go build succeeds

---

## Phase 4: TDD RED + GREEN — Integration Tests (Days 5-6)

### Phase 4.1: Integration tests

**File**: `test/integration/remediationorchestrator/override_flow_test.go`

| Test ID | What it asserts |
|---------|----------------|
| IT-OV-594-001 | Full flow: RR → AIA → RAR (Approved + workflow override) → WE with RW bundle/params |
| IT-OV-594-002 | Full flow: RR → AIA → RAR (Approved, no override) → WE matches AIA (regression) |
| IT-OV-594-003 | Full flow: RR → AIA → RAR (Approved + params-only override) → WE with AIA workflow + new params |
| IT-OV-594-004 | WE has `kubernaut.ai/override-source` annotation; RR has OperatorOverride event |

Uses envtest with:
- RR, AIAnalysis, RAR, RW CRDs registered
- Authwebhook wired (or bypass with direct status update)
- RO reconciler running
- Fake RW catalog with at least 2 workflows

### Phase 4 Checkpoint

- [ ] All 14 unit + 4 integration tests pass
- [ ] Full approve-with-override flow verified end-to-end

---

## Phase 5: TDD REFACTOR — Code Quality (Day 7)

### Phase 5.1: Extract override helper

Extract `applyWorkflowOverride` into `pkg/remediationorchestrator/override/merge.go` for reuse by:
- RO reconciler (this issue)
- Conversation override advisory (#592) — validation logic shared

### Phase 5.2: Structured logging

Add structured log fields: `override.workflowName`, `override.hasParams`, `override.rationale`, `originalWorkflow`.

### Phase 5.3: Metrics

- `kubernaut_rar_override_total` (counter, labels: type=[workflow|params|both])
- `kubernaut_rar_override_validation_failures_total` (counter, labels: reason)

### Phase 5.4: Documentation

Update ADR-040 with override extension note per #594 design.

### Phase 5 Checkpoint

- [ ] All tests pass
- [ ] Override helper reusable
- [ ] Structured logging in place
- [ ] Metrics registered

---

## Phase 6: Due Diligence & Commit (Day 8)

### Phase 6.1: Comprehensive audit

- [ ] CRD regenerated (`make manifests`) with WorkflowOverride in status
- [ ] Webhook rejects all invalid override combinations
- [ ] Merge produces correct WE for all override/no-override permutations
- [ ] Override annotation present on WE
- [ ] Existing approve-without-override behavior unchanged (regression)
- [ ] Parameters nil vs empty-map semantics correct
- [ ] No sensitive data in error responses

### Phase 6.2: Commit in logical groups

| Commit # | Scope |
|----------|-------|
| 1 | `feat(#594): add WorkflowOverride type to RAR CRD status` |
| 2 | `test(#594): TDD RED — failing tests for webhook override validation` |
| 3 | `feat(#594): authwebhook validates override RW exists and is Ready` |
| 4 | `test(#594): TDD RED — failing tests for RO merge logic` |
| 5 | `feat(#594): RO merge logic applies operator override to WE spec` |
| 6 | `feat(#594): override annotation and K8s event on RR` |
| 7 | `test(#594): integration tests for full override flow` |
| 8 | `refactor(#594): extract override helper, logging, metrics` |

---

## Estimated Effort

| Phase | Effort |
|-------|--------|
| Phase 1 (CRD types) | 1 day |
| Phase 2 (Webhook validation) | 2 days |
| Phase 3 (RO merge logic) | 2.5 days |
| Phase 4 (Integration tests) | 1.5 days |
| Phase 5 (REFACTOR) | 1 day |
| Phase 6 (Due Diligence) | 0.5 day |
| **Total** | **8.5 days** |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial implementation plan |
