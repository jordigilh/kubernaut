/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WorkflowExecutionSpec defines the desired state of WorkflowExecution.
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
	Name              string              `json:"name"`
	Version           string              `json:"version"`
	Steps             []WorkflowStep      `json:"steps"`
	Dependencies      map[string][]string `json:"dependencies,omitempty"`
	AIRecommendations *AIRecommendations  `json:"aiRecommendations,omitempty"`
}

// WorkflowStep represents a single step in the workflow
type WorkflowStep struct {
	StepNumber    int             `json:"stepNumber"`
	Name          string          `json:"name"`
	Action        string          `json:"action"` // e.g., "scale-deployment", "restart-pod"
	TargetCluster string          `json:"targetCluster"`
	Parameters    *StepParameters `json:"parameters"`
	CriticalStep  bool            `json:"criticalStep"` // Failure triggers rollback
	MaxRetries    int             `json:"maxRetries,omitempty"`
	Timeout       string          `json:"timeout,omitempty"`   // e.g., "5m"
	DependsOn     []int           `json:"dependsOn,omitempty"` // Step numbers
	RollbackSpec  *RollbackSpec   `json:"rollbackSpec,omitempty"`
}

// RollbackSpec defines how to rollback a step
type RollbackSpec struct {
	Action     string              `json:"action"` // e.g., "restore-previous-config"
	Parameters *RollbackParameters `json:"parameters,omitempty"`
	Timeout    string              `json:"timeout,omitempty"`
}

// ExecutionStrategy specifies execution behavior
type ExecutionStrategy struct {
	ApprovalRequired bool          `json:"approvalRequired"`
	DryRunFirst      bool          `json:"dryRunFirst"`
	RollbackStrategy string        `json:"rollbackStrategy"` // "automatic", "manual", "none"
	MaxRetries       int           `json:"maxRetries,omitempty"`
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
	OptimizationEnabled   bool `json:"optimizationEnabled"`
	LearningFromHistory   bool `json:"learningFromHistory"`
	DynamicStepAdjustment bool `json:"dynamicStepAdjustment"`
}

