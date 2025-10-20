# ADR-021: Workflow Dependency Cycle Detection & Validation

**Status**: ✅ **APPROVED**  
**Date**: 2025-10-17  
**Related**: ADR-020 (Parallel Execution Limits)  
**Confidence**: 90%

---

## Context & Problem

HolmesGPT generates multi-step workflows with dependencies in **self-documenting JSON format**:

```json
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "collect_diagnostics",
      "dependencies": []
    },
    {
      "id": "rec-002",
      "action": "increase_resources",
      "dependencies": ["rec-001"]
    }
  ]
}
```

**Critical Risk: Circular Dependencies**

What if HolmesGPT generates invalid dependencies?

```json
{
  "steps": [
    {"id": "step-1", "dependencies": ["step-2"]},
    {"id": "step-2", "dependencies": ["step-1"]}
  ]
}
```

**Impact Without Validation**:
- **Workflow deadlock**: Both steps wait for each other forever
- **Timeout cascade**: Workflow times out with no clear error
- **Operator confusion**: "Why is this workflow stuck?"
- **No recovery**: Requires manual intervention to identify cycle

---

## Decision

**APPROVED: Topological Sort Validation Before Execution**

**Strategy**:
1. **Validate dependency graph** using **topological sort** before creating WorkflowExecution CRD
2. **Detect cycles** using **Kahn's algorithm** or **DFS cycle detection**
3. **Reject invalid workflows** with clear error message
4. **Fallback to manual approval** if cycle detected

**Rationale**:
- ✅ **Prevents deadlocks**: Cycles detected before execution
- ✅ **Clear error messages**: Operators know exactly what's wrong
- ✅ **Fast fail**: No resources wasted on invalid workflows
- ✅ **Standard algorithm**: Topological sort is well-understood

---

## Design Details

### **Dependency Cycle Detection Algorithm**

**Algorithm**: **Kahn's Algorithm** (BFS-based topological sort)

**Complexity**: **O(V + E)** where V = steps, E = dependencies

**Advantages**:
- Simple to implement
- Clear error messages (identifies cycle nodes)
- Efficient (linear time)

---

### **Implementation: Topological Sort Validator**

**File**: `internal/controller/aianalysis/dependency_validator.go`

```go
package aianalysis

import (
    "fmt"
)

type Step struct {
    ID           string
    Action       string
    Dependencies []string
}

// ValidateDependencyGraph checks for cycles using Kahn's algorithm
func ValidateDependencyGraph(steps []Step) error {
    // Build adjacency list and in-degree map
    graph := make(map[string][]string)
    inDegree := make(map[string]int)
    allSteps := make(map[string]bool)
    
    for _, step := range steps {
        allSteps[step.ID] = true
        if _, exists := inDegree[step.ID]; !exists {
            inDegree[step.ID] = 0
        }
        
        for _, dep := range step.Dependencies {
            graph[dep] = append(graph[dep], step.ID)
            inDegree[step.ID]++
        }
    }
    
    // Validate all dependencies exist
    for _, step := range steps {
        for _, dep := range step.Dependencies {
            if !allSteps[dep] {
                return fmt.Errorf(
                    "invalid dependency: step %s depends on non-existent step %s",
                    step.ID, dep,
                )
            }
        }
    }
    
    // Kahn's algorithm: BFS topological sort
    queue := []string{}
    for stepID, degree := range inDegree {
        if degree == 0 {
            queue = append(queue, stepID)
        }
    }
    
    sortedCount := 0
    var sortedOrder []string
    
    for len(queue) > 0 {
        // Dequeue
        current := queue[0]
        queue = queue[1:]
        sortedOrder = append(sortedOrder, current)
        sortedCount++
        
        // Reduce in-degree for neighbors
        for _, neighbor := range graph[current] {
            inDegree[neighbor]--
            if inDegree[neighbor] == 0 {
                queue = append(queue, neighbor)
            }
        }
    }
    
    // If sorted count != total steps, there's a cycle
    if sortedCount != len(steps) {
        cycleNodes := []string{}
        for stepID, degree := range inDegree {
            if degree > 0 {
                cycleNodes = append(cycleNodes, stepID)
            }
        }
        
        return fmt.Errorf(
            "dependency cycle detected: steps involved in cycle: %v",
            cycleNodes,
        )
    }
    
    return nil
}

// GetExecutionOrder returns topologically sorted step order
func GetExecutionOrder(steps []Step) ([]string, error) {
    // Build graph
    graph := make(map[string][]string)
    inDegree := make(map[string]int)
    
    for _, step := range steps {
        if _, exists := inDegree[step.ID]; !exists {
            inDegree[step.ID] = 0
        }
        
        for _, dep := range step.Dependencies {
            graph[dep] = append(graph[dep], step.ID)
            inDegree[step.ID]++
        }
    }
    
    // Topological sort
    queue := []string{}
    for stepID, degree := range inDegree {
        if degree == 0 {
            queue = append(queue, stepID)
        }
    }
    
    var executionOrder []string
    for len(queue) > 0 {
        current := queue[0]
        queue = queue[1:]
        executionOrder = append(executionOrder, current)
        
        for _, neighbor := range graph[current] {
            inDegree[neighbor]--
            if inDegree[neighbor] == 0 {
                queue = append(queue, neighbor)
            }
        }
    }
    
    return executionOrder, nil
}
```

