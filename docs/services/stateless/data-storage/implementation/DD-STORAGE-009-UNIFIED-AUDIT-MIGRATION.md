# DD-STORAGE-009: Unified Audit Table Migration Plan

**Date**: November 13, 2025
**Status**: ✅ **APPROVED** (Migration Plan)
**Decision Maker**: Kubernaut Data Storage Team
**Authority**: ADR-034 (Unified Audit Table Design)
**Affects**: Data Storage Service V1.0 MVP, all services writing audit events
**Blocks**: Data Storage V1.0 MVP implementation

---

## 🎯 **Context**

**Problem**: Data Storage Service currently has `notification_audit` table (migration 010) which conflicts with ADR-034's unified `audit_events` table design.

**User Decision**: "I want to defer implementation until we have all the decisions and plans defined"

**This Document**: Comprehensive migration plan from `notification_audit` to `audit_events` (ADR-034 compliant).

**Authoritative Source**: ADR-034 (Unified Audit Table Design with Event Sourcing Pattern)

---

## ✅ **Decision**

**APPROVED**: Migrate from `notification_audit` table to ADR-034 unified `audit_events` table in Data Storage Service V1.0 MVP.

**Migration Strategy**: **Drop and Replace** (no backwards compatibility needed per project guidelines)

**Confidence**: 95%

---

## 📊 **Current State vs Target State**

### **Current State** (notification_audit)

**Table**: `notification_audit` (migration 010)
```sql
CREATE TABLE notification_audit (
    id BIGSERIAL PRIMARY KEY,
    remediation_id VARCHAR(255) NOT NULL,
    notification_id VARCHAR(255) NOT NULL UNIQUE,
    recipient VARCHAR(255) NOT NULL,
    channel VARCHAR(50) NOT NULL,
    message_summary TEXT NOT NULL,
    status VARCHAR(50) NOT NULL,
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
    delivered_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
```

**Limitations**:
- ❌ Notification-specific (cannot store other audit types)
- ❌ BIGSERIAL ID (not UUID)
- ❌ No JSONB event_data (inflexible schema)
- ❌ No event sourcing pattern
- ❌ No partitioning
- ❌ No correlation tracking
- ❌ Violates ADR-034 extensibility requirement

---

### **Target State** (audit_events)

**Table**: `audit_events` (ADR-034)
```sql
CREATE TABLE audit_events (
    -- Event Identity
    event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_version VARCHAR(10) NOT NULL DEFAULT '1.0',

    -- Temporal Information
    event_timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    event_date DATE NOT NULL GENERATED ALWAYS AS (event_timestamp::DATE) STORED,

    -- Event Classification
    event_type VARCHAR(100) NOT NULL,        -- 'notification.sent', 'notification.delivered'
    event_category VARCHAR(50) NOT NULL,     -- 'notification', 'remediation', 'workflow'
    event_action VARCHAR(50) NOT NULL,       -- 'sent', 'delivered', 'failed'
    event_outcome VARCHAR(20) NOT NULL,      -- 'success', 'failure', 'pending'

    -- Actor Information (Who)
    actor_type VARCHAR(50) NOT NULL,         -- 'service', 'external', 'user'
    actor_id VARCHAR(255) NOT NULL,          -- 'notification-service', 'slack-api'
    actor_ip INET,

    -- Resource Information (What)
    resource_type VARCHAR(100) NOT NULL,     -- 'Notification', 'RemediationRequest'
    resource_id VARCHAR(255) NOT NULL,       -- 'notif-abc123', 'rr-2025-001'
    resource_name VARCHAR(255),

    -- Context Information (Where/Why)
    correlation_id VARCHAR(255) NOT NULL,    -- remediation_id (groups related events)
    parent_event_id UUID,                    -- Links to parent event
    trace_id VARCHAR(255),                   -- OpenTelemetry trace ID
    span_id VARCHAR(255),                    -- OpenTelemetry span ID

    -- Kubernetes Context
    namespace VARCHAR(253),
    cluster_name VARCHAR(255),

    -- Event Payload (JSONB - flexible, queryable)
    event_data JSONB NOT NULL,
    event_metadata JSONB,

    -- Audit Metadata
    severity VARCHAR(20),
    duration_ms INTEGER,
    error_code VARCHAR(50),
    error_message TEXT,

    -- Compliance
    retention_days INTEGER DEFAULT 2555,     -- 7 years (SOC 2 / ISO 27001)
    is_sensitive BOOLEAN DEFAULT FALSE,

    -- Indexes
    INDEX idx_event_timestamp (event_timestamp DESC),
    INDEX idx_correlation_id (correlation_id, event_timestamp DESC),
    INDEX idx_resource (resource_type, resource_id, event_timestamp DESC),
    INDEX idx_event_type (event_type, event_timestamp DESC),
    INDEX idx_actor (actor_type, actor_id, event_timestamp DESC),
    INDEX idx_outcome (event_outcome, event_timestamp DESC),
    INDEX idx_event_data_gin (event_data) USING GIN,
    INDEX idx_parent_event (parent_event_id) WHERE parent_event_id IS NOT NULL
) PARTITION BY RANGE (event_date);
```

