# DD-STORAGE-010: Data Storage Service V1.0 Implementation Plan

**Date**: November 13, 2025
**Status**: ✅ **APPROVED** (ready for implementation after all plans defined)
**Decision Maker**: Kubernaut Data Storage Team
**Authority**: ADR-034 (Unified Audit), DD-STORAGE-008 (Playbook Schema), DD-STORAGE-009 (Audit Migration)
**Affects**: Data Storage Service V1.0 MVP
**Version**: 1.0

---

## 📋 **Version History**

| Version | Date | Author | Changes | Status |
|---------|------|--------|---------|--------|
| v1.0 | 2025-11-13 | Initial | V1.0 MVP implementation plan | ✅ Approved |

---

## 🎯 **Executive Summary**

**Purpose**: Implement Data Storage Service V1.0 MVP with unified audit trail and playbook catalog foundations.

**Key Features**:
- ✅ **Unified Audit Trail**: ADR-034 compliant `audit_events` table for all services
- ✅ **Playbook Catalog**: Schema, read-only API, semantic search (no caching)
- ✅ **Migration**: Drop `notification_audit`, migrate to unified `audit_events`
- ✅ **Redis DLQ**: Mandatory for audit trace integrity (DD-STORAGE-007)
- ✅ **No Caching**: Playbook embeddings generated real-time (DD-STORAGE-006)

**V1.0 Scope** (Foundation):
- Unified audit table with generic write/query API
- Playbook catalog table with semantic search API
- SQL-only playbook management (no REST API for writes)
- Real-time embedding generation (no caching)
- Integration tests for all features

**V1.0 Limitations** (Deferred to V1.1):
- ❌ Playbook creation/update REST API (SQL-only management)
- ❌ Version validation enforcement (manual SQL management)
- ❌ Lifecycle management API (disable/enable via SQL)
- ❌ Embedding caching (per DD-STORAGE-006)
- ❌ CRD controller for playbooks

**Timeline**: 6 days (40 hours)
**Confidence**: 95%

---

## 📊 **Context**

**Problem**: Data Storage Service needs V1.0 MVP foundations for:
1. **Unified Audit Trail**: Replace notification-specific audit with ADR-034 unified table
2. **Playbook Catalog**: Enable semantic search for incident remediation (DD-CONTEXT-005)

**Current State**:
- ✅ `notification_audit` table exists (migration 010)
- ❌ Does NOT follow ADR-034 unified audit design
- ❌ No playbook catalog table
- ❌ No semantic search implementation

**Target State** (V1.0 MVP):
- ✅ `audit_events` table (ADR-034 compliant)
- ✅ `playbook_catalog` table (DD-STORAGE-008 schema)
- ✅ Generic audit write/query API
- ✅ Playbook semantic search API
- ✅ Real-time embedding generation (no caching)
- ✅ Integration tests for all features

**Authoritative Sources**:
- **ADR-034**: Unified Audit Table Design (audit requirements)
- **DD-STORAGE-008**: Playbook Catalog Schema (playbook requirements)
- **DD-STORAGE-009**: Unified Audit Migration Plan (migration strategy)
- **DD-STORAGE-006**: V1.0 No-Cache Decision (caching strategy)
- **DD-STORAGE-007**: Redis Requirement Reassessment (DLQ mandatory)

---

## 📅 **IMPLEMENTATION TIMELINE**

### **Overview**

| Day | Phase | Duration | Focus | Deliverable |
|-----|-------|----------|-------|-------------|
| **Day 1** | Foundation | 8h | Unified Audit Schema + Shared Library | `audit_events` table, `pkg/audit/` library |
| **Day 2** | Migration | 8h | Audit Migration + Data Storage Updates | Unified repository, handlers, DLQ |
| **Day 3** | Playbook Foundation | 8h | Playbook Schema + Models + Repository | `playbook_catalog` table, read-only repo |
| **Day 4** | Semantic Search | 8h | Embedding Service + Search API | Real-time embeddings, search endpoint |
| **Day 5** | Integration Tests | 8h | Unified Audit + Playbook Tests | Comprehensive test coverage |
| **Day 6** | Documentation | 8h | API Docs + Migration Guide + Runbook | Production-ready documentation |

**Total Duration**: 6 days (40 hours)

---

## 🚨 **CRITICAL: APDC-Enhanced TDD Methodology**

**MANDATORY FOR ALL V1.0 IMPLEMENTATION**:

### **✅ CORRECT: APDC + Iterative TDD**

```
ANALYSIS Phase (5-15 min per feature)
  → Comprehensive context understanding
  → Business requirement validation
  → Existing implementation discovery

PLAN Phase (10-20 min per feature)
  → Detailed implementation strategy
  → TDD phase mapping
  → Success criteria definition

DO Phase (Variable)
  → DO-DISCOVERY: Search existing implementations
  → DO-RED: Write ONE failing test
  → DO-GREEN: Minimal implementation + integration
  → DO-REFACTOR: Enhance existing code
  → Repeat for each test (one at a time)

CHECK Phase (5-10 min per feature)
  → Business alignment verification
  → Integration testing validation
  → Confidence assessment
```

### **❌ FORBIDDEN: Batch TDD**

```
❌ Write all tests first
❌ Then implement all business logic
❌ Then run all tests together
```

**Why Forbidden**: Violates TDD principles, increases debugging complexity, harder to track progress

---

## 📊 **Day 1: Unified Audit Foundation** (8 hours)

### **Objective**

Create ADR-034 compliant unified audit table and shared library for all services.

**Business Requirements**:
- **BR-STORAGE-001**: Persist Notification Audit (migrate to unified table)
- **BR-STORAGE-002**: Typed Error Handling (extend to all audit types)
- **BR-STORAGE-003**: Database Version Validation (verify partitioning support)

---

### **Phase 1.1: Schema Migration (3 hours)**

**APDC Analysis** (15 min):
- Read ADR-034 for authoritative schema design
- Compare with existing `notification_audit` schema
- Identify migration requirements

**APDC Plan** (20 min):
- Create migration 014: Drop `notification_audit`, create `audit_events`
- Define partitioning strategy (monthly partitions)
- Plan index strategy for common queries

**APDC Do** (2 hours):

#### **DO-RED: Write Failing Schema Test**

**File**: `test/integration/datastorage/audit_events_schema_test.go`

