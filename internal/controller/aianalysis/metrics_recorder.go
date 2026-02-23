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

package aianalysis

import (
	"context"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// ========================================
// Metrics Recorder
// Pattern: P2 - Controller Decomposition
// ========================================
//
// This module handles metrics recording for reconciliation outcomes.
//
// Reference: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md

// recordPhaseMetrics records metrics for a phase reconciliation.
//
// This method:
// - Records reconciliation success/error metrics
// - Records failure reasons for failed analyses
// - Records confidence scores for completed analyses
// - Records audit events for terminal states
//
// Business Requirements:
//   - BR-AI-017: Track phase timing and outcomes
//   - BR-AI-050: Audit terminal states
//   - DD-METRICS-001: Controller metrics wiring pattern
func (r *AIAnalysisReconciler) recordPhaseMetrics(ctx context.Context, phase string, analysis *aianalysisv1.AIAnalysis, err error) {
	result := "success"
	if err != nil {
		result = "error"
	}
	r.Metrics.RecordReconciliation(phase, result)

	// Track failures with reason and sub-reason
	if analysis.Status.Phase == PhaseFailed {
		reason := analysis.Status.Reason
		if reason == "" {
			reason = "Unknown"
		}
		subReason := analysis.Status.SubReason
		if subReason == "" {
			subReason = "Unknown"
		}
		r.Metrics.RecordFailure(reason, subReason)

		// DD-AUDIT-003: Record error audit event
		// BR-AI-090: AuditClient is P0, guaranteed non-nil (controller exits if init fails)
		if err != nil {
			r.AuditClient.RecordError(ctx, analysis, phase, err)
		}
	}

	// Track confidence scores for successful analyses
	if analysis.Status.Phase == PhaseCompleted && analysis.Status.SelectedWorkflow != nil {
		signalType := analysis.Spec.AnalysisRequest.SignalContext.SignalName
		confidence := analysis.Status.SelectedWorkflow.Confidence
		r.Metrics.RecordConfidenceScore(signalType, confidence)
	}

	// AA-BUG-006: RecordAnalysisComplete moved to handlers (analyzing.go:221, investigating.go error paths)
	// This prevents duplicate analysis.completed events when terminal states reconcile multiple times
	// Recording happens on TRANSITION to terminal state, not on EVERY reconcile of terminal state
}


