# KubernetesExecutor Documentation Update Plan

**Date**: 2025-10-19
**Version**: 1.0.0
**Decision**: [ADR-025: KubernetesExecutor Service Elimination](./ADR-025-kubernetesexecutor-service-elimination.md)
**Status**: 📋 **Awaiting Approval**

---

## Executive Summary

**Total Files Requiring Updates**: **57 files** across 5 categories
**Estimated Effort**: **14-16 hours** (2 engineering days)
**Priority**: 🔴 **Critical** (architectural consistency)

This plan details all documentation updates required to reflect the elimination of the KubernetesExecutor service and adoption of Tekton Pipelines.

---

## Update Strategy

### **Phased Approach** (Recommended)

| Phase | Category | Files | Effort | Dependencies |
|-------|----------|-------|--------|--------------|
| **Phase 1** | Architecture & README | 12 files | 4-5 hours | None - START HERE |
| **Phase 2** | Service Specifications | 18 files | 5-6 hours | Phase 1 complete |
| **Phase 3** | CRD Design Docs | 8 files | 2-3 hours | Phase 1 complete |
| **Phase 4** | Analysis & Planning | 14 files | 2-3 hours | Phases 1-2 complete |
| **Phase 5** | Supporting Docs | 5 files | 1 hour | Phases 1-4 complete |

**Total**: 57 files, 14-17 hours

---

## Phase 1: Architecture & README (Critical Path)

### **Priority**: 🔴 **Critical** (4-5 hours)

These files define the authoritative architecture and are the primary reference for all developers.

| # | File | Type | Changes Required | Effort |
|---|------|------|------------------|--------|
| **1** | `README.md` | Architecture | Remove KubernetesExecutor from service list, update sequence diagram, update RBAC | 45 min |
| **2** | `docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md` | Architecture | Update service list (12→11), remove KubernetesExecutor controller, update all sequence diagrams | 60 min |
| **3** | `docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md` | Architecture | Remove KubernetesExecution CRD from flow, update reconciliation patterns | 45 min |
| **4** | `docs/architecture/CRD_SCHEMAS.md` | CRD Spec | Remove KubernetesExecution CRD schema, update WorkflowExecution references | 30 min |
| **5** | `docs/architecture/CRD_FIELD_NAMING_CONVENTION.md` | CRD Spec | Remove KubernetesExecution field examples | 15 min |
| **6** | `docs/architecture/SERVICE_DEPENDENCY_MAP.md` | Architecture | Remove KubernetesExecutor from dependency graph, update phase diagrams | 30 min |
| **7** | `docs/architecture/DESIGN_DECISIONS.md` | Architecture | Update DD-002 to reflect Tekton execution, no KubernetesExecutor validation | 20 min |
| **8** | `docs/architecture/NAMESPACE_STRATEGY.md` | Operations | Remove KubernetesExecutor deployment checklist | 10 min |
| **9** | `docs/architecture/PROMETHEUS_ALERTRULES.md` | Operations | Remove 5 KubernetesExecutor alert rules | 15 min |
| **10** | `docs/architecture/LOG_CORRELATION_ID_STANDARD.md` | Operations | Remove KubernetesExecution CRD from correlation flow | 10 min |
| **11** | `docs/architecture/CRITICAL_4_CRD_RETENTION_COMPLETE.md` | Architecture | Remove KubernetesExecution from CRD retention policy | 15 min |
| **12** | `docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE_TRIAGE.md` | Reference | Update service count, remove KubernetesExecutor references | 20 min |

**Total Phase 1**: 4 hours 35 minutes

---

## Phase 2: Service Specifications (High Priority)

### **Priority**: 🟡 **High** (5-6 hours)

These files define how services integrate with the execution layer.

### **2.1: WorkflowExecution Service** (3 hours)

