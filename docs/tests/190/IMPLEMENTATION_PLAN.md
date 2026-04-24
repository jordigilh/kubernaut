# Implementation Plan: Issue #190 — WE/RO Deduplicated Phase with Result Inheritance

**Issue**: #190
**Test Plan**: [TP-190-v3](TEST_PLAN.md)
**Branch**: `development/v1.4`
**Created**: 2026-03-04
**Methodology**: TDD RED → GREEN → REFACTOR per phase

---

## Overview

This plan decomposes the test plan into 7 implementation phases. Each phase follows strict TDD: RED (write failing tests), GREEN (minimal implementation to pass), REFACTOR (clean up). Phases are ordered by dependency — later phases depend on types/functions introduced in earlier phases.

### Phase Dependency Graph

```
Phase 1 (CRD types)
  ├─► Phase 2 (WE collision classification)
  │     └─► Phase 2-IT (WE integration tests)
  └─► Phase 3 (RO handler branching)
        └─► Phase 4 (RO cross-WE watch + result propagation)
              ├─► Phase 4-IT (RO integration tests)
              └─► Phase 5 (Consecutive failure exclusion)
                    └─► Phase 5-IT (RO blocking integration)
Phase 6 (Notification provenance) — depends on Phase 4
Phase 7 (Documentation) — depends on all prior phases
```

---

## Phase 1: CRD Type Extensions

**Goal**: Add `FailureReasonDeduplicated`, `DeduplicatedBy` to WE CRD. Add `FailurePhaseDeduplicated`, `DeduplicatedByWE` to RR CRD. Regenerate manifests.

### Phase 1 — RED

**Tests**: UT-WE-190-004

**File**: `test/unit/workflowexecution/controller_test.go`

Write a test that asserts:
- `workflowexecutionv1.FailureReasonDeduplicated` constant equals `"Deduplicated"`
- A WFE status struct has a `DeduplicatedBy` string field (compile-time check via assignment)

**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go` (new file)

Write a test that asserts:
- `remediationv1.FailurePhaseDeduplicated` constant equals `FailurePhase("Deduplicated")`
- An RR status struct has a `DeduplicatedByWE` string field (compile-time check via assignment)

**Expected**: Tests fail to compile — constants and fields do not exist yet.

**Validation**: `go build ./test/unit/workflowexecution/... ./test/unit/remediationorchestrator/...` fails.

### Phase 1 — GREEN

**Files modified**:
- `api/workflowexecution/v1alpha1/workflowexecution_types.go`:
  - Add constant: `FailureReasonDeduplicated = "Deduplicated"`
  - Add kubebuilder enum value: update `+kubebuilder:validation:Enum=OOMKilled;DeadlineExceeded;Forbidden;ResourceExhausted;ConfigurationError;ImagePullBackOff;TaskFailed;UnsupportedEngine;Unknown;Deduplicated` on `FailureDetails.Reason`
  - Add status field: `DeduplicatedBy string \`json:"deduplicatedBy,omitempty"\`` to `WorkflowExecutionStatus`
- `api/remediation/v1alpha1/remediationrequest_types.go`:
  - Add constant: `FailurePhaseDeduplicated FailurePhase = "Deduplicated"`
  - Add kubebuilder enum value: update `+kubebuilder:validation:Enum=Configuration;SignalProcessing;AIAnalysis;Approval;WorkflowExecution;Blocked;Deduplicated` on `FailurePhase`
  - Add status field: `DeduplicatedByWE string \`json:"deduplicatedByWE,omitempty"\`` to `RemediationRequestStatus`
- Run `make generate` to regenerate CRD manifests

**Expected**: Tests compile and pass. `go build ./...` succeeds.

**Validation**: `go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-190-004" -count=1` passes. `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="FailurePhaseDeduplicated" -count=1` passes.

### Phase 1 — REFACTOR

- Verify CRD field documentation comments are consistent with existing patterns
- Verify generated CRD YAML in `config/crd/bases/`, `charts/kubernaut/crds/`, `pkg/shared/assets/crds/` includes new enum values and fields
- No code duplication to clean up (additive constants only)

---

## Phase 2: WE Collision Classification

**Goal**: When a Job/PipelineRun collision is detected from another WFE, mark the WFE as `Failed/Deduplicated` with `DeduplicatedBy` set atomically.

### Phase 2 — RED

