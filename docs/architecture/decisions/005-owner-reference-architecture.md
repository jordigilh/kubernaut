# Owner Reference Architecture - CRD Ownership Hierarchy

**Date**: 2025-10-03
**Status**: ✅ **DEFINED**

---

## 📋 **OWNERSHIP HIERARCHY**

### **Visual Diagram**

```
┌─────────────────────────────────────────────────────────────┐
│            RemediationRequest (Root - Central Orchestrator)    │
│                   ⚠️  NO OWNER REFERENCE                     │
│              (Manually created or webhook-triggered)         │
│                                                              │
│  Responsibility: Watch all service CRDs and orchestrate      │
│  workflow by creating next service CRD based on completion   │
└────────────────────────┬────────────────────────────────────┘
                         │
                         │ (Creates & watches ALL service CRDs)
                         │
        ┌────────────────┼────────────────┬────────────────┬────────────────┐
        │                │                │                │                │
        ▼                ▼                ▼                ▼                ▼
  RemediationProcessing   AIAnalysis   WorkflowExecution  KubernetesExecution  AIApprovalRequest
   (Sibling 1)      (Sibling 2)     (Sibling 3)        (Sibling 4)       (Optional)
        │                │                │                │                │
        │                │                │                │                │
   📊 Enriches      🤖 Analyzes      🔄 Orchestrates   ⚙️  Executes      ✅ Approves
    Alert Data      Root Cause        Steps             K8s Ops          Risky Actions
        │                │                │                │                │
        ▼                ▼                ▼                ▼                ▼
   Updates status   Updates status   Updates status    Updates status   Updates status
        │                │                │                │                │
        └────────────────┴────────────────┴────────────────┴────────────────┘
                                        │
                                        ▼
                         RemediationRequest watches all statuses
                         and creates next CRD when ready
```

---

## 🔗 **OWNER REFERENCE RELATIONSHIPS**

### **Ownership Table**

| CRD | Owned By | Owns | Level | Cascade Delete |
|-----|----------|------|-------|----------------|
| **RemediationRequest** | None (root) | ALL service CRDs | 1 | ✅ Deletes all children |
| **RemediationProcessing** | RemediationRequest | None | 2 | ✅ Deleted when RemediationRequest deleted |
| **AIAnalysis** | RemediationRequest | None | 2 | ✅ Deleted when RemediationRequest deleted |
| **WorkflowExecution** | RemediationRequest | None | 2 | ✅ Deleted when RemediationRequest deleted |
| **KubernetesExecution** | RemediationRequest | None | 2 | ✅ Deleted when RemediationRequest deleted |
| **AIApprovalRequest** | RemediationRequest | None | 2 | ✅ Deleted when RemediationRequest deleted |

**Key Point**: All service CRDs are **siblings** at level 2. RemediationRequest is the **single orchestrator** that creates all service CRDs based on sequential workflow progression.

---

## 📊 **DATA FLOW VS OWNERSHIP - CRITICAL DISTINCTION**

### **Centralized Orchestration Pattern**

**RemediationRequest creates ALL service CRDs** and orchestrates the workflow:

```
                    RemediationRequest (Central Orchestrator)
                            │
        ┌───────────────────┼───────────────────┬───────────────────┐
        │ (owns)            │ (owns)            │ (owns)            │ (owns)
        ▼                   ▼                   ▼                   ▼
  RemediationProcessing      AIAnalysis      WorkflowExecution   KubernetesExecution
        │                   │                   │                   │
   status.phase=         status.phase=      status.phase=      status.phase=
   "completed"          "completed"        "completed"        "completed"
        │                   │                   │                   │
        └───────────────────┴───────────────────┴───────────────────┘
                            │
                            ▼
              RemediationRequest watches all statuses
              and creates next CRD when ready
```

**Data Flow** (without ownership):
```
RemediationProcessing.status ──[data snapshot]──► RemediationRequest ──[creates]──► AIAnalysis.spec
AIAnalysis.status ──[data snapshot]──► RemediationRequest ──[creates]──► WorkflowExecution.spec
WorkflowExecution.status ──[data snapshot]──► RemediationRequest ──[creates]──► KubernetesExecution.spec
```

