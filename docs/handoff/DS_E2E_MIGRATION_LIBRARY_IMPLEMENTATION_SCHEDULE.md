# 📅 DS Implementation Schedule: Shared E2E Migration Library

**Owner**: Data Storage Team
**Start Date**: December 11, 2025
**Target Completion**: December 12, 2025
**Actual Completion**: ✅ **December 10, 2025** (1 day early!)
**Priority**: 🔴 **HIGH** (WE E2E tests BLOCKED)
**Status**: ✅ **IMPLEMENTED**

---

## ✅ Implementation Complete

**Completed**: December 10, 2025
**File**: `test/infrastructure/migrations.go`
**Build Status**: ✅ Compiles successfully

### What Was Delivered

| Deliverable | Status | Notes |
|-------------|--------|-------|
| `ApplyAuditMigrations()` | ✅ Done | Shortcut for all audit event emitters |
| `ApplyAllMigrations()` | ✅ Done | Full schema (DS only) |
| `ApplyMigrationsWithConfig()` | ✅ Done | Custom table selection |
| `VerifyMigrations()` | ✅ Done | Health check (AIAnalysis request) |
| `MigrationConfig` struct | ✅ Done | Configuration options |
| `AllMigrations` slice | ✅ Done | Metadata for all 20+ migrations |
| DS integration | ✅ Done | `datastorage.go` updated |

### Next Steps for Other Teams

Each team should update their `test/infrastructure/[service].go` file:

```go
// Replace inline SQL with:
if err := infrastructure.ApplyAuditMigrations(ctx, namespace, kubeconfigPath, output); err != nil {
    return fmt.Errorf("failed to apply audit migrations: %w", err)
}
```

See [REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md](./REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md) for full usage docs.

---

## 📊 Consensus Summary

| Team | Approval | Required Tables | Special Requirements |
|------|----------|-----------------|---------------------|
| **WorkflowExecution** | ✅ | `audit_events` + partitions | Indexes for `correlation_id`, `event_type`, `timestamp` |
| DataStorage | ✅ | All (owner) | - |
| Gateway | ✅ | `audit_events` | - |
| AIAnalysis | ✅ | `audit_events`, `workflows`, `workflow_versions` | `VerifyMigrations()` function |
| Notification | ✅ | `audit_events` + partitions | - |
| RO | ✅ | `audit_events` + partitions | - |
| SP | ✅ | `audit_events` + partitions | BR-SP-090 audit trail |

**Consensus**: ✅ **7/7 teams approved**

---

## 🚨 Impact: ALL Teams Need `audit_events`

**Every service that emits audit events requires this table.** Without the shared migration library, E2E tests fail across the board.

| Team | E2E Status | Audit Event Types | Impact Without Library |
|------|------------|-------------------|------------------------|
| **WorkflowExecution** | ❌ **BLOCKED** | `workflowexecution.workflow.*` | `relation "audit_events" does not exist` |
| **AIAnalysis** | ⚠️ 9/51 tests fail | `aianalysis.analysis.*` | Silent DS failures |
| **Gateway** | ⚠️ Degraded | `gateway.signal.*` | HTTP 500 on audit write |
| **Notification** | ⚠️ Degraded | `notification.sent.*` | HTTP 500 on audit write |
| **RO** | ⚠️ Degraded | `orchestrator.remediation.*` | HTTP 500 on audit write |
| **SP** | ⚠️ BR-SP-090 blocked | `signalprocessing.categorization.*` | Audit trail not E2E tested |

### Why WE is Priority (Most Detailed Report)

**Test File**: `test/e2e/workflowexecution/02_observability_test.go:269`

```go
Context("BR-WE-005: Audit Persistence in PostgreSQL (E2E)", Label("datastorage", "audit"), func() {
    It("should persist audit events to Data Storage for completed workflow", func() {
        // Queries DS for audit events by correlation_id
        auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s",
            dataStorageServiceURL, wfe.Name)

        // FAILS with HTTP 500 if audit_events table doesn't exist
        Eventually(func() int {
            resp, err := http.Get(auditQueryURL)
            // ...
        }, 60*time.Second).Should(BeNumerically(">=", 2))
    })
})
```

**Error**: `ERROR: relation "audit_events" does not exist (SQLSTATE 42P01)`

---

## 🎯 Deliverables

### D1: `test/infrastructure/migrations.go`

