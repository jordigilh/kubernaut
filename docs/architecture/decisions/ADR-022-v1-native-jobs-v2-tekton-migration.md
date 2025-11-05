# ADR-022: V1 Native Jobs with V2 Tekton Migration Path

> ⚠️ **SUPERSEDED** ⚠️
>
> This ADR has been **superseded** by [ADR-023: Tekton from V1](ADR-023-tekton-from-v1.md).
>
> **Reason**: Eliminates 500+ lines of throwaway custom orchestration code by using Tekton Pipelines from V1. The V1/V2 split was determined to be unnecessary architectural waste given Tekton's universal availability (Tekton Pipelines + upstream).
>
> **Date Superseded**: 2025-10-19

**Status**: ❌ Superseded by [ADR-023](ADR-023-tekton-from-v1.md)
**Date**: 2025-10-19
**Deciders**: Architecture Team
**Priority**: FOUNDATIONAL

---

## Context and Problem Statement

Kubernaut needs a **clear execution strategy** for V1 and V2:
- **V1**: Production-ready execution with minimal dependencies
- **V2**: Industry-standard workflow orchestration for enterprise adoption

**Key Requirements**:
1. V1 must support **complex GitOps workflows** (clone → modify → commit → push)
2. V1 → V2 migration must be **seamless** (no action container rewrites)
3. V2 adoption driven by **industrial acceptance** and **reduced learning curve**
4. Execution time is **NOT a concern** (reliability > speed)

**Critical Question**: How do we build V1 to enable smooth V2 migration?

---

## Decision Drivers

### **Strategic Priorities**
1. **V1 Production Readiness**: Ship Q4 2025 with native Kubernetes primitives
2. **Enterprise Adoption**: Tekton familiarity reduces deployment friction
3. **Container Portability**: Same Cosign-signed images work in V1 and V2
4. **Zero Downtime Migration**: Gradual rollout from Jobs → Tekton

### **Technical Constraints**
- **V1**: No external dependencies beyond Kubernetes core
- **V2**: Tekton Pipelines as workflow orchestrator
- **Both**: Cosign-signed action containers with embedded contracts

---

## Considered Options

### **Option 1: V1 Custom Orchestrator, V2 Rewrite for Tekton** ❌
Build custom workflow engine in V1, rewrite for Tekton in V2.

**Pros**:
- ✅ V1 optimized for custom needs
- ✅ V2 fully leverages Tekton features

**Cons**:
- ❌ Different orchestration patterns between V1 and V2
- ❌ Action containers may need refactoring
- ❌ Risky migration (big-bang cutover)

**Why Rejected**: Too risky, no incremental migration path

---

### **Option 2: V1 Native Jobs with Container Portability** ⭐ **CHOSEN**
Build V1 with native Jobs using **portable container contracts**, enabling seamless V2 Tekton migration.

**Pros**:
- ✅ V1 ships with zero dependencies
- ✅ Same Cosign-signed containers work in V1 and V2
- ✅ Incremental migration path (workflow by workflow)
- ✅ Enterprise teams can choose V1 or V2 based on maturity

**Cons**:
- ⚠️ V1 requires custom Job chaining for multi-step workflows
- ⚠️ V1 orchestration logic more complex than Tekton

**Why Chosen**: **Portability is the key to safe migration**

---

### **Option 3: V1 with Tekton from Day 1** ❌
Ship V1 with Tekton Pipelines as dependency.

**Pros**:
- ✅ No migration needed
- ✅ Leverage Tekton from start

**Cons**:
- ❌ External dependency for V1 (violates Q4 2025 goals)
- ❌ Operational complexity before product validation
- ❌ Forces early adopters to learn Tekton

**Why Rejected**: Adds risk to V1 production readiness

---

## Decision Outcome

**Chosen option**: **"Option 2: V1 Native Jobs with Container Portability"**

**Rationale**:
1. **V1 Focus**: Ship production-ready with native Kubernetes primitives
2. **V2 Value**: Leverage Tekton's industrial acceptance and ecosystem
3. **Migration Safety**: Container contracts enable zero-downtime rollout
4. **Customer Choice**: Customers can stay on V1 or adopt V2 based on needs

---

## V1 Architecture: Native Jobs with Sequential Chaining

### **Single-Action Workflow (90% of V1 use cases)**

