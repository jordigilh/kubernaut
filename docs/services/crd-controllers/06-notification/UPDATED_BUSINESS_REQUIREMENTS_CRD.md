# Notification Service - Updated Business Requirements (CRD-Based Architecture)

**Date**: 2025-10-12
**Version**: 2.0 - CRD-Based Architecture
**Status**: ✅ **APPROVED**
**Previous Version**: 1.0 (Imperative REST API - Deprecated)

---

## 🎯 **Architectural Change Summary**

**From**: Stateless HTTP API (Imperative)
**To**: CRD Controller + Internal Delivery Service (Declarative)

**Rationale**:
- ✅ Prevents data loss (etcd persistence)
- ✅ Ensures complete audit trail (CRD status tracking)
- ✅ Provides automatic retry (controller reconciliation)
- ✅ Guarantees delivery (at-least-once semantics)
- ✅ Full observability (CRD status, events, metrics)

**Confidence**: 95% (vs 45% with REST API)

---

## 📋 **NEW Business Requirements (Data Loss Prevention)**

### **🔴 BR-NOT-050: Zero Data Loss** (CRITICAL - NEW)
**Category**: Reliability
**Priority**: P0 - CRITICAL

**Requirement**: Notification Service MUST guarantee zero data loss for notification requests.

**Acceptance Criteria**:
- ✅ All notification requests MUST be persisted in etcd immediately upon creation
- ✅ Notification requests MUST survive service restarts, pod crashes, and node failures
- ✅ In-flight delivery attempts MUST resume after service recovery
- ✅ No in-memory-only state that could be lost

**Implementation**:
- Use NotificationRequest CRD for persistent storage
- Kubernetes etcd provides durability and replication
- Controller reconciliation resumes failed deliveries

**Validation**:
```bash
# Test: Create notification, kill pod, verify delivery resumes
kubectl apply -f notification-request.yaml
kubectl delete pod notification-controller-xxx
# Notification MUST still be delivered after pod restart
```

**Confidence**: 100% (etcd persistence guarantees)

---

### **🔴 BR-NOT-051: Complete Audit Trail** (CRITICAL - NEW)
**Category**: Compliance
**Priority**: P0 - CRITICAL

**Requirement**: Notification Service MUST provide complete audit trail for all notification activities.

**Acceptance Criteria**:
- ✅ ALL notification requests MUST be recorded with creation timestamp
- ✅ EVERY delivery attempt MUST be tracked (channel, timestamp, status, error)
- ✅ Final delivery status MUST be recorded (sent, delivered, failed)
- ✅ Retry count and backoff intervals MUST be tracked
- ✅ Audit trail MUST be queryable via Kubernetes API
- ✅ Audit trail MUST be retained for minimum 90 days

**Implementation**:
- NotificationRequest CRD status field tracks all attempts
- Each DeliveryAttempt recorded with full context
- Kubernetes event log captures state transitions
- Integration with Data Storage service for long-term retention

**Validation**:
```bash
# Query notification history
kubectl get notificationrequests -n kubernaut-system

# View detailed audit trail
kubectl describe notificationrequest escalation-remediation-001

# Check specific delivery attempts
kubectl get notificationrequest escalation-remediation-001 -o jsonpath='{.status.deliveryAttempts}'
```

**Compliance**: SOC2, HIPAA, PCI-DSS audit requirements

---

### **🔴 BR-NOT-052: Automatic Retry with Exponential Backoff** (CRITICAL - NEW)
**Category**: Reliability
**Priority**: P0 - CRITICAL

**Requirement**: Notification Service MUST automatically retry failed deliveries without caller intervention.

**Acceptance Criteria**:
- ✅ Failed deliveries MUST be retried automatically by controller
- ✅ Retry MUST use exponential backoff (30s, 60s, 120s, 240s, 480s)
- ✅ Maximum retry attempts MUST be configurable per notification (default: 5)
- ✅ Retry policy MUST be configurable per notification priority
- ✅ Transient errors (timeout, 503) MUST be retried
- ✅ Permanent errors (401, 404, invalid recipient) MUST NOT be retried
- ✅ Retry attempts MUST be tracked in CRD status

