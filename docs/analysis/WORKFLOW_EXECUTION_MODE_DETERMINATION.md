# Workflow Execution Mode Determination (Sequential vs Parallel)

**Date**: October 8, 2025
**Purpose**: Explain how workflow execution mode (sequential vs parallel) is determined from AI recommendations through workflow execution
**Question**: How is the execution mode "sequential" or "parallel" decided?

---

## 🎯 **SHORT ANSWER**

The execution mode is **NOT explicitly set** as "sequential" or "parallel" at the workflow level. Instead, it is **dynamically determined** by the **WorkflowExecution Controller** during the **planning phase** based on:

1. **Step Dependencies** defined in AI recommendations
2. **Dependency Graph Analysis** (topological sort)
3. **Per-Step Metadata** (optional execution_mode hint)

The WorkflowExecution Controller analyzes step dependencies and **automatically determines** which steps can execute in parallel and which must execute sequentially.

---

## 📋 **DETAILED EXPLANATION**

---

## **PHASE 1: AI GENERATES RECOMMENDATIONS WITH DEPENDENCIES**

### **Step 1.1: HolmesGPT Generates Remediation Steps**

**Controller**: AIAnalysis
**Source**: HolmesGPT AI Analysis

**AI Output Example**:
```json
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "scale_deployment",
      "description": "Scale payment-api deployment to 5 replicas",
      "confidence": 0.92,
      "parameters": {
        "deployment": "payment-api",
        "namespace": "production",
        "replicas": 5
      },
      "dependencies": []
    },
    {
      "id": "rec-002",
      "action": "increase_memory_limit",
      "description": "Increase memory limit from 512Mi to 1Gi",
      "confidence": 0.88,
      "parameters": {
        "deployment": "payment-api",
        "namespace": "production",
        "memory": "1Gi"
      },
      "dependencies": ["rec-001"]
    }
  ]
}
```

**Key Point**: AI defines **dependencies** between steps, not execution mode.

---

### **Step 1.2: RemediationOrchestrator Creates WorkflowExecution CRD**

**Controller**: RemediationOrchestrator
**Action**: Copy AI recommendations → WorkflowExecution.spec

**WorkflowExecution CRD Created**:
```yaml
apiVersion: workflow.kubernaut.io/v1
kind: WorkflowExecution
metadata:
  name: rem-req-abc123-workflow
spec:
  workflowDefinition:
    steps:
    - name: "scale-deployment"
      action: "scale_deployment"
      parameters:
        deployment: "payment-api"
        namespace: "production"
        replicas: 5
      dependencies: []  # No dependencies - can execute immediately
    - name: "increase-memory"
      action: "patch_deployment"
      parameters:
        deployment: "payment-api"
        namespace: "production"
        memory: "1Gi"
      dependencies: ["scale-deployment"]  # Depends on scale-deployment
  executionStrategy:
    mode: "auto"  # NOT "sequential" or "parallel" - AUTO-DETERMINED
    dryRunFirst: true
    rollbackOnFailure: true
```

**Key Point**: The `mode` is NOT explicitly "sequential" or "parallel". The WorkflowExecution Controller will **determine execution order automatically** based on dependencies.

---

## **PHASE 2: WORKFLOW PLANNING - DEPENDENCY GRAPH ANALYSIS**

### **Step 2.1: WorkflowExecution Controller Analyzes Dependencies**

**Controller**: WorkflowExecution Reconciler
**Phase**: `planning`
**Business Requirements**: BR-WF-010, BR-WF-011

