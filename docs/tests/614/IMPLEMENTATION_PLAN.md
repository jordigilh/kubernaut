# Implementation Plan: Issue #614 — RO-level DuplicateInProgress Outcome Inheritance

**Issue**: [#614](https://github.com/jordigilh/kubernaut/issues/614)
**Test Plan**: [TEST_PLAN.md](./TEST_PLAN.md)
**Branch**: `development/v1.4`
**Created**: 2026-03-04

---

## Summary

| Metric | Value |
|--------|-------|
| Phases | 6 implementation + 2 audit checkpoints |
| TDD Steps | 18 (interleaved RED → GREEN → REFACTOR, no REFACTOR skipped) |
| Unit Tests | 12 (8 P0 new behavior + 4 P1 regression guards) |
| Integration Tests | 3 (P0 lifecycle + failure exclusion) |
| **Total Tests** | **15** |
| Files Modified | 3 production (`blocking.go`, `reconciler.go`, call sites) + 2 test files |
| Files Created | 1 test file (`dedup_blocking_test.go`) |
| Estimated Effort | Medium (refactor + targeted behavioral change) |

---

## Due Diligence Findings (Pre-Execution)

The following findings were identified during pre-execution due diligence and have been incorporated into the phases below:

| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| F-1 | LOW | `SetReady` messages in `transitionToInheritedCompleted/Failed` hardcode "WFE" | Parameterize message using `sourceKind` in Phase 1 |
| F-2 | LOW | Block fields (`BlockReason`, `DuplicateOf`) retained on terminal RRs | Confirmed safe — matches `transitionToFailedTerminal` precedent, provides audit trail |
| F-3 | MEDIUM | `ensureNotificationsCreated` will no-op for DuplicateInProgress RRs (no AIAnalysis exists) | Guard call with `sourceKind == "WorkflowExecution"` in Phase 1 |
| F-4 | LOW | `default` branch in terminal switch catches `TimedOut`/`Cancelled`/`Skipped` | Acceptable — all represent "original didn't succeed" |
| F-5 | LOW | DuplicateInProgress RRs may have nil `StartTime` | Acceptable — audit emits conditionally; test preconditions set `StartTime` when needed |
| F-6 | LOW | Gauge decrement before transition risks metric drift if status update fails | Move gauge decrement AFTER successful transition in Phase 2 |

---

## Phase 1: Generalize Transition Method Signatures (REFACTOR)

**Goal**: Change `transitionToInheritedCompleted` and `transitionToInheritedFailed` to accept `sourceRef`/`sourceKind` parameters. Update all existing call sites. No behavioral change — pure refactor.

### Step 1: Update method signatures + call sites

**Type**: REFACTOR (no RED/GREEN — pure signature change)

**Actions**:
1. Change `transitionToInheritedCompleted(ctx, rr)` → `transitionToInheritedCompleted(ctx, rr, sourceRef, sourceKind string)`
   - Replace `rr.Status.DeduplicatedByWE` references in K8s event messages with `fmt.Sprintf("%s %s", sourceKind, sourceRef)`
   - Replace log field `"originalWFE"` with `"inheritedFrom"` and `"sourceKind"`
   - **F-1**: Parameterize `SetReady` message from `"Inherited completion from original WFE"` to `fmt.Sprintf("Inherited completion from original %s", sourceKind)`
   - **F-3**: Guard `ensureNotificationsCreated` call with `if sourceKind == "WorkflowExecution"` — DuplicateInProgress RRs never reached Analyzing, so no AIAnalysis exists and notification creation would fail with a misleading error log
2. Change `transitionToInheritedFailed(ctx, rr, failureErr)` → `transitionToInheritedFailed(ctx, rr, failureErr, sourceRef, sourceKind string)`
   - Same event/log/SetReady parameterization as above
   - **F-1**: Parameterize `SetReady` message from `"Inherited failure from original WFE"` to `fmt.Sprintf("Inherited failure from original %s", sourceKind)`
3. Update `handleDedupResultPropagation` call sites (3 locations):
   - `transitionToInheritedCompleted(ctx, rr, rr.Status.DeduplicatedByWE, "WorkflowExecution")`
   - `transitionToInheritedFailed(ctx, rr, err, rr.Status.DeduplicatedByWE, "WorkflowExecution")` (2 call sites: Failed + NotFound)

**Validation**:
```bash
go build ./...
go vet ./internal/controller/remediationorchestrator/...
go test ./test/unit/remediationorchestrator/... -ginkgo.v -ginkgo.focus="UT-RO-190" -count=1
```

**Exit Criteria**: All 31 existing #190 unit tests pass. Build clean. No behavioral change.

**Modified Files**:
- `internal/controller/remediationorchestrator/reconciler.go` (signature + event/log strings)

---

## Phase 2: recheckDuplicateBlock — Happy Path Inheritance (RED → GREEN)

**Goal**: Make `recheckDuplicateBlock` inherit the original RR's terminal outcome instead of clearing to Pending.

### Step 2: RED — UT-RO-614-001 (original Completed → inherits Completed)

**Test**: Create `test/unit/remediationorchestrator/controller/dedup_blocking_test.go`

```
Describe("recheckDuplicateBlock (Issue #614)")
  Context("when original RR has reached terminal phase")
    It("UT-RO-614-001: inherits Completed when original RR completes")
      Setup: rr-dup in PhaseBlocked/DuplicateInProgress, DuplicateOf="rr-original"
             rr-original in PhaseCompleted, Outcome="Completed"
      Assert: rr-dup.Status.OverallPhase == Completed
              rr-dup.Status.Outcome == "InheritedCompleted"
              rr-dup.Status.CompletedAt is set
```

**Verify RED**: `go test ... -ginkgo.focus="UT-RO-614-001"` → FAIL (currently transitions to Pending)

### Step 3: RED — UT-RO-614-002 (original Failed → inherits Failed)

**Test**: Add to `dedup_blocking_test.go`

```
    It("UT-RO-614-002: inherits Failed when original RR fails")
      Setup: rr-original in PhaseFailed, FailurePhase=WorkflowExecution
      Assert: rr-dup.Status.OverallPhase == Failed
              rr-dup.Status.FailurePhase == Deduplicated
              rr-dup.Status.FailureReason contains "rr-original"
              rr-dup.Status.CompletedAt is set
```

**Verify RED**: `go test ... -ginkgo.focus="UT-RO-614-002"` → FAIL

### Step 4: GREEN — Implement inheritance in recheckDuplicateBlock

**Actions**:
1. In `recheckDuplicateBlock`, replace the terminal-phase branch:

   **Before** (blocking.go lines 370-373):
   ```go
   if IsTerminalPhase(originalRR.Status.OverallPhase) {
       logger.Info("Original RR reached terminal phase, clearing DuplicateInProgress block",
           "duplicateOf", originalRR.Name, "originalPhase", originalRR.Status.OverallPhase)
       return r.clearEventBasedBlock(ctx, rr, phase.Pending)
   }
   ```

   **After** (F-6: gauge decrement AFTER successful transition to prevent metric drift on failure):
   ```go
   if IsTerminalPhase(originalRR.Status.OverallPhase) {
       var result ctrl.Result
       var err error

       switch originalRR.Status.OverallPhase {
       case phase.Completed:
           logger.Info("Original RR completed, inheriting outcome",
               "duplicateOf", originalRR.Name)
           result, err = r.transitionToInheritedCompleted(ctx, rr, rr.Status.DuplicateOf, "RemediationRequest")
       default:
           logger.Info("Original RR failed, inheriting failure",
               "duplicateOf", originalRR.Name, "originalPhase", originalRR.Status.OverallPhase)
           result, err = r.transitionToInheritedFailed(ctx, rr,
               fmt.Errorf("original RemediationRequest %q reached %s", originalRR.Name, originalRR.Status.OverallPhase),
               rr.Status.DuplicateOf, "RemediationRequest")
       }
       if err == nil {
           r.Metrics.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Dec()
       }
       return result, err
   }
   ```

2. Also update the NotFound branch (blocking.go lines 361-364):

   **Before**:
   ```go
   if apierrors.IsNotFound(err) {
       logger.Info("Original RR no longer exists, clearing DuplicateInProgress block",
           "duplicateOf", rr.Status.DuplicateOf)
       return r.clearEventBasedBlock(ctx, rr, phase.Pending)
   }
   ```

   **After** (F-6: gauge decrement AFTER successful transition):
   ```go
   if apierrors.IsNotFound(err) {
       logger.Info("Original RR deleted, inheriting failure",
           "duplicateOf", rr.Status.DuplicateOf)
       result, inheritErr := r.transitionToInheritedFailed(ctx, rr,
           fmt.Errorf("original RemediationRequest %q deleted before completion", rr.Status.DuplicateOf),
           rr.Status.DuplicateOf, "RemediationRequest")
       if inheritErr == nil {
           r.Metrics.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Dec()
       }
       return result, inheritErr
   }
   ```

**Validation**:
```bash
go build ./...
go test ... -ginkgo.focus="UT-RO-614-001|UT-RO-614-002" -count=1
```

**Exit Criteria**: UT-RO-614-001 and UT-RO-614-002 pass. Build clean.

**Modified Files**:
- `internal/controller/remediationorchestrator/blocking.go` (recheckDuplicateBlock)

### Step 4b: REFACTOR — Clean up recheckDuplicateBlock

**Type**: REFACTOR (behavior-preserving)

**Actions**:
1. Review `recheckDuplicateBlock` for clarity and dead code from removed `clearEventBasedBlock` calls
2. Extract shared gauge-decrement-then-inherit pattern if the Completed/Failed/NotFound branches duplicate logic
3. Simplify branch structure (e.g., terminal switch could fold Skipped/Cancelled into default Failed path)
4. Verify no orphaned imports or unused variables

**Validation**:
```bash
go build ./...
go test ... -ginkgo.focus="UT-RO-614-001|UT-RO-614-002" -count=1
```

**Exit Criteria**: Tests still pass. Code is cleaner. No behavioral change.

---

## Phase 3: Edge Cases + Observability (RED → GREEN → REFACTOR)

**Goal**: Cover the deleted-original path, gauge decrement, K8s event provenance, audit, and notification creation.

### Step 5: RED — UT-RO-614-003 (deleted original → Failed)

**Test**: Add to `dedup_blocking_test.go`

```
    It("UT-RO-614-003: inherits Failed when original RR is deleted")
      Setup: rr-dup blocked, DuplicateOf="rr-deleted", no rr-deleted in fake client
      Assert: rr-dup.Status.OverallPhase == Failed
              rr-dup.Status.FailurePhase == Deduplicated
              rr-dup.Status.FailureReason contains "rr-deleted" and "deleted"
```

**Verify RED**: Should actually PASS immediately (NotFound branch implemented in Step 4). If it passes → skip GREEN, move on.

### Step 6: RED — UT-RO-614-004 (gauge decrement)

**Test**: Add to `dedup_blocking_test.go`

```
    It("UT-RO-614-004: decrements CurrentBlockedGauge on inheritance")
      Setup: Initialize gauge to 1.0, trigger inheritance
      Assert: gauge value == 0.0
```

**Verify**: Should PASS (gauge decrement added in Step 4). If not, fix.

### Step 7: RED — UT-RO-614-005, 006, 007, 008 (events, audit, notifications)

**Tests**: Add to `dedup_blocking_test.go`

```
    It("UT-RO-614-005: K8s event mentions RemediationRequest source")
      Assert: fake recorder contains event with "RemediationRequest rr-original"

    It("UT-RO-614-006: audit event emitted for inherited completion")
      Assert: audit store received orchestrator.lifecycle.completed with InheritedCompleted

    It("UT-RO-614-007: audit event emitted for inherited failure")
      Assert: audit store received orchestrator.lifecycle.failed with Deduplicated

    It("UT-RO-614-008: ensureNotificationsCreated called without error")
      Assert: no panic, no error (even with minimal-data RR)
```

**Verify**: Most should PASS since `transitionToInheritedCompleted/Failed` already emits events/audit. If any fail, implement the missing piece.

**GREEN** (if needed): Wire up any missing audit or notification calls.

### Step 7b: REFACTOR — Consolidate test helpers

**Type**: REFACTOR (behavior-preserving)

**Actions**:
1. Consolidate test helpers across `dedup_blocking_test.go` — reduce setup duplication
2. Extract shared helper for creating a blocked RR with DuplicateOf set
3. Extract shared reconciler builder helper (fake client + apiReader + metrics + recorder)
4. Verify assertion quality: no null-testing anti-patterns, business outcome focus

**Validation**:
```bash
go test ... -ginkgo.focus="UT-RO-614" -count=1
```

**Exit Criteria**: All UT-RO-614-001..008 pass. Test code is DRY.

---

## Audit Checkpoint 1

**Trigger**: After Phase 3 completes (all P0 new behavior tests passing).

**Checklist**:
1. `go build ./...` — clean
2. `go vet ./internal/controller/remediationorchestrator/...` — clean
3. `go test ./test/unit/remediationorchestrator/... -count=1` — ALL tests pass (including #190)
4. `go test ./test/unit/workflowexecution/... -count=1` — no regressions
5. Verify K8s event message format is correct for both WE-level and RR-level sources
6. Verify `CurrentBlockedGauge` is decremented exactly once per inheritance
7. Verify no `ToNot(BeNil)` or `ToNot(BeEmpty)` anti-patterns in new test code
8. Verify no `time.Sleep()` in tests
9. Verify no `Skip()` in tests

**If findings**: Document, fix, and re-verify before proceeding to Phase 4.

---

## Phase 4: Regression Guards (RED → REFACTOR)

**Goal**: Verify existing behavior remains unchanged for edge cases and #190 WE-level paths. GREEN is skipped because these tests validate unchanged behavior — they should pass immediately.

### Step 8: RED — UT-RO-614-009 (original still active → requeue)

**Test**: Add to `dedup_blocking_test.go`

```
  Context("when original RR is still active (regression guards)")
    It("UT-RO-614-009: requeues when original RR is non-terminal")
      Setup: rr-original in PhaseExecuting
      Assert: rr-dup.Status.OverallPhase remains Blocked
              Result.RequeueAfter > 0
              No error
```

**Verify**: Should PASS immediately (existing behavior, no code change in this path).

### Step 9: RED — UT-RO-614-010 (empty DuplicateOf → Pending)

**Test**: Add to `dedup_blocking_test.go`

```
    It("UT-RO-614-010: clears to Pending when DuplicateOf is empty")
      Setup: rr-dup blocked with DuplicateOf=""
      Assert: rr-dup.Status.OverallPhase == Pending
              Block fields cleared
```

**Verify**: Should PASS immediately (existing fallback at blocking.go:351-352).

### Step 10: RED — UT-RO-614-011, 012 (#190 WE-level regression)

**Tests**: Add to existing `dedup_handler_test.go`

```
    It("UT-RO-614-011: WE-level inherited Completed event references WorkflowExecution")
      Assert: fake recorder event contains "WorkflowExecution original-wfe"

    It("UT-RO-614-012: WE-level inherited Failed event references WorkflowExecution")
      Assert: fake recorder event contains "WorkflowExecution original-wfe"
```

**Verify**: Should PASS (method now receives "WorkflowExecution" as sourceKind from `handleDedupResultPropagation`).

### Step 10b: REFACTOR — Review full UT-RO-614 test suite

**Type**: REFACTOR (behavior-preserving)

**Actions**:
1. Review full UT-RO-614 suite structure (001..012)
2. Convert repetitive test setups to table-driven tests where appropriate
3. Deduplicate reconciler/client setup code across the suite
4. Ensure consistent naming conventions and assertion patterns

**Validation**:
```bash
go test ... -ginkgo.focus="UT-RO-614" -count=1
```

**Exit Criteria**: All 12 UTs pass. Suite is cohesive and DRY.

---

## Phase 5: Integration Tests (RED → GREEN → REFACTOR)

**Goal**: Verify full reconciliation lifecycle via envtest for DuplicateInProgress inheritance.

### Step 11: RED — IT-RO-614-001 (Blocked → original Completed → inherits)

**Test**: Add new Describe block to `test/integration/remediationorchestrator/dedup_propagation_integration_test.go`

```
Describe("Issue #614: DuplicateInProgress outcome inheritance")
  It("IT-RO-614-001: inherits Completed when original RR completes")
    Setup: Create rr-original in Completed. Create rr-dup in Blocked/DuplicateInProgress.
    Assert: Eventually rr-dup.Status.OverallPhase == Completed
            rr-dup.Status.Outcome == "InheritedCompleted"
```

### Step 12: RED — IT-RO-614-002 (Blocked → original deleted → inherits Failed)

```
  It("IT-RO-614-002: inherits Failed when original RR is deleted")
    Setup: Create rr-dup in Blocked with DuplicateOf="rr-nonexistent" (never created)
    Assert: Eventually rr-dup.Status.OverallPhase == Failed
            rr-dup.Status.FailurePhase == Deduplicated
```

### Step 13: RED — IT-RO-614-003 (consecutive failure exclusion)

```
  It("IT-RO-614-003: RR-level inherited failure excluded from consecutive count")
    Setup: 2 terminal RRs (same fingerprint): 1 Failed/Deduplicated (from DuplicateInProgress),
           1 Failed/WorkflowExecution
    Assert: countConsecutiveFailures returns 1
```

### Step 14: GREEN + REFACTOR

**Validation**:
```bash
go test ./test/integration/remediationorchestrator/... -ginkgo.v -ginkgo.focus="IT-RO-614" -count=1
```

If any fail, investigate and fix. Then REFACTOR: look for code duplication between `recheckDuplicateBlock` and `recheckResourceBusyBlock`, common patterns to extract.

---

## Audit Checkpoint 2

**Trigger**: After Phase 5 completes (all 15 tests passing).

**Comprehensive Checklist**:

### Build & Tests
1. `go build ./...` — clean
2. `go vet ./...` — clean
3. Full unit test suite: `go test ./test/unit/remediationorchestrator/... -count=1` — ALL pass
4. Full WE unit test suite: `go test ./test/unit/workflowexecution/... -count=1` — no regressions
5. Integration tests: `go test ./test/integration/remediationorchestrator/... -count=1` — ALL pass

### Anti-Pattern Compliance
6. No `ToNot(BeNil)` or `ToNot(BeEmpty)` in any new/modified test file
7. No `time.Sleep()` in any test code
8. No `Skip()` in any test code
9. All tests use Ginkgo/Gomega BDD framework

### CRD & Schema
10. No new CRD types added → no `make generate` needed → verify

### Audit & Metrics
11. All inherited terminal transitions emit audit events (ADR-032 §1)
12. `PhaseTransitionsTotal` recorded for both WE-level and RR-level inheritance
13. `CurrentBlockedGauge` decremented exactly once per DuplicateInProgress inheritance
14. No metric double-counting on reconcile retries (idempotency guards from #190 F-4 still active)

### Documentation
15. DD-RO-002-ADDENDUM: Verify "Inherits outcome" documentation now matches implementation
16. blocking.go: Function-level comments on `recheckDuplicateBlock` updated
17. reconciler.go: Function-level comments on `transitionToInherited*` updated with sourceRef/sourceKind

### Regression
18. All 31 #190 unit tests pass
19. All existing blocking tests pass
20. K8s events for WE-level inheritance still reference "WorkflowExecution"
21. K8s events for RR-level inheritance reference "RemediationRequest"

**If findings**: Document severity (CRITICAL/LOW), fix systematically, re-verify.

---

## Phase 6: Documentation

**Goal**: Update authoritative documentation to reflect #614 implementation.

### Step 15: Update DD-RO-002-ADDENDUM

Add "Issue #614" subsection to the existing "Issue #190" section (or as a sibling section):

- **recheckDuplicateBlock behavior change**: Inherits outcome instead of clearing to Pending
- **Generalized transition methods**: sourceRef/sourceKind parameterization
- **Gauge correctness**: CurrentBlockedGauge decremented before inheritance
- **Semantic: Blocked → terminal via inheritance** (not Blocked → Pending → ... → terminal)

### Step 16: Update blocking.go comments

Update `recheckDuplicateBlock` function-level comment:
```go
// recheckDuplicateBlock handles Blocked RRs with BlockReason=DuplicateInProgress.
// Uses apiReader to check if the original RR has reached a terminal phase.
// If terminal: inherits the original's outcome via transitionToInheritedCompleted/Failed.
// If deleted: inherits failure with FailurePhaseDeduplicated.
// If still active: requeues for periodic rechecking.
//
// Issue #614: Changed from clearEventBasedBlock(Pending) to outcome inheritance.
// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
```

---

## File Change Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/controller/remediationorchestrator/reconciler.go` | MODIFY | Generalize `transitionToInheritedCompleted/Failed` signatures; update `handleDedupResultPropagation` call sites |
| `internal/controller/remediationorchestrator/blocking.go` | MODIFY | Replace `clearEventBasedBlock(Pending)` with inheritance calls in `recheckDuplicateBlock` |
| `test/unit/remediationorchestrator/controller/dedup_blocking_test.go` | CREATE | 10 unit tests for recheckDuplicateBlock inheritance (UT-RO-614-001..010) |
| `test/unit/remediationorchestrator/controller/dedup_handler_test.go` | MODIFY | 2 regression guard tests (UT-RO-614-011, 012); update existing #190 tests for new method signatures |
| `test/integration/remediationorchestrator/dedup_propagation_integration_test.go` | MODIFY | 3 integration tests (IT-RO-614-001..003) |
| `docs/architecture/decisions/DD-RO-002-ADDENDUM-blocked-phase-semantics.md` | MODIFY | Add Issue #614 section |