**Implementation**:
```yaml
spec:
  retryPolicy:
    maxAttempts: 5
    initialBackoffSeconds: 30
    backoffMultiplier: 2
    maxBackoffSeconds: 480
```

**Retry Logic**:
- Attempt 1: Immediate
- Attempt 2: +30s
- Attempt 3: +60s (30s × 2)
- Attempt 4: +120s (60s × 2)
- Attempt 5: +240s (120s × 2)
- Attempt 6: +480s (capped at max)

**Validation**:
```go
// Test: Simulate Slack webhook timeout
mockSlack.SetResponse(503, "Service Unavailable")

// Create notification
notification := createNotification()

// Verify retry attempts
Eventually(func() int {
    n := getNotification(notification.Name)
    return n.Status.TotalAttempts
}).Should(Equal(5))
```

**Confidence**: 95% (controller-runtime reconciliation guarantees)

---

### **🟡 BR-NOT-053: At-Least-Once Delivery Guarantee** (HIGH - NEW)
**Category**: Reliability
**Priority**: P1 - HIGH

**Requirement**: Notification Service MUST guarantee at-least-once delivery for all notifications.

**Acceptance Criteria**:
- ✅ Every notification MUST be delivered to ALL specified channels at least once
- ✅ Duplicate deliveries ARE acceptable (at-least-once semantics)
- ✅ Delivery MUST be retried until max attempts reached OR success
- ✅ Permanent failures MUST be marked as Failed with clear reason
- ✅ Success MUST be confirmed by external service (200 OK, Slack API success)

**Implementation**:
- Controller reconciliation loop ensures delivery
- CRD status.phase tracks delivery progress:
  - Pending → Sending → Sent (all channels succeeded)
  - Pending → Sending → Failed (max retries exceeded)
- Owner references ensure notification cleanup with parent resource

**Validation**:
```bash
# Test: Create notification, verify delivery confirmed
kubectl apply -f notification-request.yaml

# Check status
kubectl get notificationrequest test-notification -o jsonpath='{.status.phase}'
# Expected: Sent (after successful delivery to all channels)
```

**Note**: Exactly-once delivery is NOT guaranteed (trade-off for reliability)

---

### **🟡 BR-NOT-054: Delivery Status Observability** (HIGH - NEW)
**Category**: Observability
**Priority**: P1 - HIGH

**Requirement**: Notification Service MUST provide real-time visibility into delivery status.

**Acceptance Criteria**:
- ✅ Notification phase MUST be queryable (Pending, Sending, Sent, Failed)
- ✅ Per-channel delivery status MUST be visible
- ✅ Failure reasons MUST be recorded with actionable error messages
- ✅ Delivery duration MUST be tracked (time from request to delivery)
- ✅ Metrics MUST be exposed for monitoring (delivery rate, failure rate, latency)
- ✅ Kubernetes events MUST be emitted for state transitions

**Implementation**:
```yaml
status:
  phase: Sending
  conditions:
  - type: EmailSent
    status: "True"
    reason: SMTPSuccess
    message: "Email delivered to oncall@company.com at 2025-10-12T10:30:00Z"
  - type: SlackSent
    status: "False"
    reason: WebhookTimeout
    message: "Slack webhook timed out after 10s, retry scheduled"
  deliveryAttempts:
  - channel: email
    attempt: 1
    timestamp: "2025-10-12T10:30:00Z"
    status: success
  - channel: slack
    attempt: 1
    timestamp: "2025-10-12T10:30:05Z"
    status: failed
    error: "webhook timeout"
```

**Prometheus Metrics**:
```
notification_delivery_total{channel="email",status="success"} 1523
notification_delivery_total{channel="slack",status="failed"} 12
notification_delivery_duration_seconds{channel="email",quantile="0.95"} 0.8
notification_retry_count{channel="slack"} 42
notification_requests_total{priority="critical",phase="Sent"} 456
```

