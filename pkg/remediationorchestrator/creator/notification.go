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
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/api/meta"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	emconditions "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// formatTargetResource formats a ResourceIdentifier for notification bodies.
// Returns a multi-line string with Kind, Name, and Namespace (if namespaced).
func formatTargetResource(r remediationv1.ResourceIdentifier) string {
	result := fmt.Sprintf("- **Kind**: %s\n- **Name**: %s", r.Kind, r.Name)
	if r.Namespace != "" {
		result += fmt.Sprintf("\n- **Namespace**: %s", r.Namespace)
	}
	return result
}

// resolveNotificationTargetResource returns the best available target resource for notifications (#305).
// Prefers AI's RemediationTarget (from LLM investigation) when the Gateway's TargetResource
// is "Unknown" (e.g., when owner resolution failed for kube-state-metrics alerts).
func resolveNotificationTargetResource(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) remediationv1.ResourceIdentifier {
	if rr.Spec.TargetResource.Kind != "Unknown" {
		return rr.Spec.TargetResource
	}
	if ai != nil && ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.RemediationTarget != nil {
		ar := ai.Status.RootCauseAnalysis.RemediationTarget
		return remediationv1.ResourceIdentifier{
			Kind:      ar.Kind,
			Name:      ar.Name,
			Namespace: ar.Namespace,
		}
	}
	return rr.Spec.TargetResource
}

// NotificationCreator creates NotificationRequest CRDs for the Remediation Orchestrator.
// Reference: BR-ORCH-001 (approval notification), BR-ORCH-034 (bulk duplicate), BR-ORCH-036 (manual review), BR-ORCH-045 (completion)
type NotificationCreator struct {
	client      client.Client
	scheme      *runtime.Scheme
	metrics     *metrics.Metrics
	clusterName string
	clusterUUID string
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

// SetClusterIdentity sets the cluster name and UUID for inclusion in notification bodies.
// Issue #615: Setter injection avoids modifying the NewNotificationCreator constructor signature.
func (c *NotificationCreator) SetClusterIdentity(name, uuid string) {
	c.clusterName = name
	c.clusterUUID = uuid
}

// FormatRemediationLine returns a formatted remediation identification line for notification bodies.
// Returns empty string when name is empty (graceful degradation).
// Issue #626: Enables operators to trace notifications back to specific CRD pipeline chain.
func FormatRemediationLine(rrName string) string {
	if rrName == "" {
		return ""
	}
	return fmt.Sprintf("**Remediation**: %s\n\n", rrName)
}

// FormatStatusLine returns a formatted status line for notification bodies.
// Issue #628: Single standardized **Status** label across all notification types.
// Display enum: Remediated | Pending Approval | Timed Out | Manual Review Required | Self-Resolved | Duplicate Handled
func FormatStatusLine(status string) string {
	return fmt.Sprintf("**Status**: %s\n\n", status)
}

// FormatClusterLine returns a formatted cluster identification line for notification bodies.
// Returns empty string when both name and uuid are empty (graceful degradation).
func FormatClusterLine(clusterName, clusterUUID string) string {
	if clusterName == "" && clusterUUID == "" {
		return ""
	}
	if clusterUUID == "" {
		return fmt.Sprintf("**Cluster**: %s\n\n", clusterName)
	}
	if clusterName == "" {
		return fmt.Sprintf("**Cluster**: (%s)\n\n", clusterUUID)
	}
	return fmt.Sprintf("**Cluster**: %s (%s)\n\n", clusterName, clusterUUID)
}

// existingNotification checks whether a NotificationRequest named `name` already exists
// (idempotent create). Returns (true, nil) when found and reusable, (false, nil) when not
// found (caller should proceed to build+create), or (false, err) on unexpected API error.
// kind labels the notification type in log messages (e.g. "Approval", "Completion").
func (c *NotificationCreator) existingNotification(ctx context.Context, logger logr.Logger, name, namespace, kind string) (bool, error) {
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, existing)
	if err == nil {
		logger.Info(kind+" notification already exists, reusing", "name", name)
		return true, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing NotificationRequest")
		return false, fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}
	return false, nil
}

// persistNotification validates rr's owner-reference eligibility, sets the controller
// reference for cascade deletion (BR-ORCH-031), and creates nr -- tolerating a
// concurrent-create race (IsAlreadyExists) as a successful idempotent outcome.
// kind labels the notification type in log messages.
func (c *NotificationCreator) persistNotification(ctx context.Context, logger logr.Logger, rr *remediationv1.RemediationRequest, nr *notificationv1.NotificationRequest, name, kind string) (string, error) {
	// Gap 2.1: Prevents orphaned child CRDs if RR not properly persisted
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference")
		return "", fmt.Errorf("failed to set owner reference: RemediationRequest UID is required but empty")
	}

	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := c.client.Create(ctx, nr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Info(kind+" NotificationRequest already exists (concurrent create), reusing", "name", name)
			return name, nil
		}
		logger.Error(err, "Failed to create "+kind+" NotificationRequest")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}
	return name, nil
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
	exists, err := c.existingNotification(ctx, logger, name, rr.Namespace, "Approval")
	if err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}

	// Build NotificationRequest for approval
	nr := c.buildApprovalNotificationRequest(name, rr, ai)

	result, err := c.persistNotification(ctx, logger, rr, nr, name, "Approval")
	if err != nil {
		return "", err
	}

	logger.Info("Created approval NotificationRequest",
		"name", result,
		"approvalReason", ai.Status.ApprovalReason,
	)

	// BR-ORCH-035: Caller (reconciler) appends to rr.Status.NotificationRequestRefs
	return result, nil
}

