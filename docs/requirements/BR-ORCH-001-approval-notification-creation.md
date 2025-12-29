# BR-ORCH-001: Approval Notification Creation

**Service**: RemediationOrchestrator Controller
**Category**: V1.0 Core Requirements
**Priority**: P0 (CRITICAL)
**Version**: 1.0
**Date**: 2025-12-02
**Status**: 🚧 Planned
**Related ADRs**: ADR-018 (Approval Notification V1.0 Integration), ADR-017 (NotificationRequest Creator)

---

## Overview

RemediationOrchestrator MUST create NotificationRequest CRDs when AIAnalysis enters the "Approving" phase (confidence between 60-79%), alerting operators that manual approval is required before workflow execution.

**Business Value**: Reduces approval miss rate from 40-60% (manual polling) to <5% (push notifications), enabling $392K savings per approval-required incident in large enterprises.

---

## BR-ORCH-001: Approval Notification Creation

### Description

When AIAnalysis requires manual approval (confidence 60-79%), RemediationOrchestrator creates a NotificationRequest CRD to push notifications to operators via configured channels (Slack, Console, etc.).

### Priority

**P0 (CRITICAL)** - Core V1.0 feature for approval workflow

### Rationale

Without push notifications:
- Operators must manually poll: `kubectl get aiapprovalrequest --watch`
- 40-60% approval miss rate (operators miss pending approvals)
- 30-40% timeout rate (15-minute default approval timeout)
- MTTR degradation: 60+ minutes for manual intervention

With push notifications:
- <5% approval miss rate (operators receive immediate alerts)
- <10% timeout rate (operators notified promptly)
- MTTR improvement: 5 minutes average for approval-required incidents

### Implementation

1. Watch AIAnalysis CRD status changes
2. When `AIAnalysis.status.phase == "Approving"`:
   - Check `RemediationRequest.status.approvalNotificationSent == false` (idempotency)
   - Extract approval context from `AIAnalysis.status.approvalContext`
   - Create NotificationRequest CRD with:
     - Investigation summary
     - Evidence collected
     - Recommended actions with rationales
     - Why approval is required
     - Links to approve/reject
   - Set `RemediationRequest.status.approvalNotificationSent = true`
3. Set OwnerReference to RemediationRequest for cascade deletion

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-001-1 | NotificationRequest created when AIAnalysis enters "Approving" phase | Unit, Integration |
| AC-001-2 | Only ONE notification per approval request (idempotency) | Unit |
| AC-001-3 | Notification contains complete approval context | Unit |
| AC-001-4 | OwnerReference set for cascade deletion | Unit |
| AC-001-5 | End-to-end latency <5 seconds from AIAnalysis "Approving" to notification | E2E |
| AC-001-6 | `approvalNotificationSent` flag prevents duplicate notifications | Unit |

### Test Scenarios

```gherkin
Scenario: Approval notification created
  Given RemediationRequest "rr-1" exists with AIAnalysis "aia-1"
  And AIAnalysis "aia-1" transitions to phase "Approving"
  And RemediationRequest "rr-1" has approvalNotificationSent = false
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created for "rr-1"
  And NotificationRequest should contain approval context
  And RemediationRequest "rr-1" should have approvalNotificationSent = true

Scenario: Duplicate notification prevented (idempotency)
  Given RemediationRequest "rr-1" has approvalNotificationSent = true
  And AIAnalysis "aia-1" is in phase "Approving"
  When RemediationOrchestrator reconciles "rr-1" again
  Then NO new NotificationRequest should be created
  And existing NotificationRequest count should remain 1

Scenario: Cascade deletion cleans up notification
  Given RemediationRequest "rr-1" exists with NotificationRequest "nr-1"
  When RemediationRequest "rr-1" is deleted
  Then NotificationRequest "nr-1" should be automatically deleted
```

---

## Performance Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **CRD Watch Latency** | <500ms | From AIAnalysis status update to RO reconciliation |
| **Notification Creation Time** | <2 seconds | From approval phase detection to NotificationRequest creation |
| **End-to-End Latency** | <5 seconds | From AIAnalysis "Approving" to operator notification delivery |
| **Approval Miss Rate** | <5% | Percentage of approvals not acted upon |

---

## Related Documents

- [ADR-018: Approval Notification V1.0 Integration](../architecture/decisions/ADR-018-approval-notification-v1-integration.md)
- [ADR-017: NotificationRequest Creator](../architecture/decisions/ADR-017-notification-crd-creator.md)
- [RemediationOrchestrator BUSINESS_REQUIREMENTS.md](../services/crd-controllers/05-remediationorchestrator/BUSINESS_REQUIREMENTS.md)
- [reconciliation-phases.md#phase-3.5](../services/crd-controllers/05-remediationorchestrator/reconciliation-phases.md)

---

**Document Version**: 1.0
**Last Updated**: December 2, 2025
**Maintained By**: Kubernaut Architecture Team


