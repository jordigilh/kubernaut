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

package handlers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	ctrl "sigs.k8s.io/controller-runtime"
)

// AnalyzingHandler handles the Analyzing phase.
// BR-AI-012: Analyzing phase handling
// BR-AI-013: Approval determination
// BR-AI-014: Graceful degradation
type AnalyzingHandler struct {
	log       logr.Logger
	evaluator rego.EvaluatorInterface
}

// AnalyzingHandlerOption is a functional option for AnalyzingHandler.
type AnalyzingHandlerOption func(*AnalyzingHandler)

// WithAnalyzingLogger sets the logger for AnalyzingHandler.
func WithAnalyzingLogger(log logr.Logger) AnalyzingHandlerOption {
	return func(h *AnalyzingHandler) {
		h.log = log
	}
}

// WithRegoEvaluator sets the Rego evaluator for AnalyzingHandler.
func WithRegoEvaluator(evaluator rego.EvaluatorInterface) AnalyzingHandlerOption {
	return func(h *AnalyzingHandler) {
		h.evaluator = evaluator
	}
}

// NewAnalyzingHandler creates a new AnalyzingHandler.
func NewAnalyzingHandler(opts ...AnalyzingHandlerOption) *AnalyzingHandler {
	h := &AnalyzingHandler{}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Phase returns the phase this handler processes.
func (h *AnalyzingHandler) Phase() string {
	return string(aianalysis.PhaseAnalyzing)
}

// Handle evaluates Rego policies and determines approval requirement.
// BR-AI-012: Analyzing phase handling
func (h *AnalyzingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	h.log.Info("Handling Analyzing phase",
		"name", analysis.Name,
		"namespace", analysis.Namespace)

	// Build policy input from analysis
	input := h.buildPolicyInput(analysis)

	// Evaluate policy
	result, err := h.evaluator.Evaluate(ctx, input)
	if err != nil {
		// This shouldn't happen due to graceful degradation in evaluator
		h.log.Error(err, "Rego evaluation returned error")
		analysis.Status.Phase = string(aianalysis.PhaseFailed)
		analysis.Status.Message = fmt.Sprintf("Rego evaluation failed: %v", err)
		return ctrl.Result{}, nil
	}

	// Store results in status
	analysis.Status.ApprovalRequired = result.ApprovalRequired
	analysis.Status.ApprovalReason = result.Reason

	if result.Degraded {
		analysis.Status.DegradedMode = true
		h.log.Info("Operating in degraded mode due to policy evaluation failure",
			"reason", result.Reason)
	}

	h.log.Info("Approval determination complete",
		"approval_required", result.ApprovalRequired,
		"reason", result.Reason,
		"degraded", result.Degraded)

	// Transition to Recommending phase
	analysis.Status.Phase = string(aianalysis.PhaseRecommending)
	return ctrl.Result{Requeue: true}, nil
}

// buildPolicyInput constructs the Rego policy input from the AIAnalysis resource.
func (h *AnalyzingHandler) buildPolicyInput(analysis *aianalysisv1.AIAnalysis) *rego.PolicyInput {
	spec := &analysis.Spec.AnalysisRequest.SignalContext
	status := &analysis.Status

	input := &rego.PolicyInput{
		Environment: spec.Environment,
		Warnings:    status.Warnings,
	}

	// Add targetInOwnerChain from status (set by InvestigatingHandler)
	if status.TargetInOwnerChain != nil {
		input.TargetInOwnerChain = *status.TargetInOwnerChain
	}

	// Add detected labels and failed detections
	if spec.EnrichmentResults.DetectedLabels != nil {
		input.DetectedLabels = h.convertDetectedLabels(spec.EnrichmentResults.DetectedLabels)
		input.FailedDetections = spec.EnrichmentResults.DetectedLabels.FailedDetections
	}

	// Add custom labels
	if spec.EnrichmentResults.CustomLabels != nil {
		input.CustomLabels = spec.EnrichmentResults.CustomLabels
	}

	return input
}

// convertDetectedLabels converts DetectedLabels struct to map[string]interface{} for Rego.
func (h *AnalyzingHandler) convertDetectedLabels(labels *aianalysisv1.DetectedLabels) map[string]interface{} {
	if labels == nil {
		return nil
	}

	result := map[string]interface{}{
		"gitOpsManaged":   labels.GitOpsManaged,
		"pdbProtected":    labels.PDBProtected,
		"hpaEnabled":      labels.HPAEnabled,
		"stateful":        labels.Stateful,
		"helmManaged":     labels.HelmManaged,
		"networkIsolated": labels.NetworkIsolated,
	}

	// Add optional string fields if present
	if labels.GitOpsTool != "" {
		result["gitOpsTool"] = labels.GitOpsTool
	}
	if labels.ServiceMesh != "" {
		result["serviceMesh"] = labels.ServiceMesh
	}

	return result
}

