## CRD Schema Specification

**Full Schema**: See [docs/design/CRD/04_WORKFLOW_EXECUTION_CRD.md](../../design/CRD/04_WORKFLOW_EXECUTION_CRD.md)

**Note**: The examples below show the conceptual structure. The authoritative OpenAPI v3 schema is defined in `04_WORKFLOW_EXECUTION_CRD.md`.

**Location**: `api/v1/workflowexecution_types.go`

### ✅ **TYPE SAFETY COMPLIANCE**

This CRD specification uses **fully structured types** and eliminates all `map[string]interface{}` anti-patterns:

| Type | Previous (Anti-Pattern) | Current (Type-Safe) | Benefit |
|------|------------------------|---------------------|---------|
| **WorkflowStep.Parameters** | `map[string]interface{}` | Action-specific parameter types (10+ types) | Compile-time validation, self-documenting |
| **StepStatus.Result** | `map[string]interface{}` | Action-specific result types | Type-safe execution results |
| **AIRecommendations** | `map[string]interface{}` | Structured AI response | Clear HolmesGPT contract |
| **DryRunResults** | `map[string]interface{}` | Structured validation results | Detailed dry-run analysis |
| **RollbackSpec.Parameters** | `map[string]interface{}` | Rollback-specific parameters | Type-safe rollback operations |

**Related Triage**: See `WORKFLOW_EXECUTION_TYPE_SAFETY_TRIAGE.md` for detailed analysis and remediation plan.

**Total Structured Types**: 30+ types defined for comprehensive type safety

