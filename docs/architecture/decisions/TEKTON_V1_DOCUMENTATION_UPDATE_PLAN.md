# Tekton V1 Architecture: Documentation Update Plan

**Date**: 2025-10-19
**Decision**: Use Tekton/Tekton Pipelines from V1 (eliminate custom orchestration)
**Status**: ✅ Approved
**Confidence**: 95% (Very High)

---

## Executive Summary

**Decision**: Kubernaut will use **Tekton Pipelines** (Tekton Pipelines for upstream community, upstream Tekton for others) as the workflow execution engine **from V1** (Q4 2025).

**Impact**: Eliminates 500+ lines of custom orchestration code, reduces development time from 16 weeks to 8 weeks, and ensures maximum upstream community alignment.

**Key Insight**: Tekton is universally available (Tekton Pipelines bundled + upstream Tekton open source), making the V1/V2 split unnecessary architectural waste.

---

## Documentation Update Plan

### **Phase 1: Decision Documentation** ✅

#### **1.1 Create ADR-023: Tekton from V1**
- **File**: `docs/architecture/decisions/ADR-023-tekton-from-v1.md`
- **Purpose**: Formal architectural decision record
- **Content**:
  - Context: Why Tekton from V1 vs custom orchestration
  - Decision: Tekton Pipelines as workflow execution engine
  - Consequences: Eliminate 500+ lines of code, no V1/V2 migration
  - Alternatives considered: Custom orchestration, Argo Workflows
  - upstream community alignment: Tekton Pipelines

#### **1.2 Update ADR-022: Mark as Superseded**
- **File**: `docs/architecture/decisions/ADR-022-v1-native-jobs-v2-tekton-migration.md`
- **Action**: Add superseded notice pointing to ADR-023
- **Reason**: V1/V2 split no longer needed

---

### **Phase 2: Architecture Documentation** ✅

#### **2.1 Rename and Update Transition Strategy**
- **Old**: `docs/architecture/V1_TO_V2_TRANSITION_STRATEGY.md`
- **New**: `docs/architecture/TEKTON_EXECUTION_ARCHITECTURE.md`
- **Changes**:
  - Remove V1/V2 split concept
  - Focus on single Tekton-based architecture
  - Add installation guide (Tekton Pipelines vs upstream Tekton)
  - Update diagrams to show Tekton from day 1
  - Remove migration timeline (not needed)

#### **2.2 Update Quick Reference**
- **File**: `docs/architecture/EXECUTION_ARCHITECTURE_QUICK_REFERENCE.md`
- **Changes**:
  - Remove V1 vs V2 comparison table
  - Update to single Tekton architecture
  - Add Tekton installation instructions
  - Update code examples to show PipelineRun creation

#### **2.3 Update Secure Container Execution Summary**
- **File**: `docs/architecture/decisions/SECURE_CONTAINER_EXECUTION_SUMMARY.md`
- **Changes**:
  - Remove Phase 1 (Generic Executor with Container Registry - not needed)
  - Remove Phase 2 (Tekton Pipelines for V2 - now V1)
  - Update to reflect Tekton from V1
  - Simplify container portability section (no V1/V2 dual mode needed)

#### **2.4 Update README.md**
- **File**: `README.md`
- **Changes**:
  - Remove "V1 → V2 Transition Strategy" section
  - Add "Tekton Execution Architecture" section
  - Update timeline to show Tekton from Q4 2025
  - Add prerequisite: Tekton Pipelines (Tekton Pipelines or upstream)

---

### **Phase 3: Service Specifications** ✅

#### **3.1 WorkflowExecution Controller (MAJOR UPDATE)**

**Files to Update**:
- `docs/services/crd-controllers/03-workflowexecution/overview.md`
- `docs/services/crd-controllers/03-workflowexecution/reconciliation-phases.md`
- `docs/services/crd-controllers/03-workflowexecution/integration-points.md`

