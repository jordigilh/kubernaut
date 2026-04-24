# Test Plan: WE/RO Deduplicated Phase with Result Inheritance

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-190-v3
**Feature**: Execution-time collision detection with deduplicated failure reason and cross-WE result inheritance
**Version**: 3.0
**Created**: 2026-03-04
**Author**: Kubernaut Development
**Status**: Draft
**Branch**: `development/v1.4`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

When two WorkflowExecutions (WFEs) target the same Kubernetes resource, the second one encounters an `AlreadyExists` error during Job or PipelineRun creation. Today this is treated as a generic `Unknown` failure, which pollutes consecutive failure counts (BR-ORCH-042), triggers incorrect failure notifications, and loses the relationship to the original WFE. This test plan validates that execution-time collisions are correctly classified as `Deduplicated`, that the deduplicated RR inherits the outcome of the original WFE, and that inherited failures are excluded from consecutive failure blocking.

### 1.2 Objectives

1. **WE collision classification**: The Job collision path in `reconcilePending` (via extended `handleJobAlreadyExists`) and the Tekton collision path in `HandleAlreadyExists` set `FailureDetails.Reason = "Deduplicated"` and `DeduplicatedBy` when a valid lock (running Job/PipelineRun) is detected from another WFE. For Jobs, `handleJobAlreadyExists` is extended to detect the original WFE from Job labels when the lock is valid (running), returning this info to the caller at `reconcilePending` line 526 which currently falls through to `MarkFailedWithReason("Unknown", ...)`.
2. **RO dedup branching**: `WorkflowExecutionHandler.HandleStatus` branches on `FailureDetails.Reason == "Deduplicated"` — does NOT call `transitionToFailed`, instead marks RR with `DeduplicatedByWE` and keeps it in `PhaseExecuting`.
3. **Cross-WE result inheritance**: When the original WFE reaches terminal, the deduplicated RR inherits `Completed` or `Failed` (with `FailurePhaseDeduplicated`).
4. **Dangling reference**: If the original WFE is deleted before reaching terminal, the deduplicated RR transitions to `Failed` with `FailurePhaseDeduplicated` and a clear message.
5. **Consecutive failure exclusion**: Inherited (deduplicated) failures do NOT increment the consecutive failure counter (BR-ORCH-042).
6. **CRD schema correctness**: New enum values and fields are additive and backward-compatible.
7. **Notification provenance**: Terminal notifications for inherited outcomes include provenance referencing the original WFE.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/workflowexecution/... ./test/unit/remediationorchestrator/...` |
| Integration test pass rate | 100% | `go test ./test/integration/workflowexecution/... ./test/integration/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable files |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- [BR-ORCH-042](../../../docs/requirements/BR-ORCH-042-consecutive-failure-blocking.md): Consecutive failure blocking with cooldown
- [DD-WE-003](../../../docs/architecture/decisions/DD-WE-003-resource-lock-persistence.md): Resource lock persistence via deterministic naming
- [DD-RO-002-ADDENDUM](../../../docs/architecture/decisions/DD-RO-002-ADDENDUM-blocked-phase-semantics.md): Blocked phase semantics
- Issue #190: WE/RO: Skipped/Deduplicated phase with result inheritance from original WE
- Issue #614: RO-level DuplicateInProgress outcome inheritance (deferred, not in scope)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Original WFE deleted before outcome read | Deduplicated RR stuck in Executing forever | Low | UT-RO-190-011, IT-RO-190-004 | Detect missing WFE → transition to Failed with FailurePhaseDeduplicated |
| R2 | Original WFE hangs (never reaches terminal) | Deduplicated RR stuck in Executing | Low | UT-RO-190-012 | Global timeout on RR still fires; tested separately (existing coverage) |
| R3 | Inherited failures counted toward consecutive blocking | Cascading blocks for signals that were never truly retried | Medium | UT-RO-190-013, IT-RO-190-005 | countConsecutiveFailures skips FailurePhaseDeduplicated |
| R4 | Cross-RR watch triggers excessive reconciles | Performance degradation | Low | IT-RO-190-006 | Terminal-phase predicate + field index on DeduplicatedByWE |
| R5 | CRD enum change breaks existing resources | Validation failures on existing CRDs | Low | UT-WE-190-004, UT-RO-190-014 | Additive enum values only; no removal |
| R6 | Job label missing (orphaned or manually created Job) | Cannot identify original WFE | Low | UT-WE-190-003 | Fallback to FailureReasonUnknown when label absent |
| R7 | Tekton PipelineRun label missing | Cannot identify original WFE for Tekton path | Low | UT-WE-190-007 | Same fallback as R6 |
| R8 | Cross-WE watch reconcile amplification | Performance degradation from excessive reconciles | Medium | IT-RO-190-006 | Terminal-phase predicate on watch; field index limits fan-out to only matching RRs |
| R9 | Namespace scoping for DeduplicatedBy | Name-only reference breaks if multi-namespace execution enabled | Low | N/A (v1.5 concern) | Documented as known limitation; name suffices within single execution namespace |
| R10 | Repeated reconcile on dedup-waiting RR re-processes dedup branch | Duplicate events, re-set fields, unstable requeue | Medium | UT-RO-190-016 | Idempotency guard: if DeduplicatedByWE already set, skip dedup processing in HandleStatus |

### 3.1 Risk-to-Test Traceability

- **R1 (High priority)**: UT-RO-190-011 (unit), IT-RO-190-004 (integration) — both verify the dangling-reference path.
- **R3 (Medium priority)**: UT-RO-190-013 (unit), IT-RO-190-005 (integration) — verify consecutive failure exclusion.
- **R6/R7 (Low priority)**: UT-WE-190-003, UT-WE-190-007 — verify fallback when labels are missing.
- **R8 (Medium priority)**: IT-RO-190-006 — verify watch predicate limits reconcile scope to terminal WFE changes only.
- **R10 (Medium priority)**: UT-RO-190-016 — verify idempotency when dedup-waiting RR is reconciled multiple times.

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **WE Controller collision paths** (`internal/controller/workflowexecution/workflowexecution_controller.go`):
  - **Job path** (`reconcilePending` lines 513-529 + `handleJobAlreadyExists`): When `handleJobAlreadyExists` returns `(nil, false, false)` for a running Job (valid lock), the current fallback at line 526 calls `MarkFailedWithReason("Unknown", ...)`. The change extends `handleJobAlreadyExists` to also return the original WFE name (from Job labels) so the caller can call `MarkFailedWithReason("Deduplicated", ...)` + set `DeduplicatedBy`.
  - **Tekton path** (`HandleAlreadyExists` line 1197): When another WFE owns the PipelineRun, change from `MarkFailedWithReason("Unknown", ...)` to `MarkFailedWithReason("Deduplicated", ...)` + set `DeduplicatedBy`.
  - **New method** `MarkFailedAsDeduplicated(ctx, wfe, originalWFE)`: Sets `FailureDetails.Reason = "Deduplicated"` AND `DeduplicatedBy` atomically inside `AtomicStatusUpdate` closure (M5 constraint — refetch overwrites pre-set fields)

