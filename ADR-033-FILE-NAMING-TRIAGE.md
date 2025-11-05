# ADR-033 File Naming Triage

**Date**: 2025-11-05
**Scope**: ADR-033 related files and naming consistency
**Status**: ⚠️ **INCONSISTENT NAMING** (13 files with ADR-033 prefix)

---

## 🚨 **EXECUTIVE SUMMARY**

**Finding**: ⚠️ **NAMING INCONSISTENCY** - Multiple ADR-033 support documents in wrong locations
**Impact**: Medium - Organizational confusion, difficult to find related documents
**Root Cause**: Support documents created with ADR-033 prefix instead of proper naming

**Key Issues**:
1. ❌ **4 support documents** in `docs/architecture/decisions/` should be in subdirectories
2. ❌ **2 support documents** in `docs/services/stateless/data-storage/` lack proper prefixes
3. ✅ **Main ADR** correctly named: `ADR-033-remediation-playbook-catalog.md`
4. ✅ **Test files** correctly use lowercase `adr033` suffix
5. ✅ **Migration script** correctly uses lowercase `adr033` prefix

---

## 📋 **FILE INVENTORY**

### **Category 1: Main ADR Document** ✅ CORRECT

| File | Location | Status |
|------|----------|--------|
| `ADR-033-remediation-playbook-catalog.md` | `docs/architecture/decisions/` | ✅ CORRECT |

**Analysis**: Main ADR document follows standard naming convention

---

### **Category 2: Support Documents** ⚠️ INCORRECT LOCATION

| File | Current Location | Issue | Recommended Location |
|------|------------------|-------|---------------------|
| `ADR-033-BR-CATEGORY-MIGRATION-PLAN.md` | `docs/architecture/decisions/` | ❌ Wrong location | `docs/architecture/decisions/adr-033/` |
| `ADR-033-CROSS-SERVICE-BRS.md` | `docs/architecture/decisions/` | ❌ Wrong location | `docs/architecture/decisions/adr-033/` |
| `ADR-033-EXECUTOR-SERVICE-NAMING-ASSESSMENT.md` | `docs/architecture/decisions/` | ❌ Wrong location | `docs/architecture/decisions/adr-033/` |
| `ADR-033-NAMING-CONFIDENCE-ASSESSMENT.md` | `docs/architecture/decisions/` | ❌ Wrong location | `docs/architecture/decisions/adr-033/` |

**Analysis**: These are **supporting documents** for ADR-033, not standalone ADRs. They should be in a subdirectory.

---

### **Category 3: Implementation Documents** ⚠️ INCONSISTENT PREFIX

| File | Current Location | Issue | Recommended Name |
|------|------------------|-------|------------------|
| `ADR-033-IMPACT-ANALYSIS.md` | `docs/services/stateless/data-storage/` | ⚠️ Inconsistent | `DATA-STORAGE-ADR-033-IMPACT-ANALYSIS.md` |
| `ADR-033-MIGRATION-GUIDE.md` | `docs/services/stateless/data-storage/` | ⚠️ Inconsistent | `DATA-STORAGE-ADR-033-MIGRATION-GUIDE.md` |

**Analysis**: Service-specific documents should have service prefix for clarity

---

### **Category 4: Business Requirements** ✅ CORRECT

| File | Location | Status |
|------|----------|--------|
| `BR-STORAGE-031-03-schema-migration-adr033.md` | `docs/requirements/` | ✅ CORRECT |

**Analysis**: BR document correctly uses BR prefix with `adr033` suffix

---

### **Category 5: Database Migration** ✅ CORRECT

| File | Location | Status |
|------|----------|--------|
| `012_adr033_multidimensional_tracking.sql` | `migrations/` | ✅ CORRECT |

**Analysis**: Migration script correctly uses sequential number + `adr033` prefix

---

### **Category 6: Test Files** ✅ CORRECT

