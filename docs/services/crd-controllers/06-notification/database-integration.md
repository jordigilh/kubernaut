# Notification Controller - Database Integration (Audit Trail Persistence)

**Version**: 1.0
**Date**: 2025-11-02
**Service Type**: CRD Controller (Kubernetes Operator)
**Status**: ✅ Schema Documented (Phase 0 Day 0.1 - GAP #1 Resolution)
**Authority**: ADR-032 v1.1 - Data Access Layer Isolation

---

## 📋 **Overview**

This document defines the Notification Controller's audit trail persistence strategy, aligning with ADR-032's mandate that **all CRD controllers write audit data via Data Storage Service REST API**, never directly to PostgreSQL.

**Business Requirements**:
- BR-NOT-001 to BR-NOT-037: Multi-channel notifications with delivery tracking
- BR-AUDIT-001: Complete audit trail for compliance (7+ years retention)
- BR-RAR-001 to BR-RAR-004: V2.0 Remediation Analysis Report generation

**Purpose**: Persist notification delivery status, retries, and channel usage for:
1. **Real-time monitoring**: Track notification delivery success/failure rates
2. **Compliance**: 7+ year audit trail for regulatory requirements
3. **V2.0 RAR**: Include notification timeline in Remediation Analysis Reports
4. **Operations**: Debug stuck notifications and channel performance

---

## 🗄️ **Audit Data Schema**

### **Go Struct Definition**

```go
// pkg/notification/audit/types.go
package audit

import "time"

// NotificationAudit represents the complete audit trail for a notification delivery
// Maps to notification_audit table via Data Storage Service
// Schema Authority: migrations/010_audit_write_api.sql
type NotificationAudit struct {
    // Identity
    ID             string    `json:"id"`              // Unique notification ID (UUID)
    RemediationID  string    `json:"remediation_id"`  // Links to remediation request

    // Notification details
    Channel        string    `json:"channel"`         // slack, pagerduty, email, webhook, teams
    RecipientCount int       `json:"recipient_count"` // Number of recipients
    Recipients     []string  `json:"recipients"`      // Array of recipient identifiers (emails, slack IDs, etc.)

    // Message content
    MessageTemplate string    `json:"message_template,omitempty"` // Template name used
    MessagePriority string    `json:"message_priority,omitempty"` // P0, P1, P2, P3
    NotificationType string   `json:"notification_type"`          // alert, escalation, resolution, status_update

    // Delivery tracking
    Status             string    `json:"status"`                        // pending, sent, delivered, failed, retrying
    DeliveryTime       *time.Time `json:"delivery_time,omitempty"`      // When notification was delivered
    DeliveryDurationMs int       `json:"delivery_duration_ms,omitempty"` // Total delivery time in milliseconds

    // Retry tracking
    RetryCount     int        `json:"retry_count"`               // Current retry attempt (0-based)
    MaxRetries     int        `json:"max_retries"`               // Maximum retry attempts (default: 3)
    LastRetryTime  *time.Time `json:"last_retry_time,omitempty"` // Timestamp of last retry

    // Failure tracking
    ErrorMessage string `json:"error_message,omitempty"` // Detailed error message
    ErrorCode    string `json:"error_code,omitempty"`    // Error classification code

    // Metadata
    CompletedAt time.Time `json:"completed_at,omitempty"` // When notification lifecycle completed (success or final failure)
    CreatedAt   time.Time `json:"created_at"`             // Record creation timestamp
    UpdatedAt   time.Time `json:"updated_at"`             // Record last update timestamp
}

// NotificationChannel represents valid notification channels
type NotificationChannel string

const (
    ChannelSlack      NotificationChannel = "slack"
    ChannelPagerDuty  NotificationChannel = "pagerduty"
    ChannelEmail      NotificationChannel = "email"
    ChannelWebhook    NotificationChannel = "webhook"
    ChannelTeams      NotificationChannel = "teams"
)

// NotificationStatus represents delivery status
type NotificationStatus string

const (
    StatusPending   NotificationStatus = "pending"   // Notification queued
    StatusSent      NotificationStatus = "sent"      // Sent to channel API
    StatusDelivered NotificationStatus = "delivered" // Confirmed delivered by channel
    StatusFailed    NotificationStatus = "failed"    // Permanent failure
    StatusRetrying  NotificationStatus = "retrying"  // In retry loop
)

// NotificationType represents notification purpose
type NotificationType string

const (
    TypeAlert        NotificationType = "alert"         // Initial alert notification
    TypeEscalation   NotificationType = "escalation"    // Escalation to on-call
    TypeResolution   NotificationType = "resolution"    // Remediation completed
    TypeStatusUpdate NotificationType = "status_update" // Progress update
)
```

---

## 🏗️ **PostgreSQL Table Schema**

**Table**: `notification_audit`
**Schema Authority**: `migrations/010_audit_write_api.sql`
**Managed By**: Data Storage Service (NEVER accessed directly by Notification Controller)

```sql
CREATE TABLE IF NOT EXISTS notification_audit (
    id BIGSERIAL PRIMARY KEY,

    -- Identity
    notification_id VARCHAR(255) NOT NULL UNIQUE,
    remediation_id VARCHAR(255) NOT NULL,

    -- Notification details
    channel VARCHAR(50) NOT NULL CHECK (channel IN ('slack', 'pagerduty', 'email', 'webhook', 'teams')),
    recipient_count INTEGER NOT NULL DEFAULT 0,
    recipients JSONB,  -- Array of recipient identifiers

    -- Message content
    message_template VARCHAR(255),
    message_priority VARCHAR(10) CHECK (message_priority IN ('P0', 'P1', 'P2', 'P3')),
    notification_type VARCHAR(50) CHECK (notification_type IN ('alert', 'escalation', 'resolution', 'status_update')),

    -- Delivery tracking
    status VARCHAR(50) NOT NULL CHECK (status IN ('pending', 'sent', 'delivered', 'failed', 'retrying')),
    delivery_time TIMESTAMP WITH TIME ZONE,
    delivery_duration_ms INTEGER,

    -- Retry tracking
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    last_retry_time TIMESTAMP WITH TIME ZONE,

    -- Failure tracking
    error_message TEXT,
    error_code VARCHAR(50),

    -- Metadata
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_notification_audit_notification_id ON notification_audit(notification_id);
CREATE INDEX IF NOT EXISTS idx_notification_audit_remediation_id ON notification_audit(remediation_id);
CREATE INDEX IF NOT EXISTS idx_notification_audit_channel ON notification_audit(channel);
CREATE INDEX IF NOT EXISTS idx_notification_audit_status ON notification_audit(status);
CREATE INDEX IF NOT EXISTS idx_notification_audit_created_at ON notification_audit(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_audit_delivery_time ON notification_audit(delivery_time DESC) WHERE delivery_time IS NOT NULL;
```

---

## 🔄 **Audit Write Trigger Points**

### **When to Write Audit Data**

The Notification Controller writes audit data to Data Storage Service at these lifecycle points:

#### **1. Initial Notification Creation** (Status: `pending`)

**Trigger**: NotificationRequest CRD created
**Timing**: Immediately after validation, before delivery attempt
**Status**: `pending`

```go
// internal/controller/notification/reconciler.go
func (r *NotificationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    notification := &v1alpha1.NotificationRequest{}
    if err := r.Get(ctx, req.NamespacedName, notification); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Create initial audit record
    auditData := &audit.NotificationAudit{
        ID:               string(notification.UID),
        RemediationID:    notification.Spec.RemediationRequestRef,
        Channel:          notification.Spec.Channel,
        RecipientCount:   len(notification.Spec.Recipients),
        Recipients:       notification.Spec.Recipients,
        MessageTemplate:  notification.Spec.Template,
        MessagePriority:  notification.Spec.Priority,
        NotificationType: notification.Spec.NotificationType,
        Status:           string(audit.StatusPending),
        RetryCount:       0,
        MaxRetries:       3,
        CreatedAt:        notification.CreationTimestamp.Time,
    }

    // Write to Data Storage Service (non-blocking, best-effort)
    if err := r.auditClient.WriteNotificationAudit(ctx, auditData); err != nil {
        r.Log.Error(err, "Failed to write initial audit", "notificationID", notification.UID)
        // DO NOT FAIL RECONCILIATION - audit is best-effort
    }

    // Continue with notification delivery...
}
```

#### **2. Delivery Attempt** (Status: `sent` or `failed`)

**Trigger**: After calling notification channel API
**Timing**: Immediately after HTTP response received
**Status**: `sent` (if successful), `failed` (if error), `retrying` (if retryable error)

```go
func (r *NotificationReconciler) deliverNotification(ctx context.Context, notification *v1alpha1.NotificationRequest) error {
    startTime := time.Now()

    // Attempt delivery via channel API
    err := r.channelClient.Send(ctx, &ChannelMessage{
        Channel:    notification.Spec.Channel,
        Recipients: notification.Spec.Recipients,
        Message:    notification.Spec.Message,
    })

    deliveryDuration := time.Since(startTime).Milliseconds()

    // Update audit with delivery result
    auditData := &audit.NotificationAudit{
        ID:                 string(notification.UID),
        Status:             determineStatus(err),
        DeliveryTime:       &startTime,
        DeliveryDurationMs: int(deliveryDuration),
        ErrorMessage:       getErrorMessage(err),
        ErrorCode:          getErrorCode(err),
        UpdatedAt:          time.Now(),
    }

    // Update audit (non-blocking)
    if updateErr := r.auditClient.UpdateNotificationAudit(ctx, auditData); updateErr != nil {
        r.Log.Error(updateErr, "Failed to update delivery audit", "notificationID", notification.UID)
    }

    return err
}

func determineStatus(err error) string {
    if err == nil {
        return string(audit.StatusSent)
    }
    if isRetryable(err) {
        return string(audit.StatusRetrying)
    }
    return string(audit.StatusFailed)
}
```

#### **3. Delivery Confirmation** (Status: `delivered`)

**Trigger**: Webhook callback from channel confirms delivery
**Timing**: Asynchronously when channel confirms
**Status**: `delivered`

```go
func (r *NotificationReconciler) HandleDeliveryConfirmation(ctx context.Context, confirmationWebhook *DeliveryConfirmation) error {
    auditData := &audit.NotificationAudit{
        ID:          confirmationWebhook.NotificationID,
        Status:      string(audit.StatusDelivered),
        CompletedAt: time.Now(),
        UpdatedAt:   time.Now(),
    }

    return r.auditClient.UpdateNotificationAudit(ctx, auditData)
}
```

#### **4. Retry Attempt** (Status: `retrying`)

**Trigger**: Exponential backoff retry triggered
**Timing**: Before each retry attempt
**Status**: `retrying`

```go
func (r *NotificationReconciler) retryDelivery(ctx context.Context, notification *v1alpha1.NotificationRequest, attempt int) error {
    auditData := &audit.NotificationAudit{
        ID:            string(notification.UID),
        Status:        string(audit.StatusRetrying),
        RetryCount:    attempt,
        LastRetryTime: func() *time.Time { t := time.Now(); return &t }(),
        UpdatedAt:     time.Now(),
    }

    // Update audit before retry
    if err := r.auditClient.UpdateNotificationAudit(ctx, auditData); err != nil {
        r.Log.Error(err, "Failed to update retry audit", "notificationID", notification.UID)
    }

    // Attempt delivery
    return r.deliverNotification(ctx, notification)
}
```

#### **5. Final Failure** (Status: `failed`)

**Trigger**: Max retries exceeded
**Timing**: After final retry attempt fails
**Status**: `failed`

```go
func (r *NotificationReconciler) markFinalFailure(ctx context.Context, notification *v1alpha1.NotificationRequest, finalError error) error {
    auditData := &audit.NotificationAudit{
        ID:           string(notification.UID),
        Status:       string(audit.StatusFailed),
        ErrorMessage: finalError.Error(),
        ErrorCode:    getErrorCode(finalError),
        CompletedAt:  time.Now(),
        UpdatedAt:    time.Now(),
    }

    return r.auditClient.WriteNotificationAudit(ctx, auditData)
}
```

---

## 🌐 **HTTP Client Integration** (Data Storage Service)

### **Endpoint**: `POST /api/v1/audit/notifications`

**Authority**: ADR-032 v1.1 - Audit endpoint for Notification Controller audit writes

```go
// pkg/notification/audit/client.go
package audit

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

// Client writes notification audit data to Data Storage Service
type Client struct {
    storageServiceURL string
    httpClient        *http.Client
    dlqClient         *DLQClient // Dead Letter Queue fallback (DD-009)
}

// NewClient creates a new audit client
func NewClient(storageServiceURL string, dlqClient *DLQClient) *Client {
    return &Client{
        storageServiceURL: storageServiceURL,
        httpClient: &http.Client{
            Timeout: 5 * time.Second,
        },
        dlqClient: dlqClient,
    }
}

// WriteNotificationAudit writes notification audit data to Data Storage Service
// Implements ADR-032 v1.1 audit writing pattern
// Includes DD-009 DLQ fallback for fault tolerance
func (c *Client) WriteNotificationAudit(ctx context.Context, audit *NotificationAudit) error {
    url := fmt.Sprintf("%s/api/v1/audit/notifications", c.storageServiceURL)

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
        // Network error - fallback to DLQ (DD-009)
        return c.fallbackToDLQ(ctx, audit, err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
        // HTTP error - fallback to DLQ
        return c.fallbackToDLQ(ctx, audit, fmt.Errorf("HTTP %d", resp.StatusCode))
    }

    return nil
}

// UpdateNotificationAudit updates existing audit record (same endpoint, idempotent)
func (c *Client) UpdateNotificationAudit(ctx context.Context, audit *NotificationAudit) error {
    return c.WriteNotificationAudit(ctx, audit) // Data Storage Service handles upsert
}

// fallbackToDLQ writes audit data to Dead Letter Queue when Data Storage unavailable
// Implements DD-009 error recovery pattern
func (c *Client) fallbackToDLQ(ctx context.Context, audit *NotificationAudit, originalErr error) error {
    if c.dlqClient == nil {
        return fmt.Errorf("DLQ not configured, audit lost: %w", originalErr)
    }

    if err := c.dlqClient.WriteNotificationAudit(ctx, audit); err != nil {
        return fmt.Errorf("Data Storage write failed AND DLQ write failed: %w (original: %v)", err, originalErr)
    }

    // Audit written to DLQ successfully - async retry worker will process
    return nil
}
```

---

## 📊 **Example Audit Lifecycle**

**Scenario**: Slack escalation notification with 1 retry before success

```json
// Step 1: Initial creation (pending)
POST /api/v1/audit/notifications
{
  "id": "notification-abc123",
  "remediation_id": "remediation-xyz789",
  "channel": "slack",
  "recipient_count": 3,
  "recipients": ["#sre-oncall", "@john.doe", "@jane.smith"],
  "message_template": "escalation-timeout",
  "message_priority": "P0",
  "notification_type": "escalation",
  "status": "pending",
  "retry_count": 0,
  "max_retries": 3,
  "created_at": "2025-11-02T10:00:00Z"
}

// Step 2: First delivery attempt fails (retrying)
POST /api/v1/audit/notifications
{
  "id": "notification-abc123",
  "status": "retrying",
  "retry_count": 1,
  "last_retry_time": "2025-11-02T10:00:05Z",
  "error_message": "Slack API rate limit exceeded",
  "error_code": "SLACK_RATE_LIMIT",
  "updated_at": "2025-11-02T10:00:05Z"
}

// Step 3: Retry succeeds (sent)
POST /api/v1/audit/notifications
{
  "id": "notification-abc123",
  "status": "sent",
  "delivery_time": "2025-11-02T10:00:35Z",
  "delivery_duration_ms": 450,
  "retry_count": 1,
  "updated_at": "2025-11-02T10:00:35Z"
}

// Step 4: Slack confirms delivery (delivered)
POST /api/v1/audit/notifications
{
  "id": "notification-abc123",
  "status": "delivered",
  "completed_at": "2025-11-02T10:00:37Z",
  "updated_at": "2025-11-02T10:00:37Z"
}
```

**Final Database State**:
- 1 audit record with 4 updates
- Total lifecycle: 37 seconds (pending → retrying → sent → delivered)
- Retry count: 1
- Delivery duration: 450ms
- Status: `delivered` ✅

---

## 🔍 **Audit Query Use Cases**

### **1. Real-Time Notification Monitoring**

**Query**: Find all pending/failed notifications for a remediation

```sql
SELECT
    notification_id,
    channel,
    status,
    retry_count,
    error_message,
    created_at
FROM notification_audit
WHERE remediation_id = 'remediation-xyz789'
AND status IN ('pending', 'failed', 'retrying')
ORDER BY created_at DESC;
```

### **2. Channel Performance Analysis**

**Query**: Calculate delivery success rate by channel

```sql
SELECT
    channel,
    COUNT(*) FILTER (WHERE status = 'delivered') AS delivered_count,
    COUNT(*) AS total_count,
    ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'delivered') / COUNT(*), 2) AS success_rate_pct,
    AVG(delivery_duration_ms) FILTER (WHERE status = 'delivered') AS avg_delivery_ms
FROM notification_audit
WHERE created_at > NOW() - INTERVAL '24 hours'
GROUP BY channel
ORDER BY success_rate_pct DESC;
```

### **3. V2.0 RAR - Notification Timeline**

**Query**: Get complete notification timeline for a remediation (for RAR generation)

```sql
SELECT
    notification_id,
    notification_type,
    channel,
    recipient_count,
    status,
    created_at AS notification_time,
    delivery_time,
    completed_at,
    retry_count,
    EXTRACT(EPOCH FROM (completed_at - created_at)) AS total_duration_seconds
FROM notification_audit
WHERE remediation_id = 'remediation-xyz789'
ORDER BY created_at ASC;
```

### **4. Failed Notification Investigation**

**Query**: Find repeatedly failing notifications (for runbook debugging)

```sql
SELECT
    notification_id,
    remediation_id,
    channel,
    retry_count,
    max_retries,
    error_code,
    error_message,
    created_at,
    last_retry_time
FROM notification_audit
WHERE status = 'failed'
AND retry_count >= max_retries
AND created_at > NOW() - INTERVAL '1 hour'
ORDER BY created_at DESC;
```

---

## 🎯 **Integration Checklist** (Phase 0 Day 0.3 Preparation)

- [x] Audit schema defined (NotificationAudit Go struct)
- [x] PostgreSQL table schema documented (notification_audit)
- [x] HTTP client implementation pattern defined
- [x] Audit trigger points identified (5 lifecycle events)
- [x] Error handling includes DLQ fallback (DD-009 compliance)
- [x] Example audit lifecycle documented
- [x] Common query use cases provided
- [ ] Implementation: Create `pkg/notification/audit/` package (Phase 1-3)
- [ ] Implementation: Integrate audit client in NotificationReconciler (Phase 1-3)
- [ ] Implementation: Add metrics for audit write success/failure (Phase 1-3)
- [ ] Testing: Unit tests for audit client (Phase 1-3)
- [ ] Testing: Integration tests with Data Storage Service (Phase 1-3)

---

## 📚 **Related Documentation**

- **Schema Authority**: `migrations/010_audit_write_api.sql`
- **Audit Architecture**: `docs/architecture/decisions/ADR-032-data-access-layer-isolation.md`
- **Error Recovery**: `docs/architecture/decisions/DD-009-audit-write-error-recovery.md` (to be created)
- **Service Integration**: `docs/services/crd-controllers/06-notification/integration-points.md`
- **Implementation Plan**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.7.md`

---

## ✅ **Phase 0 Day 0.1 - Task 2 Complete**

**Deliverable**: ✅ Notification Controller audit schema fully documented
**Validation**: Schema aligns with `notification_audit` table in `migrations/010_audit_write_api.sql`
**Confidence**: 100%

---

**Document Version**: 1.0
**Last Updated**: 2025-11-02
**Status**: ✅ GAP #1 RESOLVED