```go
package datastorage_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Audit Events Schema - ADR-034 Compliance", func() {
    // BR-STORAGE-001: Unified audit table
    // Authority: ADR-034 (Unified Audit Table Design)

    Context("Table Structure", func() {
        It("should have audit_events table with ADR-034 columns", func() {
            // BEHAVIOR: audit_events table exists with correct schema
            // CORRECTNESS: Matches ADR-034 specification exactly

            var exists bool
            err := db.QueryRow(`
                SELECT EXISTS (
                    SELECT FROM information_schema.tables
                    WHERE table_schema = 'public'
                    AND table_name = 'audit_events'
                )
            `).Scan(&exists)
            Expect(err).ToNot(HaveOccurred())
            Expect(exists).To(BeTrue(), "audit_events table should exist")

            // Verify columns
            columns := []string{
                "event_id", "event_type", "event_category", "event_outcome",
                "event_timestamp", "event_date", "service_name", "service_version",
                "actor_type", "actor_id", "actor_name", "resource_type",
                "resource_id", "resource_name", "event_data", "correlation_id",
                "parent_event_id", "trace_id", "span_id", "metadata",
            }
            for _, col := range columns {
                var colExists bool
                err := db.QueryRow(`
                    SELECT EXISTS (
                        SELECT FROM information_schema.columns
                        WHERE table_name = 'audit_events' AND column_name = $1
                    )
                `, col).Scan(&colExists)
                Expect(err).ToNot(HaveOccurred())
                Expect(colExists).To(BeTrue(), "Column %s should exist", col)
            }
        })

        It("should be partitioned by event_date", func() {
            // BEHAVIOR: Table is partitioned for performance
            // CORRECTNESS: Monthly partitions per ADR-034

            var isPartitioned bool
            err := db.QueryRow(`
                SELECT EXISTS (
                    SELECT FROM pg_partitioned_table
                    WHERE partrelid = 'audit_events'::regclass
                )
            `).Scan(&isPartitioned)
            Expect(err).ToNot(HaveOccurred())
            Expect(isPartitioned).To(BeTrue(), "audit_events should be partitioned")
        })

        It("should have required indexes", func() {
            // BEHAVIOR: Indexes exist for common queries
            // CORRECTNESS: ADR-034 index strategy

            indexes := []string{
                "idx_audit_events_event_type",
                "idx_audit_events_correlation_id",
                "idx_audit_events_service_name",
                "idx_audit_events_event_timestamp",
            }
            for _, idx := range indexes {
                var idxExists bool
                err := db.QueryRow(`
                    SELECT EXISTS (
                        SELECT FROM pg_indexes
                        WHERE tablename = 'audit_events' AND indexname = $1
                    )
                `, idx).Scan(&idxExists)
                Expect(err).ToNot(HaveOccurred())
                Expect(idxExists).To(BeTrue(), "Index %s should exist", idx)
            }
        })
    })
})
```