// WorkflowExecutionStatus defines the observed state
type WorkflowExecutionStatus struct {
	// Phase tracks current execution stage
	// +kubebuilder:validation:Enum=planning;validating;executing;monitoring;completed;failed;paused
	Phase string `json:"phase"` // "planning", "validating", "executing", "monitoring", "completed", "failed"

	// CurrentStep tracks progress
	// +kubebuilder:validation:Minimum=0
	CurrentStep int `json:"currentStep"`
	// +kubebuilder:validation:Minimum=0
	TotalSteps int `json:"totalSteps"`

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
	PlanningStartTime   *metav1.Time `json:"planningStartTime,omitempty"`
	ValidationStartTime *metav1.Time `json:"validationStartTime,omitempty"`
	ExecutionStartTime  *metav1.Time `json:"executionStartTime,omitempty"`
	MonitoringStartTime *metav1.Time `json:"monitoringStartTime,omitempty"`
	CompletionTime      *metav1.Time `json:"completionTime,omitempty"`

	// Error handling
	FailureReason  string `json:"failureReason,omitempty"`
	RollbackReason string `json:"rollbackReason,omitempty"`

	// Recovery tracking fields
	FailedStep        *int               `json:"failedStep,omitempty"`
	FailedAction      *string            `json:"failedAction,omitempty"`
	ErrorType         *string            `json:"errorType,omitempty"`
	ExecutionSnapshot *ExecutionSnapshot `json:"executionSnapshot,omitempty"`

	// Conditions for status tracking
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ExecutionSnapshot captures workflow state at failure for recovery context
type ExecutionSnapshot struct {
	CompletedSteps   []int             `json:"completedSteps"`   // Successfully completed step indices
	CurrentStep      int               `json:"currentStep"`      // Step that failed
	ClusterState     map[string]string `json:"clusterState"`     // Relevant resource states (string-encoded)
	ResourceSnapshot map[string]string `json:"resourceSnapshot"` // Target resource state before failure (string-encoded)
}

// ExecutionPlan generated during planning phase
type ExecutionPlan struct {
	// +kubebuilder:validation:Enum=sequential;parallel;sequential-with-parallel
	Strategy          string `json:"strategy"` // "sequential", "parallel", "sequential-with-parallel"
	EstimatedDuration string `json:"estimatedDuration"`
	RollbackStrategy  string `json:"rollbackStrategy"`
	SafetyChecks      []SafetyCheck `json:"safetyChecks"`
}

// ValidationResults from validation phase
type ValidationResults struct {
	SafetyChecksPassed bool           `json:"safetyChecksPassed"`
	DryRunPerformed    bool           `json:"dryRunPerformed"`
	DryRunResults      *DryRunResults `json:"dryRunResults,omitempty"`
	ApprovalReceived   bool           `json:"approvalReceived"`
	ApprovalTimestamp  *metav1.Time   `json:"approvalTimestamp,omitempty"`
	ValidationErrors   []string       `json:"validationErrors,omitempty"`
}

// StepStatus tracks individual step execution
type StepStatus struct {
	StepNumber int    `json:"stepNumber"`
	Action     string `json:"action"`
	// +kubebuilder:validation:Enum=pending;executing;completed;failed;rolled_back;skipped
	Status           string                  `json:"status"` // "pending", "executing", "completed", "failed", "rolled_back"
	StartTime        *metav1.Time            `json:"startTime,omitempty"`
	EndTime          *metav1.Time            `json:"endTime,omitempty"`
	Result           *StepExecutionResult    `json:"result,omitempty"`
	ErrorMessage     string                  `json:"errorMessage,omitempty"`
	// +kubebuilder:validation:Minimum=0
	RetriesAttempted int                     `json:"retriesAttempted,omitempty"`
	K8sExecutionRef  *corev1.ObjectReference `json:"k8sExecutionRef,omitempty"`
}

// ExecutionMetrics tracks workflow performance
type ExecutionMetrics struct {
	TotalDuration string `json:"totalDuration"`
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	StepSuccessRate    float64 `json:"stepSuccessRate"`
	RollbacksPerformed int     `json:"rollbacksPerformed"`
	ResourcesAffected  int     `json:"resourcesAffected"`
}

// AdaptiveAdjustment records runtime optimization
type AdaptiveAdjustment struct {
	Timestamp  metav1.Time `json:"timestamp"`
	Adjustment string      `json:"adjustment"` // Description of what was adjusted
	Reason     string      `json:"reason"`
}

// WorkflowResult final outcome
type WorkflowResult struct {
	Outcome            string   `json:"outcome"`            // "success", "partial_success", "failed", "unknown"
	EffectivenessScore float64  `json:"effectivenessScore"` // 0.0-1.0
	ResourceHealth     string   `json:"resourceHealth"`     // "healthy", "degraded", "unhealthy"
	NewAlertsTriggered bool     `json:"newAlertsTriggered"`
	RecommendedActions []string `json:"recommendedActions,omitempty"` // For partial success or failure
}

// ===================================================================
// STRUCTURED TYPES - Replacing map[string]interface{} anti-patterns
// ===================================================================

// AIRecommendations contains AI-generated workflow optimization suggestions
type AIRecommendations struct {
	// Confidence and metadata
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	OverallConfidence    float64 `json:"overallConfidence"`    // 0.0-1.0
	RecommendationSource string  `json:"recommendationSource"` // "holmesgpt", "history-based"
	GeneratedAt          string  `json:"generatedAt"`          // ISO 8601 timestamp

	// Step-level recommendations
	StepOptimizations []StepOptimization `json:"stepOptimizations,omitempty"`

	// Workflow-level recommendations
	ParallelExecutionSuggestions []ParallelExecutionGroup `json:"parallelExecutionSuggestions,omitempty"`
	SafetyImprovements           []SafetyImprovement      `json:"safetyImprovements,omitempty"`

	// Historical success data
	SimilarWorkflowSuccessRate float64 `json:"similarWorkflowSuccessRate"` // 0.0-1.0
	EstimatedDuration          string  `json:"estimatedDuration"`          // e.g., "5m30s"

	// Risk assessment
	RiskFactors []RiskFactor `json:"riskFactors,omitempty"`
}

// StepOptimization contains optimization suggestions for a step
type StepOptimization struct {
	StepNumber       int                    `json:"stepNumber"`
	Recommendation   string                 `json:"recommendation"` // Human-readable suggestion
	Confidence       float64                `json:"confidence"`     // 0.0-1.0
	ImpactLevel      string                 `json:"impactLevel"`    // "low", "medium", "high"
	ParameterChanges *ParameterOptimization `json:"parameterChanges,omitempty"`
}

// ParameterOptimization contains parameter change suggestions
type ParameterOptimization struct {
	SuggestedParameters map[string]string `json:"suggestedParameters"` // Key-value pairs
	Reason              string            `json:"reason"`              // Why these parameters
}

// ParallelExecutionGroup identifies steps that can run in parallel
type ParallelExecutionGroup struct {
	GroupName   string  `json:"groupName"`
	StepNumbers []int   `json:"stepNumbers"` // Steps that can run in parallel
	Confidence  float64 `json:"confidence"`  // 0.0-1.0
}

// SafetyImprovement suggests safety check additions
type SafetyImprovement struct {
	Description string `json:"description"`
	StepNumber  int    `json:"stepNumber,omitempty"` // 0 = workflow-level
	SafetyCheck string `json:"safetyCheck"`          // Suggested safety check to add
	Priority    string `json:"priority"`             // "low", "medium", "high"
}

// RiskFactor describes a potential risk
type RiskFactor struct {
	Factor      string  `json:"factor"`               // Description of risk
	Severity    string  `json:"severity"`             // "low", "medium", "high"
	Probability float64 `json:"probability"`          // 0.0-1.0
	Mitigation  string  `json:"mitigation,omitempty"` // Suggested mitigation
}

// StepParameters is a discriminated union based on Action type
// Only ONE of these should be populated based on the Action field
type StepParameters struct {
	// Deployment actions
	ScaleDeployment   *ScaleDeploymentParams   `json:"scaleDeployment,omitempty"`
	RestartDeployment *RestartDeploymentParams `json:"restartDeployment,omitempty"`
	UpdateImage       *UpdateImageParams       `json:"updateImage,omitempty"`

	// Pod actions
	RestartPod *RestartPodParams `json:"restartPod,omitempty"`
	DeletePod  *DeletePodParams  `json:"deletePod,omitempty"`

	// ConfigMap/Secret actions
	UpdateConfigMap *UpdateConfigMapParams `json:"updateConfigMap,omitempty"`
	UpdateSecret    *UpdateSecretParams    `json:"updateSecret,omitempty"`

	// Node actions
	CordonNode *CordonNodeParams `json:"cordonNode,omitempty"`
	DrainNode  *DrainNodeParams  `json:"drainNode,omitempty"`

	// Custom actions (for extensibility)
	Custom *CustomActionParams `json:"custom,omitempty"`
}

// Deployment action parameters
type ScaleDeploymentParams struct {
	Namespace  string `json:"namespace"`
	Deployment string `json:"deployment"`
	Replicas   int32  `json:"replicas"`
}

type RestartDeploymentParams struct {
	Namespace   string `json:"namespace"`
	Deployment  string `json:"deployment"`
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
	Namespace   string `json:"namespace"`
	PodName     string `json:"podName,omitempty"`  // Empty = all pods matching selector
	Selector    string `json:"selector,omitempty"` // e.g., "app=web"
	GracePeriod string `json:"gracePeriod,omitempty"`
}

type DeletePodParams struct {
	Namespace   string `json:"namespace"`
	PodName     string `json:"podName"`
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
	Config     map[string]string `json:"config"`     // String-only key-value pairs
}

// RollbackParameters is a discriminated union based on rollback action
type RollbackParameters struct {
	// Deployment rollbacks
	RestorePreviousDeployment *RestorePreviousDeploymentParams `json:"restorePreviousDeployment,omitempty"`
	ScaleToPrevious           *ScaleToPreviousParams           `json:"scaleToPrevious,omitempty"`

	// Config rollbacks
	RestorePreviousConfig *RestorePreviousConfigParams `json:"restorePreviousConfig,omitempty"`

	// Node rollbacks
	UncordonNode *UncordonNodeParams `json:"uncordonNode,omitempty"`

	// Custom rollbacks
	Custom *CustomRollbackParams `json:"custom,omitempty"`
}

type RestorePreviousDeploymentParams struct {
	Namespace  string `json:"namespace"`
	Deployment string `json:"deployment"`
	Revision   int32  `json:"revision,omitempty"` // 0 = previous revision
}

type ScaleToPreviousParams struct {
	Namespace        string `json:"namespace"`
	Deployment       string `json:"deployment"`
	PreviousReplicas int32  `json:"previousReplicas"` // Captured before change
}

type RestorePreviousConfigParams struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Type      string `json:"type"`     // "ConfigMap" or "Secret"
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
type DryRunResults struct {
	OverallSuccess  bool               `json:"overallSuccess"`
	ExecutionTime   string             `json:"executionTime"` // Duration
	StepsSimulated  int                `json:"stepsSimulated"`
	StepResults     []DryRunStepResult `json:"stepResults"`
	ResourceChanges []ResourceChange   `json:"resourceChanges,omitempty"`
	PotentialIssues []PotentialIssue   `json:"potentialIssues,omitempty"`
}

type DryRunStepResult struct {
	StepNumber        int      `json:"stepNumber"`
	Action            string   `json:"action"`
	WouldSucceed      bool     `json:"wouldSucceed"`
	SimulatedDuration string   `json:"simulatedDuration"`
	ValidationErrors  []string `json:"validationErrors,omitempty"`
}

type ResourceChange struct {
	ResourceType string            `json:"resourceType"` // "Deployment", "Pod", etc.
	ResourceName string            `json:"resourceName"`
	Namespace    string            `json:"namespace"`
	ChangeType   string            `json:"changeType"`  // "scale", "update", "delete"
	BeforeState  map[string]string `json:"beforeState"` // String key-value pairs only
	AfterState   map[string]string `json:"afterState"`
}

type PotentialIssue struct {
	Severity       string `json:"severity"` // "low", "medium", "high"
	Description    string `json:"description"`
	StepNumber     int    `json:"stepNumber,omitempty"`
	Recommendation string `json:"recommendation,omitempty"`
}

// StepExecutionResult contains structured execution results
type StepExecutionResult struct {
	Success       bool   `json:"success"`
	ExecutionTime string `json:"executionTime"` // Duration

	// Action-specific results (discriminated union - only one populated)
	ScaleResult   *ScaleExecutionResult   `json:"scaleResult,omitempty"`
	RestartResult *RestartExecutionResult `json:"restartResult,omitempty"`
	UpdateResult  *UpdateExecutionResult  `json:"updateResult,omitempty"`
	CustomResult  *CustomExecutionResult  `json:"customResult,omitempty"`

	// Common result metadata
	ResourcesAffected []AffectedResource `json:"resourcesAffected,omitempty"`
	Warnings          []string           `json:"warnings,omitempty"`
}

type ScaleExecutionResult struct {
	PreviousReplicas int32  `json:"previousReplicas"`
	NewReplicas      int32  `json:"newReplicas"`
	ScaledNamespace  string `json:"scaledNamespace"`
	ScaledDeployment string `json:"scaledDeployment"`
}

type RestartExecutionResult struct {
	PodsRestarted int      `json:"podsRestarted"`
	RestartedPods []string `json:"restartedPods"` // Pod names
	Namespace     string   `json:"namespace"`
}

type UpdateExecutionResult struct {
	ResourceType  string `json:"resourceType"` // "Deployment", "ConfigMap", etc.
	ResourceName  string `json:"resourceName"`
	Namespace     string `json:"namespace"`
	UpdateType    string `json:"updateType"` // "image", "config", "spec"
	PreviousValue string `json:"previousValue,omitempty"`
	NewValue      string `json:"newValue,omitempty"`
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WorkflowExecution is the Schema for the workflowexecutions API.
type WorkflowExecution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowExecutionSpec   `json:"spec,omitempty"`
	Status WorkflowExecutionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkflowExecutionList contains a list of WorkflowExecution.
type WorkflowExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkflowExecution `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkflowExecution{}, &WorkflowExecutionList{})
}