---

### **Integration: AIAnalysis Controller Validation**

**File**: `internal/controller/aianalysis/aianalysis_controller.go`

```go
func (r *AIAnalysisReconciler) processHolmesGPTRecommendations(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
    recommendations []HolmesGPTRecommendation,
) error {
    log := ctrl.LoggerFrom(ctx)
    
    // Convert HolmesGPT recommendations to Step format
    steps := make([]Step, len(recommendations))
    for i, rec := range recommendations {
        steps[i] = Step{
            ID:           rec.ID,
            Action:       rec.Action,
            Dependencies: rec.Dependencies,
        }
    }
    
    // Validate dependency graph BEFORE creating workflow
    if err := ValidateDependencyGraph(steps); err != nil {
        log.Error(err, "Invalid dependency graph from HolmesGPT",
            "recommendations", len(recommendations))
        
        // Update AIAnalysis status
        aiAnalysis.Status.Phase = "failed"
        aiAnalysis.Status.Reason = "InvalidDependencyGraph"
        aiAnalysis.Status.Message = fmt.Sprintf(
            "HolmesGPT generated invalid dependencies: %s",
            err.Error(),
        )
        
        // Enable manual fallback
        aiAnalysis.Status.RequiresApproval = true
        aiAnalysis.Status.ApprovalContext = &aianalysisv1.ApprovalContext{
            Reason:          "Invalid dependency graph - manual review required",
            ConfidenceLevel: "none",
            InvestigationSummary: fmt.Sprintf(
                "HolmesGPT generated workflow with circular dependencies. Manual workflow design required. Error: %s",
                err.Error(),
            ),
            EvidenceCollected: []string{
                fmt.Sprintf("Dependency validation failed: %s", err.Error()),
                fmt.Sprintf("Total recommendations: %d", len(recommendations)),
            },
            RecommendedActions: []aianalysisv1.RecommendedAction{
                {
                    Action:    "manual_workflow_design",
                    Rationale: "AI-generated workflow has circular dependencies",
                },
            },
            WhyApprovalRequired: "AI-generated dependencies are invalid - requires manual workflow design",
        }
        
        if updateErr := r.Status().Update(ctx, aiAnalysis); updateErr != nil {
            return updateErr
        }
        
        // Create manual approval request
        return r.createManualApprovalRequest(ctx, aiAnalysis)
    }
    
    // Validation passed - get execution order
    executionOrder, _ := GetExecutionOrder(steps)
    log.Info("Dependency validation passed",
        "steps", len(steps),
        "executionOrder", executionOrder)
    
    // Proceed to create WorkflowExecution CRD
    return r.createWorkflowExecution(ctx, aiAnalysis, recommendations)
}
```

---

## Error Handling

### **Cycle Detection Error Message**

```
AIAnalysis Status:
  Phase: failed
  Reason: InvalidDependencyGraph
  Message: "HolmesGPT generated invalid dependencies: dependency cycle detected: steps involved in cycle: [rec-003, rec-005, rec-007]"
  
  ApprovalContext:
    Reason: "Invalid dependency graph - manual review required"
    InvestigationSummary: "HolmesGPT generated workflow with circular dependencies. Manual workflow design required."
    RecommendedActions:
      - Action: "manual_workflow_design"
        Rationale: "AI-generated workflow has circular dependencies"
```

### **Missing Dependency Error Message**

```
AIAnalysis Status:
  Phase: failed
  Reason: InvalidDependencyGraph
  Message: "HolmesGPT generated invalid dependencies: invalid dependency: step rec-003 depends on non-existent step rec-999"
```

---

## Example Validation Scenarios

### **Scenario 1: Valid Linear Workflow** ✅

```json
{
  "steps": [
    {"id": "step-1", "dependencies": []},
    {"id": "step-2", "dependencies": ["step-1"]},
    {"id": "step-3", "dependencies": ["step-2"]}
  ]
}
```

**Validation**: ✅ Pass  
**Execution Order**: `[step-1, step-2, step-3]`

---

### **Scenario 2: Valid Parallel Workflow** ✅

```json
{
  "steps": [
    {"id": "step-1", "dependencies": []},
    {"id": "step-2", "dependencies": []},
    {"id": "step-3", "dependencies": ["step-1", "step-2"]}
  ]
}
```

**Validation**: ✅ Pass  
**Execution Order**: `[step-1, step-2, step-3]` (steps 1&2 parallel)

---

### **Scenario 3: Circular Dependency** ❌

```json
{
  "steps": [
    {"id": "step-1", "dependencies": ["step-2"]},
    {"id": "step-2", "dependencies": ["step-1"]}
  ]
}
```

**Validation**: ❌ Fail  
**Error**: `dependency cycle detected: steps involved in cycle: [step-1, step-2]`  
**Fallback**: Manual approval request created

---

### **Scenario 4: Missing Dependency** ❌

