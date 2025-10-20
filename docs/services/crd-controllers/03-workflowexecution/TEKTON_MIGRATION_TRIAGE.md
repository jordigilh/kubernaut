# WorkflowExecution Documentation Triage - Tekton Migration

**Date**: 2025-10-19
**Decision**: [ADR-024: Eliminate ActionExecution Layer](../../../architecture/decisions/ADR-024-eliminate-actionexecution-layer.md)
**Status**: 🔄 In Progress

---

## Triage Summary

**18 files** contain obsolete references to `KubernetesExecution` or `ActionExecution` CRDs that need to be updated for the new Tekton-based architecture.

---

## Obsolete Concepts to Remove

### **1. KubernetesExecution CRD** ❌
- **Old**: WorkflowExecution creates KubernetesExecution CRDs for each step
- **New**: WorkflowExecution creates single Tekton PipelineRun with multiple Tasks

### **2. ActionExecution CRD** ❌
- **Old**: Intermediate tracking layer between WorkflowExecution and Tekton
- **New**: Eliminated (see ADR-024)

### **3. Step Orchestrator Component** ⚠️
- **Old**: Dedicated component to orchestrate KubernetesExecution creation
- **New**: Simplified - WorkflowExecutionReconciler creates PipelineRun directly

### **4. Executor Service Integration** ❌
- **Old**: WorkflowExecution → KubernetesExecution → Executor Service
- **New**: WorkflowExecution → Tekton PipelineRun → Tekton TaskRun → Pod

---

## Files Requiring Updates

### **Critical Files** (Direct Implementation Impact)

| File | Obsolete References | Priority | Status |
|------|---------------------|----------|--------|
| `integration-points.md` | KubernetesExecution CRD creation | 🔴 Critical | ⏸️ Pending |
| `reconciliation-phases.md` | Step orchestration logic | 🔴 Critical | ⏸️ Pending |
| `controller-implementation.md` | KubernetesExecution creation | 🔴 Critical | ⏸️ Pending |
| `IMPLEMENTATION_PLAN_V1.0.md` | ActionExecution references | 🔴 Critical | ⏸️ Pending |

### **High-Priority Files** (Architecture & Design)

| File | Obsolete References | Priority | Status |
|------|---------------------|----------|--------|
| `overview.md` | Step Orchestrator, Mermaid diagram | 🟡 High | ✅ Partially updated |
| `README.md` | KubernetesExecution references | 🟡 High | ⏸️ Pending |
| `crd-schema.md` | Step execution patterns | 🟡 High | ⏸️ Pending |

### **Medium-Priority Files** (Supporting Documentation)

| File | Obsolete References | Priority | Status |
|------|---------------------|----------|--------|
| `testing-strategy.md` | Step Orchestrator tests | 🟢 Medium | ⏸️ Pending |
| `observability-logging.md` | KubernetesExecution logs | 🟢 Medium | ⏸️ Pending |
| `metrics-slos.md` | KubernetesExecution metrics | 🟢 Medium | ⏸️ Pending |
| `implementation-checklist.md` | KubernetesExecution tasks | 🟢 Medium | ⏸️ Pending |
| `finalizers-lifecycle.md` | KubernetesExecution cleanup | 🟢 Medium | ⏸️ Pending |
| `security-configuration.md` | Executor Service RBAC | 🟢 Medium | ⏸️ Pending |

### **Low-Priority Files** (Implementation Plans & Extensions)

| File | Obsolete References | Priority | Status |
|------|---------------------|----------|--------|
| `implementation/IMPLEMENTATION_PLAN_V1.2_PARALLEL_LIMITS_EXTENSION.md` | ActionExecution references | 🔵 Low | ⏸️ Pending |
| `implementation/EXPANSION_PLAN_TO_95_PERCENT.md` | KubernetesExecution | 🔵 Low | ⏸️ Pending |
| `implementation/testing/BR_COVERAGE_MATRIX.md` | ActionExecution | 🔵 Low | ⏸️ Pending |
| `implementation/phase0/DAY_03_EXPANDED.md` | KubernetesExecution | 🔵 Low | ⏸️ Pending |
| `migration-current-state.md` | Executor Service | 🔵 Low | ⏸️ Pending |

---

## Key Replacement Patterns

### **Pattern 1: Step Execution**

**Old (Obsolete)**:
```go
// Create KubernetesExecution CRD for each step
k8sExec := &executorv1.KubernetesExecution{
    Spec: executorv1.KubernetesExecutionSpec{
        Action:     step.Action,
        Parameters: step.Parameters,
    },
}
r.Create(ctx, k8sExec)

// Watch KubernetesExecution for completion
r.Watch(&executorv1.KubernetesExecution{})
```

**New (Tekton-based)**:
```go
// Create single Tekton PipelineRun with all steps
pipelineRun := r.createPipelineRun(workflow)
r.Create(ctx, pipelineRun)

// Write action records to Data Storage Service
for _, step := range workflow.Spec.Steps {
    r.DataStorageClient.RecordAction(ctx, &datastorage.ActionRecord{
        WorkflowID:  workflow.Name,
        ActionType:  step.ActionType,
        Image:       step.Image,
        ExecutedAt:  time.Now(),
        Status:      "executing",
    })
}

// Watch PipelineRun for completion
r.Watch(&tektonv1.PipelineRun{})
```

---

### **Pattern 2: Dependency Resolution**

**Old (Obsolete)**:
```go
// Step Orchestrator determines which KubernetesExecutions to create
func (o *StepOrchestrator) getReadySteps(workflow *WorkflowExecution) []Step {
    // Check dependencies, create KubernetesExecution CRDs
}
```

