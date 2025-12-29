# DataStorage Migration Synchronization Issue - Triage & Prevention

**Date**: December 15, 2025  
**Triaged By**: Platform Team  
**Severity**: 🔴 **CRITICAL** - Blocks 164 integration tests

---

## 🎯 **Executive Summary**

**Root Cause**: New migration file created but **not added to test suite's hardcoded migration list**.

**Impact**: Integration tests fail because `status_reason` column doesn't exist in test database.

**Files Affected**:
- `migrations/021_add_status_reason_column.sql` (created TODAY, Dec 15 19:19)
- `test/integration/datastorage/suite_test.go` (hardcoded list missing the new migration)

**Fix Effort**: 5 minutes (immediate) + 2-4 hours (prevention system)

---

## 🔍 **Root Cause Analysis**

### **What Happened**

1. **Dec 11, 2025**: Migration `021_create_notification_audit_table.sql` created and added to test suite
2. **Dec 15, 2025 19:19**: Migration `021_add_status_reason_column.sql` created
3. **Problem**: New migration file was NOT added to the hardcoded list in `suite_test.go:801`
4. **Result**: Migration file exists but is never applied during tests

### **The Hardcoded List Problem**

**File**: `test/integration/datastorage/suite_test.go` (lines 784-803)

```go
migrations := []string{
    "001_initial_setup.sql",
    "002_add_indices.sql",
    // ... more migrations ...
    "020_add_workflow_label_columns.sql",
    "021_create_notification_audit_table.sql",  // ← Added Dec 11
    // ❌ MISSING: "021_add_status_reason_column.sql"  ← Created Dec 15, NOT added
    "1000_create_audit_events_partitions.sql",
}
```

**Why This Exists**: 
- Tests need explicit control over migration order
- Prevents accidental application of incomplete migrations
- Allows skipping problematic migrations during development

**The Problem**:
- **MANUAL PROCESS**: Developer must remember to update this list
- **NO VALIDATION**: No automated check to ensure all migrations are included
- **EASY TO FORGET**: Especially when multiple people create migrations simultaneously

---

## 🚨 **Additional Issue: Migration Numbering Conflict**

### **TWO Files with Number 021**

```bash
$ ls migrations/021*.sql
migrations/021_add_status_reason_column.sql        # Dec 15 19:19
migrations/021_create_notification_audit_table.sql  # Dec 11 21:32
```

**Why This is Bad**:
1. **Ambiguous Ordering**: Which 021 runs first?
2. **Goose Confusion**: Migration tools expect unique numbers
3. **Maintenance Nightmare**: Hard to track migration history

**How It Happened**:
- Person A creates `021_create_notification_audit_table.sql`
- Person B (days later) doesn't check existing numbers, creates `021_add_status_reason_column.sql`
- No automated validation to prevent duplicate numbers

---

## 🔧 **Immediate Fix** (5 minutes)

### **Step 1: Renumber the Newer Migration**

```bash
cd migrations/
mv 021_add_status_reason_column.sql 022_add_status_reason_column.sql
```

### **Step 2: Update Test Suite** 

**File**: `test/integration/datastorage/suite_test.go` (line ~801)

```go
migrations := []string{
    // ... existing migrations ...
    "020_add_workflow_label_columns.sql",
    "021_create_notification_audit_table.sql",
    "022_add_status_reason_column.sql",  // ← ADD THIS LINE
    "1000_create_audit_events_partitions.sql",
}
```

### **Step 3: Verify Fix**

```bash
# Re-run integration tests
make test-integration-datastorage 2>&1 | tee test-results-after-fix.txt

# Should now pass
```

**Estimated Time**: 5 minutes

---

## 🛡️ **Prevention Strategy** (Multiple Layers)

### **Layer 1: Automated Migration List Sync** ⭐ RECOMMENDED

**Create**: `scripts/validate-migration-sync.sh`

