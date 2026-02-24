# DD-EM-003: Dual-Target Effectiveness Assessment (Signal Target + Remediation Target)

**Version**: 1.2
**Date**: 2026-02-24
**Status**: ✅ APPROVED
**Author**: EffectivenessMonitor Team
**Reviewers**: RemediationOrchestrator Team, AIAnalysis Team

---

## Context

The EffectivenessAssessment (EA) CRD currently has a single `targetResource` field. The EM uses this target for all four assessment components: spec hash, health, alert resolution, and metrics.

However, the resource that triggered the alert (signal target) and the resource that the workflow modifies (remediation target) are not always the same:

| Scenario | Signal Target | Remediation Target |
|----------|--------------|-------------------|
| HPA maxed out | `Deployment/api-frontend` | `HorizontalPodAutoscaler/api-frontend` |
| Pod crashloop | `Pod/payment-api-789` | `Deployment/payment-api` (restart) |
| Node disk pressure | `Node/worker-1` | `Node/worker-1` (same) |
| PDB blocking rollout | `Deployment/api-v2` | `PodDisruptionBudget/api-pdb` |

When these differ, a single target produces misleading results:

- **Spec hash on signal target**: Hash of the Deployment spec, which was never modified. Pre == Post, falsely reporting "no drift" when the real change was on the HPA.
- **Health on remediation target**: Health check on the HPA, which doesn't tell us whether the Deployment's workload improved.

### Triggering Incident

During the `hpa-maxed` demo scenario, the EA target was `Deployment/api-frontend` but the workflow patched `HorizontalPodAutoscaler/api-frontend`. The spec hash showed `preRemediationSpecHash == postRemediationSpecHash` because the Deployment spec was never modified. The hash computation was technically correct but semantically meaningless.

---

## Decision

**The EA CRD carries two target references: `signalTarget` (from the Gateway/RR) and `remediationTarget` (from HAPI/AA). Each EM assessment component uses the appropriate target.**

Both targets are already available in the Remediation Orchestrator:

- **Signal target**: Extracted by the Gateway from alert labels, propagated through the RR
- **Remediation target**: Determined by HAPI's RCA resolution, available in the AA's `status.rootCauseAnalysis.affectedResource`

This is a plumbing change -- no new data needs to be generated.

---

## Implementation

### EA CRD Extension

```go
type EffectivenessAssessmentSpec struct {
    // ... existing fields (correlationID, signalName, config, etc.)

    // SignalTarget is the resource that triggered the alert.
    // Source: RR.spec.targetResource (from Gateway alert extraction).
    // Used by: health assessment, metrics queries.
    SignalTarget TargetResource `json:"signalTarget"`

    // RemediationTarget is the resource the workflow modified.
    // Source: AA.status.rootCauseAnalysis.affectedResource (from HAPI RCA resolution).
    // Used by: spec hash computation, drift detection.
    RemediationTarget TargetResource `json:"remediationTarget"`
}

type TargetResource struct {
    Kind      string `json:"kind"`
    Name      string `json:"name"`
    Namespace string `json:"namespace,omitempty"`
}
```

### Component-to-Target Mapping

| Component | Target Field | Rationale |
|-----------|-------------|-----------|
| Spec hash (DD-EM-002) | `remediationTarget` | Hash the resource that was actually modified to detect drift |
| Health (BR-EM-001) | `signalTarget` | Check if the workload that triggered the alert is healthy |
| Alert resolution (BR-EM-003) | `signalTarget.Namespace` + signal name | AlertContext uses signal name for matching and `signalTarget.Namespace` for scoping |
| Metrics (BR-EM-002) | `signalTarget.Namespace` | PromQL queries scoped to the signal target's namespace |

### EM Reconciler Changes

```go
// assessHash uses remediationTarget (the resource the workflow modified)
func (r *Reconciler) assessHash(ctx context.Context, ea *eav1.EffectivenessAssessment) hash.ComputeResult {
    spec := r.getTargetSpec(ctx, ea.Spec.RemediationTarget)
    // ...
}

// assessHealth uses signalTarget (the resource the alert is about)
func (r *Reconciler) assessHealth(ctx context.Context, ea *eav1.EffectivenessAssessment) health.Result {
    status := r.getTargetHealthStatus(ctx, ea.Spec.SignalTarget)
    // ...
}
```

### RO Changes

The RO creates the EA with both targets populated from data it already has:

```go
func (r *Reconciler) createEffectivenessAssessment(ctx context.Context, rr *rrv1.RemediationRequest) error {
    ea := &eav1.EffectivenessAssessment{
        Spec: eav1.EffectivenessAssessmentSpec{
            // Signal target from the RR (Gateway extraction)
            SignalTarget: eav1.TargetResource{
                Kind:      rr.Spec.TargetResource.Kind,
                Name:      rr.Spec.TargetResource.Name,
                Namespace: rr.Spec.TargetResource.Namespace,
            },
            // Remediation target from the AA (HAPI RCA resolution)
            RemediationTarget: eav1.TargetResource{
                Kind:      aa.Status.RootCauseAnalysis.AffectedResource.Kind,
                Name:      aa.Status.RootCauseAnalysis.AffectedResource.Name,
                Namespace: aa.Status.RootCauseAnalysis.AffectedResource.Namespace,
            },
            // ...
        },
    }
}
```

### Data Flow

```
Gateway
  │ extracts alert labels → signalTarget (Deployment/api-frontend)
  ▼
RR ──────────────────────────────────────────────┐
                                                  │
HAPI                                              │
  │ RCA analysis → selects workflow               │
  │ determines remediationTarget                  │
  │ (HorizontalPodAutoscaler/api-frontend)        │
  ▼                                               │
AA ──────────────────────────────────────────────┐│
                                                  ││
RO (has both)                                     ││
  │ creates EA with:                              ││
  │   signalTarget ◄──────────────────────────────┘│
  │   remediationTarget ◄──────────────────────────┘
  ▼
EA
  │
  ▼
EM
  ├─ spec hash    → reads remediationTarget (HPA)
  ├─ health       → reads signalTarget (Deployment)
  ├─ alert        → reads signalName + signalTarget.Namespace
  └─ metrics      → reads signalTarget.Namespace
```

### Backward Compatibility

No backward compatibility is needed. This is pre-release v1.0 with no production EAs. The single `targetResource` field is removed entirely and replaced by `signalTarget` and `remediationTarget`.

---

## Affected Components

| Component | Team | Change |
|-----------|------|--------|
| EA CRD types | EffectivenessMonitor | Replace `targetResource` with `signalTarget` + `remediationTarget` |
| EA CRD manifest | EffectivenessMonitor | Regenerate CRD YAML with new schema |
| EM reconciler | EffectivenessMonitor | Route each component to the correct target |
| RO controller | RemediationOrchestrator | Populate both targets via `resolveDualTargets()` |
| RO EA creator | RemediationOrchestrator | Accept `DualTarget` struct, set both fields on EA |

---

## Consequences

### Positive

1. **Accurate spec hash**: Drift detection measures the resource the workflow actually modified
2. **Accurate health/metrics**: Workload assessment measures the resource under pressure
3. **Correct pre/post comparison**: Pre-remediation and post-remediation hashes will differ when the workflow successfully modifies the remediation target
4. **No new data sources**: Both targets are already available in the RO from existing pipeline stages

### Negative

1. **CRD schema change**: Requires updating the EA CRD and reapplying to the cluster
   - Mitigation: Pre-release v1.0, no production EAs to migrate
2. **RO must read from AA status**: RO needs to extract the remediation target from the AA's `rootCauseAnalysis.affectedResource`
   - Mitigation: RO already reads AA status for other fields (confidence, approval required)

### Neutral

1. **Same-target scenarios**: When signal and remediation targets are the same (e.g., restart a crashing Deployment), both fields have the same value -- no behavioral change

---

## Related Documents

- [DD-EM-002: Canonical Spec Hash](./DD-EM-002-canonical-spec-hash.md) -- Spec hash algorithm, extended here with dual-target routing
- [DD-WE-005: Workflow-Scoped RBAC](./DD-WE-005-workflow-scoped-rbac.md) -- RBAC rules also identify the modified resource
- [Issue #183: EM spec hash empty for HPA](https://github.com/jordigilh/kubernaut/issues/183) -- Fixed empty hash, revealed dual-target gap
- [Issue #184: Propagate full GVK through pipeline](https://github.com/jordigilh/kubernaut/issues/184) -- GVK propagation complements dual-target

---

## Document Maintenance

| Date | Version | Changes |
|------|---------|---------|
| 2026-02-23 | 1.0 | Initial decision - dual-target EA for accurate per-component assessment |
| 2026-02-24 | 1.2 | Issue #188: Clarified alert resolution routing -- uses `signalTarget.Namespace` + signal name (not just signal name). Metrics routing clarified as `signalTarget.Namespace`. Updated data flow diagram. |
| 2026-02-24 | 1.1 | Issue #188: Fix remediation target source to `rootCauseAnalysis.affectedResource` (not `selectedWorkflow.targetResource`). Remove deprecated `targetResource` field (pre-release v1.0, no backward compat needed). Update RO to use `resolveDualTargets()` with `DualTarget` struct. |
