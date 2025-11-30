## CRD Schema Specification

**Version**: 2.0
**Last Updated**: 2025-11-28
**Status**: ✅ Aligned with ADR-044, DD-CONTRACT-001, ADR-043

**Full Schema**: See [docs/design/CRD/04_WORKFLOW_EXECUTION_CRD.md](../../design/CRD/04_WORKFLOW_EXECUTION_CRD.md)

**Location**: `api/v1alpha1/workflowexecution_types.go`

---

## Key Design Decisions

| Document | Impact on CRD |
|----------|---------------|
| **ADR-044** | **Engine Delegation** - Tekton handles step orchestration; controller just creates PipelineRun |
| **DD-CONTRACT-001** | **Simplified Spec** - Uses `WorkflowRef` with containerImage, not complex WorkflowDefinition |
| **ADR-043** | **OCI Bundle** - Workflow definition lives in container, not CRD |
| **DD-WORKFLOW-003** | **Parameters** - UPPER_SNAKE_CASE keys for Tekton params |

---

## Architecture Overview (ADR-044)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    SIMPLIFIED WORKFLOW EXECUTION                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  WorkflowExecution Controller Responsibilities:                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ 1. Receive WorkflowExecution CRD (from RemediationOrchestrator) │   │
│  │ 2. Create Tekton PipelineRun from OCI bundle                    │   │
│  │ 3. Watch PipelineRun status                                     │   │
│  │ 4. Update WorkflowExecution status                              │   │
│  │ 5. Write audit trace                                            │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Tekton Responsibilities (DELEGATED):                                   │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ • Step orchestration and execution                              │   │
│  │ • Retry logic per task                                          │   │
│  │ • Timeout enforcement                                           │   │
│  │ • Failure handling and cleanup (finally tasks)                  │   │
│  │ • Parameter injection                                           │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  Controller does NOT:                                                   │
│  ✗ Orchestrate individual steps                                        │
│  ✗ Create per-step CRDs                                                │
│  ✗ Handle rollback (Tekton finally tasks or not at all)                │
│  ✗ Transform workflow definition                                        │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Go Type Definitions (DD-CONTRACT-001)

```go
// pkg/api/workflowexecution/v1alpha1/types.go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkflowExecutionSpec defines the desired state of WorkflowExecution
// Simplified per ADR-044 - Tekton handles step orchestration
type WorkflowExecutionSpec struct {
    // RemediationRequestRef references the parent RemediationRequest CRD
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // WorkflowRef contains the workflow catalog reference
    // Resolved from AIAnalysis.Status.SelectedWorkflow by RemediationOrchestrator
    WorkflowRef WorkflowRef `json:"workflowRef"`

    // Parameters from LLM selection (per DD-WORKFLOW-003)
    // Keys are UPPER_SNAKE_CASE for Tekton PipelineRun params
    Parameters map[string]string `json:"parameters"`

    // Confidence score from LLM (for audit trail)
    Confidence float64 `json:"confidence"`

    // Rationale from LLM (for audit trail)
    Rationale string `json:"rationale,omitempty"`

    // ExecutionConfig contains minimal execution settings
    ExecutionConfig ExecutionConfig `json:"executionConfig,omitempty"`
}

// WorkflowRef contains catalog-resolved workflow reference
type WorkflowRef struct {
    // WorkflowID is the catalog lookup key
    WorkflowID string `json:"workflowId"`

    // Version of the workflow
    Version string `json:"version"`

    // ContainerImage resolved from workflow catalog (Data Storage API)
    // OCI bundle reference for Tekton PipelineRun
    ContainerImage string `json:"containerImage"`

    // ContainerDigest for audit trail and reproducibility
    ContainerDigest string `json:"containerDigest,omitempty"`
}

// ExecutionConfig contains minimal execution settings
// Note: Most execution logic is delegated to Tekton (ADR-044)
type ExecutionConfig struct {
    // Timeout for the entire workflow (Tekton PipelineRun timeout)
    // Default: use global timeout from RemediationRequest or 30m
    Timeout *metav1.Duration `json:"timeout,omitempty"`

    // ServiceAccountName for the PipelineRun
    // Default: "kubernaut-workflow-runner"
    ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// WorkflowExecutionStatus defines the observed state
// Simplified per ADR-044 - just tracks PipelineRun status
type WorkflowExecutionStatus struct {
    // Phase tracks current execution stage
    // +kubebuilder:validation:Enum=Pending;Running;Completed;Failed
    Phase string `json:"phase"`

    // StartTime when execution started
    StartTime *metav1.Time `json:"startTime,omitempty"`

    // CompletionTime when execution completed (success or failure)
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`

    // Duration of the execution
    Duration string `json:"duration,omitempty"`

    // PipelineRunRef references the created Tekton PipelineRun
    PipelineRunRef *corev1.LocalObjectReference `json:"pipelineRunRef,omitempty"`

    // PipelineRunStatus mirrors key PipelineRun status fields
    PipelineRunStatus *PipelineRunStatusSummary `json:"pipelineRunStatus,omitempty"`

    // FailureReason explains why execution failed (if applicable)
    FailureReason string `json:"failureReason,omitempty"`

    // Conditions provide detailed status information
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// PipelineRunStatusSummary captures key PipelineRun status fields
// Lightweight summary to avoid duplicating full Tekton status
type PipelineRunStatusSummary struct {
    // Status from PipelineRun (Unknown, True, False)
    Status string `json:"status"`

    // Reason from PipelineRun (e.g., "Succeeded", "Failed", "Running")
    Reason string `json:"reason,omitempty"`

    // Message from PipelineRun
    Message string `json:"message,omitempty"`

    // CompletedTasks count
    CompletedTasks int `json:"completedTasks,omitempty"`

    // TotalTasks count (from pipeline spec)
    TotalTasks int `json:"totalTasks,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="WorkflowID",type=string,JSONPath=`.spec.workflowRef.workflowId`
//+kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.status.duration`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WorkflowExecution is the Schema for the workflowexecutions API
type WorkflowExecution struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   WorkflowExecutionSpec   `json:"spec,omitempty"`
    Status WorkflowExecutionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WorkflowExecutionList contains a list of WorkflowExecution
type WorkflowExecutionList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []WorkflowExecution `json:"items"`
}

