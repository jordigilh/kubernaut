# BR-ORCH-035: Notification Reference Tracking

**Service**: RemediationOrchestrator Controller
**Category**: V1.0 Core Requirements
**Priority**: P1 (HIGH)
**Version**: 1.0
**Date**: 2025-12-06
**Status**: 🚧 Planned
**Related BRs**: BR-ORCH-001 (Approval Notification), BR-ORCH-029 (Completion Notification), BR-ORCH-030 (Failure Notification)

---

## Overview

RemediationOrchestrator MUST track references to all NotificationRequest CRDs created during the remediation lifecycle in `RemediationRequest.status.notificationRequestRefs`.

**Business Value**: Enables instant audit trail visibility for compliance, reduces incident investigation time from minutes to seconds, and provides self-documenting evidence of user-facing communications.

---

## BR-ORCH-035: Notification Reference Tracking

### Description

When RemediationOrchestrator creates any NotificationRequest CRD (approval, completion, failure, timeout), it MUST append the notification reference to `RemediationRequest.status.notificationRequestRefs[]`.

### Priority

**P1 (HIGH)** - Critical for compliance audit trail and incident investigation

### Rationale

**Without notification reference tracking:**
- Operators must query notifications separately via field selector: `kubectl get notificationrequests --field-selector spec.remediationRequestRef.name=<name>` (Issue #91: spec.remediationRequestRef replaces label)
- Compliance audits require manual correlation between RR and notification logs
- Incident investigation: "Did we notify?" requires multiple queries
- Dashboard/UI requires N+1 API calls to show complete remediation story

**With notification reference tracking:**
- Single `kubectl get rr/<name> -o yaml` shows all notifications
- Self-documenting evidence for compliance audits
- Instant answer to "What notifications were sent?"
- Dashboard/UI needs single API call for complete view

**Key insight**: Notifications are the ONLY user-facing output of the remediation pipeline. If we track internal CRDs (SignalProcessing, AIAnalysis, WorkflowExecution) but not user-facing notifications, we're tracking the wrong thing from a business perspective.

### Acceptance Criteria

1. **AC-1**: When NotificationRequest is created for approval (BR-ORCH-001), reference MUST be appended to `notificationRequestRefs`
2. **AC-2**: When NotificationRequest is created for completion (BR-ORCH-029), reference MUST be appended to `notificationRequestRefs`
3. **AC-3**: When NotificationRequest is created for failure (BR-ORCH-030), reference MUST be appended to `notificationRequestRefs`
4. **AC-4**: When NotificationRequest is created for timeout, reference MUST be appended to `notificationRequestRefs`
5. **AC-5**: `notificationRequestRefs` MUST be a slice to support multiple notifications per remediation
6. **AC-6**: Each reference MUST include Name, Namespace, and UID for unambiguous identification

### Implementation

#### Schema Change

```go
// RemediationRequestStatus
type RemediationRequestStatus struct {
    // ... existing fields ...

    // NotificationRequestRefs tracks all notification CRDs created for this remediation.
    // Provides audit trail for compliance and instant visibility for debugging.
    // Reference: BR-ORCH-035
    // +optional
    NotificationRequestRefs []corev1.ObjectReference `json:"notificationRequestRefs,omitempty"`
}
```

#### Update Pattern (in NotificationRequestCreator)

```go
// After creating notification
rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, corev1.ObjectReference{
    APIVersion: notificationv1.GroupVersion.String(),
    Kind:       "NotificationRequest",
    Name:       nr.Name,
    Namespace:  nr.Namespace,
    UID:        nr.UID,
})
```

### Test Requirements

| Test Case | BR Reference | Expected Outcome |
|-----------|--------------|------------------|
| Approval notification adds ref | BR-ORCH-035, AC-1 | Ref appended to slice |
| Completion notification adds ref | BR-ORCH-035, AC-2 | Ref appended to slice |
| Failure notification adds ref | BR-ORCH-035, AC-3 | Ref appended to slice |
| Multiple notifications tracked | BR-ORCH-035, AC-5 | Slice contains all refs in order |
| Ref includes UID | BR-ORCH-035, AC-6 | UID populated for each ref |

### Dependencies

- BR-ORCH-001: Approval notification creation
- BR-ORCH-029: Completion notification creation
- BR-ORCH-030: Failure notification creation
- NotificationRequest CRD schema

### Success Metrics

- **Audit query time**: <1 second (single kubectl command)
- **Compliance evidence completeness**: 100% of notifications tracked
- **Dashboard API calls**: Reduced from N+1 to 1 per remediation view

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-06 | AI Assistant | Initial BR creation based on business value analysis |



