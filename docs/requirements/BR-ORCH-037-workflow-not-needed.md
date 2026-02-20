# BR-ORCH-037: Handle AIAnalysis WorkflowNotNeeded

**Service**: RemediationOrchestrator Controller
**Category**: V1.0 Core Requirements
**Priority**: P1 (HIGH)
**Version**: 1.0
**Date**: 2025-12-07
**Status**: 🚧 Planned
**Related BRs**: BR-ORCH-036 (Manual Review), BR-HAPI-200 (Resolved/Inconclusive Signals)
**Related DDs**: DD-AIANALYSIS-003 (Completion Substates)

---

## Overview

RemediationOrchestrator MUST handle the scenario where AIAnalysis determines that **no remediation is needed** because the problem has self-resolved.

**Business Value**: Prevents unnecessary workflow executions when issues have already resolved, saving compute resources and avoiding potential disruption from remediation actions on already-healthy systems.

---

## Context

BR-HAPI-200 introduces a new investigation outcome where the LLM confidently determines the problem no longer exists:

```json
{
  "needs_human_review": false,
  "human_review_reason": null,
  "selected_workflow": null,
  "confidence": 0.92,
  "investigation_summary": "Investigated OOMKilled signal. Pod 'myapp' recovered automatically. Status: Running, memory at 45% of limit. No remediation required.",
  "warnings": ["Problem self-resolved - no remediation required"]
}
```

This is **NOT a failure** - the AI successfully investigated and determined no action is needed.

---

## Trigger Condition

```yaml
# AIAnalysis Status
status:
  phase: Completed
  reason: WorkflowNotNeeded
  subReason: ProblemResolved  # Optional
  message: "Problem self-resolved. No remediation required."
  selectedWorkflow: null      # No workflow selected
  rootCauseAnalysis:          # RCA still available
    summary: "Pod recovered automatically after OOMKilled event"
    severity: "low"
```

**Key Indicators**:
- `phase: Completed` (NOT Failed)
- `reason: WorkflowNotNeeded`
- `selectedWorkflow: null`
- High confidence (≥0.7)

---

## RO Behavior

### Detection Logic

```go
// After AIAnalysis completes, check for WorkflowNotNeeded
if ai.Status.Phase == "Completed" {
    if ai.Status.Reason == "WorkflowNotNeeded" {
        return c.HandleWorkflowNotNeeded(ctx, rr, ai)
    }
    // Normal flow: ApprovalRequired or AutoExecutable
    // ...
}
```

### Handler Implementation

```go
// HandleWorkflowNotNeeded processes the "no action needed" scenario
func (h *AIAnalysisHandler) HandleWorkflowNotNeeded(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    ai *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    logger.Info("AIAnalysis determined no remediation needed",
        "reason", ai.Status.Reason,
        "message", ai.Status.Message,
    )

    // 1. Update RemediationRequest status
    rr.Status.OverallPhase = "Completed"
    rr.Status.Message = ai.Status.Message
    rr.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    rr.Status.Outcome = "NoActionRequired"  // New field

    if err := h.client.Status().Update(ctx, rr); err != nil {
        return ctrl.Result{}, err
    }

    // 2. Do NOT create WorkflowExecution
    // (skip WE creation entirely)

    // 3. Optionally create informational notification
    if h.config.NotifyOnSelfResolved {
        if _, err := h.CreateSelfResolvedNotification(ctx, rr, ai); err != nil {
            logger.Error(err, "Failed to create self-resolved notification")
            // Non-fatal - continue
        }
    }

    // 4. Record metric
    metrics.RemediationNoActionNeeded.WithLabelValues(
        rr.Namespace,
        "problem_resolved",
    ).Inc()

    // 5. No requeue - remediation complete
    return ctrl.Result{}, nil
}
```

---

## RemediationRequest Status Update

### New Status Fields

```go
type RemediationRequestStatus struct {
    // ... existing fields ...

    // Outcome indicates how the remediation concluded
    // +kubebuilder:validation:Enum=WorkflowExecuted;ApprovalGranted;ApprovalDenied;NoActionRequired;ManualReviewRequired;Timeout;Failed
    // +optional
    Outcome string `json:"outcome,omitempty"`
}
```

### Status Example

```yaml
status:
  overallPhase: Completed
  outcome: NoActionRequired
  message: "Problem self-resolved. No remediation required."
  completionTime: "2025-12-07T10:05:00Z"

  # Child CRD references
  signalProcessingRef: { name: "sp-xyz", namespace: "production" }
  aiAnalysisRef: { name: "ai-xyz", namespace: "production" }
  workflowExecutionRef: null  # NOT created
  notificationRequestRefs: []  # Optional informational notification
```

---

## Optional Notification

If configured, RO MAY create an **informational** notification:

```yaml
kind: NotificationRequest
metadata:
  name: nr-info-{rr-name}
spec:
  type: status-update  # NOT manual-review or approval (Issue #91: spec.type replaces label)
  metadata:
    outcome: no-action-required
  priority: low
  subject: "ℹ️ Auto-Resolved: {signalName}"
  body: |
    Signal was investigated but no remediation was needed.

    **Signal**: {signalName}
    **Target**: {namespace}/{kind}/{name}

    **AI Assessment**:
    {ai.Status.Message}

    **Root Cause Analysis**:
    {ai.Status.RootCauseAnalysis.Summary}

    No action was taken. This notification is for audit purposes only.
  channels:
    - console  # Minimal channels for informational
```

### Configuration

```yaml
# RO ConfigMap
data:
  notify_on_self_resolved: "false"  # Default: no notification
  # Set to "true" for audit-heavy environments
```

---

## Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-037-01 | RO detects `AIAnalysis.Status.Reason == "WorkflowNotNeeded"` | Unit |
| AC-037-02 | RO does NOT create WorkflowExecution for this scenario | Unit |
| AC-037-03 | RR status updated to `OverallPhase=Completed`, `Outcome=NoActionRequired` | Unit |
| AC-037-04 | RR `completionTime` set | Unit |
| AC-037-05 | RR message reflects AI's message | Unit |
| AC-037-06 | No requeue after handling | Unit |
| AC-037-07 | Metric `kubernaut_remediation_no_action_needed_total` incremented | Unit |
| AC-037-08 | Informational notification created when configured | Unit |
| AC-037-09 | No notification by default | Unit |
| AC-037-10 | High confidence (≥0.7) from AI is preserved in RR for audit | Unit |

---

## Test Scenarios

```gherkin
Scenario: No workflow created when problem self-resolved
  Given AIAnalysis "ai-1" has:
    | phase | Completed |
    | reason | WorkflowNotNeeded |
    | message | Problem self-resolved. No remediation required. |
    | selectedWorkflow | null |
  When RemediationOrchestrator reconciles "rr-1"
  Then WorkflowExecution should NOT be created
  And RemediationRequest "rr-1" should have:
    | overallPhase | Completed |
    | outcome | NoActionRequired |
    | workflowExecutionRef | null |
  And metric "kubernaut_remediation_no_action_needed_total" should increment

Scenario: Optional notification when configured
  Given AIAnalysis "ai-1" has reason "WorkflowNotNeeded"
  And RO configuration has notify_on_self_resolved = true
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created with:
    | type | status-update |
    | priority | low |
  And notification body should contain "no remediation was needed"

Scenario: No notification by default
  Given AIAnalysis "ai-1" has reason "WorkflowNotNeeded"
  And RO configuration has notify_on_self_resolved = false (default)
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should NOT be created
```

---

## Metrics

```prometheus
# Counter for no-action-required remediations
kubernaut_remediationorchestrator_no_action_needed_total{
  reason="problem_resolved|investigation_inconclusive",
  namespace="<rr_namespace>"
}

# Duration from signal to no-action conclusion
kubernaut_remediationorchestrator_no_action_duration_seconds{
  namespace="<rr_namespace>"
}
```

---

## Comparison: WorkflowNotNeeded vs WorkflowResolutionFailed

| Aspect | WorkflowNotNeeded | WorkflowResolutionFailed |
|--------|-------------------|--------------------------|
| **Phase** | `Completed` | `Failed` |
| **Meaning** | AI successfully concluded no action needed | AI couldn't produce valid recommendation |
| **Confidence** | High (≥0.7) | N/A or Low |
| **selectedWorkflow** | `null` (intentionally) | `null` or invalid |
| **RO Action** | Skip WE creation | Create manual-review notification |
| **RR Outcome** | `NoActionRequired` | `ManualReviewRequired` |
| **Notification** | Optional (informational) | Required (manual-review) |
| **Operator Action** | None required | Investigation required |

---

## Edge Cases

### Edge Case 1: Flapping Signals

If a signal repeatedly triggers and self-resolves (flapping):

```go
// RO tracks self-resolution count
if rr.Status.SelfResolutionCount > 3 {
    // Escalate as potential flapping
    return c.CreateFlappingNotification(ctx, rr)
}
```

**V1.0**: Not implemented. Operators must monitor metrics for patterns.
**V1.1**: Consider adding flapping detection.

### Edge Case 2: Simultaneous Signals

If multiple signals for the same resource all self-resolve:

- Each RR concludes independently
- Deduplication (BR-ORCH-032) handles active signals
- Historical signals that self-resolve don't affect new ones

---

## Related Documents

- [BR-ORCH-036: Manual Review Notification](./BR-ORCH-036-manual-review-notification.md)
- [BR-HAPI-200: Resolved/Inconclusive Signals](./BR-HAPI-200-resolved-stale-signals.md)
- [DD-AIANALYSIS-003: Completion Substates](../architecture/decisions/DD-AIANALYSIS-003-completion-substates.md)
- [NOTICE: Investigation Inconclusive BR-HAPI-200](../handoff/NOTICE_INVESTIGATION_INCONCLUSIVE_BR_HAPI_200.md)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-07 | Initial BR creation based on BR-HAPI-200 |

---

**Document Version**: 1.0
**Last Updated**: December 7, 2025
**Maintained By**: Kubernaut Architecture Team