**Benefits**:
- ✅ Unified table for ALL audit types (notification, remediation, AI, workflow)
- ✅ UUID event_id (industry standard)
- ✅ JSONB event_data (flexible, queryable)
- ✅ Event sourcing pattern (complete audit trail)
- ✅ Partitioned by date (performance at scale)
- ✅ Correlation tracking (trace signal flow)
- ✅ ADR-034 compliant (extensibility for new services)

---

## 🔄 **Migration Strategy**

### **Strategy: Drop and Replace**

**Rationale**:
1. **No Backwards Compatibility Needed**: Per project guidelines (pre-release product)
2. **No Production Data**: Development environment only
3. **Simpler**: Avoid complex data migration logic
4. **Cleaner**: Start with correct schema from the beginning

**Confidence**: 98% (project guidelines explicitly allow this)

---

### **Migration Steps**

#### **Step 1: Create Unified Audit Table** (Migration 014)

**File**: `migrations/014_unified_audit_table.sql`

```sql
-- Drop old notification_audit table
DROP TABLE IF EXISTS notification_audit CASCADE;

-- Create unified audit_events table (ADR-034)
CREATE TABLE audit_events (
    -- Event Identity
    event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_version VARCHAR(10) NOT NULL DEFAULT '1.0',

    -- Temporal Information
    event_timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    event_date DATE NOT NULL GENERATED ALWAYS AS (event_timestamp::DATE) STORED,

    -- Event Classification
    event_type VARCHAR(100) NOT NULL,
    event_category VARCHAR(50) NOT NULL,
    event_action VARCHAR(50) NOT NULL,
    event_outcome VARCHAR(20) NOT NULL,

    -- Actor Information
    actor_type VARCHAR(50) NOT NULL,
    actor_id VARCHAR(255) NOT NULL,
    actor_ip INET,

    -- Resource Information
    resource_type VARCHAR(100) NOT NULL,
    resource_id VARCHAR(255) NOT NULL,
    resource_name VARCHAR(255),

    -- Context Information
    correlation_id VARCHAR(255) NOT NULL,
    parent_event_id UUID,
    trace_id VARCHAR(255),
    span_id VARCHAR(255),

    -- Kubernetes Context
    namespace VARCHAR(253),
    cluster_name VARCHAR(255),

    -- Event Payload
    event_data JSONB NOT NULL,
    event_metadata JSONB,

    -- Audit Metadata
    severity VARCHAR(20),
    duration_ms INTEGER,
    error_code VARCHAR(50),
    error_message TEXT,

    -- Compliance
    retention_days INTEGER DEFAULT 2555,
    is_sensitive BOOLEAN DEFAULT FALSE,

    -- Constraints
    CHECK (event_outcome IN ('success', 'failure', 'pending', 'partial')),
    CHECK (event_category IN ('signal', 'notification', 'remediation', 'workflow', 'ai_analysis', 'approval'))
) PARTITION BY RANGE (event_date);

-- Create indexes
CREATE INDEX idx_audit_events_timestamp ON audit_events (event_timestamp DESC);
CREATE INDEX idx_audit_events_correlation ON audit_events (correlation_id, event_timestamp DESC);
CREATE INDEX idx_audit_events_resource ON audit_events (resource_type, resource_id, event_timestamp DESC);
CREATE INDEX idx_audit_events_type ON audit_events (event_type, event_timestamp DESC);
CREATE INDEX idx_audit_events_actor ON audit_events (actor_type, actor_id, event_timestamp DESC);
CREATE INDEX idx_audit_events_outcome ON audit_events (event_outcome, event_timestamp DESC);
CREATE INDEX idx_audit_events_data_gin ON audit_events USING GIN (event_data);
CREATE INDEX idx_audit_events_parent ON audit_events (parent_event_id) WHERE parent_event_id IS NOT NULL;

-- Create initial partitions (current month + next 2 months)
CREATE TABLE audit_events_2025_11 PARTITION OF audit_events
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

CREATE TABLE audit_events_2025_12 PARTITION OF audit_events
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

CREATE TABLE audit_events_2026_01 PARTITION OF audit_events
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

-- Create function for automatic partition creation
CREATE OR REPLACE FUNCTION create_audit_events_partition()
RETURNS void AS $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    -- Create partition for next month if it doesn't exist
    partition_date := DATE_TRUNC('month', NOW() + INTERVAL '1 month');
    partition_name := 'audit_events_' || TO_CHAR(partition_date, 'YYYY_MM');
    start_date := partition_date;
    end_date := partition_date + INTERVAL '1 month';

    IF NOT EXISTS (
        SELECT 1 FROM pg_class WHERE relname = partition_name
    ) THEN
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF audit_events FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
        RAISE NOTICE 'Created partition: %', partition_name;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for updated_at (if needed by application)
CREATE OR REPLACE FUNCTION update_audit_events_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.event_metadata = jsonb_set(
        COALESCE(NEW.event_metadata, '{}'::jsonb),
        '{updated_at}',
        to_jsonb(NOW())
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_events_updated_at
    BEFORE UPDATE ON audit_events
    FOR EACH ROW
    EXECUTE FUNCTION update_audit_events_updated_at();
```