### **Why Centralized Orchestration?**

1. **Single Orchestrator**
   - RemediationRequest is the ONLY controller that creates service CRDs
   - RemediationRequest watches ALL service CRD statuses
   - RemediationRequest decides when to create the next CRD

2. **Service Controller Simplicity**
   - Each service controller only processes its own CRD
   - Service controllers update their own status
   - Service controllers have ZERO knowledge of other services

3. **No Cross-Service Coupling**
   - RemediationProcessing doesn't know about AIAnalysis
   - AIAnalysis doesn't know about WorkflowExecution
   - WorkflowExecution doesn't know about KubernetesExecution
   - Easy to add/remove/reorder services

4. **Flat Sibling Hierarchy**
   - All service CRDs are siblings (no nested ownership)
   - Maximum depth: 2 levels (root + siblings)
   - Simple cascade deletion: delete root → all siblings deleted

---

## ✅ **VERIFICATION: NO CIRCULAR DEPENDENCIES**

### **Circular Dependency Check**

**With Centralized Orchestration** (Flat Hierarchy):

```
✅ RemediationRequest → RemediationProcessing → (none)
✅ RemediationRequest → AIAnalysis → (none)
✅ RemediationRequest → WorkflowExecution → (none)
✅ RemediationRequest → KubernetesExecution → (none)
✅ RemediationRequest → AIApprovalRequest → (none)

❌ NO CIRCULAR REFERENCES DETECTED
✅ ALL PATHS TERMINATE AT LEVEL 2
```

**Ownership Path Validation**:
1. RemediationRequest → RemediationProcessing ✅ (terminates at level 2)
2. RemediationRequest → AIAnalysis ✅ (terminates at level 2)
3. RemediationRequest → WorkflowExecution ✅ (terminates at level 2)
4. RemediationRequest → KubernetesExecution ✅ (terminates at level 2)
5. RemediationRequest → AIApprovalRequest ✅ (terminates at level 2)

**Maximum Depth**: **2 levels** (RemediationRequest → Any Service CRD)

**Why This is Simple**:
- ✅ No nested ownership chains
- ✅ All service CRDs are at same level (siblings)
- ✅ Easy to reason about
- ✅ Impossible to create circular dependencies

---

## 🔧 **IMPLEMENTATION PATTERN**

### **Owner Reference Code Pattern**

**Standard Pattern** (Used by all services):
```go
import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Create child CRD with owner reference
childCRD := &ChildCRDType{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "child-name",
        Namespace: "kubernaut-system",
        OwnerReferences: []metav1.OwnerReference{
            *metav1.NewControllerRef(parentCRD, ParentGroupVersion.WithKind("ParentKind")),
        },
    },
    Spec: ChildCRDSpec{
        // ... spec fields ...
    },
}
```

### **Example 1: RemediationRequest Creates AIAnalysis**

**File**: `05-remediation-orchestrator.md` (RemediationRequest Controller)

```go
func (r *RemediationRequestReconciler) createAIAnalysis(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    processing *alertprocessorv1.RemediationProcessing,
) error {
    aiAnalysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-ai", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                // RemediationRequest OWNS AIAnalysis
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            // ... spec from processing.Status ...
        },
    }

    return r.Create(ctx, aiAnalysis)
}
```

### **Example 2: RemediationRequest Creates AIAnalysis** (After RemediationProcessing Completes)

**File**: `05-remediation-orchestrator.md` (RemediationRequest Controller)

```go
func (r *RemediationRequestReconciler) createAIAnalysis(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    alertProcessing *alertprocessorv1.RemediationProcessing,
) error {
    // RemediationRequest watches RemediationProcessing.status.phase
    // When "completed", create AIAnalysis with data snapshot

    aiAnalysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-ai", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                // RemediationRequest OWNS AIAnalysis (NOT RemediationProcessing)
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            // Data snapshot from RemediationProcessing.status
            AlertContext: alertProcessing.Status.EnrichedAlert,
            // ...
        },
    }

    return r.Create(ctx, aiAnalysis)
}
```

