# BR-ORCH-032/033/034: Resource Lock Deduplication Handling

**Service**: RemediationOrchestrator Controller
**Category**: Resource Lock Deduplication
**Priority**: P0/P1 (CRITICAL/HIGH)
**Version**: 1.0
**Date**: 2025-12-02
**Status**: 🚧 Planned
**Design Decision**: [DD-RO-001-resource-lock-deduplication-handling.md](../architecture/decisions/DD-RO-001-resource-lock-deduplication-handling.md)

---

## Overview

This document consolidates three related business requirements for handling WorkflowExecution resource lock deduplication in RemediationOrchestrator:
1. **BR-ORCH-032** (P0): Handle WE Skipped Phase
2. **BR-ORCH-033** (P1): Track Duplicate Remediations
3. **BR-ORCH-034** (P1): Bulk Notification for Duplicates

**Context**: Kubernaut implements multi-layer deduplication:
- **Layer 1 (Gateway)**: Fingerprint deduplication - same fingerprint → update occurrence count
- **Layer 2 (Gateway)**: Storm aggregation - threshold >5 → aggregate into 1 RR
- **Layer 3 (WE)**: Resource locking - different fingerprints, same target → Skipped phase

These BRs handle **Layer 3** scenarios where WorkflowExecution returns `Skipped` phase.

---

## BR-ORCH-032: Handle WE Skipped Phase

### Description

RemediationOrchestrator MUST watch WorkflowExecution status and handle the `Skipped` phase when WE's resource locking mechanism prevents execution due to `ResourceBusy`, `RecentlyRemediated`, `ExhaustedRetries`, or `PreviousExecutionFailed` reasons.

### Priority

**P0 (CRITICAL)** - Core response to WE resource locking (DD-WE-001)

### Rationale

WorkflowExecution implements resource-level locking (DD-WE-001) to prevent:
- Parallel workflow executions on the same Kubernetes resource
- Redundant sequential executions within cooldown period
- Execution on resources with unresolved prior failures

When WE skips execution, RO must:
- Update RemediationRequest status accordingly
- Track the relationship with the active remediation (for duplicates)
- Provide clear audit trail for skipped remediations
- Handle `PreviousExecutionFailed` differently (NOT a duplicate)

### Implementation

1. Watch `WorkflowExecution.status.phase` for `Skipped` value
2. Extract skip reason from `status.skipDetails.reason`:
   - `ResourceBusy`: Another workflow executing on same target
   - `RecentlyRemediated`: Target recently remediated (cooldown period active)
   - `ExhaustedRetries`: 5+ pre-execution failures (exponential backoff exhausted)
   - `PreviousExecutionFailed`: Prior execution failed during execution (cluster state unknown)
3. **Per-reason handling**:

| Skip Reason | Is Duplicate? | Track on Parent? | Notification | Requeue? |
|-------------|---------------|------------------|--------------|----------|
| `ResourceBusy` | ✅ Yes | ✅ Yes | Bulk (BR-ORCH-034) | ✅ 30s |
| `RecentlyRemediated` | ✅ Yes | ✅ Yes | Bulk (BR-ORCH-034) | ✅ Use `NextAllowedExecution` |
| `ExhaustedRetries` | ❌ **No** | ❌ **No** | Individual (escalation) | ❌ Manual review |
| `PreviousExecutionFailed` | ❌ **No** | ❌ **No** | Individual (escalation) | ❌ Manual review |

4. For `ResourceBusy`/`RecentlyRemediated` (duplicates):
   - `status.phase = "Skipped"`
   - `status.skipReason = reason`
   - `status.duplicateOf = parentRRName`
   - `status.message = "Skipped: {reason} - see {parentRRName}"`
   - Requeue for retry

5. For `ExhaustedRetries`/`PreviousExecutionFailed` (NOT duplicates - require manual review):
   - `status.phase = "Failed"` (not Skipped - terminal failure)
   - `status.skipReason = reason`
   - `status.duplicateOf = ""` (empty - not a duplicate)
   - `status.requiresManualReview = true`
   - `status.message = we.Status.SkipDetails.Message`
   - Create individual `escalation` notification (not bulk)
   - Do NOT requeue - wait for operator intervention

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-032-1 | RO watches WorkflowExecution status changes | Integration |
| AC-032-2 | `Skipped` phase detected and handled | Unit |
| AC-032-3 | Skip reason (`ResourceBusy`, `RecentlyRemediated`, `ExhaustedRetries`, `PreviousExecutionFailed`) extracted and stored | Unit |
| AC-032-4 | Parent RR reference stored in `status.duplicateOf` for duplicates only | Unit |
| AC-032-5 | `ResourceBusy`/`RecentlyRemediated` → RR phase `Skipped`, requeue | Unit |
| AC-032-6 | `ExhaustedRetries`/`PreviousExecutionFailed` → RR phase `Failed`, no requeue | Unit |
| AC-032-7 | Audit trail clearly indicates skip reason | Unit |
| AC-032-8 | `ExhaustedRetries`/`PreviousExecutionFailed` triggers individual notification (not bulk) | Unit |
| AC-032-9 | `ExhaustedRetries`/`PreviousExecutionFailed` sets `requiresManualReview = true` | Unit |
| AC-032-10 | `RecentlyRemediated` uses `NextAllowedExecution` for requeue timing | Unit |