// buildApprovalNotificationRequest constructs the NotificationRequest object for an
// approval notification (BR-ORCH-001). Split from CreateApprovalNotification for funlen.
// #260: Channels resolved by NT routing rules (BR-NOT-065), not set by RO
// API Contract: Uses Subject/Body (not Title/Message), Context + Extensions
func (c *NotificationCreator) buildApprovalNotificationRequest(name string, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) *notificationv1.NotificationRequest {
	return &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
		},
		Spec: notificationv1.NotificationRequestSpec{
			// BR-NOT-064: Parent reference for audit correlation and lineage tracking
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			ClusterID: rr.Spec.ClusterID,
			Type:      notificationv1.NotificationTypeApproval,
			Priority:  c.mapPriority(ai.Spec.AnalysisRequest.SignalContext.BusinessPriority),
			Severity:  rr.Spec.Severity,
			Subject:   fmt.Sprintf("Approval Required: %s", rr.Spec.SignalName),
			Body:      c.buildApprovalBody(rr, ai, resolveNotificationTargetResource(rr, ai)),
			Context: &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: rr.Name,
					AIAnalysis:         ai.Name,
				},
				Workflow: &notificationv1.WorkflowContext{
					SelectedWorkflow: ai.Status.SelectedWorkflow.WorkflowID,
					Confidence:       fmt.Sprintf("%.2f", ai.Status.SelectedWorkflow.Confidence),
				},
				Analysis: &notificationv1.AnalysisContext{
					ApprovalReason: ai.Status.ApprovalReason,
				},
			},
		},
	}
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
func (c *NotificationCreator) buildApprovalBody(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis, target remediationv1.ResourceIdentifier) string {
	rootCause := ai.Status.RootCause
	if ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.Summary != "" {
		rootCause = ai.Status.RootCauseAnalysis.Summary
	}

	approvalReason := ai.Status.ApprovalReason
	if ai.Status.ApprovalContext != nil && ai.Status.ApprovalContext.Reason != "" {
		approvalReason = ai.Status.ApprovalContext.Reason
	}

	workflowLabel := ai.Status.SelectedWorkflow.WorkflowID
	if ai.Status.SelectedWorkflow.ActionType != "" {
		workflowLabel = fmt.Sprintf("%s (%s)", ai.Status.SelectedWorkflow.ActionType, ai.Status.SelectedWorkflow.WorkflowID)
	}

	body := "Remediation requires approval:\n\n" +
		FormatStatusLine("Pending Approval") +
		fmt.Sprintf(`**Signal**: %s
**Severity**: %s

**Affected Resource**:
%s

**Root Cause Analysis**:
%s

**Confidence**: %.0f%%

**Proposed Workflow**: %s

**Approval Reason**: %s`,
			rr.Spec.SignalName,
			rr.Spec.Severity,
			formatTargetResource(target),
			rootCause,
			ai.Status.SelectedWorkflow.Confidence*100,
			workflowLabel,
			approvalReason,
		)

	body += "\n\nPlease review and approve/reject the remediation."

	if ai.Status.SelectedWorkflow.Rationale != "" {
		body += fmt.Sprintf("\n\n**Selection Rationale**:\n%s", ai.Status.SelectedWorkflow.Rationale)
	}

	return FormatClusterLine(c.clusterName, c.clusterUUID) + FormatRemediationLine(rr.Name) + body
}

// CreateCompletionNotification creates a NotificationRequest for successful remediation completion (BR-ORCH-045).
// This is triggered when WorkflowExecution completes successfully and the RemediationRequest transitions to Completed.
// #318: ea is optional -- nil produces "Verification: not available" (graceful degradation).
// Reference: BR-ORCH-045 (completion notification), BR-ORCH-031 (cascade deletion), BR-ORCH-035 (ref tracking)
func (c *NotificationCreator) CreateCompletionNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
	executionEngine string,
	ea *eav1.EffectivenessAssessment,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
	)

	// Generate deterministic name
	name := fmt.Sprintf("nr-completion-%s", rr.Name)

	// Check if already exists (idempotency)
	exists, err := c.existingNotification(ctx, logger, name, rr.Namespace, "Completion")
	if err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}

	// Build NotificationRequest for completion
	nr, workflowID := c.buildCompletionNotificationRequest(name, rr, ai, executionEngine, ea)

	result, err := c.persistNotification(ctx, logger, rr, nr, name, "Completion")
	if err != nil {
		return "", err
	}

	logger.Info("Created completion NotificationRequest",
		"name", result,
		"workflowId", workflowID,
	)

	// BR-ORCH-035: Caller (reconciler) appends to rr.Status.NotificationRequestRefs
	return result, nil
}

// completionContent holds the derived content fields for a completion notification,
// resolved once from AIAnalysis/EffectivenessAssessment and shared between the
// notification body and its structured Context. Extracted for funlen.
type completionContent struct {
	RootCause        string
	WorkflowID       string
	ActionType       string
	Rationale        string
	VerificationText string
	VerificationCtx  *notificationv1.VerificationContext
}

// resolveCompletionContent derives the completion notification content fields from
// AIAnalysis and the (optional) EffectivenessAssessment.
// #318: ea is optional -- nil produces "Verification: not available" (graceful degradation).
func resolveCompletionContent(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis, ea *eav1.EffectivenessAssessment) completionContent {
	rootCause := ai.Status.RootCause
	if ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.Summary != "" {
		rootCause = ai.Status.RootCauseAnalysis.Summary
	}

	var workflowID, actionType, rationale string
	if ai.Status.SelectedWorkflow != nil {
		workflowID = ai.Status.SelectedWorkflow.WorkflowID
		actionType = ai.Status.SelectedWorkflow.ActionType
		rationale = ai.Status.SelectedWorkflow.Rationale
	}

	// #318 + #546: Build verification summary from EA (with RR for hash degradation)
	verificationText, verificationCtx := BuildVerificationSummary(ea, rr)

	return completionContent{
		RootCause:        rootCause,
		WorkflowID:       workflowID,
		ActionType:       actionType,
		Rationale:        rationale,
		VerificationText: verificationText,
		VerificationCtx:  verificationCtx,
	}
}