**Deliverable**: Unified audit_events table with partitioning

---

#### **Step 2: Create Audit Shared Library** (DD-AUDIT-002)

**File**: `pkg/audit/audit_event.go`

```go
// pkg/audit/audit_event.go
package audit

import (
    "time"
    "encoding/json"
    "github.com/google/uuid"
)

// AuditEvent represents a unified audit event (ADR-034)
type AuditEvent struct {
    // Event Identity
    EventID      uuid.UUID `json:"event_id" db:"event_id"`
    EventVersion string    `json:"event_version" db:"event_version"`

    // Temporal Information
    EventTimestamp time.Time `json:"event_timestamp" db:"event_timestamp"`

    // Event Classification
    EventType     string `json:"event_type" db:"event_type"`
    EventCategory string `json:"event_category" db:"event_category"`
    EventAction   string `json:"event_action" db:"event_action"`
    EventOutcome  string `json:"event_outcome" db:"event_outcome"`

    // Actor Information
    ActorType string  `json:"actor_type" db:"actor_type"`
    ActorID   string  `json:"actor_id" db:"actor_id"`
    ActorIP   *string `json:"actor_ip,omitempty" db:"actor_ip"`

    // Resource Information
    ResourceType string  `json:"resource_type" db:"resource_type"`
    ResourceID   string  `json:"resource_id" db:"resource_id"`
    ResourceName *string `json:"resource_name,omitempty" db:"resource_name"`

    // Context Information
    CorrelationID string     `json:"correlation_id" db:"correlation_id"`
    ParentEventID *uuid.UUID `json:"parent_event_id,omitempty" db:"parent_event_id"`
    TraceID       *string    `json:"trace_id,omitempty" db:"trace_id"`
    SpanID        *string    `json:"span_id,omitempty" db:"span_id"`

    // Kubernetes Context
    Namespace   *string `json:"namespace,omitempty" db:"namespace"`
    ClusterName *string `json:"cluster_name,omitempty" db:"cluster_name"`

    // Event Payload
    EventData     json.RawMessage `json:"event_data" db:"event_data"`
    EventMetadata json.RawMessage `json:"event_metadata,omitempty" db:"event_metadata"`

    // Audit Metadata
    Severity     *string `json:"severity,omitempty" db:"severity"`
    DurationMs   *int    `json:"duration_ms,omitempty" db:"duration_ms"`
    ErrorCode    *string `json:"error_code,omitempty" db:"error_code"`
    ErrorMessage *string `json:"error_message,omitempty" db:"error_message"`

    // Compliance
    RetentionDays int  `json:"retention_days" db:"retention_days"`
    IsSensitive   bool `json:"is_sensitive" db:"is_sensitive"`
}

// EventCategory constants
const (
    CategorySignal      = "signal"
    CategoryNotification = "notification"
    CategoryRemediation = "remediation"
    CategoryWorkflow    = "workflow"
    CategoryAIAnalysis  = "ai_analysis"
    CategoryApproval    = "approval"
)

// EventOutcome constants
const (
    OutcomeSuccess = "success"
    OutcomeFailure = "failure"
    OutcomePending = "pending"
    OutcomePartial = "partial"
)

// ActorType constants
const (
    ActorTypeService  = "service"
    ActorTypeExternal = "external"
    ActorTypeUser     = "user"
    ActorTypeSystem   = "system"
)

// NewAuditEvent creates a new audit event with defaults
func NewAuditEvent(eventType, category, action, outcome string) *AuditEvent {
    return &AuditEvent{
        EventID:        uuid.New(),
        EventVersion:   "1.0",
        EventTimestamp: time.Now(),
        EventType:      eventType,
        EventCategory:  category,
        EventAction:    action,
        EventOutcome:   outcome,
        RetentionDays:  2555, // 7 years default
        IsSensitive:    false,
    }
}

// NotificationAuditEvent creates an audit event for notification
func NotificationAuditEvent(notificationID, remediationID, recipient, channel, status string, eventData map[string]interface{}) (*AuditEvent, error) {
    event := NewAuditEvent(
        "notification."+status,  // e.g., "notification.sent", "notification.delivered"
        CategoryNotification,
        status,                  // "sent", "delivered", "failed"
        mapStatusToOutcome(status),
    )

    event.ResourceType = "Notification"
    event.ResourceID = notificationID
    event.CorrelationID = remediationID
    event.ActorType = ActorTypeService
    event.ActorID = "notification-service"

    // Add notification-specific data to event_data
    eventData["recipient"] = recipient
    eventData["channel"] = channel
    eventData["notification_id"] = notificationID

    eventDataJSON, err := json.Marshal(eventData)
    if err != nil {
        return nil, err
    }
    event.EventData = eventDataJSON

    return event, nil
}

// mapStatusToOutcome maps notification status to event outcome
func mapStatusToOutcome(status string) string {
    switch status {
    case "sent", "delivered":
        return OutcomeSuccess
    case "failed", "error":
        return OutcomeFailure
    case "pending", "queued":
        return OutcomePending
    default:
        return OutcomePending
    }
}
```

