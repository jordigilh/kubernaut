# BR-ORCH-048: Bounded Cancellation Waits

**Status:** Active
**Introduced:** 2026-11-17 (issue #2588, PR #2643)
**Controls:** SOC2 CC7.2 (safe recovery, observable escalation); FedRAMP AU-2/AU-3 (attributable cancellation lifecycle)

## Requirement

A RemediationRequest cancellation MUST NOT wait silently forever. When a
cancellation has waited beyond a wall-clock budget (10 minutes from
`status.cancellation.requestedAt`) without completing — children
acknowledging neither success nor failure, or the cancellation notification
never reaching a terminal delivery phase — the orchestrator MUST escalate:
record the terminal `Failed` cancellation state, emit a distinct
`cancellation.wait.exhausted` audit event, publish manual-review evidence,
and for an already-deleting RR release the cancellation finalizer so the
pending deletion completes.

## Rationale

The retry budget deliberately counts only observable failures — pending
child stops are free (UT-ORCH-2588-028) because legitimate child shutdowns
are paced by each child's own bounded retries. But a wait that never
terminates (a child controller that acks neither, a notification stuck in
delivery) requeues forever and hangs `kubectl delete rr` behind the retained
finalizer. CI runs 43971993157 and 44009101345 each consumed an entire
45-minute job budget this way.

## Boundaries

- The wall-clock bound MUST NOT increment or consume the failure retry
  budget; the pending-is-free contract is unchanged.
- The escalation MUST fire at most once: a `Failed` cancellation state
  guards re-entry, so subsequent wakes do not re-emit the escalation audit.
- The exhausted status MUST be persisted before any finalizer release, so
  the manual-review evidence survives the deletion.
- The cancel-API path (no deletion) retains the finalizer: watches and
  manual remediation remain its recovery path.
- The wait-exhausted audit MUST use a distinct stage
  (`cancellation.wait.exhausted`) so correlation-based reconstruction
  attributes the wall-clock cause rather than misreporting retry-budget
  exhaustion.

## Acceptance

- UT-RO-2588-015 proves the wall-clock predicate; UT-RO-2588-018 proves
  recent or untimestamped cancellations are not exhausted.
- UT-RO-2588-019 proves the reconcile path escalates exactly once and
  retains the finalizer on the cancel-API path.
- UT-RO-2588-020 proves the reconcile path persists the exhausted status
  before releasing the finalizer for a deleting RR.
- Deployed fullpipeline E2E proves a deleting RR completes within the lane
  budget.
