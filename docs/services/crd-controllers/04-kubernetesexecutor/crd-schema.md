## CRD Schema Specification

> **DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service were eliminated by ADR-025 and replaced by Tekton TaskRun via WorkflowExecution. This documentation is retained for historical reference only. API types and CRD manifests have been removed from the codebase.

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

    // ✅ NEW: Action Validation Framework (DD-002)
    // See: docs/architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2
    // PreConditions validated before Job creation (BR-EXEC-016)
    PreConditions []ActionCondition `json:"preConditions,omitempty"`

    // PostConditions verified after Job completion (BR-EXEC-036)
    PostConditions []ActionCondition `json:"postConditions,omitempty"`
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

// ========================================
// ACTION VALIDATION FRAMEWORK (DD-002)
// ✅ NEW: Per-action precondition/postcondition validation
// See: docs/architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2
// ========================================

// ActionCondition defines a validation rule for individual actions
// Evaluated before (precondition) or after (postcondition) action execution
// Semantically identical to StepCondition but scoped to action-level validation
type ActionCondition struct {
    // Type categorizes the condition (e.g., "resource_state", "capacity_check", "rbac_permissions")
    Type string `json:"type"`

    // Description provides human-readable explanation of the condition
    Description string `json:"description"`

    // Rego contains the Rego policy expression for evaluation
    // Policy should return 'allow' decision for condition pass
    // Example: "allow if { input.sufficient_capacity == true }"
    Rego string `json:"rego"`

    // Required determines if condition failure blocks execution
    // - true: Condition failure blocks Job creation (precondition) or marks execution failed (postcondition)
    // - false: Condition failure logs warning but allows execution to proceed
    Required bool `json:"required"`

    // Timeout specifies maximum wait time for async condition checks
    // Example: "30s" for preconditions, "2m" for postconditions (pod startup)
    // Default: "30s"
    Timeout string `json:"timeout,omitempty"`
}