- **WE CRD extensions** (`api/workflowexecution/v1alpha1/workflowexecution_types.go`):
  - New constant `FailureReasonDeduplicated = "Deduplicated"`
  - New status field `DeduplicatedBy string`
  - Kubebuilder enum update: `+kubebuilder:validation:Enum=...,Deduplicated`

- **RR CRD extensions** (`api/remediation/v1alpha1/remediationrequest_types.go`):
  - New constant `FailurePhaseDeduplicated FailurePhase = "Deduplicated"`
  - New status field `DeduplicatedByWE string`
  - Kubebuilder enum update

- **RO WE handler branching** (`pkg/remediationorchestrator/handler/workflowexecution.go`):
  - `HandleStatus` branching on `FailureDetails.Reason == "Deduplicated"`: skip `transitionToFailed`, set `DeduplicatedByWE`, keep `PhaseExecuting`, requeue

- **RO reconciler dedup-aware executing phase** (`internal/controller/remediationorchestrator/reconciler.go`):
  - **Design Decision (C3=Option B)**: Result propagation is handled in `handleExecutingPhase` BEFORE calling `HandleStatus`. When `rr.Status.DeduplicatedByWE` is set, `handleExecutingPhase` short-circuits to the cross-WFE propagation logic instead of delegating to `HandleStatus`. This keeps cross-WFE logic separate from single-WFE status handling.
  - `Watches(&WFE{}, ...)` with terminal-phase predicate for cross-RR reconciliation
  - Field index on `status.deduplicatedByWE`
  - **Result propagation helpers (C4)**: New `transitionToInheritedCompleted(ctx, rr, originalWFE)` and `transitionToInheritedFailed(ctx, rr, originalWFE)` functions that call through to existing terminal transition logic with provenance metadata (original WFE name/namespace). These are separate from `transitionToFailed`/`transitionToVerifying` to avoid adding dedup flags to existing functions.
  - Dangling reference: original WFE not found → `transitionToInheritedFailed` with clear message

- **RO blocking exclusion** (`internal/controller/remediationorchestrator/blocking.go`):
  - `countConsecutiveFailures`: skip RRs with `FailurePhaseDeduplicated`

- **Notification provenance** (`pkg/remediationorchestrator/handler/workflowexecution.go`):
  - Inherited-outcome notification includes provenance message

### 4.2 Features Not to be Tested

- **RO-level DuplicateInProgress outcome inheritance**: Deferred to #614
- **Ansible executor collision handling**: AWX uses HTTP API, no K8s `AlreadyExists` errors (confirmed during due diligence)
- **CRD regeneration mechanics**: Covered by `make generate` (infrastructure, not behavioral)
- **Helm chart rendering**: Covered by existing Helm test infrastructure
- **Global timeout on deduplicated RRs**: Existing coverage for `handleGlobalTimeout` applies; no new logic

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Unified `Deduplicated` reason for both Job and Tekton | Simpler CRD enum; both collision paths have identical semantics |
| Status field `DeduplicatedByWE` (not label) | Immutable via status subresource; audit-safe; supports field index |
| New `FailurePhaseDeduplicated` on RR | Enables filtering and audit distinction for inherited vs. direct failures |
| Cross-RR watch with terminal predicate | Event-driven (no polling); predicate limits reconcile fan-out |
| Deduplicated RR stays in `PhaseExecuting` while waiting | Non-terminal phase prevents Gateway from creating new RRs (DD-RO-002-ADDENDUM) |
| C3: Result propagation in `handleExecutingPhase` (Option B) | Short-circuit before `HandleStatus` when `DeduplicatedByWE` is set. Keeps cross-WFE logic separate from single-WFE status handling; `HandleStatus` stays focused on the RR's own WFE |
| C4: Dedicated `transitionToInheritedCompleted`/`Failed` helpers | Avoids adding dedup-specific flags to existing `transitionToFailed`/`transitionToVerifying`. Encapsulates provenance metadata and audit for inherited outcomes. Calls through to existing terminal logic internally |
| M1: Job-path signature change | `handleJobAlreadyExists` return type changes from `(*CreateResult, bool, bool)` to `(*CreateResult, bool, bool, string)` — 4th return is `originalWFE` name (empty if label missing). Caller at `reconcilePending` line 514 receives it and branches: if non-empty → `MarkFailedAsDeduplicated`; if empty → existing `MarkFailedWithReason("Unknown")` |
| M5: `MarkFailedAsDeduplicated` method | `MarkFailedWithReason` uses `AtomicStatusUpdate` which refetches the WFE (line 61 of `pkg/workflowexecution/status/manager.go`), overwriting any pre-set status fields. `DeduplicatedBy` MUST be set inside the closure. Solution: new method `MarkFailedAsDeduplicated(ctx, wfe, originalWFE)` that internally calls the same atomic pattern as `MarkFailedWithReason` but also sets `wfe.Status.DeduplicatedBy` inside the closure. For Tekton path, `HandleAlreadyExists` calls `MarkFailedAsDeduplicated` directly. |
| Idempotency guard on dedup-waiting reconciles | When `DeduplicatedByWE` is already set, `handleExecutingPhase` enters propagation path directly without re-processing through `HandleStatus`, preventing duplicate events and re-set fields |

### 4.4 Verified Implementation Patterns

> Patterns verified against the actual codebase during risk mitigation (2026-03-04).

**M1: `handleJobAlreadyExists` signature change**

Current signature:
```go
func (r *WorkflowExecutionReconciler) handleJobAlreadyExists(
    ctx context.Context, exec weexecutor.Executor,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
    resourceName string, createOpts weexecutor.CreateOptions,
) (*weexecutor.CreateResult, bool, bool)
```

New signature (4th return = original WFE name):
```go
func (r *WorkflowExecutionReconciler) handleJobAlreadyExists(
    ctx context.Context, exec weexecutor.Executor,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
    resourceName string, createOpts weexecutor.CreateOptions,
) (*weexecutor.CreateResult, bool, bool, string)
```

When the Job is running (valid lock), extend the `!completed` branch to:
1. Fetch the existing Job via `jobExec` (using the deterministic resource name)
2. Read label `kubernaut.ai/workflow-execution` → `originalWFE`
3. Return `(nil, false, false, originalWFE)` — or `(nil, false, false, "")` if label missing

