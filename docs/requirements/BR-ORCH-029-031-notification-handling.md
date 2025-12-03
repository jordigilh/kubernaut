# BR-ORCH-029/030/031: Notification Handling

**Service**: RemediationOrchestrator Controller
**Category**: Notification Handling
**Priority**: P0/P1 (CRITICAL/HIGH)
**Version**: 1.0
**Date**: 2025-12-02
**Status**: 🚧 Planned
**Design Decision**: [DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md](../services/crd-controllers/05-remediationorchestrator/DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md)

---

## Overview

This document consolidates three related business requirements for notification handling in RemediationOrchestrator:
1. **BR-ORCH-029** (P0): User-initiated notification cancellation
2. **BR-ORCH-030** (P1): Notification status tracking
3. **BR-ORCH-031** (P1): Cascade cleanup for child NotificationRequest CRDs

**Key Design Decision**: User deletion of NotificationRequest is intentional cancellation (not failure). RO must distinguish user-initiated cancellation from system failures.

---

## BR-ORCH-029: User-Initiated Notification Cancellation

### Description

RemediationOrchestrator MUST treat user deletion of NotificationRequest CRDs as intentional cancellation (not system failure), marking RemediationRequest as `Completed` with a cancellation condition rather than `Failed`.

### Priority

**P0 (CRITICAL)** - Prevents false positive escalations

### Rationale

Per DD-NOT-005, NotificationRequest spec is immutable, so users can only cancel notifications by deleting the CRD. RO must:
- Distinguish user-initiated cancellation from system failures
- Prevent false positive escalations
- Provide accurate audit trail
- Support operator workflow interruption

### Implementation

1. Watch NotificationRequest CRDs via owner reference pattern
2. Detect `NotFound` errors during reconciliation
3. Distinguish cascade deletion from user cancellation:
   - Cascade: RemediationRequest has `deletionTimestamp`
   - User: NotificationRequest deleted independently
4. On user cancellation:
   - Set `status.phase = Completed` (NOT Failed)
   - Set `status.notificationStatus = "Cancelled"`
   - Add condition: `NotificationDelivered=False` with reason `UserCancelled`
   - DO NOT trigger escalation workflows

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-029-1 | User deletion of NotificationRequest detected via watch | Unit, Integration |
| AC-029-2 | RemediationRequest marked as `Completed` (not `Failed`) on user cancellation | Unit |
| AC-029-3 | Condition `NotificationDelivered=False` with reason `UserCancelled` set | Unit |
| AC-029-4 | No automatic escalation triggered for user cancellations | Integration |
| AC-029-5 | Cascade deletion handled gracefully without warnings | Integration |
| AC-029-6 | Audit trail clearly indicates user-initiated cancellation | Unit |

### Test Scenarios

```gherkin
Scenario: User cancels notification
  Given RemediationRequest "rr-1" exists with NotificationRequest "nr-1"
  And RemediationRequest "rr-1" has no deletionTimestamp
  When user deletes NotificationRequest "nr-1"
  Then RemediationRequest "rr-1" phase should be "Completed"
  And status.notificationStatus should be "Cancelled"
  And condition NotificationDelivered should be False with reason "UserCancelled"
  And NO escalation notification should be created

Scenario: Cascade deletion (not user cancellation)
  Given RemediationRequest "rr-1" exists with NotificationRequest "nr-1"
  When RemediationRequest "rr-1" is deleted
  Then NotificationRequest "nr-1" should be deleted via cascade
  And NO "UserCancelled" condition should be set
```

---

## BR-ORCH-030: Notification Status Tracking

### Description

RemediationOrchestrator MUST track NotificationRequest delivery status and propagate it to RemediationRequest status for observability, enabling SREs to query remediation status including notification outcomes.

### Priority

**P1 (HIGH)** - Enables complete workflow observability

### Rationale

Notification delivery is a critical part of the remediation workflow. Tracking enables:
- Complete workflow observability
- Querying by notification outcome
- SLO tracking for notification delivery
- Debugging delivery failures

### Implementation

1. Watch NotificationRequest status updates
2. Update `status.notificationStatus` based on NotificationRequest phase:
   - `Pending` → `notificationStatus = "Pending"`
   - `Sending` → `notificationStatus = "InProgress"`
   - `Sent` → `notificationStatus = "Sent"`, condition `NotificationDelivered=True`
   - `Failed` → `notificationStatus = "Failed"`, condition `NotificationDelivered=False` with reason `DeliveryFailed`
   - `Deleted` → `notificationStatus = "Cancelled"`, condition `NotificationDelivered=False` with reason `UserCancelled`
