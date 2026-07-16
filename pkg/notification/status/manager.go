package status

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationphase "github.com/jordigilh/kubernaut/pkg/notification/phase"
)

// Manager handles NotificationRequest status updates
type Manager struct {
	client    client.Client
	apiReader client.Reader // Bypasses cache for fresh reads (DD-STATUS-001)
}

// NewManager creates a new status manager
//
// DD-STATUS-001: API Reader for Cache-Bypassed Refetches
// The apiReader parameter is critical for resolving cache consistency issues
// during rapid status updates (e.g., retries). It reads directly from the API
// server instead of the controller-runtime cache, preventing lost updates.
//
// See: docs/services/crd-controllers/06-notification/design/DD-STATUS-001-api-reader-cache-bypass.md
func NewManager(client client.Client, apiReader client.Reader) *Manager {
	return &Manager{
		client:    client,
		apiReader: apiReader,
	}
}

// RecordDeliveryAttempt records a delivery attempt in the NotificationRequest status
// Satisfies BR-NOT-051: Complete Audit Trail
//
// This method uses retry logic to handle optimistic locking conflicts.
// See: docs/development/business-requirements/DEVELOPMENT_GUIDELINES.md
func (m *Manager) RecordDeliveryAttempt(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, attempt notificationv1alpha1.DeliveryAttempt) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion (bypasses cache - DD-STATUS-001)
		if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(notification), notification); err != nil {
			return fmt.Errorf("failed to refetch notification: %w", err)
		}

		// 2. Append the attempt to the delivery attempts list
		notification.Status.DeliveryAttempts = append(notification.Status.DeliveryAttempts, attempt)

		// 3. Update counters
		notification.Status.TotalAttempts++

		// BR-NOT-051: Calculate counters based on UNIQUE CHANNELS, not total attempts
		// DD-E2E-003: SuccessfulDeliveries/FailedDeliveries track channel state, not attempt count
		successfulChannels := make(map[string]bool)
		failedChannels := make(map[string]bool)

		for _, a := range notification.Status.DeliveryAttempts {
			switch a.Status {
			case notificationv1alpha1.DeliveryAttemptStatusSuccess:
				successfulChannels[string(a.Channel)] = true
				delete(failedChannels, string(a.Channel)) // Remove from failed if it later succeeds
			case notificationv1alpha1.DeliveryAttemptStatusFailed:
				// Only count as failed if the channel never succeeded
				if !successfulChannels[string(a.Channel)] {
					failedChannels[string(a.Channel)] = true
				}
			}
		}

		notification.Status.SuccessfulDeliveries = len(successfulChannels)
		notification.Status.FailedDeliveries = len(failedChannels)

		// 4. Update status using status subresource
		if err := m.client.Status().Update(ctx, notification); err != nil {
			return fmt.Errorf("failed to record delivery attempt: %w", err)
		}

		return nil
	})
}

// AtomicStatusUpdate atomically updates phase and delivery attempts in a single API call
// This prevents race conditions and reduces API server load (1 write instead of N+1 writes)
//
// This method consolidates:
// - Phase transition (UpdatePhase)
// - Multiple delivery attempt records (RecordDeliveryAttempt x N)
//
// Satisfies BR-NOT-051, BR-NOT-056, and improves performance by 50-90%
func (m *Manager) AtomicStatusUpdate(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	newPhase notificationv1alpha1.NotificationPhase,
	reason string,
	message string,
	attempts []notificationv1alpha1.DeliveryAttempt,
	conditions []metav1.Condition,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion (bypasses cache - DD-STATUS-001)
		if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(notification), notification); err != nil {
			return fmt.Errorf("failed to refetch notification: %w", err)
		}

		// DD-STATUS-001: Debug logging to diagnose cache issues
		ctrl.Log.WithName("status-manager").Info("DD-STATUS-001: API reader refetch complete",
			"deliveryAttemptsBeforeUpdate", len(notification.Status.DeliveryAttempts),
			"newAttemptsToAdd", len(attempts),
			"conditionsToApply", len(conditions))

		// 2. Re-apply conditions after refetch to prevent in-memory conditions from being wiped
		for _, c := range conditions {
			meta.SetStatusCondition(&notification.Status.Conditions, c)
		}

		// 3. Validate phase transition (if phase is changing)
		if err := applyPhaseTransition(notification, newPhase, reason, message); err != nil {
			return err
		}

		// 4. Record all delivery attempts atomically (de-duplicated)
		mergeNewDeliveryAttempts(notification, attempts)

		// BR-NOT-051: Calculate counters based on UNIQUE CHANNELS, not total attempts
		recalculateDeliveryCounters(notification)

		// DD-CONTROLLER-001: Track processed generation to prevent duplicate reconciles
		notification.Status.ObservedGeneration = notification.Generation

		// 5. SINGLE ATOMIC UPDATE: All changes committed together
		if err := m.client.Status().Update(ctx, notification); err != nil {
			return fmt.Errorf("failed to atomically update status: %w", err)
		}

		return nil
	})
}

