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

// Package aianalysis implements the AIAnalysis CRD controller.
// DD-CONTRACT-002: Self-contained CRD pattern with HolmesGPT-API integration.
//
// Phase Flow (per reconciliation-phases.md v2.0):
// Pending → Investigating → Analyzing → Completed/Failed
//
// Phase Handlers:
// - PendingHandler: Validates spec fields and transitions to Investigating
// - InvestigatingHandler: Calls HolmesGPT-API for investigation (captures RCA + workflow)
// - AnalyzingHandler: Evaluates Rego policies, populates ApprovalContext, transitions to Completed
//
// Note: Recommending phase removed in v1.8 - workflow data captured in Investigating phase.
package aianalysis

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// ========================================
// PHASE HANDLER INTERFACE (BR-AI-002)
// ========================================

// PhaseHandler defines the interface for phase-specific processing.
// Each phase handler implements this interface to process the AIAnalysis
// CRD during that specific phase of the reconciliation loop.
//
// Phase Flow (per reconciliation-phases.md v2.0): Pending → Investigating → Analyzing → Completed/Failed
type PhaseHandler interface {
	// Handle processes the AIAnalysis for this phase.
	// Returns:
	// - ctrl.Result: Requeue instructions
	// - error: If processing fails (will trigger retry)
	Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error)

	// Name returns the phase name for logging and metrics.
	Name() string
}

// ========================================
// PHASE CONSTANTS (BR-AI-002)
// ========================================

// Phase constants define the AIAnalysis reconciliation phases.
// Per reconciliation-phases.md v2.0: Pending → Investigating → Analyzing → Completed
// Note: Recommending phase removed in v1.8 - workflow data captured in Investigating phase.
// Note: No "Validating" or "Approving" phases - validation happens in Pending→Investigating transition.
const (
	// PhasePending is the initial phase when AIAnalysis is first created.
	// Validation occurs before transitioning to Investigating.
	PhasePending = "Pending"

	// PhaseInvestigating calls HolmesGPT-API for investigation.
	// Captures RCA, SelectedWorkflow, AlternativeWorkflows, Warnings.
	PhaseInvestigating = "Investigating"

	// PhaseAnalyzing evaluates Rego policies for approval determination.
	// Populates ApprovalContext if approval required, then transitions to Completed.
	PhaseAnalyzing = "Analyzing"

	// PhaseCompleted indicates successful completion.
	// AIAnalysis is ready for RO consumption.
	PhaseCompleted = "Completed"

	// PhaseFailed indicates a permanent failure.
	PhaseFailed = "Failed"
)

// ========================================
// HANDLER RESULT TYPES (BR-AI-021)
// ========================================

// HandlerResult provides structured result from phase handlers.
// Used to communicate phase transition or error handling needs.
type HandlerResult struct {
	// NextPhase is the phase to transition to (empty = stay in current phase)
	NextPhase string

	// Requeue indicates whether to requeue for later processing
	Requeue bool

	// RequeueAfter specifies delay before requeue (0 = immediate)
	RequeueAfter int64

	// Message provides context for status updates
	Message string

	// Reason provides structured reason code for conditions
	Reason string
}

// ========================================
// ERROR TYPES (APPENDIX_B: Error Handling)
// ========================================

// TransientError indicates a temporary failure that should be retried.
// Examples: network timeouts, 503 Service Unavailable, rate limiting
type TransientError struct {
	Err     error
	Message string
}

func (e *TransientError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *TransientError) Unwrap() error {
	return e.Err
}

// PermanentError indicates a failure that will not succeed on retry.
// Examples: 401 Unauthorized, 404 Not Found, validation failures
type PermanentError struct {
	Err     error
	Message string
	Reason  string
}

func (e *PermanentError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}

// ValidationError indicates invalid user input or configuration.
// Examples: missing required fields, invalid formats, out-of-range values
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "validation error for " + e.Field + ": " + e.Message
}

// ========================================
// ERROR CONSTRUCTOR HELPERS
// ========================================

// NewTransientError creates a transient error that should be retried.
func NewTransientError(message string, err error) *TransientError {
	return &TransientError{
		Err:     err,
		Message: message,
	}
}

// NewPermanentError creates a permanent error that should not be retried.
func NewPermanentError(message, reason string, err error) *PermanentError {
	return &PermanentError{
		Err:     err,
		Message: message,
		Reason:  reason,
	}
}

// NewValidationError creates a validation error for invalid input.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}