| # | File | Changes Required | Effort |
|---|------|------------------|--------|
| **13** | `docs/services/crd-controllers/03-workflowexecution/README.md` | Remove KubernetesExecution references, add Tekton integration | 30 min |
| **14** | `docs/services/crd-controllers/03-workflowexecution/overview.md` | ✅ **COMPLETE** (already updated to v2.0.0) | 0 min |
| **15** | `docs/services/crd-controllers/03-workflowexecution/reconciliation-phases.md` | Remove KubernetesExecution creation, add Tekton PipelineRun creation | 45 min |
| **16** | `docs/services/crd-controllers/03-workflowexecution/controller-implementation.md` | Replace KubernetesExecution code examples with Tekton PipelineRun | 60 min |
| **17** | `docs/services/crd-controllers/03-workflowexecution/integration-points.md` | ✅ **PARTIALLY COMPLETE** (main section updated, structured actions pending) | 20 min |
| **18** | `docs/services/crd-controllers/03-workflowexecution/crd-schema.md` | Remove KubernetesExecutionRef fields, add PipelineRunRef | 30 min |
| **19** | `docs/services/crd-controllers/03-workflowexecution/implementation/IMPLEMENTATION_PLAN_V1.0.md` | Replace KubernetesExecution creation logic with Tekton translation | 45 min |

### **2.2: RemediationOrchestrator Service** (1.5 hours)

| # | File | Changes Required | Effort |
|---|------|------------------|--------|
| **20** | `docs/services/crd-controllers/05-remediationorchestrator/overview.md` | Remove KubernetesExecution CRD creation logic | 30 min |
| **21** | `docs/services/crd-controllers/05-remediationorchestrator/reconciliation-phases.md` | Update child CRD list (5→4 CRDs) | 20 min |
| **22** | `docs/services/crd-controllers/05-remediationorchestrator/implementation/IMPLEMENTATION_PLAN_V1.0.md` | Remove KubernetesExecution watch and creation | 40 min |

### **2.3: Data Storage Service** (0.5 hours)

| # | File | Changes Required | Effort |
|---|------|------------------|--------|
| **23** | `docs/services/stateless/data-storage/overview.md` | Update action record creation source (WorkflowExecution, not KubernetesExecutor) | 15 min |
| **24** | `docs/services/stateless/data-storage/api-specification.md` | Update action record schema (remove KubernetesExecution references) | 15 min |

### **2.4: Other Services** (1 hour)

| # | File | Changes Required | Effort |
|---|------|------------------|--------|
| **25** | `docs/services/crd-controllers/01-remediationprocessor/overview.md` | Remove KubernetesExecution from flow description | 15 min |
| **26** | `docs/services/crd-controllers/02-aianalysis/overview.md` | Remove KubernetesExecution from downstream references | 15 min |
| **27** | `docs/services/stateless/effectiveness-monitor/overview.md` | Update action tracking (query Data Storage, not KubernetesExecution CRDs) | 20 min |
| **28** | `docs/services/crd-controllers/06-notification/overview.md` | No changes (Notification is independent) | 0 min |
| **29** | `docs/services/stateless/gateway/README.md` | Update downstream CRD list (RemediationRequest only) | 10 min |
| **30** | `docs/services/stateless/context-api/overview.md` | No changes (Context API is independent) | 0 min |

**Total Phase 2**: 5 hours 25 minutes

---

## Phase 3: CRD Design Documents (Medium Priority)

### **Priority**: 🟡 **Medium** (2-3 hours)

These are reference documents that describe CRD designs.