3. Store NotificationRequest name in `status.notificationRequestName`

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-030-1 | RO watches NotificationRequest status updates | Integration |
| AC-030-2 | `status.notificationStatus` updated based on NotificationRequest phase | Unit |
| AC-030-3 | `NotificationDelivered` condition set with accurate reason | Unit |
| AC-030-4 | SREs can query RemediationRequests by notification status | Integration |
| AC-030-5 | Metrics expose notification status distribution | Unit |

### Test Scenarios

```gherkin
Scenario: Notification status tracking - sent
  Given RemediationRequest "rr-1" has NotificationRequest "nr-1"
  When NotificationRequest "nr-1" transitions to phase "Sent"
  Then RemediationRequest "rr-1" should have notificationStatus = "Sent"
  And condition NotificationDelivered should be True

Scenario: Notification status tracking - failed
  Given RemediationRequest "rr-1" has NotificationRequest "nr-1"
  When NotificationRequest "nr-1" transitions to phase "Failed"
  Then RemediationRequest "rr-1" should have notificationStatus = "Failed"
  And condition NotificationDelivered should be False with reason "DeliveryFailed"
```

---

## BR-ORCH-031: Cascade Cleanup for Child NotificationRequest CRDs

### Description

RemediationOrchestrator MUST set owner references on NotificationRequest CRDs to enable automatic cascade deletion when RemediationRequest is deleted, preventing orphaned notification CRDs.

### Priority

**P1 (HIGH)** - Prevents resource leaks

### Rationale

Kubernetes owner references provide:
- Automatic cleanup of child resources when parent is deleted
- Prevention of orphaned NotificationRequest CRDs
- Consistent resource lifecycle management
- Correct garbage collection semantics

### Implementation

1. Set `ownerReferences` on NotificationRequest during creation:
   - `apiVersion`: RemediationRequest API version
   - `kind`: "RemediationRequest"
   - `name`: RemediationRequest name
   - `uid`: RemediationRequest UID
   - `controller: true`: RO is the controlling owner
   - `blockOwnerDeletion: false`: Allow independent NotificationRequest deletion (for user cancellation)
2. Kubernetes automatically deletes NotificationRequest when RemediationRequest is deleted
3. RO detects cascade deletion (RemediationRequest has `deletionTimestamp`) vs user cancellation

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-031-1 | NotificationRequest has ownerReference to RemediationRequest | Unit |
| AC-031-2 | `blockOwnerDeletion = false` allows independent user deletion | Unit |
| AC-031-3 | Deleting RemediationRequest automatically deletes NotificationRequest | Integration |
| AC-031-4 | No orphaned NotificationRequest CRDs remain after RR deletion | Integration |
| AC-031-5 | RO distinguishes cascade deletion from user cancellation | Unit |

### Test Scenarios

```gherkin
Scenario: Owner reference set correctly
  When RemediationOrchestrator creates NotificationRequest "nr-1" for "rr-1"
  Then NotificationRequest "nr-1" should have ownerReference to "rr-1"
  And ownerReference.controller should be true
  And ownerReference.blockOwnerDeletion should be false

Scenario: Cascade deletion works
  Given RemediationRequest "rr-1" exists with NotificationRequest "nr-1"
  When RemediationRequest "rr-1" is deleted
  Then NotificationRequest "nr-1" should be automatically deleted by Kubernetes GC
  And NO orphaned NotificationRequest should exist
```

---

## Related Documents

- [DD-NOT-005: NotificationRequest Spec Immutability](../services/crd-controllers/06-notification/DD-NOT-005-SPEC-IMMUTABILITY.md)
- [DD-RO-001: Notification Cancellation Handling](../services/crd-controllers/05-remediationorchestrator/DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md)
- [ADR-001: CRD Microservices Architecture (Owner References)](../architecture/decisions/ADR-001-crd-microservices-architecture.md)
- [ADR-017: NotificationRequest Creator](../architecture/decisions/ADR-017-notification-crd-creator.md)

---

**Document Version**: 1.0
**Last Updated**: December 2, 2025
**Maintained By**: Kubernaut Architecture Team