func init() {
    SchemeBuilder.Register(&WorkflowExecution{}, &WorkflowExecutionList{})
}
```

---

## Complete YAML Example

```yaml
apiVersion: kubernaut.io/v1alpha1
kind: WorkflowExecution
metadata:
  name: workflow-payment-oom-001
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: kubernaut.io/v1alpha1
    kind: RemediationRequest
    name: remediation-payment-oom
    uid: abc-123-def-456
    controller: true
  labels:
    kubernaut.io/remediation-request: remediation-payment-oom
    kubernaut.io/workflow-id: oomkill-increase-memory
spec:
  remediationRequestRef:
    name: remediation-payment-oom
    namespace: kubernaut-system
    apiVersion: kubernaut.io/v1alpha1
    kind: RemediationRequest

  workflowRef:
    workflowId: "oomkill-increase-memory"
    version: "1.0.0"
    containerImage: "quay.io/kubernaut/workflow-oomkill:v1.0.0"
    containerDigest: "sha256:abc123def456..."

  parameters:
    NAMESPACE: "payment"
    DEPLOYMENT_NAME: "payment-api"
    NEW_MEMORY_LIMIT: "1Gi"

  confidence: 0.92
  rationale: "High confidence match for OOMKill pattern"

  executionConfig:
    timeout: "30m"
    serviceAccountName: "kubernaut-workflow-runner"

status:
  phase: Completed
  startTime: "2025-11-28T10:15:00Z"
  completionTime: "2025-11-28T10:18:30Z"
  duration: "3m30s"

  pipelineRunRef:
    name: workflow-payment-oom-001-run

  pipelineRunStatus:
    status: "True"
    reason: "Succeeded"
    message: "All tasks completed successfully"
    completedTasks: 3
    totalTasks: 3

  conditions:
  - type: PipelineRunCreated
    status: "True"
    reason: PipelineRunCreated
    message: "Tekton PipelineRun created successfully"
    lastTransitionTime: "2025-11-28T10:15:00Z"
  - type: ExecutionComplete
    status: "True"
    reason: Succeeded
    message: "Workflow execution completed successfully"
    lastTransitionTime: "2025-11-28T10:18:30Z"
```

---

## Controller Logic (Simplified per ADR-044)

```go
// pkg/workflowexecution/controller.go
package controller

import (
    "context"
    "fmt"
    "time"

    tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/v1alpha1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"
)

type WorkflowExecutionReconciler struct {
    client.Client
    AuditClient AuditClient
}

func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    _log := log.FromContext(ctx)

    var wfe kubernautv1alpha1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &wfe); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    switch wfe.Status.Phase {
    case "", "Pending":
        return r.handlePending(ctx, &wfe)
    case "Running":
        return r.handleRunning(ctx, &wfe)
    case "Completed", "Failed":
        // Terminal states - no action needed
        return ctrl.Result{}, nil
    default:
        _log.Error(nil, "Unknown phase", "phase", wfe.Status.Phase)
        return ctrl.Result{}, nil
    }
}