| File | Location | Status |
|------|----------|--------|
| `repository_adr033_test.go` | `test/unit/datastorage/` | ✅ CORRECT |
| `repository_adr033_integration_test.go` | `test/integration/datastorage/` | ✅ CORRECT |
| `aggregation_api_adr033_test.go` | `test/integration/datastorage/` | ✅ CORRECT |
| `test_adr033_migration.sh` | `.` (root) | ✅ CORRECT |

**Analysis**: Test files correctly use lowercase `adr033` suffix for feature grouping

---

## 🎯 **RECOMMENDED NAMING STANDARD**

### **Standard 1: Main ADR Documents**
**Location**: `docs/architecture/decisions/`
**Format**: `ADR-XXX-descriptive-name.md`
**Example**: `ADR-033-remediation-playbook-catalog.md` ✅

---

### **Standard 2: ADR Support Documents**
**Location**: `docs/architecture/decisions/adr-XXX/`
**Format**: `SUPPORT-DOCUMENT-NAME.md` (no ADR prefix)
**Examples**:
- `docs/architecture/decisions/adr-033/BR-CATEGORY-MIGRATION-PLAN.md`
- `docs/architecture/decisions/adr-033/CROSS-SERVICE-BRS.md`
- `docs/architecture/decisions/adr-033/EXECUTOR-SERVICE-NAMING-ASSESSMENT.md`
- `docs/architecture/decisions/adr-033/NAMING-CONFIDENCE-ASSESSMENT.md`

**Rationale**:
- Subdirectory groups related documents
- No ADR prefix needed (directory provides context)
- Easier to find all ADR-033 related documents

---

### **Standard 3: Service-Specific Implementation Documents**
**Location**: `docs/services/[service-type]/[service-name]/`
**Format**: `[SERVICE-NAME]-ADR-XXX-document-type.md`
**Examples**:
- `docs/services/stateless/data-storage/DATA-STORAGE-ADR-033-IMPACT-ANALYSIS.md`
- `docs/services/stateless/data-storage/DATA-STORAGE-ADR-033-MIGRATION-GUIDE.md`

**Rationale**:
- Service prefix clarifies scope
- ADR reference shows relationship
- Consistent with service documentation structure

---

### **Standard 4: Business Requirements**
**Location**: `docs/requirements/`
**Format**: `BR-[CATEGORY]-XXX-YY-description-adrXXX.md`
**Example**: `BR-STORAGE-031-03-schema-migration-adr033.md` ✅

**Rationale**:
- BR prefix takes precedence
- Lowercase `adrXXX` suffix shows relationship
- Maintains BR numbering system

---

### **Standard 5: Database Migrations**
**Location**: `migrations/`
**Format**: `XXX_adrYYY_description.sql`
**Example**: `012_adr033_multidimensional_tracking.sql` ✅

**Rationale**:
- Sequential number for migration order
- Lowercase `adrYYY` for feature grouping
- Standard Goose migration naming

---

### **Standard 6: Test Files**
**Location**: `test/[unit|integration]/[service]/`
**Format**: `feature_adrXXX_test.go`
**Examples**:
- `repository_adr033_test.go` ✅
- `aggregation_api_adr033_test.go` ✅

**Rationale**:
- Lowercase for Go file naming convention
- `adrXXX` suffix groups related tests
- Feature name provides context

---

## 📊 **CONFIDENCE ASSESSMENT**

### **Option A: Reorganize Now** ⭐ **RECOMMENDED**

**Confidence**: **90%**

**Actions**:
1. Create `docs/architecture/decisions/adr-033/` subdirectory
2. Move 4 support documents to subdirectory
3. Remove `ADR-033-` prefix from support documents
4. Rename 2 Data Storage documents with service prefix
5. Update all cross-references in documents

**Effort**: 30-45 minutes

