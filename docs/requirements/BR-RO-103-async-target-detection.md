# BR-RO-103: Async Target Detection for Hash Computation Timing

**Status**: Draft
**Date**: 2026-03-02
**Category**: ORCHESTRATION
**Priority**: High
**Related**: BR-EM-010, DD-EM-004, ADR-056, #132, #251

---

## Business Need

The Remediation Orchestrator creates EffectivenessAssessment CRDs after a remediation completes. For async-managed targets (GitOps or operator-managed CRDs), the hash computation must be deferred so the EM captures the post-remediation spec after the external controller has reconciled.

The RO is the correct decision point: it has access to the workflow context, detected labels from the RCA pipeline, and the target resource GVK at EA creation time.

## Requirements

### BR-RO-103.1: CRD API Group Detection

**The RO MUST detect whether the `RemediationTarget` is a Custom Resource (operator-managed) by checking its API group against a built-in Kubernetes group allowlist.**

If the Kind resolves to an API group that is not in the built-in allowlist, it is a CRD and assumed to be operator-managed.

**Acceptance Criteria:**
- `isBuiltInGroup(group string) bool` utility function implemented
- Built-in groups include: core (`""`), `apps`, `batch`, `autoscaling`, `networking.k8s.io`, `policy`, `rbac.authorization.k8s.io`, `storage.k8s.io`, `coordination.k8s.io`, `node.k8s.io`, `scheduling.k8s.io`, `discovery.k8s.io`, `admissionregistration.k8s.io`
- GVK resolved via existing `resolveGVKForKind` using the REST mapper
- Non-built-in groups (e.g., `cert-manager.io`, `acid.zalan.do`, `argoproj.io`) return false

### BR-RO-103.2: GitOps Detection

**The RO MUST detect whether the `RemediationTarget` is GitOps-managed by reading `DetectedLabels` from the AIAnalysis status.**

The RO already fetches the AIAnalysis object when creating the EA. It MUST additionally read `AA.Status.PostRCAContext.DetectedLabels.GitOpsManaged`.

**Acceptance Criteria:**
- When `AA.Status.PostRCAContext.DetectedLabels.GitOpsManaged == true`, the target is GitOps-managed
- When AIAnalysis is nil or DetectedLabels are nil, the target is not considered GitOps-managed (graceful degradation)
- No additional API calls required (AA already fetched)

### BR-RO-103.3: HashComputeAfter Population

**The RO MUST set `EA.Spec.HashComputeAfter` when either async signal is detected (CRD group or GitOps labels).**

**Acceptance Criteria:**
- When CRD group detected OR GitOps labels detected: `HashComputeAfter = time.Now() + StabilizationWindow`
- When neither detected: `HashComputeAfter` is nil (EM computes hash immediately)
- The stabilization window duration is the same value used for `EA.Spec.Config.StabilizationWindow`

## Design Rationale

1. **RO as decision maker**: The RO has the full workflow context. The EM should not need to reason about GitOps or operators.
2. **Zero extra API calls**: The AA object is already fetched for `resolveDualTargets`. The GVK is already resolved for pre-hash computation. Both detection signals are free.
3. **CRD heuristic is safe**: A CRD without an active operator simply gets a delayed (but correct) hash. No false negatives for effectiveness.
4. **Graceful degradation**: Missing AA or DetectedLabels fall back to immediate hash (existing behavior).

## Test Coverage

See [Issue #251 Test Plan](../../testing/ISSUE-251/TEST_PLAN.md) — RO detection domain:
- UT-RO-251-001 through UT-RO-251-004 (isBuiltInGroup, GitOps detection, EA creation)
- IT-RO-251-001 through IT-RO-251-003 (envtest EA creation with different target types)

## References

- [DD-EM-004](../../architecture/decisions/DD-EM-004-async-hash-deferral.md) — Async hash deferral design
- [ADR-056](../../architecture/decisions/ADR-056-post-rca-label-computation.md) — Post-RCA label computation
- [BR-EM-010](BR-EM-010-async-hash-deferral.md) — EM hash deferral requirement
- [#132](https://github.com/jordigilh/kubernaut/issues/132) — GitOps causality
- [#251](https://github.com/jordigilh/kubernaut/issues/251) — Implementation issue
