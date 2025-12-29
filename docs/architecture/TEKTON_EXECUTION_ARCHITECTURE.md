# Kubernaut Tekton Execution Architecture

**Version**: 2.0
**Date**: 2025-10-19
**Status**: ✅ Approved
**Decision**: [ADR-023: Tekton from V1](decisions/ADR-023-tekton-from-v1.md)
**Update**: [ADR-024: Eliminate ActionExecution Layer](decisions/ADR-024-eliminate-actionexecution-layer.md)

---

## Executive Summary

Kubernaut uses **Tekton Pipelines** as its workflow execution engine from **V1 (Q4 2025)**. This eliminates 500+ lines of custom orchestration code and ensures maximum Red Hat alignment through **OpenShift Pipelines**.

**Key Principles**:
- ✅ **Universal Availability**: OpenShift Pipelines (bundled) + Upstream Tekton (open source)
- ✅ **Zero Throwaway Code**: Single architecture from V1 onward (no migration)
- ✅ **Generic Meta-Task**: One Tekton Task executes all 29+ action containers
- ✅ **Direct Integration**: WorkflowExecution → Tekton PipelineRun (no intermediate layers)
- ✅ **Persistent Business Data**: Data Storage Service for action history and analytics (not CRDs)

---

## Architecture Overview

### **Execution Flow**

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
│ Source: OpenShift Pipelines (Red Hat customers)         │
│         OR Upstream Tekton (all other customers)        │
│                                                          │
│ Capabilities:                                            │
│ - DAG orchestration (runAfter dependencies)             │
│ - Parallel execution (multiple tasks simultaneously)    │
│ - Workspace management (shared PVC for multi-step)      │
│ - Retry and timeout (per-task configuration)            │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Tekton TaskRun (Generic Meta-Task)                      │
│ Task: kubernaut-action                                   │
│ - Executes ANY action container                          │
│ - Verifies Cosign signatures (via admission controller) │
│ - Loads action contract from container                   │
│ - Captures outputs to Tekton results                     │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Pod (Action Container)                                   │
│ Image: ghcr.io/kubernaut/actions/{k8s|gitops|aws}@sha256 │
│ Contract: /action-contract.yaml (embedded)              │
│ Security: Cosign-verified at admission time             │
│ Execution: Reads inputs from env, writes outputs to stdout │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Data Storage Service                                     │
│ - Stores action history (90+ days)                      │
│ - Stores effectiveness metrics                          │
│ - Queried by Pattern Monitoring                         │
│ - Queried by Effectiveness Monitor                      │
└─────────────────────────────────────────────────────────┘
```

---

## Core Components

### **1. WorkflowExecution Controller** (100 lines)

**Responsibilities**:
- Translate `WorkflowExecution` CRD to Tekton `PipelineRun`
- Monitor `PipelineRun` status
- Sync status to `WorkflowExecution` CRD
- Write action records to Data Storage Service (for pattern monitoring and effectiveness tracking)

**Key Code**:
```go
package controller

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/datastorage"
    tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/resource"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkflowExecutionReconciler reconciles WorkflowExecution objects
type WorkflowExecutionReconciler struct {
    client.Client
    DataStorageClient *datastorage.Client  // For action record persistence
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
        return r.handlePipelineRunCreation(ctx, workflow)
    case "Executing":
        return r.handlePipelineRunMonitoring(ctx, workflow)
    case "Completed", "Failed":
        return ctrl.Result{}, nil
    default:
        return ctrl.Result{}, nil
    }
}