**Deliverable**: Shared audit library for all services

---

#### **Step 3: Update Data Storage Models**

**File**: `pkg/datastorage/models/audit_event.go`

```go
// pkg/datastorage/models/audit_event.go
package models

import (
    "github.com/jordigilh/kubernaut/pkg/audit"
)

// AuditEvent is an alias to the shared audit.AuditEvent
// This allows Data Storage to use the shared type without duplication
type AuditEvent = audit.AuditEvent

// Re-export constants for convenience
const (
    CategorySignal       = audit.CategorySignal
    CategoryNotification = audit.CategoryNotification
    CategoryRemediation  = audit.CategoryRemediation
    CategoryWorkflow     = audit.CategoryWorkflow
    CategoryAIAnalysis   = audit.CategoryAIAnalysis
    CategoryApproval     = audit.CategoryApproval

    OutcomeSuccess = audit.OutcomeSuccess
    OutcomeFailure = audit.OutcomeFailure
    OutcomePending = audit.OutcomePending
    OutcomePartial = audit.OutcomePartial

    ActorTypeService  = audit.ActorTypeService
    ActorTypeExternal = audit.ActorTypeExternal
    ActorTypeUser     = audit.ActorTypeUser
    ActorTypeSystem   = audit.ActorTypeSystem
)
```

**Action**: Delete `pkg/datastorage/models/notification_audit.go` (replaced by unified model)

**Deliverable**: Data Storage using shared audit model

