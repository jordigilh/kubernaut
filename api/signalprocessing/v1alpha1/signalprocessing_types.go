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

// Package v1alpha1 contains API Schema definitions for the signalprocessing v1alpha1 API group
// Design Decision: DD-SIGNAL-PROCESSING-001 - CRD Naming per ADR-015
// Design Decision: DD-CONTRACT-002 - Structured types for AIAnalysis integration
// Implementation Plan: Day 2 - CRD Types aligned with IMPLEMENTATION_PLAN.md
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.status.environmentClassification.environment`
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.status.priorityAssignment.priority`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SignalProcessing is the Schema for the signalprocessings API.
// DD-SIGNAL-PROCESSING-001: Renamed from RemediationProcessing per ADR-015
type SignalProcessing struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SignalProcessingSpec   `json:"spec,omitempty"`
	Status SignalProcessingStatus `json:"status,omitempty"`
}

// SignalProcessingSpec defines the desired state of SignalProcessing.
// Implementation Plan Day 2: Aligned with IMPLEMENTATION_PLAN.md structure
// +kubebuilder:validation:XValidation:rule="self.remediationRequestRef.name != ''",message="remediationRequestRef.name is required for audit trail correlation"
type SignalProcessingSpec struct{
	// Reference to parent RemediationRequest
	RemediationRequestRef ObjectReference `json:"remediationRequestRef"`

	// Signal data (copied from RemediationRequest for processing)
	Signal SignalData `json:"signal"`

	// Configuration for processing
	EnrichmentConfig EnrichmentConfig `json:"enrichmentConfig,omitempty"`
}

// ObjectReference contains enough information to let you locate the referenced object.
type ObjectReference struct {
	// API version of the referent
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind of the referent
	Kind string `json:"kind,omitempty"`
	// Name of the referent
	Name string `json:"name"`
	// Namespace of the referent
	Namespace string `json:"namespace,omitempty"`
	// UID of the referent
	UID string `json:"uid,omitempty"`
}

// SignalData contains all signal information copied from RemediationRequest.
// This makes SignalProcessing self-contained for processing.
type SignalData struct {
	// Unique fingerprint for deduplication (SHA256 of signal key fields)
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern="^[a-f0-9]{64}$"
	Fingerprint string `json:"fingerprint"`

	// Human-readable signal name (e.g., "HighMemoryUsage", "CrashLoopBackOff")
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

	// Severity level (external/raw value from monitoring system)
	// DD-SEVERITY-001: No enum restriction - allows external severity schemes (Sev1-4, P0-P4, etc.)
	// Normalized severity is stored in Status.Severity
	Severity string `json:"severity"`

	// Signal type: "prometheus", "kubernetes-event", "aws-cloudwatch", etc.
	Type string `json:"type"`

	// Adapter that ingested the signal
	// +kubebuilder:validation:MaxLength=63
	Source string `json:"source,omitempty"`

	// Target system type
	// +kubebuilder:validation:Enum=kubernetes;aws;azure;gcp;datadog
	TargetType string `json:"targetType"`

	// Target resource identification
	TargetResource ResourceIdentifier `json:"targetResource"`

	// Signal labels extracted from provider-specific data
	Labels map[string]string `json:"labels,omitempty"`

	// Signal annotations extracted from provider-specific data
	Annotations map[string]string `json:"annotations,omitempty"`

	// When the signal first started firing
	FiringTime *metav1.Time `json:"firingTime,omitempty"`

	// When Gateway received the signal
	ReceivedTime metav1.Time `json:"receivedTime"`

	// Provider-specific fields in raw JSON format
	ProviderData []byte `json:"providerData,omitempty"`
}

// ResourceIdentifier identifies the target resource for remediation.
type ResourceIdentifier struct {
	// Resource kind (e.g., "Pod", "Deployment", "StatefulSet")
	Kind string `json:"kind"`
	// Resource name
	Name string `json:"name"`
	// Resource namespace
	Namespace string `json:"namespace"`
}

