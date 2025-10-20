package status

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// Manager handles NotificationRequest status updates
type Manager struct {
	client client.Client
}

// NewManager creates a new status manager
func NewManager(client client.Client) *Manager {
	return &Manager{
		client: client,
	}
}

// RecordDeliveryAttempt records a delivery attempt in the NotificationRequest status
// Satisfies BR-NOT-051: Complete Audit Trail
func (m *Manager) RecordDeliveryAttempt(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, attempt notificationv1alpha1.DeliveryAttempt) error {
	// Append the attempt to the delivery attempts list
	notification.Status.DeliveryAttempts = append(notification.Status.DeliveryAttempts, attempt)

	// Update counters
	notification.Status.TotalAttempts++

	if attempt.Status == "success" {
		notification.Status.SuccessfulDeliveries++
	} else if attempt.Status == "failed" {
		notification.Status.FailedDeliveries++
	}

	// Update status using status subresource
	if err := m.client.Status().Update(ctx, notification); err != nil {
		return fmt.Errorf("failed to record delivery attempt: %w", err)
	}

	return nil
}

// UpdatePhase updates the NotificationRequest phase with validation
// Satisfies BR-NOT-056: CRD Lifecycle Management
func (m *Manager) UpdatePhase(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, newPhase notificationv1alpha1.NotificationPhase, reason, message string) error {
	// Validate phase transition
	if !isValidPhaseTransition(notification.Status.Phase, newPhase) {
		return fmt.Errorf("invalid phase transition from %s to %s", notification.Status.Phase, newPhase)
	}

	// Update phase fields
	notification.Status.Phase = newPhase
	notification.Status.Reason = reason
	notification.Status.Message = message

	// Set completion time for terminal phases
	if isTerminalPhase(newPhase) {
		now := metav1.Now()
		notification.Status.CompletionTime = &now
	}

	// Update status using status subresource
	if err := m.client.Status().Update(ctx, notification); err != nil {
		return fmt.Errorf("failed to update phase: %w", err)
	}

	return nil
}

// UpdateObservedGeneration updates the ObservedGeneration to match Generation
// Satisfies BR-NOT-051: Complete Audit Trail (generation tracking)
func (m *Manager) UpdateObservedGeneration(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	notification.Status.ObservedGeneration = notification.Generation

	// Update status using status subresource
	if err := m.client.Status().Update(ctx, notification); err != nil {
		return fmt.Errorf("failed to update observed generation: %w", err)
	}

	return nil
}

// isValidPhaseTransition checks if a phase transition is valid
func isValidPhaseTransition(current, new notificationv1alpha1.NotificationPhase) bool {
	// Terminal phases cannot transition to any other phase
	if isTerminalPhase(current) {
		return false
	}

	// Valid transitions:
	// Pending → Sending
	// Sending → Sent | Failed | PartiallySent
	validTransitions := map[notificationv1alpha1.NotificationPhase][]notificationv1alpha1.NotificationPhase{
		notificationv1alpha1.NotificationPhasePending: {
			notificationv1alpha1.NotificationPhaseSending,
		},
		notificationv1alpha1.NotificationPhaseSending: {
			notificationv1alpha1.NotificationPhaseSent,
			notificationv1alpha1.NotificationPhaseFailed,
			notificationv1alpha1.NotificationPhasePartiallySent,
		},
	}

	allowedTransitions, ok := validTransitions[current]
	if !ok {
		return false
	}

	for _, allowed := range allowedTransitions {
		if new == allowed {
			return true
		}
	}

	return false
}

// isTerminalPhase checks if a phase is terminal (no further transitions allowed)
func isTerminalPhase(phase notificationv1alpha1.NotificationPhase) bool {
	terminalPhases := []notificationv1alpha1.NotificationPhase{
		notificationv1alpha1.NotificationPhaseSent,
		notificationv1alpha1.NotificationPhaseFailed,
		notificationv1alpha1.NotificationPhasePartiallySent,
	}

	for _, terminal := range terminalPhases {
		if phase == terminal {
			return true
		}
	}

	return false
}
