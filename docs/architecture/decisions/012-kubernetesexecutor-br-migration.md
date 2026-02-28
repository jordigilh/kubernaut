# KubernetesExecutor BR Migration Mapping (DEPRECATED - ADR-025)

**Date**: October 6, 2025
**Purpose**: Migrate BR-KE-* → BR-EXEC-* for V2 multi-cloud readiness
**Rationale**: In V2, executor expands beyond Kubernetes to AWS, Azure, GCP. "EXEC" is cloud-agnostic.

---

## Migration Strategy

**Existing BR-EXEC-* Range**: BR-EXEC-001 to BR-EXEC-055 (12 BRs in use)
**New Range for migrated BRs**: BR-EXEC-060 to BR-EXEC-086 (27 BRs from BR-KE-*)
**Total after migration**: BR-EXEC-001 to BR-EXEC-086 (39 BRs)

---

## BR-KE-* → BR-EXEC-* Mapping Table

| Old BR (BR-KE-*) | New BR (BR-EXEC-*) | Functionality | Files Affected |
|------------------|--------------------|---------------|----------------|
| BR-KE-001 | BR-EXEC-060 | Safety validation | overview.md, testing-strategy.md |
| BR-KE-010 | BR-EXEC-061 | Job creation | overview.md, controller-implementation.md |
| BR-KE-011 | BR-EXEC-062 | Dry-run execution | overview.md, controller-implementation.md |
| BR-KE-012 | BR-EXEC-063 | Action catalog | overview.md, testing-strategy.md |
| BR-KE-013 | BR-EXEC-064 | RBAC isolation | overview.md, security-configuration.md |
| BR-KE-015 | BR-EXEC-065 | Rollback capability | overview.md, controller-implementation.md |
| BR-KE-016 | BR-EXEC-066 | Audit trail | overview.md, observability-logging.md |
| BR-KE-020 | BR-EXEC-067 | Action execution | controller-implementation.md |
| BR-KE-021 | BR-EXEC-068 | Timeout handling | controller-implementation.md |
| BR-KE-022 | BR-EXEC-069 | Job monitoring | controller-implementation.md |
| BR-KE-030 | BR-EXEC-070 | Resource validation | controller-implementation.md |
| BR-KE-031 | BR-EXEC-071 | Dependency resolution | controller-implementation.md |
| BR-KE-032 | BR-EXEC-072 | Error recovery | controller-implementation.md |
| BR-KE-033 | BR-EXEC-073 | State management | controller-implementation.md |
| BR-KE-034 | BR-EXEC-074 | Execution coordination | controller-implementation.md |
| BR-KE-035 | BR-EXEC-075 | Health monitoring | controller-implementation.md |
| BR-KE-036 | BR-EXEC-076 | Metrics collection | observability-logging.md |
| BR-KE-037 | BR-EXEC-077 | Event emission | observability-logging.md |
| BR-KE-038 | BR-EXEC-078 | Status updates | controller-implementation.md |
| BR-KE-039 | BR-EXEC-079 | Finalizer handling | controller-implementation.md |
| BR-KE-040 | BR-EXEC-080 | Owner reference management | controller-implementation.md |
| BR-KE-045 | BR-EXEC-081 | Integration testing | testing-strategy.md |
| BR-KE-046 | BR-EXEC-082 | E2E testing | testing-strategy.md |
| BR-KE-050 | BR-EXEC-083 | Security compliance | security-configuration.md |
| BR-KE-051 | BR-EXEC-084 | Authentication | security-configuration.md |
| BR-KE-052 | BR-EXEC-085 | Authorization | security-configuration.md |
| BR-KE-060 | BR-EXEC-086 | Multi-cluster execution | overview.md, controller-implementation.md |

**Total Migrations**: 27 BRs

---

## V2 Expansion Context

### Current State (V1)
**Service Name**: KubernetesExecutor
**Scope**: Kubernetes API operations only
**BR Prefix**: BR-EXEC-* (unified after migration)

### Future State (V2)
**Service Name**: Multi-Cloud Executor (or keep KubernetesExecutor with expanded scope)
**Scope**: Kubernetes + AWS + Azure + GCP infrastructure operations
**BR Prefix**: BR-EXEC-* (already cloud-agnostic)
**New BRs for V2**:
- BR-EXEC-100 to BR-EXEC-120: AWS infrastructure actions
- BR-EXEC-121 to BR-EXEC-140: Azure infrastructure actions
- BR-EXEC-141 to BR-EXEC-160: GCP infrastructure actions
- BR-EXEC-161 to BR-EXEC-180: Cross-cloud orchestration

### Why BR-EXEC-* Works for V2
✅ **Generic**: "Execution" applies to any infrastructure
✅ **Scalable**: Room for 180+ BRs
✅ **Clear**: Distinct from BR-EXECUTION-* (workflow monitoring)
✅ **Future-Proof**: No renaming needed when cloud providers added

### Why Not BR-KE-*
❌ **Kubernetes-Specific**: "KE" implies Kubernetes Executor
❌ **Misleading in V2**: BR-KE-* for AWS actions would be confusing
❌ **Requires V2 Migration**: Would need to rename again
❌ **Semantic Mismatch**: "KE" doesn't represent multi-cloud

---

## Clarification: BR-EXEC-* vs BR-EXECUTION-*

**BR-EXEC-* (KubernetesExecutor)**:
- Executes **individual infrastructure actions** (K8s operations, V2: cloud provider operations)
- Single-action scope (create pod, scale deployment, create AWS EC2 instance)
- Action-level execution and monitoring

**BR-EXECUTION-* (WorkflowExecution)**:
- Monitors **overall workflow execution progress** (multi-step health, workflow-level status)
- Workflow-level scope (entire remediation workflow across multiple steps)
- Workflow-level execution monitoring

**Example**:
- BR-EXEC-001: Execute Kubernetes scale deployment action
- BR-EXECUTION-001: Track overall workflow execution progress (5 steps, 3 completed, 2 pending)

---

## Migration Steps

### Step 1: Create Mapping Table ✅
This document

### Step 2: Update All Documentation
Files to update:
1. `overview.md` (13 BR-KE-* references)
2. `controller-implementation.md` (15 BR-KE-* references)
3. `testing-strategy.md` (5 BR-KE-* references)
4. `security-configuration.md` (3 BR-KE-* references)
5. `observability-logging.md` (2 BR-KE-* references)
6. `implementation-checklist.md` (BR range updates)
7. `integration-points.md` (if any BR-KE-* references)

### Step 3: Add Clarification Notes
Add to `overview.md`:
- Business Requirements Coverage section
- Clarification notes for BR-EXEC-* vs BR-EXECUTION-*
- V2 expansion plan

### Step 4: Validate
```bash
# After migration, verify no BR-KE-* remain
grep -r "BR-KE-" docs/services/crd-controllers/04-kubernetesexecutor/ \
  --include="*.md" --exclude="*MAPPING*.md"
# Expected: 0 matches
```

---

## Estimated Effort

- **Mapping Table**: ✅ Complete (30 minutes)
- **Documentation Updates**: 2-2.5 hours (27 BRs across 7 files)
- **Clarification Notes**: 30 minutes
- **Validation**: 15 minutes

**Total**: ~3-3.5 hours

---

**Document Maintainer**: Kubernaut Documentation Team
**Migration Date**: October 6, 2025
**Status**: 🔄 **MAPPING COMPLETE - READY FOR MIGRATION**