| # | File | Type | Changes Required | Effort |
|---|------|------|------------------|--------|
| **31** | `docs/design/CRD/01_REMEDIATION_REQUEST_CRD.md` | Design | Remove KubernetesExecution from orchestration description | 20 min |
| **32** | `docs/design/CRD/02_REMEDIATION_PROCESSING_CRD.md` | Design | Remove KubernetesExecution references | 15 min |
| **33** | `docs/design/CRD/04_WORKFLOW_EXECUTION_CRD.md` | Design | Replace KubernetesExecution output with Tekton PipelineRun | 30 min |
| **34** | `docs/design/CRD/05_KUBERNETES_EXECUTION_CRD.md` | Design | **ARCHIVE** - Add deprecation notice, link to ADR-025 | 20 min |
| **35** | `docs/design/CRD/archive/01_REMEDIATION_REQUEST_CRD.md` | Archive | Add deprecation notice | 10 min |
| **36** | `docs/design/CRD/archive/02_REMEDIATION_PROCESSING_CRD.md` | Archive | Add deprecation notice | 10 min |
| **37** | `docs/design/CRD/archive/04_WORKFLOW_EXECUTION_CRD.md` | Archive | Add deprecation notice | 10 min |
| **38** | `docs/design/CRD/archive/05_KUBERNETES_EXECUTION_CRD.md` | Archive | Add deprecation notice | 10 min |

**Total Phase 3**: 2 hours 5 minutes

---

## Phase 4: Analysis & Planning Documents (Low Priority)

### **Priority**: 🟢 **Low** (2-3 hours)

These are historical analysis documents that can be marked as reference-only.

| # | File | Type | Action | Effort |
|---|------|------|--------|--------|
| **39** | `docs/analysis/CRD_DATA_FLOW_COMPREHENSIVE_SUMMARY.md` | Analysis | Add "Historical Reference" notice, link to Tekton architecture | 15 min |
| **40** | `docs/analysis/CRD_DESIGN_DOCUMENTS_COMPREHENSIVE_TRIAGE.md` | Analysis | Mark KubernetesExecution validation as replaced by Tekton | 15 min |
| **41** | `docs/analysis/OPTION_A_WORKAROUND_ASSESSMENT.md` | Analysis | Add "Superseded by Tekton" notice | 10 min |
| **42** | `docs/architecture/ARCHITECTURAL_RISKS_FINAL_SUMMARY.md` | Analysis | Update parallel execution (Tekton TaskRuns, not KubernetesExecution CRDs) | 20 min |
| **43** | `docs/architecture/ARCHITECTURAL_RISKS_MITIGATION_SUMMARY.md` | Analysis | Same as above | 20 min |
| **44** | `docs/architecture/ARCHITECTURE_TRIAGE_V1_INTEGRATION_GAPS_RISKS.md` | Analysis | Remove KubernetesExecution from implementation roadmap | 25 min |
| **45** | `docs/architecture/FAILURE_RECOVERY_DOCUMENTATION_TRIAGE.md` | Analysis | Remove KubernetesExecutor failure context task | 10 min |
| **46** | `docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md` | Design | Replace KubernetesExecution with Tekton TaskRun | 20 min |
| **47** | `docs/architecture/SCENARIO_A_RECOVERABLE_FAILURE_SEQUENCE.md` | Design | Same as above | 20 min |
| **48** | `docs/architecture/STEP_FAILURE_RECOVERY_ARCHITECTURE.md` | Design | Update to Tekton-based recovery | 25 min |
| **49** | `docs/architecture/TRIAGE_EXECUTIVE_SUMMARY.md` | Summary | Remove KubernetesExecutor task from checklist | 10 min |
| **50** | `docs/planning/SERVICE_DEVELOPMENT_ORDER_STRATEGY.md` | Planning | Remove KubernetesExecutor from Phase 3, update timeline | 20 min |

**Total Phase 4**: 3 hours 30 minutes

---

## Phase 5: Supporting Documents (Minimal Priority)

### **Priority**: 🟢 **Minimal** (1 hour)

| # | File | Type | Action | Effort |
|---|------|------|--------|--------|
| **51** | `docs/testing/README.md` | Testing | Remove KubernetesExecutor from service list | 10 min |
| **52** | `docs/V1_SOURCE_OF_TRUTH_HIERARCHY.md` | Meta | Update authoritative document list (remove KubernetesExecutor) | 15 min |
| **53** | `docs/analysis/V1_DOCUMENTATION_TRIAGE_REPORT.md` | Meta | Add note about KubernetesExecutor deprecation | 15 min |
| **54** | `.github/workflows/ci.yml` | CI/CD | Remove KubernetesExecutor build/test jobs (if any) | 10 min |
| **55** | `Makefile` | Build | Remove KubernetesExecutor targets (if any) | 10 min |

