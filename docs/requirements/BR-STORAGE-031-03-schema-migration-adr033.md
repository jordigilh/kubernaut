# BR-STORAGE-031-03: Schema Migration for ADR-033 Multi-Dimensional Tracking

**Business Requirement ID**: BR-STORAGE-031-03  
**Category**: Data Storage Service  
**Priority**: P0  
**Target Version**: V1  
**Status**: ✅ Approved  
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 introduces **Multi-Dimensional Success Tracking** (incident_type + playbook + action) for remediation effectiveness. The current `resource_action_traces` schema lacks the columns required to track:
- **Incident Type** (PRIMARY dimension): Which type of incident was remediated
- **Playbook** (SECONDARY dimension): Which playbook was executed
- **AI Execution Mode** (Hybrid Model): How AI selected the remediation strategy

**Current Limitations**:
- ❌ No `incident_type` column → Cannot track success rates by incident type
- ❌ No `playbook_id`/`playbook_version` → Cannot track playbook effectiveness
- ❌ No AI execution mode flags → Cannot validate ADR-033 Hybrid Model (90-9-1 distribution)
- ❌ Only `workflow_id` exists → Meaningless for AI-generated unique workflows

**Impact**:
- Cannot implement BR-STORAGE-031-01 (incident-type API)
- Cannot implement BR-STORAGE-031-02 (playbook API)
- AI cannot learn from historical remediation effectiveness
- No data foundation for ADR-033 Remediation Playbook Catalog

---

## 🎯 **Business Objective**

**Add 7 new columns to `resource_action_traces` table to enable multi-dimensional success tracking as defined in ADR-033.**

### **Success Criteria**
1. ✅ Schema migration adds 7 new columns (incident_type, playbook_id, playbook_version, etc.)
2. ✅ All new columns use native Go types (string, int, bool) - NO `sql.Null*` types (pre-release)
3. ✅ 7 indexes created for efficient aggregation queries
4. ✅ Migration is backward-compatible (nullable columns, no data loss)
5. ✅ Migration script is idempotent (can run multiple times safely)
6. ✅ Rollback script provided for emergency reversion
7. ✅ Zero downtime migration (additive changes only)

---

## 📊 **Use Cases**

### **Use Case 1: Enable Incident-Type Success Rate Tracking**

**Scenario**: AI needs to query success rates by incident type (e.g., `pod-oom-killer`).

**Current Flow** (Without BR-STORAGE-031-03):
```
1. AI queries Data Storage for incident-type success rate
2. Database lacks `incident_type` column
3. ❌ Query fails or returns meaningless data
4. ❌ AI cannot learn from historical effectiveness
```

**Desired Flow with BR-STORAGE-031-03**:
```
1. Migration adds `incident_type` column to schema
2. RemediationExecutor populates `incident_type` on audit creation
3. AI queries Data Storage: SELECT ... WHERE incident_type = 'pod-oom-killer'
4. ✅ Query returns accurate success rate for that incident type
5. ✅ AI can make data-driven playbook selections
```

---

### **Use Case 2: Track Playbook Effectiveness Across Versions**

**Scenario**: Team wants to compare `pod-oom-recovery v1.2` vs `v1.1` effectiveness.

**Current Flow**:
```
1. Team deploys new playbook version v1.2
2. Database lacks `playbook_id` and `playbook_version` columns
3. ❌ Cannot differentiate between v1.1 and v1.2 execution records
4. ❌ Cannot measure playbook version improvement
```

**Desired Flow with BR-STORAGE-031-03**:
```
1. Migration adds `playbook_id` and `playbook_version` columns
2. RemediationExecutor populates these fields on each execution
3. Query success rates: WHERE playbook_id = 'pod-oom-recovery' AND playbook_version = 'v1.2'
4. ✅ Measurable comparison: v1.2 (89% success) vs v1.1 (50% success)
5. ✅ Data-driven validation of new playbook versions
```

---

### **Use Case 3: Validate ADR-033 Hybrid Model Distribution**

**Scenario**: Architecture team wants to validate the ADR-033 Hybrid Model (90% catalog + 9% chaining + 1% manual).

**Current Flow**:
```
1. Architecture team wants to measure AI execution mode distribution
2. Database lacks AI execution mode flags
3. ❌ Cannot validate ADR-033 Hybrid Model assumptions
4. ❌ Cannot detect if AI is over-chaining or under-utilizing catalog
```