// applyPhaseTransition validates and (if changing) applies the new phase,
// reason, message, and Issue #118 lifecycle timestamps (QueuedAt,
// ProcessingStartedAt, CompletionTime) to the notification's status.
// No-op when the phase is unchanged. Extracted from AtomicStatusUpdate
// (Wave 6 6b GREEN: gocognit remediation) — pure code motion, no behavior
// change.
func applyPhaseTransition(notification *notificationv1alpha1.NotificationRequest, newPhase notificationv1alpha1.NotificationPhase, reason, message string) error {
	if notification.Status.Phase == newPhase {
		return nil
	}
	if !isValidPhaseTransition(notification.Status.Phase, newPhase) {
		return fmt.Errorf("invalid phase transition from %s to %s", notification.Status.Phase, newPhase)
	}

	notification.Status.Phase = newPhase
	notification.Status.Reason = notificationv1alpha1.NotificationStatusReason(reason)
	notification.Status.Message = message

	// Issue #118 Gap 6+7: Set lifecycle timestamps on phase transitions
	now := metav1.Now()
	if newPhase == notificationv1alpha1.NotificationPhasePending && notification.Status.QueuedAt == nil {
		notification.Status.QueuedAt = &now
	}
	if newPhase == notificationv1alpha1.NotificationPhaseSending && notification.Status.ProcessingStartedAt == nil {
		notification.Status.ProcessingStartedAt = &now
	}
	if isTerminalPhase(newPhase) {
		notification.Status.CompletionTime = &now
	}
	return nil
}

// mergeNewDeliveryAttempts appends the given attempts to the notification's
// status, de-duplicating against already-recorded attempts to prevent
// concurrent reconciles from double-recording the same attempt.
//
// BUG FIX (Jan 22, 2026): Relaxed deduplication to only reject truly
// identical attempts (same channel, status, error, and timestamp within 1s).
// We no longer check attempt number because concurrent reconciles can assign
// the same attempt# before the previous status update propagates (even with
// apiReader cache bypass).
//
// Extracted from AtomicStatusUpdate (Wave 6 6b GREEN: gocognit
// remediation) — pure code motion, no behavior change.
func mergeNewDeliveryAttempts(notification *notificationv1alpha1.NotificationRequest, attempts []notificationv1alpha1.DeliveryAttempt) {
	for _, attempt := range attempts {
		if isDuplicateStatusAttempt(notification.Status.DeliveryAttempts, attempt) {
			continue
		}
		notification.Status.DeliveryAttempts = append(notification.Status.DeliveryAttempts, attempt)
		notification.Status.TotalAttempts++
	}
}

// isDuplicateStatusAttempt checks whether an exact duplicate of attempt
// (same channel, status, and error message at the same time, within 1s)
// already exists in the recorded attempts. Extracted from
// mergeNewDeliveryAttempts (Wave 6 6b GREEN: gocognit remediation) — pure
// code motion, no behavior change.
func isDuplicateStatusAttempt(existing []notificationv1alpha1.DeliveryAttempt, attempt notificationv1alpha1.DeliveryAttempt) bool {
	for _, e := range existing {
		if e.Channel == attempt.Channel &&
			e.Status == attempt.Status &&
			e.Error == attempt.Error &&
			abs(e.Timestamp.Sub(attempt.Timestamp.Time)) < time.Second {
			return true
		}
	}
	return false
}

// recalculateDeliveryCounters recomputes SuccessfulDeliveries/FailedDeliveries
// based on UNIQUE CHANNELS rather than total attempts (BR-NOT-051,
// DD-E2E-003). Example: 1 console success + 5 file failures yields
// SuccessfulDeliveries=1, FailedDeliveries=1. Extracted from
// AtomicStatusUpdate (Wave 6 6b GREEN: gocognit remediation) — pure code
// motion, no behavior change.
func recalculateDeliveryCounters(notification *notificationv1alpha1.NotificationRequest) {
	successfulChannels := make(map[string]bool)
	failedChannels := make(map[string]bool)

	for _, attempt := range notification.Status.DeliveryAttempts {
		switch attempt.Status {
		case notificationv1alpha1.DeliveryAttemptStatusSuccess:
			successfulChannels[string(attempt.Channel)] = true
			delete(failedChannels, string(attempt.Channel)) // Remove from failed if it later succeeds
		case notificationv1alpha1.DeliveryAttemptStatusFailed:
			// Only count as failed if the channel never succeeded
			if !successfulChannels[string(attempt.Channel)] {
				failedChannels[string(attempt.Channel)] = true
			}
		}
	}

	notification.Status.SuccessfulDeliveries = len(successfulChannels)
	notification.Status.FailedDeliveries = len(failedChannels)
}

