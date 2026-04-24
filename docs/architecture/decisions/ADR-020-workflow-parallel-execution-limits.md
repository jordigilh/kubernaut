# ADR-020: Workflow Parallel Execution Limits & Complexity Approval

**Status**: ✅ **APPROVED**
**Date**: 2025-10-17
**Updated**: 2026-04-09 (execution via Tekton TaskRun / PipelineRun per ADR-023/025; limits still apply to parallel work)
**Related**: ADR-019 (HolmesGPT Retry Strategy), ADR-021 (Dependency Validation), ADR-023 (Tekton), ADR-025 (KubernetesExecutor elimination)
**Confidence**: 90%

---

## Context & Problem

Multi-step workflows support **parallel execution** for steps with no dependencies. With **Tekton** ([ADR-023](./ADR-023-tekton-from-v1.md), [ADR-025](./ADR-025-kubernetesexecutor-service-elimination.md)), each ready step is represented by concurrent **TaskRuns** (or parallel tasks within a `PipelineRun`), which in turn schedule Kubernetes **Jobs** / Pods:

```yaml
steps:
  - stepNumber: 1
    action: "collect_diagnostics"
    dependencies: []  # TaskRun / pipeline task can start immediately
  - stepNumber: 2
    action: "backup_data"
    dependencies: []  # May run in parallel with other ready steps
  - stepNumber: 3
    action: "health_check"
    dependencies: []  # May run in parallel with other ready steps
```

**Critical Questions**:
1. **What if 50 steps all have no dependencies?** Start 50 TaskRuns (or equivalent) simultaneously?
2. **What happens to Kubernetes API?** Rate limits exhausted?
3. **What about operational complexity?** 50 parallel Jobs overwhelming cluster?

**Impact Without Limits**:
- **API rate limit exhaustion**: Kubernetes rejects create requests (CRDs, TaskRuns, Pods)
- **Cluster resource exhaustion**: 50 simultaneous Jobs consume all resources
- **Operational complexity**: Operators cannot track 50 parallel executions
- **Debugging nightmare**: Identifying which of 50 Jobs failed

**Key Architectural Clarification**:
- ❌ **NOT unbounded goroutines**: Concurrency must be bounded in the controller
- ✅ **Tekton TaskRun / PipelineRun**: Steps materialize as Tekton workloads → Kubernetes Jobs/Pods
- ✅ **WorkflowExecution controller**: Reconciles workflow, respects parallelism caps when creating or admitting work

---

## Decision

**APPROVED: Parallel execution limit + Complexity-Based Approval** (unchanged intent; implementation targets Tekton)

**Strategy**:
1. **Max parallel step execution**: **5 concurrent steps** per workflow (configurable), enforced when starting TaskRuns / pipeline tasks (not via a `KubernetesExecution` CRD)
2. **Complexity approval threshold**: Workflows with **>10 total steps** require manual approval (configurable)
3. **Queuing**: Steps wait when the parallelism budget is full
4. **Client-side rate limiter**: Max **20 QPS** for Kubernetes API calls (configurable)

**Rationale**:
- ✅ **Prevents resource exhaustion**: Bounded concurrent Tekton / API work
- ✅ **Respects Kubernetes limits**: 20 QPS < 50 QPS default
- ✅ **Configurable**: Adjust for different cluster sizes
- ✅ **Standard pattern**: Widely used in Kubernetes controllers

---

## Design Details

### **Parallel Execution Configuration**

**ConfigMap: `kubernaut-workflowexecution-config`**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-workflowexecution-config
  namespace: kubernaut-system
data:
  max-parallel-steps: "5"       # Max concurrent steps (TaskRuns / parallel tasks) per workflow
  complexity-approval-threshold: "10"  # Workflows >10 total steps require approval
  kubernetes-qps: "20"          # Max Kubernetes API QPS
  kubernetes-burst: "30"        # Burst capacity for K8s API