**Pros**:
- ✅ Clear organization
- ✅ Easier to find related documents
- ✅ Consistent with industry standards (ADR subdirectories for complex decisions)
- ✅ Prevents future confusion
- ✅ Scales well for future ADRs with multiple support documents

**Cons**:
- ⚠️ Requires updating cross-references
- ⚠️ Git history shows file moves

**Recommended Structure**:
```
docs/architecture/decisions/
├── ADR-033-remediation-playbook-catalog.md (main ADR)
└── adr-033/
    ├── BR-CATEGORY-MIGRATION-PLAN.md
    ├── CROSS-SERVICE-BRS.md
    ├── EXECUTOR-SERVICE-NAMING-ASSESSMENT.md
    └── NAMING-CONFIDENCE-ASSESSMENT.md

docs/services/stateless/data-storage/
├── DATA-STORAGE-ADR-033-IMPACT-ANALYSIS.md
└── DATA-STORAGE-ADR-033-MIGRATION-GUIDE.md
```

---

### **Option B: Leave As-Is** ❌ **NOT RECOMMENDED**

**Confidence**: **10%**

**Pros**:
- ✅ No work required

**Cons**:
- ❌ Organizational confusion
- ❌ Difficult to find related documents
- ❌ Inconsistent with industry standards
- ❌ Sets bad precedent for future ADRs
- ❌ Clutters `docs/architecture/decisions/` directory

---

### **Option C: Minimal Cleanup** ⚠️ **PARTIAL SOLUTION**

**Confidence**: **50%**

**Actions**:
1. Create subdirectory for support documents only
2. Leave Data Storage documents as-is

**Effort**: 15-20 minutes

**Pros**:
- ✅ Improves main ADR directory organization
- ✅ Less work than Option A

**Cons**:
- ⚠️ Data Storage documents still inconsistent
- ⚠️ Partial solution

---

## 🔧 **RECOMMENDED REORGANIZATION PLAN**

### **Phase 1: Create Subdirectory** (5 minutes)

```bash
# Create ADR-033 subdirectory
mkdir -p docs/architecture/decisions/adr-033
```

---

### **Phase 2: Move Support Documents** (10 minutes)

```bash
cd docs/architecture/decisions

# Move and rename support documents
git mv ADR-033-BR-CATEGORY-MIGRATION-PLAN.md \
       adr-033/BR-CATEGORY-MIGRATION-PLAN.md

git mv ADR-033-CROSS-SERVICE-BRS.md \
       adr-033/CROSS-SERVICE-BRS.md

git mv ADR-033-EXECUTOR-SERVICE-NAMING-ASSESSMENT.md \
       adr-033/EXECUTOR-SERVICE-NAMING-ASSESSMENT.md

git mv ADR-033-NAMING-CONFIDENCE-ASSESSMENT.md \
       adr-033/NAMING-CONFIDENCE-ASSESSMENT.md
```

---

### **Phase 3: Rename Data Storage Documents** (5 minutes)

```bash
cd docs/services/stateless/data-storage

# Rename with service prefix
git mv ADR-033-IMPACT-ANALYSIS.md \
       DATA-STORAGE-ADR-033-IMPACT-ANALYSIS.md

git mv ADR-033-MIGRATION-GUIDE.md \
       DATA-STORAGE-ADR-033-MIGRATION-GUIDE.md
```

---

### **Phase 4: Update Cross-References** (15 minutes)

**Files to Update**:
1. `docs/architecture/DESIGN_DECISIONS.md` - Update ADR-033 index
2. `ADR-033-remediation-playbook-catalog.md` - Update internal references
3. `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V5.3.md` - Update references
4. Any BR documents referencing these files

**Search Pattern**:
```bash
# Find all references to moved files
grep -r "ADR-033-BR-CATEGORY-MIGRATION-PLAN" docs/
grep -r "ADR-033-CROSS-SERVICE-BRS" docs/
grep -r "ADR-033-EXECUTOR-SERVICE-NAMING-ASSESSMENT" docs/
grep -r "ADR-033-NAMING-CONFIDENCE-ASSESSMENT" docs/
grep -r "ADR-033-IMPACT-ANALYSIS" docs/
grep -r "ADR-033-MIGRATION-GUIDE" docs/
```