// UpdatePhase updates the NotificationRequest phase with validation
// Satisfies BR-NOT-056: CRD Lifecycle Management
//
// NOTE: For phase transitions that include delivery attempts, use AtomicStatusUpdate instead
// to reduce API calls and eliminate race conditions.
//
// This method uses retry logic to handle optimistic locking conflicts.
// See: docs/development/business-requirements/DEVELOPMENT_GUIDELINES.md
func (m *Manager) UpdatePhase(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, newPhase notificationv1alpha1.NotificationPhase, reason, message string, conditions []metav1.Condition) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion (bypasses cache - DD-STATUS-001)
		if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(notification), notification); err != nil {
			return fmt.Errorf("failed to refetch notification: %w", err)
		}

		// 2. Re-apply conditions after refetch to prevent in-memory conditions from being wiped
		for _, c := range conditions {
			meta.SetStatusCondition(&notification.Status.Conditions, c)
		}

		// 3. Validate phase transition
		if !isValidPhaseTransition(notification.Status.Phase, newPhase) {
			return fmt.Errorf("invalid phase transition from %s to %s", notification.Status.Phase, newPhase)
		}

		// 4. Update phase fields
		notification.Status.Phase = newPhase
		notification.Status.Reason = notificationv1alpha1.NotificationStatusReason(reason)
		notification.Status.Message = message

		// Issue #118 Gap 6+7: Set lifecycle timestamps on phase transitions
		now := metav1.Now()
		if newPhase == notificationv1alpha1.NotificationPhasePending && notification.Status.QueuedAt == nil {
			notification.Status.QueuedAt = &now
		}
		if newPhase == notificationv1alpha1.NotificationPhaseSending && notification.Status.ProcessingStartedAt == nil {
			notification.Status.ProcessingStartedAt = &now
		}

		// 5. Set completion time for terminal phases
		if isTerminalPhase(newPhase) {
			notification.Status.CompletionTime = &now
		}

		// DD-CONTROLLER-001: Track processed generation to prevent duplicate reconciles
		notification.Status.ObservedGeneration = notification.Generation

		// 6. Update status using status subresource
		if err := m.client.Status().Update(ctx, notification); err != nil {
			return fmt.Errorf("failed to update phase: %w", err)
		}

		return nil
	})
}

// UpdateObservedGeneration updates the ObservedGeneration to match Generation
// Satisfies BR-NOT-051: Complete Audit Trail (generation tracking)
//
// This method uses retry logic to handle optimistic locking conflicts.
// See: docs/development/business-requirements/DEVELOPMENT_GUIDELINES.md
func (m *Manager) UpdateObservedGeneration(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion
		if err := m.client.Get(ctx, client.ObjectKeyFromObject(notification), notification); err != nil {
			return fmt.Errorf("failed to refetch notification: %w", err)
		}

		// 2. Update ObservedGeneration
		notification.Status.ObservedGeneration = notification.Generation

		// 3. Update status using status subresource
		if err := m.client.Status().Update(ctx, notification); err != nil {
			return fmt.Errorf("failed to update observed generation: %w", err)
		}

		return nil
	})
}

// isValidPhaseTransition checks if a phase transition is valid
// ========================================
// PATTERN 1: Use Centralized Phase Logic
// 📋 Design Decision: Controller Refactoring Pattern Library §1
// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
// ========================================
//
// This function delegates to pkg/notification/phase.CanTransition()
// to maintain a single source of truth for phase transitions.
//
// BENEFITS:
// - ✅ Single source of truth (pkg/notification/phase/types.go)
// - ✅ Consistent validation across controller and status manager
// - ✅ Includes initial phase transition ("" → Pending)
// - ✅ Easy to maintain (update once, applies everywhere)
// ========================================
func isValidPhaseTransition(current, new notificationv1alpha1.NotificationPhase) bool {
	// Use centralized phase transition validation (Pattern 1)
	return notificationphase.CanTransition(current, new)
}

// isTerminalPhase checks if a phase is terminal (no further transitions allowed)
// ========================================
// PATTERN 1: Use Centralized Phase Logic
// 📋 Design Decision: Controller Refactoring Pattern Library §1
// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
// ========================================
//
// This function delegates to pkg/notification/phase.IsTerminal()
// to maintain a single source of truth for terminal state detection.
//
// BENEFITS:
// - ✅ Single source of truth (pkg/notification/phase/types.go)
// - ✅ Consistent with phase state machine
// - ✅ Easy to maintain (add terminal phase once, applies everywhere)
// ========================================
func isTerminalPhase(phase notificationv1alpha1.NotificationPhase) bool {
	// Use centralized terminal phase detection (Pattern 1)
	return notificationphase.IsTerminal(phase)
}

// abs returns the absolute value of a duration
func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
