# Notification Controller - Audit Trace Specification

**Version**: 1.0
**Date**: 2025-11-03
**Status**: ✅ **PILOT PROJECT** - First controller to integrate with Data Storage Service audit API
**Purpose**: Define WHEN, WHERE, and WHAT audit traces must be written for Notification Controller
**Authority**: ADR-032 v1.3 - Data Access Layer Isolation (Phased Audit Table Development)

---

## 📋 **Executive Summary**

This document specifies the audit trace requirements for the Notification Controller, serving as the **pilot implementation** for the phased audit table development strategy. The Notification Controller is the **only fully implemented controller** with a finalized CRD spec and status, making it the ideal candidate for validating the Data Storage Service audit write API architecture.

**Key Decisions**:
- ✅ **Pilot Project**: Notification Controller validates audit architecture for future controllers
- ✅ **Real-Time Audit Writes**: Audit data written immediately after CRD status updates
- ✅ **Non-Blocking**: Audit write failures do NOT block notification delivery
- ✅ **DLQ Fallback**: Failed audit writes go to Dead Letter Queue (DD-009)
- ✅ **Best-Effort Delivery**: Notification delivery takes priority over audit persistence

---

## 🎯 **Audit Trace Objectives**

### **Business Requirements**
- **BR-NOTIFICATION-001**: Track all notification delivery attempts for compliance
- **BR-NOTIFICATION-002**: Record notification failures for debugging and retry logic
- **BR-NOTIFICATION-003**: Capture escalation events for SLA tracking
- **BR-NOTIFICATION-004**: Enable V2.0 Remediation Analysis Report (RAR) timeline reconstruction

### **Technical Requirements**
- **REQ-1**: Audit writes must be non-blocking (notification delivery is critical path)
- **REQ-2**: Audit data must match CRD status fields exactly (single source of truth)
- **REQ-3**: Audit writes must handle Data Storage Service unavailability gracefully
- **REQ-4**: Audit data must be immutable once written (append-only)
- **REQ-5**: Audit writes must include retry logic with DLQ fallback (DD-009)

---

## 📊 **NotificationRequest CRD Status Structure**

**Authority**: `api/notification/v1alpha1/notificationrequest_types.go`

```go
// NotificationRequestStatus defines the observed state of NotificationRequest
type NotificationRequestStatus struct {
    // Status of the notification (sent, failed, acknowledged, escalated)
    Status NotificationStatus `json:"status,omitempty"`
    
    // Detailed delivery status from provider (e.g., "200 OK", "rate_limited")
    DeliveryStatus string `json:"deliveryStatus,omitempty"`
    
    // Error message if notification failed
    ErrorMessage string `json:"errorMessage,omitempty"`
    
    // Timestamp when notification was sent
    SentAt *metav1.Time `json:"sentAt,omitempty"`
    
    // Timestamp when notification was acknowledged (if applicable)
    AcknowledgedAt *metav1.Time `json:"acknowledgedAt,omitempty"`
    
    // Current escalation level (0 = initial, 1 = first escalation, etc.)
    EscalationLevel int `json:"escalationLevel,omitempty"`
    
    // Conditions represent the latest available observations
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NotificationStatus represents the status of a notification
type NotificationStatus string

const (
    NotificationStatusPending      NotificationStatus = "pending"
    NotificationStatusSending      NotificationStatus = "sending"
    NotificationStatusSent         NotificationStatus = "sent"
    NotificationStatusFailed       NotificationStatus = "failed"
    NotificationStatusAcknowledged NotificationStatus = "acknowledged"
    NotificationStatusEscalated    NotificationStatus = "escalated"
)
```

---

## 🔄 **Audit Trace Trigger Points - WHEN to Write**

### **Trigger 1: Notification Sent Successfully**
**CRD Status Transition**: `pending` → `sent`

**Trigger Condition**:
```go
if notificationRequest.Status.Status == NotificationStatusSent && 
   oldStatus != NotificationStatusSent {
    // Write audit trace
}
```

**Audit Data Payload**:
```go
NotificationAudit{
    RemediationID:   notificationRequest.Spec.RemediationID,
    NotificationID:  notificationRequest.Name,
    Recipient:       notificationRequest.Spec.Recipient,
    Channel:         string(notificationRequest.Spec.Channel),
    MessageSummary:  notificationRequest.Spec.MessageSummary,
    Status:          "sent",
    SentAt:          notificationRequest.Status.SentAt.Time,
    DeliveryStatus:  notificationRequest.Status.DeliveryStatus,
    ErrorMessage:    "",
    EscalationLevel: notificationRequest.Spec.EscalationLevel,
}
```

