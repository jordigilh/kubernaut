# 📋 REQUEST: SignalProcessing Status Schema - Required Fields Clarification

**From**: Remediation Orchestrator Team
**To**: SignalProcessing Team
**Date**: December 10, 2025
**Priority**: 🟡 MEDIUM
**Status**: ✅ **RESOLVED**

---

## 📋 Summary

RO integration tests fail when simulating SignalProcessing status updates because the SP CRD schema requires fields that aren't being populated.

---

## 🔴 Issue Details

### Error Message

```
SignalProcessing.signalprocessing.kubernaut.ai "sp-rr-phase-xxx" is invalid:
  [status.priorityAssignment.assignedAt: Required value,
   status.environmentClassification.classifiedAt: Required value]
```

### Context

RO creates SignalProcessing CRDs and waits for them to complete. In integration tests, we simulate SP completion by updating the SP status. However, the SP CRD schema requires these fields when updating status:

- `status.priorityAssignment.assignedAt` (Required)
- `status.environmentClassification.classifiedAt` (Required)

### Affected RO Tests

| Test | File | Line |
|------|------|------|
| Phase progression | `lifecycle_test.go` | 158 |
| ManualReview flow | `lifecycle_test.go` | 221, 290 |
| Approval flow | `lifecycle_test.go` | 365, 428 |

---

## ❓ Questions for SP Team

### Q1: Should these fields be required?

When a test or external system updates SP status to simulate completion, must it also provide:
- `status.priorityAssignment.assignedAt`
- `status.environmentClassification.classifiedAt`

**RO's Perspective**: These seem like SP-internal fields that SP controller should manage, not external consumers updating status.

### Q2: Recommended approach?

**Option A**: Make fields optional in SP CRD schema
- Pros: Simpler for consumers, follows Kubernetes conventions
- Cons: May require SP logic changes

**Option B**: SP provides test utilities
- SP team provides helper functions that set these required fields
- RO imports and uses these in integration tests

**Option C**: Document required fields for status updates
- SP documents what fields are required when updating status
- RO tests comply with these requirements

---

## 📊 Impact

| Metric | Value |
|--------|-------|
| RO Integration Tests Blocked | 4 tests |
| Severity | Medium (tests only, production RO doesn't update SP status) |

---

## 🔗 Related Documents

- RO Integration Tests: `test/integration/remediationorchestrator/lifecycle_test.go`
- SP CRD Schema: `api/signalprocessing/v1alpha1/signalprocessing_types.go`

---

## ✅ SP Team Response (December 10, 2025)

### Decision: **Option C** - Schema is Correct

The SP CRD schema is **correctly designed**. The issue is in RO test implementation.

### Schema Analysis

| Level | Field | Required? | Evidence |
|-------|-------|-----------|----------|
| **Status** | `environmentClassification` | ❌ Optional | Pointer with `omitempty` (line 174) |
| **Status** | `priorityAssignment` | ❌ Optional | Pointer with `omitempty` (line 175) |
| **EnvironmentClassification** | `classifiedAt` | ✅ Required | No `omitempty` (line 429) |
| **PriorityAssignment** | `assignedAt` | ✅ Required | No `omitempty` (line 445) |

### Answers to Questions

**Q1: Should these fields be required?**
> **YES** - When you set `PriorityAssignment` or `EnvironmentClassification`, you MUST set all their fields including timestamps. If you don't need these fields, leave the parent struct as `nil` (valid).

**Q2: Recommended approach?**
> **Option C** - RO tests must include timestamps when setting these structs.

### Fix for RO Integration Tests

```go
// WRONG: Missing timestamps
sp.Status.PriorityAssignment = &signalprocessingv1alpha1.PriorityAssignment{
    Priority:   "P1",
    Confidence: 0.9,
    Source:     "test",
}

// CORRECT: Include all required fields
sp.Status.PriorityAssignment = &signalprocessingv1alpha1.PriorityAssignment{
    Priority:   "P1",
    Confidence: 0.9,
    Source:     "test",
    AssignedAt: metav1.Now(),  // REQUIRED when struct is set
}

sp.Status.EnvironmentClassification = &signalprocessingv1alpha1.EnvironmentClassification{
    Environment:  "production",
    Confidence:   0.95,
    Source:       "test",
    ClassifiedAt: metav1.Now(),  // REQUIRED when struct is set
}
```

### Action Items

| # | Owner | Task | Status |
|---|-------|------|--------|
| 1 | **RO Team** | Update `updateSPStatus` helper to include `AssignedAt`/`ClassifiedAt` | ✅ Complete (2025-12-10) |
| 2 | **SP Team** | No action needed - schema is correct | ✅ Complete |

### RO Fix Applied

Updated `test/integration/remediationorchestrator/suite_test.go`:
```go
sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
    Environment:  "production",
    Confidence:   0.95,
    Source:       "test",
    ClassifiedAt: now, // REQUIRED per SP CRD schema
}
sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
    Priority:   "P1",
    Confidence: 0.90,
    Source:     "test",
    AssignedAt: now, // REQUIRED per SP CRD schema
}
```

---

**Document Version**: 1.1
**Created**: December 10, 2025
**Updated**: December 10, 2025 (SP response added)
**Maintained By**: Remediation Orchestrator Team + SignalProcessing Team