```go
package v1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkflowExecutionSpec defines the desired state of WorkflowExecution
type WorkflowExecutionSpec struct {
    // RemediationRequestRef references the parent RemediationRequest CRD
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // WorkflowDefinition contains the workflow to execute
    WorkflowDefinition WorkflowDefinition `json:"workflowDefinition"`

    // ExecutionStrategy specifies how to execute the workflow
    ExecutionStrategy ExecutionStrategy `json:"executionStrategy"`

    // AdaptiveOrchestration enables runtime optimization
    AdaptiveOrchestration AdaptiveOrchestrationConfig `json:"adaptiveOrchestration,omitempty"`
}

// WorkflowDefinition represents the workflow to execute
type WorkflowDefinition struct {
    Name             string                  `json:"name"`
    Version          string                  `json:"version"`
    Steps            []WorkflowStep          `json:"steps"`
    Dependencies     map[string][]string     `json:"dependencies,omitempty"`
    AIRecommendations *AIRecommendations     `json:"aiRecommendations,omitempty"` // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
}

// WorkflowStep represents a single step in the workflow
type WorkflowStep struct {
    StepNumber     int                    `json:"stepNumber"`
    Name           string                 `json:"name"`
    Action         string                 `json:"action"` // e.g., "scale-deployment", "restart-pod"
    TargetCluster  string                 `json:"targetCluster"`
    Parameters     *StepParameters        `json:"parameters"` // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    CriticalStep   bool                   `json:"criticalStep"` // Failure triggers rollback
    MaxRetries     int                    `json:"maxRetries,omitempty"`
    Timeout        string                 `json:"timeout,omitempty"` // e.g., "5m"
    DependsOn      []int                  `json:"dependsOn,omitempty"` // Step numbers
    RollbackSpec   *RollbackSpec          `json:"rollbackSpec,omitempty"`

    // ✅ NEW: Step Validation Framework (DD-002)
    // See: docs/architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2
    PreConditions  []StepCondition        `json:"preConditions,omitempty"`  // BR-WF-016: Validate before step execution
    PostConditions []StepCondition        `json:"postConditions,omitempty"` // BR-WF-052: Verify after step completion
}

// RollbackSpec defines how to rollback a step
type RollbackSpec struct {
    Action     string                 `json:"action"` // e.g., "restore-previous-config"
    Parameters *RollbackParameters    `json:"parameters"` // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    Timeout    string                 `json:"timeout,omitempty"`
}

// ExecutionStrategy specifies execution behavior
type ExecutionStrategy struct {
    ApprovalRequired bool   `json:"approvalRequired"`
    DryRunFirst      bool   `json:"dryRunFirst"`
    RollbackStrategy string `json:"rollbackStrategy"` // "automatic", "manual", "none"
    MaxRetries       int    `json:"maxRetries,omitempty"`
    SafetyChecks     []SafetyCheck `json:"safetyChecks,omitempty"`
}

// SafetyCheck represents a validation requirement
type SafetyCheck struct {
    Type        string `json:"type"` // "rbac", "capacity", "health", "network"
    Description string `json:"description"`
    Required    bool   `json:"required"`
}

// AdaptiveOrchestrationConfig enables runtime optimization
type AdaptiveOrchestrationConfig struct {
    OptimizationEnabled      bool `json:"optimizationEnabled"`
    LearningFromHistory      bool `json:"learningFromHistory"`
    DynamicStepAdjustment    bool `json:"dynamicStepAdjustment"`
}

// ========================================
// STEP VALIDATION FRAMEWORK (DD-002)
// ✅ NEW: Per-step precondition/postcondition validation
// See: docs/architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2
// ========================================

// StepCondition defines a validation rule for workflow steps
// Evaluated before (precondition) or after (postcondition) step execution
type StepCondition struct {
    // Type categorizes the condition (e.g., "resource_state", "metric_threshold", "pod_count")
    Type string `json:"type"`

    // Description provides human-readable explanation of the condition
    Description string `json:"description"`

    // Rego contains the Rego policy expression for evaluation
    // Policy should return 'allow' decision for condition pass
    // Example: "allow if { input.deployment_found == true }"
    Rego string `json:"rego"`

    // Required determines if condition failure blocks execution
    // - true: Condition failure blocks step execution (precondition) or marks step failed (postcondition)
    // - false: Condition failure logs warning but allows execution to proceed
    Required bool `json:"required"`

    // Timeout specifies maximum wait time for async condition checks
    // Example: "30s" for preconditions, "2m" for postconditions (pod startup)
    // Default: "30s"
    Timeout string `json:"timeout,omitempty"`
}

// WorkflowExecutionStatus defines the observed state
type WorkflowExecutionStatus struct {
    // Phase tracks current execution stage
    Phase string `json:"phase"` // "planning", "validating", "executing", "monitoring", "completed", "failed"

    // CurrentStep tracks progress
    CurrentStep int `json:"currentStep"`
    TotalSteps  int `json:"totalSteps"`

    // ExecutionPlan generated during planning phase
    ExecutionPlan *ExecutionPlan `json:"executionPlan,omitempty"`

    // ValidationResults from validation phase
    ValidationResults *ValidationResults `json:"validationResults,omitempty"`

    // StepStatuses tracks individual step execution
    StepStatuses []StepStatus `json:"stepStatuses,omitempty"`

    // ExecutionMetrics tracks workflow performance
    ExecutionMetrics *ExecutionMetrics `json:"executionMetrics,omitempty"`

    // AdaptiveAdjustments made during execution
    AdaptiveAdjustments []AdaptiveAdjustment `json:"adaptiveAdjustments,omitempty"`

    // WorkflowResult final outcome
    WorkflowResult *WorkflowResult `json:"workflowResult,omitempty"`

    // Phase timestamps
    PlanningStartTime    *metav1.Time `json:"planningStartTime,omitempty"`
    ValidationStartTime  *metav1.Time `json:"validationStartTime,omitempty"`
    ExecutionStartTime   *metav1.Time `json:"executionStartTime,omitempty"`
    MonitoringStartTime  *metav1.Time `json:"monitoringStartTime,omitempty"`
    CompletionTime       *metav1.Time `json:"completionTime,omitempty"`

    // Error handling
    FailureReason string `json:"failureReason,omitempty"`
    RollbackReason string `json:"rollbackReason,omitempty"`

    // ========================================
    // RECOVERY TRACKING FIELDS (Alternative 2)
    // See: docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md
    // ========================================

    // FailedStep tracks which workflow step failed (0-based index)
    // Used by RemediationOrchestrator to create recovery context
    FailedStep *int `json:"failedStep,omitempty"`

    // FailedAction tracks the action type that failed (e.g., "scale_deployment", "restart_pod")
    // Helps recovery analysis understand what was attempted
    FailedAction *string `json:"failedAction,omitempty"`

    // ErrorType classifies the error for recovery decision-making
    // Values: "timeout", "resource_conflict", "insufficient_permissions", "validation_error"
    ErrorType *string `json:"errorType,omitempty"`

    // ExecutionSnapshot captures cluster state at failure time
    // Used as fallback context if Context API unavailable during recovery
    ExecutionSnapshot *ExecutionSnapshot `json:"executionSnapshot,omitempty"`

    // Conditions for status tracking
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ExecutionSnapshot captures workflow state at failure for recovery context
type ExecutionSnapshot struct {
    CompletedSteps   []int                  `json:"completedSteps"`   // Successfully completed step indices
    CurrentStep      int                    `json:"currentStep"`      // Step that failed
    ClusterState     map[string]interface{} `json:"clusterState"`     // Relevant resource states
    ResourceSnapshot map[string]interface{} `json:"resourceSnapshot"` // Target resource state before failure
}

// ExecutionPlan generated during planning phase
type ExecutionPlan struct {
    Strategy          string `json:"strategy"` // "sequential", "parallel", "sequential-with-parallel"
    EstimatedDuration string `json:"estimatedDuration"`
    RollbackStrategy  string `json:"rollbackStrategy"`
    SafetyChecks      []SafetyCheck `json:"safetyChecks"`
}

// ValidationResults from validation phase
type ValidationResults struct {
    SafetyChecksPassed  bool                   `json:"safetyChecksPassed"`
    DryRunPerformed     bool                   `json:"dryRunPerformed"`
    DryRunResults       *DryRunResults         `json:"dryRunResults,omitempty"` // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    ApprovalReceived    bool                   `json:"approvalReceived"`
    ApprovalTimestamp   *metav1.Time           `json:"approvalTimestamp,omitempty"`
    ValidationErrors    []string               `json:"validationErrors,omitempty"`
}

// StepStatus tracks individual step execution
type StepStatus struct {
    StepNumber        int                    `json:"stepNumber"`
    Action            string                 `json:"action"`
    Status            string                 `json:"status"` // "pending", "executing", "completed", "failed", "rolled_back"
    StartTime         *metav1.Time           `json:"startTime,omitempty"`
    EndTime           *metav1.Time           `json:"endTime,omitempty"`
    Result            *StepExecutionResult   `json:"result,omitempty"` // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    ErrorMessage      string                 `json:"errorMessage,omitempty"`
    RetriesAttempted  int                    `json:"retriesAttempted,omitempty"`
    K8sExecutionRef   *corev1.ObjectReference `json:"k8sExecutionRef,omitempty"`

    // ✅ NEW: Step Validation Framework (DD-002)
    PreConditionResults  []ConditionResult  `json:"preConditionResults,omitempty"`  // BR-WF-016: Precondition evaluation results
    PostConditionResults []ConditionResult  `json:"postConditionResults,omitempty"` // BR-WF-052: Postcondition verification results
}

// ConditionResult captures the outcome of a single condition evaluation
// Used for both preconditions and postconditions
type ConditionResult struct {
    // ConditionType identifies which condition was evaluated (e.g., "deployment_exists", "desired_replicas_running")
    ConditionType string `json:"conditionType"`

    // Evaluated indicates whether the condition was actually evaluated
    // false if evaluation was skipped (e.g., due to earlier failure)
    Evaluated bool `json:"evaluated"`

    // Passed indicates whether the condition passed
    // Only meaningful if Evaluated == true
    Passed bool `json:"passed"`

    // ErrorMessage provides details when condition fails
    // Example: "Only 2 of 5 desired pods running (insufficient resources)"
    ErrorMessage string `json:"errorMessage,omitempty"`

    // EvaluationTime records when the condition was evaluated
    EvaluationTime metav1.Time `json:"evaluationTime"`
}

// ExecutionMetrics tracks workflow performance
type ExecutionMetrics struct {
    TotalDuration      string  `json:"totalDuration"`
    StepSuccessRate    float64 `json:"stepSuccessRate"`
    RollbacksPerformed int     `json:"rollbacksPerformed"`
    ResourcesAffected  int     `json:"resourcesAffected"`
}

// AdaptiveAdjustment records runtime optimization
type AdaptiveAdjustment struct {
    Timestamp   metav1.Time `json:"timestamp"`
    Adjustment  string      `json:"adjustment"` // Description of what was adjusted
    Reason      string      `json:"reason"`
}

// WorkflowResult final outcome
type WorkflowResult struct {
    Outcome             string  `json:"outcome"` // "success", "partial_success", "failed", "unknown"
    EffectivenessScore  float64 `json:"effectivenessScore"` // 0.0-1.0
    ResourceHealth      string  `json:"resourceHealth"` // "healthy", "degraded", "unhealthy"
    NewAlertsTriggered  bool    `json:"newAlertsTriggered"`
    RecommendedActions  []string `json:"recommendedActions,omitempty"` // For partial success or failure
}

// ===================================================================
// STRUCTURED TYPES - Replacing map[string]interface{} anti-patterns
// ===================================================================

// AIRecommendations contains AI-generated workflow optimization suggestions
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type AIRecommendations struct {
    // Confidence and metadata
    OverallConfidence    float64   `json:"overallConfidence"` // 0.0-1.0
    RecommendationSource string    `json:"recommendationSource"` // "holmesgpt", "history-based"
    GeneratedAt          string    `json:"generatedAt"` // ISO 8601 timestamp

    // Step-level recommendations
    StepOptimizations    []StepOptimization    `json:"stepOptimizations,omitempty"`

    // Workflow-level recommendations
    ParallelExecutionSuggestions []ParallelExecutionGroup `json:"parallelExecutionSuggestions,omitempty"`
    SafetyImprovements          []SafetyImprovement      `json:"safetyImprovements,omitempty"`

    // Historical success data
    SimilarWorkflowSuccessRate float64 `json:"similarWorkflowSuccessRate"` // 0.0-1.0
    EstimatedDuration          string  `json:"estimatedDuration"` // e.g., "5m30s"

    // Risk assessment
    RiskFactors []RiskFactor `json:"riskFactors,omitempty"`
}

type StepOptimization struct {
    StepNumber        int     `json:"stepNumber"`
    Recommendation    string  `json:"recommendation"` // Human-readable suggestion
    Confidence        float64 `json:"confidence"` // 0.0-1.0
    ImpactLevel       string  `json:"impactLevel"` // "low", "medium", "high"
    ParameterChanges  *ParameterOptimization `json:"parameterChanges,omitempty"`
}

type ParameterOptimization struct {
    SuggestedParameters map[string]string `json:"suggestedParameters"` // Key-value pairs
    Reason              string            `json:"reason"` // Why these parameters
}

type ParallelExecutionGroup struct {
    GroupName   string `json:"groupName"`
    StepNumbers []int  `json:"stepNumbers"` // Steps that can run in parallel
    Confidence  float64 `json:"confidence"` // 0.0-1.0
}

type SafetyImprovement struct {
    Description  string `json:"description"`
    StepNumber   int    `json:"stepNumber,omitempty"` // 0 = workflow-level
    SafetyCheck  string `json:"safetyCheck"` // Suggested safety check to add
    Priority     string `json:"priority"` // "low", "medium", "high"
}

type RiskFactor struct {
    Factor      string  `json:"factor"` // Description of risk
    Severity    string  `json:"severity"` // "low", "medium", "high"
    Probability float64 `json:"probability"` // 0.0-1.0
    Mitigation  string  `json:"mitigation,omitempty"` // Suggested mitigation
}

// StepParameters is a discriminated union based on Action type
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
// Only ONE of these should be populated based on the Action field
type StepParameters struct {
    // Deployment actions
    ScaleDeployment   *ScaleDeploymentParams   `json:"scaleDeployment,omitempty"`
    RestartDeployment *RestartDeploymentParams `json:"restartDeployment,omitempty"`
    UpdateImage       *UpdateImageParams       `json:"updateImage,omitempty"`

    // Pod actions
    RestartPod        *RestartPodParams        `json:"restartPod,omitempty"`
    DeletePod         *DeletePodParams         `json:"deletePod,omitempty"`

    // ConfigMap/Secret actions
    UpdateConfigMap   *UpdateConfigMapParams   `json:"updateConfigMap,omitempty"`
    UpdateSecret      *UpdateSecretParams      `json:"updateSecret,omitempty"`

    // Node actions
    CordonNode        *CordonNodeParams        `json:"cordonNode,omitempty"`
    DrainNode         *DrainNodeParams         `json:"drainNode,omitempty"`

    // Custom actions (for extensibility)
    Custom            *CustomActionParams      `json:"custom,omitempty"`
}

// Deployment action parameters
type ScaleDeploymentParams struct {
    Namespace  string `json:"namespace"`
    Deployment string `json:"deployment"`
    Replicas   int32  `json:"replicas"`
}

type RestartDeploymentParams struct {
    Namespace  string `json:"namespace"`
    Deployment string `json:"deployment"`
    GracePeriod string `json:"gracePeriod,omitempty"` // e.g., "30s"
}

type UpdateImageParams struct {
    Namespace  string `json:"namespace"`
    Deployment string `json:"deployment"`
    Container  string `json:"container"`
    NewImage   string `json:"newImage"`
}

// Pod action parameters
type RestartPodParams struct {
    Namespace  string `json:"namespace"`
    PodName    string `json:"podName,omitempty"` // Empty = all pods matching selector
    Selector   string `json:"selector,omitempty"` // e.g., "app=web"
    GracePeriod string `json:"gracePeriod,omitempty"`
}

type DeletePodParams struct {
    Namespace  string `json:"namespace"`
    PodName    string `json:"podName"`
    GracePeriod string `json:"gracePeriod,omitempty"`
}

// ConfigMap/Secret parameters
type UpdateConfigMapParams struct {
    Namespace   string            `json:"namespace"`
    Name        string            `json:"name"`
    DataUpdates map[string]string `json:"dataUpdates"` // Key-value pairs to update
}

type UpdateSecretParams struct {
    Namespace   string            `json:"namespace"`
    Name        string            `json:"name"`
    DataUpdates map[string]string `json:"dataUpdates"` // Base64 encoded values
}

// Node action parameters
type CordonNodeParams struct {
    NodeName string `json:"nodeName"`
}

type DrainNodeParams struct {
    NodeName         string `json:"nodeName"`
    GracePeriod      string `json:"gracePeriod,omitempty"`
    IgnoreDaemonSets bool   `json:"ignoreDaemonSets"`
    DeleteLocalData  bool   `json:"deleteLocalData"`
}

// Custom action for extensibility
type CustomActionParams struct {
    ActionType string            `json:"actionType"` // Custom action identifier
    Config     map[string]string `json:"config"` // String-only key-value pairs
}

// RollbackParameters is a discriminated union based on rollback action
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type RollbackParameters struct {
    // Deployment rollbacks
    RestorePreviousDeployment *RestorePreviousDeploymentParams `json:"restorePreviousDeployment,omitempty"`
    ScaleToPrevious           *ScaleToPreviousParams           `json:"scaleToPrevious,omitempty"`

    // Config rollbacks
    RestorePreviousConfig     *RestorePreviousConfigParams     `json:"restorePreviousConfig,omitempty"`

    // Node rollbacks
    UncordonNode              *UncordonNodeParams              `json:"uncordonNode,omitempty"`

    // Custom rollbacks
    Custom                    *CustomRollbackParams            `json:"custom,omitempty"`
}

type RestorePreviousDeploymentParams struct {
    Namespace  string `json:"namespace"`
    Deployment string `json:"deployment"`
    Revision   int32  `json:"revision,omitempty"` // 0 = previous revision
}

type ScaleToPreviousParams struct {
    Namespace  string `json:"namespace"`
    Deployment string `json:"deployment"`
    PreviousReplicas int32 `json:"previousReplicas"` // Captured before change
}

type RestorePreviousConfigParams struct {
    Namespace string `json:"namespace"`
    Name      string `json:"name"`
    Type      string `json:"type"` // "ConfigMap" or "Secret"
    Snapshot  string `json:"snapshot"` // Reference to saved snapshot
}

type UncordonNodeParams struct {
    NodeName string `json:"nodeName"`
}

type CustomRollbackParams struct {
    ActionType string            `json:"actionType"`
    Config     map[string]string `json:"config"`
}

// DryRunResults contains structured dry-run execution results
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type DryRunResults struct {
    OverallSuccess    bool                    `json:"overallSuccess"`
    ExecutionTime     string                  `json:"executionTime"` // Duration
    StepsSimulated    int                     `json:"stepsSimulated"`
    StepResults       []DryRunStepResult      `json:"stepResults"`
    ResourceChanges   []ResourceChange        `json:"resourceChanges,omitempty"`
    PotentialIssues   []PotentialIssue        `json:"potentialIssues,omitempty"`
}

type DryRunStepResult struct {
    StepNumber       int    `json:"stepNumber"`
    Action           string `json:"action"`
    WouldSucceed     bool   `json:"wouldSucceed"`
    SimulatedDuration string `json:"simulatedDuration"`
    ValidationErrors []string `json:"validationErrors,omitempty"`
}

type ResourceChange struct {
    ResourceType string            `json:"resourceType"` // "Deployment", "Pod", etc.
    ResourceName string            `json:"resourceName"`
    Namespace    string            `json:"namespace"`
    ChangeType   string            `json:"changeType"` // "scale", "update", "delete"
    BeforeState  map[string]string `json:"beforeState"` // String key-value pairs only
    AfterState   map[string]string `json:"afterState"`
}

type PotentialIssue struct {
    Severity    string `json:"severity"` // "low", "medium", "high"
    Description string `json:"description"`
    StepNumber  int    `json:"stepNumber,omitempty"`
    Recommendation string `json:"recommendation,omitempty"`
}

// StepExecutionResult contains structured execution results
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type StepExecutionResult struct {
    Success          bool                   `json:"success"`
    ExecutionTime    string                 `json:"executionTime"` // Duration

    // Action-specific results (discriminated union - only one populated)
    ScaleResult      *ScaleExecutionResult      `json:"scaleResult,omitempty"`
    RestartResult    *RestartExecutionResult    `json:"restartResult,omitempty"`
    UpdateResult     *UpdateExecutionResult     `json:"updateResult,omitempty"`
    CustomResult     *CustomExecutionResult     `json:"customResult,omitempty"`

    // Common result metadata
    ResourcesAffected []AffectedResource         `json:"resourcesAffected,omitempty"`
    Warnings          []string                   `json:"warnings,omitempty"`
}

type ScaleExecutionResult struct {
    PreviousReplicas int32 `json:"previousReplicas"`
    NewReplicas      int32 `json:"newReplicas"`
    ScaledNamespace  string `json:"scaledNamespace"`
    ScaledDeployment string `json:"scaledDeployment"`
}

type RestartExecutionResult struct {
    PodsRestarted    int      `json:"podsRestarted"`
    RestartedPods    []string `json:"restartedPods"` // Pod names
    Namespace        string   `json:"namespace"`
}

type UpdateExecutionResult struct {
    ResourceType   string `json:"resourceType"` // "Deployment", "ConfigMap", etc.
    ResourceName   string `json:"resourceName"`
    Namespace      string `json:"namespace"`
    UpdateType     string `json:"updateType"` // "image", "config", "spec"
    PreviousValue  string `json:"previousValue,omitempty"`
    NewValue       string `json:"newValue,omitempty"`
}

type CustomExecutionResult struct {
    ActionType string            `json:"actionType"`
    Output     map[string]string `json:"output"` // String key-value pairs only
}

type AffectedResource struct {
    ResourceType string `json:"resourceType"`
    Name         string `json:"name"`
    Namespace    string `json:"namespace"`
    ChangeType   string `json:"changeType"` // "modified", "restarted", "scaled"
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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

## Complete Example: WorkflowExecution with Dependencies

**Business Requirements**: BR-HOLMES-031, BR-HOLMES-032, BR-HOLMES-033

This example demonstrates a multi-step workflow with dependencies from AIAnalysis recommendations, showing both the AIAnalysis output and the resulting WorkflowExecution CRD.

### AIAnalysis Recommendations (Source)

```yaml
# AIAnalysis CRD status (upstream)
status:
  phase: completed
  recommendations:
  - id: "rec-001"
    action: "scale-deployment"
    targetResource:
      kind: Deployment
      name: payment-api
      namespace: production
    parameters:
      replicas: 5
    effectivenessProbability: 0.92
    riskLevel: low
    dependencies: []  # No dependencies - executes first

  - id: "rec-002"
    action: "restart-pods"
    targetResource:
      kind: Pod
      namespace: production
      labelSelector: "app=payment-api"
    parameters:
      gracePeriodSeconds: 30
    effectivenessProbability: 0.88
    riskLevel: medium
    dependencies: ["rec-001"]  # Waits for scale to complete

  - id: "rec-003"
    action: "increase-memory-limit"
    targetResource:
      kind: Deployment
      name: payment-api
      namespace: production
    parameters:
      newMemoryLimit: "2Gi"
    effectivenessProbability: 0.85
    riskLevel: low
    dependencies: ["rec-001"]  # Also waits for scale

  # rec-002 and rec-003 can execute IN PARALLEL after rec-001

  - id: "rec-004"
    action: "verify-deployment"
    targetResource:
      kind: Deployment
      name: payment-api
      namespace: production
    parameters:
      healthCheckEndpoint: "/health"
    effectivenessProbability: 0.95
    riskLevel: low
    dependencies: ["rec-002", "rec-003"]  # Waits for BOTH