**Business Value**: Confirms successful notification delivery for compliance tracking

---

### **Trigger 2: Notification Failed**
**CRD Status Transition**: `pending` → `failed` OR `sending` → `failed`

**Trigger Condition**:
```go
if notificationRequest.Status.Status == NotificationStatusFailed && 
   oldStatus != NotificationStatusFailed {
    // Write audit trace
}
```

**Audit Data Payload**:
```go
NotificationAudit{
    RemediationID:   notificationRequest.Spec.RemediationID,
    NotificationID:  notificationRequest.Name,
    Recipient:       notificationRequest.Spec.Recipient,
    Channel:         string(notificationRequest.Spec.Channel),
    MessageSummary:  notificationRequest.Spec.MessageSummary,
    Status:          "failed",
    SentAt:          notificationRequest.Status.SentAt.Time,  // Attempt time
    DeliveryStatus:  notificationRequest.Status.DeliveryStatus,
    ErrorMessage:    notificationRequest.Status.ErrorMessage,
    EscalationLevel: notificationRequest.Spec.EscalationLevel,
}
```

**Business Value**: Enables debugging, retry logic, and failure analysis

---

### **Trigger 3: Notification Acknowledged**
**CRD Status Transition**: `sent` → `acknowledged`

**Trigger Condition**:
```go
if notificationRequest.Status.Status == NotificationStatusAcknowledged && 
   oldStatus != NotificationStatusAcknowledged {
    // Write audit trace
}
```

**Audit Data Payload**:
```go
NotificationAudit{
    RemediationID:   notificationRequest.Spec.RemediationID,
    NotificationID:  notificationRequest.Name,
    Recipient:       notificationRequest.Spec.Recipient,
    Channel:         string(notificationRequest.Spec.Channel),
    MessageSummary:  notificationRequest.Spec.MessageSummary,
    Status:          "acknowledged",
    SentAt:          notificationRequest.Status.SentAt.Time,
    DeliveryStatus:  notificationRequest.Status.DeliveryStatus,
    ErrorMessage:    "",
    EscalationLevel: notificationRequest.Spec.EscalationLevel,
}
```

**Business Value**: Tracks SLA compliance (time to acknowledgment)

---

### **Trigger 4: Notification Escalated**
**CRD Status Transition**: `sent` → `escalated` OR `failed` → `escalated`

**Trigger Condition**:
```go
if notificationRequest.Status.Status == NotificationStatusEscalated && 
   oldStatus != NotificationStatusEscalated {
    // Write audit trace
}
```

**Audit Data Payload**:
```go
NotificationAudit{
    RemediationID:   notificationRequest.Spec.RemediationID,
    NotificationID:  notificationRequest.Name,
    Recipient:       notificationRequest.Spec.Recipient,
    Channel:         string(notificationRequest.Spec.Channel),
    MessageSummary:  notificationRequest.Spec.MessageSummary,
    Status:          "escalated",
    SentAt:          notificationRequest.Status.SentAt.Time,
    DeliveryStatus:  notificationRequest.Status.DeliveryStatus,
    ErrorMessage:    notificationRequest.Status.ErrorMessage,
    EscalationLevel: notificationRequest.Spec.EscalationLevel,  // Incremented
}
```

**Business Value**: Tracks escalation events for SLA and on-call rotation analysis

---

## 📍 **Audit Trace Integration Points - WHERE to Write**

### **Primary Integration Point: Controller Reconcile Loop**

**File**: `internal/controller/notification/notificationrequest_controller.go`

**Integration Pattern**:
```go
func (r *NotificationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... (fetch NotificationRequest CRD) ...
    
    // Store old status for comparison
    oldStatus := notificationRequest.Status.Status
    
    // ... (process notification logic) ...
    
    // Update CRD status
    if err := r.Status().Update(ctx, notificationRequest); err != nil {
        return ctrl.Result{}, err
    }
    
    // ========================================
    // AUDIT TRACE WRITE (NON-BLOCKING)
    // ========================================
    // Write audit trace AFTER CRD status update succeeds
    if notificationRequest.Status.Status != oldStatus {
        auditData := r.buildAuditData(notificationRequest, oldStatus)
        
        // Non-blocking audit write with DLQ fallback (DD-009)
        go func() {
            if err := r.auditClient.WriteNotificationAudit(ctx, auditData); err != nil {
                r.Log.Error(err, "Failed to write notification audit (DLQ fallback triggered)",
                    "notificationID", notificationRequest.Name,
                    "status", notificationRequest.Status.Status)
                // DO NOT fail reconciliation - audit is best-effort
            }
        }()
    }
    
    return ctrl.Result{}, nil
}
```

