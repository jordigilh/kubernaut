# User Guide: Notification Cancellation Workflow

**Service**: Remediation Orchestrator
**Feature**: User-Initiated Notification Cancellation (BR-ORCH-029)
**Audience**: Kubernetes Operators, SREs
**Date**: December 13, 2025
**Version**: 1.0

---

## 📋 Overview

Kubernaut's Remediation Orchestrator allows operators to cancel notifications before they are delivered. This is useful for:
- Stopping spam/duplicate notifications
- Canceling notifications for resolved issues
- Managing notification fatigue during incidents

**Key Principle**: Canceling a notification does NOT cancel the remediation. The remediation continues regardless of notification status.

---

## 🎯 Use Cases

### **Use Case 1: Stop Spam Notifications**

**Scenario**: Multiple similar signals generated spam notifications

**Action**: Delete unwanted NotificationRequests to prevent delivery

**Outcome**: Notifications are cancelled, but remediations continue

---

### **Use Case 2: Issue Already Resolved**

**Scenario**: Operator manually fixed the issue before notification was sent

**Action**: Cancel the notification since it's no longer relevant

**Outcome**: No notification sent, remediation completes normally

---

### **Use Case 3: Notification Fatigue**

**Scenario**: During a major incident, too many notifications are being sent

**Action**: Cancel non-critical notifications to reduce noise

**Outcome**: Only critical notifications are delivered

---

## 🛠️ How to Cancel a Notification

### **Step 1: List NotificationRequests**

```bash
# List all NotificationRequests for a RemediationRequest (Issue #91: use field selector, not labels)
kubectl get notificationrequests -n <namespace> --field-selector spec.remediationRequestRef.name=<rr-name>

# Example output:
# NAME                    TYPE       PRIORITY   STATUS      AGE
# nr-approval-rr-123      approval   high       Sending     30s
# nr-bulk-rr-123          simple     low        Pending     10s
```

### **Step 2: Inspect Notification Details**

```bash
# Get notification details
kubectl get notificationrequest <notif-name> -n <namespace> -o yaml

# Check status.phase to see delivery status
# Phases: Pending, Sending, Sent, Failed
```

### **Step 3: Delete the NotificationRequest**

```bash
# Cancel the notification
kubectl delete notificationrequest <notif-name> -n <namespace>

# Example:
kubectl delete notificationrequest nr-approval-rr-123 -n default
```

**Result**: Notification is cancelled and will not be delivered

---

## 📊 Checking Cancellation Status

### **View RemediationRequest Status**

```bash
# Check RemediationRequest notification status
kubectl get remediationrequest <rr-name> -n <namespace> -o jsonpath='{.status.notificationStatus}'

# Possible values:
# - Pending: Notification not yet sent
# - InProgress: Notification being delivered
# - Sent: Notification delivered successfully
# - Failed: Notification delivery failed
# - Cancelled: Notification cancelled by user
```

### **View Conditions**

```bash
# Check NotificationDelivered condition
kubectl get remediationrequest <rr-name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="NotificationDelivered")]}'

# Example output (cancelled):
# {
#   "type": "NotificationDelivered",
#   "status": "False",
#   "reason": "UserCancelled",   # Constant: remediationrequest.ReasonUserCancelled
#   "message": "NotificationRequest deleted by user",
#   "lastTransitionTime": "2025-12-13T10:30:00Z"
# }
```

---

## ⚠️ Important Considerations

### **1. Cancellation Does NOT Stop Remediation**

**CRITICAL**: Deleting a NotificationRequest only cancels the notification. The remediation workflow continues.

```
❌ INCORRECT ASSUMPTION:
"If I delete the notification, the remediation will stop"

✅ CORRECT BEHAVIOR:
"Deleting the notification cancels the notification ONLY.
 The remediation continues independently."
```

**Example**:
```bash
# Delete notification
kubectl delete notificationrequest nr-approval-rr-123 -n default

# RemediationRequest continues:
kubectl get remediationrequest rr-123 -n default
# Status: Executing (remediation still running)
```

### **2. Cannot Cancel After Delivery**

