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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
)

// ========================================
// NOTIFICATION STATUS TRACKING (BR-ORCH-029/030)
// ========================================

// trackNotificationStatus updates RemediationRequest status based on NotificationRequest phase.
// This method is called during each reconcile loop to keep notification status in sync.
//
// TDD REFACTOR (Day 2): Enhanced with defensive programming and error handling.
//
// Business Requirements:
// - BR-ORCH-029: User-initiated notification cancellation
// - BR-ORCH-030: Notification status tracking
//
// CRITICAL: This method NEVER changes overallPhase - only notification tracking fields.
func (r *Reconciler) trackNotificationStatus(ctx context.Context, rr *remediationv1.RemediationRequest) error {
	// Defensive: Validate input
	if rr == nil {
		return fmt.Errorf("RemediationRequest cannot be nil")
	}

	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"notificationRefsCount", len(rr.Status.NotificationRequestRefs),
	)

	// If no notification refs, nothing to track
	if len(rr.Status.NotificationRequestRefs) == 0 {
		logger.V(1).Info("No notification refs to track")
		return nil
	}

	// Defensive: Limit iterations to prevent infinite loops
	maxRefs := 10
	refsToProcess := rr.Status.NotificationRequestRefs
	if len(refsToProcess) > maxRefs {
		logger.Info("Too many notification refs, limiting tracking",
			"refCount", len(refsToProcess),
			"maxRefs", maxRefs,
		)
		refsToProcess = refsToProcess[:maxRefs]
	}

	// Track status for each NotificationRequest
	// Note: In v1.0, typically only one notification per RR
	// v1.1 may have multiple (e.g., approval + escalation)
	for _, ref := range refsToProcess {
		// Defensive: Validate ref
		if ref.Name == "" || ref.Namespace == "" {
			logger.Info("Invalid notification ref, skipping tracking",
				"refName", ref.Name,
				"refNamespace", ref.Namespace,
			)
			continue
		}

		notif := &notificationv1.NotificationRequest{}
		err := r.client.Get(ctx, client.ObjectKey{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		}, notif)

		if err != nil {
			if apierrors.IsNotFound(err) {
				// NotificationRequest was deleted
				// Distinguish cascade deletion from user cancellation
				logger.V(1).Info("NotificationRequest not found (deleted)",
					"notificationName", ref.Name,
				)

				// Handle deletion (BR-ORCH-029)
				if err := r.handleNotificationDeletion(ctx, rr); err != nil {
					return err
				}
				continue
			}
			// Other error - log with context and continue
			logger.Error(err, "Failed to fetch NotificationRequest",
				"notificationName", ref.Name,
				"namespace", ref.Namespace,
				"uid", ref.UID,
			)
			continue
		}

		// Update status based on NotificationRequest phase (BR-ORCH-030)
		logger.V(1).Info("Updating notification status from NotificationRequest",
			"notificationName", notif.Name,
			"notificationPhase", notif.Status.Phase,
			"currentNotificationStatus", rr.Status.NotificationStatus,
		)
		if err := r.updateNotificationStatusFromPhase(ctx, rr, notif); err != nil {
			return err
		}
	}

	return nil
}

// handleNotificationDeletion handles NotificationRequest deletion events.
// Distinguishes cascade deletion (expected cleanup) from user cancellation (intentional).
//
// REFACTOR-RO-001: Using retry helper for status updates
// Reference: BR-ORCH-029 (user cancellation), BR-ORCH-031 (cascade cleanup)
func (r *Reconciler) handleNotificationDeletion(ctx context.Context, rr *remediationv1.RemediationRequest) error {
	// Update status with retry helper (REFACTOR-RO-001)
	return helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		// Delegate to NotificationHandler
		return r.notificationHandler.HandleNotificationRequestDeletion(ctx, rr)
	})
}

// updateNotificationStatusFromPhase updates RemediationRequest status based on NotificationRequest phase.
// Maps NotificationRequest delivery status to RemediationRequest notification tracking.
//
// REFACTOR-RO-001: Using retry helper for status updates
// Reference: BR-ORCH-030 (notification status tracking)
func (r *Reconciler) updateNotificationStatusFromPhase(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	notif *notificationv1.NotificationRequest,
) error {
	// Update status with retry helper (REFACTOR-RO-001)
	return helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		// Delegate to NotificationHandler
		return r.notificationHandler.UpdateNotificationStatus(ctx, rr, notif)
	})
}
