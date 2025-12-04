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
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	aiclient "github.com/jordigilh/kubernaut/pkg/aianalysis/client"
)

// ========================================
// INVESTIGATING HANDLER (BR-AI-007, BR-AI-008, BR-AI-009)
// Phase: Investigating → Analyzing
// ========================================

const (
	// MaxRetries is the maximum number of retry attempts for transient errors.
	// Per APPENDIX_B: 5 attempts (30s, 60s, 120s, 240s, 480s)
	MaxRetries = 5

	// BaseDelay is the initial retry delay.
	BaseDelay = 30 * time.Second

	// MaxDelay is the maximum retry delay.
	MaxDelay = 480 * time.Second

	// RetryCountAnnotation is the annotation key for tracking retry count.
	// Uses kubernaut.ai domain per project standard (NOTICE_LABEL_DOMAIN_AND_NOTIFICATION_ROUTING.md)
	RetryCountAnnotation = "aianalysis.kubernaut.ai/retry-count"
)

// InvestigatingHandler handles the Investigating phase by calling HolmesGPT-API.
// It processes the response, captures warnings and targetInOwnerChain,
// and transitions to the Analyzing phase on success.
type InvestigatingHandler struct {
	client   client.Client
	log      logr.Logger
	hgClient aiclient.HolmesGPTClient
}

// InvestigatingHandlerOption configures the InvestigatingHandler.
type InvestigatingHandlerOption func(*InvestigatingHandler)

// WithInvestigatingLogger sets the logger for the handler.
func WithInvestigatingLogger(log logr.Logger) InvestigatingHandlerOption {
	return func(h *InvestigatingHandler) {
		h.log = log
	}
}

// WithInvestigatingClient sets the Kubernetes client for the handler.
func WithInvestigatingClient(c client.Client) InvestigatingHandlerOption {
	return func(h *InvestigatingHandler) {
		h.client = c
	}
}

// WithHolmesGPTClient sets the HolmesGPT-API client for the handler.
func WithHolmesGPTClient(hgClient aiclient.HolmesGPTClient) InvestigatingHandlerOption {
	return func(h *InvestigatingHandler) {
		h.hgClient = hgClient
	}
}