**Desired Flow with BR-STORAGE-031-03**:
```
1. Migration adds `ai_selected_playbook`, `ai_chained_playbooks`, `ai_manual_escalation` flags
2. RemediationExecutor sets appropriate flag on each execution
3. Query execution mode distribution:
   SELECT
     SUM(CASE WHEN ai_selected_playbook THEN 1 ELSE 0 END) AS catalog_selected,
     SUM(CASE WHEN ai_chained_playbooks THEN 1 ELSE 0 END) AS chained,
     SUM(CASE WHEN ai_manual_escalation THEN 1 ELSE 0 END) AS manual
   FROM resource_action_traces
   WHERE action_timestamp >= NOW() - INTERVAL '7 days'
4. ✅ Results: catalog=1800 (90%), chained=180 (9%), manual=20 (1%)
5. ✅ Validated ADR-033 Hybrid Model distribution
```

---

## 🔧 **Functional Requirements**

### **FR-STORAGE-031-03-01: Schema Migration Script**

**Requirement**: Data Storage Service SHALL provide a goose migration script to add 7 new columns to `resource_action_traces` table.

**Migration Script**: `migrations/002_adr033_multidimensional_tracking.sql`

**SQL Implementation**:
```sql
-- +goose Up
-- Add columns for multi-dimensional success tracking (ADR-033)
ALTER TABLE resource_action_traces
    ADD COLUMN incident_type VARCHAR(100),
    ADD COLUMN alert_name VARCHAR(255),
    ADD COLUMN incident_severity VARCHAR(20),
    ADD COLUMN playbook_id VARCHAR(64),
    ADD COLUMN playbook_version VARCHAR(20),
    ADD COLUMN playbook_step_number INT,
    ADD COLUMN playbook_execution_id VARCHAR(64),
    ADD COLUMN ai_selected_playbook BOOLEAN DEFAULT FALSE,
    ADD COLUMN ai_chained_playbooks BOOLEAN DEFAULT FALSE,
    ADD COLUMN ai_manual_escalation BOOLEAN DEFAULT FALSE,
    ADD COLUMN ai_playbook_customization JSONB;

-- Add indexes for performance on new columns
CREATE INDEX idx_resource_action_traces_incident_type ON resource_action_traces (incident_type);
CREATE INDEX idx_resource_action_traces_alert_name ON resource_action_traces (alert_name);
CREATE INDEX idx_resource_action_traces_incident_severity ON resource_action_traces (incident_severity);
CREATE INDEX idx_resource_action_traces_playbook_id ON resource_action_traces (playbook_id);
CREATE INDEX idx_resource_action_traces_playbook_version ON resource_action_traces (playbook_version);
CREATE INDEX idx_resource_action_traces_playbook_execution_id ON resource_action_traces (playbook_execution_id);
CREATE INDEX idx_resource_action_traces_ai_execution_mode ON resource_action_traces (ai_selected_playbook, ai_chained_playbooks, ai_manual_escalation);

-- Add column comments for documentation
COMMENT ON COLUMN resource_action_traces.incident_type IS 'Primary classification of the incident (e.g., pod-oom-killer)';
COMMENT ON COLUMN resource_action_traces.alert_name IS 'Original Prometheus alert name associated with the incident';
COMMENT ON COLUMN resource_action_traces.incident_severity IS 'Severity of the incident (e.g., critical, warning, info)';
COMMENT ON COLUMN resource_action_traces.playbook_id IS 'Identifier for the remediation playbook used (e.g., pod-oom-recovery)';
COMMENT ON COLUMN resource_action_traces.playbook_version IS 'Version of the remediation playbook used (e.g., v1.2, v2.0, etc.)';
COMMENT ON COLUMN resource_action_traces.playbook_step_number IS 'Step number within the playbook execution';
COMMENT ON COLUMN resource_action_traces.playbook_execution_id IS 'Unique ID for a single execution of a playbook (groups all its steps)';
COMMENT ON COLUMN resource_action_traces.ai_selected_playbook IS 'True if AI selected a single playbook from the catalog';
COMMENT ON COLUMN resource_action_traces.ai_chained_playbooks IS 'True if AI chained multiple playbooks from the catalog';
COMMENT ON COLUMN resource_action_traces.ai_manual_escalation IS 'True if AI escalated to human operator';
COMMENT ON COLUMN resource_action_traces.ai_playbook_customization IS 'JSONB field for AI-driven parameter customizations to the playbook';

-- +goose Down
-- Revert changes
ALTER TABLE resource_action_traces
    DROP COLUMN incident_type,
    DROP COLUMN alert_name,
    DROP COLUMN incident_severity,
    DROP COLUMN playbook_id,
    DROP COLUMN playbook_version,
    DROP COLUMN playbook_step_number,
    DROP COLUMN playbook_execution_id,
    DROP COLUMN ai_selected_playbook,
    DROP COLUMN ai_chained_playbooks,
    DROP COLUMN ai_manual_escalation,
    DROP COLUMN ai_playbook_customization;

DROP INDEX IF EXISTS idx_resource_action_traces_incident_type;
DROP INDEX IF EXISTS idx_resource_action_traces_alert_name;
DROP INDEX IF EXISTS idx_resource_action_traces_incident_severity;
DROP INDEX IF EXISTS idx_resource_action_traces_playbook_id;
DROP INDEX IF EXISTS idx_resource_action_traces_playbook_version;
DROP INDEX IF EXISTS idx_resource_action_traces_playbook_execution_id;
DROP INDEX IF EXISTS idx_resource_action_traces_ai_execution_mode;
```