// handlePending creates the Tekton PipelineRun
func (r *WorkflowExecutionReconciler) handlePending(
    ctx context.Context,
    wfe *kubernautv1alpha1.WorkflowExecution,
) (ctrl.Result, error) {
    _log := log.FromContext(ctx)
    _log.Info("Creating PipelineRun", "workflowId", wfe.Spec.WorkflowRef.WorkflowID)

    // Build Tekton PipelineRun
    pipelineRun := r.buildPipelineRun(wfe)

    // Create PipelineRun
    if err := r.Create(ctx, pipelineRun); err != nil {
        _log.Error(err, "Failed to create PipelineRun")
        return ctrl.Result{}, err
    }

    // Update status
    now := metav1.Now()
    wfe.Status.Phase = "Running"
    wfe.Status.StartTime = &now
    wfe.Status.PipelineRunRef = &corev1.LocalObjectReference{
        Name: pipelineRun.Name,
    }

    if err := r.Status().Update(ctx, wfe); err != nil {
        return ctrl.Result{}, err
    }

    // Write audit trace
    go r.AuditClient.WriteExecutionStarted(ctx, wfe)

    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// buildPipelineRun creates the Tekton PipelineRun spec
func (r *WorkflowExecutionReconciler) buildPipelineRun(
    wfe *kubernautv1alpha1.WorkflowExecution,
) *tektonv1.PipelineRun {
    // Convert parameters to Tekton format
    params := make([]tektonv1.Param, 0, len(wfe.Spec.Parameters))
    for key, value := range wfe.Spec.Parameters {
        params = append(params, tektonv1.Param{
            Name:  key,
            Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: value},
        })
    }

    // Determine timeout
    timeout := metav1.Duration{Duration: 30 * time.Minute}
    if wfe.Spec.ExecutionConfig.Timeout != nil {
        timeout = *wfe.Spec.ExecutionConfig.Timeout
    }

    // Service account
    serviceAccount := "kubernaut-workflow-runner"
    if wfe.Spec.ExecutionConfig.ServiceAccountName != "" {
        serviceAccount = wfe.Spec.ExecutionConfig.ServiceAccountName
    }

    return &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-run", wfe.Name),
            Namespace: wfe.Namespace,
            Labels: map[string]string{
                "kubernaut.io/workflow-execution": wfe.Name,
                "kubernaut.io/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
            },
            OwnerReferences: []metav1.OwnerReference{
                {
                    APIVersion:         wfe.APIVersion,
                    Kind:               wfe.Kind,
                    Name:               wfe.Name,
                    UID:                wfe.UID,
                    Controller:         ptrBool(true),
                    BlockOwnerDeletion: ptrBool(true),
                },
            },
        },
        Spec: tektonv1.PipelineRunSpec{
            PipelineRef: &tektonv1.PipelineRef{
                ResolverRef: tektonv1.ResolverRef{
                    Resolver: "bundles",
                    Params: []tektonv1.Param{
                        {
                            Name:  "bundle",
                            Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.WorkflowRef.ContainerImage},
                        },
                        {
                            Name:  "name",
                            Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.WorkflowRef.WorkflowID},
                        },
                    },
                },
            },
            Params: params,
            Timeouts: &tektonv1.TimeoutFields{
                Pipeline: &timeout,
            },
            TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
                ServiceAccountName: serviceAccount,
            },
        },
    }
}