**Validation**:
```bash
# Query all notifications
kubectl get notificationrequests -n kubernaut-system

# Filter by status
kubectl get notificationrequests --field-selector status.phase=Failed

# Watch notifications in real-time
watch kubectl get notificationrequests -n kubernaut-system
```

---

### **🟡 BR-NOT-055: Graceful Degradation** (HIGH - NEW)
**Category**: Reliability
**Priority**: P1 - HIGH

**Requirement**: Notification Service MUST continue operating when individual channels fail.

**Acceptance Criteria**:
- ✅ Failure of one channel MUST NOT block other channels
- ✅ Email failure MUST NOT prevent Slack delivery
- ✅ Notification MUST be marked as Partial Success if some channels succeed
- ✅ Failed channels MUST be retried independently
- ✅ Circuit breaker MUST prevent cascading failures (after 5 consecutive failures)

**Implementation**:
```go
// Deliver to each channel independently
for _, channel := range notification.Spec.Channels {
    go func(ch Channel) {
        err := deliverToChannel(ctx, notification, ch)
        if err != nil {
            recordFailure(ch, err)
            // Schedule retry for this channel only
        } else {
            recordSuccess(ch)
        }
    }(channel)
}
```

**Status Tracking**:
```yaml
status:
  phase: PartiallySent  # New phase
  conditions:
  - type: EmailSent
    status: "True"
  - type: SlackSent
    status: "False"
    reason: CircuitBreakerOpen
    message: "Circuit breaker open after 5 consecutive failures, retry in 5 minutes"
```

**Validation**:
```bash
# Test: Disable Slack webhook, verify email still delivered
kubectl annotate notificationrequest test-notification \
  test.kubernaut.ai/disable-slack="true"

# Notification MUST show:
# - Email: Sent (success)
# - Slack: Failed (webhook disabled)
# - Overall phase: PartiallySent
```

---

## 📋 **UPDATED Business Requirements (Existing BRs with CRD Context)**

### **BR-NOT-026: Comprehensive Alert Context** (UPDATED)
**Category**: Notification Content
**Priority**: P0 - CRITICAL

**Requirement**: Notification Service MUST provide comprehensive alert context in escalation notifications.

**Acceptance Criteria** (UPDATED):
- ✅ Alert metadata MUST be included (name, namespace, cluster, severity)
- ✅ Impacted resources MUST be listed
- ✅ AI analysis summary MUST be included
- ✅ Context MUST be persisted in NotificationRequest CRD for audit trail
- ✅ **NEW**: Context MUST be retrievable after delivery via CRD status

**Implementation** (UPDATED):
```yaml
apiVersion: notification.kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: escalation-remediation-001-timeout
spec:
  type: escalation
  metadata:
    remediationRequestName: remediation-001
    cluster: prod-us-west-2
    severity: P0
    alertName: "HighMemoryUsage"
  # ... rest of notification content
status:
  deliveredContent: "Archived copy of delivered notification content"
```

---

### **BR-NOT-034: Sensitive Data Sanitization** (UPDATED)
**Category**: Security
**Priority**: P0 - CRITICAL

**Requirement**: Notification Service MUST sanitize sensitive data before delivery.

**Acceptance Criteria** (UPDATED):
- ✅ Secrets, API keys, passwords MUST be redacted
- ✅ PII MUST be sanitized per policy
- ✅ **NEW**: Sanitization MUST occur before CRD status is written (prevent etcd exposure)
- ✅ **NEW**: Sanitized content MUST be marked in audit trail
- ✅ **NEW**: Original unsanitized content MUST NEVER be persisted in CRD

