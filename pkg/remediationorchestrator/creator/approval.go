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

package creator

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// DefaultApprovalTimeout is the default time allowed for operator approval (V1.0).
// Per ADR-040: 15 minutes default, within RR global timeout of 30 minutes.
const DefaultApprovalTimeout = 15 * time.Minute

// ApprovalCreator creates RemediationApprovalRequest CRDs.
// Reference: ADR-040, BR-ORCH-026
type ApprovalCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewApprovalCreator creates a new ApprovalCreator.
func NewApprovalCreator(c client.Client, s *runtime.Scheme) *ApprovalCreator {
	return &ApprovalCreator{
		client: c,
		scheme: s,
	}
}

// Create creates a RemediationApprovalRequest CRD for the given RemediationRequest and AIAnalysis.
// V1.0 Implementation: Per ADR-040 V1.0 scope.
// Reference: BR-ORCH-026 (Approval Orchestration)
func (c *ApprovalCreator) Create(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"aiAnalysis", ai.Name,
	)

	// Generate deterministic name
	name := fmt.Sprintf("rar-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &remediationv1.RemediationApprovalRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("RemediationApprovalRequest already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing RemediationApprovalRequest")
		return "", fmt.Errorf("failed to check existing RemediationApprovalRequest: %w", err)
	}

	// Build the RemediationApprovalRequest
	rar := c.buildApprovalRequest(rr, ai, name)

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, rar, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, rar); err != nil {
		logger.Error(err, "Failed to create RemediationApprovalRequest")
		return "", fmt.Errorf("failed to create RemediationApprovalRequest: %w", err)
	}

	logger.Info("Created RemediationApprovalRequest",
		"name", name,
		"confidence", ai.Status.SelectedWorkflow.Confidence,
		"requiredBy", rar.Spec.RequiredBy.Format(time.RFC3339),
	)

	return name, nil
}

// buildApprovalRequest constructs the RemediationApprovalRequest from RR and AIAnalysis.
func (c *ApprovalCreator) buildApprovalRequest(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
	name string,
) *remediationv1.RemediationApprovalRequest {
	// Calculate deadline (V1.0: default 15 minutes)
	requiredBy := metav1.NewTime(time.Now().Add(DefaultApprovalTimeout))

	// Determine confidence level
	confidence := float64(0)
	confidenceLevel := "low"
	if ai.Status.SelectedWorkflow != nil {
		confidence = ai.Status.SelectedWorkflow.Confidence
		if confidence >= 0.8 {
			confidenceLevel = "high"
		} else if confidence >= 0.6 {
			confidenceLevel = "medium"
		}
	}

	// Build recommended workflow summary
	var recommendedWorkflow remediationv1.RecommendedWorkflowSummary
	if ai.Status.SelectedWorkflow != nil {
		recommendedWorkflow = remediationv1.RecommendedWorkflowSummary{
			WorkflowID:     ai.Status.SelectedWorkflow.WorkflowID,
			Version:        ai.Status.SelectedWorkflow.Version,
			ContainerImage: ai.Status.SelectedWorkflow.ContainerImage,
			Rationale:      ai.Status.SelectedWorkflow.Rationale,
		}
	}

	// Build recommended actions
	recommendedActions := []remediationv1.ApprovalRecommendedAction{
		{
			Action:    "Review the recommended workflow and approve if appropriate",
			Rationale: fmt.Sprintf("Confidence score (%.0f%%) is below auto-approval threshold (80%%)", confidence*100),
		},
	}

	// Build investigation summary
	investigationSummary := "Investigation completed"
	if ai.Status.RootCause != "" {
		investigationSummary = ai.Status.RootCause
	} else if ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.Summary != "" {
		investigationSummary = ai.Status.RootCauseAnalysis.Summary
	}

	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/ai-analysis":         ai.Name,
				"kubernaut.ai/confidence-level":    confidenceLevel,
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: *rr.Status.SignalProcessingRef, // Use existing ref pattern
			AIAnalysisRef: remediationv1.ObjectRef{
				Name: ai.Name,
			},
			Confidence:           confidence,
			ConfidenceLevel:      confidenceLevel,
			Reason:               ai.Status.ApprovalReason,
			RecommendedWorkflow:  recommendedWorkflow,
			InvestigationSummary: investigationSummary,
			RecommendedActions:   recommendedActions,
			WhyApprovalRequired:  fmt.Sprintf("Confidence %.0f%% is below 80%% threshold. %s", confidence*100, ai.Status.ApprovalReason),
			RequiredBy:           requiredBy,
		},
	}
}