---

### **Phase 5: Commit Changes** (5 minutes)

```bash
git add docs/architecture/decisions/adr-033/
git add docs/services/stateless/data-storage/
git commit -m "docs: Reorganize ADR-033 support documents

Moved 4 ADR-033 support documents to subdirectory:
- ADR-033-BR-CATEGORY-MIGRATION-PLAN.md → adr-033/BR-CATEGORY-MIGRATION-PLAN.md
- ADR-033-CROSS-SERVICE-BRS.md → adr-033/CROSS-SERVICE-BRS.md
- ADR-033-EXECUTOR-SERVICE-NAMING-ASSESSMENT.md → adr-033/EXECUTOR-SERVICE-NAMING-ASSESSMENT.md
- ADR-033-NAMING-CONFIDENCE-ASSESSMENT.md → adr-033/NAMING-CONFIDENCE-ASSESSMENT.md

Renamed Data Storage documents with service prefix:
- ADR-033-IMPACT-ANALYSIS.md → DATA-STORAGE-ADR-033-IMPACT-ANALYSIS.md
- ADR-033-MIGRATION-GUIDE.md → DATA-STORAGE-ADR-033-MIGRATION-GUIDE.md

Rationale:
- Support documents belong in ADR subdirectory
- Service-specific documents need service prefix
- Improves organization and discoverability
- Consistent with industry standards for complex ADRs

Updated cross-references in:
- docs/architecture/DESIGN_DECISIONS.md
- ADR-033-remediation-playbook-catalog.md
- Implementation plans and BR documents

Confidence: 90% - Aligns with ADR best practices"
```

---

## 📊 **IMPACT ANALYSIS**

### **Files Requiring Updates** (Cross-References)

| File | Update Type | Effort |
|------|-------------|--------|
| `docs/architecture/DESIGN_DECISIONS.md` | Update ADR-033 index | 2 min |
| `ADR-033-remediation-playbook-catalog.md` | Update internal links | 3 min |
| `IMPLEMENTATION_PLAN_V5.3.md` | Update references | 3 min |
| BR documents | Update ADR references | 5 min |
| README files | Update documentation links | 2 min |

**Total Effort**: ~15 minutes

---

## 🔗 **INDUSTRY STANDARDS REFERENCE**

### **ADR Best Practices** (Michael Nygard, ThoughtWorks)

**Simple ADRs**: Single file in `docs/architecture/decisions/`
```
docs/architecture/decisions/
├── ADR-001-use-postgresql.md
├── ADR-002-use-redis-cache.md
└── ADR-003-use-kubernetes.md
```

**Complex ADRs**: Main file + subdirectory for support documents
```
docs/architecture/decisions/
├── ADR-033-remediation-playbook-catalog.md
└── adr-033/
    ├── analysis.md
    ├── alternatives.md
    └── implementation-notes.md
```

**Confidence**: **95%** - This is the industry standard for complex ADRs

---

## ✅ **FINAL RECOMMENDATION**

**Action**: ✅ **Option A - Reorganize Now** (90% confidence)

**Rationale**:
1. ✅ Aligns with industry standards for complex ADRs
2. ✅ Improves organization and discoverability
3. ✅ Prevents future confusion
4. ✅ Reasonable effort (30-45 minutes)
5. ✅ Sets good precedent for future ADRs

**Next Steps**:
1. Execute reorganization plan (Phases 1-5)
2. Update cross-references
3. Commit changes with comprehensive message
4. Update DESIGN_DECISIONS.md index

---

**Triage Completed By**: AI Assistant
**Triage Date**: 2025-11-05
**Recommendation**: **Reorganize Now** (90% confidence)

