# ✅ Response: SignalProcessing Team - E2E Migration Library Proposal

**Team**: SignalProcessing
**Date**: December 10, 2025
**Decision**: ✅ **APPROVED**

---

## Feedback

### 1. Agreement: **YES** - SP Requires DataStorage for Audit

Per authoritative documentation:

| Requirement | Impact |
|-------------|--------|
| **BR-SP-090** | Categorization Audit Trail - Log all decisions via Data Storage API |
| **ADR-032** | Audit writes via Data Storage REST API (**MANDATORY**) |
| **ADR-038** | Async Buffered Audit Ingestion (fire-and-forget pattern) |

**SP has a hard dependency on DataStorage** for audit event persistence.

### 2. Required Migrations

SP E2E tests need the `audit_events` table to test BR-SP-090:

| Table | Required | Reason |
|-------|----------|--------|
| `audit_events` | ✅ **YES** | BR-SP-090: Store categorization audit trail |
| `audit_events_*` partitions | ✅ **YES** | ADR-038: Partitioned storage |

**Migration Files Needed**:
- `013_create_audit_events_table.sql`
- `1000_create_audit_events_partitions.sql`

### 3. Concerns: **None**

The shared library approach is correct - DS owns the schema, SP consumes it.

### 4. Preferred Location: **`test/infrastructure/migrations.go`**

Consistent with existing infrastructure pattern.

### 5. Additional Requirements

**Current SP E2E Gap**:

The current SP E2E tests (`test/e2e/signalprocessing/`) do NOT deploy DataStorage, so:
- `AuditClient` is **nil** in current tests
- BR-SP-090 is NOT fully E2E tested

**Future Enhancement Needed**:

Once the shared migration library exists, SP E2E infrastructure should:

```go
// test/infrastructure/signalprocessing.go - ENHANCEMENT NEEDED
func SetupSignalProcessingE2ECluster(...) error {
    // ... create Kind cluster ...

    // Deploy PostgreSQL + Redis (for DataStorage)
    if err := deployPostgresAndRedis(kubeconfigPath, namespace, output); err != nil {
        return err
    }

    // Apply migrations using shared library
    if err := ApplyMigrations(kubeconfigPath, namespace, AuditMigrations, output); err != nil {
        return err
    }

    // Deploy DataStorage
    if err := deployDataStorage(kubeconfigPath, namespace, output); err != nil {
        return err
    }

    // Deploy SP controller with AuditClient configured
    // ...
}
```

---

## 📋 Current vs Required E2E Infrastructure

| Component | Current | Required for BR-SP-090 |
|-----------|---------|------------------------|
| Kind cluster | ✅ | ✅ |
| SP CRD | ✅ | ✅ |
| SP Controller | ✅ | ✅ |
| Rego ConfigMaps | ✅ | ✅ |
| PostgreSQL | ❌ | ✅ |
| Redis | ❌ | ✅ |
| DataStorage | ❌ | ✅ |
| `audit_events` table | ❌ | ✅ |

---

## ✅ Summary

| Question | Answer |
|----------|--------|
| Does SP need this library? | **YES** |
| Required migrations | `audit_events` + partitions |
| Current E2E status | ❌ Missing DS dependency |
| Owner | **DataStorage Team** (schema owner) |
| Location | `test/infrastructure/migrations.go` |

---

**Document Version**: 1.1
**Created**: December 10, 2025
**Updated**: December 10, 2025 (Fixed: SP DOES require DataStorage per BR-SP-090)
**Maintained By**: SignalProcessing Team
