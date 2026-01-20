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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// NotificationCreator creates NotificationRequest CRDs for the Remediation Orchestrator.
// Reference: BR-ORCH-001 (approval notification), BR-ORCH-034 (bulk duplicate), BR-ORCH-036 (manual review)
type NotificationCreator struct {
	client  client.Client
	scheme  *runtime.Scheme
	metrics *metrics.Metrics
}

// NewNotificationCreator creates a new NotificationCreator.
func NewNotificationCreator(c client.Client, s *runtime.Scheme, m *metrics.Metrics) *NotificationCreator {
	// DD-METRICS-001: Metrics are REQUIRED (dependency injection pattern) - AUTHORITATIVE MANDATE
	// Metrics are initialized in reconciler's NewReconciler() which gets them from main.go
	// If nil is passed here, it's a programming error in the call chain
	if m == nil {
		panic("DD-METRICS-001 violation: NotificationCreator requires non-nil metrics (authoritative mandate)")
	}
	return &NotificationCreator{
		client:  c,
		scheme:  s,
		metrics: m,
	}
}

// CreateApprovalNotification creates a NotificationRequest for approval (BR-ORCH-001).
// It receives AIAnalysis as a parameter (consistent with Day 2-3 pattern).
// Reference: BR-ORCH-001 (approval notification), BR-ORCH-031 (cascade deletion), BR-ORCH-035 (ref tracking)
func (c *NotificationCreator) CreateApprovalNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"aiAnalysis", ai.Name,
	)

	// Precondition validation (per Day 3 pattern - BR-ORCH-001)
	if ai.Status.SelectedWorkflow == nil {
		logger.Error(nil, "AIAnalysis missing SelectedWorkflow for approval notification")
		return "", fmt.Errorf("AIAnalysis %s/%s missing SelectedWorkflow for approval notification", ai.Namespace, ai.Name)
	}
	if ai.Status.SelectedWorkflow.WorkflowID == "" {
		logger.Error(nil, "AIAnalysis SelectedWorkflow missing WorkflowID")
		return "", fmt.Errorf("AIAnalysis %s/%s SelectedWorkflow missing WorkflowID", ai.Namespace, ai.Name)
	}

	// Generate deterministic name
	name := fmt.Sprintf("nr-approval-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("Approval notification already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing NotificationRequest")
		return "", fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}

	// Determine channels based on context
	channels := c.determineApprovalChannels(rr, ai)

	// Build NotificationRequest for approval
	// API Contract: Uses Subject/Body (not Title/Message), Metadata (not Context)
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "approval",
				"kubernaut.ai/severity":            rr.Spec.Severity,
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type: notificationv1.NotificationTypeApproval,
			// Priority now from AIAnalysis.Spec.SignalContext.BusinessPriority (set by SP, not RR.Spec)
			Priority: c.mapPriority(ai.Spec.AnalysisRequest.SignalContext.BusinessPriority),
			Subject:  fmt.Sprintf("Approval Required: %s", rr.Spec.SignalName),
			Body:     c.buildApprovalBody(rr, ai),
			Channels: channels,
			Metadata: map[string]string{
				"remediationRequest": rr.Name,
				"aiAnalysis":         ai.Name,
				"approvalReason":     ai.Status.ApprovalReason,
				"confidence":         fmt.Sprintf("%.2f", ai.Status.SelectedWorkflow.Confidence),
				"selectedWorkflow":   ai.Status.SelectedWorkflow.WorkflowID,
				"severity":           rr.Spec.Severity,
			},
		},
	}

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	// Gap 2.1: Prevents orphaned child CRDs if RR not properly persisted
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference")
		return "", fmt.Errorf("failed to set owner reference: RemediationRequest UID is required but empty")
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create approval NotificationRequest")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	logger.Info("Created approval NotificationRequest",
		"name", name,
		"channels", channels,
		"approvalReason", ai.Status.ApprovalReason,
	)

	// BR-ORCH-029, BR-ORCH-030: Track approval notification creation (DD-METRICS-001)
	c.metrics.ApprovalNotificationsTotal.WithLabelValues(rr.Namespace).Inc()

	// BR-ORCH-035: Caller (reconciler) appends to rr.Status.NotificationRequestRefs
	return name, nil
}

// determineApprovalChannels determines notification channels based on context.
// Returns typed Channel slice per API contract.
func (c *NotificationCreator) determineApprovalChannels(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) []notificationv1.Channel {
	channels := []notificationv1.Channel{notificationv1.ChannelSlack} // Default

	// High-risk actions or production environment get additional channels
	if ai.Status.ApprovalReason == "high_risk_action" {
		channels = append(channels, notificationv1.ChannelEmail)
	}

	return channels
}

