# BR-ORCH-036: Manual Review & Escalation Notification

**Service**: RemediationOrchestrator Controller
**Category**: V1.0 Core Requirements
**Priority**: P0 (CRITICAL)
**Version**: 4.0
**Date**: 2026-02-24
**Status**: 🚧 Planned
**Related BRs**: BR-ORCH-032 (WE Skip Handling), BR-ORCH-001 (Approval Notification), BR-HAPI-197 (needs_human_review), BR-HAPI-200 (resolved/inconclusive)
**Related DDs**: DD-WE-004 (Exponential Backoff Cooldown), DD-AIANALYSIS-003 (Completion Substates)

---

## Overview

RemediationOrchestrator MUST create NotificationRequest CRDs when:
1. **WorkflowExecution** enters a state requiring operator intervention (exhausted retries, execution failures) → `type=manual-review`
2. **AIAnalysis** fails to produce a valid workflow recommendation (`WorkflowResolutionFailed`) → `type=manual-review`
3. **(v3.0) AIAnalysis** fails with unrecoverable infrastructure errors (APIError, Timeout, MaxRetriesExceeded) → `type=manual-review` (escalation)

**Principle (v3.0)**: Any failure without automatic recovery in the remediation pipeline MUST be notified as an escalation. No failure should silently transition to Failed without operator notification.

**Business Value**: Ensures operators are immediately notified of remediation failures that cannot be automatically resolved, reducing MTTR for critical infrastructure issues by 40-60%.

---

## Trigger Conditions (Complete)

### Source: WorkflowExecution Failures

| Skip/Failure Reason | Description | Priority | Source |
|---------------------|-------------|----------|--------|
| `ExhaustedRetries` | 5+ consecutive pre-execution failures | Critical | DD-WE-004 |
| `PreviousExecutionFailed` | Prior workflow execution failed during run | Critical | DD-WE-004 |

### Source: AIAnalysis WorkflowResolutionFailed

| SubReason | Description | Priority | Source |
|-----------|-------------|----------|--------|
| `WorkflowNotFound` | LLM hallucinated workflow ID not in catalog | High | BR-HAPI-197 |
| `ImageMismatch` | Container image doesn't match catalog | High | BR-HAPI-197 |
| `ParameterValidationFailed` | Parameters don't conform to workflow schema | High | BR-HAPI-197 |
| `NoMatchingWorkflows` | Catalog search returned no results | Medium | BR-HAPI-197 |
| `LowConfidence` | Confidence below 70% threshold | Medium | BR-HAPI-197 |
| `LLMParsingError` | Cannot parse LLM response (after 3 HAPI retries) | High | BR-HAPI-197 |
| `InvestigationInconclusive` | LLM couldn't determine root cause or state | Medium | BR-HAPI-200 |

### Source: AIAnalysis Unrecoverable Infrastructure Failures (v3.0)

| Reason / SubReason | Description | Priority | Source |
|--------------------|-------------|----------|--------|
| `APIError` / `MaxRetriesExceeded` | HAPI request failed after 5 retry attempts (timeout, network error, 5xx) | High | BR-ORCH-036 v3.0 |
| `APIError` / `TransientError` | Transient HAPI error that exceeded retry budget | High | BR-ORCH-036 v3.0 |
| `APIError` / `PermanentError` | Non-retryable HAPI error (4xx, auth failure, bad request) | High | BR-ORCH-036 v3.0 |

**Rationale (v3.0)**: Prior to this version, infrastructure failures in AIAnalysis (e.g., HAPI timeout after all retries) silently transitioned the RemediationRequest to `Failed` without creating a NotificationRequest. This left operators unaware that a remediation attempt had failed due to infrastructure issues. The principle is: **any failure without automatic recovery MUST generate an escalation notification**.

### Source: AIAnalysis Completed with Missing AffectedResource (v4.0)

| Reason / SubReason | Description | Priority | Source |
|--------------------|-------------|----------|--------|
| `AffectedResourceMissing` / `rca_resource_missing` | AA completed with SelectedWorkflow but AffectedResource nil or empty Kind/Name | High | DD-HAPI-006 v1.2, BR-ORCH-036 v4.0 |

**Rationale (v4.0)**: Completes the three-layer defense-in-depth chain (HAPI → AA → RO) for the "cannot identify RCA target" scenario. HAPI catches this at the LLM level (BR-HAPI-212), AA catches it during extraction (response_processor.go), and the RO guard catches any remaining cases. All three layers produce the same operator experience: Failed + ManualReviewRequired + NotificationRequest.

---