// handlePipelineRunCreation translates WorkflowExecution to Tekton PipelineRun
func (r *WorkflowExecutionReconciler) handlePipelineRunCreation(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Create Tekton PipelineRun (no intermediate ActionExecution)
    pipelineRun := r.createPipelineRun(workflow)
    if err := r.Create(ctx, pipelineRun); err != nil {
        log.Error(err, "Failed to create PipelineRun")
        return ctrl.Result{}, err
    }

    log.Info("Created PipelineRun", "pipelineRun", pipelineRun.Name)

    // Write action records to Data Storage Service
    for _, step := range workflow.Spec.Steps {
        actionRecord := &datastorage.ActionRecord{
            WorkflowID:  workflow.Name,
            ActionType:  step.ActionType,
            Image:       step.Image,
            Inputs:      step.Inputs,
            ExecutedAt:  time.Now(),
            Status:      "executing",
        }
        if err := r.DataStorageClient.RecordAction(ctx, actionRecord); err != nil {
            log.Error(err, "Failed to record action", "step", step.Name)
            // Best effort - continue
        }
    }

    // Transition to Executing
    workflow.Status.Phase = "Executing"
    workflow.Status.Message = "PipelineRun created, monitoring execution"

    if err := r.Status().Update(ctx, workflow); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{Requeue: true}, nil
}

// createPipelineRun translates WorkflowExecution to Tekton PipelineRun
func (r *WorkflowExecutionReconciler) createPipelineRun(
    workflow *workflowv1.WorkflowExecution,
) *tektonv1.PipelineRun {
    // Build Tekton Tasks from WorkflowExecution steps
    tasks := make([]tektonv1.PipelineTask, len(workflow.Spec.Steps))

    for i, step := range workflow.Spec.Steps {
        // Marshal inputs to JSON
        inputsJSON, _ := json.Marshal(step.Inputs)

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
                        StringVal: step.Image,  // Cosign-signed image with digest
                    },
                },
                {
                    Name: "inputs",
                    Value: tektonv1.ParamValue{
                        Type:      tektonv1.ParamTypeString,
                        StringVal: string(inputsJSON),
                    },
                },
            },
            RunAfter: step.RunAfter,  // Tekton handles dependencies
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
                        Name:        "shared-workspace",
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

// createActionExecution creates tracking CRD for pattern monitoring
func (r *WorkflowExecutionReconciler) createActionExecution(
    workflow *workflowv1.WorkflowExecution,
    step workflowv1.WorkflowStep,
) *executionv1.ActionExecution {
    return &executionv1.ActionExecution{
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
                Policy: "strict",
            },
            Inputs: step.Inputs,
        },
    }
}
```

---

### **2. Generic Meta-Task** (Tekton Task)

**Purpose**: Single Tekton Task that executes ANY action container

**Specification**:
```yaml
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: kubernaut-action
  namespace: kubernaut-system
spec:
  description: |
    Generic Tekton Task that executes any Kubernaut action container.
    Container contract (embedded in image) defines action behavior.

  params:
    - name: actionType
      type: string
      description: "Action type (e.g., kubernetes/scale_deployment)"

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
    # Cosign verification happens at admission time (Sigstore Policy Controller)

    - name: execute
      image: $(params.actionImage)
      env:
        - name: ACTION_TYPE
          value: $(params.actionType)
        - name: ACTION_INPUTS
          value: $(params.inputs)
        - name: WORKSPACE_PATH
          value: $(workspaces.workspace.path)
      script: |
        #!/bin/sh
        set -e

        # Action containers read inputs from ACTION_INPUTS env var
        echo "$ACTION_INPUTS" | /action-entrypoint

        # Outputs written to stdout (captured by Tekton results)

  results:
    - name: outputs
      description: "JSON outputs from action container"
```

**Benefits**:
- ✅ 1 Task definition (not 29+)
- ✅ Container contract defines behavior
- ✅ Extensible without Task changes
- ✅ Action registry in ConfigMap

---

### **3. ActionExecution Controller** (Tracking Layer)

**Purpose**: Provide business-level tracking separate from Tekton execution

**Responsibilities**:
- Create ActionExecution CRDs for each workflow step
- Translate ActionExecution to Tekton TaskRun (optional - can be done by WorkflowExecution)
- Monitor TaskRun status
- Sync outputs back to ActionExecution CRD
- Enable pattern monitoring and effectiveness tracking

**Why Retain ActionExecution**:
1. **Pattern Monitoring**: Needs per-action CRDs with structured metadata
2. **Effectiveness Tracking**: Requires action-level success/failure metrics
3. **Audit Trail**: Kubernaut CRDs provide business-level audit (separate from Tekton)
4. **Clean Separation**: Business logic (Kubernaut) vs execution (Tekton)

---

## Deployment Prerequisites

### **For OpenShift Customers** (Primary Target)

OpenShift Pipelines (Tekton) is bundled with OpenShift 4.x.

**Installation** (if not already installed):
```bash
# Check installation
oc get pods -n openshift-pipelines

