# DD-012: Goose Database Migration Management

**Status**: ✅ Approved (2025-11-05)
**Date**: 2025-11-05
**Decision Makers**: Development Team
**Priority**: **P1 - FOUNDATIONAL** (Required for Day 12 ADR-033 implementation)
**Supersedes**: None
**Related To**: 
- ADR-033 (Remediation Playbook Catalog & Multi-Dimensional Success Tracking)
- DD-010 (PostgreSQL Driver Migration - pgx)
- DD-011 (PostgreSQL 16+ Version Requirements)
- DD-SCHEMA-001 (Data Storage Schema Authority)

---

## 📋 **Context**

**Problem**: Data Storage Service requires a reliable, version-controlled database migration management system.

**Current State**:
- Data Storage Service uses PostgreSQL 16+ with pgvector extension
- Multiple migration files exist (`001_initial_schema.sql` through `012_adr033_multidimensional_tracking.sql`)
- No formal migration management tool documented
- ADR-033 implementation (Day 12) requires applying schema changes to production

**Requirements**:
- **Version Control**: Track which migrations have been applied to which database
- **Idempotency**: Safe to run migrations multiple times without errors
- **Rollback**: Ability to revert schema changes if needed
- **Order Enforcement**: Migrations must execute in correct sequence
- **Multi-Environment**: Same migrations across dev/test/staging/production
- **Audit Trail**: Record when and by whom migrations were applied

**Business Requirements**:
- BR-STORAGE-031-03: ADR-033 schema migration (11 columns, 6 indexes)
- BR-PLATFORM-006: Production-ready infrastructure with change tracking
- BR-SECURITY-001: Audit trail for database schema changes

---

## 🎯 **Decision**

**APPROVED**: Use **Goose** (`github.com/pressly/goose/v3`) as the database migration management tool for the Data Storage Service.

**Tooling**: Goose v3.26.0+

**Scope**: All PostgreSQL schema changes for Data Storage Service

**Migration File Format**: `{version}_{description}.sql` with `-- +goose Up` and `-- +goose Down` sections

---

## 🔍 **Alternatives Considered**

### **Alternative A: Goose (pressly/goose)** ✅ **APPROVED**

**Approach**: Use Goose for all database migrations with versioned SQL files

**Migration File Example**:
```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS
    incident_type VARCHAR(100);

CREATE INDEX IF NOT EXISTS idx_incident_type_success
ON resource_action_traces(incident_type, status, action_timestamp DESC)
WHERE incident_type IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_incident_type_success;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS incident_type;
-- +goose StatementEnd
```

**Usage**:
```bash
# Apply all pending migrations
goose -dir migrations postgres "connection_string" up

# Apply migrations up to specific version
goose -dir migrations postgres "connection_string" up-to 12

# Rollback last migration
goose -dir migrations postgres "connection_string" down

# Check migration status
goose -dir migrations postgres "connection_string" status
```

**Pros**:
- ✅ **Pure SQL Migrations**: No DSL to learn, just SQL
- ✅ **Version Tracking**: `goose_db_version` table tracks applied migrations
- ✅ **Idempotent**: Built-in duplicate version detection
- ✅ **Up/Down Support**: Explicit rollback scripts required
- ✅ **PostgreSQL Native**: First-class PostgreSQL support
- ✅ **Lightweight**: Single binary, no runtime dependencies
- ✅ **Go Native**: Same language as Data Storage Service
- ✅ **CI/CD Friendly**: Command-line tool for automation
- ✅ **Multi-DB Support**: Works with PostgreSQL, MySQL, SQLite, etc.
- ✅ **Active Maintenance**: 11.6k+ GitHub stars, active development
- ✅ **Proven at Scale**: Used by production Go applications

**Cons**:
- ⚠️ **Manual Version Management**: Developers must assign unique version numbers
- ⚠️ **No Automatic Rollback**: Requires explicit `-- +goose Down` sections
- ⚠️ **File Naming Strict**: Must use `{version}_{description}.sql` format (underscore required)