**Tests**: UT-WE-190-001, UT-WE-190-002, UT-WE-190-003, UT-WE-190-005, UT-WE-190-006, UT-WE-190-007, UT-WE-190-008

**File**: `test/unit/workflowexecution/controller_test.go`

Write tests:
- **UT-WE-190-001**: Set up WFE + running Job with label `kubernaut.ai/workflow-execution: original-wfe`. Trigger `reconcilePending`. Assert `FailureDetails.Reason == "Deduplicated"` and `DeduplicatedBy == "original-wfe"`.
- **UT-WE-190-002**: Set up WFE + completed/stale Job. Trigger `reconcilePending`. Assert cleanup+retry behavior unchanged (regression guard). `DeduplicatedBy` empty.
- **UT-WE-190-003**: Set up WFE + running Job WITHOUT label. Trigger `reconcilePending`. Assert `FailureDetails.Reason == "Unknown"` and `DeduplicatedBy` empty.
- **UT-WE-190-005**: Set up WFE + PipelineRun with label `kubernaut.ai/workflow-execution: other-wfe` (different from this WFE). Trigger `HandleAlreadyExists`. Assert `FailureDetails.Reason == "Deduplicated"` and `DeduplicatedBy == "other-wfe"`.
- **UT-WE-190-006**: Set up WFE + PipelineRun with matching labels (our own). Trigger `HandleAlreadyExists`. Assert WFE transitions to `Running` (no dedup). Regression guard.
- **UT-WE-190-007**: Set up WFE + PipelineRun without label (or nil labels). Trigger `HandleAlreadyExists`. Assert `FailureDetails.Reason == "Unknown"` fallback.
- **UT-WE-190-008**: Both Job and Tekton paths: assert `DeduplicatedBy` field matches the exact WFE name from the execution resource label.

**Expected**: UT-WE-190-001, 003, 005, 007, 008 fail (still produce `"Unknown"`, `DeduplicatedBy` empty). UT-WE-190-002, 006 pass (existing behavior).

**Validation**: `go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-190" -count=1` — 5 failures, 2 passes.

### Phase 2 — GREEN

**Files modified**:

1. **`internal/controller/workflowexecution/workflowexecution_controller.go`**:

   a. **New method `MarkFailedAsDeduplicated`** (~40 lines):
   - Signature: `func (r *WorkflowExecutionReconciler) MarkFailedAsDeduplicated(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, originalWFE string) error`
   - Internally mirrors `MarkFailedWithReason` logic but inside the `AtomicStatusUpdate` closure sets:
     - `wfe.Status.FailureDetails.Reason = "Deduplicated"`
     - `wfe.Status.DeduplicatedBy = originalWFE` (M5 constraint — inside closure)
   - `WasExecutionFailure = false` (pre-execution failure)

   b. **Extend `handleJobAlreadyExists`** return type:
   - Change from `(*weexecutor.CreateResult, bool, bool)` to `(*weexecutor.CreateResult, bool, bool, string)`
   - In the `!completed` branch (currently line 1093-1094): fetch the existing Job by deterministic name, read label `kubernaut.ai/workflow-execution`, return as 4th value. Return `""` if label missing or fetch error.
   - All other return paths: append `""` as 4th value (no dedup detected)

   c. **Update `reconcilePending` call site** (line 514):
   - Receive 4th return value: `retryResult, handled, requeueForGC, originalWFE := r.handleJobAlreadyExists(...)`
   - After existing `handled`/`requeueForGC` checks, at line 525 (where `createErr != nil`):
     ```
     if originalWFE != "" {
         markErr := r.MarkFailedAsDeduplicated(ctx, wfe, originalWFE)
         return ctrl.Result{}, markErr
     }
     // existing fallback: MarkFailedWithReason("Unknown", ...)
     ```

   d. **Update `HandleAlreadyExists`** (Tekton path, line 1197):
   - When another WFE owns the PipelineRun and label is present: call `r.MarkFailedAsDeduplicated(ctx, wfe, existingPR.Labels["kubernaut.ai/workflow-execution"])` instead of `MarkFailedWithReason("Unknown", ...)`
   - When label is missing/nil: keep existing `MarkFailedWithReason("Unknown", ...)` fallback

**Expected**: All 8 UT-WE-190-* tests pass.

**Validation**: `go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-190" -count=1` — 8 passes. `go build ./...` succeeds.