### Test Scenarios

```gherkin
Scenario: WE Skipped due to ResourceBusy (duplicate - requeue)
  Given WorkflowExecution "we-2" targets same resource as "we-1"
  And WorkflowExecution "we-1" is Running
  When WorkflowExecution "we-2" is Skipped with reason "ResourceBusy"
  Then RemediationRequest "rr-2" phase should be "Skipped"
  And status.skipReason should be "ResourceBusy"
  And status.duplicateOf should reference "rr-1"
  And reconciler should requeue after 30 seconds

Scenario: WE Skipped due to RecentlyRemediated (duplicate - requeue with backoff)
  Given Resource "payment/deployment/api" was remediated 5 minutes ago by "rr-1"
  And cooldown period is 10 minutes
  And WorkflowExecution has NextAllowedExecution = now + 5 minutes
  When WorkflowExecution "we-2" is created for same resource
  Then WorkflowExecution "we-2" should be Skipped with reason "RecentlyRemediated"
  And RemediationRequest "rr-2" phase should be "Skipped"
  And status.duplicateOf should reference "rr-1"
  And reconciler should requeue using NextAllowedExecution

Scenario: WE Skipped due to ExhaustedRetries (NOT duplicate - manual review)
  Given Resource "payment/deployment/api" has 5 consecutive pre-execution failures
  When WorkflowExecution "we-6" is created for same resource
  Then WorkflowExecution "we-6" should be Skipped with reason "ExhaustedRetries"
  And RemediationRequest "rr-6" phase should be "Failed"
  And status.requiresManualReview should be TRUE
  And status.duplicateOf should be EMPTY (not a duplicate)
  And an individual escalation notification should be created
  And reconciler should NOT requeue

Scenario: WE Skipped due to PreviousExecutionFailed (NOT duplicate - manual review)
  Given Resource "payment/deployment/api" had a failed execution 5 minutes ago
  And the prior failure was during execution (wasExecutionFailure=true)
  When WorkflowExecution "we-2" is created for same resource
  Then WorkflowExecution "we-2" should be Skipped with reason "PreviousExecutionFailed"
  And RemediationRequest "rr-2" phase should be "Failed"
  And status.requiresManualReview should be TRUE
  And status.duplicateOf should be EMPTY (not a duplicate)
  And an individual escalation notification should be created
  And reconciler should NOT requeue
```

---

## BR-ORCH-033: Track Duplicate Remediations

### Description

RemediationOrchestrator MUST track the relationship between skipped (duplicate) RemediationRequests and their parent (active) RemediationRequest, enabling audit trail and consolidated reporting.

### Priority

**P1 (HIGH)** - Enables audit trail and consolidated notifications

### Rationale

When multiple signals with different fingerprints target the same resource:
- Gateway creates separate RemediationRequests
- WE's resource locking causes all but one to be skipped
- RO must track these relationships for:
  - Audit trail
  - Metrics
  - Consolidated notifications (BR-ORCH-034)

### Implementation

1. When handling Skipped phase (BR-ORCH-032):
   - Update parent RR's `status.duplicateCount++`
   - Append to parent RR's `status.duplicateRefs[]`
2. Handle race conditions with optimistic concurrency (resourceVersion)
3. Non-blocking: Continue even if parent tracking fails (log warning)

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-033-1 | Parent RR tracks count of skipped duplicates | Unit |
| AC-033-2 | Parent RR tracks list of duplicate RR names | Unit |
| AC-033-3 | Duplicate tracking survives RO restarts (persisted in status) | Integration |
| AC-033-4 | Race conditions handled gracefully | Unit |
| AC-033-5 | Tracking failure does not block remediation workflow | Unit |

### Test Scenarios

```gherkin
Scenario: Duplicate tracking on parent
  Given RemediationRequest "rr-1" is executing workflow
  When RemediationRequests "rr-2", "rr-3", "rr-4" are skipped (duplicates of rr-1)
  Then RemediationRequest "rr-1" should have:
    | duplicateCount | 3 |
    | duplicateRefs | ["rr-2", "rr-3", "rr-4"] |

Scenario: Tracking survives restart
  Given RemediationRequest "rr-1" has duplicateCount = 2
  When RemediationOrchestrator pod restarts
  And new duplicate "rr-4" is processed
  Then RemediationRequest "rr-1" duplicateCount should be 3
```

