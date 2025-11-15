# Documentation Reorganization Proposal

**Date**: November 15, 2025
**Purpose**: Archive obsolete and superseded documentation to maintain relevance in `docs/`

---

## Executive Summary

**Total Documents**: ~1,207 markdown files
**Proposed for Archival**: ~150+ documents (12-15% of total)
**Structure**: Mirror existing `docs/` structure in `docs/archived/`

---

## Archival Categories

### CATEGORY 1: EXPLICITLY DEPRECATED/SUPERSEDED (HIGH PRIORITY)
**Impact**: 9 documents
**Confidence**: 100% - Marked as DEPRECATED

#### Architecture Documents (3 files)
```
docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md
  → docs/archived/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md
  Reason: SUPERSEDED by KUBERNAUT_CRD_ARCHITECTURE.md (marked DEPRECATED)

docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE_TRIAGE.md
  → docs/archived/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE_TRIAGE.md
  Reason: Analysis of deprecated document

docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE_DEPRECATION_ASSESSMENT.md
  → docs/archived/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE_DEPRECATION_ASSESSMENT.md
  Reason: Analysis of deprecated document
```

#### CRD Design Documents (6 files)
```
docs/design/CRD/*.md (all 6 files, excluding archive/)
  → docs/archived/design/CRD/*.md
  Reason: Superseded by docs/services/crd-controllers/*/overview.md specifications
  Note: docs/design/CRD/archive/ already exists, merge into archived/
```

---

### CATEGORY 2: KUBERNETESEXECUTOR SERVICE (HIGH PRIORITY)
**Impact**: 20+ documents
**Confidence**: 100% - Service eliminated per ADR-025

```
docs/services/crd-controllers/04-kubernetesexecutor/ (entire directory)
  → docs/archived/services/crd-controllers/04-kubernetesexecutor/
  Reason: Service eliminated per ADR-025 (Tekton Pipelines replaced it)
  Includes: overview.md, implementation plans, business requirements, tests
```

---

### CATEGORY 3: ANALYSIS DOCUMENTS (MEDIUM PRIORITY)
**Impact**: 61 documents
**Confidence**: 90% - Temporary/ephemeral analysis

**Rationale**: Analysis documents are typically created for specific decisions/triages and become obsolete after implementation.

**Recommendation**: Archive all analysis documents EXCEPT:
- Documents explicitly referenced by current ADRs/DDs
- Documents marked as "living" or "ongoing"

**Proposed Action**:
```
docs/analysis/*.md (all root-level files)
  → docs/archived/analysis/*.md
  Exceptions: (to be identified during review)
```

**Sample Documents to Archive**:
- MULTI_STEP_WORKFLOW_EXAMPLES.md
- OPTION_A_WORKAROUND_ASSESSMENT.md
- TASK_2_2_ORCHESTRATOR_MAPPING_GUIDE.md
- UNMAPPED_CODE_BR_VALIDATION.md
- APPROVAL_WORKFLOW_TIMING_AND_CRD_CREATION.md
- (56 more...)

---

### CATEGORY 4: TRIAGE DOCUMENTS (MEDIUM PRIORITY)
**Impact**: 31 documents
**Confidence**: 95% - Temporary investigation documents

**Rationale**: Triage documents are point-in-time investigations that become historical after resolution.

**Proposed Action**:
```
docs/**/*TRIAGE*.md (all files with TRIAGE in name, not already in archive/)
  → docs/archived/{same-path}/*TRIAGE*.md
```

**Sample Documents to Archive**:
- CRD_DATA_FLOW_TRIAGE_REMEDIATION_TO_AI.md
- APPROVED_MICROSERVICES_ARCHITECTURE_TRIAGE_REPORT.md
- OBSOLETE-DOCUMENTATION-TRIAGE.md
- ALERT_TO_SIGNAL_NAMING_TRIAGE_REMAINING.md
- (27 more...)

---

### CATEGORY 5: LEGACY CLEANUP DOCUMENTS (LOW PRIORITY)
**Impact**: 1 document
**Confidence**: 100%

```
docs/legacy-cleanup/ (entire directory)
  → docs/archived/legacy-cleanup/
  Reason: Legacy cleanup is complete, documentation is historical
```

---

### CATEGORY 6: PRE-IMPLEMENTATION DESIGN DOCUMENTS (LOW PRIORITY - REVIEW REQUIRED)
**Impact**: 12 documents
**Confidence**: 60% - Requires case-by-case review

**Rationale**: Design documents may still be valuable for understanding design decisions, even if implementation is complete.

**Recommendation**: Manual review required for each document.

**Candidates for Archival**:
```
docs/design/resilient-workflow-engine-design.md
  → Review: If superseded by ADR-035 and current implementation

docs/design/IMPLEMENTATION_PLAN_TAINT_ACTIONS.md
  → Review: If taint actions are implemented or abandoned

docs/design/ACTION_PARAMETER_SCHEMAS.md
  → Review: If superseded by DD-PLAYBOOK-003

docs/design/STRUCTURED_ACTION_FORMAT_IMPLEMENTATION_PLAN.md
  → Review: If format is now implemented
```

---

### CATEGORY 7: DOCUMENTS IN EXISTING ARCHIVE DIRECTORIES (CONSOLIDATION)
**Impact**: ~50 documents
**Confidence**: 100%

**Current State**: Multiple archive directories exist:
- `docs/design/CRD/archive/`
- `docs/analysis/archive/triage/`
- `docs/services/crd-controllers/*/implementation/archive/`

**Proposed Action**: Consolidate all into `docs/archived/` with mirrored structure.

---