**Validation Results** (from `test_adr033_migration.sh`):
- ✅ Migration 012 applied successfully (7.13ms)
- ✅ All 11 ADR-033 columns created
- ✅ All 6 ADR-033 indexes created
- ✅ Idempotent: "no migrations to run" on second execution
- ✅ Rollback successful (3.36ms)
- ✅ Data integrity preserved through all operations

**Confidence**: **95%** - Industry-standard choice for Go + PostgreSQL projects

---

### **Alternative B: golang-migrate/migrate** ❌ **REJECTED**

**Approach**: Use `golang-migrate` for database migrations

**Pros**:
- ✅ **Popular**: 15k+ GitHub stars
- ✅ **Multi-DB Support**: Works with many databases
- ✅ **Go Library + CLI**: Can be embedded or used standalone

**Cons**:
- ❌ **Less Go-Native**: Designed for multi-language projects
- ❌ **No Up/Down in Same File**: Requires separate `{version}.up.sql` and `{version}.down.sql` files
- ❌ **More Complex**: Additional configuration for PostgreSQL
- ❌ **Overkill**: More features than needed for our use case

**Confidence**: **60%** - Good tool, but Goose is simpler for our needs

---

### **Alternative C: Manual SQL Scripts** ❌ **REJECTED**

**Approach**: Run SQL scripts manually via `psql` or `pgAdmin`

**Pros**:
- ✅ **Simple**: No tool to learn
- ✅ **Flexible**: Full control over execution

**Cons**:
- ❌ **NO VERSION TRACKING**: No way to know which migrations applied
- ❌ **NOT IDEMPOTENT**: Risk of applying same migration twice
- ❌ **NO ROLLBACK**: Manual revert scripts required
- ❌ **ERROR-PRONE**: Easy to apply migrations out of order
- ❌ **NOT AUDITABLE**: No record of who applied what and when
- ❌ **CI/CD IMPOSSIBLE**: Cannot automate deployments

**Confidence**: **5%** - Unacceptable for production systems

---

### **Alternative D: ORM Migrations (GORM, Ent, etc.)** ❌ **REJECTED**

**Approach**: Use an ORM's built-in migration system

**Pros**:
- ✅ **Auto-Generated**: Migrations created from Go structs
- ✅ **Type-Safe**: Changes tied to Go code

**Cons**:
- ❌ **Data Storage Uses Raw SQL**: Not using an ORM for queries
- ❌ **Less Control**: ORM decides how to generate SQL
- ❌ **Vendor Lock-In**: Tied to specific ORM
- ❌ **Complex Migrations**: Hard to express advanced PostgreSQL features (pgvector, indexes, partitions)
- ❌ **Not Pure SQL**: Generated SQL may not be optimal

**Confidence**: **20%** - Not suitable for raw SQL-based service

---

## 📋 **Implementation Guidance**

### **1. Installation**

```bash
# Install goose CLI tool
go install github.com/pressly/goose/v3/cmd/goose@latest

# Verify installation
goose --version
```

### **2. Migration File Naming Convention**

**Format**: `{version}_{description}.sql`

**Rules**:
- ✅ **Use underscores** (`_`) to separate version and description
- ❌ **Do NOT use dashes** (`-`) - goose will reject the file
- ✅ **Sequential version numbers**: 001, 002, 003, ..., 012, ...
- ✅ **Descriptive names**: `012_adr033_multidimensional_tracking.sql`
- ❌ **Avoid gaps**: If version 009 exists, don't skip to 012 (use 010, 011, 012)

**Examples**:
```
✅ CORRECT:
  001_initial_schema.sql
  002_fix_partitioning.sql
  012_adr033_multidimensional_tracking.sql

❌ INCORRECT:
  99-init-vector.sql          # Uses dash instead of underscore
  seed_test_data.sql          # No version number
  2_add_column.sql            # Version not zero-padded
```

### **3. Migration File Structure**

**Required Sections**:
```sql
-- +goose Up
-- SQL for applying changes
ALTER TABLE ...;
CREATE INDEX ...;

-- +goose Down
-- SQL for reverting changes (reverse order!)
DROP INDEX ...;
ALTER TABLE ...;
```