// buildCompletionNotificationRequest constructs the NotificationRequest object for a
// completion notification (BR-ORCH-045). Split from CreateCompletionNotification for funlen.
// #260: Channels resolved by NT routing rules (BR-NOT-065), not set by RO
// API Contract: Uses Subject/Body (not Title/Message), Context + Extensions
// Returns the built request plus workflowID (used by the caller for logging).
func (c *NotificationCreator) buildCompletionNotificationRequest(
	name string,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
	executionEngine string,
	ea *eav1.EffectivenessAssessment,
) (*notificationv1.NotificationRequest, string) {
	// Issue #518: executionEngine is now passed as parameter (sourced from WFE status).
	content := resolveCompletionContent(rr, ai, ea)

	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
		},
		Spec: notificationv1.NotificationRequestSpec{
			// BR-NOT-064: Parent reference for audit correlation and lineage tracking
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			ClusterID: rr.Spec.ClusterID,
			Type:      notificationv1.NotificationTypeCompletion,
			Priority:  notificationv1.NotificationPriorityLow,
			Severity:  rr.Spec.Severity,
			Subject:   fmt.Sprintf("Remediation Completed: %s", rr.Spec.SignalName),
			Body: c.buildCompletionBody(rr, completionBodyParams{
				RootCause:        content.RootCause,
				WorkflowID:       content.WorkflowID,
				ExecutionEngine:  executionEngine,
				ActionType:       content.ActionType,
				Rationale:        content.Rationale,
				Target:           resolveNotificationTargetResource(rr, ai),
				VerificationText: content.VerificationText,
			}),
			Context: &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: rr.Name,
					AIAnalysis:         ai.Name,
				},
				Workflow: &notificationv1.WorkflowContext{
					WorkflowID:      content.WorkflowID,
					ExecutionEngine: executionEngine,
				},
				Analysis: &notificationv1.AnalysisContext{
					RootCause: content.RootCause,
					Outcome:   string(rr.Status.Outcome),
				},
				Verification: content.VerificationCtx,
			},
		},
	}
	return nr, content.WorkflowID
}

// completionBodyParams groups the completion-notification content fields
// for buildCompletionBody. Extracted per AGENTS.md's 8+-param
// Options-pattern rule.
type completionBodyParams struct {
	RootCause        string
	WorkflowID       string
	ExecutionEngine  string
	ActionType       string
	Rationale        string
	Target           remediationv1.ResourceIdentifier
	VerificationText string
}

// buildCompletionBody builds the completion notification body.
// #318: verificationText is appended as a "Verification Results" section before the closing tagline.
func (c *NotificationCreator) buildCompletionBody(rr *remediationv1.RemediationRequest, p completionBodyParams) string {
	rootCause, workflowID, executionEngine, actionType, rationale, target, verificationText :=
		p.RootCause, p.WorkflowID, p.ExecutionEngine, p.ActionType, p.Rationale, p.Target, p.VerificationText

	workflowLabel := workflowID
	if actionType != "" {
		workflowLabel = fmt.Sprintf("%s (%s)", actionType, workflowID)
	}

	// Deprecated: **Outcome** retained for one release (Issue #628). Use **Status** for canonical status.
	body := "Remediation Completed Successfully\n\n" +
		FormatStatusLine(string(rr.Status.Outcome)) +
		fmt.Sprintf(`**Outcome**: %s

**Signal**: %s
**Severity**: %s

**Affected Resource**:
%s

**Root Cause Analysis**:
%s

**Workflow Executed**: %s
**Execution Engine**: %s`,
			rr.Status.Outcome,
			rr.Spec.SignalName,
			rr.Spec.Severity,
			formatTargetResource(target),
			rootCause,
			workflowLabel,
			executionEngine,
		)

	if rationale != "" {
		body += fmt.Sprintf("\n\n**Selection Rationale**:\n%s", rationale)
	}

	if verificationText != "" {
		body += fmt.Sprintf("\n\n**Verification Results**:\n%s", verificationText)
	}

	body += "\n\nThis incident was automatically detected and remediated by Kubernaut."
	return FormatClusterLine(c.clusterName, c.clusterUUID) + FormatRemediationLine(rr.Name) + body
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
	exists, err := c.existingNotification(ctx, logger, name, rr.Namespace, "Bulk")
	if err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}

	// Build bulk notification
	// API Contract: Uses Subject/Body (not Title/Message), Context + Extensions
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
		},
		Spec: notificationv1.NotificationRequestSpec{
			// BR-NOT-064: Parent reference for audit correlation and lineage tracking
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			ClusterID: rr.Spec.ClusterID,
			Type:      notificationv1.NotificationTypeSimple,
			Priority:  notificationv1.NotificationPriorityLow,
			Severity:  "info",
			Subject:   fmt.Sprintf("Remediation Completed with %d Duplicates", rr.Status.DuplicateCount),
			Body:      c.buildBulkDuplicateBody(rr),
			Context: &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: rr.Name,
				},
				Dedup: &notificationv1.DedupContext{
					DuplicateCount: fmt.Sprintf("%d", rr.Status.DuplicateCount),
				},
			},
		},
	}

	result, err := c.persistNotification(ctx, logger, rr, nr, name, "Bulk")
	if err != nil {
		return "", err
	}

	logger.Info("Created bulk duplicate NotificationRequest", "name", result)

	// BR-ORCH-035: Caller (reconciler) appends to rr.Status.NotificationRequestRefs
	return result, nil
}

// buildBulkDuplicateBody builds the bulk duplicate notification body.
func (c *NotificationCreator) buildBulkDuplicateBody(rr *remediationv1.RemediationRequest) string {
	// Deprecated: **Result** retained for one release (Issue #628). Use **Status** for canonical status.
	return FormatClusterLine(c.clusterName, c.clusterUUID) + FormatRemediationLine(rr.Name) +
		"Remediation completed successfully.\n\n" +
		FormatStatusLine("Duplicate Handled") +
		fmt.Sprintf(`**Signal**: %s
**Result**: %s

**Affected Resource**:
%s

**Duplicate Remediations**: %d

All duplicate signals have been handled by this remediation.`,
			rr.Spec.SignalName,
			rr.Status.OverallPhase,
			formatTargetResource(rr.Spec.TargetResource),
			rr.Status.DuplicateCount,
		)
}

// ========================================
// MANUAL REVIEW NOTIFICATIONS (BR-ORCH-036)
// ========================================

// rcaSentinels lists known sentinel values that KA's result_parser.py generates
// when RCA extraction fails. These are not meaningful for operators and should be
// omitted from notification bodies. Issue #588.
var rcaSentinels = []string{
	"Failed to parse RCA",
	"No structured RCA found",
}

