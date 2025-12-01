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
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// SignalProcessingSpec defines the desired state of SignalProcessing.
// Phase 1 Enhancement: Self-contained CRD pattern - contains all data from RemediationRequest
// No external CRD reads required during reconciliation (performance, reliability, isolation)
type SignalProcessingSpec struct {
	// ========================================
	// PARENT REFERENCE (Audit/Lineage Only)
	// ========================================
	// Reference to parent RemediationRequest CRD for audit trail and lineage
	// SignalProcessor does NOT read RemediationRequest - all data is self-contained
	RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

	// ========================================
	// SIGNAL IDENTIFICATION (From RemediationRequest)
	// ========================================
	// Core signal identity copied from RemediationRequest
	// Unique fingerprint for deduplication (SHA256 of signal key fields)
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern="^[a-f0-9]{64}$"
	SignalFingerprint string `json:"signalFingerprint"`

	// Human-readable signal name (e.g., "HighMemoryUsage", "CrashLoopBackOff")
	// +kubebuilder:validation:MaxLength=253
	SignalName string `json:"signalName"`

	// Severity level: "critical", "warning", "info"
	// +kubebuilder:validation:Enum=critical;warning;info
	Severity string `json:"severity"`

	// ========================================
	// SIGNAL CLASSIFICATION (From RemediationRequest)
	// ========================================
	// Environment: "prod", "staging", "dev"
	// +kubebuilder:validation:Enum=prod;staging;dev
	Environment string `json:"environment"`

	// Priority assigned by Gateway (P0=critical, P1=high, P2=normal)
	// +kubebuilder:validation:Enum=P0;P1;P2
	// +kubebuilder:validation:Pattern="^P[0-2]$"
	Priority string `json:"priority"`

	// Signal type: "prometheus", "kubernetes-event", "aws-cloudwatch", etc.
	SignalType string `json:"signalType"`

	// Adapter that ingested the signal (e.g., "prometheus-adapter", "k8s-event-adapter")
	// +kubebuilder:validation:MaxLength=63
	SignalSource string `json:"signalSource,omitempty"`

	// Target system type: "kubernetes", "aws", "azure", "gcp", "datadog"
	// +kubebuilder:validation:Enum=kubernetes;aws;azure;gcp;datadog
	TargetType string `json:"targetType"`

	// ========================================
	// SIGNAL METADATA (From RemediationRequest)
	// ========================================
	// Signal labels extracted from provider-specific data
	// For Prometheus: alert.Labels (e.g., {"alertname": "HighMemory", "namespace": "prod"})
	SignalLabels map[string]string `json:"signalLabels,omitempty"`

	// Signal annotations extracted from provider-specific data
	// For Prometheus: alert.Annotations (e.g., {"summary": "Memory usage above 90%"})
	SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`

	// ========================================
	// TARGET RESOURCE (From RemediationRequest)
	// ========================================
	// Target resource identification (extracted from providerData by RemediationOrchestrator)
	TargetResource ResourceIdentifier `json:"targetResource"`

	// ========================================
	// TIMESTAMPS (From RemediationRequest)
	// ========================================
	// When the signal first started firing (from upstream source)
	FiringTime metav1.Time `json:"firingTime,omitempty"`

	// When Gateway received the signal
	ReceivedTime metav1.Time `json:"receivedTime"`

	// ========================================
	// DEDUPLICATION (From RemediationRequest)
	// ========================================
	// Deduplication and correlation context
	// Uses shared type for API contract alignment (RO Team decision)
	Deduplication sharedtypes.DeduplicationInfo `json:"deduplication"`

	// ========================================
	// PROVIDER DATA (From RemediationRequest)
	// ========================================
	// Provider-specific fields in raw JSON format
	// Controllers parse this based on targetType/signalType if needed
	ProviderData []byte `json:"providerData,omitempty"`

	// Complete original webhook payload for debugging and audit
	OriginalPayload []byte `json:"originalPayload,omitempty"`

	// ========================================
	// STORM DETECTION (From RemediationRequest)
	// ========================================
	// True if this signal is part of a detected alert storm
	IsStorm bool `json:"isStorm,omitempty"`

	// Number of alerts in the storm
	StormAlertCount int `json:"stormAlertCount,omitempty"`

	// ========================================
	// CONFIGURATION (Processor-Specific)
	// ========================================
	// Optional enrichment configuration specific to this processing
	EnrichmentConfig *EnrichmentConfiguration `json:"enrichmentConfig,omitempty"`
}

// ResourceIdentifier identifies the target resource for remediation
type ResourceIdentifier struct {
	// Resource kind (e.g., "Pod", "Deployment", "StatefulSet")
	Kind string `json:"kind"`

	// Resource name
	Name string `json:"name"`

	// Resource namespace
	Namespace string `json:"namespace"`
}

// EnrichmentConfiguration specifies how to enrich signal context
type EnrichmentConfiguration struct {
	// Enable cluster state enrichment (Kubernetes API queries)
	EnableClusterState bool `json:"enableClusterState,omitempty"`

	// Enable metrics enrichment (Prometheus/monitoring queries)
	EnableMetrics bool `json:"enableMetrics,omitempty"`

	// Enable historical enrichment (vector DB/time-series queries)
	EnableHistorical bool `json:"enableHistorical,omitempty"`
}

// SignalProcessingStatus defines the observed state of SignalProcessing.
// DD-CONTRACT-002: Structured types replace unstructured ContextData map[string]string
type SignalProcessingStatus struct {
	// Phase tracking: "pending", "enriching", "completed", "failed"
	Phase string `json:"phase,omitempty"`

	// Structured enrichment results (DD-CONTRACT-002)
	// Replaces ContextData map[string]string anti-pattern
	EnrichmentResults EnrichmentResults `json:"enrichmentResults,omitempty"`

	// Timestamps
	StartTime   *metav1.Time `json:"startTime,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}

// EnrichmentResults contains all enrichment data from SignalProcessing
// DD-CONTRACT-002: Structured types for AIAnalysis integration
type EnrichmentResults struct {
	// Kubernetes resource context (from cluster API queries)
	KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`

	// Auto-detected cluster characteristics - NO CONFIG NEEDED
	// SignalProcessing auto-detects these from K8s resources
	// Flow: SignalProcessing → AIAnalysis → HolmesGPT-API → LLM prompt + MCP workflow filter
	DetectedLabels *DetectedLabels `json:"detectedLabels,omitempty"`

	// OwnerChain: K8s ownership traversal from signal source resource
	// DD-WORKFLOW-001 v1.7: Used by HolmesGPT-API for 100% safe DetectedLabels validation
	// SignalProcessing traverses metadata.ownerReferences to build this chain
	// Example: Pod → ReplicaSet → Deployment
	// Empty chain = orphan resource (no owners)
	// HolmesGPT-API uses this to validate DetectedLabels applicability when RCA
	// identifies a different resource than the original signal source
	OwnerChain []OwnerChainEntry `json:"ownerChain,omitempty"`

	// CustomLabels: Subdomain-based custom labels from Rego policies
	// Format: <subdomain>.kubernaut.io/<key>[:<value>] → map[subdomain][]values
	// Key = subdomain (filter dimension in Data Storage)
	// Value = list of extracted labels (boolean keys or "key=value" pairs)
	// Boolean: empty/"true" → key only; "false" → omitted
	// Key-value: other values → "key=value" string
	// Example: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
	// Reference: HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md
	CustomLabels map[string][]string `json:"customLabels,omitempty"`

	// Overall enrichment quality score (0.0-1.0)
	// 1.0 = all enrichments successful, 0.0 = all failed
	// CONSUMER: Remediation Orchestrator (RO) - NOT for LLM/HolmesGPT
	// PURPOSE: RO uses this to detect degraded mode (< 0.8) and notify operators
	EnrichmentQuality float64 `json:"enrichmentQuality,omitempty"`
}

// OwnerChainEntry represents a single entry in the K8s ownership chain
// DD-WORKFLOW-001 v1.7: SignalProcessing traverses ownerReferences to build this
// Example chain for a Pod owned by Deployment:
//
//	[0]: {Namespace: "prod", Kind: "ReplicaSet", Name: "api-7d8f9c6b5"}
//	[1]: {Namespace: "prod", Kind: "Deployment", Name: "api"}
type OwnerChainEntry struct {
	// Namespace of the owner resource (empty for cluster-scoped resources like Node)
	Namespace string `json:"namespace,omitempty"`
	// Kind of the owner resource (e.g., ReplicaSet, Deployment, StatefulSet, DaemonSet)
	Kind string `json:"kind"`
	// Name of the owner resource
	Name string `json:"name"`
}

// DetectedLabels contains auto-detected cluster characteristics
// SignalProcessing populates these automatically from K8s resources
// HolmesGPT-API uses for:
//   - Natural language in LLM prompt (context)
//   - LLM instructs model to include in MCP workflow search request (filtering)
type DetectedLabels struct {
	// ========================================
	// GITOPS MANAGEMENT
	// ========================================
	// True if namespace/deployment is managed by GitOps controller
	// Detection: ArgoCD annotations, Flux labels
	GitOpsManaged bool `json:"gitOpsManaged"`
	// GitOps tool managing this resource
	// +kubebuilder:validation:Enum=argocd;flux;""
	GitOpsTool string `json:"gitOpsTool,omitempty"`

	// ========================================
	// WORKLOAD PROTECTION
	// ========================================
	// True if PodDisruptionBudget exists for this workload
	PDBProtected bool `json:"pdbProtected"`
	// True if HorizontalPodAutoscaler targets this workload
	HPAEnabled bool `json:"hpaEnabled"`

	// ========================================
	// WORKLOAD CHARACTERISTICS
	// ========================================
	// True if StatefulSet or has PVCs attached
	Stateful bool `json:"stateful"`
	// True if managed by Helm (has helm.sh/chart label)
	HelmManaged bool `json:"helmManaged"`

	// ========================================
	// SECURITY POSTURE
	// ========================================
	// True if NetworkPolicy exists in namespace
	NetworkIsolated bool `json:"networkIsolated"`
	// Pod Security Standard level from namespace label
	// +kubebuilder:validation:Enum=privileged;baseline;restricted;""
	PodSecurityLevel string `json:"podSecurityLevel,omitempty"`
	// Service mesh if detected (from sidecar or namespace labels)
	// +kubebuilder:validation:Enum=istio;linkerd;""
	ServiceMesh string `json:"serviceMesh,omitempty"`
}

// KubernetesContext contains Kubernetes resource context (~8KB typical size)
// DD-CONTRACT-002: Type-safe structured data for HolmesGPT-API integration
type KubernetesContext struct {
	// Namespace information
	Namespace       string            `json:"namespace"`
	NamespaceLabels map[string]string `json:"namespaceLabels,omitempty"`

	// Pod context
	PodDetails *PodDetails `json:"podDetails,omitempty"`

	// Deployment/workload context
	DeploymentDetails *DeploymentDetails `json:"deploymentDetails,omitempty"`

	// Node context
	NodeDetails *NodeDetails `json:"nodeDetails,omitempty"`

	// Related resources (targeting data only)
	RelatedServices   []ServiceSummary   `json:"relatedServices,omitempty"`
	RelatedIngresses  []IngressSummary   `json:"relatedIngresses,omitempty"`
	RelatedConfigMaps []ConfigMapSummary `json:"relatedConfigMaps,omitempty"`
}

// PodDetails contains pod-level context
type PodDetails struct {
	Name              string            `json:"name"`
	Phase             string            `json:"phase"` // Running, Pending, Failed
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Containers        []ContainerStatus `json:"containers,omitempty"`
	RestartCount      int32             `json:"restartCount"`
	CreationTimestamp string            `json:"creationTimestamp"`
}

// ContainerStatus contains container-level status
type ContainerStatus struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	State        string `json:"state"` // running, waiting, terminated
}

// DeploymentDetails contains deployment-level context
type DeploymentDetails struct {
	Name              string            `json:"name"`
	Replicas          int32             `json:"replicas"`
	ReadyReplicas     int32             `json:"readyReplicas"`
	AvailableReplicas int32             `json:"availableReplicas"`
	Strategy          string            `json:"strategy"` // RollingUpdate, Recreate
	Labels            map[string]string `json:"labels,omitempty"`
}

// NodeDetails contains node-level context
type NodeDetails struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Capacity    ResourceList      `json:"capacity"`
	Allocatable ResourceList      `json:"allocatable"`
	Conditions  []NodeCondition   `json:"conditions,omitempty"`
}

// ResourceList contains resource quantities
type ResourceList struct {
	CPU    string `json:"cpu"`    // e.g., "4000m"
	Memory string `json:"memory"` // e.g., "16Gi"
}

// NodeCondition contains node condition status
type NodeCondition struct {
	Type   string `json:"type"`   // Ready, MemoryPressure, DiskPressure
	Status string `json:"status"` // True, False, Unknown
}

// ServiceSummary contains service targeting information
type ServiceSummary struct {
	Name      string        `json:"name"`
	Type      string        `json:"type"` // ClusterIP, NodePort, LoadBalancer
	ClusterIP string        `json:"clusterIP"`
	Ports     []ServicePort `json:"ports,omitempty"`
}

// ServicePort contains service port configuration
type ServicePort struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	Protocol   string `json:"protocol"` // TCP, UDP
}

// IngressSummary contains ingress targeting information
type IngressSummary struct {
	Name  string        `json:"name"`
	Hosts []string      `json:"hosts"`
	Rules []IngressRule `json:"rules,omitempty"`
}

// IngressRule contains ingress rule configuration
type IngressRule struct {
	Host string `json:"host"`
	Path string `json:"path"`
}

// ConfigMapSummary contains ConfigMap targeting information
type ConfigMapSummary struct {
	Name string   `json:"name"`
	Keys []string `json:"keys"` // ConfigMap key names (not full data)
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SignalProcessing is the Schema for the signalprocessings API.
// DD-SIGNAL-PROCESSING-001: Renamed from RemediationProcessing per ADR-015
type SignalProcessing struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SignalProcessingSpec   `json:"spec,omitempty"`
	Status SignalProcessingStatus `json:"status,omitempty"`
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