**Best Practices**:
- ✅ Use `IF NOT EXISTS` in Up section for idempotency
- ✅ Use `IF EXISTS` in Down section for safe rollback
- ✅ Include comments explaining the change (ADR reference, BR reference)
- ✅ Down section should reverse Up section in **reverse order**
- ✅ Test both Up and Down migrations in development

### **4. Running Migrations**

**Development**:
```bash
# Apply all pending migrations
goose -dir migrations postgres "host=localhost port=5433 user=slm_user password=dev_password dbname=action_history sslmode=disable" up

# Check status
goose -dir migrations postgres "connection_string" status
```

**Production** (via CI/CD):
```bash
# Apply migrations up to specific version (safer)
goose -dir migrations postgres "$POSTGRES_CONNECTION_STRING" up-to 12

# Verify version
goose -dir migrations postgres "$POSTGRES_CONNECTION_STRING" version
```

**Rollback** (emergency use only):
```bash
# Rollback last migration
goose -dir migrations postgres "$POSTGRES_CONNECTION_STRING" down

# Rollback to specific version
goose -dir migrations postgres "$POSTGRES_CONNECTION_STRING" down-to 11
```

### **5. Integration with Data Storage Service**

**Startup Validation** (future enhancement):
```go
// pkg/datastorage/server/server.go
func (s *Server) validateSchemaVersion() error {
    // Query goose_db_version table
    var currentVersion int64
    err := s.db.QueryRow(`
        SELECT version_id 
        FROM goose_db_version 
        WHERE is_applied = true 
        ORDER BY tstamp DESC 
        LIMIT 1
    `).Scan(&currentVersion)
    
    if err != nil {
        return fmt.Errorf("failed to check schema version: %w", err)
    }
    
    if currentVersion < s.requiredSchemaVersion {
        return fmt.Errorf("database schema version %d is too old, required: %d",
            currentVersion, s.requiredSchemaVersion)
    }
    
    s.logger.Info("Schema version validated", 
        zap.Int64("current_version", currentVersion),
        zap.Int64("required_version", s.requiredSchemaVersion))
    
    return nil
}
```

### **6. Directory Structure**

```
migrations/
├── 001_initial_schema.sql
├── 002_fix_partitioning.sql
├── ...
├── 012_adr033_multidimensional_tracking.sql
└── testdata/
    └── seed_test_data.sql  # Non-migration files go here
```

**Rules**:
- ✅ Only migration files (`{version}_{description}.sql`) in `migrations/` root
- ✅ Test data, seeds, and utilities go in `migrations/testdata/`
- ❌ Do NOT put non-migration files in `migrations/` root (goose will try to parse them)

---

## 🧪 **Validation & Testing**

### **Test Script**: `test_adr033_migration.sh`

**Purpose**: Validate migration correctness in isolated Podman container

**Test Scenarios**:
1. ✅ **First Run**: Migration applies successfully
2. ✅ **Idempotency**: Running migration twice produces "no migrations to run"
3. ✅ **Rollback**: Down migration cleanly removes changes
4. ✅ **Data Integrity**: Existing data preserved through all operations

**Test Results** (ADR-033 Migration 012):
- **First Run**: 7.13ms execution time
- **Columns Created**: 11/11 ADR-033 columns
- **Indexes Created**: 6/6 ADR-033 indexes
- **Idempotency**: Goose correctly skips already-applied migration
- **Rollback**: 3.36ms execution time
- **Data Integrity**: 100% - all existing records preserved

**Run Tests**:
```bash
# Test ADR-033 migration specifically
./test_adr033_migration.sh

# Expected output:
# ✅ ALL TESTS PASSED
# Migration is READY for Day 12 execution!
```

---

## 📊 **Migration Version Management**

### **Current Migration Status** (as of 2025-11-05):

