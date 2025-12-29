# Response: Notification Team - E2E Migration Library Proposal

**Team**: Notification Controller
**Date**: December 10, 2025
**Decision**: ✅ **APPROVED**

---

## 📋 Feedback

### 1. Agreement: ✅ YES

We fully support the shared E2E migration library proposal. The current state creates:
- **Maintenance burden**: Each service must track schema changes independently
- **Silent failures**: Our 2 E2E audit tests fail when Data Storage lacks `audit_events` table
- **Duplication risk**: If we add inline migrations, we duplicate DS-owned schema

### 2. Required Migrations

| Table | Purpose | Priority |
|-------|---------|----------|
| `audit_events` | Audit trail persistence (BR-NOT-062, BR-NOT-063, BR-NOT-064) | **REQUIRED** |

**Note**: Notification only writes to `audit_events`. We don't need `workflows` or other tables.

### 3. Concerns: None

The proposal correctly places ownership with DS team who owns the schema. This is the right architectural decision.

### 4. Preferred Location: `test/infrastructure/`

**Rationale**:
- `test/infrastructure/` already contains E2E cluster setup code
- Collocates with existing Kind deployment helpers
- `pkg/testutil/` is for unit/integration test utilities, not E2E infrastructure

### 5. Additional Requirements

#### 5.1 Selective Migration Support
```go
// Allow services to request only needed tables
config := MigrationConfig{
    Tables: []string{"audit_events"}, // Notification only needs this
}
ApplyMigrations(kubeconfigPath, namespace, config)
```

#### 5.2 Idempotency
Migrations should be idempotent (CREATE TABLE IF NOT EXISTS) so services can call without checking state.

#### 5.3 Error Visibility
Clear error messages when migrations fail:
```
ERROR: Failed to apply audit_events migration: connection refused to postgres:5432
```

---

## 📊 Current E2E Test Impact

| Test | Current State | With Shared Library |
|------|---------------|---------------------|
| `01_notification_lifecycle_audit_test.go` | Uses httptest mock | Could use real DS |
| `02_audit_correlation_test.go` | Uses httptest mock | Could use real DS |

**Benefit**: With shared migrations, we can convert these 2 tests from mocks to real infrastructure, achieving full E2E coverage.

---

## 🔗 Integration Plan

Once shared library is available:

1. **Remove mocks** from `01_notification_lifecycle_audit_test.go` and `02_audit_correlation_test.go`
2. **Add migration call** in `test/infrastructure/notification.go`:
   ```go
   if err := ApplyAuditEventsMigration(kubeconfigPath, namespace, output); err != nil {
       return fmt.Errorf("failed to apply audit migrations: %w", err)
   }
   ```
3. **Verify** all 12 E2E tests pass with real infrastructure

---

## ✅ Summary

| Question | Response |
|----------|----------|
| Do you agree this should be shared? | ✅ YES |
| What migrations does your service need? | `audit_events` only |
| Any concerns about shared implementation? | None |
| Preferred location? | `test/infrastructure/` |
| Additional requirements? | Selective tables, idempotency, clear errors |

---

**Contact**: Notification Team
**Related BRs**: BR-NOT-062, BR-NOT-063, BR-NOT-064