```
┌───────────────────────────────────────┐
│ WorkflowExecution Controller          │
│ - Creates single ActionExecution CRD  │
└───────────────────────────────────────┘
             ↓
┌───────────────────────────────────────┐
│ ActionExecution Controller             │
│ - Verifies Cosign signature            │
│ - Creates Kubernetes Job               │
└───────────────────────────────────────┘
             ↓
┌───────────────────────────────────────┐
│ Kubernetes Job                         │
│ Image: ghcr.io/kubernaut/actions/     │
│        scale@sha256:abc123             │
│ Contract: /action-contract.yaml        │
└───────────────────────────────────────┘
```

**Example**: Scale deployment
```yaml
apiVersion: execution.kubernaut.ai/v1alpha1
kind: ActionExecution
metadata:
  name: scale-payment-deployment
spec:
  actionType: kubernetes/scale_deployment
  image: ghcr.io/kubernaut/actions/scale@sha256:abc123
  imageVerification:
    policy: strict
  inputs:
    deployment: payment-service
    namespace: production
    replicas: 10
```

---

### **Multi-Step Workflow with Dependencies (10% of V1 use cases)**

**Example**: GitOps PR workflow (4 steps)

```
┌───────────────────────────────────────┐
│ WorkflowExecution Controller          │
│ - Parses workflow steps                │
│ - Builds dependency graph (DAG)        │
│ - Creates ActionExecution CRDs         │
│   sequentially based on dependencies   │
└───────────────────────────────────────┘
             ↓
┌───────────────────────────────────────┐
│ Step 1: git-clone                      │
│ Job creates PVC "git-workspace"        │
│ Clones repo to /workspace              │
└───────────────────────────────────────┘
             ↓ (runAfter: git-clone)
┌───────────────────────────────────────┐
│ Step 2: modify-files                   │
│ Job mounts PVC "git-workspace"         │
│ Modifies deployment YAML               │
└───────────────────────────────────────┘
             ↓ (runAfter: modify-files)
┌───────────────────────────────────────┐
│ Step 3: git-commit                     │
│ Job mounts PVC "git-workspace"         │
│ Commits changes with evidence          │
└───────────────────────────────────────┘
             ↓ (runAfter: git-commit)
┌───────────────────────────────────────┐
│ Step 4: git-push                       │
│ Job mounts PVC "git-workspace"         │
│ Creates GitHub PR                      │
└───────────────────────────────────────┘
             ↓ (cleanup)
┌───────────────────────────────────────┐
│ WorkflowExecution Controller           │
│ Deletes PVC "git-workspace"            │
└───────────────────────────────────────┘
```

**Key Pattern**: **PVC-based workspace sharing** for multi-step workflows

---

### **V1 Implementation: Sequential Job Creation with Shared PVC**

