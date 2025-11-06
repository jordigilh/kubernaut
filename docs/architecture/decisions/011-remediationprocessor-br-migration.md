# RemediationProcessor BR Migration Mapping

**Date**: October 6, 2025
**Purpose**: Unify all BRs under BR-SP-* prefix (HIGH-1 resolution)
**Rationale**: Single prefix per service pattern for clear BR ownership

---

## Migration Strategy

**Target**: Unify all RemediationProcessor BRs under BR-SP-*
**Existing Range**: BR-SP-001 to BR-SP-050 (16 BRs)
**New Range**: BR-SP-051 to BR-SP-062 (6 BRs migrated)
**Total after migration**: BR-SP-001 to BR-SP-062 (22 BRs)

---

## BR-ENV-* → BR-SP-* Mapping (Following Gateway Pattern)

**Context**: Gateway Service set precedent by migrating BR-ENV-* → BR-GATEWAY-051 to 053.
RemediationProcessor follows the same pattern.

| Old BR (BR-ENV-*) | New BR (BR-SP-*) | Functionality |
|-------------------|------------------|---------------|
| BR-ENV-001 | BR-SP-051 | Environment detection from namespace labels |
| BR-ENV-009 | BR-SP-052 | Environment validation and classification |
| BR-ENV-050 | BR-SP-053 | Environment-specific configuration loading |

**Rationale**: Gateway Service established this migration pattern for environment classification BRs.

---

## BR-ALERT-* → BR-SP-* Mapping (Resolving Shared Prefix)

**Context**: BR-ALERT-* was shared between RemediationProcessor and RemediationOrchestrator,
creating ownership ambiguity. RemediationProcessor is the primary owner (first controller in pipeline).

| Old BR (BR-ALERT-*) | New BR (BR-SP-*) | Functionality | Conflict Resolution |
|---------------------|------------------|---------------|---------------------|
| BR-ALERT-003 | BR-SP-060 | Alert enrichment with K8s context | RemediationProcessor-specific |
| BR-ALERT-005 | BR-SP-061 | Alert correlation and deduplication | RemediationProcessor-specific |
| BR-ALERT-006 | BR-SP-062 | Alert timeout and escalation handling | **Shared with RemediationOrchestrator** → RemediationProcessor is primary owner |

**Rationale**:
- RemediationProcessor is the first controller in the remediation pipeline
- It handles initial alert processing, enrichment, and correlation
- BR-ALERT-006 (timeout/escalation) is part of alert processing logic
- RemediationOrchestrator will remove its BR-ALERT-006 reference (see HIGH-2)

---

## Complete BR-SP-* Range (After Migration)

| Range | Count | Category | Description |
|-------|-------|----------|-------------|
| BR-SP-001 to 050 | 16 BRs | Core Alert Processing | Alert ingestion, validation, transformation |
| BR-SP-051 to 053 | 3 BRs | Environment Classification | Migrated from BR-ENV-* |
| BR-SP-060 to 062 | 3 BRs | Alert Enrichment | Migrated from BR-ALERT-* |
| **Total V1** | **22 BRs** | | |
| BR-SP-063 to 180 | Reserved | V2 Expansion | Multi-source alerts, advanced correlation |

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
grep -r "BR-ENV-" docs/services/crd-controllers/01-signalprocessing/ \
  --include="*.md" -l

# Replace each BR-ENV-* reference
sed -i '' 's/BR-ENV-001/BR-SP-051/g' [files]
sed -i '' 's/BR-ENV-009/BR-SP-052/g' [files]
sed -i '' 's/BR-ENV-050/BR-SP-053/g' [files]
```

### Step 2: Migrate BR-ALERT-* References
```bash
# Find all BR-ALERT-* references
grep -r "BR-ALERT-" docs/services/crd-controllers/01-signalprocessing/ \
  --include="*.md" -l

# Replace each BR-ALERT-* reference
sed -i '' 's/BR-ALERT-003/BR-SP-060/g' [files]
sed -i '' 's/BR-ALERT-005/BR-SP-061/g' [files]
sed -i '' 's/BR-ALERT-006/BR-SP-062/g' [files]
```

### Step 3: Add BR Mapping to overview.md
Add "Business Requirements Coverage" section documenting all BR ranges.

### Step 4: Update implementation-checklist.md
Add V1 scope clarification with new BR range (BR-SP-001 to BR-SP-062).

### Step 5: Validate
```bash
# Verify no BR-ENV-* or BR-ALERT-* references remain
grep -r "BR-ENV-\|BR-ALERT-" \
  docs/services/crd-controllers/01-signalprocessing/ \
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
