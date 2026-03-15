# DD-CRD-002-EffectivenessAssessment: Kubernetes Conditions for EffectivenessAssessment CRD

**Status**: ✅ IMPLEMENTED
**Version**: 1.1
**Date**: March 4, 2026
**CRD**: EffectivenessAssessment
**Service**: EffectivenessMonitor
**Parent Standard**: DD-CRD-002

---

## Overview

This document specifies the Kubernetes Conditions for the **EffectivenessAssessment** CRD per DD-CRD-002 standard.

**Business Requirements**: BR-EM-001 (health), BR-EM-002 (alerts), BR-EM-003 (metrics), BR-EM-004 (hash/drift), BR-EM-012 (alert decay detection)

---

## Condition Types (4)

| Condition Type | Purpose | Set By |
|----------------|---------|--------|
| `Ready` | Aggregate: True on Completed | Reconciler |
| `AssessmentComplete` | Assessment reached a terminal state | Reconciler (completeAssessment) |
| `SpecIntegrity` | Post-remediation spec hash is still valid | Reconciler (Step 6.5 drift guard) |
| `AlertDecayDetected` | Alert decay monitoring is active (BR-EM-012, Issue #369) | Reconciler (alert assessment step) |

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
| `True` | `AlertDecayTimeout` | "Alert decay monitoring ended: validity window expired before alert resolved" |

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

### AlertDecayDetected

Indicates whether the EM is actively monitoring Prometheus alert decay (BR-EM-012, Issue #369). The EM suspects decay when the resource is healthy, the spec is stable, but the Prometheus alert is still firing — suggesting the alert is lagging behind the actual recovery. The condition is set to `True` while decay is suspected and `False` when the situation resolves.

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `DecayActive` | "Alert decay suspected: health={score}, alert still firing, retries={n}" |
| `False` | `DecayResolved` | "Alert decay monitoring resolved: alert is no longer considered decaying" |
| `False` | `DecayTimeout` | "Alert decay monitoring ended: validity window expired before alert resolved" |

**When Set**:
- `True/DecayActive`: On each reconcile where `isAlertDecay()` returns true (health OK, spec stable, alert still firing). Updated on every decay retry with current health score and retry count.
- `False/DecayResolved`: When the decay hypothesis is killed (alert clears, health degrades, or metrics gate), or when the assessment completes via a non-timeout path while decay was active.
- `False/DecayTimeout`: When the validity window expires while decay was actively being monitored (`AlertDecayRetries > 0` and `AlertAssessed == false`).

**Status Field**: `status.components.alertDecayRetries` (int32) tracks how many consecutive decay re-checks have occurred. When this field is `> 0` and the assessment completes, `setCompletionFields` resolves the condition.

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
    ConditionAssessmentComplete  = "AssessmentComplete"
    ConditionSpecIntegrity       = "SpecIntegrity"
    ConditionAlertDecayDetected  = "AlertDecayDetected"
)

// Reasons for AssessmentComplete
const (
    ReasonAssessmentFull       = "AssessmentFull"
    ReasonAssessmentPartial    = "AssessmentPartial"
    ReasonAssessmentExpired    = "AssessmentExpired"
    ReasonSpecDrift            = "SpecDrift"
    ReasonMetricsTimedOut      = "MetricsTimedOut"
    ReasonNoExecution          = "NoExecution"
    ReasonAlertDecayTimeout    = "AlertDecayTimeout"
)

// Reasons for SpecIntegrity
const (
    ReasonSpecUnchanged = "SpecUnchanged"
    ReasonSpecDrifted   = "SpecDrifted"
)

// Reasons for AlertDecayDetected
const (
    ReasonDecayActive   = "DecayActive"
    ReasonDecayResolved = "DecayResolved"
    ReasonDecayTimeout  = "DecayTimeout"
)
```

### Integration Points

| Integration Point | Conditions Set |
|-------------------|----------------|
| `reconciler.go:Step 6.5` (drift guard) | SpecIntegrity (True/False on each reconcile) |
| `reconciler.go:Step 6.5` (drift detected) | AssessmentComplete (SpecDrift) |
| `reconciler.go:completeAssessment` (all reasons) | AssessmentComplete (Full/Partial/Expired/etc.) |
| `reconciler.go:isAlertDecay` true branch (Point A) | AlertDecayDetected (True/DecayActive) |
| `reconciler.go:isAlertDecay` false branch (Point B) | AlertDecayDetected (False/DecayResolved) — only if `AlertDecayRetries > 0` |
| `reconciler.go:setCompletionFields` (Point C) | AlertDecayDetected (False/DecayResolved or False/DecayTimeout) — only if `AlertDecayRetries > 0` |

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
| CRD Schema | ✅ Implemented (`Conditions []metav1.Condition` in Status) |
| Helper functions | ✅ Implemented (`pkg/effectivenessmonitor/conditions/conditions.go`) |
| Controller integration | ✅ Implemented (`internal/controller/effectivenessmonitor/reconciler.go` — Ready, AssessmentComplete, SpecIntegrity, AlertDecayDetected all set) |
| Unit tests | ✅ Implemented (`test/unit/effectivenessmonitor/conditions_test.go`, `test/unit/effectivenessmonitor/alert_decay_test.go`) |

---

## References

- [DD-CRD-002](mdc:docs/architecture/decisions/DD-CRD-002-kubernetes-conditions-standard.md) (Parent)
- [DD-EM-002](mdc:docs/architecture/decisions/DD-EM-002-canonical-spec-hash.md) (Spec Drift Guard v1.1)
- [DD-CRD-002-RemediationRequest](mdc:docs/architecture/decisions/DD-CRD-002-remediationrequest-conditions.md) (Pattern reference)
- [ADR-EM-001](mdc:docs/architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md) (EM integration architecture)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-14 | Initial: Ready, AssessmentComplete, SpecIntegrity |
| 1.1 | 2026-03-04 | Added AlertDecayDetected condition (BR-EM-012, Issue #369). Added AlertDecayTimeout reason to AssessmentComplete. |