```go
package controller

import (
    "context"
    "fmt"

    executionv1 "github.com/jordigilh/kubernaut/api/execution/v1alpha1"
    workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkflowExecutionReconciler reconciles a WorkflowExecution object
type WorkflowExecutionReconciler struct {
    client.Client
    MaxParallelSteps int
    ComplexityThreshold int
}

// Reconcile handles WorkflowExecution lifecycle
func (r *WorkflowExecutionReconciler) Reconcile(
    ctx context.Context,
    req ctrl.Request,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Fetch WorkflowExecution
    workflow := &workflowv1.WorkflowExecution{}
    if err := r.Get(ctx, req.NamespacedName, workflow); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Phase-based reconciliation
    switch workflow.Status.Phase {
    case "":
        return r.handleInitialization(ctx, workflow)
    case "Initializing":
        return r.handleStepExecution(ctx, workflow)
    case "Executing":
        return r.handleStepMonitoring(ctx, workflow)
    case "Completed", "Failed":
        return r.handleCleanup(ctx, workflow)
    default:
        return ctrl.Result{}, nil
    }
}

// handleInitialization validates workflow and creates workspace PVC if needed
func (r *WorkflowExecutionReconciler) handleInitialization(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Check if workflow needs shared workspace
    needsWorkspace := r.needsSharedWorkspace(workflow.Spec.Steps)

    if needsWorkspace {
        // Create PVC for workspace
        pvc := r.createWorkspacePVC(workflow)
        if err := r.Create(ctx, pvc); err != nil {
            log.Error(err, "Failed to create workspace PVC")
            return ctrl.Result{}, err
        }

        log.Info("Created workspace PVC", "pvc", pvc.Name)

        // Store workspace name in status
        workflow.Status.WorkspacePVC = pvc.Name
    }

    // Transition to Initializing
    workflow.Status.Phase = "Initializing"
    workflow.Status.Message = "Workflow initialized, ready to execute steps"

    if err := r.Status().Update(ctx, workflow); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{Requeue: true}, nil
}

// handleStepExecution creates ActionExecution CRDs sequentially
func (r *WorkflowExecutionReconciler) handleStepExecution(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Find next step(s) to execute
    nextSteps := r.findReadySteps(ctx, workflow)

    if len(nextSteps) == 0 {
        // All steps created, transition to monitoring
        workflow.Status.Phase = "Executing"
        workflow.Status.Message = "All steps created, monitoring execution"

        if err := r.Status().Update(ctx, workflow); err != nil {
            return ctrl.Result{}, err
        }

        return ctrl.Result{Requeue: true}, nil
    }

    // Create ActionExecution CRDs for ready steps
    for _, step := range nextSteps {
        actionExec := r.createActionExecution(workflow, step)

        if err := r.Create(ctx, actionExec); err != nil {
            log.Error(err, "Failed to create ActionExecution", "step", step.Name)
            return ctrl.Result{}, err
        }

        log.Info("Created ActionExecution", "step", step.Name, "action", actionExec.Name)
    }

    // Requeue to check for more steps
    return ctrl.Result{Requeue: true}, nil
}

// findReadySteps returns steps whose dependencies are completed
func (r *WorkflowExecutionReconciler) findReadySteps(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) []workflowv1.WorkflowStep {
    log := ctrl.LoggerFrom(ctx)
    var readySteps []workflowv1.WorkflowStep

    for _, step := range workflow.Spec.Steps {
        // Skip if already created
        if r.isStepCreated(workflow, step.Name) {
            continue
        }

        // Check if all dependencies are completed
        dependenciesMet := true
        for _, depName := range step.RunAfter {
            if !r.isStepCompleted(ctx, workflow, depName) {
                dependenciesMet = false
                break
            }
        }

        if dependenciesMet {
            readySteps = append(readySteps, step)

            // Limit parallel execution
            if len(readySteps) >= r.MaxParallelSteps {
                break
            }
        }
    }

    return readySteps
}

// createActionExecution creates ActionExecution CRD for a workflow step
func (r *WorkflowExecutionReconciler) createActionExecution(
    workflow *workflowv1.WorkflowExecution,
    step workflowv1.WorkflowStep,
) *executionv1.ActionExecution {
    actionExec := &executionv1.ActionExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-%s", workflow.Name, step.Name),
            Namespace: workflow.Namespace,
            Labels: map[string]string{
                "kubernaut.io/workflow": workflow.Name,
                "kubernaut.io/step":     step.Name,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(workflow, workflowv1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: executionv1.ActionExecutionSpec{
            ActionType: step.ActionType,
            Image:      step.Image,
            ImageVerification: executionv1.ImageVerification{
                Policy: "strict", // Always verify Cosign signatures
            },
            Inputs: step.Inputs,
        },
    }

    // Add workspace volume if needed
    if workflow.Status.WorkspacePVC != "" && step.UsesWorkspace {
        actionExec.Spec.Volumes = []executionv1.VolumeMount{
            {
                Name:      "workspace",
                PVCName:   workflow.Status.WorkspacePVC,
                MountPath: "/workspace",
            },
        }
    }

    return actionExec
}

// needsSharedWorkspace checks if any step requires workspace
func (r *WorkflowExecutionReconciler) needsSharedWorkspace(
    steps []workflowv1.WorkflowStep,
) bool {
    for _, step := range steps {
        if step.UsesWorkspace {
            return true
        }
    }
    return false
}

// createWorkspacePVC creates PVC for shared workspace
func (r *WorkflowExecutionReconciler) createWorkspacePVC(
    workflow *workflowv1.WorkflowExecution,
) *corev1.PersistentVolumeClaim {
    pvcName := fmt.Sprintf("%s-workspace", workflow.Name)

    return &corev1.PersistentVolumeClaim{
        ObjectMeta: metav1.ObjectMeta{
            Name:      pvcName,
            Namespace: workflow.Namespace,
            Labels: map[string]string{
                "kubernaut.io/workflow": workflow.Name,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(workflow, workflowv1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: corev1.PersistentVolumeClaimSpec{
            AccessModes: []corev1.PersistentVolumeAccessMode{
                corev1.ReadWriteOnce,
            },
            Resources: corev1.ResourceRequirements{
                Requests: corev1.ResourceList{
                    corev1.ResourceStorage: resource.MustParse("1Gi"),
                },
            },
        },
    }
}

// isStepCreated checks if ActionExecution CRD exists for step
func (r *WorkflowExecutionReconciler) isStepCreated(
    workflow *workflowv1.WorkflowExecution,
    stepName string,
) bool {
    for _, stepStatus := range workflow.Status.Steps {
        if stepStatus.Name == stepName {
            return true
        }
    }
    return false
}

// isStepCompleted checks if step has completed successfully
func (r *WorkflowExecutionReconciler) isStepCompleted(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
    stepName string,
) bool {
    log := ctrl.LoggerFrom(ctx)

    // Find ActionExecution CRD
    actionExecName := fmt.Sprintf("%s-%s", workflow.Name, stepName)
    actionExec := &executionv1.ActionExecution{}

    err := r.Get(ctx, types.NamespacedName{
        Name:      actionExecName,
        Namespace: workflow.Namespace,
    }, actionExec)

    if err != nil {
        log.Error(err, "Failed to get ActionExecution", "step", stepName)
        return false
    }

    return actionExec.Status.Phase == "Completed"
}

// handleCleanup deletes workspace PVC after workflow completes
func (r *WorkflowExecutionReconciler) handleCleanup(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Delete workspace PVC if exists
    if workflow.Status.WorkspacePVC != "" {
        pvc := &corev1.PersistentVolumeClaim{
            ObjectMeta: metav1.ObjectMeta{
                Name:      workflow.Status.WorkspacePVC,
                Namespace: workflow.Namespace,
            },
        }

        if err := r.Delete(ctx, pvc); err != nil {
            log.Error(err, "Failed to delete workspace PVC")
            // Don't return error, just log (cleanup is best-effort)
        } else {
            log.Info("Deleted workspace PVC", "pvc", workflow.Status.WorkspacePVC)
        }
    }

    return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *WorkflowExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&workflowv1.WorkflowExecution{}).
        Owns(&executionv1.ActionExecution{}).
        Complete(r)
}
```