**Implementation** (UPDATED):
```go
// Sanitize BEFORE creating NotificationRequest CRD
sanitizedBody := sanitizer.Sanitize(notification.Body)
sanitizedSubject := sanitizer.Sanitize(notification.Subject)

// Create CRD with sanitized content only
notificationRequest := &NotificationRequest{
    Spec: NotificationRequestSpec{
        Subject: sanitizedSubject,
        Body:    sanitizedBody,
        Metadata: map[string]string{
            "sanitized": "true",
            "redactedFields": "password,apiKey,connectionString",
        },
    },
}
```

**Security Note**: CRD is stored in etcd, which may be backed up. NEVER store unsanitized sensitive data in CRD.

---

### **BR-NOT-001 to BR-NOT-005: Multi-Channel Delivery** (UPDATED)
**Category**: Notification Delivery
**Priority**: P0 - CRITICAL

**Requirements**: (UNCHANGED from v1.0)
- BR-NOT-001: Email with rich formatting
- BR-NOT-002: Slack integration
- BR-NOT-003: Console/stdout for development
- BR-NOT-004: SMS notifications
- BR-NOT-005: Microsoft Teams integration

**Acceptance Criteria** (UPDATED):
- ✅ All existing criteria from v1.0
- ✅ **NEW**: Channel delivery status MUST be tracked in CRD per BR-NOT-054
- ✅ **NEW**: Failed channels MUST be retried automatically per BR-NOT-052
- ✅ **NEW**: Channel failures MUST NOT block other channels per BR-NOT-055

---

### **BR-NOT-036: Channel-Specific Formatting** (UPDATED)
**Category**: Notification Formatting
**Priority**: P1 - HIGH

**Requirement**: Notification Service MUST provide channel-specific formatting adapters.

**Acceptance Criteria** (UPDATED):
- ✅ Email: Rich HTML (1MB limit)
- ✅ Slack: Block Kit (40KB limit)
- ✅ Teams: Adaptive Cards (28KB limit)
- ✅ SMS: Plain text (160 chars)
- ✅ **NEW**: Formatted content MUST be generated from NotificationRequest CRD spec
- ✅ **NEW**: Formatting errors MUST be recorded in CRD status

**Implementation** (UPDATED):
```go
// Controller retrieves NotificationRequest CRD
notification := &NotificationRequest{}
r.Get(ctx, req.NamespacedName, notification)

// Generate channel-specific content
for _, channel := range notification.Spec.Channels {
    formattedContent, err := r.formatter.Format(notification, channel)
    if err != nil {
        // Record formatting error in CRD status
        notification.Status.Conditions = append(notification.Status.Conditions, metav1.Condition{
            Type:   fmt.Sprintf("%sFormatted", channel),
            Status: metav1.ConditionFalse,
            Reason: "FormattingError",
            Message: err.Error(),
        })
        continue
    }

    // Deliver formatted content
    err = r.deliveryService.Deliver(ctx, channel, formattedContent)
    // ... handle delivery result
}
```

---

### **BR-NOT-037: External Service Action Links** (UPDATED)
**Category**: Notification Content
**Priority**: P1 - HIGH

**Requirement**: Notification Service MUST provide action links to external services.

**Acceptance Criteria** (UPDATED):
- ✅ GitHub, GitLab, Grafana, Prometheus, K8s Dashboard links
- ✅ Authentication enforced by target service (NOT by Notification Service)
- ✅ **NEW**: Action links MUST be generated from NotificationRequest CRD metadata
- ✅ **NEW**: Link generation errors MUST be tracked in CRD status

**Implementation** (UPDATED):
```yaml
spec:
  metadata:
    remediationRequestName: remediation-001
    cluster: prod-us-west-2
    namespace: production
    podName: webapp-xyz
  actionLinks:
  - service: grafana
    url: "https://grafana.company.com/d/kubernetes-pod?var-pod=webapp-xyz&var-namespace=production"
    label: "View Pod Metrics"
  - service: kubernetes-dashboard
    url: "https://k8s-dashboard.company.com/#!/pod/production/webapp-xyz"
    label: "View Pod Details"
  - service: github
    url: "https://github.com/company/webapp/issues/new?title=Pod+webapp-xyz+failing"
    label: "Create GitHub Issue"
```