**Acceptance Criteria**:
- ✅ Migration script is syntactically valid PostgreSQL
- ✅ Migration adds exactly 7 new columns (3 for incident, 4 for playbook, 4 for AI mode)
- ✅ All new columns are nullable (backward compatible)
- ✅ Migration creates exactly 7 new indexes
- ✅ Rollback script removes all added columns and indexes cleanly
- ✅ Migration is idempotent (re-running does not cause errors)

---

### **FR-STORAGE-031-03-02: Go Model Updates**

**Requirement**: Data Storage Service SHALL update the `NotificationAudit` Go struct to reflect new schema columns.

**Go Model**: `pkg/datastorage/models/notification_audit.go`

**Pre-Release Simplification** (NO `sql.Null*` types):
```go
package models

type NotificationAudit struct {
    // ========================================
    // EXISTING FIELDS (KEEP ALL - BACKWARD COMPATIBLE)
    // ========================================
    ActionID        string          `json:"action_id" db:"action_id"`
    ActionType      string          `json:"action_type" db:"action_type"`
    Status          string          `json:"status" db:"status"`
    // ... existing fields ...

    // ========================================
    // ADR-033: NEW FIELDS (NATIVE TYPES - PRE-RELEASE)
    // ========================================

    // DIMENSION 1: INCIDENT TYPE (PRIMARY) - REQUIRED for ADR-033
    IncidentType     string `json:"incident_type" db:"incident_type"`           // REQUIRED
    AlertName        string `json:"alert_name,omitempty" db:"alert_name"`       // OPTIONAL
    IncidentSeverity string `json:"incident_severity" db:"incident_severity"`   // REQUIRED

    // DIMENSION 2: PLAYBOOK (SECONDARY) - REQUIRED for ADR-033
    PlaybookID          string `json:"playbook_id" db:"playbook_id"`                         // REQUIRED
    PlaybookVersion     string `json:"playbook_version" db:"playbook_version"`               // REQUIRED
    PlaybookStepNumber  int    `json:"playbook_step_number,omitempty" db:"playbook_step_number"` // OPTIONAL
    PlaybookExecutionID string `json:"playbook_execution_id" db:"playbook_execution_id"`     // REQUIRED

    // AI EXECUTION MODE (HYBRID MODEL: 90% catalog + 9% chaining + 1% manual)
    AISelectedPlaybook     bool            `json:"ai_selected_playbook" db:"ai_selected_playbook"`
    AIChainedPlaybooks     bool            `json:"ai_chained_playbooks" db:"ai_chained_playbooks"`
    AIManualEscalation     bool            `json:"ai_manual_escalation" db:"ai_manual_escalation"`
    AIPlaybookCustomization json.RawMessage `json:"ai_playbook_customization,omitempty" db:"ai_playbook_customization"`
}
```

**Acceptance Criteria**:
- ✅ All new fields use native Go types (`string`, `int`, `bool`, `json.RawMessage`)
- ❌ NO `sql.NullString`, `sql.NullInt32`, or `sql.NullBool` types (pre-release simplification)
- ✅ JSON tags use `omitempty` for optional fields
- ✅ Struct compiles without errors
- ✅ All fields are exported (public)

---

### **FR-STORAGE-031-03-03: Zero Downtime Migration**

**Requirement**: Migration SHALL be executed without downtime or data loss.

**Migration Strategy**:
1. **Phase 1**: Add nullable columns (additive change, no downtime)
2. **Phase 2**: Create indexes concurrently (PostgreSQL `CREATE INDEX CONCURRENTLY`)
3. **Phase 3**: Deploy RemediationExecutor update to populate new fields
4. **Phase 4**: Backfill historical data (optional, low priority)

**Acceptance Criteria**:
- ✅ Migration does not lock `resource_action_traces` table for writes
- ✅ Existing queries continue to work during migration
- ✅ New columns default to NULL (no mandatory values during transition)
- ✅ No data loss or corruption during migration

---

## 📈 **Non-Functional Requirements**

