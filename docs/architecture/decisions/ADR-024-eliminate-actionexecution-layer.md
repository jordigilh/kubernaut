# ADR-024: Eliminate ActionExecution CRD Layer

**Status**: ✅ Approved
**Date**: 2025-10-19
**Deciders**: Architecture Team
**Priority**: FOUNDATIONAL
**Updates**: [ADR-023: Tekton from V1](ADR-023-tekton-from-v1.md)

---

## Context and Problem Statement

During architectural review of [ADR-023: Tekton from V1](ADR-023-tekton-from-v1.md), the ActionExecution CRD was proposed as a "tracking layer" between WorkflowExecution and Tekton TaskRuns, with claimed benefits for:
1. Business abstraction
2. Pattern monitoring
3. Effectiveness tracking
4. Multi-target execution flexibility
5. Audit trail

**Critical Questions**:
1. Does ActionExecution provide value beyond duplicate data?
2. Should business context live in execution primitives?
3. Do pattern monitoring and effectiveness tracking need CRDs?
4. Does multi-target execution require separate executor controllers?

---

## Decision Drivers

### **1. Business Context Belongs in Business CRDs** 🎯

**Analysis**: Business context (remediationID, confidence, pattern) should live in:
- ✅ **RemediationRequest CRD**: Overall remediation context and lifecycle
- ✅ **WorkflowExecution CRD**: Workflow-level metadata and execution details
- ✅ **Data Storage Service**: Long-term historical data and analytics

**ActionExecution would contain**:
- Action type (already in WorkflowExecution.Spec.Steps)
- Image (already in WorkflowExecution.Spec.Steps)
- Inputs (already in WorkflowExecution.Spec.Steps)
- Execution status (available in Tekton TaskRun)

**Conclusion**: ActionExecution would be **duplicate data** with no unique business value.

---

### **2. Pattern Monitoring Queries Database, Not CRDs** 📊

**From Effectiveness Monitor Specification**:
```go
// Step 3: Action History Retrieval (50-100ms)
// Queries Data Storage Service for 90-day action history
history, err := s.dataStorageClient.GetActionHistory(ctx, "restart-pod", 90*24*time.Hour)

// Returns action history from PostgreSQL, NOT from CRDs
// CRDs have 24h TTL, analytics require 90+ days
```

**Reality**:
- ❌ **CRDs**: 24h TTL (ephemeral coordination primitives)
- ✅ **Data Storage Service**: 90+ day historical data (persistent analytics storage)

**Conclusion**: Pattern monitoring has **zero dependency** on ActionExecution CRDs.

---

### **3. Effectiveness Tracking Uses RemediationRequest + Database** 📈

**From Effectiveness Monitor Specification**:
```go
// Effectiveness Monitor watches RemediationRequest CRDs
func (r *EffectivenessMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) {
    var rr remediationv1.RemediationRequest
    r.Get(ctx, req.NamespacedName, &rr)

    // Process completed/failed/timeout remediations
    if rr.Status.OverallPhase == "completed" || rr.Status.OverallPhase == "failed" {
        // Query Data Storage Service for action history (NOT CRDs)
        history, err := r.dataStorageClient.GetActionHistory(...)

        // Calculate effectiveness based on DB data
        effectiveness := r.calculateEffectiveness(history)

        // Store results in Data Storage Service (NOT CRDs)
        r.dataStorageClient.PersistAssessment(effectiveness)
    }
}
```

**Reality**:
- ✅ **Triggers**: Watches RemediationRequest CRD
- ✅ **Data source**: Queries Data Storage Service (not ActionExecution)
- ✅ **Storage**: Persists to Data Storage Service (not ActionExecution)

**Conclusion**: Effectiveness tracking has **zero dependency** on ActionExecution CRDs.

---

### **4. Multi-Target Execution via Container Images, Not Controllers** 🐳

**Question**: How to support Kubernetes, GitOps, AWS executors?

**Option A: Separate Executor Controllers** ❌
```
WorkflowExecution → ActionExecution → {
    KubernetesExecutor (creates TaskRun),
    GitOpsExecutor (creates TaskRun),
    AWSExecutor (creates TaskRun)
}
```

**Problem**: Controller proliferation, complex routing logic

**Option B: Generic Task + Specialized Containers** ✅
```yaml
# Single generic Tekton Task executes ANY container
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: kubernaut-action
spec:
  params:
    - name: actionImage  # ghcr.io/kubernaut/actions/{k8s|gitops|aws}@sha256:...
    - name: inputs
  steps:
    - image: $(params.actionImage)  # Specialized container handles target
```

**Container Images**:
- `ghcr.io/kubernaut/actions/kubectl@sha256:...` - Kubernetes operations
- `ghcr.io/kubernaut/actions/argocd@sha256:...` - GitOps PR creation
- `ghcr.io/kubernaut/actions/aws-cli@sha256:...` - AWS operations

**Conclusion**: Multi-target execution handled by **container images**, not separate controllers. ActionExecution provides **no value**.

---

### **5. Tekton API Changes Only Affect WorkflowExecution Controller** 🔧

**Claim**: "ActionExecution isolates business logic from Tekton API changes"