### Phase 2 — REFACTOR

- Extract shared logic between `MarkFailedWithReason` and `MarkFailedAsDeduplicated` into a private helper (e.g., `markFailedInternal`) to eliminate code duplication (~50 lines shared)
- Verify all existing WE tests still pass: `go test ./test/unit/workflowexecution/... -count=1`
- Verify existing WE integration tests still pass: `go test ./test/integration/workflowexecution/... -count=1`
- Clean up any logging inconsistencies

### Phase 2-IT: WE Integration Tests

**Tests**: IT-WE-190-001, IT-WE-190-002

**File**: `test/integration/workflowexecution/conflict_test.go` (extend existing)

**RED**: Write integration tests that create real WFEs in envtest and trigger Job/Tekton collisions. Assert `Deduplicated` reason and `DeduplicatedBy` on the WFE status.

**GREEN**: Should pass immediately (implementation already done in Phase 2 GREEN).

**REFACTOR**: Verify existing conflict tests updated where they previously asserted `"Unknown"` for cross-WFE collisions.

**Validation**: `go test ./test/integration/workflowexecution/... -ginkgo.focus="IT-WE-190" -count=1`

---

## Phase 3: RO Handler Branching

**Goal**: `HandleStatus` detects `FailureReasonDeduplicated` on the WFE, sets `DeduplicatedByWE` on the RR, keeps the RR in `PhaseExecuting`, and requeues.

### Phase 3 — RED

**Tests**: UT-RO-190-001, UT-RO-190-002, UT-RO-190-003, UT-RO-190-004

**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

Write tests:
- **UT-RO-190-001**: RR in `Executing`, WFE in `Failed` with `FailureDetails.Reason = "Deduplicated"` and `DeduplicatedBy = "original-wfe"`. Assert `rr.Status.OverallPhase` remains `Executing`. Assert `transitionToFailed` was NOT called (RR is not terminal).
- **UT-RO-190-002**: Same setup. Assert `rr.Status.DeduplicatedByWE == "original-wfe"` after reconcile.
- **UT-RO-190-003**: Same setup. Assert `result.RequeueAfter > 0`.
- **UT-RO-190-004**: RR in `Executing`, WFE in `Failed` with `FailureDetails.Reason = "TaskFailed"` (non-dedup). Assert RR transitions to `Failed` with `FailurePhaseWorkflowExecution` (regression guard).

**Expected**: UT-RO-190-001/002/003 fail (current code calls `transitionToFailed` for all `PhaseFailed` WFEs). UT-RO-190-004 passes.

**Validation**: `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-190-00[1-4]" -count=1` — 3 failures, 1 pass.

### Phase 3 — GREEN

**Files modified**:

1. **`pkg/remediationorchestrator/handler/workflowexecution.go`** — `HandleStatus`:
   - Add new case BEFORE `case workflowexecutionv1.PhaseFailed:`:
     ```
     case workflowexecutionv1.PhaseFailed:
         if we.Status.FailureDetails != nil &&
            we.Status.FailureDetails.Reason == workflowexecutionv1.FailureReasonDeduplicated {
             // Dedup path: set DeduplicatedByWE, keep Executing, requeue
             logger.Info("WorkflowExecution deduplicated, waiting for original WFE",
                 "originalWFE", we.Status.DeduplicatedBy)
             rr.Status.DeduplicatedByWE = we.Status.DeduplicatedBy
             // Status update via AtomicStatusUpdate (set DeduplicatedByWE)
             return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
         }
         // Existing: transitionToFailed
     ```
   - The status update for `DeduplicatedByWE` needs to go through `AtomicStatusUpdate` to persist. Use existing `h.reconciler.StatusManager.AtomicStatusUpdate(ctx, rr, func() error { rr.Status.DeduplicatedByWE = ... })` — verify handler has access to status manager. If not, pass through reconciler method.

**Expected**: All 4 tests pass.

**Validation**: `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-190-00[1-4]" -count=1` — 4 passes.

### Phase 3 — REFACTOR

- Verify `HandleStatus` remains clean — dedup branch is clearly separated from the existing `PhaseFailed` path
- Verify handler has proper access to status manager for the `AtomicStatusUpdate` call (may need to add a field or delegate to reconciler)
- Run all existing RO unit tests: `go test ./test/unit/remediationorchestrator/... -count=1`