**Total Phase 5**: 1 hour

---

## Sequence Diagram Updates

### **Files with Sequence Diagrams**: 6 files

| File | Diagram Type | Changes Required |
|------|--------------|------------------|
| `README.md` | Mermaid | Remove `KubernetesExecutor` participant, update flow to end at `WorkflowExecution → Tekton` |
| `docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md` | Mermaid (3 diagrams) | Remove KubernetesExecutor from all 3 sequence diagrams |
| `docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md` | Mermaid | Replace KubernetesExecution creation with Tekton PipelineRun |
| `docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md` | Mermaid | Replace KubernetesExecutor with WorkflowExecution → Tekton pattern |
| `docs/architecture/SCENARIO_A_RECOVERABLE_FAILURE_SEQUENCE.md` | Mermaid | Same as above |
| `docs/architecture/SERVICE_DEPENDENCY_MAP.md` | Mermaid (2 diagrams) | Remove KubernetesExecutor node from both dependency graphs |

**Diagram Update Pattern**:
```mermaid
# OLD
participant WF as WorkflowExecution
participant KE as KubernetesExecutor

WF->>KE: Create KubernetesExecution CRD
KE->>K8S: Apply remediation

# NEW
participant WF as WorkflowExecution
participant TEKTON as Tekton Pipelines

WF->>TEKTON: Create PipelineRun
TEKTON->>K8S: Create TaskRun → Pod
```

---

## Automated Search & Replace Patterns

### **Pattern 1: Service Count**
```bash
# Find and replace service counts
sed -i 's/12 services/11 services/g' docs/**/*.md
sed -i 's/5 CRD controllers/4 CRD controllers/g' docs/**/*.md
```

### **Pattern 2: CRD List**
```bash
# Remove KubernetesExecution from CRD lists
sed -i 's/, KubernetesExecution//g' docs/**/*.md
sed -i 's/KubernetesExecution, //g' docs/**/*.md
```

### **Pattern 3: Service List**
```bash
# Remove KubernetesExecutor from service lists
sed -i '/KubernetesExecutor/d' docs/**/*.md
```

**Note**: These are illustrative - actual updates require manual review for context.

---

## Critical Files Requiring Special Attention

### **README.md** 🔴 Critical

**Changes**:
1. Line 13: Remove KubernetesExecutor from pending services
2. Line 51: Remove KubernetesExecution from CRD schemas list
3. Line 74: Remove KubernetesExecutor row from CRD Controllers table
4. Lines 131-202: Update V1 Service Communication Architecture sequence diagram
5. Lines 282-337: Remove KubernetesExecutor from Planned V1 Features
6. Line 357: Update bin/manager comment (4 controllers, not 5)
7. Line 470: Remove KubernetesExecutor from service list
8. Line 494: Remove KubernetesExecutor from RBAC rules

**Estimated Effort**: 45 minutes

---

### **APPROVED_MICROSERVICES_ARCHITECTURE.md** 🔴 Critical

**Changes**:
1. Service count: 12 → 11 services, 5 → 4 CRD controllers
2. Remove KubernetesExecutor from service descriptions (3 locations)
3. Update 3 sequence diagrams:
   - Basic Flow (remove KubernetesExecutor participant)
   - Parallel Workflow (remove KubernetesExecution CRD creation)
   - CRD Ownership (remove KubernetesExecution from hierarchy)
4. Remove KubernetesExecution from CRD-per-Step Pattern description
5. Remove KubernetesExecution from Owner References list

**Estimated Effort**: 60 minutes

---

### **MULTI_CRD_RECONCILIATION_ARCHITECTURE.md** 🔴 Critical