// mapPriority maps remediation priority string to NotificationPriority enum.
func (c *NotificationCreator) mapPriority(priority string) notificationv1.NotificationPriority {
	switch priority {
	case "P0":
		return notificationv1.NotificationPriorityCritical
	case "P1":
		return notificationv1.NotificationPriorityHigh
	case "P2":
		return notificationv1.NotificationPriorityMedium
	default:
		return notificationv1.NotificationPriorityLow
	}
}

// buildApprovalBody builds the approval notification body.
func (c *NotificationCreator) buildApprovalBody(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) string {
	// Safely get root cause
	rootCause := ai.Status.RootCause
	if ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.Summary != "" {
		rootCause = ai.Status.RootCauseAnalysis.Summary
	}

	// Safely get approval reason
	approvalReason := ai.Status.ApprovalReason
	if ai.Status.ApprovalContext != nil && ai.Status.ApprovalContext.Reason != "" {
		approvalReason = ai.Status.ApprovalContext.Reason
	}

	return fmt.Sprintf(`Remediation requires approval:

**Signal**: %s
**Severity**: %s

**Root Cause Analysis**:
%s

**Confidence**: %.0f%%

**Proposed Workflow**: %s

**Approval Reason**: %s

Please review and approve/reject the remediation.`,
		rr.Spec.SignalName,
		rr.Spec.Severity,
		rootCause,
		ai.Status.SelectedWorkflow.Confidence*100,
		ai.Status.SelectedWorkflow.WorkflowID,
		approvalReason,
	)
}

// CreateBulkDuplicateNotification creates a NotificationRequest for bulk duplicates (BR-ORCH-034).
// Reference: BR-ORCH-034 (bulk duplicate notification), BR-ORCH-031 (cascade deletion), BR-ORCH-035 (ref tracking)
func (c *NotificationCreator) CreateBulkDuplicateNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"duplicateCount", rr.Status.DuplicateCount,
	)

	// Generate deterministic name
	name := fmt.Sprintf("nr-bulk-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("Bulk notification already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing NotificationRequest")
		return "", fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}

	// Build bulk notification
	// API Contract: Uses Subject/Body (not Title/Message), Metadata (not Context)
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "bulk-duplicate",
				"kubernaut.ai/severity":            "low", // Informational
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeSimple, // Informational
			Priority: notificationv1.NotificationPriorityLow,
			Subject:  fmt.Sprintf("Remediation Completed with %d Duplicates", rr.Status.DuplicateCount),
			Body:     c.buildBulkDuplicateBody(rr),
			Channels: []notificationv1.Channel{notificationv1.ChannelSlack}, // Lower priority channel
			Metadata: map[string]string{
				"remediationRequest": rr.Name,
				"duplicateCount":     fmt.Sprintf("%d", rr.Status.DuplicateCount),
			},
		},
	}

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	// Gap 2.1: Prevents orphaned child CRDs if RR not properly persisted
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference")
		return "", fmt.Errorf("failed to set owner reference: RemediationRequest UID is required but empty")
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create bulk NotificationRequest")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	logger.Info("Created bulk duplicate NotificationRequest", "name", name)

	// BR-ORCH-035: Caller (reconciler) appends to rr.Status.NotificationRequestRefs
	return name, nil
}

// buildBulkDuplicateBody builds the bulk duplicate notification body.
func (c *NotificationCreator) buildBulkDuplicateBody(rr *remediationv1.RemediationRequest) string {
	return fmt.Sprintf(`Remediation completed successfully.

**Signal**: %s
**Result**: %s

**Duplicate Remediations**: %d

All duplicate signals have been handled by this remediation.`,
		rr.Spec.SignalName,
		rr.Status.OverallPhase,
		rr.Status.DuplicateCount,
	)
}

// ========================================
// MANUAL REVIEW NOTIFICATIONS (BR-ORCH-036)
// ========================================

// ManualReviewSource indicates the source of the manual review requirement.
type ManualReviewSource string

const (
	// ManualReviewSourceAIAnalysis indicates AIAnalysis WorkflowResolutionFailed
	ManualReviewSourceAIAnalysis ManualReviewSource = "AIAnalysis"
	// ManualReviewSourceWorkflowExecution indicates WE ExhaustedRetries or ExecutionFailure
	ManualReviewSourceWorkflowExecution ManualReviewSource = "WorkflowExecution"
)