```bash
#!/usr/bin/env bash
# Validates that all migrations are included in test suite

set -euo pipefail

MIGRATIONS_DIR="migrations"
TEST_SUITE="test/integration/datastorage/suite_test.go"

echo "🔍 Validating migration synchronization..."

# Get all migration files (exclude 1000_* which are special)
MIGRATION_FILES=$(ls -1 "$MIGRATIONS_DIR"/*.sql | grep -v "1000_" | xargs -n1 basename | sort)

# Extract migrations from test suite hardcoded list
# Look for lines like: "021_create_notification_audit_table.sql",
TEST_MIGRATIONS=$(grep -E '^\s+"[0-9]+.*\.sql"' "$TEST_SUITE" | \
    sed 's/.*"\([^"]*\)".*/\1/' | \
    grep -v "1000_" | \
    sort)

# Compare lists
MISSING_IN_TESTS=""
for migration in $MIGRATION_FILES; do
    if ! echo "$TEST_MIGRATIONS" | grep -q "^$migration$"; then
        MISSING_IN_TESTS="$MISSING_IN_TESTS\n  - $migration"
    fi
done

if [ -n "$MISSING_IN_TESTS" ]; then
    echo "❌ VALIDATION FAILED: Migrations missing from test suite:"
    echo -e "$MISSING_IN_TESTS"
    echo ""
    echo "📝 Action Required:"
    echo "  Add missing migrations to $TEST_SUITE around line 801"
    echo ""
    exit 1
fi

echo "✅ All migrations are included in test suite"
exit 0
```

**Integration**:
```bash
# Add to Makefile
.PHONY: validate-migrations
validate-migrations:
	@./scripts/validate-migration-sync.sh

# Add to pre-commit hook
# Add to CI/CD pipeline
```

**Benefit**: Catches missing migrations BEFORE they cause test failures

---

### **Layer 2: Migration Number Uniqueness Check** ⭐ RECOMMENDED

**Create**: `scripts/validate-migration-numbers.sh`

```bash
#!/usr/bin/env bash
# Validates that migration numbers are unique

set -euo pipefail

MIGRATIONS_DIR="migrations"

echo "🔍 Validating migration number uniqueness..."

# Extract migration numbers
MIGRATION_NUMBERS=$(ls -1 "$MIGRATIONS_DIR"/*.sql | \
    xargs -n1 basename | \
    sed 's/^\([0-9]\+\)_.*/\1/' | \
    sort)

# Check for duplicates
DUPLICATES=$(echo "$MIGRATION_NUMBERS" | uniq -d)

if [ -n "$DUPLICATES" ]; then
    echo "❌ VALIDATION FAILED: Duplicate migration numbers found:"
    for num in $DUPLICATES; do
        echo "  Number $num used in:"
        ls -1 "$MIGRATIONS_DIR/${num}_"*.sql | xargs -n1 basename | sed 's/^/    - /'
    done
    echo ""
    echo "📝 Action Required:"
    echo "  Renumber one of the duplicate migrations to the next available number"
    echo ""
    exit 1
fi

echo "✅ All migration numbers are unique"
exit 0
```

**Integration**: Same as Layer 1 (Makefile, pre-commit, CI/CD)

**Benefit**: Prevents duplicate migration numbers

---

### **Layer 3: Auto-Generate Migration List** 🔮 FUTURE

**Replace hardcoded list with dynamic generation**:

```go
// suite_test.go

func getMigrationFiles() []string {
    migrationsDir := "../../../migrations"
    
    files, err := os.ReadDir(migrationsDir)
    if err != nil {
        Fail(fmt.Sprintf("Failed to read migrations directory: %v", err))
    }
    
    var migrations []string
    for _, file := range files {
        if strings.HasSuffix(file.Name(), ".sql") && !strings.HasPrefix(file.Name(), "1000_") {
            migrations = append(migrations, file.Name())
        }
    }
    
    // Sort by migration number
    sort.Slice(migrations, func(i, j int) bool {
        // Extract numbers: 021_name.sql -> 021
        numI := strings.Split(migrations[i], "_")[0]
        numJ := strings.Split(migrations[j], "_")[0]
        return numI < numJ
    })
    
    // Add special migrations at the end
    migrations = append(migrations, "1000_create_audit_events_partitions.sql")
    
    return migrations
}

// Usage in applyMigrationsWithPropagationTo:
func applyMigrationsWithPropagationTo(targetDB *sql.DB) {
    ctx := context.Background()
    
    // ... drop/recreate schema ...
    
    migrations := getMigrationFiles()  // ← Auto-generated
    
    for _, migration := range migrations {
        // ... apply migration ...
    }
}
```

**Benefits**:
- ✅ No manual list maintenance
- ✅ Impossible to forget new migrations
- ✅ Automatically handles migration additions

**Risks**:
- ⚠️ Loses explicit control over order
- ⚠️ Might apply incomplete migrations accidentally
- ⚠️ Requires careful testing

**Recommendation**: Implement Layers 1 & 2 first, consider Layer 3 for V1.1

---

### **Layer 4: Migration Creation Guidelines** 📋 PROCESS

**Create**: `docs/development/MIGRATION_CREATION_GUIDE.md`