**Expected**: ❌ Tests fail (table doesn't exist)

---

#### **DO-GREEN: Create Migration**

**File**: `migrations/014_unified_audit_events.sql`

```sql
-- Migration 014: Unified Audit Events Table (ADR-034)
-- Authority: ADR-034 (Unified Audit Table Design)
-- Replaces: migration 010 (notification_audit)

-- Drop old notification_audit table (no backward compatibility needed)
DROP TABLE IF EXISTS notification_audit CASCADE;

-- Create unified audit_events table (partitioned by event_date)
CREATE TABLE audit_events (
    -- Event Identity
    event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(100) NOT NULL,           -- e.g., "notification.sent", "workflow.started"
    event_category VARCHAR(50) NOT NULL,        -- e.g., "notification", "workflow", "ai"
    event_outcome VARCHAR(20) NOT NULL,         -- "success", "failure", "pending"
    event_timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    event_date DATE NOT NULL DEFAULT CURRENT_DATE,

    -- Service Context
    service_name VARCHAR(100) NOT NULL,         -- e.g., "data-storage", "gateway"
    service_version VARCHAR(50),                -- e.g., "v1.0.0"

    -- Actor (Who)
    actor_type VARCHAR(50) NOT NULL,            -- "system", "user", "service", "operator"
    actor_id VARCHAR(255),                      -- User ID, service account, etc.
    actor_name VARCHAR(255),                    -- Human-readable actor name

    -- Resource (What)
    resource_type VARCHAR(100) NOT NULL,        -- "notification", "workflow", "remediation"
    resource_id VARCHAR(255) NOT NULL,          -- Notification ID, workflow ID, etc.
    resource_name VARCHAR(255),                 -- Human-readable resource name

    -- Event Data (JSONB for flexibility)
    event_data JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Correlation Tracking (Signal Flow)
    correlation_id VARCHAR(255),                -- Remediation ID (links related events)
    parent_event_id UUID,                       -- Parent event (for nested events)
    trace_id VARCHAR(255),                      -- OpenTelemetry trace ID
    span_id VARCHAR(255),                       -- OpenTelemetry span ID

    -- Metadata
    metadata JSONB DEFAULT '{}'::jsonb,         -- Additional context

    -- Constraints
    CHECK (event_outcome IN ('success', 'failure', 'pending', 'skipped')),
    CHECK (actor_type IN ('system', 'user', 'service', 'operator', 'controller'))
) PARTITION BY RANGE (event_date);

-- Create indexes for common queries
CREATE INDEX idx_audit_events_event_type ON audit_events(event_type);
CREATE INDEX idx_audit_events_event_category ON audit_events(event_category);
CREATE INDEX idx_audit_events_correlation_id ON audit_events(correlation_id) WHERE correlation_id IS NOT NULL;
CREATE INDEX idx_audit_events_service_name ON audit_events(service_name);
CREATE INDEX idx_audit_events_event_timestamp ON audit_events(event_timestamp DESC);
CREATE INDEX idx_audit_events_resource ON audit_events(resource_type, resource_id);
CREATE INDEX idx_audit_events_actor ON audit_events(actor_type, actor_id) WHERE actor_id IS NOT NULL;
CREATE INDEX idx_audit_events_event_data ON audit_events USING GIN (event_data);

-- Create initial partitions (current month + next 3 months)
CREATE TABLE audit_events_2025_11 PARTITION OF audit_events
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

CREATE TABLE audit_events_2025_12 PARTITION OF audit_events
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

CREATE TABLE audit_events_2026_01 PARTITION OF audit_events
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE audit_events_2026_02 PARTITION OF audit_events
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

-- Partition maintenance function (create new partitions automatically)
CREATE OR REPLACE FUNCTION create_audit_events_partition()
RETURNS void AS $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    -- Create partition for 3 months in the future
    partition_date := CURRENT_DATE + INTERVAL '3 months';
    partition_name := 'audit_events_' || TO_CHAR(partition_date, 'YYYY_MM');
    start_date := DATE_TRUNC('month', partition_date);
    end_date := start_date + INTERVAL '1 month';

    -- Check if partition already exists
    IF NOT EXISTS (
        SELECT 1 FROM pg_class WHERE relname = partition_name
    ) THEN
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF audit_events FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Schedule partition creation (run monthly via cron or k8s CronJob)
-- Example: kubectl create cronjob audit-partition-maintenance --schedule="0 0 1 * *" --image=postgres:16 -- psql -c "SELECT create_audit_events_partition()"

-- Comments for documentation
COMMENT ON TABLE audit_events IS 'Unified audit events table (ADR-034) - stores all service audit events with event sourcing pattern';
COMMENT ON COLUMN audit_events.event_type IS 'Event type (e.g., notification.sent, workflow.started) - defines event taxonomy';
COMMENT ON COLUMN audit_events.event_data IS 'JSONB payload with event-specific data - flexible schema per event type';
COMMENT ON COLUMN audit_events.correlation_id IS 'Remediation ID or other correlation identifier - links related events across services';
```

**Expected**: ✅ Tests pass (table exists with correct schema)

---

#### **DO-REFACTOR: Add Partition Management**

**Enhancement**: Automated partition creation for long-term retention

**File**: `scripts/audit-partition-maintenance.sh`

```bash
#!/bin/bash
# Automated audit_events partition creation
# Run monthly via Kubernetes CronJob

psql "$DATABASE_URL" -c "SELECT create_audit_events_partition();"
```

**Expected**: ✅ Tests still pass, partition management automated

---

**APDC Check** (15 min):
- ✅ audit_events table exists with ADR-034 schema
- ✅ Partitioned by event_date for performance
- ✅ Indexes support common queries
- ✅ Partition maintenance automated
- **Confidence**: 100% (ADR-034 authoritative)

---

### **Phase 1.2: Shared Audit Library (3 hours)**

**APDC Analysis** (15 min):
- Read DD-AUDIT-002 for shared library requirements
- Identify common audit event patterns across services
- Plan factory methods for each service

**APDC Plan** (20 min):
- Create `pkg/audit/` shared library
- Define `AuditEvent` struct (ADR-034 compliant)
- Create factory methods: `NewAuditEvent()`, `NotificationAuditEvent()`
- Define constants: `EventCategory`, `EventOutcome`, `ActorType`

**APDC Do** (2 hours):

#### **DO-RED: Write Failing Library Test**

**File**: `test/unit/audit/audit_event_test.go`

```go
package audit_test

import (
    "testing"
    "time"

    "github.com/jordigilh/kubernaut/pkg/audit"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestAuditEvent(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Audit Event Shared Library Test Suite")
}

var _ = Describe("Audit Event Factory", func() {
    // BR-STORAGE-001: Unified audit events
    // Authority: ADR-034, DD-AUDIT-002

    Context("NewAuditEvent", func() {
        It("should create audit event with required fields", func() {
            // BEHAVIOR: Factory creates valid audit event
            // CORRECTNESS: All required fields populated

            event := audit.NewAuditEvent(
                "notification.sent",
                audit.EventCategoryNotification,
                audit.EventOutcomeSuccess,
                "data-storage",
                "v1.0.0",
                audit.ActorTypeSystem,
                "",
                "",
                "notification",
                "notif-12345",
                "Email Notification",
                map[string]interface{}{
                    "recipient": "user@example.com",
                    "channel":   "email",
                },
            )

            Expect(event.EventType).To(Equal("notification.sent"))
            Expect(event.EventCategory).To(Equal(audit.EventCategoryNotification))
            Expect(event.EventOutcome).To(Equal(audit.EventOutcomeSuccess))
            Expect(event.ServiceName).To(Equal("data-storage"))
            Expect(event.ActorType).To(Equal(audit.ActorTypeSystem))
            Expect(event.ResourceType).To(Equal("notification"))
            Expect(event.ResourceID).To(Equal("notif-12345"))
            Expect(event.EventData).To(HaveKeyWithValue("recipient", "user@example.com"))
        })

        It("should auto-populate event_id and timestamps", func() {
            // BEHAVIOR: System fields auto-populated
            // CORRECTNESS: UUID event_id, current timestamp

            event := audit.NewAuditEvent(
                "test.event", audit.EventCategoryNotification, audit.EventOutcomeSuccess,
                "test-service", "v1.0.0", audit.ActorTypeSystem, "", "",
                "test", "test-123", "Test Resource", nil,
            )

            Expect(event.EventID).ToNot(BeEmpty(), "event_id should be auto-generated")
            Expect(event.EventTimestamp).To(BeTemporally("~", time.Now(), time.Second))
            Expect(event.EventDate).To(Equal(time.Now().Format("2006-01-02")))
        })
    })

    Context("NotificationAuditEvent Factory", func() {
        It("should create notification audit event with correct defaults", func() {
            // BEHAVIOR: Notification-specific factory
            // CORRECTNESS: Defaults for notification events

            event := audit.NotificationAuditEvent(
                "notif-12345",
                "sent",
                "user@example.com",
                "email",
                map[string]interface{}{"subject": "Alert"},
            )

            Expect(event.EventType).To(Equal("notification.sent"))
            Expect(event.EventCategory).To(Equal(audit.EventCategoryNotification))
            Expect(event.EventOutcome).To(Equal(audit.EventOutcomeSuccess))
            Expect(event.ServiceName).To(Equal("data-storage"))
            Expect(event.ResourceType).To(Equal("notification"))
            Expect(event.ResourceID).To(Equal("notif-12345"))
            Expect(event.EventData).To(HaveKeyWithValue("recipient", "user@example.com"))
            Expect(event.EventData).To(HaveKeyWithValue("channel", "email"))
        })
    })
})
```

**Expected**: ❌ Tests fail (`pkg/audit` doesn't exist)

---

#### **DO-GREEN: Create Shared Library**

**File**: `pkg/audit/audit_event.go`

```go
// Package audit provides shared audit event types and factory methods
// Authority: ADR-034 (Unified Audit Table Design), DD-AUDIT-002 (Shared Audit Library)
package audit

import (
    "time"

    "github.com/google/uuid"
)

// Event Categories (ADR-034)
const (
    EventCategoryNotification = "notification"
    EventCategoryWorkflow     = "workflow"
    EventCategoryAI           = "ai"
    EventCategoryOrchestrator = "orchestrator"
    EventCategoryGateway      = "gateway"
    EventCategoryEffectiveness = "effectiveness"
)

// Event Outcomes (ADR-034)
const (
    EventOutcomeSuccess = "success"
    EventOutcomeFailure = "failure"
    EventOutcomePending = "pending"
    EventOutcomeSkipped = "skipped"
)

// Actor Types (ADR-034)
const (
    ActorTypeSystem     = "system"
    ActorTypeUser       = "user"
    ActorTypeService    = "service"
    ActorTypeOperator   = "operator"
    ActorTypeController = "controller"
)

// AuditEvent represents a unified audit event (ADR-034 compliant)
type AuditEvent struct {
    // Event Identity
    EventID        string                 `json:"event_id" db:"event_id"`
    EventType      string                 `json:"event_type" db:"event_type"`
    EventCategory  string                 `json:"event_category" db:"event_category"`
    EventOutcome   string                 `json:"event_outcome" db:"event_outcome"`
    EventTimestamp time.Time              `json:"event_timestamp" db:"event_timestamp"`
    EventDate      string                 `json:"event_date" db:"event_date"`

    // Service Context
    ServiceName    string  `json:"service_name" db:"service_name"`
    ServiceVersion *string `json:"service_version,omitempty" db:"service_version"`

    // Actor (Who)
    ActorType string  `json:"actor_type" db:"actor_type"`
    ActorID   *string `json:"actor_id,omitempty" db:"actor_id"`
    ActorName *string `json:"actor_name,omitempty" db:"actor_name"`

    // Resource (What)
    ResourceType string  `json:"resource_type" db:"resource_type"`
    ResourceID   string  `json:"resource_id" db:"resource_id"`
    ResourceName *string `json:"resource_name,omitempty" db:"resource_name"`

    // Event Data (JSONB)
    EventData map[string]interface{} `json:"event_data" db:"event_data"`

    // Correlation Tracking
    CorrelationID *string `json:"correlation_id,omitempty" db:"correlation_id"`
    ParentEventID *string `json:"parent_event_id,omitempty" db:"parent_event_id"`
    TraceID       *string `json:"trace_id,omitempty" db:"trace_id"`
    SpanID        *string `json:"span_id,omitempty" db:"span_id"`

    // Metadata
    Metadata map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
}

// NewAuditEvent creates a new audit event with required fields
func NewAuditEvent(
    eventType, eventCategory, eventOutcome string,
    serviceName, serviceVersion string,
    actorType, actorID, actorName string,
    resourceType, resourceID, resourceName string,
    eventData map[string]interface{},
) *AuditEvent {
    now := time.Now()
    event := &AuditEvent{
        EventID:        uuid.New().String(),
        EventType:      eventType,
        EventCategory:  eventCategory,
        EventOutcome:   eventOutcome,
        EventTimestamp: now,
        EventDate:      now.Format("2006-01-02"),
        ServiceName:    serviceName,
        ActorType:      actorType,
        ResourceType:   resourceType,
        ResourceID:     resourceID,
        EventData:      eventData,
    }

    // Optional fields
    if serviceVersion != "" {
        event.ServiceVersion = &serviceVersion
    }
    if actorID != "" {
        event.ActorID = &actorID
    }
    if actorName != "" {
        event.ActorName = &actorName
    }
    if resourceName != "" {
        event.ResourceName = &resourceName
    }

    // Initialize empty maps if nil
    if event.EventData == nil {
        event.EventData = make(map[string]interface{})
    }

    return event
}

// NotificationAuditEvent creates a notification audit event (convenience factory)
func NotificationAuditEvent(
    notificationID, status, recipient, channel string,
    additionalData map[string]interface{},
) *AuditEvent {
    eventData := map[string]interface{}{
        "recipient": recipient,
        "channel":   channel,
        "status":    status,
    }

    // Merge additional data
    for k, v := range additionalData {
        eventData[k] = v
    }

    return NewAuditEvent(
        "notification."+status,       // event_type: "notification.sent", "notification.failed"
        EventCategoryNotification,    // event_category: "notification"
        EventOutcomeSuccess,          // event_outcome: "success" (adjust based on status)
        "data-storage",               // service_name
        "v1.0.0",                     // service_version
        ActorTypeSystem,              // actor_type: "system"
        "",                           // actor_id (empty for system)
        "",                           // actor_name (empty for system)
        "notification",               // resource_type
        notificationID,               // resource_id
        "Email Notification",         // resource_name
        eventData,                    // event_data
    )
}
```

**Expected**: ✅ Tests pass (shared library works)

---

#### **DO-REFACTOR: Add More Factory Methods**

**Enhancement**: Add factories for other services (workflow, AI, etc.)

**File**: `pkg/audit/audit_event.go` (append)

```go
// WorkflowAuditEvent creates a workflow audit event
func WorkflowAuditEvent(
    workflowID, phase, status string,
    additionalData map[string]interface{},
) *AuditEvent {
    eventData := map[string]interface{}{
        "phase":  phase,
        "status": status,
    }
    for k, v := range additionalData {
        eventData[k] = v
    }

    return NewAuditEvent(
        "workflow."+phase,
        EventCategoryWorkflow,
        EventOutcomeSuccess,
        "workflow-execution-controller",
        "v1.0.0",
        ActorTypeController,
        "",
        "",
        "workflow",
        workflowID,
        "Workflow Execution",
        eventData,
    )
}

// AIAnalysisAuditEvent creates an AI analysis audit event
func AIAnalysisAuditEvent(
    analysisID, analysisType string,
    additionalData map[string]interface{},
) *AuditEvent {
    eventData := map[string]interface{}{
        "analysis_type": analysisType,
    }
    for k, v := range additionalData {
        eventData[k] = v
    }

    return NewAuditEvent(
        "ai.analysis."+analysisType,
        EventCategoryAI,
        EventOutcomeSuccess,
        "ai-service",
        "v1.0.0",
        ActorTypeService,
        "",
        "",
        "analysis",
        analysisID,
        "AI Analysis",
        eventData,
    )
}
```

**Expected**: ✅ Tests still pass, more factories available

---

**APDC Check** (15 min):
- ✅ Shared audit library created (`pkg/audit/`)
- ✅ AuditEvent struct (ADR-034 compliant)
- ✅ Factory methods for all services
- ✅ Constants for event taxonomy
- **Confidence**: 100% (ADR-034 authoritative)

---

### **Phase 1.3: Documentation (2 hours)**

**Tasks**:
1. Update `docs/architecture/decisions/ADR-034-unified-audit-table-design.md` with V1.0 implementation status
2. Create `docs/services/stateless/data-storage/AUDIT_EVENT_TAXONOMY.md` (event type reference)
3. Update `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md` (BR-STORAGE-001 updated)

**Deliverable**: Day 1 complete, unified audit foundation ready

---

## 📊 **Day 2: Audit Migration + Data Storage Updates** (8 hours)

### **Objective**

Migrate Data Storage Service from `notification_audit` to unified `audit_events` table.

**Business Requirements**:
- **BR-STORAGE-001**: Persist Notification Audit (migrate to unified table)
- **BR-STORAGE-002**: Typed Error Handling (extend to all audit types)

---

### **Phase 2.1: Data Storage Repository (3 hours)**

**APDC Analysis** (15 min):
- Read existing `notification_audit_repository.go`
- Identify reusable patterns
- Plan unified repository interface

**APDC Plan** (20 min):
- Create `pkg/datastorage/repository/audit_event_repository.go`
- Define `AuditEventRepository` interface
- Implement `CreateAuditEvent()`, `QueryAuditEvents()`
- Delete old `notification_audit_repository.go`

**APDC Do** (2 hours):

#### **DO-RED: Write Failing Repository Test**

**File**: `test/integration/datastorage/audit_event_repository_test.go`

```go
package datastorage_test

import (
    "context"
    "time"

    "github.com/jordigilh/kubernaut/pkg/audit"
    "github.com/jordigilh/kubernaut/pkg/datastorage/repository"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Audit Event Repository", func() {
    // BR-STORAGE-001: Unified audit persistence
    // Authority: ADR-034

    var (
        repo repository.AuditEventRepository
        ctx  context.Context
    )

    BeforeEach(func() {
        repo = repository.NewAuditEventRepository(db)
        ctx = context.Background()
    })

    Context("CreateAuditEvent", func() {
        It("should persist audit event to audit_events table", func() {
            // BEHAVIOR: Audit event persisted to database
            // CORRECTNESS: All fields stored correctly

            event := audit.NotificationAuditEvent(
                "notif-12345",
                "sent",
                "user@example.com",
                "email",
                map[string]interface{}{"subject": "Test Alert"},
            )

            err := repo.CreateAuditEvent(ctx, event)
            Expect(err).ToNot(HaveOccurred())

            // Verify event persisted
            var count int
            err = db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE resource_id = $1`, "notif-12345").Scan(&count)
            Expect(err).ToNot(HaveOccurred())
            Expect(count).To(Equal(1))
        })

        It("should handle JSONB event_data correctly", func() {
            // BEHAVIOR: JSONB data stored and queryable
            // CORRECTNESS: Complex nested data preserved

            event := audit.NewAuditEvent(
                "test.event", audit.EventCategoryNotification, audit.EventOutcomeSuccess,
                "test-service", "v1.0.0", audit.ActorTypeSystem, "", "",
                "test", "test-123", "Test", map[string]interface{}{
                    "nested": map[string]interface{}{
                        "key": "value",
                    },
                },
            )

            err := repo.CreateAuditEvent(ctx, event)
            Expect(err).ToNot(HaveOccurred())

            // Query JSONB data
            var nestedValue string
            err = db.QueryRow(`
                SELECT event_data->'nested'->>'key'
                FROM audit_events
                WHERE resource_id = $1
            `, "test-123").Scan(&nestedValue)
            Expect(err).ToNot(HaveOccurred())
            Expect(nestedValue).To(Equal("value"))
        })
    })

    Context("QueryAuditEvents", func() {
        BeforeEach(func() {
            // Insert test data
            events := []*audit.AuditEvent{
                audit.NotificationAuditEvent("notif-1", "sent", "user1@example.com", "email", nil),
                audit.NotificationAuditEvent("notif-2", "failed", "user2@example.com", "email", nil),
                audit.WorkflowAuditEvent("workflow-1", "started", "running", nil),
            }
            for _, e := range events {
                _ = repo.CreateAuditEvent(ctx, e)
            }
        })

        It("should query by event_category", func() {
            // BEHAVIOR: Filter by event_category
            // CORRECTNESS: Returns only matching events

            events, err := repo.QueryAuditEvents(ctx, &repository.AuditEventFilters{
                EventCategory: "notification",
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(events).To(HaveLen(2))
        })

        It("should query by correlation_id", func() {
            // BEHAVIOR: Filter by correlation_id (remediation ID)
            // CORRECTNESS: Returns all events for remediation

            // Insert events with same correlation_id
            correlationID := "remediation-12345"
            events := []*audit.AuditEvent{
                audit.NotificationAuditEvent("notif-3", "sent", "user@example.com", "email", nil),
                audit.WorkflowAuditEvent("workflow-2", "started", "running", nil),
            }
            for _, e := range events {
                e.CorrelationID = &correlationID
                _ = repo.CreateAuditEvent(ctx, e)
            }

            results, err := repo.QueryAuditEvents(ctx, &repository.AuditEventFilters{
                CorrelationID: &correlationID,
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(results).To(HaveLen(2))
        })
    })
})
```

**Expected**: ❌ Tests fail (repository doesn't exist)

---

#### **DO-GREEN: Implement Repository**

**File**: `pkg/datastorage/repository/audit_event_repository.go`

```go
package repository

import (
    "context"
    "database/sql"
    "encoding/json"

    "github.com/jordigilh/kubernaut/pkg/audit"
    "github.com/jmoiron/sqlx"
)

// AuditEventRepository provides database operations for audit events
type AuditEventRepository interface {
    CreateAuditEvent(ctx context.Context, event *audit.AuditEvent) error
    QueryAuditEvents(ctx context.Context, filters *AuditEventFilters) ([]*audit.AuditEvent, error)
}

// AuditEventFilters for querying audit events
type AuditEventFilters struct {
    EventCategory *string
    EventType     *string
    ServiceName   *string
    CorrelationID *string
    ResourceType  *string
    ResourceID    *string
    StartTime     *string
    EndTime       *string
    Limit         int
    Offset        int
}

type auditEventRepository struct {
    db *sqlx.DB
}

// NewAuditEventRepository creates a new audit event repository
func NewAuditEventRepository(db *sqlx.DB) AuditEventRepository {
    return &auditEventRepository{db: db}
}

// CreateAuditEvent persists an audit event to the database
func (r *auditEventRepository) CreateAuditEvent(ctx context.Context, event *audit.AuditEvent) error {
    // Convert event_data to JSONB
    eventDataJSON, err := json.Marshal(event.EventData)
    if err != nil {
        return err
    }

    metadataJSON := []byte("{}")
    if event.Metadata != nil {
        metadataJSON, err = json.Marshal(event.Metadata)
        if err != nil {
            return err
        }
    }

    _, err = r.db.ExecContext(ctx, `
        INSERT INTO audit_events (
            event_id, event_type, event_category, event_outcome,
            event_timestamp, event_date, service_name, service_version,
            actor_type, actor_id, actor_name, resource_type, resource_id, resource_name,
            event_data, correlation_id, parent_event_id, trace_id, span_id, metadata
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
        )
    `,
        event.EventID, event.EventType, event.EventCategory, event.EventOutcome,
        event.EventTimestamp, event.EventDate, event.ServiceName, event.ServiceVersion,
        event.ActorType, event.ActorID, event.ActorName, event.ResourceType, event.ResourceID, event.ResourceName,
        eventDataJSON, event.CorrelationID, event.ParentEventID, event.TraceID, event.SpanID, metadataJSON,
    )

    return err
}

// QueryAuditEvents retrieves audit events based on filters
func (r *auditEventRepository) QueryAuditEvents(ctx context.Context, filters *AuditEventFilters) ([]*audit.AuditEvent, error) {
    query := `SELECT * FROM audit_events WHERE 1=1`
    args := []interface{}{}
    argIdx := 1

    if filters.EventCategory != nil {
        query += ` AND event_category = $` + string(rune(argIdx))
        args = append(args, *filters.EventCategory)
        argIdx++
    }

    if filters.CorrelationID != nil {
        query += ` AND correlation_id = $` + string(rune(argIdx))
        args = append(args, *filters.CorrelationID)
        argIdx++
    }

    query += ` ORDER BY event_timestamp DESC`

    if filters.Limit > 0 {
        query += ` LIMIT $` + string(rune(argIdx))
        args = append(args, filters.Limit)
        argIdx++
    }

    if filters.Offset > 0 {
        query += ` OFFSET $` + string(rune(argIdx))
        args = append(args, filters.Offset)
    }

    rows, err := r.db.QueryxContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    events := []*audit.AuditEvent{}
    for rows.Next() {
        var event audit.AuditEvent
        var eventDataJSON, metadataJSON []byte

        err := rows.Scan(
            &event.EventID, &event.EventType, &event.EventCategory, &event.EventOutcome,
            &event.EventTimestamp, &event.EventDate, &event.ServiceName, &event.ServiceVersion,
            &event.ActorType, &event.ActorID, &event.ActorName, &event.ResourceType, &event.ResourceID, &event.ResourceName,
            &eventDataJSON, &event.CorrelationID, &event.ParentEventID, &event.TraceID, &event.SpanID, &metadataJSON,
        )
        if err != nil {
            return nil, err
        }

        // Unmarshal JSONB
        if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
            return nil, err
        }
        if len(metadataJSON) > 0 && string(metadataJSON) != "{}" {
            if err := json.Unmarshal(metadataJSON, &event.Metadata); err != nil {
                return nil, err
            }
        }

        events = append(events, &event)
    }

    return events, rows.Err()
}
```

**Expected**: ✅ Tests pass (repository works)

---

**APDC Check** (15 min):
- ✅ Unified audit repository created
- ✅ CreateAuditEvent() persists to audit_events
- ✅ QueryAuditEvents() supports filtering
- ✅ JSONB event_data handled correctly
- **Confidence**: 95%

---

### **Phase 2.2: HTTP Handlers (2 hours)**

**APDC Do**: Update `pkg/datastorage/server/audit_handlers.go` to use unified audit repository

**Tasks**:
1. Replace `POST /api/v1/audits/notification` with `POST /api/v1/audit-events`
2. Add `GET /api/v1/audit-events` for querying
3. Update DLQ client to use `EnqueueAuditEvent` (generic)

**Deliverable**: Unified audit write/query API

---

### **Phase 2.3: DLQ Updates (1 hour)**

**APDC Do**: Update `pkg/datastorage/dlq/client.go` to use generic audit events

**Tasks**:
1. Rename `EnqueueNotificationAudit` to `EnqueueAuditEvent`
2. Update DLQ consumer to use `audit.AuditEvent`
3. Update integration tests

**Deliverable**: Generic DLQ for all audit types

---

### **Phase 2.4: Cleanup (2 hours)**

**Tasks**:
1. Delete `migrations/010_audit_write_api_phase1.sql` (superseded)
2. Delete `pkg/datastorage/models/notification_audit.go`
3. Delete `pkg/datastorage/repository/notification_audit_repository.go`
4. Delete `pkg/datastorage/validation/notification_audit_validator.go`
5. Update all references to use unified audit

**Deliverable**: Day 2 complete, unified audit migration done

---

## 📊 **Day 3: Playbook Catalog Foundation** (8 hours)

### **Objective**

Create playbook catalog table, models, and read-only repository.

**Business Requirements**:
- **BR-STORAGE-012**: Playbook Catalog Embedding Generation
- **BR-STORAGE-013**: Semantic Search API

---

### **Phase 3.1: Playbook Schema (2 hours)**

**APDC Analysis** (15 min):
- Read DD-STORAGE-008 for authoritative schema
- Verify pgvector extension installed
- Plan HNSW index strategy

**APDC Plan** (20 min):
- Create migration 015: `playbook_catalog` table
- Define composite primary key (playbook_id, version)
- Add indexes for search, labels, embeddings

**APDC Do** (1.5 hours):

#### **DO-RED: Write Failing Schema Test**

**File**: `test/integration/datastorage/playbook_catalog_schema_test.go`

```go
package datastorage_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Playbook Catalog Schema - DD-STORAGE-008 Compliance", func() {
    // BR-STORAGE-012: Playbook catalog storage
    // Authority: DD-STORAGE-008

    Context("Table Structure", func() {
        It("should have playbook_catalog table with DD-STORAGE-008 columns", func() {
            // BEHAVIOR: playbook_catalog table exists with correct schema
            // CORRECTNESS: Matches DD-STORAGE-008 specification

            var exists bool
            err := db.QueryRow(`
                SELECT EXISTS (
                    SELECT FROM information_schema.tables
                    WHERE table_schema = 'public'
                    AND table_name = 'playbook_catalog'
                )
            `).Scan(&exists)
            Expect(err).ToNot(HaveOccurred())
            Expect(exists).To(BeTrue(), "playbook_catalog table should exist")

            // Verify composite primary key
            var pkColumns string
            err = db.QueryRow(`
                SELECT string_agg(column_name, ', ' ORDER BY ordinal_position)
                FROM information_schema.key_column_usage
                WHERE table_name = 'playbook_catalog'
                AND constraint_name = 'playbook_catalog_pkey'
            `).Scan(&pkColumns)
            Expect(err).ToNot(HaveOccurred())
            Expect(pkColumns).To(Equal("playbook_id, version"))
        })

        It("should have pgvector embedding column with HNSW index", func() {
            // BEHAVIOR: Embedding column with vector type
            // CORRECTNESS: pgvector extension, HNSW index

            var dataType string
            err := db.QueryRow(`
                SELECT data_type
                FROM information_schema.columns
                WHERE table_name = 'playbook_catalog' AND column_name = 'embedding'
            `).Scan(&dataType)
            Expect(err).ToNot(HaveOccurred())
            Expect(dataType).To(Equal("USER-DEFINED"), "embedding should be vector type")

            // Verify HNSW index
            var indexExists bool
            err = db.QueryRow(`
                SELECT EXISTS (
                    SELECT FROM pg_indexes
                    WHERE tablename = 'playbook_catalog'
                    AND indexname = 'idx_playbook_catalog_embedding'
                )
            `).Scan(&indexExists)
            Expect(err).ToNot(HaveOccurred())
            Expect(indexExists).To(BeTrue(), "HNSW index should exist")
        })

        It("should have GIN index on labels JSONB column", func() {
            // BEHAVIOR: Labels queryable with GIN index
            // CORRECTNESS: JSONB column with GIN index

            var indexExists bool
            err := db.QueryRow(`
                SELECT EXISTS (
                    SELECT FROM pg_indexes
                    WHERE tablename = 'playbook_catalog'
                    AND indexname = 'idx_playbook_catalog_labels'
                )
            `).Scan(&indexExists)
            Expect(err).ToNot(HaveOccurred())
            Expect(indexExists).To(BeTrue(), "GIN index on labels should exist")
        })
    })
})
```

**Expected**: ❌ Tests fail (table doesn't exist)

---

#### **DO-GREEN: Create Migration**

**File**: `migrations/015_playbook_catalog.sql`

```sql
-- Migration 015: Playbook Catalog Table (DD-STORAGE-008)
-- Authority: DD-STORAGE-008 (Playbook Catalog Schema)
-- Purpose: Store remediation playbooks with semantic search support