**Planning Logic**:
```go
func (r *WorkflowExecutionReconciler) handlePlanningPhase(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    steps := workflow.Spec.WorkflowDefinition.Steps

    // Step 1: Build dependency graph
    graph, err := r.buildDependencyGraph(steps)
    if err != nil {
        return r.transitionToFailed(ctx, workflow, "dependency_resolution_failed", err)
    }

    // Step 2: Calculate execution order (topological sort)
    // This determines which steps can run in parallel
    executionOrder, err := r.calculateExecutionOrder(graph)
    if err != nil {
        return r.transitionToFailed(ctx, workflow, "execution_order_failed", err)
    }

    // Step 3: Determine execution strategy based on dependency analysis
    strategy := r.determineExecutionStrategy(executionOrder)

    // Update status with execution plan
    workflow.Status.ExecutionPlan = &workflowv1.ExecutionPlan{
        Strategy:       strategy,  // "sequential", "parallel", or "sequential-with-parallel"
        ExecutionOrder: executionOrder,
    }

    workflow.Status.Phase = "validating"
    return ctrl.Result{}, r.Status().Update(ctx, workflow)
}
```

---

### **Step 2.2: Build Dependency Graph**

**Function**: `buildDependencyGraph()`
**Algorithm**: Create directed acyclic graph (DAG) from step dependencies

**Example Graph**:
```
Scenario: 4-step workflow with mixed dependencies

Steps:
- step1: dependencies: []
- step2: dependencies: ["step1"]
- step3: dependencies: ["step1"]
- step4: dependencies: ["step2", "step3"]

Dependency Graph (DAG):
       step1
       /   \
    step2   step3
       \   /
       step4

Analysis:
- Batch 1: [step1] - no dependencies, executes first
- Batch 2: [step2, step3] - both depend only on step1, CAN EXECUTE IN PARALLEL
- Batch 3: [step4] - depends on both step2 and step3, executes after both complete
```

**Code Example** (from `pkg/workflow/engine/workflow_engine.go:1773-1817`):
```go
func (dwe *DefaultWorkflowEngine) buildDependencyGraph(steps []*ExecutableWorkflowStep) (*DependencyGraph, error) {
    graph := NewDependencyGraph()

    for _, step := range steps {
        node := &GraphNode{
            StepName:     step.Name,
            Action:       step.Action,
            Dependencies: step.Dependencies,
        }

        if err := graph.AddNode(node); err != nil {
            return nil, fmt.Errorf("failed to add node %s: %w", step.Name, err)
        }
    }

    // Detect circular dependencies
    if graph.HasCycle() {
        return nil, fmt.Errorf("circular dependency detected in workflow")
    }

    return graph, nil
}
```

---

### **Step 2.3: Calculate Execution Order (Topological Sort)**

**Function**: `calculateExecutionOrder()`
**Algorithm**: Topological sort with batching for parallelization

**Execution Order Calculation**:
```go
func (r *WorkflowExecutionReconciler) calculateExecutionOrder(
    graph *DependencyGraph,
) ([]ExecutionBatch, error) {
    order := []ExecutionBatch{}

    // Topological sort: repeatedly extract nodes with no dependencies
    for !graph.IsEmpty() {
        batch := ExecutionBatch{
            Steps:    []string{},
            Parallel: false,  // Will be set to true if multiple steps in batch
        }

        // Find all nodes with no unmet dependencies (ready to execute)
        readyNodes := graph.GetReadyNodes()

        if len(readyNodes) == 0 {
            return nil, fmt.Errorf("circular dependency detected")
        }

        // All ready nodes can execute in parallel
        for _, node := range readyNodes {
            batch.Steps = append(batch.Steps, node.StepName)
            graph.RemoveNode(node.StepName)
        }

        // Mark as parallel if multiple steps
        batch.Parallel = len(batch.Steps) > 1

        order = append(order, batch)
    }

    return order, nil
}
```

**Example Execution Order Output**:
```yaml
executionOrder:
- batch: 1
  steps: ["scale-deployment"]
  parallel: false  # Only 1 step
- batch: 2
  steps: ["restart-pod-a", "restart-pod-b"]
  parallel: true   # 2 independent steps - CAN RUN IN PARALLEL
- batch: 3
  steps: ["verify-deployment"]
  parallel: false  # Only 1 step, depends on batch 2 completion
```

---

### **Step 2.4: Determine Execution Strategy**

