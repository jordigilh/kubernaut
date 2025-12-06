# BR-ORCH-036: Manual Review Notification

**Service**: RemediationOrchestrator Controller
**Category**: V1.0 Core Requirements
**Priority**: P0 (CRITICAL)
**Version**: 1.0
**Date**: 2025-12-06
**Status**: 🚧 Planned
**Related BRs**: BR-ORCH-032 (WE Skip Handling), BR-ORCH-001 (Approval Notification)
**Related DDs**: DD-WE-004 (Exponential Backoff Cooldown)

---

## Overview

RemediationOrchestrator MUST create NotificationRequest CRDs with `type=manual-review` when WorkflowExecution enters a state requiring operator intervention due to exhausted retries or previous execution failures.

**Business Value**: Ensures operators are immediately notified of remediation failures that cannot be automatically resolved, reducing MTTR for critical infrastructure issues by 40-60%.

---

## BR-ORCH-036: Manual Review Notification

### Description

When WorkflowExecution is skipped due to `ExhaustedRetries` or `PreviousExecutionFailed`, RemediationOrchestrator creates a NotificationRequest CRD with `type=manual-review` to alert operators that manual intervention is required.

### Priority

**P0 (CRITICAL)** - Core V1.0 feature for failure escalation

### Rationale

**Why a distinct notification type (`manual-review`) instead of `escalation`?**

| Type | Purpose | Operator Action |
|------|---------|-----------------|
| `escalation` | General failures, timeouts | Investigate and possibly retry |
| `manual-review` | Pre-execution exhaustion, prior execution failures | **Must clear backoff state or investigate cluster** |

**Distinct routing enables:**
- Different notification channels (e.g., PagerDuty for manual-review)
- Different priorities based on failure type
- Label-based routing rules (BR-NOT-065): `kubernaut.ai/notification-type=manual-review`
- Separate metrics and dashboards

### Trigger Conditions

| Skip Reason | Description | Notification Type |
|-------------|-------------|-------------------|
| `ExhaustedRetries` | 5+ consecutive pre-execution failures | `manual-review` |
| `PreviousExecutionFailed` | Prior workflow execution failed during run | `manual-review` |

### Implementation

1. Watch `WorkflowExecution.status.phase` for `Skipped` or `Failed`
2. Extract skip reason from `status.skipDetails.reason` or failure details
3. If reason is `ExhaustedRetries` or `PreviousExecutionFailed`:
   - Create NotificationRequest with `type=manual-review`
   - Set `priority=critical`
   - Include `kubernaut.ai/skip-reason` label for routing
   - Include context from WE's `SkipDetails.Message` or `FailureDetails.NaturalLanguageSummary`
4. Set OwnerReference to RemediationRequest for cascade deletion
5. Do NOT requeue - wait for operator intervention

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-036-1 | NotificationRequest created with `type=manual-review` for `ExhaustedRetries` | Unit |
| AC-036-2 | NotificationRequest created with `type=manual-review` for `PreviousExecutionFailed` | Unit |
| AC-036-3 | Notification priority is `critical` | Unit |
| AC-036-4 | Notification includes `kubernaut.ai/skip-reason` label | Unit |
| AC-036-5 | Notification body includes WE failure context | Unit |
| AC-036-6 | OwnerReference set for cascade deletion (BR-ORCH-031) | Unit |
| AC-036-7 | Notification reference tracked in RR status (BR-ORCH-035) | Unit |
| AC-036-8 | Distinct from `approval` notifications (different routing) | Unit |
| AC-036-9 | End-to-end latency <5 seconds from WE skip to notification | Integration |

### Test Scenarios

