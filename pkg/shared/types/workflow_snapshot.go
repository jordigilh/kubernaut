/*
Copyright 2026 Jordi Gil.

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

package types

import (
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// WorkflowSnapshot is the catalog-resolved, immutable execution snapshot for
// a selected RemediationWorkflow (Issue #1661 Change 12, DD-WORKFLOW-018).
// It is inline-embedded (via true anonymous Go struct embedding + JSON
// `,inline`) into both AIAnalysis.Status.SelectedWorkflow and
// WorkflowExecution.Spec.WorkflowRef so the two CRDs never independently
// drift on this field list again -- ActionType was "left off" WorkflowRef's
// hand-copied list once already (see git history referenced in
// workflowexecution_types.go), and WorkflowName was never wired at all
// until Change 12 closed that gap. Embedding this single type in both
// places makes that class of bug structurally impossible: a field added
// here is automatically present in both CRDs' schemas.
//
// Wire JSON keys are unchanged from the pre-Change-12 field-by-field
// duplication (workflowId, executionEngine, etc.), so this is a Go/CRD-schema
// dedup only -- not a breaking change to either CRD's on-the-wire shape.
//
// Field requiredness/enum constraints follow the *stricter* of the two
// pre-existing (and previously inconsistent) definitions, plus two fields
// (ActionType, ExecutionEngine) tightened to Required because their
// upstream sources already guarantee non-empty values in practice (see
// each field's doc comment for the specific evidence) -- e.g. WorkflowID
// is now Required on both (previously Required only on SelectedWorkflow)
// and ExecutionEngine's enum is enforced on both (previously only on
// SelectedWorkflow) -- since RemediationOrchestrator's buildWorkflowRef
// always copies WorkflowRef verbatim from an already-validated
// SelectedWorkflow, tightening WorkflowRef to match is a no-op on real data.
type WorkflowSnapshot struct {
	// WorkflowID is the catalog lookup key (DS-assigned UUID).
	// +kubebuilder:validation:Required
	WorkflowID string `json:"workflowId"`

	// WorkflowName is the human-readable workflow identifier, always equal
	// to RemediationWorkflow.metadata.name -- a Kubernetes-guaranteed
	// non-empty value on every object, making this Required safe on both
	// embedding CRDs. Surfaced for kubectl/operator readability so CRD
	// consumers aren't limited to the opaque WorkflowID UUID.
	// Catalog-authoritative: always sourced from DataStorage, never
	// LLM-suppliable (Issue #1661 Change 12).
	// +kubebuilder:validation:Required
	WorkflowName string `json:"workflowName"`

	// ActionType is the DD-WORKFLOW-016 taxonomy action type (e.g.,
	// ScaleReplicas, RestartPod), resolved from the DS catalog at selection
	// time. Required here because the upstream source of truth,
	// RemediationWorkflow.Spec.ActionType, is itself
	// +kubebuilder:validation:Required (api/remediationworkflow/v1alpha1) --
	// so it is always non-empty once a workflow has been resolved from the
	// catalog. Audit-readability only -- WorkflowID remains the
	// functional/join key for SOC2 CC8.1 reconstruction regardless
	// (IT-AW-1111-001).
	// +kubebuilder:validation:Required
	ActionType string `json:"actionType"`

	// Version is the workflow's semantic version.
	// +kubebuilder:validation:Required
	Version string `json:"version"`

	// ExecutionBundle is the OCI execution bundle reference (digest-pinned),
	// resolved from the DS workflow catalog.
	// +kubebuilder:validation:Required
	ExecutionBundle string `json:"executionBundle"`

	// ExecutionBundleDigest is retained for audit trail and reproducibility.
	// +optional
	ExecutionBundleDigest string `json:"executionBundleDigest,omitempty"`

	// ExecutionEngine specifies the backend engine for workflow execution,
	// resolved from the DS workflow catalog at selection time. Required:
	// RemediationOrchestrator's buildWorkflowRef already fails closed on an
	// empty value ("selectedWorkflow.executionEngine is required") rather
	// than silently defaulting, so this formalizes an invariant already
	// enforced at runtime -- moving the failure to admission time instead.
	// +kubebuilder:validation:Enum=tekton;job;ansible
	// +kubebuilder:validation:Required
	ExecutionEngine string `json:"executionEngine"`

	// EngineConfig holds engine-specific configuration (BR-WE-016).
	// For ansible: {"playbookPath": "...", "jobTemplateName": "...", "inventoryName": "..."}.
	// For tekton/job: nil.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	EngineConfig *apiextensionsv1.JSON `json:"engineConfig,omitempty"`

	// ServiceAccountName is the pre-existing ServiceAccount resolved from
	// the DS workflow catalog at selection time (Issue #650), used for pod
	// execution.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Dependencies declares the Secrets/ConfigMaps the workflow's schema
	// requires in the execution namespace (DD-WE-006).
	// +optional
	Dependencies *WorkflowDependencies `json:"dependencies,omitempty"`

	// Resources declares the per-workflow Job container CPU/memory
	// requests/limits (BR-WE-019 / DD-WE-008). Nil preserves BestEffort QoS.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// DeclaredParameterNames is the parameter-name allowlist WorkflowExecution
	// uses for defense-in-depth stripping of undeclared parameters (#243). Not
	// "omitempty": nil (no schema, no filtering) and a non-nil empty map
	// (schema declares zero allowed parameters, strip everything) are
	// distinct, meaningful values (IT-WE-243-002 vs IT-WE-243-003) -- Go's
	// encoding/json "omitempty" treats a zero-length map the same as nil and
	// silently drops it from the wire payload, collapsing that distinction.
	// +optional
	// +nullable
	DeclaredParameterNames map[string]bool `json:"declaredParameterNames"`
}
