# Implementation Plan: Operator Workflow/Parameter Override via RAR Approval

**Issue**: #594
**Test Plan**: [TP-594-v1.0](TEST_PLAN.md)
**Branch**: `development/v1.4`
**Created**: 2026-03-04

---

## Overview

This plan adds the ability for operators to override the AI-recommended workflow and/or parameters when approving a RAR. The RO is the single decision point: it resolves the final workflow spec from either the RAR override (if present) or the AIA (default), then passes the result to the WE creator (unchanged). The WE has no awareness of overrides.

### Due Diligence Findings (incorporated)

| ID | Finding | Resolution |
|----|---------|-----------|
| F1 | Issue says "Ready" but CatalogStatus enum has no `Ready` | Use `sharedtypes.CatalogStatusActive` |
| F2 | Authwebhook handler needs `client.Reader` for RW GET | Add to constructor; update all call sites |
| F3 | WE creator should be agnostic to override source | RO resolves final spec; WE creator unchanged |
| F5 | `rr.Status.SelectedWorkflowRef` should reflect actual workflow used | RO sets from resolved spec (override or AIA) |
| F6 | No existing `EventReasonOperatorOverride` | Add to `pkg/shared/events/reasons.go` |
| F7 | Parameters types align (`map[string]string`) | No conversion needed |

### Key Design Decisions

- **Override on RAR status** (not spec) — per ADR-040
- **RO is the single decision point** — resolves final spec from RAR override or AIA
- **WE creator unchanged** — receives resolved data, no override awareness
- **No annotation on WE** — traceability via K8s event + RAR status
- **Validate against `CatalogStatusActive`** — not "Ready" (F1)

---

## Phase 1: TDD RED — Failing Tests (Days 1-2)

All tests written to define the business contract. Every test MUST fail at this point.

### Phase 1.1: Override type serialization tests

**File**: `test/unit/remediationorchestrator/controller/override_types_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-OV-594-001 | `WorkflowOverride` JSON round-trip preserves all fields | `WorkflowOverride` struct doesn't exist |
| UT-OV-594-002 | Nil `WorkflowOverride` is omitted from JSON | Same |

### Phase 1.2: Authwebhook override validation tests

**File**: `test/unit/authwebhook/rar_override_validation_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-AW-594-003 | Approved + override + valid Active RW → allow | No override validation logic; no `client.Reader` on handler |
| UT-AW-594-004 | Approved + override + non-existent RW → deny | Same |
| UT-AW-594-005 | Rejected + override → deny | Same |
| UT-AW-594-006 | Approved + override + RW in Pending status → deny | Same |
| UT-AW-594-007 | Approved + override (no workflowName, only params) → allow | Same |

### Phase 1.3: RO merge logic tests

**File**: `test/unit/remediationorchestrator/controller/override_merge_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-RO-594-001 | Override workflowName → resolved spec uses RW's bundle/version/engine | `ResolveWorkflow` function doesn't exist |
| UT-RO-594-002 | Override params only → AIA workflow + override params | Same |
| UT-RO-594-003 | No override → AIA SelectedWorkflow unmodified | Same |
| UT-RO-594-004 | Override both workflowName + params → both overridden | Same |
| UT-RO-594-005 | Override rationale preserved | Same |
| UT-RO-594-006 | Params `{}` → empty; params nil → AIA params | Same |
| UT-RO-594-007 | Override present → OperatorOverride event emitted | `EventReasonOperatorOverride` constant doesn't exist |
| UT-RO-594-008 | No override → no OperatorOverride event | Same (constant missing) |
| UT-RO-594-009 | Override applied → `SelectedWorkflowRef` reflects override | `ResolveWorkflow` doesn't exist |
| UT-RO-594-010 | Override RW deleted → graceful failure | Same |

### Phase 1 Checkpoint

- [ ] All 17 unit test files compile
- [ ] All 17 unit tests FAIL (RED)
- [ ] Zero new lint errors introduced
- [ ] Existing tests still compile (no import breakage)

---

## Phase 2: TDD GREEN — Minimal Implementation (Days 2-4)

Minimal code to make all 17 unit tests pass. No optimization, no refactoring.

### Phase 2.1: CRD types

**File**: `api/remediation/v1alpha1/remediationapprovalrequest_types.go`

Add `WorkflowOverride` struct:

```go
type WorkflowOverride struct {
    // +optional
    WorkflowName string `json:"workflowName,omitempty"`
    // +optional
    Parameters map[string]string `json:"parameters,omitempty"`
    // +optional
    Rationale string `json:"rationale,omitempty"`
}
```

Add field to `RemediationApprovalRequestStatus`:

```go
    // +optional
    WorkflowOverride *WorkflowOverride `json:"workflowOverride,omitempty"`