```

### WorkflowExecution CRD (Generated)

```yaml
apiVersion: workflow.kubernaut.io/v1
kind: WorkflowExecution
metadata:
  name: payment-api-workflow-abc123
  namespace: production
  ownerReferences:
  - apiVersion: remediation.kubernaut.io/v1
    kind: RemediationRequest
    name: payment-api-remediation
    uid: xyz789
    controller: true
spec:
  remediationRequestRef:
    name: payment-api-remediation
    namespace: production

  workflowDefinition:
    name: "ai-generated-workflow"
    version: "v1"
    steps:
      # Step 1: No dependencies - executes first
      - stepNumber: 1
        name: "scale-deployment"
        action: "scale-deployment"
        targetCluster: "production-cluster"
        parameters:
          deployment:
            name: payment-api
            namespace: production
            replicas: 5
        criticalStep: false
        maxRetries: 3
        timeout: "5m"
        dependsOn: []  # ✅ Empty - no dependencies

      # Step 2: Depends on step 1 (rec-001 → step 1)
      - stepNumber: 2
        name: "restart-pods"
        action: "restart-pods"
        targetCluster: "production-cluster"
        parameters:
          podSelector:
            namespace: production
            labelSelector: "app=payment-api"
          gracePeriodSeconds: 30
        criticalStep: true
        maxRetries: 2
        timeout: "5m"
        dependsOn: [1]  # ✅ rec-001 mapped to step 1

      # Step 3: Also depends on step 1 (rec-001 → step 1)
      - stepNumber: 3
        name: "increase-memory-limit"
        action: "patch-resource"
        targetCluster: "production-cluster"
        parameters:
          patchResource:
            kind: Deployment
            name: payment-api
            namespace: production
            patch:
              spec:
                template:
                  spec:
                    containers:
                    - name: api
                      resources:
                        limits:
                          memory: "2Gi"
        criticalStep: false
        maxRetries: 3
        timeout: "5m"
        dependsOn: [1]  # ✅ rec-001 mapped to step 1

      # Steps 2 and 3 execute IN PARALLEL (both depend only on step 1)

      # Step 4: Depends on steps 2 AND 3 (rec-002 → step 2, rec-003 → step 3)
      - stepNumber: 4
        name: "verify-deployment"
        action: "verify-health"
        targetCluster: "production-cluster"
        parameters:
          verifyHealth:
            kind: Deployment
            name: payment-api
            namespace: production
            healthCheckEndpoint: "/health"
        criticalStep: false
        maxRetries: 5
        timeout: "2m"
        dependsOn: [2, 3]  # ✅ rec-002 → step 2, rec-003 → step 3

    aiRecommendations:
      source: "holmesgpt"
      count: 4

  executionStrategy:
    approvalRequired: false
    dryRunFirst: true
    rollbackStrategy: "automatic"
    maxRetries: 3
    safetyChecks:
    - type: "rbac-check"
      enabled: true
    - type: "resource-availability"
      enabled: true