**New (Tekton-based)**:
```go
// Tekton handles dependency resolution via runAfter
tasks := []tektonv1.PipelineTask{
    {
        Name: "step-1",
        TaskRef: &tektonv1.TaskRef{Name: "kubernaut-action"},
        RunAfter: []string{},  // No dependencies
    },
    {
        Name: "step-2",
        TaskRef: &tektonv1.TaskRef{Name: "kubernaut-action"},
        RunAfter: []string{"step-1"},  // Depends on step-1
    },
}
```

---

### **Pattern 3: Status Monitoring**

**Old (Obsolete)**:
```go
// Watch KubernetesExecution status
k8sExecList := &executorv1.KubernetesExecutionList{}
r.List(ctx, k8sExecList, client.MatchingLabels{"workflow": workflow.Name})
for _, k8sExec := range k8sExecList.Items {
    if k8sExec.Status.Phase == "succeeded" {
        // Update workflow status
    }
}
```

**New (Tekton-based)**:
```go
// Watch PipelineRun status
pipelineRun := &tektonv1.PipelineRun{}
r.Get(ctx, types.NamespacedName{Name: workflow.Name, Namespace: workflow.Namespace}, pipelineRun)

if pipelineRun.Status.CompletionTime != nil {
    workflow.Status.Phase = "Completed"
    workflow.Status.CompletionTime = pipelineRun.Status.CompletionTime
}
```

---

### **Pattern 4: Integration Points**

**Old (Obsolete)**:
```yaml
# Downstream: KubernetesExecution → Executor Service
WorkflowExecution creates:
  - KubernetesExecution CRD (for each step)
    → Executor Service watches KubernetesExecution
      → Executor Service creates Kubernetes Job
```

**New (Tekton-based)**:
```yaml
# Downstream: Tekton PipelineRun → TaskRun → Pod
WorkflowExecution creates:
  - Tekton PipelineRun (single resource)
    → Tekton creates TaskRuns (for each step)
      → Tekton creates Pods (action containers)
```

---

## Architecture Component Changes

### **Components to Remove**

| Component | Reason |
|-----------|--------|
| **KubernetesExecution CRD** | Replaced by Tekton TaskRun |
| **ActionExecution CRD** | Eliminated (see ADR-024) |
| **Step Orchestrator** | Replaced by Tekton DAG orchestration |
| **Executor Service integration** | Direct Tekton integration |

### **Components to Add**

| Component | Purpose |
|-----------|---------|
| **Data Storage Client** | Record actions for pattern monitoring (replaces ActionExecution tracking) |
| **PipelineRun Creator** | Translate WorkflowExecution → Tekton PipelineRun |
| **PipelineRun Monitor** | Watch Tekton PipelineRun status |

---

## Mermaid Diagram Updates

### **Old Diagram** ❌
```mermaid
WorkflowExecution Controller
        ↓
Step Orchestrator
        ↓
KubernetesExecution CRD (Step 1)
KubernetesExecution CRD (Step 2)
        ↓
Executor Service
        ↓
Kubernetes Job
```

### **New Diagram** ✅
```mermaid
WorkflowExecution Controller
        ↓
Tekton PipelineRun (single resource)
        ↓
Tekton TaskRun (Step 1)
Tekton TaskRun (Step 2)
        ↓
Pod (Action Container)
        ↓
Data Storage Service (action records)
```

---

## Business Requirements Mapping

| BR | Old Implementation | New Implementation |
|----|--------------------|--------------------|
| **BR-WF-010**: Step dependency resolution | Step Orchestrator + KubernetesExecution | Tekton `runAfter` dependencies |
| **BR-WF-011**: Parallel execution | Multiple KubernetesExecution CRDs | Tekton parallel tasks |
| **BR-WF-030**: Execution monitoring | Watch KubernetesExecution CRDs | Watch Tekton PipelineRun |
| **BR-WF-050**: Rollback capability | KubernetesExecution undo operations | Tekton retry + custom rollback logic |

---

## Implementation Phases

### **Phase 1: Critical Updates** (1-2 hours)
- ✅ `overview.md` - Partially updated
- ⏸️ `integration-points.md` - Update KubernetesExecution → Tekton PipelineRun
- ⏸️ `reconciliation-phases.md` - Update step orchestration logic
- ⏸️ `controller-implementation.md` - Update controller code examples

### **Phase 2: High-Priority Updates** (1-2 hours)
- ⏸️ `README.md` - Update service overview
- ⏸️ `crd-schema.md` - Update step execution patterns
- ⏸️ `IMPLEMENTATION_PLAN_V1.0.md` - Update implementation strategy

### **Phase 3: Medium-Priority Updates** (2-3 hours)
- ⏸️ Update testing, observability, metrics, security docs

### **Phase 4: Low-Priority Updates** (1-2 hours)
- ⏸️ Update implementation plans and expansion docs

---

## Success Criteria

- ✅ Zero references to `KubernetesExecution` CRD
- ✅ Zero references to `ActionExecution` CRD
- ✅ Zero references to "Step Orchestrator" as a separate component
- ✅ All Mermaid diagrams show Tekton architecture
- ✅ All code examples use Tekton PipelineRun creation
- ✅ All integration points reference Tekton TaskRuns

---

## Next Steps

1. **Execute Phase 1**: Update critical files (integration-points, reconciliation-phases, controller-implementation)
2. **Execute Phase 2**: Update high-priority files (README, crd-schema, implementation plan)
3. **Execute Phase 3**: Update supporting documentation
4. **Execute Phase 4**: Update implementation plans
5. **Final Review**: Ensure consistency across all documents

---

**Status**: 🔄 In Progress
**Priority**: 🔴 Critical (Architectural consistency)
**Estimated Effort**: 6-8 hours (systematic updates across 18 files)




