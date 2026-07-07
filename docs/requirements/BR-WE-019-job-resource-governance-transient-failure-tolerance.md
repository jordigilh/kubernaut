# BR-WE-019: Job Resource Governance and Transient-Failure Tolerance

**Business Requirement ID**: BR-WE-019
**Category**: Workflow Execution Service — Reliability & Platform
**Priority**: **P1 (HIGH)** — closes a production-reliability gap (CI flake root cause)
**Target Version**: V1.6
**Status**: Proposed
**Date**: July 6, 2026
**Related DDs**: DD-WE-008 (Job Resource Governance and Transient-Failure Tolerance)
**Related BRs**: BR-WE-014 (Kubernetes Job Execution Backend), BR-WE-016 (Engine Config Discriminator), BR-WE-018 (Execution Pod Security Hardening), BR-WORKFLOW-008 (Runtime Dependency-Failure Observability)
**GitHub Issues**: [#1564](https://github.com/jordigilh/kubernaut/issues/1564), [#1572](https://github.com/jordigilh/kubernaut/issues/1572)

---

## Business Need

### Problem Statement

`JobExecutor.buildJob()` gives the Kubernetes Job's `"workflow"` container no
`resources.requests`/`resources.limits`, and hardcodes `Job.Spec.BackoffLimit: 0`. As a
result:

1. The container runs at `BestEffort` Quality of Service, the first target the kubelet evicts
   under node memory pressure, with no CPU/memory floor or ceiling.
2. Any pod failure — including a transient, infrastructure-caused one (OOM-kill, node
   eviction/preemption) — permanently fails the `WorkflowExecution` on the first occurrence,
   indistinguishable from a genuine remediation-script failure.

### Impact Without This BR

- Transient infrastructure events (observed in CI, issue #1564) permanently fail remediation
  attempts that had nothing wrong with their own logic, requiring manual re-trigger.
- Workflow authors have no way to size CPU/memory for workloads with different footprints
  (a one-line `kubectl patch` vs. a data-migration script), so every workflow either runs
  under-resourced (`BestEffort`) or authors work around it out-of-band.
- No mechanism exists to tell "the node evicted the pod" apart from "the script actually
  failed" — both currently produce an identical, permanent `WorkflowFailed`.

---

## Business Requirement

**The system MUST allow workflow authors to declare per-workflow CPU/memory
requests/limits for the Kubernetes Job execution engine, and MUST tolerate
infrastructure-caused transient pod failures (OOM-kill, node eviction/preemption) without
weakening today's fail-fast behavior for genuine remediation-script failures.**

### Acceptance Criteria

1. **AC1 — Per-workflow resource declaration**: A workflow schema (`workflow-schema.yaml`)
   MAY declare `execution.resources` (same shape as a Kubernetes container `resources` block)
   when `execution.engine: job`. When declared, the Job's `"workflow"` container carries the
   declared `requests`/`limits`. When absent, behavior is unchanged from today (no resources
   block).
2. **AC2 — Engine-scoping enforcement**: Declaring `execution.resources` for any engine other
   than `job` MUST fail workflow registration with a clear error (no silent no-op).
3. **AC3 — Fail-fast registration-time validation**: An invalid resource quantity, or a
   `requests` value exceeding its corresponding `limits` value for the same resource name,
   MUST fail workflow registration with a `SchemaValidationError` naming the offending field —
   not surface only later as a Job-admission failure on a live cluster.
4. **AC4 — Transient-failure tolerance**: A Job pod that fails due to OOM-kill (container exit
   code 137) or a Kubernetes-initiated disruption (`DisruptionTarget` pod condition — node
   eviction/preemption) MUST NOT count against `Job.Spec.BackoffLimit`, and the Job controller
   creates a replacement pod.
5. **AC5 — Fail-fast preserved for genuine failures**: Any other pod failure (i.e. the
   remediation script itself exits non-zero, or any failure not matching AC4) MUST continue to
   count against `BackoffLimit` exactly as today (default `backoffLimit: 0` — immediate,
   permanent `WorkflowFailed`).
6. **AC6 — Bounded retry tolerance**: The Job's existing `ActiveDeadlineSeconds`
   (BR-WORKFLOW-008) MUST remain the outer wall-clock bound on total execution time regardless
   of how many AC4-tolerated retries occur, so a chronically under-resourced workflow cannot
   retry indefinitely.
7. **AC7 — No behavior change for Tekton/Ansible**: This requirement applies exclusively to
   the `job` execution engine. Tekton `PipelineRun`s and Ansible/AWX executions are unaffected.
8. **AC8 — No fleet-wide default in kubernaut-owned code**: Neither the Helm chart nor the
   `kubernaut-operator` provisions a namespace-level `LimitRange`/`ResourceQuota` for
   `kubernaut-workflows` as part of satisfying this BR — that remains an explicit
   platform-operator decision, documented operationally (not enforced by kubernaut code).
9. **AC9 — Real-cluster proof of transient-failure tolerance**: AC4 and AC5 MUST each have at
   least one E2E test proving the behavior on a real Kubernetes cluster (real kubelet, real
   Job controller), not only structural/envtest verification — since `PodFailurePolicy`
   evaluation is a controller-loop behavior envtest does not run (Pyramid Invariant: "E2E
   proves the journey").
10. **AC10 — Audit-trail retry-count completeness**: The durable audit trail (`event_data` of
    the `workflowexecution.workflow.completed`/`.failed` event) MUST record the number of
    AC4-tolerated pod-failure attempts observed before the WFE reached a terminal phase, so that
    a remediation requiring N tolerated retries is distinguishable from one that succeeded
    cleanly on the first attempt (SOC2 CC8.1/AU-3 content completeness). Root-cause attribution
    per individual attempt is explicitly out of scope for this AC (best-effort only, unchanged
    from today).

---

## Known Limitations (Not Acceptance Criteria of This BR)

- **Root-cause attribution per retry attempt**: AC10 hard-guarantees the retry *count* in the
  durable audit trail, but *why* each individual tolerated attempt failed (OOM-kill vs.
  disruption, specifically) remains best-effort only, via the pre-existing Event-scanning
  pattern (`enrichFailureMessage`) — Pods are typically garbage-collected before `GetStatus`
  observes the terminal Job, so a fully-guaranteed, real-time causal trail per attempt is not
  achievable without a larger, separate design change (its own DD if ever required). This
  scope boundary was set deliberately, not discovered as a gap — see DD-WE-008 Section 9.

  **Correction from this document's v1.1**: the retry-*count* completeness gap was originally
  deferred to a follow-up BR/DD, on the stated grounds that the fix required a "cross-cutting"
  OpenAPI/ogen change shared by every audit-emitting service. On re-investigation, that
  justification did not hold up: the payload schema (`WorkflowExecutionAuditPayload`) is
  exclusively owned by WorkflowExecution's own event types, and the actual effort was
  comparable to this BR's other acceptance criteria. It is now **AC10** above, not deferred.

  Separately, the audit-event *count* invariant (no duplicate/missing completion audit events
  caused by tolerated retries) is a general BR-AUDIT-005 concern, not itself a BR-WE-019
  acceptance criterion — but is proven, as a bundled regression guard, by this BR's own
  implementation (`IT-WE-019-003`, extended E2E test — see DD-WE-008 Section 9 and
  IMPLEMENTATION_PLAN.md Phase 5).

---

## Related Documents

- [DD-WE-008: Job Resource Governance and Transient-Failure Tolerance](../architecture/decisions/DD-WE-008-job-resource-governance-transient-failure-tolerance.md)
- [BR-WE-014: Kubernetes Job Execution Backend](./BR-WE-014-kubernetes-job-execution-backend.md)
- [BR-WE-016: Engine Config Discriminator](./BR-WE-016-engine-config-discriminator.md)
- [BR-WE-018: Execution Pod Security Hardening](./BR-WE-018-execution-pod-security-hardening.md)
- [BR-WORKFLOW-008: Runtime Dependency-Failure Observability](./BR-WORKFLOW-008-runtime-dependency-failure-observability.md)
- [Issue #1564](https://github.com/jordigilh/kubernaut/issues/1564) — originating issue
- [Issue #1572](https://github.com/jordigilh/kubernaut/issues/1572) — implementation tracking

---

**Document Version**: 1.3
**Last Updated**: July 6, 2026
**Maintained By**: WorkflowExecution Team

## Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-07-06 | Initial requirement |
| 1.1 | 2026-07-06 | Added AC9 (E2E real-cluster proof requirement) and "Known Limitations" section documenting the deferred audit-completeness gap |
| 1.2 | 2026-07-06 | Clarified "Known Limitations" to distinguish the deferred audit *content* gap from the audit *count* invariant, which is now covered by a bundled regression guard in this BR's own implementation (cites BR-AUDIT-005, not a new BR-WE-019 AC) |
| 1.3 | 2026-07-06 | **Reversed the v1.1 deferral**: added **AC10** (audit-trail retry-count completeness) after re-investigation showed the original deferral justification ("cross-cutting" OpenAPI/ogen change) was incorrect — the fix is scoped entirely to WorkflowExecution's own audit schema. Rewrote "Known Limitations" to keep only the genuinely out-of-scope item (per-attempt root-cause attribution) |