Once a notification reaches `Sent` status, it cannot be cancelled (it's already delivered).

```bash
# Check if notification was already sent
kubectl get notificationrequest nr-approval-rr-123 -n default -o jsonpath='{.status.phase}'
# Output: Sent

# Deleting it now has no effect (already delivered)
kubectl delete notificationrequest nr-approval-rr-123 -n default
# Notification was already sent to operators
```

### **3. Cascade Deletion**

If you delete the parent RemediationRequest, all child NotificationRequests are automatically deleted (cascade deletion).

```bash
# Delete RemediationRequest
kubectl delete remediationrequest rr-123 -n default

# All NotificationRequests for rr-123 are automatically deleted
# This is expected cleanup behavior
```

---

## 📈 Monitoring Cancellations

### **Prometheus Metrics**

**Metric**: `kubernaut_remediationorchestrator_notification_cancellations_total`

**Labels**: `namespace`

**Query Examples**:

```promql
# Total cancellations across all namespaces
sum(kubernaut_remediationorchestrator_notification_cancellations_total)

# Cancellations per namespace
kubernaut_remediationorchestrator_notification_cancellations_total

# Cancellation rate (per minute)
rate(kubernaut_remediationorchestrator_notification_cancellations_total[5m])
```

### **Notification Status Distribution**

**Metric**: `kubernaut_remediationorchestrator_notification_status`

**Labels**: `namespace`, `status`

**Query Examples**:

```promql
# Current notification status distribution
kubernaut_remediationorchestrator_notification_status

# Count of cancelled notifications
kubernaut_remediationorchestrator_notification_status{status="Cancelled"}

# Percentage of cancelled notifications
(
  kubernaut_remediationorchestrator_notification_status{status="Cancelled"}
  /
  sum(kubernaut_remediationorchestrator_notification_status)
) * 100
```

---

## 🔍 Troubleshooting

### **Problem**: Notification still delivered after deletion

**Cause**: Notification was already in `Sending` or `Sent` status

**Solution**: Check notification status before deleting. Once delivery starts, cancellation may not prevent delivery.

```bash
# Always check status first
kubectl get notificationrequest <notif-name> -n <namespace> -o jsonpath='{.status.phase}'

# If "Pending" → Safe to cancel
# If "Sending" → May still be delivered
# If "Sent" → Already delivered (cannot cancel)
```

---

### **Problem**: RemediationRequest shows "Cancelled" but workflow is still running

**Expected Behavior**: This is correct! `notificationStatus: Cancelled` means the NOTIFICATION was cancelled, not the remediation.

**Verification**:
```bash
# Check remediation phase (should still be active)
kubectl get remediationrequest <rr-name> -n <namespace> -o jsonpath='{.status.overallPhase}'
# Output: Executing (remediation continues)

# Check notification status (shows cancellation)
kubectl get remediationrequest <rr-name> -n <namespace> -o jsonpath='{.status.notificationStatus}'
# Output: Cancelled
```

---

### **Problem**: How to cancel the remediation itself?

**Answer**: To stop the remediation, delete the RemediationRequest (not the NotificationRequest).

```bash
# To cancel the REMEDIATION:
kubectl delete remediationrequest <rr-name> -n <namespace>

# This will:
# 1. Stop the remediation workflow
# 2. Cascade delete all child CRDs (including NotificationRequests)
# 3. Clean up all resources
```

---

## 📚 Related Documentation

- [BR-ORCH-029-031-notification-handling.md](../../../requirements/BR-ORCH-029-031-notification-handling.md) - Business requirements
- [DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md](./DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md) - Design decision
- [BR-ORCH-034-IMPLEMENTATION.md](./BR-ORCH-034-IMPLEMENTATION.md) - Bulk notification implementation
- [METRICS.md](./METRICS.md) - Metrics documentation

---

## 🎯 Best Practices

### **1. Check Status Before Cancelling**

Always verify notification status before deletion to avoid confusion.

```bash
# Good practice: Check first
kubectl get notificationrequest <notif-name> -n <namespace> -o jsonpath='{.status.phase}'

# Then delete if appropriate
kubectl delete notificationrequest <notif-name> -n <namespace>
```

### **2. Use Field Selectors for Bulk Operations**

Cancel multiple notifications efficiently using field selectors (Issue #91: labels replaced by spec fields).

```bash
# Cancel all notifications for a specific RR (Issue #91: use field selector)
kubectl delete notificationrequests -n <namespace> --field-selector spec.remediationRequestRef.name=<rr-name>
```

### **3. Monitor Cancellation Patterns**

Track cancellation rates to identify notification spam issues.

```promql
# Alert if cancellation rate is too high (indicates spam)
rate(kubernaut_remediationorchestrator_notification_cancellations_total[5m]) > 0.5
```

### **4. Document Cancellation Reasons**

When cancelling notifications during incidents, document why in your incident log.

```bash
# Example incident log entry:
# "Cancelled 15 approval notifications for rr-* due to duplicate signals during incident INC-123"
```

---

## ❓ FAQ

**Q: Will cancelling a notification affect the remediation?**
A: No. Notification cancellation is independent of remediation execution.

**Q: Can I cancel a notification that's already been sent?**
A: No. Once `status.phase` is `Sent`, the notification has been delivered.

**Q: What happens if I delete the RemediationRequest?**
A: All child NotificationRequests are automatically deleted (cascade deletion). This is expected cleanup behavior.

**Q: How do I know if a notification was cancelled by me vs. cascade deleted?**
A: Check `remediationRequest.deletionTimestamp`. If set, it's cascade deletion. If not set, it's user cancellation.

**Q: Can I un-cancel a notification?**
A: No. Once deleted, the NotificationRequest cannot be recovered. The RemediationOrchestrator will not recreate it.

---

**Document Version**: 1.0
**Last Updated**: December 13, 2025
**Feedback**: Report issues or suggestions to the Kubernaut team


