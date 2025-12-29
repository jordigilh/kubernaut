# DD-TEST-008 Filename Standardization

**Date**: December 23, 2025
**Status**: ✅ **COMPLETE**
**Context**: Corrected filename casing to match DD-TEST naming convention

---

## 🎯 Issue Identified

The DD-TEST-008 document was created with uppercase text in the filename:
- ❌ `DD-TEST-008-REUSABLE-E2E-COVERAGE-INFRASTRUCTURE.md`

This violated the established naming pattern used by other DD-TEST documents:
- ✅ `DD-TEST-001-port-allocation-strategy.md`
- ✅ `DD-TEST-002-parallel-test-execution-standard.md`
- ✅ `DD-TEST-007-e2e-coverage-capture-standard.md`

**Pattern**: `DD-TEST-XXX-lowercase-with-hyphens.md`

---

## 🔧 Fix Applied

### Two-Step Rename (macOS Case-Insensitive Filesystem)

On macOS, the filesystem is case-insensitive by default, so a direct rename from uppercase to lowercase won't work. Solution:

```bash
# Step 1: Rename to temporary name
mv DD-TEST-008-REUSABLE-E2E-COVERAGE-INFRASTRUCTURE.md DD-TEST-008-temp.md

# Step 2: Rename to target lowercase name
mv DD-TEST-008-temp.md DD-TEST-008-reusable-e2e-coverage-infrastructure.md
```

### Updated References

Found and updated all references across 3 files:

1. **`docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md`**
   - Updated all links from uppercase to lowercase (4 occurrences)

2. **`docs/handoff/DD_TEST_007_UPDATED_WITH_DD_TEST_008_DEC_23_2025.md`**
   - Updated all references from uppercase to lowercase (3 occurrences)

3. **`docs/handoff/REUSABLE_E2E_COVERAGE_INFRASTRUCTURE_DEC_23_2025.md`**
   - Updated all references from uppercase to lowercase (2 occurrences)

---

## ✅ Validation

### Filename Verification
```bash
$ ls -la docs/architecture/decisions/DD-TEST-008-*.md
-rw-r--r--@ 1 jgil  staff  11166 Dec 23 19:52 docs/architecture/decisions/DD-TEST-008-reusable-e2e-coverage-infrastructure.md
```
✅ Correct lowercase filename

### Reference Check
```bash
$ grep -r "DD-TEST-008-REUSABLE-E2E-COVERAGE-INFRASTRUCTURE" docs/
# No matches found
```
✅ No remaining uppercase references

### Naming Convention Compliance
```bash
$ ls docs/architecture/decisions/DD-TEST-*.md
DD-TEST-001-port-allocation-strategy.md
DD-TEST-002-parallel-test-execution-standard.md
DD-TEST-007-e2e-coverage-capture-standard.md
DD-TEST-008-reusable-e2e-coverage-infrastructure.md ✅
```
✅ All DD-TEST files now follow consistent naming pattern

---

## 📊 Impact

### Before
- ❌ Filename violated naming convention
- ❌ Inconsistent with other DD-TEST documents
- ❌ Could cause confusion for teams

### After
- ✅ Filename follows established pattern
- ✅ Consistent across all DD-TEST documents
- ✅ Clear and predictable naming for teams
- ✅ All references updated

---

## 🎓 Lessons Learned

### macOS Filesystem Considerations
- **Case-Insensitive**: Default HFS+ and APFS are case-insensitive
- **Two-Step Rename**: Required for case-only changes
- **Verification**: Always verify rename worked (`ls -la`)

### Naming Convention Enforcement
- **Pattern Recognition**: Identify patterns early (DD-TEST-XXX-lowercase)
- **Comprehensive Search**: Find all references before renaming
- **Batch Update**: Update all references in same commit

### Documentation Standards
- **Consistency**: Critical for large codebases
- **Discoverability**: Predictable naming helps teams find documents
- **Tooling**: Consider pre-commit hooks for naming validation

---

## 📝 Summary

**Corrected DD-TEST-008 filename** from uppercase (`REUSABLE-E2E-COVERAGE-INFRASTRUCTURE`) to lowercase (`reusable-e2e-coverage-infrastructure`) to match the established DD-TEST naming convention.

**Files Changed**:
- ✅ Renamed: `docs/architecture/decisions/DD-TEST-008-reusable-e2e-coverage-infrastructure.md`
- ✅ Updated: `docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md`
- ✅ Updated: `docs/handoff/DD_TEST_007_UPDATED_WITH_DD_TEST_008_DEC_23_2025.md`
- ✅ Updated: `docs/handoff/REUSABLE_E2E_COVERAGE_INFRASTRUCTURE_DEC_23_2025.md`

**Result**: All DD-TEST documents now follow consistent lowercase-with-hyphens naming pattern. ✅