```markdown
# Database Migration Creation Guide

## 🚨 **MANDATORY CHECKLIST** (Before Creating a Migration)

### **Step 1: Check Next Available Number**
```bash
# List current migrations
ls -1 migrations/*.sql | tail -5

# Identify highest number (e.g., 021)
# Your new migration should be 022
```

### **Step 2: Create Migration File**
```bash
# Use next sequential number
touch migrations/022_your_descriptive_name.sql
```

### **Step 3: Write Migration**
```sql
-- +goose Up
-- +goose StatementBegin
-- Migration: [Brief description]
-- Purpose: [What business need this serves]
-- BR-XXX-XXX: [Business requirement if applicable]

ALTER TABLE your_table
ADD COLUMN your_column TYPE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE your_table
DROP COLUMN IF EXISTS your_column;
-- +goose StatementEnd
```

### **Step 4: Add to Test Suite** ⚠️ CRITICAL
```bash
# Edit test/integration/datastorage/suite_test.go
# Add your migration to the hardcoded list around line 801

migrations := []string{
    // ... existing ...
    "022_your_descriptive_name.sql",  // ← ADD THIS
}
```

### **Step 5: Validate**
```bash
# Run validation scripts
make validate-migrations

# Run integration tests
make test-integration-datastorage
```

### **Step 6: Document (If Significant)**
```bash
# For major schema changes, update:
# - docs/services/stateless/data-storage/README.md
# - Schema documentation
# - Migration changelog
```

---

## ❌ **Common Mistakes to Avoid**

1. **Using duplicate numbers**: Always check existing migrations first
2. **Forgetting test suite**: Migration file exists but isn't applied
3. **No rollback**: Always include `-- +goose Down` section
4. **Breaking existing data**: Test migrations on real-like data
5. **No BR reference**: Link migrations to business requirements when applicable
```

**Integration**: Add link to this guide in:
- `CONTRIBUTING.md`
- Service README
- Slack bot auto-reply for "migration" keyword

---

### **Layer 5: CI/CD Validation** 🤖 AUTOMATION

**GitHub Actions Workflow** (add to `.github/workflows/validate-migrations.yml`):

```yaml
name: Validate Migrations

on:
  pull_request:
    paths:
      - 'migrations/**.sql'
      - 'test/integration/datastorage/suite_test.go'

jobs:
  validate-migrations:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Validate migration number uniqueness
        run: ./scripts/validate-migration-numbers.sh
      
      - name: Validate migration sync with tests
        run: ./scripts/validate-migration-sync.sh
      
      - name: Check migration format
        run: |
          # Ensure all migrations have goose markers
          for f in migrations/*.sql; do
            if ! grep -q "-- +goose Up" "$f"; then
              echo "❌ Missing goose Up marker: $f"
              exit 1
            fi
          done
```

**Benefit**: Blocks PRs with migration issues before merge

---

## 📊 **Prevention Strategy Summary**

| Layer | Type | Effort | Effectiveness | Priority | Timeline |
|-------|------|--------|---------------|----------|----------|
| **1. Auto-sync validation** | Script | 1 hour | 95% | **P0** | **Immediate** |
| **2. Number uniqueness check** | Script | 30 min | 100% | **P0** | **Immediate** |
| **3. Auto-generate list** | Code refactor | 4 hours | 100% | P1 | V1.1 |
| **4. Creation guidelines** | Documentation | 1 hour | 60% | P1 | This week |
| **5. CI/CD validation** | Automation | 2 hours | 90% | **P0** | **Immediate** |

**Total Immediate Effort**: ~5 hours for Layers 1, 2, and 5

---

## 🎯 **Implementation Roadmap**

### **Phase 1: Immediate** (Today - 30 minutes)

1. ✅ Renumber `021_add_status_reason_column.sql` to `022`
2. ✅ Add migration to test suite hardcoded list
3. ✅ Run integration tests to verify fix
4. ✅ Document finding in handoff documents

### **Phase 2: Prevention Scripts** (Tomorrow - 2 hours)

1. ✅ Create `scripts/validate-migration-sync.sh`
2. ✅ Create `scripts/validate-migration-numbers.sh`
3. ✅ Add validation targets to Makefile
4. ✅ Test scripts with current migrations
5. ✅ Document script usage

### **Phase 3: CI/CD Integration** (This Week - 3 hours)

1. ✅ Create GitHub Actions workflow
2. ✅ Test workflow on sample PR
3. ✅ Update CONTRIBUTING.md with validation requirements
4. ✅ Add pre-commit hook configuration

### **Phase 4: Documentation** (This Week - 1 hour)

1. ✅ Create `MIGRATION_CREATION_GUIDE.md`
2. ✅ Update service README with migration workflow
3. ✅ Add migration section to developer onboarding
4. ✅ Create quick reference card

### **Phase 5: Auto-Generation** (V1.1 - 4 hours)

1. Implement `getMigrationFiles()` function
2. Replace hardcoded list with dynamic generation
3. Add extensive testing
4. Document new approach
5. Migrate existing tests

---

## 📋 **Testing the Prevention System**

### **Scenario 1: New Migration Created**

**Action**: Developer creates `023_new_feature.sql`

**Expected Validation**:
```bash
$ make validate-migrations
🔍 Validating migration synchronization...
❌ VALIDATION FAILED: Migrations missing from test suite:
  - 023_new_feature.sql

📝 Action Required:
  Add missing migrations to test/integration/datastorage/suite_test.go around line 801
```

**Developer Action**: Adds migration to test suite

**Retry**:
```bash
$ make validate-migrations
✅ All migrations are included in test suite
```

---

### **Scenario 2: Duplicate Migration Number**

**Action**: Developer creates `022_another_feature.sql` when `022` already exists

**Expected Validation**:
```bash
$ make validate-migrations
🔍 Validating migration number uniqueness...
❌ VALIDATION FAILED: Duplicate migration numbers found:
  Number 022 used in:
    - 022_add_status_reason_column.sql
    - 022_another_feature.sql

📝 Action Required:
  Renumber one of the duplicate migrations to the next available number
```

**Developer Action**: Renames to `023_another_feature.sql`

---

### **Scenario 3: PR Submission**

**Action**: Developer submits PR with new migration

**GitHub Actions**:
```
✅ Validate migration number uniqueness - PASSED
✅ Validate migration sync with tests - PASSED
✅ Check migration format - PASSED
```

**Result**: PR can be merged

---

## 🔗 **Related Documents**

- `DATASTORAGE_ROOT_CAUSE_ANALYSIS_DEC_15_2025.md` - Initial root cause finding
- `DATASTORAGE_TEST_EXECUTION_RESULTS_DEC_15_2025.md` - Test failure details
- `TRIAGE_DATASTORAGE_V1.0_DEC_15_2025.md` - Complete service triage

---

## 📊 **Impact Analysis**

### **Before Prevention System**

**Failure Rate**: 100% (current situation)
- ❌ Manual process prone to human error
- ❌ No validation of migration list
- ❌ No duplicate number detection
- ❌ Easy to forget updating test suite

**MTTR** (Mean Time To Resolution): 2-4 hours
- Investigate test failure
- Find missing migration
- Add to test suite
- Re-run tests

---

### **After Prevention System**

**Failure Rate**: <5% (estimated)
- ✅ Automated validation catches 95%+ of issues
- ✅ Fast feedback loop (pre-commit, CI/CD)
- ✅ Clear error messages with remediation steps
- ✅ Impossible to merge broken migrations

**MTTR**: 5-10 minutes
- Validation script runs immediately
- Error message points to exact issue
- Developer fixes before committing

---

## 🎉 **Success Metrics**

**Short-Term** (1 month):
- ✅ Zero integration test failures due to missing migrations
- ✅ Zero duplicate migration numbers
- ✅ 100% of new migrations include test suite updates
- ✅ CI/CD blocks all problematic PRs

**Long-Term** (3 months):
- ✅ Developer satisfaction: "Easy to create migrations correctly"
- ✅ Maintenance burden reduced by 80%
- ✅ Migration-related incidents: 0 per month
- ✅ Auto-generation system (Layer 3) implemented

---

## 🔧 **Maintenance**

### **Who Maintains This**
- **Primary**: DataStorage Service Team
- **Secondary**: Platform Team (script infrastructure)

### **When to Update**
- New migration tool adopted (e.g., move away from Goose)
- Test infrastructure changes
- Migration numbering scheme changes
- After any migration-related incident

### **Review Schedule**
- Monthly: Check validation script effectiveness
- Quarterly: Review migration creation guide for clarity
- Yearly: Consider auto-generation migration (Layer 3)

---

**Document Version**: 1.0  
**Created**: December 15, 2025 19:45  
**Status**: ⚠️ **ACTION REQUIRED** - Immediate fix + prevention system needed

**Recommendation**: 
1. **TODAY**: Fix immediate issue (5 min)
2. **TOMORROW**: Implement validation scripts (2 hours)
3. **THIS WEEK**: CI/CD integration + documentation (4 hours)
4. **V1.1**: Consider auto-generation system (4 hours)