---

## Phase 4: RO Cross-WE Watch + Result Propagation

**Goal**: When `DeduplicatedByWE` is set on an RR, `handleExecutingPhase` short-circuits to fetch the original WFE and propagate its outcome. Add cross-WE watch, field index, `transitionToInheritedCompleted`/`transitionToInheritedFailed`, and dangling reference handling.

### Phase 4 — RED

**Tests**: UT-RO-190-005, UT-RO-190-006, UT-RO-190-011, UT-RO-190-012, UT-RO-190-016

**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

Write tests:
- **UT-RO-190-005**: RR with `DeduplicatedByWE = "original-wfe"`, original WFE in `Completed`. Assert RR transitions toward Completed (via `transitionToInheritedCompleted`). Assert `CompletedAt` set.
- **UT-RO-190-006**: RR with `DeduplicatedByWE = "original-wfe"`, original WFE in `Failed`. Assert RR transitions to `Failed` with `FailurePhaseDeduplicated`. Assert `CompletedAt` set.
- **UT-RO-190-011**: RR with `DeduplicatedByWE = "original-wfe"`, original WFE does NOT exist. Assert RR transitions to `Failed` with `FailurePhaseDeduplicated` and message mentioning "original-wfe".
- **UT-RO-190-012**: RR with `DeduplicatedByWE = "original-wfe"`, original WFE in `Running` (non-terminal). Assert RR stays in `Executing`, result has `RequeueAfter > 0`.
- **UT-RO-190-016**: RR with `DeduplicatedByWE` already set, same setup as 012 but on 2nd reconcile. Assert no duplicate status writes, no events emitted, `HandleStatus` NOT called.

**Expected**: All 5 fail (C3 short-circuit doesn't exist yet; `transitionToInheritedCompleted`/`Failed` don't exist).

**Validation**: `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-190-0(05|06|11|12|16)" -count=1` — 5 failures.

### Phase 4 — GREEN

**Files modified**:

1. **`internal/controller/remediationorchestrator/reconciler.go`** — `handleExecutingPhase`:
   - Add C3 short-circuit at the TOP of the function (before existing WE fetch + HandleStatus delegation):
     ```
     if rr.Status.DeduplicatedByWE != "" {
         return r.handleDedupResultPropagation(ctx, rr)
     }
     // ... existing handleExecutingPhase logic ...
     ```

2. **`internal/controller/remediationorchestrator/reconciler.go`** — new `handleDedupResultPropagation` (~50 lines):
   - Fetch original WFE by name: `r.client.Get(ctx, client.ObjectKey{Name: rr.Status.DeduplicatedByWE, Namespace: rr.Namespace}, originalWFE)`
   - If `NotFound`: call `r.transitionToInheritedFailed(ctx, rr, rr.Status.DeduplicatedByWE, "original WorkflowExecution not found (deleted)")`
   - If fetch error: return error (retry)
   - Switch on `originalWFE.Status.Phase`:
     - `Completed`: call `r.transitionToInheritedCompleted(ctx, rr, originalWFE)`
     - `Failed`: call `r.transitionToInheritedFailed(ctx, rr, rr.Status.DeduplicatedByWE, "inherited failure from original WorkflowExecution")`
     - Other (Pending, Running, ""): requeue `ctrl.Result{RequeueAfter: 10 * time.Second}`

3. **`internal/controller/remediationorchestrator/reconciler.go`** — new `transitionToInheritedCompleted` (~30 lines):
   - Via `AtomicStatusUpdate`: transition RR toward Verifying/Completed (reuse existing `transitionToVerifying` pattern with provenance metadata)
   - Set `CompletedAt`
   - Provenance message: "Remediation resolved via [originalWFE name]"

4. **`internal/controller/remediationorchestrator/reconciler.go`** — new `transitionToInheritedFailed` (~30 lines):
   - Via `AtomicStatusUpdate`: transition RR to `Failed` with `FailurePhaseDeduplicated`
   - Set `CompletedAt`
   - Provenance message in error: "Inherited failure from [originalWFE name]: [reason]"

5. **`internal/controller/remediationorchestrator/reconciler.go`** — `SetupWithManager`:
   - Add field index on `status.deduplicatedByWE` (M3 pattern)
   - Add `Watches` for non-owned WFEs with terminal-phase predicate + map func (M2/M4 patterns):
     ```
     Watches(
         &workflowexecutionv1.WorkflowExecution{},
         handler.EnqueueRequestsFromMapFunc(r.findRRsForDedupWFE),
         builder.WithPredicates(r.wfeTerminalPhasePredicate()),
     )
     ```

