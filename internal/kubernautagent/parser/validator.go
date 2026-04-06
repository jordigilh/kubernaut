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
	"fmt"
	"time"

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

// ValidationError captures a specific validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// WorkflowMeta holds catalog metadata for a workflow.
type WorkflowMeta struct {
	ExecutionEngine       string
	ExecutionBundle       string
	ExecutionBundleDigest string
	ServiceAccountName    string
}

// Validator checks InvestigationResult against session-specific constraints.
type Validator struct {
	allowedWorkflows map[string]struct{}
	catalogMeta      map[string]WorkflowMeta
}

// NewValidator creates a result validator with the given workflow allowlist.
func NewValidator(allowedWorkflows []string) *Validator {
	allowed := make(map[string]struct{}, len(allowedWorkflows))
	for _, w := range allowedWorkflows {
		allowed[w] = struct{}{}
	}
	return &Validator{
		allowedWorkflows: allowed,
		catalogMeta:      make(map[string]WorkflowMeta),
	}
}

// SetWorkflowMeta stores catalog metadata for a workflow ID (UUID).
func (v *Validator) SetWorkflowMeta(workflowID string, meta WorkflowMeta) {
	v.catalogMeta[workflowID] = meta
}

// GetWorkflowMeta returns catalog metadata for a workflow ID, if available.
func (v *Validator) GetWorkflowMeta(workflowID string) (WorkflowMeta, bool) {
	m, ok := v.catalogMeta[workflowID]
	return m, ok
}

// Validate checks the result against the allowlist and parameter bounds.
func (v *Validator) Validate(result *katypes.InvestigationResult) error {
	if result.HumanReviewNeeded {
		return nil
	}

	if result.WorkflowID != "" {
		if _, ok := v.allowedWorkflows[result.WorkflowID]; !ok {
			return &ValidationError{
				Field:   "workflow_id",
				Message: fmt.Sprintf("workflow %q not in session allowlist", result.WorkflowID),
			}
		}
	}

	if result.Confidence < 0 || result.Confidence > 1 {
		return &ValidationError{
			Field:   "confidence",
			Message: fmt.Sprintf("confidence %.2f out of [0, 1] range", result.Confidence),
		}
	}

	return nil
}

// SelfCorrect runs a validation-correction loop up to maxAttempts times.
// Returns the corrected result with ValidationAttemptsHistory populated.
// If exhausted, sets HumanReviewNeeded + HumanReviewReason and clears WorkflowID
// per DD-HAPI-002 v1.2 (invalid workflows must not propagate to execution).
//
// The loop performs exactly maxAttempts validation checks. For each failed check
// except the last, it invokes correctionFn to request a new LLM response.
// History contains exactly maxAttempts entries when exhausted.
func (v *Validator) SelfCorrect(result *katypes.InvestigationResult, maxAttempts int,
	correctionFn func(result *katypes.InvestigationResult, err error) (*katypes.InvestigationResult, error),
) (*katypes.InvestigationResult, error) {
	current := result
	var history []katypes.ValidationAttemptRecord
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		validationErr := v.Validate(current)
		if validationErr == nil {
			history = append(history, katypes.ValidationAttemptRecord{
				Attempt:    attempt + 1,
				WorkflowID: current.WorkflowID,
				IsValid:    true,
				Timestamp:  time.Now().UTC().Format(time.RFC3339),
			})
			current.ValidationAttemptsHistory = history
			return current, nil
		}

		lastErr = validationErr
		history = append(history, katypes.ValidationAttemptRecord{
			Attempt:    attempt + 1,
			WorkflowID: current.WorkflowID,
			IsValid:    false,
			Errors:     []string{validationErr.Error()},
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		})

		if attempt < maxAttempts-1 {
			corrected, corrErr := correctionFn(current, validationErr)
			if corrErr != nil {
				return nil, fmt.Errorf("correction function failed on attempt %d: %w", attempt+1, corrErr)
			}
			current = corrected
		}
	}

	current.ValidationAttemptsHistory = history
	current.HumanReviewNeeded = true
	current.HumanReviewReason = "llm_parsing_error"
	current.Reason = fmt.Sprintf("self-correction exhausted after %d attempts: %s", maxAttempts, lastErr)
	current.WorkflowID = ""
	current.ExecutionBundle = ""
	return current, nil
}