// NewInvestigatingHandler creates a new InvestigatingHandler with the given options.
func NewInvestigatingHandler(opts ...InvestigatingHandlerOption) *InvestigatingHandler {
	h := &InvestigatingHandler{}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// Name returns the handler name for logging and metrics.
func (h *InvestigatingHandler) Name() string {
	return "InvestigatingHandler"
}

// Handle calls HolmesGPT-API and processes the response.
// BR-AI-007: Process HolmesGPT response
// BR-AI-008: Handle warnings and targetInOwnerChain
// BR-AI-009: Retry logic with exponential backoff
// BR-AI-010: Handle permanent errors
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := h.log.WithValues(
		"aianalysis", client.ObjectKeyFromObject(analysis),
		"phase", analysis.Status.Phase,
	)
	log.V(1).Info("Starting investigation via HolmesGPT-API")

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

// buildRequest constructs the HolmesGPT-API request from the AIAnalysis spec.
func (h *InvestigatingHandler) buildRequest(analysis *aianalysisv1.AIAnalysis) *aiclient.IncidentRequest {
	spec := &analysis.Spec.AnalysisRequest.SignalContext

	req := &aiclient.IncidentRequest{
		IncidentID:        analysis.Name,
		RemediationID:     analysis.Spec.RemediationID,
		SignalType:        spec.SignalType,
		Severity:          spec.Severity,
		SignalSource:      "kubernaut",
		ResourceNamespace: spec.TargetResource.Namespace,
		ResourceKind:      spec.TargetResource.Kind,
		ResourceName:      spec.TargetResource.Name,
		ErrorMessage:      fmt.Sprintf("Signal: %s in %s", spec.SignalType, spec.Environment),
		Environment:       spec.Environment,
		Priority:          spec.BusinessPriority,
	}

	// Add enrichment data if present
	if spec.EnrichmentResults.DetectedLabels != nil {
		req.DetectedLabels = h.convertDetectedLabels(spec.EnrichmentResults.DetectedLabels)
	}

	if spec.EnrichmentResults.CustomLabels != nil {
		req.CustomLabels = spec.EnrichmentResults.CustomLabels
	}

	// Add owner chain
	for _, entry := range spec.EnrichmentResults.OwnerChain {
		req.OwnerChain = append(req.OwnerChain, aiclient.OwnerChainEntry{
			Namespace: entry.Namespace,
			Kind:      entry.Kind,
			Name:      entry.Name,
		})
	}

	return req
}

// convertDetectedLabels converts DetectedLabels to a map for the API request.
func (h *InvestigatingHandler) convertDetectedLabels(labels *aianalysisv1.DetectedLabels) map[string]interface{} {
	result := make(map[string]interface{})

	result["gitOpsManaged"] = labels.GitOpsManaged
	result["pdbProtected"] = labels.PDBProtected
	result["hpaEnabled"] = labels.HPAEnabled
	result["stateful"] = labels.Stateful
	result["helmManaged"] = labels.HelmManaged
	result["networkIsolated"] = labels.NetworkIsolated

	if labels.GitOpsTool != "" {
		result["gitOpsTool"] = labels.GitOpsTool
	}
	if labels.ServiceMesh != "" {
		result["serviceMesh"] = labels.ServiceMesh
	}

	// Include FailedDetections if present (DD-WORKFLOW-001 v2.2)
	if len(labels.FailedDetections) > 0 {
		result["failedDetections"] = labels.FailedDetections
	}

	return result
}

// handleError categorizes and handles errors from HolmesGPT-API.
// BR-AI-009: Transient errors → retry with exponential backoff
// BR-AI-010: Permanent errors → fail without retry
func (h *InvestigatingHandler) handleError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
	log := h.log.WithValues("aianalysis", client.ObjectKeyFromObject(analysis))

	var apiErr *aiclient.APIError
	if errors.As(err, &apiErr) && apiErr.IsTransient() {
		// Transient error - check retry count
		retryCount := h.getRetryCount(analysis)
		if retryCount >= MaxRetries {
			// Max retries exceeded
			log.Error(err, "HolmesGPT-API max retries exceeded", "retryCount", retryCount)
			return h.markFailed(ctx, analysis, "MaxRetriesExceeded",
				fmt.Sprintf("HolmesGPT-API max retries exceeded after %d attempts: %v", retryCount, err))
		}

		// Schedule retry with exponential backoff
		delay := calculateBackoff(retryCount)
		h.incrementRetryCount(analysis)

		log.Info("Transient error, scheduling retry",
			"retryCount", retryCount+1,
			"delay", delay.String(),
		)

		// Update status with retry info
		analysis.Status.Message = fmt.Sprintf("Retrying after transient error (attempt %d/%d)", retryCount+1, MaxRetries)

		if updateErr := h.client.Status().Update(ctx, analysis); updateErr != nil {
			return ctrl.Result{}, aianalysis.NewTransientError("failed to update retry status", updateErr)
		}

		return ctrl.Result{RequeueAfter: delay}, nil
	}

	// Permanent error - fail immediately
	log.Error(err, "Permanent HolmesGPT-API error")
	return h.markFailed(ctx, analysis, "HolmesGPTAPIError", fmt.Sprintf("HolmesGPT-API error: %v", err))
}

