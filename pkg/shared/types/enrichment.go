// Package types provides shared types used across multiple Kubernaut CRDs.
// These types ensure API contract alignment between services.
//
// ========================================
// AUTHORITATIVE SOURCE - SINGLE SOURCE OF TRUTH
// ========================================
//
// This file is the AUTHORITATIVE SOURCE for:
//   - EnrichmentResults schema
//   - DetectedLabels schema (8 fields)
//   - OwnerChainEntry schema
//   - KubernetesContext schema (lean, classification-focused)
//   - NamespaceContext schema
//   - WorkloadDetails schema
//   - BusinessClassification schema
//
// All services MUST use these type definitions:
//   - SignalProcessing (populates at incident time via type aliases)
//   - AIAnalysis (passes to HolmesGPT-API via type aliases)
//   - HolmesGPT-API (uses for workflow filtering + LLM context)
//   - Data Storage (stores workflow metadata constraints)
//
// Issue #113: KubernetesContext restructured to lean, classification-focused schema.
// Per-type workload fields (PodDetails, DeploymentDetails, etc.) replaced with
// generic WorkloadDetails (kind, name, labels, annotations) for Rego classification.
// Operational details (replicas, conditions, ports) removed -- LLM fetches on demand.
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
//
// Issue #113: CustomLabels removed from EnrichmentResults. CustomLabels are now
// accessed via KubernetesContext.CustomLabels (single source, no redundancy).
type EnrichmentResults struct {
	// Kubernetes resource context (classification-focused: namespace, workload labels, owner chain)
	KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`

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
// KUBERNETES CONTEXT (DD-CONTRACT-002, Issue #113)
// ========================================

// NamespaceContext holds namespace details for classification.
// Nil for cluster-scoped signals (e.g., Node signals).
type NamespaceContext struct {
	// Namespace name
	Name string `json:"name"`
	// Namespace labels (used by environment, priority, business classifiers)
	Labels map[string]string `json:"labels,omitempty"`
	// Namespace annotations (used by business classifier for kubernaut.ai/ labels)
	Annotations map[string]string `json:"annotations,omitempty"`
}

// WorkloadDetails contains generic workload context for Rego classification.
// Replaces per-type fields (PodDetails, DeploymentDetails, StatefulSetDetails, etc.)
// with a single type that works for any K8s resource kind.
// Rego policies differentiate by Kind when needed (e.g., input.workload.kind == "Node").
type WorkloadDetails struct {
	// Resource kind (e.g., "Deployment", "StatefulSet", "Node", "Pod")
	Kind string `json:"kind"`
	// Resource name
	Name string `json:"name"`
	// Resource labels (primary classification input for Rego policies)
	Labels map[string]string `json:"labels,omitempty"`
	// Resource annotations (used by business classifier for kubernaut.ai/ labels)
	Annotations map[string]string `json:"annotations,omitempty"`
}

// KubernetesContext contains lean, classification-focused Kubernetes resource context.
// Issue #113: Restructured from per-type workload fields to generic WorkloadDetails.
// Only stores data needed for Rego classification (labels, annotations, ownership).
// Operational details (replicas, conditions, ports) are fetched by the LLM on demand.
//
// DD-CONTRACT-002: Type-safe structured data shared across SP, AA, and HAPI.
type KubernetesContext struct {
	// Namespace context (nil for cluster-scoped resources like Node)
	Namespace *NamespaceContext `json:"namespace,omitempty"`
	// Target workload context (kind, name, labels, annotations)
	Workload *WorkloadDetails `json:"workload,omitempty"`
	// Owner chain from target to top-level controller
	// DD-WORKFLOW-001 v1.8: Used for historical remediation context
	OwnerChain []OwnerChainEntry `json:"ownerChain,omitempty"`
	// Custom labels extracted via Rego policies (BR-SP-102)
	// DD-WORKFLOW-001 v1.9: map[string][]string (subdomain -> list of values)
	CustomLabels map[string][]string `json:"customLabels,omitempty"`
	// DegradedMode indicates context was built with partial data
	// DD-4: K8s Enrichment Failure Handling (target resource not found)
	DegradedMode bool `json:"degradedMode,omitempty"`
}

// ========================================
// STANDALONE TYPES (kept for external consumers)
// ========================================
// These types are no longer part of KubernetesContext (Issue #113) but are
// retained as standalone definitions for any code that needs rich workload details
// (e.g., HAPI investigation context built during RCA).

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

// DeploymentDetails contains deployment-level context.
type DeploymentDetails struct {
	Name              string            `json:"name"`
	Replicas          int32             `json:"replicas"`
	ReadyReplicas     int32             `json:"readyReplicas"`
	AvailableReplicas int32             `json:"availableReplicas"`
	Strategy          string            `json:"strategy"` // RollingUpdate, Recreate
	Labels            map[string]string `json:"labels,omitempty"`
}

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