// isRCASentinel returns true if the given RCA summary is a known sentinel value
// that should not be displayed to operators. Issue #588.
// Case-insensitive with whitespace trimming for resilience against KA formatting changes.
func isRCASentinel(rca string) bool {
	trimmed := strings.TrimSpace(rca)
	for _, sentinel := range rcaSentinels {
		if strings.EqualFold(trimmed, sentinel) {
			return true
		}
	}
	return false
}

// ManualReviewContext provides context for manual review notifications.
// Used by both AIAnalysis and WorkflowExecution failure scenarios.
type ManualReviewContext struct {
	// Source indicates which component triggered the manual review
	Source notificationv1.ReviewSourceType
	// Reason is the high-level failure reason (e.g., "WorkflowResolutionFailed", "ExhaustedRetries", "HumanReviewRequired")
	Reason string
	// SubReason provides granular detail (e.g., "WorkflowNotFound", "LowConfidence")
	SubReason string
	// HumanReviewReason (BR-HAPI-197): Explicit reason from KA when needs_human_review=true
	// Maps to AIAnalysis.Status.HumanReviewReason enum (workflow_not_found, rca_incomplete, etc.)
	HumanReviewReason string
	// Message is a human-readable description of the failure
	Message string
	// RootCauseAnalysis if available (from AIAnalysis)
	RootCauseAnalysis string
	// Warnings if available (from AIAnalysis)
	Warnings []string

	// AlignmentVerdict from AIAnalysis CRD when shadow agent alignment is enabled.
	// BR-AI-601, #1076: When non-nil, rendered prominently in notification body.
	AlignmentVerdict *aianalysisv1.AlignmentVerdictStatus

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

// EscalationContext provides context for escalation notifications.
// Used by terminal failure transitions (transitionToFailed, transitionToFailedTerminal).
type EscalationContext struct {
	FailurePhase  string
	FailureReason string
	BlockReason   string
	Message       string
}

// BlockNotificationContext provides context for block-reason notifications.
// Used by handleBlocked to create NRs for non-IneffectiveChain block reasons.
// Reference: BR-ORCH-036 GAP-6 (#810), BR-ORCH-042.5
type BlockNotificationContext struct {
	BlockReason  string
	BlockMessage string
}

// CreateEscalationNotification creates an Escalation NotificationRequest for terminal failures.
// This is triggered by transitionToFailed and transitionToFailedTerminal when no ManualReview
// NR was already created by the calling handler.
// Reference: BR-ORCH-036 (notification gap remediation), BR-ORCH-031 (cascade deletion)
func (c *NotificationCreator) CreateEscalationNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	escCtx *EscalationContext,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"failurePhase", escCtx.FailurePhase,
	)

	name := fmt.Sprintf("nr-escalation-%s", rr.Name)

	exists, err := c.existingNotification(ctx, logger, name, rr.Namespace, "Escalation")
	if err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}

	subject := fmt.Sprintf("🚨 Remediation Failed: %s (phase: %s)", rr.Spec.SignalName, escCtx.FailurePhase)
	body := c.buildEscalationBody(rr, escCtx)

	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
		},
		Spec: notificationv1.NotificationRequestSpec{
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			ClusterID: rr.Spec.ClusterID,
			Type:      notificationv1.NotificationTypeEscalation,
			Priority:  notificationv1.NotificationPriorityHigh,
			Severity:  rr.Spec.Severity,
			Subject:   subject,
			Body:      body,
		},
	}

	result, err := c.persistNotification(ctx, logger, rr, nr, name, "Escalation")
	if err != nil {
		return "", err
	}

	logger.Info("Created escalation NotificationRequest", "name", result)
	return result, nil
}

// CreateBlockNotification creates a NotificationRequest when an RR enters the Blocked phase.
// Escalation NRs (High priority) for persistent blocks: ConsecutiveFailures, UnmanagedResource.
// StatusUpdate NRs (Low priority) for transient blocks: DuplicateInProgress, ResourceBusy,
// RecentlyRemediated, ExponentialBackoff.
// Reference: BR-ORCH-036 GAP-6 (#810), BR-ORCH-042.5
func (c *NotificationCreator) CreateBlockNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	blockCtx *BlockNotificationContext,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"blockReason", blockCtx.BlockReason,
	)

	name := fmt.Sprintf("nr-block-%s-%s", strings.ToLower(blockCtx.BlockReason), rr.Name)

	exists, err := c.existingNotification(ctx, logger, name, rr.Namespace, "Block")
	if err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}

	nrType, priority := c.mapBlockReasonToTypeAndPriority(blockCtx.BlockReason)
	subject := fmt.Sprintf("Remediation Blocked: %s (%s)", rr.Spec.SignalName, blockCtx.BlockReason)
	body := c.buildBlockNotificationBody(rr, blockCtx)

	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
		},
		Spec: notificationv1.NotificationRequestSpec{
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			ClusterID: rr.Spec.ClusterID,
			Type:      nrType,
			Priority:  priority,
			Severity:  rr.Spec.Severity,
			Subject:   subject,
			Body:      body,
		},
	}

	result, err := c.persistNotification(ctx, logger, rr, nr, name, "Block")
	if err != nil {
		return "", err
	}

	logger.Info("Created block NotificationRequest", "name", result, "type", nrType, "priority", priority)
	return result, nil
}

// mapBlockReasonToTypeAndPriority determines NR type and priority based on block reason.
// Persistent blocks needing operator investigation → Escalation, High.
// Transient/auto-clearing blocks → StatusUpdate, Low.
func (c *NotificationCreator) mapBlockReasonToTypeAndPriority(reason string) (notificationv1.NotificationType, notificationv1.NotificationPriority) {
	switch remediationv1.BlockReason(reason) {
	case remediationv1.BlockReasonConsecutiveFailures, remediationv1.BlockReasonUnmanagedResource:
		return notificationv1.NotificationTypeEscalation, notificationv1.NotificationPriorityHigh
	default:
		return notificationv1.NotificationTypeStatusUpdate, notificationv1.NotificationPriorityLow
	}
}