Caller at `reconcilePending` (line 514) changes:
```go
retryResult, handled, requeueForGC, originalWFE := r.handleJobAlreadyExists(...)
// ... existing handled/requeueForGC checks ...
if createErr != nil {
    if originalWFE != "" {
        markErr := r.MarkFailedAsDeduplicated(ctx, wfe, originalWFE)
        return ctrl.Result{}, markErr
    }
    markErr := r.MarkFailedWithReason(ctx, wfe, "Unknown", ...)
    return ctrl.Result{}, markErr
}
```

**M2: controller-runtime Watches API (v0.23.3)**

Verified available at `vendor/sigs.k8s.io/controller-runtime/pkg/builder/controller.go:159`:
```go
func (blder *TypedBuilder[request]) Watches(
    object client.Object,
    eventHandler handler.TypedEventHandler[client.Object, request],
    opts ...WatchesOption,
) *TypedBuilder[request]
```

`handler.EnqueueRequestsFromMapFunc` available at `vendor/.../handler/enqueue_mapped.go:50`.
`builder.WithPredicates` available at `vendor/.../builder/options.go:48`.

Usage pattern for `SetupWithManager`:
```go
return ctrl.NewControllerManagedBy(mgr).
    For(&remediationv1.RemediationRequest{}).
    Owns(&workflowexecutionv1.WorkflowExecution{}).
    // ... existing Owns ...
    Watches(
        &workflowexecutionv1.WorkflowExecution{},
        handler.EnqueueRequestsFromMapFunc(r.findRRsForDedupWFE),
        builder.WithPredicates(r.wfeTerminalPhasePredicate()),
    ).
    Complete(r)
```

**NOTE**: This is the first cross-object watch in the Kubernaut codebase. All existing watches use `Owns()` which relies on owner references. The `Watches` + `MapFunc` pattern is required because the deduplicated RR does not own the original WFE.

**M3: Status field index**

Field indexes are in-memory cache indexes using Go extractor functions (not K8s server-side field selectors). Existing indexes use `spec.*` fields (e.g., `spec.signalFingerprint`), but status fields work identically:
```go
mgr.GetFieldIndexer().IndexField(ctx,
    &remediationv1.RemediationRequest{},
    "status.deduplicatedByWE",
    func(obj client.Object) []string {
        rr := obj.(*remediationv1.RemediationRequest)
        if rr.Status.DeduplicatedByWE == "" {
            return nil
        }
        return []string{rr.Status.DeduplicatedByWE}
    },
)
```

**M5: `MarkFailedAsDeduplicated` — AtomicStatusUpdate constraint**

Both WE and RO `AtomicStatusUpdate` implementations refetch the object before applying the closure:
```go
// pkg/workflowexecution/status/manager.go:59-76
func (m *Manager) AtomicStatusUpdate(ctx, wfe, updateFunc) error {
    return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
        m.client.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)  // REFETCH
        updateFunc()  // Apply changes to refetched object
        m.client.Status().Update(ctx, wfe)  // Write
    })
}
```

This means `DeduplicatedBy` CANNOT be set before calling `MarkFailedWithReason` — the refetch would overwrite it. New method:
```go
func (r *WorkflowExecutionReconciler) MarkFailedAsDeduplicated(
    ctx context.Context,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
    originalWFE string,
) error {
    // Reuse MarkFailedWithReason logic but set DeduplicatedBy inside closure
    // ... (same as MarkFailedWithReason with reason="Deduplicated") ...
    // Inside AtomicStatusUpdate closure:
    //   wfe.Status.DeduplicatedBy = originalWFE  // <-- MUST be inside closure
    //   wfe.Status.FailureDetails = ... reason: "Deduplicated" ...
}
```

**M4: Terminal-phase predicate for cross-WE watch**

No existing cross-object watches in codebase. Predicate pattern using `predicate.Funcs`:
```go
func (r *Reconciler) wfeTerminalPhasePredicate() predicate.Predicate {
    return predicate.Funcs{
        UpdateFunc: func(e event.UpdateEvent) bool {
            newWFE, ok := e.ObjectNew.(*workflowexecutionv1.WorkflowExecution)
            if !ok { return false }
            return newWFE.Status.Phase == workflowexecutionv1.PhaseCompleted ||
                   newWFE.Status.Phase == workflowexecutionv1.PhaseFailed
        },
        CreateFunc:  func(e event.CreateEvent) bool { return false },
        DeleteFunc:  func(e event.DeleteEvent) bool { return true },
        GenericFunc: func(e event.GenericEvent) bool { return false },
    }
}
```

`DeleteFunc` returns `true` to handle R1 (original WFE deleted — triggers reconcile on dangling reference).

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (WE collision classification, RO handler branching, consecutive failure exclusion, CRD constants)
- **Integration**: >=80% of integration-testable code (reconciler watches, field index, cross-WE result propagation, dangling reference detection)
- **E2E**: Deferred — requires multi-WFE Kind cluster scenario; covered at IT level with envtest

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers (UT + IT):
- **Unit tests**: Isolated collision classification, handler branching, consecutive failure counting
- **Integration tests**: Full reconciliation lifecycle with envtest, cross-WE watch triggering, field index queries

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes:
- "Operator sees `Deduplicated` failure reason instead of `Unknown`"
- "Deduplicated RR automatically inherits the outcome of the original WFE"
- "Consecutive failure counter is not polluted by inherited failures"
- "If original WFE is deleted, the RR does not hang — it fails with a clear message"

### 5.4 Pass/Fail Criteria

> **IEEE 829 §9**

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions approved by reviewer
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing test suites that interact with WE or RO controllers
5. `FailureReasonDeduplicated` WFEs correctly propagate through the full RO lifecycle

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing tests that were passing before the change now fail (regression)
4. Inherited failures counted toward consecutive blocking (BR-ORCH-042 violation)

### 5.5 Suspension & Resumption Criteria

> **IEEE 829 §10**

**Suspend testing when**:

- WE or RR CRD type changes do not compile (`go build ./...` fails)
- envtest infrastructure cannot start (API server unavailable)
- Cascading failures: more than 3 tests fail for the same root cause

**Resume testing when**:

- CRD types compile and `make generate` succeeds
- envtest infrastructure restored
- Root cause identified and fix deployed

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `api/workflowexecution/v1alpha1/workflowexecution_types.go` | `FailureReasonDeduplicated` constant, `DeduplicatedBy` field | ~5 |
| `api/remediation/v1alpha1/remediationrequest_types.go` | `FailurePhaseDeduplicated` constant, `DeduplicatedByWE` field | ~5 |
| `internal/controller/remediationorchestrator/blocking.go` | `countConsecutiveFailures` (dedup exclusion branch) | ~10 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `reconcilePending` (lines 513-529 dedup branch), `handleJobAlreadyExists` (extended ~60), `HandleAlreadyExists` (~71), `MarkFailedWithReason` (~99) | ~290 |
| `pkg/remediationorchestrator/handler/workflowexecution.go` | `HandleStatus` (new dedup branch, first-encounter path) | ~20 |
| `internal/controller/remediationorchestrator/reconciler.go` | `handleExecutingPhase` (C3 short-circuit + propagation ~40), `transitionToInheritedCompleted` (~30), `transitionToInheritedFailed` (~30), cross-WE watch, field index | ~140 |
| `internal/controller/remediationorchestrator/reconciler.go` | `SetupWithManager` (new watch + index) | ~25 |
| `internal/controller/remediationorchestrator/blocking.go` | `countConsecutiveFailures` (nested dedup exclusion) | ~5 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.4` HEAD | Branch |
| Dependency: #265 | Merged | RetentionExpiryTime / CompletedAt fixes |
| Dependency: #612 | Merged | Skip handler CompletedAt |
| Out of scope: #614 | Open | RO-level DuplicateInProgress inheritance |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WE-009 | Prevent parallel execution on same target | P0 | Unit | UT-WE-190-001 | Pending |
| BR-WE-009 | Prevent parallel execution on same target | P0 | Unit | UT-WE-190-002 | Pending |
| BR-WE-009 | Prevent parallel execution — label fallback | P1 | Unit | UT-WE-190-003 | Pending |
| BR-WE-009 | CRD enum accepts Deduplicated | P0 | Unit | UT-WE-190-004 | Pending |
| BR-WE-009 | Tekton collision → Deduplicated | P0 | Unit | UT-WE-190-005 | Pending |
| BR-WE-009 | Tekton — own PipelineRun (no dedup) | P0 | Unit | UT-WE-190-006 | Pending |
| BR-WE-009 | Tekton — label missing fallback | P1 | Unit | UT-WE-190-007 | Pending |
| BR-WE-009 | DeduplicatedBy field populated | P0 | Unit | UT-WE-190-008 | Pending |
| BR-ORCH-025 | RO handler branches on Deduplicated | P0 | Unit | UT-RO-190-001 | Pending |
| BR-ORCH-025 | RO handler sets DeduplicatedByWE | P0 | Unit | UT-RO-190-002 | Pending |
| BR-ORCH-025 | RO handler keeps PhaseExecuting (not Failed) | P0 | Unit | UT-RO-190-003 | Pending |
| BR-ORCH-025 | RO handler non-dedup WFE failure still transitions to Failed | P0 | Unit | UT-RO-190-004 | Pending |
| BR-ORCH-025 | Result inheritance: original WFE Completed → RR Completed | P0 | Unit | UT-RO-190-005 | Pending |
| BR-ORCH-025 | Result inheritance: original WFE Failed → RR Failed (FailurePhaseDeduplicated) | P0 | Unit | UT-RO-190-006 | Pending |
| BR-ORCH-025 | Dangling reference: original WFE deleted → RR Failed | P0 | Unit | UT-RO-190-011 | Pending |
| BR-ORCH-025 | Notification includes provenance for inherited outcomes | P1 | Unit | UT-RO-190-015 | Pending |
| BR-ORCH-042 | Inherited failures excluded from consecutive count | P0 | Unit | UT-RO-190-013 | Pending |
| BR-ORCH-042 | Non-dedup failures still counted | P0 | Unit | UT-RO-190-014 | Pending |
| BR-ORCH-025 | Idempotency: repeated reconcile on dedup-waiting RR is stable | P0 | Unit | UT-RO-190-016 | Pending |
| BR-WE-009 | Job collision full lifecycle (envtest) | P0 | Integration | IT-WE-190-001 | Pending |
| BR-WE-009 | Tekton collision full lifecycle (envtest) | P0 | Integration | IT-WE-190-002 | Pending |
| BR-ORCH-025 | RO dedup + result inheritance lifecycle | P0 | Integration | IT-RO-190-001 | Pending |
| BR-ORCH-025 | Cross-WE watch triggers reconciliation | P0 | Integration | IT-RO-190-002 | Pending |
| BR-ORCH-025 | Field index query for DeduplicatedByWE | P0 | Integration | IT-RO-190-003 | Pending |
| BR-ORCH-025 | Dangling reference lifecycle | P0 | Integration | IT-RO-190-004 | Pending |
| BR-ORCH-042 | Consecutive failure exclusion lifecycle | P0 | Integration | IT-RO-190-005 | Pending |
| BR-ORCH-025 | Watch predicate limits reconcile scope | P1 | Integration | IT-RO-190-006 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

> **IEEE 829 Test Design Specification** — How test cases are organized and grouped.

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `WE` (WorkflowExecution), `RO` (RemediationOrchestrator)
- **BR_NUMBER**: 190
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: WE collision classification, RR/WE CRD constants, RO handler branching, consecutive failure exclusion. Target: >=80% of new unit-testable code.

#### WE Controller Unit Tests

| ID | Business Outcome Under Test | Target Function | Phase |
|----|----------------------------|-----------------|-------|
| `UT-WE-190-001` | Job collision with valid lock (running Job from another WFE) → FailureReasonDeduplicated + DeduplicatedBy set | `reconcilePending` (lines 513-529, via extended `handleJobAlreadyExists`) | Pending |
| `UT-WE-190-002` | Job collision with completed/stale Job → cleanup + retry (existing behavior, no dedup) — regression guard | `handleJobAlreadyExists` | Pending |
| `UT-WE-190-003` | Job collision with valid lock but missing WFE label → FailureReasonUnknown fallback | `reconcilePending` (fallback at line 526) | Pending |
| `UT-WE-190-004` | FailureReasonDeduplicated constant exists and is valid kubebuilder enum value | CRD types | Pending |
| `UT-WE-190-005` | Tekton PipelineRun collision from another WFE → FailureReasonDeduplicated + DeduplicatedBy | `HandleAlreadyExists` (line 1197) | Pending |
| `UT-WE-190-006` | Tekton PipelineRun is ours (self-race) → continue with Running phase (no dedup) — regression guard | `HandleAlreadyExists` (line 1147-1186) | Pending |
| `UT-WE-190-007` | Tekton PipelineRun from another WFE but missing label → FailureReasonUnknown fallback | `HandleAlreadyExists` | Pending |
| `UT-WE-190-008` | DeduplicatedBy field is populated with the name of the original WFE from execution resource labels | `reconcilePending` (Job) + `HandleAlreadyExists` (Tekton) | Pending |

#### RO Handler & Reconciler Unit Tests

| ID | Business Outcome Under Test | Target Function | Phase |
|----|----------------------------|-----------------|-------|
| `UT-RO-190-001` | HandleStatus: WFE with FailureReasonDeduplicated does NOT call transitionToFailed | `HandleStatus` (new dedup branch) | Pending |
| `UT-RO-190-002` | HandleStatus: WFE with FailureReasonDeduplicated sets rr.Status.DeduplicatedByWE from WFE.Status.DeduplicatedBy | `HandleStatus` (new dedup branch) | Pending |
| `UT-RO-190-003` | HandleStatus: WFE with FailureReasonDeduplicated keeps RR in PhaseExecuting (not Failed) | `HandleStatus` (new dedup branch) | Pending |
| `UT-RO-190-004` | HandleStatus: WFE with non-dedup failure reason still calls transitionToFailed (regression guard) | `HandleStatus` (existing PhaseFailed branch) | Pending |
| `UT-RO-190-005` | Result inheritance: original WFE Completed → deduplicated RR transitions via transitionToInheritedCompleted | `handleExecutingPhase` (C3 short-circuit when DeduplicatedByWE is set) | Pending |
| `UT-RO-190-006` | Result inheritance: original WFE Failed → deduplicated RR transitions via transitionToInheritedFailed with FailurePhaseDeduplicated | `handleExecutingPhase` (C3 short-circuit) + `transitionToInheritedFailed` | Pending |
| `UT-RO-190-011` | Dangling reference: original WFE not found → transitionToInheritedFailed with FailurePhaseDeduplicated + clear message | `handleExecutingPhase` (C3 short-circuit, NotFound branch) | Pending |
| `UT-RO-190-012` | Deduplicated RR with DeduplicatedByWE set but original WFE still Running → requeue (no transition) | `handleExecutingPhase` (C3 short-circuit, non-terminal branch) | Pending |
| `UT-RO-190-013` | countConsecutiveFailures: RR with FailurePhaseDeduplicated (nested check inside `case phase.Failed`) is NOT counted | `countConsecutiveFailures` (new nested FailurePhase check at R10 granularity) | Pending |
| `UT-RO-190-014` | countConsecutiveFailures: RR with FailurePhaseWorkflowExecution IS still counted (regression guard) | `countConsecutiveFailures` | Pending |
| `UT-RO-190-015` | Notification for inherited outcome includes provenance referencing original WFE | `transitionToInheritedCompleted` / `transitionToInheritedFailed` | Pending |
| `UT-RO-190-016` | Idempotency: RR with DeduplicatedByWE already set, own WFE still Failed/Deduplicated → no duplicate events, no re-set, stable requeue | `handleExecutingPhase` (C3 short-circuit, idempotency guard) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: WE reconciler with envtest (Job/Tekton collision), RO reconciler with envtest (cross-WE watch, field index, result propagation). Target: >=80% of new integration-testable code.

#### WE Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-WE-190-001` | Full lifecycle: WFE created → Job AlreadyExists (running, another WFE) → WFE marked Failed/Deduplicated + DeduplicatedBy set | Pending |
| `IT-WE-190-002` | Full lifecycle: WFE created → PipelineRun AlreadyExists (another WFE) → WFE marked Failed/Deduplicated + DeduplicatedBy set | Pending |

