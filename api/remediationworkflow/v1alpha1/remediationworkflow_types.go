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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// RemediationWorkflowSpec defines the desired state of RemediationWorkflow.
// Declared as a Kubernetes resource; registered via kubectl apply (BR-WORKFLOW-006).
// Workflow name is derived from the CRD's metadata.name (not duplicated in spec).
type RemediationWorkflowSpec struct {
	// Version is the semantic version (e.g., "1.0.0").
	// Pattern enforces semver-core (MAJOR.MINOR.PATCH, numeric, no leading "v")
	// with optional "-<pre-release>" and "+<build>" suffixes per semver.org §9-10.
	// Issue #1661 (CRD schema format hardening).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=50
	// +kubebuilder:validation:Pattern=`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`
	Version string `json:"version"`

	// Description is a structured description for LLM and operator consumption
	Description RemediationWorkflowDescription `json:"description"`

	// ActionType is the action type from the taxonomy (PascalCase).
	// Pattern requires an uppercase leading letter followed by alphanumerics
	// only (no underscores/hyphens/spaces), matching ActionType.spec.name's
	// own PascalCase contract so the two stay in lockstep. Issue #1661.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[A-Z][A-Za-z0-9]*$`
	ActionType string `json:"actionType"`

	// Labels contains mandatory matching/filtering criteria for discovery
	Labels RemediationWorkflowLabels `json:"labels"`

	// CustomLabels contains operator-defined key-value labels for additional filtering
	// +optional
	CustomLabels map[string]string `json:"customLabels,omitempty"`

	// DetectedLabels contains author-declared infrastructure requirements
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	DetectedLabels *apiextensionsv1.JSON `json:"detectedLabels,omitempty"`

	// Execution contains execution engine configuration
	Execution RemediationWorkflowExecution `json:"execution"`

	// Dependencies declares infrastructure resources required by the workflow
	// +optional
	Dependencies *RemediationWorkflowDependencies `json:"dependencies,omitempty"`

	// Maintainers is optional maintainer information
	// +optional
	Maintainers []RemediationWorkflowMaintainer `json:"maintainers,omitempty"`

	// Parameters defines the workflow input parameters
	// +kubebuilder:validation:MinItems=1
	Parameters []RemediationWorkflowParameter `json:"parameters"`

	// RollbackParameters defines parameters needed for rollback
	// +optional
	RollbackParameters []RemediationWorkflowParameter `json:"rollbackParameters,omitempty"`
}

// RemediationWorkflowDescription provides structured information about a workflow
type RemediationWorkflowDescription struct {
	// What describes what this workflow concretely does
	// +kubebuilder:validation:Required
	What string `json:"what"`

	// WhenToUse describes conditions under which this workflow is appropriate
	// +kubebuilder:validation:Required
	WhenToUse string `json:"whenToUse"`

	// WhenNotToUse describes specific exclusion conditions
	// +optional
	WhenNotToUse string `json:"whenNotToUse,omitempty"`

	// Preconditions describes conditions that must be verified through investigation
	// +optional
	Preconditions string `json:"preconditions,omitempty"`
}

// RemediationWorkflowMaintainer contains maintainer contact information
type RemediationWorkflowMaintainer struct {
	Name string `json:"name"`

	// Email must contain exactly one "@" with a non-empty local part, domain,
	// and TLD, and no whitespace. Intentionally permissive (not full RFC 5322)
	// since this is a contact-hint field, not an auth credential. Issue #1661.
	// +kubebuilder:validation:Pattern=`^[^\s@]+@[^\s@]+\.[^\s@]+$`
	Email string `json:"email"`
}

// RemediationWorkflowLabels contains mandatory matching/filtering criteria
type RemediationWorkflowLabels struct {
	// Severity is the severity level(s)
	// +kubebuilder:validation:MinItems=1
	Severity []string `json:"severity"`

	// Environment is the target environment(s)
	// +kubebuilder:validation:MinItems=1
	Environment []string `json:"environment"`

	// Component is the Kubernetes resource type(s)
	// +kubebuilder:validation:MinItems=1
	Component []string `json:"component"`

	// Priority is the business priority level
	// +kubebuilder:validation:Required
	Priority string `json:"priority"`

	// Cluster restricts this workflow to signals whose SP-derived
	// ClusterClassification matches one of these values.
	// Empty/omitted is valid: in non-fleet deployments (no ClusterClassification
	// produced) this dimension is never evaluated. In fleet-enabled deployments,
	// once a concrete classification is supplied by KA, workflows with no
	// `cluster` entries are excluded unless they explicitly opt in with "*"
	// (see DD-FLEET-002 Matching Semantics -- this is a query-time exclusion,
	// not schema-level validation).
	// BR-FLEET-003, Issue #1511.
	// +optional
	Cluster []string `json:"cluster,omitempty"`
}