### **Example 3: RemediationRequest Creates WorkflowExecution** (After AIAnalysis Completes)

**File**: `05-remediation-orchestrator.md` (RemediationRequest Controller)

```go
func (r *RemediationRequestReconciler) createWorkflowExecution(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    aiAnalysis *aianalysisv1.AIAnalysis,
) error {
    // RemediationRequest watches AIAnalysis.status.phase
    // When "completed", create WorkflowExecution with recommendations

    workflowExecution := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-workflow", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                // RemediationRequest OWNS WorkflowExecution (NOT AIAnalysis)
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: workflowexecutionv1.WorkflowExecutionSpec{
            // Data snapshot from AIAnalysis.status
            WorkflowSteps: aiAnalysis.Status.Recommendations[0].Steps,
            // ...
        },
    }

    return r.Create(ctx, workflowExecution)
}
```

### **Example 4: RemediationRequest Creates KubernetesExecution** (After WorkflowExecution Completes)

**File**: `05-remediation-orchestrator.md` (RemediationRequest Controller)

```go
func (r *RemediationRequestReconciler) createKubernetesExecution(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    workflowExecution *workflowexecutionv1.WorkflowExecution,
) error {
    // RemediationRequest watches WorkflowExecution.status.phase
    // When "completed", create KubernetesExecution with operations

    kubernetesExecution := &kubernetesexecutionv1.KubernetesExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-execution", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                // RemediationRequest OWNS KubernetesExecution (NOT WorkflowExecution)
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: kubernetesexecutionv1.KubernetesExecutionSpec{
            // Data snapshot from WorkflowExecution.status
            Operations: workflowExecution.Status.Operations,
            // ...
        },
    }

    return r.Create(ctx, kubernetesExecution)
}
```

---

## 🗑️ **CASCADE DELETION BEHAVIOR**

### **Automatic Cleanup with Centralized Orchestration**

Kubernetes automatically deletes all child CRDs when the parent is deleted via owner references.

**Deletion Cascade** (Flat Hierarchy - Simple!):

```
DELETE RemediationRequest (root)
    │
    ├── ⚙️  Kubernetes deletes RemediationProcessing (sibling)
    ├── ⚙️  Kubernetes deletes AIAnalysis (sibling)
    ├── ⚙️  Kubernetes deletes WorkflowExecution (sibling)
    ├── ⚙️  Kubernetes deletes KubernetesExecution (sibling)
    └── ⚙️  Kubernetes deletes AIApprovalRequest (sibling, optional)

✅ All resources cleaned up in parallel (all at same level)
```

**Deletion Order** (Kubernetes handles automatically):
1. Delete RemediationRequest (user action)
2. Kubernetes deletes ALL service CRDs **in parallel** (all are siblings)
3. ✅ Complete cleanup in one pass

**Why This is Better**:
- ✅ **Parallel Deletion**: All siblings deleted simultaneously (faster)
- ✅ **No Dependency Chain**: No waiting for nested deletions
- ✅ **Simple Finalizers**: Each CRD handles its own cleanup independently
- ✅ **Predictable**: Always 2-level deletion (root → siblings)

### **Finalizer Integration**

Each controller can use finalizers to perform cleanup before deletion:

```go
const myServiceFinalizer = "myservice.kubernaut.io/finalizer"

func (r *MyServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &MyServiceCRD{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Check if being deleted
    if !obj.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(obj, myServiceFinalizer) {
            // Perform cleanup (e.g., delete external resources)
            if err := r.cleanup(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }

            // Remove finalizer
            controllerutil.RemoveFinalizer(obj, myServiceFinalizer)
            if err := r.Update(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(obj, myServiceFinalizer) {
        controllerutil.AddFinalizer(obj, myServiceFinalizer)
        if err := r.Update(ctx, obj); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Normal reconciliation logic
    // ...
}
```

---

## 📋 **PER-SERVICE OWNER REFERENCE DOCUMENTATION**

### **1. Remediation Processor (RemediationProcessing)**