**Changes**:
1. Remove Section 5: "KubernetesExecution CRD (Executor Service)"
2. Update CRD flow diagram (4 CRDs, not 5)
3. Update RemediationOrchestrator example (remove createKubernetesExecution)
4. Update sequence diagram (remove KubernetesExecutor participant)
5. Update reconciliation phases (4 phases, not 5)
6. Update Week 5-6 timeline (remove KubernetesExecution)

**Estimated Effort**: 45 minutes

---

## Validation Checklist

After all updates, verify:

- [ ] ✅ Zero references to `KubernetesExecutor` (except in DEPRECATED.md and ADR-025)
- [ ] ✅ Zero references to `KubernetesExecution` (except in historical/archive docs)
- [ ] ✅ Zero references to `ActionExecution` (except in ADR-024 and elimination docs)
- [ ] ✅ All sequence diagrams show Tekton architecture
- [ ] ✅ All RBAC examples use Tekton API permissions
- [ ] ✅ All service counts updated (12→11, 5→4)
- [ ] ✅ All CRD lists updated (5→4 CRDs)
- [ ] ✅ README.md fully updated
- [ ] ✅ APPROVED_MICROSERVICES_ARCHITECTURE.md fully updated
- [ ] ✅ All WorkflowExecution docs reference Tekton

---

## Validation Commands

```bash
# Check for remaining KubernetesExecutor references (should return only DEPRECATED/ADR files)
grep -r "KubernetesExecutor" docs/ README.md | grep -v "DEPRECATED" | grep -v "ADR-025" | grep -v "ELIMINATION"

# Check for remaining KubernetesExecution references (should return only historical/archive)
grep -r "KubernetesExecution" docs/ README.md | grep -v "archive/" | grep -v "DEPRECATED" | grep -v "ADR-025"

# Check for remaining ActionExecution references (should return only ADR-024/elimination docs)
grep -r "ActionExecution" docs/ | grep -v "ADR-024" | grep -v "ELIMINATION" | grep -v "DEPRECATED"

# Verify service counts
grep -r "12 services" docs/ README.md  # Should be 0 results
grep -r "5 CRD controllers" docs/ README.md  # Should be 0 results
```

---

## Rollback Plan

If Tekton adoption is reversed (unlikely), rollback is straightforward:
1. Git revert the documentation update commit
2. Restore KubernetesExecutor service documentation
3. Update README.md to reflect rollback

**Estimated Rollback Effort**: 2 hours (simple git revert + conflict resolution)

---

## Success Metrics

| Metric | Target | Validation Method |
|--------|--------|-------------------|
| **Zero Obsolete References** | 0 refs to KubernetesExecutor (except DEPRECATED) | `grep` validation commands |
| **Service Count Accuracy** | 11 services, 4 CRD controllers | Manual inspection of key docs |
| **Diagram Consistency** | All diagrams show Tekton | Visual inspection of 6 files |
| **Build Success** | No broken links, valid markdown | CI/CD markdown linter |

---

## Approval Required

**Critical Decisions** (from ADR-025):
1. **RBAC Strategy**: Pre-create all ServiceAccounts vs. dynamic creation
2. **Policy Distribution**: Container-embedded vs. ConfigMap-based
3. **Dry-Run Behavior**: Always enforce vs. optional skip
4. **Update Timeline**: Big-bang vs. phased

**Awaiting User Input**: Which options do you approve for each decision?

---

## Next Steps

1. ⏸️ **Await user approval** of 4 critical decisions (ADR-025)
2. ⏸️ **Begin Phase 1**: Architecture & README updates (4-5 hours)
3. ⏸️ **Complete Phases 2-5**: Systematic updates (10-12 hours)
4. ⏸️ **Run validation commands**: Ensure zero obsolete references
5. ⏸️ **Submit PR**: Single atomic commit for consistency

---

**Plan Status**: 📋 **Awaiting Approval**
**Total Effort**: 14-17 hours (2 engineering days)
**Priority**: 🔴 **Critical** (architectural consistency)
**Decision**: [ADR-025](./ADR-025-kubernetesexecutor-service-elimination.md)