## Detection Logic

### WorkflowExecution Detection

```go
// Detect WE failures requiring manual review
if we.Status.Phase == "Skipped" || we.Status.Phase == "Failed" {
    reason := we.Status.SkipDetails.Reason
    if reason == "ExhaustedRetries" || reason == "PreviousExecutionFailed" {
        return c.CreateManualReviewNotification(ctx, rr, we, nil)
    }
    if we.Status.FailureDetails != nil && we.Status.FailureDetails.WasExecutionFailure {
        return c.CreateManualReviewNotification(ctx, rr, we, nil)
    }
}
```

### AIAnalysis Detection

```go
// Detect AIAnalysis failures requiring manual review
if ai.Status.Phase == "Failed" && ai.Status.Reason == "WorkflowResolutionFailed" {
    // SubReason: WorkflowNotFound, ImageMismatch, ParameterValidationFailed,
    //            NoMatchingWorkflows, LowConfidence, LLMParsingError,
    //            InvestigationInconclusive
    return c.CreateManualReviewNotification(ctx, rr, nil, ai)
}
```

### AIAnalysis Infrastructure Failure Detection (v3.0)

```go
// v3.0: Detect unrecoverable infrastructure failures
// Triggered when AIAnalysis fails with APIError, Timeout, etc.
// (after exhausting retry budget in the AA controller)
if ai.Status.Phase == "Failed" && ai.Status.Reason != "WorkflowResolutionFailed" && !ai.Status.NeedsHumanReview {
    // Reason: APIError, SubReason: MaxRetriesExceeded, TransientError, PermanentError
    // Principle: No failure without recovery goes unnotified
    return c.CreateManualReviewNotification(ctx, rr, reviewCtx)
}
```

### Priority Mapping

| Source | Reason/SubReason | Notification Priority |
|--------|------------------|----------------------|
| WE | `ExhaustedRetries` | `critical` |
| WE | `PreviousExecutionFailed` | `critical` |
| WE | Execution failure (`WasExecutionFailure=true`) | `critical` |
| AI | `WorkflowNotFound` | `high` |
| AI | `ImageMismatch` | `high` |
| AI | `ParameterValidationFailed` | `high` |
| AI | `LLMParsingError` | `high` |
| AI | `NoMatchingWorkflows` | `medium` |
| AI | `LowConfidence` | `medium` |
| AI | `InvestigationInconclusive` | `medium` |
| AI (v3.0) | `APIError` / `MaxRetriesExceeded` | `high` |
| AI (v3.0) | `APIError` / `TransientError` | `high` |
| AI (v3.0) | `APIError` / `PermanentError` | `high` |

---

## Implementation

### Unified Handler

```go
// CreateManualReviewNotification handles both WE and AI failure scenarios
func (c *NotificationCreator) CreateManualReviewNotification(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    we *workflowexecutionv1.WorkflowExecution, // nil if AI source
    ai *aianalysisv1.AIAnalysis,               // nil if WE source
) (string, error) {

    var (
        source      string
        reason      string
        subReason   string
        message     string
        priority    notificationv1.NotificationPriority
    )

    if we != nil {
        source = "WorkflowExecution"
        if we.Status.SkipDetails != nil {
            reason = we.Status.SkipDetails.Reason
            message = we.Status.SkipDetails.Message
        } else if we.Status.FailureDetails != nil {
            reason = "ExecutionFailure"
            message = we.Status.FailureDetails.NaturalLanguageSummary
        }
        priority = c.MapWEReasonToPriority(reason)
    } else if ai != nil {
        source = "AIAnalysis"
        reason = ai.Status.Reason      // WorkflowResolutionFailed
        subReason = ai.Status.SubReason
        message = ai.Status.Message
        priority = c.MapAISubReasonToPriority(subReason)
    }

    // Create NotificationRequest (Issue #91: spec fields replace labels)
    nr := &notificationv1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("nr-manual-review-%s-%d", rr.Name, time.Now().Unix()),
            Namespace: rr.Namespace,
        },
        Spec: notificationv1.NotificationRequestSpec{
            RemediationRequestRef: &corev1.ObjectReference{Name: rr.Name, Namespace: rr.Namespace},
            Type:     notificationv1.NotificationTypeManualReview,
            Priority: priority,
            Subject:  fmt.Sprintf("⚠️ Manual Review Required: %s", rr.Spec.SignalName),
            Body:     c.buildManualReviewBody(rr, source, reason, subReason, message),
            Channels: c.determineChannelsForPriority(priority),
            Metadata: map[string]string{
                "remediationRequest": rr.Name,
                "failureSource":      source,
                "failureReason":      reason,
                "subReason":          subReason,
            },
        },
    }

    // Set OwnerReference for cascade deletion
    controllerutil.SetControllerReference(rr, nr, c.scheme)

    return nr.Name, c.client.Create(ctx, nr)
}
```