// ManualReviewContext provides context for manual review notifications.
// Used by both AIAnalysis and WorkflowExecution failure scenarios.
type ManualReviewContext struct {
	// Source indicates which component triggered the manual review
	Source ManualReviewSource
	// Reason is the high-level failure reason (e.g., "WorkflowResolutionFailed", "ExhaustedRetries", "HumanReviewRequired")
	Reason string
	// SubReason provides granular detail (e.g., "WorkflowNotFound", "LowConfidence")
	SubReason string
	// HumanReviewReason (BR-HAPI-197): Explicit reason from HAPI when needs_human_review=true
	// Maps to AIAnalysis.Status.HumanReviewReason enum (workflow_not_found, rca_incomplete, etc.)
	HumanReviewReason string
	// Message is a human-readable description of the failure
	Message string
	// RootCauseAnalysis if available (from AIAnalysis)
	RootCauseAnalysis string
	// Warnings if available (from AIAnalysis)
	Warnings []string

	// WorkflowExecution-specific fields (for ExhaustedRetries)
	// RetryCount is the number of retries attempted
	RetryCount int
	// MaxRetries is the maximum configured retry count
	MaxRetries int
	// LastExitCode is the exit code from the last execution attempt
	LastExitCode int
	// PreviousExecution is the name of the previous failed WorkflowExecution (for PreviousExecutionFailed)
	PreviousExecution string
}

// CreateManualReviewNotification creates a NotificationRequest for manual review (BR-ORCH-036).
// This is triggered by:
// - AIAnalysis WorkflowResolutionFailed (SubReasons: WorkflowNotFound, ImageMismatch, etc.)
// - WorkflowExecution ExhaustedRetries or PreviousExecutionFailed
// Reference: BR-ORCH-036 (manual review), BR-ORCH-031 (cascade deletion), BR-ORCH-035 (ref tracking)
func (c *NotificationCreator) CreateManualReviewNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	reviewCtx *ManualReviewContext,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"source", reviewCtx.Source,
		"reason", reviewCtx.Reason,
		"subReason", reviewCtx.SubReason,
	)

	// Generate deterministic name (single manual review per RR)
	name := fmt.Sprintf("nr-manual-review-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("Manual review notification already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing NotificationRequest")
		return "", fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}

	// Determine priority based on source and reason (per BR-ORCH-036 priority mapping)
	priority := c.mapManualReviewPriority(reviewCtx)

	// Determine channels based on priority
	channels := c.determineManualReviewChannels(priority)

	// Build metadata with context-specific fields
	metadata := c.buildManualReviewMetadata(rr, reviewCtx)

	// Build NotificationRequest for manual review
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "manual-review",
				"kubernaut.ai/review-source":       string(reviewCtx.Source),
				"kubernaut.ai/severity":            rr.Spec.Severity,
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeManualReview,
			Priority: priority,
			Subject:  fmt.Sprintf("⚠️ Manual Review Required: %s", rr.Spec.SignalName),
			Body:     c.buildManualReviewBody(rr, reviewCtx),
			Channels: channels,
			Metadata: metadata,
		},
	}

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	// Gap 2.1: Prevents orphaned child CRDs if RR not properly persisted
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference")
		return "", fmt.Errorf("failed to set owner reference: RemediationRequest UID is required but empty")
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create manual review NotificationRequest")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	logger.Info("Created manual review NotificationRequest",
		"name", name,
		"priority", priority,
		"channels", channels,
	)

	// BR-ORCH-029, BR-ORCH-036: Track manual review notification creation (DD-METRICS-001)
	// Metric expects: []string{"source", "reason", "sub_reason", "namespace"}
	subReason := reviewCtx.SubReason
	if subReason == "" {
		subReason = "none"
	}
	c.metrics.ManualReviewNotificationsTotal.WithLabelValues(
		string(reviewCtx.Source),
		reviewCtx.Reason,
		subReason,
		rr.Namespace,
	).Inc()

	// BR-ORCH-035: Caller (reconciler) appends to rr.Status.NotificationRequestRefs
	return name, nil
}

// mapManualReviewPriority maps manual review context to notification priority.
// Per BR-ORCH-036 priority mapping:
// - WE failures (ExhaustedRetries, PreviousExecutionFailed, ExecutionFailure) → critical
// - AI WorkflowNotFound, ImageMismatch, ParameterValidationFailed, LLMParsingError → high
// - AI NoMatchingWorkflows, LowConfidence, InvestigationInconclusive → medium
func (c *NotificationCreator) mapManualReviewPriority(ctx *ManualReviewContext) notificationv1.NotificationPriority {
	if ctx.Source == ManualReviewSourceWorkflowExecution {
		// All WE failures are critical (cluster state may be unknown)
		return notificationv1.NotificationPriorityCritical
	}

	// AIAnalysis failures - map by SubReason
	switch ctx.SubReason {
	case "WorkflowNotFound", "ImageMismatch", "ParameterValidationFailed", "LLMParsingError":
		return notificationv1.NotificationPriorityHigh
	case "NoMatchingWorkflows", "LowConfidence", "InvestigationInconclusive":
		return notificationv1.NotificationPriorityMedium
	default:
		return notificationv1.NotificationPriorityMedium
	}
}