```

Then: `make manifests` to regenerate CRDs.

**Tests passing after this**: UT-OV-594-001, UT-OV-594-002

### Phase 2.2: Event constant

**File**: `pkg/shared/events/reasons.go`

Add to RemediationOrchestrator section:

```go
    EventReasonOperatorOverride = "OperatorOverride"
```

**Tests unblocked**: UT-RO-594-007, UT-RO-594-008

### Phase 2.3: Authwebhook handler — add `client.Reader`

**File**: `pkg/authwebhook/remediationapprovalrequest_handler.go`

1. Add `reader client.Reader` field to `RemediationApprovalRequestAuthHandler`
2. Update `NewRemediationApprovalRequestAuthHandler(auditStore, reader)` constructor
3. In `Handle`, after decision validation and before identity extraction:
   - If `rar.Status.WorkflowOverride != nil`:
     - If `decision != Approved` → deny
     - If `override.WorkflowName != ""`:
       - GET `RemediationWorkflow` by name in `rar.Namespace`
       - Not found → deny
       - `status.catalogStatus != Active` → deny
     - Else (only params) → allow

4. Update all call sites:
   - `cmd/` authwebhook wiring
   - Existing test constructors

**Tests passing after this**: UT-AW-594-003 through UT-AW-594-007

### Phase 2.4: Merge logic

**File**: `pkg/remediationorchestrator/override/merge.go` (new package)

```go
func ResolveWorkflow(
    ctx context.Context,
    reader client.Reader,
    override *remediationv1.WorkflowOverride,
    aiWorkflow *aianalysisv1.SelectedWorkflow,
    namespace string,
) (*aianalysisv1.SelectedWorkflow, error)
```

Logic:
1. If `override == nil` → return `aiWorkflow` unmodified
2. Start with a deep copy of `aiWorkflow`
3. If `override.WorkflowName != ""`:
   - GET `RemediationWorkflow` by name in namespace
   - Not found → return error
   - Map: `WorkflowID = rw.Status.WorkflowID`, `Version = rw.Spec.Version`, `ExecutionBundle = rw.Spec.Execution.Bundle`, `ExecutionBundleDigest = rw.Spec.Execution.BundleDigest`, `EngineConfig = rw.Spec.Execution.EngineConfig`, `ServiceAccountName = rw.Spec.Execution.ServiceAccountName`, `ActionType = rw.Spec.ActionType`
4. If `override.Parameters != nil` → replace params (even if empty map)
5. Return resolved workflow

**Tests passing after this**: UT-RO-594-001 through UT-RO-594-010

### Phase 2 Checkpoint

- [ ] All 17 unit tests PASS (GREEN)
- [ ] `go build ./...` succeeds
- [ ] `make manifests` succeeds — CRD YAML includes `workflowOverride`
- [ ] Existing authwebhook tests compile and pass (constructor updated)
- [ ] Existing RO tests compile and pass (no regressions)

---

## Checkpoint 1: Unit-Level Due Diligence

**Scope**: Verify all findings addressed before touching reconciler (integration-level code).

- [ ] **F1**: Webhook validates against `sharedtypes.CatalogStatusActive` — confirmed in test UT-AW-594-003
- [ ] **F2**: Handler constructor accepts `client.Reader` — all call sites updated and compile
- [ ] **F5**: `ResolveWorkflow` returns full `SelectedWorkflow` that can feed both WE creator and `SelectedWorkflowRef`
- [ ] **F6**: `EventReasonOperatorOverride` exists in events registry
- [ ] **F7**: Parameters `map[string]string` matches on both sides
- [ ] **Regression**: Run `ginkgo ./test/unit/authwebhook/...` — all pre-existing tests pass
- [ ] **Regression**: Run `ginkgo ./test/unit/remediationorchestrator/controller/...` — all pre-existing tests pass
- [ ] **Build**: `go build ./...` — zero errors
- [ ] **Lint**: `golangci-lint run --timeout=5m` — no new warnings

---

## Phase 3: TDD REFACTOR — Unit Code Quality (Day 4)

Tests are GREEN. Now improve code quality without changing behavior.

### Phase 3.1: Extract validation helper

Extract webhook override validation into a testable helper:

```go
func ValidateWorkflowOverride(
    ctx context.Context,
    reader client.Reader,
    override *WorkflowOverride,
    decision ApprovalDecision,
    namespace string,
) error
```

Called from `Handle`. Allows direct unit testing without admission request machinery.

### Phase 3.2: Structured logging

Add structured log fields to both webhook and merge logic:
- `override.workflowName`
- `override.hasParams` (bool)
- `override.rationale`
- `originalWorkflowID` (from AIA)
- `resolvedWorkflowID` (after merge)

### Phase 3.3: Error message quality

Ensure all webhook denial messages are operator-friendly:
- `"override rejected: RemediationWorkflow 'drain-restart' not found in namespace 'prod'"`
- `"override rejected: RemediationWorkflow 'drain-restart' has catalogStatus 'Pending' (must be Active)"`
- `"override rejected: workflowOverride is only valid when decision is Approved"`

### Phase 3 Checkpoint

- [ ] All 17 unit tests still PASS (behavior unchanged)
- [ ] Code is cleaner, helpers extracted, logging improved
- [ ] No new lint warnings

---

## Phase 4: TDD RED — Failing Integration Tests (Day 5)

### Phase 4.1: Integration test setup

**File**: `test/integration/remediationorchestrator/override_flow_test.go`

Requires envtest with:
- RR, AIA, RAR, WE, RW CRDs registered
- RO reconciler running
- RW catalog seeded with at least 2 workflows (one Active, one Pending)
- No mocks (per No-Mocks Policy)

### Phase 4.2: Integration tests (RED)

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| IT-RO-594-001 | Approve + workflow override → WE created with RW spec | Override branch not wired in reconciler |
| IT-RO-594-002 | Approve without override → WE matches AIA (regression) | Should already pass (existing behavior) |
| IT-RO-594-003 | Approve + params-only override → WE with AIA workflow + new params | Override branch not wired |
| IT-RO-594-004 | Override applied → OperatorOverride event on RR | Not wired |
| IT-RO-594-005 | Override referencing non-existent RW → webhook denies | Webhook override validation not wired with envtest |

### Phase 4 Checkpoint

- [ ] All 5 integration test files compile
- [ ] IT-RO-594-002 passes (existing behavior — regression guard)
- [ ] IT-RO-594-001, -003, -004, -005 FAIL (RED)
- [ ] All unit tests still pass

---

## Phase 5: TDD GREEN — Wire Override into Reconciler (Days 5-6)

### Phase 5.1: Reconciler changes

**File**: `internal/controller/remediationorchestrator/reconciler.go` — `handleAwaitingApprovalPhase`

In the `ApprovalDecisionApproved` case, after fetching AIAnalysis and before calling `r.weCreator.Create`:

```go
// Resolve final workflow: RAR override (if present) takes precedence over AIA
resolvedWorkflow, overrideApplied, err := override.ResolveWorkflow(
    ctx, r.apiReader, rar.Status.WorkflowOverride, ai.Status.SelectedWorkflow, rr.Namespace,
)
if err != nil {
    r.Recorder.Eventf(rr, corev1.EventTypeWarning, events.EventReasonRemediationFailed,
        "Failed to resolve operator workflow override: %v", err)
    return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil
}
if overrideApplied {
    r.Recorder.Eventf(rr, corev1.EventTypeNormal, events.EventReasonOperatorOverride,
        "Operator override applied: workflow=%s rationale=%q",
        rar.Status.WorkflowOverride.WorkflowName,
        rar.Status.WorkflowOverride.Rationale)
}
// Use resolved workflow for WE creation
ai.Status.SelectedWorkflow = resolvedWorkflow
```

The rest of the approval flow is unchanged — `r.weCreator.Create(ctx, rr, ai)` receives the already-resolved data, and `rr.Status.SelectedWorkflowRef` is set from `ai.Status.SelectedWorkflow` (now resolved).

### Phase 5.2: Webhook wiring with envtest

If the integration test suite uses envtest with admission webhooks, ensure the updated webhook (with `client.Reader`) is wired correctly.

**Tests passing after this**: IT-RO-594-001 through IT-RO-594-005

### Phase 5 Checkpoint

- [ ] All 5 integration tests PASS (GREEN)
- [ ] All 17 unit tests still PASS
- [ ] `go build ./...` succeeds
- [ ] WE created by override flow has correct spec (verified by integration test assertions)
- [ ] `rr.Status.SelectedWorkflowRef` reflects override workflow in IT-RO-594-001

---

## Checkpoint 2: Full Due Diligence

**Scope**: Comprehensive verification before REFACTOR.

### Correctness Verification

- [ ] **Override + Approved**: WE gets overridden workflow spec (IT-RO-594-001)
- [ ] **Override + Rejected**: Webhook denies (UT-AW-594-005)
- [ ] **No override**: Existing behavior unchanged (IT-RO-594-002, UT-RO-594-003)
- [ ] **Params nil vs empty**: Nil → AIA params; empty → no params (UT-RO-594-006)
- [ ] **CatalogStatus**: Validates against `Active` (UT-AW-594-003, UT-AW-594-006)
- [ ] **SelectedWorkflowRef**: Reflects what was actually used (UT-RO-594-009)
- [ ] **K8s event**: OperatorOverride on RR when override applied (UT-RO-594-007, IT-RO-594-004)
- [ ] **Race condition**: RW deleted after webhook → graceful failure (UT-RO-594-010)

### Regression Verification

- [ ] `ginkgo ./test/unit/authwebhook/...` — all pre-existing tests pass
- [ ] `ginkgo ./test/unit/remediationorchestrator/controller/...` — all pre-existing tests pass
- [ ] `ginkgo ./test/integration/remediationorchestrator/...` — all pre-existing tests pass

### Build & Lint

- [ ] `go build ./...` — zero errors
- [ ] `golangci-lint run --timeout=5m` — no new warnings
- [ ] `make manifests` — CRD includes `workflowOverride` in status schema

### Coverage

- [ ] Unit coverage >=80% for override types, webhook validation, merge logic
- [ ] Integration coverage >=80% for reconciler override branch

---

## Phase 6: TDD REFACTOR — Integration Code Quality (Day 7)

### Phase 6.1: Reconciler code clarity

- Ensure the override resolution block is clearly delimited with comments
- Extract any complex logic into the `override` package (keep reconciler lean)

### Phase 6.2: Metrics

Add to RO metrics registry:
- `kubernaut_rar_override_applied_total` (counter, labels: `type=[workflow|params|both]`)
- `kubernaut_rar_override_validation_rejected_total` (counter, labels: `reason=[not_found|not_active|wrong_decision]`)

### Phase 6.3: ADR-040 documentation update

Update ADR-040 with override extension note per #594 design.

### Phase 6 Checkpoint

- [ ] All 22 tests (17 unit + 5 integration) still PASS
- [ ] Metrics registered and incremented in tests
- [ ] Documentation updated
- [ ] No new lint warnings

---

## Phase 7: Due Diligence & Commit (Day 8)

### Phase 7.1: Final comprehensive audit

- [ ] CRD regenerated with `WorkflowOverride` in RAR status schema
- [ ] Webhook rejects all invalid override combinations
- [ ] RO resolves correct workflow for all override/no-override permutations
- [ ] WE creator unchanged — receives resolved data, no override awareness
- [ ] Existing approve-without-override behavior 100% unchanged
- [ ] Parameters nil vs empty-map semantics correct
- [ ] No sensitive data in error responses
- [ ] All constructor call sites updated for `client.Reader`
- [ ] `go vet ./...` clean
- [ ] `golangci-lint run` clean

### Phase 7.2: Commit in logical groups

| Commit # | Scope |
|----------|-------|
| 1 | `test(#594): TDD RED — failing tests for WorkflowOverride types, webhook validation, and merge logic` |
| 2 | `feat(#594): add WorkflowOverride type to RAR CRD status + EventReasonOperatorOverride` |
| 3 | `feat(#594): authwebhook validates override RW exists and is Active` |
| 4 | `feat(#594): ResolveWorkflow merge logic — RAR override takes precedence over AIA` |
| 5 | `refactor(#594): extract validation helper, structured logging, error messages` |
| 6 | `test(#594): TDD RED — failing integration tests for full override flow` |
| 7 | `feat(#594): wire override resolution into RO handleAwaitingApprovalPhase` |
| 8 | `refactor(#594): metrics, documentation, code quality` |

---

## Estimated Effort

| Phase | Effort |
|-------|--------|
| Phase 1 (TDD RED — unit tests) | 1.5 days |
| Phase 2 (TDD GREEN — unit implementation) | 2 days |
| Checkpoint 1 | 0.5 day |
| Phase 3 (TDD REFACTOR — unit code quality) | 0.5 day |
| Phase 4 (TDD RED — integration tests) | 0.5 day |
| Phase 5 (TDD GREEN — reconciler wiring) | 1 day |
| Checkpoint 2 | 0.5 day |
| Phase 6 (TDD REFACTOR — integration code quality) | 0.5 day |
| Phase 7 (Due Diligence & Commit) | 0.5 day |
| **Total** | **7.5 days** |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial implementation plan. Due diligence F1-F7 incorporated. TDD RED/GREEN/REFACTOR phases with checkpoints. |