-- Ensure pgvector extension is installed
CREATE EXTENSION IF NOT EXISTS vector;

-- Create playbook_catalog table
CREATE TABLE playbook_catalog (
    -- Identity (Composite Primary Key)
    playbook_id VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,

    -- Metadata
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    owner VARCHAR(255),
    maintainer VARCHAR(255),

    -- Content
    content TEXT NOT NULL,
    content_hash VARCHAR(64) NOT NULL,

    -- Labels (JSONB for flexible filtering)
    labels JSONB NOT NULL,

    -- Semantic Search (pgvector)
    embedding vector(384),

    -- Lifecycle Management
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    disabled_at TIMESTAMP WITH TIME ZONE,
    disabled_by VARCHAR(255),
    disabled_reason TEXT,

    -- Version Management
    is_latest_version BOOLEAN NOT NULL DEFAULT false,
    previous_version VARCHAR(50),
    deprecation_notice TEXT,

    -- Version Change Metadata
    version_notes TEXT,
    change_summary TEXT,
    approved_by VARCHAR(255),
    approved_at TIMESTAMP WITH TIME ZONE,

    -- Success Metrics
    expected_success_rate DECIMAL(4,3),
    expected_duration_seconds INTEGER,
    actual_success_rate DECIMAL(4,3),
    total_executions INTEGER DEFAULT 0,
    successful_executions INTEGER DEFAULT 0,

    -- Audit Trail
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),
    updated_by VARCHAR(255),

    -- Constraints
    PRIMARY KEY (playbook_id, version),
    CHECK (status IN ('active', 'disabled', 'deprecated', 'archived')),
    CHECK (expected_success_rate IS NULL OR (expected_success_rate >= 0 AND expected_success_rate <= 1)),
    CHECK (actual_success_rate IS NULL OR (actual_success_rate >= 0 AND actual_success_rate <= 1)),
    CHECK (total_executions >= 0),
    CHECK (successful_executions >= 0 AND successful_executions <= total_executions)
);