---

## Acceptance Criteria

### WorkflowExecution Source

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-036-01 | NotificationRequest created with `type=manual-review` for `ExhaustedRetries` | Unit |
| AC-036-02 | NotificationRequest created with `type=manual-review` for `PreviousExecutionFailed` | Unit |
| AC-036-03 | NotificationRequest created with `type=manual-review` for `WasExecutionFailure=true` | Unit |
| AC-036-04 | Priority is `critical` for WE failures | Unit |
| AC-036-05 | `spec.metadata.failureSource=WorkflowExecution` set | Unit |

### AIAnalysis Source

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-036-10 | NotificationRequest created for `WorkflowNotFound` | Unit |
| AC-036-11 | NotificationRequest created for `ImageMismatch` | Unit |
| AC-036-12 | NotificationRequest created for `ParameterValidationFailed` | Unit |
| AC-036-13 | NotificationRequest created for `NoMatchingWorkflows` | Unit |
| AC-036-14 | NotificationRequest created for `LowConfidence` | Unit |
| AC-036-15 | NotificationRequest created for `LLMParsingError` | Unit |
| AC-036-16 | NotificationRequest created for `InvestigationInconclusive` | Unit |
| AC-036-17 | Priority mapped correctly per SubReason | Unit |
| AC-036-18 | `spec.metadata.failureSource=AIAnalysis` set | Unit |
| AC-036-19 | `spec.metadata.failureReason=WorkflowResolutionFailed` set | Unit |

### AIAnalysis Infrastructure Failures (v3.0)

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-036-30 | NotificationRequest created for `APIError` / `MaxRetriesExceeded` | Unit |
| AC-036-31 | NotificationRequest created for `APIError` / `TransientError` | Unit |
| AC-036-32 | NotificationRequest created for `APIError` / `PermanentError` | Unit |
| AC-036-33 | Priority is `high` for infrastructure failures | Unit |
| AC-036-34 | `spec.metadata.failureSource=AIAnalysis` set | Unit |
| AC-036-35 | No failure transitions to RR `Failed` without a notification | Integration |

### AIAnalysis Missing AffectedResource (v4.0)

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-036-40 | NotificationRequest created when AA completed with empty AffectedResource | Integration |
| AC-036-41 | RR transitions to Failed with RequiresManualReview=true | Integration |
| AC-036-42 | RR Outcome set to "ManualReviewRequired" | Integration |
| AC-036-43 | K8s Warning event emitted with reason EscalatedToManualReview | Integration |

### Common

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-036-20 | OwnerReference set for cascade deletion (BR-ORCH-031) | Unit |
| AC-036-21 | Notification reference tracked in RR status (BR-ORCH-035) | Unit |
| AC-036-22 | RR `requiresManualReview=true` set | Unit |
| AC-036-23 | RR phase set appropriately (`Failed` or `Blocked`) | Unit |
| AC-036-24 | No auto-retry for any manual review scenario | Unit |
| AC-036-25 | End-to-end latency <5 seconds | Integration |

---

## Test Scenarios