**Changes**:
- **Overview**: Update to describe Tekton PipelineRun creation (not ActionExecution CRDs)
- **Reconciliation Phases**:
  - Phase 1: Validate workflow and translate to PipelineRun spec
  - Phase 2: Create Tekton PipelineRun
  - Phase 3: Monitor PipelineRun status
  - Phase 4: Sync status back to WorkflowExecution CRD
- **Integration Points**:
  - Creates: Tekton PipelineRun (not ActionExecution CRDs)
  - Watches: Tekton PipelineRun status
  - Dependencies: Tekton Pipelines API

**New Files to Create**:
- `docs/services/crd-controllers/03-workflowexecution/tekton-integration.md`
  - How WorkflowExecution translates to PipelineRun
  - Generic meta-task specification
  - Workspace management
  - Status synchronization

---

#### **3.2 ActionExecution Controller (SCOPE CHANGE)**

**Decision Point**: Do we still need ActionExecution CRD?

**Option A: Eliminate ActionExecution** (Recommended)
- Tekton TaskRuns replace ActionExecution CRDs
- Simpler architecture (one less controller)
- Direct Tekton integration

**Option B: Retain ActionExecution as Tracking Layer**
- WorkflowExecution creates ActionExecution CRDs for tracking
- ActionExecution controller creates corresponding Tekton TaskRuns
- Enables pattern monitoring and effectiveness tracking via dedicated CRDs

**Recommendation**: **Option B** - Retain ActionExecution for tracking
- Pattern monitoring needs per-action CRDs
- Effectiveness tracking requires action-level metrics
- Audit trail via dedicated Kubernaut CRDs (not just Tekton TaskRuns)

**Files to Update** (if Option B):
- `docs/services/crd-controllers/04-kubernetesexecutor/overview.md` (rename to action-executor)
- Update to describe Tekton TaskRun creation (not Kubernetes Jobs)

**Files to Update** (if Option A):
- Mark ActionExecution/KubernetesExecutor as deprecated
- Update integration points to reference Tekton TaskRuns directly

---

#### **3.3 RemediationOrchestrator (MINOR UPDATE)**

**Files to Update**:
- `docs/services/crd-controllers/05-remediationorchestrator/integration-points.md`

**Changes**:
- No change to CRD creation (still creates WorkflowExecution CRDs)
- Update integration documentation to reflect Tekton execution
- Add prerequisite: Tekton Pipelines installed

---

### **Phase 4: Implementation Plans** ✅

#### **4.1 WorkflowExecution Implementation Plan**

**File**: `docs/services/crd-controllers/03-workflowexecution/implementation/IMPLEMENTATION_PLAN_V1.0.md`

**Version Bump**: `v1.3` → `v2.0` (major architectural change)

**Changelog for v2.0**:
```markdown
## Changelog

### v2.0.0 (2025-10-19) - TEKTON ARCHITECTURE
**Breaking Change**: Eliminates custom orchestration in favor of Tekton Pipelines

#### Added
- Tekton PipelineRun translation logic (ADR-023)
- Generic meta-task integration
- Tekton workspace management
- PipelineRun status monitoring and synchronization

#### Removed
- Custom DAG resolution (500 lines) - Tekton handles orchestration
- Custom parallel execution logic - Tekton handles runAfter dependencies
- Manual PVC creation/deletion - Tekton workspaces handle this
- Custom retry and timeout logic - Tekton handles this

#### Changed
- Reconciliation phases simplified (4 phases vs 6 phases)
- Integration points: Creates PipelineRun (not ActionExecution CRDs)
- Dependencies: Requires Tekton Pipelines API

#### Business Requirements
- BR-WORKFLOW-001: Multi-step workflow orchestration (via Tekton)
- BR-WORKFLOW-002: Parallel execution (via Tekton runAfter)
- BR-WORKFLOW-003: Dependency management (via Tekton DAG)
- BR-PLATFORM-006: Tekton Pipelines integration (upstream community alignment)

#### Rationale
- Eliminates 500+ lines of throwaway code
- Reduces development time: 16 weeks → 8 weeks
- Maximum upstream community alignment (Tekton Pipelines)
- CNCF Graduated project (same trust as Kubernetes)
- Universal availability (bundled + upstream)

#### Migration Impact
- No migration needed (V1 = Tekton from day 1)
- No V1/V2 split complexity
- No feature flags or dual reconcilers
```