**Key Design Decisions**:
1. **After CRD Status Update**: Audit writes happen AFTER CRD status is successfully updated (ensures CRD is source of truth)
2. **Non-Blocking Goroutine**: Audit write in separate goroutine to avoid blocking notification delivery
3. **DLQ Fallback**: Audit client automatically falls back to DLQ on failure (DD-009)
4. **No Reconciliation Failure**: Audit write failures do NOT cause reconciliation to fail

---

### **Audit Client Initialization**

**File**: `internal/controller/notification/notificationrequest_controller.go`

```go
type NotificationRequestReconciler struct {
    client.Client
    Scheme      *runtime.Scheme
    Log         logr.Logger
    auditClient *datastorage.AuditClient  // NEW: Data Storage audit client
}

func (r *NotificationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Initialize audit client
    auditServiceURL := os.Getenv("DATA_STORAGE_SERVICE_URL")
    if auditServiceURL == "" {
        auditServiceURL = "http://data-storage-service.kubernaut-system.svc.cluster.local:8080"
    }
    
    r.auditClient = datastorage.NewAuditClient(auditServiceURL, r.Log)
    
    return ctrl.NewControllerManagedBy(mgr).
        For(&notificationv1.NotificationRequest{}).
        Named("notificationrequest-notification").
        Complete(r)
}
```

---

## 🔧 **Audit Data Transformation - WHAT to Write**

### **Field Mapping: CRD → Audit Table**

| CRD Field | Audit Table Column | Transformation | Notes |
|-----------|-------------------|----------------|-------|
| `notificationRequest.Spec.RemediationID` | `remediation_id` | Direct copy | Links to parent RemediationRequest |
| `notificationRequest.Name` | `notification_id` | Direct copy | Unique CRD name |
| `notificationRequest.Spec.Recipient` | `recipient` | Direct copy | Email, Slack user, etc. |
| `notificationRequest.Spec.Channel` | `channel` | `string(Channel)` | Enum to string |
| `notificationRequest.Spec.MessageSummary` | `message_summary` | Direct copy | Short summary |
| `notificationRequest.Status.Status` | `status` | `string(Status)` | Enum to string |
| `notificationRequest.Status.SentAt` | `sent_at` | `.Time` | metav1.Time to time.Time |
| `notificationRequest.Status.DeliveryStatus` | `delivery_status` | Direct copy | Provider response |
| `notificationRequest.Status.ErrorMessage` | `error_message` | Direct copy | Failure details |
| `notificationRequest.Spec.EscalationLevel` | `escalation_level` | Direct copy | Integer |
| `time.Now()` | `created_at` | Auto-generated | Audit record creation time |

### **Helper Function: Build Audit Data**

```go
func (r *NotificationRequestReconciler) buildAuditData(
    nr *notificationv1.NotificationRequest,
    oldStatus notificationv1.NotificationStatus,
) *datastorage.NotificationAudit {
    return &datastorage.NotificationAudit{
        RemediationID:   nr.Spec.RemediationID,
        NotificationID:  nr.Name,
        Recipient:       nr.Spec.Recipient,
        Channel:         string(nr.Spec.Channel),
        MessageSummary:  nr.Spec.MessageSummary,
        Status:          string(nr.Status.Status),
        SentAt:          nr.Status.SentAt.Time,
        DeliveryStatus:  nr.Status.DeliveryStatus,
        ErrorMessage:    nr.Status.ErrorMessage,
        EscalationLevel: nr.Spec.EscalationLevel,
        CreatedAt:       time.Now(),
    }
}
```

---

## 🚨 **Error Handling & Resilience**

### **Error Scenarios**

| Error Scenario | Handling Strategy | Impact |
|---------------|------------------|--------|
| **Data Storage Service Unavailable** | DLQ fallback (Redis Streams) | Audit data queued for retry |
| **Network Timeout** | DLQ fallback | Audit data queued for retry |
| **HTTP 5xx Error** | DLQ fallback | Audit data queued for retry |
| **HTTP 4xx Error (Bad Request)** | Log error, skip DLQ | Indicates code bug, needs investigation |
| **Audit Client Panic** | Recover in goroutine | Notification delivery continues |

### **DLQ Fallback Pattern (DD-009)**

