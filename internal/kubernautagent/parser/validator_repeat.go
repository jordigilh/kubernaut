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

package parser

import (
	"slices"
	"strings"
	"unicode/utf8"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// RepeatSelectionContext is constructed from trusted, target-scoped history,
// never from an LLM proposal. Only completed valid assessments appear here.
// DD-KA-2496 / BR-KA-016: all candidates remain eligible and the context is
// request-local, including across parse recovery and bounded self-correction.
type RepeatSelectionContext struct {
	CompletedPriorWorkflows map[string][]string
	HandoffDigest           string
}

func (v *Validator) SetRepeatSelectionContext(scope *RepeatSelectionContext) {
	v.repeatContext = nil
	if scope == nil {
		return
	}
	copyScope := &RepeatSelectionContext{
		CompletedPriorWorkflows: make(map[string][]string, len(scope.CompletedPriorWorkflows)),
		HandoffDigest:           scope.HandoffDigest,
	}
	for id, priorIDs := range scope.CompletedPriorWorkflows {
		copyScope.CompletedPriorWorkflows[id] = slices.Clone(priorIDs)
	}
	v.repeatContext = copyScope
}

func (v *Validator) validateRepeatSelection(result *katypes.InvestigationResult) error {
	var priorIDs []string
	matched := false
	if v.repeatContext != nil {
		priorIDs, matched = v.repeatContext.CompletedPriorWorkflows[result.WorkflowID]
		result.HandoffDigest = v.repeatContext.HandoffDigest
	}
	result.PriorRemediationUIDs = slices.Clone(priorIDs)
	result.PriorWorkflowMatch = &matched
	required := matched
	result.RepeatJustificationRequired = &required
	result.RepeatValidationOutcome = "not_required"
	if !required && result.RepeatJustification == nil && !result.RepeatJustificationInvalid {
		return nil
	}

	result.RepeatValidationOutcome = "rejected"
	if result.RepeatJustificationInvalid {
		return &ValidationError{Field: "repeat_justification", Message: "must be an object with three string fields: evidence_basis, prior_outcome_relevance, expected_mechanism"}
	}
	if result.RepeatJustification == nil {
		return &ValidationError{Field: "repeat_justification", Message: "required for this same-workflow repeat; supply evidence_basis, prior_outcome_relevance and expected_mechanism, or choose a different workflow or no workflow"}
	}
	parts := []struct {
		name  string
		value string
	}{
		{"evidence_basis", result.RepeatJustification.EvidenceBasis},
		{"prior_outcome_relevance", result.RepeatJustification.PriorOutcomeRelevance},
		{"expected_mechanism", result.RepeatJustification.ExpectedMechanism},
	}
	for _, part := range parts {
		if strings.TrimSpace(part.value) == "" || !utf8.ValidString(part.value) || utf8.RuneCountInString(part.value) > 2000 {
			return &ValidationError{Field: "repeat_justification." + part.name, Message: "must be non-blank valid text of at most 2000 characters"}
		}
	}
	if required {
		result.RepeatValidationOutcome = "accepted"
	} else {
		result.RepeatValidationOutcome = "not_required"
	}
	return nil
}
