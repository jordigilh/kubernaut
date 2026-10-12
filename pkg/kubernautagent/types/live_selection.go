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
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// LiveSelectionContext is the DD-KA-2496 allow-list for KA -> AF/A2A. It has
// no execution configuration, prior-attempt links, or frozen evidence content.
// These fields are server-owned; present_decision never accepts them from a model.
type LiveSelectionContext struct {
	SelectedWorkflow    *LiveSelectedWorkflow    `json:"selected_workflow,omitempty"`
	SelectionAssessment *LiveSelectionAssessment `json:"selection_assessment,omitempty"`
}

type LiveSelectedWorkflow struct {
	WorkflowID                  string                           `json:"workflow_id"`
	WorkflowName                string                           `json:"workflow_name"`
	WorkflowSelectionConfidence *float64                         `json:"workflow_selection_confidence,omitempty"`
	RepeatJustification         *sharedtypes.RepeatJustification `json:"repeat_justification,omitempty"`
	RepeatJustificationRequired *bool                            `json:"repeat_justification_required,omitempty"`
	PriorWorkflowMatch          *bool                            `json:"prior_workflow_match,omitempty"`
	RepeatValidationOutcome     string                           `json:"repeat_validation_outcome,omitempty"`
	HandoffDigest               string                           `json:"handoff_digest,omitempty"`
}

// LiveSelectionAssessment cannot carry an executable workflow_id. Rejected
// reasoning may be incomplete; bounded parts remain useful to the operator.
type LiveSelectionAssessment struct {
	ProposedWorkflowID          string                                     `json:"proposed_workflow_id"`
	WorkflowName                string                                     `json:"workflow_name,omitempty"`
	WorkflowSelectionConfidence *float64                                   `json:"workflow_selection_confidence,omitempty"`
	RepeatJustification         *sharedtypes.RepeatJustificationAssessment `json:"repeat_justification,omitempty"`
	RepeatJustificationRequired *bool                                      `json:"repeat_justification_required,omitempty"`
	PriorWorkflowMatch          *bool                                      `json:"prior_workflow_match,omitempty"`
	RepeatValidationOutcome     string                                     `json:"repeat_validation_outcome,omitempty"`
	HandoffDigest               string                                     `json:"handoff_digest,omitempty"`
}

// LiveSelectionFromResult projects only trusted result fields. It never uses
// Confidence as a substitute for an unknown WorkflowSelectionConfidence.
func LiveSelectionFromResult(result *InvestigationResult) LiveSelectionContext {
	if result == nil {
		return LiveSelectionContext{}
	}
	context := LiveSelectionContext{}
	if result.WorkflowID != "" && result.WorkflowName != "" {
		context.SelectedWorkflow = &LiveSelectedWorkflow{
			WorkflowID: result.WorkflowID, WorkflowName: result.WorkflowName,
			WorkflowSelectionConfidence: result.WorkflowSelectionConfidence,
			RepeatJustification:         result.RepeatJustification,
			RepeatJustificationRequired: result.RepeatJustificationRequired,
			PriorWorkflowMatch:          result.PriorWorkflowMatch,
			RepeatValidationOutcome:     result.RepeatValidationOutcome, HandoffDigest: result.HandoffDigest,
		}
	} else if result.ProposedWorkflowID != "" {
		assessment := &LiveSelectionAssessment{
			ProposedWorkflowID: result.ProposedWorkflowID, WorkflowName: result.WorkflowName,
			WorkflowSelectionConfidence: result.WorkflowSelectionConfidence,
			RepeatJustificationRequired: result.RepeatJustificationRequired,
			PriorWorkflowMatch:          result.PriorWorkflowMatch,
			RepeatValidationOutcome:     result.RepeatValidationOutcome, HandoffDigest: result.HandoffDigest,
		}
		if result.RepeatJustification != nil {
			assessment.RepeatJustification = &sharedtypes.RepeatJustificationAssessment{
				EvidenceBasis:         result.RepeatJustification.EvidenceBasis,
				PriorOutcomeRelevance: result.RepeatJustification.PriorOutcomeRelevance,
				ExpectedMechanism:     result.RepeatJustification.ExpectedMechanism,
			}
		}
		context.SelectionAssessment = assessment
	}
	return context.SafeCopy()
}

var liveReasonSanitizer = sanitization.NewSanitizer()