```gherkin
# WorkflowExecution Failures
Scenario: Manual review notification for WE ExhaustedRetries
  Given WorkflowExecution "we-1" is Skipped with reason "ExhaustedRetries"
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created with:
    | type | manual-review |
    | priority | critical |
    | spec.metadata.failureSource | WorkflowExecution |
  And RemediationRequest "rr-1" should have requiresManualReview = true

# AIAnalysis Failures
Scenario: Manual review notification for AIAnalysis WorkflowNotFound
  Given AIAnalysis "ai-1" has:
    | phase | Failed |
    | reason | WorkflowResolutionFailed |
    | subReason | WorkflowNotFound |
    | message | Workflow 'restart-pod-v99' not found in catalog |
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created with:
    | type | manual-review |
    | priority | high |
    | spec.metadata.failureSource | AIAnalysis |
    | spec.metadata.failureReason | WorkflowResolutionFailed |
  And notification body should contain "Workflow 'restart-pod-v99' not found"
  And RemediationRequest "rr-1" should have requiresManualReview = true

Scenario: Manual review notification for AIAnalysis LowConfidence
  Given AIAnalysis "ai-1" has:
    | phase | Failed |
    | reason | WorkflowResolutionFailed |
    | subReason | LowConfidence |
    | message | Confidence (0.55) below threshold (0.70) |
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created with:
    | type | manual-review |
    | priority | medium |
  And notification body should contain "low confidence"

Scenario: Manual review notification for AIAnalysis InvestigationInconclusive
  Given AIAnalysis "ai-1" has:
    | phase | Failed |
    | reason | WorkflowResolutionFailed |
    | subReason | InvestigationInconclusive |
    | message | Unable to determine root cause. Pod status ambiguous. |
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created with:
    | type | manual-review |
    | priority | medium |
  And notification body should contain "investigation inconclusive"

# v3.0: Infrastructure Failures
Scenario: Escalation notification for AIAnalysis HAPI timeout (MaxRetriesExceeded)
  Given AIAnalysis "ai-1" has:
    | phase | Failed |
    | reason | APIError |
    | subReason | MaxRetriesExceeded |
    | message | Transient error exceeded max retries (5 attempts): HAPI request timeout |
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created with:
    | type | manual-review |
    | priority | high |
    | spec.metadata.failureSource | AIAnalysis |
  And notification body should contain "APIError"
  And notification body should contain "MaxRetriesExceeded"
  And RemediationRequest "rr-1" should have requiresManualReview = true

Scenario: Escalation notification for AIAnalysis permanent HAPI error
  Given AIAnalysis "ai-1" has:
    | phase | Failed |
    | reason | APIError |
    | subReason | PermanentError |
    | message | HolmesGPT-API returned 401 Unauthorized |
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created with:
    | type | manual-review |
    | priority | high |
  And notification body should contain "PermanentError"

# v4.0: Defense-in-Depth - Missing AffectedResource
Scenario: Manual review notification for AA completed but AffectedResource missing
  Given AIAnalysis "ai-1" has:
    | phase | Completed |
    | selectedWorkflow | wf-restart-pods |
    | affectedResource.kind | (empty) |
    | affectedResource.name | (empty) |
  When RemediationOrchestrator reconciles "rr-1"
  Then NotificationRequest should be created with:
    | type | manual-review |
    | priority | high |
    | spec.metadata.failureSource | AIAnalysis |
    | spec.metadata.failureReason | AffectedResourceMissing |
  And RemediationRequest "rr-1" should have requiresManualReview = true
  And RemediationRequest "rr-1" should have phase = Failed
```

---

## Notification Content Templates

### WorkflowExecution Failure Template

```yaml
spec:
  type: manual-review
  priority: critical
  subject: "⚠️ Manual Review Required: {signalName} - Workflow Execution Failed"
  body: |
    Remediation requires manual intervention due to workflow execution failure.

    **Signal**: {signalName}
    **Target**: {namespace}/{kind}/{name}
    **Environment**: {environment}

    **Failure Source**: WorkflowExecution
    **Reason**: {ExhaustedRetries|PreviousExecutionFailed|ExecutionFailure}
    **Details**: {message}

    **Action Required**:
    - ExhaustedRetries: Clear backoff state or investigate root cause
    - PreviousExecutionFailed: Verify cluster state before retry
    - ExecutionFailure: Review workflow logs and cluster state
```

### AIAnalysis Failure Template

```yaml
spec:
  type: manual-review
  priority: {high|medium}
  subject: "⚠️ Manual Review Required: {signalName} - AI Could Not Recommend Workflow"
  body: |
    AI analysis completed but could not produce a valid workflow recommendation.

    **Signal**: {signalName}
    **Target**: {namespace}/{kind}/{name}
    **Environment**: {environment}

    **Failure Source**: AIAnalysis
    **Reason**: WorkflowResolutionFailed
    **SubReason**: {subReason}
    **Details**: {message}

    **Root Cause Analysis**:
    {rootCauseAnalysis.summary}

    **Attempted Workflow** (if any):
    {selectedWorkflow.workflowId} (confidence: {selectedWorkflow.confidence})

    **Action Required**:
    - WorkflowNotFound/ImageMismatch: Update workflow catalog
    - ParameterValidationFailed: Fix workflow schema
    - NoMatchingWorkflows: Add workflows for this incident type
    - LowConfidence: Manual investigation and decision
    - LLMParsingError: Contact AI team if persistent
    - InvestigationInconclusive: Manual investigation required
```

### AIAnalysis Infrastructure Failure Template (v3.0)

