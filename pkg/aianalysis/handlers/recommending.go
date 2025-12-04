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

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
)

// ========================================
// RECOMMENDING HANDLER (BR-AI-016, BR-AI-017, BR-AI-018)
// Phase: Recommending → Completed
// ========================================

// RecommendingHandler handles the Recommending phase.
// It finalizes the analysis by confirming workflow selection
// and transitioning to the Completed phase.
//
// BR-AI-016: Workflow recommendation finalization
// BR-AI-017: Store recommendations in status
// BR-AI-018: Handle no recommendations scenario
type RecommendingHandler struct {
	client client.Client
	log    logr.Logger
}

// RecommendingHandlerOption configures the RecommendingHandler.
type RecommendingHandlerOption func(*RecommendingHandler)

// WithRecommendingLogger sets the logger for the handler.
func WithRecommendingLogger(log logr.Logger) RecommendingHandlerOption {
	return func(h *RecommendingHandler) {
		h.log = log
	}
}

// WithRecommendingClient sets the Kubernetes client for the handler.
func WithRecommendingClient(c client.Client) RecommendingHandlerOption {
	return func(h *RecommendingHandler) {
		h.client = c
	}
}

// NewRecommendingHandler creates a new RecommendingHandler with the given options.
func NewRecommendingHandler(opts ...RecommendingHandlerOption) *RecommendingHandler {
	h := &RecommendingHandler{}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// Name returns the handler name for logging and metrics.
func (h *RecommendingHandler) Name() string {
	return "RecommendingHandler"
}

// Handle finalizes the workflow recommendation and completes the analysis.
// BR-AI-016: Finalize workflow recommendation
func (h *RecommendingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := h.log.WithValues(
		"aianalysis", client.ObjectKeyFromObject(analysis),
		"phase", analysis.Status.Phase,
	)
	log.V(1).Info("Handling Recommending phase")

	// The workflow selection was already done in InvestigatingHandler.
	// This phase finalizes and validates the recommendation.

	if analysis.Status.SelectedWorkflow != nil {
		// Workflow was selected - log details
		log.Info("Workflow recommendation confirmed",
			"workflowID", analysis.Status.SelectedWorkflow.WorkflowID,
			"confidence", analysis.Status.SelectedWorkflow.Confidence,
		)

		// Validate confidence threshold (optional business rule)
		if analysis.Status.SelectedWorkflow.Confidence < 0.5 {
			analysis.Status.Message = "Low confidence workflow recommendation - manual review suggested"
			log.Info("Low confidence warning",
				"confidence", analysis.Status.SelectedWorkflow.Confidence,
			)
		}
	} else {
		// No workflow selected - this is valid for some scenarios
		// BR-AI-018: Handle no recommendations scenario
		log.Info("No workflow recommendation available")
		analysis.Status.Message = "Analysis complete - no suitable workflow found"
	}

	// Mark as completed
	now := metav1.Now()
	analysis.Status.Phase = string(aianalysis.PhaseCompleted)
	analysis.Status.CompletedAt = &now

	// Calculate total analysis time if StartedAt is set
	if analysis.Status.StartedAt != nil {
		elapsed := now.Sub(analysis.Status.StartedAt.Time)
		analysis.Status.TotalAnalysisTime = int64(elapsed.Seconds())
	}

	// Update status
	if err := h.client.Status().Update(ctx, analysis); err != nil {
		return ctrl.Result{}, aianalysis.NewTransientError("failed to update completed status", err)
	}

	log.Info("Analysis completed",
		"hasWorkflow", analysis.Status.SelectedWorkflow != nil,
		"approvalRequired", analysis.Status.ApprovalRequired,
		"totalTime", analysis.Status.TotalAnalysisTime,
	)

	// Final phase - no requeue
	return ctrl.Result{}, nil
}