**Content Updates**:
- **APDC Analysis Phase**: Search for Tekton integration patterns
- **APDC Plan Phase**: Design PipelineRun translation, not custom DAG
- **DO-RED Phase**: Tests for PipelineRun creation
- **DO-GREEN Phase**: Minimal PipelineRun translator
- **DO-REFACTOR Phase**: Enhance translation logic, status sync

**Code Examples**:
```go
// Replace custom orchestration with PipelineRun creation
func (r *WorkflowExecutionReconciler) createPipelineRun(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (*tektonv1.PipelineRun, error)
```

---

#### **4.2 ActionExecution Implementation Plan** (if retained)

**File**: `docs/services/crd-controllers/04-kubernetesexecutor/implementation/IMPLEMENTATION_PLAN_V1.0.md`

**Version Bump**: `v1.0` → `v2.0` (major architectural change)

**Changelog for v2.0.0**:
```markdown
## Changelog

### v2.0.0 (2025-10-19) - TEKTON TASKRUN INTEGRATION
**Breaking Change**: Creates Tekton TaskRuns instead of Kubernetes Jobs

#### Added
- Tekton TaskRun creation logic
- Generic meta-task parameter mapping
- TaskRun status monitoring

#### Removed
- Kubernetes Job creation logic (replaced by Tekton TaskRuns)
- Manual Job status monitoring (Tekton handles this)

#### Changed
- Execution primitive: TaskRun (not Job)
- Dependencies: Requires Tekton Pipelines API

#### Business Requirements
- BR-ACTION-001: Action execution (via Tekton TaskRuns)
- BR-ACTION-002: Cosign verification (via admission controller)
- BR-PLATFORM-006: Tekton Pipelines integration
```

---

#### **4.3 RemediationOrchestrator Implementation Plan**

**File**: `docs/services/crd-controllers/05-remediationorchestrator/implementation/IMPLEMENTATION_PLAN_V1.0.md`

**Version Bump**: `v1.0.2` → `v1.0.3` (minor update - add prerequisite)

**Changelog for v1.0.3**:
```markdown
## Changelog

### v1.0.3 (2025-10-19) - TEKTON PREREQUISITE
**Minor Update**: Add Tekton Pipelines as deployment prerequisite

#### Added
- Prerequisite: Tekton Pipelines (Tekton Pipelines or upstream)
- Deployment validation: Check Tekton Pipelines availability

#### Changed
- Deployment documentation updated with Tekton prerequisite
- Integration testing requires Tekton Pipelines installed

#### Business Requirements
- BR-PLATFORM-006: Tekton Pipelines integration
```

---

### **Phase 5: New Documentation** ✅

#### **5.1 Tekton Installation Guide**

**File**: `docs/deployment/TEKTON_INSTALLATION.md`

**Content**:
- Tekton Pipelines installation (upstream community customers)
- Upstream Tekton installation (non-Kubernetes customers)
- Verification steps
- Troubleshooting

---

#### **5.2 Tekton Integration Guide**

**File**: `docs/services/crd-controllers/03-workflowexecution/tekton-integration.md`

**Content**:
- WorkflowExecution → PipelineRun translation
- Generic meta-task specification
- Workspace management (PVC vs Tekton workspaces)
- Status synchronization
- Error handling
- Code examples

---

#### **5.3 Generic Meta-Task Specification**

**File**: `docs/services/crd-controllers/03-workflowexecution/generic-meta-task.md`

**Content**:
- Tekton Task definition
- Action container integration
- Input/output parameter mapping
- Cosign verification (via admission)
- Workspace mounting

---

## Implementation Order

### **Step 1: Decision Documentation** (Priority 1)
1. Create ADR-023 (Tekton from V1)
2. Update ADR-022 (mark as superseded)

