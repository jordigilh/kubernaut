# ✅ Response: Data Storage Team - E2E Migration Library Proposal

**Team**: Data Storage
**Date**: December 10, 2025
**Decision**: ✅ **APPROVED**

---

## 📋 Feedback

### 1. Agreement: ✅ YES

We agree this should be shared. Having a single source of truth for E2E migrations prevents schema drift and reduces maintenance burden.

### 2. Required Migrations

The Data Storage service already maintains all required migrations in `test/infrastructure/datastorage.go`:

| Migration | Purpose |
|-----------|---------|
| `001_initial_schema.sql` | Initial schema |
| `002_fix_partitioning.sql` | Partitioning fixes |
| `003_stored_procedures.sql` | Stored procedures |
| `004-008` | Various schema updates |
| `009_update_vector_dimensions.sql` | Vector dimensions |
| `010_audit_write_api_phase1.sql` | Audit write API |
| `011_rename_alert_to_signal.sql` | Alert→Signal rename |
| `012_adr033_multidimensional_tracking.sql` | ADR-033 tracking |
| `013_create_audit_events_table.sql` | **audit_events table** |
| `015_create_workflow_catalog_table.sql` | Workflow catalog |
| `016-020` | Additional schema updates |
| `1000_create_audit_events_partitions.sql` | Partitions |

### 3. Concerns: NONE

We already have the implementation. This is a matter of documentation and ensuring other services know how to use it.

### 4. Preferred Location: `test/infrastructure/`

Keep migrations alongside other E2E infrastructure code. The existing `datastorage.go` already has:

```go
// ApplyMigrations is an exported wrapper for applying migrations to a namespace.
// This is useful for re-applying migrations after PostgreSQL restarts (e.g., in DLQ tests).
func ApplyMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    return applyMigrationsInNamespace(ctx, namespace, kubeconfigPath, writer)
}
```

### 5. Additional Requirements: NONE

---

## 🔧 Implementation: ALREADY DONE

**Good news**: The shared migration library **already exists** in `test/infrastructure/datastorage.go`.

### How Other Services Should Use It

```go
// In test/infrastructure/workflowexecution.go (or any other service)
import (
    "github.com/jordigilh/kubernaut/test/infrastructure"
)

func CreateWorkflowExecutionCluster(...) error {
    // ... deploy PostgreSQL ...

    // Apply migrations using DS team's shared function
    if err := infrastructure.ApplyMigrations(ctx, namespace, kubeconfigPath, output); err != nil {
        return fmt.Errorf("failed to apply migrations: %w", err)
    }

    // ... deploy Data Storage service ...
}
```

### Function Signature

```go
// ApplyMigrations applies all Data Storage migrations to a namespace
//
// Parameters:
//   - ctx: Context for cancellation
//   - namespace: Kubernetes namespace where PostgreSQL is running
//   - kubeconfigPath: Path to kubeconfig for kubectl commands
//   - writer: Output writer for progress logging
//
// Prerequisites:
//   - PostgreSQL pod with label "app=postgresql" must be running in namespace
//   - PostgreSQL must be accessible via service "postgresql.{namespace}.svc.cluster.local"
//
// Tables Created:
//   - notification_audit
//   - audit_events (partitioned)
//   - remediation_workflow_catalog
//   - resource_action_traces
//   - effectiveness_assessments
//
func ApplyMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error
```

---

## ✅ Action Items

| Item | Owner | Status |
|------|-------|--------|
| Shared migration function | Data Storage | ✅ **ALREADY EXISTS** |
| Update other services to use it | Each service team | ⏳ Pending |
| Update documentation | Data Storage | ✅ Done (this document) |

---

## 📊 Summary

**No new implementation needed.** The WorkflowExecution team and other services should:

1. Import `github.com/jordigilh/kubernaut/test/infrastructure`
2. Call `infrastructure.ApplyMigrations(ctx, namespace, kubeconfigPath, writer)`
3. Remove any inline SQL migration code

---

**Document Version**: 1.0
**Created**: December 10, 2025
**Maintained By**: Data Storage Team


