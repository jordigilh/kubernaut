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
)

// ActionTypeSpec defines the desired state of ActionType.
// BR-WORKFLOW-007: ActionType CRD lifecycle management.
type ActionTypeSpec struct {
	// Name is the PascalCase action type identifier (e.g., RestartPod, ScaleReplicas).
	// Immutable after creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Description provides structured information about the action type.
	// Only this field is mutable after creation.
	Description ActionTypeDescription `json:"description"`
}

// ActionTypeDescription provides structured information about an action type.
type ActionTypeDescription struct {
	// What describes what this action type concretely does.
	// +kubebuilder:validation:Required
	What string `json:"what"`

	// WhenToUse describes conditions under which this action type is appropriate.
	// +kubebuilder:validation:Required
	WhenToUse string `json:"whenToUse"`

	// WhenNotToUse describes specific exclusion conditions.
	// +optional
	WhenNotToUse string `json:"whenNotToUse,omitempty"`

	// Preconditions describes conditions that must be verified before use.
	// +optional
	Preconditions string `json:"preconditions,omitempty"`
}

// ActionTypeStatus defines the observed state of ActionType.
type ActionTypeStatus struct {
	// Registered indicates whether the action type has been successfully registered in the DS catalog.
	// +optional
	Registered bool `json:"registered,omitempty"`

	// RegisteredAt is the timestamp of initial registration in the catalog.
	// +optional
	RegisteredAt *metav1.Time `json:"registeredAt,omitempty"`

	// RegisteredBy is the identity of the registrant (K8s SA or user).
	// +optional
	RegisteredBy string `json:"registeredBy,omitempty"`

	// PreviouslyExisted indicates if this action type was re-enabled after being disabled.
	// +optional
	PreviouslyExisted bool `json:"previouslyExisted,omitempty"`

	// ActiveWorkflowCount is the number of active RemediationWorkflows referencing this action type.
	// Best-effort, updated asynchronously by the RW admission webhook handler.
	// +optional
	ActiveWorkflowCount int `json:"activeWorkflowCount,omitempty"`

	// CatalogStatus reflects the DS catalog state (active, disabled).
	// +optional
	CatalogStatus string `json:"catalogStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=at
// +kubebuilder:selectablefield:JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Action Type",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Workflows",type=integer,JSONPath=`.status.activeWorkflowCount`
// +kubebuilder:printcolumn:name="Registered",type=boolean,JSONPath=`.status.registered`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Description",type=string,JSONPath=`.spec.description.what`,priority=1

// ActionType is the Schema for the actiontypes API.
// BR-WORKFLOW-007: Kubernetes-native action type taxonomy definition.
type ActionType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ActionTypeSpec   `json:"spec,omitempty"`
	Status ActionTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ActionTypeList contains a list of ActionType.
type ActionTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ActionType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ActionType{}, &ActionTypeList{})
}
