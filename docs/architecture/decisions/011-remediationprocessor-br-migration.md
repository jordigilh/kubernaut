# RemediationProcessor BR Migration Mapping

**Date**: October 6, 2025
**Purpose**: Unify all BRs under BR-AP-* prefix (HIGH-1 resolution)
**Rationale**: Single prefix per service pattern for clear BR ownership

---

## Migration Strategy

**Target**: Unify all RemediationProcessor BRs under BR-AP-*
**Existing Range**: BR-AP-001 to BR-AP-050 (16 BRs)
**New Range**: BR-AP-051 to BR-AP-062 (6 BRs migrated)
**Total after migration**: BR-AP-001 to BR-AP-062 (22 BRs)

---

## BR-ENV-* → BR-AP-* Mapping (Following Gateway Pattern)

**Context**: Gateway Service set precedent by migrating BR-ENV-* → BR-GATEWAY-051 to 053.
RemediationProcessor follows the same pattern.

| Old BR (BR-ENV-*) | New BR (BR-AP-*) | Functionality |
|-------------------|------------------|---------------|
| BR-ENV-001 | BR-AP-051 | Environment detection from namespace labels |
| BR-ENV-009 | BR-AP-052 | Environment validation and classification |
| BR-ENV-050 | BR-AP-053 | Environment-specific configuration loading |

**Rationale**: Gateway Service established this migration pattern for environment classification BRs.

---

## BR-ALERT-* → BR-AP-* Mapping (Resolving Shared Prefix)

**Context**: BR-ALERT-* was shared between RemediationProcessor and RemediationOrchestrator,
creating ownership ambiguity. RemediationProcessor is the primary owner (first controller in pipeline).

| Old BR (BR-ALERT-*) | New BR (BR-AP-*) | Functionality | Conflict Resolution |
|---------------------|------------------|---------------|---------------------|
| BR-ALERT-003 | BR-AP-060 | Alert enrichment with K8s context | RemediationProcessor-specific |
| BR-ALERT-005 | BR-AP-061 | Alert correlation and deduplication | RemediationProcessor-specific |
| BR-ALERT-006 | BR-AP-062 | Alert timeout and escalation handling | **Shared with RemediationOrchestrator** → RemediationProcessor is primary owner |

**Rationale**:
- RemediationProcessor is the first controller in the remediation pipeline
- It handles initial alert processing, enrichment, and correlation
- BR-ALERT-006 (timeout/escalation) is part of alert processing logic
- RemediationOrchestrator will remove its BR-ALERT-006 reference (see HIGH-2)

---

## Complete BR-AP-* Range (After Migration)

| Range | Count | Category | Description |
|-------|-------|----------|-------------|
| BR-AP-001 to 050 | 16 BRs | Core Alert Processing | Alert ingestion, validation, transformation |
| BR-AP-051 to 053 | 3 BRs | Environment Classification | Migrated from BR-ENV-* |
| BR-AP-060 to 062 | 3 BRs | Alert Enrichment | Migrated from BR-ALERT-* |
| **Total V1** | **22 BRs** | | |
| BR-AP-063 to 180 | Reserved | V2 Expansion | Multi-source alerts, advanced correlation |

---

## Files to Update

### RemediationProcessor Documentation
1. **overview.md**: Update BR references, add "Business Requirements Coverage" section
2. **testing-strategy.md**: Update all test BR references
3. **controller-implementation.md**: Update code example BR references
4. **implementation-checklist.md**: Update BR range, add V1 scope clarification

---

## Migration Steps

### Step 1: Migrate BR-ENV-* References ✅
```bash
# Find all BR-ENV-* references
grep -r "BR-ENV-" docs/services/crd-controllers/01-remediationprocessor/ \
  --include="*.md" -l

# Replace each BR-ENV-* reference
sed -i '' 's/BR-ENV-001/BR-AP-051/g' [files]
sed -i '' 's/BR-ENV-009/BR-AP-052/g' [files]
sed -i '' 's/BR-ENV-050/BR-AP-053/g' [files]
```

### Step 2: Migrate BR-ALERT-* References
```bash
# Find all BR-ALERT-* references
grep -r "BR-ALERT-" docs/services/crd-controllers/01-remediationprocessor/ \
  --include="*.md" -l

# Replace each BR-ALERT-* reference
sed -i '' 's/BR-ALERT-003/BR-AP-060/g' [files]
sed -i '' 's/BR-ALERT-005/BR-AP-061/g' [files]
sed -i '' 's/BR-ALERT-006/BR-AP-062/g' [files]
```

### Step 3: Add BR Mapping to overview.md
Add "Business Requirements Coverage" section documenting all BR ranges.

### Step 4: Update implementation-checklist.md
Add V1 scope clarification with new BR range (BR-AP-001 to BR-AP-062).

### Step 5: Validate
```bash
# Verify no BR-ENV-* or BR-ALERT-* references remain
grep -r "BR-ENV-\|BR-ALERT-" \
  docs/services/crd-controllers/01-remediationprocessor/ \
  --include="*.md" --exclude="*MAPPING*"
# Expected: 0 matches
```

---

## Estimated Effort

- **Mapping Table**: ✅ Complete (30 minutes)
- **BR Migrations**: 1 hour (6 BRs across 4 files)
- **BR Mapping Documentation**: 30 minutes
- **V1 Scope Clarification**: 15 minutes

**Total**: ~2 hours

---

**Document Maintainer**: Kubernaut Documentation Team
**Migration Date**: October 6, 2025
**Status**: 🔄 **MAPPING COMPLETE - READY FOR MIGRATION**