func (c *NotificationCreator) buildBlockNotificationBody(rr *remediationv1.RemediationRequest, blockCtx *BlockNotificationContext) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Remediation for signal '%s' has been blocked.\n\n", rr.Spec.SignalName))
	b.WriteString(fmt.Sprintf("**Block Reason:** %s\n", blockCtx.BlockReason))
	if blockCtx.BlockMessage != "" {
		b.WriteString(fmt.Sprintf("**Details:** %s\n", blockCtx.BlockMessage))
	}
	b.WriteString(fmt.Sprintf("\n**Target:** %s/%s/%s\n",
		rr.Spec.TargetResource.Kind, rr.Spec.TargetResource.Namespace, rr.Spec.TargetResource.Name))
	return b.String()
}

func (c *NotificationCreator) buildEscalationBody(rr *remediationv1.RemediationRequest, escCtx *EscalationContext) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Remediation for signal '%s' has failed.\n\n", rr.Spec.SignalName))
	b.WriteString(fmt.Sprintf("**Failure Phase:** %s\n", escCtx.FailurePhase))
	if escCtx.FailureReason != "" {
		b.WriteString(fmt.Sprintf("**Reason:** %s\n", escCtx.FailureReason))
	}
	if escCtx.BlockReason != "" {
		b.WriteString(fmt.Sprintf("**Block Reason:** %s\n", escCtx.BlockReason))
	}
	if escCtx.Message != "" {
		b.WriteString(fmt.Sprintf("**Details:** %s\n", escCtx.Message))
	}
	b.WriteString(fmt.Sprintf("\n**Target:** %s/%s/%s\n", rr.Spec.TargetResource.Kind, rr.Spec.TargetResource.Namespace, rr.Spec.TargetResource.Name))
	return b.String()
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
	exists, err := c.existingNotification(ctx, logger, name, rr.Namespace, "Manual review")
	if err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}

	// Determine priority based on source and reason (per BR-ORCH-036 priority mapping)
	priority := c.mapManualReviewPriority(reviewCtx)

	// Build NotificationRequest for manual review
	nr := c.buildManualReviewNotificationRequest(name, rr, reviewCtx, priority)

	result, err := c.persistNotification(ctx, logger, rr, nr, name, "Manual review")
	if err != nil {
		return "", err
	}

	logger.Info("Created manual review NotificationRequest",
		"name", result,
		"priority", priority,
	)

	// BR-ORCH-035: Caller (reconciler) appends to rr.Status.NotificationRequestRefs
	return result, nil
}

// buildManualReviewNotificationRequest constructs the NotificationRequest object for a
// manual review notification (BR-ORCH-036). Split from CreateManualReviewNotification for funlen.
// #260: Channels resolved by NT routing rules (BR-NOT-065), not set by RO
func (c *NotificationCreator) buildManualReviewNotificationRequest(
	name string,
	rr *remediationv1.RemediationRequest,
	reviewCtx *ManualReviewContext,
	priority notificationv1.NotificationPriority,
) *notificationv1.NotificationRequest {
	return &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
		},
		Spec: notificationv1.NotificationRequestSpec{
			// BR-NOT-064: Parent reference for audit correlation and lineage tracking
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			ClusterID:    rr.Spec.ClusterID,
			Type:         notificationv1.NotificationTypeManualReview,
			Priority:     priority,
			Severity:     rr.Spec.Severity,
			ReviewSource: reviewCtx.Source,
			Subject:      fmt.Sprintf("⚠️ Manual Review Required: %s", rr.Spec.SignalName),
			Body:         c.buildManualReviewBody(rr, reviewCtx),
			Context:      c.buildManualReviewContext(rr, reviewCtx),
		},
	}
}

// mapManualReviewPriority maps manual review context to notification priority.
// Per BR-ORCH-036 priority mapping:
// - WE failures (ExhaustedRetries, PreviousExecutionFailed, ExecutionFailure) → critical
// - AI WorkflowNotFound, ImageMismatch, ParameterValidationFailed, LLMParsingError → high
// - AI NoMatchingWorkflows, LowConfidence, InvestigationInconclusive → medium
// - BR-ORCH-036 v3.0: AI infrastructure failures (MaxRetriesExceeded, TransientError, PermanentError) → high
func (c *NotificationCreator) mapManualReviewPriority(ctx *ManualReviewContext) notificationv1.NotificationPriority {
	if ctx.Source == notificationv1.ReviewSourceWorkflowExecution {
		return notificationv1.NotificationPriorityCritical
	}

	if ctx.Source == notificationv1.ReviewSourceRoutingEngine {
		return notificationv1.NotificationPriorityHigh
	}

	// AIAnalysis failures - map by SubReason
	switch ctx.SubReason {
	case "alignment_check_failed":
		return notificationv1.NotificationPriorityCritical
	// Workflow resolution failures
	case "WorkflowNotFound", "ImageMismatch", "ParameterValidationFailed", "LLMParsingError":
		return notificationv1.NotificationPriorityHigh
	case "NoMatchingWorkflows", "LowConfidence", "InvestigationInconclusive":
		return notificationv1.NotificationPriorityMedium
	// BR-ORCH-036 v3.0: Infrastructure failures (APIError, Timeout, etc.)
	case "MaxRetriesExceeded", "TransientError", "PermanentError":
		return notificationv1.NotificationPriorityHigh
	default:
		return notificationv1.NotificationPriorityMedium
	}
}

