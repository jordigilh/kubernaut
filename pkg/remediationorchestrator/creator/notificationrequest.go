// Package creator provides child CRD creation logic for the Remediation Orchestrator.
//
// Business Requirements:
// - BR-ORCH-001: Approval notification creation
// - BR-ORCH-029: Completion notification handling
// - BR-ORCH-030: Failure notification handling
// - BR-ORCH-031: Cascade deletion via owner references
// - BR-ORCH-034: Duplicate notification handling
package creator

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// NotificationRequestCreator creates NotificationRequest CRDs for various scenarios.
// Reference: BR-ORCH-001 (approval), BR-ORCH-029-034 (notification handling)
type NotificationRequestCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewNotificationRequestCreator creates a new NotificationRequestCreator.
func NewNotificationRequestCreator(c client.Client, s *runtime.Scheme) *NotificationRequestCreator {
	return &NotificationRequestCreator{
		client: c,
		scheme: s,
	}
}

// NotificationType represents the type of notification to create.
type NotificationType string

const (
	// NotificationTypeApprovalRequired for BR-ORCH-001
	NotificationTypeApprovalRequired NotificationType = "approval_required"

	// NotificationTypeCompleted for BR-ORCH-029
	NotificationTypeCompleted NotificationType = "completed"

	// NotificationTypeFailed for BR-ORCH-030
	NotificationTypeFailed NotificationType = "failed"

	// NotificationTypeSkipped for BR-ORCH-034
	NotificationTypeSkipped NotificationType = "skipped"

	// NotificationTypeTimedOut for BR-ORCH-027
	NotificationTypeTimedOut NotificationType = "timed_out"
)

// CreateApprovalNotification creates a NotificationRequest for approval workflow.
// Reference: BR-ORCH-001
func (c *NotificationRequestCreator) CreateApprovalNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"notificationType", NotificationTypeApprovalRequired,
	)

	// Generate deterministic name
	name := fmt.Sprintf("nr-approval-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("NotificationRequest already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}

	// Build NotificationRequest
	nr := c.buildApprovalNotification(rr, ai, name)

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create NotificationRequest CRD")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	logger.Info("Created approval NotificationRequest CRD", "name", name)
	return name, nil
}

// CreateCompletionNotification creates a NotificationRequest for successful completion.
// Reference: BR-ORCH-029
func (c *NotificationRequestCreator) CreateCompletionNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) (string, error) {
	return c.createStatusNotification(ctx, rr, NotificationTypeCompleted)
}

// CreateFailureNotification creates a NotificationRequest for failure scenarios.
// Reference: BR-ORCH-030
func (c *NotificationRequestCreator) CreateFailureNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	failureReason string,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"notificationType", NotificationTypeFailed,
	)

	name := fmt.Sprintf("nr-failed-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("NotificationRequest already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}

	nr := c.buildFailureNotification(rr, name, failureReason)

	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := c.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create NotificationRequest CRD")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	logger.Info("Created failure NotificationRequest CRD", "name", name)
	return name, nil
}

// CreateSkippedNotification creates a NotificationRequest when workflow is skipped.
// Reference: BR-ORCH-034
func (c *NotificationRequestCreator) CreateSkippedNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	skipReason string,
	duplicateOf string,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"notificationType", NotificationTypeSkipped,
	)

	name := fmt.Sprintf("nr-skipped-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("NotificationRequest already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}

	nr := c.buildSkippedNotification(rr, name, skipReason, duplicateOf)

	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := c.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create NotificationRequest CRD")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	logger.Info("Created skipped NotificationRequest CRD", "name", name, "duplicateOf", duplicateOf)
	return name, nil
}

// createStatusNotification is a helper for completion and timeout notifications
func (c *NotificationRequestCreator) createStatusNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	notifType NotificationType,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"notificationType", notifType,
	)

	name := fmt.Sprintf("nr-%s-%s", notifType, rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("NotificationRequest already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}

	nr := c.buildStatusNotification(rr, name, notifType)

	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := c.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create NotificationRequest CRD")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	logger.Info("Created status NotificationRequest CRD", "name", name)
	return name, nil
}

// buildApprovalNotification constructs the approval NotificationRequest.
func (c *NotificationRequestCreator) buildApprovalNotification(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
	name string,
) *notificationv1.NotificationRequest {
	// Build subject
	subject := fmt.Sprintf("🔔 Approval Required: Remediation for %s", rr.Spec.SignalName)

	// Build body with approval context
	body := c.buildApprovalBody(rr, ai)

	return &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			// Labels for BR-NOT-065 routing rules to match
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/component":           "notification",
				"kubernaut.ai/notification-type":   string(NotificationTypeApprovalRequired),
				// Routing labels (BR-NOT-065)
				"kubernaut.ai/severity":    rr.Spec.Severity,
				"kubernaut.ai/environment": rr.Spec.Environment,
				"kubernaut.ai/priority":    rr.Spec.Priority,
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeEscalation,
			Priority: c.mapPriorityToNotification(rr.Spec.Priority),
			// Recipients and Channels determined by Notification Service routing rules (BR-NOT-065)
			// based on labels: notification-type, severity, environment, priority
			Subject: subject,
			Body:    body,
			Metadata: map[string]string{
				"remediationRequestName": rr.Name,
				"namespace":              rr.Namespace,
				"signalName":             rr.Spec.SignalName,
				"severity":               rr.Spec.Severity,
				"environment":            rr.Spec.Environment,
				"priority":               rr.Spec.Priority,
				"notificationType":       string(NotificationTypeApprovalRequired),
				"aiAnalysisName":         ai.Name,
			},
			RetentionDays: 7,
		},
	}
}