-- Indexes for Query Performance
CREATE INDEX idx_playbook_catalog_status ON playbook_catalog(status);
CREATE INDEX idx_playbook_catalog_latest ON playbook_catalog(playbook_id, is_latest_version) WHERE is_latest_version = true;
CREATE INDEX idx_playbook_catalog_labels ON playbook_catalog USING GIN (labels);
CREATE INDEX idx_playbook_catalog_embedding ON playbook_catalog USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
CREATE INDEX idx_playbook_catalog_created_at ON playbook_catalog(created_at DESC);
CREATE INDEX idx_playbook_catalog_success_rate ON playbook_catalog(actual_success_rate DESC) WHERE status = 'active';

-- Trigger for updated_at
CREATE OR REPLACE FUNCTION update_playbook_catalog_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER playbook_catalog_updated_at
    BEFORE UPDATE ON playbook_catalog
    FOR EACH ROW
    EXECUTE FUNCTION update_playbook_catalog_updated_at();

-- Comments
COMMENT ON TABLE playbook_catalog IS 'Remediation playbook catalog with semantic search support (DD-STORAGE-008)';
COMMENT ON COLUMN playbook_catalog.embedding IS 'sentence-transformers/all-MiniLM-L6-v2 (384 dimensions)';
COMMENT ON COLUMN playbook_catalog.labels IS 'JSONB labels for DD-CONTEXT-005 filtering (environment, priority, business_category)';
```

**Expected**: ✅ Tests pass (table exists with correct schema)

---

**APDC Check** (15 min):
- ✅ playbook_catalog table exists with DD-STORAGE-008 schema
- ✅ Composite primary key (playbook_id, version)
- ✅ pgvector embedding column with HNSW index
- ✅ JSONB labels with GIN index
- **Confidence**: 100% (DD-STORAGE-008 authoritative)

---

### **Phase 3.2: Playbook Models (2 hours)**

**APDC Do**: Create `pkg/datastorage/models/playbook.go` (from DD-STORAGE-008)

**Deliverable**: Playbook Go models

---

### **Phase 3.3: Playbook Repository (4 hours)**

**APDC Do**: Create `pkg/datastorage/repository/playbook_repository.go` with read-only operations

**Methods**:
- `GetLatestVersion(playbook_id)` - Get latest version
- `GetVersion(playbook_id, version)` - Get specific version
- `ListVersions(playbook_id)` - List all versions
- `SearchPlaybooks(query, labels, min_confidence)` - Semantic search

**Deliverable**: Day 3 complete, playbook catalog foundation ready

---

## 📊 **Day 4: Semantic Search Implementation** (8 hours)

### **Objective**

Implement real-time embedding generation and semantic search API.

**Business Requirements**:
- **BR-STORAGE-012**: Playbook Catalog Embedding Generation
- **BR-STORAGE-013**: Semantic Search API

---

### **Phase 4.1: Python Embedding Service (3 hours)**

**APDC Do**: Create Python HTTP server for embedding generation

**Tasks**:
1. Create `services/embedding-service/server.py`
2. Load sentence-transformers/all-MiniLM-L6-v2 model
3. Implement `POST /embed` endpoint
4. Dockerize embedding service

**Deliverable**: Embedding service ready

---

### **Phase 4.2: Go Embedding Client (2 hours)**

**APDC Do**: Create `pkg/datastorage/embedding/client.go`

**Methods**:
- `GenerateEmbedding(text string) ([]float32, error)` - Generate embedding
- **NO CACHING** (per DD-STORAGE-006)

**Deliverable**: Go client for embedding service

---

### **Phase 4.3: Semantic Search API (3 hours)**

**APDC Do**: Implement `GET /api/v1/playbooks/search` endpoint

**Query Parameters**:
- `query`: Incident description
- `label.*`: Label filters (DD-CONTEXT-005)
- `min_confidence`: Similarity threshold (default: 0.7)
- `max_results`: Limit (default: 10)

**Response Format** (DD-CONTEXT-005):
```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod on OOM",
      "confidence": 0.92
    }
  ],
  "total_results": 1
}
```

**Deliverable**: Day 4 complete, semantic search working

---

## 📊 **Day 5: Integration Tests** (8 hours)

### **Objective**

Comprehensive integration tests for unified audit and playbook catalog.

**Business Requirements**: All V1.0 BRs

---

### **Defense-in-Depth Testing Strategy**

Data Storage Service follows the **testing pyramid** with defense-in-depth validation:

```
         /\
        /  \  E2E (10%)
       /____\
      /      \
     / Integ. \ (20%)
    /__________\
   /            \
  /    Unit      \ (70%)
 /________________\