# Install OpenShift Pipelines Operator (2 minutes)
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: openshift-pipelines-operator
  namespace: openshift-operators
spec:
  channel: stable
  name: openshift-pipelines-operator-rh
  source: redhat-operators
EOF

# Verify
oc get pods -n openshift-pipelines
```

**Effort**: ✅ Pre-installed or 2-minute install
**Support**: Red Hat enterprise support

---

### **For Non-OpenShift Customers**

Upstream Tekton Pipelines is open source and available for any Kubernetes cluster.

**Installation** (5 minutes):
```bash
# Install Tekton Pipelines
kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml

# Verify installation
kubectl get pods -n tekton-pipelines
NAME                                           READY   STATUS
tekton-pipelines-controller-68b8d87b6c-9xj2k   1/1     Running
tekton-pipelines-webhook-6c8d8c6f9d-7k4lm      1/1     Running

# Optional: Install Tekton Dashboard (for debugging)
kubectl apply -f https://storage.googleapis.com/tekton-releases/dashboard/latest/release.yaml

# Optional: Install Tekton CLI (tkn)
# macOS
brew install tektoncd-cli

# Linux
curl -LO https://github.com/tektoncd/cli/releases/download/v0.33.0/tkn_0.33.0_Linux_x86_64.tar.gz
tar xvzf tkn_0.33.0_Linux_x86_64.tar.gz -C /usr/local/bin/ tkn
```

**Effort**: ✅ 5-minute one-time install
**Support**: CNCF community support

---

## Complete Workflow Example

### **WorkflowExecution CRD** (User-Defined)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: remediate-payment-oom
  namespace: kubernaut-system
spec:
  workflowType: multi-step-remediation
  reason: "OOMKilled alert detected for payment-service"

  steps:
    # Step 1, 2: Parallel emergency actions
    - name: restart-pods
      actionType: kubernetes/restart_pod
      image: ghcr.io/kubernaut/actions/restart@sha256:def456
      inputs:
        namespace: production
        labelSelector: app=payment
      runAfter: []  # No dependencies = runs immediately

    - name: scale-deployment
      actionType: kubernetes/scale_deployment
      image: ghcr.io/kubernaut/actions/scale@sha256:abc123
      inputs:
        deployment: payment-service
        namespace: production
        replicas: 10
      runAfter: []  # No dependencies = runs immediately (parallel with restart-pods)

    # Step 3: GitOps PR (depends on both emergency actions)
    - name: create-gitops-pr
      actionType: git/create-pr
      image: ghcr.io/kubernaut/actions/git-pr@sha256:ghi789
      usesWorkspace: true
      inputs:
        repository: https://github.com/company/k8s-configs
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
      runAfter:
        - restart-pods
        - scale-deployment
```

---

### **Generated Tekton PipelineRun**