```

**Environment Variables** (overrides ConfigMap):
```bash
MAX_PARALLEL_STEPS=5
COMPLEXITY_APPROVAL_THRESHOLD=10
KUBERNETES_QPS=20
KUBERNETES_BURST=30
```

---

### **Implementation: Parallel step tracker (Tekton)**

**File** (illustrative): `internal/controller/workflowexecution/parallel_executor.go`

The following pseudocode shows the **same policy** applied after ADR-025: bound concurrent work by counting **in-flight steps** (e.g. TaskRuns labeled for the workflow) instead of creating `KubernetesExecution` CRDs.

```
ParallelExecutor (conceptual):
  - maxParallelSteps: configurable cap (e.g. 5)
  - activeSteps: count in-flight Tekton work for this workflow (TaskRuns / running pipeline tasks not terminal)
  - availableSlots = max(0, maxParallelSteps - activeSteps)
  - When reconciling: start at most availableSlots new steps from the dependency-ready queue
  - Implementation detail: create/patch Tekton PipelineRun / TaskRun — see ../TEKTON_EXECUTION_ARCHITECTURE.md
```

---

### **Complexity-Based Approval Logic**

**File**: `internal/controller/aianalysis/aianalysis_controller.go`

```go
// CheckWorkflowComplexity validates if workflow exceeds complexity threshold
func (r *AIAnalysisReconciler) CheckWorkflowComplexity(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
    recommendations []HolmesGPTRecommendation,
) (bool, string) {
    log := ctrl.LoggerFrom(ctx)

    totalSteps := len(recommendations)

    // Check complexity threshold (default: 10 steps)
    if totalSteps > r.ComplexityApprovalThreshold {
        reason := fmt.Sprintf(
            "Workflow complexity exceeds threshold: %d steps (threshold: %d). Manual review required for operational safety.",
            totalSteps,
            r.ComplexityApprovalThreshold,
        )

        log.Info("Workflow complexity approval required",
            "totalSteps", totalSteps,
            "threshold", r.ComplexityApprovalThreshold)

        return true, reason
    }

    return false, ""
}

func (r *AIAnalysisReconciler) processHolmesGPTRecommendations(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
    recommendations []HolmesGPTRecommendation,
) error {
    log := ctrl.LoggerFrom(ctx)

    // 1. Validate dependency graph (no cycles)
    if err := ValidateDependencyGraph(recommendations); err != nil {
        return r.handleInvalidDependencies(ctx, aiAnalysis, err)
    }

    // 2. Check workflow complexity
    requiresApproval, reason := r.CheckWorkflowComplexity(ctx, aiAnalysis, recommendations)
    if requiresApproval {
        // Set approval required
        aiAnalysis.Status.RequiresApproval = true
        aiAnalysis.Status.ApprovalContext = &aianalysisv1.ApprovalContext{
            Reason:          reason,
            ConfidenceScore: aiAnalysis.Status.Confidence,
            ConfidenceLevel: getConfidenceLevel(aiAnalysis.Status.Confidence),
            InvestigationSummary: aiAnalysis.Status.RootCause,
            EvidenceCollected: buildEvidence(aiAnalysis),
            RecommendedActions: convertToRecommendedActions(recommendations),
            WhyApprovalRequired: fmt.Sprintf(
                "Workflow has %d steps (threshold: %d). Complex workflows require manual review to ensure operational safety and verify dependency correctness.",
                len(recommendations),
                r.ComplexityApprovalThreshold,
            ),
        }

        log.Info("Complexity approval required",
            "totalSteps", len(recommendations),
            "threshold", r.ComplexityApprovalThreshold)

        // Update status and create approval request
        aiAnalysis.Status.Phase = "approving"
        if err := r.Status().Update(ctx, aiAnalysis); err != nil {
            return err
        }

        return r.createApprovalRequest(ctx, aiAnalysis)
    }

    // 3. Proceed to create WorkflowExecution (no approval needed)
    return r.createWorkflowExecution(ctx, aiAnalysis, recommendations)
}
```

**Example Complexity Approval Message**:
```yaml
status:
  phase: "approving"
  requiresApproval: true
  approvalContext:
    reason: "Workflow complexity exceeds threshold: 15 steps (threshold: 10). Manual review required for operational safety."
    confidenceScore: 0.85
    confidenceLevel: "high"
    investigationSummary: "Cascading failure in checkout flow due to PostgreSQL connection pool exhaustion"
    recommendedActions:
      - action: "collect_diagnostics"
        rationale: "Capture PostgreSQL metrics"
      - action: "patch_config_map"
        rationale: "Increase connection pool size"
      # ... 13 more steps ...
    whyApprovalRequired: "Workflow has 15 steps (threshold: 10). Complex workflows require manual review to ensure operational safety and verify dependency correctness."
