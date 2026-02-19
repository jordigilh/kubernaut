// Package types provides shared types used across multiple Kubernaut CRDs.
// These types ensure API contract alignment between services.
//
// ========================================
// ⭐ AUTHORITATIVE SOURCE - SINGLE SOURCE OF TRUTH
// ========================================
//
// This file is the AUTHORITATIVE SOURCE for:
//   - EnrichmentResults schema
//   - DetectedLabels schema (8 fields)
//   - OwnerChainEntry schema
//   - KubernetesContext schema
//
// All services MUST use these type definitions:
//   - SignalProcessing (populates at incident time)
//   - AIAnalysis (passes to HolmesGPT-API)
//   - HolmesGPT-API (uses for workflow filtering + LLM context)
//   - Data Storage (stores workflow metadata constraints)
//
// ADR-056: DetectedLabels and OwnerChain removed from EnrichmentResults.
// DetectedLabels are now computed by HAPI post-RCA (see PostRCAContext).
// OwnerChain is resolved by HAPI via get_resource_context tool (ADR-055).
//
// Design Decision: DD-WORKFLOW-001 v2.2, DD-CONTRACT-002
// See: docs/architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md
//
// +kubebuilder:object:generate=true
package types

// ========================================
// ENRICHMENT RESULTS (DD-CONTRACT-002)
// ========================================