6. **`internal/controller/remediationorchestrator/reconciler.go`** — new `findRRsForDedupWFE` map func (~15 lines):
   - List RRs with `client.MatchingFields{"status.deduplicatedByWE": wfe.Name}`
   - Return `[]reconcile.Request` for each matching RR

7. **`internal/controller/remediationorchestrator/reconciler.go`** — new `wfeTerminalPhasePredicate` (~15 lines):
   - M4 pattern: `UpdateFunc` checks `Completed`/`Failed`, `DeleteFunc` returns `true`, others return `false`

**Expected**: All 5 tests pass.

**Validation**: `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-190-0(05|06|11|12|16)" -count=1` — 5 passes. `go build ./...` succeeds.

### Phase 4 — REFACTOR

- Extract `handleDedupResultPropagation` to a separate file if `reconciler.go` grows beyond readability threshold
- Ensure `transitionToInheritedCompleted`/`transitionToInheritedFailed` share common terminal transition logic (DRY with existing `transitionToFailed`/`transitionToVerifying`)
- Verify all existing RO tests still pass: `go test ./test/unit/remediationorchestrator/... -count=1`
- Verify no lint errors: `golangci-lint run ./internal/controller/remediationorchestrator/...`

### Phase 4-IT: RO Integration Tests

**Tests**: IT-RO-190-001, IT-RO-190-002, IT-RO-190-003, IT-RO-190-004, IT-RO-190-006

**File**: `test/integration/remediationorchestrator/dedup_inheritance_integration_test.go` (new file)

**RED**: Write 5 integration tests using envtest:
- **IT-RO-190-001**: Create original WFE (Running) + RR (Executing, DeduplicatedByWE set). Update WFE to Completed. Assert RR inherits Completed.
- **IT-RO-190-002**: Verify cross-WE watch triggers reconcile when non-owned WFE reaches terminal.
- **IT-RO-190-003**: Verify field index: list RRs by `status.deduplicatedByWE` returns only matching RRs.
- **IT-RO-190-004**: Create RR with DeduplicatedByWE pointing to nonexistent WFE. Assert RR transitions to Failed/Deduplicated.
- **IT-RO-190-006**: Update non-terminal WFE. Assert unrelated RRs are NOT reconciled (predicate filter).

**GREEN**: Should pass with Phase 4 implementation.

**REFACTOR**: Consolidate envtest setup with existing integration test suite. Verify no flaky timing issues (use `Eventually`).

**Validation**: `go test ./test/integration/remediationorchestrator/... -ginkgo.focus="IT-RO-190" -count=1`

---

## Phase 5: Consecutive Failure Exclusion

**Goal**: `countConsecutiveFailures` skips RRs with `FailurePhaseDeduplicated` inside the `case phase.Failed` branch.

### Phase 5 — RED

**Tests**: UT-RO-190-013, UT-RO-190-014

**File**: `test/unit/remediationorchestrator/blocking_test.go` (extend existing)

Write tests:
- **UT-RO-190-013**: 3 Failed RRs with same fingerprint: 1 `FailurePhaseWorkflowExecution` + 2 `FailurePhaseDeduplicated`. Assert `countConsecutiveFailures` returns `1`.
- **UT-RO-190-014**: 3 Failed RRs with same fingerprint: all `FailurePhaseWorkflowExecution`. Assert `countConsecutiveFailures` returns `3` (regression guard).

**Expected**: UT-RO-190-013 fails (returns 3). UT-RO-190-014 passes.

**Validation**: `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-190-01[34]" -count=1` — 1 failure, 1 pass.

### Phase 5 — GREEN

**File modified**: `internal/controller/remediationorchestrator/blocking.go` — `countConsecutiveFailures`:

In the `case phase.Failed:` branch (line 112), add nested check:
```go
case phase.Failed:
    if rr.Status.FailurePhase == remediationv1.FailurePhaseDeduplicated {
        continue
    }
    consecutiveFailures++
```

**Expected**: Both tests pass.

**Validation**: `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-190-01[34]" -count=1` — 2 passes.

### Phase 5 — REFACTOR