// buildApprovalBody constructs the notification body for approval requests.
func (c *NotificationRequestCreator) buildApprovalBody(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) string {
	var sb strings.Builder

	sb.WriteString("A remediation workflow requires human approval before execution.\n\n")

	sb.WriteString("**Signal Details**\n")
	sb.WriteString(fmt.Sprintf("- Signal: %s\n", rr.Spec.SignalName))
	sb.WriteString(fmt.Sprintf("- Severity: %s\n", rr.Spec.Severity))
	sb.WriteString(fmt.Sprintf("- Environment: %s\n", rr.Spec.Environment))
	sb.WriteString(fmt.Sprintf("- Priority: %s\n", rr.Spec.Priority))
	sb.WriteString("\n")

	sb.WriteString("**Target Resource**\n")
	sb.WriteString(fmt.Sprintf("- Kind: %s\n", rr.Spec.TargetResource.Kind))
	sb.WriteString(fmt.Sprintf("- Name: %s\n", rr.Spec.TargetResource.Name))
	sb.WriteString(fmt.Sprintf("- Namespace: %s\n", rr.Spec.TargetResource.Namespace))
	sb.WriteString("\n")

	if ai.Status.ApprovalReason != "" {
		sb.WriteString("**Approval Reason**\n")
		sb.WriteString(fmt.Sprintf("%s\n\n", ai.Status.ApprovalReason))
	}

	if ai.Status.SelectedWorkflow != nil {
		sw := ai.Status.SelectedWorkflow
		sb.WriteString("**Recommended Workflow**\n")
		sb.WriteString(fmt.Sprintf("- Workflow ID: %s\n", sw.WorkflowID))
		sb.WriteString(fmt.Sprintf("- Version: %s\n", sw.Version))
		sb.WriteString(fmt.Sprintf("- Confidence: %.0f%%\n", sw.Confidence*100))
		if sw.Rationale != "" {
			sb.WriteString(fmt.Sprintf("- Rationale: %s\n", sw.Rationale))
		}
	}

	return sb.String()
}

// buildFailureNotification constructs a failure NotificationRequest.
func (c *NotificationRequestCreator) buildFailureNotification(
	rr *remediationv1.RemediationRequest,
	name string,
	failureReason string,
) *notificationv1.NotificationRequest {
	subject := fmt.Sprintf("❌ Remediation Failed: %s", rr.Spec.SignalName)

	body := fmt.Sprintf(`Remediation workflow failed for signal: %s

**Failure Reason**
%s

**Signal Details**
- Environment: %s
- Priority: %s
- Target: %s/%s/%s

Please investigate and take appropriate action.`,
		rr.Spec.SignalName,
		failureReason,
		rr.Spec.Environment,
		rr.Spec.Priority,
		rr.Spec.TargetResource.Namespace,
		rr.Spec.TargetResource.Kind,
		rr.Spec.TargetResource.Name,
	)

	return &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			// Labels for BR-NOT-065 routing rules to match
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/component":           "notification",
				"kubernaut.ai/notification-type":   string(NotificationTypeFailed),
				// Routing labels (BR-NOT-065)
				"kubernaut.ai/severity":    rr.Spec.Severity,
				"kubernaut.ai/environment": rr.Spec.Environment,
				"kubernaut.ai/priority":    rr.Spec.Priority,
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeEscalation,
			Priority: notificationv1.NotificationPriorityHigh,
			// Recipients and Channels determined by Notification Service routing rules (BR-NOT-065)
			Subject: subject,
			Body:    body,
			Metadata: map[string]string{
				"remediationRequestName": rr.Name,
				"namespace":              rr.Namespace,
				"notificationType":       string(NotificationTypeFailed),
				"failureReason":          failureReason,
			},
			RetentionDays: 30, // Keep failure notifications longer
		},
	}
}

