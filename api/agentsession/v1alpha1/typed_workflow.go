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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// AgentSessionSelectedWorkflow is KA's typed, curated workflow handoff to AA.
// It intentionally contains execution metadata and values only: parameter
// schemas and schema-derived allowlists remain in the trusted workflow CRD.
type AgentSessionSelectedWorkflow struct {
	WorkflowID            string                            `json:"workflowId,omitempty"`
	WorkflowName          string                            `json:"workflowName,omitempty"`
	ActionType            string                            `json:"actionType,omitempty"`
	Version               string                            `json:"version,omitempty"`
	ExecutionBundle       string                            `json:"executionBundle,omitempty"`
	ExecutionBundleDigest string                            `json:"executionBundleDigest,omitempty"`
	ExecutionEngine       string                            `json:"executionEngine,omitempty"`
	EngineConfig          *apiextensionsv1.JSON             `json:"engineConfig,omitempty"`
	ServiceAccountName    string                            `json:"serviceAccountName,omitempty"`
	ExecutionClusterID    string                            `json:"executionClusterId,omitempty"`
	Dependencies          *sharedtypes.WorkflowDependencies `json:"dependencies,omitempty"`
	Resources             *corev1.ResourceRequirements      `json:"resources,omitempty"`
	Confidence            float64                           `json:"confidence,omitempty"`
	Parameters            sharedtypes.TypedParameterMap     `json:"parameters,omitzero"`
	ParameterEvidence     string                            `json:"parameterEvidence,omitempty"`
	Rationale             string                            `json:"rationale,omitempty"`
	RepeatJustification   *sharedtypes.RepeatJustification  `json:"repeatJustification,omitempty"`
	SelectionConfidence   *float64                          `json:"workflowSelectionConfidence,omitempty"`
	RepeatRequired        *bool                             `json:"repeatJustificationRequired,omitempty"`
	PriorWorkflowMatch    *bool                             `json:"priorWorkflowMatch,omitempty"`
	RepeatValidation      string                            `json:"repeatValidationOutcome,omitempty"`
	HandoffDigest         string                            `json:"handoffDigest,omitempty"`
	PriorRemediationUIDs  []string                          `json:"priorRemediationUIDs,omitempty"`
}