- Verify the `continue` is correctly placed and doesn't affect `Skipped` or `Blocked` handling
- Run all existing blocking tests: `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="consecutive" -count=1`
- Verify existing integration blocking tests pass: `go test ./test/integration/remediationorchestrator/... -ginkgo.focus="consecutive\|blocking" -count=1`

### Phase 5-IT: Blocking Integration Test

**Test**: IT-RO-190-005

**File**: `test/integration/remediationorchestrator/dedup_inheritance_integration_test.go`

**RED**: Create 3 terminal RRs with same fingerprint (2 Failed/Deduplicated + 1 Failed/WorkflowExecution). Create new RR for same fingerprint. Assert it is NOT blocked (count = 1 < threshold 3).

**GREEN**: Should pass with Phase 5 implementation.

**REFACTOR**: Verify no timing issues with field index queries in envtest.

**Validation**: `go test ./test/integration/remediationorchestrator/... -ginkgo.focus="IT-RO-190-005" -count=1`

---

## Phase 6: Notification Provenance

**Goal**: Inherited-outcome notifications include provenance referencing the original WFE.

### Phase 6 — RED

**Test**: UT-RO-190-015

**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

Write test:
- **UT-RO-190-015**: RR with `DeduplicatedByWE = "original-wfe"`, original WFE in `Completed`. After `transitionToInheritedCompleted`, assert notification contains provenance string referencing "original-wfe".

Do the same for the failure path (`transitionToInheritedFailed`).

**Expected**: Fails (provenance message not yet added to notification).

### Phase 6 — GREEN

**Files modified**:
- `internal/controller/remediationorchestrator/reconciler.go` — `transitionToInheritedCompleted` / `transitionToInheritedFailed`:
  - Set notification message to include provenance: `"Remediation resolved via WorkflowExecution [name]"` or `"Inherited failure from WorkflowExecution [name]: [reason]"`
  - If notifications are emitted via existing `transitionToVerifying`/`transitionToFailed` callthrough, ensure the message field is set before the call

**Expected**: UT-RO-190-015 passes.

**Validation**: `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-190-015" -count=1`

### Phase 6 — REFACTOR

- Ensure provenance message format is consistent with existing notification patterns
- No code duplication between completed and failed provenance paths

---

## Phase 7: Documentation

**Goal**: Update authoritative design documents to reflect the new dedup collision semantics, cross-WE result inheritance, and CRD field documentation.

### Files modified

1. **`docs/architecture/decisions/DD-WE-003-resource-lock-persistence.md`**:
   - Add new section: "Deduplicated Collision Classification (Issue #190)"
   - Document: when lock is valid (running Job/PipelineRun from another WFE), the colliding WFE is marked `Failed/Deduplicated` with `DeduplicatedBy` pointing to the original WFE
   - Document: `MarkFailedAsDeduplicated` method and M5 constraint (AtomicStatusUpdate)
   - Update Implementation table with new functions
   - Update changelog

2. **`docs/architecture/decisions/DD-RO-002-ADDENDUM-blocked-phase-semantics.md`**:
   - Add new section: "Cross-WE Result Inheritance (Issue #190)"
   - Document: when a WFE reports `FailureReasonDeduplicated`, the RR stays in `PhaseExecuting` with `DeduplicatedByWE` set
   - Document: C3 short-circuit in `handleExecutingPhase` for result propagation
   - Document: `transitionToInheritedCompleted`/`transitionToInheritedFailed` helpers (C4)
   - Document: cross-WE watch pattern (first in codebase)
   - Document: `FailurePhaseDeduplicated` for audit/filtering
   - Document: BR-ORCH-042 exclusion for inherited failures
   - Update DuplicateInProgress table entry (line 103) to note `#614` for full outcome inheritance

3. **CRD inline documentation** (already done in Phase 1 GREEN, but verify):
   - `api/workflowexecution/v1alpha1/workflowexecution_types.go`: `DeduplicatedBy` field doc
   - `api/remediation/v1alpha1/remediationrequest_types.go`: `DeduplicatedByWE` field doc, `FailurePhaseDeduplicated` doc

### Validation

- Documentation review for consistency with implementation
- Verify no stale references to "Unknown" reason for cross-WFE collisions

---

## Validation Checkpoints

### After each phase

```bash
go build ./...
go test ./test/unit/workflowexecution/... -count=1
go test ./test/unit/remediationorchestrator/... -count=1
```