#### RO Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-190-001` | Full lifecycle: RR in Executing → WFE reaches Failed/Deduplicated → RR stays Executing with DeduplicatedByWE → original WFE completes → RR inherits Completed | Pending |
| `IT-RO-190-002` | Cross-WE watch: updating non-owned WFE to terminal phase triggers reconcile on RR with matching DeduplicatedByWE | Pending |
| `IT-RO-190-003` | Field index: listing RRs by DeduplicatedByWE returns only matching RRs | Pending |
| `IT-RO-190-004` | Dangling reference: original WFE deleted → next reconcile transitions deduplicated RR to Failed | Pending |
| `IT-RO-190-005` | Consecutive failure exclusion: 3 RRs with FailurePhaseDeduplicated + 1 with FailurePhaseWorkflowExecution → count = 1 (not 4) | Pending |
| `IT-RO-190-006` | Watch predicate: non-terminal WFE status changes do NOT trigger reconcile on unrelated RRs | Pending |

### Tier Skip Rationale

- **E2E**: Deferred to a future iteration. The multi-WFE collision scenario requires orchestrating two concurrent WFEs in a Kind cluster targeting the same resource, which is complex infrastructure. The IT tier with envtest provides equivalent behavioral coverage for all critical paths. E2E will be added when the v1.4 E2E infrastructure supports concurrent WFE injection.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-WE-190-001: Job collision with valid lock sets FailureReasonDeduplicated

**BR**: BR-WE-009
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Preconditions**:
- WFE exists in `Pending` phase targeting `default/deployment/my-app`
- A running Job exists with name `wfe-<hash>` and label `kubernaut.ai/workflow-execution: original-wfe`
- Job is NOT completed (valid lock)

**Test Steps**:
1. **Given**: A WFE targeting `default/deployment/my-app` and a running Job owned by `original-wfe`
2. **When**: `reconcilePending` is called; `exec.Create` returns `AlreadyExists`; `handleJobAlreadyExists` detects running Job with label → returns 4th value `originalWFE = "original-wfe"`; caller branches to `MarkFailedAsDeduplicated(ctx, wfe, "original-wfe")` (M5 pattern — `DeduplicatedBy` set inside `AtomicStatusUpdate` closure)
3. **Then**: WFE is marked Failed with Deduplicated reason and DeduplicatedBy set atomically

**Expected Results**:
1. WFE transitions to `Failed` with `FailureDetails.Reason = "Deduplicated"`
2. `WFE.Status.DeduplicatedBy = "original-wfe"`
3. `FailureDetails.WasExecutionFailure = false` (pre-execution)

**Acceptance Criteria**:
- **Behavior**: WFE is marked as deduplicated, not unknown
- **Correctness**: DeduplicatedBy matches the WFE name from the Job label
- **Accuracy**: FailureDetails populated with correct reason and timestamp

### UT-WE-190-003: Job collision with missing label falls back to Unknown

**BR**: BR-WE-009
**Priority**: P1
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Preconditions**:
- WFE exists in `Pending` phase
- A running Job exists with name `wfe-<hash>` but NO `kubernaut.ai/workflow-execution` label