---

#### **Step 4: Update Repository**

**File**: `pkg/datastorage/repository/audit_event_repository.go` (NEW)

```go
// pkg/datastorage/repository/audit_event_repository.go
package repository

import (
    "context"
    "database/sql"
    "fmt"

    "github.com/jordigilh/kubernaut/pkg/audit"
    "github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

type AuditEventRepository struct {
    db *sql.DB
}

func NewAuditEventRepository(db *sql.DB) *AuditEventRepository {
    return &AuditEventRepository{db: db}
}

// CreateAuditEvent inserts a new audit event
func (r *AuditEventRepository) CreateAuditEvent(ctx context.Context, event *audit.AuditEvent) error {
    query := `
        INSERT INTO audit_events (
            event_id, event_version, event_timestamp,
            event_type, event_category, event_action, event_outcome,
            actor_type, actor_id, actor_ip,
            resource_type, resource_id, resource_name,
            correlation_id, parent_event_id, trace_id, span_id,
            namespace, cluster_name,
            event_data, event_metadata,
            severity, duration_ms, error_code, error_message,
            retention_days, is_sensitive
        ) VALUES (
            $1, $2, $3,
            $4, $5, $6, $7,
            $8, $9, $10,
            $11, $12, $13,
            $14, $15, $16, $17,
            $18, $19,
            $20, $21,
            $22, $23, $24, $25,
            $26, $27
        )
    `

    _, err := r.db.ExecContext(ctx, query,
        event.EventID, event.EventVersion, event.EventTimestamp,
        event.EventType, event.EventCategory, event.EventAction, event.EventOutcome,
        event.ActorType, event.ActorID, event.ActorIP,
        event.ResourceType, event.ResourceID, event.ResourceName,
        event.CorrelationID, event.ParentEventID, event.TraceID, event.SpanID,
        event.Namespace, event.ClusterName,
        event.EventData, event.EventMetadata,
        event.Severity, event.DurationMs, event.ErrorCode, event.ErrorMessage,
        event.RetentionDays, event.IsSensitive,
    )

    if err != nil {
        return fmt.Errorf("failed to create audit event: %w", err)
    }

    return nil
}

// GetAuditEventsByCorrelation retrieves all events for a correlation_id
func (r *AuditEventRepository) GetAuditEventsByCorrelation(ctx context.Context, correlationID string) ([]*audit.AuditEvent, error) {
    query := `
        SELECT event_id, event_version, event_timestamp,
               event_type, event_category, event_action, event_outcome,
               actor_type, actor_id, actor_ip,
               resource_type, resource_id, resource_name,
               correlation_id, parent_event_id, trace_id, span_id,
               namespace, cluster_name,
               event_data, event_metadata,
               severity, duration_ms, error_code, error_message,
               retention_days, is_sensitive
        FROM audit_events
        WHERE correlation_id = $1
        ORDER BY event_timestamp DESC
    `

    rows, err := r.db.QueryContext(ctx, query, correlationID)
    if err != nil {
        return nil, fmt.Errorf("failed to query audit events: %w", err)
    }
    defer rows.Close()

    var events []*audit.AuditEvent
    for rows.Next() {
        event := &audit.AuditEvent{}
        err := rows.Scan(
            &event.EventID, &event.EventVersion, &event.EventTimestamp,
            &event.EventType, &event.EventCategory, &event.EventAction, &event.EventOutcome,
            &event.ActorType, &event.ActorID, &event.ActorIP,
            &event.ResourceType, &event.ResourceID, &event.ResourceName,
            &event.CorrelationID, &event.ParentEventID, &event.TraceID, &event.SpanID,
            &event.Namespace, &event.ClusterName,
            &event.EventData, &event.EventMetadata,
            &event.Severity, &event.DurationMs, &event.ErrorCode, &event.ErrorMessage,
            &event.RetentionDays, &event.IsSensitive,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to scan audit event: %w", err)
        }
        events = append(events, event)
    }

    return events, nil
}
```

**Action**: Delete `pkg/datastorage/repository/notification_audit_repository.go` (replaced by unified repository)

**Deliverable**: Unified audit event repository

---

#### **Step 5: Update REST API Handlers**