// buildManualReviewContext builds typed notification context for manual review notifications.
func (c *NotificationCreator) buildManualReviewContext(rr *remediationv1.RemediationRequest, ctx *ManualReviewContext) *notificationv1.NotificationContext {
	nCtx := &notificationv1.NotificationContext{
		Lineage: &notificationv1.LineageContext{
			RemediationRequest: rr.Name,
		},
		Review: &notificationv1.ReviewContext{
			Reason: ctx.Reason,
		},
	}
	if ctx.SubReason != "" {
		nCtx.Review.SubReason = ctx.SubReason
	}
	if ctx.HumanReviewReason != "" {
		nCtx.Review.HumanReviewReason = ctx.HumanReviewReason
	}
	if ctx.RootCauseAnalysis != "" {
		nCtx.Review.RootCauseAnalysis = ctx.RootCauseAnalysis
	}
	if ctx.AlignmentVerdict != nil {
		nCtx.Review.AlignmentVerdict = ctx.AlignmentVerdict.Result
		nCtx.Review.CircuitBreakerActivated = ctx.AlignmentVerdict.CircuitBreakerActivated
	}
	if ctx.Source == notificationv1.ReviewSourceWorkflowExecution {
		nCtx.Execution = &notificationv1.ExecutionContext{}
		if ctx.RetryCount > 0 || ctx.MaxRetries > 0 {
			nCtx.Execution.RetryCount = fmt.Sprintf("%d", ctx.RetryCount)
			nCtx.Execution.MaxRetries = fmt.Sprintf("%d", ctx.MaxRetries)
		}
		if ctx.LastExitCode != 0 {
			nCtx.Execution.LastExitCode = fmt.Sprintf("%d", ctx.LastExitCode)
		}
		if ctx.PreviousExecution != "" {
			nCtx.Execution.PreviousExecution = ctx.PreviousExecution
		}
	}
	return nCtx
}

// buildManualReviewBody builds the manual review notification body.
func (c *NotificationCreator) buildManualReviewBody(rr *remediationv1.RemediationRequest, ctx *ManualReviewContext) string {
	body := "⚠️ **Manual Review Required**\n\n" +
		FormatStatusLine("Manual Review Required") +
		fmt.Sprintf(`**Signal**: %s
**Severity**: %s

**Affected Resource**:
%s

---

**Action Required**: Please investigate this remediation failure and take appropriate action.

**Options**:
1. Fix the underlying issue and re-trigger the signal
2. Manually apply the remediation
3. Mark as resolved if no action is needed

---

**Failure Source**: %s
**Reason**: %s`,
			rr.Spec.SignalName,
			rr.Spec.Severity,
			formatTargetResource(rr.Spec.TargetResource),
			ctx.Source,
			ctx.Reason,
		)

	if ctx.SubReason != "" {
		body += fmt.Sprintf("\n**Sub-Reason**: %s", ctx.SubReason)
	}

	if ctx.Message != "" {
		body += fmt.Sprintf("\n\n**Details**:\n%s", ctx.Message)
	}

	body += renderAlignmentVerdictSection(ctx)
	body += renderRootCauseSection(ctx)
	body += renderWarningsSection(ctx)
	body += renderRetryInfoSection(ctx)

	return FormatClusterLine(c.clusterName, c.clusterUUID) + FormatRemediationLine(rr.Name) + body
}

// renderRootCauseSection renders the root cause analysis section of a manual review body.
// Returns empty string when no RCA is available or it is a known sentinel value (Issue #588).
func renderRootCauseSection(ctx *ManualReviewContext) string {
	if ctx.RootCauseAnalysis == "" || isRCASentinel(ctx.RootCauseAnalysis) {
		return ""
	}
	if ctx.AlignmentVerdict != nil && ctx.AlignmentVerdict.CircuitBreakerActivated {
		return "\n\n**Primary LLM Analysis** (relegated — review with caution):\n" + ctx.RootCauseAnalysis
	}
	return fmt.Sprintf("\n\n**Root Cause Analysis**:\n%s", ctx.RootCauseAnalysis)
}

// renderWarningsSection renders the warnings list section of a manual review body.
// Returns empty string when there are no warnings.
func renderWarningsSection(ctx *ManualReviewContext) string {
	if len(ctx.Warnings) == 0 {
		return ""
	}
	body := "\n\n**Warnings**:"
	for _, w := range ctx.Warnings {
		body += fmt.Sprintf("\n- %s", w)
	}
	return body
}

// renderRetryInfoSection renders the WorkflowExecution-specific retry information section
// of a manual review body. Returns empty string for non-WorkflowExecution sources.
func renderRetryInfoSection(ctx *ManualReviewContext) string {
	if ctx.Source != notificationv1.ReviewSourceWorkflowExecution {
		return ""
	}
	var body string
	if ctx.RetryCount > 0 || ctx.MaxRetries > 0 {
		body += fmt.Sprintf("\n\n**Retry Information**:\n- Retries attempted: %d/%d", ctx.RetryCount, ctx.MaxRetries)
	}
	if ctx.LastExitCode != 0 {
		body += fmt.Sprintf("\n- Last exit code: %d", ctx.LastExitCode)
	}
	if ctx.PreviousExecution != "" {
		body += fmt.Sprintf("\n- Previous execution: %s", ctx.PreviousExecution)
	}
	return body
}

const maxFindingsInBody = 20

// renderAlignmentVerdictSection renders the shadow agent alignment verdict section
// for insertion into the notification body. Returns empty string when no verdict.
func renderAlignmentVerdictSection(ctx *ManualReviewContext) string {
	av := ctx.AlignmentVerdict
	if av == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n---\n\n")

	switch {
	case av.CircuitBreakerActivated:
		b.WriteString("**Shadow Agent Alignment Verdict**: SUSPICIOUS (Circuit Breaker Activated)\n\n")
		b.WriteString("Investigation was terminated early after the shadow agent detected suspicious LLM behavior.\n")
	case av.Result == "suspicious":
		b.WriteString("**Shadow Agent Alignment Verdict**: SUSPICIOUS\n")
	default:
		b.WriteString("**Shadow Agent Alignment Verdict**: ALIGNED\n")
		b.WriteString("All investigation steps passed alignment checks. No suspicious behavior detected.\n")
		return b.String()
	}

	if av.Summary != "" {
		b.WriteString(fmt.Sprintf("\n**Shadow Agent Summary**:\n%s\n", av.Summary))
	}

	if len(av.Findings) > 0 {
		b.WriteString("\n**Findings**:\n")
		limit := len(av.Findings)
		if limit > maxFindingsInBody {
			limit = maxFindingsInBody
		}
		for _, f := range av.Findings[:limit] {
			if f.Tool != "" {
				b.WriteString(fmt.Sprintf("- Step %d (%s, tool: %s): %s\n", f.StepIndex, f.StepKind, f.Tool, f.Explanation))
			} else {
				b.WriteString(fmt.Sprintf("- Step %d (%s): %s\n", f.StepIndex, f.StepKind, f.Explanation))
			}
		}
		if remaining := len(av.Findings) - maxFindingsInBody; remaining > 0 {
			b.WriteString(fmt.Sprintf("- ... and %d more\n", remaining))
		}
	}

	return b.String()
}