### **NFR-STORAGE-031-03-01: Performance**

- ✅ Migration execution time <5 minutes for 1M existing records
- ✅ Index creation does not block writes (use `CONCURRENTLY`)
- ✅ Query performance maintained after migration (indexes on new columns)

### **NFR-STORAGE-031-03-02: Data Integrity**

- ✅ No data loss during migration
- ✅ Rollback script restores original schema exactly
- ✅ Foreign key constraints maintained (if applicable)

### **NFR-STORAGE-031-03-03: Backward Compatibility**

- ✅ Existing API endpoints continue to work (new fields omitted for older clients)
- ✅ Existing queries do not break (new columns are nullable)
- ✅ No breaking changes for Context API or RemediationExecutor

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Remediation Playbook Catalog (architectural decision)
- ✅ PostgreSQL 16+ (per DD-011)
- ✅ goose migration tool installed

### **Downstream Impacts**
- ✅ BR-STORAGE-031-01: Incident-type API requires these columns
- ✅ BR-STORAGE-031-02: Playbook API requires these columns
- ✅ BR-REMEDIATION-015: RemediationExecutor must populate `incident_type`
- ✅ BR-REMEDIATION-016: RemediationExecutor must populate playbook fields

---

## 🚀 **Implementation Phases**

### **Phase 1: Migration Script Creation** (Day 12 - 2 hours)
- Write `002_adr033_multidimensional_tracking.sql`
- Test migration on local PostgreSQL database
- Validate rollback script works correctly

### **Phase 2: Go Model Updates** (Day 12 - 2 hours)
- Update `NotificationAudit` struct with new fields
- Run `go build` to validate struct changes
- Update OpenAPI spec with new fields

### **Phase 3: Migration Execution** (Day 12 - 1 hour)
- Apply migration to development database
- Verify indexes created successfully
- Test aggregation query performance

### **Phase 4: Integration Test Updates** (Day 12 - 2 hours)
- Update integration tests to use new fields
- Test backward compatibility (existing tests still pass)
- Add new tests for ADR-033 fields

**Total Estimated Effort**: 7 hours (1 day)

---

## 📊 **Success Metrics**

### **Migration Success**
- **Target**: 100% success rate on first migration attempt
- **Measure**: Zero failed migrations in dev/staging/production

### **Performance Impact**
- **Target**: <5% query performance degradation post-migration
- **Measure**: Compare P95 query times before/after migration

### **Data Integrity**
- **Target**: 100% data integrity maintained
- **Measure**: Row count before migration = row count after migration

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Create New Table for ADR-033 Data**

**Approach**: Create `incident_playbook_tracking` table with foreign key to `resource_action_traces`

**Rejected Because**:
- ❌ Complexity: Requires JOIN queries for aggregation
- ❌ Performance: JOINs are slower than single-table queries
- ❌ Maintenance: Two tables to manage instead of one

---

### **Alternative 2: Use JSONB for All ADR-033 Data**

**Approach**: Add single `adr033_metadata JSONB` column

**Rejected Because**:
- ❌ Performance: JSONB queries are slower than native column queries
- ❌ Indexing: Harder to create efficient indexes on JSONB fields
- ❌ Type Safety: No compile-time type checking for Go structs

---

### **Alternative 3: Require Non-Nullable Columns**

**Approach**: Add columns as `NOT NULL` with default values

**Rejected Because**:
- ❌ Backward Compatibility: Breaks existing RemediationExecutor deployments
- ❌ Data Quality: Default values would be meaningless for historical records
- ❌ Risk: Higher migration failure risk (requires backfill strategy)

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**  
**Date**: November 5, 2025  
**Decision**: Implement as P0 priority (foundation for all ADR-033 features)  
**Rationale**: Required for all other BR-STORAGE-031-XX requirements  
**Approved By**: Architecture Team  
**Related ADR**: [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-STORAGE-031-01: Incident-Type Success Rate API
- BR-STORAGE-031-02: Playbook Success Rate API
- BR-REMEDIATION-015: Populate incident_type on audit creation
- BR-REMEDIATION-016: Populate playbook metadata on audit creation

### **Related Documents**
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [DD-011: PostgreSQL 16+ Version Requirements](../architecture/decisions/DD-011-postgresql-version-requirements.md)
- [Data Storage Implementation Plan V5.0](../services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V5.0.md)
- [Migration Script: 002_adr033_multidimensional_tracking.sql](../services/stateless/data-storage/migrations/002_adr033_multidimensional_tracking.sql)

---

**Document Version**: 1.0  
**Last Updated**: November 5, 2025  
**Status**: ✅ Approved for V1 Implementation