New shared library with:
- `ApplyMigrations()` - selective migration API
- `ApplyAuditMigrations()` - **audit-specific shortcut (ALL 6 consumer teams need this)**
- `ApplyAllMigrations()` - everything (DS + AIAnalysis)
- `VerifyMigrations()` - health check (requested by AIAnalysis)
- `MigrationConfig` - configuration struct
- `Migration` - metadata struct with tables + indexes

### D2: Update ALL Service Infrastructure Files

Update each service's infrastructure file to use shared library:

| File | Team | Status | Audit Events Emitted |
|------|------|--------|---------------------|
| `workflowexecution.go` | WE | 🔴 BLOCKED | `workflowexecution.workflow.*` |
| `gateway.go` | Gateway | ⚠️ Degraded | `gateway.signal.*` |
| `aianalysis.go` | AIAnalysis | ⚠️ 9/51 fail | `aianalysis.analysis.*` |
| `notification.go` | Notification | ⚠️ Degraded | `notification.sent.*` |
| `signalprocessing.go` | SP | ⚠️ BR-SP-090 | `signalprocessing.categorization.*` |

### D3: Documentation

- Update `RESPONSE_DS_E2E_MIGRATION_LIBRARY.md` with final API
- Create usage examples for each team

---

## 📅 Schedule

### Day 1: December 11, 2025 (3-4 hours)

#### Phase 1: Create Shared Library (2 hours)

| Task | Duration | Status |
|------|----------|--------|
| Create `test/infrastructure/migrations.go` | 30 min | ✅ Done |
| Define `MigrationConfig` and `Migration` structs | 15 min | ✅ Done |
| Extract migration list from `datastorage.go` | 30 min | ✅ Done |
| Implement `ApplyMigrations()` with selective API | 30 min | ✅ Done |
| Implement `ApplyAuditMigrations()` shortcut | 15 min | ✅ Done |
| Add `VerifyMigrations()` function | 15 min | ✅ Done |

#### Phase 2: Test with DS E2E (1 hour)

| Task | Duration | Status |
|------|----------|--------|
| Run `make test-e2e-datastorage` with new library | 30 min | ✅ Done |
| Fix any issues | 30 min | ✅ Done |

---

### API Design (Complete)

```go
package infrastructure

import (
    "context"
    "io"
)

// MigrationConfig configures which migrations to apply
type MigrationConfig struct {
    Namespace       string
    KubeconfigPath  string
    PostgresService string   // Default: "postgresql"
    PostgresUser    string   // Default: "slm_user"
    PostgresDB      string   // Default: "action_history"
    Tables          []string // Empty = all tables
}

// DefaultMigrationConfig returns sensible defaults
func DefaultMigrationConfig(namespace, kubeconfigPath string) MigrationConfig {
    return MigrationConfig{
        Namespace:       namespace,
        KubeconfigPath:  kubeconfigPath,
        PostgresService: "postgresql",
        PostgresUser:    "slm_user",
        PostgresDB:      "action_history",
        Tables:          nil, // All tables
    }
}

// Migration represents a single migration file with metadata
type Migration struct {
    Name        string   // Human-readable name
    File        string   // Migration file name
    Description string   // What this migration does
    Tables      []string // Tables created by this migration
    Indexes     []string // Indexes created by this migration
}

// AvailableMigrations lists all migrations with metadata
// Order matters - migrations are applied in this sequence
var AvailableMigrations = []Migration{
    // Core schema
    {
        Name:        "initial_schema",
        File:        "001_initial_schema.sql",
        Description: "Initial database schema",
        Tables:      []string{"notification_audit", "resource_action_traces"},
    },
    // ... other core migrations ...

    // ADR-033: Multi-dimensional tracking
    {
        Name:        "adr033_tracking",
        File:        "012_adr033_multidimensional_tracking.sql",
        Description: "ADR-033 multi-dimensional success tracking",
        Tables:      []string{},
        Indexes:     []string{"idx_incident_type_success", "idx_workflow_success"},
    },

    // ADR-034: Unified audit events (REQUIRED BY ALL TEAMS)
    {
        Name:        "audit_events",
        File:        "013_create_audit_events_table.sql",
        Description: "Unified audit events table (ADR-034)",
        Tables:      []string{"audit_events"},
        Indexes: []string{
            "idx_audit_events_correlation",  // WE: Query by correlation_id
            "idx_audit_events_event_type",   // WE: Query by event type
            "idx_audit_events_timestamp",    // WE: Query by time range
        },
    },

    // Audit event partitions (REQUIRED BY WE, SP, RO, NOTIFICATION)
    {
        Name:        "audit_partitions",
        File:        "1000_create_audit_events_partitions.sql",
        Description: "Monthly partitions for audit_events",
        Tables: []string{
            "audit_events_y2025m12",  // Current month
            "audit_events_y2026m01",  // Next month
        },
    },

    // Workflow catalog (REQUIRED BY AIANALYSIS)
    {
        Name:        "workflow_catalog",
        File:        "015_create_workflow_catalog_table.sql",
        Description: "Workflow catalog for semantic search",
        Tables:      []string{"remediation_workflow_catalog"},
    },
}

// ApplyMigrations applies selected migrations based on config
// If config.Tables is empty, applies all migrations
// If config.Tables is specified, applies only migrations that create those tables
func ApplyMigrations(ctx context.Context, config MigrationConfig, writer io.Writer) error

// ApplyAuditMigrations is a shortcut for audit-only migrations
// Applies: audit_events + audit_partitions (what most teams need)
// This is equivalent to:
//   config.Tables = []string{"audit_events", "audit_events_y2025m12", "audit_events_y2026m01"}
func ApplyAuditMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error

// ApplyAllMigrations applies all available migrations
// Use this for DS E2E tests that need the complete schema
func ApplyAllMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error

// VerifyMigrations checks if required tables exist
// Returns nil if all tables in config.Tables exist, error otherwise
// If config.Tables is empty, verifies all tables from AvailableMigrations
func VerifyMigrations(ctx context.Context, config MigrationConfig, writer io.Writer) error
```