// buildSkippedNotification constructs a skipped NotificationRequest.
func (c *NotificationRequestCreator) buildSkippedNotification(
	rr *remediationv1.RemediationRequest,
	name string,
	skipReason string,
	duplicateOf string,
) *notificationv1.NotificationRequest {
	subject := fmt.Sprintf("⏭️ Remediation Skipped: %s (duplicate)", rr.Spec.SignalName)

	body := fmt.Sprintf(`Remediation was skipped due to resource lock deduplication.

**Skip Reason**
%s

**Duplicate Of**
%s

**Signal Details**
- Signal: %s
- Environment: %s
- Target: %s/%s/%s

This is expected behavior when multiple signals target the same resource.`,
		skipReason,
		duplicateOf,
		rr.Spec.SignalName,
		rr.Spec.Environment,
		rr.Spec.TargetResource.Namespace,
		rr.Spec.TargetResource.Kind,
		rr.Spec.TargetResource.Name,
	)

	return &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			// Labels for BR-NOT-065 routing rules to match
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/component":           "notification",
				"kubernaut.ai/notification-type":   string(NotificationTypeSkipped),
				// Routing labels (BR-NOT-065)
				"kubernaut.ai/severity":    rr.Spec.Severity,
				"kubernaut.ai/environment": rr.Spec.Environment,
				"kubernaut.ai/priority":    rr.Spec.Priority,
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeStatusUpdate,
			Priority: notificationv1.NotificationPriorityLow,
			// Recipients and Channels determined by Notification Service routing rules (BR-NOT-065)
			Subject: subject,
			Body:    body,
			Metadata: map[string]string{
				"remediationRequestName": rr.Name,
				"namespace":              rr.Namespace,
				"notificationType":       string(NotificationTypeSkipped),
				"skipReason":             skipReason,
				"duplicateOf":            duplicateOf,
			},
			RetentionDays: 7,
		},
	}
}

// buildStatusNotification constructs a generic status NotificationRequest.
func (c *NotificationRequestCreator) buildStatusNotification(
	rr *remediationv1.RemediationRequest,
	name string,
	notifType NotificationType,
) *notificationv1.NotificationRequest {
	var subject, body string
	var priority notificationv1.NotificationPriority
	var notifCategory notificationv1.NotificationType

	switch notifType {
	case NotificationTypeCompleted:
		subject = fmt.Sprintf("✅ Remediation Completed: %s", rr.Spec.SignalName)
		body = fmt.Sprintf("Remediation workflow completed successfully for signal: %s\n\nTarget: %s/%s/%s",
			rr.Spec.SignalName,
			rr.Spec.TargetResource.Namespace,
			rr.Spec.TargetResource.Kind,
			rr.Spec.TargetResource.Name)
		priority = notificationv1.NotificationPriorityMedium
		notifCategory = notificationv1.NotificationTypeStatusUpdate
	case NotificationTypeTimedOut:
		subject = fmt.Sprintf("⏱️ Remediation Timed Out: %s", rr.Spec.SignalName)
		body = fmt.Sprintf("Remediation workflow timed out for signal: %s\n\nTarget: %s/%s/%s\n\nPlease investigate.",
			rr.Spec.SignalName,
			rr.Spec.TargetResource.Namespace,
			rr.Spec.TargetResource.Kind,
			rr.Spec.TargetResource.Name)
		priority = notificationv1.NotificationPriorityHigh
		notifCategory = notificationv1.NotificationTypeEscalation
	default:
		subject = fmt.Sprintf("Remediation Update: %s", rr.Spec.SignalName)
		body = fmt.Sprintf("Remediation status update for signal: %s", rr.Spec.SignalName)
		priority = notificationv1.NotificationPriorityMedium
		notifCategory = notificationv1.NotificationTypeStatusUpdate
	}

	return &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			// Labels for BR-NOT-065 routing rules to match
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/component":           "notification",
				"kubernaut.ai/notification-type":   string(notifType),
				// Routing labels (BR-NOT-065)
				"kubernaut.ai/severity":    rr.Spec.Severity,
				"kubernaut.ai/environment": rr.Spec.Environment,
				"kubernaut.ai/priority":    rr.Spec.Priority,
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notifCategory,
			Priority: priority,
			// Recipients and Channels determined by Notification Service routing rules (BR-NOT-065)
			Subject: subject,
			Body:    body,
			Metadata: map[string]string{
				"remediationRequestName": rr.Name,
				"namespace":              rr.Namespace,
				"notificationType":       string(notifType),
			},
			RetentionDays: 7,
		},
	}
}

// mapPriorityToNotification maps signal priority (free-text) to notification priority (enum).
// The NotificationRequest CRD requires an enum value (critical, high, medium, low) while
// RemediationRequest.Spec.Priority is free-text (e.g., "P1", "CRITICAL", "HIGH").
// This mapping ensures CRD validation passes while preserving the original priority in metadata.
func (c *NotificationRequestCreator) mapPriorityToNotification(priority string) notificationv1.NotificationPriority {
	switch strings.ToUpper(priority) {
	case "P1", "CRITICAL":
		return notificationv1.NotificationPriorityCritical
	case "P2", "HIGH":
		return notificationv1.NotificationPriorityHigh
	case "P3", "MEDIUM":
		return notificationv1.NotificationPriorityMedium
	default:
		return notificationv1.NotificationPriorityLow
	}
}