```

---

### **Kubernetes API Rate Limiter**

**File**: `internal/controller/workflowexecution/workflowexecution_controller.go`

```go
package workflowexecution

import (
    "k8s.io/client-go/rest"
    "k8s.io/client-go/util/flowcontrol"
)

func NewWorkflowExecutionReconciler(
    client client.Client,
    config *rest.Config,
    maxParallelSteps int,
    kubernetesQPS float32,
    kubernetesBurst int,
) *WorkflowExecutionReconciler {
    // Configure client-side rate limiter
    config.QPS = kubernetesQPS      // Max 20 QPS
    config.Burst = kubernetesBurst  // Burst capacity 30

    // Create rate limiter
    rateLimiter := flowcontrol.NewTokenBucketRateLimiter(
        kubernetesQPS,
        kubernetesBurst,
    )

    // Create parallel executor with worker pool
    parallelExecutor := NewParallelExecutor(
        maxParallelSteps,
        NewExecutorClient(client, rateLimiter),
    )

    return &WorkflowExecutionReconciler{
        Client:           client,
        ParallelExecutor: parallelExecutor,
        RateLimiter:      rateLimiter,
    }
}
```

---

### **Dependency Graph Analysis**

**File**: `internal/controller/workflowexecution/dependency_graph.go`

```go
package workflowexecution