---

### Day 2: December 12, 2025 (2-3 hours)

#### Phase 1: Update WE Infrastructure (PRIORITY - UNBLOCKED)

| Task | Duration | Status | Priority |
|------|----------|--------|----------|
| Update `workflowexecution.go` to use shared library | 30 min | 🟢 Ready | 🔴 HIGH |
| Test WE E2E: `make test-e2e-workflowexecution` | 30 min | 🟢 Ready | 🔴 HIGH |
| Notify WE team: "Unblocked!" | 5 min | 🟢 Ready | 🔴 HIGH |

**WE Integration Code**:

```go
// In test/infrastructure/workflowexecution.go
func CreateWorkflowExecutionCluster(ctx context.Context, ...) error {
    // ... deploy PostgreSQL, wait for ready ...

    // Apply audit migrations using shared library
    // This creates: audit_events + partitions + indexes
    if err := ApplyAuditMigrations(ctx, namespace, kubeconfigPath, output); err != nil {
        return fmt.Errorf("failed to apply audit migrations: %w", err)
    }

    // Verify tables exist
    config := DefaultMigrationConfig(namespace, kubeconfigPath)
    config.Tables = []string{"audit_events", "audit_events_y2025m12"}
    if err := VerifyMigrations(ctx, config, output); err != nil {
        return fmt.Errorf("migration verification failed: %w", err)
    }

    // ... deploy DS ...
}
```

#### Phase 2: Update Other Services

| Task | Duration | Status |
|------|----------|--------|
| Update `gateway.go` | 20 min | ⏳ |
| Update `aianalysis.go` | 20 min | ⏳ |
| Update `notification.go` | 20 min | ⏳ |
| Update `signalprocessing.go` | 20 min | ⏳ |

#### Phase 3: Documentation & Notification

| Task | Duration | Status |
|------|----------|--------|
| Update `RESPONSE_DS_E2E_MIGRATION_LIBRARY.md` | 15 min | ⏳ |
| Notify all teams: "Ready for integration" | 10 min | ⏳ |

---

## ✅ Acceptance Criteria

### AC0: All Teams Can Use `ApplyAuditMigrations()` (CRITICAL)

```go
// This is what ALL 6 consumer teams need:
// - WorkflowExecution (BLOCKED)
// - Gateway (degraded)
// - AIAnalysis (9/51 fail)
// - Notification (degraded)
// - RO (degraded)
// - SP (BR-SP-090 blocked)

err := infrastructure.ApplyAuditMigrations(ctx, namespace, kubeconfigPath, output)
// Expected: No error
// Creates: audit_events + partitions + indexes
```

### AC1: WE E2E Tests Unblocked (PRIORITY - Most Detailed Report)

```bash
# WE E2E tests must pass
make test-e2e-workflowexecution

# Verify audit_events table exists in Kind cluster
kubectl exec -n <namespace> postgresql-0 -- psql -U slm_user -d action_history \
  -c "SELECT COUNT(*) FROM audit_events;"
# Expected: 0 (empty but table exists)
```

