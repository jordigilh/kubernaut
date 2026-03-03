# BR-EM-010: Deferred Hash Computation for Async-Managed Targets

**Status**: Draft
**Date**: 2026-03-02
**Category**: EFFECTIVENESS
**Priority**: High
**Related**: BR-EM-004, DD-EM-002, DD-EM-004, ADR-EM-001, #251, #253

---

## Business Need

The Effectiveness Monitor computes the post-remediation spec hash immediately after the stabilization window. For synchronous workflows (direct kubectl patch, in-cluster modifications), the target resource spec is already updated at this point and the hash comparison is valid.

However, for **GitOps** (ArgoCD, FluxCD) and **operator-managed CRD** workflows, the spec change propagates asynchronously: the WorkflowExecution completes, but the target resource has not yet been modified by the external controller. The EM captures `pre-hash == post-hash`, incorrectly concluding no spec change occurred.

### Timing Model Correction (#253)

The propagation delay (time for async changes to arrive) and the stabilization window (time for the system to settle after the change lands) are **distinct concerns** that must not be conflated:

```
|── propagation delay ──|──── stabilization window ────|
^                        ^                              ^
RR completes       hash computed                  health/metrics
(workflow done)    (change arrived)                assessed
                   stabilization starts
```

The hash computation marks the **beginning** of the stabilization window, not its end. Health and metrics checks occur after the full stabilization window has elapsed following hash computation.

## Requirements

### BR-EM-010.1: HashComputeAfter Gating

**The EM controller MUST defer hash computation when `EA.Spec.HashComputeAfter` is set and in the future.**

The EM reconciler MUST NOT call `assessHash` until `time.Now() >= EA.Spec.HashComputeAfter`. If the timestamp is in the future, the reconciler requeues with `RequeueAfter: time.Until(HashComputeAfter)`.

**Acceptance Criteria:**
- When `HashComputeAfter` is set and in the future, hash computation is skipped and the reconciler requeues
- When `HashComputeAfter` is nil or zero, hash computation proceeds immediately (current behavior, backward compatible)
- When `HashComputeAfter` is in the past, hash computation proceeds immediately
- The requeue duration is `time.Until(HashComputeAfter)`, not a fixed interval

### BR-EM-010.2: EA CRD Spec Field

**The EA CRD spec MUST include an optional `hashComputeAfter` field** of type `*metav1.Time`.

**Acceptance Criteria:**
- Field: `hashComputeAfter *metav1.Time` in `EffectivenessAssessmentSpec`
- JSON tag: `json:"hashComputeAfter,omitempty"`
- kubebuilder marker: `+optional`
- DeepCopy generated via `make generate`
- CRD manifest updated via `make manifests`
- Helm chart CRD synced

### BR-EM-010.3: WaitingForPropagation Phase (#253)

**The EA CRD MUST support a `WaitingForPropagation` phase** to distinguish async propagation wait from post-change stabilization.

**Phase flow:**
- `Pending → WaitingForPropagation → Stabilizing → Assessing → Completed/Failed`
- Sync targets (nil `HashComputeAfter`) skip `WaitingForPropagation` entirely

**Acceptance Criteria:**
- EA phase enum includes `WaitingForPropagation`
- EM enters `WaitingForPropagation` when `HashComputeAfter` is non-nil and in the future
- EM transitions to `Stabilizing` after hash is computed (i.e., after `HashComputeAfter` elapses and `assessHash` runs)
- Operators can distinguish "waiting for GitOps/operator sync" from "waiting for system to stabilize" via `kubectl get ea`
- Phase transition emits K8s event for observability

### BR-EM-010.4: Stabilization Anchored to Hash Computation (#253)

**When `HashComputeAfter` is set, the stabilization window MUST begin at `HashComputeAfter`, not at EA creation time.**

The health/alert/metrics checks must not begin until the system has had the full stabilization window to settle after the change has actually propagated.

**Acceptance Criteria:**
- When `HashComputeAfter` is set: `PrometheusCheckAfter = HashComputeAfter + StabilizationWindow`
- When `HashComputeAfter` is set: `AlertManagerCheckAfter = HashComputeAfter + StabilizationWindow`
- When `HashComputeAfter` is nil: existing behavior preserved (`checkAfter = EA.creation + StabilizationWindow`)
- Validity deadline extended: `ValidityDeadline = EA.creation + PropagationDelay + StabilizationWindow + ValidityWindow`

### BR-EM-010.5: Audit Trail for Propagation Timing (#253)

**The `assessment.scheduled` audit event MUST include propagation delay details** when `HashComputeAfter` is set.

**Acceptance Criteria:**
- Audit payload includes: `gitops_sync_delay` (duration string), `operator_reconcile_delay` (duration string), `total_propagation_delay` (duration string)
- Fields are omitted/null for sync targets (backward compatible)
- Operators can reconstruct the timing model from the audit trail

## Design Rationale

1. **Clean separation of concerns**: The EM does not need to know why hash computation is deferred. It follows a timestamp set by the RO.
2. **Backward compatible**: Existing EAs without `hashComputeAfter` behave identically to today.
3. **Consistent with existing patterns**: `PrometheusCheckAfter` and `AlertManagerCheckAfter` follow the same timing-gate pattern (BR-EM-009).
4. **Propagation ≠ Stabilization** (#253): The hash marks the moment the change has arrived. Stabilization measures the time for the system to settle after arrival. Conflating them produces zero effective stabilization.
5. **Operator observability** (#253): The `WaitingForPropagation` phase gives operators clear visibility into what the EA is waiting for.

## Test Coverage

See [Issue #251 Test Plan](../../testing/ISSUE-251/TEST_PLAN.md) — EM hash deferral domain:
- UT-EM-251-001 through UT-EM-251-005 (EM reconciler gating)
- IT-EM-251-001 through IT-EM-251-003 (envtest integration)
- E2E-FP-251-001 (cert-manager operator scenario)

See [Issue #253 Test Plan](../../testing/ISSUE-253/TEST_PLAN.md) — Timing model correction:
- UT/IT for `WaitingForPropagation` phase, adjusted `checkAfter`, validity extension
- E2E-FP-251-001 updated with corrected timing assertions

## References

- [DD-EM-004](../../architecture/decisions/DD-EM-004-async-hash-deferral.md) — Async hash deferral design
- [DD-EM-002](../../architecture/decisions/DD-EM-002-canonical-spec-hash.md) — Canonical spec hash algorithm
- [BR-EM-004] — Spec hash comparison to detect configuration drift
- [BR-EM-009](BR-EM-009-derived-timing-computation.md) — Derived timing computation pattern
- [#251](https://github.com/jordigilh/kubernaut/issues/251) — Async hash deferral (foundation)
- [#253](https://github.com/jordigilh/kubernaut/issues/253) — Timing model correction
