# RemediationOrchestrator BR Migration Mapping

**Date**: October 6, 2025
**Purpose**: Unify all BRs under BR-AR-* prefix (HIGH-2 resolution)
**Rationale**: Single prefix per service pattern, resolving BR-ALERT-* shared prefix

---

## Migration Strategy

**Target**: Unify all RemediationOrchestrator BRs under BR-AR-*
**Existing Range**: BR-AR-001 to BR-AR-060 (18 BRs)
**New Range**: BR-AR-061 to BR-AR-067 (7 BRs migrated from BR-ALERT-*)
**Total after migration**: BR-AR-001 to BR-AR-067 (25 BRs)

---

## BR-ALERT-* → BR-AR-* Mapping

**Context**: BR-ALERT-* was shared between RemediationProcessor and RemediationOrchestrator.
HIGH-1 resolved this by migrating RemediationProcessor's BR-ALERT-* → BR-AP-*.
Now migrating RemediationOrchestrator's BR-ALERT-* → BR-AR-*.

| Old BR (BR-ALERT-*) | New BR (BR-AR-*) | Functionality | Notes |
|---------------------|------------------|---------------|-------|
| BR-ALERT-006 | *Removed* | Alert timeout/escalation | **Migrated to RemediationProcessor** (BR-AP-062) - RemediationProcessor is primary owner |
| BR-ALERT-021 | BR-AR-061 | CRD lifecycle monitoring | RemediationOrchestrator-specific |
| BR-ALERT-024 | BR-AR-062 | Status aggregation across CRDs | RemediationOrchestrator-specific |
| BR-ALERT-025 | BR-AR-063 | Event coordination between controllers | RemediationOrchestrator-specific |
| BR-ALERT-026 | BR-AR-064 | Cross-controller integration | RemediationOrchestrator-specific |
| BR-ALERT-027 | BR-AR-065 | Workflow orchestration | RemediationOrchestrator-specific |
| BR-ALERT-028 | BR-AR-066 | Remediation completion tracking | RemediationOrchestrator-specific |

**BR-ALERT-006 Resolution**:
- Originally shared by both controllers
- Primary owner: RemediationProcessor (first in pipeline, handles alert processing)
- Action: **Remove BR-ALERT-006 from RemediationOrchestrator documentation**
- Replacement: Reference RemediationProcessor's BR-AP-062 if needed

**Total Migrations**: 6 BRs + 1 removal = 7 changes

---

## Complete BR-AR-* Range (After Migration)

| Range | Count | Category | Description |
|-------|-------|----------|-------------|
| BR-AR-001 to 060 | 18 BRs | Core Remediation Orchestration | CRD lifecycle coordination |
| BR-AR-061 to 066 | 6 BRs | CRD Monitoring | Migrated from BR-ALERT-* |
| **Total V1** | **24 BRs** | | |
| BR-AR-067 to 180 | Reserved | V2 Expansion | Multi-cluster orchestration, advanced coordination |

---

## Files to Update

### RemediationOrchestrator Documentation
1. **overview.md**: Update BR references, add "Business Requirements Coverage" section, remove BR-ALERT-006
2. **testing-strategy.md**: Update all test BR references
3. **controller-implementation.md**: Update code example BR references
4. **implementation-checklist.md**: Update BR range, add V1 scope clarification

---

## Migration Steps

### Step 1: Remove BR-ALERT-006 References
```bash
# Find BR-ALERT-006 references in RemediationOrchestrator
grep -r "BR-ALERT-006" docs/services/crd-controllers/05-remediationorchestrator/ \
  --include="*.md" -n

# Remove or replace with reference to RemediationProcessor's BR-AP-062
```

### Step 2: Migrate BR-ALERT-* References
```bash
# Replace each BR-ALERT-* reference
sed -i '' 's/BR-ALERT-021/BR-AR-061/g' [files]
sed -i '' 's/BR-ALERT-024/BR-AR-062/g' [files]
sed -i '' 's/BR-ALERT-025/BR-AR-063/g' [files]
sed -i '' 's/BR-ALERT-026/BR-AR-064/g' [files]
sed -i '' 's/BR-ALERT-027/BR-AR-065/g' [files]
sed -i '' 's/BR-ALERT-028/BR-AR-066/g' [files]
```

### Step 3: Add BR Mapping to overview.md
Add "Business Requirements Coverage" section documenting all BR ranges.

### Step 4: Update implementation-checklist.md
Add V1 scope clarification with new BR range (BR-AR-001 to BR-AR-067).

### Step 5: Validate
```bash
# Verify no BR-ALERT-* references remain
grep -r "BR-ALERT-" \
  docs/services/crd-controllers/05-remediationorchestrator/ \
  --include="*.md" --exclude="*MAPPING*"
# Expected: 0 matches
```

---

## Estimated Effort

- **Mapping Table**: ✅ Complete (30 minutes)
- **BR Migrations**: 1 hour (7 changes across 4 files)
- **BR Mapping Documentation**: 30 minutes
- **V1 Scope Clarification**: 15 minutes

**Total**: ~2 hours

---

**Document Maintainer**: Kubernaut Documentation Team
**Migration Date**: October 6, 2025
**Status**: 🔄 **MAPPING COMPLETE - READY FOR MIGRATION**