status:
  phase: "executing"
  totalSteps: 4
  currentStep: 2

  # Execution plan determined from dependencies
  executionPlan:
    strategy: "parallel-with-dependencies"
    estimatedDuration: "15m"
    rollbackStrategy: "automatic"
    batches:
    - batchNumber: 1
      steps: [1]  # Step 1 executes alone
    - batchNumber: 2
      steps: [2, 3]  # Steps 2 and 3 execute IN PARALLEL
    - batchNumber: 3
      steps: [4]  # Step 4 executes after 2 and 3 complete

  stepStatuses:
  - stepNumber: 1
    phase: "completed"
    executionTime: "2m30s"
    kubernetesExecutionRef:
      name: payment-api-workflow-abc123-step-1

  - stepNumber: 2
    phase: "running"
    startTime: "2025-10-08T10:05:00Z"
    kubernetesExecutionRef:
      name: payment-api-workflow-abc123-step-2

  - stepNumber: 3
    phase: "running"
    startTime: "2025-10-08T10:05:00Z"
    kubernetesExecutionRef:
      name: payment-api-workflow-abc123-step-3

  - stepNumber: 4
    phase: "pending"
    # Waiting for steps 2 and 3 to complete

  message: "Executing steps 2 and 3 in parallel (batch 2 of 3)"
