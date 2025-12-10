# 📋 REQUEST: Shared E2E Migration Library for Kind Clusters

**From**: WorkflowExecution Team
**To**: Data Storage Team (PRIMARY), All Service Teams (CC)
**Date**: December 10, 2025
**Priority**: 🟡 MEDIUM
**Status**: ✅ **IMPLEMENTED**
**Response Deadline**: ~~December 13, 2025~~
**Implementation Schedule**: [DS_E2E_MIGRATION_LIBRARY_IMPLEMENTATION_SCHEDULE.md](./DS_E2E_MIGRATION_LIBRARY_IMPLEMENTATION_SCHEDULE.md)
**Implementation Location**: `test/infrastructure/migrations.go`

---

## ✅ IMPLEMENTATION COMPLETE - December 10, 2025

The Data Storage team has implemented the shared migration library. **All teams can now use it.**

### 📦 New File: `test/infrastructure/migrations.go`

### Quick Start for Each Team

```go
// MOST TEAMS (WE, Gateway, Notification, RO, SP): Just need audit_events
err := infrastructure.ApplyAuditMigrations(ctx, namespace, kubeconfigPath, output)

// AIANALYSIS: Needs audit_events + workflow catalog
config := infrastructure.DefaultMigrationConfig(namespace, kubeconfigPath)
config.Tables = []string{"audit_events", "remediation_workflow_catalog"}
err := infrastructure.ApplyMigrationsWithConfig(ctx, config, output)

// DS: Needs all migrations (already wired up)
err := infrastructure.ApplyAllMigrations(ctx, namespace, kubeconfigPath, output)
```

### Functions Available

| Function | Use Case | Tables Created |
|----------|----------|----------------|
| `ApplyAuditMigrations()` | **Most teams** - audit event emitters | `audit_events`, partitions, indexes |
| `ApplyAllMigrations()` | **DS only** - full schema | All 20+ migrations |
| `ApplyMigrationsWithConfig()` | **Custom** - specific tables | Per config.Tables |
| `VerifyMigrations()` | **Health check** - verify tables exist | (verification only) |

### Tables & Indexes Created by `ApplyAuditMigrations()`

- ✅ `audit_events` - Main unified audit table (ADR-034)
- ✅ `audit_events_y2025m12` - December 2025 partition
- ✅ `audit_events_y2026m01` - January 2026 partition
- ✅ `idx_audit_events_correlation` - Query by correlation_id
- ✅ `idx_audit_events_event_type` - Query by event type
- ✅ `idx_audit_events_timestamp` - Query by time range
- ✅ `idx_audit_events_resource` - Query by resource
- ✅ `idx_audit_events_actor` - Query by actor

### Migration to Shared Library

Replace your inline SQL with the shared function:

```diff
// BEFORE: Inline SQL (DON'T DO THIS)
- createTableSQL := `CREATE TABLE IF NOT EXISTS audit_events (...)`
- cmd := exec.Command("kubectl", "exec", "-i", "-n", namespace, podName, "--", "psql", ...)

// AFTER: Shared library (DO THIS)
+ if err := infrastructure.ApplyAuditMigrations(ctx, namespace, kubeconfigPath, output); err != nil {
+     return fmt.Errorf("failed to apply audit migrations: %w", err)
+ }
```

### Questions?

Contact: Data Storage Team

---

## 🎯 Request to Data Storage Team

**We request the DS team implement a shared migration library** for E2E Kind clusters.

**Rationale**: DS owns the database schema and migrations. All other services are consumers. Having DS own this shared library ensures:
- Schema changes are propagated correctly
- Single source of truth for table definitions
- DS team controls when/how migrations are applied

---

## 📋 Problem Statement

Every service that runs E2E tests in Kind clusters needs the Data Storage `audit_events` table to exist. Currently:

| Service | E2E Infrastructure | Migration Handling |
|---------|-------------------|-------------------|
| **WorkflowExecution** | `test/infrastructure/workflowexecution.go` | ❌ Manual inline SQL |
| **AIAnalysis** | `test/infrastructure/aianalysis.go` | ❌ None (DS fails silently) |
| **Gateway** | `test/infrastructure/gateway.go` | ❌ None |
| **Notification** | `test/infrastructure/notification.go` | ❌ None |
| **RemediationOrchestrator** | `test/infrastructure/remediationorchestrator.go` | ❌ None |
| **DataStorage** | `test/infrastructure/datastorage.go` | ✅ Has migrations |

### Current Issues

1. **Code Duplication**: Each service must copy the same migration SQL
2. **Schema Drift**: If `audit_events` schema changes, every service must update
3. **Maintenance Burden**: N services × M migrations = N×M updates
4. **Silent Failures**: Some E2E tests fail with HTTP 500 due to missing tables

---

## 💡 Proposed Solution

### Create Shared Library: `test/infrastructure/migrations.go`

```go
package infrastructure

// ApplyAuditEventsMigration creates the audit_events table in PostgreSQL
// This is the SINGLE SOURCE OF TRUTH for E2E database schema
// All services call this function instead of embedding SQL
func ApplyAuditEventsMigration(kubeconfigPath, namespace string, output io.Writer) error {
    // ... implementation
}

// ApplyAllMigrations applies all required migrations for E2E tests
// Includes: audit_events, workflows, workflow_versions, etc.
func ApplyAllMigrations(kubeconfigPath, namespace string, output io.Writer) error {
    // ... implementation
}

// MigrationConfig allows customization per service
type MigrationConfig struct {
    Namespace       string
    PostgresService string   // Default: "postgres"
    PostgresUser    string   // Default: "slm_user"
    PostgresDB      string   // Default: "action_history"
    Tables          []string // Specific tables to create (empty = all)
}
```