## Proposed Directory Structure

```
docs/
├── archived/                          # NEW: Centralized archive
│   ├── architecture/                  # Archived architecture docs
│   │   ├── MULTI_CRD_RECONCILIATION_ARCHITECTURE.md
│   │   ├── MULTI_CRD_RECONCILIATION_ARCHITECTURE_TRIAGE.md
│   │   └── MULTI_CRD_RECONCILIATION_ARCHITECTURE_DEPRECATION_ASSESSMENT.md
│   ├── design/                        # Archived design docs
│   │   └── CRD/                       # Consolidated CRD design docs
│   │       ├── 01_REMEDIATION_REQUEST_CRD.md
│   │       ├── 02_REMEDIATION_PROCESSING_CRD.md
│   │       ├── 03_AI_ANALYSIS_CRD.md
│   │       ├── 04_WORKFLOW_EXECUTION_CRD.md
│   │       ├── 05_KUBERNETES_EXECUTION_CRD.md
│   │       └── README.md
│   ├── services/                      # Archived service docs
│   │   └── crd-controllers/
│   │       └── 04-kubernetesexecutor/ # Entire eliminated service
│   ├── analysis/                      # Archived analysis docs (61 files)
│   │   └── triage/                    # Consolidated triage docs
│   ├── legacy-cleanup/                # Archived legacy cleanup docs
│   └── README.md                      # Archive index and navigation
├── architecture/                      # ACTIVE architecture docs
├── services/                          # ACTIVE service docs
├── requirements/                      # ACTIVE requirements
└── ... (other active directories)
```

---

## Archive README.md Template

```markdown
# Archived Documentation

**Purpose**: Historical documentation for reference and context.

**⚠️ WARNING**: Documents in this directory are **OBSOLETE** or **SUPERSEDED**. Do not use for current development.

## Archive Categories

### Architecture
- **MULTI_CRD_RECONCILIATION_ARCHITECTURE.md**: Superseded by KUBERNAUT_CRD_ARCHITECTURE.md (2025-10-20)

### Services
- **04-kubernetesexecutor/**: Service eliminated per ADR-025 (2025-11-05)

### Design
- **CRD/*.md**: Superseded by service specifications in docs/services/

### Analysis
- **61 analysis documents**: Temporary analysis completed (various dates)

### Triage
- **31 triage documents**: Point-in-time investigations completed (various dates)

## How to Use This Archive

1. **For Historical Context**: Understand past decisions and evolution
2. **For Archaeology**: Trace why certain decisions were made
3. **For Learning**: See what approaches were tried and abandoned

## Do NOT Use For

- ❌ Current development guidance
- ❌ Architecture decisions (use ADRs/DDs instead)
- ❌ Implementation patterns (use active service docs)
```

---

## Implementation Plan

### Phase 1: High Priority (Immediate)
1. Archive MULTI_CRD_RECONCILIATION_ARCHITECTURE.md and related docs (3 files)
2. Archive docs/services/crd-controllers/04-kubernetesexecutor/ (20 files)
3. Archive docs/design/CRD/*.md (6 files)
4. Archive docs/legacy-cleanup/ (1 file)

**Total Phase 1**: 30 documents

### Phase 2: Medium Priority (Review & Archive)
1. Review and archive analysis documents (61 files)
2. Review and archive triage documents (31 files)

**Total Phase 2**: 92 documents

### Phase 3: Low Priority (Manual Review)
1. Review design documents case-by-case (12 files)
2. Consolidate existing archive directories (50 files)

**Total Phase 3**: 62 documents

### Phase 4: Documentation
1. Create docs/archived/README.md with archive index
2. Update docs/README.md to reference archive
3. Add git commit documenting reorganization

---

## Verification Checklist

After reorganization:
- [ ] All archived documents maintain original directory structure
- [ ] docs/archived/README.md created with index
- [ ] No broken links in active documentation
- [ ] Git history preserved (use `git mv` not `mv`)
- [ ] Commit message documents reorganization rationale
- [ ] Active docs/ directory contains only relevant documentation

---

## Estimated Impact

**Before**: ~1,207 documents in docs/
**After**: ~1,050 active documents, ~150 archived documents
**Reduction**: 12-15% reduction in active documentation
**Benefit**: Improved discoverability and relevance of active docs

---

## Risks & Mitigations

**Risk 1**: Accidentally archive active documentation
- **Mitigation**: Manual review of each category before archival
- **Mitigation**: Test phase with high-priority documents first

**Risk 2**: Broken links in active documentation
- **Mitigation**: Search for references before archiving
- **Mitigation**: Update links if found

**Risk 3**: Loss of valuable historical context
- **Mitigation**: Archive (don't delete) - all content remains accessible
- **Mitigation**: Create comprehensive README.md in archive

---

## Approval Required

Please review and approve:
1. ✅/❌ Overall reorganization approach
2. ✅/❌ Proposed directory structure (docs/archived/)
3. ✅/❌ Category 1: Deprecated documents (9 files)
4. ✅/❌ Category 2: KubernetesExecutor service (20 files)
5. ✅/❌ Category 3: Analysis documents (61 files)
6. ✅/❌ Category 4: Triage documents (31 files)
7. ✅/❌ Category 5: Legacy cleanup (1 file)
8. ✅/❌ Category 6: Design documents (12 files - requires review)
9. ✅/❌ Category 7: Consolidate existing archives (50 files)
10. ✅/❌ Implementation plan (3 phases)

---

**Next Steps After Approval**:
1. Execute Phase 1 (high priority, 30 documents)
2. Provide summary and request approval for Phase 2
3. Continue with remaining phases

