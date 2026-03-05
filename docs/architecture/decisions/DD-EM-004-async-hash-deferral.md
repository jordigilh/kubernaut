# DD-EM-004: Deferred Hash Computation for Async-Managed Targets

**Status**: PROPOSED
**Date**: 2026-03-02
**Author**: Architecture Team

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-03-02 | Architecture Team | Initial DD: async hash deferral via RO-driven `hashComputeAfter` timestamp, CRD API group heuristic, GitOps label detection |
| 2.0 | 2026-03-03 | Architecture Team | #253: Corrected timing model — separate propagation delay from stabilization window; `WaitingForPropagation` phase; RO config `gitOpsSyncDelay` / `operatorReconcileDelay`; compounding logic; EM `checkAfter` anchored to `HashComputeAfter` |
| 3.0 | 2026-03-05 | Architecture Team | #277: Migrated `HashComputeAfter` (absolute Time) to `HashCheckDelay` (relative Duration in EAConfig). Added `AlertCheckDelay` for proactive signal alert resolution deferral. Removed `GitOpsSyncDelay`/`OperatorReconcileDelay` from EA spec (RO audit handles these). Added `AlertCheckAfter` to `DerivedTiming`. RO detects proactive signals via `AIAnalysis.Spec.AnalysisRequest.SignalContext.SignalMode`. |

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

### RO-driven `HashCheckDelay` duration

The RO detects whether the `RemediationTarget` is async-managed and sets `EA.Spec.Config.HashCheckDelay` at EA creation time. The RO computes the total propagation delay from its config (`gitOpsSyncDelay`, `operatorReconcileDelay`) and sets it as a single `HashCheckDelay` Duration. The EM defers hash computation until `creation + HashCheckDelay`. The EM has zero awareness of GitOps or operator semantics.

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
var propagationDelay time.Duration

// Signal 1: GitOps labels (syncs manifest to cluster first)
if ai != nil && ai.Status.PostRCAContext != nil &&
   ai.Status.PostRCAContext.DetectedLabels != nil &&
   ai.Status.PostRCAContext.DetectedLabels.GitOpsManaged {
    propagationDelay += config.AsyncPropagation.GitOpsSyncDelay
}

// Signal 2: CRD group heuristic (operator reconciles after GitOps sync)
gvk, err := resolveGVKForKind(mapper, remediationTarget.Kind)
if err == nil && !isBuiltInGroup(gvk.Group) {
    propagationDelay += config.AsyncPropagation.OperatorReconcileDelay
}

if propagationDelay > 0 {
    ea.Spec.Config.HashCheckDelay = &metav1.Duration{Duration: propagationDelay}
}
```

### EA CRD spec change

```go
type EffectivenessAssessmentSpec struct {
    // ... existing fields ...
    Config *EAConfig `json:"config,omitempty"`
}

