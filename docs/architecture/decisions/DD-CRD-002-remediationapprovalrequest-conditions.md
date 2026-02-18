# DD-CRD-002-RemediationApprovalRequest: Kubernetes Conditions for RemediationApprovalRequest CRD

**Status**: ✅ APPROVED
**Version**: 1.0
**Date**: December 16, 2025
**CRD**: RemediationApprovalRequest
**Service**: RemediationOrchestrator
**Parent Standard**: DD-CRD-002

---

## 📋 Overview

This document specifies the Kubernetes Conditions for the **RemediationApprovalRequest** CRD per DD-CRD-002 standard.

---

## 🎯 Condition Types (4)

| Condition Type | Purpose | Set By |
|----------------|---------|--------|
| `Ready` | Aggregate: True on Approved/Rejected, False on Expired | Controller |
| `ApprovalPending` | Approval awaiting decision | Controller |
| `ApprovalDecided` | Decision made (approved/rejected) | Controller |
| `ApprovalExpired` | Timeout before decision | Controller |

---

## 📊 Condition Specifications

### ApprovalPending

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `AwaitingDecision` | "Waiting for approval decision (expires in {remaining})" |
| `False` | `DecisionMade` | "Approval decision received" |

### ApprovalDecided

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `Approved` | "Workflow approved by {approver}" |
| `True` | `Rejected` | "Workflow rejected by {approver}: {reason}" |
| `False` | `PendingDecision` | "No decision yet" |

### ApprovalExpired

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `Timeout` | "Approval expired after {duration} without decision" |
| `False` | `NotExpired` | "Approval has not expired" |

### Ready

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `Ready` | "Approval decided (approved or rejected)" |
| `False` | `NotReady` | "Approval expired without decision" |

**When Set**: True when Approved or Rejected; False when Expired.

---

## 🔄 State Transitions

```
+-------------------+     +-------------------+
|  ApprovalPending  | --> | ApprovalDecided   |
|  Status: True     |     |  Status: True     |
|  Reason:Awaiting  |     |  Reason:Approved  |
+-------------------+     +-------------------+
         |
         v
+-------------------+
| ApprovalExpired   |
| Status: True      |
| Reason: Timeout   |
+-------------------+
```

---

## 🔧 Implementation

**Helper File**: `pkg/remediationapprovalrequest/conditions.go`

**MANDATORY**: Use canonical Kubernetes functions per DD-CRD-002 v1.2:
- `meta.SetStatusCondition()` for setting conditions
- `meta.FindStatusCondition()` for reading conditions

---

## ✅ Validation

```bash
kubectl explain remediationapprovalrequest.status.conditions
kubectl describe remediationapprovalrequest rar-test-123 | grep -A10 "Conditions:"
kubectl wait --for=condition=ApprovalDecided rar/rar-test-123 --timeout=30m
```

---

## 🏗️ Status

| Component | Status |
|-----------|--------|
| CRD Schema | ✅ Exists (line 219) |
| Helper functions | ⏳ Pending |
| Controller integration | ⏳ Pending |
| Unit tests | ⏳ Pending |

---

## 🔗 References

- [DD-CRD-002](mdc:docs/architecture/decisions/DD-CRD-002-kubernetes-conditions-standard.md) (Parent)
- [DD-CRD-002-RemediationRequest](mdc:docs/architecture/decisions/DD-CRD-002-remediationrequest-conditions.md)