---

### **V1 GitOps Workflow Example: 4-Step PR Creation**

```yaml
apiVersion: workflowexecution.kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: create-gitops-pr-oom-fix
  namespace: kubernaut-system
spec:
  workflowType: gitops-pr-creation

  steps:
    # Step 1: Clone repository
    - name: git-clone
      actionType: git/clone
      image: ghcr.io/kubernaut/actions/git-clone@sha256:def456
      usesWorkspace: true
      inputs:
        repository: https://github.com/company/k8s-configs
        branch: main
        destination: /workspace
      runAfter: []  # No dependencies

    # Step 2: Modify deployment YAML
    - name: modify-deployment
      actionType: git/modify-file
      image: ghcr.io/kubernaut/actions/yaml-patch@sha256:ghi789
      usesWorkspace: true
      inputs:
        file: /workspace/production/payment-service.yaml
        patch: |
          spec:
            template:
              spec:
                containers:
                - name: payment
                  resources:
                    limits:
                      memory: 2Gi  # Increased from 1Gi
      runAfter:
        - git-clone  # Depends on clone

    # Step 3: Commit changes
    - name: git-commit
      actionType: git/commit
      image: ghcr.io/kubernaut/actions/git-commit@sha256:jkl012
      usesWorkspace: true
      inputs:
        message: |
          fix(payment-service): Increase memory limit to 2Gi

          Root Cause: OOMKilled events detected (AI Analysis ID: 12345)
          Evidence: Payment pods restarting 8x/hour during peak traffic
          Solution: Double memory limit based on 95th percentile usage

          Kubernaut AI Recommendation (Confidence: 87%)
        author: kubernaut-bot <kubernaut@company.com>
      runAfter:
        - modify-deployment

    # Step 4: Push and create PR
    - name: git-push-pr
      actionType: git/create-pr
      image: ghcr.io/kubernaut/actions/git-push@sha256:mno345
      usesWorkspace: true
      inputs:
        branch: kubernaut/payment-memory-fix
        title: "Kubernaut: Fix payment-service OOMKilled (AI-recommended)"
        body: |
          ## Automated Remediation

          **Trigger**: OOMKilled alert (payment-service, production)
          **Analysis**: HolmesGPT investigation ID 12345
          **Confidence**: 87%

          ## Changes
          - Increase payment-service memory limit: 1Gi → 2Gi

          ## Evidence
          - 8 OOMKilled events in last hour
          - 95th percentile memory usage: 1.6Gi
          - Recommendation: 2Gi limit (safety margin)

          **Review requested**: @platform-team
      runAfter:
        - git-commit

status:
  phase: Completed
  workspacePVC: create-gitops-pr-oom-fix-workspace
  steps:
    - name: git-clone
      phase: Completed
      startedAt: "2025-10-19T10:00:00Z"
      completedAt: "2025-10-19T10:00:15Z"
    - name: modify-deployment
      phase: Completed
      startedAt: "2025-10-19T10:00:16Z"
      completedAt: "2025-10-19T10:00:20Z"
    - name: git-commit
      phase: Completed
      startedAt: "2025-10-19T10:00:21Z"
      completedAt: "2025-10-19T10:00:23Z"
    - name: git-push-pr
      phase: Completed
      startedAt: "2025-10-19T10:00:24Z"
      completedAt: "2025-10-19T10:00:30Z"
```