**File**: `pkg/datastorage/server/audit_handlers.go` (UPDATE)

```go
// pkg/datastorage/server/audit_handlers.go
package server

import (
    "encoding/json"
    "net/http"

    "github.com/jordigilh/kubernaut/pkg/audit"
    "go.uber.org/zap"
)

// CreateAuditEventHandler handles POST /api/v1/audit-events
func (s *Server) CreateAuditEventHandler(w http.ResponseWriter, r *http.Request) {
    var event audit.AuditEvent
    if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
        s.logger.Error("failed to decode audit event", zap.Error(err))
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    // Validate audit event
    if err := s.validateAuditEvent(&event); err != nil {
        s.logger.Error("invalid audit event", zap.Error(err))
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Create audit event
    if err := s.auditEventRepo.CreateAuditEvent(r.Context(), &event); err != nil {
        s.logger.Error("failed to create audit event", zap.Error(err))

        // DLQ fallback (ADR-032 "No Audit Loss" mandate)
        if dlqErr := s.dlqClient.EnqueueAuditEvent(r.Context(), &event, err); dlqErr != nil {
            s.logger.Error("failed to enqueue audit event to DLQ", zap.Error(dlqErr))
        }

        http.Error(w, "failed to create audit event", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "event_id": event.EventID,
        "status":   "created",
    })
}

// GetAuditEventsByCorrelationHandler handles GET /api/v1/audit-events?correlation_id={id}
func (s *Server) GetAuditEventsByCorrelationHandler(w http.ResponseWriter, r *http.Request) {
    correlationID := r.URL.Query().Get("correlation_id")
    if correlationID == "" {
        http.Error(w, "correlation_id is required", http.StatusBadRequest)
        return
    }

    events, err := s.auditEventRepo.GetAuditEventsByCorrelation(r.Context(), correlationID)
    if err != nil {
        s.logger.Error("failed to get audit events", zap.Error(err))
        http.Error(w, "failed to get audit events", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "events":        events,
        "total_results": len(events),
    })
}

// validateAuditEvent validates audit event fields
func (s *Server) validateAuditEvent(event *audit.AuditEvent) error {
    if event.EventType == "" {
        return fmt.Errorf("event_type is required")
    }
    if event.EventCategory == "" {
        return fmt.Errorf("event_category is required")
    }
    if event.EventAction == "" {
        return fmt.Errorf("event_action is required")
    }
    if event.EventOutcome == "" {
        return fmt.Errorf("event_outcome is required")
    }
    if event.ActorType == "" {
        return fmt.Errorf("actor_type is required")
    }
    if event.ActorID == "" {
        return fmt.Errorf("actor_id is required")
    }
    if event.ResourceType == "" {
        return fmt.Errorf("resource_type is required")
    }
    if event.ResourceID == "" {
        return fmt.Errorf("resource_id is required")
    }
    if event.CorrelationID == "" {
        return fmt.Errorf("correlation_id is required")
    }
    if len(event.EventData) == 0 {
        return fmt.Errorf("event_data is required")
    }
    return nil
}
```

**Action**: Delete old `POST /api/v1/audits/notification` endpoint (replaced by unified endpoint)

**Deliverable**: Unified REST API endpoints

---

#### **Step 6: Update DLQ Client**

**File**: `pkg/datastorage/dlq/client.go` (UPDATE)

```go
// pkg/datastorage/dlq/client.go
package dlq

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/go-redis/redis/v8"
    "github.com/jordigilh/kubernaut/pkg/audit"
    "go.uber.org/zap"
)

// EnqueueAuditEvent enqueues a failed audit event to DLQ (unified)
func (c *Client) EnqueueAuditEvent(ctx context.Context, event *audit.AuditEvent, originalError error) error {
    // Serialize audit event
    eventJSON, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("failed to marshal audit event: %w", err)
    }

    // Create DLQ message
    dlqMessage := map[string]interface{}{
        "event_id":       event.EventID.String(),
        "event_type":     event.EventType,
        "event_category": event.EventCategory,
        "correlation_id": event.CorrelationID,
        "event_payload":  string(eventJSON),
        "error":          originalError.Error(),
        "enqueued_at":    time.Now().UTC().Format(time.RFC3339),
        "retry_count":    0,
    }

    // Add to Redis Stream: audit:dlq:events (unified stream)
    streamKey := "audit:dlq:events"
    _, err = c.redisClient.XAdd(ctx, &redis.XAddArgs{
        Stream: streamKey,
        Values: dlqMessage,
    }).Result()

    if err != nil {
        c.logger.Error("failed to enqueue audit event to DLQ",
            zap.String("event_id", event.EventID.String()),
            zap.Error(err))
        return fmt.Errorf("failed to enqueue to DLQ: %w", err)
    }

    c.logger.Info("audit event enqueued to DLQ",
        zap.String("event_id", event.EventID.String()),
        zap.String("event_type", event.EventType),
        zap.String("stream", streamKey))

    return nil
}
```

