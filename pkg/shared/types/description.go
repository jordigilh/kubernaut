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

package types

// StructuredDescription provides a structured format for workflow and action type
// descriptions, consumed by the LLM and operators.
//
// BR-WORKFLOW-004: camelCase field names for both YAML and JSONB storage.
// DD-WORKFLOW-016: Same format shared between RemediationWorkflow and ActionType.
//
// This is the canonical type. CRD types (api/) keep kubebuilder-annotated copies
// and convert via DescriptionFromCRD / DescriptionToCRD helpers on their packages.
// The ogen-generated wire types use snake_case on the REST boundary and convert
// via the ogenconv subpackage (pkg/shared/types/ogenconv).
type StructuredDescription struct {
	What          string `json:"what" yaml:"what"`
	WhenToUse     string `json:"whenToUse" yaml:"whenToUse"`
	WhenNotToUse  string `json:"whenNotToUse,omitempty" yaml:"whenNotToUse,omitempty"`
	Preconditions string `json:"preconditions,omitempty" yaml:"preconditions,omitempty"`
}

// String returns the What field as a human-readable summary.
func (d StructuredDescription) String() string {
	return d.What
}