---

## 📋 **NEW Business Requirements (CRD-Specific)**

### **🟢 BR-NOT-056: CRD Lifecycle Management** (MEDIUM - NEW)
**Category**: Resource Management
**Priority**: P2 - MEDIUM

**Requirement**: NotificationRequest CRD MUST be properly managed throughout its lifecycle.

**Acceptance Criteria**:
- ✅ NotificationRequest MUST use owner references to parent resource (RemediationRequest, AIAnalysisRequest, etc.)
- ✅ NotificationRequest MUST be deleted when parent is deleted (cascading deletion)
- ✅ Completed NotificationRequests MUST be retained for 7 days (configurable)
- ✅ Failed NotificationRequests MUST be retained for 30 days (compliance)
- ✅ Cleanup controller MUST delete old NotificationRequests based on retention policy

**Implementation**:
```yaml
apiVersion: notification.kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: escalation-remediation-001
  ownerReferences:
  - apiVersion: remediation.kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: remediation-001
    uid: abc-123
    controller: true
    blockOwnerDeletion: true
  annotations:
    notification.kubernaut.ai/retention-days: "7"
    notification.kubernaut.ai/cleanup-after: "2025-10-19T10:30:00Z"
```

**Cleanup Logic**:
```go
// Periodic cleanup job (runs hourly)
func (r *Reconciler) cleanupOldNotifications(ctx context.Context) error {
    notifications := &NotificationRequestList{}
    r.List(ctx, notifications)

    for _, n := range notifications.Items {
        if n.Status.Phase == NotificationPhaseSent || n.Status.Phase == NotificationPhaseFailed {
            retentionDays := getRetentionDays(n)
            if time.Since(n.Status.CompletionTime.Time) > time.Duration(retentionDays)*24*time.Hour {
                r.Delete(ctx, &n)
            }
        }
    }
}
```

---

### **🟢 BR-NOT-057: Notification Priority and Ordering** (MEDIUM - NEW)
**Category**: Resource Management
**Priority**: P2 - MEDIUM

**Requirement**: High-priority notifications MUST be delivered before low-priority notifications.

**Acceptance Criteria**:
- ✅ Notification priority MUST be specified (critical, high, medium, low)
- ✅ Critical notifications MUST be processed immediately
- ✅ Lower priority notifications MAY be queued if system is under load
- ✅ Priority MUST NOT affect retry behavior (all retried per BR-NOT-052)
- ✅ Priority MUST be visible in CRD status and metrics

**Implementation**:
```yaml
spec:
  priority: critical  # critical, high, medium, low

status:
  queuedAt: "2025-10-12T10:30:00Z"
  processingStartedAt: "2025-10-12T10:30:00Z"  # Immediate for critical
  processingCompletedAt: "2025-10-12T10:30:02Z"
  queueWaitTime: "0s"  # Zero for critical priority
```

**Controller Logic**:
```go
// Priority-based reconciliation
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&NotificationRequest{}).
        WithOptions(controller.Options{
            MaxConcurrentReconciles: 10,
            RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(30*time.Second, 480*time.Second),
        }).
        WithEventFilter(predicate.Funcs{
            CreateFunc: func(e event.CreateEvent) bool {
                notification := e.Object.(*NotificationRequest)
                // Process critical notifications immediately
                return notification.Spec.Priority == "critical" ||
                    notification.Spec.Priority == "high"
            },
        }).
        Complete(r)
}
```

---

### **🟢 BR-NOT-058: Notification Request Validation** (MEDIUM - NEW)
**Category**: Data Integrity
**Priority**: P2 - MEDIUM

**Requirement**: NotificationRequest CRD MUST be validated before acceptance.

