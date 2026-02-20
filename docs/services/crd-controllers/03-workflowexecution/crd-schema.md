## CRD Schema Specification

**Version**: 4.1
**Last Updated**: 2025-12-06
**Status**: ✅ Aligned with ADR-044, DD-CONTRACT-001 v1.4, ADR-043, DD-WE-004

**Full Schema**: See [docs/design/CRD/04_WORKFLOW_EXECUTION_CRD.md](../../design/CRD/04_WORKFLOW_EXECUTION_CRD.md)

**Location**: `api/workflowexecution/v1alpha1/workflowexecution_types.go`

---

## Key Design Decisions

| Document | Impact on CRD |
|----------|---------------|
| **ADR-044** | **Engine Delegation** - Tekton handles step orchestration; controller just creates PipelineRun |
| **DD-CONTRACT-001 v1.4** | **Enhanced Failure Details** + **Resource Locking** - Rich failure data + parallel execution prevention |
| **ADR-043** | **OCI Bundle** - Workflow definition lives in container, not CRD |
| **DD-WORKFLOW-003** | **Parameters** - UPPER_SNAKE_CASE keys for Tekton params |
| **DD-WE-001** | **Resource Locking** - Prevents parallel/redundant workflows on same target |
| **Issue #91** | **Metadata Migration** - `kubernaut.ai/*` labels removed from CRDs; use spec fields and field selectors |

### Metadata and Filtering (Issue #91)

**Removed from WorkflowExecution CRD**:
- `kubernaut.ai/component` → ownerRef is sufficient for ownership
- `kubernaut.ai/remediation-request` → use `spec.remediationRequestRef` instead

**Operational queries**: Use field selectors (`+kubebuilder:selectablefield`) instead of label-based filtering.