---

## BR-ORCH-034: Bulk Notification for Duplicates

### Description

RemediationOrchestrator MUST send ONE consolidated notification when a parent RemediationRequest completes (success or failure), including summary of all skipped duplicates, to avoid notification spam.

### Priority

**P1 (HIGH)** - Prevents notification spam

### Rationale

Without consolidated notifications:
- 10 skipped RRs would generate 10 separate notifications
- Overwhelming operators
- Unclear which remediation was the "real" one

With bulk notification:
- ONE notification with complete context
- Result + duplicate count in single message
- Clear audit trail

### Implementation

1. When parent RR completes (WorkflowExecution Completed/Failed):
   - Check `status.duplicateCount > 0`
   - Build notification body with:
     - Workflow result (success/failure)
     - Target resource
     - Duration
     - Duplicate count with breakdown by skip reason
     - First/last signal timestamps
   - Create single NotificationRequest with consolidated content
2. Notification triggered on parent completion (not on each skip)

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-034-1 | ONE notification sent when parent completes (not per-skip) | Unit, Integration |
| AC-034-2 | Notification includes duplicate count and skip reasons | Unit |
| AC-034-3 | Notification sent for both success AND failure outcomes | Unit |
| AC-034-4 | Duplicate RR names included in notification metadata | Unit |
| AC-034-5 | No notification spam (10 duplicates = 1 notification) | Integration |

### Test Scenarios

```gherkin
Scenario: Bulk notification on parent completion
  Given RemediationRequest "rr-1" has:
    | duplicateCount | 5 |
    | duplicateRefs | ["rr-2", "rr-3", "rr-4", "rr-5", "rr-6"] |
  And 3 duplicates were ResourceBusy, 2 were RecentlyRemediated
  When WorkflowExecution for "rr-1" completes successfully
  Then ONE NotificationRequest should be created
  And notification body should contain:
    | Result | Successful |
    | Duplicates Suppressed | 5 |
    | ResourceBusy | 3 |
    | RecentlyRemediated | 2 |

Scenario: No notification spam
  Given 10 signals for same resource within 1 minute
  And 9 are skipped as duplicates of "rr-1"
  When "rr-1" completes
  Then total NotificationRequests created should be 1 (not 10)
```

---

## Notification Content Template

```yaml
kind: NotificationRequest
spec:
  eventType: "RemediationCompleted"
  priority: "normal"
  subject: "Remediation Completed: {workflowId}"
  body: |
    Target: {targetResource}
    Result: ✅ Successful / ❌ Failed
    Duration: {duration}

    Duplicates Suppressed: {duplicateCount}
    ├─ ResourceBusy: {resourceBusyCount} (signals during execution)
    └─ RecentlyRemediated: {recentlyRemediatedCount} (cooldown period)

    First signal: {firstOccurrence}
    Last signal: {lastOccurrence}
  metadata:
    remediationRequestRef: "{parentRR.name}"
    workflowId: "{workflowId}"
    targetResource: "{namespace/kind/name}"
    duplicateCount: "{N}"
    duplicateRefs: ["rr-002", "rr-003", ...]
```

---

## Related Documents

- [DD-RO-001: Resource Lock Deduplication Handling](../architecture/decisions/DD-RO-001-resource-lock-deduplication-handling.md)
- [DD-WE-001: Resource Locking Safety](../architecture/decisions/DD-WE-001-resource-locking-safety.md)
- [DD-WE-004: Exponential Backoff Cooldown](../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md)
- [DD-GATEWAY-009: State-Based Deduplication](../architecture/decisions/DD-GATEWAY-009-state-based-deduplication.md)
- [DD-GATEWAY-008: Storm Aggregation](../architecture/decisions/DD-GATEWAY-008-storm-aggregation-first-alert-handling.md)
- [BR-WE-009/010/011: Resource Locking Safety](./BR-WE-009-011-resource-locking.md)
- [NOTICE: WE Exponential Backoff](../handoff/NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| **1.1** | 2025-12-06 | Added `ExhaustedRetries` and `PreviousExecutionFailed` skip reasons (DD-WE-004). Updated handling: duplicates → Skipped+requeue, manual review → Failed+no requeue. Added AC-032-9, AC-032-10. |
| 1.0 | 2025-12-02 | Initial version with `ResourceBusy` and `RecentlyRemediated` handling |

---

**Document Version**: 1.1
**Last Updated**: December 6, 2025
**Maintained By**: Kubernaut Architecture Team


