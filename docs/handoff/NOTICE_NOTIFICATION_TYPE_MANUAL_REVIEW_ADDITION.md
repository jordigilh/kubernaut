# NOTICE: NotificationType Enum Addition - `manual-review`

> **Note (Issue #91):** This document references `kubernaut.ai/*` CRD labels that have since been migrated to immutable spec fields. See [DD-CRD-003](../architecture/DD-CRD-003-field-selectors-operational-queries.md) for the current field-selector-based approach.

**From**: RemediationOrchestrator Team
**To**: Notification Service Team
**Date**: 2025-12-06
**Priority**: HIGH
**Status**: ⏳ Awaiting Acknowledgment
**Version**: 1.0

---

## Summary

RemediationOrchestrator requires a new `NotificationType` enum value `manual-review` to support BR-ORCH-036 (Manual Review Notification). This change has been made to the shared API types.

This is a **follow-up** to the `approval` enum addition (NOTICE_NOTIFICATION_TYPE_APPROVAL_ADDITION.md).

---

## Change Details

### API Change

**File**: `api/notification/v1alpha1/notificationrequest_types.go`

**Before** (after `approval` addition):
```go
// +kubebuilder:validation:Enum=escalation;simple;status-update;approval
type NotificationType string

const (
    NotificationTypeEscalation   NotificationType = "escalation"
    NotificationTypeSimple       NotificationType = "simple"
    NotificationTypeStatusUpdate NotificationType = "status-update"
    NotificationTypeApproval     NotificationType = "approval"
)
```

**After**:
```go
// +kubebuilder:validation:Enum=escalation;simple;status-update;approval;manual-review
type NotificationType string

const (
    NotificationTypeEscalation   NotificationType = "escalation"
    NotificationTypeSimple       NotificationType = "simple"
    NotificationTypeStatusUpdate NotificationType = "status-update"
    NotificationTypeApproval     NotificationType = "approval"
    // NotificationTypeManualReview is used for manual intervention required notifications (BR-ORCH-036)
    // Added Dec 2025 for ExhaustedRetries/PreviousExecutionFailed scenarios requiring operator action
    // Distinct from 'escalation' to enable label-based routing rules (BR-NOT-065)
    NotificationTypeManualReview NotificationType = "manual-review"
)
```

---

## Business Justification

### BR-ORCH-036: Manual Review Notification

When WorkflowExecution is skipped due to `ExhaustedRetries` or `PreviousExecutionFailed`, RemediationOrchestrator creates a NotificationRequest CRD with `type=manual-review` to alert operators that manual intervention is required.

**Why a new enum value (`manual-review`) instead of using `escalation`?**

| Type | Purpose | Trigger | Operator Action |
|------|---------|---------|-----------------|
| `escalation` | General failures, timeouts | WE Failed, RR Timeout | Investigate, possibly retry |
| `approval` | Approval requests | AI confidence 60-79% | Approve/Reject workflow |
| **`manual-review` (NEW)** | Exhausted retries, prior execution failures | WE Skipped with specific reasons | **Must clear backoff state or investigate cluster** |

**Key Differences from `escalation`**:

| Aspect | `escalation` | `manual-review` |
|--------|--------------|-----------------|
| **Trigger** | Any failure | Specific pre-execution failures |
| **Cluster State** | Known (workflow didn't run) | May be unknown (prior execution failed) |
| **Retry Possible** | Maybe (depends on error) | **No** - backoff exhausted or state unknown |
| **Operator Action** | Investigate | **Clear backoff** or **verify cluster** |

**Label-based routing benefits (BR-NOT-065)**:
- Distinct routing rules: `kubernaut.ai/notification-type=manual-review`
- Separate notification channels (e.g., PagerDuty for manual-review)
- Different priorities based on failure type
- Separate metrics and dashboards

---

## Trigger Conditions

| Skip Reason | Description | NotificationType |
|-------------|-------------|------------------|
| `ExhaustedRetries` | 5+ consecutive pre-execution failures | `manual-review` |
| `PreviousExecutionFailed` | Prior workflow execution failed during run | `manual-review` |

**Related**: DD-WE-004 (Exponential Backoff Cooldown)

---

## Impact on Notification Service

### Required Changes

| Component | Change Required | Priority |
|-----------|-----------------|----------|
| **CRD Validation** | Add `manual-review` to allowed enum values | **HIGH** |
| **Routing Rules** | Consider `type=manual-review` in routing logic (BR-NOT-065) | **HIGH** |
| **Templates** | Create manual-review-specific notification template | **MEDIUM** |
| **Metrics** | Track `manual-review` type in `notification_requests_total` | **LOW** |

### Recommended Routing Rules

**Per RO Team Response in NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md**:

```yaml
# Notification Service routing rules for manual-review
routing_rules:
  - match:
      type: manual-review
      labels:
        kubernaut.ai/skip-reason: PreviousExecutionFailed
    channels: [pagerduty, slack, email]  # Critical: cluster state unknown
    priority: critical

  - match:
      type: manual-review
      labels:
        kubernaut.ai/skip-reason: ExhaustedRetries
    channels: [slack, email]  # High: infrastructure issue
    priority: high
```

### Recommended Template Structure for `manual-review` Type

```yaml
# Manual review notification should include:
- Skip reason (ExhaustedRetries / PreviousExecutionFailed)
- Target resource (namespace/kind/name)
- Consecutive failure count
- Next allowed execution time (if applicable)
- WE failure message / natural language summary
- Clear action instructions:
  - For ExhaustedRetries: "Clear backoff state or investigate root cause"
  - For PreviousExecutionFailed: "Verify cluster state before retry"
```

---

## Usage by RO

RemediationOrchestrator will create NotificationRequests with:

```go
nr := &notificationv1.NotificationRequest{
    ObjectMeta: metav1.ObjectMeta{
        Name:      fmt.Sprintf("nr-manual-review-%s", rr.Name),
        Namespace: rr.Namespace,
        Labels: map[string]string{
            "kubernaut.ai/remediation-request": rr.Name,
            "kubernaut.ai/notification-type":   "manual-review",
            "kubernaut.ai/skip-reason":         skipReason, // ExhaustedRetries or PreviousExecutionFailed
            "kubernaut.ai/severity":            "critical", // or "high" for ExhaustedRetries
            "kubernaut.ai/environment":         rr.Spec.Environment,
            "kubernaut.ai/component":           "remediation-orchestrator",
        },
    },
    Spec: notificationv1.NotificationRequestSpec{
        Type:     notificationv1.NotificationTypeManualReview, // NEW
        Priority: notificationv1.NotificationPriorityCritical,
        Subject:  "⚠️ Manual Review Required: test-rr - ExhaustedRetries",
        Body:     "...",  // Rich manual review context
        Channels: []notificationv1.Channel{
            notificationv1.ChannelConsole,
            notificationv1.ChannelSlack,
            notificationv1.ChannelEmail,
        },
        Metadata: map[string]string{
            "remediationRequest":   rr.Name,
            "workflowExecution":    we.Name,
            "skipReason":           "ExhaustedRetries",
            "consecutiveFailures":  "5",
            "targetResource":       "payment/Deployment/api",
        },
    },
}
```

---

## Timeline

| Milestone | Date | Owner |
|-----------|------|-------|
| API change committed | 2025-12-06 | RO Team |
| Notification Service CRD updated | TBD | Notification Team |
| RO Day 5 implementation | After acknowledgment | RO Team |
| Integration testing | After both implementations | Both Teams |

---

## Questions for Notification Service Team

1. **Routing Rules**: Will you add `type=manual-review` to BR-NOT-065 supported routing labels?

2. **Skip Reason Label**: Will you add `kubernaut.ai/skip-reason` as a routing label option?
   - Per RO recommendation in NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md

3. **Template Support**: Will you create a specific template for `manual-review` type notifications?

4. **PagerDuty Integration**: Can `manual-review` + `PreviousExecutionFailed` trigger PagerDuty alerts?

---

## Acknowledgment Required

Please acknowledge this notice by adding your response below:

### Notification Service Team Response

**Status**: ✅ **ACKNOWLEDGED & IMPLEMENTED**
**Acknowledged By**: Notification Service Team
**Date**: December 7, 2025
**Notes**: All required changes already implemented. `manual-review` type and `kubernaut.ai/skip-reason` label fully supported.

```
[x] CRD enum validation updated (api/notification/v1alpha1/notificationrequest_types.go:26, 36-39)
[x] Routing rules handle `manual-review` type (pkg/notification/routing/resolver.go)
[x] `kubernaut.ai/skip-reason` label added to BR-NOT-065 (pkg/notification/routing/labels.go:58-63)
[x] Template considerations noted (manual formatting for V1.0, specialized template for V1.1)
[x] Questions answered below
```

---

### Detailed Response to RO Team Questions

#### 1. **Routing Rules**: Will you add `type=manual-review` to BR-NOT-065 supported routing labels?

**Answer**: ✅ **YES - Already Implemented**

**Evidence**:
- **File**: `pkg/notification/routing/labels.go`
- **Line 31-32**: `LabelNotificationType` constant supports all notification types including `manual-review`
- **File**: `pkg/notification/routing/resolver.go`
- **Lines 24-63**: Label-based routing operational for all notification types

**Current Implementation**:
```go
// LabelNotificationType is the label key for notification type routing.
// Values: approval_required, completed, failed, escalation, status_update, approval, manual-review
LabelNotificationType = "kubernaut.ai/notification-type"
```

**Routing Capabilities**:
- ✅ Match on `kubernaut.ai/notification-type=manual-review`
- ✅ Combine with other labels (skip-reason, severity, environment)
- ✅ First matching route wins (Alertmanager-compatible)
- ✅ Multi-channel fanout supported

**Conclusion**: ✅ **NO ACTION REQUIRED** - Routing already handles `manual-review` type

---

#### 2. **Skip Reason Label**: Will you add `kubernaut.ai/skip-reason` as a routing label option?

**Answer**: ✅ **YES - Already Implemented**

**Evidence**:
- **File**: `pkg/notification/routing/labels.go`
- **Lines 58-63**: `LabelSkipReason` constant defined with full documentation

**Current Implementation**:
```go
// LabelSkipReason is the label key for WFE skip reason-based routing.
// Enables fine-grained routing based on why a WorkflowExecution was skipped.
// Values: PreviousExecutionFailed, ExhaustedRetries, ResourceBusy, RecentlyRemediated
// See: docs/handoff/NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md
// Added per cross-team agreement: WE→NOT Q7 (2025-12-06)
LabelSkipReason = "kubernaut.ai/skip-reason"
```

**Supported Skip Reason Values**:
- ✅ `PreviousExecutionFailed` - Critical severity, PagerDuty recommended
- ✅ `ExhaustedRetries` - High severity, Slack #ops recommended
- ✅ `ResourceBusy` - Low severity, bulk/console
- ✅ `RecentlyRemediated` - Low severity, bulk/console

**Routing Capabilities**:
- ✅ Match on `kubernaut.ai/skip-reason=PreviousExecutionFailed`
- ✅ Combine with notification type: `type=manual-review` AND `skip-reason=ExhaustedRetries`
- ✅ Multi-label AND matching logic

**Conclusion**: ✅ **NO ACTION REQUIRED** - Skip reason label fully supported

---

#### 3. **Template Support**: Will you create a specific template for `manual-review` type notifications?

**Answer**: ⚠️ **Not in V1.0, but noted for V1.1**

**Current Status**:
- ❌ No manual-review-specific template exists
- ✅ RO Team can format `Body` field manually for V1.0
- ⚠️ V1.1 consideration for specialized manual-review template

**Recommendation for RO Team (V1.0)**:
Use formatted `Body` field to provide rich manual review context:

```go
Body: fmt.Sprintf(`
⚠️ **Manual Review Required: %s**

**Trigger**: WorkflowExecution skipped due to %s

**Target Resource**:
- Namespace: %s
- Kind: %s
- Name: %s

**Skip Details**:
- Skip Reason: %s
- Consecutive Failures: %d
- Next Allowed Execution: %s

**Operator Actions Required**:
%s

**Investigation Context**:
- Alert: %s
- Severity: %s
- Root Cause: %s

**Important**: %s
`,
notificationSubject,                     // "test-rr"
skipReason,                              // "ExhaustedRetries"
targetNamespace, targetKind, targetName, // "default", "Deployment", "payment-api"
skipReason,                              // "ExhaustedRetries"
consecutiveFailures,                     // 5
nextAllowedTime.Format(time.RFC3339),   // "2025-12-07T11:00:00Z"
getActionInstructions(skipReason),       // Custom instructions per skip reason
alertName, severity, rootCause,
getImportanceMessage(skipReason))        // "Cluster state may be unknown - verify before retry"
```

**Helper Function for Action Instructions**:
```go
func getActionInstructions(skipReason string) string {
    switch skipReason {
    case "ExhaustedRetries":
        return `
1. Review pre-execution failure logs for root cause
2. Clear exponential backoff state if issue resolved
3. Verify infrastructure is healthy
4. Retry workflow execution manually if safe
`
    case "PreviousExecutionFailed":
        return `
1. **CRITICAL**: Verify cluster state is stable before retry
2. Check if previous execution left cluster in unknown state
3. Investigate workflow execution logs for failure details
4. Manual cluster validation may be required
5. Clear backoff state ONLY after cluster verification
`
    default:
        return "Review skip reason and take appropriate action"
    }
}
```

**V1.1 Enhancement Plan**:
- Manual-review-specific template with automatic formatting
- Skip reason-specific action instructions
- Visual severity indicators
- Automatic backoff state information

**Conclusion**: ⚠️ **V1.0: Manual formatting | V1.1: Specialized template**

---

#### 4. **PagerDuty Integration**: Can `manual-review` + `PreviousExecutionFailed` trigger PagerDuty alerts?

**Answer**: ✅ **YES - Fully Supported via Routing Rules**

**Current Capabilities**:
- ✅ PagerDuty channel supported in NotificationRequest spec
- ✅ Routing rules can match on multiple labels (AND logic)
- ✅ PagerDuty integration ready (channel enum includes `pagerduty`)

**Recommended Routing Configuration**:
```yaml
# ConfigMap: notification-routing-config
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-routing-config
  namespace: kubernaut-notifications
data:
  config.yaml: |
    route:
      receiver: default
      routes:
        # Manual review with PreviousExecutionFailed → PagerDuty (CRITICAL)
        - match:
            kubernaut.ai/notification-type: manual-review
            kubernaut.ai/skip-reason: PreviousExecutionFailed
          receiver: critical-manual-review
          continue: true  # Also log to console

        # Manual review with ExhaustedRetries → Slack + Email (HIGH)
        - match:
            kubernaut.ai/notification-type: manual-review
            kubernaut.ai/skip-reason: ExhaustedRetries
          receiver: high-manual-review
          continue: true

        # Fallback for other manual-review notifications
        - match:
            kubernaut.ai/notification-type: manual-review
          receiver: default-manual-review

    receivers:
      - name: critical-manual-review
        pagerduty_configs:
          - service_key: "${PAGERDUTY_SERVICE_KEY}"
            severity: critical
            description: "Cluster state unknown - manual verification required"
        slack_configs:
          - channel: "#critical-alerts"
            webhook_url: "${SLACK_CRITICAL_WEBHOOK}"
        console_configs:
          - enabled: true

      - name: high-manual-review
        slack_configs:
          - channel: "#ops-alerts"
            webhook_url: "${SLACK_OPS_WEBHOOK}"
        email_configs:
          - to: "ops-team@example.com"
        console_configs:
          - enabled: true

      - name: default-manual-review
        slack_configs:
          - channel: "#general-alerts"
            webhook_url: "${SLACK_GENERAL_WEBHOOK}"
        console_configs:
          - enabled: true
```

**Multi-Label Matching**:
- ✅ Route matches when **ALL** match criteria satisfied (AND logic)
- ✅ `type=manual-review` AND `skip-reason=PreviousExecutionFailed` → PagerDuty
- ✅ `type=manual-review` AND `skip-reason=ExhaustedRetries` → Slack + Email

**Conclusion**: ✅ **FULLY SUPPORTED** - RO Team can route `manual-review` + `PreviousExecutionFailed` to PagerDuty

---

### Additional Findings from Triage

#### 1. **Both Notification Types Already in API** ✅

**Verification**: `api/notification/v1alpha1/notificationrequest_types.go`

```go
// Line 26: Both approval and manual-review in enum
// +kubebuilder:validation:Enum=escalation;simple;status-update;approval;manual-review
type NotificationType string

const (
    NotificationTypeEscalation       NotificationType = "escalation"
    NotificationTypeSimple           NotificationType = "simple"
    NotificationTypeStatusUpdate     NotificationType = "status-update"

    // NotificationTypeApproval is used for approval request notifications (BR-ORCH-001)
    // Added Dec 2025 per RO team request for explicit approval workflow support
    NotificationTypeApproval NotificationType = "approval"

    // NotificationTypeManualReview is used for manual intervention required notifications (BR-ORCH-036)
    // Added Dec 2025 for ExhaustedRetries/PreviousExecutionFailed scenarios requiring operator action
    // Distinct from 'escalation' to enable label-based routing rules (BR-NOT-065)
    NotificationTypeManualReview NotificationType = "manual-review"
)
```

**Conclusion**: ✅ Both types committed to API simultaneously

---

#### 2. **Skip Reason Label Already Documented in BR-NOT-065** ✅

**Verification**: BUSINESS_REQUIREMENTS.md already documents skip-reason routing

**Evidence** (from BUSINESS_REQUIREMENTS.md):
```markdown
| Label Key | Purpose | Example Values |
|-----------|---------|----------------|
| `kubernaut.ai/skip-reason` | WFE skip reason routing | `PreviousExecutionFailed`, `ExhaustedRetries`, `ResourceBusy`, `RecentlyRemediated` |

**Skip Reason Routing** (Added per DD-WE-004 v1.1):

| Skip Reason | Recommended Severity | Routing Target | Rationale |
|-------------|---------------------|----------------|-----------|
| `PreviousExecutionFailed` | `critical` | PagerDuty | Cluster state unknown - immediate action required |
| `ExhaustedRetries` | `high` | Slack | Infrastructure issues - team awareness required |
| `ResourceBusy` | `low` | Bulk (BR-ORCH-034) | Temporary - auto-resolves |
| `RecentlyRemediated` | `low` | Bulk (BR-ORCH-034) | Temporary - auto-resolves |
```

**Conclusion**: ✅ Skip reason label already integrated into BR-NOT-065

---

#### 3. **Spec Immutability Applies to Both Types** ⚠️

**Important for RO Team**: Per DD-NOT-005, NotificationRequest spec is **IMMUTABLE** after creation.

**Implication for Manual Review Notifications**:
- Once created, skip reason and failure context **cannot be updated**
- If RO needs to update failure count or next allowed time, must create **new NotificationRequest**
- Recommendation: Create notification **once** with complete context

---

### Summary of Current Support (Both Types)

| Feature | `approval` Type | `manual-review` Type | Implementation |
|---------|----------------|---------------------|----------------|
| **CRD Enum** | ✅ Complete | ✅ Complete | api/notification/v1alpha1/notificationrequest_types.go:26-39 |
| **Routing Rules** | ✅ Complete | ✅ Complete | pkg/notification/routing/ |
| **Skip Reason Label** | N/A | ✅ Complete | pkg/notification/routing/labels.go:58-63 |
| **ActionLinks** | ✅ Complete | ✅ Complete | api/notification/v1alpha1/notificationrequest_types.go:128-137 |
| **PagerDuty Routing** | ✅ Supported | ✅ Supported | Via routing rules |
| **Templates** | ⚠️ V1.1 | ⚠️ V1.1 | Manual Body formatting for V1.0 |

---

### Recommended RO Team Actions for V1.0 (Manual Review)

1. ✅ **Use `NotificationTypeManualReview`** - Available in `api/notification/v1alpha1`
2. ✅ **Set Skip Reason Label** - Use `kubernaut.ai/skip-reason` for routing
3. ✅ **Format Body Field** - Include skip reason, failure count, next allowed time, action instructions
4. ✅ **Add Metadata Map** - Include skip details, target resource, consecutive failures
5. ✅ **Configure Routing** - Route `PreviousExecutionFailed` → PagerDuty, `ExhaustedRetries` → Slack
6. ✅ **Set Retention** - Use `retentionDays: 30` for audit compliance

**Code Example**:
```go
nr := &notificationv1.NotificationRequest{
    ObjectMeta: metav1.ObjectMeta{
        Name:      fmt.Sprintf("nr-manual-review-%s", rr.Name),
        Namespace: rr.Namespace,
        Labels: map[string]string{
            "kubernaut.ai/notification-type":    "manual-review",
            "kubernaut.ai/skip-reason":          "PreviousExecutionFailed",  // ✅ Supported
            "kubernaut.ai/severity":             "critical",
            "kubernaut.ai/remediation-request": rr.Name,
            "kubernaut.ai/component":           "remediation-orchestrator",
            "kubernaut.ai/environment":         rr.Spec.Environment,
        },
    },
    Spec: notificationv1.NotificationRequestSpec{
        Type:     notificationv1.NotificationTypeManualReview,  // ✅ Available
        Priority: notificationv1.NotificationPriorityCritical,
        Subject:  "⚠️ Manual Review Required: test-rr - PreviousExecutionFailed",
        Body:     formatManualReviewBody(rr, we, skipReason),  // Format yourself for V1.0
        Channels: []notificationv1.Channel{
            notificationv1.ChannelConsole,   // Always log for audit
        },
        Metadata: map[string]string{
            "remediationRequest":   rr.Name,
            "workflowExecution":    we.Name,
            "skipReason":           "PreviousExecutionFailed",
            "consecutiveFailures":  fmt.Sprintf("%d", failureCount),
            "nextAllowedTime":      nextAllowedTime.Format(time.RFC3339),
            "targetResource":       fmt.Sprintf("%s/%s/%s", targetNS, targetKind, targetName),
        },
        RetentionDays: 30,  // Keep manual review records for audit
    },
}
```

**Note**: Channels can be empty (`[]`) if relying on routing rules. Routing rules will determine channels based on labels (recommended approach per BR-NOT-065).

---

#### 3. **Template Support**: Will you create a specific template for `manual-review` type notifications?

**Answer**: ⚠️ **Not in V1.0, but noted for V1.1**

**Current Status**:
- ❌ No manual-review-specific template exists
- ✅ RO Team can format `Body` field manually for V1.0
- ⚠️ V1.1 consideration for specialized manual-review template

**Recommended Body Format** (see code example in Q1 response above)

**V1.1 Enhancement Plan**:
- Manual-review-specific template with skip reason-specific formatting
- Automatic action instruction generation
- Visual severity indicators
- Backoff state visualization
- Countdown to next allowed execution time

**Conclusion**: ⚠️ **V1.0: Manual formatting | V1.1: Specialized template**

---

#### 4. **PagerDuty Integration**: Can `manual-review` + `PreviousExecutionFailed` trigger PagerDuty alerts?

**Answer**: ✅ **YES - Fully Supported via Routing Rules**

**Implementation**:
- ✅ PagerDuty channel defined in Channel enum
- ✅ Multi-label routing supports `type=manual-review` AND `skip-reason=PreviousExecutionFailed`
- ✅ Routing configuration example provided (see Q2 response)

**Recommended Configuration**:
```yaml
# Route critical manual reviews to PagerDuty
routes:
  - match:
      kubernaut.ai/notification-type: manual-review
      kubernaut.ai/skip-reason: PreviousExecutionFailed
    receiver: critical-pagerduty
    continue: true  # Also log to console

receivers:
  - name: critical-pagerduty
    pagerduty_configs:
      - service_key: "${PAGERDUTY_SERVICE_KEY}"
        severity: critical
        description: "Cluster state unknown - manual verification required"
    console_configs:
      - enabled: true  # Always log for audit
```

**Conclusion**: ✅ **FULLY SUPPORTED** - Multi-label routing enables PagerDuty for critical manual reviews

---

### Cross-Team Integration Points

#### Skip Reason Severity Mapping (Per RO Recommendation)

| Skip Reason | Recommended Severity | Channels | Rationale |
|-------------|---------------------|----------|-----------|
| `PreviousExecutionFailed` | `critical` | PagerDuty + Slack + Console | Cluster state unknown - immediate action required |
| `ExhaustedRetries` | `high` | Slack #ops + Email + Console | Infrastructure issues - team awareness required |
| `ResourceBusy` | `low` | Console only (bulk notification) | Temporary - auto-resolves |
| `RecentlyRemediated` | `low` | Console only (bulk notification) | Temporary - auto-resolves |

**Note**: RO Team should set `kubernaut.ai/severity` label to match recommended severity for proper routing.

---

### Notification Type Decision Matrix

| Scenario | NotificationType | Skip Reason Label | Priority | Channels |
|----------|-----------------|-------------------|----------|----------|
| **AI confidence 60-79%** | `approval` | N/A | High | Slack #approvals + Console |
| **WE skipped: PreviousExecutionFailed** | `manual-review` | `PreviousExecutionFailed` | Critical | PagerDuty + Slack + Console |
| **WE skipped: ExhaustedRetries** | `manual-review` | `ExhaustedRetries` | High | Slack #ops + Email + Console |
| **WE skipped: ResourceBusy** | `manual-review` | `ResourceBusy` | Low | Console only |
| **WE failed during execution** | `escalation` | N/A | High | Slack + Email + Console |
| **RR timed out** | `escalation` | N/A | Critical | PagerDuty + Slack + Console |

---

### V1.1 Enhancement Candidates (Both Types)

Based on both NOTICE documents, Notification Service will consider for V1.1:

1. **Approval Template**: Auto-format investigation summary, confidence, timeout
2. **Manual Review Template**: Auto-format skip reason, backoff state, action instructions
3. **Dynamic Countdown**: Update Slack messages with remaining time (approval timeout, next allowed execution)
4. **Automatic Expiration**: Notifications when approval timeout or backoff expires
5. **Workflow Integration**: Track approve/reject responses, backoff state changes

---

**Notification Service Team Confidence**: **100%** - All required V1.0 functionality for both `approval` and `manual-review` types is implemented and tested.

---

## Related Documents

- [BR-ORCH-036: Manual Review Notification](../requirements/BR-ORCH-036-manual-review-notification.md)
- [BR-ORCH-032: Handle WE Skipped Phase](../requirements/BR-ORCH-032-034-resource-lock-deduplication.md)
- [DD-WE-004: Exponential Backoff Cooldown](../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md)
- [NOTICE: WE Exponential Backoff](./NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md)
- [NOTICE: NotificationType `approval` Addition](./NOTICE_NOTIFICATION_TYPE_APPROVAL_ADDITION.md)
- [NotificationRequest CRD Schema](../services/crd-controllers/notification/crd-schema.md)

---

**Document Version**: 1.0
**Last Updated**: December 6, 2025
**Maintained By**: RemediationOrchestrator Team


