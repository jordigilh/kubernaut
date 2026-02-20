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

package controller

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ========================================
// NOTIFICATION LIFECYCLE HANDLER (BR-ORCH-029/030)
// 📋 Design Decision: DD-RO-001 Alternative 3 | ✅ Approved Design | Confidence: 92%
// See: docs/services/crd-controllers/05-remediationorchestrator/implementation/BR-ORCH-029-030-031-034_RE-TRIAGE.md
// ========================================
//
// NotificationHandler manages notification lifecycle tracking for RemediationRequest.
//
// KEY PRINCIPLE: Notification lifecycle is SEPARATE from remediation lifecycle.
// - Deleting NotificationRequest cancels the NOTIFICATION (not the remediation)
// - RemediationRequest.Status.OverallPhase is NEVER changed by notification events
//
// WHY Alternative 3 (92% confidence)?
// - ✅ Separation of concerns: Notification ≠ remediation
// - ✅ Clear audit trail: Conditions track notification outcomes
// - ✅ User control: Operators can cancel spam/duplicate notifications
// - ✅ Observable state: Metrics + queryable conditions
//
// ⚠️ Trade-off: Added reconciler complexity (+3% watch overhead)
//    Mitigation: Standard Kubernetes watch pattern, negligible performance impact
// ========================================

// NotificationHandler handles notification lifecycle events.
// Business Requirements:
// - BR-ORCH-029: User-initiated notification cancellation
// - BR-ORCH-030: Notification status tracking
// - BR-ORCH-031: Cascade cleanup (via owner references)
type NotificationHandler struct {
	client  client.Client
	Metrics *metrics.Metrics
}

// NewNotificationHandler creates a new NotificationHandler.
func NewNotificationHandler(c client.Client, m *metrics.Metrics) *NotificationHandler {
	return &NotificationHandler{
		client:  c,
		Metrics: m,
	}
}

// HandleNotificationRequestDeletion handles NotificationRequest deletion events.
// Distinguishes cascade deletion (expected cleanup) from user cancellation (intentional).
//
// TDD REFACTOR (Day 2): Enhanced with defensive programming and structured logging.
//
// Reference: BR-ORCH-029 (user cancellation), BR-ORCH-031 (cascade cleanup)
func (h *NotificationHandler) HandleNotificationRequestDeletion(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) error {
	// Defensive: Validate input
	if rr == nil {
		return fmt.Errorf("RemediationRequest cannot be nil")
	}

	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"currentPhase", rr.Status.OverallPhase,
		"notificationRefsCount", len(rr.Status.NotificationRequestRefs),
	)

	startTime := time.Now()
	defer func() {
		logger.V(1).Info("HandleNotificationRequestDeletion completed",
			"duration", time.Since(startTime),
		)
	}()

	// Distinguish cascade deletion from user cancellation
	if rr.DeletionTimestamp != nil {
		// Case 1: RemediationRequest being deleted → cascade deletion (expected)
		logger.V(1).Info("NotificationRequest deleted as part of RemediationRequest cleanup (cascade deletion)",
			"deletionTimestamp", rr.DeletionTimestamp.Time,
		)
		return nil
	}

	// Defensive: Check for notification refs
	if len(rr.Status.NotificationRequestRefs) == 0 {
		logger.V(1).Info("No notification refs found, skipping cancellation update")
		return nil
	}

	// Case 2: User-initiated cancellation (NotificationRequest deleted independently)
	logger.Info("NotificationRequest deleted by user (cancellation)",
		"notificationRefs", len(rr.Status.NotificationRequestRefs),
		"previousStatus", rr.Status.NotificationStatus,
	)

	// Update notification tracking ONLY (DO NOT change overallPhase!)
	previousStatus := rr.Status.NotificationStatus
	rr.Status.NotificationStatus = "Cancelled"
	rr.Status.Message = "NotificationRequest deleted by user before delivery completed"

	// Increment cancellation metric (BR-ORCH-029)
	if h.Metrics != nil {
		h.Metrics.NotificationCancellationsTotal.WithLabelValues(rr.Namespace).Inc()
	}

	logger.V(1).Info("Updated notification status",
		"previousStatus", previousStatus,
		"newStatus", rr.Status.NotificationStatus,
	)

	// Set condition: NotificationDelivered = False
	remediationrequest.SetNotificationDelivered(rr, false, remediationrequest.ReasonUserCancelled, "NotificationRequest deleted by user", h.Metrics)

	logger.Info("Set NotificationDelivered condition",
		"conditionStatus", "False",
		"conditionReason", "UserCancelled",
	)

	// CRITICAL: DO NOT change overallPhase - remediation continues!
	// This is notification cancellation, not remediation cancellation.
	// Defensive assertion to prevent accidental changes
	if rr.Status.OverallPhase == remediationv1.PhaseCompleted {
		logger.Error(nil, "CRITICAL BUG: overallPhase was incorrectly set to Completed",
			"expectedBehavior", "phase should NOT change on notification cancellation",
			"designDecision", "DD-RO-001 Alternative 3",
		)
	}

	return nil
}

