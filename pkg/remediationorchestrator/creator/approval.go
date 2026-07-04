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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// DefaultApprovalTimeout is the default time allowed for operator approval (V1.0).
// Per ADR-040: 15 minutes default, within RR global timeout of 30 minutes.
const DefaultApprovalTimeout = 15 * time.Minute

// ApprovalCreator creates RemediationApprovalRequest CRDs.
// Reference: ADR-040, BR-ORCH-026
type ApprovalCreator struct {
	client          client.Client
	scheme          *runtime.Scheme
	metrics         *metrics.Metrics // DD-METRICS-001: Dependency-injected metrics
	approvalTimeout time.Duration    // ADR-040: Configurable approval deadline
}

// NewApprovalCreator creates a new ApprovalCreator.
// The approvalTimeout parameter sets the RequiredBy deadline on new RARs.
func NewApprovalCreator(c client.Client, s *runtime.Scheme, m *metrics.Metrics, approvalTimeout time.Duration) *ApprovalCreator {
	if approvalTimeout <= 0 {
		approvalTimeout = DefaultApprovalTimeout
	}
	return &ApprovalCreator{
		client:          c,
		scheme:          s,
		metrics:         m,
		approvalTimeout: approvalTimeout,
	}
}

// validateApprovalPreconditions enforces the AIAnalysis fields required to build an approval
// request: a non-nil AIAnalysis with a SelectedWorkflow that carries a WorkflowID.
func validateApprovalPreconditions(ai *aianalysisv1.AIAnalysis) error {
	if ai == nil {
		return fmt.Errorf("AIAnalysis is nil")
	}
	if ai.Status.SelectedWorkflow == nil {
		return fmt.Errorf("AIAnalysis %s/%s missing SelectedWorkflow for approval request", ai.Namespace, ai.Name)
	}
	if ai.Status.SelectedWorkflow.WorkflowID == "" {
		return fmt.Errorf("AIAnalysis %s/%s SelectedWorkflow missing WorkflowID", ai.Namespace, ai.Name)
	}
	return nil
}

// initApprovalConditions sets the initial DD-CRD-002-RAR conditions on rar in-memory
// (Pending=true, Decided=false, Expired=false) prior to Create().
func (c *ApprovalCreator) initApprovalConditions(rar *remediationv1.RemediationApprovalRequest) {
	remediationapprovalrequest.SetApprovalPending(rar, true,
		fmt.Sprintf("Awaiting decision, expires %s", rar.Spec.RequiredBy.Format(time.RFC3339)), c.metrics)
	remediationapprovalrequest.SetApprovalDecided(rar, false,
		remediationapprovalrequest.ReasonPendingDecision,
		"No decision yet", c.metrics)
	remediationapprovalrequest.SetApprovalExpired(rar, false,
		"Approval has not expired", c.metrics)
}

// persistApprovalRequest creates rar and then writes its initial status via the status
// subresource. The status must be saved and restored around Create() because the API
// server strips status from the response for CRDs with +kubebuilder:subresource:status.
func (c *ApprovalCreator) persistApprovalRequest(ctx context.Context, rar *remediationv1.RemediationApprovalRequest) error {
	now := metav1.Now()
	rar.Status.CreatedAt = &now
	c.initApprovalConditions(rar)

	savedStatus := rar.Status
	if err := c.client.Create(ctx, rar); err != nil {
		return fmt.Errorf("failed to create RemediationApprovalRequest: %w", err)
	}

	rar.Status = savedStatus
	if err := c.client.Status().Update(ctx, rar); err != nil {
		return fmt.Errorf("failed to update RemediationApprovalRequest status: %w", err)
	}
	return nil
}

// Create creates a RemediationApprovalRequest CRD for the given RemediationRequest and AIAnalysis.
// V1.0 Implementation: Per ADR-040 V1.0 scope.
// Reference: BR-ORCH-026 (Approval Orchestration)
func (c *ApprovalCreator) Create(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (string, error) {
	if err := validateApprovalPreconditions(ai); err != nil {
		return "", err
	}

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

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	// Gap 2.1: Prevents orphaned child CRDs if RR not properly persisted
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference")
		return "", fmt.Errorf("failed to set owner reference: RemediationRequest UID is required but empty")
	}

	rar := c.buildApprovalRequest(rr, ai, name)

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, rar, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := c.persistApprovalRequest(ctx, rar); err != nil {
		logger.Error(err, "Failed to persist RemediationApprovalRequest")
		return "", err
	}

	logger.Info("Created RemediationApprovalRequest",
		"name", name,
		"confidence", ai.Status.SelectedWorkflow.Confidence,
		"requiredBy", rar.Spec.RequiredBy.Format(time.RFC3339),
	)

	return name, nil
}