### After Phase 4-IT

```bash
go test ./test/integration/remediationorchestrator/... -count=1
go test ./test/integration/workflowexecution/... -count=1
```

### Final validation (after Phase 7)

```bash
go build ./...
golangci-lint run --timeout=5m
go test ./test/unit/workflowexecution/... -ginkgo.v -count=1
go test ./test/unit/remediationorchestrator/... -ginkgo.v -count=1
go test ./test/integration/workflowexecution/... -ginkgo.v -count=1
go test ./test/integration/remediationorchestrator/... -ginkgo.v -count=1
```

### Coverage check

```bash
go test ./test/unit/workflowexecution/... -coverprofile=we-unit.out -count=1
go test ./test/unit/remediationorchestrator/... -coverprofile=ro-unit.out -count=1
go tool cover -func=we-unit.out | tail -1
go tool cover -func=ro-unit.out | tail -1
```

---

## Summary

| Phase | TDD Phase | Tests | Files Modified | Est. Lines |
|-------|-----------|-------|---------------|------------|
| 1 RED | Write CRD constant tests | UT-WE-190-004 + RR constant test | Test files only | ~30 |
| 1 GREEN | Add CRD types | — | 2 API type files + make generate | ~20 |
| 1 REFACTOR | Verify CRD docs | — | Review only | 0 |
| 2 RED | Write WE collision tests | UT-WE-190-001,002,003,005,006,007,008 | Test file | ~200 |
| 2 GREEN | Implement MarkFailedAsDeduplicated + signature change | — | WE controller | ~120 |
| 2 REFACTOR | Extract shared logic | — | WE controller | ~-30 (net reduction) |
| 2-IT | WE integration tests | IT-WE-190-001,002 | Integration test file | ~80 |
| 3 RED | Write RO handler tests | UT-RO-190-001,002,003,004 | Test file | ~120 |
| 3 GREEN | Add HandleStatus dedup branch | — | WE handler | ~20 |
| 3 REFACTOR | Clean handler access | — | Handler | ~10 |
| 4 RED | Write propagation + idempotency tests | UT-RO-190-005,006,011,012,016 | Test file | ~200 |
| 4 GREEN | Implement C3 short-circuit + C4 helpers + watch + index | — | RO reconciler | ~200 |
| 4 REFACTOR | Extract propagation to separate file if needed | — | RO reconciler | ~0 |
| 4-IT | RO integration tests | IT-RO-190-001,002,003,004,006 | Integration test file | ~250 |
| 5 RED | Write consecutive failure exclusion tests | UT-RO-190-013,014 | Blocking test file | ~60 |
| 5 GREEN | Add nested FailurePhaseDeduplicated check | — | blocking.go | ~5 |
| 5 REFACTOR | Verify blocking test suite | — | Review only | 0 |
| 5-IT | Blocking integration test | IT-RO-190-005 | Integration test file | ~50 |
| 6 RED | Write notification provenance test | UT-RO-190-015 | Test file | ~40 |
| 6 GREEN | Add provenance to inherited transitions | — | RO reconciler | ~10 |
| 6 REFACTOR | Consistent message format | — | RO reconciler | ~5 |
| 7 | Documentation updates | — | DD-WE-003, DD-RO-002-ADDENDUM | ~150 |
| **Total** | | **31 tests** (28 original + UT-RO-190-017, 018, 019 from audit gap fixes) | | **~1540** |

---

## Post-Implementation Gap Analysis

After completing all 22 steps and 3 audit checkpoints, a review identified the following gaps in inherited transitions that were missed during the initial implementation:

### Gap 1: Missing Audit Traces (ADR-032 §1 Violation)

**Problem**: `transitionToInheritedCompleted` and `transitionToInheritedFailed` did not emit DataStorage audit events (`orchestrator.lifecycle.completed` / `orchestrator.lifecycle.failed`). Per ADR-032 §1, audit is **mandatory** for all lifecycle terminal transitions.

**Root Cause**: Phase 6 focused on K8s event provenance (`Recorder.Event`) but did not cross-check with the DataStorage audit pattern used by `transitionToFailed` and `handleVerifyingPhase`.

**Fix**: Added `emitCompletionAudit(ctx, rr, "InheritedCompleted", durationMs)` to `transitionToInheritedCompleted` and `emitFailureAudit(ctx, rr, FailurePhaseDeduplicated, failureErr, durationMs)` to `transitionToInheritedFailed`.

