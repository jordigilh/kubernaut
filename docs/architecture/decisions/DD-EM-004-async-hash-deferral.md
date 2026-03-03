# DD-EM-004: Deferred Hash Computation for Async-Managed Targets

**Status**: PROPOSED
**Date**: 2026-03-02
**Author**: Architecture Team

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-03-02 | Architecture Team | Initial DD: async hash deferral via RO-driven `hashComputeAfter` timestamp, CRD API group heuristic, GitOps label detection |

---

## Context and Problem Statement

The EM computes the post-remediation spec hash on the first reconciliation after the stabilization window (DD-EM-002). For synchronous workflows (direct kubectl patch), the target resource spec is already updated and the comparison with the pre-remediation hash is valid.

For **GitOps** and **operator-managed CRD** workflows, the spec change propagates asynchronously:

- **GitOps**: WE commits to git → ArgoCD/FluxCD syncs → resource spec changes after a delay
- **Operators**: WE modifies an operator CRD → operator controller reconciles → spec changes after a delay

In both cases, the EM captures `pre-hash == post-hash` because the external controller has not yet reconciled when the hash is first computed.

### Why This Matters

An incorrect `pre == post` hash comparison means:
- The EM reports "no spec change" when the remediation actually changed the resource
- The effectiveness score for the hash component is misleading
- The HAPI remediation history context receives incorrect drift data
- The DataStorage effectiveness query returns stale results

---

## Decision

### RO-driven `hashComputeAfter` timestamp

The RO detects whether the `RemediationTarget` is async-managed and sets `EA.Spec.HashComputeAfter` at EA creation time. The EM defers hash computation until that timestamp. The EM has zero awareness of GitOps or operator semantics.

### Async detection signals

Both signals are available at zero additional API cost during EA creation:

**Signal 1 — CRD API group heuristic:**

The RO resolves the `RemediationTarget.Kind` → GVK via the REST mapper (existing `resolveGVKForKind`). If the API group is not in the built-in Kubernetes group allowlist, the target is a CRD and assumed to be operator-managed.

```go
func isBuiltInGroup(group string) bool {
    builtIn := map[string]bool{
        "": true,                              // core
        "apps": true,                          // Deployment, StatefulSet, etc.
        "batch": true,                         // Job, CronJob
        "autoscaling": true,                   // HPA
        "networking.k8s.io": true,             // Ingress, NetworkPolicy
        "policy": true,                        // PDB
        "rbac.authorization.k8s.io": true,     // RBAC
        "storage.k8s.io": true,                // StorageClass, PV, PVC
        "coordination.k8s.io": true,           // Lease
        "node.k8s.io": true,                   // RuntimeClass
        "scheduling.k8s.io": true,             // PriorityClass
        "discovery.k8s.io": true,              // EndpointSlice
        "admissionregistration.k8s.io": true,  // Webhook configs
    }
    return builtIn[group]
}
```

**Rationale**: CRDs exist because an operator was installed to manage them. A CRD without a controller is inert data storage. In the worst case (CRD without an active operator), the hash is delayed but still correct.

**Signal 2 — GitOps labels (ADR-056):**

The RO reads `AA.Status.PostRCAContext.DetectedLabels.GitOpsManaged` from the AIAnalysis object (already fetched for `resolveDualTargets`). When `GitOpsManaged == true`, the target is ArgoCD/FluxCD-managed.

### RO decision flow

```go
isAsync := false

// Signal 1: CRD group heuristic
gvk, err := resolveGVKForKind(mapper, remediationTarget.Kind)
if err == nil && !isBuiltInGroup(gvk.Group) {
    isAsync = true
}

// Signal 2: GitOps labels
if ai != nil && ai.Status.PostRCAContext != nil &&
   ai.Status.PostRCAContext.DetectedLabels != nil &&
   ai.Status.PostRCAContext.DetectedLabels.GitOpsManaged {
    isAsync = true
}

if isAsync {
    ea.Spec.HashComputeAfter = &metav1.Time{Time: time.Now().Add(stabilizationWindow)}
}
```

