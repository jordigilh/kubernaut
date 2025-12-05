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

// Package handlers provides phase-specific handlers for AIAnalysis reconciliation.
package handlers

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// RegoEvaluatorInterface for dependency injection in tests
type RegoEvaluatorInterface interface {
	Evaluate(ctx context.Context, input *rego.PolicyInput) (*rego.PolicyResult, error)
}

// AnalyzingHandler handles the Analyzing phase.
// BR-AI-012: Evaluate Rego approval policies
// BR-AI-014: Graceful degradation for policy failures
type AnalyzingHandler struct {
	log       logr.Logger
	evaluator RegoEvaluatorInterface
}

// NewAnalyzingHandler creates a new AnalyzingHandler.
func NewAnalyzingHandler(evaluator RegoEvaluatorInterface, log logr.Logger) *AnalyzingHandler {
	return &AnalyzingHandler{
		evaluator: evaluator,
		log:       log.WithName("analyzing-handler"),
	}
}

// Name returns the handler name.
func (h *AnalyzingHandler) Name() string {
	return "analyzing"
}

// Handle processes the Analyzing phase.
// BR-AI-012: Evaluate Rego policies to determine approval requirement.
// BR-AI-014: If evaluation fails, default to approvalRequired=true (safe default).
func (h *AnalyzingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	h.log.Info("Processing Analyzing phase", "name", analysis.Name)

	// Build policy input from analysis
	input := h.buildPolicyInput(analysis)

	// Evaluate Rego policy
	result, err := h.evaluator.Evaluate(ctx, input)
	if err != nil {
		// This shouldn't happen - evaluator should handle errors gracefully
		// But if it does, use safe defaults
		h.log.Error(err, "Rego evaluation returned error, using safe default")
		analysis.Status.Phase = aianalysis.PhaseFailed
		analysis.Status.Message = "Rego evaluation failed unexpectedly"
		analysis.Status.Reason = "RegoEvaluationError"
		return ctrl.Result{}, nil
	}

	// Store evaluation results in status
	analysis.Status.ApprovalRequired = result.ApprovalRequired
	analysis.Status.ApprovalReason = result.Reason
	analysis.Status.DegradedMode = result.Degraded

	h.log.Info("Rego evaluation complete",
		"approvalRequired", result.ApprovalRequired,
		"degraded", result.Degraded,
		"reason", result.Reason,
	)

	// Transition to Recommending phase
	analysis.Status.Phase = aianalysis.PhaseRecommending
	analysis.Status.Message = "Analysis complete, preparing recommendation"

	return ctrl.Result{Requeue: true}, nil
}

// buildPolicyInput constructs the Rego policy input from AIAnalysis.
// BR-AI-012: Build input from status fields populated by InvestigatingHandler.
func (h *AnalyzingHandler) buildPolicyInput(analysis *aianalysisv1.AIAnalysis) *rego.PolicyInput {
	input := &rego.PolicyInput{
		Environment:      analysis.Spec.AnalysisRequest.SignalContext.Environment,
		FailedDetections: []string{},
		Warnings:         analysis.Status.Warnings,
	}

	// Get confidence from SelectedWorkflow (populated by InvestigatingHandler)
	if analysis.Status.SelectedWorkflow != nil {
		input.Confidence = analysis.Status.SelectedWorkflow.Confidence
	}

	// Get TargetInOwnerChain from status (populated by InvestigatingHandler)
	if analysis.Status.TargetInOwnerChain != nil {
		input.TargetInOwnerChain = *analysis.Status.TargetInOwnerChain
	}

	// DetectedLabels would come from enrichment - for now, leave empty
	// This will be populated when we integrate with SignalProcessing enrichment
	input.DetectedLabels = make(map[string]interface{})
	input.CustomLabels = make(map[string][]string)

	return input
}