// handleRunning watches PipelineRun status
func (r *WorkflowExecutionReconciler) handleRunning(
    ctx context.Context,
    wfe *kubernautv1alpha1.WorkflowExecution,
) (ctrl.Result, error) {
    _log := log.FromContext(ctx)

    // Get PipelineRun
    if wfe.Status.PipelineRunRef == nil {
        _log.Error(nil, "PipelineRunRef is nil in Running phase")
        return ctrl.Result{}, fmt.Errorf("pipelineRunRef is nil")
    }

    var pipelineRun tektonv1.PipelineRun
    if err := r.Get(ctx, client.ObjectKey{
        Name:      wfe.Status.PipelineRunRef.Name,
        Namespace: wfe.Namespace,
    }, &pipelineRun); err != nil {
        _log.Error(err, "Failed to get PipelineRun")
        return ctrl.Result{}, err
    }

    // Update status summary
    wfe.Status.PipelineRunStatus = r.buildStatusSummary(&pipelineRun)

    // Check completion
    if pipelineRun.IsDone() {
        now := metav1.Now()
        wfe.Status.CompletionTime = &now

        if wfe.Status.StartTime != nil {
            duration := now.Sub(wfe.Status.StartTime.Time)
            wfe.Status.Duration = duration.Round(time.Second).String()
        }

        if pipelineRun.Status.GetCondition(tektonv1.PipelineRunConditionSucceeded).IsTrue() {
            wfe.Status.Phase = "Completed"
            _log.Info("Workflow completed successfully")
        } else {
            wfe.Status.Phase = "Failed"
            wfe.Status.FailureReason = pipelineRun.Status.GetCondition(
                tektonv1.PipelineRunConditionSucceeded,
            ).GetMessage()
            _log.Info("Workflow failed", "reason", wfe.Status.FailureReason)
        }

        // Write audit trace
        go r.AuditClient.WriteExecutionCompleted(ctx, wfe)
    }

    if err := r.Status().Update(ctx, wfe); err != nil {
        return ctrl.Result{}, err
    }

    // Requeue if still running
    if wfe.Status.Phase == "Running" {
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    return ctrl.Result{}, nil
}

// buildStatusSummary creates a lightweight status summary
func (r *WorkflowExecutionReconciler) buildStatusSummary(
    pr *tektonv1.PipelineRun,
) *kubernautv1alpha1.PipelineRunStatusSummary {
    condition := pr.Status.GetCondition(tektonv1.PipelineRunConditionSucceeded)
    if condition == nil {
        return &kubernautv1alpha1.PipelineRunStatusSummary{
            Status: "Unknown",
        }
    }

    completed := 0
    total := 0
    for _, taskStatus := range pr.Status.ChildReferences {
        total++
        if taskStatus.PipelineTaskName != "" {
            // Task has been created
            completed++
        }
    }

    return &kubernautv1alpha1.PipelineRunStatusSummary{
        Status:         string(condition.Status),
        Reason:         condition.Reason,
        Message:        condition.Message,
        CompletedTasks: completed,
        TotalTasks:     total,
    }
}

func ptrBool(b bool) *bool {
    return &b
}
```

---

## Migration from v1.x Schema

| Old Field (v1.x) | New Field (v2.0) | Notes |
|------------------|------------------|-------|
| `workflowDefinition` | `workflowRef` | OCI bundle reference, not embedded definition |
| `workflowDefinition.steps[]` | Removed | Steps live in Tekton Pipeline (ADR-044) |
| `workflowDefinition.dependencies` | Removed | Tekton handles dependencies |
| `executionStrategy` | `executionConfig` | Simplified to timeout + serviceAccount |
| `stepStatuses[]` | Removed | Use PipelineRun.status directly |
| `executionPlan` | Removed | Tekton determines execution plan |
| `validationResults` | Removed | Validation in Tekton tasks |
| `adaptiveOrchestration` | Removed | Out of scope for v1.0 |
| `executionSnapshot` | Removed | Recovery handled by RO |

---

## What Controller Does NOT Do (ADR-044)

| Responsibility | Owner | Notes |
|---------------|-------|-------|
| Step orchestration | **Tekton** | Controller creates single PipelineRun |
| Retry logic per step | **Tekton** | Tekton task retries |
| Step timeout enforcement | **Tekton** | Pipeline task timeouts |
| Rollback on failure | **Tekton/None** | Use Tekton `finally` tasks or don't rollback |
| Parameter transformation | **None** | Pass through from spec to PipelineRun |
| Workflow definition parsing | **Tekton** | OCI bundle contains Pipeline definition |

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| **ADR-044** | **Authoritative** - Workflow Execution Engine Delegation |
| **DD-CONTRACT-001** | AIAnalysis ↔ WorkflowExecution contract alignment |
| **ADR-043** | Workflow Schema Definition (OCI bundle format) |
| **DD-WORKFLOW-003** | Parameterized Actions (UPPER_SNAKE_CASE parameters) |
| **DD-WORKFLOW-005** | Automated Schema Extraction (workflow registration) |
| **DD-WORKFLOW-011** | Tekton Pipeline OCI Bundles |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.0 | 2025-11-28 | **Breaking**: Simplified schema per ADR-044. Replaced `WorkflowDefinition` with `WorkflowRef`. Removed step orchestration, status tracking, rollback logic. Controller now creates single PipelineRun and watches status. |
| 1.x | Prior | Complex schema with embedded WorkflowDefinition, step-level status, rollback spec, preconditions/postconditions |