// ConditionResult captures the outcome of a single condition evaluation
// Used for both preconditions and postconditions
// Identical structure to WorkflowExecution.ConditionResult for consistency
type ConditionResult struct {
    // ConditionType identifies which condition was evaluated
    ConditionType string `json:"conditionType"`

    // Evaluated indicates whether the condition was actually evaluated
    Evaluated bool `json:"evaluated"`

    // Passed indicates whether the condition passed
    Passed bool `json:"passed"`

    // ErrorMessage provides details when condition fails
    ErrorMessage string `json:"errorMessage,omitempty"`

    // EvaluationTime records when the condition was evaluated
    EvaluationTime metav1.Time `json:"evaluationTime"`
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

    // ✅ NEW: Action Validation Framework (DD-002)
    // Precondition evaluation results (BR-EXEC-016)
    PreConditionResults []ConditionResult `json:"preConditionResults,omitempty"`

    // Postcondition verification results (BR-EXEC-036)
    // Note: Postconditions evaluated after Job completion, stored here for consistency
    PostConditionResults []ConditionResult `json:"postConditionResults,omitempty"`
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

## Representative Example: scale_deployment with Precondition/Postcondition Validation

> **📋 Design Decision Status**
>
> **Current Implementation**: **DD-002 Alternative 2** (Approved Design)
> **Status**: ✅ **Framework Design Complete**
> **Confidence**: 78%
> **Design Decision**: [DD-002](../../../architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2)
> **Business Requirements**: BR-EXEC-016, BR-EXEC-036
>
> <details>
> <summary><b>Why DD-002?</b> (Click to expand)</summary>
>
> - ✅ **Action-Level Validation**: Extends workflow-level checks with action-specific validation
> - ✅ **Integrates with Dry-Run**: Preconditions evaluated before dry-run, postconditions after Job completion
> - ✅ **Verifies Outcomes**: Confirms kubectl action achieved intended effect
> - ✅ **Leverages Infrastructure**: Reuses Rego policy engine (BR-REGO-001 to BR-REGO-010)
>
> **Full Analysis**: See [STEP_VALIDATION_BUSINESS_REQUIREMENTS.md](../../../requirements/STEP_VALIDATION_BUSINESS_REQUIREMENTS.md)
> </details>

This example demonstrates the per-action validation framework with a `scale_deployment` action:

```yaml
apiVersion: execution.kubernaut.io/v1alpha1
kind: KubernetesExecution
metadata:
  name: web-app-scale-step-1
  namespace: production
spec:
  workflowExecutionRef:
    name: web-app-scale-workflow
    namespace: production

  stepNumber: 1
  action: "scale_deployment"

  parameters:
    scaleDeployment:
      deployment: "web-app"
      namespace: "production"
      replicas: 5

  maxRetries: 2
  timeout: "5m"
  approvalReceived: false

  # ========================================
  # PRECONDITIONS (DD-002, BR-EXEC-016)
  # Validated during "validating" phase BEFORE Job creation
  # ========================================
  preConditions:
    - type: sufficient_cluster_capacity
      description: "Cluster must have capacity for additional pods"
      rego: |
        package precondition
        import future.keywords.if
        allow if {
          input.available_cpu >= input.required_cpu_per_pod * input.additional_replicas
          input.available_memory >= input.required_memory_per_pod * input.additional_replicas
        }
      required: true
      timeout: "10s"

    - type: image_pull_secrets_valid
      description: "Deployment must have valid image pull secrets"
      rego: |
        package precondition
        import future.keywords.if
        allow if { count(input.invalid_secrets) == 0 }
      required: true
      timeout: "5s"

    - type: node_selector_matches
      description: "Cluster should have nodes matching deployment node selector"
      rego: |
        package precondition
        import future.keywords.if
        allow if { input.matching_nodes > 0 }
      required: false  # warning only, not blocking
      timeout: "5s"

  # ========================================
  # POSTCONDITIONS (DD-002, BR-EXEC-036)
  # Verified during "executing" phase AFTER Job completion
  # ========================================
  postConditions:
    - type: desired_replicas_running
      description: "All desired replicas must be running and ready"
      rego: |
        package postcondition
        import future.keywords.if
        allow if {
          input.running_pods >= input.target_replicas
          input.ready_pods >= input.target_replicas
        }
      required: true
      timeout: "2m"  # wait for pods to start

    - type: deployment_health_check
      description: "Deployment must be Available and Progressing"
      rego: |
        package postcondition
        import future.keywords.if
        allow if {
          input.conditions.Available == true
          input.conditions.Progressing == true
        }
      required: true
      timeout: "1m"

    - type: resource_usage_acceptable
      description: "Pods should not be throttled or OOMKilled"
      rego: |
        package postcondition
        import future.keywords.if
        allow if {
          count([p | p := input.pods[_]; p.throttled == true]) == 0
          count([p | p := input.pods[_]; p.oom_killed == true]) == 0
        }
      required: true
      timeout: "1m"

status:
  phase: "completed"

  # ========================================
  # PRECONDITION EVALUATION RESULTS
  # Evaluated during "validating" phase
  # ========================================
  validationResults:
    parameterValidation: true
    rbacValidation: true
    resourceExists: true

    policyValidation:
      policyName: "scale-deployment-policy"
      allowed: true
      requiredApproval: false

    dryRunResults:
      performed: true
      success: true
      estimatedImpact:
        resourcesAffected: 1
        estimatedDuration: "2m"
      warnings: []
      errors: []

    validationTime: "2025-10-14T10:00:00Z"

    # ✅ NEW: Precondition evaluation results
    preConditionResults:
      - conditionType: sufficient_cluster_capacity
        evaluated: true
        passed: true
        evaluationTime: "2025-10-14T10:00:01Z"

      - conditionType: image_pull_secrets_valid
        evaluated: true
        passed: true
        evaluationTime: "2025-10-14T10:00:02Z"

      - conditionType: node_selector_matches
        evaluated: true
        passed: false  # warning only, execution proceeded
        errorMessage: "Only 1 node matches node selector, recommend adding more nodes"
        evaluationTime: "2025-10-14T10:00:03Z"

    # ✅ NEW: Postcondition verification results
    # Evaluated after Job completion
    postConditionResults:
      - conditionType: desired_replicas_running
        evaluated: true
        passed: true
        evaluationTime: "2025-10-14T10:02:25Z"

      - conditionType: deployment_health_check
        evaluated: true
        passed: true
        evaluationTime: "2025-10-14T10:02:26Z"

      - conditionType: resource_usage_acceptable
        evaluated: true
        passed: true
        evaluationTime: "2025-10-14T10:02:27Z"

  # ========================================
  # JOB EXECUTION RESULTS
  # ========================================
  executionResults:
    success: true
    jobName: "web-app-scale-job"
    duration: "15s"
    affectedResources:
      - kind: "Deployment"
        namespace: "production"
        name: "web-app"
        action: "scaled"
        before: "3 replicas"
        after: "5 replicas"

  jobName: "web-app-scale-job"

  conditions:
    - type: "Validated"
      status: "True"
      lastTransitionTime: "2025-10-14T10:00:03Z"
      reason: "AllValidationsPassed"
      message: "All preconditions passed, dry-run successful"
    - type: "Executed"
      status: "True"
      lastTransitionTime: "2025-10-14T10:02:30Z"
      reason: "JobCompleted"
      message: "All postconditions passed, deployment scaled successfully"
```

### Validation Flow

```
1. KubernetesExecutor enters "validating" phase
   ↓
2. Perform existing validation (parameters, RBAC, resource existence)
   ↓
3. Evaluate spec.preConditions[] (BR-EXEC-016) [NEW]
   - sufficient_cluster_capacity: ✅ PASS (blocking)
   - image_pull_secrets_valid: ✅ PASS (blocking)
   - node_selector_matches: ❌ FAIL (warning only, log and proceed)
   ↓ (all required preconditions passed)
4. Perform dry-run validation (existing)
   ↓ (dry-run successful)
5. Create Kubernetes Job
   ↓
6. Monitor Job execution
   ↓ (Job.status.succeeded = 1)
7. Evaluate spec.postConditions[] (BR-EXEC-036) [NEW]
   - desired_replicas_running: ✅ PASS (5 pods running)
   - deployment_health_check: ✅ PASS (Available=true, Progressing=true)
   - resource_usage_acceptable: ✅ PASS (no throttling, no OOM)
   ↓ (all postconditions passed)
8. Mark execution as "completed"
```

### Condition Template Placeholder

**Note**: This representative example shows complete precondition/postcondition policies for the `scale_deployment` action. Condition templates for the **remaining 26 actions** will be defined during implementation.

See [Precondition/Postcondition Framework](../standards/precondition-postcondition-framework.md) for phased rollout strategy:
- **Phase 1** (Weeks 1-2): Top 5 actions with complete condition templates
- **Phase 2** (Weeks 3-4): Next 10 actions (infrastructure, storage, application lifecycle)
- **Phase 3** (Weeks 5-6): Remaining 12 actions (security, network, database, monitoring)

---