**Key V1 Features**:
- ✅ **Sequential execution** with `runAfter` dependencies
- ✅ **Shared workspace** via PVC (`usesWorkspace: true`)
- ✅ **Cosign-signed images** with `@sha256` digests
- ✅ **Portable containers** (same images work in V2 Tekton)

---

## V2 Architecture: Tekton Pipelines with Same Containers

### **V2 Workflow Translation: WorkflowExecution → PipelineRun**

```go
package controller

import (
    "context"
    "fmt"

    workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// TektonWorkflowReconciler translates WorkflowExecution to Tekton PipelineRun
type TektonWorkflowReconciler struct {
    client.Client
}

// Reconcile converts WorkflowExecution to PipelineRun
func (r *TektonWorkflowReconciler) Reconcile(
    ctx context.Context,
    req ctrl.Request,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Fetch WorkflowExecution
    workflow := &workflowv1.WorkflowExecution{}
    if err := r.Get(ctx, req.NamespacedName, workflow); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Create PipelineRun from workflow
    pipelineRun := r.createPipelineRun(workflow)

    if err := r.Create(ctx, pipelineRun); err != nil {
        log.Error(err, "Failed to create PipelineRun")
        return ctrl.Result{}, err
    }

    log.Info("Created PipelineRun", "pipelineRun", pipelineRun.Name)

    return ctrl.Result{}, nil
}

// createPipelineRun translates WorkflowExecution to Tekton PipelineRun
func (r *TektonWorkflowReconciler) createPipelineRun(
    workflow *workflowv1.WorkflowExecution,
) *tektonv1.PipelineRun {
    // Build Tekton TaskRun specs from WorkflowExecution steps
    tasks := make([]tektonv1.PipelineTask, len(workflow.Spec.Steps))

    for i, step := range workflow.Spec.Steps {
        tasks[i] = tektonv1.PipelineTask{
            Name: step.Name,
            TaskRef: &tektonv1.TaskRef{
                Name: "kubernaut-action",  // Generic meta-task
            },
            Params: []tektonv1.Param{
                {
                    Name: "actionType",
                    Value: tektonv1.ParamValue{
                        Type:      tektonv1.ParamTypeString,
                        StringVal: step.ActionType,
                    },
                },
                {
                    Name: "actionImage",
                    Value: tektonv1.ParamValue{
                        Type:      tektonv1.ParamTypeString,
                        StringVal: step.Image,  // Same Cosign-signed image!
                    },
                },
                {
                    Name: "inputs",
                    Value: tektonv1.ParamValue{
                        Type:      tektonv1.ParamTypeString,
                        StringVal: marshalJSON(step.Inputs),
                    },
                },
            },
            RunAfter: step.RunAfter,  // Same dependency structure!
        }

        // Add workspace binding if needed
        if step.UsesWorkspace {
            tasks[i].Workspaces = []tektonv1.WorkspacePipelineTaskBinding{
                {
                    Name:      "workspace",
                    Workspace: "shared-workspace",
                },
            }
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
                Workspaces: []tektonv1.PipelineWorkspaceDeclaration{
                    {
                        Name: "shared-workspace",
                        Description: "Shared workspace for multi-step workflows",
                    },
                },
            },
            Workspaces: []tektonv1.WorkspaceBinding{
                {
                    Name: "shared-workspace",
                    VolumeClaimTemplate: &corev1.PersistentVolumeClaim{
                        Spec: corev1.PersistentVolumeClaimSpec{
                            AccessModes: []corev1.PersistentVolumeAccessMode{
                                corev1.ReadWriteOnce,
                            },
                            Resources: corev1.ResourceRequirements{
                                Requests: corev1.ResourceList{
                                    corev1.ResourceStorage: resource.MustParse("1Gi"),
                                },
                            },
                        },
                    },
                },
            },
        },
    }
}
```

