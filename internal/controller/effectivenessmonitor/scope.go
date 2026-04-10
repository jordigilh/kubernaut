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

package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// assessmentScope classifies how deeply the reconciler should assess an EA,
// based on the workflow execution lifecycle (ADR-EM-001 §5, #573 G4).
type assessmentScope int

const (
	// scopeFull runs all configured component checks (health, hash, alert, metrics).
	scopeFull assessmentScope = iota
	// scopePartial runs only health + hash. Used when the WFE started but never
	// completed — metrics and alerts are meaningless without a completed workflow.
	scopePartial
	// scopeNoExecution skips all component checks. Used when no WFE started event
	// exists (remediation failed before execution, e.g., AA rejected).
	scopeNoExecution
)

// determineAssessmentScope queries DataStorage to classify the assessment depth.
// Returns scopeFull when DSQuerier is nil or on transient errors (graceful degradation).
func (r *Reconciler) determineAssessmentScope(ctx context.Context, correlationID string) assessmentScope {
	if r.DSQuerier == nil {
		return scopeFull
	}

	logger := log.FromContext(ctx)

	started, err := r.DSQuerier.HasWorkflowStarted(ctx, correlationID)
	if err != nil {
		logger.Error(err, "Failed to check workflow started status (non-fatal, assuming full assessment)",
			"correlationID", correlationID)
		return scopeFull
	}
	if !started {
		return scopeNoExecution
	}

	completed, err := r.DSQuerier.HasWorkflowCompleted(ctx, correlationID)
	if err != nil {
		logger.Error(err, "Failed to check workflow completed status (non-fatal, assuming full assessment)",
			"correlationID", correlationID)
		return scopeFull
	}
	if !completed {
		return scopePartial
	}

	return scopeFull
}

// validateEASpec checks for unrecoverable spec errors that should immediately fail the EA.
// Returns the validation failure reason, or empty string if the spec is valid.
func validateEASpec(ea *eav1.EffectivenessAssessment) string {
	if ea.Spec.CorrelationID == "" {
		return "correlationID is required"
	}
	return ""
}