**Owner**: RemediationRequest
**Owns**: None
**Purpose**: Data gathering phase

```yaml
apiVersion: alertprocessor.kubernaut.io/v1
kind: RemediationProcessing
metadata:
  name: my-alert-processing
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: alertremediation.kubernaut.io/v1
    kind: RemediationRequest
    name: my-remediation
    uid: "..."
    controller: true
    blockOwnerDeletion: true
```

---

### **2. AI Analysis (AIAnalysis)**

**Owner**: RemediationRequest
**Creates**: Nothing (RemediationRequest creates next CRDs)
**Purpose**: Root cause analysis and remediation recommendation

```yaml
apiVersion: aianalysis.kubernaut.io/v1
kind: AIAnalysis
metadata:
  name: my-ai-analysis
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: alertremediation.kubernaut.io/v1
    kind: RemediationRequest
    name: my-remediation
    uid: "..."
    controller: true
    blockOwnerDeletion: true
```

**Responsibilities**:
- Process AI analysis using HolmesGPT
- Generate remediation recommendations
- Update status.phase to "completed"
- **RemediationRequest watches status** and creates WorkflowExecution when ready

---

### **3. Workflow Execution (WorkflowExecution)**

**Owner**: RemediationRequest
**Creates**: Nothing (RemediationRequest creates next CRDs)
**Purpose**: Orchestrate remediation workflow steps

```yaml
apiVersion: workflowexecution.kubernaut.io/v1
kind: WorkflowExecution
metadata:
  name: my-workflow-execution
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: alertremediation.kubernaut.io/v1
    kind: RemediationRequest
    name: my-remediation
    uid: "..."
    controller: true
    blockOwnerDeletion: true
```

**Responsibilities**:
- Execute workflow steps (orchestration logic)
- Track step completion and overall workflow progress
- Update status.phase to "completed"
- **RemediationRequest watches status** and creates KubernetesExecution when ready

---

### **4. Kubernetes Executor (KubernetesExecution)**

**Owner**: RemediationRequest
**Creates**: Nothing (leaf node)
**Purpose**: Execute Kubernetes operations via Jobs

```yaml
apiVersion: kubernetesexecution.kubernaut.io/v1
kind: KubernetesExecution
metadata:
  name: my-k8s-execution
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: alertremediation.kubernaut.io/v1
    kind: RemediationRequest
    name: my-remediation
    uid: "..."
    controller: true
    blockOwnerDeletion: true
```

**Responsibilities**:
- Execute Kubernetes operations using native Jobs
- Track execution progress and results
- Update status.phase to "completed"
- **RemediationRequest watches status** and marks overall remediation as complete

---

### **5. Remediation Orchestrator (RemediationRequest)**

**Owner**: None (root CRD)
**Owns**: ALL service CRDs (RemediationProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution, AIApprovalRequest)
**Purpose**: Central orchestration and lifecycle management

```yaml
apiVersion: alertremediation.kubernaut.io/v1
kind: RemediationRequest
metadata:
  name: my-remediation
  namespace: kubernaut-system
  # ⚠️  NO ownerReferences - this is the ROOT CRD
```

**Creates** (Sequential based on status watching):
1. RemediationProcessing (when RemediationRequest created)
2. AIAnalysis (when RemediationProcessing completes)
3. WorkflowExecution (when AIAnalysis completes)
4. KubernetesExecution (when WorkflowExecution completes)
5. AIApprovalRequest (when manual approval needed)

**Watches**: All service CRD statuses for phase transitions

---

## ✅ **VALIDATION CHECKLIST**

### **Ownership Validation** (Centralized Orchestration)

- [x] RemediationRequest has NO owner reference (root CRD) ✅
- [x] RemediationProcessing owned by RemediationRequest ✅
- [x] AIAnalysis owned by RemediationRequest ✅
- [x] WorkflowExecution owned by RemediationRequest ✅
- [x] KubernetesExecution owned by RemediationRequest ✅
- [x] AIApprovalRequest owned by RemediationRequest (optional) ✅
- [x] NO circular dependencies detected ✅
- [x] Maximum ownership depth: **2 levels** (flat hierarchy) ✅
- [x] All child CRDs have exactly ONE owner reference ✅
- [x] All service CRDs are siblings at level 2 ✅
- [x] RemediationRequest creates ALL service CRDs ✅
- [x] Service controllers do NOT create other service CRDs ✅