// processResponse processes a successful HolmesGPT-API response.
// BR-AI-008: Capture targetInOwnerChain and warnings in status
func (h *InvestigatingHandler) processResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *aiclient.IncidentResponse) (ctrl.Result, error) {
	log := h.log.WithValues("aianalysis", client.ObjectKeyFromObject(analysis))
	log.Info("Processing HolmesGPT-API response",
		"confidence", resp.Confidence,
		"targetInOwnerChain", resp.TargetInOwnerChain,
		"warningsCount", len(resp.Warnings),
	)

	// Store investigation results in status
	analysis.Status.InvestigationID = resp.IncidentID

	// Store RCA results if present
	if resp.RootCauseAnalysis != nil {
		analysis.Status.RootCause = resp.RootCauseAnalysis.Summary
		analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:             resp.RootCauseAnalysis.Summary,
			Severity:            resp.RootCauseAnalysis.Severity,
			SignalType:          resp.RootCauseAnalysis.SignalType,
			ContributingFactors: resp.RootCauseAnalysis.ContributingFactors,
		}
	}

	// Store selected workflow if present
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

	// BR-AI-008: Store targetInOwnerChain and warnings
	analysis.Status.TargetInOwnerChain = &resp.TargetInOwnerChain
	analysis.Status.Warnings = resp.Warnings

	// Reset retry count on success
	h.resetRetryCount(analysis)

	// Transition to Analyzing phase
	analysis.Status.Phase = aianalysis.PhaseAnalyzing
	analysis.Status.Message = "Investigation complete, starting analysis"

	// Calculate investigation time
	if analysis.Status.StartedAt != nil {
		elapsed := time.Since(analysis.Status.StartedAt.Time)
		analysis.Status.InvestigationTime = int64(elapsed.Seconds())
	}

	if err := h.client.Status().Update(ctx, analysis); err != nil {
		return ctrl.Result{}, aianalysis.NewTransientError("failed to update status after investigation", err)
	}

	log.Info("Investigation complete, transitioning to Analyzing phase")
	return ctrl.Result{Requeue: true}, nil
}

// markFailed updates status to Failed phase.
func (h *InvestigatingHandler) markFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, reason, message string) (ctrl.Result, error) {
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.Message = message
	analysis.Status.Reason = reason
	analysis.Status.CompletedAt = &now

	if err := h.client.Status().Update(ctx, analysis); err != nil {
		return ctrl.Result{}, aianalysis.NewTransientError("failed to update failed status", err)
	}

	return ctrl.Result{}, nil
}

// ========================================
// RETRY HELPERS (BR-AI-009)
// ========================================

// getRetryCount returns the current retry count from annotations.
func (h *InvestigatingHandler) getRetryCount(analysis *aianalysisv1.AIAnalysis) int {
	if analysis.Annotations == nil {
		return 0
	}

	countStr, ok := analysis.Annotations[RetryCountAnnotation]
	if !ok {
		return 0
	}

	var count int
	fmt.Sscanf(countStr, "%d", &count)
	return count
}

// incrementRetryCount increments the retry count in annotations.
func (h *InvestigatingHandler) incrementRetryCount(analysis *aianalysisv1.AIAnalysis) {
	if analysis.Annotations == nil {
		analysis.Annotations = make(map[string]string)
	}

	count := h.getRetryCount(analysis) + 1
	analysis.Annotations[RetryCountAnnotation] = fmt.Sprintf("%d", count)
}

// resetRetryCount resets the retry count on success.
func (h *InvestigatingHandler) resetRetryCount(analysis *aianalysisv1.AIAnalysis) {
	if analysis.Annotations != nil {
		delete(analysis.Annotations, RetryCountAnnotation)
	}
}

// calculateBackoff calculates the delay for the next retry attempt.
// Uses exponential backoff with jitter: delay = min(baseDelay * 2^attempt, maxDelay) ± 10%
func calculateBackoff(attemptCount int) time.Duration {
	delay := time.Duration(float64(BaseDelay) * math.Pow(2, float64(attemptCount)))
	if delay > MaxDelay {
		delay = MaxDelay
	}

	// Add jitter (±10%)
	jitterFactor := 0.9 + 0.2*rand.Float64()
	return time.Duration(float64(delay) * jitterFactor)
}
