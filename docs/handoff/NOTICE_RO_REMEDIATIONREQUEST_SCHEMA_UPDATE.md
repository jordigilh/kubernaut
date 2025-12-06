# NOTICE: RemediationRequest Schema Update Required

**Date**: 2025-12-06
**Version**: 1.1
**From**: Signal Processing (SP) Team
**To**: Remediation Orchestrator (RO) Team
**Status**: 🟢 **SP READY - RO MAY PROCEED**
**Priority**: HIGH
**Related**: [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md](./NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md)

---

## 📋 Summary

Following the Gateway team's removal of classification logic (see related notice), the `RemediationRequest` CRD schema must be updated. The `environment` and `priority` fields must be **removed from `RemediationRequestSpec`** because:

1. **Gateway** is no longer populating these fields
2. **SignalProcessing** now owns classification and populates its own `SignalProcessingStatus`
3. **RO** handles data flow transitions between services

---

## 🎯 Required Changes

### Remove from `RemediationRequestSpec`

```go
// api/remediation/v1alpha1/remediationrequest_types.go

type RemediationRequestSpec struct {
    // ... keep existing fields ...

    // ❌ REMOVE THESE FIELDS - Gateway no longer populates them
    // Environment string `json:"environment"`  // DELETE
    // Priority string `json:"priority"`        // DELETE

    // ... keep other fields ...
}
```

### Architecture Clarification

| Field | Old Location | New Location | Owner |
|-------|--------------|--------------|-------|
| `environment` | `RemediationRequestSpec` | `SignalProcessingStatus.EnvironmentClassification` | SP |
| `priority` | `RemediationRequestSpec` | `SignalProcessingStatus.PriorityAssignment` | SP |

---

## 📐 Data Flow Change

### Before (Gateway populated RR)
```
Gateway → RemediationRequest.Spec.Environment
Gateway → RemediationRequest.Spec.Priority
    ↓
RO reads from RR.Spec
```

### After (SP populates its own CRD)
```
Gateway → RemediationRequest (no env/priority)
    ↓
RO creates SignalProcessing CRD
    ↓
SP enriches SignalProcessingStatus.EnvironmentClassification
SP enriches SignalProcessingStatus.PriorityAssignment
    ↓
RO reads from SignalProcessingStatus for downstream services
```

---

## ✅ RO Team Checklist

### Required Actions

- [ ] **Acknowledge receipt** of this notice (update Status section below)
- [ ] **Remove fields** from `RemediationRequestSpec`:
  - [ ] Remove `Environment string` field
  - [ ] Remove `Priority string` field
- [ ] **Update authoritative doc** (`docs/architecture/CRD_SCHEMAS.md`)
- [ ] **Update RO controller** to read environment/priority from `SignalProcessingStatus`:
  ```go
  // Before: Read from RR spec
  env := rr.Spec.Environment
  priority := rr.Spec.Priority

  // After: Read from SignalProcessing status
  sp := &signalprocessingv1alpha1.SignalProcessing{}
  r.Get(ctx, types.NamespacedName{Name: rr.Status.SignalProcessingRef.Name, Namespace: rr.Status.SignalProcessingRef.Namespace}, sp)
  env := sp.Status.EnvironmentClassification.Environment
  priority := sp.Status.PriorityAssignment.Priority
  ```
- [ ] **Run `make generate manifests`** to regenerate CRD YAML
- [ ] **Update any downstream consumers** that expected these fields in RR spec
- [ ] **Signal completion** by updating this notice

---

## 📚 SignalProcessing Status Reference

SP will populate these fields in `SignalProcessingStatus`:

```go
// api/signalprocessing/v1alpha1/signalprocessing_types.go

type SignalProcessingStatus struct {
    // ... other fields ...

    // Environment classification result
    EnvironmentClassification *EnvironmentClassification `json:"environmentClassification,omitempty"`

    // Priority assignment result
    PriorityAssignment *PriorityAssignment `json:"priorityAssignment,omitempty"`
}

type EnvironmentClassification struct {
    // Detected environment: "production", "staging", "development", "test", "unknown"
    Environment string `json:"environment"`

    // Classification confidence (0.0 - 1.0)
    Confidence float64 `json:"confidence"`

    // Source of classification: "namespace-labels", "configmap", "signal-labels", "default"
    Source string `json:"source"`
}

type PriorityAssignment struct {
    // Assigned priority: "P0", "P1", "P2", "P3"
    Priority string `json:"priority"`

    // Assignment confidence (0.0 - 1.0)
    Confidence float64 `json:"confidence"`

    // Source of assignment: "rego-policy", "fallback-matrix", "default"
    Source string `json:"source"`
}
```

---

## 🔗 Related Documents

| Document | Description |
|----------|-------------|
| [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md](./NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md) | Gateway removing classification |
| [CRD_SCHEMAS.md](../architecture/CRD_SCHEMAS.md) | Authoritative CRD schema (needs update) |
| [BR-SP-051-053](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) | Environment classification BRs |
| [BR-SP-070-072](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) | Priority assignment BRs |

---

## 📝 Communication Log