---

### **V2 Generic Tekton Task: Executes ANY Action Container**

```yaml
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: kubernaut-action
  namespace: kubernaut-system
spec:
  description: |
    Generic Tekton Task that executes any Kubernaut action container.
    Same containers work in V1 (native Jobs) and V2 (Tekton)!

  params:
    - name: actionType
      type: string
      description: "Action type (e.g., git/clone, kubernetes/scale)"

    - name: actionImage
      type: string
      description: "Cosign-signed action container image with @sha256 digest"

    - name: inputs
      type: string
      description: "JSON-encoded action inputs"

  workspaces:
    - name: workspace
      description: "Shared workspace for multi-step workflows"
      optional: true

  steps:
    # Step 1: Verify Cosign signature (via admission controller)
    # (Sigstore Policy Controller validates at admission time)

    # Step 2: Execute action container
    - name: execute
      image: $(params.actionImage)  # Same image as V1!
      env:
        - name: ACTION_TYPE
          value: $(params.actionType)
        - name: ACTION_INPUTS
          value: $(params.inputs)
        - name: WORKSPACE_PATH
          value: $(workspaces.workspace.path)
      volumeMounts:
        - name: $(workspaces.workspace.volume)
          mountPath: $(workspaces.workspace.path)
      script: |
        #!/bin/sh
        set -e

        # Action containers read inputs from env vars or stdin
        echo "$ACTION_INPUTS" | /action-entrypoint

        # Outputs written to stdout (captured by Tekton)

  results:
    - name: outputs
      description: "JSON outputs from action container"
```

**Key Point**: **Same action containers** (`ghcr.io/kubernaut/actions/*`) work in both V1 and V2! 🎯

---

## Container Contract: Portability Layer

### **Action Container Structure (V1 and V2 Compatible)**

```dockerfile
# Example: git/clone action container
FROM alpine/git:latest

# Install action contract
COPY action-contract.yaml /action-contract.yaml

# Install entrypoint
COPY entrypoint.sh /action-entrypoint
RUN chmod +x /action-entrypoint

# Entrypoint reads from env vars (V1 Job) or stdin (V2 Tekton)
ENTRYPOINT ["/action-entrypoint"]
```

### **Action Contract (Embedded in Container)**

```yaml
# /action-contract.yaml
apiVersion: kubernaut.ai/v1alpha1
kind: ActionContract
metadata:
  name: git-clone
  version: v1.0.0

spec:
  description: "Clone Git repository to workspace"

  inputs:
    - name: repository
      type: string
      required: true
      description: "Git repository URL"

    - name: branch
      type: string
      required: false
      default: "main"

    - name: destination
      type: string
      required: true
      description: "Destination path (e.g., /workspace)"

  outputs:
    - name: commitSHA
      type: string
      description: "Cloned commit SHA"

    - name: cloneTime
      type: duration
      description: "Time taken to clone"

  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"

  rbac:
    # No Kubernetes permissions needed (external Git operation)
    permissions: []

  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    allowPrivilegeEscalation: false
    capabilities:
      drop:
        - ALL
```

### **Entrypoint Script (V1 and V2 Compatible)**