```yaml
spec:
  type: manual-review
  priority: high
  subject: "⚠️ Manual Review Required: {signalName} - AI Analysis Infrastructure Failure"
  body: |
    AI analysis failed due to infrastructure issues. The remediation pipeline could not
    complete because the HolmesGPT-API backend was unreachable or returned errors.

    **Signal**: {signalName}
    **Severity**: {severity}

    **Affected Resource**:
    {targetResource}

    **Failure Source**: AIAnalysis
    **Reason**: {reason} (e.g., APIError)
    **SubReason**: {subReason} (e.g., MaxRetriesExceeded, TransientError, PermanentError)
    **Details**: {message}

    **Action Required**:
    - MaxRetriesExceeded: Check HAPI pod health, LLM backend availability, network connectivity
    - TransientError: Verify LLM provider (Vertex AI / Anthropic) is operational
    - PermanentError: Check HAPI configuration, authentication credentials, API keys
    - Review HAPI logs: kubectl logs -n kubernaut-system -l app=holmesgpt-api --tail=100
```

---

## API Change

### NotificationType Enum

```go
// +kubebuilder:validation:Enum=escalation;simple;status-update;approval;manual-review
type NotificationType string

const (
    NotificationTypeEscalation   NotificationType = "escalation"
    NotificationTypeSimple       NotificationType = "simple"
    NotificationTypeStatusUpdate NotificationType = "status-update"
    NotificationTypeApproval     NotificationType = "approval"      // BR-ORCH-001
    NotificationTypeManualReview NotificationType = "manual-review" // BR-ORCH-036
)
```

---

## Metrics

```prometheus
# Counter for manual review notifications sent
kubernaut_remediationorchestrator_manual_review_notifications_total{
  source="WorkflowExecution|AIAnalysis",
  reason="ExhaustedRetries|PreviousExecutionFailed|ExecutionFailure|WorkflowResolutionFailed|APIError",
  sub_reason="WorkflowNotFound|ImageMismatch|ParameterValidationFailed|NoMatchingWorkflows|LowConfidence|LLMParsingError|InvestigationInconclusive|MaxRetriesExceeded|TransientError|PermanentError",
  namespace="<rr_namespace>"
}
```

---

## Related Documents

- [BR-ORCH-032: Handle WE Skipped Phase](./BR-ORCH-032-034-resource-lock-deduplication.md)
- [BR-ORCH-001: Approval Notification Creation](./BR-ORCH-001-approval-notification-creation.md)
- [BR-ORCH-035: Notification Reference Tracking](./BR-ORCH-035-notification-reference-tracking.md)
- [BR-ORCH-037: Handle AIAnalysis WorkflowNotNeeded](./BR-ORCH-037-workflow-not-needed.md)
- [BR-HAPI-197: needs_human_review Field](./BR-HAPI-197-needs-human-review-field.md)
- [BR-HAPI-200: Resolved/Inconclusive Signals](./BR-HAPI-200-resolved-stale-signals.md)
- [DD-WE-004: Exponential Backoff Cooldown](../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md)
- [DD-AIANALYSIS-003: Completion Substates](../architecture/decisions/DD-AIANALYSIS-003-completion-substates.md)
- [NOTICE: AIAnalysis WorkflowResolutionFailed](../handoff/NOTICE_AIANALYSIS_WORKFLOW_RESOLUTION_FAILURE.md)
- [NOTICE: Investigation Inconclusive BR-HAPI-200](../handoff/NOTICE_INVESTIGATION_INCONCLUSIVE_BR_HAPI_200.md)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 4.0 | 2026-02-24 | **Defense-in-depth guard**: Added RO guard for AA completed with missing AffectedResource (nil or empty Kind/Name). Completes three-layer chain (HAPI → AA → RO) for "cannot identify RCA target" scenario. All layers produce same response: Failed + ManualReviewRequired + NotificationRequest. See DD-HAPI-006 v1.2. |
| 3.0 | 2026-02-09 | **Escalation principle**: Any failure without automatic recovery MUST be notified. Added AIAnalysis infrastructure failures (APIError/MaxRetriesExceeded, TransientError, PermanentError) as notification triggers. Previously, these failures silently transitioned RR to Failed without operator notification. Also increased AA controller default HAPI timeout from 60s to 10m to accommodate real LLM response times (temporary; will be replaced by session-based pulling design). |
| 2.0 | 2025-12-07 | Extended to include all AIAnalysis WorkflowResolutionFailed scenarios (7 SubReasons), added BR-HAPI-200 InvestigationInconclusive |
| 1.0 | 2025-12-06 | Initial BR creation for WE failures only |

---

**Document Version**: 4.0
**Last Updated**: February 24, 2026
**Maintained By**: Kubernaut Architecture Team