| Version | Migration | Status | Date | Purpose |
|---|---|---|---|---|
| 001 | `initial_schema.sql` | ✅ Applied | 2024-10-15 | Base tables and schema |
| 002 | `fix_partitioning.sql` | ✅ Applied | 2024-10-20 | Partition fixes |
| 003 | `stored_procedures.sql` | ✅ Applied | 2024-10-22 | Database functions |
| 004 | `add_effectiveness_assessment_due.sql` | ✅ Applied | 2024-10-25 | Effectiveness tracking |
| 005 | `vector_schema.sql` | ✅ Applied | 2024-10-28 | pgvector support |
| 006 | `effectiveness_assessment.sql` | ✅ Applied | 2024-11-01 | Assessment framework |
| 007 | `add_context_column.sql` | ✅ Applied | 2024-11-01 | Context enrichment |
| 008 | `context_api_compatibility.sql` | ✅ Applied | 2024-11-02 | API compatibility |
| 009 | `update_vector_dimensions.sql` | ✅ Applied | 2024-11-02 | Vector dimension updates |
| 010 | `audit_write_api_phase1.sql` | ✅ Applied | 2024-11-03 | Audit write API |
| 011 | `rename_alert_to_signal.sql` | ✅ Applied | 2024-11-04 | Terminology migration |
| **012** | **`adr033_multidimensional_tracking.sql`** | 🚀 **READY** | **2025-11-05** | **ADR-033 multi-dimensional success tracking** |

### **Next Migration**: Version 013 (TBD)

---

## 🔗 **Related Decisions**

### **DD-010: PostgreSQL Driver Migration (pgx)**
- Goose works with both `lib/pq` and `pgx` drivers
- Uses `database/sql` connection string format
- Compatible with Data Storage's pgx migration

### **DD-011: PostgreSQL 16+ Version Requirements**
- Goose supports all PostgreSQL versions
- Validates PostgreSQL 16+ features (pgvector, advanced indexes)
- No version-specific Goose configuration needed

### **DD-SCHEMA-001: Data Storage Schema Authority**
- Goose migrations are the **single source of truth** for Data Storage schema
- Other services (Context API) consume schema via REST API, not direct DB access
- Migration files stored in Data Storage repository

### **ADR-033: Remediation Playbook Catalog**
- Migration 012 implements ADR-033 schema changes
- Goose ensures consistent application across all environments
- Rollback capability critical for production safety

---

## 📝 **Do's and Don'ts**

### **✅ DO**

1. **Always write Down migrations** - Every Up must have a corresponding Down
2. **Test migrations locally** - Use `test_adr033_migration.sh` pattern
3. **Use IF NOT EXISTS / IF EXISTS** - Make migrations idempotent
4. **Include BR/ADR references** - Comment migrations with business context
5. **Use sequential version numbers** - Avoid gaps in version sequence
6. **Validate before production** - Run `goose status` to confirm state
7. **Review Down migrations** - Ensure reverse order of Up operations
8. **Keep migrations pure SQL** - Avoid Go code in migration files

### **❌ DON'T**

1. **Don't use dashes in filenames** - Use underscores (`_`) only
2. **Don't skip version numbers** - Use sequential numbering
3. **Don't modify applied migrations** - Create new migration instead
4. **Don't put non-migration files in migrations/** - Use `testdata/` instead
5. **Don't run migrations manually in production** - Use CI/CD automation
6. **Don't forget to test rollback** - Always validate Down migrations
7. **Don't assume idempotency** - Always test running migration twice
8. **Don't hardcode connection strings** - Use environment variables

---

## 📌 **Summary**

**Decision**: Use **Goose v3.26.0+** for database migration management

**Key Benefits**:
- ✅ Industry-standard tool for Go + PostgreSQL
- ✅ Pure SQL migrations (no DSL)
- ✅ Built-in version tracking and idempotency
- ✅ Validated with ADR-033 migration (100% test pass rate)

**Migration File Format**: `{version}_{description}.sql` with `-- +goose Up/Down`

**Status**: ✅ **APPROVED** - Ready for Day 12 ADR-033 implementation

**Confidence**: **95%** - This is the right choice for production

---

**Next Steps**:
1. Day 12.1: Apply migration 012 using goose (ADR-033 schema changes)
2. Day 12.2: Update Go models with 11 new ADR-033 fields
3. Day 12.3: Create aggregation response models for new REST APIs

