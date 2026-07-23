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

// WorkflowDependencies declares the Secrets/ConfigMaps a workflow's execution
// requires to exist in the target namespace (DD-WE-006). This is the
// canonical, kubebuilder-annotated type shared between the CRD-embedded
// execution snapshot on AIAnalysis.Status.SelectedWorkflow and
// WorkflowExecution.Spec.WorkflowRef (Issue #1661 Change 11, DD-WORKFLOW-018),
// mirroring the shape of RemediationWorkflowDependencies
// (api/remediationworkflow/v1alpha1) so KA's wire-format JSON
// ({"secrets":[...],"configMaps":[...]}) unmarshals directly into it.
type WorkflowDependencies struct {
	// +optional
	Secrets []WorkflowResourceDependency `json:"secrets,omitempty"`
	// +optional
	ConfigMaps []WorkflowResourceDependency `json:"configMaps,omitempty"`
}

// WorkflowResourceDependency identifies a required Kubernetes Secret or
// ConfigMap by name in the execution namespace.
type WorkflowResourceDependency struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}