```

### Execution Flow (Determined by Dependencies)

```
Batch 1 (Sequential):
  Step 1: scale-deployment
    ↓ completes
Batch 2 (Parallel):
  Step 2: restart-pods      ⟋ both start simultaneously
  Step 3: increase-memory   ⟍ both depend only on step 1
    ↓ both complete
Batch 3 (Sequential):
  Step 4: verify-deployment
    ↓ waits for steps 2 AND 3
```

---

## Representative Example: scale_deployment with Precondition/Postcondition Validation

> **📋 Design Decision Status**
>
> **Current Implementation**: **DD-002 Alternative 2** (Approved Design)
> **Status**: ✅ **Framework Design Complete**
> **Confidence**: 78%
> **Design Decision**: [DD-002](../../../architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2)
> **Business Requirements**: BR-WF-016, BR-WF-052, BR-WF-053
>
> <details>
> <summary><b>Why DD-002?</b> (Click to expand)</summary>
>
> - ✅ **Prevents Cascade Failures**: Validates state before each step (20% reduction)
> - ✅ **Verifies Outcomes**: Confirms intended effect achieved (15-20% effectiveness improvement)
> - ✅ **Better Observability**: Step-level failure diagnosis with state evidence
> - ✅ **Leverages Infrastructure**: Reuses Rego policy engine (BR-REGO-001 to BR-REGO-010)
>
> **Full Analysis**: See [STEP_VALIDATION_BUSINESS_REQUIREMENTS.md](../../../requirements/STEP_VALIDATION_BUSINESS_REQUIREMENTS.md)
> </details>

This example demonstrates the per-step validation framework with a `scale_deployment` action that includes both preconditions and postconditions:

```yaml
apiVersion: workflow.kubernaut.io/v1
kind: WorkflowExecution
metadata:
  name: web-app-scale-workflow
  namespace: production
