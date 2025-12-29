# Migration Sync Prevention Strategy

**Date**: 2025-12-15
**Author**: AI Assistant
**Priority**: P1 - CRITICAL (Prevents 206 test failures)
**Status**: ✅ ANALYSIS COMPLETE → 🚀 IMPLEMENTATION REQUIRED

---

## 🚨 **Problem Statement**

**What Happened**: DataStorage added migration `022_add_status_reason_column.sql` but the integration test suite had a hardcoded list that didn't include it, causing 206 integration tests to fail with:

```
ERROR: column "status_reason" of relation "remediation_workflow_catalog" does not exist
```

**Root Cause**: **Manual synchronization required** between:
1. `migrations/` directory (source of truth - 17 files)
2. `test/integration/datastorage/suite_test.go` (2 hardcoded lists at lines 786-804 and 857-875)
3. `test/infrastructure/migrations.go` (E2E shared library with hardcoded `AllMigrations` list)

**Impact**:
- **ALL services** that run integration tests against DataStorage are affected
- **Silent failures** - tests pass locally but fail in CI after DS migration merge
- **Manual intervention** required every time DS adds a migration
- **206 tests blocked** by a single missing migration

**Affected Services**:
- DataStorage (integration tests)
- Notification (integration tests - uses DS database)
- SignalProcessing (integration tests via Docker Compose)
- RemediationOrchestrator (integration tests via Docker Compose)
- WorkflowExecution (E2E tests - uses E2E migration library)
- AIAnalysis (E2E tests - uses E2E migration library)
- Gateway (E2E tests - uses E2E migration library)

---

## 🎯 **Strategic Solutions**

### **RECOMMENDED: Hybrid Approach (A + D)**

Combine **auto-discovery** for resilience with **CI validation** for safety.

---

## 🔧 **Solution A: Auto-Discovery from Filesystem** ✅ **RECOMMENDED**

### **Approach**

Replace hardcoded migration lists with **dynamic filesystem discovery**:

```go
// Instead of:
migrations := []string{
    "001_initial_schema.sql",
    "002_fix_partitioning.sql",
    // ... manual list ...
}

// Use:
migrations, err := discoverMigrations("../../../migrations")
if err != nil {
    Fail(fmt.Sprintf("Failed to discover migrations: %v", err))
}
```

### **Implementation**

Create shared migration discovery utility in `test/infrastructure/migrations.go`:

```go
// DiscoverMigrations reads all .sql migration files from a directory
// Returns them sorted by goose version number (001, 002, ..., 1000)
// Filters out non-migration files (e.g., testdata/, seed files)
func DiscoverMigrations(migrationsDir string) ([]string, error) {
    entries, err := os.ReadDir(migrationsDir)
    if err != nil {
        return nil, fmt.Errorf("failed to read migrations directory: %w", err)
    }

    var migrations []string
    // Goose migration pattern: {version}_{description}.sql
    // Examples: 001_initial_schema.sql, 022_add_status_reason_column.sql
    migrationRegex := regexp.MustCompile(`^(\d+)_[a-z0-9_]+\.sql$`)

    for _, entry := range entries {
        if entry.IsDir() {
            continue // Skip directories (e.g., testdata/)
        }

        name := entry.Name()
        if migrationRegex.MatchString(name) {
            migrations = append(migrations, name)
        }
    }

    // Sort by version number (numeric sort, not lexicographic)
    sort.Slice(migrations, func(i, j int) bool {
        // Extract version numbers
        versionI := extractVersion(migrations[i])
        versionJ := extractVersion(migrations[j])
        return versionI < versionJ
    })

    return migrations, nil
}

// extractVersion extracts the numeric version from a migration filename
// Example: "022_add_status_reason_column.sql" → 22
func extractVersion(filename string) int {
    parts := strings.Split(filename, "_")
    if len(parts) < 2 {
        return 0
    }
    version, _ := strconv.Atoi(parts[0])
    return version
}
```

### **Usage in Integration Tests**

Update `test/integration/datastorage/suite_test.go`:

```go
func applyMigrationsWithPropagationTo(targetDB *sql.DB) {
    ctx := context.Background()

    // 1. Drop and recreate schema for clean state
    GinkgoWriter.Println("  🗑️  Dropping existing schema...")
    _, err := targetDB.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
    Expect(err).ToNot(HaveOccurred())

    // 2. Auto-discover ALL migrations from filesystem
    GinkgoWriter.Println("  📜 Discovering migrations from filesystem...")
    migrationsDir := "../../../migrations"
    migrations, err := infrastructure.DiscoverMigrations(migrationsDir)
    Expect(err).ToNot(HaveOccurred(), "Migration discovery should succeed")

    GinkgoWriter.Printf("  📋 Found %d migrations to apply\n", len(migrations))

    // 3. Apply each migration
    for _, migration := range migrations {
        GinkgoWriter.Printf("  📜 Applying %s...\n", migration)
        migrationPath := filepath.Join(migrationsDir, migration)
        content, err := os.ReadFile(migrationPath)
        Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Migration file %s should exist", migration))

        // Remove CONCURRENTLY keyword for test environment
        migrationSQL := strings.ReplaceAll(string(content), "CONCURRENTLY ", "")

        // Extract only the UP migration (ignore DOWN section)
        if strings.Contains(migrationSQL, "-- +goose Down") {
            parts := strings.Split(migrationSQL, "-- +goose Down")
            migrationSQL = parts[0]
        }

        _, err = targetDB.ExecContext(ctx, migrationSQL)
        Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Migration %s should apply successfully", migration))
    }

    GinkgoWriter.Println("  ✅ All migrations applied successfully")
    // ... rest of function ...
}
```

### **Pros**

✅ **Zero manual synchronization** - New migrations automatically discovered
✅ **Single source of truth** - `migrations/` directory is authoritative
✅ **Prevents silent failures** - Tests always use complete migration set
✅ **Filesystem-based** - No external dependencies (goose binary not required)
✅ **Works everywhere** - Local dev, CI, Docker Compose environments
✅ **Future-proof** - Works with any number of migrations

### **Cons**

⚠️ **Requires filesystem access** - Tests must have read access to `migrations/` directory (already the case)
⚠️ **No migration state tracking** - Unlike goose, doesn't track which migrations already applied (not an issue since tests start from clean slate)

### **Validation**

```bash
# Test that discovery works correctly
cd test/integration/datastorage
go test -v -run TestDiscoverMigrations

# Expected output:
# ✅ Discovered 17 migrations
# ✅ Migrations sorted correctly (001, 002, ..., 022, 1000)
# ✅ Testdata directory excluded
```

---

## 🛠️ **Solution B: Use Goose Programmatically** ⚙️ **ALTERNATIVE**

### **Approach**

Use **Goose library** directly in tests instead of manual SQL execution.

### **Implementation**

```go
import (
    "database/sql"
    "github.com/pressly/goose/v3"
)

func applyMigrationsWithGoose(targetDB *sql.DB) {
    // Set goose to use PostgreSQL dialect
    goose.SetDialect("postgres")

    // Set migrations directory
    migrationsDir := "../../../migrations"

    // Apply all pending migrations
    GinkgoWriter.Println("  📜 Applying migrations with Goose...")
    err := goose.Up(targetDB, migrationsDir)
    Expect(err).ToNot(HaveOccurred(), "Goose migrations should succeed")

    // Verify migration version
    version, err := goose.GetDBVersion(targetDB)
    Expect(err).ToNot(HaveOccurred())
    GinkgoWriter.Printf("  ✅ Migrations complete (version: %d)\n", version)
}
```

### **Pros**

✅ **Industry standard** - Uses production migration tool
✅ **Migration state tracking** - Goose tracks applied migrations in `goose_db_version` table
✅ **Idempotent by design** - Safe to run multiple times
✅ **Automatic discovery** - Goose finds all migration files
✅ **Rollback support** - Can test down migrations too

### **Cons**

❌ **Additional dependency** - Requires `github.com/pressly/goose/v3` in test dependencies
⚠️ **Test isolation concerns** - `goose_db_version` table persists between tests (need to clean up)
⚠️ **Overkill for tests** - Production features (rollback, version tracking) not needed in clean-slate tests

### **When to Use**

- ✅ When you want to test migration rollback behavior
- ✅ When you need production-like migration state tracking
- ✅ When you're already using Goose in production startup

---

## 📊 **Solution C: Centralized Migration Manifest** 📋 **CURRENT PATTERN**

### **Approach**

