# Database Schema Validation

## Overview

This document describes how to validate that the PostgreSQL database schema matches the Data Storage Service models.

## Validation Strategy

### 1. Model-to-Schema Mapping

All Go models in `pkg/datastorage/models/` map directly to PostgreSQL tables:

| Go Model | PostgreSQL Table | Migration File |
|----------|------------------|----------------|
| `NotificationAudit` | `notification_audit` | `migrations/010_audit_write_api_phase1.sql` |

### 2. Automated Validation (Unit Tests)

**File**: `pkg/datastorage/models/notification_audit_test.go`

**Coverage**:
- ✅ Model structure validation (all fields present)
- ✅ Field type validation
- ✅ Enum value validation (status, channel)
- ✅ Optional field handling
- ✅ Length constraint validation
- ✅ Timestamp field validation
- ✅ Business logic validation

**Run Tests**:
```bash
go test ./pkg/datastorage/models/... -v
```

### 3. Manual Schema Validation (Integration Tests)

When a PostgreSQL database is available, you can validate the schema structure using SQL queries.

#### Check Table Existence
```sql
SELECT EXISTS (
    SELECT FROM pg_tables
    WHERE schemaname = 'public'
    AND tablename = 'notification_audit'
);
```

#### Check Column Structure
```sql
SELECT column_name, data_type, is_nullable, character_maximum_length
FROM information_schema.columns
WHERE table_schema = 'public'
AND table_name = 'notification_audit'
ORDER BY ordinal_position;
```

#### Expected Columns
| Column | Type | Nullable | Max Length |
|--------|------|----------|------------|
| `id` | `bigint` | NO | - |
| `remediation_id` | `character varying` | NO | 255 |
| `notification_id` | `character varying` | NO | 255 |
| `recipient` | `character varying` | NO | 255 |
| `channel` | `character varying` | NO | 50 |
| `message_summary` | `text` | NO | - |
| `status` | `character varying` | NO | 50 |
| `sent_at` | `timestamp with time zone` | NO | - |
| `delivery_status` | `text` | YES | - |
| `error_message` | `text` | YES | - |
| `escalation_level` | `integer` | YES | - |
| `created_at` | `timestamp with time zone` | YES | - |
| `updated_at` | `timestamp with time zone` | YES | - |

#### Check Constraints
```sql
SELECT constraint_name, constraint_type
FROM information_schema.table_constraints
WHERE table_schema = 'public'
AND table_name = 'notification_audit';
```

**Expected Constraints**:
- Primary Key: `notification_audit_pkey` on `id`
- Unique: `notification_audit_notification_id_key` on `notification_id`
- Check: Status enum validation

#### Check Indexes
```sql
SELECT indexname, indexdef
FROM pg_indexes
WHERE schemaname = 'public'
AND tablename = 'notification_audit';
```

**Expected Indexes**:
- `idx_notification_audit_remediation_id` - Fast lookup by remediation
- `idx_notification_audit_recipient` - Filter by recipient
- `idx_notification_audit_channel` - Filter by channel
- `idx_notification_audit_status` - Filter by status
- `idx_notification_audit_sent_at` - Time-range queries (DESC)

#### Check Triggers
```sql
SELECT trigger_name, event_manipulation, action_statement
FROM information_schema.triggers
WHERE event_object_schema = 'public'
AND event_object_table = 'notification_audit';
```

**Expected Triggers**:
- `trigger_notification_audit_updated_at` - Auto-update `updated_at` timestamp

#### Check pgvector Extension
```sql
SELECT extname, extversion
FROM pg_extension
WHERE extname = 'vector';
```

**Expected**: `vector` extension version `0.5.1` or higher

### 4. Migration Validation

**Authority**: `migrations/010_audit_write_api_phase1.sql`

**Validation Steps**:
1. Apply migration to test database
2. Verify table creation
3. Verify all constraints and indexes
4. Test INSERT/UPDATE/DELETE operations
5. Verify trigger functionality

### 5. Integration Test Validation

**When Available**: Integration tests will validate:
- Schema matches model
- All constraints enforced
- Indexes improve query performance
- Triggers execute correctly
- pgvector extension available

**Run Integration Tests** (requires running PostgreSQL):
```bash
make test-integration-datastorage
```

## Validation Checklist

Before deploying schema changes:

- [ ] Unit tests pass (`go test ./pkg/datastorage/models/...`)
- [ ] Migration file reviewed and approved
- [ ] Schema matches model struct tags
- [ ] All constraints documented
- [ ] All indexes documented
- [ ] Triggers tested manually
- [ ] Integration tests pass (when available)
- [ ] Performance impact assessed

## Schema Change Process

1. **Update Migration File**: Modify `migrations/0XX_*.sql`
2. **Update Model**: Modify Go struct in `pkg/datastorage/models/`
3. **Update Tests**: Modify unit tests in `pkg/datastorage/models/*_test.go`
4. **Update Documentation**: Update this file and model comments
5. **Run Validation**: Execute all validation steps above
6. **Commit**: Commit all changes together with clear message

## References

- **Migration Authority**: `migrations/010_audit_write_api_phase1.sql`
- **Model Authority**: `pkg/datastorage/models/notification_audit.go`
- **Schema Documentation**: `docs/services/crd-controllers/06-notification/database-integration.md`
- **Audit Specification**: `docs/services/crd-controllers/06-notification/audit-trace-specification.md`
- **ADR**: `docs/architecture/decisions/ADR-032-data-access-layer-isolation.md`