**Action**: Delete `EnqueueNotificationAudit()` method (replaced by unified method)

**Deliverable**: Unified DLQ client

---

#### **Step 7: Update Validation**

**File**: `pkg/datastorage/validation/audit_event_validator.go` (NEW)

```go
// pkg/datastorage/validation/audit_event_validator.go
package validation

import (
    "fmt"

    "github.com/jordigilh/kubernaut/pkg/audit"
)

// ValidateAuditEvent validates an audit event
func ValidateAuditEvent(event *audit.AuditEvent) error {
    if event.EventType == "" {
        return fmt.Errorf("event_type is required")
    }
    if event.EventCategory == "" {
        return fmt.Errorf("event_category is required")
    }
    if event.EventAction == "" {
        return fmt.Errorf("event_action is required")
    }
    if event.EventOutcome == "" {
        return fmt.Errorf("event_outcome is required")
    }
    if event.ActorType == "" {
        return fmt.Errorf("actor_type is required")
    }
    if event.ActorID == "" {
        return fmt.Errorf("actor_id is required")
    }
    if event.ResourceType == "" {
        return fmt.Errorf("resource_type is required")
    }
    if event.ResourceID == "" {
        return fmt.Errorf("resource_id is required")
    }
    if event.CorrelationID == "" {
        return fmt.Errorf("correlation_id is required")
    }
    if len(event.EventData) == 0 {
        return fmt.Errorf("event_data is required")
    }

    // Validate event_outcome
    validOutcomes := map[string]bool{
        audit.OutcomeSuccess: true,
        audit.OutcomeFailure: true,
        audit.OutcomePending: true,
        audit.OutcomePartial: true,
    }
    if !validOutcomes[event.EventOutcome] {
        return fmt.Errorf("invalid event_outcome: %s", event.EventOutcome)
    }

    // Validate event_category
    validCategories := map[string]bool{
        audit.CategorySignal:       true,
        audit.CategoryNotification: true,
        audit.CategoryRemediation:  true,
        audit.CategoryWorkflow:     true,
        audit.CategoryAIAnalysis:   true,
        audit.CategoryApproval:     true,
    }
    if !validCategories[event.EventCategory] {
        return fmt.Errorf("invalid event_category: %s", event.EventCategory)
    }

    return nil
}
```

**Action**: Delete `pkg/datastorage/validation/notification_audit_validator.go` (replaced by unified validator)

**Deliverable**: Unified audit event validator

---

## 📋 **Files to Create/Update/Delete**

### **Create** (NEW files):
1. ✅ `migrations/014_unified_audit_table.sql` - Unified audit table schema
2. ✅ `pkg/audit/audit_event.go` - Shared audit library (DD-AUDIT-002)
3. ✅ `pkg/datastorage/models/audit_event.go` - Alias to shared audit type
4. ✅ `pkg/datastorage/repository/audit_event_repository.go` - Unified repository
5. ✅ `pkg/datastorage/validation/audit_event_validator.go` - Unified validator

### **Update** (MODIFY existing files):
1. ✅ `pkg/datastorage/server/audit_handlers.go` - Replace notification endpoint with unified endpoint
2. ✅ `pkg/datastorage/dlq/client.go` - Replace `EnqueueNotificationAudit` with `EnqueueAuditEvent`
3. ✅ `pkg/datastorage/server/server.go` - Update handler registration
4. ✅ `pkg/datastorage/server/routes.go` - Update routes