**Reality**:
- ✅ **WorkflowExecution controller**: Creates Tekton PipelineRuns (affected by Tekton API)
- ❌ **Pattern Monitoring**: Queries Data Storage Service (NOT affected)
- ❌ **Effectiveness Tracking**: Queries Data Storage Service (NOT affected)
- ❌ **Data Storage Service**: Stores action records (NOT affected)

**Impact of Tekton API changes**:
- ✅ **With ActionExecution**: WorkflowExecution + ActionExecution controllers need updates (2 controllers)
- ✅ **Without ActionExecution**: WorkflowExecution controller needs updates (1 controller)

**Conclusion**: ActionExecution **increases** complexity without meaningful isolation benefit.

---

## Decision Outcome

**Chosen option**: **"Eliminate ActionExecution CRD"**

**Rationale**:
1. ✅ **Business context** belongs in RemediationRequest/WorkflowExecution (not execution primitives)
2. ✅ **Pattern monitoring** queries Data Storage Service (not CRDs)
3. ✅ **Effectiveness tracking** watches RemediationRequest + queries DB (not ActionExecution)
4. ✅ **Multi-target execution** handled by container images (not separate controllers)
5. ✅ **Tekton API changes** only affect WorkflowExecution controller (acceptable)
6. ✅ **Architectural simplicity**: One less CRD, one less controller
7. ✅ **Lower latency**: No intermediate CRD creation

---

## Simplified Architecture

### **Final Architecture** (No ActionExecution)

```
┌─────────────────────────────────────────────────────────┐
│ RemediationOrchestrator                                  │
│ Creates: RemediationRequest CRD                          │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ WorkflowExecution Controller                             │
│ - Translates WorkflowExecution → Tekton PipelineRun     │
│ - Monitors PipelineRun status                           │
│ - Syncs status to WorkflowExecution CRD                 │
│ - Writes action records to Data Storage Service         │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Tekton Pipelines                                         │
│ - Creates TaskRuns for each workflow step               │
│ - Executes generic meta-task with action containers     │
│ - Handles DAG orchestration, retry, workspace           │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Pod (Action Container)                                   │
│ - K8s actions: kubectl container                         │
│ - GitOps actions: argocd container                       │
│ - AWS actions: aws-cli container                         │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Data Storage Service                                     │
│ - Stores action history (90+ days)                      │
│ - Stores effectiveness metrics                          │
│ - Queried by Pattern Monitoring                         │
│ - Queried by Effectiveness Monitor                      │
└─────────────────────────────────────────────────────────┘
                          ↑
┌─────────────────────────────────────────────────────────┐
│ Effectiveness Monitor                                    │
│ - Watches RemediationRequest CRDs                       │
│ - Queries Data Storage for action history               │
│ - Calculates effectiveness                              │
│ - Stores results in Data Storage                        │
└─────────────────────────────────────────────────────────┘
```

---

### **WorkflowExecution Controller Implementation**

```go
package controller

import (
    "context"
    "encoding/json"

    workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
    ctrl "sigs.k8s.io/controller-runtime"
)

// WorkflowExecutionReconciler creates Tekton PipelineRuns directly
type WorkflowExecutionReconciler struct {
    client.Client
    DataStorageClient *DataStorageClient  // For action record persistence
}

func (r *WorkflowExecutionReconciler) Reconcile(
    ctx context.Context,
    req ctrl.Request,
) (ctrl.Result, error) {
    workflow := &workflowv1.WorkflowExecution{}
    if err := r.Get(ctx, req.NamespacedName, workflow); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    switch workflow.Status.Phase {
    case "":
        return r.handleInitialization(ctx, workflow)
    case "Initializing":
        return r.handlePipelineRunCreation(ctx, workflow)
    case "Executing":
        return r.handlePipelineRunMonitoring(ctx, workflow)
    default:
        return ctrl.Result{}, nil
    }
}

// handlePipelineRunCreation creates Tekton PipelineRun directly
func (r *WorkflowExecutionReconciler) handlePipelineRunCreation(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    // Create Tekton PipelineRun (no intermediate ActionExecution)
    pipelineRun := r.createPipelineRun(workflow)
    if err := r.Create(ctx, pipelineRun); err != nil {
        return ctrl.Result{}, err
    }

    // Write action records to Data Storage Service
    for _, step := range workflow.Spec.Steps {
        actionRecord := &ActionRecord{
            WorkflowID:  workflow.Name,
            ActionType:  step.ActionType,
            Image:       step.Image,
            Inputs:      step.Inputs,
            ExecutedAt:  time.Now(),
            Status:      "executing",
        }
        r.DataStorageClient.RecordAction(ctx, actionRecord)
    }

    workflow.Status.Phase = "Executing"
    return ctrl.Result{Requeue: true}, r.Status().Update(ctx, workflow)
}

// createPipelineRun translates WorkflowExecution to Tekton PipelineRun
func (r *WorkflowExecutionReconciler) createPipelineRun(
    workflow *workflowv1.WorkflowExecution,
) *tektonv1.PipelineRun {
    tasks := make([]tektonv1.PipelineTask, len(workflow.Spec.Steps))

    for i, step := range workflow.Spec.Steps {
        inputsJSON, _ := json.Marshal(step.Inputs)

        tasks[i] = tektonv1.PipelineTask{
            Name: step.Name,
            TaskRef: &tektonv1.TaskRef{
                Name: "kubernaut-action",  // Generic meta-task
            },
            Params: []tektonv1.Param{
                {Name: "actionType", Value: tektonv1.ParamValue{StringVal: step.ActionType}},
                {Name: "actionImage", Value: tektonv1.ParamValue{StringVal: step.Image}},
                {Name: "inputs", Value: tektonv1.ParamValue{StringVal: string(inputsJSON)}},
            },
            RunAfter: step.RunAfter,  // Tekton handles dependencies
        }
    }

    return &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      workflow.Name,
            Namespace: workflow.Namespace,
            Labels: map[string]string{
                "kubernaut.io/workflow": workflow.Name,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(workflow, workflowv1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: tektonv1.PipelineRunSpec{
            PipelineSpec: &tektonv1.PipelineSpec{
                Tasks: tasks,
            },
        },
    }
}
```