import (
    "fmt"

    workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// GetParallelExecutableSteps returns steps that can be executed in parallel
func GetParallelExecutableSteps(
    allSteps []workflowv1.WorkflowStep,
    completedSteps map[int]bool,
) []workflowv1.WorkflowStep {
    var parallelSteps []workflowv1.WorkflowStep

    for _, step := range allSteps {
        // Skip if already completed
        if completedSteps[step.StepNumber] {
            continue
        }

        // Check if all dependencies are satisfied
        canExecute := true
        for _, depStepNum := range step.Dependencies {
            if !completedSteps[depStepNum] {
                canExecute = false
                break
            }
        }

        if canExecute {
            parallelSteps = append(parallelSteps, step)
        }
    }

    return parallelSteps
}

// Example workflow execution flow
func (r *WorkflowExecutionReconciler) executeWorkflow(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) error {
    completedSteps := make(map[int]bool)

    for {
        // Get steps that can be executed in parallel
        parallelSteps := GetParallelExecutableSteps(
            workflow.Spec.WorkflowDefinition.Steps,
            completedSteps,
        )

        if len(parallelSteps) == 0 {
            // No more steps to execute
            break
        }

        // Execute parallel steps (worker pool limits concurrency)
        if err := r.ParallelExecutor.ExecuteStepsInParallel(ctx, workflow, parallelSteps); err != nil {
            return fmt.Errorf("parallel execution failed: %w", err)
        }

        // Mark steps as completed
        for _, step := range parallelSteps {
            completedSteps[step.StepNumber] = true
        }
    }

    return nil
}
```

---

## Prometheus Metrics

**New Metrics for Parallel Execution**:

```go
// Worker pool metrics
workflow_parallel_workers_active
workflow_parallel_workers_max
workflow_parallel_steps_queued
workflow_parallel_execution_duration_seconds

// Kubernetes API rate limiting metrics
kubernetes_api_requests_total
kubernetes_api_requests_throttled_total
kubernetes_api_rate_limiter_wait_duration_seconds
```

**Example Prometheus Queries**:
```promql
# Worker pool utilization
workflow_parallel_workers_active / workflow_parallel_workers_max

# Kubernetes API throttling rate
rate(kubernetes_api_requests_throttled_total[5m])

# Average parallel steps per workflow
avg(workflow_parallel_steps_queued)
```

---

## Performance Analysis

### **Scenario: 50-Step Workflow (All Parallel)**

**Without Limits** (❌ Unsafe):
- **Goroutines created**: 50 simultaneous
- **Kubernetes API calls**: 50 simultaneous (exceeds 50 QPS limit)
- **Result**: API rate limit exhaustion → all steps fail

**With Limits** (✅ Safe):
- **Worker pool size**: 10
- **Execution waves**: 5 waves (10 steps per wave)
- **Kubernetes API calls**: Max 20 QPS (respects limits)
- **Result**: Controlled execution, no rate limit exhaustion

**Performance Comparison**:

| Metric | Without Limits | With Limits | Difference |
|---|---|---|---|
| **Total Duration** | 30s (if API allows) | 35s (5 waves × 7s/wave) | **+5s (+17%)** |
| **API Failures** | 100% (rate limited) | 0% | **-100%** |
| **Memory Usage** | 500MB (50 goroutines) | 50MB (10 goroutines) | **-90%** |
| **Reliability** | ❌ Fails | ✅ Succeeds | **+100%** |

**Conclusion**: **5-second overhead is acceptable** for 100% reliability improvement.

---

## Configuration Tuning Guide

### **Small Clusters (1-10 nodes)**
```yaml
max-parallel-steps: "5"
kubernetes-qps: "10"
kubernetes-burst: "15"
```

### **Medium Clusters (10-50 nodes)**
```yaml
max-parallel-steps: "10"   # Default
kubernetes-qps: "20"        # Default
kubernetes-burst: "30"      # Default
```

### **Large Clusters (50-100+ nodes)**
```yaml
max-parallel-steps: "20"
kubernetes-qps: "40"
kubernetes-burst: "60"
```

---

## Business Requirements

**New BRs for Parallel Execution Safety**:

| BR | Description | Priority |
|---|---|---|
| **BR-WF-166** | WorkflowExecution MUST limit parallel step execution to 10 concurrent steps (configurable) | P0 |
| **BR-WF-167** | WorkflowExecution MUST use client-side rate limiter for Kubernetes API (20 QPS default) | P0 |
| **BR-WF-168** | WorkflowExecution MUST queue steps when worker pool full | P0 |
| **BR-WF-169** | WorkflowExecution parallel limits MUST be configurable via ConfigMap | P1 |

---

## Testing Strategy

### **Unit Tests**
1. **Worker pool**: Verify max 10 concurrent steps
2. **Queuing**: Verify steps wait when pool full
3. **Rate limiter**: Verify QPS limits respected

### **Integration Tests**
1. **50-step workflow**: All parallel, verify controlled execution
2. **Rate limit exhaustion**: Simulate K8s API throttling → verify retry
3. **Worker pool saturation**: Verify graceful queuing

### **Performance Tests**
1. **100 concurrent workflows**: Verify no resource exhaustion
2. **Kubernetes API load**: Measure QPS, verify <20 QPS

---

## References

1. **Kubernetes Client-Go Rate Limiting**: https://github.com/kubernetes/client-go/blob/master/util/flowcontrol/throttle.go
2. **Worker Pool Pattern**: Standard Go concurrency pattern
3. **Multi-Step Workflow Examples**: MULTI_STEP_WORKFLOW_EXAMPLES (document removed)

---

**Document Owner**: Platform Architecture Team
**Last Updated**: 2025-10-17
**Next Review**: After V1.0 implementation complete