**Acceptance Criteria**:
- ✅ Required fields MUST be validated (recipients, subject, body, channels)
- ✅ Invalid channel names MUST be rejected
- ✅ Invalid priority values MUST be rejected
- ✅ Recipient format MUST be validated (email format, Slack channel format)
- ✅ Validation errors MUST return clear error messages
- ✅ Validation MUST occur via admission webhook

**Implementation**:
```go
// Validating webhook
func (v *NotificationRequestValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
    notification := obj.(*NotificationRequest)

    // Validate recipients
    if len(notification.Spec.Recipients) == 0 {
        return fmt.Errorf("at least one recipient is required")
    }

    // Validate channels
    validChannels := []string{"email", "slack", "teams", "sms"}
    for _, channel := range notification.Spec.Channels {
        if !contains(validChannels, string(channel)) {
            return fmt.Errorf("invalid channel: %s", channel)
        }
    }

    // Validate priority
    validPriorities := []string{"critical", "high", "medium", "low"}
    if !contains(validPriorities, notification.Spec.Priority) {
        return fmt.Errorf("invalid priority: %s", notification.Spec.Priority)
    }

    return nil
}
```

---

## 📊 **Business Requirements Summary**

### **Requirements by Category**

| Category | Count | Priority Distribution |
|----------|-------|----------------------|
| **Data Loss Prevention** (NEW) | 5 | P0: 3, P1: 2 |
| **Notification Content** | 12 | P0: 8, P1: 4 |
| **Multi-Channel Delivery** | 5 | P0: 5 |
| **Resource Management** (NEW) | 3 | P2: 3 |
| **Security** | 1 | P0: 1 |
| **Observability** (NEW) | 1 | P1: 1 |
| **Total** | **27** | **P0: 17, P1: 7, P2: 3** |

### **NEW vs UPDATED vs UNCHANGED**

| Status | Count | BRs |
|--------|-------|-----|
| **NEW** | 8 | BR-NOT-050 to BR-NOT-058 (Data loss prevention, audit trail, retry, observability, CRD lifecycle) |
| **UPDATED** | 6 | BR-NOT-001 to BR-NOT-005, BR-NOT-026, BR-NOT-034, BR-NOT-036, BR-NOT-037 (CRD context added) |
| **UNCHANGED** | 13 | BR-NOT-027 to BR-NOT-033, BR-NOT-035, BR-NOT-006 to BR-NOT-025 (Content and formatting) |

---

## ✅ **Data Loss Prevention Requirements Coverage**

| Concern | BR | Status |
|---------|-----|--------|
| **Data Loss** | BR-NOT-050 | ✅ Addressed (etcd persistence) |
| **Audit Trail** | BR-NOT-051 | ✅ Addressed (complete status tracking) |
| **Automatic Retry** | BR-NOT-052 | ✅ Addressed (controller reconciliation) |
| **Delivery Guarantee** | BR-NOT-053 | ✅ Addressed (at-least-once) |
| **Observability** | BR-NOT-054 | ✅ Addressed (CRD status + metrics) |
| **Graceful Degradation** | BR-NOT-055 | ✅ Addressed (independent channel delivery) |
| **CRD Lifecycle** | BR-NOT-056 | ✅ Addressed (owner references + cleanup) |
| **Priority Handling** | BR-NOT-057 | ✅ Addressed (priority-based processing) |
| **Validation** | BR-NOT-058 | ✅ Addressed (admission webhook) |

**Coverage**: 100% of data loss prevention concerns addressed

---

## 🎯 **Validation Strategy**

