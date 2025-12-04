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

// Package handlers implements phase-specific handlers for AIAnalysis reconciliation.
package handlers

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
)

// ========================================
// VALIDATING HANDLER (BR-AI-001, BR-AI-002)
// Phase: Validating → Investigating
// ========================================

// ValidatingHandler validates the AIAnalysis spec before processing.
// It ensures all required fields are present and valid before
// proceeding to the Investigating phase.
type ValidatingHandler struct {
	client client.Client
	log    logr.Logger
}

// ValidatingHandlerOption configures the ValidatingHandler.
type ValidatingHandlerOption func(*ValidatingHandler)

// WithLogger sets the logger for the handler.
func WithLogger(log logr.Logger) ValidatingHandlerOption {
	return func(h *ValidatingHandler) {
		h.log = log
	}
}

// WithClient sets the Kubernetes client for the handler.
func WithClient(c client.Client) ValidatingHandlerOption {
	return func(h *ValidatingHandler) {
		h.client = c
	}
}

// NewValidatingHandler creates a new ValidatingHandler with the given options.
func NewValidatingHandler(opts ...ValidatingHandlerOption) *ValidatingHandler {
	h := &ValidatingHandler{}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// Name returns the handler name for logging and metrics.
func (h *ValidatingHandler) Name() string {
	return "ValidatingHandler"
}

// Handle validates the AIAnalysis spec and transitions to Investigating.
func (h *ValidatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := h.log.WithValues(
		"aianalysis", client.ObjectKeyFromObject(analysis),
		"phase", analysis.Status.Phase,
	)
	log.V(1).Info("Validating AIAnalysis spec")

	// Validate required fields
	if err := h.validateSpec(analysis); err != nil {
		return ctrl.Result{}, err
	}

	// Validate FailedDetections field names (DD-WORKFLOW-001 v2.2)
	if err := h.validateFailedDetections(analysis); err != nil {
		return ctrl.Result{}, err
	}

	// Validation passed - transition to Investigating
	log.Info("Validation passed, transitioning to Investigating")
	analysis.Status.Phase = aianalysis.PhaseInvestigating
	analysis.Status.Message = "Validation complete, starting investigation"

	if err := h.client.Status().Update(ctx, analysis); err != nil {
		return ctrl.Result{}, aianalysis.NewTransientError("failed to update status", err)
	}

	return ctrl.Result{Requeue: true}, nil
}

// validateSpec validates required fields in the AIAnalysis spec.
func (h *ValidatingHandler) validateSpec(analysis *aianalysisv1.AIAnalysis) error {
	spec := &analysis.Spec

	// RemediationRequestRef validation
	if spec.RemediationRequestRef.Name == "" {
		return aianalysis.NewValidationError("remediationRequestRef.name", "is required")
	}

	// RemediationID validation
	if spec.RemediationID == "" {
		return aianalysis.NewValidationError("remediationId", "is required")
	}

	// SignalContext validation
	ctx := &spec.AnalysisRequest.SignalContext

	if ctx.Fingerprint == "" {
		return aianalysis.NewValidationError("signalContext.fingerprint", "is required")
	}

	if ctx.Environment == "" {
		return aianalysis.NewValidationError("signalContext.environment", "is required")
	}

	if len(ctx.Environment) > 63 {
		return aianalysis.NewValidationError("signalContext.environment", "exceeds maximum length of 63")
	}

	if ctx.BusinessPriority == "" {
		return aianalysis.NewValidationError("signalContext.businessPriority", "is required")
	}

	if ctx.SignalType == "" {
		return aianalysis.NewValidationError("signalContext.signalType", "is required")
	}

	// TargetResource validation
	if ctx.TargetResource.Kind == "" {
		return aianalysis.NewValidationError("signalContext.targetResource.kind", "is required")
	}

	if ctx.TargetResource.Name == "" {
		return aianalysis.NewValidationError("signalContext.targetResource.name", "is required")
	}

	// AnalysisTypes validation
	if len(spec.AnalysisRequest.AnalysisTypes) == 0 {
		return aianalysis.NewValidationError("analysisRequest.analysisTypes", "at least one analysis type is required")
	}

	return nil
}

// ValidDetectedLabelFields defines valid field names for FailedDetections.
// DD-WORKFLOW-001 v2.2: Enum validation for FailedDetections slice.
var ValidDetectedLabelFields = map[string]bool{
	"gitOpsManaged":   true,
	"pdbProtected":    true,
	"hpaEnabled":      true,
	"stateful":        true,
	"helmManaged":     true,
	"networkIsolated": true,
	"serviceMesh":     true,
}

// validateFailedDetections validates the FailedDetections field names.
// DD-WORKFLOW-001 v2.2: Each field name must be in ValidDetectedLabelFields.
func (h *ValidatingHandler) validateFailedDetections(analysis *aianalysisv1.AIAnalysis) error {
	enrichment := analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults

	// If no DetectedLabels, nothing to validate
	if enrichment.DetectedLabels == nil {
		return nil
	}

	// Validate each field name in FailedDetections
	for _, field := range enrichment.DetectedLabels.FailedDetections {
		if !ValidDetectedLabelFields[field] {
			return aianalysis.NewValidationError(
				"enrichmentResults.detectedLabels.failedDetections",
				"invalid field name: "+field,
			)
		}
	}

	return nil
}

// ValidateSpec is exported for testing purposes.
// It performs full spec validation and returns any validation errors.
func (h *ValidatingHandler) ValidateSpec(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
	if err := h.validateSpec(analysis); err != nil {
		return err
	}
	return h.validateFailedDetections(analysis)
}

// ValidateFailedDetections is exported for testing purposes.
func (h *ValidatingHandler) ValidateFailedDetections(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
	return h.validateFailedDetections(analysis)
}

