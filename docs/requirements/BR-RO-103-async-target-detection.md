# BR-RO-103: Async Target Detection for Hash Computation Timing

**Status**: Draft
**Date**: 2026-03-02
**Updated**: 2026-03-05 (Issue #277: proactive signal detection, AlertCheckDelay)
**Category**: ORCHESTRATION
**Priority**: High
**Related**: BR-EM-010, DD-EM-004, ADR-056, #132, #251, #253, #277

---

## Business Need

The Remediation Orchestrator creates EffectivenessAssessment CRDs after a remediation completes. For async-managed targets (GitOps or operator-managed CRDs), the hash computation must be deferred so the EM captures the post-remediation spec after the external controller has reconciled.

The RO is the correct decision point: it has access to the workflow context, detected labels from the RCA pipeline, and the target resource GVK at EA creation time.

### Propagation Delay Model (#253)

The propagation delay is the sum of independent async stages, each with its own configurable duration. The RO computes the total and sets `HashComputeAfter` accordingly:

```
propagationDelay = gitOpsSyncDelay (if GitOps-managed)
                 + operatorReconcileDelay (if CRD target)
```

For a target that is both GitOps-managed AND an operator CRD (e.g., ArgoCD syncs a cert-manager Certificate), both delays compound.

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

### BR-RO-103.3: HashComputeAfter Population (updated #253)

**The RO MUST set `EA.Spec.HashComputeAfter` using the computed propagation delay, NOT the stabilization window.**

**Acceptance Criteria:**
- `HashComputeAfter = time.Now() + propagationDelay`
- `propagationDelay = gitOpsSyncDelay (if GitOps) + operatorReconcileDelay (if CRD)`
- When neither detected: `HashComputeAfter` is nil (EM computes hash immediately)
- The propagation delay is independent of `StabilizationWindow`

### BR-RO-103.4: Propagation Delay Configuration (#253)

**The RO MUST expose async propagation delays as configurable durations** so operators can tune them for their environment.

**Configuration schema:**
```yaml
asyncPropagation:
  gitOpsSyncDelay: 3m          # Time for GitOps tool (ArgoCD/Flux) to sync
  operatorReconcileDelay: 1m   # Time for operator to reconcile CRD
```

**Acceptance Criteria:**
- `gitOpsSyncDelay` defaults to `3m` (conservative: ArgoCD default sync interval is 3m)
- `operatorReconcileDelay` defaults to `1m` (conservative: most operators reconcile in 30s-60s)
- Both durations are configurable via the RO config file and Helm chart values
- Durations of `0` disable the respective delay (useful for environments with instant sync)
- Invalid durations (negative) are rejected at config load time

### BR-RO-103.5: Compounding Logic (#253)

**The RO MUST compound propagation delays for targets that are both GitOps-managed AND operator-managed.**

When a target triggers both async signals (e.g., ArgoCD syncs a cert-manager Certificate), the delays represent sequential stages and must be summed:

```
|── gitOpsSyncDelay ──|── operatorReconcileDelay ──|
        ~3 min                   ~1 min
                                                    ^
                                              hash computed
                                              (total: 4 min)
```

**Acceptance Criteria:**
- GitOps-only target: `propagationDelay = gitOpsSyncDelay`
- Operator-only target: `propagationDelay = operatorReconcileDelay`
- GitOps + operator target: `propagationDelay = gitOpsSyncDelay + operatorReconcileDelay`
- Sync target (neither signal): `propagationDelay = 0`, `HashComputeAfter = nil`

## Design Rationale

1. **RO as decision maker**: The RO has the full workflow context. The EM should not need to reason about GitOps or operators.
2. **Zero extra API calls**: The AA object is already fetched for `resolveDualTargets`. The GVK is already resolved for pre-hash computation. Both detection signals are free.
3. **CRD heuristic is safe**: A CRD without an active operator simply gets a delayed (but correct) hash. No false negatives for effectiveness.
4. **Graceful degradation**: Missing AA or DetectedLabels fall back to immediate hash (existing behavior).
5. **Configuration over dynamic determination** (#253): Propagation delays are environment-specific constants. Config-based approach is predictable, requires no extra RBAC, and avoids coupling to ArgoCD/Flux APIs. Dynamic determination (inspecting ArgoCD Application sync intervals) is a future V2 enhancement.
6. **Compounding reflects reality** (#253): GitOps sync and operator reconciliation are sequential stages. ArgoCD syncs the manifest to the cluster, then the operator sees the change and reconciles. The delays are additive.

## Test Coverage

See [Issue #251 Test Plan](../../testing/ISSUE-251/TEST_PLAN.md) — RO detection domain:
- UT-RO-251-001 through UT-RO-251-007 (isBuiltInGroup)
- IT-RO-251-001, IT-RO-251-002 (envtest EA creation with different target types)

See [Issue #253 Test Plan](../../testing/ISSUE-253/TEST_PLAN.md) — Propagation delay computation:
- UT: compounding logic (GitOps-only, operator-only, both, neither)
- IT: config-driven propagation delay in EA spec
- E2E-FP-251-001: cert-manager CRD with corrected timing

### BR-RO-103.6: Proactive Signal Detection and AlertCheckDelay (#277)

**The RO MUST detect proactive (predictive) signals** by reading `AIAnalysis.Spec.AnalysisRequest.SignalContext.SignalMode`. When `SignalMode == "proactive"`:

- The RO sets `EA.Spec.Config.AlertCheckDelay` from `AsyncPropagationConfig.ProactiveAlertDelay` (default: 5m)
- This causes the EM to defer alert resolution checks by `AlertCheckDelay` beyond `StabilizationWindow`
- Prometheus metric checks (`PrometheusCheckAfter`) are NOT affected

**Rationale**: Proactive alerts (e.g., `predict_linear`) may take several minutes to resolve after remediation because they predict future conditions from historical data windows. Without this delay, the EM checks alert resolution too early and incorrectly reports the alert as still active.

**Acceptance Criteria:**
- RO reads `ai.Spec.AnalysisRequest.SignalContext.SignalMode` from the AIAnalysis CRD
- When `SignalMode == "proactive"` and `ProactiveAlertDelay > 0`, RO sets `AlertCheckDelay` on the EA
- `ProactiveAlertDelay` is configurable in `asyncPropagation` config section (default: 5m)
- When `SignalMode != "proactive"` or AIAnalysis unavailable, `AlertCheckDelay` is nil

## References

- [DD-EM-004](../../architecture/decisions/DD-EM-004-async-hash-deferral.md) — Async hash deferral design
- [ADR-056](../../architecture/decisions/ADR-056-post-rca-label-computation.md) — Post-RCA label computation
- [BR-EM-010](BR-EM-010-async-hash-deferral.md) — EM hash deferral requirement
- [BR-EM-009](BR-EM-009-derived-timing-computation.md) — Derived timing computation (AlertManagerCheckAfter)
- [#132](https://github.com/jordigilh/kubernaut/issues/132) — GitOps causality
- [#251](https://github.com/jordigilh/kubernaut/issues/251) — Async hash deferral (foundation)
- [#253](https://github.com/jordigilh/kubernaut/issues/253) — Timing model correction
- [#277](https://github.com/jordigilh/kubernaut/issues/277) — Alert stabilization delay