### **Step 2: Architecture Documentation** (Priority 1)
3. Rename and update transition strategy → Tekton architecture
4. Update quick reference
5. Update secure container execution summary
6. Update README.md

### **Step 3: Service Specifications** (Priority 2)
7. WorkflowExecution: Update overview, reconciliation, integration
8. WorkflowExecution: Create Tekton integration guide
9. ActionExecution: Assess scope change (eliminate vs retain)
10. RemediationOrchestrator: Minor update for prerequisite

### **Step 4: Implementation Plans** (Priority 2)
11. WorkflowExecution: Version bump v1.3 → v2.0 + changelog
12. ActionExecution: Version bump v1.0 → v2.0 + changelog (if retained)
13. RemediationOrchestrator: Version bump v1.0.2 → v1.0.3 + changelog

### **Step 5: New Documentation** (Priority 3)
14. Create Tekton installation guide
15. Create generic meta-task specification

---

## Decision Points Requiring Input

### **Decision 1: ActionExecution Controller Scope**

**Question**: Retain ActionExecution CRD as tracking layer, or eliminate in favor of direct Tekton TaskRuns?

**Option A: Eliminate** (Simpler architecture)
- WorkflowExecution creates PipelineRun directly
- No intermediate ActionExecution CRDs
- Pattern monitoring uses Tekton TaskRuns

**Option B: Retain** (Better tracking - RECOMMENDED)
- WorkflowExecution creates WorkflowExecution CRD (as before)
- ActionExecution CRDs created for tracking (pattern monitoring, effectiveness)
- ActionExecution controller creates Tekton TaskRuns (not Jobs)
- Audit trail via Kubernaut CRDs (not just Tekton TaskRuns)

**Recommendation**: **Option B** (retain for tracking)
- Effectiveness monitoring needs per-action CRDs
- Pattern monitoring requires action-level metrics
- Cleaner separation: Kubernaut CRDs (business) vs Tekton resources (execution)

**User Input Required**: Approve Option A or Option B?

---

### **Decision 2: Generic Meta-Task vs Per-Action Tasks**

**Question**: Single generic Tekton Task for all actions, or create 29+ Tekton Tasks (one per action type)?

**Option A: Generic Meta-Task** (RECOMMENDED)
```yaml
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: kubernaut-action
spec:
  params:
    - name: actionImage
    - name: inputs
  steps:
    - image: $(params.actionImage)
```

**Option B: Per-Action Tasks** (More Tekton-idiomatic)
```yaml
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: kubernaut-scale-deployment
spec:
  params:
    - name: deployment
    - name: replicas
  steps:
    - image: ghcr.io/kubernaut/actions/scale@sha256:...
```

**Recommendation**: **Option A** (generic meta-task)
- Container contract defines action behavior (self-documenting)
- 1 Task definition vs 29+ Task definitions
- Action registry remains in ConfigMap (easier updates)
- Same container portability principle

**User Input Required**: Approve Option A or Option B?

---

## Success Metrics

### **Documentation Quality**
- ✅ All ADRs updated with Tekton decision
- ✅ All architecture docs reflect single Tekton architecture
- ✅ All service specs updated with Tekton integration
- ✅ All implementation plans version-bumped with changelogs

### **Implementation Impact**
- ✅ 500+ lines of custom orchestration eliminated
- ✅ Development time reduced: 16 weeks → 8 weeks
- ✅ No V1/V2 migration complexity
- ✅ Maximum upstream community alignment (Tekton Pipelines)

### **Timeline**
- ✅ Q4 2025: Production with Tekton
- ✅ No V2 migration needed (V1 = final architecture)

---

## Next Steps

1. **User Approval**: Confirm decisions on ActionExecution scope and meta-task approach
2. **Execute Plan**: Create/update all documentation files
3. **Version Control**: Bump all implementation plan versions with detailed changelogs
4. **Validation**: Review updated documentation for consistency

---

**Status**: ✅ Plan Approved
**Ready to Execute**: Awaiting user confirmation on Decision 1 and Decision 2
**Estimated Effort**: 4-6 hours for all documentation updates
**Confidence**: 95% (Very High)