spec:
  remediationRequestRef:
    name: web-app-high-cpu-remediation
    namespace: production

  workflowDefinition:
    name: "scale-web-application"
    version: "v1"
    steps:
      - stepNumber: 1
        name: "scale-web-deployment"
        action: "scale_deployment"
        targetCluster: "production-cluster"
        parameters:
          deployment:
            name: web-app
            namespace: production
            replicas: 5
        criticalStep: true
        maxRetries: 2
        timeout: "5m"

        # ========================================
        # PRECONDITIONS (DD-002, BR-WF-016)
        # Validated BEFORE creating KubernetesExecution CRD
        # ========================================
        preConditions:
          - type: deployment_exists
            description: "Deployment must exist before scaling"
            rego: |
              package precondition
              import future.keywords.if
              allow if { input.deployment_found == true }
            required: true
            timeout: "10s"

          - type: current_replicas_match
            description: "Current replicas should match expected baseline"
            rego: |
              package precondition
              import future.keywords.if
              allow if { input.current_replicas == 3 }
            required: false  # warning only, not blocking
            timeout: "5s"

          - type: cluster_capacity_available
            description: "Cluster must have capacity for additional pods"
            rego: |
              package precondition
              import future.keywords.if
              allow if {
                input.available_cpu >= input.required_cpu_per_pod * 2
                input.available_memory >= input.required_memory_per_pod * 2
              }
            required: true
            timeout: "10s"

        # ========================================
        # POSTCONDITIONS (DD-002, BR-WF-052)
        # Verified AFTER KubernetesExecution completes
        # ========================================
        postConditions:
          - type: desired_replicas_running
            description: "All desired replicas must be running"
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

          - type: no_crashloop_pods
            description: "No pods should be in CrashLoopBackOff"
            rego: |
              package postcondition
              import future.keywords.if
              allow if {
                count([p | p := input.pods[_]; p.status == "CrashLoopBackOff"]) == 0
              }
            required: true
            timeout: "1m"

  executionStrategy:
    approvalRequired: false
    dryRunFirst: true
    rollbackStrategy: "automatic"
    maxRetries: 2