### AC2: WE Indexes Created

```bash
# Verify indexes exist (required by WE for query performance)
kubectl exec -n <namespace> postgresql-0 -- psql -U slm_user -d action_history \
  -c "\di idx_audit_events_*"
# Expected:
#  idx_audit_events_correlation
#  idx_audit_events_event_type
#  idx_audit_events_timestamp
```

### AC3: Partitions Created

```bash
# Verify partitions exist (required by WE, SP, RO, Notification)
kubectl exec -n <namespace> postgresql-0 -- psql -U slm_user -d action_history \
  -c "\dt audit_events_*"
# Expected:
#  audit_events (partitioned)
#  audit_events_y2025m12
#  audit_events_y2026m01
```

### AC4: ApplyAuditMigrations() Works

```go
// All teams can use this shortcut
err := infrastructure.ApplyAuditMigrations(ctx, namespace, kubeconfigPath, output)
// Expected: No error, audit_events + partitions + indexes created
```

### AC5: VerifyMigrations() Works

```go
// AIAnalysis requested this
config := infrastructure.DefaultMigrationConfig(namespace, kubeconfigPath)
err := infrastructure.VerifyMigrations(ctx, config, output)
// Expected: Returns nil if all tables exist, descriptive error otherwise
```

### AC6: AIAnalysis Full Migrations

```go
// AIAnalysis needs workflows + workflow_versions too
config := infrastructure.MigrationConfig{
    Namespace:      namespace,
    KubeconfigPath: kubeconfigPath,
    Tables:         []string{"audit_events", "remediation_workflow_catalog"},
}
err := infrastructure.ApplyMigrations(ctx, config, output)
// Expected: No error, specified tables created
```

---

## 📋 Dependencies

| Dependency | Status |
|------------|--------|
| Existing `datastorage.go` migrations | ✅ Available |
| Team consensus | ✅ **7/7 approved** |
| DS E2E tests working | ✅ Verified (174 integration specs) |
| Migration files in `migrations/` | ✅ Available |

---

## 🔄 Risks & Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Breaking existing DS E2E tests | Low | High | Test thoroughly before refactoring |
| Migration ordering issues | Low | Medium | Use existing proven order from `datastorage.go` |
| Index creation fails | Low | Medium | Use `CREATE INDEX IF NOT EXISTS` |
| Cross-team integration issues | Medium | Medium | Provide clear examples + buffer day |

---

## 📞 Communication Plan

| When | Action | Recipients |
|------|--------|------------|
| Start of Day 1 | Update request tracker status to "In Progress" | All teams |
| End of Day 1 | "Shared library available for review" | All teams |
| Day 2 Morning | "WE unblocked - `ApplyAuditMigrations()` ready" | **WE team** |
| End of Day 2 | "Ready for integration" | All teams |

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md](./REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md) | Original request |
| [RESPONSE_DS_E2E_MIGRATION_LIBRARY.md](./RESPONSE_DS_E2E_MIGRATION_LIBRARY.md) | DS approval |
| [RESPONSE_WE_E2E_MIGRATION_LIBRARY.md](./RESPONSE_WE_E2E_MIGRATION_LIBRARY.md) | WE requirements (BLOCKING) |
| [test/infrastructure/datastorage.go](../../test/infrastructure/datastorage.go) | Existing implementation |

---

## 📊 Summary

| Metric | Value |
|--------|-------|
| **Total Effort** | 5-6 hours |
| **Timeline** | 2 days |
| **Teams Unblocked** | **ALL 6 consumer teams** (WE, AIAnalysis, Gateway, Notification, RO, SP) |
| **Files Created** | 1 (`migrations.go`) |
| **Files Updated** | 5 (service infrastructure files) |

### Business Impact

| Before (No Shared Library) | After (Shared Library) |
|---------------------------|------------------------|
| ❌ WE E2E tests BLOCKED | ✅ All E2E tests pass |
| ❌ AIAnalysis 9/51 tests fail | ✅ All tests pass |
| ❌ HTTP 500 on audit operations | ✅ Audit events persist |
| ❌ BR-SP-090 not E2E tested | ✅ Audit trail validated |
| ❌ Schema duplicated in N services | ✅ Single source of truth |
| ❌ O(N×M) maintenance burden | ✅ O(M) maintenance |

---

**Document Version**: 2.0
**Created**: December 10, 2025
**Updated**: December 10, 2025 (Added WE requirements, elevated priority)
**Owner**: Data Storage Team