### Usage in Service Infrastructure

```go
// In test/infrastructure/workflowexecution.go
func CreateWorkflowExecutionCluster(...) error {
    // ... deploy PostgreSQL ...

    // Shared migration function (single source of truth)
    if err := ApplyAuditEventsMigration(kubeconfigPath, namespace, output); err != nil {
        return fmt.Errorf("failed to apply migrations: %w", err)
    }

    // ... deploy DS ...
}
```

---

## 📊 Benefits

| Aspect | Before | After |
|--------|--------|-------|
| **Schema Definition** | Duplicated in each service | Single file: `migrations.go` |
| **Schema Updates** | Update N services | Update 1 file |
| **Consistency** | Risk of drift | Guaranteed same schema |
| **Maintenance** | O(N×M) effort | O(M) effort |
| **Testing** | Each service tests own copy | Shared tests validate once |

---

## 🏗️ Implementation Plan

### Phase 1: Create Shared Library
1. Create `test/infrastructure/migrations.go`
2. Extract SQL from existing implementations
3. Add configuration options for flexibility

### Phase 2: Update Services
1. Replace inline SQL with shared function calls
2. Remove duplicate schema definitions
3. Update E2E test documentation

### Phase 3: Add Schema Versioning (Optional)
1. Track migration versions applied
2. Support incremental migrations
3. Add rollback capability

---

## 📋 Design Decision Required

If approved, we will create:

**DD-E2E-001: Shared E2E Migration Library**

Covering:
- Library location and structure
- Function signatures and configuration
- Error handling and logging
- Testing strategy for the shared library
- Migration from existing inline SQL

---

## ✅ Feedback Requested

Please respond with your team's feedback:

### Questions for Each Team

1. **Do you agree this should be shared?** (Yes/No/Conditional)
2. **What migrations does your service need?** (audit_events, workflows, etc.)
3. **Any concerns about shared implementation?**
4. **Preferred location?** (`test/infrastructure/` vs `pkg/testutil/`)
5. **Additional requirements?**

---

## 📬 Response Format

Please respond in a new file: `docs/handoff/RESPONSE_[SERVICE]_E2E_MIGRATION_LIBRARY.md`

Example:

```markdown
# Response: [Service] Team - E2E Migration Library Proposal

**Team**: [Service Name]
**Date**: [Date]
**Decision**: ✅ APPROVED / ❌ REJECTED / 🟡 CONDITIONAL

## Feedback

1. **Agreement**: [Yes/No/Conditional]
2. **Required Migrations**: [List]
3. **Concerns**: [Any concerns]
4. **Preferred Location**: [Location]
5. **Additional Requirements**: [Any]
```

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [NOTICE_DATASTORAGE_MIGRATION_NOT_AUTO_APPLIED.md](./NOTICE_DATASTORAGE_MIGRATION_NOT_AUTO_APPLIED.md) | Original issue that prompted this |
| [RESPONSE_DATASTORAGE_MIGRATION_FIXED.md](./RESPONSE_DATASTORAGE_MIGRATION_FIXED.md) | DS fix for podman-compose (not Kind) |
| [DD-TEST-001](../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) | E2E port allocation standard |
| [DD-TEST-002](../architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md) | Parallel test execution standard |

---

## 📞 Questions?

Contact: WorkflowExecution Team

---

**Document Version**: 1.0
**Created**: December 10, 2025
**Maintained By**: WorkflowExecution Team

---

## 📊 Response Tracker

| Team | Response | Date | Decision |
|------|----------|------|----------|
| **WorkflowExecution** | [RESPONSE_WE_E2E_MIGRATION_LIBRARY.md](./RESPONSE_WE_E2E_MIGRATION_LIBRARY.md) | 2025-12-10 | ✅ **APPROVED** - BR-WE-005 requires `audit_events` |
| **DataStorage** | [RESPONSE_DS_E2E_MIGRATION_LIBRARY.md](./RESPONSE_DS_E2E_MIGRATION_LIBRARY.md) | 2025-12-10 | ✅ **APPROVED** - Already exists! |
| **Gateway** | [RESPONSE_GATEWAY_E2E_MIGRATION_LIBRARY.md](./RESPONSE_GATEWAY_E2E_MIGRATION_LIBRARY.md) | 2025-12-10 | ✅ APPROVED |
| **AIAnalysis** | [RESPONSE_AIANALYSIS_E2E_MIGRATION_LIBRARY.md](./RESPONSE_AIANALYSIS_E2E_MIGRATION_LIBRARY.md) | 2025-12-10 | ✅ APPROVED |
| **Notification** | [RESPONSE_NOTIFICATION_E2E_MIGRATION_LIBRARY.md](./RESPONSE_NOTIFICATION_E2E_MIGRATION_LIBRARY.md) | 2025-12-10 | ✅ APPROVED |
| **RemediationOrchestrator** | [RESPONSE_RO_E2E_MIGRATION_LIBRARY.md](./RESPONSE_RO_E2E_MIGRATION_LIBRARY.md) | 2025-12-10 | ✅ **APPROVED** |
| **SignalProcessing** | [RESPONSE_SP_E2E_MIGRATION_LIBRARY.md](./RESPONSE_SP_E2E_MIGRATION_LIBRARY.md) | 2025-12-10 | ✅ **APPROVED** (needs `audit_events` for BR-SP-090) |

**Consensus Required**: 4/7 teams must approve
**Current Status**: ✅ **CONSENSUS REACHED** (7/7 approved: WE, DataStorage, Gateway, AIAnalysis, Notification, RO, SP)