```gherkin
Scenario: Manual review notification for ExhaustedRetries
  Given WorkflowExecution "we-1" is Skipped with reason "ExhaustedRetries"
  And SkipDetails.Message contains "5 consecutive pre-execution failures"
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created with:
    | type | manual-review |
    | priority | critical |
    | labels.kubernaut.ai/skip-reason | ExhaustedRetries |
  And notification body should contain:
    | Content | "5 consecutive pre-execution failures" |
    | Content | "Manual intervention required" |
  And RemediationRequest "rr-1" phase should be "Failed"
  And RemediationRequest "rr-1" should have requiresManualReview = true

Scenario: Manual review notification for PreviousExecutionFailed
  Given WorkflowExecution "we-1" is Skipped with reason "PreviousExecutionFailed"
  And SkipDetails.Message contains "cluster state may be inconsistent"
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created with:
    | type | manual-review |
    | priority | critical |
    | labels.kubernaut.ai/skip-reason | PreviousExecutionFailed |
  And notification body should contain:
    | Content | "Previous execution failed" |
    | Content | "cluster state may be inconsistent" |

Scenario: Manual review notification distinct from approval
  Given AIAnalysis "ai-1" requires approval (confidence 72%)
  And WorkflowExecution "we-2" is Skipped with reason "ExhaustedRetries"
  When both reconciliations complete
  Then two distinct NotificationRequests should exist:
    | Name | Type | Routing Label |
    | nr-approval-rr-1 | approval | notification-type=approval |
    | nr-manual-review-rr-2 | manual-review | notification-type=manual-review |
```

---

## Notification Content Template

```yaml
kind: NotificationRequest
metadata:
  name: nr-manual-review-{rr-name}
  namespace: {namespace}
  labels:
    kubernaut.ai/remediation-request: {rr-name}
    kubernaut.ai/notification-type: manual-review
    kubernaut.ai/skip-reason: {ExhaustedRetries|PreviousExecutionFailed}
    kubernaut.ai/severity: critical
    kubernaut.ai/environment: {environment}
    kubernaut.ai/component: remediation-orchestrator
spec:
  type: manual-review
  priority: critical
  subject: "⚠️ Manual Review Required: {rr-name} - {skip-reason}"
  body: |
    Remediation requires manual intervention:

    **Signal**: {signalName}
    **Target**: {namespace}/{kind}/{name}
    **Environment**: {environment}

    **Reason**: {skip-reason}
    **Details**: {WE.SkipDetails.Message or FailureDetails.NaturalLanguageSummary}

    **Action Required**:
    - For ExhaustedRetries: Clear backoff state or investigate root cause
    - For PreviousExecutionFailed: Verify cluster state before retry

    **Consecutive Failures**: {ConsecutiveFailures}
    **Next Allowed Execution**: {NextAllowedExecution}
  channels:
    - console
    - slack
    - email  # Critical = all channels
  metadata:
    remediationRequest: {rr-name}
    workflowExecution: {we-name}
    skipReason: {skip-reason}
    consecutiveFailures: "{N}"
    environment: {environment}
```

---

## API Change

### NotificationType Enum Addition

**File**: `api/notification/v1alpha1/notificationrequest_types.go`

```go
// +kubebuilder:validation:Enum=escalation;simple;status-update;approval;manual-review
type NotificationType string

const (
    NotificationTypeEscalation   NotificationType = "escalation"
    NotificationTypeSimple       NotificationType = "simple"
    NotificationTypeStatusUpdate NotificationType = "status-update"
    NotificationTypeApproval     NotificationType = "approval"      // BR-ORCH-001
    NotificationTypeManualReview NotificationType = "manual-review" // BR-ORCH-036 (this BR)
)
```

---

## Related Documents

- [BR-ORCH-032: Handle WE Skipped Phase](./BR-ORCH-032-034-resource-lock-deduplication.md)
- [BR-ORCH-001: Approval Notification Creation](./BR-ORCH-001-approval-notification-creation.md)
- [BR-ORCH-035: Notification Reference Tracking](./BR-ORCH-035-notification-reference-tracking.md)
- [DD-WE-004: Exponential Backoff Cooldown](../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md)
- [NOTICE: WE Exponential Backoff](../handoff/NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-06 | Initial BR creation based on DD-WE-004 requirements and cross-team alignment |

---

**Document Version**: 1.0
**Last Updated**: December 6, 2025
**Maintained By**: Kubernaut Architecture Team

