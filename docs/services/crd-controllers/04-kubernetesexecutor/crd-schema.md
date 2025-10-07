## CRD Schema Specification

**Full Schema**: See [docs/design/CRD/04_KUBERNETES_EXECUTION_CRD.md](../../design/CRD/04_KUBERNETES_EXECUTION_CRD.md)

**Note**: The examples below show the conceptual structure. The authoritative OpenAPI v3 schema is defined in `04_KUBERNETES_EXECUTION_CRD.md`.

**Location**: `api/kubernetesexecution/v1/kubernetesexecution_types.go`

### ✅ **TYPE SAFETY COMPLIANCE**

This CRD specification uses **fully structured types** for all action parameters:

| Type | Approach | Benefit |
|------|----------|---------|
| **ActionParameters** | Discriminated union (10+ action types) | Compile-time validation for each action |
| **RollbackParameters** | Action-specific rollback types | Type-safe rollback operations |
| **ValidationResults** | Structured validation output | Clear validation contract |
| **ExecutionResults** | Structured execution output | Detailed execution tracking |

```go
package v1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubernetesExecutionSpec defines the desired state
type KubernetesExecutionSpec struct {
    // WorkflowExecutionRef references the parent WorkflowExecution CRD
    WorkflowExecutionRef corev1.ObjectReference `json:"workflowExecutionRef"`

    // StepNumber identifies the step within the workflow
    StepNumber int `json:"stepNumber"`

    // Action type (e.g., "scale_deployment", "restart_pod")
    Action string `json:"action"`

    // Parameters for the action (discriminated union based on Action)
    // ✅ TYPE SAFE - See ActionParameters type definition
    Parameters *ActionParameters `json:"parameters"`

    // TargetCluster for multi-cluster support (V2)
    // V1: Always empty string (local cluster)
    TargetCluster string `json:"targetCluster,omitempty"`

    // MaxRetries for failed executions
    MaxRetries int `json:"maxRetries,omitempty"` // Default: 2

    // Timeout for execution
    Timeout metav1.Duration `json:"timeout,omitempty"` // Default: 5m

    // ApprovalReceived flag (set by approval process)
    ApprovalReceived bool `json:"approvalReceived,omitempty"`
}

// ActionParameters is a discriminated union based on Action type
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type ActionParameters struct {
    ScaleDeployment      *ScaleDeploymentParams      `json:"scaleDeployment,omitempty"`
    RolloutRestart       *RolloutRestartParams       `json:"rolloutRestart,omitempty"`
    DeletePod            *DeletePodParams            `json:"deletePod,omitempty"`
    PatchDeployment      *PatchDeploymentParams      `json:"patchDeployment,omitempty"`
    CordonNode           *CordonNodeParams           `json:"cordonNode,omitempty"`
    DrainNode            *DrainNodeParams            `json:"drainNode,omitempty"`
    UncordonNode         *UncordonNodeParams         `json:"uncordonNode,omitempty"`
    UpdateConfigMap      *UpdateConfigMapParams      `json:"updateConfigMap,omitempty"`
    UpdateSecret         *UpdateSecretParams         `json:"updateSecret,omitempty"`
    ApplyManifest        *ApplyManifestParams        `json:"applyManifest,omitempty"`
}

// Action-specific parameter types
type ScaleDeploymentParams struct {
    Deployment string `json:"deployment"`
    Namespace  string `json:"namespace"`
    Replicas   int32  `json:"replicas"`
}

type RolloutRestartParams struct {
    Deployment string `json:"deployment"`
    Namespace  string `json:"namespace"`
}

type DeletePodParams struct {
    Pod              string  `json:"pod"`
    Namespace        string  `json:"namespace"`
    GracePeriodSeconds *int64 `json:"gracePeriodSeconds,omitempty"`
}

type PatchDeploymentParams struct {
    Deployment string `json:"deployment"`
    Namespace  string `json:"namespace"`
    PatchType  string `json:"patchType"` // "strategic", "merge", "json"
    Patch      string `json:"patch"`     // JSON/YAML patch content
}

type CordonNodeParams struct {
    Node string `json:"node"`
}

type DrainNodeParams struct {
    Node               string `json:"node"`
    GracePeriodSeconds int64  `json:"gracePeriodSeconds,omitempty"`
    Force              bool   `json:"force,omitempty"`
    DeleteLocalData    bool   `json:"deleteLocalData,omitempty"`
    IgnoreDaemonSets   bool   `json:"ignoreDaemonSets,omitempty"`
}

type UncordonNodeParams struct {
    Node string `json:"node"`
}

type UpdateConfigMapParams struct {
    ConfigMap string            `json:"configMap"`
    Namespace string            `json:"namespace"`
    Data      map[string]string `json:"data"`
}

type UpdateSecretParams struct {
    Secret    string            `json:"secret"`
    Namespace string            `json:"namespace"`
    Data      map[string][]byte `json:"data"`
}

type ApplyManifestParams struct {
    Manifest string `json:"manifest"` // YAML/JSON manifest content
}

// KubernetesExecutionStatus defines the observed state
type KubernetesExecutionStatus struct {
    // Phase tracks current execution stage
    Phase string `json:"phase"` // "validating", "validated", "waiting_approval", "executing", "rollback_ready", "completed", "failed"

    // ValidationResults from safety checks
    ValidationResults *ValidationResults `json:"validationResults,omitempty"`

    // ExecutionResults from Job execution
    ExecutionResults *ExecutionResults `json:"executionResults,omitempty"`

    // RollbackInformation for potential rollback
    RollbackInformation *RollbackInfo `json:"rollbackInformation,omitempty"`

    // JobName of the execution Job
    JobName string `json:"jobName,omitempty"`

    // Conditions for status tracking
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ValidationResults from pre-execution validation
// ✅ TYPE SAFE - Structured validation output
type ValidationResults struct {
    ParameterValidation bool `json:"parameterValidation"`
    RBACValidation      bool `json:"rbacValidation"`
    ResourceExists      bool `json:"resourceExists"`

    PolicyValidation    *PolicyValidationResult `json:"policyValidation,omitempty"`
    DryRunResults       *DryRunResults          `json:"dryRunResults,omitempty"`

    ValidationTime metav1.Time `json:"validationTime"`
}

type PolicyValidationResult struct {
    PolicyName       string `json:"policyName"`
    Allowed          bool   `json:"allowed"`
    RequiredApproval bool   `json:"requiredApproval"`
    Violations       []string `json:"violations,omitempty"`
}

type DryRunResults struct {
    Performed        bool     `json:"performed"`
    Success          bool     `json:"success"`
    EstimatedImpact  *ImpactAnalysis `json:"estimatedImpact,omitempty"`
    Warnings         []string `json:"warnings,omitempty"`
    Errors           []string `json:"errors,omitempty"`
}

type ImpactAnalysis struct {
    ResourcesAffected int    `json:"resourcesAffected"`
    Description       string `json:"description"` // e.g., "Replicas: 3 -> 5"
}

// ExecutionResults from Job completion
// ✅ TYPE SAFE - Structured execution output
type ExecutionResults struct {
    Success            bool               `json:"success"`
    JobName            string             `json:"jobName"`
    StartTime          *metav1.Time       `json:"startTime,omitempty"`
    EndTime            *metav1.Time       `json:"endTime,omitempty"`
    Duration           string             `json:"duration,omitempty"`
    ResourcesAffected  []AffectedResource `json:"resourcesAffected,omitempty"`
    PodLogs            string             `json:"podLogs,omitempty"`
    RetriesAttempted   int                `json:"retriesAttempted"`
    ErrorMessage       string             `json:"errorMessage,omitempty"`
}

type AffectedResource struct {
    Kind      string `json:"kind"`
    Namespace string `json:"namespace"`
    Name      string `json:"name"`
    Action    string `json:"action"` // "scaled", "restarted", "patched", etc.
    Before    string `json:"before,omitempty"`
    After     string `json:"after,omitempty"`
}

// RollbackInfo for potential rollback operations
// ✅ TYPE SAFE - Action-specific rollback parameters
type RollbackInfo struct {
    Available            bool                 `json:"available"`
    RollbackAction       string               `json:"rollbackAction"`
    RollbackParameters   *RollbackParameters  `json:"rollbackParameters,omitempty"`
    EstimatedDuration    string               `json:"estimatedDuration,omitempty"`
}

// RollbackParameters is a discriminated union based on rollback action
type RollbackParameters struct {
    ScaleToPrevious        *ScaleToPreviousParams        `json:"scaleToPrevious,omitempty"`
    RestorePreviousConfig  *RestorePreviousConfigParams  `json:"restorePreviousConfig,omitempty"`
    UncordonNode           *UncordonNodeParams           `json:"uncordonNode,omitempty"`
    Custom                 *CustomRollbackParams         `json:"custom,omitempty"`
}

type ScaleToPreviousParams struct {
    Deployment       string `json:"deployment"`
    Namespace        string `json:"namespace"`
    PreviousReplicas int32  `json:"previousReplicas"`
}

type RestorePreviousConfigParams struct {
    ResourceKind string `json:"resourceKind"`
    Name         string `json:"name"`
    Namespace    string `json:"namespace"`
    PreviousSpec string `json:"previousSpec"` // JSON-encoded previous spec
}

type CustomRollbackParams struct {
    Description string                 `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"` // Custom rollback params
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// KubernetesExecution is the Schema for the kubernetesexecutions API
type KubernetesExecution struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   KubernetesExecutionSpec   `json:"spec,omitempty"`
    Status KubernetesExecutionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KubernetesExecutionList contains a list of KubernetesExecution
type KubernetesExecutionList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []KubernetesExecution `json:"items"`
}

func init() {
    SchemeBuilder.Register(&KubernetesExecution{}, &KubernetesExecutionList{})
}
```

---

