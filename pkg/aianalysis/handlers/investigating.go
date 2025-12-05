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

// Package handlers implements phase handlers for the AIAnalysis controller.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/client"
)

const (
	// MaxRetries for transient errors before marking as Failed
	MaxRetries = 5

	// BaseDelay for exponential backoff
	BaseDelay = 30 * time.Second

	// MaxDelay caps the backoff delay
	MaxDelay = 480 * time.Second

	// RetryCountAnnotation stores retry count in CRD annotations
	RetryCountAnnotation = "aianalysis.kubernaut.ai/retry-count"
)

// HolmesGPTClientInterface defines the interface for HolmesGPT-API calls
type HolmesGPTClientInterface interface {
	Investigate(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error)
}

// InvestigatingHandler handles the Investigating phase
// BR-AI-007: Call HolmesGPT-API and process response
type InvestigatingHandler struct {
	log      logr.Logger
	hgClient HolmesGPTClientInterface
}

// NewInvestigatingHandler creates a new InvestigatingHandler
func NewInvestigatingHandler(hgClient HolmesGPTClientInterface, log logr.Logger) *InvestigatingHandler {
	return &InvestigatingHandler{
		hgClient: hgClient,
		log:      log.WithName("investigating-handler"),
	}
}

// Handle processes the Investigating phase
// BR-AI-007: Call HolmesGPT-API and update status
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	h.log.Info("Processing Investigating phase", "name", analysis.Name)

	// Build request from spec
	req := h.buildRequest(analysis)

	// Call HolmesGPT-API
	resp, err := h.hgClient.Investigate(ctx, req)
	if err != nil {
		return h.handleError(ctx, analysis, err)
	}

	// Process successful response
	return h.processResponse(ctx, analysis, resp)
}

// buildRequest constructs the HolmesGPT-API request from AIAnalysis spec
func (h *InvestigatingHandler) buildRequest(analysis *aianalysisv1.AIAnalysis) *client.IncidentRequest {
	spec := analysis.Spec.AnalysisRequest.SignalContext

	return &client.IncidentRequest{
		Context: fmt.Sprintf("Incident in %s environment, target: %s/%s/%s, signal: %s",
			spec.Environment,
			spec.TargetResource.Namespace,
			spec.TargetResource.Kind,
			spec.TargetResource.Name,
			spec.SignalType,
		),
	}
}

// handleError processes errors from HolmesGPT-API
// BR-AI-009: Retry transient errors with exponential backoff
// BR-AI-010: Fail immediately on permanent errors
func (h *InvestigatingHandler) handleError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.IsTransient() {
		// Transient error - check retry count
		retryCount := h.getRetryCount(analysis)

		if retryCount >= MaxRetries {
			// Max retries exceeded - mark as Failed
			h.log.Info("Max retries exceeded", "retryCount", retryCount)
			analysis.Status.Phase = aianalysis.PhaseFailed
			analysis.Status.Message = fmt.Sprintf("HolmesGPT-API max retries exceeded: %v", err)
			analysis.Status.Reason = "MaxRetriesExceeded"
			return ctrl.Result{}, nil
		}

		// Schedule retry with backoff
		delay := calculateBackoff(retryCount)
		h.setRetryCount(analysis, retryCount+1)

		h.log.Info("Transient error, scheduling retry",
			"retryCount", retryCount+1,
			"delay", delay.String(),
		)
		return ctrl.Result{RequeueAfter: delay}, err
	}

	// Permanent error - mark as Failed immediately
	h.log.Info("Permanent error", "error", err)
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.Message = fmt.Sprintf("HolmesGPT-API error: %v", err)
	analysis.Status.Reason = "APIError"
	return ctrl.Result{}, nil
}

// processResponse handles successful HolmesGPT-API response
// BR-AI-008: Capture all response fields including RCA, workflow, and alternatives
// Per HolmesGPT-API team (Dec 5, 2025): /incident/analyze returns ALL analysis results
func (h *InvestigatingHandler) processResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
	h.log.Info("Processing successful response",
		"confidence", resp.Confidence,
		"targetInOwnerChain", resp.TargetInOwnerChain,
		"warningsCount", len(resp.Warnings),
		"hasSelectedWorkflow", resp.SelectedWorkflow != nil,
		"alternativeWorkflowsCount", len(resp.AlternativeWorkflows),
	)

	// Store HAPI response metadata
	targetInOwnerChain := resp.TargetInOwnerChain
	analysis.Status.TargetInOwnerChain = &targetInOwnerChain
	analysis.Status.Warnings = resp.Warnings

	// Store root cause analysis (if present)
	if resp.RootCauseAnalysis != nil {
		analysis.Status.RootCause = resp.RootCauseAnalysis.Summary
		analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:             resp.RootCauseAnalysis.Summary,
			Severity:            resp.RootCauseAnalysis.Severity,
			ContributingFactors: resp.RootCauseAnalysis.ContributingFactors,
		}
	}

	// Store selected workflow (DD-CONTRACT-002)
	if resp.SelectedWorkflow != nil {
		analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:      resp.SelectedWorkflow.WorkflowID,
			Version:         resp.SelectedWorkflow.Version,
			ContainerImage:  resp.SelectedWorkflow.ContainerImage,
			ContainerDigest: resp.SelectedWorkflow.ContainerDigest,
			Confidence:      resp.SelectedWorkflow.Confidence,
			Parameters:      resp.SelectedWorkflow.Parameters,
			Rationale:       resp.SelectedWorkflow.Rationale,
		}
	}

	// Store alternative workflows (INFORMATIONAL ONLY - NOT for execution)
	// Per HolmesGPT-API team: Alternatives are for CONTEXT, not EXECUTION
	if len(resp.AlternativeWorkflows) > 0 {
		alternatives := make([]aianalysisv1.AlternativeWorkflow, 0, len(resp.AlternativeWorkflows))
		for _, alt := range resp.AlternativeWorkflows {
			alternatives = append(alternatives, aianalysisv1.AlternativeWorkflow{
				WorkflowID:     alt.WorkflowID,
				ContainerImage: alt.ContainerImage,
				Confidence:     alt.Confidence,
				Rationale:      alt.Rationale,
			})
		}
		analysis.Status.AlternativeWorkflows = alternatives
	}

	// Reset retry count on success
	h.setRetryCount(analysis, 0)

	// Transition to Analyzing phase
	analysis.Status.Phase = aianalysis.PhaseAnalyzing
	analysis.Status.Message = "Investigation complete, starting analysis"

	return ctrl.Result{Requeue: true}, nil
}

// getRetryCount reads retry count from annotations
func (h *InvestigatingHandler) getRetryCount(analysis *aianalysisv1.AIAnalysis) int {
	if analysis.Annotations == nil {
		return 0
	}
	countStr, ok := analysis.Annotations[RetryCountAnnotation]
	if !ok {
		return 0
	}
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return 0
	}
	return count
}

// setRetryCount writes retry count to annotations
func (h *InvestigatingHandler) setRetryCount(analysis *aianalysisv1.AIAnalysis, count int) {
	if analysis.Annotations == nil {
		analysis.Annotations = make(map[string]string)
	}
	analysis.Annotations[RetryCountAnnotation] = strconv.Itoa(count)
}

// calculateBackoff computes exponential backoff with jitter
func calculateBackoff(attemptCount int) time.Duration {
	delay := time.Duration(float64(BaseDelay) * math.Pow(2, float64(attemptCount)))
	if delay > MaxDelay {
		delay = MaxDelay
	}
	// Add jitter (±10%)
	jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))
	return jitter
}