Keep hardcoded list but centralize it in ONE place: `test/infrastructure/migrations.go`

### **Current State**

`test/infrastructure/migrations.go` already has `AllMigrations` list (lines 92-228):

```go
var AllMigrations = []Migration{
    {Name: "initial_schema", File: "001_initial_schema.sql", ...},
    {Name: "fix_partitioning", File: "002_fix_partitioning.sql", ...},
    // ... currently 22 migrations ...
}
```

### **Implementation**

1. **Update** `test/infrastructure/migrations.go` to include missing migration:

```go
var AllMigrations = []Migration{
    // ... existing migrations ...
    {
        Name:        "notification_audit_table",
        File:        "021_create_notification_audit_table.sql",
        Description: "Notification audit persistence (BR-NOT-062)",
        Tables:      []string{"notification_audit"},
    },
    {
        Name:        "status_reason_column",
        File:        "022_add_status_reason_column.sql",
        Description: "Workflow status management with reason tracking (BR-STORAGE-016)",
        Tables:      []string{}, // Adds column to existing table
    },
    {
        Name:        "audit_partitions",
        File:        "1000_create_audit_events_partitions.sql",
        Description: "Monthly partitions for audit_events",
        Tables:      []string{"audit_events_y2025m12", "audit_events_y2026m01"},
    },
}
```

2. **Replace hardcoded lists** in `test/integration/datastorage/suite_test.go`:

```go
func applyMigrationsWithPropagationTo(targetDB *sql.DB) {
    // ... setup code ...

    // Use centralized migration list from infrastructure package
    GinkgoWriter.Printf("  📜 Applying %d migrations from AllMigrations...\n", len(infrastructure.AllMigrations))

    for _, migration := range infrastructure.AllMigrations {
        GinkgoWriter.Printf("  📜 Applying %s (%s)...\n", migration.File, migration.Description)
        migrationPath := "../../../migrations/" + migration.File
        // ... apply migration ...
    }
}
```

### **Pros**

✅ **Single source of truth** - One place to update migration list
✅ **No new dependencies** - Uses existing infrastructure
✅ **Metadata included** - Can document tables, indexes, business requirements
✅ **Selective application** - Can apply only audit migrations, workflow migrations, etc.

### **Cons**

❌ **Still requires manual updates** - DataStorage team must update `AllMigrations` when adding migration
⚠️ **Coordination required** - DS team must remember to update test infrastructure
⚠️ **Can still miss migrations** - If DS forgets to update list, tests still fail

### **When to Use**

- ✅ As a **transition strategy** while implementing auto-discovery
- ✅ When you need **selective migration application** (e.g., audit-only, workflow-only)
- ✅ When you want to **document migration metadata** (tables, indexes, BRs)

---

## 🚨 **Solution D: CI Validation (Preventive Check)** 🔒 **MANDATORY**

### **Approach**

Add **pre-merge validation** that detects missing migrations BEFORE they break tests.

### **Implementation**

Create `.github/workflows/validate-migration-sync.yml`:

```yaml
name: Migration Sync Validation

on:
  pull_request:
    paths:
      - 'migrations/*.sql'
      - 'test/integration/datastorage/suite_test.go'
      - 'test/infrastructure/migrations.go'

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Validate migration lists are in sync
        run: |
          # Count migrations in filesystem
          MIGRATION_COUNT=$(find migrations -name '[0-9]*_*.sql' | wc -l)
          echo "📂 Filesystem: $MIGRATION_COUNT migrations"

          # Count migrations in integration test suite
          SUITE_TEST_COUNT=$(grep -c '\.sql",' test/integration/datastorage/suite_test.go || echo "0")
          echo "🧪 Integration suite: $SUITE_TEST_COUNT migrations"

          # Count migrations in E2E infrastructure
          E2E_COUNT=$(grep -c 'File.*\.sql' test/infrastructure/migrations.go || echo "0")
          echo "🏗️  E2E infrastructure: $E2E_COUNT migrations"

          # Check if counts match
          if [ "$MIGRATION_COUNT" -ne "$SUITE_TEST_COUNT" ]; then
            echo "❌ ERROR: Migration count mismatch!"
            echo "   Filesystem has $MIGRATION_COUNT migrations"
            echo "   Integration suite has $SUITE_TEST_COUNT migrations"
            echo "   Update test/integration/datastorage/suite_test.go"
            exit 1
          fi

          if [ "$MIGRATION_COUNT" -ne "$E2E_COUNT" ]; then
            echo "❌ ERROR: Migration count mismatch!"
            echo "   Filesystem has $MIGRATION_COUNT migrations"
            echo "   E2E infrastructure has $E2E_COUNT migrations"
            echo "   Update test/infrastructure/migrations.go"
            exit 1
          fi

          echo "✅ All migration lists are in sync ($MIGRATION_COUNT migrations)"
```