// confidenceLevelFor buckets a numeric confidence into the "low"/"medium"/"high" tiers
// used in the approval UI.
func confidenceLevelFor(confidence float64) string {
	switch {
	case confidence >= 0.8:
		return "high"
	case confidence >= 0.6:
		return "medium"
	default:
		return "low"
	}
}

// approvalInvestigationSummary picks the best available investigation summary: the
// legacy RootCause string, falling back to the structured RootCauseAnalysis.Summary.
func approvalInvestigationSummary(ai *aianalysisv1.AIAnalysis) string {
	if ai.Status.RootCause != "" {
		return ai.Status.RootCause
	}
	if ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.Summary != "" {
		return ai.Status.RootCauseAnalysis.Summary
	}
	return "Investigation completed"
}

// applyApprovalContext maps AIAnalysis's optional ApprovalContext (BR-AI-059, BR-AI-076,
// #307) onto rar.Spec: evidence collected, alternatives considered, and policy evaluation.
func applyApprovalContext(rar *remediationv1.RemediationApprovalRequest, ac *aianalysisv1.ApprovalContext) {
	if ac == nil {
		return
	}
	rar.Spec.EvidenceCollected = ac.EvidenceCollected

	if len(ac.AlternativesConsidered) > 0 {
		alts := make([]remediationv1.ApprovalAlternative, len(ac.AlternativesConsidered))
		for i, a := range ac.AlternativesConsidered {
			alts[i] = remediationv1.ApprovalAlternative{
				Approach: a.Approach,
				ProsCons: a.ProsCons,
			}
		}
		rar.Spec.AlternativesConsidered = alts
	}

	if ac.PolicyEvaluation != nil {
		rar.Spec.PolicyEvaluation = &remediationv1.ApprovalPolicyEvaluation{
			PolicyName:   ac.PolicyEvaluation.PolicyName,
			MatchedRules: ac.PolicyEvaluation.MatchedRules,
			Decision:     string(ac.PolicyEvaluation.Decision),
		}
	}
}

// buildApprovalRequest constructs the RemediationApprovalRequest from RR and AIAnalysis.
func (c *ApprovalCreator) buildApprovalRequest(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
	name string,
) *remediationv1.RemediationApprovalRequest {
	requiredBy := metav1.NewTime(time.Now().Add(c.approvalTimeout))

	confidence := float64(0)
	var recommendedWorkflow remediationv1.RecommendedWorkflowSummary
	if ai.Status.SelectedWorkflow != nil {
		confidence = ai.Status.SelectedWorkflow.Confidence
		recommendedWorkflow = remediationv1.RecommendedWorkflowSummary{
			WorkflowID:      ai.Status.SelectedWorkflow.WorkflowID,
			Version:         ai.Status.SelectedWorkflow.Version,
			ExecutionBundle: ai.Status.SelectedWorkflow.ExecutionBundle,
			Rationale:       ai.Status.SelectedWorkflow.Rationale,
		}
	}

	// Build recommended actions using the actual Rego policy reason (Issue #206)
	recommendedActions := []remediationv1.ApprovalRecommendedAction{
		{
			Action:    "Review the recommended workflow and approve if appropriate",
			Rationale: ai.Status.ApprovalReason,
		},
	}

	rar := &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			AIAnalysisRef: remediationv1.ObjectRef{
				Name: ai.Name,
			},
			ClusterID:            rr.Spec.ClusterID,
			Confidence:           confidence,
			ConfidenceLevel:      confidenceLevelFor(confidence),
			Reason:               ai.Status.ApprovalReason,
			RecommendedWorkflow:  recommendedWorkflow,
			InvestigationSummary: approvalInvestigationSummary(ai),
			RecommendedActions:   recommendedActions,
			WhyApprovalRequired:  ai.Status.ApprovalReason,
			RequiredBy:           requiredBy,
		},
	}

	applyApprovalContext(rar, ai.Status.ApprovalContext)

	return rar
}