```go
// pkg/datastorage/audit/client.go
func (c *AuditClient) WriteNotificationAudit(ctx context.Context, audit *NotificationAudit) error {
    url := fmt.Sprintf("%s/api/v1/audit/notifications", c.serviceURL)
    
    body, err := json.Marshal(audit)
    if err != nil {
        return fmt.Errorf("failed to marshal audit data: %w", err)
    }
    
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
    if err != nil {
        return fmt.Errorf("failed to create HTTP request: %w", err)
    }
    
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        // Network error - fallback to DLQ
        return c.fallbackToDLQ(ctx, "notification", audit, err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode >= 500 {
        // Server error - fallback to DLQ
        return c.fallbackToDLQ(ctx, "notification", audit, fmt.Errorf("HTTP %d", resp.StatusCode))
    }
    
    if resp.StatusCode >= 400 {
        // Client error - log and return (do NOT use DLQ for code bugs)
        return fmt.Errorf("HTTP %d: bad request (code bug)", resp.StatusCode)
    }
    
    return nil
}
```

---

## 📊 **Metrics & Observability**

### **Prometheus Metrics**

```go
// Audit write attempts
kubernaut_notification_audit_writes_total{status="success|failure|dlq_fallback"}

// Audit write duration
kubernaut_notification_audit_write_duration_seconds{status="success|failure"}

// DLQ fallback rate
kubernaut_notification_audit_dlq_fallback_total{reason="network_error|server_error"}
```

### **Kubernetes Events**

```go
// Record event on audit write failure (not DLQ fallback)
r.Recorder.Event(notificationRequest, corev1.EventTypeWarning, 
    "AuditWriteFailed", 
    fmt.Sprintf("Failed to write audit trace: %v", err))
```

---

## ✅ **Validation & Testing**

### **Unit Tests**

```go
// Test audit data transformation
func TestBuildAuditData(t *testing.T) {
    // Test all CRD status transitions → audit data
}

// Test audit write trigger conditions
func TestAuditWriteTriggers(t *testing.T) {
    // Test status transition detection
}
```

### **Integration Tests**

```go
// Test Notification Controller → Data Storage → PostgreSQL flow
func TestNotificationAuditIntegration(t *testing.T) {
    // 1. Create NotificationRequest CRD
    // 2. Wait for status update
    // 3. Verify audit record in PostgreSQL
}

// Test DLQ fallback
func TestNotificationAuditDLQFallback(t *testing.T) {
    // 1. Stop Data Storage Service
    // 2. Create NotificationRequest CRD
    // 3. Verify audit data in Redis DLQ
}
```

### **E2E Tests**

```go
// Test complete notification + audit flow
func TestNotificationWithAuditE2E(t *testing.T) {
    // 1. Create RemediationRequest
    // 2. Wait for NotificationRequest creation
    // 3. Verify notification sent
    // 4. Verify audit record in PostgreSQL
    // 5. Verify audit data matches CRD status
}
```

---

## 📚 **Related Documentation**

- [ADR-032 v1.3: Data Access Layer Isolation](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md)
- [DD-009: Audit Write Error Recovery (DLQ)](../../../architecture/decisions/DD-009-audit-write-error-recovery.md)
- [Notification Controller Database Integration](./database-integration.md)
- [Data Storage Implementation Plan V4.8](../../stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.8.md)
- [Notification Controller Refactoring Plan V0.1](./implementation/REFACTORING_PLAN_V0.1.md) ← **TO BE CREATED**

---

## 🎯 **Success Criteria**

| Criterion | Target | Validation Method |
|-----------|--------|-------------------|
| **Audit Write Success Rate** | >99% | Prometheus metrics |
| **DLQ Fallback Rate** | <1% | Prometheus metrics |
| **Audit Data Accuracy** | 100% | Integration tests |
| **Non-Blocking Guarantee** | 100% | Notification delivery not impacted by audit failures |
| **Audit-CRD Consistency** | 100% | E2E tests verify audit matches CRD status |

---

## 🚀 **Next Steps**

1. ✅ **This Document**: Audit trace specification complete
2. ⏸️ **Refactoring Plan**: Create `REFACTORING_PLAN_V0.1.md` for TDD implementation
3. ⏸️ **Data Storage API**: Implement `POST /api/v1/audit/notifications` endpoint
4. ⏸️ **Notification Controller**: Refactor to add audit writes
5. ⏸️ **Integration Tests**: Validate Notification → Data Storage → PostgreSQL flow
6. ⏸️ **E2E Tests**: Validate complete remediation + notification + audit flow
7. ⏸️ **Pilot Validation**: Use Notification Controller as template for remaining 5 controllers

---

**Confidence**: 100% (Notification Controller is fully implemented, CRD spec finalized)
**Status**: ✅ Ready for implementation