// ========================================
// BR-ORCH-037 AC-037-08: SELF-RESOLVED NOTIFICATION (Issue #590)
// ========================================

// CreateSelfResolvedNotification creates a status-update NotificationRequest when a signal
// self-resolves and the operator has opted in via notifications.notifySelfResolved.
// This is informational only — priority is always low, and channels are resolved by
// routing rules (BR-NOT-065), not by the RO.
// Reference: BR-ORCH-037 AC-037-08, BR-ORCH-031 (cascade deletion), BR-ORCH-035 (ref tracking)
func (c *NotificationCreator) CreateSelfResolvedNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"aiAnalysis", ai.Name,
	)

	name := fmt.Sprintf("nr-self-resolved-%s", rr.Name)

	exists, err := c.existingNotification(ctx, logger, name, rr.Namespace, "Self-resolved")
	if err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}

	nr := c.buildSelfResolvedNotificationRequest(name, rr, ai)

	result, err := c.persistNotification(ctx, logger, rr, nr, name, "Self-resolved")
	if err != nil {
		return "", err
	}

	logger.Info("Created self-resolved NotificationRequest", "name", result)
	return result, nil
}

// buildSelfResolvedNotificationRequest constructs the NotificationRequest object for a
// self-resolved notification (BR-ORCH-037 AC-037-08). Split from
// CreateSelfResolvedNotification for funlen.
func (c *NotificationCreator) buildSelfResolvedNotificationRequest(name string, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) *notificationv1.NotificationRequest {
	rootCause := ai.Status.RootCause
	if ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.Summary != "" {
		rootCause = ai.Status.RootCauseAnalysis.Summary
	}

	return &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
		},
		Spec: notificationv1.NotificationRequestSpec{
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			ClusterID: rr.Spec.ClusterID,
			Type:      notificationv1.NotificationTypeStatusUpdate,
			Priority:  notificationv1.NotificationPriorityLow,
			Severity:  rr.Spec.Severity,
			Subject:   fmt.Sprintf("ℹ️ Auto-Resolved: %s", rr.Spec.SignalName),
			Body:      c.buildSelfResolvedBody(rr, ai, rootCause, resolveNotificationTargetResource(rr, ai)),
			Context: &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: rr.Name,
					AIAnalysis:         ai.Name,
				},
				Analysis: &notificationv1.AnalysisContext{
					RootCause: rootCause,
					Outcome:   "NoActionRequired",
				},
			},
		},
	}
}

// buildSelfResolvedBody builds the informational notification body for self-resolved signals.
func (c *NotificationCreator) buildSelfResolvedBody(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
	rootCause string,
	target remediationv1.ResourceIdentifier,
) string {
	body := "Signal was investigated but no remediation was needed.\n\n" +
		FormatStatusLine("Self-Resolved") +
		fmt.Sprintf(`**Signal**: %s
**Severity**: %s

**Affected Resource**:
%s`,
			rr.Spec.SignalName,
			rr.Spec.Severity,
			formatTargetResource(target),
		)

	if ai.Status.Message != "" {
		body += fmt.Sprintf("\n\n**AI Assessment**:\n%s", ai.Status.Message)
	}

	if rootCause != "" && !isRCASentinel(rootCause) {
		body += fmt.Sprintf("\n\n**Root Cause Analysis**:\n%s", rootCause)
	}

	body += "\n\nNo action was taken. This notification is for audit purposes only."
	return FormatClusterLine(c.clusterName, c.clusterUUID) + FormatRemediationLine(rr.Name) + body
}

// ========================================
// #621: TIMEOUT BODY BUILDERS
// Extracted from reconciler.go inline body construction.
// Shared cluster line + RR name prefix for all timeout notifications.
// ========================================

// BuildGlobalTimeoutBody constructs the notification body for global timeout events.
// Issue #621: Prepends cluster line and RR name for operator traceability.
func (c *NotificationCreator) BuildGlobalTimeoutBody(
	signalName, rrName, timeoutPhase, timeoutDuration, startTime, timeoutTime string,
) string {
	body := "Remediation request has exceeded the global timeout and requires manual intervention.\n\n" +
		FormatStatusLine("Timed Out") +
		fmt.Sprintf(`**Signal**: %s
**Timeout Phase**: %s
**Timeout Duration**: %s
**Started**: %s
**Timed Out**: %s

The remediation was in %s phase when it timed out. Please investigate why the remediation did not complete within the expected timeframe.`,
			signalName, timeoutPhase, timeoutDuration, startTime, timeoutTime, timeoutPhase,
		)
	return FormatClusterLine(c.clusterName, c.clusterUUID) + FormatRemediationLine(rrName) + body
}

// BuildPhaseTimeoutBody constructs the notification body for per-phase timeout events.
// Issue #621: Prepends cluster line and RR name for operator traceability.
func (c *NotificationCreator) BuildPhaseTimeoutBody(
	signalName, rrName, phase, phaseTimeout, startTime, timeoutTime string,
) string {
	body := "Remediation phase has exceeded timeout and requires investigation.\n\n" +
		FormatStatusLine("Timed Out") +
		fmt.Sprintf(`**Signal**: %s
**Phase**: %s
**Phase Timeout**: %s
**Started**: %s
**Timed Out**: %s

The %s phase did not complete within the expected timeframe. Please investigate why this phase is taking longer than expected.`,
			signalName, phase, phaseTimeout, startTime, timeoutTime, phase,
		)
	return FormatClusterLine(c.clusterName, c.clusterUUID) + FormatRemediationLine(rrName) + body
}

// ========================================
// #318: VERIFICATION SUMMARY BUILDER
// ========================================