// determineManualReviewChannels determines channels based on priority.
func (c *NotificationCreator) determineManualReviewChannels(priority notificationv1.NotificationPriority) []notificationv1.Channel {
	switch priority {
	case notificationv1.NotificationPriorityCritical:
		// Critical: Slack + Email for immediate attention
		return []notificationv1.Channel{notificationv1.ChannelSlack, notificationv1.ChannelEmail}
	case notificationv1.NotificationPriorityHigh:
		// High: Slack + Email
		return []notificationv1.Channel{notificationv1.ChannelSlack, notificationv1.ChannelEmail}
	default:
		// Medium/Low: Slack only
		return []notificationv1.Channel{notificationv1.ChannelSlack}
	}
}

// buildManualReviewMetadata builds the metadata map for manual review notifications.
// Includes source-specific fields for context.
func (c *NotificationCreator) buildManualReviewMetadata(rr *remediationv1.RemediationRequest, ctx *ManualReviewContext) map[string]string {
	metadata := map[string]string{
		"remediationRequest": rr.Name,
		"source":             string(ctx.Source),
		"reason":             ctx.Reason,
	}

	// Add SubReason if present
	if ctx.SubReason != "" {
		metadata["subReason"] = ctx.SubReason
	}

	// BR-HAPI-197: Add HumanReviewReason if present (explicit HAPI decision)
	if ctx.HumanReviewReason != "" {
		metadata["humanReviewReason"] = ctx.HumanReviewReason
	}

	// Add RootCauseAnalysis if present (from AIAnalysis)
	if ctx.RootCauseAnalysis != "" {
		metadata["rootCauseAnalysis"] = ctx.RootCauseAnalysis
	}

	// Add WorkflowExecution-specific fields
	if ctx.Source == ManualReviewSourceWorkflowExecution {
		if ctx.RetryCount > 0 || ctx.MaxRetries > 0 {
			metadata["retryCount"] = fmt.Sprintf("%d", ctx.RetryCount)
			metadata["maxRetries"] = fmt.Sprintf("%d", ctx.MaxRetries)
		}
		if ctx.LastExitCode != 0 {
			metadata["lastExitCode"] = fmt.Sprintf("%d", ctx.LastExitCode)
		}
		if ctx.PreviousExecution != "" {
			metadata["previousExecution"] = ctx.PreviousExecution
		}
	}

	return metadata
}

// buildManualReviewBody builds the manual review notification body.
func (c *NotificationCreator) buildManualReviewBody(rr *remediationv1.RemediationRequest, ctx *ManualReviewContext) string {
	body := fmt.Sprintf(`⚠️ **Manual Review Required**

**Signal**: %s
**Severity**: %s

---

**Failure Source**: %s
**Reason**: %s`,
		rr.Spec.SignalName,
		rr.Spec.Severity,
		ctx.Source,
		ctx.Reason,
	)

	if ctx.SubReason != "" {
		body += fmt.Sprintf("\n**Sub-Reason**: %s", ctx.SubReason)
	}

	if ctx.Message != "" {
		body += fmt.Sprintf("\n\n**Details**:\n%s", ctx.Message)
	}

	if ctx.RootCauseAnalysis != "" {
		body += fmt.Sprintf("\n\n**Root Cause Analysis**:\n%s", ctx.RootCauseAnalysis)
	}

	if len(ctx.Warnings) > 0 {
		body += "\n\n**Warnings**:"
		for _, w := range ctx.Warnings {
			body += fmt.Sprintf("\n- %s", w)
		}
	}

	// Add WorkflowExecution-specific context
	if ctx.Source == ManualReviewSourceWorkflowExecution {
		if ctx.RetryCount > 0 || ctx.MaxRetries > 0 {
			body += fmt.Sprintf("\n\n**Retry Information**:\n- Retries attempted: %d/%d", ctx.RetryCount, ctx.MaxRetries)
		}
		if ctx.LastExitCode != 0 {
			body += fmt.Sprintf("\n- Last exit code: %d", ctx.LastExitCode)
		}
		if ctx.PreviousExecution != "" {
			body += fmt.Sprintf("\n- Previous execution: %s", ctx.PreviousExecution)
		}
	}

	body += `

---

**Action Required**: Please investigate this remediation failure and take appropriate action.

**Options**:
1. Fix the underlying issue and re-trigger the signal
2. Manually apply the remediation
3. Mark as resolved if no action is needed`

	return body
}
