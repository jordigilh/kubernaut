# NOTICE: NotificationType Enum Addition - `approval`

> **Note (Issue #91):** This document references `kubernaut.ai/*` CRD labels that have since been migrated to immutable spec fields. See [DD-CRD-003](../architecture/DD-CRD-003-field-selectors-operational-queries.md) for the current field-selector-based approach.

**From**: RemediationOrchestrator Team
**To**: Notification Service Team
**Date**: 2025-12-06
**Priority**: HIGH
**Status**: ⏳ Awaiting Acknowledgment

---

## Summary

RemediationOrchestrator requires a new `NotificationType` enum value `approval` to support BR-ORCH-001 (Approval Notification Creation). This change has been made to the shared API types.

---

## Change Details

### API Change

**File**: `api/notification/v1alpha1/notificationrequest_types.go`

**Before**:
```go
// +kubebuilder:validation:Enum=escalation;simple;status-update
type NotificationType string

const (
    NotificationTypeEscalation   NotificationType = "escalation"
    NotificationTypeSimple       NotificationType = "simple"
    NotificationTypeStatusUpdate NotificationType = "status-update"
)
```

**After**:
```go
// +kubebuilder:validation:Enum=escalation;simple;status-update;approval
type NotificationType string

const (
    NotificationTypeEscalation   NotificationType = "escalation"
    NotificationTypeSimple       NotificationType = "simple"
    NotificationTypeStatusUpdate NotificationType = "status-update"
    // NotificationTypeApproval is used for approval request notifications (BR-ORCH-001)
    // Added Dec 2025 per RO team request for explicit approval workflow support
    NotificationTypeApproval NotificationType = "approval"
)
```

---

## Business Justification

### BR-ORCH-001: Approval Notification Creation

When AIAnalysis requires manual approval (confidence 60-79%), RemediationOrchestrator creates a NotificationRequest CRD to push notifications to operators.

**Why a new enum value?**
- `escalation`: For failures/timeouts requiring attention
- `simple`: For informational notifications
- `status-update`: For progress updates
- **`approval` (NEW)**: For approval requests requiring human decision

Approval notifications have unique characteristics:
- **Requires action**: Operator must approve/reject
- **Time-sensitive**: Has approval timeout (default 15 minutes)
- **Bidirectional**: May include approve/reject action links
- **Context-rich**: Includes investigation summary, evidence, rationale

---

## Impact on Notification Service

### Required Changes

| Component | Change Required | Priority |
|-----------|-----------------|----------|
| **CRD Validation** | Add `approval` to allowed enum values | **HIGH** |
| **Routing Rules** | Consider `type=approval` in routing logic | **MEDIUM** |
| **Templates** | Create approval-specific notification template | **MEDIUM** |
| **Metrics** | Track `approval` type in `notification_requests_total` | **LOW** |

### Recommended Template Structure for `approval` Type

```yaml
# Approval notification should include:
- Investigation summary
- Root cause analysis
- Recommended workflow with rationale
- Confidence score
- Why approval is required
- Approve/Reject action links (if supported)
- Approval timeout information
```

---

## Usage by RO

RemediationOrchestrator will create NotificationRequests with:

```go
nr := &notificationv1.NotificationRequest{
    Spec: notificationv1.NotificationRequestSpec{
        Type:     notificationv1.NotificationTypeApproval,  // NEW
        Priority: notificationv1.NotificationPriorityHigh,
        Subject:  "Approval Required: High Memory Usage on payment-api",
        Body:     "...",  // Rich approval context
        Channels: []notificationv1.Channel{notificationv1.ChannelSlack},
        Metadata: map[string]string{
            "remediationRequest": "rr-12345",
            "aiAnalysis":         "ai-12345",
            "approvalReason":     "Confidence 72% requires manual review",
            "confidence":         "0.72",
            "selectedWorkflow":   "memory-increase-v1",
            "approvalTimeout":    "15m",
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
| RO Day 4 implementation | After acknowledgment | RO Team |
| Integration testing | After both implementations | Both Teams |

---

## Questions for Notification Service Team

1. **Template Support**: Will you create a specific template for `approval` type notifications?

2. **Action Links**: Do you support `ActionLinks` in the spec for approve/reject buttons?

3. **Routing**: Should `type=approval` have default routing rules (e.g., always include console channel)?

4. **Timeout Display**: Can the notification include countdown/timeout information?

---

## Acknowledgment Required

Please acknowledge this notice by adding your response below:

### Notification Service Team Response

**Status**: ✅ **ACKNOWLEDGED & IMPLEMENTED**
**Acknowledged By**: Notification Service Team
**Date**: December 7, 2025
**Notes**: All required changes already implemented. `approval` type fully supported.

```
[x] CRD enum validation updated (api/notification/v1alpha1/notificationrequest_types.go:26-35)
[x] Routing rules handle `approval` type (pkg/notification/routing/labels.go:31, resolver.go)
[x] Template considerations noted (no specific template yet - V1.1 consideration)
[x] Questions answered below
```

---

### Detailed Response to RO Team Questions

#### 1. **Template Support**: Will you create a specific template for `approval` type notifications?

**Answer**: **Not in V1.0, but noted for V1.1**

**Current Status**:
- ✅ The `approval` type is **fully supported** in CRD validation and routing
- ✅ Notifications with `type=approval` will be delivered to channels based on routing rules
- ⚠️ No approval-specific template exists yet

**Recommendation for RO Team (V1.0)**:
- Use the **`Body` field** in NotificationRequestSpec to provide rich approval context:
  ```go
  Body: fmt.Sprintf(`
  🔍 **Approval Required: %s**

  **Investigation Summary**:
  - Alert: %s
  - Root Cause: %s
  - Confidence: %.0f%%

  **Recommended Workflow**: %s
  **Rationale**: %s

  **Why Approval Required**: Confidence between 60-79%% requires manual review

  **Approval Timeout**: %s

  **Action Required**: Review investigation details and approve/reject workflow execution
  `,
  alertName, alertName, rootCause, confidence*100,
  workflowName, rationale, approvalTimeout)
  ```

**V1.1 Enhancement**:
- We will consider creating an approval-specific template that automatically formats:
  - Investigation summary
  - Confidence score visualization
  - Approval countdown timer
  - Approve/Reject action links (if ActionLinks populated)
  - Context-rich formatting for Slack/Teams

---

#### 2. **Action Links**: Do you support `ActionLinks` in the spec for approve/reject buttons?

**Answer**: ✅ **YES - Fully Supported**

**Implementation Status**:
- ✅ `ActionLink` struct defined in API types (lines 128-137)
- ✅ `ActionLinks []ActionLink` field in NotificationRequestSpec (lines 193-195)
- ✅ Supports service name, URL, and label

**Usage Example for RO Team**:
```go
nr := &notificationv1.NotificationRequest{
    Spec: notificationv1.NotificationRequestSpec{
        Type:     notificationv1.NotificationTypeApproval,
        Priority: notificationv1.NotificationPriorityHigh,
        Subject:  "Approval Required: High Memory Usage on payment-api",
        Body:     "...",  // Rich approval context
        Channels: []notificationv1.Channel{notificationv1.ChannelSlack},
        ActionLinks: []notificationv1.ActionLink{
            {
                Service: "kubernaut-approval",
                URL:     "https://kubernaut.example.com/approve/rr-12345",
                Label:   "✅ Approve Workflow",
            },
            {
                Service: "kubernaut-rejection",
                URL:     "https://kubernaut.example.com/reject/rr-12345",
                Label:   "❌ Reject Workflow",
            },
            {
                Service: "kubernetes-dashboard",
                URL:     "https://k8s-dashboard.example.com/pod/payment-api",
                Label:   "🔍 View Pod Details",
            },
        },
    },
}
```

**Delivery Behavior**:
- **Slack**: ActionLinks rendered as buttons (if Slack Block Kit used)
- **Email**: ActionLinks rendered as clickable links
- **Console**: ActionLinks rendered as plain URLs

---

#### 3. **Routing**: Should `type=approval` have default routing rules (e.g., always include console channel)?

**Answer**: ✅ **Yes - Routing Rules Support Approval Type**

**Current Routing Support**:
- ✅ `kubernaut.ai/notification-type` label supports `approval` value
- ✅ Routing rules can match on notification type via labels
- ✅ First matching route wins (Alertmanager-compatible)

**Recommended Default Routing Rule** (for RO Team to add):
```yaml
# Example: notification-routing-config ConfigMap
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
        # Approval notifications → High-priority channels
        - match:
            kubernaut.ai/notification-type: approval
          receiver: approval-handlers
          continue: true  # Also send to default channels

        # ... other routes

    receivers:
      - name: approval-handlers
        slack_configs:
          - channel: "#approvals"
            webhook_url: "${SLACK_APPROVALS_WEBHOOK}"
        console_configs:
          - enabled: true  # Always log approvals to console

      - name: default
        console_configs:
          - enabled: true
```

**Behavior**:
- `type=approval` notifications will match the approval-specific route
- `continue: true` allows matching subsequent routes (multi-channel fanout)
- Console channel recommended for audit trail of all approval requests

**RO Team Action**: Add routing rules to your deployment manifests

---

#### 4. **Timeout Display**: Can the notification include countdown/timeout information?

**Answer**: ⚠️ **Not Automated, But Supported via Body/Metadata**

**Current Capabilities**:
- ✅ RO can include timeout information in `Body` field (formatted text)
- ✅ RO can add timeout to `Metadata` map for programmatic access
- ❌ Notification Service does NOT provide countdown timers or automatic updates

**Recommendation for RO Team**:
```go
nr := &notificationv1.NotificationRequest{
    Spec: notificationv1.NotificationRequestSpec{
        Type:     notificationv1.NotificationTypeApproval,
        Subject:  "Approval Required: High Memory Usage",
        Body: fmt.Sprintf(`
        ⏰ **Approval Timeout**: %s (%s remaining)
        **Expires At**: %s

        If no response is received, the remediation will be skipped.
        `,
        approvalTimeout,                                    // "15m"
        time.Until(approvalDeadline).Round(time.Second),  // "14m 32s"
        approvalDeadline.Format(time.RFC3339)),           // "2025-12-07T10:45:00Z"

        Metadata: map[string]string{
            "approvalTimeout":   "15m",
            "approvalDeadline":  approvalDeadline.Format(time.RFC3339),
            "approvalStartedAt": time.Now().Format(time.RFC3339),
        },
    },
}
```

**V1.1 Consideration**:
- Dynamic countdown timers in Slack (requires periodic message updates)
- Automatic expiration notifications when timeout reached
- Currently out of scope for V1.0

---

### Additional Findings from Triage

#### 1. **Multiple Notification Types Already Supported**

The API actually supports **5 notification types** (not just 4):
```go
// Line 26: +kubebuilder:validation:Enum=escalation;simple;status-update;approval;manual-review
const (
    NotificationTypeEscalation       NotificationType = "escalation"
    NotificationTypeSimple           NotificationType = "simple"
    NotificationTypeStatusUpdate     NotificationType = "status-update"
    NotificationTypeApproval         NotificationType = "approval"        // BR-ORCH-001
    NotificationTypeManualReview     NotificationType = "manual-review"   // BR-ORCH-036
)
```

**Note**: `manual-review` was added for ExhaustedRetries/PreviousExecutionFailed scenarios (BR-ORCH-036). This is **distinct from `approval`** for label-based routing purposes.

---

#### 2. **Spec Immutability Considerations**

**Important for RO Team**: NotificationRequest spec is **IMMUTABLE** after creation per DD-NOT-005.

**Implication for Approval Timeouts**:
- Once created, timeout information in `Body` or `Metadata` **cannot be updated**
- If you need to update countdown, you must create a **new NotificationRequest**
- For approval workflows, recommend creating notification **once** with static timeout display

---

#### 3. **Retention Defaults**

**Default Retention**: 7 days after completion
**Configurable**: 1-90 days via `spec.retentionDays`

**Recommendation for Approval Notifications**:
```go
RetentionDays: 30  // Keep approval records longer for audit compliance
```

---

### Summary of Current Support

| Feature | Status | Implementation |
|---------|--------|----------------|
| **CRD Enum Validation** | ✅ Complete | api/notification/v1alpha1/notificationrequest_types.go:26-35 |
| **Routing Rules** | ✅ Complete | pkg/notification/routing/ (label-based matching) |
| **ActionLinks** | ✅ Complete | api/notification/v1alpha1/notificationrequest_types.go:128-137, 193-195 |
| **Approval Template** | ⚠️ V1.1 | RO team can format Body field for V1.0 |
| **Countdown Timers** | ⚠️ V1.1 | RO team can include static timeout text for V1.0 |

---

### Recommended RO Team Actions for V1.0

1. ✅ **Use `NotificationTypeApproval`** - Already available in API
2. ✅ **Populate ActionLinks** - For approve/reject buttons
3. ✅ **Format Body field** - Include investigation summary, confidence, timeout
4. ✅ **Add Metadata** - Include approval timeout, deadline, workflow info
5. ✅ **Set Routing Labels** - Use `kubernaut.ai/notification-type=approval`
6. ✅ **Configure Retention** - Set `retentionDays: 30` for audit compliance

---

### V1.1 Enhancement Candidates

Based on this notice, we'll consider for V1.1:
1. Approval-specific notification template
2. Dynamic countdown timer support (Slack message updates)
3. Automatic expiration notifications
4. Approval workflow integration (track approve/reject responses)

---

**Notification Service Team Confidence**: **100%** - All required V1.0 functionality is implemented and tested.

---

## Related Documents

- [BR-ORCH-001: Approval Notification Creation](../requirements/BR-ORCH-001-approval-notification-creation.md)
- [ADR-018: Approval Notification V1.0 Integration](../architecture/decisions/ADR-018-approval-notification-v1-integration.md)
- [NotificationRequest CRD Schema](../services/crd-controllers/notification/crd-schema.md)

---

**Document Version**: 1.0
**Last Updated**: December 6, 2025
**Maintained By**: RemediationOrchestrator Team


