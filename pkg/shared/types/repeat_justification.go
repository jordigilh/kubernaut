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

// RepeatJustification is selection reasoning, not execution configuration.
// DD-KA-2496 / BR-KA-016: each part is required for a same-ID repeat with a
// completed valid prior assessment. Acceptance is structural, never semantic.
type RepeatJustification struct {
	// EvidenceBasis describes current observations supporting the choice.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2000
	// +kubebuilder:validation:Pattern=`\S`
	EvidenceBasis string `json:"evidence_basis"`

	// PriorOutcomeRelevance explains the relevance of the previous outcome.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2000
	// +kubebuilder:validation:Pattern=`\S`
	PriorOutcomeRelevance string `json:"prior_outcome_relevance"`

	// ExpectedMechanism describes how the workflow is expected to help now.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2000
	// +kubebuilder:validation:Pattern=`\S`
	ExpectedMechanism string `json:"expected_mechanism"`
}

// RepeatJustificationAssessment retains bounded parts of rejected reasoning.
// DD-KA-2496: unlike executable RepeatJustification, missing/blank parts must
// remain representable for review; this type never establishes acceptance.
type RepeatJustificationAssessment struct {
	// +optional
	// +kubebuilder:validation:MaxLength=2000
	EvidenceBasis string `json:"evidence_basis,omitempty"`
	// +optional
	// +kubebuilder:validation:MaxLength=2000
	PriorOutcomeRelevance string `json:"prior_outcome_relevance,omitempty"`
	// +optional
	// +kubebuilder:validation:MaxLength=2000
	ExpectedMechanism string `json:"expected_mechanism,omitempty"`
}
