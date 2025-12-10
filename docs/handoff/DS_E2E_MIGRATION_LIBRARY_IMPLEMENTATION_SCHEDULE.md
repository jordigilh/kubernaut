# 📅 DS Implementation Schedule: Shared E2E Migration Library

**Owner**: Data Storage Team
**Start Date**: December 11, 2025
**Target Completion**: December 13, 2025
**Priority**: 🟡 MEDIUM
**Status**: 📋 **PLANNED**

---

## 📊 Consensus Summary

| Team | Approval | Required Tables |
|------|----------|-----------------|
| DataStorage | ✅ | All (owner) |
| Gateway | ✅ | `audit_events` |
| AIAnalysis | ✅ | `audit_events`, `workflows`, `workflow_versions` |
| Notification | ✅ | `audit_events` + partitions |
| RO | ✅ | `audit_events` + partitions |
| SP | ✅ | `audit_events` + partitions |

**Consensus**: ✅ **6/6 teams approved**

---

## 🎯 Deliverables

### D1: `test/infrastructure/migrations.go`

New shared library with:
- `ApplyMigrations()` - selective migration API
- `ApplyAuditMigrations()` - audit-specific shortcut
- `ApplyAllMigrations()` - everything
- `VerifyMigrations()` - health check
- `MigrationConfig` - configuration struct

### D2: Update Service Infrastructure Files

Update each service's infrastructure file to use shared library:
- `test/infrastructure/gateway.go`
- `test/infrastructure/aianalysis.go`
- `test/infrastructure/notification.go`
- `test/infrastructure/signalprocessing.go`
- `test/infrastructure/workflowexecution.go`

### D3: Documentation

- Update `RESPONSE_DS_E2E_MIGRATION_LIBRARY.md` with final API
- Create usage examples for each team

---

## 📅 Schedule

### Day 1: December 11, 2025 (2-3 hours)

#### Morning: Create Shared Library

| Task | Duration | Status |
|------|----------|--------|
| Create `test/infrastructure/migrations.go` | 1 hour | ⏳ |
| Extract migration logic from `datastorage.go` | 30 min | ⏳ |
| Implement `MigrationConfig` struct | 15 min | ⏳ |
| Implement selective migration API | 30 min | ⏳ |
| Add `VerifyMigrations()` function | 15 min | ⏳ |

**API Design**:

```go
package infrastructure

// MigrationConfig configures which migrations to apply
type MigrationConfig struct {
    Namespace       string
    KubeconfigPath  string
    PostgresService string   // Default: "postgresql"
    PostgresUser    string   // Default: "slm_user"
    PostgresDB      string   // Default: "action_history"
    Tables          []string // Empty = all tables
}

// Migration represents a single migration
type Migration struct {
    Name        string
    File        string
    Description string
    Tables      []string // Tables created by this migration
}

// AvailableMigrations lists all migrations with metadata
var AvailableMigrations = []Migration{
    {Name: "audit_events", File: "013_create_audit_events_table.sql", Tables: []string{"audit_events"}},
    {Name: "audit_partitions", File: "1000_create_audit_events_partitions.sql", Tables: []string{"audit_events_*"}},
    {Name: "workflows", File: "015_create_workflow_catalog_table.sql", Tables: []string{"remediation_workflow_catalog"}},
    // ... etc
}

// ApplyMigrations applies selected migrations
func ApplyMigrations(ctx context.Context, config MigrationConfig, writer io.Writer) error

// ApplyAuditMigrations is a shortcut for audit-only migrations
func ApplyAuditMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error

// ApplyAllMigrations applies all available migrations
func ApplyAllMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error

// VerifyMigrations checks if required tables exist
func VerifyMigrations(ctx context.Context, config MigrationConfig, writer io.Writer) error
```

### Day 2: December 12, 2025 (2-3 hours)

#### Morning: Update Service Infrastructure

| Task | Duration | Status |
|------|----------|--------|
| Update `gateway.go` to use shared library | 30 min | ⏳ |
| Update `aianalysis.go` to use shared library | 30 min | ⏳ |
| Update `notification.go` to use shared library | 30 min | ⏳ |
| Update `signalprocessing.go` to use shared library | 30 min | ⏳ |
| Update `workflowexecution.go` to use shared library | 30 min | ⏳ |

#### Afternoon: Testing & Documentation

| Task | Duration | Status |
|------|----------|--------|
| Test shared library with DS E2E tests | 30 min | ⏳ |
| Update documentation | 30 min | ⏳ |
| Notify all teams of completion | 15 min | ⏳ |

### Day 3: December 13, 2025 (Buffer)

| Task | Duration | Status |
|------|----------|--------|
| Address any team feedback | As needed | ⏳ |
| Fix any issues found during team testing | As needed | ⏳ |

---

## ✅ Acceptance Criteria

### AC1: Shared Library Works

```bash
# DS E2E tests pass with new library
make test-e2e-datastorage
# Expected: All tests pass
```

### AC2: Gateway Can Use It

```go
// In gateway.go
err := infrastructure.ApplyAuditMigrations(ctx, namespace, kubeconfigPath, output)
// Expected: No error, audit_events table exists
```

### AC3: AIAnalysis Can Use Full Migrations

```go
// In aianalysis.go
config := infrastructure.MigrationConfig{
    Namespace:      namespace,
    KubeconfigPath: kubeconfigPath,
    Tables:         []string{"audit_events", "workflows", "workflow_versions"},
}
err := infrastructure.ApplyMigrations(ctx, config, output)
// Expected: No error, all specified tables exist
```

### AC4: Verification Works

```go
err := infrastructure.VerifyMigrations(ctx, config, output)
// Expected: Returns nil if all tables exist, error otherwise
```

---

## 📋 Dependencies

| Dependency | Status |
|------------|--------|
| Existing `datastorage.go` migrations | ✅ Available |
| Team consensus | ✅ 6/6 approved |
| DS E2E tests working | ✅ Verified (174 integration specs) |

---

## 🔄 Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking existing DS E2E tests | Test thoroughly before refactoring |
| Migration ordering issues | Use existing proven order from `datastorage.go` |
| Cross-team integration issues | Provide clear examples + buffer day |

---

## 📞 Communication Plan

| When | Action |
|------|--------|
| Start of Day 1 | Update request tracker status to "In Progress" |
| End of Day 1 | Notify teams: "Shared library available for review" |
| End of Day 2 | Notify teams: "Ready for integration" |
| End of Day 3 | Close request: "Implementation complete" |

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md](./REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md) | Original request |
| [RESPONSE_DS_E2E_MIGRATION_LIBRARY.md](./RESPONSE_DS_E2E_MIGRATION_LIBRARY.md) | DS approval |
| [test/infrastructure/datastorage.go](../../test/infrastructure/datastorage.go) | Existing implementation |

---

**Document Version**: 1.0
**Created**: December 10, 2025
**Owner**: Data Storage Team