**Test Steps**:
1. **Given**: A WFE and a running Job without WFE ownership label
2. **When**: `reconcilePending` is called; `exec.Create` returns `AlreadyExists`; `handleJobAlreadyExists` detects running Job but label is missing → returns 4th value `originalWFE = ""`; caller falls through to existing `MarkFailedWithReason("Unknown", ...)`
3. **Then**: Falls back to `FailureReasonUnknown` (cannot identify original WFE)

**Expected Results**:
1. WFE transitions to `Failed` with `FailureDetails.Reason = "Unknown"`
2. `DeduplicatedBy` is empty
3. Message indicates target resource is locked

**Acceptance Criteria**:
- **Behavior**: Graceful degradation when label metadata is absent
- **Correctness**: Does not crash or panic on missing labels

### UT-WE-190-005: Tekton collision from another WFE sets FailureReasonDeduplicated

**BR**: BR-WE-009
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Preconditions**:
- WFE exists in `Pending` phase
- A PipelineRun exists with labels `kubernaut.ai/workflow-execution: other-wfe` and `kubernaut.ai/source-namespace: default` (not matching this WFE)

**Test Steps**:
1. **Given**: A WFE and a PipelineRun owned by `other-wfe`
2. **When**: `HandleAlreadyExists` is called; detects another WFE owns the PipelineRun → calls `MarkFailedAsDeduplicated(ctx, wfe, "other-wfe")` (M5 pattern)
3. **Then**: WFE is marked Failed with Deduplicated reason and DeduplicatedBy set atomically

**Expected Results**:
1. WFE transitions to `Failed` with `FailureDetails.Reason = "Deduplicated"`
2. `WFE.Status.DeduplicatedBy = "other-wfe"` (set inside `AtomicStatusUpdate` closure)

**Acceptance Criteria**:
- **Behavior**: Tekton path matches Job path semantics
- **Correctness**: DeduplicatedBy matches the WFE name from PipelineRun label
- **Atomicity**: DeduplicatedBy and FailureDetails written in single API call

### UT-RO-190-001: HandleStatus branches on Deduplicated — no transitionToFailed

**BR**: BR-ORCH-025
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

**Preconditions**:
- RR in `PhaseExecuting` with valid `WorkflowExecutionRef`, `DeduplicatedByWE` NOT yet set
- WFE in `Failed` phase with `FailureDetails.Reason = "Deduplicated"` and `DeduplicatedBy = "original-wfe"`

**Test Steps**:
1. **Given**: RR in Executing (first encounter with dedup WFE), WFE in Failed/Deduplicated
2. **When**: `handleExecutingPhase` → `HandleStatus` is called (DeduplicatedByWE not yet set, so normal flow reaches HandleStatus)
3. **Then**: HandleStatus detects dedup, sets `DeduplicatedByWE`, does NOT call `transitionToFailed`, returns requeue

**Expected Results**:
1. `rr.Status.OverallPhase` remains `Executing`
2. `rr.Status.DeduplicatedByWE = "original-wfe"`
3. Result includes `RequeueAfter > 0` (waiting for original WFE)

**Acceptance Criteria**:
- **Behavior**: Deduplicated failures do not terminate the RR
- **Correctness**: DeduplicatedByWE is set from WFE.Status.DeduplicatedBy
- **Flow**: On the next reconcile, `handleExecutingPhase` will short-circuit to C3 propagation path because `DeduplicatedByWE` is now set

### UT-RO-190-005: Result inheritance — original WFE Completed → RR Completed

**BR**: BR-ORCH-025
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

**Preconditions**:
- RR in `PhaseExecuting` with `DeduplicatedByWE = "original-wfe"`
- Original WFE `original-wfe` in `Completed` phase

**Test Steps**:
1. **Given**: RR waiting for original WFE, original WFE reaches Completed
2. **When**: `handleExecutingPhase` detects `DeduplicatedByWE` is set (C3 short-circuit), fetches original WFE, sees `Completed`
3. **Then**: Calls `transitionToInheritedCompleted(ctx, rr, originalWFE)` (C4 helper)

**Expected Results**:
1. RR transitions out of `Executing` toward completion (Verifying or Completed per existing flow)
2. No `FailurePhaseDeduplicated` set (success path)
3. Notification includes provenance: "Resolved via original-wfe"
4. `CompletedAt` is set

**Acceptance Criteria**:
- **Behavior**: Deduplicated RR inherits success from original WFE
- **Correctness**: `transitionToInheritedCompleted` routes through existing terminal transition logic with provenance

### UT-RO-190-006: Result inheritance — original WFE Failed → RR Failed with FailurePhaseDeduplicated

**BR**: BR-ORCH-025
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

**Preconditions**:
- RR in `PhaseExecuting` with `DeduplicatedByWE = "original-wfe"`
- Original WFE `original-wfe` in `Failed` phase

**Test Steps**:
1. **Given**: RR waiting for original WFE, original WFE reaches Failed
2. **When**: `handleExecutingPhase` detects `DeduplicatedByWE` is set (C3 short-circuit), fetches original WFE, sees `Failed`
3. **Then**: Calls `transitionToInheritedFailed(ctx, rr, originalWFE)` (C4 helper)

**Expected Results**:
1. `rr.Status.OverallPhase = Failed`
2. `rr.Status.FailurePhase = "Deduplicated"`
3. `rr.Status.CompletedAt` is set
4. Notification includes provenance and inherited failure details

**Acceptance Criteria**:
- **Behavior**: Inherited failure is distinguishable from direct failure
- **Correctness**: FailurePhaseDeduplicated (not WorkflowExecution)
- **Flow**: `transitionToInheritedFailed` calls through to existing terminal logic with provenance metadata

### UT-RO-190-011: Dangling reference — original WFE deleted → RR Failed

**BR**: BR-ORCH-025
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

**Preconditions**:
- RR in `PhaseExecuting` with `DeduplicatedByWE = "original-wfe"`
- WFE `original-wfe` does NOT exist (deleted)

**Test Steps**:
1. **Given**: RR waiting for original WFE, original WFE has been deleted
2. **When**: `handleExecutingPhase` detects `DeduplicatedByWE` is set (C3 short-circuit), attempts to fetch original WFE, gets `NotFound`
3. **Then**: Calls `transitionToInheritedFailed(ctx, rr, nil)` with clear message referencing "original-wfe"

**Expected Results**:
1. `rr.Status.OverallPhase = Failed`
2. `rr.Status.FailurePhase = "Deduplicated"`
3. Error message mentions "original-wfe" and indicates it was deleted/not found
4. `CompletedAt` is set

**Acceptance Criteria**:
- **Behavior**: RR does not hang when original WFE is gone
- **Correctness**: Failure message is actionable for operators

### UT-RO-190-013: Consecutive failure count excludes FailurePhaseDeduplicated

