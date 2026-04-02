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

// Validator checks InvestigationResult against session-specific constraints.
type Validator struct {
	allowedWorkflows map[string]struct{}
}

// NewValidator creates a result validator with the given workflow allowlist.
func NewValidator(allowedWorkflows []string) *Validator {
	allowed := make(map[string]struct{}, len(allowedWorkflows))
	for _, w := range allowedWorkflows {
		allowed[w] = struct{}{}
	}
	return &Validator{allowedWorkflows: allowed}
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
// Returns the corrected result, or sets HumanReviewNeeded if exhausted.
func (v *Validator) SelfCorrect(result *katypes.InvestigationResult, maxAttempts int,
	correctionFn func(result *katypes.InvestigationResult, err error) (*katypes.InvestigationResult, error),
) (*katypes.InvestigationResult, error) {
	current := result

	for attempt := 0; attempt < maxAttempts; attempt++ {
		validationErr := v.Validate(current)
		if validationErr == nil {
			return current, nil
		}

		corrected, corrErr := correctionFn(current, validationErr)
		if corrErr != nil {
			return nil, fmt.Errorf("correction function failed on attempt %d: %w", attempt+1, corrErr)
		}
		current = corrected
	}

	finalErr := v.Validate(current)
	if finalErr == nil {
		return current, nil
	}

	current.HumanReviewNeeded = true
	current.Reason = fmt.Sprintf("self-correction exhausted after %d attempts: %s", maxAttempts, finalErr)
	return current, nil
}