type EAConfig struct {
    // ... existing fields (StabilizationWindow, etc.) ...

    // HashCheckDelay is the duration after EA creation when the EM should compute
    // the post-remediation spec hash. Set by the RO for async-managed targets
    // (GitOps, operator CRDs) where spec changes propagate after the WE completes.
    // The RO computes the total propagation delay from gitOpsSyncDelay and
    // operatorReconcileDelay and sets it here. Nil or zero means compute immediately
    // (sync workflows, backward compatible).
    // Reference: DD-EM-004 (Async Hash Deferral), BR-EM-010, BR-RO-103
    // +optional
    HashCheckDelay *metav1.Duration `json:"hashCheckDelay,omitempty"`

    // AlertCheckDelay is an additive duration on top of StabilizationWindow for
    // proactive (predictive) alert resolution checks. Set by the RO when
    // SignalMode is "proactive". Prometheus metric checks are NOT affected.
    // Reference: DD-EM-004, BR-RO-103.6, #277
    // +optional
    AlertCheckDelay *metav1.Duration `json:"alertCheckDelay,omitempty"`
}
```

### EM reconciler change

In Step 7 (component checks), before the existing hash block:

```go
if !ea.Status.Components.HashComputed {
    // DD-EM-004: Defer hash for async-managed targets
    if ea.Spec.Config != nil && ea.Spec.Config.HashCheckDelay != nil && ea.Spec.Config.HashCheckDelay.Duration > 0 {
        deadline := ea.CreationTimestamp.Add(ea.Spec.Config.HashCheckDelay.Duration)
        if time.Now().Before(deadline) {
            logger.V(1).Info("Hash computation deferred for async-managed target",
                "hashCheckDelay", ea.Spec.Config.HashCheckDelay.Duration,
                "remaining", time.Until(deadline))
            return ctrl.Result{RequeueAfter: time.Until(deadline)}, nil
        }
    }
    result := r.assessHash(ctx, ea)
    // ... existing logic ...
}
```

---

## V2.0: Corrected Timing Model (#253)

### Problem with V1.0

V1.0 used `HashComputeAfter = now + stabilizationWindow`, conflating the propagation delay with the stabilization window. This means:

```
V1.0 (broken):
|───── stabilizationWindow ─────|
^                                ^
RR completes               hash computed + health checked (simultaneously)
                           zero effective stabilization after change arrives
```

The EM computes the hash and runs health/alert checks at the same time, leaving no stabilization after the change propagates.

### Corrected Model

Propagation delay and stabilization are independent sequential stages:

```
V2.0 (correct):
|── propagation delay ──|──── stabilization window ────|
^                        ^                              ^
RR completes       hash computed                  health/metrics
(workflow done)    (change arrived)                assessed
                   stabilization starts
```

### Phase Diagram

```
Sync target (nil HashCheckDelay):
  Pending → Stabilizing → Assessing → Completed/Failed

Async target (non-nil HashCheckDelay):
  Pending → WaitingForPropagation → Stabilizing → Assessing → Completed/Failed
                     │                     │
                     │ hash deferred       │ hash computed,
                     │ (requeue)           │ stabilization starts
                     └─────────────────────┘
```

### RO: Propagation Delay Configuration

The RO exposes two configurable durations for the async propagation stages:

```yaml
asyncPropagation:
  gitOpsSyncDelay: 3m          # ArgoCD/Flux sync interval
  operatorReconcileDelay: 1m   # Operator reconciliation time
```

### RO: Compounding Logic

The RO computes the total propagation delay as the sum of applicable stages and sets it as a single `HashCheckDelay` Duration on the EA. The individual `gitOpsSyncDelay` and `operatorReconcileDelay` remain in RO config only; they are not propagated to the EA spec.

```go
var propagationDelay time.Duration
if isGitOpsManaged {
    propagationDelay += config.AsyncPropagation.GitOpsSyncDelay
}
if !creator.IsBuiltInGroup(gvk.Group) {
    propagationDelay += config.AsyncPropagation.OperatorReconcileDelay
}

if propagationDelay > 0 {
    ea.Spec.Config.HashCheckDelay = &metav1.Duration{Duration: propagationDelay}
}
```

| Target Type | GitOps | CRD | propagationDelay |
|-------------|--------|-----|-----------------|
| `Deployment` (direct patch) | No | No | 0 (nil) |
| `Deployment` (ArgoCD-managed) | Yes | No | `gitOpsSyncDelay` (3m) |
| `Certificate` (cert-manager) | No | Yes | `operatorReconcileDelay` (1m) |
| `Certificate` (ArgoCD + cert-manager) | Yes | Yes | `gitOpsSyncDelay + operatorReconcileDelay` (4m) |

### EM: Adjusted Timing Computation

When `HashCheckDelay` is set, the EM computes the hash deadline as `creation + HashCheckDelay`. Health/alert check timing is anchored to that deadline (when hash is computed) instead of `EA.creationTimestamp`:

```go
stabilizationWindow := ea.Spec.Config.StabilizationWindow.Duration

