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
	totalFields := 0
	presentFields := 0

	// Validate required fields
	totalFields++
	if rr.Spec.SignalName == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, "SignalName is required in Spec")
	} else {
		presentFields++
	}

	totalFields++
	if rr.Spec.SignalType == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, "SignalType is required in Spec")
	} else {
		presentFields++
	}

	// Check optional fields and generate warnings
	totalFields++
	if len(rr.Spec.SignalLabels) == 0 {
		result.Warnings = append(result.Warnings, "SignalLabels are missing (Gap #2) - labels provide context for signal")
	} else {
		presentFields++
	}

	totalFields++
	if len(rr.Spec.SignalAnnotations) == 0 {
		result.Warnings = append(result.Warnings, "SignalAnnotations are missing (Gap #3) - annotations provide additional metadata")
	} else {
		presentFields++
	}

	totalFields++
	if len(rr.Spec.OriginalPayload) == 0 {
		result.Warnings = append(result.Warnings, "OriginalPayload is missing (Gap #1) - full audit trail requires original data")
	} else {
		presentFields++
	}

	totalFields++
	if rr.Status.TimeoutConfig == nil {
		result.Warnings = append(result.Warnings, "TimeoutConfig is missing (Gap #8) - operational timeouts may use defaults")
	} else {
		presentFields++
	}

	// Calculate completeness percentage
	if totalFields > 0 {
		result.Completeness = (presentFields * 100) / totalFields
	}

	return result, nil
}