```

**Philosophy**: Each BR is validated at multiple levels, ensuring comprehensive coverage

**V1.0 Test Distribution**:
- **Unit Tests**: 70% coverage (audit library, repository, models)
- **Integration Tests**: 20% coverage (database, HTTP API, embedding service)
- **E2E Tests**: 10% coverage (full workflow: audit write → query, playbook search)

---

### **Phase 5.1: Unified Audit Integration Tests (3 hours)**

**Test Scenarios**:
1. Audit event persistence (all event types)
2. JSONB event_data querying
3. Correlation ID tracking (signal flow)
4. Partition management
5. DLQ fallback on write failure

**Deliverable**: Unified audit integration tests

---

### **Phase 5.2: Playbook Catalog Integration Tests (3 hours)**

**Test Scenarios**:
1. Playbook version listing
2. Semantic search with label filtering
3. Real-time embedding generation
4. HNSW index performance
5. Composite primary key enforcement

**Deliverable**: Playbook catalog integration tests

---

### **Phase 5.3: E2E Tests (2 hours)**

**Test Scenarios**:
1. Full audit workflow: Write → Query → Correlation tracking
2. Full playbook workflow: Search → Select → Execute (mock)

**Deliverable**: Day 5 complete, comprehensive test coverage

---

## 📊 **Day 6: Documentation + Production Readiness** (8 hours)

### **Objective**

Production-ready documentation and deployment artifacts.

---

### **Phase 6.1: API Documentation (2 hours)**

**Tasks**:
1. OpenAPI spec for unified audit API
2. OpenAPI spec for playbook catalog API
3. Postman collection for manual testing

**Deliverable**: API documentation

---

### **Phase 6.2: Migration Guide (2 hours)**

**Tasks**:
1. Create `docs/services/stateless/data-storage/MIGRATION_GUIDE_V1.0.md`
2. Document migration from notification_audit to audit_events
3. Document playbook catalog SQL management

**Deliverable**: Migration guide

---

### **Phase 6.3: Production Runbook (2 hours)**

**Tasks**:
1. Create `docs/services/stateless/data-storage/RUNBOOK_V1.0.md`
2. Document operational procedures (partition maintenance, DLQ monitoring)
3. Document troubleshooting scenarios

**Deliverable**: Production runbook

---

### **Phase 6.4: Final Validation (2 hours)**

**Tasks**:
1. Run full test suite (unit + integration + E2E)
2. Verify all BRs have test coverage
3. Confidence assessment

**Deliverable**: Day 6 complete, V1.0 MVP ready for deployment

---

## 📊 **Business Requirement Coverage Matrix**

**V1.0 MVP Coverage**:

| BR ID | Description | Unit | Integration | E2E | Total | Coverage % |
|-------|-------------|------|-------------|-----|-------|------------|
| **BR-STORAGE-001** | Unified Audit Persistence | 5 | 8 | 1 | 14 | 1400% |
| **BR-STORAGE-002** | Typed Error Handling | 3 | 2 | 0 | 5 | 500% |
| **BR-STORAGE-003** | Database Version Validation | 3 | 0 | 0 | 3 | 300% |
| **BR-STORAGE-012** | Playbook Embedding Generation | 3 | 5 | 1 | 9 | 900% |
| **BR-STORAGE-013** | Semantic Search API | 5 | 8 | 1 | 14 | 1400% |

**Total Tests**: 45 (32 unit, 23 integration, 3 E2E)
**Average Coverage**: 1000% (10 tests per BR)

---

## ✅ **V1.0 MVP Success Criteria**

**Must Have** (Blocking):
1. ✅ `audit_events` table exists with ADR-034 schema
2. ✅ `playbook_catalog` table exists with DD-STORAGE-008 schema
3. ✅ `POST /api/v1/audit-events` persists unified audit events
4. ✅ `GET /api/v1/audit-events` queries with correlation tracking
5. ✅ `GET /api/v1/playbooks/search` returns semantically similar playbooks
6. ✅ `GET /api/v1/playbooks/{id}/versions` lists all versions
7. ✅ Embedding generation works (real-time, no cache)
8. ✅ Integration tests pass for all features
9. ✅ Latency acceptable: 2.5s for 50 playbooks (per DD-STORAGE-006)
10. ✅ Redis DLQ operational for audit integrity (per DD-STORAGE-007)

**Nice to Have** (Non-Blocking):
- ⚠️ Playbook management via SQL (manual process, documented)
- ⚠️ Version validation via manual review (no automation)

---

## 📊 **Confidence Assessment**

**Overall Confidence**: **95%**

**Breakdown**:
- **Unified Audit Migration**: 98% (ADR-034 authoritative, clear migration path)
- **Playbook Catalog Foundation**: 95% (DD-STORAGE-008 authoritative, schema validated)
- **Semantic Search**: 90% (embedding service dependency, real-time performance)
- **Integration Tests**: 95% (established patterns from Context API)
- **Timeline Accuracy**: 92% (6 days realistic, 8% buffer for unknowns)

**Why 95% (not 100%)**:
- 5% uncertainty: Embedding service performance under load (mitigated by DD-STORAGE-006 analysis)

---

## 🔗 **Related Decisions**

- **ADR-034**: Unified Audit Table Design (audit requirements)
- **DD-STORAGE-008**: Playbook Catalog Schema (playbook requirements)
- **DD-STORAGE-009**: Unified Audit Migration Plan (migration strategy)
- **DD-STORAGE-006**: V1.0 No-Cache Decision (caching strategy)
- **DD-STORAGE-007**: Redis Requirement Reassessment (DLQ mandatory)
- **DD-CONTEXT-005**: Minimal LLM Response Schema (query/response format)
- **DD-CONTEXT-006**: Context API Deprecation Decision (salvageable patterns)

---

## 📋 **Next Steps**

1. ✅ **DD-STORAGE-010 Approved** (this document)
2. 🚧 **Create DD-CONTEXT-006-MIGRATION**: Context API migration plan (salvage patterns)
3. 🚧 **Create DD-STORAGE-011**: Data Storage V1.1 Implementation Plan (high-level)
4. 🚧 **Execute Day 1**: Unified Audit Foundation (8 hours)
5. 🚧 **Execute Day 2**: Audit Migration (8 hours)
6. 🚧 **Execute Day 3**: Playbook Catalog Foundation (8 hours)
7. 🚧 **Execute Day 4**: Semantic Search (8 hours)
8. 🚧 **Execute Day 5**: Integration Tests (8 hours)
9. 🚧 **Execute Day 6**: Documentation (8 hours)

---

**Document Version**: 1.0
**Last Updated**: November 13, 2025
**Status**: ✅ **APPROVED** (95% confidence, ready for implementation after all plans defined)
**Next Review**: After Day 3 complete (playbook catalog foundation ready)

