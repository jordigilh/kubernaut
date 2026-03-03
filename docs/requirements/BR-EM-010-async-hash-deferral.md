# BR-EM-010: Deferred Hash Computation for Async-Managed Targets

**Status**: Draft
**Date**: 2026-03-02
**Category**: EFFECTIVENESS
**Priority**: High
**Related**: BR-EM-004, DD-EM-002, DD-EM-004, ADR-EM-001, #251

---

## Business Need

The Effectiveness Monitor computes the post-remediation spec hash immediately after the stabilization window. For synchronous workflows (direct kubectl patch, in-cluster modifications), the target resource spec is already updated at this point and the hash comparison is valid.

However, for **GitOps** (ArgoCD, FluxCD) and **operator-managed CRD** workflows, the spec change propagates asynchronously: the WorkflowExecution completes, but the target resource has not yet been modified by the external controller. The EM captures `pre-hash == post-hash`, incorrectly concluding no spec change occurred.

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

## Design Rationale

1. **Clean separation of concerns**: The EM does not need to know why hash computation is deferred. It follows a timestamp set by the RO.
2. **Backward compatible**: Existing EAs without `hashComputeAfter` behave identically to today.
3. **Consistent with existing patterns**: `PrometheusCheckAfter` and `AlertManagerCheckAfter` follow the same timing-gate pattern (BR-EM-009).

## Test Coverage

See [Issue #251 Test Plan](../../testing/ISSUE-251/TEST_PLAN.md) — EM hash deferral domain:
- UT-EM-251-001 through UT-EM-251-003 (EM reconciler gating)
- IT-EM-251-001 through IT-EM-251-003 (envtest integration)
- E2E-EM-251-001 (cert-manager operator scenario)

## References

- [DD-EM-004](../../architecture/decisions/DD-EM-004-async-hash-deferral.md) — Async hash deferral design
- [DD-EM-002](../../architecture/decisions/DD-EM-002-canonical-spec-hash.md) — Canonical spec hash algorithm
- [BR-EM-004] — Spec hash comparison to detect configuration drift
- [BR-EM-009](BR-EM-009-derived-timing-computation.md) — Derived timing computation pattern
- [#251](https://github.com/jordigilh/kubernaut/issues/251) — Implementation issue