| Date | Team | Action |
|------|------|--------|
| 2025-12-06 | SP | Created notice, awaiting RO acknowledgment |
| 2025-12-06 | SP | ✅ Day 4 Environment Classifier COMPLETE |
| 2025-12-06 | SP | ✅ Day 5 Priority Engine COMPLETE (ahead of schedule) |
| 2025-12-06 | Gateway | ⚠️ **Gateway proceeding with removal** - Fields will be empty in new CRDs |
| 2025-12-06 | RO | ✅ **ACKNOWLEDGED** - Impact assessed, implementation plan updated |
| 2025-12-06 | SP | 🟢 **SP READY** - Notified RO that all blockers cleared, may proceed with schema updates |

---

## 🔄 Status Updates

### RO Team Response

**Status**: ✅ **ACKNOWLEDGED**

```
Date: 2025-12-06
Acknowledged by: RO Team (AI Assistant)
Estimated completion: Day 4-5 of RO implementation
```

#### Impact Assessment

| Component | Current State | Required Change | Priority |
|-----------|---------------|-----------------|----------|
| `pkg/remediationorchestrator/creator/aianalysis.go` | Uses `rr.Spec.Environment`, `rr.Spec.Priority` | Read from `sp.Status.EnvironmentClassification.Environment`, `sp.Status.PriorityAssignment.Priority` | 🔴 HIGH |
| Day 4 Implementation Plan (12 references) | Uses `rr.Spec.Environment`, `rr.Spec.Priority` | Update all references to read from SignalProcessing status | 🔴 HIGH |
| Day 4 NotificationCreator | Uses `rr.Spec.Environment` in labels/metadata | Pass SP status to creators OR have reconciler fetch SP first | 🔴 HIGH |

#### Clarification Questions

1. **✅ CONFIRMED**: `Severity` remains on `RR.Spec` (not mentioned in removal list) - RO will continue using `rr.Spec.Severity`
2. **Question**: When Gateway removes these fields, will it set them to empty strings or omit them entirely?
3. **Question**: Should RO's SignalProcessingCreator still copy `Environment`/`Priority` to `SignalProcessing.Spec.Signal` if they're empty on RR?

#### Implementation Plan

1. **Update AIAnalysisCreator** (already implemented in Day 2):
   - Change from: `rr.Spec.Environment` / `rr.Spec.Priority`
   - Change to: `sp.Status.EnvironmentClassification.Environment` / `sp.Status.PriorityAssignment.Priority`

2. **Update NotificationCreator** (Day 4):
   - Pass SignalProcessing as additional parameter
   - OR have reconciler fetch SP before calling creators

3. **Update RemediationRequest Schema** (after SP Day 5 confirmed complete):
   - Remove `Environment` and `Priority` fields from `RemediationRequestSpec`
   - Run `make generate manifests`

#### Dependency Chain

```
Gateway removes fields → SP populates status → RO reads from SP status
                                    ↑
                          SP Day 5 COMPLETE ✅
```

**Blocker cleared**: SP Day 5 (Priority Engine) is complete. RO can proceed with schema update.

#### Timeline

| Task | Target | Status |
|------|--------|--------|
| Acknowledge notice | 2025-12-06 | ✅ Complete |
| Update AIAnalysisCreator to read from SP | Day 4 | ⏳ Pending |
| Update Day 4 plan to read from SP | Day 4 | ⏳ Pending |
| Remove fields from RR spec | After RO Day 4 | ⏳ Pending |
| Integration test with SP | Day 5+ | ⏳ Pending |

---

## ✅ Timeline

| Milestone | Target Date | Status |
|-----------|-------------|--------|
| SP acknowledges Gateway notice | 2025-12-06 | ✅ Complete |
| SP Day 4 (Environment Classifier) | 2025-12-06 | ✅ **COMPLETE** |
| SP Day 5 (Priority Engine) | 2025-12-06 | ✅ **COMPLETE** (ahead of schedule) |
| RO acknowledges this notice | 2025-12-06 | ✅ Complete |
| RO removes fields from spec | TBD | 🟢 **UNBLOCKED** |
| RO updates controller to read from SP status | TBD | 🟢 **UNBLOCKED** |
| Full integration tested | TBD | ⏳ Pending |

**✅ SP READY**: Signal Processing environment and priority classification is fully implemented. RO team may proceed with schema updates and field removal at their convenience.

### SP Implementation Summary

| Component | Status | Location |
|-----------|--------|----------|
| Environment Classifier (Rego) | ✅ | `pkg/signalprocessing/classifier/environment.go` |
| Priority Engine (Rego) | ✅ | `pkg/signalprocessing/classifier/priority.go` |
| ConfigMap Hot-Reload | ✅ | `pkg/shared/hotreload/file_watcher.go` |
| Rego Policies | ✅ | `deploy/signalprocessing/policies/` |

---

## ❓ Questions / Feedback

Contact Signal Processing team or raise in `#kubernaut-dev` channel.

---

**Document Version**: 1.1
**Last Updated**: 2025-12-06
**Status**: 🟢 **SP IMPLEMENTATION COMPLETE** - RO team may proceed with schema updates