```bash
#!/bin/sh
# /action-entrypoint

set -e

# Read inputs (supports both env vars and stdin)
if [ -n "$ACTION_INPUTS" ]; then
    # V1: Inputs from environment variable (Kubernetes Job)
    INPUTS="$ACTION_INPUTS"
elif [ ! -t 0 ]; then
    # V2: Inputs from stdin (Tekton Task)
    INPUTS=$(cat)
else
    echo "ERROR: No inputs provided" >&2
    exit 1
fi

# Parse JSON inputs
REPOSITORY=$(echo "$INPUTS" | jq -r '.repository')
BRANCH=$(echo "$INPUTS" | jq -r '.branch // "main"')
DESTINATION=$(echo "$INPUTS" | jq -r '.destination')

# Execute action logic
START_TIME=$(date +%s)

git clone --branch "$BRANCH" "$REPOSITORY" "$DESTINATION"

END_TIME=$(date +%s)
CLONE_TIME=$((END_TIME - START_TIME))

# Get commit SHA
cd "$DESTINATION"
COMMIT_SHA=$(git rev-parse HEAD)

# Output results (JSON to stdout)
cat <<EOF
{
  "commitSHA": "$COMMIT_SHA",
  "cloneTime": "${CLONE_TIME}s"
}
EOF

exit 0
```

**Key Features**:
- ✅ **Dual input mode**: Env vars (V1) or stdin (V2)
- ✅ **JSON outputs**: Captured by both Job logs and Tekton results
- ✅ **Same Cosign signature**: Verified in V1 admission controller and V2 admission controller
- ✅ **No code changes**: Container runs identically in V1 and V2

---

## Migration Path: V1 → V2

### **Phase 1: V1 Production (Q4 2025)**
- ✅ Deploy with native Kubernetes Jobs
- ✅ Build action container registry with Cosign signing
- ✅ Validate workflows in production

### **Phase 2: Tekton Preparation (Q1 2026)**
- ✅ Install Tekton Pipelines in test clusters
- ✅ Validate action containers work with Tekton meta-task
- ✅ Test dual WorkflowExecution reconcilers (Jobs + Tekton)

### **Phase 3: Gradual Rollout (Q2 2026)**
```go
// Feature flag: Enable Tekton per workflow type
func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    workflow := &workflowv1.WorkflowExecution{}
    r.Get(ctx, req.NamespacedName, workflow)

    // Check feature flag
    useTekton := r.shouldUseTekton(workflow)

    if useTekton {
        return r.TektonReconciler.Reconcile(ctx, req)
    } else {
        return r.NativeJobReconciler.Reconcile(ctx, req)
    }
}

func (r *WorkflowExecutionReconciler) shouldUseTekton(workflow *workflowv1.WorkflowExecution) bool {
    // Option 1: Annotation-based opt-in
    if workflow.Annotations["kubernaut.io/executor"] == "tekton" {
        return true
    }

    // Option 2: Workflow type-based routing
    if workflow.Spec.WorkflowType == "gitops-pr-creation" {
        return true  // GitOps workflows use Tekton
    }

    // Option 3: Cluster-wide config
    if r.Config.TektonEnabled {
        return true
    }

    // Default: Use native Jobs
    return false
}
```

**Migration Strategy**:
1. **Week 1-2**: Enable Tekton for GitOps workflows only (2% of traffic)
2. **Week 3-4**: Enable Tekton for multi-step workflows (8% of traffic)
3. **Week 5-6**: Monitor metrics, compare V1 vs V2 reliability
4. **Week 7-8**: Enable Tekton for all workflows (100%)

### **Phase 4: V2 Stabilization (Q3 2026)**
- ✅ Remove native Job reconciler (keep for rollback)
- ✅ Document Tekton best practices
- ✅ Train SRE teams on Tekton debugging

---

## Success Metrics

### **V1 Success Metrics (Q4 2025)**
- ✅ 93% average workflow success rate
- ✅ 5 min average MTTR (2-8 min by scenario)
- ✅ <5s overhead per action execution
- ✅ 100% Cosign signature verification

### **V2 Success Metrics (Q3 2026)**
- ✅ 95%+ workflow success rate (with Tekton retry)
- ✅ 4 min average MTTR (Tekton optimization)
- ✅ 50% reduction in controller code complexity
- ✅ 90%+ SRE team Tekton familiarity

---

## Consequences

### **Positive Consequences**