status:
  phase: "completed"
  currentStep: 1
  totalSteps: 1

  stepStatuses:
    - stepNumber: 1
      action: "scale_deployment"
      status: "completed"
      startTime: "2025-10-14T10:00:00Z"
      endTime: "2025-10-14T10:02:30Z"

      # ========================================
      # PRECONDITION EVALUATION RESULTS
      # ========================================
      preConditionResults:
        - conditionType: deployment_exists
          evaluated: true
          passed: true
          evaluationTime: "2025-10-14T10:00:01Z"

        - conditionType: current_replicas_match
          evaluated: true
          passed: false  # warning only, execution proceeded
          errorMessage: "Current replicas: 1, expected: 3 (previous step may have failed)"
          evaluationTime: "2025-10-14T10:00:02Z"

        - conditionType: cluster_capacity_available
          evaluated: true
          passed: true
          evaluationTime: "2025-10-14T10:00:03Z"

      # ========================================
      # POSTCONDITION VERIFICATION RESULTS
      # ========================================
      postConditionResults:
        - conditionType: desired_replicas_running
          evaluated: true
          passed: true
          evaluationTime: "2025-10-14T10:02:25Z"

        - conditionType: deployment_health_check
          evaluated: true
          passed: true
          evaluationTime: "2025-10-14T10:02:26Z"

        - conditionType: no_crashloop_pods
          evaluated: true
          passed: true
          evaluationTime: "2025-10-14T10:02:27Z"

      k8sExecutionRef:
        name: web-app-scale-workflow-step-1
        namespace: production

  executionMetrics:
    totalDuration: "2m30s"
    stepSuccessRate: 1.0
    rollbacksPerformed: 0
    resourcesAffected: 1

  completionTime: "2025-10-14T10:02:30Z"
