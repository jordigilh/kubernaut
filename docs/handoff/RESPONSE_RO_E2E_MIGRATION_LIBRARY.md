# Response: Remediation Orchestrator Team - E2E Migration Library Proposal

**Team**: Remediation Orchestrator
**Date**: December 10, 2025
**Decision**: ✅ **APPROVED**

---

## 📋 Feedback

### 1. Agreement: **YES**

The RO team **strongly supports** a shared E2E migration library.

**Rationale**:
- RO's E2E tests will need `audit_events` table for audit integration testing (BR-ORCH audit trail)
- RO doesn't currently have a `test/infrastructure/remediationorchestrator.go` file, but will need one
- Centralizing migrations prevents schema drift as RO expands E2E coverage

### 2. Required Migrations

| Table | Required? | BR Reference |
|-------|-----------|--------------|
| `audit_events` | ✅ **YES** | BR-ORCH audit trail integration |
| `workflows` | ⚠️ Maybe | Only if RO E2E tests need to verify workflow state |
| `workflow_versions` | ⚠️ Maybe | Same as above |

**Primary Need**: `audit_events` - RO emits audit events for phase transitions, completion, failures, and approval workflows.

### 3. Concerns

| Concern | Risk | Mitigation |
|---------|------|------------|
| **Idempotency** | Medium | Use `CREATE TABLE IF NOT EXISTS` pattern |
| **Race Conditions** | Low | Add mutex or PostgreSQL advisory locks for parallel service deployments |
| **Error Messages** | Medium | Clear errors if PostgreSQL not available (not just timeout) |
| **Timeouts** | Low | Configurable timeout for migration execution |

### 4. Preferred Location

**`test/infrastructure/migrations.go`** ✅

**Rationale**:
- E2E-specific code should stay in `test/infrastructure/`
- Separate from `pkg/testutil/` which is for unit/integration test helpers
- Consistent with existing infrastructure files (`workflowexecution.go`, `aianalysis.go`, etc.)

### 5. Additional Requirements

#### 5.1 Function Signature Suggestion

```go
// ApplyMigrations applies database migrations with service-specific configuration.
// Returns nil if all migrations succeed, error with context if any fails.
func ApplyMigrations(ctx context.Context, config MigrationConfig) error {
    // Context allows timeout control
    // Config allows service-specific customization
}

type MigrationConfig struct {
    KubeconfigPath  string        // Required
    Namespace       string        // Required
    PostgresService string        // Default: "postgres"
    PostgresUser    string        // Default: "slm_user"
    PostgresDB      string        // Default: "action_history"
    Tables          []string      // Empty = all tables
    Timeout         time.Duration // Default: 30s
    Logger          io.Writer     // Default: GinkgoWriter
}
```

#### 5.2 Error Handling

```go
// Specific error types for better handling
var (
    ErrPostgresNotReady = errors.New("postgres not ready")
    ErrMigrationFailed  = errors.New("migration failed")
    ErrTableExists      = errors.New("table already exists") // Not an error if idempotent
)
```

#### 5.3 Idempotent Pattern

```go
// All migrations MUST be idempotent
const createAuditEventsSQL = `
CREATE TABLE IF NOT EXISTS audit_events (
    id SERIAL PRIMARY KEY,
    ...
);
CREATE INDEX IF NOT EXISTS idx_audit_events_timestamp ON audit_events(timestamp);
`
```

#### 5.4 RO Future Infrastructure File

When RO creates `test/infrastructure/remediationorchestrator.go`, it will use:

```go
func CreateRemediationOrchestratorCluster(kubeconfigPath string, output io.Writer) error {
    // ... deploy PostgreSQL ...

    // Use shared migration function
    if err := ApplyMigrations(ctx, MigrationConfig{
        KubeconfigPath: kubeconfigPath,
        Namespace:      "kubernaut-system",
        Tables:         []string{"audit_events"}, // RO only needs audit
        Timeout:        30 * time.Second,
        Logger:         output,
    }); err != nil {
        return fmt.Errorf("failed to apply migrations: %w", err)
    }

    // ... deploy RO controller ...
}
```

---

## ✅ Summary

| Question | RO Response |
|----------|-------------|
| Agreement | ✅ **YES** |
| Required Migrations | `audit_events` (primary), `workflows`/`workflow_versions` (optional) |
| Concerns | Idempotency, race conditions, error handling |
| Preferred Location | `test/infrastructure/migrations.go` |
| Additional Requirements | Context-based timeout, specific error types, idempotent SQL |

---

## 🔗 Related RO Documents

| Document | Relevance |
|----------|-----------|
| `test/e2e/remediationorchestrator/suite_test.go` | Will use shared migrations |
| `docs/audits/v1.0-implementation-triage/REMEDIATIONORCHESTRATOR_TRIAGE.md` | E2E audit requirements |
| `pkg/remediationorchestrator/audit/helpers.go` | Audit events that need `audit_events` table |

---

**Document Version**: 1.0
**Created**: December 10, 2025
**Maintained By**: Remediation Orchestrator Team