```yaml
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: remediate-payment-oom
  namespace: kubernaut-system
  labels:
    kubernaut.io/workflow: remediate-payment-oom
  ownerReferences:
    - apiVersion: kubernaut.ai/v1alpha1
      kind: WorkflowExecution
      name: remediate-payment-oom
spec:
  pipelineSpec:
    tasks:
      # Parallel tasks (no runAfter = execute immediately)
      - name: restart-pods
        taskRef:
          name: kubernaut-action
        params:
          - name: actionType
            value: kubernetes/restart_pod
          - name: actionImage
            value: ghcr.io/kubernaut/actions/restart@sha256:def456
          - name: inputs
            value: '{"namespace":"production","labelSelector":"app=payment"}'

      - name: scale-deployment
        taskRef:
          name: kubernaut-action
        params:
          - name: actionType
            value: kubernetes/scale_deployment
          - name: actionImage
            value: ghcr.io/kubernaut/actions/scale@sha256:abc123
          - name: inputs
            value: '{"deployment":"payment-service","namespace":"production","replicas":10}'

      # Sequential task (runAfter = waits for dependencies)
      - name: create-gitops-pr
        taskRef:
          name: kubernaut-action
        params:
          - name: actionType
            value: git/create-pr
          - name: actionImage
            value: ghcr.io/kubernaut/actions/git-pr@sha256:ghi789
          - name: inputs
            value: '{"repository":"https://github.com/company/k8s-configs","branch":"kubernaut/payment-memory-fix","title":"Kubernaut: Fix payment-service OOMKilled (AI-recommended)","body":"..."}'
        runAfter:
          - restart-pods      # Wait for restart-pods
          - scale-deployment  # AND wait for scale-deployment
        workspaces:
          - name: workspace
            workspace: shared-workspace

    workspaces:
      - name: shared-workspace
        description: "Shared workspace for GitOps PR creation"

  workspaces:
    - name: shared-workspace
      volumeClaimTemplate:
        spec:
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 1Gi

status:
  # Tekton automatically populates status
  pipelineRunStatusFields:
    startTime: "2025-10-19T10:00:00Z"
    completionTime: "2025-10-19T10:01:30Z"
    conditions:
      - type: Succeeded
        status: "True"
        reason: "Succeeded"
    taskRuns:
      restart-pods-abc123:
        status:
          conditions:
            - type: Succeeded
              status: "True"
      scale-deployment-def456:
        status:
          conditions:
            - type: Succeeded
              status: "True"
      create-gitops-pr-ghi789:
        status:
          conditions:
            - type: Succeeded
              status: "True"
```

**Execution Timeline**:
```
00:00 - restart-pods START (parallel)
00:00 - scale-deployment START (parallel)
00:15 - restart-pods COMPLETE
00:20 - scale-deployment COMPLETE
00:20 - create-gitops-pr START (both dependencies met)
01:30 - create-gitops-pr COMPLETE
```

**Duration**: ~90 seconds total

---

## Security: Cosign Image Signing

All action containers **must** be Cosign-signed. Verification happens at admission time via **Sigstore Policy Controller**.

### **Image Signing** (CI/CD Pipeline)

```bash
# Build action container
docker build -t ghcr.io/kubernaut/actions/scale:v1.0.0 .

# Sign with Cosign (keyless with GitHub OIDC)
cosign sign ghcr.io/kubernaut/actions/scale:v1.0.0 \
  --oidc-issuer https://token.actions.githubusercontent.com \
  --oidc-client-id kubernaut

# Push with digest
docker push ghcr.io/kubernaut/actions/scale:v1.0.0

# Get digest for registry
DIGEST=$(docker inspect ghcr.io/kubernaut/actions/scale:v1.0.0 \
  --format='{{index .RepoDigests 0}}')

# Result: ghcr.io/kubernaut/actions/scale@sha256:abc123
```

---

### **Image Verification** (Admission Controller)

```yaml
# Install Sigstore Policy Controller
kubectl apply -f https://github.com/sigstore/policy-controller/releases/latest/download/release.yaml

# Configure policy for Kubernaut actions
apiVersion: policy.sigstore.dev/v1alpha1
kind: ClusterImagePolicy
metadata:
  name: kubernaut-actions
spec:
  images:
    - glob: "ghcr.io/kubernaut/actions/**"
  authorities:
    - keyless:
        url: https://fulcio.sigstore.dev
        identities:
          - issuer: "https://token.actions.githubusercontent.com"
            subject: "https://github.com/kubernaut/*"
```

