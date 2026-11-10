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
	"errors"
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
	sharedclock "github.com/jordigilh/kubernaut/pkg/clock"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// DefaultApprovalTimeout is the default time allowed for operator approval (V1.0).
// Per ADR-040: 15 minutes default, within RR global timeout of 30 minutes.
const DefaultApprovalTimeout = 15 * time.Minute

// ApprovalRequestFinalizerName protects the RAR cleanup decision until the
// orchestrator has confirmed the safe parent outcome.
const ApprovalRequestFinalizerName = "remediationapprovalrequest.kubernaut.ai/cleanup"

// ApprovalCreationOutcome describes the persistence boundary reached by RAR
// creation. It is deliberately separate from the underlying API error so the
// parent controller can choose a safe terminal outcome without replaying a
// business create.
type ApprovalCreationOutcome string

const (
	ApprovalCreationCreated                 ApprovalCreationOutcome = "Created"
	ApprovalCreationCollision               ApprovalCreationOutcome = "Collision"
	ApprovalCreationStatusPersistenceFailed ApprovalCreationOutcome = "StatusPersistenceFailed"
	ApprovalCreationPersistenceUnknown      ApprovalCreationOutcome = "PersistenceUnknown"
)

const (
	ApprovalCreationReasonCollision          = "RAR_CREATE_COLLISION"
	ApprovalCreationReasonStatusFailed       = "RAR_STATUS_PERSISTENCE_FAILED"
	ApprovalCreationReasonPersistenceUnknown = "RAR_CREATE_PERSISTENCE_UNKNOWN"
	ApprovalRequestDeletionReason            = "RAR_DELETED_BEFORE_DECISION"
)

// ApprovalCreationError is returned when creation reaches a safety-relevant
// persistence boundary. Error() exposes the stable machine-readable reason so
// parent status and audit records do not depend on API-server wording.
type ApprovalCreationError struct {
	Outcome    ApprovalCreationOutcome
	ReasonCode string
	Err        error
}

func (e *ApprovalCreationError) Error() string {
	if e == nil {
		return ""
	}
	if e.ReasonCode != "" {
		return e.ReasonCode
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Outcome)
}

func (e *ApprovalCreationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ApprovalCreator creates RemediationApprovalRequest CRDs.
// Reference: ADR-040, BR-ORCH-026
type ApprovalCreator struct {
	client          client.Client
	scheme          *runtime.Scheme
	metrics         *metrics.Metrics // DD-METRICS-001: Dependency-injected metrics
	approvalTimeout time.Duration    // ADR-040: Configurable approval deadline
	clock           sharedclock.Clock
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
		clock:           sharedclock.RealClock{},
	}
}

// SetClock replaces the time source used for creation timestamps and
// approval deadlines. It is intended for deterministic boundary tests.
func (c *ApprovalCreator) SetClock(value sharedclock.Clock) {
	if value != nil {
		c.clock = value
	}
}