### **Unit Tests** (70%+ coverage)
```go
// Test BR-NOT-050: Zero data loss
func TestZeroDataLoss(t *testing.T) {
    // Create notification
    notification := createNotification()

    // Kill controller pod
    killControllerPod()

    // Verify notification still exists
    exists := notificationExists(notification.Name)
    assert.True(t, exists, "Notification must survive pod restart")

    // Verify delivery resumes
    Eventually(func() string {
        n := getNotification(notification.Name)
        return n.Status.Phase
    }).Should(Equal("Sent"))
}

// Test BR-NOT-051: Complete audit trail
func TestCompleteAuditTrail(t *testing.T) {
    notification := createNotification()

    // Simulate delivery failure + retry
    mockSlack.SetResponse(503, "Service Unavailable")

    // Wait for retries
    time.Sleep(5 * time.Minute)

    // Verify all attempts tracked
    n := getNotification(notification.Name)
    assert.Greater(t, len(n.Status.DeliveryAttempts), 1)
    assert.Equal(t, n.Status.TotalAttempts, 3)
}

// Test BR-NOT-052: Automatic retry
func TestAutomaticRetry(t *testing.T) {
    notification := createNotification()

    // Simulate transient failure
    mockEmail.SetTemporaryFailure()

    // Verify retry occurs automatically (no manual intervention)
    Eventually(func() int {
        n := getNotification(notification.Name)
        return n.Status.TotalAttempts
    }).Should(BeNumerically(">", 1))
}
```

### **Integration Tests** (>50% coverage)
```go
// Test BR-NOT-053: At-least-once delivery
func TestAtLeastOnceDelivery(t *testing.T) {
    suite := kind.Setup("notification-test")

    // Create notification with real channels
    notification := &NotificationRequest{
        Spec: NotificationRequestSpec{
            Channels: []Channel{"email", "slack"},
        },
    }
    suite.KindClient.Create(ctx, notification)

    // Verify delivery to both channels
    Eventually(func() string {
        n := &NotificationRequest{}
        suite.KindClient.Get(ctx, client.ObjectKeyFromObject(notification), n)
        return n.Status.Phase
    }).Should(Equal("Sent"))

    // Verify email delivered
    assert.True(t, emailWasDelivered())

    // Verify Slack message posted
    assert.True(t, slackMessageWasPosted())
}
```

### **E2E Tests** (<10% coverage)
```go
// Test complete notification flow
func TestEndToEndNotificationFlow(t *testing.T) {
    // Create RemediationRequest
    remediation := createRemediationRequest()

    // Trigger escalation (timeout)
    time.Sleep(30 * time.Minute)

    // Verify NotificationRequest created
    notification := findNotificationForRemediation(remediation.Name)
    assert.NotNil(t, notification)

    // Verify delivery completed
    assert.Equal(t, notification.Status.Phase, "Sent")

    // Verify audit trail complete
    assert.Greater(t, len(notification.Status.DeliveryAttempts), 0)
}
```

---

## 📚 **Related Documentation**

**Architecture**:
- [ARCHITECTURE_IMPERATIVE_VS_DECLARATIVE_TRIAGE.md](./ARCHITECTURE_IMPERATIVE_VS_DECLARATIVE_TRIAGE.md) - Architectural decision rationale
- [NotificationRequest CRD API Reference](./api/v1alpha1/notificationrequest_types.go) - CRD schema (to be created)
- [Notification Controller Design](./design/notification-controller-design.md) - Controller implementation (to be created)

**Business Context**:
- [overview.md](./overview.md) - Service overview (to be updated)
- [api-specification.md](./api-specification.md) - API specification (to be updated)

**Implementation**:
- [implementation-checklist.md](./implementation-checklist.md) - Implementation guide (to be updated)
- [testing-strategy.md](./testing-strategy.md) - Testing approach (to be updated)

---

## 🎯 **Next Steps**

1. ✅ **BR Update Complete** - This document
2. 📝 **CRD API Definition** - Define NotificationRequest CRD schema
3. 🏗️ **Controller Design** - Design controller reconciliation logic
4. 🔨 **Implementation Plan** - Create detailed implementation plan (similar to Data Storage v4.1)
5. 📊 **Update Service Documentation** - Update overview.md, api-specification.md with CRD approach

---

**Status**: ✅ Business Requirements Updated
**Confidence**: 95%
**Approval**: ✅ User Approved (CRD-based architecture)
**Ready for**: CRD API Design + Controller Implementation

