# DD-CRD-002-EffectivenessAssessment: Kubernetes Conditions for EffectivenessAssessment CRD

**Status**: PROPOSED
**Version**: 1.0
**Date**: February 14, 2026
**CRD**: EffectivenessAssessment
**Service**: EffectivenessMonitor
**Parent Standard**: DD-CRD-002

---

## Overview

This document specifies the Kubernetes Conditions for the **EffectivenessAssessment** CRD per DD-CRD-002 standard.

**Business Requirements**: BR-EM-001 (health), BR-EM-002 (alerts), BR-EM-003 (metrics), BR-EM-004 (hash/drift)

---

## Condition Types (3)

| Condition Type | Purpose | Set By |
|----------------|---------|--------|
| `Ready` | Aggregate: True on Completed | Reconciler |
| `AssessmentComplete` | Assessment reached a terminal state | Reconciler (completeAssessment) |
| `SpecIntegrity` | Post-remediation spec hash is still valid | Reconciler (Step 6.5 drift guard) |

---

## Condition Specifications

### AssessmentComplete

Indicates that the EM has finished assessing the effectiveness of the remediation. The `Reason` field distinguishes between success (all components assessed), partial completion, expiry, and spec drift.

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `AssessmentFull` | "All enabled components assessed successfully" |
| `True` | `AssessmentPartial` | "Assessment completed with partial data (some components not assessed)" |
| `True` | `AssessmentExpired` | "Validity window expired before all components could be assessed" |
| `True` | `SpecDrift` | "Assessment invalidated: target resource spec was modified (spec drift detected)" |
| `True` | `MetricsTimedOut` | "Metrics not available before validity expired" |
| `True` | `NoExecution` | "No workflow execution found for this remediation" |

**Note**: `Status` is always `True` for `AssessmentComplete` because the assessment has reached a terminal state (the EA is in `Completed` phase). The `Reason` field distinguishes between the different completion outcomes.

### Ready

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `Ready` | "Assessment completed" |
| `False` | `NotReady` | "Assessment in progress" |

**When Set**: True when phase is Completed (AssessmentComplete reached terminal state).

### SpecIntegrity

Indicates whether the target resource's `.spec` has remained unchanged since the post-remediation hash was computed. This condition is set on every reconcile after `HashComputed=true` (DD-EM-002 v1.1).

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `SpecUnchanged` | "Target resource spec unchanged since post-remediation hash" |
| `False` | `SpecDrifted` | "Target spec hash changed: {postRemediationHash} -> {currentHash}" |

---

## Status Fields Referenced by Conditions

The `SpecIntegrity` condition depends on two hash fields in `status.components`, both populated by the reconciler's hash computation step (DD-EM-002 v1.1):

| Field | Type | Description | Set When |
|-------|------|-------------|----------|
| `postRemediationSpecHash` | `string` | Hash of target resource `.spec` at first reconcile (baseline) | `HashComputed` transitions to `true` |
| `currentSpecHash` | `string` | Most recent hash of target resource `.spec`, recomputed every reconcile after `HashComputed=true` | Every reconcile after baseline is established |

**Drift detection**: When `currentSpecHash != postRemediationSpecHash`, the reconciler sets `SpecIntegrity=False` with reason `SpecDrifted` and completes the assessment with reason `spec_drift` (DD-EM-002 v1.1 Spec Drift Guard).

---

## Implementation

**Helper File**: `pkg/effectivenessmonitor/conditions/conditions.go`

**MANDATORY**: Use canonical Kubernetes functions per DD-CRD-002 v1.2:
- `meta.SetStatusCondition()` for setting conditions
- `meta.FindStatusCondition()` for reading conditions

**Note**: Per DD-CRD-002 v1.1, each CRD has its own dedicated conditions file regardless of which controller manages it.

### Constants

```go
// Condition Types
const (
    ConditionAssessmentComplete = "AssessmentComplete"
    ConditionSpecIntegrity      = "SpecIntegrity"
)

// Reasons for AssessmentComplete
const (
    ReasonAssessmentFull       = "AssessmentFull"
    ReasonAssessmentPartial    = "AssessmentPartial"
    ReasonAssessmentExpired    = "AssessmentExpired"
    ReasonSpecDrift            = "SpecDrift"
    ReasonMetricsTimedOut      = "MetricsTimedOut"
    ReasonNoExecution          = "NoExecution"
)

// Reasons for SpecIntegrity
const (
    ReasonSpecUnchanged = "SpecUnchanged"
    ReasonSpecDrifted   = "SpecDrifted"
)
```

### Integration Points

| Integration Point | Conditions Set |
|-------------------|----------------|
| `reconciler.go:Step 6.5` (drift guard) | SpecIntegrity (True/False on each reconcile) |
| `reconciler.go:Step 6.5` (drift detected) | AssessmentComplete (SpecDrift) |
| `reconciler.go:completeAssessment` (all reasons) | AssessmentComplete (Full/Partial/Expired/etc.) |

---

## Validation

```bash
kubectl explain effectivenessassessment.status.conditions
kubectl describe effectivenessassessment ea-test-123 | grep -A20 "Conditions:"
kubectl wait --for=condition=AssessmentComplete ea/ea-test-123 --timeout=10m
```

---

## Status

| Component | Status |
|-----------|--------|
| CRD Schema | Exists (`Conditions []metav1.Condition` in Status) |
| Helper functions | Pending |
| Controller integration | Pending |
| Unit tests | Pending |

---

## References

- [DD-CRD-002](mdc:docs/architecture/decisions/DD-CRD-002-kubernetes-conditions-standard.md) (Parent)
- [DD-EM-002](mdc:docs/architecture/decisions/DD-EM-002-canonical-spec-hash.md) (Spec Drift Guard v1.1)
- [DD-CRD-002-RemediationRequest](mdc:docs/architecture/decisions/DD-CRD-002-remediationrequest-conditions.md) (Pattern reference)
- [ADR-EM-001](mdc:docs/architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md) (EM integration architecture)
