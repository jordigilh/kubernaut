# DD-WE-009: Terminal Audit Write Ordering (Duplicate Audit Fix)

**Version**: 1.0
**Date**: 2026-07-07
**Status**: APPROVED
**Author**: WorkflowExecution Team
**Reviewers**: Platform Team

---

## Context

`MarkCompleted`, `MarkFailed`, and `markFailedInternal` (the shared core of
`MarkFailedWithReason`/`MarkFailedAsDeduplicated`) each perform a terminal phase
transition via `StatusManager.AtomicStatusUpdate`, which wraps the caller's closure in
`k8sretry.RetryOnConflict`:

```go
func (m *Manager) AtomicStatusUpdate(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, updateFunc func() error) error {
    return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
        if err := m.client.Get(ctx, client.ObjectKeyFromObject(wfe), wfe); err != nil { // refetch
            return fmt.Errorf("failed to refetch WorkflowExecution: %w", err)
        }
        if err := updateFunc(); err != nil { // caller's closure
            return fmt.Errorf("failed to apply status updates: %w", err)
        }
        if err := m.client.Status().Update(ctx, wfe); err != nil { // may return a conflict
            return fmt.Errorf("failed to atomically update status: %w", err)
        }
        return nil
    })
}
```

Prior to this decision, the three transition helpers (`applyCompletedTransition`,
`applyFailedStatusTransition`, and `markFailedInternal`'s inline closure) called the
`AuditManager.RecordWorkflow{Completed,Failed}` audit write and set the `AuditRecorded`
condition **inside** this closure. Because `RetryOnConflict` re-runs the **entire**
closure — refetch, phase transition, and audit call — on every optimistic-lock conflict,
a single logical terminal transition that hit even one conflict produced **N audit
events for N closure executions**, not one. This is [issue #1597](https://github.com/jordigilh/kubernaut/issues/1597),
originally surfaced as a follow-up item from
[DD-WE-008](./DD-WE-008-job-resource-governance-transient-failure-tolerance.md)'s Related
Documents section during its own audit-completeness preflight.

This directly violates BR-AUDIT-005 / SOC2 CC8.1 ("complete remediation request
reconstruction from audit traces alone" — a duplicate `workflow.completed` event makes
the audit trail internally inconsistent, not just noisy) and ADR-032's "No Audit Loss"
mandate (which is about under-recording, but an over-recording duplicate is an equally
real trace-integrity defect).

### Why This Wasn't Already Caught

Every other WorkflowExecution unit/integration test exercises the happy path (a fake
client with no injected conflicts), where `RetryOnConflict`'s closure runs exactly once
and the bug is invisible. The `AuditRecorded` condition's own integration tests
(`test/integration/workflowexecution/conditions_integration_test.go`) assert via
`Eventually(...)` that the condition eventually reaches `True`/`False` — they never
assert *how many times* the underlying audit write occurred, so they pass regardless of
this bug.

### Precedent Elsewhere in the Codebase

AIAnalysis and SignalProcessing controllers already record their terminal audit events
**after** their own `AtomicStatusUpdate` closures return (with idempotency guards
tracked as `AA-BUG-001`/`SP-BUG-ENRICHMENT-001`), establishing "audit outside the
retryable closure" as the codebase convention this decision brings WorkflowExecution
into alignment with.

---

## Decision

Extract the audit-write + `AuditRecorded`-condition-setting logic into a new
`recordTerminalAudit` helper, called by `MarkCompleted`, `MarkFailed`, and
`markFailedInternal` **after** their respective primary `AtomicStatusUpdate` has
durably committed the phase transition — not from inside its closure.

```go
func (r *WorkflowExecutionReconciler) recordTerminalAudit(
    ctx context.Context,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
    eventName string,
    auditFn func(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) error,
    logger logr.Logger,
) {
    succeeded, reason, message := true, weconditions.ReasonAuditSucceeded,
        fmt.Sprintf("Audit event %s recorded to DataStorage", eventName)
    if err := auditFn(ctx, wfe); err != nil {
        succeeded, reason, message = false, weconditions.ReasonAuditFailed,
            fmt.Sprintf("Failed to record audit event: %v", err)
    }
    if err := r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() error {
        weconditions.SetAuditRecorded(wfe, succeeded, reason, message)
        return nil
    }); err != nil {
        logger.Error(err, "Failed to persist AuditRecorded condition", "event", eventName)
    }
}
```

The audit call itself now executes exactly once per logical transition, regardless of
how many conflicts the primary `AtomicStatusUpdate` absorbs. The `AuditRecorded`
condition outcome is then persisted via a **second**, small, idempotent
`AtomicStatusUpdate` call that only touches that one condition — itself safe to retry
on conflict, because setting the same condition value twice is a no-op.

This does **not** conflict with [DD-PERF-001](./DD-PERF-001-atomic-status-updates-mandate.md)'s
atomic-status-update mandate: that mandate targets consolidating multiple **status
field** writes (phase, counters, timestamps, conditions) that change together as part
of one state transition into a single API call. It does not require bundling an
external side effect (a network call to DataStorage) into that same retryable unit —
doing so is precisely what caused this bug, since network calls are not safe to
silently re-execute inside a K8s-API-conflict retry loop.

### Cost

One extra `Status().Update()` API call per terminal transition (for the
`AuditRecorded`-only follow-up), versus the single combined call before this fix. This
is a fixed, small cost (no loop, no per-conflict multiplication) and is the trade-off
this decision makes in exchange for audit-count correctness.

---

## Alternatives Considered

### Option A: Move audit outside the closure (Selected)

Described above. Matches the established AIAnalysis/SignalProcessing precedent.
**Pros**: Audit written exactly once; no schema/API changes; minimal blast radius (one
file). **Cons**: One extra API call per terminal transition; the `AuditRecorded`
condition is no longer set atomically with the phase transition (closes in a
follow-up write instead) — acceptable because existing consumers of that condition
already poll via `Eventually`, not via same-call atomicity.

### Option B: Client-side idempotency key / dedup on the audit event

Generate a stable idempotency key (e.g. hash of `wfe.UID` + phase + generation) and have
DataStorage deduplicate on it server-side.
**Cons**: `event_id` in the audit API is server-generated (`pkg/workflowexecution/audit/manager.go`
never sets it), so there is no existing request field to carry a client-supplied
idempotency key. Implementing this would require an OpenAPI schema change to
`data-storage-v1.yaml`, DataStorage-side dedup logic, and an ogen-client regen — a
cross-service change disproportionate to this bug's scope.
**Decision**: Rejected — same conclusion the DD-WE-008 follow-up note already
anticipated ("needs its own design review"); Option A fixes the bug without touching
DataStorage at all.

### Option C: In-memory guard flag inside the closure

Set a local `bool` before the first audit call and skip re-recording on closure re-runs.
**Cons**: The closure is a fresh function value captured by `RetryOnConflict`, but the
`wfe` object it mutates is **refetched** from the API server at the top of every retry
(`m.client.Get(...)`), which does not carry any local-only guard state — a package-level
or captured-variable guard would work only because the closure itself persists across
retries in the same `RetryOnConflict` call, but this conflates "retry-loop-local
memory" with "durable state," which is fragile and non-obvious to future maintainers
compared to Option A's structural fix. Also does not match the existing
AIAnalysis/SignalProcessing precedent.
**Decision**: Rejected in favor of the clearer, precedent-aligned Option A.

---

## Validation (Spike Findings)

Two spikes were run against real code (not just static reasoning) before implementation:

1. **Conflict-injection technique**: a `fake.NewClientBuilder()` with
   `WithInterceptorFuncs(interceptor.Funcs{SubResourceUpdate: ...})` that returns exactly
   one `apierrors.NewConflict(...)` on the first `Status().Update()` call, then delegates
   normally. Confirmed this reliably forces `RetryOnConflict` to re-run the closure a
   second time.
2. **Bug reproduction**: run against the pre-fix code, the audit-event count scaled 1:1
   with closure re-executions (2 events for 1 logical completion with one injected
   conflict) — reproducing #1597 exactly.
3. **Fix validation**: run against the `recordTerminalAudit`-based fix, the audit-event
   count stayed at exactly 1 regardless of conflicts injected on either the primary or
   the follow-up `AtomicStatusUpdate` call, with zero regressions across the full
   existing `pkg/workflowexecution/...` unit suite and the
   `test/integration/workflowexecution/...` suite (108/108 passing, including the
   `AuditRecorded`-condition-count assertions in
   `conditions_integration_test.go`/`job_lifecycle_integration_test.go`).

These findings are now permanently captured as regression tests `UT-WE-1597-001`
(`MarkCompleted`), `UT-WE-1597-002` (`MarkFailed`), and `UT-WE-1597-003`
(`MarkFailedWithReason`) in `pkg/workflowexecution/controller_test.go`.

---

## Affected Components

| Component | Change |
|---|---|
| `internal/controller/workflowexecution/workflowexecution_status_marking.go` | New `recordTerminalAudit` helper; `MarkCompleted`/`MarkFailed`/`markFailedInternal` call it after their primary `AtomicStatusUpdate`; audit-recording code removed from `applyCompletedTransition`/`applyFailedStatusTransition`/`markFailedInternal`'s closure |
| `pkg/workflowexecution/controller_test.go` | New `Describe("Issue #1597: ...")` block: `UT-WE-1597-001/002/003` conflict-injection regression tests |

**Explicitly NOT touched**: `pkg/workflowexecution/audit/manager.go` (audit payload
construction unchanged), `pkg/workflowexecution/status/manager.go` (`AtomicStatusUpdate`
itself unchanged — this decision changes how callers use it, not its implementation),
DataStorage OpenAPI schema/ogen-client (Option B rejected).

---

## Consequences

### Positive

1. `workflow.completed`/`workflow.failed` audit events are recorded exactly once per
   logical terminal transition, regardless of K8s API optimistic-lock conflicts —
   closes the BR-AUDIT-005/SOC2 CC8.1 trace-integrity gap.
2. Brings WorkflowExecution's terminal-audit ordering in line with the existing
   AIAnalysis/SignalProcessing precedent, reducing the number of distinct patterns for
   the same architectural concern across controllers.
3. No schema, API, or cross-service changes required.

### Negative

1. One extra `Status().Update()` call per terminal transition (the `AuditRecorded`-only
   follow-up) versus the single combined call before this fix.
2. The `AuditRecorded` condition is no longer guaranteed to reach the API server in the
   same call as the phase transition — a caller reading `wfe.Status` between the two
   writes could observe the terminal phase with `AuditRecorded` still absent/stale.
   Accepted because existing consumers already poll for eventual consistency
   (`Eventually` in integration tests), not same-call atomicity.

### Neutral

1. `DD-PERF-001`'s atomic-status-update mandate is unaffected — this decision only
   changes what happens *outside* that pattern (the audit side effect), not the
   pattern itself.

---

## Related Documents

- [DD-PERF-001: Atomic Status Updates Mandate](./DD-PERF-001-atomic-status-updates-mandate.md) — the pattern this decision does not conflict with (targets status-field consolidation, not audit-call bundling)
- [DD-WE-008: Job Resource Governance and Transient-Failure Tolerance](./DD-WE-008-job-resource-governance-transient-failure-tolerance.md) — originating follow-up note ("`RetryOnConflict`-wraps-audit-call ordering gap") that led to this decision
- BR-AUDIT-005 v2.0 (root audit business requirement, defined in `AGENTS.md`) — audit completeness/reconstruction mandate this decision protects
- [Issue #1597](https://github.com/jordigilh/kubernaut/issues/1597) — originating issue

---

## Document Maintenance

| Date | Version | Changes |
|---|---|---|
| 2026-07-07 | 1.0 | Initial decision and implementation |
