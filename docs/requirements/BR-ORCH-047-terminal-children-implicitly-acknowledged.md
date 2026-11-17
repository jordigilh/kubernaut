# BR-ORCH-047: Terminal Children Are Implicitly Acknowledged for Cancellation

**Status:** Active
**Introduced:** 2026-11-16 (issue #2588, PR #2643)
**Controls:** SOC2 CC7.2 (system monitoring / safe recovery); FedRAMP AU-2 (observable cancellation lifecycle)

## Requirement

When the RemediationOrchestrator coordinates RemediationRequest cancellation,
a child resource (SignalProcessing, AIAnalysis, WorkflowExecution,
EffectivenessAssessment) whose business phase is already terminal
(Completed, Failed, or Cancelled) MUST be treated as implicitly
acknowledged: it has no remaining work, so it never sets the
`CancellationAcknowledged` condition and must not block the parent's
cancellation completion or finalizer release.

## Rationale

A child that reached a terminal business outcome finished normally and never
enters its cancellation path, so it never emits the acknowledgement the
coordinator waits for. CI run 43971993157 proved the previous behavior hangs
`kubectl delete rr` indefinitely behind the retained cancellation finalizer
when any referenced child is terminal — user-visible product behavior, not a
test artifact.

## Boundaries

- Identity validation MUST still fail closed before the terminal fast-path.
- Non-terminal children MUST still provide explicit
  `CancellationAcknowledged` evidence, including verified backend stop.
- The bounded retry budget and finalizer retention for genuinely stuck,
  non-terminal children are unchanged.
- A terminal WorkflowExecution acknowledgement MUST NOT claim backend
  termination verification (`backendVerified=false`): no stop was requested
  because the backend already finished.

## Acceptance

- UT-RO-2588-014 proves every child kind's terminal phases are implicitly
  acknowledged; the non-terminal half (children held until explicit
  `CancellationAcknowledged`, finalizer retained) is proven by
  IT-ORCH-2588-003 and the existing condition-based child tests.
- Deployed fullpipeline E2E proves a deleting parent whose children are
  terminal completes and releases its finalizer within the job budget.