### EA CRD spec change

```go
type EffectivenessAssessmentSpec struct {
    // ... existing fields ...

    // HashComputeAfter indicates when the EM should compute the post-remediation
    // spec hash. Set by the RO for async-managed targets (GitOps, operator CRDs)
    // where spec changes propagate after the WE completes.
    // Nil or zero means compute immediately (sync workflows, backward compatible).
    // Reference: DD-EM-004 (Async Hash Deferral), BR-EM-010, BR-RO-103
    // +optional
    HashComputeAfter *metav1.Time `json:"hashComputeAfter,omitempty"`
}
```

### EM reconciler change

In Step 7 (component checks), before the existing hash block:

```go
if !ea.Status.Components.HashComputed {
    // DD-EM-004: Defer hash for async-managed targets
    if ea.Spec.HashComputeAfter != nil && time.Now().Before(ea.Spec.HashComputeAfter.Time) {
        logger.V(1).Info("Hash computation deferred for async-managed target",
            "hashComputeAfter", ea.Spec.HashComputeAfter.Time,
            "remaining", time.Until(ea.Spec.HashComputeAfter.Time))
        return ctrl.Result{RequeueAfter: time.Until(ea.Spec.HashComputeAfter.Time)}, nil
    }
    result := r.assessHash(ctx, ea)
    // ... existing logic ...
}
```

---

## Considered Alternatives

| Approach | Why Discarded |
|---|---|
| EM self-correcting (retry if pre==post) | Cannot distinguish "genuine no-op" from "async not propagated" until stabilization end; adds retry complexity to EM |
| EM watches target resource | Goroutine/watch lifecycle management; race between check and watch setup; operational complexity |
| Always defer to stabilization end | Delays sync cases unnecessarily; sync "genuine no-op" takes 5 min to confirm |
| LLM detects operator management | Non-deterministic; LLM has world knowledge but not cluster truth; prompt pollution |
| `managedFields` / finalizer checks | More complex than needed; the API group heuristic is simpler and covers the same cases with zero API calls |
| Detected labels in EA spec | EA shouldn't carry label detection data; the RO makes the timing decision and encodes it as a timestamp |
| `hashComputeAfter` in EA status | This is a desired-state input from the RO, not derived state. Belongs in spec per K8s convention. |

---

## Consequences

### Positive

- Clean separation: RO decides timing, EM follows timestamp
- Zero extra API calls: both signals come from data already in hand
- Backward compatible: existing EAs without `hashComputeAfter` work identically
- Safe false positives: delayed hash for non-operator CRDs is still correct
- Extensible: future async patterns use the same timestamp mechanism
- Consistent with existing timing patterns (PrometheusCheckAfter, AlertManagerCheckAfter)

### Negative

- Adds one optional field to the EA CRD spec (minor schema growth)
- Built-in group allowlist requires maintenance if Kubernetes adds new core API groups (rare, ~1 per year)

### Risks

- **Stabilization window too short for slow operators**: If the operator takes longer than the stabilization window to reconcile, the hash is still captured at the wrong time
  - **Mitigation**: The stabilization window is configurable. For known slow operators, increase the window via EM config.
- **CRD without operator false positive**: A CRD target with no active operator gets a delayed hash
  - **Mitigation**: The hash is still computed and correct — just delayed. No impact on effectiveness accuracy.

---

## Related Decisions

- **DD-EM-002** (v1.2): Canonical spec hash algorithm, pre/post comparison
- **DD-EM-003**: Dual-target assessment (SignalTarget vs RemediationTarget)
- **ADR-EM-001** (v1.4): EM integration architecture
- **ADR-056**: Post-RCA label computation (DetectedLabels flow)
- **BR-EM-004**: Spec hash comparison to detect drift
- **BR-EM-009**: Derived timing computation pattern
- **BR-EM-010**: EM hash deferral requirement
- **BR-RO-103**: RO async target detection requirement
- **#251**: Implementation issue
- **#132**: GitOps causality (related future work)