#### **1. Zero-Downtime Migration**
```
V1 Production (Q4 2025):
  - Native Jobs
  - 100% traffic

V1 + V2 Coexistence (Q1-Q2 2026):
  - GitOps workflows → Tekton (2%)
  - Simple workflows → Jobs (98%)

V2 Production (Q3 2026):
  - All workflows → Tekton (100%)
  - Jobs reconciler kept for rollback
```

#### **2. Container Portability**
- ✅ Same action containers work in V1 and V2
- ✅ No container rewrites during migration
- ✅ Action registry unchanged

#### **3. Industrial Acceptance**
- ✅ Tekton is CNCF graduated project
- ✅ Used by upstream community Tekton Pipelines
- ✅ Familiar to CI/CD teams

#### **4. Reduced Maintenance**
- ✅ V1: 500+ lines of orchestration code
- ✅ V2: 100 lines (Tekton handles orchestration)
- ✅ 80% code reduction in WorkflowExecution controller

---

### **Negative Consequences**

#### **1. V1 Complexity**
V1 requires custom Job chaining logic for multi-step workflows.

**Mitigation**: V1 orchestration code is well-tested and production-ready by Q4 2025.

#### **2. Dual Reconciler Maintenance (Q1-Q2 2026)**
During migration, need to maintain both Job and Tekton reconcilers.

**Mitigation**: Feature flag makes it easy to A/B test and rollback.

#### **3. Tekton Learning Curve**
SRE teams need to learn Tekton debugging.

**Mitigation**:
- Tekton is industry-standard (CI/CD teams already familiar)
- Comprehensive runbooks and training materials
- upstream community Tekton Pipelines provides enterprise support

---

## Risks and Mitigations

### **Risk 1: Tekton Version Compatibility** 🚨
**Risk**: Tekton API changes may break workflows

**Mitigation**:
- Pin Tekton version in V2 deployment
- Test upgrades in staging before production
- Maintain V1 reconciler as fallback

### **Risk 2: Container Contract Evolution** 🚨
**Risk**: Contract schema changes may break V1/V2 compatibility

**Mitigation**:
- Semantic versioning for contracts (v1.0.0, v2.0.0)
- Backward compatibility guarantee for v1.x.x
- Contract validation in admission controller

### **Risk 3: Performance Regression in V2** 🚨
**Risk**: Tekton overhead may increase MTTR

**Mitigation**:
- A/B testing in production (V1 vs V2)
- Prometheus metrics for both execution paths
- Automatic rollback if MTTR > V1 baseline

---

## Related Decisions

- **[ADR-002: Native Kubernetes Jobs](ADR-002-native-kubernetes-jobs.md)** - V1 execution foundation
- **[ADR-020: Workflow Parallel Execution Limits](ADR-020-workflow-parallel-execution-limits.md)** - Complexity thresholds
- **Future ADR-023**: Tekton Task catalog and community contributions

---

## Links

### **Business Requirements**:
- **BR-PLATFORM-001**: Kubernetes-native architecture (V1)
  - Location: `docs/requirements/03_PLATFORM_KUBERNETES_OPERATIONS.md`
  - Fulfilled: ✅ V1 uses native Jobs, V2 uses Tekton (Kubernetes-native)

- **BR-REMEDIATION-005**: Multi-step workflow orchestration (V1 + V2)
  - Location: `docs/requirements/01_WORKFLOW_ORCHESTRATION.md`
  - Fulfilled: ✅ V1 via Job chaining, V2 via Tekton DAG

- **BR-GITOPS-001**: GitOps PR creation (V1 + V2)
  - Location: `docs/requirements/17_GITOPS_PR_CREATION.md`
  - Fulfilled: ✅ V1 via PVC workspace, V2 via Tekton workspaces

### **Technical Documentation**:
- **Tekton Pipelines**: https://tekton.dev/docs/pipelines/
- **Tekton Workspaces**: https://tekton.dev/docs/pipelines/workspaces/
- **Cosign Image Verification**: https://docs.sigstore.dev/cosign/overview/
- **Sigstore Policy Controller**: https://docs.sigstore.dev/policy-controller/overview/

---

## Decision Record

**Status**: ✅ Approved
**Decision Date**: 2025-10-19
**Approved By**: Architecture Team
**Implementation Target**:
- V1 (Native Jobs): Q4 2025
- V2 (Tekton): Q3 2026

**Confidence**: **95%** (High)

**Key Insight**: **Container portability** is the secret to safe migration. Same Cosign-signed images work in V1 and V2! 🎯

