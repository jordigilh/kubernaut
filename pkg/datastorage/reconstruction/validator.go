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

package reconstruction

import (
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ValidationResult contains the outcome of reconstructed RR validation.
// It includes blocking errors, non-blocking warnings, and a completeness metric.
type ValidationResult struct {
	// IsValid indicates whether the RR passes validation (no blocking errors)
	IsValid bool

	// Errors contains blocking validation failures (missing required fields)
	Errors []string

	// Warnings contains non-blocking issues (missing optional fields)
	Warnings []string

	// Completeness is a percentage (0-100) indicating how complete the reconstruction is
	// Based on the presence of both required and optional fields
	Completeness int
}

// ValidateReconstructedRR validates a reconstructed RemediationRequest for completeness and quality.
// It checks required fields, calculates completeness, and generates warnings for missing optional fields.
// TDD GREEN: Minimal implementation to pass current validator tests.
func ValidateReconstructedRR(rr *remediationv1.RemediationRequest) (*ValidationResult, error) {
	if rr == nil {
		return &ValidationResult{
			IsValid:      false,
			Errors:       []string{"RemediationRequest cannot be nil"},
			Warnings:     []string{},
			Completeness: 0,
		}, nil
	}

	result := &ValidationResult{
		IsValid:      true,
		Errors:       []string{},
		Warnings:     []string{},
		Completeness: 0,
	}

	// Track field presence for completeness calculation
	totalRequired, presentRequired := validateRequiredRRFields(rr, result)
	totalOptional, presentOptional := validateOptionalRRFields(rr, result)
	totalFields := totalRequired + totalOptional
	presentFields := presentRequired + presentOptional

	// Calculate completeness percentage
	if totalFields > 0 {
		result.Completeness = (presentFields * 100) / totalFields
	}

	return result, nil
}

// validateRequiredRRFields checks the blocking-error fields on rr (SignalName,
// SignalType), appending to result.Errors and clearing result.IsValid when
// missing. Returns (totalFields, presentFields) for the completeness
// calculation. Extracted from ValidateReconstructedRR (Wave 6 6f GREEN:
// funlen remediation) — pure code motion, no behavior change.
func validateRequiredRRFields(rr *remediationv1.RemediationRequest, result *ValidationResult) (total int, present int) {
	total++
	if rr.Spec.SignalName == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, "SignalName is required in Spec")
	} else {
		present++
	}

	total++
	if rr.Spec.SignalType == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, "SignalType is required in Spec")
	} else {
		present++
	}

	return total, present
}

// validateOptionalRRFields checks the non-blocking fields on rr, appending a
// warning to result.Warnings for each one that is missing. Returns
// (totalFields, presentFields) for the completeness calculation. Extracted
// from ValidateReconstructedRR (Wave 6 6f GREEN: funlen remediation) — pure
// code motion, no behavior change.
func validateOptionalRRFields(rr *remediationv1.RemediationRequest, result *ValidationResult) (total int, present int) {
	checks := []struct {
		missing bool
		warning string
	}{
		{len(rr.Spec.SignalLabels) == 0, "SignalLabels are missing (Gap #2) - labels provide context for signal"},
		{len(rr.Spec.SignalAnnotations) == 0, "SignalAnnotations are missing (Gap #3) - annotations provide additional metadata"},
		{len(rr.Spec.OriginalPayload) == 0, "OriginalPayload is missing (Gap #1) - full audit trail requires original data"},
		{rr.Status.TimeoutConfig == nil, "TimeoutConfig is missing (Gap #8) - operational timeouts may use defaults"},
		{len(rr.Spec.ProviderData) == 0, "providerData is missing (Gap #4) - AI analysis summary unavailable"},
		{rr.Status.SelectedWorkflowRef == nil, "selectedWorkflowRef is missing (Gap #5) - workflow selection data unavailable"},
		{rr.Status.ExecutionRef == nil, "executionRef is missing (Gap #6) - workflow execution reference unavailable"},
	}

	for _, check := range checks {
		total++
		if check.missing {
			result.Warnings = append(result.Warnings, check.warning)
		} else {
			present++
		}
	}

	return total, present
}