### **Delete** (REMOVE old files):
1. ❌ `migrations/010_audit_write_api_phase1.sql` - Old notification_audit migration (superseded by 014)
2. ❌ `pkg/datastorage/models/notification_audit.go` - Replaced by unified model
3. ❌ `pkg/datastorage/repository/notification_audit_repository.go` - Replaced by unified repository
4. ❌ `pkg/datastorage/validation/notification_audit_validator.go` - Replaced by unified validator

---

## 🎯 **Migration Checklist**

### **Phase 1: Schema Migration** (Day 1, 4 hours)
- [ ] Create `migrations/014_unified_audit_table.sql`
- [ ] Test migration locally (drop notification_audit, create audit_events)
- [ ] Verify partitioning works
- [ ] Verify indexes created
- [ ] Test partition creation function

### **Phase 2: Shared Library** (Day 1, 2 hours)
- [ ] Create `pkg/audit/audit_event.go`
- [ ] Create `pkg/audit/audit_event_test.go` (unit tests)
- [ ] Test `NewAuditEvent()` factory
- [ ] Test `NotificationAuditEvent()` helper

### **Phase 3: Data Storage Updates** (Day 2, 6 hours)
- [ ] Create `pkg/datastorage/models/audit_event.go` (alias)
- [ ] Create `pkg/datastorage/repository/audit_event_repository.go`
- [ ] Create `pkg/datastorage/validation/audit_event_validator.go`
- [ ] Update `pkg/datastorage/server/audit_handlers.go`
- [ ] Update `pkg/datastorage/dlq/client.go`
- [ ] Update `pkg/datastorage/server/server.go` (handler registration)
- [ ] Update `pkg/datastorage/server/routes.go` (route registration)

### **Phase 4: Cleanup** (Day 2, 2 hours)
- [ ] Delete `migrations/010_audit_write_api_phase1.sql`
- [ ] Delete `pkg/datastorage/models/notification_audit.go`
- [ ] Delete `pkg/datastorage/repository/notification_audit_repository.go`
- [ ] Delete `pkg/datastorage/validation/notification_audit_validator.go`
- [ ] Update documentation to reference unified audit

### **Phase 5: Testing** (Day 3, 6 hours)
- [ ] Unit tests for `pkg/audit/audit_event.go`
- [ ] Unit tests for `pkg/datastorage/repository/audit_event_repository.go`
- [ ] Unit tests for `pkg/datastorage/validation/audit_event_validator.go`
- [ ] Integration tests for `POST /api/v1/audit-events`
- [ ] Integration tests for `GET /api/v1/audit-events?correlation_id={id}`
- [ ] Integration tests for DLQ fallback
- [ ] Test partition creation

---

## 📊 **Confidence Assessment**

**Overall Confidence**: **95%**

**Breakdown**:
- **Migration strategy**: 98% (drop and replace is simplest)
- **Schema design**: 100% (ADR-034 is authoritative)
- **Code refactoring**: 90% (straightforward replacement)
- **Testing strategy**: 95% (comprehensive test plan)
- **Timeline estimate**: 90% (3 days is realistic)

**Why 95% (not 100%)**:
- 5% uncertainty: Potential unknown dependencies on `notification_audit` in other services
  - **Mitigation**: Grep codebase for `notification_audit` references before deletion

---

## 🔗 **Related Decisions**

- **ADR-034**: Unified Audit Table Design (authoritative source)
- **DD-AUDIT-002**: Audit Shared Library (referenced in ADR-034)
- **DD-AUDIT-001**: Centralized Audit (WorkflowExecution Controller responsibility)
- **ADR-032**: Asynchronous Buffered Audit Ingestion
- **DD-STORAGE-007**: Redis Requirement (Mandatory for DLQ)
- **ADR-040**: Remediation Approval Request Architecture (uses unified audit)

---

## 🚀 **Next Steps**

**After DD-STORAGE-009 is approved**:
1. Create DD-STORAGE-010: Data Storage V1.0 Implementation Plan (detailed task breakdown)
2. Create DD-CONTEXT-006: Context API Deprecation Decision
3. Begin implementation of unified audit migration (3 days)

---

**Document Version**: 1.0
**Last Updated**: November 13, 2025
**Status**: ✅ **APPROVED** (95% confidence)
**Next Review**: After implementation complete