**KEPT on external K8s resources** (PipelineRun, Jobs): `kubernaut.ai/workflow-execution` label for WE-to-Job/PipelineRun correlation. This is on Tekton/K8s resources, not our CRDs.

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

    // TargetResource identifies the K8s resource being remediated
    // Used for resource locking (v3.1) - prevents parallel workflows on same target
    // Format: "namespace/kind/name" for namespaced resources
    //         "kind/name" for cluster-scoped resources
    // Example: "payment/deployment/payment-api", "node/worker-node-1"
    TargetResource string `json:"targetResource"`

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
// Enhanced per DD-CONTRACT-001 v1.3 - rich failure details for recovery flow
// Enhanced per DD-CONTRACT-001 v1.4 - resource locking and Skipped phase
// Enhanced per DD-WE-004 - exponential backoff cooldown
type WorkflowExecutionStatus struct {
    // Phase tracks current execution stage
    // Skipped: Resource is busy (another workflow running) or recently remediated
    // +kubebuilder:validation:Enum=Pending;Running;Completed;Failed;Skipped
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
    // DEPRECATED: Use FailureDetails for structured failure information
    FailureReason string `json:"failureReason,omitempty"`

    // ========================================
    // EXPONENTIAL BACKOFF (v4.1)
    // DD-WE-004: Prevents remediation storms via adaptive cooldown
    // ========================================

    // ConsecutiveFailures tracks consecutive failures for this target resource
    // Resets to 0 on successful completion
    // Used for exponential backoff calculation
    // +optional
    ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`

    // NextAllowedExecution is the earliest timestamp when execution is allowed
    // Calculated using exponential backoff: Base × 2^(failures-1)
    // +optional
    NextAllowedExecution *metav1.Time `json:"nextAllowedExecution,omitempty"`

    // ========================================
    // ENHANCED FAILURE INFORMATION (v3.0)
    // DD-CONTRACT-001 v1.3: Rich failure data for recovery flow
    // Consumers: RO (for recovery AIAnalysis), Notification (for user alerts)
    // ========================================

    // FailureDetails contains structured failure information
    // Populated when Phase=Failed
    // RO uses this to populate AIAnalysis.Spec.PreviousExecutions for recovery
    FailureDetails *FailureDetails `json:"failureDetails,omitempty"`

    // ========================================
    // RESOURCE LOCKING (v3.1)
    // DD-CONTRACT-001 v1.4: Prevents parallel workflows on same target
    // ========================================

    // SkipDetails contains information about why execution was skipped
    // Populated when Phase=Skipped
    // Enables RO to understand why workflow didn't execute
    SkipDetails *SkipDetails `json:"skipDetails,omitempty"`

    // Conditions provide detailed status information
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ========================================
// SKIP DETAILS (v3.1)
// DD-CONTRACT-001 v1.4: Resource locking prevents parallel/redundant execution
// ========================================

// SkipDetails contains information about why a WorkflowExecution was skipped
// Provides context for notifications and audit trail
type SkipDetails struct {
    // Reason explains why execution was skipped
    // +kubebuilder:validation:Enum=ResourceBusy;RecentlyRemediated
    Reason string `json:"reason"`

    // Message is a human-readable explanation
    Message string `json:"message"`

    // SkippedAt is when the skip decision was made
    SkippedAt metav1.Time `json:"skippedAt"`

    // ConflictingWorkflow contains details about the blocking workflow
    // Populated when Reason=ResourceBusy
    ConflictingWorkflow *ConflictingWorkflowRef `json:"conflictingWorkflow,omitempty"`

    // RecentRemediation contains details about the recent execution
    // Populated when Reason=RecentlyRemediated
    RecentRemediation *RecentRemediationRef `json:"recentRemediation,omitempty"`
}

// SkipReasonCode defines reasons for skipping execution
const (
    // SkipReasonResourceBusy indicates another workflow is running on the target
    SkipReasonResourceBusy = "ResourceBusy"

    // SkipReasonRecentlyRemediated indicates same workflow+target was recently executed
    // Also used during exponential backoff for pre-execution failures
    SkipReasonRecentlyRemediated = "RecentlyRemediated"

    // SkipReasonExhaustedRetries indicates max consecutive pre-execution failures reached (DD-WE-004)
    // Requires manual intervention to clear failure count
    SkipReasonExhaustedRetries = "ExhaustedRetries"

    // SkipReasonPreviousExecutionFailed indicates the previous workflow execution
    // ran and failed (wasExecutionFailure: true). No automatic retry is allowed
    // because non-idempotent actions may have occurred. Requires manual intervention.
    // Per cross-team agreement: WE→RO-003
    SkipReasonPreviousExecutionFailed = "PreviousExecutionFailed"
)

// ConflictingWorkflowRef identifies the workflow blocking this execution
type ConflictingWorkflowRef struct {
    // Name of the conflicting WorkflowExecution CRD
    Name string `json:"name"`

    // WorkflowID of the conflicting workflow
    WorkflowID string `json:"workflowId"`

    // StartedAt when the conflicting workflow started
    StartedAt metav1.Time `json:"startedAt"`

    // TargetResource is the resource being remediated (for audit trail)
    // Format: "namespace/kind/name" or "kind/name" for cluster-scoped
    TargetResource string `json:"targetResource"`
}

// RecentRemediationRef identifies the recent execution that caused skip
type RecentRemediationRef struct {
    // Name of the recent WorkflowExecution CRD
    Name string `json:"name"`

    // WorkflowID that was executed
    WorkflowID string `json:"workflowId"`

    // CompletedAt when the workflow completed
    CompletedAt metav1.Time `json:"completedAt"`

    // Outcome of the recent execution (Completed/Failed)
    Outcome string `json:"outcome"`

    // TargetResource is the resource that was remediated
    // Format: "namespace/kind/name" or "kind/name" for cluster-scoped
    TargetResource string `json:"targetResource"`

    // CooldownRemaining is how long until this target can be remediated again
    // Format: Go duration string (e.g., "4m30s")
    CooldownRemaining string `json:"cooldownRemaining,omitempty"`
}

// FailureDetails contains structured failure information for recovery
// DD-CONTRACT-001 v1.3: Aligned with AIAnalysis.Spec.PreviousExecutions[].Failure
// Provides both structured data (for deterministic recovery) and natural language (for LLM context)
type FailureDetails struct {
    // FailedTaskIndex is 0-indexed position of failed task in pipeline
    // Used by RO to populate AIAnalysis.Spec.PreviousExecutions[].Failure.FailedStepIndex
    FailedTaskIndex int `json:"failedTaskIndex"`

    // FailedTaskName is the name of the failed Tekton Task
    // Used by RO to populate AIAnalysis.Spec.PreviousExecutions[].Failure.FailedStepName
    FailedTaskName string `json:"failedTaskName"`

    // FailedStepName is the name of the failed step within the task (if available)
    // Tekton tasks can have multiple steps; this identifies the specific step
    FailedStepName string `json:"failedStepName,omitempty"`

    // Reason is a Kubernetes-style reason code
    // Used for deterministic recovery decisions by RO
    // +kubebuilder:validation:Enum=OOMKilled;DeadlineExceeded;Forbidden;ResourceExhausted;ConfigurationError;ImagePullBackOff;Unknown
    Reason string `json:"reason"`

    // Message is human-readable error message (for logging/UI/notifications)
    Message string `json:"message"`

    // ExitCode from container (if applicable)
    // Useful for script-based tasks that return specific exit codes
    ExitCode *int32 `json:"exitCode,omitempty"`

    // FailedAt is the timestamp when the failure occurred
    FailedAt metav1.Time `json:"failedAt"`

    // ExecutionTimeBeforeFailure is how long the workflow ran before failing
    // Format: Go duration string (e.g., "2m30s")
    ExecutionTimeBeforeFailure string `json:"executionTimeBeforeFailure"`

    // ========================================
    // NATURAL LANGUAGE SUMMARY
    // For LLM recovery context and user notifications
    // ========================================

    // NaturalLanguageSummary is a human/LLM-readable failure description
    // Generated by WE controller from structured data above
    // Example: "Task 'apply-memory-increase' (step 2 of 3) failed after 45s with OOMKilled.
    //           The container exceeded memory limits during kubectl apply operation.
    //           Exit code: 137. This suggests the workflow task itself needs more memory."
    // Used by:
    //   - RO: Included in AIAnalysis.Spec.PreviousExecutions for LLM context
    //   - Notification: Included in user-facing failure alerts
    NaturalLanguageSummary string `json:"naturalLanguageSummary"`
}

// FailureReasonCode defines Kubernetes-style reason codes for workflow failures
// Used for deterministic recovery decisions by RO
const (
    // FailureReasonOOMKilled indicates container was killed due to memory limits
    FailureReasonOOMKilled = "OOMKilled"

    // FailureReasonDeadlineExceeded indicates timeout was reached
    FailureReasonDeadlineExceeded = "DeadlineExceeded"

    // FailureReasonForbidden indicates RBAC/permission failure
    FailureReasonForbidden = "Forbidden"

    // FailureReasonResourceExhausted indicates cluster resource limits (quota, etc.)
    FailureReasonResourceExhausted = "ResourceExhausted"

    // FailureReasonConfigurationError indicates invalid parameters or config
    FailureReasonConfigurationError = "ConfigurationError"

    // FailureReasonImagePullBackOff indicates container image could not be pulled
    FailureReasonImagePullBackOff = "ImagePullBackOff"

    // FailureReasonUnknown for unclassified failures
    FailureReasonUnknown = "Unknown"
)

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
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: workflow-payment-oom-001
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: remediation-payment-oom
    uid: abc-123-def-456
    controller: true
  # Issue #91: kubernaut.ai/* labels removed from CRDs; use spec.remediationRequestRef and field selectors
spec:
  remediationRequestRef:
    name: remediation-payment-oom
    namespace: kubernaut-system
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest

  workflowRef:
    workflowId: "oomkill-increase-memory"
    version: "1.0.0"
    containerImage: "quay.io/kubernaut/workflow-oomkill:v1.0.0"
    containerDigest: "sha256:abc123def456..."

  # v3.1: Target resource for resource locking
  targetResource: "payment/deployment/payment-api"

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

## Complete YAML Example (Failed Execution with FailureDetails)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: workflow-payment-oom-002
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: remediation-payment-oom
    uid: abc-123-def-456
    controller: true
  # Issue #91: kubernaut.ai/* labels removed from CRDs; use spec.remediationRequestRef and field selectors
spec:
  remediationRequestRef:
    name: remediation-payment-oom
    namespace: kubernaut-system
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest

  workflowRef:
    workflowId: "oomkill-increase-memory"
    version: "1.0.0"
    containerImage: "quay.io/kubernaut/workflow-oomkill:v1.0.0"
    containerDigest: "sha256:abc123def456..."

  # v3.1: Target resource for resource locking
  targetResource: "payment/deployment/payment-api"

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
  phase: Failed
  startTime: "2025-12-01T10:15:00Z"
  completionTime: "2025-12-01T10:15:45Z"
  duration: "45s"

  pipelineRunRef:
    name: workflow-payment-oom-002-run

  pipelineRunStatus:
    status: "False"
    reason: "Failed"
    message: "Tasks Completed: 1 (Succeeded: 1, Failed: 1), Skipped: 1"
    completedTasks: 1
    totalTasks: 3

  # DEPRECATED - use failureDetails instead
  failureReason: "permission denied: cannot patch deployments"

  # NEW in v3.0: Structured failure information for recovery
  failureDetails:
    failedTaskIndex: 1
    failedTaskName: "apply-memory-increase"
    failedStepName: "kubectl-patch"
    reason: "Forbidden"
    message: "RBAC denied: cannot patch deployments.apps in namespace payment"
    exitCode: 1
    failedAt: "2025-12-01T10:15:45Z"
    executionTimeBeforeFailure: "45s"
    naturalLanguageSummary: |
      Task 'apply-memory-increase' (step 2 of 3) failed after 45s with Forbidden error.
      The workflow attempted to patch deployment 'payment-api' in namespace 'payment'
      but the service account 'kubernaut-workflow-runner' lacks the required RBAC
      permissions (patch deployments.apps). Exit code: 1.
      Recommendation: Grant patch permission to the service account or use an
      alternative workflow that doesn't require deployment modification.

  conditions:
  - type: PipelineRunCreated
    status: "True"
    reason: PipelineRunCreated
    message: "Tekton PipelineRun created successfully"
    lastTransitionTime: "2025-12-01T10:15:00Z"
  - type: ExecutionComplete
    status: "True"
    reason: Failed
    message: "Workflow execution failed at task 'apply-memory-increase'"
    lastTransitionTime: "2025-12-01T10:15:45Z"
```

---

## Complete YAML Example (Skipped Execution - ResourceBusy)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: workflow-payment-oom-003
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: remediation-payment-oom-duplicate
    uid: def-456-ghi-789
    controller: true
  # Issue #91: kubernaut.ai/* labels removed from CRDs; use spec.remediationRequestRef and field selectors
spec:
  remediationRequestRef:
    name: remediation-payment-oom-duplicate
    namespace: kubernaut-system
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest

  workflowRef:
    workflowId: "oomkill-increase-memory"
    version: "1.0.0"
    containerImage: "quay.io/kubernaut/workflow-oomkill:v1.0.0"
    containerDigest: "sha256:abc123def456..."

  targetResource: "payment/deployment/payment-api"

  parameters:
    NAMESPACE: "payment"
    DEPLOYMENT_NAME: "payment-api"
    NEW_MEMORY_LIMIT: "1Gi"

  confidence: 0.89
  rationale: "OOMKill pattern detected"

  executionConfig:
    timeout: "30m"
    serviceAccountName: "kubernaut-workflow-runner"

status:
  # v3.1: Skipped phase for resource locking
  phase: Skipped

  # No PipelineRun created - skipped before execution
  pipelineRunRef: null

  skipDetails:
    reason: "ResourceBusy"
    message: "Another workflow is currently remediating this resource"
    skippedAt: "2025-12-01T10:16:00Z"
    conflictingWorkflow:
      name: "workflow-payment-oom-001"
      workflowId: "oomkill-increase-memory"
      startedAt: "2025-12-01T10:15:00Z"
      targetResource: "payment/deployment/payment-api"

  conditions:
  - type: ResourceLockAcquired
    status: "False"
    reason: ResourceBusy
    message: "Resource payment/deployment/payment-api is being remediated by workflow-payment-oom-001"
    lastTransitionTime: "2025-12-01T10:16:00Z"
```

---

## Complete YAML Example (Skipped Execution - RecentlyRemediated)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: workflow-node-disk-004
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: remediation-node-disk-duplicate
    uid: ghi-789-jkl-012
    controller: true
  # Issue #91: kubernaut.ai/* labels removed from CRDs; use spec.remediationRequestRef and field selectors
spec:
  remediationRequestRef:
    name: remediation-node-disk-duplicate
    namespace: kubernaut-system
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest

  workflowRef:
    workflowId: "node-disk-cleanup"
    version: "1.0.0"
    containerImage: "quay.io/kubernaut/workflow-disk-cleanup:v1.0.0"
    containerDigest: "sha256:def456ghi789..."

  targetResource: "node/worker-node-1"

  parameters:
    NODE_NAME: "worker-node-1"
    CLEANUP_PATHS: "/var/log,/tmp"

  confidence: 0.95
  rationale: "DiskPressure detected on node"

  executionConfig:
    timeout: "15m"
    serviceAccountName: "kubernaut-node-runner"

status:
  phase: Skipped

  skipDetails:
    reason: "RecentlyRemediated"
    message: "Same workflow was recently executed on this resource"
    skippedAt: "2025-12-01T10:20:00Z"
    recentRemediation:
      name: "workflow-node-disk-001"
      workflowId: "node-disk-cleanup"
      completedAt: "2025-12-01T10:18:00Z"
      outcome: "Completed"
      targetResource: "node/worker-node-1"
      cooldownRemaining: "4m30s"

  conditions:
  - type: ResourceLockAcquired
    status: "False"
    reason: RecentlyRemediated
    message: "Resource node/worker-node-1 was remediated 2m ago by workflow-node-disk-001 (cooldown: 5m)"
    lastTransitionTime: "2025-12-01T10:20:00Z"
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
    case "Completed", "Failed", "Skipped":
        // Terminal states - no action needed
        return ctrl.Result{}, nil
    default:
        _log.Error(nil, "Unknown phase", "phase", wfe.Status.Phase)
        return ctrl.Result{}, nil
    }
}

// handlePending checks resource lock and creates the Tekton PipelineRun
func (r *WorkflowExecutionReconciler) handlePending(
    ctx context.Context,
    wfe *kubernautv1alpha1.WorkflowExecution,
) (ctrl.Result, error) {
    _log := log.FromContext(ctx)

    // v3.1: Check resource lock BEFORE creating PipelineRun
    skipDetails, shouldSkip := r.checkResourceLock(ctx, wfe)
    if shouldSkip {
        _log.Info("Skipping execution due to resource lock",
            "reason", skipDetails.Reason,
            "target", wfe.Spec.TargetResource)

        wfe.Status.Phase = "Skipped"
        wfe.Status.SkipDetails = skipDetails

        if err := r.Status().Update(ctx, wfe); err != nil {
            return ctrl.Result{}, err
        }

        // Write audit trace for skipped execution
        go r.AuditClient.WriteExecutionSkipped(ctx, wfe)

        return ctrl.Result{}, nil
    }

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
                // Issue #91: KEPT - label on external K8s resource (PipelineRun) for WE-to-PipelineRun correlation
                "kubernaut.ai/workflow-execution": wfe.Name,
                "kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
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
            // DEPRECATED: Keep for backward compatibility
            wfe.Status.FailureReason = pipelineRun.Status.GetCondition(
                tektonv1.PipelineRunConditionSucceeded,
            ).GetMessage()

            // NEW v3.0: Build structured failure details for recovery flow
            wfe.Status.FailureDetails = r.buildFailureDetails(&pipelineRun, wfe)
            _log.Info("Workflow failed",
                "reason", wfe.Status.FailureDetails.Reason,
                "task", wfe.Status.FailureDetails.FailedTaskName)
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

// buildFailureDetails extracts structured failure information from PipelineRun
// DD-CONTRACT-001 v1.3: Provides rich data for recovery flow and user notifications
func (r *WorkflowExecutionReconciler) buildFailureDetails(
    pr *tektonv1.PipelineRun,
    wfe *kubernautv1alpha1.WorkflowExecution,
) *kubernautv1alpha1.FailureDetails {
    fd := &kubernautv1alpha1.FailureDetails{
        FailedAt: metav1.Now(),
        Reason:   kubernautv1alpha1.FailureReasonUnknown,
    }

    // Calculate execution time before failure
    if wfe.Status.StartTime != nil {
        duration := time.Since(wfe.Status.StartTime.Time)
        fd.ExecutionTimeBeforeFailure = duration.Round(time.Second).String()
    }

    // Find the failed task from ChildReferences
    for i, childRef := range pr.Status.ChildReferences {
        if childRef.PipelineTaskName != "" {
            // Get TaskRun to check its status
            var taskRun tektonv1.TaskRun
            if err := r.Get(context.Background(), client.ObjectKey{
                Name:      childRef.Name,
                Namespace: pr.Namespace,
            }, &taskRun); err != nil {
                continue
            }

            // Check if this TaskRun failed
            condition := taskRun.Status.GetCondition(tektonv1.TaskRunConditionSucceeded)
            if condition != nil && condition.IsFalse() {
                fd.FailedTaskIndex = i
                fd.FailedTaskName = childRef.PipelineTaskName
                fd.Message = condition.Message

                // Extract step name if available
                for _, step := range taskRun.Status.Steps {
                    if step.Terminated != nil && step.Terminated.ExitCode != 0 {
                        fd.FailedStepName = step.Name
                        fd.ExitCode = &step.Terminated.ExitCode
                        fd.FailedAt = step.Terminated.FinishedAt
                        break
                    }
                }

                // Map Tekton/K8s reasons to our enum
                fd.Reason = mapTektonReasonToFailureReason(condition.Reason, condition.Message)
                break
            }
        }
    }

    // Generate natural language summary
    fd.NaturalLanguageSummary = r.generateNaturalLanguageSummary(fd, wfe, pr)

    return fd
}

// mapTektonReasonToFailureReason converts Tekton/K8s reasons to our enum
func mapTektonReasonToFailureReason(reason, message string) string {
    messageLower := strings.ToLower(message)

    switch {
    case strings.Contains(messageLower, "oomkilled") || strings.Contains(messageLower, "oom"):
        return kubernautv1alpha1.FailureReasonOOMKilled
    case reason == "TaskRunTimeout" || strings.Contains(messageLower, "timeout") ||
         strings.Contains(messageLower, "deadline"):
        return kubernautv1alpha1.FailureReasonDeadlineExceeded
    case strings.Contains(messageLower, "forbidden") || strings.Contains(messageLower, "rbac") ||
         strings.Contains(messageLower, "permission denied"):
        return kubernautv1alpha1.FailureReasonForbidden
    case strings.Contains(messageLower, "quota") || strings.Contains(messageLower, "resource"):
        return kubernautv1alpha1.FailureReasonResourceExhausted
    case strings.Contains(messageLower, "imagepullbackoff") || strings.Contains(messageLower, "image"):
        return kubernautv1alpha1.FailureReasonImagePullBackOff
    case strings.Contains(messageLower, "invalid") || strings.Contains(messageLower, "configuration"):
        return kubernautv1alpha1.FailureReasonConfigurationError
    default:
        return kubernautv1alpha1.FailureReasonUnknown
    }
}

// generateNaturalLanguageSummary creates a human/LLM-readable failure description
func (r *WorkflowExecutionReconciler) generateNaturalLanguageSummary(
    fd *kubernautv1alpha1.FailureDetails,
    wfe *kubernautv1alpha1.WorkflowExecution,
    pr *tektonv1.PipelineRun,
) string {
    var sb strings.Builder

    // Task identification
    totalTasks := len(pr.Status.ChildReferences)
    sb.WriteString(fmt.Sprintf("Task '%s' (step %d of %d) failed after %s with %s error.\n",
        fd.FailedTaskName,
        fd.FailedTaskIndex+1,
        totalTasks,
        fd.ExecutionTimeBeforeFailure,
        fd.Reason))

    // Error message
    sb.WriteString(fmt.Sprintf("Error: %s\n", fd.Message))

    // Exit code if available
    if fd.ExitCode != nil {
        sb.WriteString(fmt.Sprintf("Exit code: %d.\n", *fd.ExitCode))
    }

    // Reason-specific recommendations
    switch fd.Reason {
    case kubernautv1alpha1.FailureReasonOOMKilled:
        sb.WriteString("Recommendation: The workflow task itself ran out of memory. Consider increasing task resource limits or using a workflow with smaller memory footprint.\n")
    case kubernautv1alpha1.FailureReasonForbidden:
        sb.WriteString(fmt.Sprintf("Recommendation: The service account '%s' lacks required RBAC permissions. Grant appropriate permissions or use an alternative workflow.\n",
            wfe.Spec.ExecutionConfig.ServiceAccountName))
    case kubernautv1alpha1.FailureReasonDeadlineExceeded:
        sb.WriteString("Recommendation: The workflow exceeded its timeout. Consider increasing the timeout or using a faster workflow variant.\n")
    case kubernautv1alpha1.FailureReasonImagePullBackOff:
        sb.WriteString("Recommendation: Unable to pull the workflow container image. Verify image exists and credentials are configured.\n")
    }

    return sb.String()
}

// ========================================
// RESOURCE LOCKING LOGIC (v3.1)
// DD-CONTRACT-001 v1.4: Prevents parallel/redundant workflows on same target
// ========================================

// checkResourceLock verifies no other workflow is running or recently ran on the target
// Returns (SkipDetails, shouldSkip)
func (r *WorkflowExecutionReconciler) checkResourceLock(
    ctx context.Context,
    wfe *kubernautv1alpha1.WorkflowExecution,
) (*kubernautv1alpha1.SkipDetails, bool) {
    targetResource := wfe.Spec.TargetResource
    workflowID := wfe.Spec.WorkflowRef.WorkflowID

    // List all WorkflowExecutions in the namespace
    var wfeList kubernautv1alpha1.WorkflowExecutionList
    if err := r.List(ctx, &wfeList, client.InNamespace(wfe.Namespace)); err != nil {
        // On error, allow execution (fail-open for availability)
        return nil, false
    }

    for _, existing := range wfeList.Items {
        // Skip self
        if existing.Name == wfe.Name {
            continue
        }

        // Check if targeting the same resource
        if existing.Spec.TargetResource != targetResource {
            continue
        }

        // Check 1: Is another workflow RUNNING on this resource?
        if existing.Status.Phase == "Running" || existing.Status.Phase == "Pending" {
            return &kubernautv1alpha1.SkipDetails{
                Reason:    kubernautv1alpha1.SkipReasonResourceBusy,
                Message:   "Another workflow is currently remediating this resource",
                SkippedAt: metav1.Now(),
                ConflictingWorkflow: &kubernautv1alpha1.ConflictingWorkflowRef{
                    Name:           existing.Name,
                    WorkflowID:     existing.Spec.WorkflowRef.WorkflowID,
                    StartedAt:      *existing.Status.StartTime,
                    TargetResource: existing.Spec.TargetResource,
                },
            }, true
        }

        // Check 2: Was the SAME workflow recently executed on this resource?
        // Only check same workflow+target combination
        if existing.Spec.WorkflowRef.WorkflowID != workflowID {
            continue // Different workflow is allowed
        }

        // Check if recently completed (within cooldown period)
        if existing.Status.Phase == "Completed" || existing.Status.Phase == "Failed" {
            if existing.Status.CompletionTime != nil {
                cooldown := 5 * time.Minute // Configurable via controller config
                elapsed := time.Since(existing.Status.CompletionTime.Time)

                if elapsed < cooldown {
                    remaining := cooldown - elapsed
                    return &kubernautv1alpha1.SkipDetails{
                        Reason:    kubernautv1alpha1.SkipReasonRecentlyRemediated,
                        Message:   "Same workflow was recently executed on this resource",
                        SkippedAt: metav1.Now(),
                        RecentRemediation: &kubernautv1alpha1.RecentRemediationRef{
                            Name:              existing.Name,
                            WorkflowID:        existing.Spec.WorkflowRef.WorkflowID,
                            CompletedAt:       *existing.Status.CompletionTime,
                            Outcome:           existing.Status.Phase,
                            TargetResource:    existing.Spec.TargetResource,
                            CooldownRemaining: remaining.Round(time.Second).String(),
                        },
                    }, true
                }
            }
        }
    }

    return nil, false
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
| **DD-CONTRACT-001 v1.3** | AIAnalysis ↔ WorkflowExecution contract alignment, recovery data flow |
| **ADR-043** | Workflow Schema Definition (OCI bundle format) |
| **DD-WORKFLOW-003** | Parameterized Actions (UPPER_SNAKE_CASE parameters) |
| **DD-WORKFLOW-005** | Automated Schema Extraction (workflow registration) |
| **DD-WORKFLOW-011** | Tekton Pipeline OCI Bundles |
| **BR-WE-001** | Create PipelineRun from OCI Bundle |
| **BR-HAPI-191** | Primary Parameter Validation (HolmesGPT-API) |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 4.2 | 2026-02-18 | **Issue #91**: Removed `kubernaut.ai/remediation-request` and `kubernaut.ai/workflow-id` labels from WorkflowExecution CRD examples. Added Metadata and Filtering section: field selectors replace label-based filtering; `kubernaut.ai/workflow-execution` KEPT on PipelineRun (external resource). |
| 4.1 | 2025-12-06 | **Exponential Backoff (DD-WE-004)**: Added `ConsecutiveFailures` and `NextAllowedExecution` status fields. Added `SkipReasonExhaustedRetries` and `SkipReasonPreviousExecutionFailed` constants. Backoff applies only to pre-execution failures; execution failures block retries immediately. |
| 3.1 | 2025-12-01 | **Resource Locking (V1.0 Safety)**: Added `targetResource` to spec. Added `Skipped` phase with `SkipDetails` struct. Prevents parallel workflows on same target (ResourceBusy). Prevents redundant sequential workflows with same workflow+target (RecentlyRemediated). Added `SkipDetails`, `ConflictingWorkflowRef`, `RecentRemediationRef` types. Added controller logic for `checkResourceLock()`. Aligned with DD-CONTRACT-001 v1.4. Audit trail for skipped executions. |
| 3.0 | 2025-12-01 | **Enhanced Failure Details**: Added `FailureDetails` struct with structured failure information for recovery flow. Includes `failedTaskIndex`, `failedTaskName`, `reason` (K8s-style enum), `naturalLanguageSummary` for LLM context. Deprecated `failureReason` string field. Added controller logic for extracting failure details from PipelineRun. Aligned with DD-CONTRACT-001 v1.3. |
| 2.0 | 2025-11-28 | **Breaking**: Simplified schema per ADR-044. Replaced `WorkflowDefinition` with `WorkflowRef`. Removed step orchestration, status tracking, rollback logic. Controller now creates single PipelineRun and watches status. |
| 1.x | Prior | Complex schema with embedded WorkflowDefinition, step-level status, rollback spec, preconditions/postconditions |