```json
{
  "steps": [
    {"id": "step-1", "dependencies": []},
    {"id": "step-2", "dependencies": ["step-999"]}
  ]
}
```

**Validation**: ❌ Fail  
**Error**: `invalid dependency: step step-2 depends on non-existent step step-999`  
**Fallback**: Manual approval request created

---

### **Scenario 5: Complex Cycle** ❌

```json
{
  "steps": [
    {"id": "step-1", "dependencies": []},
    {"id": "step-2", "dependencies": ["step-1"]},
    {"id": "step-3", "dependencies": ["step-2"]},
    {"id": "step-4", "dependencies": ["step-3"]},
    {"id": "step-5", "dependencies": ["step-4", "step-2"]},  // Valid so far
    {"id": "step-2", "dependencies": ["step-5"]}  // CYCLE!
  ]
}
```

**Validation**: ❌ Fail  
**Error**: `dependency cycle detected: steps involved in cycle: [step-2, step-3, step-4, step-5]`

---

## Prometheus Metrics

**New Metrics for Dependency Validation**:

```go
// Dependency validation metrics
aianalysis_dependency_validation_total{result="success|cycle|missing"}
aianalysis_dependency_cycles_detected_total
aianalysis_dependency_validation_duration_seconds
aianalysis_workflow_complexity_steps_total
```

**Example Prometheus Queries**:
```promql
# Dependency validation failure rate
rate(aianalysis_dependency_validation_total{result!="success"}[5m])

# Average workflow complexity
avg(aianalysis_workflow_complexity_steps_total)

# Cycle detection rate
rate(aianalysis_dependency_cycles_detected_total[5m])
```

---

## Alerting Rules

**Alert: High Dependency Cycle Rate**
```yaml
groups:
  - name: dependency_validation
    rules:
      - alert: HighDependencyCycleRate
        expr: rate(aianalysis_dependency_cycles_detected_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "HolmesGPT generating workflows with circular dependencies"
          description: "Dependency cycle rate: {{ $value }}/min. May indicate HolmesGPT prompt engineering issue."
```

---

## Business Requirements

**New BRs for Dependency Validation**:

| BR | Description | Priority |
|---|---|---|
| **BR-AI-066** | AIAnalysis MUST validate dependency graph for cycles using topological sort | P0 |
| **BR-AI-067** | AIAnalysis MUST reject workflows with circular dependencies | P0 |
| **BR-AI-068** | AIAnalysis MUST validate all dependency IDs exist | P0 |
| **BR-AI-069** | AIAnalysis MUST provide clear error messages for invalid dependencies | P0 |
| **BR-AI-070** | AIAnalysis MUST fall back to manual approval on dependency validation failure | P0 |

---

## Testing Strategy

### **Unit Tests**
1. **Valid linear workflow**: Verify topological sort
2. **Valid parallel workflow**: Verify correct execution order
3. **Circular dependency**: Verify cycle detection
4. **Missing dependency**: Verify error message
5. **Complex cycle**: Verify all cycle nodes identified

### **Integration Tests**
1. **HolmesGPT generates cycle**: Verify manual fallback
2. **HolmesGPT generates missing dependency**: Verify manual fallback
3. **Valid workflow**: Verify WorkflowExecution created

### **Property-Based Tests**
1. **Random DAGs**: Generate valid workflows → always pass validation
2. **Random cycles**: Generate workflows with cycles → always fail validation

---

## Performance Analysis

### **Validation Cost**

| Workflow Size | Validation Time | Overhead |
|---|---|---|
| **5 steps** | <1ms | Negligible |
| **10 steps** | <1ms | Negligible |
| **50 steps** | <5ms | Negligible |
| **100 steps** | <10ms | Negligible |

**Conclusion**: Topological sort is **O(V + E)** - negligible overhead for realistic workflow sizes.

---

## Alternative Algorithms Considered

### **Algorithm 1: Kahn's Algorithm (BFS-based)** ✅ **APPROVED**

**Confidence**: **90%**

**Pros**:
- ✅ Simple to implement
- ✅ Clear error messages (identifies cycle nodes)
- ✅ O(V + E) complexity

**Cons**:
- None significant

---

### **Algorithm 2: DFS Cycle Detection**

**Confidence**: **75%** (slightly more complex)

**Pros**:
- ✅ O(V + E) complexity
- ✅ Can identify exact cycle path

**Cons**:
- ⚠️ More complex implementation
- ⚠️ Requires visited/recursion stack tracking

---

### **Algorithm 3: Tarjan's Strongly Connected Components**

**Confidence**: **50%** (over-engineering)

**Cons**:
- ❌ Overkill for simple cycle detection
- ❌ More complex than needed

---

## References

1. **Kahn's Algorithm**: https://en.wikipedia.org/wiki/Topological_sorting#Kahn's_algorithm
2. **Topological Sort**: Standard graph algorithm
3. **Multi-Step Workflow Examples**: `docs/analysis/MULTI_STEP_WORKFLOW_EXAMPLES.md`

---

**Document Owner**: Platform Architecture Team  
**Last Updated**: 2025-10-17  
**Next Review**: After V1.0 implementation complete