---

## Consequences

### **Positive Consequences** ✅

#### **1. Architectural Simplicity**
- ✅ One less CRD (ActionExecution eliminated)
- ✅ One less controller (ActionExecution controller eliminated)
- ✅ Clearer data flow: Workflow → Tekton → Data Storage

#### **2. Performance Improvement**
- ✅ Lower latency: No intermediate ActionExecution CRD creation (~50ms saved per action)
- ✅ Reduced etcd load: Fewer CRD writes

#### **3. Cleaner Separation of Concerns**
- ✅ **Business data**: RemediationRequest + WorkflowExecution + Data Storage Service
- ✅ **Execution primitives**: Tekton PipelineRun + TaskRun
- ✅ **Analytics**: Data Storage Service (not CRDs)

#### **4. Direct Integration**
- ✅ WorkflowExecution controller directly manages Tekton PipelineRuns
- ✅ No abstraction layer overhead
- ✅ Simpler to understand and debug

---

### **Negative Consequences** (Mitigated)

#### **1. Tekton API Coupling** ⚠️

**Concern**: WorkflowExecution controller directly coupled to Tekton API

**Mitigation**:
- ✅ Acceptable trade-off (single controller affected)
- ✅ Tekton API is CNCF Graduated (stable)
- ✅ Migration to different executor (if ever needed) is straightforward

**Residual Risk**: Very Low

---

#### **2. Observability via Tekton Primitives** ⚠️

**Concern**: Debugging requires understanding Tekton TaskRuns (not Kubernaut ActionExecution)

**Mitigation**:
- ✅ Tekton Dashboard provides rich visualization
- ✅ Tekton CLI (`tkn`) provides debugging commands
- ✅ RemediationRequest + WorkflowExecution provide business-level view
- ✅ Data Storage Service provides historical analytics

**Residual Risk**: Very Low (multiple observability layers)

---

## Related Decisions

- **[ADR-023: Tekton from V1](ADR-023-tekton-from-v1.md)** - Updated to remove ActionExecution layer
- **[Effectiveness Monitor Specification](../../services/stateless/effectiveness-monitor/overview.md)** - Watches RemediationRequest, queries DB

---

## Migration Impact

### **Services Affected**

| Service | Impact | Action |
|---------|--------|--------|
| **WorkflowExecution Controller** | Creates Tekton PipelineRun directly | Update implementation |
| **ActionExecution Controller** | ❌ **Eliminated** | Delete service |
| **Pattern Monitoring** | No change (already queries DB) | None |
| **Effectiveness Monitor** | No change (already queries DB) | None |
| **Data Storage Service** | No change | None |

### **CRDs Affected**

| CRD | Impact | Action |
|-----|--------|--------|
| **ActionExecution** | ❌ **Eliminated** | Delete CRD definition |
| **WorkflowExecution** | No schema change | Update controller logic |
| **RemediationRequest** | No change | None |

---

## Links

### **Business Requirements**:
- **BR-REMEDIATION-001**: Multi-step workflow orchestration
  - Fulfilled: ✅ Via Tekton Pipelines

- **BR-REMEDIATION-002**: Parallel execution support
  - Fulfilled: ✅ Via Tekton `runAfter` dependencies

- **BR-MONITORING-001**: Pattern monitoring
  - Fulfilled: ✅ Via Data Storage Service queries

- **BR-MONITORING-002**: Effectiveness tracking
  - Fulfilled: ✅ Via Effectiveness Monitor + Data Storage Service

---

## Decision Record

**Status**: ✅ Approved
**Decision Date**: 2025-10-19
**Approved By**: Architecture Team
**Implementation Target**: Q4 2025
**Confidence**: **95%** (Very High)

**Key Insight**: **ActionExecution was architectural complexity without value**. Business data belongs in business CRDs (RemediationRequest, WorkflowExecution) and persistent storage (Data Storage Service), not in ephemeral execution primitives.

**Updates**: [ADR-023](ADR-023-tekton-from-v1.md) updated to reflect simplified architecture.