**BR**: BR-ORCH-042
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/blocking_test.go`

**Preconditions**:
- 3 RRs with same signal fingerprint, all `OverallPhase = Failed`:
  - RR1: `FailurePhase = "WorkflowExecution"` (real failure)
  - RR2: `FailurePhase = "Deduplicated"` (inherited failure)
  - RR3: `FailurePhase = "Deduplicated"` (inherited failure)

**Test Steps**:
1. **Given**: 3 failed RRs, 1 real + 2 deduplicated
2. **When**: `countConsecutiveFailures` is called
3. **Then**: Returns 1 (only counts the real failure)

**Implementation Note (R10)**:
The exclusion is a **nested check inside `case phase.Failed`** in `countConsecutiveFailures`. The current code increments the counter for all `Failed` RRs. The change adds an inner check:
```go
case phase.Failed:
    if rr.Status.FailurePhase == remediationv1.FailurePhaseDeduplicated {
        continue // Skip inherited failures
    }
    consecutiveFailures++
```

**Expected Results**:
1. Return value is `1`, not `3`
2. Deduplicated failures are skipped in the counting loop

**Acceptance Criteria**:
- **Behavior**: Inherited failures do not pollute the consecutive failure counter
- **Correctness**: Only direct failures trigger blocking

### UT-RO-190-016: Idempotency — repeated reconcile on dedup-waiting RR is stable

**BR**: BR-ORCH-025
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/dedup_handler_test.go`

**Preconditions**:
- RR in `PhaseExecuting` with `DeduplicatedByWE = "original-wfe"` (already set from a previous reconcile)
- Own WFE in `Failed` phase with `FailureDetails.Reason = "Deduplicated"`
- Original WFE `original-wfe` in `Running` phase (not yet terminal)

**Test Steps**:
1. **Given**: RR already has `DeduplicatedByWE` set, original WFE still running
2. **When**: `handleExecutingPhase` is called (second or Nth reconcile)
3. **Then**: C3 short-circuit fires (DeduplicatedByWE set), fetches original WFE, sees Running → requeue. No duplicate events emitted, no field re-set, no call to `HandleStatus`.

**Expected Results**:
1. `rr.Status.OverallPhase` remains `Executing` (no change)
2. `rr.Status.DeduplicatedByWE` remains `"original-wfe"` (not re-set)
3. No audit events emitted
4. No K8s events emitted
5. Result is `RequeueAfter > 0`
6. `HandleStatus` is NOT called (short-circuit bypasses it)

**Acceptance Criteria**:
- **Behavior**: Repeated reconciles are no-ops aside from requeue
- **Correctness**: No duplicate side effects (events, audit, status writes)
- **Stability**: Consistent RequeueAfter value

**Dependencies**: UT-RO-190-001 (sets DeduplicatedByWE), UT-RO-190-012 (original WFE still running)

---

### IT-RO-190-001: Full dedup + result inheritance lifecycle (envtest)

**BR**: BR-ORCH-025
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/dedup_inheritance_integration_test.go`

**Preconditions**:
- envtest cluster running with CRDs installed
- RO reconciler started with cross-WE watch and DeduplicatedByWE field index

**Test Steps**:
1. **Given**: Create original WFE `wfe-original` in `Running` phase
2. **Given**: Create RR `rr-dedup` in `Executing` with `DeduplicatedByWE = "wfe-original"`
3. **When**: Update `wfe-original` to `Completed`
4. **Then**: `rr-dedup` transitions to `Completed` (or Verifying) via result inheritance

**Expected Results**:
1. `rr-dedup` reaches terminal `Completed` phase
2. Notification includes provenance
3. `CompletedAt` is set on `rr-dedup`

**Acceptance Criteria**:
- **Behavior**: Full end-to-end result inheritance works in envtest
- **Correctness**: Cross-WE watch correctly triggers RR reconciliation

### IT-RO-190-004: Dangling reference lifecycle (envtest)

**BR**: BR-ORCH-025
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/dedup_inheritance_integration_test.go`

**Preconditions**:
- envtest cluster with CRDs installed

**Test Steps**:
1. **Given**: Create RR `rr-dangling` in `Executing` with `DeduplicatedByWE = "wfe-deleted"`
2. **Given**: Do NOT create `wfe-deleted` (simulate deletion)
3. **When**: Reconciler processes `rr-dangling`
4. **Then**: `rr-dangling` transitions to `Failed` with `FailurePhaseDeduplicated`

**Expected Results**:
1. `rr-dangling.Status.OverallPhase = Failed`
2. `rr-dangling.Status.FailurePhase = "Deduplicated"`
3. Error message indicates the original WFE was not found

### IT-RO-190-005: Consecutive failure exclusion lifecycle (envtest)