// RemediationWorkflowExecution contains execution engine configuration
type RemediationWorkflowExecution struct {
	// Engine is the execution engine type
	// +kubebuilder:validation:Enum=tekton;job;ansible
	// +optional
	Engine string `json:"engine,omitempty"`

	// Bundle is the execution bundle or container image reference
	// +optional
	Bundle string `json:"bundle,omitempty"`

	// BundleDigest is the digest of the execution bundle
	// +optional
	BundleDigest string `json:"bundleDigest,omitempty"`

	// EngineConfig holds engine-specific configuration
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	EngineConfig *apiextensionsv1.JSON `json:"engineConfig,omitempty"`

	// ServiceAccountName is the pre-existing ServiceAccount for the execution
	// resource (Job, PipelineRun, or Ansible TokenRequest).
	// DD-WE-005 v2.0: Operators pre-create SAs with appropriate RBAC in the
	// execution namespace. If absent, K8s assigns the namespace's default SA
	// (Job/Tekton) or the Ansible executor uses the controller's in-cluster
	// credentials (#500 fallback).
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// RemediationWorkflowDependencies declares infrastructure resources
type RemediationWorkflowDependencies struct {
	// +optional
	Secrets []RemediationWorkflowResourceDependency `json:"secrets,omitempty"`
	// +optional
	ConfigMaps []RemediationWorkflowResourceDependency `json:"configMaps,omitempty"`
}

// RemediationWorkflowResourceDependency identifies a Kubernetes resource by name
type RemediationWorkflowResourceDependency struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// RemediationWorkflowParameter defines a workflow input parameter
type RemediationWorkflowParameter struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// +kubebuilder:validation:Enum=string;integer;boolean;array;float
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	// +optional
	Enum []string `json:"enum,omitempty"`
	// +optional
	Pattern string `json:"pattern,omitempty"`
	// +optional
	Minimum *float64 `json:"minimum,omitempty"`
	// +optional
	Maximum *float64 `json:"maximum,omitempty"`
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Default *apiextensionsv1.JSON `json:"default,omitempty"`
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`
}

// RemediationWorkflowStatus defines the observed state of RemediationWorkflow
type RemediationWorkflowStatus struct {
	// WorkflowID is the deterministic UUID derived from the workflow's content
	// hash (DeterministicUUID(ComputeContentHash(spec))). Computed locally by
	// AuthWebhook (#1661 Change 8a/8c) rather than assigned by Data Storage --
	// stable across the etcd-single-source-of-truth migration (DD-WORKFLOW-018).
	// +optional
	WorkflowID string `json:"workflowId,omitempty"`

	// ContentHash is the sha256 hex digest of the workflow's clean CRD content
	// (apiVersion/kind/metadata.name/spec), computed locally by AuthWebhook
	// (#1661 Change 8c). Lets DELETE/UPDATE flows and downstream consumers
	// verify content identity without any Data Storage round-trip.
	// +optional
	ContentHash string `json:"contentHash,omitempty"`

	// CatalogStatus reflects the workflow's lifecycle state. Always "Active"
	// once admitted -- there is no external catalog decision to defer to
	// (#1661 Change 8c, DD-WORKFLOW-018).
	// +optional
	// +kubebuilder:validation:Enum=Active;Invalid;Pending;Deprecated;Archived;Disabled;Superseded
	CatalogStatus sharedtypes.CatalogStatus `json:"catalogStatus,omitempty"`

	// RegisteredBy is the identity of the registrant
	// +optional
	RegisteredBy string `json:"registeredBy,omitempty"`

	// RegisteredAt is the timestamp of initial registration
	// +optional
	RegisteredAt *metav1.Time `json:"registeredAt,omitempty"`

	// PreviouslyExisted indicates if this workflow was re-registered after deletion
	// +optional
	PreviouslyExisted bool `json:"previouslyExisted,omitempty"`

	// Conditions represent the latest available observations of the workflow's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rw
// +kubebuilder:printcolumn:name="Action",type=string,JSONPath=`.spec.actionType`
// +kubebuilder:printcolumn:name="UUID",type=string,JSONPath=`.status.workflowId`
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.spec.execution.engine`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.catalogStatus`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RemediationWorkflow is the Schema for the remediationworkflows API.
// BR-WORKFLOW-006: Kubernetes-native workflow schema definition.
type RemediationWorkflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemediationWorkflowSpec   `json:"spec,omitempty"`
	Status RemediationWorkflowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemediationWorkflowList contains a list of RemediationWorkflow
type RemediationWorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemediationWorkflow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemediationWorkflow{}, &RemediationWorkflowList{})
}

// Condition type constants for RemediationWorkflow status.
const (
	// ConditionReady indicates whether the workflow is registered and active in DataStorage.
	ConditionReady = "Ready"
)

// Condition reason constants for RemediationWorkflow status.
const (
	ReasonRegistered       = "Registered"
	ReasonDependencyMissing = "DependencyMissing"
	ReasonDataStorageError = "DataStorageError"
	ReasonValidationFailed = "ValidationFailed"
)