**Function**: `determineExecutionStrategy()`
**Logic**: Classify overall workflow execution pattern

**Strategy Classification**:
```go
func (r *WorkflowExecutionReconciler) determineExecutionStrategy(
    executionOrder []ExecutionBatch,
) string {
    hasParallel := false
    hasSequential := false

    for _, batch := range executionOrder {
        if batch.Parallel && len(batch.Steps) > 1 {
            hasParallel = true
        }
        if !batch.Parallel || len(batch.Steps) == 1 {
            hasSequential = true
        }
    }

    // Classify strategy
    if hasParallel && hasSequential {
        return "sequential-with-parallel"  // Mixed: some steps parallel, some sequential
    } else if hasParallel {
        return "parallel"  // All steps can run in parallel (rare)
    } else {
        return "sequential"  // All steps must run sequentially
    }
}
```

**Example Strategy Results**:

| Workflow Pattern | Strategy | Reasoning |
|-----------------|----------|-----------|
| Linear chain (A → B → C) | `sequential` | All steps depend on previous step |
| Independent steps (A, B, C) | `parallel` | No dependencies, all can run together |
| Diamond (A → B,C → D) | `sequential-with-parallel` | B,C can run in parallel, but A must be first, D must be last |
| Fork-join (A → B,C,D → E,F) | `sequential-with-parallel` | Multiple parallel batches with sequential transitions |

---

## **PHASE 3: PARALLEL EXECUTION VALIDATION**

### **Step 3.1: Can Steps Execute in Parallel?**

**Function**: `canExecuteInParallel()`
**Source**: `pkg/workflow/engine/workflow_engine.go:1773-1817`
**Business Requirement**: BR-WF-001 (100% correctness for step dependencies)

**Validation Logic**:
```go
func (dwe *DefaultWorkflowEngine) canExecuteInParallel(steps []*ExecutableWorkflowStep) bool {
    if len(steps) <= 1 {
        return false  // Single step cannot be "parallel"
    }

    // Build map of step IDs in this batch
    stepIDs := make(map[string]bool)
    for _, step := range steps {
        stepIDs[step.ID] = true
    }

    // Check 1: No inter-step dependencies within batch
    for _, step := range steps {
        for _, depID := range step.Dependencies {
            if stepIDs[depID] {
                // Step depends on another step in this batch
                // CANNOT run in parallel
                log.Debug("Steps cannot execute in parallel due to inter-step dependency",
                    "step_id", step.ID,
                    "depends_on", depID)
                return false
            }
        }
    }

    // Check 2: No explicit sequential execution requirement
    for _, step := range steps {
        if step.Metadata != nil {
            if executionMode, exists := step.Metadata["execution_mode"]; exists {
                if executionMode == "sequential" {
                    // Step explicitly requires sequential execution
                    log.Debug("Steps cannot execute in parallel due to sequential requirement",
                        "step_id", step.ID)
                    return false
                }
            }
        }
    }

    // All checks passed - steps can execute in parallel
    log.Debug("Steps validated for parallel execution",
        "parallel_candidates", len(steps))
    return true
}
```

**Validation Checks**:
1. ✅ **No Inter-Step Dependencies**: Steps in same batch don't depend on each other
2. ✅ **No Sequential Metadata**: No step has `execution_mode: "sequential"` hint
3. ✅ **Multiple Steps**: More than 1 step in batch

---

## **PHASE 4: EXECUTION - CREATE STEPS BASED ON BATCHES**

### **Step 4.1: Execute Batches in Order**

**Controller**: WorkflowExecution Reconciler
**Phase**: `executing`

