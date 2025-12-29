# Response: Gateway Team - E2E Migration Library Proposal

**Team**: Gateway
**Date**: December 10, 2025
**Decision**: ✅ **APPROVED**

---

## Feedback

### 1. Agreement: **Yes**

We agree this should be a shared library. The current situation (duplicated SQL in each service) violates DRY and creates maintenance burden.

### 2. Required Migrations

| Migration | Required | Reason |
|-----------|----------|--------|
| `audit_events` | ✅ Yes | Gateway emits audit events (DD-AUDIT-003) |
| `workflows` | ❌ No | Gateway doesn't interact with workflow tables |
| `workflow_versions` | ❌ No | Gateway doesn't interact with workflow tables |

### 3. Concerns

| Concern | Mitigation |
|---------|------------|
| **Schema ownership** | DS team MUST own the shared library since they own the schema |
| **Selective migration** | Support applying only specific tables (not all) |
| **Migration ordering** | Ensure idempotent migrations (CREATE TABLE IF NOT EXISTS) |

### 4. Preferred Location

**`test/infrastructure/migrations.go`** ✅

Rationale:
- Already a shared package
- Follows existing patterns
- All service infrastructure files are there

### 5. Additional Requirements

#### 5.1 Selective Migration API

```go
// Allow services to specify which tables they need
config := MigrationConfig{
    Namespace: "kubernaut-system",
    Tables:    []string{"audit_events"}, // Gateway only needs this
}
if err := ApplyMigrations(kubeconfigPath, config, output); err != nil {
    return err
}
```

#### 5.2 Idempotent Migrations

All migrations should use `IF NOT EXISTS` / `CREATE OR REPLACE` patterns:
```sql
CREATE TABLE IF NOT EXISTS audit_events (...)
```

#### 5.3 Error Reporting

Clear error messages if migration fails:
```go
return fmt.Errorf("migration '%s' failed: %w", tableName, err)
```

---

## Gateway Current E2E Status

| Component | Status |
|-----------|--------|
| **E2E Test Suite** | `test/e2e/gateway/` |
| **Infrastructure** | `test/infrastructure/gateway.go` |
| **Redis** | ❌ REMOVED (DD-GATEWAY-012) |
| **PostgreSQL** | ⚠️ Needed for audit via Data Storage |
| **Audit Events** | 🔄 TDD RED (awaiting DS batch endpoint - now available) |

---

## Implementation Impact for Gateway

### Phase 2: Update Gateway Infrastructure

```go
// In test/infrastructure/gateway.go
func CreateGatewayTestCluster(ctx context.Context, ...) error {
    // ... existing setup ...

    // NEW: Apply required migrations
    migrationConfig := MigrationConfig{
        Namespace: namespace,
        Tables:    []string{"audit_events"}, // Gateway only
    }
    if err := ApplyMigrations(kubeconfigPath, migrationConfig, output); err != nil {
        return fmt.Errorf("gateway: failed to apply migrations: %w", err)
    }

    // ... deploy services ...
}
```

---

## Questions for DS Team

1. **Timeline**: When can we expect the shared library?
2. **Testing**: Will DS add unit tests for the migration functions?
3. **Versioning**: How will you handle schema version conflicts?

---

## Approval

**Gateway Team approves this proposal** contingent on:
1. DS team owns and maintains the shared library
2. Selective migration support is included
3. Migrations are idempotent

---

**Document Version**: 1.0
**Created**: December 10, 2025
**Author**: Gateway Team