if ea.Spec.Config != nil && ea.Spec.Config.HashCheckDelay != nil && ea.Spec.Config.HashCheckDelay.Duration > 0 {
    // Async target: stabilization starts after propagation completes (creation + HashCheckDelay)
    hashDeadline := ea.CreationTimestamp.Add(ea.Spec.Config.HashCheckDelay.Duration)
    checkAfter = metav1.NewTime(hashDeadline.Add(stabilizationWindow))
} else {
    // Sync target: stabilization starts at EA creation (existing behavior)
    checkAfter = metav1.NewTime(ea.CreationTimestamp.Add(stabilizationWindow))
}

ea.Status.PrometheusCheckAfter = &checkAfter
ea.Status.AlertManagerCheckAfter = &checkAfter
// When AlertCheckDelay is set (proactive signals): AlertManagerCheckAfter += AlertCheckDelay
```

### EM: Validity Deadline Extension

For async targets, the validity deadline must account for the full timeline:

```go
var totalDelay time.Duration
if ea.Spec.Config != nil && ea.Spec.Config.HashCheckDelay != nil {
    totalDelay = ea.Spec.Config.HashCheckDelay.Duration + stabilizationWindow + r.Config.ValidityWindow
} else {
    totalDelay = stabilizationWindow + r.Config.ValidityWindow
}
deadline := metav1.NewTime(ea.CreationTimestamp.Add(totalDelay))
ea.Status.ValidityDeadline = &deadline
```

### Complete Timing Example

For a GitOps + operator target (e.g., ArgoCD syncs a cert-manager Certificate):

```
RO config:
  gitOpsSyncDelay:        3m
  operatorReconcileDelay: 1m

EA Config (set by RO):
  HashCheckDelay: 4m       # RO computes total: 3m + 1m
  StabilizationWindow:    5m
  validityWindow:         10m

Timeline:
T+0m:  RR completes, EA created (Pending)
T+0m:  EM enters WaitingForPropagation phase
T+4m:  creation + HashCheckDelay reached → hash computed → Stabilizing phase
T+9m:  PrometheusCheckAfter reached → health/alert/metrics assessed → Assessing phase
T+19m: ValidityDeadline → EA expires if not yet completed
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
| Detected labels in EA spec | EA shouldn't carry label detection data; the RO makes the timing decision and encodes it as a Duration |
| `hashCheckDelay` in EA status | This is a desired-state input from the RO, not derived state. Belongs in spec per K8s convention. |

---

## Consequences

### Positive

- Clean separation: RO decides timing, EM follows `HashCheckDelay` (creation + Duration)
- Zero extra API calls: both signals come from data already in hand
- Backward compatible: existing EAs without `HashCheckDelay` work identically
- Safe false positives: delayed hash for non-operator CRDs is still correct
- Extensible: future async patterns use the same duration mechanism
- Consistent with existing timing patterns (PrometheusCheckAfter, AlertManagerCheckAfter)

### Negative

- Adds optional fields to the EA CRD spec (HashCheckDelay, AlertCheckDelay in Config; minor schema growth)
- Built-in group allowlist requires maintenance if Kubernetes adds new core API groups (rare, ~1 per year)

### Risks

- **Propagation delay underestimated**: If the actual sync/reconciliation time exceeds the RO's computed `HashCheckDelay` (from `gitOpsSyncDelay` and `operatorReconcileDelay`), the hash is captured before the change arrives
  - **Mitigation (V2.0)**: Both delays are independently configurable. Operators can tune per environment. Conservative defaults (3m + 1m) cover the majority of ArgoCD/Flux and operator deployments.
  - **Mitigation (future)**: Dynamic determination from ArgoCD `Application.spec.syncPolicy` or Flux `Kustomization.spec.interval` (see #253, future consideration)
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
- **#251**: Async hash deferral (foundation: detection + field + deferral mechanism)
- **#253**: Timing model correction (propagation delay vs stabilization)
- **#277**: HashCheckDelay migration, AlertCheckDelay for proactive signals
- **#132**: GitOps causality (related future work)