**BR**: BR-ORCH-042
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/dedup_inheritance_integration_test.go`

**Preconditions**:
- envtest cluster with CRDs and field indexes

**Test Steps**:
1. **Given**: Create 3 terminal RRs with same fingerprint: 2 Failed/Deduplicated + 1 Failed/WorkflowExecution
2. **When**: A new RR arrives for same fingerprint and `countConsecutiveFailures` is invoked
3. **Then**: Count is 1 (only the real failure), so blocking threshold (3) is not reached

**Expected Results**:
1. New RR is NOT blocked (count < threshold)
2. Deduplicated RRs correctly excluded from count

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `fake.NewClientBuilder()` for K8s API; `MockAuditStore` for audit; `record.NewFakeRecorder` for events
- **Location**: `test/unit/workflowexecution/`, `test/unit/remediationorchestrator/controller/`, `test/unit/remediationorchestrator/`
- **Resources**: Minimal (no cluster, no I/O)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see No-Mocks Policy)
- **Infrastructure**: envtest (embedded etcd + API server) with WE + RR CRDs installed
- **Location**: `test/integration/workflowexecution/`, `test/integration/remediationorchestrator/`
- **Resources**: ~512MB RAM for envtest

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-runtime | v0.18+ | envtest framework |
| kubebuilder | v4.x | CRD generation |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| #265 | Code | Merged | RetentionExpiryTime / CompletedAt — no impact | N/A |
| #612 | Code | Merged | Skip handler CompletedAt — no impact | N/A |
| CRD regeneration | Infra | Required | New enum values/fields not available to tests | Run `make generate` before tests |

### 11.2 Execution Order

1. **Phase 1 — CRD types** (RED→GREEN): Add `FailureReasonDeduplicated`, `DeduplicatedBy` to WE CRD. Add `FailurePhaseDeduplicated`, `DeduplicatedByWE` to RR CRD. Update kubebuilder enums. Run `make generate`. Unit tests for constant existence (UT-WE-190-004).
2. **Phase 2 — WE collision classification** (RED→GREEN→REFACTOR): Extend `handleJobAlreadyExists` to return original WFE name from Job labels when lock is valid. Update `reconcilePending` call site (line 526) to branch on returned info → `MarkFailedWithReason("Deduplicated")`. Update `HandleAlreadyExists` (line 1197) similarly. Set `DeduplicatedBy` on WFE status. Unit tests (UT-WE-190-001 to 008) + integration tests (IT-WE-190-001, 002).
3. **Phase 3 — RO handler branching** (RED→GREEN→REFACTOR): Add dedup branch to `HandleStatus` for first-encounter path (sets `DeduplicatedByWE`, keeps `PhaseExecuting`, requeues). Unit tests (UT-RO-190-001 to 004).
4. **Phase 4 — RO cross-WE watch + result propagation** (RED→GREEN→REFACTOR): Add C3 short-circuit in `handleExecutingPhase` (when `DeduplicatedByWE` is set, bypass `HandleStatus`). Implement `transitionToInheritedCompleted`/`transitionToInheritedFailed` (C4 helpers). Add `Watches(&WFE{})` with terminal-phase predicate. Add field index on `status.deduplicatedByWE`. Handle dangling reference (NotFound → `transitionToInheritedFailed`). Idempotency guard for repeated reconciles. Unit tests (UT-RO-190-005, 006, 011, 012, 016) + integration tests (IT-RO-190-001 to 004, 006).
5. **Phase 5 — Consecutive failure exclusion** (RED→GREEN→REFACTOR): Add nested check in `countConsecutiveFailures` `case phase.Failed` to skip `FailurePhaseDeduplicated`. Unit tests (UT-RO-190-013, 014) + integration test (IT-RO-190-005).
6. **Phase 6 — Notification provenance** (RED→GREEN): Add provenance message ("Resolved via [original WFE]") in `transitionToInheritedCompleted`/`transitionToInheritedFailed`. Unit test (UT-RO-190-015).
7. **Phase 7 — Documentation**: Update DD-WE-003 (add Deduplicated collision semantics). Update DD-RO-002-ADDENDUM (add cross-WE result inheritance pattern). Update CRD field inline docs.

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/190/TEST_PLAN.md` | Strategy and test design |
| WE unit test suite | `test/unit/workflowexecution/controller_test.go` | Dedup collision classification tests (UT-WE-190-*) |
| RO unit test suite (handler + propagation) | `test/unit/remediationorchestrator/controller/dedup_handler_test.go` | Handler branching, result inheritance, idempotency (UT-RO-190-001 to 016) |
| RO unit test suite (blocking) | `test/unit/remediationorchestrator/blocking_test.go` | Consecutive failure exclusion (UT-RO-190-013, 014) |
| WE integration test suite | `test/integration/workflowexecution/conflict_test.go` | Full collision lifecycle (IT-WE-190-*) |
| RO integration test suite | `test/integration/remediationorchestrator/dedup_inheritance_integration_test.go` | Full result inheritance + watch + field index lifecycle (IT-RO-190-*) |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests (WE)
go test ./test/unit/workflowexecution/... -ginkgo.v -ginkgo.focus="UT-WE-190"

# Unit tests (RO handler + blocking)
go test ./test/unit/remediationorchestrator/... -ginkgo.v -ginkgo.focus="UT-RO-190"

# Integration tests (WE)
go test ./test/integration/workflowexecution/... -ginkgo.v -ginkgo.focus="IT-WE-190"

# Integration tests (RO)
go test ./test/integration/remediationorchestrator/... -ginkgo.v -ginkgo.focus="IT-RO-190"

# Coverage
go test ./test/unit/workflowexecution/... -coverprofile=we-unit-coverage.out
go test ./test/unit/remediationorchestrator/... -coverprofile=ro-unit-coverage.out
go tool cover -func=we-unit-coverage.out
go tool cover -func=ro-unit-coverage.out
```

---

## 14. Existing Tests Requiring Updates

> When implementation changes behavior that existing tests assert on, document the
> required updates here to prevent surprises during TDD GREEN phase.

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/workflowexecution/controller_test.go` — Job AlreadyExists scenarios | Asserts `FailureDetails.Reason = "Unknown"` for running-Job collision | Update to `"Deduplicated"` and add `DeduplicatedBy` assertion (when label present); keep `"Unknown"` for label-missing case | New collision classification (C1: call site at `reconcilePending` line 526) |
| `test/unit/workflowexecution/controller_test.go` — Tekton AlreadyExists scenarios | Asserts `FailureDetails.Reason = "Unknown"` for cross-WFE collision (line 1197) | Update to `"Deduplicated"` and add `DeduplicatedBy` assertion (when label present); keep `"Unknown"` for label-missing case | New collision classification |
| `test/integration/workflowexecution/conflict_test.go` — conflict lifecycle | May assert `Unknown` reason for AlreadyExists conflicts | Update to `"Deduplicated"` where appropriate | Existing conflict tests cover this path |
| `test/unit/remediationorchestrator/controller/reconcile_phases_test.go` | HandleStatus WFE-Failed scenarios | May need new scenario for `Deduplicated` reason to verify non-transition | New handler branch |
| `test/unit/remediationorchestrator/blocking_test.go` | `countConsecutiveFailures` scenarios | Add `FailurePhaseDeduplicated` RR to existing test data; verify it is skipped (nested check inside `case phase.Failed`) | Exclusion logic (R10 granularity) |
| `test/unit/remediationorchestrator/controller/controller_test.go` | `handleExecutingPhase` scenarios | May need update if any existing test sets `DeduplicatedByWE` (unlikely) or if the C3 short-circuit changes return values for existing scenarios | C3 short-circuit insertion |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
| 2.0 | 2026-03-04 | Post-assessment revision: Fixed Job-path call site targets (C1/G2). Added C3/C4 decisions. Added UT-RO-190-016 idempotency test. Added risks R8-R10. |
| 3.0 | 2026-03-04 | Risk mitigation: M1 — concrete `handleJobAlreadyExists` 4th return value design verified. M2 — `Watches` + `EnqueueRequestsFromMapFunc` + `WithPredicates` API confirmed in controller-runtime v0.23.3 (first cross-object watch in codebase). M3 — status field index pattern verified (same extractor as spec fields). M4 — terminal-phase predicate with `DeleteFunc=true` for dangling reference. M5 — **CRITICAL**: `AtomicStatusUpdate` refetches WFE before closure, so `DeduplicatedBy` must be set inside closure → new `MarkFailedAsDeduplicated` method required (cannot reuse `MarkFailedWithReason`). Added Section 4.4 (Verified Implementation Patterns) with concrete code patterns for all 5 mitigations. |