### **Cascade Deletion Validation** (Centralized Orchestration)

- [x] Deleting RemediationRequest cascades to ALL service CRDs (parallel deletion) ✅
- [x] All service CRDs deleted simultaneously (flat hierarchy benefit) ✅
- [x] Finalizers allow each service to cleanup independently ✅
- [x] No nested deletion chains (all at level 2) ✅

---

## 🎯 **BENEFITS OF CENTRALIZED ORCHESTRATION ARCHITECTURE**

### **1. Automatic Resource Cleanup** (Parallel Deletion)
- ✅ No orphaned CRDs after RemediationRequest deletion
- ✅ Kubernetes handles cascade deletion automatically
- ✅ **All service CRDs deleted in parallel** (flat hierarchy benefit)
- ✅ No manual cleanup required

### **2. Simplified Ownership Hierarchy** (2 Levels Only)
- ✅ Single root CRD (RemediationRequest) controls everything
- ✅ Flat sibling relationships (all service CRDs at level 2)
- ✅ No circular dependencies (impossible with 2-level design)
- ✅ Easy to understand and reason about

### **3. Decoupled Service Controllers** (No Cross-Service Knowledge)
- ✅ Service controllers only process their own CRD
- ✅ Service controllers have ZERO knowledge of other services
- ✅ Easy to add/remove/reorder services in workflow
- ✅ Service controllers are simple (no orchestration logic)

### **4. Centralized Orchestration Logic** (Single Point of Control)
- ✅ RemediationRequest controller is the ONLY orchestrator
- ✅ All workflow transitions defined in one place
- ✅ Easy to modify workflow order or add new services
- ✅ Clear separation: orchestration vs. processing

### **5. Resource Discovery** (Simple Ownership Tree)
- ✅ Easy to find all related CRDs via owner references
- ✅ kubectl shows flat ownership tree (2 levels)
- ✅ Kubernetes Dashboard visualizes flat relationships
- ✅ All service CRDs share same parent

---

## 📚 **KUBERNETES DOCUMENTATION REFERENCES**

- **Owner References**: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
- **Cascade Deletion**: https://kubernetes.io/docs/concepts/architecture/garbage-collection/#cascading-deletion
- **Finalizers**: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/

---

## ✅ **COMPLETION STATUS**

**Owner Reference Architecture**: ✅ **CORRECTED & VERIFIED**

- [x] Ownership hierarchy corrected (centralized orchestration pattern) ✅
- [x] Visual diagrams updated (flat 2-level hierarchy) ✅
- [x] No circular dependencies verified (impossible with 2-level design) ✅
- [x] Code patterns updated (RemediationRequest creates all service CRDs) ✅
- [x] Cascade deletion behavior explained (parallel deletion) ✅
- [x] Per-service documentation provided (all owned by RemediationRequest) ✅
- [x] Benefits documented (decoupled services, centralized orchestration) ✅

**Architecture Pattern**: **Centralized Orchestration with Flat Sibling Hierarchy**

**Key Characteristics**:
- ✅ **2-Level Hierarchy**: Root + Siblings only
- ✅ **Single Orchestrator**: RemediationRequest creates ALL service CRDs
- ✅ **Decoupled Services**: Service controllers have no cross-service knowledge
- ✅ **Parallel Deletion**: All siblings deleted simultaneously
- ✅ **Simple & Maintainable**: Easy to add/remove services

**Confidence**: **100%** - Architecture aligns with `05-remediation-orchestrator.md` design and follows Kubernetes best practices for centralized orchestration

---

**Last Updated**: 2025-10-03 (Corrected to match centralized orchestration pattern)
**Maintained By**: Kubernaut Architecture Team
**Design Reference**: `05-remediation-orchestrator.md`