### **Alternative: Pre-Commit Hook**

Create `.git/hooks/pre-commit`:

```bash
#!/bin/bash
# Validate migration sync before commit

STAGED_MIGRATIONS=$(git diff --cached --name-only --diff-filter=A migrations/*.sql | wc -l)

if [ "$STAGED_MIGRATIONS" -gt 0 ]; then
    echo "🔍 Detected new migration files, validating sync..."

    # Check if test files were also updated
    SUITE_TEST_UPDATED=$(git diff --cached --name-only test/integration/datastorage/suite_test.go | wc -l)
    E2E_UPDATED=$(git diff --cached --name-only test/infrastructure/migrations.go | wc -l)

    if [ "$SUITE_TEST_UPDATED" -eq 0 ]; then
        echo "❌ ERROR: New migration added but suite_test.go not updated!"
        echo "   Please update: test/integration/datastorage/suite_test.go"
        echo "   Or use auto-discovery (Solution A)"
        exit 1
    fi

    if [ "$E2E_UPDATED" -eq 0 ]; then
        echo "⚠️  WARNING: New migration added but E2E infrastructure not updated"
        echo "   Consider updating: test/infrastructure/migrations.go"
    fi

    echo "✅ Migration sync validation passed"
fi
```

### **Pros**

✅ **Catches issues early** - Before PR merge, not after CI failure
✅ **Low overhead** - Simple shell script, no dependencies
✅ **Educational** - Reminds developers to update test infrastructure
✅ **Complements auto-discovery** - Works with any solution

### **Cons**

⚠️ **Reactive, not preventive** - Still requires manual action
⚠️ **Can be bypassed** - Pre-commit hooks can be skipped with `--no-verify`

### **When to Use**

- ✅ **ALWAYS** - Use this in addition to Solution A or B
- ✅ **As interim solution** - While implementing auto-discovery
- ✅ **Safety net** - Catches human errors in manual processes

---

## 🎯 **Recommended Implementation Plan**

### **Phase 1: Immediate Fix** ✅ **COMPLETE**

Status: Already applied

- [x] Renumber `021_add_status_reason_column.sql` → `022_add_status_reason_column.sql`
- [x] Update hardcoded list in `suite_test.go` (line 802)
- [x] Run integration tests to confirm fix

### **Phase 2: Short-Term (This Week)** 🚀 **RECOMMENDED**

**Priority**: P1 - Prevents future failures

**Tasks**:
1. Implement `DiscoverMigrations()` in `test/infrastructure/migrations.go`
2. Replace hardcoded lists in `test/integration/datastorage/suite_test.go` (2 places: lines 786-804 and 857-875)
3. Add unit tests for `DiscoverMigrations()` function
4. Update `test/infrastructure/migrations.go` to use discovery for `AllMigrations`
5. Add CI validation workflow (Solution D)

**Effort**: 2-3 hours
**Risk**: Low (filesystem discovery is simple)
**Impact**: Eliminates 100% of manual synchronization

### **Phase 3: Long-Term (V1.1)** 🔮 **FUTURE**

**Priority**: P2 - Production quality improvement

**Tasks**:
1. Consider migrating to Goose programmatic usage (Solution B)
2. Add migration rollback testing in integration suite
3. Implement migration version validation in DataStorage service startup
4. Add migration audit logging (who applied what migration when)

**Effort**: 1-2 days
**Risk**: Medium (requires Goose library integration)
**Impact**: Production-grade migration management

---

## 📋 **Decision Matrix**

| Solution | Complexity | Reliability | Maintenance | Recommendation |
|---|---|---|---|---|
| **A: Auto-Discovery** | Low | High | Zero | ✅ **IMPLEMENT NOW** |
| **B: Use Goose** | Medium | Very High | Low | 🔮 Future (V1.1) |
| **C: Centralized List** | Very Low | Medium | High | 🚫 Avoid (still manual) |
| **D: CI Validation** | Low | High | Zero | ✅ **IMPLEMENT NOW** |