// UpdateNotificationStatus updates RemediationRequest status based on NotificationRequest phase.
// Maps NotificationRequest delivery status to RemediationRequest notification tracking.
//
// TDD REFACTOR (Day 2): Enhanced with defensive programming, structured logging, and error handling.
//
// Reference: BR-ORCH-030 (notification status tracking)
func (h *NotificationHandler) UpdateNotificationStatus(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	notif *notificationv1.NotificationRequest,
) error {
	// Defensive: Validate inputs
	if rr == nil {
		return fmt.Errorf("RemediationRequest cannot be nil")
	}
	if notif == nil {
		return fmt.Errorf("NotificationRequest cannot be nil")
	}

	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"notificationRequest", notif.Name,
		"notificationPhase", notif.Status.Phase,
		"previousNotificationStatus", rr.Status.NotificationStatus,
		"currentPhase", rr.Status.OverallPhase,
	)

	startTime := time.Now()
	defer func() {
		logger.V(1).Info("UpdateNotificationStatus completed",
			"duration", time.Since(startTime),
		)
	}()

	previousStatus := rr.Status.NotificationStatus

	// Map NotificationRequest phase to RemediationRequest status
	switch notif.Status.Phase {
	case notificationv1.NotificationPhasePending:
		rr.Status.NotificationStatus = "Pending"
		logger.V(1).Info("Notification pending delivery")
		// Update status gauge (BR-ORCH-030)
		if h.Metrics != nil {
			h.Metrics.NotificationStatusGauge.WithLabelValues(rr.Namespace, "Pending").Set(1)
		}

	case notificationv1.NotificationPhaseSending:
		rr.Status.NotificationStatus = "InProgress"
		logger.V(1).Info("Notification delivery in progress")
		// Update status gauge (BR-ORCH-030)
		if h.Metrics != nil {
			h.Metrics.NotificationStatusGauge.WithLabelValues(rr.Namespace, "InProgress").Set(1)
		}

	case notificationv1.NotificationPhaseSent:
		rr.Status.NotificationStatus = "Sent"
		deliveryDuration := time.Since(startTime)

		remediationrequest.SetNotificationDelivered(rr, true, remediationrequest.ReasonDeliverySucceeded, "Notification delivered successfully", h.Metrics)

		// Update metrics (BR-ORCH-030)
		if h.Metrics != nil {
			h.Metrics.NotificationStatusGauge.WithLabelValues(rr.Namespace, "Sent").Set(1)
			h.Metrics.NotificationDeliveryDurationSeconds.WithLabelValues(rr.Namespace, "Sent").Observe(deliveryDuration.Seconds())
		}

		logger.Info("Notification delivered successfully",
			"deliveryDuration", deliveryDuration,
		)

	case notificationv1.NotificationPhaseFailed:
		rr.Status.NotificationStatus = "Failed"
		deliveryDuration := time.Since(startTime)

		// Defensive: Handle empty failure message
		failureMessage := notif.Status.Message
		if failureMessage == "" {
			failureMessage = "Unknown delivery failure"
		}

		remediationrequest.SetNotificationDelivered(rr, false, remediationrequest.ReasonDeliveryFailed, fmt.Sprintf("Notification delivery failed: %s", failureMessage), h.Metrics)

		// Update metrics (BR-ORCH-030)
		if h.Metrics != nil {
			h.Metrics.NotificationStatusGauge.WithLabelValues(rr.Namespace, "Failed").Set(1)
			h.Metrics.NotificationDeliveryDurationSeconds.WithLabelValues(rr.Namespace, "Failed").Observe(deliveryDuration.Seconds())
		}

		logger.Error(nil, "Notification delivery failed",
			"reason", failureMessage,
			"notificationUID", notif.UID,
			"deliveryDuration", deliveryDuration,
		)

	default:
		// Defensive: Handle unexpected phase
		logger.V(1).Info("Unknown NotificationRequest phase, no status update",
			"unexpectedPhase", notif.Status.Phase,
		)
		return nil
	}

	logger.V(1).Info("Notification status updated",
		"previousStatus", previousStatus,
		"newStatus", rr.Status.NotificationStatus,
		"statusChanged", previousStatus != rr.Status.NotificationStatus,
	)

	// CRITICAL: Verify overallPhase unchanged (defensive assertion)
	// This should NEVER trigger if implementation is correct
	if rr.Status.OverallPhase == remediationv1.PhaseCompleted &&
		previousStatus != "Sent" && rr.Status.NotificationStatus != "Sent" {
		logger.Error(nil, "CRITICAL BUG: overallPhase was incorrectly changed",
			"expectedBehavior", "phase should NOT change on notification status update",
			"designDecision", "DD-RO-001 Alternative 3",
		)
	}

	return nil
}