// EnrichmentResults contains all enrichment data from SignalProcessing.
// DD-CONTRACT-002: Authoritative enrichment schema for all CRDs.
// Used by: SignalProcessing (output), AIAnalysis (input)
type EnrichmentResults struct {
	// Kubernetes resource context (pod status, node conditions, etc.)
	KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`

	// Custom labels from Rego policies - CUSTOMER DEFINED
	// Key = subdomain/category (e.g., "constraint", "team", "region")
	// Value = list of label values (boolean keys or "key=value" pairs)
	// Example: {"constraint": ["cost-constrained", "stateful-safe"], "team": ["name=payments"]}
	// Passed through to HolmesGPT-API for workflow filtering + LLM context
	CustomLabels map[string][]string `json:"customLabels,omitempty"`

	// Business classification from SP categorization phase
	// BR-SP-002, BR-SP-080, BR-SP-081: Business unit, criticality, SLA
	// Passed through to HolmesGPT-API for workflow filtering and Rego approval decisions
	BusinessClassification *BusinessClassification `json:"businessClassification,omitempty"`
}

// BusinessClassification contains business context derived from SP categorization.
// BR-SP-002: Business Classification
// BR-SP-080: Business Unit Detection
// BR-SP-081: SLA Requirement Mapping
type BusinessClassification struct {
	// Business unit owning the service (e.g., "payments", "platform")
	BusinessUnit string `json:"businessUnit,omitempty"`
	// Service owner team or individual
	ServiceOwner string `json:"serviceOwner,omitempty"`
	// Business criticality level: critical, high, medium, low
	Criticality string `json:"criticality,omitempty"`
	// SLA requirement tier: platinum, gold, silver, bronze
	SLARequirement string `json:"slaRequirement,omitempty"`
}

// OwnerChainEntry represents a single entry in the K8s ownership chain.
// DD-WORKFLOW-001 v1.8: SignalProcessing traverses ownerReferences to build this.
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

// ========================================
// DETECTED LABELS (DD-WORKFLOW-001 v2.2)
// ========================================

// DetectedLabels contains auto-detected cluster characteristics.
// SignalProcessing populates these automatically from K8s resources.
// Used by HolmesGPT-API for:
//   - Workflow filtering (deterministic SQL WHERE)
//   - LLM context (natural language in prompt)
//
// DETECTION FAILURE HANDLING (DD-WORKFLOW-001 v2.2):
// All fields are plain `bool` (NOT `*bool`). Uses `FailedDetections` array to track
// which fields had query failures (RBAC denied, timeout, network error).
//
// IMPORTANT DISTINCTION:
//   - Resource doesn't exist (no PDB) → false value, NOT in FailedDetections
//   - Query failed (RBAC denied) → false value, field name IN FailedDetections
//
// Consumers should check FailedDetections before trusting a false value.
type DetectedLabels struct {
	// ========================================
	// DETECTION FAILURE TRACKING (DD-WORKFLOW-001 v2.2)
	// ========================================
	// Lists field names where detection QUERY failed (RBAC, timeout, network error).
	// If a field is in this array, its value should be ignored.
	// If empty/nil, all detections succeeded.
	// Only accepts valid field names: gitOpsManaged, pdbProtected, hpaEnabled,
	// stateful, helmManaged, networkIsolated, serviceMesh
	// +kubebuilder:validation:items:Enum={gitOpsManaged,pdbProtected,hpaEnabled,stateful,helmManaged,networkIsolated,serviceMesh}
	FailedDetections []string `json:"failedDetections,omitempty"`

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
	// Service mesh if detected (from sidecar or namespace labels)
	// +kubebuilder:validation:Enum=istio;linkerd;""
	ServiceMesh string `json:"serviceMesh,omitempty"`
}

// ========================================
// KUBERNETES CONTEXT (DD-CONTRACT-002)
// ========================================

// KubernetesContext contains Kubernetes resource context (~8KB typical size).
// DD-CONTRACT-002: Type-safe structured data for HolmesGPT-API integration.
type KubernetesContext struct {
	// Namespace information
	Namespace            string            `json:"namespace"`
	NamespaceLabels      map[string]string `json:"namespaceLabels,omitempty"`
	NamespaceAnnotations map[string]string `json:"namespaceAnnotations,omitempty"`

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

// ========================================
// POD DETAILS
// ========================================

// PodDetails contains pod-level context.
type PodDetails struct {
	Name              string            `json:"name"`
	Phase             string            `json:"phase"` // Running, Pending, Failed
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Containers        []ContainerStatus `json:"containers,omitempty"`
	RestartCount      int32             `json:"restartCount"`
	CreationTimestamp string            `json:"creationTimestamp"`
}

// ContainerStatus contains container-level status.
type ContainerStatus struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	State        string `json:"state"` // running, waiting, terminated
}

// ========================================
// DEPLOYMENT DETAILS
// ========================================

// DeploymentDetails contains deployment-level context.
type DeploymentDetails struct {
	Name              string            `json:"name"`
	Replicas          int32             `json:"replicas"`
	ReadyReplicas     int32             `json:"readyReplicas"`
	AvailableReplicas int32             `json:"availableReplicas"`
	Strategy          string            `json:"strategy"` // RollingUpdate, Recreate
	Labels            map[string]string `json:"labels,omitempty"`
}

// ========================================
// NODE DETAILS
// ========================================

// NodeDetails contains node-level context.
type NodeDetails struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Capacity    ResourceList      `json:"capacity"`
	Allocatable ResourceList      `json:"allocatable"`
	Conditions  []NodeCondition   `json:"conditions,omitempty"`
}

// ResourceList contains resource quantities.
type ResourceList struct {
	CPU    string `json:"cpu"`    // e.g., "4000m"
	Memory string `json:"memory"` // e.g., "16Gi"
}

// NodeCondition contains node condition status.
type NodeCondition struct {
	Type   string `json:"type"`   // Ready, MemoryPressure, DiskPressure
	Status string `json:"status"` // True, False, Unknown
}

// ========================================
// RELATED RESOURCES
// ========================================

// ServiceSummary contains service targeting information.
type ServiceSummary struct {
	Name      string        `json:"name"`
	Type      string        `json:"type"` // ClusterIP, NodePort, LoadBalancer
	ClusterIP string        `json:"clusterIP"`
	Ports     []ServicePort `json:"ports,omitempty"`
}

// ServicePort contains service port configuration.
type ServicePort struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	Protocol   string `json:"protocol"` // TCP, UDP
}

// IngressSummary contains ingress targeting information.
type IngressSummary struct {
	Name  string        `json:"name"`
	Hosts []string      `json:"hosts"`
	Rules []IngressRule `json:"rules,omitempty"`
}

// IngressRule contains ingress rule configuration.
type IngressRule struct {
	Host string `json:"host"`
	Path string `json:"path"`
}

// ConfigMapSummary contains ConfigMap targeting information.
type ConfigMapSummary struct {
	Name string   `json:"name"`
	Keys []string `json:"keys"` // ConfigMap key names (not full data)
}
