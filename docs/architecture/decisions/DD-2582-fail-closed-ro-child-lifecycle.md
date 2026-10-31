# DD-2582: Fail-Closed Remediation Child Lifecycle

## Status

**✅ Approved for implementation** (2026-10-31)
**Last Reviewed**: 2026-10-31
**Confidence**: 95%

## Context & Problem

Remediation Orchestrator child names are deterministic, but the existing
creators adopt an object found at that name or treat a concurrent
`AlreadyExists` response as idempotent success. A child can therefore perform
consequential work before the RemediationRequest has durably recorded the child
reference and authorized its phase. Informer watches do not provide an atomic
create-plus-parent-status transaction and cannot undo work already dispatched by
privileged actors.

This decision implements issue #2582's approved fail-closed lifecycle without
claiming cross-resource atomic authorization.

## Key Requirements

- CreateOnly semantics for SP, AIAnalysis, RAR, WE, EA, and the first AgentSession.
- Typed collisions fail the RR/AIAnalysis request and never advance parent refs or phases.
- Children perform one fresh parent API read immediately before consequential work.
- Parent failure is handled by the child controller, with observable cancellation and preserved terminal history.
- NotificationRequest remains the narrow BR-ORCH-045.2 idempotent completion/failure-delivery exception.
- InvestigationSession creation precedes RR persistence for new AF requests; a new-session collision fails the request, while explicit takeover/join remains idempotent.

## Alternatives Considered

### Alternative 1: Detection-only watches

RO watches child objects and reports unexpected objects but leaves child
controllers unchanged.

**Rejected**: a watch cannot prevent a child that already dispatched work and
does not protect the create-to-parent-status handoff.

### Alternative 2: Admission or central authorization transaction

Use admission or a central coordinator to make child creation and parent phase
authorization appear atomic.

**Rejected for this issue**: deployment coupling and multi-cluster complexity
increase, and Kubernetes still cannot atomically commit a child create and a
separate parent status update. This remains future work.

### Alternative 3: CreateOnly plus controller-owned fresh parent gates and cancellation

The authorized creator is the only source of child identity. Each child reads
its parent through an uncached reader before side effects, waits through the
legitimate create-before-parent-phase window, and owns cancellation/status
transitions. RO maps failures through explicit parent-reference indexes.

**Approved**: this is the plan-approved balance of provenance, fail-closed
behavior, restart safety, and service ownership.

## Decision

**APPROVED: Alternative 3** — CreateOnly plus controller-owned fresh parent gates and cancellation.

### Ordering contract

1. The authorized controller creates the child.
2. The parent persists the child reference.
3. The parent advances to the phase that authorizes child work.
4. The child performs a fresh parent read immediately before consequential work.

A child observed during steps 1–3 waits. A bounded handoff timeout converts an
unfinished handoff into a fail-closed/cancelled outcome. A retry after a
successful child create but lost parent status write is allowed to surface as a
collision; this is an accepted availability trade-off.

### Identity and collision contract

Owner references and object names are evidence, not provenance. An unexpected
pre-existing child or an `AlreadyExists` result is a typed collision and is
never adopted. NotificationRequest is the only deliberate reuse exception,
and only after exact parent/type identity validation.

### Cancellation contract

RR failure stops active children through their own controllers. SP, AIA, and EA
use explicit `Cancelled` phases. RAR uses `ApprovalDecisionCancelled`. WE uses
`Cancelling` until backend termination is observed for Job/Pods, Tekton
PipelineRun/TaskRuns/Pods, or AWX; a bare status transition is not proof of
termination. AgentSession cancellation uses the real KA session manager and
requires its acknowledgement before the child status is considered stopped.

## Implementation and Wiring

Primary implementation areas are the files listed in the approved issue plan:

- `pkg/shared/parentlifecycle/` for typed fresh-parent reads and gate results.
- `pkg/remediationorchestrator/creator/` for typed CreateOnly collisions.
- AIA/KA, WE executors/controllers, SP/EA/RAR/AF controllers, and RO mapping.
- API Go types, generated deepcopy/CRDs, RBAC markers/manifests, schemes,
  field indexes, and watches.

Key production comments reference **DD-2582** at the shared gate, creator
collision, WE cancellation, and AgentSession dispatch boundaries.

## Consequences

### Positive

- Unexpected deterministic-name children cannot be silently adopted.
- Stale caches, missed watches, and controller restarts do not authorize work.
- Backend stop failures remain visible and retryable instead of being reported as
  successful cancellation.
- Terminal child history is preserved and notification delivery remains active
  after RR failure.

### Negative and mitigations

- A lost parent status write after a successful child Create can require manual
  review on retry. This is documented, audited, and tested as an availability
  trade-off.
- Fresh parent reads add API-server traffic. Field indexes, bounded requeues,
  and one read immediately before side effects limit the cost.
- Watches remain asynchronous and cannot eliminate every gate-to-dispatch race.
  Children re-check before side effects and verify best-effort cancellation.

## Security and Compliance

- FedRAMP AC-4: persisted parent phase is the child information-flow gate.
- FedRAMP AC-6: only required parent-read, child-status, and backend-stop
  permissions are added.
- FedRAMP SI-10: immutable parent references, enum validation, and identity
  checks reject malformed or mismatched links.
- FedRAMP AU-2/AU-3 and BR-AUDIT-005: collisions, gate waits/timeouts,
  cancellation requested/verified/failed, failure propagation, and escalation
  remain attributable to the remediation correlation ID.
- SOC2 CC7.2: the signal-to-notification lifecycle remains reconstructable from
  audit events. CC8.1 is used only for change-management evidence.

## Validation

- BDD unit scenarios cover CreateOnly collisions, fresh gates, cancellation
  proofs, identity recovery, and notification exception behavior.
- Integration scenarios traverse production controller dispatch and dropped or
  restarted watch paths.
- E2E scenarios cover normal ordering, RR failure during AI/WE, all three WE
  backends, IS-before-RR creation, and least-privilege assumptions.