```

### Validation Flow

```
1. WorkflowExecution receives step execution request
   ↓
2. Evaluate step.preConditions[] (BR-WF-016)
   - deployment_exists: ✅ PASS (blocking)
   - current_replicas_match: ❌ FAIL (warning only, log and proceed)
   - cluster_capacity_available: ✅ PASS (blocking)
   ↓ (all required preconditions passed)
3. Create KubernetesExecution CRD
   ↓
4. KubernetesExecutor executes action (kubectl scale)
   ↓
5. KubernetesExecutor reports completion
   ↓
6. WorkflowExecution evaluates step.postConditions[] (BR-WF-052)
   - desired_replicas_running: ✅ PASS (5 pods running)
   - deployment_health_check: ✅ PASS (Available=true, Progressing=true)
   - no_crashloop_pods: ✅ PASS (0 crashloop pods)
   ↓ (all postconditions passed)
7. Mark step as "completed"
```

### Condition Template Placeholder

**Note**: This representative example shows complete precondition/postcondition policies for the `scale_deployment` action. Condition templates for the **remaining 26 actions** will be defined during implementation.

See [Precondition/Postcondition Framework](../standards/precondition-postcondition-framework.md) for phased rollout strategy:
- **Phase 1** (Weeks 1-2): Top 5 actions (scale_deployment, restart_pod, increase_resources, rollback_deployment, expand_pvc)
- **Phase 2** (Weeks 3-4): Next 10 actions (infrastructure, storage, application lifecycle)
- **Phase 3** (Weeks 5-6): Remaining 12 actions (security, network, database, monitoring)

**Key Points**:
- ✅ AIAnalysis uses `recommendation.id` (string) and `dependencies []string`
- ✅ WorkflowExecution uses `step.StepNumber` (int) and `dependsOn []int`
- ✅ `buildWorkflowFromRecommendations()` maps IDs → step numbers
- ✅ WorkflowExecution identifies parallel opportunities (steps 2 and 3)
- ✅ Execution plan shows 3 batches with parallelization in batch 2

---