**Files Modified**: `internal/controller/remediationorchestrator/reconciler.go`

### Gap 2: Missing Completion Notifications (BR-ORCH-045)

**Problem**: `transitionToInheritedCompleted` did not call `ensureNotificationsCreated`. All other completion paths (via `handleVerifyingPhase`) call this to create `NotificationRequest` CRDs. Since the RR has `Outcome="InheritedCompleted"` set before the call, the notification body correctly reflects the inherited outcome.

**Root Cause**: The inherited completion path bypasses `transitionToVerifying` / `handleVerifyingPhase` (by design — no actual execution occurred), which is where notifications are normally triggered.

**Fix**: Added `ensureNotificationsCreated(ctx, rr)` after the audit call in `transitionToInheritedCompleted`.

**Files Modified**: `internal/controller/remediationorchestrator/reconciler.go`

### Gap 3: Missing Unit Test Coverage for Audit Provenance

**Problem**: No unit tests verified that inherited failure paths include the original WFE name in `FailureReason` for audit traceability, or that inherited failure K8s events contain the original WFE name.

**Fix**: Added 2 new unit tests:
- `UT-RO-190-017`: Validates inherited Failed emits K8s event with original WFE provenance
- `UT-RO-190-018`: Validates inherited Failed sets `FailureReason` containing original WFE name

Also enhanced `UT-RO-190-005` with explicit `Outcome="InheritedCompleted"` assertion.

**Files Modified**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

### Gap 4: Field-Level Kubebuilder Enum Missing `Deduplicated` (CRD Validation Blocker)

**Problem**: The `RemediationRequestStatus.FailurePhase` field had a duplicate kubebuilder `+kubebuilder:validation:Enum` that was narrower than the type-level enum and omitted `Deduplicated`. OpenAPI validation produces `allOf` with two enum constraints — their intersection would exclude `Deduplicated`, causing the API server to reject status updates setting `FailurePhase=Deduplicated`.

**Root Cause**: When `Deduplicated` was added to the type-level enum in Phase 1 (Step 2), the field-level enum on `RemediationRequestStatus.FailurePhase` was not updated to match.

**Fix**: Added `Deduplicated` to the field-level `+kubebuilder:validation:Enum` and regenerated CRD manifests via `make generate && make manifests`.

**Files Modified**: `api/remediation/v1alpha1/remediationrequest_types.go`, CRD YAML manifests (auto-generated)

### Gap 5: Idempotency Parity and Error Path Test Coverage

**Problem A**: `transitionToInheritedCompleted` and `transitionToInheritedFailed` emitted `PhaseTransitionsTotal` metrics and `Recorder.Event` even on reconcile retries where the phase had already transitioned. `transitionToFailed` gates these on `oldPhase != Failed`.

**Fix A**: Wrapped metrics, K8s events, audit, and notifications inside `if oldPhase != targetPhase` guard in both functions.

**Problem B**: The non-NotFound `Get` error path in `handleDedupResultPropagation` (transient API server errors) had no test coverage.

**Fix B**: Added `UT-RO-190-019` using `WithInterceptorFuncs` to inject a transient error, verifying the error surfaces to the reconcile loop and the RR remains in `Executing` phase.

**Files Modified**: `internal/controller/remediationorchestrator/reconciler.go`, `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

### Impact Assessment

| Area | Before Fix | After Fix |
|------|-----------|-----------|
| CRD validation | API server could reject `FailurePhase=Deduplicated` | Both type-level and field-level enums aligned |
| Audit compliance (ADR-032) | Inherited transitions invisible in DataStorage | Full audit trail for inherited Completed/Failed |
| Operator notifications (BR-ORCH-045) | No notification on inherited completion | Completion notification with `Outcome=InheritedCompleted` |
| SOC 2 traceability | Gap in lifecycle audit chain | Complete lifecycle audit chain for all terminal paths |
| Idempotency | Metrics/events could duplicate on reconcile retry | Gated by `oldPhase` check, matching `transitionToFailed` pattern |
| Error handling | Transient Get error path untested | UT-RO-190-019 validates error propagation |
| Test count | 28 tests | **31 tests** (+ UT-RO-190-017, UT-RO-190-018, UT-RO-190-019) |