**Execution Logic**:
```go
func (r *WorkflowExecutionReconciler) handleExecutingPhase(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    executionPlan := workflow.Status.ExecutionPlan

    // Get next batch to execute
    nextBatch := r.getNextExecutionBatch(workflow, executionPlan)

    if nextBatch == nil {
        // All batches complete
        workflow.Status.Phase = "monitoring"
        return ctrl.Result{}, r.Status().Update(ctx, workflow)
    }

    // Create KubernetesExecution (DEPRECATED - ADR-025) CRDs for all steps in batch
    if nextBatch.Parallel {
        // PARALLEL: Create all KubernetesExecution CRDs at once
        log.Info("Executing parallel batch",
            "batch_number", nextBatch.BatchNumber,
            "step_count", len(nextBatch.Steps))

        for _, stepName := range nextBatch.Steps {
            step := r.getStepByName(workflow, stepName)
            ke := r.buildKubernetesExecution(workflow, step)

            // Create KubernetesExecution CRD
            if err := r.Create(ctx, ke); err != nil {
                return ctrl.Result{}, fmt.Errorf(
                    "failed to create KubernetesExecution for step %s: %w",
                    stepName, err)
            }

            log.Info("Created parallel step execution",
                "step", stepName,
                "batch", nextBatch.BatchNumber)
        }
    } else {
        // SEQUENTIAL: Create single KubernetesExecution CRD
        log.Info("Executing sequential step",
            "step", nextBatch.Steps[0],
            "batch_number", nextBatch.BatchNumber)

        step := r.getStepByName(workflow, nextBatch.Steps[0])
        ke := r.buildKubernetesExecution(workflow, step)

        if err := r.Create(ctx, ke); err != nil {
            return ctrl.Result{}, fmt.Errorf(
                "failed to create KubernetesExecution: %w", err)
        }
    }

    // Update status and requeue
    return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, workflow)
}
```

**Parallel Execution Visualization**:
```
Batch 1 (Sequential):
    Create: KubernetesExecution-step1
    Wait: step1 completes

Batch 2 (Parallel):
    Create: KubernetesExecution-step2  ← Created simultaneously
    Create: KubernetesExecution-step3  ← Created simultaneously
    Wait: BOTH step2 AND step3 complete

Batch 3 (Sequential):
    Create: KubernetesExecution-step4
    Wait: step4 completes
```

---

## **EXAMPLE WORKFLOW SCENARIOS**

---

### **Scenario 1: Pure Sequential (Linear Chain)**

**AI Recommendations**:
```yaml
recommendations:
- id: rec-001
  action: restart_pod
  dependencies: []
- id: rec-002
  action: scale_deployment
  dependencies: ["rec-001"]
- id: rec-003
  action: verify_deployment
  dependencies: ["rec-002"]
```

**Dependency Graph**:
```
rec-001 → rec-002 → rec-003
```

**Execution Plan**:
```yaml
executionPlan:
  strategy: "sequential"
  executionOrder:
  - batch: 1
    steps: ["restart_pod"]
    parallel: false
  - batch: 2
    steps: ["scale_deployment"]
    parallel: false
  - batch: 3
    steps: ["verify_deployment"]
    parallel: false
```

**Execution Behavior**: Steps execute one at a time, in order.

---

### **Scenario 2: Pure Parallel (Independent Steps)**

**AI Recommendations**:
```yaml
recommendations:
- id: rec-001
  action: restart_pod_a
  dependencies: []
- id: rec-002
  action: restart_pod_b
  dependencies: []
- id: rec-003
  action: restart_pod_c
  dependencies: []
```

**Dependency Graph**:
```
rec-001 (independent)
rec-002 (independent)
rec-003 (independent)
```

**Execution Plan**:
```yaml
executionPlan:
  strategy: "parallel"
  executionOrder:
  - batch: 1
    steps: ["restart_pod_a", "restart_pod_b", "restart_pod_c"]
    parallel: true
```

**Execution Behavior**: All 3 steps execute simultaneously.

---

### **Scenario 3: Sequential-with-Parallel (Diamond Pattern)**