---

## 🧪 **Testing Strategy**

### **Validation Tests**

Create `test/infrastructure/migrations_test.go`:

```go
func TestDiscoverMigrations(t *testing.T) {
    migrationsDir := "../../migrations"

    migrations, err := DiscoverMigrations(migrationsDir)
    assert.NoError(t, err)
    assert.NotEmpty(t, migrations)

    // Verify expected migrations exist
    expectedMigrations := []string{
        "001_initial_schema.sql",
        "013_create_audit_events_table.sql",
        "022_add_status_reason_column.sql",
        "1000_create_audit_events_partitions.sql",
    }

    for _, expected := range expectedMigrations {
        assert.Contains(t, migrations, expected,
            "Migration %s should be discovered", expected)
    }

    // Verify migrations are sorted by version number
    for i := 1; i < len(migrations); i++ {
        prevVersion := extractVersion(migrations[i-1])
        currVersion := extractVersion(migrations[i])
        assert.Less(t, prevVersion, currVersion,
            "Migrations should be sorted: %s (%d) before %s (%d)",
            migrations[i-1], prevVersion, migrations[i], currVersion)
    }

    t.Logf("✅ Discovered %d migrations in correct order", len(migrations))
}

func TestMigrationNamingConvention(t *testing.T) {
    migrationsDir := "../../migrations"
    entries, err := os.ReadDir(migrationsDir)
    assert.NoError(t, err)

    // Goose migration pattern: {version}_{description}.sql
    validPattern := regexp.MustCompile(`^(\d{3,4})_[a-z0-9_]+\.sql$`)

    for _, entry := range entries {
        if entry.IsDir() {
            continue
        }

        name := entry.Name()
        if strings.HasSuffix(name, ".sql") && !validPattern.MatchString(name) {
            t.Errorf("❌ Migration %s does not follow naming convention: {version}_{description}.sql", name)
        }
    }

    t.Log("✅ All migrations follow goose naming convention")
}
```

### **Integration Test Validation**

```bash
# Run DataStorage integration tests with auto-discovery
cd test/integration/datastorage
go test -v -count=1 ./...

# Expected output:
# 📋 Discovering migrations from filesystem...
# 📋 Found 17 migrations to apply
# ✅ All migrations applied successfully
# ✅ 206/206 tests passing
```

---

## 📊 **Success Metrics**

### **Immediate (Phase 1)**
- [x] 206 integration tests passing
- [x] No missing migration errors

### **Short-Term (Phase 2)**
- [ ] Zero manual updates required when DS adds migration
- [ ] CI catches migration sync issues before merge
- [ ] Auto-discovery tests passing (100% coverage)

### **Long-Term (Phase 3)**
- [ ] Production uses goose for migration management
- [ ] Migration state tracked in `goose_db_version` table
- [ ] Rollback capability tested and validated

---

## 🔗 **Related Documents**

- **Root Cause Analysis**: `docs/handoff/DATASTORAGE_ROOT_CAUSE_ANALYSIS_DEC_15_2025.md`
- **Triage Report**: `docs/handoff/TRIAGE_DATASTORAGE_V1.0_DEC_15_2025.md`
- **Goose Decision**: `docs/architecture/decisions/DD-012-goose-database-migration-management.md`
- **Migration Numbering Conflict**: `docs/handoff/TRIAGE_DATASTORAGE_MIGRATION_SYNC_ISSUE.md`

---

## 🎯 **Recommendation Summary**

**IMPLEMENT NOW** (This Week):
1. ✅ **Solution A**: Auto-discovery from filesystem
2. ✅ **Solution D**: CI validation workflow

**CONSIDER LATER** (V1.1):
3. 🔮 **Solution B**: Migrate to Goose programmatic usage

**AVOID**:
4. ❌ **Solution C**: Centralized manual list (still requires sync)

**Confidence**: **95%** - Auto-discovery eliminates manual synchronization completely

**Estimated Effort**: 2-3 hours for Phase 2 implementation

**Risk**: **Low** - Filesystem discovery is straightforward and testable

---

**Status**: ✅ Analysis complete, awaiting implementation approval