// EnrichmentConfig specifies enrichment settings.
type EnrichmentConfig struct {
	// Enable cluster state enrichment
	EnableClusterState bool `json:"enableClusterState,omitempty"`
	// Enable metrics enrichment
	EnableMetrics bool `json:"enableMetrics,omitempty"`
	// Enable historical enrichment
	EnableHistorical bool `json:"enableHistorical,omitempty"`
	// Timeout for enrichment operations
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// SignalProcessingPhase represents the current phase of SignalProcessing reconciliation.
// BR-SP-051: Phase State Machine
// BR-COMMON-001: Capitalized phase values per Kubernetes API conventions
// +kubebuilder:validation:Enum=Pending;Enriching;Classifying;Categorizing;Completed;Failed
type SignalProcessingPhase string

const (
	// PhasePending is the initial state when SignalProcessing is created.
	PhasePending SignalProcessingPhase = "Pending"
	// PhaseEnriching is when K8s context enrichment is in progress.
	PhaseEnriching SignalProcessingPhase = "Enriching"
	// PhaseClassifying is when environment/priority classification is in progress.
	PhaseClassifying SignalProcessingPhase = "Classifying"
	// PhaseCategorizing is when business categorization is in progress.
	PhaseCategorizing SignalProcessingPhase = "Categorizing"
	// PhaseCompleted is the terminal success state.
	PhaseCompleted SignalProcessingPhase = "Completed"
	// PhaseFailed is the terminal error state.
	PhaseFailed SignalProcessingPhase = "Failed"
)

// SignalProcessingStatus defines the observed state of SignalProcessing.
// Implementation Plan Day 2: Aligned with IMPLEMENTATION_PLAN.md structure
type SignalProcessingStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// Used to prevent duplicate reconciliations and ensure idempotency.
	// Per DD-CONTROLLER-001: Standard pattern for all Kubernetes controllers.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase: Pending, Enriching, Classifying, Categorizing, Completed, Failed
	Phase SignalProcessingPhase `json:"phase,omitempty"`

	// Processing timestamps
	StartTime      *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Enrichment results
	KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
	RecoveryContext   *RecoveryContext   `json:"recoveryContext,omitempty"`

	// Categorization results (DD-CATEGORIZATION-001)
	EnvironmentClassification *EnvironmentClassification `json:"environmentClassification,omitempty"`
	PriorityAssignment        *PriorityAssignment        `json:"priorityAssignment,omitempty"`
	BusinessClassification    *BusinessClassification    `json:"businessClassification,omitempty"`

	// Severity determination (DD-SEVERITY-001)
	// Normalized severity determined by Rego policy: "critical", "warning", or "info"
	// Enables downstream services (AIAnalysis, RemediationOrchestrator, Notification)
	// to interpret alert urgency without understanding external severity schemes.
	// +kubebuilder:validation:Enum=critical;warning;info
	// +optional
	Severity string `json:"severity,omitempty"`

	// PolicyHash is the SHA256 hash of the Rego policy used for severity determination
	// Provides audit trail and policy version tracking for compliance requirements
	// Expected format: 64-character hexadecimal string (SHA256 hash)
	// +optional
	PolicyHash string `json:"policyHash,omitempty"`

	// Conditions for detailed status
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Error information
	Error string `json:"error,omitempty"`

	// ConsecutiveFailures tracks the number of consecutive transient failures.
	// Used with shared backoff for exponential retry delays (DD-SHARED-001).
	// Reset to 0 on successful phase transition.
	// +optional
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`

	// LastFailureTime records when the last failure occurred.
	// Used to determine if enough time has passed for retry.
	// +optional
	LastFailureTime *metav1.Time `json:"lastFailureTime,omitempty"`
}

// KubernetesContext holds enriched Kubernetes resource information.
// BR-SP-001: K8s Context Enrichment
// DD-4: Supports degraded mode when K8s API enrichment fails
type KubernetesContext struct {
	// Namespace details (per plan specification)
	Namespace *NamespaceContext `json:"namespace,omitempty"`
	// Pod details if target is a pod
	Pod *PodDetails `json:"pod,omitempty"`
	// Deployment details if target is managed by deployment
	Deployment *DeploymentDetails `json:"deployment,omitempty"`
	// StatefulSet details if target is a statefulset
	StatefulSet *StatefulSetDetails `json:"statefulSet,omitempty"`
	// DaemonSet details if target is a daemonset
	DaemonSet *DaemonSetDetails `json:"daemonSet,omitempty"`
	// ReplicaSet details if target is a replicaset
	ReplicaSet *ReplicaSetDetails `json:"replicaSet,omitempty"`
	// Service details if target is a service
	Service *ServiceDetails `json:"service,omitempty"`
	// Node details where the pod is running
	Node *NodeDetails `json:"node,omitempty"`
	// Owner chain from target to top-level controller
	OwnerChain []OwnerChainEntry `json:"ownerChain,omitempty"`
	// Detected labels (auto-detected cluster characteristics)
	DetectedLabels *DetectedLabels `json:"detectedLabels,omitempty"`
	// Custom labels (extracted via Rego policies)
	// DD-WORKFLOW-001 v1.9: map[string][]string (subdomain → list of values)
	// Example: {"constraint": ["cost-constrained", "stateful-safe"], "team": ["name=payments"]}
	CustomLabels map[string][]string `json:"customLabels,omitempty"`
	// DegradedMode indicates context was built with partial data (target resource not found)
	// DD-4: K8s Enrichment Failure Handling
	DegradedMode bool `json:"degradedMode,omitempty"`
}

// NamespaceContext holds namespace details for classification.
// Per plan specification for Environment Classifier input.
type NamespaceContext struct {
	// Namespace name
	Name string `json:"name"`
	// Namespace labels
	Labels map[string]string `json:"labels,omitempty"`
	// Namespace annotations
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PodDetails contains pod-specific information.
type PodDetails struct {
	// Pod labels
	Labels map[string]string `json:"labels,omitempty"`
	// Pod annotations
	Annotations map[string]string `json:"annotations,omitempty"`
	// Pod phase
	Phase string `json:"phase,omitempty"`
	// Container statuses
	ContainerStatuses []ContainerStatus `json:"containerStatuses,omitempty"`
	// Node name where pod is scheduled
	NodeName string `json:"nodeName,omitempty"`
}

// ContainerStatus contains container state information.
type ContainerStatus struct {
	// Container name
	Name string `json:"name"`
	// Whether container is ready
	Ready bool `json:"ready"`
	// Restart count
	RestartCount int32 `json:"restartCount"`
	// Container state
	State string `json:"state,omitempty"`
	// Last termination reason
	LastTerminationReason string `json:"lastTerminationReason,omitempty"`
}

// DeploymentDetails contains deployment-specific information.
type DeploymentDetails struct {
	// Deployment labels
	Labels map[string]string `json:"labels,omitempty"`
	// Deployment annotations
	Annotations map[string]string `json:"annotations,omitempty"`
	// Desired replicas
	Replicas int32 `json:"replicas,omitempty"`
	// Available replicas
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`
	// Ready replicas
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// StatefulSetDetails contains statefulset-specific information.
type StatefulSetDetails struct {
	// StatefulSet labels
	Labels map[string]string `json:"labels,omitempty"`
	// StatefulSet annotations
	Annotations map[string]string `json:"annotations,omitempty"`
	// Desired replicas
	Replicas int32 `json:"replicas,omitempty"`
	// Ready replicas
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// Current replicas
	CurrentReplicas int32 `json:"currentReplicas,omitempty"`
}

// DaemonSetDetails contains daemonset-specific information.
type DaemonSetDetails struct {
	// DaemonSet labels
	Labels map[string]string `json:"labels,omitempty"`
	// DaemonSet annotations
	Annotations map[string]string `json:"annotations,omitempty"`
	// Desired number of nodes
	DesiredNumberScheduled int32 `json:"desiredNumberScheduled,omitempty"`
	// Current number scheduled
	CurrentNumberScheduled int32 `json:"currentNumberScheduled,omitempty"`
	// Number ready
	NumberReady int32 `json:"numberReady,omitempty"`
}

// ReplicaSetDetails contains replicaset-specific information.
type ReplicaSetDetails struct {
	// ReplicaSet labels
	Labels map[string]string `json:"labels,omitempty"`
	// ReplicaSet annotations
	Annotations map[string]string `json:"annotations,omitempty"`
	// Desired replicas
	Replicas int32 `json:"replicas,omitempty"`
	// Available replicas
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`
	// Ready replicas
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// ServiceDetails contains service-specific information.
type ServiceDetails struct {
	// Service labels
	Labels map[string]string `json:"labels,omitempty"`
	// Service annotations
	Annotations map[string]string `json:"annotations,omitempty"`
	// Service type (ClusterIP, NodePort, LoadBalancer, ExternalName)
	Type string `json:"type,omitempty"`
	// Cluster IP
	ClusterIP string `json:"clusterIP,omitempty"`
	// External IPs
	ExternalIPs []string `json:"externalIPs,omitempty"`
	// Ports
	Ports []ServicePort `json:"ports,omitempty"`
}

// ServicePort contains service port information.
type ServicePort struct {
	// Port name
	Name string `json:"name,omitempty"`
	// Port number
	Port int32 `json:"port"`
	// Target port
	TargetPort string `json:"targetPort,omitempty"`
	// Protocol (TCP, UDP, SCTP)
	Protocol string `json:"protocol,omitempty"`
}

// NodeDetails contains node-specific information.
type NodeDetails struct {
	// Node labels
	Labels map[string]string `json:"labels,omitempty"`
	// Node conditions
	Conditions []NodeCondition `json:"conditions,omitempty"`
	// Allocatable resources
	Allocatable map[string]string `json:"allocatable,omitempty"`
}

// NodeCondition represents a node condition.
type NodeCondition struct {
	// Condition type
	Type string `json:"type"`
	// Condition status
	Status string `json:"status"`
	// Reason for condition
	Reason string `json:"reason,omitempty"`
}

// OwnerChainEntry represents one owner in the ownership chain.
// BR-SP-100: OwnerChain Builder
// DD-WORKFLOW-001 v1.8: Namespace, Kind, Name ONLY (no APIVersion/UID)
// HolmesGPT-API uses for DetectedLabels validation
type OwnerChainEntry struct {
	// Owner namespace (empty for cluster-scoped resources like Node)
	Namespace string `json:"namespace,omitempty"`
	// Owner kind (e.g., ReplicaSet, Deployment, StatefulSet, DaemonSet)
	Kind string `json:"kind"`
	// Owner name
	Name string `json:"name"`
}

// DetectedLabels contains auto-detected cluster characteristics.
// BR-SP-101: DetectedLabels Detector
// DD-WORKFLOW-001 v2.2: 8 detection categories
type DetectedLabels struct {
	// Whether the target is in a production namespace
	IsProduction bool `json:"isProduction,omitempty"`
	// Whether the target has resource limits defined
	HasResourceLimits bool `json:"hasResourceLimits,omitempty"`
	// Whether the target is managed by Helm
	HelmManaged bool `json:"helmManaged,omitempty"`
	// Whether the target is managed by ArgoCD/Flux
	GitOpsManaged bool `json:"gitOpsManaged,omitempty"`
	// Whether the target has a PDB (PodDisruptionBudget)
	HasPDB bool `json:"hasPDB,omitempty"`
	// Whether the target has an HPA (HorizontalPodAutoscaler)
	HasHPA bool `json:"hasHPA,omitempty"`
	// Whether the namespace has network isolation
	NetworkIsolated bool `json:"networkIsolated,omitempty"`
	// Whether the namespace is part of a service mesh
	ServiceMesh bool `json:"serviceMesh,omitempty"`
}

// RecoveryContext holds context for recovery attempts.
// DD-001: Recovery Context Enrichment
type RecoveryContext struct {
	// Previous remediation attempt ID
	PreviousRemediationID string `json:"previousRemediationId,omitempty"`
	// Number of previous attempts
	AttemptCount int32 `json:"attemptCount,omitempty"`
	// Last failure reason
	LastFailureReason string `json:"lastFailureReason,omitempty"`
	// Time since first failure
	TimeSinceFirstFailure *metav1.Duration `json:"timeSinceFirstFailure,omitempty"`
}

// EnvironmentClassification from DD-CATEGORIZATION-001.
// BR-SP-051-053: Environment Classification (Updated per BR-SP-080 V2.0)
// DD-WORKFLOW-001 v2.2: 4 canonical environments (production, staging, development, test)
// DD-SP-001 V1.1: Removed Confidence field (redundant with source)
// BR-SP-080 V2.0: Removed signal-labels source (security vulnerability)
type EnvironmentClassification struct {
	// Environment: production, staging, development, test
	Environment string `json:"environment"`
	// Source of classification: namespace-labels, rego-inference, default
	// Valid sources per BR-SP-080 V2.0 (signal-labels removed for security)
	Source string `json:"source"`
	// When classification was performed
	ClassifiedAt metav1.Time `json:"classifiedAt"`
}

// PriorityAssignment from DD-CATEGORIZATION-001.
// BR-SP-070-072: Priority Assignment (Updated per BR-SP-080 V2.0)
// DD-SP-001 V1.1: Removed Confidence field (redundant with source)
type PriorityAssignment struct {
	// Priority level: P0, P1, P2, P3
	Priority string `json:"priority"`
	// Source of assignment: rego-policy, severity-fallback, default
	// Per BR-SP-071: severity-fallback used when Rego fails (severity-only fallback)
	Source string `json:"source"`
	// Which Rego rule matched (if applicable)
	PolicyName string `json:"policyName,omitempty"`
	// When assignment was performed
	AssignedAt metav1.Time `json:"assignedAt"`
}

// BusinessClassification for multi-dimensional categorization.
// BR-SP-080, BR-SP-081: Business Classification
type BusinessClassification struct {
	// Business unit assignment
	BusinessUnit string `json:"businessUnit,omitempty"`
	// Service owner
	ServiceOwner string `json:"serviceOwner,omitempty"`
	// Criticality level: critical, high, medium, low
	Criticality string `json:"criticality,omitempty"`
	// SLA requirement: platinum, gold, silver, bronze
	SLARequirement string `json:"slaRequirement,omitempty"`
	// Note: OverallConfidence field removed per DD-SP-001 V1.1
}

// +kubebuilder:object:root=true

// SignalProcessingList contains a list of SignalProcessing.
type SignalProcessingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SignalProcessing `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SignalProcessing{}, &SignalProcessingList{})
}