var verificationMessages = map[string]struct {
	summary string
	outcome string
}{
	eav1.AssessmentReasonFull:              {"Verification passed: all checks confirmed the remediation was effective.", "passed"},
	eav1.AssessmentReasonPartial:           {"Verification partially completed: some checks could not be performed for this resource type.", "partial"},
	eav1.AssessmentReasonSpecDrift:         {"Verification inconclusive: the resource spec was modified by an external entity after remediation.", "inconclusive"},
	eav1.AssessmentReasonAlertDecayTimeout: {"Verification inconclusive: related alerts persisted beyond the assessment window.", "inconclusive"},
	eav1.AssessmentReasonMetricsTimedOut:   {"Verification partially completed: metrics were not available before the assessment deadline.", "partial"},
	eav1.AssessmentReasonExpired:           {"Verification could not be completed: the assessment window expired.", "unavailable"},
	eav1.AssessmentReasonNoExecution:       {"Verification skipped: no workflow execution was found.", "unavailable"},
}

// BuildVerificationSummary maps an EA and RR to a human-readable verification summary
// and a typed VerificationContext for programmatic routing.
// Returns ("Verification: not available.", {Assessed:false, Outcome:"unavailable"}) when EA is nil.
// Issue #546: Checks RR PreRemediationHashCaptured and EA PostHashCaptured conditions
// to detect hash-capture degradation and include actionable guidance.
func BuildVerificationSummary(ea *eav1.EffectivenessAssessment, rr *remediationv1.RemediationRequest) (string, *notificationv1.VerificationContext) {
	if ea == nil {
		return "Verification: not available.", &notificationv1.VerificationContext{
			Assessed: false,
			Outcome:  "unavailable",
		}
	}

	reason := ea.Status.AssessmentReason
	entry, ok := verificationMessages[reason]
	if !ok {
		return fmt.Sprintf("Verification: unknown assessment reason %q.", reason), &notificationv1.VerificationContext{
			Assessed: true,
			Outcome:  "unavailable",
			Reason:   reason,
		}
	}

	summary := entry.summary
	bullets := BuildComponentBullets(ea)
	if bullets != "" {
		summary += "\n" + bullets
	}

	ctx := &notificationv1.VerificationContext{
		Assessed: true,
		Outcome:  entry.outcome,
		Reason:   reason,
		Summary:  entry.summary,
	}

	// Issue #596: "full" means all components were assessed, not that all passed.
	// When any component bullet is emitted (score < 1.0), replace the affirmative
	// "passed" message with a qualified "completed" message to avoid contradiction.
	if reason == eav1.AssessmentReasonFull && bullets != "" {
		qualified := "Verification completed: all checks were performed, but some indicate the remediation was not fully effective."
		summary = qualified + "\n" + bullets
		ctx.Outcome = "completed"
		ctx.Summary = qualified
	}

	// Issue #546: Check for hash-capture degradation
	degradedReasons := collectHashDegradationReasons(rr, ea)
	if len(degradedReasons) > 0 {
		ctx.Degraded = true
		ctx.DegradedReason = strings.Join(degradedReasons, "; ")
		summary += "\n\n" + buildDegradationWarning(degradedReasons)
	}

	return summary, ctx
}

// collectHashDegradationReasons checks RR and EA conditions for hash-capture failures (Issue #546).
func collectHashDegradationReasons(rr *remediationv1.RemediationRequest, ea *eav1.EffectivenessAssessment) []string {
	var reasons []string

	if rr != nil {
		cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionPreRemediationHashCaptured)
		if cond != nil && cond.Status == metav1.ConditionFalse {
			reasons = append(reasons, fmt.Sprintf("Pre-remediation hash: %s", cond.Message))
		}
	}

	if ea != nil {
		cond := meta.FindStatusCondition(ea.Status.Conditions, emconditions.ConditionPostHashCaptured)
		if cond != nil && cond.Status == metav1.ConditionFalse {
			reasons = append(reasons, fmt.Sprintf("Post-remediation hash: %s", cond.Message))
		}
	}

	return reasons
}

// buildDegradationWarning produces an operator-facing warning with actionable RBAC guidance.
func buildDegradationWarning(reasons []string) string {
	var b strings.Builder
	b.WriteString("**Effectiveness Assessment: Degraded**\n")
	for _, r := range reasons {
		b.WriteString(fmt.Sprintf("- %s\n", r))
	}
	b.WriteString("\nThe effectiveness assessment could not reliably compare pre- and post-remediation resource state.\n")
	b.WriteString("Action: Grant the controller ServiceAccount read access to the affected resource type, or verify the 'view' ClusterRoleBinding is present.")
	return b.String()
}

// BuildComponentBullets produces bullet lines for non-passing assessed components.
// Omits components that were not assessed or that passed (score >= 1.0).
// Returns empty string when all assessed components pass (e.g., "full" reason).
func BuildComponentBullets(ea *eav1.EffectivenessAssessment) string {
	if ea == nil {
		return ""
	}

	var bullets []string
	c := ea.Status.Components

	if c.HealthAssessed && c.HealthScore != nil && *c.HealthScore < 1.0 {
		bullets = append(bullets, "- Pod health: not recovered")
	}
	if c.AlertAssessed && c.AlertScore != nil && *c.AlertScore < 1.0 {
		bullets = append(bullets, "- Related alerts: still firing")
	}
	if c.HashComputed && c.PostRemediationSpecHash != "" && c.CurrentSpecHash != "" &&
		c.PostRemediationSpecHash != c.CurrentSpecHash {
		bullets = append(bullets, "- Resource integrity: spec modified externally after remediation")
	}
	if c.MetricsAssessed && c.MetricsScore != nil && *c.MetricsScore < 1.0 {
		switch {
		case *c.MetricsScore >= 0.5:
			bullets = append(bullets, fmt.Sprintf("- Metrics: partial improvement (score: %.2f)", *c.MetricsScore))
		case *c.MetricsScore > 0.0:
			bullets = append(bullets, fmt.Sprintf("- Metrics: minimal improvement (score: %.2f)", *c.MetricsScore))
		default:
			bullets = append(bullets, "- Metrics: no improvement detected")
		}
	}

	return strings.Join(bullets, "\n")
}