**Effect**:
- ✅ Tekton TaskRun Pods validated at admission time
- ❌ Unsigned images rejected before Pod creation
- ✅ Works identically for V1 (no custom Job logic needed)

---

## Monitoring and Debugging

### **Tekton CLI (tkn)**

```bash
# List PipelineRuns
tkn pipelinerun list -n kubernaut-system

# View PipelineRun status
tkn pipelinerun describe remediate-payment-oom -n kubernaut-system

# View PipelineRun logs
tkn pipelinerun logs remediate-payment-oom -n kubernaut-system

# View TaskRun logs
tkn taskrun logs remediate-payment-oom-restart-pods-abc123 -n kubernaut-system
```

---

### **Tekton Dashboard**

```bash
# Forward Tekton Dashboard
kubectl port-forward -n tekton-pipelines svc/tekton-dashboard 9097:9097

# Access at http://localhost:9097
```

**Dashboard Features**:
- ✅ Visual PipelineRun timeline
- ✅ Task logs and status
- ✅ Parameter inspection
- ✅ Workspace content viewing

---

### **Kubernetes Native**

```bash
# View PipelineRuns
kubectl get pipelineruns -n kubernaut-system

# View TaskRuns
kubectl get taskruns -n kubernaut-system

# View PipelineRun status
kubectl describe pipelinerun remediate-payment-oom -n kubernaut-system

# View TaskRun logs
kubectl logs -n kubernaut-system -l tekton.dev/taskRun=remediate-payment-oom-restart-pods-abc123
```

---

## Benefits Summary

### **Development Efficiency**
- ✅ **500+ lines eliminated**: No custom DAG, retry, workspace management
- ✅ **8 weeks development**: vs 16 weeks (custom V1 + Tekton V2)
- ✅ **Zero throwaway code**: Single architecture from V1 onward
- ✅ **100 lines of code**: PipelineRun translation (vs 600 lines custom orchestration)

### **Red Hat Alignment**
- ✅ **OpenShift Pipelines**: Tekton is the standard
- ✅ **Pre-installed**: No additional deployment burden
- ✅ **Enterprise support**: Red Hat backing
- ✅ **Familiar to teams**: OpenShift teams already know Tekton

### **Operational Excellence**
- ✅ **CNCF Graduated**: Same trust level as Kubernetes
- ✅ **Battle-tested**: Used by major enterprises
- ✅ **Rich tooling**: Tekton CLI, Dashboard, visualization
- ✅ **Active community**: Google, IBM, Red Hat contributors

### **Universal Availability**
- ✅ **OpenShift**: Bundled (0 effort)
- ✅ **Kubernetes**: Upstream install (5 minutes)
- ✅ **EKS, GKE, AKS**: Fully compatible
- ✅ **No customer blocked**: Universal availability

---

## Related Documentation

- **[ADR-023: Tekton from V1](decisions/ADR-023-tekton-from-v1.md)** - Architectural decision
- **[WorkflowExecution Controller](../services/crd-controllers/03-workflowexecution/)** - Controller specification
- **[ActionExecution Controller](../services/crd-controllers/04-kubernetesexecutor/)** - Tracking layer specification
- **[Action Container Registry](../services/action-execution/ACTION_REGISTRY.md)** - Action catalog
- **[Cosign Verification Guide](../security/COSIGN_VERIFICATION.md)** - Image signing

---

## External Resources

- **Tekton Pipelines**: https://tekton.dev/docs/pipelines/
- **Tekton Tasks**: https://tekton.dev/docs/pipelines/tasks/
- **Tekton Workspaces**: https://tekton.dev/docs/pipelines/workspaces/
- **OpenShift Pipelines**: https://docs.openshift.com/pipelines/
- **Tekton CLI (tkn)**: https://tekton.dev/docs/cli/
- **Sigstore Cosign**: https://docs.sigstore.dev/cosign/overview/

---

**Status**: ✅ Approved
**Version**: 1.0
**Last Updated**: 2025-10-19
**Owner**: Architecture Team