// ToMap returns only JSON-shaped values, safe for ADK state and A2A's gob
// fan-out. Callers must not put the DTO pointers behind an artifact interface.
func (c LiveSelectionContext) ToMap() (map[string]any, error) {
	raw, err := json.Marshal(c.SafeCopy())
	if err != nil {
		return nil, fmt.Errorf("encode live selection context: %w", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("decode live selection context: %w", err)
	}
	return data, nil
}

// SafeCopy bounds and sanitizes free text before JSON encoding and deep-copies
// optional values. Invalid text is omitted, never truncated into valid reasoning.
// This also normalizes KA responses read by AF without accepting private fields.
func (c LiveSelectionContext) SafeCopy() LiveSelectionContext {
	if selected := c.SelectedWorkflow; selected != nil {
		cloned := *selected
		cloned.WorkflowID = boundedLiveText(cloned.WorkflowID, 253)
		cloned.WorkflowName = boundedLiveText(cloned.WorkflowName, 253)
		if cloned.WorkflowID == "" || cloned.WorkflowName == "" {
			return LiveSelectionContext{}
		}
		cloned.WorkflowSelectionConfidence = copyLiveConfidence(cloned.WorkflowSelectionConfidence)
		cloned.RepeatJustificationRequired = copyLiveValue(cloned.RepeatJustificationRequired)
		cloned.PriorWorkflowMatch = copyLiveValue(cloned.PriorWorkflowMatch)
		cloned.RepeatValidationOutcome = liveValidationOutcome(cloned.RepeatValidationOutcome)
		cloned.HandoffDigest = boundedLiveText(cloned.HandoffDigest, 128)
		cloned.RepeatJustification = safeLiveJustification(cloned.RepeatJustification)
		return LiveSelectionContext{SelectedWorkflow: &cloned}
	}
	if assessment := c.SelectionAssessment; assessment != nil {
		cloned := *assessment
		cloned.ProposedWorkflowID = boundedLiveText(cloned.ProposedWorkflowID, 253)
		if cloned.ProposedWorkflowID == "" {
			return LiveSelectionContext{}
		}
		cloned.WorkflowName = boundedLiveText(cloned.WorkflowName, 253)
		cloned.WorkflowSelectionConfidence = copyLiveConfidence(cloned.WorkflowSelectionConfidence)
		cloned.RepeatJustificationRequired = copyLiveValue(cloned.RepeatJustificationRequired)
		cloned.PriorWorkflowMatch = copyLiveValue(cloned.PriorWorkflowMatch)
		cloned.RepeatValidationOutcome = liveValidationOutcome(cloned.RepeatValidationOutcome)
		cloned.HandoffDigest = boundedLiveText(cloned.HandoffDigest, 128)
		cloned.RepeatJustification = safeLiveAssessment(cloned.RepeatJustification)
		return LiveSelectionContext{SelectionAssessment: &cloned}
	}
	return LiveSelectionContext{}
}

func safeLiveJustification(justification *sharedtypes.RepeatJustification) *sharedtypes.RepeatJustification {
	if justification == nil {
		return nil
	}
	cloned := &sharedtypes.RepeatJustification{
		EvidenceBasis:         boundedLiveText(justification.EvidenceBasis, 2000),
		PriorOutcomeRelevance: boundedLiveText(justification.PriorOutcomeRelevance, 2000),
		ExpectedMechanism:     boundedLiveText(justification.ExpectedMechanism, 2000),
	}
	if cloned.EvidenceBasis == "" || cloned.PriorOutcomeRelevance == "" || cloned.ExpectedMechanism == "" {
		return nil
	}
	return cloned
}

func safeLiveAssessment(justification *sharedtypes.RepeatJustificationAssessment) *sharedtypes.RepeatJustificationAssessment {
	if justification == nil {
		return nil
	}
	return &sharedtypes.RepeatJustificationAssessment{
		EvidenceBasis:         boundedLiveText(justification.EvidenceBasis, 2000),
		PriorOutcomeRelevance: boundedLiveText(justification.PriorOutcomeRelevance, 2000),
		ExpectedMechanism:     boundedLiveText(justification.ExpectedMechanism, 2000),
	}
}

func boundedLiveText(text string, limit int) string {
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > limit || strings.TrimSpace(text) == "" {
		return ""
	}
	text = liveReasonSanitizer.Sanitize(text)
	if utf8.RuneCountInString(text) > limit {
		return ""
	}
	return text
}

func liveValidationOutcome(outcome string) string {
	switch outcome {
	case "accepted", "rejected", "not_required":
		return outcome
	default:
		return ""
	}
}

func copyLiveConfidence(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 1 {
		return nil
	}
	return copyLiveValue(value)
}

func copyLiveValue[T bool | float64](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
