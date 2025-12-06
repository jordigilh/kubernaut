# NOTICE: RemediationRequest Schema Update Required

**Date**: 2025-12-06
**Version**: 1.0
**From**: Signal Processing (SP) Team
**To**: Remediation Orchestrator (RO) Team
**Status**: 🔴 **ACTION REQUIRED**
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
| 2025-12-06 | SP | ✅ Day 5 Priority Engine COMPLETE (ahead of schedule) |
| 2025-12-06 | Gateway | ⚠️ **Gateway proceeding with removal** - Fields will be empty in new CRDs |
| - | RO | ⏳ Pending acknowledgment |

---

## 🔄 Status Updates

### RO Team Response

**Status**: ⏳ **PENDING ACKNOWLEDGMENT**

```
Date: -
Acknowledged by: -
Estimated completion: -
```

---

## ⚠️ Timeline

| Milestone | Target Date | Status |
|-----------|-------------|--------|
| SP acknowledges Gateway notice | 2025-12-06 | ✅ Complete |
| RO acknowledges this notice | TBD | ⏳ Pending |
| RO removes fields from spec | TBD | ⏳ Pending |
| RO updates controller to read from SP status | TBD | ⏳ Pending |
| SP Day 5 (Priority Engine) complete | 2025-12-06 | ✅ **COMPLETE** (ahead of schedule) |
| Full integration tested | TBD | ⏳ Pending |

**BLOCKING**: RO should not merge field removal until SP Day 5 (Priority Engine) is complete, ensuring SP can populate both fields.

---

## ❓ Questions / Feedback

Contact Signal Processing team or raise in `#kubernaut-dev` channel.

---

**Document Version**: 1.0
**Last Updated**: 2025-12-06