**AI Recommendations**:
```yaml
recommendations:
- id: rec-001
  action: scale_deployment
  dependencies: []
- id: rec-002
  action: restart_pod_a
  dependencies: ["rec-001"]
- id: rec-003
  action: restart_pod_b
  dependencies: ["rec-001"]
- id: rec-004
  action: verify_deployment
  dependencies: ["rec-002", "rec-003"]
```

**Dependency Graph**:
```
       rec-001
       /     \
  rec-002   rec-003
       \     /
       rec-004
```

**Execution Plan**:
```yaml
executionPlan:
  strategy: "sequential-with-parallel"
  executionOrder:
  - batch: 1
    steps: ["scale_deployment"]
    parallel: false
  - batch: 2
    steps: ["restart_pod_a", "restart_pod_b"]
    parallel: true  # CAN RUN IN PARALLEL
  - batch: 3
    steps: ["verify_deployment"]
    parallel: false
```

**Execution Behavior**:
1. Scale deployment (sequential)
2. Restart both pods **in parallel** (both depend only on step 1)
3. Verify deployment after both restarts complete (sequential)

---

## 🔑 **KEY DECISION FACTORS**

### **1. Step Dependencies (Primary Factor)**

**Source**: AI recommendations define `dependencies: []` for each step

**Decision Logic**:
- **No dependencies** → Can execute immediately (batch 1 or parallel with others)
- **Has dependencies** → Must wait for dependencies to complete first
- **Same dependencies** → Can execute in parallel with other steps with same dependencies

**Example**:
```yaml
# Step A and B have SAME dependency (step1)
# → A and B can execute in PARALLEL

stepA:
  dependencies: ["step1"]
stepB:
  dependencies: ["step1"]

# Execution: step1 completes → stepA and stepB execute in parallel
```

---

### **2. Dependency Graph Structure (Secondary Factor)**

**Algorithm**: Topological sort with batching

**Pattern Recognition**:
- **Linear chain** → Sequential execution
- **Fork pattern** → Parallel execution at fork point
- **Join pattern** → Sequential execution at join point
- **Diamond pattern** → Mixed sequential-with-parallel

---

### **3. Optional Metadata Hints (Tertiary Factor)**

**Source**: Step metadata can include `execution_mode` hint

**Example**:
```yaml
stepA:
  dependencies: []
  metadata:
    execution_mode: "sequential"  # Force sequential even if no dependencies
```

**Use Cases**:
- Steps that modify shared state
- Steps with resource contention concerns
- Steps that must execute in specific order for non-dependency reasons

---

## ✅ **SUMMARY**

### **How Execution Mode is Determined**:

1. **AI generates recommendations** with dependencies (not execution mode)
2. **RemediationOrchestrator** copies recommendations to WorkflowExecution CRD
3. **WorkflowExecution Controller** (planning phase):
   - Builds dependency graph from step dependencies
   - Performs topological sort to determine execution order
   - Groups steps into batches (steps with no inter-batch dependencies)
   - Marks batches as `parallel: true` if multiple steps with same dependencies
4. **WorkflowExecution Controller** (executing phase):
   - Executes batches in order
   - Creates KubernetesExecution CRDs for parallel steps simultaneously
   - Waits for all steps in batch to complete before next batch

### **Execution Strategy Classification**:
- **`sequential`**: All steps have linear dependencies (A → B → C)
- **`parallel`**: All steps are independent (A, B, C with no dependencies)
- **`sequential-with-parallel`**: Mixed pattern (some batches parallel, overall sequential)

### **Key Business Requirements**:
- **BR-WF-003**: MUST implement parallel and sequential execution patterns
- **BR-WF-010**: MUST support dependency-based conditions
- **BR-WF-011**: MUST coordinate execution order based on dependency graphs
- **BR-DEP-011**: MUST coordinate execution order based on dependency graphs
- **BR-DEP-012**: MUST optimize parallel execution while respecting dependencies

---

**Confidence**: **100%** - Execution mode is dynamically determined from dependency graph analysis, not explicitly set by AI or user.

**Document Status**: ✅ **COMPLETE** - Comprehensive explanation of execution mode determination