// validateApprovalPreconditions enforces the AIAnalysis fields required to build an approval
// request: a non-nil AIAnalysis with a SelectedWorkflow that carries a WorkflowID.
func validateApprovalPreconditions(ai *aianalysisv1.AIAnalysis) error {
	if ai == nil {
		return fmt.Errorf("AIAnalysis is nil")
	}
	sw := ai.Status.GetRCAResult().SelectedWorkflow
	if sw == nil {
		return fmt.Errorf("AIAnalysis %s/%s missing SelectedWorkflow for approval request", ai.Namespace, ai.Name)
	}
	if sw.WorkflowID == "" {
		return fmt.Errorf("AIAnalysis %s/%s SelectedWorkflow missing WorkflowID", ai.Namespace, ai.Name)
	}
	if sw.Parameters == nil {
		return fmt.Errorf("AIAnalysis %s/%s SelectedWorkflow missing original parameters", ai.Namespace, ai.Name)
	}
	if sw.DeclaredParameterNames == nil {
		return fmt.Errorf("AIAnalysis %s/%s SelectedWorkflow missing parameter declarations", ai.Namespace, ai.Name)
	}
	if sw.ParameterSchema == nil {
		return fmt.Errorf("AIAnalysis %s/%s SelectedWorkflow missing parameter schema", ai.Namespace, ai.Name)
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
	now := metav1.NewTime(c.clock.Now())
	rar.Status.CreatedAt = &now
	c.initApprovalConditions(rar)

	savedStatus := rar.Status
	if err := c.client.Create(ctx, rar); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return &ApprovalCreationError{
				Outcome:    ApprovalCreationCollision,
				ReasonCode: ApprovalCreationReasonCollision,
				Err:        err,
			}
		}

		// Create may have reached the API server before the transport failed.
		// Probe exactly once and never replay the business create.
		persisted := &remediationv1.RemediationApprovalRequest{}
		probeErr := c.client.Get(ctx, client.ObjectKeyFromObject(rar), persisted)
		if probeErr == nil {
			return &ApprovalCreationError{
				Outcome:    ApprovalCreationPersistenceUnknown,
				ReasonCode: ApprovalCreationReasonPersistenceUnknown,
				Err:        fmt.Errorf("create returned an ambiguous error and the object is present: %w", err),
			}
		}
		return &ApprovalCreationError{
			Outcome:    ApprovalCreationPersistenceUnknown,
			ReasonCode: ApprovalCreationReasonPersistenceUnknown,
			Err:        fmt.Errorf("create persistence could not be established: %w", err),
		}
	}

	rar.Status = savedStatus
	if err := c.client.Status().Update(ctx, rar); err != nil {
		return c.recoverApprovalStatus(ctx, rar, savedStatus, err)
	}
	return nil
}

// recoverApprovalStatus performs the single status-only recovery allowed after
// an ambiguous initial status write. It never replays the business Create.
func (c *ApprovalCreator) recoverApprovalStatus(ctx context.Context, rar *remediationv1.RemediationApprovalRequest, savedStatus remediationv1.RemediationApprovalRequestStatus, statusErr error) error {
	recovery := &remediationv1.RemediationApprovalRequest{}
	if readErr := c.client.Get(ctx, client.ObjectKeyFromObject(rar), recovery); readErr != nil {
		return statusPersistenceFailure(statusErr)
	}
	if recovery.UID != rar.UID {
		return statusPersistenceFailure(statusErr)
	}

	// A decision may have won while the initial status write was ambiguous.
	// Never restore the saved pending status over a terminal business decision.
	if approvalStatusIsTerminal(recovery.Status) {
		return &ApprovalCreationError{
			Outcome:    ApprovalCreationStatusPersistenceFailed,
			ReasonCode: ApprovalCreationReasonStatusFailed,
			Err:        fmt.Errorf("status recovery observed a concurrent terminal decision"),
		}
	}
	if recovery.Status.CreatedAt == nil {
		recovery.Status.CreatedAt = savedStatus.CreatedAt
	}
	if len(recovery.Status.Conditions) == 0 {
		recovery.Status.Conditions = savedStatus.Conditions
	}
	if recoveryErr := c.client.Status().Update(ctx, recovery); recoveryErr != nil {
		return statusPersistenceFailure(statusErr)
	}
	rar.Status = recovery.Status
	rar.ResourceVersion = recovery.ResourceVersion
	return nil
}

func approvalStatusIsTerminal(status remediationv1.RemediationApprovalRequestStatus) bool {
	return status.Decision != remediationv1.ApprovalDecisionPending ||
		status.DecidedBy != "" ||
		status.DecidedAt != nil ||
		status.DecisionMessage != "" ||
		status.WorkflowOverride != nil ||
		status.Expired
}

func statusPersistenceFailure(err error) error {
	return &ApprovalCreationError{
		Outcome:    ApprovalCreationStatusPersistenceFailed,
		ReasonCode: ApprovalCreationReasonStatusFailed,
		Err:        err,
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
	if rr == nil {
		return "", fmt.Errorf("RemediationRequest is nil")
	}
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

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	// Gap 2.1: Prevents orphaned child CRDs if RR not properly persisted
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference")
		return "", fmt.Errorf("failed to set owner reference: RemediationRequest UID is required but empty")
	}

	rar := c.buildApprovalRequest(rr, ai, name)
	controllerutil.AddFinalizer(rar, ApprovalRequestFinalizerName)

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, rar, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := c.persistApprovalRequest(ctx, rar); err != nil {
		logger.Error(err, "Failed to persist RemediationApprovalRequest")
		return name, err
	}

	logger.Info("Created RemediationApprovalRequest",
		"name", name,
		"confidence", ai.Status.GetRCAResult().SelectedWorkflow.Confidence,
		"requiredBy", rar.Spec.RequiredBy.Format(time.RFC3339),
	)

	return name, nil
}

