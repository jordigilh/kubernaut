# ✅ Response: Platform Team - E2E Migration Library Proposal

**Team**: Platform/Infrastructure
**Date**: December 10, 2025
**Decision**: ✅ **APPROVED with Recommendations**

---

## Feedback

### 1. Agreement: **YES** - This is a Valid Problem

**Evidence from Codebase**:

| Service | Migration Handling | Lines of Code |
|---------|-------------------|---------------|
| **WorkflowExecution** | ❌ 75+ lines inline SQL | `workflowexecution.go:540-612` |
| **DataStorage** | ✅ Loads from `migrations/` | Clean approach |
| **AIAnalysis** | ❌ None | Relies on DS being deployed |
| **Gateway** | ❌ None | Relies on DS being deployed |
| **Notification** | ❌ None | Relies on DS being deployed |
| **SignalProcessing** | ❌ None | No DS dependency |

**Risk Confirmed**: WE's inline SQL duplicates DS's schema. Any schema change (e.g., ADR-038 batch endpoint) requires updating both places.

### 2. Required Migrations

For E2E Kind clusters, these tables are needed:

| Table | Used By | Migration Source |
|-------|---------|------------------|
| `audit_events` | WE, AA, Gateway, Notification | `013_create_audit_events_table.sql` |
| `audit_events_*` partitions | All audit consumers | `1000_create_audit_events_partitions.sql` |
| `remediation_workflow_catalog` | DS, WE | `015_create_workflow_catalog_table.sql` |
| `notification_audit` | Notification | (verify exists) |

### 3. Concerns

**Minor**:
- Partition dates are hardcoded in WE (2025m12, 2026m01, 2026m02) - will need updating in 2026
- Function should auto-generate partition names based on current date

**None Blocking**: The proposal is sound.

### 4. Preferred Location: **`test/infrastructure/migrations.go`**

**Rationale**:
- Keeps E2E infrastructure together
- `pkg/testutil/` is for unit test utilities, not E2E
- Consistent with existing pattern (`test/infrastructure/`)

### 5. Additional Requirements

**Recommended Enhancements**:

```go
// Dynamic partition generation (avoid hardcoded dates)
func generatePartitionSQL(tableName string, monthsAhead int) string {
    // Generate partitions from current month + monthsAhead
}

// Service-specific migration sets
type MigrationSet string
const (
    MigrationSetAudit    MigrationSet = "audit"    // audit_events + partitions
    MigrationSetWorkflow MigrationSet = "workflow" // workflow_catalog + versions
    MigrationSetAll      MigrationSet = "all"      // everything
)

func ApplyMigrations(kubeconfigPath, namespace string, sets []MigrationSet) error
```

---

## 📊 Implementation Recommendation

### Option A: Extract from DS (Recommended)

DS already has clean migration handling via file loading. The shared library should:

1. **Read migration files** from `migrations/` directory (same as DS does)
2. **Execute via kubectl** to PostgreSQL pod in Kind cluster
3. **Export functions** for other services to call

```go
// In test/infrastructure/migrations.go
func ApplyMigrations(kubeconfigPath, namespace string, migrations []string, output io.Writer) error {
    for _, migration := range migrations {
        content, err := os.ReadFile(filepath.Join("migrations", migration))
        if err != nil {
            return fmt.Errorf("failed to read migration %s: %w", migration, err)
        }
        if err := executeMigrationSQL(kubeconfigPath, namespace, string(content), output); err != nil {
            return err
        }
    }
    return nil
}

// Predefined migration sets
var AuditMigrations = []string{
    "013_create_audit_events_table.sql",
    "1000_create_audit_events_partitions.sql",
}

var WorkflowMigrations = []string{
    "015_create_workflow_catalog_table.sql",
}
```

### Then Update WE

Replace 75 lines of inline SQL with:

```go
// Before (workflowexecution.go:540-612)
// 75 lines of inline SQL...

// After
if err := ApplyMigrations(kubeconfigPath, namespace, AuditMigrations, output); err != nil {
    return fmt.Errorf("failed to apply audit migrations: %w", err)
}
```

---

## ✅ Summary

| Question | Answer |
|----------|--------|
| Should this be shared? | **YES** |
| Owner | **Data Storage Team** (they own the schema) |
| Location | `test/infrastructure/migrations.go` |
| Approach | Load from existing `migrations/` files |
| Priority | 🟡 MEDIUM (prevents future drift) |

---

**Document Version**: 1.0
**Created**: December 10, 2025
**Maintained By**: Platform/Infrastructure Team