// CreateWithResult is the explicit typed creation contract used by callers
// that need to distinguish a confirmed create from a safety-relevant failure.
func (c *ApprovalCreator) CreateWithResult(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (string, ApprovalCreationOutcome, error) {
	name, err := c.Create(ctx, rr, ai)
	if err == nil {
		return name, ApprovalCreationCreated, nil
	}
	var creationErr *ApprovalCreationError
	if errors.As(err, &creationErr) {
		return name, creationErr.Outcome, err
	}
	return name, ApprovalCreationPersistenceUnknown, err
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
	rca := ai.Status.GetRCAResult()
	if rca.RootCause != "" {
		return rca.RootCause
	}
	if rca.RootCauseAnalysis != nil && rca.RootCauseAnalysis.Summary != "" {
		return rca.RootCauseAnalysis.Summary
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
			// BR-AUDIT-006, Issue #1981: pin the policy hash onto the immutable RAR spec
			PolicyHash: ac.PolicyEvaluation.PolicyHash,
		}
	}
}

// buildApprovalRequest constructs the RemediationApprovalRequest from RR and AIAnalysis.
func (c *ApprovalCreator) buildApprovalRequest(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
	name string,
) *remediationv1.RemediationApprovalRequest {
	requiredBy := metav1.NewTime(c.clock.Now().Add(c.approvalTimeout))

	confidence := float64(0)
	var recommendedWorkflow remediationv1.RecommendedWorkflowSummary
	if sw := ai.Status.GetRCAResult().SelectedWorkflow; sw != nil {
		confidence = sw.Confidence
		recommendedWorkflow = remediationv1.RecommendedWorkflowSummary{
			WorkflowID:      sw.WorkflowID,
			Version:         sw.Version,
			ExecutionBundle: sw.ExecutionBundle,
			Rationale:       sw.Rationale,
		}
	}

	approvalReason := ai.Status.GetApproval().ApprovalReason

	// Build recommended actions using the actual Rego policy reason (Issue #206)
	recommendedActions := []remediationv1.ApprovalRecommendedAction{
		{
			Action:    "Review the recommended workflow and approve if appropriate",
			Rationale: approvalReason,
		},
	}

	rar := &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RCAConfidence: ai.Status.GetRCAResult().RCAConfidence,
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
			Reason:               approvalReason,
			RecommendedWorkflow:  recommendedWorkflow,
			InvestigationSummary: approvalInvestigationSummary(ai),
			RecommendedActions:   recommendedActions,
			WhyApprovalRequired:  approvalReason,
			RequiredBy:           requiredBy,
		},
	}

	applyApprovalContext(rar, ai.Status.GetApproval().ApprovalContext)
	rar.Spec.TargetContext = &remediationv1.ApprovalTargetContext{
		SignalTarget:    rr.Spec.TargetResource,
		SignalClusterID: rr.Spec.ClusterID,
	}
	if rca := ai.Status.GetRCAResult().RootCauseAnalysis; rca != nil && rca.RemediationTarget != nil {
		target := rca.RemediationTarget
		rar.Spec.TargetContext.RemediationTarget = &remediationv1.ResourceIdentifier{
			APIVersion: target.APIVersion, Kind: target.Kind, Namespace: target.Namespace, Name: target.Name,
		}
	}
	if selected := ai.Status.GetRCAResult().SelectedWorkflow; selected != nil {
		frozen := selected.DeepCopy()
		rar.Spec.Workflow = &frozen.WorkflowSnapshot
		rar.Spec.Parameters = frozen.Parameters
		rar.Spec.ParameterSchema = frozen.ParameterSchema
	}

	return rar
}
